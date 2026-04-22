package world

import (
	"math"
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
	dx := endX - startX
	dy := endY - startY

	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist == 0 {
		return &RaycastResult{
			Hit: false,
			Pos: Point2{X: int32(startX), Y: int32(startY)},
		}
	}

	if rangeVal > 0 && dist > rangeVal {
		scale := rangeVal / dist
		endX = startX + dx*scale
		endY = startY + dy*scale
		dist = rangeVal
		dx = endX - startX
		dy = endY - startY
	}

	if w == nil {
		return &RaycastResult{Hit: false, Pos: Point2{X: int32(endX), Y: int32(endY)}, Distance: dist}
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.raycastLocked(startX, startY, endX, endY, team, false)
}

func (w *World) raycastLocked(startX, startY, endX, endY float32, team TeamID, ignoreEndTile bool) *RaycastResult {
	if w == nil || w.model == nil {
		return &RaycastResult{Hit: false, Pos: Point2{X: int32(endX), Y: int32(endY)}}
	}
	dx := endX - startX
	dy := endY - startY
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if dist <= 0 {
		return &RaycastResult{Hit: false, Pos: Point2{X: worldToTileCoord(startX), Y: worldToTileCoord(startY)}}
	}

	steps := int(math.Ceil(float64(dist / 2)))
	if steps < 1 {
		steps = 1
	}
	startTX, startTY := worldToTileCoord(startX), worldToTileCoord(startY)
	endTX, endTY := worldToTileCoord(endX), worldToTileCoord(endY)
	prevTX, prevTY := startTX, startTY
	for i := 1; i <= steps; i++ {
		t := float32(i) / float32(steps)
		x := startX + dx*t
		y := startY + dy*t
		tx, ty := worldToTileCoord(x), worldToTileCoord(y)
		if tx == startTX && ty == startTY {
			continue
		}
		if ignoreEndTile && tx == endTX && ty == endTY {
			continue
		}
		if !w.model.InBounds(int(tx), int(ty)) {
			return &RaycastResult{Hit: false, Pos: Point2{X: tx, Y: ty}, Distance: distance2D(startX, startY, x, y)}
		}
		tile := &w.model.Tiles[int(ty)*w.model.Width+int(tx)]
		if raycastTileBlocks(tile) {
			normal := raycastNormal(prevTX, prevTY, tx, ty)
			hitTeam := tile.Team
			var building *Building
			if tile.Build != nil {
				building = tile.Build
				if building.Team != 0 {
					hitTeam = building.Team
				}
			}
			return &RaycastResult{
				Hit:      true,
				Pos:      Point2{X: tx, Y: ty},
				Normal:   normal,
				Building: building,
				Tile:     tile,
				Distance: distance2D(startX, startY, x, y),
				Blocked:  true,
				Team:     hitTeam,
			}
		}
		prevTX, prevTY = tx, ty
		_ = team
	}
	return &RaycastResult{Hit: false, Pos: Point2{X: endTX, Y: endTY}, Distance: dist}
}

// RaycastBlock 射线投射（块级别）
func (w *World) RaycastBlock(startX, startY, endX, endY float32, rangeVal float32) *RaycastResult {
	return w.Raycast(startX, startY, endX, endY, 0, rangeVal)
}

// LineBlock 直线块
func (w *World) LineBlock(x1, y1, x2, y2 float32, rangeVal float32) bool {
	result := w.RaycastBlock(x1, y1, x2, y2, rangeVal)
	return result != nil && result.Hit
}

// LineBuild 直线建造
func (w *World) LineBuild(x1, y1, x2, y2 float32, rangeVal float32) []Point2 {
	points := lineTilesForWorldCoords(x1, y1, x2, y2, rangeVal)
	if w == nil || w.model == nil {
		return points
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	out := points[:0]
	for _, p := range points {
		if w.model.InBounds(int(p.X), int(p.Y)) {
			out = append(out, p)
		}
	}
	return out
}

func worldToTileCoord(v float32) int32 {
	return int32(math.Floor(float64(v / 8)))
}

func distance2D(x1, y1, x2, y2 float32) float32 {
	dx := x2 - x1
	dy := y2 - y1
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

func raycastTileBlocks(tile *Tile) bool {
	if tile == nil {
		return false
	}
	if tile.Build != nil && tile.Build.Health > 0 {
		return true
	}
	return tile.Block != 0
}

func raycastNormal(prevX, prevY, x, y int32) Vec2 {
	switch {
	case x > prevX:
		return Vec2{X: -1}
	case x < prevX:
		return Vec2{X: 1}
	case y > prevY:
		return Vec2{Y: -1}
	case y < prevY:
		return Vec2{Y: 1}
	default:
		return Vec2{}
	}
}

func lineTilesForWorldCoords(x1, y1, x2, y2 float32, rangeVal float32) []Point2 {
	dx := x2 - x1
	dy := y2 - y1
	dist := float32(math.Sqrt(float64(dx*dx + dy*dy)))
	if rangeVal > 0 && dist > rangeVal && dist > 0 {
		scale := rangeVal / dist
		x2 = x1 + dx*scale
		y2 = y1 + dy*scale
	}
	return bresenhamLine(worldToTileCoord(x1), worldToTileCoord(y1), worldToTileCoord(x2), worldToTileCoord(y2))
}

func bresenhamLine(x0, y0, x1, y1 int32) []Point2 {
	points := make([]Point2, 0)
	dx := abs32(x1 - x0)
	dy := -abs32(y1 - y0)
	sx := int32(-1)
	if x0 < x1 {
		sx = 1
	}
	sy := int32(-1)
	if y0 < y1 {
		sy = 1
	}
	err := dx + dy
	for {
		points = append(points, Point2{X: x0, Y: y0})
		if x0 == x1 && y0 == y1 {
			break
		}
		e2 := 2 * err
		if e2 >= dy {
			err += dy
			x0 += sx
		}
		if e2 <= dx {
			err += dx
			y0 += sy
		}
	}
	return points
}

func abs32(v int32) int32 {
	if v < 0 {
		return -v
	}
	return v
}

// WorldTime 时间管理
type WorldTime struct {
	Tick         uint64
	WaveTime     float32
	TotalTime    float32
	TimeScale    float32
	DayTime      float32
	DayLength    float32
	WeatherTimer float32
	Weather      *Weather
}

// NewWorldTime 创建时间管理
func NewWorldTime() *WorldTime {
	return &WorldTime{
		TimeScale: 1.0,
		DayLength: 24000, // 24000 ticks = 1 day
		Weather:   &Weather{Type: WeatherClear},
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
	if t.Weather != nil && t.Weather.Completed {
		t.Weather = &Weather{Type: WeatherClear}
		t.WeatherTimer = 0
		return
	}
	t.WeatherTimer += dt * t.TimeScale
}

// Weather 天气
type Weather struct {
	Type      WeatherType
	Intensity float32
	Duration  float32
	WindX     float32
	WindY     float32
	Completed bool
	StartTick uint64
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
	if f == nil || f.Weather == nil || f.Weather.Completed {
		return true
	}
	if f.Team == 0 {
		return true
	}
	return entity != nil && entity.GetTeam() == TeamID(f.Team)
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
		Width:   width,
		Height:  height,
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
	return b != nil && !b.CanCapture
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
	Health         float32
	MaxHealth      float32
	Items          []ItemStack
	Liquids        []LiquidStack
	Power          float32
	PowerCapacity  float32
	Payload        []byte
	ItemCapacity   int32
	LiquidCapacity float32
	Rotation       int8
	Config         []byte
	Flash          float32
	FlashTimer     float32
	Output         bool
	Team           TeamID
	Block          BlockID
	Breaking       bool
	BreakProgress  float32
	World          *World
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
		Health:         750,
		MaxHealth:      750,
		Items:          make([]ItemStack, 0),
		Liquids:        make([]LiquidStack, 0),
		ItemCapacity:   10,
		LiquidCapacity: 100,
		Team:           team,
		Block:          block,
		Flash:          0,
		FlashTimer:     0,
		Output:         false,
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
	if b == nil || b.Power+0.0001 < amount {
		return false
	}
	b.Power -= amount
	if b.Power < 0 {
		b.Power = 0
	}
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
	if b == nil {
		return
	}
	b.Output = len(b.Payload) > 0
}

// LoadPayload 装载载荷
func (b *BehaviorTile) LoadPayload(payload []byte) bool {
	if b == nil || len(payload) == 0 || b.IsLoaded() {
		return false
	}
	b.Payload = append([]byte(nil), payload...)
	b.Output = true
	return true
}

// DumpPayload 卸载载荷
func (b *BehaviorTile) DumpPayload() []byte {
	if b == nil || len(b.Payload) == 0 {
		return nil
	}
	out := append([]byte(nil), b.Payload...)
	b.Payload = nil
	b.Output = false
	return out
}

// IsLoaded 是否已装载
func (b *BehaviorTile) IsLoaded() bool {
	return b != nil && len(b.Payload) > 0
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
	capacity := b.ItemCapacity
	if capacity <= 0 {
		capacity = 10
	}
	return b.GetTotalItems() < capacity && !b.IsLoaded()
}

// Consume 消耗
func (b *BehaviorTile) Consume() bool {
	if b == nil {
		return false
	}
	if len(b.Items) > 0 {
		return b.RemoveItem(b.Items[0].Item, 1)
	}
	if len(b.Liquids) > 0 {
		amount := float32(1)
		if b.Liquids[0].Amount < amount {
			amount = b.Liquids[0].Amount
		}
		return b.ConsumeLiquids(b.Liquids[0].Liquid, amount)
	}
	return true
}

// Produce 生产
func (b *BehaviorTile) Produce() bool {
	if !b.CanProduce() {
		return false
	}
	b.Output = true
	return true
}

// CanProduce 是否可以生产
func (b *BehaviorTile) CanProduce() bool {
	return b != nil && b.IsAlive() && b.HasCapacity()
}

// Update 更新
func (b *BehaviorTile) Update() {
	if b == nil {
		return
	}
	if b.FlashTimer > 0 {
		b.FlashTimer--
		if b.FlashTimer <= 0 {
			b.Flash = 0
		}
	}
	b.UpdateConsumers()
	b.HandlePayload()
}

// UpdateConsumers 更新Consumers
func (b *BehaviorTile) UpdateConsumers() {
	if b == nil {
		return
	}
	if b.IsAlive() && (b.HasItems() || b.HasLiquids() || b.Power > 0) {
		b.Output = true
	}
}

// Draw 绘制
func (b *BehaviorTile) Draw() {
	// 服务器不执行客户端绘制，保留为空实现。
}

// DrawPlaced 绘制已放置
func (b *BehaviorTile) DrawPlaced() {
	// 服务器不执行客户端绘制，保留为空实现。
}

// DrawPlan 绘制计划
func (b *BehaviorTile) DrawPlan() {
	// 服务器不执行客户端绘制，保留为空实现。
}

// DrawPlanOverlay 绘制计划叠加
func (b *BehaviorTile) DrawPlanOverlay() {
	// 服务器不执行客户端绘制，保留为空实现。
}

// DrawPlanOverlayPlan 绘制计划叠加计划
func (b *BehaviorTile) DrawPlanOverlayPlan() {
	// 服务器不执行客户端绘制，保留为空实现。
}

// DrawPlanOverlayPlanOverlay 绘制计划叠加计划叠加
func (b *BehaviorTile) DrawPlanOverlayPlanOverlay() {
	// 服务器不执行客户端绘制，保留为空实现。
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
	if b.World == nil {
		return true
	}
	sx, sy := behaviorTileWorldCenter(b)
	tx, ty := behaviorTileWorldCenter(target)
	b.World.mu.RLock()
	defer b.World.mu.RUnlock()
	result := b.World.raycastLocked(sx, sy, tx, ty, b.Team, true)
	return result == nil || !result.Hit
}

// CanSeePos 是否可见位置
func (b *BehaviorTile) CanSeePos(x, y int32) bool {
	if b == nil || b.World == nil {
		return true
	}
	sx, sy := behaviorTileWorldCenter(b)
	tx := float32(x*8 + 4)
	ty := float32(y*8 + 4)
	b.World.mu.RLock()
	defer b.World.mu.RUnlock()
	result := b.World.raycastLocked(sx, sy, tx, ty, b.Team, true)
	return result == nil || !result.Hit
}

// TargetTarget 目标目标
func (b *BehaviorTile) TargetTarget(team TeamID) *BehaviorTile {
	return b.TargetEnemy(team)
}

// TargetEnemy 目标敌人
func (b *BehaviorTile) TargetEnemy(team TeamID) *BehaviorTile {
	return b.findTarget(func(candidate *BehaviorTile) bool {
		return candidate != nil && candidate.Team != 0 && candidate.Team != team
	})
}

// TargetFriendly 目标盟友
func (b *BehaviorTile) TargetFriendly(team TeamID) *BehaviorTile {
	return b.findTarget(func(candidate *BehaviorTile) bool {
		return candidate != nil && candidate.Team == team
	})
}

// TargetBuilding 目标建筑
func (b *BehaviorTile) TargetBuilding() *BehaviorTile {
	return b.findTarget(func(candidate *BehaviorTile) bool {
		return candidate != nil && candidate.Block != 0
	})
}

// TargetUnit 目标单位
func (b *BehaviorTile) TargetUnit() *BehaviorTile {
	return b.findTarget(func(candidate *BehaviorTile) bool {
		return candidate != nil && candidate.Block == 0
	})
}

// TargetCore 目标核心
func (b *BehaviorTile) TargetCore() *BehaviorTile {
	return b.findTarget(func(candidate *BehaviorTile) bool {
		if candidate == nil || candidate.World == nil || candidate.Block == 0 {
			return false
		}
		return isCoreBlockName(candidate.World.blockNameByID(int16(candidate.Block)))
	})
}

// TargetPlayer 目标玩家
func (b *BehaviorTile) TargetPlayer() *BehaviorTile {
	return b.TargetUnit()
}

// TargetAny 目标任意
func (b *BehaviorTile) TargetAny() *BehaviorTile {
	return b.findTarget(func(candidate *BehaviorTile) bool { return candidate != nil })
}

func behaviorTileWorldCenter(b *BehaviorTile) (float32, float32) {
	return float32(b.Tile.X*8 + 4), float32(b.Tile.Y*8 + 4)
}

func behaviorTileDistanceSq(a, c *BehaviorTile) float32 {
	ax, ay := behaviorTileWorldCenter(a)
	cx, cy := behaviorTileWorldCenter(c)
	dx := cx - ax
	dy := cy - ay
	return dx*dx + dy*dy
}

func (b *BehaviorTile) findTarget(match func(*BehaviorTile) bool) *BehaviorTile {
	if b == nil || b.World == nil || b.World.model == nil {
		return nil
	}
	b.World.mu.RLock()
	defer b.World.mu.RUnlock()

	var best *BehaviorTile
	bestDist := float32(math.MaxFloat32)
	for i := range b.World.model.Tiles {
		tile := &b.World.model.Tiles[i]
		if tile.Build == nil || tile.Build.Health <= 0 {
			continue
		}
		candidate := behaviorTileFromTileLocked(b.World, tile)
		if !match(candidate) || sameBehaviorTilePosition(b, candidate) {
			continue
		}
		if d2 := behaviorTileDistanceSq(b, candidate); d2 < bestDist {
			best = candidate
			bestDist = d2
		}
	}
	for i := range b.World.model.Entities {
		entity := &b.World.model.Entities[i]
		if entity.Health <= 0 {
			continue
		}
		candidate := behaviorTileFromEntityLocked(b.World, entity)
		if !match(candidate) || sameBehaviorTilePosition(b, candidate) {
			continue
		}
		if d2 := behaviorTileDistanceSq(b, candidate); d2 < bestDist {
			best = candidate
			bestDist = d2
		}
	}
	return best
}

func sameBehaviorTilePosition(a, b *BehaviorTile) bool {
	if a == nil || b == nil {
		return false
	}
	return a.Tile.X == b.Tile.X && a.Tile.Y == b.Tile.Y && a.Block == b.Block
}

func behaviorTileFromTileLocked(w *World, tile *Tile) *BehaviorTile {
	if tile == nil {
		return nil
	}
	out := &BehaviorTile{
		Tile:  *tile,
		World: w,
		Block: tile.Block,
		Team:  tile.Team,
	}
	if tile.Build != nil {
		out.Health = tile.Build.Health
		out.MaxHealth = tile.Build.MaxHealth
		out.Items = append([]ItemStack(nil), tile.Build.Items...)
		out.Liquids = append([]LiquidStack(nil), tile.Build.Liquids...)
		out.Rotation = tile.Build.Rotation
		out.Config = append([]byte(nil), tile.Build.Config...)
		out.Payload = append([]byte(nil), tile.Build.Payload...)
		if tile.Build.Team != 0 {
			out.Team = tile.Build.Team
		}
	}
	return out
}

func behaviorTileFromEntityLocked(w *World, entity *RawEntity) *BehaviorTile {
	if entity == nil {
		return nil
	}
	return &BehaviorTile{
		Tile: Tile{
			X: int(entity.X / 8),
			Y: int(entity.Y / 8),
		},
		World:     w,
		Health:    entity.Health,
		MaxHealth: entity.MaxHealth,
		Rotation:  int8(entity.Rotation),
		Team:      entity.Team,
	}
}
