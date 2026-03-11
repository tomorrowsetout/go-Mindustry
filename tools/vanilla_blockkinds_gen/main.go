package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type blockKindDef struct {
	Block string   `json:"block"`
	Class string   `json:"class"`
	Speed float32  `json:"speed,omitempty"`
	Group string   `json:"group,omitempty"` // distribution/liquid/payload/other
	Kind  string   `json:"kind,omitempty"`  // conveyor/duct/router/junction/bridge/overflow/underflow/payload/liquid
	Flags []string `json:"flags,omitempty"` // phase, surge, armored, stack, etc
}

func main() {
	in := flag.String("in", "", "path to Blocks.java")
	out := flag.String("out", "data/vanilla/block_kinds.json", "output json path")
	flag.Parse()
	if strings.TrimSpace(*in) == "" {
		fmt.Println("missing -in Blocks.java")
		os.Exit(2)
	}
	src, err := os.ReadFile(*in)
	if err != nil {
		fmt.Println("read Blocks.java:", err)
		os.Exit(1)
	}
	defs := parseBlocks(string(src))
	if err := os.MkdirAll(filepathDir(*out), 0755); err != nil {
		fmt.Println("mkdir:", err)
		os.Exit(1)
	}
	data, err := json.MarshalIndent(defs, "", "  ")
	if err != nil {
		fmt.Println("json:", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, data, 0644); err != nil {
		fmt.Println("write:", err)
		os.Exit(1)
	}
	fmt.Printf("block kinds: %d -> %s\n", len(defs), *out)
}

func filepathDir(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}

var blockStartRe = regexp.MustCompile(`new\s+([A-Za-z0-9_$.]+)\s*\(\s*"([^"]+)"\s*\)\s*\{`)
var speedRe = regexp.MustCompile(`(?m)\bspeed\s*=\s*([^;]+);`)

func parseBlocks(src string) []blockKindDef {
	out := []blockKindDef{}
	seen := map[string]bool{}
	matches := blockStartRe.FindAllStringSubmatchIndex(src, -1)
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		className := strings.TrimSpace(src[m[2]:m[3]])
		name := strings.ToLower(strings.TrimSpace(src[m[4]:m[5]]))
		if name == "" || seen[name] {
			continue
		}
		start := strings.Index(src[m[0]:m[1]], "{")
		if start < 0 {
			continue
		}
		start = m[0] + start
		body, ok := extractBlockBody(src, start)
		if !ok {
			continue
		}
		def := blockKindDef{
			Block: name,
			Class: className,
		}
		if v, ok := evalExprFirst(body, speedRe); ok {
			def.Speed = v
		}
		applyKindMeta(&def)
		out = append(out, def)
		seen[name] = true
	}
	return out
}

