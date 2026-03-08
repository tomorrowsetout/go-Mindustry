package protocol

import "errors"

type ContentType byte

type Content interface {
	ContentType() ContentType
	ID() int16
	Name() string
}

type Block interface{ Content }
type Item interface{ Content }
type Liquid interface{ Content }
type UnitType interface{ Content }
type BulletType interface{ Content }
type StatusEffect interface {
	Content
	Dynamic() bool
}

type statusEffectBox struct {
	id   int16
	name string
}

func (s statusEffectBox) ContentType() ContentType { return ContentStatus }
func (s statusEffectBox) ID() int16                { return s.id }
func (s statusEffectBox) Dynamic() bool            { return false }
func (s statusEffectBox) Name() string             { return s.name }

type Weather interface{ Content }

type Effect struct {
	ID   int16
	Name string
}

type Sound struct {
	ID   int16
	Name string
}

// Content type IDs mirror mindustry.ctype.ContentType order.
const (
	ContentItem        ContentType = 0
	ContentBlock       ContentType = 1
	ContentBullet      ContentType = 3
	ContentLiquid      ContentType = 4
	ContentStatus      ContentType = 5
	ContentUnit        ContentType = 6
	ContentWeather     ContentType = 7
	ContentTeam        ContentType = 15
	ContentUnitCommand ContentType = 16
	ContentUnitStance  ContentType = 17
)

type contentBox struct {
	typ  ContentType
	id   int16
	name string
}

func (c contentBox) ContentType() ContentType { return c.typ }
func (c contentBox) ID() int16                { return c.id }
func (c contentBox) Name() string             { return c.name }

type ContentMapper interface {
	Get(t ContentType, id int16) Content
}

type Unit interface {
	ID() int32
}

// BlockUnit represents a block-backed unit (BlockUnitc in Java).
// Tile() should return the backing building tile.
type BlockUnit interface {
	Unit
	Tile() Tile
}

type UnitController interface{}

type UnitSyncEntity interface {
	ID() int32
	SetID(id int32)
	ClassID() byte
	BeforeWrite()
	WriteSync(w *Writer) error
	ReadSync(r *Reader) error
	SnapSync()
	Add()
}

type Entity interface {
	ID() int32
}

type Building interface {
	Pos() int32
}

// ControlBlock represents a building that controls a unit (ControlBlock in Java).
type ControlBlock interface {
	Unit() Unit
}

type ControllerType byte

const (
	ControllerPlayer    ControllerType = 0
	ControllerFormation ControllerType = 1
	ControllerGenericAI ControllerType = 2
	ControllerLogic     ControllerType = 3
	ControllerCommand4  ControllerType = 4
	ControllerAssembler ControllerType = 5
	ControllerCommand6  ControllerType = 6
	ControllerCommand7  ControllerType = 7
	ControllerCommand8  ControllerType = 8
	ControllerCommand9  ControllerType = 9
)

type ControllerState struct {
	Type ControllerType
	// player controller
	PlayerID int32
	// logic controller
	LogicPos int32
	// command controller
	Command CommandState
}

type CommandState struct {
	HasAttack bool
	HasPos    bool
	TargetPos Vec2
	Target    CommandTarget
	CommandID int8
	Queue     []CommandTarget
	Stances   []UnitStance
}

type CommandTarget struct {
	Type byte // 0 building, 1 unit, 2 vec2, 3 invalid
	Pos  int32
	Vec  Vec2
}

type Team struct {
	ID   byte
	Name string
}

type UnitCommand struct {
	ID   int16
	Name string
}

type UnitStance struct {
	ID   int16
	Name string
}

type LAccess uint16

type TechNode struct {
	Content Content
}

type IntSeq struct {
	Items []int32
}

type Point2 struct {
	X int32
	Y int32
}

// BlockRef is a lightweight block reference for network packets.
type BlockRef struct {
	BlkID   int16
	BlkName string
}

