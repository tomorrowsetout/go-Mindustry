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

type buildReq struct {
	Item   string `json:"item"`
	Amount int32  `json:"amount"`
}

type blockBuildDef struct {
	Block               string     `json:"block"`
	BuildTime           float32    `json:"buildTime,omitempty"`
	BuildCostMultiplier float32    `json:"buildCostMultiplier,omitempty"`
	Requirements        []buildReq `json:"requirements,omitempty"`
}

type itemDef struct {
	Name string
	Cost float32
}

func main() {
	itemsPath := flag.String("items", "", "path to Items.java")
	blocksPath := flag.String("blocks", "", "path to Blocks.java")
	out := flag.String("out", "data/vanilla/block_build.json", "output json path")
	flag.Parse()
	if strings.TrimSpace(*itemsPath) == "" || strings.TrimSpace(*blocksPath) == "" {
		fmt.Println("missing -items Items.java or -blocks Blocks.java")
		os.Exit(2)
	}
	itemsSrc, err := os.ReadFile(*itemsPath)
	if err != nil {
		fmt.Println("read Items.java:", err)
		os.Exit(1)
	}
	blocksSrc, err := os.ReadFile(*blocksPath)
	if err != nil {
		fmt.Println("read Blocks.java:", err)
		os.Exit(1)
	}
	itemsByVar, itemsByName := parseItems(string(itemsSrc))
	varToBlockName := parseBlockVarNames(string(blocksSrc))
	defs := parseBlocks(string(blocksSrc), itemsByVar, itemsByName, varToBlockName)
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
	fmt.Printf("block build: %d -> %s\n", len(defs), *out)
}

func filepathDir(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}

var itemDeclRe = regexp.MustCompile(`(?m)(\w+)\s*=\s*new\s+Item\s*\(\s*"([^"]+)"`)
var blockAssignRe = regexp.MustCompile(`(?m)(\w+)\s*=\s*new\s+[A-Za-z0-9_$.]+\s*\(\s*"([^"]+)"`)
var blockStartRe = regexp.MustCompile(`new\s+[A-Za-z0-9_$.]+\s*\(\s*"([^"]+)"\s*\)\s*\{`)
var costAssignRe = regexp.MustCompile(`(?m)\bcost\s*=\s*([^;]+);`)
var buildCostMultRe = regexp.MustCompile(`(?m)\bbuildCostMultiplier\s*=\s*([^;]+);`)
var buildTimeRe = regexp.MustCompile(`(?m)\bbuildTime\s*=\s*([^;]+);`)

func parseItems(src string) (map[string]itemDef, map[string]itemDef) {
	byVar := map[string]itemDef{}
	byName := map[string]itemDef{}
	matches := itemDeclRe.FindAllStringSubmatchIndex(src, -1)
	for _, m := range matches {
		if len(m) < 6 {
			continue
		}
		varName := strings.TrimSpace(src[m[2]:m[3]])
		name := strings.ToLower(strings.TrimSpace(src[m[4]:m[5]]))
		bodyStart := m[1]
		body, ok := extractBlockBody(src, bodyStart)
		cost := float32(1)
		if ok {
			if v, ok2 := evalExprFirst(body, costAssignRe); ok2 {
				cost = v
			}
		}
		def := itemDef{Name: name, Cost: cost}
		if varName != "" {
			byVar[varName] = def
		}
		if name != "" {
			byName[name] = def
		}
	}
	return byVar, byName
}

func parseBlockVarNames(src string) map[string]string {
	out := map[string]string{}
	for _, m := range blockAssignRe.FindAllStringSubmatchIndex(src, -1) {
		if len(m) < 6 {
			continue
		}
		varName := strings.TrimSpace(src[m[2]:m[3]])
		name := strings.ToLower(strings.TrimSpace(src[m[4]:m[5]]))
		if varName != "" && name != "" {
			out[varName] = name
		}
	}
	return out
}

func parseBlocks(src string, itemsByVar, itemsByName map[string]itemDef, varToBlockName map[string]string) []blockBuildDef {
	out := []blockBuildDef{}
	seen := map[string]bool{}
	matches := blockStartRe.FindAllStringSubmatchIndex(src, -1)
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(src[m[2]:m[3]]))
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
		buildMult := float32(1)
		if v, ok := evalExprFirst(body, buildCostMultRe); ok && v > 0 {
			buildMult = v
		}
		buildTime := float32(0)
		if v, ok := evalExprFirst(body, buildTimeRe); ok && v > 0 {
			buildTime = v
		}
		reqs := parseRequirements(body, itemsByVar, varToBlockName, out)
		if buildTime <= 0 {
			if len(reqs) > 0 {
				for _, r := range reqs {
					cost := float32(1)
					if item, ok := itemsByName[r.Item]; ok && item.Cost > 0 {
						cost = item.Cost
					}
					buildTime += float32(r.Amount) * cost
				}
			} else {
				buildTime = 20
			}
		}
		if buildMult <= 0 {
			buildMult = 1
		}
		buildTime *= buildMult
		def := blockBuildDef{
			Block:               name,
			BuildTime:           buildTime,
			BuildCostMultiplier: buildMult,
			Requirements:        reqs,
		}
		out = append(out, def)
		seen[name] = true
	}
	return out
}

