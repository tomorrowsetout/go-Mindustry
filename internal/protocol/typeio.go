package protocol

import (
	"errors"
	"fmt"
	"math"
)

// TypeIO mirrors mindustry.io.TypeIO. Content/object mapping is handled via
// TypeIOContext and raw JSON strings for rules/objectives.

var ErrUnsupportedTypeIO = errors.New("typeio_unsupported")

func WriteString(w *Writer, s *string) error {
	return w.WriteStringNullable(s)
}

func ReadString(r *Reader) (*string, error) {
	return r.ReadStringNullable()
}

func WriteBytes(w *Writer, b []byte) error {
	if err := w.WriteInt16(int16(len(b))); err != nil {
		return err
	}
	return w.WriteBytes(b)
}

func ReadBytes(r *Reader) ([]byte, error) {
	l, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if l < 0 {
		return nil, nil
	}
	return r.ReadBytes(int(l))
}

func WriteStrings(w *Writer, strings []string, maxLen int) error {
	n := len(strings)
	if maxLen > 0 && n > maxLen {
		n = maxLen
	}
	if err := w.WriteByte(byte(n)); err != nil {
		return err
	}
	for i := 0; i < n; i++ {
		s := strings[i]
		if err := WriteString(w, &s); err != nil {
			return err
		}
	}
	return nil
}

func ReadStrings(r *Reader) ([]string, error) {
	n, err := r.ReadUByte()
	if err != nil {
		return nil, err
	}
	out := make([]string, n)
	for i := 0; i < int(n); i++ {
		s, err := ReadString(r)
		if err != nil {
			return nil, err
		}
		if s != nil {
			out[i] = *s
		}
	}
	return out, nil
}

func WriteTraceInfo(w *Writer, t TraceInfo) error {
	if err := WriteString(w, &t.IP); err != nil {
		return err
	}
	if err := WriteString(w, &t.UUID); err != nil {
		return err
	}
	if err := WriteString(w, &t.Locale); err != nil {
		return err
	}
	if err := w.WriteByte(boolToByteLocal(t.Modded)); err != nil {
		return err
	}
	if err := w.WriteByte(boolToByteLocal(t.Mobile)); err != nil {
		return err
	}
	if err := w.WriteInt32(t.TimesJoined); err != nil {
		return err
	}
	if err := w.WriteInt32(t.TimesKicked); err != nil {
		return err
	}
	if err := WriteStrings(w, t.IPs, 12); err != nil {
		return err
	}
	return WriteStrings(w, t.Names, 12)
}

func ReadTraceInfo(r *Reader) (TraceInfo, error) {
	ip, err := ReadString(r)
	if err != nil {
		return TraceInfo{}, err
	}
	uuid, err := ReadString(r)
	if err != nil {
		return TraceInfo{}, err
	}
	locale, err := ReadString(r)
	if err != nil {
		return TraceInfo{}, err
	}
	modded, err := r.ReadBool()
	if err != nil {
		return TraceInfo{}, err
	}
	mobile, err := r.ReadBool()
	if err != nil {
		return TraceInfo{}, err
	}
	joined, err := r.ReadInt32()
	if err != nil {
		return TraceInfo{}, err
	}
	kicked, err := r.ReadInt32()
	if err != nil {
		return TraceInfo{}, err
	}
	ips, err := ReadStrings(r)
	if err != nil {
		return TraceInfo{}, err
	}
	names, err := ReadStrings(r)
	if err != nil {
		return TraceInfo{}, err
	}
	out := TraceInfo{
		Modded:      modded,
		Mobile:      mobile,
		TimesJoined: joined,
		TimesKicked: kicked,
		IPs:         ips,
		Names:       names,
	}
	if ip != nil {
		out.IP = *ip
	}
	if uuid != nil {
		out.UUID = *uuid
	}
	if locale != nil {
		out.Locale = *locale
	}
	return out, nil
}

func WriteInts(w *Writer, ints []int32) error {
	if err := w.WriteInt16(int16(len(ints))); err != nil {
		return err
	}
	for _, v := range ints {
		if err := w.WriteInt32(v); err != nil {
			return err
		}
	}
	return nil
}

func ReadInts(r *Reader) ([]int32, error) {
	l, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if l < 0 {
		return nil, nil
	}
	out := make([]int32, l)
	for i := 0; i < int(l); i++ {
		v, err := r.ReadInt32()
		if err != nil {
			return nil, err
		}
		out[i] = v
	}
	return out, nil
}

