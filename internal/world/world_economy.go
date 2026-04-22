package world

import (
	"math"
	"strings"
	"time"

	"mdt-server/internal/protocol"
)

const unlimitedUnitCap = int32(^uint32(0) >> 1)

type unitFactoryPlan struct {
	UnitName   string
	TimeFrames float32
	Cost       []ItemStack
}

var blockCostByName = map[string][]ItemStack{
	"duo":                   {{Item: 0, Amount: 35}},
	"scatter":               {{Item: 0, Amount: 85}, {Item: 1, Amount: 45}},
	"scorch":                {{Item: 0, Amount: 25}, {Item: 1, Amount: 22}},
	"hail":                  {{Item: 0, Amount: 40}, {Item: 1, Amount: 17}},
	"lancer":                {{Item: 0, Amount: 60}, {Item: 1, Amount: 70}, {Item: 3, Amount: 30}},
	"arc":                   {{Item: 0, Amount: 50}, {Item: 1, Amount: 50}},
	"salvo":                 {{Item: 0, Amount: 100}, {Item: 1, Amount: 80}, {Item: 2, Amount: 30}},
	"mender":                {{Item: 0, Amount: 25}, {Item: 1, Amount: 30}},
	"router":                {{Item: 0, Amount: 3}},
	"junction":              {{Item: 0, Amount: 2}},
	"sorter":                {{Item: 0, Amount: 2}, {Item: 1, Amount: 2}},
	"conveyor":              {{Item: 0, Amount: 1}},
	"titanium-conveyor":     {{Item: 0, Amount: 1}, {Item: 4, Amount: 1}},
	"armored-conveyor":      {{Item: 0, Amount: 2}, {Item: 4, Amount: 2}},
	"bridge-conveyor":       {{Item: 0, Amount: 6}, {Item: 1, Amount: 6}},
	"power-node":            {{Item: 0, Amount: 1}, {Item: 1, Amount: 3}},
	"power-node-large":      {{Item: 0, Amount: 5}, {Item: 1, Amount: 10}, {Item: 4, Amount: 3}},
	"battery":               {{Item: 0, Amount: 5}, {Item: 1, Amount: 20}},
	"battery-large":         {{Item: 0, Amount: 20}, {Item: 1, Amount: 40}, {Item: 4, Amount: 20}},
	"mechanical-drill":      {{Item: 0, Amount: 12}},
	"pneumatic-drill":       {{Item: 0, Amount: 18}, {Item: 1, Amount: 10}},
	"laser-drill":           {{Item: 0, Amount: 35}, {Item: 1, Amount: 30}, {Item: 4, Amount: 20}, {Item: 3, Amount: 30}},
	"copper-wall":           {{Item: 0, Amount: 6}},
	"copper-wall-large":     {{Item: 0, Amount: 24}},
	"titanium-wall":         {{Item: 4, Amount: 6}},
	"titanium-wall-large":   {{Item: 4, Amount: 24}},
	"plastanium-wall":       {{Item: 6, Amount: 5}, {Item: 7, Amount: 2}},
	"plastanium-wall-large": {{Item: 6, Amount: 20}, {Item: 7, Amount: 8}},
	"surge-wall":            {{Item: 8, Amount: 6}},
	"surge-wall-large":      {{Item: 8, Amount: 24}},
	"ground-factory":        {{Item: 0, Amount: 60}, {Item: 1, Amount: 50}, {Item: 2, Amount: 40}},
	"air-factory":           {{Item: 0, Amount: 60}, {Item: 1, Amount: 50}, {Item: 2, Amount: 40}},
	"naval-factory":         {{Item: 0, Amount: 60}, {Item: 1, Amount: 50}, {Item: 2, Amount: 40}},
}

var unitFactoryPlansByBlockName = map[string][]unitFactoryPlan{
	"ground-factory": {
		{UnitName: "dagger", TimeFrames: 60 * 15, Cost: []ItemStack{{Item: siliconItemID, Amount: 10}, {Item: leadItemID, Amount: 10}}},
		{UnitName: "crawler", TimeFrames: 60 * 10, Cost: []ItemStack{{Item: siliconItemID, Amount: 8}, {Item: coalItemID, Amount: 10}}},
		{UnitName: "nova", TimeFrames: 60 * 40, Cost: []ItemStack{{Item: siliconItemID, Amount: 30}, {Item: leadItemID, Amount: 20}, {Item: titaniumItemID, Amount: 20}}},
	},
	"air-factory": {
		{UnitName: "flare", TimeFrames: 60 * 15, Cost: []ItemStack{{Item: siliconItemID, Amount: 15}}},
		{UnitName: "mono", TimeFrames: 60 * 35, Cost: []ItemStack{{Item: siliconItemID, Amount: 30}, {Item: leadItemID, Amount: 15}}},
	},
	"naval-factory": {
		{UnitName: "risso", TimeFrames: 60 * 45, Cost: []ItemStack{{Item: siliconItemID, Amount: 20}, {Item: metaglassItemID, Amount: 35}}},
		{UnitName: "retusa", TimeFrames: 60 * 35, Cost: []ItemStack{{Item: siliconItemID, Amount: 15}, {Item: titaniumItemID, Amount: 20}}},
	},
}

var blockUnitCapModifierByName = map[string]int32{
	"core-shard":      8,
	"core-foundation": 16,
	"core-nucleus":    24,
	"core-bastion":    15,
	"core-citadel":    15,
	"core-acropolis":  15,
}

