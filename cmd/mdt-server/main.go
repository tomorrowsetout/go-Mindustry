package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"errors"
	"flag"
	"fmt"
	"io"
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

type streamMirror struct {
	prevStdout *os.File
	prevStderr *os.File
	stdoutR    *os.File
	stdoutW    *os.File
	stderrR    *os.File
	stderrW    *os.File
	stdoutLog  *rotatingFileWriter
	stderrLog  *rotatingFileWriter
	done       chan struct{}
}

type logRules struct {
	netEnabled        bool
	netTxEnabled      bool
	netUdpTxEnabled   bool
	worldStreamEnable bool
	buildSvcEnabled   bool
	scriptEnabled     bool
	modsEnabled       bool
	featureEnabled    bool
}

type lineFilterWriter struct {
	mu   sync.Mutex
	dst  io.Writer
	rule logRules
	buf  []byte
}

type rotatingFileWriter struct {
	mu       sync.Mutex
	dir      string
	prefix   string
	maxSize  int64
	maxFiles int
	curFile  *os.File
	curSize  int64
}

func newRotatingFileWriter(dir, prefix string, maxSize int64, maxFiles int) (*rotatingFileWriter, error) {
	if maxSize <= 0 {
		maxSize = 10 * 1024 * 1024
	}
	if maxFiles <= 0 {
		maxFiles = 100
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	w := &rotatingFileWriter{
		dir:      dir,
		prefix:   prefix,
		maxSize:  maxSize,
		maxFiles: maxFiles,
	}
	if err := w.rotateLocked(); err != nil {
		return nil, err
	}
	return w, nil
}

func newLineFilterWriter(dst io.Writer, rule logRules) *lineFilterWriter {
	if dst == nil {
		dst = io.Discard
	}
	return &lineFilterWriter{dst: dst, rule: rule, buf: make([]byte, 0, 1024)}
}

func (w *lineFilterWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.buf = append(w.buf, p...)
	for {
		idx := bytes.IndexByte(w.buf, '\n')
		if idx < 0 {
			break
		}
		line := string(w.buf[:idx+1])
		if w.allowLine(line) {
			_, _ = w.dst.Write([]byte(line))
		}
		w.buf = w.buf[idx+1:]
	}
	return len(p), nil
}

func (w *lineFilterWriter) allowLine(line string) bool {
	switch {
	case strings.Contains(line, "[net] tx-udp"):
		return w.rule.netEnabled && w.rule.netTxEnabled && w.rule.netUdpTxEnabled
	case strings.Contains(line, "[net] tx "):
		return w.rule.netEnabled && w.rule.netTxEnabled
	case strings.Contains(line, "[net]"):
		return w.rule.netEnabled
	case strings.Contains(line, "[worldstream]"):
		return w.rule.worldStreamEnable
	case strings.Contains(line, "[buildsvc]"):
		return w.rule.buildSvcEnabled
	case strings.Contains(line, "[script]"):
		return w.rule.scriptEnabled
	case strings.Contains(line, "[MOD"):
		return w.rule.modsEnabled
	case strings.Contains(line, "[feature]"):
		return w.rule.featureEnabled
	default:
		return true
	}
}

func (w *rotatingFileWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.curFile == nil {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}
	if w.curSize+int64(len(p)) > w.maxSize {
		if err := w.rotateLocked(); err != nil {
			return 0, err
		}
	}
	n, err := w.curFile.Write(p)
	w.curSize += int64(n)
	return n, err
}

func (w *rotatingFileWriter) CurrentPath() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.curFile == nil {
		return ""
	}
	return w.curFile.Name()
}

func (w *rotatingFileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.curFile != nil {
		err := w.curFile.Close()
		w.curFile = nil
		w.curSize = 0
		return err
	}
	return nil
}

func (w *rotatingFileWriter) rotateLocked() error {
	if w.curFile != nil {
		_ = w.curFile.Close()
		w.curFile = nil
	}
	name := fmt.Sprintf("%s-%s.log", w.prefix, time.Now().Format("20060102-150405.000000000"))
	path := filepath.Join(w.dir, name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return err
	}
	w.curFile = f
	w.curSize = fi.Size()
	w.cleanupLocked()
	return nil
}

func (w *rotatingFileWriter) cleanupLocked() {
	pattern := filepath.Join(w.dir, w.prefix+"-*.log")
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) <= w.maxFiles {
		return
	}
	type fileInfo struct {
		path string
		time time.Time
	}
	items := make([]fileInfo, 0, len(matches))
	for _, m := range matches {
		st, err := os.Stat(m)
		if err != nil {
			continue
		}
		items = append(items, fileInfo{path: m, time: st.ModTime()})
	}
	if len(items) <= w.maxFiles {
		return
	}
	sort.Slice(items, func(i, j int) bool { return items[i].time.Before(items[j].time) })
	excess := len(items) - w.maxFiles
	for i := 0; i < excess; i++ {
		_ = os.Remove(items[i].path)
	}
}

