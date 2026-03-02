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
	out := flag.String("out", filepath.FromSlash("data/vanilla/profiles.json"), "output profiles file")
	flag.Parse()

	units, turrets, err := vanilla.GenerateProfiles(*repoRoot, *out)
	if err != nil {
		fmt.Fprintf(os.Stderr, "generate failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("generated profiles: units_by_name=%d turrets=%d out=%s\n", units, turrets, *out)
}
