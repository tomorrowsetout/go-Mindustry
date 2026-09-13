package plugin

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/dop251/goja"
)

// jsCallTimeout bounds synchronous JS invocations made from server
// goroutines (event handlers, command dispatch) so a wedged script can
// never block the game loop.
const jsCallTimeout = 250 * time.Millisecond

// jsRuntime runs one module's scripts in an embedded goja VM.
//
// goja is not goroutine-safe, so all script execution happens on a
// dedicated goroutine that pumps a task channel; timers (yzf.after /
// yzf.every) schedule tasks onto that channel.
type jsRuntime struct {
	def  *Definition
	api  *Bridge
	verb func(format string, args ...any)

	vm    *goja.Runtime
	tasks chan func()
	stop  chan struct{}
	done  chan struct{}

	mu         sync.Mutex
	enableFns  []goja.Callable
	disableFns []goja.Callable
	timers     []*time.Timer
	tickers    []*time.Ticker
	exports    map[string]goja.Callable
	started    bool

	// sourceCache holds the script source text captured at the last successful
	// load. It is used by transactional reload rollback to restore the
	// previous working state without re-reading (possibly broken) files.
	sourceCache map[string]string

	// restoreCache, when non-nil, supplies script source text to use instead
	// of reading from disk. Rollback sets this to a snapshot's sources so the
	// previous working version is re-executed even though disk now holds a
	// broken revision.
	restoreCache map[string]string
}

// NewJSRuntime creates the embedded JS runtime for a module.
func NewJSRuntime(def *Definition, api *Bridge, verb func(format string, args ...any)) Runtime {
	if verb == nil {
		verb = func(string, ...any) {}
	}
	return &jsRuntime{
		def:     def,
		api:     api,
		verb:    verb,
		tasks:   make(chan func(), 64),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
		exports: make(map[string]goja.Callable),
	}
}

// Kind reports the runtime identifier used in module.hjson.
func (r *jsRuntime) Kind() string { return "js" }

// Start boots the VM, injects the bridge, and executes every script
// (main first), then fires registered onEnable callbacks.
func (r *jsRuntime) Start(ctx context.Context) error {
	r.vm = goja.New()
	r.vm.SetFieldNameMapper(goja.TagFieldNameMapper("json", true))
	if err := r.installGlobals(); err != nil {
		return fmt.Errorf("注入 yzf 桥接失败: %w", err)
	}

	go r.pump()

	if len(r.def.Scripts) == 0 {
		return fmt.Errorf("模块 %s 没有可执行的脚本", r.def.FullID())
	}
	for _, script := range r.def.Scripts {
		if err := r.runFile(script); err != nil {
			return fmt.Errorf("执行脚本 %s 失败: %w", filepath.Base(script), err)
		}
	}

	r.mu.Lock()
	r.started = true
	enableFns := append([]goja.Callable(nil), r.enableFns...)
	r.mu.Unlock()
	for _, fn := range enableFns {
		r.invoke(fn)
	}
	return nil
}

func (r *jsRuntime) pump() {
	defer close(r.done)
	for {
		select {
		case fn := <-r.tasks:
			func() {
				defer func() {
					if rec := recover(); rec != nil {
						r.verb("JS 回调异常 (%s): %v", r.def.FullID(), rec)
					}
				}()
				fn()
			}()
		case <-r.stop:
			return
		}
	}
}

// Stop halts timers and the JS goroutine.
func (r *jsRuntime) Stop(ctx context.Context) error {
	r.mu.Lock()
	for _, timer := range r.timers {
		timer.Stop()
	}
	for _, ticker := range r.tickers {
		ticker.Stop()
	}
	r.timers = nil
	r.tickers = nil
	r.mu.Unlock()

	r.OnDisable(ctx)
	close(r.stop)
	select {
	case <-r.done:
	case <-time.After(time.Second):
	}
	return nil
}

// OnEnable fires registered onEnable callbacks.
func (r *jsRuntime) OnEnable(ctx context.Context) {
	r.mu.Lock()
	fns := append([]goja.Callable(nil), r.enableFns...)
	r.mu.Unlock()
	for _, fn := range fns {
		r.invoke(fn)
	}
}

