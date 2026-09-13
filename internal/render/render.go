package render

import (
	"mdt-server/internal/world"
)

// Renderer 渲染器接口
type Renderer interface {
	RenderTile(tile *world.Tile)
	RenderBuilding(build *world.Building)
	RenderUnit(unit *world.Unit)
	RenderEffect(effect Effect)
	RenderLine(x1, y1, x2, y2 float32, color Color)
	RenderCircle(x, y, radius float32, color Color)
	RenderText(x, y float32, text string, color Color)
	Clear()
	Flush()
}

// Effect 特效
type Effect struct {
	X, Y      float32
	Type      EffectType
	Arg1      int32
	Arg2      int32
	Arg3      int32
	Duration  float32
	Color     Color
	Size      float32
}

// EffectType 特效类型
type EffectType byte

const (
	EffectNone EffectType = iota
	EffectExplode               // 爆炸
	EffectFlame                 // 火焰
	EffectPyra                  // 金字塔
	EffectToast                 // 烟花
	EffectShockwave             // 冲击波
	EffectPulseraycast          // 脉冲射线
	EffectSpark                 // 电火花
	EffectSporeExplosion        // 孢子爆炸
	EffectSporeShrink           // 孢子收缩
	EffectScorch                // 焦痕
	EffectBulletImpact          // 子弹击中
	EffectBulletSmoke           // 子弹烟雾
	EffectMoltenImpact          // 熔融击中
	EffectMist                    // 雾气
	EffectSmoke                 // 烟雾
	EffectSteam                 // 蒸汽
	EffectAirTap                // 空气轻击
	EffectBulletPuff            // 子弹 puff
	EffectElectricSpark         // 电火花
	EffectDefenseWallImpact     // 防御墙击中
	EffectShake                 // 震动
	EffectParticles             // 粒子
	EffectBolt                  // 闪电
	EffectLine                  // 线
)

// Color 颜色
type Color struct {
	R, G, B, A float32
}

// NewColor 创建颜色
func NewColor(r, g, b, a float32) Color {
	return Color{R: r, G: g, B: b, A: a}
}

//NewColorRGB 创建RGB颜色 (0-255)
func NewColorRGB(r, g, b byte) Color {
	return Color{
		R: float32(r) / 255.0,
		G: float32(g) / 255.0,
		B: float32(b) / 255.0,
		A: 1.0,
	}
}

// 添加方法到 Color
func (c Color) WithAlpha(alpha float32) Color {
	c.A = alpha
	return c
}

// Blend 混合颜色
func (c Color) Blend(other Color, amount float32) Color {
	return Color{
		R: c.R*(1-amount) + other.R*amount,
		G: c.G*(1-amount) + other.G*amount,
		B: c.B*(1-amount) + other.B*amount,
		A: c.A*(1-amount) + other.A*amount,
	}
}

// 复制颜色
func (c Color) Clone() Color {
	return Color{
		R: c.R, G: c.G, B: c.B, A: c.A,
	}
}

// DefaultRenderer 默认渲染器（空实现）
type DefaultRenderer struct{}

// RenderTile 渲染Tile
func (r *DefaultRenderer) RenderTile(tile *world.Tile) {
	// 空实现
}

// RenderBuilding 渲染建筑
func (r *DefaultRenderer) RenderBuilding(build *world.Building) {
	// 空实现
}

// RenderUnit 渲染单位
func (r *DefaultRenderer) RenderUnit(unit *world.Unit) {
	// 空实现
}

// RenderEffect 渲染特效
func (r *DefaultRenderer) RenderEffect(effect Effect) {
	// 空实现
}

// RenderLine 渲染线
func (r *DefaultRenderer) RenderLine(x1, y1, x2, y2 float32, color Color) {
	// 空实现
}

// RenderCircle 渲染圆
func (r *DefaultRenderer) RenderCircle(x, y, radius float32, color Color) {
	// 空实现
}

// RenderText 渲染文本
func (r *DefaultRenderer) RenderText(x, y float32, text string, color Color) {
	// 空实现
}

// Clear 清除
func (r *DefaultRenderer) Clear() {
	// 空实现
}

// Flush 刷新
func (r *DefaultRenderer) Flush() {
	// 空实现
}

// DebugRenderer 调试渲染器
type DebugRenderer struct {
	Items []DebugItem
}

// DebugItem 调试项
type DebugItem struct {
	X, Y       float32
	Type       string
	Color      Color
	Text       string
	Radius     float32
}

