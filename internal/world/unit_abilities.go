package world

import (
	"math"
	"math/rand"
	"sort"
)

func squaredWorldDistance(ax, ay, bx, by float32) float32 {
	dx := ax - bx
	dy := ay - by
	return dx*dx + dy*dy
}

func abilityWorldPos(e RawEntity, x, y float32) (float32, float32) {
	return e.X + trnsx(e.Rotation-90, x, y), e.Y + trnsy(e.Rotation-90, x, y)
}

func shieldArcContainsPoint(e RawEntity, ability unitAbilityProfile, px, py float32) bool {
	if ability.Radius <= 0 {
		return false
	}
	cx, cy := abilityWorldPos(e, ability.X, ability.Y)
	width := maxf(ability.Width, 0)
	outer := ability.Radius + width
	d2 := squaredWorldDistance(cx, cy, px, py)
	if d2 > outer*outer {
		return false
	}
	if inner := ability.Radius - width; inner > 0 && d2 < inner*inner {
		return false
	}
	if ability.Angle > 0 && !angleWithin(lookAt(cx, cy, px, py), e.Rotation+ability.AngleOffset, ability.Angle*0.5) {
		return false
	}
	return true
}

type entityAbilityChangeSnapshot struct {
	health   float32
	shield   float32
	statuses []entityStatusState
}

func captureEntityAbilityChangeSnapshot(e *RawEntity) entityAbilityChangeSnapshot {
	snap := entityAbilityChangeSnapshot{}
	if e == nil {
		return snap
	}
	snap.health = e.Health
	snap.shield = e.Shield
	if len(e.Statuses) > 0 {
		snap.statuses = append([]entityStatusState(nil), e.Statuses...)
	}
	return snap
}

func (snap entityAbilityChangeSnapshot) changed(e *RawEntity) bool {
	if e == nil {
		return false
	}
	if snap.health != e.Health || snap.shield != e.Shield {
		return true
	}
	if len(snap.statuses) != len(e.Statuses) {
		return true
	}
	for i := range snap.statuses {
		if snap.statuses[i] != e.Statuses[i] {
			return true
		}
	}
	return false
}

func (w *World) entityNearCoreLocked(e RawEntity, rangeLimit float32) bool {
	if w == nil || w.model == nil || e.Team == 0 || rangeLimit <= 0 {
		return false
	}
	limit2 := rangeLimit * rangeLimit
	for _, pos := range w.teamCoreTiles[e.Team] {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Team != e.Team || tile.Build == nil || tile.Block <= 0 {
			continue
		}
		cx := float32(tile.X*8 + 4)
		cy := float32(tile.Y*8 + 4)
		dx := cx - e.X
		dy := cy - e.Y
		if dx*dx+dy*dy <= limit2 {
			return true
		}
	}
	return false
}

