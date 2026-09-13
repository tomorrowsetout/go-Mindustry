package core

import (
	"errors"
	"hash/fnv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type PolicyConfig struct {
	ConnectionBurst  int
	ConnectionWindow time.Duration
	PacketBurst      int
	PacketWindow     time.Duration
	PlayerShards     int
	CoreShards       int
}

type policyWindow struct {
	count   int
	resetAt time.Time
}

type Core4 struct {
	name         string
	messages     chan Message
	workerCount  int
	wg           sync.WaitGroup
	running      atomic.Bool
	localWorkers atomic.Bool
	stats        *Stats
	serverCore   atomic.Value // *ServerCore

	config PolicyConfig

	stateMu           sync.Mutex
	connectionWindows map[string]*policyWindow
	packetWindows     map[string]*policyWindow
	activeConnections map[int32]string
	nextPruneAt       time.Time

	remoteMu sync.RWMutex
	remote   *remoteCore4Client
}

func DefaultPolicyConfig() PolicyConfig {
	return PolicyConfig{
		ConnectionBurst:  64,
		ConnectionWindow: 10 * time.Second,
		PacketBurst:      4096,
		PacketWindow:     5 * time.Second,
		PlayerShards:     4,
		CoreShards:       4,
	}
}

func NewCore4(cfg Config) *Core4 {
	if cfg.MessageBuf <= 0 {
		cfg.MessageBuf = 1024
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 1
	}
	name := cfg.Name
	if strings.TrimSpace(name) == "" {
		name = "policy-core"
	}
	return &Core4{
		name:              name,
		messages:          make(chan Message, cfg.MessageBuf),
		workerCount:       cfg.WorkerCount,
		stats:             &Stats{lastUpdate: time.Now().UnixNano()},
		config:            DefaultPolicyConfig(),
		connectionWindows: map[string]*policyWindow{},
		packetWindows:     map[string]*policyWindow{},
		activeConnections: map[int32]string{},
	}
}

func (c4 *Core4) SetServerCore(sc *ServerCore) {
	c4.serverCore.Store(sc)
}

func (c4 *Core4) SetPolicyConfig(cfg PolicyConfig) {
	if cfg.ConnectionBurst <= 0 {
		cfg.ConnectionBurst = DefaultPolicyConfig().ConnectionBurst
	}
	if cfg.ConnectionWindow <= 0 {
		cfg.ConnectionWindow = DefaultPolicyConfig().ConnectionWindow
	}
	if cfg.PacketBurst <= 0 {
		cfg.PacketBurst = DefaultPolicyConfig().PacketBurst
	}
	if cfg.PacketWindow <= 0 {
		cfg.PacketWindow = DefaultPolicyConfig().PacketWindow
	}
	if cfg.PlayerShards <= 0 {
		cfg.PlayerShards = DefaultPolicyConfig().PlayerShards
	}
	if cfg.CoreShards <= 0 {
		cfg.CoreShards = DefaultPolicyConfig().CoreShards
	}
	c4.config = cfg
}

func (c4 *Core4) Start() {
	if c4.remoteClient() != nil {
		c4.running.Store(true)
		c4.localWorkers.Store(false)
		return
	}
	if !c4.running.Swap(true) {
		c4.localWorkers.Store(true)
		c4.wg.Add(c4.workerCount)
		for i := 0; i < c4.workerCount; i++ {
			go c4.worker()
		}
	}
}

func (c4 *Core4) worker() {
	defer c4.wg.Done()
	for msg := range c4.messages {
		c4.stats.AddReceived(1)
		start := time.Now()
		switch m := msg.(type) {
		case *PolicyMessage:
			c4.handlePolicyMessage(m)
		}
		c4.stats.AddProcessed(1)
		c4.stats.AddQueueSize(-1)
		if latency := time.Since(start).Milliseconds(); latency > 0 {
			c4.stats.SetLatency(latency)
		}
	}
}

func (c4 *Core4) Stop() {
	if c4.localWorkers.Load() {
		if c4.running.Swap(false) {
			close(c4.messages)
			c4.wg.Wait()
		}
		c4.localWorkers.Store(false)
		return
	}
	if c4.remoteClient() != nil {
		c4.running.Store(false)
		return
	}
	if c4.running.Swap(false) {
		close(c4.messages)
		c4.wg.Wait()
	}
}

