package world

import (
	"math"
	"testing"
	"time"
)

func TestProjectileBuildingDamageRespectsArmorMultiplier(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		600: "test-wall",
	}
	w.SetModel(model)
	w.blockArmorByName["test-wall"] = 5

	tile := placeTestBuilding(t, w, 12, 12, 600, 2, 0)
	tile.Build.Health = 100
	tile.Build.MaxHealth = 100

	w.bullets = append(w.bullets, simBullet{
		ID:              1,
		Team:            1,
		X:               float32(12*8 + 4),
		Y:               float32(12*8 + 4),
		Damage:          20,
		Radius:          8,
		HitBuilds:       true,
		TargetGround:    true,
		BuildingDamage:  1,
		ArmorMultiplier: 2,
	})
	w.stepBullets(0, map[int32]int{}, nil, nil)

	if got := tile.Build.Health; math.Abs(float64(got-90)) > 0.01 {
		t.Fatalf("expected projectile to deal 10 damage after building armor, got health=%f", got)
	}
}

func TestProjectileBuildingDamagePierceArmor(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		600: "test-wall",
	}
	w.SetModel(model)
	w.blockArmorByName["test-wall"] = 5

	tile := placeTestBuilding(t, w, 12, 12, 600, 2, 0)
	tile.Build.Health = 100
	tile.Build.MaxHealth = 100

	w.bullets = append(w.bullets, simBullet{
		ID:             1,
		Team:           1,
		X:              float32(12*8 + 4),
		Y:              float32(12*8 + 4),
		Damage:         20,
		Radius:         8,
		HitBuilds:      true,
		TargetGround:   true,
		BuildingDamage: 1,
		PierceArmor:    true,
	})
	w.stepBullets(0, map[int32]int{}, nil, nil)

	if got := tile.Build.Health; math.Abs(float64(got-80)) > 0.01 {
		t.Fatalf("expected pierceArmor projectile to ignore building armor, got health=%f", got)
	}
}

func TestBeamBuildingDamageRespectsArmorMultiplier(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		600: "test-wall",
	}
	w.SetModel(model)
	w.blockArmorByName["test-wall"] = 5

	tile := placeTestBuilding(t, w, 12, 12, 600, 2, 0)
	tile.Build.Health = 100
	tile.Build.MaxHealth = 100

	model.Entities = append(model.Entities, RawEntity{
		ID:                    1,
		TypeID:                35,
		Team:                  1,
		X:                     float32(12*8 - 32),
		Y:                     float32(12*8 + 4),
		Health:                100,
		MaxHealth:             100,
		AttackDamage:          20,
		AttackBuildingDamage:  1,
		AttackArmorMultiplier: 2,
		AttackInterval:        0.05,
		AttackRange:           120,
		AttackFireMode:        "beam",
		AttackBuildings:       true,
		AttackTargetGround:    true,
		SlowMul:               1,
		StatusDamageMul:       1,
		StatusHealthMul:       1,
		StatusSpeedMul:        1,
		StatusReloadMul:       1,
		StatusBuildSpeedMul:   1,
		StatusDragMul:         1,
		StatusArmorOverride:   -1,
		RuntimeInit:           true,
	})

	w.Step(time.Second / 60)

	if got := tile.Build.Health; math.Abs(float64(got-90)) > 0.01 {
		t.Fatalf("expected beam to deal 10 damage after building armor, got health=%f", got)
	}
}

func TestApplyDamageToEntityProfileRespectsArmorMultiplierAndPierceArmor(t *testing.T) {
	w := New(Config{TPS: 60})

	target := RawEntity{
		Health:              100,
		MaxHealth:           100,
		Armor:               5,
		SlowMul:             1,
		StatusArmorOverride: -1,
	}
	w.applyDamageToEntityProfile(&target, 20, damageApplyProfile{ArmorMultiplier: 2})
	if math.Abs(float64(target.Health-90)) > 0.01 {
		t.Fatalf("expected armorMultiplier to reduce damage to 10, got health=%f", target.Health)
	}

	target = RawEntity{
		Health:              100,
		MaxHealth:           100,
		Armor:               5,
		SlowMul:             1,
		StatusArmorOverride: -1,
	}
	w.applyDamageToEntityProfile(&target, 20, damageApplyProfile{PierceArmor: true})
	if math.Abs(float64(target.Health-80)) > 0.01 {
		t.Fatalf("expected pierceArmor to ignore armor, got health=%f", target.Health)
	}
}

