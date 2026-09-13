package world

import "testing"

func TestResolveMineTileUsesOverlayOreForFloorMining(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(1, 1)
	model.BlockNames = map[int16]string{
		0: "air",
		1: "stone",
		2: "ore-copper",
	}
	model.Tiles[0].Floor = 1
	model.Tiles[0].Overlay = 2
	model.Tiles[0].Block = 0
	w.SetModel(model)

	got, ok := w.ResolveMineTile(packTilePos(0, 0), true, false)
	if !ok {
		t.Fatalf("expected mine tile to resolve")
	}
	if got.Item != 0 {
		t.Fatalf("expected copper item id 0, got %d", got.Item)
	}
	if got.Hardness != 1 {
		t.Fatalf("expected hardness 1, got %d", got.Hardness)
	}
}

func TestAcceptItemAtRespectsCoreCapacity(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(1, 1)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.Tiles[0].Block = 339
	model.Tiles[0].Team = 1
	model.Tiles[0].Build = &Building{
		Block: 339,
		Team:  1,
		X:     0,
		Y:     0,
	}
	model.Tiles[0].Build.AddItem(0, 3999)
	w.SetModel(model)

	accepted := w.AcceptItemAt(packTilePos(0, 0), 0, 5)
	if accepted != 1 {
		t.Fatalf("expected exactly 1 item accepted, got %d", accepted)
	}
	if total := model.Tiles[0].Build.ItemAmount(0); total != 4000 {
		t.Fatalf("expected core copper total 4000, got %d", total)
	}
}

func TestAcceptItemAtAllowsDifferentItemWhenCoreHasFullOtherItem(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(1, 1)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.Tiles[0].Block = 339
	model.Tiles[0].Team = 1
	model.Tiles[0].Build = &Building{
		Block: 339,
		Team:  1,
		X:     0,
		Y:     0,
	}
	model.Tiles[0].Build.AddItem(0, 4000)
	w.SetModel(model)

	accepted := w.AcceptItemAt(packTilePos(0, 0), 1, 5)
	if accepted != 5 {
		t.Fatalf("expected lead to still be accepted independently, got %d", accepted)
	}
	if got := model.Tiles[0].Build.ItemAmount(1); got != 5 {
		t.Fatalf("expected lead total 5, got %d", got)
	}
}

func TestAcceptItemAtUsesTotalCapacityForStandaloneContainer(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(1, 1)
	model.BlockNames = map[int16]string{
		500: "container",
	}
	model.Tiles[0].Block = 500
	model.Tiles[0].Team = 1
	model.Tiles[0].Build = &Building{
		Block: 500,
		Team:  1,
		X:     0,
		Y:     0,
		Items: []ItemStack{{Item: copperItemID, Amount: 299}},
	}
	w.SetModel(model)

	accepted := w.AcceptItemAt(0, leadItemID, 5)
	if accepted != 1 {
		t.Fatalf("expected standalone container to accept only 1 more item by total capacity, got %d", accepted)
	}
	if got := totalBuildingItems(model.Tiles[0].Build); got != 300 {
		t.Fatalf("expected standalone container total 300, got %d", got)
	}
	if got := model.Tiles[0].Build.ItemAmount(leadItemID); got != 1 {
		t.Fatalf("expected container to store 1 lead, got %d", got)
	}
}

func TestFindClosestMineTileForItemReturnsPackedTilePos(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		0: "air",
		1: "stone",
		2: "ore-copper",
	}
	model.Tiles[2*model.Width+3].Floor = 1
	model.Tiles[2*model.Width+3].Overlay = 2
	model.Tiles[5*model.Width+5].Floor = 1
	model.Tiles[5*model.Width+5].Overlay = 2
	w.SetModel(model)

	pos, wx, wy, ok := w.FindClosestMineTileForItem(0, 0, 0, true, false, 1)
	if !ok {
		t.Fatalf("expected closest mine tile to resolve")
	}
	if pos != packTilePos(3, 2) {
		t.Fatalf("expected packed tile pos %d, got %d", packTilePos(3, 2), pos)
	}
	if wx != float32(3*8) || wy != float32(2*8) {
		t.Fatalf("expected world pos (24,16), got (%v,%v)", wx, wy)
	}
}

