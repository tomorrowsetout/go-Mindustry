package world

import (
	"math"
	"math/rand"
	"strings"
)

func cloneUnitMountProfile(src unitWeaponMountProfile) unitWeaponMountProfile {
	out := src
	out.FragmentBullet = cloneBulletRuntimeProfile(src.FragmentBullet)
	out.Bullet = cloneBulletRuntimeProfile(src.Bullet)
	return out
}

func cloneUnitMountProfilesByName(src map[string][]unitWeaponMountProfile) map[string][]unitWeaponMountProfile {
	out := make(map[string][]unitWeaponMountProfile, len(src))
	for k, mounts := range src {
		if len(mounts) == 0 {
			out[k] = nil
			continue
		}
		clone := make([]unitWeaponMountProfile, len(mounts))
		for i := range mounts {
			clone[i] = cloneUnitMountProfile(mounts[i])
		}
		out[k] = clone
	}
	return out
}

func convertVanillaMountProfile(src vanillaWeaponMountProfile) unitWeaponMountProfile {
	return unitWeaponMountProfile{
		ClassName:            src.ClassName,
		FireMode:             src.FireMode,
		Range:                src.Range,
		Damage:               src.Damage,
		SplashDamage:         src.SplashDamage,
		Interval:             src.Interval,
		BulletType:           src.BulletType,
		BulletSpeed:          src.BulletSpeed,
		BulletLifetime:       src.BulletLifetime,
		BulletHitSize:        src.BulletHitSize,
		SplashRadius:         src.SplashRadius,
		BuildingDamage:       src.BuildingDamageMultiplier,
		ArmorMultiplier:      src.ArmorMultiplier,
		MaxDamageFraction:    src.MaxDamageFraction,
		ShieldDamageMul:      src.ShieldDamageMultiplier,
		PierceDamageFactor:   src.PierceDamageFactor,
		PierceArmor:          src.PierceArmor,
		Pierce:               src.Pierce,
		PierceBuilding:       src.PierceBuilding,
		StatusID:             src.StatusID,
		StatusName:           strings.ToLower(strings.TrimSpace(src.StatusName)),
		StatusDuration:       src.StatusDuration,
		FragmentCount:        src.FragBullets,
		FragmentSpread:       src.FragSpread,
		FragmentRandomSpread: src.FragRandomSpread,
		FragmentAngle:        src.FragAngle,
		FragmentVelocityMin:  src.FragVelocityMin,
		FragmentVelocityMax:  src.FragVelocityMax,
		FragmentLifeMin:      src.FragLifeMin,
		FragmentLifeMax:      src.FragLifeMax,
		FragmentBullet:       convertVanillaBulletProfile(src.FragBullet),
		TargetAir:            src.TargetAir,
		TargetGround:         src.TargetGround,
		HitBuildings:         src.HitBuildings,
		PreferBuildings:      src.PreferBuildings,
		HitRadius:            src.HitRadius,
		ShootStatusID:        src.ShootStatusID,
		ShootStatusName:      strings.ToLower(strings.TrimSpace(src.ShootStatusName)),
		ShootStatusDuration:  src.ShootStatusDuration,
		ShootEffect:          src.ShootEffect,
		SmokeEffect:          src.SmokeEffect,
		HitEffect:            src.HitEffect,
		DespawnEffect:        src.DespawnEffect,
		Bullet:               convertVanillaBulletProfile(src.Bullet),
		X:                    src.X,
		Y:                    src.Y,
		ShootX:               src.ShootX,
		ShootY:               src.ShootY,
		Rotate:               src.Rotate,
		RotateSpeed:          src.RotateSpeed,
		BaseRotation:         src.BaseRotation,
		Mirror:               src.Mirror,
		Alternate:            src.Alternate,
		FlipSprite:           src.FlipSprite,
		OtherSide:            src.OtherSide,
		Controllable:         src.Controllable,
		AIControllable:       src.AIControllable,
		AutoTarget:           src.AutoTarget,
		PredictTarget:        src.PredictTarget,
		UseAttackRange:       src.UseAttackRange,
		AlwaysShooting:       src.AlwaysShooting,
		NoAttack:             src.NoAttack,
		TargetInterval:       src.TargetInterval,
		TargetSwitchInterval: src.TargetSwitchInterval,
		ShootCone:            src.ShootCone,
		MinShootVelocity:     src.MinShootVelocity,
		Inaccuracy:           src.Inaccuracy,
		VelocityRnd:          src.VelocityRnd,
		XRand:                src.XRand,
		YRand:                src.YRand,
		ExtraVelocity:        src.ExtraVelocity,
		RotationLimit:        src.RotationLimit,
		MinWarmup:            src.MinWarmup,
		ShootWarmupSpeed:     src.ShootWarmupSpeed,
		LinearWarmup:         src.LinearWarmup,
		AimChangeSpeed:       src.AimChangeSpeed,
		Continuous:           src.Continuous,
		AlwaysContinuous:     src.AlwaysContinuous,
		PointDefense:         src.PointDefense,
		RepairBeam:           src.RepairBeam,
		TargetUnits:          src.TargetUnits,
		TargetBuildings:      src.TargetBuildings,
		RepairSpeed:          src.RepairSpeed,
		FractionRepairSpeed:  src.FractionRepairSpeed,
		ShootPattern:         src.ShootPattern,
		ShootShots:           src.ShootShots,
		ShootFirstShotDelay:  src.ShootFirstShotDelay,
		ShootShotDelay:       src.ShootShotDelay,
		ShootSpread:          src.ShootSpread,
		ShootBarrels:         src.ShootBarrels,
		ShootBarrelOffset:    src.ShootBarrelOffset,
		ShootPatternMirror:   src.ShootPatternMirror,
		ShootHelixScl:        src.ShootHelixScl,
		ShootHelixMag:        src.ShootHelixMag,
		ShootHelixOffset:     src.ShootHelixOffset,
	}
}

