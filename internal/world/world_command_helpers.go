package world

import (
	"math"

	"mdt-server/internal/protocol"
)

type RepairTarget struct {
	EntityID int32
	BuildPos int32
	X        float32
	Y        float32
}

func (w *World) FindNextPendingBuildPlan(team TeamID, owner int32) (BuildPlanOp, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.findNextPendingBuildPlanLocked(team, owner)
}

func (w *World) findNextPendingBuildPlanLocked(team TeamID, owner int32) (BuildPlanOp, bool) {
	if w == nil || w.model == nil || team == 0 {
		return BuildPlanOp{}, false
	}

	bestPriority := 3
	bestOrder := uint64(0)
	best := BuildPlanOp{}
	found := false

	considerBuild := func(pos int32, st pendingBuildState) {
		if st.Team != team {
			return
		}
		priority := pendingPlanPriority(owner, st.Owner)
		if priority >= bestPriority {
			if !found || priority != bestPriority || st.QueueOrder >= bestOrder {
				return
			}
		}
		x := int32(int(pos) % w.model.Width)
		y := int32(int(pos) / w.model.Width)
		bestPriority = priority
		bestOrder = st.QueueOrder
		best = BuildPlanOp{
			X:        x,
			Y:        y,
			Rotation: st.Rotation,
			BlockID:  st.BlockID,
			Config:   cloneEntityPlanConfig(st.Config),
		}
		found = true
	}

	considerBreak := func(pos int32, st pendingBreakState) {
		if st.Team != team {
			return
		}
		priority := pendingPlanPriority(owner, st.Owner)
		if priority >= bestPriority {
			if !found || priority != bestPriority || st.QueueOrder >= bestOrder {
				return
			}
		}
		x := int32(int(pos) % w.model.Width)
		y := int32(int(pos) / w.model.Width)
		bestPriority = priority
		bestOrder = st.QueueOrder
		best = BuildPlanOp{
			Breaking: true,
			X:        x,
			Y:        y,
		}
		found = true
	}

	for pos, st := range w.pendingBuilds {
		considerBuild(pos, st)
	}
	for pos, st := range w.pendingBreaks {
		considerBreak(pos, st)
	}

	return best, found
}

func pendingPlanPriority(owner, candidateOwner int32) int {
	switch {
	case owner != 0 && candidateOwner == owner:
		return 0
	case candidateOwner == 0:
		return 1
	default:
		return 2
	}
}

func (w *World) FindNearestDamagedFriendlyBuilding(team TeamID, fromX, fromY float32) (RepairTarget, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w == nil || w.model == nil || team == 0 {
		return RepairTarget{}, false
	}

	best := RepairTarget{}
	bestD2 := float32(-1)
	for _, pos := range w.teamBuildingTiles[team] {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 || tile.Team != team {
			continue
		}
		if tile.Build.Health <= 0 || tile.Build.MaxHealth <= 0 || tile.Build.Health >= tile.Build.MaxHealth-0.001 {
			continue
		}
		wx := float32(tile.X*8 + 4)
		wy := float32(tile.Y*8 + 4)
		d2 := squaredWorldDistance(fromX, fromY, wx, wy)
		if bestD2 < 0 || d2 < bestD2 {
			bestD2 = d2
			best = RepairTarget{
				BuildPos: packTilePos(tile.X, tile.Y),
				X:        wx,
				Y:        wy,
			}
		}
	}

	return best, bestD2 >= 0
}

func (w *World) FindNearestAssistConstructBuilder(team TeamID, followerID int32, fromX, fromY, buildRange, speed, radius float32) (RawEntity, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.findNearestAssistConstructBuilderLocked(team, followerID, fromX, fromY, buildRange, speed, radius)
}

func (w *World) findNearestAssistConstructBuilderLocked(team TeamID, followerID int32, fromX, fromY, buildRange, speed, radius float32) (RawEntity, bool) {
	if w == nil || w.model == nil || team == 0 {
		return RawEntity{}, false
	}
	if radius <= 0 {
		radius = 1500
	}

	best := RawEntity{}
	bestFound := false
	bestScore := float32(0)
	for _, other := range w.model.Entities {
		if !w.assistConstructBuilderMatchesLocked(other, team, followerID, fromX, fromY, buildRange, speed, radius) {
			continue
		}
		score := squaredWorldDistance(fromX, fromY, other.X, other.Y)
		if bestFound && score >= bestScore {
			continue
		}
		best = cloneRawEntity(other)
		bestFound = true
		bestScore = score
	}
	return best, bestFound
}

