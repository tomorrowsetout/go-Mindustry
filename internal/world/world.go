package world

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

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

type TeamCoreItemSnapshot struct {
	Team  TeamID
	Items []ItemStack
}

type Config struct {
	TPS int
}

type World struct {
	mu sync.RWMutex

	wave     int32
	waveTime float32
	tick     uint64

	rand0 int64
	rand1 int64

	tps       int8
	actualTps int8

	tpsWindowStart time.Time
	tpsWindowTicks int32

	start time.Time

	model *WorldModel

	// 规则和波次管理器
	rulesMgr *RulesManager
	wavesMgr *WaveManager

	entityEvents []EntityEvent
	bullets      []simBullet
	bulletNextID int32

	blockNamesByID      map[int16]string
	unitNamesByID       map[int16]string
	unitTypeDefsByID    map[int16]vanilla.UnitTypeDef
	buildStates         map[int32]buildCombatState
	pendingBuilds       map[int32]pendingBuildState
	pendingBreaks       map[int32]pendingBreakState
	factoryStates       map[int32]factoryState
	unitMountCDs        map[int32][]float32
	unitTargets         map[int32]targetTrackState
	teamItems           map[TeamID]map[ItemID]int32
	teamBuilderSpeed    map[TeamID]float32
	itemSourceCfg       map[int32]ItemID
	liquidSourceCfg     map[int32]LiquidID
	sorterCfg           map[int32]ItemID
	unloaderCfg         map[int32]ItemID
	payloadRouterCfg    map[int32]protocol.Content
	bridgeLinks         map[int32]int32
	massDriverLinks     map[int32]int32
	payloadDriverLinks  map[int32]int32
	bridgeBuffers       map[int32][]bufferedBridgeItem
	bridgeAcceptAcc     map[int32]float32
	conveyorStates      map[int32]*conveyorRuntimeState
	ductStates          map[int32]*ductRuntimeState
	routerStates        map[int32]*routerRuntimeState
	stackStates         map[int32]*stackRuntimeState
	massDriverStates    map[int32]*massDriverRuntimeState
	payloadStates       map[int32]*payloadRuntimeState
	payloadDriverStates map[int32]*payloadDriverRuntimeState
	massDriverShots     []massDriverShot
	payloadDriverShots  []payloadDriverShot
	blockDumpIndex      map[int32]int
	itemSourceAccum     map[int32]float32
	routerInputPos      map[int32]int32
	routerRotation      map[int32]byte
	transportAccum      map[int32]float32
	junctionQueues      map[int32]junctionQueueState
	reactorStates       map[int32]nuclearReactorState
	storageLinkedCore   map[int32]int32
	teamPrimaryCore     map[TeamID]int32
	coreStorageCapacity map[int32]int32
	blockOccupancy      map[int32]int32
	activeTilePositions []int32
	nextPlanOrder       uint64

	unitProfilesByType     map[int16]weaponProfile
	unitProfilesByName     map[string]weaponProfile
	buildingProfilesByName map[string]buildingWeaponProfile
	blockCostsByName       map[string][]ItemStack
	blockBuildTimesByName  map[string]float32
}

type BuildPlanOp struct {
	Breaking bool
	X        int32
	Y        int32
	Rotation int8
	BlockID  int16
	Config   any
}

type BuildSyncState struct {
	Pos      int32
	X        int32
	Y        int32
	BlockID  int16
	Team     TeamID
	Rotation int8
	Health   float32
}

type pendingBuildState struct {
	Owner        int32
	Team         TeamID
	BlockID      int16
	Rotation     int8
	Config       any
	QueueOrder   uint64
	Progress     float32
	VisualPlaced bool
	LastHP       float32
}

type pendingBreakState struct {
	Owner       int32
	Team        TeamID
	BlockID     int16
	Rotation    int8
	QueueOrder  uint64
	VisualStart bool
	Progress    float32
	MaxHealth   float32
	LastHP      float32
}

type factoryState struct {
	Progress float32
	UnitType int16
}

type nuclearReactorState struct {
	Heat         float32
	FuelProgress float32
}

type bufferedBridgeItem struct {
	Item      ItemID
	AgeFrames float32
}

type conveyorRuntimeState struct {
	IDs          [3]ItemID
	XS           [3]float32
	YS           [3]float32
	Len          int
	LastInserted int
	Mid          int
	MinItem      float32
}

type routerRuntimeState struct {
	LastItem  ItemID
	HasItem   bool
	LastInput int32
	Time      float32
}

type ductRuntimeState struct {
	Progress float32
	Current  ItemID
	HasItem  bool
	RecDir   byte
}

type stackRuntimeState struct {
	Link      int32
	Cooldown  float32
	LastItem  ItemID
	HasItem   bool
	Unloading bool
}

type massDriverRuntimeState struct {
	ReloadCounter float32
}

type massDriverShot struct {
	FromPos      int32
	ToPos        int32
	TravelFrames float32
	AgeFrames    float32
	Transferred  []ItemStack
}

type payloadKind byte

const (
	payloadKindUnit payloadKind = iota
	payloadKindBlock
)

type payloadData struct {
	Kind       payloadKind
	BlockID    int16
	UnitTypeID int16
	Serialized []byte
	Items      []ItemStack
	Liquids    []LiquidStack
	Power      float32
}

type payloadRuntimeState struct {
	Payload   *payloadData
	Move      float32
	Work      float32
	RecDir    byte
	Exporting bool
}

type payloadDriverRuntimeState struct {
	ReloadCounter float32
	Charge        float32
}

type payloadDriverShot struct {
	FromPos      int32
	ToPos        int32
	TravelFrames float32
	AgeFrames    float32
	Payload      *payloadData
}

type junctionQueuedItem struct {
	Item    ItemID
	FromDir byte
	AgeSec  float32
}

type junctionQueueState [4][]junctionQueuedItem

type protocolContentLiquid LiquidID

func (l protocolContentLiquid) ContentType() protocol.ContentType { return protocol.ContentLiquid }
func (l protocolContentLiquid) ID() int16                         { return int16(l) }
func (l protocolContentLiquid) Name() string                      { return "" }

type EntityEventKind string

const (
	EntityEventRemoved             EntityEventKind = "removed"
	EntityEventBuildPlaced         EntityEventKind = "build_placed"
	EntityEventBuildConstructed    EntityEventKind = "build_constructed"
	EntityEventBuildDeconstructing EntityEventKind = "build_deconstructing"
	EntityEventBuildDestroyed      EntityEventKind = "build_destroyed"
	EntityEventBuildHealth         EntityEventKind = "build_health"
	EntityEventTeamItems           EntityEventKind = "team_items"
	EntityEventBulletFired         EntityEventKind = "bullet_fired"
)

type EntityEvent struct {
	Kind   EntityEventKind
	Entity RawEntity
	// BuildPos is packed tile position (Point2), not linear tile index.
	BuildPos    int32
	BuildTeam   TeamID
	BuildBlock  int16
	BuildRot    int8
	BuildConfig any
	BuildHP     float32
	ItemID      ItemID
	ItemAmount  int32
	Bullet      BulletEvent
}

func packTilePos(x, y int) int32 {
	return (int32(x)&0xFFFF)<<16 | (int32(y) & 0xFFFF)
}

func unpackTilePos(pos int32) (int, int) {
	return int(uint16((pos >> 16) & 0xFFFF)), int(uint16(pos & 0xFFFF))
}

type BulletEvent struct {
	Team      TeamID
	X         float32
	Y         float32
	Angle     float32
	Damage    float32
	BulletTyp int16
}

type simBullet struct {
	ID             int32
	Team           TeamID
	X              float32
	Y              float32
	VX             float32
	VY             float32
	Damage         float32
	LifeSec        float32
	AgeSec         float32
	Radius         float32
	HitUnits       bool
	HitBuilds      bool
	BulletType     int16
	SplashRadius   float32
	SlowSec        float32
	SlowMul        float32
	PierceRemain   int32
	ChainCount     int32
	ChainRange     float32
	FragmentCount  int32
	FragmentSpread float32
	FragmentSpeed  float32
	FragmentLife   float32
	TargetAir      bool
	TargetGround   bool
	TargetPriority string
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
	TargetID    int32
	RetargetCD  float32
}

type targetTrackState struct {
	TargetID   int32
	RetargetCD float32
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
	Blocks      []vanillaBlockProfile  `json:"blocks"`
}

type vanillaBlockRequirement struct {
	Item   string  `json:"item"`
	ItemID int16   `json:"item_id"`
	Amount int32   `json:"amount"`
	Cost   float32 `json:"cost"`
}

type vanillaBlockProfile struct {
	Name                string                    `json:"name"`
	BuildCostMultiplier float32                   `json:"build_cost_multiplier"`
	BuildTimeSec        float32                   `json:"build_time_sec"`
	Requirements        []vanillaBlockRequirement `json:"requirements"`
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

func New(cfg Config) *World {
	tps := cfg.TPS
	if tps <= 0 {
		tps = 60
	}
	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	return &World{
		wave:                   1,
		waveTime:               0,
		tick:                   0,
		rand0:                  rng.Int63(),
		rand1:                  rng.Int63(),
		tps:                    int8(tps),
		actualTps:              int8(tps),
		tpsWindowStart:         time.Now(),
		start:                  time.Now(),
		bulletNextID:           1,
		buildStates:            map[int32]buildCombatState{},
		pendingBuilds:          map[int32]pendingBuildState{},
		pendingBreaks:          map[int32]pendingBreakState{},
		factoryStates:          map[int32]factoryState{},
		unitMountCDs:           map[int32][]float32{},
		unitTargets:            map[int32]targetTrackState{},
		teamItems:              map[TeamID]map[ItemID]int32{},
		teamBuilderSpeed:       map[TeamID]float32{1: 0.5},
		itemSourceCfg:          map[int32]ItemID{},
		liquidSourceCfg:        map[int32]LiquidID{},
		sorterCfg:              map[int32]ItemID{},
		unloaderCfg:            map[int32]ItemID{},
		payloadRouterCfg:       map[int32]protocol.Content{},
		bridgeLinks:            map[int32]int32{},
		massDriverLinks:        map[int32]int32{},
		payloadDriverLinks:     map[int32]int32{},
		bridgeBuffers:          map[int32][]bufferedBridgeItem{},
		bridgeAcceptAcc:        map[int32]float32{},
		conveyorStates:         map[int32]*conveyorRuntimeState{},
		ductStates:             map[int32]*ductRuntimeState{},
		routerStates:           map[int32]*routerRuntimeState{},
		stackStates:            map[int32]*stackRuntimeState{},
		massDriverStates:       map[int32]*massDriverRuntimeState{},
		payloadStates:          map[int32]*payloadRuntimeState{},
		payloadDriverStates:    map[int32]*payloadDriverRuntimeState{},
		massDriverShots:        []massDriverShot{},
		payloadDriverShots:     []payloadDriverShot{},
		blockDumpIndex:         map[int32]int{},
		itemSourceAccum:        map[int32]float32{},
		routerInputPos:         map[int32]int32{},
		routerRotation:         map[int32]byte{},
		transportAccum:         map[int32]float32{},
		junctionQueues:         map[int32]junctionQueueState{},
		reactorStates:          map[int32]nuclearReactorState{},
		storageLinkedCore:      map[int32]int32{},
		teamPrimaryCore:        map[TeamID]int32{},
		coreStorageCapacity:    map[int32]int32{},
		blockOccupancy:         map[int32]int32{},
		unitProfilesByType:     cloneUnitWeaponProfiles(weaponProfilesByType),
		unitProfilesByName:     map[string]weaponProfile{},
		buildingProfilesByName: cloneBuildingWeaponProfiles(buildingWeaponProfilesByName),
		blockCostsByName:       map[string][]ItemStack{},
		blockBuildTimesByName:  map[string]float32{},
		rulesMgr:               NewRulesManager(nil),
		wavesMgr:               NewWaveManager(nil),
	}
}

func (w *World) Step(delta time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.tick++
	now := time.Now()
	if w.tpsWindowStart.IsZero() {
		w.tpsWindowStart = now
	}
	w.tpsWindowTicks++
	if elapsed := now.Sub(w.tpsWindowStart); elapsed >= time.Second {
		measured := int(math.Round(float64(w.tpsWindowTicks) / elapsed.Seconds()))
		if measured <= 0 {
			measured = 1
		}
		if measured > int(w.tps) {
			measured = int(w.tps)
		}
		w.actualTps = int8(measured)
		w.tpsWindowStart = now
		w.tpsWindowTicks = 0
	}
	if dt := float32(delta.Seconds()); dt > 0 {
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
	}

	w.stepPendingBuilds(delta)
	w.stepPendingBreaks(delta)
	w.stepFactoryProduction(delta)
	w.stepSandboxSources(delta)
	w.stepLiquidLogistics(delta)
	w.stepItemLogistics(delta)
	w.stepPayloadLogistics(delta)
	w.stepNuclearReactors(delta)
	w.stepEntities(delta)
}

func (w *World) ConfigureItemSource(pos int32, item ItemID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	w.applyBuildingConfigLocked(pos, protocol.ItemRef{ItmID: int16(item)}, true)
}

func (w *World) ConfigureLiquidSource(pos int32, liquid LiquidID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	w.applyBuildingConfigLocked(pos, protocolContentLiquid(liquid), true)
}

func (w *World) ConfigureSorter(pos int32, item ItemID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	w.applyBuildingConfigLocked(pos, protocol.ItemRef{ItmID: int16(item)}, true)
}

func (w *World) ConfigureUnloader(pos int32, item ItemID) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	w.applyBuildingConfigLocked(pos, protocol.ItemRef{ItmID: int16(item)}, true)
}

func (w *World) ConfigureBuilding(pos int32, value any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	w.applyBuildingConfigLocked(pos, value, true)
}

func (w *World) ConfigureBuildingPacked(pos int32, value any) {
	w.mu.Lock()
	defer w.mu.Unlock()
	index, ok := w.tileIndexFromPackedPosLocked(pos)
	if !ok {
		return
	}
	w.applyBuildingConfigLocked(index, value, true)
}

func (w *World) configureItemContentLocked(pos int32, item ItemID) {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	switch w.blockNameByID(int16(w.model.Tiles[pos].Block)) {
	case "item-source":
		w.itemSourceCfg[pos] = item
		delete(w.liquidSourceCfg, pos)
	case "sorter", "inverted-sorter", "duct-router", "surge-router":
		w.sorterCfg[pos] = item
	case "unloader", "duct-unloader":
		w.unloaderCfg[pos] = item
	}
}

func (w *World) configurePayloadContentLocked(pos int32, content protocol.Content) bool {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) || content == nil {
		return false
	}
	switch w.blockNameByID(int16(w.model.Tiles[pos].Block)) {
	case "payload-router", "reinforced-payload-router":
		switch content.ContentType() {
		case protocol.ContentBlock, protocol.ContentUnit:
			w.payloadRouterCfg[pos] = content
			return true
		}
	}
	return false
}

func (w *World) configureLiquidContentLocked(pos int32, liquid LiquidID) {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	switch w.blockNameByID(int16(w.model.Tiles[pos].Block)) {
	case "liquid-source":
		w.liquidSourceCfg[pos] = liquid
		delete(w.itemSourceCfg, pos)
	}
}

func (w *World) itemConfigBlockAtLocked(pos int32) bool {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	switch w.blockNameByID(int16(w.model.Tiles[pos].Block)) {
	case "item-source", "sorter", "inverted-sorter", "duct-router", "surge-router", "unloader", "duct-unloader":
		return true
	default:
		return false
	}
}

func (w *World) ClearBuildingConfig(pos int32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.clearConfiguredStateLocked(pos)
}

func (w *World) BuildingConfigPacked(pos int32) (any, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	index, ok := w.tileIndexFromPackedPosLocked(pos)
	if !ok {
		return nil, false
	}
	if value, ok := w.normalizedBuildingConfigLocked(index); ok {
		return value, true
	}
	if w.model == nil || index < 0 || int(index) >= len(w.model.Tiles) {
		return nil, false
	}
	tile := &w.model.Tiles[index]
	if tile.Build == nil || len(tile.Build.Config) == 0 {
		return nil, false
	}
	return decodeStoredBuildingConfig(tile.Build.Config)
}

func (w *World) clearBuildingRuntimeLocked(pos int32) {
	w.clearConfiguredStateLocked(pos)
	delete(w.bridgeBuffers, pos)
	delete(w.bridgeAcceptAcc, pos)
	delete(w.conveyorStates, pos)
	delete(w.ductStates, pos)
	delete(w.routerStates, pos)
	delete(w.stackStates, pos)
	delete(w.massDriverStates, pos)
	delete(w.payloadStates, pos)
	delete(w.payloadDriverStates, pos)
	delete(w.blockDumpIndex, pos)
	delete(w.itemSourceAccum, pos)
	delete(w.routerInputPos, pos)
	delete(w.routerRotation, pos)
	delete(w.transportAccum, pos)
	delete(w.junctionQueues, pos)
	delete(w.reactorStates, pos)
}

func (w *World) clearConfiguredStateLocked(pos int32) {
	delete(w.itemSourceCfg, pos)
	delete(w.liquidSourceCfg, pos)
	delete(w.sorterCfg, pos)
	delete(w.unloaderCfg, pos)
	delete(w.payloadRouterCfg, pos)
	delete(w.bridgeLinks, pos)
	delete(w.massDriverLinks, pos)
	delete(w.payloadDriverLinks, pos)
	if w.model != nil && pos >= 0 && int(pos) < len(w.model.Tiles) {
		if tile := &w.model.Tiles[pos]; tile.Build != nil {
			tile.Build.Config = nil
		}
	}
}

func (w *World) tileIndexFromPackedPosLocked(pos int32) (int32, bool) {
	if w.model == nil {
		return 0, false
	}
	x := int(protocol.UnpackPoint2X(pos))
	y := int(protocol.UnpackPoint2Y(pos))
	if !w.model.InBounds(x, y) {
		return 0, false
	}
	return int32(y*w.model.Width + x), true
}

func (w *World) applyBuildingConfigLocked(pos int32, value any, persist bool) {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 {
		return
	}

	if value == nil {
		w.clearConfiguredStateLocked(pos)
		if persist {
			tile.Build.Config = nil
		}
		return
	}

	applied := false
	switch v := value.(type) {
	case protocol.Content:
		switch v.ContentType() {
		case protocol.ContentItem:
			w.configureItemContentLocked(pos, ItemID(v.ID()))
			applied = true
		case protocol.ContentLiquid:
			w.configureLiquidContentLocked(pos, LiquidID(v.ID()))
			applied = true
		case protocol.ContentBlock, protocol.ContentUnit:
			applied = w.configurePayloadContentLocked(pos, v)
		}
	case protocol.Point2:
		applied = w.configurePointConfigLocked(pos, v)
	case int32:
		if w.itemConfigBlockAtLocked(pos) {
			w.configureItemContentLocked(pos, ItemID(v))
			applied = true
		} else {
			applied = w.configureAbsoluteLinkLocked(pos, v)
		}
	case int:
		if w.itemConfigBlockAtLocked(pos) {
			w.configureItemContentLocked(pos, ItemID(v))
			applied = true
		} else {
			applied = w.configureAbsoluteLinkLocked(pos, int32(v))
		}
	case int16:
		if w.itemConfigBlockAtLocked(pos) {
			w.configureItemContentLocked(pos, ItemID(v))
			applied = true
		} else {
			applied = w.configureAbsoluteLinkLocked(pos, int32(v))
		}
	}

	if persist && applied {
		if normalized, ok := w.normalizedBuildingConfigLocked(pos); ok {
			w.storeBuildingConfigLocked(tile, normalized)
		} else {
			w.storeBuildingConfigLocked(tile, value)
		}
	}
}

func (w *World) configurePointConfigLocked(pos int32, p protocol.Point2) bool {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	switch w.blockNameByID(int16(tile.Block)) {
	case "bridge-conveyor", "phase-conveyor", "bridge-conduit", "phase-conduit", "mass-driver", "payload-mass-driver", "large-payload-mass-driver":
		targetX := tile.X + int(p.X)
		targetY := tile.Y + int(p.Y)
		if !w.model.InBounds(targetX, targetY) {
			return false
		}
		return w.configureAbsoluteLinkLocked(pos, int32(targetY*w.model.Width+targetX))
	default:
		return false
	}
}

func (w *World) configureAbsoluteLinkLocked(pos, target int32) bool {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	if target < 0 {
		if w.model != nil && pos >= 0 && int(pos) < len(w.model.Tiles) {
			switch w.blockNameByID(int16(w.model.Tiles[pos].Block)) {
			case "bridge-conveyor", "phase-conveyor":
				delete(w.bridgeLinks, pos)
			case "mass-driver":
				delete(w.massDriverLinks, pos)
			case "payload-mass-driver", "large-payload-mass-driver":
				delete(w.payloadDriverLinks, pos)
			}
		}
		return true
	}
	tile := &w.model.Tiles[pos]
	name := w.blockNameByID(int16(tile.Block))
	if name != "bridge-conveyor" && name != "phase-conveyor" && name != "bridge-conduit" && name != "phase-conduit" && name != "mass-driver" && name != "payload-mass-driver" && name != "large-payload-mass-driver" {
		return false
	}
	targetPos, ok := w.resolveAbsoluteLinkTargetLocked(target)
	if !ok {
		return false
	}
	targetTile := &w.model.Tiles[targetPos]
	dx := targetTile.X - tile.X
	dy := targetTile.Y - tile.Y
	if dx != 0 && dy != 0 {
		return false
	}
	if dx == 0 && dy == 0 {
		return false
	}
	rangeLimit := 4
	switch name {
	case "phase-conveyor":
		rangeLimit = 12
	case "mass-driver":
		rangeLimit = 55
	case "payload-mass-driver":
		rangeLimit = 87
	case "large-payload-mass-driver":
		rangeLimit = 262
	}
	if absInt(dx) > rangeLimit || absInt(dy) > rangeLimit {
		return false
	}
	switch name {
	case "bridge-conveyor", "phase-conveyor", "bridge-conduit", "phase-conduit":
		targetName := w.blockNameByID(int16(targetTile.Block))
		if targetName != name {
			return false
		}
		w.bridgeLinks[pos] = targetPos
	case "mass-driver":
		if w.blockNameByID(int16(targetTile.Block)) != "mass-driver" {
			return false
		}
		w.massDriverLinks[pos] = targetPos
	case "payload-mass-driver", "large-payload-mass-driver":
		if w.blockNameByID(int16(targetTile.Block)) != name {
			return false
		}
		w.payloadDriverLinks[pos] = targetPos
	}
	return true
}

func (w *World) resolveAbsoluteLinkTargetLocked(target int32) (int32, bool) {
	if w.model == nil {
		return 0, false
	}
	tx := protocol.UnpackPoint2X(target)
	ty := protocol.UnpackPoint2Y(target)
	if w.model.InBounds(int(tx), int(ty)) {
		return int32(int(ty)*w.model.Width + int(tx)), true
	}
	if target >= 0 && int(target) < len(w.model.Tiles) {
		return target, true
	}
	return 0, false
}

