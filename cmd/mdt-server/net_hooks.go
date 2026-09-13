package main

import (
	"fmt"
	"strings"
	"time"

	"mdt-server/internal/config"
	netserver "mdt-server/internal/net"
	"mdt-server/internal/persist"
	"mdt-server/internal/protocol"
	"mdt-server/internal/storage"
	"mdt-server/internal/tracepoints"
	"mdt-server/internal/world"
)

// netHookDeps carries the main-local dependencies of the connection/event hooks.
type netHookDeps struct {
	state                  *worldState
	cache                  *worldCache
	detailLog              *detailedLogWriter
	recorder               storage.Recorder
	publicConnUUIDStore    *persist.PublicConnUUIDStore
	playerIdentityStore    *persist.PlayerIdentityStore
	shouldFileLogNetEvents func() bool
	currentTraceCfg        func() config.TracepointsConfig
	logTrace               func(category, point string, fields map[string]any)
	cfg                    *config.Config
}

// bindNetEventHooks wires connection lifecycle and event hooks.
func bindNetEventHooks(srv *netserver.Server, wld *world.World, d netHookDeps) {
	srv.OnConnectAccepted = func(conn *netserver.Conn, pkt *protocol.ConnectPacket) {
		if conn == nil {
			return
		}
		sourceName := strings.TrimSpace(conn.BaseName())
		if sourceName == "" && pkt != nil {
			sourceName = strings.TrimSpace(pkt.Name)
		}
		_, _ = ensureConnIdentityRecords(d.publicConnUUIDStore, d.playerIdentityStore, conn.UUID(), sourceName, connRemoteIP(conn))
	}

	srv.OnEvent = func(ev netserver.NetEvent) {
		if d.publicConnUUIDStore != nil && ev.Kind == "connect_packet" && runtimePublicConnUUIDEnabled.Load() {
			if current := publicConnUUIDValue(d.publicConnUUIDStore, ev.UUID); current == "" {
				name := strings.TrimSpace(netserver.StripMindustryColorTags(ev.Name))
				_, _ = ensureConnIdentityRecords(d.publicConnUUIDStore, d.playerIdentityStore, ev.UUID, name, ev.IP)
			}
		}
		ev.Detail = appendConnectionCheckpointDetail(ev.Detail, ev, d.publicConnUUIDStore, d.playerIdentityStore)
		_ = d.recorder.Record(storage.Event{
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
			ev.Timestamp.Format(time.RFC3339Nano), ev.Kind, ev.Packet, publicConnIDValue(d.publicConnUUIDStore, ev.UUID, ev.ConnID), ev.UUID, ev.IP, ev.Name, ev.Detail)
		if d.shouldFileLogNetEvents() {
			d.detailLog.LogLine(line)
		}
		tc := d.currentTraceCfg()
		if tc.Enabled {
			if tc.ClientRequestsEnabled && (ev.Kind == "packet_recv" || ev.Kind == "connect_packet" || strings.HasPrefix(ev.Kind, "connect_confirm") || strings.Contains(ev.Kind, "client_snapshot")) {
				d.logTrace("client_request", ev.Kind, map[string]any{
					"packet":  ev.Packet,
					"detail":  ev.Detail,
					"conn_id": ev.ConnID,
					"uuid":    ev.UUID,
					"ip":      ev.IP,
					"name":    ev.Name,
				})
			}
			if tc.ServerSendsEnabled && (ev.Kind == "packet_send" || ev.Kind == "world_handshake_sent" || strings.Contains(ev.Kind, "state_snapshot") || strings.Contains(ev.Kind, "entity_snapshot")) {
				d.logTrace("server_send", ev.Kind, map[string]any{
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

	srv.OnPostConnect = func(conn *netserver.Conn) {
		if conn == nil {
			return
		}
		mapPath := d.state.get()
		syncPostConnectWorldStateToConn(srv, conn, wld, d.cache.model(mapPath), mapPath, d.cfg.Sync.Strategy)
		showJoinPopupForConn(srv, conn)
		// Keep connect grace long enough for the official client to finish
		// applying the streamed world and rebinding its spawned unit.
		conn.SetWorldReloadGrace(2 * time.Second)
	}

	srv.OnMenuChoose = func(c *netserver.Conn, menuID, option int32) {
		handleJoinPopupMenuChoice(srv, c, menuID, option)
	}

	srv.OnHotReloadConnFn = func(conn *netserver.Conn) {
		if conn == nil {
			return
		}
		mapPath := d.state.get()
		syncPostConnectWorldStateToConn(srv, conn, wld, d.cache.model(mapPath), mapPath, d.cfg.Sync.Strategy)
		srv.RefreshPlayerDisplayNames()
		conn.SetWorldReloadGrace(2 * time.Second)
	}

	srv.OnTracePacket = func(direction string, c *netserver.Conn, obj any, packetID int, frameworkID int, size int) {
		tc := d.currentTraceCfg()
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
			d.logTrace("client_request", "packet_recv", tracepoints.PacketFields(direction, obj, packetID, frameworkID, size, extra))
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
			d.logTrace("server_send", "packet_send", tracepoints.PacketFields(direction, obj, packetID, frameworkID, size, extra))
		}
	}
}
