package bootstrap

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	mdtserver "mdt-server"
	"mdt-server/internal/config"
)

type Result struct {
	CreatedDirs  []string
	CreatedFiles []string
}

func EnsureWorkspace(cfgPath string, cfg config.Config) (Result, error) {
	var out Result
	createdDirSet := map[string]struct{}{}
	createdFileSet := map[string]struct{}{}
	configDir := filepath.FromSlash("configs")
	rootDir := "."
	if p := strings.TrimSpace(cfgPath); p != "" {
		configDir = filepath.Dir(p)
		rootDir = filepath.Dir(configDir)
	}
	if strings.TrimSpace(configDir) == "" {
		configDir = filepath.FromSlash("configs")
	}
	if strings.TrimSpace(rootDir) == "" || rootDir == configDir {
		rootDir = "."
	}
	toRoot := func(p string) string {
		p = strings.TrimSpace(p)
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(rootDir, p)
	}

	mkdir := func(dir string) error {
		dir = strings.TrimSpace(dir)
		if dir == "" {
			return nil
		}
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
		if _, ok := createdDirSet[dir]; !ok {
			out.CreatedDirs = append(out.CreatedDirs, dir)
			createdDirSet[dir] = struct{}{}
		}
		return nil
	}
	writeIfMissing := func(path string, data []byte, mode os.FileMode) error {
		path = strings.TrimSpace(path)
		if path == "" {
			return nil
		}
		if st, err := os.Stat(path); err == nil {
			if st.IsDir() {
				return fmt.Errorf("path is directory: %s", path)
			}
			return nil
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := mkdir(filepath.Dir(path)); err != nil {
			return err
		}
		if err := os.WriteFile(path, data, mode); err != nil {
			return err
		}
		if _, ok := createdFileSet[path]; !ok {
			out.CreatedFiles = append(out.CreatedFiles, path)
			createdFileSet[path] = struct{}{}
		}
		return nil
	}

	// Main config is user-owned; bootstrap should not rewrite or auto-create it.
	if strings.TrimSpace(cfgPath) != "" {
		if _, err := os.Stat(cfgPath); err != nil && !os.IsNotExist(err) {
			return out, err
		}
	}
	policy, err := loadReleasePolicy(configDir)
	if err != nil {
		return out, err
	}

	dirs := []string{
		configDir, // configs 目录存放 INI 与 JSON 配置/字典文件
		filepath.Join(configDir, "json"),
		cfg.Runtime.WorldsDir,
		cfg.Runtime.LogsDir,
		cfg.Storage.Directory,
		filepath.Join(cfg.Storage.Directory, "players"),
		cfg.Persist.Directory,
		cfg.Persist.MSAVDir,
		cfg.Mods.Directory,
		cfg.Mods.JSDir,
		cfg.Mods.NodeDir,
		cfg.Mods.GoDir,
		filepath.Dir(strings.TrimSpace(cfg.Script.File)),
		filepath.Dir(strings.TrimSpace(cfg.Admin.OpsFile)),
		filepath.Dir(strings.TrimSpace(cfg.Runtime.VanillaProfiles)),
	}
	dirs = append(dirs,
		filepath.Dir(cfgPath),
	)
	dirs = uniqDirs(dirs)
	for _, d := range dirs {
		if err := mkdir(d); err != nil {
			return out, err
		}
	}

	_ = writeIfMissing(filepath.Join(cfg.Storage.Directory, ".keep"), []byte(""), 0o644)
	_ = writeIfMissing(filepath.Join(cfg.Storage.Directory, "players", ".keep"), []byte(""), 0o644)
	_ = writeIfMissing(filepath.Join(cfg.Storage.Directory, "all.jsonl"), []byte(""), 0o644)

	scriptPayload := map[string]any{
		"version":       1,
		"startup_tasks": []any{},
		"daily_gc_time": "",
		"updated_at":    time.Now().UTC().Format(time.RFC3339),
	}
	if b, err := json.MarshalIndent(scriptPayload, "", "  "); err == nil {
		_ = writeIfMissing(cfg.Script.File, b, 0o644)
	}
	opsPayload := map[string]any{
		"ops":      []any{},
		"saved_at": time.Now().UTC().Format(time.RFC3339),
	}
	if b, err := json.MarshalIndent(opsPayload, "", "  "); err == nil {
		_ = writeIfMissing(cfg.Admin.OpsFile, b, 0o644)
	}
	statePayload := map[string]any{
		"map_path":  "",
		"wave_time": 0,
		"wave":      0,
		"tick":      0,
		"time_data": 0,
		"rand0":     0,
		"rand1":     0,
		"saved_at":  "",
	}
	if b, err := json.MarshalIndent(statePayload, "", "  "); err == nil {
		_ = writeIfMissing(filepath.Join(cfg.Persist.Directory, cfg.Persist.File), b, 0o644)
	}
	if strings.TrimSpace(cfg.Runtime.VanillaProfiles) != "" {
		_ = writeBundledRuntimeFileIfMissing(rootDir, cfg.Runtime.VanillaProfiles)
		_ = writeBundledRuntimeFileIfMissing(rootDir, filepath.Join(filepath.Dir(cfg.Runtime.VanillaProfiles), "content_ids.json"))
	}

	_ = writeIfMissing(filepath.Join(cfg.Mods.JSDir, "hello.js"), []byte("console.log('hello from mods/js/hello.js');\n"), 0o644)
	_ = writeIfMissing(filepath.Join(cfg.Mods.NodeDir, "hello.js"), []byte("console.log('hello from mods/node/hello.js');\n"), 0o644)
	_ = writeIfMissing(filepath.Join(cfg.Mods.GoDir, "hello.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello from mods/go/hello.go\")\n}\n"), 0o644)
	if strings.TrimSpace(cfg.API.ConfigFile) != "" {
		apiCfgPath := strings.TrimSpace(cfg.API.ConfigFile)
		if !filepath.IsAbs(apiCfgPath) {
			apiCfgPath = filepath.Join(configDir, apiCfgPath)
		}
		_ = writeIfMissing(apiCfgPath, []byte("# API 配置\n# 控制内置 HTTP API 的监听地址与鉴权密钥。\n\n[api]\nenabled = true\nbind = \"0.0.0.0:8090\"\nkey = \"\"\nkeys = []\n"), 0o644)
	}
	_ = writeIfMissing(filepath.Join(configDir, "core.toml"), []byte("# 核心配置\n# tps 为服务端逻辑帧率上限，默认 60，最高 120。\n\n[core]\ndual_core_enabled = true\ntps = 60\n\n[memory]\nlimit_mb = 0\nstartup_max_mb = 0\ngc_trigger_mb = 0\ncheck_interval_sec = 5\nfree_os_memory = false\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "server.toml"), []byte("# 服务器基础配置\n# 包含服务器名称、简介和虚拟在线人数显示。\n\n[server]\nname = \"mdt-server\"\ndesc = \"\"\nvirtual_players = 0\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "sync.toml"), []byte("# 同步配置\n# entity_ms 与 state_ms 默认按原版思路统一为 200ms。\n\n[sync]\nentity_ms = 200\nstate_ms = 200\nudp_retry_count = 2\nudp_retry_delay_ms = 5\nudp_fallback_tcp = true\nuse_map_sync_data_fallback = false\nblock_sync_logs_enabled = false\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "misc.toml"), []byte("# 杂项配置\n# 包含数据目录、mods、持久化、脚本与管理文件路径。\n\n[data]\nmode = \"file\"\ndirectory = \"data/events\"\ndatabase_enabled = false\ndsn = \"\"\n\n[paths]\nassets_dir = \"assets\"\nworlds_dir = \"assets/worlds\"\nlogs_dir = \"logs\"\n\n[mods]\nenabled = false\ndirectory = \"mods\"\njava_home = \"\"\njs_dir = \"mods/js\"\ngo_dir = \"mods/go\"\nnode_dir = \"mods/node\"\n\n[persist]\nenabled = true\ndirectory = \"data/state\"\nfile = \"server-state.json\"\ninterval_sec = 30\nsave_msav = true\nmsav_dir = \"data/snapshots\"\nmsav_file = \"\"\n\n[script]\nfile = \"data/state/scripts.json\"\ndaily_gc_time = \"\"\n\n[admin]\nops_file = \"configs/json/ops.json\"\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "sundries.toml"), []byte("# 附加日志配置\n# 控制文件日志大小、数量和若干运行日志输出。\n\n[sundries]\ndetailed_log_max_mb = 2\ndetailed_log_max_files = 100\nnet_event_logs_enabled = true\nchat_logs_enabled = true\nrespawn_core_logs_enabled = true\nrespawn_unit_logs_enabled = true\nbuild_place_logs_enabled = true\nbuild_finish_logs_enabled = true\nbuild_break_start_logs_enabled = true\nbuild_break_done_logs_enabled = true\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "development.toml"), []byte("# 开发调试配置\n# 主要控制控制台调试输出与开发期日志开关。\n\n[development]\npacket_events_enabled = false\npacket_recv_events_enabled = false\npacket_send_events_enabled = false\nterminal_player_logs_enabled = true\nterminal_player_uuid_enabled = false\nrespawn_core_logs_enabled = true\nrespawn_unit_logs_enabled = true\nrespawn_packet_logs_enabled = true\nbuild_snapshot_logs_enabled = true\nbuild_place_logs_enabled = true\nbuild_finish_logs_enabled = true\nbuild_break_start_logs_enabled = true\nbuild_break_done_logs_enabled = true\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "personalization.toml"), []byte("# 个性化显示配置\n# 控制启动报告、控制台展示、玩家名前后缀与控制台标题等显示行为。\n\n[personalization]\nstartup_report_enabled = true\nmap_load_details_enabled = true\nunit_id_list_enabled = true\nstartup_current_map_line_enabled = true\nconsole_intro_enabled = true\nconsole_intro_server_name_enabled = true\nconsole_intro_current_map_enabled = true\nconsole_intro_listen_addr_enabled = true\nconsole_intro_local_ip_enabled = true\nconsole_intro_api_enabled = true\nconsole_intro_help_hint_enabled = true\nstartup_help_enabled = true\njoin_leave_chat_enabled = true\nplayer_name_color_enabled = true\nplayer_name_prefix = \"\"\nplayer_name_suffix = \"\"\nplayer_bind_prefix_enabled = true\nplayer_bound_prefix = \"[green]（已绑定）[]\"\nplayer_unbound_prefix = \"[scarlet]（未绑定）[]\"\nplayer_title_enabled = true\nplayer_identity_file = \"json/player_identity.json\"\nplayer_bind_source = \"internal\"\nplayer_bind_api_url = \"\"\nplayer_bind_api_timeout_ms = 1500\nplayer_bind_api_cache_sec = 30\nplayer_conn_id_suffix_enabled = true\nplayer_conn_id_suffix_format = \" [gray]{id}[]\"\nmain_console_title = \"mdt-server | 主进程 | {server_name}\"\ncore2_console_title = \"mdt-server | Core2 | IO\"\ncore3_console_title = \"mdt-server | Core3 | Snapshot\"\ncore4_console_title = \"mdt-server | Core4 | Policy\"\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "join_popup.toml"), []byte("# 入服公告配置\n# 控制玩家进入服务器后的弹窗与帮助文案。\n\n[join_popup]\nenabled = true\ndelay_ms = 300\ntitle = \"[accent]服务器公告[]\"\nmessage = \"\"\"\n欢迎 [green]{player_name}[] 来到 [white]{server_name}[]\n当前地图: [white]{current_map}[]\n请选择下方按钮。\n\"\"\"\nannouncement_text = \"\"\"\n[accent]服务器公告[]\n\n1. 请遵守服务器规则。\n2. 如有问题可先查看快速帮助。\n3. 需要联系管理时请使用群或外部链接。\n\"\"\"\nlink_url = \"https://example.com\"\nhelp_text = \"\"\"\n[accent]帮助为分页弹窗[]\n第 1 页显示基础指令与投票换图；\n第 2 页显示常用与 OP 指令；\n第 3 页显示单位控制。\n\"\"\"\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "status_bar.toml"), []byte("# 状态栏配置\n# 控制定时弹出的服务器状态栏内容与布局。\n\n[status_bar]\nenabled = true\nrefresh_interval_sec = 2\npopup_duration_ms = 2200\nalign = \"top_left\"\ntop = 155\nleft = 0\nbottom = 0\nright = 0\npopup_id = \"server-status-bar\"\nheader_enabled = true\nheader_text = \"[accent]服务器状态[]\"\nserver_name_enabled = true\nserver_name_format = \"[green]服务器: [white]{server_name}[]\"\nperformance_enabled = true\nperformance_format = \"[green]性能: [white]CPU {cpu_percent}%[] [white]进程内存 {memory_mb} MB[]\"\ncurrent_map_enabled = true\ncurrent_map_format = \"[green]当前地图: [white]{current_map}[]\"\ngame_time_enabled = true\ngame_time_format = \"[green]本局时间: [white]{game_time}[]\"\nplayer_count_enabled = true\nplayer_count_format = \"[green]在线人数: [white]{players}[]\"\nwelcome_enabled = true\nwelcome_format = \"[gold]欢迎玩家 {player_name} 来到镜像物语[]\"\nqq_group_enabled = true\nqq_group_text = \"请在这里填写QQ群\"\nqq_group_format = \"[green]QQ群: [white]{qq_group}[]\"\ncustom_message_enabled = true\ncustom_message_text = \"请在这里填写服务器公告\"\ncustom_message_format = \"[gold]{message}[]\"\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "map_vote.toml"), []byte("# 地图投票配置\n# 控制投票时长、状态刷新频率与弹窗位置。\n\n[map_vote]\nduration_sec = 15\nstatus_refresh_ms = 1500\npopup_duration_ms = 1800\nhome_link_url = \"https://example.com/votemap\"\nalign = \"top_left\"\ntop = 220\nleft = 0\nbottom = 0\nright = 0\n"), 0o644)
	_ = writeBundledConfigIfMissing(configDir, filepath.Join("json", "block_names.json"))
	if cfg.Control.ConnUUIDAutoCreateEnabled {
		_ = writeBundledConfigIfMissing(configDir, filepath.Join("json", "conn_uuid.json"))
	}
	if cfg.Control.PlayerIdentityAutoCreateEnabled {
		_ = writeBundledConfigIfMissing(configDir, filepath.Join("json", "player_identity.json"))
	}

	if shouldReleaseEmbedded(policy) {
		if err := releaseEmbeddedConfigs(configDir); err != nil {
			return out, err
		}
		worldsDirAbs := toRoot(cfg.Runtime.WorldsDir)
		if err := releaseEmbeddedWorlds(worldsDirAbs); err != nil {
			return out, err
		}
		if err := markEmbeddedReleasedAt(configDir, policy); err != nil {
			return out, err
		}
	}
	if err := seedMapIfMissing(toRoot(cfg.Runtime.WorldsDir)); err != nil {
		return out, err
	}

	return out, nil
}

func seedMapIfMissing(dstDir string) error {
	msav, err := filepath.Glob(filepath.Join(dstDir, "*.msav"))
	if err == nil && len(msav) > 0 {
		return nil
	}
	candidates := []string{
		filepath.Join("..", "assets", "worlds", "23315.msav"),
		filepath.Join("go-server", "assets", "worlds", "23315.msav"),
		filepath.Join("..", "go-server", "assets", "worlds", "23315.msav"),
		filepath.Join(filepath.Dir(dstDir), "worlds", "file.msav"),
		filepath.Join("..", "assets", "worlds", "file.msav"),
	}
	var src string
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			src = c
			break
		}
	}
	if src == "" {
		return nil
	}
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}
	dst := filepath.Join(dstDir, filepath.Base(src))
	if st, err := os.Stat(dst); err == nil && !st.IsDir() {
		return nil
	}
	return copyFile(src, dst)
}

