package vanilla

import (
	"math"
	"regexp"
	"strings"
)

type WeaponMountProfile struct {
	ClassName                string         `json:"class_name,omitempty"`
	FireMode                 string         `json:"fire_mode"`
	Range                    float32        `json:"range"`
	Damage                   float32        `json:"damage"`
	SplashDamage             float32        `json:"splash_damage"`
	Interval                 float32        `json:"interval"`
	BulletType               int16          `json:"bullet_type"`
	BulletSpeed              float32        `json:"bullet_speed"`
	BulletLifetime           float32        `json:"bullet_lifetime"`
	BulletHitSize            float32        `json:"bullet_hit_size"`
	SplashRadius             float32        `json:"splash_radius"`
	BuildingDamageMultiplier float32        `json:"building_damage_multiplier"`
	ArmorMultiplier          float32        `json:"armor_multiplier"`
	MaxDamageFraction        float32        `json:"max_damage_fraction"`
	ShieldDamageMultiplier   float32        `json:"shield_damage_multiplier"`
	PierceDamageFactor       float32        `json:"pierce_damage_factor"`
	PierceArmor              bool           `json:"pierce_armor"`
	Pierce                   int32          `json:"pierce"`
	PierceBuilding           bool           `json:"pierce_building"`
	StatusID                 int16          `json:"status_id"`
	StatusName               string         `json:"status_name,omitempty"`
	StatusDuration           float32        `json:"status_duration"`
	FragBullets              int32          `json:"frag_bullets"`
	FragSpread               float32        `json:"frag_spread"`
	FragRandomSpread         float32        `json:"frag_random_spread"`
	FragAngle                float32        `json:"frag_angle"`
	FragVelocityMin          float32        `json:"frag_velocity_min"`
	FragVelocityMax          float32        `json:"frag_velocity_max"`
	FragLifeMin              float32        `json:"frag_life_min"`
	FragLifeMax              float32        `json:"frag_life_max"`
	FragBullet               *BulletProfile `json:"frag_bullet,omitempty"`
	TargetAir                bool           `json:"target_air"`
	TargetGround             bool           `json:"target_ground"`
	HitBuildings             bool           `json:"hit_buildings"`
	PreferBuildings          bool           `json:"prefer_buildings"`
	HitRadius                float32        `json:"hit_radius"`
	ShootStatusID            int16          `json:"shoot_status_id"`
	ShootStatusName          string         `json:"shoot_status_name,omitempty"`
	ShootStatusDuration      float32        `json:"shoot_status_duration"`
	ShootEffect              string         `json:"shoot_effect,omitempty"`
	SmokeEffect              string         `json:"smoke_effect,omitempty"`
	HitEffect                string         `json:"hit_effect,omitempty"`
	DespawnEffect            string         `json:"despawn_effect,omitempty"`
	Bullet                   *BulletProfile `json:"bullet,omitempty"`

	X                    float32 `json:"x"`
	Y                    float32 `json:"y"`
	ShootX               float32 `json:"shoot_x"`
	ShootY               float32 `json:"shoot_y"`
	Rotate               bool    `json:"rotate"`
	RotateSpeed          float32 `json:"rotate_speed"`
	BaseRotation         float32 `json:"base_rotation"`
	Mirror               bool    `json:"mirror"`
	Alternate            bool    `json:"alternate"`
	FlipSprite           bool    `json:"flip_sprite"`
	OtherSide            int32   `json:"other_side"`
	Controllable         bool    `json:"controllable"`
	AIControllable       bool    `json:"ai_controllable"`
	AutoTarget           bool    `json:"auto_target"`
	PredictTarget        bool    `json:"predict_target"`
	UseAttackRange       bool    `json:"use_attack_range"`
	AlwaysShooting       bool    `json:"always_shooting"`
	NoAttack             bool    `json:"no_attack"`
	TargetInterval       float32 `json:"target_interval"`
	TargetSwitchInterval float32 `json:"target_switch_interval"`
	ShootCone            float32 `json:"shoot_cone"`
	MinShootVelocity     float32 `json:"min_shoot_velocity"`
	Inaccuracy           float32 `json:"inaccuracy"`
	VelocityRnd          float32 `json:"velocity_rnd"`
	XRand                float32 `json:"x_rand"`
	YRand                float32 `json:"y_rand"`
	ExtraVelocity        float32 `json:"extra_velocity"`
	RotationLimit        float32 `json:"rotation_limit"`
	MinWarmup            float32 `json:"min_warmup"`
	ShootWarmupSpeed     float32 `json:"shoot_warmup_speed"`
	LinearWarmup         bool    `json:"linear_warmup"`
	AimChangeSpeed       float32 `json:"aim_change_speed"`
	Continuous           bool    `json:"continuous"`
	AlwaysContinuous     bool    `json:"always_continuous"`
	PointDefense         bool    `json:"point_defense"`
	RepairBeam           bool    `json:"repair_beam"`
	TargetUnits          bool    `json:"target_units"`
	TargetBuildings      bool    `json:"target_buildings"`
	RepairSpeed          float32 `json:"repair_speed"`
	FractionRepairSpeed  float32 `json:"fraction_repair_speed"`

	ShootPattern        string  `json:"shoot_pattern,omitempty"`
	ShootShots          int32   `json:"shoot_shots"`
	ShootFirstShotDelay float32 `json:"shoot_first_shot_delay"`
	ShootShotDelay      float32 `json:"shoot_shot_delay"`
	ShootSpread         float32 `json:"shoot_spread"`
	ShootBarrels        int32   `json:"shoot_barrels"`
	ShootBarrelOffset   int32   `json:"shoot_barrel_offset"`
	ShootPatternMirror  bool    `json:"shoot_pattern_mirror"`
	ShootHelixScl       float32 `json:"shoot_helix_scl"`
	ShootHelixMag       float32 `json:"shoot_helix_mag"`
	ShootHelixOffset    float32 `json:"shoot_helix_offset"`
}

