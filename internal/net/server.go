package net

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math"
	"math/rand"
	"net"
	"reflect"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode/utf8"

	"mdt-server/internal/devlog"
	"mdt-server/internal/protocol"
	"mdt-server/internal/runtimeassets"
	"mdt-server/internal/storage"
	"mdt-server/internal/worldstream"
)

var globalVerboseNetLog atomic.Bool

const invalidTilePos int32 = -1
const maxMindustryPlayerNameBytes = 40

const (
	previewPlanCommitDelay = 100 * time.Millisecond
	maxPlayerPreviewPlans  = 1000
	previewPlanChunkSize   = 900 / 12
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

type BeginPlaceRequest struct {
	X        int32
	Y        int32
	BlockID  int16
	Rotation int8
	Config   any
}

type ConstructFinishRequest struct {
	Pos       int32
	BlockID   int16
	BuilderID int32
	Rotation  int8
	TeamID    byte
	Config    any
}

type DeconstructFinishRequest struct {
	Pos       int32
	BlockID   int16
	BuilderID int32
}

type previewPlanState struct {
	teamID        byte
	current       []*protocol.BuildPlan
	assembling    []*protocol.BuildPlan
	lastRecvGroup int32
	nextSendGroup int32
	lastRecvAt    time.Time
	receiving     bool
}

type AssemblerDroneSpawnedRequest struct {
	Pos    int32
	UnitID int32
}

type Server struct {
	Addr     string
	Registry *protocol.PacketRegistry
	Serial   *Serializer
	Content  *protocol.ContentRegistry
	TypeIO   *protocol.TypeIOContext

	mu              sync.Mutex
	conns           map[*Conn]struct{}
	pending         map[int32]*Conn
	byUDP           map[string]*Conn
	banUUID         map[string]string
	banIP           map[string]string
	admissionMu     sync.RWMutex
	admission       AdmissionPolicy
	recentKickUntil map[string]time.Time
	udpConn         *net.UDPConn
	tcpLn           *net.TCPListener
	shuttingDown    atomic.Bool
	opMu            sync.RWMutex
	ops             map[string]struct{}
	logMu           sync.Mutex
	logTimes        map[string]time.Time
	entityMu        sync.Mutex
	entities        map[int32]protocol.UnitSyncEntity
	unitNext        int32

	previewMu    sync.Mutex
	previewPlans map[int32]*previewPlanState

	// EventManager 事件管理器
	EventManager *storage.EventManager

	BuildVersion int
	WorldDataFn  func(*Conn, *protocol.ConnectPacket) ([]byte, error)
	OnEvent      func(NetEvent)
	playerIDNext int32

	Name                string
	Description         string
	VirtualPlayers      int32
	MapNameFn           func() string
	OnChat              func(*Conn, string) bool
	SpawnTileFn         func() (protocol.Point2, bool)
	SpawnTileForConnFn  func(*Conn) (protocol.Point2, bool)
	AssignTeamForConnFn func(*Conn) byte
	// Optional provider for player-respawn unit type id (e.g. alpha).
	PlayerUnitTypeFn func() int16
	// Optional hook: apply client build plans from snapshot.
	OnBuildPlans func(*Conn, []*protocol.BuildPlan)
	// Optional hook: preview plans from clientSnapshot.
	OnBuildPlanPreview func(*Conn, []*protocol.BuildPlan)
	// Optional hooks: official building RPC chains.
	OnBeginBreak func(*Conn, int32, int32)
	OnBeginPlace func(*Conn, BeginPlaceRequest)
	// Optional hook: apply authoritative plans queue from client snapshots.
	OnBuildPlanSnapshot func(*Conn, []*protocol.BuildPlan)
	// Optional hooks: cancel queued build plans from client side.
	OnDeletePlans               func(*Conn, []int32)
	OnRemoveQueueBlock          func(*Conn, int32, int32, bool)
	OnRequestUnitPayload        func(*Conn, int32)
	OnRequestBuildPayload       func(*Conn, int32)
	OnRequestDropPayload        func(*Conn, float32, float32)
	OnUnitEnteredPayload        func(*Conn, int32, int32)
	OnCommandUnits              func(*Conn, []int32, any, any, any, bool, bool)
	OnSetUnitCommand            func(*Conn, []int32, *protocol.UnitCommand)
	OnSetUnitStance             func(*Conn, []int32, protocol.UnitStance, bool)
	OnCommandBuilding           func(*Conn, []int32, protocol.Vec2)
	OnRotateBlock               func(*Conn, int32, bool)
	OnBuildingControlSelect     func(*Conn, int32)
	OnUnitBuildingControlSelect func(*Conn, int32, int32)
	OnRequestItem               func(*Conn, int32, int16, int32)
	OnTransferInventory         func(*Conn, int32)
	OnRequestBlockSnapshot      func(*Conn, int32)
	OnDropItem                  func(*Conn, float32)
	OnTileConfig                func(*Conn, int32, any)
	OnConstructFinish           func(*Conn, ConstructFinishRequest)
	OnDeconstructFinish         func(*Conn, DeconstructFinishRequest)
	OnAssemblerUnitSpawned      func(*Conn, int32)
	OnAssemblerDroneSpawned     func(*Conn, AssemblerDroneSpawnedRequest)
	OnMenuChoose                func(*Conn, int32, int32)
	// Respawn delay in "frames" (Mindustry Time.delta units). Defaults to 60 if zero.
	RespawnDelayFrames float32
	// Optional hook: spawn player unit in world/state. Uses provided unitID, returns world coords.
	SpawnUnitFn func(c *Conn, unitID int32, tile protocol.Point2, unitType int16) (float32, float32, bool)
	// Optional hook: spawn/attach a player-controlled unit directly at a world position.
	// This mirrors vanilla InputHandler.unitClear() dock-style respawn for core builder units.
	SpawnUnitAtFn func(c *Conn, unitID int32, x, y, rotation float32, unitType int16, spawnedByCore bool) (float32, float32, bool)
	// Optional hook: remove a unit from world/state.
	DropUnitFn func(unitID int32)
	// Optional hook: query unit info from world/state.
	UnitInfoFn func(unitID int32) (UnitInfo, bool)
	// Optional hook: build an authoritative sync snapshot for one unit from world state.
	UnitSyncFn func(unitID int32, controller protocol.UnitController) (*protocol.UnitEntitySync, bool)
	// Optional hook: resolve the actual respawn unit type for a specific core/spawn tile.
	ResolveRespawnUnitTypeFn func(c *Conn, tile protocol.Point2, fallback int16) int16
	// Optional hook: reserve a world-authoritative entity ID for player units.
	ReserveUnitIDFn func() int32
	// Optional hooks: apply client snapshot motion/position into authoritative world state.
	// If unset, clientSnapshot will only update connection state but will not move entities in the world.
	SetUnitMotionFn           func(unitID int32, vx, vy, rotVel float32) bool
	SetUnitPositionFn         func(unitID int32, x, y, rotation float32) bool
	SetUnitRuntimeStateFn     func(unitID int32, state UnitRuntimeState) bool
	SetUnitStackFn            func(unitID int32, itemID int16, amount int32) bool
	SetUnitPlayerControllerFn func(unitID int32, playerID int32) bool
	ClaimControlledBuildFn    func(playerID int32, buildPos int32) (ControlledBuildInfo, bool)
	ControlledBuildInfoFn     func(playerID int32, buildPos int32) (ControlledBuildInfo, bool)
	ReleaseControlledBuildFn  func(playerID int32, buildPos int32) bool
	SetControlledBuildInputFn func(playerID int32, buildPos int32, aimX, aimY float32, shooting bool) bool
	// Optional hook: called once after initial connect/spawn sequence starts.
	OnPostConnect     func(*Conn)
	OnHotReloadConnFn func(*Conn)
	// Optional network hooks for external core scheduling.
	OnConnOpen      func(*Conn)
	OnConnClose     func(*Conn)
	OnPacketDecoded func(*Conn, any, error) bool
	OnTracePacket   func(direction string, c *Conn, obj any, packetID int, frameworkID int, size int)

	StateSnapshotFn func() *protocol.Remote_NetClient_stateSnapshot_35
	// Appends additional entities into entity snapshot stream.
	// Return value is appended entity count.
	ExtraEntitySnapshotFn func(w *protocol.Writer) (int16, error)
	// Appends additional sync entities into entity snapshots using the same
	// per-entity packet splitting path as vanilla NetServer.writeEntitySnapshot().
	ExtraEntitySnapshotEntitiesFn func() ([]protocol.UnitSyncEntity, error)
	// Optional per-viewer hide hook for entity snapshots. Hidden IDs are sent
	// through hiddenSnapshot and omitted from the viewer's entitySnapshot stream.
	EntitySnapshotHiddenFn func(viewer *Conn, entity protocol.UnitSyncEntity) bool

	UdpRetryCount  int
	UdpRetryDelay  time.Duration
	UdpFallbackTCP bool

	entitySnapshotIntervalNs  atomic.Int64
	stateSnapshotIntervalNs   atomic.Int64
	clientSnapshotConfirm     atomic.Bool
	infoMu                    sync.RWMutex
	verboseNetLog             atomic.Bool
	packetRecvEventsEnabled   atomic.Bool
	packetSendEventsEnabled   atomic.Bool
	terminalPlayerLogsEnabled atomic.Bool
	terminalPlayerUUIDEnabled atomic.Bool
	respawnPacketLogsEnabled  atomic.Bool
	playerNameColorEnabled    atomic.Bool
	translatedConnLog         atomic.Bool
	joinLeaveChatEnabled      atomic.Bool
	publicConnIDFormatter     func(*Conn) string
	playerDisplayFormatter    func(*Conn) string

	// DevLogger 开发者日志（可选）
	DevLogger *devlog.DevLogger

	// CommandHandler 命令处理器
	CommandHandler *CommandHandler

	// VoteKickManager 投票踢人管理器
	VoteKickManager *VoteKickManager

	// AdminManager 管理员管理器
	AdminManager *AdminManager
}

func NewServer(addr string, _ int) *Server {
	reg := protocol.NewRegistry()
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
		admission:          DefaultAdmissionPolicy(),
		recentKickUntil:    map[string]time.Time{},
		EventManager:       em,
		BuildVersion:       157,
		WorldDataFn:        defaultWorldData,
		playerIDNext:       0,
		Name:               "mdt-server",
		Description:        "",
		VirtualPlayers:     0,
		entities:           map[int32]protocol.UnitSyncEntity{},
		previewPlans:       map[int32]*previewPlanState{},
		unitNext:           2000000000,
		ops:                map[string]struct{}{},
		logTimes:           map[string]time.Time{},
		UdpRetryCount:      2,
		UdpRetryDelay:      5 * time.Millisecond,
		UdpFallbackTCP:     true,
		RespawnDelayFrames: 60,
	}
	s.SetSnapshotIntervals(200, 200)
	s.verboseNetLog.Store(false)
	s.packetRecvEventsEnabled.Store(false)
	s.packetSendEventsEnabled.Store(false)
	s.terminalPlayerLogsEnabled.Store(true)
	s.terminalPlayerUUIDEnabled.Store(false)
	s.respawnPacketLogsEnabled.Store(true)
	s.playerNameColorEnabled.Store(true)
	s.translatedConnLog.Store(true)
	s.joinLeaveChatEnabled.Store(true)

	// 初始化命令处理器
	s.CommandHandler = NewCommandHandler()
	s.CommandHandler.RegisterDefaultCommands(s)

	// 初始化投票踢人管理器
	s.VoteKickManager = NewVoteKickManager(s)

	// 初始化管理员管理器
	s.AdminManager = NewAdminManager()
	globalServer = s

	return s
}

func (s *Server) SetVerboseNetLog(enabled bool) {
	s.verboseNetLog.Store(enabled)
	globalVerboseNetLog.Store(enabled)
}

func (s *Server) VerboseNetLogEnabled() bool {
	return s.verboseNetLog.Load()
}

func (s *Server) SetPacketRecvEventsEnabled(enabled bool) {
	s.packetRecvEventsEnabled.Store(enabled)
}

func (s *Server) PacketRecvEventsEnabled() bool {
	return s.packetRecvEventsEnabled.Load()
}

func (s *Server) SetPacketSendEventsEnabled(enabled bool) {
	s.packetSendEventsEnabled.Store(enabled)
}

func (s *Server) PacketSendEventsEnabled() bool {
	return s.packetSendEventsEnabled.Load()
}

func (s *Server) SetTerminalPlayerLogsEnabled(enabled bool) {
	s.terminalPlayerLogsEnabled.Store(enabled)
}

func (s *Server) TerminalPlayerLogsEnabled() bool {
	return s.terminalPlayerLogsEnabled.Load()
}

func (s *Server) SetTerminalPlayerUUIDEnabled(enabled bool) {
	s.terminalPlayerUUIDEnabled.Store(enabled)
}

func (s *Server) TerminalPlayerUUIDEnabled() bool {
	return s.terminalPlayerUUIDEnabled.Load()
}

func (s *Server) SetRespawnPacketLogsEnabled(enabled bool) {
	s.respawnPacketLogsEnabled.Store(enabled)
}

func (s *Server) RespawnPacketLogsEnabled() bool {
	return s.respawnPacketLogsEnabled.Load()
}

func (s *Server) SetPlayerNameColorEnabled(enabled bool) {
	s.playerNameColorEnabled.Store(enabled)
}

func (s *Server) PlayerNameColorEnabled() bool {
	return s.playerNameColorEnabled.Load()
}

func (s *Server) SetTranslatedConnLog(enabled bool) {
	s.translatedConnLog.Store(enabled)
}

func (s *Server) TranslatedConnLogEnabled() bool {
	return s.translatedConnLog.Load()
}

func (s *Server) SetPublicConnIDFormatter(fn func(*Conn) string) {
	s.publicConnIDFormatter = fn
}

func (s *Server) SetJoinLeaveChatEnabled(enabled bool) {
	s.joinLeaveChatEnabled.Store(enabled)
}

func (s *Server) SetPlayerDisplayFormatter(fn func(*Conn) string) {
	s.playerDisplayFormatter = fn
}