func (c4 *Core4) Send(msg Message) bool {
	if c4 == nil || msg == nil {
		return false
	}
	select {
	case c4.messages <- msg:
		c4.stats.AddQueueSize(1)
		return true
	default:
		c4.stats.AddDropped(1)
		return false
	}
}

func (c4 *Core4) Stats() (int64, int64, int64, int64, int64) {
	if remote := c4.remoteClient(); remote != nil {
		received, processed, dropped, queueSize, latency, err := remote.stats()
		if err == nil {
			return received, processed, dropped, queueSize, latency
		}
	}
	return c4.stats.GetStats()
}

func (c4 *Core4) AllowConnection(ip, uuid string) (PolicyResult, error) {
	return c4.queryPolicy(&PolicyMessage{Action: "allow_connection", IP: ip, UUID: uuid})
}

func (c4 *Core4) AllowPacket(ip string, connID int32, uuid, packet string) (PolicyResult, error) {
	return c4.queryPolicy(&PolicyMessage{Action: "allow_packet", IP: ip, ConnID: connID, UUID: uuid, Packet: packet})
}

func (c4 *Core4) RecordConnectionOpen(connID int32, ip, uuid string) {
	_, _ = c4.queryPolicy(&PolicyMessage{Action: "record_open", ConnID: connID, IP: ip, UUID: uuid})
}

func (c4 *Core4) RecordConnectionClose(connID int32) {
	_, _ = c4.queryPolicy(&PolicyMessage{Action: "record_close", ConnID: connID})
}

func (c4 *Core4) PlayerShard(uuid, ip string) (PolicyResult, error) {
	return c4.queryPolicy(&PolicyMessage{Action: "player_shard", UUID: uuid, IP: ip})
}

func (c4 *Core4) CoreShard(key string) (PolicyResult, error) {
	return c4.queryPolicy(&PolicyMessage{Action: "core_shard", Key: key})
}

func (c4 *Core4) queryPolicy(msg *PolicyMessage) (PolicyResult, error) {
	if c4 == nil || msg == nil {
		return PolicyResult{}, errors.New("invalid policy request")
	}
	if remote := c4.remoteClient(); remote != nil {
		var (
			res PolicyResult
			err error
		)
		switch msg.Action {
		case "allow_connection":
			res, err = remote.allowConnection(msg.IP, msg.UUID)
		case "allow_packet":
			res, err = remote.allowPacket(msg.IP, msg.ConnID, msg.UUID, msg.Packet)
		case "record_open":
			err = remote.recordOpen(msg.ConnID, msg.IP, msg.UUID)
			res = PolicyResult{Allowed: true}
		case "record_close":
			err = remote.recordClose(msg.ConnID)
			res = PolicyResult{Allowed: true}
		case "player_shard":
			res, err = remote.playerShard(msg.UUID, msg.IP)
		case "core_shard":
			res, err = remote.coreShard(msg.Key)
		default:
			return PolicyResult{}, errors.New("unknown policy action")
		}
		if err == nil {
			return res, nil
		}
		c4.AttachRemote(nil)
	}
	if !c4.localWorkers.Load() {
		return c4.handlePolicyRequest(msg), nil
	}
	ch := make(chan PolicyResult, 1)
	msg.ResultChan = ch
	if !c4.Send(msg) {
		return c4.handlePolicyRequest(msg), nil
	}
	res := <-ch
	return res, res.Error
}

func (c4 *Core4) AttachRemote(client *ipcClient) {
	if c4 == nil {
		return
	}
	c4.remoteMu.Lock()
	defer c4.remoteMu.Unlock()
	if client == nil {
		c4.remote = nil
		return
	}
	c4.remote = &remoteCore4Client{client: client}
}

func (c4 *Core4) remoteClient() *remoteCore4Client {
	if c4 == nil {
		return nil
	}
	c4.remoteMu.RLock()
	defer c4.remoteMu.RUnlock()
	return c4.remote
}

func (c4 *Core4) handlePolicyMessage(m *PolicyMessage) {
	if m == nil {
		return
	}
	res := c4.handlePolicyRequest(m)
	if m.ResultChan != nil {
		m.ResultChan <- res
	}
}

