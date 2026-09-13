package net

import (
	"encoding/binary"
	"io"
	"net"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func TestSendUnreliableUDPSuccessDoesNotMirrorTCP(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	udpCapture := installUDPCapture(srv)
	udpAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50101}

	tcpConn, peer := net.Pipe()
	defer tcpConn.Close()
	defer peer.Close()

	conn := NewConn(tcpConn, srv.Serial)
	defer conn.Close()
	conn.setUDPAddr(udpAddr)

	obj := &protocol.Remote_NetClient_stateSnapshot_35{CoreData: []byte{0}}
	if err := srv.sendUnreliable(conn, obj); err != nil {
		t.Fatalf("sendUnreliable: %v", err)
	}

	udpPacket := udpCapture.read(t)
	if len(udpPacket.payload) == 0 {
		t.Fatal("expected udp datagram, got empty payload")
	}
	if got := udpPacket.addr.String(); got != udpAddr.String() {
		t.Fatalf("udp addr mismatch got=%s want=%s", got, udpAddr.String())
	}

	_ = peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	one := make([]byte, 1)
	_, err := peer.Read(one)
	if err == nil {
		t.Fatal("unexpected tcp mirror after successful udp send")
	}
	if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("expected tcp read timeout, got %v", err)
	}
}

func TestSendUnreliableFallsBackToTCPWhenNoUDPAddr(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.UdpFallbackTCP = true

	tcpConn, peer := net.Pipe()
	defer tcpConn.Close()
	defer peer.Close()

	conn := NewConn(tcpConn, srv.Serial)
	defer conn.Close()

	readDone := make(chan error, 1)
	go func() {
		defer close(readDone)
		_ = peer.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		lenBuf := make([]byte, 2)
		if _, err := io.ReadFull(peer, lenBuf); err != nil {
			readDone <- err
			return
		}
		payloadLen := int(lenBuf[0])<<8 | int(lenBuf[1])
		if payloadLen <= 0 {
			readDone <- io.ErrUnexpectedEOF
			return
		}
		payload := make([]byte, payloadLen)
		_, err := io.ReadFull(peer, payload)
		readDone <- err
	}()

	obj := &protocol.Remote_NetClient_stateSnapshot_35{CoreData: []byte{0}}
	if err := srv.sendUnreliable(conn, obj); err != nil {
		t.Fatalf("sendUnreliable: %v", err)
	}

	if err := <-readDone; err != nil {
		t.Fatalf("tcp fallback read failed: %v", err)
	}
	if got := conn.sendCount.Load(); got != 1 {
		t.Fatalf("expected one tcp send, got %d", got)
	}
}

