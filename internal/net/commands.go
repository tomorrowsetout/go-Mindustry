package net

import (
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"mdt-server/internal/protocol"
)

var globalServer *Server

// CommandHandler 处理客户端命令
type CommandHandler struct {
	mu        sync.RWMutex
	commands  map[string]*Command
	aliases   map[string]string
	cooldowns map[string]*time.Time
}

type Command struct {
	Name        string
	Aliases     []string
	Description string
	Usage       string
	Handler     func(*Conn, []string) error
	AdminOnly   bool
}

var globalCommandHandler *CommandHandler

// NewCommandHandler 创建命令处理器
func NewCommandHandler() *CommandHandler {
	return &CommandHandler{
		commands:  make(map[string]*Command),
		aliases:   make(map[string]string),
		cooldowns: make(map[string]*time.Time),
	}
}

// RegisterCommand 注册命令
func (h *CommandHandler) RegisterCommand(cmd *Command) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.commands[cmd.Name] = cmd
	for _, alias := range cmd.Aliases {
		h.aliases[alias] = cmd.Name
	}
}

// HandleCommand 处理命令
func (h *CommandHandler) HandleCommand(c *Conn, message string) bool {
	if !strings.HasPrefix(message, "/") {
		return false
	}

	parts := strings.Fields(message)
	if len(parts) == 0 {
		return true
	}

	cmdName := strings.TrimPrefix(parts[0], "/")
	args := parts[1:]

	h.mu.RLock()
	defer h.mu.RUnlock()

	// 检查别名
	if realName, ok := h.aliases[cmdName]; ok {
		cmdName = realName
	}

	cmd, ok := h.commands[cmdName]
	if !ok {
		// 尝试模糊匹配
		for name, cmdObj := range h.commands {
			if strings.HasPrefix(name, cmdName) || strings.HasPrefix(cmdName, name) {
				h.sendCommandHelp(c, cmdObj)
				return true
			}
		}
		h.sendUnknownCommand(c, cmdName)
		return true
	}

	// 检查管理员权限
	if cmd.AdminOnly && !c.isAdmin() {
		h.sendAdminOnly(c)
		return true
	}

	// 执行命令
	if err := cmd.Handler(c, args); err != nil {
		h.sendCommandError(c, cmdName, err.Error())
	}

	return true
}

// sendCommandHelp 发送命令帮助
func (h *CommandHandler) sendCommandHelp(c *Conn, cmd *Command) {
	msg := fmt.Sprintf("[orange]%s[white] %s", cmd.Name, cmd.Usage)
	if cmd.Description != "" {
		msg += fmt.Sprintf(" - [lightgray]%s", cmd.Description)
	}
	c.SendChat(msg)
}

// sendUnknownCommand 发送未知命令消息
func (h *CommandHandler) sendUnknownCommand(c *Conn, cmdName string) {
	c.SendChat("[scarlet]Unknown command. Check [lightgray]/help[scarlet].")
}

// sendAdminOnly 发送仅管理员可用消息
func (h *CommandHandler) sendAdminOnly(c *Conn) {
	c.SendChat("[scarlet]You must be an admin to use this command.")
}

// sendCommandError 发送命令错误消息
func (h *CommandHandler) sendCommandError(c *Conn, cmdName, error string) {
	c.SendChat(fmt.Sprintf("[scarlet]%s: [lightgray]%s", cmdName, error))
}

