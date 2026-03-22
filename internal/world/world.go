package world

import (
	"encoding/json"
	"fmt"
	"math"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

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

	tps int8

	start time.Time

	model *WorldModel

	// 规则和波次管理器
	rulesMgr *RulesManager
	wavesMgr *WaveManager

	entityEvents []EntityEvent
	bullets      []simBullet
	bulletNextID int32

	blockNamesByID         map[int16]string
	unitNamesByID          map[int16]string
	fallbackBlockNamesByID map[int16]string
	fallbackUnitNamesByID  map[int16]string
	coreBlockIDs           map[int16]struct{}
	unitTypeDefsByID       map[int16]vanilla.UnitTypeDef
	buildStates            map[int32]buildCombatState
	pendingBuilds          map[int32]pendingBuildState
	pendingBreaks          map[int32]pendingBreakState
	factoryStates          map[int32]factoryState
	unitMountCDs           map[int32][]float32
	unitTargets            map[int32]targetTrackState
	teamItems              map[TeamID]map[ItemID]int32
	teamBuilderSpeed       map[TeamID]float32
	nextPlanOrder          uint64

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
	Team         TeamID
	BlockID      int16
	Rotation     int8
	QueueOrder   uint64
	Progress     float32
	VisualPlaced bool
	LastHP       float32
}

