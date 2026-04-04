package world

import (
	"sort"
	"strings"

	"mdt-server/internal/protocol"
)

// BlockSyncSnapshot mirrors the payload written by vanilla NetServer.writeBlockSnapshots():
// packed tile position, block ID, then build.writeSync(...) bytes.
type BlockSyncSnapshot struct {
	Pos     int32
	BlockID int16
	Data    []byte
}

type blockSyncKind byte

const (
	blockSyncNone blockSyncKind = iota
	blockSyncBaseOnly
	blockSyncStorage
	blockSyncUnloader
	blockSyncUnitFactory
	blockSyncGenericCrafter
	blockSyncHeatProducer
	blockSyncSeparator
	blockSyncPowerGenerator
	blockSyncNuclearReactor
	blockSyncImpactReactor
	blockSyncHeaterGenerator
	blockSyncVariableReactor
	blockSyncDrill
	blockSyncPump
	blockSyncIncinerator
)

func (w *World) BlockSyncSnapshots() []BlockSyncSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}

	out := make([]BlockSyncSnapshot, 0, 64)
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile == nil || tile.Block == 0 || tile.Build == nil || tile.Build.Team == 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
		kind := classifyBlockSyncKind(name)
		if kind == blockSyncNone {
			continue
		}
		data, ok := w.serializeBlockSyncLocked(pos, tile, name, kind)
		if !ok || len(data) == 0 {
			continue
		}
		out = append(out, BlockSyncSnapshot{
			Pos:     packTilePos(tile.X, tile.Y),
			BlockID: int16(tile.Block),
			Data:    data,
		})
	}
	return out
}

func classifyBlockSyncKind(name string) blockSyncKind {
	name = strings.ToLower(strings.TrimSpace(name))
	switch name {
	case "battery", "battery-large",
		"power-node", "power-node-large", "surge-tower", "beam-link", "power-source",
		"beam-node", "beam-tower", "power-void", "power-diode":
		return blockSyncBaseOnly
	case "container", "vault", "reinforced-container", "reinforced-vault":
		return blockSyncStorage
	case "unloader":
		return blockSyncUnloader
	case "ground-factory", "air-factory", "naval-factory":
		return blockSyncUnitFactory
	case "separator", "disassembler", "slag-centrifuge":
		return blockSyncSeparator
	case "thorium-reactor":
		return blockSyncNuclearReactor
	case "impact-reactor":
		return blockSyncImpactReactor
	case "neoplasia-reactor":
		return blockSyncHeaterGenerator
	case "flux-reactor":
		return blockSyncVariableReactor
	case "mechanical-drill", "pneumatic-drill", "laser-drill", "blast-drill":
		return blockSyncDrill
	case "mechanical-pump", "rotary-pump", "impulse-pump", "water-extractor", "oil-extractor":
		return blockSyncPump
	case "incinerator":
		return blockSyncIncinerator
	case "combustion-generator", "thermal-generator", "steam-generator", "differential-generator",
		"rtg-generator", "solar-panel", "solar-panel-large", "turbine-condenser",
		"chemical-combustion-chamber", "pyrolysis-generator":
		return blockSyncPowerGenerator
	}
	if prof, ok := crafterProfilesByBlockName[name]; ok {
		if prof.HeatOutput > 0 {
			return blockSyncHeatProducer
		}
		return blockSyncGenericCrafter
	}
	return blockSyncNone
}

