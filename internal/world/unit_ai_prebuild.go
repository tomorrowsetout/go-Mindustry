package world

import "strings"

const (
	prebuildPlanScanSec = 2
	prebuildOreScanSec  = 1
)

func (w *World) builderAIUsesPrebuildFallbackLocked(e RawEntity) bool {
	if w == nil || e.Team == 0 || w.rulesMgr == nil {
		return false
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.PrebuildAi {
		return false
	}
	return w.teamIsAILocked(e.Team, rules)
}

func (w *World) teamIsAILocked(team TeamID, rules *Rules) bool {
	if team == 0 || rules == nil || rules.Pvp {
		return false
	}
	if !rules.Waves && !rules.AttackMode {
		return false
	}
	defaultTeam, _ := w.teamsFromRulesLocked()
	return team != defaultTeam
}

func (w *World) applyPrebuildAIMovementLocked(e *RawEntity, speed, dt float32) bool {
	if w == nil || e == nil {
		return false
	}
	w.syncEntityBuilderRuntimeLocked(e)

	state := w.unitAIStates[e.ID]
	if state.PrebuildPlanScanCD > 0 {
		state.PrebuildPlanScanCD -= dt
		if state.PrebuildPlanScanCD < 0 {
			state.PrebuildPlanScanCD = 0
		}
	}
	if state.PrebuildOreScanCD > 0 {
		state.PrebuildOreScanCD -= dt
		if state.PrebuildOreScanCD < 0 {
			state.PrebuildOreScanCD = 0
		}
	}

	if state.PrebuildCollectingItems {
		w.prebuildDoMiningLocked(e, &state, speed, dt)
		e.UpdateBuilding = false
		w.unitAIStates[e.ID] = state
		return true
	}

	if plan, ok := entityPrimaryPlanLocked(*e); ok {
		if !w.prebuildPlanValidLocked(e.Team, plan, state) {
			w.clearEntityBuilderPlansLocked(e, &state)
			e.MineTilePos = invalidEntityTilePos
			state.PrebuildCollectingItems = false
			state.PrebuildMining = false
			state.PrebuildHasTargetItem = false
			state.PrebuildOreTilePos = invalidEntityTilePos
			w.unitAIStates[e.ID] = state
			return true
		}

		target, ok := w.builderPlanTargetLocked(plan)
		if !ok {
			w.clearEntityBuilderPlansLocked(e, &state)
			w.unitAIStates[e.ID] = state
			return true
		}

		stopRadius := minf(vanillaBuilderRange-20, 100)
		if stopRadius < 24 {
			stopRadius = 24
		}
		w.prebuildMoveToWorldLocked(e, target.X, target.Y, stopRadius, speed, dt)
		e.UpdateBuilding = true
		w.unitAIStates[e.ID] = state
		return true
	}

	e.MineTilePos = invalidEntityTilePos
	state.PrebuildOreTilePos = invalidEntityTilePos
	state.PrebuildMining = false
	state.PrebuildHasTargetItem = false
	e.UpdateBuilding = true

	if state.PrebuildPlanScanCD > 0 {
		e.VelX, e.VelY = 0, 0
		w.unitAIStates[e.ID] = state
		return true
	}

	state.PrebuildPlanScanCD = prebuildPlanScanSec
	plan, ok := w.findNextPrebuildPlanLocked(*e)
	if !ok {
		e.VelX, e.VelY = 0, 0
		w.unitAIStates[e.ID] = state
		return true
	}

	op := buildPlanOpFromTeamBuildPlan(plan)
	if w.evaluateBuildPlanPlacementLocked(e.Team, op) != BuildPlanPlacementReady {
		e.VelX, e.VelY = 0, 0
		w.unitAIStates[e.ID] = state
		return true
	}

	state.PrebuildCollectingItems = !w.prebuildCanBuildBlockLocked(e.Team, plan.BlockID)
	state.PrebuildMining = false
	state.PrebuildHasTargetItem = false
	state.PrebuildOreTilePos = invalidEntityTilePos
	state.PrebuildLastPlanQueued = true
	state.PrebuildLastPlanX = plan.X
	state.PrebuildLastPlanY = plan.Y
	state.PrebuildLastPlanBlockID = plan.BlockID
	w.setEntityBuilderPlansLocked(e, []entityBuildPlan{buildPlanEntityFromTeamBuildPlan(plan)})
	e.UpdateBuilding = !state.PrebuildCollectingItems
	w.unitAIStates[e.ID] = state
	return true
}

func (w *World) findNextPrebuildPlanLocked(e RawEntity) (teamBuildPlan, bool) {
	if w == nil || w.model == nil || e.Team == 0 {
		return teamBuildPlan{}, false
	}
	if _, ok := w.findNearestFriendlyCoreLocked(e); !ok {
		return teamBuildPlan{}, false
	}
	plans := w.teamAIBuildPlans[e.Team]
	if len(plans) == 0 {
		return teamBuildPlan{}, false
	}

	out := plans[:0]
	best := -1
	bestScore := float32(0)
	for _, plan := range plans {
		if w.teamBuildPlanAlreadyBuiltLocked(plan) {
			continue
		}
		scoreable := (w.prebuildCanBuildBlockLocked(e.Team, plan.BlockID) || w.prebuildBlockMineableLocked(plan.BlockID)) &&
			w.prebuildPlanConnectedLocked(e.Team, plan)
		if scoreable {
			tx := float32(plan.X*8 + 4)
			ty := float32(plan.Y*8 + 4)
			score := squaredWorldDistance(e.X, e.Y, tx, ty)
			if best < 0 || score < bestScore {
				best = len(out)
				bestScore = score
			}
		}
		out = append(out, plan)
	}

	if len(out) == 0 {
		delete(w.teamAIBuildPlans, e.Team)
		return teamBuildPlan{}, false
	}
	w.teamAIBuildPlans[e.Team] = out
	if best >= 0 {
		return out[best], true
	}
	return out[0], true
}

func (w *World) prebuildCanBuildBlockLocked(team TeamID, blockID int16) bool {
	if w == nil || team == 0 || blockID <= 0 {
		return false
	}
	if rules := w.rulesMgr.Get(); rules != nil && rules.teamInfiniteResources(team) {
		return true
	}
	cost := w.pendingBuildScaledCostLocked(blockID)
	if len(cost) == 0 {
		return true
	}
	if _, ok := w.teamPrimaryCore[team]; !ok {
		return false
	}
	for _, stack := range cost {
		if stack.Amount <= 0 {
			continue
		}
		if w.availableBuildItemAmountLocked(team, stack.Item) < stack.Amount {
			return false
		}
	}
	return true
}

func (w *World) prebuildMissingItemLocked(team TeamID, blockID int16) (ItemID, bool) {
	if w == nil || team == 0 || blockID <= 0 {
		return 0, false
	}
	for _, stack := range w.pendingBuildScaledCostLocked(blockID) {
		if stack.Amount <= 0 {
			continue
		}
		if w.availableBuildItemAmountLocked(team, stack.Item) < stack.Amount {
			return stack.Item, true
		}
	}
	return 0, false
}

func (w *World) prebuildBlockMineableLocked(blockID int16) bool {
	if w == nil || blockID <= 0 {
		return false
	}
	for _, stack := range w.pendingBuildScaledCostLocked(blockID) {
		if stack.Amount <= 0 {
			continue
		}
		if !w.hasOreForItemLocked(stack.Item) {
			return false
		}
	}
	return true
}

func (w *World) hasOreForItemLocked(item ItemID) bool {
	if w == nil || w.model == nil {
		return false
	}
	for i := range w.model.Tiles {
		pos := packTilePos(w.model.Tiles[i].X, w.model.Tiles[i].Y)
		result, ok := w.resolveMineTileLocked(pos, true, true)
		if ok && result.Item == item {
			return true
		}
	}
	return false
}

func (w *World) prebuildPlanConnectedLocked(team TeamID, plan teamBuildPlan) bool {
	if w == nil || team == 0 {
		return false
	}
	if w.prebuildProductionLikeBlockLocked(plan.BlockID) {
		return true
	}
	return w.teamHasBuildingOverlapPlanLocked(team, int(plan.X), int(plan.Y), plan.BlockID)
}

func (w *World) prebuildProductionLikeBlockLocked(blockID int16) bool {
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(blockID)))
	switch {
	case strings.Contains(name, "drill"),
		strings.Contains(name, "pump"),
		strings.Contains(name, "extractor"),
		strings.Contains(name, "cultivator"):
		return true
	default:
		return false
	}
}

