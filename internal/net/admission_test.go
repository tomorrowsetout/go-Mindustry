package net

import (
	"io"
	"net"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func startDiscardCopy(conn net.Conn) chan struct{} {
	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, conn)
		close(done)
	}()
	return done
}

func waitSentPacket(t *testing.T, sent <-chan any) any {
	t.Helper()
	select {
	case obj := <-sent:
		return obj
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for sent packet")
		return nil
	}
}

func waitConnClosed(t *testing.T, conn *Conn) {
	t.Helper()
	select {
	case <-conn.closed:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for connection close")
	}
}

func TestConnSendWriteFailureDoesNotDeadlock(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	serverSide, clientSide := net.Pipe()
	conn := NewConn(serverSide, srv.Serial)
	defer clientSide.Close()

	_ = clientSide.Close()

	sendDone := make(chan error, 1)
	go func() {
		sendDone <- conn.Send(&protocol.Remote_NetClient_kick_22{Reason: "closed"})
	}()

	select {
	case err := <-sendDone:
		if err == nil {
			t.Fatal("expected send to fail after peer close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("send deadlocked after peer close")
	}

	closeDone := make(chan error, 1)
	go func() {
		closeDone <- conn.Close()
	}()
	select {
	case <-closeDone:
	case <-time.After(2 * time.Second):
		t.Fatal("close deadlocked after send failure")
	}
}

func TestConnectPacketRejectsInvalidVersionTypeAndCloses(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()

	sent := make(chan any, 4)
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		sent <- obj
	}
	done := startDiscardCopy(clientSide)

	srv.handlePacket(conn, &protocol.ConnectPacket{
		Version:     157,
		VersionType: "",
		Name:        "alpha",
		Locale:      "en",
		UUID:        "uuid-1",
		USID:        "usid-1",
	}, true)

	obj := waitSentPacket(t, sent)
	pkt, ok := obj.(*protocol.Remote_NetClient_kick_21)
	if !ok || pkt.Reason != protocol.KickReasonTypeMismatch {
		t.Fatalf("expected typeMismatch kick, got %T %+v", obj, obj)
	}
	waitConnClosed(t, conn)
	if conn.playerID != 0 || conn.hasBegunConnecting {
		t.Fatalf("expected rejected connect to leave player unassigned, got playerID=%d begun=%v", conn.playerID, conn.hasBegunConnecting)
	}
	if len(srv.entities) != 0 {
		t.Fatalf("expected rejected connect not to create entities, got %d", len(srv.entities))
	}

	_ = conn.Close()
	<-done
}

func TestConnectPacketRejectsDuplicateUUIDAndUSID(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.addConn(&Conn{
		rawName:            "existing",
		uuid:               "uuid-dup",
		usid:               "usid-dup",
		hasBegunConnecting: true,
	})

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()

	sent := make(chan any, 4)
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		sent <- obj
	}
	done := startDiscardCopy(clientSide)

	srv.handlePacket(conn, &protocol.ConnectPacket{
		Version:     157,
		VersionType: "official",
		Name:        "alpha",
		Locale:      "en",
		UUID:        "uuid-dup",
		USID:        "usid-dup",
	}, true)

	obj := waitSentPacket(t, sent)
	pkt, ok := obj.(*protocol.Remote_NetClient_kick_21)
	if !ok || pkt.Reason != protocol.KickReasonIDInUse {
		t.Fatalf("expected idInUse kick, got %T %+v", obj, obj)
	}
	waitConnClosed(t, conn)
	if conn.playerID != 0 || len(srv.entities) != 0 {
		t.Fatalf("expected duplicate rejection to avoid entity allocation, playerID=%d entities=%d", conn.playerID, len(srv.entities))
	}

	_ = conn.Close()
	<-done
}

func TestConnectPacketRejectsRecentKickAndWhitelist(t *testing.T) {
	t.Run("recent kick", func(t *testing.T) {
		srv := NewServer("127.0.0.1:0", 157)
		srv.noteRecentKick("uuid-1", "pipe")

		serverSide, clientSide := net.Pipe()
		defer clientSide.Close()
		conn := NewConn(serverSide, srv.Serial)
		defer conn.Close()

		sent := make(chan any, 4)
		conn.onSend = func(obj any, _ int, _ int, _ int) { sent <- obj }
		done := startDiscardCopy(clientSide)

		srv.handlePacket(conn, &protocol.ConnectPacket{
			Version:     157,
			VersionType: "official",
			Name:        "alpha",
			Locale:      "en",
			UUID:        "uuid-1",
			USID:        "usid-1",
		}, true)

		obj := waitSentPacket(t, sent)
		pkt, ok := obj.(*protocol.Remote_NetClient_kick_21)
		if !ok || pkt.Reason != protocol.KickReasonRecentKick {
			t.Fatalf("expected recentKick kick, got %T %+v", obj, obj)
		}
		waitConnClosed(t, conn)
		_ = conn.Close()
		<-done
	})

	t.Run("whitelist", func(t *testing.T) {
		srv := NewServer("127.0.0.1:0", 157)
		srv.SetAdmissionPolicy(AdmissionPolicy{
			StrictIdentity:     true,
			AllowCustomClients: false,
			PlayerLimit:        0,
			WhitelistEnabled:   true,
			Whitelist:          []AdmissionWhitelistEntry{{UUID: "uuid-allow", USID: "usid-allow"}},
			ExpectedMods:       nil,
			RecentKickDuration: 30 * time.Second,
		})

		serverSide, clientSide := net.Pipe()
		defer clientSide.Close()
		conn := NewConn(serverSide, srv.Serial)
		defer conn.Close()

		sent := make(chan any, 4)
		conn.onSend = func(obj any, _ int, _ int, _ int) { sent <- obj }
		done := startDiscardCopy(clientSide)

		srv.handlePacket(conn, &protocol.ConnectPacket{
			Version:     157,
			VersionType: "official",
			Name:        "alpha",
			Locale:      "en",
			UUID:        "uuid-deny",
			USID:        "usid-deny",
		}, true)

		obj := waitSentPacket(t, sent)
		pkt, ok := obj.(*protocol.Remote_NetClient_kick_21)
		if !ok || pkt.Reason != protocol.KickReasonWhitelist {
			t.Fatalf("expected whitelist kick, got %T %+v", obj, obj)
		}
		waitConnClosed(t, conn)
		_ = conn.Close()
		<-done
	})
}

