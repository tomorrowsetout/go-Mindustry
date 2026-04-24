package world

import (
	"encoding/json"
	"fmt"
	"log"
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

type completedBuildingPlacement struct {
	Config            any
	SelfConfigTargets []int32
	ChangedConfigs    []powerAutoLinkChange
}

type Config struct {
	TPS                    int
	UseMapSyncDataFallback bool
	BlockSyncLogsEnabled   bool
}

type itemLogisticsPerf struct {
	Junctions time.Duration
	Conveyor  time.Duration
	Duct      time.Duration
	Router    time.Duration
	Bridge    time.Duration
	Unloader  time.Duration
	MassDrive time.Duration

	JunctionCount  int
	ConveyorCount  int
	DuctCount      int
	RouterCount    int
	BridgeCount    int
	UnloaderCount  int
	MassDriveCount int
}

type entitySpatialIndex struct {
	cellSize int
	cells    map[int64][]int
}

type buildingSpatialIndex struct {
	cellSize int
	cells    map[int64][]int32
}

type World struct {
	mu sync.RWMutex

	wave     int32
	waveTime float32
	tick     uint64
	timeSec  float32

	rand0 int64
	rand1 int64

	tps       int8
	actualTps int8

	tpsWindowStart time.Time
	tpsWindowTicks int32
	perfLogAt      time.Time

	start time.Time

	model *WorldModel

	// 规则和波次管理器
	rulesMgr *RulesManager
	wavesMgr *WaveManager

	// 同步配置
	useMapSyncDataFallback bool
	blockSyncLogsEnabled   bool

	entityEvents      []EntityEvent
	bullets           []simBullet
	pendingMountShots []pendingMountShot
	bulletNextID      int32
	blockItemSyncTick map[int32]uint64

	blockNamesByID              map[int16]string
	unitNamesByID               map[int16]string
	unitTypeDefsByID            map[int16]vanilla.UnitTypeDef
	buildStates                 map[int32]buildCombatState
	controlledBuilds            map[int32]controlledBuildState
	controlledBuildByPlayer     map[int32]int32
	pendingBuilds               map[int32]pendingBuildState
	pendingBreaks               map[int32]pendingBreakState
	buildRejectLogTick          map[int32]uint64
	builderStates               map[int32]builderRuntimeState
	teamRebuildPlans            map[TeamID][]rebuildBlockPlan
	teamAIBuildPlans            map[TeamID][]teamBuildPlan
	teamBuildAIStates           map[TeamID]buildAIPlannerState
	buildAIParts                []buildAIBasePart
	buildAIPartsLoaded          bool
	factoryStates               map[int32]factoryState
	reconstructorStates         map[int32]reconstructorState
	drillStates                 map[int32]drillRuntimeState
	burstDrillStates            map[int32]burstDrillRuntimeState
	beamDrillStates             map[int32]beamDrillRuntimeState
	pumpStates                  map[int32]pumpRuntimeState
	crafterStates               map[int32]crafterRuntimeState
	heatStates                  map[int32]float32
	incineratorStates           map[int32]float32
	repairTurretStates          map[int32]repairTurretRuntimeState
	repairTowerStates           map[int32]repairTowerRuntimeState
	teamPowerStates             map[TeamID]*teamPowerState
	teamPowerBudget             map[TeamID]float32
	powerNetStates              map[int32]*powerNetState
	powerNetByPos               map[int32]int32
	powerNetDirty               bool
	powerStorageState           map[int32]float32
	powerRequested              map[int32]float32
	powerSupplied               map[int32]float32
	powerGeneratorState         map[int32]*powerGeneratorState
	unitMountCDs                map[int32][]float32
	unitMountStates             map[int32][]unitMountState
	unitTargets                 map[int32]targetTrackState
	unitAIStates                map[int32]unitAIState
	unitMiningStates            map[int32]unitMiningState
	teamItems                   map[TeamID]map[ItemID]int32
	teamBuilderSpeed            map[TeamID]float32
	itemSourceCfg               map[int32]ItemID
	liquidSourceCfg             map[int32]LiquidID
	sorterCfg                   map[int32]ItemID
	unloaderCfg                 map[int32]ItemID
	payloadRouterCfg            map[int32]protocol.Content
	powerNodeLinks              map[int32][]int32
	bridgeLinks                 map[int32]int32
	massDriverLinks             map[int32]int32
	payloadDriverLinks          map[int32]int32
	bridgeBuffers               map[int32][]bufferedBridgeItem
	bridgeAcceptAcc             map[int32]float32
	conveyorStates              map[int32]*conveyorRuntimeState
	ductStates                  map[int32]*ductRuntimeState
	routerStates                map[int32]*routerRuntimeState
	stackStates                 map[int32]*stackRuntimeState
	massDriverStates            map[int32]*massDriverRuntimeState
	payloadStates               map[int32]*payloadRuntimeState
	payloadDeconstructorStates  map[int32]*payloadDeconstructorState
	payloadDriverStates         map[int32]*payloadDriverRuntimeState
	massDriverShots             []massDriverShot
	payloadDriverShots          []payloadDriverShot
	blockDumpIndex              map[int32]int
	dumpNeighborCache           map[int32][]int32
	unloaderLastUsed            map[int64]int
	itemSourceAccum             map[int32]float32
	routerInputPos              map[int32]int32
	routerRotation              map[int32]byte
	transportAccum              map[int32]float32
	junctionQueues              map[int32]junctionQueueState
	bridgeIncomingMask          map[int32]byte
	reactorStates               map[int32]nuclearReactorState
	storageLinkedCore           map[int32]int32
	teamPrimaryCore             map[TeamID]int32
	coreStorageCapacity         map[int32]int32
	blockOccupancy              map[int32]int32
	activeTilePositions         []int32
	itemLogisticsTilePositions  []int32
	crafterTilePositions        []int32
	drillTilePositions          []int32
	burstDrillTilePositions     []int32
	beamDrillTilePositions      []int32
	pumpTilePositions           []int32
	incineratorTilePositions    []int32
	repairTurretTilePositions   []int32
	repairTowerTilePositions    []int32
	factoryTilePositions        []int32
	heatConductorTilePositions  []int32
	powerTilePositions          []int32
	powerDiodeTilePositions     []int32
	powerVoidTilePositions      []int32
	teamBuildingTiles           map[TeamID][]int32
	teamBuildingSpatial         map[TeamID]*buildingSpatialIndex
	teamCoreTiles               map[TeamID][]int32
	teamPowerTiles              map[TeamID][]int32
	teamPowerNodeTiles          map[TeamID][]int32
	turretTilePositions         []int32
	turretStates                map[int32]*turretRuntimeState
	mendProjectorPositions      []int32
	mendProjectorStates         map[int32]*mendProjectorState
	overdriveProjectorPositions []int32
	overdriveProjectorStates    map[int32]*overdriveProjectorState
	forceProjectorPositions     []int32
	forceProjectorStates        map[int32]*forceProjectorState
	nextPlanOrder               uint64

	unitProfilesByType        map[int16]weaponProfile
	unitProfilesByName        map[string]weaponProfile
	unitRuntimeProfilesByName map[string]unitRuntimeProfile
	unitMountProfilesByName   map[string][]unitWeaponMountProfile
	buildingProfilesByName    map[string]buildingWeaponProfile
	blockCostsByName          map[string][]ItemStack
	blockBuildTimesByName     map[string]float32
	blockArmorByName          map[string]float32
	statusProfilesByID        map[int16]statusEffectProfile
	statusProfilesByName      map[string]statusEffectProfile
}

const (
	// Vars.buildingRange in Mindustry 156.
	vanillaBuilderRange = 220.0
	// Builder state comes from clientSnapshot and should stop driving progress
	// quickly once snapshots stop arriving.
)

type BuildPlanOp struct {
	Breaking bool
	X        int32
	Y        int32
	Rotation int8
	BlockID  int16
	Config   any
}

type RotateBuildingResult struct {
	BlockID   int16
	Rotation  int8
	Team      TeamID
	EffectX   float32
	EffectY   float32
	EffectRot float32
}

type BuildingInfo struct {
	Pos      int32
	X        int32
	Y        int32
	BlockID  int16
	Name     string
	Team     TeamID
	Rotation int8
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
	Owner            int32
	Team             TeamID
	BlockID          int16
	Rotation         int8
	Config           any
	QueueOrder       uint64
	Progress         float32
	VisualPlaced     bool
	LastHP           float32
	BuildCost        []ItemStack
	ItemsLeft        []int32
	Accumulator      []float32
	TotalAccumulator []float32
}

type builderRuntimeState struct {
	Owner      int32
	Team       TeamID
	UnitID     int32
	X          float32
	Y          float32
	Active     bool
	BuildRange float32
	UpdatedAt  time.Time
}

type rebuildBlockPlan struct {
	X        int32
	Y        int32
	Rotation int8
	BlockID  int16
	Config   any
}

type buildAIPlannerState struct {
	PlanScanCD     float32
	SpawnCD        float32
	RefreshPathCD  float32
	StartedPathing bool
	FoundPath      bool
	PathCells      map[int32]struct{}
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
	RefundTeam  TeamID
	RefundCost  []ItemStack
	RefundAccum map[ItemID]float32
	RefundTotal map[ItemID]float32
	Refunded    map[ItemID]int32
}

const constructBlockHealthMax = float32(10)

type factoryState struct {
	Progress    float32
	UnitType    int16
	CurrentPlan int16
	CommandPos  *protocol.Vec2
	Command     *protocol.UnitCommand
}

type drillRuntimeState struct {
	Progress float32
	Warmup   float32
}

type burstDrillRuntimeState struct {
	Progress float32
	Warmup   float32
}

type beamDrillRuntimeState struct {
	Time   float32
	Warmup float32
}

type repairTurretRuntimeState struct {
	Rotation       float32
	Strength       float32
	SearchProgress float32
	TargetID       int32
}

type repairTowerRuntimeState struct {
	Refresh       float32
	Warmup        float32
	TotalProgress float32
	Targets       []int32
}

type pumpRuntimeState struct {
	Warmup      float32
	Progress    float32
	Accumulator float32
}

type crafterRuntimeState struct {
	Progress      float32
	Warmup        float32
	TotalProgress float32
	Seed          uint32
}

type nuclearReactorState struct {
	Heat         float32
	HeatProgress float32
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
	Rotation   int8
	Serialized []byte
	Config     []byte
	Items      []ItemStack
	Liquids    []LiquidStack
	Power      float32
	Health     float32
	MaxHealth  float32
	UnitState  *RawEntity
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

type unloaderCandidateStat struct {
	pos        int32
	loadFactor float32
	canLoad    bool
	canUnload  bool
	notStorage bool
	lastUsed   int
}

type protocolContentLiquid LiquidID

func (l protocolContentLiquid) ContentType() protocol.ContentType { return protocol.ContentLiquid }
func (l protocolContentLiquid) ID() int16                         { return int16(l) }
func (l protocolContentLiquid) Name() string                      { return "" }

type EntityEventKind string

const (
	EntityEventRemoved             EntityEventKind = "removed"
	EntityEventBuildPlaced         EntityEventKind = "build_placed"
	EntityEventBuildConstructed    EntityEventKind = "build_constructed"
	EntityEventBuildConfig         EntityEventKind = "build_config"
	EntityEventBuildDeconstructing EntityEventKind = "build_deconstructing"
	EntityEventBuildCancelled      EntityEventKind = "build_cancelled"
	EntityEventBuildDestroyed      EntityEventKind = "build_destroyed"
	EntityEventBuildHealth         EntityEventKind = "build_health"
	EntityEventTeamItems           EntityEventKind = "team_items"
	EntityEventBlockItemSync       EntityEventKind = "block_item_sync"
	EntityEventItemTurretAmmoSync  EntityEventKind = "item_turret_ammo_sync"
	EntityEventTransferItemToUnit  EntityEventKind = "transfer_item_to_unit"
	EntityEventTransferItemToBuild EntityEventKind = "transfer_item_to_build"
	EntityEventBulletFired         EntityEventKind = "bullet_fired"
	EntityEventEffect              EntityEventKind = "effect"
)

type EntityEvent struct {
	Kind   EntityEventKind
	Entity RawEntity
	// BuildPos is packed tile position (Point2), not linear tile index.
	BuildPos    int32
	BuildOwner  int32
	BuildTeam   TeamID
	BuildBlock  int16
	BuildRot    int8
	BuildConfig any
	BuildHP     float32
	ItemID      ItemID
	ItemAmount  int32
	UnitID      int32
	TransferX   float32
	TransferY   float32
	Bullet      BulletEvent
	EffectName  string
	EffectX     float32
	EffectY     float32
	EffectRot   float32
}

func (w *World) appendBuildConfigEventLocked(pos int32) {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 {
		return
	}
	cfg, ok := w.normalizedBuildingConfigLocked(pos)
	if !ok {
		if tile.Build == nil || len(tile.Build.Config) == 0 {
			return
		}
		var decoded any
		decoded, ok = decodeStoredBuildingConfig(tile.Build.Config)
		if !ok {
			return
		}
		cfg = decoded
	}
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:        EntityEventBuildConfig,
		BuildPos:    packTilePos(tile.X, tile.Y),
		BuildTeam:   tile.Build.Team,
		BuildBlock:  int16(tile.Block),
		BuildRot:    tile.Rotation,
		BuildConfig: cfg,
	})
}

func (w *World) appendBuildConfigValueEventLocked(pos int32, value any) {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 {
		return
	}
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:        EntityEventBuildConfig,
		BuildPos:    packTilePos(tile.X, tile.Y),
		BuildTeam:   tile.Build.Team,
		BuildBlock:  int16(tile.Block),
		BuildRot:    tile.Rotation,
		BuildConfig: value,
	})
}

func (w *World) emitBlockItemSyncLocked(pos int32) {
	if w == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	if _, _, _, ok := w.sharedCoreInventoryLocked(pos); ok {
		return
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil || tile.Block == 0 || tile.Build.Team == 0 {
		return
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	kind := w.classifyBlockSyncKindLocked(pos, tile, name)
	if kind == blockSyncNone {
		return
	}
	shouldSync := w.hasItemModuleForBlockSyncLocked(tile, name, kind)
	switch kind {
	case blockSyncUnitFactory, blockSyncReconstructor:
		shouldSync = true
	}
	// Mindustry-157 only syncs turrets through the shared blockSnapshot pass.
	// Pushing item-turret ammo changes through this extra event path creates a
	// second snapshot writer for the same build state and can rewind ammo views.
	if kind == blockSyncItemTurret {
		if lastTick, ok := w.blockItemSyncTick[pos]; ok && lastTick == w.tick {
			return
		}
		w.blockItemSyncTick[pos] = w.tick
		if w.blockSyncLogsEnabled {
			log.Printf("[turret-ammo] enqueue-sync pos=%d (%d,%d) block=%s tileTeam=%d buildTeam=%d tick=%d stacks=%s",
				pos, tile.X, tile.Y, name, tile.Team, tile.Build.Team, w.tick, w.debugItemStacksLocked(tile.Build.Items))
		}
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:       EntityEventItemTurretAmmoSync,
			BuildPos:   packTilePos(tile.X, tile.Y),
			BuildTeam:  tile.Build.Team,
			BuildBlock: int16(tile.Block),
		})
		return
	}
	if isPayloadProcessorBlockSyncKind(kind) {
		shouldSync = true
	}
	if !shouldSync {
		return
	}
	if lastTick, ok := w.blockItemSyncTick[pos]; ok && lastTick == w.tick {
		return
	}
	w.blockItemSyncTick[pos] = w.tick
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventBlockItemSync,
		BuildPos:   packTilePos(tile.X, tile.Y),
		BuildTeam:  tile.Build.Team,
		BuildBlock: int16(tile.Block),
	})
}

func itemStacksEqualByItem(a, b []ItemStack) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	counts := make(map[ItemID]int32, len(a)+len(b))
	nonZero := 0
	for _, stack := range a {
		if stack.Amount <= 0 {
			continue
		}
		counts[stack.Item] += stack.Amount
		nonZero++
	}
	for _, stack := range b {
		if stack.Amount <= 0 {
			continue
		}
		counts[stack.Item] -= stack.Amount
		nonZero++
	}
	if nonZero == 0 {
		return true
	}
	for _, amount := range counts {
		if amount != 0 {
			return false
		}
	}
	return true
}

func (w *World) replaceBuildingItemsLocked(pos int32, tile *Tile, items []ItemStack) {
	if tile == nil || tile.Build == nil {
		return
	}
	if itemStacksEqualByItem(tile.Build.Items, items) {
		return
	}
	tile.Build.Items = cloneItemStacks(items)
	w.emitBlockItemSyncLocked(pos)
}

func (w *World) invalidateItemRoutingCachesLocked() {
	w.dumpNeighborCache = map[int32][]int32{}
	w.bridgeIncomingMask = map[int32]byte{}
}

func (w *World) debugItemStacksLocked(stacks []ItemStack) string {
	if len(stacks) == 0 {
		return "[]"
	}
	var b strings.Builder
	b.WriteByte('[')
	for i, stack := range stacks {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(strconv.Itoa(int(stack.Item)))
		b.WriteByte(':')
		b.WriteString(strconv.Itoa(int(stack.Amount)))
	}
	b.WriteByte(']')
	return b.String()
}

func (w *World) BlockSyncLogsEnabled() bool {
	if w == nil {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.blockSyncLogsEnabled
}

func (w *World) DebugItemTurretAmmoPacked(packedPos int32) string {
	if w == nil {
		return "world=nil"
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil {
		return "model=nil"
	}
	pos, ok := w.tileIndexFromPackedPosLocked(packedPos)
	if !ok || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return fmt.Sprintf("packed=%d missing", packedPos)
	}
	tile := &w.model.Tiles[pos]
	if tile == nil {
		return fmt.Sprintf("packed=%d tile=nil", packedPos)
	}
	if tile.Build == nil || tile.Block == 0 {
		return fmt.Sprintf("packed=%d (%d,%d) block=%d build=nil tileTeam=%d", packedPos, tile.X, tile.Y, tile.Block, tile.Team)
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Block))))
	totalAmmo := int32(0)
	if prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block)); ok && w.buildingUsesItemAmmoLocked(tile, prof) {
		totalAmmo = w.totalBuildingAmmoLocked(tile, prof)
	}
	return fmt.Sprintf("packed=%d (%d,%d) block=%s tileTeam=%d buildTeam=%d totalAmmo=%d stacks=%s",
		packedPos, tile.X, tile.Y, name, tile.Team, tile.Build.Team, totalAmmo, w.debugItemStacksLocked(tile.Build.Items))
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
	ID                   int32
	Team                 TeamID
	X                    float32
	Y                    float32
	VX                   float32
	VY                   float32
	Damage               float32
	SplashDamage         float32
	LifeSec              float32
	AgeSec               float32
	Radius               float32
	HitUnits             bool
	HitBuilds            bool
	BulletType           int16
	BulletClass          string
	SplashRadius         float32
	BuildingDamage       float32
	ArmorMultiplier      float32
	MaxDamageFraction    float32
	ShieldDamageMul      float32
	PierceDamageFactor   float32
	PierceArmor          bool
	SlowSec              float32
	SlowMul              float32
	PierceRemain         int32
	PierceBuilding       bool
	ChainCount           int32
	ChainRange           float32
	FragmentCount        int32
	FragmentSpread       float32
	FragmentSpeed        float32
	FragmentLife         float32
	FragmentRand         float32
	FragmentAngle        float32
	FragmentVelMin       float32
	FragmentVelMax       float32
	FragmentLifeMin      float32
	FragmentLifeMax      float32
	FragmentBullet       *bulletRuntimeProfile
	StatusID             int16
	StatusName           string
	StatusDuration       float32
	ShootEffect          string
	SmokeEffect          string
	HitEffect            string
	DespawnEffect        string
	TargetAir            bool
	TargetGround         bool
	TargetPriority       string
	HelixScl             float32
	HelixMag             float32
	HelixOffset          float32
	AimX                 float32
	AimY                 float32
	KeepAlive            bool
	DamageTick           float32
	BeamLength           float32
	BeamDamageInterval   float32
	BeamOptimalLifeFract float32
	BeamFadeTime         float32
}

type weaponProfile struct {
	FireMode             string // projectile|beam
	Range                float32
	Damage               float32
	SplashDamage         float32
	Interval             float32
	BulletType           int16
	BulletClass          string
	BulletSpeed          float32
	BulletLifetime       float32
	BulletHitSize        float32
	SplashRadius         float32
	BuildingDamage       float32
	ArmorMultiplier      float32
	MaxDamageFraction    float32
	ShieldDamageMul      float32
	PierceDamageFactor   float32
	PierceArmor          bool
	SlowSec              float32
	SlowMul              float32
	Pierce               int32
	PierceBuilding       bool
	ChainCount           int32
	ChainRange           float32
	StatusID             int16
	StatusName           string
	StatusDuration       float32
	ShootStatusID        int16
	ShootStatusName      string
	ShootStatusDuration  float32
	FragmentCount        int32
	FragmentSpread       float32
	FragmentSpeed        float32
	FragmentLife         float32
	FragmentRandomSpread float32
	FragmentAngle        float32
	FragmentVelocityMin  float32
	FragmentVelocityMax  float32
	FragmentLifeMin      float32
	FragmentLifeMax      float32
	FragmentBullet       *bulletRuntimeProfile
	ShootEffect          string
	SmokeEffect          string
	HitEffect            string
	DespawnEffect        string
	PreferBuildings      bool
	TargetAir            bool
	TargetGround         bool
	TargetPriority       string
	HitBuildings         bool
}

type buildingWeaponProfile struct {
	ClassName            string
	FireMode             string // projectile|beam
	Range                float32
	Damage               float32
	SplashDamage         float32
	Interval             float32
	BulletType           int16
	BulletClass          string
	BulletSpeed          float32
	BulletLifetime       float32
	BulletHitSize        float32
	SplashRadius         float32
	BuildingDamage       float32
	ArmorMultiplier      float32
	MaxDamageFraction    float32
	ShieldDamageMul      float32
	PierceDamageFactor   float32
	PierceArmor          bool
	SlowSec              float32
	SlowMul              float32
	Pierce               int32
	PierceBuilding       bool
	ChainCount           int32
	ChainRange           float32
	StatusID             int16
	StatusName           string
	StatusDuration       float32
	FragmentCount        int32
	FragmentSpread       float32
	FragmentSpeed        float32
	FragmentLife         float32
	FragmentRandomSpread float32
	FragmentAngle        float32
	FragmentVelocityMin  float32
	FragmentVelocityMax  float32
	FragmentLifeMin      float32
	FragmentLifeMax      float32
	FragmentBullet       *bulletRuntimeProfile
	Bullet               *bulletRuntimeProfile
	ShootEffect          string
	SmokeEffect          string
	HitEffect            string
	DespawnEffect        string
	HitBuildings         bool
	TargetBuilds         bool
	TargetAir            bool
	TargetGround         bool
	TargetPriority       string
	MinTargetTeam        TeamID
	Rotate               bool
	RotateSpeed          float32
	BaseRotation         float32
	PredictTarget        bool
	TargetInterval       float32
	TargetSwitchInterval float32
	ShootCone            float32
	RotationLimit        float32

	AmmoCapacity float32
	AmmoRegen    float32
	AmmoPerShot  float32

	PowerCapacity float32
	PowerRegen    float32
	PowerPerShot  float32

	BurstShots   int32
	BurstSpacing float32

	ContinuousHold bool
	AimChangeSpeed float32
	ShootDuration  float32
}

type buildCombatState struct {
	Cooldown       float32
	BurstRemain    int32
	BurstDelay     float32
	Ammo           float32
	Power          float32
	TargetID       int32
	RetargetCD     float32
	TurretRotation float32
	HasRotation    bool
	BeamBulletID   int32
	BeamHoldRemain float32
	BeamLastLength float32
}

type targetTrackState struct {
	TargetID   int32
	RetargetCD float32
}

type pendingMountShot struct {
	EntityID    int32
	MountIndex  int
	DelaySec    float32
	XOffset     float32
	YOffset     float32
	AngleOffset float32
	HelixScl    float32
	HelixMag    float32
	HelixOffset float32
}

type unitWeaponMountProfile struct {
	AngleOffset     float32
	CooldownMul     float32
	DamageMul       float32
	RangeMul        float32
	BulletSpeedMul  float32
	BulletType      int16 // -1 means inherit entity bullet type
	SplashRadiusMul float32

	ClassName            string
	FireMode             string
	Range                float32
	Damage               float32
	SplashDamage         float32
	Interval             float32
	BulletClass          string
	BulletSpeed          float32
	BulletLifetime       float32
	BulletHitSize        float32
	SplashRadius         float32
	BuildingDamage       float32
	ArmorMultiplier      float32
	MaxDamageFraction    float32
	ShieldDamageMul      float32
	PierceDamageFactor   float32
	PierceArmor          bool
	SlowSec              float32
	SlowMul              float32
	Pierce               int32
	PierceBuilding       bool
	ChainCount           int32
	ChainRange           float32
	StatusID             int16
	StatusName           string
	StatusDuration       float32
	ShootStatusID        int16
	ShootStatusName      string
	ShootStatusDuration  float32
	FragmentCount        int32
	FragmentSpread       float32
	FragmentSpeed        float32
	FragmentLife         float32
	FragmentRandomSpread float32
	FragmentAngle        float32
	FragmentVelocityMin  float32
	FragmentVelocityMax  float32
	FragmentLifeMin      float32
	FragmentLifeMax      float32
	FragmentBullet       *bulletRuntimeProfile
	ShootEffect          string
	SmokeEffect          string
	HitEffect            string
	DespawnEffect        string
	PreferBuildings      bool
	TargetAir            bool
	TargetGround         bool
	TargetPriority       string
	HitBuildings         bool
	X                    float32
	Y                    float32
	ShootX               float32
	ShootY               float32
	Rotate               bool
	RotateSpeed          float32
	BaseRotation         float32
	Mirror               bool
	Alternate            bool
	FlipSprite           bool
	OtherSide            int32
	Controllable         bool
	AIControllable       bool
	AutoTarget           bool
	PredictTarget        bool
	UseAttackRange       bool
	AlwaysShooting       bool
	NoAttack             bool
	TargetInterval       float32
	TargetSwitchInterval float32
	ShootCone            float32
	MinShootVelocity     float32
	Inaccuracy           float32
	VelocityRnd          float32
	XRand                float32
	YRand                float32
	ExtraVelocity        float32
	RotationLimit        float32
	MinWarmup            float32
	ShootWarmupSpeed     float32
	LinearWarmup         bool
	AimChangeSpeed       float32
	Continuous           bool
	AlwaysContinuous     bool
	PointDefense         bool
	RepairBeam           bool
	TargetUnits          bool
	TargetBuildings      bool
	RepairSpeed          float32
	FractionRepairSpeed  float32
	ShootPattern         string
	ShootShots           int32
	ShootFirstShotDelay  float32
	ShootShotDelay       float32
	ShootSpread          float32
	ShootBarrels         int32
	ShootBarrelOffset    int32
	ShootPatternMirror   bool
	ShootHelixScl        float32
	ShootHelixMag        float32
	ShootHelixOffset     float32
	HitRadius            float32
	Bullet               *bulletRuntimeProfile
}

type unitMountState struct {
	Reload         float32
	Rotation       float32
	TargetRotation float32
	AimX           float32
	AimY           float32
	Side           bool
	Warmup         float32
	BarrelCounter  int32
	TargetID       int32
	TargetBuildPos int32
	RetargetCD     float32
	BeamBulletID   int32
	LastBeamLength float32
}

type vanillaProfilesFile struct {
	Units       []vanillaUnitProfile   `json:"units"`
	UnitsByName []vanillaUnitProfile   `json:"units_by_name"`
	Turrets     []vanillaTurretProfile `json:"turrets"`
	Blocks      []vanillaBlockProfile  `json:"blocks"`
	Statuses    []vanillaStatusProfile `json:"statuses"`
}

type vanillaBlockRequirement struct {
	Item   string  `json:"item"`
	ItemID int16   `json:"item_id"`
	Amount int32   `json:"amount"`
	Cost   float32 `json:"cost"`
}

type vanillaBlockProfile struct {
	Name                string                    `json:"name"`
	Armor               float32                   `json:"armor"`
	BuildCostMultiplier float32                   `json:"build_cost_multiplier"`
	BuildTimeSec        float32                   `json:"build_time_sec"`
	Requirements        []vanillaBlockRequirement `json:"requirements"`
}

type vanillaUnitProfile struct {
	Name                     string                      `json:"name"`
	TypeID                   int16                       `json:"type_id"`
	Health                   float32                     `json:"health"`
	Armor                    float32                     `json:"armor"`
	Speed                    float32                     `json:"speed"`
	HitSize                  float32                     `json:"hit_size"`
	RotateSpeed              float32                     `json:"rotate_speed"`
	BuildSpeed               float32                     `json:"build_speed"`
	MineSpeed                float32                     `json:"mine_speed"`
	MineTier                 int16                       `json:"mine_tier"`
	ItemCapacity             int32                       `json:"item_capacity"`
	AmmoCapacity             float32                     `json:"ammo_capacity"`
	AmmoRegen                float32                     `json:"ammo_regen"`
	AmmoPerShot              float32                     `json:"ammo_per_shot"`
	PayloadCapacity          float32                     `json:"payload_capacity"`
	Flying                   bool                        `json:"flying"`
	LowAltitude              bool                        `json:"low_altitude"`
	CanBoost                 bool                        `json:"can_boost"`
	MineWalls                bool                        `json:"mine_walls"`
	MineFloor                bool                        `json:"mine_floor"`
	CoreUnitDock             bool                        `json:"core_unit_dock"`
	AllowedInPayloads        bool                        `json:"allowed_in_payloads"`
	PickupUnits              bool                        `json:"pickup_units"`
	FireMode                 string                      `json:"fire_mode"`
	Range                    float32                     `json:"range"`
	Damage                   float32                     `json:"damage"`
	SplashDamage             float32                     `json:"splash_damage"`
	Interval                 float32                     `json:"interval"`
	BulletType               int16                       `json:"bullet_type"`
	BulletSpeed              float32                     `json:"bullet_speed"`
	BulletLifetime           float32                     `json:"bullet_lifetime"`
	BulletHitSize            float32                     `json:"bullet_hit_size"`
	SplashRadius             float32                     `json:"splash_radius"`
	BuildingDamageMultiplier float32                     `json:"building_damage_multiplier"`
	ArmorMultiplier          float32                     `json:"armor_multiplier"`
	MaxDamageFraction        float32                     `json:"max_damage_fraction"`
	ShieldDamageMultiplier   float32                     `json:"shield_damage_multiplier"`
	PierceDamageFactor       float32                     `json:"pierce_damage_factor"`
	PierceArmor              bool                        `json:"pierce_armor"`
	SlowSec                  float32                     `json:"slow_sec"`
	SlowMul                  float32                     `json:"slow_mul"`
	Pierce                   int32                       `json:"pierce"`
	PierceBuilding           bool                        `json:"pierce_building"`
	ChainCount               int32                       `json:"chain_count"`
	ChainRange               float32                     `json:"chain_range"`
	StatusID                 int16                       `json:"status_id"`
	StatusName               string                      `json:"status_name"`
	StatusDuration           float32                     `json:"status_duration"`
	ShootStatusID            int16                       `json:"shoot_status_id"`
	ShootStatusName          string                      `json:"shoot_status_name"`
	ShootStatusDuration      float32                     `json:"shoot_status_duration"`
	FragmentCount            int32                       `json:"frag_bullets"`
	FragmentSpread           float32                     `json:"frag_spread"`
	FragmentSpeed            float32                     `json:"fragment_speed"`
	FragmentLife             float32                     `json:"fragment_life"`
	FragmentRandomSpread     float32                     `json:"frag_random_spread"`
	FragmentAngle            float32                     `json:"frag_angle"`
	FragmentVelocityMin      float32                     `json:"frag_velocity_min"`
	FragmentVelocityMax      float32                     `json:"frag_velocity_max"`
	FragmentLifeMin          float32                     `json:"frag_life_min"`
	FragmentLifeMax          float32                     `json:"frag_life_max"`
	FragmentBullet           *vanillaBulletProfile       `json:"frag_bullet,omitempty"`
	ShootEffect              string                      `json:"shoot_effect,omitempty"`
	SmokeEffect              string                      `json:"smoke_effect,omitempty"`
	HitEffect                string                      `json:"hit_effect,omitempty"`
	DespawnEffect            string                      `json:"despawn_effect,omitempty"`
	PreferBuildings          bool                        `json:"prefer_buildings"`
	TargetAir                bool                        `json:"target_air"`
	TargetGround             bool                        `json:"target_ground"`
	TargetPriority           string                      `json:"target_priority"`
	HitBuildings             bool                        `json:"hit_buildings"`
	HitRadius                float32                     `json:"hit_radius"`
	Bullet                   *vanillaBulletProfile       `json:"bullet,omitempty"`
	Mounts                   []vanillaWeaponMountProfile `json:"mounts,omitempty"`
	Abilities                []vanillaUnitAbilityProfile `json:"abilities,omitempty"`
}

