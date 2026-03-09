package main

// 双核心架构集成示例
// 这个文件展示了如何将双核心架构集成到服务器中

import (
	"fmt"
	"time"

	"mdt-server/internal/core"
	"mdt-server/internal/config"
)

// 示例：创建 ServerCore
func exampleCreateServerCore() {
	// 1. 创建 IO Core 配置
	ioConfig := core.Config{
		Name:        "io-core",
		MessageBuf:  30000, // 大缓冲区用于 IO 操作
		WorkerCount: 4,     // 4 个 worker goroutine
	}

	// 2. 创建持久化配置
	persistCfg := config.PersistConfig{
		Enabled:     true,
		Directory:   "data/state",
		File:        "server-state.json",
		IntervalSec: 30,
		SaveMSAV:    true,
		MSAVDir:     "data/snapshots",
	}

	// 3. 创建 ServerCore
	// Core1: 60 TPS (16.67ms per tick)
	// Core2: IO Core with 4 workers
	serverCore := core.NewServerCore(
		16666666*time.Nanosecond, // 60 TPS
		ioConfig,
		persistCfg,
	)

	// 4. 设置 Game Loop 回调
	serverCore.SetGameTickFn(func(tick uint64, delta time.Duration) {
		fmt.Printf("Game Tick: %d, Delta: %v\n", tick, delta)
		// 这里执行游戏逻辑：
		// - wld.Step(delta)
		// - 广播实体同步
		// - 更新波次
		// - 处理玩家更新
	})

	// 5. 启动 Core2
	serverCore.StartAll()
	fmt.Println("Core2 started")

	// 6. 在主线程启动 Core1
	go func() {
		gameInterval := 16666666 * time.Nanosecond // 60 TPS
		serverCore.Core1.Run(gameInterval)
	}()
	fmt.Println("Core1 started in goroutine")

	// 7. 发送消息到 Core2
	// 保存状态
	serverCore.SendToCore2(&core.PersistenceMessage{
		Action: "save_state",
		Path:   "data/state/server-state.json",
	})

	// 加载世界模型
	serverCore.SendToCore2(&core.WorldStreamMessage{
		Action: "load_model",
		Path:   "maps/my-map.msav",
	})

	// 记录事件
	serverCore.SendToCore2(&core.StorageMessage{
		Action:    "record_event",
		EventData: []byte(`{"event":"player_connect","player_id":1}`),
	})

	// 8. 获取统计信息
	running, stats := serverCore.Stats()
	fmt.Printf("Core1 running: %v\n", running)
	fmt.Printf("Core2 stats: received=%d, processed=%d, dropped=%d, queue=%d, latency=%dms\n",
		stats[0], stats[1], stats[2], stats[3], stats[4])

	// 9. 停止核心
	time.Sleep(100 * time.Millisecond) // 运行一会儿
	serverCore.StopAll()
	fmt.Println("All cores stopped")
}

// 示例：使用 ResultChan 处理异步结果
func exampleAsyncResult() {
	ioConfig := core.Config{
		Name:        "io-core",
		MessageBuf:  30000,
		WorkerCount: 4,
	}

	persistCfg := config.PersistConfig{
		Enabled:   true,
		Directory: "data/state",
		File:      "server-state.json",
	}

	serverCore := core.NewServerCore(
		16666666*time.Nanosecond,
		ioConfig,
		persistCfg,
	)

	serverCore.StartAll()
	defer serverCore.StopAll()

	// 发送异步消息并等待结果
	resultChan := make(chan core.PersistenceResult, 1)

	serverCore.SendToCore2(&core.PersistenceMessage{
		Action:     "load_state",
		Path:       "data/state/server-state.json",
		ResultChan: resultChan,
	})

	// 接收结果（带超时）
	select {
	case result := <-resultChan:
		if result.Error != nil {
			fmt.Printf("Load state failed: %v\n", result.Error)
		} else {
			fmt.Printf("Load state success: %s\n", string(result.StateData))
		}
	case <-time.After(2 * time.Second):
		fmt.Println("Load state timeout")
	}
}

