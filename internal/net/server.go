package net

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mdt-server/internal/devlog"
	"mdt-server/internal/protocol"
	"mdt-server/internal/storage"
)

type NetEvent struct {
	Timestamp time.Time
	Kind      string
	Packet    string
	Detail    string
	ConnID    int32
	UUID      string
	IP        string
	Name      string
}

type Server struct {
	Addr     string
	Registry *protocol.PacketRegistry
	Serial   *Serializer
	Content  *protocol.ContentRegistry
	TypeIO   *protocol.TypeIOContext

	mu       sync.Mutex
	conns    map[*Conn]struct{}
	pending  map[int32]*Conn
	byUDP    map[string]*Conn
	banUUID  map[string]string
	banIP    map[string]string
	udpConn  *net.UDPConn
	opMu     sync.RWMutex
	ops      map[string]struct{}
	entityMu sync.Mutex
	entities map[int32]protocol.UnitSyncEntity
	unitNext int32

	// EventManager 事件管理器
	EventManager *storage.EventManager

	BuildVersion int
	WorldDataFn  func(*Conn, *protocol.ConnectPacket) ([]byte, error)
	OnEvent      func(NetEvent)
	playerIDNext int32

	Name           string
	Description    string
	VirtualPlayers int32
	MapNameFn      func() string
	OnChat         func(*Conn, string) bool
	SpawnTileFn    func() (protocol.Point2, bool)
	// Optional provider for player-respawn unit type id (e.g. alpha).
	PlayerUnitTypeFn func() int16
	// Optional hook: apply client build plans from snapshot.
	OnBuildPlans func(*Conn, []*protocol.BuildPlan)
	// Respawn delay in "frames" (Mindustry Time.delta units). Defaults to 60 if zero.
	RespawnDelayFrames float32
	// Optional hook: spawn player unit in world/state. Uses provided unitID, returns world coords.
	SpawnUnitFn func(c *Conn, unitID int32, tile protocol.Point2, unitType int16) (float32, float32, bool)
	// Optional hook: remove a unit from world/state.
	DropUnitFn func(unitID int32)
	// Optional hook: query unit info from world/state.
	UnitInfoFn func(unitID int32) (UnitInfo, bool)
	// Optional hook: called once after initial connect/spawn sequence starts.
	OnPostConnect func(*Conn)

	StateSnapshotFn func() *protocol.Remote_NetClient_stateSnapshot_35
	// Appends additional entities into entity snapshot stream.
	// Return value is appended entity count.
	ExtraEntitySnapshotFn func(w *protocol.Writer) (int16, error)

	UdpRetryCount  int
	UdpRetryDelay  time.Duration
	UdpFallbackTCP bool

	entitySnapshotIntervalNs atomic.Int64
	stateSnapshotIntervalNs  atomic.Int64
	infoMu                   sync.RWMutex

	// DevLogger 开发者日志（可选）
	DevLogger *devlog.DevLogger
}

func NewServer(addr string, buildVersion int) *Server {
	reg := protocol.NewRegistry(buildVersion)
	content := protocol.NewContentRegistry()
	ctx := content.Context()
	em := storage.NewEventManager()
	s := &Server{
		Addr:               addr,
		Registry:           reg,
		Serial:             &Serializer{Registry: reg, Ctx: ctx},
		Content:            content,
		TypeIO:             ctx,
		conns:              map[*Conn]struct{}{},
		pending:            map[int32]*Conn{},
		byUDP:              map[string]*Conn{},
		banUUID:            map[string]string{},
		banIP:              map[string]string{},
		EventManager:       em,
		BuildVersion:       buildVersion,
		WorldDataFn:        defaultWorldData,
		playerIDNext:       0,
		Name:               "mdt-server",
		Description:        "",
		VirtualPlayers:     0,
		entities:           map[int32]protocol.UnitSyncEntity{},
		unitNext:           2000000000,
		ops:                map[string]struct{}{},
		UdpRetryCount:      2,
		UdpRetryDelay:      5 * time.Millisecond,
		UdpFallbackTCP:     true,
		RespawnDelayFrames: 60,
	}
	s.SetSnapshotIntervals(100, 250)
	return s
}

func normalizeSyncInterval(ms int, def int) time.Duration {
	if ms <= 0 {
		ms = def
	}
	if ms < 20 {
		ms = 20
	}
	if ms > 5000 {
		ms = 5000
	}
	return time.Duration(ms) * time.Millisecond
}

func (s *Server) SetSnapshotIntervals(entityMs, stateMs int) {
	entity := normalizeSyncInterval(entityMs, 100)
	state := normalizeSyncInterval(stateMs, 250)
	s.entitySnapshotIntervalNs.Store(int64(entity))
	s.stateSnapshotIntervalNs.Store(int64(state))
}

func (s *Server) SnapshotIntervalsMs() (entityMs int, stateMs int) {
	entity := time.Duration(s.entitySnapshotIntervalNs.Load())
	state := time.Duration(s.stateSnapshotIntervalNs.Load())
	if entity <= 0 {
		entity = 100 * time.Millisecond
	}
	if state <= 0 {
		state = 250 * time.Millisecond
	}
	return int(entity / time.Millisecond), int(state / time.Millisecond)
}

// KillSelfUnit clears current controlled unit for a player connection.
func (s *Server) KillSelfUnit(c *Conn) bool {
	if c == nil || c.playerID == 0 {
		return false
	}
	s.markDead(c, "kill-self")
	player := &protocol.EntityBox{IDValue: c.playerID}
	_ = c.SendAsync(&protocol.Remote_InputHandler_unitClear_91{Player: player})
	return true
}

