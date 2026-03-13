package world

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mdt-server/internal/logic"
	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
)

type Snapshot struct {
	WaveTime float32
	Wave     int32
	Enemies  int32
	Paused   bool
	GameOver bool
	TimeData int32
	Tps      int8
	Rand0    int64
	Rand1    int64
	Tick     uint64
}

type bridgeBufferItem struct {
	Item      ItemID
	ReadyTick uint64
}

type junctionLaneItem struct {
	Item      ItemID
	ReadyTick uint64
}

type junctionLane struct {
	items [6]junctionLaneItem
	len   int
}

type junctionState struct {
	lanes [4]junctionLane
}

type ductJunctionState struct {
	items     [4]junctionLaneItem
	hasItem   [4]bool
	readyTick [4]uint64
}

type ductRouterState struct {
	current  ItemID
	progress float32
	dumpIdx  int
}

type stackRouterState struct {
	current   ItemID
	progress  float32
	unloading bool
	dumpIdx   int
}

type ductUnloaderState struct {
	timer  float32
	offset int
}

type Config struct {
	TPS int
}

var defaultWorldMu sync.RWMutex
var defaultWorld *World

// SetDefaultWorld registers the default world used by helper routines.
func SetDefaultWorld(w *World) {
	defaultWorldMu.Lock()
	defaultWorld = w
	defaultWorldMu.Unlock()
}

// DefaultWorld returns the current default world instance, if any.
func DefaultWorld() *World {
	defaultWorldMu.RLock()
	w := defaultWorld
	defaultWorldMu.RUnlock()
	return w
}

type World struct {
	mu sync.RWMutex

	wave     int32
	waveTime float32
	tick     uint64

	rand0 int64
	rand1 int64

	tps int8

	start time.Time

	model *WorldModel

	// 规则和波次管理器
	rulesMgr *RulesManager
	wavesMgr *WaveManager

	entityEvents []EntityEvent
	bullets      []simBullet
	bulletNextID int32

	blockNamesByID   map[int16]string
	unitNamesByID    map[int16]string
	unitTypeDefsByID map[int16]vanilla.UnitTypeDef
	buildStates      map[int32]buildCombatState
	pendingBuilds    map[int32]pendingBuildState
	unitMountCDs     map[int32][]float32
	unitTargets      map[int32]targetTrackState
	unitBurstStates  map[int32]unitBurstState
	unitMountBursts  map[int32][]unitBurstState

	unitProfilesByType      map[int16]weaponProfile
	unitProfilesByName      map[string]weaponProfile
	buildingProfilesByName  map[string]buildingWeaponProfile
	tileConfigValues        map[int32]any
	sorterRouteBits         map[int32]byte
	routerRouteBits         map[int32]byte
	routerInputDirs         map[int32]int8
	junctionInputDirs       map[int32]int8
	unloaderRotations       map[int32]int16
	massDriverStates        map[int32]massDriverState
	payloadRouterRouteBits  map[int32]byte
	payloadRouterInputDirs  map[int32]int8
	payloadMassStates       map[int32]massDriverState
	liquidJunctionInputDirs map[int32]int8
	bridgeProgress          map[int32]float32
	bridgeBuffers           map[int32][]bridgeBufferItem
	junctionStates          map[int32]*junctionState
	ductJunctionStates      map[int32]*ductJunctionState
	ductRouterStates        map[int32]*ductRouterState
	stackRouterStates       map[int32]*stackRouterState
	ductUnloaderStates      map[int32]*ductUnloaderState
	shieldStates            map[int32]shieldState
	mendCharge              map[int32]float32
	overdriveBoostByPos     map[int32]float32
	activeShields           []activeShield
	projectorUse            map[int32]float32

	timeMgr              *WorldTime
	weatherNamesByID     map[int16]string
	weatherIDsByName     map[string]int16
	lastWeatherStartTick uint64
	itemNamesByID        map[int16]string
	itemIDsByName        map[string]int16
	liquidNamesByID      map[int16]string
	liquidIDsByName      map[string]int16
	itemPropsDefs        []ItemPropsDef
	itemPropsByName      map[string]ItemProps
	itemPropsByID        map[int16]ItemProps
	liquidPropsDefs      []LiquidPropsDef
	liquidPropsByName    map[string]LiquidProps
	liquidPropsByID      map[int16]LiquidProps
	recipeDefs           []RecipeDef
	recipesByBlockName   map[string]CraftRecipe
	craftStates          map[int32]craftState
	drillStates          map[int32]craftState
	blockPropsDefs       []BlockPropsDef
	blockPropsByName     map[string]BlockProps
	blockSizeDefs        []BlockSizeDef
	blockSizesByName     map[string]int
	powerNetByPos        map[int32]*powerNet
	powerStatusByPos     map[int32]float32
	powerStoredByPos     map[int32]float32
	powerRequests        map[int32]float32
	powerLastDt          float32
	blockBuildDefs       []BlockBuildDef
	blockBuildByName     map[string]BlockBuildDef
	blockBuildByID       map[int16]BlockBuildResolved
	blockKindsByName     map[string]BlockKind
	blockKindsByID       map[int16]BlockKind

	typeIO              *protocol.TypeIOContext
	content             *protocol.ContentRegistry
	logicIDs            *logicIDMaps
	logicRuntime        map[int32]*logicRuntime
	logicMemory         map[int32]*logic.MlogCell
	logicUnitFlags      map[int32]float64
	logicProcessorPos   []int32
	logicDisplayBuffers map[int32][]uint64
	logicClientData     []logicClientDataEvent
	logicSyncVars       []logicSyncEvent
}

type BuildPlanOp struct {
	Breaking   bool
	X          int32
	Y          int32
	Rotation   int8
	BlockID    int16
	BuildSpeed float32
}

type pendingBuildState struct {
	Team       TeamID
	BlockID    int16
	Rotation   int8
	Progress   float32
	LastEmit   float32
	Breaking   bool
	MaxHealth  float32
	BuildTime  float32
	BuildCost  float32
	Req        []ItemStack
	Spent      []int32
	Refunded   []int32
	BuildSpeed float32
}

type shieldState struct {
	Shield float32
	Broken bool
}

type EntityEventKind string

const (
	EntityEventRemoved        EntityEventKind = "removed"
	EntityEventBuildPlaced    EntityEventKind = "build_placed"
	EntityEventBuildDestroyed EntityEventKind = "build_destroyed"
	EntityEventBuildHealth    EntityEventKind = "build_health"
	EntityEventBuildItems     EntityEventKind = "build_items"
	EntityEventBuildLiquids   EntityEventKind = "build_liquids"
	EntityEventBulletFired    EntityEventKind = "bullet_fired"
	EntityEventWeather        EntityEventKind = "weather"
)

type EntityEvent struct {
	Kind   EntityEventKind
	Entity RawEntity
	// BuildPos is packed tile position (Point2), not linear tile index.
	BuildPos     int32
	BuildTeam    TeamID
	BuildBlock   int16
	BuildRot     int8
	BuildHP      float32
	BuildItems   []ItemStack
	BuildLiquids []LiquidStack
	Bullet       BulletEvent
	Weather      WeatherEvent
}

func packTilePos(x, y int) int32 {
	return (int32(x)&0xFFFF)<<16 | (int32(y) & 0xFFFF)
}

type BulletEvent struct {
	Team      TeamID
	X         float32
	Y         float32
	Angle     float32
	Damage    float32
	BulletTyp int16
}

type WeatherEvent struct {
	ID        int16
	Name      string
	Intensity float32
	Duration  float32
	WindX     float32
	WindY     float32
}

type simBullet struct {
	ID              int32
	Team            TeamID
	X               float32
	Y               float32
	VX              float32
	VY              float32
	Damage          float32
	LifeSec         float32
	AgeSec          float32
	Radius          float32
	HitUnits        bool
	HitBuilds       bool
	BulletType      int16
	SplashRadius    float32
	SlowSec         float32
	SlowMul         float32
	PierceRemain    int32
	ChainCount      int32
	ChainRange      float32
	FragmentCount   int32
	FragmentSpread  float32
	FragmentSpeed   float32
	FragmentLife    float32
	TargetAir       bool
	TargetGround    bool
	TargetPriority  string
	PreferBuildings bool
}

type weaponProfile struct {
	FireMode        string // projectile|beam
	Range           float32
	Damage          float32
	Interval        float32
	BulletType      int16
	BulletSpeed     float32
	SplashRadius    float32
	SlowSec         float32
	SlowMul         float32
	Pierce          int32
	ChainCount      int32
	ChainRange      float32
	FragmentCount   int32
	FragmentSpread  float32
	FragmentSpeed   float32
	FragmentLife    float32
	BurstShots      int32
	BurstSpacing    float32
	Spread          float32
	PreferBuildings bool
	TargetAir       bool
	TargetGround    bool
	TargetPriority  string
	HitBuildings    bool
}

type buildingWeaponProfile struct {
	FireMode       string // projectile|beam
	Range          float32
	Damage         float32
	Interval       float32
	BulletType     int16
	BulletSpeed    float32
	SplashRadius   float32
	SlowSec        float32
	SlowMul        float32
	Pierce         int32
	ChainCount     int32
	ChainRange     float32
	HitBuildings   bool
	TargetBuilds   bool
	TargetAir      bool
	TargetGround   bool
	TargetPriority string
	MinTargetTeam  TeamID

	AmmoCapacity float32
	AmmoRegen    float32
	AmmoPerShot  float32

	PowerCapacity float32
	PowerRegen    float32
	PowerPerShot  float32

	BurstShots   int32
	BurstSpacing float32
}

type buildCombatState struct {
	Cooldown    float32
	BurstRemain int32
	BurstDelay  float32
	Ammo        float32
	Power       float32
	Reload      float32
	Rotation    float32
	BurstIndex  int32
	TargetID    int32
	RetargetCD  float32
}

type unitBurstState struct {
	BurstRemain int32
	BurstDelay  float32
	BurstIndex  int32
}

type targetTrackState struct {
	TargetID   int32
	RetargetCD float32
}

type craftState struct {
	Progress float32
}

type massDriverState struct {
	Reload float32
}

type unitWeaponMountProfile struct {
	AngleOffset     float32
	CooldownMul     float32
	DamageMul       float32
	RangeMul        float32
	BulletSpeedMul  float32
	BulletType      int16 // -1 means inherit entity bullet type
	SplashRadiusMul float32
}

type vanillaProfilesFile struct {
	Units       []vanillaUnitProfile   `json:"units"`
	UnitsByName []vanillaUnitProfile   `json:"units_by_name"`
	Turrets     []vanillaTurretProfile `json:"turrets"`
}

type vanillaUnitProfile struct {
	Name            string  `json:"name"`
	TypeID          int16   `json:"type_id"`
	FireMode        string  `json:"fire_mode"`
	Range           float32 `json:"range"`
	Damage          float32 `json:"damage"`
	Interval        float32 `json:"interval"`
	BulletType      int16   `json:"bullet_type"`
	BulletSpeed     float32 `json:"bullet_speed"`
	SplashRadius    float32 `json:"splash_radius"`
	SlowSec         float32 `json:"slow_sec"`
	SlowMul         float32 `json:"slow_mul"`
	Pierce          int32   `json:"pierce"`
	ChainCount      int32   `json:"chain_count"`
	ChainRange      float32 `json:"chain_range"`
	FragmentCount   int32   `json:"fragment_count"`
	FragmentSpread  float32 `json:"fragment_spread"`
	FragmentSpeed   float32 `json:"fragment_speed"`
	FragmentLife    float32 `json:"fragment_life"`
	BurstShots      int32   `json:"burst_shots"`
	BurstSpacing    float32 `json:"burst_spacing"`
	Spread          float32 `json:"spread"`
	PreferBuildings bool    `json:"prefer_buildings"`
	TargetAir       bool    `json:"target_air"`
	TargetGround    bool    `json:"target_ground"`
	TargetPriority  string  `json:"target_priority"`
	HitBuildings    bool    `json:"hit_buildings"`
	HitRadius       float32 `json:"hit_radius"`
}

type vanillaTurretProfile struct {
	Name           string  `json:"name"`
	FireMode       string  `json:"fire_mode"`
	Range          float32 `json:"range"`
	Damage         float32 `json:"damage"`
	Interval       float32 `json:"interval"`
	BulletType     int16   `json:"bullet_type"`
	BulletSpeed    float32 `json:"bullet_speed"`
	SplashRadius   float32 `json:"splash_radius"`
	SlowSec        float32 `json:"slow_sec"`
	SlowMul        float32 `json:"slow_mul"`
	Pierce         int32   `json:"pierce"`
	ChainCount     int32   `json:"chain_count"`
	ChainRange     float32 `json:"chain_range"`
	HitBuildings   bool    `json:"hit_buildings"`
	TargetBuilds   bool    `json:"target_builds"`
	TargetAir      bool    `json:"target_air"`
	TargetGround   bool    `json:"target_ground"`
	TargetPriority string  `json:"target_priority"`
	AmmoCapacity   float32 `json:"ammo_capacity"`
	AmmoRegen      float32 `json:"ammo_regen"`
	AmmoPerShot    float32 `json:"ammo_per_shot"`
	PowerCapacity  float32 `json:"power_capacity"`
	PowerRegen     float32 `json:"power_regen"`
	PowerPerShot   float32 `json:"power_per_shot"`
	BurstShots     int32   `json:"burst_shots"`
	BurstSpacing   float32 `json:"burst_spacing"`
}

var defaultWeaponProfile = weaponProfile{
	FireMode:        "projectile",
	Range:           56,
	Damage:          8,
	Interval:        0.7,
	BulletType:      0,
	BulletSpeed:     34,
	SplashRadius:    0,
	SlowSec:         0,
	SlowMul:         1,
	Pierce:          0,
	ChainCount:      0,
	ChainRange:      0,
	FragmentCount:   0,
	FragmentSpread:  0,
	FragmentSpeed:   0,
	FragmentLife:    0,
	BurstShots:      1,
	BurstSpacing:    0,
	Spread:          0,
	PreferBuildings: false,
	TargetAir:       true,
	TargetGround:    true,
	TargetPriority:  "nearest",
	HitBuildings:    true,
}

// Approximate presets by typeId to make combat behavior more varied.
var weaponProfilesByType = map[int16]weaponProfile{
	0:  {FireMode: "projectile", Range: 64, Damage: 10, Interval: 0.60, BulletType: 0, BulletSpeed: 36, TargetAir: true, TargetGround: true, HitBuildings: true},
	1:  {FireMode: "projectile", Range: 72, Damage: 12, Interval: 0.55, BulletType: 1, BulletSpeed: 40, Pierce: 1, TargetAir: true, TargetGround: true, HitBuildings: true},
	2:  {FireMode: "projectile", Range: 88, Damage: 20, Interval: 1.10, BulletType: 2, BulletSpeed: 46, SplashRadius: 14, TargetAir: false, TargetGround: true, HitBuildings: true},
	3:  {FireMode: "projectile", Range: 68, Damage: 9, Interval: 0.40, BulletType: 3, BulletSpeed: 44, TargetAir: true, TargetGround: false, HitBuildings: false},
	4:  {FireMode: "projectile", Range: 76, Damage: 11, Interval: 0.75, BulletType: 4, BulletSpeed: 38, SlowSec: 1.8, SlowMul: 0.65, ChainCount: 2, ChainRange: 28, TargetAir: false, TargetGround: true, HitBuildings: true},
	5:  {FireMode: "beam", Range: 96, Damage: 16, Interval: 0.90, BulletType: 5, BulletSpeed: 52, TargetAir: true, TargetGround: true, HitBuildings: false},
	6:  {FireMode: "projectile", Range: 80, Damage: 14, Interval: 0.80, BulletType: 6, BulletSpeed: 42, SplashRadius: 10, Pierce: 1, TargetAir: false, TargetGround: true, HitBuildings: true},
	7:  {FireMode: "projectile", Range: 120, Damage: 24, Interval: 1.30, BulletType: 7, BulletSpeed: 58, FragmentCount: 3, FragmentSpread: 24, FragmentSpeed: 34, FragmentLife: 0.6, TargetAir: true, TargetGround: true, HitBuildings: true},
	8:  {FireMode: "projectile", Range: 54, Damage: 7, Interval: 0.32, BulletType: 8, BulletSpeed: 36, TargetAir: false, TargetGround: true, HitBuildings: false},
	9:  {FireMode: "projectile", Range: 92, Damage: 15, Interval: 0.95, BulletType: 9, BulletSpeed: 48, SlowSec: 2.2, SlowMul: 0.55, ChainCount: 3, ChainRange: 34, TargetAir: true, TargetGround: true, HitBuildings: true},
	10: {FireMode: "projectile", Range: 66, Damage: 10, Interval: 0.50, BulletType: 10, BulletSpeed: 40, TargetAir: true, TargetGround: true, HitBuildings: true},
	11: {FireMode: "beam", Range: 132, Damage: 28, Interval: 1.35, BulletType: 11, TargetAir: true, TargetGround: true, TargetPriority: "threat", HitBuildings: true},
	12: {FireMode: "projectile", Range: 72, Damage: 13, Interval: 0.70, BulletType: 12, BulletSpeed: 43, PreferBuildings: true, TargetAir: false, TargetGround: true, TargetPriority: "lowest_health", HitBuildings: true},
	13: {FireMode: "projectile", Range: 58, Damage: 8, Interval: 0.30, BulletType: 13, BulletSpeed: 37, Pierce: 2, TargetAir: true, TargetGround: false, HitBuildings: false},
	14: {FireMode: "projectile", Range: 100, Damage: 19, Interval: 1.00, BulletType: 14, BulletSpeed: 50, SplashRadius: 16, PreferBuildings: true, TargetAir: false, TargetGround: true, HitBuildings: true},
	15: {FireMode: "projectile", Range: 84, Damage: 16, Interval: 0.82, BulletType: 15, BulletSpeed: 46, FragmentCount: 4, FragmentSpread: 32, FragmentSpeed: 30, FragmentLife: 0.75, TargetAir: true, TargetGround: true, TargetPriority: "threat", HitBuildings: true},
}

// Vanilla turret block-name profiles (approximate baseline).
var buildingWeaponProfilesByName = map[string]buildingWeaponProfile{
	"duo":        {FireMode: "projectile", Range: 136, Damage: 9, Interval: 0.27, BulletType: 0, BulletSpeed: 54, TargetAir: true, TargetGround: true, HitBuildings: true, AmmoCapacity: 80, AmmoRegen: 3.0, AmmoPerShot: 1, BurstShots: 2, BurstSpacing: 0.06},
	"scatter":    {FireMode: "projectile", Range: 152, Damage: 7, Interval: 0.23, BulletType: 3, BulletSpeed: 57, TargetAir: true, TargetGround: false, HitBuildings: false, AmmoCapacity: 90, AmmoRegen: 2.8, AmmoPerShot: 1, BurstShots: 3, BurstSpacing: 0.04},
	"scorch":     {FireMode: "projectile", Range: 62, Damage: 16, Interval: 0.13, BulletType: 8, BulletSpeed: 42, TargetAir: false, TargetGround: true, HitBuildings: false, AmmoCapacity: 70, AmmoRegen: 2.2, AmmoPerShot: 1},
	"hail":       {FireMode: "projectile", Range: 236, Damage: 24, Interval: 1.20, BulletType: 2, BulletSpeed: 52, SplashRadius: 18, TargetAir: false, TargetGround: true, HitBuildings: true, AmmoCapacity: 36, AmmoRegen: 1.1, AmmoPerShot: 1},
	"wave":       {FireMode: "projectile", Range: 118, Damage: 4, Interval: 0.09, BulletType: 4, BulletSpeed: 38, SlowSec: 1.8, SlowMul: 0.6, TargetAir: false, TargetGround: true, HitBuildings: false},
	"lancer":     {FireMode: "beam", Range: 172, Damage: 96, Interval: 1.35, BulletType: 11, TargetAir: true, TargetGround: true, TargetPriority: "threat", HitBuildings: true, PowerCapacity: 280, PowerRegen: 22, PowerPerShot: 80},
	"arc":        {FireMode: "beam", Range: 88, Damage: 24, Interval: 0.42, BulletType: 5, ChainCount: 2, ChainRange: 32, HitBuildings: true, PowerCapacity: 140, PowerRegen: 16, PowerPerShot: 30},
	"parallax":   {FireMode: "projectile", Range: 292, Damage: 20, Interval: 0.55, BulletType: 6, BulletSpeed: 64, SlowSec: 0.8, SlowMul: 0.75, TargetAir: true, TargetGround: false, HitBuildings: false},
	"swarmer":    {FireMode: "projectile", Range: 216, Damage: 22, Interval: 0.35, BulletType: 7, BulletSpeed: 62, SplashRadius: 12, HitBuildings: true, AmmoCapacity: 55, AmmoRegen: 1.7, AmmoPerShot: 1, BurstShots: 2, BurstSpacing: 0.05},
	"salvo":      {FireMode: "projectile", Range: 188, Damage: 23, Interval: 0.32, BulletType: 1, BulletSpeed: 60, Pierce: 1, HitBuildings: true, AmmoCapacity: 65, AmmoRegen: 2.0, AmmoPerShot: 1, BurstShots: 4, BurstSpacing: 0.045},
	"segment":    {FireMode: "beam", Range: 88, Damage: 26, Interval: 0.16, BulletType: 5, ChainCount: 1, ChainRange: 20, TargetAir: true, TargetGround: false, HitBuildings: false},
	"tsunami":    {FireMode: "projectile", Range: 174, Damage: 10, Interval: 0.08, BulletType: 4, BulletSpeed: 44, SlowSec: 2.8, SlowMul: 0.45, TargetAir: false, TargetGround: true, HitBuildings: false},
	"fuse":       {FireMode: "beam", Range: 120, Damage: 180, Interval: 0.95, BulletType: 11, HitBuildings: true, AmmoCapacity: 45, AmmoRegen: 1.2, AmmoPerShot: 1},
	"ripple":     {FireMode: "projectile", Range: 286, Damage: 62, Interval: 1.35, BulletType: 14, BulletSpeed: 72, SplashRadius: 24, HitBuildings: true, AmmoCapacity: 28, AmmoRegen: 0.9, AmmoPerShot: 1},
	"cyclone":    {FireMode: "projectile", Range: 214, Damage: 18, Interval: 0.10, BulletType: 10, BulletSpeed: 65, HitBuildings: true, AmmoCapacity: 120, AmmoRegen: 4.8, AmmoPerShot: 1},
	"foreshadow": {FireMode: "projectile", Range: 472, Damage: 640, Interval: 4.8, BulletType: 15, BulletSpeed: 94, Pierce: 3, TargetPriority: "highest_health", HitBuildings: true, PowerCapacity: 1800, PowerRegen: 90, PowerPerShot: 900},
	"spectre":    {FireMode: "projectile", Range: 300, Damage: 84, Interval: 0.18, BulletType: 12, BulletSpeed: 82, TargetPriority: "threat", HitBuildings: true, AmmoCapacity: 140, AmmoRegen: 3.4, AmmoPerShot: 1},
	"meltdown":   {FireMode: "beam", Range: 236, Damage: 94, Interval: 0.12, BulletType: 11, SlowSec: 0.7, SlowMul: 0.8, HitBuildings: true, PowerCapacity: 1200, PowerRegen: 120, PowerPerShot: 60},
	"breach":     {FireMode: "projectile", Range: 120, Damage: 25, Interval: 0.22, BulletType: 0, BulletSpeed: 56, HitBuildings: true},
	"diffuse":    {FireMode: "projectile", Range: 152, Damage: 16, Interval: 0.14, BulletType: 3, BulletSpeed: 58, HitBuildings: true},
	"sublimate":  {FireMode: "beam", Range: 156, Damage: 52, Interval: 0.22, BulletType: 5, ChainCount: 2, ChainRange: 28, HitBuildings: true, PowerCapacity: 360, PowerRegen: 28, PowerPerShot: 36},
	"titan":      {FireMode: "projectile", Range: 210, Damage: 38, Interval: 0.36, BulletType: 10, BulletSpeed: 66, HitBuildings: true},
	"disperse":   {FireMode: "projectile", Range: 230, Damage: 36, Interval: 0.28, BulletType: 14, BulletSpeed: 72, SplashRadius: 18, HitBuildings: true},
	"afflict":    {FireMode: "beam", Range: 246, Damage: 128, Interval: 0.24, BulletType: 11, HitBuildings: true, PowerCapacity: 760, PowerRegen: 62, PowerPerShot: 84},
	"lustre":     {FireMode: "beam", Range: 332, Damage: 180, Interval: 0.26, BulletType: 11, ChainCount: 1, ChainRange: 36, HitBuildings: true, PowerCapacity: 980, PowerRegen: 70, PowerPerShot: 100},
	"scathe":     {FireMode: "projectile", Range: 438, Damage: 260, Interval: 1.05, BulletType: 15, BulletSpeed: 84, SplashRadius: 26, HitBuildings: true, TargetBuilds: true, AmmoCapacity: 24, AmmoRegen: 0.55, AmmoPerShot: 1},
	"smite":      {FireMode: "projectile", Range: 352, Damage: 220, Interval: 0.65, BulletType: 15, BulletSpeed: 86, SplashRadius: 20, HitBuildings: true, AmmoCapacity: 28, AmmoRegen: 0.75, AmmoPerShot: 1},
	"malign":     {FireMode: "beam", Range: 402, Damage: 260, Interval: 0.34, BulletType: 11, ChainCount: 2, ChainRange: 44, HitBuildings: true, PowerCapacity: 1400, PowerRegen: 105, PowerPerShot: 140},
}

// Approximate multi-mount presets by unit typeId.
var unitMountProfilesByType = map[int16][]unitWeaponMountProfile{
	3: {
		{AngleOffset: -8, CooldownMul: 1.00, DamageMul: 0.55, RangeMul: 1.0, BulletSpeedMul: 1.0, BulletType: -1, SplashRadiusMul: 1},
		{AngleOffset: 8, CooldownMul: 1.00, DamageMul: 0.55, RangeMul: 1.0, BulletSpeedMul: 1.0, BulletType: -1, SplashRadiusMul: 1},
	},
	6: {
		{AngleOffset: -5, CooldownMul: 0.95, DamageMul: 0.7, RangeMul: 1.0, BulletSpeedMul: 1.0, BulletType: -1, SplashRadiusMul: 1},
		{AngleOffset: 5, CooldownMul: 0.95, DamageMul: 0.7, RangeMul: 1.0, BulletSpeedMul: 1.0, BulletType: -1, SplashRadiusMul: 1},
	},
	7: {
		{AngleOffset: -12, CooldownMul: 1.10, DamageMul: 0.62, RangeMul: 1.0, BulletSpeedMul: 1.0, BulletType: -1, SplashRadiusMul: 1},
		{AngleOffset: 0, CooldownMul: 1.05, DamageMul: 0.72, RangeMul: 1.0, BulletSpeedMul: 1.0, BulletType: -1, SplashRadiusMul: 1},
		{AngleOffset: 12, CooldownMul: 1.10, DamageMul: 0.62, RangeMul: 1.0, BulletSpeedMul: 1.0, BulletType: -1, SplashRadiusMul: 1},
	},
	11: {
		{AngleOffset: 0, CooldownMul: 1.00, DamageMul: 0.7, RangeMul: 1.0, BulletSpeedMul: 1.0, BulletType: -1, SplashRadiusMul: 1},
		{AngleOffset: -18, CooldownMul: 1.25, DamageMul: 0.35, RangeMul: 0.92, BulletSpeedMul: 1.0, BulletType: 10, SplashRadiusMul: 0.7},
		{AngleOffset: 18, CooldownMul: 1.25, DamageMul: 0.35, RangeMul: 0.92, BulletSpeedMul: 1.0, BulletType: 10, SplashRadiusMul: 0.7},
	},
	15: {
		{AngleOffset: -10, CooldownMul: 1.00, DamageMul: 0.52, RangeMul: 1.05, BulletSpeedMul: 1.0, BulletType: -1, SplashRadiusMul: 1},
		{AngleOffset: 10, CooldownMul: 1.00, DamageMul: 0.52, RangeMul: 1.05, BulletSpeedMul: 1.0, BulletType: -1, SplashRadiusMul: 1},
	},
}

var entityHitRadiusByType = map[int16]float32{
	0:  4.0,
	1:  4.5,
	2:  5.0,
	3:  5.2,
	4:  4.6,
	5:  5.4,
	6:  6.0,
	7:  6.6,
	8:  3.8,
	9:  5.8,
	10: 5.0,
	11: 7.0,
	12: 5.6,
	13: 4.2,
	14: 6.4,
	15: 7.4,
}

// Official-ish storage capacities by block name.
// Values are intentionally limited to frequently interacted storage/core blocks
// to keep behavior close without blocking unknown modded blocks.
var buildingItemCapacityByName = map[string]int32{
	"core-shard":           4000,
	"core-foundation":      9000,
	"core-nucleus":         13000,
	"core-bastion":         2000,
	"core-citadel":         3000,
	"core-acropolis":       4000,
	"container":            300,
	"vault":                1000,
	"reinforced-container": 160,
	"reinforced-vault":     900,
	"unloader":             120,
	"bridge-conveyor":      10,
	"duct-bridge":          4,
	"phase-conveyor":       10,
	"conveyor":             1,
	"titanium-conveyor":    1,
	"plastanium-conveyor":  1,
	"surge-conveyor":       10,
	"armored-conveyor":     1,
	"duct":                 1,
	"junction":             24,
	"duct-junction":        4,
	"duct-router":          1,
	"surge-router":         10,
	"router":               1,
	"overflow-gate":        1,
	"underflow-gate":       1,
	"overflow-duct":        1,
	"underflow-duct":       1,
	"duct-unloader":        0,
}

// Block sizes sourced from Mindustry 152.2 (Blocks.java + payload block constructors).
var blockSizeByName = map[string]int{
	"distributor":                 2,
	"mass-driver":                 3,
	"unit-cargo-loader":           3,
	"unit-cargo-unload-point":     2,
	"liquid-container":            2,
	"liquid-tank":                 3,
	"reinforced-liquid-container": 2,
	"reinforced-liquid-tank":      3,
	"core-shard":                  3,
	"core-foundation":             4,
	"core-nucleus":                5,
	"core-bastion":                4,
	"core-citadel":                5,
	"core-acropolis":              6,
	"container":                   2,
	"vault":                       3,
	"reinforced-container":        2,
	"reinforced-vault":            3,
	"payload-conveyor":            3,
	"payload-router":              3,
	"reinforced-payload-conveyor": 3,
	"reinforced-payload-router":   3,
	"payload-mass-driver":         3,
	"large-payload-mass-driver":   5,
	"payload-loader":              3,
	"payload-unloader":            3,
	"power-node-large":            2,
	"surge-tower":                 2,
	"battery-large":               3,
}

func isItemTransportBlockName(name string) bool {
	if name == "" {
		return false
	}
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			if k.Group == "liquid" || k.Group == "payload" {
				return false
			}
			switch k.Kind {
			case "conveyor", "duct", "router", "junction", "bridge", "overflow", "underflow",
				"duct-bridge", "duct-router", "duct-junction", "overflow-duct", "underflow-duct",
				"phase-conveyor", "stack-conveyor", "surge-conveyor", "armored-conveyor", "titanium-conveyor":
				return true
			}
		}
	}
	if strings.Contains(name, "liquid") || strings.Contains(name, "payload") {
		return false
	}
	return strings.Contains(name, "conveyor") ||
		strings.Contains(name, "router") ||
		strings.Contains(name, "distributor") ||
		strings.Contains(name, "junction") ||
		strings.Contains(name, "overflow") ||
		strings.Contains(name, "underflow") ||
		strings.Contains(name, "duct")
}

func transportDefaultCapacity(name string) int32 {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			switch k.Kind {
			case "phase-conveyor", "bridge", "surge-conveyor":
				return 10
			case "duct-bridge":
				return 4
			case "junction":
				return 24
			default:
				return 1
			}
		}
	}
	switch {
	case strings.Contains(name, "phase-conveyor"):
		return 10
	case strings.Contains(name, "bridge-conveyor"):
		return 10
	case strings.Contains(name, "duct-bridge"):
		return 4
	case strings.Contains(name, "surge-router"):
		return 10
	case strings.Contains(name, "junction"):
		return 24
	default:
		return 1
	}
}

func isConveyorName(name string) bool {
	if name == "" {
		return false
	}
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			switch k.Kind {
			case "conveyor", "duct", "phase-conveyor", "stack-conveyor", "surge-conveyor", "armored-conveyor", "titanium-conveyor", "duct-bridge", "bridge":
				return true
			}
		}
	}
	return strings.Contains(name, "conveyor") || strings.Contains(name, "duct")
}

func isBridgeItemName(name string) bool {
	if name == "" {
		return false
	}
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "bridge" || k.Kind == "duct-bridge"
		}
	}
	return strings.Contains(name, "bridge") && (strings.Contains(name, "conveyor") || strings.Contains(name, "duct"))
}

func isDuctBridgeName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "duct-bridge"
		}
	}
	return strings.Contains(name, "duct-bridge")
}

func isJunctionName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "junction"
		}
	}
	return strings.Contains(name, "junction") && !strings.Contains(name, "liquid") && !strings.Contains(name, "duct-junction")
}

func isDuctJunctionName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "duct-junction"
		}
	}
	return strings.Contains(name, "duct-junction")
}

func isDuctRouterName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "duct-router"
		}
	}
	return strings.Contains(name, "duct-router")
}

func isStackRouterName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "surge-router" || k.Kind == "surge-conveyor"
		}
	}
	return strings.Contains(name, "surge-router")
}

func isUnitCargoUnloadPointName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return strings.Contains(strings.ToLower(k.Class), "unload") || strings.Contains(k.Block, "unload")
		}
	}
	return strings.Contains(name, "unit-cargo-unload-point")
}

func isDuctUnloaderName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "duct-unloader"
		}
	}
	return strings.Contains(name, "duct-unloader")
}

func isArmoredConveyorName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			_, ok = k.Flags["armored"]
			return ok && (k.Kind == "conveyor" || k.Kind == "armored-conveyor")
		}
	}
	return strings.Contains(name, "armored-conveyor")
}

func isSurgeConveyorName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			_, ok = k.Flags["surge"]
			return ok && (k.Kind == "surge-conveyor")
		}
	}
	return strings.Contains(name, "surge-conveyor")
}

func isOverflowDuctName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "overflow-duct"
		}
	}
	return strings.Contains(name, "overflow-duct")
}

func isUnderflowDuctName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "underflow-duct"
		}
	}
	return strings.Contains(name, "underflow-duct")
}

func isDuctName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "duct" || k.Kind == "duct-router" || k.Kind == "duct-junction" || k.Kind == "duct-unloader"
		}
	}
	return strings.Contains(name, "duct") && !isBridgeItemName(name) && !isOverflowDuctName(name) && !isUnderflowDuctName(name)
}

func isArmoredDuctName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			_, ok = k.Flags["armored"]
			return ok && (k.Kind == "duct")
		}
	}
	return strings.Contains(name, "armored-duct") || strings.Contains(name, "reinforced-duct")
}

func conveyorCapacity(name string) int {
	if strings.Contains(name, "duct") {
		return 1
	}
	return 3
}

func conveyorItemSpace(name string) float32 {
	_ = name
	return 0.4
}

func isStackConveyorName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "stack-conveyor" || k.Kind == "surge-conveyor"
		}
	}
	return strings.Contains(name, "plastanium-conveyor") || strings.Contains(name, "stack-conveyor") || strings.Contains(name, "surge-conveyor")
}

func stackConveyorSpeed(name string) float32 {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			if k.Speed > 0 {
				return k.Speed
			}
			if k.Kind == "surge-conveyor" {
				return 5.0 / 60.0
			}
		}
	}
	if strings.Contains(name, "surge-conveyor") {
		return 5.0 / 60.0
	}
	return 4.0 / 60.0
}

func stackConveyorRecharge(name string) float32 {
	_ = name
	return 2.0
}

func stackConveyorOutputRouter(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			if k.Kind == "surge-conveyor" {
				return false
			}
			return true
		}
	}
	return !strings.Contains(name, "surge-conveyor")
}

func bridgeUsesBuffer(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "bridge"
		}
	}
	return strings.Contains(name, "bridge-conveyor")
}

func bridgeBufferCapacity(name string) int {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			if k.Kind == "bridge" {
				return 14
			}
			return 0
		}
	}
	if strings.Contains(name, "bridge-conveyor") {
		return 14
	}
	return 0
}

func bridgeBufferDelayTicks(name string) float32 {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			if k.Kind == "bridge" {
				return 74
			}
			return 0
		}
	}
	if strings.Contains(name, "bridge-conveyor") {
		return 74
	}
	return 0
}

func bridgeOutputIntervalTicks(name string) float32 {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			switch k.Kind {
			case "phase-conveyor":
				return 2
			case "duct-bridge":
				return 4
			case "bridge":
				return 4
			default:
				return 4
			}
		}
	}
	switch {
	case strings.Contains(name, "phase-conveyor"):
		return 2
	case strings.Contains(name, "duct-bridge"):
		return 4
	case strings.Contains(name, "bridge-conveyor"):
		return 4
	default:
		return 4
	}
}

func ticksPerSecond(tps int8) float32 {
	if tps <= 0 {
		return 60
	}
	return float32(tps)
}

func junctionSpeedFrames(name string) int {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			if k.Speed > 0 {
				return int(math.Round(float64(1 / k.Speed)))
			}
		}
	}
	return 26
}

func ductJunctionSpeedFrames(name string) int {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			if k.Speed > 0 {
				return int(math.Round(float64(1 / k.Speed)))
			}
		}
	}
	return 5
}

func ductRouterSpeedFrames(name string) int {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			if k.Speed > 0 {
				return int(math.Round(float64(1 / k.Speed)))
			}
		}
	}
	return 5
}

func stackRouterSpeedFrames(name string) int {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			if k.Speed > 0 {
				return int(math.Round(float64(1 / k.Speed)))
			}
		}
	}
	return 6
}

func ductUnloaderSpeedFrames(name string) int {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			if k.Speed > 0 {
				return int(math.Round(float64(1 / k.Speed)))
			}
		}
	}
	return 4
}

func New(cfg Config) *World {
	tps := cfg.TPS
	if tps <= 0 {
		tps = 60
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	w := &World{
		wave:                    1,
		waveTime:                0,
		tick:                    0,
		rand0:                   rng.Int63(),
		rand1:                   rng.Int63(),
		tps:                     int8(tps),
		start:                   time.Now(),
		bulletNextID:            1,
		buildStates:             map[int32]buildCombatState{},
		pendingBuilds:           map[int32]pendingBuildState{},
		unitMountCDs:            map[int32][]float32{},
		unitTargets:             map[int32]targetTrackState{},
		unitBurstStates:         map[int32]unitBurstState{},
		unitMountBursts:         map[int32][]unitBurstState{},
		unitProfilesByType:      cloneUnitWeaponProfiles(weaponProfilesByType),
		unitProfilesByName:      map[string]weaponProfile{},
		buildingProfilesByName:  cloneBuildingWeaponProfiles(buildingWeaponProfilesByName),
		tileConfigValues:        map[int32]any{},
		sorterRouteBits:         map[int32]byte{},
		routerRouteBits:         map[int32]byte{},
		routerInputDirs:         map[int32]int8{},
		junctionInputDirs:       map[int32]int8{},
		unloaderRotations:       map[int32]int16{},
		massDriverStates:        map[int32]massDriverState{},
		payloadRouterRouteBits:  map[int32]byte{},
		payloadRouterInputDirs:  map[int32]int8{},
		payloadMassStates:       map[int32]massDriverState{},
		liquidJunctionInputDirs: map[int32]int8{},
		bridgeProgress:          map[int32]float32{},
		bridgeBuffers:           map[int32][]bridgeBufferItem{},
		junctionStates:          map[int32]*junctionState{},
		ductJunctionStates:      map[int32]*ductJunctionState{},
		ductRouterStates:        map[int32]*ductRouterState{},
		stackRouterStates:       map[int32]*stackRouterState{},
		ductUnloaderStates:      map[int32]*ductUnloaderState{},
		shieldStates:            map[int32]shieldState{},
		mendCharge:              map[int32]float32{},
		overdriveBoostByPos:     map[int32]float32{},
		projectorUse:            map[int32]float32{},
		powerNetByPos:           map[int32]*powerNet{},
		powerStatusByPos:        map[int32]float32{},
		powerStoredByPos:        map[int32]float32{},
		powerRequests:           map[int32]float32{},
		rulesMgr:                NewRulesManager(nil),
		wavesMgr:                NewWaveManager(nil),
		timeMgr:                 NewWorldTime(),
		craftStates:             map[int32]craftState{},
		recipesByBlockName:      map[string]CraftRecipe{},
		drillStates:             map[int32]craftState{},
		blockPropsByName:        map[string]BlockProps{},
		blockSizesByName:        map[string]int{},
		itemPropsByName:         map[string]ItemProps{},
		liquidPropsByName:       map[string]LiquidProps{},
		logicRuntime:            map[int32]*logicRuntime{},
		logicMemory:             map[int32]*logic.MlogCell{},
		logicUnitFlags:          map[int32]float64{},
		logicDisplayBuffers:     map[int32][]uint64{},
	}
	SetDefaultWorld(w)
	return w
}

func (w *World) Step(delta time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tick++
	dt := float32(delta.Seconds())
	if dt > 0 {
		rules := w.rulesMgr.Get()
		wavesEnabled := rules == nil || rules.Waves
		waveTimer := rules == nil || rules.WaveTimer
		if wavesEnabled && waveTimer {
			// Countdown-only model: initialize when empty, then decrement.
			if w.waveTime <= 0 {
				w.waveTime = w.nextWaveSpacingSec()
			}
			w.waveTime -= dt
			if w.waveTime <= 0 {
				w.triggerWave(w.wavesMgr)
				w.waveTime = w.nextWaveSpacingSec()
			}
		}
		w.stepWeatherLocked(dt)
	}

	w.stepPendingBuilds(delta)
	w.stepPower(dt)
	w.stepDefense(dt)
	w.stepExtraction(dt)
	w.stepLiquids(dt)
	w.stepCrafting(dt)
	w.stepLogistics(dt)
	w.stepLogic(dt)
	w.stepEntities(delta)
}

func (w *World) randf() float32 {
	if w == nil {
		return rand.Float32()
	}
	w.rand0 = w.rand0*6364136223846793005 + 1
	return float32((w.rand0>>33)&0xFFFFFFFF) / float32(1<<32)
}

func (w *World) randRange(minV, maxV float32) float32 {
	if maxV <= minV {
		return minV
	}
	return minV + (maxV-minV)*w.randf()
}

func (w *World) stepLogistics(dt float32) {
	if w.model == nil || len(w.model.Tiles) == 0 || w.blockNamesByID == nil {
		return
	}
	tickDelta := dt * ticksPerSecond(w.tps)
	if tickDelta <= 0 {
		return
	}
	doTransport := true
	doAux := true
	doPayload := true
	moves := 0
	const baseMovesPerTick = 44
	maxMovesPerStep := int(math.Ceil(float64(baseMovesPerTick) * float64(tickDelta)))
	if maxMovesPerStep < 1 {
		maxMovesPerStep = 1
	}
	movedPos := map[int32]struct{}{}
	payloadMoved := map[int32]struct{}{}
	for i := range w.model.Tiles {
		if moves >= maxMovesPerStep {
			break
		}
		t := &w.model.Tiles[i]
		if t == nil || t.Build == nil || t.Block == 0 {
			continue
		}
		name := strings.ToLower(strings.TrimSpace(w.blockNamesByID[int16(t.Block)]))
		if name == "" {
			continue
		}
		if strings.Contains(name, "item-void") {
			if len(t.Build.Items) > 0 {
				t.Build.Items = nil
				w.entityEvents = append(w.entityEvents, EntityEvent{
					Kind:       EntityEventBuildItems,
					BuildPos:   packTilePos(t.X, t.Y),
					BuildItems: nil,
				})
			}
			continue
		}
		if strings.Contains(name, "item-source") {
			pos := packTilePos(t.X, t.Y)
			itemID := w.configuredItemIDForBuildLocked(pos)
			if itemID > 0 {
				capacity := int32(1000)
				if props, ok := w.blockPropsByName[name]; ok && props.ItemCapacity > 0 {
					capacity = props.ItemCapacity
				}
				needsUpdate := true
				if len(t.Build.Items) == 1 && t.Build.Items[0].Item == ItemID(itemID) && t.Build.Items[0].Amount == capacity {
					needsUpdate = false
				}
				if needsUpdate {
					t.Build.Items = []ItemStack{{Item: ItemID(itemID), Amount: capacity}}
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   pos,
						BuildItems: append([]ItemStack(nil), t.Build.Items...),
					})
				}
			}
			continue
		}
		if doPayload {
			if k, ok := w.blockKindByName(name); !ok || k.Group != "payload" {
				continue
			}
			_, _, moved, ok := w.stepPayloadLogisticsLocked(t, name, dt, payloadMoved)
			if ok && moved {
				moves++
			}
			continue
		}
		if doAux && strings.Contains(name, "mass-driver") && !strings.Contains(name, "payload") {
			srcPos, dstPos, moved, ok := w.stepMassDriverLocked(t, name, dt)
			if ok && moved {
				if srcTile, ok := w.tileForPosLocked(srcPos); ok && srcTile != nil && srcTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   srcPos,
						BuildItems: append([]ItemStack(nil), srcTile.Build.Items...),
					})
				}
				if dstTile, ok := w.tileForPosLocked(dstPos); ok && dstTile != nil && dstTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   dstPos,
						BuildItems: append([]ItemStack(nil), dstTile.Build.Items...),
					})
				}
				moves++
			}
			continue
		}
		isUnloader := strings.Contains(name, "unloader")
		isSorter := strings.Contains(name, "sorter")
		isBridge := isBridgeItemName(name)
		isStack := isStackConveyorName(name)
		isOverflowDuct := isOverflowDuctName(name) || isUnderflowDuctName(name)
		isDuct := isDuctName(name)
		isJunction := isJunctionName(name)
		isDuctJunction := isDuctJunctionName(name)
		isDuctRouter := isDuctRouterName(name)
		isStackRouter := isStackRouterName(name)
		isDuctUnloader := isDuctUnloaderName(name)
		isUnitCargoUnload := isUnitCargoUnloadPointName(name)
		if doAux && isUnloader {
			srcPos, dstPos, moved, ok := w.stepUnloaderOneLocked(t)
			if ok && moved > 0 {
				if srcTile, ok := w.tileForPosLocked(srcPos); ok && srcTile != nil && srcTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   srcPos,
						BuildItems: append([]ItemStack(nil), srcTile.Build.Items...),
					})
				}
				if dstTile, ok := w.tileForPosLocked(dstPos); ok && dstTile != nil && dstTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   dstPos,
						BuildItems: append([]ItemStack(nil), dstTile.Build.Items...),
					})
				}
				moves++
			}
			continue
		}
		if doTransport && isDuctUnloader {
			if changed, moved, ok := w.stepDuctUnloaderLocked(t, name, dt); ok && moved {
				for _, pos := range changed {
					if tile, ok := w.tileForPosLocked(pos); ok && tile != nil && tile.Build != nil {
						w.entityEvents = append(w.entityEvents, EntityEvent{
							Kind:       EntityEventBuildItems,
							BuildPos:   pos,
							BuildItems: append([]ItemStack(nil), tile.Build.Items...),
						})
					}
				}
				moves++
			}
			continue
		}
		if doAux && isUnitCargoUnload {
			preferred := w.configuredItemIDForBuildLocked(packTilePos(t.X, t.Y))
			if outPos, moved, ok := w.dumpOneFromBuildingLocked(t, preferred); ok && moved > 0 {
				w.entityEvents = append(w.entityEvents, EntityEvent{
					Kind:       EntityEventBuildItems,
					BuildPos:   packTilePos(t.X, t.Y),
					BuildItems: append([]ItemStack(nil), t.Build.Items...),
				})
				if outTile, ok := w.tileForPosLocked(outPos); ok && outTile != nil && outTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   outPos,
						BuildItems: append([]ItemStack(nil), outTile.Build.Items...),
					})
				}
				moves++
			}
			continue
		}
		if doAux && isSorter {
			srcPos, dstPos, moved, ok := w.stepSorterOneLocked(t, name)
			if ok && moved > 0 {
				if srcTile, ok := w.tileForPosLocked(srcPos); ok && srcTile != nil && srcTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   srcPos,
						BuildItems: append([]ItemStack(nil), srcTile.Build.Items...),
					})
				}
				if dstTile, ok := w.tileForPosLocked(dstPos); ok && dstTile != nil && dstTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   dstPos,
						BuildItems: append([]ItemStack(nil), dstTile.Build.Items...),
					})
				}
				moves++
			}
			continue
		}
		if doTransport && isJunction {
			if changed, moved, ok := w.stepJunctionLocked(t, name, dt); ok && moved {
				for _, pos := range changed {
					if tile, ok := w.tileForPosLocked(pos); ok && tile != nil && tile.Build != nil {
						w.entityEvents = append(w.entityEvents, EntityEvent{
							Kind:       EntityEventBuildItems,
							BuildPos:   pos,
							BuildItems: append([]ItemStack(nil), tile.Build.Items...),
						})
					}
				}
				moves++
			}
			continue
		}
		if doTransport && isDuctJunction {
			if changed, moved, ok := w.stepDuctJunctionLocked(t, name, dt); ok && moved {
				for _, pos := range changed {
					if tile, ok := w.tileForPosLocked(pos); ok && tile != nil && tile.Build != nil {
						w.entityEvents = append(w.entityEvents, EntityEvent{
							Kind:       EntityEventBuildItems,
							BuildPos:   pos,
							BuildItems: append([]ItemStack(nil), tile.Build.Items...),
						})
					}
				}
				moves++
			}
			continue
		}
		if doTransport && isDuctRouter {
			if changed, moved, ok := w.stepDuctRouterLocked(t, name, dt); ok && moved {
				for _, pos := range changed {
					if tile, ok := w.tileForPosLocked(pos); ok && tile != nil && tile.Build != nil {
						w.entityEvents = append(w.entityEvents, EntityEvent{
							Kind:       EntityEventBuildItems,
							BuildPos:   pos,
							BuildItems: append([]ItemStack(nil), tile.Build.Items...),
						})
					}
				}
				moves++
			}
			continue
		}
		if doTransport && isStackRouter {
			if changed, moved, ok := w.stepStackRouterLocked(t, name, dt); ok && moved {
				for _, pos := range changed {
					if tile, ok := w.tileForPosLocked(pos); ok && tile != nil && tile.Build != nil {
						w.entityEvents = append(w.entityEvents, EntityEvent{
							Kind:       EntityEventBuildItems,
							BuildPos:   pos,
							BuildItems: append([]ItemStack(nil), tile.Build.Items...),
						})
					}
				}
				moves++
			}
			continue
		}
		if doTransport && isBridge {
			if dt <= 0 {
				continue
			}
			curPos := packTilePos(t.X, t.Y)
			tickDelta := dt * ticksPerSecond(w.tps)
			if tickDelta <= 0 {
				tickDelta = 1
			}
			changed := map[int32]struct{}{}
			mark := func(pos int32) {
				if pos != 0 {
					changed[pos] = struct{}{}
				}
			}
			movedAny := false
			dstTile, dstPos, linkOK := w.bridgeLinkTargetLocked(t, name)
			if !linkOK {
				if isDuctBridgeName(name) {
					if len(t.Build.Items) > 0 {
						itemID := int16(0)
						for _, st := range t.Build.Items {
							if st.Amount > 0 {
								itemID = int16(st.Item)
								break
							}
						}
						if itemID > 0 {
							dir := int(t.Build.Rotation)
							nb := w.nearbyDirLocked(t, dir)
							if nb != nil && nb.Build != nil && nb.Team == t.Team && w.canAcceptItemWithDirLocked(nb, itemID, dir) {
								taken := w.removeBuildingItemLocked(curPos, itemID, 1)
								if taken > 0 {
									added := w.acceptBuildingItemLocked(packTilePos(nb.X, nb.Y), itemID, taken)
									if added < taken {
										_ = w.acceptBuildingItemLocked(curPos, itemID, taken-added)
									}
									if added > 0 {
										movedAny = true
										mark(curPos)
										mark(packTilePos(nb.X, nb.Y))
									}
								}
							}
						}
					}
				} else {
					if outPos, moved, ok := w.bridgeDumpOneLocked(t); ok && moved > 0 {
						movedAny = true
						mark(curPos)
						mark(outPos)
					}
				}
				if movedAny {
					for pos := range changed {
						if tile, ok := w.tileForPosLocked(pos); ok && tile != nil && tile.Build != nil {
							w.entityEvents = append(w.entityEvents, EntityEvent{
								Kind:       EntityEventBuildItems,
								BuildPos:   pos,
								BuildItems: append([]ItemStack(nil), tile.Build.Items...),
							})
						}
					}
					moves++
				}
				continue
			}
			powered := true
			if props, ok := w.blockPropsByName[name]; ok && props.PowerUse > 0 {
				if !w.consumePowerAtLocked(curPos, powerUseAmount(props.PowerUse, dt)) {
					powered = false
				}
			}
			inputBudget := int(math.Floor(float64(tickDelta)))
			if inputBudget < 1 {
				inputBudget = 1
			}
			for i := 0; i < inputBudget; i++ {
				src, _, itemID, ok := w.bridgePullSourceLocked(t, name, dstTile)
				if !ok || src == nil || itemID <= 0 {
					break
				}
				if w.acceptBuildingItemAmountLocked(curPos, itemID, 1) <= 0 {
					break
				}
				taken := w.removeBuildingItemLocked(packTilePos(src.X, src.Y), itemID, 1)
				if taken <= 0 {
					continue
				}
				added := w.acceptBuildingItemLocked(curPos, itemID, taken)
				if added < taken {
					_ = w.acceptBuildingItemLocked(packTilePos(src.X, src.Y), itemID, taken-added)
				}
				if added > 0 {
					movedAny = true
					mark(curPos)
					mark(packTilePos(src.X, src.Y))
				}
			}
			if bridgeUsesBuffer(name) {
				if w.bridgeBuffers == nil {
					w.bridgeBuffers = map[int32][]bridgeBufferItem{}
				}
				buf := w.bridgeBuffers[curPos]
				bufCap := bridgeBufferCapacity(name)
				moveBudget := inputBudget
				if moveBudget < 1 {
					moveBudget = 1
				}
				for i := 0; i < moveBudget && len(buf) < bufCap; i++ {
					itemID := int16(0)
					for _, st := range t.Build.Items {
						if st.Amount > 0 {
							itemID = int16(st.Item)
							break
						}
					}
					if itemID <= 0 {
						break
					}
					taken := w.removeBuildingItemLocked(curPos, itemID, 1)
					if taken <= 0 {
						break
					}
					delay := bridgeBufferDelayTicks(name)
					readyTick := w.tick
					if delay > 0 {
						readyTick += uint64(math.Round(float64(delay)))
					}
					buf = append(buf, bridgeBufferItem{Item: ItemID(itemID), ReadyTick: readyTick})
					movedAny = true
					mark(curPos)
				}
				w.bridgeBuffers[curPos] = buf
			}
			if powered {
				if bridgeUsesBuffer(name) {
					progress := w.bridgeProgress[curPos]
					progress += tickDelta
					interval := bridgeOutputIntervalTicks(name)
					if interval < 1 {
						interval = 1
					}
					if progress >= interval {
						if w.bridgeBuffers != nil {
							buf := w.bridgeBuffers[curPos]
							if len(buf) > 0 && buf[0].ReadyTick <= w.tick {
								itemID := int16(buf[0].Item)
								outDir := -1
								if d, ok := w.bridgeOutputDirLocked(t, name, dstTile); ok {
									outDir = d
								}
								dstName := w.blockNameForTileLocked(dstTile)
								canAccept := false
								if isBridgeItemName(dstName) {
									canAccept = true
								} else if outDir >= 0 && w.canAcceptItemWithDirLocked(dstTile, itemID, outDir) {
									canAccept = true
								}
								if canAccept {
									added := w.acceptBuildingItemLocked(dstPos, itemID, 1)
									if added > 0 {
										buf = buf[1:]
										movedAny = true
										mark(dstPos)
									}
								}
								w.bridgeBuffers[curPos] = buf
							}
						}
						progress -= interval
					}
					w.bridgeProgress[curPos] = progress
				} else {
					progress := w.bridgeProgress[curPos]
					interval := bridgeOutputIntervalTicks(name)
					if interval < 1 {
						interval = 1
					}
					outDir := -1
					if d, ok := w.bridgeOutputDirLocked(t, name, dstTile); ok {
						outDir = d
					}
					if isDuctBridgeName(name) {
						itemID := int16(0)
						for _, st := range t.Build.Items {
							if st.Amount > 0 {
								itemID = int16(st.Item)
								break
							}
						}
						if itemID > 0 && outDir >= 0 && w.canAcceptItemWithDirLocked(dstTile, itemID, outDir) {
							progress += tickDelta
						}
					} else {
						progress += tickDelta
					}
					for progress >= interval {
						itemID := int16(0)
						for _, st := range t.Build.Items {
							if st.Amount > 0 {
								itemID = int16(st.Item)
								break
							}
						}
						if itemID <= 0 {
							break
						}
						dstName := w.blockNameForTileLocked(dstTile)
						canAccept := false
						if isBridgeItemName(dstName) {
							canAccept = true
						} else if outDir >= 0 && w.canAcceptItemWithDirLocked(dstTile, itemID, outDir) {
							canAccept = true
						}
						if canAccept {
							taken := w.removeBuildingItemLocked(curPos, itemID, 1)
							if taken > 0 {
								added := w.acceptBuildingItemLocked(dstPos, itemID, taken)
								if added < taken {
									_ = w.acceptBuildingItemLocked(curPos, itemID, taken-added)
								}
								if added > 0 {
									movedAny = true
									mark(curPos)
									mark(dstPos)
								}
							}
						}
						progress -= interval
					}
					w.bridgeProgress[curPos] = progress
				}
			}
			if movedAny {
				for pos := range changed {
					if tile, ok := w.tileForPosLocked(pos); ok && tile != nil && tile.Build != nil {
						w.entityEvents = append(w.entityEvents, EntityEvent{
							Kind:       EntityEventBuildItems,
							BuildPos:   pos,
							BuildItems: append([]ItemStack(nil), tile.Build.Items...),
						})
					}
				}
				moves++
			}
			continue
		}
		if doTransport && isStack {
			if srcPos, dstPos, moved, ok := w.stepStackConveyorLocked(t, name, dt); ok && moved {
				if srcTile, ok := w.tileForPosLocked(srcPos); ok && srcTile != nil && srcTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   srcPos,
						BuildItems: append([]ItemStack(nil), srcTile.Build.Items...),
					})
				}
				if dstTile, ok := w.tileForPosLocked(dstPos); ok && dstTile != nil && dstTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   dstPos,
						BuildItems: append([]ItemStack(nil), dstTile.Build.Items...),
					})
				}
				moves++
			}
			continue
		}
		if doTransport && isOverflowDuct {
			if srcPos, dstPos, moved, ok := w.stepOverflowDuctLocked(t, name, dt); ok && moved {
				if srcTile, ok := w.tileForPosLocked(srcPos); ok && srcTile != nil && srcTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   srcPos,
						BuildItems: append([]ItemStack(nil), srcTile.Build.Items...),
					})
				}
				if dstTile, ok := w.tileForPosLocked(dstPos); ok && dstTile != nil && dstTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   dstPos,
						BuildItems: append([]ItemStack(nil), dstTile.Build.Items...),
					})
				}
				moves++
			}
			continue
		}
		if doTransport && isDuct {
			if srcPos, dstPos, moved, ok := w.stepDuctLocked(t, name, dt); ok && moved {
				if srcTile, ok := w.tileForPosLocked(srcPos); ok && srcTile != nil && srcTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   srcPos,
						BuildItems: append([]ItemStack(nil), srcTile.Build.Items...),
					})
				}
				if dstTile, ok := w.tileForPosLocked(dstPos); ok && dstTile != nil && dstTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   dstPos,
						BuildItems: append([]ItemStack(nil), dstTile.Build.Items...),
					})
				}
				moves++
			}
			continue
		}
		if doTransport && !isUnloader && !isSorter && !isBridge && isItemTransportBlockName(name) {
			srcPos, dstPos, moved, ok := w.stepTransportOneLocked(t, name, movedPos)
			if ok && moved {
				if srcTile, ok := w.tileForPosLocked(srcPos); ok && srcTile != nil && srcTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   srcPos,
						BuildItems: append([]ItemStack(nil), srcTile.Build.Items...),
					})
				}
				if dstTile, ok := w.tileForPosLocked(dstPos); ok && dstTile != nil && dstTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   dstPos,
						BuildItems: append([]ItemStack(nil), dstTile.Build.Items...),
					})
				}
				moves++
			}
		}
		if doTransport && isConveyorName(name) && !isBridgeItemName(name) {
			if srcPos, dstPos, moved, ok := w.stepConveyorTransitLocked(t, name, dt, movedPos); ok && moved {
				if srcTile, ok := w.tileForPosLocked(srcPos); ok && srcTile != nil && srcTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   srcPos,
						BuildItems: append([]ItemStack(nil), srcTile.Build.Items...),
					})
				}
				if dstTile, ok := w.tileForPosLocked(dstPos); ok && dstTile != nil && dstTile.Build != nil {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   dstPos,
						BuildItems: append([]ItemStack(nil), dstTile.Build.Items...),
					})
				}
				moves++
			}
		}
	}
}

func transportSpeedForName(name string) int {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			switch k.Kind {
			case "phase-conveyor", "stack-conveyor":
				return 3
			case "titanium-conveyor":
				return 2
			case "duct", "duct-router", "duct-junction", "duct-unloader", "duct-bridge":
				return 4
			default:
				return 1
			}
		}
	}
	switch {
	case strings.Contains(name, "phase-conveyor"):
		return 3
	case strings.Contains(name, "plastanium-conveyor"):
		return 3
	case strings.Contains(name, "titanium-conveyor"):
		return 2
	case strings.Contains(name, "duct"):
		return 4
	default:
		return 1
	}
}

func conveyorMoveTimeSec(name string) float32 {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			switch k.Kind {
			case "phase-conveyor":
				return 5.0 / 60.0
			case "stack-conveyor":
				return 6.0 / 60.0
			case "titanium-conveyor":
				return 7.5 / 60.0
			case "duct", "duct-router", "duct-junction", "duct-unloader", "duct-bridge":
				return 5.0 / 60.0
			default:
				return 10.0 / 60.0
			}
		}
	}
	switch {
	case strings.Contains(name, "phase-conveyor"):
		return 5.0 / 60.0
	case strings.Contains(name, "plastanium-conveyor"):
		return 6.0 / 60.0
	case strings.Contains(name, "titanium-conveyor"):
		return 7.5 / 60.0
	case strings.Contains(name, "duct"):
		return 5.0 / 60.0
	default:
		return 10.0 / 60.0
	}
}

func (w *World) stepConveyorTransitLocked(t *Tile, name string, dt float32, moved map[int32]struct{}) (srcPos int32, dstPos int32, movedItem bool, ok bool) {
	if t == nil || t.Build == nil || dt <= 0 {
		return 0, 0, false, false
	}
	if !isConveyorName(name) {
		return 0, 0, false, false
	}
	if isStackConveyorName(name) {
		return 0, 0, false, false
	}
	if isOverflowDuctName(name) || isUnderflowDuctName(name) {
		return 0, 0, false, false
	}
	if isDuctName(name) {
		return 0, 0, false, false
	}
	if isBridgeItemName(name) {
		return 0, 0, false, false
	}
	pos := packTilePos(t.X, t.Y)
	if t.Build.ConvLen == 0 {
		t.Build.ConvMin = 1
	}
	moveTime := conveyorMoveTimeSec(name)
	if moveTime < 0.01 {
		moveTime = 0.01
	}
	itemSpace := conveyorItemSpace(name)
	cap := conveyorCapacity(name)
	if t.Build.ConvLen > cap {
		t.Build.ConvLen = cap
	}
	// Resolve next conveyor linkage.
	next := w.nearbyDirLocked(t, int(t.Build.Rotation))
	var nextConv *Building
	aligned := false
	if next != nil && next.Build != nil && next.Build.Team == t.Build.Team {
		nname := w.blockNameForTileLocked(next)
		if isConveyorName(nname) && !isBridgeItemName(nname) {
			nextConv = next.Build
			aligned = next.Build.Rotation == t.Build.Rotation
		}
	}
	nextMax := float32(1)
	if aligned && nextConv != nil {
		nextMax = 1 - maxf(itemSpace-nextConv.ConvMin, 0)
	}
	movedDist := dt / moveTime
	// Move items forward.
	for i := t.Build.ConvLen - 1; i >= 0; i-- {
		nextpos := float32(100)
		if i < t.Build.ConvLen-1 {
			nextpos = t.Build.ConvPos[i+1] - itemSpace
		}
		maxmove := clampf(nextpos-t.Build.ConvPos[i], 0, movedDist)
		t.Build.ConvPos[i] += maxmove
		if t.Build.ConvPos[i] > nextMax {
			t.Build.ConvPos[i] = nextMax
		}
		if t.Build.ConvPos[i] >= 1 {
			item := t.Build.ConvItems[i]
			if item != 0 {
				dir := int(t.Build.Rotation)
				dstPos := packTilePos(t.X, t.Y)
				movedOK := false
				if next != nil && next.Build != nil && next.Build.Team == t.Build.Team {
					nname := w.blockNameForTileLocked(next)
					if isConveyorName(nname) && !isBridgeItemName(nname) {
						if w.canAcceptItemWithDirLocked(next, int16(item), dir) {
							if w.conveyorAcceptOneLocked(next, item) {
								movedOK = true
								dstPos = packTilePos(next.X, next.Y)
							}
						}
					} else if w.canAcceptItemWithDirLocked(next, int16(item), dir) {
						if w.acceptBuildingItemLocked(packTilePos(next.X, next.Y), int16(item), 1) > 0 {
							movedOK = true
							dstPos = packTilePos(next.X, next.Y)
						}
					}
				}
				if movedOK {
					w.conveyorRemoveIndexLocked(t, i)
					moved[pos] = struct{}{}
					moved[dstPos] = struct{}{}
					return pos, dstPos, true, true
				}
			}
			t.Build.ConvPos[i] = nextMax
		}
	}
	// Update min item.
	minItem := float32(1)
	for i := 0; i < t.Build.ConvLen; i++ {
		if t.Build.ConvPos[i] < minItem {
			minItem = t.Build.ConvPos[i]
		}
	}
	t.Build.ConvMin = minItem
	// Pull from source if space allows.
	src, dir, itemID, ok := w.transportPullSourceLocked(t, name)
	if !ok || src == nil || itemID <= 0 {
		return 0, 0, false, true
	}
	if !w.canAcceptItemWithDirLocked(t, itemID, (dir+2)%4) {
		return 0, 0, false, true
	}
	if t.Build.ConvLen >= cap || t.Build.ConvMin < itemSpace {
		return 0, 0, false, true
	}
	srcPos = packTilePos(src.X, src.Y)
	taken := w.removeBuildingItemLocked(srcPos, itemID, 1)
	if taken <= 0 {
		return 0, 0, false, true
	}
	w.conveyorAcceptOneLocked(t, ItemID(itemID))
	moved[pos] = struct{}{}
	moved[srcPos] = struct{}{}
	return srcPos, pos, true, true
}

func massDriverReloadSec(name string) float32 {
	_ = name
	return 200.0 / 60.0
}

func massDriverRange(name string) float32 {
	_ = name
	return 440.0
}

func (w *World) stepMassDriverLocked(t *Tile, name string, dt float32) (srcPos int32, dstPos int32, moved bool, ok bool) {
	if t == nil || t.Build == nil || dt <= 0 {
		return 0, 0, false, false
	}
	pos := packTilePos(t.X, t.Y)
	state := w.massDriverStates[pos]
	targetPos, hasTarget := w.configuredBuildPosForBuildLocked(pos)
	if !hasTarget || targetPos == pos {
		state.Reload = 0
		w.massDriverStates[pos] = state
		return 0, 0, false, false
	}
	dst, ok := w.tileForPosLocked(targetPos)
	if !ok || dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
		state.Reload = 0
		w.massDriverStates[pos] = state
		return 0, 0, false, false
	}
	rangeWorld := massDriverRange(name)
	dx := float32(dst.X-t.X) * 8
	dy := float32(dst.Y-t.Y) * 8
	if rangeWorld > 0 && dx*dx+dy*dy > rangeWorld*rangeWorld {
		state.Reload = 0
		w.massDriverStates[pos] = state
		return 0, 0, false, false
	}
	if len(t.Build.Items) == 0 {
		state.Reload = 0
		w.massDriverStates[pos] = state
		return 0, 0, false, true
	}
	if w.blockNamesByID != nil {
		if props, ok := w.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]; ok && props.PowerUse > 0 {
			if !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
				return 0, 0, false, false
			}
		}
	}
	reloadSec := massDriverReloadSec(name)
	if reloadSec < 0.05 {
		reloadSec = 0.05
	}
	if state.Reload < reloadSec {
		state.Reload += dt
		w.massDriverStates[pos] = state
		return 0, 0, false, true
	}
	movedAny := false
	for _, st := range append([]ItemStack(nil), t.Build.Items...) {
		if st.Amount <= 0 {
			continue
		}
		accepted := w.acceptBuildingItemLocked(targetPos, int16(st.Item), st.Amount)
		if accepted > 0 {
			_ = w.removeBuildingItemLocked(pos, int16(st.Item), accepted)
			movedAny = true
		}
	}
	if movedAny {
		state.Reload = 0
	} else {
		state.Reload = reloadSec
	}
	w.massDriverStates[pos] = state
	return pos, targetPos, movedAny, true
}

func (w *World) stepTransportOneLocked(t *Tile, name string, moved map[int32]struct{}) (srcPos int32, dstPos int32, movedItem bool, ok bool) {
	if t == nil || t.Build == nil || name == "" {
		return 0, 0, false, false
	}
	if isConveyorName(name) {
		return 0, 0, false, false
	}
	pos := packTilePos(t.X, t.Y)
	if _, exists := moved[pos]; exists {
		return 0, 0, false, false
	}
	isRouter := (strings.Contains(name, "router") || strings.Contains(name, "distributor")) && !strings.Contains(name, "liquid") && !isDuctRouterName(name) && !isStackRouterName(name)
	isJunction := isJunctionName(name)
	isOverflow := strings.Contains(name, "overflow") && !strings.Contains(name, "liquid")
	isUnderflow := strings.Contains(name, "underflow") && !strings.Contains(name, "liquid")
	isConveyor := strings.Contains(name, "conveyor") || strings.Contains(name, "duct")
	if isDuctJunctionName(name) {
		return 0, 0, false, false
	}
	speed := transportSpeedForName(name)
	if speed < 1 {
		speed = 1
	}
	for i := 0; i < speed; i++ {
		if len(t.Build.Items) > 0 {
			itemID := firstItemID(t.Build)
			if itemID <= 0 {
				break
			}
			dir := -1
			var dst *Tile
			if isRouter {
				if d, ok := w.routerOutputDirLocked(t, name, itemID); ok {
					dir = d
					dst = w.nearbyDirLocked(t, dir)
				}
			} else if isJunction {
				if d, ok := w.junctionOutputDirLocked(t, name); ok {
					dir = d
					dst = w.nearbyDirLocked(t, dir)
				}
			} else if isOverflow {
				if d, ok := w.overflowOutputDirLocked(t, itemID); ok {
					dir = d
					dst = w.nearbyDirLocked(t, dir)
				}
			} else if isUnderflow {
				if d, ok := w.underflowOutputDirLocked(t, itemID); ok {
					dir = d
					dst = w.nearbyDirLocked(t, dir)
				}
			} else if isConveyor {
				dir = int(t.Build.Rotation)
				dst = w.nearbyDirLocked(t, dir)
			}
			if dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
				return 0, 0, false, false
			}
			if !w.canAcceptItemWithDirLocked(dst, itemID, dir) {
				return 0, 0, false, false
			}
			if movedOK := w.moveOneItemLocked(t, dst, itemID); movedOK {
				moved[pos] = struct{}{}
				moved[packTilePos(dst.X, dst.Y)] = struct{}{}
				if isJunction && len(t.Build.Items) == 0 {
					delete(w.junctionInputDirs, pos)
				}
				return pos, packTilePos(dst.X, dst.Y), true, true
			}
			return 0, 0, false, false
		}
		if isRouter || isJunction || isOverflow || isUnderflow {
			if src, dir, itemID, ok := w.transportPullSourceLocked(t, name); ok && src != nil && itemID > 0 {
				if !w.canAcceptItemWithDirLocked(t, itemID, (dir+2)%4) {
					return 0, 0, false, false
				}
				srcPos := packTilePos(src.X, src.Y)
				if movedOK := w.moveOneItemLocked(src, t, itemID); movedOK {
					moved[pos] = struct{}{}
					moved[srcPos] = struct{}{}
					if isRouter {
						if w.routerInputDirs == nil {
							w.routerInputDirs = map[int32]int8{}
						}
						w.routerInputDirs[pos] = int8(dir)
					}
					if isJunction {
						if w.junctionInputDirs == nil {
							w.junctionInputDirs = map[int32]int8{}
						}
						w.junctionInputDirs[pos] = int8(dir)
					}
					return srcPos, pos, true, true
				}
				return 0, 0, false, false
			}
		}
	}
	return 0, 0, false, false
}

func (w *World) transportPullSourceLocked(t *Tile, name string) (*Tile, int, int16, bool) {
	if t == nil || t.Build == nil {
		return nil, 0, 0, false
	}
	rot := int(t.Build.Rotation)
	back := (rot + 2) % 4
	left := (rot + 3) % 4
	right := (rot + 1) % 4
	inputDirs := []int{back, left, right}
	if strings.Contains(name, "router") || strings.Contains(name, "junction") {
		inputDirs = []int{0, 1, 2, 3}
	}
	for _, dir := range inputDirs {
		src := w.nearbyDirLocked(t, dir)
		if src == nil || src.Build == nil || src.Build.Team != t.Build.Team {
			continue
		}
		itemID := firstItemID(src.Build)
		if itemID <= 0 {
			continue
		}
		if !w.transportSourceCanOutputLocked(src, dir) {
			continue
		}
		return src, dir, itemID, true
	}
	return nil, 0, 0, false
}

func (w *World) routerOutputDirLocked(t *Tile, name string, itemID int16) (int, bool) {
	if t == nil || t.Build == nil {
		return 0, false
	}
	pos := packTilePos(t.X, t.Y)
	last := byte(0)
	if w.routerRouteBits != nil {
		last = w.routerRouteBits[pos]
	}
	avoid := -1
	if w.routerInputDirs != nil {
		if dir, ok := w.routerInputDirs[pos]; ok {
			avoid = (int(dir) + 2) % 4
		}
	}
	start := (int(last) + 1) % 4
	for i := 0; i < 4; i++ {
		dir := (start + i) % 4
		if dir == avoid {
			continue
		}
		dst := w.nearbyDirLocked(t, dir)
		if dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			continue
		}
		if !w.canAcceptItemLocked(dst, itemID) {
			continue
		}
		if w.routerRouteBits == nil {
			w.routerRouteBits = map[int32]byte{}
		}
		w.routerRouteBits[pos] = byte(dir)
		return dir, true
	}
	return 0, false
}

func (w *World) junctionOutputDirLocked(t *Tile, name string) (int, bool) {
	if t == nil || t.Build == nil {
		return 0, false
	}
	pos := packTilePos(t.X, t.Y)
	if w.junctionInputDirs != nil {
		if dir, ok := w.junctionInputDirs[pos]; ok {
			return int(dir), true
		}
	}
	return int(t.Build.Rotation), true
}

func (w *World) overflowOutputDirLocked(t *Tile, itemID int16) (int, bool) {
	if t == nil || t.Build == nil {
		return 0, false
	}
	rot := int(t.Build.Rotation)
	forward := rot
	left := (rot + 3) % 4
	right := (rot + 1) % 4
	if dst := w.nearbyDirLocked(t, forward); dst != nil && dst.Build != nil && dst.Build.Team == t.Build.Team && w.canAcceptItemLocked(dst, itemID) {
		return forward, true
	}
	pos := packTilePos(t.X, t.Y)
	order := []int{left, right}
	if w.routerRouteBits != nil && (w.routerRouteBits[pos]&1) == 1 {
		order = []int{right, left}
	}
	for _, dir := range order {
		dst := w.nearbyDirLocked(t, dir)
		if dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			continue
		}
		if w.canAcceptItemLocked(dst, itemID) {
			if w.routerRouteBits == nil {
				w.routerRouteBits = map[int32]byte{}
			}
			w.routerRouteBits[pos] ^= 1
			return dir, true
		}
	}
	return 0, false
}

func (w *World) underflowOutputDirLocked(t *Tile, itemID int16) (int, bool) {
	if t == nil || t.Build == nil {
		return 0, false
	}
	rot := int(t.Build.Rotation)
	forward := rot
	left := (rot + 3) % 4
	right := (rot + 1) % 4
	pos := packTilePos(t.X, t.Y)
	order := []int{left, right}
	if w.routerRouteBits != nil && (w.routerRouteBits[pos]&1) == 1 {
		order = []int{right, left}
	}
	for _, dir := range order {
		dst := w.nearbyDirLocked(t, dir)
		if dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			continue
		}
		if w.canAcceptItemLocked(dst, itemID) {
			if w.routerRouteBits == nil {
				w.routerRouteBits = map[int32]byte{}
			}
			w.routerRouteBits[pos] ^= 1
			return dir, true
		}
	}
	if dst := w.nearbyDirLocked(t, forward); dst != nil && dst.Build != nil && dst.Build.Team == t.Build.Team && w.canAcceptItemLocked(dst, itemID) {
		return forward, true
	}
	return 0, false
}

func (w *World) canAcceptItemLocked(t *Tile, itemID int16) bool {
	if t == nil || t.Build == nil || itemID <= 0 {
		return false
	}
	return w.acceptBuildingItemAmountLocked(packTilePos(t.X, t.Y), itemID, 1) > 0
}

func (w *World) canAcceptItemWithDirLocked(t *Tile, itemID int16, dirFromSrcToDst int) bool {
	if !w.canAcceptItemLocked(t, itemID) {
		return false
	}
	name := ""
	if w.blockNamesByID != nil {
		name = strings.ToLower(strings.TrimSpace(w.blockNamesByID[int16(t.Block)]))
	}
	if name == "" {
		return true
	}
	if isOverflowDuctName(name) || isUnderflowDuctName(name) {
		incoming := (dirFromSrcToDst + 2) % 4
		if incoming != int(t.Build.Rotation) {
			return false
		}
		if t.Build.OverflowCurrent != 0 || len(t.Build.Items) > 0 {
			return false
		}
		return true
	}
	if isJunctionName(name) {
		incoming := (dirFromSrcToDst + 2) % 4
		if dst := w.nearbyDirLocked(t, incoming); dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			return false
		}
		return w.junctionLaneCanAcceptLocked(t, incoming)
	}
	if isDuctJunctionName(name) {
		incoming := (dirFromSrcToDst + 2) % 4
		if dst := w.nearbyDirLocked(t, incoming); dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			return false
		}
		return w.ductJunctionCanAcceptLocked(t, incoming)
	}
	if isDuctRouterName(name) || isStackRouterName(name) {
		incoming := (dirFromSrcToDst + 2) % 4
		if incoming != int(t.Build.Rotation) {
			return false
		}
		if isStackRouterName(name) {
			state := w.stackRouterStateLocked(packTilePos(t.X, t.Y))
			if state.unloading {
				return false
			}
			capacity := int32(10)
			if props, ok := w.blockPropsByName[name]; ok && props.ItemCapacity > 0 {
				capacity = props.ItemCapacity
			}
			total := int32(0)
			for i := range t.Build.Items {
				total += t.Build.Items[i].Amount
			}
			if total >= capacity {
				return false
			}
			if state.current != 0 && ItemID(itemID) != state.current {
				return false
			}
			return true
		}
		return len(t.Build.Items) == 0
	}
	if isDuctUnloaderName(name) {
		return false
	}
	if isDuctName(name) {
		incoming := (dirFromSrcToDst + 2) % 4
		if incoming == int(t.Build.Rotation) {
			return false
		}
		if t.Build.DuctCurrent != 0 || len(t.Build.Items) > 0 {
			return false
		}
		if isArmoredDuctName(name) {
			if incoming != int(t.Build.Rotation) {
				// armored ducts only accept from back or from another duct pointing into it
				return true
			}
		}
		return true
	}
	if isBridgeItemName(name) {
		dst, _, ok := w.bridgeLinkTargetLocked(t, name)
		if !ok {
			return false
		}
		if isDuctBridgeName(name) {
			incoming := (dirFromSrcToDst + 2) % 4
			if incoming == int(t.Build.Rotation) {
				return false
			}
			return true
		}
		if outDir, ok := w.bridgeOutputDirLocked(t, name, dst); ok {
			incoming := (dirFromSrcToDst + 2) % 4
			if incoming == outDir {
				return false
			}
		}
		return true
	}
	if isStackConveyorName(name) {
		state := w.stackConveyorStateLocked(t, name)
		if state != stackStateLoad {
			return false
		}
		recharge := stackConveyorRecharge(name)
		if t.Build.StackCooldown > recharge-1 {
			return false
		}
		incoming := (dirFromSrcToDst + 2) % 4
		if incoming == int(t.Build.Rotation) {
			return false
		}
		if len(t.Build.Items) > 0 {
			if t.Build.Items[0].Item != ItemID(itemID) {
				return false
			}
		}
		return true
	}
	if isConveyorName(name) && !isBridgeItemName(name) {
		incoming := (dirFromSrcToDst + 2) % 4
		rot := int(t.Build.Rotation)
		diff := (incoming - rot + 4) % 4
		if isArmoredConveyorName(name) && diff != 2 {
			return false
		}
		minItem := t.Build.ConvMin
		space := conveyorItemSpace(name)
		// from front (diff==0) requires space at start
		if diff == 0 && minItem < space {
			return false
		}
		// from side requires further free distance
		if diff%2 == 1 && minItem <= 0.7 {
			return false
		}
	}
	return true
}

func (w *World) transportSourceCanOutputLocked(src *Tile, dirToDst int) bool {
	if src == nil || src.Build == nil {
		return false
	}
	name := ""
	if w.blockNamesByID != nil {
		name = strings.ToLower(strings.TrimSpace(w.blockNamesByID[int16(src.Block)]))
	}
	if name == "" {
		return true
	}
	if strings.Contains(name, "conveyor") || strings.Contains(name, "duct") {
		return int(src.Build.Rotation) == dirToDst
	}
	return true
}

func (w *World) moveOneItemLocked(src *Tile, dst *Tile, itemID int16) bool {
	if src == nil || dst == nil || itemID <= 0 {
		return false
	}
	srcPos := packTilePos(src.X, src.Y)
	dstPos := packTilePos(dst.X, dst.Y)
	taken := w.removeBuildingItemLocked(srcPos, itemID, 1)
	if taken <= 0 {
		return false
	}
	added := w.acceptBuildingItemLocked(dstPos, itemID, taken)
	if added < taken {
		_ = w.acceptBuildingItemLocked(srcPos, itemID, taken-added)
	}
	return added > 0
}

func firstItemID(b *Building) int16 {
	if b == nil {
		return 0
	}
	for _, st := range b.Items {
		if st.Amount > 0 {
			return int16(st.Item)
		}
	}
	return 0
}

func payloadMoveTimeSec(name string) float32 {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok && k.Group == "payload" {
			if strings.Contains(k.Block, "reinforced") {
				return 35.0 / 60.0
			}
			return 1.0
		}
	}
	if strings.Contains(name, "reinforced-payload") {
		return 35.0 / 60.0
	}
	return 1.0
}

func payloadMassDriverReloadSec(name string) float32 {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok && k.Group == "payload" {
			if _, ok := k.Flags["large"]; ok {
				return 130.0 / 60.0
			}
			return 130.0 / 60.0
		}
	}
	return 130.0 / 60.0
}

func payloadMassDriverChargeSec(name string) float32 {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok && k.Group == "payload" {
			if _, ok := k.Flags["large"]; ok {
				return 100.0 / 60.0
			}
			return 90.0 / 60.0
		}
	}
	if strings.Contains(name, "large-payload-mass-driver") {
		return 100.0 / 60.0
	}
	return 90.0 / 60.0
}

func payloadMassDriverRange(name string) float32 {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok && k.Group == "payload" {
			if _, ok := k.Flags["large"]; ok {
				return 2100.0
			}
			return 700.0
		}
	}
	if strings.Contains(name, "large-payload-mass-driver") {
		return 2100.0
	}
	return 700.0
}

func (w *World) stepPayloadLogisticsLocked(t *Tile, name string, dt float32, moved map[int32]struct{}) (srcPos int32, dstPos int32, movedPayload bool, ok bool) {
	if t == nil || t.Build == nil || dt <= 0 {
		return 0, 0, false, false
	}
	pos := packTilePos(t.X, t.Y)
	if _, exists := moved[pos]; exists {
		return 0, 0, false, false
	}
	if k, ok := w.blockKindByName(name); ok && k.Kind == "payload-void" || strings.Contains(name, "payload-void") {
		if len(t.Build.Payload) > 0 {
			t.Build.Payload = nil
			return pos, pos, true, true
		}
		return 0, 0, false, true
	}
	if k, ok := w.blockKindByName(name); ok && k.Kind == "payload-source" || strings.Contains(name, "payload-source") {
		// Source has infinite payload only when preconfigured; otherwise no-op.
		if len(t.Build.Payload) == 0 {
			_ = w.payloadSourceEnsurePayloadLocked(t)
		}
		return 0, 0, false, true
	}
	if k, ok := w.blockKindByName(name); ok && k.Kind == "payload-mass-driver" || strings.Contains(name, "payload-mass-driver") {
		return w.stepPayloadMassDriverLocked(t, name, dt)
	}
	if k, ok := w.blockKindByName(name); ok && k.Kind == "payload-loader" || strings.Contains(name, "payload-loader") {
		return w.stepPayloadLoaderLocked(t, name, dt, moved)
	}
	if k, ok := w.blockKindByName(name); ok && k.Kind == "payload-unloader" || strings.Contains(name, "payload-unloader") {
		return w.stepPayloadUnloaderLocked(t, name, dt, moved)
	}
	isRouter := false
	isConveyor := false
	if k, ok := w.blockKindByName(name); ok && k.Group == "payload" {
		isRouter = k.Kind == "payload-router"
		isConveyor = k.Kind == "payload-conveyor"
	} else {
		isRouter = strings.Contains(name, "payload-router")
		isConveyor = strings.Contains(name, "payload-conveyor")
	}
	if !isRouter && !isConveyor {
		return 0, 0, false, true
	}
	moveTime := payloadMoveTimeSec(name)
	if moveTime < 0.02 {
		moveTime = 0.02
	}
	state := w.payloadMassStates[pos]
	state.Reload += dt
	if state.Reload < moveTime {
		w.payloadMassStates[pos] = state
		return 0, 0, false, true
	}
	// If we have payload, try to push it.
	if len(t.Build.Payload) > 0 {
		dir := -1
		var dst *Tile
		if isRouter {
			if d, ok := w.payloadRouterOutputDirLocked(t, name); ok {
				dir = d
				dst = w.nearbyDirLocked(t, dir)
			}
		} else {
			dir = int(t.Build.Rotation)
			dst = w.nearbyDirLocked(t, dir)
		}
		if dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			return 0, 0, false, true
		}
		if len(dst.Build.Payload) != 0 {
			return 0, 0, false, true
		}
		dst.Build.Payload = append([]byte(nil), t.Build.Payload...)
		t.Build.Payload = nil
		state.Reload = 0
		w.payloadMassStates[pos] = state
		moved[pos] = struct{}{}
		moved[packTilePos(dst.X, dst.Y)] = struct{}{}
		return pos, packTilePos(dst.X, dst.Y), true, true
	}
	// Pull payload from neighbors when empty.
	if src, dir, ok := w.payloadPullSourceLocked(t, name); ok && src != nil {
		if len(t.Build.Payload) == 0 && len(src.Build.Payload) > 0 {
			t.Build.Payload = append([]byte(nil), src.Build.Payload...)
			src.Build.Payload = nil
			if isRouter {
				if w.payloadRouterInputDirs == nil {
					w.payloadRouterInputDirs = map[int32]int8{}
				}
				w.payloadRouterInputDirs[pos] = int8(dir)
			}
			state.Reload = 0
			w.payloadMassStates[pos] = state
			moved[pos] = struct{}{}
			moved[packTilePos(src.X, src.Y)] = struct{}{}
			return packTilePos(src.X, src.Y), pos, true, true
		}
	}
	w.payloadMassStates[pos] = state
	return 0, 0, false, true
}

func (w *World) stepPayloadLoaderLocked(t *Tile, name string, dt float32, moved map[int32]struct{}) (int32, int32, bool, bool) {
	if t == nil || t.Build == nil || dt <= 0 {
		return 0, 0, false, false
	}
	pos := packTilePos(t.X, t.Y)
	if _, exists := moved[pos]; exists {
		return 0, 0, false, false
	}
	if w.blockNamesByID != nil {
		if props, ok := w.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]; ok && props.PowerUse > 0 {
			if !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
				return 0, 0, false, false
			}
		}
	}
	if len(t.Build.Payload) == 0 {
		if src := w.payloadAdjacentProviderLocked(t); src != nil && len(src.Build.Payload) > 0 {
			t.Build.Payload = append([]byte(nil), src.Build.Payload...)
			src.Build.Payload = nil
			moved[pos] = struct{}{}
			moved[packTilePos(src.X, src.Y)] = struct{}{}
			return packTilePos(src.X, src.Y), pos, true, true
		}
	}
	if len(t.Build.Payload) > 0 {
		dir := int(t.Build.Rotation)
		dst := w.nearbyDirLocked(t, dir)
		if dst != nil && dst.Build != nil && dst.Build.Team == t.Build.Team && len(dst.Build.Payload) == 0 {
			dst.Build.Payload = append([]byte(nil), t.Build.Payload...)
			t.Build.Payload = nil
			moved[pos] = struct{}{}
			moved[packTilePos(dst.X, dst.Y)] = struct{}{}
			return pos, packTilePos(dst.X, dst.Y), true, true
		}
	}
	return 0, 0, false, true
}

func (w *World) stepPayloadUnloaderLocked(t *Tile, name string, dt float32, moved map[int32]struct{}) (int32, int32, bool, bool) {
	if t == nil || t.Build == nil || dt <= 0 {
		return 0, 0, false, false
	}
	pos := packTilePos(t.X, t.Y)
	if _, exists := moved[pos]; exists {
		return 0, 0, false, false
	}
	if w.blockNamesByID != nil {
		if props, ok := w.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]; ok && props.PowerUse > 0 {
			if !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
				return 0, 0, false, false
			}
		}
	}
	if len(t.Build.Payload) > 0 {
		dir := int(t.Build.Rotation)
		dst := w.nearbyDirLocked(t, dir)
		if dst != nil && dst.Build != nil && dst.Build.Team == t.Build.Team && len(dst.Build.Payload) == 0 && !w.isPayloadBlockLocked(dst) {
			dst.Build.Payload = append([]byte(nil), t.Build.Payload...)
			t.Build.Payload = nil
			moved[pos] = struct{}{}
			moved[packTilePos(dst.X, dst.Y)] = struct{}{}
			return pos, packTilePos(dst.X, dst.Y), true, true
		}
	}
	if len(t.Build.Payload) == 0 {
		if src, dir, ok := w.payloadPullSourceLocked(t, name); ok && src != nil && len(src.Build.Payload) > 0 {
			t.Build.Payload = append([]byte(nil), src.Build.Payload...)
			src.Build.Payload = nil
			if w.payloadRouterInputDirs == nil {
				w.payloadRouterInputDirs = map[int32]int8{}
			}
			w.payloadRouterInputDirs[pos] = int8(dir)
			moved[pos] = struct{}{}
			moved[packTilePos(src.X, src.Y)] = struct{}{}
			return packTilePos(src.X, src.Y), pos, true, true
		}
	}
	return 0, 0, false, true
}

func (w *World) payloadAdjacentProviderLocked(t *Tile) *Tile {
	if t == nil || w.model == nil {
		return nil
	}
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, d := range dirs {
		nx, ny := t.X+d[0], t.Y+d[1]
		if !w.model.InBounds(nx, ny) {
			continue
		}
		nb, err := w.model.TileAt(nx, ny)
		if err != nil || nb == nil || nb.Build == nil || nb.Build.Team != t.Build.Team {
			continue
		}
		if len(nb.Build.Payload) == 0 {
			continue
		}
		if w.isPayloadBlockLocked(nb) {
			continue
		}
		return nb
	}
	return nil
}

func (w *World) isPayloadBlockLocked(t *Tile) bool {
	if t == nil || w.blockNamesByID == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNamesByID[int16(t.Block)]))
	if k, ok := w.blockKindByName(name); ok {
		return k.Group == "payload"
	}
	return strings.Contains(name, "payload")
}

func (w *World) payloadPullSourceLocked(t *Tile, name string) (*Tile, int, bool) {
	if t == nil || t.Build == nil {
		return nil, 0, false
	}
	rot := int(t.Build.Rotation)
	back := (rot + 2) % 4
	left := (rot + 3) % 4
	right := (rot + 1) % 4
	inputDirs := []int{back, left, right}
	if k, ok := w.blockKindByName(name); ok && k.Kind == "payload-router" {
		inputDirs = []int{0, 1, 2, 3}
	}
	for _, dir := range inputDirs {
		src := w.nearbyDirLocked(t, dir)
		if src == nil || src.Build == nil || src.Build.Team != t.Build.Team {
			continue
		}
		if len(src.Build.Payload) == 0 {
			continue
		}
		if !w.payloadSourceCanOutputLocked(src, dir) {
			continue
		}
		return src, dir, true
	}
	return nil, 0, false
}

func (w *World) payloadRouterOutputDirLocked(t *Tile, name string) (int, bool) {
	if t == nil || t.Build == nil {
		return 0, false
	}
	pos := packTilePos(t.X, t.Y)
	last := byte(0)
	if w.payloadRouterRouteBits != nil {
		last = w.payloadRouterRouteBits[pos]
	}
	avoid := -1
	if w.payloadRouterInputDirs != nil {
		if dir, ok := w.payloadRouterInputDirs[pos]; ok {
			avoid = (int(dir) + 2) % 4
		}
	}
	start := (int(last) + 1) % 4
	for i := 0; i < 4; i++ {
		dir := (start + i) % 4
		if dir == avoid {
			continue
		}
		dst := w.nearbyDirLocked(t, dir)
		if dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			continue
		}
		if len(dst.Build.Payload) != 0 {
			continue
		}
		if w.payloadRouterRouteBits == nil {
			w.payloadRouterRouteBits = map[int32]byte{}
		}
		w.payloadRouterRouteBits[pos] = byte(dir)
		return dir, true
	}
	return 0, false
}

func (w *World) payloadSourceCanOutputLocked(src *Tile, dirToDst int) bool {
	if src == nil || src.Build == nil {
		return false
	}
	name := ""
	if w.blockNamesByID != nil {
		name = strings.ToLower(strings.TrimSpace(w.blockNamesByID[int16(src.Block)]))
	}
	if name == "" {
		return true
	}
	if k, ok := w.blockKindByName(name); ok && k.Kind == "payload-conveyor" {
		return int(src.Build.Rotation) == dirToDst
	}
	return true
}

func (w *World) stepPayloadMassDriverLocked(t *Tile, name string, dt float32) (srcPos int32, dstPos int32, moved bool, ok bool) {
	if t == nil || t.Build == nil {
		return 0, 0, false, false
	}
	pos := packTilePos(t.X, t.Y)
	state := w.payloadMassStates[pos]
	targetPos, hasTarget := w.configuredBuildPosForBuildLocked(pos)
	if !hasTarget || targetPos == pos {
		state.Reload = 0
		w.payloadMassStates[pos] = state
		return 0, 0, false, false
	}
	dst, ok := w.tileForPosLocked(targetPos)
	if !ok || dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
		state.Reload = 0
		w.payloadMassStates[pos] = state
		return 0, 0, false, false
	}
	rangeWorld := payloadMassDriverRange(name)
	dx := float32(dst.X-t.X) * 8
	dy := float32(dst.Y-t.Y) * 8
	if rangeWorld > 0 && dx*dx+dy*dy > rangeWorld*rangeWorld {
		state.Reload = 0
		w.payloadMassStates[pos] = state
		return 0, 0, false, false
	}
	if len(t.Build.Payload) == 0 {
		state.Reload = 0
		w.payloadMassStates[pos] = state
		return 0, 0, false, true
	}
	if len(dst.Build.Payload) != 0 {
		return 0, 0, false, true
	}
	if w.blockNamesByID != nil {
		if props, ok := w.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]; ok && props.PowerUse > 0 {
			if !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
				return 0, 0, false, false
			}
		}
	}
	wait := payloadMassDriverReloadSec(name) + payloadMassDriverChargeSec(name)
	if wait < 0.05 {
		wait = 0.05
	}
	state.Reload += dt
	if state.Reload < wait {
		w.payloadMassStates[pos] = state
		return 0, 0, false, true
	}
	dst.Build.Payload = append([]byte(nil), t.Build.Payload...)
	t.Build.Payload = nil
	state.Reload = 0
	w.payloadMassStates[pos] = state
	return pos, targetPos, true, true
}

type powerNet struct {
	Energy   float32
	Capacity float32
	Produced float32
	Consumed float32
}

func powerUseAmount(use, dt float32) float32 {
	if use <= 0 || dt <= 0 {
		return 0
	}
	return use * dt * 60
}

func isPowerBlock(name string, props BlockProps) bool {
	if props.PowerCapacity > 0 || props.PowerProduction > 0 || props.PowerUse > 0 {
		return true
	}
	if props.LinkRangeTiles > 0 {
		return true
	}
	n := strings.ToLower(strings.TrimSpace(name))
	switch {
	case strings.Contains(n, "power-node"), strings.Contains(n, "surge-tower"), strings.Contains(n, "beam-link"),
		strings.Contains(n, "power-diode"):
		return true
	}
	return false
}

func (w *World) stepPower(dt float32) {
	if w.model == nil || w.blockNamesByID == nil || w.blockPropsByName == nil || dt <= 0 {
		return
	}
	w.powerLastDt = dt
	if w.tick%60 == 0 {
		w.stepPowerAutoLinksLocked()
	}
	requests := w.powerRequests
	w.powerRequests = map[int32]float32{}
	powerNetByPos := map[int32]*powerNet{}
	visited := map[int32]bool{}
	if w.powerStatusByPos == nil {
		w.powerStatusByPos = map[int32]float32{}
	}
	if w.powerStoredByPos == nil {
		w.powerStoredByPos = map[int32]float32{}
	}
	prevStored := w.powerStoredByPos
	w.powerStoredByPos = map[int32]float32{}
	for k := range w.powerStatusByPos {
		delete(w.powerStatusByPos, k)
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Block == 0 || t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		name, ok := w.blockNamesByID[int16(t.Block)]
		if !ok {
			continue
		}
		n := strings.ToLower(strings.TrimSpace(name))
		props, ok := w.blockPropsByName[n]
		if !ok || !isPowerBlock(n, props) {
			continue
		}
		pos := packTilePos(t.X, t.Y)
		if visited[pos] {
			continue
		}
		team := t.Build.Team
		net := &powerNet{}
		netPositions := make([]int32, 0, 16)
		netBatteries := make([]int32, 0, 8)
		netConsumers := make([]int32, 0, 16)
		netProducers := make([]int32, 0, 8)
		var netNeeded float32
		var netProducedTick float32
		queue := []int32{pos}
		for len(queue) > 0 {
			cur := queue[0]
			queue = queue[1:]
			if visited[cur] {
				continue
			}
			visited[cur] = true
			x, y := unpackPos(cur)
			if !w.model.InBounds(x, y) {
				continue
			}
			ct, err := w.model.TileAt(x, y)
			if err != nil || ct == nil || ct.Block == 0 || ct.Build == nil || ct.Build.Health <= 0 {
				continue
			}
			if ct.Build.Team != team {
				continue
			}
			cname, ok := w.blockNamesByID[int16(ct.Block)]
			if !ok {
				continue
			}
			cn := strings.ToLower(strings.TrimSpace(cname))
			cprops, ok := w.blockPropsByName[cn]
			if !ok || !isPowerBlock(cn, cprops) {
				continue
			}
			cpos := packTilePos(ct.X, ct.Y)
			powerNetByPos[cpos] = net
			netPositions = append(netPositions, cpos)
			if cprops.PowerCapacity > 0 {
				net.Capacity += cprops.PowerCapacity
				netBatteries = append(netBatteries, cpos)
				if stored, ok := prevStored[cpos]; ok && stored > 0 {
					net.Energy += minf(stored, cprops.PowerCapacity)
				}
			}
			if cprops.PowerProduction > 0 {
				netProducedTick += cprops.PowerProduction * dt * 60
				netProducers = append(netProducers, cpos)
			}
			if cprops.PowerUse > 0 {
				netConsumers = append(netConsumers, cpos)
				if req, ok := requests[cpos]; ok && req > 0 {
					netNeeded += req
				}
			}
			for _, d := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
				nx, ny := ct.X+d[0], ct.Y+d[1]
				if !w.model.InBounds(nx, ny) {
					continue
				}
				npos := packTilePos(nx, ny)
				queue = append(queue, npos)
			}
			if cprops.LinkRangeTiles > 0 {
				for _, linkPos := range w.powerLinksForBuildLocked(cpos, cprops.LinkRangeTiles) {
					queue = append(queue, linkPos)
				}
			}
		}
		if len(netPositions) == 0 {
			continue
		}
		energy := net.Energy
		needed := netNeeded
		produced := netProducedTick
		if needed > produced {
			deficit := needed - produced
			used := minf(deficit, energy)
			energy -= used
			produced += used
		} else if produced > needed {
			excess := produced - needed
			charge := minf(excess, net.Capacity-energy)
			energy += charge
			produced -= charge
		}
		if energy < 0 {
			energy = 0
		}
		if net.Capacity > 0 && energy > net.Capacity {
			energy = net.Capacity
		}
		net.Energy = energy
		if dt > 0 {
			net.Produced = netProducedTick / dt
			net.Consumed = netNeeded / dt
		}
		pct := float32(0)
		if net.Capacity > 0 {
			pct = clampf(net.Energy/net.Capacity, 0, 1)
		}
		for _, bpos := range netBatteries {
			if btile, ok := w.tileForPosLocked(bpos); ok && btile != nil {
				if name, ok := w.blockNamesByID[int16(btile.Block)]; ok {
					n := strings.ToLower(strings.TrimSpace(name))
					if props, ok := w.blockPropsByName[n]; ok && props.PowerCapacity > 0 {
						w.powerStoredByPos[bpos] = pct * props.PowerCapacity
						w.powerStatusByPos[bpos] = pct
						continue
					}
				}
			}
			w.powerStoredByPos[bpos] = 0
			w.powerStatusByPos[bpos] = pct
		}
		coverage := float32(0)
		if netNeeded <= 0 {
			if netProducedTick > 0 || net.Energy > 0 {
				coverage = 1
			}
		} else if produced > 0 {
			coverage = clampf(produced/netNeeded, 0, 1)
		}
		for _, cpos := range netConsumers {
			w.powerStatusByPos[cpos] = coverage
		}
		for _, ppos := range netProducers {
			if _, ok := w.powerStatusByPos[ppos]; !ok {
				if netProducedTick > 0 || net.Energy > 0 {
					w.powerStatusByPos[ppos] = 1
				} else {
					w.powerStatusByPos[ppos] = 0
				}
			}
		}
		for _, p := range netPositions {
			if _, ok := w.powerStatusByPos[p]; !ok {
				w.powerStatusByPos[p] = coverage
			}
		}
	}
	w.stepPowerDiodes(dt, powerNetByPos)
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Block == 0 || t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		name, ok := w.blockNamesByID[int16(t.Block)]
		if !ok {
			continue
		}
		n := strings.ToLower(strings.TrimSpace(name))
		props, ok := w.blockPropsByName[n]
		if !ok || props.PowerCapacity <= 0 {
			continue
		}
		pos := packTilePos(t.X, t.Y)
		net := powerNetByPos[pos]
		if net == nil || net.Capacity <= 0 {
			continue
		}
		pct := clampf(net.Energy/net.Capacity, 0, 1)
		w.powerStoredByPos[pos] = pct * props.PowerCapacity
		w.powerStatusByPos[pos] = pct
	}
	w.powerNetByPos = powerNetByPos
}

func (w *World) blockSpeedMulLocked(pos int32) float32 {
	if w.overdriveBoostByPos == nil {
		return 1
	}
	if v, ok := w.overdriveBoostByPos[pos]; ok && v > 1 {
		return v
	}
	return 1
}

type activeShield struct {
	Pos    int32
	X      float32
	Y      float32
	Radius float32
	Team   TeamID
	Cap    float32
}

func (w *World) stepDefense(dt float32) {
	if w.model == nil || w.blockNamesByID == nil || w.blockPropsByName == nil || dt <= 0 {
		return
	}
	if w.overdriveBoostByPos == nil {
		w.overdriveBoostByPos = map[int32]float32{}
	}
	for k := range w.overdriveBoostByPos {
		delete(w.overdriveBoostByPos, k)
	}
	active := make([]activeShield, 0, 8)
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Block == 0 || t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		name := w.blockNameForTileLocked(t)
		if name == "" {
			continue
		}
		props, ok := w.blockPropsByName[name]
		if !ok {
			continue
		}
		pos := packTilePos(t.X, t.Y)
		// Overdrive projector: apply speed boost to nearby buildings.
		if props.OverdriveBoost > 1 && props.OverdriveRange > 0 {
			if props.PowerUse > 0 && !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
				// no power -> no boost
			} else {
				phase := w.projectorPhaseHeatLocked(t, props, pos, dt)
				boost := props.OverdriveBoost + phase*props.OverdriveBoostPh
				r := props.OverdriveRange + phase*props.OverdrivePhaseRng
				minx := t.X - int(r/8) - 1
				maxx := t.X + int(r/8) + 1
				miny := t.Y - int(r/8) - 1
				maxy := t.Y + int(r/8) + 1
				if minx < 0 {
					minx = 0
				}
				if miny < 0 {
					miny = 0
				}
				if maxx >= w.model.Width {
					maxx = w.model.Width - 1
				}
				if maxy >= w.model.Height {
					maxy = w.model.Height - 1
				}
				cx := float32(t.X*8 + 4)
				cy := float32(t.Y*8 + 4)
				for y := miny; y <= maxy; y++ {
					for x := minx; x <= maxx; x++ {
						tt, err := w.model.TileAt(x, y)
						if err != nil || tt == nil || tt.Build == nil || tt.Build.Team != t.Build.Team {
							continue
						}
						dx := float32(x*8+4) - cx
						dy := float32(y*8+4) - cy
						if dx*dx+dy*dy > r*r {
							continue
						}
						p := packTilePos(x, y)
						if cur, ok := w.overdriveBoostByPos[p]; !ok || boost > cur {
							w.overdriveBoostByPos[p] = boost
						}
					}
				}
			}
		}
		// Mend/regen projectors: heal buildings in range.
		if props.HealPercent > 0 && props.EffectRange > 0 {
			if props.PowerUse > 0 && !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
				continue
			}
			phase := w.projectorPhaseHeatLocked(t, props, pos, dt)
			r := props.EffectRange + phase*props.OverdrivePhaseRng
			healBoost := props.PhaseBoost * phase
			cx := float32(t.X*8 + 4)
			cy := float32(t.Y*8 + 4)
			minx := t.X - int(r/8) - 1
			maxx := t.X + int(r/8) + 1
			miny := t.Y - int(r/8) - 1
			maxy := t.Y + int(r/8) + 1
			if minx < 0 {
				minx = 0
			}
			if miny < 0 {
				miny = 0
			}
			if maxx >= w.model.Width {
				maxx = w.model.Width - 1
			}
			if maxy >= w.model.Height {
				maxy = w.model.Height - 1
			}
			if props.HealReloadSec > 0 {
				charge := w.mendCharge[pos] + dt
				if charge >= props.HealReloadSec {
					charge = 0
					for y := miny; y <= maxy; y++ {
						for x := minx; x <= maxx; x++ {
							tt, err := w.model.TileAt(x, y)
							if err != nil || tt == nil || tt.Build == nil || tt.Build.Team != t.Build.Team {
								continue
							}
							dx := float32(x*8+4) - cx
							dy := float32(y*8+4) - cy
							if dx*dx+dy*dy > r*r {
								continue
							}
							maxHP := tt.Build.MaxHealth
							if maxHP <= 0 {
								maxHP = estimateBuildMaxHealth(int16(tt.Block), w.model)
							}
							heal := maxHP * (props.HealPercent + healBoost) / 100
							if heal > 0 && tt.Build.Health < maxHP {
								tt.Build.Health = minf(maxHP, tt.Build.Health+heal)
								w.entityEvents = append(w.entityEvents, EntityEvent{
									Kind:     EntityEventBuildHealth,
									BuildPos: packTilePos(tt.X, tt.Y),
									BuildHP:  tt.Build.Health,
								})
							}
						}
					}
				}
				w.mendCharge[pos] = charge
			} else {
				// continuous heal (regen projectors)
				for y := miny; y <= maxy; y++ {
					for x := minx; x <= maxx; x++ {
						tt, err := w.model.TileAt(x, y)
						if err != nil || tt == nil || tt.Build == nil || tt.Build.Team != t.Build.Team {
							continue
						}
						dx := float32(x*8+4) - cx
						dy := float32(y*8+4) - cy
						if dx*dx+dy*dy > r*r {
							continue
						}
						maxHP := tt.Build.MaxHealth
						if maxHP <= 0 {
							maxHP = estimateBuildMaxHealth(int16(tt.Block), w.model)
						}
						heal := maxHP * (props.HealPercent + healBoost) * dt
						if heal > 0 && tt.Build.Health < maxHP {
							tt.Build.Health = minf(maxHP, tt.Build.Health+heal)
							w.entityEvents = append(w.entityEvents, EntityEvent{
								Kind:     EntityEventBuildHealth,
								BuildPos: packTilePos(tt.X, tt.Y),
								BuildHP:  tt.Build.Health,
							})
						}
					}
				}
			}
		}
		// Repair turrets/points: continuous heal.
		if props.RepairSpeed > 0 && props.RepairRadius > 0 {
			if props.PowerUse > 0 && !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
				continue
			}
			target := w.findNearestDamagedBuildingLocked(t, props.RepairRadius)
			if target != nil && target.Build != nil {
				maxHP := target.Build.MaxHealth
				if maxHP <= 0 {
					maxHP = estimateBuildMaxHealth(int16(target.Block), w.model)
				}
				if target.Build.Health < maxHP {
					target.Build.Health = minf(maxHP, target.Build.Health+props.RepairSpeed*dt)
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:     EntityEventBuildHealth,
						BuildPos: packTilePos(target.X, target.Y),
						BuildHP:  target.Build.Health,
					})
				}
			}
		}
		// Shields
		if props.ShieldRadius > 0 {
			state := w.shieldStates[pos]
			phase := w.projectorPhaseHeatLocked(t, props, pos, dt)
			cap := props.ShieldHealth + phase*props.PhaseShieldBoost
			if cap < 0 {
				cap = 0
			}
			if cap == 0 {
				state.Shield = 0
				state.Broken = false
			} else {
				if state.Shield <= 0 && !state.Broken {
					state.Shield = cap
				}
				regen := props.ShieldRegenPerS
				if state.Broken && props.ShieldCooldownBrk > 0 {
					regen = props.ShieldCooldownBrk
				}
				if props.CoolantAmountPerS > 0 && props.ShieldCooldownLiq > 0 && w.liquidPropsByID != nil {
					if liqID, liqProps, amt := w.coolantForBuildingLocked(t); liqID > 0 && amt > 0 {
						need := props.CoolantAmountPerS * dt
						if need > 0 {
							used := minf(need, amt)
							if used > 0 {
								_ = w.removeBuildingLiquidLocked(pos, int16(liqID), used)
								capacity := maxf(liqProps.HeatCapacity, 0.4)
								regen *= props.ShieldCooldownLiq * (1 + (capacity-0.4)*0.9)
							}
						}
					}
				}
				if regen > 0 && state.Shield < cap {
					state.Shield += regen * dt
					if state.Shield >= cap {
						state.Shield = cap
						state.Broken = false
					}
				}
			}
			w.shieldStates[pos] = state
			rad := props.ShieldRadius + phase*props.PhaseRadiusBoost
			active = append(active, activeShield{
				Pos:    pos,
				X:      float32(t.X*8 + 4),
				Y:      float32(t.Y*8 + 4),
				Radius: rad,
				Team:   t.Build.Team,
				Cap:    cap,
			})
		}
	}
	w.activeShields = active
}

func (w *World) findNearestDamagedBuildingLocked(src *Tile, rangeLimit float32) *Tile {
	if src == nil || w.model == nil || rangeLimit <= 0 {
		return nil
	}
	best := (*Tile)(nil)
	bestDist2 := rangeLimit * rangeLimit
	cx := float32(src.X*8 + 4)
	cy := float32(src.Y*8 + 4)
	minx := src.X - int(rangeLimit/8) - 1
	maxx := src.X + int(rangeLimit/8) + 1
	miny := src.Y - int(rangeLimit/8) - 1
	maxy := src.Y + int(rangeLimit/8) + 1
	if minx < 0 {
		minx = 0
	}
	if miny < 0 {
		miny = 0
	}
	if maxx >= w.model.Width {
		maxx = w.model.Width - 1
	}
	if maxy >= w.model.Height {
		maxy = w.model.Height - 1
	}
	for y := miny; y <= maxy; y++ {
		for x := minx; x <= maxx; x++ {
			t, err := w.model.TileAt(x, y)
			if err != nil || t == nil || t.Build == nil || t.Build.Team != src.Build.Team {
				continue
			}
			maxHP := t.Build.MaxHealth
			if maxHP <= 0 {
				maxHP = estimateBuildMaxHealth(int16(t.Block), w.model)
			}
			if t.Build.Health >= maxHP {
				continue
			}
			dx := float32(x*8+4) - cx
			dy := float32(y*8+4) - cy
			d2 := dx*dx + dy*dy
			if d2 > bestDist2 {
				continue
			}
			bestDist2 = d2
			best = t
		}
	}
	return best
}

func (w *World) powerLinksForBuildLocked(buildPos int32, rangeTiles float32) []int32 {
	if rangeTiles <= 0 || w.model == nil {
		return nil
	}
	baseTile, ok := w.tileForPosLocked(buildPos)
	if !ok || baseTile == nil || baseTile.Build == nil {
		return nil
	}
	baseTeam := baseTile.Build.Team
	out := w.powerLinkListLocked(buildPos)
	if len(out) == 0 {
		return nil
	}
	base := protocol.UnpackPoint2(buildPos)
	srcName := w.blockNameForTileLocked(baseTile)
	maxLinks := powerNodeMaxLinks(srcName)
	valid := out[:0]
	for _, pos := range out {
		t, ok := w.tileForPosLocked(pos)
		if !ok || t == nil || t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		if t.Build.Team != baseTeam {
			continue
		}
		name := w.blockNameForTileLocked(t)
		if name == "" {
			continue
		}
		props, ok := w.blockPropsByName[name]
		if !ok || !isPowerBlock(name, props) {
			continue
		}
		pt := protocol.UnpackPoint2(pos)
		dx := float32(pt.X - base.X)
		dy := float32(pt.Y - base.Y)
		if dx*dx+dy*dy > rangeTiles*rangeTiles {
			continue
		}
		if w.powerLineInsulatedLocked(baseTile, t) {
			continue
		}
		valid = append(valid, pos)
		if maxLinks > 0 && len(valid) >= maxLinks {
			break
		}
	}
	if len(valid) == 0 {
		return nil
	}
	return valid
}

func (w *World) stepPowerDiodes(dt float32, nets map[int32]*powerNet) {
	if w.model == nil || w.blockNamesByID == nil || dt <= 0 {
		return
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Block == 0 || t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		name, ok := w.blockNamesByID[int16(t.Block)]
		if !ok {
			continue
		}
		blockName := strings.ToLower(strings.TrimSpace(name))
		if !strings.Contains(blockName, "power-diode") {
			continue
		}
		front := w.nearbyDirLocked(t, int(t.Build.Rotation))
		back := w.nearbyDirLocked(t, int(t.Build.Rotation)+2)
		if front == nil || back == nil || front.Build == nil || back.Build == nil {
			continue
		}
		if front.Build.Team != t.Build.Team || back.Build.Team != t.Build.Team {
			continue
		}
		fpos := packTilePos(front.X, front.Y)
		bpos := packTilePos(back.X, back.Y)
		fnet := nets[fpos]
		bnet := nets[bpos]
		if fnet == nil || bnet == nil || fnet == bnet {
			continue
		}
		if bnet.Capacity <= 0 || fnet.Capacity <= 0 {
			continue
		}
		backPct := bnet.Energy / bnet.Capacity
		frontPct := fnet.Energy / fnet.Capacity
		if backPct <= frontPct {
			continue
		}
		targetPct := (bnet.Energy + fnet.Energy) / (bnet.Capacity + fnet.Capacity)
		amount := (targetPct*fnet.Capacity - fnet.Energy) * 0.5
		if amount <= 0 {
			continue
		}
		if amount > fnet.Capacity-fnet.Energy {
			amount = fnet.Capacity - fnet.Energy
		}
		if amount > bnet.Energy {
			amount = bnet.Energy
		}
		if amount <= 0 {
			continue
		}
		bnet.Energy -= amount
		fnet.Energy += amount
	}
}

func (w *World) consumePowerAtLocked(pos int32, amount float32) bool {
	if amount <= 0 {
		return true
	}
	if w.powerRequests == nil {
		w.powerRequests = map[int32]float32{}
	}
	w.powerRequests[pos] += amount
	if w.powerStatusByPos == nil {
		return false
	}
	if status, ok := w.powerStatusByPos[pos]; ok {
		return status > 0.0001
	}
	return false
}

var powerNodeMaxLinksByName = map[string]int{
	"power-node":       10,
	"power-node-large": 15,
	"surge-tower":      2,
	"beam-link":        1,
}

func powerNodeMaxLinks(name string) int {
	if w := DefaultWorld(); w != nil {
		if props, ok := w.blockPropsByName[name]; ok && props.MaxLinks > 0 {
			return props.MaxLinks
		}
	}
	if v, ok := powerNodeMaxLinksByName[name]; ok {
		return v
	}
	return 0
}

func powerNodeAutolink(name string) bool {
	switch name {
	case "beam-link":
		return false
	default:
		return strings.Contains(name, "power-node") || strings.Contains(name, "surge-tower")
	}
}

func powerNodeSameBlockOnly(name string) bool {
	return name == "beam-link"
}

func (w *World) stepPowerAutoLinksLocked() {
	if w.model == nil || w.blockNamesByID == nil || w.blockPropsByName == nil {
		return
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Build == nil || t.Block == 0 || t.Build.Health <= 0 {
			continue
		}
		name := w.blockNameForTileLocked(t)
		if name == "" || !powerNodeAutolink(name) {
			continue
		}
		maxNodes := powerNodeMaxLinks(name)
		if maxNodes <= 0 {
			continue
		}
		props, ok := w.blockPropsByName[name]
		if !ok || props.LinkRangeTiles <= 0 {
			continue
		}
		pos := packTilePos(t.X, t.Y)
		if _, ok := w.tileConfigValues[pos]; ok {
			continue
		}
		links := w.powerLinkListLocked(pos)
		if len(links) > 0 {
			continue
		}
		candidates := w.powerPotentialLinksLocked(t, name, props.LinkRangeTiles)
		if len(candidates) == 0 {
			continue
		}
		added := 0
		for _, cand := range candidates {
			if added >= maxNodes {
				break
			}
			if w.addPowerLinkLocked(pos, cand) {
				added++
			}
		}
	}
}

func (w *World) powerPotentialLinksLocked(src *Tile, srcName string, rangeTiles float32) []int32 {
	if src == nil || w.model == nil {
		return nil
	}
	maxLinks := powerNodeMaxLinks(srcName)
	if maxLinks <= 0 {
		return nil
	}
	srcPos := packTilePos(src.X, src.Y)
	out := make([]int32, 0, maxLinks)
	minx := src.X - int(rangeTiles)
	maxx := src.X + int(rangeTiles)
	miny := src.Y - int(rangeTiles)
	maxy := src.Y + int(rangeTiles)
	if minx < 0 {
		minx = 0
	}
	if miny < 0 {
		miny = 0
	}
	if maxx >= w.model.Width {
		maxx = w.model.Width - 1
	}
	if maxy >= w.model.Height {
		maxy = w.model.Height - 1
	}
	type cand struct {
		pos  int32
		dist float32
	}
	list := make([]cand, 0, 32)
	sameOnly := powerNodeSameBlockOnly(srcName)
	for y := miny; y <= maxy; y++ {
		for x := minx; x <= maxx; x++ {
			if x == src.X && y == src.Y {
				continue
			}
			dx := float32(x - src.X)
			dy := float32(y - src.Y)
			if dx*dx+dy*dy > rangeTiles*rangeTiles {
				continue
			}
			if absInt(x-src.X)+absInt(y-src.Y) <= 1 {
				continue
			}
			t, err := w.model.TileAt(x, y)
			if err != nil || t == nil || t.Build == nil || t.Build.Health <= 0 {
				continue
			}
			if t.Build.Team != src.Build.Team {
				continue
			}
			name := w.blockNameForTileLocked(t)
			if name == "" {
				continue
			}
			if w.powerLineInsulatedLocked(src, t) {
				continue
			}
			props, ok := w.blockPropsByName[name]
			if !ok || !isPowerBlock(name, props) {
				continue
			}
			if sameOnly && name != srcName {
				continue
			}
			if otherRange := props.LinkRangeTiles; otherRange > 0 {
				if dx*dx+dy*dy > otherRange*otherRange && dx*dx+dy*dy > rangeTiles*rangeTiles {
					continue
				}
			}
			pos := packTilePos(x, y)
			if pos == srcPos {
				continue
			}
			list = append(list, cand{pos: pos, dist: dx*dx + dy*dy})
		}
	}
	sort.Slice(list, func(i, j int) bool { return list[i].dist < list[j].dist })
	for _, c := range list {
		out = append(out, c.pos)
		if len(out) >= maxLinks {
			break
		}
	}
	return out
}

func (w *World) powerLinkListLocked(pos int32) []int32 {
	if w.tileConfigValues == nil {
		return nil
	}
	v, ok := w.tileConfigValues[pos]
	if !ok || v == nil {
		return nil
	}
	base := protocol.UnpackPoint2(pos)
	out := make([]int32, 0, 4)
	addAbs := func(px, py int32) {
		if px < 0 || py < 0 || w.model == nil {
			return
		}
		if int(px) >= w.model.Width || int(py) >= w.model.Height {
			return
		}
		p := packTilePos(int(px), int(py))
		if p == pos {
			return
		}
		for _, e := range out {
			if e == p {
				return
			}
		}
		out = append(out, p)
	}
	switch x := v.(type) {
	case protocol.Point2:
		addAbs(base.X+x.X, base.Y+x.Y)
	case []protocol.Point2:
		for _, p := range x {
			addAbs(base.X+p.X, base.Y+p.Y)
		}
	case int32:
		p := protocol.UnpackPoint2(x)
		addAbs(p.X, p.Y)
	case int:
		p := protocol.UnpackPoint2(int32(x))
		addAbs(p.X, p.Y)
	case []int32:
		for _, p := range x {
			pp := protocol.UnpackPoint2(p)
			addAbs(pp.X, pp.Y)
		}
	case []int:
		for _, p := range x {
			pp := protocol.UnpackPoint2(int32(p))
			addAbs(pp.X, pp.Y)
		}
	default:
		if ref, ok := v.(interface{ Pos() int32 }); ok {
			p := protocol.UnpackPoint2(ref.Pos())
			addAbs(p.X, p.Y)
		}
	}
	return out
}

func (w *World) setPowerLinkListLocked(pos int32, links []int32) {
	if w.tileConfigValues == nil {
		w.tileConfigValues = map[int32]any{}
	}
	if len(links) == 0 {
		delete(w.tileConfigValues, pos)
		return
	}
	base := protocol.UnpackPoint2(pos)
	out := make([]protocol.Point2, 0, len(links))
	for _, p := range links {
		pp := protocol.UnpackPoint2(p)
		out = append(out, protocol.Point2{X: pp.X - base.X, Y: pp.Y - base.Y})
	}
	w.tileConfigValues[pos] = out
}

func (w *World) addPowerLinkLocked(aPos int32, bPos int32) bool {
	if aPos == bPos {
		return false
	}
	aTile, okA := w.tileForPosLocked(aPos)
	bTile, okB := w.tileForPosLocked(bPos)
	if !okA || !okB || aTile == nil || bTile == nil || aTile.Build == nil || bTile.Build == nil {
		return false
	}
	aName := w.blockNameForTileLocked(aTile)
	bName := w.blockNameForTileLocked(bTile)
	if aName == "" || bName == "" {
		return false
	}
	aProps, ok := w.blockPropsByName[aName]
	if !ok || aProps.LinkRangeTiles <= 0 {
		return false
	}
	if !w.powerLinkValid(aTile, bTile, aProps.LinkRangeTiles, powerNodeSameBlockOnly(aName)) {
		return false
	}
	if powerNodeMaxLinks(aName) > 0 && len(w.powerLinkListLocked(aPos)) >= powerNodeMaxLinks(aName) {
		return false
	}
	if powerNodeMaxLinks(bName) > 0 && len(w.powerLinkListLocked(bPos)) >= powerNodeMaxLinks(bName) {
		return false
	}
	listA := w.powerLinkListLocked(aPos)
	listB := w.powerLinkListLocked(bPos)
	if !containsPos(listA, bPos) {
		listA = append(listA, bPos)
		w.setPowerLinkListLocked(aPos, listA)
	}
	if !containsPos(listB, aPos) {
		listB = append(listB, aPos)
		w.setPowerLinkListLocked(bPos, listB)
	}
	return true
}

func (w *World) powerLinkValid(a *Tile, b *Tile, rangeTiles float32, sameBlockOnly bool) bool {
	if a == nil || b == nil || a == b || a.Build == nil || b.Build == nil {
		return false
	}
	if a.Build.Team != b.Build.Team {
		return false
	}
	aName := w.blockNameForTileLocked(a)
	bName := w.blockNameForTileLocked(b)
	if aName == "" || bName == "" {
		return false
	}
	if sameBlockOnly && aName != bName {
		return false
	}
	ap, okA := w.blockPropsByName[aName]
	bp, okB := w.blockPropsByName[bName]
	if !okA || !okB {
		return false
	}
	if !isPowerBlock(aName, ap) || !isPowerBlock(bName, bp) {
		return false
	}
	dx := float32(b.X - a.X)
	dy := float32(b.Y - a.Y)
	d2 := dx*dx + dy*dy
	r2 := rangeTiles * rangeTiles
	if bp.LinkRangeTiles > 0 && bp.LinkRangeTiles*bp.LinkRangeTiles > r2 {
		r2 = bp.LinkRangeTiles * bp.LinkRangeTiles
	}
	if d2 > r2 {
		return false
	}
	if w.powerLineInsulatedLocked(a, b) {
		return false
	}
	return true
}

func (w *World) powerLineInsulatedLocked(a *Tile, b *Tile) bool {
	if a == nil || b == nil || w.model == nil {
		return false
	}
	x0, y0 := a.X, a.Y
	x1, y1 := b.X, b.Y
	dx := absInt(x1 - x0)
	sx := -1
	if x0 < x1 {
		sx = 1
	}
	dy := -absInt(y1 - y0)
	sy := -1
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	first := true
	for {
		if !(first || (x0 == x1 && y0 == y1)) {
			t, errTile := w.model.TileAt(x0, y0)
			if errTile == nil && t != nil {
				name := w.blockNameForTileLocked(t)
				if isPowerInsulatorName(name) {
					return true
				}
			}
		}
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			if x0 == x1 {
				// no-op
			} else {
				err += dy
				x0 += sx
			}
		}
		if e2 <= dx {
			if y0 == y1 {
				// no-op
			} else {
				err += dx
				y0 += sy
			}
		}
		first = false
	}
	return false
}

func isPowerInsulatorName(name string) bool {
	if name == "" {
		return false
	}
	return strings.Contains(name, "insulated")
}

func containsPos(list []int32, pos int32) bool {
	for _, v := range list {
		if v == pos {
			return true
		}
	}
	return false
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func (w *World) conveyorAcceptOneLocked(t *Tile, item ItemID) bool {
	if t == nil || t.Build == nil {
		return false
	}
	name := w.blockNameForTileLocked(t)
	if !isConveyorName(name) || isBridgeItemName(name) {
		return false
	}
	cap := conveyorCapacity(name)
	if t.Build.ConvLen >= cap {
		return false
	}
	itemSpace := conveyorItemSpace(name)
	for i := t.Build.ConvLen; i > 0; i-- {
		t.Build.ConvItems[i] = t.Build.ConvItems[i-1]
		t.Build.ConvPos[i] = t.Build.ConvPos[i-1]
	}
	t.Build.ConvItems[0] = item
	t.Build.ConvPos[0] = 0
	if t.Build.ConvMin > itemSpace || t.Build.ConvLen == 0 {
		t.Build.ConvMin = 0
	}
	t.Build.ConvLen++
	t.Build.AddItem(item, 1)
	return true
}

func (w *World) conveyorAcceptStackLocked(t *Tile, itemID int16, amount int32) int32 {
	if t == nil || t.Build == nil || amount <= 0 || itemID <= 0 {
		return 0
	}
	name := w.blockNameForTileLocked(t)
	if !isConveyorName(name) || isBridgeItemName(name) {
		return 0
	}
	cap := conveyorCapacity(name)
	if cap <= 0 {
		return 0
	}
	itemSpace := conveyorItemSpace(name)
	accepted := int32(0)
	for amount > 0 && t.Build.ConvLen < cap {
		if t.Build.ConvMin < itemSpace {
			break
		}
		for i := t.Build.ConvLen; i > 0; i-- {
			t.Build.ConvItems[i] = t.Build.ConvItems[i-1]
			t.Build.ConvPos[i] = t.Build.ConvPos[i-1]
		}
		t.Build.ConvItems[0] = ItemID(itemID)
		t.Build.ConvPos[0] = 0
		t.Build.ConvLen++
		t.Build.ConvMin = 0
		t.Build.AddItem(ItemID(itemID), 1)
		accepted++
		amount--
	}
	return accepted
}

func (w *World) conveyorRemoveIndexLocked(t *Tile, idx int) {
	if t == nil || t.Build == nil {
		return
	}
	if idx < 0 || idx >= t.Build.ConvLen {
		return
	}
	item := t.Build.ConvItems[idx]
	for i := idx; i < t.Build.ConvLen-1; i++ {
		t.Build.ConvItems[i] = t.Build.ConvItems[i+1]
		t.Build.ConvPos[i] = t.Build.ConvPos[i+1]
	}
	t.Build.ConvLen--
	if t.Build.ConvLen < 0 {
		t.Build.ConvLen = 0
	}
	if t.Build.ConvLen == 0 {
		t.Build.ConvMin = 1
	}
	if item != 0 {
		_ = t.Build.RemoveItem(item, 1)
	}
}

func (w *World) conveyorRemoveItemLocked(t *Tile, itemID int16, amount int32) int32 {
	if t == nil || t.Build == nil || amount <= 0 {
		return 0
	}
	removed := int32(0)
	for removed < amount {
		idx := -1
		for i := 0; i < t.Build.ConvLen; i++ {
			if t.Build.ConvItems[i] == ItemID(itemID) {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		w.conveyorRemoveIndexLocked(t, idx)
		removed++
	}
	return removed
}

const (
	stackStateMove   = 0
	stackStateLoad   = 1
	stackStateUnload = 2
)

func (w *World) stackConveyorStateLocked(t *Tile, name string) int {
	if t == nil || t.Build == nil {
		return stackStateMove
	}
	outputRouter := stackConveyorOutputRouter(name)
	front := w.nearbyDirLocked(t, int(t.Build.Rotation))
	back := w.nearbyDirLocked(t, int(t.Build.Rotation)+2)
	hasFront := front != nil && front.Build != nil && front.Build.Team == t.Build.Team && isStackConveyorName(w.blockNameForTileLocked(front))
	hasBack := back != nil && back.Build != nil && back.Build.Team == t.Build.Team && isStackConveyorName(w.blockNameForTileLocked(back))

	state := stackStateMove
	if !hasBack {
		state = stackStateLoad
	}
	if outputRouter && !hasFront && hasBack {
		state = stackStateUnload
	}
	if !outputRouter && !hasFront {
		state = stackStateUnload
	}
	if hasBack && back.Build.StackState == stackStateUnload {
		state = stackStateLoad
	}
	return state
}

func (w *World) stackConveyorAcceptLocked(t *Tile, itemID int16, amount int32) int32 {
	if t == nil || t.Build == nil || amount <= 0 || itemID <= 0 {
		return 0
	}
	name := w.blockNameForTileLocked(t)
	if !isStackConveyorName(name) {
		return 0
	}
	state := w.stackConveyorStateLocked(t, name)
	t.Build.StackState = state
	outputRouter := stackConveyorOutputRouter(name)
	recharge := stackConveyorRecharge(name)
	if t.Build.StackCooldown > recharge-1 {
		return 0
	}
	if (!outputRouter && state != stackStateLoad) || (state != stackStateLoad && len(t.Build.Items) > 0) {
		return 0
	}
	if len(t.Build.Items) > 0 {
		if t.Build.Items[0].Item != ItemID(itemID) {
			return 0
		}
	}
	capacity := int32(10)
	if props, ok := w.blockPropsByName[name]; ok && props.ItemCapacity > 0 {
		capacity = props.ItemCapacity
	}
	total := int32(0)
	for i := range t.Build.Items {
		total += t.Build.Items[i].Amount
	}
	space := capacity - total
	if space <= 0 {
		return 0
	}
	if amount > space {
		amount = space
	}
	if amount <= 0 {
		return 0
	}
	if t.Build.StackLink == -1 {
		t.Build.StackLink = packTilePos(t.X, t.Y)
	}
	t.Build.AddItem(ItemID(itemID), amount)
	t.Build.StackLastItem = ItemID(itemID)
	return amount
}

func (w *World) stackConveyorRemoveLocked(t *Tile, itemID int16, amount int32) int32 {
	if t == nil || t.Build == nil || amount <= 0 {
		return 0
	}
	removed := int32(0)
	for removed < amount {
		found := false
		for i := range t.Build.Items {
			if t.Build.Items[i].Item == ItemID(itemID) && t.Build.Items[i].Amount > 0 {
				t.Build.Items[i].Amount--
				if t.Build.Items[i].Amount <= 0 {
					t.Build.Items = append(t.Build.Items[:i], t.Build.Items[i+1:]...)
				}
				removed++
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	if len(t.Build.Items) == 0 {
		t.Build.StackLink = -1
		t.Build.StackLastItem = 0
	}
	return removed
}

func (w *World) stackConveyorDumpLocked(t *Tile, item ItemID) bool {
	if t == nil || t.Build == nil || item == 0 {
		return false
	}
	dirs := []int{int(t.Build.Rotation)}
	if stackConveyorOutputRouter(w.blockNameForTileLocked(t)) {
		dirs = append(dirs, (int(t.Build.Rotation)+3)%4, (int(t.Build.Rotation)+1)%4, (int(t.Build.Rotation)+2)%4)
	}
	start := t.Build.StackDumpIdx % len(dirs)
	for i := 0; i < len(dirs); i++ {
		dir := dirs[(start+i)%len(dirs)]
		nb := w.nearbyDirLocked(t, dir)
		if nb == nil || nb.Build == nil || nb.Build.Team != t.Build.Team {
			continue
		}
		if !w.canAcceptItemWithDirLocked(nb, int16(item), dir) {
			continue
		}
		added := w.acceptBuildingItemLocked(packTilePos(nb.X, nb.Y), int16(item), 1)
		if added <= 0 {
			continue
		}
		_ = w.stackConveyorRemoveLocked(t, int16(item), 1)
		t.Build.StackDumpIdx = (start + i + 1) % len(dirs)
		return true
	}
	return false
}

func (w *World) stepStackConveyorLocked(t *Tile, name string, dt float32) (srcPos int32, dstPos int32, moved bool, ok bool) {
	if t == nil || t.Build == nil || dt <= 0 {
		return 0, 0, false, false
	}
	if !isStackConveyorName(name) {
		return 0, 0, false, false
	}
	pos := packTilePos(t.X, t.Y)
	if props, ok := w.blockPropsByName[name]; ok && props.PowerUse > 0 {
		if !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
			return 0, 0, false, true
		}
	}
	if t.Build.StackLink == 0 {
		t.Build.StackLink = -1
	}
	scale := dt * float32(w.tps)
	speed := stackConveyorSpeed(name)
	if t.Build.StackCooldown > 0 {
		t.Build.StackCooldown = maxf(0, t.Build.StackCooldown-speed*scale)
	}
	if t.Build.StackLink == -1 && len(t.Build.Items) == 0 {
		return 0, 0, false, true
	}
	state := w.stackConveyorStateLocked(t, name)
	t.Build.StackState = state
	if t.Build.StackCooldown > 0 {
		return 0, 0, false, true
	}
	// Refresh last item.
	if t.Build.StackLastItem == 0 || w.buildingItemAmountLocked(t, int16(t.Build.StackLastItem)) == 0 {
		if len(t.Build.Items) > 0 {
			t.Build.StackLastItem = t.Build.Items[0].Item
		} else {
			t.Build.StackLastItem = 0
		}
	}
	if t.Build.StackLastItem == 0 {
		t.Build.StackLink = -1
		return 0, 0, false, true
	}
	if state == stackStateUnload {
		for i := 0; i < 4; i++ {
			if w.stackConveyorDumpLocked(t, t.Build.StackLastItem) {
				return pos, pos, true, true
			}
			if len(t.Build.Items) == 0 {
				break
			}
		}
		return 0, 0, false, true
	}
	// Move between stack conveyors.
	front := w.nearbyDirLocked(t, int(t.Build.Rotation))
	if front != nil && front.Build != nil && front.Build.Team == t.Build.Team && isStackConveyorName(w.blockNameForTileLocked(front)) {
		if front.Build.StackLink == -1 && len(t.Build.Items) > 0 {
			front.Build.Items = append(front.Build.Items, t.Build.Items...)
			front.Build.StackLastItem = t.Build.StackLastItem
			front.Build.StackLink = packTilePos(t.X, t.Y)
			front.Build.StackCooldown = 1
			t.Build.Items = nil
			t.Build.StackLastItem = 0
			t.Build.StackLink = -1
			t.Build.StackCooldown = stackConveyorRecharge(name)
			return packTilePos(t.X, t.Y), packTilePos(front.X, front.Y), true, true
		}
	}
	return 0, 0, false, true
}

func overflowDuctSpeed(name string) float32 {
	_ = name
	return 5.0
}

func ductSpeed(name string) float32 {
	_ = name
	return 5.0
}

func (w *World) overflowDuctAcceptLocked(t *Tile, itemID int16, amount int32) int32 {
	if t == nil || t.Build == nil || itemID <= 0 || amount <= 0 {
		return 0
	}
	if t.Build.OverflowCurrent != 0 || len(t.Build.Items) > 0 {
		return 0
	}
	t.Build.OverflowCurrent = ItemID(itemID)
	t.Build.OverflowProgress = -1
	t.Build.AddItem(ItemID(itemID), 1)
	return 1
}

func (w *World) overflowDuctRemoveLocked(t *Tile, itemID int16, amount int32) int32 {
	if t == nil || t.Build == nil || amount <= 0 {
		return 0
	}
	removed := int32(0)
	for removed < amount {
		found := false
		for i := range t.Build.Items {
			if t.Build.Items[i].Item == ItemID(itemID) && t.Build.Items[i].Amount > 0 {
				t.Build.Items[i].Amount--
				if t.Build.Items[i].Amount <= 0 {
					t.Build.Items = append(t.Build.Items[:i], t.Build.Items[i+1:]...)
				}
				removed++
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	if ItemID(itemID) == t.Build.OverflowCurrent && removed > 0 {
		t.Build.OverflowCurrent = 0
	}
	return removed
}

func (w *World) overflowDuctTargetLocked(t *Tile, name string) *Tile {
	if t == nil || t.Build == nil || t.Build.OverflowCurrent == 0 {
		return nil
	}
	invert := isUnderflowDuctName(name)
	itemID := int16(t.Build.OverflowCurrent)
	rot := int(t.Build.Rotation)
	left := (rot + 3) % 4
	right := (rot + 1) % 4
	if invert {
		l := w.nearbyDirLocked(t, left)
		r := w.nearbyDirLocked(t, right)
		lc := l != nil && l.Build != nil && l.Build.Team == t.Build.Team && w.canAcceptItemWithDirLocked(l, itemID, left)
		rc := r != nil && r.Build != nil && r.Build.Team == t.Build.Team && w.canAcceptItemWithDirLocked(r, itemID, right)
		if lc && !rc {
			return l
		} else if rc && !lc {
			return r
		} else if lc && rc {
			if t.Build.OverflowDumpIdx == 0 {
				return l
			}
			return r
		}
		return nil
	}
	front := w.nearbyDirLocked(t, rot)
	if front != nil && front.Build != nil && front.Build.Team == t.Build.Team && w.canAcceptItemWithDirLocked(front, itemID, rot) {
		return front
	}
	for i := -1; i <= 1; i++ {
		dir := (rot + ((i + int(t.Build.OverflowDumpIdx) + 1) % 3) - 1 + 4) % 4
		if dir == rot {
			continue
		}
		nb := w.nearbyDirLocked(t, dir)
		if nb != nil && nb.Build != nil && nb.Build.Team == t.Build.Team && w.canAcceptItemWithDirLocked(nb, itemID, dir) {
			return nb
		}
	}
	return nil
}

func (w *World) stepOverflowDuctLocked(t *Tile, name string, dt float32) (srcPos int32, dstPos int32, moved bool, ok bool) {
	if t == nil || t.Build == nil || dt <= 0 {
		return 0, 0, false, false
	}
	if !isOverflowDuctName(name) && !isUnderflowDuctName(name) {
		return 0, 0, false, false
	}
	speed := overflowDuctSpeed(name)
	if speed <= 0 {
		speed = 5
	}
	t.Build.OverflowProgress += dt * float32(w.tps) / speed * 2
	if t.Build.OverflowCurrent != 0 {
		if t.Build.OverflowProgress >= (1 - 1/speed) {
			if target := w.overflowDuctTargetLocked(t, name); target != nil {
				added := w.acceptBuildingItemLocked(packTilePos(target.X, target.Y), int16(t.Build.OverflowCurrent), 1)
				if added > 0 {
					t.Build.OverflowDumpIdx = int8(func() int {
						if t.Build.OverflowDumpIdx == 0 {
							return 2
						}
						return 0
					}())
					_ = w.overflowDuctRemoveLocked(t, int16(t.Build.OverflowCurrent), 1)
					t.Build.OverflowCurrent = 0
					remain := float32(1 - 1/speed)
					if remain > 0 {
						t.Build.OverflowProgress = float32(math.Mod(float64(t.Build.OverflowProgress), float64(remain)))
					} else {
						t.Build.OverflowProgress = 0
					}
					return packTilePos(t.X, t.Y), packTilePos(target.X, target.Y), true, true
				}
			}
		}
	} else {
		t.Build.OverflowProgress = 0
	}
	if t.Build.OverflowCurrent == 0 && len(t.Build.Items) > 0 {
		t.Build.OverflowCurrent = t.Build.Items[0].Item
	}
	return 0, 0, false, true
}

func (w *World) ductAcceptLocked(t *Tile, itemID int16, amount int32) int32 {
	if t == nil || t.Build == nil || itemID <= 0 || amount <= 0 {
		return 0
	}
	if t.Build.DuctCurrent != 0 || len(t.Build.Items) > 0 {
		return 0
	}
	t.Build.DuctCurrent = ItemID(itemID)
	t.Build.DuctProgress = -1
	t.Build.DuctRecDir = 0
	t.Build.AddItem(ItemID(itemID), 1)
	return 1
}

func (w *World) ductAcceptFromLocked(t *Tile, itemID int16, recDir int) bool {
	if t == nil || t.Build == nil || itemID <= 0 {
		return false
	}
	if t.Build.DuctCurrent != 0 || len(t.Build.Items) > 0 {
		return false
	}
	t.Build.DuctCurrent = ItemID(itemID)
	t.Build.DuctProgress = -1
	t.Build.DuctRecDir = int8(recDir)
	t.Build.AddItem(ItemID(itemID), 1)
	return true
}

func (w *World) ductRemoveLocked(t *Tile, itemID int16, amount int32) int32 {
	if t == nil || t.Build == nil || amount <= 0 {
		return 0
	}
	removed := int32(0)
	for removed < amount {
		found := false
		for i := range t.Build.Items {
			if t.Build.Items[i].Item == ItemID(itemID) && t.Build.Items[i].Amount > 0 {
				t.Build.Items[i].Amount--
				if t.Build.Items[i].Amount <= 0 {
					t.Build.Items = append(t.Build.Items[:i], t.Build.Items[i+1:]...)
				}
				removed++
				found = true
				break
			}
		}
		if !found {
			break
		}
	}
	if ItemID(itemID) == t.Build.DuctCurrent && removed > 0 {
		t.Build.DuctCurrent = 0
	}
	return removed
}

func (w *World) stepDuctLocked(t *Tile, name string, dt float32) (srcPos int32, dstPos int32, moved bool, ok bool) {
	if t == nil || t.Build == nil || dt <= 0 {
		return 0, 0, false, false
	}
	if !isDuctName(name) {
		return 0, 0, false, false
	}
	speed := ductSpeed(name)
	if speed <= 0 {
		speed = 5
	}
	t.Build.DuctProgress += dt * float32(w.tps) / speed * 2
	next := w.nearbyDirLocked(t, int(t.Build.Rotation))
	if t.Build.DuctCurrent != 0 && next != nil {
		if t.Build.DuctProgress >= (1 - 1/speed) {
			if next.Build != nil && next.Build.Team == t.Build.Team && w.canAcceptItemWithDirLocked(next, int16(t.Build.DuctCurrent), int(t.Build.Rotation)) {
				movedOK := false
				nname := w.blockNameForTileLocked(next)
				if isDuctName(nname) {
					movedOK = w.ductAcceptFromLocked(next, int16(t.Build.DuctCurrent), int(t.Build.Rotation))
				} else {
					movedOK = w.acceptBuildingItemLocked(packTilePos(next.X, next.Y), int16(t.Build.DuctCurrent), 1) > 0
				}
				if movedOK {
					_ = w.ductRemoveLocked(t, int16(t.Build.DuctCurrent), 1)
					t.Build.DuctCurrent = 0
					remain := float32(1 - 1/speed)
					if remain > 0 {
						t.Build.DuctProgress = float32(math.Mod(float64(t.Build.DuctProgress), float64(remain)))
					} else {
						t.Build.DuctProgress = 0
					}
					return packTilePos(t.X, t.Y), packTilePos(next.X, next.Y), true, true
				}
			}
		}
	} else {
		t.Build.DuctProgress = 0
	}
	if t.Build.DuctCurrent == 0 && len(t.Build.Items) > 0 {
		t.Build.DuctCurrent = t.Build.Items[0].Item
	}
	if t.Build.DuctCurrent == 0 {
		// Pull from neighbors (back/side).
		rot := int(t.Build.Rotation)
		back := (rot + 2) % 4
		left := (rot + 3) % 4
		right := (rot + 1) % 4
		inputDirs := []int{back, left, right}
		for _, dir := range inputDirs {
			src := w.nearbyDirLocked(t, dir)
			if src == nil || src.Build == nil || src.Build.Team != t.Build.Team {
				continue
			}
			itemID := firstItemID(src.Build)
			if itemID <= 0 {
				continue
			}
			if !w.transportSourceCanOutputLocked(src, dir) {
				continue
			}
			srcDir := (dir + 2) % 4
			if isArmoredDuctName(name) {
				// Armored ducts accept from back or from a duct pointing into it.
				if srcDir != rot {
					sname := w.blockNameForTileLocked(src)
					if !(isDuctName(sname) && int(src.Build.Rotation) == srcDir) {
						continue
					}
				}
			}
			if !w.canAcceptItemWithDirLocked(t, itemID, srcDir) {
				continue
			}
			srcPos = packTilePos(src.X, src.Y)
			taken := w.removeBuildingItemLocked(srcPos, itemID, 1)
			if taken <= 0 {
				continue
			}
			if w.ductAcceptFromLocked(t, itemID, srcDir) {
				return srcPos, packTilePos(t.X, t.Y), true, true
			}
		}
	}
	return 0, 0, false, true
}

func (w *World) stepExtraction(dt float32) {
	if w.model == nil || w.blockNamesByID == nil || w.blockPropsByName == nil || dt <= 0 {
		return
	}
	if w.drillStates == nil {
		w.drillStates = map[int32]craftState{}
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Block == 0 || t.Build == nil || t.Build.Health <= 0 {
			pos := packTilePos(t.X, t.Y)
			delete(w.drillStates, pos)
			continue
		}
		name, ok := w.blockNamesByID[int16(t.Block)]
		if !ok {
			continue
		}
		blockName := strings.ToLower(strings.TrimSpace(name))
		props, ok := w.blockPropsByName[blockName]
		if !ok {
			continue
		}
		pos := packTilePos(t.X, t.Y)
		itemChanged := false
		liquidChanged := false
		if props.DrillTimeSec > 0 {
			if props.PowerUse > 0 {
				if !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
					continue
				}
			}
			itemID, dominant := w.drillDominantOreLocked(t, props)
			if itemID <= 0 || dominant <= 0 {
				delete(w.drillStates, pos)
				continue
			}
			speed := float32(1)
			if props.BoostMultiplier > 0 && props.BoostLiquid > 0 && props.BoostAmountPerSec > 0 {
				need := props.BoostAmountPerSec * dt
				if need > 0 {
					cur := w.buildingLiquidAmountLocked(t, int16(props.BoostLiquid))
					if cur > 0 {
						eff := minf(1, cur/need)
						speed = 1 + (props.BoostMultiplier-1)*eff
						_ = w.removeBuildingLiquidLocked(pos, int16(props.BoostLiquid), need*eff)
					}
				}
			}
			state := w.drillStates[pos]
			drillSec := props.DrillTimeSec
			if props.HardnessDrillMul > 0 {
				hardness := w.itemHardnessLocked(itemID)
				drillSec = (props.DrillTimeSec*60 + props.HardnessDrillMul*hardness) / 60
			}
			if drillSec <= 0 {
				drillSec = props.DrillTimeSec
			}
			boost := w.blockSpeedMulLocked(pos)
			state.Progress += dt * boost * speed * float32(dominant) / drillSec
			for state.Progress >= 1 {
				if w.acceptBuildingItemAmountLocked(pos, int16(itemID), 1) <= 0 {
					break
				}
				_ = w.acceptBuildingItemLocked(pos, int16(itemID), 1)
				itemChanged = true
				state.Progress -= 1
			}
			w.drillStates[pos] = state
		}
		if props.PumpAmount > 0 {
			if props.PowerUse > 0 {
				if !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
					continue
				}
			}
			liqID := w.floorLiquidAtLocked(t)
			if liqID <= 0 {
				continue
			}
			amt := props.PumpAmount * dt
			if amt > 0 {
				if w.acceptBuildingLiquidLocked(pos, int16(liqID), amt) > 0 {
					liquidChanged = true
				}
			}
		}
		if itemChanged {
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:       EntityEventBuildItems,
				BuildPos:   packTilePos(t.X, t.Y),
				BuildItems: append([]ItemStack(nil), t.Build.Items...),
			})
		}
		if liquidChanged {
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:         EntityEventBuildLiquids,
				BuildPos:     packTilePos(t.X, t.Y),
				BuildLiquids: append([]LiquidStack(nil), t.Build.Liquids...),
			})
		}
	}
}

func (w *World) stepLiquids(dt float32) {
	if w.model == nil || w.blockNamesByID == nil || dt <= 0 {
		return
	}
	tickDelta := dt * ticksPerSecond(w.tps)
	if tickDelta <= 0 {
		return
	}
	changes := map[int32]struct{}{}
	fullSteps := int(tickDelta)
	frac := tickDelta - float32(fullSteps)
	for i := 0; i < fullSteps; i++ {
		w.stepLiquidsTick(1, changes)
	}
	if frac > 0 {
		w.stepLiquidsTick(frac, changes)
	}
	for pos := range changes {
		t, ok := w.tileForPosLocked(pos)
		if !ok || t == nil || t.Build == nil {
			continue
		}
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:         EntityEventBuildLiquids,
			BuildPos:     pos,
			BuildLiquids: append([]LiquidStack(nil), t.Build.Liquids...),
		})
	}
}

func (w *World) stepLiquidsTick(scale float32, changes map[int32]struct{}) {
	if w.model == nil || w.blockNamesByID == nil || scale <= 0 {
		return
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Block == 0 || t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		name, ok := w.blockNamesByID[int16(t.Block)]
		if !ok {
			continue
		}
		blockName := strings.ToLower(strings.TrimSpace(name))
		isJunction := isLiquidJunctionName(blockName)
		isConduit := isConduitName(blockName)
		isBridge := isLiquidBridgeName(blockName)
		props, ok := w.blockPropsByName[blockName]
		if !ok {
			props = defaultLiquidPropsForName(blockName)
		}
		if props.LiquidCapacity <= 0 && !isJunction {
			continue
		}
		pos := packTilePos(t.X, t.Y)
		if isLiquidVoidName(blockName) {
			if len(t.Build.Liquids) > 0 {
				t.Build.Liquids = nil
				changes[pos] = struct{}{}
			}
			continue
		}
		if isLiquidSourceName(blockName) {
			liqID := w.configuredItemIDForBuildLocked(pos)
			if liqID > 0 {
				capacity := props.LiquidCapacity
				if capacity <= 0 {
					capacity = 1000
				}
				needsUpdate := true
				if len(t.Build.Liquids) == 1 && t.Build.Liquids[0].Liquid == LiquidID(liqID) {
					if math.Abs(float64(t.Build.Liquids[0].Amount-capacity)) < 0.001 {
						needsUpdate = false
					}
				}
				if needsUpdate {
					t.Build.Liquids = []LiquidStack{{Liquid: LiquidID(liqID), Amount: capacity}}
					changes[pos] = struct{}{}
				}
			}
			continue
		}
		if isJunction {
			// junctions are passive pass-through blocks; do not actively push liquids.
			continue
		}
		if len(t.Build.Liquids) == 0 {
			continue
		}
		srcLiquid, srcAmt := liquidCurrentLocked(t)
		if srcLiquid == 0 || srcAmt <= 0 {
			continue
		}
		if isBridge {
			if props.PowerUse > 0 {
				if !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, scale/ticksPerSecond(w.tps))) {
					continue
				}
			}
			if dst := w.liquidBridgeTargetLocked(t); dst != nil {
				moved := w.moveLiquidToLocked(t, dst, srcLiquid, props, w.liquidPropsForTileLocked(dst), scale)
				if moved > 0 {
					changes[pos] = struct{}{}
					changes[packTilePos(dst.X, dst.Y)] = struct{}{}
				}
			} else {
				neighbors := w.adjacentBuildingsLocked(t.X, t.Y)
				for _, nb := range neighbors {
					moved := w.dumpLiquidToLocked(t, nb, srcLiquid, props, w.liquidPropsForTileLocked(nb), 1.0, scale)
					if moved > 0 {
						changes[pos] = struct{}{}
						changes[packTilePos(nb.X, nb.Y)] = struct{}{}
					}
				}
			}
			continue
		}
		if isConduit {
			if dst := w.liquidConduitForwardLocked(t); dst != nil {
				moved := w.moveLiquidToLocked(t, dst, srcLiquid, props, w.liquidPropsForTileLocked(dst), scale)
				if moved > 0 {
					changes[pos] = struct{}{}
					changes[packTilePos(dst.X, dst.Y)] = struct{}{}
				}
			} else if !isArmoredConduitName(blockName) {
				dir := int(t.Build.Rotation)
				if forward := w.tileAtDirLocked(t, dir); forward != nil && forward.Build == nil && forward.Block == 0 && w.floorLiquidAtLocked(forward) == 0 {
					leak := srcAmt / 1.5 * scale
					if leak > srcAmt {
						leak = srcAmt
					}
					if leak > 0 {
						_ = w.removeBuildingLiquidLocked(pos, int16(srcLiquid), leak)
						changes[pos] = struct{}{}
					}
				}
			}
			continue
		}
		neighbors := w.liquidNeighborsLocked(t, blockName)
		if len(neighbors) == 0 {
			continue
		}
		for _, nb := range neighbors {
			moved := w.dumpLiquidToLocked(t, nb, srcLiquid, props, w.liquidPropsForTileLocked(nb), 2.0, scale)
			if moved > 0 {
				changes[pos] = struct{}{}
				changes[packTilePos(nb.X, nb.Y)] = struct{}{}
			}
		}
	}
}

func (w *World) oreItemAtLocked(t *Tile) ItemID {
	if t == nil || w.blockNamesByID == nil || w.blockPropsByName == nil {
		return 0
	}
	if name, ok := w.blockNamesByID[int16(t.Overlay)]; ok {
		if props, ok := w.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]; ok && props.ItemDrop != 0 {
			return props.ItemDrop
		}
	}
	if name, ok := w.blockNamesByID[int16(t.Floor)]; ok {
		if props, ok := w.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]; ok && props.ItemDrop != 0 {
			return props.ItemDrop
		}
	}
	return 0
}

func (w *World) itemHardnessLocked(itemID ItemID) float32 {
	if itemID <= 0 {
		return 0
	}
	if w.itemPropsByID != nil {
		if p, ok := w.itemPropsByID[int16(itemID)]; ok {
			return p.Hardness
		}
	}
	return 0
}

func (w *World) itemLowPriorityLocked(itemID ItemID) bool {
	if itemID <= 0 {
		return false
	}
	if w.itemPropsByID != nil {
		if p, ok := w.itemPropsByID[int16(itemID)]; ok {
			return p.LowPriority
		}
	}
	return false
}

func (w *World) drillDominantOreLocked(t *Tile, props BlockProps) (ItemID, int) {
	if t == nil || w.model == nil {
		return 0, 0
	}
	minX, maxX, minY, maxY, ok := w.tileFootprintLocked(t)
	if !ok {
		return 0, 0
	}
	counts := map[ItemID]int{}
	for y := minY; y <= maxY; y++ {
		for x := minX; x <= maxX; x++ {
			if !w.model.InBounds(x, y) {
				continue
			}
			ot, err := w.model.TileAt(x, y)
			if err != nil || ot == nil {
				continue
			}
			itemID := w.oreItemAtLocked(ot)
			if itemID <= 0 {
				continue
			}
			if props.DrillTier > 0 {
				hardness := w.itemHardnessLocked(itemID)
				if hardness > float32(props.DrillTier) {
					continue
				}
			}
			counts[itemID]++
		}
	}
	var best ItemID
	bestCount := 0
	bestLow := true
	for id, c := range counts {
		low := w.itemLowPriorityLocked(id)
		if best == 0 || (!low && bestLow) || c > bestCount || (c == bestCount && id < best) {
			best = id
			bestCount = c
			bestLow = low
		}
	}
	return best, bestCount
}

func (w *World) coolantForBuildingLocked(t *Tile) (LiquidID, LiquidProps, float32) {
	if t == nil || t.Build == nil || len(t.Build.Liquids) == 0 {
		return 0, LiquidProps{}, 0
	}
	best := LiquidID(0)
	bestProps := LiquidProps{}
	bestAmt := float32(0)
	bestHeat := float32(-1)
	for _, st := range t.Build.Liquids {
		if st.Amount <= 0 {
			continue
		}
		props, ok := w.liquidPropsByID[int16(st.Liquid)]
		if !ok {
			continue
		}
		if !props.Coolant {
			continue
		}
		if props.Temperature > 0.5 || props.Flammability >= 0.1 {
			continue
		}
		if props.HeatCapacity > bestHeat {
			best = st.Liquid
			bestProps = props
			bestAmt = st.Amount
			bestHeat = props.HeatCapacity
		}
	}
	if best == 0 {
		return 0, LiquidProps{}, 0
	}
	return best, bestProps, bestAmt
}

func (w *World) projectorPhaseHeatLocked(t *Tile, props BlockProps, pos int32, dt float32) float32 {
	if t == nil || t.Build == nil || props.BoostItem <= 0 || props.BoostItemAmount <= 0 {
		return 0
	}
	if w.buildingItemAmountLocked(t, int16(props.BoostItem)) < props.BoostItemAmount {
		if w.projectorUse != nil {
			w.projectorUse[pos] = 0
		}
		return 0
	}
	useTime := props.UseTimeSec
	if useTime <= 0 {
		useTime = 400.0 / 60.0
	}
	if useTime > 0 {
		if w.projectorUse == nil {
			w.projectorUse = map[int32]float32{}
		}
		charge := w.projectorUse[pos] + dt
		if charge >= useTime {
			charge -= useTime
			_ = w.removeBuildingItemLocked(pos, int16(props.BoostItem), props.BoostItemAmount)
		}
		w.projectorUse[pos] = charge
	}
	return 1
}

func (w *World) floorLiquidAtLocked(t *Tile) LiquidID {
	if t == nil || w.blockNamesByID == nil || w.blockPropsByName == nil {
		return 0
	}
	if name, ok := w.blockNamesByID[int16(t.Floor)]; ok {
		if props, ok := w.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]; ok && props.LiquidDrop != 0 {
			return props.LiquidDrop
		}
	}
	return 0
}

func (w *World) stepCrafting(dt float32) {
	if w.model == nil || w.blockNamesByID == nil || len(w.recipesByBlockName) == 0 || dt <= 0 {
		return
	}
	if w.craftStates == nil {
		w.craftStates = map[int32]craftState{}
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Block == 0 || t.Build == nil || t.Build.Health <= 0 {
			pos := packTilePos(t.X, t.Y)
			delete(w.craftStates, pos)
			continue
		}
		name, ok := w.blockNamesByID[int16(t.Block)]
		if !ok {
			continue
		}
		blockName := strings.ToLower(strings.TrimSpace(name))
		recipe, ok := w.recipesByBlockName[blockName]
		if !ok {
			continue
		}
		pos := packTilePos(t.X, t.Y)
		state := w.craftStates[pos]

		if recipe.Power > 0 {
			if !w.consumePowerAtLocked(pos, powerUseAmount(recipe.Power, dt)) {
				continue
			}
		}
		if !w.craftingInputsAvailableLocked(t, recipe) || !w.craftingOutputsAvailableLocked(pos, recipe) {
			continue
		}

		craftTime := recipe.CraftTime
		if craftTime <= 0 {
			craftTime = 1
		}
		boost := w.blockSpeedMulLocked(pos)
		state.Progress += dt * boost / craftTime
		for state.Progress >= 1 {
			if !w.craftingInputsAvailableLocked(t, recipe) || !w.craftingOutputsAvailableLocked(pos, recipe) {
				break
			}
			w.craftingConsumeLocked(pos, t, recipe)
			w.craftingProduceLocked(pos, t, recipe)
			state.Progress -= 1
		}
		w.craftStates[pos] = state
	}
}

func (w *World) craftingInputsAvailableLocked(t *Tile, recipe CraftRecipe) bool {
	if t == nil || t.Build == nil {
		return false
	}
	for _, it := range recipe.InputItems {
		if w.buildingItemAmountLocked(t, int16(it.Item)) < it.Amount {
			return false
		}
	}
	for _, liq := range recipe.InputLiquids {
		if w.buildingLiquidAmountLocked(t, int16(liq.Liquid)) < liq.Amount {
			return false
		}
	}
	return true
}

func (w *World) craftingOutputsAvailableLocked(pos int32, recipe CraftRecipe) bool {
	for _, it := range recipe.OutputItems {
		if w.acceptBuildingItemAmountLocked(pos, int16(it.Item), it.Amount) <= 0 {
			return false
		}
	}
	return true
}

func (w *World) craftingConsumeLocked(pos int32, t *Tile, recipe CraftRecipe) {
	_ = pos
	for _, it := range recipe.InputItems {
		_ = w.removeBuildingItemLocked(packTilePos(t.X, t.Y), int16(it.Item), it.Amount)
	}
	for _, liq := range recipe.InputLiquids {
		_ = w.removeBuildingLiquidLocked(packTilePos(t.X, t.Y), int16(liq.Liquid), liq.Amount)
	}
}

func (w *World) craftingProduceLocked(pos int32, t *Tile, recipe CraftRecipe) {
	for _, it := range recipe.OutputItems {
		_ = w.acceptBuildingItemLocked(pos, int16(it.Item), it.Amount)
	}
	for _, liq := range recipe.OutputLiquids {
		_ = w.acceptBuildingLiquidLocked(pos, int16(liq.Liquid), liq.Amount)
	}
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventBuildItems,
		BuildPos:   packTilePos(t.X, t.Y),
		BuildItems: append([]ItemStack(nil), t.Build.Items...),
	})
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:         EntityEventBuildLiquids,
		BuildPos:     packTilePos(t.X, t.Y),
		BuildLiquids: append([]LiquidStack(nil), t.Build.Liquids...),
	})
}

func (w *World) logisticsItemAllowed(blockName string, filterItemID int16, itemID int16) bool {
	if itemID <= 0 {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(blockName))
	if strings.Contains(name, "inverted-sorter") {
		if filterItemID <= 0 {
			return true
		}
		return itemID != filterItemID
	}
	if strings.Contains(name, "sorter") || strings.Contains(name, "unloader") {
		if filterItemID <= 0 {
			return true
		}
		return itemID == filterItemID
	}
	return true
}

func (w *World) blockSizeForTileLocked(t *Tile) int {
	if t == nil {
		return 1
	}
	name := w.blockNameForTileLocked(t)
	if name == "" {
		return 1
	}
	if w.blockSizesByName != nil {
		if v, ok := w.blockSizesByName[name]; ok && v > 0 {
			return v
		}
	}
	if v, ok := blockSizeByName[name]; ok && v > 0 {
		return v
	}
	return 1
}

func blockSizeOffset(size int) int {
	if size <= 1 {
		return 0
	}
	return -((size - 1) / 2)
}

func (w *World) tileFootprintLocked(t *Tile) (minX, maxX, minY, maxY int, ok bool) {
	if t == nil {
		return 0, 0, 0, 0, false
	}
	size := w.blockSizeForTileLocked(t)
	if size < 1 {
		size = 1
	}
	offset := blockSizeOffset(size)
	minX = t.X + offset
	minY = t.Y + offset
	maxX = minX + size - 1
	maxY = minY + size - 1
	return minX, maxX, minY, maxY, true
}

func (w *World) resolveBuildTileLocked(x, y int) *Tile {
	if w.model == nil || !w.model.InBounds(x, y) {
		return nil
	}
	t, err := w.model.TileAt(x, y)
	if err != nil || t == nil || t.Block == 0 {
		return nil
	}
	if t.Build != nil {
		return t
	}
	size := w.blockSizeForTileLocked(t)
	if size <= 1 {
		return nil
	}
	search := size - 1
	for dy := -search; dy <= search; dy++ {
		for dx := -search; dx <= search; dx++ {
			nx, ny := x+dx, y+dy
			if !w.model.InBounds(nx, ny) {
				continue
			}
			cand, err := w.model.TileAt(nx, ny)
			if err != nil || cand == nil || cand.Build == nil || cand.Block != t.Block {
				continue
			}
			minX, maxX, minY, maxY, _ := w.tileFootprintLocked(cand)
			if x >= minX && x <= maxX && y >= minY && y <= maxY {
				return cand
			}
		}
	}
	return nil
}

func (w *World) adjacentBuildingsLocked(x, y int) []*Tile {
	if w.model == nil {
		return nil
	}
	src := w.resolveBuildTileLocked(x, y)
	if src == nil || src.Build == nil {
		return nil
	}
	minX, maxX, minY, maxY, _ := w.tileFootprintLocked(src)
	seen := map[int32]struct{}{}
	out := make([]*Tile, 0, 4)
	srcPos := packTilePos(src.X, src.Y)
	add := func(t *Tile) {
		if t == nil || t.Build == nil || t.Block == 0 {
			return
		}
		pos := packTilePos(t.X, t.Y)
		if pos == srcPos {
			return
		}
		if _, ok := seen[pos]; ok {
			return
		}
		seen[pos] = struct{}{}
		out = append(out, t)
	}
	for x0 := minX; x0 <= maxX; x0++ {
		if w.model.InBounds(x0, maxY+1) {
			add(w.resolveBuildTileLocked(x0, maxY+1))
		}
		if w.model.InBounds(x0, minY-1) {
			add(w.resolveBuildTileLocked(x0, minY-1))
		}
	}
	for y0 := minY; y0 <= maxY; y0++ {
		if w.model.InBounds(maxX+1, y0) {
			add(w.resolveBuildTileLocked(maxX+1, y0))
		}
		if w.model.InBounds(minX-1, y0) {
			add(w.resolveBuildTileLocked(minX-1, y0))
		}
	}
	return out
}

func (w *World) liquidNeighborsLocked(t *Tile, blockName string) []*Tile {
	if t == nil || w.model == nil {
		return nil
	}
	out := w.adjacentBuildingsLocked(t.X, t.Y)
	name := strings.ToLower(strings.TrimSpace(blockName))
	isBridge := isLiquidBridgeName(name)
	if isBridge {
		pos := packTilePos(t.X, t.Y)
		dstPos, ok := w.configuredBuildPosForBuildLocked(pos)
		if ok && dstPos != pos && w.bridgeLinkAllowed(name, pos, dstPos) {
			if dstTile, ok := w.tileForPosLocked(dstPos); ok && dstTile != nil && dstTile.Build != nil && dstTile.Block == t.Block && dstTile.Team == t.Team {
				out = append(out, dstTile)
			}
		}
	}
	return out
}

func liquidCurrentLocked(t *Tile) (LiquidID, float32) {
	if t == nil || t.Build == nil {
		return 0, 0
	}
	srcLiquid := LiquidID(0)
	srcAmt := float32(0)
	for _, st := range t.Build.Liquids {
		if st.Amount > srcAmt {
			srcAmt = st.Amount
			srcLiquid = st.Liquid
		}
	}
	return srcLiquid, srcAmt
}

func isLiquidRouterName(name string) bool {
	if name == "" {
		return false
	}
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "liquid-router" || k.Kind == "liquid-container" || k.Kind == "liquid-tank"
		}
	}
	return strings.Contains(name, "liquid-router") || strings.Contains(name, "liquid-container") || strings.Contains(name, "liquid-tank")
}

func isLiquidVoidName(name string) bool {
	if name == "" {
		return false
	}
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "liquid-void"
		}
	}
	return strings.Contains(name, "liquid-void")
}

func isLiquidSourceName(name string) bool {
	if name == "" {
		return false
	}
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "liquid-source"
		}
	}
	return strings.Contains(name, "liquid-source")
}

func isLiquidJunctionName(name string) bool {
	if name == "" {
		return false
	}
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "liquid-junction"
		}
	}
	return strings.Contains(name, "liquid-junction")
}

func isConduitName(name string) bool {
	if name == "" {
		return false
	}
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			switch k.Kind {
			case "conduit", "phase-conduit", "bridge-conduit", "pulse-conduit", "plated-conduit", "armored-conduit", "reinforced-conduit":
				return true
			}
		}
	}
	return strings.Contains(name, "conduit")
}

func isLiquidBridgeName(name string) bool {
	if name == "" {
		return false
	}
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "bridge-conduit" || k.Kind == "phase-conduit"
		}
	}
	return strings.Contains(name, "bridge-conduit") || strings.Contains(name, "phase-conduit")
}

func liquidFlowScale(name string, isRouter, isJunction, isConduit, isBridge bool) float32 {
	switch {
	case isBridge:
		return 12
	case isConduit:
		return 8
	case isRouter:
		return 6
	case isJunction:
		return 6
	default:
		return 6
	}
}

func isArmoredConduitName(name string) bool {
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(name); ok {
			return k.Kind == "plated-conduit" || k.Kind == "reinforced-conduit" || k.Kind == "armored-conduit"
		}
	}
	return strings.Contains(name, "plated-conduit") || strings.Contains(name, "reinforced-conduit") || strings.Contains(name, "armored-conduit")
}

func (w *World) liquidBridgeTargetLocked(t *Tile) *Tile {
	if t == nil {
		return nil
	}
	pos := packTilePos(t.X, t.Y)
	dstPos, ok := w.configuredBuildPosForBuildLocked(pos)
	if !ok || dstPos == pos {
		return nil
	}
	if dst, ok := w.tileForPosLocked(dstPos); ok && dst != nil && dst.Build != nil && dst.Block == t.Block && dst.Team == t.Team {
		return dst
	}
	return nil
}

func (w *World) liquidConduitForwardLocked(t *Tile) *Tile {
	if t == nil {
		return nil
	}
	dir := int(t.Build.Rotation)
	return w.nearbyDirLocked(t, dir)
}

func (w *World) liquidJunctionForwardLocked(t *Tile) *Tile {
	if t == nil {
		return nil
	}
	pos := packTilePos(t.X, t.Y)
	if w.liquidJunctionInputDirs == nil {
		return nil
	}
	dir, ok := w.liquidJunctionInputDirs[pos]
	if !ok {
		return nil
	}
	return w.nearbyDirLocked(t, int(dir))
}

func (w *World) rememberLiquidJunctionInput(junction *Tile, source *Tile) {
	if junction == nil || source == nil {
		return
	}
	if w.liquidJunctionInputDirs == nil {
		w.liquidJunctionInputDirs = map[int32]int8{}
	}
	dir, ok := w.directionFromSourceToSorterLocked(source, junction)
	if !ok {
		return
	}
	w.liquidJunctionInputDirs[packTilePos(junction.X, junction.Y)] = int8(dir)
}

func (w *World) liquidAcceptsLocked(dst *Tile, src *Tile, srcName string, dstName string, liquidID LiquidID) bool {
	if dst == nil || dst.Build == nil || liquidID == 0 {
		return false
	}
	dname := strings.ToLower(strings.TrimSpace(dstName))
	if isLiquidRouterName(dname) {
		cur, amt := liquidCurrentLocked(dst)
		if cur != 0 && cur != liquidID && amt > 0.2 {
			return false
		}
	}
	if isConduitName(dname) {
		if isArmoredConduitName(dname) {
			if src != nil {
				sname := w.blockNameForTileLocked(src)
				if sname != "" && !isConduitName(sname) && !isLiquidBridgeName(sname) && !isLiquidJunctionName(sname) {
					return false
				}
			}
		}
		if src != nil {
			if dir, ok := w.directionFromSourceToSorterLocked(src, dst); ok {
				incoming := (dir + 2) % 4
				if incoming == int(dst.Build.Rotation) {
					return false
				}
			}
		}
		cur, amt := liquidCurrentLocked(dst)
		if cur != 0 && cur != liquidID && amt > 0.2 {
			return false
		}
	}
	return true
}

func defaultLiquidPropsForName(name string) BlockProps {
	n := strings.ToLower(strings.TrimSpace(name))
	props := BlockProps{}
	if w := DefaultWorld(); w != nil {
		if k, ok := w.blockKindByName(n); ok {
			switch k.Kind {
			case "liquid-router":
				props.LiquidCapacity = 120
			case "liquid-container":
				props.LiquidCapacity = 700
			case "liquid-tank":
				props.LiquidCapacity = 1800
			case "phase-conduit", "bridge-conduit":
				props.LiquidCapacity = 100
			case "pulse-conduit":
				props.LiquidCapacity = 40
				props.LiquidPressure = 1.025
			case "plated-conduit", "armored-conduit", "reinforced-conduit":
				props.LiquidCapacity = 50
				props.LiquidPressure = 1.025
			case "conduit":
				props.LiquidCapacity = 20
			case "liquid-junction":
				props.LiquidCapacity = 20
			}
			if props.LiquidCapacity > 0 || props.LiquidPressure > 0 {
				return props
			}
		}
	}
	switch {
	case strings.Contains(n, "liquid-router"):
		props.LiquidCapacity = 120
	case strings.Contains(n, "liquid-container"):
		props.LiquidCapacity = 700
	case strings.Contains(n, "liquid-tank"):
		props.LiquidCapacity = 1800
	case strings.Contains(n, "phase-conduit"):
		props.LiquidCapacity = 100
	case strings.Contains(n, "bridge-conduit"):
		props.LiquidCapacity = 100
	case strings.Contains(n, "pulse-conduit"):
		props.LiquidCapacity = 40
		props.LiquidPressure = 1.025
	case strings.Contains(n, "plated-conduit") || strings.Contains(n, "armored-conduit") || strings.Contains(n, "reinforced-conduit"):
		props.LiquidCapacity = 50
		props.LiquidPressure = 1.025
	case strings.Contains(n, "conduit"):
		props.LiquidCapacity = 20
	case strings.Contains(n, "liquid-junction"):
		props.LiquidCapacity = 20
	}
	return props
}

func (w *World) liquidPropsForTileLocked(t *Tile) BlockProps {
	if t == nil || w.blockNamesByID == nil {
		return BlockProps{}
	}
	name, ok := w.blockNamesByID[int16(t.Block)]
	if !ok {
		return BlockProps{}
	}
	blockName := strings.ToLower(strings.TrimSpace(name))
	if props, ok := w.blockPropsByName[blockName]; ok {
		if props.LiquidCapacity <= 0 && isLiquidJunctionName(blockName) {
			props.LiquidCapacity = 20
		}
		return props
	}
	return defaultLiquidPropsForName(blockName)
}

func (w *World) liquidPropsForIDLocked(id LiquidID) LiquidProps {
	if id <= 0 {
		return defaultLiquidProps()
	}
	if w != nil && w.liquidPropsByID != nil {
		if p, ok := w.liquidPropsByID[int16(id)]; ok {
			return p
		}
	}
	return defaultLiquidProps()
}

func (w *World) blockNameForTileLocked(t *Tile) string {
	if t == nil {
		return ""
	}
	if name, ok := w.blockNamesByID[int16(t.Block)]; ok {
		return strings.ToLower(strings.TrimSpace(name))
	}
	if w.model != nil && w.model.BlockNames != nil {
		if name, ok := w.model.BlockNames[int16(t.Block)]; ok {
			return strings.ToLower(strings.TrimSpace(name))
		}
	}
	return ""
}

func (w *World) liquidDestinationLocked(source *Tile, start *Tile, liquidID LiquidID) *Tile {
	if start == nil {
		return nil
	}
	cur := start
	prev := source
	for i := 0; i < 16; i++ {
		if cur == nil {
			return nil
		}
		name := w.blockNameForTileLocked(cur)
		if name == "" || !isLiquidJunctionName(name) {
			return cur
		}
		dir, ok := w.directionFromSourceToSorterLocked(prev, cur)
		if !ok {
			return cur
		}
		next := w.nearbyDirLocked(cur, dir)
		if next == nil {
			return cur
		}
		nname := w.blockNameForTileLocked(next)
		if next.Build == nil || (!w.liquidAcceptsLocked(next, cur, name, nname, liquidID) && !isLiquidJunctionName(nname)) {
			return cur
		}
		prev = cur
		cur = next
	}
	return cur
}

func (w *World) dumpLiquidToLocked(src *Tile, dst *Tile, liquidID LiquidID, srcProps BlockProps, dstProps BlockProps, scaling float32, tickScale float32) float32 {
	if src == nil || dst == nil || liquidID == 0 || srcProps.LiquidCapacity <= 0 || dstProps.LiquidCapacity <= 0 || tickScale <= 0 {
		return 0
	}
	if scaling <= 0 {
		scaling = 2
	}
	dst = w.liquidDestinationLocked(src, dst, liquidID)
	if dst == nil || dst.Build == nil {
		return 0
	}
	srcAmt := w.buildingLiquidAmountLocked(src, int16(liquidID))
	if srcAmt <= 0 {
		return 0
	}
	if !w.liquidAcceptsLocked(dst, src, "", w.blockNameForTileLocked(dst), liquidID) {
		return 0
	}
	dstAmt := w.buildingLiquidAmountLocked(dst, int16(liquidID))
	srcFill := srcAmt / srcProps.LiquidCapacity
	dstFill := dstAmt / dstProps.LiquidCapacity
	if dstFill >= srcFill {
		return 0
	}
	amount := (srcFill - dstFill) * srcProps.LiquidCapacity / scaling
	amount *= tickScale
	if amount <= 0 {
		return 0
	}
	if amount > srcAmt {
		amount = srcAmt
	}
	if amount > dstProps.LiquidCapacity-dstAmt {
		amount = dstProps.LiquidCapacity - dstAmt
	}
	if amount <= 0 {
		return 0
	}
	taken := w.removeBuildingLiquidLocked(packTilePos(src.X, src.Y), int16(liquidID), amount)
	if taken <= 0 {
		return 0
	}
	added := w.acceptBuildingLiquidLocked(packTilePos(dst.X, dst.Y), int16(liquidID), taken)
	if added < taken {
		_ = w.acceptBuildingLiquidLocked(packTilePos(src.X, src.Y), int16(liquidID), taken-added)
	}
	if added > 0 && isLiquidJunctionName(strings.ToLower(strings.TrimSpace(w.blockNameForTileLocked(dst)))) {
		w.rememberLiquidJunctionInput(dst, src)
	}
	return added
}

func (w *World) moveLiquidToLocked(src *Tile, dst *Tile, liquidID LiquidID, srcProps BlockProps, dstProps BlockProps, tickScale float32) float32 {
	if src == nil || dst == nil || liquidID == 0 || srcProps.LiquidCapacity <= 0 || dstProps.LiquidCapacity <= 0 || tickScale <= 0 {
		return 0
	}
	dst = w.liquidDestinationLocked(src, dst, liquidID)
	if dst == nil || dst.Build == nil {
		return 0
	}
	srcAmt := w.buildingLiquidAmountLocked(src, int16(liquidID))
	if srcAmt <= 0 {
		return 0
	}
	dstAmt := w.buildingLiquidAmountLocked(dst, int16(liquidID))
	if !w.liquidAcceptsLocked(dst, src, "", w.blockNameForTileLocked(dst), liquidID) {
		// reactive interaction when incompatible liquids are present
		otherLiquid, otherAmt := liquidCurrentLocked(dst)
		if otherLiquid != 0 && otherLiquid != liquidID {
			srcFill := srcAmt / srcProps.LiquidCapacity
			dstFill := otherAmt / dstProps.LiquidCapacity
			if srcFill > 0.1 && dstFill > 0.1 {
				srcL := w.liquidPropsForIDLocked(liquidID)
				dstL := w.liquidPropsForIDLocked(otherLiquid)
				if srcL.BlockReactive && dstL.BlockReactive {
					if (dstL.Flammability > 0.3 && srcL.Temperature > 0.7) || (srcL.Flammability > 0.3 && dstL.Temperature > 0.7) {
						w.applyDamageToBuilding(packTilePos(src.X, src.Y), 1*tickScale)
						w.applyDamageToBuilding(packTilePos(dst.X, dst.Y), 1*tickScale)
					} else if (srcL.Temperature > 0.7 && dstL.Temperature < 0.55) || (dstL.Temperature > 0.7 && srcL.Temperature < 0.55) {
						react := minf(srcAmt, 0.7*tickScale)
						if react > 0 {
							_ = w.removeBuildingLiquidLocked(packTilePos(src.X, src.Y), int16(liquidID), react)
						}
					}
				}
			}
		}
		return 0
	}
	pressure := srcProps.LiquidPressure
	if pressure <= 0 {
		pressure = 1
	}
	ofract := dstAmt / dstProps.LiquidCapacity
	fract := (srcAmt / srcProps.LiquidCapacity) * pressure
	diff := clampf(fract-ofract, 0, 1)
	flow := diff * srcProps.LiquidCapacity * tickScale
	if flow > srcAmt {
		flow = srcAmt
	}
	if flow > dstProps.LiquidCapacity-dstAmt {
		flow = dstProps.LiquidCapacity - dstAmt
	}
	if flow <= 0 || ofract > fract {
		return 0
	}
	taken := w.removeBuildingLiquidLocked(packTilePos(src.X, src.Y), int16(liquidID), flow)
	if taken <= 0 {
		return 0
	}
	added := w.acceptBuildingLiquidLocked(packTilePos(dst.X, dst.Y), int16(liquidID), taken)
	if added < taken {
		_ = w.acceptBuildingLiquidLocked(packTilePos(src.X, src.Y), int16(liquidID), taken-added)
	}
	if added > 0 && isLiquidJunctionName(strings.ToLower(strings.TrimSpace(w.blockNameForTileLocked(dst)))) {
		w.rememberLiquidJunctionInput(dst, src)
	}
	return added
}

// bridgeDumpOneLocked tries to dump one item from bridge inventory to one
// adjacent same-team building, similar to ItemBridge#doDump fallback.
func (w *World) bridgeDumpOneLocked(bridge *Tile) (outPos int32, moved int32, ok bool) {
	if bridge == nil || bridge.Build == nil || len(bridge.Build.Items) == 0 {
		return 0, 0, false
	}
	itemID := int16(0)
	for _, st := range bridge.Build.Items {
		if st.Amount > 0 {
			itemID = int16(st.Item)
			break
		}
	}
	if itemID <= 0 {
		return 0, 0, false
	}
	for _, nb := range w.adjacentBuildingsLocked(bridge.X, bridge.Y) {
		if nb == nil || nb.Build == nil || nb.Team != bridge.Team {
			continue
		}
		if nb.Block == bridge.Block {
			continue
		}
		if w.acceptBuildingItemAmountLocked(packTilePos(nb.X, nb.Y), itemID, 1) <= 0 {
			continue
		}
		taken := w.removeBuildingItemLocked(packTilePos(bridge.X, bridge.Y), itemID, 1)
		if taken <= 0 {
			return 0, 0, false
		}
		added := w.acceptBuildingItemLocked(packTilePos(nb.X, nb.Y), itemID, taken)
		if added < taken {
			_ = w.acceptBuildingItemLocked(packTilePos(bridge.X, bridge.Y), itemID, taken-added)
		}
		if added <= 0 {
			return 0, 0, false
		}
		return packTilePos(nb.X, nb.Y), added, true
	}
	return 0, 0, false
}

func (w *World) bridgeLinkTargetLocked(t *Tile, name string) (*Tile, int32, bool) {
	if t == nil || t.Build == nil || name == "" || w.model == nil {
		return nil, 0, false
	}
	if isDuctBridgeName(name) {
		rot := int(t.Build.Rotation)
		dx, dy := dirToDelta(rot)
		for i := 1; i <= 4; i++ {
			nx, ny := t.X+dx*i, t.Y+dy*i
			if !w.model.InBounds(nx, ny) {
				break
			}
			ot, err := w.model.TileAt(nx, ny)
			if err != nil || ot == nil || ot.Build == nil || ot.Block == 0 {
				continue
			}
			if ot.Block == t.Block && ot.Team == t.Team {
				return ot, packTilePos(ot.X, ot.Y), true
			}
		}
		return nil, 0, false
	}
	pos := packTilePos(t.X, t.Y)
	dstPos, ok := w.configuredBuildPosForBuildLocked(pos)
	if !ok || dstPos == pos || !w.bridgeLinkAllowed(name, pos, dstPos) {
		return nil, 0, false
	}
	dstTile, ok := w.tileForPosLocked(dstPos)
	if !ok || dstTile == nil || dstTile.Build == nil || dstTile.Block == 0 {
		return nil, 0, false
	}
	if dstTile.Block != t.Block || dstTile.Team != t.Team {
		return nil, 0, false
	}
	return dstTile, dstPos, true
}

func (w *World) bridgeLinkedToLocked(src *Tile, dst *Tile, srcName string) bool {
	if src == nil || dst == nil || src.Build == nil || dst.Build == nil {
		return false
	}
	target, _, ok := w.bridgeLinkTargetLocked(src, srcName)
	if !ok || target == nil {
		return false
	}
	return target.X == dst.X && target.Y == dst.Y
}

func (w *World) bridgeOutputDirLocked(t *Tile, name string, dst *Tile) (int, bool) {
	if t == nil || t.Build == nil {
		return 0, false
	}
	if isDuctBridgeName(name) {
		return int(t.Build.Rotation), true
	}
	if dst == nil {
		return 0, false
	}
	return directionFromTo(t.X, t.Y, dst.X, dst.Y)
}

func (w *World) bridgeAcceptFromLocked(bridge *Tile, name string, src *Tile, dirFromSrcToBridge int) bool {
	if bridge == nil || bridge.Build == nil || src == nil || src.Build == nil {
		return false
	}
	if isDuctBridgeName(name) {
		if _, _, ok := w.bridgeLinkTargetLocked(bridge, name); !ok {
			return false
		}
		incoming := (dirFromSrcToBridge + 2) % 4
		return incoming != int(bridge.Build.Rotation)
	}
	dst, _, ok := w.bridgeLinkTargetLocked(bridge, name)
	if !ok {
		return false
	}
	srcName := w.blockNameForTileLocked(src)
	if isBridgeItemName(srcName) && w.bridgeLinkedToLocked(src, bridge, srcName) {
		return true
	}
	outDir, okDir := w.bridgeOutputDirLocked(bridge, name, dst)
	if !okDir {
		return true
	}
	incoming := (dirFromSrcToBridge + 2) % 4
	return incoming != outDir
}

func (w *World) junctionStateLocked(pos int32) *junctionState {
	if w.junctionStates == nil {
		w.junctionStates = map[int32]*junctionState{}
	}
	state := w.junctionStates[pos]
	if state == nil {
		state = &junctionState{}
		w.junctionStates[pos] = state
	}
	return state
}

func (w *World) ductJunctionStateLocked(pos int32) *ductJunctionState {
	if w.ductJunctionStates == nil {
		w.ductJunctionStates = map[int32]*ductJunctionState{}
	}
	state := w.ductJunctionStates[pos]
	if state == nil {
		state = &ductJunctionState{}
		w.ductJunctionStates[pos] = state
	}
	return state
}

func (w *World) ductRouterStateLocked(pos int32) *ductRouterState {
	if w.ductRouterStates == nil {
		w.ductRouterStates = map[int32]*ductRouterState{}
	}
	state := w.ductRouterStates[pos]
	if state == nil {
		state = &ductRouterState{}
		w.ductRouterStates[pos] = state
	}
	return state
}

func (w *World) stackRouterStateLocked(pos int32) *stackRouterState {
	if w.stackRouterStates == nil {
		w.stackRouterStates = map[int32]*stackRouterState{}
	}
	state := w.stackRouterStates[pos]
	if state == nil {
		state = &stackRouterState{}
		w.stackRouterStates[pos] = state
	}
	return state
}

func (w *World) ductUnloaderStateLocked(pos int32) *ductUnloaderState {
	if w.ductUnloaderStates == nil {
		w.ductUnloaderStates = map[int32]*ductUnloaderState{}
	}
	state := w.ductUnloaderStates[pos]
	if state == nil {
		state = &ductUnloaderState{}
		w.ductUnloaderStates[pos] = state
	}
	return state
}

func (w *World) junctionLaneAcceptLocked(t *Tile, lane int, itemID int16, speedFrames int) bool {
	if t == nil || t.Build == nil || lane < 0 || lane > 3 || itemID <= 0 {
		return false
	}
	pos := packTilePos(t.X, t.Y)
	state := w.junctionStateLocked(pos)
	ln := &state.lanes[lane]
	if ln.len >= len(ln.items) {
		return false
	}
	ln.items[ln.len] = junctionLaneItem{Item: ItemID(itemID), ReadyTick: w.tick + uint64(speedFrames)}
	ln.len++
	t.Build.AddItem(ItemID(itemID), 1)
	return true
}

func (w *World) junctionLaneCanAcceptLocked(t *Tile, lane int) bool {
	if t == nil || lane < 0 || lane > 3 {
		return false
	}
	state := w.junctionStateLocked(packTilePos(t.X, t.Y))
	return state.lanes[lane].len < len(state.lanes[lane].items)
}

func (w *World) ductJunctionCanAcceptLocked(t *Tile, lane int) bool {
	if t == nil || lane < 0 || lane > 3 {
		return false
	}
	state := w.ductJunctionStateLocked(packTilePos(t.X, t.Y))
	return !state.hasItem[lane]
}

func (w *World) stepJunctionLocked(t *Tile, name string, dt float32) (changedPos []int32, moved bool, ok bool) {
	if t == nil || t.Build == nil {
		return nil, false, false
	}
	pos := packTilePos(t.X, t.Y)
	speedFrames := junctionSpeedFrames(name)
	state := w.junctionStateLocked(pos)
	changed := map[int32]struct{}{}
	// Pull from neighbors into lanes.
	for lane := 0; lane < 4; lane++ {
		if state.lanes[lane].len >= len(state.lanes[lane].items) {
			continue
		}
		src := w.nearbyDirLocked(t, lane)
		if src == nil || src.Build == nil || src.Build.Team != t.Build.Team {
			continue
		}
		dirFromSrc := (lane + 2) % 4
		if !w.transportSourceCanOutputLocked(src, dirFromSrc) {
			continue
		}
		itemID := firstItemID(src.Build)
		if itemID <= 0 {
			continue
		}
		if dst := w.nearbyDirLocked(t, lane); dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			continue
		}
		taken := w.removeBuildingItemLocked(packTilePos(src.X, src.Y), int16(itemID), 1)
		if taken <= 0 {
			continue
		}
		if !w.junctionLaneAcceptLocked(t, lane, int16(itemID), speedFrames) {
			_ = w.acceptBuildingItemLocked(packTilePos(src.X, src.Y), int16(itemID), taken)
			continue
		}
		changed[packTilePos(src.X, src.Y)] = struct{}{}
		changed[pos] = struct{}{}
		moved = true
	}
	// Push from lanes to outputs when ready.
	for lane := 0; lane < 4; lane++ {
		ln := &state.lanes[lane]
		if ln.len <= 0 {
			continue
		}
		if ln.items[0].ReadyTick > w.tick {
			continue
		}
		dst := w.nearbyDirLocked(t, lane)
		if dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			continue
		}
		itemID := int16(ln.items[0].Item)
		if !w.canAcceptItemWithDirLocked(dst, itemID, lane) {
			continue
		}
		added := w.acceptBuildingItemLocked(packTilePos(dst.X, dst.Y), itemID, 1)
		if added <= 0 {
			continue
		}
		_ = w.removeBuildingItemLocked(pos, itemID, added)
		copy(ln.items[0:], ln.items[1:ln.len])
		ln.len--
		changed[pos] = struct{}{}
		changed[packTilePos(dst.X, dst.Y)] = struct{}{}
		moved = true
	}
	for p := range changed {
		changedPos = append(changedPos, p)
	}
	return changedPos, moved, true
}

func (w *World) stepDuctJunctionLocked(t *Tile, name string, dt float32) (changedPos []int32, moved bool, ok bool) {
	if t == nil || t.Build == nil {
		return nil, false, false
	}
	_ = dt
	pos := packTilePos(t.X, t.Y)
	speedFrames := ductJunctionSpeedFrames(name)
	state := w.ductJunctionStateLocked(pos)
	changed := map[int32]struct{}{}
	for lane := 0; lane < 4; lane++ {
		if state.hasItem[lane] {
			continue
		}
		src := w.nearbyDirLocked(t, lane)
		if src == nil || src.Build == nil || src.Build.Team != t.Build.Team {
			continue
		}
		dirFromSrc := (lane + 2) % 4
		if !w.transportSourceCanOutputLocked(src, dirFromSrc) {
			continue
		}
		itemID := firstItemID(src.Build)
		if itemID <= 0 {
			continue
		}
		if dst := w.nearbyDirLocked(t, lane); dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			continue
		}
		taken := w.removeBuildingItemLocked(packTilePos(src.X, src.Y), int16(itemID), 1)
		if taken <= 0 {
			continue
		}
		state.hasItem[lane] = true
		state.items[lane] = junctionLaneItem{Item: ItemID(itemID), ReadyTick: w.tick + uint64(speedFrames)}
		state.readyTick[lane] = w.tick + uint64(speedFrames)
		t.Build.AddItem(ItemID(itemID), 1)
		changed[packTilePos(src.X, src.Y)] = struct{}{}
		changed[pos] = struct{}{}
		moved = true
	}
	for lane := 0; lane < 4; lane++ {
		if !state.hasItem[lane] {
			continue
		}
		if state.readyTick[lane] > w.tick {
			continue
		}
		dst := w.nearbyDirLocked(t, lane)
		if dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			continue
		}
		itemID := int16(state.items[lane].Item)
		if !w.canAcceptItemWithDirLocked(dst, itemID, lane) {
			continue
		}
		added := w.acceptBuildingItemLocked(packTilePos(dst.X, dst.Y), itemID, 1)
		if added <= 0 {
			continue
		}
		_ = w.removeBuildingItemLocked(pos, itemID, added)
		state.hasItem[lane] = false
		state.items[lane] = junctionLaneItem{}
		state.readyTick[lane] = 0
		changed[pos] = struct{}{}
		changed[packTilePos(dst.X, dst.Y)] = struct{}{}
		moved = true
	}
	for p := range changed {
		changedPos = append(changedPos, p)
	}
	return changedPos, moved, true
}

func (w *World) ductRouterTargetLocked(t *Tile, name string, current ItemID, sortItemID int16, state *ductRouterState) *Tile {
	if t == nil || t.Build == nil || current == 0 {
		return nil
	}
	rot := int(t.Build.Rotation)
	for i := 0; i < 4; i++ {
		dir := (state.dumpIdx + i) % 4
		if dir == (rot+2)%4 {
			continue
		}
		if sortItemID > 0 {
			if ItemID(sortItemID) == current {
				if dir != rot {
					continue
				}
			} else {
				if dir == rot {
					continue
				}
			}
		}
		dst := w.nearbyDirLocked(t, dir)
		if dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			continue
		}
		if !w.canAcceptItemWithDirLocked(dst, int16(current), dir) {
			continue
		}
		state.dumpIdx = (dir + 1) % 4
		return dst
	}
	return nil
}

func (w *World) stackRouterTargetLocked(t *Tile, name string, current ItemID, sortItemID int16, state *stackRouterState) *Tile {
	if t == nil || t.Build == nil || current == 0 {
		return nil
	}
	rot := int(t.Build.Rotation)
	for i := 0; i < 4; i++ {
		dir := (state.dumpIdx + i) % 4
		if dir == (rot+2)%4 {
			continue
		}
		if sortItemID > 0 {
			if ItemID(sortItemID) == current {
				if dir != rot {
					continue
				}
			} else {
				if dir == rot {
					continue
				}
			}
		}
		dst := w.nearbyDirLocked(t, dir)
		if dst == nil || dst.Build == nil || dst.Build.Team != t.Build.Team {
			continue
		}
		if !w.canAcceptItemWithDirLocked(dst, int16(current), dir) {
			continue
		}
		state.dumpIdx = (dir + 1) % 4
		return dst
	}
	return nil
}

func (w *World) stepDuctRouterLocked(t *Tile, name string, dt float32) (changedPos []int32, moved bool, ok bool) {
	if t == nil || t.Build == nil {
		return nil, false, false
	}
	pos := packTilePos(t.X, t.Y)
	state := w.ductRouterStateLocked(pos)
	sortItemID := w.configuredItemIDForBuildLocked(pos)
	// Pull from back when empty.
	if state.current == 0 && len(t.Build.Items) == 0 {
		back := (int(t.Build.Rotation) + 2) % 4
		src := w.nearbyDirLocked(t, back)
		if src != nil && src.Build != nil && src.Build.Team == t.Build.Team && w.transportSourceCanOutputLocked(src, int(t.Build.Rotation)) {
			itemID := firstItemID(src.Build)
			if itemID > 0 {
				taken := w.removeBuildingItemLocked(packTilePos(src.X, src.Y), int16(itemID), 1)
				if taken > 0 {
					t.Build.AddItem(ItemID(itemID), 1)
					state.current = ItemID(itemID)
					state.progress = -1
					moved = true
					changedPos = append(changedPos, packTilePos(src.X, src.Y), pos)
				}
			}
		}
	}
	if state.current == 0 && len(t.Build.Items) > 0 {
		state.current = ItemID(firstItemID(t.Build))
	}
	if state.current == 0 {
		state.progress = 0
		return changedPos, moved, true
	}
	speedFrames := float32(ductRouterSpeedFrames(name))
	if speedFrames <= 0 {
		speedFrames = 5
	}
	inc := dt * ticksPerSecond(w.tps) / speedFrames * 2
	state.progress += inc
	threshold := 1 - 1/speedFrames
	if state.progress >= threshold {
		dst := w.ductRouterTargetLocked(t, name, state.current, sortItemID, state)
		if dst != nil {
			added := w.acceptBuildingItemLocked(packTilePos(dst.X, dst.Y), int16(state.current), 1)
			if added > 0 {
				_ = w.removeBuildingItemLocked(pos, int16(state.current), 1)
				state.current = 0
				state.progress = float32(math.Mod(float64(state.progress), float64(threshold)))
				moved = true
				changedPos = append(changedPos, pos, packTilePos(dst.X, dst.Y))
			}
		}
	}
	return changedPos, moved, true
}

func (w *World) stepStackRouterLocked(t *Tile, name string, dt float32) (changedPos []int32, moved bool, ok bool) {
	if t == nil || t.Build == nil {
		return nil, false, false
	}
	pos := packTilePos(t.X, t.Y)
	state := w.stackRouterStateLocked(pos)
	sortItemID := w.configuredItemIDForBuildLocked(pos)
	props, hasProps := w.blockPropsByName[name]
	if hasProps && props.PowerUse > 0 {
		if !w.consumePowerAtLocked(pos, powerUseAmount(props.PowerUse, dt)) {
			return nil, false, true
		}
	}
	if len(t.Build.Items) == 0 {
		state.current = 0
		state.unloading = false
		state.progress = 0
		return nil, false, true
	}
	if state.current == 0 {
		state.current = ItemID(firstItemID(t.Build))
	}
	cap := int32(10)
	if props.ItemCapacity > 0 {
		cap = props.ItemCapacity
	}
	total := int32(0)
	for i := range t.Build.Items {
		total += t.Build.Items[i].Amount
	}
	if !state.unloading && state.current != 0 && total >= cap {
		state.progress += dt * ticksPerSecond(w.tps)
		if state.progress >= float32(stackRouterSpeedFrames(name)) {
			state.unloading = true
			state.progress = 0
		}
	}
	if state.unloading && state.current != 0 {
		for {
			dst := w.stackRouterTargetLocked(t, name, state.current, sortItemID, state)
			if dst == nil {
				break
			}
			added := w.acceptBuildingItemLocked(packTilePos(dst.X, dst.Y), int16(state.current), 1)
			if added <= 0 {
				break
			}
			_ = w.removeBuildingItemLocked(pos, int16(state.current), 1)
			moved = true
			changedPos = append(changedPos, pos, packTilePos(dst.X, dst.Y))
			if w.buildingItemAmountLocked(t, int16(state.current)) <= 0 {
				state.current = 0
				state.unloading = false
				break
			}
		}
	}
	return changedPos, moved, true
}

func (w *World) stepDuctUnloaderLocked(t *Tile, name string, dt float32) (changedPos []int32, moved bool, ok bool) {
	if t == nil || t.Build == nil {
		return nil, false, false
	}
	pos := packTilePos(t.X, t.Y)
	state := w.ductUnloaderStateLocked(pos)
	speed := float32(ductUnloaderSpeedFrames(name))
	if speed <= 0 {
		speed = 4
	}
	state.timer += dt * ticksPerSecond(w.tps)
	if state.timer < speed {
		return nil, false, true
	}
	state.timer = float32(math.Mod(float64(state.timer), float64(speed)))
	frontDir := int(t.Build.Rotation)
	backDir := (frontDir + 2) % 4
	front := w.nearbyDirLocked(t, frontDir)
	back := w.nearbyDirLocked(t, backDir)
	if front == nil || back == nil || front.Build == nil || back.Build == nil || front.Build.Team != t.Build.Team || back.Build.Team != t.Build.Team {
		return nil, false, true
	}
	rules := w.rulesMgr.Get()
	if rules != nil && !rules.AllowCoreUnloaders && w.isCoreTileLocked(back) {
		return nil, false, true
	}
	itemID := w.configuredItemIDForBuildLocked(pos)
	if itemID <= 0 {
		items := back.Build.Items
		if len(items) == 0 {
			return nil, false, true
		}
		start := state.offset % len(items)
		for i := 0; i < len(items); i++ {
			st := items[(start+i)%len(items)]
			if st.Amount <= 0 {
				continue
			}
			itemID = int16(st.Item)
			state.offset = (start + i + 1) % len(items)
			break
		}
	}
	if itemID <= 0 {
		return nil, false, true
	}
	if !w.canAcceptItemWithDirLocked(front, itemID, frontDir) {
		return nil, false, true
	}
	taken := w.removeBuildingItemLocked(packTilePos(back.X, back.Y), itemID, 1)
	if taken <= 0 {
		return nil, false, true
	}
	added := w.acceptBuildingItemLocked(packTilePos(front.X, front.Y), itemID, taken)
	if added < taken {
		_ = w.acceptBuildingItemLocked(packTilePos(back.X, back.Y), itemID, taken-added)
	}
	if added > 0 {
		changedPos = append(changedPos, packTilePos(back.X, back.Y), packTilePos(front.X, front.Y))
		moved = true
	}
	return changedPos, moved, true
}

func (w *World) dumpOneFromBuildingLocked(t *Tile, preferredItemID int16) (outPos int32, moved int32, ok bool) {
	if t == nil || t.Build == nil || len(t.Build.Items) == 0 {
		return 0, 0, false
	}
	itemID := int16(0)
	if preferredItemID > 0 {
		for _, st := range t.Build.Items {
			if st.Item == ItemID(preferredItemID) && st.Amount > 0 {
				itemID = preferredItemID
				break
			}
		}
	}
	if itemID == 0 {
		for _, st := range t.Build.Items {
			if st.Amount > 0 {
				itemID = int16(st.Item)
				break
			}
		}
	}
	if itemID <= 0 {
		return 0, 0, false
	}
	for dir := 0; dir < 4; dir++ {
		nb := w.nearbyDirLocked(t, dir)
		if nb == nil || nb.Build == nil || nb.Team != t.Team {
			continue
		}
		if !w.canAcceptItemWithDirLocked(nb, itemID, dir) {
			continue
		}
		taken := w.removeBuildingItemLocked(packTilePos(t.X, t.Y), itemID, 1)
		if taken <= 0 {
			return 0, 0, false
		}
		added := w.acceptBuildingItemLocked(packTilePos(nb.X, nb.Y), itemID, taken)
		if added < taken {
			_ = w.acceptBuildingItemLocked(packTilePos(t.X, t.Y), itemID, taken-added)
		}
		if added <= 0 {
			return 0, 0, false
		}
		return packTilePos(nb.X, nb.Y), added, true
	}
	return 0, 0, false
}

func (w *World) bridgePullSourceLocked(bridge *Tile, name string, dst *Tile) (src *Tile, dir int, itemID int16, ok bool) {
	if bridge == nil || bridge.Build == nil {
		return nil, 0, 0, false
	}
	neighbors := w.adjacentBuildingsLocked(bridge.X, bridge.Y)
	if len(neighbors) == 0 {
		return nil, 0, 0, false
	}
	for _, nb := range neighbors {
		if nb == nil || nb.Build == nil || nb.Team != bridge.Team {
			continue
		}
		if dst != nil && nb.X == dst.X && nb.Y == dst.Y {
			continue
		}
		dirToBridge, okDir := w.directionFromSourceToSorterLocked(nb, bridge)
		if !okDir {
			continue
		}
		if !w.transportSourceCanOutputLocked(nb, dirToBridge) {
			continue
		}
		for _, st := range nb.Build.Items {
			if st.Amount <= 0 {
				continue
			}
			itemID := int16(st.Item)
			if itemID <= 0 {
				continue
			}
			if !w.bridgeAcceptFromLocked(bridge, name, nb, dirToBridge) {
				continue
			}
			return nb, dirToBridge, itemID, true
		}
	}
	return nil, 0, 0, false
}

func (w *World) stepSorterOneLocked(sorter *Tile, blockName string) (srcPos int32, dstPos int32, moved int32, ok bool) {
	if sorter == nil || sorter.Build == nil {
		return 0, 0, 0, false
	}
	neighbors := w.adjacentBuildingsLocked(sorter.X, sorter.Y)
	if len(neighbors) == 0 {
		return 0, 0, 0, false
	}
	for _, src := range neighbors {
		if src == nil || src.Build == nil || src.Team != sorter.Team {
			continue
		}
		dir, okDir := w.directionFromSourceToSorterLocked(src, sorter)
		if !okDir {
			continue
		}
		if !w.transportSourceCanOutputLocked(src, dir) {
			continue
		}
		for _, st := range src.Build.Items {
			itemID := int16(st.Item)
			if st.Amount <= 0 || itemID <= 0 {
				continue
			}
			dst := w.sorterTargetLocked(sorter, src, itemID, dir, true, blockName)
			if dst == nil {
				continue
			}
			taken := w.removeBuildingItemLocked(packTilePos(src.X, src.Y), itemID, 1)
			if taken <= 0 {
				return 0, 0, 0, false
			}
			added := w.acceptBuildingItemLocked(packTilePos(dst.X, dst.Y), itemID, taken)
			if added < taken {
				_ = w.acceptBuildingItemLocked(packTilePos(src.X, src.Y), itemID, taken-added)
			}
			if added <= 0 {
				return 0, 0, 0, false
			}
			return packTilePos(src.X, src.Y), packTilePos(dst.X, dst.Y), added, true
		}
	}
	return 0, 0, 0, false
}

func (w *World) stepUnloaderOneLocked(unloader *Tile) (srcPos int32, dstPos int32, moved int32, ok bool) {
	if unloader == nil || unloader.Build == nil {
		return 0, 0, 0, false
	}
	neighbors := w.adjacentBuildingsLocked(unloader.X, unloader.Y)
	if len(neighbors) < 2 {
		return 0, 0, 0, false
	}
	pos := packTilePos(unloader.X, unloader.Y)
	itemID := w.configuredItemIDForBuildLocked(pos)
	if itemID <= 0 {
		itemID = w.nextUnloaderItemLocked(pos, neighbors)
	}
	if itemID <= 0 {
		return 0, 0, 0, false
	}
	var from *Tile
	var to *Tile
	var fromLF float32
	var toLF float32
	var fromCanLoad bool
	rules := w.rulesMgr.Get()
	allowCoreUnloaders := rules == nil || rules.AllowCoreUnloaders
	for _, b := range neighbors {
		if b == nil || b.Build == nil || b.Team != unloader.Team {
			continue
		}
		if !allowCoreUnloaders && w.isCoreTileLocked(b) {
			// Disallow pulling items from core when rule forbids core unloaders.
			continue
		}
		amount := w.buildingItemAmountLocked(b, itemID)
		canUnload := amount > 0
		canLoad := w.acceptBuildingItemAmountLocked(packTilePos(b.X, b.Y), itemID, 1) > 0
		lf := w.buildingItemLoadFactorLocked(b, itemID)
		if canUnload {
			if from == nil || lf > fromLF {
				from = b
				fromLF = lf
				fromCanLoad = canLoad
			}
		}
		if canLoad {
			if to == nil || lf < toLF {
				to = b
				toLF = lf
			}
		}
	}
	if from == nil || to == nil || (from.X == to.X && from.Y == to.Y) {
		return 0, 0, 0, false
	}
	if math.Abs(float64(fromLF-toLF)) < 1e-6 && fromCanLoad {
		return 0, 0, 0, false
	}
	bestSrcPos := packTilePos(from.X, from.Y)
	bestDstPos := packTilePos(to.X, to.Y)
	taken := w.removeBuildingItemLocked(bestSrcPos, itemID, 1)
	if taken <= 0 {
		return 0, 0, 0, false
	}
	added := w.acceptBuildingItemLocked(bestDstPos, itemID, taken)
	if added < taken {
		_ = w.acceptBuildingItemLocked(bestSrcPos, itemID, taken-added)
	}
	if added <= 0 {
		return 0, 0, 0, false
	}
	return bestSrcPos, bestDstPos, added, true
}

func (w *World) nextUnloaderItemLocked(unloaderPos int32, neighbors []*Tile) int16 {
	filterItemID := w.configuredItemIDForBuildLocked(unloaderPos)
	if filterItemID > 0 {
		if w.unloaderCanTradeItemLocked(neighbors, filterItemID) {
			return filterItemID
		}
		return 0
	}
	if w.unloaderRotations == nil {
		w.unloaderRotations = map[int32]int16{}
	}
	last := w.unloaderRotations[unloaderPos]
	bestAfter := int16(0)
	bestAny := int16(0)
	for _, b := range neighbors {
		if b == nil || b.Build == nil {
			continue
		}
		for _, st := range b.Build.Items {
			id := int16(st.Item)
			if st.Amount <= 0 || id <= 0 {
				continue
			}
			if !w.unloaderCanTradeItemLocked(neighbors, id) {
				continue
			}
			if bestAny == 0 || id < bestAny {
				bestAny = id
			}
			if id > last && (bestAfter == 0 || id < bestAfter) {
				bestAfter = id
			}
		}
	}
	chosen := bestAfter
	if chosen <= 0 {
		chosen = bestAny
	}
	if chosen > 0 {
		w.unloaderRotations[unloaderPos] = chosen
	}
	return chosen
}

func (w *World) unloaderCanTradeItemLocked(neighbors []*Tile, itemID int16) bool {
	hasProvider := false
	hasReceiver := false
	rules := w.rulesMgr.Get()
	allowCoreUnloaders := rules == nil || rules.AllowCoreUnloaders
	for _, b := range neighbors {
		if b == nil || b.Build == nil {
			continue
		}
		if !allowCoreUnloaders && w.isCoreTileLocked(b) {
			// core can receive but cannot provide when unloaders are disabled
		} else if w.buildingItemAmountLocked(b, itemID) > 0 {
			hasProvider = true
		}
		if w.acceptBuildingItemAmountLocked(packTilePos(b.X, b.Y), itemID, 1) > 0 {
			hasReceiver = true
		}
	}
	return hasProvider && hasReceiver
}

func (w *World) buildingItemAmountLocked(t *Tile, itemID int16) int32 {
	if t == nil || t.Build == nil || itemID <= 0 {
		return 0
	}
	for _, st := range t.Build.Items {
		if st.Item == ItemID(itemID) && st.Amount > 0 {
			return st.Amount
		}
	}
	return 0
}

func (w *World) buildingLiquidAmountLocked(t *Tile, liquidID int16) float32 {
	if t == nil || t.Build == nil || liquidID <= 0 {
		return 0
	}
	for _, st := range t.Build.Liquids {
		if st.Liquid == LiquidID(liquidID) && st.Amount > 0 {
			return st.Amount
		}
	}
	return 0
}

func (w *World) acceptBuildingLiquidLocked(buildPos int32, liquidID int16, amount float32) float32 {
	if amount <= 0 || liquidID <= 0 {
		return 0
	}
	t, ok := w.tileForPosLocked(buildPos)
	if !ok {
		return 0
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return 0
	}
	capacity := float32(0)
	if w.blockNamesByID != nil {
		if name, ok := w.blockNamesByID[int16(t.Block)]; ok {
			if props, ok := w.blockPropsByName[strings.ToLower(strings.TrimSpace(name))]; ok && props.LiquidCapacity > 0 {
				capacity = props.LiquidCapacity
			}
		}
	}
	if capacity > 0 {
		total := float32(0)
		for i := range b.Liquids {
			if b.Liquids[i].Amount > 0 {
				total += b.Liquids[i].Amount
			}
		}
		space := capacity - total
		if space <= 0 {
			return 0
		}
		if amount > space {
			amount = space
		}
	}
	for i := range b.Liquids {
		if b.Liquids[i].Liquid == LiquidID(liquidID) {
			b.Liquids[i].Amount += amount
			return amount
		}
	}
	b.Liquids = append(b.Liquids, LiquidStack{Liquid: LiquidID(liquidID), Amount: amount})
	return amount
}

func (w *World) removeBuildingLiquidLocked(buildPos int32, liquidID int16, amount float32) float32 {
	if amount <= 0 || liquidID <= 0 {
		return 0
	}
	t, ok := w.tileForPosLocked(buildPos)
	if !ok {
		return 0
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return 0
	}
	for i := range b.Liquids {
		if b.Liquids[i].Liquid == LiquidID(liquidID) {
			if b.Liquids[i].Amount <= 0 {
				return 0
			}
			removed := amount
			if b.Liquids[i].Amount < removed {
				removed = b.Liquids[i].Amount
			}
			b.Liquids[i].Amount -= removed
			if b.Liquids[i].Amount <= 0 {
				b.Liquids = append(b.Liquids[:i], b.Liquids[i+1:]...)
			}
			return removed
		}
	}
	return 0
}

func (w *World) buildingItemLoadFactorLocked(t *Tile, itemID int16) float32 {
	if t == nil || t.Build == nil || itemID <= 0 {
		return 0
	}
	amount := w.buildingItemAmountLocked(t, itemID)
	capacity := w.buildingMaxAcceptedLocked(t)
	if capacity <= 0 {
		return float32(amount)
	}
	return float32(amount) / float32(capacity)
}

func (w *World) buildingMaxAcceptedLocked(t *Tile) int32 {
	if t == nil || t.Build == nil || t.Block == 0 {
		return 0
	}
	if w.blockNamesByID == nil {
		return 0
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNamesByID[int16(t.Block)]))
	if props, ok := w.blockPropsByName[name]; ok && props.ItemCapacity > 0 {
		return props.ItemCapacity
	}
	return buildingItemCapacityByName[name]
}

func (w *World) isCoreTileLocked(t *Tile) bool {
	if t == nil || t.Block == 0 {
		return false
	}
	name := w.blockNameForTileLocked(t)
	if name == "" && w.blockNamesByID != nil {
		name = strings.ToLower(strings.TrimSpace(w.blockNamesByID[int16(t.Block)]))
	}
	return strings.Contains(name, "core")
}

func (w *World) sorterTargetLocked(sorter *Tile, source *Tile, itemID int16, dir int, flip bool, blockName string) *Tile {
	if sorter == nil || source == nil || itemID <= 0 {
		return nil
	}
	filterItemID := w.configuredItemIDForBuildLocked(packTilePos(sorter.X, sorter.Y))
	invert := strings.Contains(strings.ToLower(strings.TrimSpace(blockName)), "inverted-sorter")
	match := false
	if filterItemID > 0 {
		match = itemID == filterItemID
	}
	if invert {
		match = !match
	}
	if match {
		to := w.nearbyDirLocked(sorter, dir)
		if w.sorterCanOutputToLocked(sorter, source, to, itemID) {
			return to
		}
		return nil
	}
	a := w.nearbyDirLocked(sorter, (dir+3)%4)
	b := w.nearbyDirLocked(sorter, (dir+1)%4)
	ac := w.sorterCanOutputToLocked(sorter, source, a, itemID)
	bc := w.sorterCanOutputToLocked(sorter, source, b, itemID)
	if ac && !bc {
		return a
	}
	if bc && !ac {
		return b
	}
	if !bc {
		return nil
	}
	pos := packTilePos(sorter.X, sorter.Y)
	if w.sorterRouteBits == nil {
		w.sorterRouteBits = map[int32]byte{}
	}
	mask := w.sorterRouteBits[pos]
	useA := (mask & (1 << uint(dir))) == 0
	if flip {
		mask ^= (1 << uint(dir))
		w.sorterRouteBits[pos] = mask
	}
	if useA {
		return a
	}
	return b
}

func (w *World) sorterCanOutputToLocked(sorter *Tile, source *Tile, to *Tile, itemID int16) bool {
	if sorter == nil || source == nil || to == nil || to.Build == nil || to.Team != sorter.Team {
		return false
	}
	if to.X == source.X && to.Y == source.Y {
		return false
	}
	return w.acceptBuildingItemAmountLocked(packTilePos(to.X, to.Y), itemID, 1) > 0
}

func (w *World) nearbyDirLocked(t *Tile, dir int) *Tile {
	if t == nil || w.model == nil {
		return nil
	}
	src := w.resolveBuildTileLocked(t.X, t.Y)
	if src == nil {
		src = t
	}
	minX, maxX, minY, maxY, _ := w.tileFootprintLocked(src)
	switch (dir%4 + 4) % 4 {
	case 0:
		nx := maxX + 1
		for y := minY; y <= maxY; y++ {
			if !w.model.InBounds(nx, y) {
				continue
			}
			if tt := w.resolveBuildTileLocked(nx, y); tt != nil && tt.Build != nil {
				return tt
			}
		}
	case 1:
		ny := maxY + 1
		for x := minX; x <= maxX; x++ {
			if !w.model.InBounds(x, ny) {
				continue
			}
			if tt := w.resolveBuildTileLocked(x, ny); tt != nil && tt.Build != nil {
				return tt
			}
		}
	case 2:
		nx := minX - 1
		for y := minY; y <= maxY; y++ {
			if !w.model.InBounds(nx, y) {
				continue
			}
			if tt := w.resolveBuildTileLocked(nx, y); tt != nil && tt.Build != nil {
				return tt
			}
		}
	default:
		ny := minY - 1
		for x := minX; x <= maxX; x++ {
			if !w.model.InBounds(x, ny) {
				continue
			}
			if tt := w.resolveBuildTileLocked(x, ny); tt != nil && tt.Build != nil {
				return tt
			}
		}
	}
	return nil
}

func (w *World) tileAtDirLocked(t *Tile, dir int) *Tile {
	if t == nil || w.model == nil {
		return nil
	}
	dx, dy := dirToDelta(dir)
	nx, ny := t.X+dx, t.Y+dy
	if !w.model.InBounds(nx, ny) {
		return nil
	}
	nt, err := w.model.TileAt(nx, ny)
	if err != nil {
		return nil
	}
	return nt
}

func rangesOverlap(minA, maxA, minB, maxB int) bool {
	return maxA >= minB && maxB >= minA
}

func (w *World) directionFromSourceToSorterLocked(source *Tile, target *Tile) (int, bool) {
	if source == nil || target == nil {
		return 0, false
	}
	src := w.resolveBuildTileLocked(source.X, source.Y)
	if src == nil {
		src = source
	}
	dst := w.resolveBuildTileLocked(target.X, target.Y)
	if dst == nil {
		dst = target
	}
	sMinX, sMaxX, sMinY, sMaxY, _ := w.tileFootprintLocked(src)
	tMinX, tMaxX, tMinY, tMaxY, _ := w.tileFootprintLocked(dst)
	if tMinX == sMaxX+1 && rangesOverlap(sMinY, sMaxY, tMinY, tMaxY) {
		return 0, true
	}
	if tMaxX == sMinX-1 && rangesOverlap(sMinY, sMaxY, tMinY, tMaxY) {
		return 2, true
	}
	if tMinY == sMaxY+1 && rangesOverlap(sMinX, sMaxX, tMinX, tMaxX) {
		return 1, true
	}
	if tMaxY == sMinY-1 && rangesOverlap(sMinX, sMaxX, tMinX, tMaxX) {
		return 3, true
	}
	return directionFromSourceToSorter(src.X, src.Y, dst.X, dst.Y)
}

func directionFromSourceToSorter(sx, sy, tx, ty int) (int, bool) {
	dx, dy := tx-sx, ty-sy
	switch {
	case dx == 1 && dy == 0:
		return 0, true
	case dx == 0 && dy == 1:
		return 1, true
	case dx == -1 && dy == 0:
		return 2, true
	case dx == 0 && dy == -1:
		return 3, true
	default:
		return 0, false
	}
}

func directionFromTo(sx, sy, tx, ty int) (int, bool) {
	dx, dy := tx-sx, ty-sy
	switch {
	case dx > 0 && dy == 0:
		return 0, true
	case dx == 0 && dy > 0:
		return 1, true
	case dx < 0 && dy == 0:
		return 2, true
	case dx == 0 && dy < 0:
		return 3, true
	default:
		return 0, false
	}
}

func dirToDelta(dir int) (int, int) {
	switch (dir%4 + 4) % 4 {
	case 0:
		return 1, 0
	case 1:
		return 0, 1
	case 2:
		return -1, 0
	default:
		return 0, -1
	}
}

func (w *World) configuredItemIDForBuildLocked(buildPos int32) int16 {
	if w.tileConfigValues == nil {
		return 0
	}
	v, ok := w.tileConfigValues[buildPos]
	if !ok || v == nil {
		return 0
	}
	switch x := v.(type) {
	case int:
		return int16(x)
	case int8:
		return int16(x)
	case int16:
		return x
	case int32:
		return int16(x)
	case int64:
		return int16(x)
	case uint:
		return int16(x)
	case uint8:
		return int16(x)
	case uint16:
		return int16(x)
	case uint32:
		return int16(x)
	case uint64:
		return int16(x)
	default:
		if ref, ok := v.(interface{ ID() int16 }); ok {
			return ref.ID()
		}
	}
	return 0
}

func (w *World) configuredBuildPosForBuildLocked(buildPos int32) (int32, bool) {
	if w.tileConfigValues == nil {
		return 0, false
	}
	v, ok := w.tileConfigValues[buildPos]
	if !ok || v == nil {
		return 0, false
	}
	switch x := v.(type) {
	case int:
		return int32(x), true
	case int8:
		return int32(x), true
	case int16:
		return int32(x), true
	case int32:
		return x, true
	case int64:
		return int32(x), true
	case uint:
		return int32(x), true
	case uint8:
		return int32(x), true
	case uint16:
		return int32(x), true
	case uint32:
		return int32(x), true
	case uint64:
		return int32(x), true
	default:
		if ref, ok := v.(interface{ Pos() int32 }); ok {
			return ref.Pos(), true
		}
	}
	return 0, false
}

func (w *World) bridgeLinkAllowed(blockName string, fromPos int32, toPos int32) bool {
	from := protocol.UnpackPoint2(fromPos)
	to := protocol.UnpackPoint2(toPos)
	dx := from.X - to.X
	if dx < 0 {
		dx = -dx
	}
	dy := from.Y - to.Y
	if dy < 0 {
		dy = -dy
	}
	// Bridge-like transport links are axial in vanilla.
	if dx != 0 && dy != 0 {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(blockName))
	maxRange := int32(4)
	if w.blockPropsByName != nil {
		if props, ok := w.blockPropsByName[name]; ok && props.LinkRangeTiles > 0 {
			maxRange = int32(math.Round(float64(props.LinkRangeTiles)))
		}
	}
	switch {
	case w != nil:
		if k, ok := w.blockKindByName(name); ok {
			switch k.Kind {
			case "phase-conveyor":
				maxRange = int32(12)
			case "bridge":
				maxRange = int32(4)
			case "duct-bridge":
				maxRange = int32(4)
			case "phase-conduit":
				maxRange = int32(12)
			case "bridge-conduit":
				maxRange = int32(4)
			}
			break
		}
		fallthrough
	case strings.Contains(name, "phase-conveyor"):
		maxRange = int32(12)
	case strings.Contains(name, "bridge-conveyor"):
		maxRange = int32(4)
	case strings.Contains(name, "duct-bridge"):
		maxRange = int32(4)
	case strings.Contains(name, "phase-conduit"):
		maxRange = int32(12)
	case strings.Contains(name, "bridge-conduit"):
		maxRange = int32(4)
	}
	return dx <= maxRange && dy <= maxRange
}

func (w *World) nextWaveSpacingSec() float32 {
	rules := w.rulesMgr.Get()
	if rules == nil {
		return 90
	}
	// Before first triggered wave, prefer initial spacing when configured.
	if w.wave <= 1 && rules.InitialWaveSpacing > 0 {
		return rules.InitialWaveSpacing
	}
	if rules.WaveSpacing > 0 {
		return rules.WaveSpacing
	}
	return 90
}

func (w *World) Snapshot() Snapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return Snapshot{
		WaveTime: w.waveTime,
		Wave:     w.wave,
		Enemies:  0,
		Paused:   false,
		GameOver: false,
		TimeData: int32(time.Since(w.start).Seconds()),
		Tps:      w.tps,
		Rand0:    w.rand0,
		Rand1:    w.rand1,
		Tick:     w.tick,
	}
}

func (w *World) ApplySnapshot(s Snapshot) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.waveTime = s.WaveTime
	if w.waveTime < 0 {
		w.waveTime = 0
	}
	if s.Wave > 0 {
		w.wave = s.Wave
	}
	w.tick = s.Tick
	w.rand0 = s.Rand0
	w.rand1 = s.Rand1
	if s.Tps > 0 {
		w.tps = s.Tps
	}
	if s.TimeData > 0 {
		w.start = time.Now().Add(-time.Duration(s.TimeData) * time.Second)
	} else {
		w.start = time.Now()
	}
}

func (w *World) SetModel(m *WorldModel) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.model = m
	w.buildStates = map[int32]buildCombatState{}
	w.pendingBuilds = map[int32]pendingBuildState{}
	w.unitMountCDs = map[int32][]float32{}
	w.unitTargets = map[int32]targetTrackState{}
	w.unitBurstStates = map[int32]unitBurstState{}
	w.unitMountBursts = map[int32][]unitBurstState{}
	w.tileConfigValues = map[int32]any{}
	w.sorterRouteBits = map[int32]byte{}
	w.routerRouteBits = map[int32]byte{}
	w.routerInputDirs = map[int32]int8{}
	w.junctionInputDirs = map[int32]int8{}
	w.unloaderRotations = map[int32]int16{}
	w.massDriverStates = map[int32]massDriverState{}
	w.payloadRouterRouteBits = map[int32]byte{}
	w.payloadRouterInputDirs = map[int32]int8{}
	w.payloadMassStates = map[int32]massDriverState{}
	w.liquidJunctionInputDirs = map[int32]int8{}
	w.bridgeProgress = map[int32]float32{}
	w.bridgeBuffers = map[int32][]bridgeBufferItem{}
	w.junctionStates = map[int32]*junctionState{}
	w.ductJunctionStates = map[int32]*ductJunctionState{}
	w.ductRouterStates = map[int32]*ductRouterState{}
	w.stackRouterStates = map[int32]*stackRouterState{}
	w.ductUnloaderStates = map[int32]*ductUnloaderState{}
	w.craftStates = map[int32]craftState{}
	w.drillStates = map[int32]craftState{}
	w.logicRuntime = map[int32]*logicRuntime{}
	w.logicMemory = map[int32]*logic.MlogCell{}
	w.logicUnitFlags = map[int32]float64{}
	w.logicProcessorPos = nil
	w.logicDisplayBuffers = map[int32][]uint64{}
	w.logicClientData = nil
	w.logicSyncVars = nil
	w.blockNamesByID = nil
	w.unitNamesByID = nil
	w.unitTypeDefsByID = nil
	w.powerNetByPos = map[int32]*powerNet{}
	w.powerStatusByPos = map[int32]float32{}
	w.powerStoredByPos = map[int32]float32{}
	w.powerRequests = map[int32]float32{}
	w.powerLastDt = 0

	// 从 tags 解析规则并应用
	if m != nil && m.Tags != nil {
		if rulesJSON, ok := m.Tags["rules"]; ok && rulesJSON != "" {
			w.rulesMgr.FromJSON([]byte(rulesJSON))
			// 应用倍率到现有单位和建筑
			w.applyRulesToEntities()
		}
	}
	w.timeMgr = NewWorldTime()
	w.lastWeatherStartTick = 0
	w.syncWeatherEntriesLocked()
	w.resolveRecipesLocked()
	w.resolveBlockPropsLocked()
	w.resolveBlockBuildsLocked()
	w.resolveBlockKindsLocked()

	if m != nil && len(m.BlockNames) > 0 {
		w.blockNamesByID = make(map[int16]string, len(m.BlockNames))
		for k, v := range m.BlockNames {
			w.blockNamesByID[k] = strings.ToLower(strings.TrimSpace(v))
		}
	}
	if m != nil && len(m.UnitNames) > 0 {
		w.unitNamesByID = make(map[int16]string, len(m.UnitNames))
		for k, v := range m.UnitNames {
			w.unitNamesByID[k] = strings.ToLower(strings.TrimSpace(v))
		}
		w.unitTypeDefsByID = make(map[int16]vanilla.UnitTypeDef, len(m.UnitNames))
		for id, name := range w.unitNamesByID {
			if def, ok := vanilla.UnitTypesByName[name]; ok {
				w.unitTypeDefsByID[id] = def
			}
		}
	}
	w.resolveBlockBuildsLocked()
	w.resolveBlockKindsLocked()
}

// SetWeatherNamesByID wires weather name/id mappings for rule lookups.
func (w *World) SetWeatherNamesByID(names map[int16]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.weatherNamesByID = nil
	w.weatherIDsByName = nil
	if len(names) == 0 {
		w.syncWeatherEntriesLocked()
		return
	}
	w.weatherNamesByID = make(map[int16]string, len(names))
	w.weatherIDsByName = make(map[string]int16, len(names))
	for id, name := range names {
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "" {
			continue
		}
		w.weatherNamesByID[id] = n
		w.weatherIDsByName[n] = id
	}
	w.syncWeatherEntriesLocked()
}

func (w *World) SetItemNamesByID(names map[int16]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.itemNamesByID = nil
	w.itemIDsByName = nil
	if len(names) == 0 {
		w.resolveRecipesLocked()
		w.resolveBlockPropsLocked()
		w.resolveBlockBuildsLocked()
		w.resolveBlockKindsLocked()
		w.resolveItemPropsLocked()
		return
	}
	w.itemNamesByID = make(map[int16]string, len(names))
	w.itemIDsByName = make(map[string]int16, len(names))
	for id, name := range names {
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "" {
			continue
		}
		w.itemNamesByID[id] = n
		w.itemIDsByName[n] = id
	}
	w.resolveRecipesLocked()
	w.resolveBlockPropsLocked()
	w.resolveBlockBuildsLocked()
	w.resolveBlockKindsLocked()
	w.resolveItemPropsLocked()
}

func (w *World) SetLiquidNamesByID(names map[int16]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.liquidNamesByID = nil
	w.liquidIDsByName = nil
	if len(names) == 0 {
		w.resolveRecipesLocked()
		w.resolveBlockPropsLocked()
		w.resolveLiquidPropsLocked()
		return
	}
	w.liquidNamesByID = make(map[int16]string, len(names))
	w.liquidIDsByName = make(map[string]int16, len(names))
	for id, name := range names {
		n := strings.ToLower(strings.TrimSpace(name))
		if n == "" {
			continue
		}
		w.liquidNamesByID[id] = n
		w.liquidIDsByName[n] = id
	}
	w.resolveRecipesLocked()
	w.resolveBlockPropsLocked()
	w.resolveLiquidPropsLocked()
}

// SetTypeIO wires a TypeIO context for decoding config payloads (logic, etc).
func (w *World) SetTypeIO(ctx *protocol.TypeIOContext) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.typeIO = ctx
}

// SetContentRegistry wires content registry for logic lookup/sensors.
func (w *World) SetContentRegistry(reg *protocol.ContentRegistry) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.content = reg
	if w.logicIDs != nil {
		w.logicIDs.bindContent(reg)
	}
}

// SetLogicIDs stores logic ID mappings from content_ids.json (logicids.dat).
func (w *World) SetLogicIDs(ids *vanilla.ContentIDsFile) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.logicIDs = newLogicIDMaps(ids)
	if w.content != nil {
		w.logicIDs.bindContent(w.content)
	}
}

func (w *World) syncWeatherEntriesLocked() {
	if w.timeMgr == nil {
		w.timeMgr = NewWorldTime()
	}
	rules := w.rulesMgr.Get()
	if rules == nil || len(rules.Weather) == 0 {
		w.timeMgr.WeatherEntries = nil
		return
	}
	out := make([]WeatherEntry, 0, len(rules.Weather))
	for _, entry := range rules.Weather {
		e := entry
		name := strings.ToLower(strings.TrimSpace(e.Name))
		if e.ID == 0 && name != "" && w.weatherIDsByName != nil {
			if id, ok := w.weatherIDsByName[name]; ok {
				e.ID = id
			}
		}
		if e.Name == "" && e.ID != 0 && w.weatherNamesByID != nil {
			if nm, ok := w.weatherNamesByID[e.ID]; ok {
				e.Name = nm
			}
		}
		if e.Type == 0 && name != "" {
			e.Type = weatherTypeFromName(name)
		}
		out = append(out, e)
	}
	w.timeMgr.WeatherEntries = out
}

func weatherTypeFromName(name string) WeatherType {
	if name == "" {
		return WeatherClear
	}
	switch {
	case strings.Contains(name, "snow"):
		return WeatherSnow
	case strings.Contains(name, "rain"):
		return WeatherRain
	case strings.Contains(name, "sand"):
		return WeatherSandstorm
	case strings.Contains(name, "slag"):
		return WeatherSlag
	case strings.Contains(name, "fog"):
		return WeatherFog
	default:
		return WeatherClear
	}
}

func (w *World) stepWeatherLocked(dt float32) {
	if w.timeMgr == nil {
		w.timeMgr = NewWorldTime()
	}
	prevStart := w.lastWeatherStartTick
	w.timeMgr.Update(dt)
	if w.timeMgr.Weather == nil {
		return
	}
	if w.timeMgr.Weather.StartTick == 0 || w.timeMgr.Weather.StartTick == prevStart {
		return
	}
	w.lastWeatherStartTick = w.timeMgr.Weather.StartTick
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind: EntityEventWeather,
		Weather: WeatherEvent{
			ID:        w.timeMgr.Weather.ID,
			Name:      w.timeMgr.Weather.Name,
			Intensity: w.timeMgr.Weather.Intensity,
			Duration:  w.timeMgr.Weather.Duration,
			WindX:     w.timeMgr.Weather.WindX,
			WindY:     w.timeMgr.Weather.WindY,
		},
	})
}

// ResetRuntimeFromTags reinitializes wave/timer/tick runtime state from map tags.
// It should be called when switching to a different map to avoid carrying over
// runtime progression from the previous map.
func (w *World) ResetRuntimeFromTags(tags map[string]string) {
	w.mu.Lock()
	defer w.mu.Unlock()

	w.wave = 1
	w.waveTime = w.nextWaveSpacingSec()
	w.tick = 0
	w.start = time.Now()

	if tags == nil {
		return
	}
	if v := strings.TrimSpace(tags["wave"]); v != "" {
		if n, err := strconv.ParseInt(v, 10, 32); err == nil && n > 0 {
			w.wave = int32(n)
		}
	}
	if v := strings.TrimSpace(tags["wavetime"]); v != "" {
		if n, err := strconv.ParseFloat(v, 32); err == nil {
			w.waveTime = float32(n)
			if w.waveTime < 0 {
				w.waveTime = 0
			}
		}
	}
	if v := strings.TrimSpace(tags["tick"]); v != "" {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			w.tick = n
		}
	}
}

func (w *World) stepPendingBuilds(delta time.Duration) {
	if w.model == nil || len(w.pendingBuilds) == 0 {
		return
	}
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return
	}
	rules := w.rulesMgr.Get()
	buildSpeed := float32(1)
	if rules != nil {
		if rules.BuildSpeedMultiplier > 0 {
			buildSpeed *= rules.BuildSpeedMultiplier
		}
		if rules.UnitBuildSpeedMultiplier > 0 {
			buildSpeed *= rules.UnitBuildSpeedMultiplier
		}
	}
	if buildSpeed <= 0 {
		buildSpeed = 1
	}
	for pos, st := range w.pendingBuilds {
		if st.BuildCost <= 0 || st.BuildTime <= 0 {
			req, buildTime, buildCost := w.buildDataForBlockLocked(st.BlockID, rules)
			if len(st.Req) == 0 && len(req) > 0 {
				st.Req = req
			}
			if st.BuildTime <= 0 {
				st.BuildTime = buildTime
			}
			if st.BuildCost <= 0 {
				st.BuildCost = buildCost
			}
		}
		if !st.Breaking && len(st.Spent) != len(st.Req) {
			st.Spent = make([]int32, len(st.Req))
		}
		if st.Breaking && len(st.Refunded) != len(st.Req) {
			st.Refunded = make([]int32, len(st.Req))
		}
		planSpeed := st.BuildSpeed
		if planSpeed <= 0 {
			planSpeed = 1
		}
		progressInc := dt * 60 * buildSpeed * planSpeed / clampBuildCost(st.BuildCost)
		if progressInc < 0 {
			progressInc = 0
		}
		target := st.Progress + progressInc
		if target > 1 {
			target = 1
		}
		if !st.Breaking {
			if rules == nil || !rules.InfiniteResources {
				maxByItems := float32(1)
				if len(st.Req) > 0 {
					for i, req := range st.Req {
						if req.Amount <= 0 {
							continue
						}
						want := int32(math.Round(float64(float32(req.Amount) * target)))
						if want < 0 {
							want = 0
						}
						need := want - st.Spent[i]
						if need > 0 {
							got := w.takeFromTeamCoreLocked(st.Team, req.Item, need)
							st.Spent[i] += got
						}
						if req.Amount > 0 {
							p := float32(st.Spent[i]) / float32(req.Amount)
							if p < maxByItems {
								maxByItems = p
							}
						}
					}
				}
				if maxByItems < target {
					target = maxByItems
				}
			}
		} else if rules != nil && rules.DeconstructRefundMultiplier > 0 {
			refundMult := rules.DeconstructRefundMultiplier
			for i, req := range st.Req {
				if req.Amount <= 0 {
					continue
				}
				want := int32(math.Round(float64(float32(req.Amount) * refundMult * target)))
				if want < 0 {
					want = 0
				}
				need := want - st.Refunded[i]
				if need > 0 {
					_ = w.refundToTeamCoreLocked(st.Team, req.Item, need)
					st.Refunded[i] += need
				}
			}
		}
		if st.Breaking && len(st.Req) == 0 {
			target = 1
		}
		if target < st.Progress {
			target = st.Progress
		}
		st.Progress = target

		x := int(pos % int32(w.model.Width))
		y := int(pos / int32(w.model.Width))
		if !w.model.InBounds(x, y) {
			delete(w.pendingBuilds, pos)
			continue
		}
		tile, err := w.model.TileAt(x, y)
		if err != nil || tile == nil {
			delete(w.pendingBuilds, pos)
			continue
		}
		if st.MaxHealth <= 1 {
			st.MaxHealth = 1000
		}

		shouldEmit := st.Progress >= 1 || st.LastEmit == 0 || st.Progress-st.LastEmit >= 0.03
		if st.Breaking {
			if tile.Build == nil || tile.Block == 0 {
				delete(w.pendingBuilds, pos)
				continue
			}
			nextHP := st.MaxHealth * (1 - st.Progress)
			if nextHP < 1 {
				nextHP = 1
			}
			tile.Build.Health = nextHP
			if shouldEmit {
				st.LastEmit = st.Progress
				w.entityEvents = append(w.entityEvents, EntityEvent{
					Kind:     EntityEventBuildHealth,
					BuildPos: packTilePos(tile.X, tile.Y),
					BuildHP:  tile.Build.Health,
				})
			}
			if st.Progress >= 1 {
				teamOld := tile.Team
				var dropped []ItemStack
				if tile.Build != nil {
					teamOld = tile.Build.Team
					if len(tile.Build.Items) > 0 {
						dropped = append([]ItemStack(nil), tile.Build.Items...)
					}
				}
				tile.Build = nil
				tile.Block = 0
				tile.Team = 0
				tile.Rotation = 0
				delete(w.buildStates, pos)
				delete(w.tileConfigValues, packTilePos(tile.X, tile.Y))
				w.entityEvents = append(w.entityEvents, EntityEvent{
					Kind:       EntityEventBuildDestroyed,
					BuildPos:   packTilePos(tile.X, tile.Y),
					BuildTeam:  teamOld,
					BuildItems: dropped,
				})
				delete(w.pendingBuilds, pos)
				continue
			}
			w.pendingBuilds[pos] = st
			continue
		}

		// Build flow: tile exists immediately with increasing health for smoother client animation.
		tile.Block = BlockID(st.BlockID)
		tile.Team = st.Team
		tile.Rotation = st.Rotation
		if tile.Build == nil {
			maxHP := st.MaxHealth
			if maxHP <= 1 {
				maxHP = estimateBuildMaxHealth(st.BlockID, w.model)
			}
			tile.Build = &Building{
				Block:     tile.Block,
				Team:      st.Team,
				Rotation:  st.Rotation,
				X:         tile.X,
				Y:         tile.Y,
				Health:    1,
				MaxHealth: maxHP,
			}
		}
		tile.Build.Block = tile.Block
		tile.Build.Team = st.Team
		tile.Build.Rotation = st.Rotation
		tile.Build.X = tile.X
		tile.Build.Y = tile.Y
		tile.Build.Health = 1 + st.Progress*(st.MaxHealth-1)
		if tile.Build.Health > st.MaxHealth {
			tile.Build.Health = st.MaxHealth
		}
		tile.Build.MaxHealth = st.MaxHealth
		if shouldEmit {
			st.LastEmit = st.Progress
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:     EntityEventBuildHealth,
				BuildPos: packTilePos(tile.X, tile.Y),
				BuildHP:  tile.Build.Health,
			})
		}
		if st.Progress >= 1 {
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:       EntityEventBuildPlaced,
				BuildPos:   packTilePos(tile.X, tile.Y),
				BuildTeam:  tile.Team,
				BuildBlock: int16(tile.Block),
				BuildRot:   tile.Rotation,
				BuildHP:    tile.Build.Health,
			})
			delete(w.pendingBuilds, pos)
			continue
		}
		w.pendingBuilds[pos] = st
	}
}

func normalizeUnitName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	name = strings.ReplaceAll(name, "_", "")
	name = strings.ReplaceAll(name, "-", "")
	name = strings.ReplaceAll(name, " ", "")
	return name
}

// ResolveUnitTypeID accepts either a numeric type id string or a unit name
// like "alpha", "mono", "nova" and resolves it to type id.
func (w *World) ResolveUnitTypeID(arg string) (int16, bool) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 0, false
	}
	if v, err := strconv.ParseInt(arg, 10, 16); err == nil {
		return int16(v), true
	}
	want := normalizeUnitName(arg)
	w.mu.RLock()
	defer w.mu.RUnlock()
	for id, name := range w.unitNamesByID {
		if normalizeUnitName(name) == want {
			return id, true
		}
	}
	return 0, false
}

func (w *World) UnitNameByTypeID(typeID int16) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.unitNamesByID == nil {
		return ""
	}
	return w.unitNamesByID[typeID]
}

func (w *World) LoadVanillaProfiles(path string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if strings.TrimSpace(path) == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var payload vanillaProfilesFile
	if err := json.Unmarshal(data, &payload); err != nil {
		return err
	}
	if len(payload.Units) > 0 {
		base := cloneUnitWeaponProfiles(weaponProfilesByType)
		byName := cloneUnitWeaponProfilesByName(w.unitProfilesByName)
		for _, u := range payload.Units {
			name := strings.ToLower(strings.TrimSpace(u.Name))
			if name != "" {
				pn := defaultWeaponProfile
				if cur, ok := byName[name]; ok {
					pn = cur
				}
				mergeUnitProfile(&pn, u)
				byName[name] = pn
			}
			if u.TypeID >= 0 {
				p := defaultWeaponProfile
				if cur, ok := base[u.TypeID]; ok {
					p = cur
				}
				mergeUnitProfile(&p, u)
				base[u.TypeID] = p
				if u.HitRadius > 0 {
					entityHitRadiusByType[u.TypeID] = u.HitRadius
				}
			}
		}
		w.unitProfilesByType = base
		w.unitProfilesByName = byName
	}
	if len(payload.UnitsByName) > 0 {
		base := cloneUnitWeaponProfilesByName(w.unitProfilesByName)
		for _, u := range payload.UnitsByName {
			name := strings.ToLower(strings.TrimSpace(u.Name))
			if name == "" {
				continue
			}
			p := defaultWeaponProfile
			if cur, ok := base[name]; ok {
				p = cur
			}
			mergeUnitProfile(&p, u)
			base[name] = p
		}
		w.unitProfilesByName = base
	}
	if len(payload.Turrets) > 0 {
		base := cloneBuildingWeaponProfiles(buildingWeaponProfilesByName)
		for _, t := range payload.Turrets {
			name := strings.ToLower(strings.TrimSpace(t.Name))
			if name == "" {
				continue
			}
			p := buildingWeaponProfile{}
			if cur, ok := base[name]; ok {
				p = cur
			}
			mergeBuildingProfile(&p, t)
			base[name] = p
		}
		w.buildingProfilesByName = base
	}
	return nil
}

func cloneUnitWeaponProfiles(src map[int16]weaponProfile) map[int16]weaponProfile {
	out := make(map[int16]weaponProfile, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneBuildingWeaponProfiles(src map[string]buildingWeaponProfile) map[string]buildingWeaponProfile {
	out := make(map[string]buildingWeaponProfile, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func cloneUnitWeaponProfilesByName(src map[string]weaponProfile) map[string]weaponProfile {
	out := make(map[string]weaponProfile, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func mergeUnitProfile(p *weaponProfile, u vanillaUnitProfile) {
	if p == nil {
		return
	}
	if strings.TrimSpace(u.FireMode) != "" {
		mode := strings.TrimSpace(u.FireMode)
		if strings.EqualFold(mode, "default") {
			// Keep defaults and allow map overrides to apply later.
			return
		}
		p.FireMode = mode
		if strings.EqualFold(mode, "none") {
			// Explicitly clear weapon stats for units with no weapons.
			p.Range = 0
			p.Damage = 0
			p.Interval = 0
			p.BulletType = 0
			p.BulletSpeed = 0
			p.SplashRadius = 0
			p.SlowSec = 0
			p.SlowMul = 0
			p.Pierce = 0
			p.ChainCount = 0
			p.ChainRange = 0
			p.FragmentCount = 0
			p.FragmentSpread = 0
			p.FragmentSpeed = 0
			p.FragmentLife = 0
			p.BurstShots = 0
			p.BurstSpacing = 0
			p.Spread = 0
			p.PreferBuildings = false
			p.TargetAir = false
			p.TargetGround = false
			p.TargetPriority = "none"
			p.HitBuildings = false
			return
		}
	}
	if u.Range > 0 {
		p.Range = u.Range
	}
	if u.Damage > 0 {
		p.Damage = u.Damage
	}
	if u.Interval > 0 {
		p.Interval = u.Interval
	}
	p.BulletType = u.BulletType
	if u.BulletSpeed > 0 {
		p.BulletSpeed = u.BulletSpeed
	}
	p.SplashRadius = u.SplashRadius
	p.SlowSec = u.SlowSec
	if u.SlowMul > 0 {
		p.SlowMul = u.SlowMul
	}
	p.Pierce = u.Pierce
	p.ChainCount = u.ChainCount
	p.ChainRange = u.ChainRange
	p.FragmentCount = u.FragmentCount
	p.FragmentSpread = u.FragmentSpread
	p.FragmentSpeed = u.FragmentSpeed
	p.FragmentLife = u.FragmentLife
	if u.BurstShots > 0 {
		p.BurstShots = u.BurstShots
	}
	if u.BurstSpacing > 0 {
		p.BurstSpacing = u.BurstSpacing
	}
	if u.Spread > 0 {
		p.Spread = u.Spread
	}
	p.PreferBuildings = u.PreferBuildings
	p.TargetAir = u.TargetAir
	p.TargetGround = u.TargetGround
	if strings.TrimSpace(u.TargetPriority) != "" {
		p.TargetPriority = strings.TrimSpace(u.TargetPriority)
	}
	p.HitBuildings = u.HitBuildings
}

func mergeBuildingProfile(p *buildingWeaponProfile, t vanillaTurretProfile) {
	if p == nil {
		return
	}
	if strings.TrimSpace(t.FireMode) != "" {
		p.FireMode = strings.TrimSpace(t.FireMode)
	}
	if t.Range > 0 {
		p.Range = t.Range
	}
	if t.Damage > 0 {
		p.Damage = t.Damage
	}
	if t.Interval > 0 {
		p.Interval = t.Interval
	}
	p.BulletType = t.BulletType
	if t.BulletSpeed > 0 {
		p.BulletSpeed = t.BulletSpeed
	}
	p.SplashRadius = t.SplashRadius
	p.SlowSec = t.SlowSec
	if t.SlowMul > 0 {
		p.SlowMul = t.SlowMul
	}
	p.Pierce = t.Pierce
	p.ChainCount = t.ChainCount
	p.ChainRange = t.ChainRange
	p.HitBuildings = t.HitBuildings
	p.TargetBuilds = t.TargetBuilds
	p.TargetAir = t.TargetAir
	p.TargetGround = t.TargetGround
	if strings.TrimSpace(t.TargetPriority) != "" {
		p.TargetPriority = strings.TrimSpace(t.TargetPriority)
	}
	p.AmmoCapacity = t.AmmoCapacity
	p.AmmoRegen = t.AmmoRegen
	p.AmmoPerShot = t.AmmoPerShot
	p.PowerCapacity = t.PowerCapacity
	p.PowerRegen = t.PowerRegen
	p.PowerPerShot = t.PowerPerShot
	p.BurstShots = t.BurstShots
	p.BurstSpacing = t.BurstSpacing
}

func (w *World) Model() *WorldModel {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.model
}

func (w *World) AddEntity(typeID int16, x, y float32, team TeamID) (RawEntity, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, ErrOutOfBounds
	}
	ent := RawEntity{
		TypeID:      typeID,
		X:           x,
		Y:           y,
		Team:        team,
		Health:      100,
		MaxHealth:   100,
		Shield:      25,
		ShieldMax:   25,
		ShieldRegen: 4.5,
		Armor:       1.5,
		SlowMul:     1,
		RuntimeInit: true,
	}
	w.applyUnitTypeDef(&ent)
	w.applyWeaponProfile(&ent)
	return w.model.AddEntity(ent), nil
}

func (w *World) AddEntityWithID(typeID int16, id int32, x, y float32, team TeamID) (RawEntity, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, ErrOutOfBounds
	}
	ent := RawEntity{
		TypeID:      typeID,
		ID:          id,
		X:           x,
		Y:           y,
		Health:      100,
		MaxHealth:   100,
		Shield:      25,
		ShieldMax:   25,
		ShieldRegen: 4.5,
		Armor:       1.5,
		SlowMul:     1,
		RuntimeInit: true,
		Team:        team,
	}
	w.applyUnitTypeDef(&ent)
	w.applyWeaponProfile(&ent)
	return w.model.AddEntity(ent), nil
}

func (w *World) RemoveEntity(id int32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	ent, ok := w.model.RemoveEntity(id)
	if ok {
		delete(w.unitMountCDs, id)
		delete(w.unitTargets, id)
		delete(w.unitBurstStates, id)
		delete(w.unitMountBursts, id)
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:   EntityEventRemoved,
			Entity: ent,
		})
		if len(ent.Payload) > 0 {
			explode := false
			if w.rulesMgr != nil {
				if rules := w.rulesMgr.Get(); rules != nil {
					explode = rules.UnitPayloadsExplode
				}
			}
			if !explode {
				_, _ = w.dropPayloadAtLocked(ent.Payload, ent.X, ent.Y, ent.Team)
			}
		}
	}
	return ent, ok
}

func (w *World) GetEntity(id int32) (RawEntity, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID == id {
			return w.model.Entities[i], true
		}
	}
	return RawEntity{}, false
}

// ApplyBuildPlans applies simplified build/break operations from client plans.
// It updates server world state and returns changed tile positions.
func (w *World) ApplyBuildPlans(team TeamID, ops []BuildPlanOp) []int32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || len(ops) == 0 {
		return nil
	}
	rules := w.rulesMgr.Get()
	checkCore := rules == nil || rules.ProtectCores
	changed := make([]int32, 0, len(ops))
	seen := make(map[int32]struct{}, len(ops))

	addChanged := func(pos int32) {
		if _, ok := seen[pos]; ok {
			return
		}
		seen[pos] = struct{}{}
		changed = append(changed, pos)
	}

	for _, op := range ops {
		if !w.model.InBounds(int(op.X), int(op.Y)) {
			continue
		}
		if checkCore && !op.Breaking {
			if !w.canPlaceByCoreRulesLocked(team, int(op.X), int(op.Y)) {
				continue
			}
		}
		tile, err := w.model.TileAt(int(op.X), int(op.Y))
		if err != nil || tile == nil {
			continue
		}
		pos := int32(tile.Y*w.model.Width + tile.X)
		if op.Breaking {
			if tile.Build != nil || tile.Block != 0 {
				if pending, ok := w.pendingBuilds[pos]; ok && pending.Breaking {
					continue
				}
				maxHP := float32(1000)
				if tile.Build != nil {
					if tile.Build.MaxHealth > 1 {
						maxHP = tile.Build.MaxHealth
					} else if tile.Build.Health > 1 {
						maxHP = tile.Build.Health
					}
				}
				if tile.Build == nil {
					tile.Build = &Building{
						Block:     tile.Block,
						Team:      tile.Team,
						Rotation:  tile.Rotation,
						X:         tile.X,
						Y:         tile.Y,
						Health:    maxHP,
						MaxHealth: maxHP,
					}
				}
				req, buildTime, buildCost := w.buildDataForBlockLocked(int16(tile.Block), rules)
				refunded := make([]int32, len(req))
				buildSpeed := op.BuildSpeed
				if buildSpeed <= 0 {
					buildSpeed = 1
				}
				w.pendingBuilds[pos] = pendingBuildState{
					Team:       tile.Team,
					BlockID:    int16(tile.Block),
					Rotation:   tile.Rotation,
					Progress:   0,
					LastEmit:   0,
					Breaking:   true,
					MaxHealth:  maxHP,
					BuildTime:  buildTime,
					BuildCost:  buildCost,
					Req:        req,
					Refunded:   refunded,
					BuildSpeed: buildSpeed,
				}
				w.entityEvents = append(w.entityEvents, EntityEvent{
					Kind:     EntityEventBuildHealth,
					BuildPos: packTilePos(tile.X, tile.Y),
					BuildHP:  tile.Build.Health,
				})
				addChanged(pos)
			}
			if tile.Block == 0 && tile.Build == nil && rules != nil && rules.AllowEnvironmentDeconstruct {
				if tile.Overlay != 0 {
					tile.Overlay = 0
					addChanged(pos)
				}
			}
			continue
		}
		if op.BlockID <= 0 {
			continue
		}
		// Skip idempotent placements to avoid event spam and packet floods.
		if pending, ok := w.pendingBuilds[pos]; ok &&
			pending.BlockID == op.BlockID &&
			pending.Team == team &&
			pending.Rotation == op.Rotation {
			continue
		}
		if tile.Block == BlockID(op.BlockID) && tile.Team == team && tile.Rotation == op.Rotation && tile.Build != nil {
			continue
		}
		// Initialize placement in world state, but do not emit "placed" event yet.
		// Final placement event is emitted when pending progress reaches 100%.
		maxHP := estimateBuildMaxHealth(op.BlockID, w.model)
		req, buildTime, buildCost := w.buildDataForBlockLocked(op.BlockID, rules)
		spent := make([]int32, len(req))
		buildSpeed := op.BuildSpeed
		if buildSpeed <= 0 {
			buildSpeed = 1
		}
		tile.Block = BlockID(op.BlockID)
		tile.Team = team
		tile.Rotation = op.Rotation
		if tile.Build == nil {
			tile.Build = &Building{
				Block:     tile.Block,
				Team:      team,
				Rotation:  op.Rotation,
				X:         tile.X,
				Y:         tile.Y,
				Health:    1,
				MaxHealth: maxHP,
			}
		} else {
			tile.Build.Block = tile.Block
			tile.Build.Team = team
			tile.Build.Rotation = op.Rotation
			tile.Build.X = tile.X
			tile.Build.Y = tile.Y
			tile.Build.MaxHealth = maxHP
			if tile.Build.Health < 1 {
				tile.Build.Health = 1
			}
		}
		w.pendingBuilds[pos] = pendingBuildState{
			Team:       team,
			BlockID:    op.BlockID,
			Rotation:   op.Rotation,
			Progress:   0,
			LastEmit:   0,
			Breaking:   false,
			MaxHealth:  maxHP,
			BuildTime:  buildTime,
			BuildCost:  buildCost,
			Req:        req,
			Spent:      spent,
			BuildSpeed: buildSpeed,
		}
		addChanged(pos)
	}
	return changed
}

type coreTileInfo struct {
	X    int
	Y    int
	Team TeamID
}

func (w *World) canPlaceByCoreRulesLocked(team TeamID, tileX, tileY int) bool {
	if w.model == nil {
		return true
	}
	rules := w.rulesMgr.Get()
	if rules != nil && !rules.ProtectCores {
		return true
	}
	cores := w.collectCoreTilesLocked()
	if len(cores) == 0 {
		return true
	}
	px := float32(tileX*8 + 4)
	py := float32(tileY*8 + 4)
	if rules != nil && rules.PolygonCoreProtection {
		best := coreTileInfo{}
		bestDist2 := float32(math.MaxFloat32)
		for _, c := range cores {
			dx := float32(c.X*8+4) - px
			dy := float32(c.Y*8+4) - py
			d2 := dx*dx + dy*dy
			if d2 < bestDist2 {
				bestDist2 = d2
				best = c
			}
		}
		if bestDist2 < float32(math.MaxFloat32) && best.Team != 0 && best.Team != team {
			return false
		}
		return true
	}
	radius := float32(0)
	if rules != nil {
		radius = rules.EnemyCoreBuildRadius + rules.ExtraCoreBuildRadius
	}
	if radius <= 0 {
		return true
	}
	limit2 := radius * radius
	for _, c := range cores {
		if c.Team == 0 || c.Team == team {
			continue
		}
		dx := float32(c.X*8+4) - px
		dy := float32(c.Y*8+4) - py
		if dx*dx+dy*dy <= limit2 {
			return false
		}
	}
	return true
}

func (w *World) collectCoreTilesLocked() []coreTileInfo {
	if w.model == nil || w.blockNamesByID == nil {
		return nil
	}
	out := make([]coreTileInfo, 0, 8)
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Build == nil || t.Build.Health <= 0 || t.Block == 0 {
			continue
		}
		name, ok := w.blockNamesByID[int16(t.Block)]
		if !ok {
			continue
		}
		if !strings.Contains(strings.ToLower(strings.TrimSpace(name)), "core") {
			continue
		}
		out = append(out, coreTileInfo{X: t.X, Y: t.Y, Team: t.Build.Team})
	}
	return out
}

// RemovePendingBuild removes one queued/pending build operation at tile x,y.
// It only cancels pending progress and does not force-remove existing blocks.
func (w *World) RemovePendingBuild(x, y int32, breaking bool) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || !w.model.InBounds(int(x), int(y)) {
		return false
	}
	pos := int32(int(y)*w.model.Width + int(x))
	st, ok := w.pendingBuilds[pos]
	if !ok || st.Breaking != breaking {
		return false
	}
	delete(w.pendingBuilds, pos)
	return true
}

// PendingBuildCount returns the number of active pending build operations.
func (w *World) PendingBuildCount() int {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return len(w.pendingBuilds)
}

func unpackPos(pos int32) (int, int) {
	x := int((pos >> 16) & 0xFFFF)
	y := int(pos & 0xFFFF)
	return x, y
}

func (w *World) tileForPosLocked(pos int32) (*Tile, bool) {
	if w.model == nil {
		return nil, false
	}
	x, y := unpackPos(pos)
	if w.model.InBounds(x, y) {
		t, err := w.model.TileAt(x, y)
		return t, err == nil && t != nil
	}
	linear := int(pos)
	if linear >= 0 && linear < w.model.Width*w.model.Height {
		x = linear % w.model.Width
		y = linear / w.model.Width
		t, err := w.model.TileAt(x, y)
		return t, err == nil && t != nil
	}
	return nil, false
}

func (w *World) ensureBuildLocked(t *Tile) *Building {
	if t == nil || t.Block == 0 {
		return nil
	}
	if t.Build == nil {
		maxHP := estimateBuildMaxHealth(int16(t.Block), w.model)
		t.Build = &Building{
			Block:     t.Block,
			Team:      t.Team,
			Rotation:  t.Rotation,
			X:         t.X,
			Y:         t.Y,
			Health:    maxHP,
			MaxHealth: maxHP,
			ConvMin:   1,
		}
	}
	return t.Build
}

func (w *World) SetBuildingItem(buildPos int32, itemID int16, amount int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok {
		return false
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return false
	}
	it := ItemID(itemID)
	for i := range b.Items {
		if b.Items[i].Item == it {
			if amount <= 0 {
				b.Items = append(b.Items[:i], b.Items[i+1:]...)
			} else {
				b.Items[i].Amount = amount
			}
			return true
		}
	}
	if amount > 0 {
		b.Items = append(b.Items, ItemStack{Item: it, Amount: amount})
	}
	return true
}

func (w *World) SetBuildingItems(buildPos int32, items []ItemStack) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok {
		return false
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return false
	}
	if len(items) == 0 {
		b.Items = nil
		return true
	}
	out := make([]ItemStack, 0, len(items))
	for _, s := range items {
		if s.Amount > 0 {
			out = append(out, s)
		}
	}
	b.Items = out
	return true
}

func (w *World) SetTileItems(positions []int32, itemID int16, amount int32) int {
	if len(positions) == 0 {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	updated := 0
	for _, pos := range positions {
		t, ok := w.tileForPosLocked(pos)
		if !ok {
			continue
		}
		b := w.ensureBuildLocked(t)
		if b == nil {
			continue
		}
		it := ItemID(itemID)
		found := false
		for i := range b.Items {
			if b.Items[i].Item != it {
				continue
			}
			found = true
			if amount <= 0 {
				b.Items = append(b.Items[:i], b.Items[i+1:]...)
			} else {
				b.Items[i].Amount = amount
			}
			break
		}
		if !found && amount > 0 {
			b.Items = append(b.Items, ItemStack{Item: it, Amount: amount})
		}
		updated++
	}
	return updated
}

func (w *World) AddBuildingItem(buildPos int32, itemID int16, amount int32) bool {
	if amount <= 0 {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.acceptBuildingItemLocked(buildPos, itemID, amount) > 0
}

func (w *World) RemoveBuildingItem(buildPos int32, itemID int16, amount int32) int32 {
	if amount <= 0 {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.removeBuildingItemLocked(buildPos, itemID, amount)
}

func (w *World) removeBuildingItemLocked(buildPos int32, itemID int16, amount int32) int32 {
	t, ok := w.tileForPosLocked(buildPos)
	if !ok || t.Build == nil {
		return 0
	}
	name := w.blockNameForTileLocked(t)
	if isStackConveyorName(name) {
		return w.stackConveyorRemoveLocked(t, itemID, amount)
	}
	if isConveyorName(name) && !isBridgeItemName(name) {
		return w.conveyorRemoveItemLocked(t, itemID, amount)
	}
	if isOverflowDuctName(name) || isUnderflowDuctName(name) {
		return w.overflowDuctRemoveLocked(t, itemID, amount)
	}
	if isDuctName(name) {
		return w.ductRemoveLocked(t, itemID, amount)
	}
	for i := range t.Build.Items {
		if t.Build.Items[i].Item != ItemID(itemID) {
			continue
		}
		cur := t.Build.Items[i].Amount
		if cur <= 0 {
			return 0
		}
		take := amount
		if cur < take {
			take = cur
		}
		t.Build.Items[i].Amount -= take
		if t.Build.Items[i].Amount <= 0 {
			t.Build.Items = append(t.Build.Items[:i], t.Build.Items[i+1:]...)
		}
		return take
	}
	return 0
}

func (w *World) ClearBuildingItems(buildPos int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok || t.Build == nil {
		return false
	}
	t.Build.Items = nil
	return true
}

func (w *World) SetBuildingLiquid(buildPos int32, liquidID int16, amount float32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok {
		return false
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return false
	}
	liq := LiquidID(liquidID)
	for i := range b.Liquids {
		if b.Liquids[i].Liquid == liq {
			if amount <= 0 {
				b.Liquids = append(b.Liquids[:i], b.Liquids[i+1:]...)
			} else {
				b.Liquids[i].Amount = amount
			}
			return true
		}
	}
	if amount > 0 {
		b.Liquids = append(b.Liquids, LiquidStack{Liquid: liq, Amount: amount})
	}
	return true
}

func (w *World) SetBuildingLiquids(buildPos int32, liquids []LiquidStack) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok {
		return false
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return false
	}
	if len(liquids) == 0 {
		b.Liquids = nil
		return true
	}
	out := make([]LiquidStack, 0, len(liquids))
	for _, s := range liquids {
		if s.Amount > 0 {
			out = append(out, s)
		}
	}
	b.Liquids = out
	return true
}

func (w *World) SetTileLiquids(positions []int32, liquidID int16, amount float32) int {
	if len(positions) == 0 {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	updated := 0
	for _, pos := range positions {
		t, ok := w.tileForPosLocked(pos)
		if !ok {
			continue
		}
		b := w.ensureBuildLocked(t)
		if b == nil {
			continue
		}
		liq := LiquidID(liquidID)
		found := false
		for i := range b.Liquids {
			if b.Liquids[i].Liquid != liq {
				continue
			}
			found = true
			if amount <= 0 {
				b.Liquids = append(b.Liquids[:i], b.Liquids[i+1:]...)
			} else {
				b.Liquids[i].Amount = amount
			}
			break
		}
		if !found && amount > 0 {
			b.Liquids = append(b.Liquids, LiquidStack{Liquid: liq, Amount: amount})
		}
		updated++
	}
	return updated
}

func (w *World) ClearBuildingLiquids(buildPos int32) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok || t.Build == nil {
		return false
	}
	t.Build.Liquids = nil
	return true
}

func (w *World) AddBuildingLiquid(buildPos int32, liquidID int16, amount float32) float32 {
	if amount <= 0 {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.acceptBuildingLiquidLocked(buildPos, liquidID, amount)
}

func (w *World) RemoveBuildingLiquid(buildPos int32, liquidID int16, amount float32) float32 {
	if amount <= 0 {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.removeBuildingLiquidLocked(buildPos, liquidID, amount)
}

// AcceptBuildingLiquid adds up to amount liquid into a building inventory and
// returns the actual accepted amount, clamped by known block capacity.
func (w *World) AcceptBuildingLiquid(buildPos int32, liquidID int16, amount float32) float32 {
	if amount <= 0 || liquidID <= 0 {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.acceptBuildingLiquidLocked(buildPos, liquidID, amount)
}

func (w *World) SetBuildingConfigRaw(buildPos int32, raw []byte) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok {
		return false
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return false
	}
	if len(raw) == 0 {
		b.Config = nil
		return true
	}
	b.Config = append([]byte(nil), raw...)
	return true
}

func (w *World) SetBuildingConfigValue(buildPos int32, value any) bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if buildPos < 0 {
		return false
	}
	if w.tileConfigValues == nil {
		w.tileConfigValues = map[int32]any{}
	}
	if value == nil {
		delete(w.tileConfigValues, buildPos)
		return true
	}
	w.tileConfigValues[buildPos] = value
	return true
}

func (w *World) BuildingConfigValue(buildPos int32) (any, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.buildingConfigValueLocked(buildPos)
}

func (w *World) buildingConfigValueLocked(buildPos int32) (any, bool) {
	if w.tileConfigValues == nil || buildPos < 0 {
		return nil, false
	}
	v, ok := w.tileConfigValues[buildPos]
	return v, ok
}

func (w *World) setBuildingConfigValueLocked(buildPos int32, value any) bool {
	if buildPos < 0 {
		return false
	}
	if w.tileConfigValues == nil {
		w.tileConfigValues = map[int32]any{}
	}
	if value == nil {
		delete(w.tileConfigValues, buildPos)
		return true
	}
	w.tileConfigValues[buildPos] = value
	return true
}

func (w *World) HasBuilding(buildPos int32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	t, ok := w.tileForPosLocked(buildPos)
	return ok && t != nil && t.Block != 0 && t.Build != nil
}

func (w *World) BuildingTeam(buildPos int32) (TeamID, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok || t == nil || t.Block == 0 || t.Build == nil {
		return 0, false
	}
	return t.Team, true
}

func (w *World) CanDepositToBuilding(buildPos int32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	t, ok := w.tileForPosLocked(buildPos)
	if !ok || t == nil || t.Block == 0 || t.Build == nil {
		return false
	}
	rules := w.rulesMgr.Get()
	if rules == nil || !rules.OnlyDepositCore {
		return true
	}
	name := ""
	if w.blockNamesByID != nil {
		name = strings.ToLower(strings.TrimSpace(w.blockNamesByID[int16(t.Block)]))
	}
	return strings.Contains(name, "core") || strings.Contains(name, "foundation") || strings.Contains(name, "nucleus")
}

// AcceptBuildingItem adds up to amount items into a building inventory and
// returns the actual accepted amount, clamped by known block capacity.
func (w *World) AcceptBuildingItem(buildPos int32, itemID int16, amount int32) int32 {
	if amount <= 0 || itemID <= 0 {
		return 0
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.acceptBuildingItemLocked(buildPos, itemID, amount)
}

func (w *World) acceptBuildingItemLocked(buildPos int32, itemID int16, amount int32) int32 {
	t, ok := w.tileForPosLocked(buildPos)
	if !ok {
		return 0
	}
	rules := w.rulesMgr.Get()
	coreIncinerate := (rules == nil || rules.CoreIncinerates) && w.isCoreTileLocked(t)
	name := w.blockNameForTileLocked(t)
	if isStackConveyorName(name) {
		return w.stackConveyorAcceptLocked(t, itemID, amount)
	}
	if isConveyorName(name) && !isBridgeItemName(name) {
		return w.conveyorAcceptStackLocked(t, itemID, amount)
	}
	if isOverflowDuctName(name) || isUnderflowDuctName(name) {
		return w.overflowDuctAcceptLocked(t, itemID, amount)
	}
	if isDuctName(name) {
		return w.ductAcceptLocked(t, itemID, amount)
	}
	accepted := w.acceptBuildingItemAmountLocked(buildPos, itemID, amount)
	if accepted <= 0 {
		if coreIncinerate {
			// Core incinerates excess; treat as accepted.
			return amount
		}
		return 0
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return 0
	}
	b.AddItem(ItemID(itemID), accepted)
	if coreIncinerate && accepted < amount {
		return amount
	}
	return accepted
}

func (w *World) acceptBuildingItemAmountLocked(buildPos int32, itemID int16, amount int32) int32 {
	t, ok := w.tileForPosLocked(buildPos)
	if !ok {
		return 0
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return 0
	}
	name := w.blockNameForTileLocked(t)
	if isStackConveyorName(name) {
		capacity := int32(0)
		if props, ok := w.blockPropsByName[name]; ok && props.ItemCapacity > 0 {
			capacity = props.ItemCapacity
		} else {
			capacity = 10
		}
		total := int32(0)
		for i := range b.Items {
			total += b.Items[i].Amount
		}
		space := capacity - total
		if space <= 0 {
			return 0
		}
		if amount > space {
			return space
		}
		return amount
	}
	if isConveyorName(name) && !isBridgeItemName(name) {
		cap := conveyorCapacity(name)
		if cap <= 0 {
			return 0
		}
		if b.ConvLen >= cap {
			return 0
		}
		space := cap - b.ConvLen
		if amount > int32(space) {
			return int32(space)
		}
		return amount
	}
	if isDuctUnloaderName(name) {
		return 0
	}
	if isJunctionName(name) {
		if w.junctionLaneCanAcceptLocked(t, 0) || w.junctionLaneCanAcceptLocked(t, 1) || w.junctionLaneCanAcceptLocked(t, 2) || w.junctionLaneCanAcceptLocked(t, 3) {
			if amount > 1 {
				return 1
			}
			return amount
		}
		return 0
	}
	if isDuctJunctionName(name) {
		if w.ductJunctionCanAcceptLocked(t, 0) || w.ductJunctionCanAcceptLocked(t, 1) || w.ductJunctionCanAcceptLocked(t, 2) || w.ductJunctionCanAcceptLocked(t, 3) {
			if amount > 1 {
				return 1
			}
			return amount
		}
		return 0
	}
	if isDuctRouterName(name) {
		if len(t.Build.Items) > 0 {
			return 0
		}
		if amount > 1 {
			return 1
		}
		return amount
	}
	if isStackRouterName(name) {
		capacity := int32(10)
		if props, ok := w.blockPropsByName[name]; ok && props.ItemCapacity > 0 {
			capacity = props.ItemCapacity
		}
		total := int32(0)
		for i := range b.Items {
			total += b.Items[i].Amount
		}
		space := capacity - total
		if space <= 0 {
			return 0
		}
		if amount > space {
			return space
		}
		return amount
	}
	if isOverflowDuctName(name) || isUnderflowDuctName(name) {
		if t.Build.OverflowCurrent != 0 || len(t.Build.Items) > 0 {
			return 0
		}
		if amount > 1 {
			return 1
		}
		return amount
	}
	if isDuctName(name) {
		if t.Build.DuctCurrent != 0 || len(t.Build.Items) > 0 {
			return 0
		}
		if amount > 1 {
			return 1
		}
		return amount
	}
	capacity := int32(0)
	if name != "" {
		if props, ok := w.blockPropsByName[name]; ok && props.ItemCapacity > 0 {
			capacity = props.ItemCapacity
		} else {
			capacity = buildingItemCapacityByName[name]
		}
		if capacity == 0 && isItemTransportBlockName(name) {
			capacity = transportDefaultCapacity(name)
		}
	}
	if capacity == 0 && w.isCoreTileLocked(t) {
		// Fallback to shard capacity when name lookup fails.
		capacity = 4000
	}
	accepted := amount
	if capacity > 0 {
		total := int32(0)
		for i := range b.Items {
			if b.Items[i].Amount > 0 {
				total += b.Items[i].Amount
			}
		}
		space := capacity - total
		if space <= 0 {
			return 0
		}
		if accepted > space {
			accepted = space
		}
	}
	if accepted <= 0 {
		return 0
	}
	return accepted
}

// RotateBuilding rotates an existing building by one 90deg step.
// direction=true rotates clockwise, false rotates counterclockwise.
func (w *World) RotateBuilding(buildPos int32, direction bool) (blockID int16, rotation int8, team TeamID, ok bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	t, exists := w.tileForPosLocked(buildPos)
	if !exists || t == nil || t.Block == 0 {
		return 0, 0, 0, false
	}
	b := w.ensureBuildLocked(t)
	if b == nil {
		return 0, 0, 0, false
	}
	step := -1
	if direction {
		step = 1
	}
	t.Rotation = int8((int(t.Rotation) + step + 4) % 4)
	b.Rotation = t.Rotation
	return int16(t.Block), t.Rotation, t.Team, true
}

func (w *World) SnapshotBuildingItems() map[int32][]ItemStack {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make(map[int32][]ItemStack)
	if w.model == nil {
		return out
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Build == nil || len(t.Build.Items) == 0 {
			continue
		}
		pos := packTilePos(t.X, t.Y)
		items := append([]ItemStack(nil), t.Build.Items...)
		out[pos] = items
	}
	return out
}

// SnapshotBuildingInventories returns all building positions and their current
// item stacks, including empty inventories. This is useful for late-join replay
// where explicit clear+set keeps client state deterministic.
func (w *World) SnapshotBuildingInventories() map[int32][]ItemStack {
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := make(map[int32][]ItemStack)
	if w.model == nil {
		return out
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Build == nil {
			continue
		}
		pos := packTilePos(t.X, t.Y)
		if len(t.Build.Items) == 0 {
			out[pos] = nil
			continue
		}
		out[pos] = append([]ItemStack(nil), t.Build.Items...)
	}
	return out
}

func (w *World) RefundToTeamCore(team TeamID, items []ItemStack) bool {
	if len(items) == 0 {
		return false
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return false
	}
	var core *Building
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t == nil || t.Block == 0 || t.Team != team {
			continue
		}
		name := ""
		if w.blockNamesByID != nil {
			name = w.blockNamesByID[int16(t.Block)]
		}
		if strings.HasPrefix(name, "core-") || t.Block == 78 || (t.Block >= 339 && t.Block <= 344) {
			core = w.ensureBuildLocked(t)
			if core != nil {
				break
			}
		}
	}
	if core == nil {
		return false
	}
	for _, it := range items {
		if it.Amount <= 0 {
			continue
		}
		core.AddItem(it.Item, it.Amount)
	}
	return true
}

func estimateBuildMaxHealth(blockID int16, model *WorldModel) float32 {
	name := ""
	if model != nil && model.BlockNames != nil {
		name = strings.ToLower(strings.TrimSpace(model.BlockNames[blockID]))
	}
	if name == "" {
		return 1000
	}
	if w := DefaultWorld(); w != nil && w.blockPropsByName != nil {
		if props, ok := w.blockPropsByName[name]; ok && props.Health > 0 {
			return props.Health
		}
	}
	if strings.Contains(name, "conveyor") || strings.Contains(name, "router") || strings.Contains(name, "junction") || strings.Contains(name, "bridge") {
		return 80
	}
	if strings.Contains(name, "duct") {
		return 110
	}
	if strings.Contains(name, "wall") {
		hp := float32(400)
		switch {
		case strings.Contains(name, "titanium"):
			hp = 520
		case strings.Contains(name, "thorium"):
			hp = 800
		case strings.Contains(name, "plastanium"):
			hp = 1000
		case strings.Contains(name, "phase"):
			hp = 1200
		case strings.Contains(name, "surge"):
			hp = 1500
		}
		if strings.Contains(name, "large") {
			hp *= 4
		}
		return hp
	}
	return 1000
}

func (w *World) SetEntityMotion(id int32, vx, vy, rotVel float32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		e.VelX = vx
		e.VelY = vy
		e.RotVel = rotVel
		w.model.EntitiesRev++
		return *e, true
	}
	return RawEntity{}, false
}

func (w *World) SetEntityPosition(id int32, x, y, rotation float32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		e.X = x
		e.Y = y
		e.Rotation = rotation
		w.model.EntitiesRev++
		return *e, true
	}
	return RawEntity{}, false
}

func (w *World) SetEntityLife(id int32, lifeSec float32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		if lifeSec <= 0 {
			e.LifeSec = 0
			e.AgeSec = 0
		} else {
			e.LifeSec = lifeSec
			if e.AgeSec > e.LifeSec {
				e.AgeSec = e.LifeSec
			}
		}
		w.model.EntitiesRev++
		return *e, true
	}
	return RawEntity{}, false
}

func (w *World) SetEntityFollow(id int32, targetID int32, speed float32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		e.Behavior = "follow"
		e.TargetID = targetID
		e.PatrolToB = false
		e.MoveSpeed = speed
		w.model.EntitiesRev++
		return *e, true
	}
	return RawEntity{}, false
}

func (w *World) SetEntityPatrol(id int32, ax, ay, bx, by, speed float32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		e.Behavior = "patrol"
		e.TargetID = 0
		e.PatrolAX = ax
		e.PatrolAY = ay
		e.PatrolBX = bx
		e.PatrolBY = by
		e.PatrolToB = true
		e.MoveSpeed = speed
		w.model.EntitiesRev++
		return *e, true
	}
	return RawEntity{}, false
}

func (w *World) ClearEntityBehavior(id int32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, false
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID != id {
			continue
		}
		e := &w.model.Entities[i]
		e.Behavior = ""
		e.TargetID = 0
		e.VelX = 0
		e.VelY = 0
		e.RotVel = 0
		e.MoveSpeed = 0
		w.model.EntitiesRev++
		return *e, true
	}
	return RawEntity{}, false
}

func (w *World) DrainEntityEvents() []EntityEvent {
	w.mu.Lock()
	defer w.mu.Unlock()
	if len(w.entityEvents) == 0 {
		return nil
	}
	out := make([]EntityEvent, len(w.entityEvents))
	copy(out, w.entityEvents)
	w.entityEvents = w.entityEvents[:0]
	return out
}

func (w *World) stepEntities(delta time.Duration) {
	if w.model == nil || len(w.model.Entities) == 0 {
		return
	}
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return
	}
	maxX := float32(w.model.Width * 8)
	maxY := float32(w.model.Height * 8)
	idToIndex := map[int32]int{}
	for i := range w.model.Entities {
		w.ensureEntityDefaults(&w.model.Entities[i])
		idToIndex[w.model.Entities[i].ID] = i
	}
	for i := 0; i < len(w.model.Entities); {
		e := &w.model.Entities[i]
		changed := false
		if e.SlowRemain > 0 {
			e.SlowRemain -= dt
			if e.SlowRemain <= 0 {
				e.SlowRemain = 0
				e.SlowMul = 1
			}
			changed = true
		}
		if e.Shield < e.ShieldMax && e.ShieldRegen > 0 {
			e.Shield += e.ShieldRegen * dt
			if e.Shield > e.ShieldMax {
				e.Shield = e.ShieldMax
			}
			changed = true
		}
		applyBehaviorMotion(e, w.model.Entities, idToIndex)
		if e.VelX != 0 || e.VelY != 0 {
			e.X += e.VelX * dt
			e.Y += e.VelY * dt
			changed = true
		}
		if e.RotVel != 0 {
			e.Rotation += e.RotVel * dt
			changed = true
		}
		if e.LifeSec > 0 {
			e.AgeSec += dt
			changed = true
		}
		if e.Health > 0 {
			tx := int(e.X / 8)
			ty := int(e.Y / 8)
			if w.model.InBounds(tx, ty) {
				tile := &w.model.Tiles[ty*w.model.Width+tx]
				if tile != nil && tile.Build != nil && tile.Build.Team != e.Team {
					name := w.blockNameForTileLocked(tile)
					if strings.Contains(name, "shock-mine") {
						if props, ok := w.blockPropsByName[name]; ok && props.Damage > 0 {
							w.applyDamageToEntity(e, props.Damage)
							if props.TileDamage > 0 {
								_ = w.applyDamageToBuilding(packTilePos(tx, ty), props.TileDamage)
							}
						}
						team := tile.Team
						tile.Build = nil
						tile.Block = 0
						w.entityEvents = append(w.entityEvents, EntityEvent{
							Kind:      EntityEventBuildDestroyed,
							BuildPos:  packTilePos(tx, ty),
							BuildTeam: team,
						})
						changed = true
					}
				}
			}
		}
		if changed {
			w.model.EntitiesRev++
		}

		out := e.X < 0 || e.Y < 0 || e.X > maxX || e.Y > maxY
		expired := e.LifeSec > 0 && e.AgeSec >= e.LifeSec
		dead := e.Health <= 0
		if !out && !expired && !dead {
			i++
			continue
		}
		removed := *e
		delete(w.unitMountCDs, removed.ID)
		delete(w.unitTargets, removed.ID)
		delete(w.unitBurstStates, removed.ID)
		delete(w.unitMountBursts, removed.ID)
		last := len(w.model.Entities) - 1
		w.model.Entities[i] = w.model.Entities[last]
		w.model.Entities = w.model.Entities[:last]
		w.model.EntitiesRev++
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:   EntityEventRemoved,
			Entity: removed,
		})
	}

	idToIndex = map[int32]int{}
	for i := range w.model.Entities {
		idToIndex[w.model.Entities[i].ID] = i
	}
	w.stepEntityCombat(dt, idToIndex)
	w.stepBuildingCombat(dt)
	w.stepBullets(dt, idToIndex)
}

func (w *World) stepEntityCombat(dt float32, idToIndex map[int32]int) {
	ents := w.model.Entities
	if len(ents) == 0 {
		return
	}
	for i := range ents {
		e := &ents[i]
		if e.Health <= 0 || e.AttackDamage <= 0 {
			continue
		}
		if mounts, ok := unitMountProfilesByType[e.TypeID]; ok && len(mounts) > 0 {
			w.stepEntityMountedCombat(e, mounts, dt, idToIndex)
			continue
		}
		slowMul := clampf(e.SlowMul, 0.2, 1)
		shots := e.AttackBurstShots
		if shots <= 0 {
			shots = 1
		}
		burstSpacing := e.AttackBurstSpacing
		burstSpread := e.AttackSpread
		state := w.unitBurstStates[e.ID]
		if state.BurstDelay > 0 {
			state.BurstDelay -= dt * slowMul
			if state.BurstDelay < 0 {
				state.BurstDelay = 0
			}
		}
		canFire := false
		useBurstDelay := shots > 1 && burstSpacing > 0
		if useBurstDelay {
			if state.BurstRemain > 0 {
				canFire = state.BurstDelay <= 0
			} else {
				if e.AttackCooldown > 0 {
					e.AttackCooldown -= dt * slowMul
					if e.AttackCooldown < 0 {
						e.AttackCooldown = 0
					}
				}
				canFire = e.AttackCooldown <= 0
			}
		} else {
			if e.AttackCooldown > 0 {
				e.AttackCooldown -= dt * slowMul
				if e.AttackCooldown < 0 {
					e.AttackCooldown = 0
				}
			}
			canFire = e.AttackCooldown <= 0
		}
		if !canFire {
			w.unitBurstStates[e.ID] = state
			continue
		}
		rangeLimit := e.AttackRange
		if rangeLimit <= 0 {
			rangeLimit = 56
		}
		track := w.unitTargets[e.ID]
		retargetDelay := maxf(e.AttackInterval*0.45, 0.18)
		fired := false
		firedOnUnit := false
		if tid, ok := w.acquireTrackedEntityTarget(*e, ents, idToIndex, rangeLimit, e.AttackTargetAir, e.AttackTargetGround, e.AttackTargetPriority, &track, dt, retargetDelay); ok {
			if idx, exists := idToIndex[tid]; exists && idx >= 0 && idx < len(ents) {
				target := &ents[idx]
				baseAngle := lookAt(e.X, e.Y, target.X, target.Y)
				e.Rotation = baseAngle
				if e.AttackFireMode == "beam" {
					w.applyDamageToEntity(target, e.AttackDamage)
					applySlow(target, e.AttackSlowSec, e.AttackSlowMul)
					w.applyBeamChain(*e, idx)
					e.AttackCooldown = maxf(e.AttackInterval, 0.2)
					state.BurstRemain = 0
					state.BurstDelay = 0
					state.BurstIndex = 0
				} else {
					if useBurstDelay {
						shotIndex := int32(0)
						if state.BurstRemain > 0 {
							shotIndex = state.BurstIndex
						}
						angle := burstAngle(baseAngle, shotIndex, shots, burstSpread)
						w.spawnBulletAngle(*e, angle, target.X, target.Y)
						if state.BurstRemain > 0 {
							state.BurstRemain--
							if state.BurstRemain > 0 {
								state.BurstDelay = burstSpacing
								state.BurstIndex++
							} else {
								e.AttackCooldown = maxf(e.AttackInterval, 0.2)
								state.BurstIndex = 0
							}
						} else {
							state.BurstRemain = shots - 1
							if state.BurstRemain > 0 {
								state.BurstDelay = burstSpacing
								state.BurstIndex = 1
							} else {
								e.AttackCooldown = maxf(e.AttackInterval, 0.2)
								state.BurstIndex = 0
							}
						}
					} else {
						total := shots
						for s := int32(0); s < total; s++ {
							angle := burstAngle(baseAngle, s, total, burstSpread)
							w.spawnBulletAngle(*e, angle, target.X, target.Y)
						}
						e.AttackCooldown = maxf(e.AttackInterval, 0.2)
						state.BurstRemain = 0
						state.BurstDelay = 0
						state.BurstIndex = 0
					}
				}
				fired = true
				firedOnUnit = true
			}
		}
		w.unitTargets[e.ID] = track
		if (!fired || e.AttackPreferBuildings) && e.AttackBuildings {
			if pos, tx, ty, ok := w.findNearestEnemyBuilding(*e, rangeLimit); ok {
				baseAngle := lookAt(e.X, e.Y, tx, ty)
				e.Rotation = baseAngle
				if e.AttackFireMode == "beam" {
					_ = w.applyDamageToBuilding(pos, e.AttackDamage)
					e.AttackCooldown = maxf(e.AttackInterval, 0.2)
					state.BurstRemain = 0
					state.BurstDelay = 0
					state.BurstIndex = 0
				} else {
					if useBurstDelay {
						shotIndex := int32(0)
						if state.BurstRemain > 0 {
							shotIndex = state.BurstIndex
						}
						angle := burstAngle(baseAngle, shotIndex, shots, burstSpread)
						w.spawnBulletAngle(*e, angle, tx, ty)
						if state.BurstRemain > 0 {
							state.BurstRemain--
							if state.BurstRemain > 0 {
								state.BurstDelay = burstSpacing
								state.BurstIndex++
							} else {
								e.AttackCooldown = maxf(e.AttackInterval, 0.2)
								state.BurstIndex = 0
							}
						} else {
							state.BurstRemain = shots - 1
							if state.BurstRemain > 0 {
								state.BurstDelay = burstSpacing
								state.BurstIndex = 1
							} else {
								e.AttackCooldown = maxf(e.AttackInterval, 0.2)
								state.BurstIndex = 0
							}
						}
					} else {
						total := shots
						for s := int32(0); s < total; s++ {
							angle := burstAngle(baseAngle, s, total, burstSpread)
							w.spawnBulletAngle(*e, angle, tx, ty)
						}
						e.AttackCooldown = maxf(e.AttackInterval, 0.2)
						state.BurstRemain = 0
						state.BurstDelay = 0
						state.BurstIndex = 0
					}
				}
				fired = true
			}
		}
		if !fired && state.BurstRemain > 0 {
			state.BurstRemain = 0
			state.BurstDelay = 0
			state.BurstIndex = 0
		}
		if firedOnUnit && !e.AttackPreferBuildings {
			w.unitBurstStates[e.ID] = state
			continue
		}
		w.unitBurstStates[e.ID] = state
	}
}

func (w *World) stepEntityMountedCombat(e *RawEntity, mounts []unitWeaponMountProfile, dt float32, idToIndex map[int32]int) {
	if e == nil || len(mounts) == 0 {
		return
	}
	cds := w.unitMountCDs[e.ID]
	if len(cds) != len(mounts) {
		cds = make([]float32, len(mounts))
	}
	bursts := w.unitMountBursts[e.ID]
	if len(bursts) != len(mounts) {
		bursts = make([]unitBurstState, len(mounts))
	}
	slowMul := clampf(e.SlowMul, 0.2, 1)
	for i := range cds {
		if cds[i] <= 0 {
			continue
		}
		cds[i] -= dt * slowMul
		if cds[i] < 0 {
			cds[i] = 0
		}
	}
	for i := range bursts {
		if bursts[i].BurstDelay <= 0 {
			continue
		}
		bursts[i].BurstDelay -= dt * slowMul
		if bursts[i].BurstDelay < 0 {
			bursts[i].BurstDelay = 0
		}
	}
	rangeLimit := e.AttackRange
	if rangeLimit <= 0 {
		rangeLimit = 56
	}
	shots := e.AttackBurstShots
	if shots <= 0 {
		shots = 1
	}
	burstSpacing := e.AttackBurstSpacing
	burstSpread := e.AttackSpread
	useBurstDelay := shots > 1 && burstSpacing > 0
	unitFired := false
	track := w.unitTargets[e.ID]
	retargetDelay := maxf(e.AttackInterval*0.45, 0.18)
	if tid, ok := w.acquireTrackedEntityTarget(*e, w.model.Entities, idToIndex, rangeLimit, e.AttackTargetAir, e.AttackTargetGround, e.AttackTargetPriority, &track, dt, retargetDelay); ok {
		if idx, exists := idToIndex[tid]; exists && idx >= 0 && idx < len(w.model.Entities) {
			target := &w.model.Entities[idx]
			for mi := range mounts {
				if useBurstDelay {
					if bursts[mi].BurstRemain == 0 && cds[mi] > 0 {
						continue
					}
					if bursts[mi].BurstRemain > 0 && bursts[mi].BurstDelay > 0 {
						continue
					}
				} else if cds[mi] > 0 {
					continue
				}
				if e.AttackFireMode == "beam" || shots <= 1 || !useBurstDelay {
					total := shots
					if total < 1 {
						total = 1
					}
					if e.AttackFireMode == "beam" {
						if w.fireEntityMountAtUnit(e, target, mounts[mi], idx, 0, 1, 0) {
							cds[mi] = maxf(e.AttackInterval*maxf(mounts[mi].CooldownMul, 0.15), 0.05)
							unitFired = true
						}
					} else {
						for s := int32(0); s < total; s++ {
							if w.fireEntityMountAtUnit(e, target, mounts[mi], idx, s, total, burstSpread) {
								unitFired = true
							}
						}
						cds[mi] = maxf(e.AttackInterval*maxf(mounts[mi].CooldownMul, 0.15), 0.05)
					}
					bursts[mi] = unitBurstState{}
					continue
				}
				shotIndex := int32(0)
				if bursts[mi].BurstRemain > 0 {
					shotIndex = bursts[mi].BurstIndex
				}
				if w.fireEntityMountAtUnit(e, target, mounts[mi], idx, shotIndex, shots, burstSpread) {
					unitFired = true
					if bursts[mi].BurstRemain > 0 {
						bursts[mi].BurstRemain--
						if bursts[mi].BurstRemain > 0 {
							bursts[mi].BurstDelay = burstSpacing
							bursts[mi].BurstIndex++
						} else {
							cds[mi] = maxf(e.AttackInterval*maxf(mounts[mi].CooldownMul, 0.15), 0.05)
							bursts[mi].BurstIndex = 0
						}
					} else {
						bursts[mi].BurstRemain = shots - 1
						if bursts[mi].BurstRemain > 0 {
							bursts[mi].BurstDelay = burstSpacing
							bursts[mi].BurstIndex = 1
						} else {
							cds[mi] = maxf(e.AttackInterval*maxf(mounts[mi].CooldownMul, 0.15), 0.05)
							bursts[mi].BurstIndex = 0
						}
					}
				}
			}
		}
	}
	if e.AttackBuildings && (!unitFired || e.AttackPreferBuildings) {
		if pos, tx, ty, ok := w.findNearestEnemyBuilding(*e, rangeLimit); ok {
			for mi := range mounts {
				if useBurstDelay {
					if bursts[mi].BurstRemain == 0 && cds[mi] > 0 {
						continue
					}
					if bursts[mi].BurstRemain > 0 && bursts[mi].BurstDelay > 0 {
						continue
					}
				} else if cds[mi] > 0 {
					continue
				}
				if e.AttackFireMode == "beam" || shots <= 1 || !useBurstDelay {
					total := shots
					if total < 1 {
						total = 1
					}
					if e.AttackFireMode == "beam" {
						if w.fireEntityMountAtBuilding(e, pos, tx, ty, mounts[mi], 0, 1, 0) {
							cds[mi] = maxf(e.AttackInterval*maxf(mounts[mi].CooldownMul, 0.15), 0.05)
						}
					} else {
						for s := int32(0); s < total; s++ {
							w.fireEntityMountAtBuilding(e, pos, tx, ty, mounts[mi], s, total, burstSpread)
						}
						cds[mi] = maxf(e.AttackInterval*maxf(mounts[mi].CooldownMul, 0.15), 0.05)
					}
					bursts[mi] = unitBurstState{}
					continue
				}
				shotIndex := int32(0)
				if bursts[mi].BurstRemain > 0 {
					shotIndex = bursts[mi].BurstIndex
				}
				if w.fireEntityMountAtBuilding(e, pos, tx, ty, mounts[mi], shotIndex, shots, burstSpread) {
					if bursts[mi].BurstRemain > 0 {
						bursts[mi].BurstRemain--
						if bursts[mi].BurstRemain > 0 {
							bursts[mi].BurstDelay = burstSpacing
							bursts[mi].BurstIndex++
						} else {
							cds[mi] = maxf(e.AttackInterval*maxf(mounts[mi].CooldownMul, 0.15), 0.05)
							bursts[mi].BurstIndex = 0
						}
					} else {
						bursts[mi].BurstRemain = shots - 1
						if bursts[mi].BurstRemain > 0 {
							bursts[mi].BurstDelay = burstSpacing
							bursts[mi].BurstIndex = 1
						} else {
							cds[mi] = maxf(e.AttackInterval*maxf(mounts[mi].CooldownMul, 0.15), 0.05)
							bursts[mi].BurstIndex = 0
						}
					}
				}
			}
		}
	}
	w.unitMountCDs[e.ID] = cds
	w.unitMountBursts[e.ID] = bursts
	w.unitTargets[e.ID] = track
}

func (w *World) fireEntityMountAtUnit(e *RawEntity, target *RawEntity, mount unitWeaponMountProfile, targetIdx int, shotIndex, totalShots int32, spread float32) bool {
	if e == nil || target == nil || target.Health <= 0 {
		return false
	}
	src := *e
	applyMountStats(&src, mount)
	baseAngle := lookAt(src.X, src.Y, target.X, target.Y)
	aimAngle := baseAngle + mount.AngleOffset
	if totalShots > 1 && spread > 0 {
		aimAngle = burstAngle(aimAngle, shotIndex, totalShots, spread)
	}
	src.Rotation = aimAngle
	if src.AttackFireMode == "beam" {
		scale := maxf(mount.DamageMul, 0.05)
		w.applyDamageToEntity(target, src.AttackDamage*scale)
		applySlow(target, src.AttackSlowSec*scale, src.AttackSlowMul)
		w.applyBeamChain(src, targetIdx)
		return true
	}
	w.spawnBulletAngle(src, aimAngle, target.X, target.Y)
	return true
}

func (w *World) fireEntityMountAtBuilding(e *RawEntity, pos int32, tx, ty float32, mount unitWeaponMountProfile, shotIndex, totalShots int32, spread float32) bool {
	if e == nil {
		return false
	}
	src := *e
	applyMountStats(&src, mount)
	baseAngle := lookAt(src.X, src.Y, tx, ty)
	aimAngle := baseAngle + mount.AngleOffset
	if totalShots > 1 && spread > 0 {
		aimAngle = burstAngle(aimAngle, shotIndex, totalShots, spread)
	}
	src.Rotation = aimAngle
	if src.AttackFireMode == "beam" {
		scale := maxf(mount.DamageMul, 0.05)
		_ = w.applyDamageToBuilding(pos, src.AttackDamage*scale)
		return true
	}
	w.spawnBulletAngle(src, aimAngle, tx, ty)
	return true
}

func applyMountStats(src *RawEntity, mount unitWeaponMountProfile) {
	if src == nil {
		return
	}
	if mount.DamageMul > 0 {
		src.AttackDamage *= mount.DamageMul
	}
	if mount.RangeMul > 0 {
		src.AttackRange *= mount.RangeMul
	}
	if mount.BulletSpeedMul > 0 {
		src.AttackBulletSpeed *= mount.BulletSpeedMul
	}
	if mount.SplashRadiusMul > 0 {
		src.AttackSplashRadius *= mount.SplashRadiusMul
	}
	if mount.BulletType >= 0 {
		src.AttackBulletType = mount.BulletType
	}
}

func (w *World) stepBuildingCombat(dt float32) {
	if w.model == nil {
		return
	}
	ents := w.model.Entities
	if len(ents) == 0 {
		return
	}
	idToIndex := make(map[int32]int, len(ents))
	for i := range ents {
		idToIndex[ents[i].ID] = i
	}
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		prof, ok := w.getBuildingWeaponProfile(int16(t.Build.Block))
		if !ok || prof.Damage <= 0 || prof.Interval <= 0 || prof.Range <= 0 {
			continue
		}
		name := w.blockNameForTileLocked(t)
		props := BlockProps{}
		var kind BlockKind
		var hasKind bool
		if name != "" && w.blockPropsByName != nil {
			props = w.blockPropsByName[name]
		}
		if name != "" {
			kind, hasKind = w.blockKindByName(name)
		}

		pos := int32(i)
		state, exists := w.buildStates[pos]
		if !exists {
			state = buildCombatState{
				Ammo:     prof.AmmoCapacity,
				Power:    prof.PowerCapacity,
				Reload:   prof.Interval,
				Rotation: float32(t.Rotation) * 90,
			}
		}
		state = w.regenBuildState(state, prof, dt)
		state.Reload += dt
		if state.Reload < prof.Interval && props.CoolantAmountPerS > 0 && props.CoolantMultiplier > 0 && w.liquidPropsByID != nil {
			if liqID, liqProps, amt := w.coolantForBuildingLocked(t); liqID > 0 && amt > 0 {
				need := props.CoolantAmountPerS * dt
				if need > 0 {
					used := minf(need, amt)
					if used > 0 {
						state.Reload += used * maxf(liqProps.HeatCapacity, 0.4) * props.CoolantMultiplier
						_ = w.removeBuildingLiquidLocked(packTilePos(t.X, t.Y), int16(liqID), used)
					}
				}
			}
		}
		if state.BurstDelay > 0 {
			state.BurstDelay -= dt
			if state.BurstDelay < 0 {
				state.BurstDelay = 0
			}
		}
		if state.RetargetCD > 0 {
			state.RetargetCD -= dt
			if state.RetargetCD < 0 {
				state.RetargetCD = 0
			}
		}

		src := RawEntity{
			X:                    float32(t.X*8 + 4),
			Y:                    float32(t.Y*8 + 4),
			Rotation:             float32(t.Rotation) * 90,
			Team:                 t.Build.Team,
			AttackFireMode:       prof.FireMode,
			AttackDamage:         prof.Damage,
			AttackInterval:       prof.Interval,
			AttackRange:          prof.Range,
			AttackBulletType:     prof.BulletType,
			AttackBulletSpeed:    prof.BulletSpeed,
			AttackSplashRadius:   prof.SplashRadius,
			AttackSlowSec:        prof.SlowSec,
			AttackSlowMul:        prof.SlowMul,
			AttackPierce:         prof.Pierce,
			AttackChainCount:     prof.ChainCount,
			AttackChainRange:     prof.ChainRange,
			AttackTargetAir:      prof.TargetAir,
			AttackTargetGround:   prof.TargetGround,
			AttackTargetPriority: prof.TargetPriority,
			AttackBuildings:      prof.HitBuildings,
			AttackInaccuracy:     props.Inaccuracy,
			AttackVelocityRnd:    props.VelocityRnd,
		}

		if hasKind && strings.Contains(strings.ToLower(kind.Class), "pointdefenseturret") {
			if w.stepPointDefenseTurret(t, &state, src, props, dt) {
				w.buildStates[pos] = state
			}
			continue
		}
		if hasKind && strings.Contains(strings.ToLower(kind.Class), "tractorbeamturret") {
			w.stepTractorBeamTurret(t, &state, src, props, dt, ents, idToIndex)
			w.buildStates[pos] = state
			continue
		}

		minRange := props.MinRange
		shootCone := props.ShootCone
		if shootCone <= 0 {
			shootCone = 8
		}
		desiredAngle := float32(0)
		hasTarget := false
		targetIsEntity := false
		var targetIdx int
		var targetPos int32
		var targetX, targetY float32
		track := targetTrackState{TargetID: state.TargetID, RetargetCD: state.RetargetCD}
		retargetDelay := maxf(prof.Interval*0.55, 0.22)
		if tid, ok := w.acquireTrackedEntityTarget(src, ents, idToIndex, prof.Range, prof.TargetAir, prof.TargetGround, prof.TargetPriority, &track, dt, retargetDelay); ok {
			if idx, exists := idToIndex[tid]; exists && idx >= 0 && idx < len(ents) {
				target := &ents[idx]
				ax, ay := target.X, target.Y
				if src.AttackFireMode != "beam" && src.AttackBulletSpeed > 0 {
					ax, ay = leadTarget(src.X, src.Y, target.X, target.Y, target.VelX, target.VelY, src.AttackBulletSpeed)
				}
				desiredAngle = lookAt(src.X, src.Y, ax, ay)
				targetIsEntity = true
				targetIdx = idx
				targetX, targetY = ax, ay
				hasTarget = true
			}
		} else if prof.TargetBuilds {
			if bpos, bx, by, ok := w.findNearestEnemyBuildingPriority(src, prof.Range, minRange, prof.TargetPriority); ok {
				desiredAngle = lookAt(src.X, src.Y, bx, by)
				targetPos = bpos
				targetX, targetY = bx, by
				hasTarget = true
			}
		}
		state.TargetID = track.TargetID
		state.RetargetCD = track.RetargetCD
		rotSpeed := props.RotateSpeed
		if rotSpeed <= 0 {
			rotSpeed = 5
		}
		if hasTarget {
			state.Rotation = approachAngle(state.Rotation, desiredAngle, rotSpeed*60*dt)
		} else {
			state.Rotation = approachAngle(state.Rotation, float32(t.Rotation)*90, rotSpeed*60*dt)
		}
		t.Rotation = int8((int(state.Rotation/90) + 4) % 4)
		allowShot := (state.BurstRemain == 0 || state.BurstDelay <= 0)
		burstShots := prof.BurstShots
		burstSpacing := prof.BurstSpacing
		burstSpread := float32(0)
		if burstShots <= 0 && props.ShootShots > 1 {
			burstShots = props.ShootShots
			burstSpacing = props.ShootShotDelaySec
			if burstSpacing <= 0 {
				burstSpacing = 0.02
			}
			burstSpread = props.ShootSpread
		}
		if hasTarget && allowShot && state.Reload >= prof.Interval && angleDiff(state.Rotation, desiredAngle) <= shootCone {
			if minRange > 0 {
				dx := targetX - src.X
				dy := targetY - src.Y
				if dx*dx+dy*dy < minRange*minRange {
					hasTarget = false
				}
			}
		}
		if hasTarget && allowShot && state.Reload >= prof.Interval && angleDiff(state.Rotation, desiredAngle) <= shootCone {
			src.Rotation = state.Rotation
			shotIndex := int32(0)
			totalShots := int32(1)
			if burstShots > 1 {
				shotIndex = state.BurstIndex
				totalShots = burstShots
			}
			angle := src.Rotation
			if totalShots > 1 && burstSpread > 0 {
				angle += (float32(shotIndex)/float32(totalShots-1) - 0.5) * burstSpread
			}
			if targetIsEntity {
				target := &ents[targetIdx]
				if src.AttackFireMode == "beam" {
					w.applyDamageToEntity(target, src.AttackDamage)
					applySlow(target, src.AttackSlowSec, src.AttackSlowMul)
					w.applyBeamChain(src, targetIdx)
				} else {
					w.spawnBulletAngle(src, angle, targetX, targetY)
				}
			} else if targetPos != 0 && prof.HitBuildings {
				if src.AttackFireMode == "beam" {
					_ = w.applyDamageToBuilding(targetPos, src.AttackDamage)
				} else {
					w.spawnBulletAngle(src, angle, targetX, targetY)
				}
			}
			if state.BurstRemain > 0 {
				state.BurstRemain--
				if burstSpacing <= 0 {
					burstSpacing = 0.02
				}
				state.BurstDelay = maxf(burstSpacing, 0.02)
				state.BurstIndex++
			} else {
				shots := burstShots
				if shots < 1 {
					shots = 1
				}
				state.BurstRemain = shots - 1
				if state.BurstRemain > 0 {
					if burstSpacing <= 0 {
						burstSpacing = 0.02
					}
					state.BurstDelay = maxf(burstSpacing, 0.02)
					state.BurstIndex = 1
				} else {
					state.BurstIndex = 0
				}
				state.Reload -= prof.Interval
				if state.Reload < 0 {
					state.Reload = 0
				}
			}
		}
		w.buildStates[pos] = state
	}
}

func (w *World) regenBuildState(state buildCombatState, prof buildingWeaponProfile, dt float32) buildCombatState {
	if prof.AmmoCapacity > 0 {
		if prof.AmmoRegen > 0 {
			state.Ammo = minf(prof.AmmoCapacity, state.Ammo+prof.AmmoRegen*dt)
		}
	}
	if prof.PowerCapacity > 0 {
		if prof.PowerRegen > 0 {
			state.Power = minf(prof.PowerCapacity, state.Power+prof.PowerRegen*dt)
		}
	}
	return state
}

func (w *World) tryFireBuildingShot(src *RawEntity, state *buildCombatState, prof buildingWeaponProfile, ents []RawEntity, idToIndex map[int32]int) bool {
	if src == nil || state == nil {
		return false
	}
	if prof.AmmoPerShot > 0 && state.Ammo < prof.AmmoPerShot {
		return false
	}
	if prof.PowerPerShot > 0 && state.Power < prof.PowerPerShot {
		return false
	}

	fired := false
	track := targetTrackState{TargetID: state.TargetID, RetargetCD: state.RetargetCD}
	retargetDelay := maxf(prof.Interval*0.55, 0.22)
	if tid, ok := w.acquireTrackedEntityTarget(*src, ents, idToIndex, prof.Range, prof.TargetAir, prof.TargetGround, prof.TargetPriority, &track, 0, retargetDelay); ok {
		if idx, exists := idToIndex[tid]; exists && idx >= 0 && idx < len(ents) {
			target := &ents[idx]
			src.Rotation = lookAt(src.X, src.Y, target.X, target.Y)
			if src.AttackFireMode == "beam" {
				w.applyDamageToEntity(target, src.AttackDamage)
				applySlow(target, src.AttackSlowSec, src.AttackSlowMul)
				w.applyBeamChain(*src, idx)
			} else {
				w.spawnBullet(*src, target.X, target.Y)
			}
			fired = true
		}
	}
	state.TargetID = track.TargetID
	state.RetargetCD = track.RetargetCD
	if !fired && prof.TargetBuilds {
		if bpos, tx, ty, ok := w.findNearestEnemyBuilding(*src, prof.Range); ok {
			src.Rotation = lookAt(src.X, src.Y, tx, ty)
			if src.AttackFireMode == "beam" {
				_ = w.applyDamageToBuilding(bpos, src.AttackDamage)
			} else {
				w.spawnBullet(*src, tx, ty)
			}
			fired = true
		}
	}
	if !fired {
		return false
	}
	if prof.AmmoPerShot > 0 {
		state.Ammo -= prof.AmmoPerShot
		if state.Ammo < 0 {
			state.Ammo = 0
		}
	}
	if prof.PowerPerShot > 0 {
		state.Power -= prof.PowerPerShot
		if state.Power < 0 {
			state.Power = 0
		}
	}
	return true
}

func (w *World) stepPointDefenseTurret(t *Tile, state *buildCombatState, src RawEntity, props BlockProps, dt float32) bool {
	if t == nil || state == nil || w.model == nil {
		return false
	}
	if state.BurstDelay > 0 {
		state.BurstDelay -= dt
		if state.BurstDelay < 0 {
			state.BurstDelay = 0
		}
	}
	rangeLimit := maxf(src.AttackRange, 0)
	if rangeLimit <= 0 {
		return true
	}
	idx := w.findNearestEnemyBullet(src.Team, src.X, src.Y, rangeLimit)
	if idx < 0 {
		return true
	}
	if state.Reload < src.AttackInterval {
		return true
	}
	rotSpeed := props.RotateSpeed
	if rotSpeed <= 0 {
		rotSpeed = 8
	}
	b := w.bullets[idx]
	desired := lookAt(src.X, src.Y, b.X, b.Y)
	state.Rotation = approachAngle(state.Rotation, desired, rotSpeed*60*dt)
	t.Rotation = int8((int(state.Rotation/90) + 4) % 4)
	if angleDiff(state.Rotation, desired) > maxf(props.ShootCone, 6) {
		return true
	}
	// remove bullet
	last := len(w.bullets) - 1
	w.bullets[idx] = w.bullets[last]
	w.bullets = w.bullets[:last]
	state.Reload -= src.AttackInterval
	if state.Reload < 0 {
		state.Reload = 0
	}
	return true
}

func (w *World) findNearestEnemyBullet(team TeamID, x, y, rangeLimit float32) int {
	if rangeLimit <= 0 || len(w.bullets) == 0 {
		return -1
	}
	bestIdx := -1
	bestDist2 := rangeLimit * rangeLimit
	for i := range w.bullets {
		b := w.bullets[i]
		if b.Team == team {
			continue
		}
		dx := b.X - x
		dy := b.Y - y
		d2 := dx*dx + dy*dy
		if d2 > bestDist2 {
			continue
		}
		bestDist2 = d2
		bestIdx = i
	}
	return bestIdx
}

func (w *World) stepTractorBeamTurret(t *Tile, state *buildCombatState, src RawEntity, props BlockProps, dt float32, ents []RawEntity, idToIndex map[int32]int) {
	if t == nil || state == nil {
		return
	}
	rangeLimit := maxf(src.AttackRange, 0)
	if rangeLimit <= 0 {
		return
	}
	rotSpeed := props.RotateSpeed
	if rotSpeed <= 0 {
		rotSpeed = 10
	}
	tid, ok := findNearestEnemyEntity(src, ents, rangeLimit, src.AttackTargetAir, src.AttackTargetGround, src.AttackTargetPriority)
	if !ok {
		return
	}
	idx, exists := idToIndex[tid]
	if !exists || idx < 0 || idx >= len(ents) {
		return
	}
	target := &ents[idx]
	desired := lookAt(src.X, src.Y, target.X, target.Y)
	state.Rotation = approachAngle(state.Rotation, desired, rotSpeed*60*dt)
	t.Rotation = int8((int(state.Rotation/90) + 4) % 4)
	if angleDiff(state.Rotation, desired) > maxf(props.ShootCone, 6) {
		return
	}
	eff := float32(1)
	if props.CoolantAmountPerS > 0 && props.CoolantMultiplier > 0 {
		if liqID, liqProps, amt := w.coolantForBuildingLocked(t); liqID > 0 && amt > 0 {
			need := props.CoolantAmountPerS * dt
			if need > 0 {
				used := minf(need, amt)
				if used > 0 {
					_ = w.removeBuildingLiquidLocked(packTilePos(t.X, t.Y), int16(liqID), used)
					eff = 1 + used*maxf(liqProps.HeatCapacity, 0.4)*props.CoolantMultiplier
				}
			}
		}
	}
	if props.Damage > 0 {
		dmg := props.Damage * 60 * dt * eff
		w.applyDamageToEntity(target, dmg)
	}
	force := props.TractorForce
	if force <= 0 {
		force = 0.3
	}
	scaled := props.TractorForceScale
	dx := src.X - target.X
	dy := src.Y - target.Y
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist > 1e-3 {
		mul := force + (1-dist/rangeLimit)*scaled
		if mul < 0 {
			mul = 0
		}
		imp := mul * dt * 60 * eff
		target.VelX += dx / dist * imp
		target.VelY += dy / dist * imp
	}
}

func (w *World) spawnBullet(src RawEntity, tx, ty float32) {
	angle := lookAt(src.X, src.Y, tx, ty)
	w.spawnBulletAngle(src, angle, tx, ty)
}

func (w *World) spawnBulletAngle(src RawEntity, angle float32, tx, ty float32) {
	bulletSpeed := src.AttackBulletSpeed
	if bulletSpeed <= 0 {
		speed := src.MoveSpeed
		if speed <= 0 {
			speed = 18
		}
		bulletSpeed = maxf(speed*2.2, 28)
	}
	if src.AttackInaccuracy > 0 {
		angle += w.randRange(-src.AttackInaccuracy, src.AttackInaccuracy)
	}
	if src.AttackVelocityRnd > 0 {
		bulletSpeed *= 1 + w.randRange(-src.AttackVelocityRnd, src.AttackVelocityRnd)
		if bulletSpeed < 1 {
			bulletSpeed = 1
		}
	}
	rad := float32(angle * math.Pi / 180)
	b := simBullet{
		ID:              w.bulletNextID,
		Team:            src.Team,
		X:               src.X,
		Y:               src.Y,
		VX:              float32(math.Cos(float64(rad))) * bulletSpeed,
		VY:              float32(math.Sin(float64(rad))) * bulletSpeed,
		Damage:          src.AttackDamage,
		LifeSec:         maxf(src.AttackRange/bulletSpeed, 0.6),
		Radius:          5,
		HitUnits:        true,
		HitBuilds:       src.AttackBuildings,
		BulletType:      src.AttackBulletType,
		SplashRadius:    src.AttackSplashRadius,
		SlowSec:         src.AttackSlowSec,
		SlowMul:         clampf(src.AttackSlowMul, 0.2, 1),
		PierceRemain:    src.AttackPierce,
		ChainCount:      src.AttackChainCount,
		ChainRange:      src.AttackChainRange,
		FragmentCount:   src.AttackFragmentCount,
		FragmentSpread:  src.AttackFragmentSpread,
		FragmentSpeed:   src.AttackFragmentSpeed,
		FragmentLife:    src.AttackFragmentLife,
		TargetAir:       src.AttackTargetAir,
		TargetGround:    src.AttackTargetGround,
		TargetPriority:  src.AttackTargetPriority,
		PreferBuildings: src.AttackPreferBuildings,
	}
	w.bulletNextID++
	w.bullets = append(w.bullets, b)
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind: EntityEventBulletFired,
		Bullet: BulletEvent{
			Team:      b.Team,
			X:         b.X,
			Y:         b.Y,
			Angle:     angle,
			Damage:    b.Damage,
			BulletTyp: b.BulletType,
		},
	})
}

func (w *World) stepBullets(dt float32, idToIndex map[int32]int) {
	if len(w.bullets) == 0 {
		return
	}
	for i := 0; i < len(w.bullets); {
		b := &w.bullets[i]
		b.AgeSec += dt
		b.X += b.VX * dt
		b.Y += b.VY * dt
		hit := false
		if b.PreferBuildings && b.HitBuilds {
			hit = w.tryHitBuildingWithBullet(b)
		}
		if !hit && b.HitUnits {
			if tid, ok := findHitEnemyEntity(*b, w.model.Entities, b.Radius, b.TargetAir, b.TargetGround); ok {
				if idx, exists := idToIndex[tid]; exists && idx >= 0 && idx < len(w.model.Entities) {
					w.applyDamageToEntity(&w.model.Entities[idx], b.Damage)
					applySlow(&w.model.Entities[idx], b.SlowSec, b.SlowMul)
					w.applyChainDamage(*b, idx)
					w.applySplashDamage(*b)
					hit = true
					if b.PierceRemain > 0 {
						b.PierceRemain--
						hit = false
					}
				}
			}
		}
		if !hit && b.HitBuilds {
			hit = w.tryHitBuildingWithBullet(b)
		}
		expired := b.AgeSec >= b.LifeSec
		if !hit && !expired {
			i++
			continue
		}
		if (hit || expired) && b.FragmentCount > 0 {
			w.spawnBulletFragments(*b)
		}
		last := len(w.bullets) - 1
		w.bullets[i] = w.bullets[last]
		w.bullets = w.bullets[:last]
	}
}

func (w *World) spawnBulletFragments(parent simBullet) {
	n := parent.FragmentCount
	if n <= 0 {
		return
	}
	baseAngle := float32(math.Atan2(float64(parent.VY), float64(parent.VX)) * 180 / math.Pi)
	spread := parent.FragmentSpread
	if spread <= 0 {
		spread = 20
	}
	speed := parent.FragmentSpeed
	if speed <= 0 {
		speed = 28
	}
	life := parent.FragmentLife
	if life <= 0 {
		life = 0.6
	}
	for i := int32(0); i < n; i++ {
		t := float32(i)
		offset := float32(0)
		if n > 1 {
			offset = (t/float32(n-1))*spread - spread/2
		}
		ang := baseAngle + float32(offset)
		rad := float32(ang * math.Pi / 180)
		b := simBullet{
			ID:             w.bulletNextID,
			Team:           parent.Team,
			X:              parent.X,
			Y:              parent.Y,
			VX:             float32(math.Cos(float64(rad))) * speed,
			VY:             float32(math.Sin(float64(rad))) * speed,
			Damage:         parent.Damage * 0.45,
			LifeSec:        life,
			Radius:         4,
			HitUnits:       parent.HitUnits,
			HitBuilds:      parent.HitBuilds,
			BulletType:     parent.BulletType,
			SplashRadius:   parent.SplashRadius * 0.5,
			SlowSec:        parent.SlowSec * 0.5,
			SlowMul:        parent.SlowMul,
			PierceRemain:   0,
			ChainCount:     0,
			ChainRange:     0,
			TargetAir:      parent.TargetAir,
			TargetGround:   parent.TargetGround,
			TargetPriority: parent.TargetPriority,
		}
		w.bulletNextID++
		w.bullets = append(w.bullets, b)
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind: EntityEventBulletFired,
			Bullet: BulletEvent{
				Team:      b.Team,
				X:         b.X,
				Y:         b.Y,
				Angle:     ang,
				Damage:    b.Damage,
				BulletTyp: b.BulletType,
			},
		})
	}
}

func (w *World) tryHitBuildingWithBullet(b *simBullet) bool {
	if b == nil || w.model == nil {
		return false
	}
	x := int(b.X / 8)
	y := int(b.Y / 8)
	if w.model.InBounds(x, y) {
		t := &w.model.Tiles[y*w.model.Width+x]
		if t != nil && t.Build != nil && t.Build.Team != b.Team {
			if w.applyDamageToBuilding(packTilePos(x, y), b.Damage) {
				w.applySplashDamage(*b)
				if b.PierceRemain > 0 {
					b.PierceRemain--
					return false
				}
				return true
			}
		}
	}
	if pos, _, _, ok := w.findNearestEnemyBuilding(RawEntity{X: b.X, Y: b.Y, Team: b.Team}, b.Radius); ok {
		if w.applyDamageToBuilding(pos, b.Damage) {
			w.applySplashDamage(*b)
			if b.PierceRemain > 0 {
				b.PierceRemain--
				return false
			}
			return true
		}
	}
	return false
}

func (w *World) applySplashDamage(b simBullet) {
	if b.SplashRadius <= 0 {
		return
	}
	// Damage enemy units in splash radius.
	for i := range w.model.Entities {
		e := &w.model.Entities[i]
		if e.Health <= 0 || e.Team == b.Team {
			continue
		}
		dx := e.X - b.X
		dy := e.Y - b.Y
		d2 := dx*dx + dy*dy
		if d2 > b.SplashRadius*b.SplashRadius {
			continue
		}
		scale := 1 - float32(math.Sqrt(float64(d2)))/b.SplashRadius
		if scale < 0.15 {
			scale = 0.15
		}
		w.applyDamageToEntity(e, b.Damage*scale)
		applySlow(e, b.SlowSec*scale, b.SlowMul)
	}
	// Damage enemy buildings in splash radius.
	r := int(math.Ceil(float64(b.SplashRadius / 8)))
	cx := int(b.X / 8)
	cy := int(b.Y / 8)
	for ty := cy - r; ty <= cy+r; ty++ {
		for tx := cx - r; tx <= cx+r; tx++ {
			if !w.model.InBounds(tx, ty) {
				continue
			}
			t := &w.model.Tiles[ty*w.model.Width+tx]
			if t.Build == nil || t.Build.Health <= 0 || t.Build.Team == b.Team {
				continue
			}
			px := float32(tx*8 + 4)
			py := float32(ty*8 + 4)
			dx := px - b.X
			dy := py - b.Y
			d2 := dx*dx + dy*dy
			if d2 > b.SplashRadius*b.SplashRadius {
				continue
			}
			scale := 1 - float32(math.Sqrt(float64(d2)))/b.SplashRadius
			if scale < 0.15 {
				scale = 0.15
			}
			pos := int32(ty*w.model.Width + tx)
			_ = w.applyDamageToBuilding(pos, b.Damage*scale)
		}
	}
}

func (w *World) applyChainDamage(b simBullet, firstIdx int) {
	if b.ChainCount <= 0 || b.ChainRange <= 0 || firstIdx < 0 || firstIdx >= len(w.model.Entities) {
		return
	}
	hit := map[int]struct{}{firstIdx: {}}
	prev := firstIdx
	for c := int32(0); c < b.ChainCount; c++ {
		next := -1
		bestDist2 := b.ChainRange * b.ChainRange
		px := w.model.Entities[prev].X
		py := w.model.Entities[prev].Y
		for i := range w.model.Entities {
			if _, exists := hit[i]; exists {
				continue
			}
			e := &w.model.Entities[i]
			if e.Health <= 0 || e.Team == b.Team {
				continue
			}
			dx := e.X - px
			dy := e.Y - py
			d2 := dx*dx + dy*dy
			if d2 > bestDist2 {
				continue
			}
			bestDist2 = d2
			next = i
		}
		if next < 0 {
			return
		}
		scale := float32(math.Pow(0.72, float64(c+1)))
		damage := b.Damage * scale
		w.applyDamageToEntity(&w.model.Entities[next], damage)
		applySlow(&w.model.Entities[next], b.SlowSec*scale, b.SlowMul)
		hit[next] = struct{}{}
		prev = next
	}
}

func (w *World) applyBeamChain(src RawEntity, firstIdx int) {
	if src.AttackChainCount <= 0 || src.AttackChainRange <= 0 || firstIdx < 0 || firstIdx >= len(w.model.Entities) {
		return
	}
	hit := map[int]struct{}{firstIdx: {}}
	prev := firstIdx
	for c := int32(0); c < src.AttackChainCount; c++ {
		next := -1
		bestDist2 := src.AttackChainRange * src.AttackChainRange
		px := w.model.Entities[prev].X
		py := w.model.Entities[prev].Y
		for i := range w.model.Entities {
			if _, exists := hit[i]; exists {
				continue
			}
			e := &w.model.Entities[i]
			if e.Health <= 0 || e.Team == src.Team {
				continue
			}
			dx := e.X - px
			dy := e.Y - py
			d2 := dx*dx + dy*dy
			if d2 > bestDist2 {
				continue
			}
			bestDist2 = d2
			next = i
		}
		if next < 0 {
			return
		}
		scale := float32(math.Pow(0.72, float64(c+1)))
		dmg := src.AttackDamage * scale
		w.applyDamageToEntity(&w.model.Entities[next], dmg)
		applySlow(&w.model.Entities[next], src.AttackSlowSec*scale, src.AttackSlowMul)
		hit[next] = struct{}{}
		prev = next
	}
}

func (w *World) applyDamageToEntity(e *RawEntity, dmg float32) {
	if e == nil || dmg <= 0 {
		return
	}
	armor := e.Armor
	if armor > 0 {
		dmg -= armor
		if dmg < 0.5 {
			dmg = 0.5
		}
	}
	if e.Shield > 0 {
		absorb := minf(e.Shield, dmg)
		e.Shield -= absorb
		dmg -= absorb
	}
	if dmg > 0 {
		e.Health -= dmg
	}
}

func (w *World) getBuildingWeaponProfile(blockID int16) (buildingWeaponProfile, bool) {
	name, ok := w.blockNamesByID[blockID]
	if !ok {
		return buildingWeaponProfile{}, false
	}
	name = strings.ToLower(strings.TrimSpace(name))
	src := w.buildingProfilesByName
	if len(src) == 0 {
		src = buildingWeaponProfilesByName
	}
	p, ok := src[name]
	return p, ok
}

func (w *World) findNearestEnemyBuilding(src RawEntity, rangeLimit float32) (int32, float32, float32, bool) {
	if w.model == nil || src.Team == 0 {
		return 0, 0, 0, false
	}
	bestDist2 := rangeLimit * rangeLimit
	bestPos := int32(0)
	var bestX, bestY float32
	found := false
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		if t.Build.Team == src.Team {
			continue
		}
		tx := float32(t.X*8 + 4)
		ty := float32(t.Y*8 + 4)
		dx := tx - src.X
		dy := ty - src.Y
		d2 := dx*dx + dy*dy
		if d2 > bestDist2 {
			continue
		}
		bestDist2 = d2
		bestPos = int32(t.Y*w.model.Width + t.X)
		bestX = tx
		bestY = ty
		found = true
	}
	if !found {
		return 0, 0, 0, false
	}
	return bestPos, bestX, bestY, true
}

func (w *World) findNearestEnemyBuildingPriority(src RawEntity, rangeLimit float32, minRange float32, priority string) (int32, float32, float32, bool) {
	if w.model == nil || src.Team == 0 || rangeLimit <= 0 {
		return 0, 0, 0, false
	}
	bestScore := float32(math.MaxFloat32)
	bestPos := int32(0)
	var bestX, bestY float32
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		if t.Build.Team == src.Team {
			continue
		}
		tx := float32(t.X*8 + 4)
		ty := float32(t.Y*8 + 4)
		dx := tx - src.X
		dy := ty - src.Y
		d2 := dx*dx + dy*dy
		if d2 > rangeLimit*rangeLimit {
			continue
		}
		if minRange > 0 && d2 < minRange*minRange {
			continue
		}
		score := targetPriorityBuildingScore(t, d2, priority)
		if score < bestScore {
			bestScore = score
			bestPos = int32(t.Y*w.model.Width + t.X)
			bestX = tx
			bestY = ty
		}
	}
	if bestPos == 0 {
		return 0, 0, 0, false
	}
	return bestPos, bestX, bestY, true
}

func targetPriorityBuildingScore(t *Tile, d2 float32, priority string) float32 {
	if t == nil || t.Build == nil {
		return d2
	}
	dist := float32(math.Sqrt(float64(d2)))
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "lowest_health", "lowhp":
		return t.Build.Health + dist*0.25
	case "highest_health", "highhp", "tank":
		return -t.Build.Health + dist*0.35
	default:
		return d2
	}
}

func (w *World) applyShieldAbsorbLocked(x, y int, team TeamID, damage float32) float32 {
	if damage <= 0 || len(w.activeShields) == 0 {
		return damage
	}
	px := float32(x*8 + 4)
	py := float32(y*8 + 4)
	for _, sh := range w.activeShields {
		if sh.Team != team || sh.Radius <= 0 {
			continue
		}
		dx := px - sh.X
		dy := py - sh.Y
		if dx*dx+dy*dy > sh.Radius*sh.Radius {
			continue
		}
		if sh.Cap <= 0 {
			return 0
		}
		state := w.shieldStates[sh.Pos]
		if state.Shield <= 0 {
			continue
		}
		absorb := minf(state.Shield, damage)
		state.Shield -= absorb
		if state.Shield <= 0 {
			state.Shield = 0
			state.Broken = true
		}
		w.shieldStates[sh.Pos] = state
		damage -= absorb
		if damage <= 0 {
			return 0
		}
	}
	return damage
}

func (w *World) applyDamageToBuilding(pos int32, damage float32) bool {
	if w.model == nil || damage <= 0 {
		return false
	}
	rules := w.rulesMgr.Get()
	if rules != nil && !rules.BuildDamageEnabled {
		return false
	}
	if rules != nil && rules.BlockDamageMultiplier > 0 {
		damage *= rules.BlockDamageMultiplier
	}
	x := int(pos) % w.model.Width
	y := int(pos) / w.model.Width
	if !w.model.InBounds(x, y) {
		return false
	}
	t := &w.model.Tiles[y*w.model.Width+x]
	if t.Build == nil {
		return false
	}
	damage = w.applyShieldAbsorbLocked(x, y, t.Build.Team, damage)
	if damage <= 0 {
		return true
	}
	if name := w.blockNameForTileLocked(t); name != "" {
		if props, ok := w.blockPropsByName[name]; ok && props.Armor > 0 {
			damage -= props.Armor
			if damage < 0.5 {
				damage = 0.5
			}
		}
	}
	if damage <= 0 {
		return false
	}
	t.Build.Health -= damage
	if t.Build.Health > 0 {
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:     EntityEventBuildHealth,
			BuildPos: packTilePos(x, y),
			BuildHP:  t.Build.Health,
		})
		return true
	}
	team := t.Team
	t.Build = nil
	t.Block = 0
	delete(w.buildStates, pos)
	delete(w.tileConfigValues, packTilePos(x, y))
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:      EntityEventBuildDestroyed,
		BuildPos:  packTilePos(x, y),
		BuildTeam: team,
	})
	return true
}

func (w *World) acquireTrackedEntityTarget(
	src RawEntity,
	ents []RawEntity,
	idToIndex map[int32]int,
	rangeLimit float32,
	allowAir, allowGround bool,
	priority string,
	state *targetTrackState,
	dt float32,
	retargetDelay float32,
) (int32, bool) {
	if state == nil {
		return findNearestEnemyEntity(src, ents, rangeLimit, allowAir, allowGround, priority)
	}
	if state.RetargetCD > 0 {
		state.RetargetCD -= dt
		if state.RetargetCD < 0 {
			state.RetargetCD = 0
		}
	}
	if state.TargetID != 0 {
		if idx, ok := findEntityIndexByID(ents, idToIndex, state.TargetID); ok {
			if targetStillValid(src, ents[idx], rangeLimit, allowAir, allowGround) {
				return state.TargetID, true
			}
		}
		state.TargetID = 0
	}
	if state.RetargetCD > 0 {
		return 0, false
	}
	tid, ok := findNearestEnemyEntity(src, ents, rangeLimit, allowAir, allowGround, priority)
	if !ok {
		return 0, false
	}
	state.TargetID = tid
	state.RetargetCD = maxf(retargetDelay, 0.1)
	return tid, true
}

func findEntityIndexByID(ents []RawEntity, idToIndex map[int32]int, id int32) (int, bool) {
	if id == 0 {
		return -1, false
	}
	if idx, ok := idToIndex[id]; ok && idx >= 0 && idx < len(ents) && ents[idx].ID == id {
		return idx, true
	}
	for i := range ents {
		if ents[i].ID == id {
			return i, true
		}
	}
	return -1, false
}

func targetStillValid(src RawEntity, target RawEntity, rangeLimit float32, allowAir, allowGround bool) bool {
	if target.Health <= 0 || target.Team == src.Team {
		return false
	}
	if !canTargetEntity(target, allowAir, allowGround) {
		return false
	}
	if rangeLimit <= 0 {
		return false
	}
	hit := maxf(target.HitRadius, 0)
	limit := rangeLimit + hit/2
	dx := target.X - src.X
	dy := target.Y - src.Y
	return dx*dx+dy*dy <= limit*limit
}

func findNearestEnemyEntity(src RawEntity, ents []RawEntity, rangeLimit float32, allowAir, allowGround bool, priority string) (int32, bool) {
	if !allowAir && !allowGround {
		allowAir, allowGround = true, true
	}
	if rangeLimit <= 0 {
		return 0, false
	}
	bestID := int32(0)
	bestScore := float32(math.MaxFloat32)
	for i := range ents {
		e := ents[i]
		if e.ID == src.ID || e.Health <= 0 {
			continue
		}
		if e.Team == src.Team {
			continue
		}
		if !canTargetEntity(e, allowAir, allowGround) {
			continue
		}
		hit := maxf(e.HitRadius, 0)
		limit := rangeLimit + hit/2
		dx := e.X - src.X
		dy := e.Y - src.Y
		d2 := dx*dx + dy*dy
		if d2 > limit*limit {
			continue
		}
		d2Adj := d2 - hit*hit
		if d2Adj < 0 {
			d2Adj = 0
		}
		score := targetPriorityScore(src, e, d2Adj, priority)
		if score < bestScore {
			bestScore = score
			bestID = e.ID
		}
	}
	return bestID, bestID != 0
}

func findHitEnemyEntity(b simBullet, ents []RawEntity, radius float32, allowAir, allowGround bool) (int32, bool) {
	if !allowAir && !allowGround {
		allowAir, allowGround = true, true
	}
	bestDist2 := float32(math.MaxFloat32)
	bestID := int32(0)
	for i := range ents {
		e := ents[i]
		if e.Health <= 0 || e.Team == b.Team {
			continue
		}
		if !canTargetEntity(e, allowAir, allowGround) {
			continue
		}
		dx := e.X - b.X
		dy := e.Y - b.Y
		d2 := dx*dx + dy*dy
		hitR := radius + maxf(e.HitRadius, 1.0)
		if d2 > hitR*hitR {
			continue
		}
		if d2 >= bestDist2 {
			continue
		}
		bestDist2 = d2
		bestID = e.ID
	}
	return bestID, bestID != 0
}

func targetPriorityScore(src RawEntity, e RawEntity, d2 float32, priority string) float32 {
	dist := float32(math.Sqrt(float64(d2)))
	switch strings.ToLower(strings.TrimSpace(priority)) {
	case "lowest_health", "lowhp":
		return e.Health + dist*0.25
	case "highest_health", "highhp", "tank":
		return -e.Health + dist*0.35
	case "threat", "dps":
		threat := e.AttackDamage*1.8 + e.MaxHealth*0.15
		return -threat + dist*0.30
	default:
		return d2
	}
}

func canTargetEntity(e RawEntity, allowAir, allowGround bool) bool {
	flying := isEntityFlying(e)
	if flying {
		return allowAir
	}
	return allowGround
}

// Approximate flying type set for current compact type-id space.
func isEntityFlying(e RawEntity) bool {
	switch e.TypeID {
	case 5, 7, 9, 11, 13, 15:
		return true
	default:
		return false
	}
}

func entityHitRadiusForType(typeID int16) float32 {
	if r, ok := entityHitRadiusByType[typeID]; ok && r > 0 {
		return r
	}
	return 4.8
}

func (w *World) ensureEntityDefaults(e *RawEntity) {
	if e == nil || e.RuntimeInit {
		return
	}
	if e.Health <= 0 {
		e.Health = 100
	}
	if e.MaxHealth <= 0 {
		e.MaxHealth = 100
	}
	if e.AttackRange <= 0 {
		e.AttackRange = 56
	}
	if e.AttackDamage <= 0 {
		e.AttackDamage = 8
	}
	if e.AttackInterval <= 0 {
		e.AttackInterval = 0.7
	}
	if e.AttackBurstShots <= 0 {
		e.AttackBurstShots = 1
	}
	if e.AttackBurstSpacing < 0 {
		e.AttackBurstSpacing = 0
	}
	if e.AttackSpread < 0 {
		e.AttackSpread = 0
	}
	if e.AttackBulletSpeed <= 0 {
		e.AttackBulletSpeed = 34
	}
	if e.AttackSlowMul <= 0 {
		e.AttackSlowMul = 1
	}
	if e.SlowMul <= 0 {
		e.SlowMul = 1
	}
	if !e.AttackTargetAir && !e.AttackTargetGround {
		e.AttackTargetAir = true
		e.AttackTargetGround = true
	}
	if strings.TrimSpace(e.AttackTargetPriority) == "" {
		e.AttackTargetPriority = "nearest"
	}
	if e.HitRadius <= 0 {
		e.HitRadius = entityHitRadiusForType(e.TypeID)
	}
	if strings.TrimSpace(e.AttackFireMode) == "" {
		e.AttackFireMode = "projectile"
	}
	if e.ShieldMax <= 0 {
		e.ShieldMax = 25
	}
	if e.Shield <= 0 {
		e.Shield = e.ShieldMax
	}
	if e.ShieldRegen <= 0 {
		e.ShieldRegen = 4.5
	}
	if e.Armor < 0 {
		e.Armor = 0
	}
	w.applyWeaponProfile(e)
	e.RuntimeInit = true
}

func (w *World) applyWeaponProfile(e *RawEntity) {
	if e == nil {
		return
	}
	if w.applyWeaponFromUnitTypeDef(e) {
		return
	}
	p := defaultWeaponProfile
	if name, ok := w.unitNamesByID[e.TypeID]; ok && name != "" {
		if byName, exists := w.unitProfilesByName[name]; exists {
			p = byName
			e.AttackRange = p.Range
			e.AttackFireMode = p.FireMode
			e.AttackDamage = p.Damage
			e.AttackInterval = p.Interval
			e.AttackBulletType = p.BulletType
			e.AttackBulletSpeed = p.BulletSpeed
			e.AttackSplashRadius = p.SplashRadius
			e.AttackSlowSec = p.SlowSec
			e.AttackSlowMul = p.SlowMul
			e.AttackPierce = p.Pierce
			e.AttackChainCount = p.ChainCount
			e.AttackChainRange = p.ChainRange
			e.AttackFragmentCount = p.FragmentCount
			e.AttackFragmentSpread = p.FragmentSpread
			e.AttackFragmentSpeed = p.FragmentSpeed
			e.AttackFragmentLife = p.FragmentLife
			e.AttackBurstShots = p.BurstShots
			e.AttackBurstSpacing = p.BurstSpacing
			e.AttackSpread = p.Spread
			e.AttackPreferBuildings = p.PreferBuildings
			e.AttackTargetAir = p.TargetAir
			e.AttackTargetGround = p.TargetGround
			e.AttackTargetPriority = p.TargetPriority
			e.AttackBuildings = p.HitBuildings
			if e.HitRadius <= 0 {
				e.HitRadius = entityHitRadiusForType(e.TypeID)
			}
			if e.AttackBurstShots <= 0 {
				e.AttackBurstShots = 1
			}
			if e.AttackBurstSpacing < 0 {
				e.AttackBurstSpacing = 0
			}
			if e.AttackSpread < 0 {
				e.AttackSpread = 0
			}
			return
		}
	}
	src := w.unitProfilesByType
	if len(src) == 0 {
		src = weaponProfilesByType
	}
	if v, ok := src[e.TypeID]; ok {
		p = v
	}
	e.AttackRange = p.Range
	e.AttackFireMode = p.FireMode
	e.AttackDamage = p.Damage
	e.AttackInterval = p.Interval
	e.AttackBulletType = p.BulletType
	e.AttackBulletSpeed = p.BulletSpeed
	e.AttackSplashRadius = p.SplashRadius
	e.AttackSlowSec = p.SlowSec
	e.AttackSlowMul = p.SlowMul
	e.AttackPierce = p.Pierce
	e.AttackChainCount = p.ChainCount
	e.AttackChainRange = p.ChainRange
	e.AttackFragmentCount = p.FragmentCount
	e.AttackFragmentSpread = p.FragmentSpread
	e.AttackFragmentSpeed = p.FragmentSpeed
	e.AttackFragmentLife = p.FragmentLife
	e.AttackBurstShots = p.BurstShots
	e.AttackBurstSpacing = p.BurstSpacing
	e.AttackSpread = p.Spread
	e.AttackPreferBuildings = p.PreferBuildings
	e.AttackTargetAir = p.TargetAir
	e.AttackTargetGround = p.TargetGround
	e.AttackTargetPriority = p.TargetPriority
	e.AttackBuildings = p.HitBuildings
	if e.HitRadius <= 0 {
		e.HitRadius = entityHitRadiusForType(e.TypeID)
	}
	if e.AttackBurstShots <= 0 {
		e.AttackBurstShots = 1
	}
	if e.AttackBurstSpacing < 0 {
		e.AttackBurstSpacing = 0
	}
	if e.AttackSpread < 0 {
		e.AttackSpread = 0
	}
}

func (w *World) applyUnitTypeDef(e *RawEntity) {
	if e == nil || w.unitTypeDefsByID == nil {
		return
	}
	if def, ok := w.unitTypeDefsByID[e.TypeID]; ok {
		if def.Health > 0 {
			e.Health = def.Health
			e.MaxHealth = def.Health
		}
		if def.Armor > 0 {
			e.Armor = def.Armor
		}
		if def.HitSize > 0 {
			e.HitRadius = def.HitSize
		}
		if def.Speed > 0 {
			e.MoveSpeed = def.Speed
		}
	}
}

func (w *World) applyWeaponFromUnitTypeDef(e *RawEntity) bool {
	if e == nil || w.unitTypeDefsByID == nil {
		return false
	}
	def, ok := w.unitTypeDefsByID[e.TypeID]
	if !ok {
		return false
	}
	if def.Weapon.Damage <= 0 || def.Weapon.Interval <= 0 {
		return false
	}
	e.AttackRange = def.Weapon.Range
	e.AttackFireMode = def.Weapon.FireMode
	e.AttackDamage = def.Weapon.Damage
	e.AttackInterval = def.Weapon.Interval
	e.AttackBulletSpeed = def.Weapon.BulletSpeed
	e.AttackSplashRadius = def.Weapon.SplashRadius
	e.AttackPierce = def.Weapon.Pierce
	e.AttackBurstShots = def.Weapon.BurstShots
	e.AttackBurstSpacing = def.Weapon.BurstSpacing
	e.AttackSpread = def.Weapon.Spread
	e.AttackTargetAir = def.Weapon.TargetAir
	e.AttackTargetGround = def.Weapon.TargetGround
	e.AttackBuildings = def.Weapon.TargetGround
	if e.AttackBurstShots <= 0 {
		e.AttackBurstShots = 1
	}
	if e.AttackBurstSpacing < 0 {
		e.AttackBurstSpacing = 0
	}
	if e.AttackSpread < 0 {
		e.AttackSpread = 0
	}
	if strings.TrimSpace(e.AttackTargetPriority) == "" {
		e.AttackTargetPriority = "nearest"
	}
	return true
}

func maxf(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func clampf(v, minV, maxV float32) float32 {
	if v < minV {
		return minV
	}
	if v > maxV {
		return maxV
	}
	return v
}

func wrapAngle(a float32) float32 {
	for a <= -180 {
		a += 360
	}
	for a > 180 {
		a -= 360
	}
	return a
}

func approachAngle(cur, target, maxDelta float32) float32 {
	diff := wrapAngle(target - cur)
	if diff > maxDelta {
		diff = maxDelta
	} else if diff < -maxDelta {
		diff = -maxDelta
	}
	return wrapAngle(cur + diff)
}

func applySlow(e *RawEntity, sec, mul float32) {
	if e == nil || sec <= 0 {
		return
	}
	if mul <= 0 {
		mul = 1
	}
	e.SlowRemain = maxf(e.SlowRemain, sec)
	e.SlowMul = clampf(minf(e.SlowMul, mul), 0.2, 1)
}

func angleDiff(a, b float32) float32 {
	d := wrapAngle(a - b)
	if d < 0 {
		return -d
	}
	return d
}

func burstAngle(base float32, index, total int32, spread float32) float32 {
	if total <= 1 || spread <= 0 {
		return base
	}
	return base + (float32(index)/float32(total-1)-0.5)*spread
}

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
}

func absf(v float32) float32 {
	if v < 0 {
		return -v
	}
	return v
}

func leadTarget(sx, sy, tx, ty, tvx, tvy, speed float32) (float32, float32) {
	if speed <= 0 {
		return tx, ty
	}
	rx := tx - sx
	ry := ty - sy
	a := tvx*tvx + tvy*tvy - speed*speed
	b := 2 * (rx*tvx + ry*tvy)
	c := rx*rx + ry*ry
	if absf(a) < 1e-4 {
		if absf(b) < 1e-4 {
			return tx, ty
		}
		t := -c / b
		if t > 0 {
			return tx + tvx*t, ty + tvy*t
		}
		return tx, ty
	}
	disc := b*b - 4*a*c
	if disc < 0 {
		return tx, ty
	}
	sqrt := float32(math.Sqrt(float64(disc)))
	t1 := (-b - sqrt) / (2 * a)
	t2 := (-b + sqrt) / (2 * a)
	t := t1
	if t <= 0 || (t2 > 0 && t2 < t) {
		t = t2
	}
	if t <= 0 {
		return tx, ty
	}
	return tx + tvx*t, ty + tvy*t
}

func lookAt(x, y, tx, ty float32) float32 {
	return float32(math.Atan2(float64(ty-y), float64(tx-x)) * 180 / math.Pi)
}

func applyBehaviorMotion(e *RawEntity, ents []RawEntity, idToIndex map[int32]int) {
	speed := e.MoveSpeed
	if speed <= 0 {
		speed = 18
	}
	speed *= clampf(e.SlowMul, 0.2, 1)
	switch e.Behavior {
	case "follow":
		if e.TargetID == 0 {
			e.VelX, e.VelY = 0, 0
			return
		}
		idx, ok := idToIndex[e.TargetID]
		if !ok || idx < 0 || idx >= len(ents) {
			e.VelX, e.VelY = 0, 0
			return
		}
		tx := ents[idx].X
		ty := ents[idx].Y
		setVelocityToTarget(e, tx, ty, speed, 1.25)
	case "patrol":
		tx, ty := e.PatrolAX, e.PatrolAY
		if e.PatrolToB {
			tx, ty = e.PatrolBX, e.PatrolBY
		}
		if reachedTarget(e.X, e.Y, tx, ty, 1.25) {
			e.PatrolToB = !e.PatrolToB
			tx, ty = e.PatrolAX, e.PatrolAY
			if e.PatrolToB {
				tx, ty = e.PatrolBX, e.PatrolBY
			}
		}
		setVelocityToTarget(e, tx, ty, speed, 1.25)
	}
}

func setVelocityToTarget(e *RawEntity, tx, ty, speed, stopRadius float32) {
	dx := tx - e.X
	dy := ty - e.Y
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist <= stopRadius || dist == 0 {
		e.VelX, e.VelY = 0, 0
		return
	}
	e.VelX = speed * dx / dist
	e.VelY = speed * dy / dist
	e.Rotation = float32(math.Atan2(float64(dy), float64(dx)) * 180 / math.Pi)
}

func reachedTarget(x, y, tx, ty, radius float32) bool {
	dx := tx - x
	dy := ty - y
	return dx*dx+dy*dy <= radius*radius
}

// applyRulesToEntities 应用规则倍率到所有单位和建筑
func (w *World) applyRulesToEntities() {
	if w.model == nil {
		return
	}
	// 暂时不 aplicar倍率，因为 Unit/Building 结构与 Rules 方法不兼容
}

// GetRulesManager 返回规则管理器
func (w *World) GetRulesManager() *RulesManager {
	return w.rulesMgr
}

// GetWaveManager 返回波次管理器
func (w *World) GetWaveManager() *WaveManager {
	return w.wavesMgr
}

// triggerWave 触发波次生成
func (w *World) triggerWave(wm *WaveManager) {
	// Always advance wave counter when wave is triggered.
	nextWave := w.wave + 1
	w.wave = nextWave

	if w.model == nil {
		return
	}

	plan := wm.GeneratePlan(nextWave)
	if plan == nil {
		return
	}

	// 生成敌人（使用 RawEntity 结构）
	for group := 0; group < int(plan.GroupCount); group++ {
		for unitIdx := 0; unitIdx < int(plan.GroupSize); unitIdx++ {
			if len(w.model.Entities) >= 200 {
				break // 限制最大单位数量
			}

			enemyType := plan.EnemyTypePrior[0]
			if len(plan.EnemyTypePrior) > 0 {
				enemyType = plan.EnemyTypePrior[group%len(plan.EnemyTypePrior)]
			}

			// 在重生点生成敌人（简化实现）
			posX := float32(w.model.Width / 2)
			posY := float32(w.model.Height / 2)
			w.addEnemy(enemyType, posX, posY)
		}

	}
}

// addEnemy 添加敌方单位
func (w *World) addEnemy(unitType int16, x, y float32) {
	if w.model == nil {
		return
	}

	// 使用 RawEntity 结构创建敌人
	unit := RawEntity{
		TypeID:       unitType,
		ID:           int32(len(w.model.Entities) + 1),
		X:            x,
		Y:            y,
		Team:         2, // 敌人 team
		Health:       100,
		MaxHealth:    100,
		AttackDamage: 10,
		SlowMul:      1,
		Rotation:     0,
	}
	w.model.Entities = append(w.model.Entities, unit)
}