func (w *World) stepFactoryProduction(delta time.Duration) {
	if w.model == nil {
		return
	}
	deltaFrames := float32(delta.Seconds() * 60)
	deltaSeconds := float32(delta.Seconds())
	if deltaFrames <= 0 || deltaSeconds <= 0 {
		return
	}
	rules := w.rulesMgr.Get()
	for _, pos := range w.factoryTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		t := &w.model.Tiles[pos]
		if t.Build == nil || t.Block <= 0 || t.Team == 0 {
			continue
		}
		name := w.blockNameByID(int16(t.Block))
		plans, ok := unitFactoryPlansByBlockName[name]
		if !ok {
			continue
		}
		st, hasState := w.factoryStates[pos]
		st.CurrentPlan = unitFactoryCurrentPlanIndex(st, hasState, plans)
		if st.CurrentPlan < 0 || int(st.CurrentPlan) >= len(plans) {
			st.Progress = 0
			st.UnitType = 0
			w.factoryStates[pos] = st
			continue
		}
		plan := plans[st.CurrentPlan]
		if plan.TimeFrames <= 0 {
			st.Progress = 0
			w.factoryStates[pos] = st
			continue
		}
		payloadState := w.payloadStateLocked(pos)
		if payloadState.Payload != nil {
			st.Progress = 0
			w.factoryStates[pos] = st
			continue
		}
		typeID, ok := w.resolveUnitTypeIDLocked(normalizeUnitName(plan.UnitName))
		if !ok || typeID <= 0 {
			st.Progress = 0
			st.UnitType = 0
			w.factoryStates[pos] = st
			continue
		}
		st.UnitType = typeID

		scaledCost := w.unitFactoryScaledCostLocked(t.Team, plan.Cost)
		speedMul := w.unitBuildSpeedMultiplierLocked(t.Team, rules)
		if speedMul > 0 && hasRequiredItemsLocked(t.Build, scaledCost) && w.requirePowerAtLocked(pos, t.Team, 1.2*deltaSeconds) {
			st.Progress += deltaFrames * speedMul
		}
		if st.Progress >= plan.TimeFrames {
			payload := w.newFactoryUnitPayloadLocked(t, st)
			if payload == nil {
				st.Progress = clampf(st.Progress, 0, plan.TimeFrames)
			} else {
				removeItemStacksLocked(t.Build, scaledCost)
				w.emitBlockItemSyncLocked(pos)
				st.Progress = float32(math.Mod(float64(st.Progress), 1))
				payloadState.Payload = payload
				payloadState.Move = 0
				payloadState.Work = 0
				payloadState.Exporting = false
				w.syncPayloadTileLocked(t, payload)
			}
		}
		st.Progress = clampf(st.Progress, 0, plan.TimeFrames)
		w.factoryStates[pos] = st
	}
}

func unitFactoryPlansByName(name string) []unitFactoryPlan {
	return unitFactoryPlansByBlockName[strings.ToLower(strings.TrimSpace(name))]
}

func unitFactoryCurrentPlanIndex(state factoryState, hasState bool, plans []unitFactoryPlan) int16 {
	if len(plans) == 0 {
		return -1
	}
	if !hasState {
		return 0
	}
	if state.CurrentPlan < 0 {
		return -1
	}
	if int(state.CurrentPlan) >= len(plans) {
		return -1
	}
	return state.CurrentPlan
}

func unitFactoryTotalItemCapacity(name string) int32 {
	plans := unitFactoryPlansByName(name)
	capacity := int32(10)
	for _, plan := range plans {
		for _, stack := range plan.Cost {
			if value := stack.Amount * 2; value > capacity {
				capacity = value
			}
		}
	}
	return capacity
}

func unitFactoryItemCapacity(name string, item ItemID) int32 {
	var capacity int32
	for _, plan := range unitFactoryPlansByName(name) {
		for _, stack := range plan.Cost {
			if stack.Item != item {
				continue
			}
			if value := stack.Amount * 2; value > capacity {
				capacity = value
			}
		}
	}
	return capacity
}

func (w *World) unitBuildSpeedMultiplierLocked(team TeamID, rules *Rules) float32 {
	if rules == nil {
		return 1
	}
	speedMul := rules.UnitBuildSpeedMultiplier
	if tr, ok := rules.teamRule(team); ok && tr.UnitBuildSpeedMultiplier > 0 {
		speedMul *= tr.UnitBuildSpeedMultiplier
	}
	return speedMul
}

func (w *World) unitCostMultiplierLocked(team TeamID) float32 {
	rules := w.rulesMgr.Get()
	if rules == nil {
		return 1
	}
	costMul := rules.UnitCostMultiplier
	if tr, ok := rules.teamRule(team); ok && tr.UnitCostMultiplier > 0 {
		costMul *= tr.UnitCostMultiplier
	}
	return costMul
}

func unitFactoryScaledAmount(base int32, mul float32) int32 {
	if base <= 0 || mul <= 0 {
		return 0
	}
	return int32(math.Round(float64(float32(base) * mul)))
}

func (w *World) unitFactoryScaledCostLocked(team TeamID, cost []ItemStack) []ItemStack {
	if len(cost) == 0 {
		return nil
	}
	mul := w.unitCostMultiplierLocked(team)
	out := make([]ItemStack, 0, len(cost))
	for _, stack := range cost {
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

func (w *World) unitFactorySelectedPlanLocked(pos int32, tile *Tile) (unitFactoryPlan, bool) {
	if w == nil || w.model == nil || tile == nil {
		return unitFactoryPlan{}, false
	}
	plans := unitFactoryPlansByName(w.blockNameByID(int16(tile.Block)))
	if len(plans) == 0 {
		return unitFactoryPlan{}, false
	}
	st, hasState := w.factoryStates[pos]
	index := unitFactoryCurrentPlanIndex(st, hasState, plans)
	if index < 0 || int(index) >= len(plans) {
		return unitFactoryPlan{}, false
	}
	return plans[index], true
}

func (w *World) configureUnitFactoryPlanLocked(pos int32, planIndex int16) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	plans := unitFactoryPlansByName(w.blockNameByID(int16(tile.Block)))
	if len(plans) == 0 {
		return false
	}
	st, hasState := w.factoryStates[pos]
	current := unitFactoryCurrentPlanIndex(st, hasState, plans)
	target := planIndex
	if target >= 0 && int(target) >= len(plans) {
		target = -1
	}
	if current == target {
		if !hasState && current >= 0 {
			st.CurrentPlan = current
			if typeID, ok := w.resolveUnitTypeIDLocked(normalizeUnitName(plans[current].UnitName)); ok {
				st.UnitType = typeID
			}
			w.factoryStates[pos] = st
		}
		return true
	}
	if target < 0 {
		st.CurrentPlan = -1
		st.Progress = 0
		st.UnitType = 0
		w.factoryStates[pos] = st
		return true
	}
	st.CurrentPlan = target
	st.Progress = 0
	if typeID, ok := w.resolveUnitTypeIDLocked(normalizeUnitName(plans[target].UnitName)); ok {
		st.UnitType = typeID
	} else {
		st.UnitType = 0
	}
	w.factoryStates[pos] = st
	return true
}

func cloneFactoryCommandPos(v *protocol.Vec2) *protocol.Vec2 {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func cloneFactoryCommand(v *protocol.UnitCommand) *protocol.UnitCommand {
	if v == nil {
		return nil
	}
	out := *v
	return &out
}

func (w *World) configureUnitFactoryCommandLocked(pos int32, command *protocol.UnitCommand) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) || !w.unitFactoryConfigBlockAtLocked(pos) {
		return false
	}
	st := w.factoryStates[pos]
	st.Command = cloneFactoryCommand(command)
	w.factoryStates[pos] = st
	return true
}

