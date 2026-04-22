package world

import (
	"math"
	"sort"
	"strings"

	"mdt-server/internal/protocol"
)

type reconstructorState struct {
	Progress   float32
	CommandPos *protocol.Vec2
	Command    *protocol.UnitCommand
}

type payloadDeconstructorState struct {
	Deconstructing *payloadData
	Accum          []float32
	Progress       float32
	Time           float32
	SpeedScl       float32
	PayRotation    float32
}

type reconstructorProfile struct {
	ConstructTimeFrames float32
	PowerPerSecond      float32
	InputItems          []ItemStack
	InputLiquid         *LiquidStack
	Upgrades            map[string]string
}

type payloadDeconstructorProfile struct {
	MaxPayloadSize   int
	DeconstructSpeed float32
	ItemCapacity     int32
	PowerPerSecond   float32
	PayloadSpeed     float32
}

var reconstructorProfilesByBlockName = map[string]reconstructorProfile{
	"additive-reconstructor": {
		ConstructTimeFrames: 60 * 10,
		PowerPerSecond:      3,
		InputItems: []ItemStack{
			{Item: siliconItemID, Amount: 40},
			{Item: graphiteItemID, Amount: 40},
		},
		Upgrades: map[string]string{
			"nova":    "pulsar",
			"dagger":  "mace",
			"crawler": "atrax",
			"flare":   "horizon",
			"mono":    "poly",
			"risso":   "minke",
			"retusa":  "oxynoe",
		},
	},
	"multiplicative-reconstructor": {
		ConstructTimeFrames: 60 * 30,
		PowerPerSecond:      6,
		InputItems: []ItemStack{
			{Item: siliconItemID, Amount: 130},
			{Item: titaniumItemID, Amount: 80},
			{Item: metaglassItemID, Amount: 40},
		},
		Upgrades: map[string]string{
			"horizon": "zenith",
			"mace":    "fortress",
			"poly":    "mega",
			"minke":   "bryde",
			"pulsar":  "quasar",
			"atrax":   "spiroct",
			"oxynoe":  "cyerce",
		},
	},
	"exponential-reconstructor": {
		ConstructTimeFrames: 60 * 60 * 1.5,
		PowerPerSecond:      13,
		InputItems: []ItemStack{
			{Item: siliconItemID, Amount: 850},
			{Item: titaniumItemID, Amount: 750},
			{Item: plastaniumItemID, Amount: 650},
		},
		InputLiquid: &LiquidStack{Liquid: cryofluidLiquidID, Amount: 1},
		Upgrades: map[string]string{
			"zenith":   "antumbra",
			"spiroct":  "arkyid",
			"fortress": "scepter",
			"bryde":    "sei",
			"mega":     "quad",
			"quasar":   "vela",
			"cyerce":   "aegires",
		},
	},
	"tetrative-reconstructor": {
		ConstructTimeFrames: 60 * 60 * 4,
		PowerPerSecond:      25,
		InputItems: []ItemStack{
			{Item: siliconItemID, Amount: 1000},
			{Item: plastaniumItemID, Amount: 600},
			{Item: surgeAlloyItemID, Amount: 500},
			{Item: phaseFabricItemID, Amount: 350},
		},
		InputLiquid: &LiquidStack{Liquid: cryofluidLiquidID, Amount: 3},
		Upgrades: map[string]string{
			"antumbra": "eclipse",
			"arkyid":   "toxopid",
			"scepter":  "reign",
			"sei":      "omura",
			"quad":     "oct",
			"vela":     "corvus",
			"aegires":  "navanax",
		},
	},
	"tank-refabricator": {
		ConstructTimeFrames: 60 * 30,
		PowerPerSecond:      3,
		InputItems: []ItemStack{
			{Item: siliconItemID, Amount: 40},
			{Item: tungstenItemID, Amount: 30},
		},
		InputLiquid: &LiquidStack{Liquid: hydrogenLiquidID, Amount: 3.0 / 60.0},
		Upgrades: map[string]string{
			"stell": "locus",
		},
	},
	"ship-refabricator": {
		ConstructTimeFrames: 60 * 50,
		PowerPerSecond:      2.5,
		InputItems: []ItemStack{
			{Item: siliconItemID, Amount: 60},
			{Item: tungstenItemID, Amount: 40},
		},
		InputLiquid: &LiquidStack{Liquid: hydrogenLiquidID, Amount: 3.0 / 60.0},
		Upgrades: map[string]string{
			"elude": "avert",
		},
	},
	"mech-refabricator": {
		ConstructTimeFrames: 60 * 45,
		PowerPerSecond:      2.5,
		InputItems: []ItemStack{
			{Item: siliconItemID, Amount: 50},
			{Item: tungstenItemID, Amount: 40},
		},
		InputLiquid: &LiquidStack{Liquid: hydrogenLiquidID, Amount: 3.0 / 60.0},
		Upgrades: map[string]string{
			"merui": "cleroi",
		},
	},
	"prime-refabricator": {
		ConstructTimeFrames: 60 * 60,
		PowerPerSecond:      4.5,
		InputItems: []ItemStack{
			{Item: thoriumItemID, Amount: 80},
			{Item: siliconItemID, Amount: 100},
		},
		InputLiquid: &LiquidStack{Liquid: nitrogenLiquidID, Amount: 10.0 / 60.0},
		Upgrades: map[string]string{
			"locus":  "precept",
			"cleroi": "anthicus",
			"avert":  "obviate",
		},
	},
}

