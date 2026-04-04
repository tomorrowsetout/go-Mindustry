package protocol

// UnitEntitySync mirrors mindustry.gen.UnitEntity writeSync order (classId=3).
// It is used to provide visible spawned units in entitySnapshot.
type UnitEntitySync struct {
	IDValue int32

	ClassIDValue byte
	ClassIDSet   bool

	Abilities      []Ability
	Ammo           float32
	BaseRotation   float32
	Building       Building
	Controller     UnitController
	Elevation      float32
	Flag           float64
	Health         float32
	Shooting       bool
	Lifetime       float32
	MineTile       Tile
	Mounts         []WeaponMount
	Payloads       []Payload
	Plans          []*BuildPlan
	Rotation       float32
	Shield         float32
	SpawnedByCore  bool
	Stack          ItemStack
	Statuses       []StatusEntry
	TeamID         byte
	Time           float32
	TypeID         int16
	UpdateBuilding bool
	Vel            Vec2
	X              float32
	Y              float32
}

func (u *UnitEntitySync) ID() int32      { return u.IDValue }
func (u *UnitEntitySync) SetID(id int32) { u.IDValue = id }
func (u *UnitEntitySync) ClassID() byte {
	if u != nil && u.ClassIDSet {
		return u.ClassIDValue
	}
	return defaultUnitEntityLayout.classID
}
func (u *UnitEntitySync) BeforeWrite() {}
func (u *UnitEntitySync) SnapSync()    {}
func (u *UnitEntitySync) Add()         {}

func (u *UnitEntitySync) WriteSync(w *Writer) error {
	layout := u.layout()
	if err := WriteAbilities(w, u.Abilities); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.Ammo); err != nil {
		return err
	}
	if layout.baseRotation {
		if err := w.WriteFloat32(u.BaseRotation); err != nil {
			return err
		}
	}
	if layout.building {
		if err := WriteBuilding(w, u.Building); err != nil {
			return err
		}
	}
	if err := WriteController(w, u.Controller); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.Elevation); err != nil {
		return err
	}
	if err := w.WriteFloat64(u.Flag); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.Health); err != nil {
		return err
	}
	if err := w.WriteBool(u.Shooting); err != nil {
		return err
	}
	if layout.timedKill {
		if err := w.WriteFloat32(u.Lifetime); err != nil {
			return err
		}
	}
	if err := WriteTile(w, u.MineTile); err != nil {
		return err
	}
	if err := WriteMounts(w, u.Mounts); err != nil {
		return err
	}
	if layout.payloads {
		if err := writePayloadSeq(w, u.Payloads); err != nil {
			return err
		}
	}
	if err := WritePlansQueueNet(w, u.Plans, w.Ctx); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.Rotation); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.Shield); err != nil {
		return err
	}
	if err := w.WriteBool(u.SpawnedByCore); err != nil {
		return err
	}
	if err := WriteItems(w, u.Stack); err != nil {
		return err
	}
	if err := w.WriteInt32(int32(len(u.Statuses))); err != nil {
		return err
	}
	for _, st := range u.Statuses {
		if err := WriteStatus(w, st); err != nil {
			return err
		}
	}
	if err := WriteTeam(w, &Team{ID: u.TeamID}); err != nil {
		return err
	}
	if layout.timedKill {
		if err := w.WriteFloat32(u.Time); err != nil {
			return err
		}
	}
	if err := w.WriteInt16(u.TypeID); err != nil {
		return err
	}
	if err := w.WriteBool(u.UpdateBuilding); err != nil {
		return err
	}
	if err := WriteVec2(w, u.Vel); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.X); err != nil {
		return err
	}
	return w.WriteFloat32(u.Y)
}

func (u *UnitEntitySync) WriteEntity(w *Writer) error {
	layout := u.layout()
	if err := w.WriteInt16(layout.revision); err != nil {
		return err
	}
	if err := WriteAbilities(w, u.Abilities); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.Ammo); err != nil {
		return err
	}
	if layout.baseRotation {
		if err := w.WriteFloat32(u.BaseRotation); err != nil {
			return err
		}
	}
	if layout.building {
		if err := WriteBuilding(w, u.Building); err != nil {
			return err
		}
	}
	if err := WriteController(w, u.Controller); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.Elevation); err != nil {
		return err
	}
	if err := w.WriteFloat64(u.Flag); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.Health); err != nil {
		return err
	}
	if err := w.WriteBool(u.Shooting); err != nil {
		return err
	}
	if layout.timedKill {
		if err := w.WriteFloat32(u.Lifetime); err != nil {
			return err
		}
	}
	if err := WriteTile(w, u.MineTile); err != nil {
		return err
	}
	if err := WriteMounts(w, u.Mounts); err != nil {
		return err
	}
	if layout.payloads {
		if err := writePayloadSeq(w, u.Payloads); err != nil {
			return err
		}
	}
	if err := writeAllPlansQueue(w, u.Plans, w.Ctx); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.Rotation); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.Shield); err != nil {
		return err
	}
	if err := w.WriteBool(u.SpawnedByCore); err != nil {
		return err
	}
	if err := WriteItems(w, u.Stack); err != nil {
		return err
	}
	if err := w.WriteInt32(int32(len(u.Statuses))); err != nil {
		return err
	}
	for _, st := range u.Statuses {
		if err := WriteStatus(w, st); err != nil {
			return err
		}
	}
	if err := WriteTeam(w, &Team{ID: u.TeamID}); err != nil {
		return err
	}
	if layout.timedKill {
		if err := w.WriteFloat32(u.Time); err != nil {
			return err
		}
	}
	if err := w.WriteInt16(u.TypeID); err != nil {
		return err
	}
	if err := w.WriteBool(u.UpdateBuilding); err != nil {
		return err
	}
	if err := WriteVec2(w, u.Vel); err != nil {
		return err
	}
	if err := w.WriteFloat32(u.X); err != nil {
		return err
	}
	return w.WriteFloat32(u.Y)
}

