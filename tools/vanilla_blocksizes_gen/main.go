package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

type blockSizeDef struct {
	Block string `json:"block"`
	Size  int    `json:"size"`
}

func main() {
	in := flag.String("in", "", "path to Blocks.java")
	out := flag.String("out", "data/vanilla/block_sizes.json", "output json path")
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
	fmt.Printf("block sizes: %d -> %s\n", len(defs), *out)
}

func filepathDir(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}

var blockStartRe = regexp.MustCompile(`new\s+[A-Za-z0-9_$.]+\s*\(\s*"([^"]+)"\s*\)\s*\{`)
var sizeRe = regexp.MustCompile(`(?m)\bsize\s*=\s*([^;]+);`)

func parseBlocks(src string) []blockSizeDef {
	out := []blockSizeDef{}
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
		if v, ok := evalExprFirst(body, sizeRe); ok {
			size := int(v)
			if size > 1 {
				out = append(out, blockSizeDef{Block: name, Size: size})
			}
		}
		seen[name] = true
	}
	addOverrides(out, seen)
	sort.Slice(out, func(i, j int) bool { return out[i].Block < out[j].Block })
	return out
}

func addOverrides(out []blockSizeDef, seen map[string]bool) []blockSizeDef {
	overrides := map[string]int{
		"payload-conveyor":            3,
		"payload-router":              3,
		"reinforced-payload-conveyor": 3,
		"reinforced-payload-router":   3,
		"payload-mass-driver":         3,
		"large-payload-mass-driver":   5,
		"payload-loader":              3,
		"payload-unloader":            3,
	}
	for name, size := range overrides {
		if seen[name] {
			continue
		}
		out = append(out, blockSizeDef{Block: name, Size: size})
		seen[name] = true
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
	if strings.ContainsAny(clean, "+-*/") {
		parts := strings.FieldsFunc(clean, func(r rune) bool {
			return r == '+' || r == '-' || r == '*' || r == '/'
		})
		if len(parts) == 1 {
			clean = parts[0]
		}
	}
	if v, err := strconv.ParseFloat(clean, 32); err == nil {
		return float32(v), true
	}
	return 0, false
}