func (w *World) normalizedBuildingConfigLocked(pos int32) (any, bool) {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return nil, false
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 {
		return nil, false
	}
	switch w.blockNameByID(int16(tile.Block)) {
	case "item-source":
		item, ok := w.itemSourceCfg[pos]
		if !ok {
			return nil, false
		}
		return protocol.ItemRef{ItmID: int16(item)}, true
	case "liquid-source":
		liquid, ok := w.liquidSourceCfg[pos]
		if !ok {
			return nil, false
		}
		return protocolContentLiquid(liquid), true
	case "sorter", "inverted-sorter", "duct-router", "surge-router", "unloader", "duct-unloader":
		item, ok := w.sorterCfg[pos]
		switch w.blockNameByID(int16(tile.Block)) {
		case "unloader", "duct-unloader":
			item, ok = w.unloaderCfg[pos]
		}
		if !ok {
			return nil, false
		}
		return protocol.ItemRef{ItmID: int16(item)}, true
	case "payload-router", "reinforced-payload-router":
		filter, ok := w.payloadRouterCfg[pos]
		if !ok || filter == nil {
			return nil, false
		}
		return filter, true
	case "bridge-conveyor", "phase-conveyor", "bridge-conduit", "phase-conduit":
		target, ok := w.bridgeLinks[pos]
		if !ok || target < 0 || int(target) >= len(w.model.Tiles) {
			return nil, false
		}
		targetTile := &w.model.Tiles[target]
		return protocol.Point2{X: int32(targetTile.X - tile.X), Y: int32(targetTile.Y - tile.Y)}, true
	case "mass-driver":
		target, ok := w.massDriverLinks[pos]
		if !ok || target < 0 || int(target) >= len(w.model.Tiles) {
			return nil, false
		}
		targetTile := &w.model.Tiles[target]
		return protocol.Point2{X: int32(targetTile.X - tile.X), Y: int32(targetTile.Y - tile.Y)}, true
	case "payload-mass-driver", "large-payload-mass-driver":
		target, ok := w.payloadDriverLinks[pos]
		if !ok || target < 0 || int(target) >= len(w.model.Tiles) {
			return nil, false
		}
		targetTile := &w.model.Tiles[target]
		return protocol.Point2{X: int32(targetTile.X - tile.X), Y: int32(targetTile.Y - tile.Y)}, true
	default:
		return nil, false
	}
}

func (w *World) storeBuildingConfigLocked(tile *Tile, value any) {
	if tile == nil || tile.Build == nil {
		return
	}
	writer := protocol.NewWriter()
	normalized := value
	switch v := value.(type) {
	case int:
		normalized = int32(v)
	}
	if err := protocol.WriteObject(writer, normalized, nil); err != nil {
		return
	}
	tile.Build.Config = append(tile.Build.Config[:0], writer.Bytes()...)
}

func (w *World) restoreTileConfigsLocked() {
	if w.model == nil {
		return
	}
	for i := range w.model.Tiles {
		tile := &w.model.Tiles[i]
		if tile.Build == nil || len(tile.Build.Config) == 0 {
			continue
		}
		value, ok := decodeStoredBuildingConfig(tile.Build.Config)
		if !ok {
			continue
		}
		w.applyBuildingConfigLocked(int32(i), value, false)
	}
}

func decodeStoredBuildingConfig(data []byte) (any, bool) {
	if len(data) == 0 {
		return nil, false
	}
	value, err := protocol.ReadObject(protocol.NewReader(data), false, nil)
	if err != nil {
		return nil, false
	}
	return value, true
}

func (w *World) restorePayloadStatesLocked() {
	if w.model == nil {
		return
	}
	for i := range w.model.Tiles {
		tile := &w.model.Tiles[i]
		if tile.Build == nil || len(tile.Build.Payload) == 0 {
			continue
		}
		payload, ok := decodePayloadData(tile.Build.Payload)
		if !ok {
			continue
		}
		w.payloadStates[int32(i)] = &payloadRuntimeState{Payload: payload}
	}
}

func decodePayloadData(data []byte) (*payloadData, bool) {
	if len(data) == 0 {
		return nil, false
	}
	decoded, err := protocol.ReadPayload(protocol.NewReader(data), nil)
	if err != nil {
		return nil, false
	}
	out := &payloadData{
		UnitTypeID: -1,
		Serialized: append([]byte(nil), data...),
	}
	switch v := decoded.(type) {
	case protocol.BuildPayload:
		out.Kind = payloadKindBlock
		out.BlockID = v.BlockID
		return out, true
	case protocol.UnitPayload:
		out.Kind = payloadKindUnit
		return out, true
	case protocol.PayloadBox:
		switch data[0] {
		case protocol.PayloadBlock:
			out.Kind = payloadKindBlock
			if len(data) >= 4 {
				out.BlockID = int16(uint16(data[1])<<8 | uint16(data[2]))
			}
			return out, true
		case protocol.PayloadUnit:
			out.Kind = payloadKindUnit
			return out, true
		}
	}
	return nil, false
}

func (w *World) BuildingConfig(pos int32) (any, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return nil, false
	}
	if value, ok := w.normalizedBuildingConfigLocked(pos); ok {
		return value, true
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || len(tile.Build.Config) == 0 {
		return nil, false
	}
	return decodeStoredBuildingConfig(tile.Build.Config)
}

func (w *World) stepSandboxSources(delta time.Duration) {
	if w.model == nil {
		return
	}
	frameDelta := float32(delta.Seconds() * 60)
	liquidRate := frameDelta
	if liquidRate < 1 {
		liquidRate = 1
	}
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		switch w.blockNameByID(int16(tile.Block)) {
		case "item-source":
			if item, ok := w.itemSourceCfg[pos]; ok {
				w.pushItemSourceLocked(pos, tile, item, frameDelta)
			}
		case "liquid-source":
			if liquid, ok := w.liquidSourceCfg[pos]; ok {
				w.pushLiquidSourceLocked(pos, tile, liquid, liquidRate)
			}
		}
	}
}

func (w *World) pushItemSourceLocked(pos int32, tile *Tile, item ItemID, frameDelta float32) {
	if tile == nil || tile.Build == nil || w.model == nil || frameDelta <= 0 {
		return
	}
	const itemsPerSecond = float32(100)
	limit := float32(60) / itemsPerSecond
	w.itemSourceAccum[pos] += frameDelta
	for w.itemSourceAccum[pos] >= limit {
		tile.Build.AddItem(item, 1)
		w.dumpSingleItemLocked(pos, tile, &item, nil)
		_ = tile.Build.RemoveItem(item, 1)
		w.itemSourceAccum[pos] -= limit
	}
}

func (w *World) pushLiquidSourceLocked(pos int32, tile *Tile, liquid LiquidID, amount float32) {
	if tile == nil || tile.Build == nil || w.model == nil || amount <= 0 {
		return
	}
	for _, off := range [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}} {
		nx, ny := tile.X+off[0], tile.Y+off[1]
		if !w.model.InBounds(nx, ny) {
			continue
		}
		other := &w.model.Tiles[ny*w.model.Width+nx]
		if other.Build == nil || other.Team != tile.Team || other.Block == 0 {
			continue
		}
		if moved := w.tryMoveLiquidLocked(pos, int32(ny*w.model.Width+nx), liquid, amount, 0); moved > 0 {
			break
		}
	}
}

func (w *World) itemCapacityForBlockLocked(tile *Tile) int32 {
	if tile == nil || tile.Block == 0 {
		return 0
	}
	switch w.blockNameByID(int16(tile.Block)) {
	case "conveyor", "titanium-conveyor", "armored-conveyor":
		return 3
	case "duct", "armored-duct", "duct-router", "overflow-duct", "underflow-duct":
		return 1
	case "core-shard":
		return 4000
	case "core-foundation":
		return 9000
	case "core-nucleus":
		return 13000
	case "core-bastion":
		return 2000
	case "core-citadel":
		return 3000
	case "core-acropolis":
		return 4000
	case "container":
		return 300
	case "vault":
		return 1000
	case "reinforced-container":
		return 160
	case "reinforced-vault":
		return 900
	case "duct-bridge":
		return 4
	case "plastanium-conveyor", "surge-conveyor", "surge-router":
		return 10
	case "router", "distributor":
		return 1
	case "bridge-conveyor", "phase-conveyor":
		return 10
	case "mass-driver":
		return 120
	case "payload-loader", "payload-unloader":
		return 100
	case "thorium-reactor":
		return 30
	default:
		return 0
	}
}

func (w *World) liquidCapacityForBlockLocked(tile *Tile) float32 {
	if tile == nil || tile.Block == 0 {
		return 0
	}
	switch w.blockNameByID(int16(tile.Block)) {
	case "conduit":
		return 20
	case "pulse-conduit":
		return 40
	case "plated-conduit", "reinforced-conduit":
		return 50
	case "thorium-reactor":
		return 30
	case "liquid-router":
		return 120
	case "liquid-container":
		return 700
	case "liquid-tank":
		return 1800
	case "bridge-conduit", "phase-conduit":
		return 100
	case "payload-loader", "payload-unloader":
		return 100
	case "reinforced-liquid-router":
		return 150
	case "reinforced-liquid-container":
		return 1000
	case "reinforced-liquid-tank":
		return 2700
	case "reinforced-bridge-conduit":
		return 120
	default:
		return 0
	}
}

func (w *World) stepNuclearReactors(delta time.Duration) {
	if w.model == nil {
		return
	}
	deltaFrames := float32(delta.Seconds() * 60)
	if deltaFrames <= 0 {
		return
	}
	const (
		thoriumItemID       = ItemID(5)
		reactorItemCapacity = float32(30)
		// Mindustry 156 Blocks.thoriumReactor:
		// itemDuration = 360f, heating = 0.02f
		reactorHeatingPerFrame    = float32(0.02)
		reactorItemDurationFrames = float32(360)
		reactorAmbientCooldown    = float32(60 * 20)
		reactorCoolantPower       = float32(0.5)
		reactorExplosionRadius    = 19
		reactorExplosionDamage    = float32(1250 * 4)
	)
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 || w.blockNameByID(int16(tile.Block)) != "thorium-reactor" {
			continue
		}
		state := w.reactorStates[pos]
		fuel := tile.Build.ItemAmount(thoriumItemID)
		fullness := clampf(float32(fuel)/reactorItemCapacity, 0, 1)

		if fuel > 0 {
			state.Heat += fullness * reactorHeatingPerFrame * minf(deltaFrames, 4)
			state.FuelProgress += deltaFrames
			for state.FuelProgress >= reactorItemDurationFrames {
				if !tile.Build.RemoveItem(thoriumItemID, 1) {
					state.FuelProgress = 0
					break
				}
				state.FuelProgress -= reactorItemDurationFrames
			}
		} else {
			state.FuelProgress = 0
			state.Heat = maxf(0, state.Heat-deltaFrames/reactorAmbientCooldown)
		}

		if state.Heat > 0 && len(tile.Build.Liquids) > 0 {
			cur := tile.Build.Liquids[0]
			maxUsed := minf(cur.Amount, state.Heat/reactorCoolantPower)
			if maxUsed > 0 && tile.Build.RemoveLiquid(cur.Liquid, maxUsed) {
				state.Heat -= maxUsed * reactorCoolantPower
			}
		}

		state.Heat = clampf(state.Heat, 0, 1)
		w.reactorStates[pos] = state

		if state.Heat >= 0.999 {
			w.explodeNuclearReactorLocked(tile.X, tile.Y, pos, tile.Team, reactorExplosionRadius, reactorExplosionDamage)
		}
	}
}

func (w *World) stepLiquidLogistics(delta time.Duration) {
	if w.model == nil {
		return
	}
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return
	}
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		switch w.blockNameByID(int16(tile.Block)) {
		case "conduit":
			w.stepConduitLocked(pos, tile, 1.0, true, dt)
		case "pulse-conduit":
			w.stepConduitLocked(pos, tile, 1.025, true, dt)
		case "plated-conduit":
			w.stepConduitLocked(pos, tile, 1.025, false, dt)
		case "reinforced-conduit":
			w.stepConduitLocked(pos, tile, 1.03, true, dt)
		case "liquid-router", "liquid-container", "liquid-tank", "reinforced-liquid-router", "reinforced-liquid-container", "reinforced-liquid-tank":
			if liquid, _, ok := firstBuildingLiquid(tile.Build); ok {
				w.dumpLiquidLocked(pos, tile, liquid, dt*60)
			}
		case "bridge-conduit", "phase-conduit":
			w.stepLiquidBridgeLocked(pos, tile, dt)
		case "reinforced-bridge-conduit":
			w.stepDirectionalLiquidBridgeLocked(pos, tile, dt)
		}
	}
}

func (w *World) stepLiquidBridgeLocked(pos int32, tile *Tile, dt float32) {
	if tile == nil || tile.Build == nil {
		return
	}
	liquid, _, ok := firstBuildingLiquid(tile.Build)
	if !ok {
		return
	}
	target, linked := w.bridgeTargetLocked(pos, tile)
	if !linked {
		_ = w.dumpLiquidLocked(pos, tile, liquid, dt*60)
		return
	}
	moved := w.tryMoveLiquidLocked(pos, target, liquid, dt*60, 0)
	if moved > 0 {
		_ = tile.Build.RemoveLiquid(liquid, moved)
	}
}

func (w *World) stepDirectionalLiquidBridgeLocked(pos int32, tile *Tile, dt float32) {
	if tile == nil || tile.Build == nil {
		return
	}
	liquid, _, ok := firstBuildingLiquid(tile.Build)
	if !ok {
		return
	}
	target, linked := w.directionBridgeTargetLocked(pos, tile, "reinforced-bridge-conduit", 4)
	if linked {
		moved := w.tryMoveLiquidLocked(pos, target, liquid, dt*60, 0)
		if moved > 0 {
			_ = tile.Build.RemoveLiquid(liquid, moved)
		}
		return
	}
	if nextPos, ok := w.forwardPosLocked(pos, tile.Rotation); ok {
		if moved := w.tryMoveLiquidLocked(pos, nextPos, liquid, dt*60, 0); moved > 0 {
			_ = tile.Build.RemoveLiquid(liquid, moved)
		}
	}
}

func (w *World) stepConduitLocked(pos int32, tile *Tile, pressure float32, leaks bool, dt float32) {
	if tile == nil || tile.Build == nil || pressure <= 0 {
		return
	}
	liquid, amount, ok := firstBuildingLiquid(tile.Build)
	if !ok || amount <= 0.0001 {
		return
	}
	move := amount
	maxMove := dt * 60 * pressure
	if move > maxMove {
		move = maxMove
	}
	if move <= 0 {
		return
	}
	if nextPos, ok := w.forwardPosLocked(pos, tile.Rotation); ok {
		if moved := w.tryMoveLiquidLocked(pos, nextPos, liquid, move, 0); moved > 0 {
			_ = tile.Build.RemoveLiquid(liquid, moved)
			return
		}
	}
	if leaks {
		_ = w.dumpLiquidLocked(pos, tile, liquid, move)
	}
}

func (w *World) explodeNuclearReactorLocked(x, y int, pos int32, team TeamID, radius int, damage float32) {
	rules := w.rulesMgr.Get()
	if rules != nil && !rules.ReactorExplosions {
		_ = w.applyDamageToBuilding(pos, damage)
		w.clearBuildingRuntimeLocked(pos)
		return
	}
	for ty := y - radius; ty <= y+radius; ty++ {
		for tx := x - radius; tx <= x+radius; tx++ {
			if !w.model.InBounds(tx, ty) {
				continue
			}
			dx := tx - x
			dy := ty - y
			if dx*dx+dy*dy > radius*radius {
				continue
			}
			tpos := int32(ty*w.model.Width + tx)
			if w.applyDamageToBuilding(tpos, damage) {
				w.clearBuildingRuntimeLocked(tpos)
			}
		}
	}
	delete(w.reactorStates, pos)
}

const conveyorItemSpace = float32(0.4)

func isConveyorBlock(name string) bool {
	switch name {
	case "conveyor", "titanium-conveyor", "armored-conveyor":
		return true
	default:
		return false
	}
}

func isDuctBlock(name string) bool {
	switch name {
	case "duct", "armored-duct":
		return true
	default:
		return false
	}
}

func isStackConveyorBlock(name string) bool {
	switch name {
	case "plastanium-conveyor", "surge-conveyor":
		return true
	default:
		return false
	}
}

func isRotatingTransportBlock(name string) bool {
	switch name {
	case "conveyor", "titanium-conveyor", "armored-conveyor", "bridge-conveyor", "sorter", "inverted-sorter", "overflow-gate", "underflow-gate", "duct", "armored-duct", "duct-router", "overflow-duct", "underflow-duct", "duct-bridge", "duct-unloader", "plastanium-conveyor", "surge-conveyor", "surge-router":
		return true
	default:
		return false
	}
}

func isInstantTransferBlock(name string) bool {
	switch name {
	case "sorter", "inverted-sorter", "overflow-gate", "underflow-gate":
		return true
	default:
		return false
	}
}

func isRouterOrInstantTransferBlock(name string) bool {
	return isRouterBlock(name) || isInstantTransferBlock(name)
}

func isRouterBlock(name string) bool {
	return name == "router" || name == "distributor"
}

func isDuctRouterBlock(name string) bool {
	return name == "duct-router" || name == "surge-router"
}

func isItemBridgeBlock(name string) bool {
	return name == "bridge-conveyor" || name == "phase-conveyor" || name == "bridge-conduit" || name == "phase-conduit"
}

func (w *World) conveyorStateLocked(pos int32, tile *Tile) *conveyorRuntimeState {
	if st, ok := w.conveyorStates[pos]; ok && st != nil {
		return st
	}
	st := &conveyorRuntimeState{
		LastInserted: -1,
		MinItem:      1,
	}
	if tile != nil && tile.Build != nil {
		index := 0
		for _, stack := range tile.Build.Items {
			for amount := int32(0); amount < stack.Amount && index < len(st.IDs); amount++ {
				st.IDs[index] = stack.Item
				st.XS[index] = 0
				st.YS[index] = float32(index) * conveyorItemSpace
				index++
			}
			if index >= len(st.IDs) {
				break
			}
		}
		st.Len = index
		if st.Len > 0 {
			st.MinItem = st.YS[0]
		}
	}
	w.conveyorStates[pos] = st
	return st
}

func (w *World) syncConveyorInventoryLocked(tile *Tile, st *conveyorRuntimeState) {
	if tile == nil || tile.Build == nil || st == nil {
		return
	}
	if st.Len <= 0 {
		tile.Build.Items = nil
		st.Len = 0
		st.MinItem = 1
		st.LastInserted = -1
		st.Mid = 0
		return
	}
	items := make([]ItemStack, 0, st.Len)
	for i := 0; i < st.Len; i++ {
		item := st.IDs[i]
		found := false
		for j := range items {
			if items[j].Item == item {
				items[j].Amount++
				found = true
				break
			}
		}
		if !found {
			items = append(items, ItemStack{Item: item, Amount: 1})
		}
	}
	tile.Build.Items = items
}

func (st *conveyorRuntimeState) add(index int) {
	if st == nil || st.Len >= len(st.IDs) {
		return
	}
	if index < 0 {
		index = 0
	}
	if index > st.Len {
		index = st.Len
	}
	for i := st.Len; i > index; i-- {
		st.IDs[i] = st.IDs[i-1]
		st.XS[i] = st.XS[i-1]
		st.YS[i] = st.YS[i-1]
	}
	st.Len++
}

func (st *conveyorRuntimeState) remove(index int) {
	if st == nil || index < 0 || index >= st.Len {
		return
	}
	for i := index; i < st.Len-1; i++ {
		st.IDs[i] = st.IDs[i+1]
		st.XS[i] = st.XS[i+1]
		st.YS[i] = st.YS[i+1]
	}
	st.Len--
	st.LastInserted = -1
	if st.Len >= 0 && st.Len < len(st.IDs) {
		st.IDs[st.Len] = 0
		st.XS[st.Len] = 0
		st.YS[st.Len] = 0
	}
}

func (w *World) conveyorAcceptsItemLocked(fromPos, toPos int32) bool {
	if w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	st := w.conveyorStateLocked(toPos, toTile)
	if st.Len >= len(st.IDs) {
		return false
	}
	sourceSide, ok := w.flowDirBetweenLocked(fromPos, toPos)
	if !ok {
		return false
	}
	fromTile := &w.model.Tiles[fromPos]
	targetName := w.blockNameByID(int16(toTile.Block))
	if targetName == "armored-conveyor" {
		sourceName := w.blockNameByID(int16(fromTile.Block))
		if !isConveyorBlock(sourceName) && sourceSide != byte(((int(toTile.Rotation)%4)+4)%4) {
			return false
		}
	}
	direction := absInt(int(sourceSide) - int(toTile.Rotation))
	if direction == 0 {
		if nextPos, ok := w.forwardPosLocked(toPos, toTile.Rotation); ok && nextPos == fromPos && isRotatingTransportBlock(w.blockNameByID(int16(fromTile.Block))) {
			return false
		}
		return st.MinItem >= conveyorItemSpace
	}
	return direction%2 == 1 && st.MinItem > 0.7
}

func (w *World) conveyorHandleItemLocked(fromPos, toPos int32, item ItemID) bool {
	if !w.conveyorAcceptsItemLocked(fromPos, toPos) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	st := w.conveyorStateLocked(toPos, toTile)
	sourceSide, _ := w.flowDirBetweenLocked(fromPos, toPos)
	ang := int(sourceSide) - int(toTile.Rotation)
	x := float32(0)
	if ang == -1 || ang == 3 {
		x = 1
	} else if ang == 1 || ang == -3 {
		x = -1
	}
	if absInt(ang) == 0 {
		st.add(0)
		st.IDs[0] = item
		st.XS[0] = x
		st.YS[0] = 0
		st.LastInserted = 0
	} else {
		index := st.Mid
		if index < 0 {
			index = 0
		}
		if index > st.Len {
			index = st.Len
		}
		st.add(index)
		st.IDs[index] = item
		st.XS[index] = x
		st.YS[index] = 0.5
		st.LastInserted = index
	}
	st.MinItem = minf(st.MinItem, st.YS[st.LastInserted])
	w.syncConveyorInventoryLocked(toTile, st)
	return true
}

func (w *World) ductAcceptsItemLocked(fromPos, toPos int32, armored bool) bool {
	if w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil || totalBuildingItems(toTile.Build) > 0 {
		return false
	}
	sourceDir, ok := w.flowDirBetweenLocked(fromPos, toPos)
	if !ok {
		return false
	}
	if armored {
		if sourceDir == byte(tileRotationNorm(toTile.Rotation)) {
			return true
		}
		fromTile := &w.model.Tiles[fromPos]
		name := w.blockNameByID(int16(fromTile.Block))
		if isDuctBlock(name) || isDuctRouterBlock(name) || name == "overflow-duct" || name == "underflow-duct" || name == "duct-bridge" || name == "duct-unloader" {
			if nextPos, ok := w.forwardPosLocked(fromPos, fromTile.Rotation); ok && nextPos == toPos {
				return true
			}
		}
		return false
	}
	return sourceDir != byte((tileRotationNorm(toTile.Rotation)+2)%4)
}

func (w *World) ductHandleItemLocked(fromPos, toPos int32, item ItemID, armored bool) bool {
	if !w.ductAcceptsItemLocked(fromPos, toPos, armored) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	st := w.ductStateLocked(toPos, toTile)
	sourceDir, _ := w.flowDirBetweenLocked(fromPos, toPos)
	toTile.Build.AddItem(item, 1)
	st.Current = item
	st.HasItem = true
	st.Progress = -1
	st.RecDir = sourceDir
	return true
}