func (w *World) teamHasBuildingOverlapPlanLocked(team TeamID, x, y int, blockID int16) bool {
	if w == nil || w.model == nil || team == 0 {
		return false
	}
	size := blockSizeByName(w.blockNameByID(blockID))
	minAX, maxAX, minAY, maxAY := blockPlanBounds(x, y, size)

	visitPos := func(pos int32) bool {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			return false
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 {
			return false
		}
		planSize := w.blockSizeForTileLocked(tile)
		minBX, maxBX, minBY, maxBY := blockPlanBounds(tile.X, tile.Y, planSize)
		return minAX <= maxBX && maxAX >= minBX && minAY <= maxBY && maxAY >= minBY
	}

	if idx := w.teamBuildingSpatial[team]; idx != nil && idx.cellSize > 0 {
		const pad = 40
		minWX := minAX*8 - pad
		maxWX := (maxAX+1)*8 + pad
		minWY := minAY*8 - pad
		maxWY := (maxAY+1)*8 + pad
		minCX := minWX / idx.cellSize
		maxCX := maxWX / idx.cellSize
		minCY := minWY / idx.cellSize
		maxCY := maxWY / idx.cellSize
		for cy := minCY; cy <= maxCY; cy++ {
			for cx := minCX; cx <= maxCX; cx++ {
				for _, pos := range idx.cells[packSpatialCell(cx, cy)] {
					if visitPos(pos) {
						return true
					}
				}
			}
		}
		return false
	}

	for _, pos := range w.teamBuildingTiles[team] {
		if visitPos(pos) {
			return true
		}
	}
	return false
}

