package world

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"sync"
	"time"
)

// WavePlan 波次计划
type WavePlan struct {
	WaveNumber     int32   `json:"waveNumber"`
	EnemyCount     int32   `json:"enemyCount"`
	GroupSize      int32   `json:"groupSize"`
	GroupCount     int32   `json:"groupCount"`
	GroupSpacing   float32 `json:"groupSpacing"`
	EnemyTypePrior []int16 `json:"enemyTypePrior"`
}

// WaveConfig 波次配置
type WaveConfig struct {
	InitialSpacingSec  float32 `json:"initialWaveSpacing"`
	BaseSpacingSec     float32 `json:"waveSpacing"`
	EnemyBaseCount     int32   `json:"enemyBaseCount"`
	EnemyGrowthFactor  float32 `json:"enemyGrowthFactor"`
	MaxEnemiesPerGroup int32   `json:"maxEnemiesPerGroup"`
	EnemyTypes         []int16
	WaveEnabled        bool    `json:"waveEnabled"` // 波次启用标志
}

// WaveManager 波次管理器
type WaveManager struct {
	mu sync.RWMutex

	waveConfig *WaveConfig
	lastPlan   *WavePlan
	waveCount  int32
	waveTime   float32
	nextWaveAt float32
}

// NewWaveManager 创建波次管理器
func NewWaveManager(cfg *WaveConfig) *WaveManager {
	if cfg == nil {
		cfg = &WaveConfig{
			InitialSpacingSec:  30,
			BaseSpacingSec:     90,
			EnemyBaseCount:     10,
			EnemyGrowthFactor:  1.2,
			MaxEnemiesPerGroup: 20,
			EnemyTypes:         []int16{0, 1, 2}, // default: alpha, mono, poly
		}
	}
	return &WaveManager{
		waveConfig: cfg,
		waveTime:   cfg.InitialSpacingSec,
		nextWaveAt: cfg.InitialSpacingSec,
	}
}

// Config 获取波次配置
func (wm *WaveManager) Config() *WaveConfig {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return wm.waveConfig
}

// SetConfig 设置波次配置
func (wm *WaveManager) SetConfig(cfg *WaveConfig) {
	wm.mu.Lock()
	defer wm.mu.Unlock()
	if cfg == nil {
		return
	}
	wm.waveConfig = cfg
	wm.nextWaveAt = cfg.InitialSpacingSec
}

// LastWavePlan 返回上一波的计划
func (wm *WaveManager) LastWavePlan() *WavePlan {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	if wm.lastPlan == nil {
		return nil
	}
	// 返回副本
	plan := *wm.lastPlan
	plan.EnemyTypePrior = make([]int16, len(wm.lastPlan.EnemyTypePrior))
	copy(plan.EnemyTypePrior, wm.lastPlan.EnemyTypePrior)
	return &plan
}

// GeneratePlan 生成波次计划
func (wm *WaveManager) GeneratePlan(waveNum int32) *WavePlan {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	cfg := wm.waveConfig

	// 计算波次参数
	baseCount := cfg.EnemyBaseCount
	growths := float32(waveNum-1) * cfg.EnemyGrowthFactor
	enemyCount := int32(float32(baseCount) * (1 + growths))
	if enemyCount < baseCount {
		enemyCount = baseCount
	}
	if enemyCount > cfg.MaxEnemiesPerGroup*10 {
		enemyCount = cfg.MaxEnemiesPerGroup * 10
	}

	groupSize := cfg.MaxEnemiesPerGroup
	if groupSize <= 0 {
		groupSize = 20
	}
	groupCount := enemyCount / groupSize
	if groupCount <= 0 {
		groupCount = 1
	}
	// Remainder goes to last group
	if enemyCount%groupCount != 0 {
		groupSize = enemyCount / groupCount
	}

	// 生成敌人类型优先级（随着波次增加解锁更多类型）
	typeCount := len(cfg.EnemyTypes)
	if typeCount == 0 {
		typeCount = 1
	}
	availableTypes := typeCount
	if waveNum > 5 {
		availableTypes = min(typeCount, availableTypes+1)
	}
	if waveNum > 10 {
		availableTypes = min(typeCount, availableTypes+1)
	}

	enemyTypePrior := make([]int16, availableTypes)
	for i := 0; i < availableTypes; i++ {
		enemyTypePrior[i] = cfg.EnemyTypes[i%len(cfg.EnemyTypes)]
	}

	plan := &WavePlan{
		WaveNumber:     waveNum,
		EnemyCount:     enemyCount,
		GroupSize:      groupSize,
		GroupCount:     groupCount,
		GroupSpacing:   cfg.BaseSpacingSec / float32(groupCount),
		EnemyTypePrior: enemyTypePrior,
	}
	wm.lastPlan = plan

	return plan
}

