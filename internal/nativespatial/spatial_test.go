package nativespatial

import "testing"

func TestGridBuildAndQueryAABB(t *testing.T) {
	g := New(64)
	defer g.Destroy()

	xs := []float32{10, 70, 130, 200, 15}
	ys := []float32{10, 70, 10, 200, 90}
	g.Build(xs, ys)
	if g.Len() != 5 {
		t.Fatalf("expected len 5, got %d", g.Len())
	}

	out := make([]int32, 16)
	n := g.QueryAABB(0, 0, 80, 80, out)
	got := map[int32]bool{}
	for i := 0; i < n; i++ {
		got[out[i]] = true
	}
	// Cell-overlap: AABB [0,0]-[80,80] with cell 64 covers cells
	// (0,0),(1,0),(0,1),(1,1) → points (10,10), (70,70), (15,90).
	for _, want := range []int32{0, 1, 4} {
		if !got[want] {
			t.Fatalf("expected index %d in query, got %v (n=%d)", want, got, n)
		}
	}
	if got[2] || got[3] {
		t.Fatalf("did not expect far points, got %v", got)
	}
}

func TestGridClearAndRebuild(t *testing.T) {
	g := New(32)
	defer g.Destroy()
	g.Build([]float32{1, 2, 3}, []float32{1, 2, 3})
	if g.Len() != 3 {
		t.Fatalf("expected 3, got %d", g.Len())
	}
	g.Build(nil, nil)
	if g.Len() != 0 {
		t.Fatalf("expected empty after clear, got %d", g.Len())
	}
	out := make([]int32, 4)
	if n := g.QueryRadius(0, 0, 10, out); n != 0 {
		t.Fatalf("expected 0 hits on empty grid, got %d", n)
	}
}