func (w *World) serializeBlockSyncLocked(pos int32, tile *Tile, name string, kind blockSyncKind) ([]byte, bool) {
	if w == nil || tile == nil || tile.Build == nil {
		return nil, false
	}
	writer := protocol.NewWriter()
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, kind); err != nil {
		return nil, false
	}

	switch kind {
	case blockSyncUnloader:
		sortItem := int16(-1)
		if item, ok := w.unloaderCfg[pos]; ok {
			sortItem = int16(item)
		}
		if err := writer.WriteInt16(sortItem); err != nil {
			return nil, false
		}
	case blockSyncUnitFactory:
		payX, payY, payRotation := w.unitFactoryPayloadSyncFieldsLocked(pos, tile)
		if err := writer.WriteFloat32(payX); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(payY); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(payRotation); err != nil {
			return nil, false
		}
		if err := writeBlockPayloadBytes(writer, tile.Build); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(w.unitFactoryProgressLocked(pos, tile)); err != nil {
			return nil, false
		}
		currentPlan, _ := w.unitFactoryConfigValueLocked(pos, tile)
		if err := writer.WriteInt16(int16(currentPlan)); err != nil {
			return nil, false
		}
		if err := protocol.WriteVecNullable(writer, nil); err != nil {
			return nil, false
		}
		if err := protocol.WriteCommand(writer, nil); err != nil {
			return nil, false
		}
	case blockSyncGenericCrafter:
		state := w.crafterStates[pos]
		if err := writer.WriteFloat32(maxf(state.Progress, 0)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Warmup, 0, 1)); err != nil {
			return nil, false
		}
		// Cultivator keeps a legacy warmup slot in vanilla GenericCrafter.read().
		if name == "cultivator" {
			if err := writer.WriteFloat32(0); err != nil {
				return nil, false
			}
		}
	case blockSyncHeatProducer:
		state := w.crafterStates[pos]
		if err := writer.WriteFloat32(maxf(state.Progress, 0)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Warmup, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(maxf(w.heatStates[pos], 0)); err != nil {
			return nil, false
		}
	case blockSyncSeparator:
		state := w.crafterStates[pos]
		if err := writer.WriteFloat32(maxf(state.Progress, 0)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Warmup, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteInt32(int32(state.Seed)); err != nil {
			return nil, false
		}
	case blockSyncPowerGenerator, blockSyncNuclearReactor, blockSyncImpactReactor, blockSyncHeaterGenerator:
		productionEfficiency, generateTime, extra := w.powerGeneratorSyncFieldsLocked(pos, tile, name, kind)
		if err := writer.WriteFloat32(clampf(productionEfficiency, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(maxf(generateTime, 0)); err != nil {
			return nil, false
		}
		switch kind {
		case blockSyncNuclearReactor, blockSyncHeaterGenerator:
			if err := writer.WriteFloat32(maxf(extra, 0)); err != nil {
				return nil, false
			}
		case blockSyncImpactReactor:
			if err := writer.WriteFloat32(clampf(extra, 0, 1)); err != nil {
				return nil, false
			}
		}
	case blockSyncVariableReactor:
		productionEfficiency, heat, instability, warmup := w.variableReactorSyncFieldsLocked(pos, tile, name)
		if err := writer.WriteFloat32(clampf(productionEfficiency, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(0); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(maxf(heat, 0)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(instability, 0, 1)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(warmup, 0, 1)); err != nil {
			return nil, false
		}
	case blockSyncDrill:
		state := w.drillStates[pos]
		if err := writer.WriteFloat32(maxf(state.Progress, 0)); err != nil {
			return nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Warmup, 0, 1)); err != nil {
			return nil, false
		}
	}

	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) writeBlockBaseSyncLocked(writer *protocol.Writer, pos int32, tile *Tile, name string, kind blockSyncKind) error {
	build := tile.Build
	hasItems := w.hasItemModuleForBlockSyncLocked(tile, name, kind)
	hasPower := w.hasPowerModuleForBlockSyncLocked(tile, name, kind)
	hasLiquids := w.hasLiquidModuleForBlockSyncLocked(tile, name, kind)

	health := maxf(build.Health, 0)
	if err := writer.WriteFloat32(health); err != nil {
		return err
	}
	if err := writer.WriteByte(byte(int(tile.Rotation)&0x7f | 0x80)); err != nil {
		return err
	}
	if err := writer.WriteByte(byte(build.Team)); err != nil {
		return err
	}
	if err := writer.WriteByte(3); err != nil {
		return err
	}
	if err := writer.WriteByte(1); err != nil {
		return err
	}

	moduleBits := byte(1 << 3)
	if hasItems {
		moduleBits |= 1
	}
	if hasPower {
		moduleBits |= 1 << 1
	}
	if hasLiquids {
		moduleBits |= 1 << 2
	}
	if err := writer.WriteByte(moduleBits); err != nil {
		return err
	}
	if hasItems {
		if err := w.writeBlockItemModuleLocked(writer, pos, build); err != nil {
			return err
		}
	}
	if hasPower {
		if err := w.writeBlockPowerModuleLocked(writer, pos, tile, name); err != nil {
			return err
		}
	}
	if hasLiquids {
		if err := writeBlockLiquidModule(writer, build); err != nil {
			return err
		}
	}

	efficiency, optionalEfficiency := w.blockSyncEfficiencyLocked(pos, tile, name, kind)
	if err := writer.WriteByte(byte(clampf(efficiency, 0, 1) * 255)); err != nil {
		return err
	}
	return writer.WriteByte(byte(clampf(optionalEfficiency, 0, 1) * 255))
}

func (w *World) hasItemModuleForBlockSyncLocked(tile *Tile, name string, kind blockSyncKind) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	switch kind {
	case blockSyncStorage, blockSyncUnloader, blockSyncGenericCrafter, blockSyncNuclearReactor, blockSyncHeaterGenerator, blockSyncImpactReactor:
		return true
	case blockSyncPowerGenerator:
		return w.itemCapacityForBlockLocked(tile) > 0
	default:
		return w.itemCapacityForBlockLocked(tile) > 0
	}
}

func (w *World) hasLiquidModuleForBlockSyncLocked(tile *Tile, name string, kind blockSyncKind) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	return w.liquidCapacityForBlockLocked(tile) > 0
}

func (w *World) hasPowerModuleForBlockSyncLocked(tile *Tile, name string, kind blockSyncKind) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	return w.isPowerRelevantBuildingLocked(tile)
}

