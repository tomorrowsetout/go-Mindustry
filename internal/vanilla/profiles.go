package vanilla

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

type UnitProfile struct {
	Name                     string               `json:"name"`
	FireMode                 string               `json:"fire_mode"`
	Range                    float32              `json:"range"`
	Damage                   float32              `json:"damage"`
	SplashDamage             float32              `json:"splash_damage"`
	Interval                 float32              `json:"interval"`
	BulletType               int16                `json:"bullet_type"`
	BulletSpeed              float32              `json:"bullet_speed"`
	BulletLifetime           float32              `json:"bullet_lifetime"`
	BulletHitSize            float32              `json:"bullet_hit_size"`
	SplashRadius             float32              `json:"splash_radius"`
	BuildingDamageMultiplier float32              `json:"building_damage_multiplier"`
	Pierce                   int32                `json:"pierce"`
	PierceBuilding           bool                 `json:"pierce_building"`
	StatusID                 int16                `json:"status_id"`
	StatusName               string               `json:"status_name,omitempty"`
	StatusDuration           float32              `json:"status_duration"`
	FragBullets              int32                `json:"frag_bullets"`
	FragSpread               float32              `json:"frag_spread"`
	FragRandomSpread         float32              `json:"frag_random_spread"`
	FragAngle                float32              `json:"frag_angle"`
	FragVelocityMin          float32              `json:"frag_velocity_min"`
	FragVelocityMax          float32              `json:"frag_velocity_max"`
	FragLifeMin              float32              `json:"frag_life_min"`
	FragLifeMax              float32              `json:"frag_life_max"`
	FragBullet               *BulletProfile       `json:"frag_bullet,omitempty"`
	TargetAir                bool                 `json:"target_air"`
	TargetGround             bool                 `json:"target_ground"`
	TargetPriority           string               `json:"target_priority"`
	HitBuildings             bool                 `json:"hit_buildings"`
	PreferBuildings          bool                 `json:"prefer_buildings"`
	HitRadius                float32              `json:"hit_radius"`
	ShootStatusID            int16                `json:"shoot_status_id"`
	ShootStatusName          string               `json:"shoot_status_name,omitempty"`
	ShootStatusDuration      float32              `json:"shoot_status_duration"`
	ShootEffect              string               `json:"shoot_effect,omitempty"`
	SmokeEffect              string               `json:"smoke_effect,omitempty"`
	HitEffect                string               `json:"hit_effect,omitempty"`
	DespawnEffect            string               `json:"despawn_effect,omitempty"`
	Bullet                   *BulletProfile       `json:"bullet,omitempty"`
	Mounts                   []WeaponMountProfile `json:"mounts,omitempty"`
}

type TurretProfile struct {
	ClassName                string         `json:"class_name,omitempty"`
	Name                     string         `json:"name"`
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
	TargetPriority           string         `json:"target_priority"`
	HitBuildings             bool           `json:"hit_buildings"`
	ShootEffect              string         `json:"shoot_effect,omitempty"`
	SmokeEffect              string         `json:"smoke_effect,omitempty"`
	HitEffect                string         `json:"hit_effect,omitempty"`
	DespawnEffect            string         `json:"despawn_effect,omitempty"`
	ContinuousHold           bool           `json:"continuous_hold"`
	AimChangeSpeed           float32        `json:"aim_change_speed"`
	ShootDuration            float32        `json:"shoot_duration"`
	Bullet                   *BulletProfile `json:"bullet,omitempty"`
}

type BulletProfile struct {
	ClassName                string         `json:"class_name,omitempty"`
	Damage                   float32        `json:"damage"`
	SplashDamage             float32        `json:"splash_damage"`
	BulletType               int16          `json:"bullet_type"`
	Speed                    float32        `json:"speed"`
	Lifetime                 float32        `json:"lifetime"`
	HitSize                  float32        `json:"hit_size"`
	SplashRadius             float32        `json:"splash_radius"`
	BuildingDamageMultiplier float32        `json:"building_damage_multiplier"`
	Pierce                   int32          `json:"pierce"`
	PierceBuilding           bool           `json:"pierce_building"`
	StatusID                 int16          `json:"status_id"`
	StatusName               string         `json:"status_name,omitempty"`
	StatusDuration           float32        `json:"status_duration"`
	HitBuildings             bool           `json:"hit_buildings"`
	TargetAir                bool           `json:"target_air"`
	TargetGround             bool           `json:"target_ground"`
	ShootEffect              string         `json:"shoot_effect,omitempty"`
	SmokeEffect              string         `json:"smoke_effect,omitempty"`
	HitEffect                string         `json:"hit_effect,omitempty"`
	DespawnEffect            string         `json:"despawn_effect,omitempty"`
	Length                   float32        `json:"length"`
	DamageInterval           float32        `json:"damage_interval"`
	OptimalLifeFract         float32        `json:"optimal_life_fract"`
	FadeTime                 float32        `json:"fade_time"`
	FragBullets              int32          `json:"frag_bullets"`
	FragSpread               float32        `json:"frag_spread"`
	FragRandomSpread         float32        `json:"frag_random_spread"`
	FragAngle                float32        `json:"frag_angle"`
	FragVelocityMin          float32        `json:"frag_velocity_min"`
	FragVelocityMax          float32        `json:"frag_velocity_max"`
	FragLifeMin              float32        `json:"frag_life_min"`
	FragLifeMax              float32        `json:"frag_life_max"`
	FragBullet               *BulletProfile `json:"frag_bullet,omitempty"`
}

