package world

import (
	"math"
	"strings"
	"time"

	"mdt-server/internal/protocol"
)

const unlimitedUnitCap = int32(^uint32(0) >> 1)

type factoryProfile struct {
	UnitName string
	TimeSec  float32
	Cost     []ItemStack
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

var factoryByName = map[string]factoryProfile{
	"ground-factory": {UnitName: "dagger", TimeSec: 10, Cost: []ItemStack{{Item: 0, Amount: 8}, {Item: 1, Amount: 14}}},
	"air-factory":    {UnitName: "flare", TimeSec: 10, Cost: []ItemStack{{Item: 0, Amount: 8}, {Item: 1, Amount: 14}}},
	"naval-factory":  {UnitName: "risso", TimeSec: 12, Cost: []ItemStack{{Item: 0, Amount: 12}, {Item: 1, Amount: 20}}},
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
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return
	}
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		t := &w.model.Tiles[pos]
		if t.Build == nil || t.Block <= 0 || t.Team == 0 {
			continue
		}
		name := w.blockNameByID(int16(t.Block))
		prof, ok := factoryByName[name]
		if !ok {
			continue
		}
		st := w.factoryStates[pos]
		payloadState := w.payloadStateLocked(pos)
		if payloadState.Payload != nil {
			st.Progress = 0
			w.factoryStates[pos] = st
			continue
		}
		if st.UnitType <= 0 {
			if typeID, ok := w.resolveUnitTypeIDLocked(normalizeUnitName(prof.UnitName)); ok {
				st.UnitType = typeID
			} else {
				continue
			}
		}
		if st.Progress <= 0 && !w.consumeProductionCost(t.Team, prof.Cost) {
			continue
		}
		rules := w.rulesMgr.Get()
		speedMul := float32(1)
		if rules != nil && rules.UnitBuildSpeedMultiplier > 0 {
			speedMul = rules.UnitBuildSpeedMultiplier
		}
		if rules != nil {
			if tr, ok := rules.teamRule(t.Team); ok && tr.UnitBuildSpeedMultiplier > 0 {
				speedMul *= tr.UnitBuildSpeedMultiplier
			}
		}
		st.Progress += dt * speedMul
		if st.Progress >= prof.TimeSec {
			st.Progress = 0
			payload := w.newFactoryUnitPayloadLocked(t, st.UnitType)
			payloadState.Payload = payload
			payloadState.Move = 0
			payloadState.Work = 0
			payloadState.Exporting = false
			w.syncPayloadTileLocked(t, payload)
		}
		w.factoryStates[pos] = st
	}
}

func (w *World) newFactoryUnitPayloadLocked(tile *Tile, typeID int16) *payloadData {
	if tile == nil || tile.Build == nil || typeID <= 0 {
		return nil
	}
	unit := w.newProducedUnitEntityLocked(typeID, tile.Build.Team, float32(tile.X*8+4), float32(tile.Y*8+4), float32(tile.Rotation)*90)
	sync := &protocol.UnitEntitySync{
		Controller:     nil,
		Health:         unit.Health,
		Rotation:       unit.Rotation,
		Shield:         unit.Shield,
		SpawnedByCore:  false,
		TeamID:         byte(unit.Team),
		TypeID:         unit.TypeID,
		UpdateBuilding: false,
		Vel:            protocol.Vec2{X: unit.VelX, Y: unit.VelY},
		X:              unit.X,
		Y:              unit.Y,
	}
	writer := protocol.NewWriter()
	_ = protocol.WritePayload(writer, protocol.UnitPayload{
		ClassID: sync.ClassID(),
		Entity:  sync,
	})
	return &payloadData{
		Kind:       payloadKindUnit,
		UnitTypeID: typeID,
		Serialized: append([]byte(nil), writer.Bytes()...),
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
		Shield:      25,
		ShieldMax:   25,
		ShieldRegen: 4.5,
		Armor:       1.5,
		SlowMul:     1,
		RuntimeInit: true,
	}
	w.applyUnitTypeDef(&ent)
	w.applyWeaponProfile(&ent)
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
	for _, cost := range blockCostByName {
		for _, s := range cost {
			addItemSeed(s.Item)
		}
	}
	for _, cost := range w.blockCostsByName {
		for _, s := range cost {
			addItemSeed(s.Item)
		}
	}
	for _, prof := range factoryByName {
		for _, s := range prof.Cost {
			addItemSeed(s.Item)
		}
	}
	w.teamItems[team] = inv
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
	out := make([]*Building, 0, 4)
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Team != team || tile.Build == nil || tile.Block <= 0 {
			continue
		}
		if !strings.HasPrefix(w.blockNameByID(int16(tile.Block)), "core-") {
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

func (w *World) buildDurationSeconds(blockID int16, rules *Rules) float32 {
	return w.buildDurationSecondsForTeam(blockID, 0, rules)
}

func (w *World) buildDurationSecondsForTeam(blockID int16, team TeamID, rules *Rules) float32 {
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
	// Here "base" is buildCost/60, so divide by builder speed terms.
	builderSpeed := float32(0.5) // alpha buildSpeed in official 155.4 UnitTypes.java
	if team > 0 && w.teamBuilderSpeed != nil {
		if v, ok := w.teamBuilderSpeed[team]; ok && v > 0 {
			builderSpeed = v
		}
	}
	if rules != nil && rules.UnitBuildSpeedMultiplier > 0 {
		builderSpeed *= rules.UnitBuildSpeedMultiplier
	}
	if team > 0 && rules != nil {
		if tr, ok := rules.teamRule(team); ok && tr.UnitBuildSpeedMultiplier > 0 {
			builderSpeed *= tr.UnitBuildSpeedMultiplier
		}
	}
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
	if tile == nil {
		return
	}
	name := w.blockNameByID(int16(tile.Block))
	cost := w.buildCostByName(name)
	if len(cost) == 0 {
		return
	}
	refundMul := float32(0.5)
	rules := w.rulesMgr.Get()
	team := tile.Team
	if team == 0 {
		team = fallbackTeam
	}
	if rules != nil && rules.teamInfiniteResources(team) {
		return
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
