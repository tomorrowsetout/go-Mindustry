package net

import (
	"bytes"
	"io"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

type testUnitType struct {
	id   int16
	name string
}

func (t testUnitType) ContentType() protocol.ContentType { return protocol.ContentUnit }
func (t testUnitType) ID() int16                         { return t.id }
func (t testUnitType) Name() string                      { return t.name }

type decodedSnapshotEntity struct {
	ID      int32
	ClassID byte
	Player  *protocol.PlayerEntity
	Unit    *protocol.UnitEntitySync
}

func decodeEntitySnapshotPacket(t *testing.T, srv *Server, packet *protocol.Remote_NetClient_entitySnapshot_32) []decodedSnapshotEntity {
	t.Helper()
	if packet == nil {
		return nil
	}
	r := protocol.NewReaderWithContext(packet.Data, srv.TypeIO)
	out := make([]decodedSnapshotEntity, 0, int(packet.Amount))
	for i := 0; i < int(packet.Amount); i++ {
		id, err := r.ReadInt32()
		if err != nil {
			t.Fatalf("read entity id failed: %v", err)
		}
		classID, err := r.ReadByte()
		if err != nil {
			t.Fatalf("read entity class failed: %v", err)
		}
		entry := decodedSnapshotEntity{ID: id, ClassID: classID}
		switch classID {
		case 12:
			player := &protocol.PlayerEntity{IDValue: id}
			if err := player.ReadSync(r); err != nil {
				t.Fatalf("read player sync failed: %v", err)
			}
			entry.Player = player
		default:
			unit := &protocol.UnitEntitySync{IDValue: id, ClassIDValue: classID, ClassIDSet: true}
			if err := unit.ReadSync(r); err != nil {
				t.Fatalf("read unit sync failed: %v", err)
			}
			entry.Unit = unit
		}
		out = append(out, entry)
	}
	if rem := r.Remaining(); rem != 0 {
		t.Fatalf("expected entity snapshot packet to be fully consumed, remaining=%d", rem)
	}
	return out
}

func TestMaybeRespawnKeepsAliveWorldUnitWhenMirrorMissing(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{
		playerID:       7,
		unitID:         12345,
		teamID:         1,
		snapX:          64,
		snapY:          96,
		lastSpawnAt:    time.Now().Add(-5 * time.Second),
		lastRespawnReq: time.Time{},
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:        unitID,
			X:         64,
			Y:         96,
			Health:    240,
			MaxHealth: 240,
			TeamID:    1,
			TypeID:    37,
		}, true
	}

	srv.maybeRespawn(conn)

	if conn.dead {
		t.Fatalf("expected alive connection, got dead")
	}
	if conn.unitID != 12345 {
		t.Fatalf("expected unit id to stay unchanged, got %d", conn.unitID)
	}
	srv.entityMu.Lock()
	ent, ok := srv.entities[conn.unitID]
	srv.entityMu.Unlock()
	if !ok {
		t.Fatalf("expected unit mirror entity to be recreated")
	}
	unit, ok := ent.(*protocol.UnitEntitySync)
	if !ok {
		t.Fatalf("expected unit mirror entity type, got %T", ent)
	}
	if unit.Health != 240 {
		t.Fatalf("expected mirrored health=240, got %f", unit.Health)
	}
	if unit.TeamID != 1 {
		t.Fatalf("expected mirrored team=1, got %d", unit.TeamID)
	}
}

func TestConnectedTeamCountsSkipsUnassignedConns(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	assigned := &Conn{playerID: 1, hasBegunConnecting: true, teamID: 2}
	unassigned := &Conn{playerID: 2, hasBegunConnecting: true, teamID: 0}
	srv.conns[assigned] = struct{}{}
	srv.conns[unassigned] = struct{}{}

	counts := srv.ConnectedTeamCounts()
	if len(counts) != 1 {
		t.Fatalf("expected only assigned teams to be counted, got %+v", counts)
	}
	if counts[2] != 1 {
		t.Fatalf("expected team 2 count 1, got %+v", counts)
	}
	if _, ok := counts[0]; ok {
		t.Fatalf("expected team 0 to be ignored, got %+v", counts)
	}
}

func TestMaybeRespawnRevivesDeadFlagWhenWorldUnitStillAlive(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{
		playerID: 9,
		unitID:   54321,
		teamID:   1,
		dead:     true,
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:     unitID,
			Health: 120,
			TeamID: 1,
			TypeID: 37,
		}, true
	}

	srv.maybeRespawn(conn)

	if conn.dead {
		t.Fatalf("expected dead flag to be cleared when unit is alive")
	}
	if conn.deathTimer != 0 {
		t.Fatalf("expected death timer reset, got %f", conn.deathTimer)
	}
}