func WriteIntSeq(w *Writer, seq IntSeq) error {
	if err := w.WriteInt32(int32(len(seq.Items))); err != nil {
		return err
	}
	for _, v := range seq.Items {
		if err := w.WriteInt32(v); err != nil {
			return err
		}
	}
	return nil
}

func ReadIntSeq(r *Reader) (IntSeq, error) {
	l, err := r.ReadInt32()
	if err != nil {
		return IntSeq{}, err
	}
	items := make([]int32, l)
	for i := 0; i < int(l); i++ {
		v, err := r.ReadInt32()
		if err != nil {
			return IntSeq{}, err
		}
		items[i] = v
	}
	return IntSeq{Items: items}, nil
}

func WriteContent(w *Writer, c Content) error {
	if c == nil {
		return w.WriteByte(0)
	}
	if err := w.WriteByte(byte(c.ContentType())); err != nil {
		return err
	}
	return w.WriteInt16(c.ID())
}

func ReadContent(r *Reader, ctx *TypeIOContext) (Content, error) {
	t, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	id, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if ctx != nil && ctx.Content != nil {
		if c := ctx.Content.Get(ContentType(t), id); c != nil {
			return c, nil
		}
	}
	return contentBox{typ: ContentType(t), id: id}, nil
}

func WriteUnit(w *Writer, u Unit) error {
	if u == nil {
		if err := w.WriteByte(0); err != nil {
			return err
		}
		return w.WriteInt32(0)
	}
	if bu, ok := u.(BlockUnit); ok {
		if err := w.WriteByte(1); err != nil {
			return err
		}
		tile := bu.Tile()
		if tile == nil {
			return w.WriteInt32(0)
		}
		return w.WriteInt32(tile.Pos())
	}
	if err := w.WriteByte(2); err != nil {
		return err
	}
	return w.WriteInt32(u.ID())
}

func ReadUnit(r *Reader, ctx *TypeIOContext) (Unit, error) {
	typ, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	id, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if typ == 0 {
		return nil, nil
	}
	if typ == 1 {
		if ctx != nil && ctx.BuildingLookup != nil {
			if b := ctx.BuildingLookup(id); b != nil {
				if cb, ok := b.(ControlBlock); ok {
					return cb.Unit(), nil
				}
			}
		}
		return nil, nil
	}
	if ctx != nil && ctx.UnitLookup != nil {
		if u := ctx.UnitLookup(id); u != nil {
			return u, nil
		}
	}
	return UnitBox{IDValue: id}, nil
}

func WriteBuilding(w *Writer, b Building) error {
	if b == nil {
		return w.WriteInt32(-1)
	}
	return w.WriteInt32(b.Pos())
}

func ReadBuilding(r *Reader, ctx *TypeIOContext) (Building, error) {
	pos, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if pos == -1 {
		return nil, nil
	}
	if ctx != nil && ctx.BuildingLookup != nil {
		return ctx.BuildingLookup(pos), nil
	}
	return BuildingBox{PosValue: pos}, nil
}

func WriteTile(w *Writer, t Tile) error {
	if t == nil {
		return w.WriteInt32(PackPoint2(-1, -1))
	}
	return w.WriteInt32(t.Pos())
}

func ReadTile(r *Reader, ctx *TypeIOContext) (Tile, error) {
	pos, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if pos == PackPoint2(-1, -1) {
		return nil, nil
	}
	if ctx != nil && ctx.BuildingLookup != nil {
		if b := ctx.BuildingLookup(pos); b != nil {
			return b, nil
		}
	}
	return TileBox{PosValue: pos}, nil
}

func WriteEntity(w *Writer, e Entity) error {
	if e == nil {
		return w.WriteInt32(-1)
	}
	return w.WriteInt32(e.ID())
}

func ReadEntity(r *Reader, ctx *TypeIOContext) (Entity, error) {
	id, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if id == -1 {
		return nil, nil
	}
	if ctx != nil && ctx.EntityByID != nil {
		return ctx.EntityByID(id), nil
	}
	return &EntityBox{IDValue: id}, nil
}

func WriteVec2(w *Writer, v Vec2) error {
	if err := w.WriteFloat32(v.X); err != nil {
		return err
	}
	return w.WriteFloat32(v.Y)
}

func ReadVec2Into(r *Reader, base *Vec2) (Vec2, error) {
	x, err := r.ReadFloat32()
	if err != nil {
		return Vec2{}, err
	}
	y, err := r.ReadFloat32()
	if err != nil {
		return Vec2{}, err
	}
	if base == nil {
		return Vec2{X: x, Y: y}, nil
	}
	base.X = x
	base.Y = y
	return *base, nil
}