type parsedWeaponMount struct {
	mount WeaponMountProfile
	stats parsedProfile
}

var (
	reWeaponDeclAny        = regexp.MustCompile(`(?m)new\s+([A-Za-z0-9_$.]*Weapon)\s*(?:\([^)]*\))?\s*\{\{`)
	reWeaponX              = regexp.MustCompile(`(?m)\bx\s*=\s*([^;]+);`)
	reWeaponY              = regexp.MustCompile(`(?m)\by\s*=\s*([^;]+);`)
	reWeaponShootX         = regexp.MustCompile(`(?m)\bshootX\s*=\s*([^;]+);`)
	reWeaponShootY         = regexp.MustCompile(`(?m)\bshootY\s*=\s*([^;]+);`)
	reWeaponRotateSpeed    = regexp.MustCompile(`(?m)\brotateSpeed\s*=\s*([^;]+);`)
	reWeaponBaseRotation   = regexp.MustCompile(`(?m)\bbaseRotation\s*=\s*([^;]+);`)
	reWeaponTargetInterval = regexp.MustCompile(`(?m)\btargetInterval\s*=\s*([^;]+);`)
	reWeaponTargetSwitch   = regexp.MustCompile(`(?m)\btargetSwitchInterval\s*=\s*([^;]+);`)
	reWeaponShootCone      = regexp.MustCompile(`(?m)\bshootCone\s*=\s*([^;]+);`)
	reWeaponMinShootVel    = regexp.MustCompile(`(?m)\bminShootVelocity\s*=\s*([^;]+);`)
	reWeaponInaccuracy     = regexp.MustCompile(`(?m)\binaccuracy\s*=\s*([^;]+);`)
	reWeaponVelocityRnd    = regexp.MustCompile(`(?m)\bvelocityRnd\s*=\s*([^;]+);`)
	reWeaponXRand          = regexp.MustCompile(`(?m)\bxRand\s*=\s*([^;]+);`)
	reWeaponYRand          = regexp.MustCompile(`(?m)\byRand\s*=\s*([^;]+);`)
	reWeaponExtraVelocity  = regexp.MustCompile(`(?m)\bextraVelocity\s*=\s*([^;]+);`)
	reWeaponRotationLimit  = regexp.MustCompile(`(?m)\brotationLimit\s*=\s*([^;]+);`)
	reWeaponMinWarmup      = regexp.MustCompile(`(?m)\bminWarmup\s*=\s*([^;]+);`)
	reWeaponShootWarmup    = regexp.MustCompile(`(?m)\bshootWarmupSpeed\s*=\s*([^;]+);`)
	reWeaponRepairSpeed    = regexp.MustCompile(`(?m)\brepairSpeed\s*=\s*([^;]+);`)
	reWeaponFractionRepair = regexp.MustCompile(`(?m)\bfractionRepairSpeed\s*=\s*([^;]+);`)
	reWeaponShootPattern   = regexp.MustCompile(`(?m)\bshoot\s*=\s*new\s+([A-Za-z0-9_$.]+)\s*\(([^)]*)\)\s*(?:\{\{)?`)
	reWeaponShootShots     = regexp.MustCompile(`(?m)\bshots\s*=\s*([^;]+);`)
	reWeaponShotDelay      = regexp.MustCompile(`(?m)\bshotDelay\s*=\s*([^;]+);`)
	reWeaponFirstShotDelay = regexp.MustCompile(`(?m)\bfirstShotDelay\s*=\s*([^;]+);`)
	reWeaponShootSpread    = regexp.MustCompile(`(?m)\bspread\s*=\s*([^;]+);`)
	reWeaponShootBarrels   = regexp.MustCompile(`(?m)\bbarrels\s*=\s*([^;]+);`)
	reWeaponShootBarrelOff = regexp.MustCompile(`(?m)\bbarrelOffset\s*=\s*([^;]+);`)
	reWeaponShootHelixScl  = regexp.MustCompile(`(?m)\bscl\s*=\s*([^;]+);`)
	reWeaponShootHelixMag  = regexp.MustCompile(`(?m)\bmag\s*=\s*([^;]+);`)
	reWeaponShootHelixOff  = regexp.MustCompile(`(?m)\boffset\s*=\s*([^;]+);`)
)

