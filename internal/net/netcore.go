package net

import (
	"errors"
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
	packetMu sync.Mutex
}

// NewNetworkCore 创建网络核心
func NewNetworkCore(server *Server) *NetworkCore {
	cfg := core.Config{
		Name:        "network",
		MessageBuf:  30000,
		WorkerCount: 4,
	}

	return NewNetworkCoreWithCore(server, core.NewCore2(cfg))
}

// NewNetworkCoreWithCore 使用外部 Core2 创建网络核心。
func NewNetworkCoreWithCore(server *Server, ioCore *core.Core2) *NetworkCore {
	if ioCore == nil {
		ioCore = core.NewCore2(core.Config{
			Name:        "network",
			MessageBuf:  30000,
			WorkerCount: 4,
		})
	}
	nc := &NetworkCore{
		core:    ioCore,
		server:  server,
		connMap: make(map[int32]*Conn),
	}
	nc.core.SetConnectionHandlers(nc.HandleConnectionOpen, nc.HandleConnectionClose)
	nc.core.SetPacketHandlers(nc.HandlePacketIncoming, nc.HandlePacketOutgoing)
	return nc
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
func (nc *NetworkCore) ProcessPacket(conn *Conn, obj any, err error) {
	if conn == nil {
		return
	}
	if err != nil {
		nc.core.Send(&core.ConnectionMessage{
			ConnID:  conn.id,
			UserID:  conn.playerID,
			UUID:    conn.uuid,
			IP:      safeRemoteAddr(conn),
			Name:    conn.name,
			TCPAddr: safeRemoteAddr(conn),
			UDPAddr: safeUDPAddr(conn),
			IsOpen:  false,
		})
		return
	}

	var pkt protocol.Packet
	if p, ok := obj.(protocol.Packet); ok {
		pkt = p
	}

	nc.core.Send(&core.PacketMessage{
		ConnID: conn.id,
		Kind:   "incoming",
		Packet: pkt,
	})
}

// SendPacket 发送数据包（发送到网络）
func (nc *NetworkCore) SendPacket(conn *Conn, packet protocol.Packet) error {
	if conn == nil {
		return errors.New("nil connection")
	}
	if packet == nil {
		return errors.New("nil packet")
	}
	ok := nc.core.Send(&core.PacketMessage{
		ConnID: conn.id,
		Kind:   "outgoing",
		Packet: packet,
	})
	if !ok {
		return errors.New("network core queue is full")
	}
	return nil
}

// ConnectionOpen 处理连接打开
func (nc *NetworkCore) ConnectionOpen(conn *Conn) {
	if conn == nil {
		return
	}
	nc.connMu.Lock()
	nc.connMap[conn.id] = conn
	nc.connMu.Unlock()

	nc.core.Send(&core.ConnectionMessage{
		ConnID:  conn.id,
		UserID:  conn.playerID,
		UUID:    conn.uuid,
		IP:      safeRemoteAddr(conn),
		Name:    conn.name,
		TCPAddr: safeRemoteAddr(conn),
		UDPAddr: safeUDPAddr(conn),
		IsOpen:  true,
	})
}

// ConnectionClose 处理连接关闭
func (nc *NetworkCore) ConnectionClose(conn *Conn) {
	if conn == nil {
		return
	}
	nc.connMu.Lock()
	delete(nc.connMap, conn.id)
	nc.connMu.Unlock()

	nc.core.Send(&core.ConnectionMessage{
		ConnID:  conn.id,
		UserID:  conn.playerID,
		UUID:    conn.uuid,
		IP:      safeRemoteAddr(conn),
		Name:    conn.name,
		TCPAddr: safeRemoteAddr(conn),
		UDPAddr: safeUDPAddr(conn),
		IsOpen:  false,
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
	_ = teamID
	for _, conn := range conns {
		if conn == nil {
			continue
		}
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
	nc.packetMu.Lock()
	defer nc.packetMu.Unlock()
	conn := nc.GetConnection(m.ConnID)
	if conn == nil || m == nil || m.Packet == nil {
		return
	}
	nc.server.handlePacket(conn, m.Packet, true)
}

// HandlePacketOutgoing 处理 outgoing 包（在 Core2 worker 中调用）
func (nc *NetworkCore) HandlePacketOutgoing(m *core.PacketMessage) {
	nc.packetMu.Lock()
	defer nc.packetMu.Unlock()
	conn := nc.GetConnection(m.ConnID)
	if conn == nil || m == nil || m.Packet == nil {
		return
	}
	if err := conn.Send(m.Packet); err != nil {
		nc.core.Send(&core.ConnectionMessage{
			ConnID:  conn.id,
			UserID:  conn.playerID,
			UUID:    conn.uuid,
			IP:      safeRemoteAddr(conn),
			Name:    conn.name,
			TCPAddr: safeRemoteAddr(conn),
			UDPAddr: safeUDPAddr(conn),
			IsOpen:  false,
		})
	}
}

// HandleConnectionOpen 处理连接打开（在 Core2 worker 中调用）
func (nc *NetworkCore) HandleConnectionOpen(m *core.ConnectionMessage) {
	if m == nil {
		return
	}
	if nc.server != nil && nc.server.VerboseNetLogEnabled() {
		fmt.Printf("[NetworkCore] Connection opened: connID=%d, UUID=%s, IP=%s\n", m.ConnID, m.UUID, m.IP)
	}
}

// HandleConnectionClose 处理连接关闭（在 Core2 worker 中调用）
func (nc *NetworkCore) HandleConnectionClose(m *core.ConnectionMessage) {
	if m == nil {
		return
	}
	if nc.server != nil && nc.server.VerboseNetLogEnabled() {
		fmt.Printf("[NetworkCore] Connection closed: connID=%d, UUID=%s, IP=%s\n", m.ConnID, m.UUID, m.IP)
	}
}

// HandlePersistenceMessage 处理存档操作（在 Core2 worker 中调用）
func (nc *NetworkCore) HandlePersistenceMessage(m *core.PersistenceMessage) {
	if m == nil {
		return
	}
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
		if m.ResultChan != nil {
			m.ResultChan <- core.PersistenceResult{Error: fmt.Errorf("unknown persistence action: %s", m.Action)}
		}
	}
}

func (nc *NetworkCore) handleSaveState(m *core.PersistenceMessage) {
	if m.ResultChan != nil {
		m.ResultChan <- core.PersistenceResult{
			Error: errors.New("save_state should be handled by core persistence pipeline"),
		}
	}
}

func (nc *NetworkCore) handleLoadState(m *core.PersistenceMessage) {
	if m.ResultChan != nil {
		m.ResultChan <- core.PersistenceResult{
			Error: errors.New("load_state should be handled by core persistence pipeline"),
		}
	}
}

func (nc *NetworkCore) handleSaveWorld(m *core.PersistenceMessage) {
	if m.ResultChan != nil {
		m.ResultChan <- core.PersistenceResult{
			Error: errors.New("save_world should be handled by core persistence pipeline"),
		}
	}
}

func (nc *NetworkCore) handleLoadWorld(m *core.PersistenceMessage) {
	if m.ResultChan != nil {
		m.ResultChan <- core.PersistenceResult{
			Error: errors.New("load_world should be handled by core persistence pipeline"),
		}
	}
}

// HandleStorageMessage 处理存储事件（在 Core2 worker 中调用）
func (nc *NetworkCore) HandleStorageMessage(m *core.StorageMessage) {
	switch m.Action {
	case "record_event", "record_player", "flush", "close":
		nc.core.Send(m)
	default:
		fmt.Printf("[NetworkCore] Unknown storage action: %s\n", m.Action)
	}
}

// HandleModMessage 处理 Mod 操作（在 Core2 worker 中调用）
func (nc *NetworkCore) HandleModMessage(m *core.ModMessage) {
	if m == nil || m.ResultChan == nil {
		return
	}
	m.ResultChan <- core.ModResult{
		ID:      m.ID,
		Success: false,
		Name:    m.Name,
		Error:   errors.New("mod management should be handled by core mod pipeline"),
	}
}

// HandleWorldStreamMessage 处理 WorldStream 操作（在 Core2 worker 中调用）
func (nc *NetworkCore) HandleWorldStreamMessage(m *core.WorldStreamMessage) {
	if m == nil {
		return
	}
	// WorldStream read/write/rewrite currently belongs to core pipeline.
	fmt.Printf("[NetworkCore] WorldStream action delegated to core: action=%s path=%s playerID=%d\n", m.Action, m.Path, m.PlayerID)
}

func safeRemoteAddr(conn *Conn) string {
	if conn == nil || conn.Conn == nil || conn.RemoteAddr() == nil {
		return ""
	}
	return conn.RemoteAddr().String()
}

func safeUDPAddr(conn *Conn) string {
	if conn == nil || conn.UDPAddr() == nil {
		return ""
	}
	return conn.UDPAddr().String()
}
