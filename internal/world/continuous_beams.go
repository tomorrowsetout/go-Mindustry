package world

import (
	"math"
	"strings"
)

func normalizeBulletClassName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

func isPointLaserBulletClass(name string) bool {
	return normalizeBulletClassName(name) == "pointlaserbullettype"
}

func isContinuousLineBeamBulletClass(name string) bool {
	switch normalizeBulletClassName(name) {
	case "continuousbullettype", "continuouslaserbullettype":
		return true
	default:
		return false
	}
}

func isPersistentBeamBulletClass(name string) bool {
	return isPointLaserBulletClass(name) || isContinuousLineBeamBulletClass(name)
}

func isPersistentBeamBulletProfile(profile *bulletRuntimeProfile) bool {
	if profile == nil {
		return false
	}
	return isPersistentBeamBulletClass(profile.ClassName)
}

func (w *World) findBulletIndexByID(id int32) int {
	if w == nil || id == 0 {
		return -1
	}
	for i := range w.bullets {
		if w.bullets[i].ID == id {
			return i
		}
	}
	return -1
}

func (w *World) spawnPersistentBeamBullet(src RawEntity, bullet *bulletRuntimeProfile, angle, aimX, aimY float32, sourceIsBuilding bool) int32 {
	if w == nil || bullet == nil || !isPersistentBeamBulletProfile(bullet) {
		return 0
	}
	lifeSec := bullet.Lifetime
	if lifeSec <= 0 {
		lifeSec = src.AttackBulletLifetime
	}
	if lifeSec <= 0 {
		lifeSec = 16.0 / 60.0
	}
	damageInterval := bullet.DamageInterval
	if damageInterval <= 0 {
		damageInterval = 5.0 / 60.0
	}
	beamLength := bullet.Length
	if beamLength <= 0 {
		beamLength = src.AttackRange
	}
	scale := w.outgoingDamageScale(src, sourceIsBuilding)
	rad := float32(angle * math.Pi / 180)
	b := simBullet{
		ID:                   w.bulletNextID,
		Team:                 src.Team,
		X:                    src.X,
		Y:                    src.Y,
		VX:                   float32(math.Cos(float64(rad))),
		VY:                   float32(math.Sin(float64(rad))),
		Damage:               src.AttackDamage * scale,
		SplashDamage:         src.AttackSplashDamage * scale,
		LifeSec:              lifeSec,
		Radius:               maxf(src.AttackBulletHitSize*0.5, 4),
		HitUnits:             true,
		HitBuilds:            src.AttackBuildings,
		BulletType:           src.AttackBulletType,
		BulletClass:          bullet.ClassName,
		SplashRadius:         src.AttackSplashRadius,
		BuildingDamage:       entityBuildingDamageMultiplier(src),
		SlowSec:              src.AttackSlowSec,
		SlowMul:              clampf(src.AttackSlowMul, 0.2, 1),
		StatusID:             src.AttackStatusID,
		StatusName:           src.AttackStatusName,
		StatusDuration:       src.AttackStatusDuration,
		ShootEffect:          src.AttackShootEffect,
		SmokeEffect:          src.AttackSmokeEffect,
		HitEffect:            src.AttackHitEffect,
		DespawnEffect:        src.AttackDespawnEffect,
		TargetAir:            src.AttackTargetAir,
		TargetGround:         src.AttackTargetGround,
		TargetPriority:       src.AttackTargetPriority,
		AimX:                 aimX,
		AimY:                 aimY,
		BeamLength:           beamLength,
		BeamDamageInterval:   damageInterval,
		BeamOptimalLifeFract: bullet.OptimalLifeFract,
		BeamFadeTime:         bullet.FadeTime,
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
	return b.ID
}

func currentPersistentBeamLength(b simBullet) float32 {
	length := b.BeamLength
	if length <= 0 {
		return 0
	}
	if isContinuousLineBeamBulletClass(b.BulletClass) && b.BeamFadeTime > 0 && b.AgeSec > b.LifeSec-b.BeamFadeTime {
		fout := 1 - (b.AgeSec-(b.LifeSec-b.BeamFadeTime))/b.BeamFadeTime
		length *= clampf(fout, 0, 1)
	}
	return length
}

func beamEndPosition(b simBullet) (float32, float32) {
	if isPointLaserBulletClass(b.BulletClass) {
		return b.AimX, b.AimY
	}
	length := currentPersistentBeamLength(b)
	angle := float32(math.Atan2(float64(b.VY), float64(b.VX)))
	return b.X + float32(math.Cos(float64(angle)))*length, b.Y + float32(math.Sin(float64(angle)))*length
}

func beamImpactRotation(b simBullet) float32 {
	tx, ty := beamEndPosition(b)
	return lookAt(b.X, b.Y, tx, ty)
}

func distPointToSegmentSquared(px, py, ax, ay, bx, by float32) float32 {
	abx := bx - ax
	aby := by - ay
	apx := px - ax
	apy := py - ay
	ab2 := abx*abx + aby*aby
	if ab2 <= 0 {
		dx := px - ax
		dy := py - ay
		return dx*dx + dy*dy
	}
	t := (apx*abx + apy*aby) / ab2
	t = clampf(t, 0, 1)
	cx := ax + abx*t
	cy := ay + aby*t
	dx := px - cx
	dy := py - cy
	return dx*dx + dy*dy
}

func bulletCanHitEntity(b simBullet, e RawEntity) bool {
	if e.Health <= 0 || e.Team == b.Team {
		return false
	}
	if isEntityFlying(e) {
		return b.TargetAir
	}
	return b.TargetGround
}

func (w *World) applyPointBeamDamage(b simBullet) bool {
	if w == nil || w.model == nil {
		return false
	}
	impacted := false
	if b.HitBuilds && b.TargetGround {
		tx := int(math.Floor(float64(b.AimX / 8)))
		ty := int(math.Floor(float64(b.AimY / 8)))
		if w.model.InBounds(tx, ty) {
			pos := int32(ty*w.model.Width + tx)
			tile := &w.model.Tiles[pos]
			if tile.Build != nil && tile.Build.Team != b.Team && tile.Build.Health > 0 {
				if w.applyDamageToBuildingDetailed(pos, b.Damage*b.BuildingDamage) {
					impacted = true
				}
			}
		}
	}
	for i := range w.model.Entities {
		e := &w.model.Entities[i]
		if !bulletCanHitEntity(b, *e) {
			continue
		}
		hitR := maxf(e.HitRadius, entityHitRadiusForType(e.TypeID))
		dx := e.X - b.AimX
		dy := e.Y - b.AimY
		if dx*dx+dy*dy > hitR*hitR {
			continue
		}
		w.applyDamageToEntityDetailed(e, b.Damage, false)
		applySlow(e, b.SlowSec, b.SlowMul)
		w.applyStatusToEntity(e, b.StatusID, b.StatusName, b.StatusDuration)
		impacted = true
	}
	return impacted
}

func (w *World) applyLineBeamDamage(b simBullet) bool {
	if w == nil || w.model == nil {
		return false
	}
	ax, ay := b.X, b.Y
	bx, by := beamEndPosition(b)
	impacted := false
	for i := range w.model.Entities {
		e := &w.model.Entities[i]
		if !bulletCanHitEntity(b, *e) {
			continue
		}
		hitR := maxf(e.HitRadius, entityHitRadiusForType(e.TypeID))
		if distPointToSegmentSquared(e.X, e.Y, ax, ay, bx, by) > hitR*hitR {
			continue
		}
		w.applyDamageToEntityDetailed(e, b.Damage, false)
		applySlow(e, b.SlowSec, b.SlowMul)
		w.applyStatusToEntity(e, b.StatusID, b.StatusName, b.StatusDuration)
		impacted = true
	}
	if b.HitBuilds && b.TargetGround {
		minTileX := int(math.Floor(float64(minf(ax, bx)/8))) - 3
		maxTileX := int(math.Ceil(float64(maxf(ax, bx)/8))) + 3
		minTileY := int(math.Floor(float64(minf(ay, by)/8))) - 3
		maxTileY := int(math.Ceil(float64(maxf(ay, by)/8))) + 3
		seen := map[int32]struct{}{}
		for ty := minTileY; ty <= maxTileY; ty++ {
			for tx := minTileX; tx <= maxTileX; tx++ {
				if !w.model.InBounds(tx, ty) {
					continue
				}
				pos := int32(ty*w.model.Width + tx)
				tile := &w.model.Tiles[pos]
				if tile.Build == nil || tile.Build.Team == b.Team || tile.Build.Health <= 0 {
					continue
				}
				key := packTilePos(tile.Build.X, tile.Build.Y)
				if _, exists := seen[key]; exists {
					continue
				}
				seen[key] = struct{}{}
				cx := float32(tile.Build.X*8 + 4)
				cy := float32(tile.Build.Y*8 + 4)
				size := w.blockSizeForTileLocked(tile)
				if size <= 0 {
					size = 1
				}
				hitR := float32(size) * 4
				if distPointToSegmentSquared(cx, cy, ax, ay, bx, by) > hitR*hitR {
					continue
				}
				if w.applyDamageToBuildingDetailed(pos, b.Damage*b.BuildingDamage) {
					impacted = true
				}
			}
		}
	}
	return impacted
}

func (w *World) stepPersistentBeamBullet(b *simBullet, dt float32) (bool, bool) {
	if w == nil || b == nil {
		return false, true
	}
	if b.BeamDamageInterval <= 0 {
		b.BeamDamageInterval = 5.0 / 60.0
	}
	if b.KeepAlive {
		b.AgeSec = b.LifeSec * clampf(b.BeamOptimalLifeFract, 0, 1)
	} else {
		b.AgeSec += dt
	}
	impacted := false
	b.DamageTick += dt
	for b.BeamDamageInterval > 0 && b.DamageTick >= b.BeamDamageInterval {
		b.DamageTick -= b.BeamDamageInterval
		if isPointLaserBulletClass(b.BulletClass) {
			impacted = w.applyPointBeamDamage(*b) || impacted
		} else {
			impacted = w.applyLineBeamDamage(*b) || impacted
		}
	}
	expired := b.AgeSec >= b.LifeSec
	b.KeepAlive = false
	return impacted, expired
}

func (w *World) acquireBuildingWeaponTarget(src RawEntity, state *buildCombatState, prof buildingWeaponProfile, ents []RawEntity, idToIndex map[int32]int, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) (int, int32, float32, float32) {
	if state == nil {
		return -1, -1, 0, 0
	}
	track := targetTrackState{TargetID: state.TargetID, RetargetCD: state.RetargetCD}
	retargetDelay := maxf(prof.Interval*0.55, 0.22)
	if tid, ok := w.acquireTrackedEntityTarget(src, ents, idToIndex, spatial, teamSpatial, prof.Range, prof.TargetAir, prof.TargetGround, prof.TargetPriority, &track, 0, retargetDelay); ok {
		state.TargetID = track.TargetID
		state.RetargetCD = track.RetargetCD
		if idx, exists := idToIndex[tid]; exists && idx >= 0 && idx < len(ents) {
			return idx, -1, ents[idx].X, ents[idx].Y
		}
	}
	state.TargetID = track.TargetID
	state.RetargetCD = track.RetargetCD
	if prof.HitBuildings {
		if bpos, tx, ty, ok := w.findNearestEnemyBuilding(src, prof.Range); ok {
			return -1, bpos, tx, ty
		}
	}
	return -1, -1, 0, 0
}

func (w *World) updateBuildingContinuousBeam(src *RawEntity, state *buildCombatState, prof buildingWeaponProfile, hasTarget bool, tx, ty, dt float32) bool {
	if w == nil || src == nil || state == nil || state.BeamBulletID == 0 {
		return false
	}
	idx := w.findBulletIndexByID(state.BeamBulletID)
	if idx < 0 || idx >= len(w.bullets) {
		state.BeamBulletID = 0
		state.BeamHoldRemain = 0
		return false
	}
	b := &w.bullets[idx]
	angle := src.Rotation
	if hasTarget {
		angle = lookAt(src.X, src.Y, tx, ty)
		src.Rotation = angle
	}
	rad := float32(angle * math.Pi / 180)
	b.X = src.X
	b.Y = src.Y
	b.VX = float32(math.Cos(float64(rad)))
	b.VY = float32(math.Sin(float64(rad)))

	if isPointLaserBulletClass(b.BulletClass) {
		targetLength := state.BeamLastLength
		if hasTarget {
			targetLength = float32(math.Sqrt(float64((tx-src.X)*(tx-src.X) + (ty-src.Y)*(ty-src.Y))))
			if prof.Range > 0 && targetLength > prof.Range {
				targetLength = prof.Range
			}
			if prof.AimChangeSpeed > 0 {
				targetLength = approachValue(state.BeamLastLength, targetLength, prof.AimChangeSpeed*dt*60)
			}
			state.BeamLastLength = targetLength
		}
		b.AimX = src.X + float32(math.Cos(float64(rad)))*targetLength
		b.AimY = src.Y + float32(math.Sin(float64(rad)))*targetLength
	} else {
		length := b.BeamLength
		if length <= 0 {
			length = prof.Range
		}
		b.AimX = src.X + float32(math.Cos(float64(rad)))*length
		b.AimY = src.Y + float32(math.Sin(float64(rad)))*length
	}

	className := normalizeBulletClassName(prof.ClassName)
	keepAlive := false
	switch className {
	case "laserturret":
		if state.BeamHoldRemain > 0 {
			keepAlive = true
			state.BeamHoldRemain -= dt
			if state.BeamHoldRemain < 0 {
				state.BeamHoldRemain = 0
			}
			state.Cooldown = maxf(prof.Interval, 0.05)
		}
	case "continuousturret", "continuousliquidturret":
		keepAlive = hasTarget
		state.Cooldown = 0
	}
	if keepAlive {
		b.KeepAlive = true
	}
	return true
}

func (w *World) stepBuildingContinuousBeam(src *RawEntity, state *buildCombatState, prof buildingWeaponProfile, ents []RawEntity, idToIndex map[int32]int, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex, dt float32) bool {
	if w == nil || src == nil || state == nil || !prof.ContinuousHold || !isPersistentBeamBulletProfile(prof.Bullet) {
		return false
	}
	targetIdx, buildPos, tx, ty := w.acquireBuildingWeaponTarget(*src, state, prof, ents, idToIndex, spatial, teamSpatial)
	hasTarget := targetIdx >= 0 || buildPos >= 0
	if state.BeamBulletID != 0 {
		w.updateBuildingContinuousBeam(src, state, prof, hasTarget, tx, ty, dt)
		if state.BeamBulletID != 0 {
			return hasTarget || normalizeBulletClassName(prof.ClassName) == "laserturret"
		}
	}
	if !hasTarget || state.Cooldown > 0 {
		return false
	}
	if prof.AmmoPerShot > 0 && state.Ammo < prof.AmmoPerShot {
		return false
	}
	if prof.PowerPerShot > 0 && state.Power < prof.PowerPerShot {
		return false
	}

	src.Rotation = lookAt(src.X, src.Y, tx, ty)
	bid := w.spawnPersistentBeamBullet(*src, prof.Bullet, src.Rotation, tx, ty, true)
	if bid == 0 {
		return false
	}
	state.BeamBulletID = bid
	if normalizeBulletClassName(prof.ClassName) == "laserturret" {
		state.BeamHoldRemain = prof.ShootDuration
		state.Cooldown = maxf(prof.Interval, 0.05)
	} else {
		state.BeamHoldRemain = 0
		state.Cooldown = 0
	}
	if prof.AmmoPerShot > 0 {
		state.Ammo -= prof.AmmoPerShot
		if state.Ammo < 0 {
			state.Ammo = 0
		}
	}
	if prof.PowerPerShot > 0 {
		state.Power -= prof.PowerPerShot
		if state.Power < 0 {
			state.Power = 0
		}
	}
	w.updateBuildingContinuousBeam(src, state, prof, true, tx, ty, dt)
	return true
}