func seedVanillaDataIfMissing(dstPath, rootDir string) error {
	dstPath = strings.TrimSpace(dstPath)
	if dstPath == "" {
		return nil
	}
	if st, err := os.Stat(dstPath); err == nil {
		if st.IsDir() {
			return fmt.Errorf("path is directory: %s", dstPath)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	candidates := []string{
		filepath.Join(rootDir, "data", "vanilla", filepath.Base(dstPath)),
		filepath.Join(rootDir, "..", "data", "vanilla", filepath.Base(dstPath)),
		filepath.Join(rootDir, "..", "..", "data", "vanilla", filepath.Base(dstPath)),
	}
	for _, src := range candidates {
		if st, err := os.Stat(src); err == nil && !st.IsDir() {
			if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
				return err
			}
			return copyFile(src, dstPath)
		}
	}
	return nil
}

func writeBundledRuntimeFileIfMissing(rootDir, rel string) error {
	rootDir = strings.TrimSpace(rootDir)
	rel = filepath.Clean(strings.TrimSpace(rel))
	if rel == "" || rel == "." {
		return nil
	}
	target := rel
	if rootDir != "" && rootDir != "." && !filepath.IsAbs(target) {
		target = filepath.Join(rootDir, rel)
	}
	if st, err := os.Stat(target); err == nil && !st.IsDir() {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	bundledRel := rel
	if filepath.IsAbs(bundledRel) {
		if rootDir != "" {
			if rootAbs, err := filepath.Abs(rootDir); err == nil {
				if targetAbs, terr := filepath.Abs(bundledRel); terr == nil {
					if r, rerr := filepath.Rel(rootAbs, targetAbs); rerr == nil &&
						r != "." && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
						bundledRel = r
					}
				}
			}
		}
	}
	bundledPath := filepath.ToSlash(filepath.Clean(bundledRel))
	data, err := fs.ReadFile(mdtserver.BundledFiles, bundledPath)
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func writeBundledConfigIfMissing(configDir, rel string) error {
	configDir = strings.TrimSpace(configDir)
	rel = filepath.Clean(strings.TrimSpace(rel))
	if configDir == "" || rel == "" || rel == "." {
		return nil
	}
	target := filepath.Join(configDir, rel)
	if st, err := os.Stat(target); err == nil && !st.IsDir() {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	bundledPath := filepath.ToSlash(filepath.Join("configs", rel))
	data, err := fs.ReadFile(mdtserver.BundledFiles, bundledPath)
	if err != nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