type StatusProfile struct {
	ID                   int16    `json:"id"`
	Name                 string   `json:"name"`
	DamageMultiplier     float32  `json:"damage_multiplier"`
	HealthMultiplier     float32  `json:"health_multiplier"`
	SpeedMultiplier      float32  `json:"speed_multiplier"`
	ReloadMultiplier     float32  `json:"reload_multiplier"`
	BuildSpeedMultiplier float32  `json:"build_speed_multiplier"`
	DragMultiplier       float32  `json:"drag_multiplier"`
	TransitionDamage     float32  `json:"transition_damage"`
	Damage               float32  `json:"damage"`
	IntervalDamageTime   float32  `json:"interval_damage_time"`
	IntervalDamage       float32  `json:"interval_damage"`
	IntervalDamagePierce bool     `json:"interval_damage_pierce"`
	Disarm               bool     `json:"disarm"`
	Permanent            bool     `json:"permanent"`
	Reactive             bool     `json:"reactive"`
	Dynamic              bool     `json:"dynamic"`
	Opposites            []string `json:"opposites,omitempty"`
	Affinities           []string `json:"affinities,omitempty"`
}

type BlockRequirementProfile struct {
	Item   string  `json:"item"`
	ItemID int16   `json:"item_id"`
	Amount int32   `json:"amount"`
	Cost   float32 `json:"cost"`
}

type BlockProfile struct {
	Name                string                    `json:"name"`
	BuildCostMultiplier float32                   `json:"build_cost_multiplier"`
	BuildTimeSec        float32                   `json:"build_time_sec"`
	Requirements        []BlockRequirementProfile `json:"requirements"`
}

type ProfilesFile struct {
	UnitsByName []UnitProfile   `json:"units_by_name"`
	Turrets     []TurretProfile `json:"turrets"`
	Blocks      []BlockProfile  `json:"blocks"`
	Statuses    []StatusProfile `json:"statuses"`
}

func GenerateProfiles(repoRoot, outPath string) (int, int, int, error) {
	unitPath, blocksPath := resolveSourcePaths(repoRoot)
	itemsPath := filepath.Join(filepath.Dir(unitPath), "Items.java")
	statusPath := filepath.Join(filepath.Dir(unitPath), "StatusEffects.java")
	unitSrc, err := os.ReadFile(unitPath)
	if err != nil {
		return 0, 0, 0, err
	}
	blockSrc, err := os.ReadFile(blocksPath)
	if err != nil {
		return 0, 0, 0, err
	}
	itemSrc, err := os.ReadFile(itemsPath)
	if err != nil {
		return 0, 0, 0, err
	}
	statusSrc, err := os.ReadFile(statusPath)
	if err != nil {
		return 0, 0, 0, err
	}

	itemsByVar := extractItems(string(itemSrc))
	statuses, statusLookup := extractStatuses(string(statusSrc))
	units := extractUnits(string(unitSrc), statusLookup)
	turrets := extractTurrets(string(blockSrc), statusLookup)
	blocks := extractBlocks(string(blockSrc), itemsByVar)
	payload := ProfilesFile{
		UnitsByName: units,
		Turrets:     turrets,
		Blocks:      blocks,
		Statuses:    statuses,
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return 0, 0, 0, err
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return 0, 0, 0, err
	}
	if err := os.WriteFile(outPath, b, 0644); err != nil {
		return 0, 0, 0, err
	}
	return len(units), len(turrets), len(blocks), nil
}

func resolveSourcePaths(repoRoot string) (string, string) {
	a := filepath.Join(repoRoot, "core", "src", "mindustry", "content")
	if st, err := os.Stat(a); err == nil && st.IsDir() {
		return filepath.Join(a, "UnitTypes.java"), filepath.Join(a, "Blocks.java")
	}
	b := filepath.Join(repoRoot, "..", "core", "src", "mindustry", "content")
	return filepath.Join(b, "UnitTypes.java"), filepath.Join(b, "Blocks.java")
}

