package world

import "time"

type burstDrillProfile struct {
	Tier                 int
	DrillTimeFrames      float32
	PowerPerSecond       float32
	BaseLiquid           LiquidID
	BaseLiquidPerFrame   float32
	BoostLiquid          LiquidID
	BoostLiquidPerFrame  float32
	LiquidBoostIntensity float32
	WarmupSpeed          float32
	BlockedItems         map[ItemID]struct{}
	DrillMultipliers     map[ItemID]float32
}

var burstDrillProfilesByBlockName = map[string]burstDrillProfile{
	"impact-drill": {
		Tier:                 6,
		DrillTimeFrames:      60 * 12,
		PowerPerSecond:       160.0 / 60.0,
		BaseLiquid:           waterLiquidID,
		BaseLiquidPerFrame:   10.0 / 60.0,
		BoostLiquid:          ozoneLiquidID,
		BoostLiquidPerFrame:  3.0 / 60.0,
		LiquidBoostIntensity: 1.75,
		WarmupSpeed:          0.01,
		BlockedItems:         map[ItemID]struct{}{thoriumItemID: {}, legacyThoriumItemID: {}},
		DrillMultipliers:     map[ItemID]float32{16: 2},
	},
	"eruption-drill": {
		Tier:                 7,
		DrillTimeFrames:      281.25,
		PowerPerSecond:       6,
		BaseLiquid:           hydrogenLiquidID,
		BaseLiquidPerFrame:   4.0 / 60.0,
		BoostLiquid:          cyanogenLiquidID,
		BoostLiquidPerFrame:  0.75 / 60.0,
		LiquidBoostIntensity: 2,
		WarmupSpeed:          0.01,
		DrillMultipliers:     map[ItemID]float32{16: 2},
	},
}

func (w *World) stepBurstDrillProduction(delta time.Duration) {
	if w == nil || w.model == nil {
		return
	}
	deltaFrames := float32(delta.Seconds() * 60)
	deltaSeconds := float32(delta.Seconds())
	if deltaFrames <= 0 || deltaSeconds <= 0 {
		return
	}
	for _, pos := range w.burstDrillTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Team == 0 || tile.Block == 0 {
			continue
		}
		name := w.blockNameByID(int16(tile.Block))
		prof, ok := burstDrillProfilesByBlockName[name]
		if !ok {
			continue
		}
		state := w.burstDrillStates[pos]
		w.stepSingleBurstDrillLocked(pos, tile, prof, &state, deltaFrames, deltaSeconds)
		w.burstDrillStates[pos] = state
	}
}

func (w *World) stepSingleBurstDrillLocked(pos int32, tile *Tile, prof burstDrillProfile, state *burstDrillRuntimeState, deltaFrames, deltaSeconds float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}
	if item, exists := firstBuildingItem(tile.Build); exists {
		_ = w.dumpSingleItemLocked(pos, tile, &item, nil)
	}

	dominantItem, dominantCount, _, ok := w.countDrillOreFilteredLocked(tile, prof.Tier, prof.BlockedItems)
	capacity := w.itemCapacityForBlockLocked(tile)
	if !ok || dominantCount <= 0 || capacity <= 0 || totalBuildingItems(tile.Build) > capacity-int32(dominantCount) {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		return
	}
	if prof.PowerPerSecond > 0 && !w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*deltaSeconds) {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		return
	}
	if prof.BaseLiquidPerFrame > 0 && !consumeBuildingLiquidLocked(tile.Build, prof.BaseLiquid, prof.BaseLiquidPerFrame*deltaFrames) {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		return
	}

	speed := float32(1)
	if prof.BoostLiquidPerFrame > 0 && consumeBuildingLiquidLocked(tile.Build, prof.BoostLiquid, prof.BoostLiquidPerFrame*deltaFrames) {
		speed = prof.LiquidBoostIntensity
	}

	drillTime := prof.DrillTimeFrames
	if mul := prof.DrillMultipliers[dominantItem]; mul > 0 {
		drillTime /= mul
	}
	if drillTime <= 0 {
		return
	}
	state.Progress += deltaFrames * speed
	state.Warmup = approachf(state.Warmup, clampf(state.Progress/drillTime, 0, 1), prof.WarmupSpeed*deltaFrames)

	if state.Progress < drillTime {
		return
	}

	space := capacity - totalBuildingItems(tile.Build)
	if space <= 0 {
		return
	}
	produced := dominantCount
	if int32(produced) > space {
		produced = int(space)
	}
	for i := 0; i < produced; i++ {
		if !w.offloadProducedItemLocked(pos, tile, dominantItem) {
			break
		}
	}
	state.Progress = float32mod(state.Progress, drillTime)
}

func (w *World) countDrillOreFilteredLocked(tile *Tile, tier int, blocked map[ItemID]struct{}) (ItemID, int, int, bool) {
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
			if _, blockedItem := blocked[item]; blockedItem {
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

func float32mod(value, divisor float32) float32 {
	if divisor <= 0 {
		return 0
	}
	for value >= divisor {
		value -= divisor
	}
	for value < 0 {
		value += divisor
	}
	return value
}