type vanillaUnitAbilityProfile struct {
	Type                  string  `json:"type"`
	Amount                float32 `json:"amount"`
	Max                   float32 `json:"max"`
	Reload                float32 `json:"reload"`
	Range                 float32 `json:"range"`
	Radius                float32 `json:"radius"`
	Regen                 float32 `json:"regen"`
	Cooldown              float32 `json:"cooldown"`
	Width                 float32 `json:"width"`
	Angle                 float32 `json:"angle"`
	AngleOffset           float32 `json:"angle_offset"`
	X                     float32 `json:"x"`
	Y                     float32 `json:"y"`
	Damage                float32 `json:"damage"`
	StatusID              int16   `json:"status_id"`
	StatusName            string  `json:"status_name,omitempty"`
	StatusDuration        float32 `json:"status_duration"`
	MaxTargets            int32   `json:"max_targets"`
	HealPercent           float32 `json:"heal_percent"`
	SameTypeHealMult      float32 `json:"same_type_heal_mult"`
	ChanceDeflect         float32 `json:"chance_deflect"`
	MissileUnitMultiplier float32 `json:"missile_unit_multiplier"`
	SpawnAmount           int32   `json:"spawn_amount"`
	SpawnRandAmount       int32   `json:"spawn_rand_amount"`
	Spread                float32 `json:"spread"`
	TargetGround          bool    `json:"target_ground"`
	TargetAir             bool    `json:"target_air"`
	HitBuildings          bool    `json:"hit_buildings"`
	HitUnits              bool    `json:"hit_units"`
	Active                bool    `json:"active"`
	WhenShooting          bool    `json:"when_shooting"`
	OnShoot               bool    `json:"on_shoot"`
	UseAmmo               bool    `json:"use_ammo"`
	PushUnits             bool    `json:"push_units"`
	FaceOutwards          bool    `json:"face_outwards"`
	SpawnUnitName         string  `json:"spawn_unit_name,omitempty"`
}

type vanillaTurretProfile struct {
	ClassName                string                `json:"class_name,omitempty"`
	Name                     string                `json:"name"`
	FireMode                 string                `json:"fire_mode"`
	Range                    float32               `json:"range"`
	Damage                   float32               `json:"damage"`
	SplashDamage             float32               `json:"splash_damage"`
	Interval                 float32               `json:"interval"`
	BulletType               int16                 `json:"bullet_type"`
	BulletSpeed              float32               `json:"bullet_speed"`
	BulletLifetime           float32               `json:"bullet_lifetime"`
	BulletHitSize            float32               `json:"bullet_hit_size"`
	SplashRadius             float32               `json:"splash_radius"`
	BuildingDamageMultiplier float32               `json:"building_damage_multiplier"`
	ArmorMultiplier          float32               `json:"armor_multiplier"`
	MaxDamageFraction        float32               `json:"max_damage_fraction"`
	ShieldDamageMultiplier   float32               `json:"shield_damage_multiplier"`
	PierceDamageFactor       float32               `json:"pierce_damage_factor"`
	PierceArmor              bool                  `json:"pierce_armor"`
	SlowSec                  float32               `json:"slow_sec"`
	SlowMul                  float32               `json:"slow_mul"`
	Pierce                   int32                 `json:"pierce"`
	PierceBuilding           bool                  `json:"pierce_building"`
	ChainCount               int32                 `json:"chain_count"`
	ChainRange               float32               `json:"chain_range"`
	StatusID                 int16                 `json:"status_id"`
	StatusName               string                `json:"status_name"`
	StatusDuration           float32               `json:"status_duration"`
	FragmentCount            int32                 `json:"frag_bullets"`
	FragmentSpread           float32               `json:"frag_spread"`
	FragmentSpeed            float32               `json:"fragment_speed"`
	FragmentLife             float32               `json:"fragment_life"`
	FragmentRandomSpread     float32               `json:"frag_random_spread"`
	FragmentAngle            float32               `json:"frag_angle"`
	FragmentVelocityMin      float32               `json:"frag_velocity_min"`
	FragmentVelocityMax      float32               `json:"frag_velocity_max"`
	FragmentLifeMin          float32               `json:"frag_life_min"`
	FragmentLifeMax          float32               `json:"frag_life_max"`
	FragmentBullet           *vanillaBulletProfile `json:"frag_bullet,omitempty"`
	ShootEffect              string                `json:"shoot_effect,omitempty"`
	SmokeEffect              string                `json:"smoke_effect,omitempty"`
	HitEffect                string                `json:"hit_effect,omitempty"`
	DespawnEffect            string                `json:"despawn_effect,omitempty"`
	HitBuildings             bool                  `json:"hit_buildings"`
	TargetBuilds             bool                  `json:"target_builds"`
	TargetAir                bool                  `json:"target_air"`
	TargetGround             bool                  `json:"target_ground"`
	TargetPriority           string                `json:"target_priority"`
	AmmoCapacity             float32               `json:"ammo_capacity"`
	AmmoRegen                float32               `json:"ammo_regen"`
	AmmoPerShot              float32               `json:"ammo_per_shot"`
	PowerCapacity            float32               `json:"power_capacity"`
	PowerRegen               float32               `json:"power_regen"`
	PowerPerShot             float32               `json:"power_per_shot"`
	BurstShots               int32                 `json:"burst_shots"`
	BurstSpacing             float32               `json:"burst_spacing"`
	ContinuousHold           bool                  `json:"continuous_hold"`
	AimChangeSpeed           float32               `json:"aim_change_speed"`
	ShootDuration            float32               `json:"shoot_duration"`
	Rotate                   bool                  `json:"rotate"`
	RotateSpeed              float32               `json:"rotate_speed"`
	BaseRotation             float32               `json:"base_rotation"`
	PredictTarget            bool                  `json:"predict_target"`
	TargetInterval           float32               `json:"target_interval"`
	TargetSwitchInterval     float32               `json:"target_switch_interval"`
	ShootCone                float32               `json:"shoot_cone"`
	RotationLimit            float32               `json:"rotation_limit"`
	Bullet                   *vanillaBulletProfile `json:"bullet,omitempty"`
}

type vanillaBulletProfile struct {
	ClassName                string                `json:"class_name,omitempty"`
	Damage                   float32               `json:"damage"`
	SplashDamage             float32               `json:"splash_damage"`
	BulletType               int16                 `json:"bullet_type"`
	Speed                    float32               `json:"speed"`
	Lifetime                 float32               `json:"lifetime"`
	HitSize                  float32               `json:"hit_size"`
	SplashRadius             float32               `json:"splash_radius"`
	BuildingDamageMultiplier float32               `json:"building_damage_multiplier"`
	ArmorMultiplier          float32               `json:"armor_multiplier"`
	MaxDamageFraction        float32               `json:"max_damage_fraction"`
	ShieldDamageMultiplier   float32               `json:"shield_damage_multiplier"`
	PierceDamageFactor       float32               `json:"pierce_damage_factor"`
	PierceArmor              bool                  `json:"pierce_armor"`
	Pierce                   int32                 `json:"pierce"`
	PierceBuilding           bool                  `json:"pierce_building"`
	StatusID                 int16                 `json:"status_id"`
	StatusName               string                `json:"status_name"`
	StatusDuration           float32               `json:"status_duration"`
	HitBuildings             bool                  `json:"hit_buildings"`
	TargetAir                bool                  `json:"target_air"`
	TargetGround             bool                  `json:"target_ground"`
	ShootEffect              string                `json:"shoot_effect,omitempty"`
	SmokeEffect              string                `json:"smoke_effect,omitempty"`
	HitEffect                string                `json:"hit_effect,omitempty"`
	DespawnEffect            string                `json:"despawn_effect,omitempty"`
	Length                   float32               `json:"length"`
	DamageInterval           float32               `json:"damage_interval"`
	OptimalLifeFract         float32               `json:"optimal_life_fract"`
	FadeTime                 float32               `json:"fade_time"`
	FragBullets              int32                 `json:"frag_bullets"`
	FragSpread               float32               `json:"frag_spread"`
	FragRandomSpread         float32               `json:"frag_random_spread"`
	FragAngle                float32               `json:"frag_angle"`
	FragVelocityMin          float32               `json:"frag_velocity_min"`
	FragVelocityMax          float32               `json:"frag_velocity_max"`
	FragLifeMin              float32               `json:"frag_life_min"`
	FragLifeMax              float32               `json:"frag_life_max"`
	FragBullet               *vanillaBulletProfile `json:"frag_bullet,omitempty"`
}

type vanillaWeaponMountProfile struct {
	ClassName                string                `json:"class_name,omitempty"`
	FireMode                 string                `json:"fire_mode"`
	Range                    float32               `json:"range"`
	Damage                   float32               `json:"damage"`
	SplashDamage             float32               `json:"splash_damage"`
	Interval                 float32               `json:"interval"`
	BulletType               int16                 `json:"bullet_type"`
	BulletSpeed              float32               `json:"bullet_speed"`
	BulletLifetime           float32               `json:"bullet_lifetime"`
	BulletHitSize            float32               `json:"bullet_hit_size"`
	SplashRadius             float32               `json:"splash_radius"`
	BuildingDamageMultiplier float32               `json:"building_damage_multiplier"`
	ArmorMultiplier          float32               `json:"armor_multiplier"`
	MaxDamageFraction        float32               `json:"max_damage_fraction"`
	ShieldDamageMultiplier   float32               `json:"shield_damage_multiplier"`
	PierceDamageFactor       float32               `json:"pierce_damage_factor"`
	PierceArmor              bool                  `json:"pierce_armor"`
	Pierce                   int32                 `json:"pierce"`
	PierceBuilding           bool                  `json:"pierce_building"`
	StatusID                 int16                 `json:"status_id"`
	StatusName               string                `json:"status_name"`
	StatusDuration           float32               `json:"status_duration"`
	FragBullets              int32                 `json:"frag_bullets"`
	FragSpread               float32               `json:"frag_spread"`
	FragRandomSpread         float32               `json:"frag_random_spread"`
	FragAngle                float32               `json:"frag_angle"`
	FragVelocityMin          float32               `json:"frag_velocity_min"`
	FragVelocityMax          float32               `json:"frag_velocity_max"`
	FragLifeMin              float32               `json:"frag_life_min"`
	FragLifeMax              float32               `json:"frag_life_max"`
	FragBullet               *vanillaBulletProfile `json:"frag_bullet,omitempty"`
	TargetAir                bool                  `json:"target_air"`
	TargetGround             bool                  `json:"target_ground"`
	HitBuildings             bool                  `json:"hit_buildings"`
	PreferBuildings          bool                  `json:"prefer_buildings"`
	HitRadius                float32               `json:"hit_radius"`
	ShootStatusID            int16                 `json:"shoot_status_id"`
	ShootStatusName          string                `json:"shoot_status_name"`
	ShootStatusDuration      float32               `json:"shoot_status_duration"`
	ShootEffect              string                `json:"shoot_effect,omitempty"`
	SmokeEffect              string                `json:"smoke_effect,omitempty"`
	HitEffect                string                `json:"hit_effect,omitempty"`
	DespawnEffect            string                `json:"despawn_effect,omitempty"`
	Bullet                   *vanillaBulletProfile `json:"bullet,omitempty"`
	X                        float32               `json:"x"`
	Y                        float32               `json:"y"`
	ShootX                   float32               `json:"shoot_x"`
	ShootY                   float32               `json:"shoot_y"`
	Rotate                   bool                  `json:"rotate"`
	RotateSpeed              float32               `json:"rotate_speed"`
	BaseRotation             float32               `json:"base_rotation"`
	Mirror                   bool                  `json:"mirror"`
	Alternate                bool                  `json:"alternate"`
	FlipSprite               bool                  `json:"flip_sprite"`
	OtherSide                int32                 `json:"other_side"`
	Controllable             bool                  `json:"controllable"`
	AIControllable           bool                  `json:"ai_controllable"`
	AutoTarget               bool                  `json:"auto_target"`
	PredictTarget            bool                  `json:"predict_target"`
	UseAttackRange           bool                  `json:"use_attack_range"`
	AlwaysShooting           bool                  `json:"always_shooting"`
	NoAttack                 bool                  `json:"no_attack"`
	TargetInterval           float32               `json:"target_interval"`
	TargetSwitchInterval     float32               `json:"target_switch_interval"`
	ShootCone                float32               `json:"shoot_cone"`
	MinShootVelocity         float32               `json:"min_shoot_velocity"`
	Inaccuracy               float32               `json:"inaccuracy"`
	VelocityRnd              float32               `json:"velocity_rnd"`
	XRand                    float32               `json:"x_rand"`
	YRand                    float32               `json:"y_rand"`
	ExtraVelocity            float32               `json:"extra_velocity"`
	RotationLimit            float32               `json:"rotation_limit"`
	MinWarmup                float32               `json:"min_warmup"`
	ShootWarmupSpeed         float32               `json:"shoot_warmup_speed"`
	LinearWarmup             bool                  `json:"linear_warmup"`
	AimChangeSpeed           float32               `json:"aim_change_speed"`
	Continuous               bool                  `json:"continuous"`
	AlwaysContinuous         bool                  `json:"always_continuous"`
	PointDefense             bool                  `json:"point_defense"`
	RepairBeam               bool                  `json:"repair_beam"`
	TargetUnits              bool                  `json:"target_units"`
	TargetBuildings          bool                  `json:"target_buildings"`
	RepairSpeed              float32               `json:"repair_speed"`
	FractionRepairSpeed      float32               `json:"fraction_repair_speed"`
	ShootPattern             string                `json:"shoot_pattern,omitempty"`
	ShootShots               int32                 `json:"shoot_shots"`
	ShootFirstShotDelay      float32               `json:"shoot_first_shot_delay"`
	ShootShotDelay           float32               `json:"shoot_shot_delay"`
	ShootSpread              float32               `json:"shoot_spread"`
	ShootBarrels             int32                 `json:"shoot_barrels"`
	ShootBarrelOffset        int32                 `json:"shoot_barrel_offset"`
	ShootPatternMirror       bool                  `json:"shoot_pattern_mirror"`
	ShootHelixScl            float32               `json:"shoot_helix_scl"`
	ShootHelixMag            float32               `json:"shoot_helix_mag"`
	ShootHelixOffset         float32               `json:"shoot_helix_offset"`
}

type vanillaStatusProfile struct {
	ID                   int16    `json:"id"`
	Name                 string   `json:"name"`
	DamageMultiplier     float32  `json:"damage_multiplier"`
	HealthMultiplier     float32  `json:"health_multiplier"`
	SpeedMultiplier      float32  `json:"speed_multiplier"`
	ReloadMultiplier     float32  `json:"reload_multiplier"`
	BuildSpeedMultiplier float32  `json:"build_speed_multiplier"`
	DragMultiplier       float32  `json:"drag_multiplier"`
	TransitionDamage     float32  `json:"transition_damage"`
	Damage               float32  `json:"damage"`
	IntervalDamageTime   float32  `json:"interval_damage_time"`
	IntervalDamage       float32  `json:"interval_damage"`
	IntervalDamagePierce bool     `json:"interval_damage_pierce"`
	Disarm               bool     `json:"disarm"`
	Permanent            bool     `json:"permanent"`
	Reactive             bool     `json:"reactive"`
	Dynamic              bool     `json:"dynamic"`
	Opposites            []string `json:"opposites,omitempty"`
	Affinities           []string `json:"affinities,omitempty"`
}

var defaultWeaponProfile = weaponProfile{
	FireMode:            "projectile",
	Range:               56,
	Damage:              8,
	Interval:            0.7,
	BulletType:          0,
	BulletSpeed:         34,
	BulletHitSize:       10,
	SplashRadius:        0,
	BuildingDamage:      1,
	SlowSec:             0,
	SlowMul:             1,
	Pierce:              0,
	ChainCount:          0,
	ChainRange:          0,
	FragmentCount:       0,
	FragmentSpread:      0,
	FragmentSpeed:       0,
	FragmentLife:        0,
	FragmentVelocityMin: 0.2,
	FragmentVelocityMax: 1,
	FragmentLifeMin:     1,
	FragmentLifeMax:     1,
	PreferBuildings:     false,
	TargetAir:           true,
	TargetGround:        true,
	TargetPriority:      "nearest",
	HitBuildings:        true,
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
	"duo":        {FireMode: "projectile", Range: 136, Damage: 9, Interval: 0.27, BulletType: 94, BulletSpeed: 54, TargetAir: true, TargetGround: true, HitBuildings: true, AmmoCapacity: 80, AmmoRegen: 3.0, AmmoPerShot: 1, BurstShots: 2, BurstSpacing: 0.06},
	"scatter":    {FireMode: "projectile", Range: 152, Damage: 7, Interval: 0.23, BulletType: 99, BulletSpeed: 57, TargetAir: true, TargetGround: false, HitBuildings: false, AmmoCapacity: 30, AmmoRegen: 2.8, AmmoPerShot: 1, BurstShots: 3, BurstSpacing: 0.04},
	"scorch":     {FireMode: "projectile", Range: 62, Damage: 16, Interval: 0.13, BulletType: 101, BulletSpeed: 42, TargetAir: false, TargetGround: true, HitBuildings: false, AmmoCapacity: 30, AmmoRegen: 2.2, AmmoPerShot: 1},
	"hail":       {FireMode: "projectile", Range: 236, Damage: 24, Interval: 1.20, BulletType: 103, BulletSpeed: 52, SplashRadius: 18, TargetAir: false, TargetGround: true, HitBuildings: true, AmmoCapacity: 30, AmmoRegen: 1.1, AmmoPerShot: 1},
	"wave":       {FireMode: "projectile", Range: 118, Damage: 4, Interval: 0.09, BulletType: 106, BulletSpeed: 38, SlowSec: 1.8, SlowMul: 0.6, TargetAir: false, TargetGround: true, HitBuildings: false},
	"lancer":     {FireMode: "beam", Range: 172, Damage: 96, Interval: 1.35, BulletType: 110, TargetAir: true, TargetGround: true, TargetPriority: "threat", HitBuildings: true, PowerCapacity: 280, PowerRegen: 22, PowerPerShot: 80},
	"arc":        {FireMode: "beam", Range: 88, Damage: 24, Interval: 0.42, BulletType: 111, ChainCount: 2, ChainRange: 32, HitBuildings: true, PowerCapacity: 140, PowerRegen: 16, PowerPerShot: 30},
	"parallax":   {FireMode: "projectile", Range: 292, Damage: 20, Interval: 0.55, BulletType: 112, BulletSpeed: 64, SlowSec: 0.8, SlowMul: 0.75, TargetAir: true, TargetGround: false, HitBuildings: false},
	"swarmer":    {FireMode: "projectile", Range: 216, Damage: 22, Interval: 0.35, BulletType: 113, BulletSpeed: 62, SplashRadius: 12, HitBuildings: true, AmmoCapacity: 30, AmmoRegen: 1.7, AmmoPerShot: 1, BurstShots: 2, BurstSpacing: 0.05},
	"salvo":      {FireMode: "projectile", Range: 188, Damage: 23, Interval: 0.32, BulletType: 116, BulletSpeed: 60, Pierce: 1, HitBuildings: true, AmmoCapacity: 30, AmmoRegen: 2.0, AmmoPerShot: 1, BurstShots: 4, BurstSpacing: 0.045},
	"segment":    {FireMode: "beam", Range: 88, Damage: 26, Interval: 0.16, BulletType: 111, ChainCount: 1, ChainRange: 20, TargetAir: true, TargetGround: false, HitBuildings: false},
	"tsunami":    {FireMode: "projectile", Range: 174, Damage: 10, Interval: 0.08, BulletType: 106, BulletSpeed: 44, SlowSec: 2.8, SlowMul: 0.45, TargetAir: false, TargetGround: true, HitBuildings: false},
	"fuse":       {FireMode: "beam", Range: 120, Damage: 180, Interval: 0.95, BulletType: 125, HitBuildings: true, AmmoCapacity: 30, AmmoRegen: 1.2, AmmoPerShot: 1},
	"ripple":     {FireMode: "projectile", Range: 286, Damage: 62, Interval: 1.35, BulletType: 127, BulletSpeed: 72, SplashRadius: 24, HitBuildings: true, AmmoCapacity: 30, AmmoRegen: 0.9, AmmoPerShot: 2},
	"cyclone":    {FireMode: "projectile", Range: 214, Damage: 18, Interval: 0.10, BulletType: 133, BulletSpeed: 65, HitBuildings: true, AmmoCapacity: 30, AmmoRegen: 4.8, AmmoPerShot: 1},
	"foreshadow": {FireMode: "projectile", Range: 472, Damage: 640, Interval: 4.8, BulletType: 139, BulletSpeed: 94, Pierce: 3, TargetPriority: "highest_health", HitBuildings: true, AmmoCapacity: 40, AmmoRegen: 0.8, AmmoPerShot: 5, PowerCapacity: 1800, PowerRegen: 90, PowerPerShot: 900},
	"spectre":    {FireMode: "projectile", Range: 300, Damage: 84, Interval: 0.18, BulletType: 140, BulletSpeed: 82, TargetPriority: "threat", HitBuildings: true, AmmoCapacity: 30, AmmoRegen: 3.4, AmmoPerShot: 1},
	"meltdown":   {FireMode: "beam", Range: 236, Damage: 94, Interval: 0.12, BulletType: 143, SlowSec: 0.7, SlowMul: 0.8, HitBuildings: true, PowerCapacity: 1200, PowerRegen: 120, PowerPerShot: 60},
	"breach":     {FireMode: "projectile", Range: 120, Damage: 25, Interval: 0.22, BulletType: 144, BulletSpeed: 56, HitBuildings: true, AmmoCapacity: 30, AmmoPerShot: 2},
	"diffuse":    {FireMode: "projectile", Range: 152, Damage: 16, Interval: 0.14, BulletType: 148, BulletSpeed: 58, HitBuildings: true, AmmoCapacity: 30, AmmoPerShot: 3},
	"sublimate":  {FireMode: "beam", Range: 156, Damage: 52, Interval: 0.22, BulletType: 151, ChainCount: 2, ChainRange: 28, HitBuildings: true, PowerCapacity: 360, PowerRegen: 28, PowerPerShot: 36},
	"titan":      {FireMode: "projectile", Range: 210, Damage: 38, Interval: 0.36, BulletType: 153, BulletSpeed: 66, HitBuildings: true, AmmoCapacity: 12, AmmoPerShot: 4},
	"disperse":   {FireMode: "projectile", Range: 230, Damage: 36, Interval: 0.28, BulletType: 159, BulletSpeed: 72, SplashRadius: 18, HitBuildings: true, AmmoCapacity: 30, AmmoPerShot: 1},
	"afflict":    {FireMode: "beam", Range: 246, Damage: 128, Interval: 0.24, BulletType: 164, HitBuildings: true, PowerCapacity: 760, PowerRegen: 62, PowerPerShot: 84},
	"lustre":     {FireMode: "beam", Range: 332, Damage: 180, Interval: 0.26, BulletType: 166, ChainCount: 1, ChainRange: 36, HitBuildings: true, PowerCapacity: 980, PowerRegen: 70, PowerPerShot: 100},
	"scathe":     {FireMode: "projectile", Range: 438, Damage: 260, Interval: 1.05, BulletType: 167, BulletSpeed: 84, SplashRadius: 26, HitBuildings: true, TargetBuilds: true, AmmoCapacity: 45, AmmoRegen: 0.55, AmmoPerShot: 15},
	"smite":      {FireMode: "projectile", Range: 352, Damage: 220, Interval: 0.65, BulletType: 177, BulletSpeed: 86, SplashRadius: 20, HitBuildings: true, AmmoCapacity: 30, AmmoRegen: 0.75, AmmoPerShot: 2},
	"malign":     {FireMode: "beam", Range: 402, Damage: 260, Interval: 0.34, BulletType: 180, ChainCount: 2, ChainRange: 44, HitBuildings: true, PowerCapacity: 1400, PowerRegen: 105, PowerPerShot: 140},
}

var turretItemAmmoBulletTypesByName = map[string]map[ItemID]int16{
	"duo": {
		copperItemID:   94,
		graphiteItemID: 95,
		siliconItemID:  96,
	},
	"scatter": {
		scrapItemID:     97,
		leadItemID:      98,
		metaglassItemID: 99,
	},
	"scorch": {
		coalItemID:     101,
		pyratiteItemID: 102,
	},
	"hail": {
		graphiteItemID: 103,
		siliconItemID:  104,
		pyratiteItemID: 105,
	},
	"swarmer": {
		blastCompoundItemID: 113,
		pyratiteItemID:      114,
		surgeAlloyItemID:    115,
	},
	"salvo": {
		copperItemID:   116,
		graphiteItemID: 117,
		pyratiteItemID: 118,
		siliconItemID:  119,
		thoriumItemID:  120,
	},
	"fuse": {
		titaniumItemID: 125,
		thoriumItemID:  126,
	},
	"ripple": {
		graphiteItemID:      127,
		siliconItemID:       128,
		pyratiteItemID:      129,
		blastCompoundItemID: 130,
		plastaniumItemID:    131,
	},
	"cyclone": {
		metaglassItemID:     133,
		blastCompoundItemID: 135,
		plastaniumItemID:    136,
		surgeAlloyItemID:    138,
	},
	"spectre": {
		graphiteItemID: 140,
		thoriumItemID:  141,
		pyratiteItemID: 142,
	},
	"breach": {
		berylliumItemID: 144,
		tungstenItemID:  145,
		carbideItemID:   146,
	},
	"diffuse": {
		graphiteItemID: 148,
		oxideItemID:    149,
		siliconItemID:  150,
	},
	"titan": {
		thoriumItemID: 153,
		carbideItemID: 154,
		oxideItemID:   156,
	},
	"disperse": {
		tungstenItemID:   159,
		thoriumItemID:    160,
		siliconItemID:    161,
		surgeAlloyItemID: 162,
	},
	"scathe": {
		carbideItemID:     167,
		phaseFabricItemID: 170,
		surgeAlloyItemID:  173,
	},
	"smite": {
		surgeAlloyItemID: 177,
	},
}

var turretLiquidAmmoBulletTypesByName = map[string]map[LiquidID]int16{
	"wave": {
		waterLiquidID:     106,
		slagLiquidID:      107,
		cryofluidLiquidID: 108,
		oilLiquidID:       109,
	},
	"tsunami": {
		waterLiquidID:     106,
		slagLiquidID:      107,
		cryofluidLiquidID: 108,
		oilLiquidID:       109,
	},
	"sublimate": {
		ozoneLiquidID:    151,
		cyanogenLiquidID: 152,
	},
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
		wave:                       1,
		waveTime:                   0,
		tick:                       0,
		rand0:                      rng.Int63(),
		rand1:                      rng.Int63(),
		tps:                        int8(tps),
		actualTps:                  int8(tps),
		tpsWindowStart:             time.Now(),
		start:                      time.Now(),
		useMapSyncDataFallback:     cfg.UseMapSyncDataFallback,
		blockSyncLogsEnabled:       cfg.BlockSyncLogsEnabled,
		pendingMountShots:          []pendingMountShot{},
		bulletNextID:               1,
		blockItemSyncTick:          map[int32]uint64{},
		buildStates:                map[int32]buildCombatState{},
		controlledBuilds:           map[int32]controlledBuildState{},
		controlledBuildByPlayer:    map[int32]int32{},
		pendingBuilds:              map[int32]pendingBuildState{},
		pendingBreaks:              map[int32]pendingBreakState{},
		buildRejectLogTick:         map[int32]uint64{},
		builderStates:              map[int32]builderRuntimeState{},
		teamRebuildPlans:           map[TeamID][]rebuildBlockPlan{},
		teamAIBuildPlans:           map[TeamID][]teamBuildPlan{},
		teamBuildAIStates:          map[TeamID]buildAIPlannerState{},
		factoryStates:              map[int32]factoryState{},
		reconstructorStates:        map[int32]reconstructorState{},
		drillStates:                map[int32]drillRuntimeState{},
		burstDrillStates:           map[int32]burstDrillRuntimeState{},
		beamDrillStates:            map[int32]beamDrillRuntimeState{},
		pumpStates:                 map[int32]pumpRuntimeState{},
		crafterStates:              map[int32]crafterRuntimeState{},
		heatStates:                 map[int32]float32{},
		incineratorStates:          map[int32]float32{},
		repairTurretStates:         map[int32]repairTurretRuntimeState{},
		repairTowerStates:          map[int32]repairTowerRuntimeState{},
		teamPowerStates:            map[TeamID]*teamPowerState{},
		teamPowerBudget:            map[TeamID]float32{},
		powerNetStates:             map[int32]*powerNetState{},
		powerNetByPos:              map[int32]int32{},
		powerNetDirty:              true,
		powerStorageState:          map[int32]float32{},
		powerRequested:             map[int32]float32{},
		powerSupplied:              map[int32]float32{},
		powerGeneratorState:        map[int32]*powerGeneratorState{},
		unitMountCDs:               map[int32][]float32{},
		unitMountStates:            map[int32][]unitMountState{},
		unitTargets:                map[int32]targetTrackState{},
		unitAIStates:               map[int32]unitAIState{},
		unitMiningStates:           map[int32]unitMiningState{},
		teamItems:                  map[TeamID]map[ItemID]int32{},
		teamBuilderSpeed:           map[TeamID]float32{1: 0.5},
		itemSourceCfg:              map[int32]ItemID{},
		liquidSourceCfg:            map[int32]LiquidID{},
		sorterCfg:                  map[int32]ItemID{},
		unloaderCfg:                map[int32]ItemID{},
		payloadRouterCfg:           map[int32]protocol.Content{},
		powerNodeLinks:             map[int32][]int32{},
		bridgeLinks:                map[int32]int32{},
		massDriverLinks:            map[int32]int32{},
		payloadDriverLinks:         map[int32]int32{},
		bridgeBuffers:              map[int32][]bufferedBridgeItem{},
		bridgeAcceptAcc:            map[int32]float32{},
		conveyorStates:             map[int32]*conveyorRuntimeState{},
		ductStates:                 map[int32]*ductRuntimeState{},
		routerStates:               map[int32]*routerRuntimeState{},
		stackStates:                map[int32]*stackRuntimeState{},
		massDriverStates:           map[int32]*massDriverRuntimeState{},
		payloadStates:              map[int32]*payloadRuntimeState{},
		payloadDeconstructorStates: map[int32]*payloadDeconstructorState{},
		payloadDriverStates:        map[int32]*payloadDriverRuntimeState{},
		massDriverShots:            []massDriverShot{},
		payloadDriverShots:         []payloadDriverShot{},
		blockDumpIndex:             map[int32]int{},
		dumpNeighborCache:          map[int32][]int32{},
		unloaderLastUsed:           map[int64]int{},
		itemSourceAccum:            map[int32]float32{},
		routerInputPos:             map[int32]int32{},
		routerRotation:             map[int32]byte{},
		transportAccum:             map[int32]float32{},
		junctionQueues:             map[int32]junctionQueueState{},
		bridgeIncomingMask:         map[int32]byte{},
		reactorStates:              map[int32]nuclearReactorState{},
		storageLinkedCore:          map[int32]int32{},
		teamPrimaryCore:            map[TeamID]int32{},
		coreStorageCapacity:        map[int32]int32{},
		blockOccupancy:             map[int32]int32{},
		itemLogisticsTilePositions: []int32{},
		crafterTilePositions:       []int32{},
		drillTilePositions:         []int32{},
		burstDrillTilePositions:    []int32{},
		beamDrillTilePositions:     []int32{},
		pumpTilePositions:          []int32{},
		incineratorTilePositions:   []int32{},
		repairTurretTilePositions:  []int32{},
		repairTowerTilePositions:   []int32{},
		factoryTilePositions:       []int32{},
		heatConductorTilePositions: []int32{},
		powerTilePositions:         []int32{},
		powerDiodeTilePositions:    []int32{},
		powerVoidTilePositions:     []int32{},
		teamBuildingTiles:          map[TeamID][]int32{},
		teamBuildingSpatial:        map[TeamID]*buildingSpatialIndex{},
		teamCoreTiles:              map[TeamID][]int32{},
		teamPowerTiles:             map[TeamID][]int32{},
		teamPowerNodeTiles:         map[TeamID][]int32{},
		turretTilePositions:        []int32{},
		unitProfilesByType:         cloneUnitWeaponProfiles(weaponProfilesByType),
		unitProfilesByName:         map[string]weaponProfile{},
		unitRuntimeProfilesByName:  map[string]unitRuntimeProfile{},
		unitMountProfilesByName:    map[string][]unitWeaponMountProfile{},
		buildingProfilesByName:     cloneBuildingWeaponProfiles(buildingWeaponProfilesByName),
		blockCostsByName:           map[string][]ItemStack{},
		blockBuildTimesByName:      map[string]float32{},
		blockArmorByName:           map[string]float32{},
		statusProfilesByID:         map[int16]statusEffectProfile{},
		statusProfilesByName:       map[string]statusEffectProfile{},
		rulesMgr:                   NewRulesManager(nil),
		wavesMgr:                   NewWaveManager(nil),
	}
}

