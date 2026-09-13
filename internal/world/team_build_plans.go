package world

type teamBuildPlan struct {
	X        int32
	Y        int32
	Rotation int8
	BlockID  int16
	Config   any
}

func buildPlanOpFromTeamBuildPlan(plan teamBuildPlan) BuildPlanOp {
	return BuildPlanOp{
		X:        plan.X,
		Y:        plan.Y,
		Rotation: plan.Rotation,
		BlockID:  plan.BlockID,
		Config:   cloneEntityPlanConfig(plan.Config),
	}
}

func buildPlanEntityFromTeamBuildPlan(plan teamBuildPlan) entityBuildPlan {
	return buildPlanEntityFromOp(buildPlanOpFromTeamBuildPlan(plan))
}

func (w *World) queueTeamBuildPlanFrontLocked(team TeamID, op BuildPlanOp) {
	if w == nil || team == 0 || op.Breaking || op.BlockID <= 0 {
		return
	}
	plans := w.teamAIBuildPlans[team]
	out := plans[:0]
	for _, plan := range plans {
		if plan.X == op.X && plan.Y == op.Y {
			continue
		}
		out = append(out, plan)
	}
	next := teamBuildPlan{
		X:        op.X,
		Y:        op.Y,
		Rotation: op.Rotation,
		BlockID:  op.BlockID,
		Config:   cloneEntityPlanConfig(op.Config),
	}
	w.teamAIBuildPlans[team] = append([]teamBuildPlan{next}, out...)
}

func (w *World) queueTeamBuildPlanBackLocked(team TeamID, op BuildPlanOp) {
	if w == nil || team == 0 || op.Breaking || op.BlockID <= 0 {
		return
	}
	plans := w.teamAIBuildPlans[team]
	out := plans[:0]
	for _, plan := range plans {
		if plan.X == op.X && plan.Y == op.Y {
			continue
		}
		out = append(out, plan)
	}
	next := teamBuildPlan{
		X:        op.X,
		Y:        op.Y,
		Rotation: op.Rotation,
		BlockID:  op.BlockID,
		Config:   cloneEntityPlanConfig(op.Config),
	}
	w.teamAIBuildPlans[team] = append(out, next)
}

func (w *World) teamBuildPlanStillPresentLocked(team TeamID, x, y int32, blockID int16) bool {
	if w == nil || team == 0 {
		return false
	}
	for _, plan := range w.teamAIBuildPlans[team] {
		if plan.X == x && plan.Y == y && plan.BlockID == blockID {
			return true
		}
	}
	return false
}

func (w *World) clearTeamBuildPlanAtLocked(team TeamID, x, y int32) {
	if w == nil || team == 0 {
		return
	}
	plans := w.teamAIBuildPlans[team]
	if len(plans) == 0 {
		return
	}
	out := plans[:0]
	for _, plan := range plans {
		if plan.X == x && plan.Y == y {
			continue
		}
		out = append(out, plan)
	}
	if len(out) == 0 {
		delete(w.teamAIBuildPlans, team)
		return
	}
	w.teamAIBuildPlans[team] = out
}

func (w *World) clearOverlappingTeamBuildPlansLocked(team TeamID, x, y int, blockID int16) {
	if w == nil || w.model == nil || team == 0 {
		return
	}
	plans := w.teamAIBuildPlans[team]
	if len(plans) == 0 {
		return
	}
	size := blockSizeByName(w.blockNameByID(blockID))
	minAX, maxAX, minAY, maxAY := blockPlanBounds(x, y, size)
	out := plans[:0]
	for _, plan := range plans {
		planSize := blockSizeByName(w.blockNameByID(plan.BlockID))
		minBX, maxBX, minBY, maxBY := blockPlanBounds(int(plan.X), int(plan.Y), planSize)
		if minAX <= maxBX && maxAX >= minBX && minAY <= maxBY && maxAY >= minBY {
			continue
		}
		out = append(out, plan)
	}
	if len(out) == 0 {
		delete(w.teamAIBuildPlans, team)
		return
	}
	w.teamAIBuildPlans[team] = out
}

func (w *World) teamBuildPlanAlreadyBuiltLocked(plan teamBuildPlan) bool {
	if w == nil || w.model == nil || !w.model.InBounds(int(plan.X), int(plan.Y)) {
		return true
	}
	tile := &w.model.Tiles[int(plan.Y)*w.model.Width+int(plan.X)]
	return tile.Block == BlockID(plan.BlockID)
}

func (w *World) prebuildLastPlanStillPresentLocked(team TeamID, state unitAIState) bool {
	if w == nil || team == 0 || !state.PrebuildLastPlanQueued {
		return true
	}
	return w.teamBuildPlanStillPresentLocked(team, state.PrebuildLastPlanX, state.PrebuildLastPlanY, state.PrebuildLastPlanBlockID)
}

func (w *World) prebuildPlanValidLocked(team TeamID, plan entityBuildPlan, state unitAIState) bool {
	if w == nil || w.model == nil || team == 0 {
		return false
	}
	if !w.prebuildLastPlanStillPresentLocked(team, state) {
		return false
	}
	op := builderPlanOpFromEntity(plan)
	if !w.model.InBounds(int(op.X), int(op.Y)) {
		return false
	}
	pos := int32(int(op.Y)*w.model.Width + int(op.X))
	if st, ok := w.pendingBuilds[pos]; ok && st.Team == team && st.BlockID == op.BlockID {
		return true
	}
	return w.evaluateBuildPlanPlacementLocked(team, op) == BuildPlanPlacementReady
}