func ReadVec2(r *Reader) (Vec2, error) {
	x, err := r.ReadFloat32()
	if err != nil {
		return Vec2{}, err
	}
	y, err := r.ReadFloat32()
	if err != nil {
		return Vec2{}, err
	}
	return Vec2{X: x, Y: y}, nil
}

func WritePoint2(w *Writer, p Point2) error {
	if err := w.WriteInt32(p.X); err != nil {
		return err
	}
	return w.WriteInt32(p.Y)
}

func ReadPoint2(r *Reader) (Point2, error) {
	x, err := r.ReadInt32()
	if err != nil {
		return Point2{}, err
	}
	y, err := r.ReadInt32()
	if err != nil {
		return Point2{}, err
	}
	return Point2{X: x, Y: y}, nil
}

func WriteTeam(w *Writer, team *Team) error {
	if team == nil {
		return w.WriteByte(0)
	}
	return w.WriteByte(team.ID)
}

func ReadTeam(r *Reader, ctx *TypeIOContext) (Team, error) {
	b, err := r.ReadUByte()
	if err != nil {
		return Team{}, err
	}
	if ctx != nil && ctx.TeamLookup != nil {
		return ctx.TeamLookup(byte(b)), nil
	}
	return Team{ID: byte(b)}, nil
}

func WriteColor(w *Writer, c Color) error {
	return w.WriteInt32(c.RGBA)
}

func ReadColorInto(r *Reader, base *Color) (Color, error) {
	v, err := r.ReadInt32()
	if err != nil {
		return Color{}, err
	}
	if base == nil {
		return Color{RGBA: v}, nil
	}
	base.RGBA = v
	return *base, nil
}

func ReadColor(r *Reader) (Color, error) {
	v, err := r.ReadInt32()
	if err != nil {
		return Color{}, err
	}
	return Color{RGBA: v}, nil
}

func WriteItem(w *Writer, item Item) error {
	if item == nil {
		return w.WriteInt16(-1)
	}
	return w.WriteInt16(item.ID())
}

func ReadItem(r *Reader, ctx *TypeIOContext) (Item, error) {
	id, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if id == -1 {
		return nil, nil
	}
	if ctx != nil && ctx.ItemLookup != nil {
		if it := ctx.ItemLookup(id); it != nil {
			return it, nil
		}
	}
	return contentBox{typ: ContentItem, id: id}, nil
}

func WriteLiquid(w *Writer, liq Liquid) error {
	if liq == nil {
		return w.WriteInt16(-1)
	}
	return w.WriteInt16(liq.ID())
}

func ReadLiquid(r *Reader, ctx *TypeIOContext) (Liquid, error) {
	id, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if id == -1 {
		return nil, nil
	}
	if ctx != nil && ctx.LiquidLookup != nil {
		if liq := ctx.LiquidLookup(id); liq != nil {
			return liq, nil
		}
	}
	return contentBox{typ: ContentLiquid, id: id}, nil
}

func WriteBlock(w *Writer, block Block) error {
	if block == nil {
		return w.WriteInt16(-1)
	}
	return w.WriteInt16(block.ID())
}

func ReadBlock(r *Reader, ctx *TypeIOContext) (Block, error) {
	id, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if id == -1 {
		return nil, nil
	}
	if ctx != nil && ctx.BlockLookup != nil {
		if b := ctx.BlockLookup(id); b != nil {
			return b, nil
		}
	}
	return contentBox{typ: ContentBlock, id: id}, nil
}

func WriteUnitType(w *Writer, t UnitType) error {
	if t == nil {
		return w.WriteInt16(-1)
	}
	return w.WriteInt16(t.ID())
}

func ReadUnitType(r *Reader, ctx *TypeIOContext) (UnitType, error) {
	id, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if id == -1 {
		return nil, nil
	}
	if ctx != nil && ctx.UnitTypeLookup != nil {
		if u := ctx.UnitTypeLookup(id); u != nil {
			return u, nil
		}
	}
	return contentBox{typ: ContentUnit, id: id}, nil
}

func WriteBulletType(w *Writer, t BulletType) error {
	if t == nil {
		return w.WriteInt16(-1)
	}
	return w.WriteInt16(t.ID())
}

