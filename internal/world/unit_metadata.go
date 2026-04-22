package world

import (
	"strings"

	"mdt-server/internal/protocol"
)

type unitAbilityKind string

const (
	unitAbilityEnergyField      unitAbilityKind = "EnergyFieldAbility"
	unitAbilityForceField       unitAbilityKind = "ForceFieldAbility"
	unitAbilityMoveEffect       unitAbilityKind = "MoveEffectAbility"
	unitAbilityRepairField      unitAbilityKind = "RepairFieldAbility"
	unitAbilityShieldArc        unitAbilityKind = "ShieldArcAbility"
	unitAbilityShieldRegenField unitAbilityKind = "ShieldRegenFieldAbility"
	unitAbilitySpawnDeath       unitAbilityKind = "SpawnDeathAbility"
	unitAbilityStatusField      unitAbilityKind = "StatusFieldAbility"
	unitAbilitySuppressionField unitAbilityKind = "SuppressionFieldAbility"
)

type unitAbilityProfile struct {
	Kind                  unitAbilityKind
	Amount                float32
	Max                   float32
	Reload                float32
	Range                 float32
	Radius                float32
	Regen                 float32
	Cooldown              float32
	Width                 float32
	Angle                 float32
	AngleOffset           float32
	X                     float32
	Y                     float32
	Damage                float32
	StatusID              int16
	StatusName            string
	StatusDuration        float32
	MaxTargets            int32
	HealPercent           float32
	SameTypeHealMult      float32
	ChanceDeflect         float32
	MissileUnitMultiplier float32
	SpawnAmount           int32
	SpawnRandAmount       int32
	Spread                float32
	TargetGround          bool
	TargetAir             bool
	HitBuildings          bool
	HitUnits              bool
	Active                bool
	WhenShooting          bool
	OnShoot               bool
	UseAmmo               bool
	PushUnits             bool
	FaceOutwards          bool
	SpawnUnitName         string
}

type unitRuntimeProfile struct {
	Name              string
	Health            float32
	Armor             float32
	Speed             float32
	HitSize           float32
	RotateSpeed       float32
	BuildSpeed        float32
	MineSpeed         float32
	MineTier          int16
	ItemCapacity      int32
	AmmoCapacity      float32
	AmmoPerShot       float32
	AmmoRegen         float32
	PayloadCapacity   float32
	Flying            bool
	LowAltitude       bool
	CanBoost          bool
	MineWalls         bool
	MineFloor         bool
	CoreUnitDock      bool
	AllowedInPayloads bool
	PickupUnits       bool
	Abilities         []unitAbilityProfile
}

func defaultAllowedInPayloadsByName(name string) bool {
	name = normalizeUnitName(name)
	switch name {
	case "manifold", "assembly-drone":
		return false
	}
	return !strings.Contains(name, "missile")
}

func defaultPickupUnitsByName(name string) bool {
	switch normalizeUnitName(name) {
	case "evoke", "incite", "emanate":
		return false
	default:
		return true
	}
}

type UnitMiningProfile struct {
	Speed     float32
	Tier      int
	Capacity  int32
	MineFloor bool
	MineWalls bool
}

func convertVanillaAbilityProfile(src vanillaUnitAbilityProfile) unitAbilityProfile {
	return unitAbilityProfile{
		Kind:                  unitAbilityKind(src.Type),
		Amount:                src.Amount,
		Max:                   src.Max,
		Reload:                src.Reload,
		Range:                 src.Range,
		Radius:                src.Radius,
		Regen:                 src.Regen,
		Cooldown:              src.Cooldown,
		Width:                 src.Width,
		Angle:                 src.Angle,
		AngleOffset:           src.AngleOffset,
		X:                     src.X,
		Y:                     src.Y,
		Damage:                src.Damage,
		StatusID:              src.StatusID,
		StatusName:            normalizeStatusKey(src.StatusName),
		StatusDuration:        src.StatusDuration,
		MaxTargets:            src.MaxTargets,
		HealPercent:           src.HealPercent,
		SameTypeHealMult:      src.SameTypeHealMult,
		ChanceDeflect:         src.ChanceDeflect,
		MissileUnitMultiplier: src.MissileUnitMultiplier,
		SpawnAmount:           src.SpawnAmount,
		SpawnRandAmount:       src.SpawnRandAmount,
		Spread:                src.Spread,
		TargetGround:          src.TargetGround,
		TargetAir:             src.TargetAir,
		HitBuildings:          src.HitBuildings,
		HitUnits:              src.HitUnits,
		Active:                src.Active,
		WhenShooting:          src.WhenShooting,
		OnShoot:               src.OnShoot,
		UseAmmo:               src.UseAmmo,
		PushUnits:             src.PushUnits,
		FaceOutwards:          src.FaceOutwards,
		SpawnUnitName:         normalizeUnitName(src.SpawnUnitName),
	}
}

