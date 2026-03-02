package storage

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type FileRecorder struct {
	mu      sync.Mutex
	baseDir string
}

func NewFileRecorder(baseDir string) (*FileRecorder, error) {
	if strings.TrimSpace(baseDir) == "" {
		baseDir = filepath.Join("data", "events")
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "players"), 0o755); err != nil {
		return nil, err
	}
	return &FileRecorder{baseDir: baseDir}, nil
}

func (r *FileRecorder) Record(e Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()

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

func (r *FileRecorder) Close() error {
	return nil
}

func (r *FileRecorder) Status() string {
	return fmt.Sprintf("file:%s", r.baseDir)
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