func (w *World) Step(delta time.Duration) {
	w.mu.Lock()
	defer w.mu.Unlock()
	stepStartedAt := time.Now()
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
	dt := float32(delta.Seconds())
	if dt > 0 {
		w.timeSec += dt
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

	w.stepFillItemsLocked()

	pendingBuildStartedAt := time.Now()
	w.stepPendingBuilds(delta)
	pendingBuildDur := time.Since(pendingBuildStartedAt)

	pendingBreakStartedAt := time.Now()
	w.stepPendingBreaks(delta)
	pendingBreakDur := time.Since(pendingBreakStartedAt)

	sandboxStartedAt := time.Now()
	w.stepSandboxSources(delta)
	sandboxDur := time.Since(sandboxStartedAt)

	liquidStartedAt := time.Now()
	w.stepLiquidLogistics(delta)
	liquidDur := time.Since(liquidStartedAt)

	reactorStartedAt := time.Now()
	w.stepNuclearReactors(delta)
	reactorDur := time.Since(reactorStartedAt)

	w.beginTeamPowerStep(delta)
	factoryStartedAt := time.Now()
	w.stepFactoryProduction(delta)
	w.stepDrillProduction(delta)
	w.stepBurstDrillProduction(delta)
	w.stepBeamDrillProduction(delta)
	w.stepPumpProduction(delta)
	w.stepCrafterProduction(delta)
	w.stepHeatConductorsLocked()
	w.stepIncinerators(delta)
	w.stepRepairBlocks(delta)
	w.stepBulletsLocked(delta)
	w.stepSupportBuildingsLocked(delta)
	factoryDur := time.Since(factoryStartedAt)

	itemStartedAt := time.Now()
	itemPerf := w.stepItemLogistics(delta, w.shouldProfileItemPerfLocked(now))
	itemDur := time.Since(itemStartedAt)

	payloadStartedAt := time.Now()
	w.stepPayloadLogistics(delta)
	payloadDur := time.Since(payloadStartedAt)

	w.stepBuildAIFillCoresLocked()
	w.stepBuildAICoreSpawnLocked(dt)
	w.stepBuildAIRefreshPathsLocked(dt)
	w.stepBuildAIPlansLocked(dt)
	w.stepPrebuildAICoreBuildersLocked()

	entitiesStartedAt := time.Now()
	entityMovementDur, entityCombatDur, buildingCombatDur, bulletDur := w.stepEntities(delta)
	entitiesDur := time.Since(entitiesStartedAt)
	w.endTeamPowerStep()

	totalDur := time.Since(stepStartedAt)
	if totalDur > 0 {
		estimated := int(math.Round(float64(time.Second) / float64(totalDur)))
		if estimated <= 0 {
			estimated = 1
		}
		if estimated > int(w.tps) {
			estimated = int(w.tps)
		}
		if w.actualTps <= 0 || now.Sub(w.tpsWindowStart) < time.Second {
			w.actualTps = int8(estimated)
		}
	}
	if w.shouldLogPerfLocked(totalDur) {
		entityCount := 0
		if w.model != nil {
			entityCount = len(w.model.Entities)
		}
		fmt.Printf("[perf] step=%s tps=%d/%d active=%d entities=%d bullets=%d pendingBuilds=%d pendingBreaks=%d phases{build=%s break=%s factory=%s sandbox=%s liquid=%s item=%s payload=%s reactor=%s entities=%s} itemPhases{junction=%s/%d conveyor=%s/%d duct=%s/%d router=%s/%d bridge=%s/%d unloader=%s/%d mass=%s/%d} entityPhases{move=%s combat=%s building=%s bullets=%s}\n",
			totalDur.Round(time.Millisecond),
			w.actualTps,
			w.tps,
			len(w.activeTilePositions),
			entityCount,
			len(w.bullets),
			len(w.pendingBuilds),
			len(w.pendingBreaks),
			pendingBuildDur.Round(time.Millisecond),
			pendingBreakDur.Round(time.Millisecond),
			factoryDur.Round(time.Millisecond),
			sandboxDur.Round(time.Millisecond),
			liquidDur.Round(time.Millisecond),
			itemDur.Round(time.Millisecond),
			payloadDur.Round(time.Millisecond),
			reactorDur.Round(time.Millisecond),
			entitiesDur.Round(time.Millisecond),
			itemPerf.Junctions.Round(time.Millisecond),
			itemPerf.JunctionCount,
			itemPerf.Conveyor.Round(time.Millisecond),
			itemPerf.ConveyorCount,
			itemPerf.Duct.Round(time.Millisecond),
			itemPerf.DuctCount,
			itemPerf.Router.Round(time.Millisecond),
			itemPerf.RouterCount,
			itemPerf.Bridge.Round(time.Millisecond),
			itemPerf.BridgeCount,
			itemPerf.Unloader.Round(time.Millisecond),
			itemPerf.UnloaderCount,
			itemPerf.MassDrive.Round(time.Millisecond),
			itemPerf.MassDriveCount,
			entityMovementDur.Round(time.Millisecond),
			entityCombatDur.Round(time.Millisecond),
			buildingCombatDur.Round(time.Millisecond),
			bulletDur.Round(time.Millisecond),
		)
		w.perfLogAt = time.Now()
	}
}

func (w *World) shouldLogPerfLocked(totalDur time.Duration) bool {
	now := time.Now()
	if !w.perfLogAt.IsZero() && now.Sub(w.perfLogAt) < 2*time.Second {
		return false
	}
	if totalDur >= 50*time.Millisecond {
		return true
	}
	return w.actualTps > 0 && w.actualTps <= int8(max(20, int(w.tps)/2))
}

func (w *World) shouldProfileItemPerfLocked(now time.Time) bool {
	return w.perfLogAt.IsZero() || now.Sub(w.perfLogAt) >= 2*time.Second
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

func (w *World) BuildingInfoPacked(pos int32) (BuildingInfo, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()

	index, ok := w.buildingIndexFromPackedPosLocked(pos)
	if !ok {
		return BuildingInfo{}, false
	}
	return w.buildingInfoForTileIndexLocked(index)
}

func (w *World) BuildingInfoTileIndex(pos int32) (BuildingInfo, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.buildingInfoForTileIndexLocked(pos)
}

func (w *World) TileIndexFromPackedPos(pos int32) (int32, bool) {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.tileIndexFromPackedPosLocked(pos)
}

func (w *World) buildingInfoForTileIndexLocked(index int32) (BuildingInfo, bool) {
	if w.model == nil || index < 0 || int(index) >= len(w.model.Tiles) {
		return BuildingInfo{}, false
	}
	center, ok := w.centerBuildingIndexLocked(index)
	if !ok || center < 0 || int(center) >= len(w.model.Tiles) {
		return BuildingInfo{}, false
	}
	tile := &w.model.Tiles[center]
	if tile.Block == 0 || tile.Build == nil {
		return BuildingInfo{}, false
	}

	team := tile.Team
	if tile.Build.Team != 0 {
		team = tile.Build.Team
	}
	return BuildingInfo{
		Pos:      packTilePos(tile.X, tile.Y),
		X:        int32(tile.X),
		Y:        int32(tile.Y),
		BlockID:  int16(tile.Block),
		Name:     w.blockNameByID(int16(tile.Block)),
		Team:     team,
		Rotation: tile.Rotation,
	}, true
}

func (w *World) BlockSyncSuppressedPacked(packedPos int32) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	index, ok := w.buildingIndexFromPackedPosLocked(packedPos)
	if !ok {
		return false
	}
	return w.blockSyncSuppressedLocked(index)
}

func (w *World) RotateBuildingPacked(pos int32, direction bool) (RotateBuildingResult, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()

	index, ok := w.buildingIndexFromPackedPosLocked(pos)
	if !ok || w.model == nil || index < 0 || int(index) >= len(w.model.Tiles) {
		return RotateBuildingResult{}, false
	}
	tile := &w.model.Tiles[index]
	if tile.Block == 0 || tile.Build == nil {
		return RotateBuildingResult{}, false
	}

	step := -1
	if direction {
		step = 1
	}
	nextRotation := int8((tileRotationNorm(tile.Rotation) + step + 4) % 4)
	tile.Rotation = nextRotation
	tile.Build.Rotation = nextRotation
	if tile.Build.Team != 0 {
		tile.Team = tile.Build.Team
	}
	if w.isPowerRelevantBuildingLocked(tile) {
		w.invalidatePowerNetsLocked()
	}

	return RotateBuildingResult{
		BlockID:   int16(tile.Block),
		Rotation:  nextRotation,
		Team:      tile.Team,
		EffectX:   float32(tile.X*8 + 4),
		EffectY:   float32(tile.Y*8 + 4),
		EffectRot: float32(w.blockSizeForTileLocked(tile)),
	}, true
}

func (w *World) CommandBuildingsPacked(positions []int32, target protocol.Vec2) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil || len(positions) == 0 {
		return
	}
	for _, packed := range positions {
		index, ok := w.tileIndexFromPackedPosLocked(packed)
		if !ok {
			continue
		}
		if w.unitFactoryConfigBlockAtLocked(index) {
			w.configureUnitFactoryCommandPosLocked(index, target)
			continue
		}
		if isReconstructorBlockName(w.blockNameByID(int16(w.model.Tiles[index].Block))) {
			w.configureReconstructorCommandPosLocked(index, target)
		}
	}
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

func (w *World) unitFactoryConfigBlockAtLocked(pos int32) bool {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	switch w.blockNameByID(int16(w.model.Tiles[pos].Block)) {
	case "ground-factory", "air-factory", "naval-factory":
		return true
	default:
		return false
	}
}

func (w *World) ClearBuildingConfig(pos int32) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.unitFactoryConfigBlockAtLocked(pos) {
		_ = w.clearUnitFactoryCommandLocked(pos)
		return
	}
	if w.model != nil && pos >= 0 && int(pos) < len(w.model.Tiles) && isReconstructorBlockName(w.blockNameByID(int16(w.model.Tiles[pos].Block))) {
		_ = w.clearReconstructorCommandLocked(pos)
		return
	}
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
		return cloneStoredBuildingConfigValue(value)
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
	w.invalidateItemRoutingCachesLocked()
	w.clearControlledBuildingLocked(pos)
	w.clearConfiguredStateLocked(pos)
	delete(w.bridgeBuffers, pos)
	delete(w.bridgeAcceptAcc, pos)
	delete(w.conveyorStates, pos)
	delete(w.ductStates, pos)
	delete(w.routerStates, pos)
	delete(w.stackStates, pos)
	delete(w.massDriverStates, pos)
	delete(w.payloadStates, pos)
	delete(w.payloadDeconstructorStates, pos)
	delete(w.payloadDriverStates, pos)
	delete(w.blockDumpIndex, pos)
	delete(w.itemSourceAccum, pos)
	delete(w.routerInputPos, pos)
	delete(w.routerRotation, pos)
	delete(w.transportAccum, pos)
	delete(w.junctionQueues, pos)
	delete(w.reactorStates, pos)
	delete(w.drillStates, pos)
	delete(w.burstDrillStates, pos)
	delete(w.beamDrillStates, pos)
	delete(w.pumpStates, pos)
	delete(w.crafterStates, pos)
	delete(w.reconstructorStates, pos)
	delete(w.heatStates, pos)
	delete(w.incineratorStates, pos)
	delete(w.repairTurretStates, pos)
	delete(w.repairTowerStates, pos)
	delete(w.turretStates, pos)
	delete(w.powerStorageState, pos)
	delete(w.powerGeneratorState, pos)
}

func (w *World) clearConfiguredStateLocked(pos int32) {
	w.invalidateItemRoutingCachesLocked()
	if w.model != nil && pos >= 0 && int(pos) < len(w.model.Tiles) {
		switch w.blockNameByID(int16(w.model.Tiles[pos].Block)) {
		case "power-node", "power-node-large", "surge-tower", "beam-link", "power-source":
			w.clearPowerLinksForBuildingLocked(pos)
		}
	}
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

func (w *World) centerBuildingIndexLocked(pos int32) (int32, bool) {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0, false
	}
	tile := &w.model.Tiles[pos]
	if isCenterBuildingTile(tile) {
		return pos, true
	}
	if tile.Build == nil {
		return 0, false
	}
	cx := tile.Build.X
	cy := tile.Build.Y
	if !w.model.InBounds(cx, cy) {
		return 0, false
	}
	centerPos := int32(cy*w.model.Width + cx)
	if centerPos < 0 || int(centerPos) >= len(w.model.Tiles) {
		return 0, false
	}
	if !isCenterBuildingTile(&w.model.Tiles[centerPos]) {
		return 0, false
	}
	return centerPos, true
}

func (w *World) buildingIndexFromPackedPosLocked(pos int32) (int32, bool) {
	if w.model == nil {
		return 0, false
	}
	x := int(protocol.UnpackPoint2X(pos))
	y := int(protocol.UnpackPoint2Y(pos))
	if !w.model.InBounds(x, y) {
		return 0, false
	}
	if centerPos, ok := w.blockOccupancy[packTilePos(x, y)]; ok {
		return w.centerBuildingIndexLocked(centerPos)
	}
	return w.centerBuildingIndexLocked(int32(y*w.model.Width + x))
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
		if w.unitFactoryConfigBlockAtLocked(pos) {
			w.clearUnitFactoryCommandLocked(pos)
			if persist {
				if normalized, ok := w.normalizedBuildingConfigLocked(pos); ok {
					w.storeBuildingConfigLocked(tile, normalized)
				} else {
					tile.Build.Config = nil
				}
			}
			return
		}
		if isReconstructorBlockName(w.blockNameByID(int16(tile.Block))) {
			w.clearReconstructorCommandLocked(pos)
			if persist {
				if normalized, ok := w.normalizedBuildingConfigLocked(pos); ok {
					w.storeBuildingConfigLocked(tile, normalized)
				} else {
					tile.Build.Config = nil
				}
			}
			return
		}
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
		case protocol.ContentBlock:
			applied = w.configurePayloadContentLocked(pos, v)
		case protocol.ContentUnit:
			if w.unitFactoryConfigBlockAtLocked(pos) {
				applied = w.configureUnitFactoryUnitLocked(pos, v.ID())
			} else {
				applied = w.configurePayloadContentLocked(pos, v)
			}
		}
	case protocol.Point2:
		applied = w.configurePointConfigLocked(pos, v)
	case []protocol.Point2:
		applied = w.configurePointSeqConfigLocked(pos, v)
	case protocol.UnitCommand:
		if w.unitFactoryConfigBlockAtLocked(pos) {
			applied = w.configureUnitFactoryCommandLocked(pos, &v)
		} else if isReconstructorBlockName(w.blockNameByID(int16(tile.Block))) {
			applied = w.configureReconstructorCommandLocked(pos, &v)
		}
	case int32:
		if w.itemConfigBlockAtLocked(pos) {
			w.configureItemContentLocked(pos, ItemID(v))
			applied = true
		} else if w.unitFactoryConfigBlockAtLocked(pos) {
			applied = w.configureUnitFactoryPlanLocked(pos, int16(v))
		} else {
			applied = w.configureAbsoluteLinkLocked(pos, v)
		}
	case int:
		if w.itemConfigBlockAtLocked(pos) {
			w.configureItemContentLocked(pos, ItemID(v))
			applied = true
		} else if w.unitFactoryConfigBlockAtLocked(pos) {
			applied = w.configureUnitFactoryPlanLocked(pos, int16(v))
		} else {
			applied = w.configureAbsoluteLinkLocked(pos, int32(v))
		}
	case int16:
		if w.itemConfigBlockAtLocked(pos) {
			w.configureItemContentLocked(pos, ItemID(v))
			applied = true
		} else if w.unitFactoryConfigBlockAtLocked(pos) {
			applied = w.configureUnitFactoryPlanLocked(pos, v)
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
	if persist && applied && tile.Build != nil {
		tile.Build.MapSyncData = nil
		tile.Build.MapSyncTail = nil
		tile.Build.MapPowerLinks = nil
		tile.Build.MapPowerStatus = 0
		tile.Build.MapPowerStatusSet = false
	}
}

