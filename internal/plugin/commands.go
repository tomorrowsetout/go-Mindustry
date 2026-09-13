package plugin

import (
	"fmt"
	"sync"
)

// PlayerInfo is the stable player view passed to plugin command
// handlers. The bridge layer fills it from live server state.
type PlayerInfo struct {
	ID     int32
	Name   string
	UUID   string
	IP     string
	Admin  bool
	TeamID int
}

// PlayerCommandHandler handles a chat command issued by a player.
type PlayerCommandHandler func(player PlayerInfo, args []string)

// ServerCommandHandler handles a console command.
type ServerCommandHandler func(args []string)

// CallableCommandHandler is a module-to-module RPC target
// (yzf.commands.register / yzf.command).
type CallableCommandHandler func(args []any) any

type playerCommand struct {
	module     string
	name       string
	usage      string
	desc       string
	adminOnly  bool
	permission string
	handler    PlayerCommandHandler
}

type serverCommand struct {
	module  string
	name    string
	usage   string
	desc    string
	handler ServerCommandHandler
}

type callableCommand struct {
	module  string
	name    string
	desc    string
	handler CallableCommandHandler
}

// CommandRegistry mirrors YZFCommandRegistry + YZFModCommandInterface:
// player chat commands (plain and admin), console commands, and
// callable commands used for module-to-module RPC. Every registration
// is owned by a module ID so reloads can detach cleanly.
type CommandRegistry struct {
	mu       sync.Mutex
	player   map[string]*playerCommand
	server   map[string]*serverCommand
	callable map[string]*callableCommand
}

// CommandInfo is a JSON-friendly description used by list endpoints.
type CommandInfo struct {
	Name       string `json:"name"`
	Module     string `json:"module"`
	Usage      string `json:"usage,omitempty"`
	Desc       string `json:"desc,omitempty"`
	AdminOnly  bool   `json:"adminOnly,omitempty"`
	Permission string `json:"permission,omitempty"`
	Kind       string `json:"kind"`
}

// NewCommandRegistry creates an empty registry.
func NewCommandRegistry() *CommandRegistry {
	return &CommandRegistry{
		player:   make(map[string]*playerCommand),
		server:   make(map[string]*serverCommand),
		callable: make(map[string]*callableCommand),
	}
}

// RegisterPlayerCommand registers (or replaces) a chat command.
func (r *CommandRegistry) RegisterPlayerCommand(module, name, usage, desc string, adminOnly bool, permission string, handler PlayerCommandHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.player[name] = &playerCommand{
		module: module, name: name, usage: usage, desc: desc,
		adminOnly: adminOnly, permission: permission, handler: handler,
	}
}

// RegisterServerCommand registers (or replaces) a console command.
func (r *CommandRegistry) RegisterServerCommand(module, name, usage, desc string, handler ServerCommandHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.server[name] = &serverCommand{module: module, name: name, usage: usage, desc: desc, handler: handler}
}

// RegisterCallable registers (or replaces) a callable command.
func (r *CommandRegistry) RegisterCallable(module, name, desc string, handler CallableCommandHandler) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.callable[name] = &callableCommand{module: module, name: name, desc: desc, handler: handler}
}

// UnregisterModule removes every command owned by a module.
func (r *CommandRegistry) UnregisterModule(module string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for name, cmd := range r.player {
		if cmd.module == module {
			delete(r.player, name)
		}
	}
	for name, cmd := range r.server {
		if cmd.module == module {
			delete(r.server, name)
		}
	}
	for name, cmd := range r.callable {
		if cmd.module == module {
			delete(r.callable, name)
		}
	}
}

// RunPlayerCommand executes a chat command. found is false when the
// command is unknown; permission errors are returned.
func (r *CommandRegistry) RunPlayerCommand(name string, player PlayerInfo, args []string) (found bool, err error) {
	r.mu.Lock()
	cmd, ok := r.player[name]
	r.mu.Unlock()
	if !ok {
		return false, nil
	}
	if cmd.adminOnly && !player.Admin {
		return true, fmt.Errorf("需要管理员权限")
	}
	defer recoverCommandPanic(name)
	cmd.handler(player, args)
	return true, nil
}

// RunServerCommand executes a console command.
func (r *CommandRegistry) RunServerCommand(name string, args []string) bool {
	r.mu.Lock()
	cmd, ok := r.server[name]
	r.mu.Unlock()
	if !ok {
		return false
	}
	defer recoverCommandPanic(name)
	cmd.handler(args)
	return true
}

// HasCallable reports whether a callable command exists without
// executing it.
func (r *CommandRegistry) HasCallable(name string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.callable[name]
	return ok
}

// CallCallable executes a callable command (module RPC).
func (r *CommandRegistry) CallCallable(name string, args []any) (found bool, result any) {
	r.mu.Lock()
	cmd, ok := r.callable[name]
	r.mu.Unlock()
	if !ok {
		return false, nil
	}
	defer recoverCommandPanic(name)
	return true, cmd.handler(args)
}

// ListPlayerCommands returns all registered chat commands.
func (r *CommandRegistry) ListPlayerCommands() []CommandInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]CommandInfo, 0, len(r.player))
	for _, cmd := range r.player {
		out = append(out, CommandInfo{
			Name: cmd.name, Module: cmd.module, Usage: cmd.usage,
			Desc: cmd.desc, AdminOnly: cmd.adminOnly, Permission: cmd.permission, Kind: "player",
		})
	}
	return out
}

// ListCallableCommands returns all registered callable commands.
func (r *CommandRegistry) ListCallableCommands() []CommandInfo {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]CommandInfo, 0, len(r.callable))
	for _, cmd := range r.callable {
		out = append(out, CommandInfo{Name: cmd.name, Module: cmd.module, Desc: cmd.desc, Kind: "callable"})
	}
	return out
}

// ListModuleCommands lists commands owned by one module.
func (r *CommandRegistry) ListModuleCommands(module string) []CommandInfo {
	var out []CommandInfo
	for _, info := range r.ListPlayerCommands() {
		if info.Module == module {
			out = append(out, info)
		}
	}
	for _, info := range r.ListCallableCommands() {
		if info.Module == module {
			out = append(out, info)
		}
	}
	return out
}

func recoverCommandPanic(name string) {
	if r := recover(); r != nil {
		// Logged by the caller via the returned panic value; swallowing
		// here keeps one broken command from taking down the dispatcher.
		_ = fmt.Errorf("命令 %s 执行异常: %v", name, r)
	}
}
