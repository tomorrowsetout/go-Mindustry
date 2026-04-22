package world

import "time"

type drillProfile struct {
	Tier                 int
	DrillTimeFrames      float32
	HardnessMul          float32
	LiquidBoostIntensity float32
	LiquidPerFrame       float32
	PowerPerSecond       float32
	WarmupSpeed          float32
}

var drillProfilesByBlockName = map[string]drillProfile{
	"mechanical-drill": {Tier: 2, DrillTimeFrames: 600, HardnessMul: 50, LiquidBoostIntensity: 1.6, LiquidPerFrame: 0.05, WarmupSpeed: 0.015},
	"pneumatic-drill":  {Tier: 3, DrillTimeFrames: 400, HardnessMul: 50, LiquidBoostIntensity: 1.6, LiquidPerFrame: 3.5 / 60, WarmupSpeed: 0.015},
	"laser-drill":      {Tier: 4, DrillTimeFrames: 280, HardnessMul: 50, LiquidBoostIntensity: 1.6, LiquidPerFrame: 0.08, PowerPerSecond: 1.10, WarmupSpeed: 0.015},
	"blast-drill":      {Tier: 5, DrillTimeFrames: 280, HardnessMul: 50, LiquidBoostIntensity: 1.8, LiquidPerFrame: 0.1, PowerPerSecond: 3.0, WarmupSpeed: 0.01},
}

func (w *World) stepDrillProduction(delta time.Duration) {
	if w == nil || w.model == nil {
		return
	}
	deltaFrames := float32(delta.Seconds() * 60)
	deltaSeconds := float32(delta.Seconds())
	if deltaFrames <= 0 || deltaSeconds <= 0 {
		return
	}
	for _, pos := range w.drillTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Team == 0 || tile.Block == 0 {
			continue
		}
		name := w.blockNameByID(int16(tile.Block))
		prof, ok := drillProfilesByBlockName[name]
		if !ok {
			continue
		}
		state := w.drillStates[pos]
		w.stepSingleDrillLocked(pos, tile, name, prof, &state, deltaFrames, deltaSeconds)
		w.drillStates[pos] = state
	}
}

func (w *World) stepSingleDrillLocked(pos int32, tile *Tile, name string, prof drillProfile, state *drillRuntimeState, deltaFrames, deltaSeconds float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}
	dominantItem, dominantCount, hardness, ok := w.countDrillOreLocked(tile, prof.Tier)
	if item, exists := firstBuildingItem(tile.Build); exists {
		_ = w.dumpSingleItemLocked(pos, tile, &item, nil)
	}
	if !ok || dominantCount <= 0 || totalBuildingItems(tile.Build) >= w.itemCapacityForBlockLocked(tile) {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		return
	}
	speed := float32(1)
	if prof.LiquidPerFrame > 0 && consumeBuildingLiquidLocked(tile.Build, waterLiquidID, prof.LiquidPerFrame*deltaFrames) {
		speed = prof.LiquidBoostIntensity
	}
	if prof.PowerPerSecond > 0 && !w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*deltaSeconds) {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		return
	}
	state.Warmup = approachf(state.Warmup, speed, prof.WarmupSpeed*deltaFrames)
	delay := prof.DrillTimeFrames + prof.HardnessMul*float32(hardness)
	if delay <= 0 {
		return
	}
	state.Progress += deltaFrames * float32(dominantCount) * speed * state.Warmup
	for state.Progress >= delay {
		if !w.offloadProducedItemLocked(pos, tile, dominantItem) {
			break
		}
		state.Progress -= delay
	}
}

func (w *World) countDrillOreLocked(tile *Tile, tier int) (ItemID, int, int, bool) {
	if w == nil || w.model == nil || tile == nil || tier <= 0 {
		return 0, 0, 0, false
	}
	size := w.blockSizeForTileLocked(tile)
	low, high := blockFootprintRange(size)
	counts := map[ItemID]int{}
	hardnessByItem := map[ItemID]int{}
	bestItem := ItemID(0)
	bestCount := 0
	bestHardness := 0
	found := false
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			x := tile.X + dx
			y := tile.Y + dy
			if !w.model.InBounds(x, y) {
				continue
			}
			cell := &w.model.Tiles[y*w.model.Width+x]
			item, hardness, ok := drillMineItemFromTile(cell, w)
			if !ok || hardness > tier {
				continue
			}
			counts[item]++
			hardnessByItem[item] = hardness
			count := counts[item]
			if preferDrillOreCandidate(item, count, bestItem, bestCount, found) {
				bestItem = item
				bestCount = count
				bestHardness = hardnessByItem[item]
				found = true
			}
		}
	}
	return bestItem, bestCount, bestHardness, found
}

func preferDrillOreCandidate(candidate ItemID, candidateCount int, bestItem ItemID, bestCount int, found bool) bool {
	if !found {
		return true
	}
	candidateLowPriority := drillOreLowPriority(candidate)
	bestLowPriority := drillOreLowPriority(bestItem)
	if candidateLowPriority != bestLowPriority {
		return !candidateLowPriority
	}
	if candidateCount != bestCount {
		return candidateCount > bestCount
	}
	return candidate > bestItem
}

func drillOreLowPriority(item ItemID) bool {
	return item == sandItemID
}

func drillMineItemFromTile(tile *Tile, w *World) (ItemID, int, bool) {
	if tile == nil || w == nil {
		return 0, 0, false
	}
	if item, hardness, ok := mineItemByContentName(w.blockNameByID(int16(tile.Overlay))); ok {
		return item, hardness, true
	}
	if item, hardness, ok := mineItemByContentName(w.blockNameByID(int16(tile.Floor))); ok {
		return item, hardness, true
	}
	return 0, 0, false
}