func ReadBulletType(r *Reader, ctx *TypeIOContext) (BulletType, error) {
	id, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if id == -1 {
		return nil, nil
	}
	if ctx != nil && ctx.BulletTypeLookup != nil {
		if b := ctx.BulletTypeLookup(id); b != nil {
			return b, nil
		}
	}
	return contentBox{typ: ContentBullet, id: id}, nil
}

func WriteStatusEffect(w *Writer, s StatusEffect) error {
	if s == nil {
		return w.WriteInt16(-1)
	}
	return w.WriteInt16(s.ID())
}

func ReadStatusEffect(r *Reader, ctx *TypeIOContext) (StatusEffect, error) {
	id, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if id == -1 {
		return nil, nil
	}
	if ctx != nil && ctx.StatusEffectLookup != nil {
		return ctx.StatusEffectLookup(id), nil
	}
	return statusEffectBox{id: id}, nil
}

func WriteEffect(w *Writer, e Effect) error {
	return w.WriteInt16(e.ID)
}

func ReadEffect(r *Reader, ctx *TypeIOContext) (Effect, error) {
	id, err := r.ReadInt16()
	if err != nil {
		return Effect{}, err
	}
	if ctx != nil && ctx.EffectLookup != nil {
		return ctx.EffectLookup(id), nil
	}
	return Effect{ID: id}, nil
}

func WriteWeather(w *Writer, wth Weather) error {
	if wth == nil {
		return w.WriteInt16(-1)
	}
	return w.WriteInt16(wth.ID())
}

func ReadWeather(r *Reader, ctx *TypeIOContext) (Weather, error) {
	id, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if id == -1 {
		return nil, nil
	}
	if ctx != nil && ctx.WeatherLookup != nil {
		if wth := ctx.WeatherLookup(id); wth != nil {
			return wth, nil
		}
	}
	return contentBox{typ: ContentWeather, id: id}, nil
}

func WriteSound(w *Writer, s Sound) error {
	return w.WriteInt16(s.ID)
}

func ReadSound(r *Reader, ctx *TypeIOContext) (Sound, error) {
	id, err := r.ReadInt16()
	if err != nil {
		return Sound{}, err
	}
	if ctx != nil && ctx.SoundLookup != nil {
		return ctx.SoundLookup(id), nil
	}
	return Sound{ID: id}, nil
}

func WriteItems(w *Writer, s ItemStack) error {
	if err := WriteItem(w, s.Item); err != nil {
		return err
	}
	return w.WriteInt32(s.Amount)
}

func ReadItems(r *Reader, ctx *TypeIOContext) (ItemStack, error) {
	item, err := ReadItem(r, ctx)
	if err != nil {
		return ItemStack{}, err
	}
	amt, err := r.ReadInt32()
	if err != nil {
		return ItemStack{}, err
	}
	return ItemStack{Item: item, Amount: amt}, nil
}

func ReadItemsInto(r *Reader, ctx *TypeIOContext, base *ItemStack) (ItemStack, error) {
	item, err := ReadItem(r, ctx)
	if err != nil {
		return ItemStack{}, err
	}
	amt, err := r.ReadInt32()
	if err != nil {
		return ItemStack{}, err
	}
	if base == nil {
		return ItemStack{Item: item, Amount: amt}, nil
	}
	base.Item = item
	base.Amount = amt
	return *base, nil
}
func WriteItemStacks(w *Writer, stacks []ItemStack) error {
	if err := w.WriteInt16(int16(len(stacks))); err != nil {
		return err
	}
	for _, s := range stacks {
		if err := WriteItems(w, s); err != nil {
			return err
		}
	}
	return nil
}

func ReadItemStacks(r *Reader, ctx *TypeIOContext) ([]ItemStack, error) {
	n, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, fmt.Errorf("invalid item stack length: %d", n)
	}
	if n > 4096 {
		return nil, fmt.Errorf("item stack length too large: %d", n)
	}
	out := make([]ItemStack, int(n))
	for i := 0; i < int(n); i++ {
		s, err := ReadItems(r, ctx)
		if err != nil {
			return nil, err
		}
		out[i] = s
	}
	return out, nil
}

func WriteLiquidStacks(w *Writer, stacks []LiquidStack) error {
	if err := w.WriteInt16(int16(len(stacks))); err != nil {
		return err
	}
	for _, s := range stacks {
		if err := WriteLiquid(w, s.Liquid); err != nil {
			return err
		}
		if err := w.WriteFloat32(s.Amount); err != nil {
			return err
		}
	}
	return nil
}

