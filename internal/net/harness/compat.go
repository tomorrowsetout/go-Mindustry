package harness

import (
	"fmt"
	"sync"
)

// CompatTrace records every packet the simulated client receives, in wire
// order, so tests can lock the exact build-158 server->client sequence.
//
// Vanilla ordering matters: official clients reject servers that reorder
// critical sync packets. This trace is the assertion surface that keeps the Go
// server wire-compatible across refactors.
type CompatTrace struct {
	mu    sync.Mutex
	order []int
	byID  map[int]int
}

// NewCompatTrace returns an empty trace.
func NewCompatTrace() *CompatTrace {
	return &CompatTrace{byID: make(map[int]int)}
}

// Record appends a received packet (id is the wire packet id; 0xFE == 254 for
// framework messages).
func (t *CompatTrace) Record(id byte, _ any) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.order = append(t.order, int(id))
	t.byID[int(id)]++
}

// Order returns the wire packet ids in receive order.
func (t *CompatTrace) Order() []int {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := make([]int, len(t.order))
	copy(out, t.order)
	return out
}

// Count returns how many packets of the given wire id were received.
func (t *CompatTrace) Count(id byte) int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.byID[int(id)]
}

// Seen reports whether at least one packet of the given wire id arrived.
func (t *CompatTrace) Seen(id byte) bool {
	return t.Count(id) > 0
}

// AssertOrder verifies the received sequence contains want in the given order,
// allowing other packets in between. It returns the first mismatch or "".
func (t *CompatTrace) AssertOrder(want ...int) string {
	t.mu.Lock()
	order := make([]int, len(t.order))
	copy(order, t.order)
	t.mu.Unlock()

	idx := 0
	for _, w := range want {
		found := false
		for ; idx < len(order); idx++ {
			if order[idx] == w {
				found = true
				idx++
				break
			}
		}
		if !found {
			return fmt.Sprintf("expected wire packet id %d not found in order %v", w, order)
		}
	}
	return ""
}

// String renders a compact summary, e.g. "0,1,126,46".
func (t *CompatTrace) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	out := ""
	for i, id := range t.order {
		if i > 0 {
			out += ","
		}
		out += fmt.Sprintf("%d", id)
	}
	return out
}
