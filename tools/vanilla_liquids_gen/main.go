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

type liquidPropsDef struct {
	Liquid       string  `json:"liquid"`
	Coolant      bool    `json:"coolant,omitempty"`
	HeatCapacity float32 `json:"heatCapacity,omitempty"`
	Temperature  float32 `json:"temperature,omitempty"`
	Flammability float32 `json:"flammability,omitempty"`
	Gas          bool    `json:"gas,omitempty"`
}

func main() {
	in := flag.String("in", "", "path to Liquids.java")
	out := flag.String("out", "data/vanilla/liquid_props.json", "output json path")
	flag.Parse()
	if strings.TrimSpace(*in) == "" {
		fmt.Println("missing -in Liquids.java")
		os.Exit(2)
	}
	src, err := os.ReadFile(*in)
	if err != nil {
		fmt.Println("read Liquids.java:", err)
		os.Exit(1)
	}
	defs := parseLiquids(string(src))
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
	fmt.Printf("liquid props: %d -> %s\n", len(defs), *out)
}

func filepathDir(path string) string {
	idx := strings.LastIndexAny(path, `/\`)
	if idx <= 0 {
		return "."
	}
	return path[:idx]
}

var liquidDeclRe = regexp.MustCompile(`(?m)new\s+Liquid\("([^"]+)"[^{]*\{\{`)
var coolantRe = regexp.MustCompile(`(?m)\bcoolant\s*=\s*(true|false)\s*;`)
var heatCapRe = regexp.MustCompile(`(?m)\bheatCapacity\s*=\s*([^;]+);`)
var tempRe = regexp.MustCompile(`(?m)\btemperature\s*=\s*([^;]+);`)
var flamRe = regexp.MustCompile(`(?m)\bflammability\s*=\s*([^;]+);`)
var gasRe = regexp.MustCompile(`(?m)\bgas\s*=\s*(true|false)\s*;`)

func parseLiquids(src string) []liquidPropsDef {
	out := []liquidPropsDef{}
	seen := map[string]bool{}
	matches := liquidDeclRe.FindAllStringSubmatchIndex(src, -1)
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
		def := liquidPropsDef{Liquid: name}
		if m := coolantRe.FindStringSubmatch(body); len(m) > 1 {
			def.Coolant = strings.TrimSpace(m[1]) == "true"
		}
		if m := heatCapRe.FindStringSubmatch(body); len(m) > 1 {
			if v, ok := evalExpr(m[1]); ok {
				def.HeatCapacity = v
			}
		}
		if m := tempRe.FindStringSubmatch(body); len(m) > 1 {
			if v, ok := evalExpr(m[1]); ok {
				def.Temperature = v
			}
		}
		if m := flamRe.FindStringSubmatch(body); len(m) > 1 {
			if v, ok := evalExpr(m[1]); ok {
				def.Flammability = v
			}
		}
		if m := gasRe.FindStringSubmatch(body); len(m) > 1 {
			def.Gas = strings.TrimSpace(m[1]) == "true"
		}
		if def.Coolant || def.HeatCapacity != 0 || def.Temperature != 0 || def.Flammability != 0 || def.Gas {
			out = append(out, def)
		}
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
	v, err := strconv.ParseFloat(clean, 32)
	if err != nil {
		return 0, false
	}
	return float32(v), true
}
