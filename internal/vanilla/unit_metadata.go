package vanilla

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type UnitAbilityProfile struct {
	Type                  string  `json:"type"`
	Amount                float32 `json:"amount"`
	Max                   float32 `json:"max"`
	Reload                float32 `json:"reload"`
	Range                 float32 `json:"range"`
	Radius                float32 `json:"radius"`
	Regen                 float32 `json:"regen"`
	Cooldown              float32 `json:"cooldown"`
	Width                 float32 `json:"width"`
	Angle                 float32 `json:"angle"`
	AngleOffset           float32 `json:"angle_offset"`
	X                     float32 `json:"x"`
	Y                     float32 `json:"y"`
	Damage                float32 `json:"damage"`
	StatusID              int16   `json:"status_id"`
	StatusName            string  `json:"status_name,omitempty"`
	StatusDuration        float32 `json:"status_duration"`
	MaxTargets            int32   `json:"max_targets"`
	HealPercent           float32 `json:"heal_percent"`
	SameTypeHealMult      float32 `json:"same_type_heal_mult"`
	ChanceDeflect         float32 `json:"chance_deflect"`
	MissileUnitMultiplier float32 `json:"missile_unit_multiplier"`
	SpawnAmount           int32   `json:"spawn_amount"`
	SpawnRandAmount       int32   `json:"spawn_rand_amount"`
	Spread                float32 `json:"spread"`
	TargetGround          bool    `json:"target_ground"`
	TargetAir             bool    `json:"target_air"`
	HitBuildings          bool    `json:"hit_buildings"`
	HitUnits              bool    `json:"hit_units"`
	Active                bool    `json:"active"`
	WhenShooting          bool    `json:"when_shooting"`
	OnShoot               bool    `json:"on_shoot"`
	UseAmmo               bool    `json:"use_ammo"`
	PushUnits             bool    `json:"push_units"`
	FaceOutwards          bool    `json:"face_outwards"`
	SpawnUnitName         string  `json:"spawn_unit_name,omitempty"`
}

var (
	reFloatDecl  = regexp.MustCompile(`(?m)\bfloat\s+([^;]+);`)
	reIntDecl    = regexp.MustCompile(`(?m)\bint\s+([^;]+);`)
	reBoolDecl   = regexp.MustCompile(`(?m)\bboolean\s+([^;]+);`)
	reAbilityNew = regexp.MustCompile(`new\s+([A-Za-z0-9_$.]+Ability)\s*\(`)
)

type unitMetadata struct {
	health            float32
	armor             float32
	speed             float32
	hitSize           float32
	rotateSpeed       float32
	buildSpeed        float32
	mineSpeed         float32
	mineTier          int16
	itemCapacity      int32
	ammoCapacity      float32
	ammoPerShot       float32
	ammoRegen         float32
	payloadCapacity   float32
	flying            bool
	lowAltitude       bool
	canBoost          bool
	mineWalls         bool
	mineFloor         bool
	coreUnitDock      bool
	allowedInPayloads bool
	pickupUnits       bool
}

func extractLocalNumericVars(body string) map[string]float64 {
	out := map[string]float64{
		"tilePayload": 64,
	}
	parseDecls := func(re *regexp.Regexp) {
		for _, m := range re.FindAllStringSubmatch(body, -1) {
			if len(m) < 2 {
				continue
			}
			for _, part := range splitArgs(m[1]) {
				eq := strings.Index(part, "=")
				if eq < 0 {
					continue
				}
				name := strings.TrimSpace(part[:eq])
				expr := strings.TrimSpace(part[eq+1:])
				if name == "" || expr == "" {
					continue
				}
				if v, ok := evalNumericExprWithVars(expr, out); ok {
					out[name] = v
				}
			}
		}
	}
	parseDecls(reFloatDecl)
	parseDecls(reIntDecl)
	return out
}