// OnDisable fires registered onDisable callbacks.
func (r *jsRuntime) OnDisable(ctx context.Context) {
	r.mu.Lock()
	fns := append([]goja.Callable(nil), r.disableFns...)
	r.mu.Unlock()
	for _, fn := range fns {
		r.invoke(fn)
	}
}

// FireEvent dispatches a server event to yzf.on handlers registered by
// this module. Handlers run synchronously (bounded by jsCallTimeout);
// returning false from a handler consumes the event.
func (r *jsRuntime) FireEvent(ctx context.Context, event string, payload EventPayload) bool {
	handlers := r.api.eventHandlers(r.def.FullID(), event)
	for _, handler := range handlers {
		arg := r.toJSONValue(payload)
		result, err := r.runOnJS(func() (goja.Value, error) {
			return handler(goja.Undefined(), arg)
		})
		if err != nil {
			r.verb("事件 %s 处理异常 (%s): %v", event, r.def.FullID(), err)
			continue
		}
		if result != nil {
			if boolVal, ok := result.Export().(bool); ok && !boolVal {
				return false
			}
		}
	}
	return true
}

// exportedFunction returns a module-exported function for cross-module
// calls (yzf.module.call).
func (r *jsRuntime) exportedFunction(name string) (goja.Callable, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	fn, ok := r.exports[name]
	return fn, ok
}

// callExported invokes an exported function with JSON-friendly args.
func (r *jsRuntime) callExported(name string, args []any) (any, error) {
	fn, ok := r.exportedFunction(name)
	if !ok {
		return nil, fmt.Errorf("模块 %s 未导出函数 %s", r.def.FullID(), name)
	}
	values := make([]goja.Value, 0, len(args)+1)
	values = append(values, goja.Undefined())
	for _, arg := range args {
		values = append(values, r.toJSONValue(arg))
	}
	result, err := r.runOnJS(func() (goja.Value, error) {
		return fn(values[0], values[1:]...)
	})
	if err != nil {
		return nil, err
	}
	if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
		return nil, nil
	}
	return result.Export(), nil
}

func (r *jsRuntime) runFile(path string) error {
	var source string
	if cached, ok := r.lookupRestoreSource(path); ok {
		source = cached
	} else {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		source = string(data)
	}
	rel, relErr := filepath.Rel(r.def.Root, path)
	if relErr != nil {
		rel = filepath.Base(path)
	}
	_, err := r.vm.RunScript(filepath.ToSlash(rel), source)
	if err != nil {
		return err
	}
	r.cacheSource(path, source)
	return nil
}

// lookupRestoreSource returns cached rollback source for a path when present.
func (r *jsRuntime) lookupRestoreSource(path string) (string, bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.restoreCache == nil {
		return "", false
	}
	src, ok := r.restoreCache[path]
	return src, ok
}

// setRestoreCache installs a source snapshot used for rollback so Start
// re-executes the previous working version instead of the on-disk files.
func (r *jsRuntime) setRestoreCache(sources map[string]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreCache = sources
}

// cacheSource records the source text of a successfully loaded script so a
// transactional reload rollback can restore it without re-reading disk.
func (r *jsRuntime) cacheSource(path, source string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.sourceCache == nil {
		r.sourceCache = make(map[string]string)
	}
	r.sourceCache[path] = source
}

// cachedSources returns a copy of the source cache. Used by rollback.
func (r *jsRuntime) cachedSources() map[string]string {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make(map[string]string, len(r.sourceCache))
	for k, v := range r.sourceCache {
		out[k] = v
	}
	return out
}

// runCachedSource re-executes a previously loaded script from its cached
// source text, used during rollback when the on-disk file may be broken.
func (r *jsRuntime) runCachedSource(path, source string) error {
	rel, relErr := filepath.Rel(r.def.Root, path)
	if relErr != nil {
		rel = filepath.Base(path)
	}
	_, err := r.vm.RunScript(filepath.ToSlash(rel), source)
	return err
}