func (w *World) findClosestMineTileForItemLocked(fromX, fromY float32, item ItemID, mineFloor, mineWalls bool, tier int) (int32, float32, float32, bool) {
	if w == nil || w.model == nil || tier < 0 {
		return 0, 0, 0, false
	}
	bestPos := int32(-1)
	bestX, bestY := float32(0), float32(0)
	bestD2 := float32(-1)
	for i := range w.model.Tiles {
		tile := &w.model.Tiles[i]
		pos := packTilePos(tile.X, tile.Y)
		result, ok := w.resolveMineTileLocked(pos, mineFloor, mineWalls)
		if !ok || result.Item != item || result.Hardness > tier {
			continue
		}
		d2 := squaredWorldDistance(fromX, fromY, result.WorldX, result.WorldY)
		if bestPos < 0 || d2 < bestD2 {
			bestPos = pos
			bestX = result.WorldX
			bestY = result.WorldY
			bestD2 = d2
		}
	}
	if bestPos < 0 {
		return 0, 0, 0, false
	}
	return bestPos, bestX, bestY, true
}

func (w *World) prebuildMoveToWorldLocked(e *RawEntity, targetX, targetY, stopRadius, speed, dt float32) {
	if e == nil {
		return
	}
	if reachedTarget(e.X, e.Y, targetX, targetY, stopRadius) {
		e.VelX, e.VelY = 0, 0
		return
	}
	if isEntityFlying(*e) {
		setVelocityToTarget(e, targetX, targetY, speed, stopRadius)
		return
	}
	target := unitAITarget{X: targetX, Y: targetY, Radius: stopRadius}
	wx, wy, ok := w.nextGroundWaypointLocked(*e, target, stopRadius, dt, false)
	if !ok {
		e.VelX, e.VelY = 0, 0
		return
	}
	setVelocityToTarget(e, wx, wy, speed, unitAIWaypointReach)
}