func (s *Server) BroadcastInfoPopup(message string, duration float32, align, top, left, bottom, right int32) {
	if s == nil {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	peers := make([]*Conn, 0, len(s.conns))
	s.mu.Lock()
	for c := range s.conns {
		if c != nil && c.hasConnected {
			peers = append(peers, c)
		}
	}
	s.mu.Unlock()
	msg := message
	packet := &protocol.Remote_Menus_infoPopup_118{
		Message:  msg,
		Duration: duration,
		Align:    align,
		Top:      top,
		Left:     left,
		Bottom:   bottom,
		Right:    right,
	}
	for _, peer := range peers {
		_ = peer.SendAsync(packet)
	}
}

func (s *Server) SendInfoPopup(c *Conn, message string, duration float32, align, top, left, bottom, right int32) {
	if s == nil || c == nil || !c.hasConnected {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	msg := message
	packet := &protocol.Remote_Menus_infoPopup_118{
		Message:  msg,
		Duration: duration,
		Align:    align,
		Top:      top,
		Left:     left,
		Bottom:   bottom,
		Right:    right,
	}
	_ = c.SendAsync(packet)
}

func (s *Server) SendInfoMessage(c *Conn, message string) {
	if s == nil || c == nil || !c.hasConnected {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	_ = c.SendAsync(&protocol.Remote_Menus_infoMessage_117{Message: message})
}

func (s *Server) SendMenu(c *Conn, menuID int32, title, message string, options [][]string) {
	if s == nil || c == nil || !c.hasConnected {
		return
	}
	cleaned := make([][]string, 0, len(options))
	for _, row := range options {
		rowOut := make([]string, 0, len(row))
		for _, option := range row {
			rowOut = append(rowOut, strings.TrimSpace(option))
		}
		cleaned = append(cleaned, rowOut)
	}
	_ = c.SendAsync(&protocol.Remote_Menus_menu_106{
		MenuId:  menuID,
		Title:   strings.TrimSpace(title),
		Message: strings.TrimSpace(message),
		Options: cleaned,
	})
}

func (s *Server) SendOpenURI(c *Conn, uri string) {
	if s == nil || c == nil || !c.hasConnected {
		return
	}
	uri = strings.TrimSpace(uri)
	if uri == "" {
		return
	}
	_ = c.SendAsync(&protocol.Remote_Menus_openURI_128{Uri: uri})
}

func (s *Server) BroadcastSetHudTextReliable(message string) {
	if s == nil {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	peers := make([]*Conn, 0, len(s.conns))
	s.mu.Lock()
	for c := range s.conns {
		if c != nil && c.hasConnected {
			peers = append(peers, c)
		}
	}
	s.mu.Unlock()
	packet := &protocol.Remote_Menus_setHudTextReliable_115{Message: message}
	for _, peer := range peers {
		_ = peer.SendAsync(packet)
	}
}

func (s *Server) SendSetHudTextReliable(c *Conn, message string) {
	if s == nil || c == nil || !c.hasConnected {
		return
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return
	}
	_ = c.SendAsync(&protocol.Remote_Menus_setHudTextReliable_115{Message: message})
}

func (s *Server) BroadcastHideHudText() {
	if s == nil {
		return
	}
	peers := make([]*Conn, 0, len(s.conns))
	s.mu.Lock()
	for c := range s.conns {
		if c != nil && c.hasConnected {
			peers = append(peers, c)
		}
	}
	s.mu.Unlock()
	packet := &protocol.Remote_Menus_hideHudText_114{}
	for _, peer := range peers {
		_ = peer.SendAsync(packet)
	}
}

func (s *Server) SendHideHudText(c *Conn) {
	if s == nil || c == nil || !c.hasConnected {
		return
	}
	_ = c.SendAsync(&protocol.Remote_Menus_hideHudText_114{})
}

func (s *Server) playerDisplayName(c *Conn) string {
	if c == nil {
		return "未知玩家"
	}
	if s != nil && s.playerDisplayFormatter != nil {
		if name := strings.TrimSpace(s.playerDisplayFormatter(c)); name != "" {
			return name
		}
	}
	name := strings.TrimSpace(c.name)
	if name == "" {
		name = strings.TrimSpace(c.rawName)
	}
	if name == "" {
		return "未知玩家"
	}
	return name
}

func (s *Server) PlayerDisplayName(c *Conn) string {
	return s.playerDisplayName(c)
}

func (s *Server) refreshPlayerDisplayName(c *Conn) {
	if s == nil || c == nil {
		return
	}
	c.name = s.playerDisplayName(c)
}

func (s *Server) RefreshPlayerDisplayNames() {
	if s == nil {
		return
	}
	s.mu.Lock()
	peers := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		peers = append(peers, c)
	}
	s.mu.Unlock()
	for _, c := range peers {
		s.refreshPlayerDisplayName(c)
	}
}

func (s *Server) ListConnectedConns() []*Conn {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		if c != nil && c.hasConnected {
			out = append(out, c)
		}
	}
	return out
}

func cloneBuildPlans(plans []*protocol.BuildPlan) []*protocol.BuildPlan {
	if len(plans) == 0 {
		return nil
	}
	out := make([]*protocol.BuildPlan, 0, len(plans))
	for _, plan := range plans {
		if plan == nil {
			out = append(out, nil)
			continue
		}
		copyPlan := *plan
		if copyPlan.Block != nil {
			copyPlan.Block = protocol.BlockRef{BlkID: copyPlan.Block.ID(), BlkName: copyPlan.Block.Name()}
		}
		if clonedConfig, err := protocol.CloneObjectValue(copyPlan.Config); err == nil {
			copyPlan.Config = clonedConfig
		} else {
			copyPlan.Config = nil
		}
		out = append(out, &copyPlan)
	}
	return out
}

func (s *Server) recordClientPlanPreview(sender *Conn, groupID int32, plans []*protocol.BuildPlan) {
	if s == nil || sender == nil || sender.playerID == 0 {
		return
	}
	cloned := cloneBuildPlans(plans)

	s.previewMu.Lock()
	defer s.previewMu.Unlock()

	state := s.previewPlans[sender.playerID]
	if state == nil {
		state = &previewPlanState{}
		s.previewPlans[sender.playerID] = state
	}
	state.teamID = sender.TeamID()

	switch {
	case groupID > state.lastRecvGroup:
		state.assembling = state.assembling[:0]
		state.lastRecvGroup = groupID
		state.receiving = true
		state.lastRecvAt = time.Now()
	case groupID < state.lastRecvGroup:
		return
	case !state.receiving:
		return
	default:
		state.lastRecvAt = time.Now()
	}

	if len(cloned) == 0 {
		return
	}
	remaining := maxPlayerPreviewPlans - len(state.assembling)
	if remaining <= 0 {
		return
	}
	if len(cloned) > remaining {
		cloned = cloned[:remaining]
	}
	state.assembling = append(state.assembling, cloned...)
}

func (s *Server) commitClientPlanPreviewsLocked(now time.Time) {
	for _, state := range s.previewPlans {
		if state == nil || !state.receiving {
			continue
		}
		if now.Sub(state.lastRecvAt) < previewPlanCommitDelay {
			continue
		}
		state.receiving = false
		state.current = cloneBuildPlans(state.assembling)
		state.assembling = state.assembling[:0]
	}
}

func (s *Server) BroadcastStoredClientPlanPreviews() {
	s.BroadcastStoredClientPlanPreviewsAt(time.Now())
}

func (s *Server) BroadcastStoredClientPlanPreviewsAt(now time.Time) {
	if s == nil {
		return
	}

	type previewBatch struct {
		playerID int32
		teamID   byte
		groupID  int32
		plans    []*protocol.BuildPlan
	}

	s.previewMu.Lock()
	s.commitClientPlanPreviewsLocked(now)
	batches := make([]previewBatch, 0, len(s.previewPlans))
	for playerID, state := range s.previewPlans {
		if state == nil {
			continue
		}
		state.nextSendGroup++
		batches = append(batches, previewBatch{
			playerID: playerID,
			teamID:   state.teamID,
			groupID:  state.nextSendGroup,
			plans:    cloneBuildPlans(state.current),
		})
	}
	s.previewMu.Unlock()

	for _, batch := range batches {
		s.broadcastClientPlanPreviewPackets(batch.playerID, batch.teamID, batch.groupID, batch.plans)
	}
}

func (s *Server) broadcastClientPlanPreviewPackets(senderPlayerID int32, teamID byte, groupID int32, plans []*protocol.BuildPlan) {
	if s == nil || senderPlayerID == 0 {
		return
	}
	peers := s.ListConnectedConns()
	sendPacket := func(chunk []*protocol.BuildPlan) {
		for _, peer := range peers {
			if peer == nil || peer.playerID == 0 || peer.playerID == senderPlayerID {
				continue
			}
			if teamID != 0 && peer.TeamID() != teamID {
				continue
			}
			packet := &protocol.Remote_NetServer_clientPlanSnapshotReceived_47{
				Player:  &protocol.EntityBox{IDValue: senderPlayerID},
				GroupId: groupID,
				Plans:   cloneBuildPlans(chunk),
			}
			_ = peer.SendAsync(packet)
		}
	}

	if len(plans) == 0 {
		sendPacket(nil)
		return
	}
	for i := 0; i < len(plans); i += previewPlanChunkSize {
		end := i + previewPlanChunkSize
		if end > len(plans) {
			end = len(plans)
		}
		sendPacket(plans[i:end])
	}
}

func normalizePingLocationText(text any) any {
	switch v := text.(type) {
	case nil:
		return nil
	case string:
		return v
	default:
		return nil
	}
}

func (s *Server) broadcastPingLocation(sender *Conn, x, y float32, text any) {
	if s == nil || sender == nil || sender.playerID == 0 {
		return
	}
	peers := s.ListConnectedConns()
	for _, peer := range peers {
		if peer == nil || peer.playerID == 0 {
			continue
		}
		packet := &protocol.Remote_InputHandler_pingLocation_73{
			Player: &protocol.EntityBox{IDValue: sender.playerID},
			X:      x,
			Y:      y,
			Text:   normalizePingLocationText(text),
		}
		_ = peer.SendAsync(packet)
	}
}

func (s *Server) publicConnField(c *Conn) string {
	if s != nil && s.publicConnIDFormatter != nil {
		if id := strings.TrimSpace(s.publicConnIDFormatter(c)); id != "" {
			return fmt.Sprintf("conn_uuid=%s", id)
		}
	}
	if c == nil {
		return "connID=0"
	}
	return fmt.Sprintf("connID=%d", c.id)
}

func (s *Server) verbosef(format string, args ...any) {
	if s.verboseNetLog.Load() {
		fmt.Printf(format, args...)
	}
}

func (s *Server) shouldLogRepeatingNetEvent(key string, every time.Duration) bool {
	if s == nil || every <= 0 {
		return true
	}
	now := time.Now()
	s.logMu.Lock()
	defer s.logMu.Unlock()
	if last, ok := s.logTimes[key]; ok && now.Sub(last) < every {
		return false
	}
	s.logTimes[key] = now
	return true
}

func (s *Server) shouldLogClientDeadIgnored(c *Conn, now time.Time) bool {
	if s == nil || c == nil {
		return false
	}
	if !s.respawnPacketLogsEnabled.Load() {
		return false
	}
	if c.lastDeadIgnoreLogAt.IsZero() || now.Sub(c.lastDeadIgnoreLogAt) >= 2*time.Second {
		c.lastDeadIgnoreLogAt = now
		return true
	}
	return false
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

func (s *Server) SetClientSnapshotConnectFallbackEnabled(enabled bool) {
	s.clientSnapshotConfirm.Store(enabled)
}

func (s *Server) ClientSnapshotConnectFallbackEnabled() bool {
	if s == nil {
		return false
	}
	return s.clientSnapshotConfirm.Load()
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

func (s *Server) syncInterval() time.Duration {
	if s == nil {
		return 200 * time.Millisecond
	}
	entity := time.Duration(s.entitySnapshotIntervalNs.Load())
	state := time.Duration(s.stateSnapshotIntervalNs.Load())
	if entity <= 0 {
		entity = 200 * time.Millisecond
	}
	if state <= 0 {
		state = 200 * time.Millisecond
	}
	if state < entity {
		return state
	}
	return entity
}

func snapshotPollInterval(syncInterval time.Duration) time.Duration {
	if syncInterval <= 20*time.Millisecond {
		return 20 * time.Millisecond
	}
	if syncInterval < 100*time.Millisecond {
		return syncInterval
	}
	return 20 * time.Millisecond
}

// KillSelfUnit clears current controlled unit for a player connection.
func (s *Server) KillSelfUnit(c *Conn) bool {
	if c == nil || c.playerID == 0 {
		return false
	}
	s.markDead(c, "kill-self")
	player := &protocol.EntityBox{IDValue: c.playerID}
	_ = c.SendAsync(&protocol.Remote_InputHandler_unitClear_95{Player: player})
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
	s.tcpLn = ln
	defer func() {
		_ = ln.Close()
		s.tcpLn = nil
	}()

	udpAddr := &net.UDPAddr{IP: addr.IP, Port: addr.Port}
	udpConn, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		return err
	}
	defer udpConn.Close()
	s.udpConn = udpConn
	defer func() {
		s.udpConn = nil
	}()
	go s.serveUDP(udpConn)

	for {
		c, err := ln.AcceptTCP()
		if err != nil {
			if s.shuttingDown.Load() && isListenerClosed(err) {
				return nil
			}
			return err
		}
		conn := NewConn(c, s.Serial)
		conn.id = s.nextID()
		conn.onSend = func(obj any, packetID int, frameworkID int, size int) {
			if s.OnTracePacket != nil {
				s.OnTracePacket("send", conn, obj, packetID, frameworkID, size)
			}
			if s.shouldSuppressHotNetEvent("packet_send") {
				return
			}
			s.emitEvent(conn, "packet_send", packetTypeName(obj), packetSendDetail(size, packetID, frameworkID))
		}
		s.addConn(conn)
		s.addPending(conn)
		if s.OnConnOpen != nil {
			s.OnConnOpen(conn)
		}

		// 记录详细连接日志
		if s.DevLogger != nil {
			s.DevLogger.LogConnection("tcp accepted", conn.id, c.RemoteAddr().String(), "unknown", "")
		} else {
			s.verbosef("[net] tcp accepted remote=%s id=%d\n", c.RemoteAddr().String(), conn.id)
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
		s.clearConnControlledBuild(c)
		if c.hasBegunConnecting && !c.hasConnected {
			fmt.Printf("[net] connect aborted before confirm id=%d player=%d remote=%s name=%q uuid=%s live_worldstream=%v last_packet_id=%d last_framework_id=%d\n",
				c.id, c.playerID, c.RemoteAddr().String(), c.name, c.uuid, c.UsesLiveWorldStream(), c.lastRecvPacketID, c.lastRecvFrameworkID)
			s.emitEvent(c, "connect_aborted_pre_confirm", "", fmt.Sprintf("live_worldstream=%v last_packet_id=%d last_framework_id=%d", c.UsesLiveWorldStream(), c.lastRecvPacketID, c.lastRecvFrameworkID))
		}
		if c.hasConnected && c.playerID != 0 {
			s.broadcastPlayerDisconnect(c.playerID, c)
		}
		if c.hasConnected {
			s.broadcastJoinLeaveChat(c, false)
		}
		s.logPlayerLeaveCN(c)
		if s.OnConnClose != nil {
			s.OnConnClose(c)
		}
		c.Close()
		s.removeConn(c)
	}()

	for {
		obj, err := c.ReadObject()
		if s.OnPacketDecoded != nil && s.OnPacketDecoded(c, obj, err) {
			if err != nil {
				return
			}
			continue
		}
		if err != nil {
			if errors.Is(err, io.EOF) || isConnReadClosed(err) {
				s.verbosef("[net] tcp closed id=%d remote=%s\n", c.id, c.RemoteAddr().String())
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

func (s *Server) logPlayerJoinCN(c *Conn) {
	if c == nil || !s.translatedConnLog.Load() || !s.terminalPlayerLogsEnabled.Load() {
		return
	}
	ip, port := c.remoteEndpoint()
	name := s.playerDisplayName(c)
	if s.playerNameColorEnabled.Load() {
		name = RenderMindustryTextForTerminal(name)
	} else {
		name = StripMindustryColorTags(name)
	}
	if s.terminalPlayerUUIDEnabled.Load() {
		fmt.Printf("[终端] 玩家进入了游戏: 名称=%s UUID=%s %s 登录IP=%s 远程端口=%s\n",
			name, c.uuid, s.publicConnField(c), ip, port)
		return
	}
	fmt.Printf("[终端] 玩家进入了游戏: 名称=%s %s 登录IP=%s 远程端口=%s\n",
		name, s.publicConnField(c), ip, port)
}

func (s *Server) logPlayerLeaveCN(c *Conn) {
	if c == nil || !s.translatedConnLog.Load() || !s.terminalPlayerLogsEnabled.Load() || !c.hasBegunConnecting {
		return
	}
	ip, port := c.remoteEndpoint()
	name := s.playerDisplayName(c)
	if s.playerNameColorEnabled.Load() {
		name = RenderMindustryTextForTerminal(name)
	} else {
		name = StripMindustryColorTags(name)
	}
	if s.terminalPlayerUUIDEnabled.Load() {
		fmt.Printf("[终端] 玩家退出了游戏: 名称=%s UUID=%s %s 登录IP=%s 远程端口=%s\n",
			name, c.uuid, s.publicConnField(c), ip, port)
		return
	}
	fmt.Printf("[终端] 玩家退出了游戏: 名称=%s %s 登录IP=%s 远程端口=%s\n",
		name, s.publicConnField(c), ip, port)
}

var mindustryColorTagRE = regexp.MustCompile(`\[[^\]]*\]`)

func StripMindustryColorTags(name string) string {
	name = mindustryColorTagRE.ReplaceAllString(name, "")
	name = strings.TrimSpace(name)
	return name
}

func FixMindustryPlayerName(name string) string {
	name = strings.TrimSpace(strings.ReplaceAll(strings.ReplaceAll(name, "\n", ""), "\t", ""))
	if name == "[" || name == "]" {
		return ""
	}

	for i := 0; i < len(name); i++ {
		if name[i] != '[' {
			continue
		}
		if i == len(name)-1 || name[i+1] == '[' || (i > 0 && name[i-1] == '[') {
			continue
		}
		prev := name[:i]
		next := name[i:]
		name = prev + stripTransparentMindustryColor(next)
	}

	var b strings.Builder
	for _, r := range name {
		width := utf8.RuneLen(r)
		if width < 0 {
			width = len(string(r))
		}
		if b.Len()+width > maxMindustryPlayerNameBytes {
			break
		}
		b.WriteRune(r)
	}
	return b.String()
}

func stripTransparentMindustryColor(s string) string {
	if !strings.HasPrefix(s, "[") {
		return s
	}
	for i := 1; i < len(s); i++ {
		if s[i] != ']' {
			continue
		}
		tag := s[1:i]
		if isTransparentMindustryColor(tag) {
			return s[i+1:]
		}
		return s
	}
	return s
}

func isTransparentMindustryColor(tag string) bool {
	if strings.EqualFold(strings.TrimSpace(tag), "clear") {
		return true
	}
	tag = strings.TrimSpace(tag)
	if len(tag) == 0 {
		return false
	}
	if strings.HasPrefix(tag, "#") {
		tag = tag[1:]
	}
	if len(tag) != 8 {
		return false
	}
	raw, err := hex.DecodeString(tag)
	if err != nil || len(raw) != 4 {
		return false
	}
	return raw[3] < 0xFF
}

func RenderMindustryTextForTerminal(text string) string {
	if strings.TrimSpace(text) == "" {
		return ""
	}
	const ansiReset = "\x1b[0m"
	var b strings.Builder
	colorOpen := false
	for i := 0; i < len(text); {
		if text[i] == '[' {
			if end := strings.IndexByte(text[i:], ']'); end >= 0 {
				tag := text[i+1 : i+end]
				switch {
				case tag == "":
					if colorOpen {
						b.WriteString(ansiReset)
						colorOpen = false
					}
				case strings.HasPrefix(tag, "#"):
					if r, g, bl, ok := parseMindustryHexColor(tag[1:]); ok {
						fmt.Fprintf(&b, "\x1b[38;2;%d;%d;%dm", r, g, bl)
						colorOpen = true
					}
				}
				i += end + 1
				continue
			}
		}
		b.WriteByte(text[i])
		i++
	}
	if colorOpen {
		b.WriteString(ansiReset)
	}
	return b.String()
}

func parseMindustryHexColor(s string) (int, int, int, bool) {
	if len(s) != 6 && len(s) != 8 {
		return 0, 0, 0, false
	}
	if len(s) == 8 {
		s = s[:6]
	}
	raw, err := hex.DecodeString(s)
	if err != nil || len(raw) != 3 {
		return 0, 0, 0, false
	}
	return int(raw[0]), int(raw[1]), int(raw[2]), true
}

func displayPlainPlayerName(name string) string {
	return StripMindustryColorTags(name)
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

func isListenerClosed(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "use of closed network connection") ||
		strings.Contains(msg, "operation on non-socket")
}

func beginBreakPlan(x, y int32) *protocol.BuildPlan {
	return &protocol.BuildPlan{
		Breaking: true,
		X:        x,
		Y:        y,
	}
}

func beginPlacePlan(req BeginPlaceRequest) *protocol.BuildPlan {
	return &protocol.BuildPlan{
		Breaking: false,
		X:        req.X,
		Y:        req.Y,
		Rotation: byte(req.Rotation) & 0x03,
		Block:    protocol.BlockRef{BlkID: req.BlockID, BlkName: ""},
		Config:   req.Config,
	}
}

func beginPlaceRequestFromPlan(plan *protocol.BuildPlan) (BeginPlaceRequest, bool) {
	if plan == nil || plan.Breaking || plan.Block == nil {
		return BeginPlaceRequest{}, false
	}
	return BeginPlaceRequest{
		X:        plan.X,
		Y:        plan.Y,
		BlockID:  plan.Block.ID(),
		Rotation: int8(plan.Rotation & 0x03),
		Config:   plan.Config,
	}, true
}

func (s *Server) handleOfficialBeginBreak(c *Conn, x, y int32) {
	if s.OnBeginBreak != nil {
		s.OnBeginBreak(c, x, y)
		return
	}
	if s.OnBuildPlans != nil {
		s.OnBuildPlans(c, []*protocol.BuildPlan{beginBreakPlan(x, y)})
	}
}

func (s *Server) handleOfficialBeginPlace(c *Conn, req BeginPlaceRequest) {
	if req.BlockID <= 0 {
		return
	}
	if s.OnBeginPlace != nil {
		s.OnBeginPlace(c, req)
		return
	}
	if s.OnBuildPlans != nil {
		s.OnBuildPlans(c, []*protocol.BuildPlan{beginPlacePlan(req)})
	}
}

func (s *Server) handlePacket(c *Conn, obj any, fromTCP bool) {
	if s != nil && s.OnTracePacket != nil {
		packetID := -1
		frameworkID := -1
		if c != nil {
			packetID = c.lastRecvPacketID
			frameworkID = c.lastRecvFrameworkID
		}
		s.OnTracePacket("recv", c, obj, packetID, frameworkID, 0)
	}
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
	case *protocol.ConnectPacket:
		s.handleConnectPacket(c, v)
	case *protocol.Remote_NetServer_connectConfirm_50:
		s.handleOfficialConnectConfirm(c, v)
	case *protocol.Remote_NetServer_clientSnapshot_48:
		if s.ClientSnapshotConnectFallbackEnabled() {
			s.handleClientSnapshotConnectFallback(c, v)
		}
		if !c.hasConnected {
			s.emitEvent(c, "client_snapshot_ignored_pre_confirm", fmt.Sprintf("%T", v), "waiting_for_connect_confirm")
			return
		}
		// Drop out-of-order snapshots.
		if last := c.lastClientSnapshot.Load(); last >= 0 && v.SnapshotID < last {
			return
		}
		c.lastClientSnapshot.Store(v.SnapshotID)

		nowMs := time.Now().UnixMilli()
		elapsed := int64(16)
		if prev := c.lastClientTimeMs.Load(); prev > 0 {
			elapsed = nowMs - prev
			if elapsed < 0 {
				elapsed = 0
			}
			if elapsed > 1500 {
				elapsed = 1500
			}
		}
		c.lastClientTimeMs.Store(nowMs)

		// Sanitize floats (avoid NaN/Inf poisoning).
		safeF := func(f float32) float32 {
			if math.IsNaN(float64(f)) || math.IsInf(float64(f), 0) {
				return 0
			}
			return f
		}
		v.X = safeF(v.X)
		v.Y = safeF(v.Y)
		v.PointerX = safeF(v.PointerX)
		v.PointerY = safeF(v.PointerY)
		v.Rotation = safeF(v.Rotation)
		v.BaseRotation = safeF(v.BaseRotation)
		v.XVelocity = safeF(v.XVelocity)
		v.YVelocity = safeF(v.YVelocity)
		v.ViewX = safeF(v.ViewX)
		v.ViewY = safeF(v.ViewY)
		v.ViewWidth = safeF(v.ViewWidth)
		v.ViewHeight = safeF(v.ViewHeight)

		c.viewX = v.ViewX
		c.viewY = v.ViewY
		c.viewWidth = v.ViewWidth
		c.viewHeight = v.ViewHeight

		c.pointerX = v.PointerX
		c.pointerY = v.PointerY
		c.shooting = v.Shooting
		c.boosting = v.Boosting
		c.typing = v.Chatting
		if !v.Dead {
			c.clientDeadIgnores = 0
			c.lastDeadIgnoreAt = time.Time{}
		}
		prevUnitID := c.unitID
		if v.Dead && !c.InWorldReloadGrace() {
			now := time.Now()
			freshSpawnGrace := s.connHasFreshSpawnBinding(c, now, 2*time.Second)
			recentRespawnGrace := s.connHasRecentRespawnWindow(c, now, 2*time.Second)
			skipDeadRepair := false
			// Only honor client-dead when server agrees the unit is missing or dead.
			shouldDead := c.unitID == 0 && !c.controlBuildActive
			debugInfo := "unit=0"
			if c.controlBuildActive {
				if info, ok := s.currentControlledBuildInfo(c); ok {
					debugInfo = fmt.Sprintf("build=%d pos=(%.1f,%.1f) team=%d", info.Pos, info.X, info.Y, info.TeamID)
				} else {
					debugInfo = "build=missing"
					shouldDead = true
				}
			} else if shouldDead && s.connHasRecentRespawnWindow(c, now, 2*time.Second) {
				debugInfo = fmt.Sprintf("unit=0 respawn_age=%s", now.Sub(recentNonZeroTime(c.lastSpawnAt, c.lastRespawnReq)).Round(10*time.Millisecond))
				shouldDead = false
				skipDeadRepair = true
			} else if !shouldDead {
				if s.UnitInfoFn != nil {
					info, ok := s.UnitInfoFn(c.unitID)
					if !ok {
						if freshSpawnGrace || recentRespawnGrace {
							debugInfo = fmt.Sprintf("unit=%d world=missing respawn_age=%s", c.unitID, now.Sub(recentNonZeroTime(c.lastSpawnAt, c.lastRespawnReq)).Round(10*time.Millisecond))
							shouldDead = false
							skipDeadRepair = true
						} else {
							debugInfo = fmt.Sprintf("unit=%d world=missing", c.unitID)
							shouldDead = true
						}
					} else {
						debugInfo = fmt.Sprintf("unit=%d world_hp=%.2f team=%d type=%d", c.unitID, info.Health, info.TeamID, info.TypeID)
						if info.Health <= 0 {
							shouldDead = true
						} else if recentRespawnGrace {
							debugInfo = fmt.Sprintf("unit=%d world_hp=%.2f team=%d type=%d respawn_age=%s",
								c.unitID, info.Health, info.TeamID, info.TypeID,
								now.Sub(recentNonZeroTime(c.lastSpawnAt, c.lastRespawnReq)).Round(10*time.Millisecond))
							shouldDead = false
							skipDeadRepair = true
						}
					}
				} else {
					s.entityMu.Lock()
					_, ok := s.entities[c.unitID]
					s.entityMu.Unlock()
					debugInfo = fmt.Sprintf("unit=%d mirror_exists=%v", c.unitID, ok)
					if !ok {
						shouldDead = true
					}
				}
			}
			if shouldDead {
				if c.dead && c.unitID == 0 && !c.controlBuildActive {
					c.clientDeadIgnores = 0
					c.lastDeadIgnoreAt = time.Time{}
				} else {
					fmt.Printf("[net] client dead accepted conn=%d player=%d %s\n", c.id, c.playerID, debugInfo)
					c.clientDeadIgnores = 0
					c.lastDeadIgnoreAt = time.Time{}
					s.markDead(c, "client-dead")
				}
			} else {
				if s.shouldLogClientDeadIgnored(c, now) {
					fmt.Printf("[net] client dead ignored conn=%d player=%d %s\n", c.id, c.playerID, debugInfo)
				}
				if skipDeadRepair {
					c.clientDeadIgnores = 0
					c.lastDeadIgnoreAt = time.Time{}
				} else if s.shouldForceRespawnAfterDeadIgnored(c, now) {
					fmt.Printf("[net] client dead stuck conn=%d player=%d ignores=%d action=repair-alive-once\n",
						c.id, c.playerID, c.clientDeadIgnores)
					s.repairClientDeadStuckBinding(c)
				} else {
					s.repairClientDeadAliveBinding(c)
				}
			}
		}
		if c.controlBuildActive {
			if info, ok := s.currentControlledBuildInfo(c); ok {
				c.snapX = info.X
				c.snapY = info.Y
				if info.TeamID != 0 {
					c.teamID = info.TeamID
				}
			} else {
				s.markDead(c, "controlled-build-missing")
			}
		} else if v.UnitID != 0 {
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
			s.detachConnUnit(c, prevUnitID)
		}

		// Apply motion/position into authoritative world state when possible.
		ignorePosition := c.controlBuildActive || v.Dead || c.unitID == 0 || (v.UnitID != 0 && v.UnitID != c.unitID)
		if !ignorePosition {
			// If the client jumps too far away, correct them back to server position.
			// Vanilla uses tilesize*14. tilesize=8, so correctDist=112.
			const correctDist = 112.0
			var curX, curY float32
			var hasCur bool
			if s.UnitInfoFn != nil {
				if info, ok := s.UnitInfoFn(c.unitID); ok {
					curX, curY, hasCur = info.X, info.Y, true
				}
			}
			if !hasCur {
				// Fallback to last known snapshot position.
				curX, curY, hasCur = c.snapX, c.snapY, true
			}
			dx := float64(v.X - curX)
			dy := float64(v.Y - curY)
			dist := math.Hypot(dx, dy)
			if dist > correctDist {
				_ = c.SendAsync(&protocol.Remote_NetClient_setPosition_29{X: curX, Y: curY})
				// Keep server-side snap in sync with authoritative position.
				c.snapX, c.snapY = curX, curY
			} else {
				// Non-strict mode: accept client-reported state and apply to world.
				c.snapX = v.X
				c.snapY = v.Y
				if s.SetUnitMotionFn != nil {
					_ = s.SetUnitMotionFn(c.unitID, v.XVelocity, v.YVelocity, 0)
				}
				if s.SetUnitPositionFn != nil {
					_ = s.SetUnitPositionFn(c.unitID, v.X, v.Y, v.Rotation)
				}
			}
		} else if !c.controlBuildActive {
			// When dead or mismatched, keep player coords updated but do not move unit.
			c.snapX = v.X
			c.snapY = v.Y
		}

		c.building = v.Building
		c.selectedRotation = v.SelectedRotation
		c.selectedBlockID = -1
		if v.SelectedBlock != nil {
			c.selectedBlockID = v.SelectedBlock.ID()
		}
		c.miningTilePos = invalidTilePos
		if v.Mining != nil {
			c.miningTilePos = v.Mining.Pos()
		}
		plans := extractBuildPlans(v.Plans)
		if c.controlBuildActive {
			if s.SetControlledBuildInputFn != nil {
				_ = s.SetControlledBuildInputFn(c.playerID, c.controlBuildPos, c.pointerX, c.pointerY, c.shooting)
			}
		} else if c.unitID != 0 && s.SetUnitRuntimeStateFn != nil {
			_ = s.SetUnitRuntimeStateFn(c.unitID, UnitRuntimeState{
				Shooting:       c.shooting,
				Boosting:       c.boosting,
				UpdateBuilding: c.building,
				MineTilePos:    c.miningTilePos,
				Plans:          plans,
			})
		}
		// Keep sync entities updated for snapshot stream.
		if p := s.ensurePlayerEntity(c); p != nil {
			s.updatePlayerEntity(p, c)
		}
		if c.unitID != 0 {
			if u := s.ensurePlayerUnitEntity(c); u != nil && s.UnitSyncFn == nil {
				u.Shooting = c.shooting
				u.MineTile = v.Mining
				u.UpdateBuilding = c.building
				u.Rotation = v.Rotation
				u.Vel = protocol.Vec2{X: v.XVelocity, Y: v.YVelocity}
				// X/Y will be refreshed from world by syncUnitFromWorld if UnitInfoFn is set.
				u.X = c.snapX
				u.Y = c.snapY
			}
		}
		if s.OnBuildPlanSnapshot != nil {
			s.OnBuildPlanSnapshot(c, plans)
		} else if s.OnBuildPlanPreview != nil {
			s.OnBuildPlanPreview(c, plans)
		} else if len(plans) > 0 && s.OnBuildPlans != nil {
			s.OnBuildPlans(c, plans)
		}
	case *protocol.Remote_NetServer_clientPlanSnapshot_46:
		plans := extractBuildPlans(v.Plans)
		s.recordClientPlanPreview(c, v.GroupId, plans)
		if s.OnBuildPlanPreview != nil {
			s.OnBuildPlanPreview(c, plans)
		} else if len(plans) > 0 && s.OnBuildPlanSnapshot != nil {
			s.OnBuildPlanSnapshot(c, plans)
		} else if len(plans) > 0 && s.OnBuildPlans != nil {
			s.OnBuildPlans(c, plans)
		}
	case *protocol.Remote_InputHandler_pingLocation_73:
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 73, "Remote_InputHandler_pingLocation_73", "ping_location")
		}
		s.broadcastPingLocation(c, v.X, v.Y, v.Text)
	case *protocol.Remote_NetClient_ping_18:
		// The framework keepalive loop is authoritative for connection liveness.
		_ = v
	case *protocol.Remote_NetClient_sendChatMessage_16:
		msg := sanitizeChatMessage(v.Message)
		if msg == "" {
			return
		}
		if s.OnChat != nil && s.OnChat(c, msg) {
			return
		}
		s.broadcastPlayerChat(c, msg)
	case *protocol.Remote_InputHandler_buildingControlSelect_92:
		if s.DevLogger != nil {
			pos := int32(0)
			if v.Build != nil {
				pos = v.Build.Pos()
			}
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 10, "Remote_InputHandler_buildingControlSelect_92", fmt.Sprintf("pos=%d", pos))
		}
		if s.OnBuildingControlSelect != nil && v.Build != nil {
			s.OnBuildingControlSelect(c, v.Build.Pos())
		}
	case *protocol.Remote_Units_unitSpawn_51:
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
	case *protocol.Remote_Units_unitCapDeath_52:
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
	case *protocol.Remote_Units_unitEnvDeath_53:
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
	case *protocol.Remote_Units_unitDeath_54:
		// Handle unit death (delayed)
		unitID := v.Uid
		s.entityMu.Lock()
		if ent := s.entities[unitID]; ent != nil {
			delete(s.entities, unitID)
		}
		s.entityMu.Unlock()
		fmt.Printf("[net] unitDeath id=%d\n", unitID)
	case *protocol.Remote_Units_unitDestroy_55:
		// Handle immediate unit destruction
		unitID := v.Uid
		s.entityMu.Lock()
		if ent := s.entities[unitID]; ent != nil {
			delete(s.entities, unitID)
		}
		s.entityMu.Unlock()
		fmt.Printf("[net] unitDestroy id=%d\n", unitID)
	case *protocol.Remote_Units_unitDespawn_56:
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
	case *protocol.Remote_Units_unitSafeDeath_57:
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
	case *protocol.Remote_BulletType_createBullet_58:
		// Create bullet entity
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 55, "Remote_BulletType_createBullet_58", "create_bullet")
		}
	case *protocol.Remote_Teams_destroyPayload_59:
		// Destroy payload on team entities
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 56, "Remote_Teams_destroyPayload_59", "destroy_payload")
		}
	case *protocol.Remote_InputHandler_transferItemEffect_60:
		// 物品传输效果
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 57, "Remote_InputHandler_transferItemEffect_60", "transfer_item_effect")
		}
	case *protocol.Remote_InputHandler_takeItems_61:
		// 抽取物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 58, "Remote_InputHandler_takeItems_61", "take_items")
		}
	case *protocol.Remote_InputHandler_transferItemToUnit_62:
		// 传输物品到单位
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 59, "Remote_InputHandler_transferItemToUnit_62", "transfer_item_to_unit")
		}
	case *protocol.Remote_InputHandler_setItem_63:
		// 设置单个物品槽位
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 60, "Remote_InputHandler_setItem_63", "set_item")
		}
	case *protocol.Remote_InputHandler_setItems_64:
		// 批量设置物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 61, "Remote_InputHandler_setItems_64", "set_items")
		}
	case *protocol.Remote_InputHandler_setTileItems_65:
		// 设置地砖物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 62, "Remote_InputHandler_setTileItems_65", "set_tile_items")
		}
	case *protocol.Remote_InputHandler_clearItems_66:
		// 清空物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 63, "Remote_InputHandler_clearItems_66", "clear_items")
		}
	case *protocol.Remote_InputHandler_setLiquid_67:
		// 设置单个液体槽位
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 64, "Remote_InputHandler_setLiquid_67", "set_liquid")
		}
	case *protocol.Remote_InputHandler_setLiquids_68:
		// 批量设置液体
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 65, "Remote_InputHandler_setLiquids_68", "set_liquids")
		}
	case *protocol.Remote_InputHandler_setTileLiquids_69:
		// 设置地砖液体
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 66, "Remote_InputHandler_setTileLiquids_69", "set_tile_liquids")
		}
	case *protocol.Remote_InputHandler_clearLiquids_70:
		// 清空液体
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 67, "Remote_InputHandler_clearLiquids_70", "clear_liquids")
		}
	case *protocol.Remote_InputHandler_transferItemTo_71:
		// 通用物品传输到
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 68, "Remote_InputHandler_transferItemTo_71", "transfer_item_to")
		}
	case *protocol.Remote_InputHandler_deletePlans_72:
		// 删除建造计划
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 69, "Remote_InputHandler_deletePlans_72", "delete_plans")
		}
		if s.OnDeletePlans != nil && len(v.Positions) > 0 {
			s.OnDeletePlans(c, v.Positions)
		}
	case *protocol.Remote_InputHandler_commandUnits_74:
		// 指挥多个单位
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 70, "Remote_InputHandler_commandUnits_74", fmt.Sprintf("command_units(count=%d)", len(v.UnitIds)))
		}
		if s.OnCommandUnits != nil && len(v.UnitIds) > 0 {
			s.OnCommandUnits(c, v.UnitIds, v.BuildTarget, v.UnitTarget, v.PosTarget, v.QueueCommand, v.FinalBatch)
		}
	case *protocol.Remote_InputHandler_setUnitCommand_75:
		// 设置单位命令
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 71, "Remote_InputHandler_setUnitCommand_75", fmt.Sprintf("set_unit_command(count=%d)", len(v.UnitIds)))
		}
		if s.OnSetUnitCommand != nil && len(v.UnitIds) > 0 {
			s.OnSetUnitCommand(c, v.UnitIds, v.Command)
		}
	case *protocol.Remote_InputHandler_setUnitStance_76:
		// 设置单位姿态
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 72, "Remote_InputHandler_setUnitStance_76", fmt.Sprintf("set_unit_stance(count=%d)", len(v.UnitIds)))
		}
		if s.OnSetUnitStance != nil && len(v.UnitIds) > 0 {
			s.OnSetUnitStance(c, v.UnitIds, v.Stance, v.Enable)
		}
	case *protocol.Remote_InputHandler_commandBuilding_77:
		// 命令建筑
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 73, "Remote_InputHandler_commandBuilding_77", fmt.Sprintf("command_building(count=%d)", len(v.Buildings)))
		}
		if s.OnCommandBuilding != nil && len(v.Buildings) > 0 {
			s.OnCommandBuilding(c, v.Buildings, v.Target)
		}
	case *protocol.Remote_InputHandler_requestItem_78:
		// 请求物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 74, "Remote_InputHandler_requestItem_78", "request_item")
		}
		if s.OnRequestItem != nil && v.Build != nil && v.Item != nil {
			s.OnRequestItem(c, v.Build.Pos(), v.Item.ID(), v.Amount)
		}
	case *protocol.Remote_InputHandler_transferInventory_79:
		// 转移库存
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 75, "Remote_InputHandler_transferInventory_79", "transfer_inventory")
		}
		if s.OnTransferInventory != nil && v.Build != nil {
			s.OnTransferInventory(c, v.Build.Pos())
		}
	case *protocol.Remote_InputHandler_removeQueueBlock_80:
		// 从队列中移除方块
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 76, "Remote_InputHandler_removeQueueBlock_80", fmt.Sprintf("remove_queue(x=%d y=%d breaking=%v)", v.X, v.Y, v.Breaking))
		}
		if s.OnRemoveQueueBlock != nil {
			s.OnRemoveQueueBlock(c, v.X, v.Y, v.Breaking)
		}
	case *protocol.Remote_InputHandler_requestUnitPayload_81:
		// 请求单位载荷
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 77, "Remote_InputHandler_requestUnitPayload_81", "request_unit_payload")
		}
		if s.OnRequestUnitPayload != nil && v.Target != nil {
			s.OnRequestUnitPayload(c, v.Target.ID())
		}
	case *protocol.Remote_InputHandler_requestBuildPayload_82:
		// 请求建筑载荷
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 78, "Remote_InputHandler_requestBuildPayload_82", "request_build_payload")
		}
		if s.OnRequestBuildPayload != nil && v.Build != nil {
			s.OnRequestBuildPayload(c, v.Build.Pos())
		}
	case *protocol.Remote_InputHandler_pickedUnitPayload_83:
		// 单位运载选择
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 79, "Remote_InputHandler_pickedUnitPayload_83", "picked_unit_payload")
		}
		if s.OnRequestUnitPayload != nil && v.Target != nil {
			s.OnRequestUnitPayload(c, v.Target.ID())
		}
	case *protocol.Remote_InputHandler_pickedBuildPayload_84:
		// 建筑运载选择
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 80, "Remote_InputHandler_pickedBuildPayload_84", "picked_build_payload")
		}
		if s.OnRequestBuildPayload != nil && v.Build != nil {
			s.OnRequestBuildPayload(c, v.Build.Pos())
		}
	case *protocol.Remote_InputHandler_requestDropPayload_85:
		// 请求丢弃运载物
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 81, "Remote_InputHandler_requestDropPayload_85", fmt.Sprintf("request_drop_payload(x=%.1f y=%.1f)", v.X, v.Y))
		}
		if s.OnRequestDropPayload != nil {
			s.OnRequestDropPayload(c, v.X, v.Y)
		}
	case *protocol.Remote_InputHandler_payloadDropped_86:
		// 运载物已丢弃
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 82, "Remote_InputHandler_payloadDropped_86", fmt.Sprintf("payload_dropped(x=%.1f y=%.1f)", v.X, v.Y))
		}
		if s.OnRequestDropPayload != nil {
			s.OnRequestDropPayload(c, v.X, v.Y)
		}
	case *protocol.Remote_InputHandler_unitEnteredPayload_87:
		// 单位进入运载
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 83, "Remote_InputHandler_unitEnteredPayload_87", "unit_entered_payload")
		}
		if s.OnUnitEnteredPayload != nil && v.Unit != nil && v.Build != nil {
			s.OnUnitEnteredPayload(c, v.Unit.ID(), v.Build.Pos())
		}
	case *protocol.Remote_InputHandler_dropItem_88:
		// 投放物品
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 84, "Remote_InputHandler_dropItem_88", "drop_item")
		}
		if s.OnDropItem != nil {
			s.OnDropItem(c, v.Angle)
		}
	case *protocol.Remote_InputHandler_rotateBlock_89:
		if s.DevLogger != nil {
			pos := int32(0)
			if v.Build != nil {
				pos = v.Build.Pos()
			}
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 83, "Remote_InputHandler_rotateBlock_89", fmt.Sprintf("pos=%d dir=%v", pos, v.Direction))
		}
		if s.OnRotateBlock != nil && v.Build != nil {
			s.OnRotateBlock(c, v.Build.Pos(), v.Direction)
		}
	case *protocol.Remote_NetServer_requestDebugStatus_36:
		resp := &protocol.Remote_NetServer_debugStatusClient_37{
			Value:              0,
			LastClientSnapshot: c.lastClientSnapshot.Load(),
			SnapshotsSent:      c.snapshotsSent.Load(),
		}
		_ = c.Send(resp)
	case *protocol.Remote_NetServer_requestBlockSnapshot_45:
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 45, "Remote_NetServer_requestBlockSnapshot_45", fmt.Sprintf("pos=%d", v.Pos))
		}
		if s.OnRequestBlockSnapshot != nil {
			s.OnRequestBlockSnapshot(c, v.Pos)
		}
	case *protocol.Remote_Build_beginBreak_132:
		//客户端请求开始破坏建筑
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 123, "Remote_Build_beginBreak_132", fmt.Sprintf("x=%d y=%d", v.X, v.Y))
		}
		s.handleOfficialBeginBreak(c, v.X, v.Y)
	case *protocol.Remote_Build_beginPlace_133:
		// 客户端请求开始放置建筑
		blockID := int16(0)
		if v.Result != nil {
			blockID = v.Result.ID()
		}
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 124, "Remote_Build_beginPlace_133", fmt.Sprintf("x=%d y=%d block=%d", v.X, v.Y, blockID))
		}
		s.handleOfficialBeginPlace(c, BeginPlaceRequest{
			X:        v.X,
			Y:        v.Y,
			BlockID:  blockID,
			Rotation: int8(byte(v.Rotation) & 0x03),
			Config:   v.PlaceConfig,
		})
	case *protocol.Remote_InputHandler_tileConfig_90:
		if s.DevLogger != nil {
			pos := int32(0)
			if v.Build != nil {
				pos = v.Build.Pos()
			}
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 127, "Remote_InputHandler_tileConfig_90", fmt.Sprintf("pos=%d value=%T", pos, v.Value))
		}
		if s.OnTileConfig != nil && v.Build != nil {
			s.OnTileConfig(c, v.Build.Pos(), v.Value)
		}
	case *protocol.Remote_Tile_setFloor_137:
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
	case *protocol.Remote_Tile_setOverlay_138:
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
	case *protocol.Remote_Tile_removeTile_139:
		// 移除 Tile
		if s.DevLogger != nil {
			pos := v.Tile.Pos()
			x := int32(protocol.UnpackPoint2X(pos))
			y := int32(protocol.UnpackPoint2Y(pos))
			s.DevLogger.LogBuild(x, y, 0, "none", "removeTile",
				devlog.Int32Fld("tile_x", x),
				devlog.Int32Fld("tile_y", y))
		}
	case *protocol.Remote_Tile_setTile_140:
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
	case *protocol.Remote_Tile_buildDestroyed_143:
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
	case *protocol.Remote_Tile_buildHealthUpdate_144:
		// 建筑生命值更新 - 简化处理（数据包结构需要进一步分析）
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 135, "Remote_Tile_buildHealthUpdate_144", "building_health_update")
		}
	case *protocol.Remote_InputHandler_unitControl_94:
		// 单位控制（需要进一步分析数据包结构）
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 90, "Remote_InputHandler_unitControl_94", "unit_control")
		}
		if c != nil && c.playerID != 0 {
			if buildPos, ok := extractControlledBuildPos(v.Unit); ok {
				s.controlBlockUnit(c, buildPos)
			} else if unitID := extractUnitID(v.Unit); unitID != 0 {
				s.unitControl(c, unitID)
			}
		}
	case *protocol.Remote_InputHandler_unitClear_95:
		// Vanilla InputHandler.unitClear() clears the current unit and immediately
		// requests a new core spawn; it is not a delayed death-timer request.
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 91, "Remote_InputHandler_unitClear_95", "unit_clear")
		}
		s.handleOfficialUnitClear(c)
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
	case *protocol.Remote_LExecutor_setMapArea_96:
		// 逻辑执行器 - 设置地图区域
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 92, "Remote_LExecutor_setMapArea_96", "set_map_area")
		}
	case *protocol.Remote_LExecutor_logicExplosion_97:
		// 逻辑执行器 - 逻辑爆炸效果
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 93, "Remote_LExecutor_logicExplosion_97", "logic_explosion")
		}
	case *protocol.Remote_LExecutor_syncVariable_98:
		// 逻辑执行器 - 同步变量值
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 94, "Remote_LExecutor_syncVariable_98", "sync_variable")
		}
	case *protocol.Remote_LExecutor_setFlag_99:
		// 逻辑执行器 - 设置逻辑标志
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 95, "Remote_LExecutor_setFlag_99", "set_flag")
		}
	case *protocol.Remote_LExecutor_createMarker_100:
		// 逻辑执行器 - 创建标记
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 96, "Remote_LExecutor_createMarker_100", "create_marker")
		}
	case *protocol.Remote_LExecutor_removeMarker_101:
		// 逻辑执行器 - 删除标记
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 97, "Remote_LExecutor_removeMarker_101", "remove_marker")
		}
	case *protocol.Remote_LExecutor_updateMarker_102:
		// 逻辑执行器 - 更新标记 (忽略，服务器不处理逻辑处理器)
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 98, "Remote_LExecutor_updateMarker_102", "update_marker")
		}
	case *protocol.Remote_LExecutor_updateMarkerText_103:
		// 逻辑执行器 - 更新标记文本 (忽略，服务器不处理逻辑处理器)
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 99, "Remote_LExecutor_updateMarkerText_103", "update_marker_text")
		}
	case *protocol.Remote_LExecutor_updateMarkerTexture_104:
		// 逻辑执行器 - 更新标记纹理 (忽略，服务器不处理逻辑处理器)
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 100, "Remote_LExecutor_updateMarkerTexture_104", "update_marker_texture")
		}
	case *protocol.Remote_Weather_createWeather_105:
		// 天气 - 创建天气效果
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 101, "Remote_Weather_createWeather_105", "create_weather")
		}
	case *protocol.Remote_Menus_menu_106:
		// 菜单 - 显示菜单
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 102, "Remote_Menus_menu_106", "menu")
		}
	case *protocol.Remote_Menus_followUpMenu_107:
		// 菜单 - 跟随菜单
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 103, "Remote_Menus_followUpMenu_107", "follow_up_menu")
		}
	case *protocol.Remote_Menus_hideFollowUpMenu_108:
		// 菜单 - 隐藏跟随菜单
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 104, "Remote_Menus_hideFollowUpMenu_108", "hide_follow_up_menu")
		}
	case *protocol.Remote_Menus_menuChoose_109:
		// 菜单 - 菜单选择
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 105, "Remote_Menus_menuChoose_109", "menu_choose")
		}
		if s.OnMenuChoose != nil {
			s.OnMenuChoose(c, v.MenuId, v.Option)
		}
	case *protocol.Remote_Menus_textInput_110:
		// 菜单 - 文本输入 (merged 106/107 -> 110)
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 110, "Remote_Menus_textInput_110", "text_input")
		}
	case *protocol.Remote_Menus_textInputResult_112:
		// 菜单 - 文本输入结果
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 108, "Remote_Menus_textInputResult_112", "text_input_result")
		}
	case *protocol.Remote_Menus_setHudText_113:
		// 菜单 - 设置HUD文本
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 109, "Remote_Menus_setHudText_113", "set_hud_text")
		}
	case *protocol.Remote_Menus_hideHudText_114:
		// 菜单 - 隐藏HUD文本
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 110, "Remote_Menus_hideHudText_114", "hide_hud_text")
		}
	case *protocol.Remote_Menus_setHudTextReliable_115:
		// 菜单 - 设置可靠HUD文本
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 111, "Remote_Menus_setHudTextReliable_115", "set_hud_text_reliable")
		}
	case *protocol.Remote_Menus_announce_116:
		// 菜单 - 公告
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 112, "Remote_Menus_announce_116", "announce")
		}
	case *protocol.Remote_Menus_infoMessage_117:
		// 菜单 - 信息消息
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 113, "Remote_Menus_infoMessage_117", "info_message")
		}
	case *protocol.Remote_Menus_infoPopup_118:
		// 菜单 - 信息弹窗
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 114, "Remote_Menus_infoPopup_118", "info_popup")
		}
	case *protocol.Remote_Menus_label_122:
		// 菜单 - 标签
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 115, "Remote_Menus_label_122", "label")
		}
	case *protocol.Remote_Menus_infoPopupReliable_119:
		// 菜单 - 可靠性信息弹窗
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 116, "Remote_Menus_infoPopupReliable_119", "info_popup_reliable")
		}
	case *protocol.Remote_Menus_labelReliable_123:
		// 菜单 - 可靠性标签
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 117, "Remote_Menus_labelReliable_123", "label_reliable")
		}
	case *protocol.Remote_Menus_infoToast_126:
		// 菜单 - 信息Toast
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 118, "Remote_Menus_infoToast_126", "info_toast")
		}
	case *protocol.Remote_Menus_warningToast_127:
		// 菜单 - 警告Toast
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 119, "Remote_Menus_warningToast_127", "warning_toast")
		}
	case *protocol.Remote_Menus_openURI_128:
		// 菜单 - 打开URI
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 120, "Remote_Menus_openURI_128", "open_uri")
		}
	case *protocol.Remote_Menus_removeWorldLabel_130:
		// 菜单 - 移除世界标签
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 121, "Remote_Menus_removeWorldLabel_130", "remove_world_label")
		}
	case *protocol.Remote_HudFragment_setPlayerTeamEditor_131:
		// HUD - 设置玩家队伍编辑器
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 122, "Remote_HudFragment_setPlayerTeamEditor_131", "set_player_team_editor")
		}
	case *protocol.Remote_Tile_setTileBlocks_134:
		// Tile - 设置 Tile 的 Blocks
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 125, "Remote_Tile_setTileBlocks_134", "set_tile_blocks")
		}
	case *protocol.Remote_Tile_setTileFloors_135:
		// Tile - 设置 Tile 的 Floors
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 126, "Remote_Tile_setTileFloors_135", "set_tile_floors")
		}
	case *protocol.Remote_Tile_setTileOverlays_136:
		// Tile - 设置 Tile 的 Overlays
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 127, "Remote_Tile_setTileOverlays_136", "set_tile_overlays")
		}
	case *protocol.Remote_Tile_setTeam_141:
		// Tile - 设置单个建筑的队伍
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 132, "Remote_Tile_setTeam_141", "set_team")
		}
	case *protocol.Remote_Tile_setTeams_142:
		// Tile - 批量设置建筑队伍
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 133, "Remote_Tile_setTeams_142", "set_teams")
		}
	case *protocol.Remote_InputHandler_unitBuildingControlSelect_93:
		if s.DevLogger != nil {
			pos := int32(0)
			unitID := int32(0)
			if v.Build != nil {
				pos = v.Build.Pos()
			}
			if v.Unit != nil {
				unitID = v.Unit.ID()
			}
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 135, "Remote_InputHandler_unitBuildingControlSelect_93", fmt.Sprintf("unit=%d pos=%d", unitID, pos))
		}
		if s.OnUnitBuildingControlSelect != nil && v.Unit != nil && v.Build != nil {
			s.OnUnitBuildingControlSelect(c, v.Unit.ID(), v.Build.Pos())
		}
	case *protocol.Remote_ConstructBlock_deconstructFinish_145:
		// 构造方块 - 解构完成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 136, "Remote_ConstructBlock_deconstructFinish_145", "deconstruct_finish")
		}
		if s.OnDeconstructFinish != nil && v.Tile != nil && v.Block != nil {
			req := DeconstructFinishRequest{
				Pos:     v.Tile.Pos(),
				BlockID: v.Block.ID(),
			}
			if v.Builder != nil {
				req.BuilderID = v.Builder.ID()
			}
			s.OnDeconstructFinish(c, req)
		}
	case *protocol.Remote_ConstructBlock_constructFinish_146:
		// 构造方块 - 构造完成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 137, "Remote_ConstructBlock_constructFinish_146", "construct_finish")
		}
		if s.OnConstructFinish != nil && v.Tile != nil && v.Block != nil {
			req := ConstructFinishRequest{
				Pos:      v.Tile.Pos(),
				BlockID:  v.Block.ID(),
				Rotation: v.Rotation,
				TeamID:   v.Team.ID,
				Config:   v.Config,
			}
			if v.Builder != nil {
				// Builder is any type - try to extract ID if it has one
				switch b := v.Builder.(type) {
				case interface{ ID() int32 }:
					req.BuilderID = b.ID()
				}
			}
			s.OnConstructFinish(c, req)
		}
	case *protocol.Remote_LandingPad_landingPadLanded_147:
		// 着陆平台 - 着陆
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 138, "Remote_LandingPad_landingPadLanded_147", "landing_pad_landed")
		}
	case *protocol.Remote_AutoDoor_autoDoorToggle_148:
		// 自动门 - 切换
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 139, "Remote_AutoDoor_autoDoorToggle_148", "auto_door_toggle")
		}
	case *protocol.Remote_CoreBlock_playerSpawn_149:
		// 核心区块 - 玩家生成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 140, "Remote_CoreBlock_playerSpawn_149", "player_spawn")
		}
	case *protocol.Remote_UnitAssembler_assemblerUnitSpawned_150:
		// 单位组装器 - 单位生成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 141, "Remote_UnitAssembler_assemblerUnitSpawned_150", "assembler_unit_spawned")
		}
		if s.OnAssemblerUnitSpawned != nil && v.Tile != nil {
			s.OnAssemblerUnitSpawned(c, v.Tile.Pos())
		}
	case *protocol.Remote_UnitAssembler_assemblerDroneSpawned_151:
		// 单位组装器 - 无人机生成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 142, "Remote_UnitAssembler_assemblerDroneSpawned_151", "assembler_drone_spawned")
		}
		if s.OnAssemblerDroneSpawned != nil && v.Tile != nil {
			s.OnAssemblerDroneSpawned(c, AssemblerDroneSpawnedRequest{
				Pos:    v.Tile.Pos(),
				UnitID: v.Id,
			})
		}
	case *protocol.Remote_UnitBlock_unitBlockSpawn_152:
		// 单位方块 - 单位方块生成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 143, "Remote_UnitBlock_unitBlockSpawn_152", "unit_block_spawn")
		}
	case *protocol.Remote_UnitCargoLoader_unitTetherBlockSpawned_153:
		// 单位货物加载器 - 无人机生成
		if s.DevLogger != nil {
			s.DevLogger.LogPacketReceived(c.id, c.playerID, 144, "Remote_UnitCargoLoader_unitTetherBlockSpawned_153", "unit_tether_block_spawned")
		}
	default:
		// ignore
	}
}

