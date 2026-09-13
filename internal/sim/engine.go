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

type DispatchFunc func(p Partition)

type DispatchStats struct {
	Partitions       int
	TotalWork        int
	WorkerCount      int
	DispatchDuration time.Duration
	WorkDuration     time.Duration
	IdleDuration     time.Duration
}

type TickStats struct {
	Tick         uint64
	LastTickTime time.Time
	LastDuration time.Duration
	Overrun      bool
	TPS          int
	Partitions   int
	TotalWork    int
	WorkerCount  int
	LastDispatch DispatchStats
}

type Engine struct {
	cfg Config

	work WorkFunc

	mu         sync.RWMutex
	partitions []Partition

	running atomic.Bool
	stopCh  chan struct{}
	doneCh  chan struct{}

	workerMu      sync.Mutex
	workerCount   int
	workerStarted bool
	workerClosed  bool
	taskCh        chan workerTask
	workerStopCh  chan struct{}
	workerWG      sync.WaitGroup

	tickCount   atomic.Uint64
	lastTickNS  atomic.Int64
	lastDurNS   atomic.Int64
	lastOverrun atomic.Bool
	lastDispNS  atomic.Int64
	lastWorkNS  atomic.Int64
	lastIdleNS  atomic.Int64
	lastWorkers atomic.Int64
	lastWorkCnt atomic.Int64
	lastPartCnt atomic.Int64
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
	e.ensureWorkers()
	go e.run()
}