func (w *World) clearUnitFactoryCommandLocked(pos int32) bool {
	return w.configureUnitFactoryCommandLocked(pos, nil)
}

func (w *World) configureUnitFactoryCommandPosLocked(pos int32, target protocol.Vec2) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) || !w.unitFactoryConfigBlockAtLocked(pos) {
		return false
	}
	st := w.factoryStates[pos]
	st.CommandPos = cloneFactoryCommandPos(&target)
	w.factoryStates[pos] = st
	return true
}

func (w *World) unitFactoryCommandStateLocked(pos int32) (*protocol.Vec2, *protocol.UnitCommand) {
	st, ok := w.factoryStates[pos]
	if !ok {
		return nil, nil
	}
	return cloneFactoryCommandPos(st.CommandPos), cloneFactoryCommand(st.Command)
}

func (w *World) configureUnitFactoryUnitLocked(pos int32, typeID int16) bool {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	plans := unitFactoryPlansByName(w.blockNameByID(int16(tile.Block)))
	if len(plans) == 0 {
		return false
	}
	for i, plan := range plans {
		resolved, ok := w.resolveUnitTypeIDLocked(normalizeUnitName(plan.UnitName))
		if ok && resolved == typeID {
			return w.configureUnitFactoryPlanLocked(pos, int16(i))
		}
	}
	return w.configureUnitFactoryPlanLocked(pos, -1)
}

func (w *World) unitFactoryConfigValueLocked(pos int32, tile *Tile) (int32, bool) {
	if w == nil || w.model == nil || tile == nil {
		return 0, false
	}
	plans := unitFactoryPlansByName(w.blockNameByID(int16(tile.Block)))
	if len(plans) == 0 {
		return 0, false
	}
	st, hasState := w.factoryStates[pos]
	index := unitFactoryCurrentPlanIndex(st, hasState, plans)
	return int32(index), true
}

func (w *World) unitFactorySelectedTypeLocked(pos int32, tile *Tile) (int16, bool) {
	if w == nil || w.model == nil || tile == nil {
		return 0, false
	}
	if st, ok := w.factoryStates[pos]; ok && st.UnitType > 0 {
		return st.UnitType, true
	}
	plan, ok := w.unitFactorySelectedPlanLocked(pos, tile)
	if !ok {
		return 0, false
	}
	typeID, ok := w.resolveUnitTypeIDLocked(normalizeUnitName(plan.UnitName))
	if !ok {
		return 0, false
	}
	return typeID, true
}

func (w *World) unitFactoryProgressLocked(pos int32, tile *Tile) float32 {
	if w == nil || w.model == nil || tile == nil {
		return 0
	}
	st, ok := w.factoryStates[pos]
	if !ok {
		return 0
	}
	plan, planOK := w.unitFactorySelectedPlanLocked(pos, tile)
	if !planOK || plan.TimeFrames <= 0 {
		return maxf(st.Progress, 0)
	}
	if st.Progress < 0 {
		return 0
	}
	if st.Progress > plan.TimeFrames {
		return plan.TimeFrames
	}
	return st.Progress
}

func (w *World) unitFactoryControllerStateLocked(state factoryState) *protocol.ControllerState {
	if state.Command == nil && state.CommandPos == nil {
		return nil
	}
	ctrl := &protocol.ControllerState{Type: protocol.ControllerCommand9}
	if state.Command != nil {
		ctrl.Command.CommandID = int8(state.Command.ID)
	}
	if state.CommandPos != nil {
		ctrl.Command.HasPos = true
		ctrl.Command.TargetPos = *state.CommandPos
	}
	return ctrl
}

func (w *World) newFactoryUnitPayloadLocked(tile *Tile, state factoryState) *payloadData {
	typeID := state.UnitType
	if tile == nil || tile.Build == nil || typeID <= 0 {
		return nil
	}
	unit := w.newProducedUnitEntityLocked(typeID, tile.Build.Team, float32(tile.X*8+4), float32(tile.Y*8+4), float32(tile.Rotation)*90)
	entity := w.entitySyncUnitLocked(unit, nil, w.unitFactoryControllerStateLocked(state))
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
	return &payloadData{
		Kind:       payloadKindUnit,
		UnitTypeID: typeID,
		Serialized: append([]byte(nil), writer.Bytes()...),
		Rotation:   tile.Rotation,
		Health:     unit.Health,
		MaxHealth:  unit.MaxHealth,
		UnitState:  func() *RawEntity { clone := cloneRawEntity(unit); return &clone }(),
	}
}

