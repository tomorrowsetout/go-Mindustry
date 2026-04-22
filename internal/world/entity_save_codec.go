package world

import "mdt-server/internal/protocol"

func decodeRawUnitPayloadEntity(raw []byte, classID byte) (*RawEntity, bool) {
	if len(raw) == 0 {
		return nil, false
	}
	unit := &protocol.UnitEntitySync{
		ClassIDValue: classID,
		ClassIDSet:   true,
	}
	if err := unit.ReadEntity(protocol.NewReader(raw)); err != nil {
		return nil, false
	}
	entity := RawEntityFromUnitEntitySave(unit)
	entity.RuntimeInit = true
	return &entity, entity.TypeID > 0
}

// UnitEntitySyncFromRawEntitySave converts an in-memory unit entity into the
// save-compatible UnitEntitySync layout used by MSAV world-entity chunks.
func UnitEntitySyncFromRawEntitySave(e RawEntity, unitName string) *protocol.UnitEntitySync {
	if e.ID == 0 || e.TypeID <= 0 {
		return nil
	}
	unit := &protocol.UnitEntitySync{
		IDValue:        e.ID,
		Ammo:           maxf(e.Ammo, 0),
		BaseRotation:   e.Rotation,
		Controller:     rawEntityControllerState(e),
		Elevation:      e.Elevation,
		Flag:           e.Flag,
		Health:         maxf(e.Health, 0),
		Shooting:       e.Shooting,
		Lifetime:       e.LifeSec,
		Rotation:       e.Rotation,
		Shield:         e.Shield,
		SpawnedByCore:  e.SpawnedByCore,
		Statuses:       rawEntityStatusesToProtocol(e.Statuses),
		Abilities:      rawEntityAbilitiesToProtocol(e.Abilities),
		Plans:          rawEntityPlansToProtocol(e.Plans),
		TeamID:         byte(e.Team),
		Time:           e.AgeSec,
		TypeID:         e.TypeID,
		UpdateBuilding: e.UpdateBuilding,
		Vel:            protocol.Vec2{X: e.VelX, Y: e.VelY},
		X:              e.X,
		Y:              e.Y,
	}
	if e.MineTilePos >= 0 {
		unit.MineTile = protocol.TileBox{PosValue: e.MineTilePos}
	}
	if e.Stack.Amount > 0 {
		unit.Stack = protocol.ItemStack{
			Item:   protocol.ItemRef{ItmID: int16(e.Stack.Item)},
			Amount: e.Stack.Amount,
		}
	}
	unit.Payloads = rawEntityPayloadsToProtocolSave(e)
	unit.ApplyLayoutByName(unitName)
	return unit
}

// RawEntityFromUnitEntitySave converts a save-decoded unit entity into the
// in-memory RawEntity representation used by the runtime world model.
func RawEntityFromUnitEntitySave(unit *protocol.UnitEntitySync) RawEntity {
	if unit == nil {
		return RawEntity{}
	}
	out := RawEntity{
		TypeID:         unit.TypeID,
		ID:             unit.ID(),
		X:              unit.X,
		Y:              unit.Y,
		Rotation:       unit.Rotation,
		VelX:           unit.Vel.X,
		VelY:           unit.Vel.Y,
		LifeSec:        unit.Lifetime,
		AgeSec:         unit.Time,
		Health:         unit.Health,
		Shield:         unit.Shield,
		Ammo:           unit.Ammo,
		Elevation:      unit.Elevation,
		Shooting:       unit.Shooting,
		Flag:           unit.Flag,
		Team:           TeamID(unit.TeamID),
		SpawnedByCore:  unit.SpawnedByCore,
		UpdateBuilding: unit.UpdateBuilding,
		SlowMul:        1,
		MineTilePos:    invalidEntityTilePos,
	}
	if unit.MineTile != nil {
		out.MineTilePos = unit.MineTile.Pos()
	}
	if unit.Stack.Amount > 0 {
		if unit.Stack.Item != nil {
			out.Stack.Item = ItemID(unit.Stack.Item.ID())
		}
		out.Stack.Amount = unit.Stack.Amount
	}
	if state, ok := unit.Controller.(*protocol.ControllerState); ok && state != nil {
		switch state.Type {
		case protocol.ControllerPlayer:
			out.PlayerID = state.PlayerID
		case protocol.ControllerCommand4, protocol.ControllerCommand6, protocol.ControllerCommand7, protocol.ControllerCommand8, protocol.ControllerCommand9:
			out.CommandID = int16(state.Command.CommandID)
			if state.Command.HasPos {
				out.Behavior = "move"
				out.PatrolAX = state.Command.TargetPos.X
				out.PatrolAY = state.Command.TargetPos.Y
			} else if state.Command.HasAttack {
				out.Behavior = "follow"
				out.TargetID = state.Command.Target.Pos
			}
		}
	}
	if len(unit.Statuses) > 0 {
		out.Statuses = make([]entityStatusState, 0, len(unit.Statuses))
		for _, status := range unit.Statuses {
			id := int16(0)
			name := ""
			if status.Effect != nil {
				id = status.Effect.ID()
				name = status.Effect.Name()
			}
			out.Statuses = append(out.Statuses, entityStatusState{
				ID:   id,
				Name: name,
				Time: status.Time,
			})
		}
	}
	if len(unit.Abilities) > 0 {
		out.Abilities = make([]entityAbilityState, 0, len(unit.Abilities))
		for _, ability := range unit.Abilities {
			if ability == nil {
				continue
			}
			out.Abilities = append(out.Abilities, entityAbilityState{Data: ability.Data()})
		}
	}
	if len(unit.Plans) > 0 {
		out.Plans = make([]entityBuildPlan, 0, len(unit.Plans))
		for _, plan := range unit.Plans {
			if plan == nil {
				continue
			}
			entry := entityBuildPlan{
				Breaking: plan.Breaking,
				Pos:      protocol.PackPoint2(plan.X, plan.Y),
				Rotation: plan.Rotation,
				Config:   plan.Config,
			}
			if !plan.Breaking && plan.Block != nil {
				entry.BlockID = plan.Block.ID()
			}
			out.Plans = append(out.Plans, entry)
		}
	}
	if len(unit.Payloads) > 0 {
		out.Payloads = make([]payloadData, 0, len(unit.Payloads))
		for _, payload := range unit.Payloads {
			if payload == nil {
				continue
			}
			writer := protocol.NewWriter()
			if err := protocol.WritePayload(writer, payload); err != nil {
				continue
			}
			raw := append([]byte(nil), writer.Bytes()...)
			decoded, ok := decodePayloadData(raw)
			if !ok || decoded == nil {
				continue
			}
			decoded.Serialized = raw
			out.Payloads = append(out.Payloads, *decoded)
		}
	}
	return out
}

