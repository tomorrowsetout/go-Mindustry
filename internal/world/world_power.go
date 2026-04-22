package world

import (
	"math"
	"sort"
	"strings"
	"time"
)

const (
	waterLiquidID       = LiquidID(0)
	slagLiquidID        = LiquidID(1)
	cryofluidLiquidID   = LiquidID(3)
	neoplasmLiquidID    = LiquidID(4)
	ozoneLiquidID       = LiquidID(7)
	hydrogenLiquidID    = LiquidID(8)
	nitrogenLiquidID    = LiquidID(9)
	cyanogenLiquidID    = LiquidID(10)
	galliumLiquidID     = LiquidID(6)
	coalItemID          = ItemID(5)
	legacyThoriumItemID = ItemID(5)
	thoriumItemID       = ItemID(7)
	phaseFabricItemID   = ItemID(11)
	carbideItemID       = ItemID(19)
	fissileMatterItemID = ItemID(20)
	blastCompoundItemID = ItemID(14)
	pyratiteItemID      = ItemID(15)
	sporePodItemID      = ItemID(13)
)

type teamPowerState struct {
	Stored   float32
	Capacity float32
	Produced float32
	Consumed float32
}

type powerNetState struct {
	Team     TeamID
	Budget   float32
	Capacity float32
	Produced float32
	Consumed float32
	Storage  []powerStorageRef
}

type powerStorageRef struct {
	Pos      int32
	Capacity float32
}

type powerGeneratorState struct {
	FuelFrames  float32
	Warmup      float32
	Instability float32
}

func (w *World) beginTeamPowerStep(delta time.Duration) {
	if w == nil {
		return
	}
	dt := float32(delta.Seconds())
	if w.teamPowerBudget == nil {
		w.teamPowerBudget = map[TeamID]float32{}
	} else {
		for team := range w.teamPowerBudget {
			delete(w.teamPowerBudget, team)
		}
	}
	if w.teamPowerStates == nil {
		w.teamPowerStates = map[TeamID]*teamPowerState{}
	}
	if w.powerNetStates == nil {
		w.powerNetStates = map[int32]*powerNetState{}
	} else {
		for pos := range w.powerNetStates {
			delete(w.powerNetStates, pos)
		}
	}
	if w.powerNetByPos == nil {
		w.powerNetByPos = map[int32]int32{}
	}
	if w.powerRequested == nil {
		w.powerRequested = map[int32]float32{}
	} else {
		for pos := range w.powerRequested {
			delete(w.powerRequested, pos)
		}
	}
	if w.powerSupplied == nil {
		w.powerSupplied = map[int32]float32{}
	} else {
		for pos := range w.powerSupplied {
			delete(w.powerSupplied, pos)
		}
	}
	for team := range w.teamPowerStates {
		st := w.teamPowerStateLocked(team)
		st.Stored = 0
		st.Capacity = 0
		st.Produced = 0
		st.Consumed = 0
	}
	if dt <= 0 {
		return
	}
	w.ensurePowerNetsLocked()
	w.rebuildPowerNetStatesLocked()
	for _, pos := range w.powerTilePositions {
		if pos < 0 || w.model == nil || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Team == 0 || tile.Block == 0 {
			continue
		}
		if _, ok := w.powerNetStateForPosLocked(pos); !ok {
			continue
		}
		w.producePowerForBuildingLocked(pos, tile, dt)
	}
	w.stepPowerDiodesLocked()
	w.stepPowerVoidsLocked()
}

func (w *World) invalidatePowerNetsLocked() {
	if w == nil {
		return
	}
	w.powerNetDirty = true
}

func (w *World) ensurePowerNetsLocked() {
	if w == nil {
		return
	}
	if w.powerNetByPos == nil {
		w.powerNetByPos = map[int32]int32{}
	}
	if !w.powerNetDirty {
		return
	}
	w.buildPowerNetsLocked()
	w.powerNetDirty = false
}

func (w *World) endTeamPowerStep() {
	if w == nil {
		return
	}
	seenStorage := map[int32]struct{}{}
	for _, net := range w.powerNetStates {
		if net == nil {
			continue
		}
		if net.Capacity <= 0 {
			net.Budget = 0
		} else {
			net.Budget = minf(maxf(net.Budget, 0), net.Capacity)
		}
		remaining := net.Budget
		for i, ref := range net.Storage {
			seenStorage[ref.Pos] = struct{}{}
			share := float32(0)
			if net.Capacity > 0 {
				if i == len(net.Storage)-1 {
					share = remaining
				} else {
					share = net.Budget * (ref.Capacity / net.Capacity)
					if share > remaining {
						share = remaining
					}
					remaining -= share
				}
			}
			w.powerStorageState[ref.Pos] = share
		}
		st := w.teamPowerStateLocked(net.Team)
		st.Stored += net.Budget
		st.Capacity += net.Capacity
		st.Produced += net.Produced
		st.Consumed += net.Consumed
		w.teamPowerBudget[net.Team] += net.Budget
	}
	for pos := range w.powerStorageState {
		if _, ok := seenStorage[pos]; !ok {
			delete(w.powerStorageState, pos)
		}
	}
}

func (w *World) teamPowerStateLocked(team TeamID) *teamPowerState {
	if team == 0 {
		return &teamPowerState{}
	}
	if w.teamPowerStates == nil {
		w.teamPowerStates = map[TeamID]*teamPowerState{}
	}
	st := w.teamPowerStates[team]
	if st == nil {
		st = &teamPowerState{}
		w.teamPowerStates[team] = st
	}
	return st
}

func (w *World) powerNetStateForPosLocked(pos int32) (*powerNetState, bool) {
	if w == nil {
		return nil, false
	}
	netID, ok := w.powerNetByPos[pos]
	if !ok {
		return nil, false
	}
	net, ok := w.powerNetStates[netID]
	return net, ok && net != nil
}

func (w *World) addPowerBudgetLocked(pos int32, amount float32) {
	if amount <= 0 {
		return
	}
	net, ok := w.powerNetStateForPosLocked(pos)
	if !ok {
		return
	}
	net.Produced += amount
	net.Budget += amount
}

func (w *World) consumePowerAtLocked(pos int32, team TeamID, amount float32) float32 {
	if amount <= 0 {
		return 0
	}
	if w.powerRequested == nil {
		w.powerRequested = map[int32]float32{}
	}
	w.powerRequested[pos] += amount
	if rules := w.rulesMgr.Get(); rules != nil && rules.teamInfiniteResources(team) {
		st := w.teamPowerStateLocked(team)
		st.Consumed += amount
		if w.powerSupplied == nil {
			w.powerSupplied = map[int32]float32{}
		}
		w.powerSupplied[pos] += amount
		return amount
	}
	net, ok := w.powerNetStateForPosLocked(pos)
	if !ok {
		return 0
	}
	got := minf(net.Budget, amount)
	if got <= 0 {
		return 0
	}
	net.Budget -= got
	net.Consumed += got
	if got > 0 {
		if w.powerSupplied == nil {
			w.powerSupplied = map[int32]float32{}
		}
		w.powerSupplied[pos] += got
	}
	return got
}

func (w *World) refundPowerAtLocked(pos int32, amount float32) {
	if amount <= 0 {
		return
	}
	net, ok := w.powerNetStateForPosLocked(pos)
	if !ok {
		return
	}
	net.Budget += amount
	net.Consumed = maxf(0, net.Consumed-amount)
	if supplied := w.powerSupplied[pos] - amount; supplied > 0 {
		w.powerSupplied[pos] = supplied
	} else {
		delete(w.powerSupplied, pos)
	}
}

