package core

import (
	"strings"
	"time"

	"mdt-server/internal/config"
	"mdt-server/internal/persist"
)

// ServerCore 协调四个核心的运行
type ServerCore struct {
	Core1      *Core1
	Core2      *Core2
	Core3      *Core3
	Core4      *Core4
	persistCfg config.PersistConfig
	supervisor *coreSupervisor
}

// NewServerCore 创建服务器核心控制器（四核心架构）
func NewServerCore(gameInterval time.Duration, ioConfig Config, persistCfg config.PersistConfig) *ServerCore {
	core2Cfg := ioConfig
	if core2Cfg.Name == "" {
		core2Cfg.Name = "io-core"
	}
	core3Cfg := ioConfig
	core3Cfg.Name = "snapshot-core"
	if core3Cfg.WorkerCount > 2 {
		core3Cfg.WorkerCount = 2
	}
	if core3Cfg.WorkerCount <= 0 {
		core3Cfg.WorkerCount = 1
	}
	core4Cfg := ioConfig
	core4Cfg.Name = "policy-core"
	if core4Cfg.WorkerCount > 2 {
		core4Cfg.WorkerCount = 2
	}
	if core4Cfg.WorkerCount <= 0 {
		core4Cfg.WorkerCount = 1
	}
	sc := &ServerCore{
		Core1:      NewCore1("game-loop"),
		Core2:      NewCore2(core2Cfg),
		Core3:      NewCore3(core3Cfg),
		Core4:      NewCore4(core4Cfg),
		persistCfg: persistCfg,
		supervisor: newCoreSupervisor(),
	}
	sc.Core2.SetServerCore(sc)
	sc.Core3.SetServerCore(sc)
	sc.Core4.SetServerCore(sc)
	return sc
}

// SetGameTickFn 设置 Game Loop 的 tick 函数
func (sc *ServerCore) SetGameTickFn(fn func(tick uint64, delta time.Duration)) {
	sc.Core1.SetTickFn(fn)
}

// StartAll 启动所有异步核心
func (sc *ServerCore) StartAll() {
	// Core1 由外部（主线程）调用 Run() 启动
	sc.Core2.Start()
	sc.Core3.Start()
	sc.Core4.Start()
}

// StopAll 停止所有核心
func (sc *ServerCore) StopAll() {
	if sc == nil {
		return
	}
	if sc.Core1 != nil {
		sc.Core1.Stop()
	}
	if sc.Core2 != nil {
		sc.Core2.Stop()
	}
	if sc.Core3 != nil {
		sc.Core3.Stop()
	}
	if sc.Core4 != nil {
		sc.Core4.Stop()
	}
	if sc.supervisor != nil {
		sc.supervisor.closeAll()
	}
}

// SendToCore2 发送消息到 Core2
func (sc *ServerCore) SendToCore2(msg Message) bool {
	return sc.Core2.Send(msg)
}

func (sc *ServerCore) SendToCore3(msg Message) bool {
	if sc == nil || sc.Core3 == nil {
		return false
	}
	return sc.Core3.Send(msg)
}

func (sc *ServerCore) SendToCore4(msg Message) bool {
	if sc == nil || sc.Core4 == nil {
		return false
	}
	return sc.Core4.Send(msg)
}

// Stats 获取主线程和 Core2 的统计信息，兼容旧调用方。
func (sc *ServerCore) Stats() (core1Running bool, core2Stats [5]int64) {
	core1Running = sc.Core1.running.Load()
	core2Stats[0], core2Stats[1], core2Stats[2], core2Stats[3], core2Stats[4] = sc.Core2.Stats()
	return
}

// StatsAll 获取所有核心的统计信息。
func (sc *ServerCore) StatsAll() map[string][5]int64 {
	out := map[string][5]int64{}
	if sc == nil {
		return out
	}
	if sc.Core2 != nil {
		var stats [5]int64
		stats[0], stats[1], stats[2], stats[3], stats[4] = sc.Core2.Stats()
		out["core2"] = stats
	}
	if sc.Core3 != nil {
		var stats [5]int64
		stats[0], stats[1], stats[2], stats[3], stats[4] = sc.Core3.Stats()
		out["core3"] = stats
	}
	if sc.Core4 != nil {
		var stats [5]int64
		stats[0], stats[1], stats[2], stats[3], stats[4] = sc.Core4.Stats()
		out["core4"] = stats
	}
	return out
}

// GetPersistConfig 获取持久化配置
func (sc *ServerCore) GetPersistConfig() config.PersistConfig {
	return sc.persistCfg
}

// SetPersistStateProvider 设置 Core2 的持久化状态提供器。
func (sc *ServerCore) SetPersistStateProvider(fn func() persist.State) {
	sc.Core2.SetStateProvider(fn)
}

func (sc *ServerCore) EnableChildRoles(exePath string, extraArgs []string, roles ...string) error {
	if sc == nil {
		return nil
	}
	if exePath == "" {
		var err error
		exePath, err = executablePath()
		if err != nil {
			return err
		}
	}
	for _, role := range roles {
		role = strings.ToLower(strings.TrimSpace(role))
		if role == "" {
			continue
		}
		child, err := spawnChildCoreProcess(exePath, role, extraArgs...)
		if err != nil {
			return err
		}
		if sc.supervisor != nil {
			sc.supervisor.add(role, child)
		}
		switch role {
		case "core3":
			if sc.Core3 != nil {
				sc.Core3.AttachRemote(child.Client)
			}
		case "core4":
			if sc.Core4 != nil {
				sc.Core4.AttachRemote(child.Client)
			}
		}
	}
	return nil
}
