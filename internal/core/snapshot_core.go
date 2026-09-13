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
	name         string
	messages     chan Message
	workerCount  int
	wg           sync.WaitGroup
	running      atomic.Bool
	localWorkers atomic.Bool
	stats        *Stats
	serverCore   atomic.Value // *ServerCore

	cacheMu        sync.Mutex
	entry          *snapshotCacheEntry
	cacheBaseModel bool
	copyOnRead     bool

	remoteMu sync.RWMutex
	remote   *remoteCore3Client
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
		name:           name,
		messages:       make(chan Message, cfg.MessageBuf),
		workerCount:    cfg.WorkerCount,
		cacheBaseModel: true,
		copyOnRead:     true,
		stats: &Stats{
			lastUpdate: time.Now().UnixNano(),
		},
	}
}

func (c3 *Core3) SetServerCore(sc *ServerCore) {
	c3.serverCore.Store(sc)
}

func (c3 *Core3) SetCacheBaseModel(enabled bool) {
	if c3 == nil {
		return
	}
	c3.cacheBaseModel = enabled
}

func (c3 *Core3) SetCopyOnRead(enabled bool) {
	if c3 == nil {
		return
	}
	c3.copyOnRead = enabled
}

func (c3 *Core3) Start() {
	if c3.remoteClient() != nil {
		c3.running.Store(true)
		c3.localWorkers.Store(false)
		return
	}
	if !c3.running.Swap(true) {
		c3.localWorkers.Store(true)
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
	if c3.localWorkers.Load() {
		if c3.running.Swap(false) {
			close(c3.messages)
			c3.wg.Wait()
		}
		c3.localWorkers.Store(false)
		return
	}
	if c3.remoteClient() != nil {
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
	if remote := c3.remoteClient(); remote != nil {
		received, processed, dropped, queueSize, latency, err := remote.stats()
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
	if remote := c3.remoteClient(); remote != nil {
		if res, err := remote.getWorld(path); err == nil {
			return res, nil
		} else {
			c3.AttachRemote(nil)
		}
	}
	if !c3.localWorkers.Load() {
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
	if remote := c3.remoteClient(); remote != nil {
		if err := remote.invalidateWorld(path); err == nil {
			return nil
		} else {
			c3.AttachRemote(nil)
		}
	}
	if !c3.localWorkers.Load() {
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
	c3.remoteMu.Lock()
	defer c3.remoteMu.Unlock()
	if client == nil {
		c3.remote = nil
		return
	}
	c3.remote = &remoteCore3Client{client: client}
}

func (c3 *Core3) remoteClient() *remoteCore3Client {
	if c3 == nil {
		return nil
	}
	c3.remoteMu.RLock()
	defer c3.remoteMu.RUnlock()
	return c3.remote
}

func (c3 *Core3) invalidateWorldCache(path string) error {
	c3.cacheMu.Lock()
	defer c3.cacheMu.Unlock()
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		c3.entry = nil
		return nil
	}
	if c3.entry != nil && c3.entry.path == trimmed {
		c3.entry = nil
	}
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
	if entry, ok := c3.lookupValidEntryLocked(path, info.ModTime()); ok {
		entry.lastAccess = time.Now()
		return snapshotResultFromEntry(entry, "active", c3.copyOnRead), nil
	}
	entry, err := c3.loadEntryLocked(path, info.ModTime())
	if err != nil {
		return SnapshotResult{}, err
	}
	c3.entry = entry
	return snapshotResultFromEntry(entry, "active", c3.copyOnRead), nil
}

func (c3 *Core3) lookupValidEntryLocked(path string, modTime time.Time) (*snapshotCacheEntry, bool) {
	entry := c3.entry
	if entry == nil {
		return nil, false
	}
	if entry.path != path {
		return nil, false
	}
	if !entry.modTime.Equal(modTime) {
		c3.entry = nil
		return nil, false
	}
	return entry, true
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
		if c3.cacheBaseModel {
			if model, merr := worldstream.LoadWorldModelFromMSAV(path, nil); merr == nil {
				entry.baseModel = model
			}
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

func snapshotResultFromEntry(entry *snapshotCacheEntry, level string, copyData bool) SnapshotResult {
	if entry == nil {
		return SnapshotResult{}
	}
	data := entry.data
	if copyData {
		data = append([]byte(nil), entry.data...)
	}
	res := SnapshotResult{
		Data:      data,
		BaseModel: entry.baseModel,
		CorePos:   entry.corePos,
		CorePosOK: entry.corePosOK,
		Level:     level,
	}
	return res
}
