package world

import "strings"

const (
	buildAIPlaceAttempts    = 6
	buildAIPlaceIntervalMin = float32(12)
	buildAIPlaceIntervalMax = float32(2)
	buildAISeedRangeTiles   = 18
)

func (w *World) stepBuildAIPlansLocked(dt float32) {
	if w == nil || w.model == nil || w.rulesMgr == nil || dt <= 0 {
		return
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.BuildAi || rules.Pvp || rules.Editor || len(w.teamPrimaryCore) == 0 {
		return
	}
	if w.teamBuildAIStates == nil {
		w.teamBuildAIStates = map[TeamID]buildAIPlannerState{}
	}
	for team := range w.teamPrimaryCore {
		if team == 0 || !w.teamHasCoreLocked(team) {
			continue
		}
		state := w.teamBuildAIStates[team]
		if state.PlanScanCD > 0 {
			state.PlanScanCD -= dt
			if state.PlanScanCD < 0 {
				state.PlanScanCD = 0
			}
		}
		if len(w.teamAIBuildPlans[team]) != 0 || state.PlanScanCD > 0 {
			w.teamBuildAIStates[team] = state
			continue
		}
		state.PlanScanCD = buildAIPlaceIntervalLocked(rules)
		for attempt := 0; attempt < buildAIPlaceAttempts; attempt++ {
			if w.tryQueueBuildAIPlanLocked(team) {
				break
			}
		}
		w.teamBuildAIStates[team] = state
	}
}

func buildAIPlaceIntervalLocked(rules *Rules) float32 {
	if rules == nil {
		return buildAIPlaceIntervalMin
	}
	tier := clampf(float32(rules.BuildAiTier), 0, 1)
	return buildAIPlaceIntervalMin + (buildAIPlaceIntervalMax-buildAIPlaceIntervalMin)*tier
}

func (w *World) tryQueueBuildAIPlanLocked(team TeamID) bool {
	if w == nil || w.model == nil || team == 0 {
		return false
	}
	coreX, coreY, ok := w.randomBuildAISeedTileLocked(team)
	if !ok {
		return false
	}
	if w.tryQueueBuildAIBasePartPlanLocked(team, coreX, coreY) {
		return true
	}
	if op, ok := w.findBuildAIDrillPlanLocked(team, coreX, coreY); ok {
		w.queueTeamBuildPlanBackLocked(team, op)
		return true
	}
	return false
}

func (w *World) randomBuildAISeedTileLocked(team TeamID) (int, int, bool) {
	if w == nil || w.model == nil || team == 0 {
		return 0, 0, false
	}
	if tile, ok := w.randomBuildAICoreTileLocked(team); ok && tile != nil {
		return tile.X, tile.Y, true
	}
	return w.primaryCoreTileLocked(team)
}

func (w *World) primaryCoreTileLocked(team TeamID) (int, int, bool) {
	if w == nil || w.model == nil || team == 0 {
		return 0, 0, false
	}
	pos, ok := w.teamPrimaryCore[team]
	if !ok || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0, 0, false
	}
	tile := &w.model.Tiles[pos]
	if tile.Block == 0 || tile.Build == nil || tile.Team != team {
		return 0, 0, false
	}
	return tile.X, tile.Y, true
}

func (w *World) findBuildAIDrillPlanLocked(team TeamID, seedX, seedY int) (BuildPlanOp, bool) {
	if w == nil || w.model == nil || team == 0 {
		return BuildPlanOp{}, false
	}
	drillID, ok := w.resolveBlockIDByNameLocked("mechanical-drill")
	if !ok || drillID <= 0 {
		return BuildPlanOp{}, false
	}
	bestScore := int(^uint(0) >> 1)
	best := BuildPlanOp{}
	found := false
	for i := range w.model.Tiles {
		tile := &w.model.Tiles[i]
		if abs(tile.X-seedX) > buildAISeedRangeTiles || abs(tile.Y-seedY) > buildAISeedRangeTiles {
			continue
		}
		if !w.buildAIDrillHasOreLocked(tile.X, tile.Y) {
			continue
		}
		if w.buildAITileNearGroundSpawnLocked(tile.X, tile.Y, buildAISpawnProtectRadiusTiles) {
			continue
		}
		op := BuildPlanOp{
			X:       int32(tile.X),
			Y:       int32(tile.Y),
			BlockID: drillID,
		}
		if w.evaluateBuildPlanPlacementLocked(team, op) != BuildPlanPlacementReady {
			continue
		}
		if w.buildAIPlanIntersectsPathLocked(team, tile.X, tile.Y, "mechanical-drill") {
			continue
		}
		score := abs(tile.X-seedX) + abs(tile.Y-seedY)
		if !found || score < bestScore {
			best = op
			bestScore = score
			found = true
		}
	}
	return best, found
}

func (w *World) buildAIDrillHasOreLocked(x, y int) bool {
	if w == nil || w.model == nil || !w.model.InBounds(x, y) {
		return false
	}
	size := blockSizeByName("mechanical-drill")
	low, high := blockFootprintRange(size)
	for ty := y + low; ty <= y+high; ty++ {
		for tx := x + low; tx <= x+high; tx++ {
			if !w.model.InBounds(tx, ty) {
				return false
			}
			if _, ok := w.resolveMineTileLocked(packTilePos(tx, ty), true, false); ok {
				return true
			}
		}
	}
	return false
}

func (w *World) resolveBlockIDByNameLocked(want string) (int16, bool) {
	want = normalizeBlockLookupName(want)
	for id, name := range w.blockNamesByID {
		if normalizeBlockLookupName(name) == want {
			return id, true
		}
	}
	switch want {
	case "mechanicaldrill":
		return 429, true
	}
	return 0, false
}

func normalizeBlockLookupName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, " ", "")
	return name
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