func (w *World) prebuildDoMiningLocked(e *RawEntity, state *unitAIState, speed, dt float32) {
	if w == nil || e == nil || state == nil {
		return
	}
	plan, ok := entityPrimaryPlanLocked(*e)
	if !ok {
		state.PrebuildCollectingItems = false
		state.PrebuildMining = false
		state.PrebuildHasTargetItem = false
		state.PrebuildOreTilePos = invalidEntityTilePos
		e.MineTilePos = invalidEntityTilePos
		e.VelX, e.VelY = 0, 0
		return
	}
	profile, ok := entityMiningProfile(*e)
	if !ok {
		state.PrebuildCollectingItems = false
		state.PrebuildMining = false
		state.PrebuildHasTargetItem = false
		state.PrebuildOreTilePos = invalidEntityTilePos
		e.MineTilePos = invalidEntityTilePos
		e.VelX, e.VelY = 0, 0
		return
	}

	core, ok := w.findNearestFriendlyCoreLocked(*e)
	if !ok {
		e.MineTilePos = invalidEntityTilePos
		state.PrebuildOreTilePos = invalidEntityTilePos
		e.VelX, e.VelY = 0, 0
		return
	}

	if e.MineTilePos >= 0 {
		if result, ok := w.resolveMineTileLocked(e.MineTilePos, profile.MineFloor, profile.MineWalls); !ok || result.Hardness > profile.Tier {
			e.MineTilePos = invalidEntityTilePos
			state.PrebuildOreTilePos = invalidEntityTilePos
		}
	}

	if state.PrebuildMining {
		targetItem, ok := w.prebuildMissingItemLocked(e.Team, plan.BlockID)
		if ok {
			state.PrebuildLastTargetItem = targetItem
			state.PrebuildHasTargetItem = true
		} else if e.Stack.Amount == 0 && w.prebuildCanBuildBlockLocked(e.Team, plan.BlockID) {
			state.PrebuildCollectingItems = false
			state.PrebuildMining = false
			state.PrebuildHasTargetItem = false
			state.PrebuildOreTilePos = invalidEntityTilePos
			e.MineTilePos = invalidEntityTilePos
			e.VelX, e.VelY = 0, 0
			return
		}

		if !ok && !state.PrebuildHasTargetItem {
			e.MineTilePos = invalidEntityTilePos
			state.PrebuildOreTilePos = invalidEntityTilePos
			e.VelX, e.VelY = 0, 0
			return
		}
		targetItem = state.PrebuildLastTargetItem

		if w.acceptItemAtLocked(core.BuildPos, targetItem, 1) == 0 {
			e.Stack = ItemStack{}
			e.MineTilePos = invalidEntityTilePos
			state.PrebuildOreTilePos = invalidEntityTilePos
			e.VelX, e.VelY = 0, 0
			return
		}

		if !entityCanCarryMinedItem(*e, targetItem, profile.Capacity) {
			state.PrebuildMining = false
		} else {
			if state.PrebuildOreScanCD <= 0 || state.PrebuildOreTilePos < 0 {
				state.PrebuildOreScanCD = prebuildOreScanSec
				state.PrebuildOreTilePos = invalidEntityTilePos
				if pos, _, _, ok := w.findClosestMineTileForItemLocked(e.X, e.Y, targetItem, profile.MineFloor, profile.MineWalls, profile.Tier); ok {
					state.PrebuildOreTilePos = pos
				}
			}
			if state.PrebuildOreTilePos >= 0 {
				result, ok := w.resolveMineTileLocked(state.PrebuildOreTilePos, profile.MineFloor, profile.MineWalls)
				if ok {
					w.prebuildMoveToWorldLocked(e, result.WorldX, result.WorldY, unitMineRange*0.5, speed, dt)
					if reachedTarget(e.X, e.Y, result.WorldX, result.WorldY, unitMineRange) {
						e.MineTilePos = state.PrebuildOreTilePos
					}
					return
				}
			}
			e.MineTilePos = invalidEntityTilePos
			e.VelX, e.VelY = 0, 0
			return
		}
	}

	e.MineTilePos = invalidEntityTilePos
	state.PrebuildOreTilePos = invalidEntityTilePos
	if e.Stack.Amount == 0 {
		state.PrebuildMining = true
		if w.prebuildCanBuildBlockLocked(e.Team, plan.BlockID) {
			state.PrebuildCollectingItems = false
			state.PrebuildHasTargetItem = false
		}
		e.VelX, e.VelY = 0, 0
		return
	}

	depositRange := maxf(e.AttackRange, 60)
	if reachedTarget(e.X, e.Y, core.X, core.Y, depositRange) {
		_ = w.depositEntityStackToCoreLocked(e, core.BuildPos)
		state.PrebuildMining = true
		if w.prebuildCanBuildBlockLocked(e.Team, plan.BlockID) {
			state.PrebuildCollectingItems = false
			state.PrebuildHasTargetItem = false
		}
		e.VelX, e.VelY = 0, 0
		return
	}

	w.prebuildMoveToWorldLocked(e, core.X, core.Y, depositRange/1.8, speed, dt)
}
