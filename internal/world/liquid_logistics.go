package world

import (
	"sort"
	"strings"
)

func (w *World) normalizeBuildingLiquidCurrentsLocked() {
	if w == nil || w.model == nil {
		return
	}
	for i := range w.model.Tiles {
		normalizeBuildingLiquidCurrent(w.model.Tiles[i].Build)
	}
}

func totalBuildingLiquids(build *Building) float32 {
	if build == nil {
		return 0
	}
	total := float32(0)
	for _, stack := range build.Liquids {
		if stack.Amount > 0 {
			total += stack.Amount
		}
	}
	return total
}

func (w *World) liquidHasCapacityLocked(tile *Tile, liquid LiquidID) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	return tile.Build.LiquidAmount(liquid) < w.liquidCapacityForBlockLocked(tile)-0.0001
}

func (w *World) liquidCanStoreLocked(tile *Tile, liquid LiquidID) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	if !w.liquidHasCapacityLocked(tile, liquid) {
		return false
	}
	current, amount, ok := currentBuildingLiquid(tile.Build)
	return !ok || current == liquid || amount < 0.2
}

func (w *World) conduitAcceptsLiquidLocked(fromPos, toPos int32, liquid LiquidID, armored bool) bool {
	if w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil || !w.liquidCanStoreLocked(toTile, liquid) {
		return false
	}
	sourceSide, ok := w.flowDirBetweenLocked(fromPos, toPos)
	if !ok {
		return false
	}
	if sourceSide == byte((tileRotationNorm(toTile.Rotation)+2)%4) {
		return false
	}
	if !armored {
		return true
	}
	fromTile := &w.model.Tiles[fromPos]
	fromName := w.blockNameByID(int16(fromTile.Block))
	if fromName == "conduit" || fromName == "pulse-conduit" || fromName == "plated-conduit" || fromName == "reinforced-conduit" ||
		fromName == "reinforced-bridge-conduit" || fromName == "liquid-junction" || fromName == "reinforced-liquid-junction" {
		return true
	}
	return sourceSide == byte(tileRotationNorm(toTile.Rotation))
}

