package world

import (
	"strings"
	"time"
)

type factoryProfile struct {
	UnitName string
	TimeSec  float32
	Cost     []ItemStack
}

var blockCostByName = map[string][]ItemStack{
	"duo":              {{Item: 0, Amount: 35}},
	"mender":           {{Item: 0, Amount: 25}, {Item: 1, Amount: 30}},
	"mechanical-drill": {{Item: 0, Amount: 12}},
	"pneumatic-drill":  {{Item: 0, Amount: 18}, {Item: 1, Amount: 10}},
	"conveyor":         {{Item: 0, Amount: 1}},
	"router":           {{Item: 0, Amount: 3}},
	"junction":         {{Item: 0, Amount: 2}},
	"ground-factory":   {{Item: 0, Amount: 60}, {Item: 1, Amount: 50}, {Item: 2, Amount: 40}},
	"air-factory":      {{Item: 0, Amount: 60}, {Item: 1, Amount: 50}, {Item: 2, Amount: 40}},
	"naval-factory":    {{Item: 0, Amount: 60}, {Item: 1, Amount: 50}, {Item: 2, Amount: 40}},
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
	if _, ok := w.teamItems[team]; !ok {
		w.teamItems[team] = map[ItemID]int32{
			0: 3000, // copper
			1: 2000, // lead
			2: 1000, // graphite
			3: 500,  // silicon
		}
	}
}

func (w *World) consumeBuildCost(team TeamID, blockID int16) bool {
	name := w.blockNameByID(blockID)
	cost := blockCostByName[name]
	if len(cost) == 0 {
		return true
	}
	return w.consumeItemsForTeam(team, cost)
}

func (w *World) refundDeconstructCost(tile *Tile, fallbackTeam TeamID) {
	if tile == nil {
		return
	}
	name := w.blockNameByID(int16(tile.Block))
	cost := blockCostByName[name]
	if len(cost) == 0 {
		return
	}
	refundMul := float32(0.5)
	rules := w.rulesMgr.Get()
	if rules != nil && rules.DeconstructRefundMultiplier >= 0 {
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
		refund := int32(float32(s.Amount)*refundMul + 0.5)
		if refund <= 0 {
			continue
		}
		w.teamItems[team][s.Item] += refund
	}
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
	}
	return true
}