var (
	reItemDecl   = regexp.MustCompile(`(?m)(\w+)\s*=\s*new\s+Item\s*\(\s*"([^"]+)"`)
	reUnitDecl   = regexp.MustCompile(`(?m)(\w+)\s*=\s*new\s+UnitType\("([^"]+)"\)\s*\{\{`)
	reBlockDecl  = regexp.MustCompile(`(?m)(\w+)\s*=\s*new\s+[A-Za-z0-9_$.]+\("([^"]+)"\)\s*\{\{`)
	reTurretDecl = regexp.MustCompile(`(?m)(\w+)\s*=\s*new\s+([A-Za-z0-9_$.]+)\("([^"]+)"\)\s*\{\{`)

	reRange                 = regexp.MustCompile(`(?m)\brange\s*=\s*([^;]+);`)
	reMaxRange              = regexp.MustCompile(`(?m)\bmaxRange\s*=\s*([^;]+);`)
	reReload                = regexp.MustCompile(`(?m)\breload\s*=\s*([^;]+);`)
	reDamage                = regexp.MustCompile(`(?m)\bdamage\s*=\s*([^;]+);`)
	reSplashDamage          = regexp.MustCompile(`(?m)\bsplashDamage\s*=\s*([^;]+);`)
	reSplashRadius          = regexp.MustCompile(`(?m)\bsplashDamageRadius\s*=\s*([^;]+);`)
	reLength                = regexp.MustCompile(`(?m)\blength\s*=\s*([^;]+);`)
	reSpeed                 = regexp.MustCompile(`(?m)\bspeed\s*=\s*([^;]+);`)
	reAimChangeSpeed        = regexp.MustCompile(`(?m)\baimChangeSpeed\s*=\s*([^;]+);`)
	reShootDuration         = regexp.MustCompile(`(?m)\bshootDuration\s*=\s*([^;]+);`)
	reDamageInterval        = regexp.MustCompile(`(?m)\bdamageInterval\s*=\s*([^;]+);`)
	reOptimalLifeFract      = regexp.MustCompile(`(?m)\boptimalLifeFract\s*=\s*([^;]+);`)
	reFadeTime              = regexp.MustCompile(`(?m)\bfadeTime\s*=\s*([^;]+);`)
	reTargetAir             = regexp.MustCompile(`(?m)\btargetAir\s*=\s*(true|false)\s*;`)
	reTargetGround          = regexp.MustCompile(`(?m)\btargetGround\s*=\s*(true|false)\s*;`)
	rePierceCap             = regexp.MustCompile(`(?m)\bpierceCap\s*=\s*([^;]+);`)
	reWeaponDecl            = regexp.MustCompile(`(?m)new\s+Weapon\([^)]*\)\s*\{\{`)
	reBulletCtor            = regexp.MustCompile(`(?m)new\s+[A-Za-z0-9_$.]*BulletType\s*\(([^)]*)\)`)
	reItemCost              = regexp.MustCompile(`(?m)\bcost\s*=\s*([^;]+);`)
	reBuildCostMul          = regexp.MustCompile(`(?m)\bbuildCostMultiplier\s*=\s*([^;]+);`)
	reBuildTime             = regexp.MustCompile(`(?m)\bbuildTime\s*=\s*([^;]+);`)
	reReqWith               = regexp.MustCompile(`(?s)requirements\s*\([^;]*?\bwith\s*\((.*?)\)\s*\)\s*;`)
	reReqItemPair           = regexp.MustCompile(`Items\.(\w+)\s*,\s*([^,\)]+)`)
	reEffectAssign          = regexp.MustCompile(`(?m)\b(shootEffect|smokeEffect|hitEffect|despawnEffect)\s*=\s*(?:[A-Za-z0-9_$.]+\.)?([A-Za-z0-9_]+)\s*;`)
	reEffectChain           = regexp.MustCompile(`(?m)\b(shootEffect|smokeEffect|hitEffect|despawnEffect)\s*=\s*(shootEffect|smokeEffect|hitEffect|despawnEffect)\s*=\s*(?:[A-Za-z0-9_$.]+\.)?([A-Za-z0-9_]+)\s*;`)
	reScaleDamageEfficiency = regexp.MustCompile(`(?m)\bscaleDamageEfficiency\s*=\s*(true|false)\s*;`)
)

type itemMeta struct {
	ID   int16
	Name string
	Cost float32
}

func extractItems(src string) map[string]itemMeta {
	matches := reItemDecl.FindAllStringSubmatchIndex(src, -1)
	out := make(map[string]itemMeta, len(matches))
	var nextID int16
	for _, m := range matches {
		varName := strings.TrimSpace(src[m[2]:m[3]])
		name := strings.ToLower(strings.TrimSpace(src[m[4]:m[5]]))
		body, ok := "", false
		if off := strings.Index(src[m[1]:], "{{"); off >= 0 {
			body, ok = extractInitBody(src, m[1]+off+2)
		}
		cost := float32(1.0)
		if ok {
			if v, vok := lastValue(body, reItemCost); vok && v > 0 {
				cost = v
			}
		}
		out[varName] = itemMeta{ID: nextID, Name: name, Cost: cost}
		nextID++
	}
	return out
}

