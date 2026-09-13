package main

import (
	"fmt"
	"path/filepath"

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
	points := [][2]int{
		{316, 484}, {316, 488}, {315, 486}, {316, 486},
		{380, 482}, {383, 482}, {383, 485}, {383, 487}, {386, 486}, {387, 487}, {377, 490}, {380, 490},
	}
	for _, pt := range points {
		fmt.Printf("point %d,%d\n", pt[0], pt[1])
		for _, tick := range []int{0, 1} {
			fmt.Printf("  tick %d official: %s\n", tick, tileSummary(official, tick, pt[0], pt[1]))
			fmt.Printf("  tick %d go      : %s\n", tick, tileSummary(goTrace, tick, pt[0], pt[1]))
		}
	}
}

func tileSummary(trace oracle.Trace, tick, x, y int) string {
	if tick < 0 || tick >= len(trace.Ticks) {
		return "<no tick>"
	}
	for _, tile := range trace.Ticks[tick].Tiles {
		if tile.X == x && tile.Y == y {
			return fmt.Sprintf("block=%d team=%d rot=%d hp=%.2f reload=%.4f items=%v liquids=%v", tile.BlockID, tile.TeamID, tile.Rotation, tile.BuildHealth, tile.Reload, tile.Items, tile.Liquids)
		}
	}
	return "<missing>"
}
