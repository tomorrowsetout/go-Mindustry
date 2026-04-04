package world

import (
	"math/rand"
	"strings"
)

const minArmorDamageFraction = 0.1

type bulletRuntimeProfile struct {
	ClassName           string
	Damage              float32
	SplashDamage        float32
	BulletType          int16
	Speed               float32
	Lifetime            float32
	HitSize             float32
	SplashRadius        float32
	BuildingDamage      float32
	Pierce              int32
	PierceBuilding      bool
	StatusID            int16
	StatusName          string
	StatusDuration      float32
	HitBuildings        bool
	TargetAir           bool
	TargetGround        bool
	ShootEffect         string
	SmokeEffect         string
	HitEffect           string
	DespawnEffect       string
	Length              float32
	DamageInterval      float32
	OptimalLifeFract    float32
	FadeTime            float32
	FragmentCount       int32
	FragmentSpread      float32
	FragmentRandom      float32
	FragmentAngle       float32
	FragmentVelocityMin float32
	FragmentVelocityMax float32
	FragmentLifeMin     float32
	FragmentLifeMax     float32
	FragmentBullet      *bulletRuntimeProfile
}

type entityStatusState struct {
	ID         int16
	Name       string
	Time       float32
	DamageTime float32
}

type statusEffectProfile struct {
	ID                   int16
	Name                 string
	DamageMultiplier     float32
	HealthMultiplier     float32
	SpeedMultiplier      float32
	ReloadMultiplier     float32
	BuildSpeedMultiplier float32
	DragMultiplier       float32
	TransitionDamage     float32
	Damage               float32
	IntervalDamageTime   float32
	IntervalDamage       float32
	IntervalDamagePierce bool
	Disarm               bool
	Permanent            bool
	Reactive             bool
	Dynamic              bool
	Opposites            []string
	Affinities           []string
}

func cloneBulletRuntimeProfile(src *bulletRuntimeProfile) *bulletRuntimeProfile {
	if src == nil {
		return nil
	}
	copy := *src
	copy.FragmentBullet = cloneBulletRuntimeProfile(src.FragmentBullet)
	return &copy
}

func cloneRawEntity(src RawEntity) RawEntity {
	copy := src
	if len(src.Statuses) > 0 {
		copy.Statuses = append([]entityStatusState(nil), src.Statuses...)
	}
	copy.AttackFragmentBullet = cloneBulletRuntimeProfile(src.AttackFragmentBullet)
	return copy
}

func convertVanillaBulletProfile(src *vanillaBulletProfile) *bulletRuntimeProfile {
	if src == nil {
		return nil
	}
	return &bulletRuntimeProfile{
		ClassName:           src.ClassName,
		Damage:              src.Damage,
		SplashDamage:        src.SplashDamage,
		BulletType:          src.BulletType,
		Speed:               src.Speed,
		Lifetime:            src.Lifetime,
		HitSize:             src.HitSize,
		SplashRadius:        src.SplashRadius,
		BuildingDamage:      src.BuildingDamageMultiplier,
		Pierce:              src.Pierce,
		PierceBuilding:      src.PierceBuilding,
		StatusID:            src.StatusID,
		StatusName:          normalizeStatusKey(src.StatusName),
		StatusDuration:      src.StatusDuration,
		HitBuildings:        src.HitBuildings,
		TargetAir:           src.TargetAir,
		TargetGround:        src.TargetGround,
		ShootEffect:         src.ShootEffect,
		SmokeEffect:         src.SmokeEffect,
		HitEffect:           src.HitEffect,
		DespawnEffect:       src.DespawnEffect,
		Length:              src.Length,
		DamageInterval:      src.DamageInterval,
		OptimalLifeFract:    src.OptimalLifeFract,
		FadeTime:            src.FadeTime,
		FragmentCount:       src.FragBullets,
		FragmentSpread:      src.FragSpread,
		FragmentRandom:      src.FragRandomSpread,
		FragmentAngle:       src.FragAngle,
		FragmentVelocityMin: src.FragVelocityMin,
		FragmentVelocityMax: src.FragVelocityMax,
		FragmentLifeMin:     src.FragLifeMin,
		FragmentLifeMax:     src.FragLifeMax,
		FragmentBullet:      convertVanillaBulletProfile(src.FragBullet),
	}
}

