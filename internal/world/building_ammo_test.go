package world

import "testing"

func TestItemTurretAmmoUsesBuildingInventory(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	w.SetModel(model)
	prof := buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		AmmoCapacity: 80,
		AmmoRegen:    3,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}
	w.buildingProfilesByName["duo"] = prof

	tile := placeTestBuilding(t, w, 4, 4, 910, 1, 0)
	pos := int32(4*model.Width + 4)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 2}}

	state := buildCombatState{Ammo: 25}
	state = w.regenBuildState(pos, tile, state, prof, tile.Build.Team, 1)
	if state.Ammo != 25 {
		t.Fatalf("expected item turret regen to ignore phantom ammo, got %f", state.Ammo)
	}
	if !w.buildingHasAmmoLocked(pos, tile, prof, state) {
		t.Fatal("expected item turret to detect ammo from building inventory")
	}
	if !w.consumeBuildingAmmoLocked(pos, tile, prof, &state) {
		t.Fatal("expected item turret shot to consume building inventory")
	}
	if got := w.totalBuildingAmmoLocked(tile, prof); got != 1 {
		t.Fatalf("expected one ammo item remaining after shot, got %d", got)
	}
	if got := totalBuildingItems(tile.Build); got != 1 {
		t.Fatalf("expected temporary ammo store to keep 1 internal ammo unit, got %d", got)
	}
	if state.Ammo != 25 {
		t.Fatalf("expected item turret shot not to consume build state ammo, got %f", state.Ammo)
	}
}

func TestItemTurretStoresAgainstTotalAmmoCapacity(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	w.SetModel(model)
	prof := buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		AmmoCapacity: 6,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}
	w.buildingProfilesByName["duo"] = prof

	tile := placeTestBuilding(t, w, 4, 4, 910, 1, 0)
	pos := int32(4*model.Width + 4)

	if !w.acceptsBuildingItemLocked(pos, tile, copperItemID) {
		t.Fatal("expected empty item turret to accept ammo")
	}
	if !w.storeAcceptedBuildingItemLocked(pos, tile, copperItemID, 1) {
		t.Fatal("expected initial ammo insert to succeed")
	}
	if got := w.totalBuildingAmmoLocked(tile, prof); got != 2 {
		t.Fatalf("expected total ammo units 2 after first insert, got %d", got)
	}
	if !w.acceptsBuildingItemLocked(pos, tile, graphiteItemID) {
		t.Fatal("expected mixed ammo insert while total capacity remains")
	}
	if !w.storeAcceptedBuildingItemLocked(pos, tile, graphiteItemID, 1) {
		t.Fatal("expected second ammo insert to fill remaining total capacity")
	}
	if got := w.totalBuildingAmmoLocked(tile, prof); got != 6 {
		t.Fatalf("expected total ammo units 6 at capacity, got %d", got)
	}
	if w.acceptsBuildingItemLocked(pos, tile, copperItemID) {
		t.Fatal("expected full item turret to reject further ammo")
	}
	if w.storeAcceptedBuildingItemLocked(pos, tile, copperItemID, 1) {
		t.Fatal("expected insert past total ammo capacity to fail")
	}
}

func TestItemTurretRejectsInvalidAmmoItems(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
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

	tile := placeTestBuilding(t, w, 4, 4, 910, 1, 0)
	pos := int32(4*model.Width + 4)

	if w.acceptsBuildingItemLocked(pos, tile, leadItemID) {
		t.Fatal("expected duo to reject lead as invalid ammo")
	}
	if w.storeAcceptedBuildingItemLocked(pos, tile, leadItemID, 1) {
		t.Fatal("expected invalid duo ammo insert to fail")
	}
	if got := w.totalBuildingAmmoLocked(tile, w.buildingProfilesByName["duo"]); got != 0 {
		t.Fatalf("expected invalid ammo insert not to change inventory, got %d", got)
	}
}

