package world

import (
	"sort"
	"strings"

	"mdt-server/internal/protocol"
)

// CloneModelForWorldStream returns a model clone with best-effort live building
// payload bytes attached for map-stream/world-stream encoding.
func (w *World) CloneModelForWorldStream() *WorldModel {
	if w == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil {
		return nil
	}
	clone := w.model.Clone()
	if clone == nil {
		return nil
	}
	for i := range w.model.Tiles {
		srcTile := &w.model.Tiles[i]
		if srcTile == nil || srcTile.Block == 0 || srcTile.Build == nil {
			continue
		}
		dstTile := &clone.Tiles[i]
		if dstTile.Build == nil {
			continue
		}
		if revision, payload, ok := w.encodeMapStreamBuildingPayloadLocked(int32(i), srcTile); ok {
			dstTile.Build.MapSyncRevision = revision
			dstTile.Build.MapSyncData = append([]byte(nil), payload...)
		}
	}
	for packed, centerPos := range w.blockOccupancy {
		if centerPos < 0 || clone == nil || int(centerPos) >= len(clone.Tiles) {
			continue
		}
		x := int(protocol.UnpackPoint2X(packed))
		y := int(protocol.UnpackPoint2Y(packed))
		if !clone.InBounds(x, y) {
			continue
		}
		centerTile := &clone.Tiles[centerPos]
		if centerTile == nil || centerTile.Block == 0 || centerTile.Build == nil {
			continue
		}
		edgeTile := &clone.Tiles[y*clone.Width+x]
		edgeTile.Block = centerTile.Block
		edgeTile.Team = centerTile.Team
		edgeTile.Rotation = centerTile.Rotation
		if x != centerTile.X || y != centerTile.Y {
			edgeTile.Build = centerTile.Build
		}
	}
	return clone
}

func (w *World) HasLiveMapStreamPayloadPacked(packedPos int32) bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	index, ok := w.tileIndexFromPackedPosLocked(packedPos)
	if !ok || w.model == nil || index < 0 || int(index) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[index]
	return w.hasLiveMapStreamPayloadLocked(index, tile)
}

