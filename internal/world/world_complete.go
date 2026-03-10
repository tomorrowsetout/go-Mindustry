package world

import (
	"math"
	"math/rand"
	"strings"
	"time"
)

// 核心方块 ID - 使用内容注册表中的实际 ID
const (
	BlockCoreShard      BlockID = 316 // core-shard - 最小核心
	BlockCoreFoundation BlockID = 317 // core-foundation - 中等核心
	BlockCoreNucleus    BlockID = 318 // core-nucleus - 最大核心
	BlockCoreBastion    BlockID = 319 // core-bastion - 艾里克尔最小核心
	BlockCoreCitadel    BlockID = 320 // core-citadel - 艾里克尔中等核心
	BlockCoreAcropolis  BlockID = 321 // core-acropolis - 艾里克尔最大核心
)

// BlockCore 是旧的向后兼容常量，已弃用
// 请使用具体的核心常量（如 BlockCoreShard, BlockCoreFoundation 等）
const BlockCore BlockID = BlockCoreShard

// RaycastResult 射线投射结果
type RaycastResult struct {
	Hit      bool
	Pos      Point2
	Normal   Vec2
	Building *Building
	Tile     *Tile
	Distance float32
	Blocked  bool
	Team     TeamID
}

// Point2 点2D
type Point2 struct {
	X int32
	Y int32
}

// Raycast 射线投射（完整实现）
func (w *World) Raycast(startX, startY, endX, endY float32, team TeamID, rangeVal float32) *RaycastResult {
	if w == nil || w.model == nil || w.model.Width <= 0 || w.model.Height <= 0 {
		return &RaycastResult{Hit: false, Pos: Point2{X: int32(endX), Y: int32(endY)}}
	}

	dx := endX - startX
	dy := endY - startY
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist == 0 {
		return &RaycastResult{Hit: false, Pos: Point2{X: int32(startX), Y: int32(startY)}}
	}
	if rangeVal > 0 && dist > rangeVal {
		scale := rangeVal / dist
		endX = startX + dx*scale
		endY = startY + dy*scale
		dist = rangeVal
		dx = endX - startX
		dy = endY - startY
	}

	hit, px, py, build, tile, blocked := w.raycastTiles(startX, startY, endX, endY, team)
	if hit {
		return &RaycastResult{
			Hit:      true,
			Pos:      Point2{X: int32(px), Y: int32(py)},
			Building: build,
			Tile:     tile,
			Distance: dist,
			Blocked:  blocked,
			Team:     team,
		}
	}
	return &RaycastResult{
		Hit:      false,
		Pos:      Point2{X: int32(endX), Y: int32(endY)},
		Distance: dist,
	}
}

// RaycastBlock 射线投射（块级别）
func (w *World) RaycastBlock(startX, startY, endX, endY float32, rangeVal float32) *RaycastResult {
	if w == nil || w.model == nil {
		return &RaycastResult{Hit: false, Pos: Point2{X: int32(endX), Y: int32(endY)}}
	}
	team := TeamID(0)
	dx := endX - startX
	dy := endY - startY
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist == 0 {
		return &RaycastResult{Hit: false, Pos: Point2{X: int32(startX), Y: int32(startY)}}
	}
	if rangeVal > 0 && dist > rangeVal {
		scale := rangeVal / dist
		endX = startX + dx*scale
		endY = startY + dy*scale
		dist = rangeVal
	}
	hit, px, py, build, tile, blocked := w.raycastTiles(startX, startY, endX, endY, team)
	if hit {
		return &RaycastResult{
			Hit:      true,
			Pos:      Point2{X: int32(px), Y: int32(py)},
			Building: build,
			Tile:     tile,
			Distance: dist,
			Blocked:  blocked,
			Team:     team,
		}
	}
	return &RaycastResult{Hit: false, Pos: Point2{X: int32(endX), Y: int32(endY)}, Distance: dist}
}

// LineBlock 直线块
func (w *World) LineBlock(x1, y1, x2, y2 float32, rangeVal float32) bool {
	if w == nil || w.model == nil {
		return false
	}
	res := w.RaycastBlock(x1, y1, x2, y2, rangeVal)
	return res != nil && res.Hit
}

// LineBuild 直线建造
func (w *World) LineBuild(x1, y1, x2, y2 float32, rangeVal float32) []Point2 {
	if w == nil || w.model == nil || w.model.Width <= 0 || w.model.Height <= 0 {
		return nil
	}

	dx := x2 - x1
	dy := y2 - y1
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist == 0 {
		tx, ty := w.worldToTile(x1, y1)
		if w.model.InBounds(tx, ty) {
			return []Point2{{X: int32(tx), Y: int32(ty)}}
		}
		return nil
	}
	if rangeVal > 0 && dist > rangeVal {
		scale := rangeVal / dist
		x2 = x1 + dx*scale
		y2 = y1 + dy*scale
	}

	return w.collectLineTiles(x1, y1, x2, y2)
}

// worldToTile converts world-space coordinates to tile indices.
func (w *World) worldToTile(x, y float32) (int, int) {
	return int(math.Round(float64(x / 8))), int(math.Round(float64(y / 8)))
}