var payloadDeconstructorProfilesByBlockName = map[string]payloadDeconstructorProfile{
	"small-deconstructor": {
		MaxPayloadSize:   4,
		DeconstructSpeed: 3,
		ItemCapacity:     100,
		PowerPerSecond:   1,
		PayloadSpeed:     1,
	},
	"deconstructor": {
		MaxPayloadSize:   4,
		DeconstructSpeed: 6,
		ItemCapacity:     250,
		PowerPerSecond:   3,
		PayloadSpeed:     1,
	},
	"payload-deconstructor": {
		MaxPayloadSize:   4,
		DeconstructSpeed: 6,
		ItemCapacity:     250,
		PowerPerSecond:   3,
		PayloadSpeed:     1,
	},
}

var baseUnitRequirementsByName = map[string][]ItemStack{
	"dagger":  {{Item: siliconItemID, Amount: 10}, {Item: leadItemID, Amount: 10}},
	"crawler": {{Item: siliconItemID, Amount: 8}, {Item: coalItemID, Amount: 10}},
	"nova":    {{Item: siliconItemID, Amount: 30}, {Item: leadItemID, Amount: 20}, {Item: titaniumItemID, Amount: 20}},
	"flare":   {{Item: siliconItemID, Amount: 15}},
	"mono":    {{Item: siliconItemID, Amount: 30}, {Item: leadItemID, Amount: 15}},
	"risso":   {{Item: siliconItemID, Amount: 20}, {Item: metaglassItemID, Amount: 35}},
	"retusa":  {{Item: siliconItemID, Amount: 15}, {Item: titaniumItemID, Amount: 20}},
	"stell":   {{Item: berylliumItemID, Amount: 40}, {Item: siliconItemID, Amount: 50}},
	"elude":   {{Item: graphiteItemID, Amount: 50}, {Item: siliconItemID, Amount: 70}},
	"merui":   {{Item: berylliumItemID, Amount: 50}, {Item: siliconItemID, Amount: 70}},
}

var itemCostByID = map[ItemID]float32{
	copperItemID:        0.5,
	leadItemID:          0.7,
	metaglassItemID:     1.5,
	graphiteItemID:      1,
	titaniumItemID:      1,
	thoriumItemID:       1.1,
	legacyThoriumItemID: 1.1,
	siliconItemID:       0.8,
	plastaniumItemID:    1.3,
	phaseFabricItemID:   1.3,
	surgeAlloyItemID:    1.2,
	berylliumItemID:     1.2,
	tungstenItemID:      1.5,
	oxideItemID:         1.2,
	carbideItemID:       1.4,
}

func isReconstructorBlockName(name string) bool {
	_, ok := reconstructorProfilesByBlockName[strings.ToLower(strings.TrimSpace(name))]
	return ok
}

func reconstructorProfileByName(name string) (reconstructorProfile, bool) {
	prof, ok := reconstructorProfilesByBlockName[strings.ToLower(strings.TrimSpace(name))]
	return prof, ok
}

func payloadDeconstructorProfileByName(name string) (payloadDeconstructorProfile, bool) {
	prof, ok := payloadDeconstructorProfilesByBlockName[strings.ToLower(strings.TrimSpace(name))]
	return prof, ok
}

func payloadBlockMoveFrames(size int, speed float32) float32 {
	if size <= 0 || speed <= 0 {
		return 1
	}
	frames := float32(size*8/2) / speed
	if frames < 1 {
		return 1
	}
	return frames
}