func (w *World) requirePowerAtLocked(pos int32, team TeamID, amount float32) bool {
	if amount <= 0 {
		return true
	}
	got := w.consumePowerAtLocked(pos, team, amount)
	if got+0.0001 >= amount {
		return true
	}
	w.refundPowerAtLocked(pos, got)
	return false
}

func powerStorageCapacityByBlockName(name string) float32 {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "battery":
		return 4000
	case "battery-large":
		return 50000
	case "beam-node":
		return 1000
	case "beam-tower":
		return 40000
	default:
		return 0
	}
}

func (w *World) producePowerForBuildingLocked(pos int32, tile *Tile, dt float32) {
	if w == nil || tile == nil || tile.Build == nil || tile.Build.Team == 0 || dt <= 0 {
		return
	}
	name := w.blockNameByID(int16(tile.Block))
	switch name {
	case "solar-panel":
		w.addPowerBudgetLocked(pos, 0.12*dt)
	case "solar-panel-large":
		w.addPowerBudgetLocked(pos, 1.6*dt)
	case "thermal-generator":
		if eff := w.thermalGenerationEfficiencyLocked(tile); eff > 0 {
			w.addPowerBudgetLocked(pos, 1.8*eff*dt)
		}
	case "combustion-generator":
		_ = w.runFueledGeneratorLocked(pos, tile, dt, 1.0, 120, []generatorFuelOption{
			{Item: pyratiteItemID, DurationMul: 3},
			{Item: coalItemID, DurationMul: 1},
			{Item: sporePodItemID, DurationMul: 1},
		}, nil)
	case "steam-generator":
		_ = w.runFueledGeneratorLocked(pos, tile, dt, 5.5, 90, []generatorFuelOption{
			{Item: pyratiteItemID, DurationMul: 3},
			{Item: coalItemID, DurationMul: 1},
			{Item: sporePodItemID, DurationMul: 1},
		}, func(build *Building, seconds float32) bool {
			return consumeBuildingLiquidLocked(build, waterLiquidID, 0.1*seconds)
		})
	case "differential-generator":
		_ = w.runFueledGeneratorLocked(pos, tile, dt, 18, 220, []generatorFuelOption{
			{Item: pyratiteItemID, DurationMul: 1},
		}, func(build *Building, seconds float32) bool {
			return consumeBuildingLiquidLocked(build, cryofluidLiquidID, 0.1*seconds)
		})
	case "rtg-generator":
		_ = w.runFueledGeneratorLocked(pos, tile, dt, 4.5, 60*14, []generatorFuelOption{
			{Item: phaseFabricItemID, DurationMul: 15},
			{Item: thoriumItemID, DurationMul: 1},
			{Item: legacyThoriumItemID, DurationMul: 1},
		}, nil)
	case "thorium-reactor":
		fuel := itemAmountOneOf(tile.Build, thoriumItemID, legacyThoriumItemID)
		if fuel > 0 {
			fullness := clampf(float32(fuel)/30, 0, 1)
			w.addPowerBudgetLocked(pos, 15*fullness*dt)
		}
	case "impact-reactor":
		_ = w.runImpactReactorLocked(pos, tile, dt)
	case "turbine-condenser":
		eff := w.sumFloorAttributeLocked(tile, "steam")
		if eff <= 0 {
			return
		}
		w.addPowerBudgetLocked(pos, (3.0/9.0)*eff*dt)
		w.addGeneratorLiquidOutputLocked(pos, tile, waterLiquidID, (5.0/60.0)/9.0, dt*60, eff)
	case "chemical-combustion-chamber":
		_ = w.runLiquidGeneratorLocked(pos, tile, dt, 550.0/60.0, []LiquidStack{
			{Liquid: ozoneLiquidID, Amount: 2.0 / 60.0},
			{Liquid: arkyciteLiquidID, Amount: 40.0 / 60.0},
		}, nil)
	case "pyrolysis-generator":
		_ = w.runLiquidGeneratorLocked(pos, tile, dt, 1400.0/60.0, []LiquidStack{
			{Liquid: slagLiquidID, Amount: 20.0 / 60.0},
			{Liquid: arkyciteLiquidID, Amount: 40.0 / 60.0},
		}, &LiquidStack{Liquid: waterLiquidID, Amount: 20.0 / 60.0})
	case "flux-reactor":
		_ = w.runFluxReactorLocked(pos, tile, dt)
	case "neoplasia-reactor":
		_ = w.runNeoplasiaReactorLocked(pos, tile, dt)
	case "power-source":
		w.addPowerBudgetLocked(pos, (1000000.0/60.0)*dt)
	}
}

type generatorFuelOption struct {
	Item        ItemID
	DurationMul float32
}

func (w *World) runFueledGeneratorLocked(pos int32, tile *Tile, dt, powerPerSec, durationFrames float32, fuels []generatorFuelOption, liquidReq func(build *Building, seconds float32) bool) bool {
	if w == nil || tile == nil || tile.Build == nil || tile.Build.Team == 0 || dt <= 0 || powerPerSec <= 0 || durationFrames <= 0 {
		return false
	}
	st := w.powerGeneratorStateLocked(pos)
	deltaFrames := dt * 60
	if st.FuelFrames <= 0 {
		if item, mul, ok := consumeFirstGeneratorFuelLocked(tile.Build, fuels); ok {
			_ = item
			if mul <= 0 {
				mul = 1
			}
			st.FuelFrames = durationFrames * mul
		}
	}
	if st.FuelFrames <= 0 {
		return false
	}
	if liquidReq != nil && !liquidReq(tile.Build, dt) {
		return false
	}
	w.addPowerBudgetLocked(pos, powerPerSec*dt)
	st.FuelFrames = maxf(0, st.FuelFrames-deltaFrames)
	return true
}

func (w *World) runImpactReactorLocked(pos int32, tile *Tile, dt float32) bool {
	if w == nil || tile == nil || tile.Build == nil || tile.Build.Team == 0 || dt <= 0 {
		return false
	}
	const (
		itemDurationFrames = 140.0
		inputPowerPerSec   = 25.0
		powerPerSecond     = 130.0
		warmupInRate       = 0.001
		warmupOutRate      = 0.01
	)
	st := w.powerGeneratorStateLocked(pos)
	deltaFrames := dt * 60
	active := false
	if st.FuelFrames > 0 || tile.Build.ItemAmount(blastCompoundItemID) > 0 {
		if consumeBuildingLiquidLocked(tile.Build, cryofluidLiquidID, 0.25*dt) {
			if got := w.consumePowerAtLocked(pos, tile.Build.Team, inputPowerPerSec*dt); got >= inputPowerPerSec*dt {
				if st.FuelFrames <= 0 && removeOneItemOfLocked(tile.Build, blastCompoundItemID) {
					st.FuelFrames = itemDurationFrames
				}
				if st.FuelFrames > 0 {
					active = true
					st.FuelFrames = maxf(0, st.FuelFrames-deltaFrames)
				}
			} else {
				w.refundPowerAtLocked(pos, got)
			}
		}
	}
	if active {
		st.Warmup = lerpDeltaf(st.Warmup, 1, warmupInRate, deltaFrames)
		if st.Warmup >= 0.999 {
			st.Warmup = 1
		}
	} else {
		st.Warmup = lerpDeltaf(st.Warmup, 0, warmupOutRate, deltaFrames)
	}
	production := pow5f(st.Warmup)
	if production <= 0 {
		return false
	}
	w.addPowerBudgetLocked(pos, powerPerSecond*production*dt)
	return true
}

