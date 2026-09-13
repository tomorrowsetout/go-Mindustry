// Package modules provides lifecycle adapters that host the server subsystems
// inside core.Container.
//
// The world and persist subsystems cannot carry their own adapters (core
// already imports them, so importing core back would cycle), therefore their
// adapters are hosted here. LifecycleModule is a generic carrier for startup
// and teardown steps whose only requirement is container-managed ordering
// (dual-core IO cores, the game loop, the video recorder, ...).
package modules

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"mdt-server/internal/core"
	"mdt-server/internal/protocol"
	"mdt-server/internal/sim"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

// WorldModule adapts *world.World to the core.WorldModule contract.
type WorldModule struct {
	wld     *world.World
	content *protocol.ContentRegistry
}

// NewWorldModule wraps an already-constructed world.
func NewWorldModule(wld *world.World, content *protocol.ContentRegistry) *WorldModule {
	return &WorldModule{wld: wld, content: content}
}

// Name implements core.Module.
func (m *WorldModule) Name() string { return "world" }

// Dependencies implements core.Module: the world is the base layer.
func (m *WorldModule) Dependencies() []string { return nil }

// Init implements core.Module.
func (m *WorldModule) Init(_ core.Deps) error { return nil }

// Start implements core.Module. Ticking is driven by the runtime module.
func (m *WorldModule) Start() error { return nil }

// Stop implements core.Module.
func (m *WorldModule) Stop(_ context.Context) error { return nil }

// LoadMap implements core.WorldModule by decoding an in-memory .msav payload
// and installing it as the authoritative world model.
func (m *WorldModule) LoadMap(payload []byte) error {
	if m.wld == nil {
		return errors.New("world module not configured")
	}
	model, err := worldstream.LoadWorldModelFromMSAVBytes(payload, m.content)
	if err != nil {
		return err
	}
	m.wld.SetModel(model)
	return nil
}

// Compile-time contract guarantee.
var _ core.WorldModule = (*WorldModule)(nil)

// SimModule adapts the tick function and partition engine to the
// core.SimModule contract. It lives here (not in internal/sim) because world
// imports sim and core imports world, so sim cannot import core back.
type SimModule struct {
	stepFn func(delta time.Duration)
	delta  time.Duration
	engine *sim.Engine
	deps   []string
}

// NewSimModule wraps a tick function and the optional partition engine. deps
// lists the modules that must be running before the simulation is driven.
func NewSimModule(stepFn func(delta time.Duration), delta time.Duration, engine *sim.Engine, deps []string) *SimModule {
	if delta <= 0 {
		delta = time.Second / 60
	}
	if len(deps) == 0 {
		deps = []string{"world"}
	}
	return &SimModule{stepFn: stepFn, delta: delta, engine: engine, deps: deps}
}

// Name implements core.Module.
func (m *SimModule) Name() string { return "sim" }

// Dependencies implements core.Module.
func (m *SimModule) Dependencies() []string { return m.deps }

// Init implements core.Module.
func (m *SimModule) Init(_ core.Deps) error { return nil }

// Start implements core.Module. Tick driving lives in the runtime module.
func (m *SimModule) Start() error { return nil }

// Stop implements core.Module and releases engine workers (idempotent).
func (m *SimModule) Stop(_ context.Context) error {
	if m.engine != nil {
		m.engine.Stop()
	}
	return nil
}

// Step implements core.SimModule by advancing the simulation one tick.
func (m *SimModule) Step() error {
	if m.stepFn == nil {
		return nil
	}
	m.stepFn(m.delta)
	return nil
}

// Compile-time contract guarantee.
var _ core.SimModule = (*SimModule)(nil)

// PersistModule implements the core.PersistModule contract over the runtime
// save callbacks. Hot snapshots are written by the ticker goroutines, so Save
// only records the request; Stop performs the ordered cold flush.
//
// NOTE: Save/Load are in-memory for now. A file-backed store lands together
// with the plugin system (stage 3), which is the first real consumer.
type PersistModule struct {
	mu      sync.Mutex
	saved   map[string][]byte
	deps    []string
	flushFn func()
}

// NewPersistModule wires the cold-snapshot flush callback used on stop. deps
// must list the modules that may still mutate the world when the save runs
// (typically net and sim, plus servercore in dual-core mode) so the container
// stops them after the save.
func NewPersistModule(deps []string, flushFn func()) *PersistModule {
	if len(deps) == 0 {
		deps = []string{"sim"}
	}
	return &PersistModule{saved: make(map[string][]byte), deps: deps, flushFn: flushFn}
}

// Name implements core.Module.
func (m *PersistModule) Name() string { return "persist" }

// Dependencies implements core.Module.
func (m *PersistModule) Dependencies() []string { return m.deps }

// Init implements core.Module.
func (m *PersistModule) Init(_ core.Deps) error { return nil }

// Start implements core.Module.
func (m *PersistModule) Start() error { return nil }

// Stop implements core.Module and performs the ordered cold save.
func (m *PersistModule) Stop(_ context.Context) error {
	if m.flushFn != nil {
		m.flushFn()
	}
	return nil
}