// RegisterDefaultCommands 注册默认命令
func (h *CommandHandler) RegisterDefaultCommands(srv *Server) {
	// Help command
	h.RegisterCommand(&Command{
		Name:        "help",
		Aliases:     []string{"?", "h"},
		Description: "Lists all commands.",
		Usage:       "/help [page]",
		Handler:     func(c *Conn, args []string) error { return h.handleHelp(c, args) },
	})

	// Team chat
	h.RegisterCommand(&Command{
		Name:        "t",
		Description: "Send a message only to your teammates.",
		Usage:       "/t <message...>",
		Handler:     func(c *Conn, args []string) error { return h.handleTeamChat(c, args) },
	})

	// Admin chat
	h.RegisterCommand(&Command{
		Name:        "a",
		Description: "Send a message only to admins.",
		Usage:       "/a <message...>",
		Handler:     func(c *Conn, args []string) error { return h.handleAdminChat(c, args) },
		AdminOnly:   true,
	})

	// Sync command
	h.RegisterCommand(&Command{
		Name:        "sync",
		Description: "Re-synchronize world state.",
		Usage:       "/sync",
		Handler:     func(c *Conn, args []string) error { return h.handleSync(c, args) },
	})

	// Info command
	h.RegisterCommand(&Command{
		Name:        "info",
		Description: "Show server information.",
		Usage:       "/info",
		Handler:     func(c *Conn, args []string) error { return h.handleInfo(c, srv) },
	})

	// Admin commands
	h.RegisterCommand(&Command{
		Name:        "kick",
		Description: "Kick a player from the server.",
		Usage:       "/kick <player> [reason]",
		Handler:     func(c *Conn, args []string) error { return h.handleKick(c, srv, args) },
		AdminOnly:   true,
	})

	h.RegisterCommand(&Command{
		Name:        "ban",
		Description: "Ban a player from the server.",
		Usage:       "/ban <player> [reason]",
		Handler:     func(c *Conn, args []string) error { return h.handleBan(c, srv, args) },
		AdminOnly:   true,
	})

	h.RegisterCommand(&Command{
		Name:        "unban",
		Description: "Unban a player.",
		Usage:       "/unban <player>",
		Handler:     func(c *Conn, args []string) error { return h.handleUnban(c, srv, args) },
		AdminOnly:   true,
	})

	h.RegisterCommand(&Command{
		Name:        "bans",
		Description: "List all banned players.",
		Usage:       "/bans",
		Handler:     func(c *Conn, args []string) error { return h.handleBans(c, srv) },
		AdminOnly:   true,
	})

	h.RegisterCommand(&Command{
		Name:        "ops",
		Description: "List all operators.",
		Usage:       "/ops",
		Handler:     func(c *Conn, args []string) error { return h.handleOps(c, srv) },
		AdminOnly:   true,
	})

	h.RegisterCommand(&Command{
		Name:        "op",
		Description: "Add a player as operator.",
		Usage:       "/op <player>",
		Handler:     func(c *Conn, args []string) error { return h.handleOp(c, srv, args) },
		AdminOnly:   true,
	})

	h.RegisterCommand(&Command{
		Name:        "deop",
		Description: "Remove a player as operator.",
		Usage:       "/deop <player>",
		Handler:     func(c *Conn, args []string) error { return h.handleDeop(c, srv, args) },
		AdminOnly:   true,
	})
}

// handleHelp 处理帮助命令
func (h *CommandHandler) handleHelp(c *Conn, args []string) error {
	page := 1
	if len(args) > 0 {
		if p, err := strconv.Atoi(args[0]); err == nil {
			page = p
		}
	}

	if page < 1 {
		return fmt.Errorf("'page' must be greater than 0")
	}

	commandsPerPage := 6
	h.mu.RLock()
	defer h.mu.RUnlock()

	var cmdList []*Command
	for _, cmd := range h.commands {
		if !cmd.AdminOnly || c.isAdmin() {
			cmdList = append(cmdList, cmd)
		}
	}

	totalPages := (len(cmdList) + commandsPerPage - 1) / commandsPerPage
	if page > totalPages || page < 1 {
		return fmt.Errorf("'page' must be between %d and %d", 1, totalPages)
	}

	startIdx := (page - 1) * commandsPerPage
	endIdx := min(startIdx+commandsPerPage, len(cmdList))

	var builder strings.Builder
	builder.WriteString(fmt.Sprintf("[orange]-- Commands Page %d/%d --\n\n", page, totalPages))

	for i := startIdx; i < endIdx; i++ {
		cmd := cmdList[i]
		builder.WriteString(fmt.Sprintf("[orange]/%s[white] %s - [lightgray]%s\n",
			cmd.Name, cmd.Usage, cmd.Description))
	}

	c.SendChat(builder.String())
	return nil
}

// handleTeamChat 处理队伍聊天
func (h *CommandHandler) handleTeamChat(c *Conn, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("message cannot be empty")
	}

	message := strings.Join(args, "")
	c.SendTeamChat(message)
	return nil
}

// handleAdminChat 处理管理员聊天
func (h *CommandHandler) handleAdminChat(c *Conn, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("message cannot be empty")
	}

	message := strings.Join(args, "")
	c.SendAdminChat(message)
	return nil
}

