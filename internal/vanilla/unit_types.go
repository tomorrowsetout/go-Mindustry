package vanilla

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type UnitTypeDef struct {
	Name        string
	Health      float32
	Armor       float32
	Speed       float32
	HitSize     float32
	RotateSpeed float32
	Weapon      WeaponDef
}

type WeaponDef struct {
	FireMode     string
	Range        float32
	Damage       float32
	Interval     float32
	BulletSpeed  float32
	SplashRadius float32
	Pierce       int32
	TargetAir    bool
	TargetGround bool
}

var reUnitDeclUT = regexp.MustCompile(`(?m)(\w+)\s*=\s*new\s+UnitType\("([^"]+)"\)\s*\{\{`)
var reAssign = func(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?m)\b` + name + `\s*=\s*([^;]+);`)
}

var (
	reWeaponDeclUT = regexp.MustCompile(`(?m)new\s+Weapon\([^)]*\)\s*\{\{`)
	reBulletCtorUT = regexp.MustCompile(`(?m)new\s+([A-Za-z0-9_$.]+)BulletType\s*\(([^)]*)\)`)
)

func GenerateUnitTypes(unitTypesPath, outPath string) error {
	src, err := os.ReadFile(unitTypesPath)
	if err != nil {
		return err
	}
	units := extractUnitTypes(string(src))
	if len(units) == 0 {
		return fmt.Errorf("no units parsed from %s", unitTypesPath)
	}
	if err := os.MkdirAll(filepath.Dir(outPath), 0755); err != nil {
		return err
	}
	return writeUnitTypesGo(outPath, units)
}

func extractUnitTypes(src string) []UnitTypeDef {
	matches := reUnitDeclUT.FindAllStringSubmatchIndex(src, -1)
	out := make([]UnitTypeDef, 0, len(matches))
	for _, m := range matches {
		name := strings.ToLower(strings.TrimSpace(src[m[4]:m[5]]))
		bodyStart := m[1]
		body, ok := extractInitBodyUT(src, bodyStart)
		if !ok {
			continue
		}
		def := UnitTypeDef{Name: name}
		def.Health = parseFloatAssign(body, "health")
		def.Armor = parseFloatAssign(body, "armor")
		def.Speed = parseFloatAssign(body, "speed")
		def.HitSize = parseFloatAssign(body, "hitSize")
		def.RotateSpeed = parseFloatAssign(body, "rotateSpeed")

		def.Weapon = parseWeaponDef(body)
		out = append(out, def)
	}
	return out
}

func parseFloatAssign(body, key string) float32 {
	re := reAssign(key)
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		return 0
	}
	if v, ok := evalFloat(m[1]); ok {
		return v
	}
	return 0
}

func parseWeaponDef(body string) WeaponDef {
	matches := reWeaponDeclUT.FindAllStringIndex(body, -1)
	out := WeaponDef{FireMode: "projectile", TargetAir: true, TargetGround: true}
	found := false
	for _, m := range matches {
		wb, ok := extractInitBodyUT(body, m[1])
		if !ok {
			continue
		}
		p := parseWeaponProfile(wb)
		if p.Damage <= 0 || p.Interval <= 0 {
			continue
		}
		if !found {
			out = p
			found = true
			continue
		}
		if p.Damage > out.Damage {
			out.Damage = p.Damage
		}
		if p.Range > out.Range {
			out.Range = p.Range
		}
		if p.Interval > 0 && (out.Interval <= 0 || p.Interval < out.Interval) {
			out.Interval = p.Interval
		}
		if p.BulletSpeed > out.BulletSpeed {
			out.BulletSpeed = p.BulletSpeed
		}
		if p.SplashRadius > out.SplashRadius {
			out.SplashRadius = p.SplashRadius
		}
		if p.Pierce > out.Pierce {
			out.Pierce = p.Pierce
		}
		out.TargetAir = out.TargetAir || p.TargetAir
		out.TargetGround = out.TargetGround || p.TargetGround
		if p.FireMode == "beam" {
			out.FireMode = "beam"
		}
	}
	if !found {
		return WeaponDef{}
	}
	return out
}

func parseWeaponProfile(body string) WeaponDef {
	out := WeaponDef{FireMode: "projectile", TargetAir: true, TargetGround: true}
	if v, ok := evalFloatFirst(body, `\breload\s*=\s*([^;]+);`); ok && v > 0 {
		out.Interval = v / 60
	}
	if v, ok := evalFloatFirst(body, `\brange\s*=\s*([^;]+);`); ok && v > 0 {
		out.Range = v
	}
	if v, ok := evalFloatFirst(body, `\bmaxRange\s*=\s*([^;]+);`); ok && v > 0 {
		out.Range = v
	}
	if v, ok := evalFloatFirst(body, `\bdamage\s*=\s*([^;]+);`); ok && v > 0 {
		out.Damage = v
	}
	if v, ok := evalFloatFirst(body, `\bsplashDamage\s*=\s*([^;]+);`); ok && v > 0 {
		out.Damage = v
	}
	if v, ok := evalFloatFirst(body, `\bsplashDamageRadius\s*=\s*([^;]+);`); ok && v > 0 {
		out.SplashRadius = v
	}
	if v, ok := evalFloatFirst(body, `\bpierceCap\s*=\s*([^;]+);`); ok && v > 0 {
		out.Pierce = int32(v)
	}
	if m := regexp.MustCompile(`(?m)\btargetAir\s*=\s*(true|false)\s*;`).FindStringSubmatch(body); len(m) == 2 {
		out.TargetAir = (m[1] == "true")
	}
	if m := regexp.MustCompile(`(?m)\btargetGround\s*=\s*(true|false)\s*;`).FindStringSubmatch(body); len(m) == 2 {
		out.TargetGround = (m[1] == "true")
	}
	if m := reBulletCtorUT.FindStringSubmatch(body); len(m) >= 3 {
		args := splitArgsUT(m[2])
		if len(args) >= 1 {
			if v, ok := evalFloat(args[0]); ok {
				out.BulletSpeed = v
			}
		}
		if len(args) >= 2 {
			if v, ok := evalFloat(args[1]); ok {
				out.Damage = v
			}
		}
		cls := strings.ToLower(m[1])
		if strings.Contains(cls, "laser") || strings.Contains(cls, "beam") {
			out.FireMode = "beam"
		}
	}
	return out
}

func evalFloatFirst(body, pattern string) (float32, bool) {
	re := regexp.MustCompile(pattern)
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		return 0, false
	}
	return evalFloat(m[1])
}

func extractInitBodyUT(src string, start int) (string, bool) {
	depth := 0
	i := start
	for i < len(src)-1 {
		if src[i] == '{' && src[i+1] == '{' {
			depth++
			i += 2
			break
		}
		i++
	}
	if depth == 0 {
		return "", false
	}
	bodyStart := i
	for i < len(src)-1 {
		if src[i] == '{' && src[i+1] == '{' {
			depth++
			i += 2
			continue
		}
		if src[i] == '}' && src[i+1] == '}' {
			depth--
			if depth == 0 {
				return src[bodyStart:i], true
			}
			i += 2
			continue
		}
		i++
	}
	return "", false
}

func splitArgsUT(s string) []string {
	parts := []string{}
	cur := strings.Builder{}
	depth := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		switch ch {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(cur.String()))
				cur.Reset()
				continue
			}
		}
		cur.WriteByte(ch)
	}
	if cur.Len() > 0 {
		parts = append(parts, strings.TrimSpace(cur.String()))
	}
	return parts
}

func evalFloat(expr string) (float32, bool) {
	tokens, ok := tokenize(expr)
	if !ok {
		return 0, false
	}
	rpn, ok := toRPN(tokens)
	if !ok {
		return 0, false
	}
	val, ok := evalRPN(rpn)
	if !ok {
		return 0, false
	}
	return float32(val), true
}

type token struct {
	kind string
	val  float64
	op   rune
}

func tokenize(expr string) ([]token, bool) {
	expr = strings.TrimSpace(expr)
	expr = strings.ReplaceAll(expr, "f", "")
	expr = strings.ReplaceAll(expr, "d", "")
	expr = strings.ReplaceAll(expr, "F", "")
	expr = strings.ReplaceAll(expr, "D", "")
	out := []token{}
	i := 0
	for i < len(expr) {
		ch := expr[i]
		if ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
			i++
			continue
		}
		if ch == '(' || ch == ')' || ch == '+' || ch == '-' || ch == '*' || ch == '/' {
			out = append(out, token{kind: "op", op: rune(ch)})
			i++
			continue
		}
		if (ch >= '0' && ch <= '9') || ch == '.' {
			j := i + 1
			for j < len(expr) && ((expr[j] >= '0' && expr[j] <= '9') || expr[j] == '.') {
				j++
			}
			v, err := strconv.ParseFloat(expr[i:j], 64)
			if err != nil {
				return nil, false
			}
			out = append(out, token{kind: "num", val: v})
			i = j
			continue
		}
		// unsupported identifier
		return nil, false
	}
	return out, true
}

func precedence(op rune) int {
	switch op {
	case '+', '-':
		return 1
	case '*', '/':
		return 2
	default:
		return 0
	}
}

func toRPN(tokens []token) ([]token, bool) {
	out := []token{}
	stack := []token{}
	for _, t := range tokens {
		if t.kind == "num" {
			out = append(out, t)
			continue
		}
		if t.op == '(' {
			stack = append(stack, t)
			continue
		}
		if t.op == ')' {
			for len(stack) > 0 && stack[len(stack)-1].op != '(' {
				out = append(out, stack[len(stack)-1])
				stack = stack[:len(stack)-1]
			}
			if len(stack) == 0 {
				return nil, false
			}
			stack = stack[:len(stack)-1]
			continue
		}
		for len(stack) > 0 && stack[len(stack)-1].op != '(' && precedence(stack[len(stack)-1].op) >= precedence(t.op) {
			out = append(out, stack[len(stack)-1])
			stack = stack[:len(stack)-1]
		}
		stack = append(stack, t)
	}
	for i := len(stack) - 1; i >= 0; i-- {
		if stack[i].op == '(' {
			return nil, false
		}
		out = append(out, stack[i])
	}
	return out, true
}

func evalRPN(tokens []token) (float64, bool) {
	stack := []float64{}
	for _, t := range tokens {
		if t.kind == "num" {
			stack = append(stack, t.val)
			continue
		}
		if len(stack) < 2 {
			return 0, false
		}
		b := stack[len(stack)-1]
		a := stack[len(stack)-2]
		stack = stack[:len(stack)-2]
		switch t.op {
		case '+':
			stack = append(stack, a+b)
		case '-':
			stack = append(stack, a-b)
		case '*':
			stack = append(stack, a*b)
		case '/':
			stack = append(stack, a/b)
		default:
			return 0, false
		}
	}
	if len(stack) != 1 {
		return 0, false
	}
	return stack[0], true
}

func writeUnitTypesGo(outPath string, units []UnitTypeDef) error {
	sort.Slice(units, func(i, j int) bool { return units[i].Name < units[j].Name })
	var b strings.Builder
	b.WriteString("// Code generated by vanilla_unittypes_gen; DO NOT EDIT.\n")
	b.WriteString("package vanilla\n\n")
	b.WriteString("var UnitTypesByName = map[string]UnitTypeDef{\n")
	for _, u := range units {
		fmt.Fprintf(&b, "\t%q: {Name: %q, Health: %.3f, Armor: %.3f, Speed: %.3f, HitSize: %.3f, RotateSpeed: %.3f, Weapon: WeaponDef{FireMode: %q, Range: %.3f, Damage: %.3f, Interval: %.6f, BulletSpeed: %.3f, SplashRadius: %.3f, Pierce: %d, TargetAir: %v, TargetGround: %v}},\n",
			u.Name, u.Name, u.Health, u.Armor, u.Speed, u.HitSize, u.RotateSpeed,
			u.Weapon.FireMode, u.Weapon.Range, u.Weapon.Damage, u.Weapon.Interval, u.Weapon.BulletSpeed, u.Weapon.SplashRadius, u.Weapon.Pierce, u.Weapon.TargetAir, u.Weapon.TargetGround)
	}
	b.WriteString("}\n")
	return os.WriteFile(outPath, []byte(b.String()), 0644)
}