func (w *World) ductRouterAcceptsItemLocked(fromPos, toPos int32, item ItemID) bool {
	if w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil {
		return false
	}
	if sourceDir, ok := w.flowDirBetweenLocked(fromPos, toPos); !ok || sourceDir != byte(tileRotationNorm(toTile.Rotation)) {
		return false
	}
	name := w.blockNameByID(int16(toTile.Block))
	cap := w.itemCapacityForBlockLocked(toTile)
	if totalBuildingItems(toTile.Build) >= cap {
		return false
	}
	if name == "surge-router" {
		st := w.stackStateLocked(toPos, toTile)
		return !st.Unloading && (!st.HasItem || st.LastItem == item)
	}
	st := w.ductStateLocked(toPos, toTile)
	return !st.HasItem && totalBuildingItems(toTile.Build) == 0
}

func (w *World) ductRouterHandleItemLocked(fromPos, toPos int32, item ItemID, stack bool) bool {
	if !w.ductRouterAcceptsItemLocked(fromPos, toPos, item) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	toTile.Build.AddItem(item, 1)
	if stack {
		sst := w.stackStateLocked(toPos, toTile)
		sst.LastItem = item
		sst.HasItem = true
		sst.Link = toPos
		sst.Unloading = false
	}
	st := w.ductStateLocked(toPos, toTile)
	sourceDir, _ := w.flowDirBetweenLocked(fromPos, toPos)
	st.Current = item
	st.HasItem = true
	st.Progress = -1
	st.RecDir = sourceDir
	return true
}

func (w *World) ductBridgeAcceptsItemLocked(fromPos, toPos int32, item ItemID) bool {
	if w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil || totalBuildingItems(toTile.Build) >= w.itemCapacityForBlockLocked(toTile) {
		return false
	}
	if _, ok := w.directionBridgeTargetLocked(toPos, toTile, "duct-bridge", 4); !ok {
		return false
	}
	rel, ok := w.relativeToEdgeLocked(fromPos, toPos)
	if !ok || rel == byte(tileRotationNorm(toTile.Rotation)) {
		return false
	}
	incomingDir, ok := w.flowDirBetweenLocked(fromPos, toPos)
	if !ok {
		return false
	}
	for i := range w.model.Tiles {
		if int32(i) == fromPos || int32(i) == toPos {
			continue
		}
		other := &w.model.Tiles[i]
		if other.Build == nil || other.Team != toTile.Team || w.blockNameByID(int16(other.Block)) != "duct-bridge" {
			continue
		}
		target, ok := w.directionBridgeTargetLocked(int32(i), other, "duct-bridge", 4)
		if !ok || target != toPos {
			continue
		}
		dir, ok := w.flowDirBetweenLocked(int32(i), toPos)
		if ok && dir == incomingDir {
			return false
		}
	}
	_ = item
	return true
}

func (w *World) stackConveyorAcceptsItemLocked(fromPos, toPos int32, item ItemID) bool {
	if w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil {
		return false
	}
	st := w.stackStateLocked(toPos, toTile)
	if st.Cooldown > 1 {
		return false
	}
	if totalBuildingItems(toTile.Build) >= w.itemCapacityForBlockLocked(toTile) {
		return false
	}
	if st.HasItem && st.LastItem != item {
		return false
	}
	if nextPos, ok := w.forwardPosLocked(toPos, toTile.Rotation); ok && nextPos == fromPos {
		return false
	}
	return true
}

func (w *World) stackConveyorHandleItemLocked(fromPos, toPos int32, item ItemID) bool {
	if !w.stackConveyorAcceptsItemLocked(fromPos, toPos, item) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	toTile.Build.AddItem(item, 1)
	st := w.stackStateLocked(toPos, toTile)
	st.LastItem = item
	st.HasItem = true
	if st.Link < 0 {
		st.Link = toPos
	}
	return true
}

func tileRotationNorm(rotation int8) int {
	return ((int(rotation) % 4) + 4) % 4
}

func (w *World) routerStateLocked(pos int32, tile *Tile) *routerRuntimeState {
	if st, ok := w.routerStates[pos]; ok && st != nil {
		if !st.HasItem && tile != nil && tile.Build != nil {
			if item, ok := firstBuildingItem(tile.Build); ok {
				st.LastItem = item
				st.HasItem = true
			}
		}
		return st
	}
	st := &routerRuntimeState{LastInput: -1}
	if tile != nil && tile.Build != nil {
		if item, ok := firstBuildingItem(tile.Build); ok {
			st.LastItem = item
			st.HasItem = true
		}
	}
	w.routerStates[pos] = st
	return st
}

func (w *World) stepItemLogistics(delta time.Duration) {
	if w.model == nil {
		return
	}
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return
	}
	w.stepJunctions(dt)
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		switch w.blockNameByID(int16(tile.Block)) {
		case "conveyor":
			w.stepConveyorLocked(pos, tile, 0.03, dt)
		case "titanium-conveyor", "armored-conveyor":
			w.stepConveyorLocked(pos, tile, 0.08, dt)
		case "duct":
			w.stepDuctLocked(pos, tile, 4, false, dt)
		case "armored-duct":
			w.stepDuctLocked(pos, tile, 4, true, dt)
		case "duct-router":
			w.stepDuctRouterLocked(pos, tile, 4, false, dt)
		case "overflow-duct":
			w.stepOverflowDuctLocked(pos, tile, 4, false, dt)
		case "underflow-duct":
			w.stepOverflowDuctLocked(pos, tile, 4, true, dt)
		case "duct-bridge":
			w.stepDuctBridgeLocked(pos, tile, 4, dt)
		case "duct-unloader":
			w.stepDirectionalUnloaderLocked(pos, tile, 4, dt)
		case "router", "distributor":
			w.stepRouterLocked(pos, tile, 8, dt)
		case "bridge-conveyor":
			w.stepBridgeConveyorLocked(pos, tile, 11, dt)
		case "phase-conveyor":
			w.stepPhaseConveyorLocked(pos, tile, dt)
		case "plastanium-conveyor":
			w.stepStackConveyorLocked(pos, tile, 4.0/60.0, 2, true, dt)
		case "surge-conveyor":
			w.stepStackConveyorLocked(pos, tile, 5.0/60.0, 2, false, dt)
		case "surge-router":
			w.stepStackRouterLocked(pos, tile, 6, dt)
		case "unloader":
			w.stepUnloaderLocked(pos, tile, dt)
		case "mass-driver":
			w.stepMassDriverLocked(pos, tile, dt)
		}
	}
	w.stepMassDriverShotsLocked(dt)
}

func (w *World) payloadStateLocked(pos int32) *payloadRuntimeState {
	if st, ok := w.payloadStates[pos]; ok && st != nil {
		return st
	}
	st := &payloadRuntimeState{}
	w.payloadStates[pos] = st
	return st
}

func (w *World) payloadDriverStateLocked(pos int32) *payloadDriverRuntimeState {
	if st, ok := w.payloadDriverStates[pos]; ok && st != nil {
		return st
	}
	st := &payloadDriverRuntimeState{}
	w.payloadDriverStates[pos] = st
	return st
}

func (w *World) syncPayloadTileLocked(tile *Tile, payload *payloadData) {
	if tile == nil || tile.Build == nil {
		return
	}
	if payload == nil || len(payload.Serialized) == 0 {
		tile.Build.Payload = nil
		return
	}
	tile.Build.Payload = append(tile.Build.Payload[:0], payload.Serialized...)
}

func (w *World) clearPayloadLocked(pos int32, tile *Tile) {
	st := w.payloadStateLocked(pos)
	st.Payload = nil
	st.Move = 0
	st.Work = 0
	st.Exporting = false
	w.syncPayloadTileLocked(tile, nil)
}

func totalPayloadItems(payload *payloadData) int32 {
	if payload == nil {
		return 0
	}
	var total int32
	for _, stack := range payload.Items {
		total += stack.Amount
	}
	return total
}

func totalPayloadLiquids(payload *payloadData) float32 {
	if payload == nil {
		return 0
	}
	var total float32
	for _, stack := range payload.Liquids {
		total += stack.Amount
	}
	return total
}

func payloadAddItem(payload *payloadData, item ItemID, amount int32) {
	if payload == nil || amount <= 0 {
		return
	}
	payload.Serialized = nil
	for i := range payload.Items {
		if payload.Items[i].Item == item {
			payload.Items[i].Amount += amount
			return
		}
	}
	payload.Items = append(payload.Items, ItemStack{Item: item, Amount: amount})
}

func payloadRemoveItem(payload *payloadData, item ItemID, amount int32) bool {
	if payload == nil || amount <= 0 {
		return false
	}
	for i := range payload.Items {
		if payload.Items[i].Item != item {
			continue
		}
		if payload.Items[i].Amount < amount {
			return false
		}
		payload.Serialized = nil
		payload.Items[i].Amount -= amount
		if payload.Items[i].Amount <= 0 {
			payload.Items = append(payload.Items[:i], payload.Items[i+1:]...)
		}
		return true
	}
	return false
}

func payloadAddLiquid(payload *payloadData, liquid LiquidID, amount float32) {
	if payload == nil || amount <= 0 {
		return
	}
	payload.Serialized = nil
	for i := range payload.Liquids {
		if payload.Liquids[i].Liquid == liquid {
			payload.Liquids[i].Amount += amount
			return
		}
	}
	payload.Liquids = append(payload.Liquids, LiquidStack{Liquid: liquid, Amount: amount})
}

func payloadRemoveLiquid(payload *payloadData, liquid LiquidID, amount float32) bool {
	if payload == nil || amount <= 0 {
		return false
	}
	for i := range payload.Liquids {
		if payload.Liquids[i].Liquid != liquid {
			continue
		}
		if payload.Liquids[i].Amount+0.0001 < amount {
			return false
		}
		payload.Serialized = nil
		payload.Liquids[i].Amount -= amount
		if payload.Liquids[i].Amount <= 0.0001 {
			payload.Liquids = append(payload.Liquids[:i], payload.Liquids[i+1:]...)
		}
		return true
	}
	return false
}

func (w *World) payloadSizeBlocksLocked(payload *payloadData) int {
	if payload == nil {
		return 0
	}
	if payload.Kind == payloadKindBlock {
		name := w.blockNameByID(payload.BlockID)
		if name != "" {
			return blockSizeByName(name)
		}
	}
	return 1
}

func (w *World) payloadItemCapacityLocked(payload *payloadData) int32 {
	if payload == nil || payload.Kind != payloadKindBlock {
		return 0
	}
	return w.itemCapacityForBlockLocked(&Tile{Block: BlockID(payload.BlockID)})
}

func (w *World) payloadLiquidCapacityLocked(payload *payloadData) float32 {
	if payload == nil || payload.Kind != payloadKindBlock {
		return 0
	}
	return w.liquidCapacityForBlockLocked(&Tile{Block: BlockID(payload.BlockID)})
}

func (w *World) payloadFilterMatchesLocked(payload *payloadData, filter protocol.Content) bool {
	if payload == nil || filter == nil {
		return false
	}
	switch filter.ContentType() {
	case protocol.ContentBlock:
		return payload.Kind == payloadKindBlock && payload.BlockID == filter.ID()
	case protocol.ContentUnit:
		return payload.Kind == payloadKindUnit && payload.UnitTypeID == filter.ID()
	default:
		return false
	}
}

func isPayloadTransportBlock(name string) bool {
	switch name {
	case "payload-conveyor", "reinforced-payload-conveyor",
		"payload-router", "reinforced-payload-router",
		"payload-mass-driver", "large-payload-mass-driver",
		"payload-loader", "payload-unloader":
		return true
	default:
		return false
	}
}

func payloadMoveTimeByName(name string) float32 {
	switch name {
	case "reinforced-payload-conveyor", "reinforced-payload-router":
		return 35
	default:
		return 45
	}
}

func (w *World) payloadFrontTargetLocked(pos int32, tile *Tile, rotation int8) (int32, bool) {
	if w.model == nil || tile == nil {
		return 0, false
	}
	ntrns := w.blockSizeForTileLocked(tile)/2 + 1
	dx, dy := dirDelta(rotation)
	targetPos, ok := w.buildingOccupyingCellLocked(tile.X+dx*ntrns, tile.Y+dy*ntrns)
	if !ok || targetPos == pos || targetPos < 0 || int(targetPos) >= len(w.model.Tiles) {
		return 0, false
	}
	return targetPos, true
}

func (w *World) payloadAcceptsLocked(fromPos, toPos int32, payload *payloadData) bool {
	if w.model == nil || payload == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	fromTile := &w.model.Tiles[fromPos]
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil || toTile.Team != fromTile.Team || toTile.Block == 0 {
		return false
	}
	if w.payloadStateLocked(toPos).Payload != nil {
		return false
	}
	size := w.payloadSizeBlocksLocked(payload)
	switch w.blockNameByID(int16(toTile.Block)) {
	case "payload-conveyor", "reinforced-payload-conveyor", "payload-router", "reinforced-payload-router":
		return size <= 3
	case "payload-loader", "payload-unloader":
		return payload.Kind == payloadKindBlock && size <= 3
	case "payload-mass-driver":
		return size <= 2
	case "large-payload-mass-driver":
		return size <= 4
	default:
		return false
	}
}

func (w *World) payloadHandleLocked(fromPos, toPos int32, payload *payloadData) bool {
	if !w.payloadAcceptsLocked(fromPos, toPos, payload) {
		return false
	}
	targetTile := &w.model.Tiles[toPos]
	targetState := w.payloadStateLocked(toPos)
	targetState.Payload = payload
	targetState.Move = 0
	targetState.Work = 0
	targetState.Exporting = false
	if dir, ok := w.flowDirBetweenLocked(fromPos, toPos); ok {
		targetState.RecDir = dir
	} else {
		targetState.RecDir = byte(tileRotationNorm(targetTile.Rotation))
	}
	w.syncPayloadTileLocked(targetTile, payload)
	return true
}

func (w *World) payloadMoveOutLocked(pos int32, tile *Tile, targetPos int32) bool {
	st := w.payloadStateLocked(pos)
	if st.Payload == nil {
		return false
	}
	if !w.payloadHandleLocked(pos, targetPos, st.Payload) {
		return false
	}
	w.clearPayloadLocked(pos, tile)
	return true
}

func (w *World) stepPayloadLogistics(delta time.Duration) {
	if w.model == nil {
		return
	}
	frames := float32(delta.Seconds() * 60)
	if frames <= 0 {
		return
	}
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		switch w.blockNameByID(int16(tile.Block)) {
		case "payload-conveyor", "reinforced-payload-conveyor":
			w.stepPayloadConveyorLocked(pos, tile, payloadMoveTimeByName(w.blockNameByID(int16(tile.Block))), frames)
		case "payload-router", "reinforced-payload-router":
			w.stepPayloadRouterLocked(pos, tile, payloadMoveTimeByName(w.blockNameByID(int16(tile.Block))), frames)
		case "payload-mass-driver":
			w.stepPayloadMassDriverLocked(pos, tile, 130, 90, frames)
		case "large-payload-mass-driver":
			w.stepPayloadMassDriverLocked(pos, tile, 130, 100, frames)
		case "payload-loader":
			w.stepPayloadLoaderLocked(pos, tile, frames)
		case "payload-unloader":
			w.stepPayloadUnloaderLocked(pos, tile, frames)
		}
	}
	w.stepPayloadDriverShotsLocked(frames)
}