// handleSync 处理同步命令
func (h *CommandHandler) handleSync(c *Conn, args []string) error {
	// TODO: Implement world resynchronization
	c.SendChat("[accent]World resynchronization started...")
	return nil
}

// handleInfo 处理信息命令
func (h *CommandHandler) handleInfo(c *Conn, srv *Server) error {
	info := fmt.Sprintf("[orange]Server Info:\n[lightgray]Name: [white]%s\n[lightgray]Description: [white]%s\n[lightgray]Players: [white]%d\n[lightgray]Build: [white]%d",
		srv.Name, srv.Description, len(srv.conns), srv.BuildVersion)
	c.SendChat(info)
	return nil
}

// handleKick 处理踢人命令
func (h *CommandHandler) handleKick(c *Conn, srv *Server, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /kick <player> [reason]")
	}

	playerName := args[0]
	var reason string
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	target := srv.findPlayerByName(playerName)
	if target == nil {
		return fmt.Errorf("player '%s' not found", playerName)
	}

	if target == c {
		return fmt.Errorf("you cannot kick yourself")
	}

	kickReason := fmt.Sprintf("kicked by %s", c.playerName())
	if reason != "" {
		kickReason += fmt.Sprintf(": %s", reason)
	}

	srv.KickPlayer(target, kickReason)
	srv.BroadcastChat(fmt.Sprintf("[accent]%s [white]kicked %s", c.playerName(), target.playerName()))
	return nil
}

// handleBan 处理封禁命令
func (h *CommandHandler) handleBan(c *Conn, srv *Server, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /ban <player> [reason]")
	}

	playerName := args[0]
	var reason string
	if len(args) > 1 {
		reason = strings.Join(args[1:], " ")
	}

	target := srv.findPlayerByName(playerName)
	if target == nil {
		return fmt.Errorf("player '%s' not found", playerName)
	}

	banReason := "banned by admin"
	if reason != "" {
		banReason = fmt.Sprintf("banned: %s", reason)
	}

	// Ban by UUID
	if target.uuid != "" {
		srv.BanUUID(target.uuid, banReason)
	}

	// Ban by IP
	if target.remoteIP() != "" {
		srv.BanIP(target.remoteIP(), banReason)
	}

	srv.BroadcastChat(fmt.Sprintf("[accent]%s [white]banned %s", c.playerName(), target.playerName()))
	return nil
}

// handleUnban 处理解封命令
func (h *CommandHandler) handleUnban(c *Conn, srv *Server, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /unban <player>")
	}

	playerName := args[0]

	// Try to unban by UUID first
	if srv.UnbanUUID(playerName) {
		c.SendChat(fmt.Sprintf("[accent]Unbanned UUID: [white]%s", playerName))
		return nil
	}

	// Try to unban by IP
	if srv.UnbanIP(playerName) {
		c.SendChat(fmt.Sprintf("[accent]Unbanned IP: [white]%s", playerName))
		return nil
	}

	return fmt.Errorf("player '%s' not found in ban list", playerName)
}

// handleBans 处理列出封禁命令
func (h *CommandHandler) handleBans(c *Conn, srv *Server) error {
	uuids, ips := srv.ListBans()

	if len(uuids) == 0 && len(ips) == 0 {
		c.SendChat("[accent]No bans are currently active.")
		return nil
	}

	var builder strings.Builder
	builder.WriteString("[orange]Active Bans:\n")

	if len(uuids) > 0 {
		builder.WriteString("[lightgray]UUID Bans:\n")
		for uuid, reason := range uuids {
			builder.WriteString(fmt.Sprintf("  [white]%s: [lightgray]%s\n", uuid, reason))
		}
	}

	if len(ips) > 0 {
		builder.WriteString("[lightgray]IP Bans:\n")
		for ip, reason := range ips {
			builder.WriteString(fmt.Sprintf("  [white]%s: [lightgray]%s\n", ip, reason))
		}
	}

	c.SendChat(builder.String())
	return nil
}

// handleOps 处理列出管理员命令
func (h *CommandHandler) handleOps(c *Conn, srv *Server) error {
	ops := srv.ListOps()

	if len(ops) == 0 {
		c.SendChat("[accent]No operators are currently set.")
		return nil
	}

	var builder strings.Builder
	builder.WriteString("[orange]Operators:\n")
	for _, op := range ops {
		builder.WriteString(fmt.Sprintf("  [white]%s\n", op))
	}

	c.SendChat(builder.String())
	return nil
}

