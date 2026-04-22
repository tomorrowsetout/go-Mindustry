package world

import (
	"math"
	"math/rand"
	"strings"

	"mdt-server/internal/vanilla"
)

const buildAIEmptyPartChance = float32(0.01)

type buildAIBasePartTile struct {
	BlockName string
	BlockID   int16
	X         int
	Y         int
	Rotation  int8
	Config    any
}

type buildAIBasePart struct {
	Name            string
	Width           int
	Height          int
	Tiles           []buildAIBasePartTile
	CenterX         int
	CenterY         int
	RequiredItem    ItemID
	HasRequiredItem bool
}

type rotatedBuildAIBasePart struct {
	Width  int
	Height int
	Tiles  []buildAIBasePartTile
}

func (w *World) buildAIBasePartsLocked() []buildAIBasePart {
	if w == nil {
		return nil
	}
	if w.buildAIPartsLoaded {
		return w.buildAIParts
	}
	w.buildAIPartsLoaded = true
	raw, err := vanilla.LoadEmbeddedBasePartSchematics()
	if err != nil || len(raw) == 0 {
		return nil
	}
	parts := make([]buildAIBasePart, 0, len(raw))
	for _, schematic := range raw {
		if part, ok := w.convertBuildAIBasePartLocked(schematic); ok {
			parts = append(parts, part)
		}
	}
	w.buildAIParts = parts
	return w.buildAIParts
}

func (w *World) convertBuildAIBasePartLocked(schematic vanilla.BasePartSchematic) (buildAIBasePart, bool) {
	if w == nil || schematic.Width <= 0 || schematic.Height <= 0 || len(schematic.Tiles) == 0 {
		return buildAIBasePart{}, false
	}
	part := buildAIBasePart{
		Name:   schematic.Name,
		Width:  schematic.Width,
		Height: schematic.Height,
		Tiles:  make([]buildAIBasePartTile, 0, len(schematic.Tiles)),
	}
	sumX := float32(0)
	sumY := float32(0)
	centerCount := 0
	hasCore := false
	for _, tile := range schematic.Tiles {
		name := normalizeBlockLookupName(tile.Block)
		switch name {
		case "itemsource":
			if ref, ok := tile.Config.(vanilla.BasePartContentRef); ok && ref.ContentType == vanilla.BasePartContentItem {
				part.RequiredItem = ItemID(ref.ID)
				part.HasRequiredItem = true
			}
			continue
		case "liquidsource", "powersource", "powervoid", "payloadsource", "payloadvoid", "heatsource":
			continue
		}
		if strings.HasPrefix(name, "core") {
			hasCore = true
		}
		blockID, ok := w.resolveBlockIDByNameLocked(tile.Block)
		if !ok || blockID <= 0 {
			return buildAIBasePart{}, false
		}
		part.Tiles = append(part.Tiles, buildAIBasePartTile{
			BlockName: strings.ToLower(strings.TrimSpace(tile.Block)),
			BlockID:   blockID,
			X:         tile.X,
			Y:         tile.Y,
			Rotation:  tile.Rotation,
			Config:    cloneEntityPlanConfig(tile.Config),
		})
		if buildAIBasePartCountsForCenter(name) {
			size := blockSizeByName(tile.Block)
			offset := buildAIBlockOffsetWorld(size)
			sumX += float32(tile.X*8) + offset
			sumY += float32(tile.Y*8) + offset
			centerCount++
		}
	}
	if hasCore || len(part.Tiles) == 0 {
		return buildAIBasePart{}, false
	}
	if centerCount > 0 {
		part.CenterX = int((sumX / float32(centerCount)) / 8)
		part.CenterY = int((sumY / float32(centerCount)) / 8)
	} else {
		part.CenterX = part.Width / 2
		part.CenterY = part.Height / 2
	}
	return part, true
}

func buildAIBasePartCountsForCenter(name string) bool {
	name = normalizeBlockLookupName(name)
	return strings.Contains(name, "drill") || strings.Contains(name, "pump")
}

func buildAIBlockOffsetWorld(size int) float32 {
	if size <= 0 {
		size = 1
	}
	return float32((size + 1) % 2 * 4)
}

