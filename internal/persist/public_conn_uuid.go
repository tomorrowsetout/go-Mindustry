package persist

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type PublicConnUUIDRecord struct {
	ConnUUID  string `json:"conn_uuid"`
	Name      string `json:"name,omitempty"`
	IP        string `json:"ip,omitempty"`
	FirstSeen string `json:"first_seen,omitempty"`
	LastSeen  string `json:"last_seen,omitempty"`
}

type PublicConnUUIDFile struct {
	Version   int                              `json:"version"`
	UpdatedAt string                           `json:"updated_at"`
	ByUUID    map[string]*PublicConnUUIDRecord `json:"by_uuid"`
}

type PublicConnUUIDStore struct {
	mu         sync.Mutex
	path       string
	autoCreate bool
	rng        *rand.Rand
	data       PublicConnUUIDFile
}

type legacyPublicConnUUIDRecord struct {
	UUID      string   `json:"uuid"`
	ConnUUID  string   `json:"conn_uuid"`
	Name      string   `json:"name"`
	IP        string   `json:"ip"`
	Names     []string `json:"names"`
	IPs       []string `json:"ips"`
	FirstSeen string   `json:"first_seen"`
	LastSeen  string   `json:"last_seen"`
}

type legacyPublicConnUUIDFile struct {
	Version   int                                    `json:"version"`
	UpdatedAt string                                 `json:"updated_at"`
	ByUUID    map[string]*legacyPublicConnUUIDRecord `json:"by_uuid"`
}

func NewPublicConnUUIDStore(path string, autoCreate bool) (*PublicConnUUIDStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("empty conn_uuid path")
	}
	if autoCreate {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	s := &PublicConnUUIDStore{
		path:       path,
		autoCreate: autoCreate,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
		data: PublicConnUUIDFile{
			Version: 1,
			ByUUID:  map[string]*PublicConnUUIDRecord{},
		},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PublicConnUUIDStore) Ensure(uuid, name, ip string) (string, error) {
	if s == nil {
		return "", nil
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return "", nil
	}
	name = strings.TrimSpace(name)
	ip = strings.TrimSpace(ip)
	now := compactTimestamp(time.Now().UTC())

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.ByUUID == nil {
		s.data.ByUUID = map[string]*PublicConnUUIDRecord{}
	}
	rec := s.data.ByUUID[uuid]
	if rec == nil && !s.autoCreate {
		return "", nil
	}
	if rec == nil {
		rec = &PublicConnUUIDRecord{
			FirstSeen: now,
		}
		s.data.ByUUID[uuid] = rec
	}
	if strings.TrimSpace(rec.ConnUUID) == "" && !s.autoCreate {
		return "", nil
	}
	if strings.TrimSpace(rec.ConnUUID) == "" {
		rec.ConnUUID = s.generateConnUUIDLocked()
	}
	if !s.autoCreate {
		return strings.TrimSpace(rec.ConnUUID), nil
	}
	if rec.FirstSeen == "" {
		rec.FirstSeen = now
	}
	rec.LastSeen = now
	if name != "" {
		rec.Name = name
	}
	if ip != "" {
		rec.IP = ip
	}
	s.data.UpdatedAt = now
	return rec.ConnUUID, s.flushLocked()
}

func (s *PublicConnUUIDStore) Lookup(uuid string) (string, bool) {
	if s == nil {
		return "", false
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return "", false
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	rec := s.data.ByUUID[uuid]
	if rec == nil || strings.TrimSpace(rec.ConnUUID) == "" {
		return "", false
	}
	return strings.TrimSpace(rec.ConnUUID), true
}

func (s *PublicConnUUIDStore) ObserveDisconnect(uuid, name, ip string) error {
	if s == nil {
		return nil
	}
	uuid = strings.TrimSpace(uuid)
	if uuid == "" {
		return nil
	}
	name = strings.TrimSpace(name)
	ip = strings.TrimSpace(ip)
	now := compactTimestamp(time.Now().UTC())

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.data.ByUUID == nil {
		s.data.ByUUID = map[string]*PublicConnUUIDRecord{}
	}
	if !s.autoCreate {
		return nil
	}
	rec := s.data.ByUUID[uuid]
	if rec == nil {
		rec = &PublicConnUUIDRecord{
			FirstSeen: now,
			ConnUUID:  s.generateConnUUIDLocked(),
		}
		s.data.ByUUID[uuid] = rec
	}
	rec.LastSeen = now
	if name != "" {
		rec.Name = name
	}
	if ip != "" {
		rec.IP = ip
	}
	s.data.UpdatedAt = now
	return s.flushLocked()
}

func (s *PublicConnUUIDStore) load() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			if !s.autoCreate {
				return nil
			}
			return s.flushLocked()
		}
		return err
	}
	var legacy legacyPublicConnUUIDFile
	if err := json.Unmarshal(raw, &legacy); err != nil {
		return err
	}
	if legacy.Version <= 0 {
		legacy.Version = 1
	}
	parsed := PublicConnUUIDFile{
		Version:   legacy.Version,
		UpdatedAt: normalizeTimestampString(legacy.UpdatedAt),
		ByUUID:    map[string]*PublicConnUUIDRecord{},
	}
	for uuid, rec := range legacy.ByUUID {
		uuid = strings.TrimSpace(uuid)
		if uuid == "" || rec == nil {
			continue
		}
		name := strings.TrimSpace(rec.Name)
		if name == "" && len(rec.Names) > 0 {
			name = strings.TrimSpace(rec.Names[len(rec.Names)-1])
		}
		ip := strings.TrimSpace(rec.IP)
		if ip == "" && len(rec.IPs) > 0 {
			ip = strings.TrimSpace(rec.IPs[len(rec.IPs)-1])
		}
		parsed.ByUUID[uuid] = &PublicConnUUIDRecord{
			ConnUUID:  strings.TrimSpace(rec.ConnUUID),
			Name:      name,
			IP:        ip,
			FirstSeen: normalizeTimestampString(rec.FirstSeen),
			LastSeen:  normalizeTimestampString(rec.LastSeen),
		}
	}
	s.data = parsed
	return nil
}