func TestHandleOfficialUnitClearRespawnsAliveCoreUnit(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		return protocol.Point2{X: 4, Y: 6}, true
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 12
	conn.unitID = 777
	conn.teamID = 1
	conn.dead = false
	conn.lastSpawnAt = time.Now()

	var spawnedUnitID int32
	srv.SpawnUnitFn = func(_ *Conn, unitID int32, pos protocol.Point2, unitType int16) (float32, float32, bool) {
		if pos.X != 4 || pos.Y != 6 {
			t.Fatalf("unexpected spawn tile %+v", pos)
		}
		if unitType != 35 {
			t.Fatalf("expected respawn unit type 35, got %d", unitType)
		}
		spawnedUnitID = unitID
		return 64, 96, true
	}

	var spawnPackets int
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch obj.(type) {
		case *protocol.Remote_CoreBlock_playerSpawn_149:
			spawnPackets++
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.handleOfficialUnitClear(conn)

	if conn.dead {
		t.Fatal("expected official unitClear to finish with an alive player")
	}
	if spawnedUnitID == 0 {
		t.Fatal("expected official unitClear to spawn a replacement unit")
	}
	if conn.unitID != spawnedUnitID {
		t.Fatalf("expected conn unit id to update to %d, got %d", spawnedUnitID, conn.unitID)
	}
	if conn.unitID == 777 {
		t.Fatalf("expected official unitClear to replace old unit 777, got %d", conn.unitID)
	}
	if spawnPackets != 1 {
		t.Fatalf("expected exactly one playerSpawn packet, got %d", spawnPackets)
	}

	_ = conn.Close()
	<-done
}

func TestSpawnRespawnUnitDoesNotCreateMirrorWhenWorldSpawnFails(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		return protocol.Point2{X: 4, Y: 6}, true
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 220
	conn.teamID = 1

	srv.SpawnUnitFn = func(_ *Conn, _ int32, _ protocol.Point2, _ int16) (float32, float32, bool) {
		return 0, 0, false
	}

	var spawnPackets int
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		if _, ok := obj.(*protocol.Remote_CoreBlock_playerSpawn_149); ok {
			spawnPackets++
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	if srv.spawnRespawnUnit(conn) {
		t.Fatal("expected respawn to fail when world spawn fails")
	}
	if conn.unitID != 0 {
		t.Fatalf("expected failed respawn not to leave a unit id, got %d", conn.unitID)
	}
	if spawnPackets != 0 {
		t.Fatalf("expected failed respawn not to send playerSpawn, got %d", spawnPackets)
	}
	srv.entityMu.Lock()
	defer srv.entityMu.Unlock()
	if len(srv.entities) != 0 {
		t.Fatalf("expected failed respawn not to create mirror entities, got %d", len(srv.entities))
	}

	_ = conn.Close()
	<-done
}

func TestSpawnRespawnUnitRollsBackWhenPlayerSpawnPacketFails(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		return protocol.Point2{X: 4, Y: 6}, true
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 221
	conn.teamID = 1
	_ = conn.Close()

	var spawnedUnitID int32
	var droppedUnitID int32
	srv.SpawnUnitFn = func(_ *Conn, unitID int32, _ protocol.Point2, _ int16) (float32, float32, bool) {
		spawnedUnitID = unitID
		return 64, 96, true
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != spawnedUnitID {
			return UnitInfo{}, false
		}
		return UnitInfo{ID: unitID, X: 64, Y: 96, Health: 100, TeamID: 1, TypeID: 35}, true
	}
	srv.DropUnitFn = func(unitID int32) {
		droppedUnitID = unitID
	}

	if srv.spawnRespawnUnit(conn) {
		t.Fatal("expected respawn to fail when playerSpawn send fails on closed connection")
	}
	if spawnedUnitID == 0 {
		t.Fatal("expected test respawn to create a world unit before send failure")
	}
	if conn.unitID != 0 {
		t.Fatalf("expected failed send to clear conn unit id, got %d", conn.unitID)
	}
	if droppedUnitID != spawnedUnitID {
		t.Fatalf("expected failed send to roll back spawned unit %d, dropped=%d", spawnedUnitID, droppedUnitID)
	}
}

func TestMaybeRespawnDoesNotSelfKillWhenStateTemporarilyMissing(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{
		playerID:    15,
		unitID:      8888,
		teamID:      1,
		lastSpawnAt: time.Now().Add(-10 * time.Second),
	}

	srv.maybeRespawn(conn)

	if conn.dead {
		t.Fatalf("expected connection to stay alive when no explicit death was received")
	}
	if conn.deathTimer != 0 {
		t.Fatalf("expected death timer reset, got %f", conn.deathTimer)
	}
}

func TestHandleOfficialUnitClearIgnoresStaleEchoRightAfterRespawn(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{
		playerID:    18,
		unitID:      1800,
		teamID:      1,
		lastSpawnAt: time.Now(),
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:        unitID,
			X:         96,
			Y:         120,
			Health:    150,
			MaxHealth: 150,
			TeamID:    1,
			TypeID:    35,
		}, true
	}

	srv.handleOfficialUnitClear(conn)

	if conn.dead {
		t.Fatal("expected stale unitClear echo not to kill a freshly respawned player")
	}
	if conn.unitID != 1800 {
		t.Fatalf("expected stale unitClear echo to keep current unit, got %d", conn.unitID)
	}
}

func TestHandleOfficialUnitClearIgnoresFreshSpawnWhenWorldStillMissing(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{
		playerID:       181,
		unitID:         1810,
		teamID:         1,
		lastSpawnAt:    time.Now(),
		lastRespawnReq: time.Now(),
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) { return UnitInfo{}, false }

	srv.handleOfficialUnitClear(conn)

	if conn.dead {
		t.Fatal("expected fresh-spawn unitClear echo not to kill player while world binding is still settling")
	}
	if conn.unitID != 1810 {
		t.Fatalf("expected unit id to stay unchanged, got %d", conn.unitID)
	}
	if conn.lastRespawnReq.IsZero() {
		t.Fatal("expected recent respawn marker to remain present after ignored stale unitClear")
	}
}

func TestHandleOfficialUnitClearIgnoredDuringInitialWorldReloadGrace(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{
		playerID:     1811,
		teamID:       1,
		dead:         true,
		unitID:       0,
		snapX:        64,
		snapY:        96,
		hasConnected: true,
	}
	conn.SetWorldReloadGrace(2 * time.Second)

	var spawnCalls int
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		spawnCalls++
		return protocol.Point2{X: 5, Y: 7}, true
	}
	srv.SpawnUnitFn = func(_ *Conn, _ int32, _ protocol.Point2, _ int16) (float32, float32, bool) {
		t.Fatal("expected initial-world-reload unitClear to be ignored before spawn")
		return 0, 0, false
	}

	srv.handleOfficialUnitClear(conn)

	if spawnCalls != 0 {
		t.Fatalf("expected no spawn tile lookup during initial world reload grace, got %d", spawnCalls)
	}
	if !conn.dead {
		t.Fatal("expected ignored initial unitClear to keep player in pre-spawn dead state")
	}
	if conn.unitID != 0 {
		t.Fatalf("expected no unit to be spawned yet, got %d", conn.unitID)
	}
	if !conn.lastRespawnReq.IsZero() {
		t.Fatalf("expected ignored initial unitClear not to queue a respawn, got %v", conn.lastRespawnReq)
	}
}

func TestHandleOfficialUnitClearKeepsAuthoritativeAliveUnit(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 36, name: "beta"})

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 182
	conn.unitID = 1820
	conn.teamID = 1
	conn.lastSpawnAt = time.Now().Add(-3 * time.Second)
	conn.snapX = 64
	conn.snapY = 96

	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:        unitID,
			X:         80,
			Y:         120,
			Health:    170,
			MaxHealth: 170,
			TeamID:    1,
			TypeID:    36,
		}, true
	}

	var setPositionPackets int
	var spawnPackets int
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch obj.(type) {
		case *protocol.Remote_NetClient_setPosition_29:
			setPositionPackets++
		case *protocol.Remote_CoreBlock_playerSpawn_149:
			spawnPackets++
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.handleOfficialUnitClear(conn)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if setPositionPackets > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if conn.dead {
		t.Fatal("expected authoritative alive unitClear to keep player alive")
	}
	if conn.unitID != 1820 {
		t.Fatalf("expected authoritative alive unit to remain bound, got %d", conn.unitID)
	}
	if spawnPackets != 0 {
		t.Fatalf("expected no respawn packets for authoritative alive unitClear, got %d", spawnPackets)
	}
	if setPositionPackets == 0 {
		t.Fatal("expected alive unitClear ignore to resend authoritative position")
	}
	if conn.lastSpawnRepairAt.IsZero() {
		t.Fatal("expected alive unitClear ignore to record a repair sync")
	}
	if !conn.lastRespawnReq.IsZero() {
		t.Fatal("expected alive unitClear ignore not to queue a respawn")
	}

	_ = conn.Close()
	<-done
}

func TestHandleOfficialUnitClearAliveRepairCanRepeatAfterPreviousRepair(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 36, name: "beta"})

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	prevRepair := time.Now().Add(-1200 * time.Millisecond)
	conn.playerID = 183
	conn.unitID = 1830
	conn.teamID = 1
	conn.lastSpawnAt = time.Now().Add(-5 * time.Second)
	conn.lastSpawnRepairAt = prevRepair
	conn.snapX = 96
	conn.snapY = 128

	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:        unitID,
			X:         112,
			Y:         144,
			Health:    170,
			MaxHealth: 170,
			TeamID:    1,
			TypeID:    36,
		}, true
	}

	var setPositionPackets int
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch obj.(type) {
		case *protocol.Remote_NetClient_setPosition_29:
			setPositionPackets++
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.handleOfficialUnitClear(conn)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if setPositionPackets > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if setPositionPackets == 0 {
		t.Fatal("expected authoritative alive unitClear to resend a repair even after an earlier spawn repair")
	}
	if !conn.lastSpawnRepairAt.After(prevRepair) {
		t.Fatalf("expected repeated alive repair to refresh repair timestamp, prev=%v got=%v", prevRepair, conn.lastSpawnRepairAt)
	}

	_ = conn.Close()
	<-done
}

