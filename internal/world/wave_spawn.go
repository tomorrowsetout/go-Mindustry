package world

import (
	"math"
	"strings"
)

// triggerWave 触发波次生成
func (w *World) triggerWave(wm *WaveManager) {
	if w.model == nil {
		w.wave++
		return
	}

	rules := w.rulesMgr.Get()
	if rules != nil && len(rules.Spawns) > 0 {
		w.triggerVanillaWaveLocked(rules)
		return
	}

	// Compatibility fallback for tests and programmatic worlds that still use
	// the old compact WaveManager configuration instead of vanilla spawn groups.
	nextWave := w.wave + 1
	w.wave = nextWave

	plan := wm.GeneratePlan(nextWave)
	if plan == nil {
		return
	}

	_, waveTeam := w.teamsFromRulesLocked()

	// 生成敌人（使用 RawEntity 结构）
	for group := 0; group < int(plan.GroupCount); group++ {
		for unitIdx := 0; unitIdx < int(plan.GroupSize); unitIdx++ {
			if len(w.model.Entities) >= 200 {
				break // 限制最大单位数量
			}

			enemyType := plan.EnemyTypePrior[0]
			if len(plan.EnemyTypePrior) > 0 {
				enemyType = plan.EnemyTypePrior[group%len(plan.EnemyTypePrior)]
			}

			posX, posY, ok := w.pickWaveSpawnPositionLocked(enemyType, waveTeam)
			if !ok {
				posX = float32(w.model.Width*8) / 2
				posY = float32(w.model.Height*8) / 2
			}
			posX += float32((unitIdx%3)-1) * 8
			posY += float32((group%3)-1) * 8
			posX = clampf(posX, 0, float32(w.model.Width*8))
			posY = clampf(posY, 0, float32(w.model.Height*8))
			w.addEnemy(enemyType, waveTeam, posX, posY, 0, 0)
		}

	}
}

func (w *World) triggerVanillaWaveLocked(rules *Rules) {
	waveIndex := w.wave - 1
	_, waveTeam := w.teamsFromRulesLocked()
	w.applyWaveDropZoneShockwavesLocked(waveTeam, rules)

	for _, group := range rules.Spawns {
		typeID, ok := w.resolveUnitTypeIDLocked(normalizeUnitName(group.Type))
		if !ok {
			continue
		}
		spawned := group.Spawned(waveIndex)
		if spawned <= 0 {
			continue
		}
		team := waveTeam
		if parsed, ok := parseTeamKey(group.Team); ok {
			team = parsed
		}
		shield := group.Shield(waveIndex)
		if w.waveGroupUsesFlyerSpawnsLocked(typeID, rules) {
			for _, spawn := range w.waveFlyerSpawnPositionsLocked(group.Spawn, rules) {
				for i := int32(0); i < spawned; i++ {
					rot := normalizeAngle360Deg(lookAt(spawn.X, spawn.Y, float32(w.model.Width*8)/2, float32(w.model.Height*8)/2))
					w.addWaveEnemy(typeID, team, spawn.X, spawn.Y, rot, shield)
				}
			}
			continue
		}
		for _, spawn := range w.waveGroundSpawnPositionsLocked(group.Spawn, rules) {
			for i := int32(0); i < spawned; i++ {
				rot := normalizeAngle360Deg(lookAt(spawn.X, spawn.Y, float32(w.model.Width*8)/2, float32(w.model.Height*8)/2))
				w.addWaveEnemy(typeID, team, spawn.X, spawn.Y, rot, shield)
			}
		}
	}
	w.wave++
}

func (w *World) waveGroupUsesFlyerSpawnsLocked(typeID int16, rules *Rules) bool {
	if rules != nil && rules.AirUseSpawns {
		return false
	}
	prof, _ := w.unitRuntimeProfileForTypeLocked(typeID)
	if prof.Flying {
		return true
	}
	name := w.unitLookupNameByID(typeID)
	switch normalizeUnitName(name) {
	case "flare", "horizon", "zenith", "antumbra", "eclipse", "mono", "poly", "mega", "quad", "oct",
		"alpha", "beta", "gamma", "quell", "disrupt":
		return true
	default:
		return false
	}
}