func TestResolveBuildingWeaponProfileUsesCurrentAmmoBulletType(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	w.SetModel(model)
	base := buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		BulletType:   94,
		AmmoCapacity: 80,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}
	w.buildingProfilesByName["duo"] = base

	tile := placeTestBuilding(t, w, 4, 4, 910, 1, 0)
	tile.Build.Items = []ItemStack{{Item: graphiteItemID, Amount: 3}}

	resolved := w.resolveBuildingWeaponProfileLocked(tile, base)
	if resolved.BulletType != 95 {
		t.Fatalf("expected graphite ammo to select bullet type 95, got %d", resolved.BulletType)
	}
}

func TestItemTurretHandleItemUsesAmmoMultiplierAndMovesCurrentAmmoToEnd(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	w.SetModel(model)
	w.buildingProfilesByName["duo"] = buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		AmmoCapacity: 30,
		AmmoPerShot:  1,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}

	tile := placeTestBuilding(t, w, 4, 4, 910, 1, 0)
	pos := int32(4*model.Width + 4)

	if !w.storeAcceptedBuildingItemLocked(pos, tile, copperItemID, 1) {
		t.Fatal("expected copper ammo insert to succeed")
	}
	if !w.storeAcceptedBuildingItemLocked(pos, tile, graphiteItemID, 1) {
		t.Fatal("expected graphite ammo insert to succeed")
	}
	if !w.storeAcceptedBuildingItemLocked(pos, tile, copperItemID, 1) {
		t.Fatal("expected second copper insert to merge and move to end")
	}
	if len(tile.Build.Items) != 2 {
		t.Fatalf("expected two internal ammo entries, got %d", len(tile.Build.Items))
	}
	if tile.Build.Items[0].Item != graphiteItemID || tile.Build.Items[0].Amount != 4 {
		t.Fatalf("expected graphite entry first with 4 ammo units, got %+v", tile.Build.Items[0])
	}
	if tile.Build.Items[1].Item != copperItemID || tile.Build.Items[1].Amount != 4 {
		t.Fatalf("expected copper entry moved to end with 4 ammo units, got %+v", tile.Build.Items[1])
	}
	if got := totalBuildingItems(tile.Build); got != 8 {
		t.Fatalf("expected temporary ammo store to keep 8 internal ammo units, got %d", got)
	}
}

func TestItemTurretConsumesAmmoUnitsFromCurrentEntry(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(10, 10)
	model.BlockNames = map[int16]string{
		910: "duo",
	}
	w.SetModel(model)
	prof := buildingWeaponProfile{
		ClassName:    "ItemTurret",
		Range:        136,
		Damage:       9,
		Interval:     0.6,
		AmmoCapacity: 30,
		AmmoPerShot:  3,
		TargetAir:    true,
		TargetGround: true,
		HitBuildings: true,
	}
	w.buildingProfilesByName["duo"] = prof

	tile := placeTestBuilding(t, w, 4, 4, 910, 1, 0)
	pos := int32(4*model.Width + 4)
	tile.Build.Items = []ItemStack{
		{Item: graphiteItemID, Amount: 2},
		{Item: copperItemID, Amount: 4},
	}

	state := buildCombatState{}
	if !w.consumeBuildingAmmoLocked(pos, tile, prof, &state) {
		t.Fatal("expected item turret ammo consume to succeed")
	}
	if len(tile.Build.Items) != 2 {
		t.Fatalf("expected both internal ammo entries to remain, got %d", len(tile.Build.Items))
	}
	if tile.Build.Items[1].Item != copperItemID || tile.Build.Items[1].Amount != 1 {
		t.Fatalf("expected current copper ammo entry to drop to 1, got %+v", tile.Build.Items[1])
	}
	if got := totalBuildingItems(tile.Build); got != 3 {
		t.Fatalf("expected temporary ammo store to keep 3 internal ammo units, got %d", got)
	}
}
