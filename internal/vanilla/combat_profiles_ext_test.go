package vanilla

import (
	"math"
	"testing"
)

func TestParseInlineBulletProfilePreservesZeroBuildingDamageMultiplier(t *testing.T) {
	prof := parseInlineBulletProfile("BasicBulletType", "3f, 10f", `
		buildingDamageMultiplier = 0f;
	`, nil)

	if prof.BuildingDamageMultiplier != 0 {
		t.Fatalf("expected explicit zero building damage multiplier to be preserved, got=%f", prof.BuildingDamageMultiplier)
	}
}

func TestApplyBulletProfilePreservesZeroBuildingDamageMultiplier(t *testing.T) {
	dst := &parsedProfile{
		buildingDamageMultiplier: 1,
		targetAir:                true,
		targetGround:             true,
		hitBuildings:             true,
	}

	applyBulletProfile(dst, BulletProfile{
		Damage:                   10,
		BuildingDamageMultiplier: 0,
		TargetAir:                true,
		TargetGround:             true,
		HitBuildings:             true,
	})

	if dst.buildingDamageMultiplier != 0 {
		t.Fatalf("expected explicit zero building damage multiplier to override default, got=%f", dst.buildingDamageMultiplier)
	}
}

func TestParseInlineBulletProfileContinuousLaserDefaults(t *testing.T) {
	prof := parseInlineBulletProfile("ContinuousLaserBulletType", "78f", `
		length = 200f;
	`, nil)

	if prof.Damage != 78 {
		t.Fatalf("expected ctor damage=78, got=%f", prof.Damage)
	}
	if math.Abs(float64(prof.Lifetime-(16.0/60.0))) > 0.0001 {
		t.Fatalf("expected default continuous laser lifetime 16f/60, got=%f", prof.Lifetime)
	}
	if math.Abs(float64(prof.DamageInterval-(5.0/60.0))) > 0.0001 {
		t.Fatalf("expected default damage interval 5f/60, got=%f", prof.DamageInterval)
	}
	if math.Abs(float64(prof.FadeTime-(16.0/60.0))) > 0.0001 {
		t.Fatalf("expected default fade time 16f/60, got=%f", prof.FadeTime)
	}
	if prof.Length != 200 {
		t.Fatalf("expected length=200, got=%f", prof.Length)
	}
}

func TestExtractTurretsParsesShootTypeContinuousMetadata(t *testing.T) {
	src := `
		lustre = new ContinuousTurret("lustre"){{
			shootType = new PointLaserBulletType(){{
				damage = 210f;
				buildingDamageMultiplier = 0.3f;
			}};
			scaleDamageEfficiency = true;
			aimChangeSpeed = 0.9f;
			range = 250f;
		}};

		meltdown = new LaserTurret("meltdown"){{
			shootType = new ContinuousLaserBulletType(78f){{
				length = 200f;
			}};
			shootDuration = 230f;
			range = 195f;
			reload = 90f;
		}};
	`

	turrets := extractTurrets(src, nil)
	if len(turrets) != 2 {
		t.Fatalf("expected 2 turrets, got %d", len(turrets))
	}

	var lustre, meltdown *TurretProfile
	for i := range turrets {
		switch turrets[i].Name {
		case "lustre":
			lustre = &turrets[i]
		case "meltdown":
			meltdown = &turrets[i]
		}
	}
	if lustre == nil {
		t.Fatalf("expected lustre turret to be parsed")
	}
	if lustre.ClassName != "ContinuousTurret" {
		t.Fatalf("expected lustre class ContinuousTurret, got=%q", lustre.ClassName)
	}
	if !lustre.ContinuousHold {
		t.Fatalf("expected lustre to be marked continuous_hold")
	}
	if math.Abs(float64(lustre.AimChangeSpeed-0.9)) > 0.0001 {
		t.Fatalf("expected lustre aimChangeSpeed=0.9, got=%f", lustre.AimChangeSpeed)
	}
	if lustre.Bullet == nil || lustre.Bullet.ClassName != "PointLaserBulletType" {
		t.Fatalf("expected lustre bullet class PointLaserBulletType, got=%+v", lustre.Bullet)
	}
	if math.Abs(float64(lustre.Bullet.OptimalLifeFract-0.5)) > 0.0001 {
		t.Fatalf("expected lustre optimalLifeFract=0.5, got=%f", lustre.Bullet.OptimalLifeFract)
	}
	if math.Abs(float64(lustre.Bullet.DamageInterval-(5.0/60.0))) > 0.0001 {
		t.Fatalf("expected lustre damageInterval=5f/60, got=%f", lustre.Bullet.DamageInterval)
	}

	if meltdown == nil {
		t.Fatalf("expected meltdown turret to be parsed")
	}
	if meltdown.ClassName != "LaserTurret" {
		t.Fatalf("expected meltdown class LaserTurret, got=%q", meltdown.ClassName)
	}
	if !meltdown.ContinuousHold {
		t.Fatalf("expected meltdown to be marked continuous_hold")
	}
	if math.Abs(float64(meltdown.ShootDuration-(230.0/60.0))) > 0.0001 {
		t.Fatalf("expected meltdown shootDuration=230f/60, got=%f", meltdown.ShootDuration)
	}
	if meltdown.Bullet == nil || meltdown.Bullet.ClassName != "ContinuousLaserBulletType" {
		t.Fatalf("expected meltdown bullet class ContinuousLaserBulletType, got=%+v", meltdown.Bullet)
	}
	if meltdown.Bullet.Length != 200 {
		t.Fatalf("expected meltdown bullet length=200, got=%f", meltdown.Bullet.Length)
	}
}