func applyWeaponProfileToEntity(e *RawEntity, p weaponProfile) {
	if e == nil {
		return
	}
	e.AttackRange = p.Range
	e.AttackFireMode = p.FireMode
	e.AttackDamage = p.Damage
	e.AttackSplashDamage = p.SplashDamage
	e.AttackInterval = p.Interval
	e.AttackBulletType = p.BulletType
	e.AttackBulletSpeed = p.BulletSpeed
	e.AttackBulletLifetime = p.BulletLifetime
	e.AttackBulletHitSize = p.BulletHitSize
	e.AttackSplashRadius = p.SplashRadius
	e.AttackBuildingDamage = p.BuildingDamage
	e.AttackBuildingDamageSet = true
	e.AttackSlowSec = p.SlowSec
	e.AttackSlowMul = p.SlowMul
	e.AttackPierce = p.Pierce
	e.AttackPierceBuilding = p.PierceBuilding
	e.AttackChainCount = p.ChainCount
	e.AttackChainRange = p.ChainRange
	e.AttackStatusID = p.StatusID
	e.AttackStatusName = normalizeStatusKey(p.StatusName)
	e.AttackStatusDuration = p.StatusDuration
	e.AttackShootStatusID = p.ShootStatusID
	e.AttackShootStatusName = normalizeStatusKey(p.ShootStatusName)
	e.AttackShootStatusDur = p.ShootStatusDuration
	e.AttackFragmentCount = p.FragmentCount
	e.AttackFragmentSpread = p.FragmentSpread
	e.AttackFragmentSpeed = p.FragmentSpeed
	e.AttackFragmentLife = p.FragmentLife
	e.AttackFragmentRand = p.FragmentRandomSpread
	e.AttackFragmentAngle = p.FragmentAngle
	e.AttackFragmentVelMin = p.FragmentVelocityMin
	e.AttackFragmentVelMax = p.FragmentVelocityMax
	e.AttackFragmentLifeMin = p.FragmentLifeMin
	e.AttackFragmentLifeMax = p.FragmentLifeMax
	e.AttackFragmentBullet = cloneBulletRuntimeProfile(p.FragmentBullet)
	e.AttackShootEffect = p.ShootEffect
	e.AttackSmokeEffect = p.SmokeEffect
	e.AttackHitEffect = p.HitEffect
	e.AttackDespawnEffect = p.DespawnEffect
	e.AttackPreferBuildings = p.PreferBuildings
	e.AttackTargetAir = p.TargetAir
	e.AttackTargetGround = p.TargetGround
	e.AttackTargetPriority = p.TargetPriority
	e.AttackBuildings = p.HitBuildings
}

func normalizeStatusKey(name string) string {
	return strings.ToLower(strings.TrimSpace(name))
}

func (w *World) resolveStatusProfile(id int16, name string) (statusEffectProfile, bool) {
	if w == nil {
		return statusEffectProfile{}, false
	}
	if id >= 0 && w.statusProfilesByID != nil {
		if prof, ok := w.statusProfilesByID[id]; ok {
			return prof, true
		}
	}
	name = normalizeStatusKey(name)
	if name == "" || w.statusProfilesByName == nil {
		return statusEffectProfile{}, false
	}
	prof, ok := w.statusProfilesByName[name]
	return prof, ok
}

func (w *World) updateEntityStatuses(e *RawEntity, dt float32) {
	if e == nil {
		return
	}
	e.StatusDamageMul = 1
	e.StatusHealthMul = 1
	e.StatusSpeedMul = 1
	e.StatusReloadMul = 1
	e.StatusBuildSpeedMul = 1
	e.StatusDragMul = 1
	e.StatusArmorOverride = -1
	e.Disarmed = false
	if len(e.Statuses) == 0 {
		return
	}

	active := e.Statuses[:0]
	for _, st := range e.Statuses {
		prof, ok := w.resolveStatusProfile(st.ID, st.Name)
		if !ok {
			continue
		}
		if !prof.Permanent {
			st.Time -= dt
			if st.Time <= 0 {
				continue
			}
		}
		st.ID = prof.ID
		st.Name = prof.Name
		active = append(active, st)
		e.StatusDamageMul *= safeMul(prof.DamageMultiplier)
		e.StatusHealthMul *= safeMul(prof.HealthMultiplier)
		e.StatusSpeedMul *= safeMul(prof.SpeedMultiplier)
		e.StatusReloadMul *= safeMul(prof.ReloadMultiplier)
		e.StatusBuildSpeedMul *= safeMul(prof.BuildSpeedMultiplier)
		e.StatusDragMul *= safeMul(prof.DragMultiplier)
		e.Disarmed = e.Disarmed || prof.Disarm
	}
	e.Statuses = active

	for i := range e.Statuses {
		prof, ok := w.resolveStatusProfile(e.Statuses[i].ID, e.Statuses[i].Name)
		if !ok {
			continue
		}
		if prof.Damage > 0 {
			w.applyDamageToEntityDetailed(e, prof.Damage*dt, true)
		} else if prof.Damage < 0 {
			e.Health = minf(e.MaxHealth, e.Health+(-prof.Damage)*dt)
		}
		if prof.IntervalDamageTime > 0 {
			e.Statuses[i].DamageTime += dt
			for e.Statuses[i].DamageTime >= prof.IntervalDamageTime {
				e.Statuses[i].DamageTime -= prof.IntervalDamageTime
				if prof.IntervalDamage > 0 {
					w.applyDamageToEntityDetailed(e, prof.IntervalDamage, prof.IntervalDamagePierce)
				} else if prof.IntervalDamage < 0 {
					e.Health = minf(e.MaxHealth, e.Health-prof.IntervalDamage)
				}
			}
		}
	}
}

