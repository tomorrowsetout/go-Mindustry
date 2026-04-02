package world

func (w *World) stepHeatConductorsLocked() {
	if w == nil || w.model == nil {
		return
	}
	memo := make(map[int32]float32)
	visiting := make(map[int32]struct{})
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		if !isHeatConductorBlockName(w.blockNameByID(int16(tile.Block))) {
			continue
		}
		heat := w.computeHeatConductorLocked(pos, memo, visiting)
		if heat <= 0.0001 {
			delete(w.heatStates, pos)
			continue
		}
		w.heatStates[pos] = heat
	}
}

func (w *World) updateCrafterHeatStateLocked(pos int32, tile *Tile, prof crafterProfile, active bool, deltaFrames float32) {
	if w == nil || tile == nil || tile.Build == nil || prof.HeatOutput <= 0 {
		return
	}
	rate := prof.HeatWarmupRate
	if rate <= 0 {
		rate = 0.15
	}
	target := float32(0)
	if active {
		target = prof.HeatOutput * w.crafterBaseEfficiencyMultiplierLocked(tile, prof)
	}
	value := approachf(w.heatStates[pos], target, rate*deltaFrames)
	if value <= 0.0001 {
		delete(w.heatStates, pos)
		return
	}
	w.heatStates[pos] = value
}

func (w *World) crafterBaseEfficiencyMultiplierLocked(tile *Tile, prof crafterProfile) float32 {
	if tile == nil {
		return 0
	}
	base := prof.BaseEfficiency
	if prof.Attribute == "" && base == 0 {
		return 1
	}
	attrsum := float32(0)
	boost := float32(0)
	if prof.Attribute != "" && prof.BoostScale != 0 {
		attrsum = w.sumFloorAttributeLocked(tile, prof.Attribute)
		maxBoost := prof.MaxBoost
		if maxBoost <= 0 {
			maxBoost = 1
		}
		boost = minf(maxBoost, prof.BoostScale*attrsum)
	}
	if prof.MinEfficiency > 0 && base+attrsum < prof.MinEfficiency {
		return 0
	}
	return base + boost
}

func (w *World) crafterHeatEfficiencyScaleLocked(pos int32, tile *Tile, prof crafterProfile) float32 {
	if w == nil || tile == nil || prof.HeatRequirement <= 0 {
		return 1
	}
	heat := w.crafterReceivedHeatLocked(pos, tile)
	if heat <= 0 {
		return 0
	}
	req := prof.HeatRequirement
	scale := clampf(heat/req, 0, 1)
	if heat > req && prof.OverheatScale > 0 {
		scale += ((heat - req) / req) * prof.OverheatScale
	}
	maxEfficiency := prof.MaxEfficiency
	if maxEfficiency <= 0 {
		maxEfficiency = 1
	}
	if scale > maxEfficiency {
		scale = maxEfficiency
	}
	return scale
}

func (w *World) crafterWarmupTargetLocked(pos int32, tile *Tile, prof crafterProfile) float32 {
	if prof.HeatRequirement <= 0 {
		return 1
	}
	if w == nil || tile == nil {
		return 0
	}
	return clampf(w.crafterReceivedHeatLocked(pos, tile)/prof.HeatRequirement, 0, 1)
}

func (w *World) crafterReceivedHeatLocked(pos int32, tile *Tile) float32 {
	if w == nil || w.model == nil || tile == nil || tile.Build == nil {
		return 0
	}
	total := float32(0)
	for _, otherPos := range w.dumpProximityLocked(pos) {
		if otherPos < 0 || int(otherPos) >= len(w.model.Tiles) || otherPos == pos {
			continue
		}
		other := &w.model.Tiles[otherPos]
		if other.Build == nil || other.Block == 0 || other.Team != tile.Team {
			continue
		}
		heat := w.heatProviderAmountLocked(otherPos, other, nil, nil)
		if heat <= 0 {
			continue
		}
		if !w.heatCanTransferLocked(otherPos, other, pos, tile, nil, nil) {
			continue
		}
		contact := w.heatContactPointsLocked(other, tile)
		if contact <= 0 {
			continue
		}
		size := w.blockSizeForTileLocked(other)
		if size <= 0 {
			size = 1
		}
		add := heat / float32(size) * float32(contact)
		if isHeatRouterBlockName(w.blockNameByID(int16(other.Block))) {
			add /= 3
		}
		total += add
	}
	return total
}