func extractWeaponMountProfiles(body string, statusLookup map[string]statusLookupEntry) []WeaponMountProfile {
	parsed := extractParsedWeaponMounts(body, statusLookup)
	if len(parsed) == 0 {
		return nil
	}
	out := make([]WeaponMountProfile, 0, len(parsed))
	for _, pm := range parsed {
		out = append(out, copyWeaponMountProfile(pm.mount))
	}
	return out
}

func extractParsedWeaponMounts(body string, statusLookup map[string]statusLookupEntry) []parsedWeaponMount {
	matches := reWeaponDeclAny.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make([]parsedWeaponMount, 0, len(matches))
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		className := strings.TrimSpace(body[m[2]:m[3]])
		wb, ok := extractInitBody(body, m[1])
		if !ok {
			continue
		}
		stats := parseCommonProfile(wb, statusLookup)
		mount := defaultWeaponMountProfile(className)
		applyParsedProfileToMount(&mount, stats)
		parseWeaponMountMeta(&mount, wb)
		out = append(out, parsedWeaponMount{
			mount: mount,
			stats: stats,
		})
	}
	return expandMirroredWeaponMounts(out)
}

func mergeParsedWeaponMounts(mounts []parsedWeaponMount) parsedProfile {
	out := parsedProfile{
		fireMode:                 "projectile",
		buildingDamageMultiplier: 1,
		armorMultiplier:          1,
		shieldDamageMultiplier:   1,
		targetAir:                true,
		targetGround:             true,
		hitBuildings:             true,
	}
	found := false
	for _, pm := range mounts {
		if pm.mount.NoAttack || pm.mount.PointDefense || pm.mount.RepairBeam {
			continue
		}
		if (pm.stats.damage <= 0 && pm.stats.splashDamage <= 0) || pm.mount.Interval <= 0 {
			continue
		}
		p := pm.stats
		p.interval = pm.mount.Interval
		p.targetAir = pm.mount.TargetAir
		p.targetGround = pm.mount.TargetGround
		p.hitBuildings = pm.mount.HitBuildings
		if !found {
			out = p
			found = true
			continue
		}
		out = mergeParsedProfiles(out, p)
	}
	if !found {
		return parsedProfile{}
	}
	return out
}

