package world

type UnitItemTransferResult struct {
	UnitID   int32
	BuildPos int32
	Item     ItemID
	Amount   int32
	UnitX    float32
	UnitY    float32
	BuildX   float32
	BuildY   float32
}

func (w *World) RequestItemFromBuildingPacked(unitID, buildPos int32, item ItemID, amount int32) (UnitItemTransferResult, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.requestItemFromBuildingPackedLocked(unitID, buildPos, item, amount)
}

func (w *World) TransferUnitInventoryToBuildingPacked(unitID, buildPos int32) (UnitItemTransferResult, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.transferUnitInventoryToBuildingPackedLocked(unitID, buildPos)
}

func (w *World) DropUnitItems(unitID int32) (UnitItemTransferResult, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || unitID == 0 {
		return UnitItemTransferResult{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != unitID {
			continue
		}
		e := &w.model.Entities[i]
		if e.Stack.Amount <= 0 {
			return UnitItemTransferResult{}, false
		}
		out := UnitItemTransferResult{
			UnitID: unitID,
			Item:   e.Stack.Item,
			Amount: e.Stack.Amount,
			UnitX:  e.X,
			UnitY:  e.Y,
		}
		e.Stack = ItemStack{}
		w.model.EntitiesRev++
		return out, true
	}
	return UnitItemTransferResult{}, false
}

func (w *World) requestItemFromBuildingPackedLocked(unitID, buildPos int32, item ItemID, amount int32) (UnitItemTransferResult, bool) {
	if w == nil || w.model == nil || unitID == 0 || amount <= 0 {
		return UnitItemTransferResult{}, false
	}
	entityIndex := w.entityIndexByIDLocked(unitID)
	if entityIndex < 0 {
		return UnitItemTransferResult{}, false
	}
	tilePos, tile, ok := w.buildingTileByPackedLocked(buildPos)
	if !ok {
		return UnitItemTransferResult{}, false
	}
	e := &w.model.Entities[entityIndex]
	if e.Team == 0 || tile.Team != e.Team {
		return UnitItemTransferResult{}, false
	}
	buildX := float32(tile.X*8 + 4)
	buildY := float32(tile.Y*8 + 4)
	if !reachedTarget(e.X, e.Y, buildX, buildY, unitMineTransferRange) {
		return UnitItemTransferResult{}, false
	}
	space := unitAcceptedItemAmountLocked(*e, item)
	if space <= 0 {
		return UnitItemTransferResult{}, false
	}
	available := w.itemAmountAtLocked(tilePos, item)
	if available <= 0 {
		return UnitItemTransferResult{}, false
	}
	if amount > space {
		amount = space
	}
	if amount > available {
		amount = available
	}
	if amount <= 0 || !w.removeItemAtLocked(tilePos, item, amount) {
		return UnitItemTransferResult{}, false
	}
	if e.Stack.Amount == 0 {
		e.Stack.Item = item
	}
	e.Stack.Amount += amount
	w.model.EntitiesRev++
	return UnitItemTransferResult{
		UnitID:   unitID,
		BuildPos: buildPos,
		Item:     item,
		Amount:   amount,
		UnitX:    e.X,
		UnitY:    e.Y,
		BuildX:   buildX,
		BuildY:   buildY,
	}, true
}

func (w *World) transferUnitInventoryToBuildingPackedLocked(unitID, buildPos int32) (UnitItemTransferResult, bool) {
	if w == nil || w.model == nil || unitID == 0 {
		return UnitItemTransferResult{}, false
	}
	entityIndex := w.entityIndexByIDLocked(unitID)
	if entityIndex < 0 {
		return UnitItemTransferResult{}, false
	}
	tilePos, tile, ok := w.buildingTileByPackedLocked(buildPos)
	if !ok {
		return UnitItemTransferResult{}, false
	}
	e := &w.model.Entities[entityIndex]
	if e.Team == 0 || tile.Team != e.Team || e.Stack.Amount <= 0 {
		return UnitItemTransferResult{}, false
	}
	buildX := float32(tile.X*8 + 4)
	buildY := float32(tile.Y*8 + 4)
	if !reachedTarget(e.X, e.Y, buildX, buildY, unitMineTransferRange) {
		return UnitItemTransferResult{}, false
	}
	if rules := w.rulesMgr.Get(); rules != nil && rules.OnlyDepositCore {
		if _, _, _, shared := w.sharedCoreInventoryLocked(tilePos); !shared && !isCoreBlockName(w.blockNameByID(int16(tile.Block))) {
			return UnitItemTransferResult{}, false
		}
	}
	item := e.Stack.Item
	accepted := acceptedBuildingStackAmountLocked(w, tilePos, tile, item, e.Stack.Amount)
	if accepted <= 0 {
		return UnitItemTransferResult{}, false
	}
	e.Stack.Amount -= accepted
	if e.Stack.Amount <= 0 {
		e.Stack = ItemStack{}
	}
	w.model.EntitiesRev++
	return UnitItemTransferResult{
		UnitID:   unitID,
		BuildPos: buildPos,
		Item:     item,
		Amount:   accepted,
		UnitX:    e.X,
		UnitY:    e.Y,
		BuildX:   buildX,
		BuildY:   buildY,
	}, true
}

func acceptedBuildingStackAmountLocked(w *World, pos int32, tile *Tile, item ItemID, amount int32) int32 {
	if w == nil || tile == nil || amount <= 0 {
		return 0
	}
	if tile.Build != nil {
		if prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block)); ok && w.buildingUsesItemAmmoLocked(tile, prof) {
			ammoUnits := w.turretAmmoUnitsPerItemLocked(tile, item)
			if ammoUnits <= 0 {
				return 0
			}
			space := w.buildingItemAmmoCapacityLocked(tile, prof) - w.totalBuildingAmmoLocked(tile, prof)
			if space <= 0 {
				return 0
			}
			maxAccepted := space / ammoUnits
			if maxAccepted <= 0 {
				return 0
			}
			if amount > maxAccepted {
				amount = maxAccepted
			}
			if amount <= 0 || !w.turretHandleItemLocked(pos, tile, item, amount) {
				return 0
			}
			return amount
		}
	}
	maxAccepted := w.maximumAcceptedItemForBlockLocked(pos, tile, item)
	if maxAccepted <= 0 {
		return 0
	}
	space := maxAccepted
	if w.buildingUsesTotalItemCapacityLocked(pos, tile) {
		space -= w.totalItemsAtLocked(pos)
	} else {
		space -= w.itemAmountAtLocked(pos, item)
	}
	if space <= 0 {
		return 0
	}
	if amount > space {
		amount = space
	}
	if amount <= 0 || !w.addItemAtLocked(pos, item, amount) {
		return 0
	}
	return amount
}

