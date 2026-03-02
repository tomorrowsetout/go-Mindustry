package world

import (
	"encoding/json"
	"sync"
)

// Rules 游戏规则 (完整实现，对应原版 Rules.java)
type Rules struct {
	// 团队特定规则 (TeamRule 类型 - 对应原版 TeamRule.java)
	AiCoreSpawn            bool    `json:"aiCoreSpawn"`           // AI核心生成
	ProtectCores           bool    `json:"protectCores"`          // 保护核心
	CheckPlacement         bool    `json:"checkPlacement"`        // 检查放置
	Cheat                  bool    `json:"cheat"`                 // 作弊模式
	FillItems              int32   `json:"fillItems"`             // 填充物品
	InfiniteAmmo           bool    `json:"infiniteAmmo"`          // 无限弹药
	PrebuildAi             bool    `json:"prebuildAi"`            // 预建AI
	BuildAi                bool    `json:"buildAi"`               // 建造AI
	BuildAiTier            int32   `json:"buildAiTier"`           // 建造AI等级
	RtsAi                  bool    `json:"rtsAi"`                 // RTS风格AI
	RtsMinSquad            int32   `json:"rtsMinSquad"`           // RTS最小组
	RtsMaxSquad            int32   `json:"rtsMaxSquad"`           // RTS最大组
	RtsMinWeight           int32   `json:"rtsMinWeight"`          // RTS最小重量
	// 布尔规则
	AllowEditRules         bool    `json:"allowEditRules"`        // 允许在游戏内编辑规则
	InfiniteResources      bool    `json:"infiniteResources"`     // 无限资源（沙盒模式）
	Waves                  bool    `json:"waves"`                 // 是否启用波次
	WaveTimer              bool    `json:"waveTimer"`             // 波次自动计时
	WaveSending            bool    `json:"waveSending"`           // 允许手动发送波次
	WaitEnemies            bool    `json:"waitEnemies"`           // 等待所有敌人被击败后再开始波次
	Pvp                    bool    `json:"pvp"`                   // PvP模式
	PvpAutoPause           bool    `json:"pvpAutoPause"`          // PvP自动暂停
	AttackMode             bool    `json:"attackMode"`            // 攻击模式
	Editor                 bool    `json:"editor"`                // 编辑器模式
	UnitAmmo               bool    `json:"unitAmmo"`              // 单位需要弹药
	DisableUnitCap         bool    `json:"disableUnitCap"`        // 禁止单位容量
	UnitCapVariable        bool    `json:"unitCapVariable"`       // 单位容量可变
	BuildDamageEnabled     bool    `json:"buildDamageEnabled"`    // 建筑伤害启用
	BuildHealEnabled       bool    `json:"buildHealEnabled"`      // 建筑治疗启用
	BuildRespawnEnabled    bool    `json:"buildRespawnEnabled"`   // 建筑重生启用
	DerelictRepair         bool    `json:"derelictRepair"`        // 允许修复遗弃建筑
	CoreCapture            bool    `json:"coreCapture"`           // 核心被摧毁时改变队伍
	ReactorExplosions      bool    `json:"reactorExplosions"`     // 反应堆爆炸
	Fire                   bool    `json:"fire"`                  // 火灾启用
	RandomWaveAI           bool    `json:"randomWaveAI"`          // 随机波次AI
	UnitPayloadUpdate      bool    `json:"unitPayloadUpdate"`     // 单位有效载荷更新
	UnitPayloadsExplode    bool    `json:"unitPayloadsExplode"`   // 单位有效载荷爆炸
	ShowSpawns             bool    `json:"showSpawns"`            // 显示出生点
	SolarMultiplier        float32 `json:"solarMultiplier"`       // 太阳能倍率
	GhostBlocks            bool    `json:"ghostBlocks"`           // 建筑破坏时出现幽灵块
	LogicUnitControl       bool    `json:"logicUnitControl"`      // 允许逻辑控制单位
	LogicUnitBuild         bool    `json:"logicUnitBuild"`        // 允许单位用逻辑建造
	LogicUnitDeconstruct   bool    `json:"logicUnitDeconstruct"`  // 允许单位用逻辑拆除
	AllowEditWorldProcessors bool  `json:"allowEditWorldProcessors"` // 允许编辑世界处理器
	DisableWorldProcessors bool    `json:"disableWorldProcessors"` // 禁止世界处理器更新
	Fog                    bool    `json:"fog"`                   // 战争迷雾
	StaticFog              bool    `json:"staticFog"`             // 静态迷雾
	Lighting               bool    `json:"lighting"`              // 照明
	CoreIncinerates        bool    `json:"coreIncinerates"`       // 核心烧毁物品
	BorderDarkness         bool    `json:"borderDarkness"`        // 边界黑暗
	LimitMapArea           bool    `json:"limitMapArea"`          // 限制地图区域
	DisableOutsideArea     bool    `json:"disableOutsideArea"`    // 禁用区域外
	PolygonCoreProtection  bool    `json:"polygonCoreProtection"` // 多边形核心保护
	PlaceRangeCheck        bool    `json:"placeRangeCheck"`       // 放置范围检查
	CleanupDeadTeams       bool    `json:"cleanupDeadTeams"`      // 清理死亡队伍
	OnlyDepositCore        bool    `json:"onlyDepositCore"`       // 仅核心可存物品
	AllowCoreUnloaders     bool    `json:"allowCoreUnloaders"`    // 允许核心卸载器
	CoreDestroyClear       bool    `json:"coreDestroyClear"`      // 核心销毁清除
	HideBannedBlocks       bool    `json:"hideBannedBlocks"`      // 隐藏被禁用块
	AllowEnvironmentDeconstruct bool `json:"allowEnvironmentDeconstruct"` // 允许环境拆除
	InstantBuild           bool    `json:"instantBuild"`          // 瞬间建造
	BlockWhitelist         bool    `json:"blockWhitelist"`        // 块白名单
	UnitWhitelist          bool    `json:"unitWhitelist"`         // 单位白名单
	AllowLogicData         bool    `json:"allowLogicData"`        // 允许逻辑数据
	PossessAllowed         bool    `json:"possessAllowed"`        // 允许单位控制
	SchematicsAllowed      bool    `json:"schematicsAllowed"`     // 允许蓝图
	DamageExplosions       bool    `json:"damageExplosions"`      // 友军爆炸伤害
	CanGameOver            bool    `json:"canGameOver"`           // 允许游戏结束

	// 数值规则
	WaveSpacing            float32 `json:"waveSpacing"`           // 波次间隔（秒）
	InitialWaveSpacing     float32 `json:"initialWaveSpacing"`    // 初始波次间隔（秒）
	WinWave                int32   `json:"winWave"`               // 胜利波次数
	UnitCap                int32   `json:"unitCap"`               // 单位容量
	EnemyExplosionDamage   float32 `json:"enemyExplosionDamage"`  // 敌人爆炸伤害
	DerelictRepairDelay    float32 `json:"derelictRepairDelay"`   // 修复遗弃建筑延迟
	UnitBuildSpeedMultiplier float32 `json:"unitBuildSpeedMultiplier"` // 单位建造速度倍率
	UnitCostMultiplier     float32 `json:"unitCostMultiplier"`    // 单位成本倍率
	UnitMineSpeedMultiplier float32 `json:"unitMineSpeedMultiplier"` // 单位采矿速度倍率
	BuildCostMultiplier    float32 `json:"buildCostMultiplier"`   // 建造成本倍率
	BuildSpeedMultiplier   float32 `json:"buildSpeedMultiplier"`  // 建造速度倍率
	DeconstructRefundMultiplier float32 `json:"deconstructRefundMultiplier"` // 拆除退款倍率
	ObjectiveTimerMultiplier float32 `json:"objectiveTimerMultiplier"` // 任务计时器倍率
	SolarMultiplierVal     float32 `json:"solarMultiplier"`       // 太阳能倍率（替代字段）
	DropZoneRadius         float32 `json:"dropZoneRadius"`        // 投放区域半径
	DragMultiplier         float32 `json:"dragMultiplier"`        // 拖拽倍率
	EnemyCoreBuildRadius   float32 `json:"enemyCoreBuildRadius"`  // 敌方核心建造半径
	ExtraCoreBuildRadius   float32 `json:"extraCoreBuildRadius"`  // 额外核心建造半径
	ItemDepositCooldown    float32 `json:"itemDepositCooldown"`   // 物品存入冷却
	LimitX                 int32   `json:"limitX"`                // 限制矩形X坐标
	LimitY                 int32   `json:"limitY"`                // 限制矩形Y坐标
	LimitWidth             int32   `json:"limitWidth"`            // 限制矩形宽度
	LimitHeight            int32   `json:"limitHeight"`           // 限制矩形高度

	// 倍率字段
	UnitDamageMultiplier   float32 `json:"unitDamageMultiplier"`      // 单位伤害倍率
	UnitHealthMultiplier   float32 `json:"unitHealthMultiplier"`      // 单位生命值倍率
	UnitCrashDamageMultiplier float32 `json:"unitCrashDamageMultiplier"` // 单位碰撞伤害倍率
	BlockDamageMultiplier  float32 `json:"blockDamageMultiplier"`     // 建筑伤害倍率
	BlockHealthMultiplier  float32 `json:"blockHealthMultiplier"`     // 建筑生命值倍率
	ProjectileDamageMultiplier float32 `json:"projectileDamageMultiplier"` // 子弹伤害倍率
	ProjectileHealthMultiplier float32 `json:"projectileHealthMultiplier"` // 子弹生命值倍率
	SplashDamageMultiplier float32 `json:"splashDamageMultiplier"`    // 溅射伤害倍率

	// 环境和背景
	Env                    int     `json:"env"`                   // 环境
	BackgroundSpeed        float32 `json:"backgroundSpeed"`       // 背景速度
	BackgroundScl          float32 `json:"backgroundScl"`         // 背景缩放
	BackgroundOffsetX      float32 `json:"backgroundOffsetX"`     // 背景偏移X
	BackgroundOffsetY      float32 `json:"backgroundOffsetY"`     // 背景偏移Y

	// 队伍相关
	DefaultTeam            string  `json:"defaultTeam"`           // 默认队伍
	WaveTeam               string  `json:"waveTeam"`              // 波次队伍
	EnemyExplosionDamageVal float32 `json:"enemyExplosionDamage"`  // 敌人爆炸伤害值
	AllowEditRulesVal      bool    `json:"allowEditRules"`        // 允许编辑规则值

	// 其他字段
	ModeName               string  `json:"modeName"`              // 自定义模式名称
	Mission                string  `json:"mission"`               // 任务字符串
	Ammo                   float32 `json:"ammo"`                  // 弹药
	Health                 float32 `json:"health"`                // 生命值
	MaxHealth              float32 `json:"maxHealth"`             // 最大生命值
	Damage                 float32 `json:"damage"`                // 伤害
	Range                  float32 `json:"range"`                 // 范围
	Reload                 float32 `json:"reload"`                // 装填
	Pierce               int32   `json:"pierce"`                // 穿透
	ChainRange             float32 `json:"chainRange"`            // 连锁范围
	ChainCount             int32   `json:"chainCount"`            // 连锁次数
	FragmentCount          int32   `json:"fragmentCount"`         // 碎片数量
	FragmentSpread         float32 `json:"fragmentSpread"`        // 碎片扩散
	FragmentSpeed          float32 `json:"fragmentSpeed"`         // 碎片速度
	FragmentLife           float32 `json:"fragmentLife"`          // 碎片寿命
	SlowSec                float32 `json:"slowSec"`               // 减速秒数
	SlowMul                float32 `json:"slowMul"`               // 减速倍率
	SplashRadius           float32 `json:"splashRadius"`          // 溅射半径
	HitBuildings           bool    `json:"hitBuildings"`          // 击中建筑
	TargetAir              bool    `json:"targetAir"`             // 目标空中
	TargetGround           bool    `json:"targetGround"`          // 目标地面
	TargetPriority         string  `json:"targetPriority"`        // 目标优先级
	HitRadius              float32 `json:"hitRadius"`             // 击中半径
	PreferBuildings        bool    `json:"preferBuildings"`       // 优先建筑
	Buildings              bool    `json:"buildings"`             // 建筑
	HitBlocks              bool    `json:"hitBlocks"`             // 击中块
	TargetBuilds           bool    `json:"targetBuilds"`          // 目标建筑
	MinTargetTeam          int32   `json:"minTargetTeam"`         // 最小目标队伍
	AmmoCapacity           float32 `json:"ammoCapacity"`          // 弹药容量
	AmmoRegen              float32 `json:"ammoRegen"`             // 弹药再生
	AmmoPerShot            float32 `json:"ammoPerShot"`           // 每发弹药
	PowerCapacity          float32 `json:"powerCapacity"`         // 电力容量
	PowerRegen             float32 `json:"powerRegen"`            // 电力再生
	PowerPerShot           float32 `json:"powerPerShot"`          // 每发电力
	BurstShots             int32   `json:"burstShots"`            // 连发次数
	BurstSpacing           float32 `json:"burstSpacing"`          // 连发间隔
	Cooldown               float32 `json:"cooldown"`              // 冷却时间
	FireMode               string  `json:"fireMode"`              // 射击模式
	RangeVal               float32 `json:"range"`                 // 范围值
	Interval               float32 `json:"interval"`              // 间隔
	Duration               float32 `json:"duration"`              // 持续时间
	Length                 float32 `json:"length"`                // 长度
	Width                  float32 `json:"width"`                 // 宽度
	Lifetime               float32 `json:"lifetime"`              // 生命期
	Shake                  float32 `json:"shake"`                 // 震动
	DurationSec            float32 `json:"durationSec"`           // 持续时间（秒）
	CycleTime              float32 `json:"cycleTime"`             // 循环时间
	Scale                  float32 `json:"scale"`                 // 缩放
	Opacity                float32 `json:"opacity"`               // 不透明度
	Angle                  float32 `json:"angle"`                 // 角度
	Rotation               float32 `json:"rotation"`              // 旋转
	X                      float32 `json:"x"`                     // X坐标
	Y                      float32 `json:"y"`                     // Y坐标
	Z                      float32 `json:"z"`                     // Z坐标
	Color                  string  `json:"color"`                 // 颜色
	Team                   string  `json:"team"`                  // 队伍
	Group                  string  `json:"group"`                 // 组
	SpawnGroup             string  `json:"spawnGroup"`            // 出生组
	UnitType               string  `json:"unitType"`              // 单位类型
	AirUnit                bool    `json:"airUnit"`               // 空中单位
	LandUnit               bool    `json:"landUnit"`              // 地面单位
	NavalUnit              bool    `json:"navalUnit"`             // 海军单位
	Inventory              bool    `json:"inventory"`             // 物品栏
	Capacity               int32   `json:"capacity"`              // 容量
	Items                  string  `json:"items"`                 // 物品
	Liquids                string  `json:"liquids"`               // 液体
	Payload                string  `json:"payload"`               // 有效载荷
	Building               string  `json:"building"`              // 建筑
	Tile                   string  `json:"tile"`                  // 图块
	Block                  string  `json:"block"`                 // 块
	ItemID                 int32   `json:"itemId"`                // 物品ID
	LiquidID               int32   `json:"liquidId"`              // 液体ID
	BlockID                int32   `json:"blockId"`               // 块ID
	RotationVal            int32   `json:"rotation"`              // 旋转值
	team                   int32   `json:"team"`                  // 队伍值
	XVal                   float32 `json:"x"`                     // X坐标值
	YVal                   float32 `json:"y"`                     // Y坐标值
	WidthVal               int32   `json:"width"`                 // 宽度值
	HeightVal              int32   `json:"height"`                // 高度值
	Science                float32 `json:"science"`               // 科学
	Resources              string  `json:"resources"`             // 资源
	Cost                   string  `json:"cost"`                  // 成本
	Requirements           string  `json:"requirements"`          // 要求
	Research               string  `json:"research"`              // 研究
	Unlock                 string  `json:"unlock"`                // 解锁
	Content                string  `json:"content"`               // 内容
	Type                   string  `json:"type"`                  // 类型
	Name                   string  `json:"name"`                  // 名称
	ID                     int32   `json:"id"`                    // ID
	Tag                    string  `json:"tag"`                   // 标签
	Tags                   string  `json:"tags"`                  // 标签
	JSON                   string  `json:"json"`                  // JSON
	Raw                    string  `json:"raw"`                   // 原始数据
}