func applyKindMeta(def *blockKindDef) {
	if def == nil {
		return
	}
	c := strings.ToLower(def.Class)
	n := strings.ToLower(def.Block)
	flags := []string{}
	group := "other"
	kind := ""
	if strings.Contains(c, "payload") || strings.Contains(n, "payload") {
		group = "payload"
		kind = "payload"
		flags = append(flags, "payload")
	} else if strings.Contains(c, "conduit") || strings.Contains(n, "conduit") || strings.Contains(c, "liquid") {
		group = "liquid"
		kind = "liquid"
		flags = append(flags, "liquid")
	} else if strings.Contains(c, "conveyor") || strings.Contains(c, "duct") || strings.Contains(c, "router") ||
		strings.Contains(c, "junction") || strings.Contains(c, "bridge") || strings.Contains(c, "gate") {
		group = "distribution"
	}

	switch {
	case strings.Contains(c, "liquidsource") || strings.Contains(n, "liquid-source"):
		kind = "liquid-source"
	case strings.Contains(c, "liquidvoid") || strings.Contains(n, "liquid-void"):
		kind = "liquid-void"
	case strings.Contains(c, "liquidtank") || strings.Contains(n, "liquid-tank"):
		kind = "liquid-tank"
	case strings.Contains(c, "liquidcontainer") || strings.Contains(n, "liquid-container"):
		kind = "liquid-container"
	case strings.Contains(c, "payloadvoid") || strings.Contains(n, "payload-void"):
		kind = "payload-void"
	case strings.Contains(c, "payloadsource") || strings.Contains(n, "payload-source"):
		kind = "payload-source"
	case strings.Contains(c, "payloadmassdriver") || strings.Contains(n, "payload-mass-driver"):
		kind = "payload-mass-driver"
	case strings.Contains(c, "payloadloader") || strings.Contains(n, "payload-loader"):
		kind = "payload-loader"
	case strings.Contains(c, "payloadunloader") || strings.Contains(n, "payload-unloader"):
		kind = "payload-unloader"
	case strings.Contains(c, "payloadrouter") || strings.Contains(n, "payload-router"):
		kind = "payload-router"
	case strings.Contains(c, "payloadconveyor") || strings.Contains(n, "payload-conveyor"):
		kind = "payload-conveyor"
	case strings.Contains(c, "ductbridge") || strings.Contains(n, "duct-bridge"):
		kind = "duct-bridge"
	case strings.Contains(c, "itembridge") || strings.Contains(n, "bridge-conveyor"):
		kind = "bridge"
	case strings.Contains(c, "phaseconveyor"):
		kind = "phase-conveyor"
	case strings.Contains(c, "plastaniumconveyor"):
		kind = "stack-conveyor"
	case strings.Contains(c, "surgeconveyor"):
		kind = "surge-conveyor"
	case strings.Contains(c, "armoredconveyor"):
		kind = "armored-conveyor"
	case strings.Contains(c, "titaniumconveyor"):
		kind = "titanium-conveyor"
	case strings.Contains(c, "conveyor"):
		kind = "conveyor"
	case strings.Contains(c, "liquidrouter") || strings.Contains(n, "liquid-router"):
		kind = "liquid-router"
	case strings.Contains(c, "liquidjunction") || strings.Contains(n, "liquid-junction"):
		kind = "liquid-junction"
	case strings.Contains(c, "phaseconduit") || strings.Contains(n, "phase-conduit"):
		kind = "phase-conduit"
	case strings.Contains(c, "bridgeconduit") || strings.Contains(n, "bridge-conduit"):
		kind = "bridge-conduit"
	case strings.Contains(c, "pulseconduit") || strings.Contains(n, "pulse-conduit"):
		kind = "pulse-conduit"
	case strings.Contains(c, "platedconduit") || strings.Contains(n, "plated-conduit"):
		kind = "plated-conduit"
	case strings.Contains(c, "armoredconduit") || strings.Contains(n, "armored-conduit"):
		kind = "armored-conduit"
	case strings.Contains(c, "reinforcedconduit") || strings.Contains(n, "reinforced-conduit"):
		kind = "reinforced-conduit"
	case strings.Contains(c, "conduit") || strings.Contains(n, "conduit"):
		kind = "conduit"
	case strings.Contains(c, "ductrouter") || strings.Contains(n, "duct-router"):
		kind = "duct-router"
	case strings.Contains(c, "ductjunction") || strings.Contains(n, "duct-junction"):
		kind = "duct-junction"
	case strings.Contains(c, "ductunloader") || strings.Contains(n, "duct-unloader"):
		kind = "duct-unloader"
	case strings.Contains(c, "overflowduct") || strings.Contains(n, "overflow-duct"):
		kind = "overflow-duct"
	case strings.Contains(c, "underflowduct") || strings.Contains(n, "underflow-duct"):
		kind = "underflow-duct"
	case strings.Contains(c, "duct"):
		kind = "duct"
	case strings.Contains(c, "router") || strings.Contains(n, "router"):
		kind = "router"
	case strings.Contains(c, "junction") || strings.Contains(n, "junction"):
		kind = "junction"
	case strings.Contains(c, "overflowgate") || strings.Contains(n, "overflow-gate"):
		kind = "overflow"
	case strings.Contains(c, "underflowgate") || strings.Contains(n, "underflow-gate"):
		kind = "underflow"
	}

	if strings.Contains(n, "phase") {
		flags = append(flags, "phase")
	}
	if strings.Contains(n, "surge") {
		flags = append(flags, "surge")
	}
	if strings.Contains(n, "armored") || strings.Contains(n, "reinforced") {
		flags = append(flags, "armored")
	}
	if strings.Contains(n, "stack") || strings.Contains(n, "plastanium") {
		flags = append(flags, "stack")
	}
	if strings.Contains(n, "bridge") {
		flags = append(flags, "bridge")
	}
	if strings.Contains(n, "duct") {
		flags = append(flags, "duct")
	}
	if strings.Contains(n, "large") {
		flags = append(flags, "large")
	}
	def.Group = group
	def.Kind = kind
	def.Flags = flags
}