func (r *jsRuntime) invoke(fn goja.Callable) {
	_, err := r.runOnJS(func() (goja.Value, error) {
		return fn(goja.Undefined())
	})
	if err != nil {
		r.verb("JS 回调失败 (%s): %v", r.def.FullID(), err)
	}
}

// runOnJS posts a task to the JS goroutine and waits for the result.
func (r *jsRuntime) runOnJS(fn func() (goja.Value, error)) (goja.Value, error) {
	type callResult struct {
		value goja.Value
		err   error
	}
	out := make(chan callResult, 1)
	select {
	case r.tasks <- func() {
		value, err := fn()
		out <- callResult{value, err}
	}:
	case <-r.stop:
		return nil, fmt.Errorf("JS 运行时已停止")
	}
	select {
	case res := <-out:
		return res.value, res.err
	case <-time.After(jsCallTimeout):
		return nil, fmt.Errorf("JS 调用超时")
	}
}

func (r *jsRuntime) toJSONValue(v any) goja.Value {
	if v == nil {
		return goja.Null()
	}
	return r.vm.ToValue(v)
}

// installGlobals injects yzfModule and the yzf API object.
func (r *jsRuntime) installGlobals() error {
	moduleObj := map[string]any{
		"id":         r.def.Meta.ID,
		"fullId":     r.def.FullID(),
		"name":       r.def.Meta.Name,
		"author":     r.def.Meta.Author,
		"version":    r.def.Meta.Version,
		"root":       r.def.Root,
		"scriptsDir": r.def.ScriptsDir,
		"dataDir":    r.def.DataDir,
		"cacheDir":   r.def.CacheDir,
	}
	if err := r.vm.Set("yzfModule", moduleObj); err != nil {
		return err
	}
	return r.vm.Set("yzf", r.buildAPI())
}

