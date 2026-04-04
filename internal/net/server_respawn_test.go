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

func TestConnectedTeamCountsSkipsUnassignedConns(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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

func TestBuildEntitySnapshotPacketsSplitPlayerAndUnitAcrossPackets(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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
	if len(first) != 1 || first[0].Player == nil {
		t.Fatalf("expected first packet to contain exactly one player entity, got %+v", first)
	}
	if first[0].ID != conn.playerID {
		t.Fatalf("expected player entity id %d, got %d", conn.playerID, first[0].ID)
	}
	if first[0].Player.Unit == nil {
		t.Fatalf("expected player snapshot to retain unit reference when unit spills into next packet")
	}
	if first[0].Player.Unit.ID() != conn.unitID {
		t.Fatalf("expected player unit reference id %d, got %d", conn.unitID, first[0].Player.Unit.ID())
	}

	second := decodeEntitySnapshotPacket(t, srv, packets[1])
	if len(second) != 1 || second[0].Unit == nil {
		t.Fatalf("expected second packet to contain exactly one unit entity, got %+v", second)
	}
	if second[0].ID != conn.unitID {
		t.Fatalf("expected unit entity id %d, got %d", conn.unitID, second[0].ID)
	}
	if second[0].Unit.TypeID != 35 {
		t.Fatalf("expected unit type 35, got %d", second[0].Unit.TypeID)
	}
}

func TestPrepareUnitEntitySnapshotAppliesOfficialGammaClassID(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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
	srv := NewServer("127.0.0.1:0", 156)
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
	srv := NewServer("127.0.0.1:0", 156)
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

func TestBuildEntitySnapshotPacketsSkipPlayersInWorldReloadGrace(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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

	packets, err := srv.buildEntitySnapshotPackets()
	if err != nil {
		t.Fatalf("buildEntitySnapshotPackets returned error: %v", err)
	}
	if len(packets) == 0 {
		t.Fatal("expected packets for active player")
	}

	seen := map[int32]bool{}
	for _, packet := range packets {
		for _, entry := range decodeEntitySnapshotPacket(t, srv, packet) {
			seen[entry.ID] = true
		}
	}
	if seen[graceConn.playerID] || seen[graceConn.unitID] {
		t.Fatalf("expected world-reload-grace player/unit to be skipped, seen=%v", seen)
	}
	if !seen[activeConn.playerID] || !seen[activeConn.unitID] {
		t.Fatalf("expected active player/unit to remain in snapshot, seen=%v", seen)
	}
}

func TestPostConnectLoopSkipsEntitySnapshotsDuringWorldReloadGrace(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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

func TestEnsurePostConnectStartedSetsInitialWorldReloadGraceAndRunsOnce(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }
	srv.SetSnapshotIntervals(250, 250)
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		return protocol.Point2{X: 8, Y: 9}, true
	}

	var spawnCalls atomic.Int32
	srv.SpawnUnitFn = func(_ *Conn, _ int32, _ protocol.Point2, _ int16) (float32, float32, bool) {
		spawnCalls.Add(1)
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
	conn.playerID = 61
	conn.teamID = 1
	conn.hasConnected = true

	done := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
		close(done)
	}()

	srv.ensurePostConnectStarted(conn)
	select {
	case <-postDone:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("expected OnPostConnect to run")
	}
	if !conn.InWorldReloadGrace() {
		t.Fatal("expected initial world reload grace after post-connect start")
	}
	if spawnCalls.Load() != 1 {
		t.Fatalf("expected one initial spawn, got %d", spawnCalls.Load())
	}

	srv.ensurePostConnectStarted(conn)
	time.Sleep(30 * time.Millisecond)
	if spawnCalls.Load() != 1 {
		t.Fatalf("expected post-connect init to run once, got %d spawns", spawnCalls.Load())
	}
	if postCalls.Load() != 1 {
		t.Fatalf("expected OnPostConnect to run once, got %d", postCalls.Load())
	}

	_ = conn.Close()
	<-done
}

func TestSerializeEntityIncludesIDClassAndSyncBytes(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 156)
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
	srv := NewServer("127.0.0.1:0", 156)
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
	srv := NewServer("127.0.0.1:0", 156)
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
