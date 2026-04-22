package protocol

import (
	"fmt"
	"strings"
)

// BuildPlan mirrors mindustry.entities.units.BuildPlan.
type BuildPlan struct {
	Breaking bool
	X        int32
	Y        int32
	Rotation byte
	Block    Block
	Config   any
}

// WritePlan matches TypeIO.writePlan.
func WritePlan(w *Writer, plan *BuildPlan, ctx *TypeIOContext) error {
	if plan == nil {
		return ErrUnsupportedTypeIO
	}
	if err := w.WriteByte(boolToByteLocal(plan.Breaking)); err != nil {
		return err
	}
	if err := w.WriteInt32(PackPoint2(plan.X, plan.Y)); err != nil {
		return err
	}
	if plan.Breaking {
		return nil
	}
	if err := WriteBlock(w, plan.Block); err != nil {
		return err
	}
	if err := w.WriteByte(plan.Rotation); err != nil {
		return err
	}
	if err := w.WriteByte(1); err != nil { // always has config
		return err
	}
	return WriteObject(w, plan.Config, ctx)
}

// ReadPlan matches TypeIO.readPlan (without world tile validation).
func ReadPlan(r *Reader, ctx *TypeIOContext) (*BuildPlan, error) {
	t, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	pos, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	x := int32(int16((pos >> 16) & 0xFFFF))
	y := int32(int16(pos & 0xFFFF))

	if t == 1 {
		return &BuildPlan{Breaking: true, X: x, Y: y}, nil
	}
	block, err := ReadBlock(r, ctx)
	if err != nil {
		return nil, err
	}
	rot, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	_, err = r.ReadByte() // hasConfig byte
	if err != nil {
		return nil, err
	}
	cfg, err := ReadObject(r, false, ctx)
	if err != nil {
		return nil, err
	}
	return &BuildPlan{
		Breaking: false,
		X:        x,
		Y:        y,
		Rotation: rot,
		Block:    block,
		Config:   cfg,
	}, nil
}

func WritePlans(w *Writer, plans []*BuildPlan, ctx *TypeIOContext) error {
	if plans == nil {
		return w.WriteInt16(-1)
	}
	if err := w.WriteInt16(int16(len(plans))); err != nil {
		return err
	}
	for _, p := range plans {
		if p == nil {
			continue
		}
		if err := WritePlan(w, p, ctx); err != nil {
			return err
		}
	}
	return nil
}

func ReadPlans(r *Reader, ctx *TypeIOContext) ([]*BuildPlan, error) {
	n, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if n == -1 {
		return nil, nil
	}
	out := make([]*BuildPlan, n)
	for i := 0; i < int(n); i++ {
		p, err := ReadPlan(r, ctx)
		if err != nil {
			return nil, err
		}
		out[i] = p
	}
	return out, nil
}

// Plans queue (net) capped by size.
func WritePlansQueueNet(w *Writer, plans []*BuildPlan, ctx *TypeIOContext) error {
	if plans == nil {
		return w.WriteInt32(-1)
	}
	used := getMaxPlans(plans)
	if err := w.WriteInt32(int32(used)); err != nil {
		return err
	}
	for i := 0; i < used; i++ {
		if err := WritePlan(w, plans[i], ctx); err != nil {
			return err
		}
	}
	return nil
}

func ReadPlansQueue(r *Reader, ctx *TypeIOContext) ([]*BuildPlan, error) {
	used, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if used == -1 {
		return nil, nil
	}
	if used < 0 {
		return nil, fmt.Errorf("invalid plans queue size: %d", used)
	}
	const maxPlansQueue = 4096
	if used > maxPlansQueue {
		return nil, fmt.Errorf("plans queue too large: %d", used)
	}
	out := make([]*BuildPlan, used)
	for i := 0; i < int(used); i++ {
		p, err := ReadPlan(r, ctx)
		if err != nil {
			return nil, err
		}
		out[i] = p
	}
	return out, nil
}

// Client preview plans use Mindustry's compact TypeIO.writeClientPlans format.
func WriteClientPlans(w *Writer, plans []*BuildPlan, ctx *TypeIOContext) error {
	if plans == nil {
		return w.WriteInt16(0)
	}
	compact := make([]*BuildPlan, 0, len(plans))
	for _, plan := range plans {
		if plan == nil || plan.Breaking || plan.Block == nil {
			continue
		}
		compact = append(compact, plan)
	}
	if err := w.WriteInt16(int16(len(compact))); err != nil {
		return err
	}
	for _, plan := range compact {
		if err := w.WriteUint16(uint16(plan.X)); err != nil {
			return err
		}
		if err := w.WriteUint16(uint16(plan.Y)); err != nil {
			return err
		}
		if err := w.WriteInt16(plan.Block.ID()); err != nil {
			return err
		}
		if clientPlanBlockRotates(plan.Block) {
			if err := w.WriteByte(plan.Rotation); err != nil {
				return err
			}
		}
		if err := WriteClientPlanConfig(w, plan.Config, ctx); err != nil {
			return err
		}
	}
	return nil
}

func ReadClientPlans(r *Reader, ctx *TypeIOContext) ([]*BuildPlan, error) {
	amount, err := r.ReadInt16()
	if err != nil {
		return nil, err
	}
	if amount == 0 {
		return nil, nil
	}
	if amount < 0 {
		return nil, nil
	}
	payload, err := r.ReadBytes(r.Remaining())
	if err != nil {
		return nil, err
	}
	plans, used, ok := readClientPlansPayload(payload, int(amount), ctx)
	if !ok {
		return nil, fmt.Errorf("failed to parse %d client plans", amount)
	}
	if used != len(payload) {
		return nil, fmt.Errorf("unexpected trailing client plan bytes: %d", len(payload)-used)
	}
	return plans, nil
}