func parseRequirements(body string, itemsByVar map[string]itemDef, varToBlockName map[string]string, defs []blockBuildDef) []buildReq {
	args, ok := findCallArgs(body, "requirements")
	if !ok {
		return nil
	}
	parts := splitTopLevelArgs(args)
	if len(parts) == 0 {
		return nil
	}
	expr := strings.TrimSpace(parts[len(parts)-1])
	if expr == "" {
		return nil
	}
	if strings.Contains(expr, "ItemStack.mult") {
		if reqs, ok := parseItemStackMult(expr, varToBlockName, defs); ok {
			return reqs
		}
	}
	if strings.Contains(expr, "with(") {
		return parseWith(expr, itemsByVar)
	}
	return nil
}

func parseItemStackMult(expr string, varToBlockName map[string]string, defs []blockBuildDef) ([]buildReq, bool) {
	re := regexp.MustCompile(`ItemStack\.mult\s*\(\s*([A-Za-z0-9_]+)\.requirements\s*,\s*([^)]+)\)`)
	m := re.FindStringSubmatch(expr)
	if len(m) < 3 {
		return nil, false
	}
	varName := strings.TrimSpace(m[1])
	multExpr := strings.TrimSpace(m[2])
	blockName := varToBlockName[varName]
	if blockName == "" {
		return nil, false
	}
	base := findRequirementsByName(defs, blockName)
	if len(base) == 0 {
		return nil, false
	}
	mult := float32(1)
	if v, ok := evalExpr(multExpr); ok && v > 0 {
		mult = v
	}
	out := make([]buildReq, len(base))
	for i := range base {
		out[i] = buildReq{
			Item:   base[i].Item,
			Amount: int32(math.Round(float64(float32(base[i].Amount) * mult))),
		}
	}
	return out, true
}

func findRequirementsByName(defs []blockBuildDef, name string) []buildReq {
	for i := range defs {
		if defs[i].Block == name {
			return defs[i].Requirements
		}
	}
	return nil
}

func parseWith(expr string, itemsByVar map[string]itemDef) []buildReq {
	idx := strings.Index(expr, "with(")
	if idx < 0 {
		idx = strings.Index(expr, "ItemStack.with(")
		if idx < 0 {
			return nil
		}
		idx = strings.Index(expr[idx:], "with(")
		if idx < 0 {
			return nil
		}
		idx += strings.Index(expr, "with(")
	}
	start := idx + len("with(")
	end := findMatchingParen(expr, start-1)
	if end < 0 || end <= start {
		return nil
	}
	args := expr[start:end]
	parts := splitTopLevelArgs(args)
	if len(parts) == 0 {
		return nil
	}
	out := []buildReq{}
	for i := 0; i+1 < len(parts); i += 2 {
		itemExpr := strings.TrimSpace(parts[i])
		amountExpr := strings.TrimSpace(parts[i+1])
		itemName := parseItemName(itemExpr, itemsByVar)
		if itemName == "" {
			continue
		}
		amount := int32(0)
		if v, ok := evalExpr(amountExpr); ok {
			amount = int32(math.Round(float64(v)))
		} else if n, err := strconv.Atoi(strings.TrimSpace(amountExpr)); err == nil {
			amount = int32(n)
		}
		if amount <= 0 {
			continue
		}
		out = append(out, buildReq{Item: itemName, Amount: amount})
	}
	return out
}

func parseItemName(expr string, itemsByVar map[string]itemDef) string {
	expr = strings.TrimSpace(expr)
	if strings.HasPrefix(expr, "Items.") {
		name := strings.TrimPrefix(expr, "Items.")
		name = strings.TrimSpace(name)
		if def, ok := itemsByVar[name]; ok {
			return def.Name
		}
		return strings.ToLower(strings.ReplaceAll(name, "_", "-"))
	}
	return ""
}

func findCallArgs(body, name string) (string, bool) {
	idx := strings.Index(body, name+"(")
	if idx < 0 {
		return "", false
	}
	start := idx + len(name) + 1
	end := findMatchingParen(body, start-1)
	if end < 0 {
		return "", false
	}
	return body[start:end], true
}

func findMatchingParen(s string, start int) int {
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func splitTopLevelArgs(s string) []string {
	out := []string{}
	depth := 0
	last := 0
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
				part := strings.TrimSpace(s[last:i])
				out = append(out, part)
				last = i + 1
			}
		}
	}
	if last <= len(s) {
		part := strings.TrimSpace(s[last:])
		if part != "" {
			out = append(out, part)
		}
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
