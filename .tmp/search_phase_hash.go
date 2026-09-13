package main

import (
	"fmt"
	"math"
	"path/filepath"
	"sort"

	"mdt-server/internal/oracle"
	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/worldstream"
)

const phaseFabricID = 11

type center struct {
	x, y int
}

type candidate struct {
	c       center
	name    string
	edgeIdx int
	key     uint32
}

func main() {
	reg := protocol.NewContentRegistry()
	ids, err := vanilla.LoadContentIDs(filepath.Join("data", "vanilla", "content_ids.json"))
	if err != nil {
		panic(err)
	}
	vanilla.ApplyContentIDs(reg, ids)
	model, err := worldstream.LoadWorldModelFromMSAV(filepath.Join("assets", "worlds", "file.msav"), reg)
	if err != nil {
		panic(err)
	}
	official, err := oracle.ReadTrace(filepath.Join(".tmp", "official-go-one-tick", "trace-official.json"))
	if err != nil {
		panic(err)
	}

	cellCenter := map[[2]int]center{}
	nameByCenter := map[center]string{}
	teamByCenter := map[center]int{}
	coreCentersByTeam := map[int][]center{}
	for i := range model.Tiles {
		t := &model.Tiles[i]
		if t.Build == nil || t.Block == 0 {
			continue
		}
		c := center{int(t.Build.X), int(t.Build.Y)}
		name := model.BlockNames[int16(t.Build.Block)]
		nameByCenter[c] = name
		teamByCenter[c] = int(t.Team)
		if isCore(name) {
			if !containsCenter(coreCentersByTeam[int(t.Team)], c) {
				coreCentersByTeam[int(t.Team)] = append(coreCentersByTeam[int(t.Team)], c)
			}
		}
		low, high := footprintRange(blockSizeByName(name))
		for y := c.y + low; y <= c.y+high; y++ {
			for x := c.x + low; x <= c.x+high; x++ {
				cellCenter[[2]int{x, y}] = c
			}
		}
	}
	initial := traceCenterAmounts(official, 0, cellCenter)
	want := traceCenterAmounts(official, 1, cellCenter)
	sources := make([]center, 0)
	neighbors := map[center][]candidate{}
	for i := range model.Tiles {
		t := &model.Tiles[i]
		if t.Build == nil || model.BlockNames[int16(t.Build.Block)] != "item-source" || len(t.Build.MapSyncTail) == 0 || int(t.Build.MapSyncTail[len(t.Build.MapSyncTail)-1]) != phaseFabricID {
			continue
		}
		src := center{t.X, t.Y}
		sources = append(sources, src)
		seen := map[center]struct{}{}
		for edgeIdx, off := range edgeOffsets(1) {
			c, ok := cellCenter[[2]int{t.X + off[0], t.Y + off[1]}]
			if !ok || c == src {
				continue
			}
			if _, dup := seen[c]; dup {
				continue
			}
			seen[c] = struct{}{}
			neighbors[src] = append(neighbors[src], candidate{c: c, name: nameByCenter[c], edgeIdx: edgeIdx})
		}
	}
	sort.Slice(sources, func(i, j int) bool {
		if sources[i].y != sources[j].y {
			return sources[i].y < sources[j].y
		}
		return sources[i].x < sources[j].x
	})

	bestScore := 1 << 30
	bestMode, bestSeed := 0, uint32(0)
	perfect := 0
	for mode := 0; mode < 6; mode++ {
		for seed := uint32(1); seed <= 200000; seed++ {
			got := simulate(initial, sources, neighbors, nameByCenter, teamByCenter, coreCentersByTeam, mode, seed)
			score := scoreAmounts(got, want)
			if score == 0 {
				fmt.Printf("perfect mode=%d seed=%d\n", mode, seed)
				perfect++
				if perfect >= 40 {
					fmt.Printf("stopped after %d perfect seeds\n", perfect)
					return
				}
			}
			if score < bestScore {
				bestScore, bestMode, bestSeed = score, mode, seed
				fmt.Printf("best score=%d mode=%d seed=%d\n", bestScore, bestMode, bestSeed)
				printDiff(got, want, nameByCenter)
			}
		}
	}
	fmt.Printf("final best score=%d mode=%d seed=%d\n", bestScore, bestMode, bestSeed)
}

func simulate(initial map[center]int, sources []center, neighbors map[center][]candidate, names map[center]string, teams map[center]int, cores map[int][]center, mode int, seed uint32) map[center]int {
	out := map[center]int{}
	for c, v := range initial {
		out[c] = v
	}
	for _, src := range sources {
		list := append([]candidate(nil), neighbors[src]...)
		for i := range list {
			list[i].key = proxyKey(list[i].c, mode, seed)
		}
		sort.SliceStable(list, func(i, j int) bool {
			bi, bj := list[i].key&63, list[j].key&63
			if bi != bj {
				return bi < bj
			}
			return list[i].edgeIdx < list[j].edgeIdx
		})
		for _, cand := range list {
			if acceptsPhase(cand.name) {
				if isCore(names[cand.c]) {
					for _, core := range cores[teams[cand.c]] {
						out[core]++
					}
				} else {
					out[cand.c]++
				}
				break
			}
		}
	}
	return out
}

