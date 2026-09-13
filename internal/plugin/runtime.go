package plugin

import "context"

// Runtime executes one loaded module. The registry drives lifecycle:
// Start (initial load), OnEnable/OnDisable (enable toggles), FireEvent
// (server events), Stop (unload).
type Runtime interface {
	Kind() string
	Start(ctx context.Context) error
	Stop(ctx context.Context) error
	OnEnable(ctx context.Context)
	OnDisable(ctx context.Context)
	FireEvent(ctx context.Context, event string, payload EventPayload) bool
}

// ServerAPI is the capability surface the host server exposes to
// plugin runtimes. All methods are optional: runtimes degrade to safe
// defaults when the host does not implement a capability interface.
type ServerAPI interface {
	Logf(format string, args ...any)
}

// PlayerAPI exposes player management to plugins.
type PlayerAPI interface {
	PlayerList() []PlayerInfo
	PlayerFind(nameOrID string) *PlayerInfo
	PlayerCount() int
	PlayerSend(id int32, msg string)
	PlayerKick(id int32, reason string)
	PlayerBan(id int32)
}

// NetAPI exposes chat broadcast to plugins.
type NetAPI interface {
	BroadcastChat(msg string)
	BroadcastChatFrom(msg string, senderID int32)
}

// GameAPI exposes game state to plugins.
type GameAPI interface {
	GameTick() uint64
	GameTPS() int
	GameWave() int
	GameIsPlaying() bool
	GameIsPaused() bool
	GameMapName() string
}

// LoadedModule mirrors YZFLoadedModule: a started module plus its
// runtime bookkeeping.
type LoadedModule struct {
	Def     *Definition
	Runtime Runtime
	Config  *ConfigStore
	Enabled bool
}
