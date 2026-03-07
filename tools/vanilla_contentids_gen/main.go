package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"mdt-server/internal/vanilla"
)

func main() {
	repoRoot := flag.String("repo", ".", "Mindustry repo root (contains core/src/mindustry/content)")
	out := flag.String("out", filepath.FromSlash("data/vanilla/content_ids.json"), "output content IDs json")
	flag.Parse()

	ids, err := vanilla.GenerateContentIDs(*repoRoot, *out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate content ids failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf(
		"generated content ids: blocks=%d units=%d items=%d liquids=%d statuses=%d weathers=%d bullets=%d effects=%d sounds=%d teams=%d commands=%d stances=%d logic(blocks=%d units=%d items=%d liquids=%d) out=%s\n",
		len(ids.Blocks), len(ids.Units), len(ids.Items), len(ids.Liquids), len(ids.Statuses), len(ids.Weathers), len(ids.Bullets),
		len(ids.Effects), len(ids.Sounds), len(ids.Teams), len(ids.Commands), len(ids.Stances),
		len(ids.LogicBlocks), len(ids.LogicUnits), len(ids.LogicItems), len(ids.LogicLiquids),
		*out,
	)
}
