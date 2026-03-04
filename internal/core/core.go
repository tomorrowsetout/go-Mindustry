package core

import (
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
)

// Core1 - Game Loop 核心（主线程）
// 必须运行在单个核心上，处理 60 TPS 的实时游戏逻辑
type Core1 struct {
	name      string
	config    Config
	running   atomic.Bool
	tickFn    func(tick uint64, delta time.Duration)
}

// Core2 - IO Core（第二核心）
// 处理所有 IO 密集型任务：网络、存档、Mod、Storage、WorldStream
type Core2 struct {
	name      string
	messages  chan Message
	workerCount int
	wg        sync.WaitGroup
	running   atomic.Bool
	stats     *Stats
}

// Config 是核心配置（简化版，只用于Core2）
type Config struct {
	Name        string
	MessageBuf  int
	WorkerCount int
}

// Stats 是核心统计信息
type Stats struct {
	Received   int64
	Processed  int64
	Dropped    int64
	QueueSize  int64
	LatencyMs  int64
	lastUpdate int64
	mu         sync.Mutex
}

// AddReceived 增加接收数量
func (s *Stats) AddReceived(n int64) {
	s.mu.Lock()
	s.Received += n
	s.mu.Unlock()
}

// AddProcessed 增加处理数量
func (s *Stats) AddProcessed(n int64) {
	s.mu.Lock()
	s.Processed += n
	s.mu.Unlock()
}

// AddDropped 增加丢弃数量
func (s *Stats) AddDropped(n int64) {
	s.mu.Lock()
	s.Dropped += n
	s.mu.Unlock()
}

// AddQueueSize 增加队列大小
func (s *Stats) AddQueueSize(n int64) {
	s.mu.Lock()
	s.QueueSize += n
	s.mu.Unlock()
}

// SetLatency 设置延迟
func (s *Stats) SetLatency(ms int64) {
	s.mu.Lock()
	s.LatencyMs = ms
	s.mu.Unlock()
}

// GetStats 获取统计信息
func (s *Stats) GetStats() (int64, int64, int64, int64, int64) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.Received, s.Processed, s.Dropped, s.QueueSize, s.LatencyMs
}

// NewCore1 创建 Game Loop 核心
func NewCore1(name string) *Core1 {
	if name == "" {
		name = "game-loop"
	}
	return &Core1{
		name: name,
	}
}

// NewCore2 创建 IO Core
func NewCore2(cfg Config) *Core2 {
	if cfg.MessageBuf <= 0 {
		cfg.MessageBuf = 30000 // IO Core 需要更大的缓冲
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 4 // IO Core 的 worker 数量
		if runtime.NumCPU() < 4 {
			cfg.WorkerCount = 2
		}
	}

	return &Core2{
		name:        cfg.Name,
		messages:    make(chan Message, cfg.MessageBuf),
		workerCount: cfg.WorkerCount,
		stats: &Stats{
			lastUpdate: time.Now().UnixNano(),
		},
	}
}

// SetTickFn 设置 Game Loop 的 tick 函数
func (c1 *Core1) SetTickFn(fn func(tick uint64, delta time.Duration)) {
	c1.tickFn = fn
}

// Run 运行 Game Loop（主线程）
func (c1 *Core1) Run(interval time.Duration) {
	if !c1.running.Swap(true) {
		c1.runGameLoop(interval)
	}
}

// runGameLoop 运行游戏循环（主线程）
func (c1 *Core1) runGameLoop(interval time.Duration) {
	tickCount := uint64(0)
	next := time.Now().Add(interval)

	for {
		now := time.Now()
		if now.Before(next) {
			// 短暂休眠直到下一个 tick
			time.Sleep(next.Sub(now))
			continue
		}

		// 执行 tick
		if c1.tickFn != nil {
			c1.tickFn(tickCount, interval)
		}

		tickCount++
		next = next.Add(interval)
	}
}

// Start 启动 IO Core
func (c2 *Core2) Start() {
	if !c2.running.Swap(true) {
		c2.wg.Add(c2.workerCount)
		for i := 0; i < c2.workerCount; i++ {
			go c2.worker(i)
		}
	}
}

// worker IO Core 的工作协程
func (c2 *Core2) worker(id int) {
	defer c2.wg.Done()

	for msg := range c2.messages {
		c2.stats.AddReceived(1)
		start := time.Now()

		c2.handleMessage(msg)

		c2.stats.AddProcessed(1)
		c2.stats.AddQueueSize(-1)

		// 更新统计
		latency := time.Since(start).Milliseconds()
		if latency > 0 {
			c2.stats.SetLatency(latency)
		}
		c2.updateStats()
	}
}

// handleMessage 根据消息类型分发到对应处理函数
func (c2 *Core2) handleMessage(msg Message) {
	switch m := msg.(type) {
	case *PacketMessage:
		c2.handlePacketMessage(m)
	case *ConnectionMessage:
		c2.handleConnectionMessage(m)
	case *PersistenceMessage:
		c2.handlePersistenceMessage(m)
	case *ModMessage:
		c2.handleModMessage(m)
	case *StorageMessage:
		c2.handleStorageMessage(m)
	case *WorldStreamMessage:
		c2.handleWorldStreamMessage(m)
	default:
		fmt.Printf("[Core2 %s] Unknown message type: %T\n", c2.name, msg)
	}
}

// handlePacketMessage 处理网络包（IO Core）
func (c2 *Core2) handlePacketMessage(m *PacketMessage) {
	// 这里处理网络包收发
}

// handleConnectionMessage 处理连接事件（IO Core）
func (c2 *Core2) handleConnectionMessage(m *ConnectionMessage) {
	// 这里处理连接打开/关闭
}

// handlePersistenceMessage 处理存档操作（IO Core）
func (c2 *Core2) handlePersistenceMessage(m *PersistenceMessage) {
	// 这里处理存档保存/加载
}

// handleModMessage 处理 Mod 操作（IO Core）
func (c2 *Core2) handleModMessage(m *ModMessage) {
	// 这里处理 Mod 加载/卸载
}

// handleStorageMessage 处理 Storage 事件（IO Core）
func (c2 *Core2) handleStorageMessage(m *StorageMessage) {
	// 这里处理事件记录
}

// handleWorldStreamMessage 处理 WorldStream 操作（IO Core）
func (c2 *Core2) handleWorldStreamMessage(m *WorldStreamMessage) {
	// 这里处理 MSAV 文件读写
}

// Send 发送消息到 IO Core
func (c2 *Core2) Send(msg Message) bool {
	c2.stats.AddQueueSize(1)
	select {
	case c2.messages <- msg:
		return true
	default:
		c2.stats.AddDropped(1)
		return false
	}
}

// Stop 停止 IO Core
func (c2 *Core2) Stop() {
	if c2.running.Swap(false) {
		close(c2.messages)
		c2.wg.Wait()
	}
}

// Stats 获取 IO Core 统计信息
func (c2 *Core2) Stats() (int64, int64, int64, int64, int64) {
	return c2.stats.GetStats()
}

// UpdateStats 更新统计信息
func (c2 *Core2) updateStats() {
	now := time.Now().UnixNano()
	c2.stats.mu.Lock()
	last := c2.stats.lastUpdate
	if now-last > 1e9 { // 每秒更新
		c2.stats.lastUpdate = now
	}
	c2.stats.mu.Unlock()
}
