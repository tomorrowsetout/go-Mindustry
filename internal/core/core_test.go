package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
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

func TestCore1StopStopsTickLoop(t *testing.T) {
	c1 := NewCore1("tick-stop")
	var ticks atomic.Int32
	c1.SetTickFn(func(_ uint64, _ time.Duration) {
		ticks.Add(1)
	})

	done := make(chan struct{})
	go func() {
		c1.Run(5 * time.Millisecond)
		close(done)
	}()

	time.Sleep(25 * time.Millisecond)
	c1.Stop()

	select {
	case <-done:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected Core1 run loop to stop after Stop")
	}

	got := ticks.Load()
	time.Sleep(20 * time.Millisecond)
	if ticks.Load() != got {
		t.Fatalf("expected tick count to stop advancing after Stop, before=%d after=%d", got, ticks.Load())
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

func TestServerCoreUnexpectedChildExitDetachesRemoteAndNotifies(t *testing.T) {
	sc := NewServerCore(time.Second/60, Config{Name: "test", MessageBuf: 4, WorkerCount: 1}, config.PersistConfig{})
	sc.Core2.AttachRemote(&ipcClient{})
	sc.Core3.AttachRemote(&ipcClient{})
	sc.Core4.AttachRemote(&ipcClient{})

	var called atomic.Int32
	sc.SetChildExitHandler(func(role string, err error) {
		if role != "core3" {
			t.Fatalf("unexpected role: %s", role)
		}
		if err == nil || err.Error() != "boom" {
			t.Fatalf("unexpected err: %v", err)
		}
		called.Add(1)
	})

	sc.handleUnexpectedChildExit("core3", errors.New("boom"))

	if sc.Core3.remoteClient() != nil {
		t.Fatal("expected core3 remote to be detached")
	}
	if !sc.Core2.HasRemote() {
		t.Fatal("expected unrelated core2 remote to remain attached")
	}
	if got := called.Load(); got != 1 {
		t.Fatalf("expected child exit handler to run once, got %d", got)
	}
}

func TestCore3ConcurrentRemoteAttachDetachAccess(t *testing.T) {
	c3 := NewCore3(Config{Name: "test", MessageBuf: 4, WorkerCount: 1})
	missingPath := filepath.Join(t.TempDir(), "missing.msav")

	var wg sync.WaitGroup
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 200; j++ {
				_, _ = c3.GetWorldCache(missingPath)
				_ = c3.InvalidateWorldCache(missingPath)
				_, _, _, _, _ = c3.Stats()
			}
		}()
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 400; i++ {
			c3.AttachRemote(&ipcClient{})
			c3.AttachRemote(nil)
		}
	}()

	wg.Wait()
}