func extractLocalBoolVars(body string) map[string]bool {
	out := map[string]bool{}
	for _, m := range reBoolDecl.FindAllStringSubmatch(body, -1) {
		if len(m) < 2 {
			continue
		}
		for _, part := range splitArgs(m[1]) {
			eq := strings.Index(part, "=")
			if eq < 0 {
				continue
			}
			name := strings.TrimSpace(part[:eq])
			value := strings.TrimSpace(part[eq+1:])
			if name == "" {
				continue
			}
			switch value {
			case "true":
				out[name] = true
			case "false":
				out[name] = false
			}
		}
	}
	return out
}

func evalNumericExprWithVars(expr string, vars map[string]float64) (float64, bool) {
	replaced := strings.TrimSpace(expr)
	if replaced == "" {
		return 0, false
	}
	for {
		idx := strings.Index(replaced, "Mathf.sqr(")
		if idx < 0 {
			break
		}
		argStart := idx + len("Mathf.sqr(")
		arg, end, ok := scanParenContent(replaced, argStart-1)
		if !ok {
			return 0, false
		}
		replaced = replaced[:idx] + "((" + arg + ")*(" + arg + "))" + replaced[end+1:]
	}
	keys := make([]string, 0, len(vars))
	for key := range vars {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	for _, key := range keys {
		value := strconv.FormatFloat(vars[key], 'f', -1, 64)
		replaced = regexp.MustCompile(`\b`+regexp.QuoteMeta(key)+`\b`).ReplaceAllString(replaced, value)
	}
	return evalNumericExpr(replaced)
}

func lastNumericAssignWithVars(body, key string, vars map[string]float64) (float32, bool) {
	re := regexp.MustCompile(`(?m)\b` + regexp.QuoteMeta(key) + `\s*=\s*([^;]+);`)
	ms := re.FindAllStringSubmatch(body, -1)
	for i := len(ms) - 1; i >= 0; i-- {
		if len(ms[i]) < 2 {
			continue
		}
		if v, ok := evalNumericExprWithVars(ms[i][1], vars); ok {
			return float32(v), true
		}
	}
	return 0, false
}

func lastBoolAssignWithVars(body, key string, boolVars map[string]bool) (bool, bool) {
	re := regexp.MustCompile(`(?m)\b` + regexp.QuoteMeta(key) + `\s*=\s*([^;]+);`)
	ms := re.FindAllStringSubmatch(body, -1)
	for i := len(ms) - 1; i >= 0; i-- {
		if len(ms[i]) < 2 {
			continue
		}
		value := strings.TrimSpace(ms[i][1])
		switch value {
		case "true":
			return true, true
		case "false":
			return false, true
		default:
			if b, ok := boolVars[value]; ok {
				return b, true
			}
		}
	}
	return false, false
}

func parseUnitMetadata(body string, mounts []WeaponMountProfile, ctorType string) unitMetadata {
	flat := stripNestedInitBodies(body)
	numVars := extractLocalNumericVars(body)
	boolVars := extractLocalBoolVars(body)
	meta := unitMetadata{
		mineFloor:         true,
		allowedInPayloads: true,
		pickupUnits:       true,
	}
	if strings.HasSuffix(strings.TrimSpace(ctorType), "MissileUnitType") {
		meta.allowedInPayloads = false
	}
	meta.health, _ = lastNumericAssignWithVars(flat, "health", numVars)
	meta.armor, _ = lastNumericAssignWithVars(flat, "armor", numVars)
	meta.speed, _ = lastNumericAssignWithVars(flat, "speed", numVars)
	meta.hitSize, _ = lastNumericAssignWithVars(flat, "hitSize", numVars)
	meta.rotateSpeed, _ = lastNumericAssignWithVars(flat, "rotateSpeed", numVars)
	meta.buildSpeed, _ = lastNumericAssignWithVars(flat, "buildSpeed", numVars)
	meta.mineSpeed, _ = lastNumericAssignWithVars(flat, "mineSpeed", numVars)
	if v, ok := lastNumericAssignWithVars(flat, "mineTier", numVars); ok {
		meta.mineTier = int16(v)
	}
	if v, ok := lastNumericAssignWithVars(flat, "itemCapacity", numVars); ok {
		meta.itemCapacity = int32(v)
	}
	if v, ok := lastNumericAssignWithVars(flat, "ammoCapacity", numVars); ok {
		meta.ammoCapacity = v
	}
	if v, ok := lastNumericAssignWithVars(flat, "ammoPerShot", numVars); ok {
		meta.ammoPerShot = v
	}
	if v, ok := lastNumericAssignWithVars(flat, "ammoRegen", numVars); ok {
		meta.ammoRegen = v
	}
	if v, ok := lastNumericAssignWithVars(flat, "payloadCapacity", numVars); ok {
		meta.payloadCapacity = v
	}
	meta.flying, _ = lastBoolAssignWithVars(flat, "flying", boolVars)
	meta.lowAltitude, _ = lastBoolAssignWithVars(flat, "lowAltitude", boolVars)
	meta.canBoost, _ = lastBoolAssignWithVars(flat, "canBoost", boolVars)
	meta.mineWalls, _ = lastBoolAssignWithVars(flat, "mineWalls", boolVars)
	if v, ok := lastBoolAssignWithVars(flat, "mineFloor", boolVars); ok {
		meta.mineFloor = v
	}
	meta.coreUnitDock, _ = lastBoolAssignWithVars(flat, "coreUnitDock", boolVars)
	if v, ok := lastBoolAssignWithVars(flat, "allowedInPayloads", boolVars); ok {
		meta.allowedInPayloads = v
	}
	if v, ok := lastBoolAssignWithVars(flat, "pickupUnits", boolVars); ok {
		meta.pickupUnits = v
	}
	if meta.itemCapacity <= 0 {
		size := meta.hitSize
		if size < 0 {
			size = 0
		}
		derived := int32(size*4 + 0.5)
		if rem := derived % 10; rem != 0 {
			derived += 10 - rem
		}
		if derived < 10 {
			derived = 10
		}
		meta.itemCapacity = derived
	}
	if meta.ammoCapacity <= 0 {
		var shotsPerSecond float32
		for _, mount := range mounts {
			if mount.NoAttack || mount.Interval <= 0 {
				continue
			}
			shotsPerSecond += 1 / mount.Interval
		}
		if shotsPerSecond > 0 {
			meta.ammoCapacity = float32(int(shotsPerSecond*35 + 0.5))
			if meta.ammoCapacity < 1 {
				meta.ammoCapacity = 1
			}
			meta.ammoPerShot = 1
		}
	} else if meta.ammoPerShot <= 0 {
		meta.ammoPerShot = 1
	}
	return meta
}

func extractUnitAbilities(body string, unitNamesByVar map[string]string, statusLookup map[string]statusLookupEntry) ([]UnitAbilityProfile, error) {
	var (
		out      []UnitAbilityProfile
		offset   int
		numVars  = extractLocalNumericVars(body)
		boolVars = extractLocalBoolVars(body)
	)
	for offset < len(body) {
		loc := reAbilityNew.FindStringSubmatchIndex(body[offset:])
		if loc == nil {
			break
		}
		start := offset + loc[0]
		className := simpleAbilityClassName(body[offset+loc[2] : offset+loc[3]])
		argOpen := offset + loc[1] - 1
		argsBody, argClose, ok := scanParenContent(body, argOpen)
		if !ok {
			return nil, fmt.Errorf("parse ability args %s", className)
		}
		anonBody := ""
		next := argClose + 1
		for next < len(body) && (body[next] == ' ' || body[next] == '\n' || body[next] == '\r' || body[next] == '\t') {
			next++
		}
		if next+1 < len(body) && body[next] == '{' && body[next+1] == '{' {
			var anonClose int
			anonBody, anonClose, ok = scanDoubleBraceContent(body, next)
			if !ok {
				return nil, fmt.Errorf("parse ability body %s", className)
			}
			next = anonClose + 1
		}
		profile, supported, err := buildAbilityProfile(className, argsBody, anonBody, numVars, boolVars, unitNamesByVar, statusLookup)
		if err != nil {
			return nil, err
		}
		if !supported {
			return nil, fmt.Errorf("unsupported ability %s", className)
		}
		out = append(out, profile)
		if next <= start {
			next = start + 1
		}
		offset = next
	}
	return out, nil
}

func buildAbilityProfile(className, argsBody, anonBody string, numVars map[string]float64, boolVars map[string]bool, unitNamesByVar map[string]string, statusLookup map[string]statusLookupEntry) (UnitAbilityProfile, bool, error) {
	args := splitArgs(argsBody)
	combined := anonBody
	switch className {
	case "EnergyFieldAbility":
		prof := UnitAbilityProfile{
			Type:             className,
			StatusName:       "electrified",
			StatusDuration:   6,
			MaxTargets:       25,
			HealPercent:      3,
			SameTypeHealMult: 1,
			TargetGround:     true,
			TargetAir:        true,
			HitBuildings:     true,
			HitUnits:         true,
			UseAmmo:          true,
		}
		if len(args) >= 1 {
			prof.Damage = evalArgFloat(args[0], numVars)
		}
		if len(args) >= 2 {
			prof.Reload = evalArgSeconds(args[1], numVars)
		}
		if len(args) >= 3 {
			prof.Range = evalArgFloat(args[2], numVars)
		}
		applyAbilityFloat(&prof.StatusDuration, combined, "statusDuration", numVars, 1.0/60.0)
		if v, ok := lastNumericAssignWithVars(combined, "maxTargets", numVars); ok {
			prof.MaxTargets = int32(v)
		}
		applyAbilityFloat(&prof.HealPercent, combined, "healPercent", numVars, 1)
		applyAbilityFloat(&prof.SameTypeHealMult, combined, "sameTypeHealMult", numVars, 1)
		if statusID, statusName, ok := parseAbilityStatusAssignment(combined, statusLookup); ok {
			prof.StatusID = statusID
			prof.StatusName = statusName
		}
		applyAbilityBool(&prof.TargetGround, combined, "targetGround", boolVars)
		applyAbilityBool(&prof.TargetAir, combined, "targetAir", boolVars)
		applyAbilityBool(&prof.HitBuildings, combined, "hitBuildings", boolVars)
		applyAbilityBool(&prof.HitUnits, combined, "hitUnits", boolVars)
		applyAbilityBool(&prof.UseAmmo, combined, "useAmmo", boolVars)
		return prof, true, nil
	case "ForceFieldAbility":
		prof := UnitAbilityProfile{Type: className}
		if len(args) >= 1 {
			prof.Radius = evalArgFloat(args[0], numVars)
		}
		if len(args) >= 2 {
			prof.Regen = evalArgFloat(args[1], numVars) * 60
		}
		if len(args) >= 3 {
			prof.Max = evalArgFloat(args[2], numVars)
		}
		if len(args) >= 4 {
			prof.Cooldown = evalArgSeconds(args[3], numVars)
		}
		applyAbilityFloat(&prof.Radius, combined, "radius", numVars, 1)
		applyAbilityFloat(&prof.Regen, combined, "regen", numVars, 60)
		applyAbilityFloat(&prof.Max, combined, "max", numVars, 1)
		applyAbilityFloat(&prof.Cooldown, combined, "cooldown", numVars, 1.0/60.0)
		return prof, true, nil
	case "MoveEffectAbility":
		prof := UnitAbilityProfile{Type: className}
		if len(args) >= 1 {
			prof.X = evalArgFloat(args[0], numVars)
		}
		if len(args) >= 2 {
			prof.Y = evalArgFloat(args[1], numVars)
		}
		if len(args) >= 5 {
			prof.Reload = evalArgSeconds(args[4], numVars)
		}
		return prof, true, nil
	case "RepairFieldAbility":
		prof := UnitAbilityProfile{Type: className, SameTypeHealMult: 1}
		if len(args) >= 1 {
			prof.Amount = evalArgFloat(args[0], numVars)
		}
		if len(args) >= 2 {
			prof.Reload = evalArgSeconds(args[1], numVars)
		}
		if len(args) >= 3 {
			prof.Range = evalArgFloat(args[2], numVars)
		}
		if len(args) >= 4 {
			prof.HealPercent = evalArgFloat(args[3], numVars)
		}
		applyAbilityFloat(&prof.Amount, combined, "amount", numVars, 1)
		applyAbilityFloat(&prof.Reload, combined, "reload", numVars, 1.0/60.0)
		applyAbilityFloat(&prof.Range, combined, "range", numVars, 1)
		applyAbilityFloat(&prof.HealPercent, combined, "healPercent", numVars, 1)
		applyAbilityFloat(&prof.SameTypeHealMult, combined, "sameTypeHealMult", numVars, 1)
		return prof, true, nil
	case "ShieldArcAbility":
		prof := UnitAbilityProfile{
			Type:                  className,
			WhenShooting:          true,
			Width:                 6,
			ChanceDeflect:         -1,
			MissileUnitMultiplier: 2,
			PushUnits:             true,
		}
		applyAbilityFloat(&prof.Radius, combined, "radius", numVars, 1)
		applyAbilityFloat(&prof.Regen, combined, "regen", numVars, 60)
		applyAbilityFloat(&prof.Max, combined, "max", numVars, 1)
		applyAbilityFloat(&prof.Cooldown, combined, "cooldown", numVars, 1.0/60.0)
		applyAbilityFloat(&prof.Angle, combined, "angle", numVars, 1)
		applyAbilityFloat(&prof.AngleOffset, combined, "angleOffset", numVars, 1)
		applyAbilityFloat(&prof.X, combined, "x", numVars, 1)
		applyAbilityFloat(&prof.Y, combined, "y", numVars, 1)
		applyAbilityFloat(&prof.Width, combined, "width", numVars, 1)
		applyAbilityFloat(&prof.ChanceDeflect, combined, "chanceDeflect", numVars, 1)
		applyAbilityFloat(&prof.MissileUnitMultiplier, combined, "missileUnitMultiplier", numVars, 1)
		applyAbilityBool(&prof.WhenShooting, combined, "whenShooting", boolVars)
		applyAbilityBool(&prof.PushUnits, combined, "pushUnits", boolVars)
		return prof, true, nil
	case "ShieldRegenFieldAbility":
		prof := UnitAbilityProfile{Type: className}
		if len(args) >= 1 {
			prof.Amount = evalArgFloat(args[0], numVars)
		}
		if len(args) >= 2 {
			prof.Max = evalArgFloat(args[1], numVars)
		}
		if len(args) >= 3 {
			prof.Reload = evalArgSeconds(args[2], numVars)
		}
		if len(args) >= 4 {
			prof.Range = evalArgFloat(args[3], numVars)
		}
		applyAbilityFloat(&prof.Amount, combined, "amount", numVars, 1)
		applyAbilityFloat(&prof.Max, combined, "max", numVars, 1)
		applyAbilityFloat(&prof.Reload, combined, "reload", numVars, 1.0/60.0)
		applyAbilityFloat(&prof.Range, combined, "range", numVars, 1)
		return prof, true, nil
	case "SpawnDeathAbility":
		prof := UnitAbilityProfile{Type: className, FaceOutwards: true}
		if len(args) >= 1 {
			key := strings.TrimSpace(args[0])
			if name, ok := unitNamesByVar[key]; ok {
				prof.SpawnUnitName = name
			}
		}
		if len(args) >= 2 {
			prof.SpawnAmount = int32(evalArgFloat(args[1], numVars))
		}
		if len(args) >= 3 {
			prof.Spread = evalArgFloat(args[2], numVars)
		}
		if v, ok := lastNumericAssignWithVars(combined, "randAmount", numVars); ok {
			prof.SpawnRandAmount = int32(v)
		}
		applyAbilityBool(&prof.FaceOutwards, combined, "faceOutwards", boolVars)
		return prof, true, nil
	case "StatusFieldAbility":
		prof := UnitAbilityProfile{Type: className}
		if len(args) >= 1 {
			if statusID, statusName, ok := parseAbilityStatusRef(args[0], statusLookup); ok {
				prof.StatusID = statusID
				prof.StatusName = statusName
			}
		}
		if len(args) >= 2 {
			prof.StatusDuration = evalArgSeconds(args[1], numVars)
		}
		if len(args) >= 3 {
			prof.Reload = evalArgSeconds(args[2], numVars)
		}
		if len(args) >= 4 {
			prof.Range = evalArgFloat(args[3], numVars)
		}
		applyAbilityBool(&prof.OnShoot, combined, "onShoot", boolVars)
		return prof, true, nil
	case "SuppressionFieldAbility":
		prof := UnitAbilityProfile{
			Type:   className,
			Reload: 1.5,
			Range:  200,
			Active: true,
		}
		applyAbilityFloat(&prof.Reload, combined, "reload", numVars, 1.0/60.0)
		applyAbilityFloat(&prof.Cooldown, combined, "maxDelay", numVars, 1.0/60.0)
		if prof.Cooldown <= 0 {
			prof.Cooldown = prof.Reload
		}
		applyAbilityFloat(&prof.Range, combined, "range", numVars, 1)
		applyAbilityFloat(&prof.X, combined, "x", numVars, 1)
		applyAbilityFloat(&prof.Y, combined, "y", numVars, 1)
		applyAbilityBool(&prof.Active, combined, "active", boolVars)
		return prof, true, nil
	default:
		return UnitAbilityProfile{}, false, nil
	}
}

func applyAbilityFloat(dst *float32, body, key string, vars map[string]float64, scale float32) {
	if dst == nil {
		return
	}
	if v, ok := lastNumericAssignWithVars(body, key, vars); ok {
		*dst = v * scale
	}
}

func applyAbilityBool(dst *bool, body, key string, boolVars map[string]bool) {
	if dst == nil {
		return
	}
	if v, ok := lastBoolAssignWithVars(body, key, boolVars); ok {
		*dst = v
	}
}

func evalArgFloat(arg string, vars map[string]float64) float32 {
	if v, ok := evalNumericExprWithVars(arg, vars); ok {
		return float32(v)
	}
	return 0
}

func evalArgSeconds(arg string, vars map[string]float64) float32 {
	return evalArgFloat(arg, vars) / 60
}

func parseAbilityStatusAssignment(body string, statusLookup map[string]statusLookupEntry) (int16, string, bool) {
	re := regexp.MustCompile(`(?m)\bstatus\s*=\s*([^;]+);`)
	ms := re.FindAllStringSubmatch(body, -1)
	for i := len(ms) - 1; i >= 0; i-- {
		if len(ms[i]) < 2 {
			continue
		}
		if id, name, ok := parseAbilityStatusRef(ms[i][1], statusLookup); ok {
			return id, name, true
		}
	}
	return 0, "", false
}

func parseAbilityStatusRef(expr string, statusLookup map[string]statusLookupEntry) (int16, string, bool) {
	expr = strings.TrimSpace(expr)
	switch {
	case strings.HasPrefix(expr, "StatusEffects."):
		key := strings.TrimPrefix(expr, "StatusEffects.")
		if meta, ok := statusLookup[key]; ok {
			return meta.ID, meta.Name, true
		}
		return 0, key, true
	default:
		if meta, ok := statusLookup[expr]; ok {
			return meta.ID, meta.Name, true
		}
	}
	return 0, "", false
}

func simpleAbilityClassName(name string) string {
	name = strings.TrimSpace(name)
	if idx := strings.LastIndex(name, "."); idx >= 0 {
		name = name[idx+1:]
	}
	return name
}

func scanParenContent(src string, open int) (string, int, bool) {
	if open < 0 || open >= len(src) || src[open] != '(' {
		return "", 0, false
	}
	depth := 1
	start := open + 1
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return src[start:i], i, true
			}
		}
	}
	return "", 0, false
}

func scanDoubleBraceContent(src string, open int) (string, int, bool) {
	if open < 0 || open+1 >= len(src) || src[open] != '{' || src[open+1] != '{' {
		return "", 0, false
	}
	depth := 2
	start := open + 2
	for i := start; i < len(src)-1; i++ {
		if src[i] == '{' && src[i+1] == '{' {
			depth += 2
			i++
			continue
		}
		if src[i] == '}' && src[i+1] == '}' {
			depth -= 2
			if depth == 0 {
				return src[start:i], i + 1, true
			}
			i++
		}
	}
	return "", 0, false
}