func defaultWeaponMountProfile(className string) WeaponMountProfile {
	name := strings.ToLower(strings.TrimSpace(className))
	out := WeaponMountProfile{
		ClassName:                className,
		FireMode:                 "projectile",
		Interval:                 1.0 / 60.0,
		BuildingDamageMultiplier: 1,
		TargetAir:                true,
		TargetGround:             true,
		HitBuildings:             true,
		X:                        5,
		Y:                        0,
		ShootX:                   0,
		ShootY:                   3,
		Rotate:                   false,
		RotateSpeed:              20,
		BaseRotation:             0,
		Mirror:                   true,
		Alternate:                true,
		FlipSprite:               false,
		OtherSide:                -1,
		Controllable:             true,
		AIControllable:           true,
		AutoTarget:               false,
		PredictTarget:            true,
		UseAttackRange:           true,
		AlwaysShooting:           false,
		NoAttack:                 false,
		TargetInterval:           40.0 / 60.0,
		TargetSwitchInterval:     70.0 / 60.0,
		ShootCone:                5,
		MinShootVelocity:         -1,
		RotationLimit:            361,
		ShootWarmupSpeed:         0.1,
		AimChangeSpeed:           0,
		Continuous:               false,
		AlwaysContinuous:         false,
		TargetUnits:              true,
		TargetBuildings:          false,
		RepairSpeed:              0.3,
		FractionRepairSpeed:      0,
		ShootShots:               1,
		ShootBarrels:             2,
	}

	switch {
	case strings.Contains(name, "pointdefenseweapon"):
		out.PointDefense = true
		out.Rotate = true
		out.Controllable = false
		out.AutoTarget = true
		out.PredictTarget = false
		out.UseAttackRange = false
		out.TargetInterval = 10.0 / 60.0
	case strings.Contains(name, "repairbeamweapon"):
		out.RepairBeam = true
		out.Rotate = true
		out.Controllable = false
		out.AutoTarget = true
		out.PredictTarget = false
		out.UseAttackRange = false
		out.NoAttack = true
		out.Interval = 1.0 / 60.0
	case strings.Contains(name, "buildweapon"), strings.Contains(name, "mineweapon"):
		out.Rotate = true
		out.PredictTarget = false
		out.UseAttackRange = false
		out.NoAttack = true
		out.HitBuildings = false
	}

	return out
}

func applyParsedProfileToMount(dst *WeaponMountProfile, src parsedProfile) {
	if dst == nil {
		return
	}
	dst.FireMode = src.fireMode
	dst.Range = src.rangeV
	dst.Damage = src.damage
	dst.SplashDamage = src.splashDamage
	if src.interval > 0 {
		dst.Interval = src.interval
	}
	dst.BulletType = src.bulletType
	dst.BulletSpeed = src.bulletSpeed
	dst.BulletLifetime = src.bulletLifetime
	dst.BulletHitSize = src.bulletHitSize
	dst.SplashRadius = src.splashRadius
	dst.BuildingDamageMultiplier = src.buildingDamageMultiplier
	dst.ArmorMultiplier = src.armorMultiplier
	dst.MaxDamageFraction = src.maxDamageFraction
	dst.ShieldDamageMultiplier = src.shieldDamageMultiplier
	dst.PierceDamageFactor = src.pierceDamageFactor
	dst.PierceArmor = src.pierceArmor
	dst.Pierce = src.pierce
	dst.PierceBuilding = src.pierceBuilding
	dst.StatusID = src.statusID
	dst.StatusName = src.statusName
	dst.StatusDuration = src.statusDuration
	dst.FragBullets = src.fragBullets
	dst.FragSpread = src.fragSpread
	dst.FragRandomSpread = src.fragRandomSpread
	dst.FragAngle = src.fragAngle
	dst.FragVelocityMin = src.fragVelocityMin
	dst.FragVelocityMax = src.fragVelocityMax
	dst.FragLifeMin = src.fragLifeMin
	dst.FragLifeMax = src.fragLifeMax
	dst.FragBullet = cloneBulletProfile(src.fragBullet)
	dst.TargetAir = src.targetAir
	dst.TargetGround = src.targetGround
	dst.HitBuildings = src.hitBuildings
	dst.PreferBuildings = src.preferBuildings
	dst.HitRadius = src.hitRadius
	dst.ShootStatusID = src.shootStatusID
	dst.ShootStatusName = src.shootStatusName
	dst.ShootStatusDuration = src.shootStatusDuration
	dst.ShootEffect = src.shootEffect
	dst.SmokeEffect = src.smokeEffect
	dst.HitEffect = src.hitEffect
	dst.DespawnEffect = src.despawnEffect
	dst.Bullet = cloneBulletProfile(src.bullet)
}

