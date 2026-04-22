package net

import (
	"encoding/json"
	"fmt"
	"net"
	"os"
	"sort"
	"strings"
	"time"

	"mdt-server/internal/protocol"
)

const connectRejectDelay = 250 * time.Millisecond

type AdmissionWhitelistEntry struct {
	UUID string `json:"uuid"`
	USID string `json:"usid,omitempty"`
}

type AdmissionPolicy struct {
	StrictIdentity     bool
	AllowCustomClients bool
	PlayerLimit        int
	WhitelistEnabled   bool
	Whitelist          []AdmissionWhitelistEntry
	ExpectedMods       []string
	BannedNames        []string
	BannedSubnets      []string
	RecentKickDuration time.Duration
}

func DefaultAdmissionPolicy() AdmissionPolicy {
	return AdmissionPolicy{
		StrictIdentity:     true,
		AllowCustomClients: false,
		PlayerLimit:        0,
		WhitelistEnabled:   false,
		ExpectedMods:       nil,
		RecentKickDuration: 30 * time.Second,
	}
}

func LoadAdmissionWhitelistFile(path string) ([]AdmissionWhitelistEntry, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var entries []AdmissionWhitelistEntry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return nil, err
	}
	out := make([]AdmissionWhitelistEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		uuid := strings.TrimSpace(entry.UUID)
		usid := strings.TrimSpace(entry.USID)
		if uuid == "" {
			continue
		}
		key := uuid + "\x00" + usid
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, AdmissionWhitelistEntry{UUID: uuid, USID: usid})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UUID != out[j].UUID {
			return out[i].UUID < out[j].UUID
		}
		return out[i].USID < out[j].USID
	})
	return out, nil
}

func normalizeAdmissionPolicy(policy AdmissionPolicy) AdmissionPolicy {
	def := DefaultAdmissionPolicy()
	if policy.PlayerLimit < 0 {
		policy.PlayerLimit = 0
	}
	if policy.RecentKickDuration < 0 {
		policy.RecentKickDuration = 0
	}
	if policy.RecentKickDuration == 0 {
		policy.RecentKickDuration = def.RecentKickDuration
	}
	policy.ExpectedMods = normalizeStringSlice(policy.ExpectedMods)
	policy.BannedNames = normalizeStringSlice(policy.BannedNames)
	policy.BannedSubnets = normalizeStringSlice(policy.BannedSubnets)
	policy.Whitelist = normalizeWhitelistEntries(policy.Whitelist)
	return policy
}

func normalizeStringSlice(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func normalizeWhitelistEntries(entries []AdmissionWhitelistEntry) []AdmissionWhitelistEntry {
	if len(entries) == 0 {
		return nil
	}
	out := make([]AdmissionWhitelistEntry, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		uuid := strings.TrimSpace(entry.UUID)
		usid := strings.TrimSpace(entry.USID)
		if uuid == "" {
			continue
		}
		key := uuid + "\x00" + usid
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, AdmissionWhitelistEntry{UUID: uuid, USID: usid})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].UUID != out[j].UUID {
			return out[i].UUID < out[j].UUID
		}
		return out[i].USID < out[j].USID
	})
	return out
}

func normalizeAdmissionName(name string) string {
	return strings.ToLower(strings.TrimSpace(StripMindustryColorTags(name)))
}

func admissionNameBanned(policy AdmissionPolicy, name string) bool {
	name = normalizeAdmissionName(name)
	if name == "" {
		return false
	}
	for _, banned := range policy.BannedNames {
		if normalizeAdmissionName(banned) == name {
			return true
		}
	}
	return false
}

func admissionSubnetBanned(policy AdmissionPolicy, ip string) bool {
	ip = strings.TrimSpace(ip)
	if ip == "" || len(policy.BannedSubnets) == 0 {
		return false
	}
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, subnet := range policy.BannedSubnets {
		_, block, err := net.ParseCIDR(strings.TrimSpace(subnet))
		if err != nil || block == nil {
			continue
		}
		if block.Contains(parsed) {
			return true
		}
	}
	return false
}

func (s *Server) SetAdmissionPolicy(policy AdmissionPolicy) {
	if s == nil {
		return
	}
	policy = normalizeAdmissionPolicy(policy)
	s.admissionMu.Lock()
	s.admission = policy
	if s.recentKickUntil == nil {
		s.recentKickUntil = make(map[string]time.Time)
	}
	s.admissionMu.Unlock()
}

func (s *Server) admissionPolicy() AdmissionPolicy {
	if s == nil {
		return DefaultAdmissionPolicy()
	}
	s.admissionMu.RLock()
	policy := s.admission
	s.admissionMu.RUnlock()
	return normalizeAdmissionPolicy(policy)
}

func (s *Server) rejectConnect(c *Conn, kickType *protocol.KickReason, reason string) {
	if s == nil || c == nil {
		return
	}
	var err error
	switch {
	case kickType != nil:
		err = c.SendAsync(&protocol.Remote_NetClient_kick_21{Reason: *kickType})
	case strings.TrimSpace(reason) != "":
		err = c.SendAsync(&protocol.Remote_NetClient_kick_22{Reason: reason})
	default:
		fallback := protocol.KickReasonKick
		err = c.SendAsync(&protocol.Remote_NetClient_kick_21{Reason: fallback})
	}
	if err != nil {
		_ = c.Close()
		return
	}
	s.closeConnLater(c, connectRejectDelay)
}

