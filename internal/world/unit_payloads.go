package world

import (
	"math"
	"strings"

	"mdt-server/internal/protocol"
)

const (
	unitPayloadPickupRange  = float32(32)
	buildPayloadPickupRange = float32(40)
)

func (w *World) removeEntityLocked(id int32) (RawEntity, bool) {
	if w == nil || w.model == nil {
		return RawEntity{}, false
	}
	ent, ok := w.model.RemoveEntity(id)
	if !ok {
		return RawEntity{}, false
	}
	delete(w.unitMountCDs, id)
	delete(w.unitMountStates, id)
	delete(w.unitTargets, id)
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:   EntityEventRemoved,
		Entity: ent,
	})
	return ent, true
}

func (w *World) serializePayloadDataLocked(payload *payloadData) ([]byte, bool) {
	if payload == nil {
		return nil, false
	}
	if len(payload.Serialized) > 0 {
		return append([]byte(nil), payload.Serialized...), true
	}
	if payload.Kind == payloadKindUnit && payload.UnitState != nil {
		unit := cloneRawEntity(*payload.UnitState)
		entity := w.entitySyncUnitLocked(unit, nil, w.entitySyncControllerLocked(unit))
		writer := protocol.NewWriter()
		if err := writer.WriteBool(true); err != nil {
			return nil, false
		}
		if err := writer.WriteByte(protocol.PayloadUnit); err != nil {
			return nil, false
		}
		if err := writer.WriteByte(entity.ClassID()); err != nil {
			return nil, false
		}
		if err := entity.WriteEntity(writer); err != nil {
			return nil, false
		}
		return append([]byte(nil), writer.Bytes()...), true
	}
	obj, ok := payloadDataToProtocolPayload(clonePayloadData(*payload))
	if !ok || obj == nil {
		return nil, false
	}
	writer := protocol.NewWriter()
	if err := protocol.WritePayload(writer, obj); err != nil {
		return nil, false
	}
	return append([]byte(nil), writer.Bytes()...), true
}

func (w *World) normalizeEntityPayloadsLocked(e *RawEntity) {
	if e == nil {
		return
	}
	if len(e.Payloads) > 0 || len(e.Payload) == 0 {
		return
	}
	if payload, ok := decodePayloadData(e.Payload); ok && payload != nil {
		e.Payloads = append(e.Payloads, clonePayloadData(*payload))
	}
	e.Payload = nil
}

func (w *World) payloadWorldSizeLocked(payload *payloadData) float32 {
	if payload == nil {
		return 0
	}
	if payload.Kind == payloadKindBlock {
		size := w.payloadSizeBlocksLocked(payload)
		if size <= 0 {
			return 0
		}
		return float32(size * 8)
	}
	if payload.UnitState != nil {
		if payload.UnitState.HitRadius > 0 {
			return maxf(payload.UnitState.HitRadius, 8)
		}
	}
	if payload.UnitTypeID > 0 {
		if prof, ok := w.unitRuntimeProfileForTypeLocked(payload.UnitTypeID); ok && prof.HitSize > 0 {
			return maxf(prof.HitSize, 8)
		}
	}
	return 8
}

func (w *World) payloadUsageLocked(payload *payloadData) float32 {
	size := w.payloadWorldSizeLocked(payload)
	if size <= 0 {
		return 0
	}
	return size * size
}

func (w *World) entityPayloadUsageLocked(e *RawEntity) float32 {
	if e == nil {
		return 0
	}
	w.normalizeEntityPayloadsLocked(e)
	var used float32
	for i := range e.Payloads {
		used += w.payloadUsageLocked(&e.Payloads[i])
	}
	return used
}

func (w *World) canEntityCarryPayloadLocked(e *RawEntity, payload *payloadData) bool {
	if e == nil || payload == nil || e.PayloadCapacity <= 0 {
		return false
	}
	if payload.Kind == payloadKindUnit {
		if prof, ok := w.unitRuntimeProfileForEntityLocked(*e); ok && !prof.PickupUnits {
			return false
		}
	}
	required := w.payloadUsageLocked(payload)
	if required <= 0 {
		return false
	}
	return w.entityPayloadUsageLocked(e)+required <= e.PayloadCapacity+0.001
}

