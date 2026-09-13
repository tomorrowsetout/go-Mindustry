package storage

import (
	"fmt"
	"sync"
	"time"
)

// Event 事件结构体
type Event struct {
	Timestamp time.Time `json:"timestamp"`
	Kind      string    `json:"kind"`
	Trigger   Trigger   `json:"trigger,omitempty"`
	Packet    string    `json:"packet,omitempty"`
	Detail    string    `json:"detail,omitempty"`
	ConnID    int32     `json:"conn_id,omitempty"`
	UUID      string    `json:"uuid,omitempty"`
	IP        string    `json:"ip,omitempty"`
	Name      string    `json:"name,omitempty"`
}

// Trigger 枚举 - 事件触发器
type Trigger uint32

const (
	TriggerUpdate Trigger = iota // 游戏更新
	TriggerBeforeGameUpdate     // 游戏更新前
	TriggerAfterGameUpdate      // 游戏更新后
	TriggerDraw                   // 绘制
	TriggerPreDraw              // 绘制前
	TriggerPostDraw             // 绘制后
	TriggerUnitCommandChange    // 单位命令改变
	TriggerUnitCommandAttack    // 单位攻击命令
	TriggerUnitCommandBoost     // 单位加速命令
	TriggerNewGame              // 新游戏
	TriggerWaveSpawn            // 波次生成
	TriggerPlayerConnect        // 玩家连接
	TriggerPlayerJoin           // 玩家加入
	TriggerPlayerLeave          // 玩家离开
	TriggerBlockBuildBegin      // 建筑建造开始
	TriggerBlockBuildEnd        // 建筑建造结束
	TriggerBlockDestroy         // 建筑销毁
	TriggerUnitCreate           // 单位创建
	TriggerUnitDestroy          // 单位销毁
	TriggerCoreChange           // 核心改变
	TriggerTeamCoreDamage       // 队伍核心伤害
	TriggerWorldLoad            // 世界加载
	TriggerSaveLoad             // 存档加载
	TriggerResize               // 窗口调整
	TriggerUnitDamaged          // 单位受伤
	TriggerUnitDrown            // 单位溺水
	TriggerUnitFollow           // 单位跟随
	TriggerUnitWanted           // 单位被追逐
	TriggerItemMove             // 物品移动
	TriggerLiquidMove           // 液体移动
	TriggerBeginChestAccess     // 开始箱子访问
	TriggerEndChestAccess       // 结束箱子访问
	TriggerBegins               // 开始
	TriggerEnd                  // 结束
	TriggerTimer                // 定时器
	TriggerTouch                // 触摸
	TriggerBuildSelect          // 建筑选择
	TriggerUnitSelect           // 单位选择
	TriggerSelectTile           // 选择Tile
	Triggerdamage               // 伤害
	TriggerHeal                 // 治疗
	TriggerRespawn              // 重生
	TriggerConfig               // 配置
	TriggerEnter                // 进入
	TriggerExit                 // 退出
	TriggerBump                 // 碰撞
)

