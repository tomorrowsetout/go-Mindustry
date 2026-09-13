package world

import (
	"sort"
	"strings"
)

func (w *World) stepPrebuildAICoreBuildersLocked() {
	if w == nil || w.model == nil || w.rulesMgr == nil || len(w.teamCoreTiles) == 0 {
		return
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.PrebuildAi || rules.Editor {
		return
	}
	positions := make([]int32, 0, 4)
	for team, corePositions := range w.teamCoreTiles {
		if team == 0 {
			continue
		}
		positions = append(positions, corePositions...)
	}
	sort.Slice(positions, func(i, j int) bool {
		return positions[i] < positions[j]
	})
	for _, pos := range positions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Block == 0 || tile.Build == nil || tile.Team == 0 {
			continue
		}
		w.ensurePrebuildCoreBuilderLocked(tile)
	}
}

func (w *World) ensurePrebuildCoreBuilderLocked(tile *Tile) {
	if w == nil || w.model == nil || tile == nil || tile.Block == 0 || tile.Team == 0 {
		return
	}
	typeID, ok := w.prebuildCoreBuilderTypeIDLocked(tile)
	if !ok || typeID <= 0 {
		return
	}
	coreFlag := float64(packTilePos(tile.X, tile.Y))
	for _, unit := range w.model.Entities {
		if unit.Health <= 0 || unit.Team != tile.Team || unit.TypeID != typeID {
			continue
		}
		if unit.Flag == coreFlag {
			return
		}
	}

	x, y := tileCenterWorld(tile.X, tile.Y)
	unit := w.newProducedUnitEntityLocked(typeID, tile.Team, x, y, 0)
	unit.SpawnedByCore = true
	unit.Flag = coreFlag
	w.model.AddEntity(unit)
}

func (w *World) prebuildCoreBuilderTypeIDLocked(tile *Tile) (int16, bool) {
	if w == nil || tile == nil || tile.Block == 0 {
		return 0, false
	}
	unitName, ok := coreBuilderUnitNameByBlockName(w.blockNameByID(int16(tile.Block)))
	if !ok {
		return 0, false
	}
	return w.resolveUnitTypeIDLocked(unitName)
}

func coreBuilderUnitNameByBlockName(blockName string) (string, bool) {
	name := strings.ToLower(strings.TrimSpace(blockName))
	switch {
	case strings.Contains(name, "core-shard"):
		return "alpha", true
	case strings.Contains(name, "core-foundation"):
		return "beta", true
	case strings.Contains(name, "core-nucleus"):
		return "gamma", true
	case strings.Contains(name, "core-bastion"):
		return "evoke", true
	case strings.Contains(name, "core-citadel"):
		return "incite", true
	case strings.Contains(name, "core-acropolis"):
		return "emanate", true
	default:
		return "", false
	}
}