func (w *World) canAcceptLiquidLocked(fromPos, toPos int32, liquid LiquidID, depth int) bool {
	if depth > 8 || w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	fromTile := &w.model.Tiles[fromPos]
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil || toTile.Block == 0 || toTile.Team != fromTile.Team {
		return false
	}
	name := w.blockNameByID(int16(toTile.Block))
	switch name {
	case "conduit", "pulse-conduit":
		return w.conduitAcceptsLiquidLocked(fromPos, toPos, liquid, false)
	case "plated-conduit":
		return w.conduitAcceptsLiquidLocked(fromPos, toPos, liquid, true)
	case "reinforced-conduit":
		return w.conduitAcceptsLiquidLocked(fromPos, toPos, liquid, false)
	case "liquid-void":
		return true
	case "liquid-router", "liquid-container", "liquid-tank", "reinforced-liquid-router", "reinforced-liquid-container", "reinforced-liquid-tank", "thorium-reactor":
		return w.liquidCanStoreLocked(toTile, liquid)
	case "liquid-source":
		return false
	case "bridge-conduit", "phase-conduit":
		return w.bridgeAllowsInputLocked(fromPos, toPos) && w.liquidCanStoreLocked(toTile, liquid)
	case "mechanical-pump", "rotary-pump", "impulse-pump":
		return false
	case "reinforced-bridge-conduit":
		if _, ok := w.directionBridgeTargetLocked(toPos, toTile, "reinforced-bridge-conduit", 4); !ok {
			return false
		}
		rel, ok := w.relativeToEdgeLocked(fromPos, toPos)
		return ok && rel != byte(tileRotationNorm(toTile.Rotation)) && w.liquidCanStoreLocked(toTile, liquid)
	case "liquid-junction", "reinforced-liquid-junction":
		_, ok := w.liquidJunctionDestinationLocked(fromPos, toPos, liquid, depth+1)
		return ok
	case "incinerator":
		return w.incineratorAcceptsLiquidLocked(toPos, liquid)
	case "slag-incinerator":
		return liquid == slagLiquidID && w.incineratorAcceptsLiquidLocked(toPos, liquid)
	case "repair-turret":
		return repairTurretAcceptsLiquid(liquid) && w.liquidHasCapacityLocked(toTile, liquid)
	case "force-projector":
		return (liquid == waterLiquidID || liquid == cryofluidLiquidID) && w.liquidHasCapacityLocked(toTile, liquid)
	case "unit-repair-tower":
		return liquid == ozoneLiquidID && w.liquidHasCapacityLocked(toTile, liquid)
	case "plasma-bore":
		return liquid == hydrogenLiquidID && w.liquidHasCapacityLocked(toTile, liquid)
	case "large-plasma-bore":
		return (liquid == hydrogenLiquidID || liquid == nitrogenLiquidID) && w.liquidHasCapacityLocked(toTile, liquid)
	case "impact-drill":
		return (liquid == waterLiquidID || liquid == ozoneLiquidID) && w.liquidHasCapacityLocked(toTile, liquid)
	case "eruption-drill":
		return (liquid == hydrogenLiquidID || liquid == cyanogenLiquidID) && w.liquidHasCapacityLocked(toTile, liquid)
	default:
		if w.turretAcceptsLiquidLocked(name, liquid) {
			if ammoByLiquid, ok := turretLiquidAmmoBulletTypesByName[name]; ok {
				if _, ok := ammoByLiquid[liquid]; ok {
					return w.liquidCanStoreLocked(toTile, liquid)
				}
			}
			return w.liquidHasCapacityLocked(toTile, liquid)
		}
		if isReconstructorBlockName(name) {
			return w.reconstructorAcceptsLiquidLocked(toTile, liquid)
		}
		cap := w.liquidCapacityForBlockLocked(toTile)
		return cap > 0 && w.liquidCanStoreLocked(toTile, liquid)
	}
}

func (w *World) turretAcceptsLiquidLocked(name string, liquid LiquidID) bool {
	if w == nil || name == "" {
		return false
	}
	if ammoByLiquid, ok := turretLiquidAmmoBulletTypesByName[name]; ok {
		if _, ok := ammoByLiquid[liquid]; ok {
			return true
		}
	}
	if _, ok := w.buildingWeaponProfileByNameLocked(name); !ok {
		return false
	}
	return liquid == waterLiquidID || liquid == cryofluidLiquidID
}

func firstBuildingCoolant(build *Building) (LiquidID, float32, bool) {
	if build == nil {
		return 0, 0, false
	}
	if liquid, amount, ok := currentBuildingLiquid(build); ok && amount > 0 && (liquid == waterLiquidID || liquid == cryofluidLiquidID) {
		return liquid, amount, true
	}
	if amount := build.LiquidAmount(waterLiquidID); amount > 0 {
		return waterLiquidID, amount, true
	}
	if amount := build.LiquidAmount(cryofluidLiquidID); amount > 0 {
		return cryofluidLiquidID, amount, true
	}
	return 0, 0, false
}

func (w *World) snapshotTickStartTurretCoolantsLocked() {
	w.tickStartTurretCoolants = nil
	w.tickLiquidReceipts = nil
	if w == nil || w.model == nil || (len(w.turretTilePositions) == 0 && len(w.forceProjectorPositions) == 0) {
		return
	}
	w.tickStartTurretCoolants = make(map[int32]map[LiquidID]float32, len(w.turretTilePositions)+len(w.forceProjectorPositions))
	w.tickLiquidReceipts = make(map[int32]map[LiquidID][]liquidTickReceipt)
	for _, pos := range w.turretTilePositions {
		w.snapshotTickStartCoolantForBuildingLocked(pos)
	}
	for _, pos := range w.forceProjectorPositions {
		w.snapshotTickStartCoolantForBuildingLocked(pos)
	}
}

