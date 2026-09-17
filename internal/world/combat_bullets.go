package world

import (
	"math"
	"math/rand"
	"strings"
)

func (w *World) fireAimedBuildingBeam(src RawEntity, prof buildingWeaponProfile, tx, ty float32, sourceIsBuilding bool) bool {
	if w == nil {
		return false
	}
	beam := simBullet{
		Team:               src.Team,
		X:                  src.X,
		Y:                  src.Y,
		Damage:             src.AttackDamage * w.outgoingDamageScale(src, sourceIsBuilding),
		HitBuilds:          src.AttackBuildings,
		BuildingDamage:     entityBuildingDamageMultiplier(src),
		ArmorMultiplier:    src.AttackArmorMultiplier,
		MaxDamageFraction:  src.AttackMaxDamageFraction,
		ShieldDamageMul:    src.AttackShieldDamageMul,
		PierceDamageFactor: src.AttackPierceDamageFactor,
		PierceArmor:        src.AttackPierceArmor,
		SlowSec:            src.AttackSlowSec,
		SlowMul:            clampf(src.AttackSlowMul, 0.2, 1),
		StatusID:           src.AttackStatusID,
		StatusName:         src.AttackStatusName,
		StatusDuration:     src.AttackStatusDuration,
		TargetAir:          src.AttackTargetAir,
		TargetGround:       src.AttackTargetGround,
		AimX:               tx,
		AimY:               ty,
		BulletClass:        prof.BulletClass,
		BeamLength:         prof.Range,
		SplashRadius:       src.AttackSplashRadius,
	}
	w.emitAttackFireEffectsLocked(src)
	impacted := false
	if isPointLaserBulletClass(beam.BulletClass) {
		impacted = w.applyPointBeamDamage(beam)
	} else {
		impacted = w.applyLineBeamDamage(beam)
	}
	if impacted {
		w.emitAttackHitEffectLocked(src, tx, ty)
	}
	return true
}

func (w *World) tryFireBuildingShot(buildPos int32, tile *Tile, src *RawEntity, state *buildCombatState, prof buildingWeaponProfile, ents []RawEntity, targetIdx int, targetBuildPos int32, tx, ty float32, controlled bool, canShoot bool) bool {
	if src == nil || state == nil {
		return false
	}
	if !w.buildingHasAmmoLocked(buildPos, tile, prof, *state) {
		return false
	}
	if prof.PowerPerShot > 0 && state.Power < prof.PowerPerShot {
		return false
	}

	fired := false
	if controlled {
		if !canShoot {
			return false
		}
		if src.AttackFireMode == "beam" {
			return w.fireAimedBuildingBeam(*src, prof, tx, ty, true)
		}
		w.spawnBullet(*src, tx, ty, true)
		fired = true
	} else {
		if targetIdx >= 0 && targetIdx < len(ents) {
			target := &ents[targetIdx]
			if src.AttackFireMode == "beam" {
				w.fireBeamAtEntity(*src, target, targetIdx, true)
			} else {
				w.spawnBullet(*src, tx, ty, true)
			}
			fired = true
		}
		if !fired && targetBuildPos >= 0 {
			if src.AttackFireMode == "beam" {
				w.fireBeamAtBuilding(*src, targetBuildPos, tx, ty, true)
			} else {
				w.spawnBullet(*src, tx, ty, true)
			}
			fired = true
		}
	}
	if !fired {
		return false
	}
	if !w.consumeBuildingAmmoLocked(buildPos, tile, prof, state) {
		return false
	}
	if prof.PowerPerShot > 0 {
		state.Power -= prof.PowerPerShot
		if state.Power < 0 {
			state.Power = 0
		}
	}
	return true
}

func (w *World) spawnBullet(src RawEntity, tx, ty float32, sourceIsBuilding bool) {
	w.spawnBulletWithAngle(src, tx, ty, lookAt(src.X, src.Y, tx, ty), 1, pendingMountShot{}, sourceIsBuilding)
}

