package net

import (
	"mdt-server/internal/core"
	"mdt-server/internal/protocol"
)

// BusinessCore 是业务核心（运行在 Core1 的 Game Loop 中）
type BusinessCore struct {
	core *core.Core1
}

// NewBusinessCore 创建业务核心
func NewBusinessCore() *BusinessCore {
	return &BusinessCore{
		core: core.NewCore1("business"),
	}
}

// Start启动业务核心
func (bc *BusinessCore) Start() {
	// Core1 (Game Loop) 由外部启动
}

// Stop停止业务核心
func (bc *BusinessCore) Stop() {
	// Core1 在主线程中自然停止
}

// ProcessBuildPlan处理建筑计划（在 Game Loop 中处理）
func (bc *BusinessCore) ProcessBuildPlan(plans []*protocol.BuildPlan) {
	// 直接在主线程中处理
}

// ProcessChatMessage处理聊天消息（在 Game Loop 中处理）
func (bc *BusinessCore) ProcessChatMessage(username, message string) {
	// 直接在主线程中处理
}

// Stats获取业务核心统计信息
func (bc *BusinessCore) Stats() (recv, processed, dropped, queueSize, latency int64) {
	return 0, 0, 0, 0, 0
}