func (w *World) snapshotTickStartCoolantForBuildingLocked(pos int32) {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil {
		return
	}
	var water, cryofluid float32
	for _, stack := range tile.Build.Liquids {
		if stack.Amount <= 0 {
			continue
		}
		switch stack.Liquid {
		case waterLiquidID:
			water += stack.Amount
		case cryofluidLiquidID:
			cryofluid += stack.Amount
		}
	}
	if water <= 0 && cryofluid <= 0 {
		w.tickStartTurretCoolants[pos] = nil
		return
	}
	coolants := make(map[LiquidID]float32, 2)
	if water > 0 {
		coolants[waterLiquidID] = water
	}
	if cryofluid > 0 {
		coolants[cryofluidLiquidID] = cryofluid
	}
	w.tickStartTurretCoolants[pos] = coolants
}

func (w *World) recordLiquidReceiptLocked(sourcePos, targetPos int32, liquid LiquidID, amount float32) {
	if w == nil || w.model == nil || w.tickLiquidReceipts == nil || amount <= 0 {
		return
	}
	if liquid != waterLiquidID && liquid != cryofluidLiquidID {
		return
	}
	centerPos, ok := w.centerBuildingIndexLocked(targetPos)
	if !ok {
		return
	}
	if _, ok := w.tickStartTurretCoolants[centerPos]; !ok {
		return
	}
	if sourceCenter, ok := w.centerBuildingIndexLocked(sourcePos); ok {
		sourcePos = sourceCenter
	}
	byLiquid := w.tickLiquidReceipts[centerPos]
	if byLiquid == nil {
		byLiquid = make(map[LiquidID][]liquidTickReceipt, 2)
		w.tickLiquidReceipts[centerPos] = byLiquid
	}
	byLiquid[liquid] = append(byLiquid[liquid], liquidTickReceipt{SourcePos: sourcePos, Amount: amount})
}

func (w *World) liquidReceiptVisibleToTurretLocked(sourcePos, consumerPos int32, turretName string, liquid LiquidID) bool {
	if sourceCenter, ok := w.centerBuildingIndexLocked(sourcePos); ok {
		sourcePos = sourceCenter
	}
	if consumerCenter, ok := w.centerBuildingIndexLocked(consumerPos); ok {
		consumerPos = consumerCenter
	}
	if turretName == "spectre" && liquid == cryofluidLiquidID {
		return vanillaBuildUpdateProxyRank(sourcePos) < vanillaBuildUpdateProxyRank(consumerPos)
	}
	return sourcePos <= consumerPos
}

func (w *World) tickAvailableTurretCoolantLocked(pos int32, liquid LiquidID, turretName string, includeReceipts, requireReceiptOrder bool) float32 {
	if w == nil || w.tickStartTurretCoolants == nil {
		return 0
	}
	allowed := float32(0)
	if byLiquid, ok := w.tickStartTurretCoolants[pos]; ok {
		allowed += byLiquid[liquid]
	}
	if !includeReceipts {
		return allowed
	}
	if byLiquid, ok := w.tickLiquidReceipts[pos]; ok {
		for _, receipt := range byLiquid[liquid] {
			if !requireReceiptOrder || w.liquidReceiptVisibleToTurretLocked(receipt.SourcePos, pos, turretName, liquid) {
				allowed += receipt.Amount
			}
		}
	}
	return allowed
}

func vanillaBuildUpdateProxyRank(pos int32) uint32 {
	return mixDumpProximityProxyKey(uint32(pos)^4) & 63
}

func liquidHeatCapacity(liquid LiquidID) float32 {
	switch liquid {
	case waterLiquidID:
		return 0.4
	case cryofluidLiquidID:
		return 0.9
	default:
		return 0.4
	}
}

