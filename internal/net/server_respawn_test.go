package net

import (
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

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
