package net

import (
	"sync/atomic"
	"testing"
	"time"

	"mdt-server/internal/core"
	"mdt-server/internal/protocol"
)

func TestNetworkCoreBroadcastToTeamFiltersRecipients(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	ioCore := core.NewCore2(core.Config{Name: "network-test", MessageBuf: 16, WorkerCount: 1})
	nc := NewNetworkCoreWithCore(srv, ioCore)
	var sendsA atomic.Int32
	var sendsB atomic.Int32
	ioCore.SetPacketHandlers(nc.HandlePacketIncoming, func(m *core.PacketMessage) {
		if m == nil {
			return
		}
		switch m.ConnID {
		case 1:
			sendsA.Add(1)
		case 2:
			sendsB.Add(1)
		}
	})
	nc.Start()
	defer nc.Stop()

	connA := &Conn{id: 1, playerID: 1, teamID: 1, hasConnected: true}
	connB := &Conn{id: 2, playerID: 2, teamID: 2, hasConnected: true}

	nc.ConnectionOpen(connA)
	nc.ConnectionOpen(connB)

	nc.BroadcastToTeam(&protocol.Remote_NetClient_worldDataBegin_28{}, 1)
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if sendsA.Load() == 1 && sendsB.Load() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if sendsA.Load() != 1 {
		t.Fatalf("expected team 1 recipient to receive one packet, got %d", sendsA.Load())
	}
	if sendsB.Load() != 0 {
		t.Fatalf("expected non-team recipient to receive zero packets, got %d", sendsB.Load())
	}
}
