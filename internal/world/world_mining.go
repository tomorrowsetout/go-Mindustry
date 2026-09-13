package world

import "strings"

type MineTileResult struct {
	Item     ItemID
	Hardness int
	WorldX   float32
	WorldY   float32
}

func (w *World) ResolveMineTile(pos int32, mineFloor, mineWalls bool) (MineTileResult, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.resolveMineTileLocked(pos, mineFloor, mineWalls)
}

func (w *World) resolveMineTileLocked(pos int32, mineFloor, mineWalls bool) (MineTileResult, bool) {
	if w.model == nil {
		return MineTileResult{}, false
	}
	x, y := unpackTilePos(pos)
	if !w.model.InBounds(x, y) {
		return MineTileResult{}, false
	}
	tile := &w.model.Tiles[y*w.model.Width+x]
	blockName := w.blockNameByID(int16(tile.Block))
	if mineFloor && (tile.Block == 0 || blockName == "" || blockName == "air") {
		if item, hardness, ok := mineItemByContentName(w.blockNameByID(int16(tile.Overlay))); ok {
			return MineTileResult{Item: item, Hardness: hardness, WorldX: float32(tile.X * 8), WorldY: float32(tile.Y * 8)}, true
		}
		if item, hardness, ok := mineItemByContentName(w.blockNameByID(int16(tile.Floor))); ok {
			return MineTileResult{Item: item, Hardness: hardness, WorldX: float32(tile.X * 8), WorldY: float32(tile.Y * 8)}, true
		}
	}
	if mineWalls && !(tile.Block == 0 || blockName == "" || blockName == "air") {
		if item, hardness, ok := mineWallItemByNames(blockName, w.blockNameByID(int16(tile.Overlay))); ok {
			return MineTileResult{Item: item, Hardness: hardness, WorldX: float32(tile.X * 8), WorldY: float32(tile.Y * 8)}, true
		}
	}
	return MineTileResult{}, false
}

func (w *World) AcceptItemAt(pos int32, item ItemID, amount int32) int32 {
	if amount <= 0 {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.acceptItemAtLocked(pos, item, amount)
}

func (w *World) acceptItemAtLocked(pos int32, item ItemID, amount int32) int32 {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 {
		return 0
	}
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
	capacity := w.maximumAcceptedItemForBlockLocked(pos, tile, item)
	if capacity <= 0 {
		return 0
	}
	space := capacity
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
	if amount <= 0 {
		return 0
	}
	if !w.addItemAtLocked(pos, item, amount) {
		return 0
	}
	return amount
}

func (w *World) UnitMineSpeedMultiplier() float32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.unitMineSpeedMultiplierLocked()
}

func (w *World) FindClosestMineTileForItem(fromX, fromY float32, item ItemID, mineFloor, mineWalls bool, tier int) (int32, float32, float32, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || tier < 0 {
		return 0, 0, 0, false
	}
	bestPos := int32(-1)
	bestX, bestY := float32(0), float32(0)
	bestD2 := float32(-1)
	for i := range w.model.Tiles {
		tile := &w.model.Tiles[i]
		pos := packTilePos(tile.X, tile.Y)
		result, ok := w.resolveMineTileLocked(pos, mineFloor, mineWalls)
		if !ok || result.Item != item || result.Hardness > tier {
			continue
		}
		d2 := squaredWorldDistance(fromX, fromY, result.WorldX, result.WorldY)
		if bestPos < 0 || d2 < bestD2 {
			bestPos = pos
			bestX = result.WorldX
			bestY = result.WorldY
			bestD2 = d2
		}
	}
	if bestPos < 0 {
		return 0, 0, 0, false
	}
	return bestPos, bestX, bestY, true
}

func (w *World) unitMineSpeedMultiplierLocked() float32 {
	if rules := w.rulesMgr.Get(); rules != nil && rules.UnitMineSpeedMultiplier > 0 {
		return rules.UnitMineSpeedMultiplier
	}
	return 1
}

func mineWallItemByNames(blockName, overlayName string) (ItemID, int, bool) {
	if item, hardness, ok := mineItemByContentName(blockName); ok {
		return item, hardness, true
	}
	if item, hardness, ok := mineItemByContentName(overlayName); ok {
		return item, hardness, true
	}
	return 0, 0, false
}

func mineItemByContentName(name string) (ItemID, int, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ore-copper":
		return 0, 1, true
	case "ore-lead":
		return 1, 1, true
	case "ore-coal":
		return 5, 2, true
	case "ore-titanium":
		return 6, 3, true
	case "ore-thorium":
		return 7, 4, true
	case "ore-scrap", "scrap", "scrap-floor", "scrap-wall":
		return 8, 0, true
	case "sand", "sand-floor", "sand-wall", "darksand", "darksand-water", "darksand-tainted-water":
		return 4, 0, true
	case "ore-beryllium":
		return 16, 3, true
	case "ore-tungsten":
		return 17, 5, true
	default:
		return 0, 0, false
	}
}
