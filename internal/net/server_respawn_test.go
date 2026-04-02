package net

import (
	"net"
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

func TestMaybeRespawnKeepsAliveWorldUnitWhenMirrorMissing(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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

func TestMaybeRespawnRevivesDeadFlagWhenWorldUnitStillAlive(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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

func TestRequestRespawnIgnoresUnitClearWhileWorldUnitAlive(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
	conn := &Conn{
		playerID: 11,
		unitID:   777,
		teamID:   1,
	}
	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		if unitID != conn.unitID {
			return UnitInfo{}, false
		}
		return UnitInfo{
			ID:     unitID,
			Health: 150,
			TeamID: 1,
		}, true
	}

	srv.requestRespawn(conn, "unitClear-91")

	if conn.dead {
		t.Fatalf("expected alive connection after unitClear while world unit alive")
	}
	if conn.unitID != 777 {
		t.Fatalf("expected unit id unchanged, got %d", conn.unitID)
	}
}

func TestMaybeRespawnDoesNotSelfKillWhenStateTemporarilyMissing(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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

func TestEnsurePlayerUnitEntityPrefersWorldType(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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
	srv := NewServer("127.0.0.1:0", 156)
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

func TestShouldForceRespawnAfterDeadIgnoredResetsAfterGap(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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

func TestEnsurePlayerUnitEntitySkipsCollidingPlayerIDs(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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

func TestNextUnitIDSkipsExistingEntityIDs(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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
	srv := NewServer("127.0.0.1:0", 156)
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
	srv := NewServer("127.0.0.1:0", 156)
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

func TestSyncUnitFromWorldKeepsPlayerUnitTypeValid(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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
	srv := NewServer("127.0.0.1:0", 156)
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
	srv := NewServer("127.0.0.1:0", 156)
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
