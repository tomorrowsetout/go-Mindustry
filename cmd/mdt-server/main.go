package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	stdlog "log"
	"math"
	mathrand "math/rand"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"reflect"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"mdt-server/internal/api"
	"mdt-server/internal/bootstrap"
	"mdt-server/internal/buildinfo"
	"mdt-server/internal/buildsvc"
	"mdt-server/internal/config"
	coreio "mdt-server/internal/core"
	"mdt-server/internal/devlog"
	"mdt-server/internal/logging"
	netserver "mdt-server/internal/net"
	"mdt-server/internal/persist"
	"mdt-server/internal/protocol"
	"mdt-server/internal/runtimeassets"
	"mdt-server/internal/sim"
	"mdt-server/internal/storage"
	"mdt-server/internal/tracepoints"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/video"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

type worldState struct {
	mu      sync.RWMutex
	current string
}

type bindStatusCacheEntry struct {
	bound     bool
	expiresAt time.Time
}

type bindStatusResolver struct {
	mode     string
	apiURL   string
	client   *http.Client
	cacheTTL time.Duration
	identity *persist.PlayerIdentityStore
	mu       sync.Mutex
	cache    map[string]bindStatusCacheEntry
}

const defaultConfigPath = "configs/config.toml"

func normalizeRelativePath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	return filepath.Clean(filepath.FromSlash(path))
}

func preferredConfigBases(preferExecutable bool) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	addWithBinParent := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		add(clean)
	}

	addExecutable := func() {
		if exe, err := os.Executable(); err == nil {
			addWithBinParent(filepath.Dir(exe))
		}
	}
	addWorkingDir := func() {
		if wd, err := os.Getwd(); err == nil {
			addWithBinParent(wd)
		}
	}

	if preferExecutable {
		addExecutable()
		addWorkingDir()
	} else {
		addWorkingDir()
		addExecutable()
	}
	return out
}

func resolvePathFromBases(path string, bases []string) string {
	path = normalizeRelativePath(path)
	if path == "" {
		return ""
	}
	if filepath.IsAbs(path) {
		return path
	}
	for _, base := range bases {
		candidate := filepath.Join(base, path)
		if exists(candidate) {
			if abs, err := filepath.Abs(candidate); err == nil {
				return abs
			}
			return filepath.Clean(candidate)
		}
	}
	if len(bases) > 0 {
		candidate := filepath.Join(bases[0], path)
		if abs, err := filepath.Abs(candidate); err == nil {
			return abs
		}
		return filepath.Clean(candidate)
	}
	if abs, err := filepath.Abs(path); err == nil {
		return abs
	}
	return path
}

func resolveStartupConfigPath(raw string) string {
	raw = normalizeRelativePath(raw)
	if raw == "" {
		raw = normalizeRelativePath(defaultConfigPath)
	}
	preferExecutable := strings.EqualFold(raw, normalizeRelativePath(defaultConfigPath))
	return resolvePathFromBases(raw, preferredConfigBases(preferExecutable))
}

func newBindStatusResolver(mode, apiURL string, timeout, cacheTTL time.Duration, identity *persist.PlayerIdentityStore) *bindStatusResolver {
	if timeout <= 0 {
		timeout = 1500 * time.Millisecond
	}
	if cacheTTL <= 0 {
		cacheTTL = 30 * time.Second
	}
	mode = strings.ToLower(strings.TrimSpace(mode))
	if mode != "api" {
		mode = "internal"
	}
	return &bindStatusResolver{
		mode:     mode,
		apiURL:   strings.TrimSpace(apiURL),
		client:   &http.Client{Timeout: timeout},
		cacheTTL: cacheTTL,
		identity: identity,
		cache:    map[string]bindStatusCacheEntry{},
	}
}

func (r *bindStatusResolver) Bound(connUUID string) bool {
	connUUID = strings.TrimSpace(connUUID)
	if connUUID == "" {
		return false
	}
	if r == nil {
		return false
	}
	if r.mode != "api" {
		if r.identity == nil {
			return false
		}
		rec, ok := r.identity.Lookup(connUUID)
		return ok && rec.Bound
	}
	now := time.Now()
	r.mu.Lock()
	if rec, ok := r.cache[connUUID]; ok && now.Before(rec.expiresAt) {
		r.mu.Unlock()
		return rec.bound
	}
	r.mu.Unlock()

	url := strings.ReplaceAll(r.apiURL, "{id}", connUUID)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return false
	}
	resp, err := r.client.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64))
	if err != nil {
		return false
	}
	text := strings.ToLower(strings.TrimSpace(string(body)))
	bound := text == "yes"
	r.mu.Lock()
	r.cache[connUUID] = bindStatusCacheEntry{bound: bound, expiresAt: now.Add(r.cacheTTL)}
	r.mu.Unlock()
	return bound
}

var runtimePlayerNameColorEnabled atomic.Bool
var runtimePublicConnUUIDEnabled atomic.Bool
var runtimeJoinLeaveChatEnabled atomic.Bool
var runtimePlayerNamePrefix atomic.Value
var runtimePlayerNameSuffix atomic.Value
var runtimePlayerBindPrefixEnabled atomic.Bool
var runtimePlayerBoundPrefix atomic.Value
var runtimePlayerUnboundPrefix atomic.Value
var runtimePlayerTitleEnabled atomic.Bool
var runtimePlayerConnIDSuffixEnabled atomic.Bool
var runtimePlayerConnIDSuffixFormat atomic.Value
var runtimePublicConnUUIDStore *persist.PublicConnUUIDStore
var runtimePlayerIdentityStore *persist.PlayerIdentityStore
var runtimeBindStatusResolver *bindStatusResolver
var blockNameTranslationMu sync.RWMutex
var blockNameTranslations = defaultBlockNameTranslations()

type detailedLogWriter struct {
	mu       sync.Mutex
	dir      string
	prefix   string
	maxSize  int64
	maxFiles int
	file     *os.File
	size     int64
	seq      int64
}

func defaultBlockNameTranslations() map[string]string {
	return map[string]string{
		"conveyor":               "传送带",
		"titanium-conveyor":      "钛传送带",
		"armored-conveyor":       "装甲传送带",
		"junction":               "交叉器",
		"router":                 "分配器",
		"sorter":                 "分类器",
		"inverted-sorter":        "反向分类器",
		"overflow-gate":          "溢流门",
		"underflow-gate":         "反向溢流门",
		"duo":                    "双管炮",
		"scatter":                "散射炮",
		"scorch":                 "火焰炮",
		"core-shard":             "核心:碎片",
		"core-foundation":        "核心:基地",
		"core-nucleus":           "核心:核",
		"core-bastion":           "核心:堡垒",
		"core-citadel":           "核心:城塞",
		"core-acropolis":         "核心:卫城",
		"copper-wall":            "铜墙",
		"copper-wall-large":      "大型铜墙",
		"bridge-conveyor":        "传送带桥",
		"phase-conveyor":         "相位传送桥",
		"mass-driver":            "质量驱动器",
		"unloader":               "卸载器",
		"item-source":            "物品源",
		"item-void":              "物品黑洞",
		"power-node":             "电力节点",
		"power-node-large":       "大型电力节点",
		"rtg-generator":          "RTG发电机",
		"differential-generator": "温差发电机",
		"thorium-reactor":        "钍反应堆",
		"impact-reactor":         "冲击反应堆",
	}
}

func parseBlockNameTranslationJSON(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var data map[string]any
	if err := json.Unmarshal(raw, &data); err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for rawKey, value := range data {
		switch typed := value.(type) {
		case map[string]any:
			for rawKey, rawVal := range typed {
				key := strings.ToLower(strings.TrimSpace(rawKey))
				val, ok := rawVal.(string)
				if key == "" || !ok || strings.TrimSpace(val) == "" {
					continue
				}
				out[key] = strings.TrimSpace(val)
			}
		case string:
			key := strings.ToLower(strings.TrimSpace(rawKey))
			if key != "" && strings.TrimSpace(typed) != "" {
				out[key] = typed
			}
		}
	}
	return out, nil
}

func blockNameTranslationCandidates(configDir string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	add := func(base string) {
		base = strings.TrimSpace(base)
		if base == "" {
			return
		}
		candidates := []string{
			filepath.Join(base, "json", "block_names.json"),
			filepath.Join(base, "configs", "json", "block_names.json"),
		}
		for _, candidate := range candidates {
			candidate = filepath.Clean(candidate)
			if _, ok := seen[candidate]; ok {
				continue
			}
			seen[candidate] = struct{}{}
			out = append(out, candidate)
		}
	}
	add(configDir)
	if abs, err := filepath.Abs(configDir); err == nil {
		add(abs)
		add(filepath.Dir(abs))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		add(exeDir)
		add(filepath.Dir(exeDir))
	}
	if wd, err := os.Getwd(); err == nil {
		add(wd)
		add(filepath.Dir(wd))
	}
	return out
}

func applyBlockNameTranslations(configDir string) {
	merged := defaultBlockNameTranslations()
	for _, path := range blockNameTranslationCandidates(configDir) {
		overrides, err := parseBlockNameTranslationJSON(path)
		if err != nil || len(overrides) == 0 {
			continue
		}
		for k, v := range overrides {
			merged[k] = v
		}
		break
	}
	blockNameTranslationMu.Lock()
	blockNameTranslations = merged
	blockNameTranslationMu.Unlock()
}

func resolveConfigSidecarPath(configDir, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	return filepath.Clean(filepath.Join(configDir, raw))
}

func runtimePathBases() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	add(runtimeBaseDir)
	if abs, err := filepath.Abs(runtimeBaseDir); err == nil {
		add(abs)
	}
	if wd, err := os.Getwd(); err == nil {
		add(wd)
	}
	return out
}

func resolveRuntimePath(raw string) string {
	raw = normalizeRelativePath(raw)
	if raw == "" {
		return ""
	}
	if filepath.IsAbs(raw) {
		return filepath.Clean(raw)
	}
	for _, base := range runtimePathBases() {
		candidate := filepath.Join(base, raw)
		if _, err := os.Stat(candidate); err == nil {
			if abs, aerr := filepath.Abs(candidate); aerr == nil {
				return abs
			}
			return filepath.Clean(candidate)
		}
	}
	if base := strings.TrimSpace(runtimeBaseDir); base != "" {
		candidate := filepath.Join(base, raw)
		if abs, err := filepath.Abs(candidate); err == nil {
			return abs
		}
		return filepath.Clean(candidate)
	}
	if abs, err := filepath.Abs(raw); err == nil {
		return abs
	}
	return filepath.Clean(raw)
}

func canonicalRuntimePath(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	clean := filepath.Clean(filepath.FromSlash(raw))
	abs := clean
	if !filepath.IsAbs(abs) {
		abs = resolveRuntimePath(clean)
	}
	base := strings.TrimSpace(runtimeBaseDir)
	if base != "" {
		if baseAbs, err := filepath.Abs(base); err == nil {
			if absClean, aerr := filepath.Abs(abs); aerr == nil {
				if rel, rerr := filepath.Rel(baseAbs, absClean); rerr == nil {
					rel = filepath.Clean(rel)
					if rel == "." {
						return rel
					}
					if rel != "" && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
						return rel
					}
				}
			}
		}
	}
	if filepath.IsAbs(clean) {
		return abs
	}
	return clean
}

func publicConnIDValue(store *persist.PublicConnUUIDStore, uuid string, connID int32) string {
	if !runtimePublicConnUUIDEnabled.Load() {
		return strconv.FormatInt(int64(connID), 10)
	}
	if id := publicConnUUIDValue(store, uuid); id != "" {
		return id
	}
	return strconv.FormatInt(int64(connID), 10)
}

func publicConnUUIDValue(store *persist.PublicConnUUIDStore, uuid string) string {
	uuid = strings.TrimSpace(uuid)
	if uuid != "" && store != nil {
		if id, ok := store.Lookup(uuid); ok && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	}
	return ""
}

func ensureConnIdentityRecords(publicStore *persist.PublicConnUUIDStore, identityStore *persist.PlayerIdentityStore, uuid, name, ip string) (string, bool) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" || publicStore == nil || !runtimePublicConnUUIDEnabled.Load() {
		return "", false
	}
	connUUID, err := publicStore.Ensure(uuid, name, ip)
	if err != nil {
		return "", false
	}
	connUUID = strings.TrimSpace(connUUID)
	if connUUID == "" {
		return "", false
	}
	if identityStore == nil {
		return connUUID, false
	}
	_, ok, err := identityStore.Ensure(connUUID)
	if err != nil {
		return connUUID, false
	}
	return connUUID, ok
}

func lookupConnIdentityState(publicStore *persist.PublicConnUUIDStore, identityStore *persist.PlayerIdentityStore, uuid string) (string, bool, bool) {
	if !runtimePublicConnUUIDEnabled.Load() {
		return "", false, false
	}
	connUUID := publicConnUUIDValue(publicStore, uuid)
	if connUUID == "" {
		return "", false, false
	}
	if identityStore == nil {
		return connUUID, true, false
	}
	_, ok := identityStore.Lookup(connUUID)
	return connUUID, true, ok
}

func shouldAnnotateConnectionCheckpoint(kind string) bool {
	switch strings.TrimSpace(kind) {
	case "connect_packet", "world_handshake_sent", "connect_confirm", "connect_aborted_pre_confirm":
		return true
	default:
		return false
	}
}

func appendConnectionCheckpointDetail(detail string, ev netserver.NetEvent, publicStore *persist.PublicConnUUIDStore, identityStore *persist.PlayerIdentityStore) string {
	if !shouldAnnotateConnectionCheckpoint(ev.Kind) {
		return detail
	}
	connUUID, connUUIDReady, identityReady := lookupConnIdentityState(publicStore, identityStore, ev.UUID)
	displayName := strings.TrimSpace(ev.Name)
	displayNameReady := displayName != ""
	checkpoint := fmt.Sprintf("checkpoint conn_uuid=%q conn_uuid_ready=%t identity_ready=%t display_name_ready=%t display_name=%q",
		connUUID, connUUIDReady, identityReady, displayNameReady, displayName)
	if strings.TrimSpace(detail) == "" {
		return checkpoint
	}
	return detail + " | " + checkpoint
}

func buildCoreSnapshotData(wld *world.World) []byte {
	if wld == nil {
		return []byte{0}
	}
	snapshots := wld.TeamCoreItemSnapshots()
	w := protocol.NewWriter()
	if err := w.WriteByte(byte(len(snapshots))); err != nil {
		return []byte{0}
	}
	for _, snapshot := range snapshots {
		if err := w.WriteByte(byte(snapshot.Team)); err != nil {
			return []byte{0}
		}
		if err := w.WriteInt16(int16(len(snapshot.Items))); err != nil {
			return []byte{0}
		}
		for _, stack := range snapshot.Items {
			if err := w.WriteInt16(int16(stack.Item)); err != nil {
				return []byte{0}
			}
			if err := w.WriteInt32(stack.Amount); err != nil {
				return []byte{0}
			}
		}
	}
	return append([]byte(nil), w.Bytes()...)
}

func newDetailedLogWriter(logsDir string, maxMB, maxFiles int) (*detailedLogWriter, error) {
	dir := strings.TrimSpace(logsDir)
	if dir == "" {
		dir = "logs"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	if maxMB <= 0 {
		maxMB = 2
	}
	if maxFiles <= 0 {
		maxFiles = 100
	}
	w := &detailedLogWriter{
		dir:      dir,
		prefix:   "net-detailed-en",
		maxSize:  int64(maxMB) * 1024 * 1024,
		maxFiles: maxFiles,
	}
	if err := w.openNewLocked(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *detailedLogWriter) openNewLocked() error {
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
		w.size = 0
	}
	w.seq++
	name := fmt.Sprintf("%s-%s-%03d.log", w.prefix, time.Now().Format("20060102-150405"), w.seq)
	path := filepath.Join(w.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	w.file = f
	w.size = 0
	w.cleanupLocked()
	return nil
}

func (w *detailedLogWriter) cleanupLocked() {
	pattern := filepath.Join(w.dir, w.prefix+"-*.log")
	files, err := filepath.Glob(pattern)
	if err != nil || len(files) <= w.maxFiles {
		return
	}
	type fi struct {
		path string
		mod  time.Time
	}
	infos := make([]fi, 0, len(files))
	for _, p := range files {
		st, err := os.Stat(p)
		if err != nil || st.IsDir() {
			continue
		}
		infos = append(infos, fi{path: p, mod: st.ModTime()})
	}
	sort.Slice(infos, func(i, j int) bool { return infos[i].mod.Before(infos[j].mod) })
	for len(infos) > w.maxFiles {
		_ = os.Remove(infos[0].path)
		infos = infos[1:]
	}
}

func (w *detailedLogWriter) LogLine(line string) {
	if w == nil || strings.TrimSpace(line) == "" {
		return
	}
	data := []byte(line + "\n")
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		if err := w.openNewLocked(); err != nil {
			return
		}
	}
	if w.maxSize > 0 && w.size+int64(len(data)) > w.maxSize {
		if err := w.openNewLocked(); err != nil {
			return
		}
	}
	n, _ := w.file.Write(data)
	w.size += int64(n)
}

func (w *detailedLogWriter) Write(p []byte) (n int, err error) {
	if w == nil {
		return 0, nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		if err := w.openNewLocked(); err != nil {
			return 0, err
		}
	}
	if w.maxSize > 0 && w.size+int64(len(p)) > w.maxSize {
		if err := w.openNewLocked(); err != nil {
			return 0, err
		}
	}
	n, err = w.file.Write(p)
	w.size += int64(n)
	return n, err
}

func (w *detailedLogWriter) Close() error {
	if w == nil || w.file == nil {
		return nil
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	err := w.file.Close()
	w.file = nil
	return err
}

var (
	runtimeWorldRoots []string
	runtimeAssetsDir  = "assets"
	runtimeConfigDir  = "configs"
	runtimeBaseDir    = "."
	runtimeWorldPath  atomic.Value
)

type startupStatus int

const (
	startupOK startupStatus = iota
	startupWarn
	startupFail
	startupInfo
)

type startupItem struct {
	status startupStatus
	label  string
	detail string
}

const defaultPlayerRespawnUnitID int16 = 35 // alpha

type startupReport struct {
	items []startupItem
}

func (r *startupReport) add(status startupStatus, label, detail string) {
	r.items = append(r.items, startupItem{status: status, label: label, detail: detail})
}

func (r *startupReport) ok(label, detail string)   { r.add(startupOK, label, detail) }
func (r *startupReport) warn(label, detail string) { r.add(startupWarn, label, detail) }
func (r *startupReport) fail(label, detail string) { r.add(startupFail, label, detail) }
func (r *startupReport) info(label, detail string) { r.add(startupInfo, label, detail) }

func (r *startupReport) print() {
	const (
		green = "\x1b[32m"
		red   = "\x1b[31m"
		yell  = "\x1b[33m"
		reset = "\x1b[0m"
	)
	fmt.Println("========================================")
	fmt.Println(" 启动报告")
	fmt.Println("========================================")
	for _, it := range r.items {
		prefix := "[INFO]"
		color := ""
		switch it.status {
		case startupOK:
			prefix = "[ OK ]"
			color = green
		case startupWarn:
			prefix = "[WARN]"
			color = yell
		case startupFail:
			prefix = "[FAIL]"
			color = red
		}
		if it.detail != "" {
			fmt.Printf("%s%s%s %s: %s\n", color, prefix, reset, it.label, it.detail)
		} else {
			fmt.Printf("%s%s%s %s\n", color, prefix, reset, it.label)
		}
	}
	fmt.Println("========================================")
}

func printUnitIDList(unitNames map[int16]string) {
	if len(unitNames) == 0 {
		return
	}
	type pair struct {
		id   int16
		name string
	}
	items := make([]pair, 0, len(unitNames))
	for id, name := range unitNames {
		items = append(items, pair{id: id, name: name})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].id < items[j].id })

	fmt.Println("========================================")
	fmt.Println(" 单位 ID 列表")
	fmt.Println("========================================")
	const perLine = 6
	for i := 0; i < len(items); i += perLine {
		end := i + perLine
		if end > len(items) {
			end = len(items)
		}
		var b strings.Builder
		for j := i; j < end; j++ {
			if j > i {
				b.WriteString(", ")
			}
			fmt.Fprintf(&b, "%d(%s)", items[j].id, items[j].name)
		}
		fmt.Println(b.String())
	}
	fmt.Println("========================================")
}

type mapLoadStats struct {
	width        int
	height       int
	tiles        int
	blocks       int
	builds       int
	cores        int
	entities     int
	units        int
	blockKinds   int
	floorKinds   int
	overlayKinds int
	tags         int
	msavVersion  int32
	contentBytes int
	patchBytes   int
	rawMapBytes  int
	rawEntBytes  int
	markerBytes  int
	customBytes  int
	hasRulesTag  bool
}

func computeMapLoadStats(model *world.WorldModel) mapLoadStats {
	if model == nil {
		return mapLoadStats{}
	}
	stats := mapLoadStats{
		width:        model.Width,
		height:       model.Height,
		tiles:        len(model.Tiles),
		entities:     len(model.Entities),
		units:        len(model.Units),
		tags:         len(model.Tags),
		msavVersion:  model.MSAVVersion,
		contentBytes: len(model.Content),
		patchBytes:   len(model.Patches),
		rawMapBytes:  len(model.RawMap),
		rawEntBytes:  len(model.RawEntities),
		markerBytes:  len(model.Markers),
		customBytes:  len(model.Custom),
	}
	if model.Tags != nil {
		if v, ok := model.Tags["rules"]; ok && strings.TrimSpace(v) != "" {
			stats.hasRulesTag = true
		}
	}
	blockKinds := map[int16]struct{}{}
	floorKinds := map[int16]struct{}{}
	overlayKinds := map[int16]struct{}{}
	for i := range model.Tiles {
		tile := &model.Tiles[i]
		if tile == nil {
			continue
		}
		if tile.Block > 0 {
			stats.blocks++
			blockKinds[int16(tile.Block)] = struct{}{}
		}
		if tile.Floor > 0 {
			floorKinds[int16(tile.Floor)] = struct{}{}
		}
		if tile.Overlay > 0 {
			overlayKinds[int16(tile.Overlay)] = struct{}{}
		}
		if tile.Build != nil {
			stats.builds++
			name := strings.ToLower(strings.TrimSpace(model.BlockNames[int16(tile.Build.Block)]))
			if strings.Contains(name, "core") || strings.Contains(name, "foundation") || strings.Contains(name, "nucleus") {
				stats.cores++
			}
		}
	}
	stats.blockKinds = len(blockKinds)
	stats.floorKinds = len(floorKinds)
	stats.overlayKinds = len(overlayKinds)
	return stats
}

func printMapDetails(path string, model *world.WorldModel) {
	if model == nil {
		return
	}
	stats := computeMapLoadStats(model)
	fmt.Println("========================================")
	fmt.Println(" 地图加载详情")
	fmt.Println("========================================")
	fmt.Printf("路径: %s\n", path)
	fmt.Printf("尺寸: %dx%d  Tiles=%d\n", stats.width, stats.height, stats.tiles)
	fmt.Printf("建筑: blocks=%d builds=%d cores=%d\n", stats.blocks, stats.builds, stats.cores)
	fmt.Printf("实体: entities=%d units=%d\n", stats.entities, stats.units)
	fmt.Printf("类型: blockKinds=%d floorKinds=%d overlayKinds=%d\n", stats.blockKinds, stats.floorKinds, stats.overlayKinds)
	fmt.Printf("MSAV: version=%d tags=%d rulesTag=%v\n", stats.msavVersion, stats.tags, stats.hasRulesTag)
	fmt.Printf("数据: content=%d patch=%d rawMap=%d rawEntities=%d markers=%d custom=%d\n",
		stats.contentBytes, stats.patchBytes, stats.rawMapBytes, stats.rawEntBytes, stats.markerBytes, stats.customBytes)
	fmt.Println("========================================")
}

func validateBuildVersion(build int) error {
	if build != 157 {
		return fmt.Errorf("仅支持 Mindustry build 157；请使用 -build 157")
	}
	return nil
}

