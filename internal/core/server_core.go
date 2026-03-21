package core

import (
	"time"

	"mdt-server/internal/config"
	"mdt-server/internal/persist"
)

// ServerCore 协调两个核心的运行
type ServerCore struct {
	Core1      *Core1
	Core2      *Core2
	persistCfg config.PersistConfig
}

// NewServerCore 创建服务器核心控制器（两核心架构）
func NewServerCore(gameInterval time.Duration, ioConfig Config, persistCfg config.PersistConfig) *ServerCore {
	sc := &ServerCore{
		Core1:      NewCore1("game-loop"),
		Core2:      NewCore2(ioConfig),
		persistCfg: persistCfg,
	}
	sc.Core2.SetServerCore(sc)
	return sc
}

// SetGameTickFn 设置 Game Loop 的 tick 函数
func (sc *ServerCore) SetGameTickFn(fn func(tick uint64, delta time.Duration)) {
	sc.Core1.SetTickFn(fn)
}

// StartAll 启动所有核心
func (sc *ServerCore) StartAll() {
	// Core1 由外部（主线程）调用 Run() 启动
	// Core2 自动在 Start() 中启动 goroutine
	sc.Core2.Start()
}

// StopAll 停止所有核心
func (sc *ServerCore) StopAll() {
	sc.Core2.Stop()
}

// SendToCore2 发送消息到 Core2
func (sc *ServerCore) SendToCore2(msg Message) bool {
	return sc.Core2.Send(msg)
}

// Stats 获取所有核心的统计信息
func (sc *ServerCore) Stats() (core1Running bool, core2Stats [5]int64) {
	core1Running = sc.Core1.running.Load()
	core2Stats[0], core2Stats[1], core2Stats[2], core2Stats[3], core2Stats[4] = sc.Core2.Stats()
	return
}

// GetPersistConfig 获取持久化配置
func (sc *ServerCore) GetPersistConfig() config.PersistConfig {
	return sc.persistCfg
}

// SetPersistStateProvider 设置 Core2 的持久化状态提供器。
func (sc *ServerCore) SetPersistStateProvider(fn func() persist.State) {
	sc.Core2.SetStateProvider(fn)
}