func TestConnectPacketRejectsPlayerLimitAndUnexpectedMods(t *testing.T) {
	t.Run("player limit", func(t *testing.T) {
		srv := NewServer("127.0.0.1:0", 157)
		srv.SetAdmissionPolicy(AdmissionPolicy{
			StrictIdentity:     true,
			AllowCustomClients: false,
			PlayerLimit:        1,
			WhitelistEnabled:   false,
			ExpectedMods:       nil,
			RecentKickDuration: 30 * time.Second,
		})
		srv.addConn(&Conn{rawName: "existing", uuid: "uuid-existing", usid: "usid-existing", hasBegunConnecting: true, hasConnected: true})

		serverSide, clientSide := net.Pipe()
		defer clientSide.Close()
		conn := NewConn(serverSide, srv.Serial)
		defer conn.Close()

		sent := make(chan any, 4)
		conn.onSend = func(obj any, _ int, _ int, _ int) { sent <- obj }
		done := startDiscardCopy(clientSide)

		srv.handlePacket(conn, &protocol.ConnectPacket{
			Version:     157,
			VersionType: "official",
			Name:        "alpha",
			Locale:      "en",
			UUID:        "uuid-limit",
			USID:        "usid-limit",
		}, true)

		obj := waitSentPacket(t, sent)
		pkt, ok := obj.(*protocol.Remote_NetClient_kick_21)
		if !ok || pkt.Reason != protocol.KickReasonPlayerLimit {
			t.Fatalf("expected playerLimit kick, got %T %+v", obj, obj)
		}
		waitConnClosed(t, conn)
		_ = conn.Close()
		<-done
	})

	t.Run("unexpected mods", func(t *testing.T) {
		srv := NewServer("127.0.0.1:0", 157)

		serverSide, clientSide := net.Pipe()
		defer clientSide.Close()
		conn := NewConn(serverSide, srv.Serial)
		defer conn.Close()

		sent := make(chan any, 4)
		conn.onSend = func(obj any, _ int, _ int, _ int) { sent <- obj }
		done := startDiscardCopy(clientSide)

		srv.handlePacket(conn, &protocol.ConnectPacket{
			Version:     157,
			VersionType: "official",
			Name:        "alpha",
			Locale:      "en",
			UUID:        "uuid-mods",
			USID:        "usid-mods",
			Mods:        []string{"example-mod"},
		}, true)

		obj := waitSentPacket(t, sent)
		pkt, ok := obj.(*protocol.Remote_NetClient_kick_22)
		if !ok || pkt.Reason == "" {
			t.Fatalf("expected string mod incompatibility kick, got %T %+v", obj, obj)
		}
		waitConnClosed(t, conn)
		_ = conn.Close()
		<-done
	})
}