func (w *World) entityPayloadHitSizeLocked(unit RawEntity) float32 {
	if unit.HitRadius > 0 {
		return maxf(unit.HitRadius, 8)
	}
	if prof, ok := w.unitRuntimeProfileForEntityLocked(unit); ok && prof.HitSize > 0 {
		return maxf(prof.HitSize, 8)
	}
	return 8
}

func buildRotationFromDegrees(rotation float32) int8 {
	steps := int(math.Round(float64(rotation / 90)))
	for steps < 0 {
		steps += 4
	}
	return int8(steps % 4)
}

func (w *World) unitPayloadFromEntityLocked(src RawEntity) *payloadData {
	unit := cloneRawEntity(src)
	unit.ID = 0
	entity := w.entitySyncUnitLocked(unit, nil, w.entitySyncControllerLocked(unit))
	writer := protocol.NewWriter()
	if err := writer.WriteBool(true); err != nil {
		return nil
	}
	if err := writer.WriteByte(protocol.PayloadUnit); err != nil {
		return nil
	}
	if err := writer.WriteByte(entity.ClassID()); err != nil {
		return nil
	}
	if err := entity.WriteEntity(writer); err != nil {
		return nil
	}
	return &payloadData{
		Kind:       payloadKindUnit,
		UnitTypeID: src.TypeID,
		Rotation:   buildRotationFromDegrees(src.Rotation),
		Serialized: append([]byte(nil), writer.Bytes()...),
		Health:     src.Health,
		MaxHealth:  src.MaxHealth,
		UnitState:  &unit,
	}
}

func (w *World) buildPayloadFromTileLocked(tile *Tile) *payloadData {
	if tile == nil || tile.Build == nil || tile.Block == 0 {
		return nil
	}
	payload := &payloadData{
		Kind:      payloadKindBlock,
		BlockID:   int16(tile.Block),
		Rotation:  tile.Rotation,
		Config:    append([]byte(nil), tile.Build.Config...),
		Items:     append([]ItemStack(nil), tile.Build.Items...),
		Liquids:   append([]LiquidStack(nil), tile.Build.Liquids...),
		Health:    tile.Build.Health,
		MaxHealth: tile.Build.MaxHealth,
	}
	if serialized, ok := w.serializePayloadDataLocked(payload); ok {
		payload.Serialized = serialized
	}
	return payload
}

func (w *World) detachBuildAsPayloadLocked(pos int32, owner int32) (*payloadData, bool) {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return nil, false
	}
	tile := &w.model.Tiles[pos]
	if tile.Block == 0 || tile.Build == nil {
		return nil, false
	}
	payload := w.buildPayloadFromTileLocked(tile)
	if payload == nil {
		return nil, false
	}
	blockID := int16(tile.Block)
	teamOld := tile.Team
	if tile.Build.Team != 0 {
		teamOld = tile.Build.Team
	}
	powerRelevant := w.isPowerRelevantBuildingLocked(tile)
	if powerRelevant {
		w.clearPowerLinksForBuildingLocked(pos)
	}
	// CRITICAL: Remove from indices BEFORE clearing tile data
	w.removeActiveTileIndexLocked(pos, tile)
	w.setBuildingOccupancyLocked(pos, tile, false)
	tile.Block = 0
	tile.Rotation = 0
	tile.Team = 0
	tile.Build = nil
	w.clearBuildingRuntimeLocked(pos)
	if powerRelevant {
		w.invalidatePowerNetsLocked()
	}
	w.refreshCoreStorageLinksLocked()
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventBuildDestroyed,
		BuildPos:   packTilePos(tile.X, tile.Y),
		BuildOwner: owner,
		BuildTeam:  teamOld,
		BuildBlock: blockID,
	})
	return payload, true
}

