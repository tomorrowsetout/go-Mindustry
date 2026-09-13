package world

import (
	"testing"
)

func TestWaveManagerGeneratePlan(t *testing.T) {
	cfg := &WaveConfig{
		InitialSpacingSec:  30,
		BaseSpacingSec:     90,
		EnemyBaseCount:     10,
		EnemyGrowthFactor:  1.2,
		MaxEnemiesPerGroup: 20,
		EnemyTypes:         []int16{0, 1, 2},
	}

	wm := NewWaveManager(cfg)

	// 测试第1波
	plan1 := wm.GeneratePlan(1)
	if plan1.WaveNumber != 1 {
		t.Fatalf("expected wave 1, got %d", plan1.WaveNumber)
	}
	if plan1.EnemyCount < 10 {
		t.Fatalf("expected at least 10 enemies, got %d", plan1.EnemyCount)
	}

	// 测试第10波
	plan10 := wm.GeneratePlan(10)
	if plan10.WaveNumber != 10 {
		t.Fatalf("expected wave 10, got %d", plan10.WaveNumber)
	}
	// 应该比第1波多
	if plan10.EnemyCount <= plan1.EnemyCount {
		t.Fatalf("expected enemy count to grow, wave1=%d wave10=%d", plan1.EnemyCount, plan10.EnemyCount)
	}
}

func TestWaveManagerUpdateTick(t *testing.T) {
	cfg := &WaveConfig{
		InitialSpacingSec:  5,
		BaseSpacingSec:     10,
		EnemyBaseCount:     10,
		EnemyGrowthFactor:  1.0,
		MaxEnemiesPerGroup: 20,
		EnemyTypes:         []int16{0, 1, 2},
	}

	wm := NewWaveManager(cfg)

	// 初始应在倒计时中
	stats := wm.Stats()
	if stats.WaveCount != 0 {
		t.Fatalf("expected wave count 0 initially, got %d", stats.WaveCount)
	}

	// 模拟倒计时通过
	for i := 0; i < 10; i++ {
		trigger, waveNum := wm.UpdateTick(1.0)
		if trigger {
			if waveNum != 1 {
				t.Fatalf("expected wave 1 on trigger, got %d", waveNum)
			}
			break
		}
	}

	// 验证波次已触发
	stats = wm.Stats()
	if stats.WaveCount != 1 {
		t.Fatalf("expected wave count 1 after trigger, got %d", stats.WaveCount)
	}
}

func TestWaveManagerStats(t *testing.T) {
	cfg := &WaveConfig{
		InitialSpacingSec:  30,
		BaseSpacingSec:     90,
		EnemyBaseCount:     10,
		EnemyGrowthFactor:  1.0,
		MaxEnemiesPerGroup: 20,
		EnemyTypes:         []int16{0},
	}

	wm := NewWaveManager(cfg)
	stats := wm.Stats()

	if stats.WaveCount != 0 {
		t.Fatalf("expected initial wave count 0, got %d", stats.WaveCount)
	}
	if stats.NextWaveAt != 30.0 {
		t.Fatalf("expected initial nextWaveAt 30, got %f", stats.NextWaveAt)
	}
}

func TestWaveSpawnOverlayUsesTileWorldOrigin(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.Tiles[9*model.Width+7].Overlay = 1
	w.SetModel(model)

	ground := w.waveGroundSpawnPositionsLocked(-1, nil)
	if len(ground) != 1 {
		t.Fatalf("expected one ground spawn, got %d", len(ground))
	}
	if ground[0].X != 56 || ground[0].Y != 72 {
		t.Fatalf("expected ground spawn at tile world origin (56,72), got (%f,%f)", ground[0].X, ground[0].Y)
	}

	air := w.waveFlyerSpawnPositionsLocked(-1, &Rules{AirUseSpawns: true})
	if len(air) != 1 {
		t.Fatalf("expected one air-use-spawns position, got %d", len(air))
	}
	if air[0].X != 56 || air[0].Y != 72 {
		t.Fatalf("expected air-use-spawns position at tile world origin (56,72), got (%f,%f)", air[0].X, air[0].Y)
	}
}

func TestWaveDropZoneShockwaveUsesSpawnTileWorldOrigin(t *testing.T) {
	w := New(Config{TPS: 60})
	model := NewWorldModel(20, 20)
	model.BlockNames = map[int16]string{
		600: "test-wall",
	}
	model.Tiles[10*model.Width+10].Overlay = 1
	w.SetModel(model)
	placeTestBuilding(t, w, 7, 10, 600, 1, 0)

	w.applyWaveDropZoneShockwavesLocked(2, &Rules{DropZoneRadius: 24})

	tile, err := model.TileAt(7, 10)
	if err != nil {
		t.Fatalf("expected boundary tile to exist: %v", err)
	}
	if tile.Block != 0 || tile.Build != nil {
		t.Fatalf("expected boundary building to be destroyed using spawn tile origin, block=%d build=%v", tile.Block, tile.Build)
	}
}