func convertVanillaUnitRuntimeProfile(src vanillaUnitProfile) unitRuntimeProfile {
	name := normalizeUnitName(src.Name)
	out := unitRuntimeProfile{
		Name:              name,
		Health:            src.Health,
		Armor:             src.Armor,
		Speed:             src.Speed,
		HitSize:           src.HitSize,
		RotateSpeed:       src.RotateSpeed,
		BuildSpeed:        src.BuildSpeed,
		MineSpeed:         src.MineSpeed,
		MineTier:          src.MineTier,
		ItemCapacity:      src.ItemCapacity,
		AmmoCapacity:      src.AmmoCapacity,
		AmmoPerShot:       src.AmmoPerShot,
		AmmoRegen:         src.AmmoRegen,
		PayloadCapacity:   src.PayloadCapacity,
		Flying:            src.Flying,
		LowAltitude:       src.LowAltitude,
		CanBoost:          src.CanBoost,
		MineWalls:         src.MineWalls,
		MineFloor:         src.MineFloor,
		CoreUnitDock:      src.CoreUnitDock,
		AllowedInPayloads: defaultAllowedInPayloadsByName(name),
		PickupUnits:       defaultPickupUnitsByName(name),
	}
	if src.AllowedInPayloads {
		out.AllowedInPayloads = true
	}
	if src.PickupUnits {
		out.PickupUnits = true
	}
	if len(src.Abilities) > 0 {
		out.Abilities = make([]unitAbilityProfile, 0, len(src.Abilities))
		for _, ability := range src.Abilities {
			out.Abilities = append(out.Abilities, convertVanillaAbilityProfile(ability))
		}
	}
	return out
}

func clonePayloadData(src payloadData) payloadData {
	out := src
	out.Serialized = append([]byte(nil), src.Serialized...)
	out.Config = append([]byte(nil), src.Config...)
	out.Items = append([]ItemStack(nil), src.Items...)
	out.Liquids = append([]LiquidStack(nil), src.Liquids...)
	if src.UnitState != nil {
		unit := cloneRawEntity(*src.UnitState)
		out.UnitState = &unit
	}
	return out
}

func payloadDataToProtocolPayload(payload payloadData) (protocol.Payload, bool) {
	if len(payload.Serialized) > 0 {
		decoded, err := protocol.ReadPayload(protocol.NewReader(payload.Serialized), nil)
		if err == nil && decoded != nil {
			return decoded, true
		}
	}
	switch payload.Kind {
	case payloadKindBlock:
		return protocol.BuildPayload{
			BlockID: payload.BlockID,
			Version: 0,
		}, true
	case payloadKindUnit:
		if len(payload.Serialized) > 0 {
			raw := payload.Serialized
			if len(raw) > 1 && (raw[0] == 0 || raw[0] == 1) {
				raw = raw[1:]
			}
			return protocol.PayloadBox{Raw: append([]byte(nil), raw...)}, true
		}
	}
	return nil, false
}

func cloneUnitRuntimeProfile(src unitRuntimeProfile) unitRuntimeProfile {
	out := src
	if len(src.Abilities) > 0 {
		out.Abilities = append([]unitAbilityProfile(nil), src.Abilities...)
	}
	return out
}

func (w *World) unitRuntimeProfileByNameLocked(name string) (unitRuntimeProfile, bool) {
	if w == nil || w.unitRuntimeProfilesByName == nil {
		return unitRuntimeProfile{}, false
	}
	name = normalizeUnitName(name)
	if name == "" {
		return unitRuntimeProfile{}, false
	}
	prof, ok := w.unitRuntimeProfilesByName[name]
	if !ok {
		return unitRuntimeProfile{}, false
	}
	return cloneUnitRuntimeProfile(prof), true
}

func (w *World) unitRuntimeProfileForTypeLocked(typeID int16) (unitRuntimeProfile, bool) {
	if w == nil {
		return unitRuntimeProfile{}, false
	}
	name := ""
	if w.unitNamesByID != nil {
		name = w.unitNamesByID[typeID]
	}
	if name == "" {
		name = fallbackUnitNameByTypeID(typeID)
	}
	return w.unitRuntimeProfileByNameLocked(name)
}

func (w *World) unitRuntimeProfileForEntityLocked(e RawEntity) (unitRuntimeProfile, bool) {
	return w.unitRuntimeProfileForTypeLocked(e.TypeID)
}