func safeMul(v float32) float32 {
	if v == 0 {
		return 1
	}
	return v
}

func entityDamageMultiplier(e RawEntity) float32 {
	if e.StatusDamageMul <= 0 {
		return 1
	}
	return e.StatusDamageMul
}

func entityBuildingDamageMultiplier(e RawEntity) float32 {
	if e.AttackBuildingDamageSet {
		return e.AttackBuildingDamage
	}
	if e.AttackBuildingDamage != 0 {
		return e.AttackBuildingDamage
	}
	return 1
}

func entityHealthMultiplier(e RawEntity) float32 {
	if e.StatusHealthMul <= 0 {
		return 1
	}
	return e.StatusHealthMul
}

func entityReloadMultiplier(e RawEntity) float32 {
	if e.StatusReloadMul <= 0 {
		return 1
	}
	return e.StatusReloadMul
}

func entitySpeedMultiplier(e RawEntity) float32 {
	speedMul := float32(1)
	if e.StatusSpeedMul > 0 {
		speedMul *= e.StatusSpeedMul
	}
	if e.SlowMul > 0 {
		speedMul *= clampf(e.SlowMul, 0.2, 1)
	}
	return speedMul
}

func entityArmorValue(e RawEntity) float32 {
	if e.StatusArmorOverride >= 0 {
		return e.StatusArmorOverride
	}
	if e.Armor > 0 {
		return e.Armor
	}
	return 0
}

func attackCooldownScale(e RawEntity) float32 {
	scale := entityReloadMultiplier(e)
	if e.SlowMul > 0 {
		scale *= clampf(e.SlowMul, 0.2, 1)
	}
	return scale
}

func (w *World) outgoingDamageScale(e RawEntity, sourceIsBuilding bool) float32 {
	scale := entityDamageMultiplier(e)
	if w == nil || w.rulesMgr == nil {
		return scale
	}
	rules := w.rulesMgr.Get()
	if rules == nil {
		return scale
	}
	if sourceIsBuilding {
		scale *= rules.BlockDamageMultiplier
	} else {
		scale *= rules.UnitDamageMultiplier
	}
	return scale
}

func (w *World) applyDamageToBuildingDetailed(pos int32, damage float32) bool {
	if w == nil || w.model == nil || damage <= 0 {
		return false
	}
	if w.rulesMgr != nil {
		if rules := w.rulesMgr.Get(); rules != nil && rules.BlockHealthMultiplier > 0 {
			damage /= rules.BlockHealthMultiplier
		}
	}
	return w.applyDamageToBuildingRaw(pos, damage)
}

func randomRange(minV, maxV float32) float32 {
	switch {
	case minV <= 0 && maxV <= 0:
		return 0
	case minV <= 0 && maxV > 0:
		return maxV
	case maxV <= 0:
		return minV
	case maxV < minV:
		return minV
	case maxV == minV:
		return minV
	default:
		return minV + (maxV-minV)*randFloat32()
	}
}

func randFloat32() float32 {
	return rand.Float32()
}

func canEntityAttack(e RawEntity) bool {
	if e.Health <= 0 || e.Disarmed {
		return false
	}
	return e.AttackDamage > 0 || e.AttackSplashDamage > 0 || e.AttackStatusID != 0 || normalizeStatusKey(e.AttackStatusName) != ""
}

func (w *World) applyShootStatus(e *RawEntity) {
	if e == nil {
		return
	}
	w.applyStatusToEntity(e, e.AttackShootStatusID, e.AttackShootStatusName, e.AttackShootStatusDur)
}

func (w *World) applyAttackStatus(target *RawEntity, src RawEntity) {
	if target == nil {
		return
	}
	applySlow(target, src.AttackSlowSec, src.AttackSlowMul)
	w.applyStatusToEntity(target, src.AttackStatusID, src.AttackStatusName, src.AttackStatusDuration)
}

func (w *World) fireBeamAtEntity(src RawEntity, target *RawEntity, targetIdx int, sourceIsBuilding bool) {
	if target == nil {
		return
	}
	scale := w.outgoingDamageScale(src, sourceIsBuilding)
	w.emitAttackFireEffectsLocked(src)
	w.applyDamageToEntityDetailed(target, src.AttackDamage*scale, false)
	w.applyAttackStatus(target, src)
	w.emitAttackHitEffectLocked(src, target.X, target.Y)
	w.applyBeamChainFromSource(src, targetIdx, sourceIsBuilding)
}