func TestEnsurePlayerUnitEntityPrefersWorldType(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.Content.RegisterUnitType(testUnitType{id: 37, name: "gamma"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	conn := &Conn{
		playerID: 21,
		unitID:   4444,
		teamID:   1,
		snapX:    120,
		snapY:    144,
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:     unitID,
			X:      128,
			Y:      160,
			Health: 220,
			TeamID: 1,
			TypeID: 37,
		}, true
	}

	unit := srv.ensurePlayerUnitEntity(conn)
	if unit == nil {
		t.Fatalf("expected mirrored unit entity")
	}
	if unit.TypeID != 37 {
		t.Fatalf("expected world type 37 to win over configured spawn type, got %d", unit.TypeID)
	}
	if unit.X != 128 || unit.Y != 160 {
		t.Fatalf("expected mirrored position from world, got (%.1f, %.1f)", unit.X, unit.Y)
	}
}

func TestShouldForceRespawnAfterDeadIgnoredEscalatesRecentSpawn(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	now := time.Now()
	conn := &Conn{
		playerID:    31,
		unitID:      9001,
		teamID:      1,
		lastSpawnAt: now.Add(-800 * time.Millisecond),
	}

	if srv.shouldForceRespawnAfterDeadIgnored(conn, now) {
		t.Fatalf("first ignored dead should not force respawn")
	}
	if srv.shouldForceRespawnAfterDeadIgnored(conn, now.Add(200*time.Millisecond)) {
		t.Fatalf("second ignored dead should not force respawn")
	}
	if !srv.shouldForceRespawnAfterDeadIgnored(conn, now.Add(400*time.Millisecond)) {
		t.Fatalf("third ignored dead after recent spawn should force respawn")
	}
}

func TestClientDeadWorldMissingRightAfterRespawnIsIgnored(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{
		playerID:     321,
		unitID:       3210,
		teamID:       1,
		hasConnected: true,
		lastSpawnAt:  time.Now(),
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{}, false
	}

	srv.handlePacket(conn, &protocol.Remote_NetServer_clientSnapshot_48{
		SnapshotID: 1,
		UnitID:     conn.unitID,
		Dead:       true,
		X:          64,
		Y:          96,
	}, false)

	if conn.dead {
		t.Fatal("expected world-missing dead echo right after respawn to be ignored")
	}
	if conn.unitID != 3210 {
		t.Fatalf("expected connection unit to stay bound, got %d", conn.unitID)
	}
	if !conn.lastRespawnReq.IsZero() {
		t.Fatalf("expected ignored client-dead echo not to queue respawn, got %v", conn.lastRespawnReq)
	}
}

func TestClientDeadWorldMissingDuringRespawnWindowIsIgnored(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	now := time.Now()
	conn := &Conn{
		playerID:       322,
		unitID:         3220,
		teamID:         1,
		hasConnected:   true,
		dead:           true,
		lastRespawnReq: now.Add(-150 * time.Millisecond),
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{}, false
	}

	srv.handlePacket(conn, &protocol.Remote_NetServer_clientSnapshot_48{
		SnapshotID: 2,
		UnitID:     conn.unitID,
		Dead:       true,
		X:          72,
		Y:          104,
	}, false)

	if !conn.dead {
		t.Fatal("expected in-flight respawn dead echo to keep player in respawn state")
	}
	if conn.unitID != 3220 {
		t.Fatalf("expected respawning unit id to stay unchanged, got %d", conn.unitID)
	}
	if age := time.Since(conn.lastRespawnReq); age < 0 || age > 2*time.Second {
		t.Fatalf("expected ignored dead echo not to overwrite respawn timing, got age=%s", age)
	}
}

func TestClientDeadAliveUnitDuringRespawnWindowSkipsRepair(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	now := time.Now()
	conn := &Conn{
		playerID:     3221,
		unitID:       32210,
		teamID:       1,
		hasConnected: true,
		lastSpawnAt:  now.Add(-150 * time.Millisecond),
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:        unitID,
			X:         72,
			Y:         104,
			Health:    220,
			MaxHealth: 220,
			TeamID:    1,
			TypeID:    37,
		}, true
	}

	srv.handlePacket(conn, &protocol.Remote_NetServer_clientSnapshot_48{
		SnapshotID: 3,
		UnitID:     conn.unitID,
		Dead:       true,
		X:          72,
		Y:          104,
	}, false)

	if conn.dead {
		t.Fatal("expected recent-spawn dead echo with alive authoritative unit to be ignored")
	}
	if conn.unitID != 32210 {
		t.Fatalf("expected authoritative unit binding to remain unchanged, got %d", conn.unitID)
	}
	if !conn.lastSpawnRepairAt.IsZero() {
		t.Fatalf("expected recent-spawn dead echo to skip repair snapshot spam, got repairAt=%v", conn.lastSpawnRepairAt)
	}
	if conn.clientDeadIgnores != 0 {
		t.Fatalf("expected recent-spawn dead echo not to increment ignore escalation, got %d", conn.clientDeadIgnores)
	}
	if !conn.lastDeadIgnoreAt.IsZero() {
		t.Fatalf("expected recent-spawn dead echo to avoid dead-ignore timers, got %v", conn.lastDeadIgnoreAt)
	}
}

func TestClientDeadUnitZeroRightAfterRespawnRequestIsIgnored(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	now := time.Now()
	conn := &Conn{
		playerID:       654,
		unitID:         0,
		teamID:         1,
		hasConnected:   true,
		dead:           true,
		lastRespawnReq: now.Add(-400 * time.Millisecond),
	}

	srv.handlePacket(conn, &protocol.Remote_NetServer_clientSnapshot_48{
		SnapshotID: 2,
		UnitID:     0,
		Dead:       true,
		X:          80,
		Y:          96,
	}, false)

	if !conn.dead {
		t.Fatal("expected recent respawn dead echo to avoid reviving the player early")
	}
	if conn.unitID != 0 {
		t.Fatalf("expected unit id to remain zero during respawn window, got %d", conn.unitID)
	}
	if age := time.Since(conn.lastRespawnReq); age < 0 || age > 2*time.Second {
		t.Fatalf("expected ignored dead echo not to overwrite respawn timing, got age=%s", age)
	}
}

func TestShouldForceRespawnAfterDeadIgnoredResetsAfterGap(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	now := time.Now()
	conn := &Conn{
		playerID:    32,
		unitID:      9002,
		teamID:      1,
		lastSpawnAt: now.Add(-2 * time.Second),
	}

	if srv.shouldForceRespawnAfterDeadIgnored(conn, now) {
		t.Fatalf("first ignored dead should not force respawn")
	}
	if srv.shouldForceRespawnAfterDeadIgnored(conn, now.Add(2*time.Second)) {
		t.Fatalf("ignore counter should reset after a long gap")
	}
	if conn.clientDeadIgnores != 1 {
		t.Fatalf("expected ignore counter reset to 1, got %d", conn.clientDeadIgnores)
	}
}

func TestShouldForceRespawnAfterDeadIgnoredStopsAfterRepairForCurrentSpawn(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	now := time.Now()
	conn := &Conn{
		playerID:          320,
		unitID:            9020,
		teamID:            1,
		lastSpawnAt:       now.Add(-800 * time.Millisecond),
		lastSpawnRepairAt: now.Add(-100 * time.Millisecond),
	}

	if srv.shouldForceRespawnAfterDeadIgnored(conn, now) {
		t.Fatalf("expected ignored dead escalation to stop after one repair for the current spawn")
	}
	if conn.clientDeadIgnores != 0 {
		t.Fatalf("expected ignore counter to stay idle after repair, got %d", conn.clientDeadIgnores)
	}
}

func TestRepairAliveSpawnBindingSendsAliveSnapshotsWithoutRespawn(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 37, name: "gamma"})
	srv.PlayerUnitTypeFn = func() int16 { return 37 }
	srv.UdpFallbackTCP = true

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 33
	conn.unitID = 3300
	conn.teamID = 1
	conn.dead = true
	conn.snapX = 64
	conn.snapY = 96
	conn.lastSpawnAt = time.Now().Add(-10 * time.Second)

	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:        unitID,
			X:         128,
			Y:         160,
			Health:    220,
			MaxHealth: 220,
			TeamID:    1,
			TypeID:    37,
		}, true
	}

	var spawnPackets int
	var entityPackets int
	var positionPackets int
	var snapshot *protocol.Remote_NetClient_entitySnapshot_32
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch typed := obj.(type) {
		case *protocol.Remote_CoreBlock_playerSpawn_149:
			spawnPackets++
		case *protocol.Remote_NetClient_entitySnapshot_32:
			entityPackets++
			snapshot = typed
		case *protocol.Remote_NetClient_setPosition_29:
			positionPackets++
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.repairClientDeadAliveBinding(conn)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if entityPackets > 0 && positionPackets > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if conn.dead {
		t.Fatal("expected alive binding repair to clear dead flag")
	}
	if conn.snapX != 128 || conn.snapY != 160 {
		t.Fatalf("expected repaired position (128,160), got (%.1f, %.1f)", conn.snapX, conn.snapY)
	}
	if spawnPackets != 0 {
		t.Fatalf("expected no playerSpawn packet during alive repair, got %d", spawnPackets)
	}
	if entityPackets == 0 || snapshot == nil {
		t.Fatal("expected alive repair to send entity snapshot")
	}
	if positionPackets == 0 {
		t.Fatal("expected alive repair to send setPosition")
	}

	entries := decodeEntitySnapshotPacket(t, srv, snapshot)
	var sawPlayer bool
	var sawUnit bool
	for _, entry := range entries {
		if entry.Player != nil && entry.ID == conn.playerID {
			sawPlayer = true
			if entry.Player.Unit == nil || entry.Player.Unit.ID() != conn.unitID {
				t.Fatalf("expected repaired player snapshot to reference unit %d, got %+v", conn.unitID, entry.Player.Unit)
			}
		}
		if entry.Unit != nil && entry.ID == conn.unitID {
			sawUnit = true
			if entry.Unit.TypeID != 37 {
				t.Fatalf("expected repaired unit snapshot type 37, got %d", entry.Unit.TypeID)
			}
		}
	}
	if !sawPlayer || !sawUnit {
		t.Fatalf("expected repaired snapshot to include both player and unit, got %+v", entries)
	}

	_ = conn.Close()
	<-done
}

