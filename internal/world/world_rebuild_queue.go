package world

type BuildPlanPlacementStatus byte

const (
	BuildPlanPlacementBlocked BuildPlanPlacementStatus = iota
	BuildPlanPlacementReady
	BuildPlanPlacementBuilt
)

func (w *World) DamageBuildingPacked(packed int32, damage float32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w == nil || w.model == nil || damage <= 0 {
		return false
	}
	x, y := unpackTilePos(packed)
	if !w.model.InBounds(x, y) {
		return false
	}
	if w.rulesMgr != nil {
		if rules := w.rulesMgr.Get(); rules != nil && rules.BlockHealthMultiplier > 0 {
			damage /= rules.BlockHealthMultiplier
		}
	}
	return w.applyDamageToBuildingRaw(int32(y*w.model.Width+x), damage)
}

func (w *World) AcquireNextRebuildPlan(team TeamID) (BuildPlanOp, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.acquireNextRebuildPlanLocked(team)
}

func (w *World) acquireNextRebuildPlanLocked(team TeamID) (BuildPlanOp, bool) {
	if w == nil || w.model == nil || team == 0 {
		return BuildPlanOp{}, false
	}
	plans := w.teamRebuildPlans[team]
	for len(plans) > 0 {
		head := plans[0]
		if w.rebuildPlanAlreadyBuiltLocked(head) {
			plans = plans[1:]
			continue
		}
		if w.canPlaceRebuildPlanLocked(team, head) {
			if len(plans) > 1 {
				plans = append(plans[1:], head)
			}
			w.teamRebuildPlans[team] = plans
			return BuildPlanOp{
				X:        head.X,
				Y:        head.Y,
				Rotation: head.Rotation,
				BlockID:  head.BlockID,
				Config:   cloneEntityPlanConfig(head.Config),
			}, true
		}
		if len(plans) > 1 {
			plans = append(plans[1:], head)
		}
		w.teamRebuildPlans[team] = plans
		return BuildPlanOp{}, false
	}
	delete(w.teamRebuildPlans, team)
	return BuildPlanOp{}, false
}

func (w *World) AcquireNextRebuildPlanInRange(team TeamID, x, y, buildRange float32) (BuildPlanOp, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w == nil || w.model == nil || team == 0 {
		return BuildPlanOp{}, false
	}
	if buildRange <= 0 {
		buildRange = vanillaBuilderRange
	}
	plans := w.teamRebuildPlans[team]
	if len(plans) == 0 {
		return BuildPlanOp{}, false
	}
	out := plans[:0]
	selected := -1
	for _, plan := range plans {
		if w.rebuildPlanAlreadyBuiltLocked(plan) {
			continue
		}
		if selected < 0 && w.rebuildPlanWithinRangeLocked(plan, x, y, buildRange) && w.canPlaceRebuildPlanLocked(team, plan) {
			selected = len(out)
		}
		out = append(out, plan)
	}
	if len(out) == 0 {
		delete(w.teamRebuildPlans, team)
		return BuildPlanOp{}, false
	}
	if selected < 0 {
		w.teamRebuildPlans[team] = out
		return BuildPlanOp{}, false
	}
	plan := out[selected]
	copy(out[selected:], out[selected+1:])
	out = out[:len(out)-1]
	out = append(out, plan)
	w.teamRebuildPlans[team] = out
	return buildPlanOpFromRebuildPlan(plan), true
}

func (w *World) EvaluateBuildPlanPlacement(team TeamID, op BuildPlanOp) BuildPlanPlacementStatus {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.evaluateBuildPlanPlacementLocked(team, op)
}

func (w *World) evaluateBuildPlanPlacementLocked(team TeamID, op BuildPlanOp) BuildPlanPlacementStatus {
	if w == nil || w.model == nil || op.Breaking || op.BlockID <= 0 || !w.model.InBounds(int(op.X), int(op.Y)) {
		return BuildPlanPlacementBlocked
	}
	pos := int32(int(op.Y)*w.model.Width + int(op.X))
	tile := &w.model.Tiles[pos]
	if tile.Block == BlockID(op.BlockID) && tile.Build != nil {
		return BuildPlanPlacementBuilt
	}
	if rules := w.rulesMgr.Get(); rules != nil && rules.DerelictRepair && tile.Team == 0 && tile.Block == BlockID(op.BlockID) {
		return BuildPlanPlacementReady
	}
	if tile.Block != 0 || tile.Build != nil {
		return BuildPlanPlacementBlocked
	}
	name := w.blockNameByID(op.BlockID)
	size := blockSizeByName(name)
	low, high := blockFootprintRange(size)
	center := int32(int(op.Y)*w.model.Width + int(op.X))
	for dy := low; dy <= high; dy++ {
		for dx := low; dx <= high; dx++ {
			tx := int(op.X) + dx
			ty := int(op.Y) + dy
			if !w.model.InBounds(tx, ty) {
				return BuildPlanPlacementBlocked
			}
			if occPos, ok := w.buildingOccupyingCellLocked(tx, ty); ok && occPos != center {
				return BuildPlanPlacementBlocked
			}
			cell := &w.model.Tiles[ty*w.model.Width+tx]
			if cell.Block != 0 && cell.Build == nil {
				return BuildPlanPlacementBlocked
			}
		}
	}
	return BuildPlanPlacementReady
}