func TestHandleUDPDatagramRegistersMultipleConnections(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	udpCapture := installUDPCapture(srv)
	udpAddrA := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50111}
	udpAddrB := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50112}

	tcpA, peerA := net.Pipe()
	defer tcpA.Close()
	defer peerA.Close()
	connA := NewConn(&admissionWrappedConn{Conn: tcpA, remote: admissionAddr("127.0.0.1:10001")}, srv.Serial)
	defer connA.Close()
	connA.id = 101
	srv.addConn(connA)
	srv.addPending(connA)

	tcpB, peerB := net.Pipe()
	defer tcpB.Close()
	defer peerB.Close()
	connB := NewConn(&admissionWrappedConn{Conn: tcpB, remote: admissionAddr("127.0.0.1:10002")}, srv.Serial)
	defer connB.Close()
	connB.id = 202
	srv.addConn(connB)
	srv.addPending(connB)

	rawRegister := func(id int32) []byte {
		buf := make([]byte, 6)
		buf[0] = 0xFE
		buf[1] = protocol.FrameworkRegisterUD
		binary.BigEndian.PutUint32(buf[2:], uint32(id))
		return buf
	}

	srv.handleUDPDatagram(nil, udpAddrA, rawRegister(connA.id))
	srv.handleUDPDatagram(nil, udpAddrB, rawRegister(connB.id))

	readAck := func(wantAddr *net.UDPAddr, wantID int32) {
		t.Helper()
		packet := udpCapture.read(t)
		buf := packet.payload
		if got := packet.addr.String(); got != wantAddr.String() {
			t.Fatalf("udp ack addr id=%d got=%s want=%s", wantID, got, wantAddr.String())
		}
		if len(buf) < 6 || buf[0] != 0xFE || buf[1] != protocol.FrameworkRegisterUD {
			t.Fatalf("unexpected udp ack id=%d payload=%v", wantID, buf)
		}
		if got := int32(binary.BigEndian.Uint32(buf[2:6])); got != wantID {
			t.Fatalf("expected udp ack id=%d, got=%d", wantID, got)
		}
	}

	readAck(udpAddrA, connA.id)
	readAck(udpAddrB, connB.id)

	readTCPAck := func(peer net.Conn, wantID int32) {
		t.Helper()
		_ = peer.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		lenBuf := make([]byte, 2)
		if _, err := io.ReadFull(peer, lenBuf); err != nil {
			t.Fatalf("read tcp udp ack length id=%d: %v", wantID, err)
		}
		payloadLen := int(lenBuf[0])<<8 | int(lenBuf[1])
		payload := make([]byte, payloadLen)
		if _, err := io.ReadFull(peer, payload); err != nil {
			t.Fatalf("read tcp udp ack payload id=%d: %v", wantID, err)
		}
		if len(payload) < 6 || payload[0] != 0xFE || payload[1] != protocol.FrameworkRegisterUD {
			t.Fatalf("unexpected tcp udp ack id=%d payload=%v", wantID, payload)
		}
		if got := int32(binary.BigEndian.Uint32(payload[2:6])); got != wantID {
			t.Fatalf("expected tcp udp ack id=%d, got=%d", wantID, got)
		}
	}
	readTCPAck(peerA, connA.id)
	readTCPAck(peerB, connB.id)

	if got := connA.UDPAddr(); got == nil || got.String() != udpAddrA.String() {
		t.Fatalf("connA udp addr mismatch, got=%v want=%s", got, udpAddrA.String())
	}
	if got := connB.UDPAddr(); got == nil || got.String() != udpAddrB.String() {
		t.Fatalf("connB udp addr mismatch, got=%v want=%s", got, udpAddrB.String())
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if _, ok := srv.pending[connA.id]; ok {
		t.Fatalf("connA should be removed from pending after udp register")
	}
	if _, ok := srv.pending[connB.id]; ok {
		t.Fatalf("connB should be removed from pending after udp register")
	}
	if got := len(srv.byUDP); got != 2 {
		t.Fatalf("expected 2 udp registrations, got=%d", got)
	}
}

func TestHandleUDPDatagramRejectsRegisterWhenTCPIPDoesNotMatch(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	udpCapture := installUDPCapture(srv)
	udpAddr := &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 50113}

	tcpConn, peer := net.Pipe()
	defer tcpConn.Close()
	defer peer.Close()

	conn := NewConn(&admissionWrappedConn{Conn: tcpConn, remote: admissionAddr("203.0.113.10:12000")}, srv.Serial)
	defer conn.Close()
	conn.id = 303
	srv.addConn(conn)
	srv.addPending(conn)

	buf := make([]byte, 6)
	buf[0] = 0xFE
	buf[1] = protocol.FrameworkRegisterUD
	binary.BigEndian.PutUint32(buf[2:], uint32(conn.id))

	srv.handleUDPDatagram(nil, udpAddr, buf)
	udpCapture.assertEmpty(t)

	if got := conn.UDPAddr(); got != nil {
		t.Fatalf("expected udp addr to remain unset after ip mismatch, got=%v", got)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if _, ok := srv.pending[conn.id]; !ok {
		t.Fatalf("expected pending registration to remain after ip mismatch")
	}
}