func (w *World) UnitMiningProfile(typeID int16) (UnitMiningProfile, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	prof, ok := w.unitRuntimeProfileForTypeLocked(typeID)
	if !ok {
		return UnitMiningProfile{}, false
	}
	return UnitMiningProfile{
		Speed:     prof.MineSpeed,
		Tier:      int(prof.MineTier),
		Capacity:  prof.ItemCapacity,
		MineFloor: prof.MineFloor,
		MineWalls: prof.MineWalls,
	}, prof.MineSpeed > 0 && prof.ItemCapacity > 0 && prof.MineTier >= 0
}

func (w *World) applyUnitRuntimeProfile(e *RawEntity, prof unitRuntimeProfile) {
	if e == nil {
		return
	}
	if prof.Health > 0 {
		if e.RuntimeInit {
			e.Health = prof.Health
			e.MaxHealth = prof.Health
		} else {
			if e.MaxHealth <= 0 {
				e.MaxHealth = prof.Health
			}
			if e.Health <= 0 {
				e.Health = minf(e.MaxHealth, prof.Health)
			}
		}
	}
	if prof.Armor > 0 {
		e.Armor = prof.Armor
	}
	if prof.HitSize > 0 {
		e.HitRadius = prof.HitSize
	}
	if prof.Speed > 0 {
		e.MoveSpeed = prof.Speed
	}
	e.Flying = prof.Flying
	e.LowAltitude = prof.LowAltitude
	e.CanBoost = prof.CanBoost
	e.CoreUnitDock = prof.CoreUnitDock
	e.MineWalls = prof.MineWalls
	e.MineFloor = prof.MineFloor
	if prof.MineSpeed > 0 {
		e.MineSpeed = prof.MineSpeed
	}
	if prof.MineTier >= 0 {
		e.MineTier = prof.MineTier
	}
	if prof.BuildSpeed > 0 {
		e.BuildSpeed = prof.BuildSpeed
	}
	if prof.ItemCapacity > 0 {
		e.ItemCapacity = prof.ItemCapacity
	}
	if prof.PayloadCapacity > 0 {
		e.PayloadCapacity = prof.PayloadCapacity
	}
	if prof.AmmoCapacity > 0 {
		e.AmmoCapacity = prof.AmmoCapacity
		if e.Ammo <= 0 {
			e.Ammo = prof.AmmoCapacity
		}
	}
	if prof.AmmoPerShot > 0 {
		e.AmmoPerShot = prof.AmmoPerShot
	} else if prof.AmmoCapacity > 0 && e.AmmoPerShot <= 0 {
		e.AmmoPerShot = 1
	}
	if prof.AmmoRegen > 0 {
		e.AmmoRegen = prof.AmmoRegen
	}
	w.ensureEntityAbilityStates(e, prof)
}

func (w *World) ensureEntityAbilityStates(e *RawEntity, prof unitRuntimeProfile) {
	if e == nil {
		return
	}
	if len(prof.Abilities) == 0 {
		e.Abilities = nil
		return
	}
	priorAbilityStates := len(e.Abilities)
	priorShieldState := e.Shield != 0 || e.ShieldMax > 0 || e.ShieldRegen > 0
	if len(e.Abilities) != len(prof.Abilities) {
		states := make([]entityAbilityState, len(prof.Abilities))
		copy(states, e.Abilities)
		e.Abilities = states
	}
	for i, ability := range prof.Abilities {
		stateWasPresent := i < priorAbilityStates
		switch ability.Kind {
		case unitAbilityShieldArc:
			if ability.Max > 0 && !stateWasPresent && e.Abilities[i].Data == 0 {
				e.Abilities[i].Data = ability.Max
			}
		case unitAbilityForceField:
			if ability.Max > e.ShieldMax {
				e.ShieldMax = ability.Max
			}
			if ability.Regen > e.ShieldRegen {
				e.ShieldRegen = ability.Regen
			}
			if ability.Max > 0 && !priorShieldState {
				e.Shield = ability.Max
				priorShieldState = true
			}
		}
	}
}

func (w *World) visibleEntityAmmoLocked(e RawEntity) float32 {
	capacity := e.AmmoCapacity
	if capacity <= 0 {
		if prof, ok := w.unitRuntimeProfileForEntityLocked(e); ok {
			capacity = prof.AmmoCapacity
		}
	}
	if capacity <= 0 {
		return maxf(e.Ammo, 0)
	}
	if w != nil && w.rulesMgr != nil {
		if rules := w.rulesMgr.Get(); rules != nil && !rules.UnitAmmo {
			return capacity
		}
	}
	return clampf(maxf(e.Ammo, 0), 0, capacity)
}
