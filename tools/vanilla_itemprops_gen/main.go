package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
)

type itemPropsDef struct {
	Item        string  `json:"item"`
	Hardness    float32 `json:"hardness,omitempty"`
	LowPriority bool    `json:"lowPriority,omitempty"`
}

func main() {
	in := flag.String("in", "", "path to Items.java")
	out := flag.String("out", "data/vanilla/item_props.json", "output json path")
	flag.Parse()
	if strings.TrimSpace(*in) == "" {
		fmt.Println("missing -in Items.java")
		os.Exit(2)
	}
	src, err := os.ReadFile(*in)
	if err != nil {
		fmt.Println("read Items.java:", err)
		os.Exit(1)
	}
	defs := parseItems(string(src))
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
	fmt.Printf("item props: %d -> %s\n", len(defs), *out)
}

func filepathDir(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}

var itemDeclRe = regexp.MustCompile(`(?m)new\s+Item\("([^"]+)"[^{]*\{\{`)
var hardnessRe = regexp.MustCompile(`(?m)\bhardness\s*=\s*([^;]+);`)
var lowPriorityRe = regexp.MustCompile(`(?m)\blowPriority\s*=\s*(true|false)\s*;`)

func parseItems(src string) []itemPropsDef {
	out := []itemPropsDef{}
	seen := map[string]bool{}
	matches := itemDeclRe.FindAllStringSubmatchIndex(src, -1)
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
		body, ok := extractBody(src, start)
		if !ok {
			continue
		}
		def := itemPropsDef{Item: name}
		if m := hardnessRe.FindStringSubmatch(body); len(m) > 1 {
			if v, ok := evalExpr(m[1]); ok {
				def.Hardness = v
			}
		}
		if m := lowPriorityRe.FindStringSubmatch(body); len(m) > 1 {
			def.LowPriority = strings.TrimSpace(m[1]) == "true"
		}
		if def.Hardness != 0 || def.LowPriority {
			out = append(out, def)
		}
		seen[name] = true
	}
	for name, hardness := range map[string]float32{
		"metaglass":     0,
		"graphite":      0,
		"silicon":       0,
		"plastanium":    0,
		"phase-fabric":  0,
		"surge-alloy":   0,
		"beryllium":     0,
		"oxide":         0,
		"carbide":       0,
		"fissile-matter": 0,
		"dormant-cyst":   0,
	} {
		if seen[name] {
			continue
		}
		out = append(out, itemPropsDef{Item: name, Hardness: hardness})
		seen[name] = true
	}
	return out
}

func extractBody(src string, start int) (string, bool) {
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

func evalExpr(expr string) (float32, bool) {
	clean := strings.ReplaceAll(expr, "f", "")
	clean = strings.ReplaceAll(clean, "F", "")
	clean = strings.TrimSpace(clean)
	if clean == "" {
		return 0, false
	}
	return parseFloat(clean)
}

func parseFloat(s string) (float32, bool) {
	v, err := strconv.ParseFloat(s, 32)
	if err != nil {
		return 0, false
	}
	return float32(v), true
}