// 示例：监控 Core2 队列
func exampleMonitorCore2() {
	ioConfig := core.Config{
		Name:        "io-core",
		MessageBuf:  30000,
		WorkerCount: 4,
	}

	persistCfg := config.PersistConfig{
		Enabled:   true,
		Directory: "data/state",
		File:      "server-state.json",
	}

	serverCore := core.NewServerCore(
		16666666*time.Nanosecond,
		ioConfig,
		persistCfg,
	)

	serverCore.StartAll()
	defer serverCore.StopAll()

	// 定期检查 Core2 队列
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			_, stats := serverCore.Stats()
			queueSize := stats[3]

			if queueSize > 10000 {
				fmt.Printf("WARNING: Core2 queue is too large: %d\n", queueSize)
			} else if queueSize > 5000 {
				fmt.Printf("INFO: Core2 queue size: %d\n", queueSize)
			}
		}
	}()

	// 模拟发送大量消息
	for i := 0; i < 100; i++ {
		if !serverCore.SendToCore2(&core.StorageMessage{
			Action:    "record_event",
			EventData: []byte(fmt.Sprintf(`{"id":%d}`, i)),
		}) {
			fmt.Printf("Failed to send message %d\n", i)
		}
	}
}

// 示例：集成到现有服务器
func exampleIntegration() {
	// 这是一个集成到现有服务器的示例结构

	// 1. 创建 ServerCore
	ioConfig := core.Config{
		Name:        "io-core",
		MessageBuf:  30000,
		WorkerCount: 4,
	}

	persistCfg := config.PersistConfig{
		Enabled:     true,
		Directory:   "data/state",
		File:        "server-state.json",
		IntervalSec: 30,
	}

	serverCore := core.NewServerCore(
		16666666*time.Nanosecond,
		ioConfig,
		persistCfg,
	)

	// 2. 设置 Game Loop（替换原有的 wld.Step 调用）
	serverCore.SetGameTickFn(func(tick uint64, delta time.Duration) {
		// 原来的游戏逻辑
		// wld.Step(delta)

		// 可选：发送消息到 Core2
		serverCore.SendToCore2(&core.EntityBroadcastMessage{
			Tick:     tick,
			Entities: nil, // 实体同步数据
		})
	})

	// 3. 启动 Core2
	serverCore.StartAll()

	// 4. 在主线程启动 Core1
	go func() {
		gameInterval := 16666666 * time.Nanosecond
		serverCore.Core1.Run(gameInterval)
	}()

	// 5. 替换同步的 IO 操作为异步消息
	// 原来的代码：
	// err := persist.Save(cfg.Persist, state)
	// 改为：
	// resultChan := make(chan core.PersistenceResult, 1)
	// serverCore.SendToCore2(&core.PersistenceMessage{
	//     Action:     "save_state",
	//     Path:       statePath,
	//     ResultChan: resultChan,
	// })
	// result := <-resultChan

	// 6. 定期保存
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()

		for range ticker.C {
			serverCore.SendToCore2(&core.PersistenceMessage{
				Action: "save_state",
				Path:   "data/state/server-state.json",
			})
		}
	}()
}

func main() {
	fmt.Println("=== 双核心架构示例 ===")
	fmt.Println()

	fmt.Println("示例 1: 创建 ServerCore")
	exampleCreateServerCore()
	fmt.Println()

	fmt.Println("示例 2: 异步结果处理")
	exampleAsyncResult()
	fmt.Println()

	fmt.Println("示例 3: 监控 Core2 队列")
	exampleMonitorCore2()
	fmt.Println()

	fmt.Println("示例 4: 集成到现有服务器")
	exampleIntegration()
	fmt.Println()

	fmt.Println("所有示例运行完成！")
}