func TestRepairAliveSpawnBindingOnlyRepairsOncePerSpawn(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 37, name: "gamma"})
	srv.PlayerUnitTypeFn = func() int16 { return 37 }
	srv.UdpFallbackTCP = true

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 34
	conn.unitID = 3400
	conn.teamID = 1
	conn.dead = true
	conn.snapX = 64
	conn.snapY = 96
	conn.lastSpawnAt = time.Now().Add(-10 * time.Second)

	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:        unitID,
			X:         96,
			Y:         128,
			Health:    220,
			MaxHealth: 220,
			TeamID:    1,
			TypeID:    37,
		}, true
	}

	var entityPackets int
	var positionPackets int
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch obj.(type) {
		case *protocol.Remote_NetClient_entitySnapshot_32:
			entityPackets++
		case *protocol.Remote_NetClient_setPosition_29:
			positionPackets++
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.repairClientDeadAliveBinding(conn)
	firstRepairAt := conn.lastSpawnRepairAt
	if firstRepairAt.IsZero() {
		t.Fatal("expected first alive repair to record repair timestamp")
	}

	srv.repairClientDeadStuckBinding(conn)

	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if entityPackets > 0 && positionPackets > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if entityPackets != 1 {
		t.Fatalf("expected exactly one entity snapshot repair for a spawn, got %d", entityPackets)
	}
	if positionPackets != 1 {
		t.Fatalf("expected exactly one position repair for a spawn, got %d", positionPackets)
	}
	if !conn.lastSpawnRepairAt.Equal(firstRepairAt) {
		t.Fatalf("expected second repair attempt to be ignored, first=%v second=%v", firstRepairAt, conn.lastSpawnRepairAt)
	}

	_ = conn.Close()
	<-done
}

func TestEnsurePlayerUnitEntitySkipsCollidingPlayerIDs(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	nextReserved := int32(0)
	srv.ReserveUnitIDFn = func() int32 {
		nextReserved++
		return nextReserved
	}

	connA := &Conn{playerID: 1, hasConnected: true, teamID: 1}
	connB := &Conn{playerID: 2, hasConnected: true, teamID: 1}
	srv.mu.Lock()
	srv.conns[connA] = struct{}{}
	srv.conns[connB] = struct{}{}
	srv.mu.Unlock()

	player := srv.ensurePlayerEntity(connA)
	if player == nil {
		t.Fatal("expected player entity")
	}
	unit := srv.ensurePlayerUnitEntity(connA)
	if unit == nil {
		t.Fatal("expected unit entity")
	}
	if connA.unitID == connA.playerID {
		t.Fatalf("expected unit id to differ from player id, got %d", connA.unitID)
	}
	if connA.unitID == connB.playerID {
		t.Fatalf("expected unit id to skip other player id, got %d", connA.unitID)
	}
	if connA.unitID != 3 {
		t.Fatalf("expected first collision-free reserved id to be 3, got %d", connA.unitID)
	}

	srv.entityMu.Lock()
	defer srv.entityMu.Unlock()
	if got, ok := srv.entities[connA.playerID].(*protocol.PlayerEntity); !ok || got == nil {
		t.Fatalf("expected player entity to remain mapped at player id, got %T", srv.entities[connA.playerID])
	}
	if got, ok := srv.entities[connA.unitID].(*protocol.UnitEntitySync); !ok || got == nil {
		t.Fatalf("expected unit entity to remain mapped at unit id, got %T", srv.entities[connA.unitID])
	}
}

func TestNextPlayerIDUsesSharedEntityAllocator(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	nextReserved := int32(0)
	srv.ReserveUnitIDFn = func() int32 {
		nextReserved++
		return nextReserved
	}

	connA := &Conn{playerID: srv.nextPlayerID(), hasConnected: true, teamID: 1}
	if connA.playerID != 1 {
		t.Fatalf("expected first player id to use shared allocator and be 1, got %d", connA.playerID)
	}

	srv.mu.Lock()
	srv.conns[connA] = struct{}{}
	srv.mu.Unlock()

	connA.unitID = srv.nextUnitID()
	if connA.unitID != 2 {
		t.Fatalf("expected first unit id to follow shared allocator and be 2, got %d", connA.unitID)
	}

	if got := srv.nextPlayerID(); got != 3 {
		t.Fatalf("expected second player id to skip first unit id and be 3, got %d", got)
	}
}

func TestNextPlayerIDSkipsLiveUnitIDsBeforeMirror(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.playerIDNext = 1

	conn := &Conn{playerID: 1, unitID: 2, hasConnected: true, teamID: 1}
	srv.mu.Lock()
	srv.conns[conn] = struct{}{}
	srv.mu.Unlock()

	if got := srv.nextPlayerID(); got != 3 {
		t.Fatalf("expected next player id to skip live unit id 2 and return 3, got %d", got)
	}
}

func TestNextUnitIDSkipsExistingEntityIDs(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	nextReserved := int32(40)
	srv.ReserveUnitIDFn = func() int32 {
		nextReserved++
		return nextReserved
	}

	srv.entityMu.Lock()
	srv.entities[41] = &protocol.PlayerEntity{IDValue: 41}
	srv.entityMu.Unlock()

	if got := srv.nextUnitID(); got != 42 {
		t.Fatalf("expected next unit id to skip occupied entity id 41 and return 42, got %d", got)
	}
}

func TestPrepareUnitEntitySnapshotFallsBackToPlayerUnitType(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }

	unit := &protocol.UnitEntitySync{
		IDValue:       9001,
		Controller:    &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: 77},
		Health:        100,
		TeamID:        1,
		TypeID:        0,
		Abilities:     []protocol.Ability{},
		Mounts:        []protocol.WeaponMount{},
		Plans:         []*protocol.BuildPlan{},
		Statuses:      []protocol.StatusEntry{},
		Stack:         protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
		Elevation:     1,
		Rotation:      90,
		Vel:           protocol.Vec2{},
		SpawnedByCore: true,
	}

	if !srv.prepareUnitEntitySnapshot(unit) {
		t.Fatal("expected invalid player unit type to fall back to configured player spawn type")
	}
	if unit.TypeID != 35 {
		t.Fatalf("expected fallback unit type 35, got %d", unit.TypeID)
	}
}

func TestBuildPlayerEntitySnapshotDropsInvalidUnitReferenceWhenNoFallback(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{
		playerID:     12,
		unitID:       1200,
		teamID:       1,
		hasConnected: true,
	}

	srv.mu.Lock()
	srv.conns[conn] = struct{}{}
	srv.mu.Unlock()

	srv.entityMu.Lock()
	srv.entities[conn.unitID] = &protocol.UnitEntitySync{
		IDValue:       conn.unitID,
		Controller:    &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: conn.playerID},
		Health:        100,
		TeamID:        1,
		TypeID:        0,
		Abilities:     []protocol.Ability{},
		Mounts:        []protocol.WeaponMount{},
		Plans:         []*protocol.BuildPlan{},
		Statuses:      []protocol.StatusEntry{},
		Stack:         protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
		Elevation:     1,
		Rotation:      90,
		Vel:           protocol.Vec2{},
		SpawnedByCore: true,
	}
	srv.entityMu.Unlock()

	amount, data, err := srv.buildPlayerEntitySnapshot()
	if err != nil {
		t.Fatalf("buildPlayerEntitySnapshot returned error: %v", err)
	}
	if amount != 1 {
		t.Fatalf("expected only player entity to be sent when unit type is invalid, got amount=%d", amount)
	}

	r := protocol.NewReaderWithContext(data, srv.TypeIO)
	entityID, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read entity id failed: %v", err)
	}
	if entityID != conn.playerID {
		t.Fatalf("expected player entity id %d, got %d", conn.playerID, entityID)
	}
	classID, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read class id failed: %v", err)
	}
	if classID != 12 {
		t.Fatalf("expected player entity class id 12, got %d", classID)
	}
	var player protocol.PlayerEntity
	if err := player.ReadSync(r); err != nil {
		t.Fatalf("read player sync failed: %v", err)
	}
	if player.Unit != nil {
		t.Fatalf("expected invalid unit reference to be cleared from player snapshot, got %T", player.Unit)
	}
	if r.Remaining() != 0 {
		t.Fatalf("expected exactly one snapshot entry, got %d trailing bytes", r.Remaining())
	}
}

