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
	capacity := w.itemCapacityAtLocked(pos)
	if capacity <= 0 {
		return 0
	}
	space := capacity - w.totalItemsAtLocked(pos)
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