func (s *Server) Serve() error {
	addr, err := net.ResolveTCPAddr("tcp", s.Addr)
	if err != nil {
		return err
	}
	ln, err := net.ListenTCP("tcp", addr)
	if err != nil {
		return err
	}

	udpAddr := &net.UDPAddr{IP: addr.IP, Port: addr.Port}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	defer udpConn.Close()
	s.udpConn = udpConn
	go s.serveUDP(udpConn)

	for {
		c, err := ln.AcceptTCP()
		if err != nil {
			return err
		}
		conn := NewConn(c, s.Serial)
		conn.id = s.nextID()
		conn.onSend = func(obj any, packetID int, frameworkID int, size int) {
			detail := fmt.Sprintf("size=%d", size)
			if packetID >= 0 {
				detail = fmt.Sprintf("%s packet_id=%d", detail, packetID)
			}
			if frameworkID >= 0 {
				detail = fmt.Sprintf("%s framework_id=%d", detail, frameworkID)
			}
			s.emitEvent(conn, "packet_send", fmt.Sprintf("%T", obj), detail)
		}
		s.addConn(conn)
		s.addPending(conn)

		// 记录详细连接日志
		if s.DevLogger != nil {
			s.DevLogger.LogConnection("tcp accepted", conn.id, c.RemoteAddr().String(), "unknown", "")
		} else {
			fmt.Printf("[net] tcp accepted remote=%s id=%d\n", c.RemoteAddr().String(), conn.id)
		}

		_ = conn.Send(&protocol.RegisterTCP{ConnectionID: conn.id})
		s.emitEvent(conn, "tcp_accept", "", "")
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(c *Conn) {
	defer func() {
		if rec := recover(); rec != nil {
			errText := fmt.Sprintf("panic: %v", rec)
			fmt.Printf("[net] conn panic id=%d remote=%s err=%s\n", c.id, c.RemoteAddr().String(), errText)
			s.emitEvent(c, "conn_panic", "", errText)
		}
		if c.hasConnected && c.playerID != 0 {
			s.broadcastPlayerDisconnect(c.playerID, c)
		}
		c.Close()
		s.removeConn(c)
	}()

	for {
		obj, err := c.ReadObject()
		if err != nil {
			if !c.hasConnected && c.hasBegunConnecting && strings.Contains(err.Error(), "item stack length") {
				fmt.Printf("[net] compat ignore early packet id=%d packet_id=%d err=%v\n", c.id, c.lastRecvPacketID, err)
				s.emitEvent(c, "compat_ignore_early_packet", "", err.Error())
				continue
			}
			if errors.Is(err, io.EOF) || isConnReadClosed(err) {
				fmt.Printf("[net] tcp closed id=%d remote=%s\n", c.id, c.RemoteAddr().String())
				s.emitEvent(c, "tcp_closed", "", err.Error())
				return
			}
			// Skip malformed packet frame and keep the connection alive.
			fmt.Printf("[net] read object failed id=%d remote=%s packet_id=%d framework_id=%d err=%v\n", c.id, c.RemoteAddr().String(), c.lastRecvPacketID, c.lastRecvFrameworkID, err)
			s.emitEvent(c, "read_error", "", fmt.Sprintf("packet_id=%d framework_id=%d err=%v", c.lastRecvPacketID, c.lastRecvFrameworkID, err))
			continue
		}
		s.handlePacket(c, obj, true)
	}
}

func isConnReadClosed(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "wsarecv") &&
		(strings.Contains(msg, "aborted") || strings.Contains(msg, "forcibly closed")) {
		return true
	}
	if strings.Contains(msg, "connection reset by peer") {
		return true
	}
	return false
}

func (s *Server) handlePacket(c *Conn, obj any, fromTCP bool) {
	detail := ""
	if fromTCP {
		if c.lastRecvPacketID >= 0 {
			detail = fmt.Sprintf("packet_id=%d", c.lastRecvPacketID)
		} else if c.lastRecvFrameworkID >= 0 {
			detail = fmt.Sprintf("framework_id=%d", c.lastRecvFrameworkID)
		}
	}
	s.emitEvent(c, "packet_recv", fmt.Sprintf("%T", obj), detail)

	switch v := obj.(type) {
	case *CompatIgnoredPacket:
		// Custom client compatibility: some client->server packets are not mapped.
		// For custom clients, parse known build packet IDs and avoid aggressive respawn fallback.
		switch v.ID {
		case 9, 13, 123: // BeginBreakCallPacket (compat ids)
			if s.OnBuildPlans != nil {
				if plan, ok := decodeCompatBeginBreak(v.Payload, s.TypeIO); ok {
					s.OnBuildPlans(c, []*protocol.BuildPlan{plan})
				}
			}
		case 10, 14: // BeginPlaceCallPacket (compat ids)
			if s.OnBuildPlans != nil {
				if plan, ok := decodeCompatBeginPlace(v.Payload, s.TypeIO); ok {
					s.OnBuildPlans(c, []*protocol.BuildPlan{plan})
				}
			}
		case 124, 134: // BeginPlace/TileTapCallPacket / UnitControlCallPacket
			if v.ID == 134 {
				// UnitControl in compat mode should not force respawn directly.
				// Respawn is handled by explicit UnitClear(133). Here we only handle control switch cleanup.
				action := byte(255)
				if len(v.Payload) >= 1 {
					action = v.Payload[0]
				}
				if action == 0 {
					// Only drop when already dead; avoid clearing active units due to noisy clients.
					if c.dead && c.unitID != 0 {
						s.dropPlayerUnitEntity(c, c.unitID)
						c.unitID = 0
					}
				}
				return
			}
			// Some clients use compat id=124 for beginPlace; try full beginPlace decode first.
			if v.ID == 124 && s.OnBuildPlans != nil {
				if plan, ok := decodeCompatBeginPlace(v.Payload, s.TypeIO); ok {
					s.OnBuildPlans(c, []*protocol.BuildPlan{plan})
					return
				}
			}
			// Fallback build handling when client sends tile taps while in build mode.
			if v.ID == 124 && s.OnBuildPlans != nil && c.building && len(v.Payload) >= 4 {
				packed := int32(binary.BigEndian.Uint32(v.Payload[:4]))
				p := protocol.UnpackPoint2(packed)
				breaking := c.selectedBlockID <= 0
				plan := &protocol.BuildPlan{
					Breaking: breaking,
					X:        p.X,
					Y:        p.Y,
					Rotation: byte(c.selectedRotation),
				}
				if !breaking {
					plan.Block = protocol.BlockRef{BlkID: c.selectedBlockID, BlkName: ""}
				}
				s.OnBuildPlans(c, []*protocol.BuildPlan{plan})
			}
		}
		return
	case *CompatUnitClearPacket:
		// Treat explicit UnitClear as a player respawn request (V/Q flow).
		// Rate-limited to avoid spam loops from noisy clients.
		// Ignore if player is already alive with a unit; some clients echo unitClear after spawn.
		if c != nil && !c.dead && c.unitID != 0 {
			return
		}
		s.requestRespawn(c, "compat-unitClear")
		return
	case *protocol.ConnectPacket:
		// 记录详细连接包日志
		if s.DevLogger != nil {
			s.DevLogger.LogConnection("connect packet", c.id, c.remoteIP(), v.Name, v.UUID,
				devlog.Int32Fld("version", v.Version),
				devlog.StringFld("version_type", v.VersionType),
				devlog.IntFld("mods", len(v.Mods)))
		} else {
			fmt.Printf("[net] connect packet id=%d name=%q version=%d type=%q mods=%d\n",
				c.id, v.Name, v.Version, v.VersionType, len(v.Mods))
		}

		if c.hasBegunConnecting {
			_ = c.Send(&protocol.Remote_NetClient_kick_21{Reason: protocol.KickReasonIDInUse})
			if s.DevLogger != nil {
				s.DevLogger.LogConnection("duplicate connect", c.id, c.remoteIP(), v.Name, v.UUID,
					devlog.StringFld("reason", "idInUse"))
			} else {
				fmt.Printf("[net] duplicate connect id=%d -> kick=idInUse\n", c.id)
			}
			return
		}
		if err := ValidateConnect(v, s.BuildVersion); err != nil {
			switch {
			case errors.Is(err, ErrClientOutdated):
				_ = c.Send(&protocol.Remote_NetClient_kick_21{Reason: protocol.KickReasonClientOutdated})
				if s.DevLogger != nil {
					s.DevLogger.LogConnection("connect rejected", c.id, c.remoteIP(), v.Name, v.UUID,
						devlog.StringFld("reason", "clientOutdated"),
						devlog.StringFld("error", err.Error()))
				} else {
					fmt.Printf("[net] connect rejected id=%d reason=clientOutdated err=%v\n", c.id, err)
				}
			case errors.Is(err, ErrServerOutdated):
				_ = c.Send(&protocol.Remote_NetClient_kick_21{Reason: protocol.KickReasonServerOutdated})
				if s.DevLogger != nil {
					s.DevLogger.LogConnection("connect rejected", c.id, c.remoteIP(), v.Name, v.UUID,
						devlog.StringFld("reason", "serverOutdated"),
						devlog.StringFld("error", err.Error()))
				} else {
					fmt.Printf("[net] connect rejected id=%d reason=serverOutdated err=%v\n", c.id, err)
				}
			default:
				_ = c.Send(&protocol.Remote_NetClient_kick_21{Reason: protocol.KickReasonKick})
				if s.DevLogger != nil {
					s.DevLogger.LogConnection("connect rejected", c.id, c.remoteIP(), v.Name, v.UUID,
						devlog.StringFld("reason", "kick"),
						devlog.StringFld("error", err.Error()))
				} else {
					fmt.Printf("[net] connect rejected id=%d reason=kick err=%v\n", c.id, err)
				}
			}
			return
		}
		c.name = v.Name
		c.uuid = v.UUID
		c.versionType = strings.TrimSpace(v.VersionType)
		c.color = v.Color
		// Use official call IDs for official clients; keep compat IDs only for
		// non-official/custom clients.
		vt := strings.ToLower(strings.TrimSpace(c.versionType))
		isOfficial := vt == "" || vt == "official" || strings.Contains(vt, "official")
		c.recvCompatIDs = !isOfficial
		c.sendCompatIDs = !isOfficial
		if c.playerID == 0 {
			c.playerID = s.nextPlayerID()
		}
		s.ensurePlayerEntity(c)
		s.emitEvent(c, "connect_packet", fmt.Sprintf("%T", v), fmt.Sprintf("version=%d type=%s mods=%d", v.Version, v.VersionType, len(v.Mods)))
		ip := c.remoteIP()
		if ip != "" {
			s.mu.Lock()
			ipReason, ipBanned := s.banIP[ip]
			uuidReason, uuidBanned := s.banUUID[v.UUID]
			s.mu.Unlock()
			if ipBanned || uuidBanned {
				reason := "banned"
				if uuidBanned && uuidReason != "" {
					reason = uuidReason
				} else if ipBanned && ipReason != "" {
					reason = ipReason
				}
				_ = c.Send(&protocol.Remote_NetClient_kick_22{Reason: reason})
				fmt.Printf("[net] connect rejected id=%d reason=banned ip=%s uuid=%s\n", c.id, ip, v.UUID)
				return
			}
		}
		c.hasBegunConnecting = true
		if err := s.sendWorldHandshake(c, v); err != nil {
			_ = c.Send(&protocol.Remote_NetClient_kick_22{Reason: "world data unavailable"})
			fmt.Printf("[net] world handshake failed id=%d err=%v\n", c.id, err)
			return
		}
		fmt.Printf("[net] world handshake sent id=%d\n", c.id)
		s.emitEvent(c, "world_handshake_sent", "", "")
	case *protocol.Remote_NetServer_connectConfirm_47:
		c.hasConnected = true
		fmt.Printf("[net] connect confirm id=%d\n", c.id)
		s.emitEvent(c, "connect_confirm", fmt.Sprintf("%T", v), "")
		s.ensurePostConnectStarted(c)
	case *protocol.Remote_NetServer_clientSnapshot_45:
		if !c.hasConnected {
			c.hasConnected = true
			fmt.Printf("[net] connect confirm via clientSnapshot id=%d\n", c.id)
			s.emitEvent(c, "connect_confirm_client_snapshot", fmt.Sprintf("%T", v), "")
			s.ensurePostConnectStarted(c)
		}
		c.snapX = v.X
		c.snapY = v.Y
		c.pointerX = v.PointerX
		c.pointerY = v.PointerY
		c.shooting = v.Shooting
		c.boosting = v.Boosting
		c.typing = v.Chatting
		prevUnitID := c.unitID
		if v.Dead {
			// Only honor client-dead when server agrees the unit is missing or dead.
			shouldDead := c.unitID == 0
			if !shouldDead {
				if s.UnitInfoFn != nil {
					info, ok := s.UnitInfoFn(c.unitID)
					if !ok || info.Health <= 0 {
						shouldDead = true
					}
				} else {
					s.entityMu.Lock()
					_, ok := s.entities[c.unitID]
					s.entityMu.Unlock()
					if !ok {
						shouldDead = true
					}
				}
			}
			if shouldDead {
				s.markDead(c, "client-dead")
			}
		}
		if v.UnitID != 0 {
			// Do not trust arbitrary client-reported unit IDs.
			// Only adopt when it is already current, or when server entity ownership matches this player.
			acceptUnitID := v.UnitID == c.unitID
			if !acceptUnitID {
				s.entityMu.Lock()
				ent, exists := s.entities[v.UnitID]
				s.entityMu.Unlock()
				if exists {
					if u, ok := ent.(*protocol.UnitEntitySync); ok {
						if ctrl, ok := u.Controller.(*protocol.ControllerState); ok && ctrl != nil &&
							ctrl.Type == protocol.ControllerPlayer && ctrl.PlayerID == c.playerID {
							acceptUnitID = true
						}
					}
				}
			}
			if acceptUnitID {
				c.unitID = v.UnitID
			}
		} else if c.dead {
			c.unitID = 0
		}
		if prevUnitID != 0 && prevUnitID != c.unitID {
			s.dropPlayerUnitEntity(c, prevUnitID)
		}
		c.building = v.Building
		c.selectedRotation = v.SelectedRotation
		c.selectedBlockID = -1
		if v.SelectedBlock != nil {
			c.selectedBlockID = v.SelectedBlock.ID()
		}
		if s.OnBuildPlans != nil {
			if plans := extractBuildPlans(v.Plans); len(plans) > 0 {
				s.OnBuildPlans(c, plans)
			}
		}
	case *protocol.Remote_NetClient_ping_18:
		// Temporary compatibility: some 155 clients misdecode pingResponse call IDs.
		// Keep connection alive via framework keepalive loop only.
		_ = v
	case *protocol.Remote_Units_unitSpawn_48:
		// Handle unit spawn from server (unit from WorldStream)
		// This is called when a unit is spawned on the server and needs to be synced to clients
		if container := v.Container; container != nil {
			s.addEntity(container.Unit())
		}
		if s.DevLogger != nil {
			unit := v.Container.Unit()
			if unit != nil {
				s.DevLogger.LogUnit(unit.ID(), fmt.Sprintf("%T", unit), "unitSpawn",
					devlog.Int32Fld("unit_id", unit.ID()))
			}
		}
	case *protocol.Remote_Units_unitCapDeath_49:
		// Handle unit capacity death
		if unit := v.Unit; unit != nil {
			unitID := unit.ID()
			s.entityMu.Lock()
			if ent := s.entities[unitID]; ent != nil {
				delete(s.entities, unitID)
			}
			s.entityMu.Unlock()
			fmt.Printf("[net] unitCapDeath id=%d\n", unitID)
		}
	case *protocol.Remote_Units_unitEnvDeath_50:
		// Handle environmental unit death
		if unit := v.Unit; unit != nil {
			unitID := unit.ID()
			s.entityMu.Lock()
			if ent := s.entities[unitID]; ent != nil {
				delete(s.entities, unitID)
			}
			s.entityMu.Unlock()
			fmt.Printf("[net] unitEnvDeath id=%d\n", unitID)
		}
	case *protocol.Remote_Units_unitDeath_51:
		// Handle unit death (delayed)
		unitID := v.Uid
		s.entityMu.Lock()
		if ent := s.entities[unitID]; ent != nil {
			delete(s.entities, unitID)
		}
		s.entityMu.Unlock()
		fmt.Printf("[net] unitDeath id=%d\n", unitID)
	case *protocol.Remote_Units_unitDestroy_52:
		// Handle immediate unit destruction
		unitID := v.Uid
		s.entityMu.Lock()
		if ent := s.entities[unitID]; ent != nil {
			delete(s.entities, unitID)
		}
		s.entityMu.Unlock()
		fmt.Printf("[net] unitDestroy id=%d\n", unitID)
	case *protocol.Remote_Units_unitDespawn_53:
		// Handle unit despawn (removes unit but doesn't destroy)
		if unit := v.Unit; unit != nil {
			unitID := unit.ID()
			s.entityMu.Lock()
			if ent := s.entities[unitID]; ent != nil {
				delete(s.entities, unitID)
			}
			s.entityMu.Unlock()
			fmt.Printf("[net] unitDespawn id=%d\n", unitID)
		}
	case *protocol.Remote_Units_unitSafeDeath_54:
		// Handle unit safe death (non damaging death)
		if unit := v.Unit; unit != nil {
			unitID := unit.ID()
			s.entityMu.Lock()
			if ent := s.entities[unitID]; ent != nil {
				delete(s.entities, unitID)
			}
			s.entityMu.Unlock()
			fmt.Printf("[net] unitSafeDeath id=%d\n", unitID)
		}
	case *protocol.Remote_BulletType_createBullet_55:
		// Create bullet entity
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 55, "Remote_BulletType_createBullet_55", "create_bullet")
		}
	case *protocol.Remote_Teams_destroyPayload_56:
		// Destroy payload on team entities
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 56, "Remote_Teams_destroyPayload_56", "destroy_payload")
		}
	case *protocol.Remote_InputHandler_transferItemEffect_57:
		// 物品传输效果
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 57, "Remote_InputHandler_transferItemEffect_57", "transfer_item_effect")
		}
	case *protocol.Remote_InputHandler_takeItems_58:
		// 抽取物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 58, "Remote_InputHandler_takeItems_58", "take_items")
		}
	case *protocol.Remote_InputHandler_transferItemToUnit_59:
		// 传输物品到单位
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 59, "Remote_InputHandler_transferItemToUnit_59", "transfer_item_to_unit")
		}
	case *protocol.Remote_InputHandler_setItem_60:
		// 设置单个物品槽位
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 60, "Remote_InputHandler_setItem_60", "set_item")
		}
	case *protocol.Remote_InputHandler_setItems_61:
		// 批量设置物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 61, "Remote_InputHandler_setItems_61", "set_items")
		}
	case *protocol.Remote_InputHandler_setTileItems_62:
		// 设置地砖物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 62, "Remote_InputHandler_setTileItems_62", "set_tile_items")
		}
	case *protocol.Remote_InputHandler_clearItems_63:
		// 清空物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 63, "Remote_InputHandler_clearItems_63", "clear_items")
		}
	case *protocol.Remote_InputHandler_setLiquid_64:
		// 设置单个液体槽位
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 64, "Remote_InputHandler_setLiquid_64", "set_liquid")
		}
	case *protocol.Remote_InputHandler_setLiquids_65:
		// 批量设置液体
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 65, "Remote_InputHandler_setLiquids_65", "set_liquids")
		}
	case *protocol.Remote_InputHandler_setTileLiquids_66:
		// 设置地砖液体
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 66, "Remote_InputHandler_setTileLiquids_66", "set_tile_liquids")
		}
	case *protocol.Remote_InputHandler_clearLiquids_67:
		// 清空液体
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 67, "Remote_InputHandler_clearLiquids_67", "clear_liquids")
		}
	case *protocol.Remote_InputHandler_transferItemTo_68:
		// 通用物品传输到
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 68, "Remote_InputHandler_transferItemTo_68", "transfer_item_to")
		}
	case *protocol.Remote_InputHandler_deletePlans_69:
		// 删除建造计划
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 69, "Remote_InputHandler_deletePlans_69", "delete_plans")
		}
	case *protocol.Remote_InputHandler_commandUnits_70:
		// 指挥多个单位
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 70, "Remote_InputHandler_commandUnits_70", fmt.Sprintf("command_units(count=%d)", len(v.UnitIds)))
		}
	case *protocol.Remote_InputHandler_setUnitCommand_71:
		// 设置单位命令
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 71, "Remote_InputHandler_setUnitCommand_71", fmt.Sprintf("set_unit_command(count=%d)", len(v.UnitIds)))
		}
	case *protocol.Remote_InputHandler_setUnitStance_72:
		// 设置单位姿态
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 72, "Remote_InputHandler_setUnitStance_72", fmt.Sprintf("set_unit_stance(count=%d)", len(v.UnitIds)))
		}
	case *protocol.Remote_InputHandler_commandBuilding_73:
		// 命令建筑
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 73, "Remote_InputHandler_commandBuilding_73", fmt.Sprintf("command_building(count=%d)", len(v.Buildings)))
		}
	case *protocol.Remote_InputHandler_requestItem_74:
		// 请求物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 74, "Remote_InputHandler_requestItem_74", "request_item")
		}
	case *protocol.Remote_InputHandler_transferInventory_75:
		// 转移库存
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 75, "Remote_InputHandler_transferInventory_75", "transfer_inventory")
		}
	case *protocol.Remote_InputHandler_removeQueueBlock_76:
		// 从队列中移除方块
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 76, "Remote_InputHandler_removeQueueBlock_76", fmt.Sprintf("remove_queue(x=%d y=%d breaking=%v)", v.X, v.Y, v.Breaking))
		}
	case *protocol.Remote_InputHandler_requestUnitPayload_77:
		// 请求单位载荷
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 77, "Remote_InputHandler_requestUnitPayload_77", "request_unit_payload")
		}
	case *protocol.Remote_InputHandler_requestBuildPayload_78:
		// 请求建筑载荷
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 78, "Remote_InputHandler_requestBuildPayload_78", "request_build_payload")
		}
	case *protocol.Remote_InputHandler_pickedUnitPayload_79:
		// 单位运载选择
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 79, "Remote_InputHandler_pickedUnitPayload_79", "picked_unit_payload")
		}
	case *protocol.Remote_InputHandler_pickedBuildPayload_80:
		// 建筑运载选择
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 80, "Remote_InputHandler_pickedBuildPayload_80", "picked_build_payload")
		}
	case *protocol.Remote_InputHandler_requestDropPayload_81:
		// 请求丢弃运载物
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 81, "Remote_InputHandler_requestDropPayload_81", fmt.Sprintf("request_drop_payload(x=%.1f y=%.1f)", v.X, v.Y))
		}
	case *protocol.Remote_InputHandler_payloadDropped_82:
		// 运载物已丢弃
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 82, "Remote_InputHandler_payloadDropped_82", fmt.Sprintf("payload_dropped(x=%.1f y=%.1f)", v.X, v.Y))
		}
	case *protocol.Remote_InputHandler_unitEnteredPayload_83:
		// 单位进入运载
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 83, "Remote_InputHandler_unitEnteredPayload_83", "unit_entered_payload")
		}
	case *protocol.Remote_InputHandler_dropItem_84:
		// 投放物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 84, "Remote_InputHandler_dropItem_84", "drop_item")
		}
	case *protocol.Remote_NetServer_requestDebugStatus_36:
		resp := &protocol.Remote_NetServer_debugStatusClient_37{
			Value:              0,
			LastClientSnapshot: 0,
			SnapshotsSent:      0,
		}
		_ = c.Send(resp)
	case *protocol.Remote_Build_beginBreak_123:
		//客户端请求开始破坏建筑
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 123, "Remote_Build_beginBreak_123", fmt.Sprintf("x=%d y=%d", v.X, v.Y))
		}
		if s.OnBuildPlans != nil {
			s.OnBuildPlans(c, []*protocol.BuildPlan{{
				Breaking: true,
				X:        v.X,
				Y:        v.Y,
				Rotation: 0,
			}})
		}
	case *protocol.Remote_Build_beginPlace_124:
		// 客户端请求开始放置建筑
		blockID := int16(0)
		if v.Result != nil {
			blockID = v.Result.ID()
		}
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 124, "Remote_Build_beginPlace_124", fmt.Sprintf("x=%d y=%d block=%d", v.X, v.Y, blockID))
		}
		if s.OnBuildPlans != nil && blockID > 0 {
			s.OnBuildPlans(c, []*protocol.BuildPlan{{
				Breaking: false,
				X:        v.X,
				Y:        v.Y,
				Rotation: byte(v.Rotation) & 0x03,
				Block:    protocol.BlockRef{BlkID: blockID, BlkName: ""},
				Config:   v.PlaceConfig,
			}})
		}
	case *protocol.Remote_Tile_setFloor_128:
		// 设置地板
		if s.DevLogger != nil {
			floorID := int16(0)
			if v.Floor != nil {
				floorID = v.Floor.ID()
			}
			pos := v.Tile.Pos()
			x := int32(protocol.UnpackPoint2X(pos))
			y := int32(protocol.UnpackPoint2Y(pos))
			s.DevLogger.LogBuild(x, y, 0, "none", "setFloor",
				devlog.Int32Fld("tile_x", x),
				devlog.Int32Fld("tile_y", y),
				devlog.Int16Fld("floor_id", floorID))
		}
	case *protocol.Remote_Tile_setOverlay_129:
		// 设置覆盖层
		if s.DevLogger != nil {
			overlayID := int16(0)
			if v.Overlay != nil {
				overlayID = v.Overlay.ID()
			}
			pos := v.Tile.Pos()
			x := int32(protocol.UnpackPoint2X(pos))
			y := int32(protocol.UnpackPoint2Y(pos))
			s.DevLogger.LogBuild(x, y, 0, "none", "setOverlay",
				devlog.Int32Fld("tile_x", x),
				devlog.Int32Fld("tile_y", y),
				devlog.Int16Fld("overlay_id", overlayID))
		}
	case *protocol.Remote_Tile_removeTile_130:
		// 移除 Tile
		if s.DevLogger != nil {
			pos := v.Tile.Pos()
			x := int32(protocol.UnpackPoint2X(pos))
			y := int32(protocol.UnpackPoint2Y(pos))
			s.DevLogger.LogBuild(x, y, 0, "none", "removeTile",
				devlog.Int32Fld("tile_x", x),
				devlog.Int32Fld("tile_y", y))
		}
	case *protocol.Remote_Tile_setTile_131:
		// 设置 Tile（_block）
		if s.DevLogger != nil {
			blockID := int32(0)
			if v.Block != nil {
				blockID = int32(v.Block.ID())
			}
			team := "none"
			if v.Team.ID >= 0 && v.Team.ID < 16 {
				teamNames := []string{"none", "cyan", "purple", "pink", "sharded", "blue", "green", "crux", "moray", "brown", "olive", "teal", "cosmic", "yellow", "black", "white"}
				if int(v.Team.ID) < len(teamNames) {
					team = teamNames[v.Team.ID]
				}
			}
			pos := v.Tile.Pos()
			x := int32(protocol.UnpackPoint2X(pos))
			y := int32(protocol.UnpackPoint2Y(pos))
			s.DevLogger.LogBuild(x, y, int16(blockID), team, "setTile",
				devlog.Int32Fld("tile_pos", pos),
				devlog.Int32Fld("block_id", blockID),
				devlog.Int32Fld("rotation", v.Rotation))
		}
	case *protocol.Remote_Tile_buildDestroyed_134:
		// 建筑销毁
		if s.DevLogger != nil {
			pos := int32(0)
			if v.Build != nil {
				pos = v.Build.Pos()
			}
			x := int32(0)
			y := int32(0)
			if pos != 0 {
				pt := protocol.UnpackPoint2(pos)
				x = pt.X
				y = pt.Y
			}
			s.DevLogger.LogBuild(x, y, 0, "none", "buildDestroyed",
				devlog.Int32Fld("tile_pos", pos))
		}
	case *protocol.Remote_Tile_buildHealthUpdate_135:
		// 建筑生命值更新 - 简化处理（数据包结构需要进一步分析）
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 135, "Remote_Tile_buildHealthUpdate_135", "building_health_update")
		}
	case *protocol.Remote_InputHandler_unitControl_90:
		// 单位控制（需要进一步分析数据包结构）
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 90, "Remote_InputHandler_unitControl_90", "unit_control")
		}
		if c != nil && c.playerID != 0 {
			if unitID := extractUnitID(v.Unit); unitID != 0 {
				s.unitControl(c, unitID)
			}
		}
	case *protocol.Remote_InputHandler_unitClear_91:
		// 单位清除（玩家死亡/重生请求）
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 91, "Remote_InputHandler_unitClear_91", "unit_clear")
		}
		// Treat unitClear as a player respawn request
		s.requestRespawn(c, "unitClear-91")
	case protocol.FrameworkMessage:
		switch m := v.(type) {
		case *protocol.Ping:
			if !m.IsReply {
				m.IsReply = true
				_ = c.Send(m)
			}
		default:
			// ignore keepalive
		}
	case *protocol.Remote_Logic_sectorCapture_1:
		// 逻辑 - 领域捕获事件
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 1, "Remote_Logic_sectorCapture_1", "sector_capture")
		}
	case *protocol.Remote_Logic_updateGameOver_2:
		// 逻辑 - 游戏结束状态更新
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 2, "Remote_Logic_updateGameOver_2", "update_gameover")
		}
	case *protocol.Remote_Logic_gameOver_3:
		// 逻辑 - 游戏结束事件
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 3, "Remote_Logic_gameOver_3", "game_over")
		}
	case *protocol.Remote_Logic_researched_4:
		// 逻辑 - 科研解锁事件
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 4, "Remote_Logic_researched_4", "researched")
		}
	case *protocol.Remote_LExecutor_setMapArea_92:
		// 逻辑执行器 - 设置地图区域
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 92, "Remote_LExecutor_setMapArea_92", "set_map_area")
		}
	case *protocol.Remote_LExecutor_logicExplosion_93:
		// 逻辑执行器 - 逻辑爆炸效果
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 93, "Remote_LExecutor_logicExplosion_93", "logic_explosion")
		}
	case *protocol.Remote_LExecutor_syncVariable_94:
		// 逻辑执行器 - 同步变量值
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 94, "Remote_LExecutor_syncVariable_94", "sync_variable")
		}
	case *protocol.Remote_LExecutor_setFlag_95:
		// 逻辑执行器 - 设置逻辑标志
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 95, "Remote_LExecutor_setFlag_95", "set_flag")
		}
	case *protocol.Remote_LExecutor_createMarker_96:
		// 逻辑执行器 - 创建标记
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 96, "Remote_LExecutor_createMarker_96", "create_marker")
		}
	case *protocol.Remote_LExecutor_removeMarker_97:
		// 逻辑执行器 - 删除标记
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 97, "Remote_LExecutor_removeMarker_97", "remove_marker")
		}
	case *protocol.Remote_LExecutor_updateMarker_98:
		// 逻辑执行器 - 更新标记
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 98, "Remote_LExecutor_updateMarker_98", "update_marker")
		}
	case *protocol.Remote_LExecutor_updateMarkerText_99:
		// 逻辑执行器 - 更新标记文本
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 99, "Remote_LExecutor_updateMarkerText_99", "update_marker_text")
		}
	case *protocol.Remote_LExecutor_updateMarkerTexture_100:
		// 逻辑执行器 - 更新标记纹理
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 100, "Remote_LExecutor_updateMarkerTexture_100", "update_marker_texture")
		}
	case *protocol.Remote_Weather_createWeather_101:
		// 天气 - 创建天气效果
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 101, "Remote_Weather_createWeather_101", "create_weather")
		}
	case *protocol.Remote_Menus_menu_102:
		// 菜单 - 显示菜单
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 102, "Remote_Menus_menu_102", "menu")
		}
	case *protocol.Remote_Menus_followUpMenu_103:
		// 菜单 - 跟随菜单
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 103, "Remote_Menus_followUpMenu_103", "follow_up_menu")
		}
	case *protocol.Remote_Menus_hideFollowUpMenu_104:
		// 菜单 - 隐藏跟随菜单
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 104, "Remote_Menus_hideFollowUpMenu_104", "hide_follow_up_menu")
		}
	case *protocol.Remote_Menus_menuChoose_105:
		// 菜单 - 菜单选择
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 105, "Remote_Menus_menuChoose_105", "menu_choose")
		}
	case *protocol.Remote_Menus_textInput_106:
		// 菜单 - 文本输入1
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 106, "Remote_Menus_textInput_106", "text_input")
		}
	case *protocol.Remote_Menus_textInput_107:
		// 菜单 - 文本输入2
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 107, "Remote_Menus_textInput_107", "text_input_2")
		}
	case *protocol.Remote_Menus_textInputResult_108:
		// 菜单 - 文本输入结果
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 108, "Remote_Menus_textInputResult_108", "text_input_result")
		}
	case *protocol.Remote_Menus_setHudText_109:
		// 菜单 - 设置HUD文本
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 109, "Remote_Menus_setHudText_109", "set_hud_text")
		}
	case *protocol.Remote_Menus_hideHudText_110:
		// 菜单 - 隐藏HUD文本
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 110, "Remote_Menus_hideHudText_110", "hide_hud_text")
		}
	case *protocol.Remote_Menus_setHudTextReliable_111:
		// 菜单 - 设置可靠HUD文本
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 111, "Remote_Menus_setHudTextReliable_111", "set_hud_text_reliable")
		}
	case *protocol.Remote_Menus_announce_112:
		// 菜单 - 公告
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 112, "Remote_Menus_announce_112", "announce")
		}
	case *protocol.Remote_Menus_infoMessage_113:
		// 菜单 - 信息消息
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 113, "Remote_Menus_infoMessage_113", "info_message")
		}
	case *protocol.Remote_Menus_infoPopup_114:
		// 菜单 - 信息弹窗
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 114, "Remote_Menus_infoPopup_114", "info_popup")
		}
	case *protocol.Remote_Menus_label_115:
		// 菜单 - 标签
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 115, "Remote_Menus_label_115", "label")
		}
	case *protocol.Remote_Menus_infoPopupReliable_116:
		// 菜单 - 可靠性信息弹窗
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 116, "Remote_Menus_infoPopupReliable_116", "info_popup_reliable")
		}
	case *protocol.Remote_Menus_labelReliable_117:
		// 菜单 - 可靠性标签
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 117, "Remote_Menus_labelReliable_117", "label_reliable")
		}
	case *protocol.Remote_Menus_infoToast_118:
		// 菜单 - 信息Toast
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 118, "Remote_Menus_infoToast_118", "info_toast")
		}
	case *protocol.Remote_Menus_warningToast_119:
		// 菜单 - 警告Toast
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 119, "Remote_Menus_warningToast_119", "warning_toast")
		}
	case *protocol.Remote_Menus_openURI_120:
		// 菜单 - 打开URI
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 120, "Remote_Menus_openURI_120", "open_uri")
		}
	case *protocol.Remote_Menus_removeWorldLabel_121:
		// 菜单 - 移除世界标签
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 121, "Remote_Menus_removeWorldLabel_121", "remove_world_label")
		}
	case *protocol.Remote_HudFragment_setPlayerTeamEditor_122:
		// HUD - 设置玩家队伍编辑器
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 122, "Remote_HudFragment_setPlayerTeamEditor_122", "set_player_team_editor")
		}
	case *protocol.Remote_Tile_setTileBlocks_125:
		// Tile - 设置 Tile 的 Blocks
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 125, "Remote_Tile_setTileBlocks_125", "set_tile_blocks")
		}
	case *protocol.Remote_Tile_setTileFloors_126:
		// Tile - 设置 Tile 的 Floors
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 126, "Remote_Tile_setTileFloors_126", "set_tile_floors")
		}
	case *protocol.Remote_Tile_setTileOverlays_127:
		// Tile - 设置 Tile 的 Overlays
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 127, "Remote_Tile_setTileOverlays_127", "set_tile_overlays")
		}
	case *protocol.Remote_Tile_setTeam_132:
		// Tile - 设置单个建筑的队伍
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 132, "Remote_Tile_setTeam_132", "set_team")
		}
	case *protocol.Remote_Tile_setTeams_133:
		// Tile - 批量设置建筑队伍
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 133, "Remote_Tile_setTeams_133", "set_teams")
		}
	case *protocol.Remote_ConstructBlock_deconstructFinish_136:
		// 构造方块 - 解构完成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 136, "Remote_ConstructBlock_deconstructFinish_136", "deconstruct_finish")
		}
	case *protocol.Remote_ConstructBlock_constructFinish_137:
		// 构造方块 - 构造完成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 137, "Remote_ConstructBlock_constructFinish_137", "construct_finish")
		}
	case *protocol.Remote_LandingPad_landingPadLanded_138:
		// 着陆平台 - 着陆
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 138, "Remote_LandingPad_landingPadLanded_138", "landing_pad_landed")
		}
	case *protocol.Remote_AutoDoor_autoDoorToggle_139:
		// 自动门 - 切换
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 139, "Remote_AutoDoor_autoDoorToggle_139", "auto_door_toggle")
		}
	case *protocol.Remote_CoreBlock_playerSpawn_140:
		// 核心区块 - 玩家生成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 140, "Remote_CoreBlock_playerSpawn_140", "player_spawn")
		}
	case *protocol.Remote_UnitAssembler_assemblerUnitSpawned_141:
		// 单位组装器 - 单位生成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 141, "Remote_UnitAssembler_assemblerUnitSpawned_141", "assembler_unit_spawned")
		}
	case *protocol.Remote_UnitAssembler_assemblerDroneSpawned_142:
		// 单位组装器 - 无人机生成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 142, "Remote_UnitAssembler_assemblerDroneSpawned_142", "assembler_drone_spawned")
		}
	case *protocol.Remote_UnitBlock_unitBlockSpawn_143:
		// 单位方块 - 单位方块生成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 143, "Remote_UnitBlock_unitBlockSpawn_143", "unit_block_spawn")
		}
	case *protocol.Remote_UnitCargoLoader_unitTetherBlockSpawned_144:
		// 单位货物加载器 - 无人机生成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 144, "Remote_UnitCargoLoader_unitTetherBlockSpawned_144", "unit_tether_block_spawned")
		}
	default:
		// ignore
	}
}

