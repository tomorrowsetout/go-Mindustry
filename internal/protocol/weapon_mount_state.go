package protocol

// BasicWeaponMount is a minimal writable weapon mount state for entity snapshots.
type BasicWeaponMount struct {
	AimPosX  float32
	AimPosY  float32
	Shooting bool
	Rotating bool
}

func (m *BasicWeaponMount) AimX() float32 {
	if m == nil {
		return 0
	}
	return m.AimPosX
}

func (m *BasicWeaponMount) AimY() float32 {
	if m == nil {
		return 0
	}
	return m.AimPosY
}

func (m *BasicWeaponMount) SetAim(x, y float32) {
	if m == nil {
		return
	}
	m.AimPosX = x
	m.AimPosY = y
}

func (m *BasicWeaponMount) Shoot() bool {
	if m == nil {
		return false
	}
	return m.Shooting
}

func (m *BasicWeaponMount) Rotate() bool {
	if m == nil {
		return false
	}
	return m.Rotating
}

func (m *BasicWeaponMount) SetShoot(v bool) {
	if m == nil {
		return
	}
	m.Shooting = v
}

func (m *BasicWeaponMount) SetRotate(v bool) {
	if m == nil {
		return
	}
	m.Rotating = v
}
