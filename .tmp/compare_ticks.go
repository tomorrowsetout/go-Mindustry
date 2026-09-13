package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mdt-server/internal/oracle"
)

func main() {
	root, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	root = filepath.Clean(root)
	scenario := oracle.Scenario{
		Name:                "official-go-one-tick",
		MapPath:             "assets/worlds/file.msav",
		VanillaProfilesPath: "data/vanilla/profiles.json",
		Ticks:               1,
		CaptureInitial:      true,
	}
	official, err := oracle.CollectOfficialTrace(scenario, oracle.OfficialTraceOptions{
		WorkspaceRoot: root,
		WorkspaceDir:  filepath.Join(".tmp", "official-go-one-tick"),
	})
	if err != nil {
		panic(err)
	}
	goTrace, err := oracle.CollectGoTrace(scenario, oracle.GoTraceOptions{WorkspaceRoot: root})
	if err != nil {
		panic(err)
	}
	diffs := oracle.CompareTraces(official, goTrace)
	fmt.Printf("diffs=%d\n", len(diffs))
	limit := 80
	if limit > len(diffs) {
		limit = len(diffs)
	}
	var b strings.Builder
	for i := 0; i < limit; i++ {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(diffs[i].Path)
		b.WriteString(": ")
		b.WriteString(diffs[i].Message)
	}
	if len(diffs) > limit {
		b.WriteString("\n... ")
		b.WriteString(strconv.Itoa(len(diffs) - limit))
		b.WriteString(" more")
	}
	fmt.Println(b.String())
}
