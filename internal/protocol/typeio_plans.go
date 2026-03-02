package protocol

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