func cloneCommandPos(v *protocol.Vec2) *protocol.Vec2 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneUnitCommand(v *protocol.UnitCommand) *protocol.UnitCommand {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func (w *World) reconstructorStateLocked(pos int32) *reconstructorState {
	if st, ok := w.reconstructorStates[pos]; ok {
		copyState := st
		copyState.CommandPos = cloneCommandPos(st.CommandPos)
		copyState.Command = cloneUnitCommand(st.Command)
		w.reconstructorStates[pos] = copyState
		return &copyState
	}
	st := reconstructorState{}
	w.reconstructorStates[pos] = st
	return &st
}

func (w *World) payloadDeconstructorStateLocked(pos int32) *payloadDeconstructorState {
	if st, ok := w.payloadDeconstructorStates[pos]; ok && st != nil {
		return st
	}
	st := &payloadDeconstructorState{}
	w.payloadDeconstructorStates[pos] = st
	return st
}

func (w *World) storeReconstructorStateLocked(pos int32, st *reconstructorState) {
	if st == nil {
		delete(w.reconstructorStates, pos)
		return
	}
	copyState := *st
	copyState.CommandPos = cloneCommandPos(st.CommandPos)
	copyState.Command = cloneUnitCommand(st.Command)
	w.reconstructorStates[pos] = copyState
}

func unitCommandControllerState(commandPos *protocol.Vec2, command *protocol.UnitCommand) *protocol.ControllerState {
	if commandPos == nil && command == nil {
		return nil
	}
	ctrl := &protocol.ControllerState{Type: protocol.ControllerCommand9}
	if command != nil {
		ctrl.Command.CommandID = int8(command.ID)
	}
	if commandPos != nil {
		ctrl.Command.HasPos = true
		ctrl.Command.TargetPos = *commandPos
	}
	return ctrl
}

func (w *World) unitCommandStateAtLocked(pos int32) (*protocol.Vec2, *protocol.UnitCommand) {
	if st, ok := w.reconstructorStates[pos]; ok {
		return cloneCommandPos(st.CommandPos), cloneUnitCommand(st.Command)
	}
	return w.unitFactoryCommandStateLocked(pos)
}

func payloadRotationDegrees(payload *payloadData, fallback int8) float32 {
	if payload == nil {
		return float32(fallback) * 90
	}
	if payload.UnitState != nil && payload.UnitState.Rotation != 0 {
		return payload.UnitState.Rotation
	}
	return float32(payload.Rotation) * 90
}

func shortestAngleDelta(from, to float32) float32 {
	delta := float32(math.Mod(float64(to-from), 360))
	if delta > 180 {
		delta -= 360
	}
	if delta < -180 {
		delta += 360
	}
	return delta
}

func angleProgress(from, to, progress float32) float32 {
	return from + shortestAngleDelta(from, to)*clampf(progress, 0, 1)
}

func (w *World) newCommandedUnitPayloadLocked(typeID int16, team TeamID, x, y, rotation float32, commandPos *protocol.Vec2, command *protocol.UnitCommand) *payloadData {
	if typeID <= 0 || team == 0 {
		return nil
	}
	unit := w.newProducedUnitEntityLocked(typeID, team, x, y, rotation)
	entity := w.entitySyncUnitLocked(unit, nil, unitCommandControllerState(commandPos, command))
	writer := protocol.NewWriter()
	if err := writer.WriteBool(true); err != nil {
		return nil
	}
	if err := writer.WriteByte(protocol.PayloadUnit); err != nil {
		return nil
	}
	if err := writer.WriteByte(entity.ClassID()); err != nil {
		return nil
	}
	if err := entity.WriteEntity(writer); err != nil {
		return nil
	}
	clone := cloneRawEntity(unit)
	return &payloadData{
		Kind:       payloadKindUnit,
		UnitTypeID: typeID,
		Rotation:   buildRotationFromDegrees(rotation),
		Serialized: append([]byte(nil), writer.Bytes()...),
		Health:     unit.Health,
		MaxHealth:  unit.MaxHealth,
		UnitState:  &clone,
	}
}

func cloneItemStacks(src []ItemStack) []ItemStack {
	if len(src) == 0 {
		return nil
	}
	return append([]ItemStack(nil), src...)
}

func mergeItemStacks(src ...[]ItemStack) []ItemStack {
	if len(src) == 0 {
		return nil
	}
	acc := make(map[ItemID]int32)
	for _, stacks := range src {
		for _, stack := range stacks {
			if stack.Amount <= 0 {
				continue
			}
			acc[stack.Item] += stack.Amount
		}
	}
	if len(acc) == 0 {
		return nil
	}
	keys := make([]int, 0, len(acc))
	for item, amount := range acc {
		if amount > 0 {
			keys = append(keys, int(item))
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Ints(keys)
	out := make([]ItemStack, 0, len(keys))
	for _, raw := range keys {
		item := ItemID(raw)
		out = append(out, ItemStack{Item: item, Amount: acc[item]})
	}
	return out
}

func itemStacksBuildTimeFrames(stacks []ItemStack) float32 {
	if len(stacks) == 0 {
		return 0
	}
	var total float32
	for _, stack := range stacks {
		if stack.Amount <= 0 {
			continue
		}
		total += itemCostByID[stack.Item] * float32(stack.Amount)
	}
	if total <= 0 {
		return 1
	}
	return total
}

func rawBaseUnitRequirementsByName(name string) []ItemStack {
	return cloneItemStacks(baseUnitRequirementsByName[normalizeUnitName(name)])
}

func (w *World) reconstructorUpgradeSourceLocked(targetName string) (string, []ItemStack, bool) {
	targetName = normalizeUnitName(targetName)
	if targetName == "" {
		return "", nil, false
	}
	for _, prof := range reconstructorProfilesByBlockName {
		for from, to := range prof.Upgrades {
			if normalizeUnitName(to) == targetName {
				return normalizeUnitName(from), cloneItemStacks(prof.InputItems), true
			}
		}
	}
	return "", nil, false
}

func (w *World) unitRequirementsAndBuildTimeByNameLocked(name string, seen map[string]bool) ([]ItemStack, float32) {
	name = normalizeUnitName(name)
	if name == "" || seen[name] {
		return nil, 0
	}
	seen[name] = true
	if base := rawBaseUnitRequirementsByName(name); len(base) > 0 {
		return base, itemStacksBuildTimeFrames(base)
	}
	if prev, upgradeCost, ok := w.reconstructorUpgradeSourceLocked(name); ok {
		prevReqs, _ := w.unitRequirementsAndBuildTimeByNameLocked(prev, seen)
		total := mergeItemStacks(upgradeCost, prevReqs)
		return total, itemStacksBuildTimeFrames(total)
	}
	return nil, 0
}

func (w *World) unitRequirementsAndBuildTimeLocked(typeID int16) ([]ItemStack, float32) {
	if typeID <= 0 {
		return nil, 0
	}
	return w.unitRequirementsAndBuildTimeByNameLocked(normalizeUnitName(w.unitNamesByID[typeID]), map[string]bool{})
}

func (w *World) payloadRequirementsLocked(payload *payloadData) []ItemStack {
	if payload == nil {
		return nil
	}
	switch payload.Kind {
	case payloadKindBlock:
		return cloneItemStacks(w.buildCostByName(w.blockNameByID(payload.BlockID)))
	case payloadKindUnit:
		reqs, _ := w.unitRequirementsAndBuildTimeLocked(payload.UnitTypeID)
		return reqs
	default:
		return nil
	}
}

func (w *World) payloadBuildTimeFramesLocked(payload *payloadData) float32 {
	if payload == nil {
		return 0
	}
	switch payload.Kind {
	case payloadKindBlock:
		name := w.blockNameByID(payload.BlockID)
		if t, ok := w.blockBuildTimesByName[name]; ok && t > 0 {
			return t * 60
		}
		return itemStacksBuildTimeFrames(w.payloadRequirementsLocked(payload))
	case payloadKindUnit:
		_, total := w.unitRequirementsAndBuildTimeLocked(payload.UnitTypeID)
		return total
	default:
		return 0
	}
}

func (w *World) payloadDeconstructionMultiplierLocked(payload *payloadData, team TeamID) float32 {
	if payload == nil {
		return 1
	}
	if payload.Kind == payloadKindBlock {
		rules := w.rulesMgr.Get()
		if rules != nil && rules.BuildCostMultiplier > 0 {
			return rules.BuildCostMultiplier
		}
		return 1
	}
	return w.unitCostMultiplierLocked(team)
}

func (w *World) reconstructorUpgradeTargetTypeLocked(buildPos int32, tile *Tile, payload *payloadData) (int16, bool) {
	if payload == nil || payload.Kind != payloadKindUnit || payload.UnitTypeID <= 0 || tile == nil {
		return 0, false
	}
	prof, ok := reconstructorProfileByName(w.blockNameByID(int16(tile.Block)))
	if !ok {
		return 0, false
	}
	fromName := normalizeUnitName(w.unitNamesByID[payload.UnitTypeID])
	targetName, ok := prof.Upgrades[fromName]
	if !ok {
		return 0, false
	}
	return w.resolveUnitTypeIDLocked(normalizeUnitName(targetName))
}

func (w *World) reconstructorAcceptsPayloadLocked(buildPos int32, tile *Tile, payload *payloadData, sourcePos int32, enforceInputSide bool) bool {
	if w == nil || tile == nil || tile.Build == nil || payload == nil || payload.Kind != payloadKindUnit {
		return false
	}
	if current := w.payloadStateLocked(buildPos).Payload; current != nil {
		return false
	}
	if enforceInputSide && sourcePos != buildPos {
		if side, ok := w.relativeToEdgeLocked(sourcePos, buildPos); ok && side == byte(tileRotationNorm(tile.Rotation)) {
			return false
		}
	}
	_, ok := w.reconstructorUpgradeTargetTypeLocked(buildPos, tile, payload)
	return ok
}

func (w *World) payloadDeconstructorAcceptsPayloadLocked(buildPos int32, tile *Tile, payload *payloadData) bool {
	if w == nil || tile == nil || tile.Build == nil || payload == nil {
		return false
	}
	if current := w.payloadStateLocked(buildPos).Payload; current != nil {
		return false
	}
	st := w.payloadDeconstructorStateLocked(buildPos)
	if st.Deconstructing != nil {
		return false
	}
	prof, ok := payloadDeconstructorProfileByName(w.blockNameByID(int16(tile.Block)))
	if !ok {
		return false
	}
	if len(w.payloadRequirementsLocked(payload)) == 0 {
		return false
	}
	return w.payloadSizeBlocksLocked(payload) <= prof.MaxPayloadSize
}

func (w *World) payloadVoidAcceptsPayloadLocked(buildPos int32) bool {
	return w.payloadStateLocked(buildPos).Payload == nil
}

func (w *World) reconstructorScaledInputItemsLocked(team TeamID, items []ItemStack) []ItemStack {
	if len(items) == 0 {
		return nil
	}
	mul := w.unitCostMultiplierLocked(team)
	out := make([]ItemStack, 0, len(items))
	for _, stack := range items {
		if stack.Amount <= 0 {
			continue
		}
		amount := unitFactoryScaledAmount(stack.Amount, mul)
		if amount <= 0 {
			continue
		}
		out = append(out, ItemStack{Item: stack.Item, Amount: amount})
	}
	return out
}

func (w *World) reconstructorMaximumAcceptedItemLocked(tile *Tile, item ItemID) int32 {
	if tile == nil {
		return 0
	}
	prof, ok := reconstructorProfileByName(w.blockNameByID(int16(tile.Block)))
	if !ok {
		return 0
	}
	team := tile.Team
	if tile.Build != nil && tile.Build.Team != 0 {
		team = tile.Build.Team
	}
	for _, stack := range prof.InputItems {
		if stack.Item == item {
			return unitFactoryScaledAmount(stack.Amount*2, w.unitCostMultiplierLocked(team))
		}
	}
	return 0
}

func (w *World) reconstructorItemCapacityLocked(tile *Tile) int32 {
	if tile == nil {
		return 0
	}
	prof, ok := reconstructorProfileByName(w.blockNameByID(int16(tile.Block)))
	if !ok {
		return 0
	}
	team := tile.Team
	if tile.Build != nil && tile.Build.Team != 0 {
		team = tile.Build.Team
	}
	base := int32(10)
	for _, stack := range prof.InputItems {
		if amount := stack.Amount * 2; amount > base {
			base = amount
		}
	}
	return unitFactoryScaledAmount(base, w.unitCostMultiplierLocked(team))
}

func reconstructorLiquidCapacity(prof reconstructorProfile) float32 {
	if prof.InputLiquid == nil || prof.InputLiquid.Amount <= 0 {
		return 0
	}
	return float32(math.Round(float64(prof.InputLiquid.Amount * 600)))
}

func (w *World) reconstructorAcceptsLiquidLocked(tile *Tile, liquid LiquidID) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	prof, ok := reconstructorProfileByName(w.blockNameByID(int16(tile.Block)))
	if !ok || prof.InputLiquid == nil || prof.InputLiquid.Amount <= 0 {
		return false
	}
	return prof.InputLiquid.Liquid == liquid && w.liquidCanStoreLocked(tile, liquid)
}

func (w *World) configureReconstructorCommandLocked(pos int32, command *protocol.UnitCommand) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) || !isReconstructorBlockName(w.blockNameByID(int16(w.model.Tiles[pos].Block))) {
		return false
	}
	st := w.reconstructorStates[pos]
	st.Command = cloneUnitCommand(command)
	w.reconstructorStates[pos] = st
	return true
}