func (w *World) spawnBulletWithAngle(src RawEntity, tx, ty, angle, speedScale float32, shot pendingMountShot, sourceIsBuilding bool) {
	bulletSpeed := src.AttackBulletSpeed
	if bulletSpeed <= 0 {
		speed := src.MoveSpeed
		if speed <= 0 {
			speed = 18
		}
		bulletSpeed = maxf(speed*2.2, 28)
	}
	if speedScale <= 0 {
		speedScale = 1
	}
	bulletSpeed *= speedScale
	rad := float32(angle * math.Pi / 180)
	damageScale := w.outgoingDamageScale(src, sourceIsBuilding)
	lifeSec := src.AttackBulletLifetime
	if lifeSec <= 0 && bulletSpeed > 0 {
		lifeSec = maxf(src.AttackRange/bulletSpeed, 0.6)
	}
	radius := maxf(src.AttackBulletHitSize*0.5, 4)
	buildingMul := entityBuildingDamageMultiplier(src)
	b := simBullet{
		ID:                 w.bulletNextID,
		Team:               src.Team,
		X:                  src.X,
		Y:                  src.Y,
		VX:                 float32(math.Cos(float64(rad))) * bulletSpeed,
		VY:                 float32(math.Sin(float64(rad))) * bulletSpeed,
		Damage:             src.AttackDamage * damageScale,
		SplashDamage:       src.AttackSplashDamage * damageScale,
		LifeSec:            lifeSec,
		AgeSec:             0,
		Radius:             radius,
		HitUnits:           true,
		HitBuilds:          src.AttackBuildings,
		BulletType:         src.AttackBulletType,
		SplashRadius:       src.AttackSplashRadius,
		// Multiplier only; actual hit uses Damage * BuildingDamage.
		BuildingDamage:     buildingMul,
		ArmorMultiplier:    src.AttackArmorMultiplier,
		MaxDamageFraction:  src.AttackMaxDamageFraction,
		ShieldDamageMul:    src.AttackShieldDamageMul,
		PierceDamageFactor: src.AttackPierceDamageFactor,
		PierceArmor:        src.AttackPierceArmor,
		SlowSec:            src.AttackSlowSec,
		SlowMul:            clampf(src.AttackSlowMul, 0.2, 1),
		PierceRemain:       src.AttackPierce,
		PierceBuilding:     src.AttackPierceBuilding,
		ChainCount:         src.AttackChainCount,
		ChainRange:         src.AttackChainRange,
		FragmentCount:      src.AttackFragmentCount,
		FragmentSpread:     src.AttackFragmentSpread,
		FragmentSpeed:      src.AttackFragmentSpeed,
		FragmentLife:       src.AttackFragmentLife,
		FragmentRand:       src.AttackFragmentRand,
		FragmentAngle:      src.AttackFragmentAngle,
		FragmentVelMin:     src.AttackFragmentVelMin,
		FragmentVelMax:     src.AttackFragmentVelMax,
		FragmentLifeMin:    src.AttackFragmentLifeMin,
		FragmentLifeMax:    src.AttackFragmentLifeMax,
		FragmentBullet:     cloneBulletRuntimeProfile(src.AttackFragmentBullet),
		StatusID:           src.AttackStatusID,
		StatusName:         src.AttackStatusName,
		StatusDuration:     src.AttackStatusDuration,
		ShootEffect:        src.AttackShootEffect,
		SmokeEffect:        src.AttackSmokeEffect,
		HitEffect:          src.AttackHitEffect,
		DespawnEffect:      src.AttackDespawnEffect,
		TargetAir:          src.AttackTargetAir,
		TargetGround:       src.AttackTargetGround,
		TargetPriority:     src.AttackTargetPriority,
		HelixScl:           shot.HelixScl,
		HelixMag:           shot.HelixMag,
		HelixOffset:        shot.HelixOffset,
	}
	w.bulletNextID++
	w.bullets = append(w.bullets, b)
	w.emitAttackFireEffectsLocked(src)
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind: EntityEventBulletFired,
		Bullet: BulletEvent{
			Team:      b.Team,
			X:         b.X,
			Y:         b.Y,
			Angle:     angle,
			Damage:    b.Damage,
			BulletTyp: b.BulletType,
		},
	})
}

