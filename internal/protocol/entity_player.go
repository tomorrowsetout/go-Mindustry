package protocol

// PlayerEntity is a minimal UnitSyncEntity implementation for player sync.
type PlayerEntity struct {
	IDValue          int32
	Admin            bool
	Boosting         bool
	ColorRGBA        int32
	MouseX           float32
	MouseY           float32
	Name             string
	SelectedBlock    int16
	SelectedRotation int32
	Shooting         bool
	TeamID           byte
	Typing           bool
	Unit             Unit
	X                float32
	Y                float32
}

func (p *PlayerEntity) ID() int32      { return p.IDValue }
func (p *PlayerEntity) SetID(id int32) { p.IDValue = id }
func (p *PlayerEntity) ClassID() byte  { return 12 }
func (p *PlayerEntity) BeforeWrite()   {}
func (p *PlayerEntity) SnapSync()      {}
func (p *PlayerEntity) Add()           {}

func (p *PlayerEntity) WriteSync(w *Writer) error {
	if err := w.WriteBool(p.Admin); err != nil {
		return err
	}
	if err := w.WriteBool(p.Boosting); err != nil {
		return err
	}
	if err := WriteColor(w, Color{RGBA: p.ColorRGBA}); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.MouseX); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.MouseY); err != nil {
		return err
	}
	name := p.Name
	if err := WriteString(w, &name); err != nil {
		return err
	}
	if err := w.WriteInt16(p.SelectedBlock); err != nil {
		return err
	}
	if err := w.WriteInt32(p.SelectedRotation); err != nil {
		return err
	}
	if err := w.WriteBool(p.Shooting); err != nil {
		return err
	}
	if err := WriteTeam(w, &Team{ID: p.TeamID}); err != nil {
		return err
	}
	if err := w.WriteBool(p.Typing); err != nil {
		return err
	}
	if err := WriteUnit(w, p.Unit); err != nil {
		return err
	}
	if err := w.WriteFloat32(p.X); err != nil {
		return err
	}
	return w.WriteFloat32(p.Y)
}

func (p *PlayerEntity) ReadSync(r *Reader) error {
	var err error
	if p.Admin, err = r.ReadBool(); err != nil {
		return err
	}
	if p.Boosting, err = r.ReadBool(); err != nil {
		return err
	}
	col, err := ReadColor(r)
	if err != nil {
		return err
	}
	p.ColorRGBA = col.RGBA
	if p.MouseX, err = r.ReadFloat32(); err != nil {
		return err
	}
	if p.MouseY, err = r.ReadFloat32(); err != nil {
		return err
	}
	name, err := ReadString(r)
	if err != nil {
		return err
	}
	if name != nil {
		p.Name = *name
	}
	if p.SelectedBlock, err = r.ReadInt16(); err != nil {
		return err
	}
	if p.SelectedRotation, err = r.ReadInt32(); err != nil {
		return err
	}
	if p.Shooting, err = r.ReadBool(); err != nil {
		return err
	}
	team, err := ReadTeam(r, nil)
	if err != nil {
		return err
	}
	p.TeamID = team.ID
	if p.Typing, err = r.ReadBool(); err != nil {
		return err
	}
	unit, err := ReadUnit(r, nil)
	if err != nil {
		return err
	}
	p.Unit = unit
	if p.X, err = r.ReadFloat32(); err != nil {
		return err
	}
	if p.Y, err = r.ReadFloat32(); err != nil {
		return err
	}
	return nil
}
