package world

import (
	"math/rand"
	"strings"
	"time"
)

const crafterProgressEpsilon = 0.0001

const (
	copperItemID     = ItemID(0)
	leadItemID       = ItemID(1)
	metaglassItemID  = ItemID(2)
	graphiteItemID   = ItemID(3)
	scrapItemID      = ItemID(8)
	siliconItemID    = ItemID(9)
	titaniumItemID   = ItemID(6)
	berylliumItemID  = ItemID(16)
	tungstenItemID   = ItemID(17)
	oxideItemID      = ItemID(18)
	plastaniumItemID = ItemID(10)
	surgeAlloyItemID = ItemID(12)
)

type crafterProfile struct {
	CraftTimeFrames  float32
	PowerPerSecond   float32
	InputItems       []ItemStack
	InputLiquids     []LiquidStack
	OutputItems      []ItemStack
	OutputLiquids    []LiquidStack
	OutputDirections []int8
	Attribute        string
	BaseEfficiency   float32
	MinEfficiency    float32
	BoostScale       float32
	MaxBoost         float32
	HeatRequirement  float32
	OverheatScale    float32
	MaxEfficiency    float32
	HeatOutput       float32
	HeatWarmupRate   float32
	DumpExtraLiquid  bool
	CraftEffect      string
	UpdateEffect     string
	UpdateEffectRate float32
	WarmupSpeed      float32
}

type separatorProfile struct {
	CraftTimeFrames float32
	PowerPerSecond  float32
	InputItems      []ItemStack
	InputLiquids    []LiquidStack
	Results         []ItemStack
	ItemCapacity    int32
	WarmupSpeed     float32
}

