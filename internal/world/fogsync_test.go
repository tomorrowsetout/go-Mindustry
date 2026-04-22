package world

import (
	"testing"

	"mdt-server/internal/protocol"
)

func TestUnitSyncHiddenForViewerByFogDistance(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(64, 64)
	w.SetModel(model)
	rules := w.GetRulesManager().Get()
	rules.Fog = true

	visible := &protocol.UnitEntitySync{
		IDValue:  101,
		TeamID:   2,
		X:        40,
		Y:        40,
		Health:   100,
		Shooting: false,
	}
	if hidden := w.UnitSyncHiddenForViewer(1, 32, 32, visible); hidden {
		t.Fatal("expected nearby enemy unit to remain visible")
	}

	hiddenUnit := &protocol.UnitEntitySync{
		IDValue:  102,
		TeamID:   2,
		X:        400,
		Y:        400,
		Health:   100,
		Shooting: false,
	}
	if hidden := w.UnitSyncHiddenForViewer(1, 32, 32, hiddenUnit); !hidden {
		t.Fatal("expected distant enemy unit to be hidden by fog")
	}
}

func TestUnitSyncHiddenForViewerKeepsShootingEnemyVisible(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(64, 64)
	w.SetModel(model)
	rules := w.GetRulesManager().Get()
	rules.Fog = true

	unit := &protocol.UnitEntitySync{
		IDValue:  103,
		TeamID:   2,
		X:        400,
		Y:        400,
		Health:   100,
		Shooting: true,
	}
	if hidden := w.UnitSyncHiddenForViewer(1, 32, 32, unit); hidden {
		t.Fatal("expected shooting enemy unit to stay visible even in fog")
	}
}