func (w *World) fireBeamAtBuilding(src RawEntity, pos int32, tx, ty float32, sourceIsBuilding bool) {
	scale := w.outgoingDamageScale(src, sourceIsBuilding)
	buildingMul := entityBuildingDamageMultiplier(src)
	w.emitAttackFireEffectsLocked(src)
	_ = w.applyDamageToBuildingDetailed(pos, src.AttackDamage*scale*buildingMul)
	w.emitAttackHitEffectLocked(src, tx, ty)
}

func (w *World) applyStatusToEntity(e *RawEntity, statusID int16, statusName string, duration float32) {
	if e == nil || duration <= 0 {
		return
	}
	prof, ok := w.resolveStatusProfile(statusID, statusName)
	if !ok || normalizeStatusKey(prof.Name) == "" || normalizeStatusKey(prof.Name) == "none" {
		return
	}
	for i := range e.Statuses {
		if normalizeStatusKey(e.Statuses[i].Name) == normalizeStatusKey(prof.Name) || (e.Statuses[i].ID == prof.ID && prof.ID != 0) {
			e.Statuses[i].Time = maxf(e.Statuses[i].Time, duration)
			return
		}
	}
	for i := range e.Statuses {
		if w.applyStatusTransition(e, i, prof, duration) {
			return
		}
	}
	if prof.Reactive {
		return
	}
	e.Statuses = append(e.Statuses, entityStatusState{
		ID:   prof.ID,
		Name: prof.Name,
		Time: duration,
	})
}

func (w *World) applyStatusTransition(e *RawEntity, idx int, incoming statusEffectProfile, duration float32) bool {
	if e == nil || idx < 0 || idx >= len(e.Statuses) {
		return false
	}
	current, ok := w.resolveStatusProfile(e.Statuses[idx].ID, e.Statuses[idx].Name)
	if !ok {
		return false
	}
	curName := normalizeStatusKey(current.Name)
	nextName := normalizeStatusKey(incoming.Name)
	for _, opp := range current.Opposites {
		if normalizeStatusKey(opp) != nextName {
			continue
		}
		e.Statuses[idx].Time -= duration * 0.5
		if e.Statuses[idx].Time <= 0 {
			e.Statuses[idx] = entityStatusState{ID: incoming.ID, Name: incoming.Name, Time: duration}
		}
		return true
	}

	switch curName {
	case "burning":
		if nextName == "tarred" {
			w.applyDamageToEntityDetailed(e, current.TransitionDamage, true)
			e.Statuses[idx].Time = minf(e.Statuses[idx].Time+duration, 300.0/60.0)
			return true
		}
	case "freezing":
		if nextName == "blasted" {
			w.applyDamageToEntityDetailed(e, current.TransitionDamage, true)
			return true
		}
	case "wet":
		if nextName == "shocked" {
			w.applyDamageToEntityDetailed(e, current.TransitionDamage, false)
			return true
		}
	case "melting":
		if nextName == "tarred" {
			w.applyDamageToEntityDetailed(e, 8, true)
			e.Statuses[idx].Time = minf(e.Statuses[idx].Time+duration, 200.0/60.0)
			return true
		}
	case "tarred":
		if nextName == "melting" || nextName == "burning" {
			e.Statuses[idx] = entityStatusState{
				ID:   incoming.ID,
				Name: incoming.Name,
				Time: e.Statuses[idx].Time + duration,
			}
			return true
		}
	}

	return false
}

func (w *World) applyDamageToEntityDetailed(e *RawEntity, dmg float32, pierceArmor bool) {
	if e == nil || dmg <= 0 {
		return
	}
	if e.PlayerID != 0 {
		return
	}
	if !pierceArmor {
		armor := entityArmorValue(*e)
		if armor > 0 {
			dmg = maxf(dmg-armor, dmg*minArmorDamageFraction)
		}
	}
	healthMul := entityHealthMultiplier(*e)
	if healthMul > 0 {
		dmg /= healthMul
	}
	if w != nil && w.rulesMgr != nil {
		if rules := w.rulesMgr.Get(); rules != nil && rules.UnitHealthMultiplier > 0 {
			dmg /= rules.UnitHealthMultiplier
		}
	}
	if dmg <= 0 {
		return
	}
	if e.Shield > 0 {
		absorb := minf(e.Shield, dmg)
		e.Shield -= absorb
		dmg -= absorb
	}
	if dmg > 0 {
		e.Health -= dmg
	}
}