func TestApplyDamageToEntityProfileRespectsMaxDamageFraction(t *testing.T) {
	w := New(Config{TPS: 60})

	target := RawEntity{
		Health:    100,
		MaxHealth: 100,
		Shield:    5,
		ShieldMax: 5,
		SlowMul:   1,
	}
	w.applyDamageToEntityProfile(&target, 80, damageApplyProfile{MaxDamageFraction: 0.25})

	if math.Abs(float64(target.Shield)) > 0.01 {
		t.Fatalf("expected shield to be fully consumed by capped damage, got=%f", target.Shield)
	}
	if math.Abs(float64(target.Health-75)) > 0.01 {
		t.Fatalf("expected capped damage to leave health=75, got=%f", target.Health)
	}
}

func TestForceFieldAbilityAbsorbsProjectileBeforeDirectUnitHit(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "shielded",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["shielded"] = unitRuntimeProfile{
		Name: "shielded",
		Abilities: []unitAbilityProfile{{
			Kind:     unitAbilityForceField,
			Radius:   18,
			Regen:    0.4,
			Max:      30,
			Cooldown: 6,
		}},
	}

	model.Entities = append(model.Entities, RawEntity{
		ID:        1,
		TypeID:    35,
		Team:      1,
		X:         80,
		Y:         80,
		Health:    100,
		MaxHealth: 100,
	})
	w.ensureEntityDefaults(&model.Entities[0])

	w.bullets = append(w.bullets, simBullet{
		ID:              1,
		Team:            2,
		X:               96,
		Y:               80,
		Damage:          10,
		Radius:          1,
		LifeSec:         1,
		HitUnits:        true,
		TargetGround:    true,
		BuildingDamage:  1,
		ShieldDamageMul: 1,
	})

	w.stepBullets(1.0/60.0, map[int32]int{1: 0}, nil, nil)

	if len(w.bullets) != 0 {
		t.Fatalf("expected projectile to be absorbed by force field, bullets=%d", len(w.bullets))
	}
	got := model.Entities[0]
	if math.Abs(float64(got.Health-100)) > 0.01 {
		t.Fatalf("expected force field absorption to preserve health, got=%f", got.Health)
	}
	if math.Abs(float64(got.Shield-20)) > 0.01 {
		t.Fatalf("expected force field to lose 10 shield, got=%f", got.Shield)
	}
}

func TestShieldArcAbilityAbsorbsProjectileWithinArcBand(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "arcshield",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["arcshield"] = unitRuntimeProfile{
		Name: "arcshield",
		Abilities: []unitAbilityProfile{{
			Kind:         unitAbilityShieldArc,
			Radius:       18,
			Width:        4,
			Angle:        90,
			AngleOffset:  0,
			WhenShooting: false,
			Regen:        0.2,
			Max:          15,
			Cooldown:     5,
		}},
	}

	model.Entities = append(model.Entities, RawEntity{
		ID:        2,
		TypeID:    35,
		Team:      1,
		X:         100,
		Y:         100,
		Rotation:  0,
		Health:    100,
		MaxHealth: 100,
	})
	w.ensureEntityDefaults(&model.Entities[0])

	w.bullets = append(w.bullets, simBullet{
		ID:             2,
		Team:           2,
		X:              118,
		Y:              100,
		Damage:         5,
		Radius:         1,
		LifeSec:        1,
		HitUnits:       true,
		TargetGround:   true,
		BuildingDamage: 1,
	})

	w.stepBullets(1.0/60.0, map[int32]int{2: 0}, nil, nil)

	if len(w.bullets) != 0 {
		t.Fatalf("expected projectile to be absorbed by shield arc, bullets=%d", len(w.bullets))
	}
	got := model.Entities[0]
	if math.Abs(float64(got.Health-100)) > 0.01 {
		t.Fatalf("expected shield arc absorption to preserve health, got=%f", got.Health)
	}
	if len(got.Abilities) != 1 {
		t.Fatalf("expected shield arc runtime state to exist, got=%d", len(got.Abilities))
	}
	if math.Abs(float64(got.Abilities[0].Data-10)) > 0.01 {
		t.Fatalf("expected shield arc to lose 5 shield data, got=%f", got.Abilities[0].Data)
	}
}