func extractBlockBody(src string, start int) (string, bool) {
	depth := 0
	end := -1
	for i := start; i < len(src); i++ {
		switch src[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				end = i
				break
			}
		}
		if end >= 0 {
			break
		}
	}
	if end < 0 || start+1 > end-1 {
		return "", false
	}
	return src[start+1 : end-1], true
}

func evalExprFirst(body string, re *regexp.Regexp) (float32, bool) {
	m := re.FindStringSubmatch(body)
	if len(m) < 2 {
		return 0, false
	}
	return evalExpr(m[1])
}

func evalExpr(expr string) (float32, bool) {
	clean := strings.ReplaceAll(expr, "f", "")
	clean = strings.ReplaceAll(clean, "F", "")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return 0, false
	}
	for _, r := range clean {
		if (r >= '0' && r <= '9') || r == '.' || r == '+' || r == '-' || r == '*' || r == '/' || r == '(' || r == ')' || r == 'e' || r == 'E' || r == ' ' {
			continue
		}
		return 0, false
	}
	tokens, ok := tokenize(clean)
	if !ok {
		return 0, false
	}
	out, ok := shuntingEval(tokens)
	if !ok {
		return 0, false
	}
	if math.IsNaN(float64(out)) || math.IsInf(float64(out), 0) {
		return 0, false
	}
	return out, true
}

func tokenize(s string) ([]string, bool) {
	var out []string
	i := 0
	for i < len(s) {
		ch := s[i]
		switch {
		case ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r':
			i++
		case ch == '+' || ch == '-' || ch == '*' || ch == '/' || ch == '(' || ch == ')':
			out = append(out, string(ch))
			i++
		case (ch >= '0' && ch <= '9') || ch == '.':
			j := i + 1
			for j < len(s) {
				c := s[j]
				if (c >= '0' && c <= '9') || c == '.' || c == 'e' || c == 'E' || c == '+' || c == '-' {
					j++
					continue
				}
				break
			}
			out = append(out, strings.TrimSpace(s[i:j]))
			i = j
		default:
			return nil, false
		}
	}
	return out, true
}

func shuntingEval(tokens []string) (float32, bool) {
	var vals []float32
	var ops []string
	prec := func(op string) int {
		switch op {
		case "+", "-":
			return 1
		case "*", "/":
			return 2
		default:
			return 0
		}
	}
	apply := func() bool {
		if len(ops) == 0 || len(vals) < 2 {
			return false
		}
		op := ops[len(ops)-1]
		ops = ops[:len(ops)-1]
		b := vals[len(vals)-1]
		a := vals[len(vals)-2]
		vals = vals[:len(vals)-2]
		switch op {
		case "+":
			vals = append(vals, a+b)
		case "-":
			vals = append(vals, a-b)
		case "*":
			vals = append(vals, a*b)
		case "/":
			if b == 0 {
				return false
			}
			vals = append(vals, a/b)
		}
		return true
	}
	for _, tok := range tokens {
		switch tok {
		case "+", "-", "*", "/":
			for len(ops) > 0 && prec(ops[len(ops)-1]) >= prec(tok) {
				if !apply() {
					return 0, false
				}
			}
			ops = append(ops, tok)
		case "(":
			ops = append(ops, tok)
		case ")":
			for len(ops) > 0 && ops[len(ops)-1] != "(" {
				if !apply() {
					return 0, false
				}
			}
			if len(ops) == 0 {
				return 0, false
			}
			ops = ops[:len(ops)-1]
		default:
			v, err := strconv.ParseFloat(tok, 32)
			if err != nil {
				return 0, false
			}
			vals = append(vals, float32(v))
		}
	}
	for len(ops) > 0 {
		if ops[len(ops)-1] == "(" {
			return 0, false
		}
		if !apply() {
			return 0, false
		}
	}
	if len(vals) != 1 {
		return 0, false
	}
	return vals[0], true
}
