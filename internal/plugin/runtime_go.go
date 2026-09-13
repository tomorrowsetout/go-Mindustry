package plugin

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
)

// goRuntime runs a Go-native plugin as a child process speaking
// line-delimited JSON-RPC over stdin/stdout. This mirrors YZF's
// process runtime (YZFProcessRuntime) while giving first-class Go
// plugins crash isolation and hot-reloadability that Go's plugin
// package cannot provide.
//
// Wire protocol (one JSON object per line, both directions):
//
//	server -> plugin: {"id":1,"method":"init","params":{...}}
//	plugin -> server: {"id":1,"result":{...}}  /  {"id":1,"error":"..."}
//	plugin -> server: {"method":"log","params":{...}}   (notification, no id)
type goRuntime struct {
	def  *Definition
	api  *Bridge
	verb func(format string, args ...any)

	cmd    *exec.Cmd
	stdin  *json.Encoder
	stdout *bufio.Reader
	writeMu sync.Mutex // serializes every stdin write

	mu        sync.Mutex
	nextID    int64
	pending   map[int64]chan rpcMessage
	closed    bool
	started   atomic.Bool
	processMu sync.Mutex
}

type rpcMessage struct {
	ID     int64           `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// GoRuntimeEnv describes the init handshake payload.
type GoRuntimeEnv struct {
	Meta     json.RawMessage `json:"meta"`
	FullID   string          `json:"fullId"`
	Root     string          `json:"root"`
	DataDir  string          `json:"dataDir"`
	CacheDir string          `json:"cacheDir"`
	Mode     string          `json:"mode"`
}

// NewGoRuntime creates the subprocess runtime for a Go plugin. The
// module's main entry must be an executable (built .exe/.bin) or a
// .go source file executed via `go run`.
func NewGoRuntime(def *Definition, api *Bridge, verb func(format string, args ...any)) Runtime {
	if verb == nil {
		verb = func(string, ...any) {}
	}
	return &goRuntime{
		def:     def,
		api:     api,
		verb:    verb,
		pending: make(map[int64]chan rpcMessage),
	}
}

// Kind reports the runtime identifier used in module.hjson.
func (r *goRuntime) Kind() string { return "go" }

// Start launches the plugin process and performs the init handshake.
func (r *goRuntime) Start(ctx context.Context) error {
	argv, err := r.launchCommand()
	if err != nil {
		return err
	}
	r.cmd = exec.CommandContext(ctx, argv[0], argv[1:]...)
	r.cmd.Dir = r.def.Root
	r.cmd.Env = append(os.Environ(), "YZF_PLUGIN=1", "YZF_FULL_ID="+r.def.FullID())

	stdinPipe, err := r.cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdoutPipe, err := r.cmd.StdoutPipe()
	if err != nil {
		return err
	}
	r.cmd.Stderr = rpcLogWriter{verb: r.verb, prefix: "[" + r.def.FullID() + "]"}
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("启动插件进程失败: %w", err)
	}

	r.stdin = json.NewEncoder(stdinPipe)
	r.stdout = bufio.NewReader(stdoutPipe)
	go r.readLoop()

	env := GoRuntimeEnv{
		Meta:     json.RawMessage(r.def.metaJSON()),
		FullID:   r.def.FullID(),
		Root:     r.def.Root,
		DataDir:  r.def.DataDir,
		CacheDir: r.def.CacheDir,
		Mode:     "go-rpc",
	}
	if _, err := r.call("init", env); err != nil {
		_ = r.cmd.Process.Kill()
		return fmt.Errorf("插件 init 握手失败: %w", err)
	}
	r.started.Store(true)
	return nil
}

// launchCommand resolves the plugin executable from meta.main.
func (r *goRuntime) launchCommand() ([]string, error) {
	main := filepath.FromSlash(r.def.Meta.Main)
	if main == "" {
		main = "main.go"
	}
	path := filepath.Join(r.def.Root, main)
	ext := filepath.Ext(path)
	switch ext {
	case ".go":
		// Run source directly via go run.
		return []string{"go", "run", path}, nil
	case ".exe", ".bin", "":
		if fileExists(path) {
			return []string{path}, nil
		}
		return nil, fmt.Errorf("找不到插件可执行文件 %s", path)
	default:
		return nil, fmt.Errorf("不支持的 Go 插件入口类型: %s", main)
	}
}

// Stop terminates the plugin process.
func (r *goRuntime) Stop(ctx context.Context) error {
	if r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	r.processMu.Lock()
	defer r.processMu.Unlock()
	// Give the plugin a chance to flush; fall back to kill.
	_, _ = r.call("shutdown", nil)
	_ = r.cmd.Process.Kill()
	_, _ = r.cmd.Process.Wait()
	r.mu.Lock()
	r.closed = true
	for _, ch := range r.pending {
		close(ch)
	}
	r.pending = make(map[int64]chan rpcMessage)
	r.mu.Unlock()
	r.started.Store(false)
	return nil
}

// OnEnable notifies the plugin process.
func (r *goRuntime) OnEnable(ctx context.Context) {
	_, _ = r.notify("enable", nil)
}

// OnDisable notifies the plugin process.
func (r *goRuntime) OnDisable(ctx context.Context) {
	_, _ = r.notify("disable", nil)
}

// FireEvent forwards a server event to the plugin process and reports
// whether the plugin consumed it.
func (r *goRuntime) FireEvent(ctx context.Context, event string, payload EventPayload) bool {
	if !r.started.Load() {
		return true
	}
	result, err := r.call("event", map[string]any{"event": event, "payload": payload})
	if err != nil {
		return true // unreachable plugin must not cancel the chain
	}
	var out struct {
		Consumed bool `json:"consumed"`
	}
	if json.Unmarshal(result, &out) != nil {
		return true
	}
	return !out.Consumed
}

// writeLocked serializes a single JSON line onto the plugin's stdin.
func (r *goRuntime) writeLocked(msg rpcMessage) error {
	r.writeMu.Lock()
	defer r.writeMu.Unlock()
	if r.stdin == nil {
		return fmt.Errorf("插件进程未启动")
	}
	return r.stdin.Encode(msg)
}

// call sends a request and waits for the matching response.
func (r *goRuntime) call(method string, params any) (json.RawMessage, error) {
	id := atomic.AddInt64(&r.nextID, 1)
	payload, err := json.Marshal(params)
	if err != nil {
		return nil, err
	}
	ch := make(chan rpcMessage, 1)
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil, fmt.Errorf("插件进程已关闭")
	}
	r.pending[id] = ch
	r.mu.Unlock()

	if err := r.writeLocked(rpcMessage{ID: id, Method: method, Params: payload}); err != nil {
		r.mu.Lock()
		delete(r.pending, id)
		r.mu.Unlock()
		return nil, err
	}
	reply, ok := <-ch
	if !ok {
		return nil, fmt.Errorf("插件进程已关闭")
	}
	if reply.Error != "" {
		return nil, fmt.Errorf("%s", reply.Error)
	}
	return reply.Result, nil
}

// notify sends a fire-and-forget message.
func (r *goRuntime) notify(method string, params any) (bool, error) {
	payload, err := json.Marshal(params)
	if err != nil {
		return false, err
	}
	r.mu.Lock()
	closed := r.closed
	r.mu.Unlock()
	if closed {
		return false, fmt.Errorf("插件进程已关闭")
	}
	if err := r.writeLocked(rpcMessage{Method: method, Params: payload}); err != nil {
		return false, err
	}
	return true, nil
}

// readLoop dispatches plugin -> server messages.
func (r *goRuntime) readLoop() {
	for {
		line, err := r.stdout.ReadBytes('\n')
		if err != nil {
			r.failAll(fmt.Errorf("插件进程输出中断: %w", err))
			return
		}
		var msg rpcMessage
		if json.Unmarshal(line, &msg) != nil {
			continue
		}
		if msg.ID != 0 && (msg.Result != nil || msg.Error != "") {
			r.mu.Lock()
			ch, ok := r.pending[msg.ID]
			if ok {
				delete(r.pending, msg.ID)
			}
			r.mu.Unlock()
			if ok {
				ch <- msg
			}
			continue
		}
		if msg.Method != "" {
			r.handlePluginCall(msg)
		}
	}
}

// handlePluginCall serves requests issued by the plugin process.
func (r *goRuntime) handlePluginCall(msg rpcMessage) {
	fullID := r.def.FullID()
	reply := rpcMessage{ID: msg.ID}

	switch msg.Method {
	case "log":
		var p struct {
			Level   string `json:"level"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(msg.Params, &p)
		r.verb("[%s] [%s] %s", p.Level, r.def.Meta.Name, p.Message)

	case "configGet":
		var p struct{ Key, Def string }
		_ = json.Unmarshal(msg.Params, &p)
		reply.Result = mustJSON(r.api.config(fullID).GetString(p.Key, p.Def))
	case "configSet":
		var p struct{ Key, Value string }
		_ = json.Unmarshal(msg.Params, &p)
		_ = r.api.config(fullID).SetString(p.Key, p.Value)
		reply.Result = mustJSON(true)

	case "playerList":
		reply.Result = mustJSON(r.api.playerList())
	case "playerCount":
		reply.Result = mustJSON(r.api.playerCount())
	case "playerFind":
		var p struct{ NameOrID string }
		_ = json.Unmarshal(msg.Params, &p)
		reply.Result = mustJSON(r.api.playerFind(p.NameOrID))
	case "playerSend":
		var p struct {
			ID  int32
			Msg string
		}
		_ = json.Unmarshal(msg.Params, &p)
		r.api.playerSend(p.ID, p.Msg)
		reply.Result = mustJSON(true)
	case "playerKick":
		var p struct {
			ID     int32
			Reason string
		}
		_ = json.Unmarshal(msg.Params, &p)
		r.api.playerKick(p.ID, p.Reason)
		reply.Result = mustJSON(true)

	case "netBroadcast":
		var p struct{ Msg string }
		_ = json.Unmarshal(msg.Params, &p)
		r.api.broadcast(p.Msg)
		reply.Result = mustJSON(true)
	case "netBroadcastFrom":
		var p struct {
			Msg      string
			SenderID int32
		}
		_ = json.Unmarshal(msg.Params, &p)
		r.api.broadcastFrom(p.Msg, p.SenderID)
		reply.Result = mustJSON(true)

	case "gameTick":
		reply.Result = mustJSON(r.api.gameTick())
	case "gameTPS":
		reply.Result = mustJSON(r.api.gameTPS())
	case "gameWave":
		reply.Result = mustJSON(r.api.gameWave())
	case "gameIsPlaying":
		reply.Result = mustJSON(r.api.gameIsPlaying())
	case "gameMapName":
		reply.Result = mustJSON(r.api.gameMapName())

	case "registerPlayerCommand":
		var p struct {
			Name, Usage, Desc string
			AdminOnly         bool
			Permission        string
		}
		_ = json.Unmarshal(msg.Params, &p)
		r.api.commands.RegisterPlayerCommand(fullID, p.Name, p.Usage, p.Desc, p.AdminOnly, p.Permission,
			r.rpcPlayerHandler(p.Name))
		reply.Result = mustJSON(true)
	case "registerServerCommand":
		var p struct{ Name, Usage, Desc string }
		_ = json.Unmarshal(msg.Params, &p)
		r.api.commands.RegisterServerCommand(fullID, p.Name, p.Usage, p.Desc, r.rpcServerHandler(p.Name))
		reply.Result = mustJSON(true)
	case "registerCallable":
		var p struct{ Name, Desc string }
		_ = json.Unmarshal(msg.Params, &p)
		r.api.commands.RegisterCallable(fullID, p.Name, p.Desc, r.rpcCallableHandler(p.Name))
		reply.Result = mustJSON(true)

	case "command":
		var p struct {
			Name string
			Args []any
		}
		_ = json.Unmarshal(msg.Params, &p)
		found, result := r.api.commands.CallCallable(p.Name, p.Args)
		if !found {
			reply.Error = "未找到可调用命令 " + p.Name
		} else {
			reply.Result = mustJSON(result)
		}

	case "moduleList":
		reply.Result = mustJSON(r.api.moduleList())
	case "moduleInfo":
		var p struct{ ModuleID string }
		_ = json.Unmarshal(msg.Params, &p)
		reply.Result = mustJSON(r.api.moduleInfo(p.ModuleID))

	default:
		reply.Error = "未知方法 " + msg.Method
	}

	if msg.ID != 0 {
		r.mu.Lock()
		closed := r.closed
		r.mu.Unlock()
		if !closed {
			_ = r.writeLocked(reply)
		}
	}
}