func extractString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case *string:
		if t == nil {
			return ""
		}
		return *t
	default:
		return ""
	}
}

func extractBuildPlans(v any) []*protocol.BuildPlan {
	switch t := v.(type) {
	case []*protocol.BuildPlan:
		return t
	case []any:
		out := make([]*protocol.BuildPlan, 0, len(t))
		for _, it := range t {
			if p, ok := it.(*protocol.BuildPlan); ok && p != nil {
				out = append(out, p)
			}
		}
		return out
	default:
		return nil
	}
}

func decodeCompatBeginBreak(payload []byte, ctx *protocol.TypeIOContext) (*protocol.BuildPlan, bool) {
	r := protocol.NewReaderWithContext(payload, ctx)
	// beginBreak starts with unit object in call payload.
	if _, err := protocol.ReadObject(r, false, ctx); err != nil {
		return nil, false
	}
	if _, err := protocol.ReadTeam(r, ctx); err != nil {
		return nil, false
	}
	x, err := r.ReadInt32()
	if err != nil {
		return nil, false
	}
	y, err := r.ReadInt32()
	if err != nil {
		return nil, false
	}
	return &protocol.BuildPlan{
		Breaking: true,
		X:        x,
		Y:        y,
	}, true
}

func decodeCompatBeginPlace(payload []byte, ctx *protocol.TypeIOContext) (*protocol.BuildPlan, bool) {
	r := protocol.NewReaderWithContext(payload, ctx)
	// beginPlace starts with unit object in call payload.
	if _, err := protocol.ReadObject(r, false, ctx); err != nil {
		return nil, false
	}
	block, err := protocol.ReadBlock(r, ctx)
	if err != nil || block == nil {
		return nil, false
	}
	if _, err := protocol.ReadTeam(r, ctx); err != nil {
		return nil, false
	}
	x, err := r.ReadInt32()
	if err != nil {
		return nil, false
	}
	y, err := r.ReadInt32()
	if err != nil {
		return nil, false
	}
	rot, err := r.ReadInt32()
	if err != nil {
		return nil, false
	}
	// placeConfig (ignored)
	if _, err := protocol.ReadObject(r, false, ctx); err != nil {
		return nil, false
	}
	return &protocol.BuildPlan{
		Breaking: false,
		X:        x,
		Y:        y,
		Rotation: byte(rot),
		Block:    protocol.BlockRef{BlkID: block.ID(), BlkName: ""},
	}, true
}