func (w *World) regenEntityAmmoLocked(e *RawEntity, dt float32) bool {
	if w == nil || e == nil || dt <= 0 {
		return false
	}
	capacity := e.AmmoCapacity
	if capacity <= 0 {
		return false
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.UnitAmmo {
		if e.Ammo != capacity {
			e.Ammo = capacity
			return true
		}
		return false
	}
	before := e.Ammo
	if e.AmmoRegen > 0 {
		e.Ammo = minf(capacity, e.Ammo+e.AmmoRegen*dt)
	}
	if e.Ammo < capacity && w.entityNearCoreLocked(*e, 80) {
		resupply := maxf(capacity*0.22, 10)
		e.Ammo = minf(capacity, e.Ammo+resupply*dt)
	}
	return before != e.Ammo
}

func (w *World) tryConsumeEntityAmmoLocked(e *RawEntity, amount float32) bool {
	if w == nil || e == nil || amount <= 0 {
		return true
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.UnitAmmo || e.AmmoCapacity <= 0 {
		return true
	}
	if e.Ammo < amount {
		return false
	}
	e.Ammo -= amount
	if e.Ammo < 0 {
		e.Ammo = 0
	}
	return true
}

func (w *World) stepEntityAbilities(dt float32) {
	if w == nil || w.model == nil || dt <= 0 {
		return
	}
	for i := range w.model.Entities {
		e := &w.model.Entities[i]
		changed := w.regenEntityAmmoLocked(e, dt)
		prof, ok := w.unitRuntimeProfileForEntityLocked(*e)
		if !ok || len(prof.Abilities) == 0 {
			if changed {
				w.model.EntitiesRev++
			}
			continue
		}
		w.ensureEntityAbilityStates(e, prof)
		for ai, ability := range prof.Abilities {
			if ai >= len(e.Abilities) {
				break
			}
			if w.stepEntityAbility(e, &e.Abilities[ai], ability, dt) {
				changed = true
			}
		}
		if changed {
			w.model.EntitiesRev++
		}
	}
}

func (w *World) stepEntityAbility(e *RawEntity, state *entityAbilityState, ability unitAbilityProfile, dt float32) bool {
	if e == nil || state == nil {
		return false
	}
	switch ability.Kind {
	case unitAbilityForceField:
		return false
	case unitAbilityShieldArc:
		before := state.Data
		if ability.Regen > 0 && state.Data < ability.Max {
			state.Data = minf(ability.Max, state.Data+ability.Regen*dt)
		}
		changed := before != state.Data
		if state.Data <= 0 || (ability.WhenShooting && !e.Shooting) {
			return changed
		}
		cx, cy := abilityWorldPos(*e, ability.X, ability.Y)
		reach := ability.Radius + maxf(ability.Width, 0)
		for i := range w.model.Entities {
			other := &w.model.Entities[i]
			if other.ID == e.ID || other.Team == e.Team || other.Health <= 0 {
				continue
			}
			if !shieldArcContainsPoint(*e, ability, other.X, other.Y) {
				continue
			}
			if w.unitAIKindForEntityLocked(*other) == unitAIMissile && ability.MissileUnitMultiplier >= 0 {
				missileDamageScale := float32(1)
				if w != nil && w.rulesMgr != nil {
					if rules := w.rulesMgr.Get(); rules != nil && rules.UnitDamageMultiplier > 0 {
						missileDamageScale = rules.UnitDamageMultiplier
					}
				}
				state.Data -= other.Health * ability.MissileUnitMultiplier * missileDamageScale
				applyAbilityShieldBreakPenalty(&state.Data, ability)
				other.Health = 0
				other.Shield = 0
				changed = true
				if state.Data <= 0 {
					break
				}
				continue
			}
			if !ability.PushUnits || (!isEntityFlying(*other) && isEntityFlying(*e)) {
				continue
			}
			dist2 := squaredWorldDistance(cx, cy, other.X, other.Y)
			if dist2 <= 0 {
				continue
			}
			dist := float32(math.Sqrt(float64(dist2)))
			overlapDst := reach - dist
			if overlapDst <= 0 {
				continue
			}
			if entityVelocityLen(*other) >= 0.1 {
				velAngle := lookAt(0, 0, other.VelX, other.VelY)
				if angleDistDeg(lookAt(other.X, other.Y, cx, cy), velAngle) < 90 {
					other.VelX, other.VelY = 0, 0
				}
			}
			push := overlapDst + 0.01
			other.X += (other.X - cx) / dist * push
			other.Y += (other.Y - cy) / dist * push
			changed = true
		}
		return changed
	case unitAbilityRepairField:
		state.Timer += dt
		if ability.Reload <= 0 || state.Timer < ability.Reload {
			return false
		}
		changed := false
		for i := range w.model.Entities {
			other := &w.model.Entities[i]
			if other.Team != e.Team || other.Health <= 0 {
				continue
			}
			if squaredWorldDistance(e.X, e.Y, other.X, other.Y) > ability.Range*ability.Range {
				continue
			}
			amount := ability.Amount
			if ability.HealPercent > 0 {
				amount += other.MaxHealth * ability.HealPercent / 100
			}
			if other.TypeID == e.TypeID && ability.SameTypeHealMult > 0 {
				amount *= ability.SameTypeHealMult
			}
			if amount > 0 && w.healEntity(other, amount) {
				changed = true
			}
		}
		state.Timer = 0
		return changed
	case unitAbilityShieldRegenField:
		state.Timer += dt
		if ability.Reload <= 0 || state.Timer < ability.Reload {
			return false
		}
		changed := false
		for i := range w.model.Entities {
			other := &w.model.Entities[i]
			if other.Team != e.Team || other.Health <= 0 {
				continue
			}
			if squaredWorldDistance(e.X, e.Y, other.X, other.Y) > ability.Range*ability.Range {
				continue
			}
			before := other.Shield
			limit := ability.Max
			if limit <= 0 {
				limit = other.ShieldMax
			}
			if limit <= 0 {
				continue
			}
			other.Shield = minf(limit, other.Shield+ability.Amount)
			if other.Shield != before {
				changed = true
			}
		}
		state.Timer = 0
		return changed
	case unitAbilityStatusField:
		state.Timer += dt
		if ability.Reload <= 0 || state.Timer < ability.Reload || (ability.OnShoot && !e.Shooting) {
			return false
		}
		changed := false
		for i := range w.model.Entities {
			other := &w.model.Entities[i]
			if other.Team != e.Team || other.Health <= 0 {
				continue
			}
			if squaredWorldDistance(e.X, e.Y, other.X, other.Y) > ability.Range*ability.Range {
				continue
			}
			before := captureEntityAbilityChangeSnapshot(other)
			w.applyStatusToEntity(other, ability.StatusID, ability.StatusName, ability.StatusDuration)
			if before.changed(other) {
				changed = true
			}
		}
		state.Timer = 0
		return changed
	case unitAbilityEnergyField:
		state.Timer += dt
		if ability.Reload <= 0 || state.Timer < ability.Reload {
			return false
		}
		cx, cy := abilityWorldPos(*e, ability.X, ability.Y)
		type target struct {
			entity   *RawEntity
			buildPos int32
			dist2    float32
			friendly bool
		}
		targets := make([]target, 0, 16)
		for i := range w.model.Entities {
			other := &w.model.Entities[i]
			if other.ID == e.ID || other.Health <= 0 {
				continue
			}
			d2 := squaredWorldDistance(cx, cy, other.X, other.Y)
			if d2 > ability.Range*ability.Range {
				continue
			}
			if other.Team == e.Team {
				if other.Health < other.MaxHealth {
					targets = append(targets, target{entity: other, dist2: d2, friendly: true})
				}
				continue
			}
			if (isEntityFlying(*other) && !ability.TargetAir) || (!isEntityFlying(*other) && !ability.TargetGround) {
				continue
			}
			targets = append(targets, target{entity: other, dist2: d2})
		}
		if ability.HitBuildings && ability.TargetGround && w.model != nil {
			r := int(math.Ceil(float64(ability.Range / 8)))
			tx := int(cx / 8)
			ty := int(cy / 8)
			for y := ty - r; y <= ty+r; y++ {
				for x := tx - r; x <= tx+r; x++ {
					if !w.model.InBounds(x, y) {
						continue
					}
					pos := int32(y*w.model.Width + x)
					build := w.model.Tiles[pos].Build
					if build == nil || build.Health <= 0 {
						continue
					}
					bx := float32(x*8 + 4)
					by := float32(y*8 + 4)
					d2 := squaredWorldDistance(cx, cy, bx, by)
					if d2 > ability.Range*ability.Range {
						continue
					}
					if build.Team == e.Team {
						if build.Health < build.MaxHealth {
							targets = append(targets, target{buildPos: pos, dist2: d2, friendly: true})
						}
						continue
					}
					targets = append(targets, target{buildPos: pos, dist2: d2})
				}
			}
		}
		sort.Slice(targets, func(i, j int) bool { return targets[i].dist2 < targets[j].dist2 })
		limit := int(ability.MaxTargets)
		if limit <= 0 || limit > len(targets) {
			limit = len(targets)
		}
		if limit == 0 {
			state.Timer = 0
			return false
		}
		if ability.UseAmmo && !w.tryConsumeEntityAmmoLocked(e, maxf(1, e.AmmoPerShot)) {
			return false
		}
		changed := false
		for i := 0; i < limit; i++ {
			t := targets[i]
			if t.entity != nil {
				if pos, ok := w.findLaserAbsorberLocked(e.Team, cx, cy, t.entity.X, t.entity.Y); ok {
					t.entity = nil
					t.buildPos = pos
					t.friendly = false
				}
			} else if t.buildPos >= 0 {
				bx := float32(w.model.Tiles[t.buildPos].X*8 + 4)
				by := float32(w.model.Tiles[t.buildPos].Y*8 + 4)
				if pos, ok := w.findLaserAbsorberLocked(e.Team, cx, cy, bx, by); ok {
					t.buildPos = pos
					t.friendly = false
				}
			}
			if t.entity != nil {
				if t.friendly {
					amount := t.entity.MaxHealth * ability.HealPercent / 100
					if t.entity.TypeID == e.TypeID && ability.SameTypeHealMult > 0 {
						amount *= ability.SameTypeHealMult
					}
					if amount > 0 && w.healEntity(t.entity, amount) {
						changed = true
					}
				} else {
					before := captureEntityAbilityChangeSnapshot(t.entity)
					w.applyDamageToEntityDetailed(t.entity, ability.Damage*w.outgoingDamageScale(*e, false), false)
					w.applyStatusToEntity(t.entity, ability.StatusID, ability.StatusName, ability.StatusDuration)
					if before.changed(t.entity) {
						changed = true
					}
				}
				continue
			}
			if t.buildPos >= 0 {
				if t.friendly {
					if w.healBuilding(t.buildPos, w.model.Tiles[t.buildPos].Build.MaxHealth*ability.HealPercent/100) {
						changed = true
					}
				} else if w.applyDamageToBuildingDetailed(t.buildPos, ability.Damage*w.outgoingDamageScale(*e, false)) {
					changed = true
				}
			}
		}
		state.Timer = 0
		return changed
	case unitAbilitySuppressionField:
		if !ability.Active {
			return false
		}
		state.Timer += dt
		trigger := ability.Cooldown
		if trigger <= 0 {
			trigger = ability.Reload
		}
		if trigger <= 0 || state.Timer < trigger {
			return false
		}
		cx, cy := abilityWorldPos(*e, ability.X, ability.Y)
		duration := ability.Reload + 1.0/60.0
		if duration <= 0 {
			duration = trigger + 1.0/60.0
		}
		changed := false
		w.forEachEnemyBuildingInRange(e.Team, cx, cy, ability.Range, func(pos int32) {
			if pos < 0 || int(pos) >= len(w.model.Tiles) {
				return
			}
			tile := &w.model.Tiles[pos]
			if tile.Build == nil || tile.Build.Health <= 0 || tile.Build.Team == e.Team {
				return
			}
			bx := float32(tile.X*8 + 4)
			by := float32(tile.Y*8 + 4)
			if squaredWorldDistance(cx, cy, bx, by) > ability.Range*ability.Range {
				return
			}
			if w.applyBuildingHealSuppressionLocked(tile.Build, duration) {
				changed = true
			}
		})
		state.Timer = 0
		return changed
	case unitAbilityMoveEffect:
		state.Timer += dt
		if ability.Reload > 0 && state.Timer >= ability.Reload {
			state.Timer -= ability.Reload
		}
		return false
	default:
		return false
	}
}

func bulletShieldDamageValue(b simBullet) float32 {
	damage := b.Damage
	if b.ShieldDamageMul > 0 {
		damage *= b.ShieldDamageMul
	}
	return maxf(damage, 0)
}

func bulletShieldBreakValue(b simBullet) float32 {
	return maxf(b.Damage, 0)
}

func applyAbilityShieldBreakPenalty(value *float32, ability unitAbilityProfile) {
	if value == nil || *value > 0 || ability.Cooldown <= 0 || ability.Regen <= 0 {
		return
	}
	*value -= ability.Cooldown * ability.Regen
}

func (w *World) absorbBulletByUnitAbilitiesLocked(b *simBullet, dt float32) (handled bool, remove bool) {
	if w == nil || w.model == nil || b == nil {
		return false, false
	}
	damage := bulletShieldDamageValue(*b)
	if damage <= 0 {
		return false, false
	}
	for i := range w.model.Entities {
		e := &w.model.Entities[i]
		if e.Health <= 0 || e.Team == 0 || e.Team == b.Team {
			continue
		}
		prof, ok := w.unitRuntimeProfileForEntityLocked(*e)
		if !ok || len(prof.Abilities) == 0 {
			continue
		}
		w.ensureEntityAbilityStates(e, prof)
		if handled, remove := w.absorbBulletWithEntityAbilitiesLocked(e, prof, b, dt); handled {
			w.model.EntitiesRev++
			return true, remove
		}
	}
	return false, false
}

func reflectShieldArcBullet(e *RawEntity, ability unitAbilityProfile, b *simBullet, dt float32) {
	if e == nil || b == nil {
		return
	}
	oldVX, oldVY := b.VX, b.VY
	if dt > 0 {
		b.X -= oldVX * dt
		b.Y -= oldVY * dt
	}
	cx, cy := abilityWorldPos(*e, ability.X, ability.Y)
	penX := float32(math.Abs(float64(cx - b.X)))
	penY := float32(math.Abs(float64(cy - b.Y)))
	if penX > penY {
		b.VX = -oldVX
		b.VY = oldVY
	} else {
		b.VY = -oldVY
		b.VX = oldVX
	}
	b.Team = e.Team
	if b.LifeSec > 0 {
		b.AgeSec = b.LifeSec * 0.5
	}
}

func (w *World) absorbBulletWithEntityAbilitiesLocked(e *RawEntity, prof unitRuntimeProfile, b *simBullet, dt float32) (handled bool, remove bool) {
	if e == nil || b == nil || len(prof.Abilities) == 0 {
		return false, false
	}
	damage := bulletShieldDamageValue(*b)
	breakDamage := bulletShieldBreakValue(*b)
	if damage <= 0 {
		return false, false
	}
	for i, ability := range prof.Abilities {
		if i >= len(e.Abilities) {
			break
		}
		switch ability.Kind {
		case unitAbilityForceField:
			if e.Shield <= 0 || ability.Radius <= 0 {
				continue
			}
			if squaredWorldDistance(e.X, e.Y, b.X, b.Y) > ability.Radius*ability.Radius {
				continue
			}
			e.Shield -= damage
			applyAbilityShieldBreakPenalty(&e.Shield, ability)
			return true, true
		case unitAbilityShieldArc:
			if e.Abilities[i].Data <= 0 || ability.Radius <= 0 {
				continue
			}
			if ability.WhenShooting && !e.Shooting {
				continue
			}
			if !shieldArcContainsPoint(*e, ability, b.X, b.Y) {
				continue
			}
			if breakDamage > 0 && e.Abilities[i].Data <= breakDamage {
				e.Abilities[i].Data -= ability.Cooldown * ability.Regen
			}
			e.Abilities[i].Data -= damage
			if ability.ChanceDeflect > 0 && damage > 0 && (b.VX != 0 || b.VY != 0) {
				reflectShieldArcBullet(e, ability, b, dt)
				return true, false
			}
			return true, true
		}
	}
	return false, false
}

func (w *World) absorbEntityAbilityDamage(e *RawEntity, sourceX, sourceY, dmg float32) (float32, bool) {
	if w == nil || e == nil || dmg <= 0 {
		return dmg, false
	}
	prof, ok := w.unitRuntimeProfileForEntityLocked(*e)
	if !ok || len(prof.Abilities) == 0 {
		return dmg, false
	}
	absorbed := false
	for i, ability := range prof.Abilities {
		if i >= len(e.Abilities) {
			break
		}
		switch ability.Kind {
		case unitAbilityForceField:
			if e.Shield <= 0 || ability.Radius <= 0 {
				continue
			}
			if squaredWorldDistance(e.X, e.Y, sourceX, sourceY) > ability.Radius*ability.Radius {
				continue
			}
			e.Shield -= dmg
			applyAbilityShieldBreakPenalty(&e.Shield, ability)
			return 0, true
		case unitAbilityShieldArc:
			if e.Abilities[i].Data <= 0 || ability.Radius <= 0 {
				continue
			}
			if ability.WhenShooting && !e.Shooting {
				continue
			}
			if !shieldArcContainsPoint(*e, ability, sourceX, sourceY) {
				continue
			}
			e.Abilities[i].Data -= dmg
			applyAbilityShieldBreakPenalty(&e.Abilities[i].Data, ability)
			absorbed = true
			return 0, absorbed
		}
	}
	return dmg, absorbed
}

func (w *World) handleEntityDeathAbilitiesLocked(e RawEntity) {
	prof, ok := w.unitRuntimeProfileForEntityLocked(e)
	if !ok || len(prof.Abilities) == 0 {
		return
	}
	for _, ability := range prof.Abilities {
		if ability.Kind != unitAbilitySpawnDeath || ability.SpawnAmount <= 0 {
			continue
		}
		typeID, ok := w.resolveUnitTypeIDLocked(ability.SpawnUnitName)
		if !ok || typeID <= 0 {
			continue
		}
		count := ability.SpawnAmount
		if ability.SpawnRandAmount > 0 {
			count += int32(rand.Int31n(ability.SpawnRandAmount + 1))
		}
		for i := int32(0); i < count; i++ {
			if !w.canCreateUnitLocked(e.Team, typeID, w.rulesMgr.Get(), nil, map[TeamID]map[int16]int32{
				e.Team: {typeID: w.teamUnitCountByTypeLocked(e.Team, typeID)},
			}) {
				break
			}
			angle := rand.Float64() * math.Pi * 2
			dist := rand.Float64() * float64(maxf(ability.Spread, 0))
			x := e.X + float32(math.Cos(angle))*float32(dist)
			y := e.Y + float32(math.Sin(angle))*float32(dist)
			rotation := e.Rotation
			if ability.FaceOutwards {
				rotation = float32(angle * 180 / math.Pi)
			} else {
				rotation += rand.Float32()*10 - 5
			}
			child := w.newProducedUnitEntityLocked(typeID, e.Team, x, y, rotation)
			child.SpawnedByCore = e.SpawnedByCore
			w.model.AddEntity(child)
		}
	}
}