func TestConnectPacketRejectsBannedNameAndSubnetAndStoresLocaleMobile(t *testing.T) {
	t.Run("banned name", func(t *testing.T) {
		srv := NewServer("127.0.0.1:0", 157)
		srv.SetAdmissionPolicy(AdmissionPolicy{
			StrictIdentity:     true,
			AllowCustomClients: false,
			BannedNames:        []string{"alpha"},
			RecentKickDuration: 30 * time.Second,
		})

		serverSide, clientSide := net.Pipe()
		defer clientSide.Close()
		conn := NewConn(serverSide, srv.Serial)
		defer conn.Close()
		sent := make(chan any, 4)
		conn.onSend = func(obj any, _ int, _ int, _ int) { sent <- obj }
		done := startDiscardCopy(clientSide)

		srv.handlePacket(conn, &protocol.ConnectPacket{
			Version:     157,
			VersionType: "official",
			Name:        "[accent]alpha[]",
			Locale:      "zh_CN",
			UUID:        "uuid-ban-name",
			USID:        "usid-ban-name",
		}, true)

		obj := waitSentPacket(t, sent)
		if _, ok := obj.(*protocol.Remote_NetClient_kick_22); !ok {
			t.Fatalf("expected string banned kick, got %T", obj)
		}
		waitConnClosed(t, conn)
		_ = conn.Close()
		<-done
	})

	t.Run("banned subnet", func(t *testing.T) {
		srv := NewServer("127.0.0.1:0", 157)
		srv.SetAdmissionPolicy(AdmissionPolicy{
			StrictIdentity:     true,
			AllowCustomClients: false,
			BannedSubnets:      []string{"10.0.0.0/8"},
			RecentKickDuration: 30 * time.Second,
		})
		serverSide, clientSide := net.Pipe()
		defer clientSide.Close()
		conn := NewConn(serverSide, srv.Serial)
		conn.id = 1
		conn.Conn = &admissionWrappedConn{Conn: serverSide, remote: admissionAddr("10.1.2.3:1234")}
		defer conn.Close()
		sent := make(chan any, 4)
		conn.onSend = func(obj any, _ int, _ int, _ int) { sent <- obj }
		done := startDiscardCopy(clientSide)

		srv.handlePacket(conn, &protocol.ConnectPacket{
			Version:     157,
			VersionType: "official",
			Name:        "beta",
			Locale:      "en_US",
			UUID:        "uuid-ban-subnet",
			USID:        "usid-ban-subnet",
		}, true)

		obj := waitSentPacket(t, sent)
		if _, ok := obj.(*protocol.Remote_NetClient_kick_22); !ok {
			t.Fatalf("expected string banned kick, got %T", obj)
		}
		waitConnClosed(t, conn)
		_ = conn.Close()
		<-done
	})

	t.Run("rejects vanilla empty fixed name", func(t *testing.T) {
		srv := NewServer("127.0.0.1:0", 157)
		serverSide, clientSide := net.Pipe()
		defer clientSide.Close()
		conn := NewConn(serverSide, srv.Serial)
		defer conn.Close()
		sent := make(chan any, 4)
		conn.onSend = func(obj any, _ int, _ int, _ int) { sent <- obj }
		done := startDiscardCopy(clientSide)

		srv.handlePacket(conn, &protocol.ConnectPacket{
			Version:     157,
			VersionType: "official",
			Name:        "[",
			Locale:      "en_US",
			UUID:        "uuid-empty-fixed",
			USID:        "usid-empty-fixed",
		}, true)

		obj := waitSentPacket(t, sent)
		pkt, ok := obj.(*protocol.Remote_NetClient_kick_21)
		if !ok || pkt.Reason != protocol.KickReasonNameEmpty {
			t.Fatalf("expected nameEmpty kick, got %T %+v", obj, obj)
		}
		waitConnClosed(t, conn)
		_ = conn.Close()
		<-done
	})

	t.Run("stores locale and mobile", func(t *testing.T) {
		srv := NewServer("127.0.0.1:0", 157)
		serverSide, clientSide := net.Pipe()
		defer clientSide.Close()
		conn := NewConn(serverSide, srv.Serial)
		defer conn.Close()
		done := startDiscardCopy(clientSide)
		srv.WorldDataFn = func(_ *Conn, _ *protocol.ConnectPacket) ([]byte, error) {
			return []byte("ok"), nil
		}

		srv.handlePacket(conn, &protocol.ConnectPacket{
			Version:     157,
			VersionType: "official",
			Name:        "gamma",
			Locale:      "",
			UUID:        "uuid-ok",
			USID:        "usid-ok",
			Mobile:      true,
		}, true)

		if conn.locale != "en" {
			t.Fatalf("expected default locale en, got %q", conn.locale)
		}
		if !conn.mobile {
			t.Fatal("expected mobile flag to be stored")
		}
		if conn.rawName != "gamma" {
			t.Fatalf("expected fixed name to be stored, got %q", conn.rawName)
		}
		_ = conn.Close()
		<-done
	})
}

func TestFixMindustryPlayerNameMatchesVanillaCases(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "trim and strip control whitespace", in: "  alpha\t\n", want: "alpha"},
		{name: "single bracket becomes empty", in: "[", want: ""},
		{name: "transparent hex tag removed", in: "[#11223300]alpha", want: "alpha"},
		{name: "clear tag removed", in: "[clear]alpha", want: "alpha"},
		{name: "opaque color preserved", in: "[accent]alpha", want: "[accent]alpha"},
		{name: "name limited by bytes", in: "12345678901234567890123456789012345678901", want: "1234567890123456789012345678901234567890"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := FixMindustryPlayerName(tc.in); got != tc.want {
				t.Fatalf("FixMindustryPlayerName(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

type admissionAddr string

func (a admissionAddr) Network() string { return "tcp" }
func (a admissionAddr) String() string  { return string(a) }

type admissionWrappedConn struct {
	net.Conn
	remote net.Addr
}

func (c *admissionWrappedConn) RemoteAddr() net.Addr {
	if c.remote != nil {
		return c.remote
	}
	return c.Conn.RemoteAddr()
}
