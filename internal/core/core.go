package core

import (
	"encoding/json"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"mdt-server/internal/persist"
	"mdt-server/internal/storage"
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
	serverCore atomic.Value // *ServerCore
	recorder  storage.Recorder
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

// handleConnectionMessage 处理连接事件（IO Core）
func (c2 *Core2) handleConnectionMessage(m *ConnectionMessage) {
	switch {
	case m.IsOpen:
		c2.handleConnectionOpen(m)
	default:
		c2.handleConnectionClose(m)
	}
}

// handleConnectionOpen 处理连接打开
func (c2 *Core2) handleConnectionOpen(m *ConnectionMessage) {
	// TODO: 实现连接打开逻辑
	// 例如：记录连接事件、初始化连接状态等
	fmt.Printf("[Core2 %s] Connection opened: connID=%d, UUID=%s, IP=%s\n", 
		c2.name, m.ConnID, m.UUID, m.IP)
}

// handleConnectionClose 处理连接关闭
func (c2 *Core2) handleConnectionClose(m *ConnectionMessage) {
	// TODO: 实现连接关闭逻辑
	// 例如：清理连接资源、记录断开事件等
	fmt.Printf("[Core2 %s] Connection closed: connID=%d, UUID=%s, IP=%s\n", 
		c2.name, m.ConnID, m.UUID, m.IP)
}

// handleModMessage 处理 Mod 操作（IO Core）
func (c2 *Core2) handleModMessage(m *ModMessage) {
	switch m.Action {
	case "load":
		c2.handleModLoad(m)
	case "unload":
		c2.handleModUnload(m)
	case "start":
		c2.handleModStart(m)
	case "stop":
		c2.handleModStop(m)
	case "reload":
		c2.handleModReload(m)
	case "scan":
		c2.handleModScan(m)
	default:
		fmt.Printf("[Core2 %s] Unknown mod action: %s\n", c2.name, m.Action)
		if m.ResultChan != nil {
			m.ResultChan <- ModResult{Error: fmt.Errorf("unknown action: %s", m.Action)}
		}
	}
}

// handleModLoad 加载 Mod
func (c2 *Core2) handleModLoad(m *ModMessage) {
	result := ModResult{}

	// TODO: 实现 Mod 加载逻辑
	// 根据 ModType (java/js/go/node) 加载不同的 Mod
	fmt.Printf("[Core2 %s] Loading mod: name=%s, type=%s, path=%s\n", 
		c2.name, m.Name, m.ModType, m.Path)

	// 模拟加载成功
	result.ID = m.ID
	result.Success = true
	result.Name = m.Name

	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

// handleModUnload 卸载 Mod
func (c2 *Core2) handleModUnload(m *ModMessage) {
	result := ModResult{}

	// TODO: 实现 Mod 卸载逻辑
	fmt.Printf("[Core2 %s] Unloading mod: name=%s, type=%s\n", 
		c2.name, m.Name, m.ModType)

	// 模拟卸载成功
	result.ID = m.ID
	result.Success = true

	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

// handleModStart 启动 Mod
func (c2 *Core2) handleModStart(m *ModMessage) {
	result := ModResult{}

	// TODO: 实现 Mod 启动逻辑
	fmt.Printf("[Core2 %s] Starting mod: name=%s, type=%s\n", 
		c2.name, m.Name, m.ModType)

	// 模拟启动成功
	result.ID = m.ID
	result.Success = true

	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

// handleModStop 停止 Mod
func (c2 *Core2) handleModStop(m *ModMessage) {
	result := ModResult{}

	// TODO: 实现 Mod 停止逻辑
	fmt.Printf("[Core2 %s] Stopping mod: name=%s, type=%s\n", 
		c2.name, m.Name, m.ModType)

	// 模拟停止成功
	result.ID = m.ID
	result.Success = true

	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

// handleModReload 重新加载 Mod
func (c2 *Core2) handleModReload(m *ModMessage) {
	result := ModResult{}

	// TODO: 实现 Mod 重新加载逻辑
	fmt.Printf("[Core2 %s] Reloading mod: name=%s, type=%s\n", 
		c2.name, m.Name, m.ModType)

	// 模拟重新加载成功
	result.ID = m.ID
	result.Success = true

	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

// handleModScan 扫描 Mod 目录
func (c2 *Core2) handleModScan(m *ModMessage) {
	result := ModResult{}

	// TODO: 实现 Mod 扫描逻辑
	fmt.Printf("[Core2 %s] Scanning mods directory\n", c2.name)

	// 模拟扫描成功
	result.ID = m.ID
	result.Success = true

	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

// handleWorldStreamMessage 处理 WorldStream 操作（IO Core）
func (c2 *Core2) handleWorldStreamMessage(m *WorldStreamMessage) {
	switch m.Action {
	case "load_model":
		c2.handleWorldStreamLoadModel(m)
	case "save_snapshot":
		c2.handleWorldStreamSaveSnapshot(m)
	case "rewrite_player":
		c2.handleWorldStreamRewritePlayer(m)
	default:
		fmt.Printf("[Core2 %s] Unknown worldstream action: %s\n", c2.name, m.Action)
	}
}

// handleWorldStreamLoadModel 从 MSAV 加载世界模型
func (c2 *Core2) handleWorldStreamLoadModel(m *WorldStreamMessage) {
	// TODO: 实现从 MSAV 加载世界模型
	// model, err := worldstream.LoadWorldModelFromMSAV(m.Path)
	// if err != nil {
	//     result.Error = err
	// } else {
	//     result.WorldData = worldstream.BuildWorldStreamFromModel(model)
	// }

	fmt.Printf("[Core2 %s] Loading world model from: %s\n", c2.name, m.Path)

	// 模拟加载成功
	// result.WorldData = []byte("mock_world_stream_data")
}

// handleWorldStreamSaveSnapshot 保存世界快照到 MSAV
func (c2 *Core2) handleWorldStreamSaveSnapshot(m *WorldStreamMessage) {
	// TODO: 实现保存世界快照到 MSAV
	// err := worldstream.SaveWorldModelToMSAV(m.Path, m.ModelData)
	// if err != nil {
	//     result.Error = err
	// }

	fmt.Printf("[Core2 %s] Saving world snapshot to: %s\n", c2.name, m.Path)

	// 模拟保存成功
}

// handleWorldStreamRewritePlayer 重写玩家数据
func (c2 *Core2) handleWorldStreamRewritePlayer(m *WorldStreamMessage) {
	// TODO: 实现重写玩家数据
	// result.WorldData, result.Error = worldstream.RewritePlayerIDInWorldStream(m.ModelData, m.PlayerID)

	fmt.Printf("[Core2 %s] Rewriting player ID %d in world stream: %s\n",
		c2.name, m.PlayerID, m.Path)

	// 模拟重写成功
	// result.WorldData = []byte("mock_world_stream_data_with_new_player_id")
}

// handlePacketMessage 处理网络包（IO Core）
func (c2 *Core2) handlePacketMessage(m *PacketMessage) {
	switch m.Kind {
	case "incoming":
		c2.handlePacketIncoming(m)
	case "outgoing":
		c2.handlePacketOutgoing(m)
	default:
		fmt.Printf("[Core2 %s] Unknown packet kind: %s\n", c2.name, m.Kind)
	}
}

// handlePacketIncoming 处理 incoming 包
func (c2 *Core2) handlePacketIncoming(m *PacketMessage) {
	// TODO: 实现 incoming 包处理
	// 例如：解码包、分发到游戏逻辑等
	fmt.Printf("[Core2 %s] Incoming packet: connID=%d, packet=%T\n", 
		c2.name, m.ConnID, m.Packet)
}

// handlePacketOutgoing 处理 outgoing 包
func (c2 *Core2) handlePacketOutgoing(m *PacketMessage) {
	// TODO: 实现 outgoing 包处理
	// 例如：编码包、发送到网络等
	fmt.Printf("[Core2 %s] Outgoing packet: connID=%d, packet=%T\n", 
		c2.name, m.ConnID, m.Packet)
}

// handleStorageMessage 处理 Storage 事件（IO Core）
func (c2 *Core2) handleStorageMessage(m *StorageMessage) {
	switch m.Action {
	case "record_event":
		c2.handleRecordEvent(m)
	case "record_player":
		c2.handleRecordPlayer(m)
	case "flush":
		c2.handleFlush(m)
	case "close":
		c2.handleClose(m)
	default:
		fmt.Printf("[Core2 %s] Unknown storage action: %s\n", c2.name, m.Action)
	}
}

// handleRecordEvent 记录事件
func (c2 *Core2) handleRecordEvent(m *StorageMessage) {
	if c2.recorder == nil {
		return
	}

	// 解析事件数据
	var ev storage.Event
	if len(m.EventData) > 0 {
		_ = json.Unmarshal(m.EventData, &ev)
	}
	_ = c2.recorder.Record(ev)
}

// handleRecordPlayer 记录玩家事件
func (c2 *Core2) handleRecordPlayer(m *StorageMessage) {
	if c2.recorder == nil {
		return
	}

	// 解析事件数据
	var ev storage.Event
	if len(m.EventData) > 0 {
		_ = json.Unmarshal(m.EventData, &ev)
	}
	_ = c2.recorder.Record(ev)
}

// handleFlush 刷新事件
func (c2 *Core2) handleFlush(m *StorageMessage) {
	_ = m
	if c2.recorder == nil {
		return
	}
	if f, ok := c2.recorder.(storage.Flusher); ok {
		_ = f.Flush()
	}
}

// handleClose 关闭记录器
func (c2 *Core2) handleClose(m *StorageMessage) {
	_ = m
	if c2.recorder != nil {
		if f, ok := c2.recorder.(storage.Flusher); ok {
			_ = f.Flush()
		}
		_ = c2.recorder.Close()
	}
}

// handlePersistenceMessage 处理存档操作（IO Core）
func (c2 *Core2) handlePersistenceMessage(m *PersistenceMessage) {
	switch m.Action {
	case "save_state":
		c2.handleSaveState(m)
	case "load_state":
		c2.handleLoadState(m)
	case "save_world":
		c2.handleSaveWorld(m)
	case "load_world":
		c2.handleLoadWorld(m)
	default:
		fmt.Printf("[Core2 %s] Unknown persistence action: %s\n", c2.name, m.Action)
		if m.ResultChan != nil {
			m.ResultChan <- PersistenceResult{Error: fmt.Errorf("unknown action: %s", m.Action)}
		}
	}
}

// handleSaveState 保存游戏状态
func (c2 *Core2) handleSaveState(m *PersistenceMessage) {
	result := PersistenceResult{}

	// 从 ServerCore 获取配置
	if sc, ok := c2.serverCore.Load().(*ServerCore); ok {
		// 保存状态到 JSON
		err := persist.Save(sc.persistCfg, persist.State{
			MapPath:  m.Path,
			WaveTime: 0, // TODO: 从游戏状态获取
			Wave:     0, // TODO: 从游戏状态获取
			Tick:     0, // TODO: 从游戏状态获取
			TimeData: 0, // TODO: 从游戏状态获取
			Rand0:    0, // TODO: 从游戏状态获取
			Rand1:    0, // TODO: 从游戏状态获取
			SavedAt:  time.Now().UTC().Format(time.RFC3339),
		})
		if err != nil {
			result.Error = err
		}
	} else {
		result.Error = fmt.Errorf("server core not initialized")
	}

	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

// handleLoadState 加载游戏状态
func (c2 *Core2) handleLoadState(m *PersistenceMessage) {
	result := PersistenceResult{}

	// 从 ServerCore 获取配置
	if sc, ok := c2.serverCore.Load().(*ServerCore); ok {
		st, ok, err := persist.Load(sc.persistCfg)
		if err != nil {
			result.Error = err
		} else if ok {
			result.StateData = []byte(fmt.Sprintf("wave=%d,tick=%d", st.Wave, st.Tick))
		}
	} else {
		result.Error = fmt.Errorf("server core not initialized")
	}

	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

// handleSaveWorld 保存世界数据（MSAV）
func (c2 *Core2) handleSaveWorld(m *PersistenceMessage) {
	result := PersistenceResult{}

	// TODO: 从世界模型保存 MSAV
	// err := worldstream.SaveWorldModelToMSAV(m.Path, m.ModelData)
	// if err != nil {
	//     result.Error = err
	// }

	if m.ResultChan != nil {
		m.ResultChan <- result
	}
}

// handleLoadWorld 加载世界数据（MSAV）
func (c2 *Core2) handleLoadWorld(m *PersistenceMessage) {
	result := PersistenceResult{}

	// TODO: 加载 MSAV 文件
	// model, err := worldstream.LoadWorldModelFromMSAV(m.Path)
	// if err != nil {
	//     result.Error = err
	// } else {
	//     result.WorldData = worldstream.BuildWorldStreamFromModel(model)
	// }

	if m.ResultChan != nil {
		m.ResultChan <- result
	}
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

// SetServerCore 设置 ServerCore 引用
func (c2 *Core2) SetServerCore(sc *ServerCore) {
	c2.serverCore.Store(sc)
}

// SetRecorder 设置事件记录器
func (c2 *Core2) SetRecorder(rec storage.Recorder) {
	c2.recorder = rec
}

// Recorder 获取事件记录器
func (c2 *Core2) Recorder() storage.Recorder {
	return c2.recorder
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
