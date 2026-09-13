package harness

import (
	"testing"
	"time"

	"mdt-server/internal/net"
	"mdt-server/internal/protocol"
)

// harnessUnitType is a minimal UnitType so the server can resolve the entity
// we inject into entitySnapshot sync.
type harnessUnitType struct {
	id   int16
	name string
}

func (t harnessUnitType) ContentType() protocol.ContentType { return protocol.ContentUnit }
func (t harnessUnitType) ID() int16                         { return t.id }
func (t harnessUnitType) Name() string                      { return t.name }

func isStreamBegin(o any) bool     { _, ok := o.(*protocol.StreamBegin); return ok }
func isStateSnapshot(o any) bool   { _, ok := o.(*protocol.Remote_NetClient_stateSnapshot_35); return ok }
func isEntitySnapshot(o any) bool  { _, ok := o.(*protocol.Remote_NetClient_entitySnapshot_32); return ok }

// TestSimClientVanillaJoinAndSync boots a real server, connects a simulated
// build-158 client over real TCP, and verifies the exact vanilla handshake:
//  1. initial join sends the WorldStream (and nothing else sync-related);
//  2. real-time sync (state/entity snapshots) does NOT start before connectConfirm;
//  3. after connectConfirm the server streams state + entity snapshots;
//  4. client->server input (clientSnapshot) is processed without crashing.
func TestSimClientVanillaJoinAndSync(t *testing.T) {
	srv := net.NewServer("127.0.0.1:0", 159)
	srv.WorldDataFn = func(_ *net.Conn, _ *protocol.ConnectPacket) ([]byte, error) {
		return []byte("sim-world-payload-bytes"), nil
	}
	srv.Content.RegisterUnitType(harnessUnitType{id: 35, name: "alpha"})
	srv.SetSnapshotIntervals(20, 80)
	srv.UnitInfoFn = func(int32) (net.UnitInfo, bool) { return net.UnitInfo{}, false }
	srv.ExtraEntitySnapshotEntitiesFn = func() ([]protocol.UnitSyncEntity, error) {
		return []protocol.UnitSyncEntity{&protocol.UnitEntitySync{
			IDValue:      7001,
			ClassIDValue: 30,
			ClassIDSet:   true,
			TypeID:       35,
			TeamID:       2,
			Health:       100,
			X:            24,
			Y:            40,
		}}, nil
	}

	go srv.Serve()
	defer srv.Shutdown()

	addr := waitBoundAddr(t, srv)
	cli := NewSimClient(addr)
	if err := cli.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cli.Close()

	// (1) The server must send the WorldStream on initial join, and it must
	// reassemble into the exact payload we returned from WorldDataFn.
	if _, ok := cli.WaitWorldStream(3 * time.Second); !ok {
		t.Fatalf("expected WorldStream to reassemble on initial join (trace=%s)", cli.Trace())
	}
	if !cli.Trace().Seen(0) {
		t.Fatal("expected StreamBegin (wire id 0) recorded in trace")
	}

	// (2) Real-time sync must NOT begin before connectConfirm (vanilla gating).
	if cli.Trace().Count(126) != 0 {
		t.Fatalf("stateSnapshot arrived before connectConfirm (gating broken): trace=%s", cli.Trace())
	}

	// (3) Client confirms -> server unlocks post-connect sync.
	if err := cli.Confirm(); err != nil {
		t.Fatalf("confirm: %v", err)
	}
	if _, ok := cli.WaitFor(isStateSnapshot, 3*time.Second); !ok {
		t.Fatalf("expected stateSnapshot after connectConfirm (trace=%s)", cli.Trace())
	}
	if _, ok := cli.WaitFor(isEntitySnapshot, 3*time.Second); !ok {
		t.Fatalf("expected entitySnapshot after connectConfirm (trace=%s)", cli.Trace())
	}

	// (4) Client->server input must be processed without crashing the server.
	if err := cli.SendClientSnapshot(10, 20); err != nil {
		t.Fatalf("clientSnapshot send: %v", err)
	}
	time.Sleep(150 * time.Millisecond)

	// The documented vanilla wire prefix must hold: world stream, then
	// state + entity snapshots (framework/RegisterTCP and chunk ids may
	// interleave, which AssertOrder permits).
	if msg := cli.Trace().AssertOrder(0, 126, 46); msg != "" {
		t.Fatalf("wire order mismatch: %s (trace=%s)", msg, cli.Trace())
	}
}

func waitBoundAddr(t *testing.T, srv *net.Server) string {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if a := srv.TCPAddr(); a != nil {
			return a.String()
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("server did not bind an address")
	return ""
}