// String 返回触发器名称
func (t Trigger) String() string {
	switch t {
	case TriggerUpdate:
		return "update"
	case TriggerBeforeGameUpdate:
		return "beforeGameUpdate"
	case TriggerAfterGameUpdate:
		return "afterGameUpdate"
	case TriggerDraw:
		return "draw"
	case TriggerPreDraw:
		return "preDraw"
	case TriggerPostDraw:
		return "postDraw"
	case TriggerUnitCommandChange:
		return "unitCommandChange"
	case TriggerUnitCommandAttack:
		return "unitCommandAttack"
	case TriggerUnitCommandBoost:
		return "unitCommandBoost"
	case TriggerNewGame:
		return "newGame"
	case TriggerWaveSpawn:
		return "waveSpawn"
	case TriggerPlayerConnect:
		return "playerConnect"
	case TriggerPlayerJoin:
		return "playerJoin"
	case TriggerPlayerLeave:
		return "playerLeave"
	case TriggerBlockBuildBegin:
		return "blockBuildBegin"
	case TriggerBlockBuildEnd:
		return "blockBuildEnd"
	case TriggerBlockDestroy:
		return "blockDestroy"
	case TriggerUnitCreate:
		return "unitCreate"
	case TriggerUnitDestroy:
		return "unitDestroy"
	case TriggerCoreChange:
		return "coreChange"
	case TriggerTeamCoreDamage:
		return "teamCoreDamage"
	case TriggerWorldLoad:
		return "worldLoad"
	case TriggerSaveLoad:
		return "saveLoad"
	case TriggerResize:
		return "resize"
	case TriggerUnitDamaged:
		return "unitDamaged"
	case TriggerUnitDrown:
		return "unitDrown"
	case TriggerUnitFollow:
		return "unitFollow"
	case TriggerUnitWanted:
		return "unitWanted"
	case TriggerItemMove:
		return "itemMove"
	case TriggerLiquidMove:
		return "liquidMove"
	case TriggerBeginChestAccess:
		return "beginChestAccess"
	case TriggerEndChestAccess:
		return "endChestAccess"
	case TriggerBegins:
		return "begins"
	case TriggerEnd:
		return "end"
	case TriggerTimer:
		return "timer"
	case TriggerTouch:
		return "touch"
	case TriggerBuildSelect:
		return "buildSelect"
	case TriggerUnitSelect:
		return "unitSelect"
	case TriggerSelectTile:
		return "selectTile"
	case Triggerdamage:
		return "damage"
	case TriggerHeal:
		return "heal"
	case TriggerRespawn:
		return "respawn"
	case TriggerConfig:
		return "config"
	case TriggerEnter:
		return "enter"
	case TriggerExit:
		return "exit"
	case TriggerBump:
		return "bump"
	default:
		return fmt.Sprintf("unknown(%d)", t)
	}
}

// BaseEvent 基础事件
type BaseEvent struct {
	Timestamp time.Time
	Trigger   Trigger
}

// PlayerConnectEvent 玩家连接事件
type PlayerConnectEvent struct {
	BaseEvent
	PlayerID int32
	UUID     string
	IP       string
	Name     string
}

// PlayerJoinEvent 玩家加入事件
type PlayerJoinEvent struct {
	BaseEvent
	PlayerID int32
	UUID     string
	Name     string
}

// PlayerLeaveEvent 玩家离开事件
type PlayerLeaveEvent struct {
	BaseEvent
	PlayerID int32
	Reason   string
}

// BlockBuildBeginEvent 建筑建造开始事件
type BlockBuildBeginEvent struct {
	BaseEvent
	PlayerID int32
	TileX    int32
	TileY    int32
	BlockID  int16
}

// BlockBuildEndEvent 建筑建造结束事件
type BlockBuildEndEvent struct {
	BaseEvent
	PlayerID int32
	TileX    int32
	TileY    int32
	BlockID  int16
	TeamID   int32
}

// BlockDestroyEvent 建筑销毁事件
type BlockDestroyEvent struct {
	BaseEvent
	TileX    int32
	TileY    int32
	BlockID  int16
	TeamID   int32
	Damage   float32
}

// UnitCreateEvent 单位创建事件
type UnitCreateEvent struct {
	BaseEvent
	UnitID  int32
	TypeID  int16
	TeamID  int32
	X       float32
	Y       float32
	Health  float32
}

// UnitDestroyEvent 单位销毁事件
type UnitDestroyEvent struct {
	BaseEvent
	UnitID int32
	Reason string
}

// UnitDamagedEvent 单位受伤事件
type UnitDamagedEvent struct {
	BaseEvent
	UnitID  int32
	Damage  float32
	Source  string
	Health  float32
}

// UnitDrownEvent 单位溺水事件
type UnitDrownEvent struct {
	BaseEvent
	UnitID int32
}

// UnitFollowEvent 单位跟随事件
type UnitFollowEvent struct {
	BaseEvent
	UnitID     int32
	FollowUnit int32
}

