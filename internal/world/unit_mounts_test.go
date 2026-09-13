package world

import (
	"testing"
	"time"
)

func TestStepEntityMountedCombatQueuesDelayedPatternShots(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(128, 128)
	model.UnitNames = map[int16]string{
		1: "pattern-tester",
		2: "pattern-target",
	}
	model.Entities = []RawEntity{
		{
			ID:        1,
			TypeID:    1,
			Team:      1,
			X:         64,
			Y:         64,
			Rotation:  0,
			Health:    100,
			MaxHealth: 100,
		},
		{
			ID:        2,
			TypeID:    2,
			Team:      2,
			X:         220,
			Y:         64,
			Rotation:  180,
			Health:    100,
			MaxHealth: 100,
		},
	}
	w.SetModel(model)
	w.unitNamesByID = map[int16]string{
		1: "pattern-tester",
		2: "pattern-target",
	}
	w.unitMountProfilesByName = map[string][]unitWeaponMountProfile{
		"pattern-tester": {{
			FireMode:             "projectile",
			Range:                240,
			Damage:               10,
			Interval:             0.5,
			BulletType:           1,
			BulletSpeed:          60,
			BulletLifetime:       5,
			TargetAir:            true,
			TargetGround:         true,
			HitBuildings:         true,
			ShootCone:            180,
			ShootWarmupSpeed:     1,
			RotationLimit:        361,
			ShootPattern:         "spread",
			ShootShots:           2,
			ShootShotDelay:       1.0 / 60.0,
			ShootSpread:          0,
			TargetInterval:       1.0 / 60.0,
			TargetSwitchInterval: 1.0 / 60.0,
		}},
	}

	w.Step(time.Second / 60)

	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected first frame to fire one immediate bullet, got %d", got)
	}
	if got := len(w.pendingMountShots); got != 1 {
		t.Fatalf("expected first frame to queue one delayed shot, got %d", got)
	}

	w.Step(time.Second / 60)

	if got := len(w.pendingMountShots); got != 0 {
		t.Fatalf("expected delayed shot queue to flush on second frame, got %d", got)
	}
	if got := len(w.bullets); got != 2 {
		t.Fatalf("expected second frame to contain both bullets in flight, got %d", got)
	}
}
