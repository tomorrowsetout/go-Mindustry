package net

import (
	"encoding/json"
	"fmt"
	"os"
	"sync"

	"mdt-server/internal/core"
	"mdt-server/internal/persist"
	"mdt-server/internal/protocol"
	"mdt-server/internal/storage"
	"mdt-server/internal/worldstream"
)

// NetworkCore 是网络核心（运行在 Core2 的 IO Core 中）
type NetworkCore struct {
	core       *core.Core2
	server     *Server
	serverCore *core.ServerCore
	recorder   storage.Recorder
	connMap    map[int32]*Conn
	connMu     sync.RWMutex
	modMu      sync.RWMutex
	modHandler func(action string, msg *core.ModMessage) core.ModResult
}

// NewNetworkCore 创建网络核心
func NewNetworkCore(server *Server) *NetworkCore {
	cfg := core.Config{
		Name:        "network",
		MessageBuf:  30000,
		WorkerCount: 4,
	}

	nc := &NetworkCore{
		core:    core.NewCore2(cfg),
		server:  server,
		connMap: make(map[int32]*Conn),
	}
	nc.core.SetPacketHandlers(nc.HandlePacketIncoming, nc.HandlePacketOutgoing)
	nc.core.SetConnectionHandlers(nc.HandleConnectionOpen, nc.HandleConnectionClose)
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
	nc.serverCore = sc
	nc.core.SetServerCore(sc)
}

// SetRecorder 设置事件记录器
func (nc *NetworkCore) SetRecorder(rec storage.Recorder) {
	nc.recorder = rec
	nc.core.SetRecorder(rec)
}

// SetModHandler sets an optional mod handler for load/unload/start/stop/reload/scan.
func (nc *NetworkCore) SetModHandler(fn func(action string, msg *core.ModMessage) core.ModResult) {
	nc.modMu.Lock()
	nc.modHandler = fn
	nc.modMu.Unlock()
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
	nc.core.Send(&core.PacketMessage{
		ConnID: conn.id,
		Kind:   "incoming",
		Packet: obj,
		Data:   nil,
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
		Data:   nil,
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
		if conn != nil && conn.teamID != teamID {
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

	if m.Packet == nil {
		return
	}
	if err := conn.Send(m.Packet); err != nil {
		fmt.Printf("[NetworkCore] send failed connID=%d err=%v\n", m.ConnID, err)
	}
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
	result := core.PersistenceResult{}
	if nc.serverCore == nil {
		result.Error = fmt.Errorf("server core not initialized")
	} else {
		state := persist.State{}
		if len(m.StateData) > 0 {
			if err := json.Unmarshal(m.StateData, &state); err != nil {
				result.Error = err
			}
		}
		if result.Error == nil && state.MapPath == "" && m.Path != "" {
			state.MapPath = m.Path
		}
		if result.Error == nil {
			if err := persist.Save(nc.serverCore.GetPersistConfig(), state); err != nil {
				result.Error = err
			}
		}
	}
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) handleLoadState(m *core.PersistenceMessage) {
	result := core.PersistenceResult{}
	if nc.serverCore == nil {
		result.Error = fmt.Errorf("server core not initialized")
	} else {
		st, ok, err := persist.Load(nc.serverCore.GetPersistConfig())
		if err != nil {
			result.Error = err
		} else if ok {
			if data, err := json.Marshal(st); err == nil {
				result.StateData = data
			} else {
				result.Error = err
			}
		}
	}
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) handleSaveWorld(m *core.PersistenceMessage) {
	result := core.PersistenceResult{}
	if m.Path == "" {
		result.Error = fmt.Errorf("world path is empty")
	} else if len(m.WorldData) == 0 {
		result.Error = fmt.Errorf("world data is empty")
	} else if err := os.WriteFile(m.Path, m.WorldData, 0644); err != nil {
		result.Error = err
	}
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) handleLoadWorld(m *core.PersistenceMessage) {
	result := core.PersistenceResult{}
	if m.Path == "" {
		result.Error = fmt.Errorf("world path is empty")
	} else if data, err := os.ReadFile(m.Path); err != nil {
		result.Error = err
	} else {
		result.WorldData = data
	}
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
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
	if nc.recorder == nil {
		return
	}
	var ev storage.Event
	if len(m.EventData) > 0 {
		_ = json.Unmarshal(m.EventData, &ev)
	}
	_ = nc.recorder.Record(ev)
}

func (nc *NetworkCore) handleRecordPlayer(m *core.StorageMessage) {
	if nc.recorder == nil {
		return
	}
	var ev storage.Event
	if len(m.EventData) > 0 {
		_ = json.Unmarshal(m.EventData, &ev)
	}
	_ = nc.recorder.Record(ev)
}

func (nc *NetworkCore) handleFlush(m *core.StorageMessage) {
	if nc.recorder == nil {
		return
	}
	if fl, ok := nc.recorder.(storage.Flusher); ok {
		_ = fl.Flush()
	}
}

func (nc *NetworkCore) handleClose(m *core.StorageMessage) {
	if nc.recorder == nil {
		return
	}
	_ = nc.recorder.Close()
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
	result := nc.runModAction("load", m)
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) handleModUnload(m *core.ModMessage) {
	result := nc.runModAction("unload", m)
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) handleModStart(m *core.ModMessage) {
	result := nc.runModAction("start", m)
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) handleModStop(m *core.ModMessage) {
	result := nc.runModAction("stop", m)
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) handleModReload(m *core.ModMessage) {
	result := nc.runModAction("reload", m)
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) handleModScan(m *core.ModMessage) {
	result := nc.runModAction("scan", m)
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) runModAction(action string, m *core.ModMessage) core.ModResult {
	nc.modMu.RLock()
	handler := nc.modHandler
	nc.modMu.RUnlock()
	if handler == nil {
		err := fmt.Errorf("mod handler not configured")
		return core.ModResult{ID: m.ID, Name: m.Name, Success: false, Error: err}
	}
	out := handler(action, m)
	if out.ID == 0 {
		out.ID = m.ID
	}
	if out.Name == "" {
		out.Name = m.Name
	}
	return out
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
	result := core.WorldStreamResult{}
	if m.Path == "" {
		result.Error = fmt.Errorf("worldstream load path is empty")
	} else if data, err := worldstream.BuildWorldStreamFromMSAV(m.Path); err != nil {
		result.Error = err
	} else {
		result.WorldData = data
	}
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) handleWorldStreamSaveSnapshot(m *core.WorldStreamMessage) {
	result := core.WorldStreamResult{}
	if m.Path == "" {
		result.Error = fmt.Errorf("worldstream save path is empty")
	} else if len(m.ModelData) > 0 {
		if err := os.WriteFile(m.Path, m.ModelData, 0644); err != nil {
			result.Error = err
		}
	} else if len(m.Tags) > 0 {
		if err := worldstream.WriteMSAVSnapshot(m.Path, m.Path, m.Tags); err != nil {
			result.Error = err
		}
	} else {
		result.Error = fmt.Errorf("worldstream save has no model data or tags")
	}
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

func (nc *NetworkCore) handleWorldStreamRewritePlayer(m *core.WorldStreamMessage) {
	result := core.WorldStreamResult{}
	if len(m.ModelData) == 0 {
		result.Error = fmt.Errorf("worldstream data is empty")
	} else if out, err := worldstream.RewritePlayerIDInWorldStream(m.ModelData, m.PlayerID); err != nil {
		result.Error = err
	} else {
		result.WorldData = out
	}
	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}