func (w *World) tileHasSolid(t *Tile, team TeamID) (hit bool, blocked bool) {
	if t == nil {
		return false, false
	}
	if t.Build != nil && t.Build.Health > 0 {
		// Block if hostile or neutral to shooter team.
		if team == 0 || t.Build.Team != team {
			return true, true
		}
	}
	if t.Block != 0 {
		return true, true
	}
	return false, false
}

func (w *World) raycastTiles(startX, startY, endX, endY float32, team TeamID) (bool, int, int, *Building, *Tile, bool) {
	x1, y1 := w.worldToTile(startX, startY)
	x2, y2 := w.worldToTile(endX, endY)
	if w.model == nil {
		return false, x1, y1, nil, nil, false
	}

	hit := false
	var hx, hy int
	var hbuild *Building
	var htile *Tile
	var hblocked bool
	w.raycastEach(x1, y1, x2, y2, func(tx, ty int) bool {
		if !w.model.InBounds(tx, ty) {
			return true
		}
		t := &w.model.Tiles[ty*w.model.Width+tx]
		if ok, blocked := w.tileHasSolid(t, team); ok {
			hit = true
			hx, hy = tx, ty
			hbuild = t.Build
			htile = t
			hblocked = blocked
			return true
		}
		return false
	})
	return hit, hx, hy, hbuild, htile, hblocked
}

func (w *World) collectLineTiles(startX, startY, endX, endY float32) []Point2 {
	out := make([]Point2, 0, 64)
	x1, y1 := w.worldToTile(startX, startY)
	x2, y2 := w.worldToTile(endX, endY)
	w.raycastEach(x1, y1, x2, y2, func(tx, ty int) bool {
		if w.model.InBounds(tx, ty) {
			out = append(out, Point2{X: int32(tx), Y: int32(ty)})
		}
		return false
	})
	return out
}

func (w *World) raycastEach(x1, y1, x2, y2 int, accept func(x, y int) bool) {
	x := x1
	y := y1
	dx := int(math.Abs(float64(x2 - x)))
	dy := int(math.Abs(float64(y2 - y)))
	sx := -1
	if x < x2 {
		sx = 1
	}
	sy := -1
	if y < y2 {
		sy = 1
	}
	err := dx - dy

	for {
		if accept(x, y) {
			return
		}
		if x == x2 && y == y2 {
			return
		}
		e2 := err * 2
		if e2 > -dy {
			err -= dy
			x += sx
		}
		if e2 < dx {
			err += dx
			y += sy
		}
	}
}

// WorldTime 时间管理
type WorldTime struct {
	Tick           uint64
	WaveTime       float32
	TotalTime      float32
	TimeScale      float32
	DayTime        float32
	DayLength      float32
	WeatherTimer   float32
	Weather        *Weather
	WeatherEntries []WeatherEntry
	rand           *rand.Rand
}