func TestEntityMiningProducesInventoryStack(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		0: "air",
		1: "stone",
		2: "ore-copper",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	model.Tiles[2*model.Width+2].Floor = 1
	model.Tiles[2*model.Width+2].Overlay = 2
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{
		Name:         "alpha",
		Speed:        3,
		Flying:       true,
		MineSpeed:    6.5,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
	}
	placeTestBuilding(t, w, 0, 0, 339, 1, 0)

	ent := w.Model().AddEntity(RawEntity{
		TypeID:      35,
		PlayerID:    7,
		Team:        1,
		X:           float32(2 * 8),
		Y:           float32(2 * 8),
		MineTilePos: packTilePos(2, 2),
	})

	stepForSeconds(w, 0.35)
	got := findTestEntity(t, w, ent.ID)
	if got.Stack.Item != 0 || got.Stack.Amount <= 0 {
		t.Fatalf("expected player mining to produce copper stack, got %+v", got.Stack)
	}
	if got.MineTilePos != packTilePos(2, 2) {
		t.Fatalf("expected valid mine target to stay active, got %d", got.MineTilePos)
	}
}

func TestEntityMiningEmitsTransferItemToUnitEvent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		0: "air",
		1: "stone",
		2: "ore-copper",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	model.Tiles[2*model.Width+2].Floor = 1
	model.Tiles[2*model.Width+2].Overlay = 2
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{
		Name:         "alpha",
		Speed:        3,
		Flying:       true,
		MineSpeed:    6.5,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
	}
	placeTestBuilding(t, w, 0, 0, 339, 1, 0)

	ent := w.Model().AddEntity(RawEntity{
		TypeID:      35,
		PlayerID:    7,
		Team:        1,
		X:           float32(2 * 8),
		Y:           float32(2 * 8),
		MineTilePos: packTilePos(2, 2),
	})

	stepForSeconds(w, 0.35)
	evs := w.DrainEntityEvents()
	for _, ev := range evs {
		if ev.Kind != EntityEventTransferItemToUnit {
			continue
		}
		if ev.UnitID != ent.ID || ev.ItemID != 0 || ev.ItemAmount != 1 {
			t.Fatalf("unexpected transfer-to-unit event: %+v", ev)
		}
		return
	}
	t.Fatalf("expected mining to emit transfer_item_to_unit event, got=%+v", evs)
}

func TestEntityMiningClearsInvalidHardnessTarget(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		0: "air",
		1: "stone",
		7: "ore-thorium",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	model.Tiles[2*model.Width+2].Floor = 1
	model.Tiles[2*model.Width+2].Overlay = 7
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{
		Name:         "alpha",
		Speed:        3,
		Flying:       true,
		MineSpeed:    6.5,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
	}
	placeTestBuilding(t, w, 0, 0, 339, 1, 0)

	ent := w.Model().AddEntity(RawEntity{
		TypeID:      35,
		PlayerID:    7,
		Team:        1,
		X:           float32(2 * 8),
		Y:           float32(2 * 8),
		MineTilePos: packTilePos(2, 2),
	})

	stepForSeconds(w, 0.1)
	got := findTestEntity(t, w, ent.ID)
	if got.MineTilePos != invalidEntityTilePos {
		t.Fatalf("expected too-hard ore target to clear, got %d", got.MineTilePos)
	}
	if got.Stack.Amount != 0 {
		t.Fatalf("expected invalid mining target to produce no stack, got %+v", got.Stack)
	}
}

func TestPlayerMiningDepositsDirectlyIntoNearbyCore(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		0:   "air",
		1:   "stone",
		2:   "ore-copper",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	model.Tiles[1*model.Width+1].Floor = 1
	model.Tiles[1*model.Width+1].Overlay = 2
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{
		Name:         "alpha",
		Speed:        3,
		Flying:       true,
		MineSpeed:    6.5,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
	}

	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	ent := w.Model().AddEntity(RawEntity{
		TypeID:      35,
		PlayerID:    7,
		Team:        1,
		X:           16,
		Y:           16,
		MineTilePos: packTilePos(1, 1),
	})

	stepForSeconds(w, 0.35)
	got := findTestEntity(t, w, ent.ID)
	if got.Stack.Amount != 0 {
		t.Fatalf("expected nearby core to receive mined item immediately, got stack %+v", got.Stack)
	}
	if core.Build.ItemAmount(0) <= 0 {
		t.Fatalf("expected nearby core to receive copper, got %d", core.Build.ItemAmount(0))
	}
}

