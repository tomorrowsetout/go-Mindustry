package main

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/pierrec/lz4/v4"

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
	payload := make([]byte, size)
	switch comp {
	case 0:
		if _, err := io.ReadFull(r, payload); err != nil {
			t.Fatalf("read framed payload failed: %v", err)
		}
	case 1:
		compressed := make([]byte, r.Len())
		if _, err := io.ReadFull(r, compressed); err != nil {
			t.Fatalf("read compressed framed payload failed: %v", err)
		}
		if _, err := lz4.UncompressBlock(compressed, payload); err != nil {
			t.Fatalf("decompress framed payload failed: %v", err)
		}
	default:
		t.Fatalf("unexpected compressed packet flag in test: comp=%d", comp)
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
	nodePos := placeSyncTestBuilding(t, model, 6, 10, 425, 1, 0)
	targetPos := placeSyncTestBuilding(t, model, 12, 10, 430, 1, 0)
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

func TestSendBlockSnapshotsToConnEmitsBlockSnapshotPayload(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 156)
	blockSnapshotID, ok := srv.Registry.PacketID(&protocol.Remote_NetClient_blockSnapshot_7{})
	if !ok {
		t.Fatal("resolve blockSnapshot packet id")
	}
	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	sendConn := netserver.NewConn(serverSide, srv.Serial)
	defer sendConn.Close()

	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		500: "container",
	}
	containerPos := placeSyncTestBuilding(t, model, 9, 8, 500, 1, 0)
	tile, err := model.TileAt(9, 8)
	if err != nil || tile == nil || tile.Build == nil {
		t.Fatalf("container tile lookup failed: %v", err)
	}
	tile.Build.Items = []world.ItemStack{{Item: 0, Amount: 37}}
	w.SetModel(model)

	sendBlockSnapshotsToConn(sendConn, w)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 4)

	foundSnapshot := false
	descriptions := make([]string, 0, len(packets))
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, payload := decodeFramedPacket(t, packet)
		if packetID != blockSnapshotID {
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d", packetID))
			continue
		}
		typed := &protocol.Remote_NetClient_blockSnapshot_7{}
		if err := typed.Read(protocol.NewReader(payload), 0); err != nil {
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d(decode-failed)", packetID))
			continue
		}
		descriptions = append(descriptions, fmt.Sprintf("packetID=%d blockSnapshot(amount=%d)", packetID, typed.Amount))
		r := protocol.NewReader(typed.Data)
		for i := 0; i < int(typed.Amount); i++ {
			pos, err := r.ReadInt32()
			if err != nil {
				t.Fatalf("read blockSnapshot pos failed: %v", err)
			}
			blockID, err := r.ReadInt16()
			if err != nil {
				t.Fatalf("read blockSnapshot block failed: %v", err)
			}
			if pos == containerPos && blockID == 500 {
				foundSnapshot = true
				break
			}
			if _, err := protocol.ReadBytes(r); err != nil {
				t.Fatalf("read blockSnapshot payload failed: %v", err)
			}
		}
	}

	if !foundSnapshot {
		t.Fatalf("expected sendBlockSnapshotsToConn to emit blockSnapshot payload for synced container, got packets: %s", strings.Join(descriptions, ", "))
	}
}

func TestSyncWorldDiffToConnIncludesPowerNodeBlockSnapshotWithoutTileDiff(t *testing.T) {
	srv := netserver.NewServer("127.0.0.1:0", 156)
	blockSnapshotID, ok := srv.Registry.PacketID(&protocol.Remote_NetClient_blockSnapshot_7{})
	if !ok {
		t.Fatal("resolve blockSnapshot packet id")
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
	nodePos := placeSyncTestBuilding(t, model, 6, 10, 425, 1, 0)
	targetPos := placeSyncTestBuilding(t, model, 12, 10, 430, 1, 0)
	w.SetModel(model)
	w.ConfigureBuildingPacked(nodePos, targetPos)
	baseModel := w.CloneModel()
	if baseModel == nil {
		t.Fatal("expected clone model for diff sync test")
	}

	syncWorldDiffToConn(sendConn, w, baseModel)
	packets := collectRawPacketsUntilTimeout(t, clientSide, 8)

	foundSnapshot := false
	descriptions := make([]string, 0, len(packets))
	for _, packet := range packets {
		if len(packet) == 0 {
			continue
		}
		packetID, payload := decodeFramedPacket(t, packet)
		if packetID != blockSnapshotID {
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d", packetID))
			continue
		}
		typed := &protocol.Remote_NetClient_blockSnapshot_7{}
		if err := typed.Read(protocol.NewReader(payload), 0); err != nil {
			descriptions = append(descriptions, fmt.Sprintf("packetID=%d", packetID))
			continue
		}
		descriptions = append(descriptions, fmt.Sprintf("packetID=%d blockSnapshot(amount=%d)", packetID, typed.Amount))
		r := protocol.NewReader(typed.Data)
		for i := 0; i < int(typed.Amount); i++ {
			pos, err := r.ReadInt32()
			if err != nil {
				t.Fatalf("read blockSnapshot pos failed: %v", err)
			}
			blockID, err := r.ReadInt16()
			if err != nil {
				t.Fatalf("read blockSnapshot block failed: %v", err)
			}
			if pos == nodePos && blockID == 425 {
				foundSnapshot = true
				break
			}
			if _, err := protocol.ReadBytes(r); err != nil {
				t.Fatalf("read blockSnapshot payload failed: %v", err)
			}
		}
	}

	if !foundSnapshot {
		t.Fatalf("expected diff sync to emit blockSnapshot payload for unchanged power node runtime, got packets: %s", strings.Join(descriptions, ", "))
	}
}
