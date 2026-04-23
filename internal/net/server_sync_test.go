package net

import (
	"fmt"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func TestHandleConnectPacketSendsInitialWorldStreamWithoutWorldDataBegin(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.WorldDataFn = func(_ *Conn, pkt *protocol.ConnectPacket) ([]byte, error) {
		if pkt == nil || pkt.Name != "alpha" || pkt.UUID != "uuid-1" {
			t.Fatalf("unexpected connect packet passed to WorldDataFn: %+v", pkt)
		}
		return []byte("initial-world"), nil
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()

	worldID, ok := srv.Registry.PacketID(&protocol.WorldStream{})
	if !ok {
		t.Fatal("expected world stream packet id")
	}

	var sent []string
	var streamType byte
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch typed := obj.(type) {
		case *protocol.Remote_NetClient_worldDataBegin_28:
			sent = append(sent, "worldDataBegin")
		case *protocol.StreamBegin:
			sent = append(sent, "streamBegin")
			streamType = typed.Type
		case *protocol.StreamChunk:
			sent = append(sent, "streamChunk")
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.handlePacket(conn, &protocol.ConnectPacket{
		Version:     157,
		VersionType: "official",
		Name:        "alpha",
		Locale:      "en",
		UUID:        "uuid-1",
		USID:        "usid-1",
	}, true)

	if len(sent) < 2 {
		t.Fatalf("expected initial connect to send world stream, got %v", sent)
	}
	if sent[0] != "streamBegin" {
		t.Fatalf("expected initial connect to start with streamBegin, got %v", sent)
	}
	if sent[1] != "streamChunk" {
		t.Fatalf("expected initial connect to continue with streamChunk, got %v", sent)
	}
	for _, event := range sent {
		if event == "worldDataBegin" {
			t.Fatalf("expected initial connect not to send worldDataBegin, got %v", sent)
		}
	}
	if streamType != worldID {
		t.Fatalf("expected world stream type %d, got %d", worldID, streamType)
	}
	if !conn.hasBegunConnecting {
		t.Fatal("expected connect packet to mark connection as begun")
	}
	if conn.hasConnected {
		t.Fatal("expected initial connect not to mark connection fully connected before connectConfirm")
	}

	_ = conn.Close()
	<-done
}

func TestHandleConnectPacketRunsAcceptedHookBeforeDisplayRefreshAndEvent(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.WorldDataFn = func(_ *Conn, _ *protocol.ConnectPacket) ([]byte, error) {
		return []byte("initial-world"), nil
	}

	var accepted atomic.Bool
	srv.OnConnectAccepted = func(c *Conn, pkt *protocol.ConnectPacket) {
		if c == nil || pkt == nil {
			t.Fatal("expected accepted hook inputs")
		}
		accepted.Store(true)
	}
	srv.SetPlayerDisplayFormatter(func(c *Conn) string {
		if !accepted.Load() {
			t.Fatal("display formatter ran before OnConnectAccepted")
		}
		base := strings.TrimSpace(c.BaseName())
		if base == "" {
			base = "未知玩家"
		}
		return fmt.Sprintf("[accent]%s[]", base)
	})

	var sawConnectEvent atomic.Bool
	srv.OnEvent = func(ev NetEvent) {
		if ev.Kind != "connect_packet" {
			return
		}
		if !accepted.Load() {
			t.Fatal("connect_packet event emitted before OnConnectAccepted")
		}
		if ev.Name != "[accent]alpha[]" {
			t.Fatalf("expected refreshed display name in connect_packet event, got %q", ev.Name)
		}
		sawConnectEvent.Store(true)
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.handlePacket(conn, &protocol.ConnectPacket{
		Version:     157,
		VersionType: "official",
		Name:        "alpha",
		Locale:      "en",
		UUID:        "uuid-1",
		USID:        "usid-1",
	}, true)

	if !accepted.Load() {
		t.Fatal("expected OnConnectAccepted hook to run")
	}
	if !sawConnectEvent.Load() {
		t.Fatal("expected connect_packet event to be emitted")
	}
	if conn.name != "[accent]alpha[]" {
		t.Fatalf("expected refreshed connection display name, got %q", conn.name)
	}

	_ = conn.Close()
	<-done
}

func TestRefreshPlayerDisplayNameWithBaseNameFormatterIsIdempotent(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.SetPlayerDisplayFormatter(func(c *Conn) string {
		base := strings.TrimSpace(c.BaseName())
		if base == "" {
			base = "未知玩家"
		}
		return fmt.Sprintf("[scarlet]（未绑定）[]%s[gray]%d[]", base, c.id)
	})

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.id = 77

	srv.refreshPlayerDisplayName(conn)
	first := conn.name
	srv.refreshPlayerDisplayName(conn)
	second := conn.name

	if first != second {
		t.Fatalf("expected repeated refresh to be idempotent, first=%q second=%q", first, second)
	}
	if strings.Count(second, "（未绑定）") != 1 {
		t.Fatalf("expected single unbound prefix, got %q", second)
	}
	if strings.Count(second, "[gray]77[]") != 1 {
		t.Fatalf("expected single conn id suffix, got %q", second)
	}
}

func TestClientSnapshotDoesNotConfirmConnectionByDefault(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 11
	conn.hasBegunConnecting = true

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.handlePacket(conn, &protocol.Remote_NetServer_clientSnapshot_48{
		SnapshotID: 1,
		ViewWidth:  16,
		ViewHeight: 16,
	}, true)

	if conn.hasConnected {
		t.Fatal("expected Java-style connection flow to wait for connectConfirm, not clientSnapshot")
	}
	if got := conn.lastClientSnapshot.Load(); got != -1 {
		t.Fatalf("expected pre-confirm clientSnapshot to be ignored, got last=%d", got)
	}

	_ = conn.Close()
	<-done
}

func TestClientSnapshotIDWraparoundAcceptsNewerWrappedValue(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 41
	conn.hasBegunConnecting = true
	conn.hasConnected = true
	const maxSnapshotID int32 = 1<<31 - 1
	const minSnapshotID int32 = -1 << 31
	conn.lastClientSnapshot.Store(maxSnapshotID)
	conn.lastClientSnapshotSet.Store(true)

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.handlePacket(conn, &protocol.Remote_NetServer_clientSnapshot_48{
		SnapshotID: minSnapshotID,
		ViewWidth:  16,
		ViewHeight: 16,
	}, true)

	if got := conn.lastClientSnapshot.Load(); got != minSnapshotID {
		t.Fatalf("expected wrapped snapshot id %d to be accepted after %d, got %d", minSnapshotID, maxSnapshotID, got)
	}

	_ = conn.Close()
	<-done
}

func TestClientSnapshotIDWraparoundRejectsOlderWrappedValue(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 42
	conn.hasBegunConnecting = true
	conn.hasConnected = true
	const maxSnapshotID int32 = 1<<31 - 1
	const minSnapshotID int32 = -1 << 31
	conn.lastClientSnapshot.Store(minSnapshotID)
	conn.lastClientSnapshotSet.Store(true)

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.handlePacket(conn, &protocol.Remote_NetServer_clientSnapshot_48{
		SnapshotID: maxSnapshotID,
		ViewWidth:  16,
		ViewHeight: 16,
	}, true)

	if got := conn.lastClientSnapshot.Load(); got != minSnapshotID {
		t.Fatalf("expected older wrapped snapshot id %d to be rejected after %d, got %d", maxSnapshotID, minSnapshotID, got)
	}

	_ = conn.Close()
	<-done
}

func TestPostConnectLoopUsesSingleSyncTimeForStateAndEntitySnapshots(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.SetSnapshotIntervals(20, 80)

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 21
	conn.unitID = 2100
	conn.teamID = 1
	conn.hasConnected = true

	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{ID: unitID, X: 64, Y: 96, Health: 100, TeamID: 1, TypeID: 35}, true
	}

	srv.mu.Lock()
	srv.conns[conn] = struct{}{}
	srv.mu.Unlock()

	var stateSent atomic.Int32
	var entitySent atomic.Int32
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch obj.(type) {
		case *protocol.Remote_NetClient_stateSnapshot_35:
			stateSent.Add(1)
		case *protocol.Remote_NetClient_entitySnapshot_32:
			entitySent.Add(1)
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	go srv.postConnectLoop(conn)
	time.Sleep(120 * time.Millisecond)
	_ = conn.Close()
	<-done

	if stateSent.Load() == 0 {
		t.Fatal("expected state snapshots")
	}
	if entitySent.Load() == 0 {
		t.Fatal("expected entity snapshots")
	}
	if stateSent.Load() != entitySent.Load() {
		t.Fatalf("expected Java-style syncTime cycle to send state and entity together, state=%d entity=%d", stateSent.Load(), entitySent.Load())
	}
	if conn.syncTime.Load() == 0 {
		t.Fatal("expected syncTime to be recorded after periodic sync")
	}
}

func TestSyncWorldToConnSendsWorldDataBeginBeforeWorldStream(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.WorldDataFn = func(_ *Conn, _ *protocol.ConnectPacket) ([]byte, error) {
		return []byte("vanilla-world"), nil
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 91
	conn.unitID = 222
	conn.dead = false

	worldID, ok := srv.Registry.PacketID(&protocol.WorldStream{})
	if !ok {
		t.Fatal("expected world stream packet id")
	}

	var sent []string
	var streamType byte
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch typed := obj.(type) {
		case *protocol.Remote_NetClient_worldDataBegin_28:
			sent = append(sent, "worldDataBegin")
		case *protocol.StreamBegin:
			sent = append(sent, "streamBegin")
			streamType = typed.Type
		case *protocol.StreamChunk:
			sent = append(sent, "streamChunk")
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	if err := srv.SyncWorldToConn(conn); err != nil {
		t.Fatalf("SyncWorldToConn: %v", err)
	}

	if len(sent) < 3 {
		t.Fatalf("expected worldDataBegin + stream send sequence, got %v", sent)
	}
	if sent[0] != "worldDataBegin" {
		t.Fatalf("expected worldDataBegin first, got %v", sent)
	}
	if sent[1] != "streamBegin" {
		t.Fatalf("expected streamBegin second, got %v", sent)
	}
	if sent[2] != "streamChunk" {
		t.Fatalf("expected streamChunk third, got %v", sent)
	}
	if streamType != worldID {
		t.Fatalf("expected stream type %d, got %d", worldID, streamType)
	}
	if conn.dead {
		t.Fatal("expected SyncWorldToConn not to mark player dead")
	}
	if conn.unitID != 222 {
		t.Fatalf("expected SyncWorldToConn to keep unit id 222, got %d", conn.unitID)
	}

	_ = conn.Close()
	<-done
}

func TestSyncWorldToConnWaitsForReloadConnectConfirmBeforeHotReloadHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.WorldDataFn = func(_ *Conn, _ *protocol.ConnectPacket) ([]byte, error) {
		return []byte("reload-world"), nil
	}

	var hotReloadCalls atomic.Int32
	hotReloadDone := make(chan struct{}, 1)
	srv.OnHotReloadConnFn = func(_ *Conn) {
		hotReloadCalls.Add(1)
		select {
		case hotReloadDone <- struct{}{}:
		default:
		}
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	conn.playerID = 71
	conn.teamID = 1
	conn.hasConnected = true

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	if err := srv.SyncWorldToConn(conn); err != nil {
		t.Fatalf("SyncWorldToConn: %v", err)
	}

	select {
	case <-hotReloadDone:
		t.Fatal("expected hot reload hook to wait for reload connectConfirm")
	case <-time.After(450 * time.Millisecond):
	}
	if got := hotReloadCalls.Load(); got != 0 {
		t.Fatalf("expected no hot reload hook before confirm, got %d", got)
	}

	srv.handleOfficialConnectConfirm(conn, &protocol.Remote_NetServer_connectConfirm_50{})
	select {
	case <-hotReloadDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected reload connectConfirm to trigger hot reload hook")
	}
	if got := hotReloadCalls.Load(); got != 1 {
		t.Fatalf("expected exactly one hot reload hook after confirm, got %d", got)
	}

	_ = conn.Close()
	<-done
}

func TestReloadWorldLiveForAllSeparatesReloadChainFromInitialConnectAndSync(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	srv.WorldDataFn = func(_ *Conn, _ *protocol.ConnectPacket) ([]byte, error) {
		return []byte("reload-world"), nil
	}
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		return protocol.Point2{X: 7, Y: 9}, true
	}

	var spawnedUnitID int32
	srv.SpawnUnitFn = func(_ *Conn, unitID int32, pos protocol.Point2, unitType int16) (float32, float32, bool) {
		if pos.X != 7 || pos.Y != 9 {
			t.Fatalf("unexpected spawn tile %+v", pos)
		}
		if unitType != 35 {
			t.Fatalf("expected respawn unit type 35, got %d", unitType)
		}
		spawnedUnitID = unitID
		return 112, 144, true
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 44
	conn.unitID = 4400
	conn.teamID = 1
	conn.hasConnected = true

	srv.mu.Lock()
	srv.conns[conn] = struct{}{}
	srv.mu.Unlock()

	worldID, ok := srv.Registry.PacketID(&protocol.WorldStream{})
	if !ok {
		t.Fatal("expected world stream packet id")
	}

	var sent []string
	var streamType byte
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch typed := obj.(type) {
		case *protocol.Remote_NetClient_worldDataBegin_28:
			sent = append(sent, "worldDataBegin")
		case *protocol.StreamBegin:
			sent = append(sent, "streamBegin")
			streamType = typed.Type
		case *protocol.StreamChunk:
			sent = append(sent, "streamChunk")
		case *protocol.Remote_CoreBlock_playerSpawn_149:
			sent = append(sent, "playerSpawn")
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	reloaded, failed := srv.ReloadWorldLiveForAll()
	if reloaded != 1 || failed != 0 {
		t.Fatalf("expected one successful reload, got reloaded=%d failed=%d", reloaded, failed)
	}

	deadline := time.Now().Add(800 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(sent) >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if len(sent) < 3 {
		t.Fatalf("expected reload chain to send worldDataBegin and stream before reload confirm, got %v", sent)
	}
	if sent[0] != "worldDataBegin" {
		t.Fatalf("expected hot reload chain to begin with worldDataBegin, got %v", sent)
	}
	if sent[1] != "streamBegin" || sent[2] != "streamChunk" {
		t.Fatalf("expected hot reload chain to send world stream after worldDataBegin, got %v", sent)
	}
	if len(sent) > 3 {
		t.Fatalf("expected reload respawn to wait for connectConfirm, got %v", sent)
	}
	if streamType != worldID {
		t.Fatalf("expected hot reload stream type %d, got %d", worldID, streamType)
	}

	srv.handleOfficialConnectConfirm(conn, &protocol.Remote_NetServer_connectConfirm_50{})

	deadline = time.Now().Add(800 * time.Millisecond)
	for time.Now().Before(deadline) {
		if len(sent) >= 4 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(sent) < 4 || sent[3] != "playerSpawn" {
		t.Fatalf("expected reload connectConfirm to trigger respawn, got %v", sent)
	}
	if spawnedUnitID == 0 || conn.unitID != spawnedUnitID {
		t.Fatalf("expected hot reload confirm to replace the player unit, spawned=%d current=%d", spawnedUnitID, conn.unitID)
	}

	_ = conn.Close()
	<-done
}

func TestSyncEntitySnapshotsToConnSendsCurrentEntityPackets(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.ExtraEntitySnapshotEntitiesFn = func() ([]protocol.UnitSyncEntity, error) {
		return []protocol.UnitSyncEntity{
			&protocol.UnitEntitySync{
				IDValue:      7001,
				ClassIDValue: 30,
				ClassIDSet:   true,
				TypeID:       35,
				TeamID:       2,
				Health:       100,
				X:            24,
				Y:            40,
			},
		}, nil
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 77
	conn.hasConnected = true

	srv.mu.Lock()
	srv.conns[conn] = struct{}{}
	srv.mu.Unlock()

	var sentEntitySnapshot bool
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		if _, ok := obj.(*protocol.Remote_NetClient_entitySnapshot_32); ok {
			sentEntitySnapshot = true
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	if err := srv.SyncEntitySnapshotsToConn(conn); err != nil {
		t.Fatalf("SyncEntitySnapshotsToConn: %v", err)
	}
	if !sentEntitySnapshot {
		t.Fatal("expected SyncEntitySnapshotsToConn to send an entity snapshot packet")
	}

	_ = conn.Close()
	<-done
}

func TestSyncEntitySnapshotsToConnSendsHiddenSnapshotForViewer(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.ExtraEntitySnapshotEntitiesFn = func() ([]protocol.UnitSyncEntity, error) {
		return []protocol.UnitSyncEntity{
			&protocol.UnitEntitySync{
				IDValue:      7001,
				ClassIDValue: 30,
				ClassIDSet:   true,
				TypeID:       35,
				TeamID:       2,
				Health:       100,
				X:            24,
				Y:            40,
			},
		}, nil
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 77
	conn.hasConnected = true

	srv.EntitySnapshotHiddenFn = func(viewer *Conn, entity protocol.UnitSyncEntity) bool {
		return viewer == conn && entity != nil && entity.ID() == 7001
	}

	srv.mu.Lock()
	srv.conns[conn] = struct{}{}
	srv.mu.Unlock()

	var packets []*protocol.Remote_NetClient_entitySnapshot_32
	var hiddenIDs []int32
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch typed := obj.(type) {
		case *protocol.Remote_NetClient_entitySnapshot_32:
			packets = append(packets, typed)
		case *protocol.Remote_NetClient_hiddenSnapshot_33:
			hiddenIDs = append(hiddenIDs, typed.Ids.Items...)
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	if err := srv.SyncEntitySnapshotsToConn(conn); err != nil {
		t.Fatalf("SyncEntitySnapshotsToConn: %v", err)
	}
	if len(hiddenIDs) != 1 || hiddenIDs[0] != 7001 {
		t.Fatalf("expected hidden snapshot for entity 7001, got %v", hiddenIDs)
	}
	for _, packet := range packets {
		for _, entry := range decodeEntitySnapshotPacket(t, srv, packet) {
			if entry.ID == 7001 {
				t.Fatalf("expected hidden entity 7001 to be omitted from entitySnapshot, got %+v", entry)
			}
		}
	}

	_ = conn.Close()
	<-done
}
