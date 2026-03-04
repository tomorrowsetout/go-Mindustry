package protocol

// UnitCommandBox is a boxed unit command for TypeIO serialization (ID=15).
type UnitCommandBox struct {
	ID int16
}

// UnitStanceBox is a boxed unit stance for TypeIO serialization.
type UnitStanceBox struct {
	ID int16
}

func WriteCommand(w *Writer, cmd *UnitCommand) error {
	if cmd == nil {
		return w.WriteByte(255)
	}
	return w.WriteByte(byte(cmd.ID))
}

func ReadCommand(r *Reader, ctx *TypeIOContext) (*UnitCommand, error) {
	v, err := r.ReadUByte()
	if err != nil {
		return nil, err
	}
	if v == 255 {
		return nil, nil
	}
	id := int16(v)
	if ctx != nil && ctx.UnitCommandLookup != nil {
		c := ctx.UnitCommandLookup(id)
		return &c, nil
	}
	return &UnitCommand{ID: id}, nil
}

func WriteStance(w *Writer, stance *UnitStance) error {
	if stance == nil {
		return w.WriteByte(255)
	}
	return w.WriteByte(byte(stance.ID))
}

func ReadStance(r *Reader, ctx *TypeIOContext) (UnitStance, error) {
	v, err := r.ReadUByte()
	if err != nil {
		return UnitStance{}, err
	}
	if v == 255 {
		return UnitStance{ID: 0}, nil
	}
	id := int16(v)
	if ctx != nil && ctx.UnitStanceLookup != nil {
		return ctx.UnitStanceLookup(id), nil
	}
	return UnitStance{ID: id}, nil
}