func (w *World) writeBlockItemModuleLocked(writer *protocol.Writer, pos int32, build *Building) error {
	if writer == nil {
		return nil
	}
	src := build
	if _, _, shared, ok := w.sharedCoreInventoryLocked(pos); ok && shared != nil {
		src = shared
	}
	if src == nil {
		return writer.WriteInt16(0)
	}
	items := make([]ItemStack, 0, len(src.Items))
	for _, stack := range src.Items {
		if stack.Amount > 0 {
			items = append(items, stack)
		}
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i].Item < items[j].Item
	})
	if err := writer.WriteInt16(int16(len(items))); err != nil {
		return err
	}
	for _, stack := range items {
		if err := writer.WriteInt16(int16(stack.Item)); err != nil {
			return err
		}
		if err := writer.WriteInt32(stack.Amount); err != nil {
			return err
		}
	}
	return nil
}

func writeBlockLiquidModule(writer *protocol.Writer, build *Building) error {
	liquids := make([]LiquidStack, 0, len(build.Liquids))
	for _, stack := range build.Liquids {
		if stack.Amount > 0.0001 {
			liquids = append(liquids, stack)
		}
	}
	sort.Slice(liquids, func(i, j int) bool {
		return liquids[i].Liquid < liquids[j].Liquid
	})
	if err := writer.WriteInt16(int16(len(liquids))); err != nil {
		return err
	}
	for _, stack := range liquids {
		if err := writer.WriteInt16(int16(stack.Liquid)); err != nil {
			return err
		}
		if err := writer.WriteFloat32(stack.Amount); err != nil {
			return err
		}
	}
	return nil
}

