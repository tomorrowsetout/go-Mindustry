package world

import (
	"math"
	"testing"
)

func TestWaveFlyerSpawnUsesArcTrigTable(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(500, 500)
	pos := 268*model.Width + 462
	model.Tiles[pos].Overlay = 1
	w.SetModel(model)

	spawns := w.waveFlyerSpawnPositionsLocked(-1, nil)
	if len(spawns) != 1 {
		t.Fatalf("expected one flyer spawn, got %d", len(spawns))
	}
	if math.Abs(float64(spawns[0].X-4000)) > 0.0001 {
		t.Fatalf("expected flyer spawn x to clamp to 4000, got %f", spawns[0].X)
	}
	if math.Abs(float64(spawns[0].Y-2477.7776)) > 0.0001 {
		t.Fatalf("expected Arc-table flyer spawn y, got %f", spawns[0].Y)
	}
}

func TestVanillaFlyingWobbleUsesTimeAndUnitID(t *testing.T) {
	w := New(Config{TPS: 60})
	w.timeSec = 0.016
	ent := RawEntity{ID: 1197528, Flying: true, Health: 1}

	if !w.applyVanillaFlyingWobbleLocked(&ent, 0.96) {
		t.Fatal("expected wobble to move flying entity")
	}
	if math.Abs(float64(ent.X-(-0.032255))) > 0.00001 {
		t.Fatalf("unexpected wobble x offset %f", ent.X)
	}
	if math.Abs(float64(ent.Y-(-0.035547))) > 0.00001 {
		t.Fatalf("unexpected wobble y offset %f", ent.Y)
	}
}
