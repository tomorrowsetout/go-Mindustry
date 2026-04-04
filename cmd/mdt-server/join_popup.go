package main

import (
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"mdt-server/internal/config"
	netserver "mdt-server/internal/net"
)

const (
	joinPopupMenuID    int32 = 910001
	helpPageBaseMenuID int32 = 910020
	helpPageSize             = 10
)

const (
	joinPopupOptionOpenURI int32 = 0
	joinPopupOptionHelp    int32 = 1
	joinPopupOptionVoteMap int32 = 2
	joinPopupOptionClose   int32 = 3
)

type joinPopupRuntimeConfig struct {
	Config         config.JoinPopupConfig
	ServerName     string
	VirtualPlayers int
}

type helpPage struct {
	Title   string
	Message string
	Buttons []helpCommandButton
}

type helpCommandButton struct {
	Label        string
	RunText      string
	UsageMessage string
}

var runtimeJoinPopupConfig atomic.Value

func initJoinPopupRuntime(cfg config.Config) {
	runtimeJoinPopupConfig.Store(joinPopupRuntimeConfig{
		Config:         cfg.JoinPopup,
		ServerName:     cfg.Runtime.ServerName,
		VirtualPlayers: cfg.Runtime.VirtualPlayers,
	})
}

func currentJoinPopupRuntime() joinPopupRuntimeConfig {
	if v := runtimeJoinPopupConfig.Load(); v != nil {
		if cfg, ok := v.(joinPopupRuntimeConfig); ok {
			return cfg
		}
	}
	return joinPopupRuntimeConfig{
		Config:         config.Default().JoinPopup,
		ServerName:     "mdt-server",
		VirtualPlayers: 0,
	}
}

func showJoinPopupForConn(srv *netserver.Server, c *netserver.Conn) {
	if srv == nil || c == nil {
		return
	}
	runtimeCfg := currentJoinPopupRuntime()
	cfg := runtimeCfg.Config
	if !cfg.Enabled {
		return
	}
	if delay := time.Duration(cfg.DelayMs) * time.Millisecond; delay > 0 {
		time.Sleep(delay)
	}
	showJoinPopupMenu(srv, c, runtimeCfg)
}

func showJoinPopupMenu(srv *netserver.Server, c *netserver.Conn, runtimeCfg joinPopupRuntimeConfig) {
	if srv == nil || c == nil {
		return
	}
	cfg := runtimeCfg.Config
	title := renderJoinPopupValue(runtimeCfg, srv, c, cfg.Title)
	message := buildJoinPopupMessage(runtimeCfg, srv, c)
	if strings.TrimSpace(title) == "" && strings.TrimSpace(message) == "" {
		return
	}
	srv.SendMenu(c, joinPopupMenuID, title, message, [][]string{
		{"打开链接", "帮助"},
		{"投票换图", "关闭"},
	})
}

func showHelpPageMenu(srv *netserver.Server, c *netserver.Conn, runtimeCfg joinPopupRuntimeConfig, page int) {
	if srv == nil || c == nil {
		return
	}
	pages := helpPages(runtimeCfg, srv, c)
	if len(pages) == 0 {
		return
	}
	if page < 0 {
		page = 0
	}
	if page >= len(pages) {
		page = len(pages) - 1
	}
	p := pages[page]
	srv.SendMenu(c, helpPageBaseMenuID+int32(page), p.Title, p.Message, helpPageOptions(p, page, len(pages)))
}

func helpPages(runtimeCfg joinPopupRuntimeConfig, srv *netserver.Server, c *netserver.Conn) []helpPage {
	intro := renderJoinPopupValue(runtimeCfg, srv, c, runtimeCfg.Config.HelpText)
	buttons := helpCommandButtons()
	if len(buttons) == 0 {
		return nil
	}
	totalPages := (len(buttons) + helpPageSize - 1) / helpPageSize
	pages := make([]helpPage, 0, totalPages)
	for page := 0; page < totalPages; page++ {
		start := page * helpPageSize
		end := min(start+helpPageSize, len(buttons))
		msgLines := make([]string, 0, 3)
		if page == 0 && strings.TrimSpace(intro) != "" {
			msgLines = append(msgLines, intro)
			msgLines = append(msgLines, "")
		}
		msgLines = append(msgLines, "[accent]点击下方命令按钮会直接执行对应命令。[]")
		msgLines = append(msgLines, "[gray]需要参数的命令会在聊天框提示用法。[]")
		pages = append(pages, helpPage{
			Title:   "[accent]帮助 " + strconv.Itoa(page+1) + "/" + strconv.Itoa(totalPages) + "[]",
			Message: strings.Join(compactHelpLines(msgLines), "\n"),
			Buttons: append([]helpCommandButton(nil), buttons[start:end]...),
		})
	}
	return pages
}

func helpCommandButtons() []helpCommandButton {
	return []helpCommandButton{
		{Label: "/help\n打开帮助", RunText: "/help"},
		{Label: "/status\n查看状态", RunText: "/status"},
		{Label: "/sync\n重新同步", RunText: "/sync"},
		{Label: "/votemap\n投票换图", RunText: "/votemap"},
		{Label: "/vote\n投票页面", RunText: "/vote"},
		{Label: "/kill\n清除单位", RunText: "/kill"},
		{Label: "/stop\nOP停服", RunText: "/stop"},
		{Label: "/summon\nOP召唤", UsageMessage: "[scarlet]用法: /summon <typeId|unitName> [x y] [count] [team][]"},
		{Label: "/despawn\nOP移除", UsageMessage: "[scarlet]用法: /despawn <entityId>[]"},
		{Label: "/umove\n单位速度", UsageMessage: "[scarlet]用法: /umove <entityId> <vx> <vy> [rotVel][]"},
		{Label: "/uteleport\n单位传送", UsageMessage: "[scarlet]用法: /uteleport <entityId> <x> <y> [rotation][]"},
		{Label: "/ulife\n单位寿命", UsageMessage: "[scarlet]用法: /ulife <entityId> <seconds>[]"},
		{Label: "/ufollow\n单位跟随", UsageMessage: "[scarlet]用法: /ufollow <entityId> <targetId> [speed][]"},
		{Label: "/upatrol\n单位巡逻", UsageMessage: "[scarlet]用法: /upatrol <entityId> <x1> <y1> <x2> <y2> [speed][]"},
		{Label: "/ubehavior\n清除行为", UsageMessage: "[scarlet]用法: /ubehavior clear <entityId>[]"},
	}
}

