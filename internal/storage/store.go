package storage

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"mdt-server/internal/config"
)

// Store extends Recorder with structured logs and player data persistence.
type Store interface {
	Recorder
	Log(kind string, data any) error
	UpsertPlayer(rec PlayerRecord) error
}

// PlayerRecord is a lightweight persisted player profile.
type PlayerRecord struct {
	UUID        string   `json:"uuid"`
	USID        string   `json:"usid,omitempty"`
	Name        string   `json:"name,omitempty"`
	IP          string   `json:"ip,omitempty"`
	FirstSeen   string   `json:"first_seen,omitempty"`
	LastSeen    string   `json:"last_seen,omitempty"`
	TimesJoined int      `json:"times_joined,omitempty"`
	TimesKicked int      `json:"times_kicked,omitempty"`
	Names       []string `json:"names,omitempty"`
	IPs         []string `json:"ips,omitempty"`
}

// LogEntry is a generic log payload.
type LogEntry struct {
	Timestamp string `json:"ts"`
	Kind      string `json:"kind"`
	Data      any    `json:"data,omitempty"`
}

// FileStore implements Store using local files.
type FileStore struct {
	baseDir  string
	logDir   string
	format   string
	maxSize  int64
	maxFiles int

	logsMu   sync.Mutex
	logs     map[string]*rotatingLog
	playerMu sync.Mutex
	closed   atomic.Bool
	lastErr  atomic.Value
}

func NewStore(cfg config.StorageConfig) (Store, error) {
	backend, format := normalizeBackend(cfg)
	switch backend {
	case "file":
		return NewFileStore(cfg, format)
	case "sqlite":
		dsn := strings.TrimSpace(cfg.DSN)
		if dsn == "" {
			dsn = filepath.Join(cfg.Directory, "sqlite.db")
		}
		return NewSQLStore("sqlite", []string{dsn})
	case "postgres":
		return newSQLFromDSN("postgres", cfg.DSN)
	case "postgres-cluster":
		return newSQLCluster("postgres", cfg.DSN)
	case "mysql":
		return newSQLFromDSN("mysql", cfg.DSN)
	case "mysql-cluster":
		return newSQLCluster("mysql", cfg.DSN)
	case "redis":
		return NewRedisStore("redis", cfg.DSN, false)
	case "redis-cluster":
		return NewRedisStore("redis-cluster", cfg.DSN, true)
	case "keydb":
		return NewRedisStore("keydb", cfg.DSN, false)
	case "keydb-cluster":
		return NewRedisStore("keydb-cluster", cfg.DSN, true)
	default:
		return nil, fmt.Errorf("unknown storage backend: %q", backend)
	}
}

func newSQLFromDSN(driver, dsn string) (Store, error) {
	dsn = strings.TrimSpace(dsn)
	if dsn == "" {
		return nil, fmt.Errorf("%s dsn empty", driver)
	}
	return NewSQLStore(driver, []string{dsn})
}

func newSQLCluster(driver, dsn string) (Store, error) {
	list := splitCSV(dsn)
	if len(list) == 0 {
		return nil, fmt.Errorf("%s cluster dsn empty", driver)
	}
	return NewSQLStore(driver, list)
}

func normalizeBackend(cfg config.StorageConfig) (backend, format string) {
	backend = strings.ToLower(strings.TrimSpace(cfg.Backend))
	if backend == "" {
		backend = strings.ToLower(strings.TrimSpace(cfg.Mode))
	}
	if backend == "" {
		backend = "file"
	}
	if !cfg.DatabaseEnabled {
		if backend != "file" && backend != "file-json" && backend != "file-txt" {
			backend = "file"
		}
	}
	format = strings.ToLower(strings.TrimSpace(cfg.FileFormat))
	if strings.Contains(backend, "txt") {
		backend = "file"
		format = "txt"
	}
	if strings.Contains(backend, "json") {
		backend = "file"
		format = "json"
	}
	if format != "txt" {
		format = "json"
	}
	return backend, format
}