func (w *World) computeHeatConductorLocked(pos int32, memo map[int32]float32, visiting map[int32]struct{}) float32 {
	if heat, ok := memo[pos]; ok {
		return heat
	}
	if _, seen := visiting[pos]; seen {
		return 0
	}
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 || !isHeatConductorBlockName(w.blockNameByID(int16(tile.Block))) {
		return 0
	}
	visiting[pos] = struct{}{}
	total := float32(0)
	for _, otherPos := range w.dumpProximityLocked(pos) {
		if otherPos < 0 || int(otherPos) >= len(w.model.Tiles) || otherPos == pos {
			continue
		}
		other := &w.model.Tiles[otherPos]
		if !w.heatCanTransferLocked(otherPos, other, pos, tile, memo, visiting) {
			continue
		}
		heat := w.heatProviderAmountLocked(otherPos, other, memo, visiting)
		if heat <= 0 {
			continue
		}
		contact := w.heatContactPointsLocked(other, tile)
		if contact <= 0 {
			continue
		}
		size := w.blockSizeForTileLocked(other)
		if size <= 0 {
			size = 1
		}
		add := heat / float32(size) * float32(contact)
		if isHeatRouterBlockName(w.blockNameByID(int16(other.Block))) {
			add /= 3
		}
		total += add
	}
	delete(visiting, pos)
	memo[pos] = total
	return total
}

func (w *World) heatProviderAmountLocked(pos int32, tile *Tile, memo map[int32]float32, visiting map[int32]struct{}) float32 {
	if w == nil || tile == nil || tile.Build == nil || tile.Block == 0 {
		return 0
	}
	name := w.blockNameByID(int16(tile.Block))
	if isHeatConductorBlockName(name) {
		if memo == nil && visiting == nil {
			return w.heatStates[pos]
		}
		if memo == nil {
			memo = make(map[int32]float32)
		}
		if visiting == nil {
			visiting = make(map[int32]struct{})
		}
		return w.computeHeatConductorLocked(pos, memo, visiting)
	}
	return w.heatStates[pos]
}

func (w *World) heatCanTransferLocked(fromPos int32, fromTile *Tile, toPos int32, toTile *Tile, memo map[int32]float32, visiting map[int32]struct{}) bool {
	if w == nil || fromTile == nil || toTile == nil || fromTile.Build == nil || toTile.Build == nil || fromTile.Block == 0 || toTile.Block == 0 {
		return false
	}
	if fromTile.Team != toTile.Team {
		return false
	}
	if w.heatProviderAmountLocked(fromPos, fromTile, memo, visiting) <= 0 {
		return false
	}
	dir, ok := w.heatDirectionBetweenLocked(fromTile, toTile)
	if !ok {
		return false
	}
	name := w.blockNameByID(int16(fromTile.Block))
	if isHeatRouterBlockName(name) {
		return int(dir) != tileRotationNorm(fromTile.Rotation)
	}
	return int(dir) == tileRotationNorm(fromTile.Rotation)
}

func isHeatConductorBlockName(name string) bool {
	switch name {
	case "heat-redirector", "small-heat-redirector", "heat-router":
		return true
	default:
		return false
	}
}

func isHeatRouterBlockName(name string) bool {
	return name == "heat-router"
}

func (w *World) heatDirectionBetweenLocked(fromTile, toTile *Tile) (byte, bool) {
	if w == nil || fromTile == nil || toTile == nil {
		return 0, false
	}
	lowA, highA := blockFootprintRange(w.blockSizeForTileLocked(fromTile))
	lowB, highB := blockFootprintRange(w.blockSizeForTileLocked(toTile))
	leftA, rightA := fromTile.X+lowA, fromTile.X+highA
	topA, bottomA := fromTile.Y+lowA, fromTile.Y+highA
	leftB, rightB := toTile.X+lowB, toTile.X+highB
	topB, bottomB := toTile.Y+lowB, toTile.Y+highB
	switch {
	case rightA < leftB:
		return 0, true
	case bottomA < topB:
		return 1, true
	case leftA > rightB:
		return 2, true
	case topA > bottomB:
		return 3, true
	default:
		return 0, false
	}
}

func (w *World) heatContactPointsLocked(a, b *Tile) int {
	if w == nil || a == nil || b == nil {
		return 0
	}
	lowA, highA := blockFootprintRange(w.blockSizeForTileLocked(a))
	lowB, highB := blockFootprintRange(w.blockSizeForTileLocked(b))
	leftA, rightA := a.X+lowA, a.X+highA
	topA, bottomA := a.Y+lowA, a.Y+highA
	leftB, rightB := b.X+lowB, b.X+highB
	topB, bottomB := b.Y+lowB, b.Y+highB
	if rightA < leftB || leftA > rightB {
		overlapTop := maxInt(topA, topB)
		overlapBottom := minInt(bottomA, bottomB)
		if overlapBottom < overlapTop {
			return 0
		}
		return overlapBottom - overlapTop + 1
	}
	if bottomA < topB || topA > bottomB {
		overlapLeft := maxInt(leftA, leftB)
		overlapRight := minInt(rightA, rightB)
		if overlapRight < overlapLeft {
			return 0
		}
		return overlapRight - overlapLeft + 1
	}
	return 0
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
