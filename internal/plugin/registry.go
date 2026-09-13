package plugin

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// reloadDispatchDelay is the coalescing window for burst reload requests,
// mirroring YZF's scheduleReloadDispatch delay.
const reloadDispatchDelay = 50 * time.Millisecond

// Registry discovers, resolves, loads and supervises modules. It
// mirrors YZFModuleRegistry + MindustryYZF bootstrap responsibilities:
// scan config/yzf/plugins (flat) and config/yzf/modules (author/id),
// resolve dependencies, then start runtimes in topological order.
type Registry struct {
	mu       sync.Mutex
	plugins  string // config/yzf/plugins
	modules  string // config/yzf/modules
	bridge   *Bridge
	commands *CommandRegistry
	events   *EventBus
	verb     func(format string, args ...any)

	loaded map[string]*LoadedModule
	order  []string // load order (topological)

	// reloadCh queues reload requests; the watcher (phase 5) and the
	// yzf.runtime.reload* API push here. Single consumer serializes.
	reloadCh chan string
}

// NewRegistry creates a registry rooted at the given yzf directory
// (the directory containing plugins/ and modules/).
func NewRegistry(yzfRoot string, verb func(format string, args ...any)) *Registry {
	if verb == nil {
		verb = func(string, ...any) {}
	}
	commands := NewCommandRegistry()
	events := NewEventBus()
	registry := &Registry{
		plugins:  filepath.Join(yzfRoot, "plugins"),
		modules:  filepath.Join(yzfRoot, "modules"),
		commands: commands,
		events:   events,
		verb:     verb,
		loaded:   make(map[string]*LoadedModule),
		reloadCh: make(chan string, 16),
	}
	bridge := NewBridge(nil, nil, nil, nil, commands)
	bridge.WireCrossModule(
		registry.moduleSummaries,
		registry.moduleSummary,
		registry.moduleExportNames,
		registry.callModuleExport,
	)
	bridge.WireReload(registry.RequestReload, registry.RequestReloadAll)
	registry.bridge = bridge
	return registry
}

// BindCapabilities wires host server capabilities into the bridge.
func (r *Registry) BindCapabilities(server ServerAPI, players PlayerAPI, net NetAPI, game GameAPI) {
	r.bridge.server = server
	r.bridge.players = players
	r.bridge.net = net
	r.bridge.game = game
}

// Commands exposes the command registry to the host (chat dispatch).
func (r *Registry) Commands() *CommandRegistry { return r.commands }

// Events exposes the event bus to the host (event firing).
func (r *Registry) Events() *EventBus { return r.events }

// ScanAndLoad performs discovery + resolution + start. It is called
// once at server startup (and again after a full reload).
func (r *Registry) ScanAndLoad(ctx context.Context) error {
	discovered := r.scan()
	resolution := Resolve(discovered)
	for _, warning := range resolution.Warnings {
		r.verb("[插件] 警告: %s", warning)
	}
	if len(resolution.Errors) > 0 {
		for _, errMsg := range resolution.Errors {
			r.verb("[插件] 错误: %s", errMsg)
		}
		// Hard-dependency errors disable the dependent module; cycles
		// disable everything in the cycle. Continue loading the rest.
	}

	r.mu.Lock()
	r.order = nil
	r.mu.Unlock()

	skipped := r.brokenSet(resolution)
	for _, def := range resolution.Ordered {
		fullID := def.FullID()
		if skipped[fullID] {
			r.verb("[插件] 跳过 %s（依赖解析失败）", fullID)
			continue
		}
		if !def.Meta.Enabled {
			r.verb("[插件] %s 已禁用，跳过", fullID)
			continue
		}
		if err := r.startModule(ctx, def); err != nil {
			r.verb("[插件] 启动 %s 失败: %v", fullID, err)
		}
	}
	r.verb("[插件] 加载完成：共 %d 个模块定义，成功启动 %d 个", len(discovered), r.loadedCount())
	return nil
}

// brokenSet computes modules that must not start: those with missing
// hard dependencies and everyone inside a dependency cycle.
func (r *Registry) brokenSet(resolution Resolution) map[string]bool {
	skipped := make(map[string]bool)
	for _, errMsg := range resolution.Errors {
		// Error formats come from Resolve: missing hard deps name the
		// dependent module; cycle errors list the chain.
		if id := extractModuleFromMissingDep(errMsg); id != "" {
			skipped[id] = true
		}
		if ids := extractCycleIDs(errMsg); len(ids) > 0 {
			for _, id := range ids {
				skipped[id] = true
			}
		}
	}
	return skipped
}