func parseWeaponMountMeta(dst *WeaponMountProfile, body string) {
	if dst == nil {
		return
	}
	flat := stripNestedInitBodies(body)

	if v, ok := lastValue(flat, reWeaponX); ok {
		dst.X = v
	}
	if v, ok := lastValue(flat, reWeaponY); ok {
		dst.Y = v
	}
	if v, ok := lastValue(flat, reWeaponShootX); ok {
		dst.ShootX = v
	}
	if v, ok := lastValue(flat, reWeaponShootY); ok {
		dst.ShootY = v
	}
	if v, ok := lastValue(flat, reWeaponRotateSpeed); ok && v > 0 {
		dst.RotateSpeed = v
	}
	if v, ok := lastValue(flat, reWeaponBaseRotation); ok {
		dst.BaseRotation = v
	}
	if v, ok := lastValue(flat, reWeaponTargetInterval); ok && v >= 0 {
		dst.TargetInterval = v / 60
	}
	if v, ok := lastValue(flat, reWeaponTargetSwitch); ok && v >= 0 {
		dst.TargetSwitchInterval = v / 60
	}
	if v, ok := lastValue(flat, reWeaponShootCone); ok && v >= 0 {
		dst.ShootCone = v
	}
	if v, ok := lastValue(flat, reWeaponMinShootVel); ok {
		dst.MinShootVelocity = v
	}
	if v, ok := lastValue(flat, reWeaponInaccuracy); ok && v >= 0 {
		dst.Inaccuracy = v
	}
	if v, ok := lastValue(flat, reWeaponVelocityRnd); ok && v >= 0 {
		dst.VelocityRnd = v
	}
	if v, ok := lastValue(flat, reWeaponXRand); ok && v >= 0 {
		dst.XRand = v
	}
	if v, ok := lastValue(flat, reWeaponYRand); ok && v >= 0 {
		dst.YRand = v
	}
	if v, ok := lastValue(flat, reWeaponExtraVelocity); ok {
		dst.ExtraVelocity = v
	}
	if v, ok := lastValue(flat, reWeaponRotationLimit); ok && v >= 0 {
		dst.RotationLimit = v
	}
	if v, ok := lastValue(flat, reWeaponMinWarmup); ok && v >= 0 {
		dst.MinWarmup = v
	}
	if v, ok := lastValue(flat, reWeaponShootWarmup); ok && v >= 0 {
		dst.ShootWarmupSpeed = v
	}
	if v, ok := lastValue(flat, reWeaponRepairSpeed); ok && v >= 0 {
		dst.RepairSpeed = v
	}
	if v, ok := lastValue(flat, reWeaponFractionRepair); ok && v >= 0 {
		dst.FractionRepairSpeed = v
	}
	if v, ok := lastValue(flat, reAimChangeSpeed); ok && v >= 0 {
		dst.AimChangeSpeed = v
	}

	if v, ok := lastValue(body, reWeaponShootShots); ok && v > 0 {
		dst.ShootShots = int32(v)
	}
	if v, ok := lastValue(body, reWeaponFirstShotDelay); ok && v >= 0 {
		dst.ShootFirstShotDelay = v / 60
	}
	if v, ok := lastValue(body, reWeaponShotDelay); ok && v >= 0 {
		dst.ShootShotDelay = v / 60
	}

	if className, args, shootBody, ok := findShootPatternSpec(body); ok {
		dst.ShootPattern = className
		switch className {
		case "spread":
			if len(args) >= 1 {
				if v, vok := evalNumericExpr(args[0]); vok && v > 0 {
					dst.ShootShots = int32(v)
				}
			}
			if len(args) >= 2 {
				if v, vok := evalNumericExpr(args[1]); vok {
					dst.ShootSpread = float32(v)
				}
			}
		case "alternate":
			if len(args) >= 1 {
				if v, vok := evalNumericExpr(args[0]); vok {
					dst.ShootSpread = float32(v)
				}
			}
			if v, ok := lastBool(shootBody, "mirror"); ok {
				dst.ShootPatternMirror = v
			}
		case "helix":
			if dst.ShootShots <= 1 {
				dst.ShootShots = 2
			}
			dst.ShootHelixScl = 2
			dst.ShootHelixMag = 1.5
			dst.ShootHelixOffset = float32(math.Pi * 1.25)
			if len(args) >= 1 {
				if v, vok := evalNumericExpr(args[0]); vok && v > 0 {
					dst.ShootHelixScl = float32(v)
				}
			}
			if len(args) >= 2 {
				if v, vok := evalNumericExpr(args[1]); vok {
					dst.ShootHelixMag = float32(v)
				}
			}
			if len(args) >= 3 {
				if v, vok := evalNumericExpr(args[2]); vok {
					dst.ShootHelixOffset = float32(v)
				}
			}
			if v, ok := lastValue(shootBody, reWeaponShootHelixScl); ok && v > 0 {
				dst.ShootHelixScl = v
			}
			if v, ok := lastValue(shootBody, reWeaponShootHelixMag); ok {
				dst.ShootHelixMag = v
			}
			if v, ok := lastValue(shootBody, reWeaponShootHelixOff); ok {
				dst.ShootHelixOffset = v
			}
		}
	}
	if v, ok := lastValue(body, reWeaponShootSpread); ok {
		dst.ShootSpread = v
	}
	if v, ok := lastValue(body, reWeaponShootBarrels); ok && v > 0 {
		dst.ShootBarrels = int32(v)
	}
	if v, ok := lastValue(body, reWeaponShootBarrelOff); ok {
		dst.ShootBarrelOffset = int32(v)
	}

	if v, ok := lastBool(flat, "rotate"); ok {
		dst.Rotate = v
	}
	if v, ok := lastBool(flat, "mirror"); ok {
		dst.Mirror = v
	}
	if v, ok := lastBool(flat, "alternate"); ok {
		dst.Alternate = v
	}
	if v, ok := lastBool(flat, "controllable"); ok {
		dst.Controllable = v
	}
	if v, ok := lastBool(flat, "aiControllable"); ok {
		dst.AIControllable = v
	}
	if v, ok := lastBool(flat, "autoTarget"); ok {
		dst.AutoTarget = v
	}
	if v, ok := lastBool(flat, "predictTarget"); ok {
		dst.PredictTarget = v
	}
	if v, ok := lastBool(flat, "useAttackRange"); ok {
		dst.UseAttackRange = v
	}
	if v, ok := lastBool(flat, "alwaysShooting"); ok {
		dst.AlwaysShooting = v
	}
	if v, ok := lastBool(flat, "noAttack"); ok {
		dst.NoAttack = v
	}
	if v, ok := lastBool(flat, "continuous"); ok {
		dst.Continuous = v
	}
	if v, ok := lastBool(flat, "alwaysContinuous"); ok {
		dst.AlwaysContinuous = v
	}
	if v, ok := lastBool(flat, "linearWarmup"); ok {
		dst.LinearWarmup = v
	}
	if v, ok := lastBool(flat, "targetUnits"); ok {
		dst.TargetUnits = v
	}
	if v, ok := lastBool(flat, "targetBuildings"); ok {
		dst.TargetBuildings = v
	}
}