func (w *World) newProducedUnitEntityLocked(typeID int16, team TeamID, x, y, rotation float32) RawEntity {
	ent := RawEntity{
		TypeID:      typeID,
		X:           x,
		Y:           y,
		Rotation:    rotation,
		Team:        team,
		Health:      100,
		MaxHealth:   100,
		Shield:      0,
		ShieldMax:   0,
		ShieldRegen: 0,
		Armor:       0,
		SlowMul:     1,
		RuntimeInit: true,
		MineTilePos: invalidEntityTilePos,
	}
	w.applyUnitTypeDef(&ent)
	w.applyWeaponProfile(&ent)
	if isEntityFlying(ent) {
		ent.Elevation = 1
	}
	return ent
}

func (w *World) teamUnitCountsByTypeLocked() map[TeamID]map[int16]int32 {
	out := make(map[TeamID]map[int16]int32)
	if w.model == nil {
		return out
	}
	for _, e := range w.model.Entities {
		if e.Team == 0 || e.TypeID <= 0 {
			continue
		}
		if _, ok := out[e.Team]; !ok {
			out[e.Team] = map[int16]int32{}
		}
		out[e.Team][e.TypeID]++
	}
	return out
}

func (w *World) canCreateUnitLocked(team TeamID, typeID int16, rules *Rules, teamCaps map[TeamID]int32, unitCounts map[TeamID]map[int16]int32) bool {
	if team == 0 || typeID <= 0 {
		return false
	}
	if !w.unitTypeUsesCapLocked(typeID) {
		return true
	}
	cap, ok := teamCaps[team]
	if !ok {
		cap = w.teamUnitCapLocked(team, rules)
	}
	if cap >= unlimitedUnitCap {
		return true
	}
	return unitCounts[team][typeID] < cap
}

func (w *World) teamUnitCountByTypeLocked(team TeamID, typeID int16) int32 {
	if w.model == nil || team == 0 || typeID <= 0 {
		return 0
	}
	var count int32
	for _, e := range w.model.Entities {
		if e.Team == team && e.TypeID == typeID {
			count++
		}
	}
	return count
}

func (w *World) unitTypeUsesCapLocked(typeID int16) bool {
	name := normalizeUnitName(w.unitNamesByID[typeID])
	switch name {
	case "", "block":
		return name != "block"
	default:
		return true
	}
}

func (w *World) teamUnitCapsLocked(rules *Rules) map[TeamID]int32 {
	out := make(map[TeamID]int32, len(w.teamBuildingTiles))
	for team := range w.teamBuildingTiles {
		out[team] = w.teamUnitCapLocked(team, rules)
	}
	return out
}

func (w *World) teamUnitCapLocked(team TeamID, rules *Rules) int32 {
	if team == 0 {
		return 0
	}
	if rules != nil {
		if waveTeam, ok := parseTeamKey(rules.WaveTeam); ok && waveTeam == team && !rules.Pvp {
			return unlimitedUnitCap
		}
		if rules.DisableUnitCap {
			return unlimitedUnitCap
		}
	}
	modifier := int32(0)
	for _, pos := range w.teamBuildingTiles[team] {
		if pos < 0 || w.model == nil || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		modifier += blockUnitCapModifierByName[w.blockNameByID(int16(tile.Block))]
	}
	base := int32(0)
	if rules != nil {
		base = rules.UnitCap
		if rules.UnitCapVariable {
			base += modifier
		}
	}
	if base < 0 {
		return 0
	}
	return base
}

func (w *World) consumeProductionCost(team TeamID, cost []ItemStack) bool {
	if team == 0 || len(cost) == 0 {
		return true
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(team) {
		return true
	}
	if w.consumeItemsFromTeamCoresLocked(team, cost) {
		return true
	}
	if !w.syntheticTeamInventoryEnabledLocked(team) {
		return false
	}
	return w.consumeItemsForTeam(team, cost)
}

func (w *World) blockNameByID(blockID int16) string {
	if blockID <= 0 || len(w.blockNamesByID) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(w.blockNamesByID[blockID]))
}

func (w *World) ensureTeamInventory(team TeamID) {
	if team == 0 {
		return
	}
	if _, ok := w.teamItems[team]; ok {
		return
	}
	inv := map[ItemID]int32{}
	seed := int32(0)
	if rules := w.rulesMgr.Get(); rules != nil && rules.teamFillItems(team) {
		// Vanilla fillItems writes full stacks into real cores every logic update.
		// The synthetic inventory exists only as a no-core fallback, so a large seed
		// is sufficient here without leaking fake stock into normal player cores.
		seed = 1_000_000
	}
	if seed <= 0 {
		w.teamItems[team] = inv
		return
	}
	addItemSeed := func(item ItemID) {
		if item < 0 {
			return
		}
		if _, ok := inv[item]; !ok {
			inv[item] = seed
		}
	}
	addStackSeed := func(stacks []ItemStack) {
		for _, s := range stacks {
			addItemSeed(s.Item)
		}
	}
	for _, cost := range blockCostByName {
		addStackSeed(cost)
	}
	for _, cost := range w.blockCostsByName {
		addStackSeed(cost)
	}
	for _, plans := range unitFactoryPlansByBlockName {
		for _, plan := range plans {
			addStackSeed(plan.Cost)
		}
	}
	for _, prof := range crafterProfilesByBlockName {
		addStackSeed(prof.InputItems)
		addStackSeed(prof.OutputItems)
	}
	for _, prof := range separatorProfilesByBlockName {
		addStackSeed(prof.InputItems)
		addStackSeed(prof.Results)
	}
	for _, prof := range solidPumpProfilesByBlockName {
		addItemSeed(prof.ItemConsume)
	}
	for _, item := range []ItemID{
		copperItemID,
		leadItemID,
		metaglassItemID,
		graphiteItemID,
		sandItemID,
		coalItemID,
		titaniumItemID,
		thoriumItemID,
		scrapItemID,
		siliconItemID,
		plastaniumItemID,
		phaseFabricItemID,
		surgeAlloyItemID,
		sporePodItemID,
		blastCompoundItemID,
		pyratiteItemID,
		berylliumItemID,
		tungstenItemID,
		oxideItemID,
		carbideItemID,
		fissileMatterItemID,
	} {
		addItemSeed(item)
	}
	if w.model != nil {
		for i := range w.model.Tiles {
			tile := &w.model.Tiles[i]
			if tile.Build == nil {
				continue
			}
			for _, stack := range tile.Build.Items {
				addItemSeed(stack.Item)
			}
		}
	}
	w.teamItems[team] = inv
}

