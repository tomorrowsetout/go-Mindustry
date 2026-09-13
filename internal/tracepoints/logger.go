package tracepoints

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mdt-server/internal/protocol"
)

type Event struct {
	Timestamp time.Time      `json:"ts"`
	Category  string         `json:"category"`
	Point     string         `json:"point"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type Logger struct {
	mu       sync.RWMutex
	enabled  bool
	file     *os.File
	lines    chan []byte
	done     sync.WaitGroup
	closed   bool
	close    sync.Once
	closeErr error
}

func New(path string, enabled bool) (*Logger, error) {
	l := &Logger{enabled: enabled}
	if !enabled {
		return l, nil
	}
	if path == "" {
		return l, nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, err
	}
	l.file = f
	l.lines = make(chan []byte, 8192)
	l.done.Add(1)
	go l.writer(l.lines)
	return l, nil
}

func (l *Logger) Enabled() bool {
	if l == nil {
		return false
	}
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.enabled && !l.closed && l.lines != nil
}

func (l *Logger) Log(category, point string, fields map[string]any) {
	if !l.Enabled() {
		return
	}
	entry := Event{
		Timestamp: time.Now().UTC(),
		Category:  category,
		Point:     point,
		Fields:    fields,
	}
	line, err := json.Marshal(entry)
	if err != nil {
		line = []byte(fmt.Sprintf(`{"ts":%q,"category":%q,"point":%q,"fields":{"marshal_error":%q}}`,
			entry.Timestamp.Format(time.RFC3339Nano), category, point, err.Error()))
	}
	line = append(line, '\n')
	l.mu.RLock()
	defer l.mu.RUnlock()
	if l.closed || l.lines == nil {
		return
	}
	select {
	case l.lines <- line:
	default:
		// Tracepoints must never block gameplay/connection handling.
	}
}

func (l *Logger) writer(lines <-chan []byte) {
	defer l.done.Done()
	for line := range lines {
		if l.file == nil {
			continue
		}
		_, _ = l.file.Write(line)
	}
	if l.file != nil {
		l.closeErr = l.file.Close()
		l.file = nil
	}
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.close.Do(func() {
		l.mu.Lock()
		l.closed = true
		lines := l.lines
		l.lines = nil
		l.mu.Unlock()
		if lines != nil {
			close(lines)
			l.done.Wait()
		} else if l.file != nil {
			l.closeErr = l.file.Close()
			l.file = nil
		}
	})
	return l.closeErr
}

func PacketFields(direction string, obj any, packetID, frameworkID, size int, extra map[string]any) map[string]any {
	fields := map[string]any{
		"direction":    direction,
		"packet_type":  fmt.Sprintf("%T", obj),
		"packet_id":    packetID,
		"framework_id": frameworkID,
		"size":         size,
		"summary":      packetSummary(obj, size),
	}
	for k, v := range extra {
		fields[k] = v
	}
	return fields
}

func packetSummary(obj any, size int) string {
	switch p := obj.(type) {
	case *protocol.StreamBegin:
		return fmt.Sprintf("streamBegin id=%d total=%d type=%d", p.ID, p.Total, p.Type)
	case *protocol.StreamChunk:
		return fmt.Sprintf("streamChunk id=%d bytes=%d", p.ID, len(p.Data))
	case *protocol.WorldStream:
		return "worldStream"
	case *protocol.Remote_NetClient_blockSnapshot_34:
		return fmt.Sprintf("blockSnapshot amount=%d bytes=%d", p.Amount, len(p.Data))
	case *protocol.Remote_NetClient_entitySnapshot_32:
		return fmt.Sprintf("entitySnapshot amount=%d bytes=%d", p.Amount, len(p.Data))
	case *protocol.Remote_NetClient_stateSnapshot_35:
		return fmt.Sprintf("stateSnapshot wave=%d waveTime=%.2f tps=%d coreBytes=%d", p.Wave, p.WaveTime, p.Tps, len(p.CoreData))
	}
	if size > 256 {
		return fmt.Sprintf("%T size=%d", obj, size)
	}
	return fmt.Sprintf("%+v", obj)
}