func (w *World) restoreDroppedBuildPayloadLocked(pos int32, carrier RawEntity, payload *payloadData) bool {
	if w == nil || w.model == nil || payload == nil || payload.BlockID <= 0 || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	if tile.Block != 0 || tile.Build != nil {
		return false
	}
	var config any
	if len(payload.Config) > 0 {
		if decoded, ok := decodeStoredBuildingConfig(payload.Config); ok {
			config = decoded
		}
	}
	w.placeTileLocked(tile, carrier.Team, payload.BlockID, payload.Rotation, config, carrier.PlayerID)
	if tile.Build == nil {
		return false
	}
	tile.Build.Items = append([]ItemStack(nil), payload.Items...)
	tile.Build.Liquids = append([]LiquidStack(nil), payload.Liquids...)
	if payload.MaxHealth > 0 {
		tile.Build.MaxHealth = maxf(payload.MaxHealth, tile.Build.MaxHealth)
	}
	if payload.Health > 0 {
		tile.Build.Health = clampf(payload.Health, 1, maxf(tile.Build.MaxHealth, payload.Health))
	}
	if len(payload.Config) > 0 {
		tile.Build.Config = append(tile.Build.Config[:0], payload.Config...)
	}
	w.syncPayloadTileLocked(tile, nil)
	return true
}

func (w *World) canDropPayloadUnitLocked(payload *payloadData, x, y float32) bool {
	if w == nil || w.model == nil || payload == nil {
		return false
	}
	ent, typeID, ok := resolvePayloadUnitEntity(payload)
	if !ok || typeID <= 0 {
		return false
	}
	tx := int(x / 8)
	ty := int(y / 8)
	if _, ok := w.buildingOccupyingCellLocked(tx, ty); ok {
		return false
	}
	if !isEntityFlying(ent) {
		selfRadius := ent.HitRadius
		if selfRadius <= 0 {
			selfRadius = entityHitRadiusForType(typeID)
		}
		maxDist := selfRadius * 1.05
		for _, other := range w.model.Entities {
			if other.Health <= 0 || isEntityFlying(other) {
				continue
			}
			otherRadius := other.HitRadius
			if otherRadius <= 0 {
				otherRadius = entityHitRadiusForType(other.TypeID)
			}
			limit := maxDist + otherRadius*0.5
			dx := other.X - x
			dy := other.Y - y
			if dx*dx+dy*dy <= limit*limit {
				return false
			}
		}
	}
	return true
}

func resolvePayloadUnitEntity(payload *payloadData) (RawEntity, int16, bool) {
	if payload == nil {
		return RawEntity{}, 0, false
	}
	if payload.UnitState == nil && len(payload.Serialized) > 0 {
		if decoded, ok := decodePayloadData(payload.Serialized); ok && decoded != nil {
			if payload.UnitTypeID <= 0 && decoded.UnitTypeID > 0 {
				payload.UnitTypeID = decoded.UnitTypeID
			}
			if payload.Health <= 0 && decoded.Health > 0 {
				payload.Health = decoded.Health
			}
			if payload.MaxHealth <= 0 && decoded.MaxHealth > 0 {
				payload.MaxHealth = decoded.MaxHealth
			}
			if payload.UnitState == nil && decoded.UnitState != nil {
				clone := cloneRawEntity(*decoded.UnitState)
				payload.UnitState = &clone
			}
		}
	}
	if payload.UnitState != nil {
		unit := cloneRawEntity(*payload.UnitState)
		typeID := unit.TypeID
		if typeID <= 0 {
			typeID = payload.UnitTypeID
			unit.TypeID = typeID
		}
		if unit.MaxHealth <= 0 && payload.MaxHealth > 0 {
			unit.MaxHealth = payload.MaxHealth
		}
		if unit.Health <= 0 && payload.Health > 0 {
			unit.Health = clampf(payload.Health, 1, maxf(unit.MaxHealth, payload.Health))
		}
		return unit, typeID, typeID > 0
	}
	if payload.UnitTypeID <= 0 {
		return RawEntity{}, 0, false
	}
	return RawEntity{TypeID: payload.UnitTypeID}, payload.UnitTypeID, true
}