type pendingBreakState struct {
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

type EntityEventKind string

const (
	EntityEventRemoved             EntityEventKind = "removed"
	EntityEventBuildPlaced         EntityEventKind = "build_placed"
	EntityEventBuildConstructed    EntityEventKind = "build_constructed"
	EntityEventBuildDeconstructing EntityEventKind = "build_deconstructing"
	EntityEventBuildDestroyed      EntityEventKind = "build_destroyed"
	EntityEventBuildRejected       EntityEventKind = "build_rejected"
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
	BuildHP     float32
	BuildReason string
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
		fallbackBlockNamesByID: map[int16]string{},
		fallbackUnitNamesByID:  map[int16]string{},
		coreBlockIDs:           map[int16]struct{}{},
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
	w.stepEntities(delta)
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
	w.pendingBreaks = map[int32]pendingBreakState{}
	w.factoryStates = map[int32]factoryState{}
	w.unitMountCDs = map[int32][]float32{}
	w.unitTargets = map[int32]targetTrackState{}
	w.teamItems = map[TeamID]map[ItemID]int32{}
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
	if len(w.fallbackBlockNamesByID) > 0 {
		if w.blockNamesByID == nil {
			w.blockNamesByID = make(map[int16]string, len(w.fallbackBlockNamesByID))
		}
		for id, name := range w.fallbackBlockNamesByID {
			if strings.TrimSpace(w.blockNamesByID[id]) == "" {
				w.blockNamesByID[id] = strings.ToLower(strings.TrimSpace(name))
			}
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
	if len(w.fallbackUnitNamesByID) > 0 {
		if w.unitNamesByID == nil {
			w.unitNamesByID = make(map[int16]string, len(w.fallbackUnitNamesByID))
		}
		for id, name := range w.fallbackUnitNamesByID {
			if strings.TrimSpace(w.unitNamesByID[id]) == "" {
				w.unitNamesByID[id] = strings.ToLower(strings.TrimSpace(name))
			}
		}
	}
	w.refreshCoreBlockIDsLocked()
}

// SetFallbackContentNames installs vanilla content-id name fallbacks.
// These names are used when current map content chunk lacks entries for some IDs.
func (w *World) SetFallbackContentNames(blocks, units map[int16]string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if blocks != nil {
		if w.fallbackBlockNamesByID == nil {
			w.fallbackBlockNamesByID = make(map[int16]string, len(blocks))
		}
		for id, name := range blocks {
			n := strings.ToLower(strings.TrimSpace(name))
			if n == "" {
				continue
			}
			w.fallbackBlockNamesByID[id] = n
			if w.blockNamesByID == nil {
				w.blockNamesByID = make(map[int16]string, len(blocks))
			}
			if strings.TrimSpace(w.blockNamesByID[id]) == "" {
				w.blockNamesByID[id] = n
			}
		}
	}
	if units != nil {
		if w.fallbackUnitNamesByID == nil {
			w.fallbackUnitNamesByID = make(map[int16]string, len(units))
		}
		for id, name := range units {
			n := strings.ToLower(strings.TrimSpace(name))
			if n == "" {
				continue
			}
			w.fallbackUnitNamesByID[id] = n
			if w.unitNamesByID == nil {
				w.unitNamesByID = make(map[int16]string, len(units))
			}
			if strings.TrimSpace(w.unitNamesByID[id]) == "" {
				w.unitNamesByID[id] = n
			}
		}
	}
	w.refreshCoreBlockIDsLocked()
}

func (w *World) stepPendingBuilds(delta time.Duration) {
	if w.model == nil || len(w.pendingBuilds) == 0 {
		return
	}
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return
	}
	activePosByTeam := make(map[TeamID]int32, len(w.pendingBuilds))
	activeOrderByTeam := make(map[TeamID]uint64, len(w.pendingBuilds))
	for pos, st := range w.pendingBuilds {
		if st.Team == 0 {
			continue
		}
		if st.QueueOrder == 0 {
			w.nextPlanOrder++
			st.QueueOrder = w.nextPlanOrder
			w.pendingBuilds[pos] = st
		}
		if curOrder, ok := activeOrderByTeam[st.Team]; !ok || st.QueueOrder < curOrder {
			activeOrderByTeam[st.Team] = st.QueueOrder
			activePosByTeam[st.Team] = pos
		}
	}
	earliestBreakByTeam := make(map[TeamID]uint64, len(w.pendingBreaks))
	for _, st := range w.pendingBreaks {
		if st.Team == 0 {
			continue
		}
		if cur, ok := earliestBreakByTeam[st.Team]; !ok || st.QueueOrder < cur {
			earliestBreakByTeam[st.Team] = st.QueueOrder
		}
	}
	rules := w.rulesMgr.Get()
	for team, pos := range activePosByTeam {
		st, ok := w.pendingBuilds[pos]
		if !ok {
			continue
		}
		if breakOrder, ok := earliestBreakByTeam[team]; ok && breakOrder < st.QueueOrder {
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
				Kind:       EntityEventBuildPlaced,
				BuildPos:   packTilePos(tile.X, tile.Y),
				BuildTeam:  st.Team,
				BuildBlock: st.BlockID,
				BuildRot:   st.Rotation,
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
		w.ensureTeamInventory(st.Team)
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:     EntityEventBuildHealth,
			BuildPos: packTilePos(tile.X, tile.Y),
			BuildHP:  tile.Build.Health,
		}, EntityEvent{
			Kind:       EntityEventBuildConstructed,
			BuildPos:   packTilePos(tile.X, tile.Y),
			BuildTeam:  st.Team,
			BuildBlock: st.BlockID,
			BuildRot:   st.Rotation,
		})
		delete(w.pendingBuilds, pos)
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
	activePosByTeam := make(map[TeamID]int32, len(w.pendingBreaks))
	activeOrderByTeam := make(map[TeamID]uint64, len(w.pendingBreaks))
	for pos, st := range w.pendingBreaks {
		if st.Team == 0 {
			continue
		}
		if st.QueueOrder == 0 {
			w.nextPlanOrder++
			st.QueueOrder = w.nextPlanOrder
			w.pendingBreaks[pos] = st
		}
		if curOrder, ok := activeOrderByTeam[st.Team]; !ok || st.QueueOrder < curOrder {
			activeOrderByTeam[st.Team] = st.QueueOrder
			activePosByTeam[st.Team] = pos
		}
	}
	earliestBuildByTeam := make(map[TeamID]uint64, len(w.pendingBuilds))
	for _, st := range w.pendingBuilds {
		if st.Team == 0 {
			continue
		}
		if cur, ok := earliestBuildByTeam[st.Team]; !ok || st.QueueOrder < cur {
			earliestBuildByTeam[st.Team] = st.QueueOrder
		}
	}
	rules := w.rulesMgr.Get()
	for team, pos := range activePosByTeam {
		st, ok := w.pendingBreaks[pos]
		if !ok {
			continue
		}
		if buildOrder, ok := earliestBuildByTeam[team]; ok && buildOrder < st.QueueOrder {
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

func (w *World) BlockNameByID(blockID int16) string {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if blockID <= 0 || len(w.blockNamesByID) == 0 {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(w.blockNamesByID[blockID]))
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

// EnsureTeamItems initializes team inventory if missing and returns a copy.
func (w *World) EnsureTeamItems(team TeamID) map[ItemID]int32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if team == 0 {
		return map[ItemID]int32{}
	}
	w.ensureTeamInventory(team)
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

func isCoreBlockName(name string) bool {
	n := strings.ToLower(strings.TrimSpace(name))
	if n == "core-zone" {
		return false
	}
	return strings.HasPrefix(n, "core-")
}

func isKnownCoreBlockID(blockID int16) bool {
	switch blockID {
	// Official content-id sequence from core/src/mindustry/content/Blocks.java
	case 316, 317, 318, 319, 320, 321:
		return true
	// Runtime/world-content sequence observed in 155.4 map streams.
	case 339, 340, 341, 342, 343, 344:
		return true
	default:
		return false
	}
}

func (w *World) refreshCoreBlockIDsLocked() {
	if w.coreBlockIDs == nil {
		w.coreBlockIDs = make(map[int16]struct{})
	} else {
		clear(w.coreBlockIDs)
	}
	for id, name := range w.blockNamesByID {
		if isCoreBlockName(name) {
			w.coreBlockIDs[id] = struct{}{}
		}
	}
	for id, name := range w.fallbackBlockNamesByID {
		if isCoreBlockName(name) {
			w.coreBlockIDs[id] = struct{}{}
		}
	}
}

func (w *World) isCoreBlockLocked(blockID int16, name string) bool {
	if isCoreBlockName(name) {
		return true
	}
	if _, ok := w.coreBlockIDs[blockID]; ok {
		return true
	}
	return isKnownCoreBlockID(blockID)
}

func unitBuildSpeedByName(name string) float32 {
	switch normalizeUnitName(name) {
	case "nova":
		return 0.3
	case "pulsar":
		return 0.5
	case "quasar":
		return 1.1
	case "vela":
		return 3.0
	case "poly":
		return 0.5
	case "mega":
		return 2.6
	case "quad":
		return 2.5
	case "oct":
		return 4.0
	case "retusa":
		return 1.5
	case "oxynoe":
		return 2.0
	case "cyerce":
		return 2.0
	case "aegires":
		return 3.0
	case "navanax":
		return 3.5
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
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t.Block <= 0 {
			continue
		}
		owner := t.Team
		if owner == 0 && t.Build != nil && t.Build.Team != 0 {
			owner = t.Build.Team
		}
		if owner != team {
			continue
		}
		name := w.blockNameByID(int16(t.Block))
		if w.isCoreBlockLocked(int16(t.Block), name) {
			out = append(out, packTilePos(t.X, t.Y))
		}
	}
	return out
}

// TileInfoByPackedPos returns block/team/core info for a packed point2 tile position.
func (w *World) TileInfoByPackedPos(pos int32) (blockID int16, team TeamID, isCore bool, ok bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return 0, 0, false, false
	}
	x, y := unpackTilePos(pos)
	if !w.model.InBounds(x, y) {
		return 0, 0, false, false
	}
	t := &w.model.Tiles[y*w.model.Width+x]
	if t == nil || t.Block <= 0 {
		return 0, 0, false, false
	}
	owner := t.Team
	if owner == 0 && t.Build != nil && t.Build.Team != 0 {
		owner = t.Build.Team
	}
	name := w.blockNameByID(int16(t.Block))
	return int16(t.Block), owner, w.isCoreBlockLocked(int16(t.Block), name), true
}

func (w *World) BuildSyncSnapshot() []BuildSyncState {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil || len(w.model.Tiles) == 0 {
		return nil
	}
	out := make([]BuildSyncState, 0, len(w.model.Tiles)/8)
	for i := range w.model.Tiles {
		t := w.model.Tiles[i]
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
		w.applyBuildPlanOpLocked(team, op, w.nextPlanOrder, addChanged)
	}
	return changed
}

// ApplyBuildPlanSnapshot applies client snapshot plans as queue updates.
// Vanilla does not remove absent plans here; queue removals come from explicit
// removeQueueBlock/deletePlans packets.
func (w *World) ApplyBuildPlanSnapshot(team TeamID, ops []BuildPlanOp) []int32 {
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

	for i, op := range ordered {
		w.applyBuildPlanOpLocked(team, op, uint64(i+1), addChanged)
	}
	return changed
}

func (w *World) applyBuildPlanOpLocked(team TeamID, op BuildPlanOp, queueOrder uint64, addChanged func(int32)) {
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
			if st.BlockID == int16(tile.Block) {
				st.QueueOrder = queueOrder
				w.pendingBreaks[pos] = st
				return
			}
		}
		if tile.Build == nil && tile.Block == 0 {
			delete(w.pendingBreaks, pos)
			return
		}
		maxHP := float32(1000)
		if tile.Build != nil && tile.Build.Health > 0 {
			maxHP = tile.Build.Health
		}
		w.pendingBreaks[pos] = pendingBreakState{
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
		if pending.BlockID == op.BlockID && pending.Team == team && pending.Rotation == op.Rotation {
			pending.QueueOrder = queueOrder
			w.pendingBuilds[pos] = pending
			return
		}
		w.refundBuildCost(pending.Team, pending.BlockID, 1.0)
	}
	if tile.Block == BlockID(op.BlockID) && tile.Team == team && tile.Rotation == op.Rotation && tile.Build != nil {
		delete(w.pendingBreaks, pos)
		delete(w.pendingBuilds, pos)
		return
	}
	if itemID, missingAmount, missing := w.firstMissingBuildCost(team, op.BlockID); missing {
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:        EntityEventBuildRejected,
			BuildPos:    packTilePos(tile.X, tile.Y),
			BuildTeam:   team,
			BuildBlock:  op.BlockID,
			BuildReason: "insufficient_items",
			ItemID:      itemID,
			ItemAmount:  missingAmount,
		})
		fmt.Printf("[buildtrace] reject plan xy=(%d,%d) block=%d team=%d reason=insufficient_items item=%d missing=%d\n",
			tile.X, tile.Y, op.BlockID, team, itemID, missingAmount)
		return
	}
	if !w.consumeBuildCost(team, op.BlockID) {
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:        EntityEventBuildRejected,
			BuildPos:    packTilePos(tile.X, tile.Y),
			BuildTeam:   team,
			BuildBlock:  op.BlockID,
			BuildReason: "insufficient_items",
		})
		fmt.Printf("[buildtrace] reject plan xy=(%d,%d) block=%d team=%d reason=insufficient_items\n", tile.X, tile.Y, op.BlockID, team)
		return
	}
	w.pendingBuilds[pos] = pendingBuildState{
		Team:       team,
		BlockID:    op.BlockID,
		Rotation:   op.Rotation,
		QueueOrder: queueOrder,
		Progress:   0,
	}
	delete(w.pendingBreaks, pos)
	addChanged(pos)
}

func (w *World) CancelBuildPlansPacked(positions []int32) {
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
		delete(w.pendingBreaks, pos)
	}
}

func (w *World) CancelBuildAt(x, y int32, breaking bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || !w.model.InBounds(int(x), int(y)) {
		return
	}
	pos := int32(int(y)*w.model.Width + int(x))
	if breaking {
		delete(w.pendingBreaks, pos)
		return
	}
	if st, ok := w.pendingBuilds[pos]; ok {
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
	for i := range w.model.Tiles {
		t := &w.model.Tiles[i]
		if t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		prof, ok := w.getBuildingWeaponProfile(int16(t.Build.Block))
		if !ok || prof.Damage <= 0 || prof.Interval <= 0 || prof.Range <= 0 {
			continue
		}

		pos := int32(i)
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