var crafterProfilesByBlockName = map[string]crafterProfile{
	"graphite-press": {
		CraftTimeFrames: 90,
		InputItems:      []ItemStack{{Item: coalItemID, Amount: 2}},
		OutputItems:     []ItemStack{{Item: graphiteItemID, Amount: 1}},
		CraftEffect:     "pulverizemedium",
		WarmupSpeed:     0.019,
	},
	"multi-press": {
		CraftTimeFrames: 30,
		PowerPerSecond:  1.8,
		InputItems:      []ItemStack{{Item: coalItemID, Amount: 3}},
		InputLiquids:    []LiquidStack{{Liquid: waterLiquidID, Amount: 0.1}},
		OutputItems:     []ItemStack{{Item: graphiteItemID, Amount: 2}},
		CraftEffect:     "pulverizemedium",
		WarmupSpeed:     0.019,
	},
	"silicon-smelter": {
		CraftTimeFrames: 40,
		PowerPerSecond:  0.5,
		InputItems:      []ItemStack{{Item: coalItemID, Amount: 1}, {Item: sandItemID, Amount: 2}},
		OutputItems:     []ItemStack{{Item: siliconItemID, Amount: 1}},
		CraftEffect:     "smeltsmoke",
		WarmupSpeed:     0.019,
	},
	"silicon-arc-furnace": {
		CraftTimeFrames: 50,
		PowerPerSecond:  5,
		InputItems:      []ItemStack{{Item: graphiteItemID, Amount: 1}, {Item: sandItemID, Amount: 4}},
		OutputItems:     []ItemStack{{Item: siliconItemID, Amount: 4}},
		WarmupSpeed:     0.019,
	},
	"electrolyzer": {
		CraftTimeFrames: 10,
		PowerPerSecond:  1,
		InputLiquids:    []LiquidStack{{Liquid: waterLiquidID, Amount: 10.0 / 60.0}},
		OutputLiquids: []LiquidStack{
			{Liquid: ozoneLiquidID, Amount: 4.0 / 60.0},
			{Liquid: hydrogenLiquidID, Amount: 6.0 / 60.0},
		},
		OutputDirections: []int8{1, 3},
		DumpExtraLiquid:  true,
		WarmupSpeed:      0.019,
	},
	"atmospheric-concentrator": {
		CraftTimeFrames: 80,
		PowerPerSecond:  2,
		OutputLiquids:   []LiquidStack{{Liquid: nitrogenLiquidID, Amount: 16.0 / 60.0}},
		HeatRequirement: 24,
		MaxEfficiency:   1,
		WarmupSpeed:     0.019,
	},
	"oxidation-chamber": {
		CraftTimeFrames: 120,
		PowerPerSecond:  0.5,
		InputItems:      []ItemStack{{Item: berylliumItemID, Amount: 1}},
		InputLiquids:    []LiquidStack{{Liquid: ozoneLiquidID, Amount: 2.0 / 60.0}},
		OutputItems:     []ItemStack{{Item: oxideItemID, Amount: 1}},
		HeatOutput:      5,
		HeatWarmupRate:  0.15,
		WarmupSpeed:     0.019,
	},
	"electric-heater": {
		CraftTimeFrames: 80,
		PowerPerSecond:  100.0 / 60.0,
		HeatOutput:      3,
		HeatWarmupRate:  0.15,
		WarmupSpeed:     0.019,
	},
	"slag-heater": {
		CraftTimeFrames: 80,
		InputLiquids:    []LiquidStack{{Liquid: slagLiquidID, Amount: 40.0 / 60.0}},
		HeatOutput:      8,
		HeatWarmupRate:  0.15,
		WarmupSpeed:     0.019,
	},
	"phase-heater": {
		CraftTimeFrames: 480,
		InputItems:      []ItemStack{{Item: phaseFabricItemID, Amount: 1}},
		HeatOutput:      15,
		HeatWarmupRate:  0.15,
		WarmupSpeed:     0.019,
	},
	"heat-reactor": {
		CraftTimeFrames: 60 * 10,
		InputItems:      []ItemStack{{Item: thoriumItemID, Amount: 3}},
		InputLiquids:    []LiquidStack{{Liquid: nitrogenLiquidID, Amount: 1.0 / 60.0}},
		OutputItems:     []ItemStack{{Item: fissileMatterItemID, Amount: 1}},
		HeatOutput:      10,
		HeatWarmupRate:  0.15,
		CraftEffect:     "heatreactorsmoke",
		WarmupSpeed:     0.019,
	},
	"heat-source": {
		CraftTimeFrames: 1,
		HeatOutput:      1000,
		HeatWarmupRate:  1000,
		WarmupSpeed:     1,
	},
	"carbide-crucible": {
		CraftTimeFrames: 60 * 2.25 / 4,
		PowerPerSecond:  2,
		InputItems: []ItemStack{
			{Item: tungstenItemID, Amount: 2},
			{Item: graphiteItemID, Amount: 3},
		},
		OutputItems:     []ItemStack{{Item: carbideItemID, Amount: 1}},
		HeatRequirement: 40,
		MaxEfficiency:   1,
		CraftEffect:     "none",
		WarmupSpeed:     0.019,
	},
	"silicon-crucible": {
		CraftTimeFrames: 90,
		PowerPerSecond:  4,
		InputItems: []ItemStack{
			{Item: coalItemID, Amount: 4},
			{Item: sandItemID, Amount: 6},
			{Item: pyratiteItemID, Amount: 1},
		},
		OutputItems:    []ItemStack{{Item: siliconItemID, Amount: 8}},
		Attribute:      "heat",
		BaseEfficiency: 1,
		BoostScale:     0.15,
		MaxBoost:       1,
		CraftEffect:    "smeltsmoke",
		WarmupSpeed:    0.019,
	},
	"kiln": {
		CraftTimeFrames: 30,
		PowerPerSecond:  0.6,
		InputItems:      []ItemStack{{Item: leadItemID, Amount: 1}, {Item: sandItemID, Amount: 1}},
		OutputItems:     []ItemStack{{Item: metaglassItemID, Amount: 1}},
		CraftEffect:     "smeltsmoke",
		WarmupSpeed:     0.019,
	},
	"plastanium-compressor": {
		CraftTimeFrames:  60,
		PowerPerSecond:   3,
		InputItems:       []ItemStack{{Item: titaniumItemID, Amount: 2}},
		InputLiquids:     []LiquidStack{{Liquid: oilLiquidID, Amount: 0.25}},
		OutputItems:      []ItemStack{{Item: plastaniumItemID, Amount: 1}},
		CraftEffect:      "formsmoke",
		UpdateEffect:     "plasticburn",
		UpdateEffectRate: 0.04,
		WarmupSpeed:      0.019,
	},
	"phase-weaver": {
		CraftTimeFrames: 120,
		PowerPerSecond:  5,
		InputItems:      []ItemStack{{Item: thoriumItemID, Amount: 4}, {Item: sandItemID, Amount: 10}},
		OutputItems:     []ItemStack{{Item: phaseFabricItemID, Amount: 1}},
		CraftEffect:     "smeltsmoke",
		WarmupSpeed:     0.019,
	},
	"surge-smelter": {
		CraftTimeFrames: 75,
		PowerPerSecond:  4,
		InputItems: []ItemStack{
			{Item: copperItemID, Amount: 3},
			{Item: leadItemID, Amount: 4},
			{Item: titaniumItemID, Amount: 2},
			{Item: siliconItemID, Amount: 3},
		},
		OutputItems: []ItemStack{{Item: surgeAlloyItemID, Amount: 1}},
		CraftEffect: "smeltsmoke",
		WarmupSpeed: 0.019,
	},
	"cryofluid-mixer": {
		CraftTimeFrames: 120,
		PowerPerSecond:  1,
		InputItems:      []ItemStack{{Item: titaniumItemID, Amount: 1}},
		InputLiquids:    []LiquidStack{{Liquid: waterLiquidID, Amount: 12.0 / 60.0}},
		OutputLiquids:   []LiquidStack{{Liquid: cryofluidLiquidID, Amount: 12.0 / 60.0}},
		WarmupSpeed:     0.019,
	},
	"pyratite-mixer": {
		CraftTimeFrames: 60,
		PowerPerSecond:  0.2,
		InputItems: []ItemStack{
			{Item: coalItemID, Amount: 1},
			{Item: leadItemID, Amount: 2},
			{Item: sandItemID, Amount: 2},
		},
		OutputItems: []ItemStack{{Item: pyratiteItemID, Amount: 1}},
		WarmupSpeed: 0.019,
	},
	"blast-mixer": {
		CraftTimeFrames: 60,
		PowerPerSecond:  0.4,
		InputItems:      []ItemStack{{Item: pyratiteItemID, Amount: 1}, {Item: sporePodItemID, Amount: 1}},
		OutputItems:     []ItemStack{{Item: blastCompoundItemID, Amount: 1}},
		WarmupSpeed:     0.019,
	},
	"melter": {
		CraftTimeFrames: 10,
		PowerPerSecond:  1,
		InputItems:      []ItemStack{{Item: scrapItemID, Amount: 1}},
		OutputLiquids:   []LiquidStack{{Liquid: slagLiquidID, Amount: 12.0 / 60.0}},
		WarmupSpeed:     0.019,
	},
	"slag-centrifuge": {
		CraftTimeFrames: 120,
		PowerPerSecond:  2.0 / 60.0,
		InputItems:      []ItemStack{{Item: sandItemID, Amount: 1}},
		InputLiquids:    []LiquidStack{{Liquid: slagLiquidID, Amount: 40.0 / 60.0}},
		OutputLiquids:   []LiquidStack{{Liquid: galliumLiquidID, Amount: 1.0 / 60.0}},
		WarmupSpeed:     0.019,
	},
	"surge-crucible": {
		CraftTimeFrames: 60 * 3.0 / 4.0,
		PowerPerSecond:  1.5,
		InputItems:      []ItemStack{{Item: siliconItemID, Amount: 3}},
		InputLiquids:    []LiquidStack{{Liquid: slagLiquidID, Amount: 160.0 / 60.0}},
		OutputItems:     []ItemStack{{Item: surgeAlloyItemID, Amount: 1}},
		HeatRequirement: 40,
		MaxEfficiency:   1,
		CraftEffect:     "surgecrucismoke",
		WarmupSpeed:     0.019,
	},
	"cyanogen-synthesizer": {
		CraftTimeFrames: 80.0 / 4.0,
		PowerPerSecond:  2,
		InputItems:      []ItemStack{{Item: graphiteItemID, Amount: 1}},
		InputLiquids:    []LiquidStack{{Liquid: arkyciteLiquidID, Amount: 160.0 / 60.0}},
		OutputLiquids:   []LiquidStack{{Liquid: cyanogenLiquidID, Amount: 12.0 / 60.0}},
		HeatRequirement: 20,
		MaxEfficiency:   1,
		WarmupSpeed:     0.019,
	},
	"phase-synthesizer": {
		CraftTimeFrames: 60.0 * 2.0 / 4.0,
		PowerPerSecond:  8,
		InputItems: []ItemStack{
			{Item: thoriumItemID, Amount: 2},
			{Item: sandItemID, Amount: 6},
		},
		InputLiquids:    []LiquidStack{{Liquid: ozoneLiquidID, Amount: 8.0 / 60.0}},
		OutputItems:     []ItemStack{{Item: phaseFabricItemID, Amount: 1}},
		HeatRequirement: 32,
		MaxEfficiency:   1,
		WarmupSpeed:     0.019,
	},
	"pulverizer": {
		CraftTimeFrames:  40,
		PowerPerSecond:   0.5,
		InputItems:       []ItemStack{{Item: scrapItemID, Amount: 1}},
		OutputItems:      []ItemStack{{Item: sandItemID, Amount: 1}},
		CraftEffect:      "pulverize",
		UpdateEffect:     "pulverizesmall",
		UpdateEffectRate: 0.04,
		WarmupSpeed:      0.019,
	},
	"coal-centrifuge": {
		CraftTimeFrames: 30,
		PowerPerSecond:  0.7,
		InputLiquids:    []LiquidStack{{Liquid: oilLiquidID, Amount: 0.1}},
		OutputItems:     []ItemStack{{Item: coalItemID, Amount: 1}},
		CraftEffect:     "coalsmeltsmoke",
		WarmupSpeed:     0.019,
	},
	"spore-press": {
		CraftTimeFrames: 20,
		PowerPerSecond:  0.7,
		InputItems:      []ItemStack{{Item: sporePodItemID, Amount: 1}},
		OutputLiquids:   []LiquidStack{{Liquid: oilLiquidID, Amount: 18.0 / 60.0}},
		WarmupSpeed:     0.019,
	},
	"cultivator": {
		CraftTimeFrames: 100,
		PowerPerSecond:  80.0 / 60.0,
		InputLiquids:    []LiquidStack{{Liquid: waterLiquidID, Amount: 18.0 / 60.0}},
		OutputItems:     []ItemStack{{Item: sporePodItemID, Amount: 1}},
		Attribute:       "spores",
		BaseEfficiency:  1,
		BoostScale:      1,
		MaxBoost:        2,
		WarmupSpeed:     0.019,
	},
	"vent-condenser": {
		CraftTimeFrames: 120,
		PowerPerSecond:  0.5,
		OutputLiquids:   []LiquidStack{{Liquid: waterLiquidID, Amount: 30.0 / 60.0}},
		Attribute:       "steam",
		BaseEfficiency:  0,
		MinEfficiency:   9 - 0.0001,
		BoostScale:      1.0 / 9.0,
		CraftEffect:     "turbinegenerate",
		WarmupSpeed:     0.019,
	},
}