func (w *World) stepBullets(dt float32, idToIndex map[int32]int, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex, abilityCandidates ...[]unitAbilityCandidate) {
	if len(w.bullets) == 0 {
		return
	}
	candidates := []unitAbilityCandidate(nil)
	if len(abilityCandidates) > 0 {
		candidates = abilityCandidates[0]
	} else {
		w.unitAbilityCandidates = w.collectUnitAbilityCandidatesLocked(w.unitAbilityCandidates)
		candidates = w.unitAbilityCandidates
	}
	for i := 0; i < len(w.bullets); {
		b := &w.bullets[i]
		if isPersistentBeamBulletClass(b.BulletClass) {
			impacted, expired := w.stepPersistentBeamBullet(b, dt)
			impactRot := beamImpactRotation(*b)
			if impacted {
				tx, ty := beamEndPosition(*b)
				w.emitEffectLocked(b.HitEffect, tx, ty, impactRot)
			} else if expired {
				tx, ty := beamEndPosition(*b)
				w.emitEffectLocked(b.DespawnEffect, tx, ty, impactRot)
			}
			if !expired {
				i++
				continue
			}
			last := len(w.bullets) - 1
			w.bullets[i] = w.bullets[last]
			w.bullets = w.bullets[:last]
			continue
		}
		b.AgeSec += dt
		b.X += b.VX * dt
		b.Y += b.VY * dt
		if b.HelixScl > 0 && b.HelixMag != 0 {
			rot := float32(math.Atan2(float64(b.VY), float64(b.VX)) * 180 / math.Pi)
			side := float32(math.Sin(float64((b.AgeSec*60+b.HelixOffset)/b.HelixScl))) * b.HelixMag * dt * 60
			b.X += trnsx(rot, 0, side)
			b.Y += trnsy(rot, 0, side)
		}
		if handled, remove := w.absorbBulletByUnitAbilitiesLocked(b, dt, candidates); handled {
			if remove {
				last := len(w.bullets) - 1
				w.bullets[i] = w.bullets[last]
				w.bullets = w.bullets[:last]
				continue
			}
			i++
			continue
		}
		hit := false
		impacted := false
		if b.HitUnits {
			if idx, ok := findHitEnemyEntityIndex(*b, w.model.Entities, spatial, teamSpatial, b.Radius, b.TargetAir, b.TargetGround); ok && idx >= 0 && idx < len(w.model.Entities) {
				target := &w.model.Entities[idx]
				if remaining, absorbed := w.absorbEntityAbilityDamage(target, b.X, b.Y, b.Damage); absorbed {
					hit = true
					impacted = true
				} else {
					initialHealth := w.applyDamageToEntityProfile(target, remaining, bulletDamageApplyProfile(*b))
					applyPierceDamageLoss(&b.Damage, b.PierceDamageFactor, initialHealth)
					applySlow(target, b.SlowSec, b.SlowMul)
					w.applyStatusToEntity(target, b.StatusID, b.StatusName, b.StatusDuration)
					hit = true
					impacted = true
				}
				w.applyChainDamage(*b, idx)
				w.applySplashDamage(*b)
				if b.PierceRemain > 0 {
					b.PierceRemain--
					hit = false
				}
			}
		}
		if !hit && b.HitBuilds {
			// Prefer the tile the bullet is on (vanilla-style), then a nearby
			// enemy building center. Radius-only center search misses 1x1 walls.
			buildPos := int32(-1)
			if w.model != nil {
				tx := int(b.X / 8)
				ty := int(b.Y / 8)
				if w.model.InBounds(tx, ty) {
					p := int32(ty*w.model.Width + tx)
					t := &w.model.Tiles[p]
					if t.Build != nil && t.Build.Team != b.Team && t.Build.Health > 0 {
						buildPos = p
					}
				}
			}
			if buildPos < 0 {
				searchR := maxf(b.Radius, 12)
				if pos, _, _, ok := w.findNearestEnemyBuilding(RawEntity{X: b.X, Y: b.Y, Team: b.Team}, searchR); ok {
					buildPos = pos
				}
			}
			if buildPos >= 0 {
				initialHealth := float32(0)
				if int(buildPos) < len(w.model.Tiles) && w.model.Tiles[buildPos].Build != nil {
					initialHealth = w.model.Tiles[buildPos].Build.Health
				}
				if w.applyDamageToBuildingProfile(buildPos, b.Damage*b.BuildingDamage, bulletDamageApplyProfile(*b)) {
					applyPierceDamageLoss(&b.Damage, b.PierceDamageFactor, initialHealth)
					w.applySplashDamage(*b)
					hit = true
					impacted = true
					if b.PierceBuilding && b.PierceRemain > 0 {
						b.PierceRemain--
						hit = false
					}
				}
			}
		}
		expired := b.AgeSec >= b.LifeSec
		impactRot := float32(math.Atan2(float64(b.VY), float64(b.VX)) * 180 / math.Pi)
		if impacted {
			w.emitEffectLocked(b.HitEffect, b.X, b.Y, impactRot)
		} else if expired {
			w.emitEffectLocked(b.DespawnEffect, b.X, b.Y, impactRot)
		}
		if !hit && !expired {
			i++
			continue
		}
		if (hit || expired) && b.FragmentCount > 0 {
			w.spawnBulletFragments(*b)
		}
		last := len(w.bullets) - 1
		w.bullets[i] = w.bullets[last]
		w.bullets = w.bullets[:last]
	}
}

