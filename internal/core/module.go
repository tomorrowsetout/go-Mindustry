// Package core defines the modular architecture for the mdt-server.
//
// Design goal (from project owner): the server is built as a set of swappable
// modules. Core networking stays in Go, but every module exposes a clean
// interface so it can later be replaced wholesale — for example by a module
// written in another language (C, Rust, ...) if that language runs a given
// task faster. The boundary is the interface, not the implementation.
//
// A Module is any self-contained server subsystem (net, world, sim, logic,
// persist, api, ...). The Container owns the lifecycle and starts/stops
// modules in dependency order with graceful shutdown.
package core

import (
	"context"
	"fmt"
	"sort"
	"sync"
)

// Module is the lifecycle contract every swappable server subsystem must satisfy.
//
// Implementations may be written in Go today and replaced by a foreign-language
// module tomorrow, as long as they keep this same interface. The Container
// only ever talks to a module through Module (and, where relevant, the more
// specific contracts in contracts.go).
type Module interface {
	// Name returns a stable, unique module identifier, e.g. "net", "world".
	Name() string

	// Dependencies lists the module names that must be started before this one.
	// The Container topologically sorts startup using this information.
	Dependencies() []string

	// Init prepares the module. deps carries already-initialized modules so a
	// module can grab references to the interfaces it needs (e.g. the net
	// module needs the world module). Init must be safe to call once.
	Init(deps Deps) error

	// Start begins the module's runtime work (listening, ticking, ...).
	// It should return quickly; long-running work belongs in goroutines.
	Start() error

	// Stop requests a graceful shutdown. It must return once in-flight work
	// has been drained or a context deadline forces teardown.
	Stop(ctx context.Context) error
}

// LifecycleState is the current phase of a module.
type LifecycleState int32

const (
	// StateRegistered means the module is known to the Container but not yet Init'd.
	StateRegistered LifecycleState = iota
	// StateInitializing means Init is running.
	StateInitializing
	// StateInitialized means Init succeeded, before Start.
	StateInitialized
	// StateRunning means Start succeeded and the module is active.
	StateRunning
	// StateStopping means Stop is running.
	StateStopping
	// StateStopped means the module has been torn down.
	StateStopped
	// StateFailed means Init or Start returned an error.
	StateFailed
)

func (s LifecycleState) String() string {
	switch s {
	case StateRegistered:
		return "registered"
	case StateInitializing:
		return "initializing"
	case StateInitialized:
		return "initialized"
	case StateRunning:
		return "running"
	case StateStopping:
		return "stopping"
	case StateStopped:
		return "stopped"
	case StateFailed:
		return "failed"
	default:
		return "unknown"
	}
}

// Deps is the read-only view a module receives during Init. It lets a module
// look up a sibling by name and assert the interface it expects.
type Deps interface {
	// Get returns the registered module with the given name, or nil.
	Get(name string) Module
	// Must returns the module with the given name, or panics if absent.
	Must(name string) Module
}

// Container owns the module set and drives the lifecycle.
//
// It is intentionally small and free of game logic so it can be reused by any
// future module host (including a foreign-function module loader).
type Container struct {
	mu      sync.Mutex
	modules map[string]Module
	state   map[string]LifecycleState
	order   []string
}

// NewContainer returns an empty module container.
func NewContainer() *Container {
	return &Container{
		modules: make(map[string]Module),
		state:   make(map[string]LifecycleState),
	}
}

// Register adds a module. It panics on duplicate names — that is a programming
// error, not a runtime condition.
func (c *Container) Register(m Module) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.modules[m.Name()]; ok {
		panic(fmt.Sprintf("core: duplicate module %q", m.Name()))
	}
	c.modules[m.Name()] = m
	c.state[m.Name()] = StateRegistered
}

// Get returns a registered module by name.
func (c *Container) Get(name string) Module {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.modules[name]
}

// Must returns a module by name or panics.
func (c *Container) Must(name string) Module {
	c.mu.Lock()
	defer c.mu.Unlock()
	m, ok := c.modules[name]
	if !ok {
		panic(fmt.Sprintf("core: missing required module %q", name))
	}
	return m
}

