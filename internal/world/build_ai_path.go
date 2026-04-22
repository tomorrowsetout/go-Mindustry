package world

import (
	"math"
	"sort"
)

const buildAIPathRefreshIntervalSec = float32(3 * 60)
const buildAISpawnProtectRadiusTiles = 40

const (
	buildAIWaveCoreSpawnMarginWorld = float32(2 * 8)
	buildAIWaveCoreSpawnStepWorld   = float32(8) * 1.1
	buildAIWaveCoreSpawnMaxSteps    = 30
	buildAIWaveCoreSpawnSolidRadius = 3
)

func (w *World) stepBuildAIRefreshPathsLocked(dt float32) {
	if w == nil || w.model == nil || w.rulesMgr == nil || dt <= 0 || len(w.teamPrimaryCore) == 0 {
		return
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.BuildAi || rules.Pvp || rules.Editor {
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
		if state.StartedPathing && state.RefreshPathCD > 0 {
			state.RefreshPathCD -= dt
			if state.RefreshPathCD > 0 {
				w.teamBuildAIStates[team] = state
				continue
			}
		}
		state.StartedPathing = true
		state.RefreshPathCD = buildAIPathRefreshIntervalSec
		if path, ok := w.computeBuildAIPathCellsLocked(team); ok && len(path) > 0 {
			state.PathCells = path
			state.FoundPath = true
		} else if len(state.PathCells) == 0 {
			state.FoundPath = false
		}
		w.teamBuildAIStates[team] = state
	}
}

func (w *World) computeBuildAIPathCellsLocked(team TeamID) (map[int32]struct{}, bool) {
	if w == nil || w.model == nil || team == 0 || w.model.Width <= 0 || w.model.Height <= 0 {
		return nil, false
	}
	sourceSet, enemyCoreCells, ok := w.buildAIEnemyCoreCellsLocked(team)
	if !ok || len(sourceSet) == 0 {
		return nil, false
	}
	spawnX, spawnY, ok := w.buildAIPathSpawnCellLocked(enemyCoreCells)
	if !ok {
		return nil, false
	}
	start := int32(spawnY*w.model.Width + spawnX)
	if _, ok := sourceSet[start]; !ok && w.groundCellBlockedLocked(spawnX, spawnY) {
		return nil, false
	}
	dist := make([]int32, len(w.model.Tiles))
	for i := range dist {
		dist[i] = -1
	}
	queue := make([]int32, 0, len(sourceSet))
	for pos := range sourceSet {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		dist[pos] = 0
		queue = append(queue, pos)
	}
	for head := 0; head < len(queue); head++ {
		cur := queue[head]
		cx := int(cur % int32(w.model.Width))
		cy := int(cur / int32(w.model.Width))
		for _, dir := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx := cx + dir[0]
			ny := cy + dir[1]
			if nx < 0 || ny < 0 || nx >= w.model.Width || ny >= w.model.Height {
				continue
			}
			next := int32(ny*w.model.Width + nx)
			if dist[next] >= 0 {
				continue
			}
			if _, isSource := sourceSet[next]; !isSource && w.groundCellBlockedLocked(nx, ny) {
				continue
			}
			dist[next] = dist[cur] + 1
			queue = append(queue, next)
		}
	}
	if dist[start] < 0 {
		return nil, false
	}
	path := make(map[int32]struct{}, 64)
	cur := start
	steps := 0
	for {
		w.addBuildAIPathCorridorLocked(path, cur)
		if _, ok := sourceSet[cur]; ok {
			return path, true
		}
		cx := int(cur % int32(w.model.Width))
		cy := int(cur / int32(w.model.Width))
		best := int32(-1)
		bestDist := dist[cur]
		for _, dir := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
			nx := cx + dir[0]
			ny := cy + dir[1]
			if nx < 0 || ny < 0 || nx >= w.model.Width || ny >= w.model.Height {
				continue
			}
			next := int32(ny*w.model.Width + nx)
			if dist[next] < 0 || dist[next] >= bestDist {
				continue
			}
			best = next
			bestDist = dist[next]
		}
		if best < 0 {
			return nil, false
		}
		cur = best
		steps++
		if steps > len(w.model.Tiles) {
			return nil, false
		}
	}
}

func (w *World) buildAIEnemyCoreCellsLocked(team TeamID) (map[int32]struct{}, []int32, bool) {
	if w == nil || w.model == nil || team == 0 {
		return nil, nil, false
	}
	sourceSet := make(map[int32]struct{})
	rootCells := make([]int32, 0, 4)
	for otherTeam, positions := range w.teamCoreTiles {
		if otherTeam == 0 || otherTeam == team {
			continue
		}
		rootCells = append(rootCells, positions...)
	}
	sort.Slice(rootCells, func(i, j int) bool {
		return rootCells[i] < rootCells[j]
	})
	for _, pos := range rootCells {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Block == 0 || tile.Build == nil {
			continue
		}
		low, high := blockFootprintRange(w.blockSizeForTileLocked(tile))
		for dy := low; dy <= high; dy++ {
			for dx := low; dx <= high; dx++ {
				x := tile.X + dx
				y := tile.Y + dy
				if !w.model.InBounds(x, y) {
					continue
				}
				sourceSet[int32(y*w.model.Width+x)] = struct{}{}
			}
		}
	}
	if len(sourceSet) == 0 {
		return nil, nil, false
	}
	return sourceSet, rootCells, true
}