func (w *World) unitMountProfilesForEntity(e RawEntity) []unitWeaponMountProfile {
	if w == nil {
		return nil
	}
	if name, ok := w.unitNamesByID[e.TypeID]; ok && name != "" {
		if mounts, exists := w.unitMountProfilesByName[name]; exists && len(mounts) > 0 {
			return mounts
		}
	}
	return nil
}

func (w *World) ensureUnitMountStates(id int32, mounts []unitWeaponMountProfile) []unitMountState {
	if len(mounts) == 0 {
		return nil
	}
	states := w.unitMountStates[id]
	if len(states) == len(mounts) {
		return states
	}
	states = make([]unitMountState, len(mounts))
	for i := range mounts {
		states[i].Rotation = mounts[i].BaseRotation
		states[i].TargetRotation = mounts[i].BaseRotation
		states[i].TargetID = 0
		states[i].TargetBuildPos = -1
	}
	return states
}

func applyMountWeaponProfile(src *RawEntity, mount unitWeaponMountProfile) {
	if src == nil {
		return
	}
	src.AttackFireMode = mount.FireMode
	src.AttackRange = mount.Range
	src.AttackDamage = mount.Damage
	src.AttackSplashDamage = mount.SplashDamage
	src.AttackInterval = mount.Interval
	src.AttackBulletType = mount.BulletType
	src.AttackBulletSpeed = mount.BulletSpeed
	src.AttackBulletLifetime = mount.BulletLifetime
	src.AttackBulletHitSize = mount.BulletHitSize
	src.AttackSplashRadius = mount.SplashRadius
	src.AttackBuildingDamage = mount.BuildingDamage
	src.AttackBuildingDamageSet = true
	src.AttackArmorMultiplier = mount.ArmorMultiplier
	src.AttackMaxDamageFraction = mount.MaxDamageFraction
	src.AttackShieldDamageMul = mount.ShieldDamageMul
	src.AttackPierceDamageFactor = mount.PierceDamageFactor
	src.AttackPierceArmor = mount.PierceArmor
	src.AttackSlowSec = mount.SlowSec
	src.AttackSlowMul = mount.SlowMul
	src.AttackPierce = mount.Pierce
	src.AttackPierceBuilding = mount.PierceBuilding
	src.AttackChainCount = mount.ChainCount
	src.AttackChainRange = mount.ChainRange
	src.AttackStatusID = mount.StatusID
	src.AttackStatusName = mount.StatusName
	src.AttackStatusDuration = mount.StatusDuration
	src.AttackShootStatusID = mount.ShootStatusID
	src.AttackShootStatusName = mount.ShootStatusName
	src.AttackShootStatusDur = mount.ShootStatusDuration
	src.AttackFragmentCount = mount.FragmentCount
	src.AttackFragmentSpread = mount.FragmentSpread
	src.AttackFragmentRand = mount.FragmentRandomSpread
	src.AttackFragmentAngle = mount.FragmentAngle
	src.AttackFragmentVelMin = mount.FragmentVelocityMin
	src.AttackFragmentVelMax = mount.FragmentVelocityMax
	src.AttackFragmentLifeMin = mount.FragmentLifeMin
	src.AttackFragmentLifeMax = mount.FragmentLifeMax
	src.AttackFragmentBullet = cloneBulletRuntimeProfile(mount.FragmentBullet)
	src.AttackShootEffect = mount.ShootEffect
	src.AttackSmokeEffect = mount.SmokeEffect
	src.AttackHitEffect = mount.HitEffect
	src.AttackDespawnEffect = mount.DespawnEffect
	src.AttackPreferBuildings = mount.PreferBuildings
	src.AttackTargetAir = mount.TargetAir
	src.AttackTargetGround = mount.TargetGround
	src.AttackTargetPriority = mount.TargetPriority
	src.AttackBuildings = mount.HitBuildings
	if mount.HitRadius > 0 {
		src.HitRadius = mount.HitRadius
	}
}

