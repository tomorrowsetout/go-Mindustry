package main

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"mdt-server/internal/oracle"
)

func main() {
	root, _ := filepath.Abs(".")
	scenario := oracle.Scenario{
		Name:                "official-go-one-tick",
		MapPath:             "assets/worlds/file.msav",
		VanillaProfilesPath: "data/vanilla/profiles.json",
		Ticks:               1,
		CaptureInitial:      true,
	}
	official, err := oracle.ReadTrace(filepath.Join(".tmp", "official-go-one-tick", "trace-official.json"))
	if err != nil {
		panic(err)
	}
	goTrace, err := oracle.CollectGoTrace(scenario, oracle.GoTraceOptions{WorkspaceRoot: root})
	if err != nil {
		panic(err)
	}
	diffs := oracle.CompareTraces(official, goTrace)
	fmt.Printf("diffs=%d\n", len(diffs))
	counts := map[string]int{}
	for _, diff := range diffs {
		counts[fieldName(diff.Path)]++
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		fmt.Printf("%s=%d\n", key, counts[key])
	}
	limit := 120
	if len(diffs) < limit {
		limit = len(diffs)
	}
	for i := 0; i < limit; i++ {
		fmt.Printf("%s: %s\n", diffs[i].Path, diffs[i].Message)
	}
}

func fieldName(path string) string {
	if idx := strings.LastIndex(path, "."); idx >= 0 {
		return path[idx+1:]
	}
	return path
}