// RenderTile 渲染Tile
func (r *DebugRenderer) RenderTile(tile *world.Tile) {
	if tile.Build != nil {
		r.Items = append(r.Items, DebugItem{
			X:     float32(tile.X),
			Y:     float32(tile.Y),
			Type:  "building",
			Color: GetTeamColor(tile.Team),
			Text:  "B",
		})
	}
}

// RenderBuilding 渲染建筑
func (r *DebugRenderer) RenderBuilding(build *world.Building) {
	r.Items = append(r.Items, DebugItem{
		X:     float32(build.X),
		Y:     float32(build.Y),
		Type:  "building",
		Color: GetTeamColor(build.Team),
		Text:  "B",
	})
}

// RenderUnit 渲染单位
func (r *DebugRenderer) RenderUnit(unit *world.Unit) {
	r.Items = append(r.Items, DebugItem{
		X:     unit.Pos.X,
		Y:     unit.Pos.Y,
		Type:  "unit",
		Color: GetTeamColor(unit.Team),
		Text:  "U",
	})
}

// RenderEffect 渲染特效
func (r *DebugRenderer) RenderEffect(effect Effect) {
	r.Items = append(r.Items, DebugItem{
		X:     effect.X,
		Y:     effect.Y,
		Type:  "effect",
		Color: effect.Color,
		Text:  effect.Type.String(),
	})
}

// RenderLine 渲染线
func (r *DebugRenderer) RenderLine(x1, y1, x2, y2 float32, color Color) {
	r.Items = append(r.Items, DebugItem{
		X:     x1,
		Y:     y1,
		Type:  "line",
		Color: color,
	})
}

// RenderCircle 渲染圆
func (r *DebugRenderer) RenderCircle(x, y, radius float32, color Color) {
	r.Items = append(r.Items, DebugItem{
		X:     x,
		Y:     y,
		Type:  "circle",
		Color: color,
		Radius: radius,
	})
}

// RenderText 渲染文本
func (r *DebugRenderer) RenderText(x, y float32, text string, color Color) {
	r.Items = append(r.Items, DebugItem{
		X:     x,
		Y:     y,
		Type:  "text",
		Color: color,
		Text:  text,
	})
}

// Clear 清除
func (r *DebugRenderer) Clear() {
	r.Items = nil
}

// Flush 刷新
func (r *DebugRenderer) Flush() {
	// 调试信息已记录在 Items 中
}

// GetTeamColor 获取队伍颜色
func GetTeamColor(team world.TeamID) Color {
	// 根据队伍ID返回颜色
	colors := []Color{
		{R: 0.93, G: 0.36, B: 0.36, A: 1.0}, // neutor
		{R: 0.34, G: 0.69, B: 0.79, A: 1.0}, // Shiyan
		{R: 0.67, G: 0.5, B: 0.76, A: 1.0},  // path
		{R: 0.32, G: 0.66, B: 0.36, A: 1.0}, // linner
		{R: 0.95, G: 0.56, B: 0.12, A: 1.0}, // rime
	}
	// Ensure teamIdx is non-negative and within bounds
	teamIdx := int(team) % len(colors)
	if teamIdx < 0 {
		teamIdx += len(colors)
	}
	return colors[teamIdx]
}

// String 返回特效类型名称
func (e EffectType) String() string {
	switch e {
	case EffectExplode:
		return "explode"
	case EffectFlame:
		return "flame"
	case EffectPyra:
		return "pyra"
	case EffectToast:
		return "toast"
	case EffectShockwave:
		return "shockwave"
	case EffectPulseraycast:
		return "pulseraycast"
	case EffectSpark:
		return "spark"
	case EffectSporeExplosion:
		return "spore_explosion"
	case EffectSporeShrink:
		return "spore_shrink"
	case EffectScorch:
		return "scorch"
	case EffectBulletImpact:
		return "bullet_impact"
	case EffectBulletSmoke:
		return "bullet_smoke"
	case EffectMoltenImpact:
		return "molten_impact"
	case EffectMist:
		return "mist"
	case EffectSmoke:
		return "smoke"
	case EffectSteam:
		return "steam"
	case EffectAirTap:
		return "air_tap"
	case EffectBulletPuff:
		return "bullet_puff"
	case EffectElectricSpark:
		return "electric_spark"
	case EffectDefenseWallImpact:
		return "defense_wall_impact"
	case EffectShake:
		return "shake"
	case EffectParticles:
		return "particles"
	case EffectBolt:
		return "bolt"
	case EffectLine:
		return "line"
	default:
		return "unknown"
	}
}
