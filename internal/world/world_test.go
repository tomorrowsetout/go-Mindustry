package world

import (
	"testing"
	"time"
)

func TestWorldSnapshot(t *testing.T) {
	w := New(Config{TPS: 60})
	before := w.Snapshot()
	w.Step(500 * time.Millisecond)
	after := w.Snapshot()
	if after.WaveTime <= before.WaveTime {
		t.Fatalf("expected wavetime to increase, before=%v after=%v", before.WaveTime, after.WaveTime)
	}
	if after.Tps != 60 {
		t.Fatalf("expected tps=60, got=%d", after.Tps)
	}
}

func TestApplyBuildPlansIsAsync(t *testing.T) {
	w := New(Config{TPS: 60})
	w.SetModel(NewWorldModel(8, 8))

	ops := []BuildPlanOp{{
		Breaking: false,
		X:        2,
		Y:        3,
		Rotation: 1,
		BlockID:  45,
	}}
	w.ApplyBuildPlans(TeamID(1), ops)

	tile, err := w.Model().TileAt(2, 3)
	if err != nil || tile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	if tile.Block != 45 || tile.Build == nil {
		t.Fatalf("expected immediate preview placement, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if tile.Build.Health <= 0 || tile.Build.Health >= 1000 {
		t.Fatalf("expected low initial build health, got=%.2f", tile.Build.Health)
	}

	w.Step(300 * time.Millisecond)
	tile, _ = w.Model().TileAt(2, 3)
	if tile.Block != 45 || tile.Build == nil {
		t.Fatalf("expected pending build at 0.3s, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if tile.Build.Health <= 1 || tile.Build.Health >= 1000 {
		t.Fatalf("expected intermediate build health at 0.3s, got=%.2f", tile.Build.Health)
	}

	w.Step(600 * time.Millisecond)
	tile, _ = w.Model().TileAt(2, 3)
	if tile.Block != 45 || tile.Build == nil {
		t.Fatalf("expected placed block after progress, got block=%d build=%v", tile.Block, tile.Build != nil)
	}
	if tile.Build.Health < 999 {
		t.Fatalf("expected completed build health, got=%.2f", tile.Build.Health)
	}
}
