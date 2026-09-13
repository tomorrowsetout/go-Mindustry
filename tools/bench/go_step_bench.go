package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/sim"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

type result struct {
	Producer       string  `json:"producer"`
	MapPath        string  `json:"mapPath"`
	ProfilesPath   string  `json:"profilesPath,omitempty"`
	Warmup         int     `json:"warmup"`
	Ticks          int     `json:"ticks"`
	DeltaMS        float64 `json:"deltaMs"`
	Scheduler      bool    `json:"scheduler"`
	ElapsedNS      int64   `json:"elapsedNs"`
	NSPerTick      float64 `json:"nsPerTick"`
	TicksPerSecond float64 `json:"ticksPerSecond"`
	Units          int     `json:"units"`
	Bullets        int     `json:"bullets"`
	ActiveTiles    int     `json:"activeTiles"`
}

func main() {
	mapPath := flag.String("map", filepath.Join("assets", "worlds", "file.msav"), "MSAV map/save path")
	profilesPath := flag.String("profiles", filepath.Join("data", "vanilla", "profiles.json"), "vanilla profiles path")
	warmup := flag.Int("warmup", 20, "warmup ticks")
	ticks := flag.Int("ticks", 300, "measured ticks")
	deltaMS := flag.Float64("delta-ms", 1000.0/60.0, "delta per tick in milliseconds")
	scheduler := flag.Bool("scheduler", false, "enable Go scheduler-backed world workers")
	flag.Parse()

	if *ticks <= 0 {
		log.Fatal("-ticks must be > 0")
	}

	reg := protocol.NewContentRegistry()
	if *profilesPath != "" {
		ids, err := vanilla.LoadContentIDs(filepath.Join(filepath.Dir(*profilesPath), "content_ids.json"))
		if err != nil {
			log.Fatalf("load content ids: %v", err)
		}
		vanilla.ApplyContentIDs(reg, ids)
	}
	model, err := worldstream.LoadWorldModelFromMSAV(*mapPath, reg)
	if err != nil {
		log.Fatalf("load world: %v", err)
	}
	w := world.New(world.Config{TPS: int(msToTPS(*deltaMS))})
	if *profilesPath != "" {
		if err := w.LoadVanillaProfiles(*profilesPath); err != nil {
			log.Fatalf("load vanilla profiles: %v", err)
		}
	}
	var engine *sim.Engine
	if *scheduler {
		cores := runtime.NumCPU()
		workers := cores - 1
		if workers < 1 {
			workers = 1
		}
		engine = sim.NewEngine(sim.Config{TPS: int(msToTPS(*deltaMS)), Cores: cores, Partitions: workers})
		w.SetScheduler(engine)
		defer engine.Stop()
	}
	w.SetModel(model)

	delta := time.Duration(*deltaMS * float64(time.Millisecond))
	eventBuf := make([]world.EntityEvent, 0, 1024)
	for i := 0; i < *warmup; i++ {
		w.Step(delta)
		eventBuf = w.DrainEntityEventsInto(eventBuf)
	}
	start := time.Now()
	for i := 0; i < *ticks; i++ {
		w.Step(delta)
		eventBuf = w.DrainEntityEventsInto(eventBuf)
	}
	elapsed := time.Since(start)
	snapshot := w.CloneModel()
	active := 0
	if snapshot != nil {
		for i := range snapshot.Tiles {
			if snapshot.Tiles[i].Block != 0 || snapshot.Tiles[i].Build != nil {
				active++
			}
		}
	}
	nsPerTick := float64(elapsed.Nanoseconds()) / float64(*ticks)
	out := result{
		Producer:       "go-world",
		MapPath:        *mapPath,
		ProfilesPath:   *profilesPath,
		Warmup:         *warmup,
		Ticks:          *ticks,
		DeltaMS:        *deltaMS,
		Scheduler:      *scheduler,
		ElapsedNS:      elapsed.Nanoseconds(),
		NSPerTick:      nsPerTick,
		TicksPerSecond: 1_000_000_000.0 / nsPerTick,
		Units:          len(snapshot.Entities),
		Bullets:        0,
		ActiveTiles:    active,
	}
	raw, err := json.Marshal(out)
	if err != nil {
		log.Fatalf("marshal result: %v", err)
	}
	fmt.Println(string(raw))
}

func msToTPS(ms float64) float64 {
	if ms <= 0 {
		return 60
	}
	return 1000.0 / ms
}
