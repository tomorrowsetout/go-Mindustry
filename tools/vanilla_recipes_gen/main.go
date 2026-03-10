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

type RecipeItem struct {
	Name   string `json:"name"`
	Amount int32  `json:"amount"`
}

type RecipeLiquid struct {
	Name   string  `json:"name"`
	Amount float32 `json:"amount"`
}

type RecipeDef struct {
	Block         string         `json:"block"`
	CraftTime     float32        `json:"craftTime"`
	Power         float32        `json:"power,omitempty"`
	PowerBuffered float32        `json:"powerBuffered,omitempty"`
	InputItems    []RecipeItem   `json:"inputItems,omitempty"`
	InputLiquids  []RecipeLiquid `json:"inputLiquids,omitempty"`
	OutputItems   []RecipeItem   `json:"outputItems,omitempty"`
	OutputLiquids []RecipeLiquid `json:"outputLiquids,omitempty"`
}

func main() {
	in := flag.String("in", "", "path to Blocks.java")
	out := flag.String("out", "data/vanilla/recipes.json", "output json path")
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
	fmt.Printf("recipes: %d -> %s\n", len(defs), *out)
}

func filepathDir(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}

var blockStartRe = regexp.MustCompile(`new\s+[A-Za-z0-9_$.]+\s*\(\s*"([^"]+)"\s*\)\s*\{`)
var craftTimeRe = regexp.MustCompile(`craftTime\s*=\s*([^;]+);`)
var outputItemRe = regexp.MustCompile(`outputItem\s*=\s*new\s+ItemStack\s*\(\s*Items\.([A-Za-z0-9_]+)\s*,\s*([^)]+)\)`)
var outputLiquidRe = regexp.MustCompile(`outputLiquid\s*=\s*new\s+LiquidStack\s*\(\s*Liquids\.([A-Za-z0-9_]+)\s*,\s*([^)]+)\)`)
var outputLiquidsRe = regexp.MustCompile(`outputLiquids\s*=\s*LiquidStack\.with\s*\(([^)]+)\)`)
var consumeItemsRe = regexp.MustCompile(`consumeItems\s*\(\s*(?:ItemStack\.)?with\s*\(([^)]+)\)\s*\)`)
var consumeItemRe = regexp.MustCompile(`consumeItem\s*\(\s*Items\.([A-Za-z0-9_]+)(?:\s*,\s*([^)]+))?\)`)
var consumeLiquidRe = regexp.MustCompile(`consumeLiquid\s*\(\s*Liquids\.([A-Za-z0-9_]+)\s*,\s*([^)]+)\)`)
var consumeLiquidsRe = regexp.MustCompile(`consumeLiquids\s*\(\s*LiquidStack\.with\s*\(([^)]+)\)\s*\)`)
var consumePowerRe = regexp.MustCompile(`consumePower\s*\(\s*([^)]+)\)`)
var consumePowerBufRe = regexp.MustCompile(`consumePowerBuffered\s*\(\s*([^)]+)\)`)

func parseBlocks(src string) []RecipeDef {
	matches := blockStartRe.FindAllStringSubmatchIndex(src, -1)
	out := make([]RecipeDef, 0, len(matches))
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		name := src[m[2]:m[3]]
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
		if len(def.OutputItems) == 0 && len(def.OutputLiquids) == 0 {
			continue
		}
		out = append(out, def)
	}
	return out
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

func parseBlockBody(name, body string) RecipeDef {
	def := RecipeDef{Block: strings.ToLower(strings.TrimSpace(name))}
	if m := craftTimeRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.CraftTime = v
		}
	}
	if m := outputItemRe.FindStringSubmatch(body); len(m) > 2 {
		if v, ok := evalExpr(m[2]); ok {
			def.OutputItems = append(def.OutputItems, RecipeItem{Name: strings.ToLower(m[1]), Amount: int32(math.Round(float64(v)))})
		}
	}
	if m := outputLiquidRe.FindStringSubmatch(body); len(m) > 2 {
		if v, ok := evalExpr(m[2]); ok {
			def.OutputLiquids = append(def.OutputLiquids, RecipeLiquid{Name: strings.ToLower(m[1]), Amount: v})
		}
	}
	if m := outputLiquidsRe.FindStringSubmatch(body); len(m) > 1 {
		def.OutputLiquids = append(def.OutputLiquids, parseLiquidPairs(m[1])...)
	}
	if m := consumeItemsRe.FindStringSubmatch(body); len(m) > 1 {
		def.InputItems = append(def.InputItems, parseItemPairs(m[1])...)
	}
	for _, m := range consumeItemRe.FindAllStringSubmatch(body, -1) {
		if len(m) < 2 {
			continue
		}
		amt := float32(1)
		if len(m) > 2 && strings.TrimSpace(m[2]) != "" {
			if v, ok := evalExpr(m[2]); ok {
				amt = v
			}
		}
		def.InputItems = append(def.InputItems, RecipeItem{Name: strings.ToLower(m[1]), Amount: int32(math.Round(float64(amt)))})
	}
	for _, m := range consumeLiquidRe.FindAllStringSubmatch(body, -1) {
		if len(m) < 3 {
			continue
		}
		if v, ok := evalExpr(m[2]); ok {
			def.InputLiquids = append(def.InputLiquids, RecipeLiquid{Name: strings.ToLower(m[1]), Amount: v})
		}
	}
	if m := consumeLiquidsRe.FindStringSubmatch(body); len(m) > 1 {
		def.InputLiquids = append(def.InputLiquids, parseLiquidPairs(m[1])...)
	}
	if m := consumePowerRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.Power = v
		}
	}
	if m := consumePowerBufRe.FindStringSubmatch(body); len(m) > 1 {
		if v, ok := evalExpr(m[1]); ok {
			def.PowerBuffered = v
		}
	}
	return def
}

func parseItemPairs(arg string) []RecipeItem {
	tokens := splitCSV(arg)
	out := []RecipeItem{}
	for i := 0; i+1 < len(tokens); i += 2 {
		name := strings.TrimSpace(tokens[i])
		if strings.HasPrefix(name, "Items.") {
			name = strings.TrimPrefix(name, "Items.")
		}
		if name == "" {
			continue
		}
		if v, ok := evalExpr(tokens[i+1]); ok {
			out = append(out, RecipeItem{Name: strings.ToLower(name), Amount: int32(math.Round(float64(v)))})
		}
	}
	return out
}

func parseLiquidPairs(arg string) []RecipeLiquid {
	tokens := splitCSV(arg)
	out := []RecipeLiquid{}
	for i := 0; i+1 < len(tokens); i += 2 {
		name := strings.TrimSpace(tokens[i])
		if strings.HasPrefix(name, "Liquids.") {
			name = strings.TrimPrefix(name, "Liquids.")
		}
		if name == "" {
			continue
		}
		if v, ok := evalExpr(tokens[i+1]); ok {
			out = append(out, RecipeLiquid{Name: strings.ToLower(name), Amount: v})
		}
	}
	return out
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
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
