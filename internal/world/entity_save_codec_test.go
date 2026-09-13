package world

import (
	"testing"
	"time"
)

func TestLoadedEntityHealthSurvivesRuntimeInit(t *testing.T) {
	w := New(Config{})
	model := NewWorldModel(16, 16)
	model.UnitNames = map[int16]string{35: "alpha"}
	model.Entities = append(model.Entities, RawEntity{
		ID:          1,
		TypeID:      35,
		Team:        1,
		X:           32,
		Y:           32,
		Health:      42,
		MaxHealth:   220,
		MineTilePos: invalidEntityTilePos,
	})
	w.SetModel(model)

	w.Step(time.Second / 60)

	got, ok := w.GetEntity(1)
	if !ok {
		t.Fatal("expected loaded entity to remain in world")
	}
	if got.Health != 42 {
		t.Fatalf("expected runtime init to preserve saved health=42, got %f", got.Health)
	}
	if got.MaxHealth != 220 {
		t.Fatalf("expected runtime init to preserve saved maxHealth=220, got %f", got.MaxHealth)
	}
}

func TestSaveCodecPreservesNegativeShield(t *testing.T) {
	raw := RawEntity{
		ID:        7,
		TypeID:    35,
		Team:      1,
		X:         32,
		Y:         48,
		Health:    90,
		MaxHealth: 100,
		Shield:    -3.5,
		SlowMul:   1,
	}

	unit := UnitEntitySyncFromRawEntitySave(raw, "alpha")
	if unit == nil {
		t.Fatal("expected save unit entity")
	}
	if unit.Shield != -3.5 {
		t.Fatalf("expected save codec to keep negative shield, got=%f", unit.Shield)
	}

	decoded := RawEntityFromUnitEntitySave(unit)
	if decoded.Shield != -3.5 {
		t.Fatalf("expected decode to restore negative shield, got=%f", decoded.Shield)
	}
}