func (w *World) applyMountShootStatus(e *RawEntity, mount unitWeaponMountProfile) {
	if e == nil {
		return
	}
	w.applyStatusToEntity(e, mount.ShootStatusID, mount.ShootStatusName, mount.ShootStatusDuration)
}

func trnsx(rotation, x, y float32) float32 {
	rad := float64(rotation) * math.Pi / 180
	return x*float32(math.Cos(rad)) - y*float32(math.Sin(rad))
}

func trnsy(rotation, x, y float32) float32 {
	rad := float64(rotation) * math.Pi / 180
	return x*float32(math.Sin(rad)) + y*float32(math.Cos(rad))
}

func unitMountBasePosition(e RawEntity, mount unitWeaponMountProfile) (float32, float32) {
	rotation := e.Rotation
	return e.X + trnsx(rotation, mount.X, mount.Y), e.Y + trnsy(rotation, mount.X, mount.Y)
}

func unitMountShootPosition(e RawEntity, mount unitWeaponMountProfile, state unitMountState) (float32, float32, float32) {
	mx, my := unitMountBasePosition(e, mount)
	weaponRotation := e.Rotation + mount.BaseRotation
	if mount.Rotate {
		weaponRotation = e.Rotation + state.Rotation
	}
	return mx + trnsx(weaponRotation, mount.ShootX, mount.ShootY), my + trnsy(weaponRotation, mount.ShootX, mount.ShootY), weaponRotation
}

func normalizeAngleDeg(v float32) float32 {
	for v <= -180 {
		v += 360
	}
	for v > 180 {
		v -= 360
	}
	return v
}

func angleDistDeg(a, b float32) float32 {
	diff := normalizeAngleDeg(a - b)
	if diff < 0 {
		return -diff
	}
	return diff
}

func moveAngleToward(current, target, amount float32) float32 {
	diff := normalizeAngleDeg(target - current)
	if diff > amount {
		diff = amount
	} else if diff < -amount {
		diff = -amount
	}
	return normalizeAngleDeg(current + diff)
}

func angleWithin(current, target, cone float32) bool {
	return angleDistDeg(current, target) <= cone
}

func entityVelocityLen(e RawEntity) float32 {
	return float32(math.Sqrt(float64(e.VelX*e.VelX + e.VelY*e.VelY)))
}

func (w *World) updateMountAim(e RawEntity, mount unitWeaponMountProfile, state *unitMountState, aimX, aimY, dt float32) {
	if state == nil {
		return
	}
	state.AimX = aimX
	state.AimY = aimY
	if mount.Rotate {
		axisX, axisY := unitMountBasePosition(e, mount)
		state.TargetRotation = normalizeAngleDeg(lookAt(axisX, axisY, aimX, aimY) - e.Rotation)
		speed := mount.RotateSpeed
		if speed <= 0 {
			speed = 20
		}
		state.Rotation = moveAngleToward(state.Rotation, state.TargetRotation, speed*dt*60)
		if mount.RotationLimit > 0 && mount.RotationLimit < 360 {
			dst := angleDistDeg(state.Rotation, mount.BaseRotation)
			limit := mount.RotationLimit * 0.5
			if dst > limit {
				state.Rotation = moveAngleToward(state.Rotation, mount.BaseRotation, dst-limit)
			}
		}
	} else {
		state.Rotation = mount.BaseRotation
		state.TargetRotation = lookAt(e.X, e.Y, aimX, aimY)
	}
}