func (w *World) spawnBulletFragments(parent simBullet) {
	n := parent.FragmentCount
	if n <= 0 {
		return
	}
	baseAngle := float32(math.Atan2(float64(parent.VY), float64(parent.VX)) * 180 / math.Pi)
	spread := parent.FragmentSpread
	if spread <= 0 {
		spread = 20
	}
	for i := int32(0); i < n; i++ {
		t := float32(i)
		offset := float32(0)
		if n > 1 {
			offset = (t/float32(n-1))*spread - spread/2
		}
		randomSpread := parent.FragmentRand
		if randomSpread <= 0 {
			randomSpread = spread
		}
		ang := baseAngle + parent.FragmentAngle + float32(offset) + (rand.Float32()-0.5)*randomSpread
		rad := float32(ang * math.Pi / 180)
		template := parent.FragmentBullet
		speed := parent.FragmentSpeed
		life := parent.FragmentLife
		damage := parent.Damage * 0.45
		splashDamage := parent.SplashDamage * 0.45
		splashRadius := parent.SplashRadius * 0.5
		radius := float32(4)
		buildingDamage := parent.BuildingDamage
		armorMultiplier := parent.ArmorMultiplier
		maxDamageFraction := parent.MaxDamageFraction
		shieldDamageMul := parent.ShieldDamageMul
		pierceDamageFactor := parent.PierceDamageFactor
		pierceArmor := parent.PierceArmor
		bulletType := parent.BulletType
		bulletClass := parent.BulletClass
		pierce := int32(0)
		pierceBuilding := false
		statusID := parent.StatusID
		statusName := parent.StatusName
		statusDuration := parent.StatusDuration
		hitBuilds := parent.HitBuilds
		targetAir := parent.TargetAir
		targetGround := parent.TargetGround
		hitEffect := parent.HitEffect
		despawnEffect := parent.DespawnEffect
		fragCount := int32(0)
		fragSpread2 := float32(0)
		fragRand := float32(0)
		fragAngle := float32(0)
		fragVelMin := float32(0)
		fragVelMax := float32(0)
		fragLifeMin := float32(0)
		fragLifeMax := float32(0)
		var fragBullet *bulletRuntimeProfile
		if template != nil {
			if template.Speed > 0 {
				speed = template.Speed
			}
			if template.Lifetime > 0 {
				life = template.Lifetime
			}
			damage = template.Damage
			splashDamage = template.SplashDamage
			splashRadius = template.SplashRadius
			radius = maxf(template.HitSize*0.5, 4)
			buildingDamage = template.BuildingDamage
			armorMultiplier = template.ArmorMultiplier
			maxDamageFraction = template.MaxDamageFraction
			shieldDamageMul = template.ShieldDamageMul
			pierceDamageFactor = template.PierceDamageFactor
			pierceArmor = template.PierceArmor
			bulletType = template.BulletType
			bulletClass = template.ClassName
			pierce = template.Pierce
			pierceBuilding = template.PierceBuilding
			statusID = template.StatusID
			statusName = template.StatusName
			statusDuration = template.StatusDuration
			hitBuilds = template.HitBuildings
			targetAir = template.TargetAir
			targetGround = template.TargetGround
			hitEffect = template.HitEffect
			despawnEffect = template.DespawnEffect
			fragCount = template.FragmentCount
			fragSpread2 = template.FragmentSpread
			fragRand = template.FragmentRandom
			fragAngle = template.FragmentAngle
			fragVelMin = template.FragmentVelocityMin
			fragVelMax = template.FragmentVelocityMax
			fragLifeMin = template.FragmentLifeMin
			fragLifeMax = template.FragmentLifeMax
			fragBullet = cloneBulletRuntimeProfile(template.FragmentBullet)
		}
		speedMul := randomRange(parent.FragmentVelMin, parent.FragmentVelMax)
		if speedMul == 0 {
			speedMul = 1
		}
		lifeMul := randomRange(parent.FragmentLifeMin, parent.FragmentLifeMax)
		if lifeMul == 0 {
			lifeMul = 1
		}
		b := simBullet{
			ID:                 w.bulletNextID,
			Team:               parent.Team,
			X:                  parent.X,
			Y:                  parent.Y,
			VX:                 float32(math.Cos(float64(rad))) * speed * speedMul,
			VY:                 float32(math.Sin(float64(rad))) * speed * speedMul,
			Damage:             damage,
			SplashDamage:       splashDamage,
			LifeSec:            maxf(life*lifeMul, 0.2),
			Radius:             radius,
			HitUnits:           parent.HitUnits,
			HitBuilds:          hitBuilds,
			BulletType:         bulletType,
			BulletClass:        bulletClass,
			SplashRadius:       splashRadius,
			BuildingDamage:     buildingDamage,
			ArmorMultiplier:    armorMultiplier,
			MaxDamageFraction:  maxDamageFraction,
			ShieldDamageMul:    shieldDamageMul,
			PierceDamageFactor: pierceDamageFactor,
			PierceArmor:        pierceArmor,
			SlowSec:            parent.SlowSec,
			SlowMul:            parent.SlowMul,
			PierceRemain:       pierce,
			PierceBuilding:     pierceBuilding,
			ChainCount:         0,
			ChainRange:         0,
			FragmentCount:      fragCount,
			FragmentSpread:     fragSpread2,
			FragmentRand:       fragRand,
			FragmentAngle:      fragAngle,
			FragmentVelMin:     fragVelMin,
			FragmentVelMax:     fragVelMax,
			FragmentLifeMin:    fragLifeMin,
			FragmentLifeMax:    fragLifeMax,
			FragmentBullet:     fragBullet,
			StatusID:           statusID,
			StatusName:         statusName,
			StatusDuration:     statusDuration,
			ShootEffect:        "",
			SmokeEffect:        "",
			HitEffect:          hitEffect,
			DespawnEffect:      despawnEffect,
			TargetAir:          targetAir,
			TargetGround:       targetGround,
			TargetPriority:     parent.TargetPriority,
		}
		w.bulletNextID++
		w.bullets = append(w.bullets, b)
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind: EntityEventBulletFired,
			Bullet: BulletEvent{
				Team:      b.Team,
				X:         b.X,
				Y:         b.Y,
				Angle:     ang,
				Damage:    b.Damage,
				BulletTyp: b.BulletType,
			},
		})
	}
}

