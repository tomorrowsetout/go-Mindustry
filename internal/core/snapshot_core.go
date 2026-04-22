package core

import (
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
	"mdt-server/internal/worldstream"
)

const (
	snapshotL1Capacity = 4
	snapshotL2Capacity = 16
	snapshotL3Capacity = 64
)

type snapshotCacheEntry struct {
	path       string
	modTime    time.Time
	data       []byte
	baseModel  *world.WorldModel
	corePos    protocol.Point2
	corePosOK  bool
	lastAccess time.Time
}

type Core3 struct {
	name        string
	messages    chan Message
	workerCount int
	wg          sync.WaitGroup
	running     atomic.Bool
	stats       *Stats
	serverCore  atomic.Value // *ServerCore

	cacheMu sync.Mutex
	l1      map[string]*snapshotCacheEntry
	l2      map[string]*snapshotCacheEntry
	l3      map[string]*snapshotCacheEntry

	remote *remoteCore3Client
}

func NewCore3(cfg Config) *Core3 {
	if cfg.MessageBuf <= 0 {
		cfg.MessageBuf = 1024
	}
	if cfg.WorkerCount <= 0 {
		cfg.WorkerCount = 1
	}
	name := cfg.Name
	if strings.TrimSpace(name) == "" {
		name = "snapshot-core"
	}
	return &Core3{
		name:        name,
		messages:    make(chan Message, cfg.MessageBuf),
		workerCount: cfg.WorkerCount,
		stats: &Stats{
			lastUpdate: time.Now().UnixNano(),
		},
		l1: map[string]*snapshotCacheEntry{},
		l2: map[string]*snapshotCacheEntry{},
		l3: map[string]*snapshotCacheEntry{},
	}
}

func (c3 *Core3) SetServerCore(sc *ServerCore) {
	c3.serverCore.Store(sc)
}

func (c3 *Core3) Start() {
	if c3.remote != nil {
		c3.running.Store(true)
		return
	}
	if !c3.running.Swap(true) {
		c3.wg.Add(c3.workerCount)
		for i := 0; i < c3.workerCount; i++ {
			go c3.worker()
		}
	}
}

func (c3 *Core3) worker() {
	defer c3.wg.Done()
	for msg := range c3.messages {
		c3.stats.AddReceived(1)
		start := time.Now()
		switch m := msg.(type) {
		case *SnapshotMessage:
			c3.handleSnapshotMessage(m)
		}
		c3.stats.AddProcessed(1)
		c3.stats.AddQueueSize(-1)
		if latency := time.Since(start).Milliseconds(); latency > 0 {
			c3.stats.SetLatency(latency)
		}
	}
}

func (c3 *Core3) Stop() {
	if c3.remote != nil {
		c3.running.Store(false)
		return
	}
	if c3.running.Swap(false) {
		close(c3.messages)
		c3.wg.Wait()
	}
}

func (c3 *Core3) Send(msg Message) bool {
	if c3 == nil || msg == nil {
		return false
	}
	select {
	case c3.messages <- msg:
		c3.stats.AddQueueSize(1)
		return true
	default:
		c3.stats.AddDropped(1)
		return false
	}
}

func (c3 *Core3) Stats() (int64, int64, int64, int64, int64) {
	if c3.remote != nil {
		received, processed, dropped, queueSize, latency, err := c3.remote.stats()
		if err == nil {
			return received, processed, dropped, queueSize, latency
		}
	}
	return c3.stats.GetStats()
}

func (c3 *Core3) GetWorldCache(path string) (SnapshotResult, error) {
	if c3 == nil {
		return SnapshotResult{}, fmt.Errorf("nil snapshot core")
	}
	if c3.remote != nil {
		return c3.remote.getWorld(path)
	}
	if !c3.running.Load() {
		return c3.getWorldCache(path)
	}
	ch := make(chan SnapshotResult, 1)
	if !c3.Send(&SnapshotMessage{Action: "get_world", Path: path, ResultChan: ch}) {
		return c3.getWorldCache(path)
	}
	res := <-ch
	return res, res.Error
}

func (c3 *Core3) InvalidateWorldCache(path string) error {
	if c3 == nil {
		return nil
	}
	if c3.remote != nil {
		return c3.remote.invalidateWorld(path)
	}
	if !c3.running.Load() {
		return c3.invalidateWorldCache(path)
	}
	ch := make(chan SnapshotResult, 1)
	if !c3.Send(&SnapshotMessage{Action: "invalidate_world", Path: path, ResultChan: ch}) {
		return c3.invalidateWorldCache(path)
	}
	return (<-ch).Error
}

func (c3 *Core3) handleSnapshotMessage(m *SnapshotMessage) {
	if m == nil {
		return
	}
	var res SnapshotResult
	switch m.Action {
	case "invalidate_world":
		res.Error = c3.invalidateWorldCache(m.Path)
	default:
		res, _ = c3.getWorldCache(m.Path)
	}
	if m.ResultChan != nil {
		m.ResultChan <- res
	}
}

