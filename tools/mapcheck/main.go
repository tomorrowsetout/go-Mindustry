package main

import (
	"flag"
	"fmt"
	"os"
	"sort"
	"strings"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/worldstream"
)

type coreInfo struct {
	x, y      int
	name      string
	tileTeam  byte
	buildTeam byte
	hasBuild  bool
}

func main() {
	mapPath := flag.String("map", "assets/worlds/file.msav", "msav map path")
	useRegistry := flag.Bool("use-registry", false, "load vanilla content_ids.json registry for decode")
	contentIDsPath := flag.String("content-ids", "data/vanilla/content_ids.json", "content_ids.json path (used with -use-registry)")
	flag.Parse()

	if _, err := os.Stat(*mapPath); err != nil {
		fmt.Printf("map not found: %s (%v)\n", *mapPath, err)
		os.Exit(2)
	}

	fmt.Printf("map=%s\n", *mapPath)

	rawCores, rawErr := worldstream.FindCoreTilesFromMSAV(*mapPath)
	if rawErr != nil {
		fmt.Printf("raw-core-scan error: %v\n", rawErr)
		os.Exit(1)
	}
	fmt.Printf("raw-core-scan count=%d\n", len(rawCores))
	printCoreSamples("raw", rawCores)

	var reg *protocol.ContentRegistry
	if *useRegistry {
		ids, err := vanilla.LoadContentIDs(*contentIDsPath)
		if err != nil {
			fmt.Printf("load content ids failed: %v\n", err)
			os.Exit(2)
		}
		reg = protocol.NewContentRegistry()
		applied := vanilla.ApplyContentIDs(reg, ids)
		fmt.Printf("registry enabled: contentIDs=%s entries=%d\n", *contentIDsPath, applied)
	}

	model, err := worldstream.LoadWorldModelFromMSAV(*mapPath, reg)
	if err != nil {
		fmt.Printf("decode error: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("model size=%dx%d blockNames=%d unitNames=%d entities=%d\n",
		model.Width, model.Height, len(model.BlockNames), len(model.UnitNames), len(model.Entities))

	coreNames := map[string]struct{}{
		"core-shard":      {},
		"core-foundation": {},
		"core-nucleus":    {},
		"core-bastion":    {},
		"core-citadel":    {},
		"core-acropolis":  {},
	}
	cores := make([]coreInfo, 0, 32)
	byName := map[string]int{}
	teamTiles := map[byte]int{}
	teamBuilds := map[byte]int{}
	tilesWithBuild := 0

	for i := range model.Tiles {
		t := &model.Tiles[i]
		if t.Build != nil {
			tilesWithBuild++
			teamBuilds[byte(t.Build.Team)]++
		}
		if t.Team != 0 {
			teamTiles[byte(t.Team)]++
		}
		if t.Block <= 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(model.BlockNames[int16(t.Block)]))
		if _, ok := coreNames[name]; !ok {
			continue
		}
		info := coreInfo{
			x:        t.X,
			y:        t.Y,
			name:     name,
			tileTeam: byte(t.Team),
			hasBuild: t.Build != nil,
		}
		if t.Build != nil {
			info.buildTeam = byte(t.Build.Team)
		}
		cores = append(cores, info)
		byName[name]++
	}

	fmt.Printf("decoded-core-tiles count=%d\n", len(cores))
	if len(byName) > 0 {
		names := make([]string, 0, len(byName))
		for k := range byName {
			names = append(names, k)
		}
		sort.Strings(names)
		for _, n := range names {
			fmt.Printf("core-name %-16s count=%d\n", n, byName[n])
		}
	}
	fmt.Printf("decoded-buildings total=%d\n", tilesWithBuild)
	printTeamCounts("tile-team", teamTiles)
	printTeamCounts("build-team", teamBuilds)
	printDecodedCoreSamples(cores)

	if len(rawCores) == 0 && len(cores) == 0 {
		fmt.Println("result=PASS no cores found in both scanners")
		return
	}
	if len(rawCores) == 0 || len(cores) == 0 {
		fmt.Println("result=FAIL scanner mismatch: one side has cores, the other side has none")
		os.Exit(1)
	}
	ratio := float64(len(cores)) / float64(len(rawCores))
	if ratio > 30.0 {
		fmt.Printf("result=FAIL suspicious core tile ratio decoded/raw=%.2f (%d/%d)\n", ratio, len(cores), len(rawCores))
		os.Exit(1)
	}
	if len(teamTiles) == 0 && len(teamBuilds) == 0 {
		fmt.Printf("result=FAIL decoded map has cores but no team/build data (raw=%d decoded=%d)\n", len(rawCores), len(cores))
		os.Exit(1)
	}
	fmt.Printf("result=PASS decoded map looks consistent (raw=%d decoded=%d ratio=%.2f)\n", len(rawCores), len(cores), ratio)
}

func printCoreSamples(label string, cores []protocol.Point2) {
	limit := 8
	if len(cores) < limit {
		limit = len(cores)
	}
	for i := 0; i < limit; i++ {
		c := cores[i]
		fmt.Printf("%s-core[%d]=(%d,%d)\n", label, i, c.X, c.Y)
	}
}

func printTeamCounts(label string, counts map[byte]int) {
	if len(counts) == 0 {
		fmt.Printf("%s counts: none\n", label)
		return
	}
	keys := make([]int, 0, len(counts))
	for k := range counts {
		keys = append(keys, int(k))
	}
	sort.Ints(keys)
	for _, k := range keys {
		fmt.Printf("%s=%d count=%d\n", label, k, counts[byte(k)])
	}
}

func printDecodedCoreSamples(cores []coreInfo) {
	limit := 8
	if len(cores) < limit {
		limit = len(cores)
	}
	for i := 0; i < limit; i++ {
		c := cores[i]
		fmt.Printf("decoded-core[%d]=(%d,%d) name=%s tileTeam=%d hasBuild=%v buildTeam=%d\n",
			i, c.x, c.y, c.name, c.tileTeam, c.hasBuild, c.buildTeam)
	}
}