func extractModuleFromMissingDep(msg string) string {
	const prefix = "模块 "
	const marker = " 缺少硬依赖 "
	if len(msg) > len(prefix) && msg[:len(prefix)] == prefix {
		if idx := indexOf(msg, marker); idx > 0 {
			return msg[len(prefix):idx]
		}
	}
	return ""
}

func extractCycleIDs(msg string) []string {
	const prefix = "检测到循环依赖: "
	if len(msg) <= len(prefix) || msg[:len(prefix)] != prefix {
		return nil
	}
	return splitString(msg[len(prefix):], " -> ")
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func splitString(s, sep string) []string {
	var out []string
	for {
		idx := indexOf(s, sep)
		if idx < 0 {
			out = append(out, s)
			return out
		}
		out = append(out, s[:idx])
		s = s[idx+len(sep):]
	}
}

// scan discovers module definitions from both directory layouts.
func (r *Registry) scan() []*Definition {
	var discovered []*Definition

	// plugins/<name>/module.hjson (flat layout, as shipped by YF)
	if entries, err := os.ReadDir(r.plugins); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			def, err := LoadModule(filepath.Join(r.plugins, entry.Name()))
			if err != nil {
				r.verb("[插件] 读取 %s 失败: %v", entry.Name(), err)
				continue
			}
			if def != nil {
				def.Meta.Source = "plugins"
				if def.Meta.LoadType == "" || def.Meta.LoadType == "module" {
					def.Meta.LoadType = "plugin"
				}
				discovered = append(discovered, def)
			}
		}
	}

	// modules/<author>/<id>/module.hjson (namespaced layout)
	if authors, err := os.ReadDir(r.modules); err == nil {
		for _, author := range authors {
			if !author.IsDir() {
				continue
			}
			authorDir := filepath.Join(r.modules, author.Name())
			entries, err := os.ReadDir(authorDir)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					continue
				}
				def, err := LoadModule(filepath.Join(authorDir, entry.Name()))
				if err != nil {
					r.verb("[插件] 读取 %s/%s 失败: %v", author.Name(), entry.Name(), err)
					continue
				}
				if def != nil {
					def.Meta.Source = "modules"
					discovered = append(discovered, def)
				}
			}
		}
	}

	sort.SliceStable(discovered, func(i, j int) bool {
		return discovered[i].FullID() < discovered[j].FullID()
	})
	return discovered
}

// startModule instantiates the runtime and starts it.
func (r *Registry) startModule(ctx context.Context, def *Definition) error {
	fullID := def.FullID()
	r.mu.Lock()
	if _, exists := r.loaded[fullID]; exists {
		r.mu.Unlock()
		return fmt.Errorf("模块 %s 已在运行", fullID)
	}
	r.mu.Unlock()

	store := r.bridge.registerConfig(def)
	runtime, err := r.makeRuntime(def)
	if err != nil {
		return err
	}
	if err := runtime.Start(ctx); err != nil {
		r.bridge.dropConfig(fullID)
		return err
	}

	r.mu.Lock()
	r.loaded[fullID] = &LoadedModule{Def: def, Runtime: runtime, Config: store, Enabled: true}
	r.order = append(r.order, fullID)
	r.mu.Unlock()
	r.verb("[插件] 已启动 %s (%s, runtime=%s)", def.Meta.Name, fullID, def.Meta.Runtime)
	return nil
}

// makeRuntime selects the runtime implementation by meta.runtime.
func (r *Registry) makeRuntime(def *Definition) (Runtime, error) {
	switch def.Meta.Runtime {
	case "js":
		return NewJSRuntime(def, r.bridge, r.verb), nil
	case "go":
		return NewGoRuntime(def, r.bridge, r.verb), nil
	case "node", "java", "kt", "kts":
		return nil, fmt.Errorf("运行时 %q 尚未在 Go 服务端实现（模块 %s）", def.Meta.Runtime, def.FullID())
	default:
		return nil, fmt.Errorf("未知运行时 %q（模块 %s）", def.Meta.Runtime, def.FullID())
	}
}