func TestShieldArcAbilityBreakUsesRawBulletDamageThreshold(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "arcshield",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["arcshield"] = unitRuntimeProfile{
		Name: "arcshield",
		Abilities: []unitAbilityProfile{{
			Kind:         unitAbilityShieldArc,
			Radius:       18,
			Width:        4,
			Angle:        90,
			AngleOffset:  0,
			WhenShooting: false,
			Regen:        1,
			Max:          15,
			Cooldown:     10,
		}},
	}

	model.Entities = append(model.Entities, RawEntity{
		ID:          27,
		TypeID:      35,
		Team:        1,
		X:           100,
		Y:           100,
		Rotation:    0,
		Health:      100,
		MaxHealth:   100,
		Abilities:   []entityAbilityState{{Data: 8}},
		RuntimeInit: true,
	})

	w.bullets = append(w.bullets, simBullet{
		ID:              28,
		Team:            2,
		X:               118,
		Y:               100,
		Damage:          10,
		Radius:          1,
		LifeSec:         1,
		HitUnits:        true,
		TargetGround:    true,
		BuildingDamage:  1,
		ShieldDamageMul: 0.5,
	})

	w.stepBullets(1.0/60.0, map[int32]int{27: 0}, nil, nil)

	if got := len(w.bullets); got != 0 {
		t.Fatalf("expected projectile to be absorbed by shield arc, bullets=%d", got)
	}
	if got := model.Entities[0].Health; math.Abs(float64(got-100)) > 0.01 {
		t.Fatalf("expected shield arc absorption to preserve health, got=%f", got)
	}
	if got := model.Entities[0].Abilities[0].Data; math.Abs(float64(got-(-7))) > 0.01 {
		t.Fatalf("expected raw bullet damage to trigger shield break penalty before shield damage multiplier, got=%f", got)
	}
}

func TestForceFieldAbilityDoesNotResetBrokenShieldState(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "shielded",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["shielded"] = unitRuntimeProfile{
		Name: "shielded",
		Abilities: []unitAbilityProfile{{
			Kind:     unitAbilityForceField,
			Radius:   18,
			Regen:    0.4,
			Max:      30,
			Cooldown: 6,
		}},
	}

	model.Entities = append(model.Entities, RawEntity{
		ID:          3,
		TypeID:      35,
		Team:        1,
		X:           80,
		Y:           80,
		Health:      100,
		MaxHealth:   100,
		Shield:      -2.4,
		ShieldMax:   30,
		ShieldRegen: 0.4,
		RuntimeInit: true,
	})

	w.stepEntityAbilities(1)

	if got := model.Entities[0].Shield; math.Abs(float64(got-(-2.4))) > 0.01 {
		t.Fatalf("expected broken force field shield to remain in cooldown state, got=%f", got)
	}
}

func TestShieldArcAbilityDoesNotResetBrokenStateDuringRegen(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "arcshield",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["arcshield"] = unitRuntimeProfile{
		Name: "arcshield",
		Abilities: []unitAbilityProfile{{
			Kind:         unitAbilityShieldArc,
			Radius:       18,
			Width:        4,
			Angle:        90,
			WhenShooting: false,
			Regen:        0.2,
			Max:          15,
			Cooldown:     5,
		}},
	}

	model.Entities = append(model.Entities, RawEntity{
		ID:          4,
		TypeID:      35,
		Team:        1,
		X:           100,
		Y:           100,
		Health:      100,
		MaxHealth:   100,
		Abilities:   []entityAbilityState{{Data: -1}},
		RuntimeInit: true,
	})

	w.stepEntityAbilities(1)

	if len(model.Entities[0].Abilities) != 1 {
		t.Fatalf("expected one shield arc runtime state, got=%d", len(model.Entities[0].Abilities))
	}
	if got := model.Entities[0].Abilities[0].Data; math.Abs(float64(got-(-0.8))) > 0.01 {
		t.Fatalf("expected broken shield arc to regen from negative state, got=%f", got)
	}
}

