package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	netserver "mdt-server/internal/net"
	"mdt-server/internal/protocol"
	"mdt-server/internal/worldstream"
)

type pendingStream struct {
	id     int32
	total  int32
	typ    byte
	buffer bytes.Buffer
}

func main() {
	addr := flag.String("addr", "127.0.0.1:6567", "server address")
	name := flag.String("name", "headless-probe", "player name")
	uuid := flag.String("uuid", "", "base64 16-byte uuid; generated when empty")
	usid := flag.String("usid", "headless-probe-usid", "usid")
	locale := flag.String("locale", "en", "locale")
	out := flag.String("out", "probe-worldstream.bin", "path to save compressed worldstream payload")
	timeout := flag.Duration("timeout", 20*time.Second, "probe timeout")
	useUDP := flag.Bool("udp", true, "send raw UDP registration after RegisterTCP")
	sendConfirm := flag.Bool("confirm", true, "send connectConfirm after worldstream inspection")
	linger := flag.Duration("linger", 5*time.Second, "time to keep the connection open after connectConfirm")
	flag.Parse()

	if *uuid == "" {
		*uuid = randomUUID()
	}

	if err := run(*addr, *name, *uuid, *usid, *locale, *out, *timeout, *useUDP, *sendConfirm, *linger); err != nil {
		fmt.Fprintf(os.Stderr, "[probe] failed: %v\n", err)
		os.Exit(1)
	}
}