func NewFileStore(cfg config.StorageConfig, format string) (*FileStore, error) {
	baseDir := strings.TrimSpace(cfg.Directory)
	if baseDir == "" {
		baseDir = filepath.Join("data", "storage")
	}
	logDir := strings.TrimSpace(cfg.LogDir)
	if logDir == "" {
		logDir = filepath.Join(baseDir, "logs")
	}
	if format == "" {
		format = "json"
	}
	if err := os.MkdirAll(filepath.Join(baseDir, "players"), 0o755); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(logDir, 0o755); err != nil {
		return nil, err
	}
	maxSize := int64(cfg.LogMaxMB) * 1024 * 1024
	if maxSize <= 0 {
		maxSize = 5 * 1024 * 1024
	}
	maxFiles := cfg.LogMaxFiles
	if maxFiles <= 0 {
		maxFiles = 50
	}
	return &FileStore{
		baseDir:  baseDir,
		logDir:   logDir,
		format:   format,
		maxSize:  maxSize,
		maxFiles: maxFiles,
		logs:     make(map[string]*rotatingLog),
	}, nil
}

func (s *FileStore) Record(ev Event) error {
	return s.logEvent(ev)
}

func (s *FileStore) Log(kind string, data any) error {
	return s.logWithTimestamp(kind, time.Now().UTC(), data)
}

func (s *FileStore) UpsertPlayer(rec PlayerRecord) error {
	if s == nil {
		return nil
	}
	if strings.TrimSpace(rec.UUID) == "" {
		return errors.New("player uuid is empty")
	}
	if s.closed.Load() {
		return os.ErrClosed
	}
	s.playerMu.Lock()
	defer s.playerMu.Unlock()
	path, err := s.playerPath(rec.UUID)
	if err != nil {
		return err
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	cur := PlayerRecord{UUID: rec.UUID}
	if data, err := os.ReadFile(path); err == nil {
		if s.format == "txt" {
			cur = parsePlayerTXT(string(data), rec.UUID)
		} else {
			_ = json.Unmarshal(data, &cur)
		}
	}
	if cur.FirstSeen == "" {
		cur.FirstSeen = now
	}
	cur.LastSeen = now
	if rec.USID != "" {
		cur.USID = rec.USID
	}
	if rec.Name != "" {
		cur.Name = rec.Name
		cur.Names = appendUnique(cur.Names, rec.Name)
	}
	if rec.IP != "" {
		cur.IP = rec.IP
		cur.IPs = appendUnique(cur.IPs, rec.IP)
	}
	if rec.TimesJoined > 0 {
		cur.TimesJoined += rec.TimesJoined
	}
	if rec.TimesKicked > 0 {
		cur.TimesKicked += rec.TimesKicked
	}
	return s.writePlayer(path, cur)
}

func (s *FileStore) Close() error {
	if s.closed.Swap(true) {
		return nil
	}
	s.logsMu.Lock()
	for _, l := range s.logs {
		_ = l.Close()
	}
	s.logsMu.Unlock()
	if v := s.lastErr.Load(); v != nil {
		if err, ok := v.(error); ok {
			return err
		}
	}
	return nil
}

func (s *FileStore) Status() string {
	return fmt.Sprintf("file:%s logs=%s format=%s", s.baseDir, s.logDir, s.format)
}

func (s *FileStore) logEvent(ev Event) error {
	kind := mapEventKind(ev.Kind)
	return s.logWithTimestamp(kind, ev.Timestamp, ev)
}

func (s *FileStore) logWithTimestamp(kind string, ts time.Time, data any) error {
	if s == nil {
		return nil
	}
	if s.closed.Load() {
		return os.ErrClosed
	}
	if kind == "" {
		kind = "events"
	}
	logger, err := s.getLogger(kind)
	if err != nil {
		s.lastErr.Store(err)
		return err
	}
	if ts.IsZero() {
		ts = time.Now().UTC()
	}
	if s.format == "txt" {
		payload := formatTextPayload(data)
		line := fmt.Sprintf("%s [%s] %s\n", ts.UTC().Format(time.RFC3339Nano), kind, payload)
		return logger.WriteLine([]byte(line))
	}
	entry := LogEntry{
		Timestamp: ts.UTC().Format(time.RFC3339Nano),
		Kind:      kind,
		Data:      data,
	}
	raw, err := json.Marshal(entry)
	if err != nil {
		s.lastErr.Store(err)
		return err
	}
	raw = append(raw, '\n')
	return logger.WriteLine(raw)
}

func (s *FileStore) getLogger(kind string) (*rotatingLog, error) {
	key := sanitizeFilename(kind)
	s.logsMu.Lock()
	defer s.logsMu.Unlock()
	if l, ok := s.logs[key]; ok {
		return l, nil
	}
	ext := ".jsonl"
	if s.format == "txt" {
		ext = ".txt"
	}
	l, err := newRotatingLog(s.logDir, key, ext, s.maxSize, s.maxFiles)
	if err != nil {
		return nil, err
	}
	s.logs[key] = l
	return l, nil
}

func (s *FileStore) playerPath(uuid string) (string, error) {
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return "", errors.New("player uuid empty")
	}
	ext := ".json"
	if s.format == "txt" {
		ext = ".txt"
	}
	dir := filepath.Join(s.baseDir, "players")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return filepath.Join(dir, sanitizeFilename(uuid)+ext), nil
}