func (w *World) stepPayloadConveyorLocked(pos int32, tile *Tile, moveTime, frames float32) {
	st := w.payloadStateLocked(pos)
	if st.Payload == nil {
		st.Move = 0
		w.syncPayloadTileLocked(tile, nil)
		return
	}
	st.Move += frames
	if st.Move < moveTime {
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	targetPos, ok := w.payloadFrontTargetLocked(pos, tile, tile.Rotation)
	if !ok || !w.payloadMoveOutLocked(pos, tile, targetPos) {
		st.Move = moveTime
		w.syncPayloadTileLocked(tile, st.Payload)
	}
}

func (w *World) stepPayloadRouterLocked(pos int32, tile *Tile, moveTime, frames float32) {
	st := w.payloadStateLocked(pos)
	if st.Payload == nil {
		st.Move = 0
		w.syncPayloadTileLocked(tile, nil)
		return
	}
	st.Move += frames
	if st.Move < moveTime {
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	filter := w.payloadRouterCfg[pos]
	matches := w.payloadFilterMatchesLocked(st.Payload, filter)
	forward := int8(st.RecDir % 4)
	candidates := make([]int8, 0, 4)
	if matches {
		candidates = append(candidates, forward)
	} else {
		start := int8((tileRotationNorm(tile.Rotation) + 1) % 4)
		for i := 0; i < 4; i++ {
			dir := int8((int(start) + i) % 4)
			if filter != nil && dir == forward {
				continue
			}
			candidates = append(candidates, dir)
		}
		if len(candidates) == 0 {
			candidates = append(candidates, forward)
		}
	}
	for _, dir := range candidates {
		targetPos, ok := w.payloadFrontTargetLocked(pos, tile, dir)
		if !ok {
			continue
		}
		if !w.payloadMoveOutLocked(pos, tile, targetPos) {
			continue
		}
		tile.Rotation = dir
		if tile.Build != nil {
			tile.Build.Rotation = dir
		}
		return
	}
	st.Move = moveTime
	w.syncPayloadTileLocked(tile, st.Payload)
}

func (w *World) payloadDriverTargetLocked(pos int32, tile *Tile) (int32, bool) {
	if w.model == nil || tile == nil {
		return 0, false
	}
	target, ok := w.payloadDriverLinks[pos]
	if !ok || target < 0 || int(target) >= len(w.model.Tiles) {
		return 0, false
	}
	targetTile := &w.model.Tiles[target]
	if targetTile.Build == nil || targetTile.Team != tile.Team || w.blockNameByID(int16(targetTile.Block)) != w.blockNameByID(int16(tile.Block)) {
		return 0, false
	}
	return target, true
}

func (w *World) payloadDriverIncomingShotsLocked(pos int32) int {
	count := 0
	for _, shot := range w.payloadDriverShots {
		if shot.ToPos == pos {
			count++
		}
	}
	return count
}

func (w *World) stepPayloadMassDriverLocked(pos int32, tile *Tile, reloadFrames, chargeFrames, frames float32) {
	st := w.payloadStateLocked(pos)
	driver := w.payloadDriverStateLocked(pos)
	if driver.ReloadCounter > 0 {
		driver.ReloadCounter = maxf(0, driver.ReloadCounter-frames)
	}
	if st.Payload == nil {
		driver.Charge = 0
		w.syncPayloadTileLocked(tile, nil)
		return
	}
	size := w.payloadSizeBlocksLocked(st.Payload)
	switch w.blockNameByID(int16(tile.Block)) {
	case "payload-mass-driver":
		if size > 2 {
			driver.Charge = 0
			w.syncPayloadTileLocked(tile, st.Payload)
			return
		}
	case "large-payload-mass-driver":
		if size > 4 {
			driver.Charge = 0
			w.syncPayloadTileLocked(tile, st.Payload)
			return
		}
	}
	target, ok := w.payloadDriverTargetLocked(pos, tile)
	if !ok || driver.ReloadCounter > 0 {
		driver.Charge = 0
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	targetState := w.payloadStateLocked(target)
	targetDriver := w.payloadDriverStateLocked(target)
	if targetState.Payload != nil || targetDriver.ReloadCounter > 0 || w.payloadDriverIncomingShotsLocked(target) > 0 {
		driver.Charge = 0
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	driver.Charge += frames
	if driver.Charge < chargeFrames {
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	targetTile := &w.model.Tiles[target]
	dx := float32(targetTile.X-tile.X) * 8
	dy := float32(targetTile.Y-tile.Y) * 8
	travelFrames := float32(math.Sqrt(float64(dx*dx+dy*dy)) / 5.5)
	if travelFrames < 1 {
		travelFrames = 1
	}
	w.payloadDriverShots = append(w.payloadDriverShots, payloadDriverShot{
		FromPos:      pos,
		ToPos:        target,
		TravelFrames: travelFrames,
		Payload:      st.Payload,
	})
	driver.Charge = 0
	driver.ReloadCounter = reloadFrames
	w.clearPayloadLocked(pos, tile)
}

func (w *World) stepPayloadDriverShotsLocked(frames float32) {
	if len(w.payloadDriverShots) == 0 {
		return
	}
	kept := w.payloadDriverShots[:0]
	for _, shot := range w.payloadDriverShots {
		shot.AgeFrames += frames
		if shot.AgeFrames < shot.TravelFrames {
			kept = append(kept, shot)
			continue
		}
		if w.model == nil || shot.ToPos < 0 || int(shot.ToPos) >= len(w.model.Tiles) {
			continue
		}
		targetTile := &w.model.Tiles[shot.ToPos]
		if targetTile.Build == nil {
			continue
		}
		if w.payloadStateLocked(shot.ToPos).Payload != nil {
			shot.AgeFrames = shot.TravelFrames
			kept = append(kept, shot)
			continue
		}
		state := w.payloadStateLocked(shot.ToPos)
		state.Payload = shot.Payload
		state.Move = 0
		state.Work = 0
		state.Exporting = false
		w.syncPayloadTileLocked(targetTile, shot.Payload)
	}
	w.payloadDriverShots = kept
}

func (w *World) stepPayloadLoaderLocked(pos int32, tile *Tile, frames float32) {
	st := w.payloadStateLocked(pos)
	if st.Payload == nil {
		st.Work = 0
		w.syncPayloadTileLocked(tile, nil)
		return
	}
	moved := false
	holding := false
	itemCap := w.payloadItemCapacityLocked(st.Payload)
	if itemCap > 0 && totalBuildingItems(tile.Build) > 0 && totalPayloadItems(st.Payload) < itemCap {
		holding = true
		st.Work += frames
		for st.Work >= 2 {
			transferred := false
			for j := 0; j < 8; j++ {
				item, ok := firstBuildingItem(tile.Build)
				if !ok || totalPayloadItems(st.Payload) >= itemCap {
					break
				}
				if !tile.Build.RemoveItem(item, 1) {
					break
				}
				payloadAddItem(st.Payload, item, 1)
				moved = true
				transferred = true
			}
			st.Work -= 2
			if !transferred {
				break
			}
		}
	}
	if liquid, _, ok := firstBuildingLiquid(tile.Build); ok {
		if liquidCap := w.payloadLiquidCapacityLocked(st.Payload); liquidCap > 0 {
			space := liquidCap - totalPayloadLiquids(st.Payload)
			available := tile.Build.LiquidAmount(liquid)
			if space > 0 && available > 0 {
				holding = true
				flow := minf(space, minf(available, 40*frames))
				if flow > 0 && tile.Build.RemoveLiquid(liquid, flow) {
					payloadAddLiquid(st.Payload, liquid, flow)
					moved = true
				}
			}
		}
	}
	if holding || moved {
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	if !moved {
		targetPos, ok := w.payloadFrontTargetLocked(pos, tile, tile.Rotation)
		if ok {
			_ = w.payloadMoveOutLocked(pos, tile, targetPos)
		}
		return
	}
}

func (w *World) stepPayloadUnloaderLocked(pos int32, tile *Tile, frames float32) {
	st := w.payloadStateLocked(pos)
	if st.Payload == nil {
		st.Work = 0
		w.syncPayloadTileLocked(tile, nil)
		return
	}
	moved := false
	holding := false
	itemCap := w.itemCapacityForBlockLocked(tile)
	if itemCap > 0 && totalPayloadItems(st.Payload) > 0 && totalBuildingItems(tile.Build) < itemCap {
		holding = true
		st.Work += frames
		for st.Work >= 2 {
			transferred := false
			for j := 0; j < 8; j++ {
				if totalBuildingItems(tile.Build) >= itemCap || len(st.Payload.Items) == 0 {
					break
				}
				item := st.Payload.Items[0].Item
				if !payloadRemoveItem(st.Payload, item, 1) {
					break
				}
				tile.Build.AddItem(item, 1)
				moved = true
				transferred = true
			}
			st.Work -= 2
			if !transferred {
				break
			}
		}
	}
	if liquidCap := w.liquidCapacityForBlockLocked(tile); liquidCap > 0 && len(st.Payload.Liquids) > 0 {
		liq := st.Payload.Liquids[0].Liquid
		space := liquidCap - totalBuildingLiquids(tile.Build)
		available := st.Payload.Liquids[0].Amount
		if space > 0 && available > 0 {
			holding = true
			flow := minf(space, minf(available, 40*frames))
			if flow > 0 && payloadRemoveLiquid(st.Payload, liq, flow) {
				tile.Build.AddLiquid(liq, flow)
				moved = true
			}
		}
	}
	if holding || moved {
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	if !moved && totalPayloadItems(st.Payload) == 0 && totalPayloadLiquids(st.Payload) == 0 {
		targetPos, ok := w.payloadFrontTargetLocked(pos, tile, tile.Rotation)
		if ok {
			_ = w.payloadMoveOutLocked(pos, tile, targetPos)
		}
		return
	}
}

func (w *World) stepConveyorLocked(pos int32, tile *Tile, speed float32, dt float32) {
	if tile == nil || tile.Build == nil || speed <= 0 {
		return
	}
	st := w.conveyorStateLocked(pos, tile)
	st.MinItem = 1
	st.Mid = 0
	if st.Len == 0 {
		w.syncConveyorInventoryLocked(tile, st)
		return
	}

	var (
		nextState *conveyorRuntimeState
		nextPos   int32
		hasNext   bool
		aligned   bool
		nextMax   = float32(1)
	)
	if outPos, ok := w.forwardItemTargetPosLocked(pos, tile.Rotation); ok {
		nextPos = outPos
		hasNext = true
		nextTile := &w.model.Tiles[nextPos]
		if nextTile.Build != nil && nextTile.Team == tile.Team && isConveyorBlock(w.blockNameByID(int16(nextTile.Block))) {
			nextState = w.conveyorStateLocked(nextPos, nextTile)
			aligned = nextTile.Rotation == tile.Rotation
			if aligned {
				nextMax = 1 - maxf(conveyorItemSpace-nextState.MinItem, 0)
			}
		}
	}

	moved := speed * dt * 60
	for i := st.Len - 1; i >= 0; i-- {
		nextPosY := float32(100)
		if i < st.Len-1 {
			nextPosY = st.YS[i+1] - conveyorItemSpace
		}
		maxMove := clampf(nextPosY-st.YS[i], 0, moved)
		st.YS[i] += maxMove
		if st.YS[i] > nextMax {
			st.YS[i] = nextMax
		}
		if st.YS[i] > 0.5 && i > 0 {
			st.Mid = i - 1
		}
		st.XS[i] = approachf(st.XS[i], 0, moved*2)

		if st.YS[i] >= 1 {
			item := st.IDs[i]
			xcarry := st.XS[i]
			if hasNext && w.tryInsertItemLocked(pos, nextPos, item, 0) {
				if aligned && nextState != nil && nextState.LastInserted >= 0 && nextState.LastInserted < nextState.Len {
					nextState.XS[nextState.LastInserted] = xcarry
					w.syncConveyorInventoryLocked(&w.model.Tiles[nextPos], nextState)
				}
				st.remove(i)
				continue
			}
		}
		if st.YS[i] < st.MinItem {
			st.MinItem = st.YS[i]
		}
	}
	if st.Len == 0 {
		st.MinItem = 1
	}
	w.syncConveyorInventoryLocked(tile, st)
}

func (w *World) ductStateLocked(pos int32, tile *Tile) *ductRuntimeState {
	if st, ok := w.ductStates[pos]; ok && st != nil {
		if !st.HasItem && tile != nil && tile.Build != nil {
			if item, ok := firstBuildingItem(tile.Build); ok {
				st.Current = item
				st.HasItem = true
			}
		}
		return st
	}
	st := &ductRuntimeState{}
	if tile != nil && tile.Build != nil {
		if item, ok := firstBuildingItem(tile.Build); ok {
			st.Current = item
			st.HasItem = true
		}
	}
	w.ductStates[pos] = st
	return st
}

func (w *World) stackStateLocked(pos int32, tile *Tile) *stackRuntimeState {
	if st, ok := w.stackStates[pos]; ok && st != nil {
		if tile != nil && tile.Build != nil {
			if item, ok := firstBuildingItem(tile.Build); ok {
				st.LastItem = item
				st.HasItem = true
			} else {
				st.HasItem = false
			}
		}
		return st
	}
	st := &stackRuntimeState{Link: -1}
	if tile != nil && tile.Build != nil {
		if item, ok := firstBuildingItem(tile.Build); ok {
			st.LastItem = item
			st.HasItem = true
			st.Link = pos
		}
	}
	w.stackStates[pos] = st
	return st
}

func (w *World) stepDuctLocked(pos int32, tile *Tile, speed float32, armored bool, dt float32) {
	if tile == nil || tile.Build == nil || speed <= 0 {
		return
	}
	st := w.ductStateLocked(pos, tile)
	if !st.HasItem {
		st.Progress = 0
		return
	}
	nextPos, ok := w.forwardItemTargetPosLocked(pos, tile.Rotation)
	if !ok {
		st.Progress = 0
		return
	}
	st.Progress += dt * 60 / speed * 2
	threshold := float32(1 - 1/speed)
	if st.Progress < threshold {
		return
	}
	if !w.tryInsertItemLocked(pos, nextPos, st.Current, 0) {
		return
	}
	if !tile.Build.RemoveItem(st.Current, 1) {
		return
	}
	st.HasItem = false
	st.Progress = float32(math.Mod(float64(st.Progress), float64(threshold)))
	if item, ok := firstBuildingItem(tile.Build); ok {
		st.Current = item
		st.HasItem = true
	} else {
		st.Progress = 0
	}
	_ = armored
}

func (w *World) stepDuctRouterLocked(pos int32, tile *Tile, speed float32, stack bool, dt float32) {
	if tile == nil || tile.Build == nil || speed <= 0 {
		return
	}
	cap := w.itemCapacityForBlockLocked(tile)
	total := totalBuildingItems(tile.Build)
	st := w.ductStateLocked(pos, tile)
	if stack {
		sst := w.stackStateLocked(pos, tile)
		if sst.Unloading {
			for {
				target, ok := w.ductRouterTargetLocked(pos, tile, sst.LastItem)
				if !ok || tile.Build.ItemAmount(sst.LastItem) <= 0 {
					break
				}
				if !w.tryInsertItemLocked(pos, target, sst.LastItem, 0) {
					break
				}
				if !tile.Build.RemoveItem(sst.LastItem, 1) {
					break
				}
			}
			if item, ok := firstBuildingItem(tile.Build); ok {
				sst.LastItem = item
				sst.HasItem = true
			} else {
				sst.HasItem = false
				sst.Unloading = false
				st.HasItem = false
				st.Progress = 0
			}
			return
		}
		if sst.HasItem && total >= cap {
			st.Progress += dt * 60
			if st.Progress >= speed {
				st.Progress = float32(math.Mod(float64(st.Progress), float64(speed)))
				sst.Unloading = true
			}
		} else if !sst.HasItem {
			st.Progress = 0
		}
		return
	}
	if !st.HasItem {
		st.Progress = 0
		return
	}
	st.Progress += dt * 60 / speed * 2
	threshold := float32(1 - 1/speed)
	if st.Progress < threshold {
		return
	}
	target, ok := w.ductRouterTargetLocked(pos, tile, st.Current)
	if !ok {
		return
	}
	if !w.tryInsertItemLocked(pos, target, st.Current, 0) {
		return
	}
	if !tile.Build.RemoveItem(st.Current, 1) {
		return
	}
	st.HasItem = false
	st.Progress = float32(math.Mod(float64(st.Progress), float64(threshold)))
	if item, ok := firstBuildingItem(tile.Build); ok {
		st.Current = item
		st.HasItem = true
	} else {
		st.Progress = 0
	}
}

func (w *World) stepOverflowDuctLocked(pos int32, tile *Tile, speed float32, invert bool, dt float32) {
	if tile == nil || tile.Build == nil || speed <= 0 {
		return
	}
	st := w.ductStateLocked(pos, tile)
	if !st.HasItem {
		st.Progress = 0
		return
	}
	st.Progress += dt * 60 / speed * 2
	threshold := float32(1 - 1/speed)
	if st.Progress < threshold {
		return
	}
	target, ok := w.overflowDuctTargetLocked(pos, tile, st.Current, invert)
	if !ok {
		return
	}
	if !w.tryInsertItemLocked(pos, target, st.Current, 0) {
		return
	}
	if !tile.Build.RemoveItem(st.Current, 1) {
		return
	}
	st.HasItem = false
	st.Progress = float32(math.Mod(float64(st.Progress), float64(threshold)))
	if item, ok := firstBuildingItem(tile.Build); ok {
		st.Current = item
		st.HasItem = true
	} else {
		st.Progress = 0
	}
}

func (w *World) stepDuctBridgeLocked(pos int32, tile *Tile, speed float32, dt float32) {
	if tile == nil || tile.Build == nil || speed <= 0 {
		return
	}
	target, ok := w.directionBridgeTargetLocked(pos, tile, "duct-bridge", 4)
	if ok {
		w.transportAccum[pos] += dt * 60
		for w.transportAccum[pos] > speed {
			item, exists := firstBuildingItem(tile.Build)
			if !exists {
				break
			}
			targetTile := &w.model.Tiles[target]
			if totalBuildingItems(targetTile.Build) >= w.itemCapacityForBlockLocked(targetTile) {
				break
			}
			if !tile.Build.RemoveItem(item, 1) {
				break
			}
			targetTile.Build.AddItem(item, 1)
			w.transportAccum[pos] -= speed
		}
		return
	}
	item, exists := firstBuildingItem(tile.Build)
	if !exists {
		return
	}
	nextPos, ok := w.forwardItemTargetPosLocked(pos, tile.Rotation)
	if !ok {
		return
	}
	if !w.tryInsertItemLocked(pos, nextPos, item, 0) {
		return
	}
	_ = tile.Build.RemoveItem(item, 1)
}

func (w *World) stepDirectionalUnloaderLocked(pos int32, tile *Tile, speed float32, dt float32) {
	if tile == nil || tile.Build == nil || speed <= 0 {
		return
	}
	w.transportAccum[pos] += dt * 60
	if w.transportAccum[pos] < speed {
		return
	}
	frontPos, fok := w.forwardItemTargetPosLocked(pos, tile.Rotation)
	backPos, bok := w.forwardItemTargetPosLocked(pos, int8((int(tile.Rotation)+2)%4))
	if !fok || !bok {
		w.transportAccum[pos] = minf(w.transportAccum[pos], speed)
		return
	}
	frontTile := &w.model.Tiles[frontPos]
	backTile := &w.model.Tiles[backPos]
	if frontTile.Build == nil || backTile.Build == nil || frontTile.Team != tile.Team || backTile.Team != tile.Team {
		w.transportAccum[pos] = minf(w.transportAccum[pos], speed)
		return
	}
	backName := w.blockNameByID(int16(backTile.Block))
	if strings.HasPrefix(backName, "core-") {
		w.transportAccum[pos] = minf(w.transportAccum[pos], speed)
		return
	}
	tryMove := func(item ItemID) bool {
		if backTile.Build.ItemAmount(item) <= 0 {
			return false
		}
		if !w.tryInsertItemLocked(pos, frontPos, item, 0) {
			return false
		}
		if !backTile.Build.RemoveItem(item, 1) {
			_ = frontTile.Build.RemoveItem(item, 1)
			return false
		}
		w.transportAccum[pos] = float32(math.Mod(float64(w.transportAccum[pos]), float64(speed)))
		return true
	}
	if item, ok := w.unloaderCfg[pos]; ok {
		if !tryMove(item) {
			w.transportAccum[pos] = minf(w.transportAccum[pos], speed)
		}
		return
	}
	start := w.blockDumpIndex[pos]
	for i := 0; i < 256; i++ {
		item := ItemID((start + i) % 256)
		if tryMove(item) {
			w.blockDumpIndex[pos] = int(item) + 1
			return
		}
	}
	w.transportAccum[pos] = minf(w.transportAccum[pos], speed)
}

func (w *World) stepStackConveyorLocked(pos int32, tile *Tile, speed float32, recharge float32, outputRouter bool, dt float32) {
	if tile == nil || tile.Build == nil || speed <= 0 {
		return
	}
	st := w.stackStateLocked(pos, tile)
	frames := dt * 60
	if st.Cooldown > 0 {
		st.Cooldown = maxf(0, st.Cooldown-speed*frames)
	}
	if item, ok := firstBuildingItem(tile.Build); ok {
		st.LastItem = item
		st.HasItem = true
		if st.Link < 0 {
			st.Link = pos
		}
	} else {
		st.HasItem = false
		st.Unloading = false
		st.Link = -1
		return
	}
	frontPos, hasFront := w.forwardItemTargetPosLocked(pos, tile.Rotation)
	if hasFront {
		frontTile := &w.model.Tiles[frontPos]
		if frontTile.Build != nil && frontTile.Team == tile.Team && isStackConveyorBlock(w.blockNameByID(int16(frontTile.Block))) && st.Cooldown <= 0 {
			frontState := w.stackStateLocked(frontPos, frontTile)
			if frontState.Link < 0 && (!outputRouter || totalBuildingItems(tile.Build) >= w.itemCapacityForBlockLocked(tile) || st.Link != pos) {
				frontTile.Build.Items = append(frontTile.Build.Items[:0], tile.Build.Items...)
				frontState.LastItem = st.LastItem
				frontState.HasItem = true
				frontState.Link = pos
				frontState.Cooldown = 1
				tile.Build.Items = nil
				st.HasItem = false
				st.Link = -1
				st.Cooldown = recharge
				return
			}
		}
	}
	if outputRouter {
		_ = w.dumpSingleItemLocked(pos, tile, &st.LastItem, nil)
	} else if hasFront {
		if w.tryInsertItemLocked(pos, frontPos, st.LastItem, 0) {
			_ = tile.Build.RemoveItem(st.LastItem, 1)
		}
	}
	if item, ok := firstBuildingItem(tile.Build); ok {
		st.LastItem = item
		st.HasItem = true
	} else {
		st.HasItem = false
		st.Link = -1
	}
}

func (w *World) stepStackRouterLocked(pos int32, tile *Tile, speed float32, dt float32) {
	if tile == nil || tile.Build == nil || speed <= 0 {
		return
	}
	dst := w.ductStateLocked(pos, tile)
	sst := w.stackStateLocked(pos, tile)
	if item, ok := firstBuildingItem(tile.Build); ok {
		sst.LastItem = item
		sst.HasItem = true
		dst.Current = item
		dst.HasItem = true
	} else {
		sst.HasItem = false
		sst.Unloading = false
		dst.HasItem = false
		dst.Progress = 0
		return
	}
	if !sst.Unloading && totalBuildingItems(tile.Build) >= w.itemCapacityForBlockLocked(tile) {
		dst.Progress += dt * 60
		if dst.Progress >= speed {
			dst.Progress = float32(math.Mod(float64(dst.Progress), float64(speed)))
			sst.Unloading = true
		}
	}
	if !sst.Unloading {
		return
	}
	for {
		target, ok := w.ductRouterTargetLocked(pos, tile, sst.LastItem)
		if !ok || tile.Build.ItemAmount(sst.LastItem) <= 0 {
			break
		}
		if !w.tryInsertItemLocked(pos, target, sst.LastItem, 0) {
			break
		}
		if !tile.Build.RemoveItem(sst.LastItem, 1) {
			break
		}
	}
	if item, ok := firstBuildingItem(tile.Build); ok {
		sst.LastItem = item
		sst.HasItem = true
		dst.Current = item
		dst.HasItem = true
	} else {
		sst.HasItem = false
		sst.Unloading = false
		dst.HasItem = false
		dst.Progress = 0
	}
}

func (w *World) stepRouterLocked(pos int32, tile *Tile, rate float32, dt float32) {
	if tile == nil || tile.Build == nil || rate <= 0 {
		return
	}
	st := w.routerStateLocked(pos, tile)
	if !st.HasItem {
		if item, ok := firstBuildingItem(tile.Build); ok {
			st.LastItem = item
			st.HasItem = true
		} else {
			st.Time = 0
			w.transportAccum[pos] = 0
			return
		}
	}
	st.Time += rate * dt
	target, ok := w.routerTargetLocked(pos, tile, st.LastItem, false)
	if !ok {
		return
	}
	targetName := w.blockNameByID(int16(w.model.Tiles[target].Block))
	if st.Time < 1 && isRouterOrInstantTransferBlock(targetName) {
		return
	}
	target, ok = w.routerTargetLocked(pos, tile, st.LastItem, true)
	if !ok {
		return
	}
	if !w.tryInsertItemLocked(pos, target, st.LastItem, 0) {
		return
	}
	_ = tile.Build.RemoveItem(st.LastItem, 1)
	st.HasItem = false
	st.Time = 0
	w.transportAccum[pos] = 0
}

func (w *World) stepBridgeConveyorLocked(pos int32, tile *Tile, rate float32, dt float32) {
	if tile == nil || tile.Build == nil || rate <= 0 {
		return
	}
	target, linked := w.bridgeTargetLocked(pos, tile)
	if !linked {
		w.dumpSingleItemLocked(pos, tile, nil, func(targetPos int32, item ItemID) bool {
			side, ok := w.flowDirBetweenLocked(pos, targetPos)
			return ok && !w.bridgeHasIncomingFromSideLocked(pos, side)
		})
		return
	}

	const (
		bufferCapacity   = 14
		bufferDelayFrame = float32(74)
		acceptEveryFrame = float32(4)
	)

	buffer := w.bridgeBuffers[pos]
	for len(buffer) < bufferCapacity {
		item, ok := firstBuildingItem(tile.Build)
		if !ok {
			break
		}
		if !tile.Build.RemoveItem(item, 1) {
			break
		}
		buffer = append(buffer, bufferedBridgeItem{Item: item})
	}
	for i := range buffer {
		buffer[i].AgeFrames += dt * 60
	}
	w.bridgeAcceptAcc[pos] += dt * 60
	for len(buffer) > 0 && buffer[0].AgeFrames >= bufferDelayFrame && w.bridgeAcceptAcc[pos] >= acceptEveryFrame {
		if !w.tryInsertItemLocked(pos, target, buffer[0].Item, 0) {
			break
		}
		buffer = buffer[1:]
		w.bridgeAcceptAcc[pos] -= acceptEveryFrame
	}
	if len(buffer) == 0 {
		delete(w.bridgeBuffers, pos)
	} else {
		w.bridgeBuffers[pos] = buffer
	}
}

func (w *World) stepPhaseConveyorLocked(pos int32, tile *Tile, dt float32) {
	if tile == nil || tile.Build == nil {
		return
	}
	target, linked := w.bridgeTargetLocked(pos, tile)
	if !linked {
		w.dumpSingleItemLocked(pos, tile, nil, func(targetPos int32, item ItemID) bool {
			side, ok := w.flowDirBetweenLocked(pos, targetPos)
			return ok && !w.bridgeHasIncomingFromSideLocked(pos, side)
		})
		return
	}
	w.transportAccum[pos] += dt * 60
	for w.transportAccum[pos] >= 2 {
		item, ok := firstBuildingItem(tile.Build)
		if !ok {
			break
		}
		if !w.tryInsertItemLocked(pos, target, item, 0) {
			break
		}
		if !tile.Build.RemoveItem(item, 1) {
			break
		}
		w.transportAccum[pos] -= 2
	}
}

func (w *World) stepUnloaderLocked(pos int32, tile *Tile, dt float32) {
	if tile == nil || tile.Build == nil {
		return
	}
	const unloadSpeedFrames = float32(60.0 / 11.0)
	w.transportAccum[pos] += dt * 60
	if w.transportAccum[pos] < unloadSpeedFrames {
		return
	}
	neighbors := w.dumpProximityLocked(pos)
	if len(neighbors) < 2 {
		return
	}
	item, ok := w.unloaderTargetItemLocked(pos, neighbors)
	if !ok {
		w.transportAccum[pos] = minf(w.transportAccum[pos], unloadSpeedFrames)
		return
	}
	fromPos, toPos, ok := w.unloaderTransferPairLocked(pos, neighbors, item)
	if !ok {
		w.transportAccum[pos] = minf(w.transportAccum[pos], unloadSpeedFrames)
		return
	}
	fromTile := &w.model.Tiles[fromPos]
	toTile := &w.model.Tiles[toPos]
	if fromTile.Build == nil || toTile.Build == nil {
		w.transportAccum[pos] = minf(w.transportAccum[pos], unloadSpeedFrames)
		return
	}
	if !w.tryInsertItemLocked(pos, toPos, item, 0) {
		w.transportAccum[pos] = minf(w.transportAccum[pos], unloadSpeedFrames)
		return
	}
	if !w.removeItemAtLocked(fromPos, item, 1) {
		_ = w.removeItemAtLocked(toPos, item, 1)
		w.transportAccum[pos] = minf(w.transportAccum[pos], unloadSpeedFrames)
		return
	}
	w.transportAccum[pos] = float32(math.Mod(float64(w.transportAccum[pos]), float64(unloadSpeedFrames)))
}

func (w *World) stepMassDriverLocked(pos int32, tile *Tile, dt float32) {
	if tile == nil || tile.Build == nil {
		return
	}
	st := w.massDriverStateLocked(pos)
	if st.ReloadCounter > 0 {
		st.ReloadCounter = maxf(0, st.ReloadCounter-dt*60/200)
	}
	target, ok := w.massDriverTargetLocked(pos, tile)
	if !ok || st.ReloadCounter > 0 {
		return
	}
	if totalBuildingItems(tile.Build) < 10 {
		return
	}
	targetTile := &w.model.Tiles[target]
	if targetTile.Build == nil || totalBuildingItems(targetTile.Build) > 230 {
		return
	}
	if w.massDriverIncomingShotsLocked(target) > 0 {
		return
	}
	items := w.massDriverTakePayloadLocked(tile, 120)
	if len(items) == 0 {
		return
	}
	st.ReloadCounter = 1
	dx := float32(targetTile.X-tile.X) * 8
	dy := float32(targetTile.Y-tile.Y) * 8
	travelFrames := float32(math.Sqrt(float64(dx*dx+dy*dy)) / 5.5)
	if travelFrames < 1 {
		travelFrames = 1
	}
	w.massDriverShots = append(w.massDriverShots, massDriverShot{
		FromPos:      pos,
		ToPos:        target,
		TravelFrames: travelFrames,
		Transferred:  items,
	})
}

func (w *World) stepMassDriverShotsLocked(dt float32) {
	if len(w.massDriverShots) == 0 {
		return
	}
	kept := w.massDriverShots[:0]
	for _, shot := range w.massDriverShots {
		shot.AgeFrames += dt * 60
		if shot.AgeFrames < shot.TravelFrames {
			kept = append(kept, shot)
			continue
		}
		targetTile := (*Tile)(nil)
		if w.model != nil && shot.ToPos >= 0 && int(shot.ToPos) < len(w.model.Tiles) {
			targetTile = &w.model.Tiles[shot.ToPos]
		}
		if targetTile == nil || targetTile.Build == nil || w.blockNameByID(int16(targetTile.Block)) != "mass-driver" {
			continue
		}
		total := totalBuildingItems(targetTile.Build)
		for _, stack := range shot.Transferred {
			if stack.Amount <= 0 {
				continue
			}
			space := int32(240 - total)
			if space <= 0 {
				break
			}
			amount := stack.Amount
			if amount > space {
				amount = space
			}
			targetTile.Build.AddItem(stack.Item, amount)
			total += amount
		}
		w.massDriverStateLocked(shot.ToPos).ReloadCounter = 1
	}
	w.massDriverShots = kept
}

func (w *World) stepJunctions(dt float32) {
	const travelSec = float32(26.0 / 60.0)
	for pos, state := range w.junctionQueues {
		empty := true
		for dir := 0; dir < len(state); dir++ {
			queue := state[dir]
			if len(queue) == 0 {
				continue
			}
			empty = false
			for i := range queue {
				queue[i].AgeSec += dt
			}
			head := queue[0]
			if head.AgeSec >= travelSec {
				outPos, ok := w.forwardItemTargetPosLocked(pos, int8(head.FromDir))
				if ok && w.tryInsertItemLocked(pos, outPos, head.Item, 0) {
					queue = queue[1:]
				}
			}
			state[dir] = queue
			if len(queue) > 0 {
				empty = false
			}
		}
		if empty {
			delete(w.junctionQueues, pos)
			continue
		}
		w.junctionQueues[pos] = state
	}
}

func (w *World) tryInsertItemLocked(fromPos, toPos int32, item ItemID, depth int) bool {
	if depth > 8 || w.model == nil || toPos < 0 || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	fromTile := &w.model.Tiles[fromPos]
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil || toTile.Block == 0 || toTile.Team != fromTile.Team {
		return false
	}
	switch w.blockNameByID(int16(toTile.Block)) {
	case "conveyor", "titanium-conveyor", "armored-conveyor":
		return w.conveyorHandleItemLocked(fromPos, toPos, item)
	case "duct":
		return w.ductHandleItemLocked(fromPos, toPos, item, false)
	case "armored-duct":
		return w.ductHandleItemLocked(fromPos, toPos, item, true)
	case "duct-router":
		return w.ductRouterHandleItemLocked(fromPos, toPos, item, false)
	case "overflow-duct":
		return w.ductHandleItemLocked(fromPos, toPos, item, false)
	case "underflow-duct":
		return w.ductHandleItemLocked(fromPos, toPos, item, false)
	case "duct-bridge":
		if !w.ductBridgeAcceptsItemLocked(fromPos, toPos, item) {
			return false
		}
		toTile.Build.AddItem(item, 1)
		return true
	case "duct-unloader":
		return false
	case "bridge-conveyor", "phase-conveyor":
		if !w.bridgeAllowsInputLocked(fromPos, toPos) {
			return false
		}
		cap := w.itemCapacityAtLocked(toPos)
		if w.totalItemsAtLocked(toPos) >= cap {
			return false
		}
		return w.addItemAtLocked(toPos, item, 1)
	case "mass-driver":
		cap := w.itemCapacityAtLocked(toPos)
		if cap <= 0 || w.totalItemsAtLocked(toPos) >= cap {
			return false
		}
		if _, ok := w.massDriverTargetLocked(toPos, toTile); !ok {
			return false
		}
		return w.addItemAtLocked(toPos, item, 1)
	case "router", "distributor":
		st := w.routerStateLocked(toPos, toTile)
		if st.HasItem || totalBuildingItems(toTile.Build) >= 1 {
			return false
		}
		toTile.Build.AddItem(item, 1)
		st.LastItem = item
		st.HasItem = true
		st.Time = 0
		st.LastInput = fromPos
		w.routerInputPos[toPos] = fromPos
		return true
	case "plastanium-conveyor", "surge-conveyor":
		return w.stackConveyorHandleItemLocked(fromPos, toPos, item)
	case "surge-router":
		return w.ductRouterHandleItemLocked(fromPos, toPos, item, true)
	case "junction":
		dir, ok := flowDir(fromTile.X, fromTile.Y, toTile.X, toTile.Y)
		if !ok {
			return false
		}
		outPos, ok := w.forwardItemTargetPosLocked(toPos, int8(dir))
		if !ok {
			return false
		}
		outTile := &w.model.Tiles[outPos]
		if outTile.Build == nil || outTile.Block == 0 || outTile.Team != toTile.Team {
			return false
		}
		state := w.junctionQueues[toPos]
		if len(state[dir]) >= 6 {
			return false
		}
		state[dir] = append(state[dir], junctionQueuedItem{Item: item, FromDir: dir})
		w.junctionQueues[toPos] = state
		return true
	case "sorter", "inverted-sorter":
		target, ok := w.sorterTargetLocked(fromPos, toPos, item, w.blockNameByID(int16(toTile.Block)) == "inverted-sorter", true)
		if !ok {
			return false
		}
		return w.tryInsertItemLocked(toPos, target, item, depth+1)
	case "overflow-gate":
		target, ok := w.overflowTargetLocked(fromPos, toPos, item, false, true)
		if !ok {
			return false
		}
		return w.tryInsertItemLocked(toPos, target, item, depth+1)
	case "underflow-gate":
		target, ok := w.overflowTargetLocked(fromPos, toPos, item, true, true)
		if !ok {
			return false
		}
		return w.tryInsertItemLocked(toPos, target, item, depth+1)
	case "thorium-reactor":
		cap := w.itemCapacityAtLocked(toPos)
		if w.itemAmountAtLocked(toPos, item) >= cap {
			return false
		}
		return w.addItemAtLocked(toPos, item, 1)
	default:
		cap := w.itemCapacityAtLocked(toPos)
		if cap <= 0 || w.itemAmountAtLocked(toPos, item) >= cap {
			return false
		}
		return w.addItemAtLocked(toPos, item, 1)
	}
}

func (w *World) bridgeTargetLocked(pos int32, tile *Tile) (int32, bool) {
	target, ok := w.bridgeLinks[pos]
	if !ok || target < 0 || int(target) >= len(w.model.Tiles) {
		return 0, false
	}
	targetTile := &w.model.Tiles[target]
	name := w.blockNameByID(int16(tile.Block))
	if targetTile.Build == nil || targetTile.Team != tile.Team || w.blockNameByID(int16(targetTile.Block)) != name {
		return 0, false
	}
	return target, true
}

func firstBuildingLiquid(build *Building) (LiquidID, float32, bool) {
	if build == nil {
		return 0, 0, false
	}
	var (
		bestLiquid LiquidID
		bestAmount float32
		found      bool
	)
	for _, stack := range build.Liquids {
		if stack.Amount <= 0 {
			continue
		}
		if !found || stack.Amount > bestAmount {
			bestLiquid = stack.Liquid
			bestAmount = stack.Amount
			found = true
		}
	}
	return bestLiquid, bestAmount, found
}

func totalBuildingLiquids(build *Building) float32 {
	if build == nil {
		return 0
	}
	total := float32(0)
	for _, stack := range build.Liquids {
		if stack.Amount > 0 {
			total += stack.Amount
		}
	}
	return total
}

func (w *World) liquidCanStoreLocked(tile *Tile, liquid LiquidID) bool {
	if tile == nil || tile.Build == nil {
		return false
	}
	total := totalBuildingLiquids(tile.Build)
	if total >= w.liquidCapacityForBlockLocked(tile) {
		return false
	}
	current, amount, ok := firstBuildingLiquid(tile.Build)
	return !ok || current == liquid || amount < 0.2
}

func (w *World) conduitAcceptsLiquidLocked(fromPos, toPos int32, liquid LiquidID, armored bool) bool {
	if w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil || !w.liquidCanStoreLocked(toTile, liquid) {
		return false
	}
	sourceSide, ok := w.flowDirBetweenLocked(fromPos, toPos)
	if !ok {
		return false
	}
	if sourceSide == byte((tileRotationNorm(toTile.Rotation)+2)%4) {
		return false
	}
	if !armored {
		return true
	}
	fromTile := &w.model.Tiles[fromPos]
	fromName := w.blockNameByID(int16(fromTile.Block))
	if fromName == "conduit" || fromName == "pulse-conduit" || fromName == "plated-conduit" || fromName == "reinforced-conduit" ||
		fromName == "reinforced-bridge-conduit" || fromName == "liquid-junction" || fromName == "reinforced-liquid-junction" {
		return true
	}
	return sourceSide == byte(tileRotationNorm(toTile.Rotation))
}

func (w *World) canAcceptLiquidLocked(fromPos, toPos int32, liquid LiquidID, depth int) bool {
	if depth > 8 || w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	fromTile := &w.model.Tiles[fromPos]
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil || toTile.Block == 0 || toTile.Team != fromTile.Team {
		return false
	}
	name := w.blockNameByID(int16(toTile.Block))
	switch name {
	case "conduit", "pulse-conduit":
		return w.conduitAcceptsLiquidLocked(fromPos, toPos, liquid, false)
	case "plated-conduit":
		return w.conduitAcceptsLiquidLocked(fromPos, toPos, liquid, true)
	case "reinforced-conduit":
		return w.conduitAcceptsLiquidLocked(fromPos, toPos, liquid, false)
	case "liquid-router", "liquid-container", "liquid-tank", "reinforced-liquid-router", "reinforced-liquid-container", "reinforced-liquid-tank", "thorium-reactor":
		return w.liquidCanStoreLocked(toTile, liquid)
	case "bridge-conduit", "phase-conduit":
		return w.bridgeAllowsInputLocked(fromPos, toPos) && w.liquidCanStoreLocked(toTile, liquid)
	case "reinforced-bridge-conduit":
		if _, ok := w.directionBridgeTargetLocked(toPos, toTile, "reinforced-bridge-conduit", 4); !ok {
			return false
		}
		rel, ok := w.relativeToEdgeLocked(fromPos, toPos)
		return ok && rel != byte(tileRotationNorm(toTile.Rotation)) && w.liquidCanStoreLocked(toTile, liquid)
	case "liquid-junction", "reinforced-liquid-junction":
		_, ok := w.liquidJunctionDestinationLocked(fromPos, toPos, liquid, depth+1)
		return ok
	default:
		cap := w.liquidCapacityForBlockLocked(toTile)
		return cap > 0 && w.liquidCanStoreLocked(toTile, liquid)
	}
}

func (w *World) liquidJunctionDestinationLocked(fromPos, junctionPos int32, liquid LiquidID, depth int) (int32, bool) {
	if depth > 8 || w.model == nil || junctionPos < 0 || int(junctionPos) >= len(w.model.Tiles) {
		return 0, false
	}
	sourceDir, ok := w.relativeToEdgeLocked(fromPos, junctionPos)
	if !ok {
		return 0, false
	}
	outDir := byte((int(sourceDir) + 2) % 4)
	nextPos, ok := w.forwardPosLocked(junctionPos, int8(outDir))
	if !ok {
		return 0, false
	}
	nextTile := &w.model.Tiles[nextPos]
	if nextTile.Build == nil || nextTile.Block == 0 {
		return 0, false
	}
	name := w.blockNameByID(int16(nextTile.Block))
	if name == "liquid-junction" || name == "reinforced-liquid-junction" {
		return w.liquidJunctionDestinationLocked(junctionPos, nextPos, liquid, depth+1)
	}
	if !w.canAcceptLiquidLocked(junctionPos, nextPos, liquid, depth+1) {
		return 0, false
	}
	return nextPos, true
}

func (w *World) tryMoveLiquidLocked(fromPos, toPos int32, liquid LiquidID, amount float32, depth int) float32 {
	if amount <= 0 || !w.canAcceptLiquidLocked(fromPos, toPos, liquid, depth) || w.model == nil {
		return 0
	}
	toTile := &w.model.Tiles[toPos]
	name := w.blockNameByID(int16(toTile.Block))
	if name == "liquid-junction" || name == "reinforced-liquid-junction" {
		target, ok := w.liquidJunctionDestinationLocked(fromPos, toPos, liquid, depth+1)
		if !ok {
			return 0
		}
		return w.tryMoveLiquidLocked(toPos, target, liquid, amount, depth+1)
	}
	cap := w.liquidCapacityForBlockLocked(toTile)
	if cap <= 0 {
		return 0
	}
	current := totalBuildingLiquids(toTile.Build)
	space := cap - current
	if space <= 0 {
		return 0
	}
	if amount > space {
		amount = space
	}
	if amount <= 0 {
		return 0
	}
	toTile.Build.AddLiquid(liquid, amount)
	return amount
}

func (w *World) dumpLiquidLocked(pos int32, tile *Tile, liquid LiquidID, amount float32) bool {
	if tile == nil || tile.Build == nil || amount <= 0 || w.model == nil {
		return false
	}
	neighbors := w.dumpProximityLocked(pos)
	if len(neighbors) == 0 {
		return false
	}
	start := 0
	if idx, ok := w.blockDumpIndex[pos]; ok && len(neighbors) > 0 {
		start = ((idx % len(neighbors)) + len(neighbors)) % len(neighbors)
	}
	for i := 0; i < len(neighbors); i++ {
		index := (start + i) % len(neighbors)
		target := neighbors[index]
		moved := w.tryMoveLiquidLocked(pos, target, liquid, amount, 0)
		w.advanceDumpIndexLocked(pos, index+1, len(neighbors))
		if moved > 0 {
			_ = tile.Build.RemoveLiquid(liquid, moved)
			return true
		}
	}
	return false
}

func (w *World) bridgeAllowsInputLocked(fromPos, bridgePos int32) bool {
	if w.model == nil || fromPos < 0 || bridgePos < 0 || int(fromPos) >= len(w.model.Tiles) || int(bridgePos) >= len(w.model.Tiles) {
		return false
	}
	bridgeTile := &w.model.Tiles[bridgePos]
	bridgeName := w.blockNameByID(int16(bridgeTile.Block))
	if !isItemBridgeBlock(bridgeName) {
		return false
	}
	if w.bridgeLinks[fromPos] == bridgePos && w.blockNameByID(int16(w.model.Tiles[fromPos].Block)) == bridgeName {
		return true
	}
	target, ok := w.bridgeLinks[bridgePos]
	if !ok || target < 0 || int(target) >= len(w.model.Tiles) {
		return false
	}
	targetTile := &w.model.Tiles[target]
	if targetTile.Build == nil || targetTile.Team != bridgeTile.Team || w.blockNameByID(int16(targetTile.Block)) != bridgeName {
		return false
	}
	linkSide, ok := axisDir(targetTile.X, targetTile.Y, bridgeTile.X, bridgeTile.Y)
	if !ok {
		return false
	}
	fromTile := &w.model.Tiles[fromPos]
	sourceSide, ok := relativeDir(fromTile.X, fromTile.Y, bridgeTile.X, bridgeTile.Y)
	if !ok {
		return false
	}
	return sourceSide != linkSide
}

func (w *World) bridgeHasIncomingFromSideLocked(bridgePos int32, side byte) bool {
	if w.model == nil || bridgePos < 0 || int(bridgePos) >= len(w.model.Tiles) {
		return false
	}
	bridgeTile := &w.model.Tiles[bridgePos]
	bridgeName := w.blockNameByID(int16(bridgeTile.Block))
	for otherPos, target := range w.bridgeLinks {
		if target != bridgePos || otherPos < 0 || int(otherPos) >= len(w.model.Tiles) {
			continue
		}
		otherTile := &w.model.Tiles[otherPos]
		if otherTile.Build == nil || otherTile.Team != bridgeTile.Team || w.blockNameByID(int16(otherTile.Block)) != bridgeName {
			continue
		}
		incomingSide, ok := axisDir(otherTile.X, otherTile.Y, bridgeTile.X, bridgeTile.Y)
		if ok && incomingSide == side {
			return true
		}
	}
	return false
}

func (w *World) massDriverStateLocked(pos int32) *massDriverRuntimeState {
	if st, ok := w.massDriverStates[pos]; ok && st != nil {
		return st
	}
	st := &massDriverRuntimeState{}
	w.massDriverStates[pos] = st
	return st
}

func (w *World) massDriverTargetLocked(pos int32, tile *Tile) (int32, bool) {
	target, ok := w.massDriverLinks[pos]
	if !ok || target < 0 || int(target) >= len(w.model.Tiles) {
		return 0, false
	}
	targetTile := &w.model.Tiles[target]
	if tile == nil || targetTile.Build == nil || targetTile.Team != tile.Team || w.blockNameByID(int16(targetTile.Block)) != "mass-driver" {
		return 0, false
	}
	dx := float32(targetTile.X - tile.X)
	dy := float32(targetTile.Y - tile.Y)
	if dx*dx+dy*dy > 55*55 {
		return 0, false
	}
	return target, true
}

func (w *World) massDriverIncomingShotsLocked(targetPos int32) int {
	count := 0
	for _, shot := range w.massDriverShots {
		if shot.ToPos == targetPos {
			count++
		}
	}
	return count
}

func (w *World) massDriverTakePayloadLocked(tile *Tile, limit int32) []ItemStack {
	if tile == nil || tile.Build == nil || limit <= 0 {
		return nil
	}
	total := int32(0)
	out := make([]ItemStack, 0, len(tile.Build.Items))
	for _, stack := range append([]ItemStack(nil), tile.Build.Items...) {
		if stack.Amount <= 0 || total >= limit {
			continue
		}
		amount := stack.Amount
		if amount > limit-total {
			amount = limit - total
		}
		if amount <= 0 {
			continue
		}
		if tile.Build.RemoveItem(stack.Item, amount) {
			out = append(out, ItemStack{Item: stack.Item, Amount: amount})
			total += amount
		}
	}
	return out
}

func isStorageLikeBlock(name string) bool {
	switch name {
	case "core-shard", "core-foundation", "core-nucleus", "core-bastion", "core-citadel", "core-acropolis", "container", "vault", "reinforced-container", "reinforced-vault":
		return true
	default:
		return false
	}
}

func isCoreBlockName(name string) bool {
	return strings.HasPrefix(name, "core-")
}

func isCoreMergeStorageBlock(name string) bool {
	switch name {
	case "container", "vault", "reinforced-container", "reinforced-vault":
		return true
	default:
		return false
	}
}

func normalizeItemStackMap(items map[ItemID]int32, maxPerItem int32) []ItemStack {
	if len(items) == 0 {
		return nil
	}
	ids := make([]int, 0, len(items))
	for item, amount := range items {
		if amount <= 0 {
			continue
		}
		ids = append(ids, int(item))
	}
	sort.Ints(ids)
	out := make([]ItemStack, 0, len(ids))
	for _, rawID := range ids {
		item := ItemID(rawID)
		amount := items[item]
		if amount <= 0 {
			continue
		}
		if maxPerItem > 0 && amount > maxPerItem {
			amount = maxPerItem
		}
		out = append(out, ItemStack{Item: item, Amount: amount})
	}
	return out
}

func (w *World) refreshCoreStorageLinksLocked() {
	w.storageLinkedCore = map[int32]int32{}
	w.teamPrimaryCore = map[TeamID]int32{}
	w.coreStorageCapacity = map[int32]int32{}
	if w.model == nil {
		return
	}

	teamCores := make(map[TeamID][]int32)
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Team == 0 || tile.Build == nil || tile.Block == 0 {
			continue
		}
		if !isCoreBlockName(w.blockNameByID(int16(tile.Block))) {
			continue
		}
		teamCores[tile.Team] = append(teamCores[tile.Team], pos)
	}

	for team, cores := range teamCores {
		if len(cores) == 0 {
			continue
		}
		sort.Slice(cores, func(i, j int) bool { return cores[i] < cores[j] })
		primary := cores[0]
		w.teamPrimaryCore[team] = primary

		totalCapacity := int32(0)
		mergedItems := make(map[ItemID]int32)
		ownedStorages := make(map[int32]struct{})

		for _, corePos := range cores {
			coreTile := &w.model.Tiles[corePos]
			totalCapacity += w.itemCapacityForBlockLocked(coreTile)
			for _, stack := range coreTile.Build.Items {
				if stack.Amount > 0 {
					mergedItems[stack.Item] += stack.Amount
				}
			}
		}

		for _, corePos := range cores {
			for _, otherPos := range w.dumpProximityLocked(corePos) {
				if otherPos < 0 || int(otherPos) >= len(w.model.Tiles) || otherPos == corePos {
					continue
				}
				other := &w.model.Tiles[otherPos]
				if other.Build == nil || other.Block == 0 || other.Team != team {
					continue
				}
				if !isCoreMergeStorageBlock(w.blockNameByID(int16(other.Block))) {
					continue
				}
				if _, exists := ownedStorages[otherPos]; exists {
					continue
				}
				ownedStorages[otherPos] = struct{}{}
				w.storageLinkedCore[otherPos] = corePos
				totalCapacity += w.itemCapacityForBlockLocked(other)
				for _, stack := range other.Build.Items {
					if stack.Amount > 0 {
						mergedItems[stack.Item] += stack.Amount
					}
				}
			}
		}

		normalized := normalizeItemStackMap(mergedItems, totalCapacity)
		if primary >= 0 && int(primary) < len(w.model.Tiles) {
			if build := w.model.Tiles[primary].Build; build != nil {
				build.Items = normalized
			}
		}
		for _, corePos := range cores {
			w.coreStorageCapacity[corePos] = totalCapacity
			if corePos == primary {
				continue
			}
			if build := w.model.Tiles[corePos].Build; build != nil {
				build.Items = nil
			}
		}
		for storagePos := range ownedStorages {
			if build := w.model.Tiles[storagePos].Build; build != nil {
				build.Items = nil
			}
		}
	}
}

func (w *World) itemInventoryPosLocked(pos int32) (int32, bool) {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0, false
	}
	cur := pos
	for i := 0; i < 3; i++ {
		tile := &w.model.Tiles[cur]
		name := w.blockNameByID(int16(tile.Block))
		if linked, ok := w.storageLinkedCore[cur]; ok && linked != cur {
			cur = linked
			continue
		}
		if isCoreBlockName(name) {
			if primary, ok := w.teamPrimaryCore[tile.Team]; ok && primary != cur {
				cur = primary
				continue
			}
		}
		return cur, true
	}
	return cur, true
}

func (w *World) sharedCoreInventoryLocked(pos int32) (TeamID, int32, *Building, bool) {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0, 0, nil, false
	}
	tile := &w.model.Tiles[pos]
	name := w.blockNameByID(int16(tile.Block))
	if !isCoreBlockName(name) {
		if _, ok := w.storageLinkedCore[pos]; !ok {
			return 0, 0, nil, false
		}
	}
	proxyPos, ok := w.itemInventoryPosLocked(pos)
	if !ok || proxyPos < 0 || int(proxyPos) >= len(w.model.Tiles) {
		return 0, 0, nil, false
	}
	proxyTile := &w.model.Tiles[proxyPos]
	if proxyTile.Build == nil || proxyTile.Block == 0 {
		return 0, 0, nil, false
	}
	capacity := w.coreStorageCapacity[proxyPos]
	if capacity <= 0 {
		capacity = w.itemCapacityForBlockLocked(proxyTile)
	}
	return proxyTile.Team, proxyPos, proxyTile.Build, true
}

func (w *World) itemCapacityAtLocked(pos int32) int32 {
	if team, proxyPos, _, ok := w.sharedCoreInventoryLocked(pos); ok && team != 0 {
		if cap := w.coreStorageCapacity[proxyPos]; cap > 0 {
			return cap
		}
	}
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0
	}
	return w.itemCapacityForBlockLocked(&w.model.Tiles[pos])
}

func (w *World) itemAmountAtLocked(pos int32, item ItemID) int32 {
	if _, _, build, ok := w.sharedCoreInventoryLocked(pos); ok {
		return build.ItemAmount(item)
	}
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil {
		return 0
	}
	return tile.Build.ItemAmount(item)
}

func (w *World) totalItemsAtLocked(pos int32) int32 {
	if _, _, build, ok := w.sharedCoreInventoryLocked(pos); ok {
		return totalBuildingItems(build)
	}
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil {
		return 0
	}
	return totalBuildingItems(tile.Build)
}

func (w *World) addItemAtLocked(pos int32, item ItemID, amount int32) bool {
	if amount <= 0 {
		return false
	}
	if team, _, build, ok := w.sharedCoreInventoryLocked(pos); ok {
		build.AddItem(item, amount)
		w.emitTeamCoreItemsLocked(team, []ItemID{item})
		return true
	}
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil {
		return false
	}
	tile.Build.AddItem(item, amount)
	return true
}

func (w *World) removeItemAtLocked(pos int32, item ItemID, amount int32) bool {
	if amount <= 0 {
		return false
	}
	if team, _, build, ok := w.sharedCoreInventoryLocked(pos); ok {
		if !build.RemoveItem(item, amount) {
			return false
		}
		w.emitTeamCoreItemsLocked(team, []ItemID{item})
		return true
	}
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil {
		return false
	}
	return tile.Build.RemoveItem(item, amount)
}

func (w *World) unloaderTargetItemLocked(pos int32, neighbors []int32) (ItemID, bool) {
	if item, ok := w.unloaderCfg[pos]; ok {
		if _, _, found := w.unloaderTransferPairLocked(pos, neighbors, item); found {
			return item, true
		}
		return 0, false
	}
	start := 0
	if idx, ok := w.blockDumpIndex[pos]; ok {
		start = idx
	}
	for i := 0; i < 256; i++ {
		item := ItemID((start + i) % 256)
		if _, _, found := w.unloaderTransferPairLocked(pos, neighbors, item); found {
			w.blockDumpIndex[pos] = int(item)
			return item, true
		}
	}
	return 0, false
}

func (w *World) unloaderTransferPairLocked(pos int32, neighbors []int32, item ItemID) (int32, int32, bool) {
	var (
		fromPos    int32 = -1
		toPos      int32 = -1
		bestFromLF       = float32(-1)
		bestToLF         = float32(2)
	)
	for _, otherPos := range neighbors {
		if otherPos < 0 || int(otherPos) >= len(w.model.Tiles) || otherPos == pos {
			continue
		}
		other := &w.model.Tiles[otherPos]
		if other.Build == nil || other.Team != w.model.Tiles[pos].Team {
			continue
		}
		otherName := w.blockNameByID(int16(other.Block))
		otherAmount := w.itemAmountAtLocked(otherPos, item)
		if otherAmount > 0 {
			cap := w.itemCapacityAtLocked(otherPos)
			load := float32(1)
			if cap > 0 {
				load = float32(otherAmount) / float32(cap)
			}
			if isStorageLikeBlock(otherName) {
				load += 1
			}
			if load > bestFromLF {
				bestFromLF = load
				fromPos = otherPos
			}
		}
		if !isStorageLikeBlock(otherName) && w.canAcceptItemLocked(pos, otherPos, item, 0) {
			cap := w.itemCapacityAtLocked(otherPos)
			load := float32(0)
			if cap > 0 {
				load = float32(w.itemAmountAtLocked(otherPos, item)) / float32(cap)
			}
			if load < bestToLF {
				bestToLF = load
				toPos = otherPos
			}
		}
	}
	return fromPos, toPos, fromPos >= 0 && toPos >= 0 && fromPos != toPos
}

func (w *World) dumpProximityLocked(pos int32) []int32 {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return nil
	}
	tile := &w.model.Tiles[pos]
	offsets := blockEdgeOffsets(w.blockSizeForTileLocked(tile))
	out := make([]int32, 0, len(offsets))
	seen := make(map[int32]struct{}, len(offsets))
	for _, off := range offsets {
		otherPos, ok := w.buildingOccupyingCellLocked(tile.X+off[0], tile.Y+off[1])
		if !ok || otherPos == pos {
			continue
		}
		other := &w.model.Tiles[otherPos]
		if other.Build == nil || other.Block == 0 || other.Team != tile.Team {
			continue
		}
		if _, exists := seen[otherPos]; exists {
			continue
		}
		seen[otherPos] = struct{}{}
		out = append(out, otherPos)
	}
	return out
}

func (w *World) advanceDumpIndexLocked(pos int32, next int, count int) {
	if count <= 0 {
		delete(w.blockDumpIndex, pos)
		return
	}
	w.blockDumpIndex[pos] = ((next % count) + count) % count
}

func (w *World) dumpSingleItemLocked(pos int32, tile *Tile, specific *ItemID, canDump func(int32, ItemID) bool) bool {
	if tile == nil || tile.Build == nil || w.model == nil || len(tile.Build.Items) == 0 {
		return false
	}
	neighbors := w.dumpProximityLocked(pos)
	if len(neighbors) == 0 {
		return false
	}
	start := 0
	if idx, ok := w.blockDumpIndex[pos]; ok {
		start = ((idx % len(neighbors)) + len(neighbors)) % len(neighbors)
	}
	for i := 0; i < len(neighbors); i++ {
		index := (start + i) % len(neighbors)
		target := neighbors[index]
		tryDump := func(item ItemID) bool {
			if canDump != nil && !canDump(target, item) {
				return false
			}
			if !w.tryInsertItemLocked(pos, target, item, 0) {
				return false
			}
			if tile.Build.RemoveItem(item, 1) {
				w.advanceDumpIndexLocked(pos, index+1, len(neighbors))
				return true
			}
			return false
		}
		if specific != nil {
			if tile.Build.ItemAmount(*specific) > 0 && tryDump(*specific) {
				return true
			}
		} else {
			for _, stack := range tile.Build.Items {
				if stack.Amount <= 0 {
					continue
				}
				if tryDump(stack.Item) {
					return true
				}
			}
		}
		w.advanceDumpIndexLocked(pos, index+1, len(neighbors))
	}
	return false
}

func (w *World) bridgeDumpTargetLocked(pos int32, tile *Tile, item ItemID) (int32, bool) {
	neighbors := w.dumpProximityLocked(pos)
	if len(neighbors) == 0 {
		return 0, false
	}
	start := 0
	if idx, ok := w.blockDumpIndex[pos]; ok && len(neighbors) > 0 {
		start = ((idx % len(neighbors)) + len(neighbors)) % len(neighbors)
	}
	for i := 0; i < len(neighbors); i++ {
		index := (start + i) % len(neighbors)
		target := neighbors[index]
		other := &w.model.Tiles[target]
		side, ok := flowDir(tile.X, tile.Y, other.X, other.Y)
		if !ok {
			w.blockDumpIndex[pos] = (index + 1) % len(neighbors)
			continue
		}
		if !w.bridgeHasIncomingFromSideLocked(pos, side) && w.canAcceptItemLocked(pos, target, item, 0) {
			w.blockDumpIndex[pos] = (index + 1) % len(neighbors)
			return target, true
		}
		w.blockDumpIndex[pos] = (index + 1) % len(neighbors)
	}
	return 0, false
}

func (w *World) directionBridgeTargetLocked(pos int32, tile *Tile, want string, maxRange int) (int32, bool) {
	if w.model == nil || tile == nil {
		return 0, false
	}
	dx, dy := dirDelta(tile.Rotation)
	for i := 1; i <= maxRange; i++ {
		nx, ny := tile.X+dx*i, tile.Y+dy*i
		if !w.model.InBounds(nx, ny) {
			break
		}
		target := int32(ny*w.model.Width + nx)
		other := &w.model.Tiles[target]
		if other.Build == nil || other.Team != tile.Team {
			continue
		}
		if w.blockNameByID(int16(other.Block)) == want {
			return target, true
		}
	}
	return 0, false
}

func (w *World) dumpTargetLocked(pos int32, tile *Tile, item ItemID) (int32, bool) {
	neighbors := w.dumpProximityLocked(pos)
	if len(neighbors) == 0 {
		return 0, false
	}
	start := 0
	if idx, ok := w.blockDumpIndex[pos]; ok && len(neighbors) > 0 {
		start = ((idx % len(neighbors)) + len(neighbors)) % len(neighbors)
	}
	for i := 0; i < len(neighbors); i++ {
		index := (start + i) % len(neighbors)
		target := neighbors[index]
		if w.canAcceptItemLocked(pos, target, item, 0) {
			w.blockDumpIndex[pos] = (index + 1) % len(neighbors)
			return target, true
		}
		w.blockDumpIndex[pos] = (index + 1) % len(neighbors)
	}
	return 0, false
}

func (w *World) ductRouterTargetLocked(pos int32, tile *Tile, item ItemID) (int32, bool) {
	neighbors := w.dumpProximityLocked(pos)
	if len(neighbors) == 0 || tile == nil {
		return 0, false
	}
	filter, hasFilter := w.sorterCfg[pos]
	start := 0
	if idx, ok := w.blockDumpIndex[pos]; ok && len(neighbors) > 0 {
		start = ((idx % len(neighbors)) + len(neighbors)) % len(neighbors)
	}
	for i := 0; i < len(neighbors); i++ {
		index := (start + i) % len(neighbors)
		target := neighbors[index]
		other := &w.model.Tiles[target]
		rel, ok := relativeDir(other.X, other.Y, tile.X, tile.Y)
		if !ok || rel == byte((int(tile.Rotation)+2)%4) {
			continue
		}
		if hasFilter && ((item == filter) != (rel == byte(tile.Rotation))) {
			continue
		}
		if w.canAcceptItemLocked(pos, target, item, 0) {
			w.advanceDumpIndexLocked(pos, index+1, len(neighbors))
			return target, true
		}
	}
	return 0, false
}

func (w *World) routerTargetLocked(pos int32, tile *Tile, item ItemID, set bool) (int32, bool) {
	neighbors := w.dumpProximityLocked(pos)
	if len(neighbors) == 0 {
		return 0, false
	}
	inPos := w.routerInputPos[pos]
	if st, ok := w.routerStates[pos]; ok && st != nil && st.LastInput >= 0 {
		inPos = st.LastInput
	}
	start := int(w.routerRotation[pos] % byte(len(neighbors)))
	skipInput := false
	if inPos >= 0 && int(inPos) < len(w.model.Tiles) {
		skipInput = w.blockNameByID(int16(w.model.Tiles[inPos].Block)) == "overflow-gate"
	}
	for i := 0; i < len(neighbors); i++ {
		outPos := neighbors[(start+i)%len(neighbors)]
		if set {
			w.routerRotation[pos] = byte((int(w.routerRotation[pos]) + 1) % len(neighbors))
		}
		if skipInput && outPos == inPos {
			continue
		}
		if w.canAcceptItemLocked(pos, outPos, item, 0) {
			return outPos, true
		}
	}
	return 0, false
}

func (w *World) overflowDuctTargetLocked(pos int32, tile *Tile, item ItemID, invert bool) (int32, bool) {
	if w.model == nil || tile == nil {
		return 0, false
	}
	tryDir := func(dir byte) (int32, bool) {
		target, ok := w.forwardItemTargetPosLocked(pos, int8(dir))
		if !ok {
			return 0, false
		}
		if !w.canAcceptItemLocked(pos, target, item, 0) {
			return 0, false
		}
		return target, true
	}
	leftDir := byte((int(tile.Rotation) + 3) % 4)
	rightDir := byte((int(tile.Rotation) + 1) % 4)
	if invert {
		left, lok := tryDir(leftDir)
		right, rok := tryDir(rightDir)
		if lok && !rok {
			return left, true
		}
		if rok && !lok {
			return right, true
		}
		if lok && rok {
			if w.blockDumpIndex[pos]%2 == 0 {
				w.blockDumpIndex[pos] = 1
				return left, true
			}
			w.blockDumpIndex[pos] = 0
			return right, true
		}
		return 0, false
	}
	if front, ok := tryDir(byte(tile.Rotation)); ok {
		return front, true
	}
	left, lok := tryDir(leftDir)
	right, rok := tryDir(rightDir)
	if lok && !rok {
		return left, true
	}
	if rok && !lok {
		return right, true
	}
	if lok && rok {
		if w.blockDumpIndex[pos]%2 == 0 {
			w.blockDumpIndex[pos] = 1
			return left, true
		}
		w.blockDumpIndex[pos] = 0
		return right, true
	}
	return 0, false
}

func (w *World) sorterTargetLocked(fromPos, sorterPos int32, item ItemID, invert bool, flip bool) (int32, bool) {
	fromTile := &w.model.Tiles[fromPos]
	sorterTile := &w.model.Tiles[sorterPos]
	sourceSide, ok := relativeDir(fromTile.X, fromTile.Y, sorterTile.X, sorterTile.Y)
	if !ok {
		return 0, false
	}
	dir := oppositeDir(sourceSide)
	filter, hasFilter := w.sorterCfg[sorterPos]
	match := hasFilter && filter == item
	fromInst := isInstantTransferBlock(w.blockNameByID(int16(fromTile.Block)))
	if match != invert {
		out, ok := w.forwardItemTargetPosLocked(sorterPos, int8(dir))
		if ok && (!fromInst || !isInstantTransferBlock(w.blockNameByID(int16(w.model.Tiles[out].Block)))) && w.canAcceptItemLocked(sorterPos, out, item, 1) {
			return out, true
		}
		return 0, false
	}
	leftDir := byte((int(dir) + 3) % 4)
	rightDir := byte((int(dir) + 1) % 4)
	left, lok := w.forwardItemTargetPosLocked(sorterPos, int8(leftDir))
	right, rok := w.forwardItemTargetPosLocked(sorterPos, int8(rightDir))
	canLeft := lok && (!fromInst || !isInstantTransferBlock(w.blockNameByID(int16(w.model.Tiles[left].Block)))) && w.canAcceptItemLocked(sorterPos, left, item, 1)
	canRight := rok && (!fromInst || !isInstantTransferBlock(w.blockNameByID(int16(w.model.Tiles[right].Block)))) && w.canAcceptItemLocked(sorterPos, right, item, 1)
	if canLeft && !canRight {
		return left, true
	}
	if canRight && !canLeft {
		return right, true
	}
	if canLeft && canRight {
		bit := byte(1 << dir)
		useLeft := (w.routerRotation[sorterPos] & bit) == 0
		if flip {
			w.routerRotation[sorterPos] ^= bit
		}
		if useLeft {
			return left, true
		}
		return right, true
	}
	return 0, false
}

func flowDir(fromX, fromY, toX, toY int) (byte, bool) {
	side, ok := relativeDir(fromX, fromY, toX, toY)
	if !ok {
		return 0, false
	}
	return oppositeDir(side), true
}

func oppositeDir(dir byte) byte {
	return byte((int(dir) + 2) % 4)
}

func (w *World) overflowTargetLocked(fromPos, gatePos int32, item ItemID, invert bool, flip bool) (int32, bool) {
	fromTile := &w.model.Tiles[fromPos]
	fromDir, ok := w.relativeToEdgeLocked(fromPos, gatePos)
	if !ok {
		return 0, false
	}
	fromInst := isInstantTransferBlock(w.blockNameByID(int16(fromTile.Block)))
	forwardDir := byte((int(fromDir) + 2) % 4)
	forward, fok := w.forwardItemTargetPosLocked(gatePos, int8(forwardDir))
	canForward := fok && (!fromInst || !isInstantTransferBlock(w.blockNameByID(int16(w.model.Tiles[forward].Block)))) && w.canAcceptItemLocked(gatePos, forward, item, 1)
	if !canForward || invert {
		leftDir := byte((int(fromDir) + 3) % 4)
		rightDir := byte((int(fromDir) + 1) % 4)
		left, lok := w.forwardItemTargetPosLocked(gatePos, int8(leftDir))
		right, rok := w.forwardItemTargetPosLocked(gatePos, int8(rightDir))
		canLeft := lok && (!fromInst || !isInstantTransferBlock(w.blockNameByID(int16(w.model.Tiles[left].Block)))) && w.canAcceptItemLocked(gatePos, left, item, 1)
		canRight := rok && (!fromInst || !isInstantTransferBlock(w.blockNameByID(int16(w.model.Tiles[right].Block)))) && w.canAcceptItemLocked(gatePos, right, item, 1)
		if !canLeft && !canRight {
			if invert && canForward {
				return forward, true
			}
			return 0, false
		}
		if canLeft && !canRight {
			return left, true
		}
		if canRight && !canLeft {
			return right, true
		}
		bit := byte(1 << fromDir)
		useLeft := (w.routerRotation[gatePos] & bit) == 0
		if flip {
			w.routerRotation[gatePos] ^= bit
		}
		if useLeft {
			return left, true
		}
		return right, true
	}
	return forward, true
}

func (w *World) canAcceptItemLocked(fromPos, toPos int32, item ItemID, depth int) bool {
	if depth > 8 || w.model == nil || toPos < 0 || int(toPos) >= len(w.model.Tiles) {
		return false
	}
	fromTile := &w.model.Tiles[fromPos]
	toTile := &w.model.Tiles[toPos]
	if toTile.Build == nil || toTile.Block == 0 || toTile.Team != fromTile.Team {
		return false
	}
	switch w.blockNameByID(int16(toTile.Block)) {
	case "conveyor", "titanium-conveyor", "armored-conveyor":
		return w.conveyorAcceptsItemLocked(fromPos, toPos)
	case "duct":
		return w.ductAcceptsItemLocked(fromPos, toPos, false)
	case "armored-duct":
		return w.ductAcceptsItemLocked(fromPos, toPos, true)
	case "duct-router", "surge-router":
		return w.ductRouterAcceptsItemLocked(fromPos, toPos, item)
	case "overflow-duct", "underflow-duct":
		return w.ductAcceptsItemLocked(fromPos, toPos, false)
	case "duct-bridge":
		return w.ductBridgeAcceptsItemLocked(fromPos, toPos, item)
	case "duct-unloader":
		return false
	case "bridge-conveyor", "phase-conveyor":
		return w.bridgeAllowsInputLocked(fromPos, toPos) && w.totalItemsAtLocked(toPos) < w.itemCapacityAtLocked(toPos)
	case "mass-driver":
		_, ok := w.massDriverTargetLocked(toPos, toTile)
		return ok && w.totalItemsAtLocked(toPos) < w.itemCapacityAtLocked(toPos)
	case "router", "distributor":
		st := w.routerStateLocked(toPos, toTile)
		return !st.HasItem && totalBuildingItems(toTile.Build) < 1
	case "plastanium-conveyor", "surge-conveyor":
		return w.stackConveyorAcceptsItemLocked(fromPos, toPos, item)
	case "junction":
		dir, ok := flowDir(fromTile.X, fromTile.Y, toTile.X, toTile.Y)
		if !ok {
			return false
		}
		outPos, ok := w.forwardItemTargetPosLocked(toPos, int8(dir))
		if !ok {
			return false
		}
		outTile := &w.model.Tiles[outPos]
		if outTile.Build == nil || outTile.Block == 0 || outTile.Team != toTile.Team {
			return false
		}
		state := w.junctionQueues[toPos]
		return len(state[dir]) < 6
	case "sorter", "inverted-sorter":
		target, ok := w.sorterTargetLocked(fromPos, toPos, item, w.blockNameByID(int16(toTile.Block)) == "inverted-sorter", false)
		return ok && w.canAcceptItemLocked(toPos, target, item, depth+1)
	case "overflow-gate":
		target, ok := w.overflowTargetLocked(fromPos, toPos, item, false, false)
		return ok && w.canAcceptItemLocked(toPos, target, item, depth+1)
	case "underflow-gate":
		target, ok := w.overflowTargetLocked(fromPos, toPos, item, true, false)
		return ok && w.canAcceptItemLocked(toPos, target, item, depth+1)
	case "thorium-reactor":
		return w.itemAmountAtLocked(toPos, item) < w.itemCapacityAtLocked(toPos)
	default:
		cap := w.itemCapacityAtLocked(toPos)
		return cap > 0 && w.itemAmountAtLocked(toPos, item) < cap
	}
}

func (w *World) forwardPosLocked(pos int32, rotation int8) (int32, bool) {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0, false
	}
	tile := &w.model.Tiles[pos]
	dx, dy := dirDelta(rotation)
	nx, ny := tile.X+dx, tile.Y+dy
	if !w.model.InBounds(nx, ny) {
		return 0, false
	}
	return int32(ny*w.model.Width + nx), true
}

func (w *World) forwardItemTargetPosLocked(pos int32, rotation int8) (int32, bool) {
	targetPos, ok := w.forwardPosLocked(pos, rotation)
	if !ok || w.model == nil || targetPos < 0 || int(targetPos) >= len(w.model.Tiles) {
		return 0, false
	}
	tile := &w.model.Tiles[targetPos]
	if occ, ok := w.buildingOccupyingCellLocked(tile.X, tile.Y); ok {
		return occ, true
	}
	return targetPos, true
}

func blockSizeByName(name string) int {
	switch name {
	case "distributor":
		return 2
	case "container", "reinforced-container":
		return 2
	case "core-shard", "vault", "reinforced-vault", "thorium-reactor", "mass-driver", "payload-conveyor", "reinforced-payload-conveyor", "payload-router", "reinforced-payload-router", "payload-mass-driver", "payload-loader", "payload-unloader":
		return 3
	case "core-foundation", "core-bastion", "impact-reactor":
		return 4
	case "core-nucleus", "core-citadel", "large-payload-mass-driver":
		return 5
	case "core-acropolis":
		return 6
	default:
		return 1
	}
}

func (w *World) blockSizeForTileLocked(tile *Tile) int {
	if tile == nil || tile.Block == 0 {
		return 1
	}
	return blockSizeByName(w.blockNameByID(int16(tile.Block)))
}

func blockFootprintRange(size int) (int, int) {
	if size <= 1 {
		return 0, 0
	}
	return -((size - 1) / 2), size / 2
}

func blockEdgeOffsets(size int) [][2]int {
	low, high := blockFootprintRange(size)
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
		if ai == aj {
			if out[i][0] == out[j][0] {
				return out[i][1] < out[j][1]
			}
			return out[i][0] < out[j][0]
		}
		return ai < aj
	})
	return out
}