func randRange(v float32) float32 {
	if v == 0 {
		return 0
	}
	return (rand.Float32()*2 - 1) * v
}

func approachValue(current, target, amount float32) float32 {
	if current < target {
		current += amount
		if current > target {
			return target
		}
		return current
	}
	current -= amount
	if current < target {
		return target
	}
	return current
}

func warmupToward(current, target, speed float32, linear bool, dt float32) float32 {
	if speed <= 0 {
		speed = 0.1
	}
	amount := speed * dt * 60
	if linear {
		return clampf(approachValue(current, target, amount), 0, 1)
	}
	return clampf(current+(target-current)*amount, 0, 1)
}

func buildMountShotPattern(mount unitWeaponMountProfile, state *unitMountState) []pendingMountShot {
	if state == nil {
		return nil
	}
	shots := mount.ShootShots
	if shots <= 0 {
		shots = 1
	}
	switch mount.ShootPattern {
	case "spread":
		out := make([]pendingMountShot, 0, shots)
		for i := int32(0); i < shots; i++ {
			angleOffset := float32(0)
			if shots > 1 {
				angleOffset = float32(i)*mount.ShootSpread - float32(shots-1)*mount.ShootSpread*0.5
			}
			out = append(out, pendingMountShot{
				DelaySec:    mount.ShootFirstShotDelay + mount.ShootShotDelay*float32(i),
				AngleOffset: angleOffset,
			})
		}
		return out
	case "alternate":
		barrels := mount.ShootBarrels
		if barrels <= 0 {
			barrels = 2
		}
		sign := float32(1)
		if mount.ShootPatternMirror {
			sign = -1
		}
		out := make([]pendingMountShot, 0, shots)
		for i := int32(0); i < shots; i++ {
			index := (state.BarrelCounter + i + mount.ShootBarrelOffset) % barrels
			spreadIndex := float32(index) - float32(barrels-1)/2
			out = append(out, pendingMountShot{
				DelaySec: mount.ShootFirstShotDelay + mount.ShootShotDelay*float32(i),
				XOffset:  spreadIndex * mount.ShootSpread * sign,
			})
			state.BarrelCounter++
		}
		return out
	case "helix":
		if mount.ShootHelixScl <= 0 {
			mount.ShootHelixScl = 2
		}
		if mount.ShootHelixMag == 0 {
			mount.ShootHelixMag = 1.5
		}
		if mount.ShootHelixOffset == 0 {
			mount.ShootHelixOffset = float32(math.Pi * 1.25)
		}
		out := make([]pendingMountShot, 0, shots*2)
		for i := int32(0); i < shots; i++ {
			delay := mount.ShootFirstShotDelay + mount.ShootShotDelay*float32(i)
			out = append(out,
				pendingMountShot{
					DelaySec:    delay,
					HelixScl:    mount.ShootHelixScl,
					HelixMag:    mount.ShootHelixMag,
					HelixOffset: mount.ShootHelixOffset,
				},
				pendingMountShot{
					DelaySec:    delay,
					HelixScl:    mount.ShootHelixScl,
					HelixMag:    -mount.ShootHelixMag,
					HelixOffset: mount.ShootHelixOffset,
				},
			)
		}
		return out
	default:
		out := make([]pendingMountShot, 0, shots)
		for i := int32(0); i < shots; i++ {
			out = append(out, pendingMountShot{
				DelaySec: mount.ShootFirstShotDelay + mount.ShootShotDelay*float32(i),
			})
		}
		return out
	}
}

func (w *World) queueMountShot(entityID int32, mountIndex int, shot pendingMountShot) {
	if w == nil {
		return
	}
	shot.EntityID = entityID
	shot.MountIndex = mountIndex
	w.pendingMountShots = append(w.pendingMountShots, shot)
}