func (s *FileStore) writePlayer(path string, rec PlayerRecord) error {
	var data []byte
	var err error
	if s.format == "txt" {
		data = []byte(formatPlayerTXT(rec))
	} else {
		data, err = json.MarshalIndent(rec, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
	}
	return os.WriteFile(path, data, 0o644)
}

func mapEventKind(kind string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "packet_send", "packet_recv":
		return kind
	case "connect_packet", "connect_confirm", "connect_confirm_client_snapshot", "tcp_accept", "tcp_closed", "disconnect":
		return "player"
	case "world_handshake_sent", "world_hot_reload_failed":
		return "map"
	default:
		if kind == "" {
			return "events"
		}
		return kind
	}
}

func formatTextPayload(data any) string {
	switch v := data.(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		raw, err := json.Marshal(v)
		if err != nil {
			return fmt.Sprintf("%v", v)
		}
		return string(raw)
	}
}

func appendUnique(items []string, value string) []string {
	for _, v := range items {
		if v == value {
			return items
		}
	}
	return append(items, value)
}

func parsePlayerTXT(in, uuid string) PlayerRecord {
	out := PlayerRecord{UUID: uuid}
	lines := strings.Split(in, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		val := strings.TrimSpace(parts[1])
		switch key {
		case "uuid":
			out.UUID = val
		case "usid":
			out.USID = val
		case "name":
			out.Name = val
		case "ip":
			out.IP = val
		case "first_seen":
			out.FirstSeen = val
		case "last_seen":
			out.LastSeen = val
		case "times_joined":
			out.TimesJoined = int(parseIntSafe(val))
		case "times_kicked":
			out.TimesKicked = int(parseIntSafe(val))
		case "names":
			out.Names = splitCSV(val)
		case "ips":
			out.IPs = splitCSV(val)
		}
	}
	return out
}

func formatPlayerTXT(rec PlayerRecord) string {
	b := &strings.Builder{}
	writeKV(b, "uuid", rec.UUID)
	writeKV(b, "usid", rec.USID)
	writeKV(b, "name", rec.Name)
	writeKV(b, "ip", rec.IP)
	writeKV(b, "first_seen", rec.FirstSeen)
	writeKV(b, "last_seen", rec.LastSeen)
	writeKV(b, "times_joined", fmt.Sprintf("%d", rec.TimesJoined))
	writeKV(b, "times_kicked", fmt.Sprintf("%d", rec.TimesKicked))
	writeKV(b, "names", strings.Join(rec.Names, ","))
	writeKV(b, "ips", strings.Join(rec.IPs, ","))
	return b.String()
}

func writeKV(b *strings.Builder, key, val string) {
	if strings.TrimSpace(val) == "" {
		return
	}
	_, _ = b.WriteString(key)
	_, _ = b.WriteString("=")
	_, _ = b.WriteString(val)
	_, _ = b.WriteString("\n")
}

func splitCSV(val string) []string {
	val = strings.TrimSpace(val)
	if val == "" {
		return nil
	}
	parts := strings.Split(val, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseIntSafe(v string) int64 {
	n, _ := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
	return n
}