func (s *Server) noteRecentKick(uuid, ip string) {
	if s == nil {
		return
	}
	policy := s.admissionPolicy()
	if policy.RecentKickDuration <= 0 {
		return
	}
	expires := time.Now().Add(policy.RecentKickDuration)
	s.admissionMu.Lock()
	if s.recentKickUntil == nil {
		s.recentKickUntil = make(map[string]time.Time)
	}
	pruneRecentKickLocked(s.recentKickUntil, time.Now())
	if uuid = strings.TrimSpace(uuid); uuid != "" {
		s.recentKickUntil["uuid:"+uuid] = expires
	}
	if ip = strings.TrimSpace(ip); ip != "" {
		s.recentKickUntil["ip:"+ip] = expires
	}
	s.admissionMu.Unlock()
}

func (s *Server) isRecentlyKicked(uuid, ip string) bool {
	if s == nil {
		return false
	}
	now := time.Now()
	s.admissionMu.Lock()
	defer s.admissionMu.Unlock()
	pruneRecentKickLocked(s.recentKickUntil, now)
	if uuid = strings.TrimSpace(uuid); uuid != "" {
		if until, ok := s.recentKickUntil["uuid:"+uuid]; ok && now.Before(until) {
			return true
		}
	}
	if ip = strings.TrimSpace(ip); ip != "" {
		if until, ok := s.recentKickUntil["ip:"+ip]; ok && now.Before(until) {
			return true
		}
	}
	return false
}

func pruneRecentKickLocked(entries map[string]time.Time, now time.Time) {
	if len(entries) == 0 {
		return
	}
	for key, until := range entries {
		if !now.Before(until) {
			delete(entries, key)
		}
	}
}

func (s *Server) activePlayerCount() int {
	if s == nil {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for c := range s.conns {
		if c == nil || c.hasDisconnected || !c.hasConnected {
			continue
		}
		total++
	}
	return total
}

func (s *Server) hasDuplicateIdentity(c *Conn, uuid, usid, name string) (protocol.KickReason, bool) {
	if s == nil || c == nil {
		return 0, false
	}
	wantName := normalizeAdmissionName(name)
	s.mu.Lock()
	defer s.mu.Unlock()
	for peer := range s.conns {
		if peer == nil || peer == c || peer.hasDisconnected || !peer.hasBegunConnecting {
			continue
		}
		if wantName != "" && normalizeAdmissionName(peer.rawName) == wantName {
			return protocol.KickReasonNameInUse, true
		}
		if uuid != "" && strings.TrimSpace(peer.uuid) == uuid {
			return protocol.KickReasonIDInUse, true
		}
		if usid != "" && strings.TrimSpace(peer.usid) == usid {
			return protocol.KickReasonIDInUse, true
		}
	}
	return 0, false
}

func whitelistAllows(entries []AdmissionWhitelistEntry, uuid, usid string) bool {
	uuid = strings.TrimSpace(uuid)
	usid = strings.TrimSpace(usid)
	if uuid == "" {
		return false
	}
	for _, entry := range entries {
		if entry.UUID != uuid {
			continue
		}
		if entry.USID == "" || entry.USID == usid {
			return true
		}
	}
	return false
}

func incompatibleModsMessage(expected, actual []string) (string, bool) {
	expected = normalizeStringSlice(expected)
	actual = normalizeStringSlice(actual)

	expectedSet := make(map[string]struct{}, len(expected))
	for _, mod := range expected {
		expectedSet[mod] = struct{}{}
	}
	actualSet := make(map[string]struct{}, len(actual))
	for _, mod := range actual {
		actualSet[mod] = struct{}{}
	}

	missing := make([]string, 0, len(expected))
	for _, mod := range expected {
		if _, ok := actualSet[mod]; !ok {
			missing = append(missing, mod)
		}
	}
	extra := make([]string, 0, len(actual))
	for _, mod := range actual {
		if _, ok := expectedSet[mod]; !ok {
			extra = append(extra, mod)
		}
	}
	if len(missing) == 0 && len(extra) == 0 {
		return "", false
	}
	var b strings.Builder
	b.WriteString("[accent]Incompatible mods![]\n\n")
	if len(missing) > 0 {
		b.WriteString("Missing:[lightgray]\n> ")
		b.WriteString(strings.Join(missing, "\n> "))
		b.WriteString("[]\n")
	}
	if len(extra) > 0 {
		b.WriteString("Unnecessary mods:[lightgray]\n> ")
		b.WriteString(strings.Join(extra, "\n> "))
	}
	return b.String(), true
}

func formatConnectRejectLog(connID int32, reason string, err error) string {
	if err != nil {
		return fmt.Sprintf("[net] connect rejected id=%d reason=%s err=%v\n", connID, reason, err)
	}
	return fmt.Sprintf("[net] connect rejected id=%d reason=%s\n", connID, reason)
}