func TestPlayerMiningDirectCoreDepositEmitsTransferItemToBuildEvent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		0:   "air",
		1:   "stone",
		2:   "ore-copper",
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	model.Tiles[1*model.Width+1].Floor = 1
	model.Tiles[1*model.Width+1].Overlay = 2
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{
		Name:         "alpha",
		Speed:        3,
		Flying:       true,
		MineSpeed:    6.5,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
	}
	placeTestBuilding(t, w, 0, 0, 339, 1, 0)

	ent := w.Model().AddEntity(RawEntity{
		TypeID:      35,
		PlayerID:    7,
		Team:        1,
		X:           16,
		Y:           16,
		MineTilePos: packTilePos(1, 1),
	})

	stepForSeconds(w, 0.35)
	evs := w.DrainEntityEvents()
	for _, ev := range evs {
		if ev.Kind != EntityEventTransferItemToBuild {
			continue
		}
		if ev.UnitID != ent.ID || ev.ItemID != 0 || ev.ItemAmount != 1 || ev.BuildPos != packTilePos(0, 0) {
			t.Fatalf("unexpected transfer-to-build event: %+v", ev)
		}
		return
	}
	t.Fatalf("expected nearby core mining deposit to emit transfer_item_to_build event, got=%+v", evs)
}

func TestPlayerMiningDepositsHeldStackWhenReturningToCore(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{
		Name:         "alpha",
		Speed:        3,
		Flying:       true,
		MineSpeed:    6.5,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
	}

	core := placeTestBuilding(t, w, 0, 0, 339, 1, 0)
	ent := w.Model().AddEntity(RawEntity{
		TypeID:      35,
		PlayerID:    7,
		Team:        1,
		X:           8,
		Y:           8,
		MineTilePos: invalidEntityTilePos,
		Stack:       ItemStack{Item: 0, Amount: 4},
	})

	stepForSeconds(w, 0.1)
	got := findTestEntity(t, w, ent.ID)
	if got.Stack.Amount != 0 {
		t.Fatalf("expected held stack to unload into core when returning, got %+v", got.Stack)
	}
	if core.Build.ItemAmount(0) != 4 {
		t.Fatalf("expected core copper total 4 after unload, got %d", core.Build.ItemAmount(0))
	}
}

func TestPlayerReturningToCoreEmitsTransferItemToBuildEvent(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	model.UnitNames = map[int16]string{
		35: "alpha",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["alpha"] = unitRuntimeProfile{
		Name:         "alpha",
		Speed:        3,
		Flying:       true,
		MineSpeed:    6.5,
		MineTier:     1,
		ItemCapacity: 30,
		MineFloor:    true,
	}
	placeTestBuilding(t, w, 0, 0, 339, 1, 0)

	ent := w.Model().AddEntity(RawEntity{
		TypeID:      35,
		PlayerID:    7,
		Team:        1,
		X:           8,
		Y:           8,
		MineTilePos: invalidEntityTilePos,
		Stack:       ItemStack{Item: 0, Amount: 4},
	})

	stepForSeconds(w, 0.1)
	evs := w.DrainEntityEvents()
	for _, ev := range evs {
		if ev.Kind != EntityEventTransferItemToBuild {
			continue
		}
		if ev.UnitID != ent.ID || ev.ItemID != 0 || ev.ItemAmount != 4 || ev.BuildPos != packTilePos(0, 0) {
			t.Fatalf("unexpected held-stack transfer event: %+v", ev)
		}
		return
	}
	t.Fatalf("expected held stack unload to emit transfer_item_to_build event, got=%+v", evs)
}

func TestRequestItemFromBuildingPackedMovesItemsIntoUnit(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 1, 1, 339, 1, 0)
	core.Build.AddItem(0, 9)
	ent := w.Model().AddEntity(RawEntity{
		TypeID:       35,
		PlayerID:     7,
		Team:         1,
		X:            8,
		Y:            8,
		Health:       150,
		MaxHealth:    150,
		ItemCapacity: 30,
	})

	result, ok := w.RequestItemFromBuildingPacked(ent.ID, packTilePos(1, 1), 0, 5)
	if !ok {
		t.Fatalf("expected request item to succeed")
	}
	if result.Amount != 5 {
		t.Fatalf("expected transfer amount 5, got %d", result.Amount)
	}
	got := findTestEntity(t, w, ent.ID)
	if got.Stack.Item != 0 || got.Stack.Amount != 5 {
		t.Fatalf("expected entity stack copper=5, got %+v", got.Stack)
	}
	if remaining := core.Build.ItemAmount(0); remaining != 4 {
		t.Fatalf("expected core copper remaining 4, got %d", remaining)
	}
}

func TestRequestItemFromItemTurretIsRejected(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
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

	tile := placeTestBuilding(t, w, 1, 1, 910, 1, 0)
	tile.Build.Items = []ItemStack{{Item: copperItemID, Amount: 6}}
	ent := w.Model().AddEntity(RawEntity{
		TypeID:       35,
		PlayerID:     7,
		Team:         1,
		X:            12,
		Y:            12,
		Health:       150,
		MaxHealth:    150,
		ItemCapacity: 30,
	})

	if _, ok := w.RequestItemFromBuildingPacked(ent.ID, packTilePos(1, 1), copperItemID, 1); ok {
		t.Fatal("expected request item from item turret ammo storage to be rejected")
	}
}