func (w *World) waveSpawnTilesLocked(filterPos int32) []Tile {
	if w == nil || w.model == nil {
		return nil
	}
	out := make([]Tile, 0, 4)
	for i := range w.model.Tiles {
		tile := w.model.Tiles[i]
		if !w.isSpawnOverlayLocked(tile.Overlay) {
			continue
		}
		if filterPos >= 0 && packTilePos(tile.X, tile.Y) != filterPos {
			continue
		}
		out = append(out, tile)
	}
	return out
}

func (w *World) isSpawnOverlayLocked(overlay OverlayID) bool {
	if overlay == 1 {
		return true
	}
	if w == nil || w.model == nil || w.model.BlockNames == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(w.model.BlockNames[int16(overlay)]), "spawn")
}

func (w *World) waveGroundSpawnPositionsLocked(filterPos int32, rules *Rules) []Vec2 {
	tiles := w.waveSpawnTilesLocked(filterPos)
	out := make([]Vec2, 0, len(tiles))
	for _, tile := range tiles {
		out = append(out, Vec2{X: float32(tile.X * 8), Y: float32(tile.Y * 8)})
	}
	return out
}

func (w *World) waveFlyerSpawnPositionsLocked(filterPos int32, rules *Rules) []Vec2 {
	tiles := w.waveSpawnTilesLocked(filterPos)
	out := make([]Vec2, 0, len(tiles))
	width := float32(w.model.Width)
	height := float32(w.model.Height)
	worldW := width * 8
	worldH := height * 8
	trns := maxf(worldW, worldH) * float32(math.Sqrt2)
	for _, tile := range tiles {
		if rules != nil && rules.AirUseSpawns {
			out = append(out, Vec2{X: float32(tile.X * 8), Y: float32(tile.Y * 8)})
			continue
		}
		angle := lookAt(width/2, height/2, float32(tile.X), float32(tile.Y))
		x := clampf(worldW/2+arcCosDeg(angle)*trns, 0, worldW)
		y := clampf(worldH/2+arcSinDeg(angle)*trns, 0, worldH)
		out = append(out, Vec2{X: x, Y: y})
	}
	return out
}

func (w *World) applyWaveDropZoneShockwavesLocked(waveTeam TeamID, rules *Rules) {
	radius := float32(300)
	if rules != nil && rules.DropZoneRadius > 0 {
		radius = rules.DropZoneRadius
	}
	for _, spawn := range w.waveGroundSpawnPositionsLocked(-1, rules) {
		w.applyCompleteBuildingDamageLocked(waveTeam, spawn.X, spawn.Y, radius, 99999999)
	}
}

func (w *World) applyCompleteBuildingDamageLocked(team TeamID, x, y, radius, damage float32) {
	if w == nil || w.model == nil || radius <= 0 || damage <= 0 {
		return
	}
	trad := int(radius / 8)
	cx := int(math.Floor(float64(x/8 + 0.5)))
	cy := int(math.Floor(float64(y/8 + 0.5)))
	for dx := -trad; dx <= trad; dx++ {
		for dy := -trad; dy <= trad; dy++ {
			if dx*dx+dy*dy > trad*trad {
				continue
			}
			tx, ty := cx+dx, cy+dy
			if !w.model.InBounds(tx, ty) {
				continue
			}
			pos := int32(ty*w.model.Width + tx)
			tile := &w.model.Tiles[pos]
			if tile.Build == nil || tile.Team == team {
				continue
			}
			_ = w.applyDamageToBuildingRaw(pos, damage)
		}
	}
}

func (w *World) vanillaAIControllerTypeForUnitLocked(typeID int16) string {
	name := w.unitLookupNameByID(typeID)
	prof, _ := w.unitRuntimeProfileForTypeLocked(typeID)
	switch defaultUnitAIKindByName(name, prof) {
	case unitAIDefender:
		return "DefenderAI"
	case unitAIFlying:
		return "FlyingAI"
	case unitAIFlyingFollow:
		return "FlyingFollowAI"
	case unitAISuicide:
		return "SuicideAI"
	case unitAIHug:
		return "HugAI"
	case unitAIBuilder:
		return "BuilderAI"
	case unitAICargo:
		return "CargoAI"
	case unitAIAssembler:
		return "AssemblerAI"
	case unitAIMissile:
		return "MissileAI"
	default:
		return "GroundAI"
	}
}
