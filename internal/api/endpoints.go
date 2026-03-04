package api

import (
	"encoding/json"
	"net/http"
	"sort"
	"strings"
	"sync"

	"mdt-server/internal/config"
)

// ====================
// API Server - API服务器
// ====================

// APIServer API服务器
type APIServer struct {
	mu         sync.RWMutex
	Config     config.Config
	endpoints  map[string]EndpointHandler
	keys       map[string]struct{}
	stopFn     func()
	// typeID, x, y, team
	summonFn      func(int16, float32, float32, byte) error
	moveFn        func(int32, float32, float32, float32) error
	teleportFn    func(int32, float32, float32, float32) error
	lifeFn        func(int32, float32) error
	followFn      func(int32, int32, float32) error
	patrolFn      func(int32, float32, float32, float32, float32, float32) error
	behaviorFn    func(int32, string) error
	opsChangedFn  func()
}

// EndpointHandler 端点处理器接口
type EndpointHandler interface {
	Handle(w http.ResponseWriter, r *http.Request) error
}

// APIEndpoint API端点
type APIEndpoint struct {
	Method  string
	Path    string
	Handler EndpointHandler
}

// APIResponse API响应
type APIResponse struct {
	Success bool        `json:"success"`
	Data    interface{} `json:"data,omitempty"`
	Error   string      `json:"error,omitempty"`
}

// ListResponse 列表响应
type ListResponse struct {
	Items  []interface{} `json:"items"`
	Page   int           `json:"page"`
	PageSize int         `json:"page_size"`
	Total  int           `json:"total"`
}

// Pages 分页信息
type Pages struct {
	Page     int `json:"page"`
	PageSize int `json:"page_size"`
}

// ConfigResponse 配置响应
type ConfigResponse struct {
	API      config.APIConfig     `json:"api"`
	Runtime  config.RuntimeConfig `json:"runtime"`
	Storage  string               `json:"storage,omitempty"`
	Net      string               `json:"net,omitempty"`
}

// NewAPIServer 创建新的API服务器
// cfg: API配置
func NewAPIServer(cfg config.Config) *APIServer {
	return &APIServer{
		Config:    cfg,
		endpoints: make(map[string]EndpointHandler),
		keys:      make(map[string]struct{}),
	}
}

// RegisterEndpoint 注册端点
// path: 路径
// handler: 处理器
func (s *APIServer) RegisterEndpoint(path string, handler EndpointHandler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.endpoints[path] = handler
}

// unregisterEndpoint 注销端点
// path: 路径
func (s *APIServer) unregisterEndpoint(path string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.endpoints, path)
}

// AddAPIKey 添加API密钥
// key: 密钥
// 返回: 是否成功
func (s *APIServer) AddAPIKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.keys[key]; ok {
		return false
	}
	s.keys[key] = struct{}{}
	return true
}

// DeleteAPIKey 删除API密钥
// key: 密钥
// 返回: 是否成功
func (s *APIServer) DeleteAPIKey(key string) bool {
	key = strings.TrimSpace(key)
	if key == "" {
		return false
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.keys[key]; !ok {
		return false
	}
	delete(s.keys, key)
	return true
}

// ListAPIKeys 获取所有API密钥
// 返回: 密钥列表
func (s *APIServer) ListAPIKeys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	keys := make([]string, 0, len(s.keys))
	for k := range s.keys {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// GetRegisteredEndpoints 获取所有已注册的端点
// 返回: 端点列表
func (s *APIServer) GetRegisteredEndpoints() []APIEndpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	endpoints := make([]APIEndpoint, 0, len(s.endpoints))
	for path, handler := range s.endpoints {
		endpoints = append(endpoints, APIEndpoint{
			Method:  "GET",
			Path:    path,
			Handler: handler,
		})
	}

	// 按路径排序
	sort.Slice(endpoints, func(i, j int) bool {
		return endpoints[i].Path < endpoints[j].Path
	})

	return endpoints
}

// RequiresAuth 检查是否需要认证
// 返回: 是否需要
func (s *APIServer) RequiresAuth() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.keys) > 0
}

//-handler 注册所有内置端点
// server: API服务器
func RegisterAllEndpoints(server *APIServer) {
	if server == nil {
		return
	}

	// 内置端点
	server.RegisterEndpoint("/api/version", &genericHandler{handler: func(w http.ResponseWriter, r *http.Request) error {
		return writeJSON(w, http.StatusOK, map[string]string{"version": "1.0.0"})
	}})
	server.RegisterEndpoint("/api/health", &genericHandler{handler: func(w http.ResponseWriter, r *http.Request) error {
		return writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}})
	server.RegisterEndpoint("/api/status", &genericHandler{handler: func(w http.ResponseWriter, r *http.Request) error {
		return writeJSON(w, http.StatusOK, map[string]string{"status": "running"})
	}})
	server.RegisterEndpoint("/api/config", &genericHandler{handler: func(w http.ResponseWriter, r *http.Request) error {
		_ = writeJSON(w, http.StatusOK, map[string]string{"config": "mdt-server"})
		return nil
	}})
	server.RegisterEndpoint("/api/shutdown", &genericHandler{handler: func(w http.ResponseWriter, r *http.Request) error {
		_ = writeJSON(w, http.StatusOK, map[string]string{"shutdown": "requested"})
		return nil
	}})
}

// genericHandler 通用处理器
type genericHandler struct {
	handler func(http.ResponseWriter, *http.Request) error
}

func (h *genericHandler) Handle(w http.ResponseWriter, r *http.Request) error {
	return h.handler(w, r)
}

// writeJSON 写入JSON响应
// w: 响应writer
// code: HTTP状态码
// v: 数据
func writeJSON(w http.ResponseWriter, code int, v interface{}) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// NewServerReference 创建服务器引用
// srv: 服务器对象
// wld: 世界对象
// cfg: 配置
func NewServerReference(srv, wld interface{}, cfg config.Config) *ServerReference {
	return &ServerReference{
		Srv:    srv,
		Wld:    wld,
		Config: cfg,
	}
}

// ServerReference 服务器引用
type ServerReference struct {
	Srv    interface{} // *net.Server
	Wld    interface{} // *world.World
	Config config.Config
}

// APINodeStats API节点统计
type APINodeStats struct {
	TotalRequests int64            `json:"total_requests"`
	ByEndpoint    map[string]int64 `json:"by_endpoint"`
	Errors        int64            `json:"errors"`
	Timestamp     string           `json:"timestamp"`
}

// GetStats 获取统计信息
// 返回: 统计信息
func (s *APIServer) GetStats() APINodeStats {
	s.mu.RLock()
	defer s.mu.RUnlock()

	return APINodeStats{
		TotalRequests: 0, // TODO: 统计请求
		ByEndpoint:    make(map[string]int64),
		Errors:        0, // TODO: 统计错误
		Timestamp:     "2026-03-03T00:00:00Z",
	}
}
