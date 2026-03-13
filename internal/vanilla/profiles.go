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
	Name            string  `json:"name"`
	FireMode        string  `json:"fire_mode"`
	Range           float32 `json:"range"`
	Damage          float32 `json:"damage"`
	Interval        float32 `json:"interval"`
	BulletType      int16   `json:"bullet_type"`
	BulletSpeed     float32 `json:"bullet_speed"`
	SplashRadius    float32 `json:"splash_radius"`
	Pierce          int32   `json:"pierce"`
	BurstShots      int32   `json:"burst_shots"`
	BurstSpacing    float32 `json:"burst_spacing"`
	Spread          float32 `json:"spread"`
	TargetAir       bool    `json:"target_air"`
	TargetGround    bool    `json:"target_ground"`
	TargetPriority  string  `json:"target_priority"`
	HitBuildings    bool    `json:"hit_buildings"`
	PreferBuildings bool    `json:"prefer_buildings"`
}

type TurretProfile struct {
	Name           string  `json:"name"`
	FireMode       string  `json:"fire_mode"`
	Range          float32 `json:"range"`
	Damage         float32 `json:"damage"`
	Interval       float32 `json:"interval"`
	BulletType     int16   `json:"bullet_type"`
	BulletSpeed    float32 `json:"bullet_speed"`
	SplashRadius   float32 `json:"splash_radius"`
	Pierce         int32   `json:"pierce"`
	TargetAir      bool    `json:"target_air"`
	TargetGround   bool    `json:"target_ground"`
	TargetPriority string  `json:"target_priority"`
	HitBuildings   bool    `json:"hit_buildings"`
}

type ProfilesFile struct {
	UnitsByName []UnitProfile   `json:"units_by_name"`
	Turrets     []TurretProfile `json:"turrets"`
}

func GenerateProfiles(repoRoot, outPath string) (int, int, error) {
	unitPath, blocksPath := resolveSourcePaths(repoRoot)
	unitSrc, err := os.ReadFile(unitPath)
	if err != nil {
		return 0, 0, err
	}
	blockSrc, err := os.ReadFile(blocksPath)
	if err != nil {
		return 0, 0, err
	}

	units := extractUnits(string(unitSrc))
	turrets := extractTurrets(string(blockSrc))
	payload := ProfilesFile{
		UnitsByName: units,
		Turrets:     turrets,
	}

	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return 0, 0, err
	}
	b, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return 0, 0, err
	}
	if err := os.WriteFile(outPath, b, 0644); err != nil {
		return 0, 0, err
	}
	return len(units), len(turrets), nil
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
	reUnitDecl   = regexp.MustCompile(`(?m)(\w+)\s*=\s*new\s+UnitType\("([^"]+)"\)\s*\{\{`)
	reTurretDecl = regexp.MustCompile(`(?m)(\w+)\s*=\s*new\s+([A-Za-z0-9_$.]+)\("([^"]+)"\)\s*\{\{`)

	reRange        = regexp.MustCompile(`(?m)\brange\s*=\s*([^;]+);`)
	reMaxRange     = regexp.MustCompile(`(?m)\bmaxRange\s*=\s*([^;]+);`)
	reReload       = regexp.MustCompile(`(?m)\breload\s*=\s*([^;]+);`)
	reDamage       = regexp.MustCompile(`(?m)\bdamage\s*=\s*([^;]+);`)
	reSplashDamage = regexp.MustCompile(`(?m)\bsplashDamage\s*=\s*([^;]+);`)
	reSplashRadius = regexp.MustCompile(`(?m)\bsplashDamageRadius\s*=\s*([^;]+);`)
	reSpeed        = regexp.MustCompile(`(?m)\bspeed\s*=\s*([^;]+);`)
	reTargetAir    = regexp.MustCompile(`(?m)\btargetAir\s*=\s*(true|false)\s*;`)
	reTargetGround = regexp.MustCompile(`(?m)\btargetGround\s*=\s*(true|false)\s*;`)
	rePierceCap    = regexp.MustCompile(`(?m)\bpierceCap\s*=\s*([^;]+);`)
	reWeaponDecl   = regexp.MustCompile(`(?m)new\s+Weapon\([^)]*\)\s*\{\{`)
	reBulletCtor   = regexp.MustCompile(`(?m)new\s+[A-Za-z0-9_$.]*BulletType\s*\(([^)]*)\)`)
	reShootShots   = regexp.MustCompile(`(?m)\bshoot\s*\.\s*shots\s*=\s*([^;]+);`)
	reShootDelay   = regexp.MustCompile(`(?m)\bshoot\s*\.\s*shotDelay\s*=\s*([^;]+);`)
	reShootSpread  = regexp.MustCompile(`(?m)\bshoot\s*\.\s*spread\s*=\s*([^;]+);`)
	reShootCtor    = regexp.MustCompile(`(?m)new\s+ShootSpread\s*\(([^)]*)\)`)
)