func turretCoolantReloadMultiplier(name string) float32 {
	if mult, ok := turretCoolantMultiplierByName[name]; ok {
		return mult
	}
	return 5
}

func (w *World) consumeTurretCoolantLocked(pos int32, tile *Tile, name string, deltaFrames float32) (LiquidID, float32, float32, bool) {
	if tile == nil || tile.Build == nil || deltaFrames <= 0 {
		return 0, 0, 0, false
	}
	amountPerFrame, ok := turretCoolantAmountByName[name]
	if !ok || amountPerFrame <= 0 {
		return 0, 0, 0, false
	}
	liquid, available, ok := firstBuildingCoolant(tile.Build)
	if !ok || available <= 0 {
		return 0, 0, 0, false
	}
	if w != nil && w.tickStartTurretCoolants != nil {
		includeReceipts := true
		if prof, ok := w.buildingWeaponProfileByNameLocked(name); ok && (prof.PowerCapacity > 0 || prof.PowerRegen > 0 || prof.PowerPerShot > 0) {
			includeReceipts = false
		}
		if liquid == waterLiquidID {
			includeReceipts = false
		}
		requireReceiptOrder := !(name == "ripple" && liquid == cryofluidLiquidID)
		allowed := w.tickAvailableTurretCoolantLocked(pos, liquid, name, includeReceipts, requireReceiptOrder)
		available = minf(available, allowed)
		if available <= 0 {
			return 0, 0, 0, false
		}
	}
	used := minf(available, amountPerFrame*deltaFrames)
	if used <= 0 || !tile.Build.RemoveLiquid(liquid, used) {
		return 0, 0, 0, false
	}
	advanceSeconds := used * liquidHeatCapacity(liquid) * turretCoolantReloadMultiplier(name) / 60
	return liquid, used, advanceSeconds, true
}

func (w *World) liquidJunctionDestinationLocked(fromPos, junctionPos int32, liquid LiquidID, depth int) (int32, bool) {
	if depth > 8 || w.model == nil || junctionPos < 0 || int(junctionPos) >= len(w.model.Tiles) {
		return 0, false
	}
	sourceDir, ok := w.relativeToEdgeLocked(fromPos, junctionPos)
	if !ok {
		return 0, false
	}
	outDir := byte((int(sourceDir) + 2) % 4)
	nextPos, ok := w.forwardPosLocked(junctionPos, int8(outDir))
	if !ok {
		return 0, false
	}
	nextTile := &w.model.Tiles[nextPos]
	if nextTile.Build == nil || nextTile.Block == 0 {
		return 0, false
	}
	name := w.blockNameByID(int16(nextTile.Block))
	if name == "liquid-junction" || name == "reinforced-liquid-junction" {
		return w.liquidJunctionDestinationLocked(junctionPos, nextPos, liquid, depth+1)
	}
	if !w.canAcceptLiquidLocked(junctionPos, nextPos, liquid, depth+1) {
		return 0, false
	}
	return nextPos, true
}

func (w *World) tryMoveLiquidLocked(fromPos, toPos int32, liquid LiquidID, amount float32, depth int) float32 {
	return w.tryMoveLiquidFromLocked(fromPos, fromPos, toPos, liquid, amount, depth)
}