// Save implements core.PersistModule.
func (m *PersistModule) Save(key string, value []byte) error {
	m.mu.Lock()
	m.saved[key] = append([]byte(nil), value...)
	m.mu.Unlock()
	return nil
}

// Load implements core.PersistModule. Returns nil, nil when the key is absent.
func (m *PersistModule) Load(key string) ([]byte, error) {
	m.mu.Lock()
	v, ok := m.saved[key]
	m.mu.Unlock()
	if !ok {
		return nil, nil
	}
	return append([]byte(nil), v...), nil
}

// Compile-time contract guarantee.
var _ core.PersistModule = (*PersistModule)(nil)

// LifecycleModule is a generic core.Module carrier for startup/teardown steps
// that only need container-managed ordering. The narrow contracts live in the
// subsystem-specific adapters; this type deliberately exposes no capability
// methods.
type LifecycleModule struct {
	name    string
	deps    []string
	startFn func() error
	stopFn  func(ctx context.Context) error
}

// NewLifecycleModule builds a module from explicit lifecycle functions. Either
// function may be nil (treated as a no-op).
func NewLifecycleModule(name string, deps []string, startFn func() error, stopFn func(ctx context.Context) error) *LifecycleModule {
	return &LifecycleModule{name: name, deps: deps, startFn: startFn, stopFn: stopFn}
}

// Name implements core.Module.
func (m *LifecycleModule) Name() string { return m.name }

// Dependencies implements core.Module.
func (m *LifecycleModule) Dependencies() []string { return m.deps }

// Init implements core.Module.
func (m *LifecycleModule) Init(_ core.Deps) error { return nil }

// Start implements core.Module.
func (m *LifecycleModule) Start() error {
	if m.startFn == nil {
		return nil
	}
	return m.startFn()
}

// Stop implements core.Module.
func (m *LifecycleModule) Stop(ctx context.Context) error {
	if m.stopFn == nil {
		return nil
	}
	return m.stopFn(ctx)
}

// Compile-time contract guarantee.
var _ core.Module = (*LifecycleModule)(nil)

// RuntimeModule owns the game tick driver. In dual-core mode it runs the Core1
// blocking loop on a dedicated goroutine (Core1 pins it to an OS thread); in
// single-core mode it runs its own catch-up ticker. It should depend on every
// other registered module so the container starts it last and stops it first —
// matching the original order where the loop stops before network shutdown,
// saves and engine teardown.
type RuntimeModule struct {
	tickFn    func(delta time.Duration)
	interval  time.Duration
	deps      []string
	dualCore  bool
	core1Run  func(interval time.Duration)
	core1Stop func()
	stopCh    chan struct{}
	doneCh    chan struct{}
	stopped   atomic.Bool
}

// NewRuntimeModule builds the tick driver. deps must list every module the
// container registers alongside it.
func NewRuntimeModule(tickFn func(delta time.Duration), interval time.Duration, deps []string) *RuntimeModule {
	if interval <= 0 {
		interval = time.Second / 60
	}
	return &RuntimeModule{
		tickFn:   tickFn,
		interval: interval,
		deps:     deps,
		stopCh:   make(chan struct{}),
		doneCh:   make(chan struct{}),
	}
}

// BindDualCore wires the dual-core game loop controls (Core1.Run / Core1.Stop).
func (m *RuntimeModule) BindDualCore(run func(time.Duration), stop func()) {
	m.dualCore = true
	m.core1Run = run
	m.core1Stop = stop
}

// Name implements core.Module.
func (m *RuntimeModule) Name() string { return "runtime" }

// Dependencies implements core.Module.
func (m *RuntimeModule) Dependencies() []string { return m.deps }

// Init implements core.Module.
func (m *RuntimeModule) Init(_ core.Deps) error { return nil }

// Start implements core.Module.
func (m *RuntimeModule) Start() error {
	if m.dualCore {
		if m.core1Run != nil {
			go m.core1Run(m.interval)
		}
		return nil
	}
	go m.loop()
	return nil
}

// Stop implements core.Module and halts the tick driver.
func (m *RuntimeModule) Stop(_ context.Context) error {
	if !m.stopped.CompareAndSwap(false, true) {
		return nil
	}
	if m.dualCore {
		if m.core1Stop != nil {
			m.core1Stop()
		}
		return nil
	}
	close(m.stopCh)
	<-m.doneCh
	return nil
}

func (m *RuntimeModule) loop() {
	defer close(m.doneCh)
	interval := m.interval
	next := time.Now().Add(interval)
	const maxCatchUp = 4
	for {
		select {
		case <-m.stopCh:
			return
		default:
		}
		now := time.Now()
		if now.Before(next) {
			timer := time.NewTimer(next.Sub(now))
			select {
			case <-timer.C:
			case <-m.stopCh:
				if !timer.Stop() {
					<-timer.C
				}
				return
			}
			continue
		}
		steps := 0
		for !now.Before(next) && steps < maxCatchUp {
			if m.tickFn != nil {
				m.tickFn(interval)
			}
			steps++
			next = next.Add(interval)
			now = time.Now()
		}
		if steps == maxCatchUp && !now.Before(next) {
			next = now.Add(interval)
		}
	}
}

// Compile-time contract guarantee.
var _ core.Module = (*RuntimeModule)(nil)
