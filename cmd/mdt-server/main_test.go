package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	netserver "mdt-server/internal/net"
	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func placeSyncTestBuilding(t *testing.T, model *world.WorldModel, x, y int, block int16, team world.TeamID, rotation int8) int32 {
	t.Helper()
	tile, err := model.TileAt(x, y)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed at (%d,%d): %v", x, y, err)
	}
	tile.Block = world.BlockID(block)
	tile.Team = team
	tile.Rotation = rotation
	tile.Build = &world.Building{
		Block:     world.BlockID(block),
		Team:      team,
		Rotation:  rotation,
		X:         x,
		Y:         y,
		Health:    1000,
		MaxHealth: 1000,
	}
	return protocol.PackPoint2(int32(x), int32(y))
}

func collectRawPacketsUntilTimeout(t *testing.T, conn net.Conn, max int) [][]byte {
	t.Helper()
	out := make([][]byte, 0, max)
	for len(out) < max {
		_ = conn.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
		lenBuf := make([]byte, 2)
		_, err := io.ReadFull(conn, lenBuf)
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			t.Fatalf("read packet length failed: %v", err)
		}
		size := int(lenBuf[0])<<8 | int(lenBuf[1])
		if size <= 0 {
			t.Fatalf("invalid packet length %d", size)
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(conn, payload); err != nil {
			t.Fatalf("read packet payload failed: %v", err)
		}
		out = append(out, payload)
	}
	return out
}

func decodeFramedPacket(t *testing.T, framed []byte) (byte, []byte) {
	t.Helper()
	r := bytes.NewReader(framed)
	packetID, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read packet id failed: %v", err)
	}
	lenBuf := make([]byte, 2)
	if _, err := io.ReadFull(r, lenBuf); err != nil {
		t.Fatalf("read packet payload length failed: %v", err)
	}
	size := int(lenBuf[0])<<8 | int(lenBuf[1])
	comp, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read packet compression flag failed: %v", err)
	}
	if comp != 0 {
		t.Fatalf("unexpected compressed packet in test: comp=%d", comp)
	}
	payload := make([]byte, size)
	if _, err := io.ReadFull(r, payload); err != nil {
		t.Fatalf("read framed payload failed: %v", err)
	}
	return packetID, payload
}

func TestSyncCurrentWorldToConnIncludesBuildingConfigs(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 156)
	srv.TypeIO.BuildingLookup = func(pos int32) protocol.Building {
		return protocol.BuildingBox{PosValue: pos}
	}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		425: "power-node-large",
		430: "laser-drill",
	}
	nodePos := placeSyncTestBuilding(t, model, 12, 10, 425, 1, 0)
	targetPos := placeSyncTestBuilding(t, model, 6, 10, 430, 1, 0)
	w.SetModel(model)
	w.ConfigureBuildingPacked(nodePos, targetPos)

	syncCurrentWorldToConn(sendConn, w)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 8)

	var foundNodeTile bool
	var foundNodeConfig bool
	descriptions := make([]string, 0, len(packets))
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, payload := decodeFramedPacket(t, packet)
		switch packetID {
		case 114:
			typed := &protocol.Remote_Tile_setTile_110{}
			r := protocol.NewReaderWithContext(payload, srv.TypeIO)
			if err := typed.Read(r, 0); err != nil {
				t.Fatalf("decode setTile failed: %v", err)
			}
			tilePos := int32(-1)
			if typed.Tile != nil {
				tilePos = typed.Tile.Pos()
			}
			descriptions = append(descriptions, fmt.Sprintf("setTile(pos=%d)", tilePos))
			if typed.Tile != nil && typed.Tile.Pos() == nodePos {
				foundNodeTile = true
			}
		case 131:
			r := protocol.NewReaderWithContext(payload, srv.TypeIO)
			if _, err := protocol.ReadEntity(r, srv.TypeIO); err != nil {
				t.Fatalf("decode tileConfig player failed: %v", err)
			}
			build, err := protocol.ReadBuilding(r, srv.TypeIO)
			if err != nil {
				t.Fatalf("decode tileConfig building failed: %v", err)
			}
			value, err := protocol.ReadObject(r, false, srv.TypeIO)
			if err != nil {
				t.Fatalf("decode tileConfig value failed: %v", err)
			}
			buildPos := int32(-1)
			if build != nil {
				buildPos = build.Pos()
			}
			descriptions = append(descriptions, fmt.Sprintf("tileConfig(pos=%d,value=%T)", buildPos, value))
			if build == nil || build.Pos() != nodePos {
				continue
			}
			links, ok := value.([]protocol.Point2)
			if !ok {
				t.Fatalf("expected power-node config payload as []Point2, got %T", value)
			}
			if len(links) != 1 {
				t.Fatalf("expected one power-node link, got %d", len(links))
			}
			target := protocol.UnpackPoint2(targetPos)
			node := protocol.UnpackPoint2(nodePos)
			want := protocol.Point2{X: target.X - node.X, Y: target.Y - node.Y}
			if links[0] != want {
				t.Fatalf("expected power-node link %+v, got %+v", want, links[0])
			}
			foundNodeConfig = true
		default:
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d", packetID))
		}
	}

	if !foundNodeTile {
		t.Fatalf("expected syncCurrentWorldToConn to send setTile for the power node, got packets: %s", strings.Join(descriptions, ", "))
	}
	if !foundNodeConfig {
		t.Fatalf("expected syncCurrentWorldToConn to send tileConfig for the power node, got packets: %s", strings.Join(descriptions, ", "))
	}
}