func (w *World) clearReconstructorCommandLocked(pos int32) bool {
	return w.configureReconstructorCommandLocked(pos, nil)
}

func (w *World) configureReconstructorCommandPosLocked(pos int32, target protocol.Vec2) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) || !isReconstructorBlockName(w.blockNameByID(int16(w.model.Tiles[pos].Block))) {
		return false
	}
	st := w.reconstructorStates[pos]
	st.CommandPos = cloneCommandPos(&target)
	w.reconstructorStates[pos] = st
	return true
}

func (w *World) reconstructorCommandStateLocked(pos int32) (*protocol.Vec2, *protocol.UnitCommand) {
	st, ok := w.reconstructorStates[pos]
	if !ok {
		return nil, nil
	}
	return cloneCommandPos(st.CommandPos), cloneUnitCommand(st.Command)
}

func (w *World) reconstructorProgressLocked(pos int32) float32 {
	st, ok := w.reconstructorStates[pos]
	if !ok {
		return 0
	}
	return maxf(st.Progress, 0)
}

func (w *World) stepPayloadVoidLocked(pos int32, tile *Tile, frames float32) {
	st := w.payloadStateLocked(pos)
	if st.Payload == nil {
		st.Move = 0
		w.syncPayloadTileLocked(tile, nil)
		return
	}
	st.Move += frames
	if st.Move < payloadBlockMoveFrames(w.blockSizeForTileLocked(tile), 1.2) {
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	w.clearPayloadLocked(pos, tile)
}

func (w *World) stepPayloadDeconstructorLocked(pos int32, tile *Tile, frames float32) {
	st := w.payloadStateLocked(pos)
	state := w.payloadDeconstructorStateLocked(pos)
	prof, ok := payloadDeconstructorProfileByName(w.blockNameByID(int16(tile.Block)))
	if !ok {
		return
	}
	for i := 0; i < 4 && totalBuildingItems(tile.Build) > 0; i++ {
		if !w.dumpSingleItemLocked(pos, tile, nil, nil) {
			break
		}
	}
	if state.Deconstructing == nil {
		state.Progress = 0
	}
	state.PayRotation = moveAngleToward(state.PayRotation, 90, 5*frames)
	if state.Deconstructing != nil {
		reqs := w.payloadRequirementsLocked(state.Deconstructing)
		if len(reqs) == 0 {
			state.Deconstructing = nil
			state.Accum = nil
			state.Progress = 0
			return
		}
		if len(state.Accum) != len(reqs) {
			state.Accum = make([]float32, len(reqs))
		}
		canProgress := totalBuildingItems(tile.Build) <= prof.ItemCapacity
		if canProgress {
			for _, v := range state.Accum {
				if v >= 1 {
					canProgress = false
					break
				}
			}
		}
		if canProgress && prof.PowerPerSecond > 0 && !w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*(frames/60)) {
			canProgress = false
		}
		if canProgress {
			buildTime := w.payloadBuildTimeFramesLocked(state.Deconstructing)
			if buildTime <= 0 {
				buildTime = 1
			}
			shift := frames * prof.DeconstructSpeed / buildTime
			realShift := minf(shift, 1-state.Progress)
			if realShift > 0 {
				state.Progress += shift
				state.Time += frames
				mul := w.payloadDeconstructionMultiplierLocked(state.Deconstructing, tile.Build.Team)
				for i := range reqs {
					state.Accum[i] += float32(reqs[i].Amount) * mul * realShift
				}
			}
		}
		targetSpeed := float32(0)
		if canProgress {
			targetSpeed = 1
		}
		state.SpeedScl += (targetSpeed - state.SpeedScl) * minf(1, 0.1*frames)
		for i := range reqs {
			space := prof.ItemCapacity - totalBuildingItems(tile.Build)
			taken := int32(state.Accum[i])
			if space <= 0 || taken <= 0 {
				continue
			}
			if taken > space {
				taken = space
			}
			tile.Build.AddItem(reqs[i].Item, taken)
			state.Accum[i] -= float32(taken)
		}
		if state.Progress >= 1 {
			finished := true
			for i := range reqs {
				if math.Abs(float64(state.Accum[i]-1)) <= 0.0001 {
					if totalBuildingItems(tile.Build) < prof.ItemCapacity {
						tile.Build.AddItem(reqs[i].Item, 1)
						state.Accum[i] = 0
					} else {
						finished = false
						break
					}
				}
			}
			if finished {
				state.Deconstructing = nil
				state.Accum = nil
				state.Progress = 0
				state.SpeedScl = 0
			}
		}
		return
	}
	if st.Payload == nil {
		st.Move = 0
		w.syncPayloadTileLocked(tile, nil)
		return
	}
	st.Move += frames
	if st.Move < payloadBlockMoveFrames(w.blockSizeForTileLocked(tile), prof.PayloadSpeed) {
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	if len(w.payloadRequirementsLocked(st.Payload)) == 0 || w.payloadSizeBlocksLocked(st.Payload) > prof.MaxPayloadSize {
		w.clearPayloadLocked(pos, tile)
		return
	}
	state.Deconstructing = func() *payloadData {
		copyPayload := clonePayloadData(*st.Payload)
		return &copyPayload
	}()
	state.Accum = make([]float32, len(w.payloadRequirementsLocked(st.Payload)))
	state.Progress = 0
	state.SpeedScl = 0
	state.PayRotation = payloadRotationDegrees(st.Payload, tile.Rotation)
	w.clearPayloadLocked(pos, tile)
}

func (w *World) stepReconstructorLocked(pos int32, tile *Tile, frames float32) {
	st := w.payloadStateLocked(pos)
	state := w.reconstructorStateLocked(pos)
	prof, ok := reconstructorProfileByName(w.blockNameByID(int16(tile.Block)))
	if !ok {
		return
	}
	if st.Payload == nil {
		state.Progress = 0
		w.storeReconstructorStateLocked(pos, state)
		st.Move = 0
		w.syncPayloadTileLocked(tile, nil)
		return
	}
	targetType, ok := w.reconstructorUpgradeTargetTypeLocked(pos, tile, st.Payload)
	if !ok {
		state.Progress = 0
		w.storeReconstructorStateLocked(pos, state)
		w.stepUnitFactoryPayloadLocked(pos, tile, frames)
		return
	}
	moveTime := w.unitBlockPayloadMoveFramesLocked(tile)
	st.Move += frames
	if st.Move < moveTime {
		w.storeReconstructorStateLocked(pos, state)
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	scaledItems := w.reconstructorScaledInputItemsLocked(tile.Build.Team, prof.InputItems)
	speedMul := w.unitBuildSpeedMultiplierLocked(tile.Build.Team, w.rulesMgr.Get())
	canProgress := speedMul > 0 && hasRequiredItemsLocked(tile.Build, scaledItems)
	if canProgress && prof.InputLiquid != nil && prof.InputLiquid.Amount > 0 {
		if !consumeBuildingLiquidLocked(tile.Build, prof.InputLiquid.Liquid, prof.InputLiquid.Amount*frames) {
			canProgress = false
		}
	}
	if canProgress && prof.PowerPerSecond > 0 && !w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*(frames/60)) {
		canProgress = false
	}
	if canProgress {
		state.Progress += frames * speedMul
	}
	if state.Progress >= prof.ConstructTimeFrames {
		if hasRequiredItemsLocked(tile.Build, scaledItems) {
			removeItemStacksLocked(tile.Build, scaledItems)
			rotation := payloadRotationDegrees(st.Payload, tile.Rotation)
			payload := w.newCommandedUnitPayloadLocked(targetType, tile.Build.Team, float32(tile.X*8+4), float32(tile.Y*8+4), rotation, state.CommandPos, state.Command)
			if payload != nil {
				st.Payload = payload
				st.Move = moveTime
				st.Work = 0
				st.Exporting = false
				w.syncPayloadTileLocked(tile, payload)
				state.Progress = float32(math.Mod(float64(state.Progress), 1))
			}
		} else {
			state.Progress = clampf(state.Progress, 0, prof.ConstructTimeFrames)
		}
	}
	w.storeReconstructorStateLocked(pos, state)
	w.syncPayloadTileLocked(tile, st.Payload)
}

func (w *World) reconstructorSyncEfficiencyLocked(pos int32, tile *Tile) float32 {
	if w == nil || tile == nil || tile.Build == nil {
		return 0
	}
	if _, ok := w.reconstructorStates[pos]; !ok {
		return 0
	}
	payload := w.payloadStateLocked(pos).Payload
	if payload == nil {
		return 0
	}
	prof, ok := reconstructorProfileByName(w.blockNameByID(int16(tile.Block)))
	if !ok {
		return 0
	}
	if _, ok := w.reconstructorUpgradeTargetTypeLocked(pos, tile, payload); !ok {
		return 0
	}
	if w.unitBuildSpeedMultiplierLocked(tile.Build.Team, w.rulesMgr.Get()) <= 0 {
		return 0
	}
	if scaled := w.reconstructorScaledInputItemsLocked(tile.Build.Team, prof.InputItems); len(scaled) > 0 && !hasRequiredItemsLocked(tile.Build, scaled) {
		return 0
	}
	if prof.InputLiquid != nil && prof.InputLiquid.Amount > 0 {
		if tile.Build.LiquidAmount(prof.InputLiquid.Liquid) < prof.InputLiquid.Amount-0.0001 {
			return 0
		}
	}
	if rules := w.rulesMgr.Get(); rules != nil && rules.teamInfiniteResources(tile.Build.Team) {
		return 1
	}
	if !w.isPowerRelevantBuildingLocked(tile) {
		return 1
	}
	return clampf(w.blockSyncPowerStatusLocked(pos, tile, w.blockNameByID(int16(tile.Block))), 0, 1)
}

func (w *World) payloadDeconstructorSyncEfficiencyLocked(pos int32, tile *Tile) float32 {
	if w == nil || tile == nil || tile.Build == nil {
		return 0
	}
	state := w.payloadDeconstructorStateLocked(pos)
	if state.Deconstructing == nil {
		return 0
	}
	if rules := w.rulesMgr.Get(); rules != nil && rules.teamInfiniteResources(tile.Build.Team) {
		return 1
	}
	return clampf(state.SpeedScl, 0, 1)
}

func (w *World) payloadInputSyncFieldsLocked(pos int32, tile *Tile, speed float32, rotation float32) (float32, float32, float32) {
	if tile == nil {
		return 0, 0, rotation
	}
	st := w.payloadStates[pos]
	if st == nil || st.Payload == nil {
		return 0, 0, rotation
	}
	moveTime := payloadBlockMoveFrames(w.blockSizeForTileLocked(tile), speed)
	if moveTime <= 0 {
		return 0, 0, rotation
	}
	progress := clampf(st.Move/moveTime, 0, 1)
	sourceDir := byte((int(st.RecDir) + 2) % 4)
	dx, dy := dirDelta(int8(sourceDir))
	dist := float32(w.blockSizeForTileLocked(tile)) * 8 / 2
	return float32(dx) * dist * (1 - progress), float32(dy) * dist * (1 - progress), rotation
}

func (w *World) payloadVoidSyncFieldsLocked(pos int32, tile *Tile) (float32, float32, float32) {
	payload := w.payloadStateLocked(pos).Payload
	return w.payloadInputSyncFieldsLocked(pos, tile, 1.2, payloadRotationDegrees(payload, tile.Rotation))
}

func (w *World) payloadDeconstructorSyncFieldsLocked(pos int32, tile *Tile) (float32, float32, float32) {
	state := w.payloadDeconstructorStateLocked(pos)
	if state.Deconstructing != nil {
		return 0, 0, state.PayRotation
	}
	payload := w.payloadStateLocked(pos).Payload
	return w.payloadInputSyncFieldsLocked(pos, tile, 1, payloadRotationDegrees(payload, tile.Rotation))
}

func (w *World) reconstructorPayloadSyncFieldsLocked(pos int32, tile *Tile) (float32, float32, float32) {
	payload := w.payloadStateLocked(pos).Payload
	if payload == nil || tile == nil {
		return 0, 0, float32(0)
	}
	if _, ok := w.reconstructorUpgradeTargetTypeLocked(pos, tile, payload); ok {
		moveTime := w.unitBlockPayloadMoveFramesLocked(tile)
		progress := clampf(w.payloadStateLocked(pos).Move/moveTime, 0, 1)
		startRotation := payloadRotationDegrees(payload, tile.Rotation)
		targetRotation := float32(tile.Rotation) * 90
		x, y, _ := w.payloadInputSyncFieldsLocked(pos, tile, 0.7, angleProgress(startRotation, targetRotation, progress))
		return x, y, angleProgress(startRotation, targetRotation, progress)
	}
	return w.unitFactoryPayloadSyncFieldsLocked(pos, tile)
}
