package world

import (
	"encoding/json"
	"math"
	"math/rand"
	"os"
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

	blockNamesByID   map[int16]string
	unitNamesByID    map[int16]string
	unitTypeDefsByID map[int16]vanilla.UnitTypeDef
	buildStates      map[int32]buildCombatState
	pendingBuilds    map[int32]pendingBuildState
	unitMountCDs     map[int32][]float32
	unitTargets      map[int32]targetTrackState

	unitProfilesByType     map[int16]weaponProfile
	unitProfilesByName     map[string]weaponProfile
	buildingProfilesByName map[string]buildingWeaponProfile
	tileConfigValues       map[int32]any
	sorterRouteBits        map[int32]byte
	unloaderRotations      map[int32]int16
}

type BuildPlanOp struct {
	Breaking bool
	X        int32
	Y        int32
	Rotation int8
	BlockID  int16
}

type pendingBuildState struct {
	Team      TeamID
	BlockID   int16
	Rotation  int8
	Progress  float32
	LastEmit  float32
	Breaking  bool
	MaxHealth float32
}

type EntityEventKind string

const (
	EntityEventRemoved        EntityEventKind = "removed"
	EntityEventBuildPlaced    EntityEventKind = "build_placed"
	EntityEventBuildDestroyed EntityEventKind = "build_destroyed"
	EntityEventBuildHealth    EntityEventKind = "build_health"
	EntityEventBuildItems     EntityEventKind = "build_items"
	EntityEventBulletFired    EntityEventKind = "bullet_fired"
)

type EntityEvent struct {
	Kind   EntityEventKind
	Entity RawEntity
	// BuildPos is packed tile position (Point2), not linear tile index.
	BuildPos   int32
	BuildTeam  TeamID
	BuildBlock int16
	BuildRot   int8
	BuildHP    float32
	BuildItems []ItemStack
	Bullet     BulletEvent
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
		unitMountCDs:           map[int32][]float32{},
		unitTargets:            map[int32]targetTrackState{},
		unitProfilesByType:     cloneUnitWeaponProfiles(weaponProfilesByType),
		unitProfilesByName:     map[string]weaponProfile{},
		buildingProfilesByName: cloneBuildingWeaponProfiles(buildingWeaponProfilesByName),
		tileConfigValues:       map[int32]any{},
		sorterRouteBits:        map[int32]byte{},
		unloaderRotations:      map[int32]int16{},
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
	w.stepLogistics()
	w.stepEntities(delta)
}