var separatorProfilesByBlockName = map[string]separatorProfile{
	"separator": {
		CraftTimeFrames: 35,
		PowerPerSecond:  1.1,
		InputLiquids:    []LiquidStack{{Liquid: slagLiquidID, Amount: 4.0 / 60.0}},
		Results: []ItemStack{
			{Item: copperItemID, Amount: 5},
			{Item: leadItemID, Amount: 3},
			{Item: graphiteItemID, Amount: 2},
			{Item: titaniumItemID, Amount: 2},
		},
		ItemCapacity: 10,
		WarmupSpeed:  0.02,
	},
	"disassembler": {
		CraftTimeFrames: 15,
		PowerPerSecond:  4,
		InputItems:      []ItemStack{{Item: scrapItemID, Amount: 1}},
		InputLiquids:    []LiquidStack{{Liquid: slagLiquidID, Amount: 0.12}},
		Results: []ItemStack{
			{Item: sandItemID, Amount: 2},
			{Item: graphiteItemID, Amount: 1},
			{Item: titaniumItemID, Amount: 1},
			{Item: thoriumItemID, Amount: 1},
		},
		ItemCapacity: 20,
		WarmupSpeed:  0.02,
	},
}

func (w *World) stepCrafterProduction(delta time.Duration) {
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
		if prof, ok := crafterProfilesByBlockName[name]; ok {
			state := w.crafterStates[pos]
			w.stepGenericCrafterLocked(pos, tile, prof, &state, deltaFrames, deltaSeconds)
			w.crafterStates[pos] = state
			continue
		}
		if prof, ok := separatorProfilesByBlockName[name]; ok {
			state := w.crafterStates[pos]
			w.stepSeparatorLocked(pos, tile, prof, &state, deltaFrames, deltaSeconds)
			w.crafterStates[pos] = state
		}
	}
}