func (w *World) triggerEntityMountFire(e *RawEntity, mountIndex int, mount unitWeaponMountProfile, state *unitMountState, idToIndex map[int32]int) bool {
	if e == nil || state == nil {
		return false
	}
	shots := buildMountShotPattern(mount, state)
	if len(shots) == 0 {
		return false
	}
	w.applyMountShootStatus(e, mount)
	fired := false
	for _, shot := range shots {
		if shot.DelaySec <= 0 {
			if w.fireEntityMountShot(e, mount, state, shot, idToIndex) {
				fired = true
			}
			continue
		}
		w.queueMountShot(e.ID, mountIndex, shot)
		fired = true
	}
	return fired
}

func (w *World) fireEntityMountShot(e *RawEntity, mount unitWeaponMountProfile, state *unitMountState, shot pendingMountShot, idToIndex map[int32]int) bool {
	if e == nil || state == nil {
		return false
	}
	src := *e
	applyMountWeaponProfile(&src, mount)

	sx, sy, weaponRotation := unitMountShootPosition(*e, mount, *state)
	xOffset := shot.XOffset + randRange(mount.XRand)
	yOffset := shot.YOffset + randRange(mount.YRand)
	sx += trnsx(weaponRotation, xOffset, yOffset)
	sy += trnsy(weaponRotation, xOffset, yOffset)

	aimX, aimY := state.AimX, state.AimY
	var target *RawEntity
	targetIdx := -1
	if state.TargetID != 0 {
		if idx, ok := idToIndex[state.TargetID]; ok && idx >= 0 && idx < len(w.model.Entities) {
			target = &w.model.Entities[idx]
			if target.Health > 0 {
				targetIdx = idx
				aimX = target.X
				aimY = target.Y
			} else {
				target = nil
			}
		}
	}

	buildPos := state.TargetBuildPos
	if target == nil && buildPos >= 0 && w.model != nil {
		x := int(buildPos) % w.model.Width
		y := int(buildPos) / w.model.Width
		if w.model.InBounds(x, y) {
			if build := w.model.Tiles[buildPos].Build; build != nil && build.Health > 0 && build.Team != e.Team {
				aimX = float32(x*8 + 4)
				aimY = float32(y*8 + 4)
			} else {
				buildPos = -1
			}
		} else {
			buildPos = -1
		}
	}

	baseAngle := lookAt(sx, sy, aimX, aimY)
	if mount.Rotate {
		baseAngle = e.Rotation + state.Rotation
	}
	angle := baseAngle + shot.AngleOffset + randRange(mount.Inaccuracy)
	speedScale := float32(1)
	if mount.VelocityRnd > 0 {
		speedScale = (1 - mount.VelocityRnd) + rand.Float32()*mount.VelocityRnd
	}
	speedScale += mount.ExtraVelocity
	if speedScale <= 0 {
		speedScale = 0.01
	}
	src.X = sx
	src.Y = sy
	src.Rotation = angle
	if !w.tryConsumeEntityAmmoLocked(e, maxf(e.AmmoPerShot, 1)) {
		return false
	}

	if src.AttackFireMode == "beam" {
		if mount.Continuous && isPersistentBeamBulletProfile(mount.Bullet) {
			bid := w.spawnPersistentBeamBullet(src, mount.Bullet, angle, aimX, aimY, false)
			if bid == 0 {
				return false
			}
			state.BeamBulletID = bid
			w.updateMountedBeamBullet(e, mount, state, 0)
			return true
		}
		if target != nil && targetIdx >= 0 {
			w.fireBeamAtEntity(src, target, targetIdx, false)
			return true
		}
		if buildPos >= 0 {
			w.fireBeamAtBuilding(src, buildPos, aimX, aimY, false)
			return true
		}
		return false
	}

	w.spawnBulletWithAngle(src, aimX, aimY, angle, speedScale, shot, false)
	return true
}

