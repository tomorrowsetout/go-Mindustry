package net

import (
	"fmt"
	"sync"

	"mdt-server/internal/core"
	"mdt-server/internal/protocol"
	"mdt-server/internal/storage"
)

// NetworkCore 是网络核心（运行在 Core2 的 IO Core 中）
type NetworkCore struct {
	core     *core.Core2
	server   *Server
	connMap  map[int32]*Conn
	connMu   sync.RWMutex
}

// NewNetworkCore 创建网络核心
func NewNetworkCore(server *Server) *NetworkCore {
	cfg := core.Config{
		Name:        "network",
		MessageBuf:  30000,
		WorkerCount: 4,
	}

	return &NetworkCore{
		core:    core.NewCore2(cfg),
		server:  server,
		connMap: make(map[int32]*Conn),
	}
}

// Start 启动网络核心
func (nc *NetworkCore) Start() {
	nc.core.Start()
}

// Stop 停止网络核心
func (nc *NetworkCore) Stop() {
	nc.core.Stop()
}

// SetServerCore 设置 ServerCore 引用
func (nc *NetworkCore) SetServerCore(sc *core.ServerCore) {
	nc.core.SetServerCore(sc)
}

// SetRecorder 设置事件记录器
func (nc *NetworkCore) SetRecorder(rec storage.Recorder) {
	nc.core.SetRecorder(rec)
}

// ProcessPacket 处理数据包（从网络读取）
// 这个方法应该在 IO Core 的 worker 中调用
func (nc *NetworkCore) ProcessPacket(conn *Conn, obj any, err error) {
	if err != nil {
		// 发送连接关闭消息
		nc.core.Send(&core.ConnectionMessage{
			ConnID: conn.id,
			IsOpen: false,
		})
		return
	}

	// 发送包消息到 Core2
	// 注意：这里不直接发送 obj，因为 obj 可能不是 protocol.Packet 类型
	nc.core.Send(&core.PacketMessage{
		ConnID: conn.id,
		Kind:   "incoming",
		Packet: nil, // obj, // 暂时不发送，等待类型转换
		Data:   nil, // 可以序列化包数据
	})
}

// SendPacket 发送数据包（发送到网络）
// 这个方法应该在 IO Core 的 worker 中调用
func (nc *NetworkCore) SendPacket(conn *Conn, packet protocol.Packet) error {
	// 发送包消息到 Core2
	nc.core.Send(&core.PacketMessage{
		ConnID: conn.id,
		Kind:   "outgoing",
		Packet: packet,
		Data:   nil, // 可以序列化包数据
	})
	return nil
}

// ConnectionOpen 处理连接打开
// 这个方法应该在 IO Core 的 worker 中调用
func (nc *NetworkCore) ConnectionOpen(conn *Conn) {
	nc.connMu.Lock()
	nc.connMap[conn.id] = conn
	nc.connMu.Unlock()

	// 发送连接打开消息
	nc.core.Send(&core.ConnectionMessage{
		ConnID: conn.id,
		UserID: conn.playerID,
		UUID:   conn.uuid,
		IP:     conn.RemoteAddr().String(),
		Name:   conn.name,
		IsOpen: true,
	})
}

// ConnectionClose 处理连接关闭
// 这个方法应该在 IO Core 的 worker 中调用
func (nc *NetworkCore) ConnectionClose(conn *Conn) {
	nc.connMu.Lock()
	delete(nc.connMap, conn.id)
	nc.connMu.Unlock()

	// 发送连接关闭消息
	nc.core.Send(&core.ConnectionMessage{
		ConnID: conn.id,
		UserID: conn.playerID,
		UUID:   conn.uuid,
		IP:     conn.RemoteAddr().String(),
		Name:   conn.name,
		IsOpen: false,
	})
}

// GetConnection 获取连接
func (nc *NetworkCore) GetConnection(id int32) *Conn {
	nc.connMu.RLock()
	defer nc.connMu.RUnlock()
	return nc.connMap[id]
}

// GetAllConnections 获取所有连接
func (nc *NetworkCore) GetAllConnections() []*Conn {
	nc.connMu.RLock()
	defer nc.connMu.RUnlock()
	conns := make([]*Conn, 0, len(nc.connMap))
	for _, conn := range nc.connMap {
		conns = append(conns, conn)
	}
	return conns
}

// BroadcastPacket 广播包到所有连接
func (nc *NetworkCore) BroadcastPacket(packet protocol.Packet) {
	conns := nc.GetAllConnections()
	for _, conn := range conns {
		_ = nc.SendPacket(conn, packet)
	}
}

