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

// EntitySyncSnapshots builds vanilla-shaped unit sync entities for world units
// that are not directly controlled by a player connection.
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
		if src.PlayerID != 0 {
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
		out = append(out, w.entitySyncUnitLocked(e, content))
	}
	return out
}

func (w *World) entitySyncUnitLocked(e RawEntity, content *protocol.ContentRegistry) *protocol.UnitEntitySync {
	mounts := w.entitySyncMountsLocked(e)
	statuses := w.entitySyncStatusesLocked(e, content)
	shooting := false
	for _, mount := range mounts {
		if mount != nil && mount.Shoot() {
			shooting = true
			break
		}
	}

	elevation := float32(0)
	if isEntityFlying(e) {
		elevation = 1
	}

	unit := &protocol.UnitEntitySync{
		IDValue:        e.ID,
		Abilities:      []protocol.Ability{},
		Ammo:           0,
		BaseRotation:   e.Rotation,
		Controller:     &protocol.ControllerState{Type: protocol.ControllerGenericAI},
		Elevation:      elevation,
		Flag:           0,
		Health:         maxf(e.Health, 0),
		Shooting:       shooting,
		MineTile:       nil,
		Mounts:         mounts,
		Plans:          []*protocol.BuildPlan{},
		Rotation:       e.Rotation,
		Shield:         maxf(e.Shield, 0),
		SpawnedByCore:  false,
		Stack:          protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}, Amount: 0},
		Statuses:       statuses,
		TeamID:         byte(e.Team),
		TypeID:         e.TypeID,
		UpdateBuilding: false,
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
			rotating = state.TargetID != 0 || state.TargetBuildPos != 0 || angleDistDeg(state.Rotation, state.TargetRotation) > 0.1
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