func containsCenter(list []center, c center) bool {
	for _, existing := range list {
		if existing == c {
			return true
		}
	}
	return false
}

func scoreAmounts(got, want map[center]int) int {
	keys := map[center]struct{}{}
	for c := range got {
		keys[c] = struct{}{}
	}
	for c := range want {
		keys[c] = struct{}{}
	}
	score := 0
	for c := range keys {
		if got[c] != want[c] {
			d := got[c] - want[c]
			if d < 0 {
				d = -d
			}
			score += d
		}
	}
	return score
}

func printDiff(got, want map[center]int, names map[center]string) {
	keys := make([]center, 0)
	seen := map[center]struct{}{}
	for c := range got {
		seen[c] = struct{}{}
	}
	for c := range want {
		seen[c] = struct{}{}
	}
	for c := range seen {
		if got[c] != want[c] {
			keys = append(keys, c)
		}
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].y != keys[j].y {
			return keys[i].y < keys[j].y
		}
		return keys[i].x < keys[j].x
	})
	for _, c := range keys {
		fmt.Printf("  %d,%d %-20s got=%d want=%d\n", c.x, c.y, names[c], got[c], want[c])
	}
}

func proxyKey(c center, mode int, seed uint32) uint32 {
	pos := uint32(c.y*500 + c.x)
	xy := uint32(c.x)<<16 | uint32(c.y)
	switch mode {
	case 0:
		return mix32(pos ^ seed)
	case 1:
		return mix32(xy ^ seed)
	case 2:
		return mix32(pos*seed + 0x9e3779b9)
	case 3:
		return mix32(xy*seed + 0x85ebca6b)
	case 4:
		return mix32((uint32(c.x)*73856093 ^ uint32(c.y)*19349663) + seed)
	default:
		return mix32((uint32(c.x)*83492791 + uint32(c.y)*2654435761) ^ seed)
	}
}

func mix32(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return x
}

func acceptsPhase(name string) bool {
	switch name {
	case "core-shard", "core-foundation", "core-nucleus", "core-bastion", "core-citadel", "core-acropolis",
		"mender", "mend-projector", "overdrive-projector", "overdrive-dome", "force-projector", "regen-projector":
		return true
	default:
		return false
	}
}

func isCore(name string) bool {
	switch name {
	case "core-shard", "core-foundation", "core-nucleus", "core-bastion", "core-citadel", "core-acropolis":
		return true
	default:
		return false
	}
}

func traceCenterAmounts(trace oracle.Trace, tick int, cellCenter map[[2]int]center) map[center]int {
	out := map[center]int{}
	for _, t := range trace.Ticks[tick].Tiles {
		amt := amount(t, phaseFabricID)
		if amt <= 0 {
			continue
		}
		c, ok := cellCenter[[2]int{t.X, t.Y}]
		if !ok {
			c = center{t.X, t.Y}
		}
		if _, exists := out[c]; !exists {
			out[c] = amt
		}
	}
	return out
}

func amount(t oracle.TileState, id int) int {
	for _, s := range t.Items {
		if s.ID == id {
			return int(s.Amount)
		}
	}
	return 0
}

func edgeOffsets(size int) [][2]int {
	low, high := footprintRange(size)
	out := make([][2]int, 0, size*4)
	for x := low; x <= high; x++ {
		out = append(out, [2]int{x, low - 1})
		out = append(out, [2]int{x, high + 1})
	}
	for y := low; y <= high; y++ {
		out = append(out, [2]int{low - 1, y})
		out = append(out, [2]int{high + 1, y})
	}
	sort.Slice(out, func(i, j int) bool {
		ai := math.Atan2(float64(out[i][1]), float64(out[i][0]))
		aj := math.Atan2(float64(out[j][1]), float64(out[j][0]))
		if ai < 0 {
			ai += 2 * math.Pi
		}
		if aj < 0 {
			aj += 2 * math.Pi
		}
		return ai < aj
	})
	return out
}

func footprintRange(size int) (int, int) {
	if size <= 1 {
		return 0, 0
	}
	return -((size - 1) / 2), size / 2
}

func blockSizeByName(name string) int {
	switch name {
	case "mend-projector", "overdrive-projector", "segment":
		return 2
	case "force-projector", "overdrive-dome":
		return 3
	case "core-foundation":
		return 4
	case "core-nucleus":
		return 5
	default:
		return 1
	}
}
