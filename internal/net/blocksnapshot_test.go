package net

import (
	"bytes"
	"io"
	"net"
	"testing"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func TestBuildBlockSnapshotPacketsKeepsOfficialPayloadLimit(t *testing.T) {
	largeA := bytes.Repeat([]byte{1}, 400)
	largeB := bytes.Repeat([]byte{2}, 400)
	snaps := []world.BlockSyncSnapshot{
		{Pos: protocol.PackPoint2(2, 3), BlockID: 500, Data: largeA},
		{Pos: protocol.PackPoint2(4, 5), BlockID: 910, Data: largeB},
	}

	packets := BuildBlockSnapshotPackets(snaps)
	if len(packets) != 2 {
		t.Fatalf("expected 2 packets after payload split, got %d", len(packets))
	}
	for i, packet := range packets {
		if packet == nil {
			t.Fatalf("packet[%d] is nil", i)
		}
		if packet.Amount != 1 {
			t.Fatalf("expected packet[%d] amount=1, got %d", i, packet.Amount)
		}
		if len(packet.Data) > maxBlockSnapshotPayloadBytes {
			t.Fatalf("expected packet[%d] to stay within %d bytes, got %d", i, maxBlockSnapshotPayloadBytes, len(packet.Data))
		}
	}
}

func TestBuildIsolatedBlockSnapshotPacketsKeepsOneEntryPerPacket(t *testing.T) {
	packets := BuildIsolatedBlockSnapshotPackets([]world.BlockSyncSnapshot{
		{Pos: protocol.PackPoint2(2, 3), BlockID: 500, Data: []byte{1, 2, 3}},
		{Pos: protocol.PackPoint2(4, 5), BlockID: 910, Data: []byte{4, 5, 6}},
	})
	if len(packets) != 2 {
		t.Fatalf("expected 2 isolated packets, got %d", len(packets))
	}
	for i, packet := range packets {
		if packet == nil {
			t.Fatalf("packet[%d] is nil", i)
		}
		if packet.Amount != 1 {
			t.Fatalf("expected isolated packet[%d] amount=1, got %d", i, packet.Amount)
		}
	}
}

func TestSyncBlockSnapshotsToConnUsesUnreliableTransport(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	udpCapture := installUDPCapture(srv)
	udpAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50021}

	tcpConn, peer := net.Pipe()
	defer tcpConn.Close()
	defer peer.Close()

	conn := NewConn(tcpConn, srv.Serial)
	defer conn.Close()
	conn.playerID = 21
	conn.setUDPAddr(udpAddr)

	snaps := []world.BlockSyncSnapshot{
		{Pos: protocol.PackPoint2(9, 8), BlockID: 500, Data: []byte{1, 2, 3, 4}},
	}
	if err := srv.SyncBlockSnapshotsToConn(conn, snaps); err != nil {
		t.Fatalf("SyncBlockSnapshotsToConn: %v", err)
	}

	udpPacket := udpCapture.read(t)
	if got := udpPacket.addr.String(); got != udpAddr.String() {
		t.Fatalf("udp addr mismatch got=%s want=%s", got, udpAddr.String())
	}
	obj, err := srv.Serial.ReadObject(bytes.NewReader(udpPacket.payload))
	if err != nil {
		t.Fatalf("decode udp blockSnapshot: %v", err)
	}
	packet, ok := obj.(*protocol.Remote_NetClient_blockSnapshot_34)
	if !ok {
		t.Fatalf("expected blockSnapshot packet, got %T", obj)
	}
	if packet.Amount != 1 {
		t.Fatalf("expected one blockSnapshot entry, got %d", packet.Amount)
	}

	reader := protocol.NewReader(packet.Data)
	pos, err := reader.ReadInt32()
	if err != nil {
		t.Fatalf("read blockSnapshot pos: %v", err)
	}
	blockID, err := reader.ReadInt16()
	if err != nil {
		t.Fatalf("read blockSnapshot block id: %v", err)
	}
	data, err := reader.ReadBytes(reader.Remaining())
	if err != nil {
		t.Fatalf("read blockSnapshot data: %v", err)
	}
	if pos != protocol.PackPoint2(9, 8) || blockID != 500 || string(data) != string([]byte{1, 2, 3, 4}) {
		t.Fatalf("unexpected blockSnapshot payload pos=%d block=%d data=%v", pos, blockID, data)
	}

	_ = peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	one := make([]byte, 1)
	_, err = peer.Read(one)
	if err == nil {
		t.Fatal("unexpected tcp mirror after successful udp blockSnapshot send")
	}
	if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("expected tcp read timeout, got %v", err)
	}
}

func TestSyncIsolatedBlockSnapshotsToConnFallsBackToTCPWithoutUDPAddr(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.UdpFallbackTCP = true

	tcpConn, peer := net.Pipe()
	defer tcpConn.Close()
	defer peer.Close()

	conn := NewConn(tcpConn, srv.Serial)
	defer conn.Close()
	conn.playerID = 44

	readDone := make(chan []byte, 1)
	go func() {
		defer close(readDone)
		_ = peer.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		lenBuf := make([]byte, 2)
		if _, err := io.ReadFull(peer, lenBuf); err != nil {
			return
		}
		payloadLen := int(lenBuf[0])<<8 | int(lenBuf[1])
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(peer, payload); err != nil {
			return
		}
		readDone <- payload
	}()

	snaps := []world.BlockSyncSnapshot{
		{Pos: protocol.PackPoint2(5, 7), BlockID: 222, Data: []byte{9, 8, 7}},
	}
	if err := srv.SyncIsolatedBlockSnapshotsToConn(conn, snaps); err != nil {
		t.Fatalf("SyncIsolatedBlockSnapshotsToConn: %v", err)
	}

	payload, ok := <-readDone
	if !ok || len(payload) == 0 {
		t.Fatal("expected tcp fallback payload")
	}
	obj, err := srv.Serial.ReadObject(bytes.NewReader(payload))
	if err != nil {
		t.Fatalf("decode tcp fallback blockSnapshot: %v", err)
	}
	packet, ok := obj.(*protocol.Remote_NetClient_blockSnapshot_34)
	if !ok {
		t.Fatalf("expected blockSnapshot packet, got %T", obj)
	}
	if packet.Amount != 1 {
		t.Fatalf("expected isolated blockSnapshot amount=1, got %d", packet.Amount)
	}
}