func (w *World) stepGenericCrafterLocked(pos int32, tile *Tile, prof crafterProfile, state *crafterRuntimeState, deltaFrames, deltaSeconds float32) {
	if tile == nil || tile.Build == nil || state == nil || prof.CraftTimeFrames <= 0 {
		return
	}
	active := w.genericCrafterCanRunLocked(pos, tile, prof, deltaFrames, deltaSeconds)
	efficiencyMul := w.crafterEfficiencyMultiplierLocked(pos, tile, prof)
	if active {
		progressDelta := deltaFrames / prof.CraftTimeFrames
		progressDelta *= efficiencyMul
		if scale := w.genericLiquidOutputScaleLocked(tile, prof, deltaFrames); scale <= 0 {
			active = false
		} else {
			progressDelta *= scale
			if prof.PowerPerSecond > 0 && !w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*deltaSeconds*scale) {
				active = false
			} else if !consumeLiquidStacksLocked(tile.Build, prof.InputLiquids, deltaFrames*scale) {
				active = false
			} else {
				w.addCrafterLiquidStacksLocked(tile, prof.OutputLiquids, deltaFrames*scale*efficiencyMul)
				state.Progress += progressDelta
				state.Warmup = approachf(state.Warmup, w.crafterWarmupTargetLocked(pos, tile, prof), prof.WarmupSpeed*deltaFrames)
				state.TotalProgress += state.Warmup * deltaFrames
				if prof.UpdateEffect != "" && prof.UpdateEffectRate > 0 {
					if rand.Float32() < clampf(deltaFrames*prof.UpdateEffectRate, 0, 1) {
						w.emitEffectLocked(
							prof.UpdateEffect,
							float32(tile.X*8+4)+(rand.Float32()*2-1)*float32(w.blockSizeForTileLocked(tile)*4),
							float32(tile.Y*8+4)+(rand.Float32()*2-1)*float32(w.blockSizeForTileLocked(tile)*4),
							0,
						)
					}
				}
			}
		}
	}
	if !active {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
	}
	w.updateCrafterHeatStateLocked(pos, tile, prof, active, deltaFrames)
	for state.Progress+crafterProgressEpsilon >= 1 {
		if !hasRequiredItemsLocked(tile.Build, prof.InputItems) || !w.canStoreCrafterOutputsLocked(tile, prof.OutputItems) {
			break
		}
		removeItemStacksLocked(tile.Build, prof.InputItems)
		addItemStacksLocked(tile.Build, prof.OutputItems)
		state.Progress -= 1
		if state.Progress < 0 {
			state.Progress = 0
		}
		w.emitEffectLocked(prof.CraftEffect, float32(tile.X*8+4), float32(tile.Y*8+4), 0)
	}
	w.dumpCrafterOutputsLocked(pos, tile, prof.OutputItems, prof.OutputLiquids, prof.OutputDirections)
}