// UnitWantedEvent 单位被追逐事件
type UnitWantedEvent struct {
	BaseEvent
	UnitID int32
	Wanted bool
}

// ItemMoveEvent 物品移动事件
type ItemMoveEvent struct {
	BaseEvent
	ItemID  int16
	Amount  int32
	FromX   float32
	FromY   float32
	ToX     float32
	ToY     float32
}

// LiquidMoveEvent 液体移动事件
type LiquidMoveEvent struct {
	BaseEvent
	LiquidID int16
	Amount   float32
	FromX    float32
	FromY    float32
	ToX      float32
	ToY      float32
}

// CoreChangeEvent 核心改变事件
type CoreChangeEvent struct {
	BaseEvent
	TeamID int32
	TileX  int32
	TileY  int32
}

// TeamCoreDamageEvent 队伍核心伤害事件
type TeamCoreDamageEvent struct {
	BaseEvent
	TeamID    int32
	Damage    float32
	Health    float32
	MaxHealth float32
}

// WaveSpawnEvent 波次生成事件
type WaveSpawnEvent struct {
	BaseEvent
	Wave       int32
	EnemyCount int32
}

// SoundEvent 音效事件
type SoundEvent struct {
	BaseEvent
	SoundID int32
	X       float32
	Y       float32
	Volume  float32
	Pitch   float32
}

// EffectEvent 特效事件
type EffectEvent struct {
	BaseEvent
	EffectID int32
	X        float32
	Y        float32
	Arg1     int32
	Arg2     int32
	Arg3     int32
}

// MessageEvent 消息事件
type MessageEvent struct {
	BaseEvent
	Type    string
	Message string
	Color   string
}

// Recorder 事件监听器接口
type Recorder interface {
	Record(Event) error
	Close() error
	Status() string
}

// 存储的事件列表
type storageEvents struct {
	mu     sync.RWMutex
	events []Event
	limit  int
}

// NewEventLogger 创建新的事件日志记录器
func NewEventLogger(limit int) *storageEvents {
	return &storageEvents{
		limit: limit,
	}
}

// Record 记录事件
func (s *storageEvents) Record(ev Event) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.limit > 0 && len(s.events) >= s.limit {
		// 移除最早的事件
		s.events = s.events[1:]
	}

	s.events = append(s.events, ev)
	return nil
}

// Close 关闭
func (s *storageEvents) Close() error {
	return nil
}

// Status 返回状态
func (s *storageEvents) Status() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return fmt.Sprintf("events: %d", len(s.events))
}

// GetEvents 获取事件列表
func (s *storageEvents) GetEvents() []Event {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.events
}

// Clear 清除事件
func (s *storageEvents) Clear() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.events = nil
}

// Hook 事件钩子函数类型
type Hook func(ev Event)

// EventManager 事件管理器
type EventManager struct {
	mu       sync.RWMutex
	hooks    map[Trigger][]Hook
	allHooks []Hook
}

// NewEventManager 创建新的事件管理器
func NewEventManager() *EventManager {
	return &EventManager{
		hooks: make(map[Trigger][]Hook),
	}
}

// AddHook 添加钩子函数（指定触发器）
func (em *EventManager) AddHook(trigger Trigger, hook Hook) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.hooks[trigger] = append(em.hooks[trigger], hook)
}

// AddAllHook 添加所有触发器的钩子函数
func (em *EventManager) AddAllHook(hook Hook) {
	em.mu.Lock()
	defer em.mu.Unlock()
	em.allHooks = append(em.allHooks, hook)
}

// Dispatch 分发事件
func (em *EventManager) Dispatch(ev Event) {
	em.mu.RLock()
	defer em.mu.RUnlock()

	// 调用所有触发器的钩子
	for _, hook := range em.allHooks {
		hook(ev)
	}

	// 调用指定触发器的钩子
	if hooks, ok := em.hooks[ev.Trigger]; ok {
		for _, hook := range hooks {
			hook(ev)
		}
	}
}
