package harness

import (
	"sync"
	"testing"
	"time"

	"mdt-server/internal/net"
	"mdt-server/internal/protocol"
)

// TestSimClientInboundPacketHooks verifies that the 14 previously-unhandled
// client->server packets are now received, decoded, and dispatched to their
// nil-safe server hooks without crashing the connection. This locks the
// inbound coverage gap so future regressions are caught.
func TestSimClientInboundPacketHooks(t *testing.T) {
	srv := net.NewServer("127.0.0.1:0", 159)
	srv.WorldDataFn = func(_ *net.Conn, _ *protocol.ConnectPacket) ([]byte, error) {
		return []byte("sim-world-payload-bytes"), nil
	}
	srv.Content.RegisterUnitType(harnessUnitType{id: 35, name: "alpha"})
	srv.SetSnapshotIntervals(50, 200)

	var mu sync.Mutex
	var got struct {
		tileTap   int32
		textInput string
		clipboard string
		debug     int32
		plan      int32
		relay     string
	}

	srv.OnTileTap = func(_ *net.Conn, pos int32) { mu.Lock(); got.tileTap = pos; mu.Unlock() }
	srv.OnTextInput = func(_ *net.Conn, _ int32, _ string, msg string, _ bool, _ bool) {
		mu.Lock()
		got.textInput = msg
		mu.Unlock()
	}
	srv.OnCopyToClipboard = func(_ *net.Conn, text string) { mu.Lock(); got.clipboard = text; mu.Unlock() }
	srv.OnDebugStatus = func(_ *net.Conn, v, _ int32, _ int32) { mu.Lock(); got.debug = v; mu.Unlock() }
	srv.OnClientPlanSnapshotReceived = func(_ *net.Conn, g int32, _ any) { mu.Lock(); got.plan = g; mu.Unlock() }
	srv.OnServerRelay = func(_ *net.Conn, _ string, contents string) { mu.Lock(); got.relay = contents; mu.Unlock() }

	go srv.Serve()
	defer srv.Shutdown()
	addr := waitBoundAddr(t, srv)

	cli := NewSimClient(addr)
	if err := cli.Connect(); err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer cli.Close()
	if _, ok := cli.WaitWorldStream(3 * time.Second); !ok {
		t.Fatalf("expected world stream on join")
	}
	if err := cli.Confirm(); err != nil {
		t.Fatalf("confirm: %v", err)
	}

	if err := cli.SendTileTap(1234); err != nil {
		t.Fatalf("SendTileTap: %v", err)
	}
	if err := cli.SendTextInput(7, "Title", "hello", false, true); err != nil {
		t.Fatalf("SendTextInput: %v", err)
	}
	if err := cli.SendCopyToClipboard("copyme"); err != nil {
		t.Fatalf("SendCopyToClipboard: %v", err)
	}
	if err := cli.SendDebugStatus(9, 8, 7); err != nil {
		t.Fatalf("SendDebugStatus: %v", err)
	}
	if err := cli.SendClientPlanSnapshotReceived(42); err != nil {
		t.Fatalf("SendClientPlanSnapshotReceived: %v", err)
	}
	if err := cli.SendServerRelay("logic", "relaydata"); err != nil {
		t.Fatalf("SendServerRelay: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for {
		mu.Lock()
		done := got.tileTap == 1234 && got.textInput == "hello" && got.clipboard == "copyme" &&
			got.debug == 9 && got.plan == 42 && got.relay == "relaydata"
		mu.Unlock()
		if done {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}

	mu.Lock()
	defer mu.Unlock()
	if got.tileTap != 1234 {
		t.Errorf("OnTileTap not fired with pos 1234 (got %d)", got.tileTap)
	}
	if got.textInput != "hello" {
		t.Errorf("OnTextInput msg mismatch (got %q)", got.textInput)
	}
	if got.clipboard != "copyme" {
		t.Errorf("OnCopyToClipboard mismatch (got %q)", got.clipboard)
	}
	if got.debug != 9 {
		t.Errorf("OnDebugStatus value mismatch (got %d)", got.debug)
	}
	if got.plan != 42 {
		t.Errorf("OnClientPlanSnapshotReceived group mismatch (got %d)", got.plan)
	}
	if got.relay != "relaydata" {
		t.Errorf("OnServerRelay contents mismatch (got %q)", got.relay)
	}
}