func (w *World) stepSeparatorLocked(pos int32, tile *Tile, prof separatorProfile, state *crafterRuntimeState, deltaFrames, deltaSeconds float32) {
	if tile == nil || tile.Build == nil || state == nil || prof.CraftTimeFrames <= 0 {
		return
	}
	active := separatorStoredOutputsLocked(tile.Build, prof.InputItems) < prof.ItemCapacity
	if active && !hasRequiredItemsLocked(tile.Build, prof.InputItems) {
		active = false
	}
	if active && prof.PowerPerSecond > 0 {
		active = w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*deltaSeconds)
	}
	if active {
		active = consumeLiquidStacksLocked(tile.Build, prof.InputLiquids, deltaFrames)
	}
	if active {
		state.Progress += deltaFrames / prof.CraftTimeFrames
		state.Warmup = approachf(state.Warmup, 1, prof.WarmupSpeed*deltaFrames)
		state.TotalProgress += state.Warmup * deltaFrames
	} else {
		state.Warmup = approachf(state.Warmup, 0, prof.WarmupSpeed*deltaFrames)
	}
	for state.Progress+crafterProgressEpsilon >= 1 && separatorStoredOutputsLocked(tile.Build, prof.InputItems) < prof.ItemCapacity {
		item, ok := pickSeparatorResult(prof.Results, &state.Seed, pos)
		if !ok {
			break
		}
		removeItemStacksLocked(tile.Build, prof.InputItems)
		tile.Build.AddItem(item, 1)
		state.Progress -= 1
		if state.Progress < 0 {
			state.Progress = 0
		}
	}
	w.dumpSingleItemLocked(pos, tile, nil, func(targetPos int32, item ItemID) bool {
		return separatorCanDumpItem(prof.Results, item) && !separatorConsumesItem(prof.InputItems, item)
	})
}

