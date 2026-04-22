package world

import (
	"sort"
	"strings"

	"mdt-server/internal/protocol"
)

type entitySyncStatusEffect struct {
	id   int16
	name string
}

func (s entitySyncStatusEffect) ContentType() protocol.ContentType { return protocol.ContentStatus }
func (s entitySyncStatusEffect) ID() int16                         { return s.id }
func (s entitySyncStatusEffect) Name() string                      { return s.name }
func (s entitySyncStatusEffect) Dynamic() bool                     { return false }

type entitySyncAbility struct {
	data float32
}

func (a *entitySyncAbility) Data() float32 {
	if a == nil {
		return 0
	}
	return a.data
}

func (a *entitySyncAbility) SetData(v float32) {
	if a == nil {
		return
	}
	a.data = v
}

// EntitySyncSnapshots builds vanilla-shaped unit sync entities for world units.
// Active player-bound units are skipped through the supplied playerUnits set so
// they can be serialized once through the dedicated player snapshot path.
func (w *World) EntitySyncSnapshots(content *protocol.ContentRegistry, playerUnits map[int32]struct{}) []protocol.UnitSyncEntity {
	if w == nil {
		return nil
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Entities) == 0 {
		return nil
	}

	entities := make([]RawEntity, 0, len(w.model.Entities))
	for _, src := range w.model.Entities {
		if src.ID == 0 || src.TypeID <= 0 || src.Health <= 0 {
			continue
		}
		if _, skip := playerUnits[src.ID]; skip {
			continue
		}
		if content != nil && content.UnitType(src.TypeID) == nil {
			continue
		}
		entities = append(entities, cloneRawEntity(src))
	}
	sort.Slice(entities, func(i, j int) bool {
		return entities[i].ID < entities[j].ID
	})

	out := make([]protocol.UnitSyncEntity, 0, len(entities))
	for _, e := range entities {
		out = append(out, w.entitySyncUnitLocked(e, content, nil))
	}
	return out
}

