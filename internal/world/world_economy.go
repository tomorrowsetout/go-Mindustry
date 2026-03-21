package world

import (
	"math"
	"strings"
	"time"
)

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

func (w *World) stepFactoryProduction(delta time.Duration) {
	if w.model == nil {
		return
	}
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t.Build == nil || t.Block <= 0 || t.Team == 0 {
			continue
		}
		name := w.blockNameByID(int16(t.Block))
		prof, ok := factoryByName[name]
		if !ok {
			continue
		}
		pos := int32(t.Y*w.model.Width + t.X)
		st := w.factoryStates[pos]
		if st.UnitType <= 0 {
			if typeID, ok := w.resolveUnitTypeIDLocked(normalizeUnitName(prof.UnitName)); ok {
				st.UnitType = typeID
			} else {
				continue
			}
		}
		if st.Progress <= 0 && !w.consumeItemsForTeam(t.Team, prof.Cost) {
			continue
		}
		rules := w.rulesMgr.Get()
		speedMul := float32(1)
		if rules != nil && rules.UnitBuildSpeedMultiplier > 0 {
			speedMul = rules.UnitBuildSpeedMultiplier
		}
		st.Progress += dt * speedMul
		if st.Progress >= prof.TimeSec {
			st.Progress -= prof.TimeSec
			ent := RawEntity{
				TypeID:      st.UnitType,
				X:           float32(t.X*8 + 4),
				Y:           float32(t.Y*8 + 4),
				Team:        t.Team,
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
			w.model.AddEntity(ent)
		}
		w.factoryStates[pos] = st
	}
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
	inv := map[ItemID]int32{
		0: 3000, // copper
		1: 2000, // lead
		2: 1000, // graphite
		3: 500,  // silicon
	}
	seed := int32(3000)
	if rules := w.rulesMgr.Get(); rules != nil && rules.FillItems > 0 {
		seed = rules.FillItems
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

func (w *World) consumeBuildCost(team TeamID, blockID int16) bool {
	name := w.blockNameByID(blockID)
	cost := w.buildCostByName(name)
	if len(cost) == 0 {
		return true
	}
	rules := w.rulesMgr.Get()
	if rules != nil && rules.InfiniteResources {
		return true
	}
	scaled := w.scaleBuildCost(cost, rules)
	return w.consumeItemsForTeam(team, scaled)
}

func (w *World) buildDurationSeconds(blockID int16, rules *Rules) float32 {
	return w.buildDurationSecondsForTeam(blockID, 0, rules)
}

func (w *World) buildDurationSecondsForTeam(blockID int16, team TeamID, rules *Rules) float32 {
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
	if builderSpeed > 0 {
		base /= builderSpeed
	}
	// Avoid same-tick place+finish for ultra-cheap blocks; keep visible progression.
	if base < 0.35 {
		base = 0.35
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
	if rules != nil && rules.InfiniteResources {
		return
	}
	if rules != nil && rules.DeconstructRefundMultiplier > 0 {
		refundMul = rules.DeconstructRefundMultiplier
	}
	team := tile.Team
	if team == 0 {
		team = fallbackTeam
	}
	w.ensureTeamInventory(team)
	for _, s := range cost {
		if s.Amount <= 0 {
			continue
		}
		refund := w.scaleSingleAmount(s.Amount, rules, refundMul)
		if refund <= 0 {
			continue
		}
		w.addTeamItems(team, s.Item, refund)
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
	if rules != nil && rules.InfiniteResources {
		return
	}
	w.ensureTeamInventory(team)
	for _, s := range cost {
		if s.Amount <= 0 {
			continue
		}
		add := w.scaleSingleAmount(s.Amount, rules, mul)
		if add <= 0 {
			continue
		}
		w.addTeamItems(team, s.Item, add)
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
	if rules != nil && rules.InfiniteResources {
		return true
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