func extractBlocks(src string, items map[string]itemMeta) []BlockProfile {
	matches := reBlockDecl.FindAllStringSubmatchIndex(src, -1)
	out := make([]BlockProfile, 0, len(matches))
	for _, m := range matches {
		name := strings.ToLower(strings.TrimSpace(src[m[4]:m[5]]))
		if name == "" {
			continue
		}
		body, ok := extractInitBody(src, m[1])
		if !ok {
			continue
		}
		req := parseRequirements(body, items)
		if len(req) == 0 {
			continue
		}
		mul := float32(1.0)
		if v, vok := lastValue(body, reBuildCostMul); vok && v > 0 {
			mul = v
		}
		buildTimeSec := float32(0)
		if v, vok := lastValue(body, reBuildTime); vok && v > 0 {
			buildTimeSec = (v * mul) / 60.0
		} else {
			sum := float32(0)
			for _, r := range req {
				sum += float32(r.Amount) * r.Cost
			}
			buildTimeSec = (sum * mul) / 60.0
		}
		out = append(out, BlockProfile{
			Name:                name,
			BuildCostMultiplier: mul,
			BuildTimeSec:        buildTimeSec,
			Requirements:        req,
		})
	}
	return out
}

func parseRequirements(body string, items map[string]itemMeta) []BlockRequirementProfile {
	ms := reReqWith.FindAllStringSubmatch(body, -1)
	if len(ms) == 0 {
		return nil
	}
	seen := map[string]int{}
	out := make([]BlockRequirementProfile, 0, 6)
	for _, m := range ms {
		if len(m) < 2 {
			continue
		}
		pairs := reReqItemPair.FindAllStringSubmatch(m[1], -1)
		for _, p := range pairs {
			if len(p) < 3 {
				continue
			}
			meta, ok := items[strings.TrimSpace(p[1])]
			if !ok {
				continue
			}
			v, vok := evalNumericExpr(p[2])
			if !vok {
				continue
			}
			amount := int32(v + 0.5)
			if amount <= 0 {
				continue
			}
			key := meta.Name
			if idx, exists := seen[key]; exists {
				out[idx].Amount += amount
				continue
			}
			seen[key] = len(out)
			out = append(out, BlockRequirementProfile{
				Item:   meta.Name,
				ItemID: meta.ID,
				Amount: amount,
				Cost:   meta.Cost,
			})
		}
	}
	return out
}

func extractUnits(src string, statusLookup map[string]statusLookupEntry) []UnitProfile {
	matches := reUnitDecl.FindAllStringSubmatchIndex(src, -1)
	out := make([]UnitProfile, 0, len(matches))
	for _, m := range matches {
		name := src[m[4]:m[5]]
		bodyStart := m[1]
		body, ok := extractInitBody(src, bodyStart)
		if !ok {
			continue
		}
		p := parseCommonProfile(body, statusLookup)
		wp := parseWeaponsProfile(body, statusLookup)
		p = mergeParsedProfiles(p, wp)
		if (p.damage <= 0 && p.splashDamage <= 0) || p.interval <= 0 {
			continue
		}
		out = append(out, UnitProfile{
			Name:                     strings.ToLower(strings.TrimSpace(name)),
			FireMode:                 p.fireMode,
			Range:                    p.rangeV,
			Damage:                   p.damage,
			SplashDamage:             p.splashDamage,
			Interval:                 p.interval,
			BulletType:               p.bulletType,
			BulletSpeed:              p.bulletSpeed,
			BulletLifetime:           p.bulletLifetime,
			BulletHitSize:            p.bulletHitSize,
			SplashRadius:             p.splashRadius,
			BuildingDamageMultiplier: p.buildingDamageMultiplier,
			Pierce:                   p.pierce,
			PierceBuilding:           p.pierceBuilding,
			StatusID:                 p.statusID,
			StatusName:               p.statusName,
			StatusDuration:           p.statusDuration,
			FragBullets:              p.fragBullets,
			FragSpread:               p.fragSpread,
			FragRandomSpread:         p.fragRandomSpread,
			FragAngle:                p.fragAngle,
			FragVelocityMin:          p.fragVelocityMin,
			FragVelocityMax:          p.fragVelocityMax,
			FragLifeMin:              p.fragLifeMin,
			FragLifeMax:              p.fragLifeMax,
			FragBullet:               cloneBulletProfile(p.fragBullet),
			TargetAir:                p.targetAir,
			TargetGround:             p.targetGround,
			TargetPriority:           "nearest",
			HitBuildings:             p.hitBuildings,
			PreferBuildings:          p.preferBuildings,
			HitRadius:                p.hitRadius,
			ShootStatusID:            p.shootStatusID,
			ShootStatusName:          p.shootStatusName,
			ShootStatusDuration:      p.shootStatusDuration,
			ShootEffect:              p.shootEffect,
			SmokeEffect:              p.smokeEffect,
			HitEffect:                p.hitEffect,
			DespawnEffect:            p.despawnEffect,
			Bullet:                   cloneBulletProfile(p.bullet),
			Mounts:                   extractWeaponMountProfiles(body, statusLookup),
		})
	}
	return out
}