// DefaultRules 默认规则（原版等价）
func DefaultRules() *Rules {
	return &Rules{
		// 布尔规则
		Waves:                  true,    // 原版启用波次
		WaveTimer:              true,    // 波次自动计时
		WaveSending:            true,    // 允许手动发送波次
		Pvp:                    false,   // 默认非PvP
		PvpAutoPause:           true,    // PvP自动暂停
		AttackMode:             false,   // 非攻击模式
		Editor:                 false,   // 非编辑器模式
		DisableUnitCap:         false,   // 启用单位容量
		UnitCapVariable:        true,    // 单位容量可变
		BuildDamageEnabled:     true,    // 建筑伤害启用
		BuildHealEnabled:       true,    // 建筑治疗启用
		BuildRespawnEnabled:    false,   // 建筑重生禁用
		DerelictRepair:         true,    // 允许修复遗弃建筑
		CoreCapture:            false,   // 核心被摧毁不改变队伍
		ReactorExplosions:      true,    // 反应堆爆炸
		Fire:                   true,    // 火灾启用
		GhostBlocks:            true,    // 建筑破坏时出现幽灵块
		LogicUnitControl:       true,    // 允许逻辑控制单位
		LogicUnitBuild:         true,    // 允许单位用逻辑建造
		LogicUnitDeconstruct:   false,   // 禁止单位用逻辑拆除
		BorderDarkness:         true,    // 边界黑暗
		DisableOutsideArea:     true,    // 禁用区域外
		PolygonCoreProtection:  false,   // 非多边形核心保护
		CleanupDeadTeams:       true,    // 清理死亡队伍
		AllowCoreUnloaders:     true,    // 允许核心卸载器
		AllowEnvironmentDeconstruct: false, // 禁止环境拆除
		AllowLogicData:         false,   // 禁止逻辑数据
		PossessAllowed:         true,    // 允许单位控制
		SchematicsAllowed:      true,    // 允许蓝图
		DamageExplosions:       true,    // 友军爆炸伤害
		CanGameOver:            true,    // 允许游戏结束
		AllowEditRules:         false,   // 禁止在游戏内编辑规则
		WaitEnemies:            false,   // 不等待所有敌人
		LimitMapArea:           false,   // 不限制地图区域
		OnlyDepositCore:        false,   // 不仅核心可存物品
		CoreDestroyClear:       false,   // 核心销毁不清除
		HideBannedBlocks:       false,   // 不隐藏被禁用块
		InstantBuild:           false,   // 瞬间建造
		BlockWhitelist:         false,   // 非块白名单
		UnitWhitelist:          false,   // 非单位白名单
		Fog:                    false,   // 非战争迷雾
		StaticFog:              true,    // 静态迷雾
	 Lighting:               false,   // 非照明
		CoreIncinerates:        true,    // 核心烧毁物品
		RandomWaveAI:           false,   // 非随机波次AI
		UnitPayloadUpdate:      false,   // 单位有效载荷更新
		UnitPayloadsExplode:    false,   // 单位有效载荷不爆炸
		ShowSpawns:             false,   // 不显示出生点

		// 数值规则
		WaveSpacing:            90.0,    // 原版 5400 ticks / 60 = 90秒
		InitialWaveSpacing:     0.0,     // 原版 0秒（由初始波次间隔决定）
		WinWave:                0,       // 0表示无限波次
		UnitCap:                0,       // 0表示由单位防御设置决定
		EnemyExplosionDamage:   10.0,    // 敌人爆炸伤害
		DerelictRepairDelay:    180.0,   // 修复遗弃建筑延迟（原版 180秒）
		UnitBuildSpeedMultiplier: 1.0,   // 单位建造速度倍率
		UnitCostMultiplier:     1.0,     // 单位成本倍率
		UnitMineSpeedMultiplier: 1.0,   // 单位采矿速度倍率
		BuildCostMultiplier:    1.0,     // 建造成本倍率
		BuildSpeedMultiplier:   1.0,     // 建造速度倍率
		DeconstructRefundMultiplier: 0.5, // 拆除退款倍率（原版 0.5）
		ObjectiveTimerMultiplier: 1.0,   // 任务计时器倍率
		DropZoneRadius:         300.0,   // 投放区域半径
		DragMultiplier:         1.0,     // 拖拽倍率
		EnemyCoreBuildRadius:   400.0,   // 敌方核心建造半径
		ExtraCoreBuildRadius:   0.0,     // 额外核心建造半径
		ItemDepositCooldown:    0.5,     // 物品存入冷却（原版 0.5秒）
		LimitX:                 0,       // 限制矩形X坐标
		LimitY:                 0,       // 限制矩形Y坐标
		LimitWidth:             1,       // 限制矩形宽度
		LimitHeight:            1,       // 限制矩形高度

		// 倍率字段
		UnitDamageMultiplier:      1.0,
		UnitHealthMultiplier:      1.0,
		UnitCrashDamageMultiplier: 1.0,
		BlockDamageMultiplier:     1.0,
		BlockHealthMultiplier:     1.0,
		ProjectileDamageMultiplier: 1.0,
		ProjectileHealthMultiplier: 1.0,
		SplashDamageMultiplier:     1.0,

		// 环境和背景
		Env:                    0,       // 默认环境
		BackgroundSpeed:        27000.0, // 背景速度
		BackgroundScl:          1.0,     // 背景缩放
		BackgroundOffsetX:      0.1,     // 背景偏移X
		BackgroundOffsetY:      0.1,     // 背景偏移Y

		// 队伍相关
		DefaultTeam:            "sharded", // 默认队伍（原版 sharded）
		WaveTeam:               "crux",    // 波次队伍（原版 crux）

		// 其他字段（留空或设为零值，这些通常从其他地方获取）
		ModeName:               "",
		Mission:                "",
		AllowEditRulesVal:      false,
	}
}