func TestBuildEntitySnapshotPacketsSplitPlayerAndUnitAcrossPackets(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }

	conn := &Conn{
		playerID:     12,
		unitID:       1200,
		teamID:       1,
		hasConnected: true,
	}

	srv.mu.Lock()
	srv.conns[conn] = struct{}{}
	srv.mu.Unlock()

	srv.entityMu.Lock()
	srv.entities[conn.unitID] = &protocol.UnitEntitySync{
		IDValue:       conn.unitID,
		Controller:    &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: conn.playerID},
		Health:        100,
		TeamID:        1,
		TypeID:        35,
		Abilities:     []protocol.Ability{},
		Mounts:        []protocol.WeaponMount{},
		Plans:         []*protocol.BuildPlan{},
		Statuses:      []protocol.StatusEntry{},
		Stack:         protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
		Elevation:     1,
		Rotation:      90,
		Vel:           protocol.Vec2{},
		SpawnedByCore: true,
	}
	srv.entityMu.Unlock()

	unit := srv.snapshotPlayerUnitEntity(conn)
	if unit == nil {
		t.Fatal("expected snapshot unit entity")
	}
	player := &protocol.PlayerEntity{IDValue: conn.playerID}
	srv.updatePlayerEntity(player, conn)
	player.Unit = protocol.UnitBox{IDValue: unit.ID()}

	playerEntry, err := func() ([]byte, error) {
		w := protocol.NewWriterWithContext(srv.TypeIO)
		if err := w.WriteInt32(player.ID()); err != nil {
			return nil, err
		}
		if err := w.WriteByte(player.ClassID()); err != nil {
			return nil, err
		}
		if err := player.WriteSync(w); err != nil {
			return nil, err
		}
		return w.Bytes(), nil
	}()
	if err != nil {
		t.Fatalf("encode player entry: %v", err)
	}
	unitEntry, err := func() ([]byte, error) {
		w := protocol.NewWriterWithContext(srv.TypeIO)
		if err := w.WriteInt32(unit.ID()); err != nil {
			return nil, err
		}
		if err := w.WriteByte(unit.ClassID()); err != nil {
			return nil, err
		}
		if err := unit.WriteSync(w); err != nil {
			return nil, err
		}
		return w.Bytes(), nil
	}()
	if err != nil {
		t.Fatalf("encode unit entry: %v", err)
	}

	const maxEntitySnapshotData = 32000
	basePlayerSize := len(playerEntry)
	unitEntrySize := len(unitEntry)
	nameLen := maxEntitySnapshotData - unitEntrySize + 1 - basePlayerSize + len(player.Name)
	if nameLen <= 0 {
		t.Fatalf("expected positive name length, player=%d unit=%d", basePlayerSize, unitEntrySize)
	}
	conn.name = strings.Repeat("a", nameLen)

	packets, err := srv.buildEntitySnapshotPackets()
	if err != nil {
		t.Fatalf("buildEntitySnapshotPackets returned error: %v", err)
	}
	if len(packets) != 2 {
		t.Fatalf("expected player and unit to be split into two packets, got %d packets", len(packets))
	}

	first := decodeEntitySnapshotPacket(t, srv, packets[0])
	if len(first) != 1 || first[0].Unit == nil {
		t.Fatalf("expected first packet to contain exactly one unit entity, got %+v", first)
	}
	if first[0].ID != conn.unitID {
		t.Fatalf("expected unit entity id %d, got %d", conn.unitID, first[0].ID)
	}
	if first[0].Unit.TypeID != 35 {
		t.Fatalf("expected unit type 35, got %d", first[0].Unit.TypeID)
	}

	second := decodeEntitySnapshotPacket(t, srv, packets[1])
	if len(second) != 1 || second[0].Player == nil {
		t.Fatalf("expected second packet to contain exactly one player entity, got %+v", second)
	}
	if second[0].ID != conn.playerID {
		t.Fatalf("expected player entity id %d, got %d", conn.playerID, second[0].ID)
	}
	if second[0].Player.Unit == nil {
		t.Fatalf("expected player snapshot to retain unit reference when unit spills into next packet")
	}
	if second[0].Player.Unit.ID() != conn.unitID {
		t.Fatalf("expected player unit reference id %d, got %d", conn.unitID, second[0].Player.Unit.ID())
	}
}

func TestPrepareUnitEntitySnapshotAppliesOfficialGammaClassID(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 37, name: "gamma"})

	unit := &protocol.UnitEntitySync{
		IDValue:      9001,
		Controller:   &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: 7},
		Health:       220,
		TeamID:       1,
		TypeID:       37,
		Abilities:    []protocol.Ability{},
		Mounts:       []protocol.WeaponMount{},
		Plans:        []*protocol.BuildPlan{},
		Statuses:     []protocol.StatusEntry{},
		Stack:        protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
		Elevation:    1,
		Rotation:     90,
		BaseRotation: 90,
		Vel:          protocol.Vec2{},
	}

	if !srv.prepareUnitEntitySnapshot(unit) {
		t.Fatal("expected gamma unit snapshot to stay valid")
	}
	if got := unit.ClassID(); got != 31 {
		t.Fatalf("expected gamma unit class id 31, got %d", got)
	}
}

func TestPrepareUnitEntitySnapshotAppliesPayloadUnitLayout(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 55, name: "emanate"})

	unit := &protocol.UnitEntitySync{
		IDValue:        9101,
		Controller:     &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: 9},
		Health:         700,
		TeamID:         1,
		TypeID:         55,
		Abilities:      []protocol.Ability{},
		Mounts:         []protocol.WeaponMount{},
		Plans:          []*protocol.BuildPlan{},
		Payloads:       []protocol.Payload{},
		Statuses:       []protocol.StatusEntry{},
		Stack:          protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
		Elevation:      1,
		Rotation:       90,
		SpawnedByCore:  true,
		UpdateBuilding: false,
		Vel:            protocol.Vec2{},
		X:              64,
		Y:              96,
	}

	if !srv.prepareUnitEntitySnapshot(unit) {
		t.Fatal("expected payload core unit snapshot to stay valid")
	}
	if got := unit.ClassID(); got != 5 {
		t.Fatalf("expected emanate unit class id 5, got %d", got)
	}

	w := protocol.NewWriterWithContext(srv.TypeIO)
	if err := unit.WriteSync(w); err != nil {
		t.Fatalf("write payload unit sync failed: %v", err)
	}
	decoded := &protocol.UnitEntitySync{IDValue: unit.ID(), ClassIDValue: unit.ClassID(), ClassIDSet: true}
	if err := decoded.ReadSync(protocol.NewReaderWithContext(w.Bytes(), srv.TypeIO)); err != nil {
		t.Fatalf("read payload unit sync failed: %v", err)
	}
	if decoded.TypeID != 55 {
		t.Fatalf("expected decoded unit type 55, got %d", decoded.TypeID)
	}
	if len(decoded.Payloads) != 0 {
		t.Fatalf("expected empty payload seq, got %d entries", len(decoded.Payloads))
	}
}