func (w *World) tryQueueBuildAIBasePartPlanLocked(team TeamID, seedX, seedY int) bool {
	parts := w.buildAIBasePartsLocked()
	if len(parts) == 0 || w == nil || w.model == nil || team == 0 {
		return false
	}
	for attempt := 0; attempt < buildAIPlaceAttempts; attempt++ {
		x, y, ok := w.randomBuildAITileNearLocked(seedX, seedY)
		if !ok {
			continue
		}
		if w.buildAITileNearGroundSpawnLocked(x, y, buildAISpawnProtectRadiusTiles) {
			continue
		}
		candidates := w.buildAIPartCandidatesForTileLocked(parts, x, y)
		if len(candidates) == 0 {
			continue
		}
		part := candidates[rand.Intn(len(candidates))]
		rotation := rand.Intn(4)
		if w.queueBuildAIPartAtSeedLocked(team, part, x, y, rotation) {
			return true
		}
	}
	return false
}

func (w *World) randomBuildAITileNearLocked(seedX, seedY int) (int, int, bool) {
	if w == nil || w.model == nil {
		return 0, 0, false
	}
	x := seedX + rand.Intn(buildAISeedRangeTiles*2+1) - buildAISeedRangeTiles
	y := seedY + rand.Intn(buildAISeedRangeTiles*2+1) - buildAISeedRangeTiles
	if !w.model.InBounds(x, y) {
		return 0, 0, false
	}
	return x, y, true
}