func ReadLiquidStacks(r *Reader, ctx *TypeIOContext) ([]LiquidStack, error) {
	n, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if n < 0 {
		return nil, fmt.Errorf("invalid liquid stack length: %d", n)
	}
	if n > 4096 {
		return nil, fmt.Errorf("liquid stack length too large: %d", n)
	}
	out := make([]LiquidStack, int(n))
	for i := 0; i < int(n); i++ {
		liq, err := ReadLiquid(r, ctx)
		if err != nil {
			return nil, err
		}
		amt, err := r.ReadFloat32()
		if err != nil {
			return nil, err
		}
		out[i] = LiquidStack{Liquid: liq, Amount: amt}
	}
	return out, nil
}

func WriteRules(w *Writer, rules Rules) error {
	b := []byte(rules.Raw)
	if err := w.WriteInt32(int32(len(b))); err != nil {
		return err
	}
	return w.WriteBytes(b)
}

func ReadRules(r *Reader) (Rules, error) {
	l, err := r.ReadInt32()
	if err != nil {
		return Rules{}, err
	}
	b, err := r.ReadBytes(int(l))
	if err != nil {
		return Rules{}, err
	}
	return Rules{Raw: string(b)}, nil
}

func WriteObjectives(w *Writer, obj MapObjectives) error {
	b := []byte(obj.Raw)
	if err := w.WriteInt32(int32(len(b))); err != nil {
		return err
	}
	return w.WriteBytes(b)
}

func ReadObjectives(r *Reader) (MapObjectives, error) {
	l, err := r.ReadInt32()
	if err != nil {
		return MapObjectives{}, err
	}
	b, err := r.ReadBytes(int(l))
	if err != nil {
		return MapObjectives{}, err
	}
	return MapObjectives{Raw: string(b)}, nil
}

func WriteObjectiveMarker(w *Writer, marker ObjectiveMarker) error {
	b := []byte(marker.Raw)
	if err := w.WriteInt32(int32(len(b))); err != nil {
		return err
	}
	return w.WriteBytes(b)
}

func ReadObjectiveMarker(r *Reader) (ObjectiveMarker, error) {
	l, err := r.ReadInt32()
	if err != nil {
		return ObjectiveMarker{}, err
	}
	b, err := r.ReadBytes(int(l))
	if err != nil {
		return ObjectiveMarker{}, err
	}
	return ObjectiveMarker{Raw: string(b)}, nil
}

func WriteVecNullable(w *Writer, v *Vec2) error {
	if v == nil {
		if err := w.WriteFloat32(float32(math.NaN())); err != nil {
			return err
		}
		return w.WriteFloat32(float32(math.NaN()))
	}
	if err := w.WriteFloat32(v.X); err != nil {
		return err
	}
	return w.WriteFloat32(v.Y)
}

func ReadVecNullable(r *Reader) (*Vec2, error) {
	x, err := r.ReadFloat32()
	if err != nil {
		return nil, err
	}
	y, err := r.ReadFloat32()
	if err != nil {
		return nil, err
	}
	if math.IsNaN(float64(x)) || math.IsNaN(float64(y)) {
		return nil, nil
	}
	return &Vec2{X: x, Y: y}, nil
}

func WriteStatus(w *Writer, e StatusEntry) error {
	if err := WriteStatusEffect(w, e.Effect); err != nil {
		return err
	}
	if err := w.WriteFloat32(e.Time); err != nil {
		return err
	}
	if e.Effect != nil && e.Effect.Dynamic() {
		flags := 0
		if e.DamageMultiplier != 1 {
			flags |= 1 << 0
		}
		if e.HealthMultiplier != 1 {
			flags |= 1 << 1
		}
		if e.SpeedMultiplier != 1 {
			flags |= 1 << 2
		}
		if e.ReloadMultiplier != 1 {
			flags |= 1 << 3
		}
		if e.BuildSpeedMultiplier != 1 {
			flags |= 1 << 4
		}
		if e.DragMultiplier != 1 {
			flags |= 1 << 5
		}
		if e.ArmorOverride >= 0 {
			flags |= 1 << 6
		}
		if err := w.WriteByte(byte(flags)); err != nil {
			return err
		}
		if e.DamageMultiplier != 1 {
			if err := w.WriteFloat32(e.DamageMultiplier); err != nil {
				return err
			}
		}
		if e.HealthMultiplier != 1 {
			if err := w.WriteFloat32(e.HealthMultiplier); err != nil {
				return err
			}
		}
		if e.SpeedMultiplier != 1 {
			if err := w.WriteFloat32(e.SpeedMultiplier); err != nil {
				return err
			}
		}
		if e.ReloadMultiplier != 1 {
			if err := w.WriteFloat32(e.ReloadMultiplier); err != nil {
				return err
			}
		}
		if e.BuildSpeedMultiplier != 1 {
			if err := w.WriteFloat32(e.BuildSpeedMultiplier); err != nil {
				return err
			}
		}
		if e.DragMultiplier != 1 {
			if err := w.WriteFloat32(e.DragMultiplier); err != nil {
				return err
			}
		}
		if e.ArmorOverride >= 0 {
			if err := w.WriteFloat32(e.ArmorOverride); err != nil {
				return err
			}
		}
	}
	return nil
}