// rpcPlayerHandler forwards player command invocations to the plugin.
func (r *goRuntime) rpcPlayerHandler(name string) PlayerCommandHandler {
	return func(player PlayerInfo, args []string) {
		_, _ = r.call("playerCommand", map[string]any{
			"name": name, "player": player, "args": args,
		})
	}
}

// rpcServerHandler forwards console command invocations to the plugin.
func (r *goRuntime) rpcServerHandler(name string) ServerCommandHandler {
	return func(args []string) {
		_, _ = r.call("serverCommand", map[string]any{"name": name, "args": args})
	}
}

// rpcCallableHandler forwards callable command invocations to the plugin.
func (r *goRuntime) rpcCallableHandler(name string) CallableCommandHandler {
	return func(args []any) any {
		result, err := r.call("callableCommand", map[string]any{"name": name, "args": args})
		if err != nil {
			return map[string]any{"ok": false, "message": err.Error()}
		}
		var out any
		if json.Unmarshal(result, &out) != nil {
			return nil
		}
		return out
	}
}

func (r *goRuntime) failAll(err error) {
	r.mu.Lock()
	r.closed = true
	for _, ch := range r.pending {
		close(ch)
	}
	r.pending = make(map[int64]chan rpcMessage)
	r.mu.Unlock()
	r.verb("插件进程退出 (%s): %v", r.def.FullID(), err)
}

func mustJSON(v any) json.RawMessage {
	payload, err := json.Marshal(v)
	if err != nil {
		return json.RawMessage("null")
	}
	return payload
}

// rpcLogWriter forwards plugin stderr lines to the server log.
type rpcLogWriter struct {
	verb   func(format string, args ...any)
	prefix string
}

func (w rpcLogWriter) Write(p []byte) (int, error) {
	w.verb("%s stderr: %s", w.prefix, string(p))
	return len(p), nil
}