func (w *World) stepLogistics() {
	if w.model == nil || len(w.model.Tiles) == 0 || w.blockNamesByID == nil {
		return
	}
	if w.tick%15 != 0 {
		return
	}
	moves := 0
	const maxMovesPerStep = 96
	for i := range w.model.Tiles {
		if moves >= maxMovesPerStep {
			break
		}
		t := &w.model.Tiles[i]
		if t == nil || t.Build == nil || t.Block == 0 {
			continue
		}
		name := w.blockNamesByID[int16(t.Block)]
		isUnloader := strings.Contains(name, "unloader")
		isSorter := strings.Contains(name, "sorter")
		isBridge := strings.Contains(name, "bridge") && (strings.Contains(name, "conveyor") || strings.Contains(name, "duct"))
		if !isUnloader && !isSorter && !isBridge {
			continue
		}
		if isUnloader {
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
		if isSorter {
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
		if isBridge {
			curPos := packTilePos(t.X, t.Y)
			dstPos, ok := w.configuredBuildPosForBuildLocked(curPos)
			validLink := ok && dstPos != curPos && w.bridgeLinkAllowed(name, curPos, dstPos)
			var dstTile *Tile
			if validLink {
				if dt, ok := w.tileForPosLocked(dstPos); ok && dt != nil && dt.Build != nil && dt.Block != 0 && dt.Team == t.Team && dt.Block == t.Block {
					dstTile = dt
				} else {
					validLink = false
				}
			}
			if !validLink {
				if outPos, moved, ok := w.bridgeDumpOneLocked(t); ok && moved > 0 {
					w.entityEvents = append(w.entityEvents, EntityEvent{
						Kind:       EntityEventBuildItems,
						BuildPos:   curPos,
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
			neighbors := w.adjacentBuildingsLocked(t.X, t.Y)
			var src *Tile
			var itemID int16
			for _, nb := range neighbors {
				if nb == nil || nb.Build == nil || nb.Team != t.Team || (nb.X == dstTile.X && nb.Y == dstTile.Y) {
					continue
				}
				for _, st := range nb.Build.Items {
					if st.Amount > 0 {
						src = nb
						itemID = int16(st.Item)
						break
					}
				}
				if src != nil {
					break
				}
			}
			if src == nil || itemID <= 0 {
				continue
			}
			taken := w.removeBuildingItemLocked(packTilePos(src.X, src.Y), itemID, 1)
			if taken <= 0 {
				continue
			}
			added := w.acceptBuildingItemLocked(packTilePos(dstTile.X, dstTile.Y), itemID, taken)
			if added < taken {
				_ = w.acceptBuildingItemLocked(packTilePos(src.X, src.Y), itemID, taken-added)
			}
			if added <= 0 {
				continue
			}
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:       EntityEventBuildItems,
				BuildPos:   packTilePos(src.X, src.Y),
				BuildItems: append([]ItemStack(nil), src.Build.Items...),
			})
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:       EntityEventBuildItems,
				BuildPos:   packTilePos(dstTile.X, dstTile.Y),
				BuildItems: append([]ItemStack(nil), dstTile.Build.Items...),
			})
			moves++
		}
	}
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

func (w *World) adjacentBuildingsLocked(x, y int) []*Tile {
	if w.model == nil {
		return nil
	}
	out := make([]*Tile, 0, 4)
	dirs := [][2]int{{1, 0}, {-1, 0}, {0, 1}, {0, -1}}
	for _, d := range dirs {
		nx, ny := x+d[0], y+d[1]
		if !w.model.InBounds(nx, ny) {
			continue
		}
		t, err := w.model.TileAt(nx, ny)
		if err != nil || t == nil || t.Build == nil || t.Block == 0 {
			continue
		}
		out = append(out, t)
	}
	return out
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
		dir, okDir := directionFromSourceToSorter(src.X, src.Y, sorter.X, sorter.Y)
		if !okDir {
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
	for _, b := range neighbors {
		if b == nil || b.Build == nil || b.Team != unloader.Team {
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
	for _, b := range neighbors {
		if b == nil || b.Build == nil {
			continue
		}
		if w.buildingItemAmountLocked(b, itemID) > 0 {
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
	return buildingItemCapacityByName[name]
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
	dx, dy := dirToDelta(dir)
	nx, ny := t.X+dx, t.Y+dy
	if !w.model.InBounds(nx, ny) {
		return nil
	}
	tt, err := w.model.TileAt(nx, ny)
	if err != nil || tt == nil || tt.Build == nil || tt.Block == 0 {
		return nil
	}
	return tt
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
	switch {
	case strings.Contains(name, "phase-conveyor"):
		maxRange = int32(12)
	case strings.Contains(name, "bridge-conveyor"):
		maxRange = int32(4)
	case strings.Contains(name, "duct-bridge"):
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
	w.tileConfigValues = map[int32]any{}
	w.sorterRouteBits = map[int32]byte{}
	w.unloaderRotations = map[int32]int16{}
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
	// Keep construction/deconstruction visibly progressive and smooth.
	progressPerSec := float32(1.25)
	for pos, st := range w.pendingBuilds {
		st.Progress += dt * progressPerSec
		if st.Progress > 1 {
			st.Progress = 1
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

// ApplyBuildPlans applies simplified build/break operations from client plans.
// It updates server world state and returns changed tile positions.
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
				w.pendingBuilds[pos] = pendingBuildState{
					Team:      tile.Team,
					BlockID:   int16(tile.Block),
					Rotation:  tile.Rotation,
					Progress:  0,
					LastEmit:  0,
					Breaking:  true,
					MaxHealth: maxHP,
				}
				w.entityEvents = append(w.entityEvents, EntityEvent{
					Kind:     EntityEventBuildHealth,
					BuildPos: packTilePos(tile.X, tile.Y),
					BuildHP:  tile.Build.Health,
				})
				addChanged(pos)
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
			Team:      team,
			BlockID:   op.BlockID,
			Rotation:  op.Rotation,
			Progress:  0,
			LastEmit:  0,
			Breaking:  false,
			MaxHealth: maxHP,
		}
		addChanged(pos)
	}
	return changed
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
	accepted := w.acceptBuildingItemAmountLocked(buildPos, itemID, amount)
	if accepted <= 0 {
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
	b.AddItem(ItemID(itemID), accepted)
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
	capacity := int32(0)
	if w.blockNamesByID != nil {
		if name, ok := w.blockNamesByID[int16(t.Block)]; ok {
			capacity = buildingItemCapacityByName[strings.ToLower(strings.TrimSpace(name))]
		}
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