func expandMirroredWeaponMounts(src []parsedWeaponMount) []parsedWeaponMount {
	if len(src) == 0 {
		return nil
	}
	out := make([]parsedWeaponMount, 0, len(src)*2)
	for _, pm := range src {
		mirror := pm.mount.Mirror
		base := parsedWeaponMount{
			mount: copyWeaponMountProfile(pm.mount),
			stats: cloneParsedProfile(pm.stats),
		}
		if mirror {
			if base.mount.Interval > 0 {
				base.mount.Interval *= 2
			}
			if base.stats.interval > 0 {
				base.stats.interval *= 2
			}
			copy := parsedWeaponMount{
				mount: copyWeaponMountProfile(base.mount),
				stats: cloneParsedProfile(base.stats),
			}
			base.mount.Mirror = false
			copy.mount.Mirror = false
			flipWeaponMount(&copy.mount)
			base.mount.OtherSide = int32(len(out) + 1)
			copy.mount.OtherSide = int32(len(out))
			out = append(out, base, copy)
			continue
		}
		base.mount.OtherSide = -1
		out = append(out, base)
	}
	return out
}

func flipWeaponMount(m *WeaponMountProfile) {
	if m == nil {
		return
	}
	m.X *= -1
	m.ShootX *= -1
	m.BaseRotation *= -1
	m.FlipSprite = !m.FlipSprite
	if m.ShootPattern == "alternate" {
		m.ShootPatternMirror = !m.ShootPatternMirror
	}
}

