package net

import (
	"sync"
)

// AdminManager 管理员管理器
type AdminManager struct {
	mu         sync.RWMutex
	adminUUIDs map[string]struct{}
	adminIPs   map[string]string
	ops        map[string]struct{} // 管理员UUID集合
	whitelist  map[string]struct{}
	banUUIDs   map[string]string
	banIPs     map[string]string
}

// NewAdminManager 创建管理员管理器
func NewAdminManager() *AdminManager {
	return &AdminManager{
		adminUUIDs: make(map[string]struct{}),
		adminIPs:   make(map[string]string),
		ops:        make(map[string]struct{}),
		whitelist:  make(map[string]struct{}),
		banUUIDs:   make(map[string]string),
		banIPs:     make(map[string]string),
	}
}

// IsAdmin 检查是否是管理员
func (a *AdminManager) IsAdmin(uuid, usid string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if _, ok := a.adminUUIDs[uuid]; ok {
		return true
	}

	if _, ok := a.ops[uuid]; ok {
		return true
	}

	if _, ok := a.adminIPs[usid]; ok {
		return true
	}

	return false
}

// IsWhitelisted 检查是否在白名单中
func (a *AdminManager) IsWhitelisted(uuid, usid string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if len(a.whitelist) == 0 {
		return true // 没有白名单时允许所有人
	}

	if _, ok := a.whitelist[uuid]; ok {
		return true
	}

	return false
}

// IsBannedUUID 检查UUID是否被封禁
func (a *AdminManager) IsBannedUUID(uuid string) (bool, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	reason, ok := a.banUUIDs[uuid]
	return ok, reason
}

// IsBannedIP 检查IP是否被封禁
func (a *AdminManager) IsBannedIP(ip string) (bool, string) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	reason, ok := a.banIPs[ip]
	return ok, reason
}

// IsSubnetBanned 检查子网是否被封禁
func (a *AdminManager) IsSubnetBanned(ip string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// 简单的子网检查（检查IP的前三段）
	for bannedIP := range a.banIPs {
		if isSubnetMatch(ip, bannedIP) {
			return true
		}
	}

	return false
}

// isSubnetMatch 检查两个IP是否在同一子网
func isSubnetMatch(ip1, ip2 string) bool {
	// 简化的子网匹配：匹配IP的前三段
	// IP格式：xxx.xxx.xxx.xxx
	parts1 := splitIP(ip1)
	parts2 := splitIP(ip2)

	if len(parts1) < 3 || len(parts2) < 3 {
		return false
	}

	for i := 0; i < 3; i++ {
		if parts1[i] != parts2[i] {
			return false
		}
	}

	return true
}

// splitIP 分割IP地址
func splitIP(ip string) []string {
	var parts []string
	current := ""
	for _, ch := range ip {
		if ch == '.' {
			parts = append(parts, current)
			current = ""
		} else {
			current += string(ch)
		}
	}
	if current != "" {
		parts = append(parts, current)
	}
	return parts
}

// AddAdmin 添加管理员
func (a *AdminManager) AddAdmin(uuid, usid string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	if uuid != "" {
		a.adminUUIDs[uuid] = struct{}{}
	}
	if usid != "" {
		a.adminIPs[usid] = uuid
	}
}

// RemoveAdmin 移除管理员
func (a *AdminManager) RemoveAdmin(uuid, usid string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.adminUUIDs, uuid)
	if usid != "" {
		delete(a.adminIPs, usid)
	}
}

// AddOp 添加管理员
func (a *AdminManager) AddOp(uuid string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.ops[uuid] = struct{}{}
}

// RemoveOp 移除管理员
func (a *AdminManager) RemoveOp(uuid string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.ops, uuid)
}

// IsOp 检查是否是管理员
func (a *AdminManager) IsOp(uuid string) bool {
	a.mu.RLock()
	defer a.mu.RUnlock()

	_, ok := a.ops[uuid]
	return ok
}

// AddToWhitelist 添加到白名单
func (a *AdminManager) AddToWhitelist(uuid string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.whitelist[uuid] = struct{}{}
}

// RemoveFromWhitelist 从白名单移除
func (a *AdminManager) RemoveFromWhitelist(uuid string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	delete(a.whitelist, uuid)
}

// BanUUID 封禁UUID
func (a *AdminManager) BanUUID(uuid, reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.banUUIDs[uuid] = reason
}

// BanIP 封禁IP
func (a *AdminManager) BanIP(ip, reason string) {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.banIPs[ip] = reason
}

// UnbanUUID 解封UUID
func (a *AdminManager) UnbanUUID(uuid string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.banUUIDs[uuid]; ok {
		delete(a.banUUIDs, uuid)
		return true
	}

	return false
}

// UnbanIP 解封IP
func (a *AdminManager) UnbanIP(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()

	if _, ok := a.banIPs[ip]; ok {
		delete(a.banIPs, ip)
		return true
	}

	return false
}

// ListBans 列出所有封禁
func (a *AdminManager) ListBans() (uuids, ips map[string]string) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	uuids = make(map[string]string, len(a.banUUIDs))
	ips = make(map[string]string, len(a.banIPs))

	for k, v := range a.banUUIDs {
		uuids[k] = v
	}

	for k, v := range a.banIPs {
		ips[k] = v
	}

	return uuids, ips
}

// ClearBans 清除所有封禁
func (a *AdminManager) ClearBans() {
	a.mu.Lock()
	defer a.mu.Unlock()

	a.banUUIDs = make(map[string]string)
	a.banIPs = make(map[string]string)
}

// ListOps 列出所有管理员
func (a *AdminManager) ListOps() []string {
	a.mu.RLock()
	defer a.mu.RUnlock()

	ops := make([]string, 0, len(a.ops))
	for uuid := range a.ops {
		ops = append(ops, uuid)
	}

	return ops
}

// GetPlayerCount 获取在线玩家数量
func (a *AdminManager) GetPlayerCount() int {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return len(a.ops) + len(a.adminUUIDs)
}
