package persist

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type PlayerIdentityRecord struct {
	Bound  bool   `json:"bound"`
	Title  string `json:"title,omitempty"`
	Prefix string `json:"prefix,omitempty"`
	Suffix string `json:"suffix,omitempty"`
}

type PlayerIdentityFile struct {
	Version    int                              `json:"version"`
	ByConnUUID map[string]*PlayerIdentityRecord `json:"by_conn_uuid"`
}

type PlayerIdentityStore struct {
	mu         sync.RWMutex
	path       string
	autoCreate bool
	data       PlayerIdentityFile
}

func NewPlayerIdentityStore(path string, autoCreate bool) (*PlayerIdentityStore, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("empty player identity path")
	}
	if autoCreate {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	s := &PlayerIdentityStore{
		path:       path,
		autoCreate: autoCreate,
		data: PlayerIdentityFile{
			Version:    1,
			ByConnUUID: map[string]*PlayerIdentityRecord{},
		},
	}
	if err := s.load(); err != nil {
		return nil, err
	}
	return s, nil
}

func (s *PlayerIdentityStore) Lookup(connUUID string) (PlayerIdentityRecord, bool) {
	if s == nil {
		return PlayerIdentityRecord{}, false
	}
	connUUID = strings.TrimSpace(connUUID)
	if connUUID == "" {
		return PlayerIdentityRecord{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	rec := s.data.ByConnUUID[connUUID]
	if rec == nil {
		return PlayerIdentityRecord{}, false
	}
	return *rec, true
}

func (s *PlayerIdentityStore) Ensure(connUUID string) (PlayerIdentityRecord, bool, error) {
	if s == nil {
		return PlayerIdentityRecord{}, false, nil
	}
	connUUID = strings.TrimSpace(connUUID)
	if connUUID == "" {
		return PlayerIdentityRecord{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.data.ByConnUUID == nil {
		s.data.ByConnUUID = map[string]*PlayerIdentityRecord{}
	}
	if rec := s.data.ByConnUUID[connUUID]; rec != nil {
		return *rec, true, nil
	}
	if !s.autoCreate {
		return PlayerIdentityRecord{}, false, nil
	}
	rec := &PlayerIdentityRecord{
		Bound:  false,
		Title:  "",
		Prefix: "",
		Suffix: "",
	}
	s.data.ByConnUUID[connUUID] = rec
	return *rec, true, s.flushLocked()
}

func (s *PlayerIdentityStore) load() error {
	raw, err := os.ReadFile(s.path)
	if err != nil {
		if os.IsNotExist(err) {
			if s.autoCreate {
				return s.flushEmpty()
			}
			return nil
		}
		return err
	}
	var parsed PlayerIdentityFile
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return err
	}
	if parsed.Version <= 0 {
		parsed.Version = 1
	}
	if parsed.ByConnUUID == nil {
		parsed.ByConnUUID = map[string]*PlayerIdentityRecord{}
	}
	normalized := make(map[string]*PlayerIdentityRecord, len(parsed.ByConnUUID))
	for connUUID, rec := range parsed.ByConnUUID {
		connUUID = strings.TrimSpace(connUUID)
		if connUUID == "" || rec == nil {
			continue
		}
		normalized[connUUID] = &PlayerIdentityRecord{
			Bound:  rec.Bound,
			Title:  strings.TrimSpace(rec.Title),
			Prefix: strings.TrimSpace(rec.Prefix),
			Suffix: strings.TrimSpace(rec.Suffix),
		}
	}
	parsed.ByConnUUID = normalized
	s.data = parsed
	return nil
}

func (s *PlayerIdentityStore) flushEmpty() error {
	return s.flushLocked()
}

func (s *PlayerIdentityStore) flushLocked() error {
	if s.data.ByConnUUID == nil {
		s.data.ByConnUUID = map[string]*PlayerIdentityRecord{}
	}
	if s.data.Version <= 0 {
		s.data.Version = 1
	}
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