func copyWeaponMountProfile(src WeaponMountProfile) WeaponMountProfile {
	out := src
	out.FragBullet = cloneBulletProfile(src.FragBullet)
	out.Bullet = cloneBulletProfile(src.Bullet)
	return out
}

func cloneParsedProfile(src parsedProfile) parsedProfile {
	out := src
	out.fragBullet = cloneBulletProfile(src.fragBullet)
	out.bullet = cloneBulletProfile(src.bullet)
	return out
}

func stripNestedInitBodies(body string) string {
	if body == "" {
		return body
	}
	var out strings.Builder
	for i := 0; i < len(body); {
		if i+1 < len(body) && body[i] == '{' && body[i+1] == '{' {
			depth := 2
			i += 2
			for i < len(body) {
				switch body[i] {
				case '{':
					depth++
				case '}':
					depth--
					if depth == 0 {
						i++
						goto next
					}
				}
				i++
			}
			break
		}
		out.WriteByte(body[i])
		i++
	next:
	}
	return out.String()
}

func findShootPatternSpec(body string) (string, []string, string, bool) {
	m := reWeaponShootPattern.FindStringSubmatchIndex(body)
	if len(m) < 6 {
		return "", nil, "", false
	}
	className := strings.ToLower(strings.TrimSpace(body[m[2]:m[3]]))
	args := splitArgs(body[m[4]:m[5]])
	shootBody := ""
	if strings.HasSuffix(strings.TrimSpace(body[m[0]:m[1]]), "{{") {
		if sub, ok := extractInitBody(body, m[1]); ok {
			shootBody = sub
		}
	}
	switch {
	case strings.Contains(className, "shootspread"):
		return "spread", args, shootBody, true
	case strings.Contains(className, "shootalternate"):
		return "alternate", args, shootBody, true
	case strings.Contains(className, "shoothelix"):
		return "helix", args, shootBody, true
	default:
		return className, args, shootBody, true
	}
}

func lastBool(body, key string) (bool, bool) {
	re := regexp.MustCompile(`(?m)\b` + regexp.QuoteMeta(strings.TrimSpace(key)) + `\s*=\s*(true|false)\s*;`)
	ms := re.FindAllStringSubmatch(body, -1)
	if len(ms) == 0 {
		return false, false
	}
	return strings.EqualFold(ms[len(ms)-1][1], "true"), true
}