func ReadStatus(r *Reader, ctx *TypeIOContext) (StatusEntry, error) {
	eff, err := ReadStatusEffect(r, ctx)
	if err != nil {
		return StatusEntry{}, err
	}
	t, err := r.ReadFloat32()
	if err != nil {
		return StatusEntry{}, err
	}
	result := StatusEntry{Effect: eff, Time: t}
	if result.Effect != nil && result.Effect.Dynamic() {
		flags, err := r.ReadByte()
		if err != nil {
			return StatusEntry{}, err
		}
		if (flags & (1 << 0)) != 0 {
			if result.DamageMultiplier, err = r.ReadFloat32(); err != nil {
				return StatusEntry{}, err
			}
		}
		if (flags & (1 << 1)) != 0 {
			if result.HealthMultiplier, err = r.ReadFloat32(); err != nil {
				return StatusEntry{}, err
			}
		}
		if (flags & (1 << 2)) != 0 {
			if result.SpeedMultiplier, err = r.ReadFloat32(); err != nil {
				return StatusEntry{}, err
			}
		}
		if (flags & (1 << 3)) != 0 {
			if result.ReloadMultiplier, err = r.ReadFloat32(); err != nil {
				return StatusEntry{}, err
			}
		}
		if (flags & (1 << 4)) != 0 {
			if result.BuildSpeedMultiplier, err = r.ReadFloat32(); err != nil {
				return StatusEntry{}, err
			}
		}
		if (flags & (1 << 5)) != 0 {
			if result.DragMultiplier, err = r.ReadFloat32(); err != nil {
				return StatusEntry{}, err
			}
		}
		if (flags & (1 << 6)) != 0 {
			if result.ArmorOverride, err = r.ReadFloat32(); err != nil {
				return StatusEntry{}, err
			}
		}
	}
	return result, nil
}

func WriteStringArray(w *Writer, rows [][]string) error {
	if err := w.WriteByte(byte(len(rows))); err != nil {
		return err
	}
	for _, row := range rows {
		if err := w.WriteByte(byte(len(row))); err != nil {
			return err
		}
		for _, s := range row {
			val := s
			if err := WriteString(w, &val); err != nil {
				return err
			}
		}
	}
	return nil
}

func ReadStringArray(r *Reader) ([][]string, error) {
	rows, err := r.ReadUByte()
	if err != nil {
		return nil, err
	}
	out := make([][]string, rows)
	for i := 0; i < int(rows); i++ {
		cols, err := r.ReadUByte()
		if err != nil {
			return nil, err
		}
		out[i] = make([]string, cols)
		for j := 0; j < int(cols); j++ {
			s, err := ReadString(r)
			if err != nil {
				return nil, err
			}
			if s != nil {
				out[i][j] = *s
			}
		}
	}
	return out, nil
}

func WriteAction(w *Writer, a AdminAction) error {
	return w.WriteByte(byte(a))
}

func ReadAction(r *Reader) (AdminAction, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	return AdminAction(b), nil
}

func WriteKick(w *Writer, k KickReason) error {
	return w.WriteByte(byte(k))
}

func ReadKick(r *Reader) (KickReason, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	return KickReason(b), nil
}

func WriteMarkerControl(w *Writer, c LMarkerControl) error {
	return w.WriteByte(byte(c))
}

func ReadMarkerControl(r *Reader) (LMarkerControl, error) {
	b, err := r.ReadByte()
	if err != nil {
		return 0, err
	}
	return LMarkerControl(b), nil
}

// TypeIO parity notes:
// - Content mapping is delegated to TypeIOContext lookups.
// - Rules/Objectives/ObjectiveMarker are encoded as raw JSON strings.