func (b BlockRef) ContentType() ContentType { return ContentBlock }
func (b BlockRef) ID() int16                { return b.BlkID }
func (b BlockRef) Name() string             { return b.BlkName }

// ItemRef is a lightweight item reference for network packets.
type ItemRef struct {
	ItmID   int16
	ItmName string
}

func (i ItemRef) ContentType() ContentType { return ContentItem }
func (i ItemRef) ID() int16                { return i.ItmID }
func (i ItemRef) Name() string             { return i.ItmName }

func PackPoint2(x, y int32) int32 {
	return (x&0xFFFF)<<16 | (y & 0xFFFF)
}

func UnpackPoint2(v int32) Point2 {
	x := int16((v >> 16) & 0xFFFF)
	y := int16(v & 0xFFFF)
	return Point2{X: int32(x), Y: int32(y)}
}

func UnpackPoint2X(v int32) int32 {
	return int32(int16((v >> 16) & 0xFFFF))
}

func UnpackPoint2Y(v int32) int32 {
	return int32(int16(v & 0xFFFF))
}

type Vec2 struct {
	X float32
	Y float32
}

type Tile interface {
	Pos() int32
}

type Color struct {
	RGBA int32
}

type ItemStack struct {
	Item   Item
	Amount int32
}

type LiquidStack struct {
	Liquid Liquid
	Amount float32
}

type StatusEntry struct {
	Effect StatusEffect
	Time   float32

	DamageMultiplier     float32
	HealthMultiplier     float32
	SpeedMultiplier      float32
	ReloadMultiplier     float32
	BuildSpeedMultiplier float32
	DragMultiplier       float32
	ArmorOverride        float32
	Dynamic              bool
}

type KickReason byte
type AdminAction byte
type LMarkerControl byte

const (
	KickReasonKick KickReason = iota
	KickReasonClientOutdated
	KickReasonServerOutdated
	KickReasonBanned
	KickReasonGameOver
	KickReasonRecentKick
	KickReasonNameInUse
	KickReasonIDInUse
	KickReasonNameEmpty
	KickReasonCustomClient
	KickReasonServerClose
	KickReasonVote
	KickReasonTypeMismatch
	KickReasonWhitelist
	KickReasonPlayerLimit
	KickReasonServerRestarting
)

type Payload interface {
	WritePayload(w *Writer) error
}

const (
	PayloadUnit  byte = 0
	PayloadBlock byte = 1
)

// PayloadBox is a raw payload wrapper. Raw should contain the payload payload bytes
// starting with the payload type byte (e.g. payloadUnit/payloadBlock).
type PayloadBox struct {
	Raw []byte
}

func (p PayloadBox) WritePayload(w *Writer) error {
	if len(p.Raw) == 0 {
		return ErrUnsupportedTypeIO
	}
	return w.WriteBytes(p.Raw)
}

type UnitPayload struct {
	ClassID byte
	Entity  UnitSyncEntity
	Raw     []byte
}

func (p UnitPayload) WritePayload(w *Writer) error {
	if err := w.WriteByte(PayloadUnit); err != nil {
		return err
	}
	if err := w.WriteByte(p.ClassID); err != nil {
		return err
	}
	if len(p.Raw) > 0 {
		return w.WriteBytes(p.Raw)
	}
	if p.Entity != nil {
		return p.Entity.WriteSync(w)
	}
	return nil
}

type BuildPayload struct {
	BlockID int16
	Version byte
	Raw     []byte
}

func (p BuildPayload) WritePayload(w *Writer) error {
	if err := w.WriteByte(PayloadBlock); err != nil {
		return err
	}
	if err := w.WriteInt16(p.BlockID); err != nil {
		return err
	}
	if err := w.WriteByte(p.Version); err != nil {
		return err
	}
	if len(p.Raw) > 0 {
		return w.WriteBytes(p.Raw)
	}
	return nil
}

type PayloadReader func(r *Reader) (Payload, error)

