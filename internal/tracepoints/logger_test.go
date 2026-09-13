package tracepoints

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func TestLoggerCloseFlushesAndReleasesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracepoints.jsonl")
	logger, err := New(path, true)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}

	logger.Log("client_request", "packet_recv", map[string]any{"conn_id": 1})
	logger.Log("client_request", "connect_packet", map[string]any{"conn_id": 1})

	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tracepoints file: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, `"point":"packet_recv"`) {
		t.Fatalf("expected packet_recv tracepoint, got %q", text)
	}
	if !strings.Contains(text, `"point":"connect_packet"`) {
		t.Fatalf("expected connect_packet tracepoint, got %q", text)
	}

	renamed := path + ".moved"
	if err := os.Rename(path, renamed); err != nil {
		t.Fatalf("expected tracepoints file handle to be released after close, rename failed: %v", err)
	}
}

func TestLoggerLogAfterCloseIsIgnored(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tracepoints.jsonl")
	logger, err := New(path, true)
	if err != nil {
		t.Fatalf("new logger: %v", err)
	}
	if err := logger.Close(); err != nil {
		t.Fatalf("close logger: %v", err)
	}

	logger.Log("client_request", "packet_recv", map[string]any{"conn_id": 2})
	time.Sleep(10 * time.Millisecond)

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read tracepoints file: %v", err)
	}
	if strings.TrimSpace(string(raw)) != "" {
		t.Fatalf("expected no data after log-on-closed logger, got %q", string(raw))
	}
}

func TestPacketFieldsSummarizesBinaryPayloads(t *testing.T) {
	fields := PacketFields("send", &protocol.StreamChunk{ID: 7, Data: []byte{1, 2, 3, 4}}, 1, -1, 10, nil)
	summary, _ := fields["summary"].(string)
	if summary != "streamChunk id=7 bytes=4" {
		t.Fatalf("expected compact stream summary, got %q", summary)
	}
	if strings.Contains(summary, "[1 2 3 4]") {
		t.Fatalf("expected binary payload bytes to be omitted, got %q", summary)
	}

	fields = PacketFields("send", &protocol.Remote_NetClient_blockSnapshot_34{Amount: 2, Data: []byte{9, 8, 7}}, 34, -1, 12, nil)
	summary, _ = fields["summary"].(string)
	if summary != "blockSnapshot amount=2 bytes=3" {
		t.Fatalf("expected compact block snapshot summary, got %q", summary)
	}
}