func (u *UnitEntitySync) ReadSync(r *Reader) error {
	layout := u.layout()
	var err error
	if u.Abilities, err = ReadAbilities(r, u.Abilities); err != nil {
		return err
	}
	if u.Ammo, err = r.ReadFloat32(); err != nil {
		return err
	}
	if layout.baseRotation {
		if u.BaseRotation, err = r.ReadFloat32(); err != nil {
			return err
		}
	}
	if layout.building {
		if u.Building, err = ReadBuilding(r, r.Ctx); err != nil {
			return err
		}
	}
	if u.Controller, err = ReadController(r, u.Controller); err != nil {
		return err
	}
	if u.Elevation, err = r.ReadFloat32(); err != nil {
		return err
	}
	if u.Flag, err = r.ReadFloat64(); err != nil {
		return err
	}
	if u.Health, err = r.ReadFloat32(); err != nil {
		return err
	}
	if u.Shooting, err = r.ReadBool(); err != nil {
		return err
	}
	if layout.timedKill {
		if u.Lifetime, err = r.ReadFloat32(); err != nil {
			return err
		}
	}
	if u.MineTile, err = ReadTile(r, r.Ctx); err != nil {
		return err
	}
	if u.Mounts, err = ReadMounts(r, u.Mounts); err != nil {
		return err
	}
	if layout.payloads {
		if u.Payloads, err = readPayloadSeq(r); err != nil {
			return err
		}
	}
	if u.Plans, err = ReadPlansQueue(r, r.Ctx); err != nil {
		return err
	}
	if u.Rotation, err = r.ReadFloat32(); err != nil {
		return err
	}
	if u.Shield, err = r.ReadFloat32(); err != nil {
		return err
	}
	if u.SpawnedByCore, err = r.ReadBool(); err != nil {
		return err
	}
	if u.Stack, err = ReadItems(r, r.Ctx); err != nil {
		return err
	}
	n, err := r.ReadInt32()
	if err != nil {
		return err
	}
	if n < 0 {
		n = 0
	}
	u.Statuses = u.Statuses[:0]
	for i := 0; i < int(n); i++ {
		st, serr := ReadStatus(r, r.Ctx)
		if serr != nil {
			return serr
		}
		u.Statuses = append(u.Statuses, st)
	}
	team, err := ReadTeam(r, r.Ctx)
	if err != nil {
		return err
	}
	u.TeamID = team.ID
	if layout.timedKill {
		if u.Time, err = r.ReadFloat32(); err != nil {
			return err
		}
	}
	if u.TypeID, err = r.ReadInt16(); err != nil {
		return err
	}
	if u.UpdateBuilding, err = r.ReadBool(); err != nil {
		return err
	}
	if u.Vel, err = ReadVec2(r); err != nil {
		return err
	}
	if u.X, err = r.ReadFloat32(); err != nil {
		return err
	}
	if u.Y, err = r.ReadFloat32(); err != nil {
		return err
	}
	return nil
}

func writePayloadSeq(w *Writer, payloads []Payload) error {
	if payloads == nil {
		return w.WriteInt32(0)
	}
	if err := w.WriteInt32(int32(len(payloads))); err != nil {
		return err
	}
	for _, payload := range payloads {
		if err := WritePayload(w, payload); err != nil {
			return err
		}
	}
	return nil
}

func readPayloadSeq(r *Reader) ([]Payload, error) {
	n, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if n <= 0 {
		return []Payload{}, nil
	}
	payloads := make([]Payload, 0, n)
	for i := 0; i < int(n); i++ {
		payload, err := ReadPayload(r, r.Ctx)
		if err != nil {
			return nil, err
		}
		if payload != nil {
			payloads = append(payloads, payload)
		}
	}
	return payloads, nil
}

func writeAllPlansQueue(w *Writer, plans []*BuildPlan, ctx *TypeIOContext) error {
	if plans == nil {
		return w.WriteInt32(0)
	}
	if err := w.WriteInt32(int32(len(plans))); err != nil {
		return err
	}
	for _, plan := range plans {
		if plan == nil {
			continue
		}
		if err := WritePlan(w, plan, ctx); err != nil {
			return err
		}
	}
	return nil
}