func sanitizeChatMessage(s string) string {
	s = strings.ReplaceAll(s, "\n", "")
	s = strings.ReplaceAll(s, "\r", "")
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	// Keep parity with Mindustry text length constraint.
	runes := []rune(s)
	if len(runes) > 150 {
		s = string(runes[:150])
	}
	return s
}

func formatChatName(name string) string {
	if strings.TrimSpace(name) == "" {
		return "[lightgray][server][white]: "
	}
	return name + "[white]: "
}

func (s *Server) broadcastSimpleMessage(message string) {
	s.mu.Lock()
	peers := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		peers = append(peers, c)
	}
	s.mu.Unlock()

	for _, peer := range peers {
		_ = peer.SendAsync(&protocol.Remote_NetClient_sendMessage_15{Message: message})
	}
}

func (s *Server) sendSystemMessage(c *Conn, message string) {
	if c == nil {
		return
	}
	_ = c.SendAsync(&protocol.Remote_NetClient_sendMessage_15{Message: message})
}

func (s *Server) SendChat(c *Conn, message string) {
	s.sendSystemMessage(c, message)
}

func (s *Server) BroadcastChat(message string) {
	message = sanitizeChatMessage(message)
	if message == "" {
		return
	}
	formatted := formatChatName("") + message
	s.broadcastSimpleMessage(formatted)
}

func (s *Server) Broadcast(obj any) {
	if obj == nil {
		return
	}
	s.mu.Lock()
	peers := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		peers = append(peers, c)
	}
	s.mu.Unlock()
	for _, peer := range peers {
		_ = peer.SendAsync(obj)
	}
}

func (s *Server) SendStatusTo(c *Conn) {
	if c == nil {
		return
	}
	msg := fmt.Sprintf("[accent]status[]: sessions=%d", len(s.ListSessions()))
	s.sendSystemMessage(c, msg)
}

func (s *Server) emitEvent(c *Conn, kind, packet, detail string) {
	if s.OnEvent == nil {
		return
	}
	ev := NetEvent{
		Timestamp: time.Now().UTC(),
		Kind:      kind,
		Packet:    packet,
		Detail:    detail,
	}
	if c != nil {
		ev.ConnID = c.id
		ev.UUID = c.uuid
		ev.IP = c.remoteIP()
		ev.Name = c.name
	}
	s.OnEvent(ev)

	// 也分发到事件管理器
	if s.EventManager != nil {
		// 将 NetEvent 转换 storage.Event
		stgEv := storage.Event{
			Timestamp: ev.Timestamp,
			Kind:      ev.Kind,
			Packet:    ev.Packet,
			Detail:    ev.Detail,
			ConnID:    ev.ConnID,
			UUID:      ev.UUID,
			IP:        ev.IP,
			Name:      ev.Name,
		}
		// todo: 映射 kind 到 Trigger
		s.EventManager.Dispatch(stgEv)
	}
}

func (s *Server) addConn(c *Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[c] = struct{}{}
}

func (s *Server) removeConn(c *Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, c)
	delete(s.pending, c.id)
	if c.udpAddr != nil {
		delete(s.byUDP, c.udpAddr.String())
	}
	if c.playerID != 0 {
		s.entityMu.Lock()
		delete(s.entities, c.playerID)
		if c.unitID != 0 {
			delete(s.entities, c.unitID)
		}
		s.entityMu.Unlock()
	}
}

type Conn struct {
	net.Conn
	serial              *Serializer
	mu                  sync.Mutex
	id                  int32
	playerID            int32
	recvCompatIDs       bool
	sendCompatIDs       bool
	udpMu               sync.RWMutex
	udpAddr             *net.UDPAddr
	hasBegunConnecting  bool
	hasConnected        bool
	name                string
	uuid                string
	versionType         string
	color               int32
	snapX               float32
	snapY               float32
	pointerX            float32
	pointerY            float32
	shooting            bool
	boosting            bool
	typing              bool
	dead                bool
	deathTimer          float32
	lastRespawnCheck    time.Time
	lastSpawnAt         time.Time
	unitID              int32
	lastRespawnReq      time.Time
	building            bool
	selectedBlockID     int16
	selectedRotation    int32
	lastRecvPacketID    int
	lastRecvFrameworkID int
	closed              chan struct{}
	closeOnce           sync.Once
	postOnce            sync.Once
	onSend              func(obj any, packetID int, frameworkID int, size int)
	streamMu            sync.Mutex
	streams             map[int32]*StreamBuilder
	outHigh             chan any
	outNorm             chan any
	outClosed           chan struct{}
	sendCount           atomic.Int64
	sendErrors          atomic.Int64
	sendQueued          atomic.Int64
	sendQueueFull       atomic.Int64
	bytesSent           atomic.Int64
	udpSent             atomic.Int64
	udpErrors           atomic.Int64
	statsMu             sync.Mutex
	byTypeSent          map[string]int64
	byTypeBytes         map[string]int64
}