func (w *World) rebuildBlockOccupancyLocked() {
	w.blockOccupancy = map[int32]int32{}
	w.activeTilePositions = w.activeTilePositions[:0]
	if w.model == nil {
		return
	}
	for i := range w.model.Tiles {
		tile := &w.model.Tiles[i]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		w.activeTilePositions = append(w.activeTilePositions, int32(i))
		w.setBuildingOccupancyLocked(int32(i), tile, true)
	}
	w.refreshCoreStorageLinksLocked()
}

func (w *World) rebuildActiveTilesLocked() {
	w.activeTilePositions = w.activeTilePositions[:0]
	if w.model == nil {
		return
	}
	for i := range w.model.Tiles {
		tile := &w.model.Tiles[i]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		w.activeTilePositions = append(w.activeTilePositions, int32(i))
	}
}

func (w *World) setBuildingOccupancyLocked(pos int32, tile *Tile, occupy bool) {
	if tile == nil {
		return
	}
	low, high := blockFootprintRange(w.blockSizeForTileLocked(tile))
	for y := tile.Y + low; y <= tile.Y+high; y++ {
		for x := tile.X + low; x <= tile.X+high; x++ {
			if w.model != nil && !w.model.InBounds(x, y) {
				continue
			}
			key := packTilePos(x, y)
			if occupy {
				w.blockOccupancy[key] = pos
				continue
			}
			if cur, ok := w.blockOccupancy[key]; ok && cur == pos {
				delete(w.blockOccupancy, key)
			}
		}
	}
}

