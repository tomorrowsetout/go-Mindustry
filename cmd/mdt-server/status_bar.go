package main

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"mdt-server/internal/config"
	netserver "mdt-server/internal/net"
)

type statusBarRuntimeConfig struct {
	Config         config.StatusBarConfig
	ServerName     string
	VirtualPlayers int
}

var runtimeStatusBarConfig atomic.Value

func initStatusBarRuntime(cfg config.Config) {
	runtimeStatusBarConfig.Store(statusBarRuntimeConfig{
		Config:         cfg.StatusBar,
		ServerName:     cfg.Runtime.ServerName,
		VirtualPlayers: cfg.Runtime.VirtualPlayers,
	})
}

func currentStatusBarRuntime() statusBarRuntimeConfig {
	if v := runtimeStatusBarConfig.Load(); v != nil {
		if cfg, ok := v.(statusBarRuntimeConfig); ok {
			return cfg
		}
	}
	return statusBarRuntimeConfig{Config: config.Default().StatusBar, ServerName: "mdt-server"}
}

func startStatusBarLoop(srv *netserver.Server) {
	if srv == nil {
		return
	}
	go func() {
		cpuTracker := newProcessCPUTracker()
		for {
			cfg := currentStatusBarRuntime()
			interval := time.Duration(cfg.Config.RefreshIntervalSec) * time.Second
			if interval <= 0 {
				interval = 2 * time.Second
			}
			if cfg.Config.Enabled {
				cpuPercent := cpuTracker.Sample()
				memoryMB := currentProcessMemoryMB()
				for _, c := range srv.ListConnectedConns() {
					if message := renderStatusBarMessage(cfg, srv, cpuPercent, memoryMB, c); strings.TrimSpace(message) != "" {
						srv.SendInfoPopup(
							c,
							message,
							float32(cfg.Config.PopupDurationMs)/1000,
							statusBarAlignValue(cfg.Config.Align),
							int32(cfg.Config.Top),
							int32(cfg.Config.Left),
							int32(cfg.Config.Bottom),
							int32(cfg.Config.Right),
						)
					}
				}
			}
			time.Sleep(interval)
		}
	}()
}

func renderStatusBarMessage(runtimeCfg statusBarRuntimeConfig, srv *netserver.Server, cpuPercent, memoryMB float64, c *netserver.Conn) string {
	cfg := runtimeCfg.Config
	players := connectedPlayerCount(srv) + max(runtimeCfg.VirtualPlayers, 0)
	repl := statusBarReplacer(
		runtimeCfg.ServerName,
		cpuPercent,
		memoryMB,
		players,
		currentStatusBarMapName(srv),
		currentStatusBarGameTime(srv),
		currentStatusBarPlayerName(srv, c),
		cfg.QQGroupText,
		cfg.CustomMessageText,
	)

	lines := make([]string, 0, 6)
	if cfg.HeaderEnabled {
		lines = appendIfNotBlank(lines, repl.Replace(cfg.HeaderText))
	}
	if cfg.ServerNameEnabled {
		lines = appendIfNotBlank(lines, repl.Replace(cfg.ServerNameFormat))
	}
	if cfg.PerformanceEnabled {
		lines = appendIfNotBlank(lines, repl.Replace(cfg.PerformanceFormat))
	}
	if cfg.CurrentMapEnabled {
		lines = appendIfNotBlank(lines, repl.Replace(cfg.CurrentMapFormat))
	}
	if cfg.GameTimeEnabled {
		lines = appendIfNotBlank(lines, repl.Replace(cfg.GameTimeFormat))
	}
	if cfg.PlayerCountEnabled {
		lines = appendIfNotBlank(lines, repl.Replace(cfg.PlayerCountFormat))
	}
	if cfg.WelcomeEnabled {
		lines = appendIfNotBlank(lines, repl.Replace(cfg.WelcomeFormat))
	}
	if cfg.QQGroupEnabled {
		lines = appendIfNotBlank(lines, repl.Replace(cfg.QQGroupFormat))
	}
	if cfg.CustomMessageEnabled {
		lines = appendIfNotBlank(lines, repl.Replace(cfg.CustomMessageFormat))
	}
	return strings.Join(lines, "\n")
}

func appendIfNotBlank(lines []string, line string) []string {
	if strings.TrimSpace(line) == "" {
		return lines
	}
	return append(lines, line)
}

func connectedPlayerCount(srv *netserver.Server) int {
	if srv == nil {
		return 0
	}
	count := 0
	for _, session := range srv.ListSessions() {
		if session.Connected {
			count++
		}
	}
	return count
}

func statusBarReplacer(serverName string, cpuPercent, memoryMB float64, players int, currentMap, gameTime, playerName, qqGroup, message string) *strings.Replacer {
	return strings.NewReplacer(
		"{server_name}", strings.TrimSpace(serverName),
		"{cpu_percent}", formatStatusBarFloat(cpuPercent),
		"{memory_mb}", formatStatusBarFloat(memoryMB),
		"{players}", strconv.Itoa(players),
		"{current_map}", strings.TrimSpace(currentMap),
		"{game_time}", strings.TrimSpace(gameTime),
		"{player_name}", strings.TrimSpace(playerName),
		"{qq_group}", strings.TrimSpace(qqGroup),
		"{message}", strings.TrimSpace(message),
		"{uptime}", time.Since(statusBarStartTime).Truncate(time.Second).String(),
	)
}

func currentStatusBarMapName(srv *netserver.Server) string {
	if srv == nil || srv.MapNameFn == nil {
		return "unknown"
	}
	name := strings.TrimSpace(srv.MapNameFn())
	if name == "" {
		return "unknown"
	}
	return name
}

func currentStatusBarGameTime(srv *netserver.Server) string {
	if srv != nil && srv.StateSnapshotFn != nil {
		if snap := srv.StateSnapshotFn(); snap != nil && snap.TimeData > 0 {
			return formatStatusBarDuration(time.Duration(snap.TimeData) * time.Second)
		}
	}
	return formatStatusBarDuration(time.Since(statusBarStartTime))
}

func currentStatusBarPlayerName(srv *netserver.Server, c *netserver.Conn) string {
	if c == nil {
		return "玩家"
	}
	if srv == nil {
		return strings.TrimSpace(c.Name())
	}
	name := strings.TrimSpace(srv.PlayerDisplayName(c))
	if name == "" {
		name = strings.TrimSpace(c.Name())
	}
	if name == "" {
		return "玩家"
	}
	return name
}

func formatStatusBarDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d.Seconds())
	hours := total / 3600
	minutes := (total % 3600) / 60
	seconds := total % 60
	if hours > 0 {
		return fmt.Sprintf("%02d:%02d:%02d", hours, minutes, seconds)
	}
	return fmt.Sprintf("%02d:%02d", minutes, seconds)
}

func formatStatusBarFloat(v float64) string {
	if v < 0 {
		v = 0
	}
	return fmt.Sprintf("%.1f", v)
}

func statusBarAlignValue(raw string) int32 {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "top_left", "topleft":
		return 10
	case "top":
		return 3
	case "top_right", "topright":
		return 18
	case "left":
		return 9
	case "center":
		return 1
	case "right":
		return 17
	case "bottom_left", "bottomleft":
		return 12
	case "bottom":
		return 5
	case "bottom_right", "bottomright":
		return 20
	default:
		return 10
	}
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

var statusBarStartTime = time.Now()
