package core

// This file defines the swap boundary for each server subsystem.
//
// Every module exposes a narrow contract. The implementation today is Go, but
// any module can later be rewritten in another language (C, Rust, ...) as long
// as it keeps the same contract — the Container and the rest of the server only
// ever talk to the interface, never the concrete type. Capability methods are
// intentionally minimal so a replacement focuses on behavior, not internals.
//
// NOTE: core must NOT import the concrete subsystem packages (net, world, ...)
// to avoid import cycles. Keep these contracts free of those types — use `any`
// for payloads and `string` for addresses.

// NetModule owns the wire protocol: listening, connection admission, and
// broadcast of server-authored packets to connected clients.
type NetModule interface {
	Module
	// Broadcast sends a server-authored packet to all connected clients.
	Broadcast(obj any) error
}

// WorldModule owns the authoritative world state (tiles, blocks, teams).
type WorldModule interface {
	Module
	// LoadMap loads a map payload — the bytes carried by the WorldStream
	// packet during join and resync.
	LoadMap(payload []byte) error
}

// SimModule advances the simulation each tick (units, power, items, waves).
type SimModule interface {
	Module
	// Step advances the simulation by one tick.
	Step() error
}

// LogicModule runs the Mindustry Logic programming language.
type LogicModule interface {
	Module
	// Run executes a logic fragment and returns its output. Kept narrow so a
	// reimplementation (or a language-specific runtime) only honors this.
	Run(source string) (string, error)
}

// PersistModule stores durable server state (bans, ranks, world saves).
type PersistModule interface {
	Module
	// Save writes a key/value pair; Load reads it back.
	Save(key string, value []byte) error
	// Load returns nil, nil when the key is absent.
	Load(key string) ([]byte, error)
}

// APIModule exposes the admin / HTTP surface.
type APIModule interface {
	Module
	// Route registers an HTTP-style handler. The handler type is `any` so the
	// contract stays independent of the concrete web framework.
	Route(pattern string, handler any) error
}
