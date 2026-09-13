package plugin

import (
	"fmt"
	"sync"

	"github.com/dop251/goja"
)

// Bridge is the capability facade shared by every runtime. It routes
// plugin requests to the host server via the narrow capability
// interfaces (ServerAPI/PlayerAPI/NetAPI/GameAPI), owns per-module
// config stores, JS event subscriptions and cross-module calls.
type Bridge struct {
	mu       sync.Mutex
	server   ServerAPI
	players  PlayerAPI
	net      NetAPI
	game     GameAPI
	commands *CommandRegistry
	configs  map[string]*ConfigStore

	// eventSubs holds JS-side yzf.on registrations, keyed by
	// fullID -> event -> handlers. Cross-runtime delivery goes through
	// the registry; JS handlers additionally fan out here.
	eventSubs map[string]map[string][]goja.Callable

	// cross-module call hooks
	moduleListFn    func() []ModuleSummary
	moduleInfoFn    func(id string) *ModuleSummary
	moduleExportsFn func(id string) []string
	callModuleFn    func(id, fn string, args []any) (any, error)
	// jsCallExport invokes an exported function of a loaded JS runtime.
	jsCallExport func(id, fn string, args []any) (any, error)
	// reload hooks
	reloadFn    func(id string) bool
	reloadAllFn func() bool
}

// ModuleSummary is the JSON-friendly module descriptor exposed via
// yzf.module.list / yzf.runtime.modules.
type ModuleSummary struct {
	FullID   string `json:"fullId"`
	ID       string `json:"id"`
	Name     string `json:"name"`
	Author   string `json:"author"`
	Version  string `json:"version"`
	Runtime  string `json:"runtime"`
	Category string `json:"category"`
	Enabled  bool   `json:"enabled"`
	Source   string `json:"source"`
}

// NewBridge wires the bridge to host capabilities.
func NewBridge(
	server ServerAPI,
	players PlayerAPI,
	net NetAPI,
	game GameAPI,
	commands *CommandRegistry,
) *Bridge {
	return &Bridge{
		server:    server,
		players:   players,
		net:       net,
		game:      game,
		commands:  commands,
		configs:   make(map[string]*ConfigStore),
		eventSubs: make(map[string]map[string][]goja.Callable),
	}
}

// WireCrossModule installs the registry hooks used for module listing
// and cross-module calls.
func (b *Bridge) WireCrossModule(
	listFn func() []ModuleSummary,
	infoFn func(id string) *ModuleSummary,
	exportsFn func(id string) []string,
	callFn func(id, fn string, args []any) (any, error),
) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.moduleListFn = listFn
	b.moduleInfoFn = infoFn
	b.moduleExportsFn = exportsFn
	b.callModuleFn = callFn
}

// WireReload installs the registry reload hooks.
func (b *Bridge) WireReload(reloadFn func(id string) bool, reloadAllFn func() bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.reloadFn = reloadFn
	b.reloadAllFn = reloadAllFn
}

// config returns (creating on demand) the module config store.
func (b *Bridge) config(fullID string) *ConfigStore {
	b.mu.Lock()
	defer b.mu.Unlock()
	if store, ok := b.configs[fullID]; ok {
		return store
	}
	// The registry pre-registers stores at load time; reaching here
	// means the runtime asked before registration (defensive path).
	return nil
}

// registerConfig pre-creates the config store for a loaded module.
func (b *Bridge) registerConfig(def *Definition) *ConfigStore {
	store := NewConfigStore(def)
	b.mu.Lock()
	b.configs[def.FullID()] = store
	b.mu.Unlock()
	return store
}

// dropConfig forgets a module's store handle on unload.
func (b *Bridge) dropConfig(fullID string) {
	b.mu.Lock()
	delete(b.configs, fullID)
	b.mu.Unlock()
}

// subscribeEvent records a JS event handler.
func (b *Bridge) subscribeEvent(fullID, event string, fn goja.Callable) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.eventSubs[fullID] == nil {
		b.eventSubs[fullID] = make(map[string][]goja.Callable)
	}
	b.eventSubs[fullID][event] = append(b.eventSubs[fullID][event], fn)
}

// eventHandlers returns a snapshot of one module's handlers for an event.
func (b *Bridge) eventHandlers(fullID, event string) []goja.Callable {
	b.mu.Lock()
	defer b.mu.Unlock()
	if perEvent, ok := b.eventSubs[fullID]; ok {
		return append([]goja.Callable(nil), perEvent[event]...)
	}
	return nil
}