// buildAPI assembles the yzf shim object mirroring the YF JS bridge.
func (r *jsRuntime) buildAPI() map[string]any {
	fullID := r.def.FullID()
	notSupported := func(feature string) func(goja.FunctionCall) goja.Value {
		return func(goja.FunctionCall) goja.Value {
			panic(r.vm.NewGoError(fmt.Errorf("yzf.%s 在当前 Go 运行时不可用", feature)))
		}
	}

	api := map[string]any{
		// --- lifecycle / events ---
		"on": func(call goja.FunctionCall) goja.Value {
			event := call.Argument(0).String()
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				r.api.subscribeEvent(fullID, event, fn)
			}
			return goja.Undefined()
		},
		"after": func(call goja.FunctionCall) goja.Value {
			delay := time.Duration(call.Argument(0).ToFloat() * float64(time.Second))
			if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
				r.scheduleAfter(delay, fn)
			}
			return goja.Undefined()
		},
		"every": func(call goja.FunctionCall) goja.Value {
			delay := time.Duration(call.Argument(0).ToFloat() * float64(time.Second))
			interval := time.Duration(call.Argument(1).ToFloat() * float64(time.Second))
			if fn, ok := goja.AssertFunction(call.Argument(2)); ok {
				r.scheduleEvery(delay, interval, fn)
			}
			return goja.Undefined()
		},
		"onEnable": func(call goja.FunctionCall) goja.Value {
			if fn, ok := goja.AssertFunction(call.Argument(0)); ok {
				r.mu.Lock()
				if r.started {
					r.mu.Unlock()
					r.invoke(fn)
				} else {
					r.enableFns = append(r.enableFns, fn)
					r.mu.Unlock()
				}
			}
			return goja.Undefined()
		},
		"onDisable": func(call goja.FunctionCall) goja.Value {
			if fn, ok := goja.AssertFunction(call.Argument(0)); ok {
				r.mu.Lock()
				r.disableFns = append(r.disableFns, fn)
				r.mu.Unlock()
			}
			return goja.Undefined()
		},

		// --- logging ---
		"log":  r.logFn("INFO"),
		"info": r.logFn("INFO"),
		"warn": r.logFn("WARN"),
		"err":  r.logFn("ERROR"),

		// --- config ---
		"config": map[string]any{
			"get": func(call goja.FunctionCall) goja.Value {
				return r.vm.ToValue(r.api.config(fullID).GetString(call.Argument(0).String(), call.Argument(1).String()))
			},
			"getBool": func(call goja.FunctionCall) goja.Value {
				return r.vm.ToValue(r.api.config(fullID).GetBool(call.Argument(0).String(), call.Argument(1).ToBoolean()))
			},
			"getInt": func(call goja.FunctionCall) goja.Value {
				return r.vm.ToValue(r.api.config(fullID).GetInt(call.Argument(0).String(), int(call.Argument(1).ToInteger())))
			},
			"set": func(call goja.FunctionCall) goja.Value {
				_ = r.api.config(fullID).SetString(call.Argument(0).String(), call.Argument(1).String())
				return goja.Undefined()
			},
			"setBool": func(call goja.FunctionCall) goja.Value {
				_ = r.api.config(fullID).SetBool(call.Argument(0).String(), call.Argument(1).ToBoolean())
				return goja.Undefined()
			},
			"setInt": func(call goja.FunctionCall) goja.Value {
				_ = r.api.config(fullID).SetInt(call.Argument(0).String(), int(call.Argument(1).ToInteger()))
				return goja.Undefined()
			},
			"path": func(goja.FunctionCall) goja.Value {
				return r.vm.ToValue(r.api.config(fullID).Path())
			},
		},

		// --- callable commands / registration ---
		"command": func(call goja.FunctionCall) goja.Value {
			name := call.Argument(0).String()
			args := make([]any, 0, len(call.Arguments)-1)
			for _, arg := range call.Arguments[1:] {
				args = append(args, arg.Export())
			}
			found, result := r.api.commands.CallCallable(name, args)
			if !found {
				panic(r.vm.NewGoError(fmt.Errorf("未找到可调用命令 %s", name)))
			}
			return r.toJSONValue(result)
		},
		"playerCommand": func(call goja.FunctionCall) goja.Value {
			name := call.Argument(0).String()
			usage := call.Argument(1).String()
			desc := call.Argument(2).String()
			if fn, ok := goja.AssertFunction(call.Argument(3)); ok {
				r.api.commands.RegisterPlayerCommand(fullID, name, usage, desc, false, "", r.playerHandler(fn))
			}
			return goja.Undefined()
		},
		"adminCommand": func(call goja.FunctionCall) goja.Value {
			name := call.Argument(0).String()
			usage := call.Argument(1).String()
			desc := call.Argument(2).String()
			permission := call.Argument(3).String()
			if fn, ok := goja.AssertFunction(call.Argument(4)); ok {
				r.api.commands.RegisterPlayerCommand(fullID, name, usage, desc, true, permission, r.playerHandler(fn))
			}
			return goja.Undefined()
		},
		"commands": map[string]any{
			"register": func(call goja.FunctionCall) goja.Value {
				name := call.Argument(0).String()
				desc := call.Argument(1).String()
				if fn, ok := goja.AssertFunction(call.Argument(2)); ok {
					r.api.commands.RegisterCallable(fullID, name, desc, r.callableHandler(fn))
				}
				return goja.Undefined()
			},
			"unregister": func(call goja.FunctionCall) goja.Value {
				r.api.unregisterCallable(fullID, call.Argument(0).String())
				return goja.Undefined()
			},
			"has": func(call goja.FunctionCall) goja.Value {
				found, _ := r.api.commands.CallCallable(call.Argument(0).String(), nil)
				return r.vm.ToValue(found)
			},
			"call": func(call goja.FunctionCall) goja.Value {
				name := call.Argument(0).String()
				args := exportRest(call.Arguments[1:])
				found, result := r.api.commands.CallCallable(name, args)
				if !found {
					return goja.Null()
				}
				return r.toJSONValue(result)
			},
			"run": func(call goja.FunctionCall) goja.Value {
				name := call.Argument(0).String()
				var args []string
				for _, arg := range call.Arguments[1:] {
					if !goja.IsUndefined(arg) && !goja.IsNull(arg) {
						args = append(args, arg.String())
					}
				}
				return r.vm.ToValue(r.api.commands.RunServerCommand(name, args))
			},
			"list": func(goja.FunctionCall) goja.Value {
				return r.toJSONValue(r.api.commands.ListCallableCommands())
			},
			"listModule": func(call goja.FunctionCall) goja.Value {
				return r.toJSONValue(r.api.commands.ListModuleCommands(call.Argument(0).String()))
			},
		},
		"mod": map[string]any{
			"registerServerCommand": func(call goja.FunctionCall) goja.Value {
				name := call.Argument(0).String()
				usage := call.Argument(1).String()
				desc := call.Argument(2).String()
				if fn, ok := goja.AssertFunction(call.Argument(3)); ok {
					r.api.commands.RegisterServerCommand(fullID, name, usage, desc, r.serverHandler(fn))
				}
				return goja.Undefined()
			},
			"registerPlayerCommand": func(call goja.FunctionCall) goja.Value {
				name := call.Argument(0).String()
				usage := call.Argument(1).String()
				desc := call.Argument(2).String()
				if fn, ok := goja.AssertFunction(call.Argument(3)); ok {
					r.api.commands.RegisterPlayerCommand(fullID, name, usage, desc, false, "", r.playerHandler(fn))
				}
				return goja.Undefined()
			},
			"registerAdminCommand": func(call goja.FunctionCall) goja.Value {
				name := call.Argument(0).String()
				usage := call.Argument(1).String()
				desc := call.Argument(2).String()
				permission := call.Argument(3).String()
				if fn, ok := goja.AssertFunction(call.Argument(4)); ok {
					r.api.commands.RegisterPlayerCommand(fullID, name, usage, desc, true, permission, r.playerHandler(fn))
				}
				return goja.Undefined()
			},
			"registerCallableCommand": func(call goja.FunctionCall) goja.Value {
				name := call.Argument(0).String()
				desc := call.Argument(1).String()
				if fn, ok := goja.AssertFunction(call.Argument(2)); ok {
					r.api.commands.RegisterCallable(fullID, name, desc, r.callableHandler(fn))
				}
				return goja.Undefined()
			},
			"unregisterCommand": func(call goja.FunctionCall) goja.Value {
				r.api.unregisterCallable(fullID, call.Argument(0).String())
				return goja.Undefined()
			},
		},

		// --- module info / cross-module calls ---
		"module": map[string]any{
			"list": func(goja.FunctionCall) goja.Value {
				return r.toJSONValue(r.api.moduleList())
			},
			"info": func(call goja.FunctionCall) goja.Value {
				return r.toJSONValue(r.api.moduleInfo(call.Argument(0).String()))
			},
			"export": func(call goja.FunctionCall) goja.Value {
				name := call.Argument(0).String()
				if fn, ok := goja.AssertFunction(call.Argument(1)); ok {
					r.mu.Lock()
					r.exports[name] = fn
					r.mu.Unlock()
				}
				return goja.Undefined()
			},
			"call": func(call goja.FunctionCall) goja.Value {
				moduleID := call.Argument(0).String()
				fnName := call.Argument(1).String()
				args := exportRest(call.Arguments[2:])
				result, err := r.api.callModule(moduleID, fnName, args)
				if err != nil {
					panic(r.vm.NewGoError(err))
				}
				return r.toJSONValue(result)
			},
			"exported": func(call goja.FunctionCall) goja.Value {
				return r.toJSONValue(r.api.moduleExports(call.Argument(0).String()))
			},
		},
		"runtime": map[string]any{
			"mode": func(goja.FunctionCall) goja.Value {
				return r.vm.ToValue("go-embedded")
			},
			"modules": func(goja.FunctionCall) goja.Value {
				return r.toJSONValue(r.api.moduleList())
			},
			"reloadSelf": func(goja.FunctionCall) goja.Value {
				return r.vm.ToValue(r.api.requestReload(fullID))
			},
			"reloadModule": func(call goja.FunctionCall) goja.Value {
				return r.vm.ToValue(r.api.requestReload(call.Argument(0).String()))
			},
			"reloadAll": func(goja.FunctionCall) goja.Value {
				return r.vm.ToValue(r.api.requestReloadAll())
			},
		},

		// --- player / net / game ---
		"player": map[string]any{
			"list": func(goja.FunctionCall) goja.Value {
				return r.toJSONValue(r.api.playerList())
			},
			"count": func(goja.FunctionCall) goja.Value {
				return r.vm.ToValue(r.api.playerCount())
			},
			"find": func(call goja.FunctionCall) goja.Value {
				return r.toJSONValue(r.api.playerFind(call.Argument(0).String()))
			},
			"info": func(call goja.FunctionCall) goja.Value {
				return r.toJSONValue(r.api.playerFindByID(int32(call.Argument(0).ToInteger())))
			},
			"send": func(call goja.FunctionCall) goja.Value {
				r.api.playerSend(int32(call.Argument(0).ToInteger()), call.Argument(1).String())
				return goja.Undefined()
			},
			"kick": func(call goja.FunctionCall) goja.Value {
				r.api.playerKick(int32(call.Argument(0).ToInteger()), call.Argument(1).String())
				return goja.Undefined()
			},
			"ban": func(call goja.FunctionCall) goja.Value {
				r.api.playerBan(int32(call.Argument(0).ToInteger()))
				return goja.Undefined()
			},
		},
		"net": map[string]any{
			"send": func(call goja.FunctionCall) goja.Value {
				r.api.playerSend(int32(call.Argument(0).ToInteger()), call.Argument(1).String())
				return goja.Undefined()
			},
			"broadcast": func(call goja.FunctionCall) goja.Value {
				msg := call.Argument(0).String()
				if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
					r.api.broadcastFrom(msg, int32(call.Argument(1).ToInteger()))
				} else {
					r.api.broadcast(msg)
				}
				return goja.Undefined()
			},
		},
		"game": map[string]any{
			"tick":      func(goja.FunctionCall) goja.Value { return r.vm.ToValue(r.api.gameTick()) },
			"tps":       func(goja.FunctionCall) goja.Value { return r.vm.ToValue(r.api.gameTPS()) },
			"wave":      func(goja.FunctionCall) goja.Value { return r.vm.ToValue(r.api.gameWave()) },
			"isPlaying": func(goja.FunctionCall) goja.Value { return r.vm.ToValue(r.api.gameIsPlaying()) },
			"isPaused":  func(goja.FunctionCall) goja.Value { return r.vm.ToValue(r.api.gameIsPaused()) },
			"map": func(goja.FunctionCall) goja.Value {
				return r.toJSONValue(map[string]any{"name": r.api.gameMapName()})
			},
		},

		// --- response helpers (pure JS, same shape as YF) ---
		"response": map[string]any{
			"ok": func(call goja.FunctionCall) goja.Value {
				return r.responseValue(true, call)
			},
			"fail": func(call goja.FunctionCall) goja.Value {
				return r.responseValue(false, call)
			},
		},

		// --- script loading ---
		"evalFile": func(call goja.FunctionCall) goja.Value {
			path := call.Argument(0).String()
			if !filepath.IsAbs(path) {
				path = filepath.Join(r.def.Root, filepath.FromSlash(path))
			}
			if err := r.runFile(path); err != nil {
				panic(r.vm.NewGoError(err))
			}
			return goja.Undefined()
		},

		// --- Java-framework-only capabilities: names stay present so
		// scripts can feature-detect, calls fail with a clear error ---
		"remote":    map[string]any{"get": notSupported("remote.get"), "postJson": notSupported("remote.postJson")},
		"service":   map[string]any{"has": notSupported("service"), "summary": notSupported("service"), "list": notSupported("service"), "info": notSupported("service"), "call": notSupported("service")},
		"memory":    map[string]any{"jvm": notSupported("memory"), "list": notSupported("memory"), "info": notSupported("memory"), "create": notSupported("memory"), "load": notSupported("memory"), "stop": notSupported("memory")},
		"openapi":   map[string]any{"manifest": notSupported("openapi"), "list": notSupported("openapi"), "info": notSupported("openapi"), "summary": notSupported("openapi"), "readOnly": notSupported("openapi"), "writeOnly": notSupported("openapi")},
		"status":    map[string]any{"snapshot": notSupported("status.snapshot"), "ui": notSupported("status.ui")},
		"redis":     map[string]any{"get": notSupported("redis"), "set": notSupported("redis"), "del": notSupported("redis"), "incr": notSupported("redis"), "hget": notSupported("redis"), "hset": notSupported("redis")},
		"sql":       map[string]any{"queryFirstCell": notSupported("sql"), "execute": notSupported("sql"), "queryJson": notSupported("sql")},
		"minio":     map[string]any{"putText": notSupported("minio")},
		"ws":        map[string]any{"connect": notSupported("ws"), "send": notSupported("ws"), "sendBinary": notSupported("ws"), "close": notSupported("ws"), "isOpen": notSupported("ws"), "list": notSupported("ws")},
		"comid":     map[string]any{"get": notSupported("comid"), "getOrCreate": notSupported("comid"), "uuid": notSupported("comid"), "exists": notSupported("comid"), "digits": notSupported("comid"), "remaining": notSupported("comid"), "total": notSupported("comid")},
		"data":      map[string]any{"get": notSupported("data"), "set": notSupported("data"), "getInt": notSupported("data"), "setInt": notSupported("data"), "getBool": notSupported("data"), "setBool": notSupported("data"), "getDouble": notSupported("data"), "setDouble": notSupported("data"), "all": notSupported("data"), "remove": notSupported("data"), "clear": notSupported("data")},
		"db":        map[string]any{"list": notSupported("db"), "info": notSupported("db"), "has": notSupported("db"), "addLocal": notSupported("db"), "addRemote": notSupported("db"), "remove": notSupported("db"), "categories": notSupported("db"), "keys": notSupported("db"), "get": notSupported("db"), "set": notSupported("db"), "removeEntry": notSupported("db"), "dump": notSupported("db"), "import": notSupported("db"), "defaultId": notSupported("db"), "count": notSupported("db")},
		"content":   map[string]any{"block": notSupported("content"), "item": notSupported("content"), "liquid": notSupported("content"), "unit": notSupported("content"), "status": notSupported("content"), "weather": notSupported("content"), "planet": notSupported("content"), "blocks": notSupported("content"), "items": notSupported("content"), "liquids": notSupported("content"), "units": notSupported("content"), "registerMeta": notSupported("content"), "getMeta": notSupported("content"), "listMeta": notSupported("content"), "listNamespaces": notSupported("content"), "removeMeta": notSupported("content"), "setProperty": notSupported("content"), "getProperty": notSupported("content")},
		"world":     map[string]any{"spawn": notSupported("world.spawn"), "batchSpawn": notSupported("world.batchSpawn"), "fill": notSupported("world.fill")},
		"ui":        map[string]any{"registerPage": notSupported("ui.registerPage"), "unregisterPage": notSupported("ui.unregisterPage")},
		"stableApi": notSupported("stableApi"),
	}
	return api
}