// stopModule stops one module and detaches its registrations.
func (r *Registry) stopModule(ctx context.Context, fullID string) error {
	r.mu.Lock()
	module, ok := r.loaded[fullID]
	if !ok {
		r.mu.Unlock()
		return fmt.Errorf("模块 %s 未加载", fullID)
	}
	delete(r.loaded, fullID)
	r.removeFromOrderLocked(fullID)
	r.mu.Unlock()

	if err := module.Runtime.Stop(ctx); err != nil {
		r.verb("[插件] 停止 %s 出错: %v", fullID, err)
	}
	r.commands.UnregisterModule(fullID)
	r.events.UnsubscribeModule(fullID)
	r.bridge.dropEvents(fullID)
	r.bridge.dropConfig(fullID)
	r.verb("[插件] 已停止 %s", fullID)
	return nil
}

func (r *Registry) removeFromOrderLocked(fullID string) {
	kept := r.order[:0]
	for _, id := range r.order {
		if id != fullID {
			kept = append(kept, id)
		}
	}
	r.order = kept
}

// StopAll stops every module in reverse load order.
func (r *Registry) StopAll(ctx context.Context) {
	r.mu.Lock()
	order := append([]string(nil), r.order...)
	r.mu.Unlock()
	for i := len(order) - 1; i >= 0; i-- {
		if err := r.stopModule(ctx, order[i]); err != nil {
			r.verb("[插件] %v", err)
		}
	}
}

// FireEvent delivers a server event to every loaded runtime and then
// to bus subscribers. Returns false when the event was consumed.
func (r *Registry) FireEvent(ctx context.Context, event string, payload EventPayload) bool {
	r.mu.Lock()
	order := append([]string(nil), r.order...)
	modules := make([]*LoadedModule, 0, len(order))
	for _, id := range order {
		if module, ok := r.loaded[id]; ok {
			modules = append(modules, module)
		}
	}
	r.mu.Unlock()

	for _, module := range modules {
		if !module.Enabled {
			continue
		}
		if !module.Runtime.FireEvent(ctx, event, payload) {
			return false
		}
	}
	return r.events.Fire(event, payload)
}

// RequestReload queues a single-module reload.
func (r *Registry) RequestReload(fullID string) bool {
	select {
	case r.reloadCh <- fullID:
		return true
	default:
		return false
	}
}

// RequestReloadAll queues a full reload.
func (r *Registry) RequestReloadAll() bool {
	select {
	case r.reloadCh <- "*":
		return true
	default:
		return false
	}
}

// ReloadLoop serializes reload requests (started by the plugin module's
// Start). The file watcher and the yzf.runtime.reload* API push here.
//
// Requests arriving within the debounce window are coalesced into one round,
// mirroring YZF's scheduleReloadDispatch behaviour so a burst of editor saves
// lands as a single reload.
func (r *Registry) ReloadLoop(ctx context.Context) {
	for {
		var batch []string
		select {
		case target := <-r.reloadCh:
			batch = append(batch, target)
		case <-ctx.Done():
			return
		}
		timer := time.NewTimer(reloadDispatchDelay)
	collect:
		for {
			select {
			case target := <-r.reloadCh:
				batch = append(batch, target)
			case <-timer.C:
				break collect
			case <-ctx.Done():
				timer.Stop()
				return
			}
		}
		for _, target := range coalesceReloadTargets(batch) {
			r.performReload(ctx, target)
		}
	}
}

// coalesceReloadTargets de-duplicates module reload requests. When a full
// reload ("*") is present it subsumes every module-level request.
func coalesceReloadTargets(batch []string) []string {
	seen := make(map[string]bool)
	var out []string
	hasAll := false
	for _, target := range batch {
		if target == "*" {
			hasAll = true
			continue
		}
		if !seen[target] {
			seen[target] = true
			out = append(out, target)
		}
	}
	if hasAll {
		return []string{"*"}
	}
	return out
}

// reloadSnapshotEntry captures one module's previous working state so a
// failed transactional reload can restore it. jsSources holds the script
// source text for JS modules so the old version can be re-executed even
// though disk now carries a broken revision.
type reloadSnapshotEntry struct {
	def       *Definition
	jsSources map[string]string
}