func (w *World) genericCrafterCanRunLocked(pos int32, tile *Tile, prof crafterProfile, deltaFrames, deltaSeconds float32) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	if w.crafterEfficiencyMultiplierLocked(pos, tile, prof) <= 0 {
		return false
	}
	if !hasRequiredItemsLocked(tile.Build, prof.InputItems) {
		return false
	}
	if !w.canStoreCrafterOutputsLocked(tile, prof.OutputItems) {
		return false
	}
	if len(prof.OutputLiquids) > 0 && w.genericLiquidOutputScaleLocked(tile, prof, deltaFrames) <= 0 {
		return false
	}
	if prof.PowerPerSecond > 0 && !w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*deltaSeconds) {
		return false
	}
	if prof.PowerPerSecond > 0 {
		w.refundPowerAtLocked(pos, prof.PowerPerSecond*deltaSeconds)
	}
	return canConsumeLiquidStacksLocked(tile.Build, prof.InputLiquids, deltaFrames)
}

func (w *World) crafterEfficiencyMultiplierLocked(pos int32, tile *Tile, prof crafterProfile) float32 {
	base := w.crafterBaseEfficiencyMultiplierLocked(tile, prof)
	if base <= 0 {
		return 0
	}
	return base * w.crafterHeatEfficiencyScaleLocked(pos, tile, prof)
}

func (w *World) genericLiquidOutputScaleLocked(tile *Tile, prof crafterProfile, deltaFrames float32) float32 {
	if tile == nil || tile.Build == nil || len(prof.OutputLiquids) == 0 {
		return 1
	}
	scale := float32(1)
	maxScale := float32(0)
	for _, output := range prof.OutputLiquids {
		if output.Amount <= 0 {
			continue
		}
		space := w.liquidCapacityForBlockLocked(tile) - tile.Build.LiquidAmount(output.Liquid)
		if space <= 0 {
			if !prof.DumpExtraLiquid {
				return 0
			}
			continue
		}
		value := space / (output.Amount * deltaFrames)
		if value < scale {
			scale = value
		}
		if value > maxScale {
			maxScale = value
		}
	}
	if prof.DumpExtraLiquid {
		return clampf(maxScale, 0, 1)
	}
	return clampf(scale, 0, 1)
}

