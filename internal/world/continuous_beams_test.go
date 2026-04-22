package world

import (
	"math"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func stepWorldFrames(w *World, frames int) {
	for i := 0; i < frames; i++ {
		w.Step(time.Second / 60)
	}
}

func TestContinuousMountLineBeamDamagesOverIntervals(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(64, 64)
	model.UnitNames = map[int16]string{
		1: "beam-unit",
		2: "beam-target",
	}
	model.Entities = []RawEntity{
		{
			ID:        1,
			TypeID:    1,
			Team:      1,
			X:         64,
			Y:         64,
			Rotation:  0,
			Health:    120,
			MaxHealth: 120,
			Shield:    0.1,
			ShieldMax: 0.1,
		},
		{
			ID:        2,
			TypeID:    2,
			Team:      2,
			X:         180,
			Y:         64,
			Rotation:  180,
			Health:    200,
			MaxHealth: 200,
			Shield:    0.1,
			ShieldMax: 0.1,
		},
	}
	w.SetModel(model)
	w.unitNamesByID = map[int16]string{
		1: "beam-unit",
		2: "beam-target",
	}
	w.unitProfilesByName = map[string]weaponProfile{
		"beam-unit": {
			FireMode:     "projectile",
			Range:        1,
			Damage:       1,
			Interval:     1,
			TargetAir:    true,
			TargetGround: true,
		},
		"beam-target": {},
	}
	w.unitMountProfilesByName = map[string][]unitWeaponMountProfile{
		"beam-unit": {{
			FireMode:         "beam",
			Range:            180,
			Damage:           12,
			Interval:         0.5,
			TargetAir:        true,
			TargetGround:     true,
			HitBuildings:     true,
			X:                0,
			Y:                0,
			ShootX:           0,
			ShootY:           0,
			ShootCone:        180,
			ShootWarmupSpeed: 1,
			RotationLimit:    361,
			Continuous:       true,
			Bullet: &bulletRuntimeProfile{
				ClassName:      "ContinuousLaserBulletType",
				Lifetime:       12.0 / 60.0,
				DamageInterval: 5.0 / 60.0,
				Length:         180,
				FadeTime:       4.0 / 60.0,
			},
		}},
	}

	before := w.model.Entities[1].Health
	stepWorldFrames(w, 1)

	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected first frame to spawn one beam bullet, got %d", got)
	}
	if got := normalizeBulletClassName(w.bullets[0].BulletClass); got != "continuouslaserbullettype" {
		t.Fatalf("expected continuous laser bullet, got %q", got)
	}
	if got := w.model.Entities[1].Health; got != before {
		t.Fatalf("expected no immediate damage on spawn frame, health before=%f after=%f", before, got)
	}

	stepWorldFrames(w, 5)

	if got := w.model.Entities[1].Health; got >= before {
		t.Fatalf("expected beam damage after first damageInterval, before=%f after=%f", before, got)
	}
	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected same beam bullet to stay alive during lifetime, got %d bullets", got)
	}

	stepWorldFrames(w, 7)

	if got := len(w.bullets); got != 0 {
		t.Fatalf("expected beam bullet to expire after lifetime, got %d", got)
	}

	stepWorldFrames(w, 10)

	if got := len(w.bullets); got != 0 {
		t.Fatalf("expected mount reload to prevent immediate refire, got %d bullets", got)
	}
}