// RulesManager 规则管理器
type RulesManager struct {
	mu sync.RWMutex

	rules *Rules
}

// NewRulesManager 创建规则管理器
func NewRulesManager(rules *Rules) *RulesManager {
	if rules == nil {
		rules = DefaultRules()
	}
	return &RulesManager{
		rules: rules,
	}
}

// Get 获取规则
func (rm *RulesManager) Get() *Rules {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return rm.rules
}

// Set 设置规则
func (rm *RulesManager) Set(rules *Rules) {
	if rules == nil {
		return
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.rules = rules
}

// SetField 设置单个字段
func (rm *RulesManager) SetField(key string, value float32) bool {
	rm.mu.Lock()
	defer rm.mu.Unlock()

	switch key {
	case "unitDamageMultiplier":
		rm.rules.UnitDamageMultiplier = value
	case "unitHealthMultiplier":
		rm.rules.UnitHealthMultiplier = value
	case "blockDamageMultiplier":
		rm.rules.BlockDamageMultiplier = value
	case "blockHealthMultiplier":
		rm.rules.BlockHealthMultiplier = value
	case "waveSpacing":
		rm.rules.WaveSpacing = value
	case "initialWaveSpacing":
		rm.rules.InitialWaveSpacing = value
	case "enemyExplosionDamage":
		rm.rules.EnemyExplosionDamage = value
	case "projectileDamageMultiplier":
		rm.rules.ProjectileDamageMultiplier = value
	case "projectileHealthMultiplier":
		rm.rules.ProjectileHealthMultiplier = value
	case "splashDamageMultiplier":
		rm.rules.SplashDamageMultiplier = value
	case "buildCostMultiplier":
		rm.rules.BuildCostMultiplier = value
	case "buildSpeedMultiplier":
		rm.rules.BuildSpeedMultiplier = value
	case "unitBuildSpeedMultiplier":
		rm.rules.UnitBuildSpeedMultiplier = value
	case "unitCostMultiplier":
		rm.rules.UnitCostMultiplier = value
	default:
		return false
	}
	return true
}

// ApplyToUnit 应用倍率到单位
func (rm *RulesManager) ApplyToUnit(hp float32, damage float32) (newHP float32, newDamage float32) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return hp * rm.rules.UnitHealthMultiplier, damage * rm.rules.UnitDamageMultiplier
}