func (w *World) tryMoveLiquidFromLocked(originPos, fromPos, toPos int32, liquid LiquidID, amount float32, depth int) float32 {
	if amount <= 0 || !w.canAcceptLiquidLocked(fromPos, toPos, liquid, depth) || w.model == nil {
		return 0
	}
	toTile := &w.model.Tiles[toPos]
	name := w.blockNameByID(int16(toTile.Block))
	if name == "liquid-junction" || name == "reinforced-liquid-junction" {
		target, ok := w.liquidJunctionDestinationLocked(fromPos, toPos, liquid, depth+1)
		if !ok {
			return 0
		}
		return w.tryMoveLiquidFromLocked(originPos, toPos, target, liquid, amount, depth+1)
	}
	if name == "incinerator" {
		w.incineratorBurnLiquidLocked(toPos)
		return amount
	}
	if name == "liquid-void" {
		return amount
	}
	cap := w.liquidCapacityForBlockLocked(toTile)
	if cap <= 0 {
		return 0
	}
	space := cap - toTile.Build.LiquidAmount(liquid)
	if space <= 0 {
		return 0
	}
	if amount > space {
		amount = space
	}
	if amount <= 0 {
		return 0
	}
	toTile.Build.AddLiquid(liquid, amount)
	w.recordLiquidReceiptLocked(originPos, toPos, liquid, amount)
	return amount
}

func (w *World) moveLiquidProportionalLocked(fromPos, toPos int32, liquid LiquidID, pressure, scaling float32, depth int) float32 {
	return w.moveLiquidProportionalFromLocked(fromPos, fromPos, toPos, liquid, pressure, scaling, depth)
}

func (w *World) moveLiquidProportionalFromLocked(originPos, fromPos, toPos int32, liquid LiquidID, pressure, scaling float32, depth int) float32 {
	if pressure <= 0 {
		pressure = 1
	}
	if scaling <= 0 {
		scaling = 1
	}
	if !w.canAcceptLiquidLocked(fromPos, toPos, liquid, depth) || w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return 0
	}
	toTile := &w.model.Tiles[toPos]
	name := w.blockNameByID(int16(toTile.Block))
	if name == "liquid-junction" || name == "reinforced-liquid-junction" {
		target, ok := w.liquidJunctionDestinationLocked(fromPos, toPos, liquid, depth+1)
		if !ok {
			return 0
		}
		return w.moveLiquidProportionalFromLocked(originPos, toPos, target, liquid, pressure, scaling, depth+1)
	}
	if name == "incinerator" || name == "liquid-void" {
		return w.tryMoveLiquidFromLocked(originPos, fromPos, toPos, liquid, 0.1, depth)
	}
	fromTile := &w.model.Tiles[fromPos]
	if fromTile.Build == nil || toTile.Build == nil {
		return 0
	}
	fromCap := w.liquidCapacityForBlockLocked(fromTile)
	toCap := w.liquidCapacityForBlockLocked(toTile)
	if fromCap <= 0 || toCap <= 0 {
		return 0
	}
	sourceAmount := fromTile.Build.LiquidAmount(liquid)
	if sourceAmount <= 0.0001 {
		return 0
	}
	sourceFrac := sourceAmount / fromCap * pressure
	targetFrac := toTile.Build.LiquidAmount(liquid) / toCap
	flow := clampf(sourceFrac-targetFrac, 0, 1) * fromCap / scaling
	if flow > sourceAmount {
		flow = sourceAmount
	}
	space := toCap - toTile.Build.LiquidAmount(liquid)
	if flow > space {
		flow = space
	}
	if flow <= 0 {
		return 0
	}
	toTile.Build.AddLiquid(liquid, flow)
	w.recordLiquidReceiptLocked(originPos, toPos, liquid, flow)
	return flow
}

func (w *World) liquidDumpNeighborsLocked(pos int32) []int32 {
	if w == nil {
		return nil
	}
	if liquidDumpProximityProxySeed == dumpProximityProxySeed {
		return w.dumpProximityLocked(pos)
	}
	if cached, ok := w.liquidDumpNeighborCache[pos]; ok {
		return cached
	}
	base := w.dumpProximityLocked(pos)
	if len(base) <= 1 {
		w.liquidDumpNeighborCache[pos] = base
		return base
	}
	out := append(make([]int32, 0, len(base)), base...)
	sort.SliceStable(out, func(i, j int) bool {
		ki := dumpProximityProxyKeyForSeed(out[i], liquidDumpProximityProxySeed)
		kj := dumpProximityProxyKeyForSeed(out[j], liquidDumpProximityProxySeed)
		return ki < kj
	})
	w.liquidDumpNeighborCache[pos] = out
	return out
}