func (w *World) applySplashDamage(b simBullet) {
	if b.SplashRadius <= 0 || (b.SplashDamage <= 0 && b.StatusID == 0 && strings.TrimSpace(b.StatusName) == "") {
		return
	}
	// Damage enemy units in splash radius.
	for i := range w.model.Entities {
		e := &w.model.Entities[i]
		if e.Health <= 0 || e.Team == b.Team {
			continue
		}
		dx := e.X - b.X
		dy := e.Y - b.Y
		d2 := dx*dx + dy*dy
		if d2 > b.SplashRadius*b.SplashRadius {
			continue
		}
		dist := float32(math.Sqrt(float64(d2)))
		scale := 1 - 0.6*(dist/b.SplashRadius)
		if scale < 0.4 {
			scale = 0.4
		}
		if b.SplashDamage > 0 {
			if remaining, absorbed := w.absorbEntityAbilityDamage(e, b.X, b.Y, b.SplashDamage*scale); !absorbed {
				w.applyDamageToEntityProfile(e, remaining, bulletDamageApplyProfile(b))
			}
		}
		applySlow(e, b.SlowSec*scale, b.SlowMul)
		w.applyStatusToEntity(e, b.StatusID, b.StatusName, b.StatusDuration)
	}
	// Damage enemy buildings in splash radius.
	w.forEachEnemyBuildingInRange(b.Team, b.X, b.Y, b.SplashRadius, func(pos int32) {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			return
		}
		t := &w.model.Tiles[pos]
		if t.Build == nil || t.Build.Health <= 0 {
			return
		}
		px := float32(t.X*8 + 4)
		py := float32(t.Y*8 + 4)
		dx := px - b.X
		dy := py - b.Y
		d2 := dx*dx + dy*dy
		if d2 > b.SplashRadius*b.SplashRadius {
			return
		}
		dist := float32(math.Sqrt(float64(d2)))
		scale := 1 - 0.6*(dist/b.SplashRadius)
		if scale < 0.4 {
			scale = 0.4
		}
		if b.SplashDamage > 0 {
			_ = w.applyDamageToBuildingDetailed(pos, b.SplashDamage*scale*b.BuildingDamage)
		}
	})
}