// performReload executes one reload transaction. Module reloads are
// transactional: the target and everything depending on it is stopped, then
// restarted in dependency order; if any start fails, the previously running
// versions are restored from the snapshot.
func (r *Registry) performReload(ctx context.Context, target string) {
	if target == "*" {
		r.reloadAll(ctx)
		return
	}
	plan := r.resolveReloadPlan(target)
	if len(plan) == 0 {
		// The module vanished from disk: if it was running, stop it.
		if err := r.stopModule(ctx, target); err == nil {
			r.verb("[插件] 模块 %s 已从磁盘移除，停止旧实例", target)
		} else {
			r.verb("[插件] 重载失败：找不到模块 %s", target)
		}
		return
	}

	snapshot := r.takeSnapshot(plan)
	for i := len(plan) - 1; i >= 0; i-- {
		if err := r.stopModule(ctx, plan[i].FullID()); err != nil {
			r.verb("[插件] %v", err)
		}
	}

	applied := make([]*Definition, 0, len(plan))
	for _, def := range plan {
		if err := r.startModule(ctx, def); err != nil {
			r.verb("[插件] 事务重载失败 %s: %v，恢复先前模块状态", def.FullID(), err)
			for i := len(applied) - 1; i >= 0; i-- {
				_ = r.stopModule(ctx, applied[i].FullID())
			}
			r.restoreSnapshot(ctx, snapshot)
			return
		}
		applied = append(applied, def)
	}
	r.verb("[插件] 模块 %s 及其依赖者重载完成（%d 个模块）", target, len(applied))
}

// reloadAll performs a full transactional reload: snapshot everything loaded,
// rescan and load, then restore any previously running module that failed to
// reload.
func (r *Registry) reloadAll(ctx context.Context) {
	r.verb("[插件] 执行全量重载")
	r.mu.Lock()
	defs := make([]*Definition, 0, len(r.loaded))
	for _, id := range r.order {
		if module, ok := r.loaded[id]; ok {
			defs = append(defs, module.Def)
		}
	}
	r.mu.Unlock()
	snapshot := r.takeSnapshot(defs)

	r.StopAll(ctx)
	if err := r.ScanAndLoad(ctx); err != nil {
		r.verb("[插件] 全量重载失败: %v", err)
	}

	for _, entry := range snapshot {
		fullID := entry.def.FullID()
		r.mu.Lock()
		_, running := r.loaded[fullID]
		r.mu.Unlock()
		if running {
			continue
		}
		r.verb("[插件] 全量重载未能启动 %s，恢复先前版本", fullID)
		r.restoreEntry(ctx, entry)
	}
}

// resolveReloadPlan computes the reload scope for a module: the module
// itself plus every (transitive) dependent, ordered so dependencies restart
// before dependents. Mirrors YZFModuleRegistry.resolveReloadPlan.
func (r *Registry) resolveReloadPlan(target string) []*Definition {
	discovered := r.scan()
	byID := make(map[string]*Definition, len(discovered))
	for _, def := range discovered {
		byID[def.FullID()] = def
	}
	if byID[target] == nil {
		return nil
	}

	affected := make(map[string]bool)
	var collect func(id string)
	collect = func(id string) {
		if affected[id] {
			return
		}
		affected[id] = true
		for _, def := range discovered {
			if def.FullID() == id {
				continue
			}
			if containsString(def.Meta.Depends, id) || containsString(def.Meta.SoftDepends, id) {
				collect(def.FullID())
			}
		}
	}
	collect(target)

	resolution := Resolve(discovered)
	plan := make([]*Definition, 0, len(affected))
	for _, def := range resolution.Ordered {
		if affected[def.FullID()] {
			plan = append(plan, def)
		}
	}
	return plan
}

// takeSnapshot records the previously loaded state (definition + JS source
// text) for every plan module that is currently running.
func (r *Registry) takeSnapshot(plan []*Definition) []reloadSnapshotEntry {
	r.mu.Lock()
	defer r.mu.Unlock()
	entries := make([]reloadSnapshotEntry, 0, len(plan))
	for _, def := range plan {
		module, ok := r.loaded[def.FullID()]
		if !ok {
			continue
		}
		entry := reloadSnapshotEntry{def: module.Def}
		if js, ok := module.Runtime.(*jsRuntime); ok {
			entry.jsSources = js.cachedSources()
		}
		entries = append(entries, entry)
	}
	return entries
}