func (w *World) dropUnitPayloadLocked(carrier RawEntity, payload *payloadData, x, y float32) bool {
	if w == nil || w.model == nil || payload == nil || !w.canDropPayloadUnitLocked(payload, x, y) {
		return false
	}
	resolved, typeID, ok := resolvePayloadUnitEntity(payload)
	if !ok || typeID <= 0 {
		return false
	}
	ent := w.newProducedUnitEntityLocked(typeID, carrier.Team, x, y, carrier.Rotation)
	if payload.UnitState != nil {
		ent = resolved
		ent.ID = 0
		ent.X = x
		ent.Y = y
		if ent.Team == 0 {
			ent.Team = carrier.Team
		}
		if ent.Rotation == 0 {
			ent.Rotation = carrier.Rotation
		}
	} else {
		if payload.MaxHealth > 0 {
			ent.MaxHealth = maxf(payload.MaxHealth, 1)
		}
		if payload.Health > 0 {
			ent.Health = clampf(payload.Health, 1, maxf(ent.MaxHealth, payload.Health))
		}
	}
	if ent.Health <= 0 {
		ent.Health = maxf(ent.MaxHealth, 1)
	}
	if ent.MaxHealth <= 0 {
		ent.MaxHealth = maxf(ent.Health, 1)
	}
	ent.PlayerID = 0
	w.ensureEntityDefaults(&ent)
	if isEntityFlying(ent) {
		ent.Elevation = maxf(ent.Elevation, 1)
	}
	w.model.AddEntity(ent)
	return true
}

func (w *World) tryInsertPayloadIntoBuildingLocked(pos int32, payload *payloadData) bool {
	if w == nil || w.model == nil || payload == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || !w.payloadAcceptsLocked(pos, pos, payload) {
		return false
	}
	state := w.payloadStateLocked(pos)
	copyPayload := clonePayloadData(*payload)
	state.Payload = &copyPayload
	state.Move = 0
	state.Work = 0
	state.Exporting = false
	w.syncPayloadTileLocked(tile, state.Payload)
	return true
}

func (w *World) insertUnitPayloadIntoBuildingLocked(pos int32, payload *payloadData, setRouterRecDir bool) bool {
	if w == nil || w.model == nil || payload == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 {
		return false
	}
	state := w.payloadStateLocked(pos)
	copyPayload := clonePayloadData(*payload)
	state.Payload = &copyPayload
	state.Move = 0
	state.Work = 0
	state.Exporting = false
	if setRouterRecDir {
		switch w.blockNameByID(int16(tile.Block)) {
		case "payload-router", "reinforced-payload-router":
			state.RecDir = byte(tileRotationNorm(tile.Rotation))
		}
	}
	w.syncPayloadTileLocked(tile, state.Payload)
	return true
}

func (w *World) unitAllowedInPayloadsLocked(unit RawEntity) bool {
	if unit.TypeID <= 0 {
		return false
	}
	name := ""
	if w.unitNamesByID != nil {
		name = w.unitNamesByID[unit.TypeID]
	}
	name = normalizeUnitName(name)
	switch name {
	case "manifold", "assembly-drone":
		return false
	}
	if strings.Contains(name, "missile") {
		return false
	}
	if prof, ok := w.unitRuntimeProfileForEntityLocked(unit); ok {
		return prof.AllowedInPayloads
	}
	return true
}

func (w *World) unitStandingOnBuildingLocked(unit RawEntity, buildPos int32) bool {
	if w == nil || w.model == nil {
		return false
	}
	tx := int(math.Floor(float64(unit.X / 8)))
	ty := int(math.Floor(float64(unit.Y / 8)))
	if !w.model.InBounds(tx, ty) {
		return false
	}
	pos, ok := w.buildingOccupyingCellLocked(tx, ty)
	return ok && pos == buildPos
}

func (w *World) currentBuildingPayloadLocked(pos int32, tile *Tile) (*payloadData, bool) {
	if w == nil || w.model == nil || tile == nil || tile.Build == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return nil, false
	}
	if st, ok := w.payloadStates[pos]; ok && st != nil && st.Payload != nil {
		payload := clonePayloadData(*st.Payload)
		return &payload, true
	}
	if len(tile.Build.Payload) == 0 {
		return nil, false
	}
	payload, ok := decodePayloadData(tile.Build.Payload)
	if !ok || payload == nil {
		return nil, false
	}
	copyPayload := clonePayloadData(*payload)
	return &copyPayload, true
}