func TestBuildEntitySnapshotPacketsIncludeExtraWorldUnits(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.ExtraEntitySnapshotEntitiesFn = func() ([]protocol.UnitSyncEntity, error) {
		return []protocol.UnitSyncEntity{
			&protocol.UnitEntitySync{
				IDValue:        9001,
				Controller:     &protocol.ControllerState{Type: protocol.ControllerGenericAI},
				Health:         175,
				TeamID:         2,
				TypeID:         35,
				Abilities:      []protocol.Ability{},
				Mounts:         []protocol.WeaponMount{},
				Plans:          []*protocol.BuildPlan{},
				Statuses:       []protocol.StatusEntry{},
				Stack:          protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
				Elevation:      1,
				Rotation:       45,
				Vel:            protocol.Vec2{X: 1, Y: 2},
				SpawnedByCore:  false,
				UpdateBuilding: false,
				X:              64,
				Y:              96,
			},
		}, nil
	}

	packets, err := srv.buildEntitySnapshotPackets()
	if err != nil {
		t.Fatalf("buildEntitySnapshotPackets returned error: %v", err)
	}
	if len(packets) != 1 {
		t.Fatalf("expected one packet for one extra entity, got %d", len(packets))
	}

	entries := decodeEntitySnapshotPacket(t, srv, packets[0])
	if len(entries) != 1 || entries[0].Unit == nil {
		t.Fatalf("expected packet to contain one extra unit entry, got %+v", entries)
	}
	if entries[0].ID != 9001 {
		t.Fatalf("expected unit id 9001, got %d", entries[0].ID)
	}
	if entries[0].Unit.TypeID != 35 {
		t.Fatalf("expected extra unit type 35, got %d", entries[0].Unit.TypeID)
	}
	if entries[0].Unit.TeamID != 2 {
		t.Fatalf("expected extra unit team 2, got %d", entries[0].Unit.TeamID)
	}
}

func TestBuildEntitySnapshotPacketsForConnKeepsOtherPlayersDuringWorldReloadGrace(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})

	graceConn := &Conn{
		playerID:     41,
		unitID:       4100,
		teamID:       1,
		hasConnected: true,
	}
	graceConn.SetWorldReloadGrace(3 * time.Second)
	activeConn := &Conn{
		playerID:     42,
		unitID:       4200,
		teamID:       1,
		hasConnected: true,
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		switch unitID {
		case graceConn.unitID:
			return UnitInfo{ID: unitID, X: 16, Y: 24, Health: 100, TeamID: 1, TypeID: 35}, true
		case activeConn.unitID:
			return UnitInfo{ID: unitID, X: 32, Y: 48, Health: 100, TeamID: 1, TypeID: 35}, true
		default:
			return UnitInfo{}, false
		}
	}

	srv.mu.Lock()
	srv.conns[graceConn] = struct{}{}
	srv.conns[activeConn] = struct{}{}
	srv.mu.Unlock()

	packets, hiddenIDs, err := srv.buildEntitySnapshotPacketsForConn(activeConn)
	if err != nil {
		t.Fatalf("buildEntitySnapshotPacketsForConn returned error: %v", err)
	}
	if len(packets) == 0 {
		t.Fatal("expected packets for both players")
	}
	if len(hiddenIDs) != 0 {
		t.Fatalf("expected no hidden ids, got %v", hiddenIDs)
	}

	seen := map[int32]bool{}
	for _, packet := range packets {
		for _, entry := range decodeEntitySnapshotPacket(t, srv, packet) {
			seen[entry.ID] = true
		}
	}
	if !seen[graceConn.playerID] || !seen[graceConn.unitID] {
		t.Fatalf("expected world-reload-grace player/unit to remain visible to other viewers, seen=%v", seen)
	}
	if !seen[activeConn.playerID] || !seen[activeConn.unitID] {
		t.Fatalf("expected active player/unit to remain in snapshot, seen=%v", seen)
	}
}

func TestBroadcastSkipsPeersInWorldReloadGrace(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	activeServer, activeClient := net.Pipe()
	defer activeClient.Close()
	activeConn := NewConn(activeServer, srv.Serial)
	defer activeConn.Close()
	activeConn.playerID = 4201
	activeConn.hasConnected = true

	graceServer, graceClient := net.Pipe()
	defer graceClient.Close()
	graceConn := NewConn(graceServer, srv.Serial)
	defer graceConn.Close()
	graceConn.playerID = 4202
	graceConn.hasConnected = true
	graceConn.SetWorldReloadGrace(2 * time.Second)

	srv.mu.Lock()
	srv.conns[activeConn] = struct{}{}
	srv.conns[graceConn] = struct{}{}
	srv.mu.Unlock()

	var activeSends atomic.Int32
	var graceSends atomic.Int32
	activeConn.onSend = func(obj any, _ int, _ int, _ int) {
		if _, ok := obj.(*protocol.Remote_NetClient_setPosition_29); ok {
			activeSends.Add(1)
		}
	}
	graceConn.onSend = func(obj any, _ int, _ int, _ int) {
		if _, ok := obj.(*protocol.Remote_NetClient_setPosition_29); ok {
			graceSends.Add(1)
		}
	}

	activeDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, activeClient)
		close(activeDone)
	}()
	graceDone := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, graceClient)
		close(graceDone)
	}()

	srv.Broadcast(&protocol.Remote_NetClient_setPosition_29{X: 12, Y: 18})
	time.Sleep(60 * time.Millisecond)

	if activeSends.Load() != 1 {
		t.Fatalf("expected active peer to receive one broadcast packet, got %d", activeSends.Load())
	}
	if graceSends.Load() != 0 {
		t.Fatalf("expected world-reload-grace peer to be skipped, got %d packets", graceSends.Load())
	}

	_ = activeConn.Close()
	_ = graceConn.Close()
	<-activeDone
	<-graceDone
}

func TestBroadcastUnreliableSkipsBlockSnapshotsWithoutUDPAddr(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	udpConn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0})
	if err != nil {
		t.Fatalf("listen udp: %v", err)
	}
	defer udpConn.Close()
	srv.udpConn = udpConn

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 4301
	conn.hasConnected = true

	srv.mu.Lock()
	srv.conns[conn] = struct{}{}
	srv.mu.Unlock()

	var sends atomic.Int32
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		if _, ok := obj.(*protocol.Remote_NetClient_blockSnapshot_34); ok {
			sends.Add(1)
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.BroadcastUnreliable(&protocol.Remote_NetClient_blockSnapshot_34{
		Amount: 1,
		Data:   []byte{1, 2, 3},
	})
	time.Sleep(60 * time.Millisecond)

	if sends.Load() != 0 {
		t.Fatalf("expected blockSnapshot unreliable broadcast to skip tcp fallback peers without udp addr, got %d sends", sends.Load())
	}

	_ = conn.Close()
	<-done
}

func TestPostConnectLoopSkipsEntitySnapshotsDuringWorldReloadGrace(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.SetSnapshotIntervals(20, 20)

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 51
	conn.unitID = 5100
	conn.teamID = 1
	conn.hasConnected = true
	conn.SetWorldReloadGrace(200 * time.Millisecond)

	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:     unitID,
			X:      64,
			Y:      96,
			Health: 100,
			TeamID: 1,
			TypeID: 35,
		}, true
	}

	srv.mu.Lock()
	srv.conns[conn] = struct{}{}
	srv.mu.Unlock()

	var entitySent atomic.Int32
	var stateSent atomic.Int32
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		switch obj.(type) {
		case *protocol.Remote_NetClient_entitySnapshot_32:
			entitySent.Add(1)
		case *protocol.Remote_NetClient_stateSnapshot_35:
			stateSent.Add(1)
		}
	}

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	go srv.postConnectLoop(conn)
	time.Sleep(90 * time.Millisecond)
	_ = conn.Close()
	<-done

	if stateSent.Load() == 0 {
		t.Fatal("expected postConnectLoop to keep sending state snapshots during grace")
	}
	if entitySent.Load() != 0 {
		t.Fatalf("expected no entity snapshots during world reload grace, got %d", entitySent.Load())
	}
}

