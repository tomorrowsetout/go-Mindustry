package world

import "testing"

func placeUnitAITargetCacheBuilding(t *testing.T, w *World, x, y int, block int16, team TeamID) int32 {
	t.Helper()
	tile, err := w.Model().TileAt(x, y)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed at (%d,%d): %v", x, y, err)
	}
	tile.Block = BlockID(block)
	tile.Team = team
	tile.Build = &Building{
		Block:     BlockID(block),
		Team:      team,
		X:         x,
		Y:         y,
		Health:    1000,
		MaxHealth: 1000,
	}
	w.rebuildBlockOccupancyLocked()
	return int32(y*w.Model().Width + x)
}

func newUnitAITargetCacheWorld() *World {
	w := New(Config{TPS: 120})
	model := NewWorldModel(48, 16)
	model.BlockNames = map[int16]string{1: "duo"}
	w.SetModel(model)
	return w
}

func TestAcquireUnitAITargetCachesBuildingTargetBetweenRescans(t *testing.T) {
	w := newUnitAITargetCacheWorld()
	far := placeUnitAITargetCacheBuilding(t, w, 40, 8, 1, 2)
	src := RawEntity{
		ID:                 1,
		Team:               1,
		X:                  4*8 + 4,
		Y:                  8*8 + 4,
		Health:             100,
		AttackRange:        32,
		AttackTargetGround: true,
	}

	target, ok := w.acquireUnitAITargetLocked(src, unitAIFlying, 1.0/120.0, nil, nil)
	if !ok || !target.HasBuild || target.BuildPos != far {
		t.Fatalf("expected initial far building target, got target=%+v ok=%v", target, ok)
	}

	close := placeUnitAITargetCacheBuilding(t, w, 6, 8, 1, 2)
	target, ok = w.acquireUnitAITargetLocked(src, unitAIFlying, 1.0/120.0, nil, nil)
	if !ok || target.BuildPos != far {
		t.Fatalf("expected cached target before rescan, got target=%+v ok=%v", target, ok)
	}

	target, ok = w.acquireUnitAITargetLocked(src, unitAIFlying, autonomousAITargetRescanSec+0.01, nil, nil)
	if !ok || target.BuildPos != close {
		t.Fatalf("expected closer target after rescan, got target=%+v ok=%v", target, ok)
	}
}

func TestAcquireUnitAITargetDropsDestroyedCachedBuilding(t *testing.T) {
	w := newUnitAITargetCacheWorld()
	pos := placeUnitAITargetCacheBuilding(t, w, 40, 8, 1, 2)
	src := RawEntity{
		ID:                 1,
		Team:               1,
		X:                  4*8 + 4,
		Y:                  8*8 + 4,
		Health:             100,
		AttackRange:        32,
		AttackTargetGround: true,
	}

	if target, ok := w.acquireUnitAITargetLocked(src, unitAIFlying, 1.0/120.0, nil, nil); !ok || target.BuildPos != pos {
		t.Fatalf("expected initial target, got target=%+v ok=%v", target, ok)
	}
	tile := &w.Model().Tiles[pos]
	tile.Block = 0
	tile.Build = nil
	w.rebuildBlockOccupancyLocked()

	target, ok := w.acquireUnitAITargetLocked(src, unitAIFlying, 1.0/120.0, nil, nil)
	if ok {
		t.Fatalf("expected destroyed cached target to be dropped, got target=%+v", target)
	}
	if state := w.unitAIStates[src.ID]; state.CachedTargetValid {
		t.Fatalf("expected cache to be invalidated, got state=%+v", state)
	}
}
