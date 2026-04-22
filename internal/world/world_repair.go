package world

import (
	"math"
	"time"
)

type repairTurretProfile struct {
	Range              float32
	RepairSpeed        float32
	PowerPerSecond     float32
	AcceptCoolant      bool
	CoolantUsePerFrame float32
	CoolantMultiplier  float32
}

type repairTowerProfile struct {
	Range                  float32
	HealAmount             float32
	PowerPerSecond         float32
	RequiredLiquid         LiquidID
	RequiredLiquidPerFrame float32
}

var repairTurretProfilesByBlockName = map[string]repairTurretProfile{
	"repair-point": {
		Range:          60,
		RepairSpeed:    0.45,
		PowerPerSecond: 1,
	},
	"repair-turret": {
		Range:              145,
		RepairSpeed:        3,
		PowerPerSecond:     5,
		AcceptCoolant:      true,
		CoolantUsePerFrame: 0.16,
		CoolantMultiplier:  1.6,
	},
}

var repairTowerProfilesByBlockName = map[string]repairTowerProfile{
	"unit-repair-tower": {
		Range:                  100,
		HealAmount:             1.5,
		PowerPerSecond:         1,
		RequiredLiquid:         ozoneLiquidID,
		RequiredLiquidPerFrame: 3.0 / 60.0,
	},
}

func (w *World) stepRepairBlocks(delta time.Duration) {
	if w == nil || w.model == nil {
		return
	}
	deltaFrames := float32(delta.Seconds() * 60)
	deltaSeconds := float32(delta.Seconds())
	if deltaFrames <= 0 || deltaSeconds <= 0 {
		return
	}
	w.stepRepairTurrets(deltaFrames, deltaSeconds)
	w.stepRepairTowers(deltaFrames, deltaSeconds)
}

func (w *World) stepRepairTurrets(deltaFrames, deltaSeconds float32) {
	for _, pos := range w.repairTurretTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Team == 0 || tile.Block == 0 {
			continue
		}
		prof, ok := repairTurretProfilesByBlockName[w.blockNameByID(int16(tile.Block))]
		if !ok {
			continue
		}
		state, exists := w.repairTurretStates[pos]
		if !exists {
			state.Rotation = 90
		}
		w.stepSingleRepairTurretLocked(pos, tile, prof, &state, deltaFrames, deltaSeconds)
		w.repairTurretStates[pos] = state
	}
}

func (w *World) stepSingleRepairTurretLocked(pos int32, tile *Tile, prof repairTurretProfile, state *repairTurretRuntimeState, deltaFrames, deltaSeconds float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}
	targetIdx, hasTarget := findEntityIndexByID(w.model.Entities, nil, state.TargetID)
	if hasTarget {
		target := &w.model.Entities[targetIdx]
		if !repairTurretTargetValid(*target, tile.Build.Team, tile.X, tile.Y, prof.Range) {
			hasTarget = false
			state.TargetID = 0
		}
	}

	state.SearchProgress += deltaFrames
	if state.SearchProgress >= 20 || !hasTarget {
		state.SearchProgress = float32mod(state.SearchProgress, 20)
		if idx, ok := w.findNearestDamagedFriendlyUnitLocked(tile.Build.Team, float32(tile.X*8+4), float32(tile.Y*8+4), prof.Range); ok {
			state.TargetID = w.model.Entities[idx].ID
			targetIdx = idx
			hasTarget = true
		} else {
			state.TargetID = 0
			hasTarget = false
		}
	}

	healed := false
	if hasTarget && w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*deltaSeconds) {
		target := &w.model.Entities[targetIdx]
		angle := lookAt(float32(tile.X*8+4), float32(tile.Y*8+4), target.X, target.Y)
		multiplier := float32(1)
		if prof.AcceptCoolant {
			if used, heatCap := consumeRepairTurretCoolantLocked(tile.Build, prof.CoolantUsePerFrame*deltaFrames); used {
				multiplier = 1 + heatCap*prof.CoolantMultiplier
			}
		}
		if angleDistDeg(angle, state.Rotation) < 30 {
			healed = true
			_ = w.healEntity(target, prof.RepairSpeed*state.Strength*deltaFrames*multiplier)
		}
		state.Rotation = lerpAngleDeltaDeg(state.Rotation, angle, 0.5, deltaFrames)
	}

	state.Strength = lerpDeltaf(state.Strength, condf(healed, 1, 0), 0.08, deltaFrames)
}

func (w *World) stepRepairTowers(deltaFrames, deltaSeconds float32) {
	for _, pos := range w.repairTowerTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Team == 0 || tile.Block == 0 {
			continue
		}
		prof, ok := repairTowerProfilesByBlockName[w.blockNameByID(int16(tile.Block))]
		if !ok {
			continue
		}
		state := w.repairTowerStates[pos]
		w.stepSingleRepairTowerLocked(pos, tile, prof, &state, deltaFrames, deltaSeconds)
		w.repairTowerStates[pos] = state
	}
}