func (w *World) UnitSyncSnapshot(content *protocol.ContentRegistry, unitID int32, controller protocol.UnitController) (*protocol.UnitEntitySync, bool) {
	if w == nil || unitID == 0 {
		return nil, false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil {
		return nil, false
	}
	for _, src := range w.model.Entities {
		if src.ID != unitID || src.TypeID <= 0 || src.Health <= 0 {
			continue
		}
		if content != nil && content.UnitType(src.TypeID) == nil {
			return nil, false
		}
		return w.entitySyncUnitLocked(cloneRawEntity(src), content, controller), true
	}
	return nil, false
}

func (w *World) entitySyncUnitLocked(e RawEntity, content *protocol.ContentRegistry, controller protocol.UnitController) *protocol.UnitEntitySync {
	mounts := w.entitySyncMountsLocked(e)
	statuses := w.entitySyncStatusesLocked(e, content)
	abilities := w.entitySyncAbilitiesLocked(e)
	plans := w.entitySyncPlansLocked(e, content)
	payloads := w.entitySyncPayloadsLocked(e)
	if !e.RuntimeInit && e.PlayerID == 0 {
		// Save-loaded map entities can carry legacy payload/plan blobs that we do
		// not yet round-trip byte-perfectly for official 157 clients. Keep the
		// entity visible, but strip the unstable variable-length tails so one bad
		// map unit cannot corrupt the whole entitySnapshot packet on join.
		plans = []*protocol.BuildPlan{}
		payloads = []protocol.Payload{}
	}
	stack := w.entitySyncStackLocked(e, content)
	shooting := false
	for _, mount := range mounts {
		if mount != nil && mount.Shoot() {
			shooting = true
			break
		}
	}
	shooting = shooting || e.Shooting

	elevation := e.Elevation
	if isEntityFlying(e) {
		elevation = 1
	}

	if controller == nil {
		controller = w.entitySyncControllerLocked(e)
	}

	unit := &protocol.UnitEntitySync{
		IDValue:        e.ID,
		Abilities:      abilities,
		Ammo:           w.visibleEntityAmmoLocked(e),
		BaseRotation:   e.Rotation,
		Controller:     controller,
		Elevation:      elevation,
		Flag:           0,
		Health:         maxf(e.Health, 0),
		Shooting:       shooting,
		MineTile:       w.entitySyncMineTileLocked(e),
		Mounts:         mounts,
		Payloads:       payloads,
		Plans:          plans,
		Rotation:       e.Rotation,
		Shield:         e.Shield,
		SpawnedByCore:  e.SpawnedByCore,
		Stack:          stack,
		Statuses:       statuses,
		TeamID:         byte(e.Team),
		TypeID:         e.TypeID,
		UpdateBuilding: e.UpdateBuilding,
		Vel:            protocol.Vec2{X: e.VelX, Y: e.VelY},
		X:              e.X,
		Y:              e.Y,
	}
	if content != nil {
		if typ := content.UnitType(e.TypeID); typ != nil {
			unit.ApplyLayoutByName(typ.Name())
			return unit
		}
	}
	if name, ok := w.unitNamesByID[e.TypeID]; ok {
		unit.ApplyLayoutByName(name)
	}
	return unit
}

func (w *World) entitySyncControllerLocked(e RawEntity) protocol.UnitController {
	if e.PlayerID != 0 {
		return &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: e.PlayerID}
	}
	if e.Behavior == "" && e.CommandID == 0 && !w.entityUsesCommandControllerLocked(e) {
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

func (w *World) entityUsesCommandControllerLocked(e RawEntity) bool {
	if w == nil || e.Team == 0 || e.TypeID <= 0 {
		return false
	}
	name := ""
	if w.unitNamesByID != nil {
		name = w.unitNamesByID[e.TypeID]
	}
	if strings.TrimSpace(name) == "" {
		name = fallbackUnitNameByTypeID(e.TypeID)
	}
	name = normalizeUnitName(name)
	if !unitUsesCommandControllerByName(name) {
		return false
	}
	if !w.teamUsesCommandControllerLocked(e.Team) {
		return false
	}
	return true
}

func unitUsesCommandControllerByName(name string) bool {
	name = normalizeUnitName(name)
	switch name {
	case "", "block", "manifold", "assemblydrone":
		return false
	}
	return !strings.Contains(name, "missile")
}

func (w *World) teamUsesCommandControllerLocked(team TeamID) bool {
	if w == nil || team == 0 {
		return false
	}
	if w.rulesMgr == nil {
		return true
	}
	rules := w.rulesMgr.Get()
	if rules == nil {
		return true
	}
	if !rules.RtsAi && w.isWaveTeamLocked(team) {
		return false
	}
	return true
}

func (w *World) entitySyncMineTileLocked(e RawEntity) protocol.Tile {
	if e.MineTilePos < 0 {
		return nil
	}
	return protocol.TileBox{PosValue: e.MineTilePos}
}

func (w *World) entitySyncStackLocked(e RawEntity, content *protocol.ContentRegistry) protocol.ItemStack {
	if e.Stack.Amount <= 0 {
		return protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}, Amount: 0}
	}
	itemID := int16(e.Stack.Item)
	if content != nil {
		if item := content.Item(itemID); item != nil {
			return protocol.ItemStack{Item: item, Amount: e.Stack.Amount}
		}
	}
	return protocol.ItemStack{
		Item:   protocol.ItemRef{ItmID: itemID, ItmName: ""},
		Amount: e.Stack.Amount,
	}
}