type UnitInfo struct {
	ID        int32
	X         float32
	Y         float32
	Health    float32
	MaxHealth float32
	TeamID    byte
	TypeID    int16
}

func NewConn(c net.Conn, serial *Serializer) *Conn {
	conn := &Conn{
		Conn:                c,
		serial:              serial,
		closed:              make(chan struct{}),
		lastRecvPacketID:    -1,
		lastRecvFrameworkID: -1,
		streams:             make(map[int32]*StreamBuilder),
		outHigh:             make(chan any, 128),
		outNorm:             make(chan any, 256),
		outClosed:           make(chan struct{}),
		byTypeSent:          make(map[string]int64),
		byTypeBytes:         make(map[string]int64),
	}
	go conn.sendLoop()
	return conn
}

func (c *Conn) ReadObject() (any, error) {
	for {
		lenbuf := make([]byte, 2)
		if _, err := io.ReadFull(c.Conn, lenbuf); err != nil {
			return nil, err
		}
		n := binary.BigEndian.Uint16(lenbuf)
		payload := make([]byte, n)
		if _, err := io.ReadFull(c.Conn, payload); err != nil {
			return nil, err
		}
		c.lastRecvPacketID = -1
		c.lastRecvFrameworkID = -1
		if len(payload) > 0 {
			if payload[0] == 0xFE {
				if len(payload) > 1 {
					c.lastRecvFrameworkID = int(payload[1])
				}
			} else {
				c.lastRecvPacketID = int(payload[0])
			}
		}
		r := bytesReader(payload)
		obj, err := c.serial.ReadObjectMode(r, c.recvCompatIDs)
		if err != nil {
			// Early auto-fallback: probe opposite decode mode for mixed-ID clients.
			if !c.hasConnected {
				if altObj, altErr := c.serial.ReadObjectMode(bytesReader(payload), !c.recvCompatIDs); altErr == nil {
					c.recvCompatIDs = !c.recvCompatIDs
					fmt.Printf("[net] rx mode switch id=%d recvCompat=%v packet_id=%d\n", c.id, c.recvCompatIDs, c.lastRecvPacketID)
					obj = altObj
				} else {
					return nil, err
				}
			} else {
				return nil, err
			}
		}
		if ignored, ok := obj.(*CompatIgnoredPacket); ok && !c.hasConnected {
			if altObj, altErr := c.serial.ReadObjectMode(bytesReader(payload), !c.recvCompatIDs); altErr == nil {
				c.recvCompatIDs = !c.recvCompatIDs
				fmt.Printf("[net] rx mode switch(id=ignored) id=%d recvCompat=%v packet_id=%d ignored=%d\n", c.id, c.recvCompatIDs, c.lastRecvPacketID, ignored.ID)
				obj = altObj
			}
		}
		switch v := obj.(type) {
		case *protocol.StreamBegin:
			c.streamMu.Lock()
			c.streams[v.ID] = NewStreamBuilder(v, c.serial.Registry)
			c.streamMu.Unlock()
			continue
		case *protocol.StreamChunk:
			c.streamMu.Lock()
			sb := c.streams[v.ID]
			if sb != nil {
				sb.Add(v)
				if sb.Done() {
					delete(c.streams, v.ID)
					c.streamMu.Unlock()
					packet, err := sb.Build()
					if err != nil {
						return nil, err
					}
					return packet, nil
				}
			}
			c.streamMu.Unlock()
			continue
		default:
			return obj, nil
		}
	}
}

func (c *Conn) Send(obj any) error {
	return c.sendNow(obj)
}

func (c *Conn) sendNow(obj any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	buf := newBuffer()
	if err := c.serial.WriteObjectCompat(buf, obj, c.sendCompatIDs); err != nil {
		fmt.Printf("[net] sendNow encode failed id=%d err=%v obj=%T\n", c.id, err, obj)
		return err
	}
	payload := buf.Bytes()
	packetID := -1
	frameworkID := -1
	if len(payload) >= 1 {
		if payload[0] == 0xFE {
			if len(payload) >= 2 {
				frameworkID = int(payload[1])
			}
		} else {
			packetID = int(payload[0])
		}
	}
	// Keep per-packet logs off by default to avoid log flood.
	if packetID >= 0 && c.sendCount.Load() < 40 {
		fmt.Printf("[net] tx id=%d packet_id=%d type=%T len=%d sendCompat=%v recvCompat=%v\n", c.id, packetID, obj, len(payload), c.sendCompatIDs, c.recvCompatIDs)
	}
	if len(payload) > 0xFFFF {
		return fmt.Errorf("payload too large: %d", len(payload))
	}
	lenbuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenbuf, uint16(len(payload)))
	if _, err := c.Conn.Write(lenbuf); err != nil {
		return err
	}
	if _, err := c.Conn.Write(payload); err != nil {
		c.sendErrors.Add(1)
		return err
	}
	if c.onSend != nil {
		c.onSend(obj, packetID, frameworkID, len(payload))
	}
	c.sendCount.Add(1)
	c.bytesSent.Add(int64(len(payload)))
	c.recordSend(obj, int64(len(payload)))
	return nil
}

func (c *Conn) Encode(obj any) ([]byte, int, int, error) {
	buf := newBuffer()
	if err := c.serial.WriteObjectCompat(buf, obj, c.sendCompatIDs); err != nil {
		return nil, -1, -1, err
	}
	payload := buf.Bytes()
	packetID := -1
	frameworkID := -1
	if len(payload) >= 1 {
		if payload[0] == 0xFE {
			if len(payload) >= 2 {
				frameworkID = int(payload[1])
			}
		} else {
			packetID = int(payload[0])
		}
	}
	return payload, packetID, frameworkID, nil
}

func (c *Conn) setUDPAddr(addr *net.UDPAddr) {
	c.udpMu.Lock()
	c.udpAddr = addr
	c.udpMu.Unlock()
}

func (c *Conn) UDPAddr() *net.UDPAddr {
	c.udpMu.RLock()
	defer c.udpMu.RUnlock()
	return c.udpAddr
}

func bytesReader(b []byte) *bytes.Reader {
	return bytes.NewReader(b)
}

func newBuffer() *bytes.Buffer {
	return &bytes.Buffer{}
}

func (c *Conn) Close() error {
	var err error
	c.closeOnce.Do(func() {
		close(c.closed)
		close(c.outClosed)
		_ = c.Conn.SetDeadline(time.Now())
		err = c.Conn.Close()
	})
	return err
}

func (c *Conn) PlayerID() int32 {
	return c.playerID
}

func (c *Conn) UUID() string {
	return c.uuid
}

func (c *Conn) VersionType() string {
	return c.versionType
}

func (c *Conn) SnapshotPos() (float32, float32) {
	return c.snapX, c.snapY
}

func (c *Conn) SendStream(typeID byte, payload []byte) error {
	begin := &protocol.StreamBegin{
		ID:    rand.Int31(),
		Total: int32(len(payload)),
		Type:  typeID,
	}
	if begin.ID == 0 {
		begin.ID = 1
	}
	if err := c.Send(begin); err != nil {
		return err
	}
	for len(payload) > 0 {
		chunkLen := 1100
		if len(payload) < chunkLen {
			chunkLen = len(payload)
		}
		chunk := &protocol.StreamChunk{
			ID:   begin.ID,
			Data: append([]byte(nil), payload[:chunkLen]...),
		}
		if err := c.Send(chunk); err != nil {
			return err
		}
		payload = payload[chunkLen:]
	}
	return nil
}

func (c *Conn) SendAsync(obj any) error {
	return c.SendAsyncPriority(obj, priorityOf(obj))
}

func (c *Conn) SendAsyncPriority(obj any, prio int) error {
	select {
	case <-c.outClosed:
		return errors.New("connection closed")
	default:
	}
	switch prio {
	case protocol.PriorityHigh:
		select {
		case c.outHigh <- obj:
			c.sendQueued.Add(1)
			return nil
		default:
			c.sendQueueFull.Add(1)
			return c.sendNow(obj)
		}
	default:
		select {
		case c.outNorm <- obj:
			c.sendQueued.Add(1)
			return nil
		default:
			c.sendQueueFull.Add(1)
			return c.sendNow(obj)
		}
	}
}

func (c *Conn) sendLoop() {
	for {
		select {
		case obj := <-c.outHigh:
			if err := c.sendNow(obj); err != nil {
				_ = c.Close()
				return
			}
			continue
		default:
		}

		select {
		case obj := <-c.outHigh:
			if err := c.sendNow(obj); err != nil {
				_ = c.Close()
				return
			}
		case obj := <-c.outNorm:
			if err := c.sendNow(obj); err != nil {
				_ = c.Close()
				return
			}
		case <-c.outClosed:
			return
		}
	}
}

func priorityOf(obj any) int {
	if p, ok := obj.(protocol.Packet); ok {
		return p.Priority()
	}
	return protocol.PriorityNormal
}

func (c *Conn) remoteIP() string {
	if c.Conn == nil || c.Conn.RemoteAddr() == nil {
		return ""
	}
	host, _, err := net.SplitHostPort(c.Conn.RemoteAddr().String())
	if err != nil {
		return c.Conn.RemoteAddr().String()
	}
	return host
}

func (s *Server) addPending(c *Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[c.id] = c
}

func (s *Server) nextID() int32 {
	for {
		id := rand.Int31()
		if id == 0 {
			continue
		}
		if _, ok := s.pending[id]; ok {
			continue
		}
		return id
	}
}

func (s *Server) nextPlayerID() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		s.playerIDNext++
		if s.playerIDNext == 0 {
			continue
		}
		return s.playerIDNext
	}
}

func (s *Server) serveUDP(conn *net.UDPConn) {
	buf := make([]byte, 65535)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		b := buf[:n]
		obj, err := s.Serial.ReadObject(bytesReader(b))
		if err != nil {
			if ru, ok := parseRegisterUDPRaw(b); ok {
				obj = ru
			} else {
				fmt.Printf("[net] udp read failed remote=%s len=%d err=%v\n", addr.String(), n, err)
				continue
			}
		}
		switch v := obj.(type) {
		case *protocol.RegisterUDP:
			var tc *Conn
			s.mu.Lock()
			c := s.pending[v.ConnectionID]
			if c != nil {
				delete(s.pending, c.id)
			} else {
				// Client may retry UDP registration if ACK is lost.
				// In that case connection has already moved out of pending.
				for live := range s.conns {
					if live.id == v.ConnectionID {
						c = live
						break
					}
				}
			}
			if c != nil {
				if old := c.UDPAddr(); old != nil {
					delete(s.byUDP, old.String())
				}
				c.setUDPAddr(addr)
				s.byUDP[addr.String()] = c
				tc = c
				if err := s.sendUDPRegisterAck(addr, v.ConnectionID); err != nil {
					fmt.Printf("[net] udp register ack failed remote=%s id=%d err=%v\n", addr.String(), c.id, err)
				}
				fmt.Printf("[net] udp registered remote=%s id=%d\n", addr.String(), c.id)
			}
			s.mu.Unlock()
			// ArcNet clients treat this TCP framework message as UDP registration completion.
			if tc != nil {
				_ = tc.SendAsync(&protocol.RegisterUDP{ConnectionID: v.ConnectionID})
			}
		case *protocol.DiscoverHost:
			payload := s.buildServerData()
			if len(payload) > 0 {
				_, _ = conn.WriteToUDP(payload, addr)
			}
		default:
			s.mu.Lock()
			c := s.byUDP[addr.String()]
			s.mu.Unlock()
			if c != nil {
				s.handlePacket(c, v, false)
			}
		}
	}
}

func parseRegisterUDPRaw(b []byte) (*protocol.RegisterUDP, bool) {
	if len(b) >= 6 && b[0] == 0xFE && b[1] == protocol.FrameworkRegisterUD {
		id := int32(binary.BigEndian.Uint32(b[2:6]))
		return &protocol.RegisterUDP{ConnectionID: id}, true
	}
	if len(b) >= 8 {
		// Some clients may prefix 2-byte length before framework message.
		if b[2] == 0xFE && b[3] == protocol.FrameworkRegisterUD {
			id := int32(binary.BigEndian.Uint32(b[4:8]))
			return &protocol.RegisterUDP{ConnectionID: id}, true
		}
	}
	return nil, false
}

