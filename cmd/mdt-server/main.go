package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"math"
	mathrand "math/rand"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mdt-server/internal/api"
	"mdt-server/internal/bootstrap"
	"mdt-server/internal/buildinfo"
	"mdt-server/internal/buildsvc"
	"mdt-server/internal/config"
	coreio "mdt-server/internal/core"
	"mdt-server/internal/devlog"
	"mdt-server/internal/logging"
	"mdt-server/internal/mods/java"
	netserver "mdt-server/internal/net"
	"mdt-server/internal/persist"
	"mdt-server/internal/protocol"
	"mdt-server/internal/sim"
	"mdt-server/internal/storage"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

type worldState struct {
	mu      sync.RWMutex
	current string
}

var runtimePlayerNameColorEnabled atomic.Bool
var runtimePublicConnUUIDEnabled atomic.Bool
var runtimeJoinLeaveChatEnabled atomic.Bool
var runtimePlayerNamePrefix atomic.Value
var runtimePlayerNameSuffix atomic.Value
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

func publicConnIDValue(store *persist.PublicConnUUIDStore, uuid string, connID int32) string {
	if !runtimePublicConnUUIDEnabled.Load() {
		return strconv.FormatInt(int64(connID), 10)
	}
	uuid = strings.TrimSpace(uuid)
	if uuid != "" && store != nil {
		if id, ok := store.Lookup(uuid); ok && strings.TrimSpace(id) != "" {
			return strings.TrimSpace(id)
		}
	}
	return strconv.FormatInt(int64(connID), 10)
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

func main() {
	cfgPath := flag.String("config", filepath.FromSlash("configs/config.ini"), "path to config file")
	addr := flag.String("addr", "0.0.0.0:6567", "listen address for Mindustry protocol (TCP+UDP)")
	buildVersion := flag.Int("build", 156, "Mindustry build version for strict check; must match client build")
	worldArg := flag.String("world", "random", "world source: random | <map-name> | <.msav file path>")
	printVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *printVersion {
		fmt.Printf("mdt-server %s (%s)\n", buildinfo.Version, buildinfo.Commit)
		return
	}
	if *buildVersion <= 0 {
		fmt.Fprintln(os.Stderr, "build 必须设置为客户端对应的 build 号，例如 156")
		os.Exit(1)
	}

	cfg := config.Default()
	cfg.Source = *cfgPath
	if loaded, err := config.Load(cfg.Source); err != nil {
		fmt.Fprintf(os.Stderr, "读取配置失败: %v\n", err)
		os.Exit(1)
	} else {
		cfg = loaded
		cfg.Source = *cfgPath
	}
	configDir := filepath.Dir(cfg.Source)
	if strings.TrimSpace(configDir) == "" {
		configDir = filepath.FromSlash("configs")
	}
	rootDir := filepath.Dir(configDir)
	if strings.TrimSpace(rootDir) == "" || rootDir == configDir {
		rootDir = "."
	}
	// 非配置类目录/文件全部以 EXE 根目录为基准生成；configs 只存放配置文件。
	config.ApplyBaseDir(&cfg, rootDir)
	detailLog, detailLogErr := newDetailedLogWriter(cfg.Runtime.LogsDir, cfg.Sundries.DetailedLogMaxMB, cfg.Sundries.DetailedLogMaxFiles)
	if detailLogErr != nil {
		fmt.Fprintf(os.Stderr, "初始化 logs 详细日志失败: %v\n", detailLogErr)
		os.Exit(1)
	}
	runtimeConfigDir = configDir
	runtimeAssetsDir = cfg.Runtime.AssetsDir
	runtimeWorldRoots = []string{cfg.Runtime.WorldsDir}
	applyBlockNameTranslations(configDir)

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
		return config.Save(cfg.Source, cfg)
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

	fmt.Fprintf(os.Stdout, "mdt-server %s (%s)\n", buildinfo.Version, buildinfo.Commit)
	fmt.Fprintf(os.Stdout, "config=%s cores=%d addr=%s build=%d world=%s vanilla=%s\n", cfg.Source, cfg.Runtime.Cores, *addr, *buildVersion, *worldArg, cfg.Runtime.VanillaProfiles)
	if len(bootstrapResult.CreatedDirs) > 0 || len(bootstrapResult.CreatedFiles) > 0 {
		fmt.Fprintf(os.Stdout, "workspace initialized: dirs=%d files=%d\n", len(bootstrapResult.CreatedDirs), len(bootstrapResult.CreatedFiles))
	}

	// 初始化开发者日志
	devLog := devlog.New(os.Stdout)
	devLog.SetLevel(devlog.LogLevelDebug)
	if cfg.Runtime.DevLogEnabled {
		startup.ok("开发者日志", "已启用")
	} else {
		startup.info("开发者日志", "未启用")
	}

	modMgr := java.New(cfg.Mods)
	if cfg.Mods.Enabled {
		if err := modMgr.Scan(); err != nil {
			log.Error("mods scan failed", logging.Field{Key: "error", Value: err.Error()})
			startup.fail("Mods 扫描", err.Error())
		} else if err := modMgr.Start(); err != nil {
			log.Warn("mods start", logging.Field{Key: "error", Value: err.Error()})
			startup.warn("Mods 启动", err.Error())
		} else {
			startup.ok("Mods 加载", fmt.Sprintf("count=%d", len(modMgr.Mods())))
		}
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
	state := &worldState{current: initialWorld}
	runtimePlayerNameColorEnabled.Store(cfg.Personalization.PlayerNameColorEnabled)
	runtimeJoinLeaveChatEnabled.Store(cfg.Personalization.JoinLeaveChatEnabled)
	runtimePlayerNamePrefix.Store(cfg.Personalization.PlayerNamePrefix)
	runtimePlayerNameSuffix.Store(cfg.Personalization.PlayerNameSuffix)
	if cfg.Personalization.StartupCurrentMapLineEnabled {
		fmt.Fprintf(os.Stdout, "当前地图: %s\n", initialWorld)
	}

	srv := netserver.NewServer(*addr, *buildVersion)
	srv.SetVerboseNetLog(false)
	srv.SetPacketRecvEventsEnabled(cfg.Development.PacketRecvEventsEnabled)
	srv.SetPacketSendEventsEnabled(cfg.Development.PacketSendEventsEnabled)
	srv.SetTerminalPlayerLogsEnabled(cfg.Development.TerminalPlayerLogsEnabled)
	srv.SetRespawnPacketLogsEnabled(cfg.Development.RespawnPacketLogsEnabled)
	srv.SetPlayerNameColorEnabled(cfg.Personalization.PlayerNameColorEnabled)
	srv.SetTranslatedConnLog(cfg.Control.TranslatedConnLogEnabled)
	srv.SetJoinLeaveChatEnabled(cfg.Personalization.JoinLeaveChatEnabled)
	srv.SetPlayerDisplayFormatter(func(c *netserver.Conn) string {
		if c == nil {
			return ""
		}
		return formatDisplayPlayerNameRaw(c.Name())
	})
	srv.RefreshPlayerDisplayNames()
	contentIDsPath := filepath.Join(filepath.Dir(cfg.Runtime.VanillaProfiles), "content_ids.json")
	if ids, err := vanilla.LoadContentIDs(contentIDsPath); err != nil {
		startup.warn("原版 content IDs", fmt.Sprintf("未加载(%s): %v", contentIDsPath, err))
	} else {
		count := vanilla.ApplyContentIDs(srv.Content, ids)
		startup.ok("原版 content IDs", fmt.Sprintf("entries=%d path=%s", count, contentIDsPath))
	}
	srv.SetServerName(cfg.Runtime.ServerName)
	srv.SetServerDescription(cfg.Runtime.ServerDesc)
	srv.SetVirtualPlayers(int32(cfg.Runtime.VirtualPlayers))
	srv.UdpRetryCount = cfg.Net.UdpRetryCount
	srv.UdpRetryDelay = time.Duration(cfg.Net.UdpRetryDelayMs) * time.Millisecond
	srv.UdpFallbackTCP = cfg.Net.UdpFallbackTCP
	srv.SetSnapshotIntervals(cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs)
	wld := world.New(world.Config{TPS: sim.DefaultTPS})
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
		_ = ent
		return x, y, true
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
	var unitNamesByID map[int16]string
	var loadedModel *world.WorldModel
	var loadedMapPath string

	var playerSpawnTypeID int32 = int32(defaultPlayerRespawnUnitID)
	if err := wld.LoadVanillaProfiles(cfg.Runtime.VanillaProfiles); err != nil {
		log.Warn("vanilla profiles load failed", logging.Field{Key: "path", Value: cfg.Runtime.VanillaProfiles}, logging.Field{Key: "error", Value: err.Error()})
		startup.warn("原版 profiles", fmt.Sprintf("加载失败: %s", err.Error()))
	} else if strings.TrimSpace(cfg.Runtime.VanillaProfiles) != "" {
		startup.ok("原版 profiles", cfg.Runtime.VanillaProfiles)
	}
	loadWorldModel := func(path string) {
		buildService.Reset()
		lower := strings.ToLower(path)
		if !strings.HasSuffix(lower, ".msav") && !strings.HasSuffix(lower, ".msav.msav") {
			wld.SetModel(nil)
			loadedModel = nil
			loadedMapPath = ""
			return
		}
		model, lerr := worldstream.LoadWorldModelFromMSAV(path, srv.Content)
		if lerr != nil {
			log.Warn("world model load failed", logging.Field{Key: "path", Value: path}, logging.Field{Key: "error", Value: lerr.Error()})
			startup.warn("地图模型", fmt.Sprintf("加载失败: %s", lerr.Error()))
			loadedModel = nil
			loadedMapPath = ""
			return
		}
		wld.SetModel(model)
		loadedModel = model
		loadedMapPath = path
		startup.ok("地图模型", fmt.Sprintf("%s (%dx%d)", path, model.Width, model.Height))
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
		wld.SetTeamBuilderSpeed(world.TeamID(1), wld.BuilderSpeedForUnitType(spawnType))
		startup.ok("玩家出生单位", fmt.Sprintf("typeId=%d", spawnType))
	}
	if persistedOK {
		waveTime := persisted.WaveTime
		if waveTime < 0 || waveTime > 3600 {
			waveTime = 600
		}
		wld.ApplySnapshot(world.Snapshot{
			WaveTime: waveTime,
			Wave:     persisted.Wave,
			TimeData: persisted.TimeData,
			Tps:      int8(sim.DefaultTPS),
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
	publicConnUUIDStore, publicConnUUIDErr := persist.NewPublicConnUUIDStore(publicConnUUIDPath)
	if publicConnUUIDErr != nil {
		log.Warn("public conn_uuid store init failed", logging.Field{Key: "error", Value: publicConnUUIDErr.Error()})
		startup.warn("公开 conn_uuid", publicConnUUIDErr.Error())
	} else {
		startup.ok("公开 conn_uuid", publicConnUUIDPath)
	}
	srv.OnEvent = func(ev netserver.NetEvent) {
		if publicConnUUIDStore != nil && ev.Kind == "connect_packet" && runtimePublicConnUUIDEnabled.Load() {
			_, _ = publicConnUUIDStore.Ensure(ev.UUID, ev.Name, ev.IP)
		}
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

	cache := &worldCache{}
	srv.WorldDataFn = func(conn *netserver.Conn, _ *protocol.ConnectPacket) ([]byte, error) {
		path := state.get()
		base, err := cache.get(path)
		if err != nil {
			return nil, err
		}
		snap := wld.Snapshot()
		playerID := int32(1)
		if conn != nil && conn.PlayerID() != 0 {
			playerID = conn.PlayerID()
		}
		if patched, perr := worldstream.RewriteRuntimeStateInWorldStream(base, snap.Wave, snap.WaveTime*60, float64(snap.Tick), playerID); perr == nil {
			return patched, nil
		}
		if conn != nil && conn.PlayerID() != 0 {
			if patched, perr := worldstream.RewritePlayerIDInWorldStream(base, conn.PlayerID()); perr == nil {
				return patched, nil
			}
		}
		return base, nil
	}
	srv.OnPostConnect = func(conn *netserver.Conn) {
		if conn == nil {
			return
		}
		// Wait until client applies world stream, then patch runtime build states.
		time.Sleep(350 * time.Millisecond)
		syncCurrentWorldToConn(conn, wld)
	}
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		if pos, ok := resolveTeamCoreTile(wld, world.TeamID(1), protocol.Point2{}); ok {
			return pos, true
		}
		pos, ok, err := cache.spawnPos(state.get())
		if err == nil && ok {
			return pos, true
		}
		// Fallback for maps where core tile cannot be parsed from msav metadata.
		return fallbackSpawnPosFromModel(wld.Model())
	}
	// Official 156 build authority comes from clientSnapshot/clientPlanSnapshot queues.
	// Do not feed legacy/compat beginPlace packets into a second custom queue, otherwise
	// cancelled plans continue building long after the client has removed them.
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
		teamActorMu   sync.Mutex
		teamActorBy   = make(map[world.TeamID]string)
	)
	srv.OnBuildPlanSnapshot = func(c *netserver.Conn, plans []*protocol.BuildPlan) {
		if c == nil {
			return
		}
		owner := resolveBuildOwner(c)
		team := resolveConnTeam(c, wld)
		teamActorMu.Lock()
		teamActorBy[team] = displayPlayerName(c)
		teamActorMu.Unlock()
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
		if shouldLogBuildSnapshots() && !cfg.Building.Translated {
			fmt.Printf("[buildtrace] recv removeQueue player=%d remote=%s xy=(%d,%d) breaking=%v\n", c.PlayerID(), c.RemoteAddr().String(), x, y, breaking)
		}
		buildService.CancelPositions(owner, []int32{protocol.PackPoint2(x, y)})
		wld.CancelBuildAtForOwner(owner, x, y, breaking)
	}
	srv.OnTileConfig = func(c *netserver.Conn, pos int32, value any) {
		wld.ConfigureBuildingPacked(pos, value)
		if normalized, ok := wld.BuildingConfigPacked(pos); ok {
			srv.BroadcastTileConfig(pos, normalized, c)
			return
		}
		srv.BroadcastTileConfig(pos, value, c)
	}
	srv.PlayerUnitTypeFn = func() int16 {
		return int16(atomic.LoadInt32(&playerSpawnTypeID))
	}
	srv.StateSnapshotFn = func() *protocol.Remote_NetClient_stateSnapshot_35 {
		snap := wld.Snapshot()
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
	srv.ExtraEntitySnapshotFn = func(w *protocol.Writer) (int16, error) {
		// Disabled for now: full-map entity sync can exceed TypeIO int16 byte-array limits
		// and corrupt packet decoding on official clients.
		_ = w
		return 0, nil
	}
	go func() {
		t := time.NewTicker(time.Second / time.Duration(sim.DefaultTPS))
		defer t.Stop()
		for range t.C {
			evs := wld.DrainEntityEvents()
			buildHealth := make([]int32, 0, len(evs)*2)
			for _, ev := range evs {
				switch ev.Kind {
				case world.EntityEventRemoved:
					broadcastUnitDestroy(srv, ev.Entity.ID)
					srv.MarkUnitDead(ev.Entity.ID, "world-removed")
				case world.EntityEventBuildPlaced:
					x, y := unpackTilePos(ev.BuildPos)
					if shouldFileLogBuildPlace() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=placed x=%d y=%d block_id=%d block=%s team=%d rot=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam, ev.BuildRot))
					}
					if shouldLogBuildPlace() {
						if cfg.Building.Translated {
							teamActorMu.Lock()
							actor := teamActorBy[ev.BuildTeam]
							teamActorMu.Unlock()
							if strings.TrimSpace(actor) == "" {
								actor = fmt.Sprintf("team-%d", ev.BuildTeam)
							}
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
							teamActorMu.Lock()
							actor := teamActorBy[ev.BuildTeam]
							teamActorMu.Unlock()
							if strings.TrimSpace(actor) == "" {
								actor = fmt.Sprintf("team-%d", ev.BuildTeam)
							}
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 完成建造 block=%d(%s) team=%d rot=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam, ev.BuildRot)
						} else {
							fmt.Printf("[buildtrace] constructed xy=(%d,%d) block=%d team=%d rot=%d\n", x, y, ev.BuildBlock, ev.BuildTeam, ev.BuildRot)
						}
					}
					broadcastSetTile(srv, ev.BuildPos, ev.BuildBlock, ev.BuildRot, byte(ev.BuildTeam))
					broadcastConstructFinish(srv, ev.BuildPos, ev.BuildBlock, ev.BuildRot, byte(ev.BuildTeam))
					if cfgValue, ok := wld.BuildingConfigPacked(ev.BuildPos); ok {
						srv.BroadcastTileConfig(ev.BuildPos, cfgValue, nil)
					} else if ev.BuildConfig != nil {
						srv.BroadcastTileConfig(ev.BuildPos, ev.BuildConfig, nil)
					}
				case world.EntityEventBuildDeconstructing:
					x, y := unpackTilePos(ev.BuildPos)
					if shouldFileLogBuildBreakStart() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=deconstructing x=%d y=%d block_id=%d block=%s team=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam))
					}
					if shouldLogBuildBreakStart() {
						if cfg.Building.Translated {
							teamActorMu.Lock()
							actor := teamActorBy[ev.BuildTeam]
							teamActorMu.Unlock()
							if strings.TrimSpace(actor) == "" {
								actor = fmt.Sprintf("team-%d", ev.BuildTeam)
							}
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 正在拆除 block=%d(%s) team=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam)
						} else {
							fmt.Printf("[buildtrace] deconstructing xy=(%d,%d) block=%d team=%d\n", x, y, ev.BuildBlock, ev.BuildTeam)
						}
					}
					broadcastBuildDeconstructBegin(srv, ev.BuildPos, byte(ev.BuildTeam))
				case world.EntityEventBuildDestroyed:
					x, y := unpackTilePos(ev.BuildPos)
					if shouldFileLogBuildBreakDone() {
						detailLog.LogLine(fmt.Sprintf("%s [BUILD] action=destroyed x=%d y=%d block_id=%d block=%s team=%d",
							time.Now().Format(time.RFC3339Nano), x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam))
					}
					if shouldLogBuildBreakDone() {
						if cfg.Building.Translated {
							teamActorMu.Lock()
							actor := teamActorBy[ev.BuildTeam]
							teamActorMu.Unlock()
							if strings.TrimSpace(actor) == "" {
								actor = fmt.Sprintf("team-%d", ev.BuildTeam)
							}
							fmt.Printf("[建筑] 玩家=%s (x%d-y%d) 拆除了 block=%d(%s) team=%d\n", actor, x, y, ev.BuildBlock, blockDisplayName(wld, ev.BuildBlock), ev.BuildTeam)
						} else {
							fmt.Printf("[buildtrace] destroyed xy=(%d,%d) block=%d team=%d\n", x, y, ev.BuildBlock, ev.BuildTeam)
						}
					}
					broadcastSetTile(srv, ev.BuildPos, 0, 0, 0)
					broadcastBuildDestroyed(srv, ev.BuildPos, ev.BuildBlock)
				case world.EntityEventBuildHealth:
					buildHealth = append(buildHealth, ev.BuildPos, int32(math.Float32bits(ev.BuildHP)))
				case world.EntityEventTeamItems:
					positions := wld.TeamItemSyncPositions(ev.BuildTeam)
					if len(positions) > 0 {
						broadcastSetTileItems(srv, int16(ev.ItemID), ev.ItemAmount, positions)
					}
				case world.EntityEventBulletFired:
					broadcastBulletCreate(srv, ev.Bullet)
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
		}
	}()
	saveState := func() {}
	srv.OnChat = func(c *netserver.Conn, msg string) bool {
		if c != nil && strings.TrimSpace(msg) != "" && shouldFileLogChat() {
			detailLog.LogLine(fmt.Sprintf("%s [CHAT] from=%q player_id=%d uuid=%s ip=%s msg=%q",
				time.Now().Format(time.RFC3339Nano), c.Name(), c.PlayerID(), c.UUID(), c.RemoteAddr().String(), strings.TrimSpace(msg)))
		}
		switch strings.TrimSpace(msg) {
		case "/help":
			sendChatHelp(srv, c, cfg)
			return true
		case "/status":
			srv.SendStatusTo(c)
			return true
		}
		if strings.EqualFold(strings.TrimSpace(msg), "/stop") {
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
		if strings.HasPrefix(strings.TrimSpace(msg), "/summon ") {
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
		if strings.HasPrefix(strings.TrimSpace(msg), "/despawn ") {
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
		if strings.EqualFold(strings.TrimSpace(msg), "/kill") {
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
		if strings.HasPrefix(strings.TrimSpace(msg), "/umove ") {
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
		if strings.HasPrefix(strings.TrimSpace(msg), "/uteleport ") {
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
		if strings.HasPrefix(strings.TrimSpace(msg), "/ulife ") {
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
		if strings.HasPrefix(strings.TrimSpace(msg), "/ufollow ") {
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
		if strings.HasPrefix(strings.TrimSpace(msg), "/upatrol ") {
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
		if strings.HasPrefix(strings.TrimSpace(msg), "/ubehavior ") {
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
			time.Second/time.Duration(sim.DefaultTPS),
			coreio.Config{
				Name:          "io-core",
				MessageBuf:    30000,
				WorkerCount:   ioWorkers,
				VerboseNetLog: false,
			},
			cfg.Persist,
		)
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
		})

		netCore := netserver.NewNetworkCoreWithCore(srv, serverCore.Core2)
		netCore.SetServerCore(serverCore)
		netCore.SetRecorder(recorder)
		srv.OnConnOpen = netCore.ConnectionOpen
		srv.OnConnClose = func(c *netserver.Conn) {
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
				if unitID := c.UnitID(); unitID != 0 {
					wld.RemoveEntity(unitID)
				}
				wld.CancelBuildPlansByOwner(resolveBuildOwner(c))
			}
			netCore.ConnectionClose(c)
		}
		srv.OnPacketDecoded = func(c *netserver.Conn, obj any, err error) bool {
			if err != nil {
				// Let server.handleConn run its normal close/error path to avoid duplicate close events.
				return false
			}
			netCore.ProcessPacket(c, obj, nil)
			return true
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
			if unitID := c.UnitID(); unitID != 0 {
				wld.RemoveEntity(unitID)
			}
			wld.CancelBuildPlansByOwner(resolveBuildOwner(c))
		}
		go func() {
			interval := time.Second / time.Duration(sim.DefaultTPS)
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
			if serverCore != nil {
				ch := make(chan coreio.PersistenceResult, 1)
				serverCore.SendToCore2(&coreio.PersistenceMessage{
					Action:     "save_state",
					Path:       state.get(),
					ResultChan: ch,
				})
				if res := <-ch; res.Error != nil {
					fmt.Printf("persist save_state failed: %v\n", res.Error)
				}
			} else {
				snap := wld.Snapshot()
				_ = persist.Save(cfg.Persist, persist.State{
					MapPath:  state.get(),
					WaveTime: snap.WaveTime,
					Wave:     snap.Wave,
					Tick:     snap.Tick,
					TimeData: snap.TimeData,
					Rand0:    snap.Rand0,
					Rand1:    snap.Rand1,
				})
			}
			snap := wld.Snapshot()
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
	var apiSrv *api.Server
	if cfg.API.Enabled {
		var statsFn func() *sim.TickStats
		if engine != nil {
			statsFn = func() *sim.TickStats {
				st := engine.Stats()
				return &st
			}
		}
		apiSrv = api.New(cfg.API, srv, statsFn)
		go func() {
			if err := apiSrv.Serve(); err != nil {
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
			if reason != "" {
				fmt.Println(reason)
			}
			saveState()
			saveOps()
			_ = detailLog.Close()
			_ = recorder.Close()
			os.Exit(0)
		})
	}
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
			srv.SetRespawnPacketLogsEnabled(loaded.Development.RespawnPacketLogsEnabled)
			srv.SetPlayerNameColorEnabled(loaded.Personalization.PlayerNameColorEnabled)
			srv.SetTranslatedConnLog(loaded.Control.TranslatedConnLogEnabled)
			srv.SetJoinLeaveChatEnabled(loaded.Personalization.JoinLeaveChatEnabled)
			runtimeJoinLeaveChatEnabled.Store(loaded.Personalization.JoinLeaveChatEnabled)
			runtimePlayerNamePrefix.Store(loaded.Personalization.PlayerNamePrefix)
			runtimePlayerNameSuffix.Store(loaded.Personalization.PlayerNameSuffix)
			srv.RefreshPlayerDisplayNames()
			runtimePublicConnUUIDEnabled.Store(loaded.Control.PublicConnUUIDEnabled)
			applyBlockNameTranslations(configDir)
			if serverCore != nil && serverCore.Core2 != nil {
				serverCore.Core2.SetVerboseNetLog(false)
			}
			if loaded.Control.PublicConnUUIDFile != cfg.Control.PublicConnUUIDFile && cfg.Control.ReloadLogEnabled {
				fmt.Printf("[config] public_conn_uuid_file changed to %s, restart required to reopen mapping file\n", loaded.Control.PublicConnUUIDFile)
			}

			if apiSrv != nil {
				applyAPIKeySet(apiSrv, loaded.API.Keys)
			}
			if loaded.API.Enabled != cfg.API.Enabled || loaded.API.Bind != cfg.API.Bind {
				if cfg.Control.ReloadLogEnabled {
					fmt.Printf("[config] api enabled/bind changed (enabled=%v bind=%s), restart required to apply\n", loaded.API.Enabled, loaded.API.Bind)
				}
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
	go runConsole(srv, state, modMgr, apiSrv, scriptCtl, *addr, *buildVersion, &cfg, saveConfig, saveScript, recorder, monitor, saveOps, loadWorldModel, reloadVanillaProfiles, reloadVanillaContentIDs, removeEntityByID, setEntityMotion, setEntityPos, setEntityLife, setEntityFollow, setEntityPatrol, clearEntityBehavior, stopServer)
	if serverCore != nil {
		go func() {
			if err := srv.Serve(); err != nil {
				fmt.Fprintf(os.Stderr, "服务器启动失败: %v\n", err)
				os.Exit(1)
			}
		}()
		serverCore.Core1.Run(time.Second / time.Duration(sim.DefaultTPS))
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
	s.current = path
}

func (s *worldState) get() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.current
}

func runConsole(
	srv *netserver.Server,
	state *worldState,
	modMgr *java.Manager,
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
) {
	sc := bufio.NewScanner(os.Stdin)
	name, _, _ := srv.ServerMeta()
	printConsoleIntro(name, state.get(), listenAddr, cfg.API.Bind, cfg.API.Enabled, cfg.Personalization)
	if cfg.Personalization.StartupHelpEnabled {
		printHelp(*cfg)
	}
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
			mods := modMgr.Mods()
			if len(mods) == 0 {
				fmt.Println("当前无 Java 模组（jar）")
				continue
			}
			for _, m := range mods {
				fmt.Printf("mod name=%s size=%d path=%s\n", m.Name, m.Size, m.Path)
			}
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
			fmt.Printf("当前地图: %s\n", state.get())
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
			fmt.Printf("地图已切换: %s\n", next)
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
			_ = recorder.Close()
			os.Exit(0)
		case "quit":
			fmt.Println("直接退出服务器（不保存）")
			_ = recorder.Close()
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
				fmt.Printf("storage: %s (db_enabled=%v mode=%s dir=%s dsn=%q)\n", recorder.Status(), cfg.Storage.DatabaseEnabled, cfg.Storage.Mode, cfg.Storage.Directory, cfg.Storage.DSN)
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
				fmt.Printf("sync: entity=%dms state=%dms (config entity=%dms state=%dms)\n", entityMs, stateMs, cfg.Net.SyncEntityMs, cfg.Net.SyncStateMs)
				continue
			}
			switch strings.ToLower(parts[1]) {
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
				fmt.Println("用法: sync status | sync entity <ms> | sync state <ms> | sync set <entityMs> <stateMs> | sync default")
			}
		case "vanilla":
			if len(parts) == 1 || strings.EqualFold(parts[1], "status") {
				fmt.Printf("vanilla profiles: %s\n", cfg.Runtime.VanillaProfiles)
				fmt.Printf("vanilla content ids: %s\n", filepath.Join(filepath.Dir(cfg.Runtime.VanillaProfiles), "content_ids.json"))
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
				fmt.Printf("vanilla profiles 已加载: %s\n", path)
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
				fmt.Printf("vanilla profiles 生成并加载完成: units_by_name=%d turrets=%d blocks=%d path=%s\n", units, turrets, blocks, out)
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
					fmt.Printf("vanilla content ids 已加载: %s\n", path)
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
		printHelpCmd("host random", "切换到随机地图")
		printHelpCmd("host <map-name>", "切换到 core/assets/maps/default/<map-name>.msav")
		printHelpCmd("host <file-path>", "切换到指定 .msav")
		printHelpCmd("ip", "显示本机 IP 和监听地址")
		printHelpCmd("selfcheck", "基本自检（地址/端口/地图/配置）")
	case "vanilla":
		printHelpCmd("vanilla status", "查看原版参数文件路径")
		printHelpCmd("vanilla reload [path]", "重载原版参数文件（可选修改路径并写入配置）")
		printHelpCmd("vanilla gen [repoRoot] [outPath]", "从原版源码自动生成并加载 profiles.json（可选输出路径）")
		printHelpCmd("vanilla ids gen [repoRoot] [outPath]", "从原版源码/logicids.dat 生成并加载 content IDs")
		printHelpCmd("vanilla ids reload [path]", "重载 content IDs 到协议内容注册表")
		fmt.Printf("  当前文件: %s\n", cfg.Runtime.VanillaProfiles)
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
		fmt.Printf("  persist: enabled=%v dir=%s file=%s interval=%ds\n", cfg.Persist.Enabled, cfg.Persist.Directory, cfg.Persist.File, cfg.Persist.IntervalSec)
		fmt.Printf("  msav: enabled=%v dir=%s file=%s\n", cfg.Persist.SaveMSAV, cfg.Persist.MSAVDir, cfg.Persist.MSAVFile)
		fmt.Printf("  script file: %s\n", cfg.Script.File)
		fmt.Printf("  ops file: %s\n", cfg.Admin.OpsFile)
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
	srv.SendChat(c, "[accent]命令列表：")
	srv.SendChat(c, "[white]/help[] 查看命令")
	srv.SendChat(c, "[white]/status[] 服务器状态")
	srv.SendChat(c, "[white]/summon <typeId|unitName> [x y] [count] [team][] 召唤单位（OP，省略坐标=玩家脚底）")
	srv.SendChat(c, "[white]/despawn <entityId>[] 移除单位（OP）")
	srv.SendChat(c, "[white]/kill[] 杀死当前单位（附身/未附身均可）")
	srv.SendChat(c, "[white]/stop[] 保存并关闭服务器（OP）")
	state := "[red]关闭[]"
	if cfg.Runtime.SchedulerEnabled {
		state = "[green]开启[]"
	}
	srv.SendChat(c, "[accent]调度器状态: "+state)
	dbState := "[red]关闭[]"
	if cfg.Storage.DatabaseEnabled {
		dbState = "[green]开启[]"
	}
	srv.SendChat(c, "[accent]数据库: "+dbState)
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

func formatDisplayPlayerNameRaw(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		name = "未知玩家"
	}
	return currentPlayerNamePrefix() + name + currentPlayerNameSuffix()
}

func displayPlayerName(c *netserver.Conn) string {
	if c == nil {
		return "未知玩家"
	}
	name := strings.TrimSpace(c.Name())
	if name == "" {
		name = fmt.Sprintf("player-%d", c.PlayerID())
	}
	name = formatDisplayPlayerNameRaw(name)
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

type unitTypeRef struct {
	id int16
}

func (u unitTypeRef) ContentType() protocol.ContentType { return protocol.ContentUnit }
func (u unitTypeRef) ID() int16                         { return u.id }

type bulletTypeRef struct {
	id int16
}

func (b bulletTypeRef) ContentType() protocol.ContentType { return protocol.ContentBullet }
func (b bulletTypeRef) ID() int16                         { return b.id }

type itemRef struct {
	id int16
}

func (i itemRef) ContentType() protocol.ContentType { return protocol.ContentItem }
func (i itemRef) ID() int16                         { return i.id }

type blockRef struct {
	id int16
}

func (b blockRef) ContentType() protocol.ContentType { return protocol.ContentBlock }
func (b blockRef) ID() int16                         { return b.id }

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
	srv.Broadcast(&protocol.Remote_Units_unitDestroy_52{Uid: entityID})
}

func unpackTilePos(pos int32) (int32, int32) {
	return int32(uint16((pos >> 16) & 0xFFFF)), int32(uint16(pos & 0xFFFF))
}

func broadcastSetTile(srv *netserver.Server, buildPos int32, blockID int16, rot int8, team byte) {
	if srv == nil || buildPos < 0 || blockID < 0 {
		return
	}
	srv.Broadcast(&protocol.Remote_Tile_setTile_131{
		Tile:     protocol.TileBox{PosValue: buildPos},
		Block:    protocol.BlockRef{BlkID: blockID, BlkName: ""},
		Team:     protocol.Team{ID: team},
		Rotation: int32(rot) & 0x3,
	})
}

func broadcastBuildBeginPlace(srv *netserver.Server, buildPos int32, blockID int16, rot int8, team byte, config any) {
	if srv == nil || buildPos < 0 || blockID <= 0 {
		return
	}
	x, y := unpackTilePos(buildPos)
	srv.Broadcast(&protocol.Remote_Build_beginPlace_124{
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
	srv.Broadcast(&protocol.Remote_Build_beginBreak_123{
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
	srv.Broadcast(&protocol.Remote_ConstructBlock_deconstructFinish_136{
		Tile:    protocol.TileBox{PosValue: buildPos},
		Block:   protocol.BlockRef{BlkID: blockID, BlkName: ""},
		Builder: nil,
	})
}

func broadcastBuildHealthUpdate(srv *netserver.Server, items []int32) {
	if srv == nil || len(items) == 0 {
		return
	}
	srv.Broadcast(&protocol.Remote_Tile_buildHealthUpdate_135{
		Buildings: protocol.IntSeq{Items: items},
	})
}

func broadcastSetTileItems(srv *netserver.Server, itemID int16, amount int32, positions []int32) {
	if srv == nil || itemID < 0 || len(positions) == 0 {
		return
	}
	srv.Broadcast(&protocol.Remote_InputHandler_setTileItems_62{
		Item:      protocol.ItemRef{ItmID: itemID},
		Amount:    amount,
		Positions: append([]int32(nil), positions...),
	})
}

func syncCurrentWorldToConn(conn *netserver.Conn, wld *world.World) {
	if conn == nil || wld == nil {
		return
	}
	builds := wld.BuildSyncSnapshot()
	if len(builds) == 0 {
		return
	}
	health := make([]int32, 0, 256)
	for i := range builds {
		b := builds[i]
		if b.BlockID <= 0 {
			continue
		}
		_ = conn.SendAsync(&protocol.Remote_Tile_setTile_131{
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
			_ = conn.SendAsync(&protocol.Remote_Tile_buildHealthUpdate_135{
				Buildings: protocol.IntSeq{Items: append([]int32(nil), health...)},
			})
			health = health[:0]
		}
		if i > 0 && i%128 == 0 {
			time.Sleep(5 * time.Millisecond)
		}
	}
	if len(health) > 0 {
		_ = conn.SendAsync(&protocol.Remote_Tile_buildHealthUpdate_135{
			Buildings: protocol.IntSeq{Items: health},
		})
	}
	for _, snapshot := range wld.TeamCoreItemSnapshots() {
		positions := wld.TeamItemSyncPositions(snapshot.Team)
		if len(positions) == 0 {
			continue
		}
		for _, stack := range snapshot.Items {
			_ = conn.SendAsync(&protocol.Remote_InputHandler_setTileItems_62{
				Item:      protocol.ItemRef{ItmID: int16(stack.Item)},
				Amount:    stack.Amount,
				Positions: append([]int32(nil), positions...),
			})
		}
	}
}

func broadcastConstructFinish(srv *netserver.Server, buildPos int32, blockID int16, rot int8, team byte) {
	if srv == nil || buildPos < 0 || blockID <= 0 {
		return
	}
	srv.Broadcast(&protocol.Remote_ConstructBlock_constructFinish_137{
		Tile:     protocol.TileBox{PosValue: buildPos},
		Block:    protocol.BlockRef{BlkID: blockID, BlkName: ""},
		Builder:  nil,
		Rotation: rot & 0x3,
		Team:     protocol.Team{ID: team},
		Config:   nil,
	})
}

func broadcastBulletCreate(srv *netserver.Server, b world.BulletEvent) {
	_ = srv
	_ = b
	// Disabled in compatibility mode for now; wrong packet ID causes crashes.
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
	unitSyncBase := srv != nil && srv.ExtraEntitySnapshotFn != nil
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
			return trimmed, nil
		}
		if strings.HasSuffix(lower, ".msav") || strings.HasSuffix(lower, ".msav.msav") {
			base := worldstream.TrimMapName(filepath.Base(trimmed))
			if p, ok, err := findWorldByBaseName(base); err != nil {
				return "", err
			} else if ok {
				return p, nil
			}
			for _, candidate := range []string{
				filepath.Join("..", "core", "assets", "maps", "default", base+".msav"),
				filepath.Join("..", "..", "core", "assets", "maps", "default", base+".msav"),
			} {
				if exists(candidate) {
					return candidate, nil
				}
			}
		}
		return "", fmt.Errorf("地图文件不存在: %s", trimmed)
	}

	if p, ok, err := findWorldByBaseName(trimmed); err != nil {
		return "", err
	} else if ok {
		return p, nil
	}

	for _, candidate := range []string{
		filepath.Join("..", "core", "assets", "maps", "default", trimmed+".msav"),
		filepath.Join("..", "..", "core", "assets", "maps", "default", trimmed+".msav"),
	} {
		if exists(candidate) {
			return candidate, nil
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
		return localFiles[mathrand.New(mathrand.NewSource(time.Now().UnixNano())).Intn(len(localFiles))], nil
	}
	coreCandidates := []string{
		filepath.Join("..", "core", "assets", "maps", "default", "*.msav"),
		filepath.Join("..", "..", "core", "assets", "maps", "default", "*.msav"),
	}
	for _, g := range coreCandidates {
		files, err := filepath.Glob(g)
		if err == nil && len(files) > 0 {
			return files[mathrand.New(mathrand.NewSource(time.Now().UnixNano())).Intn(len(files))], nil
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
	fmt.Printf("  当前地图: %s\n", worldPath)
	fmt.Printf("  API: enabled=%v bind=%s auth=%v keys=%d\n", cfg.API.Enabled, cfg.API.Bind, len(cfg.API.Keys) > 0, len(cfg.API.Keys))
	fmt.Printf("  Storage: mode=%s db=%v dir=%s\n", cfg.Storage.Mode, cfg.Storage.DatabaseEnabled, cfg.Storage.Directory)
	fmt.Printf("  Mods: enabled=%v dir=%s\n", cfg.Mods.Enabled, cfg.Mods.Directory)
	fmt.Printf("  Persist: enabled=%v dir=%s file=%s interval=%ds\n", cfg.Persist.Enabled, cfg.Persist.Directory, cfg.Persist.File, cfg.Persist.IntervalSec)
	fmt.Printf("  MSAV snapshot: enabled=%v dir=%s file=%s\n", cfg.Persist.SaveMSAV, cfg.Persist.MSAVDir, cfg.Persist.MSAVFile)
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
	var firstTeamBuild protocol.Point2
	firstTeamBuildOK := false
	for i := range model.Tiles {
		tile := &model.Tiles[i]
		if tile == nil || tile.Build == nil || tile.Build.Health <= 0 || tile.Build.Team != 1 {
			continue
		}
		if !firstTeamBuildOK {
			firstTeamBuild = protocol.Point2{X: int32(tile.X), Y: int32(tile.Y)}
			firstTeamBuildOK = true
		}
		name := strings.ToLower(strings.TrimSpace(model.BlockNames[int16(tile.Build.Block)]))
		if strings.Contains(name, "core") || strings.Contains(name, "foundation") || strings.Contains(name, "nucleus") {
			return protocol.Point2{X: int32(tile.X), Y: int32(tile.Y)}, true
		}
	}
	if firstTeamBuildOK {
		return firstTeamBuild, true
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
	fallbackRank := -1
	fallbackDist2 := int(^uint(0) >> 1)
	fallbackPos := protocol.Point2{}
	fallbackName := ""
	for i := range model.Tiles {
		t := &model.Tiles[i]
		if t == nil || t.Block <= 0 {
			continue
		}
		blockName := model.BlockNames[int16(t.Block)]
		_, rank, ok := coreUnitNameAndRankByBlockName(blockName)
		if !ok {
			continue
		}
		dx := t.X - refX
		dy := t.Y - refY
		dist2 := dx*dx + dy*dy
		if rank > fallbackRank || (rank == fallbackRank && dist2 < fallbackDist2) {
			fallbackRank = rank
			fallbackDist2 = dist2
			fallbackPos = protocol.Point2{X: int32(t.X), Y: int32(t.Y)}
			fallbackName = blockName
		}
		owner := t.Team
		if owner == 0 && t.Build != nil && t.Build.Team != 0 {
			owner = t.Build.Team
		}
		if owner != team {
			continue
		}
		if rank > bestRank || (rank == bestRank && dist2 < bestDist2) {
			bestRank = rank
			bestDist2 = dist2
			bestPos = protocol.Point2{X: int32(t.X), Y: int32(t.Y)}
			bestName = blockName
		}
	}
	if bestRank < 0 {
		if fallbackRank >= 0 {
			return fallbackPos, fallbackName, true
		}
		return protocol.Point2{}, "", false
	}
	return bestPos, bestName, true
}

func resolveConnTeam(c *netserver.Conn, wld *world.World) world.TeamID {
	const defaultTeam = world.TeamID(1)
	if c == nil || wld == nil {
		return defaultTeam
	}
	if unitID := c.UnitID(); unitID != 0 {
		if ent, ok := wld.GetEntity(unitID); ok && ent.Team == defaultTeam {
			return ent.Team
		}
	}
	if playerID := c.PlayerID(); playerID != 0 {
		if ent, ok := wld.GetEntity(playerID); ok && ent.Team == defaultTeam {
			return ent.Team
		}
	}
	return defaultTeam
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
	corePos   protocol.Point2
	corePosOK bool
}

func (c *worldCache) get(path string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if c.path == path && c.modTime.Equal(info.ModTime()) && len(c.data) > 0 {
		return c.data, nil
	}

	data, err := loadWorldStream(path)
	if err != nil {
		return nil, err
	}
	c.path = path
	c.modTime = info.ModTime()
	c.data = data
	c.corePosOK = false
	if strings.HasSuffix(strings.ToLower(path), ".msav") || strings.HasSuffix(strings.ToLower(path), ".msav.msav") {
		if pos, ok, err := worldstream.FindCoreTileFromMSAV(path); err == nil {
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

func loadWorldStream(path string) ([]byte, error) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".msav") || strings.HasSuffix(lower, ".msav.msav") {
		if ver, err := worldstream.ReadMSAVVersion(path); err == nil && ver < 11 {
			fmt.Printf("地图保存版本较旧: %d（继续尝试加载并在失败时回退） path=%s\n", ver, path)
		}
		data, err := worldstream.BuildWorldStreamFromMSAV(path)
		if err == nil {
			return data, nil
		}
		fmt.Printf("地图转换失败，回退到 bootstrap-world.bin: path=%s err=%v\n", path, err)
		fallback, ferr := loadBootstrapWorldFallback()
		if ferr != nil {
			return nil, fmt.Errorf("msav convert failed: %w; bootstrap fallback failed: %v", err, ferr)
		}
		return fallback, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func loadBootstrapWorldFallback() ([]byte, error) {
	candidates := []string{
		filepath.Join(runtimeAssetsDir, "bootstrap-world.bin"),
		filepath.Join("go-server", "assets", "bootstrap-world.bin"),
	}
	for _, p := range candidates {
		data, err := os.ReadFile(p)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}
	return nil, errors.New("bootstrap-world.bin not found")
}