func (w *World) applyChainDamage(b simBullet, firstIdx int) {
	if b.ChainCount <= 0 || b.ChainRange <= 0 || firstIdx < 0 || firstIdx >= len(w.model.Entities) {
		return
	}
	hit := map[int]struct{}{firstIdx: {}}
	prev := firstIdx
	for c := int32(0); c < b.ChainCount; c++ {
		next := -1
		bestDist2 := b.ChainRange * b.ChainRange
		px := w.model.Entities[prev].X
		py := w.model.Entities[prev].Y
		for i := range w.model.Entities {
			if _, exists := hit[i]; exists {
				continue
			}
			e := &w.model.Entities[i]
			if e.Health <= 0 || e.Team == b.Team {
				continue
			}
			dx := e.X - px
			dy := e.Y - py
			d2 := dx*dx + dy*dy
			if d2 > bestDist2 {
				continue
			}
			bestDist2 = d2
			next = i
		}
		if next < 0 {
			return
		}
		scale := float32(math.Pow(0.72, float64(c+1)))
		damage := b.Damage * scale
		target := &w.model.Entities[next]
		if remaining, absorbed := w.absorbEntityAbilityDamage(target, px, py, damage); !absorbed {
			w.applyDamageToEntityProfile(target, remaining, bulletDamageApplyProfile(b))
			applySlow(target, b.SlowSec*scale, b.SlowMul)
			w.applyStatusToEntity(target, b.StatusID, b.StatusName, b.StatusDuration)
		}
		hit[next] = struct{}{}
		prev = next
	}
}

func (w *World) applyBeamChainFromSource(src RawEntity, firstIdx int, sourceIsBuilding bool) {
	if src.AttackChainCount <= 0 || src.AttackChainRange <= 0 || firstIdx < 0 || firstIdx >= len(w.model.Entities) {
		return
	}
	hit := map[int]struct{}{firstIdx: {}}
	prev := firstIdx
	for c := int32(0); c < src.AttackChainCount; c++ {
		next := -1
		bestDist2 := src.AttackChainRange * src.AttackChainRange
		px := w.model.Entities[prev].X
		py := w.model.Entities[prev].Y
		for i := range w.model.Entities {
			if _, exists := hit[i]; exists {
				continue
			}
			e := &w.model.Entities[i]
			if e.Health <= 0 || e.Team == src.Team {
				continue
			}
			dx := e.X - px
			dy := e.Y - py
			d2 := dx*dx + dy*dy
			if d2 > bestDist2 {
				continue
			}
			bestDist2 = d2
			next = i
		}
		if next < 0 {
			return
		}
		scale := float32(math.Pow(0.72, float64(c+1)))
		dmg := src.AttackDamage * scale * w.outgoingDamageScale(src, sourceIsBuilding)
		target := &w.model.Entities[next]
		if remaining, absorbed := w.absorbEntityAbilityDamage(target, px, py, dmg); !absorbed {
			w.applyDamageToEntityProfile(target, remaining, attackDamageApplyProfile(src))
			applySlow(target, src.AttackSlowSec*scale, src.AttackSlowMul)
			w.applyStatusToEntity(target, src.AttackStatusID, src.AttackStatusName, src.AttackStatusDuration)
		}
		hit[next] = struct{}{}
		prev = next
	}
}

