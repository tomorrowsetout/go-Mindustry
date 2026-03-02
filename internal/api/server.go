package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"mdt-server/internal/config"
	netserver "mdt-server/internal/net"
	"mdt-server/internal/sim"
)

type Server struct {
	cfg    config.APIConfig
	srv    *netserver.Server
	start  time.Time
	engine func() *sim.TickStats
	http   *http.Server
	keyMu  sync.RWMutex
	keys   map[string]struct{}
	stopFn func()
	// typeID, x, y, team
	summonFn     func(int16, float32, float32, byte) error
	moveFn       func(int32, float32, float32, float32) error
	teleportFn   func(int32, float32, float32, float32) error
	lifeFn       func(int32, float32) error
	followFn     func(int32, int32, float32) error
	patrolFn     func(int32, float32, float32, float32, float32, float32) error
	behaviorFn   func(int32, string) error
	opsChangedFn func()
}

func New(cfg config.APIConfig, srv *netserver.Server, engineStats func() *sim.TickStats) *Server {
	s := &Server{
		cfg:    cfg,
		srv:    srv,
		start:  time.Now(),
		engine: engineStats,
		keys:   map[string]struct{}{},
	}
	for _, k := range cfg.Keys {
		k = strings.TrimSpace(k)
		if k != "" {
			s.keys[k] = struct{}{}
		}
	}
	if k := strings.TrimSpace(cfg.Key); k != "" {
		s.keys[k] = struct{}{}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.withAuth(s.handleHealth))
	mux.HandleFunc("/help", s.withAuth(s.handleHelp))
	mux.HandleFunc("/status", s.withAuth(s.handleStatus))
	mux.HandleFunc("/players", s.withAuth(s.handlePlayers))
	mux.HandleFunc("/kick", s.withAuth(s.handleKick))
	mux.HandleFunc("/summon", s.withAuth(s.handleSummon))
	mux.HandleFunc("/unit/move", s.withAuth(s.handleUnitMove))
	mux.HandleFunc("/unit/teleport", s.withAuth(s.handleUnitTeleport))
	mux.HandleFunc("/unit/life", s.withAuth(s.handleUnitLife))
	mux.HandleFunc("/unit/follow", s.withAuth(s.handleUnitFollow))
	mux.HandleFunc("/unit/patrol", s.withAuth(s.handleUnitPatrol))
	mux.HandleFunc("/unit/behavior", s.withAuth(s.handleUnitBehavior))
	mux.HandleFunc("/stop", s.withAuth(s.handleStop))
	mux.HandleFunc("/ban", s.withAuth(s.handleBan))
	mux.HandleFunc("/unban", s.withAuth(s.handleUnban))
	mux.HandleFunc("/bans", s.withAuth(s.handleBans))
	mux.HandleFunc("/ops", s.withAuth(s.handleOps))
	s.http = &http.Server{
		Addr:    cfg.Bind,
		Handler: mux,
	}
	return s
}

func (s *Server) Serve() error {
	if !s.cfg.Enabled {
		return nil
	}
	if s.http == nil {
		return errors.New("api server not initialized")
	}
	return s.http.ListenAndServe()
}

func (s *Server) withAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !s.cfg.Enabled {
			http.Error(w, "api disabled", http.StatusForbidden)
			return
		}
		if s.requiresAuth() {
			key := r.Header.Get("X-API-Key")
			if key == "" {
				auth := r.Header.Get("Authorization")
				if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
					key = strings.TrimSpace(auth[7:])
				}
			}
			if !s.HasAPIKey(key) {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":  true,
		"ts":  time.Now().UTC().Format(time.RFC3339),
		"api": "mdt-server",
	})
}

func (s *Server) handleHelp(w http.ResponseWriter, r *http.Request) {
	category := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("category")))
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	size, _ := strconv.Atoi(r.URL.Query().Get("size"))
	if page <= 0 {
		page = 1
	}
	if size <= 0 {
		size = 12
	}
	if size > 100 {
		size = 100
	}
	all := apiHelpCategories()
	if category == "" || category == "all" {
		writeJSON(w, http.StatusOK, map[string]any{
			"categories": all,
		})
		return
	}
	lines, ok := all[category]
	if !ok {
		http.Error(w, "unknown help category", http.StatusBadRequest)
		return
	}
	start := (page - 1) * size
	if start > len(lines) {
		start = len(lines)
	}
	end := start + size
	if end > len(lines) {
		end = len(lines)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"category":   category,
		"page":       page,
		"page_size":  size,
		"total":      len(lines),
		"total_page": (len(lines) + size - 1) / size,
		"items":      lines[start:end],
	})
}