func TestShieldArcAbilityConsumesMissileUnitsInsideArc(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "arcshield",
		91: "scathe-missile",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["arcshield"] = unitRuntimeProfile{
		Name: "arcshield",
		Abilities: []unitAbilityProfile{{
			Kind:                  unitAbilityShieldArc,
			Radius:                18,
			Width:                 4,
			Angle:                 120,
			WhenShooting:          false,
			Max:                   20,
			MissileUnitMultiplier: 2,
		}},
	}

	model.Entities = append(model.Entities,
		RawEntity{
			ID:          10,
			TypeID:      35,
			Team:        1,
			X:           100,
			Y:           100,
			Health:      100,
			MaxHealth:   100,
			Abilities:   []entityAbilityState{{Data: 20}},
			RuntimeInit: true,
		},
		RawEntity{
			ID:          11,
			TypeID:      91,
			Team:        2,
			X:           118,
			Y:           100,
			Health:      5,
			MaxHealth:   5,
			RuntimeInit: true,
			SlowMul:     1,
		},
	)

	w.stepEntityAbilities(1.0 / 60.0)

	if got := model.Entities[1].Health; got > 0 {
		t.Fatalf("expected shield arc to consume missile unit, got health=%f", got)
	}
	if got := model.Entities[0].Abilities[0].Data; math.Abs(float64(got-10)) > 0.01 {
		t.Fatalf("expected shield arc missile hit to consume 10 shield, got=%f", got)
	}
}

func TestShieldArcAbilityMissileDamageRespectsUnitDamageRule(t *testing.T) {
	w := New(Config{TPS: 60})
	rules := DefaultRules()
	rules.UnitDamageMultiplier = 2
	w.rulesMgr.Set(rules)
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "arcshield",
		91: "scathe-missile",
	}
	w.SetModel(model)
	w.rulesMgr.Set(rules)
	w.unitRuntimeProfilesByName["arcshield"] = unitRuntimeProfile{
		Name: "arcshield",
		Abilities: []unitAbilityProfile{{
			Kind:                  unitAbilityShieldArc,
			Radius:                18,
			Width:                 4,
			Angle:                 120,
			WhenShooting:          false,
			Max:                   30,
			MissileUnitMultiplier: 2,
		}},
	}

	model.Entities = append(model.Entities,
		RawEntity{
			ID:          22,
			TypeID:      35,
			Team:        1,
			X:           100,
			Y:           100,
			Health:      100,
			MaxHealth:   100,
			Abilities:   []entityAbilityState{{Data: 30}},
			RuntimeInit: true,
		},
		RawEntity{
			ID:          23,
			TypeID:      91,
			Team:        2,
			X:           118,
			Y:           100,
			Health:      5,
			MaxHealth:   5,
			RuntimeInit: true,
			SlowMul:     1,
		},
	)

	w.stepEntityAbilities(1.0 / 60.0)

	if got := model.Entities[1].Health; got > 0 {
		t.Fatalf("expected shield arc to consume missile unit, got health=%f", got)
	}
	if got := model.Entities[0].Abilities[0].Data; math.Abs(float64(got-10)) > 0.01 {
		t.Fatalf("expected shield arc missile hit to consume 20 shield under unitDamageMultiplier=2, got=%f", got)
	}
}

func TestShieldArcAbilityDirectDamageOutsideArcRadiusIsNotAbsorbed(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "arcshield",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["arcshield"] = unitRuntimeProfile{
		Name: "arcshield",
		Abilities: []unitAbilityProfile{{
			Kind:         unitAbilityShieldArc,
			Radius:       18,
			Width:        4,
			Angle:        120,
			WhenShooting: false,
			Max:          20,
		}},
	}

	target := RawEntity{
		ID:          24,
		TypeID:      35,
		Team:        1,
		X:           100,
		Y:           100,
		Health:      100,
		MaxHealth:   100,
		Abilities:   []entityAbilityState{{Data: 20}},
		RuntimeInit: true,
	}

	if remaining, absorbed := w.absorbEntityAbilityDamage(&target, 200, 100, 6); absorbed || remaining != 6 {
		t.Fatalf("expected direct damage outside shield arc radius to bypass shield, absorbed=%v remaining=%f", absorbed, remaining)
	}
	if got := target.Abilities[0].Data; math.Abs(float64(got-20)) > 0.01 {
		t.Fatalf("expected shield arc data unchanged for out-of-range direct damage, got=%f", got)
	}
}

