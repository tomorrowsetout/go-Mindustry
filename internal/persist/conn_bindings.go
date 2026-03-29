package persist

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type ConnBindingRecord struct {
	UUID       string   `json:"uuid"`
	LastConnID int32    `json:"last_conn_id"`
	ConnIDs    []int32  `json:"conn_ids,omitempty"`
	Names      []string `json:"names,omitempty"`
	IPs        []string `json:"ips,omitempty"`
	FirstSeen  string   `json:"first_seen,omitempty"`
	LastSeen   string   `json:"last_seen,omitempty"`
	Sessions   int64    `json:"sessions"`
}

type ConnBindingsFile struct {
	Version   int                           `json:"version"`
	UpdatedAt string                        `json:"updated_at"`
	ByUUID    map[string]*ConnBindingRecord `json:"by_uuid"`
	ByConnID  map[string]string             `json:"by_conn_id"`
}

type ConnBindingStore struct {
	mu   sync.Mutex
	path string
	data ConnBindingsFile
}

func NewConnBindingStore(configDir string) (*ConnBindingStore, error) {
	base := strings.TrimSpace(configDir)
	if base == "" {
		base = "configs"
	}
	dir := filepath.Join(base, "persistent", "public")
	path := filepath.Join(dir, "conn_uuid_bindings.json")
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}

	s := &ConnBindingStore{
		path: path,
		data: ConnBindingsFile{
			Version:  1,
			ByUUID:   map[string]*ConnBindingRecord{},
			ByConnID: map[string]string{},
		},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *ConnBindingStore) ObserveConnect(connID int32, uuid, name, ip string) {
	s.observe(connID, uuid, name, ip, true)
}

func (s *ConnBindingStore) ObserveDisconnect(connID int32, uuid, name, ip string) {
	s.observe(connID, uuid, name, ip, false)
}

func (s *ConnBindingStore) observe(connID int32, uuid, name, ip string, countSession bool) {
	if s == nil {
		return
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" || connID == 0 {
		return
	}
	name = strings.TrimSpace(name)
	ip = strings.TrimSpace(ip)
	now := time.Now().UTC().Format(time.RFC3339Nano)

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.ByUUID == nil {
		s.data.ByUUID = map[string]*ConnBindingRecord{}
	}
	if s.data.ByConnID == nil {
		s.data.ByConnID = map[string]string{}
	}
	rec := s.data.ByUUID[uuid]
	if rec == nil {
		rec = &ConnBindingRecord{
			UUID:      uuid,
			FirstSeen: now,
		}
		s.data.ByUUID[uuid] = rec
	}
	rec.LastSeen = now
	rec.LastConnID = connID
	if countSession {
		rec.Sessions++
	}
	rec.ConnIDs = appendInt32UniqueCap(rec.ConnIDs, connID, 128)
	if name != "" {
		rec.Names = appendStringUniqueCap(rec.Names, name, 32)
	}
	if ip != "" {
		rec.IPs = appendStringUniqueCap(rec.IPs, ip, 32)
	}
	s.data.ByConnID[strconv.FormatInt(int64(connID), 10)] = uuid
	s.data.UpdatedAt = now
	_ = s.flushLocked()
}

func (s *ConnBindingStore) load() error {
	data, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			return s.flushLocked()
		}
		return err
	}
	var parsed ConnBindingsFile
	if err := json.Unmarshal(data, &parsed); err != nil {
		return err
	}
	if parsed.Version <= 0 {
		parsed.Version = 1
	}
	if parsed.ByUUID == nil {
		parsed.ByUUID = map[string]*ConnBindingRecord{}
	}
	if parsed.ByConnID == nil {
		parsed.ByConnID = map[string]string{}
	}
	s.data = parsed
	return nil
}

func (s *ConnBindingStore) flushLocked() error {
	if s == nil {
		return nil
	}
	if s.data.ByUUID == nil {
		s.data.ByUUID = map[string]*ConnBindingRecord{}
	}
	if s.data.ByConnID == nil {
		s.data.ByConnID = map[string]string{}
	}
	s.data.Version = 1
	if strings.TrimSpace(s.data.UpdatedAt) == "" {
		s.data.UpdatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	// Stable order for easier diffs.
	for _, rec := range s.data.ByUUID {
		sort.Slice(rec.ConnIDs, func(i, j int) bool { return rec.ConnIDs[i] < rec.ConnIDs[j] })
		sort.Strings(rec.Names)
		sort.Strings(rec.IPs)
	}
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func appendInt32UniqueCap(in []int32, v int32, capMax int) []int32 {
	for _, it := range in {
		if it == v {
			return in
		}
	}
	in = append(in, v)
	if capMax > 0 && len(in) > capMax {
		in = in[len(in)-capMax:]
	}
	return in
}

func appendStringUniqueCap(in []string, v string, capMax int) []string {
	for _, it := range in {
		if strings.EqualFold(it, v) {
			return in
		}
	}
	in = append(in, v)
	if capMax > 0 && len(in) > capMax {
		in = in[len(in)-capMax:]
	}
	return in
}
