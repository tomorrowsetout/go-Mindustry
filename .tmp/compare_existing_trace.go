package main

import (
	"fmt"
	"path/filepath"
	"sort"

	"mdt-server/internal/oracle"
)

func amount(t oracle.TileState, id int) float64 {
	for _, s := range t.Items {
		if s.ID == id {
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
	diffs := oracle.CompareTraces(official, goTrace)
	fmt.Printf("diffs=%d\n", len(diffs))
	off := indexTiles(official, 1)
	got := indexTiles(goTrace, 1)
	keys := make([]string, 0)
	for k, t := range off {
		if amount(t, 11) > 0 || amount(got[k], 11) > 0 {
			keys = append(keys, k)
		}
	}
	for k, t := range got {
		if _, ok := off[k]; !ok && amount(t, 11) > 0 {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	for _, k := range keys {
		o := amount(off[k], 11)
		g := amount(got[k], 11)
		if o != g {
			fmt.Printf("%s block off=%d go=%d phase official=%.0f go=%.0f\n", k, off[k].BlockID, got[k].BlockID, o, g)
		}
	}
}