func (w *World) applyDamageToEntity(e *RawEntity, dmg float32) {
	w.applyDamageToEntityDetailed(e, dmg, false)
}

func (w *World) getBuildingWeaponProfile(blockID int16) (buildingWeaponProfile, bool) {
	if w != nil && w.buildingProfileCacheStep && blockID > 0 {
		if idx := int(blockID); idx < len(w.buildingProfileBlockState) {
			switch w.buildingProfileBlockState[idx] {
			case 1:
				return w.buildingProfilesByBlock[idx], true
			case 2:
				return buildingWeaponProfile{}, false
			}
		}
	}
	name := w.blockNameByID(blockID)
	if name == "" {
		return buildingWeaponProfile{}, false
	}
	return w.buildingWeaponProfileByNameLocked(name)
}

func (w *World) findNearestEnemyBuilding(src RawEntity, rangeLimit float32) (int32, float32, float32, bool) {
	if w.model == nil || src.Team == 0 {
		return 0, 0, 0, false
	}
	bestDist2 := rangeLimit * rangeLimit
	bestPos := int32(0)
	var bestX, bestY float32
	found := false
	visitPos := func(pos int32) {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			return
		}
		t := &w.model.Tiles[pos]
		if t.Build == nil || t.Build.Health <= 0 {
			return
		}
		if t.Build.Team == src.Team {
			return
		}
		tx := float32(t.X*8 + 4)
		ty := float32(t.Y*8 + 4)
		dx := tx - src.X
		dy := ty - src.Y
		d2 := dx*dx + dy*dy
		if d2 > bestDist2 {
			return
		}
		bestDist2 = d2
		bestPos = pos
		bestX = tx
		bestY = ty
		found = true
	}
	w.forEachEnemyBuildingInRange(src.Team, src.X, src.Y, rangeLimit, visitPos)
	if !found {
		return 0, 0, 0, false
	}
	return bestPos, bestX, bestY, true
}

func (w *World) applyDamageToBuilding(pos int32, damage float32) bool {
	return w.applyDamageToBuildingDetailed(pos, damage)
}

func (w *World) applyDamageToBuildingRaw(pos int32, damage float32) bool {
	if w.model == nil || damage <= 0 {
		return false
	}
	x := int(pos) % w.model.Width
	y := int(pos) / w.model.Width
	if !w.model.InBounds(x, y) {
		return false
	}
	t := &w.model.Tiles[y*w.model.Width+x]
	if t.Build == nil {
		return false
	}
	prevBlock := int16(t.Block)
	prevBlockName := w.blockNameByID(prevBlock)
	t.Build.Health -= damage
	if t.Build.Health > 0 {
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:     EntityEventBuildHealth,
			BuildPos: packTilePos(x, y),
			BuildHP:  t.Build.Health,
		})
		return true
	}
	team := t.Team
	powerRelevant := w.isPowerRelevantBuildingLocked(t)
	w.queueBrokenBuildPlanLocked(pos, t)
	w.removeActiveTileIndexLocked(pos, t)
	w.setBuildingOccupancyLocked(pos, t, false)
	t.Build = nil
	t.Block = 0
	t.Team = 0
	t.Rotation = 0
	delete(w.buildStates, pos)
	w.clearBuildingRuntimeLocked(pos)
	if powerRelevant {
		w.invalidatePowerNetsLocked()
	}
	if affectsCoreStorageLinks(prevBlockName) {
		w.refreshCoreStorageLinksLocked()
	}
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventBuildDestroyed,
		BuildPos:   packTilePos(x, y),
		BuildTeam:  team,
		BuildBlock: prevBlock,
	})
	return true
}
