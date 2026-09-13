package vanilla

import (
	"math"
	"testing"
)

func TestExtractWeaponMountProfilesExpandsMirrorAndAlternatePattern(t *testing.T) {
	body := `
		new Weapon("sei-launcher"){{
			x = 3f;
			baseRotation = 12f;
			reload = 45f;
			shoot = new ShootAlternate(){{
				shots = 6;
				shotDelay = 1.5f;
				spread = 4f;
				barrels = 3;
			}};
			bullet = new MissileBulletType(4.2f, 42){{
				lifetime = 62f;
			}};
		}}
	`

	mounts := extractWeaponMountProfiles(body, nil)
	if len(mounts) != 2 {
		t.Fatalf("expected mirrored weapon to expand to 2 mounts, got %d", len(mounts))
	}

	left, right := mounts[0], mounts[1]
	if left.OtherSide != 1 || right.OtherSide != 0 {
		t.Fatalf("unexpected mirrored sides: left=%d right=%d", left.OtherSide, right.OtherSide)
	}
	if left.ShootPattern != "alternate" || right.ShootPattern != "alternate" {
		t.Fatalf("expected alternate pattern on both mounts, left=%q right=%q", left.ShootPattern, right.ShootPattern)
	}
	if left.ShootBarrels != 3 || right.ShootBarrels != 3 {
		t.Fatalf("expected barrels=3 on both mirrored mounts, left=%d right=%d", left.ShootBarrels, right.ShootBarrels)
	}
	if math.Abs(float64(left.Interval-1.5)) > 0.0001 || math.Abs(float64(right.Interval-1.5)) > 0.0001 {
		t.Fatalf("expected mirrored reload doubling to 1.5s, left=%f right=%f", left.Interval, right.Interval)
	}
	if right.X != -left.X {
		t.Fatalf("expected mirrored X to be flipped, left=%f right=%f", left.X, right.X)
	}
	if right.BaseRotation != -left.BaseRotation {
		t.Fatalf("expected mirrored base rotation to be flipped, left=%f right=%f", left.BaseRotation, right.BaseRotation)
	}
	if left.ShootPatternMirror == right.ShootPatternMirror {
		t.Fatalf("expected mirrored alternate pattern to flip mirror flag, left=%v right=%v", left.ShootPatternMirror, right.ShootPatternMirror)
	}
}

func TestExtractWeaponMountProfilesParsesRepairBeamAndHelixPattern(t *testing.T) {
	body := `
		new RepairBeamWeapon(){{
			mirror = false;
			shoot = new ShootHelix(){{
				mag = 1f;
				scl = 5f;
			}};
			bullet = new BulletType(){{
				maxRange = 120f;
			}};
		}}
	`

	mounts := extractWeaponMountProfiles(body, nil)
	if len(mounts) != 1 {
		t.Fatalf("expected non-mirrored repair beam weapon to stay at 1 mount, got %d", len(mounts))
	}

	mount := mounts[0]
	if !mount.RepairBeam || !mount.NoAttack || !mount.Rotate {
		t.Fatalf("expected repair beam defaults to be applied, got repair=%v noAttack=%v rotate=%v", mount.RepairBeam, mount.NoAttack, mount.Rotate)
	}
	if mount.ShootPattern != "helix" {
		t.Fatalf("expected helix shoot pattern, got %q", mount.ShootPattern)
	}
	if mount.ShootShots != 2 {
		t.Fatalf("expected helix default to emit 2 shots, got %d", mount.ShootShots)
	}
	if math.Abs(float64(mount.ShootHelixScl-5)) > 0.0001 || math.Abs(float64(mount.ShootHelixMag-1)) > 0.0001 {
		t.Fatalf("unexpected helix parameters scl=%f mag=%f", mount.ShootHelixScl, mount.ShootHelixMag)
	}
}

func TestExtractWeaponMountProfilesParsesPointLaserContinuousMetadata(t *testing.T) {
	body := `
		new Weapon("test-point-laser"){{
			mirror = false;
			rotate = true;
			continuous = true;
			alwaysContinuous = true;
			aimChangeSpeed = 0.9f;
			bullet = new PointLaserBulletType(){{
				damage = 18f;
			}};
		}}
	`

	mounts := extractWeaponMountProfiles(body, nil)
	if len(mounts) != 1 {
		t.Fatalf("expected non-mirrored point laser weapon to stay at 1 mount, got %d", len(mounts))
	}

	mount := mounts[0]
	if !mount.Continuous {
		t.Fatalf("expected continuous point laser mount")
	}
	if !mount.AlwaysContinuous {
		t.Fatalf("expected alwaysContinuous point laser mount")
	}
	if math.Abs(float64(mount.AimChangeSpeed-0.9)) > 0.0001 {
		t.Fatalf("expected aimChangeSpeed=0.9, got %f", mount.AimChangeSpeed)
	}
	if mount.Bullet == nil || mount.Bullet.ClassName != "PointLaserBulletType" {
		t.Fatalf("expected point laser bullet profile, got %+v", mount.Bullet)
	}
}