func TestOfficialConnectConfirmPostConnectSetsInitialWorldReloadGraceAndRunsOnce(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	srv.SetSnapshotIntervals(250, 250)
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		return protocol.Point2{X: 8, Y: 9}, true
	}

	var spawnCalls atomic.Int32
	spawnDone := make(chan struct{}, 2)
	srv.SpawnUnitFn = func(_ *Conn, _ int32, _ protocol.Point2, _ int16) (float32, float32, bool) {
		spawnCalls.Add(1)
		select {
		case spawnDone <- struct{}{}:
		default:
		}
		return 64, 72, true
	}

	var postCalls atomic.Int32
	postDone := make(chan struct{}, 1)
	srv.OnPostConnect = func(_ *Conn) {
		postCalls.Add(1)
		select {
		case postDone <- struct{}{}:
		default:
		}
	}
	var hotReloadCalls atomic.Int32
	srv.OnHotReloadConnFn = func(_ *Conn) {
		hotReloadCalls.Add(1)
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	conn := NewConn(serverSide, srv.Serial)
	conn.playerID = 61
	conn.teamID = 1
	conn.hasBegunConnecting = true

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.handleOfficialConnectConfirm(conn, &protocol.Remote_NetServer_connectConfirm_50{})
	select {
	case <-postDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected OnPostConnect to run")
	}
	if !conn.InWorldReloadGrace() {
		t.Fatal("expected initial world reload grace after post-connect start")
	}
	select {
	case <-spawnDone:
	case <-time.After(time.Second):
		t.Fatal("expected delayed initial spawn to run")
	}
	if spawnCalls.Load() != 1 {
		t.Fatalf("expected one initial spawn, got %d", spawnCalls.Load())
	}

	srv.handleOfficialConnectConfirm(conn, &protocol.Remote_NetServer_connectConfirm_50{})
	time.Sleep(30 * time.Millisecond)
	if spawnCalls.Load() != 1 {
		t.Fatalf("expected post-connect init to run once, got %d spawns", spawnCalls.Load())
	}
	if postCalls.Load() != 1 {
		t.Fatalf("expected OnPostConnect to run once, got %d", postCalls.Load())
	}
	if hotReloadCalls.Load() != 0 {
		t.Fatalf("expected duplicate official confirm without queued reload not to run hot reload hook, got %d", hotReloadCalls.Load())
	}

	_ = conn.Close()
	<-done
}

func TestDebugClientSnapshotFallbackPostConnectRunsOnceAndOfficialConfirmDoesNotRestart(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.SetClientSnapshotConnectFallbackEnabled(true)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	srv.SetSnapshotIntervals(250, 250)
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		return protocol.Point2{X: 8, Y: 9}, true
	}

	var spawnCalls atomic.Int32
	spawnDone := make(chan struct{}, 2)
	srv.SpawnUnitFn = func(_ *Conn, _ int32, _ protocol.Point2, _ int16) (float32, float32, bool) {
		spawnCalls.Add(1)
		select {
		case spawnDone <- struct{}{}:
		default:
		}
		return 64, 72, true
	}

	var postCalls atomic.Int32
	postDone := make(chan struct{}, 1)
	srv.OnPostConnect = func(_ *Conn) {
		postCalls.Add(1)
		select {
		case postDone <- struct{}{}:
		default:
		}
	}

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()
	conn := NewConn(serverSide, srv.Serial)
	conn.playerID = 62
	conn.teamID = 1
	conn.hasBegunConnecting = true

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.handleClientSnapshotConnectFallback(conn, &protocol.Remote_NetServer_clientSnapshot_48{})
	select {
	case <-postDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected OnPostConnect to run from clientSnapshot fallback")
	}
	if !conn.hasConnected {
		t.Fatal("expected clientSnapshot fallback to mark connection connected")
	}
	if !conn.InWorldReloadGrace() {
		t.Fatal("expected world reload grace after fallback post-connect start")
	}
	select {
	case <-spawnDone:
	case <-time.After(time.Second):
		t.Fatal("expected delayed initial spawn from fallback chain")
	}
	if spawnCalls.Load() != 1 {
		t.Fatalf("expected one initial spawn from fallback chain, got %d", spawnCalls.Load())
	}

	srv.handleOfficialConnectConfirm(conn, &protocol.Remote_NetServer_connectConfirm_50{})
	time.Sleep(30 * time.Millisecond)
	if spawnCalls.Load() != 1 {
		t.Fatalf("expected official confirm not to restart post-connect, got %d spawns", spawnCalls.Load())
	}
	if postCalls.Load() != 1 {
		t.Fatalf("expected OnPostConnect to run once across fallback+official confirm, got %d", postCalls.Load())
	}

	_ = conn.Close()
	<-done
}

func TestSerializeEntityIncludesIDClassAndSyncBytes(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 37, name: "gamma"})

	unit := &protocol.UnitEntitySync{
		IDValue:      9001,
		Controller:   &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: 7},
		Health:       220,
		TeamID:       1,
		TypeID:       37,
		Abilities:    []protocol.Ability{},
		Mounts:       []protocol.WeaponMount{},
		Plans:        []*protocol.BuildPlan{},
		Statuses:     []protocol.StatusEntry{},
		Stack:        protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
		Elevation:    1,
		Rotation:     90,
		BaseRotation: 90,
		Vel:          protocol.Vec2{},
		X:            64,
		Y:            96,
	}
	if !srv.prepareUnitEntitySnapshot(unit) {
		t.Fatal("expected gamma unit snapshot to stay valid")
	}

	got := srv.serializeEntity(unit)

	wantWriter := protocol.NewWriterWithContext(srv.TypeIO)
	if err := wantWriter.WriteInt32(unit.ID()); err != nil {
		t.Fatalf("write expected entity id failed: %v", err)
	}
	if err := wantWriter.WriteByte(unit.ClassID()); err != nil {
		t.Fatalf("write expected entity class failed: %v", err)
	}
	if err := unit.WriteSync(wantWriter); err != nil {
		t.Fatalf("write expected entity sync failed: %v", err)
	}
	want := append([]byte(nil), wantWriter.Bytes()...)

	if !bytes.Equal(got, want) {
		t.Fatalf("serializeEntity mismatch:\n got=%x\nwant=%x", got, want)
	}

	entries := decodeEntitySnapshotPacket(t, srv, &protocol.Remote_NetClient_entitySnapshot_32{
		Amount: 1,
		Data:   got,
	})
	if len(entries) != 1 || entries[0].Unit == nil {
		t.Fatalf("expected one decoded unit entry, got %+v", entries)
	}
	if entries[0].ID != 9001 {
		t.Fatalf("expected unit id 9001, got %d", entries[0].ID)
	}
	if entries[0].ClassID != unit.ClassID() {
		t.Fatalf("expected class id %d, got %d", unit.ClassID(), entries[0].ClassID)
	}
	if entries[0].Unit.TypeID != 37 {
		t.Fatalf("expected decoded unit type 37, got %d", entries[0].Unit.TypeID)
	}
}

func TestSnapshotPlayerUnitEntityDoesNotCreatePhantomUnitWhenWorldUnitMissing(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	conn := &Conn{
		playerID:     42,
		unitID:       4242,
		teamID:       1,
		hasConnected: true,
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		return UnitInfo{}, false
	}

	if unit := srv.snapshotPlayerUnitEntity(conn); unit != nil {
		t.Fatalf("expected missing world unit to skip snapshot, got %+v", unit)
	}

	srv.entityMu.Lock()
	_, ok := srv.entities[conn.unitID]
	srv.entityMu.Unlock()
	if ok {
		t.Fatalf("expected snapshot path not to create phantom mirror entity for missing world unit")
	}
}

func TestSnapshotPlayerUnitEntityCreatesMirrorOnlyForAliveWorldUnit(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 36, name: "beta"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	conn := &Conn{
		playerID:     43,
		unitID:       4300,
		teamID:       1,
		hasConnected: true,
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:     unitID,
			X:      40,
			Y:      72,
			Health: 170,
			TeamID: 1,
			TypeID: 36,
		}, true
	}

	unit := srv.snapshotPlayerUnitEntity(conn)
	if unit == nil {
		t.Fatal("expected alive world unit to be snapshotted")
	}
	if unit.TypeID != 36 {
		t.Fatalf("expected world unit type 36, got %d", unit.TypeID)
	}
	if unit.X != 40 || unit.Y != 72 {
		t.Fatalf("expected world position (40,72), got (%.1f,%.1f)", unit.X, unit.Y)
	}

	srv.entityMu.Lock()
	_, ok := srv.entities[conn.unitID]
	srv.entityMu.Unlock()
	if !ok {
		t.Fatalf("expected snapshot path to cache mirror entity for alive world unit")
	}
}