func (r *jsRuntime) logFn(level string) func(goja.FunctionCall) goja.Value {
	return func(call goja.FunctionCall) goja.Value {
		r.verb("[%s] [%s] %s", level, r.def.Meta.Name, call.Argument(0).String())
		return goja.Undefined()
	}
}

func (r *jsRuntime) responseValue(ok bool, call goja.FunctionCall) goja.Value {
	code := "ok"
	if !ok {
		code = "error"
	}
	if !goja.IsUndefined(call.Argument(0)) && call.Argument(0).String() != "" {
		code = call.Argument(0).String()
	}
	message := ""
	if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) {
		message = call.Argument(1).String()
	}
	var data any
	if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) {
		data = call.Argument(2).Export()
	}
	payload := map[string]any{
		"ok":          ok,
		"success":     ok,
		"code":        code,
		"message":     message,
		"data":        data,
		"timestampMs": time.Now().UnixMilli(),
	}
	return r.toJSONValue(payload)
}

// playerHandler adapts a JS function to PlayerCommandHandler.
// The JS side receives (player, argsArray).
func (r *jsRuntime) playerHandler(fn goja.Callable) PlayerCommandHandler {
	return func(player PlayerInfo, args []string) {
		playerValue := r.toJSONValue(map[string]any{
			"id": player.ID, "name": player.Name, "uuid": player.UUID,
			"ip": player.IP, "admin": player.Admin, "team": player.TeamID,
		})
		argsValue := r.toJSONValue(args)
		_, err := r.runOnJS(func() (goja.Value, error) {
			return fn(goja.Undefined(), playerValue, argsValue)
		})
		if err != nil {
			r.verb("玩家命令处理失败 (%s): %v", r.def.FullID(), err)
		}
	}
}