func TestAlwaysContinuousMountPointLaserKeepsBeamAlive(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(64, 64)
	model.UnitNames = map[int16]string{
		1: "point-beam-unit",
		2: "point-beam-target",
	}
	model.Entities = []RawEntity{
		{
			ID:        1,
			TypeID:    1,
			Team:      1,
			X:         64,
			Y:         64,
			Rotation:  0,
			Health:    120,
			MaxHealth: 120,
			Shield:    0.1,
			ShieldMax: 0.1,
		},
		{
			ID:        2,
			TypeID:    2,
			Team:      2,
			X:         76,
			Y:         64,
			Rotation:  180,
			Health:    200,
			MaxHealth: 200,
			Shield:    0.1,
			ShieldMax: 0.1,
		},
	}
	w.SetModel(model)
	w.unitNamesByID = map[int16]string{
		1: "point-beam-unit",
		2: "point-beam-target",
	}
	w.unitProfilesByName = map[string]weaponProfile{
		"point-beam-unit": {
			FireMode:     "projectile",
			Range:        1,
			Damage:       1,
			Interval:     1,
			TargetAir:    true,
			TargetGround: true,
		},
		"point-beam-target": {},
	}
	w.unitMountProfilesByName = map[string][]unitWeaponMountProfile{
		"point-beam-unit": {{
			FireMode:         "beam",
			Range:            80,
			Damage:           18,
			Interval:         0.5,
			TargetAir:        true,
			TargetGround:     true,
			HitBuildings:     true,
			X:                0,
			Y:                0,
			ShootX:           0,
			ShootY:           0,
			ShootCone:        180,
			ShootWarmupSpeed: 1,
			RotationLimit:    361,
			Continuous:       true,
			AlwaysContinuous: true,
			AimChangeSpeed:   0.9,
			Bullet: &bulletRuntimeProfile{
				ClassName:        "PointLaserBulletType",
				Lifetime:         10.0 / 60.0,
				DamageInterval:   5.0 / 60.0,
				OptimalLifeFract: 0.5,
			},
		}},
	}

	stepWorldFrames(w, 1)

	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected first frame to spawn one point-laser bullet, got %d", got)
	}
	bid := w.bullets[0].ID

	stepWorldFrames(w, 20)

	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected alwaysContinuous point laser to stay on one bullet, got %d", got)
	}
	if got := w.bullets[0].ID; got != bid {
		t.Fatalf("expected alwaysContinuous mount to keep same bullet alive, first=%d current=%d", bid, got)
	}
	lifeHalf := w.bullets[0].LifeSec * 0.5
	if diff := math.Abs(float64(w.bullets[0].AgeSec - lifeHalf)); diff > 0.02 {
		t.Fatalf("expected keepAlive to pin point laser around optimalLifeFract, age=%f lifeHalf=%f", w.bullets[0].AgeSec, lifeHalf)
	}
}