func (w *World) canStoreCrafterOutputsLocked(tile *Tile, outputs []ItemStack) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	for _, output := range outputs {
		if tile.Build.ItemAmount(output.Item)+output.Amount > w.itemCapacityForBlockLocked(tile) {
			return false
		}
	}
	return true
}

func hasRequiredItemsLocked(build *Building, required []ItemStack) bool {
	if build == nil {
		return false
	}
	for _, stack := range required {
		if build.ItemAmount(stack.Item) < stack.Amount {
			return false
		}
	}
	return true
}

func removeItemStacksLocked(build *Building, stacks []ItemStack) {
	if build == nil {
		return
	}
	for _, stack := range stacks {
		_ = build.RemoveItem(stack.Item, stack.Amount)
	}
}

func addItemStacksLocked(build *Building, stacks []ItemStack) {
	if build == nil {
		return
	}
	for _, stack := range stacks {
		build.AddItem(stack.Item, stack.Amount)
	}
}

func canConsumeLiquidStacksLocked(build *Building, stacks []LiquidStack, deltaFrames float32) bool {
	if len(stacks) == 0 {
		return true
	}
	if build == nil {
		return false
	}
	for _, stack := range stacks {
		if build.LiquidAmount(stack.Liquid)+0.0001 < stack.Amount*deltaFrames {
			return false
		}
	}
	return true
}

func consumeLiquidStacksLocked(build *Building, stacks []LiquidStack, deltaFrames float32) bool {
	if len(stacks) == 0 {
		return true
	}
	if build == nil {
		return false
	}
	for _, stack := range stacks {
		if !consumeBuildingLiquidLocked(build, stack.Liquid, stack.Amount*deltaFrames) {
			return false
		}
	}
	return true
}

func addLiquidStacksLocked(build *Building, stacks []LiquidStack, deltaFrames float32) {
	if build == nil {
		return
	}
	for _, stack := range stacks {
		build.AddLiquid(stack.Liquid, stack.Amount*deltaFrames)
	}
}

func (w *World) addCrafterLiquidStacksLocked(tile *Tile, stacks []LiquidStack, deltaFrames float32) {
	if tile == nil || tile.Build == nil || deltaFrames <= 0 {
		return
	}
	capacity := w.liquidCapacityForBlockLocked(tile)
	if capacity <= 0 {
		return
	}
	for _, stack := range stacks {
		if stack.Amount <= 0 {
			continue
		}
		space := capacity - tile.Build.LiquidAmount(stack.Liquid)
		if space <= 0 {
			continue
		}
		amount := stack.Amount * deltaFrames
		if amount > space {
			amount = space
		}
		if amount > 0 {
			tile.Build.AddLiquid(stack.Liquid, amount)
		}
	}
}

func (w *World) dumpCrafterOutputsLocked(pos int32, tile *Tile, items []ItemStack, liquids []LiquidStack, directions []int8) {
	if tile == nil || tile.Build == nil {
		return
	}
	for _, output := range items {
		item := output.Item
		_ = w.dumpSingleItemLocked(pos, tile, &item, nil)
	}
	for i, output := range liquids {
		dir := int8(-1)
		if i < len(directions) {
			dir = directions[i]
		}
		_ = w.dumpCrafterLiquidLocked(pos, tile, output.Liquid, 2, dir)
	}
}

func (w *World) dumpCrafterLiquidLocked(pos int32, tile *Tile, liquid LiquidID, amount float32, dir int8) bool {
	if dir < 0 {
		return w.dumpLiquidLocked(pos, tile, liquid, amount)
	}
	targets := w.dumpCrafterDirectionTargetsLocked(pos, tile, dir)
	for _, target := range targets {
		if moved := w.tryMoveLiquidLocked(pos, target, liquid, amount, 0); moved > 0 {
			_ = tile.Build.RemoveLiquid(liquid, moved)
			return true
		}
	}
	return false
}