func (e *Engine) Stop() {
	if e.running.Swap(false) {
		close(e.stopCh)
		<-e.doneCh
	}
	e.closeWorkers()
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

func (e *Engine) RunTick(delta time.Duration, fn func()) {
	if fn == nil {
		return
	}
	start := time.Now()
	fn()
	e.tickCount.Add(1)
	e.recordTick(start, time.Since(start), delta > 0 && time.Since(start) > delta)
}

func (e *Engine) RecordTick(start time.Time, duration time.Duration, overrun bool) {
	if start.IsZero() {
		start = time.Now().Add(-duration)
	}
	e.tickCount.Add(1)
	e.recordTick(start, duration, overrun)
}

func (e *Engine) Dispatch(totalWork int, fn DispatchFunc) DispatchStats {
	if fn == nil {
		return DispatchStats{}
	}
	parts := buildPartitions(totalWork, e.PartitionCount(totalWork))
	if len(parts) == 0 {
		e.lastDispNS.Store(0)
		e.lastWorkNS.Store(0)
		e.lastIdleNS.Store(0)
		e.lastWorkCnt.Store(int64(totalWork))
		e.lastPartCnt.Store(0)
		return DispatchStats{TotalWork: totalWork, WorkerCount: e.WorkerCount()}
	}
	start := time.Now()
	workDur, workerCount := e.dispatch(parts, fn)
	dispatchDur := time.Since(start)
	idleDur := time.Duration(0)
	if capacity := dispatchDur * time.Duration(len(parts)); capacity > workDur {
		idleDur = capacity - workDur
	}
	stats := DispatchStats{
		Partitions:       len(parts),
		TotalWork:        totalWork,
		WorkerCount:      workerCount,
		DispatchDuration: dispatchDur,
		WorkDuration:     workDur,
		IdleDuration:     idleDur,
	}
	e.lastDispNS.Store(int64(dispatchDur))
	e.lastWorkNS.Store(int64(workDur))
	e.lastIdleNS.Store(int64(idleDur))
	e.lastWorkers.Store(int64(workerCount))
	e.lastWorkCnt.Store(int64(totalWork))
	e.lastPartCnt.Store(int64(len(parts)))
	return stats
}

func (e *Engine) WorkerCount() int {
	e.workerMu.Lock()
	defer e.workerMu.Unlock()
	return e.workerCountLocked()
}

func (e *Engine) PartitionCount(totalWork int) int {
	e.mu.RLock()
	count := e.cfg.Partitions
	e.mu.RUnlock()
	return len(buildPartitions(totalWork, count))
}

func (e *Engine) Stats() TickStats {
	e.mu.RLock()
	configuredParts := len(e.partitions)
	configuredTotal := e.cfg.TotalWork
	tps := e.cfg.TPS
	e.mu.RUnlock()

	lastNS := e.lastTickNS.Load()
	lastTime := time.Time{}
	if lastNS > 0 {
		lastTime = time.Unix(0, lastNS)
	}

	lastParts := int(e.lastPartCnt.Load())
	if lastParts <= 0 {
		lastParts = configuredParts
	}
	lastWork := int(e.lastWorkCnt.Load())
	if lastWork <= 0 {
		lastWork = configuredTotal
	}
	workerCount := int(e.lastWorkers.Load())
	if workerCount <= 0 {
		workerCount = e.WorkerCount()
	}

	return TickStats{
		Tick:         e.tickCount.Load(),
		LastTickTime: lastTime,
		LastDuration: time.Duration(e.lastDurNS.Load()),
		Overrun:      e.lastOverrun.Load(),
		TPS:          tps,
		Partitions:   lastParts,
		TotalWork:    lastWork,
		WorkerCount:  workerCount,
		LastDispatch: DispatchStats{
			Partitions:       lastParts,
			TotalWork:        lastWork,
			WorkerCount:      workerCount,
			DispatchDuration: time.Duration(e.lastDispNS.Load()),
			WorkDuration:     time.Duration(e.lastWorkNS.Load()),
			IdleDuration:     time.Duration(e.lastIdleNS.Load()),
		},
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
		e.dispatchWithContext(ctx, parts, work)
	}

	e.recordTick(start, time.Since(start), time.Since(start) > interval)
}

func (e *Engine) dispatchWithContext(ctx TickContext, parts []Partition, work WorkFunc) (time.Duration, int) {
	return e.dispatch(parts, func(p Partition) {
		work(ctx, p)
	})
}

func (e *Engine) dispatch(parts []Partition, fn DispatchFunc) (time.Duration, int) {
	if len(parts) == 0 || fn == nil {
		return 0, 0
	}
	workers := e.ensureWorkers()
	if workers <= 1 || len(parts) <= 1 {
		start := time.Now()
		for _, p := range parts {
			fn(p)
		}
		return time.Since(start), minInt(workers, len(parts))
	}

	var wg sync.WaitGroup
	var workNS atomic.Int64
	wg.Add(len(parts))
	for _, p := range parts {
		e.taskCh <- workerTask{
			part:   p,
			fn:     fn,
			done:   &wg,
			workNS: &workNS,
		}
	}
	wg.Wait()
	return time.Duration(workNS.Load()), minInt(workers, len(parts))
}

func (e *Engine) ensureWorkers() int {
	e.workerMu.Lock()
	defer e.workerMu.Unlock()
	if e.workerClosed {
		return 0
	}
	count := e.workerCountLocked()
	if e.workerStarted {
		return count
	}
	e.taskCh = make(chan workerTask, count*2)
	e.workerStopCh = make(chan struct{})
	e.workerStarted = true
	for i := 0; i < count; i++ {
		e.workerWG.Add(1)
		go e.worker()
	}
	runtime.GOMAXPROCS(e.cfg.Cores)
	return count
}

func (e *Engine) closeWorkers() {
	e.workerMu.Lock()
	if !e.workerStarted || e.workerClosed {
		e.workerMu.Unlock()
		return
	}
	close(e.workerStopCh)
	e.workerClosed = true
	e.workerMu.Unlock()
	e.workerWG.Wait()
}

func (e *Engine) worker() {
	defer e.workerWG.Done()
	for {
		select {
		case <-e.workerStopCh:
			return
		case task := <-e.taskCh:
			if task.fn == nil || task.done == nil {
				continue
			}
			start := time.Now()
			task.fn(task.part)
			if task.workNS != nil {
				task.workNS.Add(int64(time.Since(start)))
			}
			task.done.Done()
		}
	}
}

func (e *Engine) workerCountLocked() int {
	if e.workerCount <= 0 {
		count := e.cfg.Partitions
		if count <= 0 {
			count = e.cfg.Cores
		}
		if count <= 0 {
			count = 1
		}
		e.workerCount = count
	}
	return e.workerCount
}

func (e *Engine) recordTick(start time.Time, dur time.Duration, overrun bool) {
	e.lastTickNS.Store(start.UnixNano())
	e.lastDurNS.Store(int64(dur))
	e.lastOverrun.Store(overrun)
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

type workerTask struct {
	part   Partition
	fn     DispatchFunc
	done   *sync.WaitGroup
	workNS *atomic.Int64
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

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