func TestForceFieldDirectDamageOutsideRadiusIsNotAbsorbed(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "shielded",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["shielded"] = unitRuntimeProfile{
		Name: "shielded",
		Abilities: []unitAbilityProfile{{
			Kind:     unitAbilityForceField,
			Radius:   18,
			Regen:    0.4,
			Max:      30,
			Cooldown: 6,
		}},
	}

	target := RawEntity{
		ID:          25,
		TypeID:      35,
		Team:        1,
		X:           80,
		Y:           80,
		Health:      100,
		MaxHealth:   100,
		Shield:      30,
		ShieldMax:   30,
		ShieldRegen: 0.4,
		RuntimeInit: true,
	}

	if remaining, absorbed := w.absorbEntityAbilityDamage(&target, 140, 80, 10); absorbed || remaining != 10 {
		t.Fatalf("expected direct damage outside force field radius to bypass shield, absorbed=%v remaining=%f", absorbed, remaining)
	}
	if got := target.Shield; math.Abs(float64(got-30)) > 0.01 {
		t.Fatalf("expected force field shield unchanged for out-of-range direct damage, got=%f", got)
	}
}

func TestShieldArcAbilityDeflectsBulletWhenChanceDeflectEnabled(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "tecta",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["tecta"] = unitRuntimeProfile{
		Name: "tecta",
		Abilities: []unitAbilityProfile{{
			Kind:          unitAbilityShieldArc,
			Radius:        18,
			Width:         4,
			Angle:         120,
			WhenShooting:  false,
			Max:           20,
			ChanceDeflect: 1,
		}},
	}

	model.Entities = append(model.Entities, RawEntity{
		ID:          26,
		TypeID:      35,
		Team:        1,
		X:           100,
		Y:           100,
		Rotation:    0,
		Health:      100,
		MaxHealth:   100,
		Abilities:   []entityAbilityState{{Data: 20}},
		RuntimeInit: true,
	})

	w.bullets = append(w.bullets, simBullet{
		ID:             3,
		Team:           2,
		X:              118,
		Y:              100,
		VX:             -60,
		VY:             0,
		Damage:         5,
		Radius:         1,
		LifeSec:        1,
		HitUnits:       true,
		TargetGround:   true,
		BuildingDamage: 1,
	})

	w.stepBullets(1.0/60.0, map[int32]int{26: 0}, nil, nil)

	if got := len(w.bullets); got != 1 {
		t.Fatalf("expected deflected bullet to remain in flight, bullets=%d", got)
	}
	if got := w.bullets[0].Team; got != 1 {
		t.Fatalf("expected deflected bullet to switch to shield owner team, got=%d", got)
	}
	if got := w.bullets[0].VX; got <= 0 {
		t.Fatalf("expected deflected bullet to reverse X velocity, got=%f", got)
	}
	if got := w.bullets[0].AgeSec; math.Abs(float64(got-0.5)) > 0.01 {
		t.Fatalf("expected deflected bullet age to reset to half lifetime, got=%f", got)
	}
	if got := model.Entities[0].Abilities[0].Data; math.Abs(float64(got-15)) > 0.01 {
		t.Fatalf("expected shield arc deflection to still consume 5 shield, got=%f", got)
	}
}

func TestShieldArcAbilityPushesEnemyUnitsOutward(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "arcshield",
		92: "intruder",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["arcshield"] = unitRuntimeProfile{
		Name: "arcshield",
		Abilities: []unitAbilityProfile{{
			Kind:         unitAbilityShieldArc,
			Radius:       18,
			Width:        4,
			Angle:        180,
			WhenShooting: false,
			Max:          20,
			PushUnits:    true,
		}},
	}

	model.Entities = append(model.Entities,
		RawEntity{
			ID:          12,
			TypeID:      35,
			Team:        1,
			X:           100,
			Y:           100,
			Health:      100,
			MaxHealth:   100,
			Abilities:   []entityAbilityState{{Data: 20}},
			RuntimeInit: true,
		},
		RawEntity{
			ID:          13,
			TypeID:      92,
			Team:        2,
			X:           117,
			Y:           100,
			Health:      20,
			MaxHealth:   20,
			VelX:        -5,
			VelY:        0,
			RuntimeInit: true,
			SlowMul:     1,
		},
	)

	w.stepEntityAbilities(1.0 / 60.0)

	if got := model.Entities[1].X; got <= 121.9 {
		t.Fatalf("expected shield arc to push enemy unit outward, got x=%f", got)
	}
	if model.Entities[1].VelX != 0 || model.Entities[1].VelY != 0 {
		t.Fatalf("expected inward-moving unit velocity to be cancelled, got vel=(%f,%f)", model.Entities[1].VelX, model.Entities[1].VelY)
	}
}