func startStreamMirror(cfg config.LoggingConfig) (*streamMirror, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	logDir := strings.TrimSpace(cfg.Directory)
	if logDir == "" {
		logDir = "logs"
	}
	maxBytes := int64(cfg.MaxFileMB) * 1024 * 1024
	if maxBytes <= 0 {
		maxBytes = 10 * 1024 * 1024
	}
	maxFiles := cfg.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 100
	}
	var stdoutLog *rotatingFileWriter
	var stderrLog *rotatingFileWriter
	var err error
	if cfg.FileEnabled {
		stdoutLog, err = newRotatingFileWriter(logDir, "server-stdout", maxBytes, maxFiles)
		if err != nil {
			return nil, err
		}
		stderrLog, err = newRotatingFileWriter(logDir, "server-stderr", maxBytes, maxFiles)
		if err != nil {
			_ = stdoutLog.Close()
			return nil, err
		}
	}

	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		if stdoutLog != nil {
			_ = stdoutLog.Close()
		}
		if stderrLog != nil {
			_ = stderrLog.Close()
		}
		return nil, err
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		_ = stdoutR.Close()
		_ = stdoutW.Close()
		if stdoutLog != nil {
			_ = stdoutLog.Close()
		}
		if stderrLog != nil {
			_ = stderrLog.Close()
		}
		return nil, err
	}

	m := &streamMirror{
		prevStdout: os.Stdout,
		prevStderr: os.Stderr,
		stdoutR:    stdoutR,
		stdoutW:    stdoutW,
		stderrR:    stderrR,
		stderrW:    stderrW,
		stdoutLog:  stdoutLog,
		stderrLog:  stderrLog,
		done:       make(chan struct{}, 2),
	}

	os.Stdout = stdoutW
	os.Stderr = stderrW

	rule := logRules{
		netEnabled:        cfg.NetEnabled,
		netTxEnabled:      cfg.NetTxEnabled,
		netUdpTxEnabled:   cfg.NetUdpTxEnabled,
		worldStreamEnable: cfg.WorldStreamEnable,
		buildSvcEnabled:   cfg.BuildSvcEnabled,
		scriptEnabled:     cfg.ScriptEnabled,
		modsEnabled:       cfg.ModsEnabled,
		featureEnabled:    cfg.FeatureEnabled,
	}
	outTargets := make([]io.Writer, 0, 2)
	errTargets := make([]io.Writer, 0, 2)
	if cfg.ConsoleEnabled {
		outTargets = append(outTargets, m.prevStdout)
		errTargets = append(errTargets, m.prevStderr)
	}
	if cfg.FileEnabled {
		outTargets = append(outTargets, m.stdoutLog)
		errTargets = append(errTargets, m.stderrLog)
	}
	if len(outTargets) == 0 {
		outTargets = append(outTargets, io.Discard)
	}
	if len(errTargets) == 0 {
		errTargets = append(errTargets, io.Discard)
	}
	outSink := newLineFilterWriter(io.MultiWriter(outTargets...), rule)
	errSink := newLineFilterWriter(io.MultiWriter(errTargets...), rule)

	go func() {
		_, _ = io.Copy(outSink, m.stdoutR)
		m.done <- struct{}{}
	}()
	go func() {
		_, _ = io.Copy(errSink, m.stderrR)
		m.done <- struct{}{}
	}()

	if cfg.FileEnabled {
		fmt.Fprintf(m.prevStdout, "[log] stdout -> %s (rotate=%dMB maxFiles=%d)\n", stdoutLog.CurrentPath(), cfg.MaxFileMB, maxFiles)
		fmt.Fprintf(m.prevStdout, "[log] stderr -> %s (rotate=%dMB maxFiles=%d)\n", stderrLog.CurrentPath(), cfg.MaxFileMB, maxFiles)
	}
	return m, nil
}

func (m *streamMirror) Close() {
	if m == nil {
		return
	}
	os.Stdout = m.prevStdout
	os.Stderr = m.prevStderr
	_ = m.stdoutW.Close()
	_ = m.stderrW.Close()
	select {
	case <-m.done:
	case <-time.After(500 * time.Millisecond):
	}
	select {
	case <-m.done:
	case <-time.After(500 * time.Millisecond):
	}
	_ = m.stdoutR.Close()
	_ = m.stderrR.Close()
	if m.stdoutLog != nil {
		_ = m.stdoutLog.Close()
	}
	if m.stderrLog != nil {
		_ = m.stderrLog.Close()
	}
}

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

type runtimeTileSyncState struct {
	BlockID int16
	Team    byte
	Rot     int8
	Health  float32
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

func mapModeSummary(r *world.Rules) string {
	if r == nil {
		return "unknown"
	}
	mode := "survival"
	switch {
	case r.Editor || r.InfiniteResources:
		mode = "sandbox"
	case r.Pvp:
		mode = "pvp"
	case r.AttackMode:
		mode = "attack"
	}
	return fmt.Sprintf("mode=%s waves=%v waveTimer=%v waveSpacing=%.1fs initialWave=%.1fs", mode, r.Waves, r.WaveTimer, r.WaveSpacing, r.InitialWaveSpacing)
}

func chdirToExecutableDir() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	dir := filepath.Dir(exe)
	if strings.TrimSpace(dir) == "" {
		return nil
	}
	return os.Chdir(dir)
}

func normalizeWorldPathForExeRoot(path string) string {
	p := strings.TrimSpace(filepath.Clean(path))
	if p == "" {
		return ""
	}
	if filepath.IsAbs(p) || strings.HasPrefix(p, ".."+string(filepath.Separator)) || p == ".." {
		base := filepath.Base(p)
		if strings.TrimSpace(base) == "" || base == "." || base == string(filepath.Separator) {
			return ""
		}
		return filepath.Join("assets", "worlds", base)
	}
	return p
}

func stableString(s string) string {
	if s == "" {
		return ""
	}
	b := make([]byte, len(s))
	copy(b, s)
	return string(b)
}