func (s *Server) sendUDP(addr *net.UDPAddr, obj any) error {
	if s.udpConn == nil || addr == nil {
		return errors.New("udp not ready")
	}
	buf := newBuffer()
	if err := s.Serial.WriteObject(buf, obj); err != nil {
		return err
	}
	_, err := s.udpConn.WriteToUDP(buf.Bytes(), addr)
	return err
}

func (s *Server) sendUDPRegisterAck(addr *net.UDPAddr, id int32) error {
	if s.udpConn == nil || addr == nil {
		return errors.New("udp not ready")
	}
	buf := make([]byte, 6)
	buf[0] = 0xFE
	buf[1] = protocol.FrameworkRegisterUD
	binary.BigEndian.PutUint32(buf[2:], uint32(id))
	_, err := s.udpConn.WriteToUDP(buf, addr)
	return err
}

func (s *Server) buildServerData() []byte {
	s.infoMu.RLock()
	name := s.Name
	description := s.Description
	virtualPlayers := s.VirtualPlayers
	s.infoMu.RUnlock()
	if name == "" {
		name = "mdt-server"
	}
	mapName := "unknown"
	if s.MapNameFn != nil {
		if m := s.MapNameFn(); m != "" {
			mapName = m
		}
	}
	players := len(s.ListSessions()) + int(virtualPlayers)
	if players < 0 {
		players = 0
	}
	wave := int32(1)
	version := int32(s.BuildVersion)
	versionType := "official"
	mode := byte(0)
	playerLimit := int32(0)
	modeName := ""
	port := int16(portFromAddr(s.Addr))

	buf := &bytes.Buffer{}
	writeByteString(buf, name, 100)
	writeByteString(buf, mapName, 64)
	_ = binary.Write(buf, binary.BigEndian, int32(players))
	_ = binary.Write(buf, binary.BigEndian, wave)
	_ = binary.Write(buf, binary.BigEndian, version)
	writeByteString(buf, versionType, 32)
	_ = buf.WriteByte(mode)
	_ = binary.Write(buf, binary.BigEndian, playerLimit)
	writeByteString(buf, description, 100)
	writeByteString(buf, modeName, 50)
	_ = binary.Write(buf, binary.BigEndian, port)
	return buf.Bytes()
}

func (s *Server) SetServerName(name string) {
	s.infoMu.Lock()
	s.Name = name
	s.infoMu.Unlock()
}

func (s *Server) SetServerDescription(desc string) {
	s.infoMu.Lock()
	s.Description = desc
	s.infoMu.Unlock()
}

func (s *Server) SetVirtualPlayers(n int32) {
	s.infoMu.Lock()
	s.VirtualPlayers = n
	s.infoMu.Unlock()
}

func (s *Server) ServerMeta() (name string, desc string, virtual int32) {
	s.infoMu.RLock()
	defer s.infoMu.RUnlock()
	return s.Name, s.Description, s.VirtualPlayers
}

func writeByteString(buf *bytes.Buffer, s string, maxLen int) {
	b := []byte(s)
	if len(b) > maxLen {
		b = b[:maxLen]
	}
	_ = buf.WriteByte(byte(len(b)))
	_, _ = buf.Write(b)
}

func portFromAddr(addr string) int {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return 0
	}
	v, err := strconv.Atoi(port)
	if err != nil {
		return 0
	}
	return v
}

func (s *Server) ensurePostConnectStarted(c *Conn) {
	if c == nil {
		return
	}
	c.postOnce.Do(func() {
		s.spawnPlayerInitial(c)
		if s.OnPostConnect != nil {
			go s.OnPostConnect(c)
		}
		go s.postConnectLoop(c)
	})
}

func (s *Server) postConnectLoop(c *Conn) {
	entityInterval := time.Duration(s.entitySnapshotIntervalNs.Load())
	if entityInterval <= 0 {
		entityInterval = 100 * time.Millisecond
	}
	stateInterval := time.Duration(s.stateSnapshotIntervalNs.Load())
	if stateInterval <= 0 {
		stateInterval = 250 * time.Millisecond
	}
	entityTicker := time.NewTicker(entityInterval)
	defer entityTicker.Stop()
	stateTicker := time.NewTicker(stateInterval)
	defer stateTicker.Stop()
	keepAliveTicker := time.NewTicker(3 * time.Second)
	defer keepAliveTicker.Stop()
	for {
		select {
		case <-stateTicker.C:
			s.maybeRespawn(c)
			state := buildStateSnapshot(s, c)
			if err := s.sendUnreliable(c, state); err != nil {
				fmt.Printf("[net] state snapshot send failed id=%d err=%v\n", c.id, err)
				s.emitEvent(c, "state_snapshot_send_failed", "*protocol.Remote_NetClient_stateSnapshot_35", err.Error())
				return
			}
		case <-entityTicker.C:
			amount, data, err := s.buildPlayerEntitySnapshot()
			if err != nil {
				fmt.Printf("[net] entity snapshot build failed id=%d err=%v\n", c.id, err)
				s.emitEvent(c, "entity_snapshot_build_failed", "*protocol.Remote_NetClient_entitySnapshot_32", err.Error())
				return
			}
			// Keep lastSnapshotTimestamp fresh on client to prevent snapshot timeout.
			if err := s.sendUnreliable(c, &protocol.Remote_NetClient_entitySnapshot_32{Amount: amount, Data: data}); err != nil {
				fmt.Printf("[net] entity snapshot send failed id=%d err=%v\n", c.id, err)
				s.emitEvent(c, "entity_snapshot_send_failed", "*protocol.Remote_NetClient_entitySnapshot_32", err.Error())
				return
			}
		case <-keepAliveTicker.C:
			if err := c.SendAsync(&protocol.KeepAlive{}); err != nil {
				fmt.Printf("[net] keepalive send failed id=%d err=%v\n", c.id, err)
				s.emitEvent(c, "keepalive_send_failed", "*protocol.KeepAlive", err.Error())
				return
			}
		case <-c.closed:
			return
		}
	}
}

func buildStateSnapshot(s *Server, c *Conn) *protocol.Remote_NetClient_stateSnapshot_35 {
	if s != nil && s.StateSnapshotFn != nil {
		if snap := s.StateSnapshotFn(); snap != nil {
			if len(snap.CoreData) == 0 {
				snap.CoreData = []byte{0}
			}
			return snap
		}
	}
	// Minimal legal state snapshot payload:
	// coreData starts with "teams" byte (0 teams).
	return &protocol.Remote_NetClient_stateSnapshot_35{
		WaveTime: 0,
		Wave:     1,
		Enemies:  0,
		Paused:   false,
		GameOver: false,
		TimeData: int32(time.Now().Unix()),
		Tps:      60,
		Rand0:    time.Now().UnixNano(),
		Rand1:    int64(c.id),
		CoreData: []byte{0},
	}
}

func (s *Server) sendUnreliable(c *Conn, obj any) error {
	if c == nil {
		return nil
	}
	if s.udpConn != nil {
		if addr := c.UDPAddr(); addr != nil {
			payload, packetID, frameworkID, err := c.Encode(obj)
			if err != nil {
				c.udpErrors.Add(1)
				fmt.Printf("[net] sendUnreliable encode failed id=%d err=%v obj=%T\n", c.id, err, obj)
				return err
			}
			if packetID >= 0 && c.sendCount.Load() < 80 {
				fmt.Printf("[net] tx-udp id=%d packet_id=%d type=%T len=%d sendCompat=%v recvCompat=%v\n", c.id, packetID, obj, len(payload), c.sendCompatIDs, c.recvCompatIDs)
			}
			retries := s.UdpRetryCount
			delay := s.UdpRetryDelay
			if retries < 0 {
				retries = 0
			}
			if delay <= 0 {
				delay = 5 * time.Millisecond
			}
			for attempt := 0; attempt <= retries; attempt++ {
				_, err = s.udpConn.WriteToUDP(payload, addr)
				if err == nil {
					if c.onSend != nil {
						c.onSend(obj, packetID, frameworkID, len(payload))
					}
					c.udpSent.Add(1)
					c.bytesSent.Add(int64(len(payload)))
					c.recordSend(obj, int64(len(payload)))
					return nil
				}
				c.udpErrors.Add(1)
				if attempt < retries {
					time.Sleep(delay)
				}
			}
		}
	}
	if s.UdpFallbackTCP {
		return c.Send(obj)
	}
	return nil
}

func (s *Server) sendPlayerSpawn(c *Conn) bool {
	if c == nil || c.playerID == 0 || s.SpawnTileFn == nil {
		if s.DevLogger != nil {
			s.DevLogger.LogConnection("sendPlayerSpawn skipped", c.id, c.remoteIP(), c.name, c.uuid,
				devlog.BoolFld("has_player", c.playerID != 0),
				devlog.BoolFld("has_spawntilefn", s.SpawnTileFn != nil))
		}
		return false
	}
	pos, ok := s.SpawnTileFn()
	if !ok {
		if s.DevLogger != nil {
			s.DevLogger.LogConnection("sendPlayerSpawn no spawn tile", c.id, c.remoteIP(), c.name, c.uuid)
		}
		return false
	}
	return s.sendPlayerSpawnAt(c, pos)
}

func (s *Server) sendPlayerSpawnAt(c *Conn, pos protocol.Point2) bool {
	if c == nil || c.playerID == 0 {
		return false
	}
	tile := protocol.TileBox{PosValue: protocol.PackPoint2(pos.X, pos.Y)}
	player := &protocol.EntityBox{IDValue: c.playerID}
	fmt.Printf("[net] sendPlayerSpawn sending id=%d tile=(%d,%d) playerID=%d\n", c.id, pos.X, pos.Y, c.playerID)
	if err := c.Send(&protocol.Remote_CoreBlock_playerSpawn_140{Tile: tile, Player: player}); err != nil {
		fmt.Printf("[net] sendPlayerSpawn send failed id=%d err=%v\n", c.id, err)
		if s.DevLogger != nil {
			s.DevLogger.LogConnection("sendPlayerSpawn send failed", c.id, c.remoteIP(), c.name, c.uuid,
				devlog.StringFld("error", err.Error()))
		}
		return false
	}
	fmt.Printf("[net] sendPlayerSpawn sent id=%d\n", c.id)
	return true
}

func (s *Server) spawnPlayerInitial(c *Conn) {
	if c == nil || c.playerID == 0 || s.SpawnTileFn == nil {
		return
	}
	pos, ok := s.SpawnTileFn()
	if !ok {
		return
	}
	playerTypeID := int16(1)
	if s.PlayerUnitTypeFn != nil {
		if t := s.PlayerUnitTypeFn(); t >= 0 {
			playerTypeID = t
		}
	}
	if s.SpawnUnitFn != nil {
		c.unitID = s.nextUnitID()
		if x, y, ok := s.SpawnUnitFn(c, c.unitID, pos, playerTypeID); ok {
			c.snapX = x
			c.snapY = y
			c.lastSpawnAt = time.Now()
		} else {
			c.unitID = 0
		}
	}
	s.ensurePlayerUnitEntity(c)
	_ = s.sendPlayerSpawnAt(c, pos)
}

func (s *Server) forceRespawn(c *Conn, source string) {
	if c == nil || c.playerID == 0 {
		return
	}
	if s.SpawnTileFn == nil {
		return
	}
	pos, ok := s.SpawnTileFn()
	if !ok {
		return
	}
	playerTypeID := int16(1)
	if s.PlayerUnitTypeFn != nil {
		if t := s.PlayerUnitTypeFn(); t >= 0 {
			playerTypeID = t
		}
	}
	if s.SpawnUnitFn != nil {
		c.unitID = s.nextUnitID()
		if x, y, ok := s.SpawnUnitFn(c, c.unitID, pos, playerTypeID); ok {
			c.snapX = x
			c.snapY = y
			c.lastSpawnAt = time.Now()
		} else {
			c.unitID = 0
		}
	}
	s.ensurePlayerUnitEntity(c)
	player := &protocol.EntityBox{IDValue: c.playerID}
	_ = c.SendAsync(&protocol.Remote_InputHandler_unitClear_91{Player: player})
	if s.sendPlayerSpawnAt(c, pos) {
		c.dead = false
		c.deathTimer = 0
		c.lastRespawnCheck = time.Time{}
		c.lastRespawnReq = time.Now()
		fmt.Printf("[net] respawn sent conn=%d source=%s\n", c.id, source)
	} else {
		fmt.Printf("[net] respawn skipped conn=%d source=%s reason=no-spawn-tile\n", c.id, source)
	}
}