func (s *Server) handleStatus(w http.ResponseWriter, _ *http.Request) {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	sessions := s.srv.ListSessions()
	var totalSent, totalErr, totalQueued, totalFull, totalBytes, totalUdpSent, totalUdpErr int64
	byTypeSent := map[string]int64{}
	byTypeBytes := map[string]int64{}
	for _, sess := range sessions {
		totalSent += sess.Stats.Sent
		totalErr += sess.Stats.SendErrors
		totalQueued += sess.Stats.Queued
		totalFull += sess.Stats.QueueFull
		totalBytes += sess.Stats.BytesSent
		totalUdpSent += sess.Stats.UdpSent
		totalUdpErr += sess.Stats.UdpErrors
		for k, v := range sess.Stats.ByTypeSent {
			byTypeSent[k] += v
		}
		for k, v := range sess.Stats.ByTypeBytes {
			byTypeBytes[k] += v
		}
	}
	resp := map[string]any{
		"time":             time.Now().UTC().Format(time.RFC3339),
		"uptime_seconds":   int64(time.Since(s.start).Seconds()),
		"goroutines":       runtime.NumGoroutine(),
		"mem_alloc_mb":     float64(ms.Alloc) / 1024 / 1024,
		"mem_sys_mb":       float64(ms.Sys) / 1024 / 1024,
		"sessions":         len(sessions),
		"build_version":    s.srv.BuildVersion,
		"server_name":      s.srv.Name,
		"map":              safeMapName(s.srv),
		"listen_address":   s.srv.Addr,
		"api_bind":         s.cfg.Bind,
		"api_auth":         s.requiresAuth(),
		"api_keys":         len(s.ListAPIKeys()),
		"protocol_packets": s.srv.Registry.Count(),
		"send": map[string]any{
			"sent":          totalSent,
			"errors":        totalErr,
			"queued":        totalQueued,
			"queue_full":    totalFull,
			"bytes":         totalBytes,
			"udp_sent":      totalUdpSent,
			"udp_errors":    totalUdpErr,
			"by_type_sent":  byTypeSent,
			"by_type_bytes": byTypeBytes,
		},
	}
	if s.engine != nil {
		if stats := s.engine(); stats != nil {
			resp["tick"] = stats.Tick
			resp["tps"] = stats.TPS
			resp["last_tick"] = stats.LastTickTime.UTC().Format(time.RFC3339Nano)
			resp["last_duration_ms"] = stats.LastDuration.Milliseconds()
			resp["overrun"] = stats.Overrun
			resp["partitions"] = stats.Partitions
			resp["total_work"] = stats.TotalWork
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handlePlayers(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, s.srv.ListSessions())
}

type kickRequest struct {
	ID     int32  `json:"id"`
	Reason string `json:"reason"`
}

func (s *Server) handleKick(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req kickRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.ID == 0 {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	if req.Reason == "" {
		req.Reason = "kicked by admin"
	}
	ok := s.srv.KickByID(req.ID, req.Reason)
	writeJSON(w, http.StatusOK, map[string]any{"ok": ok})
}

type summonRequest struct {
	TypeID int16   `json:"type_id"`
	X      float32 `json:"x"`
	Y      float32 `json:"y"`
	Team   byte    `json:"team"`
}

type unitMoveRequest struct {
	ID     int32   `json:"id"`
	VX     float32 `json:"vx"`
	VY     float32 `json:"vy"`
	RotVel float32 `json:"rot_vel"`
}

type unitTeleportRequest struct {
	ID       int32   `json:"id"`
	X        float32 `json:"x"`
	Y        float32 `json:"y"`
	Rotation float32 `json:"rotation"`
}

type unitLifeRequest struct {
	ID      int32   `json:"id"`
	LifeSec float32 `json:"life_sec"`
}

type unitFollowRequest struct {
	ID       int32   `json:"id"`
	TargetID int32   `json:"target_id"`
	Speed    float32 `json:"speed"`
}

type unitPatrolRequest struct {
	ID    int32   `json:"id"`
	X1    float32 `json:"x1"`
	Y1    float32 `json:"y1"`
	X2    float32 `json:"x2"`
	Y2    float32 `json:"y2"`
	Speed float32 `json:"speed"`
}

type unitBehaviorRequest struct {
	ID   int32  `json:"id"`
	Mode string `json:"mode"`
}

func (s *Server) handleSummon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.summonFn == nil {
		http.Error(w, "summon not available", http.StatusNotImplemented)
		return
	}
	var req summonRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Team == 0 {
		req.Team = 1
	}
	if err := s.summonFn(req.TypeID, req.X, req.Y, req.Team); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleUnitMove(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.moveFn == nil {
		http.Error(w, "unit move not available", http.StatusNotImplemented)
		return
	}
	var req unitMoveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.ID <= 0 {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	if err := s.moveFn(req.ID, req.VX, req.VY, req.RotVel); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleUnitTeleport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.teleportFn == nil {
		http.Error(w, "unit teleport not available", http.StatusNotImplemented)
		return
	}
	var req unitTeleportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.ID <= 0 {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	if err := s.teleportFn(req.ID, req.X, req.Y, req.Rotation); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleUnitLife(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.lifeFn == nil {
		http.Error(w, "unit life not available", http.StatusNotImplemented)
		return
	}
	var req unitLifeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.ID <= 0 {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	if err := s.lifeFn(req.ID, req.LifeSec); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleUnitFollow(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.followFn == nil {
		http.Error(w, "unit follow not available", http.StatusNotImplemented)
		return
	}
	var req unitFollowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.ID <= 0 || req.TargetID <= 0 {
		http.Error(w, "id and target_id required", http.StatusBadRequest)
		return
	}
	if err := s.followFn(req.ID, req.TargetID, req.Speed); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleUnitPatrol(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.patrolFn == nil {
		http.Error(w, "unit patrol not available", http.StatusNotImplemented)
		return
	}
	var req unitPatrolRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.ID <= 0 {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	if err := s.patrolFn(req.ID, req.X1, req.Y1, req.X2, req.Y2, req.Speed); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleUnitBehavior(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.behaviorFn == nil {
		http.Error(w, "unit behavior not available", http.StatusNotImplemented)
		return
	}
	var req unitBehaviorRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.ID <= 0 {
		http.Error(w, "id required", http.StatusBadRequest)
		return
	}
	mode := strings.ToLower(strings.TrimSpace(req.Mode))
	if mode == "" {
		mode = "clear"
	}
	if err := s.behaviorFn(req.ID, mode); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.stopFn == nil {
		http.Error(w, "stop not available", http.StatusNotImplemented)
		return
	}
	go s.stopFn()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type banRequest struct {
	Type   string `json:"type"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

func (s *Server) handleBan(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req banRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if req.Reason == "" {
		req.Reason = "banned by admin"
	}
	kind := strings.ToLower(strings.TrimSpace(req.Type))
	value := strings.TrimSpace(req.Value)
	switch kind {
	case "uuid":
		count := s.srv.BanUUID(value, req.Reason)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": count})
	case "ip":
		count := s.srv.BanIP(value, req.Reason)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": count})
	case "id":
		id, err := strconv.ParseInt(value, 10, 32)
		if err != nil {
			http.Error(w, "invalid id", http.StatusBadRequest)
			return
		}
		var hit *netserver.SessionInfo
		for _, s := range s.srv.ListSessions() {
			if s.ID == int32(id) {
				cp := s
				hit = &cp
				break
			}
		}
		if hit == nil {
			writeJSON(w, http.StatusOK, map[string]any{"ok": false})
			return
		}
		count := 0
		if hit.UUID != "" {
			count = s.srv.BanUUID(hit.UUID, req.Reason)
		}
		if hit.IP != "" {
			_ = s.srv.BanIP(hit.IP, req.Reason)
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": count})
	default:
		http.Error(w, "type must be uuid|ip|id", http.StatusBadRequest)
	}
}

type unbanRequest struct {
	Type  string `json:"type"`
	Value string `json:"value"`
}

func (s *Server) handleUnban(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var req unbanRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	kind := strings.ToLower(strings.TrimSpace(req.Type))
	value := strings.TrimSpace(req.Value)
	switch kind {
	case "uuid":
		ok := s.srv.UnbanUUID(value)
		writeJSON(w, http.StatusOK, map[string]any{"ok": ok})
	case "ip":
		ok := s.srv.UnbanIP(value)
		writeJSON(w, http.StatusOK, map[string]any{"ok": ok})
	default:
		http.Error(w, "type must be uuid|ip", http.StatusBadRequest)
	}
}

func (s *Server) handleBans(w http.ResponseWriter, _ *http.Request) {
	uuids, ips := s.srv.BanLists()
	writeJSON(w, http.StatusOK, map[string]any{
		"uuids": uuids,
		"ips":   ips,
	})
}

type opsRequest struct {
	Action string `json:"action"`
	UUID   string `json:"uuid"`
}

func (s *Server) handleOps(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"ops": s.srv.ListOps()})
	case http.MethodPost:
		var req opsRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid json", http.StatusBadRequest)
			return
		}
		action := strings.ToLower(strings.TrimSpace(req.Action))
		uuid := strings.TrimSpace(req.UUID)
		if uuid == "" {
			http.Error(w, "uuid required", http.StatusBadRequest)
			return
		}
		switch action {
		case "add":
			s.srv.AddOp(uuid)
		case "remove", "del", "delete":
			s.srv.RemoveOp(uuid)
		default:
			http.Error(w, "action must be add|remove", http.StatusBadRequest)
			return
		}
		if s.opsChangedFn != nil {
			s.opsChangedFn()
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "ops": s.srv.ListOps()})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) ListAPIKeys() []string {
	s.keyMu.RLock()
	defer s.keyMu.RUnlock()
	out := make([]string, 0, len(s.keys))
	for k := range s.keys {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func (s *Server) AddAPIKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	if _, ok := s.keys[key]; ok {
		return false
	}
	s.keys[key] = struct{}{}
	return true
}

func (s *Server) DeleteAPIKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	s.keyMu.Lock()
	defer s.keyMu.Unlock()
	if _, ok := s.keys[key]; !ok {
		return false
	}
	delete(s.keys, key)
	return true
}

func (s *Server) SetStopFunc(fn func()) {
	s.stopFn = fn
}

func (s *Server) SetSummonFunc(fn func(int16, float32, float32, byte) error) {
	s.summonFn = fn
}

func (s *Server) SetUnitMoveFunc(fn func(int32, float32, float32, float32) error) {
	s.moveFn = fn
}

func (s *Server) SetUnitTeleportFunc(fn func(int32, float32, float32, float32) error) {
	s.teleportFn = fn
}

func (s *Server) SetUnitLifeFunc(fn func(int32, float32) error) {
	s.lifeFn = fn
}

func (s *Server) SetUnitFollowFunc(fn func(int32, int32, float32) error) {
	s.followFn = fn
}

func (s *Server) SetUnitPatrolFunc(fn func(int32, float32, float32, float32, float32, float32) error) {
	s.patrolFn = fn
}

func (s *Server) SetUnitBehaviorFunc(fn func(int32, string) error) {
	s.behaviorFn = fn
}

func (s *Server) SetOpsChangedFunc(fn func()) {
	s.opsChangedFn = fn
}

func (s *Server) HasAPIKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}
	s.keyMu.RLock()
	_, ok := s.keys[key]
	s.keyMu.RUnlock()
	return ok
}

func (s *Server) requiresAuth() bool {
	s.keyMu.RLock()
	n := len(s.keys)
	s.keyMu.RUnlock()
	return n > 0
}

func apiHelpCategories() map[string][]string {
	return map[string][]string{
		"basic": {
			"help / help <category> / help all",
			"maps / world / host random|<map>|<file>",
			"ip / selfcheck",
		},
		"runtime": {
			"status / status watch on|off",
			"players",
			"scheduler status|on|off",
		},
		"admin": {
			"kick <id> [reason]",
			"ban uuid|ip|id ...",
			"unban uuid|ip ...",
			"bans",
			"op / deop / ops",
			"summon",
			"despawn",
			"umove / uteleport / ulife",
			"ufollow / upatrol / ubehavior",
			"stop / exit / quit",
		},
		"plugins": {
			"mods / mod",
			"js <script.js> [args]",
			"node <script.js> [args]",
			"go <target|.> [args]",
		},
		"storage": {
			"data status",
			"data db on|off",
			"data mode file|postgres|mysql|redis",
			"data dir <path>",
		},
		"api": {
			"api status",
			"api keys",
			"api keygen",
			"api keydel <key>",
		},
		"chat": {
			"/help",
			"/status",
			"/summon <typeId> <x> <y> [team] (OP)",
			"/despawn <entityId> (OP)",
			"/umove <id> <vx> <vy> [rotVel] (OP)",
			"/uteleport <id> <x> <y> [rot] (OP)",
			"/ulife <id> <seconds> (OP)",
			"/ufollow <id> <targetId> [speed] (OP)",
			"/upatrol <id> <x1> <y1> <x2> <y2> [speed] (OP)",
			"/ubehavior clear <id> (OP)",
			"/stop (OP)",
		},
		"unitapi": {
			"POST /unit/move {id,vx,vy,rot_vel}",
			"POST /unit/teleport {id,x,y,rotation}",
			"POST /unit/life {id,life_sec}",
			"POST /unit/follow {id,target_id,speed}",
			"POST /unit/patrol {id,x1,y1,x2,y2,speed}",
			"POST /unit/behavior {id,mode=clear}",
		},
	}
}

func safeMapName(srv *netserver.Server) string {
	if srv == nil || srv.MapNameFn == nil {
		return ""
	}
	return srv.MapNameFn()
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
