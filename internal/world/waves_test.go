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