func (w *World) stepFillItemsLocked() {
	if w == nil || w.model == nil {
		return
	}
	rules := w.rulesMgr.Get()
	if rules == nil || len(w.teamPrimaryCore) == 0 {
		return
	}
	for team, corePos := range w.teamPrimaryCore {
		if team == 0 || !rules.teamFillItems(team) {
			continue
		}
		if corePos < 0 || int(corePos) >= len(w.model.Tiles) {
			continue
		}
		coreTile := &w.model.Tiles[corePos]
		if coreTile.Build == nil || coreTile.Block == 0 {
			continue
		}
		capacity := w.itemCapacityAtLocked(corePos)
		if capacity <= 0 {
			continue
		}
		w.ensureTeamInventory(team)
		if len(w.teamItems[team]) == 0 {
			continue
		}
		changed := make([]ItemID, 0, len(w.teamItems[team]))
		for item := range w.teamItems[team] {
			if item < 0 {
				continue
			}
			if coreTile.Build.ItemAmount(item) == capacity {
				continue
			}
			coreTile.Build.SetItemAmount(item, capacity)
			changed = append(changed, item)
		}
		if len(changed) > 0 {
			w.emitTeamCoreItemsLocked(team, changed)
		}
	}
}

func (w *World) syntheticTeamInventoryEnabledLocked(team TeamID) bool {
	if team == 0 {
		return false
	}
	if inv, ok := w.teamItems[team]; ok && len(inv) > 0 {
		return true
	}
	rules := w.rulesMgr.Get()
	return rules != nil && rules.teamFillItems(team)
}

func (w *World) teamCoreBuildsLocked(team TeamID) []*Building {
	if w.model == nil || team == 0 {
		return nil
	}
	positions := w.teamCoreTiles[team]
	if len(positions) == 0 {
		return nil
	}
	out := make([]*Building, 0, len(positions))
	for _, pos := range positions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Team != team || tile.Build == nil || tile.Block <= 0 {
			continue
		}
		out = append(out, tile.Build)
	}
	return out
}

func (w *World) teamHasCoreLocked(team TeamID) bool {
	return len(w.teamCoreBuildsLocked(team)) > 0
}

func (w *World) teamCoreItemsLocked(team TeamID) map[ItemID]int32 {
	cores := w.teamCoreBuildsLocked(team)
	if len(cores) == 0 {
		return nil
	}
	out := make(map[ItemID]int32)
	for _, core := range cores {
		for _, stack := range core.Items {
			if stack.Amount <= 0 {
				continue
			}
			out[stack.Item] += stack.Amount
		}
	}
	return out
}

func (w *World) emitTeamCoreItemsLocked(team TeamID, items []ItemID) {
	if team == 0 || len(items) == 0 {
		return
	}
	totals := w.teamCoreItemsLocked(team)
	seen := make(map[ItemID]struct{}, len(items))
	for _, item := range items {
		if _, ok := seen[item]; ok {
			continue
		}
		seen[item] = struct{}{}
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:       EntityEventTeamItems,
			BuildTeam:  team,
			ItemID:     item,
			ItemAmount: totals[item],
		})
	}
}

func (w *World) addItemsToTeamCoresLocked(team TeamID, stacks []ItemStack) bool {
	cores := w.teamCoreBuildsLocked(team)
	if len(cores) == 0 || len(stacks) == 0 {
		return false
	}
	changed := make([]ItemID, 0, len(stacks))
	for _, stack := range stacks {
		if stack.Amount <= 0 {
			continue
		}
		remaining := stack.Amount
		for _, core := range cores {
			if remaining <= 0 {
				break
			}
			core.AddItem(stack.Item, remaining)
			remaining = 0
		}
		if remaining != stack.Amount {
			changed = append(changed, stack.Item)
		}
	}
	if len(changed) == 0 {
		return false
	}
	w.emitTeamCoreItemsLocked(team, changed)
	return true
}

func (w *World) consumeItemsFromTeamCoresLocked(team TeamID, cost []ItemStack) bool {
	cores := w.teamCoreBuildsLocked(team)
	if len(cores) == 0 || len(cost) == 0 {
		return false
	}
	totals := w.teamCoreItemsLocked(team)
	for _, stack := range cost {
		if stack.Amount <= 0 {
			continue
		}
		if totals[stack.Item] < stack.Amount {
			return false
		}
	}
	changed := make([]ItemID, 0, len(cost))
	for _, stack := range cost {
		if stack.Amount <= 0 {
			continue
		}
		remaining := stack.Amount
		for _, core := range cores {
			if remaining <= 0 {
				break
			}
			available := core.ItemAmount(stack.Item)
			if available <= 0 {
				continue
			}
			use := remaining
			if available < use {
				use = available
			}
			if core.RemoveItem(stack.Item, use) {
				remaining -= use
			}
		}
		changed = append(changed, stack.Item)
	}
	w.emitTeamCoreItemsLocked(team, changed)
	return true
}

