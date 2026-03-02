package storage

import (
	"fmt"
	"strings"

	"mdt-server/internal/config"
)

func NewRecorder(cfg config.StorageConfig) (Recorder, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Mode))
	if mode == "" {
		mode = "file"
	}

	// DB backend is opt-in. Default path is file recorder.
	if !cfg.DatabaseEnabled || mode == "file" {
		return NewFileRecorder(cfg.Directory)
	}

	// Placeholder adapters: keep events durable even when DB mode requested.
	// This keeps server behavior predictable until DB adapters are implemented.
	rec, err := NewFileRecorder(cfg.Directory)
	if err != nil {
		return nil, err
	}
	return &fallbackRecorder{
		inner: rec,
		note:  fmt.Sprintf("db backend %q requested but not implemented; using file recorder", mode),
	}, nil
}

type fallbackRecorder struct {
	inner Recorder
	note  string
}

func (f *fallbackRecorder) Record(e Event) error { return f.inner.Record(e) }
func (f *fallbackRecorder) Close() error         { return f.inner.Close() }
func (f *fallbackRecorder) Status() string {
	return fmt.Sprintf("%s (%s)", f.inner.Status(), f.note)
}