func (s *PublicConnUUIDStore) flushLocked() error {
	if !s.autoCreate {
		return nil
	}
	if s.data.ByUUID == nil {
		s.data.ByUUID = map[string]*PublicConnUUIDRecord{}
	}
	s.data.Version = 1
	s.data.UpdatedAt = compactTimestamp(time.Now().UTC())
	raw, err := json.MarshalIndent(s.data, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *PublicConnUUIDStore) generateConnUUIDLocked() string {
	used := make(map[string]struct{}, len(s.data.ByUUID))
	usedByDigits := make(map[int]int64, 7)
	for _, rec := range s.data.ByUUID {
		id := strings.TrimSpace(rec.ConnUUID)
		if id != "" {
			if _, ok := used[id]; ok {
				continue
			}
			used[id] = struct{}{}
			digits := len(id)
			if digits >= 5 && digits <= 11 {
				usedByDigits[digits]++
			}
		}
	}
	for digits := 5; digits <= 11; digits++ {
		min := pow10(digits - 1)
		span := 9 * min
		if usedByDigits[digits] >= span {
			continue
		}
		start := s.rng.Int63n(span)
		for offset := int64(0); offset < span; offset++ {
			n := min + ((start + offset) % span)
			id := fmt.Sprintf("%d", n)
			if _, exists := used[id]; exists {
				continue
			}
			return id
		}
	}
	return fmt.Sprintf("%d", time.Now().UnixNano()%90000000000+10000000000)
}

func pow10(n int) int64 {
	out := int64(1)
	for i := 0; i < n; i++ {
		out *= 10
	}
	return out
}

func compactTimestamp(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func normalizeTimestampString(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if t, err := time.Parse(time.RFC3339Nano, raw); err == nil {
		return compactTimestamp(t)
	}
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return compactTimestamp(t)
	}
	return raw
}