func main() {
	cfgPath := flag.String("config", filepath.FromSlash(defaultConfigPath), "path to config file")
	addr := flag.String("addr", "0.0.0.0:6567", "listen address for Mindustry protocol (TCP+UDP)")
	buildVersion := flag.Int("build", 157, "Mindustry build version; only official build 157 is supported")
	worldArg := flag.String("world", "random", "world source: random | <map-name> | <.msav file path>")
	recordVideo := flag.Bool("record-video", false, "record a realtime top-down match video from live server state")
	videoDir := flag.String("video-dir", filepath.FromSlash("data/video"), "base directory for recorded match video sessions")
	videoFPS := flag.Int("video-fps", 30, "capture FPS for realtime match video recording; common values: 5,10,15,20,25,30")
	videoWidth := flag.Int("video-width", 1920, "video output width in pixels")
	videoHeight := flag.Int("video-height", 1080, "video output height in pixels")
	videoTileSize := flag.Int("video-tile-size", 0, "optional max pixels per tile; 0 lets the recorder fit the whole map automatically")
	coreRole := flag.String("core-role", "", "internal use only: child core role (core2|core3|core4)")
	ipcEndpoint := flag.String("ipc-endpoint", "", "internal use only: child core IPC named pipe endpoint")
	parentPID := flag.Int("parent-pid", 0, "internal use only: parent process ID for child cores")
	printVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *printVersion {
		name := strings.TrimSpace(buildinfo.DisplayName)
		if name == "" {
			name = "mdt-server"
		}
		fmt.Printf("%s %s (%s)\n", name, buildinfo.Version, buildinfo.Commit)
		return
	}

	resolvedCfgPath := resolveStartupConfigPath(*cfgPath)
	cfg := config.Default()
	cfg.Source = resolvedCfgPath
	if loaded, err := config.Load(cfg.Source); err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		os.Exit(1)
	} else {
		cfg = loaded
		cfg.Source = resolvedCfgPath
	}
	applyProcessConsoleTitle(cfg, strings.TrimSpace(*coreRole), cfg.Runtime.ServerName)
	applyProcessWindowIcon()
	if strings.TrimSpace(*coreRole) != "" {
		if err := coreio.RunChildCore(*coreRole, *ipcEndpoint, *parentPID); err != nil {
			fmt.Fprintf(os.Stderr, "child core %s failed: %v\n", *coreRole, err)
			os.Exit(1)
		}
		return
	}
	if err := validateBuildVersion(*buildVersion); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	configDir := filepath.Dir(cfg.Source)
	if strings.TrimSpace(configDir) == "" {
		configDir = filepath.FromSlash("configs")
	}
	rootDir := filepath.Dir(configDir)
	if strings.TrimSpace(rootDir) == "" || rootDir == configDir {
		rootDir = "."
	}
	runtimeBaseDir = rootDir
	// 非配置类目录/文件全部以 EXE 根目录为基准生成；configs 只存放配置文件。
	config.ApplyBaseDir(&cfg, rootDir)
	detailLog, detailLogErr := newDetailedLogWriter(cfg.Runtime.LogsDir, cfg.Sundries.DetailedLogMaxMB, cfg.Sundries.DetailedLogMaxFiles)
	if detailLogErr != nil {
		fmt.Fprintf(os.Stderr, "初始化 logs 详细日志失败: %v\n", detailLogErr)
		os.Exit(1)
	}
	traceLog, traceLogErr := tracepoints.New(cfg.Tracepoints.File, cfg.Tracepoints.Enabled)
	if traceLogErr != nil {
		fmt.Fprintf(os.Stderr, "初始化 tracepoints 日志失败: %v\n", traceLogErr)
		os.Exit(1)
	}
	defer func() {
		_ = traceLog.Close()
	}()

	// 设置标准log输出到文件和控制台
	logMultiWriter := io.MultiWriter(os.Stdout, detailLog)
	stdlog.SetOutput(logMultiWriter)

	var runtimeTraceCfg atomic.Value
	runtimeTraceCfg.Store(cfg.Tracepoints)
	currentTraceCfg := func() config.TracepointsConfig {
		if v := runtimeTraceCfg.Load(); v != nil {
			if loaded, ok := v.(config.TracepointsConfig); ok {
				return loaded
			}
		}
		return config.Default().Tracepoints
	}
	logTrace := func(category, point string, fields map[string]any) {
		if traceLog == nil || !traceLog.Enabled() {
			return
		}
		traceLog.Log(category, point, fields)
	}

	runtimeConfigDir = configDir
	runtimeAssetsDir = cfg.Runtime.AssetsDir
	runtimeWorldRoots = []string{cfg.Runtime.WorldsDir}
	applyBlockNameTranslations(configDir)
	initStatusBarRuntime(cfg)
	initJoinPopupRuntime(cfg)
	initMapVoteRuntimeConfig(cfg)

	startMemoryGuard(cfg.Core)

	startup := &startupReport{}
	bootstrapResult, err := bootstrap.EnsureWorkspace(cfg.Source, cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化工作目录失败: %v\n", err)
		os.Exit(1)
	}

	log := logging.New(os.Stdout)
	saveConfig := func() error {
		if cfg.Source == "" {
			return nil
		}
		return config.SaveSidecars(cfg.Source, cfg)
	}
	keys, keyErr := mergeValidAPIKeys(cfg.API.Keys, cfg.API.Key)
	if keyErr != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", keyErr)
		os.Exit(1)
	}
	cfg.API.Keys = keys
	cfg.API.Key = ""
	if st, ok, err := persist.LoadScriptConfig(cfg.Script); err == nil && ok {
		cfg.Script.StartupTasks = st.StartupTasks
		cfg.Script.DailyGCTime = st.DailyGCTime
	}

	nameForBanner := strings.TrimSpace(buildinfo.DisplayName)
	if nameForBanner == "" {
		nameForBanner = "mdt-server"
	}
	fmt.Fprintf(os.Stdout, "%s %s (%s)\n", nameForBanner, buildinfo.Version, buildinfo.Commit)
	fmt.Fprintf(os.Stdout, "config=%s cores=%d tps=%d addr=%s build=%d world=%s vanilla=%s\n", canonicalRuntimePath(cfg.Source), cfg.Runtime.Cores, cfg.Core.TPS, *addr, *buildVersion, *worldArg, canonicalRuntimePath(cfg.Runtime.VanillaProfiles))
	if len(bootstrapResult.CreatedDirs) > 0 || len(bootstrapResult.CreatedFiles) > 0 {
		fmt.Fprintf(os.Stdout, "workspace initialized: dirs=%d files=%d\n", len(bootstrapResult.CreatedDirs), len(bootstrapResult.CreatedFiles))
	}
	gameVersion := strings.TrimSpace(buildinfo.GameVersion)
	if gameVersion == "" {
		gameVersion = fmt.Sprintf("Mindustry %d", *buildVersion)
	}
	startup.ok("游戏版本", gameVersion)
	if strings.TrimSpace(buildinfo.Version) != "" {
		startup.ok("外部版本", buildinfo.Version)
	}

	// 初始化开发者日志
	devLog := devlog.New(os.Stdout)
	devLog.SetLevel(devlog.LogLevelDebug)
	if cfg.Runtime.DevLogEnabled {
		startup.ok("开发者日志", "已启用")
	} else {
		startup.info("开发者日志", "未启用")
	}

	// Mod system disabled for now
	var modMgr interface{} = nil
	if cfg.Mods.Enabled {
		startup.warn("模组系统", "暂未实现")
	} else {
		startup.info("Mods", "未启用")
	}

	worldChoice := *worldArg
	var persisted persist.State
	var persistedOK bool
	if cfg.Persist.Enabled {
		if st, ok, err := persist.Load(cfg.Persist); err != nil {
			log.Warn("persist load failed", logging.Field{Key: "error", Value: err.Error()})
			startup.warn("持久化加载", err.Error())
		} else if ok {
			persisted = st
			persistedOK = true
			if worldChoice == "random" && st.MapPath != "" {
				worldChoice = st.MapPath
				startup.ok("持久化地图恢复", st.MapPath)
			}
		}
	} else {
		startup.info("持久化", "未启用")
	}

	initialWorld, err := resolveWorldSelection(worldChoice)
	if err != nil {
		fmt.Fprintf(os.Stderr, "世界选择无效: %v\n", err)
		os.Exit(1)
	}
	initialWorld = canonicalRuntimePath(initialWorld)
	state := &worldState{current: initialWorld}
	runtimePlayerNameColorEnabled.Store(cfg.Personalization.PlayerNameColorEnabled)
	runtimeJoinLeaveChatEnabled.Store(cfg.Personalization.JoinLeaveChatEnabled)
	runtimePlayerNamePrefix.Store(cfg.Personalization.PlayerNamePrefix)
	runtimePlayerNameSuffix.Store(cfg.Personalization.PlayerNameSuffix)
	runtimePlayerBindPrefixEnabled.Store(cfg.Personalization.PlayerBindPrefixEnabled)
	runtimePlayerBoundPrefix.Store(cfg.Personalization.PlayerBoundPrefix)
	runtimePlayerUnboundPrefix.Store(cfg.Personalization.PlayerUnboundPrefix)
	runtimePlayerTitleEnabled.Store(cfg.Personalization.PlayerTitleEnabled)
	runtimePlayerConnIDSuffixEnabled.Store(cfg.Personalization.PlayerConnIDSuffixEnabled)
	runtimePlayerConnIDSuffixFormat.Store(cfg.Personalization.PlayerConnIDSuffixFormat)
	if cfg.Personalization.StartupCurrentMapLineEnabled {
		fmt.Fprintf(os.Stdout, "当前地图: %s\n", canonicalRuntimePath(initialWorld))
	}

	var publicConnUUIDStore *persist.PublicConnUUIDStore
	var playerIdentityStore *persist.PlayerIdentityStore

	srv := netserver.NewServer(*addr, *buildVersion)
	applyAdmissionPolicy := func(loaded config.Config) error {
		var entries []netserver.AdmissionWhitelistEntry
		if loaded.Admin.WhitelistEnabled {
			loadedEntries, err := netserver.LoadAdmissionWhitelistFile(loaded.Admin.WhitelistFile)
			if err != nil {
				return err
			}
			entries = loadedEntries
		}
		srv.SetAdmissionPolicy(netserver.AdmissionPolicy{
			StrictIdentity:     loaded.Admin.StrictIdentity,
			AllowCustomClients: loaded.Admin.AllowCustomClients,
			PlayerLimit:        loaded.Admin.PlayerLimit,
			WhitelistEnabled:   loaded.Admin.WhitelistEnabled,
			Whitelist:          entries,
			ExpectedMods:       loaded.Mods.ExpectedClientMods,
			BannedNames:        loaded.Admin.BannedNames,
			BannedSubnets:      loaded.Admin.BannedSubnets,
			RecentKickDuration: time.Duration(loaded.Admin.RecentKickSeconds) * time.Second,
		})
		return nil
	}
	if err := applyAdmissionPolicy(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "加载 admission 配置失败: %v\n", err)
		os.Exit(1)
	}
	srv.SetVerboseNetLog(false)
	srv.SetPacketRecvEventsEnabled(cfg.Development.PacketRecvEventsEnabled)
	srv.SetPacketSendEventsEnabled(cfg.Development.PacketSendEventsEnabled)
	srv.SetTerminalPlayerLogsEnabled(cfg.Development.TerminalPlayerLogsEnabled)
	srv.SetTerminalPlayerUUIDEnabled(cfg.Development.TerminalPlayerUUIDEnabled)
	srv.SetRespawnPacketLogsEnabled(cfg.Development.RespawnPacketLogsEnabled)
	srv.SetPlayerNameColorEnabled(cfg.Personalization.PlayerNameColorEnabled)
	srv.SetTranslatedConnLog(cfg.Control.TranslatedConnLogEnabled)
	srv.SetJoinLeaveChatEnabled(cfg.Personalization.JoinLeaveChatEnabled)
	srv.OnTracePacket = func(direction string, c *netserver.Conn, obj any, packetID int, frameworkID int, size int) {
		tc := currentTraceCfg()
		if !tc.Enabled {
			return
		}
		switch direction {
		case "recv":
			if !tc.ClientRequestsEnabled {
				return
			}
			extra := map[string]any{}
			if c != nil {
				extra["conn_id"] = c.ConnID()
				extra["player_id"] = c.PlayerID()
				extra["uuid"] = c.UUID()
			}
			logTrace("client_request", "packet_recv", tracepoints.PacketFields(direction, obj, packetID, frameworkID, size, extra))
		case "send":
			if !tc.ServerSendsEnabled {
				return
			}
			extra := map[string]any{}
			if c != nil {
				extra["conn_id"] = c.ConnID()
				extra["player_id"] = c.PlayerID()
				extra["uuid"] = c.UUID()
			}
			logTrace("server_send", "packet_send", tracepoints.PacketFields(direction, obj, packetID, frameworkID, size, extra))
		}
	}
	startStatusBarLoop(srv)
	srv.SetPlayerDisplayFormatter(func(c *netserver.Conn) string {
		if c == nil {
			return ""
		}
		return formatDisplayPlayerNameRaw(c.BaseName(), c, publicConnUUIDStore, playerIdentityStore)
	})
	srv.RefreshPlayerDisplayNames()
	var (
		effectIDMu      sync.RWMutex
		effectIDsByName = map[string]int16{}
	)
	setEffectIDs := func(ids *vanilla.ContentIDsFile) {
		next := make(map[string]int16)
		if ids != nil {
			for _, entry := range ids.Effects {
				name := strings.ToLower(strings.TrimSpace(entry.Name))
				if name == "" {
					continue
				}
				next[name] = entry.ID
			}
		}
		effectIDMu.Lock()
		effectIDsByName = next
		effectIDMu.Unlock()
	}
	lookupEffectID := func(name string) (int16, bool) {
		name = strings.ToLower(strings.TrimSpace(name))
		if name == "" {
			return 0, false
		}
		effectIDMu.RLock()
		id, ok := effectIDsByName[name]
		effectIDMu.RUnlock()
		return id, ok
	}
	contentIDsPath := filepath.Join(filepath.Dir(cfg.Runtime.VanillaProfiles), "content_ids.json")
	if ids, err := vanilla.LoadContentIDs(contentIDsPath); err != nil {
		startup.warn("原版 content IDs", fmt.Sprintf("未加载(%s): %v", canonicalRuntimePath(contentIDsPath), err))
	} else {
		setEffectIDs(ids)
		count := vanilla.ApplyContentIDs(srv.Content, ids)
		startup.ok("原版 content IDs", fmt.Sprintf("entries=%d path=%s", count, canonicalRuntimePath(contentIDsPath)))
	}
	srv.SetServerName(cfg.Runtime.ServerName)
	srv.SetServerDescription(cfg.Runtime.ServerDesc)
	srv.SetVirtualPlayers(int32(cfg.Runtime.VirtualPlayers))
	srv.UdpRetryCount = cfg.Net.UdpRetryCount
	srv.UdpRetryDelay = time.Duration(cfg.Net.UdpRetryDelayMs) * time.Millisecond
	srv.UdpFallbackTCP = cfg.Net.UdpFallbackTCP
	srv.SetSnapshotIntervals(cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs)
	gameTPS := cfg.Core.TPS
	if gameTPS <= 0 {
		gameTPS = sim.DefaultTPS
	}
	wld := world.New(world.Config{
		TPS:                    gameTPS,
		UseMapSyncDataFallback: cfg.Sync.UseMapSyncDataFallback,
		BlockSyncLogsEnabled:   cfg.Sync.BlockSyncLogsEnabled,
	})
	srv.EntitySnapshotHiddenFn = func(viewer *netserver.Conn, entity protocol.UnitSyncEntity) bool {
		if viewer == nil || wld == nil {
			return false
		}
		unit, ok := entity.(*protocol.UnitEntitySync)
		if !ok || unit == nil {
			return false
		}
		viewerX, viewerY := viewer.SnapshotPos()
		return wld.UnitSyncHiddenForViewer(world.TeamID(viewer.TeamID()), viewerX, viewerY, unit)
	}
	srv.TypeIO.BuildingLookup = func(pos int32) protocol.Building {
		if wld == nil {
			return nil
		}
		info, ok := wld.BuildingInfoTileIndex(pos)
		if !ok {
			return nil
		}
		if wld.CanControlBuildingPacked(info.Pos) {
			return protocol.ControlBuildingRef{
				PosValue: pos,
				UnitRef: protocol.BlockUnitRef{
					TileRef: protocol.BlockUnitTileRef{PosValue: pos},
				},
			}
		}
		return protocol.BuildingBox{PosValue: pos}
	}
	buildInfoToControlled := func(info world.BuildingInfo) netserver.ControlledBuildInfo {
		return netserver.ControlledBuildInfo{
			Pos:    info.Pos,
			X:      float32(info.X*8 + 4),
			Y:      float32(info.Y*8 + 4),
			TeamID: byte(info.Team),
		}
	}
	srv.ClaimControlledBuildFn = func(playerID int32, buildPos int32) (netserver.ControlledBuildInfo, bool) {
		info, ok := wld.ClaimControlledBuildingPacked(playerID, buildPos)
		if !ok {
			return netserver.ControlledBuildInfo{}, false
		}
		return buildInfoToControlled(info), true
	}
	srv.ControlledBuildInfoFn = func(playerID int32, buildPos int32) (netserver.ControlledBuildInfo, bool) {
		info, ok := wld.ControlledBuildingInfoPacked(playerID, buildPos)
		if !ok {
			return netserver.ControlledBuildInfo{}, false
		}
		return buildInfoToControlled(info), true
	}
	srv.ReleaseControlledBuildFn = func(playerID int32, buildPos int32) bool {
		if buildPos != 0 {
			if wld.ReleaseControlledBuildingPacked(playerID, buildPos) {
				return true
			}
		}
		return wld.ReleaseControlledBuildingByPlayer(playerID)
	}
	srv.SetControlledBuildInputFn = func(playerID int32, buildPos int32, aimX, aimY float32, shooting bool) bool {
		return wld.SetControlledBuildingInputPacked(playerID, buildPos, aimX, aimY, shooting)
	}
	// clientSnapshot writes client motion/position back into the authoritative world model.
	// This mirrors vanilla NetServer.clientSnapshot behavior; without it, entitySnapshot will
	// keep snapping units back to stale positions.
	srv.SetUnitMotionFn = func(unitID int32, vx, vy, rotVel float32) bool {
		_, ok := wld.SetEntityMotion(unitID, vx, vy, rotVel)
		return ok
	}
	srv.SetUnitPositionFn = func(unitID int32, x, y, rotation float32) bool {
		_, ok := wld.SetEntityPosition(unitID, x, y, rotation)
		return ok
	}
	srv.SetUnitRuntimeStateFn = func(unitID int32, state netserver.UnitRuntimeState) bool {
		_, ok := wld.SetEntityRuntimeState(unitID, state.Shooting, state.Boosting, state.UpdateBuilding, state.MineTilePos, state.Plans)
		return ok
	}
	srv.SetUnitStackFn = func(unitID int32, itemID int16, amount int32) bool {
		_, ok := wld.SetEntityStack(unitID, world.ItemID(itemID), amount)
		return ok
	}
	srv.SetUnitPlayerControllerFn = func(unitID int32, playerID int32) bool {
		_, ok := wld.SetEntityPlayerController(unitID, playerID)
		return ok
	}
	srv.OnRequestUnitPayload = func(c *netserver.Conn, targetID int32) {
		if c == nil || wld == nil || c.UnitID() == 0 || targetID == 0 {
			return
		}
		_, _ = wld.RequestUnitPayload(c.UnitID(), targetID)
	}
	srv.OnRequestBuildPayload = func(c *netserver.Conn, buildPos int32) {
		if c == nil || wld == nil || c.UnitID() == 0 || buildPos < 0 {
			return
		}
		_, _ = wld.RequestBuildPayloadPacked(c.UnitID(), buildPos)
	}
	srv.OnRequestDropPayload = func(c *netserver.Conn, x, y float32) {
		if c == nil || wld == nil || c.UnitID() == 0 {
			return
		}
		_, _ = wld.RequestDropPayload(c.UnitID(), x, y)
	}
	srv.OnRequestItem = func(c *netserver.Conn, pos int32, itemID int16, amount int32) {
		if c == nil || wld == nil || c.UnitID() == 0 || amount <= 0 {
			return
		}
		result, ok := wld.RequestItemFromBuildingPacked(c.UnitID(), pos, world.ItemID(itemID), amount)
		if !ok || result.Amount <= 0 {
			return
		}
		broadcastTakeItems(srv, pos, itemID, result.Amount, result.UnitID)
	}
	srv.OnTransferInventory = func(c *netserver.Conn, pos int32) {
		if c == nil || wld == nil || c.UnitID() == 0 {
			return
		}
		result, ok := wld.TransferUnitInventoryToBuildingPacked(c.UnitID(), pos)
		if !ok || result.Amount <= 0 {
			return
		}
		broadcastTransferItemTo(srv, result.UnitID, int16(result.Item), result.Amount, result.UnitX, result.UnitY, pos)
	}
	srv.OnDropItem = func(c *netserver.Conn, angle float32) {
		if c == nil || wld == nil || c.UnitID() == 0 || c.PlayerID() == 0 {
			return
		}
		if _, ok := wld.DropUnitItems(c.UnitID()); !ok {
			return
		}
		broadcastDropItem(srv, c.PlayerID(), angle)
	}
	srv.OnUnitEnteredPayload = func(c *netserver.Conn, unitID, buildPos int32) {
		if wld == nil || unitID == 0 || buildPos < 0 {
			return
		}
		if wld.EnterUnitPayloadPacked(buildPos, unitID) && c != nil && c.UnitID() == unitID {
			srv.ConsumeConnUnit(c, unitID)
		}
	}
	unitCommands := newUnitCommandService()
	buildService := buildsvc.New(wld, buildsvc.Options{
		MaxQueuedBatches: 256,
		MaxPlansPerBatch: 20,
		MaxOpsPerTick:    64,
	})
	shouldLogBuildSnapshots := func() bool {
		return cfg.Building.Enabled && cfg.Development.BuildSnapshotLogsEnabled
	}
	shouldLogBuildPlace := func() bool {
		return cfg.Building.Enabled && cfg.Development.BuildPlaceLogsEnabled
	}
	shouldLogBuildFinish := func() bool {
		return cfg.Building.Enabled && cfg.Development.BuildFinishLogsEnabled
	}
	shouldLogBuildBreakStart := func() bool {
		return cfg.Building.Enabled && cfg.Development.BuildBreakStartLogsEnabled
	}
	shouldLogBuildBreakDone := func() bool {
		return cfg.Building.Enabled && cfg.Development.BuildBreakDoneLogsEnabled
	}
	shouldLogRespawnCore := func() bool {
		return cfg.Development.RespawnCoreLogsEnabled
	}
	shouldLogRespawnUnit := func() bool {
		return cfg.Development.RespawnUnitLogsEnabled
	}
	shouldFileLogNetEvents := func() bool {
		return cfg.Sundries.NetEventLogsEnabled
	}
	shouldFileLogChat := func() bool {
		return cfg.Sundries.ChatLogsEnabled
	}
	shouldFileLogRespawnCore := func() bool {
		return cfg.Sundries.RespawnCoreLogsEnabled
	}
	shouldFileLogRespawnUnit := func() bool {
		return cfg.Sundries.RespawnUnitLogsEnabled
	}
	shouldFileLogBuildPlace := func() bool {
		return cfg.Sundries.BuildPlaceLogsEnabled
	}
	shouldFileLogBuildFinish := func() bool {
		return cfg.Sundries.BuildFinishLogsEnabled
	}
	shouldFileLogBuildBreakStart := func() bool {
		return cfg.Sundries.BuildBreakStartLogsEnabled
	}
	shouldFileLogBuildBreakDone := func() bool {
		return cfg.Sundries.BuildBreakDoneLogsEnabled
	}
	srv.SpawnUnitFn = func(c *netserver.Conn, unitID int32, tile protocol.Point2, unitType int16) (float32, float32, bool) {
		if c == nil || wld == nil {
			return 0, 0, false
		}
		team := resolveConnTeam(c, wld)
		if corePos, coreName, ok := resolveTeamCoreTileWithName(wld, team, tile); ok && shouldLogRespawnCore() {
			fmt.Printf("[重生] 玩家=%s 队伍=%d 核心=%s 核心坐标=(%d,%d) 出生点=(%d,%d)\n",
				displayPlayerName(c), team, translateBlockNameCN(coreName), corePos.X, corePos.Y, tile.X, tile.Y)
		} else if shouldLogRespawnCore() {
			if model := wld.Model(); model != nil && model.InBounds(int(tile.X), int(tile.Y)) {
				if t, err := model.TileAt(int(tile.X), int(tile.Y)); err == nil && t != nil {
					blockName := strings.ToLower(strings.TrimSpace(model.BlockNames[int16(t.Block)]))
					fmt.Printf("[重生] 玩家=%s 队伍=%d 未找到核心，回退出生点=(%d,%d) 地块=%s\n",
						displayPlayerName(c), team, tile.X, tile.Y, translateBlockNameCN(blockName))
				}
			}
		}
		if corePos, coreName, ok := resolveTeamCoreTileWithName(wld, team, tile); ok && shouldFileLogRespawnCore() {
			detailLog.LogLine(fmt.Sprintf("%s [RESPAWN] player=%q team=%d core=%s core_x=%d core_y=%d spawn_x=%d spawn_y=%d",
				time.Now().Format(time.RFC3339Nano), c.Name(), team, strings.ToLower(strings.TrimSpace(coreName)), corePos.X, corePos.Y, tile.X, tile.Y))
		} else if shouldFileLogRespawnCore() {
			blockName := ""
			if model := wld.Model(); model != nil && model.InBounds(int(tile.X), int(tile.Y)) {
				if t, err := model.TileAt(int(tile.X), int(tile.Y)); err == nil && t != nil {
					blockName = strings.ToLower(strings.TrimSpace(model.BlockNames[int16(t.Block)]))
				}
			}
			detailLog.LogLine(fmt.Sprintf("%s [RESPAWN] player=%q team=%d no_core=1 spawn_x=%d spawn_y=%d tile=%s",
				time.Now().Format(time.RFC3339Nano), c.Name(), team, tile.X, tile.Y, blockName))
		}
		spawnUnitType := unitType
		if spawnUnitType <= 0 {
			spawnUnitType = defaultPlayerRespawnUnitID
			if alphaID, ok := wld.ResolveUnitTypeID("alpha"); ok {
				spawnUnitType = alphaID
			}
		}
		spawnUnitType = resolveRespawnUnitTypeByCoreTile(wld, tile, team, spawnUnitType)
		builderSpeed := wld.BuilderSpeedForUnitType(spawnUnitType)
		wld.SetTeamBuilderSpeed(team, builderSpeed)
		if shouldLogRespawnUnit() {
			fmt.Printf("[重生] 玩家=%s 队伍=%d 出生单位=%d 建造速度=%.2f 出生点=(%d,%d)\n",
				displayPlayerName(c), team, spawnUnitType, builderSpeed, tile.X, tile.Y)
		}
		if shouldFileLogRespawnUnit() {
			detailLog.LogLine(fmt.Sprintf("%s [RESPAWN] player=%q team=%d unit=%d build_speed=%.2f spawn_x=%d spawn_y=%d",
				time.Now().Format(time.RFC3339Nano), c.Name(), team, spawnUnitType, builderSpeed, tile.X, tile.Y))
		}
		x := float32(tile.X*8 + 4)
		y := float32(tile.Y*8 + 4)
		ent, err := wld.AddEntityWithID(spawnUnitType, unitID, x, y, team)
		if err != nil {
			return 0, 0, false
		}
		_, _ = wld.SetEntitySpawnedByCore(ent.ID, true)
		_, _ = wld.SetEntityPlayerController(ent.ID, c.PlayerID())
		return x, y, true
	}
	srv.SpawnUnitAtFn = func(c *netserver.Conn, unitID int32, x, y, rotation float32, unitType int16, spawnedByCore bool) (float32, float32, bool) {
		if c == nil || wld == nil || unitType <= 0 {
			return 0, 0, false
		}
		team := resolveConnTeam(c, wld)
		builderSpeed := wld.BuilderSpeedForUnitType(unitType)
		wld.SetTeamBuilderSpeed(team, builderSpeed)
		ent, err := wld.AddEntityWithID(unitType, unitID, x, y, team)
		if err != nil {
			return 0, 0, false
		}
		_, _ = wld.SetEntitySpawnedByCore(ent.ID, spawnedByCore)
		_, _ = wld.SetEntityPosition(ent.ID, x, y, rotation)
		_, _ = wld.SetEntityPlayerController(ent.ID, c.PlayerID())
		return x, y, true
	}
	srv.ResolveRespawnUnitTypeFn = func(c *netserver.Conn, tile protocol.Point2, fallback int16) int16 {
		if wld == nil {
			return fallback
		}
		return resolveRespawnUnitTypeByCoreTile(wld, tile, resolveConnTeam(c, wld), fallback)
	}
	srv.ReserveUnitIDFn = func() int32 {
		if wld == nil {
			return 0
		}
		return wld.ReserveEntityID()
	}
	srv.DropUnitFn = func(unitID int32) {
		if wld == nil {
			return
		}
		wld.RemoveEntity(unitID)
	}
	srv.UnitInfoFn = func(unitID int32) (netserver.UnitInfo, bool) {
		if wld == nil {
			return netserver.UnitInfo{}, false
		}
		ent, ok := wld.GetEntity(unitID)
		if !ok {
			return netserver.UnitInfo{}, false
		}
		return netserver.UnitInfo{
			ID:        ent.ID,
			X:         ent.X,
			Y:         ent.Y,
			Health:    ent.Health,
			MaxHealth: ent.MaxHealth,
			TeamID:    byte(ent.Team),
			TypeID:    ent.TypeID,
		}, true
	}
	srv.UnitSyncFn = func(unitID int32, controller protocol.UnitController) (*protocol.UnitEntitySync, bool) {
		if wld == nil {
			return nil, false
		}
		return wld.UnitSyncSnapshot(srv.Content, unitID, controller)
	}
	var unitNamesByID map[int16]string
	var loadedModel *world.WorldModel
	var loadedMapPath string
	invalidateWorldCache := func() {}

	var playerSpawnTypeID int32 = int32(defaultPlayerRespawnUnitID)
	if err := wld.LoadVanillaProfiles(cfg.Runtime.VanillaProfiles); err != nil {
		log.Warn("vanilla profiles load failed", logging.Field{Key: "path", Value: cfg.Runtime.VanillaProfiles}, logging.Field{Key: "error", Value: err.Error()})
		startup.warn("原版 profiles", fmt.Sprintf("加载失败: %s", err.Error()))
	} else if strings.TrimSpace(cfg.Runtime.VanillaProfiles) != "" {
		startup.ok("原版 profiles", canonicalRuntimePath(cfg.Runtime.VanillaProfiles))
	}
	loadWorldModel := func(path string) {
		path = canonicalRuntimePath(path)
		runtimeWorldPath.Store(path)
		actualPath := resolveRuntimePath(path)
		buildService.Reset()
		lower := strings.ToLower(path)
		if !strings.HasSuffix(lower, ".msav") && !strings.HasSuffix(lower, ".msav.msav") {
			wld.SetModel(nil)
			loadedModel = nil
			loadedMapPath = ""
			return
		}
		model, lerr := worldstream.LoadWorldModelFromMSAV(actualPath, srv.Content)
		if lerr != nil {
			log.Warn("world model load failed", logging.Field{Key: "path", Value: path}, logging.Field{Key: "error", Value: lerr.Error()})
			startup.warn("地图模型", fmt.Sprintf("加载失败: %s", lerr.Error()))
			loadedModel = nil
			loadedMapPath = ""
			return
		}
		wld.SetModel(model)
		if srv.Content != nil && model != nil {
			mapBlockRegs := 0
			for id, name := range model.BlockNames {
				normalized := strings.ToLower(strings.TrimSpace(name))
				if normalized == "" {
					continue
				}
				srv.Content.RegisterBlock(blockRef{id: id, name: normalized})
				mapBlockRegs++
			}
			mapUnitRegs := 0
			for id, name := range model.UnitNames {
				normalized := strings.ToLower(strings.TrimSpace(name))
				if normalized == "" {
					continue
				}
				srv.Content.RegisterUnitType(unitTypeRef{id: id, name: normalized})
				mapUnitRegs++
			}
			if mapBlockRegs > 0 || mapUnitRegs > 0 {
				startup.info("地图内容注册", fmt.Sprintf("blocks=%d units=%d", mapBlockRegs, mapUnitRegs))
			}
		}
		loadedModel = model
		loadedMapPath = path
		startup.ok("地图模型", fmt.Sprintf("%s (%dx%d)", path, model.Width, model.Height))
		if summary := world.DescribeRuleMode(model, wld.GetRulesManager().Get()); summary.Mode != "" {
			modeName := summary.ModeName
			if modeName == "" {
				modeName = "-"
			}
			startup.info("地图模式", fmt.Sprintf("mode=%s modeName=%s waves=%v waveTimer=%v pvp=%v attack=%v editor=%v infiniteResources=%v infiniteAmmo=%v",
				summary.Mode,
				modeName,
				summary.Waves,
				summary.WaveTimer,
				summary.Pvp,
				summary.AttackMode,
				summary.Editor,
				summary.InfiniteResources,
				summary.InfiniteAmmo,
			))
		}
		if model != nil && len(model.UnitNames) > 0 {
			unitNamesByID = make(map[int16]string, len(model.UnitNames))
			for k, v := range model.UnitNames {
				unitNamesByID[k] = strings.ToLower(strings.TrimSpace(v))
			}
			startup.ok("单位 ID 列表", fmt.Sprintf("count=%d", len(unitNamesByID)))
		}
		spawnType := defaultPlayerRespawnUnitID
		if alphaID, ok := wld.ResolveUnitTypeID("alpha"); ok {
			spawnType = alphaID
		}
		atomic.StoreInt32(&playerSpawnTypeID, int32(spawnType))
		for rawTeam := 1; rawTeam <= 255; rawTeam++ {
			team := world.TeamID(rawTeam)
			corePos, ok := resolveTeamCoreTile(wld, team, protocol.Point2{})
			if !ok {
				continue
			}
			unitType := resolveRespawnUnitTypeByCoreTile(wld, corePos, team, spawnType)
			if speed := wld.BuilderSpeedForUnitType(unitType); speed > 0 {
				wld.SetTeamBuilderSpeed(team, speed)
			}
		}
		startup.ok("玩家出生单位", fmt.Sprintf("typeId=%d", spawnType))
	}
	var cache *worldCache
	applyVotedWorld := func(next string) error {
		state.set(next)
		loadWorldModel(next)
		if invalidateWorldCache != nil {
			invalidateWorldCache()
		}
		reloaded, failed := srv.ReloadWorldLiveForAll()
		mapName := worldstream.TrimMapName(filepath.Base(next))
		if reloaded == 0 && failed == 0 {
			srv.BroadcastChat(fmt.Sprintf("[accent]地图已切换[]: [white]%s[]（当前无在线玩家）", mapName))
			return nil
		}
		srv.BroadcastChat(fmt.Sprintf("[accent]地图已切换[]: [white]%s[]（成功=%d 失败=%d）", mapName, reloaded, failed))
		return nil
	}
	initMapVoteRuntime(listWorldMaps, resolveWorldSelection, applyVotedWorld, func(result mapVoteResult) {
		handleMapVoteResult(srv, currentMapVoteRuntime(), result)
	}, srv)
	if persistedOK {
		waveTime := persisted.WaveTime
		if waveTime < 0 || waveTime > 3600 {
			waveTime = 600
		}
		wld.ApplySnapshot(world.Snapshot{
			WaveTime: waveTime,
			Wave:     persisted.Wave,
			TimeData: persisted.TimeData,
			Tps:      int8(gameTPS),
			Rand0:    persisted.Rand0,
			Rand1:    persisted.Rand1,
			Tick:     persisted.Tick,
		})
	}
	loadWorldModel(initialWorld)
	srv.MapNameFn = func() string {
		path := state.get()
		if path == "" {
			return "unknown"
		}
		return worldstream.TrimMapName(filepath.Base(path))
	}
	recorder, rerr := storage.NewRecorder(cfg.Storage)
	if rerr != nil {
		fmt.Fprintf(os.Stderr, "事件存储初始化失败: %v\n", rerr)
		os.Exit(1)
	}
	runtimePublicConnUUIDEnabled.Store(cfg.Control.PublicConnUUIDEnabled)
	publicConnUUIDPath := resolveConfigSidecarPath(runtimeConfigDir, cfg.Control.PublicConnUUIDFile)
	var publicConnUUIDErr error
	publicConnUUIDStore, publicConnUUIDErr = persist.NewPublicConnUUIDStore(publicConnUUIDPath, cfg.Control.ConnUUIDAutoCreateEnabled)
	if publicConnUUIDErr != nil {
		log.Warn("public conn_uuid store init failed", logging.Field{Key: "error", Value: publicConnUUIDErr.Error()})
		startup.warn("公开 conn_uuid", publicConnUUIDErr.Error())
	} else {
		runtimePublicConnUUIDStore = publicConnUUIDStore
	}
	playerIdentityPath := resolveConfigSidecarPath(runtimeConfigDir, cfg.Personalization.PlayerIdentityFile)
	playerIdentityStore, publicIdentityErr := persist.NewPlayerIdentityStore(playerIdentityPath, cfg.Control.PlayerIdentityAutoCreateEnabled)
	if publicIdentityErr != nil {
		log.Warn("player identity store init failed", logging.Field{Key: "error", Value: publicIdentityErr.Error()})
		startup.warn("玩家身份配置", publicIdentityErr.Error())
	} else {
		runtimePlayerIdentityStore = playerIdentityStore
	}
	srv.OnConnectAccepted = func(conn *netserver.Conn, pkt *protocol.ConnectPacket) {
		if conn == nil {
			return
		}
		sourceName := strings.TrimSpace(conn.BaseName())
		if sourceName == "" && pkt != nil {
			sourceName = strings.TrimSpace(pkt.Name)
		}
		_, _ = ensureConnIdentityRecords(publicConnUUIDStore, playerIdentityStore, conn.UUID(), sourceName, connRemoteIP(conn))
	}
	runtimeBindStatusResolver = newBindStatusResolver(
		cfg.Personalization.PlayerBindSource,
		cfg.Personalization.PlayerBindAPIURL,
		time.Duration(cfg.Personalization.PlayerBindAPITimeoutMs)*time.Millisecond,
		time.Duration(cfg.Personalization.PlayerBindAPICacheSec)*time.Second,
		playerIdentityStore,
	)
	srv.OnEvent = func(ev netserver.NetEvent) {
		if publicConnUUIDStore != nil && ev.Kind == "connect_packet" && runtimePublicConnUUIDEnabled.Load() {
			if current := publicConnUUIDValue(publicConnUUIDStore, ev.UUID); current == "" {
				name := strings.TrimSpace(netserver.StripMindustryColorTags(ev.Name))
				_, _ = ensureConnIdentityRecords(publicConnUUIDStore, playerIdentityStore, ev.UUID, name, ev.IP)
			}
		}
		ev.Detail = appendConnectionCheckpointDetail(ev.Detail, ev, publicConnUUIDStore, playerIdentityStore)
		_ = recorder.Record(storage.Event{
			Timestamp: ev.Timestamp,
			Kind:      ev.Kind,
			Packet:    ev.Packet,
			Detail:    ev.Detail,
			ConnID:    ev.ConnID,
			UUID:      ev.UUID,
			IP:        ev.IP,
			Name:      ev.Name,
		})
		line := fmt.Sprintf("%s [NET] kind=%s packet=%s conn_id=%s uuid=%s ip=%s name=%q detail=%s",
			ev.Timestamp.Format(time.RFC3339Nano), ev.Kind, ev.Packet, publicConnIDValue(publicConnUUIDStore, ev.UUID, ev.ConnID), ev.UUID, ev.IP, ev.Name, ev.Detail)
		if shouldFileLogNetEvents() {
			detailLog.LogLine(line)
		}
		tc := currentTraceCfg()
		if tc.Enabled {
			if tc.ClientRequestsEnabled && (ev.Kind == "packet_recv" || ev.Kind == "connect_packet" || strings.HasPrefix(ev.Kind, "connect_confirm") || strings.Contains(ev.Kind, "client_snapshot")) {
				logTrace("client_request", ev.Kind, map[string]any{
					"packet":  ev.Packet,
					"detail":  ev.Detail,
					"conn_id": ev.ConnID,
					"uuid":    ev.UUID,
					"ip":      ev.IP,
					"name":    ev.Name,
				})
			}
			if tc.ServerSendsEnabled && (ev.Kind == "packet_send" || ev.Kind == "world_handshake_sent" || strings.Contains(ev.Kind, "state_snapshot") || strings.Contains(ev.Kind, "entity_snapshot")) {
				logTrace("server_send", ev.Kind, map[string]any{
					"packet":  ev.Packet,
					"detail":  ev.Detail,
					"conn_id": ev.ConnID,
					"uuid":    ev.UUID,
					"ip":      ev.IP,
					"name":    ev.Name,
				})
			}
		}
	}
	srv.SetPublicConnIDFormatter(func(c *netserver.Conn) string {
		if c == nil || !runtimePublicConnUUIDEnabled.Load() {
			return ""
		}
		if publicConnUUIDStore == nil {
			return ""
		}
		id, ok := publicConnUUIDStore.Lookup(c.UUID())
		if !ok {
			return ""
		}
		return id
	})
	if ops, ok, err := persist.LoadOps(cfg.Admin); err != nil {
		log.Warn("ops load failed", logging.Field{Key: "error", Value: err.Error()})
		startup.warn("OP 列表", err.Error())
	} else if ok {
		for _, u := range ops {
			srv.AddOp(u)
		}
		startup.ok("OP 列表", fmt.Sprintf("count=%d", len(ops)))
	}
	saveOps := func() {
		_ = persist.SaveOps(cfg.Admin, srv.ListOps())
	}
	saveScript := func() error {
		return persist.SaveScriptConfig(cfg.Script, persist.ScriptState{
			Version:      1,
			StartupTasks: cfg.Script.StartupTasks,
			DailyGCTime:  cfg.Script.DailyGCTime,
		})
	}
	scriptCtl := newScriptController(cfg.Mods)
	scriptCtl.ScheduleStartupTasks(cfg.Script.StartupTasks)
	scriptCtl.SetDailyGC(cfg.Script.DailyGCTime)

	cache = &worldCache{content: srv.Content}
	invalidateWorldCache = func() {
		cache.invalidate()
		if err := warmWorldCache(cache, state.get()); err != nil {
			log.Warn("world cache warm failed", logging.Field{Key: "path", Value: state.get()}, logging.Field{Key: "error", Value: err.Error()})
		}
	}
	if err := warmWorldCache(cache, state.get()); err != nil {
		log.Warn("world cache warm failed", logging.Field{Key: "path", Value: state.get()}, logging.Field{Key: "error", Value: err.Error()})
		startup.warn("世界流缓存", err.Error())
	} else {
		startup.ok("世界流缓存", state.get())
	}
	srv.WorldDataFn = func(conn *netserver.Conn, _ *protocol.ConnectPacket) ([]byte, error) {
		payload, err := buildInitialWorldDataPayload(conn, wld, cache, state.get())
		if err == nil {
			tc := currentTraceCfg()
			if tc.Enabled && tc.WorldStreamEnabled {
				playerID := int32(0)
				connID := int32(0)
				liveStream := false
				if conn != nil {
					playerID = conn.PlayerID()
					connID = conn.ConnID()
					liveStream = conn.UsesLiveWorldStream()
				}
				snap := world.Snapshot{}
				if wld != nil {
					snap = wld.Snapshot()
				}
				logTrace("world_stream", "build_initial_world_data_payload", map[string]any{
					"conn_id":          connID,
					"player_id":        playerID,
					"map_path":         state.get(),
					"payload_bytes":    len(payload),
					"live_worldstream": liveStream,
					"wave":             snap.Wave,
					"wave_time":        snap.WaveTime,
					"tick":             snap.Tick,
				})
			}
		}
		return payload, err
	}
	srv.OnPostConnect = func(conn *netserver.Conn) {
		if conn == nil {
			return
		}
		// Let the client finish applying the world stream, then only push lightweight
		// overlays. Full world reload remains on explicit /sync.
		time.Sleep(350 * time.Millisecond)
		syncRulesToConn(conn, wld, state.get())
		showJoinPopupForConn(srv, conn)
	}
	srv.OnMenuChoose = func(c *netserver.Conn, menuID, option int32) {
		handleJoinPopupMenuChoice(srv, c, menuID, option)
	}
	srv.OnHotReloadConnFn = func(conn *netserver.Conn) {
		if conn == nil {
			return
		}
		syncRulesToConn(conn, wld, state.get())
		srv.RefreshPlayerDisplayNames()
	}
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		if pos, ok := resolveTeamCoreTile(wld, resolveDefaultPlayerTeam(wld), protocol.Point2{}); ok {
			return pos, true
		}
		pos, ok, err := cache.spawnPos(state.get())
		if err == nil && ok {
			return pos, true
		}
		// Fallback for maps where core tile cannot be parsed from msav metadata.
		return fallbackSpawnPosFromModel(wld.Model())
	}
	srv.AssignTeamForConnFn = func(c *netserver.Conn) byte {
		return byte(assignConnTeamVanilla(srv, wld, c))
	}
	spawnRefForConn := func(c *netserver.Conn) protocol.Point2 {
		if c == nil {
			return protocol.Point2{}
		}
		if wld != nil {
			if unitID := c.UnitID(); unitID != 0 {
				if ent, ok := wld.GetEntity(unitID); ok {
					return protocol.Point2{
						X: int32(math.Round((float64(ent.X) - 4) / 8)),
						Y: int32(math.Round((float64(ent.Y) - 4) / 8)),
					}
				}
			}
		}
		x, y := c.SnapshotPos()
		if x == 0 && y == 0 {
			return protocol.Point2{}
		}
		return protocol.Point2{
			X: int32(math.Round((float64(x) - 4) / 8)),
			Y: int32(math.Round((float64(y) - 4) / 8)),
		}
	}
	srv.SpawnTileForConnFn = func(c *netserver.Conn) (protocol.Point2, bool) {
		team := resolveConnTeam(c, wld)
		if pos, ok := resolveTeamCoreTile(wld, team, spawnRefForConn(c)); ok {
			return pos, true
		}
		if wld != nil {
			if pos, ok := fallbackSpawnPosFromModel(wld.Model()); ok {
				return pos, true
			}
		}
		return srv.SpawnTileFn()
	}
	// Official 157 build authority comes from clientSnapshot queue updates.
	// Keep the old shared OnBuildPlans queue disabled so cancelled plans are removed
	// by authoritative snapshot reconciliation instead of lingering in a second queue.
	srv.OnBuildPlans = nil
	type snapshotLogKey struct {
		count    int
		breaking bool
		x        int32
		y        int32
		blockID  int16
	}
	var (
		snapshotLogMu sync.Mutex
		snapshotLogBy = make(map[int32]snapshotLogKey)
		ownerActorMu  sync.RWMutex
		ownerActorBy  = make(map[int32]string)
	)
	buildActor := func(owner int32, team world.TeamID) string {
		if owner != 0 {
			ownerActorMu.RLock()
			actor := strings.TrimSpace(ownerActorBy[owner])
			ownerActorMu.RUnlock()
			if actor != "" {
				return actor
			}
		}
		return fmt.Sprintf("team-%d", team)
	}
	rememberBuildOwner := func(c *netserver.Conn, owner int32) {
		if c == nil || owner == 0 {
			return
		}
		ownerActorMu.Lock()
		ownerActorBy[owner] = displayPlayerName(c)
		ownerActorMu.Unlock()
	}
	handleCoreBuildingControlSelect := func(c *netserver.Conn, info world.BuildingInfo) {
		if c == nil || wld == nil || c.IsDead() || c.UnitID() == 0 {
			return
		}
		if !strings.HasPrefix(info.Name, "core-") || info.Team != resolveConnTeam(c, wld) {
			return
		}
		_ = srv.HandleCoreBuildingControlSelect(c, protocol.UnpackPoint2(info.Pos))
	}
	handlePlayerPayloadBuildingControlSelect := func(c *netserver.Conn, info world.BuildingInfo) {
		if c == nil || wld == nil || c.IsDead() || c.UnitID() == 0 {
			return
		}
		unitID := c.UnitID()
		if !wld.ControlSelectPayloadUnitPacked(info.Pos, unitID) {
			return
		}
		srv.ConsumeConnUnit(c, unitID)
		broadcastRelatedBlockSnapshots(srv, wld, info.Pos)
	}
	handleUnitPayloadBuildingControlSelect := func(c *netserver.Conn, info world.BuildingInfo, unitID int32) {
		if wld == nil || unitID == 0 {
			return
		}
		if !wld.ControlSelectPayloadUnitPacked(info.Pos, unitID) {
			return
		}
		if c != nil && c.UnitID() == unitID {
			srv.ConsumeConnUnit(c, unitID)
		}
		broadcastRelatedBlockSnapshots(srv, wld, info.Pos)
	}
	srv.OnRotateBlock = func(c *netserver.Conn, pos int32, direction bool) {
		res, ok := wld.RotateBuildingPacked(pos, direction)
		if !ok {
			return
		}
		broadcastSetTile(srv, pos, res.BlockID, res.Rotation, byte(res.Team))
		if effectID, ok := lookupEffectID("rotateblock"); ok {
			broadcastEffectReliable(srv, effectID, res.EffectX, res.EffectY, res.EffectRot)
		}
		broadcastRelatedBlockSnapshots(srv, wld, pos)
	}
	srv.OnRequestBlockSnapshot = func(c *netserver.Conn, pos int32) {
		if c == nil || wld == nil {
			return
		}
		info, ok := wld.BuildingInfoTileIndex(pos)
		if !ok || info.Team != resolveConnTeam(c, wld) {
			return
		}
		sendRequestedBlockSnapshotToConn(c, wld, info.Pos)
	}
	srv.OnBuildingControlSelect = func(c *netserver.Conn, pos int32) {
		if wld == nil || c == nil {
			return
		}
		info, ok := wld.BuildingInfoTileIndex(pos)
		if !ok {
			return
		}
		switch {
		case strings.HasPrefix(info.Name, "core-"):
			if !wld.CanControlSelectBuildingPacked(info.Pos) {
				return
			}
			handleCoreBuildingControlSelect(c, info)
		case wld.CanControlBuildingPacked(info.Pos):
			if info.Team != resolveConnTeam(c, wld) {
				return
			}
			_ = srv.HandleBuildingControlSelect(c, info.Pos)
		default:
			if !wld.CanControlSelectBuildingPacked(info.Pos) {
				return
			}
			handlePlayerPayloadBuildingControlSelect(c, info)
		}
	}
	srv.OnUnitBuildingControlSelect = func(c *netserver.Conn, unitID, pos int32) {
		if wld == nil || unitID == 0 {
			return
		}
		info, ok := wld.BuildingInfoTileIndex(pos)
		if !ok || !wld.CanControlSelectBuildingPacked(info.Pos) {
			return
		}
		if strings.HasPrefix(info.Name, "core-") {
			return
		}
		handleUnitPayloadBuildingControlSelect(c, info, unitID)
	}
	srv.OnBuildPlanSnapshot = func(c *netserver.Conn, plans []*protocol.BuildPlan) {
		if c == nil {
			return
		}
		owner := resolveBuildOwner(c)
		team := resolveConnTeam(c, wld)
		syncBuilderStateFromConnSnapshot(wld, c, owner, team, plans, false)
		rememberBuildOwner(c, owner)
		if len(plans) == 0 {
			key := snapshotLogKey{count: 0}
			snapshotLogMu.Lock()
			prev, ok := snapshotLogBy[c.PlayerID()]
			changed := !ok || prev != key
			if changed {
				snapshotLogBy[c.PlayerID()] = key
			}
			snapshotLogMu.Unlock()
			if changed && shouldLogBuildSnapshots() && !cfg.Building.Translated {
				fmt.Printf("[buildtrace] recv snapshot player=%d remote=%s count=0\n", c.PlayerID(), c.RemoteAddr().String())
			}
		} else {
			first := plans[0]
			blockID := int16(0)
			if first != nil && !first.Breaking && first.Block != nil {
				blockID = first.Block.ID()
			}
			if first != nil {
				key := snapshotLogKey{
					count:    len(plans),
					breaking: first.Breaking,
					x:        first.X,
					y:        first.Y,
					blockID:  blockID,
				}
				snapshotLogMu.Lock()
				prev, ok := snapshotLogBy[c.PlayerID()]
				changed := !ok || prev != key
				if changed {
					snapshotLogBy[c.PlayerID()] = key
				}
				snapshotLogMu.Unlock()
				if changed && shouldLogBuildSnapshots() {
					if cfg.Building.Translated {
						action := "建造"
						if first.Breaking {
							action = "拆除"
						}
						fmt.Printf("[建筑] 玩家=%s 快照队列=%d 首项=(x%d-y%d) 动作=%s block=%d(%s) team=%d\n",
							displayPlayerName(c), len(plans), first.X, first.Y, action, blockID, blockDisplayName(wld, blockID), team)
					} else {
						fmt.Printf("[buildtrace] recv snapshot player=%d remote=%s count=%d first_break=%v first_xy=(%d,%d) first_block=%d\n",
							c.PlayerID(), c.RemoteAddr().String(), len(plans), first.Breaking, first.X, first.Y, blockID)
					}
				}
			}
		}
		buildService.SyncPlans(owner, team, plans)
	}
	srv.OnDeletePlans = func(c *netserver.Conn, positions []int32) {
		owner := resolveBuildOwner(c)
		if c != nil && len(positions) > 0 && shouldLogBuildSnapshots() && !cfg.Building.Translated {
			fmt.Printf("[buildtrace] recv deletePlans player=%d remote=%s count=%d\n", c.PlayerID(), c.RemoteAddr().String(), len(positions))
		}
		buildService.CancelPositions(owner, positions)
		wld.CancelBuildPlansPackedForOwner(owner, positions)
	}
	srv.OnRemoveQueueBlock = func(c *netserver.Conn, x, y int32, breaking bool) {
		if c == nil {
			return
		}
		owner := resolveBuildOwner(c)
		if cfg.Building.Translated {
			action := "取消建造"
			if breaking {
				action = "取消拆除"
			}
			fmt.Printf("[建筑] 玩家=%s (x%d-y%d) %s\n", displayPlayerName(c), x, y, action)
		}
		if shouldLogBuildSnapshots() && !cfg.Building.Translated {
			fmt.Printf("[buildtrace] recv removeQueue player=%d remote=%s xy=(%d,%d) breaking=%v\n", c.PlayerID(), c.RemoteAddr().String(), x, y, breaking)
		}
		buildService.CancelPositions(owner, []int32{protocol.PackPoint2(x, y)})
		wld.CancelBuildAtForOwner(owner, x, y, breaking)
	}
	srv.OnCommandUnits = func(c *netserver.Conn, unitIDs []int32, buildTarget any, unitTarget any, posTarget any, queueCommand bool, _ bool) {
		unitCommands.applyCommandUnits(c, wld, unitIDs, buildTarget, unitTarget, posTarget, queueCommand)
	}
	srv.OnSetUnitCommand = func(c *netserver.Conn, unitIDs []int32, command *protocol.UnitCommand) {
		unitCommands.applySetUnitCommand(c, wld, unitIDs, command)
	}
	srv.OnSetUnitStance = func(c *netserver.Conn, unitIDs []int32, stance protocol.UnitStance, enable bool) {
		unitCommands.applySetUnitStance(c, wld, unitIDs, stance, enable)
	}
	srv.OnCommandBuilding = func(c *netserver.Conn, buildings []int32, target protocol.Vec2) {
		wld.CommandBuildingsPacked(buildings, target)
		for _, pos := range buildings {
			broadcastRelatedBlockSnapshots(srv, wld, pos)
		}
	}
	srv.OnTileConfig = func(c *netserver.Conn, pos int32, value any) {
		wld.ConfigureBuildingPacked(pos, value)
		if normalized, ok := wld.BuildingConfigPacked(pos); ok {
			srv.BroadcastTileConfig(pos, normalized, c)
		} else {
			srv.BroadcastTileConfig(pos, value, c)
		}
		broadcastRelatedBlockSnapshots(srv, wld, pos)
	}
	srv.PlayerUnitTypeFn = func() int16 {
		return int16(atomic.LoadInt32(&playerSpawnTypeID))
	}
	srv.StateSnapshotFn = func() *protocol.Remote_NetClient_stateSnapshot_35 {
		snap := wld.Snapshot()
		tc := currentTraceCfg()
		if tc.Enabled && tc.StateBuildEnabled {
			logTrace("state_build", "build_state_snapshot", map[string]any{
				"wave":      snap.Wave,
				"wave_time": snap.WaveTime,
				"tick":      snap.Tick,
				"time_data": snap.TimeData,
				"tps":       snap.Tps,
			})
		}
		return &protocol.Remote_NetClient_stateSnapshot_35{
			// Mindustry state.wavetime is in "tick" units (~60 per second).
			WaveTime: snap.WaveTime * 60,
			Wave:     snap.Wave,
			Enemies:  snap.Enemies,
			Paused:   snap.Paused,
			GameOver: snap.GameOver,
			TimeData: snap.TimeData,
			Tps:      snap.Tps,
			Rand0:    snap.Rand0,
			Rand1:    snap.Rand1,
			CoreData: buildCoreSnapshotData(wld),
		}
	}
	srv.ExtraEntitySnapshotEntitiesFn = func() ([]protocol.UnitSyncEntity, error) {
		return unitCommands.overlay(wld.EntitySyncSnapshots(srv.Content, srv.PlayerUnitIDSet())), nil
	}
	traceWorldRuntimeTick := func(stage string) {
		tc := currentTraceCfg()
		if !tc.Enabled || !tc.WorldRuntimeEnabled || wld == nil {
			return
		}
		st := wld.TraceRuntimeState()
		logTrace("world_runtime", stage, map[string]any{
			"tick":           st.Tick,
			"wave":           st.Wave,
			"wave_time":      st.WaveTime,
			"time_data":      st.TimeData,
			"tps":            st.TPS,
			"active_tiles":   st.ActiveTiles,
			"entities":       st.Entities,
			"bullets":        st.Bullets,
			"pending_builds": st.PendingBuilds,
			"pending_breaks": st.PendingBreaks,
		})
	}
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				fmt.Printf("[net] event-loop panic err=%v\n", rec)
			}
		}()
		t := time.NewTicker(time.Second / time.Duration(gameTPS))
		defer t.Stop()
		nextBlockSnapshotSync := time.Now()
		nextUnitFactorySnapshotSync := time.Now()
		nextPayloadProcessorSnapshotSync := time.Now()
		nextPlanPreviewSync := time.Now()
		for range t.C {
			now := time.Now()
			if !now.Before(nextPlanPreviewSync) {
				// Vanilla relays teammate build-preview plans, but our current
				// clientPlanSnapshotReceived writer is not byte-perfect yet and can
				// crash 157 clients while decoding preview blocks. Keep receiving
				// client previews server-side, but stop re-broadcasting them until the
				// packet format is fully aligned with the original implementation.
				nextPlanPreviewSync = now.Add(500 * time.Millisecond)
			}
			evs := wld.DrainEntityEvents()
			groupedExplosionBuilds := classifyReactorExplosionBuilds(wld, evs)
			buildHealth := make([]int32, 0, len(evs)*2)
			blockItemSync := make(map[int32]struct{})
			itemTurretAmmoSync := make(map[int32]struct{})
			for i := range evs {
				ev := evs[i]
				switch ev.Kind {
				case world.EntityEventRemoved:
					unitCommands.remove(ev.Entity.ID)
					broadcastUnitDestroy(srv, ev.Entity.ID)
					if ev.Entity.Health <= 0 {
						if _, ok := srv.PlayerUnitIDSet()[ev.Entity.ID]; ok {
							fmt.Printf("[net] world removed player-unit=%d hp=%.2f pos=(%.1f,%.1f) team=%d\n",
								ev.Entity.ID, ev.Entity.Health, ev.Entity.X, ev.Entity.Y, ev.Entity.Team)
						}
						srv.MarkUnitDead(ev.Entity.ID, "world-removed")
					} else {
						if _, ok := srv.PlayerUnitIDSet()[ev.Entity.ID]; ok {
							fmt.Printf("[net] ignored unit removal conn-unit=%d source=world-removed-positive-health hp=%.2f pos=(%.1f,%.1f)\n",
								ev.Entity.ID, ev.Entity.Health, ev.Entity.X, ev.Entity.Y)
						}
					}
				case world.EntityEventBuildPlaced:
					x, y := unpackTilePos(ev.BuildPos)
					if shouldFileLogBuildPlace() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=placed x=%d y=%d block_id=%d block=%s team=%d rot=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam, ev.BuildRot))
					}
					if shouldLogBuildPlace() {
						if cfg.Building.Translated {
							actor := buildActor(ev.BuildOwner, ev.BuildTeam)
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 建造了 block=%d(%s) team=%d rot=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam, ev.BuildRot)
						} else {
							fmt.Printf("[buildtrace] placed xy=(%d,%d) block=%d team=%d rot=%d\n", x, y, ev.BuildBlock, ev.BuildTeam, ev.BuildRot)
						}
					}
					broadcastBuildBeginPlace(srv, ev.BuildPos, ev.BuildBlock, ev.BuildRot, byte(ev.BuildTeam), ev.BuildConfig)
				case world.EntityEventBuildConstructed:
					x, y := unpackTilePos(ev.BuildPos)
					if shouldFileLogBuildFinish() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=constructed x=%d y=%d block_id=%d block=%s team=%d rot=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam, ev.BuildRot))
					}
					if shouldLogBuildFinish() {
						if cfg.Building.Translated {
							actor := buildActor(ev.BuildOwner, ev.BuildTeam)
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 完成建造 block=%d(%s) team=%d rot=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam, ev.BuildRot)
						} else {
							fmt.Printf("[buildtrace] constructed xy=(%d,%d) block=%d team=%d rot=%d\n", x, y, ev.BuildBlock, ev.BuildTeam, ev.BuildRot)
						}
					}
					broadcastBuildConstructedState(srv, wld, ev)
				case world.EntityEventBuildConfig:
					if cfgValue, ok := wld.BuildingConfigPacked(ev.BuildPos); ok {
						srv.BroadcastTileConfig(ev.BuildPos, cfgValue, nil)
					} else if ev.BuildConfig != nil {
						srv.BroadcastTileConfig(ev.BuildPos, ev.BuildConfig, nil)
					}
					broadcastRelatedBlockSnapshots(srv, wld, ev.BuildPos)
				case world.EntityEventBuildDeconstructing:
					x, y := unpackTilePos(ev.BuildPos)
					if shouldFileLogBuildBreakStart() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=deconstructing x=%d y=%d block_id=%d block=%s team=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam))
					}
					if shouldLogBuildBreakStart() {
						if cfg.Building.Translated {
							actor := buildActor(ev.BuildOwner, ev.BuildTeam)
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 正在拆除 block=%d(%s) team=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam)
						} else {
							fmt.Printf("[buildtrace] deconstructing xy=(%d,%d) block=%d team=%d\n", x, y, ev.BuildBlock, ev.BuildTeam)
						}
					}
					broadcastBuildDeconstructBegin(srv, ev.BuildPos, byte(ev.BuildTeam))
				case world.EntityEventBuildCancelled:
					x, y := unpackTilePos(ev.BuildPos)
					if shouldLogBuildBreakDone() {
						if cfg.Building.Translated {
							actor := buildActor(ev.BuildOwner, ev.BuildTeam)
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 取消了建造 block=%d(%s) team=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam)
						} else {
							fmt.Printf("[buildtrace] cancelled xy=(%d,%d) block=%d team=%d\n", x, y, ev.BuildBlock, ev.BuildTeam)
						}
					}
					broadcastBuildDestroyed(srv, ev.BuildPos, ev.BuildBlock)
				case world.EntityEventBuildDestroyed:
					x, y := unpackTilePos(ev.BuildPos)
					if shouldFileLogBuildBreakDone() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=destroyed x=%d y=%d block_id=%d block=%s team=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam))
					}
					if shouldLogBuildBreakDone() && groupedExplosionBuilds[i] == nil {
						if cfg.Building.Translated {
							if ev.BuildOwner != 0 {
								actor := buildActor(ev.BuildOwner, ev.BuildTeam)
								fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 拆除了 block=%d(%s) team=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam)
							} else {
								fmt.Printf("[建筑] (x%d-y%d) 被摧毁了 block=%d(%s) team=%d\n", x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam)
							}
						} else {
							fmt.Printf("[buildtrace] destroyed xy=(%d,%d) block=%d team=%d\n", x, y, ev.BuildBlock, ev.BuildTeam)
						}
					}
					broadcastBuildDestroyedState(srv, ev)
				case world.EntityEventBuildHealth:
					buildHealth = append(buildHealth, ev.BuildPos, int32(math.Float32bits(ev.BuildHP)))
				case world.EntityEventBlockItemSync:
					blockItemSync[ev.BuildPos] = struct{}{}
				case world.EntityEventItemTurretAmmoSync:
					itemTurretAmmoSync[ev.BuildPos] = struct{}{}
				case world.EntityEventTransferItemToUnit:
					amount := ev.ItemAmount
					if amount <= 0 {
						amount = 1
					}
					for n := int32(0); n < amount; n++ {
						broadcastTransferItemToUnit(srv, int16(ev.ItemID), ev.TransferX, ev.TransferY, ev.UnitID)
					}
				case world.EntityEventTransferItemToBuild:
					broadcastTransferItemTo(srv, ev.UnitID, int16(ev.ItemID), ev.ItemAmount, ev.TransferX, ev.TransferY, ev.BuildPos)
				case world.EntityEventBulletFired:
					broadcastBulletCreate(srv, ev.Bullet)
				case world.EntityEventEffect:
					if effectID, ok := lookupEffectID(ev.EffectName); ok {
						broadcastEffectReliable(srv, effectID, ev.EffectX, ev.EffectY, ev.EffectRot)
					}
				}
			}
			if len(buildHealth) > 0 {
				// Send all health deltas in small chunks; do not trim tail,
				// otherwise construct/deconstruct progress appears to "jump".
				const maxInts = 256 // 128 buildings per packet
				for i := 0; i < len(buildHealth); i += maxInts {
					end := i + maxInts
					if end > len(buildHealth) {
						end = len(buildHealth)
					}
					broadcastBuildHealthUpdate(srv, buildHealth[i:end])
				}
			}
			if len(blockItemSync) > 0 {
				positions := make([]int32, 0, len(blockItemSync))
				for packed := range blockItemSync {
					positions = append(positions, packed)
				}
				sort.Slice(positions, func(i, j int) bool { return positions[i] < positions[j] })
				broadcastItemBlockSnapshotsForPacked(srv, wld, positions)
			}
			if len(itemTurretAmmoSync) > 0 {
				positions := make([]int32, 0, len(itemTurretAmmoSync))
				for packed := range itemTurretAmmoSync {
					positions = append(positions, packed)
				}
				sort.Slice(positions, func(i, j int) bool { return positions[i] < positions[j] })
				broadcastItemTurretAmmoSnapshotsForPacked(srv, wld, positions)
			}
			snapshotNow := time.Now()
			if !snapshotNow.Before(nextUnitFactorySnapshotSync) {
				broadcastUnitFactorySnapshots(srv, wld)
				nextUnitFactorySnapshotSync = snapshotNow.Add(time.Second)
			}
			if !snapshotNow.Before(nextPayloadProcessorSnapshotSync) {
				broadcastPayloadProcessorSnapshots(srv, wld)
				nextPayloadProcessorSnapshotSync = snapshotNow.Add(time.Second)
			}
			if !now.Before(nextBlockSnapshotSync) {
				broadcastBlockSnapshots(srv, wld)
				nextBlockSnapshotSync = now.Add(6 * time.Second)
			}
			if shouldLogBuildBreakDone() {
				logGroupedReactorExplosions(wld, evs, groupedExplosionBuilds)
			}
		}
	}()
	saveState := func() {}
	srv.OnChat = func(c *netserver.Conn, msg string) bool {
		if c != nil && strings.TrimSpace(msg) != "" && shouldFileLogChat() {
			detailLog.LogLine(fmt.Sprintf("%s [CHAT] from=%q player_id=%d uuid=%s ip=%s msg=%q",
				time.Now().Format(time.RFC3339Nano), c.Name(), c.PlayerID(), c.UUID(), c.RemoteAddr().String(), strings.TrimSpace(msg)))
		}
		trimmed := strings.TrimSpace(msg)
		switch trimmed {
		case "/help":
			sendChatHelp(srv, c, cfg)
			return true
		case "/votemap":
			if c == nil {
				return true
			}
			showMapVoteMenu(srv, c, 0)
			return true
		case "/vote":
			if c == nil {
				return true
			}
			showActiveMapVoteMenu(srv, c)
			return true
		case "/status":
			srv.SendStatusTo(c)
			return true
		case "/sync":
			if c == nil {
				return true
			}
			if err := srv.SyncWorldToConn(c); err != nil {
				srv.SendChat(c, fmt.Sprintf("[scarlet]同步失败: %s[]", err.Error()))
				return true
			}
			srv.SendChat(c, "[accent]正在重新加载地图并同步当前运行状态[]")
			return true
		}
		lowerTrimmed := strings.ToLower(trimmed)
		if strings.HasPrefix(lowerTrimmed, "/votemap ") {
			if c == nil {
				return true
			}
			startMapVote(srv, c, strings.TrimSpace(trimmed[len("/votemap "):]))
			return true
		}
		if strings.HasPrefix(lowerTrimmed, "/vote ") {
			if c == nil {
				return true
			}
			args := strings.Fields(lowerTrimmed)
			if len(args) < 2 {
				showActiveMapVoteMenu(srv, c)
				return true
			}
			switch args[1] {
			case "yes", "y", "1", "同意":
				castMapVote(srv, c, 1)
			case "no", "n", "0", "反对":
				castMapVote(srv, c, -1)
			case "neutral", "mid", "中立", "abstain":
				castMapVote(srv, c, 0)
			default:
				srv.SendChat(c, "[scarlet]用法: /vote yes|no|neutral[]")
			}
			return true
		}
		if strings.EqualFold(trimmed, "/stop") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			saveState()
			saveOps()
			srv.BroadcastChat("[accent]服务器正在保存并关闭...")
			go func() {
				time.Sleep(200 * time.Millisecond)
				_ = recorder.Close()
				os.Exit(0)
			}()
			return true
		}
		if strings.HasPrefix(trimmed, "/summon ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 2 {
				srv.SendChat(c, "[scarlet]用法: /summon <typeId|unitName> [x y] [count] [team] []")
				return true
			}
			typeID, typeName, ok := resolveUnitTypeArg(args[1], wld)
			if !ok {
				srv.SendChat(c, "[scarlet]typeId/unitName 无效[]")
				return true
			}
			px, py := c.SnapshotPos()
			x := float64(px)
			y := float64(py)
			team := world.TeamID(1)
			count := 1
			next := 2
			if len(args) >= 4 {
				if xv, err := strconv.ParseFloat(args[2], 32); err == nil {
					if yv, err2 := strconv.ParseFloat(args[3], 32); err2 == nil {
						x = xv
						y = yv
						next = 4
					}
				}
			}
			if len(args) > next {
				if n, err := strconv.ParseInt(args[next], 10, 32); err == nil {
					count = int(n)
					next++
				}
			}
			if len(args) > next {
				if t, err := strconv.ParseInt(args[next], 10, 8); err == nil {
					team = world.TeamID(t)
				}
			}
			if count < 1 {
				count = 1
			}
			if count > 500 {
				count = 500
			}
			success := 0
			var firstID int32
			for i := 0; i < count; i++ {
				sx := float32(x)
				sy := float32(y)
				if i > 0 {
					ring := float32((i-1)/12+1) * 12
					ang := float64(i) * 2 * math.Pi / 12
					sx += float32(math.Cos(ang)) * ring
					sy += float32(math.Sin(ang)) * ring
				}
				ent, err := wld.AddEntity(typeID, sx, sy, team)
				if err != nil {
					continue
				}
				if success == 0 {
					firstID = ent.ID
				}
				success++
			}
			if success == 0 {
				srv.SendChat(c, "[scarlet]召唤失败[]")
				return true
			}
			broadcastSummonVisible(srv, typeID, float32(x), float32(y), byte(team))
			saveState()
			srv.BroadcastChat(fmt.Sprintf("[accent]OP召唤单位[] firstId=%d count=%d type=%d(%s) x=%.1f y=%.1f team=%d", firstID, success, typeID, typeName, x, y, team))
			return true
		}
		if strings.HasPrefix(trimmed, "/despawn ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 2 {
				srv.SendChat(c, "[scarlet]用法: /despawn <entityId>[]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]entityId 无效[]")
				return true
			}
			if _, ok := wld.RemoveEntity(int32(id)); !ok {
				srv.SendChat(c, "[scarlet]entityId 不存在[]")
				return true
			}
			saveState()
			srv.BroadcastChat(fmt.Sprintf("[accent]OP移除单位[] id=%d", id))
			return true
		}
		if strings.EqualFold(trimmed, "/kill") {
			if c == nil {
				return true
			}
			if !srv.KillSelfUnit(c) {
				srv.SendChat(c, "[scarlet]当前没有可处理的单位[]")
				return true
			}
			srv.SendChat(c, "[accent]已执行 /kill：当前单位已清除[]")
			return true
		}
		if strings.HasPrefix(trimmed, "/umove ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 4 {
				srv.SendChat(c, "[scarlet]用法: /umove <entityId> <vx> <vy> [rotVel][]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]entityId 无效[]")
				return true
			}
			vx, err := strconv.ParseFloat(args[2], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]vx 无效[]")
				return true
			}
			vy, err := strconv.ParseFloat(args[3], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]vy 无效[]")
				return true
			}
			rotVel := float32(0)
			if len(args) >= 5 {
				if rv, rerr := strconv.ParseFloat(args[4], 32); rerr == nil {
					rotVel = float32(rv)
				}
			}
			if _, ok := wld.SetEntityMotion(int32(id), float32(vx), float32(vy), rotVel); !ok {
				srv.SendChat(c, "[scarlet]entityId 不存在[]")
				return true
			}
			saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位运动已设置[] id=%d vx=%.2f vy=%.2f rv=%.2f", id, vx, vy, rotVel))
			return true
		}
		if strings.HasPrefix(trimmed, "/uteleport ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 4 {
				srv.SendChat(c, "[scarlet]用法: /uteleport <entityId> <x> <y> [rotation][]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]entityId 无效[]")
				return true
			}
			x, err := strconv.ParseFloat(args[2], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]x 无效[]")
				return true
			}
			y, err := strconv.ParseFloat(args[3], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]y 无效[]")
				return true
			}
			rot := float32(0)
			if len(args) >= 5 {
				if rv, rerr := strconv.ParseFloat(args[4], 32); rerr == nil {
					rot = float32(rv)
				}
			}
			if _, ok := wld.SetEntityPosition(int32(id), float32(x), float32(y), rot); !ok {
				srv.SendChat(c, "[scarlet]entityId 不存在[]")
				return true
			}
			saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位传送完成[] id=%d x=%.1f y=%.1f rot=%.1f", id, x, y, rot))
			return true
		}
		if strings.HasPrefix(trimmed, "/ulife ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 3 {
				srv.SendChat(c, "[scarlet]用法: /ulife <entityId> <seconds(<=0表示无限)>[]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]entityId 无效[]")
				return true
			}
			life, err := strconv.ParseFloat(args[2], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]seconds 无效[]")
				return true
			}
			if _, ok := wld.SetEntityLife(int32(id), float32(life)); !ok {
				srv.SendChat(c, "[scarlet]entityId 不存在[]")
				return true
			}
			saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位寿命已设置[] id=%d life=%.2fs", id, life))
			return true
		}
		if strings.HasPrefix(trimmed, "/ufollow ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 3 {
				srv.SendChat(c, "[scarlet]用法: /ufollow <id> <targetId> [speed][]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]id 无效[]")
				return true
			}
			targetID, err := strconv.ParseInt(args[2], 10, 32)
			if err != nil || targetID <= 0 {
				srv.SendChat(c, "[scarlet]targetId 无效[]")
				return true
			}
			speed := float32(0)
			if len(args) >= 4 {
				if sp, serr := strconv.ParseFloat(args[3], 32); serr == nil {
					speed = float32(sp)
				}
			}
			if _, ok := wld.SetEntityFollow(int32(id), int32(targetID), speed); !ok {
				srv.SendChat(c, "[scarlet]id 不存在[]")
				return true
			}
			saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位跟随已设置[] id=%d -> target=%d speed=%.2f", id, targetID, speed))
			return true
		}
		if strings.HasPrefix(trimmed, "/upatrol ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 6 {
				srv.SendChat(c, "[scarlet]用法: /upatrol <id> <x1> <y1> <x2> <y2> [speed][]")
				return true
			}
			id, err := strconv.ParseInt(args[1], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]id 无效[]")
				return true
			}
			x1, err := strconv.ParseFloat(args[2], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]x1 无效[]")
				return true
			}
			y1, err := strconv.ParseFloat(args[3], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]y1 无效[]")
				return true
			}
			x2, err := strconv.ParseFloat(args[4], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]x2 无效[]")
				return true
			}
			y2, err := strconv.ParseFloat(args[5], 32)
			if err != nil {
				srv.SendChat(c, "[scarlet]y2 无效[]")
				return true
			}
			speed := float32(0)
			if len(args) >= 7 {
				if sp, serr := strconv.ParseFloat(args[6], 32); serr == nil {
					speed = float32(sp)
				}
			}
			if _, ok := wld.SetEntityPatrol(int32(id), float32(x1), float32(y1), float32(x2), float32(y2), speed); !ok {
				srv.SendChat(c, "[scarlet]id 不存在[]")
				return true
			}
			saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位巡逻已设置[] id=%d A(%.1f,%.1f) B(%.1f,%.1f) speed=%.2f", id, x1, y1, x2, y2, speed))
			return true
		}
		if strings.HasPrefix(trimmed, "/ubehavior ") {
			if c == nil || !srv.IsOp(c.UUID()) {
				srv.SendChat(c, "[scarlet]没有权限（需要OP）[]")
				return true
			}
			args := strings.Fields(strings.TrimSpace(msg))
			if len(args) < 3 {
				srv.SendChat(c, "[scarlet]用法: /ubehavior clear <id>[]")
				return true
			}
			action := strings.ToLower(args[1])
			if action != "clear" {
				srv.SendChat(c, "[scarlet]仅支持: clear[]")
				return true
			}
			id, err := strconv.ParseInt(args[2], 10, 32)
			if err != nil || id <= 0 {
				srv.SendChat(c, "[scarlet]id 无效[]")
				return true
			}
			if _, ok := wld.ClearEntityBehavior(int32(id)); !ok {
				srv.SendChat(c, "[scarlet]id 不存在[]")
				return true
			}
			saveState()
			srv.SendChat(c, fmt.Sprintf("[accent]单位行为已清除[] id=%d", id))
			return true
		}
		if strings.HasPrefix(trimmed, "/") {
			srv.SendChat(c, fmt.Sprintf("[scarlet]无效命令: %s[]", trimmed))
			return true
		}
		return false
	}

	var engine *sim.Engine
	var serverCore *coreio.ServerCore
	if cfg.Core.DualCoreEnabled {
		ioWorkers := cfg.Runtime.Cores / 2
		if ioWorkers < 2 {
			ioWorkers = 2
		}
		serverCore = coreio.NewServerCore(
			time.Second/time.Duration(gameTPS),
			coreio.Config{
				Name:          "io-core",
				MessageBuf:    30000,
				WorkerCount:   ioWorkers,
				VerboseNetLog: false,
			},
			cfg.Persist,
		)
		if exePath, err := os.Executable(); err == nil {
			if err := serverCore.EnableChildRoles(exePath, []string{"--config=" + cfg.Source}, "core2", "core3", "core4"); err != nil {
				startup.warn("子核心进程", fmt.Sprintf("启动失败，回退到进程内核心: %v", err))
			} else {
				startup.ok("子核心进程", "core2/core3/core4 IPC 已连接")
			}
		} else {
			startup.warn("子核心进程", fmt.Sprintf("无法定位可执行文件，回退到进程内核心: %v", err))
		}
		cache.backend = serverCore.Core3
		serverCore.Core2.SetVerboseNetLog(false)
		serverCore.Core2.SetRecorder(recorder)
		serverCore.SetPersistStateProvider(func() persist.State {
			snap := wld.Snapshot()
			return persist.State{
				MapPath:  state.get(),
				WaveTime: snap.WaveTime,
				Wave:     snap.Wave,
				Tick:     snap.Tick,
				TimeData: snap.TimeData,
				Rand0:    snap.Rand0,
				Rand1:    snap.Rand1,
			}
		})
		serverCore.SetGameTickFn(func(_ uint64, delta time.Duration) {
			wld.Step(delta)
			unitCommands.step(wld)
			traceWorldRuntimeTick("game_tick")
		})

		netCore := netserver.NewNetworkCoreWithCore(srv, serverCore.Core2)
		netCore.SetServerCore(serverCore)
		netCore.SetRecorder(recorder)
		baseConnOpen := func(c *netserver.Conn) {
			netCore.ConnectionOpen(c)
		}
		baseConnClose := func(c *netserver.Conn) {
			if c != nil {
				if publicConnUUIDStore != nil && runtimePublicConnUUIDEnabled.Load() {
					host := ""
					if c.RemoteAddr() != nil {
						if h, _, err := net.SplitHostPort(c.RemoteAddr().String()); err == nil {
							host = h
						} else {
							host = c.RemoteAddr().String()
						}
					}
					_ = publicConnUUIDStore.ObserveDisconnect(c.UUID(), c.Name(), host)
				}
				buildService.ClearOwner(resolveBuildOwner(c))
				wld.ClearBuilderState(resolveBuildOwner(c))
				if unitID := c.UnitID(); unitID != 0 {
					wld.RemoveEntity(unitID)
				}
				wld.CancelBuildPlansByOwner(resolveBuildOwner(c))
			}
			netCore.ConnectionClose(c)
		}
		srv.OnConnOpen = baseConnOpen
		srv.OnConnClose = baseConnClose
		basePacketDecoded := func(c *netserver.Conn, obj any, err error) bool {
			if err != nil {
				// Let server.handleConn run its normal close/error path to avoid duplicate close events.
				return false
			}
			netCore.ProcessPacket(c, obj, nil)
			return true
		}
		srv.OnPacketDecoded = basePacketDecoded

		if serverCore.Core4 != nil {
			srv.OnConnOpen = func(c *netserver.Conn) {
				baseConnOpen(c)
				if c != nil {
					serverCore.Core4.RecordConnectionOpen(c.ConnID(), connRemoteIP(c), c.UUID())
				}
			}
			srv.OnConnClose = func(c *netserver.Conn) {
				if c != nil {
					serverCore.Core4.RecordConnectionClose(c.ConnID())
				}
				baseConnClose(c)
			}
			srv.OnPacketDecoded = func(c *netserver.Conn, obj any, err error) bool {
				if err != nil {
					return basePacketDecoded(c, obj, err)
				}
				if serverCore.Core4 != nil && c != nil {
					if pkt, ok := obj.(*protocol.ConnectPacket); ok {
						if res, perr := serverCore.Core4.AllowConnection(connRemoteIP(c), pkt.UUID); perr == nil && !res.Allowed {
							_ = c.Close()
							return true
						}
						if connUUID := pkt.UUID; strings.TrimSpace(connUUID) != "" {
							_, _ = serverCore.Core4.PlayerShard(connUUID, connRemoteIP(c))
						}
					}
					if res, perr := serverCore.Core4.AllowPacket(connRemoteIP(c), c.ConnID(), c.UUID(), fmt.Sprintf("%T", obj)); perr == nil && !res.Allowed {
						_ = c.Close()
						return true
					}
				}
				return basePacketDecoded(c, obj, err)
			}
		}
		serverCore.StartAll()
	} else {
		// Single-core mode: keep server's own packet loop; only add disconnect cleanup and a simple tick loop.
		srv.OnConnClose = func(c *netserver.Conn) {
			if c == nil {
				return
			}
			if publicConnUUIDStore != nil && runtimePublicConnUUIDEnabled.Load() {
				host := ""
				if c.RemoteAddr() != nil {
					if h, _, err := net.SplitHostPort(c.RemoteAddr().String()); err == nil {
						host = h
					} else {
						host = c.RemoteAddr().String()
					}
				}
				_ = publicConnUUIDStore.ObserveDisconnect(c.UUID(), c.Name(), host)
			}
			buildService.ClearOwner(resolveBuildOwner(c))
			wld.ClearBuilderState(resolveBuildOwner(c))
			if unitID := c.UnitID(); unitID != 0 {
				wld.RemoveEntity(unitID)
			}
			wld.CancelBuildPlansByOwner(resolveBuildOwner(c))
		}
		go func() {
			interval := time.Second / time.Duration(gameTPS)
			next := time.Now().Add(interval)
			const maxCatchUp = 4
			for {
				now := time.Now()
				if now.Before(next) {
					time.Sleep(next.Sub(now))
					continue
				}
				steps := 0
				for !now.Before(next) && steps < maxCatchUp {
					wld.Step(interval)
					unitCommands.step(wld)
					traceWorldRuntimeTick("game_tick")
					steps++
					next = next.Add(interval)
					now = time.Now()
				}
				if steps == maxCatchUp && !now.Before(next) {
					next = now.Add(interval)
				}
			}
		}()
	}
	monitor := newStatusMonitor(srv, cfg, engine)
	saveState = func() {}
	if cfg.Persist.Enabled {
		saveState = func() {
			snap := wld.Snapshot()
			stateData := persist.State{
				MapPath:  state.get(),
				WaveTime: snap.WaveTime,
				Wave:     snap.Wave,
				Tick:     snap.Tick,
				TimeData: snap.TimeData,
				Rand0:    snap.Rand0,
				Rand1:    snap.Rand1,
			}
			savedState := false
			if serverCore != nil {
				ch := make(chan coreio.PersistenceResult, 1)
				if !serverCore.SendToCore2(&coreio.PersistenceMessage{
					Action:     "save_state",
					Path:       state.get(),
					ResultChan: ch,
				}) {
					fmt.Println("persist save_state skipped: core2 queue unavailable, using direct save")
				} else {
					select {
					case res := <-ch:
						if res.Error != nil {
							fmt.Printf("persist save_state failed: %v\n", res.Error)
						} else {
							savedState = true
						}
					case <-time.After(2 * time.Second):
						fmt.Println("persist save_state timeout: using direct save")
					}
				}
			}
			if !savedState {
				_ = persist.Save(cfg.Persist, stateData)
			}
			_ = persist.SaveMSAVSnapshotFromModel(cfg.Persist, snap, wld.Model(), state.get())
		}
		interval := time.Duration(cfg.Persist.IntervalSec) * time.Second
		if interval <= 0 {
			interval = 30 * time.Second
		}
		go func() {
			t := time.NewTicker(interval)
			defer t.Stop()
			for range t.C {
				saveState()
			}
		}()
	}
	var videoRecorder *video.Recorder
	if *recordVideo {
		rec, err := video.Start(video.Config{
			Enabled:   true,
			OutputDir: *videoDir,
			FPS:       *videoFPS,
			Width:     *videoWidth,
			Height:    *videoHeight,
			TileSize:  *videoTileSize,
		}, wld.CloneModel, wld.Snapshot, state.get, func() []video.PlayerState {
			snaps := srv.ListPlayerSnapshots()
			out := make([]video.PlayerState, 0, len(snaps))
			for _, snap := range snaps {
				out = append(out, video.PlayerState{
					Name:      snap.Name,
					UUID:      snap.UUID,
					UnitID:    snap.UnitID,
					TeamID:    snap.TeamID,
					X:         snap.X,
					Y:         snap.Y,
					Connected: snap.Connected,
					Dead:      snap.Dead,
				})
			}
			return out
		})
		if err != nil {
			startup.warn("视频录制", err.Error())
		} else {
			videoRecorder = rec
			startup.ok("视频录制", fmt.Sprintf("实时编码 dir=%s fps=%d size=%dx%d", rec.SessionDir(), *videoFPS, *videoWidth, *videoHeight))
		}
	} else {
		startup.info("视频录制", "未启用")
	}
	var (
		apiSrv      *api.Server
		apiListener net.Listener
	)
	if cfg.API.Enabled {
		ln, err := net.Listen("tcp", cfg.API.Bind)
		if err != nil {
			startup.fail("API", err.Error())
			if cfg.Personalization.StartupReportEnabled {
				startup.print()
			}
			fmt.Fprintf(os.Stderr, "API 启动前预检失败: %v\n", err)
			os.Exit(1)
		}
		apiListener = ln
		var statsFn func() *sim.TickStats
		if engine != nil {
			statsFn = func() *sim.TickStats {
				st := engine.Stats()
				return &st
			}
		}
		apiSrv = api.New(cfg.API, srv, statsFn)
		go func() {
			if err := apiSrv.ServeListener(apiListener); err != nil {
				log.Error("api serve failed", logging.Field{Key: "error", Value: err.Error()})
			}
		}()
		startup.ok("API", fmt.Sprintf("bind=%s auth=%v", cfg.API.Bind, len(cfg.API.Keys) > 0))
	} else {
		startup.info("API", "未启用")
	}
	var stopOnce sync.Once
	stopServer := func(reason string) {
		stopOnce.Do(func() {
			const shutdownForceTimeout = 6 * time.Second
			forceExit := time.AfterFunc(shutdownForceTimeout, func() {
				fmt.Println("关闭超时，强制退出服务器")
				os.Exit(0)
			})
			defer forceExit.Stop()

			if reason != "" {
				fmt.Println(reason)
			}
			if serverCore != nil && serverCore.Core1 != nil {
				serverCore.Core1.Stop()
			}
			const shutdownKickReason = "服务器正在关闭"
			const shutdownKickDelay = 400 * time.Millisecond
			notifyDone := make(chan int, 1)
			go func() {
				notifyDone <- srv.NotifyShutdown(shutdownKickReason, shutdownKickDelay)
			}()
			select {
			case notified := <-notifyDone:
				if notified > 0 {
					time.Sleep(shutdownKickDelay + 100*time.Millisecond)
				}
			case <-time.After(1200 * time.Millisecond):
				fmt.Println("玩家关闭通知超时，继续强制关闭监听")
			}
			srv.Shutdown()
			if videoRecorder != nil {
				if err := videoRecorder.Close(); err != nil {
					fmt.Printf("[video] finalize failed: %v\n", err)
				}
				videoPath := videoRecorder.VideoPath()
				if _, err := os.Stat(videoPath); err == nil {
					fmt.Printf("[video] saved match video: %s (frames=%d dropped=%d)\n", videoPath, videoRecorder.FrameCount(), videoRecorder.DroppedCount())
				} else {
					fmt.Printf("[video] recording session kept at: %s (frames=%d dropped=%d)\n", videoRecorder.SessionDir(), videoRecorder.FrameCount(), videoRecorder.DroppedCount())
				}
			}
			saveState()
			saveOps()
			if serverCore != nil {
				serverCore.StopAll()
			}
			_ = traceLog.Close()
			_ = detailLog.Close()
			_ = recorder.Close()
			os.Exit(0)
		})
	}
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(signalCh)
	go func() {
		sig, ok := <-signalCh
		if !ok {
			return
		}
		stopServer(fmt.Sprintf("收到信号 %s，正在保存并关闭服务器", sig))
	}()
	if apiSrv != nil {
		apiSrv.SetSummonFunc(func(typeID int16, x, y float32, team byte) error {
			ent, err := wld.AddEntity(typeID, x, y, world.TeamID(team))
			if err != nil {
				return err
			}
			broadcastSummonVisible(srv, typeID, x, y, team)
			saveState()
			srv.BroadcastChat(fmt.Sprintf("[accent]API召唤单位[] id=%d type=%d x=%.1f y=%.1f team=%d", ent.ID, typeID, x, y, team))
			return nil
		})
		apiSrv.SetStopFunc(func() {
			stopServer("API 请求关闭服务器")
		})
		apiSrv.SetUnitMoveFunc(func(id int32, vx, vy, rotVel float32) error {
			if _, ok := wld.SetEntityMotion(id, vx, vy, rotVel); !ok {
				return errors.New("entity not found")
			}
			saveState()
			return nil
		})
		apiSrv.SetUnitTeleportFunc(func(id int32, x, y, rotation float32) error {
			if _, ok := wld.SetEntityPosition(id, x, y, rotation); !ok {
				return errors.New("entity not found")
			}
			saveState()
			return nil
		})
		apiSrv.SetUnitLifeFunc(func(id int32, lifeSec float32) error {
			if _, ok := wld.SetEntityLife(id, lifeSec); !ok {
				return errors.New("entity not found")
			}
			saveState()
			return nil
		})
		apiSrv.SetUnitFollowFunc(func(id int32, targetID int32, speed float32) error {
			if _, ok := wld.SetEntityFollow(id, targetID, speed); !ok {
				return errors.New("entity not found")
			}
			saveState()
			return nil
		})
		apiSrv.SetUnitPatrolFunc(func(id int32, x1, y1, x2, y2, speed float32) error {
			if _, ok := wld.SetEntityPatrol(id, x1, y1, x2, y2, speed); !ok {
				return errors.New("entity not found")
			}
			saveState()
			return nil
		})
		apiSrv.SetUnitBehaviorFunc(func(id int32, mode string) error {
			switch strings.ToLower(strings.TrimSpace(mode)) {
			case "", "clear", "none", "stop":
				if _, ok := wld.ClearEntityBehavior(id); !ok {
					return errors.New("entity not found")
				}
				saveState()
				return nil
			default:
				return errors.New("unsupported behavior mode")
			}
		})
		apiSrv.SetOpsChangedFunc(func() {
			saveOps()
		})
	}
	go func() {
		interval := time.Duration(cfg.Control.ReloadIntervalSec) * time.Second
		if interval <= 0 {
			interval = 5 * time.Second
		}
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			loaded, loadErr := config.Load(cfg.Source)
			if loadErr != nil {
				if cfg.Control.ReloadLogEnabled {
					fmt.Printf("[config] reload failed: %v\n", loadErr)
				}
				continue
			}
			loaded.Source = cfg.Source
			config.ApplyBaseDir(&loaded, rootDir)
			keys, keyErr := mergeValidAPIKeys(loaded.API.Keys, loaded.API.Key)
			if keyErr != nil {
				if cfg.Control.ReloadLogEnabled {
					fmt.Printf("[config] reload failed: %v\n", keyErr)
				}
				continue
			}
			loaded.API.Keys = keys
			loaded.API.Key = ""

			srv.SetServerName(loaded.Runtime.ServerName)
			srv.SetServerDescription(loaded.Runtime.ServerDesc)
			srv.SetVirtualPlayers(int32(loaded.Runtime.VirtualPlayers))
			srv.UdpRetryCount = loaded.Net.UdpRetryCount
			srv.UdpRetryDelay = time.Duration(loaded.Net.UdpRetryDelayMs) * time.Millisecond
			srv.UdpFallbackTCP = loaded.Net.UdpFallbackTCP
			srv.SetSnapshotIntervals(loaded.Net.SyncEntityMs, loaded.Net.SyncStateMs)
			runtimePlayerNameColorEnabled.Store(loaded.Personalization.PlayerNameColorEnabled)
			srv.SetPacketRecvEventsEnabled(loaded.Development.PacketRecvEventsEnabled)
			srv.SetPacketSendEventsEnabled(loaded.Development.PacketSendEventsEnabled)
			srv.SetTerminalPlayerLogsEnabled(loaded.Development.TerminalPlayerLogsEnabled)
			srv.SetTerminalPlayerUUIDEnabled(loaded.Development.TerminalPlayerUUIDEnabled)
			srv.SetRespawnPacketLogsEnabled(loaded.Development.RespawnPacketLogsEnabled)
			srv.SetPlayerNameColorEnabled(loaded.Personalization.PlayerNameColorEnabled)
			srv.SetTranslatedConnLog(loaded.Control.TranslatedConnLogEnabled)
			srv.SetJoinLeaveChatEnabled(loaded.Personalization.JoinLeaveChatEnabled)
			runtimeJoinLeaveChatEnabled.Store(loaded.Personalization.JoinLeaveChatEnabled)
			runtimeTraceCfg.Store(loaded.Tracepoints)
			runtimePlayerNamePrefix.Store(loaded.Personalization.PlayerNamePrefix)
			runtimePlayerNameSuffix.Store(loaded.Personalization.PlayerNameSuffix)
			runtimePlayerBindPrefixEnabled.Store(loaded.Personalization.PlayerBindPrefixEnabled)
			runtimePlayerBoundPrefix.Store(loaded.Personalization.PlayerBoundPrefix)
			runtimePlayerUnboundPrefix.Store(loaded.Personalization.PlayerUnboundPrefix)
			runtimePlayerTitleEnabled.Store(loaded.Personalization.PlayerTitleEnabled)
			runtimePlayerConnIDSuffixEnabled.Store(loaded.Personalization.PlayerConnIDSuffixEnabled)
			runtimePlayerConnIDSuffixFormat.Store(loaded.Personalization.PlayerConnIDSuffixFormat)
			initStatusBarRuntime(loaded)
			initJoinPopupRuntime(loaded)
			initMapVoteRuntimeConfig(loaded)
			runtimeBindStatusResolver = newBindStatusResolver(
				loaded.Personalization.PlayerBindSource,
				loaded.Personalization.PlayerBindAPIURL,
				time.Duration(loaded.Personalization.PlayerBindAPITimeoutMs)*time.Millisecond,
				time.Duration(loaded.Personalization.PlayerBindAPICacheSec)*time.Second,
				runtimePlayerIdentityStore,
			)
			srv.RefreshPlayerDisplayNames()
			runtimePublicConnUUIDEnabled.Store(loaded.Control.PublicConnUUIDEnabled)
			applyBlockNameTranslations(configDir)
			if serverCore != nil && serverCore.Core2 != nil {
				serverCore.Core2.SetVerboseNetLog(false)
			}
			if loaded.Control.PublicConnUUIDFile != cfg.Control.PublicConnUUIDFile && cfg.Control.ReloadLogEnabled {
				fmt.Printf("[config] public_conn_uuid_file changed to %s, restart required to reopen mapping file\n", loaded.Control.PublicConnUUIDFile)
			}
			if loaded.Control.ConnUUIDAutoCreateEnabled != cfg.Control.ConnUUIDAutoCreateEnabled && cfg.Control.ReloadLogEnabled {
				fmt.Printf("[config] conn_uuid_auto_create changed to %t, restart required to reopen mapping policy\n", loaded.Control.ConnUUIDAutoCreateEnabled)
			}
			if loaded.Control.PlayerIdentityAutoCreateEnabled != cfg.Control.PlayerIdentityAutoCreateEnabled && cfg.Control.ReloadLogEnabled {
				fmt.Printf("[config] player_identity_auto_create changed to %t, restart required to reopen identity file policy\n", loaded.Control.PlayerIdentityAutoCreateEnabled)
			}
			if loaded.Personalization.PlayerIdentityFile != cfg.Personalization.PlayerIdentityFile && cfg.Control.ReloadLogEnabled {
				fmt.Printf("[config] player_identity_file changed to %s, restart required to reopen identity file\n", loaded.Personalization.PlayerIdentityFile)
			}
			if loaded.Tracepoints.File != cfg.Tracepoints.File && cfg.Control.ReloadLogEnabled {
				fmt.Printf("[config] tracepoints file changed to %s, restart required to reopen trace log file\n", loaded.Tracepoints.File)
			}

			if apiSrv != nil {
				applyAPIKeySet(apiSrv, loaded.API.Keys)
			}
			if loaded.API.Enabled != cfg.API.Enabled || loaded.API.Bind != cfg.API.Bind {
				if cfg.Control.ReloadLogEnabled {
					fmt.Printf("[config] api enabled/bind changed (enabled=%v bind=%s), restart required to apply\n", loaded.API.Enabled, loaded.API.Bind)
				}
			}
			if err := applyAdmissionPolicy(loaded); err != nil {
				if cfg.Control.ReloadLogEnabled {
					fmt.Printf("[config] reload failed: %v\n", err)
				}
				continue
			}

			cfg = loaded
			next := time.Duration(cfg.Control.ReloadIntervalSec) * time.Second
			if next <= 0 {
				next = 5 * time.Second
			}
			ticker.Reset(next)
			if cfg.Control.ReloadLogEnabled {
				fmt.Printf("[config] reloaded: server=%q entity_ms=%d state_ms=%d keys=%d interval=%ds\n",
					cfg.Runtime.ServerName, cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs, len(cfg.API.Keys), cfg.Control.ReloadIntervalSec)
			}
		}
	}()
	removeEntityByID := func(id int32) bool {
		_, ok := wld.RemoveEntity(id)
		return ok
	}
	setEntityMotion := func(id int32, vx, vy, rotVel float32) bool {
		_, ok := wld.SetEntityMotion(id, vx, vy, rotVel)
		return ok
	}
	setEntityPos := func(id int32, x, y, rot float32) bool {
		_, ok := wld.SetEntityPosition(id, x, y, rot)
		return ok
	}
	setEntityLife := func(id int32, life float32) bool {
		_, ok := wld.SetEntityLife(id, life)
		return ok
	}
	setEntityFollow := func(id, targetID int32, speed float32) bool {
		_, ok := wld.SetEntityFollow(id, targetID, speed)
		return ok
	}
	setEntityPatrol := func(id int32, x1, y1, x2, y2, speed float32) bool {
		_, ok := wld.SetEntityPatrol(id, x1, y1, x2, y2, speed)
		return ok
	}
	clearEntityBehavior := func(id int32) bool {
		_, ok := wld.ClearEntityBehavior(id)
		return ok
	}
	reloadVanillaProfiles := func(path string) error {
		return wld.LoadVanillaProfiles(path)
	}
	reloadVanillaContentIDs := func(path string) error {
		ids, err := vanilla.LoadContentIDs(path)
		if err != nil {
			return err
		}
		setEffectIDs(ids)
		_ = vanilla.ApplyContentIDs(srv.Content, ids)
		return nil
	}
	startup.ok("服务端启动", "初始化完成")
	if cfg.Personalization.StartupReportEnabled {
		startup.print()
	}
	if cfg.Personalization.MapLoadDetailsEnabled && loadedModel != nil {
		printMapDetails(loadedMapPath, loadedModel)
	}
	if cfg.Personalization.UnitIDListEnabled {
		printUnitIDList(unitNamesByID)
	}
	closeImmediate := func() {
		_ = traceLog.Close()
		_ = detailLog.Close()
		_ = recorder.Close()
	}
	go runConsole(srv, state, modMgr, apiSrv, scriptCtl, *addr, *buildVersion, &cfg, saveConfig, saveScript, recorder, monitor, saveOps, loadWorldModel, invalidateWorldCache, reloadVanillaProfiles, reloadVanillaContentIDs, removeEntityByID, setEntityMotion, setEntityPos, setEntityLife, setEntityFollow, setEntityPatrol, clearEntityBehavior, stopServer, closeImmediate)
	if serverCore != nil {
		go func() {
			if err := srv.Serve(); err != nil {
				fmt.Fprintf(os.Stderr, "服务器启动失败: %v\n", err)
				os.Exit(1)
			}
		}()
		serverCore.Core1.Run(time.Second / time.Duration(gameTPS))
	} else {
		if err := srv.Serve(); err != nil {
			fmt.Fprintf(os.Stderr, "服务器启动失败: %v\n", err)
			os.Exit(1)
		}
	}
}

