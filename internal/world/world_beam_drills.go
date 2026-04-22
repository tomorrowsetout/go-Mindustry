package world

import "time"

type beamDrillProfile struct {
	Tier                   int
	DrillTimeFrames        float32
	Range                  int
	PowerPerSecond         float32
	BaseLiquid             LiquidID
	BaseLiquidPerFrame     float32
	BaseLiquidRequired     bool
	BoostLiquid            LiquidID
	BoostLiquidPerFrame    float32
	OptionalBoostIntensity float32
	BlockedItems           map[ItemID]struct{}
	DrillMultipliers       map[ItemID]float32
}

var beamDrillProfilesByBlockName = map[string]beamDrillProfile{
	"plasma-bore": {
		Tier:                   3,
		DrillTimeFrames:        160,
		Range:                  5,
		PowerPerSecond:         0.15,
		BoostLiquid:            hydrogenLiquidID,
		BoostLiquidPerFrame:    0.25 / 60.0,
		OptionalBoostIntensity: 2.5,
	},
	"large-plasma-bore": {
		Tier:                   5,
		DrillTimeFrames:        100,
		Range:                  6,
		PowerPerSecond:         0.8,
		BaseLiquid:             hydrogenLiquidID,
		BaseLiquidPerFrame:     0.5 / 60.0,
		BaseLiquidRequired:     true,
		BoostLiquid:            nitrogenLiquidID,
		BoostLiquidPerFrame:    3.0 / 60.0,
		OptionalBoostIntensity: 2.5,
	},
}

type beamDrillFacing struct {
	item  ItemID
	count int
}

func (w *World) stepBeamDrillProduction(delta time.Duration) {
	if w == nil || w.model == nil {
		return
	}
	deltaFrames := float32(delta.Seconds() * 60)
	deltaSeconds := float32(delta.Seconds())
	if deltaFrames <= 0 || deltaSeconds <= 0 {
		return
	}
	for _, pos := range w.beamDrillTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Team == 0 || tile.Block == 0 {
			continue
		}
		name := w.blockNameByID(int16(tile.Block))
		prof, ok := beamDrillProfilesByBlockName[name]
		if !ok {
			continue
		}
		state := w.beamDrillStates[pos]
		w.stepSingleBeamDrillLocked(pos, tile, prof, &state, deltaFrames, deltaSeconds)
		w.beamDrillStates[pos] = state
	}
}

func (w *World) stepSingleBeamDrillLocked(pos int32, tile *Tile, prof beamDrillProfile, state *beamDrillRuntimeState, deltaFrames, deltaSeconds float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}
	if item, exists := firstBuildingItem(tile.Build); exists {
		_ = w.dumpSingleItemLocked(pos, tile, &item, nil)
	}
	item, facings, multiple := w.scanBeamDrillFacingLocked(tile, prof)
	if multiple || item == 0 || len(facings) == 0 || totalBuildingItems(tile.Build) >= w.itemCapacityForBlockLocked(tile) {
		state.Warmup = approachf(state.Warmup, 0, deltaFrames/60.0)
		return
	}
	if prof.PowerPerSecond > 0 && !w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*deltaSeconds) {
		state.Warmup = approachf(state.Warmup, 0, deltaFrames/60.0)
		return
	}
	if prof.BaseLiquidRequired && prof.BaseLiquidPerFrame > 0 && !consumeBuildingLiquidLocked(tile.Build, prof.BaseLiquid, prof.BaseLiquidPerFrame*deltaFrames) {
		state.Warmup = approachf(state.Warmup, 0, deltaFrames/60.0)
		return
	}

	speed := float32(1)
	if !prof.BaseLiquidRequired && prof.BaseLiquidPerFrame > 0 && consumeBuildingLiquidLocked(tile.Build, prof.BaseLiquid, prof.BaseLiquidPerFrame*deltaFrames) {
		speed = prof.OptionalBoostIntensity
	}
	if prof.BoostLiquidPerFrame > 0 && consumeBuildingLiquidLocked(tile.Build, prof.BoostLiquid, prof.BoostLiquidPerFrame*deltaFrames) {
		speed = prof.OptionalBoostIntensity
	}

	drillTime := prof.DrillTimeFrames
	if mul := prof.DrillMultipliers[item]; mul > 0 {
		drillTime /= mul
	}
	if drillTime <= 0 {
		return
	}
	state.Warmup = approachf(state.Warmup, 1, deltaFrames/60.0)
	state.Time += deltaFrames * speed
	if state.Time < drillTime {
		return
	}
	state.Time = float32mod(state.Time, drillTime)
	for _, facing := range facings {
		for i := 0; i < facing.count; i++ {
			if !w.offloadProducedItemLocked(pos, tile, facing.item) {
				break
			}
		}
	}
}

func (w *World) scanBeamDrillFacingLocked(tile *Tile, prof beamDrillProfile) (ItemID, []beamDrillFacing, bool) {
	if w == nil || w.model == nil || tile == nil {
		return 0, nil, false
	}
	size := w.blockSizeForTileLocked(tile)
	dx, dy := dirDelta(tile.Rotation)
	facings := make([]beamDrillFacing, 0, size)
	var lastItem ItemID
	multiple := false
	for p := 0; p < size; p++ {
		sx, sy := beamDrillSideOrigin(tile.X, tile.Y, size, tile.Rotation, p)
		foundItem := ItemID(0)
		for step := 0; step < prof.Range; step++ {
			rx := sx + dx*step
			ry := sy + dy*step
			if !w.model.InBounds(rx, ry) {
				break
			}
			other := &w.model.Tiles[ry*w.model.Width+rx]
			if other.Block == 0 {
				continue
			}
			item, hardness, ok := beamDrillWallItemFromTile(other, w)
			if ok && hardness <= prof.Tier {
				if _, blocked := prof.BlockedItems[item]; !blocked {
					foundItem = item
				}
			}
			break
		}
		if foundItem == 0 {
			continue
		}
		if lastItem != 0 && lastItem != foundItem {
			multiple = true
		}
		lastItem = foundItem
		facings = append(facings, beamDrillFacing{item: foundItem, count: 1})
	}
	if multiple {
		return 0, nil, true
	}
	return lastItem, facings, false
}

func beamDrillSideOrigin(x, y, size int, rotation int8, index int) (int, int) {
	low, _ := blockFootprintRange(size)
	switch tileRotationNorm(rotation) {
	case 0:
		return x + low + index, y + size/2 + 1
	case 1:
		return x + size/2 + 1, y - size/2 + index
	case 2:
		return x - size/2, y - size/2 + index
	default:
		return x + low + index, y - size/2
	}
}

func beamDrillWallItemFromTile(tile *Tile, w *World) (ItemID, int, bool) {
	if tile == nil || w == nil {
		return 0, 0, false
	}
	return mineWallItemByNames(w.blockNameByID(int16(tile.Block)), w.blockNameByID(int16(tile.Overlay)))
}
