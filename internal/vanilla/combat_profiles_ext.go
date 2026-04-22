package vanilla

import (
	"regexp"
	"strings"
)

type statusLookupEntry struct {
	ID   int16
	Name string
}

var (
	reHitSize                  = regexp.MustCompile(`(?m)\bhitSize\s*=\s*([^;]+);`)
	reLifetime                 = regexp.MustCompile(`(?m)\blifetime\s*=\s*([^;]+);`)
	reBuildingDamageMultiplier = regexp.MustCompile(`(?m)\bbuildingDamageMultiplier\s*=\s*([^;]+);`)
	reArmorMultiplier          = regexp.MustCompile(`(?m)\barmorMultiplier\s*=\s*([^;]+);`)
	reMaxDamageFraction        = regexp.MustCompile(`(?m)\bmaxDamageFraction\s*=\s*([^;]+);`)
	reShieldDamageMultiplier   = regexp.MustCompile(`(?m)\bshieldDamageMultiplier\s*=\s*([^;]+);`)
	rePierceDamageFactor       = regexp.MustCompile(`(?m)\bpierceDamageFactor\s*=\s*([^;]+);`)
	rePierceArmor              = regexp.MustCompile(`(?m)\bpierceArmor\s*=\s*(true|false)\s*;`)
	rePierceBuilding           = regexp.MustCompile(`(?m)\bpierceBuilding\s*=\s*(true|false)\s*;`)
	reTargetBlocks             = regexp.MustCompile(`(?m)\btargetBlocks\s*=\s*(true|false)\s*;`)
	reStatusDuration           = regexp.MustCompile(`(?m)\bstatusDuration\s*=\s*([^;]+);`)
	reShootStatusDuration      = regexp.MustCompile(`(?m)\bshootStatusDuration\s*=\s*([^;]+);`)
	reFragBullets              = regexp.MustCompile(`(?m)\bfragBullets\s*=\s*([^;]+);`)
	reFragSpread               = regexp.MustCompile(`(?m)\bfragSpread\s*=\s*([^;]+);`)
	reFragRandomSpread         = regexp.MustCompile(`(?m)\bfragRandomSpread\s*=\s*([^;]+);`)
	reFragAngle                = regexp.MustCompile(`(?m)\bfragAngle\s*=\s*([^;]+);`)
	reFragVelocityMin          = regexp.MustCompile(`(?m)\bfragVelocityMin\s*=\s*([^;]+);`)
	reFragVelocityMax          = regexp.MustCompile(`(?m)\bfragVelocityMax\s*=\s*([^;]+);`)
	reFragLifeMin              = regexp.MustCompile(`(?m)\bfragLifeMin\s*=\s*([^;]+);`)
	reFragLifeMax              = regexp.MustCompile(`(?m)\bfragLifeMax\s*=\s*([^;]+);`)
	reStatusAssign             = regexp.MustCompile(`(?m)\bstatus\s*=\s*(?:StatusEffects\.)?([A-Za-z0-9_-]+)\s*;`)
	reShootStatusAssign        = regexp.MustCompile(`(?m)\bshootStatus\s*=\s*(?:StatusEffects\.)?([A-Za-z0-9_-]+)\s*;`)
	reLocalBulletDecl          = regexp.MustCompile(`(?m)\b(?:[A-Za-z0-9_$.]+BulletType|BulletType)\s+(\w+)\s*=\s*new\s+([A-Za-z0-9_$.]+)\s*\(([^)]*)\)\s*\{\{`)
	reStatusDecl               = regexp.MustCompile(`(?m)(\w+)\s*=\s*new\s+StatusEffect\s*\(\s*"([^"]+)"\s*\)(?:\s*\{\{)?`)
	reOpposite                 = regexp.MustCompile(`(?m)\bopposite\s*\(([^)]*)\)`)
	reAffinity                 = regexp.MustCompile(`(?m)\baffinity\s*\(\s*([A-Za-z0-9_]+)\s*,`)
	reDamageMultiplierStatus   = regexp.MustCompile(`(?m)\bdamageMultiplier\s*=\s*([^;]+);`)
	reHealthMultiplierStatus   = regexp.MustCompile(`(?m)\bhealthMultiplier\s*=\s*([^;]+);`)
	reSpeedMultiplierStatus    = regexp.MustCompile(`(?m)\bspeedMultiplier\s*=\s*([^;]+);`)
	reReloadMultiplierStatus   = regexp.MustCompile(`(?m)\breloadMultiplier\s*=\s*([^;]+);`)
	reBuildSpeedMultiplier     = regexp.MustCompile(`(?m)\bbuildSpeedMultiplier\s*=\s*([^;]+);`)
	reDragMultiplierStatus     = regexp.MustCompile(`(?m)\bdragMultiplier\s*=\s*([^;]+);`)
	reTransitionDamageStatus   = regexp.MustCompile(`(?m)\btransitionDamage\s*=\s*([^;]+);`)
	reIntervalDamageTime       = regexp.MustCompile(`(?m)\bintervalDamageTime\s*=\s*([^;]+);`)
	reIntervalDamage           = regexp.MustCompile(`(?m)\bintervalDamage\s*=\s*([^;]+);`)
	reIntervalDamagePierce     = regexp.MustCompile(`(?m)\bintervalDamagePierce\s*=\s*(true|false)\s*;`)
	reDisarmStatus             = regexp.MustCompile(`(?m)\bdisarm\s*=\s*(true|false)\s*;`)
	rePermanentStatus          = regexp.MustCompile(`(?m)\bpermanent\s*=\s*(true|false)\s*;`)
	reReactiveStatus           = regexp.MustCompile(`(?m)\breactive\s*=\s*(true|false)\s*;`)
	reDynamicStatus            = regexp.MustCompile(`(?m)\bdynamic\s*=\s*(true|false)\s*;`)
)

