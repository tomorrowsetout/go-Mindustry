package tracepoints

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type Event struct {
	Timestamp time.Time      `json:"ts"`
	Category  string         `json:"category"`
	Point     string         `json:"point"`
	Fields    map[string]any `json:"fields,omitempty"`
}

type Logger struct {
	mu      sync.Mutex
	enabled bool
	file    *os.File
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
	return l, nil
}

func (l *Logger) Enabled() bool {
	return l != nil && l.enabled && l.file != nil
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
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return
	}
	_, _ = l.file.Write(append(line, '\n'))
}

func (l *Logger) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		return nil
	}
	err := l.file.Close()
	l.file = nil
	return err
}

func PacketFields(direction string, obj any, packetID, frameworkID, size int, extra map[string]any) map[string]any {
	fields := map[string]any{
		"direction":    direction,
		"packet_type":  fmt.Sprintf("%T", obj),
		"packet_id":    packetID,
		"framework_id": frameworkID,
		"size":         size,
		"summary":      fmt.Sprintf("%+v", obj),
	}
	for k, v := range extra {
		fields[k] = v
	}
	return fields
}