func run(addr, name, uuid, usid, locale, out string, timeout time.Duration, useUDP, sendConfirm bool, linger time.Duration) error {
	reg := protocol.NewRegistry()
	content := protocol.NewContentRegistry()
	serial := &netserver.Serializer{Registry: reg, Ctx: content.Context()}
	worldID, ok := reg.PacketID(&protocol.WorldStream{})
	if !ok {
		return fmt.Errorf("worldstream packet id not found")
	}

	conn, err := net.DialTimeout("tcp", addr, timeout)
	if err != nil {
		return err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	fmt.Printf("[probe] tcp connected addr=%s\n", addr)

	streams := map[int32]*pendingStream{}
	connectionID := int32(0)
	for {
		obj, stream, err := readNext(conn, serial, streams)
		if err != nil {
			return fmt.Errorf("before ConnectPacket read failed: %w", err)
		}
		if stream != nil {
			return fmt.Errorf("unexpected stream before ConnectPacket: type=%d len=%d", stream.typ, stream.buffer.Len())
		}
		fmt.Printf("[probe] recv %T\n", obj)
		if pkt, ok := obj.(*protocol.RegisterTCP); ok {
			connectionID = pkt.ConnectionID
			break
		}
	}

	if useUDP {
		if err := sendUDPRegister(addr, connectionID); err != nil {
			fmt.Printf("[probe] udp register failed: %v\n", err)
		} else {
			fmt.Printf("[probe] udp register sent id=%d\n", connectionID)
		}
	}

	connect := &protocol.ConnectPacket{
		Version:     157,
		VersionType: "official",
		Mods:        nil,
		Name:        name,
		Locale:      locale,
		UUID:        uuid,
		USID:        usid,
		Mobile:      false,
		Color:       -1,
	}
	if err := writeObject(conn, serial, connect); err != nil {
		return fmt.Errorf("send ConnectPacket: %w", err)
	}
	fmt.Printf("[probe] sent ConnectPacket name=%q uuid=%s usid=%s\n", name, uuid, usid)

	worldReceived := false
	readUntil := time.Now().Add(timeout)
	for time.Now().Before(readUntil) {
		obj, stream, err := readNext(conn, serial, streams)
		if err != nil {
			if worldReceived {
				fmt.Printf("[probe] connection closed after worldstream: %v\n", err)
				return nil
			}
			return fmt.Errorf("read failed before worldstream complete: %w", err)
		}
		if stream != nil {
			fmt.Printf("[probe] stream complete id=%d type=%d bytes=%d total=%d\n", stream.id, stream.typ, stream.buffer.Len(), stream.total)
			if stream.typ != worldID {
				continue
			}
			payload := append([]byte(nil), stream.buffer.Bytes()...)
			if out != "" {
				if err := os.WriteFile(out, payload, 0o644); err != nil {
					return fmt.Errorf("save worldstream: %w", err)
				}
				fmt.Printf("[probe] saved worldstream=%s bytes=%d\n", out, len(payload))
			}
			inspection, err := worldstream.InspectWorldStreamPayload(payload)
			if err != nil {
				return fmt.Errorf("inspect worldstream: %w", err)
			}
			fmt.Printf("[probe] worldstream inspect compressed=%d raw=%d player=%d..%d content=%d..%d patchesEnd=%d mapEnd=%d teamBlocks=%d tail=%d tailPrefix=%s\n",
				inspection.CompressedLen,
				inspection.RawLen,
				inspection.PlayerStart,
				inspection.PlayerEnd,
				inspection.ContentStart,
				inspection.ContentEnd,
				inspection.PatchesEnd,
				inspection.MapEnd,
				inspection.TeamBlocksLen,
				inspection.TailLen,
				inspection.TailPrefixHex)
			worldReceived = true
			if sendConfirm {
				if err := writeObject(conn, serial, &protocol.Remote_NetServer_connectConfirm_50{}); err != nil {
					return fmt.Errorf("send connectConfirm: %w", err)
				}
				fmt.Printf("[probe] sent connectConfirm\n")
			}
			continue
		}
		fmt.Printf("[probe] recv %T\n", obj)
		switch pkt := obj.(type) {
		case *protocol.Remote_NetClient_kick_21:
			return fmt.Errorf("kicked enum=%v", pkt.Reason)
		case *protocol.Remote_NetClient_kick_22:
			return fmt.Errorf("kicked reason=%s", pkt.Reason)
		case *protocol.RegisterUDP:
			fmt.Printf("[probe] tcp udp-register ack id=%d\n", pkt.ConnectionID)
		}
		if worldReceived && linger <= 0 {
			return nil
		}
	}
	if !worldReceived {
		return fmt.Errorf("timed out waiting for worldstream")
	}
	if linger > 0 {
		fmt.Printf("[probe] lingering for %s after connectConfirm\n", linger)
		_ = conn.SetDeadline(time.Now().Add(linger))
		end := time.Now().Add(linger)
		for time.Now().Before(end) {
			obj, stream, err := readNext(conn, serial, streams)
			if err != nil {
				if ne, ok := err.(net.Error); ok && ne.Timeout() {
					break
				}
				return fmt.Errorf("post-connect read failed: %w", err)
			}
			if stream != nil {
				fmt.Printf("[probe] post-connect unexpected stream type=%d len=%d\n", stream.typ, stream.buffer.Len())
				continue
			}
			fmt.Printf("[probe] post-connect recv %T\n", obj)
		}
	}
	return nil
}

func readNext(conn net.Conn, serial *netserver.Serializer, streams map[int32]*pendingStream) (any, *pendingStream, error) {
	for {
		obj, err := readObject(conn, serial)
		if err != nil {
			return nil, nil, err
		}
		switch pkt := obj.(type) {
		case *protocol.StreamBegin:
			streams[pkt.ID] = &pendingStream{id: pkt.ID, total: pkt.Total, typ: pkt.Type}
			fmt.Printf("[probe] stream begin id=%d type=%d total=%d\n", pkt.ID, pkt.Type, pkt.Total)
		case *protocol.StreamChunk:
			stream := streams[pkt.ID]
			if stream == nil {
				fmt.Printf("[probe] orphan stream chunk id=%d len=%d\n", pkt.ID, len(pkt.Data))
				continue
			}
			stream.buffer.Write(pkt.Data)
			fmt.Printf("[probe] stream chunk id=%d len=%d progress=%d/%d\n", pkt.ID, len(pkt.Data), stream.buffer.Len(), stream.total)
			if int32(stream.buffer.Len()) >= stream.total {
				delete(streams, pkt.ID)
				return nil, stream, nil
			}
		default:
			return obj, nil, nil
		}
	}
}

func readObject(conn net.Conn, serial *netserver.Serializer) (any, error) {
	lenbuf := make([]byte, 2)
	if _, err := io.ReadFull(conn, lenbuf); err != nil {
		return nil, err
	}
	n := binary.BigEndian.Uint16(lenbuf)
	payload := make([]byte, n)
	if _, err := io.ReadFull(conn, payload); err != nil {
		return nil, err
	}
	return serial.ReadObject(bytes.NewReader(payload))
}

func writeObject(conn net.Conn, serial *netserver.Serializer, obj any) error {
	var payload bytes.Buffer
	if err := serial.WriteObject(&payload, obj); err != nil {
		return err
	}
	if payload.Len() > 0xffff {
		return fmt.Errorf("payload too large: %d", payload.Len())
	}
	lenbuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenbuf, uint16(payload.Len()))
	if _, err := conn.Write(lenbuf); err != nil {
		return err
	}
	_, err := conn.Write(payload.Bytes())
	return err
}

func sendUDPRegister(addr string, connectionID int32) error {
	udpAddr, err := net.ResolveUDPAddr("udp", addr)
	if err != nil {
		return err
	}
	conn, err := net.DialUDP("udp", nil, udpAddr)
	if err != nil {
		return err
	}
	defer conn.Close()
	buf := make([]byte, 6)
	buf[0] = 0xfe
	buf[1] = protocol.FrameworkRegisterUD
	binary.BigEndian.PutUint32(buf[2:], uint32(connectionID))
	_, err = conn.Write(buf)
	return err
}

func randomUUID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		for i := range b {
			b[i] = byte(i + 1)
		}
	}
	return base64.StdEncoding.EncodeToString(b)
}