func (w *World) runLiquidGeneratorLocked(pos int32, tile *Tile, dt, powerPerSec float32, inputs []LiquidStack, output *LiquidStack) bool {
	if w == nil || tile == nil || tile.Build == nil || tile.Build.Team == 0 || dt <= 0 || powerPerSec <= 0 {
		return false
	}
	deltaFrames := dt * 60
	if !canConsumeLiquidStacksLocked(tile.Build, inputs, deltaFrames) {
		return false
	}
	if !consumeLiquidStacksLocked(tile.Build, inputs, deltaFrames) {
		return false
	}
	w.addPowerBudgetLocked(pos, powerPerSec*dt)
	if output != nil && output.Amount > 0 {
		w.addGeneratorLiquidOutputLocked(pos, tile, output.Liquid, output.Amount, deltaFrames, 1)
	}
	return true
}

func (w *World) runFluxReactorLocked(pos int32, tile *Tile, dt float32) bool {
	if w == nil || tile == nil || tile.Build == nil || tile.Build.Team == 0 || dt <= 0 {
		return false
	}
	const (
		maxHeat           = 150.0
		powerPerSecond    = 18000.0 / 60.0
		cyanogenPerSecond = 9.0 / 60.0
		unstablePerSecond = 1.0 / 3.0
		stablePerSecond   = 0.5
		warmupPerSecond   = 0.1 * 60.0
	)
	st := w.powerGeneratorStateLocked(pos)
	heat := w.crafterReceivedHeatLocked(pos, tile)
	target := clampf(heat/maxHeat, 0, 1)
	if target <= 0 {
		st.Warmup = approachf(st.Warmup, 0, warmupPerSecond*dt)
		st.Instability = approachf(st.Instability, 0, stablePerSecond*dt)
		return false
	}
	required := cyanogenPerSecond * dt * target
	coolantRatio := float32(1)
	if required > 0.000001 {
		available := tile.Build.LiquidAmount(cyanogenLiquidID)
		coolantRatio = clampf(available/required, 0, 1)
		if coolantRatio > 0 {
			_ = consumeBuildingLiquidLocked(tile.Build, cyanogenLiquidID, required*coolantRatio)
		}
	}
	production := target * coolantRatio
	st.Warmup = lerpDeltaf(st.Warmup, condf(production > 0, 1, 0), 0.1, dt*60)
	if coolantRatio >= 0.99999 {
		st.Instability = approachf(st.Instability, 0, stablePerSecond*dt)
	} else {
		st.Instability = approachf(st.Instability, 1, unstablePerSecond*(1-coolantRatio)*dt)
	}
	if production <= 0 {
		return false
	}
	w.addPowerBudgetLocked(pos, powerPerSecond*production*dt)
	return true
}

func (w *World) runNeoplasiaReactorLocked(pos int32, tile *Tile, dt float32) bool {
	if w == nil || tile == nil || tile.Build == nil || tile.Build.Team == 0 || dt <= 0 {
		return false
	}
	const (
		itemDurationFrames = 60 * 3
		powerPerSecond     = 140.0
		heatOutput         = 60.0
		heatWarmupRate     = 0.15
		liquidCapacity     = 80.0
	)
	st := w.powerGeneratorStateLocked(pos)
	deltaFrames := dt * 60

	if st.FuelFrames <= 0 {
		if !hasRequiredItemsLocked(tile.Build, []ItemStack{{Item: phaseFabricItemID, Amount: 1}}) {
			w.updateGeneratorHeatStateLocked(pos, 0, heatWarmupRate, deltaFrames)
			return false
		}
		if !canConsumeLiquidStacksLocked(tile.Build, []LiquidStack{
			{Liquid: arkyciteLiquidID, Amount: 80.0 / 60.0},
			{Liquid: waterLiquidID, Amount: 10.0 / 60.0},
		}, deltaFrames) {
			w.updateGeneratorHeatStateLocked(pos, 0, heatWarmupRate, deltaFrames)
			return false
		}
		removeItemStacksLocked(tile.Build, []ItemStack{{Item: phaseFabricItemID, Amount: 1}})
		st.FuelFrames = itemDurationFrames
		w.emitEffectLocked("neoplasiasmoke", float32(tile.X*8+4), float32(tile.Y*8+4), 0)
	}

	active := false
	if st.FuelFrames > 0 {
		if consumeLiquidStacksLocked(tile.Build, []LiquidStack{
			{Liquid: arkyciteLiquidID, Amount: 80.0 / 60.0},
			{Liquid: waterLiquidID, Amount: 10.0 / 60.0},
		}, deltaFrames) {
			w.addPowerBudgetLocked(pos, powerPerSecond*dt)
			w.addGeneratorLiquidOutputLocked(pos, tile, neoplasmLiquidID, 20.0/60.0, deltaFrames, 1)
			active = true
		}
		st.FuelFrames = maxf(0, st.FuelFrames-deltaFrames)
	}

	w.updateGeneratorHeatStateLocked(pos, condf(active, heatOutput, 0), heatWarmupRate, deltaFrames)
	if tile.Build.LiquidAmount(neoplasmLiquidID) >= liquidCapacity-0.01 {
		w.explodePowerGeneratorLocked(tile.X, tile.Y, pos, tile.Team, 9, 2000, "explosionreactorneoplasm")
		return false
	}
	return active
}

func (w *World) explodePowerGeneratorLocked(x, y int, pos int32, team TeamID, radius int, damage float32, effectName string) {
	if w == nil || w.model == nil {
		return
	}
	rules := w.rulesMgr.Get()
	if rules != nil && !rules.ReactorExplosions {
		if pos >= 0 && int(pos) < len(w.model.Tiles) {
			w.queueBrokenBuildPlanLocked(pos, &w.model.Tiles[pos])
			w.destroyTileLocked(&w.model.Tiles[pos], team, 0)
		}
		return
	}
	if effectName != "" {
		w.emitEffectLocked(effectName, float32(x*8+4), float32(y*8+4), 0)
	}
	for ty := y - radius; ty <= y+radius; ty++ {
		for tx := x - radius; tx <= x+radius; tx++ {
			if !w.model.InBounds(tx, ty) {
				continue
			}
			dx := tx - x
			dy := ty - y
			if dx*dx+dy*dy > radius*radius {
				continue
			}
			tpos := int32(ty*w.model.Width + tx)
			_ = w.applyDamageToBuilding(tpos, damage)
		}
	}
	if pos >= 0 && int(pos) < len(w.model.Tiles) {
		w.queueBrokenBuildPlanLocked(pos, &w.model.Tiles[pos])
		w.destroyTileLocked(&w.model.Tiles[pos], team, 0)
	}
}

func (w *World) updateGeneratorHeatStateLocked(pos int32, target, rate, deltaFrames float32) {
	if w == nil {
		return
	}
	if rate <= 0 {
		rate = 0.15
	}
	value := approachf(w.heatStates[pos], target, rate*deltaFrames)
	if value <= 0.0001 {
		delete(w.heatStates, pos)
		return
	}
	w.heatStates[pos] = value
}