func (w *World) entitySyncPlansLocked(e RawEntity, content *protocol.ContentRegistry) []*protocol.BuildPlan {
	if len(e.Plans) == 0 {
		return []*protocol.BuildPlan{}
	}
	out := make([]*protocol.BuildPlan, 0, len(e.Plans))
	for _, plan := range e.Plans {
		pos := protocol.UnpackPoint2(plan.Pos)
		entry := &protocol.BuildPlan{
			Breaking: plan.Breaking,
			X:        pos.X,
			Y:        pos.Y,
			Rotation: plan.Rotation,
			Config:   plan.Config,
		}
		if !plan.Breaking && plan.BlockID > 0 {
			if content != nil {
				entry.Block = content.Block(plan.BlockID)
			}
			if entry.Block == nil {
				entry.Block = protocol.BlockRef{
					BlkID:   plan.BlockID,
					BlkName: w.blockNamesByID[plan.BlockID],
				}
			}
		}
		out = append(out, entry)
	}
	return out
}

func (w *World) entitySyncAbilitiesLocked(e RawEntity) []protocol.Ability {
	if len(e.Abilities) == 0 {
		return []protocol.Ability{}
	}
	out := make([]protocol.Ability, 0, len(e.Abilities))
	for _, ability := range e.Abilities {
		out = append(out, &entitySyncAbility{data: ability.Data})
	}
	return out
}

func (w *World) entitySyncPayloadsLocked(e RawEntity) []protocol.Payload {
	if len(e.Payloads) > 0 {
		out := make([]protocol.Payload, 0, len(e.Payloads))
		for i := range e.Payloads {
			if payload, ok := payloadDataToProtocolPayload(clonePayloadData(e.Payloads[i])); ok {
				out = append(out, payload)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	if len(e.Payload) == 0 {
		return []protocol.Payload{}
	}
	if payload, ok := payloadDataToProtocolPayload(payloadData{
		Serialized: append([]byte(nil), e.Payload...),
	}); ok {
		return []protocol.Payload{payload}
	}
	return []protocol.Payload{
		protocol.PayloadBox{Raw: append([]byte(nil), e.Payload...)},
	}
}

func (w *World) entitySyncMountsLocked(e RawEntity) []protocol.WeaponMount {
	profiles := w.unitMountProfilesForEntity(e)
	states := w.unitMountStates[e.ID]
	count := len(profiles)
	if len(states) > count {
		count = len(states)
	}
	if count == 0 {
		return []protocol.WeaponMount{}
	}

	out := make([]protocol.WeaponMount, 0, count)
	for i := 0; i < count; i++ {
		state := unitMountState{}
		if i < len(states) {
			state = states[i]
		}

		shooting := state.BeamBulletID != 0 || state.Warmup > 0.01
		rotating := false
		if i < len(profiles) && profiles[i].Rotate {
			rotating = state.TargetID != 0 || state.TargetBuildPos >= 0 || angleDistDeg(state.Rotation, state.TargetRotation) > 0.1
		}

		out = append(out, &protocol.BasicWeaponMount{
			AimPosX:  state.AimX,
			AimPosY:  state.AimY,
			Shooting: shooting,
			Rotating: rotating,
		})
	}
	return out
}

func (w *World) entitySyncStatusesLocked(e RawEntity, content *protocol.ContentRegistry) []protocol.StatusEntry {
	if len(e.Statuses) == 0 {
		return []protocol.StatusEntry{}
	}
	out := make([]protocol.StatusEntry, 0, len(e.Statuses))
	for _, st := range e.Statuses {
		eff := entitySyncStatusEffectFor(st, content)
		if eff == nil {
			continue
		}
		out = append(out, protocol.StatusEntry{
			Effect: eff,
			Time:   maxf(st.Time, 0),
		})
	}
	return out
}

func entitySyncStatusEffectFor(st entityStatusState, content *protocol.ContentRegistry) protocol.StatusEffect {
	if content != nil {
		if eff := content.StatusEffect(st.ID); eff != nil {
			return eff
		}
	}
	name := strings.TrimSpace(st.Name)
	if st.ID <= 0 && name == "" {
		return nil
	}
	return entitySyncStatusEffect{id: st.ID, name: name}
}