type WeaponMount interface {
	AimX() float32
	AimY() float32
	SetAim(x, y float32)
	Shoot() bool
	Rotate() bool
	SetShoot(bool)
	SetRotate(bool)
}

type Ability interface {
	Data() float32
	SetData(float32)
}

type TraceInfo struct {
	IP          string
	UUID        string
	Locale      string
	Modded      bool
	Mobile      bool
	TimesJoined int32
	TimesKicked int32
	IPs         []string
	Names       []string
}

type Rules struct {
	Raw string
}

type MapObjectives struct {
	Raw string
}

type ObjectiveMarker struct {
	Raw string
}

type BuildingBox struct {
	PosValue int32
}

func (b BuildingBox) Pos() int32 { return b.PosValue }

type UnitBox struct {
	IDValue int32
}

func (u UnitBox) ID() int32 { return u.IDValue }

type TileBox struct {
	PosValue int32
}

func (t TileBox) Pos() int32 { return t.PosValue }

type EntityBox struct {
	IDValue int32
}

func (e *EntityBox) ID() int32               { return e.IDValue }
func (e *EntityBox) SetID(id int32)          { e.IDValue = id }
func (e *EntityBox) ClassID() byte           { return 0 }
func (e *EntityBox) BeforeWrite()            {}
func (e *EntityBox) WriteSync(*Writer) error { return nil }
func (e *EntityBox) ReadSync(*Reader) error  { return nil }
func (e *EntityBox) SnapSync()               {}
func (e *EntityBox) Add()                    {}

type TypeIOContext struct {
	Content            ContentMapper
	BlockLookup        func(id int16) Block
	ItemLookup         func(id int16) Item
	LiquidLookup       func(id int16) Liquid
	UnitTypeLookup     func(id int16) UnitType
	BulletTypeLookup   func(id int16) BulletType
	StatusEffectLookup func(id int16) StatusEffect
	WeatherLookup      func(id int16) Weather
	EffectLookup       func(id int16) Effect
	SoundLookup        func(id int16) Sound
	UnitLookup         func(id int32) Unit
	BuildingLookup     func(pos int32) Building
	TeamLookup         func(id byte) Team
	UnitCommandLookup  func(id int16) UnitCommand
	UnitStanceLookup   func(id int16) UnitStance
	PayloadRead        PayloadReader
	// Optional full payload decoders. If set, DefaultPayloadRead will use them.
	PayloadUnitRead  func(r *Reader, classID byte) (Payload, error)
	PayloadBuildRead func(r *Reader, blockID int16, version byte) (Payload, error)
	EntityByID         func(id int32) UnitSyncEntity
	EntityFactory      func(classID byte) UnitSyncEntity
	IsEntityUsed       func(id int32) bool
	AddEntity          func(ent UnitSyncEntity)
	AddRemovedEntity   func(id int32)
}