func (w *World) writeBlockPowerModuleLocked(writer *protocol.Writer, pos int32, tile *Tile, name string) error {
	links := w.blockSyncPowerLinksLocked(pos, tile, name)
	if err := writer.WriteInt16(int16(len(links))); err != nil {
		return err
	}
	for _, link := range links {
		if err := writer.WriteInt32(link); err != nil {
			return err
		}
	}
	return writer.WriteFloat32(clampf(w.blockSyncPowerStatusLocked(pos, tile, name), 0, 1))
}

func writeBlockPayloadBytes(writer *protocol.Writer, build *Building) error {
	if build == nil || len(build.Payload) == 0 {
		return writer.WriteBool(false)
	}
	return writer.WriteBytes(build.Payload)
}

func (w *World) unitFactoryPayloadSyncFieldsLocked(pos int32, tile *Tile) (float32, float32, float32) {
	rotation := float32(0)
	if tile != nil {
		rotation = float32(tile.Rotation) * 90
	}
	if tile == nil {
		return 0, 0, rotation
	}
	st := w.payloadStates[pos]
	if st == nil || st.Payload == nil {
		return 0, 0, rotation
	}
	moveTime := w.unitBlockPayloadMoveFramesLocked(tile)
	if moveTime <= 0 {
		return 0, 0, rotation
	}
	progress := clampf(st.Move/moveTime, 0, 1)
	dx, dy := dirDelta(tile.Rotation)
	dist := float32(w.blockSizeForTileLocked(tile)) * 8 / 2
	return float32(dx) * dist * progress, float32(dy) * dist * progress, rotation
}

func (w *World) blockSyncPowerLinksLocked(pos int32, tile *Tile, name string) []int32 {
	if w == nil || tile == nil || tile.Build == nil {
		return nil
	}
	links := make([]int32, 0, 6)
	if isPowerNodeBlockName(name) {
		for _, link := range w.powerNodeLinks[pos] {
			if w.blockSyncPowerLinkPresentLocked(pos, link) {
				links = appendUniquePowerPos(links, link)
			}
		}
	}
	if isBeamNodeBlockName(name) {
		for _, link := range w.beamNodeTargetsLocked(pos, tile) {
			if w.blockSyncPowerLinkPresentLocked(pos, link) {
				links = appendUniquePowerPos(links, link)
			}
		}
	}
	for otherPos, otherLinks := range w.powerNodeLinks {
		if otherPos == pos {
			continue
		}
		for _, link := range otherLinks {
			if link == pos && w.blockSyncPowerLinkPresentLocked(otherPos, pos) {
				links = appendUniquePowerPos(links, otherPos)
				break
			}
		}
	}
	if w.model != nil {
		for _, otherPos := range w.activeTilePositions {
			if otherPos == pos || otherPos < 0 || int(otherPos) >= len(w.model.Tiles) {
				continue
			}
			other := &w.model.Tiles[otherPos]
			if other.Build == nil || other.Block == 0 || !isBeamNodeBlockName(w.blockNameByID(int16(other.Block))) {
				continue
			}
			for _, link := range w.beamNodeTargetsLocked(otherPos, other) {
				if link == pos && w.blockSyncPowerLinkPresentLocked(otherPos, pos) {
					links = appendUniquePowerPos(links, otherPos)
					break
				}
			}
		}
	}
	if len(links) == 0 {
		return nil
	}
	sort.Slice(links, func(i, j int) bool { return links[i] < links[j] })
	return append([]int32(nil), links...)
}

func (w *World) blockSyncPowerLinkPresentLocked(pos, link int32) bool {
	if w == nil || w.model == nil || pos < 0 || link < 0 || int(pos) >= len(w.model.Tiles) || int(link) >= len(w.model.Tiles) {
		return false
	}
	from := &w.model.Tiles[pos]
	to := &w.model.Tiles[link]
	if from.Build == nil || to.Build == nil || from.Block == 0 || to.Block == 0 {
		return false
	}
	if from.Build.Team == 0 || from.Build.Team != to.Build.Team {
		return false
	}
	if !w.isPowerRelevantBuildingLocked(from) || !w.isPowerRelevantBuildingLocked(to) {
		return false
	}
	if isPowerDiodeBlockName(w.blockNameByID(int16(from.Block))) || isPowerDiodeBlockName(w.blockNameByID(int16(to.Block))) {
		return false
	}
	return true
}

