package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mdt-server/internal/api"
	"mdt-server/internal/buildinfo"
	"mdt-server/internal/config"
	netserver "mdt-server/internal/net"
	"mdt-server/internal/storage"
	"mdt-server/internal/vanilla"
)

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
	pluginList func() []consolePluginRow,
	pluginReload func(target string) bool,
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
		case "plugin":
			if len(parts) < 2 {
				fmt.Println("用法: plugin list | plugin reload <author/id> | plugin reload *")
				continue
			}
			sub := strings.ToLower(parts[1])
			switch sub {
			case "list", "ls":
				rows := pluginList()
				if len(rows) == 0 {
					fmt.Println("当前没有已加载的 YF 插件")
					continue
				}
				for _, r := range rows {
					fmt.Printf("plugin %s name=%q runtime=%s version=%s category=%s enabled=%v source=%s\n",
						r.FullID, r.Name, r.Runtime, r.Version, r.Category, r.Enabled, r.Source)
				}
			case "reload":
				if len(parts) < 3 {
					fmt.Println("用法: plugin reload <author/id> | plugin reload *")
					continue
				}
				if ok := pluginReload(parts[2]); ok {
					fmt.Printf("已请求重载: %s\n", parts[2])
				} else {
					fmt.Printf("重载请求被拒绝（队列已满）: %s\n", parts[2])
				}
			default:
				fmt.Println("用法: plugin list | plugin reload <author/id> | plugin reload *")
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
				applyProcessConsoleTitle(*cfg, "", name)
				cfg.Runtime.ServerName = name
				if err := saveConfig(); err != nil {
					fmt.Printf("保存服务器名称失败: %v\n", err)
					continue
				}
				fmt.Printf("已设置服务器名称: %s\n", name)
			case "desc":
				if len(parts) < 3 {
					fmt.Println("用法: server desc <简介>")
					continue
				}
				desc := strings.TrimSpace(strings.Join(parts[2:], " "))
				srv.SetServerDescription(desc)
				cfg.Runtime.ServerDesc = desc
				if err := saveConfig(); err != nil {
					fmt.Printf("保存服务器简介失败: %v\n", err)
					continue
				}
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
				if err := saveConfig(); err != nil {
					fmt.Printf("保存虚拟人数失败: %v\n", err)
					continue
				}
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
		fmt.Printf("  snapshot: enabled=%v cold_dir=%s file=%s cold_interval=%ds hot_interval=%ds retention=%dd\n",
			cfg.Persist.Enabled, canonicalRuntimePath(cfg.Persist.Directory), cfg.Persist.File, cfg.Persist.IntervalSec, cfg.Persist.HotIntervalSec, cfg.Persist.RetentionDays)
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