func (w *World) buildAIPathSpawnCellLocked(enemyCoreCells []int32) (int, int, bool) {
	if w == nil || w.model == nil {
		return 0, 0, false
	}
	spawns := w.buildAIPathGroundSpawnCellsLocked()
	if len(spawns) == 0 {
		return 0, 0, false
	}
	x, y := unpackTilePos(spawns[len(spawns)-1])
	return x, y, true
}

func (w *World) buildAIPathGroundSpawnCellsLocked() []int32 {
	if w == nil || w.model == nil {
		return nil
	}
	out := append([]int32(nil), w.buildAIGroundSpawnCellsLocked()...)
	if w.rulesMgr == nil {
		return out
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.WavesSpawnAtCores || !rules.AttackMode {
		return out
	}
	_, waveTeam := w.teamsFromRulesLocked()
	if waveTeam == 0 {
		return out
	}
	firstPlayerCore, ok := w.firstBuildAIPathPlayerCoreTileLocked(waveTeam)
	if !ok {
		return out
	}
	for _, pos := range w.teamCoreTiles[waveTeam] {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Block == 0 || tile.Build == nil {
			continue
		}
		x, y, ok := w.buildAIWaveCoreSpawnCellLocked(tile, firstPlayerCore)
		if !ok {
			continue
		}
		out = append(out, packTilePos(x, y))
	}
	return out
}

func (w *World) buildAIGroundSpawnCellsLocked() []int32 {
	if w == nil || w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}
	out := make([]int32, 0, 4)
	for i := range w.model.Tiles {
		tile := &w.model.Tiles[i]
		if tile.Overlay <= 0 {
			continue
		}
		if w.blockNameByID(int16(tile.Overlay)) != "spawn" {
			continue
		}
		out = append(out, packTilePos(tile.X, tile.Y))
	}
	return out
}

func (w *World) firstBuildAIPathPlayerCoreTileLocked(waveTeam TeamID) (*Tile, bool) {
	if w == nil || w.model == nil {
		return nil, false
	}
	best := int32(-1)
	for team, positions := range w.teamCoreTiles {
		if team == 0 || team == waveTeam {
			continue
		}
		for _, pos := range positions {
			if best < 0 || pos < best {
				best = pos
			}
		}
	}
	if best >= 0 && int(best) < len(w.model.Tiles) {
		tile := &w.model.Tiles[best]
		if tile.Block != 0 && tile.Build != nil {
			return tile, true
		}
	}
	return nil, false
}

func (w *World) buildAIWaveCoreSpawnCellLocked(coreTile, firstPlayerCore *Tile) (int, int, bool) {
	if w == nil || w.model == nil || coreTile == nil || firstPlayerCore == nil {
		return 0, 0, false
	}
	coreWX, coreWY := tileCenterWorld(coreTile.X, coreTile.Y)
	targetWX, targetWY := tileCenterWorld(firstPlayerCore.X, firstPlayerCore.Y)
	dx := targetWX - coreWX
	dy := targetWY - coreWY
	curLen := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	minLen := buildAIWaveCoreSpawnMarginWorld + float32(w.blockSizeForTileLocked(coreTile))*4*float32(math.Sqrt2)
	switch {
	case curLen > minLen:
		scale := minLen / curLen
		dx *= scale
		dy *= scale
		curLen = minLen
	case curLen == 0:
		dx = -minLen
		dy = 0
		curLen = minLen
	}
	for steps := 0; steps < buildAIWaveCoreSpawnMaxSteps; steps++ {
		tx := int((coreWX + dx) / 8)
		ty := int((coreWY + dy) / 8)
		if w.buildAIWaveSpawnCellClearLocked(tx, ty, buildAIWaveCoreSpawnSolidRadius) {
			return tx, ty, true
		}
		nextLen := curLen + buildAIWaveCoreSpawnStepWorld
		if curLen > 0 {
			scale := nextLen / curLen
			dx *= scale
			dy *= scale
		} else {
			dx = nextLen
			dy = 0
		}
		curLen = nextLen
	}
	return 0, 0, false
}

func (w *World) buildAIWaveSpawnCellClearLocked(tx, ty, radius int) bool {
	if w == nil || w.model == nil || radius < 0 {
		return false
	}
	limit2 := radius * radius
	for dy := -radius; dy <= radius; dy++ {
		for dx := -radius; dx <= radius; dx++ {
			if dx*dx+dy*dy > limit2 {
				continue
			}
			if w.groundCellBlockedLocked(tx+dx, ty+dy) {
				return false
			}
		}
	}
	return true
}

func (w *World) buildAITileNearGroundSpawnLocked(x, y, radiusTiles int) bool {
	if w == nil || w.model == nil || radiusTiles <= 0 || !w.model.InBounds(x, y) {
		return false
	}
	limit := float32(radiusTiles * 8)
	limit2 := limit * limit
	wx, wy := tileCenterWorld(x, y)
	for _, packed := range w.buildAIGroundSpawnCellsLocked() {
		sx, sy := unpackTilePos(packed)
		px, py := tileCenterWorld(sx, sy)
		if squaredWorldDistance(wx, wy, px, py) <= limit2 {
			return true
		}
	}
	return false
}

func (w *World) addBuildAIPathCorridorLocked(path map[int32]struct{}, pos int32) {
	if w == nil || w.model == nil || path == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	x := int(pos % int32(w.model.Width))
	y := int(pos / int32(w.model.Width))
	for dy := -1; dy <= 1; dy++ {
		for dx := -1; dx <= 1; dx++ {
			nx := x + dx
			ny := y + dy
			if !w.model.InBounds(nx, ny) {
				continue
			}
			path[packTilePos(nx, ny)] = struct{}{}
		}
	}
}