func (w *World) buildingOccupyingCellLocked(x, y int) (int32, bool) {
	if w.model == nil || !w.model.InBounds(x, y) {
		return 0, false
	}
	if pos, ok := w.blockOccupancy[packTilePos(x, y)]; ok && pos >= 0 && int(pos) < len(w.model.Tiles) {
		tile := &w.model.Tiles[pos]
		if tile.Build != nil && tile.Block != 0 {
			return pos, true
		}
	}
	pos := int32(y*w.model.Width + x)
	tile := &w.model.Tiles[pos]
	return pos, tile.Build != nil && tile.Block != 0
}

func (w *World) facingEdgeLocked(fromPos, toPos int32) (int, int, bool) {
	if w.model == nil || fromPos < 0 || toPos < 0 || int(fromPos) >= len(w.model.Tiles) || int(toPos) >= len(w.model.Tiles) {
		return 0, 0, false
	}
	fromTile := &w.model.Tiles[fromPos]
	toTile := &w.model.Tiles[toPos]
	low, high := blockFootprintRange(w.blockSizeForTileLocked(fromTile))
	dx := toTile.X - fromTile.X
	dy := toTile.Y - fromTile.Y
	if dx < low {
		dx = low
	}
	if dx > high {
		dx = high
	}
	if dy < low {
		dy = low
	}
	if dy > high {
		dy = high
	}
	return fromTile.X + dx, fromTile.Y + dy, true
}