func parseWeaponsProfile(body string, statusLookup map[string]statusLookupEntry) parsedProfile {
	return mergeParsedWeaponMounts(extractParsedWeaponMounts(body, statusLookup))
}

func mergeParsedProfiles(a, b parsedProfile) parsedProfile {
	if b.damage <= 0 && b.splashDamage <= 0 && b.interval <= 0 {
		return a
	}
	if a.damage <= 0 || b.damage > a.damage {
		a.damage = b.damage
	}
	if b.splashDamage > a.splashDamage {
		a.splashDamage = b.splashDamage
	}
	if a.interval <= 0 || (b.interval > 0 && b.interval < a.interval) {
		a.interval = b.interval
	}
	if b.rangeV > a.rangeV {
		a.rangeV = b.rangeV
	}
	if b.bulletSpeed > a.bulletSpeed {
		a.bulletSpeed = b.bulletSpeed
	}
	if b.bulletLifetime > a.bulletLifetime {
		a.bulletLifetime = b.bulletLifetime
	}
	if b.bulletHitSize > a.bulletHitSize {
		a.bulletHitSize = b.bulletHitSize
	}
	if b.splashRadius > a.splashRadius {
		a.splashRadius = b.splashRadius
	}
	if b.buildingDamageMultiplier != 1 {
		a.buildingDamageMultiplier = b.buildingDamageMultiplier
	}
	if b.pierce > a.pierce {
		a.pierce = b.pierce
	}
	a.pierceBuilding = a.pierceBuilding || b.pierceBuilding
	if b.statusID != 0 || b.statusName != "" {
		a.statusID = b.statusID
		a.statusName = b.statusName
		a.statusDuration = b.statusDuration
	}
	if b.fragBullets > a.fragBullets {
		a.fragBullets = b.fragBullets
		a.fragSpread = b.fragSpread
		a.fragRandomSpread = b.fragRandomSpread
		a.fragAngle = b.fragAngle
		a.fragVelocityMin = b.fragVelocityMin
		a.fragVelocityMax = b.fragVelocityMax
		a.fragLifeMin = b.fragLifeMin
		a.fragLifeMax = b.fragLifeMax
		a.fragBullet = cloneBulletProfile(b.fragBullet)
	}
	if b.bullet != nil {
		a.bullet = cloneBulletProfile(b.bullet)
	}
	if b.shootStatusID != 0 || b.shootStatusName != "" {
		a.shootStatusID = b.shootStatusID
		a.shootStatusName = b.shootStatusName
		a.shootStatusDuration = b.shootStatusDuration
	}
	a.hitBuildings = a.hitBuildings || b.hitBuildings
	a.preferBuildings = a.preferBuildings || b.preferBuildings
	if b.hitRadius > a.hitRadius {
		a.hitRadius = b.hitRadius
	}
	a.targetAir = a.targetAir || b.targetAir
	a.targetGround = a.targetGround || b.targetGround
	if b.fireMode == "beam" {
		a.fireMode = "beam"
	}
	if strings.TrimSpace(b.shootEffect) != "" {
		a.shootEffect = b.shootEffect
	}
	if strings.TrimSpace(b.smokeEffect) != "" {
		a.smokeEffect = b.smokeEffect
	}
	if strings.TrimSpace(b.hitEffect) != "" {
		a.hitEffect = b.hitEffect
	}
	if strings.TrimSpace(b.despawnEffect) != "" {
		a.despawnEffect = b.despawnEffect
	}
	return a
}

