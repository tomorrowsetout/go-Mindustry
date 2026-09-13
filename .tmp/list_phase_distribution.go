package main

import (
	"fmt"
	"path/filepath"
	"sort"

	"mdt-server/internal/oracle"
)

func phaseAmount(t oracle.TileState) float64 {
	for _, s := range t.Items {
		if s.ID == 11 {
			return s.Amount
		}
	}
	return 0
}

func indexTiles(trace oracle.Trace, tick int) map[string]oracle.TileState {
	out := map[string]oracle.TileState{}
	for _, t := range trace.Ticks[tick].Tiles {
		out[fmt.Sprintf("%d,%d", t.X, t.Y)] = t
	}
	return out
}

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
	off := indexTiles(official, 1)
	got := indexTiles(goTrace, 1)
	keys := make([]string, 0)
	seen := map[string]struct{}{}
	for k, t := range off {
		if phaseAmount(t) > 0 {
			keys = append(keys, k)
			seen[k] = struct{}{}
		}
	}
	for k, t := range got {
		if phaseAmount(t) > 0 {
			if _, ok := seen[k]; !ok {
				keys = append(keys, k)
			}
		}
	}
	sort.Strings(keys)
	var offTotal, goTotal float64
	for _, k := range keys {
		o := off[k]
		g := got[k]
		oa := phaseAmount(o)
		ga := phaseAmount(g)
		offTotal += oa
		goTotal += ga
		if oa == ga {
			continue
		}
		fmt.Printf("%s block off=%d go=%d phase off=%.0f go=%.0f\n", k, o.BlockID, g.BlockID, oa, ga)
	}
	fmt.Printf("phase totals off=%.0f go=%.0f\n", offTotal, goTotal)
}
