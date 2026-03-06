package main

import (
    "flag"
    "fmt"
    "os"
    "path/filepath"
    "mdt-server/internal/vanilla"
)

func main(){
    repo := flag.String("repo", "", "Mindustry repo root")
    out := flag.String("out", "internal/vanilla/unit_types_gen.go", "output Go file")
    flag.Parse()
    if *repo == "" {
        fmt.Fprintln(os.Stderr, "-repo is required")
        os.Exit(1)
    }
    unitPath := filepath.Join(*repo, "core", "src", "mindustry", "content", "UnitTypes.java")
    if _, err := os.Stat(unitPath); err != nil {
        unitPath = filepath.Join(*repo, "..", "core", "src", "mindustry", "content", "UnitTypes.java")
    }
    if err := vanilla.GenerateUnitTypes(unitPath, *out); err != nil {
        fmt.Fprintln(os.Stderr, err)
        os.Exit(1)
    }
}