func extractTurrets(src string, statusLookup map[string]statusLookupEntry) []TurretProfile {
	matches := reTurretDecl.FindAllStringSubmatchIndex(src, -1)
	out := make([]TurretProfile, 0, len(matches))
	for _, m := range matches {
		className := src[m[4]:m[5]]
		if !strings.Contains(className, "Turret") {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(src[m[6]:m[7]]))
		bodyStart := m[1]
		body, ok := extractInitBody(src, bodyStart)
		if !ok {
			continue
		}
		p := parseCommonProfile(body, statusLookup)
		flat := stripNestedInitBodies(body)
		continuousHold := isContinuousTurretClass(className)
		if (p.damage <= 0 && p.splashDamage <= 0) || p.rangeV <= 0 || (!continuousHold && p.interval <= 0) {
			continue
		}
		aimChangeSpeed := float32(0)
		if v, ok := lastValue(flat, reAimChangeSpeed); ok && v >= 0 {
			aimChangeSpeed = v
		}
		shootDuration := float32(0)
		if v, ok := lastValue(flat, reShootDuration); ok && v > 0 {
			shootDuration = v / 60
		}
		out = append(out, TurretProfile{
			ClassName:                className,
			Name:                     name,
			FireMode:                 p.fireMode,
			Range:                    p.rangeV,
			Damage:                   p.damage,
			SplashDamage:             p.splashDamage,
			Interval:                 p.interval,
			BulletType:               p.bulletType,
			BulletSpeed:              p.bulletSpeed,
			BulletLifetime:           p.bulletLifetime,
			BulletHitSize:            p.bulletHitSize,
			SplashRadius:             p.splashRadius,
			BuildingDamageMultiplier: p.buildingDamageMultiplier,
			Pierce:                   p.pierce,
			PierceBuilding:           p.pierceBuilding,
			StatusID:                 p.statusID,
			StatusName:               p.statusName,
			StatusDuration:           p.statusDuration,
			FragBullets:              p.fragBullets,
			FragSpread:               p.fragSpread,
			FragRandomSpread:         p.fragRandomSpread,
			FragAngle:                p.fragAngle,
			FragVelocityMin:          p.fragVelocityMin,
			FragVelocityMax:          p.fragVelocityMax,
			FragLifeMin:              p.fragLifeMin,
			FragLifeMax:              p.fragLifeMax,
			FragBullet:               cloneBulletProfile(p.fragBullet),
			TargetAir:                p.targetAir,
			TargetGround:             p.targetGround,
			TargetPriority:           "nearest",
			HitBuildings:             p.hitBuildings,
			ShootEffect:              p.shootEffect,
			SmokeEffect:              p.smokeEffect,
			HitEffect:                p.hitEffect,
			DespawnEffect:            p.despawnEffect,
			ContinuousHold:           continuousHold,
			AimChangeSpeed:           aimChangeSpeed,
			ShootDuration:            shootDuration,
			Bullet:                   cloneBulletProfile(p.bullet),
		})
	}
	return out
}

func isContinuousTurretClass(className string) bool {
	className = strings.TrimSpace(className)
	return strings.HasSuffix(className, "LaserTurret") ||
		strings.HasSuffix(className, "ContinuousTurret") ||
		strings.HasSuffix(className, "ContinuousLiquidTurret")
}

type parsedProfile struct {
	fireMode                 string
	rangeV                   float32
	damage                   float32
	splashDamage             float32
	interval                 float32
	bulletType               int16
	bulletSpeed              float32
	bulletLifetime           float32
	bulletHitSize            float32
	splashRadius             float32
	buildingDamageMultiplier float32
	pierce                   int32
	pierceBuilding           bool
	statusID                 int16
	statusName               string
	statusDuration           float32
	fragBullets              int32
	fragSpread               float32
	fragRandomSpread         float32
	fragAngle                float32
	fragVelocityMin          float32
	fragVelocityMax          float32
	fragLifeMin              float32
	fragLifeMax              float32
	fragBullet               *BulletProfile
	bullet                   *BulletProfile
	targetAir                bool
	targetGround             bool
	hitBuildings             bool
	preferBuildings          bool
	hitRadius                float32
	shootStatusID            int16
	shootStatusName          string
	shootStatusDuration      float32
	shootEffect              string
	smokeEffect              string
	hitEffect                string
	despawnEffect            string
}