func extractUnits(src string) []UnitProfile {
	matches := reUnitDecl.FindAllStringSubmatchIndex(src, -1)
	out := make([]UnitProfile, 0, len(matches))
	for _, m := range matches {
		name := src[m[4]:m[5]]
		bodyStart := m[1]
		body, ok := extractInitBody(src, bodyStart)
		if !ok {
			continue
		}
		p := parseCommonProfile(body)
		wp := parseWeaponsProfile(body)
		p = mergeParsedProfiles(p, wp)
		unitName := strings.ToLower(strings.TrimSpace(name))
		if unitName == "" {
			continue
		}

		// Repair beam units have non-damaging weapons; include them with beam profile.
		if (p.damage <= 0 || p.interval <= 0) && strings.Contains(body, "RepairBeamWeapon") {
			out = append(out, UnitProfile{
				Name:            unitName,
				FireMode:        "beam",
				Range:           p.rangeV,
				Damage:          0,
				Interval:        p.interval,
				BulletType:      0,
				BulletSpeed:     p.bulletSpeed,
				SplashRadius:    p.splashRadius,
				Pierce:          0,
				BurstShots:      0,
				BurstSpacing:    0,
				Spread:          0,
				TargetAir:       false,
				TargetGround:    true,
				TargetPriority:  "nearest",
				HitBuildings:    true,
				PreferBuildings: true,
			})
			continue
		}

		// Units with no weapons get a marker to keep defaults (map overrides may apply).
		if p.damage <= 0 || p.interval <= 0 {
			out = append(out, UnitProfile{
				Name:            unitName,
				FireMode:        "default",
				Range:           0,
				Damage:          0,
				Interval:        0,
				BulletType:      0,
				BulletSpeed:     0,
				SplashRadius:    0,
				Pierce:          0,
				BurstShots:      0,
				BurstSpacing:    0,
				Spread:          0,
				TargetAir:       false,
				TargetGround:    false,
				TargetPriority:  "none",
				HitBuildings:    false,
				PreferBuildings: false,
			})
			continue
		}

		out = append(out, UnitProfile{
			Name:            unitName,
			FireMode:        p.fireMode,
			Range:           p.rangeV,
			Damage:          p.damage,
			Interval:        p.interval,
			BulletType:      0,
			BulletSpeed:     p.bulletSpeed,
			SplashRadius:    p.splashRadius,
			Pierce:          p.pierce,
			BurstShots:      p.burstShots,
			BurstSpacing:    p.burstSpacing,
			Spread:          p.spread,
			TargetAir:       p.targetAir,
			TargetGround:    p.targetGround,
			TargetPriority:  "nearest",
			HitBuildings:    p.targetGround,
			PreferBuildings: false,
		})
	}
	return out
}

func parseWeaponsProfile(body string) parsedProfile {
	matches := reWeaponDecl.FindAllStringIndex(body, -1)
	out := parsedProfile{
		fireMode:     "projectile",
		targetAir:    true,
		targetGround: true,
	}
	found := false
	for _, m := range matches {
		wb, ok := extractInitBody(body, m[1])
		if !ok {
			continue
		}
		p := parseCommonProfile(wb)
		if p.damage <= 0 || p.interval <= 0 {
			continue
		}
		if !found {
			out = p
			found = true
			continue
		}
		// Merge multiple weapon mounts: higher damage/range, faster reload.
		if p.damage > out.damage {
			out.damage = p.damage
		}
		if p.rangeV > out.rangeV {
			out.rangeV = p.rangeV
		}
		if p.interval > 0 && (out.interval <= 0 || p.interval < out.interval) {
			out.interval = p.interval
		}
		if p.bulletSpeed > out.bulletSpeed {
			out.bulletSpeed = p.bulletSpeed
		}
		if p.splashRadius > out.splashRadius {
			out.splashRadius = p.splashRadius
		}
		if p.pierce > out.pierce {
			out.pierce = p.pierce
		}
		out.targetAir = out.targetAir || p.targetAir
		out.targetGround = out.targetGround || p.targetGround
		if p.fireMode == "beam" {
			out.fireMode = "beam"
		}
	}
	if !found {
		return parsedProfile{}
	}
	return out
}

