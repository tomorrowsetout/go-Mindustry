//go:build !cgo

// Pure-Go spatial hash (default). Same public API and cell-overlap queries as
// the optional cgo CSR grid; enable native with CGO_ENABLED=1 and a C++ toolchain.

package nativespatial

import "math"

// Grid is a pure-Go fallback spatial hash used when cgo is disabled.
// Query uses cell-overlap semantics, matching the native CSR grid.
type Grid struct {
	cellSize int32
	// cell key -> entity indices
	cells map[int64][]int32
	n     int
}

func packCell(cx, cy int32) int64 {
	return (int64(uint32(cx)) << 32) | int64(uint32(cy))
}

// New creates an empty grid.
func New(cellSize int32) *Grid {
	if cellSize <= 0 {
		cellSize = 64
	}
	return &Grid{cellSize: cellSize, cells: make(map[int64][]int32)}
}

// Destroy is a no-op in the pure-Go implementation.
func (g *Grid) Destroy() {}

// CellSize returns the configured cell size.
func (g *Grid) CellSize() int32 {
	if g == nil {
		return 0
	}
	return g.cellSize
}

// Len returns the number of points.
func (g *Grid) Len() int {
	if g == nil {
		return 0
	}
	return g.n
}

// Build inserts all points into hashed cells.
func (g *Grid) Build(xs, ys []float32) {
	if g == nil {
		return
	}
	n := len(xs)
	if n != len(ys) {
		n = 0
	}
	g.cells = make(map[int64][]int32, n)
	g.n = n
	cell := float32(g.cellSize)
	for i := 0; i < n; i++ {
		cx := int32(math.Floor(float64(xs[i] / cell)))
		cy := int32(math.Floor(float64(ys[i] / cell)))
		key := packCell(cx, cy)
		g.cells[key] = append(g.cells[key], int32(i))
	}
}

// QueryAABB appends entity indices in cells overlapping the box.
func (g *Grid) QueryAABB(minX, minY, maxX, maxY float32, out []int32) int {
	if g == nil || len(out) == 0 || g.n == 0 {
		return 0
	}
	cell := float32(g.cellSize)
	minCX := int32(math.Floor(float64(minX / cell)))
	maxCX := int32(math.Floor(float64(maxX / cell)))
	minCY := int32(math.Floor(float64(minY / cell)))
	maxCY := int32(math.Floor(float64(maxY / cell)))
	written := 0
	for cy := minCY; cy <= maxCY; cy++ {
		for cx := minCX; cx <= maxCX; cx++ {
			for _, i := range g.cells[packCell(cx, cy)] {
				out[written] = i
				written++
				if written >= len(out) {
					return written
				}
			}
		}
	}
	return written
}

// QueryRadius is QueryAABB for the circle bounding box.
func (g *Grid) QueryRadius(x, y, radius float32, out []int32) int {
	return g.QueryAABB(x-radius, y-radius, x+radius, y+radius, out)
}

// Available reports whether the native cgo implementation is linked.
func Available() bool { return false }
