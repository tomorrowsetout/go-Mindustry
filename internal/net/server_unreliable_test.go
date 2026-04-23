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

	udpServer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp server: %v", err)
	}
	defer udpServer.Close()
	srv.udpConn = udpServer

	udpClient, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp client: %v", err)
	}
	defer udpClient.Close()

	tcpConn, peer := net.Pipe()
	defer tcpConn.Close()
	defer peer.Close()

	conn := NewConn(tcpConn, srv.Serial)
	defer conn.Close()
	conn.setUDPAddr(udpClient.LocalAddr().(*net.UDPAddr))

	obj := &protocol.Remote_NetClient_stateSnapshot_35{CoreData: []byte{0}}
	if err := srv.sendUnreliable(conn, obj); err != nil {
		t.Fatalf("sendUnreliable: %v", err)
	}

	buf := make([]byte, 4096)
	_ = udpClient.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
	n, _, err := udpClient.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read udp datagram: %v", err)
	}
	if n == 0 {
		t.Fatal("expected udp datagram, got empty payload")
	}

	_ = peer.SetReadDeadline(time.Now().Add(100 * time.Millisecond))
	one := make([]byte, 1)
	_, err = peer.Read(one)
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

	udpServer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp server: %v", err)
	}
	defer udpServer.Close()
	srv.udpConn = udpServer

	udpClientA, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp client A: %v", err)
	}
	defer udpClientA.Close()

	udpClientB, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp client B: %v", err)
	}
	defer udpClientB.Close()

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

	srv.handleUDPDatagram(udpServer, udpClientA.LocalAddr().(*net.UDPAddr), rawRegister(connA.id))
	srv.handleUDPDatagram(udpServer, udpClientB.LocalAddr().(*net.UDPAddr), rawRegister(connB.id))

	readAck := func(client *net.UDPConn, wantID int32) {
		t.Helper()
		buf := make([]byte, 32)
		_ = client.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
		n, _, err := client.ReadFromUDP(buf)
		if err != nil {
			t.Fatalf("read udp ack id=%d: %v", wantID, err)
		}
		if n < 6 || buf[0] != 0xFE || buf[1] != protocol.FrameworkRegisterUD {
			t.Fatalf("unexpected udp ack id=%d payload=%v", wantID, buf[:n])
		}
		if got := int32(binary.BigEndian.Uint32(buf[2:6])); got != wantID {
			t.Fatalf("expected udp ack id=%d, got=%d", wantID, got)
		}
	}

	readAck(udpClientA, connA.id)
	readAck(udpClientB, connB.id)

	if got := connA.UDPAddr(); got == nil || got.String() != udpClientA.LocalAddr().String() {
		t.Fatalf("connA udp addr mismatch, got=%v want=%s", got, udpClientA.LocalAddr().String())
	}
	if got := connB.UDPAddr(); got == nil || got.String() != udpClientB.LocalAddr().String() {
		t.Fatalf("connB udp addr mismatch, got=%v want=%s", got, udpClientB.LocalAddr().String())
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

	udpServer, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp server: %v", err)
	}
	defer udpServer.Close()
	srv.udpConn = udpServer

	udpClient, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp client: %v", err)
	}
	defer udpClient.Close()

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

	srv.handleUDPDatagram(udpServer, udpClient.LocalAddr().(*net.UDPAddr), buf)

	ack := make([]byte, 32)
	_ = udpClient.SetReadDeadline(time.Now().Add(150 * time.Millisecond))
	if _, _, err := udpClient.ReadFromUDP(ack); err == nil {
		t.Fatal("expected mismatched udp register to be rejected without ack")
	} else if ne, ok := err.(net.Error); !ok || !ne.Timeout() {
		t.Fatalf("expected udp ack timeout, got %v", err)
	}

	if got := conn.UDPAddr(); got != nil {
		t.Fatalf("expected udp addr to remain unset after ip mismatch, got=%v", got)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if _, ok := srv.pending[conn.id]; !ok {
		t.Fatalf("expected pending registration to remain after ip mismatch")
	}
}