func cloneBulletProfile(src *BulletProfile) *BulletProfile {
	if src == nil {
		return nil
	}
	copy := *src
	copy.FragBullet = cloneBulletProfile(src.FragBullet)
	return &copy
}

func applyBulletProfile(dst *parsedProfile, bullet BulletProfile) {
	if dst == nil {
		return
	}
	if bullet.ClassName != "" {
		cls := strings.ToLower(strings.TrimSpace(bullet.ClassName))
		if strings.Contains(cls, "laser") || strings.Contains(cls, "beam") {
			dst.fireMode = "beam"
		}
	}
	if bullet.Damage > dst.damage {
		dst.damage = bullet.Damage
	}
	if bullet.SplashDamage > dst.splashDamage {
		dst.splashDamage = bullet.SplashDamage
	}
	if bullet.BulletType != 0 {
		dst.bulletType = bullet.BulletType
	}
	if bullet.Speed > dst.bulletSpeed {
		dst.bulletSpeed = bullet.Speed
	}
	if bullet.Lifetime > dst.bulletLifetime {
		dst.bulletLifetime = bullet.Lifetime
	}
	if bullet.Length > dst.rangeV {
		dst.rangeV = bullet.Length
	}
	if bullet.HitSize > dst.bulletHitSize {
		dst.bulletHitSize = bullet.HitSize
	}
	if bullet.SplashRadius > dst.splashRadius {
		dst.splashRadius = bullet.SplashRadius
	}
	if bullet.BuildingDamageMultiplier != 1 {
		dst.buildingDamageMultiplier = bullet.BuildingDamageMultiplier
	}
	if bullet.ArmorMultiplier > 0 {
		dst.armorMultiplier = bullet.ArmorMultiplier
	}
	if bullet.MaxDamageFraction > 0 {
		dst.maxDamageFraction = bullet.MaxDamageFraction
	}
	if bullet.ShieldDamageMultiplier > 0 {
		dst.shieldDamageMultiplier = bullet.ShieldDamageMultiplier
	}
	if bullet.PierceDamageFactor > 0 {
		dst.pierceDamageFactor = bullet.PierceDamageFactor
	}
	dst.pierceArmor = dst.pierceArmor || bullet.PierceArmor
	if bullet.Pierce > dst.pierce {
		dst.pierce = bullet.Pierce
	}
	dst.pierceBuilding = dst.pierceBuilding || bullet.PierceBuilding
	if bullet.StatusID != 0 || bullet.StatusName != "" {
		dst.statusID = bullet.StatusID
		dst.statusName = bullet.StatusName
		dst.statusDuration = bullet.StatusDuration
	}
	dst.targetAir = dst.targetAir && bullet.TargetAir
	dst.targetGround = dst.targetGround && bullet.TargetGround
	dst.hitBuildings = dst.hitBuildings && bullet.HitBuildings
	if bullet.FragBullets > dst.fragBullets {
		dst.fragBullets = bullet.FragBullets
		dst.fragSpread = bullet.FragSpread
		dst.fragRandomSpread = bullet.FragRandomSpread
		dst.fragAngle = bullet.FragAngle
		dst.fragVelocityMin = bullet.FragVelocityMin
		dst.fragVelocityMax = bullet.FragVelocityMax
		dst.fragLifeMin = bullet.FragLifeMin
		dst.fragLifeMax = bullet.FragLifeMax
		dst.fragBullet = cloneBulletProfile(bullet.FragBullet)
	}
	dst.bullet = cloneBulletProfile(&bullet)
}