func (w *World) addGeneratorLiquidOutputLocked(pos int32, tile *Tile, liquid LiquidID, amountPerFrame, deltaFrames, efficiency float32) {
	if w == nil || tile == nil || tile.Build == nil || amountPerFrame <= 0 || deltaFrames <= 0 || efficiency <= 0 {
		return
	}
	capacity := w.liquidCapacityForBlockLocked(tile)
	if capacity <= 0 {
		return
	}
	amount := amountPerFrame * deltaFrames * efficiency
	if amount <= 0 {
		return
	}
	space := capacity - tile.Build.LiquidAmount(liquid)
	if space < 0 {
		space = 0
	}
	if amount > space {
		amount = space
	}
	if amount > 0 {
		tile.Build.AddLiquid(liquid, amount)
	}
	_ = w.dumpLiquidLocked(pos, tile, liquid, 2)
}

func (w *World) powerGeneratorStateLocked(pos int32) *powerGeneratorState {
	if w.powerGeneratorState == nil {
		w.powerGeneratorState = map[int32]*powerGeneratorState{}
	}
	st := w.powerGeneratorState[pos]
	if st == nil {
		st = &powerGeneratorState{}
		w.powerGeneratorState[pos] = st
	}
	return st
}

func consumeFirstGeneratorFuelLocked(build *Building, fuels []generatorFuelOption) (ItemID, float32, bool) {
	if build == nil {
		return 0, 0, false
	}
	for _, fuel := range fuels {
		if fuel.Item < 0 {
			continue
		}
		if build.ItemAmount(fuel.Item) <= 0 {
			continue
		}
		if build.RemoveItem(fuel.Item, 1) {
			return fuel.Item, fuel.DurationMul, true
		}
	}
	return 0, 0, false
}

func removeOneItemOfLocked(build *Building, ids ...ItemID) bool {
	if build == nil {
		return false
	}
	for _, id := range ids {
		if build.ItemAmount(id) <= 0 {
			continue
		}
		if build.RemoveItem(id, 1) {
			return true
		}
	}
	return false
}

func itemAmountOneOf(build *Building, ids ...ItemID) int32 {
	if build == nil {
		return 0
	}
	var total int32
	for _, id := range ids {
		total += build.ItemAmount(id)
	}
	return total
}

func consumeBuildingLiquidLocked(build *Building, liquid LiquidID, amount float32) bool {
	if build == nil || amount <= 0 {
		return false
	}
	for i := range build.Liquids {
		if build.Liquids[i].Liquid != liquid {
			continue
		}
		if build.Liquids[i].Amount+0.0001 < amount {
			return false
		}
		remove := amount
		if build.Liquids[i].Amount < remove {
			remove = build.Liquids[i].Amount
		}
		build.Liquids[i].Amount -= remove
		if build.Liquids[i].Amount <= 0.0001 {
			build.Liquids = append(build.Liquids[:i], build.Liquids[i+1:]...)
		}
		return true
	}
	return false
}

func condf(ok bool, yes, no float32) float32 {
	if ok {
		return yes
	}
	return no
}

func lerpDeltaf(from, to, alphaPerFrame, deltaFrames float32) float32 {
	if deltaFrames <= 0 || alphaPerFrame <= 0 {
		return from
	}
	alpha := clampf(alphaPerFrame, 0, 1)
	t := 1 - float32(math.Pow(float64(1-alpha), float64(deltaFrames)))
	return from + (to-from)*t
}

func pow5f(v float32) float32 {
	sq := v * v
	return sq * sq * v
}

func (w *World) thermalGenerationEfficiencyLocked(tile *Tile) float32 {
	if w == nil || w.model == nil || tile == nil {
		return 0
	}
	size := w.blockSizeForTileLocked(tile)
	low, high := blockFootprintRange(size)
	best := float32(0)
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			x := tile.X + dx
			y := tile.Y + dy
			if !w.model.InBounds(x, y) {
				continue
			}
			cell := &w.model.Tiles[y*w.model.Width+x]
			best = maxf(best, thermalContentEfficiencyByName(w.blockNameByID(int16(cell.Floor))))
			best = maxf(best, thermalContentEfficiencyByName(w.blockNameByID(int16(cell.Overlay))))
		}
	}
	return best
}

func thermalContentEfficiencyByName(name string) float32 {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "hotrock", "magmarock", "molten-slag", "lava", "basalt-vent", "yellow-stone-vent", "arkyic-vent":
		return 1
	default:
		return 0
	}
}

func (w *World) buildPowerNetsLocked() {
	if w == nil || w.model == nil {
		return
	}
	for pos := range w.powerNetByPos {
		delete(w.powerNetByPos, pos)
	}
	relevant := map[int32]*Tile{}
	positions := make([]int32, 0, len(w.powerTilePositions))
	for _, pos := range w.powerTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile == nil || tile.Build == nil || tile.Block == 0 || tile.Build.Team == 0 {
			continue
		}
		relevant[pos] = tile
		positions = append(positions, pos)
	}
	adj := map[int32][]int32{}
	for _, pos := range positions {
		tile := relevant[pos]
		if tile == nil || isPowerDiodeBlockName(w.blockNameByID(int16(tile.Block))) {
			continue
		}
		adj[pos] = adj[pos]
	}
	for _, pos := range positions {
		tile := relevant[pos]
		if tile == nil {
			continue
		}
		for _, edge := range blockEdgeOffsets(w.blockSizeForTileLocked(tile)) {
			otherPos, ok := w.buildingOccupyingCellLocked(tile.X+edge[0], tile.Y+edge[1])
			if !ok || otherPos == pos {
				continue
			}
			otherTile, ok := relevant[otherPos]
			if !ok || otherTile == nil || !w.powerAdjacentConnectedLocked(pos, tile, otherPos, otherTile) {
				continue
			}
			adj[pos] = appendUniquePowerPos(adj[pos], otherPos)
			adj[otherPos] = appendUniquePowerPos(adj[otherPos], pos)
		}
	}
	for _, pos := range positions {
		tile := relevant[pos]
		if tile == nil || !isPowerNodeBlockName(w.blockNameByID(int16(tile.Block))) {
			continue
		}
		for _, target := range w.powerNodeLinks[pos] {
			if !w.powerNodeLinkValidLocked(pos, target) {
				continue
			}
			targetTile, ok := relevant[target]
			if !ok || targetTile == nil || isPowerDiodeBlockName(w.blockNameByID(int16(targetTile.Block))) {
				continue
			}
			adj[pos] = appendUniquePowerPos(adj[pos], target)
			adj[target] = appendUniquePowerPos(adj[target], pos)
		}
	}
	for _, pos := range positions {
		tile := relevant[pos]
		if tile == nil || !isBeamNodeBlockName(w.blockNameByID(int16(tile.Block))) {
			continue
		}
		for _, target := range w.beamNodeTargetsLocked(pos, tile) {
			targetTile, ok := relevant[target]
			if !ok || targetTile == nil || isPowerDiodeBlockName(w.blockNameByID(int16(targetTile.Block))) {
				continue
			}
			adj[pos] = appendUniquePowerPos(adj[pos], target)
			adj[target] = appendUniquePowerPos(adj[target], pos)
		}
	}
	visited := map[int32]struct{}{}
	for _, start := range positions {
		tile := relevant[start]
		if tile == nil || isPowerDiodeBlockName(w.blockNameByID(int16(tile.Block))) {
			continue
		}
		if _, ok := visited[start]; ok {
			continue
		}
		queue := []int32{start}
		visited[start] = struct{}{}
		component := make([]int32, 0, 8)
		graphID := start
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			component = append(component, cur)
			if cur < graphID {
				graphID = cur
			}
			for _, next := range adj[cur] {
				if _, ok := visited[next]; ok {
					continue
				}
				visited[next] = struct{}{}
				queue = append(queue, next)
			}
		}
		for _, pos := range component {
			w.powerNetByPos[pos] = graphID
		}
	}
}