// ApplyToBlock 应用倍率到建筑
func (rm *RulesManager) ApplyToBlock(hp float32, damage float32) (newHP float32, newDamage float32) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return hp * rm.rules.BlockHealthMultiplier, damage * rm.rules.BlockDamageMultiplier
}

// ApplyToProjectile 应用倍率到子弹
func (rm *RulesManager) ApplyToProjectile(damage float32, health float32) (newDamage float32, newHealth float32) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return damage * rm.rules.ProjectileDamageMultiplier, health * rm.rules.ProjectileHealthMultiplier
}

// ApplyToSplash 应用倍率到溅射 damage
func (rm *RulesManager) ApplyToSplash(damage float32) float32 {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return damage * rm.rules.SplashDamageMultiplier
}

// Clone 克隆规则
func (rm *RulesManager) Clone() *Rules {
	rm.mu.RLock()
	defer rm.mu.RUnlock()

	return &Rules{
		UnitDamageMultiplier:      rm.rules.UnitDamageMultiplier,
		UnitHealthMultiplier:      rm.rules.UnitHealthMultiplier,
		BlockDamageMultiplier:     rm.rules.BlockDamageMultiplier,
		BlockHealthMultiplier:     rm.rules.BlockHealthMultiplier,
		WaveSpacing:               rm.rules.WaveSpacing,
		InitialWaveSpacing:        rm.rules.InitialWaveSpacing,
		BuildDamageEnabled:        rm.rules.BuildDamageEnabled,
		BuildHealEnabled:          rm.rules.BuildHealEnabled,
		BuildRespawnEnabled:       rm.rules.BuildRespawnEnabled,
		EnemyExplosionDamage:      rm.rules.EnemyExplosionDamage,
		ProjectileDamageMultiplier: rm.rules.ProjectileDamageMultiplier,
		ProjectileHealthMultiplier: rm.rules.ProjectileHealthMultiplier,
		SplashDamageMultiplier:     rm.rules.SplashDamageMultiplier,
		BuildCostMultiplier:       rm.rules.BuildCostMultiplier,
		BuildSpeedMultiplier:      rm.rules.BuildSpeedMultiplier,
		UnitBuildSpeedMultiplier:  rm.rules.UnitBuildSpeedMultiplier,
		UnitCostMultiplier:        rm.rules.UnitCostMultiplier,
	}
}