func (w *World) updateMountedBeamBullet(e *RawEntity, mount unitWeaponMountProfile, state *unitMountState, dt float32) bool {
	if w == nil || e == nil || state == nil || state.BeamBulletID == 0 {
		return false
	}
	idx := w.findBulletIndexByID(state.BeamBulletID)
	if idx < 0 || idx >= len(w.bullets) {
		state.BeamBulletID = 0
		return false
	}
	b := &w.bullets[idx]
	if !isPersistentBeamBulletClass(b.BulletClass) {
		state.BeamBulletID = 0
		return false
	}

	sx, sy, _ := unitMountShootPosition(*e, mount, *state)
	angle := lookAt(sx, sy, state.AimX, state.AimY)
	if mount.Rotate {
		angle = e.Rotation + state.Rotation
	}
	rad := float32(angle * math.Pi / 180)
	b.X = sx
	b.Y = sy
	b.VX = float32(math.Cos(float64(rad)))
	b.VY = float32(math.Sin(float64(rad)))

	rangeLimit := mount.Range
	if rangeLimit <= 0 {
		rangeLimit = e.AttackRange
	}
	if rangeLimit <= 0 {
		rangeLimit = b.BeamLength
	}
	if isPointLaserBulletClass(b.BulletClass) {
		targetLength := float32(math.Sqrt(float64((state.AimX-sx)*(state.AimX-sx) + (state.AimY-sy)*(state.AimY-sy))))
		if rangeLimit > 0 && targetLength > rangeLimit {
			targetLength = rangeLimit
		}
		if mount.AimChangeSpeed > 0 && dt > 0 {
			targetLength = approachValue(state.LastBeamLength, targetLength, mount.AimChangeSpeed*dt*60)
		}
		state.LastBeamLength = targetLength
		b.AimX = sx + float32(math.Cos(float64(rad)))*targetLength
		b.AimY = sy + float32(math.Sin(float64(rad)))*targetLength
	} else {
		length := b.BeamLength
		if length <= 0 {
			length = rangeLimit
		}
		b.AimX = sx + float32(math.Cos(float64(rad)))*length
		b.AimY = sy + float32(math.Sin(float64(rad)))*length
	}
	return true
}

func (w *World) keepMountedBeamAlive(e *RawEntity, mount unitWeaponMountProfile, state *unitMountState) {
	if w == nil || state == nil || state.BeamBulletID == 0 {
		return
	}
	idx := w.findBulletIndexByID(state.BeamBulletID)
	if idx < 0 || idx >= len(w.bullets) {
		return
	}
	b := &w.bullets[idx]
	if !isPersistentBeamBulletClass(b.BulletClass) {
		return
	}
	baseFract := b.BeamOptimalLifeFract
	if mount.Bullet != nil {
		baseFract = mount.Bullet.OptimalLifeFract
	}
	b.BeamOptimalLifeFract = clampf(baseFract*maxf(state.Warmup, 0), 0, 1)
	b.KeepAlive = true
	if e != nil {
		w.applyMountShootStatus(e, mount)
	}
}

func (w *World) stepPendingMountShots(dt float32, idToIndex map[int32]int) {
	if w == nil || len(w.pendingMountShots) == 0 || w.model == nil {
		return
	}
	out := w.pendingMountShots[:0]
	for _, shot := range w.pendingMountShots {
		shot.DelaySec -= dt
		if shot.DelaySec > 0 {
			out = append(out, shot)
			continue
		}
		idx, ok := idToIndex[shot.EntityID]
		if !ok || idx < 0 || idx >= len(w.model.Entities) {
			continue
		}
		e := &w.model.Entities[idx]
		mounts := w.unitMountProfilesForEntity(*e)
		if shot.MountIndex < 0 || shot.MountIndex >= len(mounts) {
			continue
		}
		states := w.ensureUnitMountStates(e.ID, mounts)
		if shot.MountIndex < 0 || shot.MountIndex >= len(states) {
			continue
		}
		w.fireEntityMountShot(e, mounts[shot.MountIndex], &states[shot.MountIndex], shot, idToIndex)
		w.unitMountStates[e.ID] = states
	}
	w.pendingMountShots = out
}

func (w *World) firePointDefenseMount(src RawEntity, targetIdx int) bool {
	if w == nil || targetIdx < 0 || targetIdx >= len(w.bullets) {
		return false
	}
	b := &w.bullets[targetIdx]
	if b.Team == src.Team || b.Damage <= 0 {
		return false
	}
	damage := src.AttackDamage * w.outgoingDamageScale(src, false)
	if damage <= 0 {
		return false
	}
	w.emitAttackFireEffectsLocked(src)
	if b.Damage > damage {
		b.Damage -= damage
	} else {
		b.Damage = 0
		b.SplashDamage = 0
		b.AgeSec = b.LifeSec
	}
	w.emitAttackHitEffectLocked(src, b.X, b.Y)
	return true
}

