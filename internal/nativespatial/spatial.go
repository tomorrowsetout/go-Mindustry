//go:build cgo

package nativespatial

/*
#cgo CXXFLAGS: -O3 -std=c++17
#cgo LDFLAGS: -lstdc++
#include <stdlib.h>
#include "spatial.h"
*/
import "C"

import (
	"runtime"
	"unsafe"
)

// Grid is a dense uniform spatial hash over entity indices.
// Rebuild is O(n); radius query writes matching entity indices into a caller buffer.
type Grid struct {
	h     *C.ns_grid
	cell  int32
	count int
}

const defaultQueryCap = 4096

// New creates an empty grid with the given cell size in world units.
func New(cellSize int32) *Grid {
	if cellSize <= 0 {
		cellSize = 64
	}
	g := &Grid{
		h:    C.ns_grid_create(C.int(cellSize)),
		cell: cellSize,
	}
	runtime.SetFinalizer(g, func(g *Grid) { g.Destroy() })
	return g
}

// Destroy frees the native grid. Safe to call multiple times.
func (g *Grid) Destroy() {
	if g == nil || g.h == nil {
		return
	}
	C.ns_grid_destroy(g.h)
	g.h = nil
	runtime.SetFinalizer(g, nil)
}

// CellSize returns the grid cell size in world units.
func (g *Grid) CellSize() int32 {
	if g == nil {
		return 0
	}
	return g.cell
}

// Len returns the number of points currently inserted.
func (g *Grid) Len() int {
	if g == nil || g.h == nil {
		return 0
	}
	return int(C.ns_grid_len(g.h))
}

// Build clears the grid and inserts all points. Empty cells are compacted into a CSR layout.
func (g *Grid) Build(xs, ys []float32) {
	if g == nil || g.h == nil {
		return
	}
	n := len(xs)
	if n != len(ys) {
		n = 0
	}
	if n == 0 {
		C.ns_grid_clear(g.h)
		g.count = 0
		return
	}
	C.ns_grid_build(g.h, (*C.float)(unsafe.Pointer(&xs[0])), (*C.float)(unsafe.Pointer(&ys[0])), C.int(n))
	g.count = n
}

// QueryAABB appends entity indices whose cell overlaps the axis-aligned box.
// Returns the number of indices written to out.
func (g *Grid) QueryAABB(minX, minY, maxX, maxY float32, out []int32) int {
	if g == nil || g.h == nil || len(out) == 0 {
		return 0
	}
	return int(C.ns_grid_query_aabb(
		g.h,
		C.float(minX), C.float(minY), C.float(maxX), C.float(maxY),
		(*C.int)(unsafe.Pointer(&out[0])), C.int(len(out)),
	))
}

// QueryRadius appends entity indices in cells overlapping the circle AABB.
// Callers still perform exact distance filters.
func (g *Grid) QueryRadius(x, y, radius float32, out []int32) int {
	return g.QueryAABB(x-radius, y-radius, x+radius, y+radius, out)
}

// Available reports whether the native cgo implementation is linked.
func Available() bool { return true }
