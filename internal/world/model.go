package world

import (
	"errors"
	"math"
)

var ErrOutOfBounds = errors.New("world: out of bounds")

type TeamID byte

type BlockID int16
type FloorID int16
type OverlayID int16
type ConID int16

type ItemID int16
type LiquidID int16

type Vec2 struct {
	X float32
	Y float32
}

// Entity 实体接口
type Entity interface {
	GetX() float32
	GetY() float32
	GetTeam() TeamID
}

type Tile struct {
	X        int
	Y        int
	Floor    FloorID
	Overlay  OverlayID
	Block    BlockID
	Team     TeamID
	Rotation int8
	Con      ConID
	Build    *Building
}

type Building struct {
	Block     BlockID
	Team      TeamID
	Rotation  int8
	X         int
	Y         int
	Items     []ItemStack
	Liquids   []LiquidStack
	Health    float32
	Config    []byte
	Payload   []byte
	MaxHealth float32
}

// GetX 获取X坐标
func (b *Building) GetX() float32 {
	return float32(b.X)
}

// GetY 获取Y坐标
func (b *Building) GetY() float32 {
	return float32(b.Y)
}

// GetTeam 获取队伍
func (b *Building) GetTeam() TeamID {
	return b.Team
}

// AddItem 添加物品
func (b *Building) AddItem(item ItemID, amount int32) {
	for i, stack := range b.Items {
		if stack.Item == item {
			b.Items[i].Amount += amount
			return
		}
	}
	b.Items = append(b.Items, ItemStack{Item: item, Amount: amount})
}

// RemoveItem 移除物品
func (b *Building) RemoveItem(item ItemID, amount int32) bool {
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

func (b *Building) ItemAmount(item ItemID) int32 {
	for _, stack := range b.Items {
		if stack.Item == item {
			return stack.Amount
		}
	}
	return 0
}

func (b *Building) AddLiquid(liquid LiquidID, amount float32) {
	for i, stack := range b.Liquids {
		if stack.Liquid == liquid {
			b.Liquids[i].Amount += amount
			return
		}
	}
	b.Liquids = append(b.Liquids, LiquidStack{Liquid: liquid, Amount: amount})
}

func (b *Building) RemoveLiquid(liquid LiquidID, amount float32) bool {
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

func (b *Building) LiquidAmount(liquid LiquidID) float32 {
	for _, stack := range b.Liquids {
		if stack.Liquid == liquid {
			return stack.Amount
		}
	}
	return 0
}

// DistanceTo 距离到
func (b *Building) DistanceTo(other *Building) float32 {
	dx := float32(b.X - other.X)
	dy := float32(b.Y - other.Y)
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

type ItemStack struct {
	Item   ItemID
	Amount int32
}

type LiquidStack struct {
	Liquid LiquidID
	Amount float32
}

type Unit struct {
	ID        int32
	Team      TeamID
	Pos       Vec2
	Health    float32
	Type      int16
	MaxHealth float32
}

// GetX 获取X坐标
func (u *Unit) GetX() float32 {
	return u.Pos.X
}

// GetY 获取Y坐标
func (u *Unit) GetY() float32 {
	return u.Pos.Y
}

// GetTeam 获取队伍
func (u *Unit) GetTeam() TeamID {
	return u.Team
}

// DistanceTo 距离到
func (u *Unit) DistanceTo(other *Unit) float32 {
	dx := u.Pos.X - other.Pos.X
	dy := u.Pos.Y - other.Pos.Y
	return float32(math.Sqrt(float64(dx*dx + dy*dy)))
}

type RawEntity struct {
	TypeID      int16
	ID          int32
	X           float32
	Y           float32
	Rotation    float32
	VelX        float32
	VelY        float32
	RotVel      float32
	LifeSec     float32
	AgeSec      float32
	Health      float32
	MaxHealth   float32
	Shield      float32
	ShieldMax   float32
	ShieldRegen float32
	Armor       float32

	AttackRange           float32
	AttackFireMode        string
	AttackDamage          float32
	AttackInterval        float32
	AttackCooldown        float32
	AttackBulletType      int16
	AttackBulletSpeed     float32
	AttackSplashRadius    float32
	AttackSlowSec         float32
	AttackSlowMul         float32
	AttackPierce          int32
	AttackChainCount      int32
	AttackChainRange      float32
	AttackPreferBuildings bool
	AttackFragmentCount   int32
	AttackFragmentSpread  float32
	AttackFragmentSpeed   float32
	AttackFragmentLife    float32
	AttackTargetAir       bool
	AttackTargetGround    bool
	AttackTargetPriority  string
	AttackBuildings       bool
	RuntimeInit           bool
	SlowRemain            float32
	SlowMul               float32
	HitRadius             float32

	Behavior  string
	TargetID  int32
	PatrolAX  float32
	PatrolAY  float32
	PatrolBX  float32
	PatrolBY  float32
	PatrolToB bool
	MoveSpeed float32
	Team      TeamID
	Payload   []byte
}

type WorldModel struct {
	Width  int
	Height int
	Tiles  []Tile

	Units        map[int32]*Unit
	Entities     []RawEntity
	NextEntityID int32

	MSAVVersion int32
	Tags        map[string]string
	Content     []byte
	Patches     []byte
	RawMap      []byte
	RawEntities []byte
	Markers     []byte
	Custom      []byte
	BlockNames  map[int16]string
	UnitNames   map[int16]string

	EntitiesRev byte
}

func NewWorldModel(width, height int) *WorldModel {
	total := width * height
	tiles := make([]Tile, total)
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			i := y*width + x
			tiles[i] = Tile{X: x, Y: y}
		}
	}
	return &WorldModel{
		Width:        width,
		Height:       height,
		Tiles:        tiles,
		Units:        make(map[int32]*Unit),
		Entities:     make([]RawEntity, 0),
		NextEntityID: 1,
	}
}

func (w *WorldModel) InBounds(x, y int) bool {
	return x >= 0 && y >= 0 && x < w.Width && y < w.Height
}

func (w *WorldModel) TileAt(x, y int) (*Tile, error) {
	if !w.InBounds(x, y) {
		return nil, ErrOutOfBounds
	}
	return &w.Tiles[y*w.Width+x], nil
}

func (w *WorldModel) AddEntity(e RawEntity) RawEntity {
	if e.ID == 0 {
		e.ID = w.NextEntityID
	}
	if e.ID >= w.NextEntityID {
		w.NextEntityID = e.ID + 1
	}
	w.Entities = append(w.Entities, e)
	w.EntitiesRev++
	return e
}

func (w *WorldModel) RemoveEntity(id int32) (RawEntity, bool) {
	for i := range w.Entities {
		if w.Entities[i].ID != id {
			continue
		}
		removed := w.Entities[i]
		last := len(w.Entities) - 1
		w.Entities[i] = w.Entities[last]
		w.Entities = w.Entities[:last]
		w.EntitiesRev++
		return removed, true
	}
	return RawEntity{}, false
}
