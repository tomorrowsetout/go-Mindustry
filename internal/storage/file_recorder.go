package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
)

type FileRecorder struct {
	mu      sync.RWMutex
	baseDir string
	queue   chan Event
	wg      sync.WaitGroup
	closed  bool
	dropped atomic.Int64
	written atomic.Int64
	lastErr atomic.Value
}

func NewFileRecorder(baseDir string) (*FileRecorder, error) {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = filepath.Join("data", "events")
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "players"), 0o755); err != nil {
		return nil, err
	}
	r := &FileRecorder{
		baseDir: baseDir,
		queue:   make(chan Event, 8192),
	}
	r.wg.Add(1)
	go r.loop()
	return r, nil
}

func (r *FileRecorder) Record(e Event) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.closed {
		return os.ErrClosed
	}
	select {
	case r.queue <- e:
	default:
		r.dropped.Add(1)
	}
	return nil
}

func (r *FileRecorder) recordSync(e Event) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')

	allPath := filepath.Join(r.baseDir, "all.jsonl")
	if err := appendFile(allPath, line); err != nil {
		return err
	}

	uuid := strings.TrimSpace(e.UUID)
	if uuid == "" {
		uuid = "_anonymous"
	}
	playerPath := filepath.Join(r.baseDir, "players", sanitizeFilename(uuid)+".jsonl")
	return appendFile(playerPath, line)
}

func (r *FileRecorder) loop() {
	defer r.wg.Done()
	for e := range r.queue {
		if err := r.recordSync(e); err != nil {
			r.lastErr.Store(err)
			continue
		}
		r.written.Add(1)
	}
}

func (r *FileRecorder) Close() error {
	r.mu.Lock()
	if r.closed {
		r.mu.Unlock()
		return nil
	}
	r.closed = true
	close(r.queue)
	r.mu.Unlock()
	r.wg.Wait()
	if v := r.lastErr.Load(); v != nil {
		if err, ok := v.(error); ok {
			return err
		}
	}
	return nil
}

func (r *FileRecorder) Status() string {
	return fmt.Sprintf("file:%s queued=%d written=%d dropped=%d", r.baseDir, len(r.queue), r.written.Load(), r.dropped.Load())
}

func appendFile(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(data)
	return err
}

func sanitizeFilename(s string) string {
	r := strings.NewReplacer(
		"/", "_", "\\", "_", ":", "_", "*", "_", "?", "_",
		"\"", "_", "<", "_", ">", "_", "|", "_", " ", "_",
	)
	out := r.Replace(strings.TrimSpace(s))
	if out == "" {
		return "_anonymous"
	}
	return out
}