func (w *World) buildAIPartCandidatesForTileLocked(parts []buildAIBasePart, x, y int) []buildAIBasePart {
	if len(parts) == 0 {
		return nil
	}
	var item ItemID
	hasItem := false
	if w != nil && w.model != nil && w.model.InBounds(x, y) {
		if result, ok := w.resolveMineTileLocked(packTilePos(x, y), true, false); ok {
			item = result.Item
			hasItem = true
		}
	}
	if hasItem {
		out := make([]buildAIBasePart, 0, 8)
		for _, part := range parts {
			if part.HasRequiredItem && part.RequiredItem == item {
				out = append(out, part)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if rand.Float32() > buildAIEmptyPartChance {
		return nil
	}
	out := make([]buildAIBasePart, 0, 8)
	for _, part := range parts {
		if !part.HasRequiredItem {
			out = append(out, part)
		}
	}
	return out
}

func (w *World) queueBuildAIPartAtSeedLocked(team TeamID, part buildAIBasePart, seedX, seedY, rotation int) bool {
	if w == nil || w.model == nil || team == 0 || len(part.Tiles) == 0 {
		return false
	}
	rotated := rotateBuildAIBasePart(part, rotation)
	centerX, centerY := rotateBuildAIPartCenter(part, rotation)
	cx := seedX - int(centerX)
	cy := seedY - int(centerY)
	ops := make([]BuildPlanOp, 0, len(rotated.Tiles))
	correct := 0
	incorrect := 0
	anyDrills := false
	for _, tile := range rotated.Tiles {
		realX := tile.X + cx
		realY := tile.Y + cy
		op := BuildPlanOp{
			X:        int32(realX),
			Y:        int32(realY),
			Rotation: tile.Rotation,
			BlockID:  tile.BlockID,
			Config:   cloneEntityPlanConfig(tile.Config),
		}
		if w.evaluateBuildPlanPlacementLocked(team, op) != BuildPlanPlacementReady {
			return false
		}
		if w.buildAIPlanIntersectsPathLocked(team, realX, realY, tile.BlockName) {
			return false
		}
		if buildAIBlockNeedsPayloadSpacing(tile.BlockName) && w.buildAIHasAdjacentBuildingLocked(realX, realY, blockSizeByName(tile.BlockName)) {
			return false
		}
		if part.HasRequiredItem && buildAIBasePartCountsForCenter(tile.BlockName) {
			anyDrills = true
			w.buildAICountRequiredOreFitLocked(realX, realY, blockSizeByName(tile.BlockName), part.RequiredItem, &correct, &incorrect)
		}
		ops = append(ops, op)
	}
	if anyDrills && (incorrect != 0 || correct == 0) {
		return false
	}
	for _, op := range ops {
		w.queueTeamBuildPlanBackLocked(team, op)
	}
	return true
}

func buildAIBlockNeedsPayloadSpacing(name string) bool {
	name = normalizeBlockLookupName(name)
	return strings.Contains(name, "payload")
}

func (w *World) buildAIHasAdjacentBuildingLocked(x, y, size int) bool {
	if w == nil || w.model == nil {
		return false
	}
	edges, ok := blockEdgeOffsetCache[size]
	if !ok {
		edges = computeBlockEdgeOffsets(size)
	}
	for _, edge := range edges {
		if _, occupied := w.buildingOccupyingCellLocked(x+edge[0], y+edge[1]); occupied {
			return true
		}
	}
	return false
}

func (w *World) buildAICountRequiredOreFitLocked(x, y, size int, want ItemID, correct, incorrect *int) {
	if w == nil || w.model == nil || correct == nil || incorrect == nil {
		return
	}
	low, high := blockFootprintRange(size)
	for ty := y + low; ty <= y+high; ty++ {
		for tx := x + low; tx <= x+high; tx++ {
			if !w.model.InBounds(tx, ty) {
				continue
			}
			if result, ok := w.resolveMineTileLocked(packTilePos(tx, ty), true, false); ok {
				if result.Item == want {
					*correct = *correct + 1
				} else {
					*incorrect = *incorrect + 1
				}
			}
		}
	}
}

func (w *World) buildAIPlanIntersectsPathLocked(team TeamID, x, y int, blockName string) bool {
	if w == nil || w.model == nil || team == 0 || !buildAIBlockCanBlockPath(blockName) {
		return false
	}
	state, ok := w.teamBuildAIStates[team]
	if !ok || len(state.PathCells) == 0 {
		return false
	}
	low, high := blockFootprintRange(blockSizeByName(blockName))
	for ty := y + low; ty <= y+high; ty++ {
		for tx := x + low; tx <= x+high; tx++ {
			if !w.model.InBounds(tx, ty) {
				continue
			}
			if _, ok := state.PathCells[packTilePos(tx, ty)]; ok {
				return true
			}
		}
	}
	return false
}

func buildAIBlockCanBlockPath(name string) bool {
	name = normalizeBlockLookupName(name)
	switch name {
	case
		// Official solid=false transport blocks.
		"conveyor", "titaniumconveyor", "armoredconveyor",
		"junction",
		"sorter", "invertedsorter", "overflowgate", "underflowgate",
		"router", "distributor",
		"duct", "armoredduct", "ductrouter", "overflowduct", "underflowduct", "ductjunction", "ductunloader",
		"plastaniumconveyor", "surgeconveyor", "surgerouter",
		// Official solid=false payload transport blocks.
		"payloadconveyor", "reinforcedpayloadconveyor", "payloadrouter", "reinforcedpayloadrouter",
		// Official solid=false liquid transport blocks.
		"conduit", "pulseconduit", "platedconduit", "reinforcedconduit",
		"liquidrouter", "liquidjunction", "reinforcedliquidjunction", "reinforcedliquidrouter":
		return false
	default:
		return true
	}
}

func rotateBuildAIBasePart(part buildAIBasePart, times int) rotatedBuildAIBasePart {
	times = ((times % 4) + 4) % 4
	out := rotatedBuildAIBasePart{
		Width:  part.Width,
		Height: part.Height,
		Tiles:  make([]buildAIBasePartTile, len(part.Tiles)),
	}
	copy(out.Tiles, part.Tiles)
	for step := 0; step < times; step++ {
		ox := out.Width / 2
		oy := out.Height / 2
		for i := range out.Tiles {
			size := blockSizeByName(out.Tiles[i].BlockName)
			offset := buildAIBlockOffsetWorld(size)
			wx := float32((out.Tiles[i].X-ox)*8) + offset
			wy := float32((out.Tiles[i].Y-oy)*8) + offset
			nx := -wy
			ny := wx
			out.Tiles[i].X = int(math.Floor(float64((nx - offset) / 8))) + ox
			out.Tiles[i].Y = int(math.Floor(float64((ny - offset) / 8))) + oy
			out.Tiles[i].Rotation = int8((int(out.Tiles[i].Rotation) + 1) % 4)
		}
		out.Width, out.Height = out.Height, out.Width
	}
	return out
}

func rotateBuildAIPartCenter(part buildAIBasePart, times int) (float32, float32) {
	times = ((times % 4) + 4) % 4
	axisX := float32(part.Width) / 2
	axisY := float32(part.Height) / 2
	x := float32(part.CenterX)
	y := float32(part.CenterY)
	for step := 0; step < times; step++ {
		dx := x - axisX
		dy := y - axisY
		x = -dy + axisX
		y = dx + axisY
	}
	return x, y
}
