package net

import (
	"io"
	"net"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func TestNotifyShutdownSendsKickAndClosesConnection(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	srv.conns[conn] = struct{}{}

	kickReason := ""
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		pkt, ok := obj.(*protocol.Remote_NetClient_kick_22)
		if !ok {
			return
		}
		kickReason = pkt.Reason
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	if notified := srv.NotifyShutdown("server closing", 20*time.Millisecond); notified != 1 {
		t.Fatalf("expected one notified connection, got %d", notified)
	}
	if kickReason != "server closing" {
		t.Fatalf("expected kick reason to be sent, got %q", kickReason)
	}

	select {
	case <-conn.closed:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected connection to close after shutdown notification")
	}

	<-done
}

func TestServerShutdownClosesListenersAndConnections(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()

	srv.mu.Lock()
	srv.conns[conn] = struct{}{}
	srv.pending[conn.id] = conn
	srv.mu.Unlock()

	tcpLn, err := net.ListenTCP("tcp", &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen tcp: %v", err)
	}
	defer tcpLn.Close()

	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer udpConn.Close()

	srv.tcpLn = tcpLn
	srv.udpConn = udpConn
	srv.Shutdown()

	if !srv.shuttingDown.Load() {
		t.Fatal("expected shuttingDown flag")
	}
	select {
	case <-conn.closed:
	case <-time.After(300 * time.Millisecond):
		t.Fatal("expected connection to close during shutdown")
	}
	if _, err := tcpLn.AcceptTCP(); err == nil {
		t.Fatal("expected tcp listener to be closed by shutdown")
	}
}