func compactHelpLines(lines []string) []string {
	out := make([]string, 0, len(lines))
	prevBlank := true
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			if !prevBlank {
				out = append(out, "")
			}
			prevBlank = true
			continue
		}
		out = append(out, line)
		prevBlank = false
	}
	if len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func helpPageOptions(p helpPage, page, total int) [][]string {
	options := make([][]string, 0, len(p.Buttons)+1)
	for _, button := range p.Buttons {
		options = append(options, []string{button.Label})
	}
	prevLabel := "[gray]上一页[]"
	nextLabel := "[gray]下一页[]"
	if page > 0 {
		prevLabel = "上一页"
	}
	if page+1 < total {
		nextLabel = "下一页"
	}
	options = append(options, []string{prevLabel, "关闭", nextLabel})
	return options
}

func handleJoinPopupMenuChoice(srv *netserver.Server, c *netserver.Conn, menuID, option int32) {
	if srv == nil || c == nil || option < 0 {
		return
	}
	if handleMapVoteMenuChoice(srv, c, menuID, option) {
		return
	}
	runtimeCfg := currentJoinPopupRuntime()
	switch menuID {
	case joinPopupMenuID:
		switch option {
		case joinPopupOptionOpenURI:
			uri := renderJoinPopupValue(runtimeCfg, srv, c, runtimeCfg.Config.LinkURL)
			if strings.TrimSpace(uri) == "" {
				srv.SendInfoMessage(c, "[scarlet]当前未配置公告链接。[]")
				return
			}
			srv.SendOpenURI(c, uri)
		case joinPopupOptionHelp:
			showHelpPageMenu(srv, c, runtimeCfg, 0)
		case joinPopupOptionVoteMap:
			showMapVoteMenu(srv, c, 0)
		case joinPopupOptionClose:
			return
		}
	default:
		handleHelpPageChoice(srv, c, runtimeCfg, menuID, option)
	}
}

func handleHelpPageChoice(srv *netserver.Server, c *netserver.Conn, runtimeCfg joinPopupRuntimeConfig, menuID, option int32) {
	pages := helpPages(runtimeCfg, srv, c)
	if len(pages) == 0 {
		return
	}
	page := int(menuID - helpPageBaseMenuID)
	if page < 0 || page >= len(pages) {
		return
	}
	buttons := pages[page].Buttons
	if option >= 0 && option < int32(len(buttons)) {
		runHelpCommandButton(srv, c, buttons[option])
		return
	}
	option -= int32(len(buttons))
	if option < 0 || option > 2 {
		return
	}
	switch option {
	case 0:
		if page > 0 {
			showHelpPageMenu(srv, c, runtimeCfg, page-1)
		}
	case 1:
		return
	case 2:
		if page+1 < len(pages) {
			showHelpPageMenu(srv, c, runtimeCfg, page+1)
		}
	}
}

func runHelpCommandButton(srv *netserver.Server, c *netserver.Conn, button helpCommandButton) {
	if srv == nil || c == nil {
		return
	}
	if strings.TrimSpace(button.RunText) != "" && srv.OnChat != nil {
		if srv.OnChat(c, button.RunText) {
			return
		}
	}
	if msg := strings.TrimSpace(button.UsageMessage); msg != "" {
		srv.SendChat(c, msg)
	}
}

func buildJoinPopupMessage(runtimeCfg joinPopupRuntimeConfig, srv *netserver.Server, c *netserver.Conn) string {
	intro := renderJoinPopupValue(runtimeCfg, srv, c, runtimeCfg.Config.Message)
	announcement := renderJoinPopupValue(runtimeCfg, srv, c, runtimeCfg.Config.AnnouncementText)
	switch {
	case strings.TrimSpace(intro) == "":
		return announcement
	case strings.TrimSpace(announcement) == "":
		return intro
	default:
		return intro + "\n\n" + announcement
	}
}

func renderJoinPopupValue(runtimeCfg joinPopupRuntimeConfig, srv *netserver.Server, c *netserver.Conn, raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	serverName := strings.TrimSpace(runtimeCfg.ServerName)
	if serverName == "" {
		serverName = "mdt-server"
	}
	playerName := strings.TrimSpace(currentStatusBarPlayerName(srv, c))
	if playerName == "" {
		playerName = "玩家"
	}
	currentMap := strings.TrimSpace(currentStatusBarMapName(srv))
	if currentMap == "" {
		currentMap = "unknown"
	}
	players := connectedPlayerCount(srv) + max(runtimeCfg.VirtualPlayers, 0)
	return strings.NewReplacer(
		"{server_name}", serverName,
		"{player_name}", playerName,
		"{current_map}", currentMap,
		"{players}", strconv.Itoa(players),
		"{link_url}", strings.TrimSpace(runtimeCfg.Config.LinkURL),
	).Replace(raw)
}