func (c3 *Core3) AttachRemote(client *ipcClient) {
	if c3 == nil {
		return
	}
	if client == nil {
		c3.remote = nil
		return
	}
	c3.remote = &remoteCore3Client{client: client}
}

func (c3 *Core3) invalidateWorldCache(path string) error {
	c3.cacheMu.Lock()
	defer c3.cacheMu.Unlock()
	if strings.TrimSpace(path) == "" {
		c3.l1 = map[string]*snapshotCacheEntry{}
		c3.l2 = map[string]*snapshotCacheEntry{}
		c3.l3 = map[string]*snapshotCacheEntry{}
		return nil
	}
	delete(c3.l1, path)
	delete(c3.l2, path)
	delete(c3.l3, path)
	return nil
}

func (c3 *Core3) getWorldCache(path string) (SnapshotResult, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return SnapshotResult{}, fmt.Errorf("empty world cache path")
	}
	info, err := os.Stat(path)
	if err != nil {
		return SnapshotResult{}, err
	}
	c3.cacheMu.Lock()
	defer c3.cacheMu.Unlock()
	if entry, level, ok := c3.lookupValidEntryLocked(path, info.ModTime()); ok {
		entry.lastAccess = time.Now()
		c3.promoteLocked(path, entry, level)
		return snapshotResultFromEntry(entry, "L1"), nil
	}
	entry, err := c3.loadEntryLocked(path, info.ModTime())
	if err != nil {
		return SnapshotResult{}, err
	}
	c3.l1[path] = entry
	c3.rebalanceLocked()
	return snapshotResultFromEntry(entry, "L1"), nil
}

func (c3 *Core3) lookupValidEntryLocked(path string, modTime time.Time) (*snapshotCacheEntry, string, bool) {
	check := func(store map[string]*snapshotCacheEntry, level string) (*snapshotCacheEntry, string, bool) {
		entry, ok := store[path]
		if !ok || entry == nil {
			return nil, "", false
		}
		if !entry.modTime.Equal(modTime) {
			delete(store, path)
			return nil, "", false
		}
		return entry, level, true
	}
	if entry, level, ok := check(c3.l1, "L1"); ok {
		return entry, level, true
	}
	if entry, level, ok := check(c3.l2, "L2"); ok {
		return entry, level, true
	}
	if entry, level, ok := check(c3.l3, "L3"); ok {
		return entry, level, true
	}
	return nil, "", false
}

func (c3 *Core3) loadEntryLocked(path string, modTime time.Time) (*snapshotCacheEntry, error) {
	data, err := loadWorldCachePayload(path)
	if err != nil {
		return nil, err
	}
	entry := &snapshotCacheEntry{
		path:       path,
		modTime:    modTime,
		data:       data,
		lastAccess: time.Now(),
	}
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".msav") || strings.HasSuffix(lower, ".msav.msav") {
		if model, merr := worldstream.LoadWorldModelFromMSAV(path, nil); merr == nil {
			entry.baseModel = model
		}
		if pos, ok, perr := worldstream.FindCoreTileFromMSAV(path); perr == nil {
			entry.corePos = pos
			entry.corePosOK = ok
		}
	}
	return entry, nil
}

func loadWorldCachePayload(path string) ([]byte, error) {
	lower := strings.ToLower(path)
	if strings.HasSuffix(lower, ".msav") || strings.HasSuffix(lower, ".msav.msav") {
		return worldstream.BuildWorldStreamFromMSAV(path)
	}
	return os.ReadFile(path)
}

func snapshotResultFromEntry(entry *snapshotCacheEntry, level string) SnapshotResult {
	if entry == nil {
		return SnapshotResult{}
	}
	res := SnapshotResult{
		Data:      append([]byte(nil), entry.data...),
		BaseModel: entry.baseModel,
		CorePos:   entry.corePos,
		CorePosOK: entry.corePosOK,
		Level:     level,
	}
	return res
}

func (c3 *Core3) promoteLocked(path string, entry *snapshotCacheEntry, level string) {
	switch level {
	case "L2":
		delete(c3.l2, path)
		c3.l1[path] = entry
	case "L3":
		delete(c3.l3, path)
		c3.l1[path] = entry
	}
	c3.rebalanceLocked()
}

func (c3 *Core3) rebalanceLocked() {
	rebalance := func(src map[string]*snapshotCacheEntry, srcCap int, dst map[string]*snapshotCacheEntry) {
		for len(src) > srcCap {
			var oldestKey string
			var oldestTime time.Time
			first := true
			for key, entry := range src {
				if entry == nil {
					oldestKey = key
					first = false
					break
				}
				if first || entry.lastAccess.Before(oldestTime) {
					oldestKey = key
					oldestTime = entry.lastAccess
					first = false
				}
			}
			entry := src[oldestKey]
			delete(src, oldestKey)
			if dst != nil && entry != nil {
				dst[oldestKey] = entry
			}
		}
	}
	rebalance(c3.l1, snapshotL1Capacity, c3.l2)
	rebalance(c3.l2, snapshotL2Capacity, c3.l3)
	rebalance(c3.l3, snapshotL3Capacity, nil)
}