func (w *World) rebuildPowerNetStatesLocked() {
	if w == nil || w.model == nil {
		return
	}
	for _, pos := range w.powerTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if !w.isPowerRelevantBuildingLocked(tile) || isPowerDiodeBlockName(w.blockNameByID(int16(tile.Block))) {
			continue
		}
		netID, ok := w.powerNetByPos[pos]
		if !ok {
			continue
		}
		net := w.powerNetStates[netID]
		if net == nil {
			net = &powerNetState{Team: tile.Build.Team}
			w.powerNetStates[netID] = net
		}
		capacity := powerStorageCapacityByBlockName(w.blockNameByID(int16(tile.Block)))
		if capacity <= 0 {
			continue
		}
		net.Budget += clampf(w.powerStorageState[pos], 0, capacity)
		net.Capacity += capacity
		net.Storage = append(net.Storage, powerStorageRef{Pos: pos, Capacity: capacity})
	}
}

func (w *World) stepPowerDiodesLocked() {
	if w == nil || w.model == nil {
		return
	}
	for _, pos := range w.powerDiodeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Team == 0 || w.blockNameByID(int16(tile.Block)) != "diode" {
			continue
		}
		dx, dy := dirDelta(tile.Rotation)
		backPos, backOK := w.buildingOccupyingCellLocked(tile.X-dx, tile.Y-dy)
		frontPos, frontOK := w.buildingOccupyingCellLocked(tile.X+dx, tile.Y+dy)
		if !backOK || !frontOK || backPos == frontPos {
			continue
		}
		if backPos < 0 || frontPos < 0 || int(backPos) >= len(w.model.Tiles) || int(frontPos) >= len(w.model.Tiles) {
			continue
		}
		backTile := &w.model.Tiles[backPos]
		frontTile := &w.model.Tiles[frontPos]
		if backTile.Build == nil || frontTile.Build == nil {
			continue
		}
		if backTile.Build.Team != tile.Build.Team || frontTile.Build.Team != tile.Build.Team {
			continue
		}
		backNet, backLinked := w.powerNetStateForPosLocked(backPos)
		frontNet, frontLinked := w.powerNetStateForPosLocked(frontPos)
		if !backLinked || !frontLinked || backNet == nil || frontNet == nil || backNet == frontNet {
			continue
		}
		if backNet.Capacity <= 0 || frontNet.Capacity <= 0 {
			continue
		}
		backStored := minf(maxf(backNet.Budget, 0), backNet.Capacity)
		frontStored := minf(maxf(frontNet.Budget, 0), frontNet.Capacity)
		if backStored/backNet.Capacity <= frontStored/frontNet.Capacity {
			continue
		}
		targetPercent := (frontStored + backStored) / (frontNet.Capacity + backNet.Capacity)
		amount := (targetPercent*frontNet.Capacity - frontStored) / 2
		amount = clampf(amount, 0, frontNet.Capacity-frontStored)
		if amount <= 0 {
			continue
		}
		backNet.Budget = maxf(0, backNet.Budget-amount)
		frontNet.Budget += amount
	}
}

func (w *World) stepPowerVoidsLocked() {
	if w == nil || w.model == nil {
		return
	}
	drained := make(map[int32]struct{})
	for _, pos := range w.powerVoidTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Team == 0 || w.blockNameByID(int16(tile.Block)) != "power-void" {
			continue
		}
		net, ok := w.powerNetStateForPosLocked(pos)
		if !ok || net == nil {
			continue
		}
		netID := w.powerNetByPos[pos]
		if _, seen := drained[netID]; seen {
			continue
		}
		if net.Budget > 0 {
			net.Consumed += net.Budget
			net.Budget = 0
		}
		drained[netID] = struct{}{}
	}
}

func (w *World) isPowerRelevantBuildingLocked(tile *Tile) bool {
	if w == nil || tile == nil || tile.Build == nil || tile.Block == 0 || tile.Build.Team == 0 {
		return false
	}
	name := w.blockNameByID(int16(tile.Block))
	if isPowerNodeBlockName(name) || isPowerDiodeBlockName(name) || isPowerProducerBlockName(name) || isPowerConsumerBlockName(name) || name == "power-void" || powerStorageCapacityByBlockName(name) > 0 {
		return true
	}
	if prof, ok := w.buildingProfilesByName[name]; ok {
		return prof.PowerCapacity > 0 || prof.PowerPerShot > 0 || prof.PowerRegen > 0
	}
	return false
}

func isPowerProducerBlockName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "solar-panel", "solar-panel-large", "thermal-generator", "combustion-generator", "steam-generator", "differential-generator", "rtg-generator", "thorium-reactor", "impact-reactor", "turbine-condenser", "chemical-combustion-chamber", "pyrolysis-generator", "flux-reactor", "neoplasia-reactor", "power-source":
		return true
	default:
		return false
	}
}

func isPowerConsumerBlockName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "ground-factory", "air-factory", "naval-factory",
		"additive-reconstructor", "multiplicative-reconstructor", "exponential-reconstructor", "tetrative-reconstructor",
		"tank-refabricator", "ship-refabricator", "mech-refabricator", "prime-refabricator",
		"small-deconstructor", "deconstructor", "payload-deconstructor",
		"surge-conveyor", "surge-router", "mass-driver", "payload-mass-driver", "large-payload-mass-driver",
		"laser-drill", "blast-drill", "impact-drill", "eruption-drill", "plasma-bore", "large-plasma-bore",
		"repair-point", "repair-turret", "unit-repair-tower",
		"rotary-pump", "impulse-pump", "water-extractor", "oil-extractor",
		"multi-press", "silicon-smelter", "silicon-arc-furnace", "electrolyzer", "atmospheric-concentrator",
		"oxidation-chamber", "electric-heater", "silicon-crucible", "carbide-crucible", "surge-crucible",
		"cyanogen-synthesizer", "phase-synthesizer", "kiln", "plastanium-compressor", "phase-weaver", "surge-smelter",
		"cryofluid-mixer", "pyratite-mixer", "blast-mixer", "melter", "separator", "disassembler", "slag-centrifuge",
		"pulverizer", "coal-centrifuge", "spore-press", "cultivator", "vent-condenser", "incinerator":
		return true
	default:
		return false
	}
}

func (w *World) blockConsumesPowerLocked(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if isPowerConsumerBlockName(name) || powerStorageCapacityByBlockName(name) > 0 || name == "power-void" {
		return true
	}
	if prof, ok := w.buildingProfilesByName[name]; ok {
		return prof.PowerCapacity > 0 || prof.PowerPerShot > 0 || prof.PowerRegen > 0
	}
	return false
}

func (w *World) blockOutputsPowerLocked(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	return isPowerProducerBlockName(name) || powerStorageCapacityByBlockName(name) > 0
}

func (w *World) blockConductivePowerLocked(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "shielded-wall", "surge-conveyor", "surge-router":
		return true
	default:
		return false
	}
}