// NewWorldTime 创建时间管理
func NewWorldTime() *WorldTime {
	return &WorldTime{
		TimeScale: 1.0,
		DayLength: 24000, // 24000 ticks = 1 day
		Weather:   &Weather{Type: WeatherClear},
		rand:      rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Update 更新时间
func (t *WorldTime) Update(dt float32) {
	t.Tick++
	t.TotalTime += dt * t.TimeScale
	t.WaveTime += dt * t.TimeScale

	// 更新昼夜
	t.DayTime = float32(t.Tick % uint64(t.DayLength))

	// 更新天气
	t.updateWeather(dt)
}

// updateWeather 更新天气
func (t *WorldTime) updateWeather(dt float32) {
	if t == nil {
		return
	}
	if t.rand == nil {
		t.rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	}
	if t.Weather != nil && !t.Weather.Completed {
		t.Weather.Update(dt)
	}
	if len(t.WeatherEntries) == 0 {
		if t.Weather == nil {
			t.Weather = &Weather{Type: WeatherClear, Completed: true}
		}
		return
	}
	for i := range t.WeatherEntries {
		entry := &t.WeatherEntries[i]
		if entry.MinDuration <= 0 && entry.MaxDuration <= 0 {
			entry.MinDuration = 60
			entry.MaxDuration = 180
		}
		if entry.MaxDuration < entry.MinDuration {
			entry.MaxDuration = entry.MinDuration
		}
		if entry.MaxFrequency < entry.MinFrequency {
			entry.MaxFrequency = entry.MinFrequency
		}
		if entry.Cooldown > 0 {
			entry.Cooldown -= dt
		}
	}
	if t.Weather != nil && !t.Weather.Completed {
		return
	}
	candidates := make([]*WeatherEntry, 0, len(t.WeatherEntries))
	for i := range t.WeatherEntries {
		entry := &t.WeatherEntries[i]
		if entry.Always || entry.Cooldown <= 0 {
			candidates = append(candidates, entry)
		}
	}
	if len(candidates) == 0 {
		return
	}
	entry := candidates[t.rand.Intn(len(candidates))]
	duration := entry.MinDuration
	if entry.Always {
		duration = float32(math.Inf(1))
	} else if entry.MaxDuration > entry.MinDuration {
		duration = entry.MinDuration + t.rand.Float32()*(entry.MaxDuration-entry.MinDuration)
	}
	nextFreq := entry.MinFrequency
	if entry.MaxFrequency > entry.MinFrequency {
		nextFreq = entry.MinFrequency + t.rand.Float32()*(entry.MaxFrequency-entry.MinFrequency)
	}
	entry.Cooldown = duration + nextFreq
	angle := t.rand.Float32() * 2 * math.Pi
	windX := float32(math.Cos(float64(angle)))
	windY := float32(math.Sin(float64(angle)))
	if entry.WindScale != 0 {
		windX *= entry.WindScale
		windY *= entry.WindScale
	}
	t.Weather = NewWeather(entry.Type, entry.Intensity, duration, windX, windY)
	t.Weather.ID = entry.ID
	t.Weather.Name = entry.Name
	t.Weather.StartTick = t.Tick
}

// Weather 天气
type Weather struct {
	Type      WeatherType
	ID        int16
	Name      string
	Intensity float32
	Duration  float32
	WindX     float32
	WindY     float32
	Completed bool
	StartTick uint64
}

// WeatherEntry 天气配置
type WeatherEntry struct {
	Type         WeatherType `json:"-"`
	Name         string      `json:"weather"`
	ID           int16       `json:"id,omitempty"`
	Intensity    float32     `json:"intensity"`
	MinFrequency float32     `json:"minFrequency"`
	MaxFrequency float32     `json:"maxFrequency"`
	MinDuration  float32     `json:"minDuration"`
	MaxDuration  float32     `json:"maxDuration"`
	Always       bool        `json:"always"`
	Cooldown     float32     `json:"cooldown"`
	WindScale    float32     `json:"windScale"`
}

// WeatherType 天气类型
type WeatherType byte

const (
	WeatherClear     WeatherType = iota // 晴天
	WeatherRain                         // 雨天
	WeatherSnow                         // 雪天
	WeatherSandstorm                    // 沙暴
	WeatherSlag                         // 泥浆
	WeatherFog                          // 雾
)

// NewWeather 创建天气
func NewWeather(wtype WeatherType, intensity, duration, windX, windY float32) *Weather {
	return &Weather{
		Type:      wtype,
		Intensity: intensity,
		Duration:  duration,
		WindX:     windX,
		WindY:     windY,
		Completed: false,
	}
}

// Update 更新天气
func (w *Weather) Update(dt float32) {
	w.Duration -= dt
	if w.Duration <= 0 {
		w.Completed = true
	}
}

// Biome 生物群系类型
type BiomeType byte

const (
	BiomeNormal     BiomeType = iota // 普通
	BiomeSnow                        // 雪地
	BiomeDesert                      // 沙漠
	BiomeWastes                      // 废土
	BiomeScorchers                   // 焦土
	BiomeSwamp                       // 沼泽
	BiomeWetlands                    // 湿地
	BiomeBamboo                      // 竹林
	BiomeForest                      // 森林
	BiomeAsh                         // 灰烬
	BiomeSalt                        // 盐碱地
	BiomeCrystal                     // 水晶
	BiomeBadlands                    // 杂地
	BiomeBoneDesert                  // 骨沙漠
	BiomeChangelog                   // 日志林
	BiomeSpores                      // 孢子林
	BiomeVolcanic                    // 火山
	BiomeBasalt                      // 玄武岩
	BiomeErosion                     // 侵蚀
	BiomeIcy                         // 冰封
)

// Biome 生物群系
type Biome struct {
	Type      BiomeType
	Name      string
	Temp      float32 // 温度
	Wetness   float32 // 湿度
	Fertility float32 // 肥沃度
	Shore     bool    // 是否海岸
	Surface   string  // 地表块
	Wall      string  // 墙块
	Sprout    string  // 生长块
	Walls     []string
	Sprouts   []string
	Items     []string
}

// NewBiome 创建生物群系
func NewBiome(btype BiomeType, name string) *Biome {
	return &Biome{
		Type: btype,
		Name: name,
	}
}

// WeatherFilter 天气过滤器
type WeatherFilter struct {
	Team    int32
	Weather *Weather
}

// Accept 接受
func (f *WeatherFilter) Accept(entity Entity) bool {
	if f == nil {
		return true
	}
	if f.Weather == nil || f.Weather.Completed {
		return false
	}
	if f.Team < 0 {
		return true
	}
	if unit, ok := entity.(*Unit); ok {
		return int32(unit.Team) == f.Team
	}
	if build, ok := entity.(*Building); ok {
		return int32(build.Team) == f.Team
	}
	return false
}

// TeamFilter 队伍过滤器
type TeamFilter struct {
	Team TeamID
}

// Accept 接受
func (f *TeamFilter) Accept(entity Entity) bool {
	if unit, ok := entity.(*Unit); ok {
		return unit.Team == f.Team
	}
	if build, ok := entity.(*Building); ok {
		return build.Team == f.Team
	}
	return false
}

// EnemyFilter 敌人过滤器
type EnemyFilter struct {
	Team TeamID
}

// Accept 接受
func (f *EnemyFilter) Accept(entity Entity) bool {
	if unit, ok := entity.(*Unit); ok {
		return unit.Team != f.Team
	}
	if build, ok := entity.(*Building); ok {
		return build.Team != f.Team
	}
	return false
}

// RangeFilter 范围过滤器
type RangeFilter struct {
	X, Y  float32
	Range float32
	Team  TeamID
}

// Accept 接受
func (f *RangeFilter) Accept(entity Entity) bool {
	if unit, ok := entity.(*Unit); ok {
		if unit.Team == f.Team {
			return false
		}
		dist := math.Sqrt(float64((unit.Pos.X-f.X)*(unit.Pos.X-f.X) + (unit.Pos.Y-f.Y)*(unit.Pos.Y-f.Y)))
		return float32(dist) <= f.Range
	}
	if build, ok := entity.(*Building); ok {
		if build.Team == f.Team {
			return false
		}
		dist := math.Sqrt(float64((float32(build.X)-f.X)*(float32(build.X)-f.X) + (float32(build.Y)-f.Y)*(float32(build.Y)-f.Y)))
		return float32(dist) <= f.Range
	}
	return false
}

// Darkness 黑暗度
type Darkness struct {
	Data   []float32
	Width  int32
	Height int32
}

// NewDarkness 创建黑暗度
func NewDarkness(width, height int32) *Darkness {
	return &Darkness{
		Data:   make([]float32, width*height),
		Width:  width,
		Height: height,
	}
}

// GetDarkness 获取黑暗度
func (d *Darkness) GetDarkness(x, y int32) float32 {
	if x < 0 || x >= d.Width || y < 0 || y >= d.Height {
		return 1.0
	}
	return d.Data[y*d.Width+x]
}

// SetDarkness 设置黑暗度
func (d *Darkness) SetDarkness(x, y int32, amount float32) {
	if x < 0 || x >= d.Width || y < 0 || y >= d.Height {
		return
	}
	d.Data[y*d.Width+x] = amount
}

// AddDarkness 添加黑暗度
func (d *Darkness) AddDarkness(x, y int32, amount float32) {
	if x < 0 || x >= d.Width || y < 0 || y >= d.Height {
		return
	}
	d.Data[y*d.Width+x] += amount
	if d.Data[y*d.Width+x] > 1.0 {
		d.Data[y*d.Width+x] = 1.0
	}
}

// ClearDarkness 清除黑暗度
func (d *Darkness) ClearDarkness() {
	for i := range d.Data {
		d.Data[i] = 0.0
	}
}

// Fog 阴影/迷雾
type Fog struct {
	Visible []bool
	Width   int32
	Height  int32
	Teams   map[TeamID]*TeamFog
}

// TeamFog 队伍迷雾
type TeamFog struct {
	Visible []bool
}

// NewFog 创建迷雾
func NewFog(width, height int32) *Fog {
	return &Fog{
		Visible: make([]bool, width*height),
		Teams:   make(map[TeamID]*TeamFog),
	}
}

// GetFog 获取迷雾
func (f *Fog) GetFog(x, y int32, team TeamID) bool {
	if teamFog, ok := f.Teams[team]; ok {
		return f.isVisible(x, y, teamFog.Visible)
	}
	return true
}

// SetFog 设置迷雾
func (f *Fog) SetFog(x, y int32, team TeamID, visible bool) {
	if teamFog, ok := f.Teams[team]; ok {
		f.setVisible(x, y, teamFog.Visible, visible)
	}
}

// isVisible 检查是否可见
func (f *Fog) isVisible(x, y int32, visible []bool) bool {
	idx := y*f.Width + x
	if idx >= 0 && idx < int32(len(visible)) {
		return visible[idx]
	}
	return true
}

// setVisible 设置可见
func (f *Fog) setVisible(x, y int32, visible []bool, state bool) {
	idx := y*f.Width + x
	if idx >= 0 && idx < int32(len(visible)) {
		visible[idx] = state
	}
}

// AddTeamFog 添加队伍迷雾
func (f *Fog) AddTeamFog(team TeamID) {
	if _, ok := f.Teams[team]; !ok {
		f.Teams[team] = &TeamFog{
			Visible: make([]bool, f.Width*f.Height),
		}
	}
}

// CoreBuild 核心建筑（完整实现）
type CoreBuild struct {
	Building
	Units        []*Unit
	LaunchCap    int32
	CanLaunch    bool
	CanSpeed     bool
	LaunchTime   float32
	MaxUnits     int32
	LoadProgress float32
	LaunchCapMod float32
	CanCapture   bool
}

// NewCoreBuild 创建新的核心建筑
func NewCoreBuild(x, y int32, team TeamID) *CoreBuild {
	return &CoreBuild{
		Building: Building{
			Block:     BlockCore,
			Team:      team,
			X:         int(x),
			Y:         int(y),
			Health:    750,
			MaxHealth: 750,
		},
		Units:        make([]*Unit, 0),
		LaunchCap:    15,
		CanLaunch:    true,
		CanSpeed:     true,
		LaunchCapMod: 1.0,
		CanCapture:   true,
	}
}

// AddUnit 添加单位
func (b *CoreBuild) AddUnit(unit *Unit) bool {
	if b.CanLaunch && int32(len(b.Units)) < b.LaunchCap {
		b.Units = append(b.Units, unit)
		return true
	}
	return false
}

// RemoveUnit 移除单位
func (b *CoreBuild) RemoveUnit(unit *Unit) {
	for i, u := range b.Units {
		if u == unit {
			b.Units = append(b.Units[:i], b.Units[i+1:]...)
			return
		}
	}
}

// Launch 启动单位
func (b *CoreBuild) Launch() *Unit {
	if len(b.Units) > 0 {
		unit := b.Units[0]
		b.Units = append(b.Units[:0], b.Units[1:]...)
		return unit
	}
	return nil
}

// GetUnitCount 获取单位数量
func (b *CoreBuild) GetUnitCount() int32 {
	return int32(len(b.Units))
}

// IsFull 是否已满
func (b *CoreBuild) IsFull() bool {
	return int32(len(b.Units)) >= b.LaunchCap
}

// CanSpawn 是否可以生成单位
func (b *CoreBuild) CanSpawn() bool {
	return b.CanLaunch && !b.IsFull()
}

// RequestSpawn 请求生成单位
func (b *CoreBuild) RequestSpawn(unitType int16, x, y float32) *Unit {
	if !b.CanSpawn() {
		return nil
	}

	// 创建新单位
	unit := &Unit{
		ID:        0,
		Team:      b.Team,
		Pos:       Vec2{X: x, Y: y},
		Health:    50,
		MaxHealth: 50,
		Type:      unitType,
	}

	if b.AddUnit(unit) {
		return unit
	}
	return nil
}

// Damage 受到伤害
func (b *CoreBuild) Damage(amount float32) {
	b.Health -= amount
	if b.Health <= 0 {
		b.Health = 0
	}
}

// Heal 治疗
func (b *CoreBuild) Heal(amount float32) {
	b.Health += amount
	if b.Health > b.MaxHealth {
		b.Health = b.MaxHealth
	}
}

// IsProtected 核心是否受保护
func (b *CoreBuild) IsProtected() bool {
	if b == nil {
		return false
	}
	w := DefaultWorld()
	if w == nil {
		return false
	}
	rules := w.rulesMgr.Get()
	if rules != nil && !rules.ProtectCores {
		return false
	}
	if rules != nil && rules.PolygonCoreProtection {
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
	w.mu.RLock()
	cores := w.collectCoreTilesLocked()
	w.mu.RUnlock()
	if len(cores) == 0 {
		return true
	}
	px := float32(b.X*8 + 4)
	py := float32(b.Y*8 + 4)
	for _, c := range cores {
		if c.Team == 0 || c.Team == b.Team {
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

// CanDrop 是否可以掉落
func (b *CoreBuild) CanDrop() bool {
	return b.CanLaunch
}

// CanSpeedUp 是否可以加速
func (b *CoreBuild) CanSpeedUp() bool {
	return b.CanSpeed
}

// BehaviorTile 属性Tile（完整实现）
type BehaviorTile struct {
	Tile
	Health        float32
	MaxHealth     float32
	Items         []ItemStack
	Liquids       []LiquidStack
	Payload       []byte
	Rotation      int8
	Config        []byte
	Flash         float32
	FlashTimer    float32
	Output        bool
	Team          TeamID
	Block         BlockID
	Breaking      bool
	BreakProgress float32
	world         *World
}

// NewBehaviorTile 创建新的属性Tile
func NewBehaviorTile(x, y int32, block BlockID, team TeamID) *BehaviorTile {
	return &BehaviorTile{
		Tile: Tile{
			X:     int(x),
			Y:     int(y),
			Block: block,
			Team:  team,
		},
		Health:     750,
		MaxHealth:  750,
		Items:      make([]ItemStack, 0),
		Liquids:    make([]LiquidStack, 0),
		Team:       team,
		Block:      block,
		Flash:      0,
		FlashTimer: 0,
		Output:     false,
		world:      DefaultWorld(),
	}
}

// Damage 受到伤害
func (b *BehaviorTile) Damage(amount float32) {
	b.Health -= amount
	if b.Health <= 0 {
		b.Health = 0
	}
}

// Heal 治疗
func (b *BehaviorTile) Heal(amount float32) {
	b.Health += amount
	if b.Health > b.MaxHealth {
		b.Health = b.MaxHealth
	}
}

// ConsumeItem 消耗物品
func (b *BehaviorTile) ConsumeItem(item ItemID, amount int32) bool {
	return b.RemoveItem(item, amount)
}

// ConsumeLiquids 消耗液体
func (b *BehaviorTile) ConsumeLiquids(liquid LiquidID, amount float32) bool {
	for i, stack := range b.Liquids {
		if stack.Liquid == liquid {
			if stack.Amount >= amount {
				b.Liquids[i].Amount -= amount
				if b.Liquids[i].Amount <= 0 {
					b.Liquids = append(b.Liquids[:i], b.Liquids[i+1:]...)
				}
				return true
			}
		}
	}
	return false
}

// ConsumePower 消耗电力
func (b *BehaviorTile) ConsumePower(amount float32) bool {
	if amount <= 0 {
		return true
	}
	w := b.worldRef()
	if w == nil || w.model == nil {
		return true
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	pos := int32(b.Tile.Y*w.model.Width + b.Tile.X)
	state, ok := w.buildStates[pos]
	if !ok {
		return false
	}
	if state.Power < amount {
		return false
	}
	state.Power -= amount
	if state.Power < 0 {
		state.Power = 0
	}
	w.buildStates[pos] = state
	return true
}

// ConsumeItems 消耗多个物品
func (b *BehaviorTile) ConsumeItems(items []ItemStack) bool {
	for _, item := range items {
		if !b.RemoveItem(item.Item, item.Amount) {
			return false
		}
	}
	return true
}

// ConsumeLiquidsMultiple 消耗多种液体
func (b *BehaviorTile) ConsumeLiquidsMultiple(liquids []LiquidStack) bool {
	for _, liquid := range liquids {
		if !b.ConsumeLiquids(liquid.Liquid, liquid.Amount) {
			return false
		}
	}
	return true
}

// HasItems 检查是否有物品
func (b *BehaviorTile) HasItems() bool {
	return len(b.Items) > 0
}

// HasLiquids 检查是否有液体
func (b *BehaviorTile) HasLiquids() bool {
	return len(b.Liquids) > 0
}

// GetTotalItems 获取总物品数
func (b *BehaviorTile) GetTotalItems() int32 {
	total := int32(0)
	for _, stack := range b.Items {
		total += stack.Amount
	}
	return total
}

// GetTotalLiquids 获取总液体数
func (b *BehaviorTile) GetTotalLiquids() float32 {
	total := float32(0)
	for _, stack := range b.Liquids {
		total += stack.Amount
	}
	return total
}

// GetItem 获取物品
func (b *BehaviorTile) GetItem(item ItemID) (int32, bool) {
	for _, stack := range b.Items {
		if stack.Item == item {
			return stack.Amount, true
		}
	}
	return 0, false
}

// GetLiquid 获取液体
func (b *BehaviorTile) GetLiquid(liquid LiquidID) (float32, bool) {
	for _, stack := range b.Liquids {
		if stack.Liquid == liquid {
			return stack.Amount, true
		}
	}
	return 0, false
}

// RemoveItem 移除物品
func (b *BehaviorTile) RemoveItem(item ItemID, amount int32) bool {
	for i, stack := range b.Items {
		if stack.Item == item {
			if stack.Amount >= amount {
				b.Items[i].Amount -= amount
				if b.Items[i].Amount <= 0 {
					b.Items = append(b.Items[:i], b.Items[i+1:]...)
				}
				return true
			}
		}
	}
	return false
}

// AddItem 添加物品
func (b *BehaviorTile) AddItem(item ItemID, amount int32) bool {
	for i, stack := range b.Items {
		if stack.Item == item {
			b.Items[i].Amount += amount
			return true
		}
	}
	b.Items = append(b.Items, ItemStack{Item: item, Amount: amount})
	return true
}

// HandlePayload 处理载荷
func (b *BehaviorTile) HandlePayload() {
	_ = b
}

// LoadPayload 装载载荷
func (b *BehaviorTile) LoadPayload(payload []byte) bool {
	if b == nil {
		return false
	}
	if len(payload) == 0 {
		return false
	}
	if b.Payload != nil {
		return false
	}
	b.Payload = append([]byte(nil), payload...)
	return true
}

// DumpPayload 卸载载荷
func (b *BehaviorTile) DumpPayload() []byte {
	if b == nil || b.Payload == nil {
		return nil
	}
	out := b.Payload
	b.Payload = nil
	return out
}

// IsLoaded 是否已装载
func (b *BehaviorTile) IsLoaded() bool {
	return b != nil && b.Payload != nil && len(b.Payload) > 0
}

// HasOutput 是否有输出
func (b *BehaviorTile) HasOutput() bool {
	return b.Output
}

// HasCapacity 是否有容量
func (b *BehaviorTile) HasCapacity() bool {
	if b == nil {
		return false
	}
	w := b.worldRef()
	if w == nil {
		return len(b.Items) < 10
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	capacity := int32(10)
	if w.blockNamesByID != nil {
		if name, ok := w.blockNamesByID[int16(b.Block)]; ok {
			if c, ok := buildingItemCapacityByName[strings.ToLower(strings.TrimSpace(name))]; ok {
				capacity = c
			}
		}
	}
	total := int32(0)
	for _, st := range b.Items {
		total += st.Amount
	}
	return total < capacity
}

// Consume 消耗
func (b *BehaviorTile) Consume() bool {
	// Simplified placeholder: treat as always consuming successfully.
	// Real logic is handled in World step loops.
	return true
}

// Produce 生产
func (b *BehaviorTile) Produce() bool {
	// Simplified placeholder: treat as producing successfully.
	// Real logic is handled in World step loops.
	return true
}

// CanProduce 是否可以生产
func (b *BehaviorTile) CanProduce() bool {
	return b != nil && b.HasCapacity()
}

// Update 更新
func (b *BehaviorTile) Update() {
	b.UpdateConsumers()
}

// UpdateConsumers 更新Consumers
func (b *BehaviorTile) UpdateConsumers() {
	_ = b
}

// Draw 绘制
func (b *BehaviorTile) Draw() {
	// TODO: 实现绘制逻辑
}

// DrawPlaced 绘制已放置
func (b *BehaviorTile) DrawPlaced() {
	// TODO: 实现已放置绘制逻辑
}

// DrawPlan 绘制计划
func (b *BehaviorTile) DrawPlan() {
	// TODO: 实现计划绘制逻辑
}

// DrawPlanOverlay 绘制计划叠加
func (b *BehaviorTile) DrawPlanOverlay() {
	// TODO: 实现计划叠加绘制逻辑
}

// DrawPlanOverlayPlan 绘制计划叠加计划
func (b *BehaviorTile) DrawPlanOverlayPlan() {
	// TODO: 实现计划叠加计划绘制逻辑
}

// DrawPlanOverlayPlanOverlay 绘制计划叠加计划叠加
func (b *BehaviorTile) DrawPlanOverlayPlanOverlay() {
	// TODO: 实现计划叠加计划叠加绘制逻辑
}

// GetHealth 获取健康度
func (b *BehaviorTile) GetHealth() float32 {
	return b.Health
}

// GetMaxHealth 获取最大健康度
func (b *BehaviorTile) GetMaxHealth() float32 {
	return b.MaxHealth
}

// GetTeam 获取队伍
func (b *BehaviorTile) GetTeam() TeamID {
	return b.Team
}

// GetBlock 获取块
func (b *BehaviorTile) GetBlock() BlockID {
	return b.Block
}

// GetPos 获取位置
func (b *BehaviorTile) GetPos() (int32, int32) {
	return int32(b.Tile.X), int32(b.Tile.Y)
}

// SetConfig 设置配置
func (b *BehaviorTile) SetConfig(config []byte) {
	b.Config = config
}

// GetConfig 获取配置
func (b *BehaviorTile) GetConfig() []byte {
	return b.Config
}

// SetRotation 设置旋转
func (b *BehaviorTile) SetRotation(rotation int8) {
	b.Rotation = rotation
}

// GetRotation 获取旋转
func (b *BehaviorTile) GetRotation() int8 {
	return b.Rotation
}

// IsDead 是否已死亡
func (b *BehaviorTile) IsDead() bool {
	return b.Health <= 0
}

// IsAlive 是否存活
func (b *BehaviorTile) IsAlive() bool {
	return b.Health > 0
}

// IsTeam 是否为队伍
func (b *BehaviorTile) IsTeam(team TeamID) bool {
	return b.Team == team
}

// IsEnemy 是否为敌人
func (b *BehaviorTile) IsEnemy(team TeamID) bool {
	return b.Team != team
}

// IsAlly 是否为盟友
func (b *BehaviorTile) IsAlly(team TeamID) bool {
	return b.Team == team
}

// DistanceTo 距离到
func (b *BehaviorTile) DistanceTo(target *BehaviorTile) float32 {
	dx := float32(b.Tile.X - target.Tile.X)
	dy := float32(b.Tile.Y - target.Tile.Y)
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// DistanceToPos 距离到位置
func (b *BehaviorTile) DistanceToPos(x, y int32) float32 {
	dx := float32(b.Tile.X - int(x))
	dy := float32(b.Tile.Y - int(y))
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

// InRange 是否在范围内
func (b *BehaviorTile) InRange(target *BehaviorTile, rangeVal float32) bool {
	return b.DistanceTo(target) <= rangeVal
}

// InRangePos 是否在位置范围内
func (b *BehaviorTile) InRangePos(x, y int32, rangeVal float32) bool {
	return b.DistanceToPos(x, y) <= rangeVal
}

// CanSee 是否可见
func (b *BehaviorTile) CanSee(target *BehaviorTile) bool {
	if b == nil || target == nil {
		return false
	}
	return b.CanSeePos(int32(target.Tile.X), int32(target.Tile.Y))
}

// CanSeePos 是否可见位置
func (b *BehaviorTile) CanSeePos(x, y int32) bool {
	if b == nil {
		return false
	}
	w := b.world
	if w == nil {
		w = DefaultWorld()
	}
	if w == nil {
		return true
	}
	startX := float32(b.Tile.X*8 + 4)
	startY := float32(b.Tile.Y*8 + 4)
	endX := float32(int(x)*8 + 4)
	endY := float32(int(y)*8 + 4)
	res := w.RaycastBlock(startX, startY, endX, endY, -1)
	return res == nil || !res.Hit
}

func (b *BehaviorTile) worldRef() *World {
	if b == nil {
		return nil
	}
	if b.world != nil {
		return b.world
	}
	return DefaultWorld()
}

func (b *BehaviorTile) worldModel() *WorldModel {
	w := b.worldRef()
	if w == nil {
		return nil
	}
	return w.model
}

func (b *BehaviorTile) worldCenter() (float32, float32) {
	if b == nil {
		return 0, 0
	}
	return float32(b.Tile.X*8 + 4), float32(b.Tile.Y*8 + 4)
}

func behaviorTileFromTile(t *Tile) *BehaviorTile {
	if t == nil {
		return nil
	}
	bt := &BehaviorTile{
		Tile: Tile{
			X:        t.X,
			Y:        t.Y,
			Floor:    t.Floor,
			Overlay:  t.Overlay,
			Block:    t.Block,
			Team:     t.Team,
			Rotation: t.Rotation,
			Con:      t.Con,
		},
		Team:  t.Team,
		Block: t.Block,
	}
	if t.Build != nil {
		bt.Health = t.Build.Health
		bt.MaxHealth = t.Build.MaxHealth
		bt.Team = t.Build.Team
		bt.Block = t.Build.Block
		bt.Rotation = t.Build.Rotation
		bt.Config = t.Build.Config
		bt.Items = append([]ItemStack(nil), t.Build.Items...)
		bt.Liquids = append([]LiquidStack(nil), t.Build.Liquids...)
	}
	return bt
}

func (b *BehaviorTile) findNearestBuild(filter func(t *Tile) bool, requireLOS bool) *BehaviorTile {
	model := b.worldModel()
	if model == nil {
		return nil
	}
	sx, sy := b.worldCenter()
	bestDist2 := float32(math.MaxFloat32)
	var best *Tile
	for i := range model.Tiles {
		t := &model.Tiles[i]
		if t.Build == nil || t.Build.Health <= 0 {
			continue
		}
		if filter != nil && !filter(t) {
			continue
		}
		if requireLOS && !b.CanSeePos(int32(t.X), int32(t.Y)) {
			continue
		}
		x := float32(t.X*8 + 4)
		y := float32(t.Y*8 + 4)
		dx := x - sx
		dy := y - sy
		d2 := dx*dx + dy*dy
		if d2 < bestDist2 {
			bestDist2 = d2
			best = t
		}
	}
	return behaviorTileFromTile(best)
}

func (b *BehaviorTile) findNearestUnit(filter func(e RawEntity) bool, requireLOS bool) *BehaviorTile {
	model := b.worldModel()
	if model == nil {
		return nil
	}
	sx, sy := b.worldCenter()
	bestDist2 := float32(math.MaxFloat32)
	var best *RawEntity
	for i := range model.Entities {
		e := &model.Entities[i]
		if e.Health <= 0 {
			continue
		}
		if filter != nil && !filter(*e) {
			continue
		}
		if requireLOS {
			tx := int32(e.X / 8)
			ty := int32(e.Y / 8)
			if !b.CanSeePos(tx, ty) {
				continue
			}
		}
		dx := e.X - sx
		dy := e.Y - sy
		d2 := dx*dx + dy*dy
		if d2 < bestDist2 {
			bestDist2 = d2
			best = e
		}
	}
	if best == nil {
		return nil
	}
	return &BehaviorTile{
		Tile: Tile{
			X: int(best.X / 8),
			Y: int(best.Y / 8),
		},
		Team:      best.Team,
		Health:    best.Health,
		MaxHealth: best.MaxHealth,
	}
}

// TargetTarget 目标目标
func (b *BehaviorTile) TargetTarget(team TeamID) *BehaviorTile {
	if b == nil {
		return nil
	}
	return b.findNearestBuild(func(t *Tile) bool {
		if t.Build == nil || t.Build.Health <= 0 {
			return false
		}
		return t.Build.Team != team
	}, false)
}

// TargetEnemy 目标敌人
func (b *BehaviorTile) TargetEnemy(team TeamID) *BehaviorTile {
	return b.TargetTarget(team)
}

// TargetFriendly 目标盟友
func (b *BehaviorTile) TargetFriendly(team TeamID) *BehaviorTile {
	if b == nil {
		return nil
	}
	return b.findNearestBuild(func(t *Tile) bool {
		if t.Build == nil || t.Build.Health <= 0 {
			return false
		}
		return t.Build.Team == team
	}, false)
}

// TargetBuilding 目标建筑
func (b *BehaviorTile) TargetBuilding() *BehaviorTile {
	if b == nil {
		return nil
	}
	team := b.Team
	return b.findNearestBuild(func(t *Tile) bool {
		if t.Build == nil || t.Build.Health <= 0 {
			return false
		}
		if team == 0 {
			return true
		}
		return t.Build.Team != team
	}, false)
}

// TargetUnit 目标单位
func (b *BehaviorTile) TargetUnit() *BehaviorTile {
	if b == nil {
		return nil
	}
	team := b.Team
	return b.findNearestUnit(func(e RawEntity) bool {
		if e.Health <= 0 {
			return false
		}
		if team == 0 {
			return true
		}
		return e.Team != team
	}, false)
}

// TargetCore 目标核心
func (b *BehaviorTile) TargetCore() *BehaviorTile {
	if b == nil {
		return nil
	}
	team := b.Team
	return b.findNearestBuild(func(t *Tile) bool {
		if t.Build == nil || t.Build.Health <= 0 {
			return false
		}
		if team != 0 && t.Build.Team == team {
			return false
		}
		switch t.Build.Block {
		case BlockCoreShard, BlockCoreFoundation, BlockCoreNucleus, BlockCoreBastion, BlockCoreCitadel, BlockCoreAcropolis:
			return true
		default:
			return false
		}
	}, false)
}

// TargetPlayer 目标玩家
func (b *BehaviorTile) TargetPlayer() *BehaviorTile {
	// Players are not tracked as explicit entities in current server model.
	return nil
}

// TargetAny 目标任意
func (b *BehaviorTile) TargetAny() *BehaviorTile {
	if b == nil {
		return nil
	}
	if t := b.TargetUnit(); t != nil {
		return t
	}
	return b.TargetBuilding()
}
