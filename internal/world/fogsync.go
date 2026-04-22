package world

import (
	"strings"

	"mdt-server/internal/protocol"
)

const (
	defaultUnitFogVisionWorld     = float32(12 * 8)
	defaultCoreFogVisionWorld     = float32(22 * 8)
	defaultBuildingFogVisionWorld = float32(10 * 8)
)

func (w *World) UnitSyncHiddenForViewer(viewerTeam TeamID, viewerX, viewerY float32, entity *protocol.UnitEntitySync) bool {
	if w == nil || entity == nil || viewerTeam == 0 {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.unitSyncHiddenForViewerLocked(viewerTeam, viewerX, viewerY, entity)
}

func (w *World) unitSyncHiddenForViewerLocked(viewerTeam TeamID, viewerX, viewerY float32, entity *protocol.UnitEntitySync) bool {
	if w == nil || entity == nil {
		return false
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.Fog {
		return false
	}
	targetTeam := TeamID(entity.TeamID)
	if targetTeam == viewerTeam {
		return false
	}
	if entity.Shooting {
		return false
	}

	hitRadius := float32(8)
	for i := range w.model.Entities {
		raw := &w.model.Entities[i]
		if raw.ID != entity.ID() {
			continue
		}
		if raw.Team != 0 {
			targetTeam = raw.Team
			if targetTeam == viewerTeam {
				return false
			}
		}
		if raw.Shooting {
			return false
		}
		if raw.HitRadius > 0 {
			hitRadius = raw.HitRadius
		}
		break
	}

	if w.visibleToTeamLocked(viewerTeam, viewerX, viewerY, entity.X, entity.Y, hitRadius) {
		return false
	}
	return true
}

func (w *World) visibleToTeamLocked(viewerTeam TeamID, viewerX, viewerY, targetX, targetY, hitRadius float32) bool {
	if viewerTeam == 0 {
		return true
	}
	if w.model == nil {
		return true
	}
	if hitRadius <= 0 {
		hitRadius = 8
	}
	if w.sourceCanSeeTargetLocked(viewerTeam, viewerX, viewerY, defaultUnitFogVisionWorld, targetX, targetY, hitRadius) {
		return true
	}

	for _, entity := range w.model.Entities {
		if entity.Team != viewerTeam || entity.Health <= 0 {
			continue
		}
		rangeWorld := defaultUnitFogVisionWorld
		if entity.HitRadius > 0 && entity.HitRadius > rangeWorld/6 {
			rangeWorld = maxf(rangeWorld, entity.HitRadius*6)
		}
		if w.sourceCanSeeTargetLocked(viewerTeam, entity.X, entity.Y, rangeWorld, targetX, targetY, hitRadius) {
			return true
		}
	}

	for _, pos := range w.teamCoreTiles[viewerTeam] {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Health <= 0 {
			continue
		}
		sx := float32(tile.X*8 + 4)
		sy := float32(tile.Y*8 + 4)
		if w.sourceCanSeeTargetLocked(viewerTeam, sx, sy, defaultCoreFogVisionWorld, targetX, targetY, hitRadius) {
			return true
		}
	}

	for _, pos := range w.teamBuildingTiles[viewerTeam] {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Build.Health <= 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
		if !buildingProvidesFogVision(name) {
			continue
		}
		sx := float32(tile.X*8 + 4)
		sy := float32(tile.Y*8 + 4)
		if w.sourceCanSeeTargetLocked(viewerTeam, sx, sy, defaultBuildingFogVisionWorld, targetX, targetY, hitRadius) {
			return true
		}
	}

	return false
}

func buildingProvidesFogVision(name string) bool {
	switch {
	case strings.Contains(name, "core"):
		return true
	case strings.Contains(name, "radar"):
		return true
	case strings.Contains(name, "repair"):
		return true
	case strings.Contains(name, "projector"):
		return true
	default:
		return false
	}
}

func (w *World) sourceCanSeeTargetLocked(viewerTeam TeamID, sourceX, sourceY, rangeWorld, targetX, targetY, hitRadius float32) bool {
	if rangeWorld <= 0 {
		return false
	}
	if squaredWorldDistance(sourceX, sourceY, targetX, targetY) > (rangeWorld+hitRadius)*(rangeWorld+hitRadius) {
		return false
	}
	if hitRadius <= 8 {
		return w.pointVisibleToTeamLocked(viewerTeam, sourceX, sourceY, targetX, targetY)
	}
	samples := [][2]float32{
		{targetX, targetY},
		{targetX + hitRadius, targetY},
		{targetX - hitRadius, targetY},
		{targetX, targetY + hitRadius},
		{targetX, targetY - hitRadius},
	}
	for _, sample := range samples {
		if squaredWorldDistance(sourceX, sourceY, sample[0], sample[1]) > (rangeWorld+hitRadius)*(rangeWorld+hitRadius) {
			continue
		}
		if w.pointVisibleToTeamLocked(viewerTeam, sourceX, sourceY, sample[0], sample[1]) {
			return true
		}
	}
	return false
}

func (w *World) pointVisibleToTeamLocked(viewerTeam TeamID, sourceX, sourceY, targetX, targetY float32) bool {
	result := w.raycastLocked(sourceX, sourceY, targetX, targetY, viewerTeam, true)
	return result == nil || !result.Hit
}