func (w *World) takeBuildingPayloadLocked(pos int32, tile *Tile) (*payloadData, bool) {
	payload, ok := w.currentBuildingPayloadLocked(pos, tile)
	if !ok || payload == nil {
		return nil, false
	}
	w.clearPayloadLocked(pos, tile)
	return payload, true
}

func (w *World) controlSelectUnitPayloadAcceptedByBuildingLocked(buildPos int32, tile *Tile, unit RawEntity, payload *payloadData) bool {
	if w == nil || w.model == nil || tile == nil || tile.Build == nil || tile.Block == 0 || payload == nil {
		return false
	}
	if w.payloadStateLocked(buildPos).Payload != nil {
		return false
	}
	size := w.payloadSizeBlocksLocked(payload)
	name := w.blockNameByID(int16(tile.Block))
	switch name {
	case "payload-conveyor", "reinforced-payload-conveyor", "payload-router", "reinforced-payload-router":
		return size <= 3
	case "payload-void":
		return true
	case "small-deconstructor", "deconstructor", "payload-deconstructor":
		return w.payloadDeconstructorAcceptsPayloadLocked(buildPos, tile, payload)
	default:
		if isReconstructorBlockName(name) {
			return w.reconstructorAcceptsPayloadLocked(buildPos, tile, payload, buildPos, false)
		}
		return false
	}
}

func (w *World) enteredUnitPayloadAcceptedByBuildingLocked(buildPos int32, tile *Tile, payload *payloadData) bool {
	if w == nil || w.model == nil || tile == nil || tile.Build == nil || tile.Block == 0 || payload == nil {
		return false
	}
	if w.payloadStateLocked(buildPos).Payload != nil {
		return false
	}
	size := w.payloadSizeBlocksLocked(payload)
	name := w.blockNameByID(int16(tile.Block))
	switch name {
	case "payload-conveyor", "reinforced-payload-conveyor", "payload-router", "reinforced-payload-router":
		return size <= 3
	case "payload-mass-driver":
		return size <= 2
	case "large-payload-mass-driver":
		return size <= 4
	case "payload-void":
		return true
	case "small-deconstructor", "deconstructor", "payload-deconstructor":
		if payload.UnitState != nil && payload.UnitState.SpawnedByCore {
			return false
		}
		return w.payloadDeconstructorAcceptsPayloadLocked(buildPos, tile, payload)
	default:
		if isReconstructorBlockName(name) {
			return w.reconstructorAcceptsPayloadLocked(buildPos, tile, payload, buildPos, false)
		}
		return false
	}
}

func (w *World) controlSelectPayloadUnitLocked(buildPos, unitID int32) bool {
	if w == nil || w.model == nil || buildPos < 0 || int(buildPos) >= len(w.model.Tiles) || unitID == 0 {
		return false
	}
	unitIndex := -1
	for i := range w.model.Entities {
		if w.model.Entities[i].ID == unitID {
			unitIndex = i
			break
		}
	}
	if unitIndex < 0 {
		return false
	}
	tile := &w.model.Tiles[buildPos]
	if tile.Build == nil || tile.Block == 0 {
		return false
	}
	team := tile.Team
	if tile.Build.Team != 0 {
		team = tile.Build.Team
	}
	unit := w.model.Entities[unitIndex]
	if unit.Health <= 0 || unit.Team == 0 || unit.Team != team || unit.SpawnedByCore {
		return false
	}
	if !w.unitAllowedInPayloadsLocked(unit) || !w.unitStandingOnBuildingLocked(unit, buildPos) {
		return false
	}

	payload := w.unitPayloadFromEntityLocked(unit)
	if payload == nil || !w.controlSelectUnitPayloadAcceptedByBuildingLocked(buildPos, tile, unit, payload) {
		return false
	}
	if !w.insertUnitPayloadIntoBuildingLocked(buildPos, payload, true) {
		return false
	}
	_, _ = w.removeEntityLocked(unitID)
	return true
}