// serverHandler adapts a JS function to ServerCommandHandler.
func (r *jsRuntime) serverHandler(fn goja.Callable) ServerCommandHandler {
	return func(args []string) {
		argsValue := r.toJSONValue(args)
		_, err := r.runOnJS(func() (goja.Value, error) {
			return fn(goja.Undefined(), argsValue)
		})
		if err != nil {
			r.verb("服务器命令处理失败 (%s): %v", r.def.FullID(), err)
		}
	}
}

// callableHandler adapts a JS function to CallableCommandHandler.
func (r *jsRuntime) callableHandler(fn goja.Callable) CallableCommandHandler {
	return func(args []any) any {
		values := make([]goja.Value, 0, len(args)+1)
		values = append(values, goja.Undefined())
		for _, arg := range args {
			values = append(values, r.toJSONValue(arg))
		}
		result, err := r.runOnJS(func() (goja.Value, error) {
			return fn(values[0], values[1:]...)
		})
		if err != nil {
			r.verb("可调用命令执行失败 (%s): %v", r.def.FullID(), err)
			return map[string]any{"ok": false, "message": err.Error()}
		}
		if result == nil || goja.IsUndefined(result) || goja.IsNull(result) {
			return nil
		}
		return result.Export()
	}
}

func (r *jsRuntime) scheduleAfter(delay time.Duration, fn goja.Callable) {
	timer := time.AfterFunc(delay, func() {
		select {
		case r.tasks <- func() { _, _ = fn(goja.Undefined()) }:
		case <-r.stop:
		}
	})
	r.mu.Lock()
	r.timers = append(r.timers, timer)
	r.mu.Unlock()
}

func (r *jsRuntime) scheduleEvery(delay, interval time.Duration, fn goja.Callable) {
	if interval <= 0 {
		interval = time.Second
	}
	var ticker *time.Ticker
	timer := time.AfterFunc(delay, func() {
		ticker = time.NewTicker(interval)
		r.mu.Lock()
		r.tickers = append(r.tickers, ticker)
		r.mu.Unlock()
		go func() {
			for {
				select {
				case <-ticker.C:
					select {
					case r.tasks <- func() { _, _ = fn(goja.Undefined()) }:
					case <-r.stop:
						return
					}
				case <-r.stop:
					return
				}
			}
		}()
	})
	r.mu.Lock()
	r.timers = append(r.timers, timer)
	r.mu.Unlock()
}

func exportRest(values []goja.Value) []any {
	out := make([]any, 0, len(values))
	for _, value := range values {
		if goja.IsUndefined(value) || goja.IsNull(value) {
			continue
		}
		out = append(out, value.Export())
	}
	return out
}

// marshalJSON is a small helper for debug endpoints.
func marshalJSON(v any) string {
	payload, err := json.Marshal(v)
	if err != nil {
		return "{}"
	}
	return string(payload)
}