// handleOp 处理添加管理员命令
func (h *CommandHandler) handleOp(c *Conn, srv *Server, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /op <player>")
	}

	playerName := args[0]
	target := srv.findPlayerByName(playerName)
	if target == nil {
		return fmt.Errorf("player '%s' not found", playerName)
	}

	srv.AddOp(target.uuid)
	srv.BroadcastChat(fmt.Sprintf("[accent]%s [white]added %s as operator", c.playerName(), target.playerName()))
	return nil
}

// handleDeop 处理移除管理员命令
func (h *CommandHandler) handleDeop(c *Conn, srv *Server, args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: /deop <player>")
	}

	playerName := args[0]
	target := srv.findPlayerByName(playerName)
	if target == nil {
		return fmt.Errorf("player '%s' not found", playerName)
	}

	srv.RemoveOp(target.uuid)
	srv.BroadcastChat(fmt.Sprintf("[accent]%s [white]removed %s as operator", c.playerName(), target.playerName()))
	return nil
}

// findPlayerByName 根据名字查找玩家
func (srv *Server) findPlayerByName(name string) *Conn {
	srv.mu.Lock()
	defer srv.mu.Unlock()

	// 首先尝试精确匹配
	for conn := range srv.conns {
		if strings.EqualFold(conn.playerName(), name) {
			return conn
		}
	}

	// 然后尝试模糊匹配
	var candidates []*Conn
	for conn := range srv.conns {
		if strings.Contains(strings.ToLower(conn.playerName()), strings.ToLower(name)) {
			candidates = append(candidates, conn)
		}
	}

	if len(candidates) == 1 {
		return candidates[0]
	}

	return nil
}

// Helper functions implemented using AdminManager
func (c *Conn) SendTeamChat(message string) {
	if c == nil {
		return
	}

	teamColor := getTeamColor(c.teamID)
	formatted := fmt.Sprintf("[%s]<T> [coral]%s[coral]: [white]%s",
		teamColor, c.playerName(), message)

	if globalServer == nil {
		return
	}

	globalServer.mu.Lock()
	defer globalServer.mu.Unlock()

	for conn := range globalServer.conns {
		if conn == nil || !conn.hasConnected || conn.TeamID() != c.TeamID() {
			continue
		}
		_ = conn.SendChat(formatted)
	}
}

func (c *Conn) SendAdminChat(message string) {
	if c == nil {
		return
	}

	formatted := fmt.Sprintf("[#ff6b6b]<A> [coral]%s[coral]: [white]%s",
		c.playerName(), message)

	// 发送给所有管理员
	if globalServer != nil {
		globalServer.mu.Lock()
		defer globalServer.mu.Unlock()

		for conn := range globalServer.conns {
			if conn != nil && conn.isAdmin() {
				conn.SendChat(formatted)
			}
		}
	}
}

func (srv *Server) KickPlayer(c *Conn, reason string) {
	if c == nil || srv == nil {
		return
	}

	if srv.AdminManager != nil {
		// 设置临时封禁，防止立即重连
		srv.AdminManager.BanUUID(c.uuid, reason)
		srv.AdminManager.BanIP(c.remoteIP(), reason)

		// 踢出玩家
		c.SendAsync(&protocol.Remote_NetClient_kick_22{Reason: reason})

		// 延迟后清除封禁（用于临时踢人）
		go func() {
			time.Sleep(5 * time.Second)
			srv.AdminManager.UnbanUUID(c.uuid)
			srv.AdminManager.UnbanIP(c.remoteIP())
		}()
	}
}

func (srv *Server) BanUUID(uuid, reason string) int {
	if srv == nil {
		return 0
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return 0
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "banned by admin"
	}

	var targets []*Conn
	srv.mu.Lock()
	if srv.banUUID == nil {
		srv.banUUID = make(map[string]string)
	}
	srv.banUUID[uuid] = reason
	for c := range srv.conns {
		if c != nil && c.uuid == uuid {
			targets = append(targets, c)
		}
	}
	srv.mu.Unlock()

	if srv.AdminManager != nil {
		srv.AdminManager.BanUUID(uuid, reason)
	}

	for _, target := range targets {
		srv.noteRecentKick(target.uuid, target.remoteIP())
		_ = target.SendAsync(&protocol.Remote_NetClient_kick_22{Reason: reason})
		srv.closeConnLater(target, 250*time.Millisecond)
	}
	return len(targets)
}

