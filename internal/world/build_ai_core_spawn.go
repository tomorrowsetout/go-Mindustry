package world

import (
	"math/rand"
	"strings"
)

const (
	buildAICoreSpawnIntervalSec = float32(6)
	buildAICoreUnitMultiplier   = 2
)

func (w *World) stepBuildAICoreSpawnLocked(dt float32) {
	if w == nil || w.model == nil || w.rulesMgr == nil || dt <= 0 || len(w.teamPrimaryCore) == 0 {
		return
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.BuildAi || rules.Pvp || rules.Editor || !rules.AiCoreSpawn {
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
		if state.SpawnCD <= 0 {
			state.SpawnCD = buildAICoreSpawnIntervalSec
			w.teamBuildAIStates[team] = state
			continue
		}
		state.SpawnCD -= dt
		if state.SpawnCD > 0 {
			w.teamBuildAIStates[team] = state
			continue
		}
		state.SpawnCD = buildAICoreSpawnIntervalSec
		w.trySpawnBuildAICoreUnitLocked(team)
		w.teamBuildAIStates[team] = state
	}
}

func (w *World) trySpawnBuildAICoreUnitLocked(team TeamID) bool {
	if w == nil || w.model == nil || team == 0 {
		return false
	}
	coreTile, ok := w.randomBuildAICoreTileLocked(team)
	if !ok {
		return false
	}
	typeID, ok := w.buildAICoreSpawnUnitTypeIDLocked(coreTile)
	if !ok || typeID <= 0 {
		return false
	}
	coreCount := len(w.teamCoreBuildsLocked(team))
	if coreCount <= 0 {
		return false
	}
	if w.buildAITeamUnitTypeCountLocked(team, typeID) >= coreCount*buildAICoreUnitMultiplier {
		return false
	}
	x, y := tileCenterWorld(coreTile.X, coreTile.Y)
	unit := w.newProducedUnitEntityLocked(typeID, team, x, y, 0)
	unit.SpawnedByCore = true
	w.model.AddEntity(unit)
	return true
}

func (w *World) randomBuildAICoreTileLocked(team TeamID) (*Tile, bool) {
	if w == nil || w.model == nil || team == 0 {
		return nil, false
	}
	cores := w.teamCoreTiles[team]
	if len(cores) == 0 {
		return nil, false
	}
	pos := cores[rand.Intn(len(cores))]
	tile := &w.model.Tiles[pos]
	return tile, true
}

func (w *World) buildAITeamUnitTypeCountLocked(team TeamID, typeID int16) int {
	if w == nil || w.model == nil || team == 0 || typeID <= 0 {
		return 0
	}
	count := 0
	for i := range w.model.Entities {
		entity := w.model.Entities[i]
		if entity.Health <= 0 || entity.Team != team || entity.TypeID != typeID {
			continue
		}
		count++
	}
	return count
}

func (w *World) buildAICoreSpawnUnitTypeIDLocked(tile *Tile) (int16, bool) {
	if w == nil || tile == nil || tile.Block == 0 {
		return 0, false
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	switch {
	case strings.Contains(name, "core-shard"):
		return w.resolveUnitTypeIDLocked("alpha")
	case strings.Contains(name, "core-foundation"):
		return w.resolveUnitTypeIDLocked("beta")
	case strings.Contains(name, "core-nucleus"):
		return w.resolveUnitTypeIDLocked("gamma")
	case strings.Contains(name, "core-bastion"):
		return w.resolveUnitTypeIDLocked("evoke")
	case strings.Contains(name, "core-citadel"):
		return w.resolveUnitTypeIDLocked("incite")
	case strings.Contains(name, "core-acropolis"):
		return w.resolveUnitTypeIDLocked("emanate")
	default:
		return 0, false
	}
}