func TestSnapshotPlayerUnitEntityUsesAuthoritativeUnitSync(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.Content.RegisterItem(protocol.ItemRef{ItmID: 5, ItmName: "copper"})
	srv.Content.RegisterBlock(protocol.BlockRef{BlkID: 12, BlkName: "conveyor"})

	conn := &Conn{
		playerID:     44,
		unitID:       4400,
		teamID:       1,
		hasConnected: true,
		snapX:        64,
		snapY:        96,
	}
	srv.UnitSyncFn = func(unitID int32, controller protocol.UnitController) (*protocol.UnitEntitySync, bool) {
		if unitID != conn.unitID {
			return nil, false
		}
		ctrl, ok := controller.(*protocol.ControllerState)
		if !ok || ctrl.Type != protocol.ControllerPlayer || ctrl.PlayerID != conn.playerID {
			t.Fatalf("unexpected controller passed to UnitSyncFn: %+v", controller)
		}
		return &protocol.UnitEntitySync{
			IDValue:        unitID,
			Controller:     controller,
			Health:         170,
			Ammo:           4,
			MineTile:       protocol.TileBox{PosValue: protocol.PackPoint2(2, 3)},
			Plans:          []*protocol.BuildPlan{{X: 5, Y: 6, Rotation: 1, Block: protocol.BlockRef{BlkID: 12, BlkName: "conveyor"}}},
			Stack:          protocol.ItemStack{Item: protocol.ItemRef{ItmID: 5, ItmName: "copper"}, Amount: 9},
			SpawnedByCore:  true,
			Statuses:       []protocol.StatusEntry{},
			Abilities:      []protocol.Ability{},
			Mounts:         []protocol.WeaponMount{},
			TeamID:         1,
			TypeID:         35,
			Elevation:      1,
			Rotation:       33,
			BaseRotation:   33,
			UpdateBuilding: true,
			Vel:            protocol.Vec2{X: 1, Y: -1},
			X:              80,
			Y:              120,
		}, true
	}

	unit := srv.snapshotPlayerUnitEntity(conn)
	if unit == nil {
		t.Fatal("expected authoritative player unit snapshot")
	}
	if unit.TypeID != 35 || unit.Health != 170 || unit.Ammo != 4 {
		t.Fatalf("unexpected unit core fields %+v", unit)
	}
	if unit.MineTile == nil || unit.MineTile.Pos() != protocol.PackPoint2(2, 3) {
		t.Fatalf("unexpected mine tile %+v", unit.MineTile)
	}
	if !unit.SpawnedByCore || !unit.UpdateBuilding {
		t.Fatalf("expected spawnedByCore/updateBuilding to be preserved, got %+v", unit)
	}
	if unit.Stack.Item == nil || unit.Stack.Item.ID() != 5 || unit.Stack.Amount != 9 {
		t.Fatalf("unexpected stack %+v", unit.Stack)
	}
	if len(unit.Plans) != 1 || unit.Plans[0] == nil || unit.Plans[0].Block == nil || unit.Plans[0].Block.ID() != 12 {
		t.Fatalf("unexpected plans %+v", unit.Plans)
	}
	srv.entityMu.Lock()
	cached, ok := srv.entities[conn.unitID].(*protocol.UnitEntitySync)
	srv.entityMu.Unlock()
	if !ok || cached == nil {
		t.Fatal("expected authoritative snapshot to be cached as mirror entity")
	}
	if cached.TypeID != 35 || cached.Health != 170 {
		t.Fatalf("unexpected cached unit %+v", cached)
	}
}

func TestSyncUnitFromWorldKeepsPlayerUnitTypeValid(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		return UnitInfo{
			ID:     unitID,
			X:      80,
			Y:      96,
			Health: 220,
			TeamID: 1,
			TypeID: 999,
		}, true
	}

	unit := &protocol.UnitEntitySync{
		IDValue:    123,
		Controller: &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: 5},
		TypeID:     999,
		Health:     1,
		TeamID:     1,
		Abilities:  []protocol.Ability{},
		Mounts:     []protocol.WeaponMount{},
		Plans:      []*protocol.BuildPlan{},
		Statuses:   []protocol.StatusEntry{},
		Stack:      protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
	}

	srv.syncUnitFromWorld(unit)

	if unit.TypeID != 35 {
		t.Fatalf("expected invalid world type to fall back to 35, got %d", unit.TypeID)
	}
	if unit.X != 80 || unit.Y != 96 {
		t.Fatalf("expected world position to sync, got (%.1f, %.1f)", unit.X, unit.Y)
	}
	if unit.Health != 220 {
		t.Fatalf("expected world health to sync, got %.1f", unit.Health)
	}
}

func TestPlayerRespawnUnitTypeUsesResolveHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.Content.RegisterUnitType(testUnitType{id: 55, name: "emanate"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	srv.ResolveRespawnUnitTypeFn = func(c *Conn, tile protocol.Point2, fallback int16) int16 {
		if tile.X != 9 || tile.Y != 11 {
			t.Fatalf("unexpected spawn tile %+v", tile)
		}
		if fallback != 35 {
			t.Fatalf("expected fallback 35, got %d", fallback)
		}
		return 55
	}

	got := srv.playerRespawnUnitType(&Conn{}, protocol.Point2{X: 9, Y: 11})
	if got != 55 {
		t.Fatalf("expected resolve hook to override respawn type to 55, got %d", got)
	}
}

func TestTryDockedUnitClearRespawnSpawnsAttachedCoreUnit(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 53, name: "evoke"})
	srv.ResolveRespawnUnitTypeFn = func(c *Conn, tile protocol.Point2, fallback int16) int16 { return 53 }
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		return protocol.Point2{X: 4, Y: 6}, true
	}

	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	conn.playerID = 7
	conn.unitID = 100
	conn.teamID = 1
	conn.snapX = 64
	conn.snapY = 96

	srv.entityMu.Lock()
	srv.entities[conn.unitID] = &protocol.UnitEntitySync{
		IDValue:       conn.unitID,
		Controller:    &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: conn.playerID},
		TypeID:        37,
		TeamID:        1,
		X:             80,
		Y:             120,
		Rotation:      33,
		SpawnedByCore: false,
		Abilities:     []protocol.Ability{},
		Mounts:        []protocol.WeaponMount{},
		Plans:         []*protocol.BuildPlan{},
		Statuses:      []protocol.StatusEntry{},
		Stack:         protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
	}
	srv.entityMu.Unlock()

	var called bool
	var gotUnitID int32
	var gotType int16
	var gotSpawnedByCore bool
	var gotRotation float32
	srv.SpawnUnitAtFn = func(c *Conn, unitID int32, x, y, rotation float32, unitType int16, spawnedByCore bool) (float32, float32, bool) {
		called = true
		gotUnitID = unitID
		gotType = unitType
		gotSpawnedByCore = spawnedByCore
		gotRotation = rotation
		return x, y, true
	}

	if !srv.tryDockedUnitClearRespawn(conn, "unitClear-91") {
		t.Fatal("expected docked unit clear respawn to succeed")
	}
	if !called {
		t.Fatal("expected SpawnUnitAtFn to be called")
	}
	if gotUnitID == 100 || conn.unitID == 100 {
		t.Fatalf("expected a fresh unit id, got old=%d new=%d", gotUnitID, conn.unitID)
	}
	if gotType != 53 {
		t.Fatalf("expected dock respawn type 53, got %d", gotType)
	}
	if !gotSpawnedByCore {
		t.Fatal("expected dock respawn to mark unit as spawnedByCore")
	}
	if gotRotation != 33 {
		t.Fatalf("expected rotation 33 to carry into respawn, got %.1f", gotRotation)
	}
	if conn.dead {
		t.Fatal("expected player to remain alive after dock respawn")
	}
	if conn.snapX != 80 || conn.snapY != 120 {
		t.Fatalf("expected dock respawn to use current unit position, got (%.1f, %.1f)", conn.snapX, conn.snapY)
	}
}
