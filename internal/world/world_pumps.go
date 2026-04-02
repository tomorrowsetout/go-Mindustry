package world

import (
	"math/rand"
	"strings"
	"time"
)

const (
	oilLiquidID      = LiquidID(2)
	arkyciteLiquidID = LiquidID(5)
	sandItemID       = ItemID(4)
)

type floorPumpProfile struct {
	PumpAmountPerFrame float32
	PowerPerSecond     float32
	WarmupSpeed        float32
}

type solidPumpProfile struct {
	Result             LiquidID
	PumpAmountPerFrame float32
	PowerPerSecond     float32
	BaseEfficiency     float32
	Attribute          string
	UpdateEffect       string
	UpdateEffectChance float32
	ItemUseTimeFrames  float32
	ItemConsume        ItemID
	LiquidConsume      LiquidID
	LiquidPerFrame     float32
	WarmupSpeed        float32
}

var floorPumpProfilesByBlockName = map[string]floorPumpProfile{
	"mechanical-pump": {PumpAmountPerFrame: 7.0 / 60.0, WarmupSpeed: 0.019},
	"rotary-pump":     {PumpAmountPerFrame: 0.2, PowerPerSecond: 0.3, WarmupSpeed: 0.019},
	"impulse-pump":    {PumpAmountPerFrame: 0.22, PowerPerSecond: 1.3, WarmupSpeed: 0.019},
}

var solidPumpProfilesByBlockName = map[string]solidPumpProfile{
	"water-extractor": {
		Result:             waterLiquidID,
		PumpAmountPerFrame: 0.11,
		PowerPerSecond:     1.5,
		BaseEfficiency:     1,
		Attribute:          "water",
		WarmupSpeed:        0.02,
	},
	"oil-extractor": {
		Result:             oilLiquidID,
		PumpAmountPerFrame: 0.25,
		PowerPerSecond:     3,
		BaseEfficiency:     0,
		Attribute:          "oil",
		UpdateEffect:       "pulverize",
		UpdateEffectChance: 0.05,
		ItemUseTimeFrames:  60,
		ItemConsume:        sandItemID,
		LiquidConsume:      waterLiquidID,
		LiquidPerFrame:     0.15,
		WarmupSpeed:        0.02,
	},
}

func (w *World) stepPumpProduction(delta time.Duration) {
	if w == nil || w.model == nil {
		return
	}
	deltaFrames := float32(delta.Seconds() * 60)
	deltaSeconds := float32(delta.Seconds())
	if deltaFrames <= 0 || deltaSeconds <= 0 {
		return
	}
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 || tile.Build.Team == 0 {
			continue
		}
		name := w.blockNameByID(int16(tile.Block))
		if prof, ok := floorPumpProfilesByBlockName[name]; ok {
			state := w.pumpStates[pos]
			w.stepFloorPumpLocked(pos, tile, prof, &state, deltaFrames, deltaSeconds)
			w.pumpStates[pos] = state
			continue
		}
		if prof, ok := solidPumpProfilesByBlockName[name]; ok {
			state := w.pumpStates[pos]
			w.stepSolidPumpLocked(pos, tile, prof, &state, deltaFrames, deltaSeconds)
			w.pumpStates[pos] = state
		}
	}
}

func (w *World) stepFloorPumpLocked(pos int32, tile *Tile, prof floorPumpProfile, state *pumpRuntimeState, deltaFrames, deltaSeconds float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}
	liquid, multiplier, ok := w.floorPumpSourceLocked(tile)
	if !ok || multiplier <= 0 {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		return
	}
	if totalBuildingLiquids(tile.Build) >= w.liquidCapacityForBlockLocked(tile)-0.001 {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		_ = w.dumpLiquidLocked(pos, tile, liquid, deltaFrames)
		return
	}
	if prof.PowerPerSecond > 0 && !w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*deltaSeconds) {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		return
	}
	capacity := w.liquidCapacityForBlockLocked(tile)
	space := capacity - totalBuildingLiquids(tile.Build)
	if space <= 0 {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		_ = w.dumpLiquidLocked(pos, tile, liquid, deltaFrames)
		return
	}
	produced := prof.PumpAmountPerFrame * multiplier * deltaFrames
	if produced > space {
		produced = space
	}
	if produced > 0 {
		tile.Build.AddLiquid(liquid, produced)
		state.Warmup = approachf(state.Warmup, 1, prof.WarmupSpeed*deltaFrames)
	} else {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
	}
	state.Progress += state.Warmup * deltaFrames
	_ = w.dumpLiquidLocked(pos, tile, liquid, deltaFrames)
}