func (srv *Server) BanIP(ip, reason string) int {
	if srv == nil {
		return 0
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return 0
	}
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "banned by admin"
	}

	var targets []*Conn
	srv.mu.Lock()
	if srv.banIP == nil {
		srv.banIP = make(map[string]string)
	}
	srv.banIP[ip] = reason
	for c := range srv.conns {
		if c != nil && c.remoteIP() == ip {
			targets = append(targets, c)
		}
	}
	srv.mu.Unlock()

	if srv.AdminManager != nil {
		srv.AdminManager.BanIP(ip, reason)
	}

	for _, target := range targets {
		srv.noteRecentKick(target.uuid, target.remoteIP())
		_ = target.SendAsync(&protocol.Remote_NetClient_kick_22{Reason: reason})
		srv.closeConnLater(target, 250*time.Millisecond)
	}
	return len(targets)
}

func (srv *Server) UnbanUUID(uuid string) bool {
	if srv == nil {
		return false
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return false
	}
	removed := false
	srv.mu.Lock()
	if _, ok := srv.banUUID[uuid]; ok {
		delete(srv.banUUID, uuid)
		removed = true
	}
	srv.mu.Unlock()
	if srv.AdminManager != nil && srv.AdminManager.UnbanUUID(uuid) {
		removed = true
	}
	return removed
}

func (srv *Server) UnbanIP(ip string) bool {
	if srv == nil {
		return false
	}
	ip = strings.TrimSpace(ip)
	if ip == "" {
		return false
	}
	removed := false
	srv.mu.Lock()
	if _, ok := srv.banIP[ip]; ok {
		delete(srv.banIP, ip)
		removed = true
	}
	srv.mu.Unlock()
	if srv.AdminManager != nil && srv.AdminManager.UnbanIP(ip) {
		removed = true
	}
	return removed
}

func (srv *Server) ListBans() (uuids, ips map[string]string) {
	uuids = map[string]string{}
	ips = map[string]string{}
	if srv == nil {
		return uuids, ips
	}

	srv.mu.Lock()
	for k, v := range srv.banUUID {
		uuids[k] = v
	}
	for k, v := range srv.banIP {
		ips[k] = v
	}
	srv.mu.Unlock()

	if srv.AdminManager != nil {
		adminUUIDs, adminIPs := srv.AdminManager.ListBans()
		for k, v := range adminUUIDs {
			uuids[k] = v
		}
		for k, v := range adminIPs {
			ips[k] = v
		}
	}
	return uuids, ips
}

func (srv *Server) BanLists() (uuids, ips map[string]string) {
	return srv.ListBans()
}

func (srv *Server) AddOp(uuid string) {
	if srv.AdminManager != nil {
		srv.AdminManager.AddOp(uuid)
	}
}

func (srv *Server) RemoveOp(uuid string) {
	if srv.AdminManager != nil {
		srv.AdminManager.RemoveOp(uuid)
	}
}

func (srv *Server) ListOps() []string {
	if srv.AdminManager != nil {
		return srv.AdminManager.ListOps()
	}
	return nil
}

func (srv *Server) IsOp(uuid string) bool {
	if srv == nil || srv.AdminManager == nil {
		return false
	}
	return srv.AdminManager.IsOp(strings.TrimSpace(uuid))
}

func (srv *Server) PlayerUnitIDSet() map[int32]struct{} {
	out := map[int32]struct{}{}
	if srv == nil {
		return out
	}
	srv.mu.Lock()
	for c := range srv.conns {
		if c != nil && c.unitID != 0 {
			out[c.unitID] = struct{}{}
		}
	}
	srv.mu.Unlock()
	return out
}

// getTeamColor 获取队伍颜色
func getTeamColor(teamID byte) string {
	switch teamID {
	case 0: // derelict
		return "7f7f7f"
	case 1: // sharded
		return "f4a460"
	case 2: // crux
		return "ff3f3f"
	case 3: // malis
		return "a9d8e0"
	case 4: // green
		return "6b8cff"
	case 5: // blue
		return "3a5ccc"
	case 6: // pink
		return "e05e6d"
	default:
		return "ffffff"
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