func (w *World) dumpLiquidProportionalLocked(pos int32, tile *Tile, liquid LiquidID, scaling float32) bool {
	if tile == nil || tile.Build == nil || w.model == nil || tile.Build.LiquidAmount(liquid) <= 0.0001 {
		return false
	}
	if scaling <= 0 {
		scaling = 2
	}
	neighbors := w.liquidDumpNeighborsLocked(pos)
	if len(neighbors) == 0 {
		return false
	}
	start := 0
	if idx, ok := w.blockDumpIndex[pos]; ok && len(neighbors) > 0 {
		start = ((idx % len(neighbors)) + len(neighbors)) % len(neighbors)
	}
	movedAny := false
	for i := 0; i < len(neighbors); i++ {
		index := (start + i) % len(neighbors)
		target := neighbors[index]
		moved := w.moveLiquidProportionalLocked(pos, target, liquid, 1, scaling, 0)
		w.advanceDumpIndexLocked(pos, index+1, len(neighbors))
		if moved > 0 {
			_ = tile.Build.RemoveLiquid(liquid, moved)
			movedAny = true
		}
	}
	return movedAny
}

func (w *World) dumpLiquidLocked(pos int32, tile *Tile, liquid LiquidID, amount float32) bool {
	if tile == nil || tile.Build == nil || amount <= 0 || w.model == nil {
		return false
	}
	neighbors := w.dumpProximityLocked(pos)
	if len(neighbors) == 0 {
		return false
	}
	start := 0
	if idx, ok := w.blockDumpIndex[pos]; ok && len(neighbors) > 0 {
		start = ((idx % len(neighbors)) + len(neighbors)) % len(neighbors)
	}
	for i := 0; i < len(neighbors); i++ {
		index := (start + i) % len(neighbors)
		target := neighbors[index]
		moved := w.tryMoveLiquidLocked(pos, target, liquid, amount, 0)
		w.advanceDumpIndexLocked(pos, index+1, len(neighbors))
		if moved > 0 {
			_ = tile.Build.RemoveLiquid(liquid, moved)
			return true
		}
	}
	return false
}

func (w *World) bridgeAllowsInputLocked(fromPos, bridgePos int32) bool {
	if w.model == nil || fromPos < 0 || bridgePos < 0 || int(fromPos) >= len(w.model.Tiles) || int(bridgePos) >= len(w.model.Tiles) {
		return false
	}
	bridgeTile := &w.model.Tiles[bridgePos]
	bridgeName := w.blockNameByID(int16(bridgeTile.Block))
	if !isItemBridgeBlock(bridgeName) {
		return false
	}
	if w.bridgeLinks[fromPos] == bridgePos && w.blockNameByID(int16(w.model.Tiles[fromPos].Block)) == bridgeName {
		return true
	}
	target, ok := w.bridgeLinks[bridgePos]
	if !ok || target < 0 || int(target) >= len(w.model.Tiles) {
		return false
	}
	targetTile := &w.model.Tiles[target]
	if targetTile.Build == nil || targetTile.Team != bridgeTile.Team || w.blockNameByID(int16(targetTile.Block)) != bridgeName {
		return false
	}
	linkSide, ok := axisDir(targetTile.X, targetTile.Y, bridgeTile.X, bridgeTile.Y)
	if !ok {
		return false
	}
	fromTile := &w.model.Tiles[fromPos]
	sourceSide, ok := relativeDir(fromTile.X, fromTile.Y, bridgeTile.X, bridgeTile.Y)
	if !ok {
		return false
	}
	return sourceSide != linkSide
}