// UpdateTick 更新波次计时
func (wm *WaveManager) UpdateTick(deltaSec float32) (triggerWave bool, waveNum int32) {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	wm.waveTime -= deltaSec
	if wm.waveTime <= 0 {
		wm.waveCount++
		waveNum = wm.waveCount
		triggerWave = true

		// 计算下一波时间
		cfg := wm.waveConfig
		baseSpacing := cfg.BaseSpacingSec
		if wm.waveCount == 1 {
			baseSpacing = cfg.InitialSpacingSec
		}
		wm.nextWaveAt = baseSpacing
		wm.waveTime = wm.nextWaveAt
	}

	return triggerWave, waveNum
}

// Stats 返回波次统计
func (wm *WaveManager) Stats() struct {
	WaveCount  int32
	WaveTime   float32
	NextWaveAt float32
} {
	wm.mu.RLock()
	defer wm.mu.RUnlock()
	return struct {
		WaveCount  int32
		WaveTime   float32
		NextWaveAt float32
	}{
		WaveCount:  wm.waveCount,
		WaveTime:   wm.waveTime,
		NextWaveAt: wm.nextWaveAt,
	}
}

// Serialize 序列化波次管理器状态
func (wm *WaveManager) Serialize() []byte {
	wm.mu.RLock()
	defer wm.mu.RUnlock()

	state := map[string]interface{}{
		"waveCount":  wm.waveCount,
		"waveTime":   wm.waveTime,
		"nextWaveAt": wm.nextWaveAt,
	}
	if wm.lastPlan != nil {
		state["lastPlan"] = wm.lastPlan
	}
	if wm.waveConfig != nil {
		state["waveConfig"] = wm.waveConfig
	}

	data, _ := json.Marshal(state)
	return data
}

// Deserialize 反序列化波次管理器状态
func (wm *WaveManager) Deserialize(data []byte) error {
	wm.mu.Lock()
	defer wm.mu.Unlock()

	var state map[string]interface{}
	if err := json.Unmarshal(data, &state); err != nil {
		return err
	}

	if v, ok := state["waveCount"].(float64); ok {
		wm.waveCount = int32(v)
	}
	if v, ok := state["waveTime"].(float64); ok {
		wm.waveTime = float32(v)
	}
	if v, ok := state["nextWaveAt"].(float64); ok {
		wm.nextWaveAt = float32(v)
	}

	if v, ok := state["lastPlan"].(map[string]interface{}); ok {
		plan := &WavePlan{}
		if b, err := json.Marshal(v); err == nil {
			json.Unmarshal(b, plan)
			wm.lastPlan = plan
		}
	}

	if v, ok := state["waveConfig"].(map[string]interface{}); ok {
		cfg := &WaveConfig{}
		if b, err := json.Marshal(v); err == nil {
			json.Unmarshal(b, cfg)
			wm.waveConfig = cfg
		}
	}

	return nil
}

// Inline functions
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// 测试辅助函数
func TestGenerateWavePlan(config *WaveConfig, waveNum int32) *WavePlan {
	wm := NewWaveManager(config)
	return wm.GeneratePlan(waveNum)
}

// DemoWaveConfig 示例波次配置
var DemoWaveConfig = &WaveConfig{
	InitialSpacingSec:  30,
	BaseSpacingSec:     90,
	EnemyBaseCount:     10,
	EnemyGrowthFactor:  1.2,
	MaxEnemiesPerGroup: 20,
	EnemyTypes:         []int16{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
}

// 测试용
func ExampleWaveManager() {
	cfg := &WaveConfig{
		InitialSpacingSec:  30,
		BaseSpacingSec:     90,
		EnemyBaseCount:     10,
		EnemyGrowthFactor:  1.2,
		MaxEnemiesPerGroup: 20,
		EnemyTypes:         []int16{0, 1, 2},
	}

	wm := NewWaveManager(cfg)

	// 生成第1波
	plan1 := wm.GeneratePlan(1)
	fmt.Printf("Wave 1: count=%d, groups=%d\n", plan1.EnemyCount, plan1.GroupCount)

	// 生成第5波
	plan5 := wm.GeneratePlan(5)
	fmt.Printf("Wave 5: count=%d, groups=%d\n", plan5.EnemyCount, plan5.GroupCount)

	// 更新时间
	for i := 0; i < 10; i++ {
		trigger, _ := wm.UpdateTick(1.0)
		if trigger {
			plan := wm.GeneratePlan(wm.waveCount)
			fmt.Printf("Trigger wave %d\n", plan.WaveNumber)
		}
	}

	_ = plan1
	_ = plan5
}

// Test helper to run examples
func init() {
	rand.Seed(time.Now().UnixNano())
}