func parseAssignedBulletProfile(body, key string, statusLookup map[string]statusLookupEntry) (BulletProfile, bool) {
	locals := extractNamedBulletProfiles(body, statusLookup)
	refRE := regexp.MustCompile(`(?m)\b` + regexp.QuoteMeta(key) + `\s*=\s*([A-Za-z_]\w*)\s*;`)
	if m := refRE.FindStringSubmatch(body); len(m) == 2 {
		if local, ok := locals[normalizeStatusLookupKey(m[1])]; ok {
			return local, true
		}
	}
	inlineRE := regexp.MustCompile(`(?m)\b` + regexp.QuoteMeta(key) + `\s*=\s*new\s+([A-Za-z0-9_$.]+)\s*\(([^)]*)\)\s*\{\{`)
	if m := inlineRE.FindStringSubmatchIndex(body); len(m) >= 6 {
		className := body[m[2]:m[3]]
		args := body[m[4]:m[5]]
		inlineBody, ok := extractInitBody(body, m[1])
		if !ok {
			return BulletProfile{}, false
		}
		return parseInlineBulletProfile(className, args, inlineBody, statusLookup), true
	}
	return BulletProfile{}, false
}

func extractNamedBulletProfiles(body string, statusLookup map[string]statusLookupEntry) map[string]BulletProfile {
	matches := reLocalBulletDecl.FindAllStringSubmatchIndex(body, -1)
	if len(matches) == 0 {
		return nil
	}
	out := make(map[string]BulletProfile, len(matches))
	for _, m := range matches {
		if len(m) < 8 {
			continue
		}
		name := normalizeStatusLookupKey(body[m[2]:m[3]])
		className := body[m[4]:m[5]]
		args := body[m[6]:m[7]]
		inlineBody, ok := extractInitBody(body, m[1])
		if !ok || name == "" {
			continue
		}
		out[name] = parseInlineBulletProfile(className, args, inlineBody, statusLookup)
	}
	return out
}