func TestContinuousTurretPointLaserKeepsBeamAliveAndSmoothsAim(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(64, 64)
	model.BlockNames = map[int16]string{
		900: "test-point-turret",
	}
	model.UnitNames = map[int16]string{
		2: "point-target",
	}
	model.Entities = []RawEntity{
		{
			ID:        2,
			TypeID:    2,
			Team:      2,
			X:         100,
			Y:         84,
			Rotation:  180,
			Health:    300,
			MaxHealth: 300,
			Shield:    0.1,
			ShieldMax: 0.1,
		},
	}
	w.SetModel(model)
	w.unitProfilesByName = map[string]weaponProfile{
		"point-target": {},
	}
	placeTestBuilding(t, w, 10, 10, 900, 1, 0)
	w.buildingProfilesByName = map[string]buildingWeaponProfile{
		"test-point-turret": {
			ClassName:      "ContinuousTurret",
			FireMode:       "beam",
			Range:          80,
			Damage:         30,
			Interval:       0.2,
			TargetAir:      true,
			TargetGround:   true,
			HitBuildings:   true,
			ContinuousHold: true,
			AimChangeSpeed: 0.9,
			Bullet: &bulletRuntimeProfile{
				ClassName:        "PointLaserBulletType",
				Lifetime:         20.0 / 60.0,
				DamageInterval:   5.0 / 60.0,
				OptimalLifeFract: 0.5,
			},
		},
	}
	w.rebuildActiveTilesLocked()

	pos := int32(10*model.Width + 10)
	before := w.model.Entities[0].Health
	stepWorldFrames(w, 1)

	state := w.buildStates[pos]
	if state.BeamBulletID == 0 {
		t.Fatalf("expected continuous turret to keep a beam bullet alive")
	}
	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected one point-laser bullet, got %d", got)
	}
	if got := normalizeBulletClassName(w.bullets[0].BulletClass); got != "pointlaserbullettype" {
		t.Fatalf("expected point laser bullet, got %q", got)
	}
	if state.BeamLastLength <= 0 || state.BeamLastLength >= 16 {
		t.Fatalf("expected aimChangeSpeed to start with a partial beam length, got %f", state.BeamLastLength)
	}
	firstLength := state.BeamLastLength

	stepWorldFrames(w, 10)

	state = w.buildStates[pos]
	if state.BeamLastLength <= firstLength {
		t.Fatalf("expected point laser beam length to increase over time, first=%f current=%f", firstLength, state.BeamLastLength)
	}
	if state.BeamLastLength >= 16 {
		t.Fatalf("expected point laser beam length to still be approaching target after 11 frames, got %f", state.BeamLastLength)
	}

	stepWorldFrames(w, 25)

	state = w.buildStates[pos]
	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected continuous turret to keep one beam bullet alive, got %d", got)
	}
	idx := w.findBulletIndexByID(state.BeamBulletID)
	if idx < 0 {
		t.Fatalf("expected tracked beam bullet id=%d to stay valid", state.BeamBulletID)
	}
	b := w.bullets[idx]
	if diff := math.Abs(float64(b.AgeSec - b.LifeSec*0.5)); diff > 0.02 {
		t.Fatalf("expected continuous turret keepAlive to pin bullet age near optimalLifeFract, age=%f life=%f", b.AgeSec, b.LifeSec)
	}
	if got := w.model.Entities[0].Health; got >= before {
		t.Fatalf("expected point laser damage after beam reached target, before=%f after=%f", before, got)
	}
}

func TestLaserTurretContinuousBeamHonorsShootDuration(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(64, 64)
	model.BlockNames = map[int16]string{
		901: "test-laser-turret",
	}
	model.UnitNames = map[int16]string{
		2: "laser-target",
	}
	model.Entities = []RawEntity{
		{
			ID:        2,
			TypeID:    2,
			Team:      2,
			X:         100,
			Y:         84,
			Rotation:  180,
			Health:    500,
			MaxHealth: 500,
			Shield:    0.1,
			ShieldMax: 0.1,
		},
	}
	w.SetModel(model)
	w.unitProfilesByName = map[string]weaponProfile{
		"laser-target": {},
	}
	placeTestBuilding(t, w, 10, 10, 901, 1, 0)
	w.buildingProfilesByName = map[string]buildingWeaponProfile{
		"test-laser-turret": {
			ClassName:      "LaserTurret",
			FireMode:       "beam",
			Range:          120,
			Damage:         24,
			Interval:       0.5,
			TargetAir:      true,
			TargetGround:   true,
			HitBuildings:   true,
			ContinuousHold: true,
			ShootDuration:  6.0 / 60.0,
			Bullet: &bulletRuntimeProfile{
				ClassName:      "ContinuousLaserBulletType",
				Lifetime:       8.0 / 60.0,
				DamageInterval: 5.0 / 60.0,
				Length:         120,
				FadeTime:       4.0 / 60.0,
			},
		},
	}
	w.rebuildActiveTilesLocked()

	pos := int32(10*model.Width + 10)
	stepWorldFrames(w, 1)

	state := w.buildStates[pos]
	if state.BeamBulletID == 0 {
		t.Fatalf("expected laser turret to spawn a held beam bullet")
	}
	bid := state.BeamBulletID

	stepWorldFrames(w, 4)

	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected held laser beam to stay alive during shootDuration, got %d", got)
	}
	idx := w.findBulletIndexByID(bid)
	if idx < 0 {
		t.Fatalf("expected beam bullet id=%d to still exist during hold", bid)
	}
	if w.bullets[idx].AgeSec > 0.001 {
		t.Fatalf("expected keepAlive to hold laser bullet age at 0 during shootDuration, got %f", w.bullets[idx].AgeSec)
	}

	stepWorldFrames(w, 3)

	idx = w.findBulletIndexByID(bid)
	if idx < 0 {
		t.Fatalf("expected beam bullet id=%d to still exist just after hold ended", bid)
	}
	if w.bullets[idx].AgeSec <= 0 {
		t.Fatalf("expected bullet age to start advancing after shootDuration ended, got %f", w.bullets[idx].AgeSec)
	}

	stepWorldFrames(w, 10)

	if got := len(w.bullets); got != 0 {
		t.Fatalf("expected laser beam to expire after keepAlive stopped, got %d bullets", got)
	}

	stepWorldFrames(w, 10)

	if got := len(w.bullets); got != 0 {
		t.Fatalf("expected laser turret cooldown to block immediate refire, got %d bullets", got)
	}

	stepWorldFrames(w, 20)

	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected laser turret to refire after cooldown completed, got %d bullets", got)
	}
}

