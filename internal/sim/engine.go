package sim

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

const DefaultTPS = 60

type Config struct {
	TPS        int
	Cores      int
	Partitions int
	TotalWork  int
	MaxCatchUp int
}

type Partition struct {
	ID    int
	Start int
	End   int
}

type TickContext struct {
	Tick       uint64
	Now        time.Time
	Delta      time.Duration
	Partitions int
}

type WorkFunc func(ctx TickContext, p Partition)

type TickStats struct {
	Tick         uint64
	LastTickTime time.Time
	LastDuration time.Duration
	Overrun      bool
	TPS          int
	Partitions   int
	TotalWork    int
}

type Engine struct {
	cfg Config

	work WorkFunc

	mu         sync.RWMutex
	partitions []Partition

	running atomic.Bool
	stopCh  chan struct{}
	doneCh  chan struct{}

	tickCount   atomic.Uint64
	lastTickNS  atomic.Int64
	lastDurNS   atomic.Int64
	lastOverrun atomic.Bool
}

func NewEngine(cfg Config) *Engine {
	cfg = normalizeConfig(cfg)
	parts := buildPartitions(cfg.TotalWork, cfg.Partitions)
	return &Engine{
		cfg:        cfg,
		work:       noopWork,
		partitions: parts,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

func (e *Engine) Start() {
	if e.running.Swap(true) {
		return
	}
	go e.run()
}

func (e *Engine) Stop() {
	if !e.running.Swap(false) {
		return
	}
	close(e.stopCh)
	<-e.doneCh
}

func (e *Engine) SetWork(fn WorkFunc) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.work = fn
}

func (e *Engine) SetTotalWork(total int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if total < 0 {
		total = 0
	}
	e.cfg.TotalWork = total
	e.partitions = buildPartitions(e.cfg.TotalWork, e.cfg.Partitions)
}

func (e *Engine) SetPartitions(count int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if count <= 0 {
		count = 1
	}
	e.cfg.Partitions = count
	e.partitions = buildPartitions(e.cfg.TotalWork, e.cfg.Partitions)
}

func (e *Engine) Stats() TickStats {
	e.mu.RLock()
	parts := len(e.partitions)
	total := e.cfg.TotalWork
	tps := e.cfg.TPS
	e.mu.RUnlock()

	lastNS := e.lastTickNS.Load()
	lastTime := time.Time{}
	if lastNS > 0 {
		lastTime = time.Unix(0, lastNS)
	}

	return TickStats{
		Tick:         e.tickCount.Load(),
		LastTickTime: lastTime,
		LastDuration: time.Duration(e.lastDurNS.Load()),
		Overrun:      e.lastOverrun.Load(),
		TPS:          tps,
		Partitions:   parts,
		TotalWork:    total,
	}
}

func (e *Engine) run() {
	runtime.GOMAXPROCS(e.cfg.Cores)

	interval := time.Second / time.Duration(e.cfg.TPS)
	next := time.Now().Add(interval)

	for {
		now := time.Now()
		if now.Before(next) {
			if !sleepWithStop(e.stopCh, next.Sub(now)) {
				close(e.doneCh)
				return
			}
			continue
		}

		steps := 0
		for !now.Before(next) && steps < e.cfg.MaxCatchUp {
			e.step(interval)
			steps++
			next = next.Add(interval)
			now = time.Now()
		}
		if steps == e.cfg.MaxCatchUp && !now.Before(next) {
			next = now.Add(interval)
		}

		select {
		case <-e.stopCh:
			close(e.doneCh)
			return
		default:
		}
	}
}

func (e *Engine) step(interval time.Duration) {
	start := time.Now()
	tick := e.tickCount.Add(1)

	e.mu.RLock()
	work := e.work
	parts := append([]Partition(nil), e.partitions...)
	e.mu.RUnlock()

	if work != nil && len(parts) > 0 {
		ctx := TickContext{
			Tick:       tick,
			Now:        start,
			Delta:      interval,
			Partitions: len(parts),
		}
		var wg sync.WaitGroup
		wg.Add(len(parts))
		for _, p := range parts {
			part := p
			go func() {
				defer wg.Done()
				work(ctx, part)
			}()
		}
		wg.Wait()
	}

	dur := time.Since(start)
	e.lastTickNS.Store(start.UnixNano())
	e.lastDurNS.Store(int64(dur))
	e.lastOverrun.Store(dur > interval)
}

func normalizeConfig(cfg Config) Config {
	if cfg.TPS <= 0 {
		cfg.TPS = DefaultTPS
	}
	if cfg.Cores <= 0 {
		cfg.Cores = runtime.NumCPU()
	}
	if cfg.Partitions <= 0 {
		cfg.Partitions = cfg.Cores
	}
	if cfg.TotalWork < 0 {
		cfg.TotalWork = 0
	}
	if cfg.MaxCatchUp <= 0 {
		cfg.MaxCatchUp = 4
	}
	return cfg
}

func buildPartitions(total, count int) []Partition {
	if count <= 0 {
		count = 1
	}
	if total < 0 {
		total = 0
	}
	parts := make([]Partition, count)
	if total == 0 {
		for i := 0; i < count; i++ {
			parts[i] = Partition{ID: i, Start: 0, End: 0}
		}
		return parts
	}

	step := total / count
	rem := total % count
	start := 0
	for i := 0; i < count; i++ {
		size := step
		if i < rem {
			size++
		}
		end := start + size
		parts[i] = Partition{ID: i, Start: start, End: end}
		start = end
	}
	return parts
}

func sleepWithStop(stop <-chan struct{}, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-stop:
		return false
	}
}

func noopWork(_ TickContext, _ Partition) {}