func TestTransferUnitInventoryToBuildingPackedDepositsStack(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}
	w.SetModel(model)

	core := placeTestBuilding(t, w, 1, 1, 339, 1, 0)
	ent := w.Model().AddEntity(RawEntity{
		TypeID:       35,
		PlayerID:     7,
		Team:         1,
		X:            8,
		Y:            8,
		Health:       150,
		MaxHealth:    150,
		ItemCapacity: 30,
		Stack:        ItemStack{Item: 0, Amount: 6},
	})

	result, ok := w.TransferUnitInventoryToBuildingPacked(ent.ID, packTilePos(1, 1))
	if !ok {
		t.Fatalf("expected inventory deposit to succeed")
	}
	if result.Amount != 6 {
		t.Fatalf("expected deposit amount 6, got %d", result.Amount)
	}
	got := findTestEntity(t, w, ent.ID)
	if got.Stack.Amount != 0 {
		t.Fatalf("expected entity stack to clear, got %+v", got.Stack)
	}
	if stored := core.Build.ItemAmount(0); stored != 6 {
		t.Fatalf("expected core copper total 6, got %d", stored)
	}
}

func TestTransferUnitInventoryToItemTurretPackedPersistsAmmoInSnapshots(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(8, 8)
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

	tile := placeTestBuilding(t, w, 1, 1, 910, 1, 0)
	ent := w.Model().AddEntity(RawEntity{
		TypeID:       35,
		PlayerID:     7,
		Team:         1,
		X:            12,
		Y:            12,
		Health:       150,
		MaxHealth:    150,
		ItemCapacity: 30,
		Stack:        ItemStack{Item: copperItemID, Amount: 3},
	})

	result, ok := w.TransferUnitInventoryToBuildingPacked(ent.ID, packTilePos(1, 1))
	if !ok {
		t.Fatal("expected item turret deposit to succeed")
	}
	if result.Amount != 3 {
		t.Fatalf("expected deposit amount 3, got %d", result.Amount)
	}
	got := findTestEntity(t, w, ent.ID)
	if got.Stack.Amount != 0 {
		t.Fatalf("expected entity stack to clear, got %+v", got.Stack)
	}
	if gotAmmo := w.totalBuildingAmmoLocked(tile, w.buildingProfilesByName["duo"]); gotAmmo != 6 {
		t.Fatalf("expected duo ammo total 6 after deposit, got %d", gotAmmo)
	}
	if gotItems := totalBuildingItems(tile.Build); gotItems != 6 {
		t.Fatalf("expected temporary ammo store to keep 6 internal ammo units, got %d", gotItems)
	}

	snaps := w.ItemTurretBlockSyncSnapshotsForPackedLiveOnly([]int32{packTilePos(1, 1)})
	if len(snaps) != 1 {
		t.Fatalf("expected one duo snapshot, got %d", len(snaps))
	}
	_, r := decodeBlockSyncBase(t, snaps[0].Data)
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read turret reload counter failed: %v", err)
	}
	if _, err := r.ReadFloat32(); err != nil {
		t.Fatalf("read turret rotation failed: %v", err)
	}
	ammoKinds, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read turret ammo entry count failed: %v", err)
	}
	if ammoKinds != 1 {
		t.Fatalf("expected one ammo entry, got %d", ammoKinds)
	}
	ammoItem, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read turret ammo item failed: %v", err)
	}
	ammoAmount, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read turret ammo amount failed: %v", err)
	}
	if ammoItem != int16(copperItemID) || ammoAmount != 6 {
		t.Fatalf("expected copper ammo x6 in snapshot, got item=%d amount=%d", ammoItem, ammoAmount)
	}
}

func TestDropUnitItemsClearsStack(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(4, 4)
	w.SetModel(model)

	ent := w.Model().AddEntity(RawEntity{
		TypeID:       35,
		PlayerID:     7,
		Team:         1,
		X:            8,
		Y:            8,
		Health:       150,
		MaxHealth:    150,
		ItemCapacity: 30,
		Stack:        ItemStack{Item: 3, Amount: 11},
	})

	result, ok := w.DropUnitItems(ent.ID)
	if !ok {
		t.Fatalf("expected drop items to succeed")
	}
	if result.Item != 3 || result.Amount != 11 {
		t.Fatalf("expected dropped stack item=3 amount=11, got %+v", result)
	}
	got := findTestEntity(t, w, ent.ID)
	if got.Stack.Amount != 0 {
		t.Fatalf("expected entity stack to clear after drop, got %+v", got.Stack)
	}
}