func parseInlineBulletProfile(className, args, body string, statusLookup map[string]statusLookupEntry) BulletProfile {
	out := BulletProfile{
		ClassName:                strings.TrimSpace(className),
		BuildingDamageMultiplier: 1,
		ArmorMultiplier:          1,
		ShieldDamageMultiplier:   1,
		HitBuildings:             true,
		TargetAir:                true,
		TargetGround:             true,
		FragVelocityMin:          0.2,
		FragVelocityMax:          1,
		FragLifeMin:              1,
		FragLifeMax:              1,
	}
	switch {
	case isBulletClass(className, "ContinuousLaserBulletType"):
		out.Lifetime = 16.0 / 60.0
		out.DamageInterval = 5.0 / 60.0
		out.FadeTime = 16.0 / 60.0
	case isBulletClass(className, "ContinuousBulletType"):
		out.Lifetime = 16.0 / 60.0
		out.DamageInterval = 5.0 / 60.0
	case isBulletClass(className, "PointLaserBulletType"):
		out.Lifetime = 20.0 / 60.0
		out.DamageInterval = 5.0 / 60.0
		out.OptimalLifeFract = 0.5
	}
	values := splitArgs(args)
	if constructorArgIsDamage(className) {
		if len(values) >= 1 {
			if v, ok := evalNumericExpr(values[0]); ok {
				out.Damage = float32(v)
			}
		}
	} else {
		if len(values) >= 1 {
			if v, ok := evalNumericExpr(values[0]); ok {
				out.Speed = float32(v)
			}
		}
		if len(values) >= 2 {
			if v, ok := evalNumericExpr(values[1]); ok {
				out.Damage = float32(v)
			}
		}
	}
	if v, ok := lastValue(body, reSpeed); ok && v > 0 {
		out.Speed = v
	}
	if v, ok := lastValue(body, reDamage); ok && v > 0 {
		out.Damage = v
	}
	if v, ok := lastValue(body, reSplashDamage); ok && v > 0 {
		out.SplashDamage = v
	}
	if v, ok := lastValue(body, reSplashRadius); ok && v > 0 {
		out.SplashRadius = v
	}
	if v, ok := lastValue(body, reLifetime); ok && v > 0 {
		out.Lifetime = v / 60
	}
	if v, ok := lastValue(body, reLength); ok && v > 0 {
		out.Length = v
	}
	if v, ok := lastValue(body, reHitSize); ok && v > 0 {
		out.HitSize = v
	}
	if v, ok := lastValue(body, reDamageInterval); ok && v > 0 {
		out.DamageInterval = v / 60
	}
	if v, ok := lastValue(body, reOptimalLifeFract); ok && v >= 0 {
		out.OptimalLifeFract = v
	}
	if v, ok := lastValue(body, reFadeTime); ok && v > 0 {
		out.FadeTime = v / 60
	}
	if v, ok := lastValue(body, reBuildingDamageMultiplier); ok {
		out.BuildingDamageMultiplier = v
	}
	if v, ok := lastValue(body, reArmorMultiplier); ok && v > 0 {
		out.ArmorMultiplier = v
	}
	if v, ok := lastValue(body, reMaxDamageFraction); ok && v > 0 {
		out.MaxDamageFraction = v
	}
	if v, ok := lastValue(body, reShieldDamageMultiplier); ok && v > 0 {
		out.ShieldDamageMultiplier = v
	}
	if v, ok := lastValue(body, rePierceDamageFactor); ok && v > 0 {
		out.PierceDamageFactor = v
	}
	if m := rePierceArmor.FindAllStringSubmatch(body, -1); len(m) > 0 {
		out.PierceArmor = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if strings.Contains(body, "pierce = true") {
		out.Pierce = 1
	}
	if v, ok := lastValue(body, rePierceCap); ok && v > 0 {
		out.Pierce = int32(v)
	}
	if m := rePierceBuilding.FindAllStringSubmatch(body, -1); len(m) > 0 {
		out.PierceBuilding = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if m := reTargetAir.FindAllStringSubmatch(body, -1); len(m) > 0 {
		out.TargetAir = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if m := reTargetGround.FindAllStringSubmatch(body, -1); len(m) > 0 {
		out.TargetGround = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if m := reTargetBlocks.FindAllStringSubmatch(body, -1); len(m) > 0 {
		out.HitBuildings = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if statusID, statusName, ok := parseStatusAssignment(body, statusLookup); ok {
		out.StatusID = statusID
		out.StatusName = statusName
	}
	if v, ok := lastValue(body, reStatusDuration); ok && v > 0 {
		out.StatusDuration = v / 60
	}
	if v, ok := lastValue(body, reFragBullets); ok && v > 0 {
		out.FragBullets = int32(v)
	}
	if v, ok := lastValue(body, reFragSpread); ok {
		out.FragSpread = v
	}
	if v, ok := lastValue(body, reFragRandomSpread); ok {
		out.FragRandomSpread = v
	}
	if v, ok := lastValue(body, reFragAngle); ok {
		out.FragAngle = v
	}
	if v, ok := lastValue(body, reFragVelocityMin); ok && v > 0 {
		out.FragVelocityMin = v
	}
	if v, ok := lastValue(body, reFragVelocityMax); ok && v > 0 {
		out.FragVelocityMax = v
	}
	if v, ok := lastValue(body, reFragLifeMin); ok && v > 0 {
		out.FragLifeMin = v
	}
	if v, ok := lastValue(body, reFragLifeMax); ok && v > 0 {
		out.FragLifeMax = v
	}
	if frag, ok := parseAssignedBulletProfile(body, "fragBullet", statusLookup); ok {
		out.FragBullet = cloneBulletProfile(&frag)
		if out.FragBullets <= 0 {
			out.FragBullets = 1
		}
	}
	var effects parsedProfile
	parseEffectNames(body, &effects)
	out.ShootEffect = effects.shootEffect
	out.SmokeEffect = effects.smokeEffect
	out.HitEffect = effects.hitEffect
	out.DespawnEffect = effects.despawnEffect
	return out
}

func isBulletClass(className, suffix string) bool {
	className = strings.ToLower(strings.TrimSpace(className))
	suffix = strings.ToLower(strings.TrimSpace(suffix))
	return className == suffix || strings.HasSuffix(className, "."+suffix)
}

func constructorArgIsDamage(className string) bool {
	return isBulletClass(className, "LaserBulletType") ||
		isBulletClass(className, "ContinuousLaserBulletType")
}

func parseStatusAssignment(body string, statusLookup map[string]statusLookupEntry) (int16, string, bool) {
	return parseNamedStatusAssignment(body, reStatusAssign, statusLookup)
}

func parseShootStatusAssignment(body string, statusLookup map[string]statusLookupEntry) (int16, string, bool) {
	return parseNamedStatusAssignment(body, reShootStatusAssign, statusLookup)
}

func parseNamedStatusAssignment(body string, re *regexp.Regexp, statusLookup map[string]statusLookupEntry) (int16, string, bool) {
	if re == nil {
		return 0, "", false
	}
	matches := re.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return 0, "", false
	}
	raw := normalizeStatusLookupKey(matches[len(matches)-1][1])
	if raw == "" || raw == "none" {
		return 0, "", false
	}
	if statusLookup != nil {
		if entry, ok := statusLookup[raw]; ok {
			return entry.ID, entry.Name, true
		}
	}
	return 0, raw, true
}

func normalizeStatusLookupKey(v string) string {
	return strings.ToLower(strings.TrimSpace(v))
}

func extractStatuses(src string) ([]StatusProfile, map[string]statusLookupEntry) {
	matches := reStatusDecl.FindAllStringSubmatchIndex(src, -1)
	out := make([]StatusProfile, 0, len(matches))
	lookup := make(map[string]statusLookupEntry, len(matches)*2)
	for i, m := range matches {
		if len(m) < 6 {
			continue
		}
		varName := normalizeStatusLookupKey(src[m[2]:m[3]])
		name := normalizeStatusLookupKey(src[m[4]:m[5]])
		entry := statusLookupEntry{ID: int16(i), Name: name}
		if varName != "" {
			lookup[varName] = entry
		}
		if name != "" {
			lookup[name] = entry
		}
	}
	for i, m := range matches {
		if len(m) < 6 {
			continue
		}
		name := normalizeStatusLookupKey(src[m[4]:m[5]])
		body, _ := extractInitBody(src, m[1])
		profile := StatusProfile{
			ID:                   int16(i),
			Name:                 name,
			DamageMultiplier:     1,
			HealthMultiplier:     1,
			SpeedMultiplier:      1,
			ReloadMultiplier:     1,
			BuildSpeedMultiplier: 1,
			DragMultiplier:       1,
		}
		if v, ok := lastValue(body, reDamageMultiplierStatus); ok && v != 0 {
			profile.DamageMultiplier = v
		}
		if v, ok := lastValue(body, reHealthMultiplierStatus); ok && v != 0 {
			profile.HealthMultiplier = v
		}
		if v, ok := lastValue(body, reSpeedMultiplierStatus); ok && v != 0 {
			profile.SpeedMultiplier = v
		}
		if v, ok := lastValue(body, reReloadMultiplierStatus); ok && v != 0 {
			profile.ReloadMultiplier = v
		}
		if v, ok := lastValue(body, reBuildSpeedMultiplier); ok && v != 0 {
			profile.BuildSpeedMultiplier = v
		}
		if v, ok := lastValue(body, reDragMultiplierStatus); ok && v != 0 {
			profile.DragMultiplier = v
		}
		if v, ok := lastValue(body, reTransitionDamageStatus); ok && v > 0 {
			profile.TransitionDamage = v
		}
		if v, ok := lastValue(body, reDamage); ok && v != 0 {
			profile.Damage = v * 60
		}
		if v, ok := lastValue(body, reIntervalDamageTime); ok && v > 0 {
			profile.IntervalDamageTime = v / 60
		}
		if v, ok := lastValue(body, reIntervalDamage); ok && v != 0 {
			profile.IntervalDamage = v
		}
		if m := reIntervalDamagePierce.FindAllStringSubmatch(body, -1); len(m) > 0 {
			profile.IntervalDamagePierce = strings.EqualFold(m[len(m)-1][1], "true")
		}
		if m := reDisarmStatus.FindAllStringSubmatch(body, -1); len(m) > 0 {
			profile.Disarm = strings.EqualFold(m[len(m)-1][1], "true")
		}
		if m := rePermanentStatus.FindAllStringSubmatch(body, -1); len(m) > 0 {
			profile.Permanent = strings.EqualFold(m[len(m)-1][1], "true")
		}
		if m := reReactiveStatus.FindAllStringSubmatch(body, -1); len(m) > 0 {
			profile.Reactive = strings.EqualFold(m[len(m)-1][1], "true")
		}
		if m := reDynamicStatus.FindAllStringSubmatch(body, -1); len(m) > 0 {
			profile.Dynamic = strings.EqualFold(m[len(m)-1][1], "true")
		}
		for _, opp := range reOpposite.FindAllStringSubmatch(body, -1) {
			if len(opp) < 2 {
				continue
			}
			for _, raw := range splitArgs(opp[1]) {
				key := normalizeStatusLookupKey(raw)
				if key == "" {
					continue
				}
				if entry, ok := lookup[key]; ok && entry.Name != "" {
					profile.Opposites = append(profile.Opposites, entry.Name)
				}
			}
		}
		for _, aff := range reAffinity.FindAllStringSubmatch(body, -1) {
			if len(aff) < 2 {
				continue
			}
			key := normalizeStatusLookupKey(aff[1])
			if entry, ok := lookup[key]; ok && entry.Name != "" {
				profile.Affinities = append(profile.Affinities, entry.Name)
			}
		}
		out = append(out, profile)
	}
	return out, lookup
}