func TestControlledContinuousTurretRequiresShootInputAndTracksAim(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(64, 64)
	model.BlockNames = map[int16]string{
		902: "test-controlled-continuous-turret",
	}
	w.SetModel(model)

	tile := placeTestBuilding(t, w, 10, 10, 902, 1, 0)
	w.buildingProfilesByName = map[string]buildingWeaponProfile{
		"test-controlled-continuous-turret": {
			ClassName:      "ContinuousTurret",
			FireMode:       "beam",
			Range:          120,
			Damage:         20,
			Interval:       0.1,
			TargetAir:      true,
			TargetGround:   true,
			HitBuildings:   true,
			ContinuousHold: true,
			AimChangeSpeed: 999,
			Bullet: &bulletRuntimeProfile{
				ClassName:        "PointLaserBulletType",
				Lifetime:         10.0 / 60.0,
				DamageInterval:   5.0 / 60.0,
				OptimalLifeFract: 0.5,
			},
		},
	}
	w.rebuildActiveTilesLocked()

	buildPos := protocol.PackPoint2(10, 10)
	if _, ok := w.ClaimControlledBuildingPacked(7, buildPos); !ok {
		t.Fatal("expected continuous turret to be claimable as a control block")
	}

	leftAimX := float32(10*8 - 48)
	leftAimY := float32(10*8 + 4)
	if ok := w.SetControlledBuildingInputPacked(7, buildPos, leftAimX, leftAimY, false); !ok {
		t.Fatal("expected controlled turret idle aim update to succeed")
	}
	stepWorldFrames(w, 3)

	if got := len(w.bullets); got != 0 {
		t.Fatalf("expected controlled continuous turret to stay idle without shoot input, got %d bullets", got)
	}
	if tile.Rotation == 0 {
		t.Fatalf("expected controlled continuous turret rotation to follow idle aim, got=%d", tile.Rotation)
	}

	rightAimX := float32(10*8 + 60)
	rightAimY := float32(10*8 + 4)
	if ok := w.SetControlledBuildingInputPacked(7, buildPos, rightAimX, rightAimY, true); !ok {
		t.Fatal("expected controlled turret shooting update to succeed")
	}
	stepWorldFrames(w, 1)

	pos := int32(10*model.Width + 10)
	state := w.buildStates[pos]
	if state.BeamBulletID == 0 {
		t.Fatalf("expected controlled continuous turret to spawn a beam while shooting")
	}
	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected controlled continuous turret to keep one beam bullet, got %d", got)
	}
	if tile.Rotation != 0 {
		t.Fatalf("expected controlled continuous turret to rotate toward new aim while firing, got=%d", tile.Rotation)
	}

	if ok := w.SetControlledBuildingInputPacked(7, buildPos, rightAimX, rightAimY, false); !ok {
		t.Fatal("expected controlled turret shoot release update to succeed")
	}
	stepWorldFrames(w, 12)

	if got := len(w.bullets); got != 0 {
		t.Fatalf("expected controlled continuous beam to expire after shoot release, got %d bullets", got)
	}
}