func WriteObject(w *Writer, obj any, ctx *TypeIOContext) error {
	switch v := obj.(type) {
	case nil:
		return w.WriteByte(0)
	case int32:
		if err := w.WriteByte(1); err != nil {
			return err
		}
		return w.WriteInt32(v)
	case int64:
		if err := w.WriteByte(2); err != nil {
			return err
		}
		return w.WriteInt64(v)
	case float32:
		if err := w.WriteByte(3); err != nil {
			return err
		}
		return w.WriteFloat32(v)
	case string:
		if err := w.WriteByte(4); err != nil {
			return err
		}
		s := v
		return w.WriteStringNullable(&s)
	case Content:
		if err := w.WriteByte(5); err != nil {
			return err
		}
		if err := w.WriteByte(byte(v.ContentType())); err != nil {
			return err
		}
		return w.WriteInt16(v.ID())
	case IntSeq:
		if err := w.WriteByte(6); err != nil {
			return err
		}
		if err := w.WriteInt16(int16(len(v.Items))); err != nil {
			return err
		}
		for _, n := range v.Items {
			if err := w.WriteInt32(n); err != nil {
				return err
			}
		}
		return nil
	case Point2:
		if err := w.WriteByte(7); err != nil {
			return err
		}
		if err := w.WriteInt32(v.X); err != nil {
			return err
		}
		return w.WriteInt32(v.Y)
	case []Point2:
		if err := w.WriteByte(8); err != nil {
			return err
		}
		if err := w.WriteByte(byte(len(v))); err != nil {
			return err
		}
		for _, p := range v {
			if err := w.WriteInt32(PackPoint2(p.X, p.Y)); err != nil {
				return err
			}
		}
		return nil
	case TechNode:
		if err := w.WriteByte(9); err != nil {
			return err
		}
		if v.Content == nil {
			return errors.New("tech node has nil content")
		}
		if err := w.WriteByte(byte(v.Content.ContentType())); err != nil {
			return err
		}
		return w.WriteInt16(v.Content.ID())
	case bool:
		if err := w.WriteByte(10); err != nil {
			return err
		}
		return w.WriteBool(v)
	case float64:
		if err := w.WriteByte(11); err != nil {
			return err
		}
		return w.WriteFloat64(v)
	case Building:
		if err := w.WriteByte(12); err != nil {
			return err
		}
		return w.WriteInt32(v.Pos())
	case BuildingBox:
		if err := w.WriteByte(12); err != nil {
			return err
		}
		return w.WriteInt32(v.PosValue)
	case LAccess:
		if err := w.WriteByte(13); err != nil {
			return err
		}
		return w.WriteInt16(int16(v))
	case []byte:
		if err := w.WriteByte(14); err != nil {
			return err
		}
		if err := w.WriteInt32(int32(len(v))); err != nil {
			return err
		}
		return w.WriteBytes(v)
	case []bool:
		if err := w.WriteByte(16); err != nil {
			return err
		}
		if err := w.WriteInt32(int32(len(v))); err != nil {
			return err
		}
		for _, b := range v {
			if err := w.WriteBool(b); err != nil {
				return err
			}
		}
		return nil
	case Unit:
		if err := w.WriteByte(17); err != nil {
			return err
		}
		return w.WriteInt32(v.ID())
	case UnitBox:
		if err := w.WriteByte(17); err != nil {
			return err
		}
		return w.WriteInt32(v.IDValue)
	case []Vec2:
		if err := w.WriteByte(18); err != nil {
			return err
		}
		if err := w.WriteInt16(int16(len(v))); err != nil {
			return err
		}
		for _, vec := range v {
			if err := w.WriteFloat32(vec.X); err != nil {
				return err
			}
			if err := w.WriteFloat32(vec.Y); err != nil {
				return err
			}
		}
		return nil
	case Vec2:
		if err := w.WriteByte(19); err != nil {
			return err
		}
		if err := w.WriteFloat32(v.X); err != nil {
			return err
		}
		return w.WriteFloat32(v.Y)
	case Team:
		if err := w.WriteByte(20); err != nil {
			return err
		}
		return w.WriteByte(v.ID)
	case []int32:
		if err := w.WriteByte(21); err != nil {
			return err
		}
		return WriteInts(w, v)
	case []any:
		if err := w.WriteByte(22); err != nil {
			return err
		}
		if err := w.WriteInt32(int32(len(v))); err != nil {
			return err
		}
		for _, o := range v {
			if err := WriteObject(w, o, ctx); err != nil {
				return err
			}
		}
		return nil
	case UnitCommand:
		if err := w.WriteByte(23); err != nil {
			return err
		}
		return w.WriteInt16(v.ID)
	default:
		return ErrUnsupportedTypeIO
	}
}

