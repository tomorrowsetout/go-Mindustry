package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"mdt-server/internal/config"
	"mdt-server/internal/persist"
	"mdt-server/internal/protocol"
	"mdt-server/internal/storage"
)

type testRecorder struct {
	events []storage.Event
}

func (r *testRecorder) Record(e storage.Event) error {
	r.events = append(r.events, e)
	return nil
}

func (r *testRecorder) Close() error { return nil }
func (r *testRecorder) Status() string {
	return "ok"
}

type noopMessage struct{}

func (m *noopMessage) Type() MessageType { return MessageStorageRecord }

func TestCore2ConnectionLifecycleRecordsEvents(t *testing.T) {
	c2 := NewCore2(Config{Name: "test", MessageBuf: 1, WorkerCount: 1})
	rec := &testRecorder{}
	c2.SetRecorder(rec)

	open := &ConnectionMessage{
		ConnID:  11,
		UUID:    "u-1",
		IP:      "127.0.0.1:6567",
		Name:    "tester",
		TCPAddr: "127.0.0.1:9000",
		UDPAddr: "127.0.0.1:9001",
		IsOpen:  true,
	}
	c2.handleConnectionOpen(open)
	if got := c2.ConnectionCount(); got != 1 {
		t.Fatalf("expected 1 active connection, got %d", got)
	}

	closeMsg := *open
	closeMsg.IsOpen = false
	c2.handleConnectionClose(&closeMsg)
	if got := c2.ConnectionCount(); got != 0 {
		t.Fatalf("expected 0 active connections, got %d", got)
	}

	if len(rec.events) != 2 {
		t.Fatalf("expected 2 recorder events, got %d", len(rec.events))
	}
	if rec.events[0].Kind != "connection_open" {
		t.Fatalf("unexpected first event kind: %s", rec.events[0].Kind)
	}
	if rec.events[1].Kind != "connection_close" {
		t.Fatalf("unexpected second event kind: %s", rec.events[1].Kind)
	}
}

func TestCore2SendDropRollsBackQueueSize(t *testing.T) {
	c2 := NewCore2(Config{Name: "test", MessageBuf: 1, WorkerCount: 1})

	ok1 := c2.Send(&noopMessage{})
	ok2 := c2.Send(&noopMessage{})
	if !ok1 {
		t.Fatalf("expected first send to succeed")
	}
	if ok2 {
		t.Fatalf("expected second send to be dropped on full queue")
	}

	_, _, dropped, queue, _ := c2.Stats()
	if dropped != 1 {
		t.Fatalf("expected dropped=1, got %d", dropped)
	}
	if queue != 1 {
		t.Fatalf("expected queue size to remain 1, got %d", queue)
	}
}

func TestCore2PacketHandlersDoNotRecordEventsByDefault(t *testing.T) {
	c2 := NewCore2(Config{Name: "test", MessageBuf: 4, WorkerCount: 1})
	rec := &testRecorder{}
	c2.SetRecorder(rec)

	c2.handlePacketIncoming(&PacketMessage{
		ConnID:   7,
		Kind:     "incoming",
		Packet:   &noopPacket{},
		Data:     []byte{1, 2, 3},
		IdleTime: 50 * time.Millisecond,
	})
	c2.handlePacketOutgoing(&PacketMessage{
		ConnID: 7,
		Kind:   "outgoing",
		Packet: &noopPacket{},
		Data:   []byte{4},
	})

	if len(rec.events) != 0 {
		t.Fatalf("expected no packet events by default, got %d", len(rec.events))
	}
}