func (w *World) stepSingleRepairTowerLocked(pos int32, tile *Tile, prof repairTowerProfile, state *repairTowerRuntimeState, deltaFrames, deltaSeconds float32) {
	if tile == nil || tile.Build == nil || state == nil {
		return
	}
	state.Refresh += deltaFrames
	if state.Refresh >= 6 {
		state.Refresh = float32mod(state.Refresh, 6)
		state.Targets = state.Targets[:0]
		cx := float32(tile.X*8 + 4)
		cy := float32(tile.Y*8 + 4)
		limit2 := prof.Range * prof.Range
		for i := range w.model.Entities {
			other := &w.model.Entities[i]
			if other.Team != tile.Build.Team || other.Health <= 0 {
				continue
			}
			if !entityNeedsRepair(*other) || squaredWorldDistance(cx, cy, other.X, other.Y) > limit2 {
				continue
			}
			state.Targets = append(state.Targets, other.ID)
		}
	}
	if w.isBuildingHealSuppressedLocked(tile.Build) {
		state.Warmup = 0
		return
	}
	if len(state.Targets) == 0 {
		state.Warmup = lerpDeltaf(state.Warmup, 0, 0.08, deltaFrames)
		return
	}
	if prof.PowerPerSecond > 0 && !w.requirePowerAtLocked(pos, tile.Build.Team, prof.PowerPerSecond*deltaSeconds) {
		state.Warmup = lerpDeltaf(state.Warmup, 0, 0.08, deltaFrames)
		return
	}
	if prof.RequiredLiquidPerFrame > 0 && !consumeBuildingLiquidLocked(tile.Build, prof.RequiredLiquid, prof.RequiredLiquidPerFrame*deltaFrames) {
		state.Warmup = lerpDeltaf(state.Warmup, 0, 0.08, deltaFrames)
		return
	}

	any := false
	for _, id := range state.Targets {
		idx, ok := findEntityIndexByID(w.model.Entities, nil, id)
		if !ok {
			continue
		}
		target := &w.model.Entities[idx]
		if target.Team != tile.Build.Team || !entityNeedsRepair(*target) {
			continue
		}
		if squaredWorldDistance(float32(tile.X*8+4), float32(tile.Y*8+4), target.X, target.Y) > prof.Range*prof.Range {
			continue
		}
		if w.healEntity(target, prof.HealAmount*deltaFrames) {
			any = true
		}
	}
	state.Warmup = lerpDeltaf(state.Warmup, condf(any, 1, 0), 0.08, deltaFrames)
	state.TotalProgress += deltaFrames / 120.0
}

func (w *World) findNearestDamagedFriendlyUnitLocked(team TeamID, fromX, fromY, rangeLimit float32) (int, bool) {
	if w == nil || w.model == nil {
		return -1, false
	}
	best := -1
	bestD2 := rangeLimit * rangeLimit
	for i := range w.model.Entities {
		other := &w.model.Entities[i]
		if other.Team != team || other.Health <= 0 || !entityNeedsRepair(*other) {
			continue
		}
		d2 := squaredWorldDistance(fromX, fromY, other.X, other.Y)
		if d2 > bestD2 {
			continue
		}
		bestD2 = d2
		best = i
	}
	return best, best >= 0
}

func repairTurretTargetValid(target RawEntity, team TeamID, tileX, tileY int, rangeLimit float32) bool {
	if target.Team != team || target.Health <= 0 || !entityNeedsRepair(target) {
		return false
	}
	return squaredWorldDistance(float32(tileX*8+4), float32(tileY*8+4), target.X, target.Y) <= rangeLimit*rangeLimit
}

func entityNeedsRepair(e RawEntity) bool {
	if e.Health <= 0 {
		return false
	}
	maxHealth := e.MaxHealth
	if hm := entityHealthMultiplier(e); hm > 0 {
		maxHealth *= hm
	}
	if maxHealth <= 0 {
		maxHealth = e.MaxHealth
	}
	return maxHealth > 0 && e.Health < maxHealth-0.001
}

func repairTurretAcceptsLiquid(liquid LiquidID) bool {
	return liquid == waterLiquidID || liquid == cryofluidLiquidID
}

func consumeRepairTurretCoolantLocked(build *Building, amount float32) (bool, float32) {
	if build == nil || amount <= 0 {
		return false, 0
	}
	for _, liquid := range []LiquidID{cryofluidLiquidID, waterLiquidID} {
		if consumeBuildingLiquidLocked(build, liquid, amount) {
			return true, repairTurretCoolantHeatCapacity(liquid)
		}
	}
	return false, 0
}

func repairTurretCoolantHeatCapacity(liquid LiquidID) float32 {
	switch liquid {
	case cryofluidLiquidID:
		return 0.9
	case waterLiquidID:
		return 0.4
	default:
		return 0
	}
}

func lerpAngleDeltaDeg(from, to, alphaPerFrame, deltaFrames float32) float32 {
	if deltaFrames <= 0 || alphaPerFrame <= 0 {
		return normalizeAngleDeg(from)
	}
	alpha := 1 - powf(1-clampf(alphaPerFrame, 0, 1), deltaFrames)
	diff := normalizeAngleDeg(to - from)
	return normalizeAngleDeg(from + diff*alpha)
}

func powf(base, exp float32) float32 {
	return float32(math.Pow(float64(base), float64(exp)))
}