func (s *Server) requestRespawn(c *Conn, source string) {
	if c == nil || c.playerID == 0 {
		return
	}
	now := time.Now()
	cooldown := 600 * time.Millisecond
	if source == "compat-unitClear" {
		cooldown = 1500 * time.Millisecond
	}
	if !c.lastRespawnReq.IsZero() && now.Sub(c.lastRespawnReq) < cooldown {
		return
	}
	c.lastRespawnReq = now
	s.markDead(c, source)
	fmt.Printf("[net] respawn request conn=%d source=%s\n", c.id, source)
}

func (s *Server) respawnDelayFrames() float32 {
	if s == nil || s.RespawnDelayFrames <= 0 {
		return 60
	}
	return s.RespawnDelayFrames
}

func (s *Server) markDead(c *Conn, source string) {
	if c == nil || c.playerID == 0 {
		return
	}
	if c.unitID != 0 {
		s.dropPlayerUnitEntity(c, c.unitID)
		c.unitID = 0
	}
	if !c.dead {
		c.dead = true
		c.deathTimer = 0
		c.lastRespawnCheck = time.Now()
		c.lastSpawnAt = time.Time{}
	}
	if s.DevLogger != nil {
		s.DevLogger.LogConnection("player_dead", c.id, c.remoteIP(), c.name, c.uuid,
			devlog.StringFld("source", source))
	}
}

func (s *Server) maybeRespawn(c *Conn) {
	if c == nil || c.playerID == 0 {
		return
	}
	if !c.dead {
		if c.unitID == 0 {
			s.markDead(c, "unit-missing")
			return
		}
		// Avoid immediate false-death right after spawn.
		if !c.lastSpawnAt.IsZero() && time.Since(c.lastSpawnAt) < 1500*time.Millisecond {
			return
		}
		s.entityMu.Lock()
		ent, ok := s.entities[c.unitID]
		s.entityMu.Unlock()
		if !ok {
			s.markDead(c, "unit-entity-missing")
			return
		}
		if s.UnitInfoFn != nil {
			info, ok := s.UnitInfoFn(c.unitID)
			if !ok {
				s.markDead(c, "unit-state-missing")
				return
			}
			if info.Health <= 0 {
				s.markDead(c, "unit-health")
				return
			}
		}
		if u, ok := ent.(*protocol.UnitEntitySync); ok {
			if u.Health <= 0 {
				s.markDead(c, "unit-health")
				return
			}
		}
		c.deathTimer = 0
		c.lastRespawnCheck = time.Now()
		return
	}
	if s.SpawnTileFn == nil {
		c.lastRespawnCheck = time.Now()
		return
	}
	if _, ok := s.SpawnTileFn(); !ok {
		c.lastRespawnCheck = time.Now()
		return
	}
	now := time.Now()
	if c.lastRespawnCheck.IsZero() {
		c.lastRespawnCheck = now
		return
	}
	dt := now.Sub(c.lastRespawnCheck).Seconds()
	if dt <= 0 {
		c.lastRespawnCheck = now
		return
	}
	c.lastRespawnCheck = now
	// Mindustry Time.delta is scaled to 60 FPS; deathDelay=60f == ~1s.
	c.deathTimer += float32(dt) * 60
	if c.deathTimer >= s.respawnDelayFrames() {
		c.deathTimer = 0
		s.forceRespawn(c, "death-timer")
	}
}

func (s *Server) MarkUnitDead(unitID int32, source string) {
	if s == nil || unitID == 0 {
		return
	}
	s.mu.Lock()
	conns := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		conns = append(conns, c)
	}
	s.mu.Unlock()
	for _, c := range conns {
		if c.unitID == unitID {
			s.markDead(c, source)
			return
		}
	}
}

func (s *Server) buildPlayerEntitySnapshot() (int16, []byte, error) {
	// TypeIO.WriteBytes uses int16 length; keep snapshot data under signed-short limit.
	const maxEntitySnapshotData = 32000

	s.mu.Lock()
	players := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		if c.hasConnected && c.playerID != 0 {
			players = append(players, c)
		}
	}
	s.mu.Unlock()

	w := protocol.NewWriterWithContext(s.TypeIO)
	var total int16
	appendEntity := func(id int32, classID byte, syncWrite func(*protocol.Writer) error) error {
		ew := protocol.NewWriterWithContext(s.TypeIO)
		if err := ew.WriteInt32(id); err != nil {
			return err
		}
		if err := ew.WriteByte(classID); err != nil {
			return err
		}
		if err := syncWrite(ew); err != nil {
			return err
		}
		entry := ew.Bytes()
		if len(w.Bytes())+len(entry) > maxEntitySnapshotData {
			return io.EOF
		}
		if err := w.WriteBytes(entry); err != nil {
			return err
		}
		total++
		return nil
	}
	for _, p := range players {
		ent := s.ensurePlayerEntity(p)
		s.updatePlayerEntity(ent, p)
		if err := appendEntity(ent.ID(), ent.ClassID(), ent.WriteSync); err != nil {
			if errors.Is(err, io.EOF) {
				break
			}
			return 0, nil, err
		}
		// Send currently controlled player unit entity when available.
		unit := s.playerUnitEntity(p)
		if unit != nil {
			if err := appendEntity(unit.ID(), unit.ClassID(), unit.WriteSync); err != nil {
				if errors.Is(err, io.EOF) {
					break
				}
				return 0, nil, err
			}
		}
	}
	if s.ExtraEntitySnapshotFn != nil {
		n, err := s.ExtraEntitySnapshotFn(w)
		if err != nil {
			return 0, nil, err
		}
		total += n
	}
	if len(w.Bytes()) > maxEntitySnapshotData {
		return total, w.Bytes()[:maxEntitySnapshotData], nil
	}
	return total, w.Bytes(), nil
}

func writePlayerSync(w *protocol.Writer, c *Conn) error {
	if err := w.WriteBool(false); err != nil { // admin
		return err
	}
	if err := w.WriteBool(c.boosting); err != nil { // boosting
		return err
	}
	if err := protocol.WriteColor(w, protocol.Color{RGBA: c.color}); err != nil {
		return err
	}
	if err := w.WriteFloat32(c.pointerX); err != nil { // mouseX
		return err
	}
	if err := w.WriteFloat32(c.pointerY); err != nil { // mouseY
		return err
	}
	name := c.name
	if err := protocol.WriteString(w, &name); err != nil {
		return err
	}
	if err := w.WriteInt16(-1); err != nil { // selectedBlock
		return err
	}
	if err := w.WriteInt32(0); err != nil { // selectedRotation
		return err
	}
	if err := w.WriteBool(c.shooting); err != nil { // shooting
		return err
	}
	if err := protocol.WriteTeam(w, &protocol.Team{ID: 1}); err != nil {
		return err
	}
	if err := w.WriteBool(c.typing); err != nil { // typing
		return err
	}
	if err := protocol.WriteUnit(w, nil); err != nil {
		return err
	}
	if err := w.WriteFloat32(c.snapX); err != nil { // x
		return err
	}
	if err := w.WriteFloat32(c.snapY); err != nil { // y
		return err
	}
	return nil
}

func (s *Server) ensurePlayerEntity(c *Conn) *protocol.PlayerEntity {
	if c == nil || c.playerID == 0 {
		return nil
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	if ent, ok := s.entities[c.playerID]; ok {
		if p, ok2 := ent.(*protocol.PlayerEntity); ok2 {
			return p
		}
	}
	p := &protocol.PlayerEntity{IDValue: c.playerID}
	s.entities[c.playerID] = p
	return p
}

func (s *Server) updatePlayerEntity(p *protocol.PlayerEntity, c *Conn) {
	if p == nil || c == nil {
		return
	}
	p.Admin = false
	p.Boosting = c.boosting
	p.ColorRGBA = c.color
	p.MouseX = c.pointerX
	p.MouseY = c.pointerY
	p.Name = c.name
	p.SelectedBlock = -1
	p.SelectedRotation = 0
	p.Shooting = c.shooting
	p.TeamID = 1
	p.Typing = c.typing
	if c.unitID != 0 {
		p.Unit = protocol.UnitBox{IDValue: c.unitID}
	} else {
		p.Unit = nil
	}
	p.X = c.snapX
	p.Y = c.snapY
}

func (s *Server) nextUnitID() int32 {
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	id := s.unitNext
	s.unitNext++
	if s.unitNext <= 0 {
		s.unitNext = 2000000000
	}
	return id
}

func (s *Server) ensurePlayerUnitEntity(c *Conn) *protocol.UnitEntitySync {
	if c == nil || c.playerID == 0 {
		return nil
	}
	if c.unitID == 0 {
		c.unitID = s.nextUnitID()
	}
	playerTypeID := int16(1)
	if s.PlayerUnitTypeFn != nil {
		if t := s.PlayerUnitTypeFn(); t >= 0 {
			playerTypeID = t
		}
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	if ent, ok := s.entities[c.unitID]; ok {
		if u, ok2 := ent.(*protocol.UnitEntitySync); ok2 {
			// keep unit synced with latest client snapshot values
			u.Controller = &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}
			if u.Plans == nil {
				u.Plans = []*protocol.BuildPlan{}
			}
			if u.Mounts == nil {
				u.Mounts = []protocol.WeaponMount{}
			}
			if u.Abilities == nil {
				u.Abilities = []protocol.Ability{}
			}
			if u.Statuses == nil {
				u.Statuses = []protocol.StatusEntry{}
			}
			if u.Stack.Item == nil {
				u.Stack.Item = protocol.ItemRef{ItmID: 0, ItmName: ""}
				u.Stack.Amount = 0
			}
			s.syncUnitFromWorld(u)
			if u.X == 0 && u.Y == 0 {
				u.X = c.snapX
				u.Y = c.snapY
			}
			u.Rotation = 90
			u.TeamID = 1
			u.TypeID = playerTypeID
			u.Elevation = 1
			if u.Health <= 0 {
				u.Health = 100
			}
			return u
		}
	}
	u := &protocol.UnitEntitySync{
		IDValue:        c.unitID,
		Abilities:      []protocol.Ability{},
		Ammo:           0,
		Controller:     &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID},
		Elevation:      1,
		Flag:           0,
		Health:         100,
		Shooting:       false,
		MineTile:       nil,
		Mounts:         []protocol.WeaponMount{},
		Plans:          []*protocol.BuildPlan{},
		Rotation:       90,
		Shield:         0,
		SpawnedByCore:  true,
		Stack:          protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}, Amount: 0},
		Statuses:       []protocol.StatusEntry{},
		TeamID:         1,
		TypeID:         playerTypeID,
		UpdateBuilding: false,
		Vel:            protocol.Vec2{X: 0, Y: 0},
		X:              c.snapX,
		Y:              c.snapY,
	}
	s.syncUnitFromWorld(u)
	s.entities[c.unitID] = u
	return u
}

func (s *Server) playerUnitEntity(c *Conn) *protocol.UnitEntitySync {
	if c == nil || c.playerID == 0 || c.unitID == 0 {
		return nil
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	if ent, ok := s.entities[c.unitID]; ok {
		if u, ok2 := ent.(*protocol.UnitEntitySync); ok2 {
			u.Controller = &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}
			s.syncUnitFromWorld(u)
			if u.X == 0 && u.Y == 0 {
				u.X = c.snapX
				u.Y = c.snapY
			}
			return u
		}
	}
	return nil
}

func (s *Server) syncUnitFromWorld(u *protocol.UnitEntitySync) {
	if s == nil || u == nil || s.UnitInfoFn == nil {
		return
	}
	info, ok := s.UnitInfoFn(u.ID())
	if !ok {
		return
	}
	u.X = info.X
	u.Y = info.Y
	if info.Health >= 0 {
		u.Health = info.Health
	}
	if info.TypeID != 0 {
		u.TypeID = info.TypeID
	}
}

func (s *Server) dropPlayerUnitEntity(c *Conn, unitID int32) {
	if c == nil || c.playerID == 0 || unitID == 0 {
		return
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	ent, ok := s.entities[unitID]
	if !ok {
		return
	}
	u, ok := ent.(*protocol.UnitEntitySync)
	if !ok {
		return
	}
	state, ok := u.Controller.(*protocol.ControllerState)
	if !ok || state == nil || state.Type != protocol.ControllerPlayer || state.PlayerID != c.playerID {
		return
	}
	delete(s.entities, unitID)
	if s.DropUnitFn != nil {
		s.DropUnitFn(unitID)
	}
}

func (s *Server) broadcastPlayerDisconnect(playerID int32, except *Conn) {
	s.mu.Lock()
	peers := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		if c == nil || c == except {
			continue
		}
		peers = append(peers, c)
	}
	s.mu.Unlock()
	for _, peer := range peers {
		_ = peer.SendAsync(&protocol.Remote_NetClient_playerDisconnect_31{Playerid: playerID})
	}
}