func readClientPlansPayload(payload []byte, amount int, ctx *TypeIOContext) ([]*BuildPlan, int, bool) {
	if amount == 0 {
		return nil, 0, true
	}

	var parse func(offset int, index int) ([]*BuildPlan, int, bool)
	parse = func(offset int, index int) ([]*BuildPlan, int, bool) {
		if index == amount {
			return nil, offset, offset == len(payload)
		}
		rd := NewReaderWithContext(payload[offset:], ctx)
		x, err := rd.ReadUint16()
		if err != nil {
			return nil, 0, false
		}
		y, err := rd.ReadUint16()
		if err != nil {
			return nil, 0, false
		}
		blockID, err := rd.ReadInt16()
		if err != nil {
			return nil, 0, false
		}

		var block Block = BlockRef{BlkID: blockID}
		if ctx != nil && ctx.BlockLookup != nil {
			if resolved := ctx.BlockLookup(blockID); resolved != nil {
				block = resolved
			}
		}

		rotationChoices := []bool{clientPlanBlockRotates(block)}
		if _, ok := block.(BlockRef); ok {
			rotationChoices = []bool{false, true}
		}

		baseUsed := offset + 6
		for _, hasRotation := range rotationChoices {
			localOffset := baseUsed
			rotation := byte(0)
			if hasRotation {
				if localOffset >= len(payload) {
					continue
				}
				rotation = payload[localOffset]
				localOffset++
			}

			cfg, cfgUsed, ok := tryReadClientPlanConfigPayload(payload[localOffset:], ctx)
			if !ok {
				continue
			}
			nextPlans, used, ok := parse(localOffset+cfgUsed, index+1)
			if !ok {
				continue
			}
			plan := &BuildPlan{
				X:        int32(x),
				Y:        int32(y),
				Rotation: rotation,
				Block:    block,
				Config:   cfg,
			}
			return append([]*BuildPlan{plan}, nextPlans...), used, true
		}
		return nil, 0, false
	}

	return parse(0, 0)
}

func tryReadClientPlanConfigPayload(payload []byte, ctx *TypeIOContext) (any, int, bool) {
	rd := NewReaderWithContext(payload, ctx)
	value, err := ReadClientPlanConfig(rd, ctx)
	if err != nil {
		return nil, 0, false
	}
	return value, len(payload) - rd.Remaining(), true
}

func ReadClientPlanConfig(r *Reader, ctx *TypeIOContext) (any, error) {
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
	case 10:
		return r.ReadBool()
	case 11:
		return r.ReadFloat64()
	default:
		return nil, fmt.Errorf("unknown client plan config object type: %d", t)
	}
}

func WriteClientPlanConfig(w *Writer, value any, ctx *TypeIOContext) error {
	switch v := value.(type) {
	case nil:
		return w.WriteByte(0)
	case int:
		if err := w.WriteByte(1); err != nil {
			return err
		}
		return w.WriteInt32(int32(v))
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
	case Content:
		if err := w.WriteByte(5); err != nil {
			return err
		}
		if err := w.WriteByte(byte(v.ContentType())); err != nil {
			return err
		}
		return w.WriteInt16(v.ID())
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
	default:
		_ = ctx
		return w.WriteByte(0)
	}
}

func clientPlanBlockRotates(block Block) bool {
	if block == nil {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(block.Name()))
	if name == "" {
		return false
	}
	switch {
	case strings.Contains(name, "conveyor"),
		strings.Contains(name, "junction"),
		strings.Contains(name, "router"),
		strings.Contains(name, "sorter"),
		strings.Contains(name, "gate"),
		strings.Contains(name, "bridge"),
		strings.Contains(name, "loader"),
		strings.Contains(name, "unloader"),
		strings.Contains(name, "source"),
		strings.Contains(name, "void"),
		strings.Contains(name, "pump"),
		strings.Contains(name, "drill"),
		strings.Contains(name, "turret"),
		strings.Contains(name, "factory"),
		strings.Contains(name, "reconstructor"),
		strings.Contains(name, "assembler"),
		strings.Contains(name, "projector"),
		strings.Contains(name, "mender"),
		strings.Contains(name, "door"),
		strings.Contains(name, "wall"),
		strings.Contains(name, "payload"),
		strings.Contains(name, "node"),
		strings.Contains(name, "driver"),
		strings.Contains(name, "cannon"),
		strings.Contains(name, "launch"),
		strings.Contains(name, "foreshadow"),
		strings.Contains(name, "duo"),
		strings.Contains(name, "scatter"),
		strings.Contains(name, "scorch"),
		strings.Contains(name, "hail"),
		strings.Contains(name, "lancer"),
		strings.Contains(name, "arc"),
		strings.Contains(name, "salvo"),
		strings.Contains(name, "swarmer"),
		strings.Contains(name, "cyclone"),
		strings.Contains(name, "spectre"),
		strings.Contains(name, "meltdown"),
		strings.Contains(name, "parallax"),
		strings.Contains(name, "segment"),
		strings.Contains(name, "tsunami"),
		strings.Contains(name, "fuse"),
		strings.Contains(name, "ripple"):
		return true
	default:
		return false
	}
}

func getMaxPlans(plans []*BuildPlan) int {
	used := len(plans)
	if used > 20 {
		used = 20
	}
	totalLength := 0
	for i := 0; i < used; i++ {
		if plans[i] == nil {
			continue
		}
		switch v := plans[i].Config.(type) {
		case []byte:
			totalLength += len(v)
		case string:
			totalLength += len(v)
		}
		if totalLength > 500 {
			return i + 1
		}
	}
	return used
}

func boolToByteLocal(v bool) byte {
	if v {
		return 1
	}
	return 0
}