func (w *World) isPowerNodeLinkTargetLocked(tile *Tile) bool {
	if w == nil || tile == nil || tile.Build == nil || tile.Block == 0 {
		return false
	}
	name := w.blockNameByID(int16(tile.Block))
	return isPowerNodeBlockName(name) || w.blockOutputsPowerLocked(name) || w.blockConsumesPowerLocked(name)
}

func isAutoLinkPowerBuildingName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if isPowerNodeBlockName(name) || isPowerProducerBlockName(name) || isPowerConsumerBlockName(name) || powerStorageCapacityByBlockName(name) > 0 {
		return true
	}
	return false
}

func isPowerNodeBlockName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "power-node", "power-node-large", "surge-tower", "beam-link", "power-source":
		return true
	default:
		return false
	}
}

func isBeamNodeBlockName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "beam-node", "beam-tower":
		return true
	default:
		return false
	}
}

func beamNodeRangeByBlockName(name string) int {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "beam-node":
		return 10
	case "beam-tower":
		return 23
	default:
		return 0
	}
}

func isBeamInsulatedBlockName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "plastanium-wall", "plastanium-wall-large":
		return true
	default:
		return false
	}
}

func (w *World) powerNodeInsulatedLocked(pos, target int32) bool {
	if w == nil || w.model == nil || pos < 0 || target < 0 || int(pos) >= len(w.model.Tiles) || int(target) >= len(w.model.Tiles) {
		return false
	}
	fromTile := &w.model.Tiles[pos]
	targetTile := &w.model.Tiles[target]
	x0, y0 := fromTile.X, fromTile.Y
	x1, y1 := targetTile.X, targetTile.Y
	dx := absInt(x1 - x0)
	dy := absInt(y1 - y0)
	sx := 1
	if x0 > x1 {
		sx = -1
	}
	sy := 1
	if y0 > y1 {
		sy = -1
	}
	err := dx - dy
	for {
		if x0 == x1 && y0 == y1 {
			break
		}
		if (x0 != fromTile.X || y0 != fromTile.Y) && (x0 != targetTile.X || y0 != targetTile.Y) {
			if otherPos, ok := w.buildingOccupyingCellLocked(x0, y0); ok && otherPos != pos && otherPos != target && otherPos >= 0 && int(otherPos) < len(w.model.Tiles) {
				if isBeamInsulatedBlockName(w.blockNameByID(int16(w.model.Tiles[otherPos].Block))) {
					return true
				}
			}
		}
		e2 := err * 2
		if e2 > -dy {
			err -= dy
			x0 += sx
		}
		if e2 < dx {
			err += dx
			y0 += sy
		}
	}
	return false
}

func (w *World) beamNodeTargetsLocked(pos int32, tile *Tile) []int32 {
	if w == nil || w.model == nil || tile == nil || tile.Build == nil {
		return nil
	}
	name := w.blockNameByID(int16(tile.Block))
	rng := beamNodeRangeByBlockName(name)
	if rng <= 0 {
		return nil
	}
	size := w.blockSizeForTileLocked(tile)
	offset := size / 2
	out := make([]int32, 0, 4)
	for dir := 0; dir < 4; dir++ {
		dx, dy := dirDelta(int8(dir))
		for step := 1 + offset; step <= rng+offset; step++ {
			x := tile.X + dx*step
			y := tile.Y + dy*step
			if !w.model.InBounds(x, y) {
				break
			}
			otherPos, ok := w.buildingOccupyingCellLocked(x, y)
			if !ok || otherPos == pos || otherPos < 0 || int(otherPos) >= len(w.model.Tiles) {
				continue
			}
			other := &w.model.Tiles[otherPos]
			if other.Build == nil || other.Block == 0 {
				continue
			}
			otherName := w.blockNameByID(int16(other.Block))
			if isBeamInsulatedBlockName(otherName) {
				break
			}
			if other.Build.Team != tile.Build.Team {
				continue
			}
			if isPowerNodeBlockName(otherName) {
				continue
			}
			if !w.isPowerRelevantBuildingLocked(other) {
				continue
			}
			out = appendUniquePowerPos(out, otherPos)
			break
		}
	}
	return out
}

func (w *World) powerAdjacentConnectedLocked(pos int32, tile *Tile, otherPos int32, other *Tile) bool {
	if w == nil || tile == nil || other == nil || tile.Build == nil || other.Build == nil || tile.Block == 0 || other.Block == 0 {
		return false
	}
	if pos == otherPos || tile.Build.Team == 0 || tile.Build.Team != other.Build.Team {
		return false
	}
	fromName := w.blockNameByID(int16(tile.Block))
	otherName := w.blockNameByID(int16(other.Block))
	if isPowerDiodeBlockName(fromName) || isPowerDiodeBlockName(otherName) {
		return false
	}
	if w.blockConsumesPowerLocked(fromName) && w.blockConsumesPowerLocked(otherName) &&
		!w.blockOutputsPowerLocked(fromName) && !w.blockOutputsPowerLocked(otherName) &&
		!w.blockConductivePowerLocked(fromName) && !w.blockConductivePowerLocked(otherName) {
		return false
	}
	return true
}

func isPowerDiodeBlockName(name string) bool {
	return strings.ToLower(strings.TrimSpace(name)) == "diode"
}

func powerNodeRulesByBlockName(name string) (int, float32, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "power-node":
		return 10, 6, true
	case "power-node-large":
		return 15, 15, true
	case "surge-tower":
		return 2, 40, true
	case "beam-link":
		return 1, 500, true
	case "power-source":
		return 100, 6, true
	default:
		return 0, 0, false
	}
}

func appendUniquePowerPos(values []int32, value int32) []int32 {
	for _, cur := range values {
		if cur == value {
			return values
		}
	}
	return append(values, value)
}

func removePowerPos(values []int32, value int32) ([]int32, bool) {
	for i, cur := range values {
		if cur != value {
			continue
		}
		out := append([]int32(nil), values[:i]...)
		out = append(out, values[i+1:]...)
		return out, true
	}
	return values, false
}

func (w *World) persistPowerNodeConfigLocked(pos int32) {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 {
		return
	}
	if !isPowerNodeBlockName(w.blockNameByID(int16(tile.Block))) {
		return
	}
	if normalized, ok := w.normalizedBuildingConfigLocked(pos); ok {
		w.storeBuildingConfigLocked(tile, normalized)
		return
	}
	tile.Build.Config = nil
}

func (w *World) addPowerLinkOneWayLocked(pos, target int32) bool {
	if w == nil {
		return false
	}
	links := w.powerNodeLinks[pos]
	for _, cur := range links {
		if cur == target {
			return false
		}
	}
	w.powerNodeLinks[pos] = append(links, target)
	w.persistPowerNodeConfigLocked(pos)
	return true
}

func (w *World) removePowerLinkOneWayLocked(pos, target int32) bool {
	if w == nil {
		return false
	}
	links, removed := removePowerPos(w.powerNodeLinks[pos], target)
	if !removed {
		return false
	}
	if len(links) == 0 {
		delete(w.powerNodeLinks, pos)
	} else {
		w.powerNodeLinks[pos] = links
	}
	w.persistPowerNodeConfigLocked(pos)
	return true
}

