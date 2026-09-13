package main

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	netserver "mdt-server/internal/net"
	"mdt-server/internal/plugin"
	"mdt-server/internal/world"
)

// pluginPlayerAdapter bridges net.Server players into the plugin PlayerAPI.
type pluginPlayerAdapter struct {
	srv *netserver.Server
}

func (a *pluginPlayerAdapter) PlayerList() []plugin.PlayerInfo {
	var out []plugin.PlayerInfo
	for _, snap := range a.srv.ListPlayerSnapshots() {
		if !snap.Connected {
			continue
		}
		out = append(out, plugin.PlayerInfo{
			ID:     snap.ID,
			Name:   snap.Name,
			UUID:   snap.UUID,
			IP:     snap.IP,
			TeamID: int(snap.TeamID),
			Admin:  false,
		})
	}
	return out
}

func (a *pluginPlayerAdapter) PlayerCount() int { return len(a.PlayerList()) }

func (a *pluginPlayerAdapter) PlayerFind(nameOrID string) *plugin.PlayerInfo {
	nameOrID = strings.TrimSpace(nameOrID)
	if nameOrID == "" {
		return nil
	}
	idValue, idErr := strconv.ParseInt(nameOrID, 10, 32)
	for _, info := range a.PlayerList() {
		if strings.EqualFold(info.Name, nameOrID) {
			found := info
			return &found
		}
		if idErr == nil && int64(info.ID) == idValue {
			found := info
			return &found
		}
	}
	return nil
}

func (a *pluginPlayerAdapter) PlayerSend(id int32, msg string) {
	for _, c := range a.srv.ListConnectedConns() {
		if c.ConnID() == id {
			_ = c.SendChat(msg)
			return
		}
	}
}

func (a *pluginPlayerAdapter) PlayerKick(id int32, reason string) {
	a.srv.KickByID(id, reason)
}

func (a *pluginPlayerAdapter) PlayerBan(id int32) {
	for _, c := range a.srv.ListConnectedConns() {
		if c.ConnID() == id {
			a.srv.BanUUID(c.UUID(), "banned by plugin")
			a.srv.KickByID(id, "banned by plugin")
			return
		}
	}
}

// pluginNetAdapter bridges chat broadcast into the plugin NetAPI.
type pluginNetAdapter struct {
	srv *netserver.Server
}

func (a *pluginNetAdapter) BroadcastChat(msg string) { a.srv.BroadcastChat(msg) }

// BroadcastChatFrom mirrors net.broadcast but tags the sender name so the
// message still reads naturally; sender exclusion is not enforced.
func (a *pluginNetAdapter) BroadcastChatFrom(msg string, senderID int32) {
	name := ""
	for _, c := range a.srv.ListConnectedConns() {
		if c.ConnID() == senderID {
			name = c.Name()
			break
		}
	}
	if name == "" {
		a.srv.BroadcastChat(msg)
		return
	}
	a.srv.BroadcastChat("[" + name + "] " + msg)
}

// pluginGameAdapter bridges world state into the plugin GameAPI.
type pluginGameAdapter struct {
	wld     *world.World
	gameTPS int
	state   *worldState
}

func (a *pluginGameAdapter) GameTick() uint64 { return a.wld.GameTick() }
func (a *pluginGameAdapter) GameTPS() int     { return a.gameTPS }
func (a *pluginGameAdapter) GameWave() int    { return a.wld.Wave() }
func (a *pluginGameAdapter) GameIsPlaying() bool {
	return a.state != nil && a.state.current != ""
}
func (a *pluginGameAdapter) GameIsPaused() bool { return false }
func (a *pluginGameAdapter) GameMapName() string {
	if a.state == nil {
		return ""
	}
	return a.state.current
}

// pluginServerLogger routes plugin log lines into the server console.
type pluginServerLogger struct{}

func (pluginServerLogger) Logf(format string, args ...any) {
	fmt.Printf("[插件] "+format+"\n", args...)
}

// firePluginPlayerEvent dispatches a player join/leave event into the plugin
// event bus. It is nil-safe and never blocks the connection path beyond the
// jsCallTimeout bound enforced by the JS runtime.
func firePluginPlayerEvent(reg *plugin.Registry, event string, c *netserver.Conn) {
	if reg == nil || c == nil {
		return
	}
	reg.FireEvent(context.Background(), event, plugin.EventPayload{
		"id":   c.PlayerID(),
		"name": c.Name(),
		"uuid": c.UUID(),
	})
}

// consolePluginRow is one row of the console "plugin list" table.
type consolePluginRow struct {
	FullID   string
	Name     string
	Runtime  string
	Version  string
	Category string
	Enabled  bool
	Source   string
}

// consolePluginList adapts the registry summary for console display.
func consolePluginList(reg *plugin.Registry) []consolePluginRow {
	if reg == nil {
		return nil
	}
	summaries := reg.LoadedSummaries()
	out := make([]consolePluginRow, 0, len(summaries))
	for _, s := range summaries {
		out = append(out, consolePluginRow{
			FullID:   s.FullID,
			Name:     s.Name,
			Runtime:  s.Runtime,
			Version:  s.Version,
			Category: s.Category,
			Enabled:  s.Enabled,
			Source:   s.Source,
		})
	}
	return out
}
