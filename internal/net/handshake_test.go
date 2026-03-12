package net

import (
	"errors"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func baseConnectPacket() *protocol.ConnectPacket {
	return &protocol.ConnectPacket{
		Version:     155,
		VersionType: "official",
		Name:        "player",
		Locale:      "en",
		UUID:        "AAAAAAAAAAAAAAAAAAAAAA==",
		USID:        "usid-1",
	}
}

func TestValidateConnect_VersionAndTypeRules(t *testing.T) {
	tests := []struct {
		name string
		pkt  *protocol.ConnectPacket
		want error
	}{
		{name: "ok", pkt: baseConnectPacket(), want: nil},
		{name: "empty-usid", pkt: func() *protocol.ConnectPacket {
			p := baseConnectPacket()
			p.USID = ""
			return p
		}(), want: ErrIDInUse},
		{name: "empty-name", pkt: func() *protocol.ConnectPacket {
			p := baseConnectPacket()
			p.Name = "   "
			return p
		}(), want: ErrNameEmpty},
		{name: "empty-version-type", pkt: func() *protocol.ConnectPacket {
			p := baseConnectPacket()
			p.VersionType = ""
			return p
		}(), want: ErrTypeMismatch},
		{name: "mod-client", pkt: func() *protocol.ConnectPacket {
			p := baseConnectPacket()
			p.Version = -1
			return p
		}(), want: ErrCustomClient},
		{name: "client-outdated", pkt: func() *protocol.ConnectPacket {
			p := baseConnectPacket()
			p.Version = 154
			return p
		}(), want: ErrClientOutdated},
		{name: "server-outdated", pkt: func() *protocol.ConnectPacket {
			p := baseConnectPacket()
			p.Version = 156
			return p
		}(), want: ErrServerOutdated},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateConnect(tc.pkt, 155, true)
			if tc.want == nil {
				if err != nil {
					t.Fatalf("expected nil err, got %v", err)
				}
				return
			}
			if !errors.Is(err, tc.want) {
				t.Fatalf("expected error %v, got %v", tc.want, err)
			}
		})
	}
}

func TestValidateConnect_NonStrictAllowsCustom(t *testing.T) {
	p := baseConnectPacket()
	p.VersionType = ""
	if err := ValidateConnect(p, 155, false); err != nil {
		t.Fatalf("expected empty versionType accepted in non-strict, got %v", err)
	}

	p = baseConnectPacket()
	p.Version = -1
	p.VersionType = "custom"
	if err := ValidateConnect(p, 155, false); err != nil {
		t.Fatalf("expected modded build accepted in non-strict, got %v", err)
	}

	p = baseConnectPacket()
	p.Version = 154
	p.VersionType = "custom"
	if err := ValidateConnect(p, 155, false); !errors.Is(err, ErrClientOutdated) {
		t.Fatalf("expected outdated error in non-strict, got %v", err)
	}
}

func TestValidateConnect_BuildMinusOneRelaxed(t *testing.T) {
	p := baseConnectPacket()
	p.Version = -1
	p.VersionType = "custom"
	if err := ValidateConnect(p, -1, true); err != nil {
		t.Fatalf("expected relaxed validation on build=-1, got %v", err)
	}
}

func TestDuplicateIdentity(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	existing := &Conn{name: "Alice", uuid: "U1", usid: "S1", hasBegunConnecting: true}
	curr := &Conn{}
	s.conns[existing] = struct{}{}
	s.conns[curr] = struct{}{}

	if reason, ok := s.hasDuplicateIdentity(curr, "alice", "U2", "S2"); !ok || reason != protocol.KickReasonNameInUse {
		t.Fatalf("expected duplicate name, got ok=%v reason=%v", ok, reason)
	}
	if reason, ok := s.hasDuplicateIdentity(curr, "Bob", "U1", "S2"); !ok || reason != protocol.KickReasonIDInUse {
		t.Fatalf("expected duplicate uuid, got ok=%v reason=%v", ok, reason)
	}
	if reason, ok := s.hasDuplicateIdentity(curr, "Bob", "U2", "S1"); !ok || reason != protocol.KickReasonIDInUse {
		t.Fatalf("expected duplicate usid, got ok=%v reason=%v", ok, reason)
	}
	if _, ok := s.hasDuplicateIdentity(curr, "Bob", "U2", "S2"); ok {
		t.Fatalf("did not expect duplicate identity")
	}
}

func TestRecentKickWindow(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	s.registerRecentKick("U1", "1.2.3.4", 50*time.Millisecond)
	if !s.isRecentlyKicked("U1", "1.2.3.4") {
		t.Fatalf("expected recent kick hit")
	}
	time.Sleep(70 * time.Millisecond)
	if s.isRecentlyKicked("U1", "1.2.3.4") {
		t.Fatalf("expected recent kick expired")
	}
}

func TestWhitelistAndPlayerLimitSetters(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	s.SetPlayerLimit(3)
	if got := s.PlayerLimit(); got != 3 {
		t.Fatalf("unexpected player limit: %d", got)
	}

	s.SetWhitelistEnabled(true)
	s.AddWhitelistUUID("U1")
	s.AddWhitelistUSID("S1")
	if !s.isWhitelisted("U1", "x") {
		t.Fatalf("expected uuid whitelist")
	}
	if !s.isWhitelisted("x", "S1") {
		t.Fatalf("expected usid whitelist")
	}
	if s.isWhitelisted("x", "y") {
		t.Fatalf("did not expect unrelated whitelist pass")
	}
}