// dropEvents removes every JS subscription owned by a module.
func (b *Bridge) dropEvents(fullID string) {
	b.mu.Lock()
	delete(b.eventSubs, fullID)
	b.mu.Unlock()
}

// unregisterCallable removes a callable command by name if owned by the
// module (used by yzf.commands.unregister).
func (b *Bridge) unregisterCallable(fullID, name string) {
	b.mu.Lock()
	cmd, ok := b.commands.callable[name]
	b.mu.Unlock()
	if ok && cmd.module == fullID {
		b.commands.mu.Lock()
		delete(b.commands.callable, name)
		b.commands.mu.Unlock()
	}
}

// --- capability routing (nil-safe) ---

func (b *Bridge) playerList() []PlayerInfo {
	if b.players == nil {
		return nil
	}
	return b.players.PlayerList()
}

func (b *Bridge) playerCount() int {
	if b.players == nil {
		return 0
	}
	return b.players.PlayerCount()
}

func (b *Bridge) playerFind(nameOrID string) *PlayerInfo {
	if b.players == nil {
		return nil
	}
	return b.players.PlayerFind(nameOrID)
}

func (b *Bridge) playerFindByID(id int32) *PlayerInfo {
	for _, player := range b.playerList() {
		if player.ID == id {
			info := player
			return &info
		}
	}
	return nil
}

func (b *Bridge) playerSend(id int32, msg string) {
	if b.players != nil {
		b.players.PlayerSend(id, msg)
	}
}

func (b *Bridge) playerKick(id int32, reason string) {
	if b.players != nil {
		b.players.PlayerKick(id, reason)
	}
}

func (b *Bridge) playerBan(id int32) {
	if b.players != nil {
		b.players.PlayerBan(id)
	}
}

func (b *Bridge) broadcast(msg string) {
	if b.net != nil {
		b.net.BroadcastChat(msg)
	}
}

func (b *Bridge) broadcastFrom(msg string, senderID int32) {
	if b.net != nil {
		b.net.BroadcastChatFrom(msg, senderID)
	}
}

func (b *Bridge) gameTick() uint64 {
	if b.game == nil {
		return 0
	}
	return b.game.GameTick()
}

func (b *Bridge) gameTPS() int {
	if b.game == nil {
		return 0
	}
	return b.game.GameTPS()
}

func (b *Bridge) gameWave() int {
	if b.game == nil {
		return 0
	}
	return b.game.GameWave()
}

func (b *Bridge) gameIsPlaying() bool {
	if b.game == nil {
		return false
	}
	return b.game.GameIsPlaying()
}

func (b *Bridge) gameIsPaused() bool {
	if b.game == nil {
		return false
	}
	return b.game.GameIsPaused()
}

func (b *Bridge) gameMapName() string {
	if b.game == nil {
		return ""
	}
	return b.game.GameMapName()
}

// --- cross-module routing ---

func (b *Bridge) moduleList() []ModuleSummary {
	b.mu.Lock()
	fn := b.moduleListFn
	b.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn()
}

func (b *Bridge) moduleInfo(id string) *ModuleSummary {
	b.mu.Lock()
	fn := b.moduleInfoFn
	b.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn(id)
}

func (b *Bridge) moduleExports(id string) []string {
	b.mu.Lock()
	fn := b.moduleExportsFn
	b.mu.Unlock()
	if fn == nil {
		return nil
	}
	return fn(id)
}

func (b *Bridge) callModule(id, fnName string, args []any) (any, error) {
	b.mu.Lock()
	callFn := b.callModuleFn
	jsExport := b.jsCallExport
	b.mu.Unlock()
	if callFn != nil {
		return callFn(id, fnName, args)
	}
	if jsExport != nil {
		return jsExport(id, fnName, args)
	}
	return nil, fmt.Errorf("模块间调用未接线")
}

func (b *Bridge) requestReload(id string) bool {
	b.mu.Lock()
	fn := b.reloadFn
	b.mu.Unlock()
	if fn == nil {
		return false
	}
	return fn(id)
}

func (b *Bridge) requestReloadAll() bool {
	b.mu.Lock()
	fn := b.reloadAllFn
	b.mu.Unlock()
	if fn == nil {
		return false
	}
	return fn()
}
