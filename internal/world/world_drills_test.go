package world

import "testing"

func TestCountDrillOreLockedPrefersCopperOverMoreSand(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		0:   "air",
		1:   "stone",
		2:   "ore-copper",
		3:   "sand-floor",
		429: "mechanical-drill",
	}
	w.SetModel(model)

	paintAreaFloor(t, w, 5, 5, 2, 3)
	drill := placeTestBuilding(t, w, 5, 5, 429, 1, 0)
	copperTile, err := w.Model().TileAt(5, 5)
	if err != nil || copperTile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	copperTile.Overlay = 2

	item, count, hardness, ok := w.countDrillOreLocked(drill, drillProfilesByBlockName["mechanical-drill"].Tier)
	if !ok {
		t.Fatal("expected drill ore selection to succeed")
	}
	if item != copperItemID {
		t.Fatalf("expected dominant item copper, got %d", item)
	}
	if count != 1 {
		t.Fatalf("expected dominant copper count 1, got %d", count)
	}
	if hardness != 1 {
		t.Fatalf("expected copper hardness 1, got %d", hardness)
	}
}

func TestCountDrillOreLockedFallsBackToSandWhenOnlySandIsMineable(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		0:   "air",
		3:   "sand-floor",
		429: "mechanical-drill",
	}
	w.SetModel(model)

	paintAreaFloor(t, w, 5, 5, 2, 3)
	drill := placeTestBuilding(t, w, 5, 5, 429, 1, 0)

	item, count, hardness, ok := w.countDrillOreLocked(drill, drillProfilesByBlockName["mechanical-drill"].Tier)
	if !ok {
		t.Fatal("expected drill ore selection to succeed")
	}
	if item != sandItemID {
		t.Fatalf("expected dominant item sand, got %d", item)
	}
	if count != 4 {
		t.Fatalf("expected sand count 4, got %d", count)
	}
	if hardness != 0 {
		t.Fatalf("expected sand hardness 0, got %d", hardness)
	}
}

func TestCountDrillOreLockedIgnoresHarderOreAndStillPrefersCopperOverSand(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		0:   "air",
		2:   "ore-copper",
		3:   "sand-floor",
		4:   "ore-titanium",
		429: "mechanical-drill",
	}
	w.SetModel(model)

	paintAreaFloor(t, w, 5, 5, 2, 3)
	drill := placeTestBuilding(t, w, 5, 5, 429, 1, 0)
	copperTile, err := w.Model().TileAt(5, 5)
	if err != nil || copperTile == nil {
		t.Fatalf("copper tile lookup failed: %v", err)
	}
	copperTile.Overlay = 2
	titaniumTile, err := w.Model().TileAt(6, 5)
	if err != nil || titaniumTile == nil {
		t.Fatalf("titanium tile lookup failed: %v", err)
	}
	titaniumTile.Overlay = 4

	item, count, hardness, ok := w.countDrillOreLocked(drill, drillProfilesByBlockName["mechanical-drill"].Tier)
	if !ok {
		t.Fatal("expected drill ore selection to succeed")
	}
	if item != copperItemID {
		t.Fatalf("expected dominant item copper after filtering harder ore, got %d", item)
	}
	if count != 1 {
		t.Fatalf("expected copper count 1 after filtering harder ore, got %d", count)
	}
	if hardness != 1 {
		t.Fatalf("expected copper hardness 1, got %d", hardness)
	}
}

func TestCountDrillOreFilteredLockedUsesSameDominantOrePriority(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		0:   "air",
		2:   "ore-copper",
		3:   "sand-floor",
		904: "impact-drill",
	}
	w.SetModel(model)

	paintAreaFloor(t, w, 8, 8, 4, 3)
	drill := placeTestBuilding(t, w, 8, 8, 904, 1, 0)
	copperTile, err := w.Model().TileAt(8, 8)
	if err != nil || copperTile == nil {
		t.Fatalf("tile lookup failed: %v", err)
	}
	copperTile.Overlay = 2

	item, count, hardness, ok := w.countDrillOreFilteredLocked(drill, burstDrillProfilesByBlockName["impact-drill"].Tier, nil)
	if !ok {
		t.Fatal("expected burst drill ore selection to succeed")
	}
	if item != copperItemID {
		t.Fatalf("expected filtered dominant item copper, got %d", item)
	}
	if count != 1 {
		t.Fatalf("expected filtered copper count 1, got %d", count)
	}
	if hardness != 1 {
		t.Fatalf("expected filtered copper hardness 1, got %d", hardness)
	}
}
