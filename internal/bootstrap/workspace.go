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

	// Ensure config exists on first launch for better discoverability.
	if strings.TrimSpace(cfgPath) != "" {
		if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
			if err := config.Save(cfgPath, cfg); err != nil {
				return out, err
			}
			out.CreatedFiles = append(out.CreatedFiles, cfgPath)
			createdFileSet[cfgPath] = struct{}{}
		} else if err != nil {
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
		_ = seedVanillaDataIfMissing(cfg.Runtime.VanillaProfiles, rootDir)
		_ = seedVanillaDataIfMissing(filepath.Join(filepath.Dir(cfg.Runtime.VanillaProfiles), "content_ids.json"), rootDir)
	}

	_ = writeIfMissing(filepath.Join(cfg.Mods.JSDir, "hello.js"), []byte("console.log('hello from mods/js/hello.js');\n"), 0o644)
	_ = writeIfMissing(filepath.Join(cfg.Mods.NodeDir, "hello.js"), []byte("console.log('hello from mods/node/hello.js');\n"), 0o644)
	_ = writeIfMissing(filepath.Join(cfg.Mods.GoDir, "hello.go"), []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello from mods/go/hello.go\")\n}\n"), 0o644)
	if strings.TrimSpace(cfg.API.ConfigFile) != "" {
		apiCfgPath := strings.TrimSpace(cfg.API.ConfigFile)
		if !filepath.IsAbs(apiCfgPath) {
			apiCfgPath = filepath.Join(configDir, apiCfgPath)
		}
		_ = writeIfMissing(apiCfgPath, []byte("; api settings\n[api]\nenabled = 1\nbind = 0.0.0.0:8090\nkey =\nkeys =\n"), 0o644)
	}
	_ = writeIfMissing(filepath.Join(configDir, "Development mode.ini"), []byte("; 开发模式配置\n; 1 = 开启，0 = 关闭\n\n[development]\npacket_events_enabled = 0 ; 数据包事件兼容总开关，实际以 recv/send 两项为准\npacket_recv_events_enabled = 0 ; 记录 packet_recv 事件\npacket_send_events_enabled = 0 ; 记录 packet_send 事件\nterminal_player_logs_enabled = 1 ; 控制 [终端] 玩家进入/退出游戏日志\nterminal_player_uuid_enabled = 0 ; 控制 [终端] 玩家进入/退出游戏日志是否显示真实 UUID\nrespawn_core_logs_enabled = 1 ; 控制核心坐标、出生点、未找到核心等日志\nrespawn_unit_logs_enabled = 1 ; 控制出生单位、建造速度等日志\nrespawn_packet_logs_enabled = 1 ; 控制玩家出生包发送开始/失败/完成日志\nbuild_snapshot_logs_enabled = 1 ; 控制 [建筑] 快照队列日志\nbuild_place_logs_enabled = 1 ; 控制 [建筑] 建造了 日志\nbuild_finish_logs_enabled = 1 ; 控制 [建筑] 完成建造 日志\nbuild_break_start_logs_enabled = 1 ; 控制 [建筑] 正在拆除 日志\nbuild_break_done_logs_enabled = 1 ; 控制 [建筑] 拆除了 日志\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "Personalization.ini"), []byte("; 个性化显示配置\n; 1 = 开启，0 = 关闭\n\n[personalization]\nstartup_report_enabled = 1 ; 控制启动报告输出\nmap_load_details_enabled = 1 ; 控制地图加载详情输出\nunit_id_list_enabled = 1 ; 控制单位 ID 列表输出\nstartup_current_map_line_enabled = 1 ; 控制启动时单独输出 当前地图: ...\nconsole_intro_enabled = 1 ; 控制启动后的信息面板总开关\nconsole_intro_server_name_enabled = 1 ; 控制信息面板中的 服务器名称\nconsole_intro_current_map_enabled = 1 ; 控制信息面板中的 当前地图\nconsole_intro_listen_addr_enabled = 1 ; 控制信息面板中的 监听地址\nconsole_intro_local_ip_enabled = 1 ; 控制信息面板中的 本机IP\nconsole_intro_api_enabled = 1 ; 控制信息面板中的 API地址\nconsole_intro_help_hint_enabled = 1 ; 控制信息面板中的 help all 提示\nstartup_help_enabled = 1 ; 控制启动时完整帮助列表输出\njoin_leave_chat_enabled = 1 ; 控制玩家加入/退出时是否向全服发送聊天提示\nplayer_name_color_enabled = 1 ; 控制终端中玩家名称是否保留颜色显示\nplayer_name_prefix =  ; 玩家显示名前缀，可写 Mindustry 颜色标签\nplayer_name_suffix =  ; 玩家显示名后缀，可写 Mindustry 颜色标签\nplayer_bind_prefix_enabled = 1 ; 控制是否在玩家名前显示 已绑定/未绑定 前缀\nplayer_bound_prefix = [green]（已绑定）[] ; 已绑定玩家名前缀，可写 Mindustry 颜色标签\nplayer_unbound_prefix = [scarlet]（未绑定）[] ; 未绑定玩家名前缀，可写 Mindustry 颜色标签\nplayer_title_enabled = 1 ; 控制是否读取 json/player_identity.json 中的自定义头衔\nplayer_identity_file = json/player_identity.json ; 玩家身份配置文件，只读，按 conn_uuid 识别，位于 configs 目录内\nplayer_bind_source = internal ; 玩家绑定识别来源：internal=读取身份文件，api=通过接口查询 yes/no\nplayer_bind_api_url =  ; 绑定查询接口地址，使用 {id} 代替 conn_uuid\nplayer_bind_api_timeout_ms = 1500 ; 绑定查询接口超时（毫秒）\nplayer_bind_api_cache_sec = 30 ; 绑定查询接口缓存秒数，避免频繁请求\nplayer_conn_id_suffix_enabled = 1 ; 控制是否在玩家名后显示 connID/conn_uuid 后缀\nplayer_conn_id_suffix_format =  [gray]{id}[] ; 玩家名后缀格式，使用 {id} 代表 connID 或 conn_uuid\n"), 0o644)
	_ = writeIfMissing(filepath.Join(configDir, "Status bar.ini"), []byte("; 服务器状态栏配置\n; 使用 infoPopup 在左上区域持续刷新显示，可贴近下一波下方\n; 1 = 开启，0 = 关闭\n; 可用占位符: {server_name} {cpu_percent} {memory_mb} {players} {qq_group} {message} {uptime} {current_map} {game_time} {player_name}\n\n[status_bar]\nenabled = 1 ; 状态栏总开关\nrefresh_interval_sec = 2 ; 刷新周期（秒）\npopup_duration_ms = 2200 ; 单次显示持续时间（毫秒），建议略大于刷新周期\nalign = top_left ; 对齐方式：top_left / top / center / bottom_right 等\ntop = 155 ; 顶部偏移，156 常见状态栏位置\nleft = 0 ; 左侧偏移\nbottom = 0 ; 底部偏移\nright = 0 ; 右侧偏移\npopup_id = server-status-bar ; 保留字段，当前使用无 ID popup 包\nheader_enabled = 1 ; 标题行开关\nheader_text = [accent]服务器状态[] ; 标题行内容\nserver_name_enabled = 1 ; 服务器名称行开关\nserver_name_format = [green]服务器: [white]{server_name}[] ; 服务器名称行模板\nperformance_enabled = 1 ; CPU/内存行开关\nperformance_format = [green]性能: [white]CPU {cpu_percent}%[] [white]进程内存 {memory_mb} MB[] ; CPU/进程内存行模板\ncurrent_map_enabled = 1 ; 当前地图行开关\ncurrent_map_format = [green]当前地图: [white]{current_map}[] ; 当前地图行模板\ngame_time_enabled = 1 ; 本局游戏时间行开关\ngame_time_format = [green]本局时间: [white]{game_time}[] ; 本局游戏时间行模板\nplayer_count_enabled = 1 ; 在线人数行开关\nplayer_count_format = [green]在线人数: [white]{players}[] ; 在线人数行模板\nwelcome_enabled = 1 ; 欢迎语行开关\nwelcome_format = [gold]欢迎玩家 {player_name} 来到镜像物语[] ; 欢迎语行模板\nqq_group_enabled = 1 ; QQ群行开关\nqq_group_text = 请在这里填写QQ群 ; QQ群号码或说明\nqq_group_format = [green]QQ群: [white]{qq_group}[] ; QQ群行模板\ncustom_message_enabled = 1 ; 自定义文案行开关\ncustom_message_text = 请在这里填写服务器公告 ; 自定义文案内容\ncustom_message_format = [gold]{message}[] ; 自定义文案行模板\n"), 0o644)
	_ = writeBundledConfigIfMissing(configDir, filepath.Join("json", "block_names.json"))
	if cfg.Control.ConnUUIDAutoCreateEnabled {
		_ = writeBundledConfigIfMissing(configDir, filepath.Join("json", "conn_uuid.json"))
	}
	if cfg.Control.PlayerIdentityAutoCreateEnabled {
		_ = writeBundledConfigIfMissing(configDir, filepath.Join("json", "player_identity.json"))
	}

	if err := seedMapIfMissing(cfg.Runtime.WorldsDir); err != nil {
		return out, err
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