func ReadObject(r *Reader, box bool, ctx *TypeIOContext) (any, error) {
	t, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch t {
	case 0:
		return nil, nil
	case 1:
		return r.ReadInt32()
	case 2:
		return r.ReadInt64()
	case 3:
		return r.ReadFloat32()
	case 4:
		s, err := r.ReadStringNullable()
		if err != nil {
			return nil, err
		}
		if s == nil {
			return nil, nil
		}
		return *s, nil
	case 5:
		ct, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		id, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		if ctx != nil && ctx.Content != nil {
			if c := ctx.Content.Get(ContentType(ct), id); c != nil {
				return c, nil
			}
		}
		return contentBox{typ: ContentType(ct), id: id}, nil
	case 6:
		l, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		items := make([]int32, l)
		for i := 0; i < int(l); i++ {
			v, err := r.ReadInt32()
			if err != nil {
				return nil, err
			}
			items[i] = v
		}
		return IntSeq{Items: items}, nil
	case 7:
		x, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		y, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		return Point2{X: x, Y: y}, nil
	case 8:
		lb, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		out := make([]Point2, lb)
		for i := 0; i < int(lb); i++ {
			v, err := r.ReadInt32()
			if err != nil {
				return nil, err
			}
			out[i] = UnpackPoint2(v)
		}
		return out, nil
	case 9:
		ct, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		id, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		if ctx != nil && ctx.Content != nil {
			if c := ctx.Content.Get(ContentType(ct), id); c != nil {
				return TechNode{Content: c}, nil
			}
		}
		return TechNode{Content: contentBox{typ: ContentType(ct), id: id}}, nil
	case 10:
		return r.ReadBool()
	case 11:
		v, err := r.ReadFloat64()
		if err != nil {
			return nil, err
		}
		return v, nil
	case 12:
		pos, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		if pos == -1 {
			return nil, nil
		}
		if box || ctx == nil || ctx.BuildingLookup == nil {
			return BuildingBox{PosValue: pos}, nil
		}
		return ctx.BuildingLookup(pos), nil
	case 13:
		s, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		return LAccess(uint16(s)), nil
	case 14:
		l, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		return r.ReadBytes(int(l))
	case 15:
		_, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		return nil, nil
	case 16:
		l, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		bools := make([]bool, l)
		for i := 0; i < int(l); i++ {
			v, err := r.ReadBool()
			if err != nil {
				return nil, err
			}
			bools[i] = v
		}
		return bools, nil
	case 17:
		id, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		if box || ctx == nil || ctx.UnitLookup == nil {
			return UnitBox{IDValue: id}, nil
		}
		return ctx.UnitLookup(id), nil
	case 18:
		l, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		out := make([]Vec2, l)
		for i := 0; i < int(l); i++ {
			x, err := r.ReadFloat32()
			if err != nil {
				return nil, err
			}
			y, err := r.ReadFloat32()
			if err != nil {
				return nil, err
			}
			out[i] = Vec2{X: x, Y: y}
		}
		return out, nil
	case 19:
		x, err := r.ReadFloat32()
		if err != nil {
			return nil, err
		}
		y, err := r.ReadFloat32()
		if err != nil {
			return nil, err
		}
		return Vec2{X: x, Y: y}, nil
	case 20:
		b, err := r.ReadUByte()
		if err != nil {
			return nil, err
		}
		if ctx != nil && ctx.TeamLookup != nil {
			return ctx.TeamLookup(byte(b)), nil
		}
		return Team{ID: byte(b)}, nil
	case 21:
		return ReadInts(r)
	case 22:
		l, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		out := make([]any, l)
		for i := 0; i < int(l); i++ {
			o, err := ReadObject(r, box, ctx)
			if err != nil {
				return nil, err
			}
			out[i] = o
		}
		return out, nil
	case 23:
		id, err := r.ReadUint16()
		if err != nil {
			return nil, err
		}
		if ctx != nil && ctx.UnitCommandLookup != nil {
			return ctx.UnitCommandLookup(int16(id)), nil
		}
		return UnitCommand{ID: int16(id)}, nil
	default:
		return nil, ErrUnsupportedTypeIO
	}
}
