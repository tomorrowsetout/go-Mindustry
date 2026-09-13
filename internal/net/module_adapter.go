package net

import (
	"context"
	"fmt"
	"os"
	"time"

	"mdt-server/internal/core"
)

// NetModuleAdapter adapts *Server to the core.Module / core.NetModule contract.
//
// This exists to prove the existing network layer already fits the modular
// swap boundary: the server can be hosted by core.Container and later replaced
// by a foreign-language net module without touching the rest of the server.
type NetModuleAdapter struct {
	srv  *Server
	deps []string
}

// NewNetModule wraps an already-constructed *Server as a swappable module.
// deps optionally lists modules that must start before the listener.
func NewNetModule(srv *Server, deps ...string) *NetModuleAdapter {
	return &NetModuleAdapter{srv: srv, deps: deps}
}

// Name implements core.Module.
func (a *NetModuleAdapter) Name() string { return "net" }

// Dependencies implements core.Module.
func (a *NetModuleAdapter) Dependencies() []string { return a.deps }

// Init implements core.Module. The server is already constructed; nothing to do.
func (a *NetModuleAdapter) Init(_ core.Deps) error { return nil }

// Start implements core.Module by serving in a background goroutine. A bind
// failure is fatal for the whole server, mirroring the original startup path.
func (a *NetModuleAdapter) Start() error {
	go func() {
		if err := a.srv.Serve(); err != nil {
			fmt.Fprintf(os.Stderr, "服务器启动失败: %v\n", err)
			os.Exit(1)
		}
	}()
	return nil
}

// Stop implements core.Module. It kicks connected players with a short grace
// period (so the kick packets actually leave the socket) and then shuts the
// listener and all connections down.
func (a *NetModuleAdapter) Stop(ctx context.Context) error {
	const kickReason = "服务器正在关闭"
	const kickDelay = 400 * time.Millisecond
	notifyDone := make(chan int, 1)
	go func() {
		notifyDone <- a.srv.NotifyShutdown(kickReason, kickDelay)
	}()
	select {
	case notified := <-notifyDone:
		if notified > 0 {
			time.Sleep(kickDelay + 100*time.Millisecond)
		}
	case <-time.After(1200 * time.Millisecond):
	case <-ctx.Done():
	}
	a.srv.Shutdown()
	return nil
}

// Broadcast implements core.NetModule.
func (a *NetModuleAdapter) Broadcast(obj any) error {
	a.srv.Broadcast(obj)
	return nil
}

// Compile-time guarantees that the adapter satisfies the contracts.
var (
	_ core.Module    = (*NetModuleAdapter)(nil)
	_ core.NetModule = (*NetModuleAdapter)(nil)
)
