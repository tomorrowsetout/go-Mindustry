package storage

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFileRecorderFlushWritesQueuedEvents(t *testing.T) {
	dir := t.TempDir()
	rec, err := NewFileRecorder(dir)
	if err != nil {
		t.Fatalf("NewFileRecorder: %v", err)
	}
	t.Cleanup(func() { _ = rec.Close() })

	err = rec.Record(Event{
		Timestamp: time.Now().UTC(),
		Kind:      "unit_spawn",
		UUID:      "player-1",
		Detail:    "test",
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}

	if err := rec.Flush(); err != nil {
		t.Fatalf("Flush: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "all.jsonl"))
	if err != nil {
		t.Fatalf("ReadFile(all.jsonl): %v", err)
	}
	text := string(data)
	if !strings.Contains(text, "unit_spawn") {
		t.Fatalf("expected event payload in all.jsonl, got %q", text)
	}
}

func TestFileRecorderFlushOnClosedRecorder(t *testing.T) {
	rec, err := NewFileRecorder(t.TempDir())
	if err != nil {
		t.Fatalf("NewFileRecorder: %v", err)
	}
	if err := rec.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := rec.Flush(); err == nil {
		t.Fatalf("expected Flush to fail on closed recorder")
	}
}