func (w *World) stepSolidPumpLocked(pos int32, tile *Tile, prof solidPumpProfile, state *pumpRuntimeState, deltaFrames, deltaSeconds float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}
	if tile.Build.LiquidAmount(prof.Result) >= w.liquidCapacityForBlockLocked(tile)-0.001 {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		_ = w.dumpLiquidLocked(pos, tile, prof.Result, deltaFrames)
		return
	}
	fraction := w.solidPumpFractionLocked(tile, prof)
	if fraction <= 0 {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		return
	}
	if prof.PowerPerSecond > 0 && !w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*deltaSeconds) {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		return
	}
	if prof.ItemUseTimeFrames > 0 {
		if tile.Build.ItemAmount(prof.ItemConsume) <= 0 {
			state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
			return
		}
		if state.Accumulator >= prof.ItemUseTimeFrames {
			if !tile.Build.RemoveItem(prof.ItemConsume, 1) {
				state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
				return
			}
			state.Accumulator -= prof.ItemUseTimeFrames
		}
	}
	if prof.LiquidPerFrame > 0 && !consumeBuildingLiquidLocked(tile.Build, prof.LiquidConsume, prof.LiquidPerFrame*deltaFrames) {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		return
	}
	space := w.liquidCapacityForBlockLocked(tile) - totalBuildingLiquids(tile.Build)
	if space <= 0 {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
		_ = w.dumpLiquidLocked(pos, tile, prof.Result, deltaFrames)
		return
	}
	produced := prof.PumpAmountPerFrame * fraction * deltaFrames
	if produced > space {
		produced = space
	}
	if produced > 0 {
		tile.Build.AddLiquid(prof.Result, produced)
		state.Warmup = approachf(state.Warmup, 1, prof.WarmupSpeed*deltaFrames)
		state.Progress += state.Warmup * deltaFrames
		if prof.ItemUseTimeFrames > 0 {
			state.Accumulator += deltaFrames
		}
		if prof.UpdateEffect != "" && prof.UpdateEffectChance > 0 {
			chance := clampf(deltaFrames*prof.UpdateEffectChance, 0, 1)
			if rand.Float32() < chance {
				w.emitEffectLocked(
					prof.UpdateEffect,
					float32(tile.X*8+4)+(rand.Float32()*2-1)*float32(w.blockSizeForTileLocked(tile)*4),
					float32(tile.Y*8+4)+(rand.Float32()*2-1)*float32(w.blockSizeForTileLocked(tile)*4),
					0,
				)
			}
		}
	} else {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
	}
	_ = w.dumpLiquidLocked(pos, tile, prof.Result, deltaFrames)
}

func (w *World) floorPumpSourceLocked(tile *Tile) (LiquidID, float32, bool) {
	if w == nil || w.model == nil || tile == nil {
		return 0, 0, false
	}
	size := w.blockSizeForTileLocked(tile)
	low, high := blockFootprintRange(size)
	found := false
	source := LiquidID(0)
	totalMultiplier := float32(0)
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			x := tile.X + dx
			y := tile.Y + dy
			if !w.model.InBounds(x, y) {
				return 0, 0, false
			}
			cell := &w.model.Tiles[y*w.model.Width+x]
			liquid, multiplier, ok := pumpFloorLiquidByName(w.blockNameByID(int16(cell.Floor)))
			if !ok {
				return 0, 0, false
			}
			if !found {
				source = liquid
				found = true
			} else if liquid != source {
				return 0, 0, false
			}
			totalMultiplier += multiplier
		}
	}
	return source, totalMultiplier, found
}

func (w *World) solidPumpFractionLocked(tile *Tile, prof solidPumpProfile) float32 {
	if w == nil || w.model == nil || tile == nil {
		return 0
	}
	size := w.blockSizeForTileLocked(tile)
	low, high := blockFootprintRange(size)
	divisor := float32(size * size)
	if divisor <= 0 {
		return 0
	}
	validTiles := float32(0)
	boost := float32(0)
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			x := tile.X + dx
			y := tile.Y + dy
			if !w.model.InBounds(x, y) {
				continue
			}
			cell := &w.model.Tiles[y*w.model.Width+x]
			floorName := w.blockNameByID(int16(cell.Floor))
			if !solidPumpCanPumpFloor(floorName) {
				continue
			}
			validTiles += prof.BaseEfficiency / divisor
			boost += solidPumpAttributeValueByFloorName(prof.Attribute, floorName) / divisor
		}
	}
	return maxf(validTiles+boost, 0)
}

func solidPumpCanPumpFloor(name string) bool {
	_, _, ok := pumpFloorLiquidByName(name)
	return !ok
}

func pumpFloorLiquidByName(name string) (LiquidID, float32, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "deep-water", "deep-tainted-water":
		return waterLiquidID, 1.5, true
	case "water", "tainted-water", "sand-water", "darksand-water", "darksand-tainted-water":
		return waterLiquidID, 1, true
	case "tar":
		return oilLiquidID, 1, true
	case "slag":
		return slagLiquidID, 1, true
	case "cryofluid":
		return cryofluidLiquidID, 0.5, true
	case "arkycite-floor":
		return arkyciteLiquidID, 1, true
	default:
		return 0, 0, false
	}
}

func solidPumpAttributeValueByFloorName(attribute, floorName string) float32 {
	switch strings.ToLower(strings.TrimSpace(attribute)) {
	case "water":
		switch strings.ToLower(strings.TrimSpace(floorName)) {
		case "char", "basalt":
			return -0.25
		case "hotrock":
			return -0.5
		case "magmarock":
			return -0.75
		case "mud":
			return 1
		case "rhyolite", "rhyolite-crater", "rough-rhyolite", "regolith", "yellow-stone", "carbon-stone", "ferric-stone", "ferric-craters", "red-stone", "dense-red-stone":
			return -1
		case "red-ice", "ice":
			return 0.4
		case "grass":
			return 0.1
		case "salt":
			return -0.3
		case "snow":
			return 0.2
		case "ice-snow":
			return 0.3
		default:
			return 0
		}
	case "oil":
		switch strings.ToLower(strings.TrimSpace(floorName)) {
		case "sand-floor":
			return 0.7
		case "darksand":
			return 1.5
		case "salt":
			return 0.3
		case "shale":
			return 2
		default:
			return 0
		}
	default:
		return 0
	}
}