// State returns the current lifecycle state of a module.
func (c *Container) State(name string) LifecycleState {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.state[name]
}

// Names returns all registered module names.
func (c *Container) Names() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	out := make([]string, 0, len(c.modules))
	for n := range c.modules {
		out = append(out, n)
	}
	return out
}

// orderedStart returns module names sorted so dependencies precede dependents.
// It returns an error on cycles.
func (c *Container) orderedStart() ([]string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.orderedStartLocked()
}

// orderedStartLocked performs the topological sort; the caller must hold c.mu.
func (c *Container) orderedStartLocked() ([]string, error) {
	names := make([]string, 0, len(c.modules))
	for n := range c.modules {
		names = append(names, n)
	}
	sort.Strings(names) // deterministic tie-break
	seen := make(map[string]bool)
	var out []string
	var visit func(n string, stack []string) error
	visit = func(n string, stack []string) error {
		if seen[n] {
			return nil
		}
		for _, dep := range c.modules[n].Dependencies() {
			if dep == n {
				continue
			}
			if _, ok := c.modules[dep]; !ok {
				return fmt.Errorf("core: module %q depends on unknown module %q", n, dep)
			}
			for _, s := range stack {
				if s == dep {
					return fmt.Errorf("core: dependency cycle: %v -> %q", stack, dep)
				}
			}
			if err := visit(dep, append(stack, n)); err != nil {
				return err
			}
		}
		seen[n] = true
		out = append(out, n)
		return nil
	}
	for _, n := range names {
		if err := visit(n, nil); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// Init initializes every module in dependency order.
func (c *Container) Init() error {
	c.mu.Lock()
	order, err := c.orderedStartLocked()
	c.order = order
	deps := &containerDeps{c: c}
	c.mu.Unlock()
	if err != nil {
		return err
	}
	for _, n := range order {
		c.mu.Lock()
		c.state[n] = StateInitializing
		c.mu.Unlock()
		if err := c.modules[n].Init(deps); err != nil {
			c.mu.Lock()
			c.state[n] = StateFailed
			c.mu.Unlock()
			return fmt.Errorf("core: module %q init failed: %w", n, err)
		}
		c.mu.Lock()
		c.state[n] = StateInitialized
		c.mu.Unlock()
	}
	return nil
}

// Start starts every module in dependency order.
func (c *Container) Start() error {
	c.mu.Lock()
	order := c.order
	c.mu.Unlock()
	for _, n := range order {
		if err := c.modules[n].Start(); err != nil {
			c.mu.Lock()
			c.state[n] = StateFailed
			c.mu.Unlock()
			return fmt.Errorf("core: module %q start failed: %w", n, err)
		}
		c.mu.Lock()
		c.state[n] = StateRunning
		c.mu.Unlock()
	}
	return nil
}

// Stop stops every module in reverse dependency order.
func (c *Container) Stop(ctx context.Context) error {
	c.mu.Lock()
	order := c.order
	c.mu.Unlock()
	// reverse
	for i := len(order) - 1; i >= 0; i-- {
		n := order[i]
		c.mu.Lock()
		c.state[n] = StateStopping
		c.mu.Unlock()
		if err := c.modules[n].Stop(ctx); err != nil {
			c.mu.Lock()
			c.state[n] = StateFailed
			c.mu.Unlock()
			return fmt.Errorf("core: module %q stop failed: %w", n, err)
		}
		c.mu.Lock()
		c.state[n] = StateStopped
		c.mu.Unlock()
	}
	return nil
}

// Run is a convenience that Inits, Starts, and waits for the given context to
// be cancelled, then Stops everything in reverse order.
func (c *Container) Run(ctx context.Context) error {
	if err := c.Init(); err != nil {
		return err
	}
	if err := c.Start(); err != nil {
		return err
	}
	<-ctx.Done()
	return c.Stop(ctx)
}

type containerDeps struct {
	c *Container
}

func (d *containerDeps) Get(name string) Module  { return d.c.Get(name) }
func (d *containerDeps) Must(name string) Module { return d.c.Must(name) }
