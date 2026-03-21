package buildsvc

import (
	"fmt"
	"sync"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

type Options struct {
	MaxQueuedBatches int
	MaxPlansPerBatch int
	MaxOpsPerTick    int
}

type Service struct {
	w *world.World

	maxQueuedBatches int
	maxPlansPerBatch int
	maxOpsPerTick    int

	mu    sync.Mutex
	queue []queuedBatch

	lastByTeam   map[world.TeamID][]world.BuildPlanOp
	lastAtByTeam map[world.TeamID]time.Time
}

type queuedBatch struct {
	team world.TeamID
	ops  []world.BuildPlanOp
}

func New(w *world.World, opts Options) *Service {
	maxQueuedBatches := opts.MaxQueuedBatches
	if maxQueuedBatches <= 0 {
		maxQueuedBatches = 256
	}
	maxPlansPerBatch := opts.MaxPlansPerBatch
	if maxPlansPerBatch <= 0 {
		maxPlansPerBatch = 20
	}
	maxOpsPerTick := opts.MaxOpsPerTick
	if maxOpsPerTick <= 0 {
		maxOpsPerTick = 64
	}
	if maxPlansPerBatch > maxOpsPerTick {
		maxPlansPerBatch = maxOpsPerTick
	}

	return &Service{
		w:                w,
		maxQueuedBatches: maxQueuedBatches,
		maxPlansPerBatch: maxPlansPerBatch,
		maxOpsPerTick:    maxOpsPerTick,
		queue:            make([]queuedBatch, 0, 64),
		lastByTeam:       make(map[world.TeamID][]world.BuildPlanOp),
		lastAtByTeam:     make(map[world.TeamID]time.Time),
	}
}

// Reset clears queued build operations. Useful when world map/model changes.
func (s *Service) Reset() {
	if s == nil {
		return
	}
	s.mu.Lock()
	s.queue = s.queue[:0]
	clear(s.lastByTeam)
	clear(s.lastAtByTeam)
	s.mu.Unlock()
}

// EnqueuePlans validates and enqueues protocol build plans.
func (s *Service) EnqueuePlans(team world.TeamID, plans []*protocol.BuildPlan) {
	if s == nil || s.w == nil || len(plans) == 0 {
		return
	}
	model := s.w.Model()
	if model == nil || model.Width <= 0 || model.Height <= 0 {
		return
	}

	ops := make([]world.BuildPlanOp, 0, minInt(len(plans), s.maxPlansPerBatch))
	seen := make(map[world.BuildPlanOp]struct{}, minInt(len(plans), s.maxPlansPerBatch))
	for _, p := range plans {
		if p == nil {
			continue
		}
		op, ok := sanitizePlan(model, p)
		if !ok {
			continue
		}
		if _, ok := seen[op]; ok {
			continue
		}
		seen[op] = struct{}{}
		ops = append(ops, op)
		if len(ops) >= s.maxPlansPerBatch {
			break
		}
	}
	if len(ops) == 0 {
		return
	}

	s.mu.Lock()
	// Snapshot packets can resend identical plans every tick.
	// Suppress only short-interval duplicates to avoid queue spam,
	// while still allowing later retries for the same coordinates.
	if prev := s.lastByTeam[team]; sameOps(prev, ops) {
		if lastAt := s.lastAtByTeam[team]; !lastAt.IsZero() && time.Since(lastAt) < 500*time.Millisecond {
			s.mu.Unlock()
			return
		}
	}
	if len(s.queue) >= s.maxQueuedBatches {
		// Keep recent requests when overloaded.
		copy(s.queue, s.queue[1:])
		s.queue = s.queue[:len(s.queue)-1]
	}
	s.queue = append(s.queue, queuedBatch{
		team: team,
		ops:  ops,
	})
	s.lastByTeam[team] = append(s.lastByTeam[team][:0], ops...)
	s.lastAtByTeam[team] = time.Now()
	s.mu.Unlock()
}

// SyncPlans applies authoritative client queue snapshots for one team.
// Unlike EnqueuePlans, this reconciles removals and order directly in world state.
func (s *Service) SyncPlans(team world.TeamID, plans []*protocol.BuildPlan) {
	if s == nil || s.w == nil {
		return
	}
	model := s.w.Model()
	if model == nil || model.Width <= 0 || model.Height <= 0 {
		return
	}
	ops := make([]world.BuildPlanOp, 0, len(plans))
	seenByPosType := make(map[[3]int32]struct{}, len(plans))
	for _, p := range plans {
		if p == nil {
			continue
		}
		op, ok := sanitizePlan(model, p)
		if !ok {
			continue
		}
		key := [3]int32{op.X, op.Y, 0}
		if op.Breaking {
			key[2] = 1
		}
		if _, ok := seenByPosType[key]; ok {
			continue
		}
		seenByPosType[key] = struct{}{}
		ops = append(ops, op)
	}

	s.mu.Lock()
	if prev := s.lastByTeam[team]; sameOps(prev, ops) {
		s.mu.Unlock()
		return
	}
	s.lastByTeam[team] = append(s.lastByTeam[team][:0], ops...)
	s.lastAtByTeam[team] = time.Now()
	s.mu.Unlock()

	s.w.ApplyBuildPlanSnapshot(team, ops)
}

func (s *Service) Tick() int {
	if s == nil || s.w == nil {
		return 0
	}
	remaining := s.maxOpsPerTick
	if remaining <= 0 {
		return 0
	}
	applied := 0

	for remaining > 0 {
		batch, ok := s.popFront()
		if !ok {
			break
		}
		if len(batch.ops) > remaining {
			split := remaining
			if split <= 0 {
				s.pushFront(batch)
				break
			}
			opsNow := batch.ops[:split]
			rest := append([]world.BuildPlanOp(nil), batch.ops[split:]...)
			batch.ops = rest
			s.pushFront(batch)
			_ = s.w.ApplyBuildPlans(batch.team, opsNow)
			applied += len(opsNow)
			remaining -= len(opsNow)
			continue
		}

		_ = s.w.ApplyBuildPlans(batch.team, batch.ops)
		applied += len(batch.ops)
		remaining -= len(batch.ops)
	}
	return applied
}

func (s *Service) popFront() (queuedBatch, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.queue) == 0 {
		return queuedBatch{}, false
	}
	b := s.queue[0]
	copy(s.queue, s.queue[1:])
	s.queue = s.queue[:len(s.queue)-1]
	return b, true
}