// BroadcastToTeam 广播包到指定队伍
func (nc *NetworkCore) BroadcastToTeam(packet protocol.Packet, teamID byte) {
	conns := nc.GetAllConnections()
	for _, conn := range conns {
		// TODO: 检查队伍ID
		_ = nc.SendPacket(conn, packet)
	}
}

// Stats 获取网络核心统计信息
func (nc *NetworkCore) Stats() (int64, int64, int64, int64, int64) {
	return nc.core.Stats()
}

// SendToCore2 发送消息到 Core2
func (nc *NetworkCore) SendToCore2(msg core.Message) bool {
	return nc.core.Send(msg)
}

// HandlePacketIncoming 处理 incoming 包（在 Core2 worker 中调用）
func (nc *NetworkCore) HandlePacketIncoming(m *core.PacketMessage) {
	conn := nc.GetConnection(m.ConnID)
	if conn == nil {
		fmt.Printf("[NetworkCore] Connection not found: %d\n", m.ConnID)
		return
	}

	// 这里处理 incoming 包
	// 例如：解码包、分发到游戏逻辑等
	fmt.Printf("[NetworkCore] Incoming packet: connID=%d, packet=%T\n",
		m.ConnID, m.Packet)

	// 将包传递给服务器处理
	nc.server.handlePacket(conn, m.Packet, true)
}

// HandlePacketOutgoing 处理 outgoing 包（在 Core2 worker 中调用）
func (nc *NetworkCore) HandlePacketOutgoing(m *core.PacketMessage) {
	conn := nc.GetConnection(m.ConnID)
	if conn == nil {
		fmt.Printf("[NetworkCore] Connection not found: %d\n", m.ConnID)
		return
	}

	// 这里处理 outgoing 包
	// 例如：编码包、发送到网络等
	fmt.Printf("[NetworkCore] Outgoing packet: connID=%d, packet=%T\n",
		m.ConnID, m.Packet)

	// TODO: 发送到网络
	// _ = conn.WriteObject(m.Packet)
}

// HandleConnectionOpen 处理连接打开（在 Core2 worker 中调用）
func (nc *NetworkCore) HandleConnectionOpen(m *core.ConnectionMessage) {
	fmt.Printf("[NetworkCore] Connection opened: connID=%d, UUID=%s, IP=%s\n",
		m.ConnID, m.UUID, m.IP)

	// 这里可以添加连接打开的逻辑
	// 例如：初始化连接状态、记录日志等
}

// HandleConnectionClose 处理连接关闭（在 Core2 worker 中调用）
func (nc *NetworkCore) HandleConnectionClose(m *core.ConnectionMessage) {
	fmt.Printf("[NetworkCore] Connection closed: connID=%d, UUID=%s, IP=%s\n",
		m.ConnID, m.UUID, m.IP)

	// 这里可以添加连接关闭的逻辑
	// 例如：清理连接资源、记录日志等
}

// HandlePersistenceMessage 处理存档操作（在 Core2 worker 中调用）
func (nc *NetworkCore) HandlePersistenceMessage(m *core.PersistenceMessage) {
	switch m.Action {
	case "save_state":
		nc.handleSaveState(m)
	case "load_state":
		nc.handleLoadState(m)
	case "save_world":
		nc.handleSaveWorld(m)
	case "load_world":
		nc.handleLoadWorld(m)
	default:
		fmt.Printf("[NetworkCore] Unknown persistence action: %s\n", m.Action)
	}
}

func (nc *NetworkCore) handleSaveState(m *core.PersistenceMessage) {
	// TODO: 实现存档保存逻辑
	fmt.Printf("[NetworkCore] Saving state: path=%s\n", m.Path)
}

func (nc *NetworkCore) handleLoadState(m *core.PersistenceMessage) {
	// TODO: 实现存档加载逻辑
	fmt.Printf("[NetworkCore] Loading state: path=%s\n", m.Path)
}

func (nc *NetworkCore) handleSaveWorld(m *core.PersistenceMessage) {
	// TODO: 实现世界保存逻辑
	fmt.Printf("[NetworkCore] Saving world: path=%s\n", m.Path)
}

func (nc *NetworkCore) handleLoadWorld(m *core.PersistenceMessage) {
	// TODO: 实现世界加载逻辑
	fmt.Printf("[NetworkCore] Loading world: path=%s\n", m.Path)
}

