package plugin

import (
	"context"

	"mdt-server/internal/core"
)

// Module adapts the plugin Registry to the core.Module contract so the
// extension system participates in the container lifecycle. It depends on
// net (player access) and world (game state) by default.
type Module struct {
	registry *Registry
	watcher  *FileWatcher
	deps     []string
	cancel   context.CancelFunc
}

// NewModule wraps a registry for container management.
func NewModule(registry *Registry, deps ...string) *Module {
	if len(deps) == 0 {
		deps = []string{"net", "world"}
	}
	return &Module{registry: registry, deps: deps}
}

// Name implements core.Module.
func (m *Module) Name() string { return "plugins" }

// Dependencies implements core.Module.
func (m *Module) Dependencies() []string { return m.deps }

// Init implements core.Module.
func (m *Module) Init(_ core.Deps) error { return nil }

// Start implements core.Module: scan, resolve, load plugins, start the
// reload dispatcher and the file watcher.
func (m *Module) Start() error {
	if m.registry == nil {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	if err := m.registry.ScanAndLoad(ctx); err != nil {
		cancel()
		return err
	}
	go m.registry.ReloadLoop(ctx)
	m.watcher = NewFileWatcher(m.registry)
	m.watcher.Start()
	return nil
}

// Stop implements core.Module: stop the watcher, stop plugins in reverse
// load order and cancel the reload loop.
func (m *Module) Stop(ctx context.Context) error {
	if m.registry == nil {
		return nil
	}
	if m.watcher != nil {
		m.watcher.Stop()
		m.watcher = nil
	}
	if m.cancel != nil {
		m.cancel()
	}
	m.registry.StopAll(ctx)
	return nil
}

// Registry exposes the underlying registry for host wiring (event firing,
// command dispatch).
func (m *Module) Registry() *Registry { return m.registry }

// Compile-time contract guarantee.
var _ core.Module = (*Module)(nil)
