package world_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"mdt-server/internal/sim"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

func BenchmarkWorldStepCurrentMap(b *testing.B) {
	benchmarkWorldStepCurrentMap(b, false)
}

func BenchmarkWorldStepCurrentMapScheduler(b *testing.B) {
	benchmarkWorldStepCurrentMap(b, true)
}

func BenchmarkWorldStepOfficialCompareMap(b *testing.B) {
	benchmarkWorldStepMSAV(b, filepath.Join("assets", "worlds", "file.msav"), 60, 16*time.Millisecond, false)
}

func BenchmarkWorldStepOfficialCompareMapScheduler(b *testing.B) {
	benchmarkWorldStepMSAV(b, filepath.Join("assets", "worlds", "file.msav"), 60, 16*time.Millisecond, true)
}

func benchmarkWorldStepCurrentMap(b *testing.B, scheduler bool) {
	benchmarkWorldStepMSAV(b, filepath.Join("assets", "worlds", "22908.msav"), 120, time.Second/120, scheduler)
}

func benchmarkWorldStepMSAV(b *testing.B, relMapPath string, tps int, delta time.Duration, scheduler bool) {
	root := filepath.Join("..", "..")
	mapPath := filepath.Join(root, relMapPath)
	if _, err := os.Stat(mapPath); err != nil {
		b.Skipf("current map not available: %v", err)
	}
	wld := world.New(world.Config{TPS: tps})
	profilesPath := filepath.Join(root, "data", "vanilla", "profiles.json")
	if _, err := os.Stat(profilesPath); err == nil {
		if err := wld.LoadVanillaProfiles(profilesPath); err != nil {
			b.Fatalf("load vanilla profiles: %v", err)
		}
	}
	if scheduler {
		prevProcs := runtime.GOMAXPROCS(0)
		engine := sim.NewEngine(sim.Config{TPS: tps, Cores: 6, Partitions: 4})
		wld.SetScheduler(engine)
		b.Cleanup(func() {
			wld.SetScheduler(nil)
			runtime.GOMAXPROCS(prevProcs)
		})
	}
	model, err := worldstream.LoadWorldModelFromMSAV(mapPath, nil)
	if err != nil {
		b.Fatalf("load map: %v", err)
	}
	wld.SetModel(model)
	eventBuf := make([]world.EntityEvent, 0, 1024)
	for i := 0; i < 20; i++ {
		wld.Step(delta)
		eventBuf = wld.DrainEntityEventsInto(eventBuf)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		wld.Step(delta)
		eventBuf = wld.DrainEntityEventsInto(eventBuf)
	}
}
