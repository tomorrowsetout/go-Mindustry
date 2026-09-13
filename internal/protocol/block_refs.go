package protocol

// BlockUnitTileRef is a lightweight tile reference for block-backed units.
type BlockUnitTileRef struct {
	PosValue int32
}

func (t BlockUnitTileRef) Pos() int32 { return t.PosValue }

// BlockUnitRef is a lightweight BlockUnit implementation used for
// serializing control-block proxy units.
type BlockUnitRef struct {
	IDValue int32
	TileRef Tile
}

func (u BlockUnitRef) ID() int32  { return u.IDValue }
func (u BlockUnitRef) Tile() Tile { return u.TileRef }

// ControlBuildingRef is a lightweight Building+ControlBlock wrapper.
type ControlBuildingRef struct {
	PosValue int32
	UnitRef  Unit
}

func (b ControlBuildingRef) Pos() int32 { return b.PosValue }
func (b ControlBuildingRef) Unit() Unit { return b.UnitRef }
