package protocol

// Packets added/changed for the Mindustry 159.7 migration.
// Wire IDs follow the official build-159.7 registration order
// (see MD/wire-protocol.md and the generator notes in tools/gen_registry.py).

// AssetRequirementStream (base packet id 4) is sent by the server to tell the
// client which external assets it must pull before the world stream. Mirrors
// mindustry.net.Packets.AssetRequirementStream (empty body).
type AssetRequirementStream struct{}

func (p *AssetRequirementStream) Read(r *Reader, _ int) error { return nil }
func (p *AssetRequirementStream) Write(w *Writer) error      { return nil }
func (p *AssetRequirementStream) Priority() int              { return PriorityNormal }

// AssetStream (base packet id 5) mirrors StreamChunk for external asset bytes.
type AssetStream struct {
	ID   int32
	Data []byte
}

func (p *AssetStream) Read(r *Reader, _ int) error {
	id, err := r.ReadInt32()
	if err != nil {
		return err
	}
	l, err := r.ReadInt16()
	if err != nil {
		return err
	}
	data, err := r.ReadBytes(int(l))
	if err != nil {
		return err
	}
	p.ID = id
	p.Data = data
	return nil
}

func (p *AssetStream) Write(w *Writer) error {
	if err := w.WriteInt32(p.ID); err != nil {
		return err
	}
	if err := w.WriteInt16(int16(len(p.Data))); err != nil {
		return err
	}
	return w.WriteBytes(p.Data)
}
func (p *AssetStream) Priority() int { return PriorityNormal }

// Remote_NetClient_playMusic (id 28, server->client): play a music track.
type Remote_NetClient_playMusic struct {
	MusicName string
	Interrupt bool
}

func (p *Remote_NetClient_playMusic) Read(r *Reader, _ int) error {
	name, err := r.ReadStringRaw()
	if err != nil {
		return err
	}
	p.MusicName = name
	if v, err := r.ReadBool(); err != nil {
		return err
	} else {
		p.Interrupt = v
	}
	return nil
}

func (p *Remote_NetClient_playMusic) Write(w *Writer) error {
	if err := w.WriteStringRaw(p.MusicName); err != nil {
		return err
	}
	return w.WriteBool(p.Interrupt)
}
func (p *Remote_NetClient_playMusic) Priority() int { return PriorityNormal }

// Remote_NetServer_requestAssets (id 52, client->server): client asks for the
// external assets it is missing, identified by their ids. The player is the
// connection's sender and is serialized as the first field.
type Remote_NetServer_requestAssets struct {
	Player Entity
	Ids    []int16
}

func (p *Remote_NetServer_requestAssets) Read(r *Reader, _ int) error {
	if v, err := ReadEntity(r, r.Ctx); err != nil {
		return err
	} else {
		p.Player = v
	}
	n, err := r.ReadInt32()
	if err != nil {
		return err
	}
	if n < 0 {
		p.Ids = nil
		return nil
	}
	p.Ids = make([]int16, n)
	for i := int32(0); i < n; i++ {
		v, err := r.ReadInt16()
		if err != nil {
			return err
		}
		p.Ids[i] = v
	}
	return nil
}

func (p *Remote_NetServer_requestAssets) Write(w *Writer) error {
	if err := WriteEntity(w, p.Player); err != nil {
		return err
	}
	if p.Ids == nil {
		return w.WriteInt32(-1)
	}
	if err := w.WriteInt32(int32(len(p.Ids))); err != nil {
		return err
	}
	for _, id := range p.Ids {
		if err := w.WriteInt16(id); err != nil {
			return err
		}
	}
	return nil
}
func (p *Remote_NetServer_requestAssets) Priority() int { return PriorityNormal }

// Remote_NetServer_requestWorld (id 55, client->server): client finished
// pulling assets and requests the world stream.
type Remote_NetServer_requestWorld struct {
	Player Entity
}

func (p *Remote_NetServer_requestWorld) Read(r *Reader, _ int) error {
	if v, err := ReadEntity(r, r.Ctx); err != nil {
		return err
	} else {
		p.Player = v
	}
	return nil
}

func (p *Remote_NetServer_requestWorld) Write(w *Writer) error {
	return WriteEntity(w, p.Player)
}
func (p *Remote_NetServer_requestWorld) Priority() int { return PriorityNormal }

// Remote_Menus_label3 (id 128, both variants): world label with a flags int,
// matching label(String,int,float,float,float,int).
type Remote_Menus_label3 struct {
	Message  any
	Id       int32
	Duration float32
	Worldx   float32
	Worldy   float32
	Flags    int32
}

func (p *Remote_Menus_label3) Read(r *Reader, _ int) error {
	if v, err := ReadObject(r, false, r.Ctx); err != nil {
		return err
	} else {
		p.Message = v
	}
	if v, err := r.ReadInt32(); err != nil {
		return err
	} else {
		p.Id = v
	}
	if v, err := r.ReadFloat32(); err != nil {
		return err
	} else {
		p.Duration = v
	}
	if v, err := r.ReadFloat32(); err != nil {
		return err
	} else {
		p.Worldx = v
	}
	if v, err := r.ReadFloat32(); err != nil {
		return err
	} else {
		p.Worldy = v
	}
	if v, err := r.ReadInt32(); err != nil {
		return err
	} else {
		p.Flags = v
	}
	return nil
}

func (p *Remote_Menus_label3) Write(w *Writer) error {
	if err := WriteObject(w, p.Message, w.Ctx); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldx); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldy); err != nil {
		return err
	}
	return w.WriteInt32(p.Flags)
}
func (p *Remote_Menus_label3) Priority() int { return PriorityNormal }

// Remote_Menus_labelReliable3 (id 131, both variants, reliable): reliable
// counterpart of label3.
type Remote_Menus_labelReliable3 struct {
	Message  any
	Id       int32
	Duration float32
	Worldx   float32
	Worldy   float32
	Flags    int32
}

func (p *Remote_Menus_labelReliable3) Read(r *Reader, _ int) error {
	if v, err := ReadObject(r, false, r.Ctx); err != nil {
		return err
	} else {
		p.Message = v
	}
	if v, err := r.ReadInt32(); err != nil {
		return err
	} else {
		p.Id = v
	}
	if v, err := r.ReadFloat32(); err != nil {
		return err
	} else {
		p.Duration = v
	}
	if v, err := r.ReadFloat32(); err != nil {
		return err
	} else {
		p.Worldx = v
	}
	if v, err := r.ReadFloat32(); err != nil {
		return err
	} else {
		p.Worldy = v
	}
	if v, err := r.ReadInt32(); err != nil {
		return err
	} else {
		p.Flags = v
	}
	return nil
}

func (p *Remote_Menus_labelReliable3) Write(w *Writer) error {
	if err := WriteObject(w, p.Message, w.Ctx); err != nil {
		return err
	}
	if err := w.WriteInt32(p.Id); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Duration); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldx); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.Worldy); err != nil {
		return err
	}
	return w.WriteInt32(p.Flags)
}
func (p *Remote_Menus_labelReliable3) Priority() int { return PriorityNormal }