func (w *World) relativeToEdgeLocked(fromPos, toPos int32) (byte, bool) {
	fx, fy, ok := w.facingEdgeLocked(fromPos, toPos)
	if !ok {
		return 0, false
	}
	toTile := &w.model.Tiles[toPos]
	return relativeDir(fx, fy, toTile.X, toTile.Y)
}

func (w *World) flowDirBetweenLocked(fromPos, toPos int32) (byte, bool) {
	side, ok := w.relativeToEdgeLocked(fromPos, toPos)
	if !ok {
		return 0, false
	}
	return oppositeDir(side), true
}

func dirDelta(rotation int8) (int, int) {
	switch ((int(rotation) % 4) + 4) % 4 {
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

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func relativeDir(fromX, fromY, toX, toY int) (byte, bool) {
	switch {
	case fromX == toX+1 && fromY == toY:
		return 0, true
	case fromX == toX && fromY == toY+1:
		return 1, true
	case fromX == toX-1 && fromY == toY:
		return 2, true
	case fromX == toX && fromY == toY-1:
		return 3, true
	default:
		return 0, false
	}
}

func axisDir(fromX, fromY, toX, toY int) (byte, bool) {
	dx := fromX - toX
	dy := fromY - toY
	if dx == 0 && dy == 0 {
		return 0, false
	}
	if absInt(dx) >= absInt(dy) {
		if dx > 0 {
			return 0, true
		}
		return 2, true
	}
	if dy > 0 {
		return 1, true
	}
	return 3, true
}

func firstBuildingItem(b *Building) (ItemID, bool) {
	if b == nil {
		return 0, false
	}
	for _, stack := range b.Items {
		if stack.Amount > 0 {
			return stack.Item, true
		}
	}
	return 0, false
}

func totalBuildingItems(b *Building) int32 {
	if b == nil {
		return 0
	}
	total := int32(0)
	for _, stack := range b.Items {
		total += stack.Amount
	}
	return total
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
	tps := w.actualTps
	if tps <= 0 {
		tps = w.tps
	}
	return Snapshot{
		WaveTime: w.waveTime,
		Wave:     w.wave,
		Enemies:  0,
		Paused:   false,
		GameOver: false,
		TimeData: int32(time.Since(w.start).Seconds()),
		Tps:      tps,
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
		w.actualTps = s.Tps
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
	w.pendingBreaks = map[int32]pendingBreakState{}
	w.factoryStates = map[int32]factoryState{}
	w.unitMountCDs = map[int32][]float32{}
	w.unitTargets = map[int32]targetTrackState{}
	w.teamItems = map[TeamID]map[ItemID]int32{}
	w.itemSourceCfg = map[int32]ItemID{}
	w.liquidSourceCfg = map[int32]LiquidID{}
	w.sorterCfg = map[int32]ItemID{}
	w.unloaderCfg = map[int32]ItemID{}
	w.payloadRouterCfg = map[int32]protocol.Content{}
	w.bridgeLinks = map[int32]int32{}
	w.massDriverLinks = map[int32]int32{}
	w.payloadDriverLinks = map[int32]int32{}
	w.bridgeBuffers = map[int32][]bufferedBridgeItem{}
	w.bridgeAcceptAcc = map[int32]float32{}
	w.conveyorStates = map[int32]*conveyorRuntimeState{}
	w.ductStates = map[int32]*ductRuntimeState{}
	w.routerStates = map[int32]*routerRuntimeState{}
	w.stackStates = map[int32]*stackRuntimeState{}
	w.massDriverStates = map[int32]*massDriverRuntimeState{}
	w.payloadStates = map[int32]*payloadRuntimeState{}
	w.payloadDriverStates = map[int32]*payloadDriverRuntimeState{}
	w.massDriverShots = []massDriverShot{}
	w.payloadDriverShots = []payloadDriverShot{}
	w.blockDumpIndex = map[int32]int{}
	w.itemSourceAccum = map[int32]float32{}
	w.routerInputPos = map[int32]int32{}
	w.routerRotation = map[int32]byte{}
	w.transportAccum = map[int32]float32{}
	w.junctionQueues = map[int32]junctionQueueState{}
	w.reactorStates = map[int32]nuclearReactorState{}
	w.storageLinkedCore = map[int32]int32{}
	w.teamPrimaryCore = map[TeamID]int32{}
	w.coreStorageCapacity = map[int32]int32{}
	w.blockOccupancy = map[int32]int32{}
	w.activeTilePositions = nil
	w.nextPlanOrder = 0
	w.blockNamesByID = nil
	w.unitNamesByID = nil
	w.unitTypeDefsByID = nil

	// 从 tags 解析规则并应用
	if m != nil && m.Tags != nil {
		if rulesJSON, ok := m.Tags["rules"]; ok && rulesJSON != "" {
			w.rulesMgr.FromJSON([]byte(rulesJSON))
			// 应用倍率到现有单位和建筑
			w.applyRulesToEntities()
		}
	}

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
	w.rebuildBlockOccupancyLocked()
	w.restoreTileConfigsLocked()
	w.restorePayloadStatesLocked()
}

func (w *World) stepPendingBuilds(delta time.Duration) {
	if w.model == nil || len(w.pendingBuilds) == 0 {
		return
	}
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return
	}
	activePosByOwner := make(map[int32]int32, len(w.pendingBuilds))
	activeOrderByOwner := make(map[int32]uint64, len(w.pendingBuilds))
	for pos, st := range w.pendingBuilds {
		if st.Team == 0 {
			continue
		}
		ownerKey := st.Owner
		if ownerKey == 0 {
			ownerKey = -1 - int32(st.Team)
		}
		if st.QueueOrder == 0 {
			w.nextPlanOrder++
			st.QueueOrder = w.nextPlanOrder
			w.pendingBuilds[pos] = st
		}
		if curOrder, ok := activeOrderByOwner[ownerKey]; !ok || st.QueueOrder < curOrder {
			activeOrderByOwner[ownerKey] = st.QueueOrder
			activePosByOwner[ownerKey] = pos
		}
	}
	earliestBreakByOwner := make(map[int32]uint64, len(w.pendingBreaks))
	for _, st := range w.pendingBreaks {
		if st.Team == 0 {
			continue
		}
		ownerKey := st.Owner
		if ownerKey == 0 {
			ownerKey = -1 - int32(st.Team)
		}
		if cur, ok := earliestBreakByOwner[ownerKey]; !ok || st.QueueOrder < cur {
			earliestBreakByOwner[ownerKey] = st.QueueOrder
		}
	}
	rules := w.rulesMgr.Get()
	dirtyActiveTiles := false
	for owner, pos := range activePosByOwner {
		st, ok := w.pendingBuilds[pos]
		if !ok {
			continue
		}
		if breakOrder, ok := earliestBreakByOwner[owner]; ok && breakOrder < st.QueueOrder {
			continue
		}
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
		if !st.VisualPlaced {
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:        EntityEventBuildPlaced,
				BuildPos:    packTilePos(tile.X, tile.Y),
				BuildTeam:   st.Team,
				BuildBlock:  st.BlockID,
				BuildRot:    st.Rotation,
				BuildConfig: st.Config,
			})
			st.VisualPlaced = true
			st.LastHP = 1
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:     EntityEventBuildHealth,
				BuildPos: packTilePos(tile.X, tile.Y),
				BuildHP:  st.LastHP,
			})
		}
		buildDuration := w.buildDurationSecondsForTeam(st.BlockID, st.Team, rules)
		st.Progress += dt / buildDuration
		hpNow := float32(1000) * clampf(st.Progress, 0, 1)
		if hpNow < 1 {
			hpNow = 1
		}
		if hpNow-st.LastHP >= 25 || st.Progress >= 1 {
			st.LastHP = hpNow
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:     EntityEventBuildHealth,
				BuildPos: packTilePos(tile.X, tile.Y),
				BuildHP:  hpNow,
			})
		}
		if st.Progress < 1 {
			w.pendingBuilds[pos] = st
			continue
		}
		tile.Block = BlockID(st.BlockID)
		tile.Team = st.Team
		tile.Rotation = st.Rotation
		tile.Build = &Building{
			Block:    tile.Block,
			Team:     st.Team,
			Rotation: st.Rotation,
			X:        tile.X,
			Y:        tile.Y,
			Health:   1000,
		}
		w.setBuildingOccupancyLocked(pos, tile, true)
		w.applyBuildingConfigLocked(pos, st.Config, true)
		w.ensureTeamInventory(st.Team)
		dirtyActiveTiles = true
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:     EntityEventBuildHealth,
			BuildPos: packTilePos(tile.X, tile.Y),
			BuildHP:  tile.Build.Health,
		}, EntityEvent{
			Kind:        EntityEventBuildConstructed,
			BuildPos:    packTilePos(tile.X, tile.Y),
			BuildTeam:   st.Team,
			BuildBlock:  st.BlockID,
			BuildRot:    st.Rotation,
			BuildConfig: st.Config,
		})
		delete(w.pendingBuilds, pos)
	}
	if dirtyActiveTiles {
		w.rebuildActiveTilesLocked()
	}
}

func (w *World) stepPendingBreaks(delta time.Duration) {
	if w.model == nil || len(w.pendingBreaks) == 0 {
		return
	}
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return
	}
	activePosByOwner := make(map[int32]int32, len(w.pendingBreaks))
	activeOrderByOwner := make(map[int32]uint64, len(w.pendingBreaks))
	for pos, st := range w.pendingBreaks {
		if st.Team == 0 {
			continue
		}
		ownerKey := st.Owner
		if ownerKey == 0 {
			ownerKey = -1 - int32(st.Team)
		}
		if st.QueueOrder == 0 {
			w.nextPlanOrder++
			st.QueueOrder = w.nextPlanOrder
			w.pendingBreaks[pos] = st
		}
		if curOrder, ok := activeOrderByOwner[ownerKey]; !ok || st.QueueOrder < curOrder {
			activeOrderByOwner[ownerKey] = st.QueueOrder
			activePosByOwner[ownerKey] = pos
		}
	}
	earliestBuildByOwner := make(map[int32]uint64, len(w.pendingBuilds))
	for _, st := range w.pendingBuilds {
		if st.Team == 0 {
			continue
		}
		ownerKey := st.Owner
		if ownerKey == 0 {
			ownerKey = -1 - int32(st.Team)
		}
		if cur, ok := earliestBuildByOwner[ownerKey]; !ok || st.QueueOrder < cur {
			earliestBuildByOwner[ownerKey] = st.QueueOrder
		}
	}
	rules := w.rulesMgr.Get()
	dirtyActiveTiles := false
	for owner, pos := range activePosByOwner {
		st, ok := w.pendingBreaks[pos]
		if !ok {
			continue
		}
		if buildOrder, ok := earliestBuildByOwner[owner]; ok && buildOrder < st.QueueOrder {
			continue
		}
		x := int(pos % int32(w.model.Width))
		y := int(pos / int32(w.model.Width))
		if !w.model.InBounds(x, y) {
			delete(w.pendingBreaks, pos)
			continue
		}
		tile, err := w.model.TileAt(x, y)
		if err != nil || tile == nil || tile.Block == 0 {
			delete(w.pendingBreaks, pos)
			continue
		}
		breakDuration := w.buildDurationSecondsForTeam(st.BlockID, st.Team, rules)
		if breakDuration < 0.6 {
			breakDuration = 0.6
		}
		if !st.VisualStart {
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:       EntityEventBuildDeconstructing,
				BuildPos:   packTilePos(tile.X, tile.Y),
				BuildTeam:  st.Team,
				BuildBlock: st.BlockID,
				BuildRot:   st.Rotation,
			})
			st.VisualStart = true
		}
		st.Progress += dt / breakDuration
		hpNow := st.MaxHealth * (1 - clampf(st.Progress, 0, 1))
		if hpNow < 1 && st.Progress < 1 {
			hpNow = 1
		}
		if tile.Build != nil {
			tile.Build.Health = hpNow
		}
		if st.LastHP-hpNow >= 25 || st.Progress >= 1 {
			st.LastHP = hpNow
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:     EntityEventBuildHealth,
				BuildPos: packTilePos(tile.X, tile.Y),
				BuildHP:  hpNow,
			})
		}
		if st.Progress < 1 {
			w.pendingBreaks[pos] = st
			continue
		}
		w.refundDeconstructCost(tile, st.Team)
		teamOld := tile.Team
		if tile.Build != nil && tile.Build.Team != 0 {
			teamOld = tile.Build.Team
		}
		if teamOld == 0 {
			teamOld = st.Team
		}
		w.setBuildingOccupancyLocked(pos, tile, false)
		tile.Build = nil
		tile.Block = 0
		tile.Team = 0
		tile.Rotation = 0
		delete(w.buildStates, pos)
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:       EntityEventBuildDestroyed,
			BuildPos:   packTilePos(tile.X, tile.Y),
			BuildTeam:  teamOld,
			BuildBlock: st.BlockID,
		})
		delete(w.pendingBreaks, pos)
		dirtyActiveTiles = true
	}
	if dirtyActiveTiles {
		w.rebuildActiveTilesLocked()
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
	return w.resolveUnitTypeIDLocked(want)
}

func (w *World) resolveUnitTypeIDLocked(want string) (int16, bool) {
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
	if len(payload.Blocks) > 0 {
		costs := make(map[string][]ItemStack, len(payload.Blocks))
		times := make(map[string]float32, len(payload.Blocks))
		for _, b := range payload.Blocks {
			name := strings.ToLower(strings.TrimSpace(b.Name))
			if name == "" {
				continue
			}
			if b.BuildTimeSec > 0 {
				times[name] = b.BuildTimeSec
			}
			if len(b.Requirements) == 0 {
				continue
			}
			items := make([]ItemStack, 0, len(b.Requirements))
			for _, r := range b.Requirements {
				if r.Amount <= 0 || r.ItemID < 0 {
					continue
				}
				items = append(items, ItemStack{Item: ItemID(r.ItemID), Amount: r.Amount})
			}
			if len(items) > 0 {
				costs[name] = items
			}
		}
		if len(costs) > 0 {
			w.blockCostsByName = costs
		}
		if len(times) > 0 {
			w.blockBuildTimesByName = times
		}
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
		p.FireMode = strings.TrimSpace(u.FireMode)
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
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:   EntityEventRemoved,
			Entity: ent,
		})
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

func (w *World) TeamItems(team TeamID) map[ItemID]int32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if items := w.teamCoreItemsLocked(team); len(items) > 0 {
		return items
	}
	src := w.teamItems[team]
	if len(src) == 0 {
		return map[ItemID]int32{}
	}
	out := make(map[ItemID]int32, len(src))
	for k, v := range src {
		out[k] = v
	}
	return out
}

func (w *World) TeamCoreItemSnapshots() []TeamCoreItemSnapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil {
		return nil
	}
	teams := make(map[TeamID]map[ItemID]int32)
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Team == 0 || tile.Build == nil || tile.Block <= 0 {
			continue
		}
		if !strings.HasPrefix(w.blockNameByID(int16(tile.Block)), "core-") {
			continue
		}
		items, ok := teams[tile.Team]
		if !ok {
			items = make(map[ItemID]int32)
			teams[tile.Team] = items
		}
		for _, stack := range tile.Build.Items {
			if stack.Amount <= 0 {
				continue
			}
			items[stack.Item] += stack.Amount
		}
	}
	if len(teams) == 0 {
		return nil
	}
	order := make([]int, 0, len(teams))
	for team := range teams {
		order = append(order, int(team))
	}
	sort.Ints(order)
	out := make([]TeamCoreItemSnapshot, 0, len(order))
	for _, rawTeam := range order {
		team := TeamID(rawTeam)
		itemMap := teams[team]
		itemIDs := make([]int, 0, len(itemMap))
		for item, amount := range itemMap {
			if amount > 0 {
				itemIDs = append(itemIDs, int(item))
			}
		}
		sort.Ints(itemIDs)
		items := make([]ItemStack, 0, len(itemIDs))
		for _, rawItem := range itemIDs {
			item := ItemID(rawItem)
			items = append(items, ItemStack{Item: item, Amount: itemMap[item]})
		}
		out = append(out, TeamCoreItemSnapshot{Team: team, Items: items})
	}
	return out
}

func (w *World) TeamItemSyncPositions(team TeamID) []int32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || team == 0 {
		return nil
	}
	positions := make([]int32, 0, 8)
	seen := make(map[int32]struct{})
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Team != team || tile.Build == nil || tile.Block == 0 {
			continue
		}
		name := w.blockNameByID(int16(tile.Block))
		if !isCoreBlockName(name) {
			continue
		}
		packed := packTilePos(tile.X, tile.Y)
		if _, ok := seen[packed]; !ok {
			seen[packed] = struct{}{}
			positions = append(positions, packed)
		}
	}
	for pos, corePos := range w.storageLinkedCore {
		if corePos < 0 || int(corePos) >= len(w.model.Tiles) || pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		coreTile := &w.model.Tiles[corePos]
		storageTile := &w.model.Tiles[pos]
		if coreTile.Team != team || storageTile.Team != team || storageTile.Build == nil || storageTile.Block == 0 {
			continue
		}
		packed := packTilePos(storageTile.X, storageTile.Y)
		if _, ok := seen[packed]; !ok {
			seen[packed] = struct{}{}
			positions = append(positions, packed)
		}
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i] < positions[j] })
	return positions
}

func unitBuildSpeedByName(name string) float32 {
	switch normalizeUnitName(name) {
	case "alpha":
		return 0.5
	case "beta":
		return 0.75
	case "gamma":
		return 1.0
	case "evoke":
		return 1.2
	case "incite":
		return 1.4
	case "emanate":
		return 1.5
	default:
		return 0.5
	}
}

func (w *World) BuilderSpeedForUnitType(typeID int16) float32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.unitNamesByID == nil {
		return 0.5
	}
	name := w.unitNamesByID[typeID]
	return unitBuildSpeedByName(name)
}

func (w *World) SetTeamBuilderSpeed(team TeamID, speed float32) {
	if team == 0 || speed <= 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.teamBuilderSpeed == nil {
		w.teamBuilderSpeed = make(map[TeamID]float32)
	}
	w.teamBuilderSpeed[team] = speed
}

func (w *World) TeamCorePositions(team TeamID) []int32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || team == 0 {
		return nil
	}
	out := make([]int32, 0, 4)
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		t := &w.model.Tiles[pos]
		if t.Team != team || t.Block <= 0 {
			continue
		}
		name := w.blockNameByID(int16(t.Block))
		if strings.Contains(name, "core-") {
			out = append(out, packTilePos(t.X, t.Y))
		}
	}
	return out
}

func (w *World) BuildSyncSnapshot() []BuildSyncState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}
	out := make([]BuildSyncState, 0, len(w.activeTilePositions))
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		t := w.model.Tiles[pos]
		if t.Block <= 0 {
			continue
		}
		if t.Team == 0 && t.Build == nil {
			continue
		}
		hp := float32(1000)
		if t.Build != nil && t.Build.Health > 0 {
			hp = t.Build.Health
		}
		out = append(out, BuildSyncState{
			Pos:      packTilePos(t.X, t.Y),
			X:        int32(t.X),
			Y:        int32(t.Y),
			BlockID:  int16(t.Block),
			Team:     t.Team,
			Rotation: t.Rotation,
			Health:   hp,
		})
	}
	return out
}

// ApplyBuildPlans applies incremental build/break operations from client packets.
func (w *World) ApplyBuildPlans(team TeamID, ops []BuildPlanOp) []int32 {
	return w.ApplyBuildPlansForOwner(0, team, ops)
}

func (w *World) ApplyBuildPlansForOwner(owner int32, team TeamID, ops []BuildPlanOp) []int32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || len(ops) == 0 {
		return nil
	}
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
		w.nextPlanOrder++
		w.applyBuildPlanOpLocked(owner, team, op, w.nextPlanOrder, addChanged)
	}
	return changed
}

// ApplyBuildPlanSnapshot reconciles one team's queue with authoritative snapshot plans.
// This matches vanilla queue semantics: absent plans are removed, present plans are ordered.
func (w *World) ApplyBuildPlanSnapshot(team TeamID, ops []BuildPlanOp) []int32 {
	return w.ApplyBuildPlanSnapshotForOwner(0, team, ops)
}

