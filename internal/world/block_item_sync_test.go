package world

import "testing"

func TestEmitBlockItemSyncLockedRoutesItemTurretToDedicatedAmmoEvent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	w.SetModel(model)
	w.buildingProfilesByName["duo"] = buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		AmmoCapacity: 80,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}

	placeTestBuilding(t, w, 5, 5, 910, 2, 0)
	pos := int32(5*model.Width + 5)

	w.mu.Lock()
	w.emitBlockItemSyncLocked(pos)
	events := append([]EntityEvent(nil), w.entityEvents...)
	w.mu.Unlock()

	if len(events) != 1 {
		t.Fatalf("expected one dedicated ammo sync event for item turret, got %d", len(events))
	}
	if events[0].Kind != EntityEventItemTurretAmmoSync {
		t.Fatalf("expected item turret ammo sync event, got %s", events[0].Kind)
	}
}

func TestEmitBlockItemSyncLockedForUnitFactory(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(12, 12)
	model.BlockNames = map[int16]string{
		920: "ground-factory",
	}
	w.SetModel(model)

	placeTestBuilding(t, w, 6, 6, 920, 2, 0)
	pos := int32(6*model.Width + 6)

	w.mu.Lock()
	w.emitBlockItemSyncLocked(pos)
	events := append([]EntityEvent(nil), w.entityEvents...)
	w.mu.Unlock()

	if len(events) != 1 {
		t.Fatalf("expected one block item sync event for unit factory, got %d", len(events))
	}
	if events[0].Kind != EntityEventBlockItemSync {
		t.Fatalf("expected block item sync event, got %s", events[0].Kind)
	}
}
