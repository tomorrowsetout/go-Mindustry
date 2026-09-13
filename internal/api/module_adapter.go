package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"

	"mdt-server/internal/core"
)

// Module adapts the HTTP admin surface to the core.Module / core.APIModule
// contract. When a pre-bound listener is supplied, Start serves on it; this
// preserves the startup ordering where main binds the port before any module
// runs (so a bind failure aborts startup instead of surfacing mid-flight).
type Module struct {
	srv      *Server
	listener net.Listener
}

// NewModule wraps an already-constructed *Server. listener may be nil when the
// API is disabled.
func NewModule(srv *Server, listener net.Listener) *Module {
	return &Module{srv: srv, listener: listener}
}

// Name implements core.Module.
func (m *Module) Name() string { return "api" }

// Dependencies implements core.Module: the admin surface operates the net layer.
func (m *Module) Dependencies() []string { return []string{"net"} }

// Init implements core.Module.
func (m *Module) Init(_ core.Deps) error { return nil }

// Start implements core.Module by serving on the pre-bound listener.
func (m *Module) Start() error {
	if m.srv == nil {
		return nil
	}
	if m.listener == nil {
		return errors.New("api: no listener bound")
	}
	go func() { _ = m.srv.ServeListener(m.listener) }()
	return nil
}

// Stop implements core.Module and shuts the HTTP server down.
func (m *Module) Stop(ctx context.Context) error {
	if m.srv == nil {
		return nil
	}
	return m.srv.Shutdown(ctx)
}

// Route implements core.APIModule by registering an auth-wrapped handler.
func (m *Module) Route(pattern string, handler any) error {
	if m.srv == nil {
		return errors.New("api server disabled")
	}
	h, ok := handler.(http.HandlerFunc)
	if !ok {
		return fmt.Errorf("api: unsupported handler type %T", handler)
	}
	return m.srv.Route(pattern, h)
}

// Compile-time guarantees that the adapter satisfies the contracts.
var (
	_ core.Module    = (*Module)(nil)
	_ core.APIModule = (*Module)(nil)
)