func main() {
	if err := chdirToExecutableDir(); err != nil {
		fmt.Fprintf(os.Stderr, "切换到程序目录失败: %v\n", err)
		os.Exit(1)
	}

	cfgPath := flag.String("config", "config.json", "path to config file")
	addr := flag.String("addr", "0.0.0.0:6567", "listen address for Mindustry protocol (TCP+UDP)")
	buildVersion := flag.Int("build", 155, "Mindustry build version for strict check; must match client build")
	worldArg := flag.String("world", "random", "world source: random | <map-name> | <.msav file path>")
	printVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *printVersion {
		fmt.Printf("mdt-server %s (%s)\n", buildinfo.Version, buildinfo.Commit)
		return
	}
	if *buildVersion <= 0 {
		fmt.Fprintln(os.Stderr, "build 必须设置为客户端对应的 build 号，例如 155")
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

	streamLog, err := startStreamMirror(cfg.Logging)
	if err != nil {
		fmt.Fprintf(os.Stderr, "初始化日志镜像失败: %v\n", err)
	} else {
		defer streamLog.Close()
	}

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
	cfg.API.Keys = mergeKeys(cfg.API.Keys, cfg.API.Key)
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
	if cfg.Logging.DevLogEnabled {
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
	recoveredMapFromPersist := false
	if cfg.Persist.Enabled {
		if st, ok, err := persist.Load(cfg.Persist); err != nil {
			log.Warn("persist load failed", logging.Field{Key: "error", Value: err.Error()})
			startup.warn("持久化加载", err.Error())
		} else if ok {
			persisted = st
			persistedOK = true
			if worldChoice == "random" && st.MapPath != "" {
				worldChoice = normalizeWorldPathForExeRoot(st.MapPath)
				recoveredMapFromPersist = true
				startup.ok("持久化地图恢复", worldChoice)
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
	fmt.Fprintf(os.Stdout, "当前地图: %s\n", initialWorld)

	srv := netserver.NewServer(*addr, *buildVersion)
	srv.SetSnapshotLogSample(cfg.Logging.SnapshotLogSample)
	srv.SetTileConfigForwardMode(cfg.Net.TileConfigForwardMode)
	contentIDsPath := filepath.FromSlash("data/vanilla/content_ids.json")
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
	srv.SetItemDepositCooldown(wld.GetRulesManager().Get().ItemDepositCooldown)
	buildService := buildsvc.New(wld, buildsvc.Options{
		MaxQueuedBatches: 4096,
		MaxPlansPerBatch: 64,
		MaxOpsPerTick:    128,
	})
	var runtimeTileMu sync.RWMutex
	runtimeTiles := make(map[int32]runtimeTileSyncState)
	srv.SpawnUnitFn = func(c *netserver.Conn, unitID int32, tile protocol.Point2, unitType int16) (float32, float32, bool) {
		if c == nil || wld == nil {
			return 0, 0, false
		}
		x := float32(tile.X*8 + 4)
		y := float32(tile.Y*8 + 4)
		ent, err := wld.AddEntityWithID(unitType, unitID, x, y, world.TeamID(1))
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
	srv.SyncUnitStateFn = func(unitID int32, x, y, rotation, vx, vy float32) {
		if wld == nil || unitID == 0 {
			return
		}
		_, _ = wld.SetEntityPosition(unitID, x, y, rotation)
		_, _ = wld.SetEntityMotion(unitID, vx, vy, 0)
	}
	var unitNamesByID map[int16]string
	var loadedModel *world.WorldModel
	var loadedMapPath string

	var playerSpawnTypeID int32 = 1
	if err := wld.LoadVanillaProfiles(cfg.Runtime.VanillaProfiles); err != nil {
		log.Warn("vanilla profiles load failed", logging.Field{Key: "path", Value: cfg.Runtime.VanillaProfiles}, logging.Field{Key: "error", Value: err.Error()})
		startup.warn("原版 profiles", fmt.Sprintf("加载失败: %s", err.Error()))
	} else if strings.TrimSpace(cfg.Runtime.VanillaProfiles) != "" {
		startup.ok("原版 profiles", cfg.Runtime.VanillaProfiles)
	}
	loadWorldModel := func(path string) {
		buildService.Reset()
		runtimeTileMu.Lock()
		clear(runtimeTiles)
		runtimeTileMu.Unlock()
		srv.ClearTileConfigs()
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
		wld.ResetRuntimeFromTags(model.Tags)
		srv.SetItemDepositCooldown(wld.GetRulesManager().Get().ItemDepositCooldown)
		startup.ok("地图规则", mapModeSummary(wld.GetRulesManager().Get()))
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
		spawnType := int16(1)
		if alphaID, ok := wld.ResolveUnitTypeID("alpha"); ok {
			spawnType = alphaID
		}
		if model != nil {
			for i := range model.Tiles {
				tile := &model.Tiles[i]
				if tile == nil || tile.Block <= 0 {
					continue
				}
				blockName := strings.ToLower(strings.TrimSpace(model.BlockNames[int16(tile.Block)]))
				if strings.Contains(blockName, "foundation") || strings.Contains(blockName, "nucleus") {
					if betaID, ok := wld.ResolveUnitTypeID("beta"); ok {
						spawnType = betaID
					}
					break
				}
			}
		}
		atomic.StoreInt32(&playerSpawnTypeID, int32(spawnType))
		startup.ok("玩家出生单位", fmt.Sprintf("typeId=%d", spawnType))
	}
	loadWorldModel(initialWorld)
	if persistedOK && recoveredMapFromPersist {
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
	var eventPacketCounter atomic.Int64
	srv.OnEvent = func(ev netserver.NetEvent) {
		if !cfg.Logging.EventStoreEnabled {
			return
		}
		if ev.Kind == "packet_send" && !cfg.Logging.EventPacketSend {
			return
		}
		if ev.Kind == "packet_recv" && !cfg.Logging.EventPacketRecv {
			return
		}
		if ev.Kind == "packet_send" || ev.Kind == "packet_recv" {
			n := eventPacketCounter.Add(1)
			sample := cfg.Logging.EventPacketSample
			if sample <= 1 {
				sample = 1
			}
			if n%int64(sample) != 1 {
				return
			}
		}
		_ = recorder.Record(storage.Event{
			Timestamp: ev.Timestamp,
			Kind:      stableString(ev.Kind),
			Packet:    stableString(ev.Packet),
			Detail:    stableString(ev.Detail),
			ConnID:    ev.ConnID,
			UUID:      stableString(ev.UUID),
			IP:        stableString(ev.IP),
			Name:      stableString(ev.Name),
		})
	}
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

	cache := &worldCache{
		preloadEnabled: cfg.Net.WorldDataPreload,
		maxBytes:       int64(cfg.Net.WorldDataPreloadMaxMB) * 1024 * 1024,
	}
	if err := cache.preload(state.get()); err != nil {
		startup.warn("WorldData 预载入", err.Error())
	} else if cache.preloadEnabled {
		if cache.maxBytes > 0 {
			startup.ok("WorldData 预载入", fmt.Sprintf("enabled=true max=%dMB", cfg.Net.WorldDataPreloadMaxMB))
		} else {
			startup.ok("WorldData 预载入", "enabled=true max=unlimited")
		}
	} else {
		startup.info("WorldData 预载入", "enabled=false")
	}
	srv.WorldDataFn = func(conn *netserver.Conn, _ *protocol.ConnectPacket) ([]byte, error) {
		// Keep handshake payload strictly compatible with official client parser.
		// Runtime build/config deltas are replayed after connect via OnPostConnect.
		path := state.get()
		base, err := cache.get(path)
		if err != nil {
			return nil, err
		}
		if conn != nil && conn.PlayerID() != 0 {
			if patched, perr := worldstream.RewritePlayerIDInWorldStream(base, conn.PlayerID()); perr == nil {
				return patched, nil
			}
		}
		return base, nil
	}
	srv.SpawnTileFn = func() (protocol.Point2, bool) {
		pos, ok, err := cache.spawnPos(state.get())
		if err == nil && ok {
			return pos, true
		}
		// Fallback for maps where core tile cannot be parsed from msav metadata.
		return fallbackSpawnPosFromModel(wld.Model())
	}
	srv.OnPostConnect = func(c *netserver.Conn) {
		if c == nil {
			return
		}
		replaySync := strings.EqualFold(cfg.Net.PostConnectReplayMode, "sync")
		sendReplay := func(obj any) {
			if replaySync {
				_ = c.Send(obj)
			} else {
				_ = c.SendAsync(obj)
			}
		}
		// Replay runtime build/destroy deltas for late joiners.
		runtimeTileMu.RLock()
		pending := make([]struct {
			pos int32
			st  runtimeTileSyncState
		}, 0, len(runtimeTiles))
		for pos, st := range runtimeTiles {
			pending = append(pending, struct {
				pos int32
				st  runtimeTileSyncState
			}{pos: pos, st: st})
		}
		runtimeTileMu.RUnlock()
		sort.Slice(pending, func(i, j int) bool { return pending[i].pos < pending[j].pos })
		for _, it := range pending {
			sendReplay(&protocol.Remote_Tile_setTile_131{
				Tile:     protocol.TileBox{PosValue: it.pos},
				Block:    protocol.BlockRef{BlkID: it.st.BlockID, BlkName: ""},
				Team:     protocol.Team{ID: it.st.Team},
				Rotation: int32(it.st.Rot) & 0x3,
			})
			sendReplay(&protocol.Remote_Tile_buildHealthUpdate_135{
				Buildings: protocol.IntSeq{Items: []int32{
					it.pos, int32(math.Float32bits(it.st.Health)),
				}},
			})
		}

		// Replay inventory state for late joiners (e.g. core/sorter/unloader items).
		// Clear then set makes replay deterministic even if base world stream had stale values.
		inventoriesByPos := wld.SnapshotBuildingInventories()
		if len(inventoriesByPos) > 0 {
			posList := make([]int32, 0, len(inventoriesByPos))
			for pos := range inventoriesByPos {
				posList = append(posList, pos)
			}
			sort.Slice(posList, func(i, j int) bool { return posList[i] < posList[j] })
			for _, pos := range posList {
				sendReplay(&protocol.Remote_InputHandler_clearItems_63{
					Build: protocol.BuildingBox{PosValue: pos},
				})

				src := inventoriesByPos[pos]
				items := make([]protocol.ItemStack, 0, len(src))
				for _, st := range src {
					if st.Amount <= 0 {
						continue
					}
					items = append(items, protocol.ItemStack{
						Item:   itemRef{id: int16(st.Item)},
						Amount: st.Amount,
					})
				}
				if len(items) == 0 {
					continue
				}
				sendReplay(&protocol.Remote_InputHandler_setItems_61{
					Build: protocol.BuildingBox{PosValue: pos},
					Items: items,
				})
			}
		}

		// Replay per-tile configs (sorter/bridge/logic/etc.) after tile state is present.
		// For runtime-changed positions, explicitly sending nil clears stale client config.
		configs := srv.SnapshotTileConfigs()
		replayConfigPos := make(map[int32]struct{}, len(pending)+len(configs))
		for _, it := range pending {
			replayConfigPos[it.pos] = struct{}{}
		}
		for pos := range configs {
			replayConfigPos[pos] = struct{}{}
		}
		if len(replayConfigPos) == 0 {
			return
		}
		posList := make([]int32, 0, len(replayConfigPos))
		for pos := range replayConfigPos {
			posList = append(posList, pos)
		}
		sort.Slice(posList, func(i, j int) bool { return posList[i] < posList[j] })
		for _, pos := range posList {
			val, ok := configs[pos]
			if !ok {
				val = nil
			}
			sendReplay(&protocol.Remote_InputHandler_tileConfig_86{
				Build: protocol.BuildingBox{PosValue: pos},
				Value: val,
			})
		}
	}
	srv.OnBuildPlans = func(c *netserver.Conn, plans []*protocol.BuildPlan) {
		if c == nil || len(plans) == 0 {
			return
		}
		if model := wld.Model(); model != nil && len(model.BlockNames) > 0 {
			for _, p := range plans {
				if p == nil || p.Breaking || p.Block == nil {
					continue
				}
				blockID := p.Block.ID()
				blockName := strings.ToLower(strings.TrimSpace(model.BlockNames[blockID]))
				if blockName == "" {
					continue
				}
				if strings.Contains(blockName, "bridge-conveyor") || strings.Contains(blockName, "sorter") || strings.Contains(blockName, "unloader") {
					srv.WarnFeatureOnce(
						"logistics-"+strconv.Itoa(int(blockID)),
						fmt.Sprintf("[feature] logistics behavior not fully simulated yet: block=%d name=%s (multi-player desync possible)", blockID, blockName),
					)
				}
			}
		}
		_ = buildService.ApplyPlansNow(world.TeamID(1), plans)
	}
	srv.CanInteractBuildFn = func(c *netserver.Conn, buildPos int32, action string) bool {
		if buildPos < 0 {
			return false
		}
		if !wld.HasBuilding(buildPos) {
			return false
		}
		rules := wld.GetRulesManager().Get()
		team, ok := wld.BuildingTeam(buildPos)
		if !ok {
			return false
		}
		connTeam := world.TeamID(srv.ConnTeamID(c))
		if team != connTeam && (rules == nil || !rules.Editor) {
			return false
		}
		switch action {
		case "transfer_inventory", "transfer_item_to":
			return wld.CanDepositToBuilding(buildPos)
		default:
			return true
		}
	}
	srv.OnSetPlayerTeamEditor = func(c *netserver.Conn, teamID byte) bool {
		_ = c
		_ = teamID
		r := wld.GetRulesManager().Get()
		return r != nil && r.Editor
	}
	srv.OnSetItem = func(buildPos int32, itemID int16, amount int32) {
		_ = wld.SetBuildingItem(buildPos, itemID, amount)
	}
	srv.OnSetItems = func(buildPos int32, items []protocol.ItemStack) {
		out := make([]world.ItemStack, 0, len(items))
		for _, st := range items {
			if st.Item == nil || st.Amount <= 0 {
				continue
			}
			out = append(out, world.ItemStack{
				Item:   world.ItemID(st.Item.ID()),
				Amount: st.Amount,
			})
		}
		_ = wld.SetBuildingItems(buildPos, out)
	}
	srv.OnSetTileItems = func(itemID int16, amount int32, positions []int32) {
		_ = wld.SetTileItems(positions, itemID, amount)
	}
	srv.OnClearItems = func(buildPos int32) {
		_ = wld.ClearBuildingItems(buildPos)
	}
	srv.OnTransferItemTo = func(buildPos int32, itemID int16, amount int32) int32 {
		return wld.AcceptBuildingItem(buildPos, itemID, amount)
	}
	srv.OnTakeItems = func(buildPos int32, itemID int16, amount int32, toUnitID int32) int32 {
		if buildPos < 0 || itemID <= 0 || amount <= 0 || toUnitID == 0 {
			return 0
		}
		removed := wld.RemoveBuildingItem(buildPos, itemID, amount)
		if removed <= 0 {
			return 0
		}
		added := srv.AddUnitItemByID(toUnitID, itemID, removed)
		if added < removed {
			_ = wld.AddBuildingItem(buildPos, itemID, removed-added)
		}
		return added
	}
	srv.OnTransferItemToUnit = func(itemID int16, amount int32, toUnitID int32) int32 {
		if itemID <= 0 || amount <= 0 || toUnitID == 0 {
			return 0
		}
		return srv.AddUnitItemByID(toUnitID, itemID, amount)
	}
	srv.OnTransferInventory = func(c *netserver.Conn, buildPos int32) (int16, int32) {
		if c == nil || buildPos < 0 {
			return 0, 0
		}
		itemID, amount, ok := srv.ConsumePlayerUnitStack(c, 0)
		if !ok || amount <= 0 {
			return 0, 0
		}
		accepted := wld.AcceptBuildingItem(buildPos, itemID, amount)
		if accepted < amount {
			_ = srv.AddPlayerUnitItem(c, itemID, amount-accepted)
		}
		return itemID, accepted
	}
	srv.OnRequestItem = func(c *netserver.Conn, buildPos int32, itemID int16, amount int32) int32 {
		if c == nil || buildPos < 0 || itemID <= 0 || amount <= 0 {
			return 0
		}
		if !srv.CanPlayerUnitCarry(c, itemID) {
			return 0
		}
		moved := wld.RemoveBuildingItem(buildPos, itemID, amount)
		if moved <= 0 {
			return 0
		}
		added := srv.AddPlayerUnitItem(c, itemID, moved)
		if added < moved {
			_ = wld.AcceptBuildingItem(buildPos, itemID, moved-added)
		}
		return added
	}
	srv.OnRemoveQueueBlock = func(x, y int32, breaking bool) {
		_ = wld.RemovePendingBuild(x, y, breaking)
	}
	srv.OnTileConfig = func(buildPos int32, value any) {
		raw, err := encodeTileConfigRaw(value, srv.TypeIO)
		if err != nil {
			return
		}
		_ = wld.SetBuildingConfigRaw(buildPos, raw)
		_ = wld.SetBuildingConfigValue(buildPos, value)
	}
	srv.OnRotateBlock = func(buildPos int32, direction bool) {
		blockID, rot, team, ok := wld.RotateBuilding(buildPos, direction)
		if !ok {
			return
		}
		broadcastConstructFinish(srv, buildPos, blockID, rot, byte(team))
	}
	srv.OnRequestDropPayload = func(c *netserver.Conn, x, y float32) {
		if c == nil {
			return
		}
		unitID := c.UnitID()
		if unitID == 0 {
			return
		}
		rot := float32(0)
		if ent, ok := wld.GetEntity(unitID); ok {
			rot = ent.Rotation
		}
		_, _ = wld.SetEntityPosition(unitID, x, y, rot)
	}
	srv.OnPayloadDropped = func(c *netserver.Conn, unitID int32, x, y float32) {
		if unitID == 0 {
			return
		}
		rot := float32(0)
		if ent, ok := wld.GetEntity(unitID); ok {
			rot = ent.Rotation
		}
		_, _ = wld.SetEntityPosition(unitID, x, y, rot)
		_, _ = wld.ClearEntityBehavior(unitID)
	}
	srv.OnUnitEnteredPayload = func(c *netserver.Conn, unitID int32, buildPos int32) {
		if unitID == 0 {
			return
		}
		if buildPos != 0 {
			pt := protocol.UnpackPoint2(buildPos)
			rot := float32(0)
			if ent, ok := wld.GetEntity(unitID); ok {
				rot = ent.Rotation
			}
			_, _ = wld.SetEntityPosition(unitID, float32(pt.X), float32(pt.Y), rot)
		}
		_, _ = wld.ClearEntityBehavior(unitID)
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
			CoreData: []byte{0},
		}
	}
	srv.ExtraEntitySnapshotFn = func(w *protocol.Writer) (int16, error) {
		// Disabled for now: full-map entity sync can exceed TypeIO int16 byte-array limits
		// and corrupt packet decoding on official clients.
		_ = w
		return 0, nil
	}
	go func() {
		t := time.NewTicker(33 * time.Millisecond)
		defer t.Stop()
		for range t.C {
			evs := wld.DrainEntityEvents()
			buildHealth := make([]int32, 0, len(evs)*2)
			buildItems := make(map[int32][]world.ItemStack)
			for _, ev := range evs {
				switch ev.Kind {
				case world.EntityEventRemoved:
					broadcastUnitDestroy(srv, ev.Entity.ID)
					srv.MarkUnitDead(ev.Entity.ID, "world-removed")
				case world.EntityEventBuildPlaced:
					runtimeTileMu.Lock()
					runtimeTiles[ev.BuildPos] = runtimeTileSyncState{
						BlockID: ev.BuildBlock,
						Team:    byte(ev.BuildTeam),
						Rot:     ev.BuildRot,
						Health:  maxf32(ev.BuildHP, 1),
					}
					runtimeTileMu.Unlock()
					broadcastConstructFinish(srv, ev.BuildPos, ev.BuildBlock, ev.BuildRot, byte(ev.BuildTeam))
				case world.EntityEventBuildDestroyed:
					runtimeTileMu.Lock()
					delete(runtimeTiles, ev.BuildPos)
					runtimeTileMu.Unlock()
					srv.ClearTileConfig(ev.BuildPos)
					if len(ev.BuildItems) > 0 {
						_ = wld.RefundToTeamCore(ev.BuildTeam, ev.BuildItems)
					}
					broadcastBuildDestroyed(srv, ev.BuildPos)
				case world.EntityEventBuildHealth:
					runtimeTileMu.Lock()
					if st, ok := runtimeTiles[ev.BuildPos]; ok {
						st.Health = ev.BuildHP
						runtimeTiles[ev.BuildPos] = st
					}
					runtimeTileMu.Unlock()
					buildHealth = append(buildHealth, ev.BuildPos, int32(math.Float32bits(ev.BuildHP)))
				case world.EntityEventBuildItems:
					buildItems[ev.BuildPos] = append([]world.ItemStack(nil), ev.BuildItems...)
				case world.EntityEventBulletFired:
					broadcastBulletCreate(srv, ev.Bullet)
				}
			}
			if len(buildHealth) > 0 {
				broadcastBuildHealthUpdate(srv, buildHealth)
			}
			if len(buildItems) > 0 {
				for pos, items := range buildItems {
					broadcastBuildItems(srv, pos, items)
				}
			}
		}
	}()
	saveState := func() {}
	srv.OnChat = func(c *netserver.Conn, msg string) bool {
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
	if cfg.Runtime.SchedulerEnabled {
		engine = sim.NewEngine(sim.Config{
			TPS:        sim.DefaultTPS,
			Cores:      cfg.Runtime.Cores,
			Partitions: cfg.Runtime.Cores,
			TotalWork:  0,
			MaxCatchUp: 4,
		})
		engine.SetWork(func(ctx sim.TickContext, p sim.Partition) {
			if p.ID == 0 {
				buildService.Tick()
				wld.Step(ctx.Delta)
			}
		})
		engine.Start()
	} else {
		go func() {
			interval := time.Second / time.Duration(sim.DefaultTPS)
			t := time.NewTicker(interval)
			defer t.Stop()
			for range t.C {
				buildService.Tick()
				wld.Step(interval)
			}
		}()
	}

	monitor := newStatusMonitor(srv, cfg, engine)
	saveState = func() {}
	if cfg.Persist.Enabled {
		saveState = func() {
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
	startup.print()
	if loadedModel != nil {
		printMapDetails(loadedMapPath, loadedModel)
	}
	printUnitIDList(unitNamesByID)
	go runConsole(srv, state, modMgr, apiSrv, scriptCtl, *addr, *buildVersion, &cfg, saveConfig, saveScript, recorder, monitor, saveOps, loadWorldModel, reloadVanillaProfiles, reloadVanillaContentIDs, removeEntityByID, setEntityMotion, setEntityPos, setEntityLife, setEntityFollow, setEntityPatrol, clearEntityBehavior, stopServer)
	if err := srv.Serve(); err != nil {
		fmt.Fprintf(os.Stderr, "服务器启动失败: %v\n", err)
		os.Exit(1)
	}
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
	printConsoleIntro(name, state.get(), listenAddr, cfg.API.Bind, cfg.API.Enabled)
	printHelp(*cfg)
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
				fmt.Printf("vanilla content ids: %s\n", filepath.FromSlash("data/vanilla/content_ids.json"))
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
				if len(parts) >= 3 {
					out = strings.TrimSpace(strings.Join(parts[2:], " "))
					cfg.Runtime.VanillaProfiles = out
					_ = saveConfig()
				}
				repoRoot, _ := os.Getwd()
				units, turrets, err := vanilla.GenerateProfiles(repoRoot, out)
				if err != nil {
					fmt.Printf("vanilla gen 失败: %v\n", err)
					continue
				}
				if err := reloadVanillaProfiles(out); err != nil {
					fmt.Printf("profiles 生成成功但加载失败: %v\n", err)
					continue
				}
				fmt.Printf("vanilla profiles 生成并加载完成: units_by_name=%d turrets=%d path=%s\n", units, turrets, out)
			case "ids":
				if len(parts) < 3 {
					fmt.Println("用法: vanilla ids gen [repoRoot] [outPath] | vanilla ids reload [path]")
					continue
				}
				sub2 := strings.ToLower(parts[2])
				switch sub2 {
				case "gen":
					repo := "."
					out := filepath.FromSlash("data/vanilla/content_ids.json")
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
					path := filepath.FromSlash("data/vanilla/content_ids.json")
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
				fmt.Println("用法: vanilla status | vanilla reload [path] | vanilla gen [path] | vanilla ids gen [repoRoot] [outPath] | vanilla ids reload [path]")
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

func printConsoleIntro(serverName, worldPath, listenAddr, apiBind string, apiEnabled bool) {
	fmt.Println("========================================")
	if strings.TrimSpace(serverName) == "" {
		serverName = "mdt-server"
	}
	fmt.Printf("服务器名称: %s\n", serverName)
	fmt.Printf("当前地图:   %s\n", worldPath)
	fmt.Printf("监听地址:   %s\n", listenAddr)
	if ip := firstLocalIPv4(); ip != "" {
		fmt.Printf("本机IP:     %s\n", ip)
	}
	if apiEnabled {
		fmt.Printf("API地址:    %s\n", apiBind)
	} else {
		fmt.Println("API地址:    已关闭")
	}
	fmt.Println("输入 `help all` 查看完整帮助")
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
		printHelpCmd("vanilla gen [path]", "从原版源码自动生成并加载 profiles.json")
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

type unitTypeRef struct {
	id int16
}

func (u unitTypeRef) ContentType() protocol.ContentType { return protocol.ContentUnit }
func (u unitTypeRef) ID() int16                         { return u.id }
func (u unitTypeRef) Name() string                      { return "" }

type bulletTypeRef struct {
	id int16
}

func (b bulletTypeRef) ContentType() protocol.ContentType { return protocol.ContentBullet }
func (b bulletTypeRef) ID() int16                         { return b.id }
func (b bulletTypeRef) Name() string                      { return "" }

type itemRef struct {
	id int16
}

func (i itemRef) ContentType() protocol.ContentType { return protocol.ContentItem }
func (i itemRef) ID() int16                         { return i.id }
func (i itemRef) Name() string                      { return "" }

type blockRef struct {
	id int16
}

func (b blockRef) ContentType() protocol.ContentType { return protocol.ContentBlock }
func (b blockRef) ID() int16                         { return b.id }
func (b blockRef) Name() string                      { return "" }

func maxf32(a, b float32) float32 {
	if a > b {
		return a
	}
	return b
}

func encodeTileConfigRaw(v any, ctx *protocol.TypeIOContext) ([]byte, error) {
	if v == nil {
		return nil, nil
	}
	w := protocol.NewWriterWithContext(ctx)
	if err := protocol.WriteObject(w, v, ctx); err != nil {
		return nil, err
	}
	raw := w.Bytes()
	if len(raw) == 0 {
		return nil, nil
	}
	out := make([]byte, len(raw))
	copy(out, raw)
	return out, nil
}

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

func broadcastBuildDestroyed(srv *netserver.Server, buildPos int32) {
	if srv == nil || buildPos < 0 {
		return
	}
	// Send beginBreak first (client->server request), then deconstructFinish (server->client confirmation)
	// For server-initiated broadcast, we send deconstructFinish directly
	srv.Broadcast(&protocol.Remote_ConstructBlock_deconstructFinish_136{
		Tile:    protocol.TileBox{PosValue: buildPos},
		Block:   protocol.BlockRef{BlkID: 0, BlkName: "air"}, // block ID unknown, use 0
		Builder: protocol.UnitBox{IDValue: 0},                // 0 for server
	})
	// Also send removeTile for compatibility
	srv.Broadcast(&protocol.Remote_Tile_removeTile_130{
		Tile: protocol.TileBox{PosValue: buildPos},
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

func broadcastBuildItems(srv *netserver.Server, buildPos int32, items []world.ItemStack) {
	if srv == nil || buildPos < 0 {
		return
	}
	out := make([]protocol.ItemStack, 0, len(items))
	for _, st := range items {
		if st.Amount <= 0 {
			continue
		}
		out = append(out, protocol.ItemStack{
			Item:   itemRef{id: int16(st.Item)},
			Amount: st.Amount,
		})
	}
	if len(out) == 0 {
		srv.Broadcast(&protocol.Remote_InputHandler_clearItems_63{
			Build: protocol.BuildingBox{PosValue: buildPos},
		})
		return
	}
	srv.Broadcast(&protocol.Remote_InputHandler_setItems_61{
		Build: protocol.BuildingBox{PosValue: buildPos},
		Items: out,
	})
}

func broadcastConstructFinish(srv *netserver.Server, buildPos int32, blockID int16, rot int8, team byte) {
	if srv == nil || buildPos < 0 || blockID <= 0 {
		return
	}
	// Compatibility path: avoid constructFinish(137), it still crashes some official clients.
	// Use setTile directly; sending remove+set for each completion can cause visible flicker.
	srv.Broadcast(&protocol.Remote_Tile_setTile_131{
		Tile:     protocol.TileBox{PosValue: buildPos},
		Block:    protocol.BlockRef{BlkID: blockID, BlkName: ""},
		Team:     protocol.Team{ID: team},
		Rotation: int32(rot) & 0x3,
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
			for _, candidateLocal := range []string{
				filepath.Join("assets", "worlds", base+".msav"),
				filepath.Join("go-server", "assets", "worlds", base+".msav"),
				filepath.Join("..", "assets", "worlds", base+".msav"),
			} {
				if exists(candidateLocal) {
					return candidateLocal, nil
				}
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

	for _, localMSAV := range []string{
		filepath.Join("assets", "worlds", trimmed+".msav"),
		filepath.Join("go-server", "assets", "worlds", trimmed+".msav"),
		filepath.Join("..", "assets", "worlds", trimmed+".msav"),
	} {
		if exists(localMSAV) {
			return localMSAV, nil
		}
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
	localCandidates := []string{
		filepath.Join("assets", "worlds", "*.msav"),
		filepath.Join("go-server", "assets", "worlds", "*.msav"),
		filepath.Join("..", "assets", "worlds", "*.msav"),
	}
	for _, g := range localCandidates {
		files, err := filepath.Glob(g)
		if err == nil && len(files) > 0 {
			return files[mathrand.New(mathrand.NewSource(time.Now().UnixNano())).Intn(len(files))], nil
		}
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
	localMsavGlobs := []string{
		filepath.Join("assets", "worlds", "*.msav"),
		filepath.Join("go-server", "assets", "worlds", "*.msav"),
		filepath.Join("..", "assets", "worlds", "*.msav"),
	}
	for _, g := range localMsavGlobs {
		files, err := filepath.Glob(g)
		if err != nil {
			return nil, err
		}
		for _, f := range files {
			outSet[worldstream.TrimMapName(filepath.Base(f))] = struct{}{}
		}
	}

	out := make([]string, 0, len(outSet))
	for name := range outSet {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
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
	mu             sync.Mutex
	path           string
	modTime        time.Time
	data           []byte
	corePos        protocol.Point2
	corePosOK      bool
	preloadEnabled bool
	maxBytes       int64
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
	cacheable := len(data) > 0
	if c.maxBytes > 0 && int64(len(data)) > c.maxBytes {
		cacheable = false
	}
	if cacheable {
		c.path = path
		c.modTime = info.ModTime()
		c.data = data
	} else {
		c.path = ""
		c.modTime = time.Time{}
		c.data = nil
	}
	c.corePosOK = false
	if strings.HasSuffix(strings.ToLower(path), ".msav") || strings.HasSuffix(strings.ToLower(path), ".msav.msav") {
		if pos, ok, err := worldstream.FindCoreTileFromMSAV(path); err == nil {
			c.corePos = pos
			c.corePosOK = ok
		}
	}
	return data, nil
}

func (c *worldCache) preload(path string) error {
	if c == nil || !c.preloadEnabled {
		return nil
	}
	_, err := c.get(path)
	return err
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
		filepath.Join("assets", "bootstrap-world.bin"),
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