func (c4 *Core4) handlePolicyRequest(m *PolicyMessage) PolicyResult {
	c4.stateMu.Lock()
	defer c4.stateMu.Unlock()
	c4.pruneStateLocked(time.Now())

	switch m.Action {
	case "allow_connection":
		allowed := c4.allowWindowLocked(c4.connectionWindows, normalizePolicyKey(m.UUID, m.IP), c4.config.ConnectionBurst, c4.config.ConnectionWindow)
		return PolicyResult{Allowed: allowed, PlayerShard: c4.playerShardLocked(m.UUID, m.IP)}
	case "allow_packet":
		packetKey := normalizePolicyKey(m.UUID, m.IP) + "|" + strings.TrimSpace(m.Packet)
		allowed := c4.allowWindowLocked(c4.packetWindows, packetKey, c4.config.PacketBurst, c4.config.PacketWindow)
		return PolicyResult{Allowed: allowed, PlayerShard: c4.playerShardLocked(m.UUID, m.IP)}
	case "record_open":
		c4.activeConnections[m.ConnID] = normalizePolicyKey(m.UUID, m.IP)
		return PolicyResult{Allowed: true, PlayerShard: c4.playerShardLocked(m.UUID, m.IP)}
	case "record_close":
		delete(c4.activeConnections, m.ConnID)
		return PolicyResult{Allowed: true}
	case "player_shard":
		return PolicyResult{Allowed: true, PlayerShard: c4.playerShardLocked(m.UUID, m.IP)}
	case "core_shard":
		return PolicyResult{Allowed: true, CoreShard: c4.coreShardLocked(m.Key)}
	default:
		return PolicyResult{Error: errors.New("unknown policy action")}
	}
}

func (c4 *Core4) pruneStateLocked(now time.Time) {
	if c4 == nil {
		return
	}
	if !c4.nextPruneAt.IsZero() && now.Before(c4.nextPruneAt) {
		return
	}
	prunePolicyWindowsLocked(c4.connectionWindows, now)
	prunePolicyWindowsLocked(c4.packetWindows, now)
	c4.nextPruneAt = now.Add(c4.policyPruneIntervalLocked())
}

func prunePolicyWindowsLocked(store map[string]*policyWindow, now time.Time) {
	for key, entry := range store {
		if entry == nil || !entry.resetAt.After(now) {
			delete(store, key)
		}
	}
}

func (c4 *Core4) policyPruneIntervalLocked() time.Duration {
	interval := c4.config.PacketWindow
	if interval <= 0 || (c4.config.ConnectionWindow > 0 && c4.config.ConnectionWindow < interval) {
		interval = c4.config.ConnectionWindow
	}
	if interval <= 0 {
		interval = time.Second
	}
	if interval < time.Second {
		return time.Second
	}
	if interval > 10*time.Second {
		return 10 * time.Second
	}
	return interval
}

func (c4 *Core4) allowWindowLocked(store map[string]*policyWindow, key string, burst int, window time.Duration) bool {
	now := time.Now()
	entry, ok := store[key]
	if !ok || entry == nil || now.After(entry.resetAt) {
		store[key] = &policyWindow{count: 1, resetAt: now.Add(window)}
		return true
	}
	if entry.count >= burst {
		return false
	}
	entry.count++
	return true
}

func normalizePolicyKey(uuid, ip string) string {
	if strings.TrimSpace(uuid) != "" {
		return "uuid:" + strings.TrimSpace(uuid)
	}
	if strings.TrimSpace(ip) != "" {
		return "ip:" + strings.TrimSpace(ip)
	}
	return "anonymous"
}

func (c4 *Core4) playerShardLocked(uuid, ip string) int {
	key := normalizePolicyKey(uuid, ip)
	return shardForKey(key, c4.config.PlayerShards)
}

func (c4 *Core4) coreShardLocked(key string) int {
	key = strings.TrimSpace(key)
	if key == "" {
		key = "default"
	}
	return shardForKey(key, c4.config.CoreShards)
}

func shardForKey(key string, total int) int {
	if total <= 0 {
		total = 1
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.ToLower(strings.TrimSpace(key))))
	return int(h.Sum32()%uint32(total)) + 1
}