func parseCommonProfile(body string, statusLookup map[string]statusLookupEntry) parsedProfile {
	p := parsedProfile{
		fireMode:                 "projectile",
		rangeV:                   0,
		damage:                   0,
		splashDamage:             0,
		interval:                 0,
		bulletSpeed:              0,
		bulletLifetime:           0,
		bulletHitSize:            0,
		splashRadius:             0,
		buildingDamageMultiplier: 1,
		pierce:                   0,
		targetAir:                true,
		targetGround:             true,
		hitBuildings:             true,
	}
	if strings.Contains(body, "LaserBulletType") || strings.Contains(body, "ContinuousLaserBulletType") {
		p.fireMode = "beam"
	}
	if v, ok := lastValue(body, reRange); ok {
		p.rangeV = v
	}
	if v, ok := lastValue(body, reMaxRange); ok && v > p.rangeV {
		p.rangeV = v
	}
	if v, ok := lastValue(body, reReload); ok && v > 0 {
		p.interval = v / 60
	}
	if v, ok := lastValue(body, reHitSize); ok && v > 0 {
		p.hitRadius = v
	}
	if v, ok := maxValue(body, reDamage); ok {
		p.damage = v
	}
	if spd, dmg, ok := maxCtorSpeedDamage(body); ok {
		if spd > p.bulletSpeed {
			p.bulletSpeed = spd
		}
		if dmg > p.damage {
			p.damage = dmg
		}
	}
	if v, ok := maxValue(body, reSplashDamage); ok && v > p.damage {
		p.damage = v
	}
	if v, ok := maxValue(body, reSplashDamage); ok {
		p.splashDamage = v
	}
	if v, ok := firstValue(body, reSpeed); ok {
		p.bulletSpeed = v
	}
	if v, ok := maxValue(body, reSplashRadius); ok {
		p.splashRadius = v
	}
	if strings.Contains(body, "pierce = true") {
		p.pierce = 1
	}
	if m := rePierceBuilding.FindAllStringSubmatch(body, -1); len(m) > 0 {
		p.pierceBuilding = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if v, ok := maxValue(body, rePierceCap); ok && v > 0 {
		p.pierce = int32(v)
	}
	if v, ok := lastValue(body, reBuildingDamageMultiplier); ok {
		p.buildingDamageMultiplier = v
	}
	if m := reTargetAir.FindAllStringSubmatch(body, -1); len(m) > 0 {
		p.targetAir = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if m := reTargetGround.FindAllStringSubmatch(body, -1); len(m) > 0 {
		p.targetGround = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if m := reTargetBlocks.FindAllStringSubmatch(body, -1); len(m) > 0 {
		p.hitBuildings = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if statusID, statusName, ok := parseStatusAssignment(body, statusLookup); ok {
		p.statusID = statusID
		p.statusName = statusName
	}
	if v, ok := lastValue(body, reStatusDuration); ok && v > 0 {
		p.statusDuration = v / 60
	}
	if statusID, statusName, ok := parseShootStatusAssignment(body, statusLookup); ok {
		p.shootStatusID = statusID
		p.shootStatusName = statusName
	}
	if v, ok := lastValue(body, reShootStatusDuration); ok && v > 0 {
		p.shootStatusDuration = v / 60
	}
	if v, ok := lastValue(body, reFragBullets); ok && v > 0 {
		p.fragBullets = int32(v)
	}
	if v, ok := lastValue(body, reFragSpread); ok {
		p.fragSpread = v
	}
	if v, ok := lastValue(body, reFragRandomSpread); ok {
		p.fragRandomSpread = v
	}
	if v, ok := lastValue(body, reFragAngle); ok {
		p.fragAngle = v
	}
	if v, ok := lastValue(body, reFragVelocityMin); ok && v > 0 {
		p.fragVelocityMin = v
	}
	if v, ok := lastValue(body, reFragVelocityMax); ok && v > 0 {
		p.fragVelocityMax = v
	}
	if v, ok := lastValue(body, reFragLifeMin); ok && v > 0 {
		p.fragLifeMin = v
	}
	if v, ok := lastValue(body, reFragLifeMax); ok && v > 0 {
		p.fragLifeMax = v
	}
	if bullet, ok := parseAssignedBulletProfile(body, "bullet", statusLookup); ok {
		p.bullet = cloneBulletProfile(&bullet)
		applyBulletProfile(&p, bullet)
	}
	if bullet, ok := parseAssignedBulletProfile(body, "shootType", statusLookup); ok {
		p.bullet = cloneBulletProfile(&bullet)
		applyBulletProfile(&p, bullet)
	}
	if frag, ok := parseAssignedBulletProfile(body, "fragBullet", statusLookup); ok {
		p.fragBullet = cloneBulletProfile(&frag)
		if p.fragBullets <= 0 {
			p.fragBullets = 1
		}
	}
	parseEffectNames(body, &p)
	return p
}

func parseEffectNames(body string, p *parsedProfile) {
	if p == nil || strings.TrimSpace(body) == "" {
		return
	}
	for _, m := range reEffectChain.FindAllStringSubmatch(body, -1) {
		if len(m) < 4 {
			continue
		}
		assignEffectName(p, m[1], m[3])
		assignEffectName(p, m[2], m[3])
	}
	for _, m := range reEffectAssign.FindAllStringSubmatch(body, -1) {
		if len(m) < 3 {
			continue
		}
		assignEffectName(p, m[1], m[2])
	}
}

func assignEffectName(p *parsedProfile, key, name string) {
	if p == nil {
		return
	}
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "none" {
		return
	}
	switch strings.TrimSpace(key) {
	case "shootEffect":
		p.shootEffect = name
	case "smokeEffect":
		p.smokeEffect = name
	case "hitEffect":
		p.hitEffect = name
	case "despawnEffect":
		p.despawnEffect = name
	}
}

func maxCtorSpeedDamage(body string) (float32, float32, bool) {
	ms := reBulletCtor.FindAllStringSubmatch(body, -1)
	bestSpeed := float32(0)
	bestDamage := float32(0)
	found := false
	for _, m := range ms {
		if len(m) < 2 {
			continue
		}
		args := splitArgs(m[1])
		if len(args) == 0 {
			continue
		}
		vals := make([]float32, 0, len(args))
		for _, a := range args {
			if v, ok := evalNumericExpr(a); ok {
				vals = append(vals, float32(v))
			} else {
				break
			}
		}
		if len(vals) == 0 {
			continue
		}
		found = true
		if len(vals) >= 2 {
			if vals[0] > bestSpeed {
				bestSpeed = vals[0]
			}
			if vals[1] > bestDamage {
				bestDamage = vals[1]
			}
		} else {
			if vals[0] > bestDamage {
				bestDamage = vals[0]
			}
		}
	}
	return bestSpeed, bestDamage, found
}

func splitArgs(s string) []string {
	out := make([]string, 0, 4)
	start := 0
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				out = append(out, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if start < len(s) {
		out = append(out, strings.TrimSpace(s[start:]))
	}
	return out
}

func firstValue(body string, re *regexp.Regexp) (float32, bool) {
	ms := re.FindAllStringSubmatch(body, -1)
	for _, m := range ms {
		if len(m) < 2 {
			continue
		}
		if v, ok := evalNumericExpr(m[1]); ok {
			return float32(v), true
		}
	}
	return 0, false
}

func lastValue(body string, re *regexp.Regexp) (float32, bool) {
	ms := re.FindAllStringSubmatch(body, -1)
	for i := len(ms) - 1; i >= 0; i-- {
		if len(ms[i]) < 2 {
			continue
		}
		if v, ok := evalNumericExpr(ms[i][1]); ok {
			return float32(v), true
		}
	}
	return 0, false
}

func maxValue(body string, re *regexp.Regexp) (float32, bool) {
	ms := re.FindAllStringSubmatch(body, -1)
	best := float32(0)
	okAny := false
	for _, m := range ms {
		if len(m) < 2 {
			continue
		}
		if v, ok := evalNumericExpr(m[1]); ok {
			f := float32(v)
			if !okAny || f > best {
				best = f
				okAny = true
			}
		}
	}
	return best, okAny
}

func extractInitBody(src string, bodyStart int) (string, bool) {
	depth := 2
	start := bodyStart
	for i := bodyStart; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return src[start:i], true
			}
		}
	}
	return "", false
}

func evalNumericExpr(expr string) (float64, bool) {
	s := strings.TrimSpace(expr)
	if s == "" {
		return 0, false
	}
	s = strings.ReplaceAll(s, " ", "")
	s = strings.ReplaceAll(s, "f", "")
	s = strings.ReplaceAll(s, "F", "")
	s = strings.ReplaceAll(s, "d", "")
	s = strings.ReplaceAll(s, "D", "")
	if strings.ContainsAny(s, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ_") {
		return 0, false
	}
	p := &numParser{s: s}
	v, ok := p.parseExpr()
	if !ok || p.pos != len(p.s) {
		return 0, false
	}
	return v, true
}

type numParser struct {
	s   string
	pos int
}

func (p *numParser) parseExpr() (float64, bool) {
	v, ok := p.parseTerm()
	if !ok {
		return 0, false
	}
	for p.pos < len(p.s) {
		op := p.s[p.pos]
		if op != '+' && op != '-' {
			break
		}
		p.pos++
		rhs, ok := p.parseTerm()
		if !ok {
			return 0, false
		}
		if op == '+' {
			v += rhs
		} else {
			v -= rhs
		}
	}
	return v, true
}

func (p *numParser) parseTerm() (float64, bool) {
	v, ok := p.parseFactor()
	if !ok {
		return 0, false
	}
	for p.pos < len(p.s) {
		op := p.s[p.pos]
		if op != '*' && op != '/' {
			break
		}
		p.pos++
		rhs, ok := p.parseFactor()
		if !ok {
			return 0, false
		}
		if op == '*' {
			v *= rhs
		} else {
			if rhs == 0 {
				return 0, false
			}
			v /= rhs
		}
	}
	return v, true
}

func (p *numParser) parseFactor() (float64, bool) {
	if p.pos >= len(p.s) {
		return 0, false
	}
	if p.s[p.pos] == '+' {
		p.pos++
		return p.parseFactor()
	}
	if p.s[p.pos] == '-' {
		p.pos++
		v, ok := p.parseFactor()
		return -v, ok
	}
	if p.s[p.pos] == '(' {
		p.pos++
		v, ok := p.parseExpr()
		if !ok || p.pos >= len(p.s) || p.s[p.pos] != ')' {
			return 0, false
		}
		p.pos++
		return v, true
	}
	start := p.pos
	for p.pos < len(p.s) {
		c := p.s[p.pos]
		if (c >= '0' && c <= '9') || c == '.' {
			p.pos++
			continue
		}
		break
	}
	if start == p.pos {
		return 0, false
	}
	v, err := strconv.ParseFloat(p.s[start:p.pos], 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

func ExampleUsage() string {
	return fmt.Sprintf("vanilla gen [repo-root] [out-path], default out: %s", filepath.FromSlash("data/vanilla/profiles.json"))
}
