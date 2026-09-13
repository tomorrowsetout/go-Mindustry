package plugin

import (
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/fsnotify/fsnotify"
)

// FileWatcher monitors the yzf tree (plugins/ and modules/) and pushes module
// reload requests into the registry. It mirrors YZFFileWatcher semantics:
//   - relevant changes: module.hjson/module.json, runtime plugin configs
//     (data/config/config.hjson), and files with supported script extensions
//   - ignored paths: cache/logs/tmp/temp/node_modules segments, *.tmp/.log/
//     .bak/.swp files, stable-api-debug.js
//   - new subdirectories are registered recursively
//   - watcher overflow triggers a full reload
type FileWatcher struct {
	registry *Registry
	verb     func(format string, args ...any)

	watcher  *fsnotify.Watcher
	stopCh   chan struct{}
	doneCh   chan struct{}
	running  bool
	mu       sync.Mutex
	overflow bool
}

// NewFileWatcher creates a watcher bound to the registry. Call Start to begin
// monitoring.
func NewFileWatcher(registry *Registry) *FileWatcher {
	return &FileWatcher{
		registry: registry,
		verb:     registry.verb,
	}
}

// Start begins monitoring the yzf tree. It returns false when the watcher
// could not be created.
func (w *FileWatcher) Start() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.running {
		return true
	}
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		w.verb("[插件] 启动文件监听失败: %v", err)
		return false
	}
	w.watcher = watcher
	w.registerTree(w.registry.plugins)
	w.registerTree(w.registry.modules)
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	w.running = true
	go w.runLoop()
	w.verb("[插件] 文件监听已开启")
	return true
}

// Stop halts monitoring and waits for the loop to exit.
func (w *FileWatcher) Stop() {
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopCh)
	if w.watcher != nil {
		_ = w.watcher.Close()
		w.watcher = nil
	}
	w.mu.Unlock()
	<-w.doneCh
	w.verb("[插件] 文件监听已关闭")
}

// Running reports whether the watcher loop is active.
func (w *FileWatcher) Running() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.running
}

func (w *FileWatcher) runLoop() {
	defer close(w.doneCh)
	for {
		select {
		case <-w.stopCh:
			return
		case event, ok := <-w.watcher.Events:
			if !ok {
				return
			}
			w.handleEvent(event)
		case err, ok := <-w.watcher.Errors:
			if !ok {
				return
			}
			w.verb("[插件] 文件监听异常: %v", err)
		}
	}
}

func (w *FileWatcher) handleEvent(event fsnotify.Event) {
	path := filepath.Clean(event.Name)

	// New subdirectories get watched so plugins added at runtime are picked up.
	if event.Op&fsnotify.Create != 0 {
		if info, err := os.Stat(path); err == nil && info.IsDir() {
			w.mu.Lock()
			if w.watcher != nil {
				w.registerTree(path)
			}
			w.mu.Unlock()
		}
	}

	if !w.relevantChange(path) {
		return
	}
	moduleID := w.resolveModuleID(path)
	if moduleID == "" {
		// A change we cannot attribute to a module (e.g. a plugin dir added or
		// removed wholesale) triggers a full reload, matching YZF behaviour.
		w.registry.RequestReloadAll()
		return
	}
	w.registry.RequestReload(moduleID)
}

// relevantChange mirrors YZFFileWatcher.isRelevantChange.
func (w *FileWatcher) relevantChange(path string) bool {
	if path == "" {
		return false
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() {
		return false
	}
	// Ignore rules apply to the path relative to the watched roots so an
	// OS-level prefix (e.g. a Temp directory hosting the server during tests)
	// can never suppress legitimate plugin changes.
	rel, ok := w.watchedRel(path)
	if !ok {
		return false
	}
	if isIgnoredRelative(rel) {
		return false
	}
	name := strings.ToLower(filepath.Base(path))
	if name == "" {
		return false
	}
	for _, suffix := range []string{".tmp", ".log", ".bak", ".swp"} {
		if strings.HasSuffix(name, suffix) {
			return false
		}
	}
	if name == "module.hjson" || name == "module.json" {
		return true
	}
	// Runtime plugin configuration stored under data/config/config.hjson must
	// trigger a reload just like metadata and scripts.
	if name == "config.hjson" && isRuntimeConfigPath(path) {
		return true
	}
	for ext := range scriptExtensions {
		if strings.HasSuffix(name, "."+ext) {
			return true
		}
	}
	return false
}

// watchedRel returns the path relative to the plugins or modules root.
func (w *FileWatcher) watchedRel(path string) (string, bool) {
	for _, root := range []string{w.registry.plugins, w.registry.modules} {
		if root == "" {
			continue
		}
		rel, err := filepath.Rel(root, path)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			continue
		}
		return filepath.ToSlash(rel), true
	}
	return "", false
}

// resolveModuleID maps a changed file path back to its "author/id" by walking
// up to the plugins/<name> or modules/<author>/<id> root.
func (w *FileWatcher) resolveModuleID(path string) string {
	rel, err := filepath.Rel(w.registry.plugins, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		segments := strings.Split(filepath.ToSlash(rel), "/")
		if len(segments) >= 1 {
			return w.fullIDForPluginDir(segments[0])
		}
	}
	rel, err = filepath.Rel(w.registry.modules, path)
	if err == nil && !strings.HasPrefix(rel, "..") {
		segments := strings.Split(filepath.ToSlash(rel), "/")
		if len(segments) >= 2 {
			return segments[0] + "/" + segments[1]
		}
	}
	return ""
}

// fullIDForPluginDir resolves a plugins/<name> directory to its full id via
// its metadata (author may differ from the directory name).
func (w *FileWatcher) fullIDForPluginDir(dirName string) string {
	def, err := LoadModule(filepath.Join(w.registry.plugins, dirName))
	if err != nil || def == nil {
		return ""
	}
	return def.FullID()
}

// registerTree registers a directory tree recursively. Ignore rules are
// evaluated against the path relative to the watched root so an OS-level
// prefix (e.g. a Temp directory hosting the server) never suppresses a
// legitimate plugin directory.
func (w *FileWatcher) registerTree(dir string) {
	if dir == "" {
		return
	}
	rel, ok := w.watchedRel(dir)
	if !ok {
		return
	}
	if isIgnoredRelative(rel) {
		return
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		return
	}
	if err := w.watcher.Add(dir); err != nil {
		w.verb("[插件] 监听目录失败 %s: %v", dir, err)
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if entry.IsDir() {
			w.registerTree(filepath.Join(dir, entry.Name()))
		}
	}
}

// isIgnoredRelative mirrors YZFFileWatcher.isIgnoredPath but operates on the
// watched-root-relative path so OS-level path segments cannot trigger it.
func isIgnoredRelative(rel string) bool {
	if rel == "" {
		return false
	}
	rel = filepath.ToSlash(rel)
	if strings.EqualFold(filepath.Base(rel), "stable-api-debug.js") {
		return true
	}
	for _, segment := range strings.Split(rel, "/") {
		switch strings.ToLower(segment) {
		case "cache", "logs", "tmp", "temp", "node_modules":
			return true
		}
	}
	return false
}

// isRuntimeConfigPath mirrors YZFFileWatcher.isRuntimeConfigPath.
func isRuntimeConfigPath(path string) bool {
	normalized := strings.ToLower(filepath.ToSlash(path))
	return strings.HasSuffix(normalized, "/config/runtime.hjson") ||
		strings.HasSuffix(normalized, "/data/config/config.hjson")
}