func (w *World) CanAssistFollowBuilder(team TeamID, leaderID, followerID int32, fromX, fromY, buildRange, speed, radius float32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w == nil || w.model == nil || team == 0 || leaderID == 0 {
		return false
	}
	for _, other := range w.model.Entities {
		if other.ID != leaderID {
			continue
		}
		return w.assistConstructBuilderMatchesLocked(other, team, followerID, fromX, fromY, buildRange, speed, radius)
	}
	return false
}

func (w *World) FindNearestPlayerBuilder(team TeamID, followerID int32, fromX, fromY float32) (RawEntity, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w == nil || w.model == nil || team == 0 {
		return RawEntity{}, false
	}

	best := RawEntity{}
	bestFound := false
	bestScore := float32(0)
	for _, other := range w.model.Entities {
		if other.ID == 0 || other.ID == followerID || other.Team != team || other.Health <= 0 {
			continue
		}
		if other.PlayerID == 0 || other.BuildSpeed <= 0 {
			continue
		}
		score := squaredWorldDistance(fromX, fromY, other.X, other.Y)
		if bestFound && score >= bestScore {
			continue
		}
		best = cloneRawEntity(other)
		bestFound = true
		bestScore = score
	}
	return best, bestFound
}

func (w *World) assistConstructBuilderMatchesLocked(other RawEntity, team TeamID, followerID int32, fromX, fromY, buildRange, speed, radius float32) bool {
	if other.ID == 0 || other.ID == followerID || other.Team != team || other.Health <= 0 {
		return false
	}
	if other.BuildSpeed <= 0 || !other.UpdateBuilding || len(other.Plans) == 0 {
		return false
	}
	if radius > 0 && squaredWorldDistance(fromX, fromY, other.X, other.Y) > radius*radius {
		return false
	}
	plan, ok := primaryAssistBuildPlan(other)
	if !ok {
		return false
	}
	return w.assistConstructPlanReachableLocked(team, plan, fromX, fromY, buildRange, speed)
}

func primaryAssistBuildPlan(entity RawEntity) (BuildPlanOp, bool) {
	if len(entity.Plans) == 0 {
		return BuildPlanOp{}, false
	}
	plan := entity.Plans[0]
	pos := protocol.UnpackPoint2(plan.Pos)
	return BuildPlanOp{
		Breaking: plan.Breaking,
		X:        pos.X,
		Y:        pos.Y,
		Rotation: int8(plan.Rotation),
		BlockID:  plan.BlockID,
		Config:   cloneEntityPlanConfig(plan.Config),
	}, true
}

func (w *World) assistConstructPlanReachableLocked(team TeamID, op BuildPlanOp, fromX, fromY, buildRange, speed float32) bool {
	buildCost, targetX, targetY, ok := w.assistConstructTargetLocked(team, op)
	if !ok {
		return false
	}
	if buildRange <= 0 {
		buildRange = vanillaBuilderRange
	}
	if speed <= 0 {
		return false
	}
	// Match BuilderAI exactly: Math.min(cons.dst(unit) - unit.type.buildRange, 0).
	dist := minf(float32(math.Sqrt(float64(squaredWorldDistance(fromX, fromY, targetX, targetY))))-buildRange, 0)
	return dist/speed < buildCost*0.9
}

func (w *World) assistConstructTargetLocked(team TeamID, op BuildPlanOp) (float32, float32, float32, bool) {
	if w == nil || w.model == nil || team == 0 || !w.model.InBounds(int(op.X), int(op.Y)) {
		return 0, 0, 0, false
	}
	pos := int32(int(op.Y)*w.model.Width + int(op.X))
	targetX, targetY := tileCenterWorld(int(op.X), int(op.Y))
	if op.Breaking {
		st, ok := w.pendingBreaks[pos]
		if !ok || st.Team != team || !st.VisualStart {
			return 0, 0, 0, false
		}
		return w.assistConstructBuildCostLocked(st.BlockID), targetX, targetY, true
	}
	st, ok := w.pendingBuilds[pos]
	if !ok || st.Team != team || !st.VisualPlaced {
		return 0, 0, 0, false
	}
	return w.assistConstructBuildCostLocked(st.BlockID), targetX, targetY, true
}

func (w *World) assistConstructBuildCostLocked(blockID int16) float32 {
	cost := float32(1)
	if name := w.blockNameByID(blockID); name != "" {
		if buildTime, ok := w.blockBuildTimesByName[name]; ok && buildTime > 0 {
			cost = buildTime
		}
	}
	if rules := w.rulesMgr.Get(); rules != nil && rules.BuildCostMultiplier > 0 {
		cost *= rules.BuildCostMultiplier
	}
	return cost
}