func (w *World) powerNodeCanAcceptLinkLocked(pos, target int32) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	for _, cur := range w.powerNodeLinks[pos] {
		if cur == target {
			return true
		}
	}
	name := w.blockNameByID(int16(w.model.Tiles[pos].Block))
	if !isPowerNodeBlockName(name) {
		return true
	}
	if strings.EqualFold(strings.TrimSpace(name), "beam-link") {
		return true
	}
	maxNodes, _, ok := powerNodeRulesByBlockName(name)
	if !ok {
		return false
	}
	return len(w.powerNodeLinks[pos]) < maxNodes
}

func (w *World) addPowerLinkPairLocked(pos, target int32) bool {
	if !w.powerNodeLinkValidLocked(pos, target) {
		return false
	}
	if !w.powerNodeCanAcceptLinkLocked(pos, target) || !w.powerNodeCanAcceptLinkLocked(target, pos) {
		return false
	}
	changed := w.addPowerLinkOneWayLocked(pos, target)
	changed = w.addPowerLinkOneWayLocked(target, pos) || changed
	if changed {
		w.invalidatePowerNetsLocked()
	}
	return changed
}

func (w *World) removePowerLinkPairLocked(pos, target int32) bool {
	if w == nil {
		return false
	}
	changed := w.removePowerLinkOneWayLocked(pos, target)
	changed = w.removePowerLinkOneWayLocked(target, pos) || changed
	if changed {
		w.invalidatePowerNetsLocked()
	}
	return changed
}

func (w *World) clearPowerLinksForBuildingLocked(pos int32) bool {
	if w == nil {
		return false
	}
	related := make([]int32, 0, len(w.powerNodeLinks[pos]))
	seen := map[int32]struct{}{}
	for _, target := range w.powerNodeLinks[pos] {
		if _, ok := seen[target]; ok {
			continue
		}
		seen[target] = struct{}{}
		related = append(related, target)
	}
	for otherPos, otherLinks := range w.powerNodeLinks {
		if otherPos == pos {
			continue
		}
		for _, link := range otherLinks {
			if link != pos {
				continue
			}
			if _, ok := seen[otherPos]; ok {
				break
			}
			seen[otherPos] = struct{}{}
			related = append(related, otherPos)
			break
		}
	}
	changed := false
	for _, otherPos := range related {
		changed = w.removePowerLinkPairLocked(pos, otherPos) || changed
	}
	return changed
}

func (w *World) configurePowerNodeLinksLocked(pos int32, targets []int32) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	name := w.blockNameByID(int16(w.model.Tiles[pos].Block))
	maxNodes, _, ok := powerNodeRulesByBlockName(name)
	if !ok {
		return false
	}
	out := make([]int32, 0, len(targets))
	seen := map[int32]struct{}{}
	for _, raw := range targets {
		target, ok := w.resolveAbsoluteLinkTargetLocked(raw)
		if !ok {
			continue
		}
		if _, dup := seen[target]; dup {
			continue
		}
		if !w.powerNodeLinkValidLocked(pos, target) {
			continue
		}
		out = append(out, target)
		seen[target] = struct{}{}
		if len(out) >= maxNodes {
			break
		}
	}
	current := append([]int32(nil), w.powerNodeLinks[pos]...)
	want := make(map[int32]struct{}, len(out))
	for _, target := range out {
		want[target] = struct{}{}
	}
	for _, target := range current {
		if _, keep := want[target]; keep {
			continue
		}
		w.removePowerLinkPairLocked(pos, target)
	}
	for _, target := range out {
		w.addPowerLinkPairLocked(pos, target)
	}
	if len(out) == 0 {
		w.persistPowerNodeConfigLocked(pos)
	}
	return true
}

func (w *World) autoLinkPowerNodeLocked(pos int32) []int32 {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return nil
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 || len(w.powerNodeLinks[pos]) > 0 {
		return nil
	}
	maxNodes, _, ok := powerNodeRulesByBlockName(w.blockNameByID(int16(tile.Block)))
	if !ok || maxNodes <= 0 {
		return nil
	}

	w.ensurePowerNetsLocked()

	type powerNodeCandidate struct {
		pos      int32
		isNode   bool
		dist2    float32
		netID    int32
		hasNetID bool
	}

	seenNet := map[int32]struct{}{}
	if netID, ok := w.powerNetByPos[pos]; ok {
		seenNet[netID] = struct{}{}
	}
	for _, edge := range blockEdgeOffsets(w.blockSizeForTileLocked(tile)) {
		otherPos, ok := w.buildingOccupyingCellLocked(tile.X+edge[0], tile.Y+edge[1])
		if !ok || otherPos == pos || otherPos < 0 || int(otherPos) >= len(w.model.Tiles) {
			continue
		}
		if netID, ok := w.powerNetByPos[otherPos]; ok {
			seenNet[netID] = struct{}{}
		}
	}

	candidates := make([]powerNodeCandidate, 0, maxNodes)
	for _, otherPos := range w.teamPowerTiles[tile.Build.Team] {
		if otherPos == pos || otherPos < 0 || int(otherPos) >= len(w.model.Tiles) {
			continue
		}
		other := &w.model.Tiles[otherPos]
		if other.Build == nil || other.Block == 0 || other.Build.Team != tile.Build.Team {
			continue
		}
		if !w.powerNodeLinkValidLocked(pos, otherPos) || w.powerNodeAdjacentLocked(pos, otherPos) || w.powerNodeInsulatedLocked(pos, otherPos) {
			continue
		}
		otherName := w.blockNameByID(int16(other.Block))
		if isPowerNodeBlockName(otherName) {
			if otherMaxNodes, _, ok := powerNodeRulesByBlockName(otherName); ok && len(w.powerNodeLinks[otherPos]) >= otherMaxNodes {
				continue
			}
		}
		netID, hasNetID := w.powerNetByPos[otherPos]
		if hasNetID {
			if _, dup := seenNet[netID]; dup {
				continue
			}
		}
		dx := float32(tile.X - other.X)
		dy := float32(tile.Y - other.Y)
		candidates = append(candidates, powerNodeCandidate{
			pos:      otherPos,
			isNode:   isPowerNodeBlockName(otherName),
			dist2:    dx*dx + dy*dy,
			netID:    netID,
			hasNetID: hasNetID,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].isNode != candidates[j].isNode {
			return candidates[i].isNode
		}
		if candidates[i].dist2 != candidates[j].dist2 {
			return candidates[i].dist2 < candidates[j].dist2
		}
		return candidates[i].pos < candidates[j].pos
	})

	links := make([]int32, 0, maxNodes)
	for _, cand := range candidates {
		if cand.hasNetID {
			if _, dup := seenNet[cand.netID]; dup {
				continue
			}
			seenNet[cand.netID] = struct{}{}
		}
		links = append(links, cand.pos)
		if len(links) >= maxNodes {
			break
		}
	}
	if len(links) == 0 {
		return nil
	}
	added := make([]int32, 0, len(links))
	for _, target := range links {
		if w.addPowerLinkPairLocked(pos, target) {
			added = append(added, target)
		}
	}
	return added
}

type powerAutoLinkChange struct {
	nodePos   int32
	targetPos int32
}