func (w *World) consumeBuildCost(team TeamID, blockID int16) bool {
	name := w.blockNameByID(blockID)
	cost := w.buildCostByName(name)
	if len(cost) == 0 {
		return true
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(team) {
		return true
	}
	scaled := w.scaleBuildCost(cost, rules)
	if w.consumeItemsFromTeamCoresLocked(team, scaled) {
		return true
	}
	if w.teamHasCoreLocked(team) || !w.syntheticTeamInventoryEnabledLocked(team) {
		return false
	}
	return w.consumeItemsForTeam(team, scaled)
}

func (w *World) pendingBuildScaledCostLocked(blockID int16) []ItemStack {
	if blockID <= 0 {
		return nil
	}
	name := w.blockNameByID(blockID)
	cost := w.buildCostByName(name)
	if len(cost) == 0 {
		return nil
	}
	return w.scaleBuildCost(cost, w.rulesMgr.Get())
}

func (w *World) ensurePendingBuildCostStateLocked(st *pendingBuildState) {
	if st == nil || st.BlockID <= 0 {
		return
	}
	if len(st.BuildCost) == len(st.ItemsLeft) &&
		len(st.BuildCost) == len(st.Accumulator) &&
		len(st.BuildCost) == len(st.TotalAccumulator) &&
		len(st.BuildCost) > 0 {
		return
	}
	scaled := w.pendingBuildScaledCostLocked(st.BlockID)
	st.BuildCost = append([]ItemStack(nil), scaled...)
	st.ItemsLeft = make([]int32, len(scaled))
	st.Accumulator = make([]float32, len(scaled))
	st.TotalAccumulator = make([]float32, len(scaled))
	for i, stack := range scaled {
		st.ItemsLeft[i] = stack.Amount
	}
}

func (w *World) pendingBuildHasStartItemsLocked(team TeamID, blockID int16) bool {
	if team == 0 || blockID <= 0 {
		return false
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(team) {
		return true
	}
	for _, stack := range w.pendingBuildScaledCostLocked(blockID) {
		if stack.Amount <= 0 {
			continue
		}
		if w.availableBuildItemAmountLocked(team, stack.Item) < 1 {
			return false
		}
	}
	return true
}

func (w *World) availableBuildItemAmountLocked(team TeamID, item ItemID) int32 {
	if team == 0 {
		return 0
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(team) {
		return unlimitedUnitCap
	}
	if cores := w.teamCoreBuildsLocked(team); len(cores) > 0 {
		total := int32(0)
		for _, core := range cores {
			total += core.ItemAmount(item)
		}
		return total
	}
	if !w.syntheticTeamInventoryEnabledLocked(team) {
		return 0
	}
	w.ensureTeamInventory(team)
	return w.teamItems[team][item]
}

func (w *World) consumeBuildItemAmountLocked(team TeamID, item ItemID, amount int32) int32 {
	if team == 0 || amount <= 0 {
		return 0
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(team) {
		return amount
	}
	if cores := w.teamCoreBuildsLocked(team); len(cores) > 0 {
		remaining := amount
		for _, core := range cores {
			if remaining <= 0 {
				break
			}
			available := core.ItemAmount(item)
			if available <= 0 {
				continue
			}
			use := remaining
			if available < use {
				use = available
			}
			if core.RemoveItem(item, use) {
				remaining -= use
			}
		}
		removed := amount - remaining
		if removed > 0 {
			w.emitTeamCoreItemsLocked(team, []ItemID{item})
		}
		return removed
	}
	if !w.syntheticTeamInventoryEnabledLocked(team) {
		return 0
	}
	w.ensureTeamInventory(team)
	available := w.teamItems[team][item]
	if available <= 0 {
		return 0
	}
	if available < amount {
		amount = available
	}
	w.teamItems[team][item] -= amount
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventTeamItems,
		BuildTeam:  team,
		ItemID:     item,
		ItemAmount: w.teamItems[team][item],
	})
	return amount
}

func (w *World) checkPendingBuildRequiredLocked(team TeamID, st *pendingBuildState, amount float32, remove bool) float32 {
	if st == nil || amount <= 0 {
		return 0
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(team) {
		return amount
	}
	maxProgress := amount
	for i, stack := range st.BuildCost {
		if i >= len(st.ItemsLeft) || stack.Amount <= 0 || st.ItemsLeft[i] <= 0 {
			continue
		}
		required := int32(st.Accumulator[i])
		available := w.availableBuildItemAmountLocked(team, stack.Item)
		if available == 0 {
			maxProgress = 0
			continue
		}
		if required <= 0 {
			continue
		}
		maxUse := required
		if available < maxUse {
			maxUse = available
		}
		fraction := float32(maxUse) / float32(required)
		maxProgress = minf(maxProgress, maxProgress*fraction)
		st.Accumulator[i] -= float32(maxUse)
		if st.Accumulator[i] < 0 {
			st.Accumulator[i] = 0
		}
		if remove {
			removed := w.consumeBuildItemAmountLocked(team, stack.Item, maxUse)
			st.ItemsLeft[i] -= removed
			if st.ItemsLeft[i] < 0 {
				st.ItemsLeft[i] = 0
			}
		}
	}
	return maxProgress
}

func (w *World) applyVanillaBuildCostStepLocked(team TeamID, st *pendingBuildState, amount float32) float32 {
	if st == nil || amount <= 0 {
		return 0
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(team) {
		return amount
	}
	w.ensurePendingBuildCostStateLocked(st)
	if len(st.BuildCost) == 0 {
		return amount
	}
	maxProgress := w.checkPendingBuildRequiredLocked(team, st, amount, false)
	for i, stack := range st.BuildCost {
		if stack.Amount <= 0 {
			continue
		}
		target := float32(stack.Amount)
		add := minf(target*maxProgress, target-st.TotalAccumulator[i])
		if add < 0 {
			add = 0
		}
		st.Accumulator[i] += add
		st.TotalAccumulator[i] = minf(st.TotalAccumulator[i]+target*maxProgress, target)
	}
	maxProgress = w.checkPendingBuildRequiredLocked(team, st, maxProgress, true)
	if maxProgress < 0 {
		maxProgress = 0
	}
	if maxProgress > amount {
		maxProgress = amount
	}
	return maxProgress
}

func (w *World) finishPendingBuildCostLocked(team TeamID, st *pendingBuildState) bool {
	if st == nil {
		return false
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(team) {
		return true
	}
	if len(st.BuildCost) == 0 {
		return true
	}
	for i, stack := range st.BuildCost {
		if i >= len(st.ItemsLeft) || st.ItemsLeft[i] <= 0 || stack.Amount <= 0 {
			continue
		}
		if w.availableBuildItemAmountLocked(team, stack.Item) < st.ItemsLeft[i] {
			return false
		}
	}
	for i, stack := range st.BuildCost {
		if i >= len(st.ItemsLeft) || st.ItemsLeft[i] <= 0 || stack.Amount <= 0 {
			continue
		}
		need := st.ItemsLeft[i]
		if need <= 0 {
			continue
		}
		if removed := w.consumeBuildItemAmountLocked(team, stack.Item, need); removed != need {
			st.ItemsLeft[i] -= removed
			if st.ItemsLeft[i] < 0 {
				st.ItemsLeft[i] = 0
			}
			return false
		}
		st.ItemsLeft[i] = 0
	}
	return true
}

func (w *World) refundPendingBuildConsumedLocked(st pendingBuildState) {
	if st.Team == 0 {
		return
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(st.Team) {
		return
	}
	if len(st.BuildCost) == 0 || len(st.ItemsLeft) < len(st.BuildCost) {
		return
	}
	refundStacks := make([]ItemStack, 0, len(st.BuildCost))
	for i, stack := range st.BuildCost {
		if stack.Amount <= 0 {
			continue
		}
		consumed := stack.Amount - st.ItemsLeft[i]
		if consumed <= 0 {
			continue
		}
		refundStacks = append(refundStacks, ItemStack{Item: stack.Item, Amount: consumed})
	}
	w.addRefundStacksToTeamLocked(st.Team, refundStacks)
}

func (w *World) buildDurationSeconds(blockID int16, rules *Rules) float32 {
	return w.buildDurationSecondsForTeam(blockID, 0, rules)
}

func (w *World) buildDurationSecondsForTeam(blockID int16, team TeamID, rules *Rules) float32 {
	return w.buildDurationSecondsForOwnerLocked(blockID, 0, team, rules)
}

func (w *World) builderSpeedForOwnerLocked(owner int32, team TeamID) float32 {
	builderSpeed := float32(0.5) // alpha buildSpeed in vanilla BuilderComp path
	if owner != 0 && w.model != nil {
		if st, ok := w.builderStates[owner]; ok && st.UnitID != 0 {
			for i := range w.model.Entities {
				if w.model.Entities[i].ID != st.UnitID {
					continue
				}
				if speed := w.builderSpeedForUnitTypeLocked(w.model.Entities[i].TypeID); speed > 0 {
					return speed
				}
				break
			}
		}
	}
	if team > 0 && w.teamBuilderSpeed != nil {
		if v, ok := w.teamBuilderSpeed[team]; ok && v > 0 {
			builderSpeed = v
		}
	}
	return builderSpeed
}

func (w *World) buildDurationSecondsForOwnerLocked(blockID int16, owner int32, team TeamID, rules *Rules) float32 {
	return w.buildDurationSecondsForBuilderSpeedLocked(blockID, team, rules, w.builderSpeedForOwnerLocked(owner, team))
}

func (w *World) buildDurationSecondsForBuilderSpeedLocked(blockID int16, team TeamID, rules *Rules, builderSpeed float32) float32 {
	if rules != nil && (rules.InstantBuild || rules.Editor) {
		return 0.01
	}
	name := w.blockNameByID(blockID)
	base := float32(1.0)
	if name != "" {
		if t, ok := w.blockBuildTimesByName[name]; ok && t > 0 {
			base = t
		}
	}
	if rules != nil && rules.BuildCostMultiplier > 0 {
		base *= rules.BuildCostMultiplier
	}
	if rules != nil && rules.BuildSpeedMultiplier > 0 {
		base /= rules.BuildSpeedMultiplier
	}
	if team > 0 && rules != nil {
		if tr, ok := rules.teamRule(team); ok && tr.BuildSpeedMultiplier > 0 {
			base /= tr.BuildSpeedMultiplier
		}
	}
	// Match vanilla BuilderComp formula:
	// bs = 1/buildCost * type.buildSpeed * buildSpeedMultiplier * rules.buildSpeed(team)
	// Here "base" is buildCost/60, so divide only by unit buildSpeed and rules.buildSpeed(team).
	// unitBuildSpeedMultiplier is for unit factories/spawners in vanilla, not BuilderComp.
	if builderSpeed > 0 {
		base /= builderSpeed
	}
	// Vanilla allows very cheap blocks like conveyors to complete in ~1 tick.
	// Keep only a one-tick minimum to avoid divide-by-zero / same-packet corruption.
	if base < float32(1.0/60.0) {
		base = float32(1.0 / 60.0)
	}
	return base
}

func (w *World) refundDeconstructCost(tile *Tile, fallbackTeam TeamID) {
	team, refundStacks := w.deconstructRefundStacks(tile, fallbackTeam)
	if team == 0 || len(refundStacks) == 0 {
		return
	}
	w.addRefundStacksToTeamLocked(team, refundStacks)
}

func (w *World) deconstructRefundStacks(tile *Tile, fallbackTeam TeamID) (TeamID, []ItemStack) {
	if tile == nil {
		return 0, nil
	}
	name := w.blockNameByID(int16(tile.Block))
	cost := w.buildCostByName(name)
	if len(cost) == 0 {
		return 0, nil
	}
	refundMul := float32(0.5)
	rules := w.rulesMgr.Get()
	team := tile.Team
	if team == 0 {
		team = fallbackTeam
	}
	if team == 0 || (rules != nil && rules.teamInfiniteResources(team)) {
		return 0, nil
	}
	if rules != nil && rules.DeconstructRefundMultiplier > 0 {
		refundMul = rules.DeconstructRefundMultiplier
	}
	refundStacks := make([]ItemStack, 0, len(cost))
	for _, s := range cost {
		if s.Amount <= 0 {
			continue
		}
		refund := w.scaleSingleAmount(s.Amount, rules, refundMul)
		if refund <= 0 {
			continue
		}
		refundStacks = append(refundStacks, ItemStack{Item: s.Item, Amount: refund})
	}
	return team, refundStacks
}

func (w *World) addRefundStacksToTeamLocked(team TeamID, stacks []ItemStack) {
	if team == 0 || len(stacks) == 0 {
		return
	}
	if w.addItemsToTeamCoresLocked(team, stacks) {
		return
	}
	if !w.syntheticTeamInventoryEnabledLocked(team) {
		return
	}
	w.ensureTeamInventory(team)
	for _, stack := range stacks {
		w.addTeamItems(team, stack.Item, stack.Amount)
	}
}

func (w *World) applyVanillaDeconstructRefundStepLocked(team TeamID, refundStacks []ItemStack, amount float32, accum map[ItemID]float32, total map[ItemID]float32, refunded map[ItemID]int32) (map[ItemID]float32, map[ItemID]float32, map[ItemID]int32) {
	if team == 0 || len(refundStacks) == 0 || amount <= 0 {
		return accum, total, refunded
	}
	if accum == nil {
		accum = make(map[ItemID]float32, len(refundStacks))
	}
	if total == nil {
		total = make(map[ItemID]float32, len(refundStacks))
	}
	if refunded == nil {
		refunded = make(map[ItemID]int32, len(refundStacks))
	}
	deltaStacks := make([]ItemStack, 0, len(refundStacks))
	for _, stack := range refundStacks {
		if stack.Amount <= 0 {
			continue
		}
		target := float32(stack.Amount)
		item := stack.Item
		add := amount * target
		if remain := target - total[item]; add > remain {
			add = remain
		}
		if add < 0 {
			add = 0
		}
		accum[item] += add
		total[item] = minf(total[item]+amount*target, target)
		accumulated := int32(accum[item])
		if accumulated <= 0 {
			continue
		}
		refunded[item] += accumulated
		accum[item] -= float32(accumulated)
		deltaStacks = append(deltaStacks, ItemStack{Item: item, Amount: accumulated})
	}
	w.addRefundStacksToTeamLocked(team, deltaStacks)
	return accum, total, refunded
}

func (w *World) finishVanillaDeconstructRefundLocked(team TeamID, refundStacks []ItemStack, refunded map[ItemID]int32) map[ItemID]int32 {
	if team == 0 || len(refundStacks) == 0 {
		return refunded
	}
	if refunded == nil {
		refunded = make(map[ItemID]int32, len(refundStacks))
	}
	deltaStacks := make([]ItemStack, 0, len(refundStacks))
	for _, stack := range refundStacks {
		if stack.Amount <= 0 {
			continue
		}
		current := refunded[stack.Item]
		if current >= stack.Amount {
			continue
		}
		delta := stack.Amount - current
		refunded[stack.Item] = stack.Amount
		deltaStacks = append(deltaStacks, ItemStack{Item: stack.Item, Amount: delta})
	}
	w.addRefundStacksToTeamLocked(team, deltaStacks)
	return refunded
}

func (w *World) refundBuildCost(team TeamID, blockID int16, mul float32) {
	if team == 0 || blockID <= 0 || mul <= 0 {
		return
	}
	name := w.blockNameByID(blockID)
	cost := w.buildCostByName(name)
	if len(cost) == 0 {
		return
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(team) {
		return
	}
	refundStacks := make([]ItemStack, 0, len(cost))
	for _, s := range cost {
		if s.Amount <= 0 {
			continue
		}
		add := w.scaleSingleAmount(s.Amount, rules, mul)
		if add <= 0 {
			continue
		}
		refundStacks = append(refundStacks, ItemStack{Item: s.Item, Amount: add})
	}
	if w.addItemsToTeamCoresLocked(team, refundStacks) {
		return
	}
	if !w.syntheticTeamInventoryEnabledLocked(team) {
		return
	}
	w.ensureTeamInventory(team)
	for _, stack := range refundStacks {
		w.addTeamItems(team, stack.Item, stack.Amount)
	}
}

func (w *World) addTeamItems(team TeamID, item ItemID, delta int32) {
	if team == 0 || delta == 0 {
		return
	}
	w.ensureTeamInventory(team)
	w.teamItems[team][item] += delta
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventTeamItems,
		BuildTeam:  team,
		ItemID:     item,
		ItemAmount: w.teamItems[team][item],
	})
}

func (w *World) consumeItemsForTeam(team TeamID, cost []ItemStack) bool {
	if team == 0 || len(cost) == 0 {
		return true
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.teamInfiniteResources(team) {
		return true
	}
	if !w.syntheticTeamInventoryEnabledLocked(team) {
		return false
	}
	w.ensureTeamInventory(team)
	inv := w.teamItems[team]
	for _, s := range cost {
		if s.Amount <= 0 {
			continue
		}
		if inv[s.Item] < s.Amount {
			return false
		}
	}
	for _, s := range cost {
		if s.Amount <= 0 {
			continue
		}
		inv[s.Item] -= s.Amount
		w.entityEvents = append(w.entityEvents, EntityEvent{Kind: EntityEventTeamItems, BuildTeam: team, ItemID: s.Item, ItemAmount: inv[s.Item]})
	}
	return true
}

func (w *World) buildCostByName(name string) []ItemStack {
	if name == "" {
		return nil
	}
	if len(w.blockCostsByName) > 0 {
		if c, ok := w.blockCostsByName[name]; ok {
			return c
		}
	}
	return blockCostByName[name]
}

func (w *World) scaleBuildCost(cost []ItemStack, rules *Rules) []ItemStack {
	if len(cost) == 0 {
		return nil
	}
	out := make([]ItemStack, 0, len(cost))
	for _, s := range cost {
		if s.Amount <= 0 {
			continue
		}
		amt := w.scaleSingleAmount(s.Amount, rules, 1)
		if amt <= 0 {
			continue
		}
		out = append(out, ItemStack{Item: s.Item, Amount: amt})
	}
	return out
}

func (w *World) scaleSingleAmount(base int32, rules *Rules, extraMul float32) int32 {
	if base <= 0 {
		return 0
	}
	mul := float32(1)
	if rules != nil && rules.BuildCostMultiplier > 0 {
		mul = rules.BuildCostMultiplier
	}
	mul *= extraMul
	if mul <= 0 {
		return 0
	}
	out := int32(math.Round(float64(float32(base) * mul)))
	if out <= 0 {
		out = 1
	}
	return out
}