func (s *Service) pushFront(b queuedBatch) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue = append(s.queue, queuedBatch{})
	copy(s.queue[1:], s.queue[:len(s.queue)-1])
	s.queue[0] = b
}

func sanitizePlan(model *world.WorldModel, p *protocol.BuildPlan) (world.BuildPlanOp, bool) {
	if p == nil || model == nil {
		return world.BuildPlanOp{}, false
	}
	x := int(p.X)
	y := int(p.Y)
	if !model.InBounds(x, y) {
		return world.BuildPlanOp{}, false
	}
	if p.Breaking {
		return world.BuildPlanOp{
			Breaking: true,
			X:        p.X,
			Y:        p.Y,
		}, true
	}
	if p.Block == nil {
		fmt.Printf("[buildsvc] plan rejected: block is nil at (%d,%d)\n", x, y)
		return world.BuildPlanOp{}, false
	}
	blockID := p.Block.ID()
	if blockID <= 0 {
		fmt.Printf("[buildsvc] plan rejected: blockID=%d (<=0) at (%d,%d)\n", blockID, x, y)
		return world.BuildPlanOp{}, false
	}
	return world.BuildPlanOp{
		Breaking: false,
		X:        p.X,
		Y:        p.Y,
		Rotation: int8(p.Rotation & 0x03),
		BlockID:  blockID,
	}, true
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func sameOps(a, b []world.BuildPlanOp) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