func (w *World) findPointDefenseTarget(team TeamID, x, y, rangeLimit float32) int {
	if w == nil || rangeLimit <= 0 {
		return -1
	}
	best := -1
	bestDist2 := rangeLimit * rangeLimit
	for i := range w.bullets {
		b := &w.bullets[i]
		if b.Team == team || b.Damage <= 0 {
			continue
		}
		dx := b.X - x
		dy := b.Y - y
		d2 := dx*dx + dy*dy
		if d2 > bestDist2 {
			continue
		}
		bestDist2 = d2
		best = i
	}
	return best
}

func (w *World) healEntity(e *RawEntity, amount float32) bool {
	if e == nil || amount <= 0 || e.Health <= 0 {
		return false
	}
	maxHealth := e.MaxHealth
	if hm := entityHealthMultiplier(*e); hm > 0 {
		maxHealth *= hm
	}
	if maxHealth <= 0 {
		maxHealth = e.MaxHealth
	}
	before := e.Health
	e.Health = minf(maxHealth, e.Health+amount)
	return e.Health > before
}

func (w *World) healBuilding(pos int32, amount float32) bool {
	if w == nil || w.model == nil || amount <= 0 {
		return false
	}
	x := int(pos) % w.model.Width
	y := int(pos) / w.model.Width
	if !w.model.InBounds(x, y) {
		return false
	}
	t := &w.model.Tiles[y*w.model.Width+x]
	if t.Build == nil || t.Build.Health <= 0 {
		return false
	}
	before := t.Build.Health
	t.Build.Health = minf(t.Build.MaxHealth, t.Build.Health+amount)
	if t.Build.Health > before {
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:     EntityEventBuildHealth,
			BuildPos: packTilePos(x, y),
			BuildHP:  t.Build.Health,
		})
		return true
	}
	return false
}

func (w *World) findRepairTarget(src RawEntity, mount unitWeaponMountProfile, rangeLimit float32) (int, int32, float32, float32, bool) {
	if w == nil || w.model == nil || rangeLimit <= 0 {
		return -1, 0, 0, 0, false
	}
	bestDist2 := rangeLimit * rangeLimit
	bestEntity := -1
	bestPos := int32(0)
	bestX, bestY := float32(0), float32(0)
	found := false

	if mount.TargetUnits {
		for i := range w.model.Entities {
			e := &w.model.Entities[i]
			if e.Health <= 0 || e.Team != src.Team || e.ID == src.ID {
				continue
			}
			maxHealth := e.MaxHealth
			if hm := entityHealthMultiplier(*e); hm > 0 {
				maxHealth *= hm
			}
			if e.Health >= maxHealth-0.001 {
				continue
			}
			dx := e.X - src.X
			dy := e.Y - src.Y
			d2 := dx*dx + dy*dy
			if d2 > bestDist2 {
				continue
			}
			bestDist2 = d2
			bestEntity = i
			bestPos = 0
			bestX = e.X
			bestY = e.Y
			found = true
		}
	}

	if mount.TargetBuildings {
		r := int(math.Ceil(float64(rangeLimit / 8)))
		cx := int(src.X / 8)
		cy := int(src.Y / 8)
		for ty := cy - r; ty <= cy+r; ty++ {
			for tx := cx - r; tx <= cx+r; tx++ {
				if !w.model.InBounds(tx, ty) {
					continue
				}
				pos := int32(ty*w.model.Width + tx)
				t := &w.model.Tiles[pos]
				if t.Build == nil || t.Build.Team != src.Team || t.Build.Health <= 0 || t.Build.Health >= t.Build.MaxHealth-0.001 {
					continue
				}
				wx := float32(tx*8 + 4)
				wy := float32(ty*8 + 4)
				dx := wx - src.X
				dy := wy - src.Y
				d2 := dx*dx + dy*dy
				if d2 > bestDist2 {
					continue
				}
				bestDist2 = d2
				bestEntity = -1
				bestPos = pos
				bestX = wx
				bestY = wy
				found = true
			}
		}
	}

	if !found {
		return -1, 0, 0, 0, false
	}
	return bestEntity, bestPos, bestX, bestY, true
}