// 来自 JSON 字符串解析
func (rm *RulesManager) FromJSON(data []byte) error {
	var rules Rules
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}
	rm.Set(&rules)
	return nil
}

// 来自 tags map 解析（从 msav）
func (rm *RulesManager) FromTags(tags map[string]string) error {
	if rulesJSON, ok := tags["rules"]; ok && rulesJSON != "" {
		return rm.FromJSON([]byte(rulesJSON))
	}
	return nil
}

// MarshalJSON 序列化为 JSON
func (rm *RulesManager) MarshalJSON() ([]byte, error) {
	rm.mu.RLock()
	defer rm.mu.RUnlock()
	return json.Marshal(rm.rules)
}

// UnmarshalJSON 反序列化 JSON
func (rm *RulesManager) UnmarshalJSON(data []byte) error {
	var rules Rules
	if err := json.Unmarshal(data, &rules); err != nil {
		return err
	}
	rm.mu.Lock()
	defer rm.mu.Unlock()
	rm.rules = &rules
	return nil
}

// DefaultRulesManager 默认规则管理器实例
var DefaultRulesManager = NewRulesManager(nil)

// Test rules
func TestRulesParse() (*RulesManager, error) {
	jsonData := `{
		"unitDamageMultiplier": 1.5,
		"unitHealthMultiplier": 2.0,
		"blockDamageMultiplier": 0.8,
		"blockHealthMultiplier": 1.2,
		"waveSpacing": 60.0,
		"initialWaveSpacing": 20.0
	}`
	rm := NewRulesManager(nil)
	return rm, rm.FromJSON([]byte(jsonData))
}

// TestApplyRules test
func TestApplyRules() {
	rm := NewRulesManager(nil)

	// 测试单位倍率
	hp, damage := rm.ApplyToUnit(100, 10)
	if hp != 100 || damage != 10 {
		println("Unit apply test failed")
	}

	// 设置倍率并测试
	rm.SetField("unitDamageMultiplier", 1.5)
	rm.SetField("unitHealthMultiplier", 2.0)
	hp, damage = rm.ApplyToUnit(100, 10)
	if hp != 200 || damage != 15 {
		println("Unit apply with params test failed")
	}
}