func (w *World) autoLinkNearbyPowerNodesForBuildingLocked(pos int32) []powerAutoLinkChange {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return nil
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 || tile.Build.Team == 0 {
		return nil
	}
	if !w.isAutoLinkPowerBuildingLocked(tile) {
		return nil
	}

	w.ensurePowerNetsLocked()

	placedName := w.blockNameByID(int16(tile.Block))
	placedConsumes := w.blockConsumesPowerLocked(placedName)
	placedOutputs := w.blockOutputsPowerLocked(placedName)

	type candidate struct {
		pos      int32
		dist2    float32
		netID    int32
		hasNetID bool
	}
	candidates := make([]candidate, 0, 8)
	for _, otherPos := range w.teamPowerNodeTiles[tile.Build.Team] {
		if otherPos == pos || otherPos < 0 || int(otherPos) >= len(w.model.Tiles) {
			continue
		}
		other := &w.model.Tiles[otherPos]
		if other.Build == nil || other.Block == 0 || other.Build.Team != tile.Build.Team {
			continue
		}
		otherName := w.blockNameByID(int16(other.Block))
		if !isPowerNodeBlockName(otherName) {
			continue
		}
		maxNodes, _, ok := powerNodeRulesByBlockName(otherName)
		if !ok || len(w.powerNodeLinks[otherPos]) >= maxNodes {
			continue
		}
		if !w.powerNodeLinkValidLocked(otherPos, pos) || w.powerNodeAdjacentLocked(otherPos, pos) || w.powerNodeInsulatedLocked(otherPos, pos) {
			continue
		}
		dx := float32(tile.X - other.X)
		dy := float32(tile.Y - other.Y)
		netID, hasNetID := w.powerNetByPos[otherPos]
		candidates = append(candidates, candidate{
			pos:      otherPos,
			dist2:    dx*dx + dy*dy,
			netID:    netID,
			hasNetID: hasNetID,
		})
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].dist2 != candidates[j].dist2 {
			return candidates[i].dist2 < candidates[j].dist2
		}
		return candidates[i].pos < candidates[j].pos
	})

	seenNet := map[int32]struct{}{}
	adjacentPowered := make([]int32, 0, 8)
	for _, edge := range blockEdgeOffsets(w.blockSizeForTileLocked(tile)) {
		otherPos, ok := w.buildingOccupyingCellLocked(tile.X+edge[0], tile.Y+edge[1])
		if !ok || otherPos == pos || otherPos < 0 || int(otherPos) >= len(w.model.Tiles) {
			continue
		}
		other := &w.model.Tiles[otherPos]
		if other.Build == nil || other.Block == 0 || other.Build.Team != tile.Build.Team {
			continue
		}
		otherName := w.blockNameByID(int16(other.Block))
		otherConsumes := w.blockConsumesPowerLocked(otherName)
		otherOutputs := w.blockOutputsPowerLocked(otherName)
		if placedConsumes && otherConsumes && !placedOutputs && !otherOutputs {
			continue
		}
		adjacentPowered = appendUniquePowerPos(adjacentPowered, otherPos)
		if netID, ok := w.powerNetByPos[otherPos]; ok {
			seenNet[netID] = struct{}{}
		}
	}
	changed := make([]powerAutoLinkChange, 0, len(candidates))
	for _, cand := range candidates {
		skip := false
		for _, adjacentPos := range adjacentPowered {
			for _, link := range w.powerNodeLinks[cand.pos] {
				if link == adjacentPos {
					skip = true
					break
				}
			}
			if skip {
				break
			}
			for _, link := range w.powerNodeLinks[adjacentPos] {
				if link == cand.pos {
					skip = true
					break
				}
			}
			if skip {
				break
			}
		}
		if skip {
			continue
		}
		if cand.hasNetID {
			if _, dup := seenNet[cand.netID]; dup {
				continue
			}
		}
		if w.addPowerLinkPairLocked(cand.pos, pos) {
			changed = append(changed, powerAutoLinkChange{nodePos: cand.pos, targetPos: pos})
		}
		if cand.hasNetID {
			seenNet[cand.netID] = struct{}{}
		}
	}
	return changed
}

func (w *World) isAutoLinkPowerBuildingLocked(tile *Tile) bool {
	if tile == nil || tile.Block == 0 {
		return false
	}
	return isAutoLinkPowerBuildingName(w.blockNameByID(int16(tile.Block)))
}

func (w *World) togglePowerNodeLinkLocked(pos, rawTarget int32) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	target, ok := w.resolveAbsoluteLinkTargetLocked(rawTarget)
	if !ok || !w.powerNodeLinkValidLocked(pos, target) {
		return false
	}
	for _, cur := range w.powerNodeLinks[pos] {
		if cur == target {
			return w.removePowerLinkPairLocked(pos, target)
		}
	}
	return w.addPowerLinkPairLocked(pos, target)
}

func (w *World) powerNodeLinkValidLocked(pos, target int32) bool {
	if w == nil || w.model == nil || pos < 0 || target < 0 || int(pos) >= len(w.model.Tiles) || int(target) >= len(w.model.Tiles) || pos == target {
		return false
	}
	fromTile := &w.model.Tiles[pos]
	targetTile := &w.model.Tiles[target]
	if fromTile.Build == nil || targetTile.Build == nil || fromTile.Build.Team == 0 || targetTile.Build.Team == 0 {
		return false
	}
	if fromTile.Build.Team != targetTile.Build.Team {
		return false
	}
	if !w.isPowerNodeLinkTargetLocked(targetTile) || isPowerDiodeBlockName(w.blockNameByID(int16(targetTile.Block))) {
		return false
	}
	return w.powerNodeCanReachLocked(pos, target)
}

func (w *World) powerNodeAdjacentLocked(pos, target int32) bool {
	if w == nil || w.model == nil || pos < 0 || target < 0 || int(pos) >= len(w.model.Tiles) || int(target) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	for _, edge := range blockEdgeOffsets(w.blockSizeForTileLocked(tile)) {
		otherPos, ok := w.buildingOccupyingCellLocked(tile.X+edge[0], tile.Y+edge[1])
		if ok && otherPos == target {
			return true
		}
	}
	return false
}

func (w *World) powerNodeCanReachLocked(pos, target int32) bool {
	if w == nil || w.model == nil || pos < 0 || target < 0 || int(pos) >= len(w.model.Tiles) || int(target) >= len(w.model.Tiles) {
		return false
	}
	fromTile := &w.model.Tiles[pos]
	targetTile := &w.model.Tiles[target]
	if _, rng, ok := powerNodeRulesByBlockName(w.blockNameByID(int16(fromTile.Block))); ok && w.powerNodeRangeReachesLocked(fromTile, targetTile, rng) {
		return true
	}
	if _, rng, ok := powerNodeRulesByBlockName(w.blockNameByID(int16(targetTile.Block))); ok && w.powerNodeRangeReachesLocked(targetTile, fromTile, rng) {
		return true
	}
	return false
}

func (w *World) powerNodeRangeReachesLocked(fromTile, targetTile *Tile, rangeBlocks float32) bool {
	if w == nil || fromTile == nil || targetTile == nil || rangeBlocks <= 0 {
		return false
	}
	sx := float32(fromTile.X) + 0.5
	sy := float32(fromTile.Y) + 0.5
	size := float32(w.blockSizeForTileLocked(targetTile))
	half := size / 2
	minX := float32(targetTile.X) + 0.5 - half
	maxX := float32(targetTile.X) + 0.5 + half
	minY := float32(targetTile.Y) + 0.5 - half
	maxY := float32(targetTile.Y) + 0.5 + half
	nx := clampf(sx, minX, maxX)
	ny := clampf(sy, minY, maxY)
	dx := sx - nx
	dy := sy - ny
	return dx*dx+dy*dy <= rangeBlocks*rangeBlocks+0.0001
}