func previewHex(data []byte, max int) string {
	if len(data) == 0 || max <= 0 {
		return ""
	}
	if len(data) > max {
		data = data[:max]
	}
	return hex.EncodeToString(data)
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

func makeSendMessagePacket(message string, unformatted *string) protocol.Packet {
	if unformatted == nil {
		return &protocol.Remote_NetClient_sendMessage_14{Message: message}
	}
	return &protocol.Remote_NetClient_sendMessage_15{
		Message:      message,
		Unformatted:  *unformatted,
		Playersender: nil,
	}
}

func (s *Server) broadcastSimpleMessage(message string) {
	s.mu.Lock()
	peers := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		peers = append(peers, c)
	}
	s.mu.Unlock()

	for _, peer := range peers {
		_ = peer.SendAsync(makeSendMessagePacket(message, nil))
	}
}

func (s *Server) broadcastPlayerChat(sender *Conn, message string) {
	if sender == nil {
		return
	}
	message = sanitizeChatMessage(message)
	if message == "" {
		return
	}
	senderName := s.playerDisplayName(sender)
	if sender.playerID == 0 {
		// Fallback for unexpected state.
		s.broadcastSimpleMessage(formatChatName(senderName) + message)
		return
	}
	s.mu.Lock()
	peers := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		peers = append(peers, c)
	}
	s.mu.Unlock()
	formatted := formatChatName(senderName) + message
	for _, peer := range peers {
		raw := message
		_ = peer.SendAsync(makeSendMessagePacket(formatted, &raw))
	}
}