func (s *Server) addEntity(u protocol.Unit) {
	if u == nil {
		return
	}
	// Convert Unit to UnitSyncEntity if needed
	var syncEnt protocol.UnitSyncEntity
	if ent, ok := u.(protocol.UnitSyncEntity); ok {
		syncEnt = ent
	} else {
		// If it's just a Unit with no sync support, create a sync entity from it
		// This is a simplified approach - in practice, units should already be UnitSyncEntity
		return
	}

	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	id := syncEnt.ID()
	if id == 0 {
		// Assign a new ID if not set
		s.unitNext++
		id = s.unitNext
		syncEnt.SetID(id)
	}
	s.entities[id] = syncEnt
}

// removeEntity 从实体列表中移除指定ID的实体
func (s *Server) removeEntity(id int32) {
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	delete(s.entities, id)
}

// getEntity 获取指定ID的实体
func (s *Server) getEntity(id int32) (protocol.UnitSyncEntity, bool) {
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	ent, ok := s.entities[id]
	if syncEnt, ok2 := ent.(protocol.UnitSyncEntity); ok2 && ok {
		return syncEnt, true
	}
	return nil, false
}

// broadcastEntitySync 广播实体同步数据给所有连接
func (s *Server) broadcastEntitySync(entity protocol.UnitSyncEntity) {
	s.mu.Lock()
	peers := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		if c != nil && c.hasConnected {
			peers = append(peers, c)
		}
	}
	s.mu.Unlock()

	for _, c := range peers {
		if err := c.Send(&protocol.Remote_NetClient_entitySnapshot_32{
			Amount: 1,
			Data:   s.serializeEntity(entity),
		}); err != nil {
			fmt.Printf("[net] failed to send entity sync: %v\n", err)
		}
	}
}

// serializeEntity 序列化实体为字节数组
func (s *Server) serializeEntity(entity protocol.UnitSyncEntity) []byte {
	w := protocol.NewWriter()
	if err := entity.WriteSync(w); err != nil {
		return nil
	}
	return w.Bytes()
}

// sendWorldHandshake pushes world stream data to the connected client.
func (s *Server) sendWorldHandshake(c *Conn, pkt *protocol.ConnectPacket) error {
	worldData, err := s.WorldDataFn(c, pkt)
	if err != nil {
		return err
	}
	worldID, ok := s.Registry.PacketID(&protocol.WorldStream{})
	if !ok {
		return fmt.Errorf("world stream packet id not found")
	}
	return c.SendStream(worldID, worldData)
}

// ReloadWorldLiveForAll pushes a fresh world stream to all online players without kicking.
// After stream sync, it forces a respawn so client camera/unit rebinds to the new world.
func (s *Server) ReloadWorldLiveForAll() (reloaded int, failed int) {
	s.mu.Lock()
	peers := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		if c != nil && c.hasConnected && c.playerID != 0 {
			peers = append(peers, c)
		}
	}
	s.mu.Unlock()

	for _, c := range peers {
		if err := s.sendWorldHandshake(c, nil); err != nil {
			failed++
			s.emitEvent(c, "world_hot_reload_failed", "", err.Error())
			continue
		}
		// Rebind player unit to new map spawn after world replacement.
		s.forceRespawn(c, "map-hot-reload")
		reloaded++
	}
	return reloaded, failed
}

func defaultWorldData(_ *Conn, _ *protocol.ConnectPacket) ([]byte, error) {
	fileCandidates := []string{
		filepath.Join("assets", "bootstrap-world.bin"),
		filepath.Join("go-server", "assets", "bootstrap-world.bin"),
	}
	for _, p := range fileCandidates {
		data, err := os.ReadFile(p)
		if err == nil && len(data) > 0 {
			return data, nil
		}
	}

	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

type SessionInfo struct {
	ID        int32
	Name      string
	UUID      string
	IP        string
	Connected bool
	Stats     ConnStats
}

type ConnStats struct {
	Sent        int64
	SendErrors  int64
	Queued      int64
	QueueFull   int64
	BytesSent   int64
	UdpSent     int64
	UdpErrors   int64
	ByTypeSent  map[string]int64
	ByTypeBytes map[string]int64
}

func (c *Conn) Stats() ConnStats {
	byTypeSent := map[string]int64{}
	byTypeBytes := map[string]int64{}
	c.statsMu.Lock()
	for k, v := range c.byTypeSent {
		byTypeSent[k] = v
	}
	for k, v := range c.byTypeBytes {
		byTypeBytes[k] = v
	}
	c.statsMu.Unlock()
	return ConnStats{
		Sent:        c.sendCount.Load(),
		SendErrors:  c.sendErrors.Load(),
		Queued:      c.sendQueued.Load(),
		QueueFull:   c.sendQueueFull.Load(),
		BytesSent:   c.bytesSent.Load(),
		UdpSent:     c.udpSent.Load(),
		UdpErrors:   c.udpErrors.Load(),
		ByTypeSent:  byTypeSent,
		ByTypeBytes: byTypeBytes,
	}
}

func (c *Conn) recordSend(obj any, size int64) {
	name := fmt.Sprintf("%T", obj)
	c.statsMu.Lock()
	c.byTypeSent[name]++
	c.byTypeBytes[name] += size
	c.statsMu.Unlock()
}

func (s *Server) ListSessions() []SessionInfo {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]SessionInfo, 0, len(s.conns))
	for c := range s.conns {
		out = append(out, SessionInfo{
			ID:        c.id,
			Name:      c.name,
			UUID:      c.uuid,
			IP:        c.remoteIP(),
			Connected: c.hasConnected,
			Stats:     c.Stats(),
		})
	}
	return out
}

func (s *Server) KickByID(id int32, reason string) bool {
	var target *Conn
	s.mu.Lock()
	for c := range s.conns {
		if c.id == id {
			target = c
			break
		}
	}
	s.mu.Unlock()
	if target == nil {
		return false
	}
	if reason == "" {
		reason = "kicked by admin"
	}
	_ = target.SendAsync(&protocol.Remote_NetClient_kick_22{Reason: reason})
	s.closeConnLater(target, 250*time.Millisecond)
	return true
}

// KickForMapChange sends the original-like map-change kick string (id=53 on compat mapping)
// and then closes after a short delay so clients can auto-reconnect cleanly.
func (s *Server) KickForMapChange(id int32) bool {
	var target *Conn
	s.mu.Lock()
	for c := range s.conns {
		if c.id == id {
			target = c
			break
		}
	}
	s.mu.Unlock()
	if target == nil {
		return false
	}
	// Use blocking send for map-change path to avoid drop under send-queue pressure.
	_ = target.Send(&protocol.Remote_NetClient_kick_22{Reason: "map changed, please reconnect"})
	s.closeConnLater(target, 400*time.Millisecond)
	return true
}

func (s *Server) closeConnLater(c *Conn, delay time.Duration) {
	if c == nil {
		return
	}
	if delay < 0 {
		delay = 0
	}
	time.AfterFunc(delay, func() {
		_ = c.Close()
	})
}

func (s *Server) BanUUID(uuid, reason string) int {
	if uuid == "" {
		return 0
	}
	if reason == "" {
		reason = "banned by admin"
	}
	s.mu.Lock()
	s.banUUID[uuid] = reason
	var targets []*Conn
	for c := range s.conns {
		if c.uuid == uuid {
			targets = append(targets, c)
		}
	}
	s.mu.Unlock()
	for _, c := range targets {
		_ = c.SendAsync(&protocol.Remote_NetClient_kick_22{Reason: reason})
		s.closeConnLater(c, 250*time.Millisecond)
	}
	return len(targets)
}

func (s *Server) BanIP(ip, reason string) int {
	if ip == "" {
		return 0
	}
	if reason == "" {
		reason = "banned by admin"
	}
	s.mu.Lock()
	s.banIP[ip] = reason
	var targets []*Conn
	for c := range s.conns {
		if c.remoteIP() == ip {
			targets = append(targets, c)
		}
	}
	s.mu.Unlock()
	for _, c := range targets {
		_ = c.SendAsync(&protocol.Remote_NetClient_kick_22{Reason: reason})
		s.closeConnLater(c, 250*time.Millisecond)
	}
	return len(targets)
}

func (s *Server) UnbanUUID(uuid string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.banUUID[uuid]; !ok {
		return false
	}
	delete(s.banUUID, uuid)
	return true
}

func (s *Server) UnbanIP(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.banIP[ip]; !ok {
		return false
	}
	delete(s.banIP, ip)
	return true
}

func (s *Server) BanLists() (map[string]string, map[string]string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	uuidCopy := make(map[string]string, len(s.banUUID))
	for k, v := range s.banUUID {
		uuidCopy[k] = v
	}
	ipCopy := make(map[string]string, len(s.banIP))
	for k, v := range s.banIP {
		ipCopy[k] = v
	}
	return uuidCopy, ipCopy
}

func (s *Server) AddOp(uuid string) {
	if uuid == "" {
		return
	}
	s.opMu.Lock()
	s.ops[uuid] = struct{}{}
	s.opMu.Unlock()
}

func (s *Server) RemoveOp(uuid string) {
	if uuid == "" {
		return
	}
	s.opMu.Lock()
	delete(s.ops, uuid)
	s.opMu.Unlock()
}

func (s *Server) IsOp(uuid string) bool {
	if uuid == "" {
		return false
	}
	s.opMu.RLock()
	_, ok := s.ops[uuid]
	s.opMu.RUnlock()
	return ok
}

func (s *Server) ListOps() []string {
	s.opMu.RLock()
	defer s.opMu.RUnlock()
	out := make([]string, 0, len(s.ops))
	for u := range s.ops {
		out = append(out, u)
	}
	return out
}

func (s *Server) PlayerUnitIDSet() map[int32]struct{} {
	out := map[int32]struct{}{}
	if s == nil {
		return out
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		if c != nil && c.unitID != 0 {
			out[c.unitID] = struct{}{}
		}
	}
	return out
}

func (s *Server) unitControl(c *Conn, unitID int32) {
	if c == nil || c.playerID == 0 || unitID == 0 {
		return
	}
	// Look up unit info from world when available.
	var info UnitInfo
	if s.UnitInfoFn != nil {
		if v, ok := s.UnitInfoFn(unitID); ok {
			info = v
		} else {
			return
		}
	}

	// Load or create entity sync entry for this unit.
	var u *protocol.UnitEntitySync
	s.entityMu.Lock()
	if ent, ok := s.entities[unitID]; ok {
		if uu, ok2 := ent.(*protocol.UnitEntitySync); ok2 {
			u = uu
		}
	}
	if u == nil {
		u = &protocol.UnitEntitySync{
			IDValue:        unitID,
			Abilities:      []protocol.Ability{},
			Controller:     nil,
			Elevation:      0,
			Flag:           0,
			Health:         info.Health,
			Shooting:       false,
			Mounts:         []protocol.WeaponMount{},
			Plans:          []*protocol.BuildPlan{},
			Rotation:       0,
			Shield:         0,
			SpawnedByCore:  false,
			Stack:          protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}, Amount: 0},
			Statuses:       []protocol.StatusEntry{},
			TeamID:         info.TeamID,
			TypeID:         info.TypeID,
			UpdateBuilding: false,
			Vel:            protocol.Vec2{X: 0, Y: 0},
			X:              info.X,
			Y:              info.Y,
		}
		s.entities[unitID] = u
	}
	s.entityMu.Unlock()

	// Basic validation: same team and reasonably close.
	if info.TeamID != 0 && info.TeamID != 1 {
		return
	}
	dx := float64(info.X - c.snapX)
	dy := float64(info.Y - c.snapY)
	if dx*dx+dy*dy > 120*120 {
		return
	}
	if state, ok := u.Controller.(*protocol.ControllerState); ok && state != nil {
		if state.Type == protocol.ControllerPlayer && state.PlayerID != c.playerID {
			return
		}
	}
	u.Controller = &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}
	c.unitID = unitID
}

func extractUnitID(obj any) int32 {
	switch v := obj.(type) {
	case nil:
		return 0
	case protocol.UnitBox:
		return v.ID()
	case *protocol.EntityBox:
		if v == nil {
			return 0
		}
		return v.ID()
	case protocol.UnitSyncEntity:
		if v == nil {
			return 0
		}
		return v.ID()
	case *protocol.UnitEntitySync:
		if v == nil {
			return 0
		}
		return v.ID()
	default:
		return 0
	}
}