// restoreSnapshot restarts every snapshotted module from its previous
// definition (and cached JS source), rolling the registry back to the state
// before the failed reload.
func (r *Registry) restoreSnapshot(ctx context.Context, entries []reloadSnapshotEntry) {
	for _, entry := range entries {
		r.restoreEntry(ctx, entry)
	}
}

// restoreEntry restarts one module from its snapshot entry.
func (r *Registry) restoreEntry(ctx context.Context, entry reloadSnapshotEntry) {
	def := entry.def
	fullID := def.FullID()
	store := r.bridge.registerConfig(def)
	runtime, err := r.makeRuntime(def)
	if err != nil {
		r.bridge.dropConfig(fullID)
		r.verb("[插件] 恢复 %s 失败: %v", fullID, err)
		return
	}
	if js, ok := runtime.(*jsRuntime); ok && entry.jsSources != nil {
		js.setRestoreCache(entry.jsSources)
	}
	if err := runtime.Start(ctx); err != nil {
		r.bridge.dropConfig(fullID)
		r.verb("[插件] 恢复 %s 失败: %v", fullID, err)
		return
	}
	r.mu.Lock()
	r.loaded[fullID] = &LoadedModule{Def: def, Runtime: runtime, Config: store, Enabled: true}
	r.order = append(r.order, fullID)
	r.mu.Unlock()
	r.verb("[插件] 已恢复 %s", fullID)
}

// LoadModuleByFullID locates a module directory by "author/id".
func LoadModuleByFullID(pluginsDir, modulesDir, fullID string) (*Definition, error) {
	// namespaced layout first
	namespaced := filepath.Join(modulesDir, filepath.FromSlash(fullID))
	if def, err := LoadModule(namespaced); err != nil {
		return nil, err
	} else if def != nil {
		return def, nil
	}
	// flat plugins layout: match by fullId inside plugins/<name>
	if entries, err := os.ReadDir(pluginsDir); err == nil {
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			def, err := LoadModule(filepath.Join(pluginsDir, entry.Name()))
			if err != nil || def == nil {
				continue
			}
			if def.FullID() == fullID {
				return def, nil
			}
		}
	}
	return nil, nil
}

// --- cross-module hooks used by the bridge ---

func (r *Registry) moduleSummaries() []ModuleSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]ModuleSummary, 0, len(r.loaded))
	for _, id := range r.order {
		if module, ok := r.loaded[id]; ok {
			out = append(out, r.summaryLocked(module))
		}
	}
	return out
}

func (r *Registry) moduleSummary(fullID string) *ModuleSummary {
	r.mu.Lock()
	defer r.mu.Unlock()
	if module, ok := r.loaded[fullID]; ok {
		summary := r.summaryLocked(module)
		return &summary
	}
	return nil
}

func (r *Registry) summaryLocked(module *LoadedModule) ModuleSummary {
	return ModuleSummary{
		FullID:   module.Def.FullID(),
		ID:       module.Def.Meta.ID,
		Name:     module.Def.Meta.Name,
		Author:   module.Def.Meta.Author,
		Version:  module.Def.Meta.Version,
		Runtime:  module.Def.Meta.Runtime,
		Category: module.Def.Meta.Category,
		Enabled:  module.Enabled,
		Source:   module.Def.Meta.Source,
	}
}

func (r *Registry) moduleExportNames(fullID string) []string {
	r.mu.Lock()
	module, ok := r.loaded[fullID]
	r.mu.Unlock()
	if !ok {
		return nil
	}
	if js, ok := module.Runtime.(*jsRuntime); ok {
		js.mu.Lock()
		names := make([]string, 0, len(js.exports))
		for name := range js.exports {
			names = append(names, name)
		}
		js.mu.Unlock()
		return names
	}
	return nil
}

func (r *Registry) callModuleExport(fullID, fnName string, args []any) (any, error) {
	r.mu.Lock()
	module, ok := r.loaded[fullID]
	r.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("模块 %s 未加载", fullID)
	}
	if js, ok := module.Runtime.(*jsRuntime); ok {
		return js.callExported(fnName, args)
	}
	return nil, fmt.Errorf("模块 %s 的运行时不支持函数导出", fullID)
}

func (r *Registry) loadedCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.loaded)
}

// LoadedSummaries lists loaded modules (for status/API endpoints).
func (r *Registry) LoadedSummaries() []ModuleSummary { return r.moduleSummaries() }