func (w *World) blockSyncPowerStatusLocked(pos int32, tile *Tile, name string) float32 {
	if w == nil || tile == nil || tile.Build == nil {
		return 0
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if capacity := powerStorageCapacityByBlockName(name); capacity > 0 {
		if rules := w.rulesMgr.Get(); rules != nil && rules.teamInfiniteResources(tile.Build.Team) {
			return 1
		}
		return clampf(w.powerStorageState[pos]/capacity, 0, 1)
	}
	if !w.blockConsumesPowerLocked(name) {
		return 0
	}
	if rules := w.rulesMgr.Get(); rules != nil && rules.teamInfiniteResources(tile.Build.Team) {
		return 1
	}
	if requested := w.powerRequested[pos]; requested > 0.000001 {
		return clampf(w.powerSupplied[pos]/requested, 0, 1)
	}
	if net, ok := w.powerNetStateForPosLocked(pos); ok {
		// Vanilla only stores status for consumers/buffers; inactive consumers estimate
		// based on whether their graph can currently supply power at all.
		if net.Budget > 0.0001 || net.Produced > 0.0001 {
			return 1
		}
	}
	return 0
}

func (w *World) blockSyncEfficiencyLocked(pos int32, tile *Tile, name string, kind blockSyncKind) (float32, float32) {
	switch kind {
	case blockSyncStorage, blockSyncUnloader:
		return 1, 1
	case blockSyncUnitFactory:
		efficiency := w.unitFactorySyncEfficiencyLocked(pos, tile)
		return efficiency, efficiency
	case blockSyncGenericCrafter:
		state := w.crafterStates[pos]
		return clampf(state.Warmup, 0, 1), clampf(state.Warmup, 0, 1)
	case blockSyncHeatProducer:
		state := w.crafterStates[pos]
		return clampf(state.Warmup, 0, 1), clampf(state.Warmup, 0, 1)
	case blockSyncDrill:
		state := w.drillStates[pos]
		return clampf(state.Warmup, 0, 1), clampf(state.Warmup, 0, 1)
	case blockSyncPump:
		state := w.pumpStates[pos]
		return clampf(state.Warmup, 0, 1), clampf(state.Warmup, 0, 1)
	case blockSyncPowerGenerator, blockSyncNuclearReactor, blockSyncImpactReactor, blockSyncHeaterGenerator:
		productionEfficiency, _, _ := w.powerGeneratorSyncFieldsLocked(pos, tile, name, kind)
		return clampf(productionEfficiency, 0, 1), clampf(productionEfficiency, 0, 1)
	case blockSyncVariableReactor:
		productionEfficiency, _, _, _ := w.variableReactorSyncFieldsLocked(pos, tile, name)
		return clampf(productionEfficiency, 0, 1), clampf(productionEfficiency, 0, 1)
	case blockSyncIncinerator:
		heat := clampf(w.incineratorStates[pos], 0, 1)
		return heat, heat
	default:
		return 1, 1
	}
}

func (w *World) unitFactorySyncEfficiencyLocked(pos int32, tile *Tile) float32 {
	if w == nil || tile == nil || tile.Build == nil {
		return 0
	}
	if plan, ok := w.unitFactorySelectedPlanLocked(pos, tile); !ok {
		return 0
	} else {
		if st := w.payloadStates[pos]; st != nil && st.Payload != nil {
			return 0
		}
		if w.unitBuildSpeedMultiplierLocked(tile.Build.Team, w.rulesMgr.Get()) <= 0 {
			return 0
		}
		if scaledCost := w.unitFactoryScaledCostLocked(tile.Build.Team, plan.Cost); len(scaledCost) > 0 && !hasRequiredItemsLocked(tile.Build, scaledCost) {
			return 0
		}
		if rules := w.rulesMgr.Get(); rules != nil && rules.teamInfiniteResources(tile.Build.Team) {
			return 1
		}
		if !w.isPowerRelevantBuildingLocked(tile) {
			return 1
		}
		return clampf(w.blockSyncPowerStatusLocked(pos, tile, w.blockNameByID(int16(tile.Block))), 0, 1)
	}
}

func (w *World) powerGeneratorSyncFieldsLocked(pos int32, tile *Tile, name string, kind blockSyncKind) (float32, float32, float32) {
	st := w.powerGeneratorState[pos]
	switch kind {
	case blockSyncImpactReactor:
		warmup := float32(0)
		generateTime := float32(0)
		if st != nil {
			warmup = clampf(st.Warmup, 0, 1)
			generateTime = maxf(st.FuelFrames, 0)
		}
		return clampf(pow5f(warmup), 0, 1), generateTime, warmup
	case blockSyncNuclearReactor:
		generateTime := float32(0)
		if st != nil {
			generateTime = maxf(st.FuelFrames, 0)
		}
		productionEfficiency := float32(0)
		if tile != nil && tile.Build != nil {
			if tile.Build.ItemAmount(thoriumItemID) > 0 || tile.Build.ItemAmount(legacyThoriumItemID) > 0 {
				productionEfficiency = 1
			}
		}
		if generateTime > 0 {
			productionEfficiency = 1
		}
		return clampf(productionEfficiency, 0, 1), generateTime, maxf(w.heatStates[pos], 0)
	case blockSyncHeaterGenerator:
		heat := maxf(w.heatStates[pos], 0)
		heatOutput := float32(10)
		if name == "neoplasia-reactor" {
			heatOutput = 60
		}
		return clampf(heat/maxf(heatOutput, 0.0001), 0, 1), 0, heat
	case blockSyncPowerGenerator:
		generateTime := float32(0)
		if st != nil {
			generateTime = maxf(st.FuelFrames, 0)
		}
		productionEfficiency := float32(0)
		switch name {
		case "solar-panel", "solar-panel-large":
			productionEfficiency = 1
		case "thermal-generator":
			productionEfficiency = clampf(w.thermalGenerationEfficiencyLocked(tile), 0, 1)
		case "turbine-condenser":
			productionEfficiency = clampf(w.sumFloorAttributeLocked(tile, "steam")/9, 0, 1)
		case "chemical-combustion-chamber":
			if canConsumeLiquidStacksLocked(tile.Build, []LiquidStack{
				{Liquid: ozoneLiquidID, Amount: 2.0 / 60.0},
				{Liquid: arkyciteLiquidID, Amount: 40.0 / 60.0},
			}, 1) {
				productionEfficiency = 1
			}
		case "pyrolysis-generator":
			if canConsumeLiquidStacksLocked(tile.Build, []LiquidStack{
				{Liquid: slagLiquidID, Amount: 20.0 / 60.0},
				{Liquid: arkyciteLiquidID, Amount: 40.0 / 60.0},
			}, 1) {
				productionEfficiency = 1
			}
		}
		if generateTime > 0 {
			productionEfficiency = 1
		}
		return productionEfficiency, generateTime, 0
	default:
		return 0, 0, 0
	}
}

func (w *World) variableReactorSyncFieldsLocked(pos int32, tile *Tile, name string) (productionEfficiency, heat, instability, warmup float32) {
	st := w.powerGeneratorState[pos]
	heat = maxf(w.heatStates[pos], 0)
	if st != nil {
		instability = st.Instability
		warmup = st.Warmup
	}
	maxHeat := float32(0)
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "flux-reactor":
		maxHeat = 150
	}
	if maxHeat > 0 {
		productionEfficiency = clampf(heat/maxHeat, 0, 1)
	}
	return clampf(productionEfficiency, 0, 1), heat, clampf(instability, 0, 1), clampf(warmup, 0, 1)
}