func mergeParsedProfiles(a, b parsedProfile) parsedProfile {
	if b.damage <= 0 && b.interval <= 0 {
		return a
	}
	if a.damage <= 0 || b.damage > a.damage {
		a.damage = b.damage
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
	if b.splashRadius > a.splashRadius {
		a.splashRadius = b.splashRadius
	}
	if b.pierce > a.pierce {
		a.pierce = b.pierce
	}
	if b.burstShots > a.burstShots {
		a.burstShots = b.burstShots
	}
	if b.burstSpacing > 0 && (a.burstSpacing <= 0 || b.burstSpacing < a.burstSpacing) {
		a.burstSpacing = b.burstSpacing
	}
	if b.spread > a.spread {
		a.spread = b.spread
	}
	a.targetAir = a.targetAir || b.targetAir
	a.targetGround = a.targetGround || b.targetGround
	if b.fireMode == "beam" {
		a.fireMode = "beam"
	}
	return a
}

func extractTurrets(src string) []TurretProfile {
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
		p := parseCommonProfile(body)
		if p.damage <= 0 || p.interval <= 0 || p.rangeV <= 0 {
			continue
		}
		out = append(out, TurretProfile{
			Name:           name,
			FireMode:       p.fireMode,
			Range:          p.rangeV,
			Damage:         p.damage,
			Interval:       p.interval,
			BulletType:     0,
			BulletSpeed:    p.bulletSpeed,
			SplashRadius:   p.splashRadius,
			Pierce:         p.pierce,
			TargetAir:      p.targetAir,
			TargetGround:   p.targetGround,
			TargetPriority: "nearest",
			HitBuildings:   p.targetGround,
		})
	}
	return out
}

type parsedProfile struct {
	fireMode     string
	rangeV       float32
	damage       float32
	interval     float32
	bulletSpeed  float32
	splashRadius float32
	pierce       int32
	burstShots   int32
	burstSpacing float32
	spread       float32
	targetAir    bool
	targetGround bool
}

func parseCommonProfile(body string) parsedProfile {
	p := parsedProfile{
		fireMode:     "projectile",
		rangeV:       0,
		damage:       0,
		interval:     0,
		bulletSpeed:  0,
		splashRadius: 0,
		pierce:       0,
		targetAir:    true,
		targetGround: true,
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
	if v, ok := firstValue(body, reSpeed); ok {
		p.bulletSpeed = v
	}
	if v, ok := maxValue(body, reSplashRadius); ok {
		p.splashRadius = v
	}
	if strings.Contains(body, "pierce = true") {
		p.pierce = 1
	}
	if v, ok := maxValue(body, rePierceCap); ok && v > 0 {
		p.pierce = int32(v)
	}
	if m := reTargetAir.FindAllStringSubmatch(body, -1); len(m) > 0 {
		p.targetAir = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if m := reTargetGround.FindAllStringSubmatch(body, -1); len(m) > 0 {
		p.targetGround = strings.EqualFold(m[len(m)-1][1], "true")
	}
	if v, ok := lastValue(body, reShootShots); ok {
		if v > 0 {
			p.burstShots = int32(v)
		}
	}
	if v, ok := lastValue(body, reShootDelay); ok {
		if v > 0 {
			p.burstSpacing = v / 60
		}
	}
	if v, ok := lastValue(body, reShootSpread); ok {
		if v > 0 {
			p.spread = v
		}
	}
	if p.burstShots <= 0 || p.spread <= 0 {
		if matches := reShootCtor.FindAllStringSubmatch(body, -1); len(matches) > 0 {
			for _, m := range matches {
				if len(m) < 2 {
					continue
				}
				args := splitArgs(m[1])
				if len(args) < 2 {
					continue
				}
				shots, ok1 := evalNumericExpr(args[0])
				spread, ok2 := evalNumericExpr(args[1])
				if ok1 && shots > 0 && p.burstShots <= 0 {
					p.burstShots = int32(shots)
				}
				if ok2 && spread > 0 && p.spread <= 0 {
					p.spread = float32(spread)
				}
			}
		}
	}
	return p
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
	return fmt.Sprintf("vanilla gen [out-path], default out: %s", filepath.FromSlash("data/vanilla/profiles.json"))
}