func TestEnergyFieldAbilityHitsAbsorberWallBeforeEnemyUnit(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.BlockNames = map[int16]string{
		475: "plastanium-wall",
	}
	model.UnitNames = map[int16]string{
		35: "fielder",
		36: "target",
	}
	w.SetModel(model)
	w.unitRuntimeProfilesByName["fielder"] = unitRuntimeProfile{
		Name: "fielder",
		Abilities: []unitAbilityProfile{{
			Kind:         unitAbilityEnergyField,
			Damage:       20,
			Reload:       1,
			Range:        80,
			MaxTargets:   1,
			TargetGround: true,
			TargetAir:    false,
			HitUnits:     true,
			HitBuildings: false,
			UseAmmo:      false,
		}},
	}

	wall := placeTestBuilding(t, w, 12, 10, 475, 2, 0)
	wall.Build.Health = 100
	wall.Build.MaxHealth = 100

	model.Entities = append(model.Entities,
		RawEntity{
			ID:          20,
			TypeID:      35,
			Team:        1,
			X:           float32(10*8 + 4),
			Y:           float32(10*8 + 4),
			Health:      100,
			MaxHealth:   100,
			RuntimeInit: true,
		},
		RawEntity{
			ID:          21,
			TypeID:      36,
			Team:        2,
			X:           float32(14*8 + 4),
			Y:           float32(10*8 + 4),
			Health:      60,
			MaxHealth:   60,
			RuntimeInit: true,
			SlowMul:     1,
		},
	)

	model.Entities[0].Abilities = []entityAbilityState{{Timer: 1}}
	w.stepEntityAbilities(1.0 / 60.0)

	if got := model.Entities[1].Health; math.Abs(float64(got-60)) > 0.01 {
		t.Fatalf("expected plastanium wall to absorb energy field before enemy unit, got target health=%f", got)
	}
	if got := wall.Build.Health; math.Abs(float64(got-80)) > 0.01 {
		t.Fatalf("expected absorber wall to take 20 damage, got wall health=%f", got)
	}
}

func TestStatusFieldAbilityRefreshMarksEntityChanged(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "buffer",
	}
	w.SetModel(model)
	w.statusProfilesByID[14] = statusEffectProfile{
		ID:                   14,
		Name:                 "overclock",
		DamageMultiplier:     1,
		HealthMultiplier:     1,
		SpeedMultiplier:      1,
		ReloadMultiplier:     1.15,
		BuildSpeedMultiplier: 1.15,
		DragMultiplier:       1,
	}
	w.statusProfilesByName["overclock"] = w.statusProfilesByID[14]
	w.unitRuntimeProfilesByName["buffer"] = unitRuntimeProfile{
		Name: "buffer",
		Abilities: []unitAbilityProfile{{
			Kind:           unitAbilityStatusField,
			Reload:         1,
			Range:          60,
			StatusID:       14,
			StatusName:     "overclock",
			StatusDuration: 6,
		}},
	}

	model.Entities = append(model.Entities,
		RawEntity{
			ID:          30,
			TypeID:      35,
			Team:        1,
			X:           100,
			Y:           100,
			Health:      100,
			MaxHealth:   100,
			Statuses:    []entityStatusState{{ID: 14, Name: "overclock", Time: 1}},
			Abilities:   []entityAbilityState{{Timer: 1}},
			RuntimeInit: true,
		},
		RawEntity{
			ID:          31,
			TypeID:      35,
			Team:        1,
			X:           120,
			Y:           100,
			Health:      100,
			MaxHealth:   100,
			Statuses:    []entityStatusState{{ID: 14, Name: "overclock", Time: 1}},
			RuntimeInit: true,
		},
	)

	initialRev := model.EntitiesRev
	w.stepEntityAbilities(1.0 / 60.0)

	if got := model.Entities[1].Statuses[0].Time; math.Abs(float64(got-6)) > 0.01 {
		t.Fatalf("expected status field to refresh ally status duration to 6, got=%f", got)
	}
	if got := model.EntitiesRev; got <= initialRev {
		t.Fatalf("expected refreshed status duration to mark entities changed, rev before=%d after=%d", initialRev, got)
	}
}