func (w *World) configurePointConfigLocked(pos int32, p protocol.Point2) bool {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	switch w.blockNameByID(int16(tile.Block)) {
	case "power-node", "power-node-large", "surge-tower", "beam-link", "power-source":
		targetX := tile.X + int(p.X)
		targetY := tile.Y + int(p.Y)
		if !w.model.InBounds(targetX, targetY) {
			return false
		}
		return w.configureAbsoluteLinkLocked(pos, int32(targetY*w.model.Width+targetX))
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

func (w *World) configurePointSeqConfigLocked(pos int32, points []protocol.Point2) bool {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return false
	}
	tile := &w.model.Tiles[pos]
	switch w.blockNameByID(int16(tile.Block)) {
	case "power-node", "power-node-large", "surge-tower", "beam-link", "power-source":
		targets := make([]int32, 0, len(points))
		for _, p := range points {
			targetX := tile.X + int(p.X)
			targetY := tile.Y + int(p.Y)
			if !w.model.InBounds(targetX, targetY) {
				continue
			}
			targets = append(targets, int32(targetY*w.model.Width+targetX))
		}
		return w.configurePowerNodeLinksLocked(pos, targets)
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
			case "power-node", "power-node-large", "surge-tower", "beam-link", "power-source":
				w.clearPowerLinksForBuildingLocked(pos)
			case "bridge-conveyor", "phase-conveyor":
				delete(w.bridgeLinks, pos)
				w.invalidateItemRoutingCachesLocked()
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
	if name == "power-node" || name == "power-node-large" || name == "surge-tower" || name == "beam-link" || name == "power-source" {
		return w.togglePowerNodeLinkLocked(pos, target)
	}
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
		w.invalidateItemRoutingCachesLocked()
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
	case "ground-factory", "air-factory", "naval-factory":
		value, ok := w.unitFactoryConfigValueLocked(pos, tile)
		if !ok {
			return nil, false
		}
		return value, true
	case "additive-reconstructor", "multiplicative-reconstructor", "exponential-reconstructor", "tetrative-reconstructor",
		"tank-refabricator", "ship-refabricator", "mech-refabricator", "prime-refabricator":
		_, command := w.reconstructorCommandStateLocked(pos)
		if command == nil {
			return nil, false
		}
		return *command, true
	case "power-node", "power-node-large", "surge-tower", "beam-link", "power-source":
		links := w.powerNodeLinks[pos]
		if len(links) == 0 {
			return nil, false
		}
		out := make([]protocol.Point2, 0, len(links))
		for _, target := range links {
			if target < 0 || int(target) >= len(w.model.Tiles) {
				continue
			}
			targetTile := &w.model.Tiles[target]
			out = append(out, protocol.Point2{X: int32(targetTile.X - tile.X), Y: int32(targetTile.Y - tile.Y)})
		}
		if len(out) == 0 {
			return nil, false
		}
		sort.Slice(out, func(i, j int) bool {
			if out[i].X == out[j].X {
				return out[i].Y < out[j].Y
			}
			return out[i].X < out[j].X
		})
		return out, true
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

func cloneStoredBuildingConfigValue(value any) (any, bool) {
	switch v := value.(type) {
	case protocol.ItemRef:
		return v, true
	case protocol.BlockRef:
		return v, true
	case protocol.Point2:
		return v, true
	case []protocol.Point2:
		out := append([]protocol.Point2(nil), v...)
		return out, true
	case protocol.UnitCommand:
		return v, true
	case int32:
		return v, true
	case int16:
		return v, true
	case int:
		return v, true
	case bool:
		return v, true
	case float64:
		return v, true
	case string:
		return v, true
	case []byte:
		out := append([]byte(nil), v...)
		return out, true
	}
	writer := protocol.NewWriter()
	if err := protocol.WriteObject(writer, value, nil); err != nil {
		return nil, false
	}
	return decodeStoredBuildingConfig(writer.Bytes())
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
		if entity, ok := decodeRawUnitPayloadEntity(v.Raw, v.ClassID); ok && entity != nil {
			out.UnitTypeID = entity.TypeID
			out.Rotation = buildRotationFromDegrees(entity.Rotation)
			out.Health = entity.Health
			out.MaxHealth = entity.MaxHealth
			clone := cloneRawEntity(*entity)
			out.UnitState = &clone
		}
		return out, true
	case protocol.PayloadBox:
		if len(v.Raw) == 0 {
			return nil, false
		}
		switch v.Raw[0] {
		case protocol.PayloadBlock:
			out.Kind = payloadKindBlock
			if len(v.Raw) >= 4 {
				out.BlockID = int16(uint16(v.Raw[1])<<8 | uint16(v.Raw[2]))
			}
			return out, true
		case protocol.PayloadUnit:
			out.Kind = payloadKindUnit
			if len(v.Raw) >= 3 {
				if entity, ok := decodeRawUnitPayloadEntity(v.Raw[2:], v.Raw[1]); ok && entity != nil {
					out.UnitTypeID = entity.TypeID
					out.Rotation = buildRotationFromDegrees(entity.Rotation)
					out.Health = entity.Health
					out.MaxHealth = entity.MaxHealth
					clone := cloneRawEntity(*entity)
					out.UnitState = &clone
				}
			}
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
	name := w.blockNameByID(int16(tile.Block))
	switch name {
	case "conveyor", "titanium-conveyor", "armored-conveyor":
		return 3
	case "duct", "armored-duct", "duct-router", "overflow-duct", "underflow-duct":
		return 1
	case "combustion-generator", "steam-generator", "differential-generator", "rtg-generator":
		return 10
	case "graphite-press", "silicon-smelter", "kiln", "plastanium-compressor", "cryofluid-mixer", "pyratite-mixer", "blast-mixer", "separator", "pulverizer", "coal-centrifuge", "spore-press", "cultivator", "oxidation-chamber", "phase-heater":
		return 10
	case "neoplasia-reactor":
		return 10
	case "mechanical-drill", "pneumatic-drill", "laser-drill":
		return 10
	case "blast-drill":
		return 20
	case "plasma-bore":
		return 10
	case "large-plasma-bore":
		return 20
	case "impact-drill":
		return 40
	case "eruption-drill":
		return 60
	case "oil-extractor", "melter", "disassembler", "slag-centrifuge":
		return 10
	case "multi-press", "surge-smelter", "carbide-crucible", "surge-crucible", "heat-reactor":
		return 20
	case "phase-weaver", "silicon-crucible", "silicon-arc-furnace":
		return 30
	case "phase-synthesizer":
		return 40
	case "ground-factory", "air-factory", "naval-factory":
		return unitFactoryScaledAmount(unitFactoryTotalItemCapacity(name), w.unitCostMultiplierLocked(tile.Team))
	case "small-deconstructor", "deconstructor", "payload-deconstructor":
		if prof, ok := payloadDeconstructorProfileByName(name); ok {
			return prof.ItemCapacity
		}
		return 0
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
		if isReconstructorBlockName(name) {
			return w.reconstructorItemCapacityLocked(tile)
		}
		return 0
	}
}

func (w *World) liquidCapacityForBlockLocked(tile *Tile) float32 {
	if tile == nil || tile.Block == 0 {
		return 0
	}
	name := w.blockNameByID(int16(tile.Block))
	switch name {
	case "conduit":
		return 20
	case "pulse-conduit":
		return 40
	case "plated-conduit", "reinforced-conduit":
		return 50
	case "multi-press", "plastanium-compressor", "coal-centrifuge", "spore-press":
		return 60
	case "thorium-reactor":
		return 30
	case "mechanical-pump":
		return 20
	case "rotary-pump":
		return 80
	case "impulse-pump":
		return 200
	case "water-extractor", "oil-extractor":
		return 40
	case "cryofluid-mixer":
		return 36
	case "electrolyzer":
		return 50
	case "melter":
		return 10
	case "slag-incinerator":
		return 10
	case "separator":
		return 40
	case "slag-centrifuge":
		return 80
	case "repair-turret":
		return 96
	case "unit-repair-tower":
		return 30
	case "plasma-bore":
		return 10
	case "large-plasma-bore":
		return 30
	case "impact-drill":
		return 100
	case "eruption-drill":
		return 40
	case "heat-reactor":
		return 10
	case "surge-crucible":
		return 80 * 5
	case "slag-heater":
		return 120
	case "neoplasia-reactor":
		return 80
	case "vent-condenser":
		return 60
	case "atmospheric-concentrator":
		return 60
	case "oxidation-chamber":
		return 30
	case "turbine-condenser":
		return 20
	case "chemical-combustion-chamber":
		return 20 * 5
	case "pyrolysis-generator":
		return 30 * 5
	case "flux-reactor":
		return 30
	case "cyanogen-synthesizer":
		return 80
	case "phase-synthesizer":
		return 10 * 4
	case "cultivator":
		return 80
	case "disassembler":
		return 12
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
		if prof, ok := reconstructorProfileByName(name); ok {
			return reconstructorLiquidCapacity(prof)
		}
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
		reactorItemCapacity = float32(30)
		// Mindustry 156 Blocks.thoriumReactor:
		// itemDuration = 360f, heating = 0.02f, heatOutput = 15f, heatWarmupRate = 1f
		reactorHeatingPerFrame    = float32(0.02)
		reactorItemDurationFrames = float32(360)
		reactorHeatOutput         = float32(15)
		reactorHeatWarmupRate     = float32(1)
		reactorAmbientCooldown    = float32(60 * 20)
		reactorCoolantPower       = float32(0.5)
		reactorExplosionRadius    = 19
		reactorExplosionDamage    = float32(1250 * 4)
		reactorSmokeThreshold     = float32(0.3)
		reactorSmokeRadius        = float32(12)
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
		fuel := itemAmountOneOf(tile.Build, thoriumItemID, legacyThoriumItemID)
		fullness := clampf(float32(fuel)/reactorItemCapacity, 0, 1)

		if fuel > 0 {
			state.Heat += fullness * reactorHeatingPerFrame * minf(deltaFrames, 4)
			state.FuelProgress += deltaFrames
			for state.FuelProgress >= reactorItemDurationFrames {
				if !removeOneItemOfLocked(tile.Build, thoriumItemID, legacyThoriumItemID) {
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

		if state.Heat > reactorSmokeThreshold {
			smoke := 1 + (state.Heat-reactorSmokeThreshold)/(1-reactorSmokeThreshold)
			chance := clampf((smoke/20)*deltaFrames, 0, 1)
			if rand.Float32() < chance {
				cx := float32(tile.X*8 + 4)
				cy := float32(tile.Y*8 + 4)
				w.emitEffectLocked(
					"reactorsmoke",
					cx+(rand.Float32()*2-1)*reactorSmokeRadius,
					cy+(rand.Float32()*2-1)*reactorSmokeRadius,
					0,
				)
			}
		}

		state.Heat = clampf(state.Heat, 0, 1)
		state.HeatProgress = approachf(state.HeatProgress, state.Heat*reactorHeatOutput, reactorHeatWarmupRate*deltaFrames)
		if state.HeatProgress <= 0.0001 {
			delete(w.heatStates, pos)
		} else {
			w.heatStates[pos] = state.HeatProgress
		}
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
	w.emitEffectLocked("reactorexplosion", float32(x*8+4), float32(y*8+4), 0)
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

func (w *World) syncConveyorInventoryLocked(pos int32, tile *Tile, st *conveyorRuntimeState) {
	if tile == nil || tile.Build == nil || st == nil {
		return
	}
	if st.Len <= 0 {
		if len(tile.Build.Items) != 0 {
			w.replaceBuildingItemsLocked(pos, tile, nil)
		}
		st.Len = 0
		st.MinItem = 1
		st.LastInserted = -1
		st.Mid = 0
		return
	}

	var (
		ids    [3]ItemID
		counts [3]int32
		used   int
	)
	for i := 0; i < st.Len; i++ {
		item := st.IDs[i]
		matched := false
		for j := 0; j < used; j++ {
			if ids[j] == item {
				counts[j]++
				matched = true
				break
			}
		}
		if !matched && used < len(ids) {
			ids[used] = item
			counts[used] = 1
			used++
		}
	}

	if len(tile.Build.Items) == used {
		matches := true
		for i := 0; i < used; i++ {
			found := false
			for _, stack := range tile.Build.Items {
				if stack.Item == ids[i] && stack.Amount == counts[i] {
					found = true
					break
				}
			}
			if !found {
				matches = false
				break
			}
		}
		if matches {
			return
		}
	}

	items := make([]ItemStack, 0, used)
	for i := 0; i < used; i++ {
		items = append(items, ItemStack{Item: ids[i], Amount: counts[i]})
	}
	w.replaceBuildingItemsLocked(pos, tile, items)
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
	w.syncConveyorInventoryLocked(toPos, toTile, st)
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
	if !w.addItemAtLocked(toPos, item, 1) {
		return false
	}
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
	if !w.addItemAtLocked(toPos, item, 1) {
		return false
	}
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
	if !w.addItemAtLocked(toPos, item, 1) {
		return false
	}
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

func isItemLogisticsBlockName(name string) bool {
	switch name {
	case "conveyor", "titanium-conveyor", "armored-conveyor",
		"duct", "armored-duct", "duct-router", "overflow-duct", "underflow-duct", "duct-bridge", "duct-unloader",
		"router", "distributor",
		"bridge-conveyor", "phase-conveyor",
		"plastanium-conveyor", "surge-conveyor", "surge-router",
		"unloader", "mass-driver":
		return true
	default:
		return false
	}
}

func (w *World) stepItemLogistics(delta time.Duration, profileDetails bool) itemLogisticsPerf {
	var perf itemLogisticsPerf
	if w.model == nil {
		return perf
	}
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return perf
	}
	var junctionStartedAt time.Time
	if profileDetails {
		junctionStartedAt = time.Now()
	}
	w.stepJunctions(dt)
	if profileDetails {
		perf.Junctions = time.Since(junctionStartedAt)
		perf.JunctionCount = len(w.junctionQueues)
	}
	for _, pos := range w.itemLogisticsTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Build == nil || tile.Block == 0 {
			continue
		}
		name := w.blockNameByID(int16(tile.Block))
		switch name {
		case "conveyor":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepConveyorLocked(pos, tile, 0.03, dt)
			if profileDetails {
				perf.Conveyor += time.Since(startedAt)
				perf.ConveyorCount++
			}
		case "titanium-conveyor", "armored-conveyor":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepConveyorLocked(pos, tile, 0.08, dt)
			if profileDetails {
				perf.Conveyor += time.Since(startedAt)
				perf.ConveyorCount++
			}
		case "duct":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepDuctLocked(pos, tile, 4, false, dt)
			if profileDetails {
				perf.Duct += time.Since(startedAt)
				perf.DuctCount++
			}
		case "armored-duct":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepDuctLocked(pos, tile, 4, true, dt)
			if profileDetails {
				perf.Duct += time.Since(startedAt)
				perf.DuctCount++
			}
		case "duct-router":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepDuctRouterLocked(pos, tile, 4, false, dt)
			if profileDetails {
				perf.Router += time.Since(startedAt)
				perf.RouterCount++
			}
		case "overflow-duct":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepOverflowDuctLocked(pos, tile, 4, false, dt)
			if profileDetails {
				perf.Router += time.Since(startedAt)
				perf.RouterCount++
			}
		case "underflow-duct":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepOverflowDuctLocked(pos, tile, 4, true, dt)
			if profileDetails {
				perf.Router += time.Since(startedAt)
				perf.RouterCount++
			}
		case "duct-bridge":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepDuctBridgeLocked(pos, tile, 4, dt)
			if profileDetails {
				perf.Bridge += time.Since(startedAt)
				perf.BridgeCount++
			}
		case "duct-unloader":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepDirectionalUnloaderLocked(pos, tile, 4, dt)
			if profileDetails {
				perf.Unloader += time.Since(startedAt)
				perf.UnloaderCount++
			}
		case "router", "distributor":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepRouterLocked(pos, tile, 8, dt)
			if profileDetails {
				perf.Router += time.Since(startedAt)
				perf.RouterCount++
			}
		case "bridge-conveyor":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepBridgeConveyorLocked(pos, tile, 11, dt)
			if profileDetails {
				perf.Bridge += time.Since(startedAt)
				perf.BridgeCount++
			}
		case "phase-conveyor":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepPhaseConveyorLocked(pos, tile, dt)
			if profileDetails {
				perf.Bridge += time.Since(startedAt)
				perf.BridgeCount++
			}
		case "plastanium-conveyor":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepStackConveyorLocked(pos, tile, 4.0/60.0, 2, true, dt)
			if profileDetails {
				perf.Conveyor += time.Since(startedAt)
				perf.ConveyorCount++
			}
		case "surge-conveyor":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepStackConveyorLocked(pos, tile, 5.0/60.0, 2, false, dt)
			if profileDetails {
				perf.Conveyor += time.Since(startedAt)
				perf.ConveyorCount++
			}
		case "surge-router":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepStackRouterLocked(pos, tile, 6, dt)
			if profileDetails {
				perf.Router += time.Since(startedAt)
				perf.RouterCount++
			}
		case "unloader":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepUnloaderLocked(pos, tile, dt)
			if profileDetails {
				perf.Unloader += time.Since(startedAt)
				perf.UnloaderCount++
			}
		case "mass-driver":
			var startedAt time.Time
			if profileDetails {
				startedAt = time.Now()
			}
			w.stepMassDriverLocked(pos, tile, dt)
			if profileDetails {
				perf.MassDrive += time.Since(startedAt)
				perf.MassDriveCount++
			}
		}
	}
	var massShotStartedAt time.Time
	if profileDetails {
		massShotStartedAt = time.Now()
	}
	w.stepMassDriverShotsLocked(dt)
	if profileDetails {
		perf.MassDrive += time.Since(massShotStartedAt)
		if len(w.massDriverShots) > 0 {
			perf.MassDriveCount += len(w.massDriverShots)
		}
	}
	return perf
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
	if payload == nil {
		tile.Build.Payload = nil
		return
	}
	serialized, ok := w.serializePayloadDataLocked(payload)
	if !ok || len(serialized) == 0 {
		tile.Build.Payload = nil
		return
	}
	tile.Build.Payload = append(tile.Build.Payload[:0], serialized...)
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
	if payload.Kind == payloadKindUnit {
		size := w.payloadWorldSizeLocked(payload) / 8
		if size > 0 {
			if blocks := int(math.Ceil(float64(size - 0.001))); blocks > 1 {
				return blocks
			}
			return 1
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
	case "payload-void":
		return w.payloadVoidAcceptsPayloadLocked(toPos)
	case "small-deconstructor", "deconstructor", "payload-deconstructor":
		return w.payloadDeconstructorAcceptsPayloadLocked(toPos, toTile, payload)
	default:
		if isReconstructorBlockName(w.blockNameByID(int16(toTile.Block))) {
			return w.reconstructorAcceptsPayloadLocked(toPos, toTile, payload, fromPos, true)
		}
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
		case "ground-factory", "air-factory", "naval-factory":
			w.stepUnitFactoryPayloadLocked(pos, tile, frames)
		case "payload-conveyor", "reinforced-payload-conveyor":
			w.stepPayloadConveyorLocked(pos, tile, payloadMoveTimeByName(w.blockNameByID(int16(tile.Block))), frames)
		case "payload-router", "reinforced-payload-router":
			w.stepPayloadRouterLocked(pos, tile, payloadMoveTimeByName(w.blockNameByID(int16(tile.Block))), frames)
		case "payload-void":
			w.stepPayloadVoidLocked(pos, tile, frames)
		case "small-deconstructor", "deconstructor", "payload-deconstructor":
			w.stepPayloadDeconstructorLocked(pos, tile, frames)
		case "payload-mass-driver":
			w.stepPayloadMassDriverLocked(pos, tile, 130, 90, frames)
		case "large-payload-mass-driver":
			w.stepPayloadMassDriverLocked(pos, tile, 130, 100, frames)
		case "payload-loader":
			w.stepPayloadLoaderLocked(pos, tile, frames)
		case "payload-unloader":
			w.stepPayloadUnloaderLocked(pos, tile, frames)
		default:
			if isReconstructorBlockName(w.blockNameByID(int16(tile.Block))) {
				w.stepReconstructorLocked(pos, tile, frames)
			}
		}
	}
	w.stepPayloadDriverShotsLocked(frames)
}

func (w *World) stepUnitFactoryPayloadLocked(pos int32, tile *Tile, frames float32) {
	st := w.payloadStateLocked(pos)
	if st.Payload == nil {
		st.Move = 0
		w.syncPayloadTileLocked(tile, nil)
		return
	}
	moveTime := w.unitBlockPayloadMoveFramesLocked(tile)
	st.Move += frames
	if st.Move < moveTime {
		w.syncPayloadTileLocked(tile, st.Payload)
		return
	}
	if targetPos, ok := w.payloadFrontTargetLocked(pos, tile, tile.Rotation); ok && w.payloadMoveOutLocked(pos, tile, targetPos) {
		return
	}
	if w.dumpUnitPayloadFromTileLocked(pos, tile) {
		return
	}
	st.Move = moveTime
	w.syncPayloadTileLocked(tile, st.Payload)
}

func (w *World) unitBlockPayloadMoveFramesLocked(tile *Tile) float32 {
	size := float32(w.blockSizeForTileLocked(tile))
	// Match PayloadBlock payloadSpeed=0.7f closely enough for server-side timing.
	frames := (size * 8 / 2) / 0.7
	if frames < 1 {
		return 1
	}
	return frames
}

func (w *World) dumpUnitPayloadFromTileLocked(pos int32, tile *Tile) bool {
	if tile == nil || tile.Build == nil || w.model == nil {
		return false
	}
	state := w.payloadStateLocked(pos)
	payload := state.Payload
	if payload == nil || payload.Kind != payloadKindUnit {
		return false
	}
	typeID := payload.UnitTypeID
	if typeID <= 0 {
		if st, ok := w.factoryStates[pos]; ok && st.UnitType > 0 {
			typeID = st.UnitType
		}
	}
	if typeID <= 0 {
		return false
	}
	rotation := float32(tile.Rotation) * 90
	dist := float32(w.blockSizeForTileLocked(tile))*4 + 0.1
	rad := float32(rotation * math.Pi / 180)
	spawnX := float32(tile.X*8+4) + float32(math.Cos(float64(rad)))*dist
	spawnY := float32(tile.Y*8+4) + float32(math.Sin(float64(rad)))*dist
	if !w.canDumpProducedUnitLocked(tile.Build.Team, typeID, spawnX, spawnY, rotation) {
		return false
	}
	ent := w.newProducedUnitEntityLocked(typeID, tile.Build.Team, spawnX, spawnY, rotation)
	commandPos, command := w.unitCommandStateAtLocked(pos)
	if command != nil {
		ent.CommandID = command.ID
	}
	if commandPos != nil {
		ent.Behavior = "move"
		ent.TargetID = 0
		ent.PatrolAX = commandPos.X
		ent.PatrolAY = commandPos.Y
	}
	w.model.AddEntity(ent)
	w.clearPayloadLocked(pos, tile)
	return true
}

func (w *World) canDumpProducedUnitLocked(team TeamID, typeID int16, x, y, rotation float32) bool {
	if team == 0 || typeID <= 0 {
		return false
	}
	counts := map[TeamID]map[int16]int32{
		team: map[int16]int32{typeID: w.teamUnitCountByTypeLocked(team, typeID)},
	}
	if !w.canCreateUnitLocked(team, typeID, w.rulesMgr.Get(), nil, counts) {
		return false
	}
	dx := float32(math.Cos(float64(rotation * math.Pi / 180)))
	dy := float32(math.Sin(float64(rotation * math.Pi / 180)))
	tx := int((x + dx) / 8)
	ty := int((y + dy) / 8)
	if _, ok := w.buildingOccupyingCellLocked(tx, ty); ok {
		return false
	}
	tmp := w.newProducedUnitEntityLocked(typeID, team, x, y, rotation)
	if !isEntityFlying(tmp) {
		selfRadius := tmp.HitRadius
		if selfRadius <= 0 {
			selfRadius = entityHitRadiusForType(typeID)
		}
		maxDist := selfRadius * 1.05
		for _, other := range w.model.Entities {
			if other.Health <= 0 || isEntityFlying(other) {
				continue
			}
			otherRadius := other.HitRadius
			if otherRadius <= 0 {
				otherRadius = entityHitRadiusForType(other.TypeID)
			}
			limit := maxDist + otherRadius*0.5
			dx := other.X - x
			dy := other.Y - y
			if dx*dx+dy*dy <= limit*limit {
				return false
			}
		}
	}
	return true
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
	powerUse := float32(0.5)
	if w.blockNameByID(int16(tile.Block)) == "large-payload-mass-driver" {
		powerUse = 3
	}
	if !w.requirePowerAtLocked(pos, tile.Team, powerUse*(frames/60)) {
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
		w.syncConveyorInventoryLocked(pos, tile, st)
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
					w.syncConveyorInventoryLocked(nextPos, &w.model.Tiles[nextPos], nextState)
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
	w.syncConveyorInventoryLocked(pos, tile, st)
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
	if !w.removeItemAtLocked(pos, st.Current, 1) {
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
				if !w.removeItemAtLocked(pos, sst.LastItem, 1) {
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
	if !w.removeItemAtLocked(pos, st.Current, 1) {
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
	if !w.removeItemAtLocked(pos, st.Current, 1) {
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
			if !w.removeItemAtLocked(pos, item, 1) {
				break
			}
			if !w.addItemAtLocked(target, item, 1) {
				break
			}
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
	_ = w.removeItemAtLocked(pos, item, 1)
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
		if !w.removeItemAtLocked(backPos, item, 1) {
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
	for _, item := range w.rotatedInventoryItemIDsLocked(backPos, w.blockDumpIndex[pos]) {
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
	if w.blockNameByID(int16(tile.Block)) == "surge-conveyor" && !w.requirePowerAtLocked(pos, tile.Team, (1.0/60.0)*dt) {
		return
	}
	frontPos, hasFront := w.forwardItemTargetPosLocked(pos, tile.Rotation)
	if hasFront {
		frontTile := &w.model.Tiles[frontPos]
		if frontTile.Build != nil && frontTile.Team == tile.Team && isStackConveyorBlock(w.blockNameByID(int16(frontTile.Block))) && st.Cooldown <= 0 {
			frontState := w.stackStateLocked(frontPos, frontTile)
			if frontState.Link < 0 && (!outputRouter || totalBuildingItems(tile.Build) >= w.itemCapacityForBlockLocked(tile) || st.Link != pos) {
				w.replaceBuildingItemsLocked(frontPos, frontTile, tile.Build.Items)
				frontState.LastItem = st.LastItem
				frontState.HasItem = true
				frontState.Link = pos
				frontState.Cooldown = 1
				w.replaceBuildingItemsLocked(pos, tile, nil)
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
			_ = w.removeItemAtLocked(pos, st.LastItem, 1)
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
	if w.blockNameByID(int16(tile.Block)) == "surge-router" && !w.requirePowerAtLocked(pos, tile.Team, (3.0/60.0)*dt) {
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
		if !w.removeItemAtLocked(pos, sst.LastItem, 1) {
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
	_ = w.removeItemAtLocked(pos, st.LastItem, 1)
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
		if !w.removeItemAtLocked(pos, item, 1) {
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
		if !w.removeItemAtLocked(pos, item, 1) {
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
	if !w.requirePowerAtLocked(pos, tile.Team, 1.75*dt) {
		return
	}
	items := w.massDriverTakePayloadLocked(pos, tile, 120)
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
	srcX := float32(tile.X*8 + 4)
	srcY := float32(tile.Y*8 + 4)
	dstX := float32(targetTile.X*8 + 4)
	dstY := float32(targetTile.Y*8 + 4)
	angle := lookAt(srcX, srcY, dstX, dstY)
	rad := float32(angle * math.Pi / 180)
	const massDriverEffectOffset = float32(7)
	w.emitEffectLocked("shootbig2", srcX+float32(math.Cos(float64(rad)))*massDriverEffectOffset, srcY+float32(math.Sin(float64(rad)))*massDriverEffectOffset, angle)
	w.emitEffectLocked("shootbigsmoke2", srcX+float32(math.Cos(float64(rad)))*massDriverEffectOffset, srcY+float32(math.Sin(float64(rad)))*massDriverEffectOffset, angle)
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
			if !w.addItemAtLocked(shot.ToPos, stack.Item, amount) {
				continue
			}
			total += amount
		}
		w.massDriverStateLocked(shot.ToPos).ReloadCounter = 1
		w.emitEffectLocked("minebig", float32(targetTile.X*8+4), float32(targetTile.Y*8+4), 0)
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
		return w.addItemAtLocked(toPos, item, 1)
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
		if !w.addItemAtLocked(toPos, item, 1) {
			return false
		}
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
		return w.storeAcceptedBuildingItemLocked(toPos, toTile, item, 1)
	case "item-void":
		return true
	case "incinerator", "slag-incinerator":
		if !w.incineratorAcceptsItemLocked(toPos) {
			return false
		}
		w.incineratorBurnItemLocked(toPos)
		return true
	default:
		return w.storeAcceptedBuildingItemLocked(toPos, toTile, item, 1)
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
	case "liquid-void":
		return true
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
	case "incinerator":
		return w.incineratorAcceptsLiquidLocked(toPos, liquid)
	case "slag-incinerator":
		return liquid == slagLiquidID && w.incineratorAcceptsLiquidLocked(toPos, liquid)
	case "repair-turret":
		return repairTurretAcceptsLiquid(liquid) && w.liquidCanStoreLocked(toTile, liquid)
	case "unit-repair-tower":
		return liquid == ozoneLiquidID && w.liquidCanStoreLocked(toTile, liquid)
	case "plasma-bore":
		return liquid == hydrogenLiquidID && w.liquidCanStoreLocked(toTile, liquid)
	case "large-plasma-bore":
		return (liquid == hydrogenLiquidID || liquid == nitrogenLiquidID) && w.liquidCanStoreLocked(toTile, liquid)
	case "impact-drill":
		return (liquid == waterLiquidID || liquid == ozoneLiquidID) && w.liquidCanStoreLocked(toTile, liquid)
	case "eruption-drill":
		return (liquid == hydrogenLiquidID || liquid == cyanogenLiquidID) && w.liquidCanStoreLocked(toTile, liquid)
	default:
		if isReconstructorBlockName(name) {
			return w.reconstructorAcceptsLiquidLocked(toTile, liquid)
		}
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
	if name == "incinerator" {
		w.incineratorBurnLiquidLocked(toPos)
		return amount
	}
	if name == "liquid-void" {
		return amount
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
	if len(w.bridgeIncomingMask) == 0 && len(w.bridgeLinks) > 0 {
		mask := make(map[int32]byte, len(w.bridgeLinks))
		for otherPos, target := range w.bridgeLinks {
			if target < 0 || otherPos < 0 || int(target) >= len(w.model.Tiles) || int(otherPos) >= len(w.model.Tiles) {
				continue
			}
			bridgeTile := &w.model.Tiles[target]
			otherTile := &w.model.Tiles[otherPos]
			if bridgeTile.Build == nil || otherTile.Build == nil || otherTile.Team != bridgeTile.Team {
				continue
			}
			bridgeName := w.blockNameByID(int16(bridgeTile.Block))
			if w.blockNameByID(int16(otherTile.Block)) != bridgeName {
				continue
			}
			incomingSide, ok := axisDir(otherTile.X, otherTile.Y, bridgeTile.X, bridgeTile.Y)
			if !ok || incomingSide >= 8 {
				continue
			}
			mask[target] |= 1 << incomingSide
		}
		w.bridgeIncomingMask = mask
	}
	return (w.bridgeIncomingMask[bridgePos] & (1 << side)) != 0
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

func (w *World) massDriverTakePayloadLocked(pos int32, tile *Tile, limit int32) []ItemStack {
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
		if w.removeItemAtLocked(pos, stack.Item, amount) {
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

func affectsCoreStorageLinks(name string) bool {
	return isCoreBlockName(name) || isCoreMergeStorageBlock(name)
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

	for team, cores := range w.teamCoreTiles {
		if len(cores) == 0 {
			continue
		}
		cores = append([]int32(nil), cores...)
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

		queue := append([]int32(nil), cores...)
		for len(queue) > 0 {
			anchorPos := queue[0]
			queue = queue[1:]
			if anchorPos < 0 || int(anchorPos) >= len(w.model.Tiles) {
				continue
			}
			anchor := &w.model.Tiles[anchorPos]
			for _, otherPos := range w.teamBuildingTiles[team] {
				if otherPos < 0 || int(otherPos) >= len(w.model.Tiles) || otherPos == anchorPos {
					continue
				}
				if _, exists := ownedStorages[otherPos]; exists {
					continue
				}
				other := &w.model.Tiles[otherPos]
				if other.Build == nil || other.Block == 0 || other.Team != team {
					continue
				}
				if !isCoreMergeStorageBlock(w.blockNameByID(int16(other.Block))) {
					continue
				}
				if !w.storageFootprintsTouchLocked(anchor, other) {
					continue
				}
				ownedStorages[otherPos] = struct{}{}
				w.storageLinkedCore[otherPos] = primary
				totalCapacity += w.itemCapacityForBlockLocked(other)
				for _, stack := range other.Build.Items {
					if stack.Amount > 0 {
						mergedItems[stack.Item] += stack.Amount
					}
				}
				queue = append(queue, otherPos)
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

func (w *World) storageFootprintsTouchLocked(a, b *Tile) bool {
	if w == nil || a == nil || b == nil {
		return false
	}
	lowA, highA := blockFootprintRange(w.blockSizeForTileLocked(a))
	lowB, highB := blockFootprintRange(w.blockSizeForTileLocked(b))
	ax1, ax2 := a.X+lowA, a.X+highA
	ay1, ay2 := a.Y+lowA, a.Y+highA
	bx1, bx2 := b.X+lowB, b.X+highB
	by1, by2 := b.Y+lowB, b.Y+highB
	return ax1 <= bx2+1 && bx1 <= ax2+1 && ay1 <= by2+1 && by1 <= ay2+1
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
	if w.buildingHidesInventoryItemsLocked(pos, tile) {
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
	if w.buildingHidesInventoryItemsLocked(pos, tile) {
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
	w.emitBlockItemSyncLocked(pos)
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
	if w.buildingHidesInventoryItemsLocked(pos, tile) {
		return false
	}
	if !tile.Build.RemoveItem(item, amount) {
		return false
	}
	w.emitBlockItemSyncLocked(pos)
	return true
}

func (w *World) unloaderTargetItemLocked(pos int32, neighbors []int32) (ItemID, bool) {
	if item, ok := w.unloaderCfg[pos]; ok {
		if _, _, found := w.unloaderTransferPairPreviewLocked(pos, neighbors, item); found {
			return item, true
		}
		return 0, false
	}
	for _, item := range w.rotatedUnloaderCandidateItemIDsLocked(pos, neighbors) {
		if _, _, found := w.unloaderTransferPairPreviewLocked(pos, neighbors, item); found {
			w.blockDumpIndex[pos] = int(item)
			return item, true
		}
	}
	return 0, false
}

func (w *World) rotatedUnloaderCandidateItemIDsLocked(pos int32, neighbors []int32) []ItemID {
	start := 0
	if idx, ok := w.blockDumpIndex[pos]; ok {
		start = idx
	}
	seen := make(map[ItemID]struct{}, 16)
	items := make([]ItemID, 0, 16)
	for _, otherPos := range neighbors {
		if otherPos < 0 || int(otherPos) >= len(w.model.Tiles) || otherPos == pos {
			continue
		}
		other := &w.model.Tiles[otherPos]
		if other.Build == nil || other.Team != w.model.Tiles[pos].Team {
			continue
		}
		w.appendInventoryItemIDsLocked(otherPos, &items, seen)
	}
	return rotateItemIDsByStart(items, start)
}

func (w *World) rotatedInventoryItemIDsLocked(pos int32, start int) []ItemID {
	items := make([]ItemID, 0, 8)
	seen := make(map[ItemID]struct{}, 8)
	w.appendInventoryItemIDsLocked(pos, &items, seen)
	return rotateItemIDsByStart(items, start)
}

func (w *World) appendInventoryItemIDsLocked(pos int32, dst *[]ItemID, seen map[ItemID]struct{}) {
	if seen == nil || w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return
	}
	if _, _, build, ok := w.sharedCoreInventoryLocked(pos); ok {
		for _, stack := range build.Items {
			if stack.Amount <= 0 {
				continue
			}
			if _, exists := seen[stack.Item]; exists {
				continue
			}
			seen[stack.Item] = struct{}{}
			*dst = append(*dst, stack.Item)
		}
		return
	}
	tile := &w.model.Tiles[pos]
	if tile.Build == nil {
		return
	}
	if w.buildingHidesInventoryItemsLocked(pos, tile) {
		return
	}
	for _, stack := range tile.Build.Items {
		if stack.Amount <= 0 {
			continue
		}
		if _, exists := seen[stack.Item]; exists {
			continue
		}
		seen[stack.Item] = struct{}{}
		*dst = append(*dst, stack.Item)
	}
}

func rotateItemIDsByStart(items []ItemID, start int) []ItemID {
	if len(items) == 0 {
		return nil
	}
	sort.Slice(items, func(i, j int) bool {
		return items[i] < items[j]
	})
	if start <= 0 {
		return items
	}
	split := 0
	for split < len(items) && int(items[split]) < start {
		split++
	}
	if split == 0 || split >= len(items) {
		return items
	}
	out := make([]ItemID, 0, len(items))
	out = append(out, items[split:]...)
	out = append(out, items[:split]...)
	return out
}

func boolCompare(a, b bool) int {
	switch {
	case a == b:
		return 0
	case !a && b:
		return -1
	default:
		return 1
	}
}

func unloaderLastUsedKey(unloaderPos, otherPos int32) int64 {
	return int64(uint32(unloaderPos))<<32 | int64(uint32(otherPos))
}

func (w *World) unloaderTransferPairPreviewLocked(pos int32, neighbors []int32, item ItemID) (int32, int32, bool) {
	return w.unloaderTransferPairInternalLocked(pos, neighbors, item, false)
}

func (w *World) unloaderTransferPairLocked(pos int32, neighbors []int32, item ItemID) (int32, int32, bool) {
	return w.unloaderTransferPairInternalLocked(pos, neighbors, item, true)
}

func (w *World) unloaderTransferPairInternalLocked(pos int32, neighbors []int32, item ItemID, updateLastUsed bool) (int32, int32, bool) {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return 0, 0, false
	}

	stats := make([]unloaderCandidateStat, 0, len(neighbors))
	hasProvider := false
	hasReceiver := false
	isDistinct := false

	for _, otherPos := range neighbors {
		if otherPos < 0 || int(otherPos) >= len(w.model.Tiles) || otherPos == pos {
			continue
		}
		other := &w.model.Tiles[otherPos]
		if other.Build == nil || other.Block == 0 || other.Team != w.model.Tiles[pos].Team {
			continue
		}

		notStorage := !isStorageLikeBlock(w.blockNameByID(int16(other.Block)))
		canLoad := notStorage && w.canAcceptItemLocked(pos, otherPos, item, 0)
		canUnload := w.itemAmountAtLocked(otherPos, item) > 0
		if !canLoad && !canUnload {
			continue
		}

		isDistinct = isDistinct || (hasProvider && canLoad) || (hasReceiver && canUnload)
		hasProvider = hasProvider || canUnload
		hasReceiver = hasReceiver || canLoad

		cap := w.unloaderMaximumAcceptedItemLocked(otherPos, other, item)
		loadFactor := float32(0)
		if cap > 0 {
			loadFactor = float32(w.itemAmountAtLocked(otherPos, item)) / float32(cap)
		}

		lastUsed := w.unloaderLastUsed[unloaderLastUsedKey(pos, otherPos)] + 1
		if updateLastUsed {
			w.unloaderLastUsed[unloaderLastUsedKey(pos, otherPos)] = lastUsed
		}

		stats = append(stats, unloaderCandidateStat{
			pos:        otherPos,
			loadFactor: loadFactor,
			canLoad:    canLoad,
			canUnload:  canUnload,
			notStorage: notStorage,
			lastUsed:   lastUsed,
		})
	}

	if !isDistinct || len(stats) < 2 {
		return 0, 0, false
	}

	sort.SliceStable(stats, func(i, j int) bool {
		x, y := stats[i], stats[j]
		if cmp := boolCompare(!x.notStorage, !y.notStorage); cmp != 0 {
			return cmp < 0
		}
		if cmp := boolCompare(x.canUnload && !x.canLoad, y.canUnload && !y.canLoad); cmp != 0 {
			return cmp < 0
		}
		if cmp := boolCompare(x.canUnload || !x.canLoad, y.canUnload || !y.canLoad); cmp != 0 {
			return cmp < 0
		}
		if x.loadFactor != y.loadFactor {
			return x.loadFactor < y.loadFactor
		}
		return x.lastUsed > y.lastUsed
	})

	var dumpingTo *unloaderCandidateStat
	var dumpingFrom *unloaderCandidateStat
	for i := range stats {
		if stats[i].canLoad {
			dumpingTo = &stats[i]
			break
		}
	}
	for i := len(stats) - 1; i >= 0; i-- {
		if stats[i].canUnload {
			dumpingFrom = &stats[i]
			break
		}
	}

	if dumpingFrom == nil || dumpingTo == nil || dumpingFrom.pos == dumpingTo.pos {
		return 0, 0, false
	}
	if dumpingFrom.loadFactor == dumpingTo.loadFactor && dumpingFrom.canLoad {
		return 0, 0, false
	}
	if updateLastUsed {
		w.unloaderLastUsed[unloaderLastUsedKey(pos, dumpingTo.pos)] = 0
		w.unloaderLastUsed[unloaderLastUsedKey(pos, dumpingFrom.pos)] = 0
	}
	return dumpingFrom.pos, dumpingTo.pos, true
}

func (w *World) unloaderMaximumAcceptedItemLocked(pos int32, tile *Tile, item ItemID) int32 {
	if w == nil || tile == nil || tile.Build == nil || tile.Block == 0 {
		return 0
	}
	switch w.blockNameByID(int16(tile.Block)) {
	case "ground-factory", "air-factory", "naval-factory",
		"additive-reconstructor", "multiplicative-reconstructor", "exponential-reconstructor", "tetrative-reconstructor",
		"tank-refabricator", "ship-refabricator", "mech-refabricator", "prime-refabricator":
		return w.maximumAcceptedItemForBlockLocked(pos, tile, item)
	default:
		return w.itemCapacityAtLocked(pos)
	}
}

func (w *World) dumpProximityLocked(pos int32) []int32 {
	if w.model == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return nil
	}
	tile := &w.model.Tiles[pos]
	raw, ok := w.dumpNeighborCache[pos]
	if !ok {
		offsets := blockEdgeOffsets(w.blockSizeForTileLocked(tile))
		raw = make([]int32, 0, len(offsets))
		seen := make(map[int32]struct{}, len(offsets))
		for _, off := range offsets {
			otherPos, ok := w.buildingOccupyingCellLocked(tile.X+off[0], tile.Y+off[1])
			if !ok || otherPos == pos {
				continue
			}
			if _, exists := seen[otherPos]; exists {
				continue
			}
			seen[otherPos] = struct{}{}
			raw = append(raw, otherPos)
		}
		w.dumpNeighborCache[pos] = raw
	}
	if len(raw) == 0 {
		return nil
	}
	out := make([]int32, 0, len(raw))
	for _, otherPos := range raw {
		if otherPos < 0 || int(otherPos) >= len(w.model.Tiles) {
			continue
		}
		other := &w.model.Tiles[otherPos]
		if other.Build == nil || other.Block == 0 || other.Team != tile.Team {
			continue
		}
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
				w.emitBlockItemSyncLocked(pos)
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

func (w *World) offloadProducedItemLocked(pos int32, tile *Tile, item ItemID) bool {
	if tile == nil || tile.Build == nil || w.model == nil {
		return false
	}
	if target, ok := w.dumpTargetLocked(pos, tile, item); ok {
		if w.tryInsertItemLocked(pos, target, item, 0) {
			return true
		}
	}
	if totalBuildingItems(tile.Build) >= w.itemCapacityForBlockLocked(tile) {
		return false
	}
	tile.Build.AddItem(item, 1)
	w.emitBlockItemSyncLocked(pos)
	return true
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

func containsItemInStacks(stacks []ItemStack, item ItemID) bool {
	for _, stack := range stacks {
		if stack.Item == item && stack.Amount > 0 {
			return true
		}
	}
	return false
}

func (w *World) maximumAcceptedItemForBlockLocked(pos int32, tile *Tile, item ItemID) int32 {
	if w == nil || tile == nil || tile.Build == nil || tile.Block == 0 {
		return 0
	}
	name := w.blockNameByID(int16(tile.Block))
	switch name {
	case "core-shard", "core-foundation", "core-nucleus", "core-bastion", "core-citadel", "core-acropolis",
		"container", "vault", "reinforced-container", "reinforced-vault":
		return w.itemCapacityAtLocked(pos)
	case "payload-loader", "payload-unloader":
		return w.itemCapacityForBlockLocked(tile)
	case "ground-factory", "air-factory", "naval-factory":
		plan, ok := w.unitFactorySelectedPlanLocked(pos, tile)
		if !ok || !containsItemInStacks(plan.Cost, item) {
			return 0
		}
		return unitFactoryScaledAmount(unitFactoryItemCapacity(name, item), w.unitCostMultiplierLocked(tile.Build.Team))
	case "additive-reconstructor", "multiplicative-reconstructor", "exponential-reconstructor", "tetrative-reconstructor",
		"tank-refabricator", "ship-refabricator", "mech-refabricator", "prime-refabricator":
		return w.reconstructorMaximumAcceptedItemLocked(tile, item)
	case "combustion-generator", "steam-generator":
		if item == coalItemID || item == pyratiteItemID || item == sporePodItemID {
			return w.itemCapacityForBlockLocked(tile)
		}
		return 0
	case "differential-generator":
		if item == pyratiteItemID {
			return w.itemCapacityForBlockLocked(tile)
		}
		return 0
	case "rtg-generator":
		if item == phaseFabricItemID || item == thoriumItemID || item == legacyThoriumItemID {
			return w.itemCapacityForBlockLocked(tile)
		}
		return 0
	case "thorium-reactor":
		if item == thoriumItemID || item == legacyThoriumItemID {
			return w.itemCapacityForBlockLocked(tile)
		}
		return 0
	case "impact-reactor":
		if item == blastCompoundItemID {
			return w.itemCapacityForBlockLocked(tile)
		}
		return 0
	case "neoplasia-reactor":
		if item == phaseFabricItemID {
			return w.itemCapacityForBlockLocked(tile)
		}
		return 0
	}
	if prof, ok := crafterProfilesByBlockName[name]; ok {
		if containsItemInStacks(prof.InputItems, item) {
			return w.itemCapacityForBlockLocked(tile)
		}
		return 0
	}
	if prof, ok := separatorProfilesByBlockName[name]; ok {
		if containsItemInStacks(prof.InputItems, item) {
			return w.itemCapacityForBlockLocked(tile)
		}
		return 0
	}
	if prof, ok := solidPumpProfilesByBlockName[name]; ok {
		if prof.ItemUseTimeFrames > 0 && prof.ItemConsume == item {
			return w.itemCapacityForBlockLocked(tile)
		}
		return 0
	}
	return 0
}

func (w *World) buildingUsesTotalItemCapacityLocked(pos int32, tile *Tile) bool {
	if w == nil || tile == nil || tile.Build == nil || tile.Block == 0 {
		return false
	}
	if _, _, _, shared := w.sharedCoreInventoryLocked(pos); shared {
		return false
	}
	switch w.blockNameByID(int16(tile.Block)) {
	case "container", "vault", "reinforced-container", "reinforced-vault":
		return true
	default:
		return false
	}
}

func (w *World) acceptsBuildingItemLocked(pos int32, tile *Tile, item ItemID) bool {
	if tile != nil && tile.Build != nil {
		if prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block)); ok && w.buildingUsesItemAmmoLocked(tile, prof) {
			return w.turretAcceptItemLocked(pos, tile, item)
		}
	}
	cap := w.maximumAcceptedItemForBlockLocked(pos, tile, item)
	if cap <= 0 {
		return false
	}
	if w.buildingUsesTotalItemCapacityLocked(pos, tile) {
		return w.totalItemsAtLocked(pos) < cap
	}
	return w.itemAmountAtLocked(pos, item) < cap
}

func (w *World) storeAcceptedBuildingItemLocked(pos int32, tile *Tile, item ItemID, amount int32) bool {
	if amount <= 0 {
		return false
	}
	if tile != nil && tile.Build != nil {
		if prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block)); ok && w.buildingUsesItemAmmoLocked(tile, prof) {
			return w.turretHandleItemLocked(pos, tile, item, amount)
		}
	}
	cap := w.maximumAcceptedItemForBlockLocked(pos, tile, item)
	if cap <= 0 {
		return false
	}
	if w.buildingUsesTotalItemCapacityLocked(pos, tile) {
		if w.totalItemsAtLocked(pos)+amount > cap {
			return false
		}
	} else if w.itemAmountAtLocked(pos, item)+amount > cap {
		return false
	}
	return w.addItemAtLocked(pos, item, amount)
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
		return w.acceptsBuildingItemLocked(toPos, toTile, item)
	case "item-void":
		return true
	case "incinerator", "slag-incinerator":
		return w.incineratorAcceptsItemLocked(toPos)
	default:
		return w.acceptsBuildingItemLocked(toPos, toTile, item)
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
	case "mechanical-drill", "pneumatic-drill", "rotary-pump", "water-extractor", "graphite-press", "silicon-smelter", "kiln", "plastanium-compressor", "phase-weaver", "cryofluid-mixer", "pyratite-mixer", "blast-mixer", "separator", "coal-centrifuge", "spore-press", "cultivator", "electric-heater", "phase-heater":
		return 2
	case "repair-turret", "unit-repair-tower", "plasma-bore":
		return 2
	case "power-node-large", "surge-tower", "thermal-generator", "steam-generator", "rtg-generator", "multi-press", "surge-smelter", "small-heat-redirector":
		return 2
	case "container", "reinforced-container":
		return 2
	case "laser-drill", "impulse-pump", "oil-extractor", "melter", "silicon-crucible", "disassembler", "silicon-arc-furnace", "electrolyzer", "atmospheric-concentrator", "oxidation-chamber", "slag-heater", "vent-condenser", "slag-centrifuge", "heat-reactor", "turbine-condenser", "chemical-combustion-chamber", "pyrolysis-generator", "carbide-crucible", "surge-crucible", "cyanogen-synthesizer", "phase-synthesizer", "heat-redirector", "heat-router", "large-plasma-bore":
		return 3
	case "battery-large", "solar-panel-large", "differential-generator", "beam-tower", "beam-link":
		return 3
	case "core-shard", "vault", "reinforced-vault", "thorium-reactor", "mass-driver", "payload-conveyor", "reinforced-payload-conveyor", "payload-router", "reinforced-payload-router", "payload-mass-driver", "payload-loader", "payload-unloader", "additive-reconstructor", "tank-refabricator", "ship-refabricator", "mech-refabricator", "small-deconstructor":
		return 3
	case "ground-factory", "air-factory", "naval-factory":
		return 3
	case "blast-drill", "impact-drill":
		return 4
	case "core-foundation", "core-bastion", "impact-reactor":
		return 4
	case "core-nucleus", "core-citadel", "large-payload-mass-driver", "flux-reactor", "neoplasia-reactor", "multiplicative-reconstructor", "prime-refabricator", "deconstructor", "payload-deconstructor", "payload-void", "eruption-drill":
		return 5
	case "exponential-reconstructor":
		return 7
	case "tetrative-reconstructor":
		return 9
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

var blockEdgeOffsetCache = newBlockEdgeOffsetCache()

func newBlockEdgeOffsetCache() map[int][][2]int {
	cache := make(map[int][][2]int, 9)
	for size := 1; size <= 9; size++ {
		cache[size] = computeBlockEdgeOffsets(size)
	}
	return cache
}

func blockEdgeOffsets(size int) [][2]int {
	if cached, ok := blockEdgeOffsetCache[size]; ok {
		return cached
	}
	return computeBlockEdgeOffsets(size)
}

func computeBlockEdgeOffsets(size int) [][2]int {
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
	w.dumpNeighborCache = map[int32][]int32{}
	w.unloaderLastUsed = map[int64]int{}
	w.activeTilePositions = w.activeTilePositions[:0]
	w.itemLogisticsTilePositions = w.itemLogisticsTilePositions[:0]
	w.crafterTilePositions = w.crafterTilePositions[:0]
	w.drillTilePositions = w.drillTilePositions[:0]
	w.burstDrillTilePositions = w.burstDrillTilePositions[:0]
	w.beamDrillTilePositions = w.beamDrillTilePositions[:0]
	w.pumpTilePositions = w.pumpTilePositions[:0]
	w.incineratorTilePositions = w.incineratorTilePositions[:0]
	w.repairTurretTilePositions = w.repairTurretTilePositions[:0]
	w.repairTowerTilePositions = w.repairTowerTilePositions[:0]
	w.factoryTilePositions = w.factoryTilePositions[:0]
	w.heatConductorTilePositions = w.heatConductorTilePositions[:0]
	w.powerTilePositions = w.powerTilePositions[:0]
	w.powerDiodeTilePositions = w.powerDiodeTilePositions[:0]
	w.powerVoidTilePositions = w.powerVoidTilePositions[:0]
	w.turretTilePositions = w.turretTilePositions[:0]
	w.mendProjectorPositions = w.mendProjectorPositions[:0]
	w.overdriveProjectorPositions = w.overdriveProjectorPositions[:0]
	w.forceProjectorPositions = w.forceProjectorPositions[:0]
	w.teamBuildingTiles = map[TeamID][]int32{}
	w.teamBuildingSpatial = map[TeamID]*buildingSpatialIndex{}
	w.teamCoreTiles = map[TeamID][]int32{}
	w.teamPowerTiles = map[TeamID][]int32{}
	w.teamPowerNodeTiles = map[TeamID][]int32{}
	if w.model == nil {
		return
	}
	for i := range w.model.Tiles {
		tile := &w.model.Tiles[i]
		if !isCenterBuildingTile(tile) {
			continue
		}
		w.indexActiveTileLocked(int32(i), tile)
		w.setBuildingOccupancyLocked(int32(i), tile, true)
	}
	w.refreshCoreStorageLinksLocked()
}

func (w *World) rebuildActiveTilesLocked() {
	// CRITICAL: This function should ONLY be called on map load or major world changes
	// For individual building changes, use indexActiveTileLocked/removeActiveTileIndexLocked instead
	w.activeTilePositions = w.activeTilePositions[:0]
	w.itemLogisticsTilePositions = w.itemLogisticsTilePositions[:0]
	w.crafterTilePositions = w.crafterTilePositions[:0]
	w.drillTilePositions = w.drillTilePositions[:0]
	w.burstDrillTilePositions = w.burstDrillTilePositions[:0]
	w.beamDrillTilePositions = w.beamDrillTilePositions[:0]
	w.pumpTilePositions = w.pumpTilePositions[:0]
	w.incineratorTilePositions = w.incineratorTilePositions[:0]
	w.repairTurretTilePositions = w.repairTurretTilePositions[:0]
	w.repairTowerTilePositions = w.repairTowerTilePositions[:0]
	w.factoryTilePositions = w.factoryTilePositions[:0]
	w.heatConductorTilePositions = w.heatConductorTilePositions[:0]
	w.powerTilePositions = w.powerTilePositions[:0]
	w.powerDiodeTilePositions = w.powerDiodeTilePositions[:0]
	w.powerVoidTilePositions = w.powerVoidTilePositions[:0]
	w.turretTilePositions = w.turretTilePositions[:0]
	w.mendProjectorPositions = w.mendProjectorPositions[:0]
	w.overdriveProjectorPositions = w.overdriveProjectorPositions[:0]
	w.forceProjectorPositions = w.forceProjectorPositions[:0]
	w.teamBuildingTiles = map[TeamID][]int32{}
	w.teamBuildingSpatial = map[TeamID]*buildingSpatialIndex{}
	w.teamCoreTiles = map[TeamID][]int32{}
	w.teamPowerTiles = map[TeamID][]int32{}
	w.teamPowerNodeTiles = map[TeamID][]int32{}
	if w.model == nil {
		return
	}
	for i := range w.model.Tiles {
		tile := &w.model.Tiles[i]
		if !isCenterBuildingTile(tile) {
			continue
		}
		w.indexActiveTileLocked(int32(i), tile)
	}
}

// removeActiveTileIndexLocked removes a single tile from all active tile indices
// This is the incremental version that should be used instead of rebuildActiveTilesLocked
func (w *World) removeActiveTileIndexLocked(pos int32, tile *Tile) {
	if tile == nil {
		return
	}

	// Remove from activeTilePositions
	for i, p := range w.activeTilePositions {
		if p == pos {
			w.activeTilePositions = append(w.activeTilePositions[:i], w.activeTilePositions[i+1:]...)
			break
		}
	}

	name := w.blockNameByID(int16(tile.Block))

	// Remove from itemLogisticsTilePositions
	if isItemLogisticsBlockName(name) {
		for i, p := range w.itemLogisticsTilePositions {
			if p == pos {
				w.itemLogisticsTilePositions = append(w.itemLogisticsTilePositions[:i], w.itemLogisticsTilePositions[i+1:]...)
				break
			}
		}
	}

	// Remove from crafterTilePositions
	if _, ok := crafterProfilesByBlockName[name]; ok {
		for i, p := range w.crafterTilePositions {
			if p == pos {
				w.crafterTilePositions = append(w.crafterTilePositions[:i], w.crafterTilePositions[i+1:]...)
				break
			}
		}
	} else if _, ok := separatorProfilesByBlockName[name]; ok {
		for i, p := range w.crafterTilePositions {
			if p == pos {
				w.crafterTilePositions = append(w.crafterTilePositions[:i], w.crafterTilePositions[i+1:]...)
				break
			}
		}
	}

	// Remove from other tile position lists
	if _, ok := drillProfilesByBlockName[name]; ok {
		for i, p := range w.drillTilePositions {
			if p == pos {
				w.drillTilePositions = append(w.drillTilePositions[:i], w.drillTilePositions[i+1:]...)
				break
			}
		}
	}

	if _, ok := burstDrillProfilesByBlockName[name]; ok {
		for i, p := range w.burstDrillTilePositions {
			if p == pos {
				w.burstDrillTilePositions = append(w.burstDrillTilePositions[:i], w.burstDrillTilePositions[i+1:]...)
				break
			}
		}
	}

	if _, ok := beamDrillProfilesByBlockName[name]; ok {
		for i, p := range w.beamDrillTilePositions {
			if p == pos {
				w.beamDrillTilePositions = append(w.beamDrillTilePositions[:i], w.beamDrillTilePositions[i+1:]...)
				break
			}
		}
	}

	if _, ok := floorPumpProfilesByBlockName[name]; ok {
		for i, p := range w.pumpTilePositions {
			if p == pos {
				w.pumpTilePositions = append(w.pumpTilePositions[:i], w.pumpTilePositions[i+1:]...)
				break
			}
		}
	} else if _, ok := solidPumpProfilesByBlockName[name]; ok {
		for i, p := range w.pumpTilePositions {
			if p == pos {
				w.pumpTilePositions = append(w.pumpTilePositions[:i], w.pumpTilePositions[i+1:]...)
				break
			}
		}
	}

	if name == "incinerator" || name == "slag-incinerator" {
		for i, p := range w.incineratorTilePositions {
			if p == pos {
				w.incineratorTilePositions = append(w.incineratorTilePositions[:i], w.incineratorTilePositions[i+1:]...)
				break
			}
		}
	}

	if _, ok := repairTurretProfilesByBlockName[name]; ok {
		for i, p := range w.repairTurretTilePositions {
			if p == pos {
				w.repairTurretTilePositions = append(w.repairTurretTilePositions[:i], w.repairTurretTilePositions[i+1:]...)
				break
			}
		}
	}

	if _, ok := repairTowerProfilesByBlockName[name]; ok {
		for i, p := range w.repairTowerTilePositions {
			if p == pos {
				w.repairTowerTilePositions = append(w.repairTowerTilePositions[:i], w.repairTowerTilePositions[i+1:]...)
				break
			}
		}
	}

	if _, ok := unitFactoryPlansByBlockName[name]; ok {
		for i, p := range w.factoryTilePositions {
			if p == pos {
				w.factoryTilePositions = append(w.factoryTilePositions[:i], w.factoryTilePositions[i+1:]...)
				break
			}
		}
	}

	if isHeatConductorBlockName(name) {
		for i, p := range w.heatConductorTilePositions {
			if p == pos {
				w.heatConductorTilePositions = append(w.heatConductorTilePositions[:i], w.heatConductorTilePositions[i+1:]...)
				break
			}
		}
	}

	// Remove from turret positions
	if prof, ok := w.getBuildingWeaponProfile(int16(tile.Block)); ok && prof.Damage > 0 && prof.Interval > 0 && prof.Range > 0 {
		for i, p := range w.turretTilePositions {
			if p == pos {
				w.turretTilePositions = append(w.turretTilePositions[:i], w.turretTilePositions[i+1:]...)
				break
			}
		}
	}

	// Remove from support building positions
	if _, ok := mendProjectorProfiles[name]; ok {
		for i, p := range w.mendProjectorPositions {
			if p == pos {
				w.mendProjectorPositions = append(w.mendProjectorPositions[:i], w.mendProjectorPositions[i+1:]...)
				break
			}
		}
	}

	if _, ok := overdriveProjectorProfiles[name]; ok {
		for i, p := range w.overdriveProjectorPositions {
			if p == pos {
				w.overdriveProjectorPositions = append(w.overdriveProjectorPositions[:i], w.overdriveProjectorPositions[i+1:]...)
				break
			}
		}
	}

	if _, ok := forceProjectorProfiles[name]; ok {
		for i, p := range w.forceProjectorPositions {
			if p == pos {
				w.forceProjectorPositions = append(w.forceProjectorPositions[:i], w.forceProjectorPositions[i+1:]...)
				break
			}
		}
	}

	// Remove from team building tiles
	if tile.Build != nil {
		team := tile.Build.Team
		if team == 0 {
			team = tile.Team
		}
		if team != 0 {
			if tiles, ok := w.teamBuildingTiles[team]; ok {
				for i, p := range tiles {
					if p == pos {
						w.teamBuildingTiles[team] = append(tiles[:i], tiles[i+1:]...)
						break
					}
				}
			}

			// Remove from spatial index
			if idx, ok := w.teamBuildingSpatial[team]; ok {
				idx.remove(tile.X, tile.Y, pos)
			}

			// Remove from core tiles
			if isCoreBlockName(name) {
				if cores, ok := w.teamCoreTiles[team]; ok {
					for i, p := range cores {
						if p == pos {
							w.teamCoreTiles[team] = append(cores[:i], cores[i+1:]...)
							break
						}
					}
				}
			}
		}
	}
}

func (w *World) indexActiveTileLocked(pos int32, tile *Tile) {
	if !isCenterBuildingTile(tile) {
		return
	}
	w.activeTilePositions = append(w.activeTilePositions, pos)
	name := w.blockNameByID(int16(tile.Block))
	if isItemLogisticsBlockName(name) {
		w.itemLogisticsTilePositions = append(w.itemLogisticsTilePositions, pos)
	}
	if _, ok := crafterProfilesByBlockName[name]; ok {
		w.crafterTilePositions = append(w.crafterTilePositions, pos)
	} else if _, ok := separatorProfilesByBlockName[name]; ok {
		w.crafterTilePositions = append(w.crafterTilePositions, pos)
	}
	if _, ok := drillProfilesByBlockName[name]; ok {
		w.drillTilePositions = append(w.drillTilePositions, pos)
	}
	if _, ok := burstDrillProfilesByBlockName[name]; ok {
		w.burstDrillTilePositions = append(w.burstDrillTilePositions, pos)
	}
	if _, ok := beamDrillProfilesByBlockName[name]; ok {
		w.beamDrillTilePositions = append(w.beamDrillTilePositions, pos)
	}
	if _, ok := floorPumpProfilesByBlockName[name]; ok {
		w.pumpTilePositions = append(w.pumpTilePositions, pos)
	} else if _, ok := solidPumpProfilesByBlockName[name]; ok {
		w.pumpTilePositions = append(w.pumpTilePositions, pos)
	}
	if name == "incinerator" || name == "slag-incinerator" {
		w.incineratorTilePositions = append(w.incineratorTilePositions, pos)
	}
	if _, ok := repairTurretProfilesByBlockName[name]; ok {
		w.repairTurretTilePositions = append(w.repairTurretTilePositions, pos)
	}
	if _, ok := repairTowerProfilesByBlockName[name]; ok {
		w.repairTowerTilePositions = append(w.repairTowerTilePositions, pos)
	}
	if _, ok := unitFactoryPlansByBlockName[name]; ok {
		w.factoryTilePositions = append(w.factoryTilePositions, pos)
	}
	if isHeatConductorBlockName(name) {
		w.heatConductorTilePositions = append(w.heatConductorTilePositions, pos)
	}
	team := tile.Build.Team
	if team == 0 {
		team = tile.Team
	}
	if team != 0 && w.isPowerRelevantBuildingLocked(tile) {
		w.powerTilePositions = append(w.powerTilePositions, pos)
		w.teamPowerTiles[team] = append(w.teamPowerTiles[team], pos)
		if isPowerNodeBlockName(name) {
			w.teamPowerNodeTiles[team] = append(w.teamPowerNodeTiles[team], pos)
		}
		if name == "diode" {
			w.powerDiodeTilePositions = append(w.powerDiodeTilePositions, pos)
		}
		if name == "power-void" {
			w.powerVoidTilePositions = append(w.powerVoidTilePositions, pos)
		}
	}
	if team != 0 {
		w.teamBuildingTiles[team] = append(w.teamBuildingTiles[team], pos)
		idx := w.teamBuildingSpatial[team]
		if idx == nil {
			idx = &buildingSpatialIndex{cellSize: 64, cells: map[int64][]int32{}}
			w.teamBuildingSpatial[team] = idx
		}
		idx.insert(tile.X, tile.Y, pos)
		if isCoreBlockName(name) {
			w.teamCoreTiles[team] = append(w.teamCoreTiles[team], pos)
		}
	}
	if prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block)); ok && prof.Damage > 0 && prof.Interval > 0 && prof.Range > 0 {
		w.turretTilePositions = append(w.turretTilePositions, pos)
	}
	// Index support buildings
	if _, ok := mendProjectorProfiles[name]; ok {
		w.mendProjectorPositions = append(w.mendProjectorPositions, pos)
	}
	if _, ok := overdriveProjectorProfiles[name]; ok {
		w.overdriveProjectorPositions = append(w.overdriveProjectorPositions, pos)
	}
	if _, ok := forceProjectorProfiles[name]; ok {
		w.forceProjectorPositions = append(w.forceProjectorPositions, pos)
	}
}

func (w *World) setBuildingOccupancyLocked(pos int32, tile *Tile, occupy bool) {
	if tile == nil || !isCenterBuildingTile(tile) {
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
		return w.centerBuildingIndexLocked(pos)
	}
	pos := int32(y*w.model.Width + x)
	return w.centerBuildingIndexLocked(pos)
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
	w.rulesMgr.Set(DefaultRules())
	w.buildStates = map[int32]buildCombatState{}
	w.pendingBuilds = map[int32]pendingBuildState{}
	w.pendingBreaks = map[int32]pendingBreakState{}
	w.buildRejectLogTick = map[int32]uint64{}
	w.builderStates = map[int32]builderRuntimeState{}
	w.teamRebuildPlans = map[TeamID][]rebuildBlockPlan{}
	w.teamAIBuildPlans = map[TeamID][]teamBuildPlan{}
	w.teamBuildAIStates = map[TeamID]buildAIPlannerState{}
	w.buildAIParts = nil
	w.buildAIPartsLoaded = false
	w.factoryStates = map[int32]factoryState{}
	w.reconstructorStates = map[int32]reconstructorState{}
	w.drillStates = map[int32]drillRuntimeState{}
	w.burstDrillStates = map[int32]burstDrillRuntimeState{}
	w.beamDrillStates = map[int32]beamDrillRuntimeState{}
	w.pumpStates = map[int32]pumpRuntimeState{}
	w.crafterStates = map[int32]crafterRuntimeState{}
	w.heatStates = map[int32]float32{}
	w.incineratorStates = map[int32]float32{}
	w.repairTurretStates = map[int32]repairTurretRuntimeState{}
	w.repairTowerStates = map[int32]repairTowerRuntimeState{}
	w.teamPowerStates = map[TeamID]*teamPowerState{}
	w.teamPowerBudget = map[TeamID]float32{}
	w.powerNetStates = map[int32]*powerNetState{}
	w.powerNetByPos = map[int32]int32{}
	w.powerNetDirty = true
	w.powerStorageState = map[int32]float32{}
	w.powerGeneratorState = map[int32]*powerGeneratorState{}
	w.unitMountCDs = map[int32][]float32{}
	w.unitMountStates = map[int32][]unitMountState{}
	w.pendingMountShots = []pendingMountShot{}
	w.unitTargets = map[int32]targetTrackState{}
	w.unitAIStates = map[int32]unitAIState{}
	w.unitMiningStates = map[int32]unitMiningState{}
	w.teamItems = map[TeamID]map[ItemID]int32{}
	w.itemSourceCfg = map[int32]ItemID{}
	w.liquidSourceCfg = map[int32]LiquidID{}
	w.sorterCfg = map[int32]ItemID{}
	w.unloaderCfg = map[int32]ItemID{}
	w.payloadRouterCfg = map[int32]protocol.Content{}
	w.powerNodeLinks = map[int32][]int32{}
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
	w.payloadDeconstructorStates = map[int32]*payloadDeconstructorState{}
	w.payloadDriverStates = map[int32]*payloadDriverRuntimeState{}
	w.massDriverShots = []massDriverShot{}
	w.payloadDriverShots = []payloadDriverShot{}
	w.blockDumpIndex = map[int32]int{}
	w.dumpNeighborCache = map[int32][]int32{}
	w.itemSourceAccum = map[int32]float32{}
	w.routerInputPos = map[int32]int32{}
	w.routerRotation = map[int32]byte{}
	w.transportAccum = map[int32]float32{}
	w.junctionQueues = map[int32]junctionQueueState{}
	w.bridgeIncomingMask = map[int32]byte{}
	w.reactorStates = map[int32]nuclearReactorState{}
	w.storageLinkedCore = map[int32]int32{}
	w.teamPrimaryCore = map[TeamID]int32{}
	w.coreStorageCapacity = map[int32]int32{}
	w.blockOccupancy = map[int32]int32{}
	w.activeTilePositions = nil
	w.itemLogisticsTilePositions = nil
	w.crafterTilePositions = nil
	w.drillTilePositions = nil
	w.burstDrillTilePositions = nil
	w.beamDrillTilePositions = nil
	w.pumpTilePositions = nil
	w.incineratorTilePositions = nil
	w.repairTurretTilePositions = nil
	w.repairTowerTilePositions = nil
	w.factoryTilePositions = nil
	w.heatConductorTilePositions = nil
	w.powerTilePositions = nil
	w.powerDiodeTilePositions = nil
	w.powerVoidTilePositions = nil
	w.teamBuildingTiles = map[TeamID][]int32{}
	w.teamBuildingSpatial = map[TeamID]*buildingSpatialIndex{}
	w.teamCoreTiles = map[TeamID][]int32{}
	w.teamPowerTiles = map[TeamID][]int32{}
	w.teamPowerNodeTiles = map[TeamID][]int32{}
	w.turretTilePositions = nil
	w.nextPlanOrder = 0
	w.blockNamesByID = nil
	w.unitNamesByID = nil
	w.unitTypeDefsByID = nil
	w.statusProfilesByID = map[int16]statusEffectProfile{}
	w.statusProfilesByName = map[string]statusEffectProfile{}

	// 每次切图都从默认规则重新解析，再按原版 Gamemode 预设与地图 rules 叠加，
	// 避免旧地图规则残留，也避免漏掉 attack/sandbox/editor/pvp 的模式默认值。
	if m != nil {
		raw := strings.TrimSpace(tagValue(m.Tags, "rules"))
		if base, err := decodeRulesWithGamemodeDefaults([]byte(raw), m.Tags, m); err == nil && base != nil {
			w.rulesMgr.Set(base)
		} else {
			w.rulesMgr.Set(DefaultRules())
		}
		// 应用倍率到现有单位和建筑
		w.applyRulesToEntities()
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

func (w *World) UpdateBuilderState(owner int32, team TeamID, unitID int32, x, y float32, active bool, buildRange float32) {
	if w == nil || owner == 0 {
		return
	}
	if buildRange <= 0 {
		buildRange = vanillaBuilderRange
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.builderStates == nil {
		w.builderStates = map[int32]builderRuntimeState{}
	}
	w.builderStates[owner] = builderRuntimeState{
		Owner:      owner,
		Team:       team,
		UnitID:     unitID,
		X:          x,
		Y:          y,
		Active:     active,
		BuildRange: buildRange,
		UpdatedAt:  time.Now(),
	}
}

func (w *World) ClearBuilderState(owner int32) {
	if w == nil || owner == 0 {
		return
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	delete(w.builderStates, owner)
}

func (w *World) HasPendingPlansForOwner(owner int32) bool {
	if w == nil || owner == 0 {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	for _, st := range w.pendingBuilds {
		if st.Owner == owner {
			return true
		}
	}
	for _, st := range w.pendingBreaks {
		if st.Owner == owner {
			return true
		}
	}
	return false
}

func (w *World) builderCanActLocked(owner int32, team TeamID, tile *Tile) bool {
	if owner == 0 || tile == nil {
		return true
	}
	state, ok := w.builderStates[owner]
	if !ok {
		return false
	}
	if !state.Active {
		return false
	}
	if state.Team != 0 && team != 0 && state.Team != team {
		return false
	}
	rules := w.rulesMgr.Get()
	if rules != nil && (rules.Editor || rules.InfiniteResources || rules.teamInfiniteResources(team)) {
		return true
	}
	rangeLimit := state.BuildRange
	if rangeLimit <= 0 {
		rangeLimit = vanillaBuilderRange
	}
	tx := float32(tile.X*8 + 4)
	ty := float32(tile.Y*8 + 4)
	dx := tx - state.X
	dy := ty - state.Y
	return dx*dx+dy*dy <= rangeLimit*rangeLimit
}

func (w *World) pendingConstructContributorSpeedLocked(pos int32, tile *Tile, owner int32, team TeamID, breaking bool, blockID int16, rotation int8) float32 {
	if w == nil || w.model == nil || tile == nil || team == 0 {
		return 0
	}
	total := float32(0)
	if owner == 0 || w.builderCanActLocked(owner, team, tile) {
		total += w.builderSpeedForOwnerLocked(owner, team)
	}
	for candidateOwner, state := range w.builderStates {
		if candidateOwner == owner || state.Team != 0 && state.Team != team {
			continue
		}
		if !w.builderCanActLocked(candidateOwner, team, tile) {
			continue
		}
		entity, ok := w.entityByIDLocked(state.UnitID)
		if !ok || entity.Team != team || entity.Health <= 0 || entity.BuildSpeed <= 0 || !entity.UpdateBuilding || len(entity.Plans) == 0 {
			continue
		}
		plan, ok := primaryAssistBuildPlan(entity)
		if !ok || plan.Breaking != breaking || int32(tile.X) != plan.X || int32(tile.Y) != plan.Y {
			continue
		}
		if !breaking {
			if plan.BlockID != blockID || int8(plan.Rotation) != rotation {
				continue
			}
		}
		total += w.builderSpeedForOwnerLocked(candidateOwner, team)
	}
	return total
}

func (w *World) entityByIDLocked(id int32) (RawEntity, bool) {
	if w == nil || w.model == nil || id == 0 {
		return RawEntity{}, false
	}
	for _, entity := range w.model.Entities {
		if entity.ID != id {
			continue
		}
		return entity, true
	}
	return RawEntity{}, false
}

func (w *World) appendBuildCancelledLocked(pos int32, st pendingBuildState) {
	if !st.VisualPlaced {
		return
	}
	x := int(pos % int32(w.model.Width))
	y := int(pos / int32(w.model.Width))
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventBuildCancelled,
		BuildPos:   packTilePos(x, y),
		BuildOwner: st.Owner,
		BuildTeam:  st.Team,
		BuildBlock: st.BlockID,
	})
}

func (w *World) cancelPendingBuildLocked(pos int32, st pendingBuildState) {
	delete(w.pendingBuilds, pos)
	w.refundPendingBuildConsumedLocked(st)
	w.appendBuildCancelledLocked(pos, st)
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
		builderSpeed := w.pendingConstructContributorSpeedLocked(pos, tile, st.Owner, st.Team, false, st.BlockID, st.Rotation)
		if builderSpeed <= 0 {
			continue
		}
		w.ensurePendingBuildCostStateLocked(&st)
		buildDuration := w.buildDurationSecondsForBuilderSpeedLocked(st.BlockID, st.Team, rules, builderSpeed)
		progressBefore := clampf(st.Progress, 0, 1)
		progressStep := dt / buildDuration
		if progressStep > 0 {
			progressStep = w.applyVanillaBuildCostStepLocked(st.Team, &st, progressStep)
		}
		if !st.VisualPlaced {
			shouldVisualPlace := progressStep > 0
			if !shouldVisualPlace && st.Owner != 0 && w.builderCanActLocked(st.Owner, st.Team, tile) {
				shouldVisualPlace = true
			}
			if !shouldVisualPlace {
				w.pendingBuilds[pos] = st
				continue
			}
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:        EntityEventBuildPlaced,
				BuildPos:    packTilePos(tile.X, tile.Y),
				BuildOwner:  st.Owner,
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
		st.Progress = clampf(st.Progress+progressStep, 0, 1)
		hpNow := constructBlockHealthMax * clampf(st.Progress, 0, 1)
		if hpNow < 1 {
			hpNow = 1
		}
		if hpNow-st.LastHP >= 1 || st.Progress >= 1 {
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
		if !w.finishPendingBuildCostLocked(st.Team, &st) {
			if progressBefore != st.Progress {
				w.pendingBuilds[pos] = st
				continue
			}
			w.pendingBuilds[pos] = st
			continue
		}
		placed := w.placeCompletedBuildingLocked(pos, tile, st.Team, st.BlockID, st.Rotation, st.Config)
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:     EntityEventBuildHealth,
			BuildPos: packTilePos(tile.X, tile.Y),
			BuildHP:  tile.Build.Health,
		}, EntityEvent{
			Kind:        EntityEventBuildConstructed,
			BuildPos:    packTilePos(tile.X, tile.Y),
			BuildOwner:  st.Owner,
			BuildTeam:   st.Team,
			BuildBlock:  st.BlockID,
			BuildRot:    st.Rotation,
			BuildConfig: placed.Config,
		})
		for _, target := range placed.SelfConfigTargets {
			if target < 0 || int(target) >= len(w.model.Tiles) {
				continue
			}
			targetTile := &w.model.Tiles[target]
			w.appendBuildConfigValueEventLocked(pos, packTilePos(targetTile.X, targetTile.Y))
		}
		for _, changed := range placed.ChangedConfigs {
			if changed.targetPos < 0 || int(changed.targetPos) >= len(w.model.Tiles) {
				continue
			}
			targetTile := &w.model.Tiles[changed.targetPos]
			w.appendBuildConfigValueEventLocked(changed.nodePos, packTilePos(targetTile.X, targetTile.Y))
		}
		delete(w.pendingBuilds, pos)
	}
	// REMOVED: rebuildActiveTilesLocked() - use incremental indexing instead
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
		builderSpeed := w.pendingConstructContributorSpeedLocked(pos, tile, st.Owner, st.Team, true, st.BlockID, st.Rotation)
		if builderSpeed <= 0 {
			continue
		}
		breakDuration := w.buildDurationSecondsForBuilderSpeedLocked(st.BlockID, st.Team, rules, builderSpeed)
		if breakDuration < float32(1.0/60.0) {
			breakDuration = float32(1.0 / 60.0)
		}
		if !st.VisualStart {
			w.entityEvents = append(w.entityEvents, EntityEvent{
				Kind:       EntityEventBuildDeconstructing,
				BuildPos:   packTilePos(tile.X, tile.Y),
				BuildOwner: st.Owner,
				BuildTeam:  st.Team,
				BuildBlock: st.BlockID,
				BuildRot:   st.Rotation,
			})
			st.VisualStart = true
		}
		amount := dt / breakDuration
		progressBefore := clampf(st.Progress, 0, 1)
		clampedAmount := amount
		if remaining := 1 - progressBefore; clampedAmount > remaining {
			clampedAmount = remaining
		}
		if clampedAmount > 0 {
			st.RefundAccum, st.RefundTotal, st.Refunded = w.applyVanillaDeconstructRefundStepLocked(
				st.RefundTeam, st.RefundCost, clampedAmount, st.RefundAccum, st.RefundTotal, st.Refunded,
			)
		}
		st.Progress += amount
		progress := clampf(st.Progress, 0, 1)
		hpNow := st.MaxHealth * (1 - progress)
		if hpNow < 0 {
			hpNow = 0
		}
		if tile.Build != nil {
			tile.Build.Health = hpNow
		}
		if st.LastHP-hpNow >= 1 || hpNow <= 0 || st.Progress >= 1 {
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
		st.Refunded = w.finishVanillaDeconstructRefundLocked(st.RefundTeam, st.RefundCost, st.Refunded)
		teamOld := tile.Team
		if tile.Build != nil && tile.Build.Team != 0 {
			teamOld = tile.Build.Team
		}
		if teamOld == 0 {
			teamOld = st.Team
		}
		// CRITICAL: Remove from indices BEFORE clearing tile data
		// Otherwise removeActiveTileIndexLocked cannot identify the building type
		w.removeActiveTileIndexLocked(pos, tile)
		w.setBuildingOccupancyLocked(pos, tile, false)
		tile.Build = nil
		tile.Block = 0
		tile.Team = 0
		tile.Rotation = 0
		delete(w.buildStates, pos)
		w.entityEvents = append(w.entityEvents, EntityEvent{
			Kind:       EntityEventBuildDestroyed,
			BuildPos:   packTilePos(tile.X, tile.Y),
			BuildOwner: st.Owner,
			BuildTeam:  teamOld,
			BuildBlock: st.BlockID,
		})
		delete(w.pendingBreaks, pos)
	}
	// REMOVED: rebuildActiveTilesLocked() - use incremental indexing instead
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
	switch want {
	case "alpha":
		return 35, true
	case "beta":
		return 36, true
	case "gamma":
		return 37, true
	case "evoke":
		return 53, true
	case "incite":
		return 54, true
	case "emanate":
		return 55, true
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
		metaByName := make(map[string]unitRuntimeProfile, len(w.unitRuntimeProfilesByName))
		for k, v := range w.unitRuntimeProfilesByName {
			metaByName[k] = cloneUnitRuntimeProfile(v)
		}
		mountsByName := cloneUnitMountProfilesByName(w.unitMountProfilesByName)
		for _, u := range payload.Units {
			name := strings.ToLower(strings.TrimSpace(u.Name))
			if name != "" {
				pn := defaultWeaponProfile
				if cur, ok := byName[name]; ok {
					pn = cur
				}
				mergeUnitProfile(&pn, u)
				byName[name] = pn
				if len(u.Mounts) > 0 {
					parsed := make([]unitWeaponMountProfile, 0, len(u.Mounts))
					for _, m := range u.Mounts {
						parsed = append(parsed, convertVanillaMountProfile(m))
					}
					mountsByName[name] = parsed
				}
				metaByName[name] = convertVanillaUnitRuntimeProfile(u)
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
		w.unitRuntimeProfilesByName = metaByName
		w.unitMountProfilesByName = mountsByName
	}
	if len(payload.UnitsByName) > 0 {
		base := cloneUnitWeaponProfilesByName(w.unitProfilesByName)
		metaByName := make(map[string]unitRuntimeProfile, len(w.unitRuntimeProfilesByName))
		for k, v := range w.unitRuntimeProfilesByName {
			metaByName[k] = cloneUnitRuntimeProfile(v)
		}
		mountsByName := cloneUnitMountProfilesByName(w.unitMountProfilesByName)
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
			if len(u.Mounts) > 0 {
				parsed := make([]unitWeaponMountProfile, 0, len(u.Mounts))
				for _, m := range u.Mounts {
					parsed = append(parsed, convertVanillaMountProfile(m))
				}
				mountsByName[name] = parsed
			}
			metaByName[name] = convertVanillaUnitRuntimeProfile(u)
		}
		w.unitProfilesByName = base
		w.unitRuntimeProfilesByName = metaByName
		w.unitMountProfilesByName = mountsByName
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
		armor := make(map[string]float32, len(payload.Blocks))
		for _, b := range payload.Blocks {
			name := strings.ToLower(strings.TrimSpace(b.Name))
			if name == "" {
				continue
			}
			if b.Armor > 0 {
				armor[name] = b.Armor
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
		w.blockArmorByName = armor
	}
	if len(payload.Statuses) > 0 {
		byID := make(map[int16]statusEffectProfile, len(payload.Statuses))
		byName := make(map[string]statusEffectProfile, len(payload.Statuses))
		for _, s := range payload.Statuses {
			prof := statusEffectProfile{
				ID:                   s.ID,
				Name:                 strings.ToLower(strings.TrimSpace(s.Name)),
				DamageMultiplier:     s.DamageMultiplier,
				HealthMultiplier:     s.HealthMultiplier,
				SpeedMultiplier:      s.SpeedMultiplier,
				ReloadMultiplier:     s.ReloadMultiplier,
				BuildSpeedMultiplier: s.BuildSpeedMultiplier,
				DragMultiplier:       s.DragMultiplier,
				TransitionDamage:     s.TransitionDamage,
				Damage:               s.Damage,
				IntervalDamageTime:   s.IntervalDamageTime,
				IntervalDamage:       s.IntervalDamage,
				IntervalDamagePierce: s.IntervalDamagePierce,
				Disarm:               s.Disarm,
				Permanent:            s.Permanent,
				Reactive:             s.Reactive,
				Dynamic:              s.Dynamic,
				Opposites:            append([]string(nil), s.Opposites...),
				Affinities:           append([]string(nil), s.Affinities...),
			}
			byID[prof.ID] = prof
			if prof.Name != "" {
				byName[prof.Name] = prof
			}
		}
		w.statusProfilesByID = byID
		w.statusProfilesByName = byName
	}
	return nil
}

func cloneUnitWeaponProfiles(src map[int16]weaponProfile) map[int16]weaponProfile {
	out := make(map[int16]weaponProfile, len(src))
	for k, v := range src {
		v.FragmentBullet = cloneBulletRuntimeProfile(v.FragmentBullet)
		out[k] = v
	}
	return out
}

func cloneBuildingWeaponProfiles(src map[string]buildingWeaponProfile) map[string]buildingWeaponProfile {
	out := make(map[string]buildingWeaponProfile, len(src))
	for k, v := range src {
		v.FragmentBullet = cloneBulletRuntimeProfile(v.FragmentBullet)
		v.Bullet = cloneBulletRuntimeProfile(v.Bullet)
		out[k] = v
	}
	return out
}

func cloneUnitWeaponProfilesByName(src map[string]weaponProfile) map[string]weaponProfile {
	out := make(map[string]weaponProfile, len(src))
	for k, v := range src {
		v.FragmentBullet = cloneBulletRuntimeProfile(v.FragmentBullet)
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
	if u.SplashDamage > 0 {
		p.SplashDamage = u.SplashDamage
	}
	if u.Interval > 0 {
		p.Interval = u.Interval
	}
	if u.BulletType > 0 {
		p.BulletType = u.BulletType
	}
	p.BulletClass = ""
	if u.Bullet != nil {
		p.BulletClass = strings.TrimSpace(u.Bullet.ClassName)
	}
	if u.BulletSpeed > 0 {
		p.BulletSpeed = u.BulletSpeed
	}
	if u.BulletLifetime > 0 {
		p.BulletLifetime = u.BulletLifetime
	}
	if u.BulletHitSize > 0 {
		p.BulletHitSize = u.BulletHitSize
	}
	p.SplashRadius = u.SplashRadius
	p.BuildingDamage = u.BuildingDamageMultiplier
	p.ArmorMultiplier = u.ArmorMultiplier
	p.MaxDamageFraction = u.MaxDamageFraction
	p.ShieldDamageMul = u.ShieldDamageMultiplier
	p.PierceDamageFactor = u.PierceDamageFactor
	p.PierceArmor = u.PierceArmor
	p.SlowSec = u.SlowSec
	if u.SlowMul > 0 {
		p.SlowMul = u.SlowMul
	}
	p.Pierce = u.Pierce
	p.PierceBuilding = u.PierceBuilding
	p.ChainCount = u.ChainCount
	p.ChainRange = u.ChainRange
	p.FragmentCount = u.FragmentCount
	p.FragmentSpread = u.FragmentSpread
	p.FragmentSpeed = u.FragmentSpeed
	p.FragmentLife = u.FragmentLife
	p.FragmentRandomSpread = u.FragmentRandomSpread
	p.FragmentAngle = u.FragmentAngle
	if u.FragmentVelocityMin > 0 {
		p.FragmentVelocityMin = u.FragmentVelocityMin
	}
	if u.FragmentVelocityMax > 0 {
		p.FragmentVelocityMax = u.FragmentVelocityMax
	}
	if u.FragmentLifeMin > 0 {
		p.FragmentLifeMin = u.FragmentLifeMin
	}
	if u.FragmentLifeMax > 0 {
		p.FragmentLifeMax = u.FragmentLifeMax
	}
	p.FragmentBullet = convertVanillaBulletProfile(u.FragmentBullet)
	p.StatusID = u.StatusID
	p.StatusName = strings.ToLower(strings.TrimSpace(u.StatusName))
	p.StatusDuration = u.StatusDuration
	p.ShootStatusID = u.ShootStatusID
	p.ShootStatusName = strings.ToLower(strings.TrimSpace(u.ShootStatusName))
	p.ShootStatusDuration = u.ShootStatusDuration
	if strings.TrimSpace(u.ShootEffect) != "" {
		p.ShootEffect = strings.TrimSpace(u.ShootEffect)
	}
	if strings.TrimSpace(u.SmokeEffect) != "" {
		p.SmokeEffect = strings.TrimSpace(u.SmokeEffect)
	}
	if strings.TrimSpace(u.HitEffect) != "" {
		p.HitEffect = strings.TrimSpace(u.HitEffect)
	}
	if strings.TrimSpace(u.DespawnEffect) != "" {
		p.DespawnEffect = strings.TrimSpace(u.DespawnEffect)
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
	if strings.TrimSpace(t.ClassName) != "" {
		p.ClassName = strings.TrimSpace(t.ClassName)
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
	if t.SplashDamage > 0 {
		p.SplashDamage = t.SplashDamage
	}
	if t.Interval > 0 || t.ContinuousHold {
		p.Interval = t.Interval
	}
	if t.BulletType > 0 {
		p.BulletType = t.BulletType
	}
	p.BulletClass = ""
	if t.Bullet != nil {
		p.BulletClass = strings.TrimSpace(t.Bullet.ClassName)
	}
	if t.BulletSpeed > 0 {
		p.BulletSpeed = t.BulletSpeed
	}
	if t.BulletLifetime > 0 {
		p.BulletLifetime = t.BulletLifetime
	}
	if t.BulletHitSize > 0 {
		p.BulletHitSize = t.BulletHitSize
	}
	p.SplashRadius = t.SplashRadius
	p.BuildingDamage = t.BuildingDamageMultiplier
	p.ArmorMultiplier = t.ArmorMultiplier
	p.MaxDamageFraction = t.MaxDamageFraction
	p.ShieldDamageMul = t.ShieldDamageMultiplier
	p.PierceDamageFactor = t.PierceDamageFactor
	p.PierceArmor = t.PierceArmor
	p.SlowSec = t.SlowSec
	if t.SlowMul > 0 {
		p.SlowMul = t.SlowMul
	}
	p.Pierce = t.Pierce
	p.PierceBuilding = t.PierceBuilding
	p.ChainCount = t.ChainCount
	p.ChainRange = t.ChainRange
	p.StatusID = t.StatusID
	p.StatusName = strings.ToLower(strings.TrimSpace(t.StatusName))
	p.StatusDuration = t.StatusDuration
	p.FragmentCount = t.FragmentCount
	p.FragmentSpread = t.FragmentSpread
	p.FragmentSpeed = t.FragmentSpeed
	p.FragmentLife = t.FragmentLife
	p.FragmentRandomSpread = t.FragmentRandomSpread
	p.FragmentAngle = t.FragmentAngle
	if t.FragmentVelocityMin > 0 {
		p.FragmentVelocityMin = t.FragmentVelocityMin
	}
	if t.FragmentVelocityMax > 0 {
		p.FragmentVelocityMax = t.FragmentVelocityMax
	}
	if t.FragmentLifeMin > 0 {
		p.FragmentLifeMin = t.FragmentLifeMin
	}
	if t.FragmentLifeMax > 0 {
		p.FragmentLifeMax = t.FragmentLifeMax
	}
	p.FragmentBullet = convertVanillaBulletProfile(t.FragmentBullet)
	p.Bullet = convertVanillaBulletProfile(t.Bullet)
	if strings.TrimSpace(t.ShootEffect) != "" {
		p.ShootEffect = strings.TrimSpace(t.ShootEffect)
	}
	if strings.TrimSpace(t.SmokeEffect) != "" {
		p.SmokeEffect = strings.TrimSpace(t.SmokeEffect)
	}
	if strings.TrimSpace(t.HitEffect) != "" {
		p.HitEffect = strings.TrimSpace(t.HitEffect)
	}
	if strings.TrimSpace(t.DespawnEffect) != "" {
		p.DespawnEffect = strings.TrimSpace(t.DespawnEffect)
	}
	p.HitBuildings = t.HitBuildings
	p.TargetBuilds = t.TargetBuilds
	p.TargetAir = t.TargetAir
	p.TargetGround = t.TargetGround
	p.Rotate = t.Rotate
	if t.RotateSpeed > 0 {
		p.RotateSpeed = t.RotateSpeed
	}
	p.BaseRotation = t.BaseRotation
	p.PredictTarget = t.PredictTarget
	if t.TargetInterval > 0 {
		p.TargetInterval = t.TargetInterval
	}
	if t.TargetSwitchInterval > 0 {
		p.TargetSwitchInterval = t.TargetSwitchInterval
	}
	if t.ShootCone > 0 {
		p.ShootCone = t.ShootCone
	}
	if t.RotationLimit > 0 {
		p.RotationLimit = t.RotationLimit
	}
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
	p.ContinuousHold = t.ContinuousHold
	p.AimChangeSpeed = t.AimChangeSpeed
	p.ShootDuration = t.ShootDuration
}

func (w *World) Model() *WorldModel {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.model
}

func (w *World) CloneModel() *WorldModel {
	w.mu.RLock()
	defer w.mu.RUnlock()
	if w.model == nil {
		return nil
	}
	return w.model.Clone()
}

func (w *World) AddEntity(typeID int16, x, y float32, team TeamID) (RawEntity, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, ErrOutOfBounds
	}
	ent := RawEntity{
		TypeID:              typeID,
		X:                   x,
		Y:                   y,
		Team:                team,
		Health:              100,
		MaxHealth:           100,
		Shield:              0,
		ShieldMax:           0,
		ShieldRegen:         0,
		Armor:               0,
		SlowMul:             1,
		StatusDamageMul:     1,
		StatusHealthMul:     1,
		StatusSpeedMul:      1,
		StatusReloadMul:     1,
		StatusBuildSpeedMul: 1,
		StatusDragMul:       1,
		StatusArmorOverride: -1,
		RuntimeInit:         true,
		MineTilePos:         invalidEntityTilePos,
	}
	w.applyUnitTypeDef(&ent)
	w.applyWeaponProfile(&ent)
	if isEntityFlying(ent) {
		ent.Elevation = 1
	}
	return w.model.AddEntity(ent), nil
}

func (w *World) ReserveEntityID() int32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return 0
	}
	if w.model.NextEntityID <= 0 {
		w.model.NextEntityID = 1
	}
	id := w.model.NextEntityID
	w.model.NextEntityID++
	return id
}

func (w *World) AddEntityWithID(typeID int16, id int32, x, y float32, team TeamID) (RawEntity, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.model == nil {
		return RawEntity{}, ErrOutOfBounds
	}
	for i := range w.model.Entities {
		if w.model.Entities[i].ID == id {
			return RawEntity{}, ErrEntityExists
		}
	}
	ent := RawEntity{
		TypeID:              typeID,
		ID:                  id,
		X:                   x,
		Y:                   y,
		Health:              100,
		MaxHealth:           100,
		Shield:              0,
		ShieldMax:           0,
		ShieldRegen:         0,
		Armor:               0,
		SlowMul:             1,
		StatusDamageMul:     1,
		StatusHealthMul:     1,
		StatusSpeedMul:      1,
		StatusReloadMul:     1,
		StatusBuildSpeedMul: 1,
		StatusDragMul:       1,
		StatusArmorOverride: -1,
		RuntimeInit:         true,
		MineTilePos:         invalidEntityTilePos,
		Team:                team,
	}
	w.applyUnitTypeDef(&ent)
	w.applyWeaponProfile(&ent)
	if isEntityFlying(ent) {
		ent.Elevation = 1
	}
	return w.model.AddEntity(ent), nil
}

func (w *World) RemoveEntity(id int32) (RawEntity, bool) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.removeEntityLocked(id)
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
	if items := w.teamCoreItemsLocked(team); items != nil {
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
	teams := make(map[TeamID]map[ItemID]int32, len(w.teamCoreTiles))
	for team, positions := range w.teamCoreTiles {
		if team == 0 || len(positions) == 0 {
			continue
		}
		items := make(map[ItemID]int32)
		for _, pos := range positions {
			if pos < 0 || int(pos) >= len(w.model.Tiles) {
				continue
			}
			tile := &w.model.Tiles[pos]
			if tile.Build == nil || tile.Block <= 0 {
				continue
			}
			for _, stack := range tile.Build.Items {
				if stack.Amount <= 0 {
					continue
				}
				items[stack.Item] += stack.Amount
			}
		}
		teams[team] = items
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
	for _, pos := range w.teamCoreTiles[team] {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		if w.blockSyncSuppressedLocked(pos) {
			continue
		}
		tile := &w.model.Tiles[pos]
		if tile.Team != team || tile.Build == nil || tile.Block == 0 {
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
		if w.blockSyncSuppressedLocked(pos) || w.blockSyncSuppressedLocked(corePos) {
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

func constructBreakStartHealth(tile *Tile) float32 {
	if tile == nil || tile.Build == nil {
		return constructBlockHealthMax
	}
	maxHealth := tile.Build.MaxHealth
	if maxHealth <= 0 {
		maxHealth = tile.Build.Health
	}
	if maxHealth <= 0 {
		return constructBlockHealthMax
	}
	start := constructBlockHealthMax * clampf(tile.Build.Health/maxHealth, 0, 1)
	if start < 0 {
		return 0
	}
	if start > constructBlockHealthMax {
		return constructBlockHealthMax
	}
	return start
}

func (w *World) builderSpeedForUnitTypeLocked(typeID int16) float32 {
	if prof, ok := w.unitRuntimeProfileForTypeLocked(typeID); ok && prof.BuildSpeed > 0 {
		return prof.BuildSpeed
	}
	name := ""
	if w.unitNamesByID != nil {
		name = w.unitNamesByID[typeID]
	}
	if strings.TrimSpace(name) == "" {
		name = fallbackUnitNameByTypeID(typeID)
	}
	return unitBuildSpeedByName(name)
}

func fallbackUnitNameByTypeID(typeID int16) string {
	switch typeID {
	case 35:
		return "alpha"
	case 36:
		return "beta"
	case 37:
		return "gamma"
	case 53:
		return "evoke"
	case 54:
		return "incite"
	case 55:
		return "emanate"
	default:
		return ""
	}
}

func fallbackCoreUnitTypeDef(name string) (vanilla.UnitTypeDef, bool) {
	switch normalizeUnitName(name) {
	case "alpha":
		return vanilla.UnitTypeDef{Name: "alpha", Health: 150, Speed: 3, HitSize: 8, RotateSpeed: 15}, true
	case "beta":
		return vanilla.UnitTypeDef{Name: "beta", Health: 170, Speed: 3.3, HitSize: 9, RotateSpeed: 17}, true
	case "gamma":
		return vanilla.UnitTypeDef{Name: "gamma", Health: 220, Speed: 3.55, HitSize: 11, RotateSpeed: 19}, true
	case "evoke":
		return vanilla.UnitTypeDef{Name: "evoke", Health: 300, Armor: 1, Speed: 5.6, HitSize: 9, RotateSpeed: 15}, true
	case "incite":
		return vanilla.UnitTypeDef{Name: "incite", Health: 500, Armor: 2, Speed: 7, HitSize: 11, RotateSpeed: 17}, true
	case "emanate":
		return vanilla.UnitTypeDef{Name: "emanate", Health: 700, Armor: 3, Speed: 7.5, HitSize: 12, RotateSpeed: 19}, true
	default:
		return vanilla.UnitTypeDef{}, false
	}
}

func (w *World) BuilderSpeedForUnitType(typeID int16) float32 {
	w.mu.RLock()
	defer w.mu.RUnlock()
	name := ""
	if w.unitNamesByID != nil {
		name = w.unitNamesByID[typeID]
	}
	if strings.TrimSpace(name) == "" {
		name = fallbackUnitNameByTypeID(typeID)
	}
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
	positions := w.teamCoreTiles[team]
	out := make([]int32, 0, len(positions))
	for _, pos := range positions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		t := &w.model.Tiles[pos]
		if t.Block > 0 {
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
		if w.blockSyncSuppressedLocked(pos) {
			continue
		}
		t := w.model.Tiles[pos]
		if !isCenterBuildingTile(&t) {
			continue
		}
		hp := float32(1000)
		if t.Build != nil && t.Build.Health > 0 {
			hp = t.Build.Health
		}
		team := t.Team
		if t.Build != nil && t.Build.Team != 0 {
			team = t.Build.Team
		}
		out = append(out, BuildSyncState{
			Pos:      packTilePos(t.X, t.Y),
			X:        int32(t.X),
			Y:        int32(t.Y),
			BlockID:  int16(t.Block),
			Team:     team,
			Rotation: t.Rotation,
			Health:   hp,
		})
	}
	return out
}

func isCenterBuildingTile(tile *Tile) bool {
	if tile == nil || tile.Build == nil || tile.Block == 0 {
		return false
	}
	return tile.Build.X == tile.X && tile.Build.Y == tile.Y
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

// ApplyPlacementPlanSnapshot reconciles only placement plans from client preview snapshots.
// Official client snapshots do not authoritatively carry break queues, so pending breaks
// must remain driven by beginBreak/removeQueue packets instead of being cleared here.
func (w *World) ApplyPlacementPlanSnapshot(team TeamID, ops []BuildPlanOp) []int32 {
	return w.ApplyPlacementPlanSnapshotForOwner(0, team, ops)
}

func (w *World) ApplyBuildPlanSnapshotForOwner(owner int32, team TeamID, ops []BuildPlanOp) []int32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.applyBuildPlanSnapshotForOwnerLocked(owner, team, ops, true)
}

func (w *World) ApplyPlacementPlanSnapshotForOwner(owner int32, team TeamID, ops []BuildPlanOp) []int32 {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.applyBuildPlanSnapshotForOwnerLocked(owner, team, ops, false)
}

func (w *World) applyBuildPlanSnapshotForOwnerLocked(owner int32, team TeamID, ops []BuildPlanOp, reconcileBreaks bool) []int32 {
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
		w.cancelPendingBuildLocked(pos, st)
		addChanged(pos)
	}
	if reconcileBreaks {
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
			w.cancelPendingBuildLocked(pos, st)
			addChanged(pos)
		}
		delete(w.factoryStates, pos)
		if st, ok := w.pendingBreaks[pos]; ok {
			if st.BlockID == int16(tile.Block) && st.Team == team {
				if owner != 0 && st.Owner != 0 && st.Owner != owner {
					return
				}
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
			w.destroyTileLocked(tile, team, owner)
			delete(w.pendingBreaks, pos)
			addChanged(pos)
			return
		}
		maxHP := constructBreakStartHealth(tile)
		refundTeam, refundStacks := w.deconstructRefundStacks(tile, team)
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
			RefundTeam:  refundTeam,
			RefundCost:  append([]ItemStack(nil), refundStacks...),
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
		w.cancelPendingBuildLocked(pos, pending)
	}
	if rules := w.rulesMgr.Get(); rules != nil &&
		rules.DerelictRepair &&
		team != 0 &&
		tile.Team == 0 &&
		tile.Block == BlockID(op.BlockID) {
		placed := w.placeCompletedBuildingLocked(pos, tile, team, op.BlockID, op.Rotation, op.Config)
		w.entityEvents = append(w.entityEvents,
			EntityEvent{
				Kind:     EntityEventBuildHealth,
				BuildPos: packTilePos(tile.X, tile.Y),
				BuildHP:  tile.Build.Health,
			},
			EntityEvent{
				Kind:        EntityEventBuildConstructed,
				BuildPos:    packTilePos(tile.X, tile.Y),
				BuildOwner:  owner,
				BuildTeam:   team,
				BuildBlock:  op.BlockID,
				BuildRot:    op.Rotation,
				BuildConfig: placed.Config,
			},
		)
		for _, target := range placed.SelfConfigTargets {
			if target < 0 || int(target) >= len(w.model.Tiles) {
				continue
			}
			targetTile := &w.model.Tiles[target]
			w.appendBuildConfigValueEventLocked(pos, packTilePos(targetTile.X, targetTile.Y))
		}
		for _, changed := range placed.ChangedConfigs {
			if changed.targetPos < 0 || int(changed.targetPos) >= len(w.model.Tiles) {
				continue
			}
			targetTile := &w.model.Tiles[changed.targetPos]
			w.appendBuildConfigValueEventLocked(changed.nodePos, packTilePos(targetTile.X, targetTile.Y))
		}
		w.clearOverlappingRebuildPlansLocked(team, tile.X, tile.Y, op.BlockID)
		w.clearOverlappingTeamBuildPlansLocked(team, tile.X, tile.Y, op.BlockID)
		delete(w.pendingBreaks, pos)
		delete(w.pendingBuilds, pos)
		addChanged(pos)
		return
	}
	if tile.Block == BlockID(op.BlockID) && tile.Team == team && tile.Rotation == op.Rotation && tile.Build != nil {
		w.clearOverlappingRebuildPlansLocked(team, tile.X, tile.Y, op.BlockID)
		w.clearOverlappingTeamBuildPlansLocked(team, tile.X, tile.Y, op.BlockID)
		w.applyBuildingConfigLocked(pos, op.Config, true)
		delete(w.pendingBreaks, pos)
		delete(w.pendingBuilds, pos)
		return
	}
	if rules := w.rulesMgr.Get(); rules != nil && (rules.InstantBuild || rules.Editor) {
		w.placeTileLocked(tile, team, op.BlockID, int8(op.Rotation), op.Config, owner)
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

func cloneLiquidStacks(src []LiquidStack) []LiquidStack {
	if len(src) == 0 {
		return nil
	}
	out := make([]LiquidStack, len(src))
	copy(out, src)
	return out
}

func cloneBytes(src []byte) []byte {
	if len(src) == 0 {
		return nil
	}
	out := make([]byte, len(src))
	copy(out, src)
	return out
}

func (w *World) placeCompletedBuildingLocked(pos int32, tile *Tile, team TeamID, blockID int16, rotation int8, config any) completedBuildingPlacement {
	result := completedBuildingPlacement{Config: config}
	if w == nil || w.model == nil || tile == nil || pos < 0 || int(pos) >= len(w.model.Tiles) {
		return result
	}
	prevBlockName := w.blockNameByID(int16(tile.Block))

	prevItems := []ItemStack(nil)
	prevLiquids := []LiquidStack(nil)
	prevPayload := []byte(nil)
	prevHealth := float32(1000)
	prevMaxHealth := float32(1000)
	if tile.Build != nil && tile.Block == BlockID(blockID) {
		prevItems = cloneItemStacks(tile.Build.Items)
		prevLiquids = cloneLiquidStacks(tile.Build.Liquids)
		prevPayload = cloneBytes(tile.Build.Payload)
		if normalized, ok := w.normalizedBuildingConfigLocked(pos); ok && result.Config == nil {
			result.Config = normalized
		}
		if tile.Build.MaxHealth > 0 {
			prevMaxHealth = tile.Build.MaxHealth
		}
		if tile.Build.Health > 0 {
			prevHealth = tile.Build.Health
		}
		if prevHealth > prevMaxHealth {
			prevHealth = prevMaxHealth
		}
	}

	w.clearBuildingRuntimeLocked(pos)

	tile.Block = BlockID(blockID)
	tile.Team = team
	tile.Rotation = rotation
	tile.Build = &Building{
		Block:     tile.Block,
		Team:      team,
		Rotation:  rotation,
		X:         tile.X,
		Y:         tile.Y,
		Items:     prevItems,
		Liquids:   prevLiquids,
		Payload:   prevPayload,
		Health:    prevHealth,
		MaxHealth: prevMaxHealth,
	}
	if tile.Build.Health <= 0 {
		tile.Build.Health = tile.Build.MaxHealth
	}
	if tile.Build.MaxHealth <= 0 {
		tile.Build.MaxHealth = 1000
		if tile.Build.Health <= 0 {
			tile.Build.Health = tile.Build.MaxHealth
		}
	}

	if w.isPowerRelevantBuildingLocked(tile) {
		w.invalidatePowerNetsLocked()
	}
	w.setBuildingOccupancyLocked(pos, tile, true)
	w.indexActiveTileLocked(pos, tile)
	w.applyBuildingConfigLocked(pos, result.Config, true)
	result.SelfConfigTargets = w.autoLinkPowerNodeLocked(pos)
	result.ChangedConfigs = w.autoLinkNearbyPowerNodesForBuildingLocked(pos)
	w.ensureTeamInventory(team)
	if affectsCoreStorageLinks(prevBlockName) || affectsCoreStorageLinks(w.blockNameByID(int16(tile.Block))) {
		w.refreshCoreStorageLinksLocked()
	}

	name := w.blockNameByID(int16(tile.Block))
	if _, ok := crafterProfilesByBlockName[name]; ok {
		w.crafterStates[pos] = crafterRuntimeState{}
	} else if _, ok := separatorProfilesByBlockName[name]; ok {
		w.crafterStates[pos] = crafterRuntimeState{}
	}
	if prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block)); ok {
		w.buildStates[pos] = buildCombatState{Cooldown: prof.Interval, BeamLastLength: 0}
	}

	w.clearOverlappingRebuildPlansLocked(team, tile.X, tile.Y, blockID)
	w.clearOverlappingTeamBuildPlansLocked(team, tile.X, tile.Y, blockID)
	return result
}

func (w *World) placeTileLocked(tile *Tile, team TeamID, blockID int16, rotation int8, config any, owner int32) {
	if tile == nil {
		return
	}
	pos := packTilePos(tile.X, tile.Y)
	w.entityEvents = append(w.entityEvents,
		EntityEvent{
			Kind:        EntityEventBuildPlaced,
			BuildPos:    pos,
			BuildOwner:  owner,
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
	posIndex := int32(tile.Y*w.model.Width + tile.X)
	placed := w.placeCompletedBuildingLocked(posIndex, tile, team, blockID, rotation, config)
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:        EntityEventBuildConstructed,
		BuildPos:    pos,
		BuildOwner:  owner,
		BuildTeam:   team,
		BuildBlock:  blockID,
		BuildRot:    rotation,
		BuildConfig: placed.Config,
	})
	for _, target := range placed.SelfConfigTargets {
		if target < 0 || int(target) >= len(w.model.Tiles) {
			continue
		}
		targetTile := &w.model.Tiles[target]
		w.appendBuildConfigValueEventLocked(posIndex, packTilePos(targetTile.X, targetTile.Y))
	}
	for _, changed := range placed.ChangedConfigs {
		if changed.targetPos < 0 || int(changed.targetPos) >= len(w.model.Tiles) {
			continue
		}
		targetTile := &w.model.Tiles[changed.targetPos]
		w.appendBuildConfigValueEventLocked(changed.nodePos, packTilePos(targetTile.X, targetTile.Y))
	}
}

func (w *World) destroyTileLocked(tile *Tile, fallbackTeam TeamID, owner int32) {
	if tile == nil || (tile.Block == 0 && tile.Build == nil) {
		return
	}
	pos := int32(tile.Y*w.model.Width + tile.X)
	blockID := int16(tile.Block)
	oldBlockName := w.blockNameByID(blockID)
	teamOld := tile.Team
	if tile.Build != nil && tile.Build.Team != 0 {
		teamOld = tile.Build.Team
	}
	if teamOld == 0 {
		teamOld = fallbackTeam
	}
	powerRelevant := w.isPowerRelevantBuildingLocked(tile)
	if powerRelevant {
		w.clearPowerLinksForBuildingLocked(pos)
	}
	w.refundDeconstructCost(tile, fallbackTeam)
	// CRITICAL: Remove from indices BEFORE clearing tile data
	w.removeActiveTileIndexLocked(pos, tile)
	w.setBuildingOccupancyLocked(pos, tile, false)
	tile.Block = 0
	tile.Rotation = 0
	tile.Team = 0
	tile.Build = nil
	w.clearBuildingRuntimeLocked(pos)
	if powerRelevant {
		w.invalidatePowerNetsLocked()
	}
	if affectsCoreStorageLinks(oldBlockName) {
		w.refreshCoreStorageLinksLocked()
	}
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventBuildDestroyed,
		BuildPos:   packTilePos(tile.X, tile.Y),
		BuildOwner: owner,
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
			w.cancelPendingBuildLocked(pos, st)
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
		w.cancelPendingBuildLocked(pos, st)
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
		w.cancelPendingBuildLocked(pos, st)
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
	w.cancelBuildPlansByOwnerLocked(owner)
}

func (w *World) cancelBuildPlansByOwnerLocked(owner int32) {
	if owner == 0 || w.model == nil {
		return
	}
	delete(w.builderStates, owner)
	if w.model == nil {
		return
	}
	for pos, st := range w.pendingBuilds {
		if st.Owner != owner {
			continue
		}
		w.cancelPendingBuildLocked(pos, st)
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

func (w *World) SetEntityPlayerController(id, playerID int32) (RawEntity, bool) {
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
		e.PlayerID = playerID
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

func (w *World) SetEntityMoveTo(id int32, x, y, speed float32) (RawEntity, bool) {
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
		e.Behavior = "move"
		e.TargetID = 0
		e.PatrolAX = x
		e.PatrolAY = y
		e.PatrolBX = 0
		e.PatrolBY = 0
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
		w.model.EntitiesRev++
		return *e, true
	}
	return RawEntity{}, false
}

func (w *World) SetEntityCommandIdle(id int32) (RawEntity, bool) {
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
		e.Behavior = "command"
		e.TargetID = 0
		e.PatrolAX = 0
		e.PatrolAY = 0
		e.PatrolBX = 0
		e.PatrolBY = 0
		e.PatrolToB = false
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

func (w *World) stepEntities(delta time.Duration) (movementDur, combatDur, buildingCombatDur, bulletDur time.Duration) {
	if w.model == nil {
		return 0, 0, 0, 0
	}
	dt := float32(delta.Seconds())
	if dt <= 0 {
		return 0, 0, 0, 0
	}
	movementStartedAt := time.Now()
	maxX := float32(w.model.Width * 8)
	maxY := float32(w.model.Height * 8)
	idToIndex := map[int32]int{}
	for i := range w.model.Entities {
		w.ensureEntityDefaults(&w.model.Entities[i])
		idToIndex[w.model.Entities[i].ID] = i
	}
	spatial := buildEntitySpatialIndex(w.model.Entities)
	teamSpatial := buildTeamEntitySpatialIndexes(w.model.Entities)
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
		prevStatusCount := len(e.Statuses)
		prevHealth := e.Health
		prevShield := e.Shield
		prevDisarmed := e.Disarmed
		w.updateEntityStatuses(e, dt)
		if prevStatusCount != len(e.Statuses) || prevHealth != e.Health || prevShield != e.Shield || prevDisarmed != e.Disarmed {
			changed = true
		}
		if e.Shield < e.ShieldMax && e.ShieldRegen > 0 {
			e.Shield += e.ShieldRegen * dt
			if e.Shield > e.ShieldMax {
				e.Shield = e.ShieldMax
			}
			changed = true
		}
		w.stepEntityAutonomousAILocked(e, dt, spatial, teamSpatial)
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
		if w.stepEntityMiningLocked(e, dt) {
			changed = true
		}
		if changed {
			w.model.EntitiesRev++
		}

		out := e.X < 0 || e.Y < 0 || e.X > maxX || e.Y > maxY
		if out && e.PlayerID != 0 {
			e.X = clampf(e.X, 0, maxX)
			e.Y = clampf(e.Y, 0, maxY)
			e.VelX = 0
			e.VelY = 0
			out = false
			w.model.EntitiesRev++
		}
		expired := e.LifeSec > 0 && e.AgeSec >= e.LifeSec
		dead := e.Health <= 0
		if !out && !expired && !dead {
			i++
			continue
		}
		removed := *e
		if dead {
			w.handleEntityDeathAbilitiesLocked(removed)
		}
		delete(w.builderStates, removed.ID)
		w.cancelBuildPlansByOwnerLocked(removed.ID)
		delete(w.unitMountCDs, removed.ID)
		delete(w.unitMountStates, removed.ID)
		delete(w.unitTargets, removed.ID)
		delete(w.unitAIStates, removed.ID)
		delete(w.unitMiningStates, removed.ID)
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
	spatial = buildEntitySpatialIndex(w.model.Entities)
	teamSpatial = buildTeamEntitySpatialIndexes(w.model.Entities)
	movementDur = time.Since(movementStartedAt)

	w.stepEntityAbilities(dt)
	idToIndex = map[int32]int{}
	for i := range w.model.Entities {
		idToIndex[w.model.Entities[i].ID] = i
	}
	spatial = buildEntitySpatialIndex(w.model.Entities)
	teamSpatial = buildTeamEntitySpatialIndexes(w.model.Entities)

	combatStartedAt := time.Now()
	w.stepEntityCombat(dt, idToIndex, spatial, teamSpatial)
	combatDur = time.Since(combatStartedAt)

	buildingCombatStartedAt := time.Now()
	w.stepBuildingCombat(dt, idToIndex, spatial, teamSpatial)
	buildingCombatDur = time.Since(buildingCombatStartedAt)

	bulletStartedAt := time.Now()
	w.stepPendingMountShots(dt, idToIndex)
	w.stepBullets(dt, idToIndex, spatial, teamSpatial)
	bulletDur = time.Since(bulletStartedAt)
	return movementDur, combatDur, buildingCombatDur, bulletDur
}

func (w *World) stepEntityCombat(dt float32, idToIndex map[int32]int, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) {
	ents := w.model.Entities
	if len(ents) == 0 {
		return
	}
	for i := range ents {
		e := &ents[i]
		if !canEntityAttack(*e) {
			continue
		}
		if mounts := w.unitMountProfilesForEntity(*e); len(mounts) > 0 {
			w.stepEntityMountedCombat(e, mounts, dt, idToIndex, spatial, teamSpatial)
			continue
		}
		if e.AttackCooldown > 0 {
			e.AttackCooldown -= dt * attackCooldownScale(*e)
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
		if e.AttackBuildings && e.AttackPreferBuildings {
			if pos, tx, ty, ok := w.findNearestEnemyBuilding(*e, rangeLimit); ok {
				if !w.tryConsumeEntityAmmoLocked(e, maxf(e.AmmoPerShot, 1)) {
					w.unitTargets[e.ID] = track
					continue
				}
				e.AttackCooldown = maxf(e.AttackInterval, 0.2)
				e.Rotation = lookAt(e.X, e.Y, tx, ty)
				w.applyShootStatus(e)
				if e.AttackFireMode == "beam" {
					w.fireBeamAtBuilding(*e, pos, tx, ty, false)
				} else {
					w.spawnBullet(*e, tx, ty, false)
				}
				w.unitTargets[e.ID] = track
				continue
			}
		}
		if tid, ok := w.acquireTrackedEntityTarget(*e, ents, idToIndex, spatial, teamSpatial, rangeLimit, e.AttackTargetAir, e.AttackTargetGround, e.AttackTargetPriority, &track, dt, retargetDelay); ok {
			if idx, exists := idToIndex[tid]; exists && idx >= 0 && idx < len(ents) {
				target := &ents[idx]
				if !w.tryConsumeEntityAmmoLocked(e, maxf(e.AmmoPerShot, 1)) {
					w.unitTargets[e.ID] = track
					continue
				}
				e.AttackCooldown = maxf(e.AttackInterval, 0.2)
				e.Rotation = lookAt(e.X, e.Y, target.X, target.Y)
				w.applyShootStatus(e)
				if e.AttackFireMode == "beam" {
					w.fireBeamAtEntity(*e, target, idx, false)
				} else {
					w.spawnBullet(*e, target.X, target.Y, false)
				}
				w.unitTargets[e.ID] = track
				continue
			}
		}
		w.unitTargets[e.ID] = track
		if e.AttackBuildings {
			if pos, tx, ty, ok := w.findNearestEnemyBuilding(*e, rangeLimit); ok {
				if !w.tryConsumeEntityAmmoLocked(e, maxf(e.AmmoPerShot, 1)) {
					continue
				}
				e.AttackCooldown = maxf(e.AttackInterval, 0.2)
				e.Rotation = lookAt(e.X, e.Y, tx, ty)
				w.applyShootStatus(e)
				if e.AttackFireMode == "beam" {
					w.fireBeamAtBuilding(*e, pos, tx, ty, false)
				} else {
					w.spawnBullet(*e, tx, ty, false)
				}
			}
		}
	}
}

func (w *World) stepEntityMountedCombat(e *RawEntity, mounts []unitWeaponMountProfile, dt float32, idToIndex map[int32]int, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) {
	if e == nil || len(mounts) == 0 {
		return
	}
	states := w.ensureUnitMountStates(e.ID, mounts)
	scale := attackCooldownScale(*e)
	for i := range mounts {
		lastReload := states[i].Reload
		if states[i].Reload > 0 {
			states[i].Reload -= dt * scale
			if states[i].Reload < 0 {
				states[i].Reload = 0
			}
		}
		if mounts[i].Alternate && mounts[i].OtherSide >= 0 {
			half := mounts[i].Interval * 0.5
			if half > 0 && states[i].Reload <= half && lastReload > half {
				other := int(mounts[i].OtherSide)
				states[i].Side = !states[i].Side
				if other >= 0 && other < len(states) {
					states[other].Side = !states[other].Side
				}
			}
		}
	}

	for mi := range mounts {
		mount := mounts[mi]
		state := &states[mi]
		rangeLimit := mount.Range
		if rangeLimit <= 0 {
			rangeLimit = e.AttackRange
		}
		if rangeLimit <= 0 {
			rangeLimit = 56
		}
		baseX, baseY := unitMountBasePosition(*e, mount)

		if mount.NoAttack {
			state.TargetBuildPos = -1
			state.Warmup = warmupToward(state.Warmup, 0, mount.ShootWarmupSpeed, mount.LinearWarmup, dt)
			if mount.RepairBeam {
				src := RawEntity{ID: e.ID, Team: e.Team, X: baseX, Y: baseY}
				if entIdx, pos, tx, ty, ok := w.findRepairTarget(src, mount, rangeLimit); ok {
					w.updateMountAim(*e, mount, state, tx, ty, dt)
					state.Warmup = warmupToward(state.Warmup, 1, mount.ShootWarmupSpeed, mount.LinearWarmup, dt)
					deltaFrames := dt * 60
					if entIdx >= 0 && entIdx < len(w.model.Entities) {
						target := &w.model.Entities[entIdx]
						amount := mount.RepairSpeed*deltaFrames + mount.FractionRepairSpeed*deltaFrames*target.MaxHealth/100
						_ = w.healEntity(target, amount)
					} else if pos >= 0 {
						x := int(pos) % w.model.Width
						y := int(pos) / w.model.Width
						if w.model.InBounds(x, y) {
							build := w.model.Tiles[pos].Build
							if build != nil {
								amount := mount.RepairSpeed*deltaFrames + mount.FractionRepairSpeed*deltaFrames*build.MaxHealth/100
								_ = w.healBuilding(pos, amount)
							}
						}
					}
				}
			}
			continue
		}

		if mount.PointDefense {
			state.TargetBuildPos = -1
			targetIdx := w.findPointDefenseTarget(e.Team, baseX, baseY, rangeLimit)
			state.Warmup = warmupToward(state.Warmup, 0, mount.ShootWarmupSpeed, mount.LinearWarmup, dt)
			if targetIdx >= 0 {
				target := w.bullets[targetIdx]
				w.updateMountAim(*e, mount, state, target.X, target.Y, dt)
				state.Warmup = warmupToward(state.Warmup, 1, mount.ShootWarmupSpeed, mount.LinearWarmup, dt)
				if states[mi].Reload <= 0 &&
					(mount.MinShootVelocity < 0 || entityVelocityLen(*e) >= mount.MinShootVelocity) &&
					state.Warmup >= mount.MinWarmup &&
					(!mount.Rotate || angleWithin(state.Rotation, state.TargetRotation, mount.ShootCone)) &&
					(!mount.Alternate || mount.OtherSide < 0 || state.Side == mount.FlipSprite) {
					src := *e
					applyMountWeaponProfile(&src, mount)
					sx, sy, _ := unitMountShootPosition(*e, mount, *state)
					src.X = sx
					src.Y = sy
					src.Rotation = lookAt(sx, sy, target.X, target.Y)
					if w.firePointDefenseMount(src, targetIdx) {
						reload := mount.Interval
						if reload <= 0 {
							reload = 1.0 / 60.0
						}
						state.Reload = reload
					}
				}
			}
			continue
		}

		if mount.MinShootVelocity >= 0 && entityVelocityLen(*e) < mount.MinShootVelocity {
			continue
		}

		src := RawEntity{ID: e.ID, Team: e.Team, X: baseX, Y: baseY}
		track := targetTrackState{TargetID: state.TargetID, RetargetCD: state.RetargetCD}
		retargetDelay := mount.TargetInterval
		if track.TargetID != 0 && mount.TargetSwitchInterval > 0 {
			retargetDelay = mount.TargetSwitchInterval
		}

		unitIdx := -1
		targetBuild := false
		buildPos := int32(-1)
		targetX, targetY := float32(0), float32(0)

		if mount.PreferBuildings && mount.HitBuildings {
			if pos, tx, ty, ok := w.findNearestEnemyBuilding(src, rangeLimit); ok {
				targetBuild = true
				buildPos = pos
				targetX, targetY = tx, ty
			}
		}

		if !targetBuild {
			if tid, ok := w.acquireTrackedEntityTarget(src, w.model.Entities, idToIndex, spatial, teamSpatial, rangeLimit, mount.TargetAir, mount.TargetGround, "nearest", &track, dt, retargetDelay); ok {
				if idx, exists := idToIndex[tid]; exists && idx >= 0 && idx < len(w.model.Entities) {
					unitIdx = idx
					targetX = w.model.Entities[idx].X
					targetY = w.model.Entities[idx].Y
				}
			}
		}

		if unitIdx < 0 && mount.HitBuildings {
			if pos, tx, ty, ok := w.findNearestEnemyBuilding(src, rangeLimit); ok {
				targetBuild = true
				buildPos = pos
				targetX, targetY = tx, ty
			}
		}

		state.TargetID = track.TargetID
		state.RetargetCD = track.RetargetCD
		state.TargetBuildPos = buildPos
		beamActive := mount.Continuous && state.BeamBulletID != 0
		reload := mount.Interval
		if reload <= 0 {
			reload = maxf(e.AttackInterval, 1.0/60.0)
		}
		warmupTarget := float32(0)
		if beamActive {
			warmupTarget = 1
		}
		if unitIdx < 0 && !targetBuild {
			state.Warmup = warmupToward(state.Warmup, warmupTarget, mount.ShootWarmupSpeed, mount.LinearWarmup, dt)
			if beamActive {
				if w.updateMountedBeamBullet(e, mount, state, dt) {
					state.Reload = reload
					continue
				}
			}
			continue
		}
		warmupTarget = 1

		w.updateMountAim(*e, mount, state, targetX, targetY, dt)
		state.Warmup = warmupToward(state.Warmup, warmupTarget, mount.ShootWarmupSpeed, mount.LinearWarmup, dt)
		if beamActive {
			if w.updateMountedBeamBullet(e, mount, state, dt) {
				if mount.AlwaysContinuous {
					w.keepMountedBeamAlive(e, mount, state)
				}
				state.Reload = reload
				continue
			}
		}
		if state.Reload > 0 && !(mount.AlwaysContinuous && state.BeamBulletID == 0) {
			continue
		}
		if state.Warmup < mount.MinWarmup {
			continue
		}
		if mount.Alternate && mount.OtherSide >= 0 && state.Side != mount.FlipSprite {
			continue
		}
		if mount.Rotate {
			if !angleWithin(state.Rotation, state.TargetRotation, mount.ShootCone) {
				continue
			}
		} else if !mount.AlwaysShooting && !angleWithin(e.Rotation+mount.BaseRotation, state.TargetRotation, mount.ShootCone) {
			continue
		}

		if w.triggerEntityMountFire(e, mi, mount, state, idToIndex) {
			state.Reload = reload
		}
	}

	w.unitMountStates[e.ID] = states
}

func (w *World) fireEntityMountAtUnit(e *RawEntity, target *RawEntity, mount unitWeaponMountProfile, state unitMountState, targetIdx int) bool {
	if e == nil || target == nil || target.Health <= 0 {
		return false
	}
	src := *e
	applyMountWeaponProfile(&src, mount)
	sx, sy, _ := unitMountShootPosition(*e, mount, state)
	src.X = sx
	src.Y = sy
	src.Rotation = lookAt(sx, sy, target.X, target.Y)
	if src.AttackFireMode == "beam" {
		w.applyMountShootStatus(e, mount)
		w.fireBeamAtEntity(src, target, targetIdx, false)
		return true
	}
	w.applyMountShootStatus(e, mount)
	w.spawnBullet(src, target.X, target.Y, false)
	return true
}

func (w *World) fireEntityMountAtBuilding(e *RawEntity, pos int32, tx, ty float32, mount unitWeaponMountProfile, state unitMountState) bool {
	if e == nil {
		return false
	}
	src := *e
	applyMountWeaponProfile(&src, mount)
	sx, sy, _ := unitMountShootPosition(*e, mount, state)
	src.X = sx
	src.Y = sy
	src.Rotation = lookAt(sx, sy, tx, ty)
	if src.AttackFireMode == "beam" {
		w.applyMountShootStatus(e, mount)
		w.fireBeamAtBuilding(src, pos, tx, ty, false)
		return true
	}
	w.applyMountShootStatus(e, mount)
	w.spawnBullet(src, tx, ty, false)
	return true
}

func applyMountStats(src *RawEntity, mount unitWeaponMountProfile) {
	if src == nil {
		return
	}
	if mount.DamageMul > 0 {
		src.AttackDamage *= mount.DamageMul
		src.AttackSplashDamage *= mount.DamageMul
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

func (w *World) stepBuildingCombat(dt float32, idToIndex map[int32]int, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) {
	if w.model == nil {
		return
	}
	ents := w.model.Entities
	for _, pos := range w.turretTilePositions {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			continue
		}
		t := &w.model.Tiles[pos]
		if t.Build == nil || t.Build.Health <= 0 || t.Build.Team == 0 {
			continue
		}
		prof, ok := w.getBuildingWeaponProfile(int16(t.Build.Block))
		if !ok || (prof.Damage <= 0 && prof.SplashDamage <= 0 && prof.StatusID == 0 && strings.TrimSpace(prof.StatusName) == "") || (!prof.ContinuousHold && prof.Interval <= 0) || prof.Range <= 0 {
			continue
		}
		prof = w.resolveBuildingWeaponProfileLocked(t, prof)
		state, exists := w.buildStates[pos]
		if !exists {
			state = buildCombatState{
				Ammo:           prof.AmmoCapacity,
				Power:          prof.PowerCapacity,
				TurretRotation: float32(t.Rotation) * 90,
				HasRotation:    true,
			}
		} else if !state.HasRotation {
			state.TurretRotation = float32(t.Rotation) * 90
			state.HasRotation = true
		}
		state = w.regenBuildState(pos, t, state, prof, t.Build.Team, dt)
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
			X:                        float32(t.X*8 + 4),
			Y:                        float32(t.Y*8 + 4),
			Rotation:                 float32(t.Rotation) * 90,
			Team:                     t.Build.Team,
			AttackFireMode:           prof.FireMode,
			AttackDamage:             prof.Damage,
			AttackSplashDamage:       prof.SplashDamage,
			AttackInterval:           prof.Interval,
			AttackRange:              prof.Range,
			AttackBulletType:         prof.BulletType,
			AttackBulletLifetime:     prof.BulletLifetime,
			AttackBulletHitSize:      prof.BulletHitSize,
			AttackBulletSpeed:        prof.BulletSpeed,
			AttackSplashRadius:       prof.SplashRadius,
			AttackBuildingDamage:     defaultBuildingDamageMultiplier(prof.BuildingDamage, prof.HitBuildings),
			AttackBuildingDamageSet:  prof.HitBuildings || prof.BuildingDamage != 0,
			AttackArmorMultiplier:    prof.ArmorMultiplier,
			AttackMaxDamageFraction:  prof.MaxDamageFraction,
			AttackShieldDamageMul:    prof.ShieldDamageMul,
			AttackPierceDamageFactor: prof.PierceDamageFactor,
			AttackPierceArmor:        prof.PierceArmor,
			AttackSlowSec:            prof.SlowSec,
			AttackSlowMul:            prof.SlowMul,
			AttackPierce:             prof.Pierce,
			AttackPierceBuilding:     prof.PierceBuilding,
			AttackChainCount:         prof.ChainCount,
			AttackChainRange:         prof.ChainRange,
			AttackStatusID:           prof.StatusID,
			AttackStatusName:         prof.StatusName,
			AttackStatusDuration:     prof.StatusDuration,
			AttackFragmentCount:      prof.FragmentCount,
			AttackFragmentSpread:     prof.FragmentSpread,
			AttackFragmentSpeed:      prof.FragmentSpeed,
			AttackFragmentLife:       prof.FragmentLife,
			AttackFragmentRand:       prof.FragmentRandomSpread,
			AttackFragmentAngle:      prof.FragmentAngle,
			AttackFragmentVelMin:     prof.FragmentVelocityMin,
			AttackFragmentVelMax:     prof.FragmentVelocityMax,
			AttackFragmentLifeMin:    prof.FragmentLifeMin,
			AttackFragmentLifeMax:    prof.FragmentLifeMax,
			AttackFragmentBullet:     cloneBulletRuntimeProfile(prof.FragmentBullet),
			AttackShootEffect:        prof.ShootEffect,
			AttackSmokeEffect:        prof.SmokeEffect,
			AttackHitEffect:          prof.HitEffect,
			AttackDespawnEffect:      prof.DespawnEffect,
			AttackTargetAir:          prof.TargetAir,
			AttackTargetGround:       prof.TargetGround,
			AttackTargetPriority:     prof.TargetPriority,
			AttackBuildings:          prof.HitBuildings,
		}
		if state.HasRotation {
			src.Rotation = state.TurretRotation
		}

		controlled, controlledCanShoot, aimX, aimY := w.controlledBuildingAimLocked(pos)
		targetIdx := -1
		targetBuildPos := int32(-1)
		targetX, targetY := float32(0), float32(0)
		hasAim := false
		targetRotation := src.Rotation
		if controlled {
			targetX, targetY = aimX, aimY
			hasAim = true
			targetRotation = updateBuildingAim(t, src, prof, &state, targetX, targetY, dt)
			src.Rotation = state.TurretRotation
		} else {
			targetIdx, targetBuildPos, targetX, targetY = w.acquireBuildingWeaponTarget(src, &state, prof, ents, idToIndex, spatial, teamSpatial)
			hasAim = targetIdx >= 0 || targetBuildPos >= 0
			if targetIdx >= 0 && targetIdx < len(ents) {
				targetX, targetY = predictBuildingAimPosition(src, ents[targetIdx], prof)
			}
			if hasAim {
				targetRotation = updateBuildingAim(t, src, prof, &state, targetX, targetY, dt)
				src.Rotation = state.TurretRotation
			}
		}

		if prof.ContinuousHold && isPersistentBeamBulletProfile(prof.Bullet) {
			if w.stepBuildingContinuousBeam(pos, &src, &state, prof, ents, idToIndex, spatial, teamSpatial, dt) {
				state.TurretRotation = src.Rotation
				state.HasRotation = true
			}
			w.buildStates[pos] = state
			continue
		}

		allowShot := hasAim && state.Cooldown <= 0 && (state.BurstRemain == 0 || state.BurstDelay <= 0)
		if allowShot && controlled && !controlledCanShoot {
			allowShot = false
		}
		if allowShot && !buildingCanFireAtAim(prof, src.Rotation, targetRotation) {
			allowShot = false
		}
		if allowShot && w.tryFireBuildingShot(pos, t, &src, &state, prof, ents, targetIdx, targetBuildPos, targetX, targetY, controlled, controlledCanShoot) {
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
			state.TurretRotation = src.Rotation
			state.HasRotation = true
		}
		w.buildStates[pos] = state
	}
}

func (w *World) buildingUsesItemAmmoLocked(tile *Tile, prof buildingWeaponProfile) bool {
	if w == nil || tile == nil || tile.Build == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Build.Block))))
	return classifyTurretBlockSyncKind(name, prof) == blockSyncItemTurret
}

func (w *World) buildingHidesInventoryItemsLocked(pos int32, tile *Tile) bool {
	_ = pos
	if w == nil || tile == nil || tile.Build == nil {
		return false
	}
	prof, ok := w.getBuildingWeaponProfile(int16(tile.Build.Block))
	return ok && w.buildingUsesItemAmmoLocked(tile, prof)
}

func (w *World) resolveBuildingWeaponProfileLocked(tile *Tile, prof buildingWeaponProfile) buildingWeaponProfile {
	if w == nil || tile == nil || tile.Build == nil {
		return prof
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Build.Block))))
	if ammoByItem, ok := turretItemAmmoBulletTypesByName[name]; ok {
		if ammoItem, ok := w.currentBuildingAmmoItemLocked(tile, prof); ok {
			if bulletType, exists := ammoByItem[ammoItem]; exists && bulletType > 0 {
				prof.BulletType = bulletType
				return prof
			}
		}
	}
	if ammoByLiquid, ok := turretLiquidAmmoBulletTypesByName[name]; ok {
		bestAmount := float32(0)
		bestType := int16(0)
		for _, stack := range tile.Build.Liquids {
			if stack.Amount <= 0.0001 {
				continue
			}
			if bulletType, exists := ammoByLiquid[stack.Liquid]; exists && bulletType > 0 && stack.Amount > bestAmount {
				bestAmount = stack.Amount
				bestType = bulletType
			}
		}
		if bestType > 0 {
			prof.BulletType = bestType
		}
	}
	return prof
}

func (w *World) buildingAcceptsAmmoItemLocked(tile *Tile, prof buildingWeaponProfile, item ItemID) bool {
	if w == nil || tile == nil || tile.Build == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(w.blockNameByID(int16(tile.Build.Block))))
	ammoByItem, ok := turretItemAmmoBulletTypesByName[name]
	if !ok {
		return true
	}
	_, exists := ammoByItem[item]
	return exists
}

func (w *World) firstBuildingAmmoItemLocked(tile *Tile, prof buildingWeaponProfile) (ItemID, bool) {
	if tile == nil || tile.Build == nil {
		return 0, false
	}
	w.normalizeTurretAmmoEntriesLocked(tile, prof)
	for _, entry := range tile.Build.Items {
		if entry.Amount <= 0 || !w.buildingAcceptsAmmoItemLocked(tile, prof, entry.Item) {
			continue
		}
		return entry.Item, true
	}
	return 0, false
}

func (w *World) buildingItemAmmoCapacityLocked(tile *Tile, prof buildingWeaponProfile) int32 {
	if !w.buildingUsesItemAmmoLocked(tile, prof) || prof.AmmoCapacity <= 0 {
		return 0
	}
	return int32(math.Ceil(float64(prof.AmmoCapacity)))
}

func buildingAmmoPerShotCount(prof buildingWeaponProfile) int32 {
	if prof.AmmoPerShot <= 0 {
		return 0
	}
	amount := int32(math.Ceil(float64(prof.AmmoPerShot)))
	if amount < 1 {
		amount = 1
	}
	return amount
}

func (w *World) buildingHasAmmoLocked(pos int32, tile *Tile, prof buildingWeaponProfile, state buildCombatState) bool {
	if w.buildingUsesItemAmmoLocked(tile, prof) {
		required := buildingAmmoPerShotCount(prof)
		if required <= 0 || tile == nil || tile.Build == nil {
			return required <= 0
		}
		if amount, ok := w.currentBuildingAmmoAmountLocked(tile, prof); ok {
			return amount >= required
		}
		return false
	}
	if prof.AmmoPerShot > 0 {
		return state.Ammo >= prof.AmmoPerShot
	}
	return true
}

func (w *World) consumeBuildingAmmoLocked(pos int32, tile *Tile, prof buildingWeaponProfile, state *buildCombatState) bool {
	if prof.AmmoPerShot <= 0 {
		return true
	}
	if w.buildingUsesItemAmmoLocked(tile, prof) {
		if tile == nil || tile.Build == nil {
			return false
		}
		remaining := buildingAmmoPerShotCount(prof)
		index := w.currentBuildingAmmoIndexLocked(tile, prof)
		if index < 0 || index >= len(tile.Build.Items) || tile.Build.Items[index].Amount < remaining {
			return false
		}
		tile.Build.Items[index].Amount -= remaining
		if tile.Build.Items[index].Amount <= 0 {
			tile.Build.Items = append(tile.Build.Items[:index], tile.Build.Items[index+1:]...)
		}
		w.emitBlockItemSyncLocked(pos)
		return true
	}
	if state == nil || state.Ammo < prof.AmmoPerShot {
		return false
	}
	state.Ammo -= prof.AmmoPerShot
	if state.Ammo < 0 {
		state.Ammo = 0
	}
	return true
}

func (w *World) regenBuildState(pos int32, tile *Tile, state buildCombatState, prof buildingWeaponProfile, team TeamID, dt float32) buildCombatState {
	if prof.AmmoCapacity > 0 && !w.buildingUsesItemAmmoLocked(tile, prof) {
		if prof.AmmoRegen > 0 {
			state.Ammo = minf(prof.AmmoCapacity, state.Ammo+prof.AmmoRegen*dt)
		}
	}
	if prof.PowerCapacity > 0 {
		if prof.PowerRegen > 0 {
			got := w.consumePowerAtLocked(pos, team, prof.PowerRegen*dt)
			state.Power = minf(prof.PowerCapacity, state.Power+got)
		}
	}
	return state
}

func buildingWeaponRetargetDelay(prof buildingWeaponProfile, hasTarget bool) float32 {
	if hasTarget && prof.TargetSwitchInterval > 0 {
		return maxf(prof.TargetSwitchInterval, 1.0/60.0)
	}
	if prof.TargetInterval > 0 {
		return maxf(prof.TargetInterval, 1.0/60.0)
	}
	return maxf(prof.Interval*0.55, 0.22)
}

func buildingWeaponShootCone(prof buildingWeaponProfile) float32 {
	if prof.ShootCone > 0 {
		return prof.ShootCone
	}
	return 5
}

func buildingCanFireAtAim(prof buildingWeaponProfile, currentRotation, targetRotation float32) bool {
	return angleWithin(currentRotation, targetRotation, buildingWeaponShootCone(prof))
}

func predictBuildingAimPosition(src RawEntity, target RawEntity, prof buildingWeaponProfile) (float32, float32) {
	if !prof.PredictTarget || prof.BulletSpeed < 0.01 {
		return target.X, target.Y
	}
	dx := target.X - src.X
	dy := target.Y - src.Y
	vx := target.VelX
	vy := target.VelY
	speed := prof.BulletSpeed
	a := vx*vx + vy*vy - speed*speed
	b := 2 * (dx*vx + dy*vy)
	c := dx*dx + dy*dy
	t := float32(-1)
	if math.Abs(float64(a)) < 1e-6 {
		if math.Abs(float64(b)) > 1e-6 {
			t = -c / b
		}
	} else {
		discriminant := b*b - 4*a*c
		if discriminant >= 0 {
			sqrtDisc := float32(math.Sqrt(float64(discriminant)))
			t1 := (-b - sqrtDisc) / (2 * a)
			t2 := (-b + sqrtDisc) / (2 * a)
			switch {
			case t1 > 0 && t2 > 0:
				t = minf(t1, t2)
			case t1 > 0:
				t = t1
			case t2 > 0:
				t = t2
			}
		}
	}
	if t <= 0 {
		return target.X, target.Y
	}
	return target.X + vx*t, target.Y + vy*t
}

func updateBuildingAim(tile *Tile, src RawEntity, prof buildingWeaponProfile, state *buildCombatState, aimX, aimY, dt float32) float32 {
	targetRotation := normalizeAngleDeg(lookAt(src.X, src.Y, aimX, aimY))
	if state == nil {
		return targetRotation
	}
	baseRotation := prof.BaseRotation
	if tile != nil {
		baseRotation += float32(tile.Rotation) * 90
	}
	currentRotation := baseRotation
	if state.HasRotation {
		currentRotation = state.TurretRotation
	}
	if prof.Rotate {
		speed := prof.RotateSpeed
		if speed <= 0 {
			speed = 20
		}
		currentRotation = moveAngleToward(currentRotation, targetRotation, speed*dt*60)
		if prof.RotationLimit > 0 && prof.RotationLimit < 360 {
			dst := angleDistDeg(currentRotation, baseRotation)
			limit := prof.RotationLimit * 0.5
			if dst > limit {
				currentRotation = moveAngleToward(currentRotation, baseRotation, dst-limit)
			}
		}
	} else {
		currentRotation = targetRotation
	}
	state.TurretRotation = normalizeAngleDeg(currentRotation)
	state.HasRotation = true
	if tile != nil {
		cardinal := buildRotationFromDegrees(state.TurretRotation)
		tile.Rotation = cardinal
		if tile.Build != nil {
			tile.Build.Rotation = cardinal
		}
	}
	return targetRotation
}

func (w *World) fireAimedBuildingBeam(src RawEntity, prof buildingWeaponProfile, tx, ty float32, sourceIsBuilding bool) bool {
	if w == nil {
		return false
	}
	beam := simBullet{
		Team:               src.Team,
		X:                  src.X,
		Y:                  src.Y,
		Damage:             src.AttackDamage * w.outgoingDamageScale(src, sourceIsBuilding),
		HitBuilds:          src.AttackBuildings,
		BuildingDamage:     entityBuildingDamageMultiplier(src),
		ArmorMultiplier:    src.AttackArmorMultiplier,
		MaxDamageFraction:  src.AttackMaxDamageFraction,
		ShieldDamageMul:    src.AttackShieldDamageMul,
		PierceDamageFactor: src.AttackPierceDamageFactor,
		PierceArmor:        src.AttackPierceArmor,
		SlowSec:            src.AttackSlowSec,
		SlowMul:            clampf(src.AttackSlowMul, 0.2, 1),
		StatusID:           src.AttackStatusID,
		StatusName:         src.AttackStatusName,
		StatusDuration:     src.AttackStatusDuration,
		TargetAir:          src.AttackTargetAir,
		TargetGround:       src.AttackTargetGround,
		AimX:               tx,
		AimY:               ty,
		BulletClass:        prof.BulletClass,
		BeamLength:         prof.Range,
		SplashRadius:       src.AttackSplashRadius,
	}
	w.emitAttackFireEffectsLocked(src)
	impacted := false
	if isPointLaserBulletClass(beam.BulletClass) {
		impacted = w.applyPointBeamDamage(beam)
	} else {
		impacted = w.applyLineBeamDamage(beam)
	}
	if impacted {
		w.emitAttackHitEffectLocked(src, tx, ty)
	}
	return true
}

func (w *World) tryFireBuildingShot(buildPos int32, tile *Tile, src *RawEntity, state *buildCombatState, prof buildingWeaponProfile, ents []RawEntity, targetIdx int, targetBuildPos int32, tx, ty float32, controlled bool, canShoot bool) bool {
	if src == nil || state == nil {
		return false
	}
	if !w.buildingHasAmmoLocked(buildPos, tile, prof, *state) {
		return false
	}
	if prof.PowerPerShot > 0 && state.Power < prof.PowerPerShot {
		return false
	}

	fired := false
	if controlled {
		if !canShoot {
			return false
		}
		if src.AttackFireMode == "beam" {
			return w.fireAimedBuildingBeam(*src, prof, tx, ty, true)
		}
		w.spawnBullet(*src, tx, ty, true)
		fired = true
	} else {
		if targetIdx >= 0 && targetIdx < len(ents) {
			target := &ents[targetIdx]
			if src.AttackFireMode == "beam" {
				w.fireBeamAtEntity(*src, target, targetIdx, true)
			} else {
				w.spawnBullet(*src, tx, ty, true)
			}
			fired = true
		}
		if !fired && targetBuildPos >= 0 {
			if src.AttackFireMode == "beam" {
				w.fireBeamAtBuilding(*src, targetBuildPos, tx, ty, true)
			} else {
				w.spawnBullet(*src, tx, ty, true)
			}
			fired = true
		}
	}
	if !fired {
		return false
	}
	if !w.consumeBuildingAmmoLocked(buildPos, tile, prof, state) {
		return false
	}
	if prof.PowerPerShot > 0 {
		state.Power -= prof.PowerPerShot
		if state.Power < 0 {
			state.Power = 0
		}
	}
	return true
}

func (w *World) spawnBullet(src RawEntity, tx, ty float32, sourceIsBuilding bool) {
	w.spawnBulletWithAngle(src, tx, ty, lookAt(src.X, src.Y, tx, ty), 1, pendingMountShot{}, sourceIsBuilding)
}

func (w *World) spawnBulletWithAngle(src RawEntity, tx, ty, angle, speedScale float32, shot pendingMountShot, sourceIsBuilding bool) {
	bulletSpeed := src.AttackBulletSpeed
	if bulletSpeed <= 0 {
		speed := src.MoveSpeed
		if speed <= 0 {
			speed = 18
		}
		bulletSpeed = maxf(speed*2.2, 28)
	}
	if speedScale <= 0 {
		speedScale = 1
	}
	bulletSpeed *= speedScale
	rad := float32(angle * math.Pi / 180)
	damageScale := w.outgoingDamageScale(src, sourceIsBuilding)
	lifeSec := src.AttackBulletLifetime
	if lifeSec <= 0 && bulletSpeed > 0 {
		lifeSec = maxf(src.AttackRange/bulletSpeed, 0.6)
	}
	radius := maxf(src.AttackBulletHitSize*0.5, 4)
	buildingMul := entityBuildingDamageMultiplier(src)
	b := simBullet{
		ID:                 w.bulletNextID,
		Team:               src.Team,
		X:                  src.X,
		Y:                  src.Y,
		VX:                 float32(math.Cos(float64(rad))) * bulletSpeed,
		VY:                 float32(math.Sin(float64(rad))) * bulletSpeed,
		Damage:             src.AttackDamage * damageScale,
		SplashDamage:       src.AttackSplashDamage * damageScale,
		LifeSec:            lifeSec,
		AgeSec:             0,
		Radius:             radius,
		HitUnits:           true,
		HitBuilds:          src.AttackBuildings,
		BulletType:         src.AttackBulletType,
		SplashRadius:       src.AttackSplashRadius,
		BuildingDamage:     buildingMul,
		ArmorMultiplier:    src.AttackArmorMultiplier,
		MaxDamageFraction:  src.AttackMaxDamageFraction,
		ShieldDamageMul:    src.AttackShieldDamageMul,
		PierceDamageFactor: src.AttackPierceDamageFactor,
		PierceArmor:        src.AttackPierceArmor,
		SlowSec:            src.AttackSlowSec,
		SlowMul:            clampf(src.AttackSlowMul, 0.2, 1),
		PierceRemain:       src.AttackPierce,
		PierceBuilding:     src.AttackPierceBuilding,
		ChainCount:         src.AttackChainCount,
		ChainRange:         src.AttackChainRange,
		FragmentCount:      src.AttackFragmentCount,
		FragmentSpread:     src.AttackFragmentSpread,
		FragmentSpeed:      src.AttackFragmentSpeed,
		FragmentLife:       src.AttackFragmentLife,
		FragmentRand:       src.AttackFragmentRand,
		FragmentAngle:      src.AttackFragmentAngle,
		FragmentVelMin:     src.AttackFragmentVelMin,
		FragmentVelMax:     src.AttackFragmentVelMax,
		FragmentLifeMin:    src.AttackFragmentLifeMin,
		FragmentLifeMax:    src.AttackFragmentLifeMax,
		FragmentBullet:     cloneBulletRuntimeProfile(src.AttackFragmentBullet),
		StatusID:           src.AttackStatusID,
		StatusName:         src.AttackStatusName,
		StatusDuration:     src.AttackStatusDuration,
		ShootEffect:        src.AttackShootEffect,
		SmokeEffect:        src.AttackSmokeEffect,
		HitEffect:          src.AttackHitEffect,
		DespawnEffect:      src.AttackDespawnEffect,
		TargetAir:          src.AttackTargetAir,
		TargetGround:       src.AttackTargetGround,
		TargetPriority:     src.AttackTargetPriority,
		HelixScl:           shot.HelixScl,
		HelixMag:           shot.HelixMag,
		HelixOffset:        shot.HelixOffset,
	}
	w.bulletNextID++
	w.bullets = append(w.bullets, b)
	w.emitAttackFireEffectsLocked(src)
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

func (w *World) stepBullets(dt float32, idToIndex map[int32]int, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex) {
	if len(w.bullets) == 0 {
		return
	}
	for i := 0; i < len(w.bullets); {
		b := &w.bullets[i]
		if isPersistentBeamBulletClass(b.BulletClass) {
			impacted, expired := w.stepPersistentBeamBullet(b, dt)
			impactRot := beamImpactRotation(*b)
			if impacted {
				tx, ty := beamEndPosition(*b)
				w.emitEffectLocked(b.HitEffect, tx, ty, impactRot)
			} else if expired {
				tx, ty := beamEndPosition(*b)
				w.emitEffectLocked(b.DespawnEffect, tx, ty, impactRot)
			}
			if !expired {
				i++
				continue
			}
			last := len(w.bullets) - 1
			w.bullets[i] = w.bullets[last]
			w.bullets = w.bullets[:last]
			continue
		}
		b.AgeSec += dt
		b.X += b.VX * dt
		b.Y += b.VY * dt
		if b.HelixScl > 0 && b.HelixMag != 0 {
			rot := float32(math.Atan2(float64(b.VY), float64(b.VX)) * 180 / math.Pi)
			side := float32(math.Sin(float64((b.AgeSec*60+b.HelixOffset)/b.HelixScl))) * b.HelixMag * dt * 60
			b.X += trnsx(rot, 0, side)
			b.Y += trnsy(rot, 0, side)
		}
		if handled, remove := w.absorbBulletByUnitAbilitiesLocked(b, dt); handled {
			if remove {
				last := len(w.bullets) - 1
				w.bullets[i] = w.bullets[last]
				w.bullets = w.bullets[:last]
				continue
			}
			i++
			continue
		}
		hit := false
		impacted := false
		if b.HitUnits {
			if idx, ok := findHitEnemyEntityIndex(*b, w.model.Entities, spatial, teamSpatial, b.Radius, b.TargetAir, b.TargetGround); ok && idx >= 0 && idx < len(w.model.Entities) {
				target := &w.model.Entities[idx]
				if remaining, absorbed := w.absorbEntityAbilityDamage(target, b.X, b.Y, b.Damage); absorbed {
					hit = true
					impacted = true
				} else {
					initialHealth := w.applyDamageToEntityProfile(target, remaining, bulletDamageApplyProfile(*b))
					applyPierceDamageLoss(&b.Damage, b.PierceDamageFactor, initialHealth)
					applySlow(target, b.SlowSec, b.SlowMul)
					w.applyStatusToEntity(target, b.StatusID, b.StatusName, b.StatusDuration)
					hit = true
					impacted = true
				}
				w.applyChainDamage(*b, idx)
				w.applySplashDamage(*b)
				if b.PierceRemain > 0 {
					b.PierceRemain--
					hit = false
				}
			}
		}
		if !hit && b.HitBuilds {
			if pos, _, _, ok := w.findNearestEnemyBuilding(RawEntity{X: b.X, Y: b.Y, Team: b.Team}, b.Radius); ok {
				initialHealth := float32(0)
				if pos >= 0 && int(pos) < len(w.model.Tiles) && w.model.Tiles[pos].Build != nil {
					initialHealth = w.model.Tiles[pos].Build.Health
				}
				if w.applyDamageToBuildingProfile(pos, b.Damage*b.BuildingDamage, bulletDamageApplyProfile(*b)) {
					applyPierceDamageLoss(&b.Damage, b.PierceDamageFactor, initialHealth)
					w.applySplashDamage(*b)
					hit = true
					impacted = true
					if b.PierceBuilding && b.PierceRemain > 0 {
						b.PierceRemain--
						hit = false
					}
				}
			}
		}
		expired := b.AgeSec >= b.LifeSec
		impactRot := float32(math.Atan2(float64(b.VY), float64(b.VX)) * 180 / math.Pi)
		if impacted {
			w.emitEffectLocked(b.HitEffect, b.X, b.Y, impactRot)
		} else if expired {
			w.emitEffectLocked(b.DespawnEffect, b.X, b.Y, impactRot)
		}
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
	for i := int32(0); i < n; i++ {
		t := float32(i)
		offset := float32(0)
		if n > 1 {
			offset = (t/float32(n-1))*spread - spread/2
		}
		randomSpread := parent.FragmentRand
		if randomSpread <= 0 {
			randomSpread = spread
		}
		ang := baseAngle + parent.FragmentAngle + float32(offset) + (rand.Float32()-0.5)*randomSpread
		rad := float32(ang * math.Pi / 180)
		template := parent.FragmentBullet
		speed := parent.FragmentSpeed
		life := parent.FragmentLife
		damage := parent.Damage * 0.45
		splashDamage := parent.SplashDamage * 0.45
		splashRadius := parent.SplashRadius * 0.5
		radius := float32(4)
		buildingDamage := parent.BuildingDamage
		armorMultiplier := parent.ArmorMultiplier
		maxDamageFraction := parent.MaxDamageFraction
		shieldDamageMul := parent.ShieldDamageMul
		pierceDamageFactor := parent.PierceDamageFactor
		pierceArmor := parent.PierceArmor
		bulletType := parent.BulletType
		bulletClass := parent.BulletClass
		pierce := int32(0)
		pierceBuilding := false
		statusID := parent.StatusID
		statusName := parent.StatusName
		statusDuration := parent.StatusDuration
		hitBuilds := parent.HitBuilds
		targetAir := parent.TargetAir
		targetGround := parent.TargetGround
		hitEffect := parent.HitEffect
		despawnEffect := parent.DespawnEffect
		fragCount := int32(0)
		fragSpread2 := float32(0)
		fragRand := float32(0)
		fragAngle := float32(0)
		fragVelMin := float32(0)
		fragVelMax := float32(0)
		fragLifeMin := float32(0)
		fragLifeMax := float32(0)
		var fragBullet *bulletRuntimeProfile
		if template != nil {
			if template.Speed > 0 {
				speed = template.Speed
			}
			if template.Lifetime > 0 {
				life = template.Lifetime
			}
			damage = template.Damage
			splashDamage = template.SplashDamage
			splashRadius = template.SplashRadius
			radius = maxf(template.HitSize*0.5, 4)
			buildingDamage = template.BuildingDamage
			armorMultiplier = template.ArmorMultiplier
			maxDamageFraction = template.MaxDamageFraction
			shieldDamageMul = template.ShieldDamageMul
			pierceDamageFactor = template.PierceDamageFactor
			pierceArmor = template.PierceArmor
			bulletType = template.BulletType
			bulletClass = template.ClassName
			pierce = template.Pierce
			pierceBuilding = template.PierceBuilding
			statusID = template.StatusID
			statusName = template.StatusName
			statusDuration = template.StatusDuration
			hitBuilds = template.HitBuildings
			targetAir = template.TargetAir
			targetGround = template.TargetGround
			hitEffect = template.HitEffect
			despawnEffect = template.DespawnEffect
			fragCount = template.FragmentCount
			fragSpread2 = template.FragmentSpread
			fragRand = template.FragmentRandom
			fragAngle = template.FragmentAngle
			fragVelMin = template.FragmentVelocityMin
			fragVelMax = template.FragmentVelocityMax
			fragLifeMin = template.FragmentLifeMin
			fragLifeMax = template.FragmentLifeMax
			fragBullet = cloneBulletRuntimeProfile(template.FragmentBullet)
		}
		speedMul := randomRange(parent.FragmentVelMin, parent.FragmentVelMax)
		if speedMul == 0 {
			speedMul = 1
		}
		lifeMul := randomRange(parent.FragmentLifeMin, parent.FragmentLifeMax)
		if lifeMul == 0 {
			lifeMul = 1
		}
		b := simBullet{
			ID:                 w.bulletNextID,
			Team:               parent.Team,
			X:                  parent.X,
			Y:                  parent.Y,
			VX:                 float32(math.Cos(float64(rad))) * speed * speedMul,
			VY:                 float32(math.Sin(float64(rad))) * speed * speedMul,
			Damage:             damage,
			SplashDamage:       splashDamage,
			LifeSec:            maxf(life*lifeMul, 0.2),
			Radius:             radius,
			HitUnits:           parent.HitUnits,
			HitBuilds:          hitBuilds,
			BulletType:         bulletType,
			BulletClass:        bulletClass,
			SplashRadius:       splashRadius,
			BuildingDamage:     buildingDamage,
			ArmorMultiplier:    armorMultiplier,
			MaxDamageFraction:  maxDamageFraction,
			ShieldDamageMul:    shieldDamageMul,
			PierceDamageFactor: pierceDamageFactor,
			PierceArmor:        pierceArmor,
			SlowSec:            parent.SlowSec,
			SlowMul:            parent.SlowMul,
			PierceRemain:       pierce,
			PierceBuilding:     pierceBuilding,
			ChainCount:         0,
			ChainRange:         0,
			FragmentCount:      fragCount,
			FragmentSpread:     fragSpread2,
			FragmentRand:       fragRand,
			FragmentAngle:      fragAngle,
			FragmentVelMin:     fragVelMin,
			FragmentVelMax:     fragVelMax,
			FragmentLifeMin:    fragLifeMin,
			FragmentLifeMax:    fragLifeMax,
			FragmentBullet:     fragBullet,
			StatusID:           statusID,
			StatusName:         statusName,
			StatusDuration:     statusDuration,
			ShootEffect:        "",
			SmokeEffect:        "",
			HitEffect:          hitEffect,
			DespawnEffect:      despawnEffect,
			TargetAir:          targetAir,
			TargetGround:       targetGround,
			TargetPriority:     parent.TargetPriority,
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
	if b.SplashRadius <= 0 || (b.SplashDamage <= 0 && b.StatusID == 0 && strings.TrimSpace(b.StatusName) == "") {
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
		dist := float32(math.Sqrt(float64(d2)))
		scale := 1 - 0.6*(dist/b.SplashRadius)
		if scale < 0.4 {
			scale = 0.4
		}
		if b.SplashDamage > 0 {
			if remaining, absorbed := w.absorbEntityAbilityDamage(e, b.X, b.Y, b.SplashDamage*scale); !absorbed {
				w.applyDamageToEntityProfile(e, remaining, bulletDamageApplyProfile(b))
			}
		}
		applySlow(e, b.SlowSec*scale, b.SlowMul)
		w.applyStatusToEntity(e, b.StatusID, b.StatusName, b.StatusDuration)
	}
	// Damage enemy buildings in splash radius.
	w.forEachEnemyBuildingInRange(b.Team, b.X, b.Y, b.SplashRadius, func(pos int32) {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			return
		}
		t := &w.model.Tiles[pos]
		if t.Build == nil || t.Build.Health <= 0 {
			return
		}
		px := float32(t.X*8 + 4)
		py := float32(t.Y*8 + 4)
		dx := px - b.X
		dy := py - b.Y
		d2 := dx*dx + dy*dy
		if d2 > b.SplashRadius*b.SplashRadius {
			return
		}
		dist := float32(math.Sqrt(float64(d2)))
		scale := 1 - 0.6*(dist/b.SplashRadius)
		if scale < 0.4 {
			scale = 0.4
		}
		if b.SplashDamage > 0 {
			_ = w.applyDamageToBuildingDetailed(pos, b.SplashDamage*scale*b.BuildingDamage)
		}
	})
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
		target := &w.model.Entities[next]
		if remaining, absorbed := w.absorbEntityAbilityDamage(target, px, py, damage); !absorbed {
			w.applyDamageToEntityProfile(target, remaining, bulletDamageApplyProfile(b))
			applySlow(target, b.SlowSec*scale, b.SlowMul)
			w.applyStatusToEntity(target, b.StatusID, b.StatusName, b.StatusDuration)
		}
		hit[next] = struct{}{}
		prev = next
	}
}

func (w *World) applyBeamChainFromSource(src RawEntity, firstIdx int, sourceIsBuilding bool) {
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
		dmg := src.AttackDamage * scale * w.outgoingDamageScale(src, sourceIsBuilding)
		target := &w.model.Entities[next]
		if remaining, absorbed := w.absorbEntityAbilityDamage(target, px, py, dmg); !absorbed {
			w.applyDamageToEntityProfile(target, remaining, attackDamageApplyProfile(src))
			applySlow(target, src.AttackSlowSec*scale, src.AttackSlowMul)
			w.applyStatusToEntity(target, src.AttackStatusID, src.AttackStatusName, src.AttackStatusDuration)
		}
		hit[next] = struct{}{}
		prev = next
	}
}

func (w *World) applyDamageToEntity(e *RawEntity, dmg float32) {
	w.applyDamageToEntityDetailed(e, dmg, false)
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
	visitPos := func(pos int32) {
		if pos < 0 || int(pos) >= len(w.model.Tiles) {
			return
		}
		t := &w.model.Tiles[pos]
		if t.Build == nil || t.Build.Health <= 0 {
			return
		}
		if t.Build.Team == src.Team {
			return
		}
		tx := float32(t.X*8 + 4)
		ty := float32(t.Y*8 + 4)
		dx := tx - src.X
		dy := ty - src.Y
		d2 := dx*dx + dy*dy
		if d2 > bestDist2 {
			return
		}
		bestDist2 = d2
		bestPos = pos
		bestX = tx
		bestY = ty
		found = true
	}
	w.forEachEnemyBuildingInRange(src.Team, src.X, src.Y, rangeLimit, visitPos)
	if !found {
		return 0, 0, 0, false
	}
	return bestPos, bestX, bestY, true
}

func (w *World) applyDamageToBuilding(pos int32, damage float32) bool {
	return w.applyDamageToBuildingDetailed(pos, damage)
}

func (w *World) applyDamageToBuildingRaw(pos int32, damage float32) bool {
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
	powerRelevant := w.isPowerRelevantBuildingLocked(t)
	w.queueBrokenBuildPlanLocked(pos, t)
	w.removeActiveTileIndexLocked(pos, t)
	w.setBuildingOccupancyLocked(pos, t, false)
	t.Build = nil
	t.Block = 0
	delete(w.buildStates, pos)
	w.clearBuildingRuntimeLocked(pos)
	if powerRelevant {
		w.invalidatePowerNetsLocked()
	}
	w.refreshCoreStorageLinksLocked()
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventBuildDestroyed,
		BuildPos:   packTilePos(x, y),
		BuildTeam:  team,
		BuildBlock: prevBlock,
	})
	return true
}

func (w *World) blockSyncSuppressedLocked(pos int32) bool {
	if pos < 0 {
		return true
	}
	if _, ok := w.pendingBreaks[pos]; ok {
		return true
	}
	return false
}

func (w *World) acquireTrackedEntityTarget(
	src RawEntity,
	ents []RawEntity,
	idToIndex map[int32]int,
	spatial *entitySpatialIndex,
	teamSpatial map[TeamID]*entitySpatialIndex,
	rangeLimit float32,
	allowAir, allowGround bool,
	priority string,
	state *targetTrackState,
	dt float32,
	retargetDelay float32,
) (int32, bool) {
	if state == nil {
		return findNearestEnemyEntity(src, ents, spatial, teamSpatial, rangeLimit, allowAir, allowGround, priority)
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
	tid, ok := findNearestEnemyEntity(src, ents, spatial, teamSpatial, rangeLimit, allowAir, allowGround, priority)
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

func isPlayerControlledEntity(e RawEntity) bool {
	return e.PlayerID != 0
}

func targetStillValid(src RawEntity, target RawEntity, rangeLimit float32, allowAir, allowGround bool) bool {
	if src.Team == 0 || target.Health <= 0 || target.Team == 0 || target.Team == src.Team {
		return false
	}
	if !canTargetEntity(target, allowAir, allowGround) {
		return false
	}
	dx := target.X - src.X
	dy := target.Y - src.Y
	return dx*dx+dy*dy <= rangeLimit*rangeLimit
}

func findNearestEnemyEntity(src RawEntity, ents []RawEntity, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex, rangeLimit float32, allowAir, allowGround bool, priority string) (int32, bool) {
	if src.Team == 0 {
		return 0, false
	}
	if !allowAir && !allowGround {
		allowAir, allowGround = true, true
	}
	bestDist2 := rangeLimit * rangeLimit
	bestID := int32(0)
	bestScore := float32(math.MaxFloat32)
	bestPriority := float32(-99999)
	visit := func(i int) {
		if i < 0 || i >= len(ents) {
			return
		}
		e := ents[i]
		if e.ID == src.ID || e.Health <= 0 {
			return
		}
		if e.Team == 0 || e.Team == src.Team {
			return
		}
		if !canTargetEntity(e, allowAir, allowGround) {
			return
		}
		dx := e.X - src.X
		dy := e.Y - src.Y
		d2 := dx*dx + dy*dy
		if d2 > bestDist2 {
			return
		}
		score := targetPriorityScore(src, e, d2, priority)
		targetPriority := entityTargetPriorityValue(e)
		if bestID == 0 || targetPriority > bestPriority || (targetPriority >= bestPriority && score < bestScore) {
			bestPriority = targetPriority
			bestScore = score
			bestDist2 = d2
			bestID = e.ID
		}
	}
	if len(teamSpatial) != 0 {
		for team, idx := range teamSpatial {
			if team == 0 || team == src.Team || idx == nil {
				continue
			}
			idx.forEachInRange(src.X, src.Y, rangeLimit, visit)
		}
	} else if spatial != nil {
		spatial.forEachInRange(src.X, src.Y, rangeLimit, visit)
	} else {
		for i := range ents {
			visit(i)
		}
	}
	return bestID, bestID != 0
}

func buildEntitySpatialIndex(ents []RawEntity) *entitySpatialIndex {
	if len(ents) == 0 {
		return nil
	}
	const entitySpatialCellSize = 64
	idx := &entitySpatialIndex{
		cellSize: entitySpatialCellSize,
		cells:    make(map[int64][]int, len(ents)),
	}
	for i := range ents {
		cx := int(math.Floor(float64(ents[i].X) / float64(entitySpatialCellSize)))
		cy := int(math.Floor(float64(ents[i].Y) / float64(entitySpatialCellSize)))
		key := packSpatialCell(cx, cy)
		idx.cells[key] = append(idx.cells[key], i)
	}
	return idx
}

func buildTeamEntitySpatialIndexes(ents []RawEntity) map[TeamID]*entitySpatialIndex {
	if len(ents) == 0 {
		return nil
	}
	const entitySpatialCellSize = 64
	out := make(map[TeamID]*entitySpatialIndex)
	for i := range ents {
		team := ents[i].Team
		if team == 0 {
			continue
		}
		idx := out[team]
		if idx == nil {
			idx = &entitySpatialIndex{
				cellSize: entitySpatialCellSize,
				cells:    map[int64][]int{},
			}
			out[team] = idx
		}
		cx := int(math.Floor(float64(ents[i].X) / float64(entitySpatialCellSize)))
		cy := int(math.Floor(float64(ents[i].Y) / float64(entitySpatialCellSize)))
		key := packSpatialCell(cx, cy)
		idx.cells[key] = append(idx.cells[key], i)
	}
	return out
}

func (w *World) forEachEnemyBuildingInRange(team TeamID, x, y, radius float32, visit func(pos int32)) {
	if w == nil || w.model == nil || team == 0 || radius < 0 || visit == nil {
		return
	}
	if len(w.teamBuildingSpatial) != 0 {
		for otherTeam, idx := range w.teamBuildingSpatial {
			if otherTeam == 0 || otherTeam == team || idx == nil {
				continue
			}
			idx.forEachInRange(x, y, radius, visit)
		}
		return
	}
	if len(w.teamBuildingTiles) != 0 {
		for otherTeam, positions := range w.teamBuildingTiles {
			if otherTeam == 0 || otherTeam == team {
				continue
			}
			for _, pos := range positions {
				visit(pos)
			}
		}
		return
	}
	rangeTiles := int(math.Ceil(float64(radius/8))) + 1
	centerX := int(x / 8)
	centerY := int(y / 8)
	minX := max(0, centerX-rangeTiles)
	maxX := min(w.model.Width-1, centerX+rangeTiles)
	minY := max(0, centerY-rangeTiles)
	maxY := min(w.model.Height-1, centerY+rangeTiles)
	for ty := minY; ty <= maxY; ty++ {
		row := ty * w.model.Width
		for tx := minX; tx <= maxX; tx++ {
			pos := int32(row + tx)
			tile := &w.model.Tiles[pos]
			if tile.Build == nil || tile.Block == 0 || tile.Build.Team == 0 || tile.Build.Team == team {
				continue
			}
			visit(pos)
		}
	}
}

func (idx *buildingSpatialIndex) insert(tileX, tileY int, pos int32) {
	if idx == nil || idx.cellSize <= 0 {
		return
	}
	cx := tileX * 8 / idx.cellSize
	cy := tileY * 8 / idx.cellSize
	key := packSpatialCell(cx, cy)
	idx.cells[key] = append(idx.cells[key], pos)
}

func (idx *buildingSpatialIndex) remove(tileX, tileY int, pos int32) {
	if idx == nil || idx.cellSize <= 0 {
		return
	}
	cx := tileX * 8 / idx.cellSize
	cy := tileY * 8 / idx.cellSize
	key := packSpatialCell(cx, cy)
	if cell, ok := idx.cells[key]; ok {
		for i, p := range cell {
			if p == pos {
				idx.cells[key] = append(cell[:i], cell[i+1:]...)
				break
			}
		}
	}
}

func packSpatialCell(x, y int) int64 {
	return (int64(int32(x)) << 32) | int64(uint32(y))
}

func (idx *buildingSpatialIndex) forEachInRange(x, y, radius float32, visit func(pos int32)) {
	if idx == nil || idx.cellSize <= 0 || visit == nil {
		return
	}
	cell := float32(idx.cellSize)
	minCX := int(math.Floor(float64((x - radius) / cell)))
	maxCX := int(math.Floor(float64((x + radius) / cell)))
	minCY := int(math.Floor(float64((y - radius) / cell)))
	maxCY := int(math.Floor(float64((y + radius) / cell)))
	for cy := minCY; cy <= maxCY; cy++ {
		for cx := minCX; cx <= maxCX; cx++ {
			for _, pos := range idx.cells[packSpatialCell(cx, cy)] {
				visit(pos)
			}
		}
	}
}

func (idx *entitySpatialIndex) forEachInRange(x, y, radius float32, visit func(i int)) {
	if idx == nil || idx.cellSize <= 0 || visit == nil {
		return
	}
	cell := float32(idx.cellSize)
	minCX := int(math.Floor(float64((x - radius) / cell)))
	maxCX := int(math.Floor(float64((x + radius) / cell)))
	minCY := int(math.Floor(float64((y - radius) / cell)))
	maxCY := int(math.Floor(float64((y + radius) / cell)))
	for cy := minCY; cy <= maxCY; cy++ {
		for cx := minCX; cx <= maxCX; cx++ {
			for _, i := range idx.cells[packSpatialCell(cx, cy)] {
				visit(i)
			}
		}
	}
}

func findHitEnemyEntityIndex(b simBullet, ents []RawEntity, spatial *entitySpatialIndex, teamSpatial map[TeamID]*entitySpatialIndex, radius float32, allowAir, allowGround bool) (int, bool) {
	if !allowAir && !allowGround {
		allowAir, allowGround = true, true
	}
	bestDist2 := float32(math.MaxFloat32)
	bestIdx := -1
	visit := func(i int) {
		if i < 0 || i >= len(ents) {
			return
		}
		e := ents[i]
		if e.Health <= 0 || e.Team == b.Team {
			return
		}
		if !canTargetEntity(e, allowAir, allowGround) {
			return
		}
		dx := e.X - b.X
		dy := e.Y - b.Y
		d2 := dx*dx + dy*dy
		hitR := radius + maxf(e.HitRadius, 1.0)
		if d2 > hitR*hitR {
			return
		}
		if d2 >= bestDist2 {
			return
		}
		bestDist2 = d2
		bestIdx = i
	}
	if len(teamSpatial) != 0 {
		for team, idx := range teamSpatial {
			if team == 0 || team == b.Team || idx == nil {
				continue
			}
			idx.forEachInRange(b.X, b.Y, radius+16, visit)
		}
	} else if spatial != nil {
		spatial.forEachInRange(b.X, b.Y, radius+16, visit)
	} else {
		for i := range ents {
			visit(i)
		}
	}
	return bestIdx, bestIdx >= 0
}

func targetPriorityScore(src RawEntity, e RawEntity, d2 float32, priority string) float32 {
	_ = src
	base := d2 - e.HitRadius*e.HitRadius
	if base < 0 {
		base = 0
	}
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
		return base
	}
}

func entityTargetPriorityValue(e RawEntity) float32 {
	// Vanilla UnitType.targetPriority defaults to 0; missile-style units are below that.
	if e.LifeSec > 0 && e.AgeSec >= 0 {
		return -1
	}
	return 0
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
	if e.Flying {
		return true
	}
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
	if e.AttackBulletHitSize <= 0 {
		e.AttackBulletHitSize = 10
	}
	if !e.AttackBuildingDamageSet && e.AttackBuildingDamage != 0 {
		e.AttackBuildingDamageSet = true
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
	if e.StatusDamageMul <= 0 {
		e.StatusDamageMul = 1
	}
	if e.StatusHealthMul <= 0 {
		e.StatusHealthMul = 1
	}
	if e.StatusSpeedMul <= 0 {
		e.StatusSpeedMul = 1
	}
	if e.StatusReloadMul <= 0 {
		e.StatusReloadMul = 1
	}
	if e.StatusBuildSpeedMul <= 0 {
		e.StatusBuildSpeedMul = 1
	}
	if e.StatusDragMul <= 0 {
		e.StatusDragMul = 1
	}
	if e.StatusArmorOverride < 0 {
		e.StatusArmorOverride = -1
	}
	if e.HitRadius <= 0 {
		e.HitRadius = entityHitRadiusForType(e.TypeID)
	}
	if strings.TrimSpace(e.AttackFireMode) == "" {
		e.AttackFireMode = "projectile"
	}
	if e.ShieldMax < 0 {
		e.ShieldMax = 0
	}
	if e.Shield < 0 {
		e.Shield = 0
	}
	if e.ShieldRegen < 0 {
		e.ShieldRegen = 0
	}
	if e.Armor < 0 {
		e.Armor = 0
	}
	if prof, ok := w.unitRuntimeProfileForEntityLocked(*e); ok {
		w.applyUnitRuntimeProfile(e, prof)
	}
	w.applyWeaponProfile(e)
	e.RuntimeInit = true
}

func (w *World) applyWeaponProfile(e *RawEntity) {
	if e == nil {
		return
	}
	p := defaultWeaponProfile
	if name, ok := w.unitNamesByID[e.TypeID]; ok && name != "" {
		if byName, exists := w.unitProfilesByName[name]; exists {
			p = byName
			applyWeaponProfileToEntity(e, p)
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
	if p != defaultWeaponProfile || len(src) > 0 {
		applyWeaponProfileToEntity(e, p)
	} else {
		w.applyWeaponFromUnitTypeDef(e)
	}
	if e.HitRadius <= 0 {
		e.HitRadius = entityHitRadiusForType(e.TypeID)
	}
}

func (w *World) applyUnitTypeDef(e *RawEntity) {
	if e == nil {
		return
	}
	if prof, ok := w.unitRuntimeProfileForEntityLocked(*e); ok {
		w.applyUnitRuntimeProfile(e, prof)
	}
	def, ok := vanilla.UnitTypeDef{}, false
	if w.unitTypeDefsByID != nil {
		def, ok = w.unitTypeDefsByID[e.TypeID]
	}
	if name := strings.TrimSpace(w.unitNamesByID[e.TypeID]); name != "" {
		if fallback, fallbackOK := fallbackCoreUnitTypeDef(name); fallbackOK {
			if !ok || def.Health <= 0 {
				def.Health = fallback.Health
			}
			if !ok || def.Armor <= 0 {
				def.Armor = fallback.Armor
			}
			if !ok || def.HitSize <= 0 {
				def.HitSize = fallback.HitSize
			}
			if !ok || def.Speed <= 0 {
				def.Speed = fallback.Speed
			}
			if !ok || def.RotateSpeed <= 0 {
				def.RotateSpeed = fallback.RotateSpeed
			}
			ok = true
		}
	} else if fallbackName := fallbackUnitNameByTypeID(e.TypeID); fallbackName != "" {
		if fallback, fallbackOK := fallbackCoreUnitTypeDef(fallbackName); fallbackOK {
			if !ok || def.Health <= 0 {
				def.Health = fallback.Health
			}
			if !ok || def.Armor <= 0 {
				def.Armor = fallback.Armor
			}
			if !ok || def.HitSize <= 0 {
				def.HitSize = fallback.HitSize
			}
			if !ok || def.Speed <= 0 {
				def.Speed = fallback.Speed
			}
			if !ok || def.RotateSpeed <= 0 {
				def.RotateSpeed = fallback.RotateSpeed
			}
			ok = true
		}
	}
	if !ok {
		return
	}
	if def.Health > 0 {
		if e.RuntimeInit {
			e.Health = def.Health
			e.MaxHealth = def.Health
		} else {
			if e.MaxHealth <= 0 {
				e.MaxHealth = def.Health
			}
			if e.Health <= 0 {
				e.Health = minf(e.MaxHealth, def.Health)
			}
		}
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
	e.AttackBulletHitSize = 10
	e.AttackSplashRadius = def.Weapon.SplashRadius
	e.AttackBuildingDamage = 1
	e.AttackBuildingDamageSet = true
	e.AttackPierce = def.Weapon.Pierce
	e.AttackShootEffect = ""
	e.AttackSmokeEffect = ""
	e.AttackHitEffect = ""
	e.AttackDespawnEffect = ""
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

func normalizeEffectName(name string) string {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" || name == "none" {
		return ""
	}
	return name
}

func (w *World) emitEffectLocked(name string, x, y, rotation float32) {
	name = normalizeEffectName(name)
	if w == nil || name == "" {
		return
	}
	w.entityEvents = append(w.entityEvents, EntityEvent{
		Kind:       EntityEventEffect,
		EffectName: name,
		EffectX:    x,
		EffectY:    y,
		EffectRot:  rotation,
	})
}

func (w *World) emitAttackFireEffectsLocked(src RawEntity) {
	w.emitEffectLocked(src.AttackShootEffect, src.X, src.Y, src.Rotation)
	w.emitEffectLocked(src.AttackSmokeEffect, src.X, src.Y, src.Rotation)
}

func (w *World) emitAttackHitEffectLocked(src RawEntity, x, y float32) {
	w.emitEffectLocked(src.AttackHitEffect, x, y, src.Rotation)
}

func (w *World) emitAttackDespawnEffectLocked(src RawEntity, x, y float32) {
	w.emitEffectLocked(src.AttackDespawnEffect, x, y, src.Rotation)
}

func applyBehaviorMotion(e *RawEntity, ents []RawEntity, idToIndex map[int32]int) {
	speed := e.MoveSpeed
	if speed <= 0 {
		speed = 18
	}
	speed *= entitySpeedMultiplier(*e)
	switch e.Behavior {
	case "move":
		if reachedTarget(e.X, e.Y, e.PatrolAX, e.PatrolAY, 1.25) {
			e.Behavior = ""
			e.VelX, e.VelY = 0, 0
			return
		}
		setVelocityToTarget(e, e.PatrolAX, e.PatrolAY, speed, 1.25)
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

	_, waveTeam := w.teamsFromRulesLocked()

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

			posX, posY, ok := w.pickWaveSpawnPositionLocked(enemyType, waveTeam)
			if !ok {
				posX = float32(w.model.Width*8) / 2
				posY = float32(w.model.Height*8) / 2
			}
			posX += float32((unitIdx%3)-1) * 8
			posY += float32((group%3)-1) * 8
			posX = clampf(posX, 0, float32(w.model.Width*8))
			posY = clampf(posY, 0, float32(w.model.Height*8))
			w.addEnemy(enemyType, posX, posY)
		}

	}
}

// addEnemy 添加敌方单位
func (w *World) addEnemy(unitType int16, x, y float32) {
	if w.model == nil {
		return
	}

	unit := RawEntity{
		TypeID:       unitType,
		X:            x,
		Y:            y,
		Team:         2, // default fallback, rules may override below
		Health:       100,
		MaxHealth:    100,
		AttackDamage: 10,
		SlowMul:      1,
		Rotation:     0,
		RuntimeInit:  true,
		MineTilePos:  invalidEntityTilePos,
	}
	if _, waveTeam := w.teamsFromRulesLocked(); waveTeam != 0 {
		unit.Team = waveTeam
	}
	w.applyUnitTypeDef(&unit)
	w.applyWeaponProfile(&unit)
	if isEntityFlying(unit) {
		unit.Elevation = 1
	}
	w.model.AddEntity(unit)
}