func startMemoryGuard(cfg config.CoreConfig) {
	mb := func(n int) int64 { return int64(n) * 1024 * 1024 }
	if cfg.MemoryLimitMB > 0 {
		debug.SetMemoryLimit(mb(cfg.MemoryLimitMB))
		fmt.Printf("[memory] set memory limit: %dMB\n", cfg.MemoryLimitMB)
	}

	readHeapAlloc := func() uint64 {
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		return ms.HeapAlloc
	}

	if cfg.MemoryStartupMaxMB > 0 {
		maxB := uint64(mb(cfg.MemoryStartupMaxMB))
		before := readHeapAlloc()
		if before > maxB {
			runtime.GC()
			if cfg.MemoryFreeOSMemory {
				debug.FreeOSMemory()
			}
			after := readHeapAlloc()
			fmt.Printf("[memory] startup heap_alloc too high: before=%dMB after=%dMB max=%dMB\n",
				before/1024/1024, after/1024/1024, cfg.MemoryStartupMaxMB)
		}
	}

	if cfg.MemoryGCTriggerMB <= 0 {
		return
	}
	interval := time.Duration(cfg.MemoryCheckIntervalSec)
	if interval <= 0 {
		interval = 5
	}
	triggerB := uint64(mb(cfg.MemoryGCTriggerMB))
	go func() {
		t := time.NewTicker(interval * time.Second)
		defer t.Stop()
		for range t.C {
			before := readHeapAlloc()
			if before < triggerB {
				continue
			}
			runtime.GC()
			if cfg.MemoryFreeOSMemory {
				debug.FreeOSMemory()
			}
			after := readHeapAlloc()
			fmt.Printf("[memory] gc triggered: before=%dMB after=%dMB trigger=%dMB\n",
				before/1024/1024, after/1024/1024, cfg.MemoryGCTriggerMB)
		}
	}()
}

