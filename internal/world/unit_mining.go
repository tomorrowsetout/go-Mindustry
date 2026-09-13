package world

const (
	unitMineRange         = float32(70)
	unitMineTransferRange = float32(220)
)

type unitMiningState struct {
	TargetPos int32
	Progress  float32
}

func (w *World) emitTransferItemToUnitEventLocked(unitID int32, item ItemID, x, y float32) {
	if w == nil || w.model == nil || unitID == 0 {
		return
	}
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventTransferItemToUnit,
		UnitID:     unitID,
		ItemID:     item,
		ItemAmount: 1,
		TransferX:  x,
		TransferY:  y,
	})
}

func (w *World) emitTransferItemToBuildEventLocked(unitID int32, buildPos int32, item ItemID, amount int32, x, y float32) {
	if w == nil || w.model == nil || buildPos < 0 || amount <= 0 || int(buildPos) >= len(w.model.Tiles) {
		return
	}
	tile := &w.model.Tiles[buildPos]
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventTransferItemToBuild,
		BuildPos:   packTilePos(tile.X, tile.Y),
		UnitID:     unitID,
		ItemID:     item,
		ItemAmount: amount,
		TransferX:  x,
		TransferY:  y,
	})
}

func (w *World) stepEntityMiningLocked(e *RawEntity, dt float32) bool {
	if w == nil || e == nil || e.Health <= 0 || dt <= 0 {
		return false
	}

	changed := false
	if e.PlayerID != 0 && w.depositEntityStackToNearbyCoreLocked(e, unitMineTransferRange) {
		changed = true
	}

	if e.MineTilePos < 0 {
		delete(w.unitMiningStates, e.ID)
		return changed
	}

	profile, ok := entityMiningProfile(*e)
	if !ok {
		e.MineTilePos = invalidEntityTilePos
		delete(w.unitMiningStates, e.ID)
		return true
	}

	result, ok := w.resolveMineTileLocked(e.MineTilePos, profile.MineFloor, profile.MineWalls)
	if !ok || result.Hardness > profile.Tier || !reachedTarget(e.X, e.Y, result.WorldX, result.WorldY, unitMineRange) {
		e.MineTilePos = invalidEntityTilePos
		delete(w.unitMiningStates, e.ID)
		return true
	}

	core, coreInRange := w.findNearestFriendlyCoreInRangeLocked(*e, unitMineTransferRange)
	if e.PlayerID == 0 && coreInRange && e.Stack.Amount > 0 && !entityCanCarryMinedItem(*e, result.Item, profile.Capacity) {
		if w.depositEntityStackToCoreLocked(e, core.BuildPos) {
			changed = true
		}
	}

	state := w.unitMiningStates[e.ID]
	if state.TargetPos != e.MineTilePos {
		state.TargetPos = e.MineTilePos
		state.Progress = 0
	}

	if !entityCanCarryMinedItem(*e, result.Item, profile.Capacity) {
		state.Progress = 0
		w.unitMiningStates[e.ID] = state
		return changed
	}

	threshold := float32(50 + result.Hardness*15)
	state.Progress += dt * 60 * profile.Speed * w.unitMineSpeedMultiplierLocked()

	for state.Progress >= threshold {
		state.Progress -= threshold

		if e.PlayerID != 0 && coreInRange && w.acceptItemAtLocked(core.BuildPos, result.Item, 1) == 1 {
			w.emitTransferItemToBuildEventLocked(e.ID, core.BuildPos, result.Item, 1, result.WorldX, result.WorldY)
			changed = true
			continue
		}

		if !entityCanCarryMinedItem(*e, result.Item, profile.Capacity) {
			e.MineTilePos = invalidEntityTilePos
			delete(w.unitMiningStates, e.ID)
			return true
		}

		if e.Stack.Amount == 0 {
			e.Stack.Item = result.Item
		}
		e.Stack.Amount++
		w.emitTransferItemToUnitEventLocked(e.ID, result.Item, result.WorldX, result.WorldY)
		changed = true
	}

	w.unitMiningStates[e.ID] = state
	return changed
}

func entityMiningProfile(e RawEntity) (UnitMiningProfile, bool) {
	profile := UnitMiningProfile{
		Speed:     e.MineSpeed,
		Tier:      int(e.MineTier),
		Capacity:  e.ItemCapacity,
		MineFloor: e.MineFloor,
		MineWalls: e.MineWalls,
	}
	return profile, profile.Speed > 0 && profile.Capacity > 0 && profile.Tier >= 0
}

func entityCanCarryMinedItem(e RawEntity, item ItemID, capacity int32) bool {
	if capacity <= 0 {
		return false
	}
	if e.Stack.Amount <= 0 {
		return true
	}
	if e.Stack.Item != item {
		return false
	}
	return e.Stack.Amount < capacity
}

func (w *World) findNearestFriendlyCoreInRangeLocked(src RawEntity, rangeLimit float32) (unitAITarget, bool) {
	core, ok := w.findNearestFriendlyCoreLocked(src)
	if !ok {
		return unitAITarget{}, false
	}
	if !reachedTarget(src.X, src.Y, core.X, core.Y, rangeLimit) {
		return unitAITarget{}, false
	}
	return core, true
}

func (w *World) depositEntityStackToNearbyCoreLocked(e *RawEntity, rangeLimit float32) bool {
	if w == nil || e == nil || e.Stack.Amount <= 0 || e.Team == 0 {
		return false
	}
	core, ok := w.findNearestFriendlyCoreInRangeLocked(*e, rangeLimit)
	if !ok {
		return false
	}
	return w.depositEntityStackToCoreLocked(e, core.BuildPos)
}

func (w *World) depositEntityStackToCoreLocked(e *RawEntity, corePos int32) bool {
	if w == nil || e == nil || e.Stack.Amount <= 0 || corePos < 0 {
		return false
	}
	item := e.Stack.Item
	amountBefore := e.Stack.Amount
	accepted := w.acceptItemAtLocked(corePos, e.Stack.Item, e.Stack.Amount)
	if accepted <= 0 {
		return false
	}
	e.Stack.Amount -= accepted
	if e.Stack.Amount <= 0 {
		e.Stack = ItemStack{}
	}
	if accepted > amountBefore {
		accepted = amountBefore
	}
	w.emitTransferItemToBuildEventLocked(e.ID, corePos, item, accepted, e.X, e.Y)
	return true
}