func (w *World) hasLiveMapStreamPayloadLocked(pos int32, tile *Tile) bool {
	if w == nil || tile == nil || tile.Build == nil || tile.Block == 0 {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	if _, _, ok := w.encodeSpecialMapStreamBuildingPayloadLocked(pos, tile, name); ok {
		return true
	}
	kind := w.classifyBlockSyncKindLocked(pos, tile, name)
	switch kind {
	case blockSyncBaseOnly,
		blockSyncStorage,
		blockSyncPowerGenerator,
		blockSyncNuclearReactor,
		blockSyncImpactReactor,
		blockSyncHeaterGenerator,
		blockSyncVariableReactor,
		blockSyncDrill:
		return false
	}
	_, supported := mapStreamRevisionForBlock(name, kind)
	return supported
}

func (w *World) encodeMapStreamBuildingPayloadLocked(pos int32, tile *Tile) (byte, []byte, bool) {
	if w == nil || tile == nil || tile.Build == nil || tile.Block == 0 {
		return 0, nil, false
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	if revision, payload, ok := w.encodeSpecialMapStreamBuildingPayloadLocked(pos, tile, name); ok {
		return revision, payload, true
	}
	kind := w.classifyBlockSyncKindLocked(pos, tile, name)
	revision, supported := mapStreamRevisionForBlock(name, kind)
	if !supported {
		if len(tile.Build.MapSyncData) == 0 {
			return 0, nil, false
		}
		return tile.Build.MapSyncRevision, append([]byte(nil), tile.Build.MapSyncData...), true
	}

	writer := protocol.NewWriter()
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, kind); err != nil {
		return 0, nil, false
	}

	switch kind {
	case blockSyncConveyor:
		if err := w.writeBlockConveyorSyncLocked(writer, pos, tile); err != nil {
			return 0, nil, false
		}
	case blockSyncStackConveyor:
		if err := w.writeBlockStackConveyorSyncLocked(writer, pos, tile); err != nil {
			return 0, nil, false
		}
	case blockSyncMassDriver:
		if err := w.writeBlockMassDriverSyncLocked(writer, pos, tile); err != nil {
			return 0, nil, false
		}
	case blockSyncTurret:
		if err := w.writeBlockTurretSyncLocked(writer, pos, tile); err != nil {
			return 0, nil, false
		}
	case blockSyncItemTurret:
		if err := w.writeBlockTurretSyncLocked(writer, pos, tile); err != nil {
			return 0, nil, false
		}
		if err := w.writeBlockItemTurretAmmoLocked(writer, tile); err != nil {
			return 0, nil, false
		}
	case blockSyncContinuousTurret:
		if err := w.writeBlockTurretSyncLocked(writer, pos, tile); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(w.blockSyncContinuousTurretLengthLocked(pos, tile)); err != nil {
			return 0, nil, false
		}
	case blockSyncPayloadTurret:
		if err := w.writeBlockTurretSyncLocked(writer, pos, tile); err != nil {
			return 0, nil, false
		}
		if err := w.writeBlockPayloadTurretAmmoLocked(writer, tile); err != nil {
			return 0, nil, false
		}
	case blockSyncPointDefenseTurret, blockSyncTractorBeamTurret:
		if err := writer.WriteFloat32(w.blockSyncTurretRotationLocked(pos, tile)); err != nil {
			return 0, nil, false
		}
	case blockSyncUnloader:
		sortItem := int16(-1)
		if item, ok := w.unloaderCfg[pos]; ok {
			sortItem = int16(item)
		}
		if err := writer.WriteInt16(sortItem); err != nil {
			return 0, nil, false
		}
	case blockSyncUnitFactory:
		payX, payY, payRotation := w.unitFactoryPayloadSyncFieldsLocked(pos, tile)
		if err := writer.WriteFloat32(payX); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(payY); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(payRotation); err != nil {
			return 0, nil, false
		}
		if err := writeBlockPayloadBytes(writer, tile.Build); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(w.unitFactoryProgressLocked(pos, tile)); err != nil {
			return 0, nil, false
		}
		currentPlan, _ := w.unitFactoryConfigValueLocked(pos, tile)
		if err := writer.WriteInt16(int16(currentPlan)); err != nil {
			return 0, nil, false
		}
		commandPos, command := w.unitFactoryCommandStateLocked(pos)
		if err := protocol.WriteVecNullable(writer, commandPos); err != nil {
			return 0, nil, false
		}
		if err := protocol.WriteCommand(writer, command); err != nil {
			return 0, nil, false
		}
	case blockSyncReconstructor:
		payX, payY, payRotation := w.reconstructorPayloadSyncFieldsLocked(pos, tile)
		if err := writer.WriteFloat32(payX); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(payY); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(payRotation); err != nil {
			return 0, nil, false
		}
		if err := writeBlockPayloadBytes(writer, tile.Build); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(w.reconstructorProgressLocked(pos)); err != nil {
			return 0, nil, false
		}
		commandPos, command := w.reconstructorCommandStateLocked(pos)
		if err := protocol.WriteVecNullable(writer, commandPos); err != nil {
			return 0, nil, false
		}
		if err := protocol.WriteCommand(writer, command); err != nil {
			return 0, nil, false
		}
	case blockSyncPayloadVoid:
		payX, payY, payRotation := w.payloadVoidSyncFieldsLocked(pos, tile)
		if err := writer.WriteFloat32(payX); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(payY); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(payRotation); err != nil {
			return 0, nil, false
		}
		if err := writeBlockPayloadBytes(writer, tile.Build); err != nil {
			return 0, nil, false
		}
	case blockSyncPayloadDeconstructor:
		payX, payY, payRotation := w.payloadDeconstructorSyncFieldsLocked(pos, tile)
		if err := writer.WriteFloat32(payX); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(payY); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(payRotation); err != nil {
			return 0, nil, false
		}
		if err := writeBlockPayloadBytes(writer, tile.Build); err != nil {
			return 0, nil, false
		}
		state := w.payloadDeconstructorStates[pos]
		if state == nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Progress, 0, 1)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteInt16(int16(len(state.Accum))); err != nil {
			return 0, nil, false
		}
		for _, value := range state.Accum {
			if err := writer.WriteFloat32(value); err != nil {
				return 0, nil, false
			}
		}
		if err := writePayloadDataBytes(writer, state.Deconstructing); err != nil {
			return 0, nil, false
		}
	case blockSyncGenericCrafter:
		state := w.crafterStates[pos]
		if err := writer.WriteFloat32(maxf(state.Progress, 0)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Warmup, 0, 1)); err != nil {
			return 0, nil, false
		}
		if name == "cultivator" {
			if err := writer.WriteFloat32(0); err != nil {
				return 0, nil, false
			}
		}
	case blockSyncSeparator:
		state := w.crafterStates[pos]
		if err := writer.WriteFloat32(maxf(state.Progress, 0)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Warmup, 0, 1)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteInt32(int32(state.Seed)); err != nil {
			return 0, nil, false
		}
	case blockSyncHeatProducer:
		state := w.crafterStates[pos]
		if err := writer.WriteFloat32(maxf(state.Progress, 0)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(clampf(state.Warmup, 0, 1)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(maxf(w.heatStates[pos], 0)); err != nil {
			return 0, nil, false
		}
	case blockSyncDrill:
		progress, warmup := w.drillSyncStateLocked(pos, name)
		if err := writer.WriteFloat32(maxf(progress, 0)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(clampf(warmup, 0, 1)); err != nil {
			return 0, nil, false
		}
	case blockSyncPowerGenerator:
		productionEfficiency, generateTime, _ := w.powerGeneratorSyncFieldsLocked(pos, tile, name, kind)
		if err := writer.WriteFloat32(clampf(productionEfficiency, 0, 1)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(maxf(generateTime, 0)); err != nil {
			return 0, nil, false
		}
	case blockSyncNuclearReactor, blockSyncHeaterGenerator:
		productionEfficiency, generateTime, extra := w.powerGeneratorSyncFieldsLocked(pos, tile, name, kind)
		if err := writer.WriteFloat32(clampf(productionEfficiency, 0, 1)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(maxf(generateTime, 0)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(maxf(extra, 0)); err != nil {
			return 0, nil, false
		}
	case blockSyncImpactReactor:
		productionEfficiency, generateTime, extra := w.powerGeneratorSyncFieldsLocked(pos, tile, name, kind)
		if err := writer.WriteFloat32(clampf(productionEfficiency, 0, 1)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(maxf(generateTime, 0)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(clampf(extra, 0, 1)); err != nil {
			return 0, nil, false
		}
	case blockSyncVariableReactor:
		productionEfficiency, heat, instability, warmup := w.variableReactorSyncFieldsLocked(pos, tile, name)
		if err := writer.WriteFloat32(clampf(productionEfficiency, 0, 1)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(0); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(maxf(heat, 0)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(clampf(instability, 0, 1)); err != nil {
			return 0, nil, false
		}
		if err := writer.WriteFloat32(clampf(warmup, 0, 1)); err != nil {
			return 0, nil, false
		}
	case blockSyncRepairTurret:
		rotation := float32(90)
		if state, ok := w.repairTurretStates[pos]; ok && state.Rotation != 0 {
			rotation = state.Rotation
		}
		if err := writer.WriteFloat32(rotation); err != nil {
			return 0, nil, false
		}
	case blockSyncBaseOnly, blockSyncStorage:
		// Base-only blocks have no subclass payload to append.
	default:
		return 0, nil, false
	}

	return revision, append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodeSpecialMapStreamBuildingPayloadLocked(pos int32, tile *Tile, name string) (byte, []byte, bool) {
	if w == nil || tile == nil || tile.Build == nil {
		return 0, nil, false
	}
	switch name {
	case "core-shard", "core-foundation", "core-nucleus", "core-bastion", "core-citadel", "core-acropolis":
		writer := protocol.NewWriter()
		if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, blockSyncNone); err != nil {
			return 0, nil, false
		}
		return 0, writer.Bytes(), true
	case "item-source":
		if payload, ok := w.encodeItemSourceMapStreamPayloadLocked(pos, tile); ok {
			return 0, payload, true
		}
		return 0, nil, false
	case "liquid-source":
		if payload, ok := w.encodeLiquidSourceMapStreamPayloadLocked(pos, tile); ok {
			return 1, payload, true
		}
		return 0, nil, false
	case "duct", "armored-duct":
		if payload, ok := w.encodeDuctMapStreamPayloadLocked(pos, tile); ok {
			return 1, payload, true
		}
		return 0, nil, false
	case "duct-router", "surge-router":
		if payload, ok := w.encodeDuctRouterMapStreamPayloadLocked(pos, tile); ok {
			return 1, payload, true
		}
		return 0, nil, false
	case "sorter", "inverted-sorter":
		if payload, ok := w.encodeSorterMapStreamPayloadLocked(pos, tile); ok {
			return 2, payload, true
		}
		return 0, nil, false
	case "overflow-gate", "underflow-gate":
		if payload, ok := w.encodeOverflowGateMapStreamPayloadLocked(pos, tile); ok {
			return 4, payload, true
		}
		return 0, nil, false
	case "phase-conveyor":
		if payload, ok := w.encodeItemBridgeMapStreamPayloadLocked(pos, tile); ok {
			return 1, payload, true
		}
		return 0, nil, false
	case "payload-conveyor", "reinforced-payload-conveyor":
		if payload, ok := w.encodePayloadConveyorMapStreamPayloadLocked(pos, tile); ok {
			return 0, payload, true
		}
		return 0, nil, false
	case "payload-router", "reinforced-payload-router":
		if payload, ok := w.encodePayloadRouterMapStreamPayloadLocked(pos, tile); ok {
			return 1, payload, true
		}
		return 0, nil, false
	case "payload-loader", "payload-unloader":
		if payload, ok := w.encodePayloadLoaderMapStreamPayloadLocked(pos, tile); ok {
			return 1, payload, true
		}
		return 0, nil, false
	case "payload-mass-driver", "large-payload-mass-driver":
		if payload, ok := w.encodePayloadMassDriverMapStreamPayloadLocked(pos, tile); ok {
			return 1, payload, true
		}
		return 0, nil, false
	case "payload-void":
		if payload, ok := w.encodePayloadVoidMapStreamPayloadLocked(pos, tile); ok {
			return 0, payload, true
		}
		return 0, nil, false
	case "small-deconstructor", "deconstructor", "payload-deconstructor":
		if payload, ok := w.encodePayloadDeconstructorMapStreamPayloadLocked(pos, tile); ok {
			return 0, payload, true
		}
		return 0, nil, false
	}
	return 0, nil, false
}

func (w *World) encodeItemSourceMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, blockSyncBaseOnly); err != nil {
		return nil, false
	}
	itemID := int16(-1)
	if item, ok := w.itemSourceCfg[pos]; ok {
		itemID = int16(item)
	}
	if err := writer.WriteInt16(itemID); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodeLiquidSourceMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, blockSyncBaseOnly); err != nil {
		return nil, false
	}
	liquidID := int16(-1)
	if liquid, ok := w.liquidSourceCfg[pos]; ok {
		liquidID = int16(liquid)
	}
	if err := writer.WriteInt16(liquidID); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodeDuctMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, blockSyncNone); err != nil {
		return nil, false
	}
	st := w.ductStateLocked(pos, tile)
	if err := writer.WriteByte(st.RecDir); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodeDuctRouterMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, blockSyncNone); err != nil {
		return nil, false
	}
	sortItem := int16(-1)
	if item, ok := w.sorterCfg[pos]; ok {
		sortItem = int16(item)
	}
	if err := writer.WriteInt16(sortItem); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodeSorterMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, blockSyncBaseOnly); err != nil {
		return nil, false
	}
	sortItem := int16(-1)
	if item, ok := w.sorterCfg[pos]; ok {
		sortItem = int16(item)
	}
	if err := writer.WriteInt16(sortItem); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodeOverflowGateMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, blockSyncBaseOnly); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodeItemBridgeMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, name, blockSyncBaseOnly); err != nil {
		return nil, false
	}
	link := int32(-1)
	linked := false
	if target, ok := w.bridgeLinks[pos]; ok && w.model != nil && target >= 0 && int(target) < len(w.model.Tiles) {
		targetTile := &w.model.Tiles[target]
		link = packTilePos(targetTile.X, targetTile.Y)
		linked = true
	}
	if err := writer.WriteInt32(link); err != nil {
		return nil, false
	}
	warmup := float32(0)
	if linked {
		warmup = 1
		if name == "phase-conveyor" && w.isPowerRelevantBuildingLocked(tile) {
			warmup = clampf(w.blockSyncPowerStatusLocked(pos, tile, name), 0, 1)
		}
	}
	if err := writer.WriteFloat32(warmup); err != nil {
		return nil, false
	}
	incoming := make([]int32, 0, 4)
	for otherPos, target := range w.bridgeLinks {
		if target == pos {
			if w.model == nil || otherPos < 0 || int(otherPos) >= len(w.model.Tiles) {
				continue
			}
			otherTile := &w.model.Tiles[otherPos]
			incoming = append(incoming, packTilePos(otherTile.X, otherTile.Y))
		}
	}
	sort.Slice(incoming, func(i, j int) bool { return incoming[i] < incoming[j] })
	if len(incoming) > 255 {
		incoming = incoming[:255]
	}
	if err := writer.WriteByte(byte(len(incoming))); err != nil {
		return nil, false
	}
	for _, other := range incoming {
		if err := writer.WriteInt32(other); err != nil {
			return nil, false
		}
	}
	moved := linked && (len(w.bridgeBuffers[pos]) > 0 || w.totalItemsAtLocked(pos) > 0 || w.transportAccum[pos] > 0.001)
	if err := writer.WriteBool(moved); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) writeOfficialPayloadValueLocked(writer *protocol.Writer, payload *payloadData) error {
	if writer == nil {
		return nil
	}
	if payload == nil {
		return writer.WriteBool(false)
	}
	obj, ok := payloadDataToProtocolPayload(clonePayloadData(*payload))
	if !ok || obj == nil {
		return writer.WriteBool(false)
	}
	return protocol.WritePayload(writer, obj)
}

func (w *World) encodePayloadConveyorMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block)))), blockSyncNone); err != nil {
		return nil, false
	}
	st := w.payloadStateLocked(pos)
	payRotation := payloadRotationDegrees(st.Payload, tile.Rotation)
	moveTime := payloadMoveTimeByName(strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block)))))
	if moveTime <= 0 {
		moveTime = 1
	}
	if err := writer.WriteFloat32(clampf(st.Move/moveTime, 0, 1)); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(payRotation); err != nil {
		return nil, false
	}
	if err := w.writeOfficialPayloadValueLocked(writer, st.Payload); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodePayloadRouterMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	base, ok := w.encodePayloadConveyorMapStreamPayloadLocked(pos, tile)
	if !ok {
		return nil, false
	}
	writer := protocol.NewWriter()
	if err := writer.WriteBytes(base); err != nil {
		return nil, false
	}
	filter := w.payloadRouterCfg[pos]
	ctype := byte(0xFF)
	cid := int16(-1)
	if filter != nil {
		ctype = byte(filter.ContentType())
		cid = filter.ID()
	}
	if err := writer.WriteByte(ctype); err != nil {
		return nil, false
	}
	if err := writer.WriteInt16(cid); err != nil {
		return nil, false
	}
	if err := writer.WriteByte(w.payloadStateLocked(pos).RecDir); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodePayloadLoaderMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block)))), blockSyncNone); err != nil {
		return nil, false
	}
	st := w.payloadStateLocked(pos)
	payX, payY, payRotation := w.payloadInputSyncFieldsLocked(pos, tile, 1, payloadRotationDegrees(st.Payload, tile.Rotation))
	if err := writer.WriteFloat32(payX); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(payY); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(payRotation); err != nil {
		return nil, false
	}
	if err := w.writeOfficialPayloadValueLocked(writer, st.Payload); err != nil {
		return nil, false
	}
	if err := writer.WriteBool(st.Exporting); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodePayloadMassDriverMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block)))), blockSyncNone); err != nil {
		return nil, false
	}
	st := w.payloadStateLocked(pos)
	driver := w.payloadDriverStateLocked(pos)
	payRotation := payloadRotationDegrees(st.Payload, tile.Rotation)
	if err := writer.WriteFloat32(0); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(0); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(payRotation); err != nil {
		return nil, false
	}
	if err := w.writeOfficialPayloadValueLocked(writer, st.Payload); err != nil {
		return nil, false
	}
	link := int32(-1)
	if v, ok := w.payloadDriverLinks[pos]; ok {
		link = v
	}
	if err := writer.WriteInt32(link); err != nil {
		return nil, false
	}
	turretRotation := float32(tile.Rotation) * 90
	if link >= 0 && w.model != nil && int(link) < len(w.model.Tiles) {
		targetTile := &w.model.Tiles[link]
		turretRotation = lookAt(float32(tile.X*8+4), float32(tile.Y*8+4), float32(targetTile.X*8+4), float32(targetTile.Y*8+4))
	}
	if err := writer.WriteFloat32(turretRotation); err != nil {
		return nil, false
	}
	stateOrdinal := byte(0)
	if st.Payload != nil {
		stateOrdinal = 2
	} else if w.payloadDriverIncomingShotsLocked(pos) > 0 {
		stateOrdinal = 1
	}
	if err := writer.WriteByte(stateOrdinal); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(driver.ReloadCounter); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(driver.Charge); err != nil {
		return nil, false
	}
	if err := writer.WriteBool(st.Payload != nil); err != nil {
		return nil, false
	}
	if err := writer.WriteBool(driver.Charge > 0); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodePayloadVoidMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block)))), blockSyncNone); err != nil {
		return nil, false
	}
	st := w.payloadStateLocked(pos)
	payX, payY, payRotation := w.payloadVoidSyncFieldsLocked(pos, tile)
	if err := writer.WriteFloat32(payX); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(payY); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(payRotation); err != nil {
		return nil, false
	}
	if err := w.writeOfficialPayloadValueLocked(writer, st.Payload); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) encodePayloadDeconstructorMapStreamPayloadLocked(pos int32, tile *Tile) ([]byte, bool) {
	writer := protocol.NewWriter()
	if err := w.writeBlockBaseSyncLocked(writer, pos, tile, strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block)))), blockSyncNone); err != nil {
		return nil, false
	}
	st := w.payloadStateLocked(pos)
	state := w.payloadDeconstructorStateLocked(pos)
	if state == nil {
		return nil, false
	}
	baseRotation := payloadRotationDegrees(st.Payload, tile.Rotation)
	if state.Deconstructing != nil {
		baseRotation = state.PayRotation
	}
	payX, payY := float32(0), float32(0)
	if state.Deconstructing == nil {
		payX, payY, baseRotation = w.payloadDeconstructorSyncFieldsLocked(pos, tile)
	}
	if err := writer.WriteFloat32(payX); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(payY); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(baseRotation); err != nil {
		return nil, false
	}
	if err := w.writeOfficialPayloadValueLocked(writer, st.Payload); err != nil {
		return nil, false
	}
	if err := writer.WriteFloat32(clampf(state.Progress, 0, 1)); err != nil {
		return nil, false
	}
	if state.Accum != nil {
		if err := writer.WriteInt16(int16(len(state.Accum))); err != nil {
			return nil, false
		}
		for _, v := range state.Accum {
			if err := writer.WriteFloat32(v); err != nil {
				return nil, false
			}
		}
	} else {
		if err := writer.WriteInt16(0); err != nil {
			return nil, false
		}
	}
	if err := w.writeOfficialPayloadValueLocked(writer, state.Deconstructing); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func mapStreamRevisionForBlock(name string, kind blockSyncKind) (byte, bool) {
	switch kind {
	case blockSyncBaseOnly:
		return 0, true
	case blockSyncConveyor:
		return 1, true
	case blockSyncStackConveyor:
		return 0, true
	case blockSyncMassDriver:
		return 0, true
	case blockSyncTurret:
		return 1, true
	case blockSyncItemTurret:
		return 2, true
	case blockSyncContinuousTurret:
		return 3, true
	case blockSyncPayloadTurret:
		return 1, true
	case blockSyncPointDefenseTurret, blockSyncTractorBeamTurret:
		return 0, true
	case blockSyncUnloader:
		return 1, true
	case blockSyncUnitFactory, blockSyncReconstructor:
		return 3, true
	case blockSyncPayloadVoid, blockSyncPayloadDeconstructor:
		return 0, true
	case blockSyncGenericCrafter, blockSyncSeparator, blockSyncHeatProducer:
		return 0, true
	case blockSyncDrill,
		blockSyncPowerGenerator,
		blockSyncNuclearReactor,
		blockSyncImpactReactor,
		blockSyncHeaterGenerator,
		blockSyncVariableReactor:
		return 1, true
	case blockSyncRepairTurret:
		return 1, true
	case blockSyncStorage:
		return 0, true
	default:
		_ = name
		return 0, false
	}
}
