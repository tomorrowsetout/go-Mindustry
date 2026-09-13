package net

import (
	"fmt"
	"strings"
)

// ModCompatibilityManager Mod兼容性管理器
type ModCompatibilityManager struct {
	expectedMods []string
}

// NewModCompatibilityManager 创建Mod兼容性管理器
func NewModCompatibilityManager(expectedMods []string) *ModCompatibilityManager {
	return &ModCompatibilityManager{
		expectedMods: expectedMods,
	}
}

// CheckCompatibility 检查Mod兼容性
func (m *ModCompatibilityManager) CheckCompatibility(clientMods []string) ([]string, []string, string, bool) {
	if len(m.expectedMods) == 0 {
		return nil, nil, "", true
	}

	missing := findMissingMods(m.expectedMods, clientMods)
	extra := findExtraMods(m.expectedMods, clientMods)

	if len(missing) == 0 && len(extra) == 0 {
		return nil, nil, "", true
	}

	// 构建错误消息
	var message strings.Builder
	message.WriteString("[accent]Incompatible mods![]\n\n")

	if len(missing) > 0 {
		message.WriteString("[scarlet]Missing:[lightgray]\n")
		for _, mod := range missing {
			message.WriteString(fmt.Sprintf("> %s\n", mod))
		}
		message.WriteString("[]\n")
	}

	if len(extra) > 0 {
		message.WriteString("[scarlet]Unnecessary mods:[lightgray]\n")
		for _, mod := range extra {
			message.WriteString(fmt.Sprintf("> %s\n", mod))
		}
	}

	return missing, extra, message.String(), false
}

// findMissingMods 查找缺失的Mod
func findMissingMods(expected, clientMods []string) []string {
	clientMap := make(map[string]struct{}, len(clientMods))
	for _, mod := range clientMods {
		clientMap[strings.ToLower(strings.TrimSpace(mod))] = struct{}{}
	}

	var missing []string
	for _, mod := range expected {
		lowerMod := strings.ToLower(strings.TrimSpace(mod))
		if _, exists := clientMap[lowerMod]; !exists {
			missing = append(missing, mod)
		}
	}

	return missing
}

// findExtraMods 查找多余的Mod
func findExtraMods(expected, clientMods []string) []string {
	expectedMap := make(map[string]struct{})
	for _, mod := range expected {
		expectedMap[strings.ToLower(strings.TrimSpace(mod))] = struct{}{}
	}

	var extra []string
	for _, mod := range clientMods {
		lowerMod := strings.ToLower(strings.TrimSpace(mod))
		if _, exists := expectedMap[lowerMod]; !exists {
			extra = append(extra, mod)
		}
	}

	return extra
}

// ValidateModList 验证Mod列表格式
func ValidateModList(mods []string) error {
	for i, mod := range mods {
		trimmed := strings.TrimSpace(mod)
		if trimmed == "" {
			return fmt.Errorf("mod at index %d is empty", i)
		}
		if len(trimmed) > 100 {
			return fmt.Errorf("mod at index %d is too long (max 100 characters)", i)
		}
		if strings.Contains(trimmed, "\n") || strings.Contains(trimmed, "\r") {
			return fmt.Errorf("mod at index %d contains invalid characters", i)
		}
	}

	return nil
}

// FormatModKickReason 格式化Mod踢人原因
func FormatModKickReason(missing, extra []string) string {
	if len(missing) == 0 && len(extra) == 0 {
		return ""
	}

	var reason strings.Builder
	if len(missing) > 0 {
		reason.WriteString("missing mods: ")
		reason.WriteString(strings.Join(missing, ", "))
	}

	if len(extra) > 0 {
		if len(missing) > 0 {
			reason.WriteString("; ")
		}
		reason.WriteString("extra mods: ")
		reason.WriteString(strings.Join(extra, ", "))
	}

	return reason.String()
}
