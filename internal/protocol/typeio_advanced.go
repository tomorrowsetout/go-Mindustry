package protocol

import "errors"

var ErrMissingContext = errors.New("typeio_missing_context")

func WritePayload(w *Writer, p Payload) error {
	if p == nil {
		return w.WriteBool(false)
	}
	if err := w.WriteBool(true); err != nil {
		return err
	}
	return p.WritePayload(w)
}

func ReadPayload(r *Reader, ctx *TypeIOContext) (Payload, error) {
	exists, err := r.ReadBool()
	if err != nil {
		return nil, err
	}
	if !exists {
		return nil, nil
	}
	if ctx != nil && ctx.PayloadRead != nil {
		return ctx.PayloadRead(r)
	}
	return DefaultPayloadRead(r)
}

func WriteMounts(w *Writer, mounts []WeaponMount) error {
	if err := w.WriteByte(byte(len(mounts))); err != nil {
		return err
	}
	for _, m := range mounts {
		state := byte(0)
		if m.Shoot() {
			state |= 1
		}
		if m.Rotate() {
			state |= 2
		}
		if err := w.WriteByte(state); err != nil {
			return err
		}
		if err := w.WriteFloat32(m.AimX()); err != nil {
			return err
		}
		if err := w.WriteFloat32(m.AimY()); err != nil {
			return err
		}
	}
	return nil
}

// DefaultPayloadRead reads payloads with minimal decoding and a raw fallback.
func DefaultPayloadRead(r *Reader) (Payload, error) {
	payloadType, err := r.ReadByte()
	if err != nil {
		return nil, err
	}
	switch payloadType {
	case PayloadUnit:
		classID, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if r.Ctx != nil && r.Ctx.PayloadUnitRead != nil {
			if p, err := r.Ctx.PayloadUnitRead(r, classID); err != nil {
				return nil, err
			} else if p != nil {
				return p, nil
			}
		}
		var ent UnitSyncEntity
		if r.Ctx != nil && r.Ctx.EntityFactory != nil {
			ent = r.Ctx.EntityFactory(classID)
			if ent != nil {
				ent.SetID(0)
			}
		}
		raw, err := r.ReadBytes(r.Remaining())
		if err != nil {
			return nil, err
		}
		return UnitPayload{ClassID: classID, Entity: ent, Raw: raw}, nil
	case PayloadBlock:
		blockID, err := r.ReadInt16()
		if err != nil {
			return nil, err
		}
		version, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		if r.Ctx != nil && r.Ctx.PayloadBuildRead != nil {
			if p, err := r.Ctx.PayloadBuildRead(r, blockID, version); err != nil {
				return nil, err
			} else if p != nil {
				return p, nil
			}
		}
		raw, err := r.ReadBytes(r.Remaining())
		if err != nil {
			return nil, err
		}
		return BuildPayload{BlockID: blockID, Version: version, Raw: raw}, nil
	default:
		raw, err := r.ReadBytes(r.Remaining())
		if err != nil {
			return nil, err
		}
		out := make([]byte, 0, 1+len(raw))
		out = append(out, payloadType)
		out = append(out, raw...)
		return PayloadBox{Raw: out}, nil
	}
}

func ReadMounts(r *Reader, mounts []WeaponMount) ([]WeaponMount, error) {
	n, err := r.ReadUByte()
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(n); i++ {
		state, err := r.ReadByte()
		if err != nil {
			return nil, err
		}
		ax, err := r.ReadFloat32()
		if err != nil {
			return nil, err
		}
		ay, err := r.ReadFloat32()
		if err != nil {
			return nil, err
		}
		if i <= len(mounts)-1 {
			m := mounts[i]
			m.SetAim(ax, ay)
			m.SetShoot((state & 1) != 0)
			m.SetRotate((state & 2) != 0)
		}
	}
	return mounts, nil
}

func ReadMountsSkip(r *Reader) error {
	n, err := r.ReadUByte()
	if err != nil {
		return err
	}
	skip := int(n) * (1 + 4 + 4)
	_, err = r.ReadBytes(skip)
	return err
}