func TestCore3RemoteFailureFallsBackToDirectLoadAfterRemoteOnlyStart(t *testing.T) {
	c3 := NewCore3(Config{Name: "snapshot", MessageBuf: 4, WorkerCount: 1})
	c3.AttachRemote(&ipcClient{})
	c3.Start()

	started := time.Now()
	_, err := c3.GetWorldCache(filepath.Join(t.TempDir(), "missing.msav"))
	if err == nil {
		t.Fatal("expected missing world cache path error")
	}
	if time.Since(started) > time.Second {
		t.Fatalf("expected direct fallback without queue deadlock, elapsed=%s", time.Since(started))
	}
	if c3.remoteClient() != nil {
		t.Fatal("expected remote snapshot client to detach after fallback")
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

func TestCore2ModLifecycleUsesRealFilesystemState(t *testing.T) {
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	if err := os.Chdir(t.TempDir()); err != nil {
		t.Fatalf("chdir temp dir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	modPath := filepath.Join("mods", "go", "hello.go")
	if err := os.MkdirAll(filepath.Dir(modPath), 0o755); err != nil {
		t.Fatalf("mkdir mods dir: %v", err)
	}
	if err := os.WriteFile(modPath, []byte("package main\nfunc main() {}\n"), 0o644); err != nil {
		t.Fatalf("write mod file: %v", err)
	}

	c2 := NewCore2(Config{Name: "mods", MessageBuf: 4, WorkerCount: 1})

	scanCh := make(chan ModResult, 1)
	c2.handleModScan(&ModMessage{ID: 1, Action: "scan", ResultChan: scanCh})
	scanRes := <-scanCh
	if scanRes.Error != nil || !scanRes.Success {
		t.Fatalf("scan failed: %+v", scanRes)
	}
	if len(c2.mods) != 1 {
		t.Fatalf("expected 1 scanned mod, got %d", len(c2.mods))
	}

	loadCh := make(chan ModResult, 1)
	c2.handleModLoad(&ModMessage{ID: 2, Action: "load", Path: modPath, ModType: "go", ResultChan: loadCh})
	loadRes := <-loadCh
	if loadRes.Error != nil || !loadRes.Success {
		t.Fatalf("load failed: %+v", loadRes)
	}

	startCh := make(chan ModResult, 1)
	c2.handleModStart(&ModMessage{ID: 3, Action: "start", Path: modPath, ModType: "go", ResultChan: startCh})
	startRes := <-startCh
	if startRes.Error != nil || !startRes.Success {
		t.Fatalf("start failed: %+v", startRes)
	}

	reloadCh := make(chan ModResult, 1)
	c2.handleModReload(&ModMessage{ID: 4, Action: "reload", Path: modPath, ModType: "go", ResultChan: reloadCh})
	reloadRes := <-reloadCh
	if reloadRes.Error != nil || !reloadRes.Success {
		t.Fatalf("reload failed: %+v", reloadRes)
	}

	stopCh := make(chan ModResult, 1)
	c2.handleModStop(&ModMessage{ID: 5, Action: "stop", Path: modPath, ModType: "go", ResultChan: stopCh})
	stopRes := <-stopCh
	if stopRes.Error != nil || !stopRes.Success {
		t.Fatalf("stop failed: %+v", stopRes)
	}

	unloadCh := make(chan ModResult, 1)
	c2.handleModUnload(&ModMessage{ID: 6, Action: "unload", Path: modPath, ModType: "go", ResultChan: unloadCh})
	unloadRes := <-unloadCh
	if unloadRes.Error != nil || !unloadRes.Success {
		t.Fatalf("unload failed: %+v", unloadRes)
	}

	c2.modMu.RLock()
	defer c2.modMu.RUnlock()
	var got *managedMod
	for _, mod := range c2.mods {
		got = mod
		break
	}
	if got == nil {
		t.Fatal("expected managed mod to remain registered")
	}
	if filepath.Base(got.Path) != "hello.go" {
		t.Fatalf("unexpected managed mod path: %s", got.Path)
	}
	if got.Loaded {
		t.Fatal("expected unload to clear loaded state")
	}
	if got.Running {
		t.Fatal("expected stop/unload to clear running state")
	}
}

func TestCore3WorldCacheProvidesAndInvalidatesCachedPayload(t *testing.T) {
	c3 := NewCore3(Config{Name: "snapshot", MessageBuf: 4, WorkerCount: 1})
	path := filepath.Join("..", "..", "assets", "worlds", "file.msav")
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			t.Skip("file.msav not present in workspace")
		}
		t.Fatalf("stat file.msav: %v", err)
	}

	res, err := c3.GetWorldCache(path)
	if err != nil {
		t.Fatalf("GetWorldCache: %v", err)
	}
	if len(res.Data) == 0 {
		t.Fatal("expected cached worldstream payload")
	}
	if res.BaseModel == nil {
		t.Fatal("expected cached base model")
	}

	if err := c3.InvalidateWorldCache(path); err != nil {
		t.Fatalf("InvalidateWorldCache: %v", err)
	}
	if c3.entry != nil {
		t.Fatal("expected invalidated path not to remain in active cache")
	}
}

func TestCore4PolicyRateLimitAndShardAssignment(t *testing.T) {
	c4 := NewCore4(Config{Name: "policy", MessageBuf: 4, WorkerCount: 1})
	c4.SetPolicyConfig(PolicyConfig{
		ConnectionBurst:  1,
		ConnectionWindow: time.Minute,
		PacketBurst:      1,
		PacketWindow:     time.Minute,
		PlayerShards:     8,
		CoreShards:       8,
	})

	firstConn, err := c4.AllowConnection("127.0.0.1", "uuid-a")
	if err != nil || !firstConn.Allowed {
		t.Fatalf("expected first connection to be allowed, got %+v err=%v", firstConn, err)
	}
	secondConn, err := c4.AllowConnection("127.0.0.1", "uuid-a")
	if err != nil {
		t.Fatalf("second AllowConnection error: %v", err)
	}
	if secondConn.Allowed {
		t.Fatal("expected second connection in same window to be rate-limited")
	}

	firstPacket, err := c4.AllowPacket("127.0.0.1", 1, "uuid-a", "*protocol.ConnectPacket")
	if err != nil || !firstPacket.Allowed {
		t.Fatalf("expected first packet to be allowed, got %+v err=%v", firstPacket, err)
	}
	secondPacket, err := c4.AllowPacket("127.0.0.1", 1, "uuid-a", "*protocol.ConnectPacket")
	if err != nil {
		t.Fatalf("second AllowPacket error: %v", err)
	}
	if secondPacket.Allowed {
		t.Fatal("expected second packet in same window to be rate-limited")
	}

	playerShardA, err := c4.PlayerShard("uuid-a", "127.0.0.1")
	if err != nil {
		t.Fatalf("PlayerShard A: %v", err)
	}
	playerShardA2, err := c4.PlayerShard("uuid-a", "127.0.0.1")
	if err != nil {
		t.Fatalf("PlayerShard A repeat: %v", err)
	}
	if playerShardA.PlayerShard != playerShardA2.PlayerShard || playerShardA.PlayerShard <= 0 {
		t.Fatalf("expected stable positive player shard, got %d and %d", playerShardA.PlayerShard, playerShardA2.PlayerShard)
	}

	coreShard, err := c4.CoreShard("map:origin.msav")
	if err != nil {
		t.Fatalf("CoreShard: %v", err)
	}
	if coreShard.CoreShard <= 0 {
		t.Fatalf("expected positive core shard, got %d", coreShard.CoreShard)
	}
}

func TestCore4RemoteFailureFallsBackToLocalPolicyAfterRemoteOnlyStart(t *testing.T) {
	c4 := NewCore4(Config{Name: "policy", MessageBuf: 4, WorkerCount: 1})
	c4.AttachRemote(&ipcClient{})
	c4.Start()

	started := time.Now()
	res, err := c4.AllowConnection("127.0.0.1", "uuid-a")
	if err != nil {
		t.Fatalf("expected local fallback instead of remote error, err=%v", err)
	}
	if !res.Allowed {
		t.Fatalf("expected local fallback policy to allow first connection, got %+v", res)
	}
	if time.Since(started) > time.Second {
		t.Fatalf("expected direct fallback without queue deadlock, elapsed=%s", time.Since(started))
	}
	if c4.remoteClient() != nil {
		t.Fatal("expected remote policy client to detach after fallback")
	}
}

func TestCore4PrunesExpiredPolicyWindows(t *testing.T) {
	c4 := NewCore4(Config{Name: "policy", MessageBuf: 4, WorkerCount: 1})
	c4.SetPolicyConfig(PolicyConfig{
		ConnectionBurst:  1,
		ConnectionWindow: time.Second,
		PacketBurst:      1,
		PacketWindow:     time.Second,
		PlayerShards:     4,
		CoreShards:       4,
	})

	c4.stateMu.Lock()
	now := time.Now()
	c4.connectionWindows["expired-conn"] = &policyWindow{count: 1, resetAt: now.Add(-time.Second)}
	c4.packetWindows["expired-packet"] = &policyWindow{count: 1, resetAt: now.Add(-time.Second)}
	c4.nextPruneAt = time.Time{}
	c4.stateMu.Unlock()

	if _, err := c4.AllowConnection("127.0.0.1", "uuid-prune"); err != nil {
		t.Fatalf("AllowConnection: %v", err)
	}

	c4.stateMu.Lock()
	defer c4.stateMu.Unlock()
	if _, ok := c4.connectionWindows["expired-conn"]; ok {
		t.Fatal("expected expired connection window to be pruned")
	}
	if _, ok := c4.packetWindows["expired-packet"]; ok {
		t.Fatal("expected expired packet window to be pruned")
	}
}

func TestEnableChildRolesRollsBackPendingChildrenOnSpawnFailure(t *testing.T) {
	oldSpawn := spawnChildCoreProcessFn
	oldClose := closeChildCoreProcessFn
	defer func() {
		spawnChildCoreProcessFn = oldSpawn
		closeChildCoreProcessFn = oldClose
	}()

	var closed []string
	closeChildCoreProcessFn = func(child *childCoreProcess) error {
		if child != nil {
			closed = append(closed, child.Role)
		}
		return nil
	}

	spawned := 0
	spawnChildCoreProcessFn = func(exePath, role string, extraArgs ...string) (*childCoreProcess, error) {
		spawned++
		if role == "core3" {
			return nil, fmt.Errorf("boom")
		}
		return &childCoreProcess{Role: role}, nil
	}

	sc := NewServerCore(time.Second/60, Config{Name: "test", MessageBuf: 4, WorkerCount: 1}, config.PersistConfig{})
	err := sc.EnableChildRoles("fake.exe", nil, "core2", "core3", "core4")
	if err == nil || err.Error() != "boom" {
		t.Fatalf("expected spawn failure, got %v", err)
	}
	if spawned != 2 {
		t.Fatalf("expected spawn attempts to stop on first failure, got %d", spawned)
	}
	if len(closed) != 1 || closed[0] != "core2" {
		t.Fatalf("expected only spawned core2 child to be rolled back, closed=%v", closed)
	}
	if sc.Core2.remoteClient() != nil || sc.Core3.remoteClient() != nil || sc.Core4.remoteClient() != nil {
		t.Fatal("expected no child remotes to attach after rollback")
	}
	if len(sc.supervisor.children) != 0 {
		t.Fatalf("expected supervisor to remain empty after rollback, got %d children", len(sc.supervisor.children))
	}
}

func TestCore2SaveStateUsesProvider(t *testing.T) {
	dir := t.TempDir()
	sc := NewServerCore(time.Second, Config{Name: "test", MessageBuf: 4, WorkerCount: 1}, config.PersistConfig{
		Enabled:   true,
		Directory: dir,
		File:      "latest.json",
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