func unitAcceptedItemAmountLocked(e RawEntity, item ItemID) int32 {
	if e.ItemCapacity <= 0 {
		return 0
	}
	if e.Stack.Amount <= 0 {
		return e.ItemCapacity
	}
	if e.Stack.Item != item {
		return 0
	}
	if e.Stack.Amount >= e.ItemCapacity {
		return 0
	}
	return e.ItemCapacity - e.Stack.Amount
}

func (w *World) entityIndexByIDLocked(id int32) int {
	if w == nil || w.model == nil || id == 0 {
		return -1
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID == id {
			return i
		}
	}
	return -1
}

func (w *World) buildingTileByPackedLocked(packedPos int32) (int32, *Tile, bool) {
	if w == nil || w.model == nil {
		return 0, nil, false
	}
	pos, ok := w.tileIndexFromPackedPosLocked(packedPos)
	if !ok || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0, nil, false
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 {
		return 0, nil, false
	}
	return pos, tile, true
}

func (w *World) UnitIDsByType(typeID int16, excludeWaveTeam TeamID) []int32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w == nil || w.model == nil || typeID <= 0 {
		return nil
	}
	out := make([]int32, 0, 4)
	for _, entity := range w.model.Entities {
		if entity.ID == 0 || entity.TypeID != typeID || entity.PlayerID != 0 || entity.Team == 0 {
			continue
		}
		if excludeWaveTeam != 0 && entity.Team == excludeWaveTeam {
			continue
		}
		out = append(out, entity.ID)
	}
	return out
}