func WriteAbilities(w *Writer, abilities []Ability) error {
	if err := w.WriteByte(byte(len(abilities))); err != nil {
		return err
	}
	for _, a := range abilities {
		if err := w.WriteFloat32(a.Data()); err != nil {
			return err
		}
	}
	return nil
}

func ReadAbilities(r *Reader, abilities []Ability) ([]Ability, error) {
	n, err := r.ReadUByte()
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(n); i++ {
		data, err := r.ReadFloat32()
		if err != nil {
			return nil, err
		}
		if i <= len(abilities)-1 {
			abilities[i].SetData(data)
		}
	}
	return abilities, nil
}

func ReadAbilitiesSkip(r *Reader) error {
	n, err := r.ReadUByte()
	if err != nil {
		return err
	}
	_, err = r.ReadBytes(int(n))
	return err
}

type UnitSyncContainer interface {
	Unit() UnitSyncEntity
}

type UnitSyncBox struct {
	Entity UnitSyncEntity
}

func (b UnitSyncBox) Unit() UnitSyncEntity { return b.Entity }

func WriteUnitContainer(w *Writer, cont UnitSyncContainer) error {
	if cont == nil || cont.Unit() == nil {
		return ErrUnsupportedTypeIO
	}
	u := cont.Unit()
	if err := w.WriteInt32(u.ID()); err != nil {
		return err
	}
	if err := w.WriteByte(u.ClassID()); err != nil {
		return err
	}
	u.BeforeWrite()
	return u.WriteSync(w)
}

func ReadUnitContainer(r *Reader, ctx *TypeIOContext) error {
	if ctx == nil {
		return ErrMissingContext
	}
	id, err := r.ReadInt32()
	if err != nil {
		return err
	}
	typeID, err := r.ReadUByte()
	if err != nil {
		return err
	}

	entity := UnitSyncEntity(nil)
	if ctx.EntityByID != nil {
		entity = ctx.EntityByID(id)
	}

	add := false
	created := false
	if entity == nil {
		if ctx.EntityFactory == nil {
			return ErrMissingContext
		}
		entity = ctx.EntityFactory(byte(typeID))
		if entity == nil {
			return ErrUnsupportedTypeIO
		}
		entity.SetID(id)
		if ctx.IsEntityUsed != nil && !ctx.IsEntityUsed(id) {
			add = true
		}
		created = true
	}

	if err := entity.ReadSync(r); err != nil {
		return err
	}
	if created {
		entity.SnapSync()
	}
	if add {
		if ctx.AddEntity != nil {
			entity.Add()
			ctx.AddEntity(entity)
		}
		if ctx.AddRemovedEntity != nil {
			ctx.AddRemovedEntity(id)
		}
	}
	return nil
}

// ReadUnitContainerBox reads a unit container and returns a minimal UnitSyncBox.
// This function is used when only minimal entity information is needed (e.g., for parsing).
// If full entity handling is required, use ReadUnitContainer instead.
func ReadUnitContainerBox(r *Reader, ctx *TypeIOContext) (*UnitSyncBox, error) {
	id, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	typeID, err := r.ReadUByte()
	if err != nil {
		return nil, err
	}

	// Try to get entity by ID first (for existing entities)
	var ent UnitSyncEntity
	if ctx != nil && ctx.EntityByID != nil {
		if e := ctx.EntityByID(id); e != nil {
			ent = e
		}
	}

	// If not found, try to create new entity via factory
	if ent == nil && ctx != nil && ctx.EntityFactory != nil {
		if e := ctx.EntityFactory(byte(typeID)); e != nil {
			ent = e
			ent.SetID(id)
		}
	}

	// Fallback: create EntityBox with fallback classID=0
	// This ensures the function always returns a valid entity, even if incomplete
	if ent == nil {
		ent = &EntityBox{IDValue: id}
	}

	// Read entity sync data
	if err := ent.ReadSync(r); err != nil {
		return nil, err
	}

	return &UnitSyncBox{Entity: ent}, nil
}