func (w *World) bridgeHasIncomingFromSideLocked(bridgePos int32, side byte) bool {
	if w.model == nil || bridgePos < 0 || int(bridgePos) >= len(w.model.Tiles) {
		return false
	}
	if len(w.bridgeIncomingMask) == 0 && len(w.bridgeLinks) > 0 {
		mask := make(map[int32]byte, len(w.bridgeLinks))
		for otherPos, target := range w.bridgeLinks {
			if target < 0 || otherPos < 0 || int(target) >= len(w.model.Tiles) || int(otherPos) >= len(w.model.Tiles) {
				continue
			}
			bridgeTile := &w.model.Tiles[target]
			otherTile := &w.model.Tiles[otherPos]
			if bridgeTile.Build == nil || otherTile.Build == nil || otherTile.Team != bridgeTile.Team {
				continue
			}
			bridgeName := w.blockNameByID(int16(bridgeTile.Block))
			if w.blockNameByID(int16(otherTile.Block)) != bridgeName {
				continue
			}
			incomingSide, ok := axisDir(otherTile.X, otherTile.Y, bridgeTile.X, bridgeTile.Y)
			if !ok || incomingSide >= 8 {
				continue
			}
			mask[target] |= 1 << incomingSide
		}
		w.bridgeIncomingMask = mask
	}
	return (w.bridgeIncomingMask[bridgePos] & (1 << side)) != 0
}

func (w *World) massDriverStateLocked(pos int32) *massDriverRuntimeState {
	if st, ok := w.massDriverStates[pos]; ok && st != nil {
		return st
	}
	st := &massDriverRuntimeState{}
	w.massDriverStates[pos] = st
	return st
}

func (w *World) massDriverTargetLocked(pos int32, tile *Tile) (int32, bool) {
	target, ok := w.massDriverLinks[pos]
	if !ok || target < 0 || int(target) >= len(w.model.Tiles) {
		return 0, false
	}
	targetTile := &w.model.Tiles[target]
	if tile == nil || targetTile.Build == nil || targetTile.Team != tile.Team || w.blockNameByID(int16(targetTile.Block)) != "mass-driver" {
		return 0, false
	}
	dx := float32(targetTile.X - tile.X)
	dy := float32(targetTile.Y - tile.Y)
	if dx*dx+dy*dy > 55*55 {
		return 0, false
	}
	return target, true
}

func (w *World) massDriverIncomingShotsLocked(targetPos int32) int {
	count := 0
	for _, shot := range w.massDriverShots {
		if shot.ToPos == targetPos {
			count++
		}
	}
	return count
}

func (w *World) massDriverTakePayloadLocked(pos int32, tile *Tile, limit int32) []ItemStack {
	if tile == nil || tile.Build == nil || limit <= 0 {
		return nil
	}
	total := int32(0)
	out := make([]ItemStack, 0, len(tile.Build.Items))
	for _, stack := range append([]ItemStack(nil), tile.Build.Items...) {
		if stack.Amount <= 0 || total >= limit {
			continue
		}
		amount := stack.Amount
		if amount > limit-total {
			amount = limit - total
		}
		if amount <= 0 {
			continue
		}
		if w.removeItemAtLocked(pos, stack.Item, amount) {
			out = append(out, ItemStack{Item: stack.Item, Amount: amount})
			total += amount
		}
	}
	return out
}

func isStorageLikeBlock(name string) bool {
	switch name {
	case "core-shard", "core-foundation", "core-nucleus", "core-bastion", "core-citadel", "core-acropolis", "container", "vault", "reinforced-container", "reinforced-vault":
		return true
	default:
		return false
	}
}

func isCoreBlockName(name string) bool {
	return strings.HasPrefix(name, "core-")
}

func isCoreMergeStorageBlock(name string) bool {
	switch name {
	case "container", "vault", "reinforced-container", "reinforced-vault":
		return true
	default:
		return false
	}
}

func affectsCoreStorageLinks(name string) bool {
	return isCoreBlockName(name) || isCoreMergeStorageBlock(name)
}