func (w *World) enterUnitPayloadLocked(buildPos, unitID int32) bool {
	if w == nil || w.model == nil || buildPos < 0 || int(buildPos) >= len(w.model.Tiles) || unitID == 0 {
		return false
	}
	unitIndex := -1
	for i := range w.model.Entities {
		if w.model.Entities[i].ID == unitID {
			unitIndex = i
			break
		}
	}
	if unitIndex < 0 {
		return false
	}
	tile := &w.model.Tiles[buildPos]
	if tile.Build == nil || tile.Block == 0 {
		return false
	}
	team := tile.Team
	if tile.Build.Team != 0 {
		team = tile.Build.Team
	}
	unit := w.model.Entities[unitIndex]
	if unit.Health <= 0 || unit.Team == 0 || unit.Team != team {
		return false
	}
	payload := w.unitPayloadFromEntityLocked(unit)
	if payload == nil || !w.enteredUnitPayloadAcceptedByBuildingLocked(buildPos, tile, payload) {
		return false
	}
	if !w.insertUnitPayloadIntoBuildingLocked(buildPos, payload, true) {
		return false
	}
	_, _ = w.removeEntityLocked(unitID)
	return true
}

func (w *World) ControlSelectPayloadUnitPacked(buildPos, unitID int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	pos, ok := w.tileIndexFromPackedPosLocked(buildPos)
	if !ok {
		return false
	}
	return w.controlSelectPayloadUnitLocked(pos, unitID)
}

func (w *World) RequestUnitPayload(carrierID, targetID int32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || carrierID == 0 || targetID == 0 || carrierID == targetID {
		return RawEntity{}, false
	}
	carrierIndex := -1
	targetIndex := -1
	for i := range w.model.Entities {
		switch w.model.Entities[i].ID {
		case carrierID:
			carrierIndex = i
		case targetID:
			targetIndex = i
		}
	}
	if carrierIndex < 0 || targetIndex < 0 {
		return RawEntity{}, false
	}
	carrier := &w.model.Entities[carrierIndex]
	target := w.model.Entities[targetIndex]
	if carrier.Team == 0 || target.Team == 0 || carrier.Team != target.Team || target.Health <= 0 || target.PlayerID != 0 {
		return RawEntity{}, false
	}
	if isEntityFlying(target) {
		return RawEntity{}, false
	}
	if !w.unitAllowedInPayloadsLocked(target) {
		return RawEntity{}, false
	}
	pickupRange := w.entityPayloadHitSizeLocked(*carrier)*2 + w.entityPayloadHitSizeLocked(target)*2
	if pickupRange <= 0 {
		pickupRange = unitPayloadPickupRange
	}
	if squaredWorldDistance(carrier.X, carrier.Y, target.X, target.Y) > pickupRange*pickupRange {
		return RawEntity{}, false
	}
	payload := w.unitPayloadFromEntityLocked(target)
	if payload == nil || !w.canEntityCarryPayloadLocked(carrier, payload) {
		return RawEntity{}, false
	}
	if _, ok := w.removeEntityLocked(targetID); !ok {
		return RawEntity{}, false
	}
	w.normalizeEntityPayloadsLocked(carrier)
	carrier.Payloads = append(carrier.Payloads, *payload)
	carrier.Payload = nil
	w.model.EntitiesRev++
	return cloneRawEntity(*carrier), true
}