func (w *World) ApplyBuildPlanSnapshotForOwner(owner int32, team TeamID, ops []BuildPlanOp) []int32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return nil
	}
	changed := make([]int32, 0, len(ops))
	seen := make(map[int32]struct{}, len(ops))
	addChanged := func(pos int32) {
		if _, ok := seen[pos]; ok {
			return
		}
		seen[pos] = struct{}{}
		changed = append(changed, pos)
	}

	ordered := make([]BuildPlanOp, 0, len(ops))
	wantBuild := make(map[int32]struct{}, len(ops))
	wantBreak := make(map[int32]struct{}, len(ops))
	for _, op := range ops {
		if !w.model.InBounds(int(op.X), int(op.Y)) {
			continue
		}
		pos := int32(int(op.Y)*w.model.Width + int(op.X))
		if op.Breaking {
			if _, ok := wantBreak[pos]; ok {
				continue
			}
			wantBreak[pos] = struct{}{}
			ordered = append(ordered, BuildPlanOp{
				Breaking: true,
				X:        op.X,
				Y:        op.Y,
			})
			continue
		}
		if op.BlockID <= 0 {
			continue
		}
		if _, ok := wantBuild[pos]; ok {
			continue
		}
		wantBuild[pos] = struct{}{}
		ordered = append(ordered, op)
	}

	_ = wantBuild
	_ = wantBreak

	// Reconcile removals: any queued plan for this team that is absent from
	// the latest authoritative snapshot must be dropped immediately.
	for pos, st := range w.pendingBuilds {
		if st.Team != team || st.Owner != owner {
			continue
		}
		if _, ok := wantBuild[pos]; ok {
			continue
		}
		delete(w.pendingBuilds, pos)
		w.refundBuildCost(st.Team, st.BlockID, 1.0)
		x := int(pos % int32(w.model.Width))
		y := int(pos / int32(w.model.Width))
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:       EntityEventBuildDestroyed,
			BuildPos:   packTilePos(x, y),
			BuildTeam:  st.Team,
			BuildBlock: st.BlockID,
		})
		addChanged(pos)
	}
	for pos, st := range w.pendingBreaks {
		if st.Team != team || st.Owner != owner {
			continue
		}
		if _, ok := wantBreak[pos]; ok {
			continue
		}
		delete(w.pendingBreaks, pos)
		addChanged(pos)
	}

	for i, op := range ordered {
		w.applyBuildPlanOpLocked(owner, team, op, uint64(i+1), addChanged)
	}
	return changed
}

func (w *World) applyBuildPlanOpLocked(owner int32, team TeamID, op BuildPlanOp, queueOrder uint64, addChanged func(int32)) {
	if w.model == nil || !w.model.InBounds(int(op.X), int(op.Y)) {
		return
	}
	tile, err := w.model.TileAt(int(op.X), int(op.Y))
	if err != nil || tile == nil {
		return
	}
	pos := int32(tile.Y*w.model.Width + tile.X)

	if op.Breaking {
		if st, ok := w.pendingBuilds[pos]; ok {
			if owner != 0 && st.Owner != 0 && st.Owner != owner {
				return
			}
			delete(w.pendingBuilds, pos)
			w.refundBuildCost(st.Team, st.BlockID, 1.0)
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:       EntityEventBuildDestroyed,
				BuildPos:   packTilePos(tile.X, tile.Y),
				BuildTeam:  st.Team,
				BuildBlock: st.BlockID,
			})
			addChanged(pos)
		}
		delete(w.factoryStates, pos)
		if st, ok := w.pendingBreaks[pos]; ok {
			if st.BlockID == int16(tile.Block) && (owner == 0 || st.Owner == 0 || st.Owner == owner) {
				st.Owner = owner
				st.QueueOrder = queueOrder
				w.pendingBreaks[pos] = st
				return
			}
		}
		if tile.Build == nil && tile.Block == 0 {
			delete(w.pendingBreaks, pos)
			return
		}
		if rules := w.rulesMgr.Get(); rules != nil && (rules.InstantBuild || rules.Editor) {
			w.destroyTileLocked(tile, team)
			delete(w.pendingBreaks, pos)
			addChanged(pos)
			return
		}
		maxHP := float32(1000)
		if tile.Build != nil && tile.Build.Health > 0 {
			maxHP = tile.Build.Health
		}
		w.pendingBreaks[pos] = pendingBreakState{
			Owner:       owner,
			Team:        team,
			BlockID:     int16(tile.Block),
			Rotation:    tile.Rotation,
			QueueOrder:  queueOrder,
			VisualStart: false,
			Progress:    0,
			MaxHealth:   maxHP,
			LastHP:      maxHP,
		}
		addChanged(pos)
		return
	}

	if op.BlockID <= 0 {
		return
	}
	if pending, ok := w.pendingBuilds[pos]; ok {
		if owner != 0 && pending.Owner != 0 && pending.Owner != owner {
			return
		}
		if pending.BlockID == op.BlockID && pending.Team == team && pending.Rotation == op.Rotation && reflect.DeepEqual(pending.Config, op.Config) {
			pending.Owner = owner
			pending.QueueOrder = queueOrder
			w.pendingBuilds[pos] = pending
			return
		}
		w.refundBuildCost(pending.Team, pending.BlockID, 1.0)
	}
	if tile.Block == BlockID(op.BlockID) && tile.Team == team && tile.Rotation == op.Rotation && tile.Build != nil {
		w.applyBuildingConfigLocked(pos, op.Config, true)
		delete(w.pendingBreaks, pos)
		delete(w.pendingBuilds, pos)
		return
	}
	if !w.consumeBuildCost(team, op.BlockID) {
		fmt.Printf("[buildtrace] reject plan xy=(%d,%d) block=%d team=%d reason=insufficient_items\n", tile.X, tile.Y, op.BlockID, team)
		return
	}
	if rules := w.rulesMgr.Get(); rules != nil && (rules.InstantBuild || rules.Editor) {
		w.placeTileLocked(tile, team, op.BlockID, int8(op.Rotation), op.Config)
		delete(w.pendingBuilds, pos)
		delete(w.pendingBreaks, pos)
		addChanged(pos)
		return
	}
	w.pendingBuilds[pos] = pendingBuildState{
		Owner:      owner,
		Team:       team,
		BlockID:    op.BlockID,
		Rotation:   op.Rotation,
		Config:     op.Config,
		QueueOrder: queueOrder,
		Progress:   0,
	}
	delete(w.pendingBreaks, pos)
	addChanged(pos)
}

func (w *World) placeTileLocked(tile *Tile, team TeamID, blockID int16, rotation int8, config any) {
	if tile == nil {
		return
	}
	pos := packTilePos(tile.X, tile.Y)
	w.entityEvents = append(w.entityEvents,
		EntityEvent{
			Kind:        EntityEventBuildPlaced,
			BuildPos:    pos,
			BuildTeam:   team,
			BuildBlock:  blockID,
			BuildRot:    rotation,
			BuildConfig: config,
		},
		EntityEvent{
			Kind:     EntityEventBuildHealth,
			BuildPos: pos,
			BuildHP:  1000,
		},
	)
	tile.Block = BlockID(blockID)
	tile.Team = team
	tile.Rotation = rotation
	tile.Build = &Building{
		Block:     tile.Block,
		Team:      team,
		Rotation:  rotation,
		X:         tile.X,
		Y:         tile.Y,
		Health:    1000,
		MaxHealth: 1000,
	}
	w.setBuildingOccupancyLocked(int32(tile.Y*w.model.Width+tile.X), tile, true)
	w.rebuildActiveTilesLocked()
	w.refreshCoreStorageLinksLocked()
	w.applyBuildingConfigLocked(int32(tile.Y*w.model.Width+tile.X), config, true)
	w.ensureTeamInventory(team)
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:        EntityEventBuildConstructed,
		BuildPos:    pos,
		BuildTeam:   team,
		BuildBlock:  blockID,
		BuildRot:    rotation,
		BuildConfig: config,
	})
}

func (w *World) destroyTileLocked(tile *Tile, fallbackTeam TeamID) {
	if tile == nil || (tile.Block == 0 && tile.Build == nil) {
		return
	}
	pos := int32(tile.Y*w.model.Width + tile.X)
	blockID := int16(tile.Block)
	teamOld := tile.Team
	if tile.Build != nil && tile.Build.Team != 0 {
		teamOld = tile.Build.Team
	}
	if teamOld == 0 {
		teamOld = fallbackTeam
	}
	w.refundDeconstructCost(tile, fallbackTeam)
	w.setBuildingOccupancyLocked(pos, tile, false)
	tile.Block = 0
	tile.Rotation = 0
	tile.Team = 0
	tile.Build = nil
	w.clearBuildingRuntimeLocked(pos)
	w.rebuildActiveTilesLocked()
	w.refreshCoreStorageLinksLocked()
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventBuildDestroyed,
		BuildPos:   packTilePos(tile.X, tile.Y),
		BuildTeam:  teamOld,
		BuildBlock: blockID,
	})
}

func (w *World) CancelBuildPlansPacked(positions []int32) {
	w.CancelBuildPlansPackedForOwner(0, positions)
}

func (w *World) CancelBuildPlansPackedForOwner(owner int32, positions []int32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || len(positions) == 0 {
		return
	}
	for _, packed := range positions {
		x, y := unpackTilePos(packed)
		if !w.model.InBounds(x, y) {
			continue
		}
		pos := int32(y*w.model.Width + x)
		if st, ok := w.pendingBuilds[pos]; ok {
			if owner != 0 && st.Owner != 0 && st.Owner != owner {
				continue
			}
			delete(w.pendingBuilds, pos)
			w.refundBuildCost(st.Team, st.BlockID, 1.0)
			// Ensure client-side construct ghost is cleared when queue cancellation happens mid-build.
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:       EntityEventBuildDestroyed,
				BuildPos:   packTilePos(x, y),
				BuildTeam:  st.Team,
				BuildBlock: st.BlockID,
			})
		}
		if st, ok := w.pendingBreaks[pos]; ok {
			if owner != 0 && st.Owner != 0 && st.Owner != owner {
				continue
			}
			delete(w.pendingBreaks, pos)
		}
	}
}

func (w *World) CancelBuildAt(x, y int32, breaking bool) {
	w.CancelBuildAtForOwner(0, x, y, breaking)
}

func (w *World) CancelBuildAtForOwner(owner int32, x, y int32, breaking bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || !w.model.InBounds(int(x), int(y)) {
		return
	}
	pos := int32(int(y)*w.model.Width + int(x))
	if breaking {
		if st, ok := w.pendingBreaks[pos]; ok {
			if owner != 0 && st.Owner != 0 && st.Owner != owner {
				return
			}
			delete(w.pendingBreaks, pos)
		}
		return
	}
	if st, ok := w.pendingBuilds[pos]; ok {
		if owner != 0 && st.Owner != 0 && st.Owner != owner {
			return
		}
		delete(w.pendingBuilds, pos)
		w.refundBuildCost(st.Team, st.BlockID, 1.0)
		// Ensure client-side construct ghost is cleared when queue cancellation happens mid-build.
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:       EntityEventBuildDestroyed,
			BuildPos:   packTilePos(int(x), int(y)),
			BuildTeam:  st.Team,
			BuildBlock: st.BlockID,
		})
	}
}

func (w *World) CancelBuildPlansByTeam(team TeamID) {
	if team == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return
	}
	for pos, st := range w.pendingBuilds {
		if st.Team != team {
			continue
		}
		delete(w.pendingBuilds, pos)
		w.refundBuildCost(st.Team, st.BlockID, 1.0)
		x := int(pos % int32(w.model.Width))
		y := int(pos / int32(w.model.Width))
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:       EntityEventBuildDestroyed,
			BuildPos:   packTilePos(x, y),
			BuildTeam:  st.Team,
			BuildBlock: st.BlockID,
		})
	}
	for pos, st := range w.pendingBreaks {
		if st.Team == team {
			delete(w.pendingBreaks, pos)
		}
	}
}

func (w *World) CancelBuildPlansByOwner(owner int32) {
	if owner == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return
	}
	for pos, st := range w.pendingBuilds {
		if st.Owner != owner {
			continue
		}
		delete(w.pendingBuilds, pos)
		w.refundBuildCost(st.Team, st.BlockID, 1.0)
		x := int(pos % int32(w.model.Width))
		y := int(pos / int32(w.model.Width))
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:       EntityEventBuildDestroyed,
			BuildPos:   packTilePos(x, y),
			BuildTeam:  st.Team,
			BuildBlock: st.BlockID,
		})
	}
	for pos, st := range w.pendingBreaks {
		if st.Owner == owner {
			delete(w.pendingBreaks, pos)
		}
	}
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
		if e.AttackCooldown > 0 {
			slowMul := clampf(e.SlowMul, 0.2, 1)
			e.AttackCooldown -= dt * slowMul
			if e.AttackCooldown < 0 {
				e.AttackCooldown = 0
			}
			continue
		}
		rangeLimit := e.AttackRange
		if rangeLimit <= 0 {
			rangeLimit = 56
		}
		track := w.unitTargets[e.ID]
		retargetDelay := maxf(e.AttackInterval*0.45, 0.18)
		if tid, ok := w.acquireTrackedEntityTarget(*e, ents, idToIndex, rangeLimit, e.AttackTargetAir, e.AttackTargetGround, e.AttackTargetPriority, &track, dt, retargetDelay); ok {
			if idx, exists := idToIndex[tid]; exists && idx >= 0 && idx < len(ents) {
				target := &ents[idx]
				e.AttackCooldown = maxf(e.AttackInterval, 0.2)
				e.Rotation = lookAt(e.X, e.Y, target.X, target.Y)
				if e.AttackFireMode == "beam" {
					w.applyDamageToEntity(target, e.AttackDamage)
					applySlow(target, e.AttackSlowSec, e.AttackSlowMul)
					w.applyBeamChain(*e, idx)
				} else {
					w.spawnBullet(*e, target.X, target.Y)
				}
				if !e.AttackPreferBuildings {
					continue
				}
			}
		}
		w.unitTargets[e.ID] = track
		if e.AttackBuildings {
			if pos, tx, ty, ok := w.findNearestEnemyBuilding(*e, rangeLimit); ok {
				_ = pos
				e.AttackCooldown = maxf(e.AttackInterval, 0.2)
				e.Rotation = lookAt(e.X, e.Y, tx, ty)
				if e.AttackFireMode == "beam" {
					_ = w.applyDamageToBuilding(pos, e.AttackDamage)
				} else {
					w.spawnBullet(*e, tx, ty)
				}
			}
		}
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
	rangeLimit := e.AttackRange
	if rangeLimit <= 0 {
		rangeLimit = 56
	}
	unitFired := false
	track := w.unitTargets[e.ID]
	retargetDelay := maxf(e.AttackInterval*0.45, 0.18)
	if tid, ok := w.acquireTrackedEntityTarget(*e, w.model.Entities, idToIndex, rangeLimit, e.AttackTargetAir, e.AttackTargetGround, e.AttackTargetPriority, &track, dt, retargetDelay); ok {
		if idx, exists := idToIndex[tid]; exists && idx >= 0 && idx < len(w.model.Entities) {
			target := &w.model.Entities[idx]
			for mi := range mounts {
				if cds[mi] > 0 {
					continue
				}
				if w.fireEntityMountAtUnit(e, target, mounts[mi], idx) {
					cds[mi] = maxf(e.AttackInterval*maxf(mounts[mi].CooldownMul, 0.15), 0.05)
					unitFired = true
				}
			}
		}
	}
	if e.AttackBuildings && (!unitFired || e.AttackPreferBuildings) {
		if pos, tx, ty, ok := w.findNearestEnemyBuilding(*e, rangeLimit); ok {
			for mi := range mounts {
				if cds[mi] > 0 {
					continue
				}
				if w.fireEntityMountAtBuilding(e, pos, tx, ty, mounts[mi]) {
					cds[mi] = maxf(e.AttackInterval*maxf(mounts[mi].CooldownMul, 0.15), 0.05)
				}
			}
		}
	}
	w.unitMountCDs[e.ID] = cds
	w.unitTargets[e.ID] = track
}

func (w *World) fireEntityMountAtUnit(e *RawEntity, target *RawEntity, mount unitWeaponMountProfile, targetIdx int) bool {
	if e == nil || target == nil || target.Health <= 0 {
		return false
	}
	src := *e
	applyMountStats(&src, mount)
	baseAngle := lookAt(src.X, src.Y, target.X, target.Y)
	aimAngle := baseAngle + mount.AngleOffset
	src.Rotation = aimAngle
	if src.AttackFireMode == "beam" {
		scale := maxf(mount.DamageMul, 0.05)
		w.applyDamageToEntity(target, src.AttackDamage*scale)
		applySlow(target, src.AttackSlowSec*scale, src.AttackSlowMul)
		w.applyBeamChain(src, targetIdx)
		return true
	}
	tx, ty := target.X, target.Y
	if mount.AngleOffset != 0 {
		rad := float32(aimAngle * math.Pi / 180)
		dist := maxf(src.AttackRange*0.85, 24)
		tx = src.X + float32(math.Cos(float64(rad)))*dist
		ty = src.Y + float32(math.Sin(float64(rad)))*dist
	}
	w.spawnBullet(src, tx, ty)
	return true
}

func (w *World) fireEntityMountAtBuilding(e *RawEntity, pos int32, tx, ty float32, mount unitWeaponMountProfile) bool {
	if e == nil {
		return false
	}
	src := *e
	applyMountStats(&src, mount)
	src.Rotation = lookAt(src.X, src.Y, tx, ty) + mount.AngleOffset
	if src.AttackFireMode == "beam" {
		scale := maxf(mount.DamageMul, 0.05)
		_ = w.applyDamageToBuilding(pos, src.AttackDamage*scale)
		return true
	}
	if mount.AngleOffset != 0 {
		rad := float32(src.Rotation * math.Pi / 180)
		dist := maxf(src.AttackRange*0.85, 24)
		tx = src.X + float32(math.Cos(float64(rad)))*dist
		ty = src.Y + float32(math.Sin(float64(rad)))*dist
	}
	w.spawnBullet(src, tx, ty)
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
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		t := &w.model.Tiles[pos]
		if t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		prof, ok := w.getBuildingWeaponProfile(int16(t.Build.Block))
		if !ok || prof.Damage <= 0 || prof.Interval <= 0 || prof.Range <= 0 {
			continue
		}
		state, exists := w.buildStates[pos]
		if !exists {
			state = buildCombatState{
				Ammo:  prof.AmmoCapacity,
				Power: prof.PowerCapacity,
			}
		}
		state = w.regenBuildState(state, prof, dt)
		if state.Cooldown > 0 {
			state.Cooldown -= dt
			if state.Cooldown < 0 {
				state.Cooldown = 0
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
		}

		allowShot := state.Cooldown <= 0 && (state.BurstRemain == 0 || state.BurstDelay <= 0)
		if allowShot && w.tryFireBuildingShot(&src, &state, prof, ents, idToIndex) {
			if state.BurstRemain > 0 {
				state.BurstRemain--
				state.BurstDelay = maxf(prof.BurstSpacing, 0.02)
			} else {
				shots := prof.BurstShots
				if shots < 1 {
					shots = 1
				}
				state.BurstRemain = shots - 1
				if state.BurstRemain > 0 {
					state.BurstDelay = maxf(prof.BurstSpacing, 0.02)
				}
				state.Cooldown = maxf(prof.Interval, 0.05)
			}
			t.Rotation = int8((int(src.Rotation/90) + 4) % 4)
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

func (w *World) spawnBullet(src RawEntity, tx, ty float32) {
	bulletSpeed := src.AttackBulletSpeed
	if bulletSpeed <= 0 {
		speed := src.MoveSpeed
		if speed <= 0 {
			speed = 18
		}
		bulletSpeed = maxf(speed*2.2, 28)
	}
	angle := lookAt(src.X, src.Y, tx, ty)
	rad := float32(angle * math.Pi / 180)
	b := simBullet{
		ID:             w.bulletNextID,
		Team:           src.Team,
		X:              src.X,
		Y:              src.Y,
		VX:             float32(math.Cos(float64(rad))) * bulletSpeed,
		VY:             float32(math.Sin(float64(rad))) * bulletSpeed,
		Damage:         src.AttackDamage,
		LifeSec:        maxf(src.AttackRange/bulletSpeed, 0.6),
		Radius:         5,
		HitUnits:       true,
		HitBuilds:      src.AttackBuildings,
		BulletType:     src.AttackBulletType,
		SplashRadius:   src.AttackSplashRadius,
		SlowSec:        src.AttackSlowSec,
		SlowMul:        clampf(src.AttackSlowMul, 0.2, 1),
		PierceRemain:   src.AttackPierce,
		ChainCount:     src.AttackChainCount,
		ChainRange:     src.AttackChainRange,
		FragmentCount:  src.AttackFragmentCount,
		FragmentSpread: src.AttackFragmentSpread,
		FragmentSpeed:  src.AttackFragmentSpeed,
		FragmentLife:   src.AttackFragmentLife,
		TargetAir:      src.AttackTargetAir,
		TargetGround:   src.AttackTargetGround,
		TargetPriority: src.AttackTargetPriority,
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
		if b.HitUnits {
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
			if pos, _, _, ok := w.findNearestEnemyBuilding(RawEntity{X: b.X, Y: b.Y, Team: b.Team}, b.Radius); ok {
				if w.applyDamageToBuilding(pos, b.Damage) {
					w.applySplashDamage(*b)
					hit = true
				}
			}
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
	for _, pos := range w.activeTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		t := &w.model.Tiles[pos]
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
		bestPos = pos
		bestX = tx
		bestY = ty
		found = true
	}
	if !found {
		return 0, 0, 0, false
	}
	return bestPos, bestX, bestY, true
}

func (w *World) applyDamageToBuilding(pos int32, damage float32) bool {
	if w.model == nil || damage <= 0 {
		return false
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
	prevBlock := int16(t.Block)
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
	w.clearBuildingRuntimeLocked(pos)
	w.rebuildActiveTilesLocked()
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventBuildDestroyed,
		BuildPos:   packTilePos(x, y),
		BuildTeam:  team,
		BuildBlock: prevBlock,
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
	dx := target.X - src.X
	dy := target.Y - src.Y
	return dx*dx+dy*dy <= rangeLimit*rangeLimit
}

func findNearestEnemyEntity(src RawEntity, ents []RawEntity, rangeLimit float32, allowAir, allowGround bool, priority string) (int32, bool) {
	if !allowAir && !allowGround {
		allowAir, allowGround = true, true
	}
	bestDist2 := rangeLimit * rangeLimit
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
		dx := e.X - src.X
		dy := e.Y - src.Y
		d2 := dx*dx + dy*dy
		if d2 > bestDist2 {
			continue
		}
		score := targetPriorityScore(src, e, d2, priority)
		if score < bestScore {
			bestScore = score
			bestDist2 = d2
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
			e.AttackPreferBuildings = p.PreferBuildings
			e.AttackTargetAir = p.TargetAir
			e.AttackTargetGround = p.TargetGround
			e.AttackTargetPriority = p.TargetPriority
			e.AttackBuildings = p.HitBuildings
			if e.HitRadius <= 0 {
				e.HitRadius = entityHitRadiusForType(e.TypeID)
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
	e.AttackPreferBuildings = p.PreferBuildings
	e.AttackTargetAir = p.TargetAir
	e.AttackTargetGround = p.TargetGround
	e.AttackTargetPriority = p.TargetPriority
	e.AttackBuildings = p.HitBuildings
	if e.HitRadius <= 0 {
		e.HitRadius = entityHitRadiusForType(e.TypeID)
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
	e.AttackTargetAir = def.Weapon.TargetAir
	e.AttackTargetGround = def.Weapon.TargetGround
	e.AttackBuildings = def.Weapon.TargetGround
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

func approachf(value, target, amount float32) float32 {
	if value < target {
		value += amount
		if value > target {
			return target
		}
		return value
	}
	value -= amount
	if value < target {
		return target
	}
	return value
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

func minf(a, b float32) float32 {
	if a < b {
		return a
	}
	return b
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
