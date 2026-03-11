package world

import (
	"math"
	"strings"
)

func clampBuildCost(val float32) float32 {
	if val <= 0 {
		return 1
	}
	return val
}

func (w *World) buildDataForBlockLocked(blockID int16, rules *Rules) (req []ItemStack, buildTime float32, buildCost float32) {
	def, ok := w.blockBuildForID(blockID)
	if ok {
		buildTime = def.BuildTime
		req = append([]ItemStack(nil), def.Requirements...)
	}
	if buildTime <= 0 {
		buildTime = 20
	}
	mult := float32(1)
	if rules != nil && rules.BuildCostMultiplier > 0 {
		mult = rules.BuildCostMultiplier
	}
	if mult != 1 && len(req) > 0 {
		for i := range req {
			if req[i].Amount <= 0 {
				continue
			}
			req[i].Amount = int32(math.Round(float64(float32(req[i].Amount) * mult)))
			if req[i].Amount < 0 {
				req[i].Amount = 0
			}
		}
	}
	buildCost = buildTime * mult
	if buildCost <= 0 {
		buildCost = buildTime
	}
	return req, buildTime, buildCost
}

func (w *World) teamCoreLocked(team TeamID) *Building {
	if w.model == nil {
		return nil
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Block == 0 || t.Team != team {
			continue
		}
		name := ""
		if w.blockNamesByID != nil {
			name = w.blockNamesByID[int16(t.Block)]
		}
		if strings.HasPrefix(name, "core-") || t.Block == 78 || (t.Block >= 339 && t.Block <= 344) {
			return w.ensureBuildLocked(t)
		}
	}
	return nil
}

func (w *World) takeFromTeamCoreLocked(team TeamID, item ItemID, amount int32) int32 {
	if amount <= 0 {
		return 0
	}
	core := w.teamCoreLocked(team)
	if core == nil {
		return 0
	}
	removed := int32(0)
	for i := 0; i < len(core.Items) && removed < amount; i++ {
		if core.Items[i].Item != item {
			continue
		}
		need := amount - removed
		if core.Items[i].Amount <= need {
			removed += core.Items[i].Amount
			core.Items[i].Amount = 0
		} else {
			core.Items[i].Amount -= need
			removed += need
		}
		if core.Items[i].Amount <= 0 {
			core.Items = append(core.Items[:i], core.Items[i+1:]...)
			i--
		}
	}
	return removed
}

func (w *World) refundToTeamCoreLocked(team TeamID, item ItemID, amount int32) int32 {
	if amount <= 0 {
		return 0
	}
	core := w.teamCoreLocked(team)
	if core == nil {
		return 0
	}
	core.AddItem(item, amount)
	return amount
}

// UnitBuildSpeed returns the build speed for a unit id, falling back to 1.
func (w *World) UnitBuildSpeed(unitID int32) float32 {
	if w == nil || unitID == 0 {
		return 1
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil {
		return 1
	}
	for i := range w.model.Entities {
		e := w.model.Entities[i]
		if e.ID != unitID {
			continue
		}
		if w.unitTypeDefsByID != nil {
			if def, ok := w.unitTypeDefsByID[e.TypeID]; ok && def.BuildSpeed > 0 {
				return def.BuildSpeed
			}
		}
		break
	}
	return 1
}