func (s *Server) sendSystemMessage(c *Conn, message string) {
	if c == nil {
		return
	}
	_ = c.SendAsync(makeSendMessagePacket(message, nil))
}

func (s *Server) SendChat(c *Conn, message string) {
	s.sendSystemMessage(c, message)
}

func (s *Server) broadcastJoinLeaveChat(c *Conn, joined bool) {
	if s == nil || c == nil || !s.joinLeaveChatEnabled.Load() || !c.hasBegunConnecting {
		return
	}
	name := strings.TrimSpace(s.playerDisplayName(c))
	if name == "" {
		name = "未知玩家"
	}
	action := "加入了游戏"
	if !joined {
		action = "退出了游戏"
	}
	message := sanitizeChatMessage(fmt.Sprintf("%s %s", name, action))
	if message == "" {
		return
	}
	s.broadcastSimpleMessage(message)
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
		if peer == nil || !peer.hasConnected || peer.InWorldReloadGrace() {
			continue
		}
		_ = peer.SendAsync(obj)
	}
}

func (s *Server) BroadcastUnreliable(obj any) {
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
		if peer == nil || !peer.hasConnected || peer.InWorldReloadGrace() {
			continue
		}
		if s.udpConn != nil && peer.UDPAddr() == nil {
			if _, isBlockSnapshot := obj.(*protocol.Remote_NetClient_blockSnapshot_34); isBlockSnapshot {
				continue
			}
		}
		_ = s.sendUnreliable(peer, obj)
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
		if s.EventManager == nil || s.shouldSuppressHotNetEvent(kind) {
			return
		}
	} else if s.shouldSuppressHotNetEvent(kind) {
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
	if s.OnEvent != nil {
		s.OnEvent(ev)
	}

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

func (s *Server) shouldSuppressHotNetEvent(kind string) bool {
	if s == nil || s.verboseNetLog.Load() {
		return false
	}
	switch kind {
	case "packet_recv":
		return !s.packetRecvEventsEnabled.Load()
	case "packet_send":
		return !s.packetSendEventsEnabled.Load()
	default:
		return false
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
		s.previewMu.Lock()
		delete(s.previewPlans, c.playerID)
		s.previewMu.Unlock()
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
	serial                *Serializer
	mu                    sync.Mutex
	id                    int32
	playerID              int32
	udpMu                 sync.RWMutex
	udpAddr               *net.UDPAddr
	hasBegunConnecting    bool
	hasConnected          bool
	hasDisconnected       bool
	connectTime           time.Time
	rawName               string
	name                  string
	uuid                  string
	usid                  string
	locale                string
	mobile                bool
	versionType           string
	color                 int32
	snapX                 float32
	snapY                 float32
	pointerX              float32
	pointerY              float32
	shooting              bool
	boosting              bool
	typing                bool
	miningTilePos         int32
	dead                  bool
	deathTimer            float32
	lastRespawnCheck      time.Time
	lastSpawnAt           time.Time
	teamID                byte
	AdminManager          *AdminManager
	worldReloadUntil      time.Time
	liveWorldStream       bool
	pendingReloadConfirms int
	pendingReloadRespawns int
	unitID                int32
	controlBuildPos       int32
	controlBuildActive    bool
	lastRespawnReq        time.Time
	lastSpawnRepairAt     time.Time
	lastDeadIgnoreAt      time.Time
	lastDeadIgnoreLogAt   time.Time
	clientDeadIgnores     int
	building              bool
	selectedBlockID       int16
	selectedRotation      int32
	viewX                 float32
	viewY                 float32
	viewWidth             float32
	viewHeight            float32
	lastClientSnapshot    atomic.Int32
	lastClientTimeMs      atomic.Int64
	syncTime              atomic.Int64
	snapshotsSent         atomic.Int32
	entityDebugSent       atomic.Int32
	lastRecvPacketID      int
	lastRecvFrameworkID   int
	closed                chan struct{}
	closeOnce             sync.Once
	postOnce              sync.Once
	onSend                func(obj any, packetID int, frameworkID int, size int)
	streamMu              sync.Mutex
	streams               map[int32]*StreamBuilder
	outHigh               chan any
	outNorm               chan any
	outClosed             chan struct{}
	sendCount             atomic.Int64
	sendErrors            atomic.Int64
	sendQueued            atomic.Int64
	sendQueueFull         atomic.Int64
	bytesSent             atomic.Int64
	udpSent               atomic.Int64
	udpErrors             atomic.Int64
	statsMu               sync.Mutex
	byTypeSent            map[string]int64
	byTypeBytes           map[string]int64
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

type ControlledBuildInfo struct {
	Pos    int32
	X      float32
	Y      float32
	TeamID byte
}

type UnitRuntimeState struct {
	Shooting       bool
	Boosting       bool
	UpdateBuilding bool
	MineTilePos    int32
	Plans          []*protocol.BuildPlan
}

const sendWriteTimeout = 3 * time.Second

func NewConn(c net.Conn, serial *Serializer) *Conn {
	conn := &Conn{
		Conn:                c,
		serial:              serial,
		connectTime:         time.Now(),
		closed:              make(chan struct{}),
		lastRecvPacketID:    -1,
		lastRecvFrameworkID: -1,
		streams:             make(map[int32]*StreamBuilder),
		outHigh:             make(chan any, 128),
		outNorm:             make(chan any, 256),
		outClosed:           make(chan struct{}),
		byTypeSent:          make(map[string]int64),
		byTypeBytes:         make(map[string]int64),
		miningTilePos:       invalidTilePos,
	}
	conn.lastClientSnapshot.Store(-1)
	go conn.sendLoop()
	return conn
}

func (c *Conn) syncDue(now time.Time, interval time.Duration) bool {
	if c == nil {
		return false
	}
	if interval <= 0 {
		interval = 200 * time.Millisecond
	}
	last := c.syncTime.Load()
	if last <= 0 {
		return true
	}
	return now.UnixMilli()-last >= interval.Milliseconds()
}

func (c *Conn) markSyncTime(now time.Time) {
	if c == nil {
		return
	}
	c.syncTime.Store(now.UnixMilli())
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
		obj, err := c.serial.ReadObject(r)
		if err != nil {
			return nil, err
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
	if c.serial == nil {
		return errors.New("serializer unavailable")
	}

	buf := newBuffer()
	if err := c.serial.WriteObject(buf, obj); err != nil {
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
	if packetID >= 0 && c.sendCount.Load() < 40 && globalVerboseNetLog.Load() {
		fmt.Printf("[net] tx id=%d packet_id=%d type=%T len=%d\n", c.id, packetID, obj, len(payload))
	}
	if len(payload) > 0xFFFF {
		return fmt.Errorf("payload too large: %d", len(payload))
	}
	lenbuf := make([]byte, 2)
	binary.BigEndian.PutUint16(lenbuf, uint16(len(payload)))
	if err := c.Conn.SetWriteDeadline(time.Now().Add(sendWriteTimeout)); err == nil {
		defer func() {
			_ = c.Conn.SetWriteDeadline(time.Time{})
		}()
	}
	if _, err := c.Conn.Write(lenbuf); err != nil {
		c.sendErrors.Add(1)
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
	if err := c.serial.WriteObject(buf, obj); err != nil {
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
		c.mu.Lock()
		c.hasDisconnected = true
		c.mu.Unlock()
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

func (c *Conn) ConnID() int32 {
	return c.id
}

func (c *Conn) UnitID() int32 {
	return c.unitID
}

func (c *Conn) ControlledBuild() (int32, bool) {
	if c == nil || !c.controlBuildActive {
		return 0, false
	}
	return c.controlBuildPos, true
}

func (c *Conn) TeamID() byte {
	if c == nil {
		return 0
	}
	return c.teamID
}

func (c *Conn) SetTeamID(teamID byte) {
	if c == nil {
		return
	}
	c.teamID = teamID
}

func (c *Conn) UUID() string {
	return c.uuid
}

func (c *Conn) USID() string {
	return c.usid
}

func (c *Conn) Name() string {
	if strings.TrimSpace(c.rawName) != "" {
		return c.rawName
	}
	return c.name
}

func (c *Conn) VersionType() string {
	return c.versionType
}

func (c *Conn) SnapshotPos() (float32, float32) {
	return c.snapX, c.snapY
}

func (c *Conn) IsBuilding() bool {
	if c == nil {
		return false
	}
	return c.building
}

func (c *Conn) IsDead() bool {
	if c == nil {
		return true
	}
	return c.dead
}

func (c *Conn) MiningTilePos() (int32, bool) {
	if c == nil || c.miningTilePos == invalidTilePos {
		return 0, false
	}
	return c.miningTilePos, true
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
		// Match Mindustry 157 ArcNetProvider InputStreamSender(stream, 1024).
		chunkLen := 1024
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

func (c *Conn) queueReloadConfirm(respawn bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.pendingReloadConfirms++
	if respawn {
		c.pendingReloadRespawns++
	}
	c.mu.Unlock()
}

func (c *Conn) SetLiveWorldStream(enabled bool) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.liveWorldStream = enabled
	c.mu.Unlock()
}

func (c *Conn) UsesLiveWorldStream() bool {
	if c == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.liveWorldStream
}

func (c *Conn) takeQueuedReloadConfirm() (pending bool, respawn bool) {
	if c == nil {
		return false, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.pendingReloadConfirms <= 0 {
		return false, false
	}
	c.pendingReloadConfirms--
	if c.pendingReloadRespawns > 0 {
		c.pendingReloadRespawns--
		respawn = true
	}
	return true, respawn
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

func (c *Conn) remoteEndpoint() (string, string) {
	if c == nil || c.Conn == nil || c.Conn.RemoteAddr() == nil {
		return "", ""
	}
	host, port, err := net.SplitHostPort(c.Conn.RemoteAddr().String())
	if err != nil {
		return c.Conn.RemoteAddr().String(), ""
	}
	return host, port
}

func (s *Server) addPending(c *Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pending[c.id] = c
}

func (s *Server) nextID() int32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	for {
		id := rand.Int31()
		if id == 0 {
			continue
		}
		if _, ok := s.pending[id]; ok {
			continue
		}
		used := false
		for live := range s.conns {
			if live.id == id {
				used = true
				break
			}
		}
		if used {
			continue
		}
		return id
	}
}

func (s *Server) nextPlayerID() int32 {
	for {
		var id int32
		if s.ReserveUnitIDFn != nil {
			id = s.ReserveUnitIDFn()
		}
		if id <= 0 {
			s.mu.Lock()
			s.playerIDNext++
			id = s.playerIDNext
			s.mu.Unlock()
		}
		if id <= 0 || s.entityIDConflicts(id) {
			continue
		}
		return id
	}
}

func (s *Server) serveUDP(conn *net.UDPConn) {
	buf := make([]byte, 65535)
	for {
		n, addr, err := conn.ReadFromUDP(buf)
		if err != nil {
			if s != nil && s.shuttingDown.Load() && isListenerClosed(err) {
				return
			}
			if ne, ok := err.(net.Error); ok && (ne.Timeout() || ne.Temporary()) {
				continue
			}
			return
		}
		b := append([]byte(nil), buf[:n]...)
		s.handleUDPDatagram(conn, addr, b)
	}
}

func (s *Server) handleUDPDatagram(conn *net.UDPConn, addr *net.UDPAddr, b []byte) {
	defer func() {
		if rec := recover(); rec != nil {
			remote := "<nil>"
			if addr != nil {
				remote = addr.String()
			}
			fmt.Printf("[net] udp handler panic remote=%s err=%v\n", remote, rec)
		}
	}()

	// Handle raw framework UDP register first, before normal packet decoding.
	if ru, ok := parseRegisterUDPRaw(b); ok {
		s.handleUDPRegister(addr, ru.ConnectionID)
		return
	}

	s.mu.Lock()
	c := s.byUDP[addr.String()]
	if c != nil {
		select {
		case <-c.closed:
			delete(s.byUDP, addr.String())
			c = nil
		default:
		}
	}
	s.mu.Unlock()

	// Unregistered peers should only send framework discovery / UDP register
	// packets. Drop all other datagrams silently to avoid spending CPU and log
	// bandwidth on random Internet scans hitting the UDP port.
	if c == nil && (len(b) == 0 || b[0] != 0xFE) {
		return
	}

	var (
		obj any
		err error
	)
	if c != nil {
		obj, err = c.serial.ReadObject(bytesReader(b))
	} else {
		obj, err = s.Serial.ReadObject(bytesReader(b))
	}
	if err != nil {
		if c != nil && isIgnorableUDPPacketReadError(c, err) {
			s.emitEvent(c, "udp_read_error_ignored", "", fmt.Sprintf("packet_id=%d framework_id=%d err=%v", c.lastRecvPacketID, c.lastRecvFrameworkID, err))
			return
		}
		key := fmt.Sprintf("udp-read-failed:%s:%v", addr.String(), err)
		if s.shouldLogRepeatingNetEvent(key, 2*time.Second) {
			fmt.Printf("[net] udp read failed remote=%s len=%d err=%v\n", addr.String(), len(b), err)
		}
		return
	}
	switch v := obj.(type) {
	case *protocol.RegisterUDP:
		s.handleUDPRegister(addr, v.ConnectionID)
	case *protocol.DiscoverHost:
		payload := s.buildServerData()
		if len(payload) > 0 {
			_, _ = conn.WriteToUDP(payload, addr)
		}
	default:
		if c != nil {
			s.handlePacket(c, v, false)
		}
	}
}

func isIgnorableUDPPacketReadError(c *Conn, err error) bool {
	if c == nil || err == nil {
		return false
	}
	// Public UDP ports attract random Internet traffic. If the peer has only
	// completed UDP registration but has not begun the official connect flow yet,
	// treat unknown packet IDs as ignorable noise instead of noisy errors.
	if c.hasBegunConnecting {
		return false
	}
	if c.lastRecvPacketID == 3 {
		return false
	}
	return strings.Contains(err.Error(), "unknown packet id")
}

func (s *Server) handleUDPRegister(addr *net.UDPAddr, connectionID int32) {
	var tc *Conn
	s.mu.Lock()
	c := s.pending[connectionID]
	if c != nil {
		delete(s.pending, c.id)
	} else {
		// Client may retry UDP registration if ACK is lost.
		// In that case connection has already moved out of pending.
		for live := range s.conns {
			if live.id == connectionID {
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
		fmt.Printf("[net] udp registered remote=%s id=%d\n", addr.String(), c.id)
		if err := s.sendUDPRegisterAck(addr, connectionID); err != nil {
			fmt.Printf("[net] udp register ack failed remote=%s id=%d err=%v\n", addr.String(), c.id, err)
		}
		s.verbosef("[net] udp registered remote=%s id=%d\n", addr.String(), c.id)
	}
	s.mu.Unlock()
	// ArcNet clients treat this TCP framework message as UDP registration completion.
	if tc != nil {
		_ = tc.SendAsync(&protocol.RegisterUDP{ConnectionID: connectionID})
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
	players := len(s.ListConnectedConns()) + int(virtualPlayers)
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

func (s *Server) startOfficialConnectConfirmPostConnect(c *Conn) {
	if c == nil {
		return
	}
	c.postOnce.Do(func() {
		c.SetWorldReloadGrace(3 * time.Second)
		s.prepareInitialConnectState(c)
		s.sendInitialPlayerSnapshot(c)
		if s.OnPostConnect != nil {
			go s.OnPostConnect(c)
		}
		go s.postConnectLoop(c)
		s.scheduleInitialConnectRespawn(c)
	})
}

func (s *Server) handleConnectPacket(c *Conn, packet *protocol.ConnectPacket) {
	if s == nil || c == nil || packet == nil {
		return
	}
	if s.DevLogger != nil {
		s.DevLogger.LogConnection("connect packet", c.id, c.remoteIP(), packet.Name, packet.UUID,
			devlog.Int32Fld("version", packet.Version),
			devlog.StringFld("version_type", packet.VersionType),
			devlog.IntFld("mods", len(packet.Mods)))
	} else {
		s.verbosef("[net] connect packet id=%d name=%q version=%d type=%q mods=%d\n",
			c.id, packet.Name, packet.Version, packet.VersionType, len(packet.Mods))
	}

	policy := s.admissionPolicy()
	kick := func(reason protocol.KickReason, label string) {
		s.rejectConnect(c, &reason, "")
		if s.DevLogger != nil {
			s.DevLogger.LogConnection("connect rejected", c.id, c.remoteIP(), packet.Name, packet.UUID,
				devlog.StringFld("reason", label))
		} else {
			fmt.Print(formatConnectRejectLog(c.id, label, nil))
		}
	}
	kickText := func(reason string) {
		s.rejectConnect(c, nil, reason)
		if s.DevLogger != nil {
			s.DevLogger.LogConnection("connect rejected", c.id, c.remoteIP(), packet.Name, packet.UUID,
				devlog.StringFld("reason", reason))
		} else {
			fmt.Print(formatConnectRejectLog(c.id, reason, nil))
		}
	}

	if c.hasBegunConnecting {
		kick(protocol.KickReasonIDInUse, "idInUse")
		return
	}

	versionType := strings.TrimSpace(packet.VersionType)
	if versionType == "" {
		kick(protocol.KickReasonTypeMismatch, "typeMismatch")
		return
	}
	if packet.Version == -1 && !policy.AllowCustomClients {
		kick(protocol.KickReasonCustomClient, "customClient")
		return
	}
	if versionType != "official" && !policy.AllowCustomClients {
		kick(protocol.KickReasonTypeMismatch, "typeMismatch")
		return
	}
	if err := ValidateConnect(packet, s.BuildVersion); err != nil {
		switch {
		case errors.Is(err, ErrClientOutdated):
			kick(protocol.KickReasonClientOutdated, "clientOutdated")
		case errors.Is(err, ErrServerOutdated):
			kick(protocol.KickReasonServerOutdated, "serverOutdated")
		default:
			s.rejectConnect(c, nil, "")
			if s.DevLogger != nil {
				s.DevLogger.LogConnection("connect rejected", c.id, c.remoteIP(), packet.Name, packet.UUID,
					devlog.StringFld("reason", "kick"),
					devlog.StringFld("error", err.Error()))
			} else {
				fmt.Print(formatConnectRejectLog(c.id, "kick", err))
			}
		}
		return
	}

	uuid := strings.TrimSpace(packet.UUID)
	usid := strings.TrimSpace(packet.USID)
	if uuid == "" || usid == "" {
		kick(protocol.KickReasonIDInUse, "idInUse")
		return
	}
	name := packet.Name
	fixedName := FixMindustryPlayerName(name)
	if strings.TrimSpace(fixedName) == "" {
		kick(protocol.KickReasonNameEmpty, "nameEmpty")
		return
	}

	ip := c.remoteIP()
	if admissionSubnetBanned(policy, ip) {
		kickText("banned")
		return
	}
	if admissionNameBanned(policy, name) {
		kickText("banned")
		return
	}
	s.mu.Lock()
	ipReason, ipBanned := s.banIP[ip]
	uuidReason, uuidBanned := s.banUUID[uuid]
	s.mu.Unlock()
	if ipBanned || uuidBanned {
		reason := "banned"
		if uuidBanned && strings.TrimSpace(uuidReason) != "" {
			reason = uuidReason
		} else if ipBanned && strings.TrimSpace(ipReason) != "" {
			reason = ipReason
		}
		kickText(reason)
		return
	}
	if s.isRecentlyKicked(uuid, ip) {
		kick(protocol.KickReasonRecentKick, "recentKick")
		return
	}
	if policy.PlayerLimit > 0 && s.activePlayerCount() >= policy.PlayerLimit && !(s.AdminManager != nil && s.AdminManager.IsOp(uuid)) {
		kick(protocol.KickReasonPlayerLimit, "playerLimit")
		return
	}
	if msg, incompatible := incompatibleModsMessage(policy.ExpectedMods, packet.Mods); incompatible {
		kickText(msg)
		return
	}
	if policy.WhitelistEnabled && !whitelistAllows(policy.Whitelist, uuid, usid) {
		kick(protocol.KickReasonWhitelist, "whitelist")
		return
	}
	if policy.StrictIdentity {
		if reason, duplicate := s.hasDuplicateIdentity(c, uuid, usid, name); duplicate {
			label := "idInUse"
			if reason == protocol.KickReasonNameInUse {
				label = "nameInUse"
			}
			kick(reason, label)
			return
		}
	}

	c.connectTime = time.Now()
	c.uuid = uuid
	c.usid = usid
	c.rawName = fixedName
	c.locale = strings.TrimSpace(packet.Locale)
	if c.locale == "" {
		c.locale = "en"
	}
	c.mobile = packet.Mobile
	c.versionType = versionType
	c.color = packet.Color
	if c.playerID == 0 {
		c.playerID = s.nextPlayerID()
	}
	s.assignConnTeam(c, true)
	c.hasBegunConnecting = true
	s.ensurePlayerEntity(c)
	s.emitEvent(c, "connect_packet", fmt.Sprintf("%T", packet), fmt.Sprintf("version=%d type=%s mods=%d", packet.Version, packet.VersionType, len(packet.Mods)))
	s.refreshPlayerDisplayName(c)
	if err := s.sendWorldHandshake(c, packet); err != nil {
		kickText("world data unavailable")
		fmt.Printf("[net] world handshake failed id=%d err=%v\n", c.id, err)
		return
	}
	s.verbosef("[net] world handshake sent id=%d\n", c.id)
	s.emitEvent(c, "world_handshake_sent", "", "")
}

func (s *Server) startClientSnapshotFallbackPostConnect(c *Conn) {
	if c == nil {
		return
	}
	c.postOnce.Do(func() {
		c.SetWorldReloadGrace(3 * time.Second)
		s.prepareInitialConnectState(c)
		s.sendInitialPlayerSnapshot(c)
		if s.OnPostConnect != nil {
			go s.OnPostConnect(c)
		}
		go s.postConnectLoop(c)
		s.scheduleInitialConnectRespawn(c)
	})
}

func (s *Server) handleOfficialConnectConfirm(c *Conn, _ *protocol.Remote_NetServer_connectConfirm_50) {
	if c == nil {
		return
	}
	wasConnected := c.hasConnected
	c.hasConnected = true
	if !wasConnected {
		s.logPlayerJoinCN(c)
		s.broadcastJoinLeaveChat(c, true)
	}
	s.verbosef("[net] connect confirm id=%d\n", c.id)
	s.emitEvent(c, "connect_confirm", "*protocol.Remote_NetServer_connectConfirm_50", "")
	if !wasConnected {
		c.syncTime.Store(0)
		s.startOfficialConnectConfirmPostConnect(c)
		return
	}
	if pendingReload, reloadRespawn := c.takeQueuedReloadConfirm(); pendingReload {
		c.syncTime.Store(0)
		if s.OnHotReloadConnFn != nil {
			go s.OnHotReloadConnFn(c)
		}
		if reloadRespawn {
			go s.handleMapHotReloadRespawn(c)
		}
		c.SetWorldReloadGrace(2 * time.Second)
	}
}

func (s *Server) handleClientSnapshotConnectFallback(c *Conn, _ *protocol.Remote_NetServer_clientSnapshot_48) {
	if c == nil || c.hasConnected || !s.ClientSnapshotConnectFallbackEnabled() {
		return
	}
	c.hasConnected = true
	s.logPlayerJoinCN(c)
	s.broadcastJoinLeaveChat(c, true)
	s.verbosef("[net] connect confirm via clientSnapshot id=%d\n", c.id)
	s.emitEvent(c, "connect_confirm_client_snapshot", "*protocol.Remote_NetServer_clientSnapshot_48", "")
	s.startClientSnapshotFallbackPostConnect(c)
}

func (s *Server) assignConnTeam(c *Conn, force bool) {
	if s == nil || c == nil {
		return
	}
	if !force && c.teamID != 0 {
		return
	}
	teamID := c.TeamID()
	if s.AssignTeamForConnFn != nil {
		if assigned := s.AssignTeamForConnFn(c); assigned != 0 {
			teamID = assigned
		}
	}
	if teamID == 0 {
		teamID = 1
	}
	c.SetTeamID(teamID)
}

func (s *Server) ConnectedTeamCounts() map[byte]int {
	out := make(map[byte]int)
	if s == nil {
		return out
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for c := range s.conns {
		if c == nil || !c.hasBegunConnecting {
			continue
		}
		if teamID := c.TeamID(); teamID != 0 {
			out[teamID]++
		}
	}
	return out
}

func (c *Conn) InWorldReloadGrace() bool {
	return c != nil && !c.worldReloadUntil.IsZero() && time.Now().Before(c.worldReloadUntil)
}

func (c *Conn) SetWorldReloadGrace(d time.Duration) {
	if c == nil {
		return
	}
	if d <= 0 {
		c.worldReloadUntil = time.Time{}
		return
	}
	c.worldReloadUntil = time.Now().Add(d)
}

func (s *Server) beginWorldHotReload(c *Conn) {
	if c == nil || c.playerID == 0 {
		return
	}
	s.clearConnControlledBuild(c)
	if c.unitID != 0 {
		s.dropPlayerUnitEntity(c, c.unitID)
		c.unitID = 0
	}
	c.dead = true
	c.deathTimer = 0
	c.syncTime.Store(0)
	c.lastRespawnCheck = time.Now()
	c.lastSpawnAt = time.Time{}
	c.SetWorldReloadGrace(3 * time.Second)
	c.queueReloadConfirm(true)
}

func (s *Server) postConnectLoop(c *Conn) {
	syncInterval := s.syncInterval()
	if syncInterval <= 0 {
		syncInterval = 200 * time.Millisecond
	}
	snapshotTicker := time.NewTicker(snapshotPollInterval(syncInterval))
	defer snapshotTicker.Stop()
	keepAliveTicker := time.NewTicker(3 * time.Second)
	defer keepAliveTicker.Stop()
	for {
		select {
		case now := <-snapshotTicker.C:
			if c == nil || !c.hasConnected || c.playerID == 0 || !c.syncDue(now, syncInterval) {
				continue
			}
			c.markSyncTime(now)

			s.maybeRespawn(c)
			state := buildStateSnapshot(s, c)
			if err := s.sendUnreliable(c, state); err != nil {
				fmt.Printf("[net] state snapshot send failed id=%d err=%v\n", c.id, err)
				s.emitEvent(c, "state_snapshot_send_failed", "*protocol.Remote_NetClient_stateSnapshot_35", err.Error())
				return
			}

			if !c.InWorldReloadGrace() {
				packets, hiddenIDs, err := s.buildEntitySnapshotPacketsForConn(c)
				if err != nil {
					fmt.Printf("[net] entity snapshot build failed id=%d err=%v\n", c.id, err)
					s.emitEvent(c, "entity_snapshot_build_failed", "*protocol.Remote_NetClient_entitySnapshot_32", err.Error())
					return
				}
				s.logInitialEntitySnapshotDebug(c, packets)
				for _, packet := range packets {
					if packet == nil {
						continue
					}
					if err := s.sendUnreliable(c, packet); err != nil {
						fmt.Printf("[net] entity snapshot send failed id=%d err=%v\n", c.id, err)
						s.emitEvent(c, "entity_snapshot_send_failed", "*protocol.Remote_NetClient_entitySnapshot_32", err.Error())
						return
					}
				}
				if err := s.sendHiddenSnapshotToConn(c, hiddenIDs); err != nil {
					fmt.Printf("[net] hidden snapshot send failed id=%d err=%v\n", c.id, err)
					s.emitEvent(c, "hidden_snapshot_send_failed", "*protocol.Remote_NetClient_hiddenSnapshot_33", err.Error())
					return
				}
			}
			c.snapshotsSent.Add(1)
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

func (s *Server) sendHiddenSnapshotToConn(c *Conn, hiddenIDs []int32) error {
	if s == nil || c == nil || len(hiddenIDs) == 0 {
		return nil
	}
	return s.sendUnreliable(c, &protocol.Remote_NetClient_hiddenSnapshot_33{
		Ids: protocol.IntSeq{Items: append([]int32(nil), hiddenIDs...)},
	})
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
				s.verbosef("[net] tx-udp id=%d packet_id=%d type=%T len=%d\n", c.id, packetID, obj, len(payload))
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
		if s.udpConn != nil && c.UDPAddr() == nil {
			fmt.Printf("[net] sendUnreliable tcp fallback id=%d obj=%T reason=no_udp_addr\n", c.id, obj)
		}
		return c.Send(obj)
	}
	return nil
}

func (s *Server) sendPlayerSpawn(c *Conn) bool {
	if c == nil || c.playerID == 0 || (s.SpawnTileFn == nil && s.SpawnTileForConnFn == nil) {
		if s.DevLogger != nil {
			s.DevLogger.LogConnection("sendPlayerSpawn skipped", c.id, c.remoteIP(), c.name, c.uuid,
				devlog.BoolFld("has_player", c.playerID != 0),
				devlog.BoolFld("has_spawntilefn", s.SpawnTileFn != nil))
		}
		return false
	}
	pos, ok := s.spawnTileForConn(c)
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
	if s.respawnPacketLogsEnabled.Load() {
		fmt.Printf("[重生] 正在发送玩家出生包: %s 出生点=(%d,%d) playerID=%d\n", s.publicConnField(c), pos.X, pos.Y, c.playerID)
	}
	if err := c.Send(&protocol.Remote_CoreBlock_playerSpawn_149{Tile: tile, Player: player}); err != nil {
		if s.respawnPacketLogsEnabled.Load() {
			fmt.Printf("[重生] 玩家出生包发送失败: %s error=%v\n", s.publicConnField(c), err)
		}
		if s.DevLogger != nil {
			s.DevLogger.LogConnection("sendPlayerSpawn send failed", c.id, c.remoteIP(), c.name, c.uuid,
				devlog.StringFld("error", err.Error()))
		}
		return false
	}
	if s.respawnPacketLogsEnabled.Load() {
		fmt.Printf("[重生] 玩家出生包发送完成: %s\n", s.publicConnField(c))
	}
	return true
}

func tileCenterWorld(pos protocol.Point2) (float32, float32) {
	return float32(pos.X*8 + 4), float32(pos.Y*8 + 4)
}

func (s *Server) prepareInitialConnectState(c *Conn) {
	if s == nil || c == nil || c.playerID == 0 {
		return
	}
	s.assignConnTeam(c, false)
	s.clearConnControlledBuild(c)
	if c.unitID != 0 {
		s.dropPlayerUnitEntity(c, c.unitID)
		c.unitID = 0
	}
	if pos, ok := s.spawnTileForConn(c); ok {
		c.snapX, c.snapY = tileCenterWorld(pos)
	}
	c.dead = true
	c.deathTimer = 0
	c.syncTime.Store(0)
	c.lastRespawnCheck = time.Now()
	c.lastRespawnReq = time.Time{}
	c.lastSpawnAt = time.Time{}
	c.lastSpawnRepairAt = time.Time{}
	c.lastDeadIgnoreAt = time.Time{}
	c.clientDeadIgnores = 0
	c.miningTilePos = invalidTilePos
	if p := s.ensurePlayerEntity(c); p != nil {
		s.updatePlayerEntity(p, c)
	}
}

func (s *Server) sendInitialPlayerSnapshot(c *Conn) {
	if s == nil || c == nil || c.playerID == 0 || c.serial == nil {
		return
	}
	if p := s.ensurePlayerEntity(c); p != nil {
		s.updatePlayerEntity(p, c)
		_ = s.sendEntitySnapshotToConn(c, p)
	}
}

func (s *Server) initialConnectRespawnDelay() time.Duration {
	if s == nil {
		return 350 * time.Millisecond
	}
	delay := time.Duration(s.entitySnapshotIntervalNs.Load()) * 2
	if delay < 350*time.Millisecond {
		delay = 350 * time.Millisecond
	}
	if delay > time.Second {
		delay = time.Second
	}
	return delay
}

func (s *Server) scheduleInitialConnectRespawn(c *Conn) {
	if s == nil || c == nil || c.playerID == 0 {
		return
	}
	time.AfterFunc(s.initialConnectRespawnDelay(), func() {
		if c == nil {
			return
		}
		select {
		case <-c.closed:
			return
		default:
		}
		now := time.Now()
		if !c.hasConnected || c.playerID == 0 || !c.dead || c.unitID != 0 || c.controlBuildActive {
			return
		}
		if s.connUnitAlive(c) {
			return
		}
		if !c.lastSpawnAt.IsZero() && now.Sub(c.lastSpawnAt) >= 0 && now.Sub(c.lastSpawnAt) < 2*time.Second {
			return
		}
		if s.spawnRespawnUnit(c) {
			s.finishRespawn(c)
			s.sendImmediateAliveSync(c, "initial-connect")
			fmt.Printf("[net] respawn sent conn=%d chain=initial-connect\n", c.id)
		} else {
			fmt.Printf("[net] respawn skipped conn=%d chain=initial-connect reason=no-spawn-tile\n", c.id)
		}
	})
}

func (s *Server) playerRespawnUnitType(c *Conn, pos protocol.Point2) int16 {
	playerTypeID := int16(1)
	if fallback := s.fallbackPlayerUnitTypeID(); fallback > 0 {
		playerTypeID = fallback
	} else if s.PlayerUnitTypeFn != nil {
		if t := s.PlayerUnitTypeFn(); t > 0 {
			playerTypeID = t
		}
	}
	if s.ResolveRespawnUnitTypeFn != nil {
		if resolved := s.ResolveRespawnUnitTypeFn(c, pos, playerTypeID); s.validUnitTypeID(resolved) {
			return resolved
		}
	}
	return playerTypeID
}

func (s *Server) isCoreDockRespawnUnitType(typeID int16) bool {
	if !s.validUnitTypeID(typeID) || s.Content == nil {
		return false
	}
	unit := s.Content.UnitType(typeID)
	if unit == nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(unit.Name())) {
	case "evoke", "incite", "emanate":
		return true
	default:
		return false
	}
}

func (s *Server) currentControlledBuildInfo(c *Conn) (ControlledBuildInfo, bool) {
	if s == nil || c == nil || c.playerID == 0 || !c.controlBuildActive || s.ControlledBuildInfoFn == nil {
		return ControlledBuildInfo{}, false
	}
	info, ok := s.ControlledBuildInfoFn(c.playerID, c.controlBuildPos)
	if !ok {
		return ControlledBuildInfo{}, false
	}
	return info, true
}

func (s *Server) clearConnControlledBuild(c *Conn) bool {
	if s == nil || c == nil || c.playerID == 0 || !c.controlBuildActive {
		return false
	}
	if s.ReleaseControlledBuildFn != nil {
		_ = s.ReleaseControlledBuildFn(c.playerID, c.controlBuildPos)
	}
	c.controlBuildPos = 0
	c.controlBuildActive = false
	return true
}

func (s *Server) controlBlockUnit(c *Conn, buildPos int32) bool {
	if s == nil || c == nil || c.playerID == 0 || c.dead || buildPos == 0 || s.ClaimControlledBuildFn == nil {
		return false
	}
	info, ok := s.ClaimControlledBuildFn(c.playerID, buildPos)
	if !ok {
		return false
	}
	if c.unitID != 0 {
		s.detachConnUnit(c, c.unitID)
		c.unitID = 0
	}
	if c.controlBuildActive && c.controlBuildPos != buildPos {
		s.clearConnControlledBuild(c)
	}
	c.controlBuildPos = info.Pos
	c.controlBuildActive = true
	c.snapX = info.X
	c.snapY = info.Y
	if info.TeamID != 0 {
		c.teamID = info.TeamID
	}
	c.dead = false
	c.deathTimer = 0
	c.lastRespawnCheck = time.Time{}
	c.miningTilePos = invalidTilePos
	if s.SetControlledBuildInputFn != nil {
		_ = s.SetControlledBuildInputFn(c.playerID, c.controlBuildPos, c.pointerX, c.pointerY, c.shooting)
	}
	if p := s.ensurePlayerEntity(c); p != nil {
		s.updatePlayerEntity(p, c)
		if c.serial != nil {
			s.sendEntitySnapshotToConn(c, p)
		}
	}
	return true
}

func (s *Server) currentPlayerUnitState(c *Conn) (*protocol.UnitEntitySync, bool) {
	if s == nil || c == nil || c.playerID == 0 || c.unitID == 0 || c.controlBuildActive {
		return nil, false
	}
	return s.playerControlledUnitState(c, c.unitID)
}

func (s *Server) playerControlledUnitState(c *Conn, unitID int32) (*protocol.UnitEntitySync, bool) {
	if s == nil || c == nil || c.playerID == 0 || unitID == 0 {
		return nil, false
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	ent, ok := s.entities[unitID]
	if !ok {
		return nil, false
	}
	unit, ok := ent.(*protocol.UnitEntitySync)
	if !ok || unit == nil {
		return nil, false
	}
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil || state.Type != protocol.ControllerPlayer || state.PlayerID != c.playerID {
		return nil, false
	}
	copy := *unit
	return &copy, true
}

func (s *Server) detachConnUnit(c *Conn, unitID int32) bool {
	if s == nil || c == nil || c.playerID == 0 || unitID == 0 {
		return false
	}
	if unit, ok := s.playerControlledUnitState(c, unitID); ok && unit != nil && unit.SpawnedByCore {
		s.dropPlayerUnitEntity(c, unitID)
		if c.unitID == unitID {
			c.unitID = 0
		}
		return true
	}
	return s.releaseConnUnitControl(c, unitID)
}

func (s *Server) releaseConnUnitControl(c *Conn, unitID int32) bool {
	if s == nil || c == nil || c.playerID == 0 || unitID == 0 {
		return false
	}
	if s.SetUnitPlayerControllerFn != nil {
		_ = s.SetUnitPlayerControllerFn(unitID, 0)
	}
	s.entityMu.Lock()
	if ent, ok := s.entities[unitID]; ok {
		if unit, ok := ent.(*protocol.UnitEntitySync); ok && unit != nil {
			if state, ok := unit.Controller.(*protocol.ControllerState); ok && state != nil &&
				state.Type == protocol.ControllerPlayer && state.PlayerID == c.playerID {
				unit.Controller = &protocol.ControllerState{Type: protocol.ControllerGenericAI}
				unit.UpdateBuilding = false
			}
		}
	}
	s.entityMu.Unlock()
	if c.unitID == unitID {
		c.unitID = 0
	}
	return true
}

func (s *Server) consumeConnUnit(c *Conn, unitID int32) bool {
	if s == nil || c == nil || c.playerID == 0 || unitID == 0 {
		return false
	}
	s.entityMu.Lock()
	if ent, ok := s.entities[unitID]; ok {
		if unit, ok := ent.(*protocol.UnitEntitySync); ok && unit != nil {
			if state, ok := unit.Controller.(*protocol.ControllerState); ok && state != nil &&
				state.Type == protocol.ControllerPlayer && state.PlayerID == c.playerID {
				delete(s.entities, unitID)
			}
		}
	}
	s.entityMu.Unlock()
	if c.unitID == unitID {
		c.unitID = 0
	}
	return true
}

func (s *Server) ReleaseConnUnitControl(c *Conn) bool {
	if s == nil || c == nil {
		return false
	}
	if c.controlBuildActive {
		return s.clearConnControlledBuild(c)
	}
	return s.releaseConnUnitControl(c, c.unitID)
}

func (s *Server) ConsumeConnUnit(c *Conn, unitID int32) bool {
	return s.consumeConnUnit(c, unitID)
}

func (s *Server) HandleCoreBuildingControlSelect(c *Conn, pos protocol.Point2) bool {
	if s == nil || c == nil || c.playerID == 0 || c.dead {
		return false
	}

	s.clearConnControlledBuild(c)
	oldUnitID := c.unitID
	if oldUnitID != 0 {
		s.detachConnUnit(c, oldUnitID)
	}

	playerTypeID := s.playerRespawnUnitType(c, pos)
	if s.SpawnUnitFn != nil {
		c.lastRespawnReq = time.Now()
		c.unitID = s.nextUnitID()
		if x, y, ok := s.SpawnUnitFn(c, c.unitID, pos, playerTypeID); ok {
			c.snapX = x
			c.snapY = y
			c.lastSpawnAt = time.Now()
			c.lastSpawnRepairAt = time.Time{}
			c.lastDeadIgnoreAt = time.Time{}
			c.clientDeadIgnores = 0
			c.miningTilePos = invalidTilePos
		} else {
			c.unitID = 0
			return false
		}
	}
	s.ensurePlayerUnitEntity(c)
	if !s.sendPlayerSpawnAt(c, pos) {
		return false
	}
	c.dead = false
	c.deathTimer = 0
	c.lastRespawnCheck = time.Time{}
	c.lastRespawnReq = time.Now()
	return true
}

func (s *Server) tryDockedUnitClearRespawn(c *Conn, source string) bool {
	if s == nil || c == nil || c.playerID == 0 || c.dead || c.unitID == 0 || s.SpawnUnitAtFn == nil {
		return false
	}
	unit, ok := s.currentPlayerUnitState(c)
	if !ok || unit == nil || unit.SpawnedByCore {
		return false
	}
	pos, ok := s.spawnTileForConn(c)
	if !ok {
		return false
	}
	respawnType := s.playerRespawnUnitType(c, pos)
	if !s.isCoreDockRespawnUnitType(respawnType) {
		return false
	}

	x, y := c.snapX, c.snapY
	if unit.X != 0 || unit.Y != 0 {
		x, y = unit.X, unit.Y
	}
	rotation := unit.Rotation

	oldUnitID := c.unitID
	s.dropPlayerUnitEntity(c, oldUnitID)

	c.lastRespawnReq = time.Now()
	c.unitID = s.nextUnitID()
	sx, sy, spawned := s.SpawnUnitAtFn(c, c.unitID, x, y, rotation, respawnType, true)
	if !spawned {
		c.unitID = 0
		return false
	}

	c.snapX = sx
	c.snapY = sy
	c.dead = false
	c.deathTimer = 0
	c.lastSpawnAt = time.Now()
	c.lastSpawnRepairAt = time.Time{}
	c.lastDeadIgnoreAt = time.Time{}
	c.clientDeadIgnores = 0
	c.lastRespawnCheck = time.Time{}
	c.lastRespawnReq = time.Now()
	c.miningTilePos = invalidTilePos

	if u := s.ensurePlayerUnitEntity(c); u != nil {
		u.SpawnedByCore = true
		u.Rotation = rotation
	}
	if p := s.ensurePlayerEntity(c); p != nil {
		s.updatePlayerEntity(p, c)
	}
	_ = c.SendAsync(&protocol.Remote_NetClient_setPosition_29{X: c.snapX, Y: c.snapY})
	fmt.Printf("[net] dock respawn conn=%d player=%d old_unit=%d new_unit=%d type=%d source=%s pos=(%.1f,%.1f)\n",
		c.id, c.playerID, oldUnitID, c.unitID, respawnType, source, c.snapX, c.snapY)
	return true
}

func (s *Server) spawnPlayerInitial(c *Conn) {
	if c == nil || c.playerID == 0 || (s.SpawnTileFn == nil && s.SpawnTileForConnFn == nil) {
		return
	}
	s.assignConnTeam(c, false)
	pos, ok := s.spawnTileForConn(c)
	if !ok {
		return
	}
	playerTypeID := s.playerRespawnUnitType(c, pos)
	if s.SpawnUnitFn != nil {
		if c.unitID != 0 {
			s.dropPlayerUnitEntity(c, c.unitID)
			c.unitID = 0
		}
		c.lastRespawnReq = time.Now()
		c.unitID = s.nextUnitID()
		if x, y, ok := s.SpawnUnitFn(c, c.unitID, pos, playerTypeID); ok {
			if s.UnitInfoFn != nil {
				if _, exists := s.UnitInfoFn(c.unitID); !exists {
					if s.DropUnitFn != nil {
						s.DropUnitFn(c.unitID)
					}
					c.unitID = 0
					return
				}
			}
			c.snapX = x
			c.snapY = y
			c.lastSpawnAt = time.Now()
			c.lastSpawnRepairAt = time.Time{}
			c.lastDeadIgnoreAt = time.Time{}
			c.clientDeadIgnores = 0
			c.miningTilePos = invalidTilePos
		} else {
			c.unitID = 0
			return
		}
	}
	if c.unitID == 0 {
		return
	}
	s.ensurePlayerUnitEntity(c)
	if !s.sendPlayerSpawnAt(c, pos) {
		s.dropPlayerUnitEntity(c, c.unitID)
		c.unitID = 0
	}
}

func (s *Server) spawnRespawnUnit(c *Conn) bool {
	if c == nil || c.playerID == 0 {
		return false
	}
	if s.SpawnTileFn == nil && s.SpawnTileForConnFn == nil {
		return false
	}
	s.assignConnTeam(c, false)
	pos, ok := s.spawnTileForConn(c)
	if !ok {
		return false
	}
	playerTypeID := s.playerRespawnUnitType(c, pos)
	if s.SpawnUnitFn != nil {
		if c.unitID != 0 {
			s.dropPlayerUnitEntity(c, c.unitID)
			c.unitID = 0
		}
		c.lastRespawnReq = time.Now()
		c.unitID = s.nextUnitID()
		if x, y, ok := s.SpawnUnitFn(c, c.unitID, pos, playerTypeID); ok {
			if s.UnitInfoFn != nil {
				if _, exists := s.UnitInfoFn(c.unitID); !exists {
					if s.DropUnitFn != nil {
						s.DropUnitFn(c.unitID)
					}
					c.unitID = 0
					return false
				}
			}
			c.snapX = x
			c.snapY = y
			c.lastSpawnAt = time.Now()
			c.lastSpawnRepairAt = time.Time{}
			c.lastDeadIgnoreAt = time.Time{}
			c.clientDeadIgnores = 0
			c.miningTilePos = invalidTilePos
		} else {
			c.unitID = 0
			return false
		}
	}
	if c.unitID == 0 {
		return false
	}
	s.ensurePlayerUnitEntity(c)
	if !s.sendPlayerSpawnAt(c, pos) {
		s.dropPlayerUnitEntity(c, c.unitID)
		c.unitID = 0
		return false
	}
	return true
}

func (s *Server) finishRespawn(c *Conn) {
	if c == nil {
		return
	}
	c.dead = false
	c.deathTimer = 0
	c.lastRespawnCheck = time.Time{}
	c.lastRespawnReq = time.Now()
}

// Official InputHandler.unitClear() chain:
// explicit player respawn request from the 157 client.
func (s *Server) handleOfficialUnitClear(c *Conn) {
	if c == nil || c.playerID == 0 {
		return
	}
	if c.InWorldReloadGrace() && c.lastSpawnAt.IsZero() && c.unitID == 0 && !c.controlBuildActive {
		return
	}
	now := time.Now()
	if !c.lastRespawnReq.IsZero() && now.Sub(c.lastRespawnReq) < 250*time.Millisecond {
		return
	}
	if s.connUnitAlive(c) {
		if s.tryDockedUnitClearRespawn(c, "unitClear-91") {
			return
		}
		spawnedByCore := false
		if unit, ok := s.currentPlayerUnitState(c); ok && unit != nil {
			spawnedByCore = unit.SpawnedByCore
		}
		fmt.Printf("[net] alive unitClear ignored conn=%d player=%d unit=%d spawned_by_core=%v age=%s\n",
			c.id, c.playerID, c.unitID, spawnedByCore, now.Sub(c.lastSpawnAt).Round(10*time.Millisecond))
		s.repairOfficialUnitClearAliveBinding(c)
		return
	}
	if s.connFreshSpawnWorldBindingMissing(c, now, 2*time.Second) {
		fmt.Printf("[net] stale unitClear ignored conn=%d player=%d unit=%d age=%s\n",
			c.id, c.playerID, c.unitID, now.Sub(c.lastSpawnAt).Round(10*time.Millisecond))
		return
	}
	c.lastRespawnReq = now
	s.markDead(c, "unitClear-91")
	if s.spawnRespawnUnit(c) {
		s.finishRespawn(c)
		s.sendImmediateAliveSync(c, "official-unitClear")
		fmt.Printf("[net] respawn sent conn=%d chain=official-unitClear\n", c.id)
	} else {
		fmt.Printf("[net] respawn skipped conn=%d chain=official-unitClear reason=no-spawn-tile\n", c.id)
	}
}

func (s *Server) spawnTileForConn(c *Conn) (protocol.Point2, bool) {
	if s == nil {
		return protocol.Point2{}, false
	}
	if s.SpawnTileForConnFn != nil {
		if pos, ok := s.SpawnTileForConnFn(c); ok {
			return pos, true
		}
	}
	if s.SpawnTileFn != nil {
		return s.SpawnTileFn()
	}
	return protocol.Point2{}, false
}

func (s *Server) shouldForceRespawnAfterDeadIgnored(c *Conn, now time.Time) bool {
	if s == nil || c == nil || c.playerID == 0 {
		return false
	}
	if !c.lastSpawnRepairAt.IsZero() && (c.lastSpawnAt.IsZero() || !c.lastSpawnAt.After(c.lastSpawnRepairAt)) {
		return false
	}
	if c.lastDeadIgnoreAt.IsZero() || now.Sub(c.lastDeadIgnoreAt) > 1500*time.Millisecond {
		c.clientDeadIgnores = 0
	}
	c.lastDeadIgnoreAt = now
	c.clientDeadIgnores++
	if c.lastSpawnAt.IsZero() {
		return false
	}
	if now.Sub(c.lastSpawnAt) < 500*time.Millisecond {
		return false
	}
	return c.clientDeadIgnores >= 3
}

func recentNonZeroTime(a, b time.Time) time.Time {
	switch {
	case a.IsZero():
		return b
	case b.IsZero():
		return a
	case a.After(b):
		return a
	default:
		return b
	}
}

func (s *Server) connHasRecentRespawnWindow(c *Conn, now time.Time, window time.Duration) bool {
	if c == nil || c.playerID == 0 || window <= 0 {
		return false
	}
	last := recentNonZeroTime(c.lastSpawnAt, c.lastRespawnReq)
	if last.IsZero() {
		return false
	}
	age := now.Sub(last)
	return age >= 0 && age < window
}

func (s *Server) connHasFreshSpawnBinding(c *Conn, now time.Time, window time.Duration) bool {
	if s == nil || c == nil || c.playerID == 0 {
		return false
	}
	if c.controlBuildActive || c.dead || c.unitID == 0 || c.lastSpawnAt.IsZero() {
		return false
	}
	age := now.Sub(c.lastSpawnAt)
	return age >= 0 && age < window
}

func (s *Server) connFreshSpawnWorldBindingMissing(c *Conn, now time.Time, window time.Duration) bool {
	if !s.connHasFreshSpawnBinding(c, now, window) {
		return false
	}
	if c.lastRespawnReq.IsZero() || now.Sub(c.lastRespawnReq) >= window {
		return false
	}
	if s.UnitInfoFn != nil {
		_, ok := s.UnitInfoFn(c.unitID)
		return !ok
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	_, ok := s.entities[c.unitID]
	return !ok
}

func (s *Server) sendEntitySnapshotToConn(c *Conn, entities ...protocol.UnitSyncEntity) bool {
	if s == nil || c == nil {
		return false
	}

	var data []byte
	var amount int16
	appendEntity := func(entity protocol.UnitSyncEntity) {
		if entity == nil {
			return
		}
		entry := s.serializeEntity(entity)
		if len(entry) == 0 {
			return
		}
		data = append(data, entry...)
		amount++
	}
	for _, entity := range entities {
		if entity == nil || entity.ClassID() == 12 {
			continue
		}
		appendEntity(entity)
	}
	for _, entity := range entities {
		if entity == nil || entity.ClassID() != 12 {
			continue
		}
		appendEntity(entity)
	}
	if amount == 0 {
		return false
	}

	if err := s.sendUnreliable(c, &protocol.Remote_NetClient_entitySnapshot_32{
		Amount: amount,
		Data:   data,
	}); err != nil {
		fmt.Printf("[net] alive entity snapshot send failed conn=%d player=%d err=%v\n", c.id, c.playerID, err)
		return false
	}
	return true
}

func (s *Server) repairAliveSpawnBinding(c *Conn, reason string, allowRepeat bool) {
	if s == nil || c == nil || c.playerID == 0 || c.unitID == 0 {
		return
	}
	if c.InWorldReloadGrace() || !s.connUnitAlive(c) {
		return
	}
	now := time.Now()
	if !c.lastSpawnRepairAt.IsZero() {
		if !allowRepeat {
			if c.lastSpawnAt.IsZero() || !c.lastSpawnAt.After(c.lastSpawnRepairAt) {
				return
			}
		} else if now.Sub(c.lastSpawnRepairAt) < 250*time.Millisecond {
			return
		}
	}
	var unit protocol.UnitSyncEntity
	if u := s.ensurePlayerUnitEntity(c); u != nil {
		s.syncUnitFromWorld(u)
		c.snapX = u.X
		c.snapY = u.Y
		if s.prepareUnitEntitySnapshot(u) {
			unit = u
		}
	}
	var player protocol.UnitSyncEntity
	if p := s.ensurePlayerEntity(c); p != nil {
		s.updatePlayerEntity(p, c)
		player = p
	}
	c.dead = false
	c.deathTimer = 0
	c.lastRespawnCheck = now
	c.lastSpawnRepairAt = now
	c.clientDeadIgnores = 0
	c.lastDeadIgnoreAt = time.Time{}
	snapshotSent := false
	if c.serial != nil {
		snapshotSent = s.sendEntitySnapshotToConn(c, player, unit)
		_ = c.SendAsync(&protocol.Remote_NetClient_setPosition_29{X: c.snapX, Y: c.snapY})
	}
	fmt.Printf("[net] alive binding synced conn=%d player=%d unit=%d source=%s snapshot=%v pos=(%.1f,%.1f)\n",
		c.id, c.playerID, c.unitID, reason, snapshotSent, c.snapX, c.snapY)
}

func (s *Server) sendImmediateAliveSync(c *Conn, source string) {
	if s == nil || c == nil || c.playerID == 0 {
		return
	}
	var unit protocol.UnitSyncEntity
	if u := s.snapshotPlayerUnitEntity(c); u != nil && s.prepareUnitEntitySnapshot(u) {
		unit = u
		c.snapX = u.X
		c.snapY = u.Y
	}
	var player protocol.UnitSyncEntity
	if p := s.ensurePlayerEntity(c); p != nil {
		s.updatePlayerEntity(p, c)
		player = p
	}
	snapshotSent := false
	if c.serial != nil {
		snapshotSent = s.sendEntitySnapshotToConn(c, unit, player)
		_ = c.SendAsync(&protocol.Remote_NetClient_setPosition_29{X: c.snapX, Y: c.snapY})
	}
	fmt.Printf("[net] alive binding synced conn=%d player=%d unit=%d source=%s snapshot=%v pos=(%.1f,%.1f)\n",
		c.id, c.playerID, c.unitID, source, snapshotSent, c.snapX, c.snapY)
}

func (s *Server) repairClientDeadAliveBinding(c *Conn) {
	s.repairAliveSpawnBinding(c, "client-dead-ignored", false)
}

func (s *Server) repairClientDeadStuckBinding(c *Conn) {
	s.repairAliveSpawnBinding(c, "client-dead-stuck", true)
}

func (s *Server) repairOfficialUnitClearAliveBinding(c *Conn) {
	s.repairAliveSpawnBinding(c, "official-unitClear-alive", true)
}

func (s *Server) handleDeathTimerRespawn(c *Conn) {
	if s.spawnRespawnUnit(c) {
		s.finishRespawn(c)
		s.sendImmediateAliveSync(c, "death-timer")
		fmt.Printf("[net] respawn sent conn=%d chain=death-timer\n", c.id)
	} else {
		fmt.Printf("[net] respawn skipped conn=%d chain=death-timer reason=no-spawn-tile\n", c.id)
	}
}

func (s *Server) handleMapHotReloadRespawn(c *Conn) {
	if s.spawnRespawnUnit(c) {
		s.finishRespawn(c)
		s.sendImmediateAliveSync(c, "map-hot-reload")
		fmt.Printf("[net] respawn sent conn=%d chain=map-hot-reload\n", c.id)
	} else {
		fmt.Printf("[net] respawn skipped conn=%d chain=map-hot-reload reason=no-spawn-tile\n", c.id)
	}
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
	s.clearConnControlledBuild(c)
	if c.unitID != 0 {
		s.dropPlayerUnitEntity(c, c.unitID)
		c.unitID = 0
	}
	if !c.dead {
		c.dead = true
		c.deathTimer = 0
		c.lastRespawnCheck = time.Now()
		c.lastSpawnAt = time.Time{}
		c.lastSpawnRepairAt = time.Time{}
		c.lastDeadIgnoreAt = time.Time{}
		c.clientDeadIgnores = 0
		c.miningTilePos = invalidTilePos
	}
	fmt.Printf("[net] player dead conn=%d player=%d team=%d source=%s snap=(%.1f,%.1f)\n",
		c.id, c.playerID, c.TeamID(), source, c.snapX, c.snapY)
	if s.DevLogger != nil {
		s.DevLogger.LogConnection("player_dead", c.id, c.remoteIP(), c.name, c.uuid,
			devlog.StringFld("source", source))
	}
}

func (s *Server) maybeRespawn(c *Conn) {
	if c == nil || c.playerID == 0 {
		return
	}
	if c.InWorldReloadGrace() {
		return
	}
	if c.dead && s.connUnitAlive(c) {
		c.dead = false
		c.deathTimer = 0
		c.lastRespawnCheck = time.Now()
		return
	}
	if !c.dead {
		// Do not self-mark living players as dead from polling. Actual deaths are
		// already signalled by explicit world/client events; mirror gaps here would
		// otherwise create false death-timer respawn loops.
		if c.unitID != 0 {
			if u := s.playerUnitEntity(c); u != nil {
				s.syncUnitFromWorld(u)
			} else if s.connUnitAlive(c) {
				if u := s.ensurePlayerUnitEntity(c); u != nil {
					s.syncUnitFromWorld(u)
				}
			}
		}
		c.deathTimer = 0
		c.lastRespawnCheck = time.Now()
		return
	}
	if _, ok := s.spawnTileForConn(c); !ok {
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
		s.handleDeathTimerRespawn(c)
	}
}

func (s *Server) connUnitAlive(c *Conn) bool {
	if s == nil || c == nil || c.playerID == 0 {
		return false
	}
	if c.controlBuildActive {
		_, ok := s.currentControlledBuildInfo(c)
		return ok
	}
	if c.unitID == 0 {
		return false
	}
	if s.UnitInfoFn != nil {
		if info, ok := s.UnitInfoFn(c.unitID); ok {
			return info.Health > 0
		}
		return false
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	ent, ok := s.entities[c.unitID]
	if !ok {
		return false
	}
	if u, ok := ent.(*protocol.UnitEntitySync); ok {
		return u.Health > 0
	}
	return true
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
	packets, _, err := s.buildEntitySnapshotPacketsForConn(nil)
	if err != nil {
		return 0, nil, err
	}
	if len(packets) == 0 || packets[0] == nil {
		return 0, nil, nil
	}
	return packets[0].Amount, packets[0].Data, nil
}

func (s *Server) buildEntitySnapshotPackets() ([]*protocol.Remote_NetClient_entitySnapshot_32, error) {
	packets, _, err := s.buildEntitySnapshotPacketsForConn(nil)
	return packets, err
}

func (s *Server) entitySnapshotHidden(viewer *Conn, entity protocol.UnitSyncEntity) bool {
	if s == nil || viewer == nil || entity == nil || s.EntitySnapshotHiddenFn == nil {
		return false
	}
	return s.EntitySnapshotHiddenFn(viewer, entity)
}

func appendHiddenEntityID(hiddenIDs *[]int32, hiddenSet map[int32]struct{}, entityID int32) {
	if entityID == 0 {
		return
	}
	if _, ok := hiddenSet[entityID]; ok {
		return
	}
	hiddenSet[entityID] = struct{}{}
	*hiddenIDs = append(*hiddenIDs, entityID)
}

func (s *Server) buildEntitySnapshotPacketsForConn(viewer *Conn) ([]*protocol.Remote_NetClient_entitySnapshot_32, []int32, error) {
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
	sort.Slice(players, func(i, j int) bool {
		if players[i].playerID != players[j].playerID {
			return players[i].playerID < players[j].playerID
		}
		return players[i].id < players[j].id
	})

	extraEntities := []protocol.UnitSyncEntity(nil)
	if s.ExtraEntitySnapshotEntitiesFn != nil {
		var err error
		extraEntities, err = s.ExtraEntitySnapshotEntitiesFn()
		if err != nil {
			return nil, nil, err
		}
		filtered := make([]protocol.UnitSyncEntity, 0, len(extraEntities))
		for _, ent := range extraEntities {
			if ent == nil {
				continue
			}
			filtered = append(filtered, ent)
		}
		extraEntities = filtered
		sort.Slice(extraEntities, func(i, j int) bool {
			if extraEntities[i].ID() != extraEntities[j].ID() {
				return extraEntities[i].ID() < extraEntities[j].ID()
			}
			return extraEntities[i].ClassID() < extraEntities[j].ClassID()
		})
	}

	encodeEntity := func(entity protocol.UnitSyncEntity) ([]byte, error) {
		if entity == nil {
			return nil, nil
		}
		ew := protocol.NewWriterWithContext(s.TypeIO)
		if err := ew.WriteInt32(entity.ID()); err != nil {
			return nil, err
		}
		if err := ew.WriteByte(entity.ClassID()); err != nil {
			return nil, err
		}
		entity.BeforeWrite()
		if err := entity.WriteSync(ew); err != nil {
			return nil, err
		}
		return ew.Bytes(), nil
	}

	writer := protocol.NewWriterWithContext(s.TypeIO)
	packets := make([]*protocol.Remote_NetClient_entitySnapshot_32, 0, 4)
	hiddenIDs := make([]int32, 0, 8)
	hiddenSet := map[int32]struct{}{}
	var sent int16
	flush := func() {
		if sent <= 0 {
			return
		}
		packets = append(packets, &protocol.Remote_NetClient_entitySnapshot_32{
			Amount: sent,
			Data:   append([]byte(nil), writer.Bytes()...),
		})
		writer = protocol.NewWriterWithContext(s.TypeIO)
		sent = 0
	}
	appendEntry := func(entry []byte) error {
		if len(entry) == 0 {
			return nil
		}
		if len(entry) > maxEntitySnapshotData {
			return fmt.Errorf("entity snapshot entry too large: %d", len(entry))
		}
		if sent > 0 && len(writer.Bytes())+len(entry) > maxEntitySnapshotData {
			flush()
		}
		if err := writer.WriteBytes(entry); err != nil {
			return err
		}
		sent++
		return nil
	}

	for _, p := range players {
		unit := s.snapshotPlayerUnitEntity(p)
		if unit != nil && !s.prepareUnitEntitySnapshot(unit) {
			fmt.Printf("[net] skipped unit snapshot conn=%d player=%d unit=%d invalid_type=%d\n",
				p.id, p.playerID, unit.ID(), unit.TypeID)
			unit = nil
		}
		ent := &protocol.PlayerEntity{IDValue: p.playerID}
		s.updatePlayerEntity(ent, p)
		if unit != nil && s.entitySnapshotHidden(viewer, unit) {
			appendHiddenEntityID(&hiddenIDs, hiddenSet, unit.ID())
			unit = nil
		}
		if unit != nil {
			ent.Unit = protocol.UnitBox{IDValue: unit.ID()}
		} else {
			ent.Unit = nil
		}
		playerHidden := s.entitySnapshotHidden(viewer, ent)
		if playerHidden {
			appendHiddenEntityID(&hiddenIDs, hiddenSet, ent.ID())
		}
		if unit != nil {
			unitEntry, err := encodeEntity(unit)
			if err != nil {
				return nil, nil, err
			}
			if err := appendEntry(unitEntry); err != nil {
				return nil, nil, err
			}
		}
		if !playerHidden {
			playerEntry, err := encodeEntity(ent)
			if err != nil {
				return nil, nil, err
			}
			if err := appendEntry(playerEntry); err != nil {
				return nil, nil, err
			}
		}
	}

	for _, ent := range extraEntities {
		switch typed := ent.(type) {
		case *protocol.UnitEntitySync:
			unit := cloneUnitEntitySync(typed)
			if unit == nil || !s.prepareUnitEntitySnapshot(unit) {
				continue
			}
			if s.entitySnapshotHidden(viewer, unit) {
				appendHiddenEntityID(&hiddenIDs, hiddenSet, unit.ID())
				continue
			}
			entry, err := encodeEntity(unit)
			if err != nil {
				return nil, nil, err
			}
			if err := appendEntry(entry); err != nil {
				return nil, nil, err
			}
		default:
			if s.entitySnapshotHidden(viewer, ent) {
				appendHiddenEntityID(&hiddenIDs, hiddenSet, ent.ID())
				continue
			}
			entry, err := encodeEntity(ent)
			if err != nil {
				return nil, nil, err
			}
			if err := appendEntry(entry); err != nil {
				return nil, nil, err
			}
		}
	}

	if s.ExtraEntitySnapshotFn != nil {
		if sent > 0 {
			flush()
		}
		legacyWriter := protocol.NewWriterWithContext(s.TypeIO)
		n, err := s.ExtraEntitySnapshotFn(legacyWriter)
		if err != nil {
			return nil, nil, err
		}
		if n > 0 || len(legacyWriter.Bytes()) > 0 {
			if len(legacyWriter.Bytes()) > maxEntitySnapshotData {
				return nil, nil, fmt.Errorf("legacy entity snapshot payload too large: %d", len(legacyWriter.Bytes()))
			}
			packets = append(packets, &protocol.Remote_NetClient_entitySnapshot_32{
				Amount: n,
				Data:   append([]byte(nil), legacyWriter.Bytes()...),
			})
		}
	}

	flush()
	return packets, hiddenIDs, nil
}

func (s *Server) logInitialEntitySnapshotDebug(c *Conn, packets []*protocol.Remote_NetClient_entitySnapshot_32) {
	if s == nil || c == nil || len(packets) == 0 || !s.verboseNetLog.Load() {
		return
	}
	seq := int(c.entityDebugSent.Add(1))
	if seq > 4 {
		return
	}
	for i, packet := range packets {
		if packet == nil {
			continue
		}
		fmt.Printf("[net] entity snapshot debug conn=%d player=%d seq=%d packet=%d/%d amount=%d bytes=%d entries=%s\n",
			c.id, c.playerID, seq, i+1, len(packets), packet.Amount, len(packet.Data),
			strings.Join(s.describeEntitySnapshotPacket(packet), " | "))
	}
}

func (s *Server) describeEntitySnapshotPacket(packet *protocol.Remote_NetClient_entitySnapshot_32) []string {
	if s == nil || packet == nil {
		return nil
	}
	r := protocol.NewReaderWithContext(packet.Data, s.TypeIO)
	out := make([]string, 0, int(packet.Amount)+1)
	for i := 0; i < int(packet.Amount); i++ {
		entryStart := r.Remaining()
		id, err := r.ReadInt32()
		if err != nil {
			out = append(out, fmt.Sprintf("entry=%d read_id_err=%v", i, err))
			return out
		}
		classID, err := r.ReadByte()
		if err != nil {
			out = append(out, fmt.Sprintf("id=%d read_class_err=%v", id, err))
			return out
		}
		switch classID {
		case 12:
			player := &protocol.PlayerEntity{IDValue: id}
			if err := player.ReadSync(r); err != nil {
				out = append(out, fmt.Sprintf("id=%d class=%d player_err=%v", id, classID, err))
				return out
			}
			unitID := int32(0)
			if player.Unit != nil {
				unitID = player.Unit.ID()
			}
			out = append(out, fmt.Sprintf("id=%d class=%d player team=%d unit=%d name_bytes=%d len=%d",
				id, classID, player.TeamID, unitID, len([]byte(player.Name)), entryStart-r.Remaining()))
		default:
			unit := &protocol.UnitEntitySync{IDValue: id, ClassIDValue: classID, ClassIDSet: true}
			if err := unit.ReadSync(r); err != nil {
				out = append(out, fmt.Sprintf("id=%d class=%d unit_err=%v", id, classID, err))
				return out
			}
			out = append(out, fmt.Sprintf("id=%d class=%d unit type=%d team=%d len=%d",
				id, classID, unit.TypeID, unit.TeamID, entryStart-r.Remaining()))
		}
	}
	if rem := r.Remaining(); rem != 0 {
		out = append(out, fmt.Sprintf("remaining=%d", rem))
	}
	return out
}

func cloneControllerState(src any) any {
	state, ok := src.(*protocol.ControllerState)
	if !ok || state == nil {
		return src
	}
	copy := *state
	return &copy
}

func cloneUnitEntitySync(src *protocol.UnitEntitySync) *protocol.UnitEntitySync {
	if src == nil {
		return nil
	}
	copy := *src
	copy.Controller = cloneControllerState(src.Controller)
	copy.Abilities = append([]protocol.Ability(nil), src.Abilities...)
	copy.Mounts = append([]protocol.WeaponMount(nil), src.Mounts...)
	copy.Payloads = append([]protocol.Payload(nil), src.Payloads...)
	copy.Statuses = append([]protocol.StatusEntry(nil), src.Statuses...)
	if src.Plans != nil {
		copy.Plans = make([]*protocol.BuildPlan, len(src.Plans))
		for i, plan := range src.Plans {
			if plan == nil {
				continue
			}
			planCopy := *plan
			copy.Plans[i] = &planCopy
		}
	}
	return &copy
}

func controlledBlockUnitRef(pos int32) protocol.Unit {
	return protocol.BlockUnitRef{
		TileRef: protocol.BlockUnitTileRef{PosValue: pos},
	}
}

func extractControlledBuildPos(obj any) (int32, bool) {
	switch v := obj.(type) {
	case protocol.BlockUnit:
		if v.Tile() == nil {
			return 0, false
		}
		return v.Tile().Pos(), true
	default:
		return 0, false
	}
}

func (s *Server) writePlayerSync(w *protocol.Writer, c *Conn) error {
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
	name := s.playerDisplayName(c)
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
	teamID := c.TeamID()
	playerUnit := protocol.Unit(nil)
	if info, ok := s.currentControlledBuildInfo(c); ok {
		teamID = info.TeamID
		playerUnit = controlledBlockUnitRef(info.Pos)
	}
	if s.UnitInfoFn != nil && c.unitID != 0 {
		if info, ok := s.UnitInfoFn(c.unitID); ok && info.TeamID != 0 {
			teamID = info.TeamID
		}
	}
	if err := protocol.WriteTeam(w, &protocol.Team{ID: teamID}); err != nil {
		return err
	}
	if err := w.WriteBool(c.typing); err != nil { // typing
		return err
	}
	if err := protocol.WriteUnit(w, playerUnit); err != nil {
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
	p.Name = s.playerDisplayName(c)
	p.SelectedBlock = -1
	p.SelectedRotation = 0
	p.Shooting = c.shooting
	p.TeamID = c.TeamID()
	p.Unit = nil
	if info, ok := s.currentControlledBuildInfo(c); ok {
		p.TeamID = info.TeamID
		p.Unit = controlledBlockUnitRef(info.Pos)
		p.X = info.X
		p.Y = info.Y
	} else {
		p.X = c.snapX
		p.Y = c.snapY
	}
	if s.UnitInfoFn != nil && c.unitID != 0 {
		if info, ok := s.UnitInfoFn(c.unitID); ok && info.TeamID != 0 {
			p.TeamID = info.TeamID
		}
	}
	p.Typing = c.typing
	if c.unitID != 0 && !c.controlBuildActive && s.hasValidPlayerUnitEntity(c) {
		p.Unit = protocol.UnitBox{IDValue: c.unitID}
	}
}

func (s *Server) validUnitTypeID(typeID int16) bool {
	if s == nil || typeID <= 0 || s.Content == nil {
		return false
	}
	return s.Content.UnitType(typeID) != nil
}

func (s *Server) fallbackPlayerUnitTypeID() int16 {
	if s == nil || s.PlayerUnitTypeFn == nil {
		return 0
	}
	typeID := s.PlayerUnitTypeFn()
	if !s.validUnitTypeID(typeID) {
		return 0
	}
	return typeID
}

func (s *Server) unitTypeName(typeID int16) string {
	if s == nil || typeID <= 0 || s.Content == nil {
		return ""
	}
	unit := s.Content.UnitType(typeID)
	if unit == nil {
		return ""
	}
	return unit.Name()
}

func (s *Server) applyUnitEntityLayout(u *protocol.UnitEntitySync) {
	if u == nil {
		return
	}
	u.ApplyLayoutByName(s.unitTypeName(u.TypeID))
}

func (s *Server) authoritativeUnitSnapshot(unitID int32, controller protocol.UnitController) *protocol.UnitEntitySync {
	if s == nil || unitID == 0 || s.UnitSyncFn == nil {
		return nil
	}
	unit, ok := s.UnitSyncFn(unitID, controller)
	if !ok || unit == nil {
		return nil
	}
	return cloneUnitEntitySync(unit)
}

func (s *Server) normalizedUnitTypeID(typeID int16, controller protocol.UnitController) int16 {
	if s.validUnitTypeID(typeID) {
		return typeID
	}
	if state, ok := controller.(*protocol.ControllerState); ok && state != nil && state.Type == protocol.ControllerPlayer {
		return s.fallbackPlayerUnitTypeID()
	}
	return 0
}

func (s *Server) hasValidPlayerUnitEntity(c *Conn) bool {
	if s == nil || c == nil || c.unitID == 0 || c.controlBuildActive {
		return false
	}
	if unit := s.authoritativeUnitSnapshot(c.unitID, &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}); unit != nil {
		return unit.Health > 0 && s.normalizedUnitTypeID(unit.TypeID, unit.Controller) > 0
	}
	if s.UnitInfoFn != nil {
		info, ok := s.UnitInfoFn(c.unitID)
		if !ok || info.Health <= 0 {
			return false
		}
		return s.normalizedUnitTypeID(info.TypeID, &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}) > 0
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	ent, ok := s.entities[c.unitID]
	if !ok {
		return false
	}
	u, ok := ent.(*protocol.UnitEntitySync)
	if !ok || u == nil {
		return false
	}
	return s.validUnitTypeID(u.TypeID)
}

func (s *Server) prepareUnitEntitySnapshot(u *protocol.UnitEntitySync) bool {
	if s == nil || u == nil {
		return false
	}
	s.syncUnitFromWorld(u)
	if normalized := s.normalizedUnitTypeID(u.TypeID, u.Controller); normalized > 0 {
		u.TypeID = normalized
		s.applyUnitEntityLayout(u)
		return true
	}
	return false
}

func (s *Server) nextUnitID() int32 {
	for {
		var id int32
		if s.ReserveUnitIDFn != nil {
			id = s.ReserveUnitIDFn()
		}
		if id <= 0 {
			s.entityMu.Lock()
			id = s.unitNext
			s.unitNext++
			if s.unitNext <= 0 {
				s.unitNext = 2000000000
			}
			s.entityMu.Unlock()
		}
		if id <= 0 || s.entityIDConflicts(id) {
			continue
		}
		return id
	}
}

func (s *Server) entityIDConflicts(id int32) bool {
	if id <= 0 {
		return true
	}
	s.mu.Lock()
	for c := range s.conns {
		if c != nil && (c.playerID == id || c.unitID == id) {
			s.mu.Unlock()
			return true
		}
	}
	for _, c := range s.pending {
		if c != nil && (c.playerID == id || c.unitID == id) {
			s.mu.Unlock()
			return true
		}
	}
	s.mu.Unlock()

	s.entityMu.Lock()
	_, exists := s.entities[id]
	s.entityMu.Unlock()
	return exists
}

func (s *Server) ensurePlayerUnitEntity(c *Conn) *protocol.UnitEntitySync {
	if c == nil || c.playerID == 0 {
		return nil
	}
	if c.controlBuildActive {
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
	if fallback := s.fallbackPlayerUnitTypeID(); fallback > 0 {
		playerTypeID = fallback
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
			if normalized := s.normalizedUnitTypeID(u.TypeID, u.Controller); normalized > 0 {
				u.TypeID = normalized
			}
			if u.X == 0 && u.Y == 0 {
				u.X = c.snapX
				u.Y = c.snapY
			}
			if s.UnitInfoFn != nil {
				if info, ok := s.UnitInfoFn(c.unitID); ok && info.TeamID != 0 {
					u.TeamID = info.TeamID
				} else {
					u.TeamID = c.TeamID()
				}
			} else {
				u.TeamID = c.TeamID()
			}
			if s.UnitInfoFn == nil {
				u.TypeID = playerTypeID
			} else if info, ok := s.UnitInfoFn(c.unitID); !ok || !s.validUnitTypeID(info.TypeID) {
				u.TypeID = playerTypeID
			}
			s.applyUnitEntityLayout(u)
			u.Elevation = 1
			if u.Health <= 0 {
				u.Health = 100
			}
			return u
		}
	}
	if snapshot := s.authoritativeUnitSnapshot(c.unitID, &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}); snapshot != nil {
		if snapshot.Controller == nil {
			snapshot.Controller = &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}
		}
		if snapshot.X == 0 && snapshot.Y == 0 {
			snapshot.X = c.snapX
			snapshot.Y = c.snapY
		}
		s.entities[c.unitID] = snapshot
		return snapshot
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
		TeamID:         c.TeamID(),
		TypeID:         playerTypeID,
		UpdateBuilding: false,
		Vel:            protocol.Vec2{X: 0, Y: 0},
		X:              c.snapX,
		Y:              c.snapY,
	}
	s.syncUnitFromWorld(u)
	s.applyUnitEntityLayout(u)
	s.entities[c.unitID] = u
	return u
}

func (s *Server) playerUnitEntity(c *Conn) *protocol.UnitEntitySync {
	if c == nil || c.playerID == 0 || c.unitID == 0 || c.controlBuildActive {
		return nil
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	if ent, ok := s.entities[c.unitID]; ok {
		if u, ok2 := ent.(*protocol.UnitEntitySync); ok2 {
			u.Controller = &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}
			s.syncUnitFromWorld(u)
			if normalized := s.normalizedUnitTypeID(u.TypeID, u.Controller); normalized > 0 {
				u.TypeID = normalized
			}
			s.applyUnitEntityLayout(u)
			if u.X == 0 && u.Y == 0 {
				u.X = c.snapX
				u.Y = c.snapY
			}
			return u
		}
	}
	if snapshot := s.authoritativeUnitSnapshot(c.unitID, &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}); snapshot != nil {
		s.entities[c.unitID] = snapshot
		return snapshot
	}
	return nil
}

func (s *Server) snapshotPlayerUnitEntity(c *Conn) *protocol.UnitEntitySync {
	if s == nil || c == nil || c.playerID == 0 || c.unitID == 0 || c.dead || c.controlBuildActive {
		return nil
	}
	var worldInfo UnitInfo
	var hasWorldInfo bool
	if s.UnitInfoFn != nil {
		info, ok := s.UnitInfoFn(c.unitID)
		if !ok || info.Health <= 0 {
			return nil
		}
		if s.normalizedUnitTypeID(info.TypeID, &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}) <= 0 {
			return nil
		}
		worldInfo = info
		hasWorldInfo = true
	}
	s.entityMu.Lock()
	defer s.entityMu.Unlock()
	ent, ok := s.entities[c.unitID]
	var u *protocol.UnitEntitySync
	if ok {
		u, _ = ent.(*protocol.UnitEntitySync)
	}
	if u == nil {
		if snapshot := s.authoritativeUnitSnapshot(c.unitID, &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}); snapshot != nil {
			if snapshot.Controller == nil {
				snapshot.Controller = &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}
			}
			if snapshot.X == 0 && snapshot.Y == 0 {
				snapshot.X = c.snapX
				snapshot.Y = c.snapY
			}
			s.entities[c.unitID] = snapshot
			return snapshot
		}
		if !hasWorldInfo {
			return nil
		}
		typeID := s.normalizedUnitTypeID(worldInfo.TypeID, &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID})
		if typeID <= 0 {
			return nil
		}
		u = &protocol.UnitEntitySync{
			IDValue:        c.unitID,
			Abilities:      []protocol.Ability{},
			Ammo:           0,
			Controller:     &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID},
			Elevation:      1,
			Flag:           0,
			Health:         worldInfo.Health,
			Shooting:       false,
			MineTile:       nil,
			Mounts:         []protocol.WeaponMount{},
			Plans:          []*protocol.BuildPlan{},
			Rotation:       90,
			Shield:         0,
			SpawnedByCore:  true,
			Stack:          protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}, Amount: 0},
			Statuses:       []protocol.StatusEntry{},
			TeamID:         worldInfo.TeamID,
			TypeID:         typeID,
			UpdateBuilding: false,
			Vel:            protocol.Vec2{X: 0, Y: 0},
			X:              worldInfo.X,
			Y:              worldInfo.Y,
		}
		s.entities[c.unitID] = u
	}
	if u.Controller == nil {
		u.Controller = &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}
	}
	s.syncUnitFromWorld(u)
	if normalized := s.normalizedUnitTypeID(u.TypeID, u.Controller); normalized > 0 {
		u.TypeID = normalized
	} else {
		return nil
	}
	s.applyUnitEntityLayout(u)
	if u.X == 0 && u.Y == 0 {
		u.X = c.snapX
		u.Y = c.snapY
	}
	return cloneUnitEntitySync(u)
}

func (s *Server) SetConnUnitStack(c *Conn, itemID int16, amount int32) {
	if s == nil || c == nil || c.playerID == 0 {
		return
	}
	if c.unitID != 0 && s.SetUnitStackFn != nil {
		_ = s.SetUnitStackFn(c.unitID, itemID, amount)
	}
	u := s.playerUnitEntity(c)
	if u == nil && c.unitID != 0 && s.connUnitAlive(c) {
		u = s.ensurePlayerUnitEntity(c)
	}
	if u == nil {
		return
	}
	if amount <= 0 {
		u.Stack.Item = protocol.ItemRef{ItmID: 0, ItmName: ""}
		u.Stack.Amount = 0
		return
	}
	if s.TypeIO != nil && s.TypeIO.ItemLookup != nil {
		if item := s.TypeIO.ItemLookup(itemID); item != nil {
			u.Stack.Item = item
			u.Stack.Amount = amount
			return
		}
	}
	u.Stack.Item = protocol.ItemRef{ItmID: itemID, ItmName: ""}
	u.Stack.Amount = amount
}

func (s *Server) syncUnitFromWorld(u *protocol.UnitEntitySync) {
	if s == nil || u == nil {
		return
	}
	if snapshot := s.authoritativeUnitSnapshot(u.ID(), u.Controller); snapshot != nil {
		*u = *snapshot
		return
	}
	if s.UnitInfoFn == nil {
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
	if normalized := s.normalizedUnitTypeID(info.TypeID, u.Controller); normalized > 0 {
		u.TypeID = normalized
	}
	if info.TeamID != 0 {
		u.TeamID = info.TeamID
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

func (s *Server) BroadcastTileConfig(pos int32, value any, except *Conn) {
	if pos < 0 {
		return
	}
	s.mu.Lock()
	peers := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		if c == nil || c == except || !c.hasConnected {
			continue
		}
		peers = append(peers, c)
	}
	s.mu.Unlock()
	for _, peer := range peers {
		clonedValue, err := protocol.CloneObjectValue(value)
		if err != nil {
			fmt.Printf("[net] skip tileConfig pos=%d err=%v type=%T\n", pos, err, value)
			continue
		}
		packet := &protocol.Remote_InputHandler_tileConfig_90{
			Build: protocol.BuildingBox{PosValue: pos},
			Value: clonedValue,
		}
		_ = peer.SendAsync(packet)
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
	if entity == nil {
		return nil
	}
	w := protocol.NewWriterWithContext(s.TypeIO)
	if err := w.WriteInt32(entity.ID()); err != nil {
		return nil
	}
	if err := w.WriteByte(entity.ClassID()); err != nil {
		return nil
	}
	entity.BeforeWrite()
	if err := entity.WriteSync(w); err != nil {
		return nil
	}
	return append([]byte(nil), w.Bytes()...)
}

// sendWorldHandshake pushes world stream data to the connected client.
func (s *Server) sendWorldHandshake(c *Conn, pkt *protocol.ConnectPacket) error {
	worldData, err := s.WorldDataFn(c, pkt)
	if err != nil {
		return err
	}
	if inspection, ierr := worldstream.InspectWorldStreamPayload(worldData); ierr == nil {
		fmt.Printf("[net] world handshake payload conn=%d player=%d live=%v bytes=%d raw=%d player=%d..%d content=%d..%d patchesEnd=%d mapEnd=%d teamBlocks=%d tail=%d tailPrefix=%s\n",
			c.id, c.playerID, c.UsesLiveWorldStream(), inspection.CompressedLen, inspection.RawLen,
			inspection.PlayerStart, inspection.PlayerEnd, inspection.ContentStart, inspection.ContentEnd,
			inspection.PatchesEnd, inspection.MapEnd, inspection.TeamBlocksLen, inspection.TailLen, inspection.TailPrefixHex)
	} else {
		fmt.Printf("[net] world handshake inspect failed conn=%d player=%d live=%v bytes=%d err=%v\n",
			c.id, c.playerID, c.UsesLiveWorldStream(), len(worldData), ierr)
	}
	worldID, ok := s.Registry.PacketID(&protocol.WorldStream{})
	if !ok {
		return fmt.Errorf("world stream packet id not found")
	}
	return c.SendStream(worldID, worldData)
}

func (s *Server) sendWorldDataBegin(c *Conn) error {
	if c == nil {
		return fmt.Errorf("invalid connection")
	}
	return c.Send(&protocol.Remote_NetClient_worldDataBegin_28{})
}

// SyncWorldToConn re-synchronizes a single connected client with the current world.
// Match vanilla /sync semantics: send worldDataBegin(), then stream the world again.
// Do not clear server-side unit ownership or mark the player dead here.
func (s *Server) SyncWorldToConn(c *Conn) error {
	if s == nil || c == nil || c.playerID == 0 {
		return fmt.Errorf("invalid connection")
	}
	c.syncTime.Store(0)
	c.SetWorldReloadGrace(4 * time.Second)
	c.queueReloadConfirm(false)
	if err := s.sendWorldDataBegin(c); err != nil {
		return err
	}
	if err := s.sendWorldHandshake(c, nil); err != nil {
		return err
	}
	return nil
}

// SyncEntitySnapshotsToConn pushes the current entity snapshot set to one client
// immediately, instead of waiting for the periodic entity snapshot ticker.
func (s *Server) SyncEntitySnapshotsToConn(c *Conn) error {
	if s == nil || c == nil || c.playerID == 0 {
		return fmt.Errorf("invalid connection")
	}
	packets, hiddenIDs, err := s.buildEntitySnapshotPacketsForConn(c)
	if err != nil {
		return err
	}
	for _, packet := range packets {
		if packet == nil {
			continue
		}
		if err := s.sendUnreliable(c, packet); err != nil {
			return err
		}
	}
	return s.sendHiddenSnapshotToConn(c, hiddenIDs)
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

	ready := make([]*Conn, 0, len(peers))
	for _, c := range peers {
		s.assignConnTeam(c, true)
		s.beginWorldHotReload(c)
		if err := s.sendWorldDataBegin(c); err != nil {
			failed++
			s.emitEvent(c, "world_hot_reload_failed", "", err.Error())
			continue
		}
		ready = append(ready, c)
	}

	for _, c := range ready {
		if err := s.sendWorldHandshake(c, nil); err != nil {
			failed++
			s.emitEvent(c, "world_hot_reload_failed", "", err.Error())
			continue
		}
		reloaded++
	}
	return reloaded, failed
}

// ReloadWorldLiveForAllLegacy is a deprecated alias for ReloadWorldLiveForAll.
func (s *Server) ReloadWorldLiveForAllLegacy() (reloaded int, failed int) {
	return s.ReloadWorldLiveForAll()
}

func defaultWorldData(_ *Conn, _ *protocol.ConnectPacket) ([]byte, error) {
	if data, _, err := runtimeassets.LoadBootstrapWorld(""); err == nil {
		return data, nil
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

type PlayerSnapshot struct {
	ID        int32
	Name      string
	UUID      string
	IP        string
	Connected bool
	UnitID    int32
	TeamID    byte
	X         float32
	Y         float32
	Building  bool
	Dead      bool
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
	name := packetTypeName(obj)
	c.statsMu.Lock()
	c.byTypeSent[name]++
	c.byTypeBytes[name] += size
	c.statsMu.Unlock()
}

func packetTypeName(obj any) string {
	if obj == nil {
		return "<nil>"
	}
	return reflect.TypeOf(obj).String()
}

func packetSendDetail(size, packetID, frameworkID int) string {
	var b strings.Builder
	b.Grow(40)
	b.WriteString("size=")
	b.WriteString(strconv.Itoa(size))
	if packetID >= 0 {
		b.WriteString(" packet_id=")
		b.WriteString(strconv.Itoa(packetID))
	}
	if frameworkID >= 0 {
		b.WriteString(" framework_id=")
		b.WriteString(strconv.Itoa(frameworkID))
	}
	return b.String()
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

func (s *Server) ListPlayerSnapshots() []PlayerSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]PlayerSnapshot, 0, len(s.conns))
	for c := range s.conns {
		x, y := c.SnapshotPos()
		out = append(out, PlayerSnapshot{
			ID:        c.id,
			Name:      c.Name(),
			UUID:      c.UUID(),
			IP:        c.remoteIP(),
			Connected: c.hasConnected,
			UnitID:    c.UnitID(),
			TeamID:    c.TeamID(),
			X:         x,
			Y:         y,
			Building:  c.IsBuilding(),
			Dead:      c.IsDead(),
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
	s.noteRecentKick(target.uuid, target.remoteIP())
	_ = target.SendAsync(&protocol.Remote_NetClient_kick_22{Reason: reason})
	s.closeConnLater(target, 250*time.Millisecond)
	return true
}

// KickForMapChange sends the map-change kick string and then closes shortly after
// so clients can auto-reconnect cleanly.
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

// NotifyShutdown sends a disconnect reason to all live connections and closes them shortly after.
// Use blocking sends here so clients have the best chance to display the message before exit.
func (s *Server) NotifyShutdown(reason string, delay time.Duration) int {
	if s == nil {
		return 0
	}
	if reason == "" {
		reason = "server shutting down"
	}
	if delay < 0 {
		delay = 0
	}
	s.mu.Lock()
	targets := make([]*Conn, 0, len(s.conns))
	for c := range s.conns {
		targets = append(targets, c)
	}
	s.mu.Unlock()
	for _, c := range targets {
		if c == nil {
			continue
		}
		_ = c.Send(&protocol.Remote_NetClient_kick_22{Reason: reason})
		s.closeConnLater(c, delay)
	}
	return len(targets)
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

func (s *Server) Shutdown() {
	if s == nil {
		return
	}
	s.shuttingDown.Store(true)

	if s.tcpLn != nil {
		_ = s.tcpLn.Close()
	}
	if s.udpConn != nil {
		_ = s.udpConn.Close()
	}

	s.mu.Lock()
	targets := make([]*Conn, 0, len(s.conns)+len(s.pending))
	for c := range s.conns {
		targets = append(targets, c)
	}
	for _, c := range s.pending {
		targets = append(targets, c)
	}
	s.mu.Unlock()

	for _, c := range targets {
		if c != nil {
			_ = c.Close()
		}
	}
}

func (s *Server) unitControl(c *Conn, unitID int32) {
	if c == nil || c.playerID == 0 || unitID == 0 {
		return
	}
	s.clearConnControlledBuild(c)
	oldUnitID := c.unitID
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
		if snapshot := s.authoritativeUnitSnapshot(unitID, nil); snapshot != nil {
			u = snapshot
			s.entities[unitID] = u
		} else {
			typeID := info.TypeID
			if !s.validUnitTypeID(typeID) {
				typeID = s.fallbackPlayerUnitTypeID()
			}
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
				TypeID:         typeID,
				UpdateBuilding: false,
				Vel:            protocol.Vec2{X: 0, Y: 0},
				X:              info.X,
				Y:              info.Y,
			}
			s.entities[unitID] = u
		}
	}
	// Basic validation: same team and reasonably close.
	if info.TeamID != 0 && info.TeamID != c.TeamID() {
		s.entityMu.Unlock()
		return
	}
	if state, ok := u.Controller.(*protocol.ControllerState); ok && state != nil {
		if state.Type == protocol.ControllerPlayer && state.PlayerID != c.playerID {
			s.entityMu.Unlock()
			return
		}
	}
	u.Controller = &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: c.playerID}
	if normalized := s.normalizedUnitTypeID(u.TypeID, u.Controller); normalized > 0 {
		u.TypeID = normalized
	} else {
		delete(s.entities, unitID)
		s.entityMu.Unlock()
		return
	}
	s.entityMu.Unlock()

	if s.SetUnitPlayerControllerFn != nil && !s.SetUnitPlayerControllerFn(unitID, c.playerID) {
		return
	}
	if oldUnitID != 0 && oldUnitID != unitID {
		s.detachConnUnit(c, oldUnitID)
	}

	c.unitID = unitID
	c.snapX = info.X
	c.snapY = info.Y
	if info.TeamID != 0 {
		c.teamID = info.TeamID
	}
	c.dead = false
	c.deathTimer = 0
	c.lastRespawnCheck = time.Time{}
	c.lastDeadIgnoreAt = time.Time{}
	c.clientDeadIgnores = 0
	c.miningTilePos = invalidTilePos

	var unit protocol.UnitSyncEntity
	if snapshot := s.playerUnitEntity(c); snapshot != nil && s.prepareUnitEntitySnapshot(snapshot) {
		unit = snapshot
	}
	var player protocol.UnitSyncEntity
	if p := s.ensurePlayerEntity(c); p != nil {
		s.updatePlayerEntity(p, c)
		player = p
	}
	_ = s.sendEntitySnapshotToConn(c, player, unit)
	_ = c.SendAsync(&protocol.Remote_NetClient_setPosition_29{X: c.snapX, Y: c.snapY})
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

// isAdmin 检查连接是否是管理员
func (c *Conn) isAdmin() bool {
	if c == nil {
		return false
	}
	// 检查UUID是否在操作员列表中
	server := globalServer
	if server != nil {
		return server.AdminManager != nil && server.AdminManager.IsOp(c.uuid)
	}
	return false
}

// SendChat 发送聊天消息
func (c *Conn) SendChat(message string) error {
	if c == nil {
		return fmt.Errorf("connection is nil")
	}
	return c.SendAsync(makeSendMessagePacket(message, nil))
}

// playerName 返回玩家名称
func (c *Conn) playerName() string {
	if c == nil {
		return ""
	}
	return c.Name()
}