func (w *World) dumpCrafterDirectionTargetsLocked(pos int32, tile *Tile, dir int8) []int32 {
	if w == nil || w.model == nil || tile == nil {
		return nil
	}
	low, high := blockFootprintRange(w.blockSizeForTileLocked(tile))
	absDir := tileRotationNorm(tile.Rotation) + tileRotationNorm(dir)
	absDir %= 4
	out := make([]int32, 0, high-low+1)
	seen := make(map[int32]struct{}, high-low+1)
	appendTarget := func(x, y int) {
		otherPos, ok := w.buildingOccupyingCellLocked(x, y)
		if !ok || otherPos == pos {
			return
		}
		other := &w.model.Tiles[otherPos]
		if other.Build == nil || other.Block == 0 || other.Team != tile.Team {
			return
		}
		if _, exists := seen[otherPos]; exists {
			return
		}
		seen[otherPos] = struct{}{}
		out = append(out, otherPos)
	}
	switch absDir {
	case 0:
		x := tile.X + high + 1
		for y := tile.Y + low; y <= tile.Y+high; y++ {
			appendTarget(x, y)
		}
	case 1:
		y := tile.Y + high + 1
		for x := tile.X + low; x <= tile.X+high; x++ {
			appendTarget(x, y)
		}
	case 2:
		x := tile.X + low - 1
		for y := tile.Y + low; y <= tile.Y+high; y++ {
			appendTarget(x, y)
		}
	default:
		y := tile.Y + low - 1
		for x := tile.X + low; x <= tile.X+high; x++ {
			appendTarget(x, y)
		}
	}
	return out
}

func pickSeparatorResult(results []ItemStack, seed *uint32, pos int32) (ItemID, bool) {
	total := 0
	for _, stack := range results {
		if stack.Amount > 0 {
			total += int(stack.Amount)
		}
	}
	if total <= 0 {
		return 0, false
	}
	if *seed == 0 {
		*seed = uint32(pos) + 1
	}
	*seed = *seed*1664525 + 1013904223
	roll := int(*seed % uint32(total))
	count := 0
	for _, stack := range results {
		if stack.Amount <= 0 {
			continue
		}
		count += int(stack.Amount)
		if roll < count {
			return stack.Item, true
		}
	}
	return 0, false
}

func separatorCanDumpItem(results []ItemStack, item ItemID) bool {
	for _, stack := range results {
		if stack.Item == item {
			return true
		}
	}
	return false
}

func separatorConsumesItem(inputs []ItemStack, item ItemID) bool {
	for _, stack := range inputs {
		if stack.Item == item {
			return true
		}
	}
	return false
}

func separatorStoredOutputsLocked(build *Building, inputs []ItemStack) int32 {
	total := totalBuildingItems(build)
	for _, stack := range inputs {
		total -= build.ItemAmount(stack.Item)
	}
	if total < 0 {
		return 0
	}
	return total
}

func (w *World) sumFloorAttributeLocked(tile *Tile, attribute string) float32 {
	if w == nil || w.model == nil || tile == nil || attribute == "" {
		return 0
	}
	size := w.blockSizeForTileLocked(tile)
	low, high := blockFootprintRange(size)
	total := float32(0)
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			x := tile.X + dx
			y := tile.Y + dy
			if !w.model.InBounds(x, y) {
				continue
			}
			cell := &w.model.Tiles[y*w.model.Width+x]
			total += floorAttributeValueByName(attribute, w.blockNameByID(int16(cell.Floor)))
		}
	}
	return total
}

func floorAttributeValueByName(attribute, floorName string) float32 {
	switch normalizeAttributeName(attribute) {
	case "heat":
		switch floorName {
		case "hotrock":
			return 0.5
		case "magmarock":
			return 0.75
		case "basalt-vent", "yellow-stone-vent", "arkyic-vent":
			return 1
		default:
			return 0
		}
	case "spores":
		switch floorName {
		case "moss":
			return 0.15
		case "spore-moss":
			return 0.3
		case "tainted-water", "deep-tainted-water":
			return 0.15
		default:
			return 0
		}
	case "steam":
		switch floorName {
		case "rhyolite-vent", "carbon-vent", "arkyic-vent", "yellow-stone-vent", "red-stone-vent", "crystalline-vent", "stone-vent", "basalt-vent":
			return 1
		default:
			return 0
		}
	default:
		return 0
	}
}

func normalizeAttributeName(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}
