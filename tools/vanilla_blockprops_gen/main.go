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

type BlockPropsDef struct {
	Block             string  `json:"block"`
	ItemCapacity      float32 `json:"itemCapacity,omitempty"`
	LiquidCapacity    float32 `json:"liquidCapacity,omitempty"`
	LiquidPressure    float32 `json:"liquidPressure,omitempty"`
	PowerCapacity     float32 `json:"powerCapacity,omitempty"`
	PowerProduction   float32 `json:"powerProduction,omitempty"`
	PowerOutput       float32 `json:"powerOutput,omitempty"`
	PowerUse          float32 `json:"powerUse,omitempty"`
	LinkRange         float32 `json:"linkRange,omitempty"`
	DrillTime         float32 `json:"drillTime,omitempty"`
	PumpAmount        float32 `json:"pumpAmount,omitempty"`
	ItemDrop          string  `json:"itemDrop,omitempty"`
	LiquidDrop        string  `json:"liquidDrop,omitempty"`
	LiquidBoostName   string  `json:"boostLiquid,omitempty"`
	LiquidBoostAmount float32 `json:"boostAmount,omitempty"`
	LiquidBoostMul    float32 `json:"boostMultiplier,omitempty"`
}

func main() {
	in := flag.String("in", "", "path to Blocks.java")
	out := flag.String("out", "data/vanilla/block_props.json", "output json path")
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
	fmt.Printf("block props: %d -> %s\n", len(defs), *out)
}

func filepathDir(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}

var blockStartRe = regexp.MustCompile(`new\s+[A-Za-z0-9_$.]+\s*\(\s*"([^"]+)"\s*\)\s*\{`)
var itemCapacityRe = regexp.MustCompile(`itemCapacity\s*=\s*([^;]+);`)
var liquidCapacityRe = regexp.MustCompile(`liquidCapacity\s*=\s*([^;]+);`)
var liquidPressureRe = regexp.MustCompile(`liquidPressure\s*=\s*([^;]+);`)
var powerCapacityRe = regexp.MustCompile(`powerCapacity\s*=\s*([^;]+);`)
var powerProductionRe = regexp.MustCompile(`powerProduction\s*=\s*([^;]+);`)
var powerOutputRe = regexp.MustCompile(`powerOutput\s*=\s*([^;]+);`)
var powerUseRe = regexp.MustCompile(`consumePower\s*\(\s*([^)]+)\)`)
var linkRangeRe = regexp.MustCompile(`laserRange\s*=\s*([^;]+);`)
var drillTimeRe = regexp.MustCompile(`drillTime\s*=\s*([^;]+);`)
var pumpAmountRe = regexp.MustCompile(`pumpAmount\s*=\s*([^;]+);`)
var itemDropRe = regexp.MustCompile(`itemDrop\s*=\s*Items\.([A-Za-z0-9_]+);`)
var liquidDropRe = regexp.MustCompile(`liquidDrop\s*=\s*Liquids\.([A-Za-z0-9_]+);`)
var liquidBoostMulRe = regexp.MustCompile(`liquidBoostIntensity\s*=\s*([^;]+);`)
var liquidBoostRe = regexp.MustCompile(`consumeLiquid\s*\(\s*Liquids\.([A-Za-z0-9_]+)\s*,\s*([^)]+)\)\s*\.boost\(\)`)
var oreBlockNoNameRe = regexp.MustCompile(`([A-Za-z0-9_]+)\s*=\s*new\s+OreBlock\s*\(\s*Items\.([A-Za-z0-9_]+)\s*\)`)
var oreBlockNamedRe = regexp.MustCompile(`new\s+OreBlock\s*\(\s*"([^"]+)"\s*,\s*Items\.([A-Za-z0-9_]+)\s*\)`)

func parseBlocks(src string) []BlockPropsDef {
	out := []BlockPropsDef{}
	seen := map[string]bool{}
	matches := blockStartRe.FindAllStringSubmatchIndex(src, -1)
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(src[m[2]:m[3]]))
		if name == "" {
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
		def := parseBlockBody(name, body)
		if def.Block == "" {
			continue
		}
		if defIsEmpty(def) {
			continue
		}
		out = append(out, def)
		seen[def.Block] = true
	}
	for _, m := range oreBlockNoNameRe.FindAllStringSubmatch(src, -1) {
		if len(m) < 3 {
			continue
		}
		item := strings.ToLower(strings.TrimSpace(m[2]))
		if item == "" {
			continue
		}
		name := "ore-" + item
		if seen[name] {
			continue
		}
		out = append(out, BlockPropsDef{Block: name, ItemDrop: item})
		seen[name] = true
	}
	for _, m := range oreBlockNamedRe.FindAllStringSubmatch(src, -1) {
		if len(m) < 3 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(m[1]))
		item := strings.ToLower(strings.TrimSpace(m[2]))
		if name == "" || item == "" {
			continue
		}
		if seen[name] {
			continue
		}
		out = append(out, BlockPropsDef{Block: name, ItemDrop: item})
		seen[name] = true
	}
	return out
}

func defIsEmpty(d BlockPropsDef) bool {
	return d.ItemCapacity == 0 && d.LiquidCapacity == 0 && d.LiquidPressure == 0 &&
		d.PowerCapacity == 0 && d.PowerProduction == 0 && d.PowerOutput == 0 && d.PowerUse == 0 &&
		d.LinkRange == 0 && d.DrillTime == 0 && d.PumpAmount == 0 &&
		d.ItemDrop == "" && d.LiquidDrop == "" && d.LiquidBoostName == "" && d.LiquidBoostMul == 0
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

func parseBlockBody(name, body string) BlockPropsDef {
	def := BlockPropsDef{Block: strings.ToLower(strings.TrimSpace(name))}
	if m := itemCapacityRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.ItemCapacity = v
		}
	}
	if m := liquidCapacityRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.LiquidCapacity = v
		}
	}
	if m := liquidPressureRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.LiquidPressure = v
		}
	}
	if m := powerCapacityRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PowerCapacity = v
		}
	}
	if m := powerProductionRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PowerProduction = v
		}
	}
	if m := powerOutputRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PowerOutput = v
		}
	}
	if m := powerUseRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PowerUse = v
		}
	}
	if m := linkRangeRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.LinkRange = v
		}
	}
	if m := drillTimeRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.DrillTime = v
		}
	}
	if m := pumpAmountRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PumpAmount = v
		}
	}
	if m := itemDropRe.FindStringSubmatch(body); len(m) > 1 {
		def.ItemDrop = strings.ToLower(strings.TrimSpace(m[1]))
	}
	if m := liquidDropRe.FindStringSubmatch(body); len(m) > 1 {
		def.LiquidDrop = strings.ToLower(strings.TrimSpace(m[1]))
	}
	if m := liquidBoostMulRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.LiquidBoostMul = v
		}
	}
	if m := liquidBoostRe.FindStringSubmatch(body); len(m) > 2 {
		def.LiquidBoostName = strings.ToLower(strings.TrimSpace(m[1]))
		if v, ok := evalExpr(m[2]); ok {
			def.LiquidBoostAmount = v
		}
	}
	return def
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
		default:
			return false
		}
		return true
	}
	for i := 0; i < len(tokens); i++ {
		t := tokens[i]
		switch t {
		case "+", "-", "*", "/":
			for len(ops) > 0 && prec(ops[len(ops)-1]) >= prec(t) {
				if !apply() {
					return 0, false
				}
			}
			ops = append(ops, t)
		case "(":
			ops = append(ops, t)
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
			v, err := strconv.ParseFloat(t, 32)
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