func TestCore2PacketHandlersRecordEventsWhenVerbose(t *testing.T) {
	c2 := NewCore2(Config{Name: "test", MessageBuf: 4, WorkerCount: 1, VerboseNetLog: true})
	rec := &testRecorder{}
	c2.SetRecorder(rec)

	c2.handlePacketIncoming(&PacketMessage{
		ConnID:   7,
		Kind:     "incoming",
		Packet:   &noopPacket{},
		Data:     []byte{1, 2, 3},
		IdleTime: 50 * time.Millisecond,
	})
	c2.handlePacketOutgoing(&PacketMessage{
		ConnID: 7,
		Kind:   "outgoing",
		Packet: &noopPacket{},
		Data:   []byte{4},
	})

	if len(rec.events) != 2 {
		t.Fatalf("expected 2 packet events when verbose, got %d", len(rec.events))
	}
	if rec.events[0].Kind != "packet_incoming" || rec.events[1].Kind != "packet_outgoing" {
		t.Fatalf("unexpected packet event sequence: %+v", rec.events)
	}
}

func TestCore2SaveLoadWorld(t *testing.T) {
	c2 := NewCore2(Config{Name: "test", MessageBuf: 4, WorkerCount: 1})
	dir := t.TempDir()
	path := filepath.Join(dir, "world.bin")
	want := []byte{9, 8, 7, 6}

	saveCh := make(chan PersistenceResult, 1)
	c2.handleSaveWorld(&PersistenceMessage{
		Action:     "save_world",
		Path:       path,
		WorldData:  want,
		ResultChan: saveCh,
	})
	saveRes := <-saveCh
	if saveRes.Error != nil {
		t.Fatalf("save_world failed: %v", saveRes.Error)
	}
	if got, err := os.ReadFile(path); err != nil {
		t.Fatalf("read saved world failed: %v", err)
	} else if string(got) != string(want) {
		t.Fatalf("saved data mismatch: got=%v want=%v", got, want)
	}

	loadCh := make(chan PersistenceResult, 1)
	c2.handleLoadWorld(&PersistenceMessage{
		Action:     "load_world",
		Path:       path,
		ResultChan: loadCh,
	})
	loadRes := <-loadCh
	if loadRes.Error != nil {
		t.Fatalf("load_world failed: %v", loadRes.Error)
	}
	if string(loadRes.WorldData) != string(want) {
		t.Fatalf("loaded data mismatch: got=%v want=%v", loadRes.WorldData, want)
	}
}

func TestCore2SaveStateUsesProvider(t *testing.T) {
	dir := t.TempDir()
	sc := NewServerCore(time.Second, Config{Name: "test", MessageBuf: 4, WorkerCount: 1}, config.PersistConfig{
		Enabled:   true,
		Directory: dir,
		File:      "server-state.json",
	})
	sc.SetPersistStateProvider(func() persist.State {
		return persist.State{
			MapPath:  "assets/worlds/test.msav",
			WaveTime: 123.5,
			Wave:     9,
			Tick:     4567,
			TimeData: 77,
			Rand0:    101,
			Rand1:    202,
		}
	})

	ch := make(chan PersistenceResult, 1)
	sc.Core2.handleSaveState(&PersistenceMessage{
		Action:     "save_state",
		Path:       "ignored-by-provider",
		ResultChan: ch,
	})
	res := <-ch
	if res.Error != nil {
		t.Fatalf("save_state failed: %v", res.Error)
	}

	st, ok, err := persist.Load(sc.GetPersistConfig())
	if err != nil {
		t.Fatalf("persist load failed: %v", err)
	}
	if !ok {
		t.Fatalf("persist state not found")
	}
	if st.Wave != 9 || st.Tick != 4567 || st.Rand0 != 101 || st.Rand1 != 202 || st.TimeData != 77 {
		t.Fatalf("unexpected saved state: %+v", st)
	}
	if st.MapPath != "assets/worlds/test.msav" {
		t.Fatalf("unexpected map path: %s", st.MapPath)
	}
}

type noopPacket struct{}

func (p *noopPacket) Read(_ *protocol.Reader, _ int) error { return nil }
func (p *noopPacket) Write(_ *protocol.Writer) error       { return nil }
func (p *noopPacket) Priority() int                        { return 0 }
