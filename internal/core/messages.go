// core/ 消息类型定义
package core

import (
	"time"

	"mdt-server/internal/protocol"
)

// Message is the base type for all core messages
type Message interface {
	Type() MessageType
}

// MessageType is the message type identifier
type MessageType int32

const (
	// Core1 (Game Loop) messages
	MessageGameTick MessageType = iota + 1
	MessageWaveUpdate
	MessageEntityBroadcast
	MessagePlayerUpdate
	MessageBuildQueueProcess
	MessageLogicCompile
	MessageLogicRun

	// Core2 (IO Core) messages
	MessagePacketIncoming
	MessagePacketOutgoing
	MessageConnectionOpen
	MessageConnectionClose
	MessageSaveState
	MessageLoadState
	MessageSaveWorld
	MessageLoadWorld
	MessageModLoad
	MessageModUnload
	MessageModStart
	MessageModStop
	MessageStorageRecord
	MessageStorageFlush
	MessageWorldStreamLoad
	MessageWorldStreamSave
)

// GameTickMessage - Core1: 世界 tick
type GameTickMessage struct {
	Tick       uint64
	Delta      time.Duration
	Wave       int32
	WaveTime   float32
	Time       float32
	Defeated   bool
}

func (m *GameTickMessage) Type() MessageType {
	return MessageGameTick
}

// WaveUpdateMessage - Core1: 波次更新
type WaveUpdateMessage struct {
	Wave       int32
	WaveTime   float32
	NextWaveAt float32
}

func (m *WaveUpdateMessage) Type() MessageType {
	return MessageWaveUpdate
}

// EntityBroadcastMessage - Core1: 实体同步广播
type EntityBroadcastMessage struct {
	Tick       uint64
	Entities   []protocol.UnitSyncEntity
}

func (m *EntityBroadcastMessage) Type() MessageType {
	return MessageEntityBroadcast
}

// PlayerUpdateMessage - Core1: 玩家更新（Game Loop 相关）
type PlayerUpdateMessage struct {
	PlayerID   int32
	UnitID     int32
	Position   protocol.Point2
	Control    bool
}

func (m *PlayerUpdateMessage) Type() MessageType {
	return MessagePlayerUpdate
}

// BuildQueueProcessMessage - Core1: 建筑队列处理
type BuildQueueProcessMessage struct {
	Plans      []*protocol.BuildPlan
	 deadlines []time.Time
}

func (m *BuildQueueProcessMessage) Type() MessageType {
	return MessageBuildQueueProcess
}

// LogicCompileMessage - Core1: 逻辑编译
type LogicCompileMessage struct {
	Source     string
	UserID     int32
}

func (m *LogicCompileMessage) Type() MessageType {
	return MessageLogicCompile
}

// LogicRunMessage - Core1: 逻辑执行
type LogicRunMessage struct {
	ID         int64
	Program    []byte
	Inputs     map[string]int32
}

func (m *LogicRunMessage) Type() MessageType {
	return MessageLogicRun
}

// PacketMessage - Core2: 网络包
type PacketMessage struct {
	ConnID     int32
	Kind       string // "incoming" or "outgoing"
	Packet     protocol.Packet
	Data       []byte
	IdleTime   time.Duration
}

func (m *PacketMessage) Type() MessageType {
	return MessagePacketIncoming
}

// ConnectionMessage - Core2: 连接事件
type ConnectionMessage struct {
	ConnID     int32
	UserID     int32
	UUID       string
	IP         string
	Name       string
	proto      *protocol.ConnectPacket
	TCPAddr    string
	UDPAddr    string
	IsOpen     bool
}

func (m *ConnectionMessage) Type() MessageType {
	if m.IsOpen {
		return MessageConnectionOpen
	}
	return MessageConnectionClose
}

// PersistenceMessage - Core2: 存档操作
type PersistenceMessage struct {
	ID         int64
	Action     string // "save_state", "load_state", "save_world", "load_world"
	Path       string
	StateData  []byte
	WorldData  []byte
	ResultChan chan PersistenceResult
}

func (m *PersistenceMessage) Type() MessageType {
	return MessageSaveState
}

// PersistenceResult - Core2: 存档结果
type PersistenceResult struct {
	StateData  []byte
	WorldData  []byte
	Error      error
}

// ModMessage - Core2: Mod 操作
type ModMessage struct {
	ID         int64
	Action     string // "load", "unload", "start", "stop", "reload", "scan"
	Path       string
	Name       string
	ModType    string // "java", "js", "go", "node"
	ResultChan chan ModResult
}

func (m *ModMessage) Type() MessageType {
	return MessageModLoad
}

// ModResult - Core2: Mod 结果
type ModResult struct {
	ID         int64
	Success    bool
	ModID      int64
	Name       string
	Error      error
}

// StorageMessage - Core2: 存储事件
type StorageMessage struct {
	ID         int64
	Action     string // "record_event", "record_player", "flush", "close"
	EventData  []byte
	PlayerPath string
}

func (m *StorageMessage) Type() MessageType {
	return MessageStorageRecord
}

// WorldStreamMessage - Core2: 世界流操作
type WorldStreamMessage struct {
	ID         int64
	Action     string // "load_model", "save_snapshot", "rewrite_player"
	Path       string
	PlayerID   int32
	Tags       map[string]string
	ModelData  []byte
}

func (m *WorldStreamMessage) Type() MessageType {
	return MessageWorldStreamLoad
}