func TestEnergyFieldAbilityShieldDamageAndStatusRefreshMarkChanged(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "fielder",
		36: "target",
	}
	w.SetModel(model)
	w.statusProfilesByID[7] = statusEffectProfile{
		ID:                   7,
		Name:                 "electrified",
		DamageMultiplier:     1,
		HealthMultiplier:     1,
		SpeedMultiplier:      1,
		ReloadMultiplier:     1,
		BuildSpeedMultiplier: 1,
		DragMultiplier:       1,
	}
	w.statusProfilesByName["electrified"] = w.statusProfilesByID[7]
	w.unitRuntimeProfilesByName["fielder"] = unitRuntimeProfile{
		Name: "fielder",
		Abilities: []unitAbilityProfile{{
			Kind:           unitAbilityEnergyField,
			Damage:         10,
			Reload:         1,
			Range:          80,
			MaxTargets:     1,
			TargetGround:   true,
			TargetAir:      false,
			HitUnits:       true,
			HitBuildings:   false,
			StatusID:       7,
			StatusName:     "electrified",
			StatusDuration: 6,
			UseAmmo:        false,
		}},
	}

	model.Entities = append(model.Entities,
		RawEntity{
			ID:          32,
			TypeID:      35,
			Team:        1,
			X:           100,
			Y:           100,
			Health:      100,
			MaxHealth:   100,
			Abilities:   []entityAbilityState{{Timer: 1}},
			RuntimeInit: true,
		},
		RawEntity{
			ID:          33,
			TypeID:      36,
			Team:        2,
			X:           120,
			Y:           100,
			Health:      100,
			MaxHealth:   100,
			Shield:      15,
			Statuses:    []entityStatusState{{ID: 7, Name: "electrified", Time: 1}},
			RuntimeInit: true,
			SlowMul:     1,
		},
	)

	initialRev := model.EntitiesRev
	w.stepEntityAbilities(1.0 / 60.0)

	if got := model.Entities[1].Health; math.Abs(float64(got-100)) > 0.01 {
		t.Fatalf("expected energy field to leave health untouched when shield absorbs damage, got=%f", got)
	}
	if got := model.Entities[1].Shield; math.Abs(float64(got-5)) > 0.01 {
		t.Fatalf("expected energy field to consume 10 shield, got=%f", got)
	}
	if got := model.Entities[1].Statuses[0].Time; math.Abs(float64(got-6)) > 0.01 {
		t.Fatalf("expected energy field to refresh existing status to 6, got=%f", got)
	}
	if got := model.EntitiesRev; got <= initialRev {
		t.Fatalf("expected shield/status-only energy field hit to mark entities changed, rev before=%d after=%d", initialRev, got)
	}
}

func TestSpawnDeathAbilityZeroSpreadSpawnsAtDeathPosition(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(24, 24)
	model.UnitNames = map[int16]string{
		35: "parent",
		36: "child",
	}
	w.SetModel(model)
	rules := DefaultRules()
	rules.DisableUnitCap = true
	w.rulesMgr.Set(rules)
	w.unitRuntimeProfilesByName["parent"] = unitRuntimeProfile{
		Name: "parent",
		Abilities: []unitAbilityProfile{{
			Kind:          unitAbilitySpawnDeath,
			SpawnAmount:   1,
			SpawnUnitName: "child",
			Spread:        0,
			FaceOutwards:  false,
		}},
	}
	w.unitRuntimeProfilesByName["child"] = unitRuntimeProfile{
		Name:   "child",
		Health: 50,
		Speed:  1,
	}

	w.handleEntityDeathAbilitiesLocked(RawEntity{
		ID:        30,
		TypeID:    35,
		Team:      1,
		X:         96,
		Y:         80,
		Rotation:  33,
		Health:    0,
		MaxHealth: 100,
	})

	if got := len(w.Model().Entities); got != 1 {
		t.Fatalf("expected one spawned child unit, got=%d", got)
	}
	spawned := w.Model().Entities[0]
	if math.Abs(float64(spawned.X-96)) > 0.001 || math.Abs(float64(spawned.Y-80)) > 0.001 {
		t.Fatalf("expected zero-spread death spawn at exact death position, got=(%f,%f)", spawned.X, spawned.Y)
	}
}
