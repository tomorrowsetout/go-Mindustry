package entity

import (
	"math"

	"mdt-server/internal/world"
)

// Vector2 2D vector
type Vector2 struct {
	X float64
	Y float64
}

// Distance calculate distance between two vectors
func (v Vector2) Distance(other Vector2) float64 {
	dx := v.X - other.X
	dy := v.Y - other.Y
	return math.Sqrt(dx*dx + dy*dy)
}

// Tile tile structure
type Tile struct {
	Floor     world.FloorID
	Overlay   world.OverlayID
	Con       world.ConID
	Block     world.BlockID
	Team      world.TeamID
	Rotation  int8
	Build     *world.Building
}

// RawEntityRaw entity structure
type RawEntity struct {
	TypeID   int16
	ID       int32
	X        float32
	Y        float32
	Rotation float32
	Team     world.TeamID
	Payload  []byte
}

// Building building structure
type Building struct {
	Block     world.BlockID
	X         int32
	Y         int32
	Team      world.TeamID
	Rotation  int8
	Health    float32
	Config    []byte
	Payload   []byte
}