func rawEntityControllerState(e RawEntity) protocol.UnitController {
	if e.PlayerID != 0 {
		return &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: e.PlayerID}
	}
	if e.Behavior == "" && e.CommandID == 0 {
		return &protocol.ControllerState{Type: protocol.ControllerGenericAI}
	}
	state := &protocol.ControllerState{Type: protocol.ControllerCommand9}
	state.Command.CommandID = int8(e.CommandID)
	switch e.Behavior {
	case "move", "patrol":
		state.Command.HasPos = true
		state.Command.TargetPos = protocol.Vec2{X: e.PatrolAX, Y: e.PatrolAY}
	case "follow":
		if e.TargetID != 0 {
			state.Command.HasAttack = true
			state.Command.Target = protocol.CommandTarget{Type: 0, Pos: e.TargetID}
		}
	}
	return state
}

func rawEntityPlansToProtocol(plans []entityBuildPlan) []*protocol.BuildPlan {
	if len(plans) == 0 {
		return []*protocol.BuildPlan{}
	}
	out := make([]*protocol.BuildPlan, 0, len(plans))
	for _, plan := range plans {
		pos := protocol.UnpackPoint2(plan.Pos)
		entry := &protocol.BuildPlan{
			Breaking: plan.Breaking,
			X:        pos.X,
			Y:        pos.Y,
			Rotation: plan.Rotation,
			Config:   plan.Config,
		}
		if !plan.Breaking && plan.BlockID > 0 {
			entry.Block = protocol.BlockRef{BlkID: plan.BlockID}
		}
		out = append(out, entry)
	}
	return out
}

func rawEntityStatusesToProtocol(statuses []entityStatusState) []protocol.StatusEntry {
	if len(statuses) == 0 {
		return []protocol.StatusEntry{}
	}
	out := make([]protocol.StatusEntry, 0, len(statuses))
	for _, status := range statuses {
		out = append(out, protocol.StatusEntry{
			Effect: entitySyncStatusEffect{id: status.ID, name: status.Name},
			Time:   status.Time,
		})
	}
	return out
}

func rawEntityAbilitiesToProtocol(abilities []entityAbilityState) []protocol.Ability {
	if len(abilities) == 0 {
		return []protocol.Ability{}
	}
	out := make([]protocol.Ability, 0, len(abilities))
	for i := range abilities {
		out = append(out, &entitySyncAbility{data: abilities[i].Data})
	}
	return out
}

func rawEntityPayloadsToProtocolSave(e RawEntity) []protocol.Payload {
	if len(e.Payloads) > 0 {
		out := make([]protocol.Payload, 0, len(e.Payloads))
		for i := range e.Payloads {
			if payload, ok := payloadDataToProtocolPayload(clonePayloadData(e.Payloads[i])); ok {
				out = append(out, payload)
			}
		}
		return out
	}
	if len(e.Payload) == 0 {
		return []protocol.Payload{}
	}
	return []protocol.Payload{protocol.PayloadBox{Raw: append([]byte(nil), e.Payload...)}}
}