// HandleStorageMessage 处理存储事件（在 Core2 worker 中调用）
func (nc *NetworkCore) HandleStorageMessage(m *core.StorageMessage) {
	switch m.Action {
	case "record_event":
		nc.handleRecordEvent(m)
	case "record_player":
		nc.handleRecordPlayer(m)
	case "flush":
		nc.handleFlush(m)
	case "close":
		nc.handleClose(m)
	default:
		fmt.Printf("[NetworkCore] Unknown storage action: %s\n", m.Action)
	}
}

func (nc *NetworkCore) handleRecordEvent(m *core.StorageMessage) {
	// TODO: 实现事件记录逻辑
	fmt.Printf("[NetworkCore] Recording event\n")
}

func (nc *NetworkCore) handleRecordPlayer(m *core.StorageMessage) {
	// TODO: 实现玩家事件记录逻辑
	fmt.Printf("[NetworkCore] Recording player event\n")
}

func (nc *NetworkCore) handleFlush(m *core.StorageMessage) {
	// TODO: 实现刷新逻辑
	fmt.Printf("[NetworkCore] Flushing storage\n")
}

func (nc *NetworkCore) handleClose(m *core.StorageMessage) {
	// TODO: 实现关闭记录器逻辑
	fmt.Printf("[NetworkCore] Storage closed\n")
}

// HandleModMessage 处理 Mod 操作（在 Core2 worker 中调用）
func (nc *NetworkCore) HandleModMessage(m *core.ModMessage) {
	switch m.Action {
	case "load":
		nc.handleModLoad(m)
	case "unload":
		nc.handleModUnload(m)
	case "start":
		nc.handleModStart(m)
	case "stop":
		nc.handleModStop(m)
	case "reload":
		nc.handleModReload(m)
	case "scan":
		nc.handleModScan(m)
	default:
		fmt.Printf("[NetworkCore] Unknown mod action: %s\n", m.Action)
	}
}

func (nc *NetworkCore) handleModLoad(m *core.ModMessage) {
	// TODO: 实现 Mod 加载逻辑
	fmt.Printf("[NetworkCore] Loading mod: name=%s, type=%s\n", m.Name, m.ModType)
}

func (nc *NetworkCore) handleModUnload(m *core.ModMessage) {
	// TODO: 实现 Mod 卸载逻辑
	fmt.Printf("[NetworkCore] Unloading mod: name=%s, type=%s\n", m.Name, m.ModType)
}

func (nc *NetworkCore) handleModStart(m *core.ModMessage) {
	// TODO: 实现 Mod 启动逻辑
	fmt.Printf("[NetworkCore] Starting mod: name=%s, type=%s\n", m.Name, m.ModType)
}

func (nc *NetworkCore) handleModStop(m *core.ModMessage) {
	// TODO: 实现 Mod 停止逻辑
	fmt.Printf("[NetworkCore] Stopping mod: name=%s, type=%s\n", m.Name, m.ModType)
}

func (nc *NetworkCore) handleModReload(m *core.ModMessage) {
	// TODO: 实现 Mod 重新加载逻辑
	fmt.Printf("[NetworkCore] Reloading mod: name=%s, type=%s\n", m.Name, m.ModType)
}

func (nc *NetworkCore) handleModScan(m *core.ModMessage) {
	// TODO: 实现 Mod 扫描逻辑
	fmt.Printf("[NetworkCore] Scanning mods directory\n")
}

// HandleWorldStreamMessage 处理 WorldStream 操作（在 Core2 worker 中调用）
func (nc *NetworkCore) HandleWorldStreamMessage(m *core.WorldStreamMessage) {
	switch m.Action {
	case "load_model":
		nc.handleWorldStreamLoadModel(m)
	case "save_snapshot":
		nc.handleWorldStreamSaveSnapshot(m)
	case "rewrite_player":
		nc.handleWorldStreamRewritePlayer(m)
	default:
		fmt.Printf("[NetworkCore] Unknown worldstream action: %s\n", m.Action)
	}
}

func (nc *NetworkCore) handleWorldStreamLoadModel(m *core.WorldStreamMessage) {
	// TODO: 实现从 MSAV 加载世界模型
	fmt.Printf("[NetworkCore] Loading world model: path=%s\n", m.Path)
}

func (nc *NetworkCore) handleWorldStreamSaveSnapshot(m *core.WorldStreamMessage) {
	// TODO: 实现保存世界快照到 MSAV
	fmt.Printf("[NetworkCore] Saving world snapshot: path=%s\n", m.Path)
}

func (nc *NetworkCore) handleWorldStreamRewritePlayer(m *core.WorldStreamMessage) {
	// TODO: 实现重写玩家数据
	fmt.Printf("[NetworkCore] Rewriting player ID %d in world stream: path=%s\n",
		m.PlayerID, m.Path)
}