func (s *worldState) set(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = canonicalRuntimePath(path)
}

func (s *worldState) get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func connRemoteIP(c *netserver.Conn) string {
	if c == nil || c.RemoteAddr() == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(c.RemoteAddr().String())
	if err != nil {
		return c.RemoteAddr().String()
	}
	return host
}

func runConsole(
	srv *netserver.Server,
	state *worldState,
	modMgr interface{},
	apiSrv *api.Server,
	scriptCtl *scriptController,
	listenAddr string,
	build int,
	cfg *config.Config,
	saveConfig func() error,
	saveScript func() error,
	recorder storage.Recorder,
	monitor *statusMonitor,
	saveOps func(),
	loadWorldModel func(path string),
	invalidateWorldCache func(),
	reloadVanillaProfiles func(path string) error,
	reloadVanillaContentIDs func(path string) error,
	removeEntityByID func(id int32) bool,
	setEntityMotion func(id int32, vx, vy, rotVel float32) bool,
	setEntityPos func(id int32, x, y, rot float32) bool,
	setEntityLife func(id int32, life float32) bool,
	setEntityFollow func(id, targetID int32, speed float32) bool,
	setEntityPatrol func(id int32, x1, y1, x2, y2, speed float32) bool,
	clearEntityBehavior func(id int32) bool,
	stopServer func(reason string),
	closeImmediate func(),
) {
	sc := bufio.NewScanner(os.Stdin)
	name, _, _ := srv.ServerMeta()
	printConsoleIntro(name, state.get(), listenAddr, cfg.API.Bind, cfg.API.Enabled, cfg.Personalization)
	if cfg.Personalization.StartupHelpEnabled {
		printHelp(*cfg)
	}
	printBrandFooter()
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "#") {
			msg := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			if msg == "" {
				continue
			}
			srv.BroadcastChat(msg)
			fmt.Printf("已发送聊天: %q\n", msg)
			continue
		}
		parts := strings.Fields(line)
		cmd := strings.ToLower(parts[0])
		switch cmd {
		case "help", "?":
			if len(parts) == 1 {
				printHelp(*cfg)
				continue
			}
			cat := strings.ToLower(strings.Trim(parts[1], "`'\" "))
			printHelpCategory(*cfg, cat)
		case "maps":
			maps, err := listWorldMaps()
			if err != nil {
				fmt.Printf("地图列表错误: %v\n", err)
				continue
			}
			if len(maps) == 0 {
				fmt.Println("go-server/assets/worlds 下没有可用地图")
				continue
			}
			fmt.Printf("地图列表: %s\n", strings.Join(maps, ", "))
		case "mods":
			if modMgr == nil {
				fmt.Println("mod 管理器未初始化")
				continue
			}
			fmt.Println("模组系统暂未实现")
			continue

		case "mod":
			plugins, err := listScriptPlugins(cfg.Mods)
			if err != nil {
				fmt.Printf("插件列表错误: %v\n", err)
				continue
			}
			if len(plugins) == 0 {
				fmt.Println("当前无脚本插件")
				continue
			}
			for _, p := range plugins {
				fmt.Printf("plugin type=%s name=%s path=%s\n", p.Runtime, p.Name, p.Path)
			}
		case "world":
			fmt.Printf("当前地图: %s\n", canonicalRuntimePath(state.get()))
		case "host":
			if len(parts) < 2 {
				fmt.Println("用法: host random | host <地图名> | host <.msav 文件路径>")
				continue
			}
			next, err := resolveWorldSelection(parts[1])
			if err != nil {
				fmt.Printf("切图失败: %v\n", err)
				continue
			}
			state.set(next)
			loadWorldModel(next)
			if invalidateWorldCache != nil {
				invalidateWorldCache()
			}
			fmt.Printf("地图已切换: %s\n", next)
			reloaded, failed := srv.ReloadWorldLiveForAll()
			if reloaded == 0 && failed == 0 {
				fmt.Println("已应用新地图（当前无在线玩家）")
			} else {
				fmt.Printf("已应用新地图（原版式重载: 成功=%d 失败=%d，不踢出在线玩家）\n", reloaded, failed)
			}
		case "hotload":
			if len(parts) < 2 {
				fmt.Println("用法: hotload random | hotload <地图名> | hotload <.msav 文件路径>")
				continue
			}
			next, err := resolveWorldSelection(parts[1])
			if err != nil {
				fmt.Printf("热加载切图失败: %v\n", err)
				continue
			}
			state.set(next)
			loadWorldModel(next)
			if invalidateWorldCache != nil {
				invalidateWorldCache()
			}
			fmt.Printf("地图已热加载: %s\n", next)
			reloaded, failed := srv.ReloadWorldLiveForAll()
			if reloaded == 0 && failed == 0 {
				fmt.Println("已应用新地图（当前无在线玩家）")
			} else {
				fmt.Printf("已应用新地图（在线热更新: 成功=%d 失败=%d，不踢出在线玩家）\n", reloaded, failed)
			}
		case "stop":
			stopServer("正在保存并关闭服务器")
		case "exit":
			fmt.Println("直接退出服务器（不保存）")
			if closeImmediate != nil {
				closeImmediate()
			}
			os.Exit(0)
		case "quit":
			fmt.Println("直接退出服务器（不保存）")
			if closeImmediate != nil {
				closeImmediate()
			}
			os.Exit(0)
		case "ip":
			printIPs(listenAddr)
		case "server":
			if len(parts) == 1 || strings.EqualFold(parts[1], "status") {
				name, desc, fake := srv.ServerMeta()
				fmt.Printf("server: name=%q desc=%q virtual_players=%d\n", name, desc, fake)
				continue
			}
			switch strings.ToLower(parts[1]) {
			case "name":
				if len(parts) < 3 {
					fmt.Println("用法: server name <名称>")
					continue
				}
				name := strings.TrimSpace(strings.Join(parts[2:], " "))
				srv.SetServerName(name)
				cfg.Runtime.ServerName = name
				_ = saveConfig()
				fmt.Printf("已设置服务器名称: %s\n", name)
			case "desc":
				if len(parts) < 3 {
					fmt.Println("用法: server desc <简介>")
					continue
				}
				desc := strings.TrimSpace(strings.Join(parts[2:], " "))
				srv.SetServerDescription(desc)
				cfg.Runtime.ServerDesc = desc
				_ = saveConfig()
				fmt.Printf("已设置服务器简介: %s\n", desc)
			case "players":
				if len(parts) < 3 {
					fmt.Println("用法: server players <虚拟人数>")
					continue
				}
				n, err := strconv.Atoi(parts[2])
				if err != nil || n < 0 {
					fmt.Println("参数错误: 虚拟人数需要 >= 0")
					continue
				}
				srv.SetVirtualPlayers(int32(n))
				cfg.Runtime.VirtualPlayers = n
				_ = saveConfig()
				fmt.Printf("已设置虚拟人数: %d\n", n)
			default:
				fmt.Println("用法: server status | server name <名称> | server desc <简介> | server players <虚拟人数>")
			}
		case "selfcheck":
			printSelfCheck(listenAddr, build, state.get(), *cfg)
		case "apikey":
			printAPIKey(*cfg)
		case "storage", "data":
			if len(parts) == 1 || strings.EqualFold(parts[1], "status") {
				fmt.Printf("storage: file:%s (db_enabled=%v mode=%s dir=%s dsn=%q)\n", canonicalRuntimePath(cfg.Storage.Directory), cfg.Storage.DatabaseEnabled, cfg.Storage.Mode, canonicalRuntimePath(cfg.Storage.Directory), cfg.Storage.DSN)
				continue
			}
			if len(parts) >= 3 && strings.EqualFold(parts[1], "db") {
				switch strings.ToLower(parts[2]) {
				case "on":
					cfg.Storage.DatabaseEnabled = true
					_ = saveConfig()
					fmt.Println("已设置 database_enabled=true（已写入配置）")
				case "off":
					cfg.Storage.DatabaseEnabled = false
					_ = saveConfig()
					fmt.Println("已设置 database_enabled=false（已写入配置）")
				default:
					fmt.Println("用法: storage db on|off")
				}
				continue
			}
			if len(parts) >= 3 && strings.EqualFold(parts[1], "mode") {
				mode := strings.ToLower(parts[2])
				switch mode {
				case "file", "postgres", "mysql", "redis":
					cfg.Storage.Mode = mode
					_ = saveConfig()
					fmt.Printf("已设置 storage.mode=%s（已写入配置）\n", mode)
				default:
					fmt.Println("用法: storage mode file|postgres|mysql|redis")
				}
				continue
			}
			if len(parts) >= 3 && strings.EqualFold(parts[1], "dir") {
				cfg.Storage.Directory = strings.TrimSpace(strings.Join(parts[2:], " "))
				_ = saveConfig()
				fmt.Printf("已设置 storage.directory=%s（已写入配置）\n", cfg.Storage.Directory)
				continue
			}
			fmt.Println("用法: data status | data db on|off | data mode file|postgres|mysql|redis | data dir <path>")
		case "scheduler":
			if len(parts) == 1 || strings.EqualFold(parts[1], "status") {
				fmt.Printf("scheduler: enabled=%v cores=%d\n", cfg.Runtime.SchedulerEnabled, cfg.Runtime.Cores)
				continue
			}
			switch strings.ToLower(parts[1]) {
			case "on":
				cfg.Runtime.SchedulerEnabled = true
				_ = saveConfig()
				fmt.Println("scheduler 已启用（已写入配置）")
			case "off":
				cfg.Runtime.SchedulerEnabled = false
				_ = saveConfig()
				fmt.Println("scheduler 已关闭（已写入配置）")
			default:
				fmt.Println("用法: scheduler status | scheduler on | scheduler off")
			}
		case "sync":
			if len(parts) == 1 || strings.EqualFold(parts[1], "status") {
				entityMs, stateMs := srv.SnapshotIntervalsMs()
				fmt.Printf("sync: strategy=%s entity=%dms state=%dms (config entity=%dms state=%dms)\n", cfg.Sync.Strategy, entityMs, stateMs, cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs)
				continue
			}
			switch strings.ToLower(parts[1]) {
			case "strategy":
				if len(parts) < 3 {
					fmt.Println("用法: sync strategy official|static|dynamic")
					continue
				}
				strategy, ok := config.ParseAuthoritySyncStrategy(parts[2])
				if !ok {
					fmt.Println("参数错误: strategy 只能是 official|static|dynamic")
					continue
				}
				cfg.Sync.Strategy = strategy
				_ = saveConfig()
				fmt.Printf("已设置 sync.strategy=%s（已写入 sidecar 配置）\n", cfg.Sync.Strategy)
			case "entity":
				if len(parts) < 3 {
					fmt.Println("用法: sync entity <ms>")
					continue
				}
				ms, err := strconv.Atoi(parts[2])
				if err != nil {
					fmt.Printf("参数错误: %v\n", err)
					continue
				}
				cfg.Net.SyncEntityMs = ms
				srv.SetSnapshotIntervals(cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs)
				e, s := srv.SnapshotIntervalsMs()
				cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs = e, s
				_ = saveConfig()
				fmt.Printf("已设置 sync.entity=%dms（state=%dms，已写入配置）\n", e, s)
			case "state":
				if len(parts) < 3 {
					fmt.Println("用法: sync state <ms>")
					continue
				}
				ms, err := strconv.Atoi(parts[2])
				if err != nil {
					fmt.Printf("参数错误: %v\n", err)
					continue
				}
				cfg.Net.SyncStateMs = ms
				srv.SetSnapshotIntervals(cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs)
				e, s := srv.SnapshotIntervalsMs()
				cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs = e, s
				_ = saveConfig()
				fmt.Printf("已设置 sync.state=%dms（entity=%dms，已写入配置）\n", s, e)
			case "set":
				if len(parts) < 4 {
					fmt.Println("用法: sync set <entityMs> <stateMs>")
					continue
				}
				em, err1 := strconv.Atoi(parts[2])
				sm, err2 := strconv.Atoi(parts[3])
				if err1 != nil || err2 != nil {
					fmt.Println("参数错误: 需要数字毫秒")
					continue
				}
				srv.SetSnapshotIntervals(em, sm)
				e, s := srv.SnapshotIntervalsMs()
				cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs = e, s
				_ = saveConfig()
				fmt.Printf("已设置 sync: entity=%dms state=%dms（已写入配置）\n", e, s)
			case "default":
				srv.SetSnapshotIntervals(100, 250)
				e, s := srv.SnapshotIntervalsMs()
				cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs = e, s
				_ = saveConfig()
				fmt.Printf("已恢复默认 sync: entity=%dms state=%dms（已写入配置）\n", e, s)
			default:
				fmt.Println("用法: sync status | sync strategy official|static|dynamic | sync entity <ms> | sync state <ms> | sync set <entityMs> <stateMs> | sync default")
			}
		case "vanilla":
			if len(parts) == 1 || strings.EqualFold(parts[1], "status") {
				fmt.Printf("vanilla profiles: %s\n", canonicalRuntimePath(cfg.Runtime.VanillaProfiles))
				fmt.Printf("vanilla content ids: %s\n", canonicalRuntimePath(filepath.Join(filepath.Dir(cfg.Runtime.VanillaProfiles), "content_ids.json")))
				continue
			}
			sub := strings.ToLower(parts[1])
			switch sub {
			case "reload":
				path := cfg.Runtime.VanillaProfiles
				if len(parts) >= 3 {
					path = strings.TrimSpace(strings.Join(parts[2:], " "))
					cfg.Runtime.VanillaProfiles = path
					_ = saveConfig()
				}
				if err := reloadVanillaProfiles(path); err != nil {
					fmt.Printf("vanilla reload 失败: %v\n", err)
					continue
				}
				fmt.Printf("vanilla profiles 已加载: %s\n", canonicalRuntimePath(path))
			case "gen":
				out := cfg.Runtime.VanillaProfiles
				repoRoot := "."
				if len(parts) >= 3 {
					arg1 := strings.TrimSpace(parts[2])
					if strings.HasSuffix(strings.ToLower(arg1), ".json") {
						out = strings.TrimSpace(strings.Join(parts[2:], " "))
					} else {
						repoRoot = arg1
						if len(parts) >= 4 {
							out = strings.TrimSpace(strings.Join(parts[3:], " "))
						}
					}
					cfg.Runtime.VanillaProfiles = out
					_ = saveConfig()
				}
				units, turrets, blocks, err := vanilla.GenerateProfiles(repoRoot, out)
				if err != nil {
					fmt.Printf("vanilla gen 失败: %v\n", err)
					continue
				}
				if err := reloadVanillaProfiles(out); err != nil {
					fmt.Printf("profiles 生成成功但加载失败: %v\n", err)
					continue
				}
				fmt.Printf("vanilla profiles 生成并加载完成: units_by_name=%d turrets=%d blocks=%d path=%s\n", units, turrets, blocks, canonicalRuntimePath(out))
			case "ids":
				if len(parts) < 3 {
					fmt.Println("用法: vanilla ids gen [repoRoot] [outPath] | vanilla ids reload [path]")
					continue
				}
				sub2 := strings.ToLower(parts[2])
				switch sub2 {
				case "gen":
					repo := "."
					out := filepath.Join(filepath.Dir(cfg.Runtime.VanillaProfiles), "content_ids.json")
					if len(parts) >= 4 {
						repo = strings.TrimSpace(parts[3])
					}
					if len(parts) >= 5 {
						out = strings.TrimSpace(strings.Join(parts[4:], " "))
					}
					ids, err := vanilla.GenerateContentIDs(repo, out)
					if err != nil {
						fmt.Printf("vanilla ids gen 失败: %v\n", err)
						continue
					}
					if err := reloadVanillaContentIDs(out); err != nil {
						fmt.Printf("vanilla ids 已生成但加载失败: %v\n", err)
						continue
					}
					fmt.Printf("vanilla ids 生成并加载完成: blocks=%d units=%d items=%d liquids=%d statuses=%d weathers=%d bullets=%d effects=%d sounds=%d teams=%d commands=%d stances=%d path=%s\n",
						len(ids.Blocks), len(ids.Units), len(ids.Items), len(ids.Liquids), len(ids.Statuses), len(ids.Weathers), len(ids.Bullets),
						len(ids.Effects), len(ids.Sounds), len(ids.Teams), len(ids.Commands), len(ids.Stances), out)
				case "reload":
					path := filepath.Join(filepath.Dir(cfg.Runtime.VanillaProfiles), "content_ids.json")
					if len(parts) >= 4 {
						path = strings.TrimSpace(strings.Join(parts[3:], " "))
					}
					if err := reloadVanillaContentIDs(path); err != nil {
						fmt.Printf("vanilla ids reload 失败: %v\n", err)
						continue
					}
					fmt.Printf("vanilla content ids 已加载: %s\n", canonicalRuntimePath(path))
				default:
					fmt.Println("用法: vanilla ids gen [repoRoot] [outPath] | vanilla ids reload [path]")
				}
			default:
				fmt.Println("用法: vanilla status | vanilla reload [path] | vanilla gen [repoRoot] [outPath] | vanilla ids gen [repoRoot] [outPath] | vanilla ids reload [path]")
			}
		case "players":
			sessions := srv.ListSessions()
			if len(sessions) == 0 {
				fmt.Println("当前无在线连接")
				continue
			}
			for _, p := range sessions {
				fmt.Printf("id=%d connected=%v ip=%s uuid=%s name=%q\n", p.ID, p.Connected, p.IP, p.UUID, p.Name)
			}
		case "uuid":
			sessions := srv.ListSessions()
			if len(parts) == 1 {
				if len(sessions) == 0 {
					fmt.Println("当前无在线连接")
					continue
				}
				for _, p := range sessions {
					fmt.Printf("id=%d uuid=%s name=%q ip=%s\n", p.ID, p.UUID, p.Name, p.IP)
				}
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil {
				fmt.Println("用法: uuid [conn-id]")
				continue
			}
			found := false
			for _, p := range sessions {
				if p.ID == int32(id) {
					fmt.Printf("id=%d uuid=%s name=%q ip=%s\n", p.ID, p.UUID, p.Name, p.IP)
					found = true
					break
				}
			}
			if !found {
				fmt.Println("未找到该连接ID")
			}
		case "status":
			if len(parts) == 1 {
				fmt.Println(monitor.FormatOnce())
				continue
			}
			if len(parts) >= 3 && strings.EqualFold(parts[1], "watch") {
				switch strings.ToLower(parts[2]) {
				case "on":
					monitor.Enable()
					fmt.Println("status watch 已开启")
				case "off":
					monitor.Disable()
					fmt.Println("status watch 已关闭")
				default:
					fmt.Println("用法: status watch on|off")
				}
				continue
			}
			fmt.Println("用法: status | status watch on|off")
		case "kick":
			if len(parts) < 2 {
				fmt.Println("用法: kick <conn-id> [reason]")
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil {
				fmt.Printf("连接ID无效: %v\n", err)
				continue
			}
			reason := "kicked by admin"
			if len(parts) > 2 {
				reason = strings.TrimSpace(strings.Join(parts[2:], " "))
			}
			if !srv.KickByID(int32(id), reason) {
				fmt.Println("未找到该连接ID")
				continue
			}
			fmt.Printf("已踢出: id=%d reason=%q\n", id, reason)
		case "ban":
			if len(parts) < 3 {
				fmt.Println("用法: ban uuid <uuid> [reason] | ban ip <ip> [reason] | ban id <conn-id> [reason]")
				continue
			}
			targetType := strings.ToLower(parts[1])
			reason := "banned by admin"
			if len(parts) > 3 {
				reason = strings.TrimSpace(strings.Join(parts[3:], " "))
			}
			switch targetType {
			case "uuid":
				count := srv.BanUUID(parts[2], reason)
				fmt.Printf("已封禁UUID=%s，踢出连接数=%d\n", parts[2], count)
			case "ip":
				count := srv.BanIP(parts[2], reason)
				fmt.Printf("已封禁IP=%s，踢出连接数=%d\n", parts[2], count)
			case "id":
				id, err := strconv.ParseInt(parts[2], 10, 32)
				if err != nil {
					fmt.Printf("连接ID无效: %v\n", err)
					continue
				}
				var hit *netserver.SessionInfo
				for _, s := range srv.ListSessions() {
					if s.ID == int32(id) {
						cp := s
						hit = &cp
						break
					}
				}
				if hit == nil {
					fmt.Println("未找到该连接ID")
					continue
				}
				if hit.UUID != "" {
					count := srv.BanUUID(hit.UUID, reason)
					fmt.Printf("已封禁UUID=%s，踢出连接数=%d\n", hit.UUID, count)
				}
				if hit.IP != "" {
					_ = srv.BanIP(hit.IP, reason)
				}
			default:
				fmt.Println("ban 子命令仅支持: uuid | ip | id")
			}
		case "unban":
			if len(parts) < 3 {
				fmt.Println("用法: unban uuid <uuid> | unban ip <ip>")
				continue
			}
			switch strings.ToLower(parts[1]) {
			case "uuid":
				if srv.UnbanUUID(parts[2]) {
					fmt.Printf("已解封UUID=%s\n", parts[2])
				} else {
					fmt.Println("该UUID不在封禁列表")
				}
			case "ip":
				if srv.UnbanIP(parts[2]) {
					fmt.Printf("已解封IP=%s\n", parts[2])
				} else {
					fmt.Println("该IP不在封禁列表")
				}
			default:
				fmt.Println("unban 子命令仅支持: uuid | ip")
			}
		case "bans":
			uuidBans, ipBans := srv.BanLists()
			fmt.Printf("UUID封禁(%d):\n", len(uuidBans))
			for u, r := range uuidBans {
				fmt.Printf("  %s => %s\n", u, r)
			}
			fmt.Printf("IP封禁(%d):\n", len(ipBans))
			for ip, r := range ipBans {
				fmt.Printf("  %s => %s\n", ip, r)
			}
		case "op":
			if len(parts) < 2 {
				fmt.Println("用法: op <uuid>")
				continue
			}
			srv.AddOp(parts[1])
			saveOps()
			fmt.Printf("已设置OP: %s\n", parts[1])
		case "opid":
			if len(parts) < 2 {
				fmt.Println("用法: opid <conn-id>")
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil {
				fmt.Println("连接ID无效")
				continue
			}
			var uuid string
			for _, s := range srv.ListSessions() {
				if s.ID == int32(id) {
					uuid = s.UUID
					break
				}
			}
			if uuid == "" {
				fmt.Println("未找到该连接ID或该连接无uuid")
				continue
			}
			srv.AddOp(uuid)
			saveOps()
			fmt.Printf("已设置OP: conn-id=%d uuid=%s\n", id, uuid)
		case "deop":
			if len(parts) < 2 {
				fmt.Println("用法: deop <uuid>")
				continue
			}
			srv.RemoveOp(parts[1])
			saveOps()
			fmt.Printf("已移除OP: %s\n", parts[1])
		case "ops":
			ops := srv.ListOps()
			if len(ops) == 0 {
				fmt.Println("当前无OP")
				continue
			}
			fmt.Printf("OP列表: %s\n", strings.Join(ops, ", "))
		case "despawn":
			if len(parts) < 2 {
				fmt.Println("用法: despawn <entity-id>")
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil || id <= 0 {
				fmt.Println("entity-id 无效")
				continue
			}
			if removeEntityByID == nil || !removeEntityByID(int32(id)) {
				fmt.Println("entity-id 不存在")
				continue
			}
			fmt.Printf("已移除单位: id=%d\n", id)
		case "umove":
			if len(parts) < 4 {
				fmt.Println("用法: umove <entity-id> <vx> <vy> [rot-vel]")
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil || id <= 0 {
				fmt.Println("entity-id 无效")
				continue
			}
			vx, err := strconv.ParseFloat(parts[2], 32)
			if err != nil {
				fmt.Println("vx 无效")
				continue
			}
			vy, err := strconv.ParseFloat(parts[3], 32)
			if err != nil {
				fmt.Println("vy 无效")
				continue
			}
			rotVel := float32(0)
			if len(parts) >= 5 {
				if v, e := strconv.ParseFloat(parts[4], 32); e == nil {
					rotVel = float32(v)
				}
			}
			if setEntityMotion == nil || !setEntityMotion(int32(id), float32(vx), float32(vy), rotVel) {
				fmt.Println("entity-id 不存在")
				continue
			}
			fmt.Printf("已设置单位移动: id=%d vx=%.2f vy=%.2f rv=%.2f\n", id, vx, vy, rotVel)
		case "uteleport":
			if len(parts) < 4 {
				fmt.Println("用法: uteleport <entity-id> <x> <y> [rotation]")
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil || id <= 0 {
				fmt.Println("entity-id 无效")
				continue
			}
			x, err := strconv.ParseFloat(parts[2], 32)
			if err != nil {
				fmt.Println("x 无效")
				continue
			}
			y, err := strconv.ParseFloat(parts[3], 32)
			if err != nil {
				fmt.Println("y 无效")
				continue
			}
			rot := float32(0)
			if len(parts) >= 5 {
				if v, e := strconv.ParseFloat(parts[4], 32); e == nil {
					rot = float32(v)
				}
			}
			if setEntityPos == nil || !setEntityPos(int32(id), float32(x), float32(y), rot) {
				fmt.Println("entity-id 不存在")
				continue
			}
			fmt.Printf("已传送单位: id=%d x=%.1f y=%.1f rot=%.1f\n", id, x, y, rot)
		case "ulife":
			if len(parts) < 3 {
				fmt.Println("用法: ulife <entity-id> <seconds(<=0表示无限)>")
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil || id <= 0 {
				fmt.Println("entity-id 无效")
				continue
			}
			sec, err := strconv.ParseFloat(parts[2], 32)
			if err != nil {
				fmt.Println("seconds 无效")
				continue
			}
			if setEntityLife == nil || !setEntityLife(int32(id), float32(sec)) {
				fmt.Println("entity-id 不存在")
				continue
			}
			fmt.Printf("已设置单位寿命: id=%d life=%.2fs\n", id, sec)
		case "ufollow":
			if len(parts) < 3 {
				fmt.Println("用法: ufollow <entity-id> <target-id> [speed]")
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil || id <= 0 {
				fmt.Println("entity-id 无效")
				continue
			}
			targetID, err := strconv.ParseInt(parts[2], 10, 32)
			if err != nil || targetID <= 0 {
				fmt.Println("target-id 无效")
				continue
			}
			speed := float32(0)
			if len(parts) >= 4 {
				if v, e := strconv.ParseFloat(parts[3], 32); e == nil {
					speed = float32(v)
				}
			}
			if setEntityFollow == nil || !setEntityFollow(int32(id), int32(targetID), speed) {
				fmt.Println("entity-id 不存在")
				continue
			}
			fmt.Printf("已设置单位跟随: id=%d target=%d speed=%.2f\n", id, targetID, speed)
		case "upatrol":
			if len(parts) < 6 {
				fmt.Println("用法: upatrol <entity-id> <x1> <y1> <x2> <y2> [speed]")
				continue
			}
			id, err := strconv.ParseInt(parts[1], 10, 32)
			if err != nil || id <= 0 {
				fmt.Println("entity-id 无效")
				continue
			}
			x1, err := strconv.ParseFloat(parts[2], 32)
			if err != nil {
				fmt.Println("x1 无效")
				continue
			}
			y1, err := strconv.ParseFloat(parts[3], 32)
			if err != nil {
				fmt.Println("y1 无效")
				continue
			}
			x2, err := strconv.ParseFloat(parts[4], 32)
			if err != nil {
				fmt.Println("x2 无效")
				continue
			}
			y2, err := strconv.ParseFloat(parts[5], 32)
			if err != nil {
				fmt.Println("y2 无效")
				continue
			}
			speed := float32(0)
			if len(parts) >= 7 {
				if v, e := strconv.ParseFloat(parts[6], 32); e == nil {
					speed = float32(v)
				}
			}
			if setEntityPatrol == nil || !setEntityPatrol(int32(id), float32(x1), float32(y1), float32(x2), float32(y2), speed) {
				fmt.Println("entity-id 不存在")
				continue
			}
			fmt.Printf("已设置单位巡逻: id=%d A(%.1f,%.1f) B(%.1f,%.1f) speed=%.2f\n", id, x1, y1, x2, y2, speed)
		case "ubehavior":
			if len(parts) < 3 {
				fmt.Println("用法: ubehavior clear <entity-id>")
				continue
			}
			action := strings.ToLower(parts[1])
			if action != "clear" {
				fmt.Println("仅支持: clear")
				continue
			}
			id, err := strconv.ParseInt(parts[2], 10, 32)
			if err != nil || id <= 0 {
				fmt.Println("entity-id 无效")
				continue
			}
			if clearEntityBehavior == nil || !clearEntityBehavior(int32(id)) {
				fmt.Println("entity-id 不存在")
				continue
			}
			fmt.Printf("已清除单位行为: id=%d\n", id)
		case "api":
			handleAPIConsole(parts, cfg, apiSrv, saveConfig)
		case "progress":
			printServerProgress(*cfg, apiSrv != nil, scriptCtl)
		case "compat":
			printCompatStatus(*cfg, srv)
		case "script":
			handleScriptConsole(parts, cfg, saveScript, scriptCtl)
		case "js":
			if len(parts) < 2 {
				fmt.Println("用法: js <script.js> [args...]")
				continue
			}
			out, err := runNodeScriptInDir(cfg.Mods.JSDir, parts[1], parts[2:]...)
			if out != "" {
				fmt.Print(out)
			}
			if err != nil {
				fmt.Printf("js 执行失败: %v\n", err)
			}
		case "node":
			if len(parts) < 2 {
				fmt.Println("用法: node <script.js> [args...]")
				continue
			}
			out, err := runNodeScriptInDir(cfg.Mods.NodeDir, parts[1], parts[2:]...)
			if out != "" {
				fmt.Print(out)
			}
			if err != nil {
				fmt.Printf("node 执行失败: %v\n", err)
			}
		case "go":
			if len(parts) < 2 {
				fmt.Println("用法: go <target.go|.> [args...]")
				continue
			}
			out, err := runGoInDir(cfg.Mods.GoDir, parts[1], parts[2:]...)
			if out != "" {
				fmt.Print(out)
			}
			if err != nil {
				fmt.Printf("go 执行失败: %v\n", err)
			}
		default:
			fmt.Printf("未知命令: %s\n", cmd)
		}
	}
}

func printConsoleIntro(serverName, worldPath, listenAddr, apiBind string, apiEnabled bool, personalization config.PersonalizationConfig) {
	if !personalization.ConsoleIntroEnabled {
		return
	}
	fmt.Println("========================================")
	if strings.TrimSpace(serverName) == "" {
		serverName = "mdt-server"
	}
	if personalization.ConsoleIntroServerNameEnabled {
		fmt.Printf("服务器名称: %s\n", serverName)
	}
	if personalization.ConsoleIntroCurrentMapEnabled {
		fmt.Printf("当前地图:   %s\n", worldPath)
	}
	if personalization.ConsoleIntroListenAddrEnabled {
		fmt.Printf("监听地址:   %s\n", listenAddr)
	}
	if personalization.ConsoleIntroLocalIPEnabled {
		if ip := firstLocalIPv4(); ip != "" {
			fmt.Printf("本机IP:     %s\n", ip)
		}
	}
	if personalization.ConsoleIntroAPIEnabled {
		if apiEnabled {
			fmt.Printf("API地址:    %s\n", apiBind)
		} else {
			fmt.Println("API地址:    已关闭")
		}
	}
	if personalization.ConsoleIntroHelpHintEnabled {
		fmt.Println("输入 `help all` 查看完整帮助")
	}
	fmt.Println("========================================")
}

func printHelp(cfg config.Config) {
	fmt.Println("控制台命令（输入 `help <分类>` 或 `help all`）:")
	fmt.Println("分类: basic vanilla runtime admin plugin script data scheduler persist net sync api chat close")
	fmt.Println("提示: 输入 `help basic` 查看常用命令")
}

func printBrandFooter() {
	displayName := strings.TrimSpace(buildinfo.DisplayName)
	gameVersion := strings.TrimSpace(buildinfo.GameVersion)
	externalVersion := strings.TrimSpace(buildinfo.Version)
	qqGroup := strings.TrimSpace(buildinfo.QQGroup)
	footer := strings.TrimSpace(buildinfo.FooterText)

	if displayName == "" && gameVersion == "" && externalVersion == "" && qqGroup == "" && footer == "" {
		return
	}

	const (
		deepBlue  = "\x1b[34m"
		lightBlue = "\x1b[94m"
		purple    = "\x1b[35m"
		pink      = "\x1b[95m"
		reset     = "\x1b[0m"
	)

	fmt.Printf("%s========================================%s\n", purple, reset)
	fmt.Println()
	if displayName != "" {
		fmt.Printf("%s名称：%s%s%s\n", deepBlue, lightBlue, displayName, reset)
	}
	if gameVersion != "" {
		fmt.Printf("%s游戏版本: %s%s%s\n", deepBlue, lightBlue, gameVersion, reset)
	}
	if externalVersion != "" {
		fmt.Printf("%s外部版本: %s%s%s\n", deepBlue, lightBlue, externalVersion, reset)
	}
	if qqGroup != "" {
		fmt.Printf("%s加入qq群：%s%s%s\n", deepBlue, lightBlue, qqGroup, reset)
	}
	if footer != "" {
		fmt.Printf("%s%s%s\n", pink, footer, reset)
	}
	fmt.Println()
	fmt.Printf("%s========================================%s\n", purple, reset)
}

func printHelpCategory(cfg config.Config, category string) {
	if category == "" {
		category = "basic"
	}
	if category == "all" {
		for _, c := range []string{"basic", "vanilla", "runtime", "admin", "plugin", "script", "data", "scheduler", "persist", "net", "sync", "api", "chat", "close"} {
			printHelpCategory(cfg, c)
		}
		return
	}
	fmt.Printf("【%s】\n", category)
	switch category {
	case "basic":
		printHelpCmd("help [category|all]", "显示帮助")
		printHelpCmd("maps", "列出可用地图")
		printHelpCmd("world", "查看当前地图文件")
		printHelpCmd("server status", "查看服务器名称/简介/虚拟人数")
		printHelpCmd("server name <名称>", "设置服务器名称（写入配置）")
		printHelpCmd("server desc <简介>", "设置服务器简介（写入配置）")
		printHelpCmd("server players <虚拟人数>", "设置大厅虚拟人数（写入配置）")
		printHelpCmd("host random", "原版式重载到随机地图（不踢人）")
		printHelpCmd("host <map-name>", "原版式重载到 core/assets/maps/default/<map-name>.msav")
		printHelpCmd("host <file-path>", "原版式重载到指定 .msav")
		printHelpCmd("hotload random", "在线热加载到随机地图（不踢人）")
		printHelpCmd("hotload <map-name|file-path>", "在线热加载到指定地图（不踢人）")
		printHelpCmd("ip", "显示本机 IP 和监听地址")
		printHelpCmd("selfcheck", "基本自检（地址/端口/地图/配置）")
	case "vanilla":
		printHelpCmd("vanilla status", "查看原版参数文件路径")
		printHelpCmd("vanilla reload [path]", "重载原版参数文件（可选修改路径并写入配置）")
		printHelpCmd("vanilla gen [repoRoot] [outPath]", "从原版源码自动生成并加载 profiles.json（可选输出路径）")
		printHelpCmd("vanilla ids gen [repoRoot] [outPath]", "从原版源码/logicids.dat 生成并加载 content IDs")
		printHelpCmd("vanilla ids reload [path]", "重载 content IDs 到协议内容注册表")
		fmt.Printf("  当前文件: %s\n", canonicalRuntimePath(cfg.Runtime.VanillaProfiles))
	case "runtime":
		printHelpCmd("status", "输出服务器资源状态")
		printHelpCmd("status watch on|off", "周期输出服务器资源状态")
		printHelpCmd("players", "列出当前连接")
		printHelpCmd("uuid [conn-id]", "列出在线连接uuid或查询指定连接uuid")
	case "admin":
		printHelpCmd("#<msg>", "向所有玩家发送聊天")
		printHelpCmd("kick <conn-id> [reason]", "踢出指定连接")
		printHelpCmd("ban uuid|ip|id ...", "封禁")
		printHelpCmd("unban uuid|ip ...", "解封")
		printHelpCmd("bans", "查看封禁列表")
		printHelpCmd("op <uuid>", "设置OP")
		printHelpCmd("opid <conn-id>", "按连接ID设置OP（自动取uuid）")
		printHelpCmd("deop <uuid>", "移除OP")
		printHelpCmd("ops", "列出OP")
		printHelpCmd("despawn <entity-id>", "移除单位实体并广播销毁")
		printHelpCmd("umove <entity-id> <vx> <vy> [rot-vel]", "设置单位速度/角速度")
		printHelpCmd("uteleport <entity-id> <x> <y> [rotation]", "传送单位并设置朝向")
		printHelpCmd("ulife <entity-id> <seconds>", "设置单位寿命(<=0为无限)")
		printHelpCmd("ufollow <entity-id> <target-id> [speed]", "设置单位跟随目标")
		printHelpCmd("upatrol <entity-id> <x1> <y1> <x2> <y2> [speed]", "设置单位巡逻")
		printHelpCmd("ubehavior clear <entity-id>", "清除单位行为并停止")
	case "plugin":
		printHelpCmd("mods", "列出 Java 模组（jar）")
		printHelpCmd("mod", "列出脚本插件（js/go/node）")
		printHelpCmd("js <script.js> [args]", fmt.Sprintf("在 %s 目录运行 Node.js 脚本", cfg.Mods.JSDir))
		printHelpCmd("node <script.js> [args]", fmt.Sprintf("在 %s 目录运行 Node.js 脚本", cfg.Mods.NodeDir))
		printHelpCmd("go <target|.> [args]", fmt.Sprintf("在 %s 目录运行 go run", cfg.Mods.GoDir))
	case "script":
		printHelpCmd("script file", "显示脚本配置文件路径（JSON）")
		printHelpCmd("script gc now", "立即执行 GC+释放内存")
		printHelpCmd("script gc daily <HH:MM|off>", "设置每日定时 GC（写入配置）")
		printHelpCmd("script startup list", "查看开机任务（来自配置文件）")
		fmt.Println("  建议: 通过 JSON 配置脚本任务，开机自动读取执行")
	case "data":
		printHelpCmd("data status", "查看事件存储状态")
		printHelpCmd("data db on|off", "切换 database_enabled")
		printHelpCmd("data mode <mode>", "设置 file|postgres|mysql|redis")
		printHelpCmd("data dir <path>", "设置文件存储目录")
	case "scheduler":
		printHelpCmd("scheduler status|on|off", "调度器配置")
		fmt.Printf("  scheduler 状态:           %s\n", colorState(cfg.Runtime.SchedulerEnabled))
	case "persist":
		fmt.Printf("  persist: enabled=%v dir=%s file=%s interval=%ds\n", cfg.Persist.Enabled, canonicalRuntimePath(cfg.Persist.Directory), cfg.Persist.File, cfg.Persist.IntervalSec)
		fmt.Printf("  msav: enabled=%v dir=%s file=%s\n", cfg.Persist.SaveMSAV, canonicalRuntimePath(cfg.Persist.MSAVDir), cfg.Persist.MSAVFile)
		fmt.Printf("  script file: %s\n", canonicalRuntimePath(cfg.Script.File))
		fmt.Printf("  ops file: %s\n", canonicalRuntimePath(cfg.Admin.OpsFile))
	case "net":
		fmt.Printf("  UDP 重试次数: %d\n", cfg.Net.UdpRetryCount)
		fmt.Printf("  UDP 重试间隔: %dms\n", cfg.Net.UdpRetryDelayMs)
		fmt.Printf("  UDP 失败回退 TCP: %v\n", cfg.Net.UdpFallbackTCP)
	case "sync":
		printHelpCmd("sync status", "查看同步频率")
		printHelpCmd("sync entity <ms>", "设置实体快照频率（毫秒）")
		printHelpCmd("sync state <ms>", "设置状态快照频率（毫秒）")
		printHelpCmd("sync set <entityMs> <stateMs>", "一次性设置实体/状态频率（毫秒）")
		printHelpCmd("sync default", "恢复默认：entity=100ms state=250ms")
		fmt.Printf("  当前配置: entity=%dms state=%dms\n", cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs)
	case "api":
		printHelpCmd("api status", "查看 API 状态")
		printHelpCmd("api keys", "查看已有 APIKEY")
		printHelpCmd("api keygen", "生成并保存 APIKEY")
		printHelpCmd("api keydel <key>", "删除 APIKEY")
		printHelpCmd("apikey", "兼容旧命令，显示状态")
	case "chat":
		printHelpCmd("/help", "查看玩家命令帮助")
		printHelpCmd("/status", "查看服务器状态")
		printHelpCmd("/summon <typeId|unitName> [x y] [count] [team]", "OP召唤单位（支持 alpha/mono/nova；省略坐标=玩家脚底）")
		printHelpCmd("/despawn <entityId>", "OP移除指定单位")
		printHelpCmd("/kill", "杀死自己当前单位（附身/未附身均可）")
		printHelpCmd("/stop", "OP保存并关闭服务器")
	case "close":
		printHelpCmd("stop", "保存并关闭服务器")
		printHelpCmd("exit", "立即退出服务器（不保存）")
	default:
		fmt.Printf("未知分类: %s\n", category)
	}
}

func printHelpCmd(cmd, desc string) {
	fmt.Printf("  \x1b[34m%-28s\x1b[0m %s\n", cmd, desc)
}

func firstLocalIPv4() string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet == nil || ipNet.IP == nil {
				continue
			}
			ip := ipNet.IP.To4()
			if ip == nil || ip.IsLoopback() {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

func colorState(enabled bool) string {
	if enabled {
		return "\x1b[32m✓ 开启\x1b[0m"
	}
	return "\x1b[31m✗ 关闭\x1b[0m"
}

func sendChatHelp(srv *netserver.Server, c *netserver.Conn, cfg config.Config) {
	_ = cfg
	if srv == nil || c == nil {
		return
	}
	showHelpPageMenu(srv, c, currentJoinPopupRuntime(), 0)
}

type scriptPlugin struct {
	Runtime string
	Name    string
	Path    string
}

func listScriptPlugins(cfg config.ModsConfig) ([]scriptPlugin, error) {
	var out []scriptPlugin
	lists := []struct {
		runtime string
		dir     string
		exts    map[string]struct{}
	}{
		{runtime: "js", dir: cfg.JSDir, exts: map[string]struct{}{".js": {}, ".mjs": {}, ".cjs": {}}},
		{runtime: "node", dir: cfg.NodeDir, exts: map[string]struct{}{".js": {}, ".mjs": {}, ".cjs": {}}},
		{runtime: "go", dir: cfg.GoDir, exts: map[string]struct{}{".go": {}}},
	}
	for _, item := range lists {
		dir := strings.TrimSpace(item.dir)
		if dir == "" {
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			ext := strings.ToLower(filepath.Ext(e.Name()))
			if _, ok := item.exts[ext]; !ok {
				continue
			}
			out = append(out, scriptPlugin{
				Runtime: item.runtime,
				Name:    strings.TrimSuffix(e.Name(), ext),
				Path:    filepath.Join(dir, e.Name()),
			})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Runtime == out[j].Runtime {
			return out[i].Path < out[j].Path
		}
		return out[i].Runtime < out[j].Runtime
	})
	return out, nil
}

func runNodeScriptInDir(baseDir, script string, args ...string) (string, error) {
	absScript, absBase, err := securePathInDir(baseDir, script)
	if err != nil {
		return "", err
	}
	ext := strings.ToLower(filepath.Ext(absScript))
	switch ext {
	case ".js", ".mjs", ".cjs":
	default:
		return "", fmt.Errorf("仅支持 .js/.mjs/.cjs: %s", absScript)
	}
	cmd := exec.Command("node", append([]string{absScript}, args...)...)
	cmd.Dir = absBase
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func runGoInDir(baseDir, target string, args ...string) (string, error) {
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(absBase, 0755); err != nil {
		return "", err
	}
	clean := strings.TrimSpace(target)
	if clean == "" {
		return "", errors.New("target 不能为空")
	}
	if filepath.IsAbs(clean) {
		return "", errors.New("target 必须是相对路径")
	}
	if clean != "." {
		absTarget, _, serr := securePathInDir(absBase, clean)
		if serr != nil {
			return "", serr
		}
		relTarget, rerr := filepath.Rel(absBase, absTarget)
		if rerr != nil {
			return "", rerr
		}
		clean = relTarget
	}
	cmdArgs := append([]string{"run", clean}, args...)
	cmd := exec.Command("go", cmdArgs...)
	cmd.Dir = absBase
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func securePathInDir(baseDir, relative string) (string, string, error) {
	base := strings.TrimSpace(baseDir)
	if base == "" {
		return "", "", errors.New("目录未配置")
	}
	absBase, err := filepath.Abs(base)
	if err != nil {
		return "", "", err
	}
	if err := os.MkdirAll(absBase, 0755); err != nil {
		return "", "", err
	}
	rel := filepath.Clean(strings.TrimSpace(relative))
	if rel == "." || rel == "" {
		return "", "", errors.New("目标不能为空")
	}
	if filepath.IsAbs(rel) {
		return "", "", errors.New("目标必须是相对路径")
	}
	absTarget, err := filepath.Abs(filepath.Join(absBase, rel))
	if err != nil {
		return "", "", err
	}
	prefix := absBase + string(os.PathSeparator)
	if absTarget != absBase && !strings.HasPrefix(absTarget, prefix) {
		return "", "", errors.New("目标超出允许目录")
	}
	st, err := os.Stat(absTarget)
	if err != nil {
		return "", "", err
	}
	if st.IsDir() {
		return "", "", errors.New("目标不能是目录")
	}
	return absTarget, absBase, nil
}

func handleAPIConsole(parts []string, cfg *config.Config, apiSrv *api.Server, saveConfig func() error) {
	if len(parts) == 1 || strings.EqualFold(parts[1], "status") {
		fmt.Printf("api: enabled=%v bind=%s keys=%d\n", cfg.API.Enabled, cfg.API.Bind, len(cfg.API.Keys))
		fmt.Println("用法: api status | api keys | api keygen | api keydel <key>")
		return
	}
	switch strings.ToLower(parts[1]) {
	case "keys":
		if len(cfg.API.Keys) == 0 {
			fmt.Println("当前无 APIKEY")
			return
		}
		fmt.Printf("APIKEY(%d):\n", len(cfg.API.Keys))
		for _, k := range cfg.API.Keys {
			fmt.Printf("  %s\n", k)
		}
	case "keygen":
		key, err := generateAPIKey()
		if err != nil {
			fmt.Printf("生成 APIKEY 失败: %v\n", err)
			return
		}
		cfg.API.Keys = mergeKeys(cfg.API.Keys, key)
		cfg.API.Key = ""
		if apiSrv != nil {
			_ = apiSrv.AddAPIKey(key)
		}
		if err := saveConfig(); err != nil {
			fmt.Printf("保存配置失败: %v\n", err)
			return
		}
		fmt.Printf("已生成 APIKEY: %s\n", key)
	case "keydel":
		if len(parts) < 3 {
			fmt.Println("用法: api keydel <key>")
			return
		}
		target := strings.TrimSpace(parts[2])
		if target == "" {
			fmt.Println("key 不能为空")
			return
		}
		cfg.API.Keys = removeKey(cfg.API.Keys, target)
		cfg.API.Key = ""
		if apiSrv != nil {
			_ = apiSrv.DeleteAPIKey(target)
		}
		if err := saveConfig(); err != nil {
			fmt.Printf("保存配置失败: %v\n", err)
			return
		}
		fmt.Println("已删除 APIKEY")
	default:
		fmt.Println("用法: api status | api keys | api keygen | api keydel <key>")
	}
}

func mergeKeys(keys []string, extra ...string) []string {
	set := map[string]struct{}{}
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k != "" {
			set[k] = struct{}{}
		}
	}
	for _, k := range extra {
		k = strings.TrimSpace(k)
		if k != "" {
			set[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mergeValidAPIKeys(keys []string, extra ...string) ([]string, error) {
	merged := mergeKeys(keys, extra...)
	for _, key := range merged {
		if !config.IsValidAPIKey(key) {
			return nil, fmt.Errorf("API密钥不合格: %s", key)
		}
	}
	return merged, nil
}

func applyAPIKeySet(apiSrv *api.Server, desired []string) {
	if apiSrv == nil {
		return
	}
	current := apiSrv.ListAPIKeys()
	curSet := map[string]struct{}{}
	dstSet := map[string]struct{}{}
	for _, k := range current {
		curSet[k] = struct{}{}
	}
	for _, k := range desired {
		dstSet[k] = struct{}{}
	}
	for k := range curSet {
		if _, ok := dstSet[k]; !ok {
			_ = apiSrv.DeleteAPIKey(k)
		}
	}
	for k := range dstSet {
		if _, ok := curSet[k]; !ok {
			_ = apiSrv.AddAPIKey(k)
		}
	}
}

func removeKey(keys []string, target string) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return mergeKeys(keys)
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		if strings.TrimSpace(k) == target {
			continue
		}
		out = append(out, k)
	}
	return mergeKeys(out)
}

func generateAPIKey() (string, error) {
	parts := []int{15, 13, 15, 19, 12, 10}
	out := []string{"mdt-server-go"}
	for i, n := range parts {
		if i == 5 {
			out = append(out, "yzf")
		}
		s, err := randomAlphaNum(n)
		if err != nil {
			return "", err
		}
		out = append(out, s)
	}
	return strings.Join(out, "-"), nil
}

func randomAlphaNum(n int) (string, error) {
	const alpha = "abcdefghijklmnopqrstuvwxyz0123456789"
	if n <= 0 {
		return "", nil
	}
	buf := make([]byte, n)
	raw := make([]byte, n)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	for i := 0; i < n; i++ {
		buf[i] = alpha[int(raw[i])%len(alpha)]
	}
	return string(buf), nil
}

func currentPlayerNamePrefix() string {
	if v := runtimePlayerNamePrefix.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func currentPlayerNameSuffix() string {
	if v := runtimePlayerNameSuffix.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func currentPlayerBoundPrefix() string {
	if v := runtimePlayerBoundPrefix.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func currentPlayerUnboundPrefix() string {
	if v := runtimePlayerUnboundPrefix.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

func currentPlayerConnIDSuffixFormat() string {
	if v := runtimePlayerConnIDSuffixFormat.Load(); v != nil {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			return s
		}
	}
	return " [gray]{id}[]"
}

func applyIDFormat(format, id string) string {
	if strings.TrimSpace(format) == "" || strings.TrimSpace(id) == "" {
		return ""
	}
	return strings.ReplaceAll(format, "{id}", id)
}

func formatDisplayPlayerNameRaw(name string, c *netserver.Conn, publicStore *persist.PublicConnUUIDStore, identityStore *persist.PlayerIdentityStore) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "未知玩家"
	}
	if publicStore == nil {
		publicStore = runtimePublicConnUUIDStore
	}
	if identityStore == nil {
		identityStore = runtimePlayerIdentityStore
	}
	resolver := runtimeBindStatusResolver
	var prefix strings.Builder
	var suffix strings.Builder
	prefix.WriteString(currentPlayerNamePrefix())
	if c != nil {
		connUUID := publicConnUUIDValue(publicStore, c.UUID())
		if runtimePlayerBindPrefixEnabled.Load() {
			if resolver != nil && resolver.Bound(connUUID) {
				prefix.WriteString(currentPlayerBoundPrefix())
			} else {
				prefix.WriteString(currentPlayerUnboundPrefix())
			}
		}
		if rec, ok := identityStore.Lookup(connUUID); ok {
			prefix.WriteString(rec.Prefix)
			if runtimePlayerTitleEnabled.Load() && strings.TrimSpace(rec.Title) != "" {
				prefix.WriteString(rec.Title)
			}
			suffix.WriteString(rec.Suffix)
		}
	}
	if c != nil && runtimePlayerConnIDSuffixEnabled.Load() {
		suffix.WriteString(applyIDFormat(currentPlayerConnIDSuffixFormat(), publicConnIDValue(publicStore, c.UUID(), c.ConnID())))
	}
	suffix.WriteString(currentPlayerNameSuffix())
	return prefix.String() + name + suffix.String()
}

func displayPlayerName(c *netserver.Conn) string {
	if c == nil {
		return "未知玩家"
	}
	name := strings.TrimSpace(c.BaseName())
	if name == "" {
		if c.PlayerID() != 0 {
			name = fmt.Sprintf("player-%d", c.PlayerID())
		}
	}
	name = formatDisplayPlayerNameRaw(name, c, nil, nil)
	if runtimePlayerNameColorEnabled.Load() {
		return netserver.RenderMindustryTextForTerminal(name)
	}
	return netserver.StripMindustryColorTags(name)
}

func blockDisplayName(wld *world.World, blockID int16) string {
	if blockID <= 0 || wld == nil {
		return "空"
	}
	model := wld.Model()
	if model == nil {
		return fmt.Sprintf("block-%d", blockID)
	}
	name := strings.TrimSpace(model.BlockNames[blockID])
	if name == "" {
		return fmt.Sprintf("block-%d", blockID)
	}
	return translateBlockNameCN(name)
}

func translateBlockNameCN(name string) string {
	n := strings.ToLower(strings.TrimSpace(name))
	blockNameTranslationMu.RLock()
	cn, ok := blockNameTranslations[n]
	blockNameTranslationMu.RUnlock()
	if ok {
		return cn
	}
	return n
}

type reactorExplosionGroup struct {
	effectName    string
	centerX       int
	centerY       int
	radiusTiles   int
	sourceIndex   int
	affectedIndex []int
}

func isReactorBlockName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "thorium-reactor", "impact-reactor", "flux-reactor", "neoplasia-reactor", "heat-reactor":
		return true
	default:
		return false
	}
}

func reactorExplosionRadiusByEffect(name string) (int, bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "reactorexplosion":
		return 19, true
	case "explosionreactorneoplasm":
		return 9, true
	default:
		return 0, false
	}
}

func effectWorldToTile(v float32) int {
	return int(math.Round(float64((v - 4) / 8)))
}

func destroyedBuildCoords(ev world.EntityEvent) (int, int) {
	pt := protocol.UnpackPoint2(ev.BuildPos)
	return int(pt.X), int(pt.Y)
}

func classifyReactorExplosionBuilds(wld *world.World, evs []world.EntityEvent) map[int]*reactorExplosionGroup {
	groups := make([]*reactorExplosionGroup, 0, 4)
	for i, ev := range evs {
		if ev.Kind != world.EntityEventEffect {
			continue
		}
		radius, ok := reactorExplosionRadiusByEffect(ev.EffectName)
		if !ok {
			continue
		}
		groups = append(groups, &reactorExplosionGroup{
			effectName:  ev.EffectName,
			centerX:     effectWorldToTile(ev.EffectX),
			centerY:     effectWorldToTile(ev.EffectY),
			radiusTiles: radius,
			sourceIndex: -1,
		})
		_ = i
	}
	if len(groups) == 0 {
		return nil
	}

	out := make(map[int]*reactorExplosionGroup)

	for _, group := range groups {
		for i, ev := range evs {
			if ev.Kind != world.EntityEventBuildDestroyed {
				continue
			}
			x, y := destroyedBuildCoords(ev)
			if x != group.centerX || y != group.centerY {
				continue
			}
			blockName := ""
			if wld != nil {
				model := wld.Model()
				if model != nil {
					blockName = model.BlockNames[ev.BuildBlock]
				}
			}
			if isReactorBlockName(blockName) {
				group.sourceIndex = i
				out[i] = group
				break
			}
			if group.sourceIndex < 0 {
				group.sourceIndex = i
				out[i] = group
			}
		}
	}

	for i, ev := range evs {
		if ev.Kind != world.EntityEventBuildDestroyed {
			continue
		}
		if _, ok := out[i]; ok {
			continue
		}
		x, y := destroyedBuildCoords(ev)
		best := (*reactorExplosionGroup)(nil)
		bestDist := math.MaxFloat64
		for _, group := range groups {
			dx := float64(x - group.centerX)
			dy := float64(y - group.centerY)
			dist2 := dx*dx + dy*dy
			if dist2 > float64(group.radiusTiles*group.radiusTiles) {
				continue
			}
			if best == nil || dist2 < bestDist {
				best = group
				bestDist = dist2
			}
		}
		if best != nil {
			best.affectedIndex = append(best.affectedIndex, i)
			out[i] = best
		}
	}

	return out
}

func logGroupedReactorExplosions(wld *world.World, evs []world.EntityEvent, grouped map[int]*reactorExplosionGroup) {
	if len(grouped) == 0 {
		return
	}
	seen := map[*reactorExplosionGroup]struct{}{}
	for _, group := range grouped {
		if group == nil {
			continue
		}
		if _, ok := seen[group]; ok {
			continue
		}
		seen[group] = struct{}{}

		sourceLabel := fmt.Sprintf("(x%d-y%d)", group.centerX, group.centerY)
		sourceTeam := world.TeamID(0)
		sourceBlockID := int16(0)
		sourceBlockName := "未知"
		if group.sourceIndex >= 0 && group.sourceIndex < len(evs) {
			ev := evs[group.sourceIndex]
			sourceTeam = ev.BuildTeam
			sourceBlockID = ev.BuildBlock
			sourceBlockName = blockDisplayName(wld, ev.BuildBlock)
			sx, sy := destroyedBuildCoords(ev)
			sourceLabel = fmt.Sprintf("(x%d-y%d)", sx, sy)
		}

		fmt.Printf("[反应堆爆炸] 源头=%s block=%d(%s) team=%d effect=%s 波及=%d\n",
			sourceLabel, sourceBlockID, sourceBlockName, sourceTeam, group.effectName, len(group.affectedIndex))

		if len(group.affectedIndex) == 0 {
			continue
		}
		parts := make([]string, 0, len(group.affectedIndex))
		for _, idx := range group.affectedIndex {
			if idx < 0 || idx >= len(evs) {
				continue
			}
			ev := evs[idx]
			x, y := destroyedBuildCoords(ev)
			parts = append(parts, fmt.Sprintf("(x%d-y%d)%s team=%d", x, y, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam))
		}
		if len(parts) > 0 {
			fmt.Printf("[爆炸波及] 源头=%s %s\n", sourceLabel, strings.Join(parts, ", "))
		}
	}
}

type unitTypeRef struct {
	id   int16
	name string
}

func (u unitTypeRef) ContentType() protocol.ContentType { return protocol.ContentUnit }
func (u unitTypeRef) ID() int16                         { return u.id }
func (u unitTypeRef) Name() string                      { return u.name }

type bulletTypeRef struct {
	id   int16
	name string
}

func (b bulletTypeRef) ContentType() protocol.ContentType { return protocol.ContentBullet }
func (b bulletTypeRef) ID() int16                         { return b.id }
func (b bulletTypeRef) Name() string                      { return b.name }

type itemRef struct {
	id int16
}

func (i itemRef) ContentType() protocol.ContentType { return protocol.ContentItem }
func (i itemRef) ID() int16                         { return i.id }

type blockRef struct {
	id   int16
	name string
}

func (b blockRef) ContentType() protocol.ContentType { return protocol.ContentBlock }
func (b blockRef) ID() int16                         { return b.id }
func (b blockRef) Name() string                      { return b.name }

func broadcastSummonVisible(srv *netserver.Server, typeID int16, x, y float32, team byte) {
	if srv == nil {
		return
	}
	_ = team
	_ = typeID
	_ = x
	_ = y
	// Disabled for custom-client compatibility: this packet ID is not mapped yet
	// and can be misread as another call packet (causing client crash).
}

func broadcastUnitDestroy(srv *netserver.Server, entityID int32) {
	if srv == nil || entityID == 0 {
		return
	}
	srv.Broadcast(&protocol.Remote_Units_unitDestroy_55{Uid: entityID})
}

func unpackTilePos(pos int32) (int32, int32) {
	return int32(uint16((pos >> 16) & 0xFFFF)), int32(uint16(pos & 0xFFFF))
}

func broadcastSetTile(srv *netserver.Server, buildPos int32, blockID int16, rot int8, team byte) {
	if srv == nil || buildPos < 0 || blockID < 0 {
		return
	}
	srv.Broadcast(&protocol.Remote_Tile_setTile_140{
		Tile:     protocol.TileBox{PosValue: buildPos},
		Block:    protocol.BlockRef{BlkID: blockID, BlkName: ""},
		Team:     protocol.Team{ID: team},
		Rotation: int32(rot) & 0x3,
	})
}

func builderUnitForOwner(srv *netserver.Server, owner int32) protocol.Unit {
	if srv == nil || owner == 0 {
		return nil
	}
	for _, conn := range srv.ListConnectedConns() {
		if conn == nil || conn.PlayerID() != owner || conn.UnitID() == 0 {
			continue
		}
		return protocol.UnitBox{IDValue: conn.UnitID()}
	}
	return nil
}

func buildConstructConfigValue(wld *world.World, ev world.EntityEvent) any {
	if wld == nil {
		return ev.BuildConfig
	}
	if cfgValue, ok := wld.BuildingConfigPacked(ev.BuildPos); ok {
		return cfgValue
	}
	return ev.BuildConfig
}

func broadcastBuildConstructedState(srv *netserver.Server, wld *world.World, ev world.EntityEvent) {
	if srv == nil || ev.BuildPos < 0 || ev.BuildBlock <= 0 {
		return
	}
	cfgValue := buildConstructConfigValue(wld, ev)
	// Finish the client's ConstructBuild first. Sending setTile before this can
	// leave the client stuck on build2 and cause later blockSnapshot mismatches.
	broadcastConstructFinish(srv, ev.BuildPos, ev.BuildBlock, ev.BuildRot, byte(ev.BuildTeam), builderUnitForOwner(srv, ev.BuildOwner), cfgValue)
	broadcastSetTile(srv, ev.BuildPos, ev.BuildBlock, ev.BuildRot, byte(ev.BuildTeam))
	if cfgValue != nil {
		srv.BroadcastTileConfig(ev.BuildPos, cfgValue, nil)
	}
	broadcastRelatedBlockSnapshots(srv, wld, ev.BuildPos)
}

func broadcastBuildBeginPlace(srv *netserver.Server, buildPos int32, blockID int16, rot int8, team byte, config any) {
	if srv == nil || buildPos < 0 || blockID <= 0 {
		return
	}
	x, y := unpackTilePos(buildPos)
	srv.Broadcast(&protocol.Remote_Build_beginPlace_133{
		Unit:        nil,
		Result:      protocol.BlockRef{BlkID: blockID, BlkName: ""},
		Team:        protocol.Team{ID: team},
		X:           x,
		Y:           y,
		Rotation:    int32(rot) & 0x3,
		PlaceConfig: config,
	})
}

func broadcastBuildDeconstructBegin(srv *netserver.Server, buildPos int32, team byte) {
	if srv == nil || buildPos < 0 {
		return
	}
	x, y := unpackTilePos(buildPos)
	srv.Broadcast(&protocol.Remote_Build_beginBreak_132{
		Unit: nil,
		Team: protocol.Team{ID: team},
		X:    x,
		Y:    y,
	})
}

func broadcastBuildDestroyed(srv *netserver.Server, buildPos int32, blockID int16) {
	if srv == nil || buildPos < 0 {
		return
	}
	if blockID < 0 {
		blockID = 0
	}
	srv.Broadcast(&protocol.Remote_ConstructBlock_deconstructFinish_145{
		Tile:    protocol.TileBox{PosValue: buildPos},
		Block:   protocol.BlockRef{BlkID: blockID, BlkName: ""},
		Builder: nil,
	})
}

func broadcastTileBuildDestroyed(srv *netserver.Server, buildPos int32) {
	if srv == nil || buildPos < 0 {
		return
	}
	srv.Broadcast(&protocol.Remote_Tile_buildDestroyed_143{
		Build: protocol.BuildingBox{PosValue: buildPos},
	})
}

func broadcastBuildHealthUpdate(srv *netserver.Server, items []int32) {
	if srv == nil || len(items) == 0 {
		return
	}
	srv.Broadcast(&protocol.Remote_Tile_buildHealthUpdate_144{
		Buildings: protocol.IntSeq{Items: items},
	})
}

func broadcastEffectReliable(srv *netserver.Server, effectID int16, x, y, rotation float32) {
	if srv == nil || effectID < 0 {
		return
	}
	srv.Broadcast(&protocol.Remote_NetClient_effectReliable_13{
		Effect:   protocol.Effect{ID: effectID},
		X:        x,
		Y:        y,
		Rotation: rotation,
		Color:    protocol.Color{RGBA: -1},
	})
}

func broadcastTakeItems(srv *netserver.Server, buildPos int32, itemID int16, amount int32, unitID int32) {
	if srv == nil || buildPos < 0 || itemID < 0 || amount <= 0 || unitID == 0 {
		return
	}
	srv.BroadcastUnreliable(&protocol.Remote_InputHandler_takeItems_61{
		Build:  protocol.BuildingBox{PosValue: buildPos},
		Item:   protocol.ItemRef{ItmID: itemID},
		Amount: amount,
		To:     protocol.UnitBox{IDValue: unitID},
	})
}

func broadcastTransferItemToUnit(srv *netserver.Server, itemID int16, x, y float32, unitID int32) {
	if srv == nil || itemID < 0 || unitID == 0 {
		return
	}
	srv.BroadcastUnreliable(&protocol.Remote_InputHandler_transferItemToUnit_62{
		Item: protocol.ItemRef{ItmID: itemID},
		X:    x,
		Y:    y,
		To:   &protocol.EntityBox{IDValue: unitID},
	})
}

func broadcastTransferItemTo(srv *netserver.Server, unitID int32, itemID int16, amount int32, x, y float32, buildPos int32) {
	if srv == nil || unitID == 0 || buildPos < 0 || itemID < 0 || amount <= 0 {
		return
	}
	srv.BroadcastUnreliable(&protocol.Remote_InputHandler_transferItemTo_71{
		Unit:   protocol.UnitBox{IDValue: unitID},
		Item:   protocol.ItemRef{ItmID: itemID},
		Amount: amount,
		X:      x,
		Y:      y,
		Build:  protocol.BuildingBox{PosValue: buildPos},
	})
}

func broadcastDropItem(srv *netserver.Server, playerID int32, angle float32) {
	if srv == nil || playerID == 0 {
		return
	}
	srv.BroadcastUnreliable(&protocol.Remote_InputHandler_dropItem_88{
		Player: &protocol.EntityBox{IDValue: playerID},
		Angle:  angle,
	})
}

func buildBlockSnapshotPackets(snaps []world.BlockSyncSnapshot) []*protocol.Remote_NetClient_blockSnapshot_34 {
	if len(snaps) == 0 {
		return nil
	}
	// The official client aborts the rest of a blockSnapshot packet as soon as it
	// hits one mismatched or missing building. Keep each snapshot isolated so one
	// bad tile cannot poison unrelated turret/container updates in the same batch.
	packets := make([]*protocol.Remote_NetClient_blockSnapshot_34, 0, len(snaps))
	for _, snap := range snaps {
		if snap.BlockID <= 0 || len(snap.Data) == 0 {
			continue
		}
		writer := protocol.NewWriter()
		_ = writer.WriteInt32(snap.Pos)
		_ = writer.WriteInt16(snap.BlockID)
		_ = writer.WriteBytes(snap.Data)
		packets = append(packets, &protocol.Remote_NetClient_blockSnapshot_34{
			Amount: 1,
			Data:   append([]byte(nil), writer.Bytes()...),
		})
	}
	return packets
}

func buildBlockSnapshotPacketsForWorld(wld *world.World, snaps []world.BlockSyncSnapshot) []*protocol.Remote_NetClient_blockSnapshot_34 {
	if len(snaps) == 0 {
		return nil
	}
	packets := make([]*protocol.Remote_NetClient_blockSnapshot_34, 0, len(snaps))
	for _, snap := range snaps {
		if snap.BlockID <= 0 || len(snap.Data) == 0 {
			continue
		}
		wirePos := snap.Pos
		if wld != nil {
			linearPos, ok := wld.TileIndexFromPackedPos(snap.Pos)
			if !ok {
				continue
			}
			wirePos = linearPos
		}
		writer := protocol.NewWriter()
		_ = writer.WriteInt32(wirePos)
		_ = writer.WriteInt16(snap.BlockID)
		_ = writer.WriteBytes(snap.Data)
		packets = append(packets, &protocol.Remote_NetClient_blockSnapshot_34{
			Amount: 1,
			Data:   append([]byte(nil), writer.Bytes()...),
		})
	}
	return packets
}

func currentWorldPathLooksHidden(path string) bool {
	path = strings.ToLower(strings.TrimSpace(filepath.ToSlash(path)))
	return strings.Contains(path, "/hidden/")
}

func currentRuntimeWorldPath() string {
	if v := runtimeWorldPath.Load(); v != nil {
		if path, ok := v.(string); ok {
			return path
		}
	}
	return ""
}

func filterBlockSnapshotsForViewerTeam(wld *world.World, snaps []world.BlockSyncSnapshot, viewerTeam byte) []world.BlockSyncSnapshot {
	if wld == nil || len(snaps) == 0 || viewerTeam == 0 {
		return snaps
	}
	out := make([]world.BlockSyncSnapshot, 0, len(snaps))
	for _, snap := range snaps {
		info, ok := wld.BuildingInfoPacked(snap.Pos)
		if !ok {
			continue
		}
		if byte(info.Team) != viewerTeam {
			continue
		}
		out = append(out, snap)
	}
	return out
}

func broadcastBlockSnapshotsForHiddenMap(srv *netserver.Server, wld *world.World, snaps []world.BlockSyncSnapshot) {
	if srv == nil || wld == nil || len(snaps) == 0 {
		return
	}
	for _, conn := range srv.ListConnectedConns() {
		if conn == nil || conn.InWorldReloadGrace() {
			continue
		}
		filtered := filterBlockSnapshotsForViewerTeam(wld, snaps, conn.TeamID())
		for _, packet := range buildBlockSnapshotPacketsForWorld(wld, filtered) {
			_ = conn.SendAsync(packet)
		}
	}
}

func sendBlockSnapshotsToConnFiltered(conn *netserver.Conn, wld *world.World, snaps []world.BlockSyncSnapshot) {
	if conn == nil || wld == nil || len(snaps) == 0 {
		return
	}
	if currentWorldPathLooksHidden(currentRuntimeWorldPath()) {
		snaps = filterBlockSnapshotsForViewerTeam(wld, snaps, conn.TeamID())
	}
	for _, packet := range buildBlockSnapshotPacketsForWorld(wld, snaps) {
		_ = conn.SendAsync(packet)
	}
}

func broadcastBlockSnapshotsFiltered(srv *netserver.Server, wld *world.World, snaps []world.BlockSyncSnapshot, unreliable bool) {
	if srv == nil || wld == nil || len(snaps) == 0 {
		return
	}
	if currentWorldPathLooksHidden(currentRuntimeWorldPath()) {
		broadcastBlockSnapshotsForHiddenMap(srv, wld, snaps)
		return
	}
	for _, packet := range buildBlockSnapshotPacketsForWorld(wld, snaps) {
		if unreliable {
			srv.BroadcastUnreliable(packet)
		} else {
			srv.Broadcast(packet)
		}
	}
}

func sendBlockSnapshotsToConn(conn *netserver.Conn, wld *world.World) {
	if conn == nil || wld == nil {
		return
	}
	sendBlockSnapshotsToConnFiltered(conn, wld, wld.BlockSyncSnapshotsLiveOnly())
	sendBlockSnapshotsToConnFiltered(conn, wld, wld.ItemTurretBlockSyncSnapshotsLiveOnly())
}

func newTileConfigPacket(pos int32, value any) (*protocol.Remote_InputHandler_tileConfig_90, bool) {
	clonedValue, err := protocol.CloneObjectValue(value)
	if err != nil {
		stdlog.Printf("[tileconfig] skip pos=%d err=%v type=%T", pos, err, value)
		return nil, false
	}
	return &protocol.Remote_InputHandler_tileConfig_90{
		Build: protocol.BuildingBox{PosValue: pos},
		Value: clonedValue,
	}, true
}

func shouldSendTileConfigForPacked(wld *world.World, pos int32, value any) bool {
	if wld == nil || value == nil {
		return false
	}
	if !isSupportedTileConfigValue(value) {
		return false
	}
	info, ok := wld.BuildingInfoPacked(pos)
	if !ok {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(info.Name)) {
	case "item-source",
		"liquid-source",
		"sorter",
		"inverted-sorter",
		"duct-router",
		"surge-router",
		"unloader",
		"duct-unloader",
		"payload-router",
		"reinforced-payload-router",
		"power-node",
		"power-node-large",
		"surge-tower",
		"beam-link",
		"power-source",
		"bridge-conveyor",
		"phase-conveyor",
		"bridge-conduit",
		"phase-conduit",
		"mass-driver",
		"payload-mass-driver",
		"large-payload-mass-driver":
		return true
	default:
		return false
	}
}

func isSupportedTileConfigValue(value any) bool {
	switch value.(type) {
	case protocol.ItemRef,
		protocol.Point2,
		[]protocol.Point2,
		protocol.Content:
		return true
	default:
		return false
	}
}

func sendBlockSnapshotsForPackedToConn(conn *netserver.Conn, wld *world.World, packedPositions []int32) {
	if conn == nil || wld == nil || len(packedPositions) == 0 {
		return
	}
	sendBlockSnapshotsToConnFiltered(conn, wld, wld.BlockSyncSnapshotsForPackedLiveOnly(packedPositions))
	sendBlockSnapshotsToConnFiltered(conn, wld, wld.ItemTurretBlockSyncSnapshotsForPackedLiveOnly(packedPositions))
}

func expandRelatedBlockSyncPackedPositions(wld *world.World, packedPositions []int32) []int32 {
	if wld == nil || len(packedPositions) == 0 {
		return nil
	}
	seen := make(map[int32]struct{}, len(packedPositions)*2)
	out := make([]int32, 0, len(packedPositions)*2)
	add := func(pos int32) {
		if pos < 0 {
			return
		}
		if _, ok := seen[pos]; ok {
			return
		}
		seen[pos] = struct{}{}
		out = append(out, pos)
	}
	for _, packed := range packedPositions {
		add(packed)
		for _, related := range wld.RelatedBlockSyncPackedPositions(packed) {
			add(related)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

func sendRequestedBlockSnapshotToConn(conn *netserver.Conn, wld *world.World, pos int32) {
	if conn == nil || wld == nil || pos < 0 {
		return
	}
	// Match vanilla NetServer.requestBlockSnapshot(): only send writeSync bytes for
	// the requested building. Re-sending construct/setTile here races blockSnapshot
	// against fresh build creation and can wipe client-side inventory/ammo views.
	sendBlockSnapshotsForPackedToConn(conn, wld, []int32{pos})
}

func authoritativeTileStatePacketsForPacked(wld *world.World, pos int32) []any {
	if wld == nil || pos < 0 {
		return nil
	}
	if wld.BlockSyncSuppressedPacked(pos) {
		return []any{
			&protocol.Remote_Tile_buildDestroyed_143{
				Build: protocol.BuildingBox{PosValue: pos},
			},
			&protocol.Remote_Tile_setTile_140{
				Tile:     protocol.TileBox{PosValue: pos},
				Block:    protocol.BlockRef{BlkID: 0, BlkName: ""},
				Team:     protocol.Team{ID: 0},
				Rotation: 0,
			},
			&protocol.Remote_ConstructBlock_deconstructFinish_145{
				Tile:    protocol.TileBox{PosValue: pos},
				Block:   protocol.BlockRef{BlkID: 0, BlkName: ""},
				Builder: nil,
			},
		}
	}
	liveModel := wld.CloneModel()
	if liveModel == nil {
		return nil
	}
	tile, ok := tileAtPacked(liveModel, pos)
	if !ok {
		return nil
	}
	if tile == nil || tile.Block <= 0 || tile.Build == nil {
		return []any{
			&protocol.Remote_Tile_buildDestroyed_143{
				Build: protocol.BuildingBox{PosValue: pos},
			},
			&protocol.Remote_Tile_setTile_140{
				Tile:     protocol.TileBox{PosValue: pos},
				Block:    protocol.BlockRef{BlkID: 0, BlkName: ""},
				Team:     protocol.Team{ID: 0},
				Rotation: 0,
			},
			&protocol.Remote_ConstructBlock_deconstructFinish_145{
				Tile:    protocol.TileBox{PosValue: pos},
				Block:   protocol.BlockRef{BlkID: 0, BlkName: ""},
				Builder: nil,
			},
		}
	}

	team := tile.Team
	if tile.Build.Team != 0 {
		team = tile.Build.Team
	}
	hp := tile.Build.Health
	if hp <= 0 {
		hp = tile.Build.MaxHealth
	}
	if hp <= 0 {
		hp = 1000
	}

	cfgValue, cfgOK := wld.BuildingConfigPacked(pos)
	packets := []any{
		&protocol.Remote_ConstructBlock_constructFinish_146{
			Tile:     protocol.TileBox{PosValue: pos},
			Block:    protocol.BlockRef{BlkID: int16(tile.Block), BlkName: ""},
			Builder:  nil,
			Rotation: tile.Rotation & 0x3,
			Team:     protocol.Team{ID: byte(team)},
			Config:   cfgValue,
		},
		&protocol.Remote_Tile_setTile_140{
			Tile:     protocol.TileBox{PosValue: pos},
			Block:    protocol.BlockRef{BlkID: int16(tile.Block), BlkName: ""},
			Team:     protocol.Team{ID: byte(team)},
			Rotation: int32(tile.Rotation) & 0x3,
		},
		&protocol.Remote_Tile_buildHealthUpdate_144{
			Buildings: protocol.IntSeq{
				Items: []int32{pos, int32(math.Float32bits(hp))},
			},
		},
	}
	if cfgOK {
		if packet, ok := newTileConfigPacket(pos, cfgValue); ok {
			packets = append(packets, packet)
		}
	}
	for _, packet := range buildBlockSnapshotPacketsForWorld(wld, wld.BlockSyncSnapshotsForPackedLiveOnly([]int32{pos})) {
		packets = append(packets, packet)
	}
	for _, packet := range buildBlockSnapshotPacketsForWorld(wld, wld.ItemTurretBlockSyncSnapshotsForPackedLiveOnly([]int32{pos})) {
		packets = append(packets, packet)
	}
	return packets
}

func sendAuthoritativeTileStateToConn(conn *netserver.Conn, wld *world.World, pos int32) {
	if conn == nil || wld == nil {
		return
	}
	for _, packet := range authoritativeTileStatePacketsForPacked(wld, pos) {
		_ = conn.SendAsync(packet)
	}
	sendSharedTeamItemStateForPackedToConn(conn, wld, pos)
}

func sharedTeamItemStatePacketsForPacked(wld *world.World, pos int32) []any {
	// Official NetServer does not emit InputHandler.setTileItems packets for
	// corrective sync. Keeping this path disabled avoids overwriting client item
	// modules with stale team totals.
	_ = wld
	_ = pos
	return nil
}

func sendSharedTeamItemStateForPackedToConn(conn *netserver.Conn, wld *world.World, pos int32) {
	if conn == nil || wld == nil {
		return
	}
	for _, packet := range sharedTeamItemStatePacketsForPacked(wld, pos) {
		_ = conn.SendAsync(packet)
	}
}

func broadcastBlockSnapshots(srv *netserver.Server, wld *world.World) {
	if srv == nil || wld == nil {
		return
	}
	// The world stream already delivered msav inline sync bytes on load/connect.
	// Periodic overlays must be generated from current runtime state only, or they
	// can replay stale map bytes back onto active clients.
	broadcastBlockSnapshotsFiltered(srv, wld, wld.PeriodicBlockSyncSnapshotsLiveOnly(), true)
}

func broadcastBlockSnapshotsForPacked(srv *netserver.Server, wld *world.World, packedPositions []int32) {
	if srv == nil || wld == nil || len(packedPositions) == 0 {
		return
	}
	broadcastBlockSnapshotsFiltered(srv, wld, wld.BlockSyncSnapshotsForPackedLiveOnly(packedPositions), false)
}

func broadcastItemBlockSnapshotsForPacked(srv *netserver.Server, wld *world.World, packedPositions []int32) {
	if srv == nil || wld == nil || len(packedPositions) == 0 {
		return
	}
	broadcastBlockSnapshotsFiltered(srv, wld, wld.BlockSyncSnapshotsForPackedLiveOnly(packedPositions), false)
}

func broadcastItemTurretAmmoSnapshotsForPacked(srv *netserver.Server, wld *world.World, packedPositions []int32) {
	if srv == nil || wld == nil || len(packedPositions) == 0 {
		return
	}
	if wld.BlockSyncLogsEnabled() {
		for _, packed := range packedPositions {
			stdlog.Printf("[turret-ammo] broadcast %s", wld.DebugItemTurretAmmoPacked(packed))
		}
	}
	broadcastBlockSnapshotsFiltered(srv, wld, wld.ItemTurretBlockSyncSnapshotsForPackedLiveOnly(packedPositions), false)
}

func broadcastRelatedBlockSnapshots(srv *netserver.Server, wld *world.World, packedPos int32) {
	if srv == nil || wld == nil || packedPos < 0 {
		return
	}
	positions := wld.RelatedBlockSyncPackedPositions(packedPos)
	broadcastBlockSnapshotsForPacked(srv, wld, positions)
	broadcastItemTurretAmmoSnapshotsForPacked(srv, wld, positions)
}

func broadcastUnitFactorySnapshots(srv *netserver.Server, wld *world.World) {
	if srv == nil || wld == nil {
		return
	}
	broadcastBlockSnapshotsFiltered(srv, wld, wld.UnitFactoryBlockSyncSnapshotsLiveOnly(), true)
}

func broadcastPayloadProcessorSnapshots(srv *netserver.Server, wld *world.World) {
	if srv == nil || wld == nil {
		return
	}
	broadcastBlockSnapshotsFiltered(srv, wld, wld.PayloadProcessorBlockSyncSnapshotsLiveOnly(), true)
}

func syncCurrentWorldToConn(conn *netserver.Conn, wld *world.World) {
	if conn == nil || wld == nil {
		return
	}
	builds := wld.BuildSyncSnapshot()
	health := make([]int32, 0, 256)
	type buildConfigState struct {
		pos   int32
		value any
	}
	configs := make([]buildConfigState, 0, 64)
	for i := range builds {
		b := builds[i]
		if b.BlockID <= 0 {
			continue
		}
		_ = conn.SendAsync(&protocol.Remote_Tile_setTile_140{
			Tile:     protocol.TileBox{PosValue: b.Pos},
			Block:    protocol.BlockRef{BlkID: b.BlockID, BlkName: ""},
			Team:     protocol.Team{ID: byte(b.Team)},
			Rotation: int32(b.Rotation) & 0x3,
		})
		hp := b.Health
		if hp <= 0 {
			hp = 1000
		}
		health = append(health, b.Pos, int32(math.Float32bits(hp)))
		if len(health) >= 256 {
			_ = conn.SendAsync(&protocol.Remote_Tile_buildHealthUpdate_144{
				Buildings: protocol.IntSeq{Items: append([]int32(nil), health...)},
			})
			health = health[:0]
		}
		if cfgValue, ok := wld.BuildingConfigPacked(b.Pos); ok && shouldSendTileConfigForPacked(wld, b.Pos, cfgValue) {
			configs = append(configs, buildConfigState{
				pos:   b.Pos,
				value: cfgValue,
			})
		}
		if i > 0 && i%128 == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if len(health) > 0 {
		_ = conn.SendAsync(&protocol.Remote_Tile_buildHealthUpdate_144{
			Buildings: protocol.IntSeq{Items: health},
		})
	}
	sendBlockSnapshotsToConn(conn, wld)
	for i, cfg := range configs {
		if packet, ok := newTileConfigPacket(cfg.pos, cfg.value); ok {
			_ = conn.SendAsync(packet)
		}
		if i > 0 && i%128 == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func syncLiveWorldRuntimeToConn(conn *netserver.Conn, wld *world.World) {
	if conn == nil || wld == nil {
		return
	}
	builds := wld.BuildSyncSnapshot()
	health := make([]int32, 0, 256)
	unsupportedPositions := make([]int32, 0, 64)
	type buildConfigState struct {
		pos   int32
		value any
	}
	configs := make([]buildConfigState, 0, 64)
	for i := range builds {
		b := builds[i]
		if b.BlockID <= 0 {
			continue
		}
		if wld.HasLiveMapStreamPayloadPacked(b.Pos) {
			continue
		}
		unsupportedPositions = append(unsupportedPositions, b.Pos)
		hp := b.Health
		if hp <= 0 {
			hp = 1000
		}
		health = append(health, b.Pos, int32(math.Float32bits(hp)))
		if len(health) >= 256 {
			_ = conn.SendAsync(&protocol.Remote_Tile_buildHealthUpdate_144{
				Buildings: protocol.IntSeq{Items: append([]int32(nil), health...)},
			})
			health = health[:0]
		}
		if cfgValue, ok := wld.BuildingConfigPacked(b.Pos); ok && shouldSendTileConfigForPacked(wld, b.Pos, cfgValue) {
			configs = append(configs, buildConfigState{
				pos:   b.Pos,
				value: cfgValue,
			})
		}
		if i > 0 && i%128 == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if len(health) > 0 {
		_ = conn.SendAsync(&protocol.Remote_Tile_buildHealthUpdate_144{
			Buildings: protocol.IntSeq{Items: health},
		})
	}
	sendBlockSnapshotsForPackedToConn(conn, wld, unsupportedPositions)
	for i, cfg := range configs {
		if packet, ok := newTileConfigPacket(cfg.pos, cfg.value); ok {
			_ = conn.SendAsync(packet)
		}
		if i > 0 && i%128 == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
}

func syncCoreItemsToConn(conn *netserver.Conn, wld *world.World) {
	// Mindustry-157 does not use InputHandler.setTileItems for authoritative
	// correction. Core inventory travels in stateSnapshot(coreData), and building
	// inventory/ammo travels in blockSnapshot(writeSync).
	_ = conn
	_ = wld
}

func syncAuthoritativeWorldToConn(srv *netserver.Server, conn *netserver.Conn, wld *world.World, baseModel *world.WorldModel, strategy config.AuthoritySyncStrategy) {
	if conn == nil || wld == nil {
		return
	}
	if conn.UsesLiveWorldStream() {
		if srv != nil {
			_ = srv.SyncEntitySnapshotsToConn(conn)
		}
		return
	}
	currentModel := wld.Model()
	if currentModel == nil {
		return
	}
	if baseModel == nil || baseModel.Width != currentModel.Width || baseModel.Height != currentModel.Height {
		syncCurrentWorldToConn(conn, wld)
		if srv != nil {
			_ = srv.SyncEntitySnapshotsToConn(conn)
		}
		return
	}
	switch strategy {
	case config.AuthoritySyncOfficial:
		// Vanilla writeWorld already contains current live map state.
		// Our template stream does not, so the closest emulation is:
		// template world stream + diff correction + live block snapshots.
		syncWorldDiffToConn(conn, wld, baseModel)
	case config.AuthoritySyncStatic:
		// Static mode only reconciles differences against the template map stream.
		syncWorldDiffToConn(conn, wld, baseModel)
	default:
		// Dynamic mode previously replayed every live build through setTile/tileConfig,
		// which could reset factory/progress runtime on clients. Keep it on the
		// safer diff + blockSnapshot path until we have a byte-perfect live map writer.
		syncWorldDiffToConn(conn, wld, baseModel)
	}
	if srv != nil {
		_ = srv.SyncEntitySnapshotsToConn(conn)
	}
}

func syncWorldDiffToConn(conn *netserver.Conn, wld *world.World, baseModel *world.WorldModel) {
	if conn == nil || wld == nil {
		return
	}
	currentModel := wld.Model()
	if currentModel == nil {
		return
	}
	if baseModel == nil || baseModel.Width != currentModel.Width || baseModel.Height != currentModel.Height {
		syncCurrentWorldToConn(conn, wld)
		return
	}

	baseStates := buildSyncSnapshotFromModel(baseModel)
	liveStates := wld.BuildSyncSnapshot()
	baseByPos := make(map[int32]world.BuildSyncState, len(baseStates))
	liveByPos := make(map[int32]world.BuildSyncState, len(liveStates))
	posSet := make(map[int32]struct{}, len(baseStates)+len(liveStates))
	for _, state := range baseStates {
		baseByPos[state.Pos] = state
		posSet[state.Pos] = struct{}{}
	}
	for _, state := range liveStates {
		liveByPos[state.Pos] = state
		posSet[state.Pos] = struct{}{}
	}
	if len(posSet) == 0 {
		sendBlockSnapshotsToConn(conn, wld)
		syncCoreItemsToConn(conn, wld)
		return
	}
	positions := make([]int32, 0, len(posSet))
	for pos := range posSet {
		positions = append(positions, pos)
	}
	sort.Slice(positions, func(i, j int) bool { return positions[i] < positions[j] })

	health := make([]int32, 0, 256)
	changedPacked := make([]int32, 0, 256)
	for i, pos := range positions {
		baseState, baseOK := baseByPos[pos]
		liveState, liveOK := liveByPos[pos]
		if baseOK && liveOK &&
			baseState.BlockID == liveState.BlockID &&
			baseState.Team == liveState.Team &&
			baseState.Rotation == liveState.Rotation &&
			sameBuildHealth(baseState.Health, liveState.Health) &&
			sameTileConfigAtPosLive(baseModel, wld, pos) {
			continue
		}

		if !liveOK || liveState.BlockID <= 0 {
			if baseOK && baseState.BlockID > 0 {
				_ = conn.SendAsync(&protocol.Remote_Tile_buildDestroyed_143{
					Build: protocol.BuildingBox{PosValue: pos},
				})
			}
			_ = conn.SendAsync(&protocol.Remote_Tile_setTile_140{
				Tile:     protocol.TileBox{PosValue: pos},
				Block:    protocol.BlockRef{BlkID: 0, BlkName: ""},
				Team:     protocol.Team{ID: 0},
				Rotation: 0,
			})
			blockID := int16(0)
			if baseOK {
				blockID = baseState.BlockID
			}
			_ = conn.SendAsync(&protocol.Remote_ConstructBlock_deconstructFinish_145{
				Tile:    protocol.TileBox{PosValue: pos},
				Block:   protocol.BlockRef{BlkID: blockID, BlkName: ""},
				Builder: nil,
			})
			changedPacked = append(changedPacked, pos)
			continue
		}

		var cfgValue any
		cfgOK := false
		if value, ok := wld.BuildingConfigPacked(pos); ok {
			cfgValue, cfgOK = value, true
		}
		if modelTileIsConstructLike(baseModel, pos) {
			_ = conn.SendAsync(&protocol.Remote_ConstructBlock_constructFinish_146{
				Tile:     protocol.TileBox{PosValue: pos},
				Block:    protocol.BlockRef{BlkID: liveState.BlockID, BlkName: ""},
				Builder:  nil,
				Rotation: liveState.Rotation & 0x3,
				Team:     protocol.Team{ID: byte(liveState.Team)},
				Config:   cfgValue,
			})
		}
		_ = conn.SendAsync(&protocol.Remote_Tile_setTile_140{
			Tile:     protocol.TileBox{PosValue: pos},
			Block:    protocol.BlockRef{BlkID: liveState.BlockID, BlkName: ""},
			Team:     protocol.Team{ID: byte(liveState.Team)},
			Rotation: int32(liveState.Rotation) & 0x3,
		})

		hp := liveState.Health
		if hp <= 0 {
			hp = 1000
		}
		health = append(health, pos, int32(math.Float32bits(hp)))
		if (cfgOK || !sameTileConfigAtPosLive(baseModel, wld, pos)) && shouldSendTileConfigForPacked(wld, pos, cfgValue) {
			if packet, ok := newTileConfigPacket(pos, cfgValue); ok {
				_ = conn.SendAsync(packet)
			}
		}
		changedPacked = append(changedPacked, pos)
		if len(health) >= 256 {
			_ = conn.SendAsync(&protocol.Remote_Tile_buildHealthUpdate_144{
				Buildings: protocol.IntSeq{Items: append([]int32(nil), health...)},
			})
			health = health[:0]
		}
		if i > 0 && i%128 == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if len(health) > 0 {
		_ = conn.SendAsync(&protocol.Remote_Tile_buildHealthUpdate_144{
			Buildings: protocol.IntSeq{Items: health},
		})
	}
	sendBlockSnapshotsForPackedToConn(conn, wld, expandRelatedBlockSyncPackedPositions(wld, changedPacked))
	// The template world stream replays the .msav baseline. Structural diff only
	// updates changed tiles; unchanged conveyors, unloaders, containers, turrets,
	// crafters and factories still need live writeSync bytes so /sync and initial
	// connect do not roll item/progress/ammo state back to map-load values.
	sendBlockSnapshotsToConn(conn, wld)
	syncCoreItemsToConn(conn, wld)
}

func buildSyncSnapshotFromModel(model *world.WorldModel) []world.BuildSyncState {
	if model == nil || len(model.Tiles) == 0 {
		return nil
	}
	out := make([]world.BuildSyncState, 0, len(model.Tiles)/4)
	for i := range model.Tiles {
		tile := model.Tiles[i]
		if tile.Block <= 0 || tile.Build == nil {
			continue
		}
		if tile.Build.X != tile.X || tile.Build.Y != tile.Y {
			continue
		}
		hp := float32(1000)
		if tile.Build != nil && tile.Build.Health > 0 {
			hp = tile.Build.Health
		}
		team := tile.Team
		if tile.Build != nil && tile.Build.Team != 0 {
			team = tile.Build.Team
		}
		out = append(out, world.BuildSyncState{
			Pos:      protocol.PackPoint2(int32(tile.X), int32(tile.Y)),
			X:        int32(tile.X),
			Y:        int32(tile.Y),
			BlockID:  int16(tile.Block),
			Team:     team,
			Rotation: tile.Rotation,
			Health:   hp,
		})
	}
	return out
}

func tileAtPacked(model *world.WorldModel, pos int32) (*world.Tile, bool) {
	if model == nil {
		return nil, false
	}
	pt := protocol.UnpackPoint2(pos)
	if !model.InBounds(int(pt.X), int(pt.Y)) {
		return nil, false
	}
	tile, err := model.TileAt(int(pt.X), int(pt.Y))
	if err != nil || tile == nil {
		return nil, false
	}
	return tile, true
}

func decodeTileConfigValue(tile *world.Tile) (any, bool) {
	if tile == nil || tile.Build == nil || len(tile.Build.Config) == 0 {
		return nil, false
	}
	value, err := protocol.ReadObject(protocol.NewReader(tile.Build.Config), false, nil)
	if err != nil {
		return nil, false
	}
	return value, true
}

func isConstructLikeBlockName(name string) bool {
	name = strings.ToLower(strings.TrimSpace(name))
	if !strings.HasPrefix(name, "build") || len(name) <= len("build") {
		return false
	}
	for _, r := range name[len("build"):] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func modelTileIsConstructLike(model *world.WorldModel, pos int32) bool {
	tile, ok := tileAtPacked(model, pos)
	if !ok || tile == nil || tile.Block <= 0 || model == nil {
		return false
	}
	return isConstructLikeBlockName(model.BlockNames[int16(tile.Block)])
}

func sameTileConfigAtPos(baseModel, liveModel *world.WorldModel, pos int32) bool {
	baseTile, baseOK := tileAtPacked(baseModel, pos)
	liveTile, liveOK := tileAtPacked(liveModel, pos)
	if !baseOK && !liveOK {
		return true
	}
	var baseCfg, liveCfg []byte
	if baseOK && baseTile != nil && baseTile.Build != nil {
		baseCfg = baseTile.Build.Config
	}
	if liveOK && liveTile != nil && liveTile.Build != nil {
		liveCfg = liveTile.Build.Config
	}
	return bytes.Equal(baseCfg, liveCfg)
}

func sameTileConfigAtPosLive(baseModel *world.WorldModel, wld *world.World, pos int32) bool {
	baseTile, baseOK := tileAtPacked(baseModel, pos)
	var baseCfg any
	var liveCfg any
	var baseHasCfg bool
	var liveHasCfg bool
	if baseOK && baseTile != nil {
		baseCfg, baseHasCfg = decodeTileConfigValue(baseTile)
	}
	if wld != nil {
		liveCfg, liveHasCfg = wld.BuildingConfigPacked(pos)
	}
	if !baseHasCfg && !liveHasCfg {
		return true
	}
	return reflect.DeepEqual(baseCfg, liveCfg)
}

func sameBuildHealth(a, b float32) bool {
	return math.Abs(float64(a-b)) < 0.01
}

func buildRuntimeRulesRaw(wld *world.World, mapPath string) string {
	merged := map[string]any{}
	if wld == nil {
		return "{}"
	}
	marshal := func() string {
		// The Go server does not currently synchronize Mindustry's fog discovery
		// bitsets. Leaving map/campaign fog enabled makes official clients render
		// the world and minimap as fully undiscovered black.
		merged["fog"] = false
		merged["staticFog"] = false
		raw, err := json.Marshal(merged)
		if err != nil || len(raw) == 0 {
			return "{}"
		}
		return string(raw)
	}
	model := wld.Model()
	if model != nil && model.Tags != nil {
		if raw := strings.TrimSpace(model.Tags["rules"]); raw != "" {
			_ = json.Unmarshal([]byte(raw), &merged)
		}
	}
	rulesMgr := wld.GetRulesManager()
	if rulesMgr == nil {
		return marshal()
	}
	rules := rulesMgr.Get()
	if rules == nil {
		return marshal()
	}

	merged["allowEditRules"] = rules.AllowEditRules
	merged["infiniteResources"] = rules.InfiniteResources
	merged["waves"] = rules.Waves
	merged["waveTimer"] = rules.WaveTimer
	merged["airUseSpawns"] = rules.AirUseSpawns
	merged["wavesSpawnAtCores"] = rules.WavesSpawnAtCores
	merged["waveSpacing"] = rules.WaveSpacing
	merged["pvp"] = rules.Pvp
	merged["attackMode"] = rules.AttackMode
	merged["editor"] = rules.Editor
	merged["instantBuild"] = rules.InstantBuild
	merged["buildCostMultiplier"] = rules.BuildCostMultiplier
	merged["buildSpeedMultiplier"] = rules.BuildSpeedMultiplier
	merged["unitBuildSpeedMultiplier"] = rules.UnitBuildSpeedMultiplier
	merged["deconstructRefundMultiplier"] = rules.DeconstructRefundMultiplier
	merged["enemyCoreBuildRadius"] = rules.EnemyCoreBuildRadius
	if rules.Env != 0 {
		merged["env"] = rules.Env
	}
	if modeName := strings.TrimSpace(rules.ModeName); modeName != "" {
		merged["modeName"] = modeName
	}
	return marshal()
}

func syncRulesToConn(conn *netserver.Conn, wld *world.World, mapPath string) {
	if conn == nil || wld == nil {
		return
	}
	raw := buildRuntimeRulesRaw(wld, mapPath)
	_ = conn.SendAsync(&protocol.Remote_NetClient_setRules_23{
		Rules: protocol.Rules{Raw: raw},
	})
}

func broadcastConstructFinish(srv *netserver.Server, buildPos int32, blockID int16, rot int8, team byte, builder protocol.Unit, config any) {
	if srv == nil || buildPos < 0 || blockID <= 0 {
		return
	}
	srv.Broadcast(&protocol.Remote_ConstructBlock_constructFinish_146{
		Tile:     protocol.TileBox{PosValue: buildPos},
		Block:    protocol.BlockRef{BlkID: blockID, BlkName: ""},
		Builder:  builder,
		Rotation: rot & 0x3,
		Team:     protocol.Team{ID: team},
		Config:   config,
	})
}

func broadcastBuildDestroyedState(srv *netserver.Server, ev world.EntityEvent) {
	if srv == nil || ev.BuildPos < 0 {
		return
	}
	broadcastTileBuildDestroyed(srv, ev.BuildPos)
	broadcastBuildDestroyed(srv, ev.BuildPos, ev.BuildBlock)
	broadcastSetTile(srv, ev.BuildPos, 0, 0, 0)
}

func bulletCreatePacketFromEvent(srv *netserver.Server, b world.BulletEvent) *protocol.Remote_BulletType_createBullet_58 {
	if srv == nil || b.BulletTyp < 0 {
		return nil
	}
	bulletType := srv.Content.BulletType(b.BulletTyp)
	if bulletType == nil {
		return nil
	}
	return &protocol.Remote_BulletType_createBullet_58{
		Type:        bulletType,
		Team:        protocol.Team{ID: byte(b.Team)},
		X:           b.X,
		Y:           b.Y,
		Angle:       b.Angle,
		Damage:      b.Damage,
		VelocityScl: 1,
		LifetimeScl: 1,
	}
}

func broadcastBulletCreate(srv *netserver.Server, b world.BulletEvent) {
	packet := bulletCreatePacketFromEvent(srv, b)
	if packet == nil {
		return
	}
	srv.BroadcastUnreliable(packet)
}

type scriptController struct {
	modsCfg config.ModsConfig
	mu      sync.Mutex
	gcStop  chan struct{}
}

func newScriptController(modsCfg config.ModsConfig) *scriptController {
	return &scriptController{modsCfg: modsCfg}
}

func (s *scriptController) RunTask(task config.ScriptTask) (string, error) {
	rt := strings.ToLower(strings.TrimSpace(task.Runtime))
	switch rt {
	case "js":
		return runNodeScriptInDir(s.modsCfg.JSDir, task.Target, task.Args...)
	case "node":
		return runNodeScriptInDir(s.modsCfg.NodeDir, task.Target, task.Args...)
	case "go":
		return runGoInDir(s.modsCfg.GoDir, task.Target, task.Args...)
	default:
		return "", fmt.Errorf("不支持的 runtime: %s", task.Runtime)
	}
}

func (s *scriptController) ScheduleStartupTasks(tasks []config.ScriptTask) {
	for i := range tasks {
		task := tasks[i]
		delay := task.DelaySec
		if delay < 0 {
			delay = 0
		}
		go func(t config.ScriptTask, d int) {
			if d > 0 {
				time.Sleep(time.Duration(d) * time.Second)
			}
			out, err := s.RunTask(t)
			if out != "" {
				fmt.Printf("[script][startup] output runtime=%s target=%s\n%s\n", t.Runtime, t.Target, out)
			}
			if err != nil {
				fmt.Printf("[script][startup] failed runtime=%s target=%s err=%v\n", t.Runtime, t.Target, err)
				return
			}
			fmt.Printf("[script][startup] done runtime=%s target=%s delay=%ds\n", t.Runtime, t.Target, d)
		}(task, delay)
	}
}

func (s *scriptController) RunGCNow() {
	runtime.GC()
	debug.FreeOSMemory()
	fmt.Println("[script] 已执行 GC 与内存回收")
}

func (s *scriptController) SetDailyGC(hhmm string) error {
	hhmm = strings.TrimSpace(hhmm)
	s.mu.Lock()
	if s.gcStop != nil {
		close(s.gcStop)
		s.gcStop = nil
	}
	s.mu.Unlock()
	if hhmm == "" || strings.EqualFold(hhmm, "off") {
		fmt.Println("[script] 每日 GC 已关闭")
		return nil
	}
	if _, err := time.Parse("15:04", hhmm); err != nil {
		return fmt.Errorf("时间格式错误，需 HH:MM: %w", err)
	}
	stop := make(chan struct{})
	s.mu.Lock()
	s.gcStop = stop
	s.mu.Unlock()
	go s.dailyGCLoop(hhmm, stop)
	fmt.Printf("[script] 每日 GC 已设置: %s\n", hhmm)
	return nil
}

func (s *scriptController) dailyGCLoop(hhmm string, stop <-chan struct{}) {
	for {
		now := time.Now()
		today, _ := time.ParseInLocation("15:04", hhmm, now.Location())
		next := time.Date(now.Year(), now.Month(), now.Day(), today.Hour(), today.Minute(), 0, 0, now.Location())
		if !next.After(now) {
			next = next.Add(24 * time.Hour)
		}
		timer := time.NewTimer(time.Until(next))
		select {
		case <-timer.C:
			s.RunGCNow()
		case <-stop:
			timer.Stop()
			return
		}
	}
}

func handleScriptConsole(parts []string, cfg *config.Config, saveScript func() error, ctl *scriptController) {
	if ctl == nil {
		fmt.Println("script 控制器未初始化")
		return
	}
	if len(parts) == 1 || strings.EqualFold(parts[1], "help") {
		fmt.Println("script 用法:")
		fmt.Println("  script help")
		fmt.Println("  script file")
		fmt.Println("  script gc now")
		fmt.Println("  script gc daily <HH:MM|off>")
		fmt.Println("  script startup list")
		fmt.Println("  script startup add <delaySec> <js|node|go> <target> [args...]")
		fmt.Println("  script startup del <index>")
		return
	}
	switch strings.ToLower(parts[1]) {
	case "file":
		fmt.Printf("script 配置文件: %s\n", cfg.Script.File)
	case "gc":
		if len(parts) < 3 {
			fmt.Println("用法: script gc now | script gc daily <HH:MM|off>")
			return
		}
		switch strings.ToLower(parts[2]) {
		case "now":
			ctl.RunGCNow()
		case "daily":
			if len(parts) < 4 {
				fmt.Println("用法: script gc daily <HH:MM|off>")
				return
			}
			val := strings.TrimSpace(parts[3])
			if err := ctl.SetDailyGC(val); err != nil {
				fmt.Printf("设置每日 GC 失败: %v\n", err)
				return
			}
			cfg.Script.DailyGCTime = val
			_ = saveScript()
		default:
			fmt.Println("用法: script gc now | script gc daily <HH:MM|off>")
		}
	case "startup":
		if len(parts) < 3 {
			fmt.Println("用法: script startup list|add|del ...")
			return
		}
		switch strings.ToLower(parts[2]) {
		case "list":
			if len(cfg.Script.StartupTasks) == 0 {
				fmt.Println("当前无开机脚本任务")
				return
			}
			for i, t := range cfg.Script.StartupTasks {
				fmt.Printf("[%d] delay=%ds runtime=%s target=%s args=%v\n", i, t.DelaySec, t.Runtime, t.Target, t.Args)
			}
		case "add":
			if len(parts) < 6 {
				fmt.Println("用法: script startup add <delaySec> <js|node|go> <target> [args...]")
				return
			}
			delay, err := strconv.Atoi(parts[3])
			if err != nil || delay < 0 {
				fmt.Println("delaySec 必须是 >=0 的整数")
				return
			}
			task := config.ScriptTask{
				DelaySec: delay,
				Runtime:  strings.ToLower(parts[4]),
				Target:   parts[5],
				Args:     append([]string(nil), parts[6:]...),
			}
			cfg.Script.StartupTasks = append(cfg.Script.StartupTasks, task)
			_ = saveScript()
			fmt.Println("已添加开机脚本任务（下次启动自动执行）")
		case "del":
			if len(parts) < 4 {
				fmt.Println("用法: script startup del <index>")
				return
			}
			idx, err := strconv.Atoi(parts[3])
			if err != nil || idx < 0 || idx >= len(cfg.Script.StartupTasks) {
				fmt.Println("index 无效")
				return
			}
			cfg.Script.StartupTasks = append(cfg.Script.StartupTasks[:idx], cfg.Script.StartupTasks[idx+1:]...)
			_ = saveScript()
			fmt.Println("已删除开机脚本任务")
		default:
			fmt.Println("用法: script startup list|add|del ...")
		}
	default:
		fmt.Println("用法: script help")
	}
}

func printServerProgress(cfg config.Config, apiEnabled bool, scriptCtl *scriptController) {
	items := []struct {
		Name string
		Done bool
	}{
		{"网络协议握手/连接管理", true},
		{"地图与 MSAV 读写/快照", true},
		{"OP 权限与持久化", true},
		{"召唤指令与写回", true},
		{"API 多 Key 与管理命令", len(cfg.API.Keys) >= 0},
		{"API 管理接口(/help,/ops,/summon,/stop)", apiEnabled},
		{"脚本执行(js/node/go)", true},
		{"脚本自动化(启动任务/每日GC)", scriptCtl != nil},
		{"分类 help + 中文备注 + 高亮", true},
	}
	done := 0
	for _, it := range items {
		if it.Done {
			done++
		}
	}
	percent := int(float64(done) * 100 / float64(len(items)))
	fmt.Printf("服务器当前进度: %d%% (%d/%d)\n", percent, done, len(items))
	for _, it := range items {
		flag := "✗"
		if it.Done {
			flag = "✓"
		}
		fmt.Printf("  [%s] %s\n", flag, it.Name)
	}
}

func printCompatStatus(cfg config.Config, srv *netserver.Server) {
	fmt.Println("原版一致性状态（基于当前 Go 多核服务端）:")
	unitSyncBase := srv != nil && (srv.ExtraEntitySnapshotEntitiesFn != nil || srv.ExtraEntitySnapshotFn != nil)
	items := []struct {
		Name   string
		Status string
	}{
		{"基础握手/连接流程", "已实现"},
		{"地图加载与流发送", "已实现"},
		{"聊天/踢封/OP/基础管理", "已实现"},
		{"API 管理与脚本自动化", "已实现"},
		{"原版参数加载管线(单位/炮塔 profiles.json)", "已实现"},
		{"原版源码提取器(vanilla gen 自动生成profiles)", "已实现"},
		{"单位同步(UnitEntity可见同步+销毁生命周期)", ternary(unitSyncBase, "已实现", "未完成")},
		{"单位战斗最小闭环(自动攻击/伤害/死亡)", "已实现"},
		{"单位多武器挂点(分挂点冷却并行开火)", "已实现"},
		{"目标过滤(空军/地面命中筛选)", "已实现"},
		{"目标优先级与命中体积(碰撞半径)", "已实现"},
		{"目标锁定与转火延迟(anti-jitter)", "已实现"},
		{"建筑最小闭环(受击/销毁广播)", "已实现"},
		{"关键战斗包对齐(buildHealthUpdate/销毁/子弹)", "已实现"},
		{"建筑炮塔攻击(按原版炮塔名匹配开火参数)", "已实现"},
		{"建筑炮塔资源约束(弹药/电力/连发)", "已实现"},
		{"单位同步(全部原版实体行为)", "未完成"},
		{"建筑/逻辑/战斗完整模拟(全机制)", "未完成"},
		{"原版全部网络包语义一致", "未完成"},
	}
	done := 0
	for _, it := range items {
		if it.Status == "已实现" {
			done++
		}
	}
	pct := int(float64(done) * 100 / float64(len(items)))
	fmt.Printf("一致性进度: %d%% (%d/%d)\n", pct, done, len(items))
	fmt.Printf("当前脚本配置文件: %s\n", cfg.Script.File)
	for _, it := range items {
		fmt.Printf("  - %s: %s\n", it.Name, it.Status)
	}
	fmt.Println("说明: 要做到“与原版一模一样”，核心缺口在完整单位/建筑/战斗逻辑模拟与全部包语义对齐。")
}

func ternary(ok bool, yes, no string) string {
	if ok {
		return yes
	}
	return no
}

type statusMonitor struct {
	srv     *netserver.Server
	cfg     config.Config
	engine  *sim.Engine
	start   time.Time
	enabled atomic.Bool
}

func newStatusMonitor(srv *netserver.Server, cfg config.Config, engine *sim.Engine) *statusMonitor {
	m := &statusMonitor{srv: srv, cfg: cfg, engine: engine, start: time.Now()}
	go func() {
		t := time.NewTicker(5 * time.Second)
		defer t.Stop()
		for range t.C {
			if m.enabled.Load() {
				fmt.Println(m.FormatOnce())
			}
		}
	}()
	return m
}

func (m *statusMonitor) Enable()  { m.enabled.Store(true) }
func (m *statusMonitor) Disable() { m.enabled.Store(false) }

func (m *statusMonitor) FormatOnce() string {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	uptime := time.Since(m.start).Truncate(time.Second)
	base := fmt.Sprintf("status: pid=%d uptime=%s goroutines=%d mem=%.1fMB sys=%.1fMB sessions=%d",
		os.Getpid(), uptime, runtime.NumGoroutine(), float64(ms.Alloc)/1024/1024, float64(ms.Sys)/1024/1024, len(m.srv.ListSessions()))
	if m.engine == nil {
		return base
	}
	stats := m.engine.Stats()
	overrun := "ok"
	if stats.Overrun {
		overrun = "overrun"
	}
	last := "n/a"
	if !stats.LastTickTime.IsZero() {
		last = stats.LastTickTime.Format("15:04:05")
	}
	return fmt.Sprintf("%s tick=%d tps=%d last=%s last_dur=%s part=%d work=%d %s",
		base,
		stats.Tick,
		stats.TPS,
		last,
		stats.LastDuration.Truncate(time.Millisecond),
		stats.Partitions,
		stats.TotalWork,
		overrun)
}

func resolveWorldSelection(arg string) (string, error) {
	if strings.EqualFold(arg, "random") {
		return pickRandomWorld()
	}

	trimmed := strings.TrimSpace(arg)
	if trimmed == "" {
		return "", errors.New("地图参数为空")
	}

	lower := strings.ToLower(trimmed)
	if strings.HasSuffix(lower, ".bin") {
		return "", fmt.Errorf("已禁用 .bin 地图，请使用 .msav")
	}
	if strings.HasSuffix(lower, ".msav") || strings.HasSuffix(lower, ".msav.msav") {
		if exists(trimmed) {
			return canonicalRuntimePath(trimmed), nil
		}
		if strings.HasSuffix(lower, ".msav") || strings.HasSuffix(lower, ".msav.msav") {
			base := worldstream.TrimMapName(filepath.Base(trimmed))
			if p, ok, err := findWorldByBaseName(base); err != nil {
				return "", err
			} else if ok {
				return canonicalRuntimePath(p), nil
			}
			for _, candidate := range []string{
				filepath.Join("..", "core", "assets", "maps", "default", base+".msav"),
				filepath.Join("..", "..", "core", "assets", "maps", "default", base+".msav"),
			} {
				if exists(candidate) {
					return canonicalRuntimePath(candidate), nil
				}
			}
		}
		return "", fmt.Errorf("地图文件不存在: %s", trimmed)
	}

	if p, ok, err := findWorldByBaseName(trimmed); err != nil {
		return "", err
	} else if ok {
		return canonicalRuntimePath(p), nil
	}

	for _, candidate := range []string{
		filepath.Join("..", "core", "assets", "maps", "default", trimmed+".msav"),
		filepath.Join("..", "..", "core", "assets", "maps", "default", trimmed+".msav"),
	} {
		if exists(candidate) {
			return canonicalRuntimePath(candidate), nil
		}
	}

	if exists(trimmed) {
		return trimmed, nil
	}
	return "", fmt.Errorf("地图不存在: %s", trimmed)
}

func pickRandomWorld() (string, error) {
	localFiles, err := listWorldFilesRecursive(localWorldRoots())
	if err == nil && len(localFiles) > 0 {
		return canonicalRuntimePath(localFiles[mathrand.New(mathrand.NewSource(time.Now().UnixNano())).Intn(len(localFiles))]), nil
	}
	coreCandidates := []string{
		filepath.Join("..", "core", "assets", "maps", "default", "*.msav"),
		filepath.Join("..", "..", "core", "assets", "maps", "default", "*.msav"),
	}
	for _, g := range coreCandidates {
		files, err := filepath.Glob(g)
		if err == nil && len(files) > 0 {
			return canonicalRuntimePath(files[mathrand.New(mathrand.NewSource(time.Now().UnixNano())).Intn(len(files))]), nil
		}
	}
	return "", errors.New("未找到地图文件（需要 assets/worlds/*.msav 或 core/assets/maps/default/*.msav）")
}

func listWorldMaps() ([]string, error) {
	outSet := map[string]struct{}{}

	for _, g := range []string{
		filepath.Join("..", "core", "assets", "maps", "default", "*.msav"),
		filepath.Join("..", "..", "core", "assets", "maps", "default", "*.msav"),
	} {
		msavFiles, err := filepath.Glob(g)
		if err != nil {
			return nil, err
		}
		for _, f := range msavFiles {
			outSet[worldstream.TrimMapName(filepath.Base(f))] = struct{}{}
		}
	}
	localFiles, err := listWorldFilesRecursive(localWorldRoots())
	if err != nil {
		return nil, err
	}
	for _, f := range localFiles {
		outSet[worldstream.TrimMapName(filepath.Base(f))] = struct{}{}
	}

	out := make([]string, 0, len(outSet))
	for name := range outSet {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func localWorldRoots() []string {
	if len(runtimeWorldRoots) > 0 {
		return append([]string(nil), runtimeWorldRoots...)
	}
	return []string{
		filepath.Join("assets", "worlds"),
		filepath.Join("go-server", "assets", "worlds"),
		filepath.Join("..", "assets", "worlds"),
	}
}

func listWorldFilesRecursive(roots []string) ([]string, error) {
	outSet := make(map[string]struct{})
	out := make([]string, 0, 64)
	for _, root := range roots {
		root = strings.TrimSpace(root)
		if root == "" {
			continue
		}
		st, err := os.Stat(root)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		if !st.IsDir() {
			continue
		}
		err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if !strings.EqualFold(filepath.Ext(path), ".msav") {
				return nil
			}
			clean := filepath.Clean(path)
			if _, ok := outSet[clean]; ok {
				return nil
			}
			outSet[clean] = struct{}{}
			out = append(out, clean)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(out)
	return out, nil
}

func findWorldByBaseName(name string) (string, bool, error) {
	name = strings.ToLower(strings.TrimSpace(name))
	if name == "" {
		return "", false, nil
	}
	files, err := listWorldFilesRecursive(localWorldRoots())
	if err != nil {
		return "", false, err
	}
	for _, f := range files {
		base := strings.ToLower(worldstream.TrimMapName(filepath.Base(f)))
		if base == name {
			return f, true, nil
		}
	}
	return "", false, nil
}

func exists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}

func printIPs(listenAddr string) {
	fmt.Printf("监听地址: %s\n", listenAddr)
	ifaces, err := net.Interfaces()
	if err != nil {
		fmt.Printf("获取网卡失败: %v\n", err)
		return
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok || ipNet.IP == nil {
				continue
			}
			ip := ipNet.IP
			if ip.IsLoopback() {
				continue
			}
			fmt.Printf("IP: %s (%s)\n", ip.String(), iface.Name)
		}
	}
}

func printSelfCheck(listenAddr string, build int, worldPath string, cfg config.Config) {
	fmt.Println("自检（无网络探测模式）:")
	fmt.Printf("  监听地址: %s\n", listenAddr)
	fmt.Printf("  目标版本: %d\n", build)
	fmt.Printf("  当前地图: %s\n", canonicalRuntimePath(worldPath))
	fmt.Printf("  API: enabled=%v bind=%s auth=%v keys=%d\n", cfg.API.Enabled, cfg.API.Bind, len(cfg.API.Keys) > 0, len(cfg.API.Keys))
	fmt.Printf("  Storage: mode=%s db=%v dir=%s\n", cfg.Storage.Mode, cfg.Storage.DatabaseEnabled, canonicalRuntimePath(cfg.Storage.Directory))
	fmt.Printf("  Mods: enabled=%v dir=%s\n", cfg.Mods.Enabled, cfg.Mods.Directory)
	fmt.Printf("  Persist: enabled=%v dir=%s file=%s interval=%ds\n", cfg.Persist.Enabled, canonicalRuntimePath(cfg.Persist.Directory), cfg.Persist.File, cfg.Persist.IntervalSec)
	fmt.Printf("  MSAV snapshot: enabled=%v dir=%s file=%s\n", cfg.Persist.SaveMSAV, canonicalRuntimePath(cfg.Persist.MSAVDir), cfg.Persist.MSAVFile)
	host, port, err := net.SplitHostPort(listenAddr)
	if err != nil {
		fmt.Printf("  端口解析失败: %v\n", err)
		return
	}
	fmt.Printf("  地址解析: host=%s port=%s (仅检查格式)\n", host, port)
	if exists(worldPath) {
		fmt.Println("  地图文件: 正常")
	} else {
		fmt.Println("  地图文件: 不存在或不可读")
	}
	fmt.Println("  网络探测: 已禁用（避免触发连接中断日志）")
}

func printAPIKey(cfg config.Config) {
	fmt.Printf("API 绑定: %s\n", cfg.API.Bind)
	fmt.Printf("API 启用: %v\n", cfg.API.Enabled)
	if len(cfg.API.Keys) == 0 {
		fmt.Println("API Key: 未设置")
		return
	}
	fmt.Printf("API Keys(%d):\n", len(cfg.API.Keys))
	for _, k := range cfg.API.Keys {
		fmt.Printf("  %s\n", k)
	}
}

func resolveUnitTypeArg(arg string, wld *world.World) (int16, string, bool) {
	arg = strings.TrimSpace(arg)
	if arg == "" {
		return 0, "", false
	}
	if v, err := strconv.ParseInt(arg, 10, 16); err == nil {
		typeID := int16(v)
		name := ""
		if wld != nil {
			name = wld.UnitNameByTypeID(typeID)
		}
		if name == "" {
			name = "unknown"
		}
		return typeID, name, true
	}
	if wld == nil {
		return 0, "", false
	}
	if typeID, ok := wld.ResolveUnitTypeID(arg); ok {
		name := wld.UnitNameByTypeID(typeID)
		if name == "" {
			name = strings.ToLower(strings.TrimSpace(arg))
		}
		return typeID, name, true
	}
	return 0, "", false
}

func fallbackSpawnPosFromModel(model *world.WorldModel) (protocol.Point2, bool) {
	if model == nil || model.Width <= 0 || model.Height <= 0 || len(model.Tiles) == 0 {
		return protocol.Point2{}, false
	}
	var firstOwnedBuild protocol.Point2
	firstOwnedBuildOK := false
	for i := range model.Tiles {
		tile := &model.Tiles[i]
		if tile == nil || tile.Build == nil || tile.Block == 0 || tile.Build.Health <= 0 {
			continue
		}
		if tile.Build.X != tile.X || tile.Build.Y != tile.Y {
			continue
		}
		owner := tile.Team
		if tile.Build.Team != 0 {
			owner = tile.Build.Team
		}
		if owner == 0 {
			continue
		}
		if !firstOwnedBuildOK {
			firstOwnedBuild = protocol.Point2{X: int32(tile.X), Y: int32(tile.Y)}
			firstOwnedBuildOK = true
		}
		name := strings.ToLower(strings.TrimSpace(model.BlockNames[int16(tile.Build.Block)]))
		if strings.Contains(name, "core") || strings.Contains(name, "foundation") || strings.Contains(name, "nucleus") {
			return protocol.Point2{X: int32(tile.X), Y: int32(tile.Y)}, true
		}
	}
	if firstOwnedBuildOK {
		return firstOwnedBuild, true
	}
	return protocol.Point2{X: int32(model.Width / 2), Y: int32(model.Height / 2)}, true
}

func resolveRespawnUnitTypeByCoreTile(wld *world.World, tile protocol.Point2, team world.TeamID, fallback int16) int16 {
	if wld == nil {
		return fallback
	}
	if _, coreName, ok := resolveTeamCoreTileWithName(wld, team, tile); ok {
		if unitName, _, ok := coreUnitNameAndRankByBlockName(coreName); ok {
			if unitTypeID, ok := wld.ResolveUnitTypeID(unitName); ok {
				return unitTypeID
			}
		}
	}
	model := wld.Model()
	if model == nil || !model.InBounds(int(tile.X), int(tile.Y)) {
		return fallback
	}
	if t, err := model.TileAt(int(tile.X), int(tile.Y)); err == nil && t != nil && t.Block > 0 {
		if unitName, _, ok := coreUnitNameAndRankByBlockName(model.BlockNames[int16(t.Block)]); ok {
			if unitTypeID, ok := wld.ResolveUnitTypeID(unitName); ok {
				return unitTypeID
			}
		}
	}
	return fallback
}

func coreUnitNameAndRankByBlockName(blockName string) (string, int, bool) {
	name := strings.ToLower(strings.TrimSpace(blockName))
	switch {
	case strings.Contains(name, "core-shard"):
		return "alpha", 1, true
	case strings.Contains(name, "core-foundation"):
		return "beta", 2, true
	case strings.Contains(name, "core-nucleus"):
		return "gamma", 3, true
	case strings.Contains(name, "core-bastion"):
		return "evoke", 1, true
	case strings.Contains(name, "core-citadel"):
		return "incite", 2, true
	case strings.Contains(name, "core-acropolis"):
		return "emanate", 3, true
	default:
		return "", 0, false
	}
}

func resolveTeamCoreTile(wld *world.World, team world.TeamID, ref protocol.Point2) (protocol.Point2, bool) {
	pos, _, ok := resolveTeamCoreTileWithName(wld, team, ref)
	return pos, ok
}

func resolveTeamCoreTileWithName(wld *world.World, team world.TeamID, ref protocol.Point2) (protocol.Point2, string, bool) {
	if wld == nil || team == 0 {
		return protocol.Point2{}, "", false
	}
	model := wld.Model()
	if model == nil || model.Width <= 0 || model.Height <= 0 || len(model.Tiles) == 0 {
		return protocol.Point2{}, "", false
	}
	refX := int(ref.X)
	refY := int(ref.Y)
	if !model.InBounds(refX, refY) {
		refX = model.Width / 2
		refY = model.Height / 2
	}
	bestRank := -1
	bestDist2 := int(^uint(0) >> 1)
	bestPos := protocol.Point2{}
	bestName := ""
	consider := func(t *world.Tile) {
		if t == nil || t.Block <= 0 || t.Build == nil {
			return
		}
		blockName := model.BlockNames[int16(t.Block)]
		_, rank, ok := coreUnitNameAndRankByBlockName(blockName)
		if !ok {
			return
		}
		dx := t.X - refX
		dy := t.Y - refY
		dist2 := dx*dx + dy*dy
		if rank > bestRank || (rank == bestRank && dist2 < bestDist2) {
			bestRank = rank
			bestDist2 = dist2
			bestPos = protocol.Point2{X: int32(t.X), Y: int32(t.Y)}
			bestName = blockName
		}
	}
	for _, packed := range wld.TeamCorePositions(team) {
		x := int(protocol.UnpackPoint2X(packed))
		y := int(protocol.UnpackPoint2Y(packed))
		t, err := model.TileAt(x, y)
		if err != nil {
			continue
		}
		consider(t)
	}
	if bestRank < 0 {
		for i := range model.Tiles {
			t := &model.Tiles[i]
			if t == nil || t.Build == nil || t.Block <= 0 {
				continue
			}
			if t.Build.X != t.X || t.Build.Y != t.Y {
				continue
			}
			owner := t.Team
			if t.Build.Team != 0 {
				owner = t.Build.Team
			}
			if owner != team {
				continue
			}
			consider(t)
		}
	}
	if bestRank < 0 {
		return protocol.Point2{}, "", false
	}
	return bestPos, bestName, true
}

func resolveConnTeam(c *netserver.Conn, wld *world.World) world.TeamID {
	defaultTeam := resolveDefaultPlayerTeam(wld)
	if c == nil {
		return defaultTeam
	}
	if teamID := c.TeamID(); teamID != 0 {
		return world.TeamID(teamID)
	}
	if wld == nil {
		return defaultTeam
	}
	if unitID := c.UnitID(); unitID != 0 {
		if ent, ok := wld.GetEntity(unitID); ok && ent.Team != 0 {
			return ent.Team
		}
	}
	if playerID := c.PlayerID(); playerID != 0 {
		if ent, ok := wld.GetEntity(playerID); ok && ent.Team != 0 {
			return ent.Team
		}
	}
	return defaultTeam
}

func assignConnTeamVanilla(srv *netserver.Server, wld *world.World, c *netserver.Conn) world.TeamID {
	defaultTeam := resolveDefaultPlayerTeam(wld)
	if wld == nil {
		return defaultTeam
	}
	rulesMgr := wld.GetRulesManager()
	if rulesMgr == nil {
		return defaultTeam
	}
	rules := rulesMgr.Get()
	if rules == nil || !rules.Pvp {
		return defaultTeam
	}
	waveTeam := resolveConfiguredTeamID(rules.WaveTeam, world.TeamID(2))
	counts := map[byte]int{}
	if srv != nil {
		counts = srv.ConnectedTeamCounts()
	}
	if c != nil {
		if current := c.TeamID(); counts[current] > 0 {
			counts[current]--
		}
	}
	bestTeam := world.TeamID(0)
	bestCount := int(^uint(0) >> 1)
	for rawTeam := 1; rawTeam <= 255; rawTeam++ {
		team := world.TeamID(rawTeam)
		if team == waveTeam && rules.Waves {
			continue
		}
		if _, ok := resolveTeamCoreTile(wld, team, protocol.Point2{}); !ok {
			continue
		}
		count := counts[byte(team)]
		if count < bestCount || (count == bestCount && (bestTeam == 0 || team < bestTeam)) {
			bestTeam = team
			bestCount = count
		}
	}
	if bestTeam != 0 {
		return bestTeam
	}
	return defaultTeam
}

func resolveDefaultPlayerTeam(wld *world.World) world.TeamID {
	const fallback = world.TeamID(1)
	if wld == nil {
		return fallback
	}
	rulesMgr := wld.GetRulesManager()
	if rulesMgr == nil {
		return fallback
	}
	rules := rulesMgr.Get()
	if rules == nil {
		return fallback
	}
	return resolveConfiguredTeamID(rules.DefaultTeam, fallback)
}

func resolveConfiguredTeamID(value string, fallback world.TeamID) world.TeamID {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "derelict":
		return world.TeamID(0)
	case "sharded":
		return world.TeamID(1)
	case "crux":
		return world.TeamID(2)
	case "malis":
		return world.TeamID(3)
	case "green":
		return world.TeamID(4)
	case "blue":
		return world.TeamID(5)
	default:
		if n, err := strconv.Atoi(strings.TrimSpace(value)); err == nil && n >= 0 && n <= 255 {
			return world.TeamID(n)
		}
		return fallback
	}
}

func resolveBuildOwner(c *netserver.Conn) int32 {
	if c == nil {
		return 0
	}
	if id := c.PlayerID(); id != 0 {
		return id
	}
	return c.ConnID()
}

func hasQueuedBuildPlans(plans []*protocol.BuildPlan) bool {
	for _, plan := range plans {
		if plan != nil {
			return true
		}
	}
	return false
}

func builderSnapshotActive(dead bool, unitID int32, building bool, plans []*protocol.BuildPlan, forceActive bool) bool {
	if dead || unitID == 0 {
		return false
	}
	if forceActive {
		return true
	}
	return building || hasQueuedBuildPlans(plans)
}

func syncBuilderStateFromConnSnapshot(wld *world.World, c *netserver.Conn, owner int32, team world.TeamID, plans []*protocol.BuildPlan, forceActive bool) {
	if wld == nil || c == nil || owner == 0 {
		return
	}
	snapX, snapY := c.SnapshotPos()
	active := builderSnapshotActive(c.IsDead(), c.UnitID(), c.IsBuilding(), plans, forceActive)
	if !active && wld.HasPendingPlansForOwner(owner) {
		active = true
	}
	// UnitType.buildRange defaults to Vars.buildingRange in 157.
	wld.UpdateBuilderState(owner, team, c.UnitID(), snapX, snapY, active, 220)
}

// getSpawnPos 获取重生点位置（支持多核心轮转）
func getSpawnPos(model *world.WorldModel, cache *worldCache, path string) (protocol.Point2, bool) {
	// 1. 优先使用核心位置缓存
	if pos, ok, err := spawnPosFromCache(cache, path); err == nil && ok {
		return pos, true
	}
	// 2. 回退到模型中的重生点
	return fallbackSpawnPosFromModel(model)
}

// spawnPosFromCache 从缓存获取重生点
func spawnPosFromCache(cache *worldCache, path string) (protocol.Point2, bool, error) {
	if cache == nil {
		return protocol.Point2{}, false, nil
	}
	return cache.spawnPos(path)
}

type worldCache struct {
	mu        sync.Mutex
	path      string
	modTime   time.Time
	data      []byte
	baseModel *world.WorldModel
	corePos   protocol.Point2
	corePosOK bool
	backend   *coreio.Core3
	content   *protocol.ContentRegistry
}

func (c *worldCache) invalidate() {
	if c != nil && c.backend != nil {
		_ = c.backend.InvalidateWorldCache("")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.path = ""
	c.modTime = time.Time{}
	c.data = nil
	c.baseModel = nil
	c.corePos = protocol.Point2{}
	c.corePosOK = false
}

func isMSAVPath(path string) bool {
	lower := strings.ToLower(strings.TrimSpace(path))
	return strings.HasSuffix(lower, ".msav") || strings.HasSuffix(lower, ".msav.msav")
}

func (c *worldCache) get(path string) ([]byte, error) {
	if c != nil && c.backend != nil && !isMSAVPath(path) {
		if res, err := c.backend.GetWorldCache(path); err == nil {
			if _, inspectErr := worldstream.InspectWorldStreamPayload(res.Data); inspectErr == nil {
				c.mu.Lock()
				c.path = canonicalRuntimePath(path)
				if info, statErr := os.Stat(resolveRuntimePath(path)); statErr == nil {
					c.modTime = info.ModTime()
				}
				c.data = append([]byte(nil), res.Data...)
				c.baseModel = res.BaseModel
				c.corePos = res.CorePos
				c.corePosOK = res.CorePosOK
				c.mu.Unlock()
				return res.Data, nil
			}
		}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	canonicalPath := canonicalRuntimePath(path)
	actualPath := resolveRuntimePath(canonicalPath)

	info, err := os.Stat(actualPath)
	if err != nil {
		return nil, err
	}
	if c.path == canonicalPath && c.modTime.Equal(info.ModTime()) && len(c.data) > 0 {
		return c.data, nil
	}

	data, err := loadWorldStream(canonicalPath, c.content)
	if err != nil {
		return nil, err
	}
	if _, inspectErr := worldstream.InspectWorldStreamPayload(data); inspectErr != nil {
		return nil, fmt.Errorf("inspect local worldstream payload: %w", inspectErr)
	}
	c.path = canonicalPath
	c.modTime = info.ModTime()
	c.data = data
	c.baseModel = nil
	c.corePosOK = false
	if isMSAVPath(canonicalPath) {
		if model, merr := worldstream.LoadWorldModelFromMSAV(actualPath, c.content); merr == nil {
			c.baseModel = model
		}
		if pos, ok, err := worldstream.FindCoreTileFromMSAV(actualPath); err == nil {
			c.corePos = pos
			c.corePosOK = ok
		}
	}
	return data, nil
}

func (c *worldCache) spawnPos(path string) (protocol.Point2, bool, error) {
	if _, err := c.get(path); err != nil {
		return protocol.Point2{}, false, err
	}
	return c.corePos, c.corePosOK, nil
}

func (c *worldCache) model(path string) *world.WorldModel {
	if c == nil {
		return nil
	}
	if _, err := c.get(path); err != nil {
		return nil
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.baseModel
}

func buildInitialWorldDataPayload(conn *netserver.Conn, wld *world.World, cache *worldCache, path string) ([]byte, error) {
	if cache == nil {
		return nil, fmt.Errorf("world cache unavailable")
	}
	if conn != nil {
		conn.SetLiveWorldStream(false)
	}
	playerID := int32(1)
	if conn != nil && conn.PlayerID() != 0 {
		playerID = conn.PlayerID()
	}
	base, err := cache.get(path)
	if err == nil && len(base) > 0 {
		payload := base
		if wld == nil {
			return payload, nil
		}

		snap := wld.Snapshot()
		if patched, perr := worldstream.RewriteRuntimeStateInWorldStream(payload, snap.Wave, snap.WaveTime*60, float64(snap.Tick), playerID); perr == nil {
			payload = patched
		} else if conn != nil && conn.PlayerID() != 0 {
			if patched, perr := worldstream.RewritePlayerIDInWorldStream(payload, conn.PlayerID()); perr == nil {
				payload = patched
			}
		}
		if patched, perr := worldstream.RewriteRulesInWorldStream(payload, buildRuntimeRulesRaw(wld, path)); perr == nil {
			payload = patched
		}
		return payload, nil
	}

	if wld != nil {
		snap := wld.Snapshot()
		if liveModel := wld.CloneModelForWorldStream(); liveModel != nil {
			if payload, lerr := worldstream.BuildWorldStreamFromModelSnapshot(liveModel, playerID, snap); lerr == nil && len(payload) > 0 {
				if patched, perr := worldstream.RewriteRulesInWorldStream(payload, buildRuntimeRulesRaw(wld, path)); perr == nil {
					payload = patched
				}
				if _, inspectErr := worldstream.InspectWorldStreamPayload(payload); inspectErr == nil {
					if conn != nil {
						conn.SetLiveWorldStream(true)
					}
					return payload, nil
				}
			}
		}
		if baseModel := wld.Model(); baseModel != nil {
			if payload, lerr := worldstream.BuildWorldStreamFromModelSnapshot(baseModel.Clone(), playerID, snap); lerr == nil && len(payload) > 0 {
				if patched, perr := worldstream.RewriteRulesInWorldStream(payload, buildRuntimeRulesRaw(wld, path)); perr == nil {
					payload = patched
				}
				if _, inspectErr := worldstream.InspectWorldStreamPayload(payload); inspectErr == nil {
					return payload, nil
				}
			}
		}
	}

	return nil, fmt.Errorf("build initial world payload: cache=%v", err)
}

func loadWorldStream(path string, content *protocol.ContentRegistry) ([]byte, error) {
	actualPath := resolveRuntimePath(path)
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".msav") || strings.HasSuffix(lower, ".msav.msav") {
		if payload, err := worldstream.BuildWorldStreamFromMSAV(actualPath); err == nil && len(payload) > 0 {
			return payload, nil
		}
		if model, err := worldstream.LoadWorldModelFromMSAV(actualPath, content); err == nil && model != nil {
			if payload, berr := worldstream.BuildWorldStreamFromModel(model, 1); berr == nil && len(payload) > 0 {
				return payload, nil
			}
		}
		return worldstream.BuildWorldStreamFromMSAV(actualPath)
	}
	data, err := os.ReadFile(actualPath)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func warmWorldCache(cache *worldCache, path string) error {
	if cache == nil {
		return nil
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	_, err := cache.get(path)
	return err
}

func loadBootstrapWorldFallback() ([]byte, error) {
	data, _, err := runtimeassets.LoadBootstrapWorld(runtimeAssetsDir)
	return data, err
}