func (w *World) requestBuildPayloadLocked(carrierID, buildPos int32) (RawEntity, bool) {
	if w.model == nil || carrierID == 0 || buildPos < 0 || int(buildPos) >= len(w.model.Tiles) {
		return RawEntity{}, false
	}
	carrierIndex := -1
	for i := range w.model.Entities {
		if w.model.Entities[i].ID == carrierID {
			carrierIndex = i
			break
		}
	}
	if carrierIndex < 0 {
		return RawEntity{}, false
	}
	carrier := &w.model.Entities[carrierIndex]
	tile := &w.model.Tiles[buildPos]
	if tile.Build == nil || tile.Block == 0 || tile.Build.Team == 0 || tile.Build.Team != carrier.Team {
		return RawEntity{}, false
	}
	cx := float32(tile.X*8 + 4)
	cy := float32(tile.Y*8 + 4)
	pickupRange := float32(w.blockSizeForTileLocked(tile))*8*1.2 + 8*5
	if squaredWorldDistance(carrier.X, carrier.Y, cx, cy) > pickupRange*pickupRange {
		return RawEntity{}, false
	}
	if currentPayload, ok := w.currentBuildingPayloadLocked(buildPos, tile); ok && currentPayload != nil {
		if w.canEntityCarryPayloadLocked(carrier, currentPayload) {
			taken, ok := w.takeBuildingPayloadLocked(buildPos, tile)
			if !ok || taken == nil {
				return RawEntity{}, false
			}
			w.normalizeEntityPayloadsLocked(carrier)
			carrier.Payloads = append(carrier.Payloads, *taken)
			carrier.Payload = nil
			w.model.EntitiesRev++
			return cloneRawEntity(*carrier), true
		}
	}
	buildPayload := w.buildPayloadFromTileLocked(tile)
	if buildPayload == nil || !w.canEntityCarryPayloadLocked(carrier, buildPayload) {
		return RawEntity{}, false
	}
	if _, ok := w.detachBuildAsPayloadLocked(buildPos, carrier.PlayerID); !ok {
		return RawEntity{}, false
	}
	w.normalizeEntityPayloadsLocked(carrier)
	carrier.Payloads = append(carrier.Payloads, *buildPayload)
	carrier.Payload = nil
	w.model.EntitiesRev++
	return cloneRawEntity(*carrier), true
}

func (w *World) RequestBuildPayload(carrierID, buildPos int32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.requestBuildPayloadLocked(carrierID, buildPos)
}

func (w *World) RequestBuildPayloadPacked(carrierID, buildPos int32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	pos, ok := w.tileIndexFromPackedPosLocked(buildPos)
	if !ok {
		return RawEntity{}, false
	}
	return w.requestBuildPayloadLocked(carrierID, pos)
}

func (w *World) RequestDropPayload(carrierID int32, x, y float32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || carrierID == 0 {
		return RawEntity{}, false
	}
	carrierIndex := -1
	for i := range w.model.Entities {
		if w.model.Entities[i].ID == carrierID {
			carrierIndex = i
			break
		}
	}
	if carrierIndex < 0 {
		return RawEntity{}, false
	}
	carrier := &w.model.Entities[carrierIndex]
	w.normalizeEntityPayloadsLocked(carrier)
	if len(carrier.Payloads) == 0 {
		return RawEntity{}, false
	}
	payload := clonePayloadData(carrier.Payloads[0])
	pos := packTilePos(int(x/8), int(y/8))
	if pos >= 0 && int(pos) < len(w.model.Tiles) {
		if w.tryInsertPayloadIntoBuildingLocked(pos, &payload) {
			carrier.Payloads = append(carrier.Payloads[:0], carrier.Payloads[1:]...)
			w.model.EntitiesRev++
			return cloneRawEntity(*carrier), true
		}
	}
	dropped := false
	switch payload.Kind {
	case payloadKindUnit:
		dropped = w.dropUnitPayloadLocked(*carrier, &payload, x, y)
	case payloadKindBlock:
		if pos >= 0 && int(pos) < len(w.model.Tiles) {
			dropped = w.restoreDroppedBuildPayloadLocked(pos, *carrier, &payload)
		}
	}
	if !dropped {
		return RawEntity{}, false
	}
	carrier.Payloads = append(carrier.Payloads[:0], carrier.Payloads[1:]...)
	w.model.EntitiesRev++
	return cloneRawEntity(*carrier), true
}

func (w *World) EnterUnitPayload(buildPos, unitID int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.enterUnitPayloadLocked(buildPos, unitID)
}

func (w *World) EnterUnitPayloadPacked(buildPos, unitID int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	pos, ok := w.tileIndexFromPackedPosLocked(buildPos)
	if !ok {
		return false
	}
	return w.enterUnitPayloadLocked(pos, unitID)
}
