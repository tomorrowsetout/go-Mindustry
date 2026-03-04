package net

import (
	"mdt-server/internal/core"
	"mdt-server/internal/protocol"
)

// NetworkCore 是网络核心（运行在 Core2 的 IO Core 中）
type NetworkCore struct {
	core *core.Core2
	server *Server
}

// NewNetworkCore创建网络核心
func NewNetworkCore(server *Server) *NetworkCore {
	cfg := core.Config{
		Name:        "network",
		MessageBuf:  30000,
		WorkerCount: 4,
	}

	return &NetworkCore{
		core:   core.NewCore2(cfg),
		server: server,
	}
}

// Start启动网络核心
func (nc *NetworkCore) Start() {
	nc.core.Start()
}

// Stop停止网络核心
func (nc *NetworkCore) Stop() {
	nc.core.Stop()
}

// ProcessPacket处理数据包（从网络读取）
func (nc *NetworkCore) ProcessPacket(conn *Conn, obj any, err error) {
	// 这里处理读取包
}

// SendPacket发送数据包（发送到网络）
func (nc *NetworkCore) SendPacket(conn *Conn, packet protocol.Packet) error {
	return nil
}

// ConnectionOpen处理连接打开
func (nc *NetworkCore) ConnectionOpen(conn *Conn) {
	// 这里处理连接打开
}

// ConnectionClose处理连接关闭
func (nc *NetworkCore) ConnectionClose(conn *Conn) {
	// 这里处理连接关闭
}