func (w *World) queueBrokenBuildPlanLocked(pos int32, tile *Tile) {
	if w == nil || w.model == nil || tile == nil || tile.Block == 0 {
		return
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.GhostBlocks {
		return
	}
	team := tile.Team
	if tile.Build != nil && tile.Build.Team != 0 {
		team = tile.Build.Team
	}
	if team == 0 || !w.blockSupportsRebuildQueueLocked(int16(tile.Block)) {
		return
	}

	config := w.rebuildPlanConfigLocked(pos, tile)
	out := w.teamRebuildPlans[team][:0]
	for _, plan := range w.teamRebuildPlans[team] {
		if plan.X == int32(tile.X) && plan.Y == int32(tile.Y) {
			continue
		}
		out = append(out, plan)
	}
	plan := rebuildBlockPlan{
		X:        int32(tile.X),
		Y:        int32(tile.Y),
		Rotation: tile.Rotation,
		BlockID:  int16(tile.Block),
		Config:   cloneEntityPlanConfig(config),
	}
	w.teamRebuildPlans[team] = append([]rebuildBlockPlan{plan}, out...)
	w.queueTeamBuildPlanFrontLocked(team, BuildPlanOp{
		X:        int32(tile.X),
		Y:        int32(tile.Y),
		Rotation: tile.Rotation,
		BlockID:  int16(tile.Block),
		Config:   cloneEntityPlanConfig(config),
	})
}

func (w *World) rebuildPlanConfigLocked(pos int32, tile *Tile) any {
	if w == nil || tile == nil || tile.Build == nil {
		return nil
	}
	if value, ok := w.normalizedBuildingConfigLocked(pos); ok {
		return cloneEntityPlanConfig(value)
	}
	if len(tile.Build.Config) == 0 {
		return nil
	}
	if decoded, ok := decodeStoredBuildingConfig(tile.Build.Config); ok {
		return cloneEntityPlanConfig(decoded)
	}
	return nil
}

func (w *World) blockSupportsRebuildQueueLocked(blockID int16) bool {
	name := w.blockNameByID(blockID)
	if name == "" {
		return false
	}
	switch name {
	case "base-shield", "thorium-reactor", "impact-reactor", "neoplasia-reactor", "flux-reactor":
		return false
	default:
		return true
	}
}

func (w *World) rebuildPlanAlreadyBuiltLocked(plan rebuildBlockPlan) bool {
	if w == nil || w.model == nil || !w.model.InBounds(int(plan.X), int(plan.Y)) {
		return true
	}
	tile := &w.model.Tiles[int(plan.Y)*w.model.Width+int(plan.X)]
	return tile.Block == BlockID(plan.BlockID)
}

func (w *World) canPlaceRebuildPlanLocked(team TeamID, plan rebuildBlockPlan) bool {
	return w.evaluateBuildPlanPlacementLocked(team, BuildPlanOp{
		X:        plan.X,
		Y:        plan.Y,
		Rotation: plan.Rotation,
		BlockID:  plan.BlockID,
		Config:   plan.Config,
	}) == BuildPlanPlacementReady
}

func buildPlanOpFromRebuildPlan(plan rebuildBlockPlan) BuildPlanOp {
	return BuildPlanOp{
		X:        plan.X,
		Y:        plan.Y,
		Rotation: plan.Rotation,
		BlockID:  plan.BlockID,
		Config:   cloneEntityPlanConfig(plan.Config),
	}
}

func (w *World) rebuildPlanWithinRangeLocked(plan rebuildBlockPlan, x, y, buildRange float32) bool {
	targetX := float32(plan.X*8 + 4)
	targetY := float32(plan.Y*8 + 4)
	return squaredWorldDistance(x, y, targetX, targetY) <= buildRange*buildRange
}

func (w *World) clearOverlappingRebuildPlansLocked(team TeamID, x, y int, blockID int16) {
	if w == nil || w.model == nil || team == 0 {
		return
	}
	plans := w.teamRebuildPlans[team]
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
		delete(w.teamRebuildPlans, team)
		return
	}
	w.teamRebuildPlans[team] = out
}

func blockPlanBounds(x, y, size int) (minX, maxX, minY, maxY int) {
	low, high := blockFootprintRange(size)
	return x + low, x + high, y + low, y + high
}
