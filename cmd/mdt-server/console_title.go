package main

import (
	"strings"

	"mdt-server/internal/config"
)

func applyProcessConsoleTitle(cfg config.Config, role, serverName string) {
	title := resolveProcessConsoleTitle(cfg.Personalization, role, serverName)
	if strings.TrimSpace(title) == "" {
		return
	}
	setProcessConsoleTitle(title)
}

func resolveProcessConsoleTitle(p config.PersonalizationConfig, role, serverName string) string {
	role = strings.ToLower(strings.TrimSpace(role))
	template := ""
	switch role {
	case "core2":
		template = p.Core2ConsoleTitle
	case "core3":
		template = p.Core3ConsoleTitle
	case "core4":
		template = p.Core4ConsoleTitle
	default:
		template = p.MainConsoleTitle
	}
	if strings.TrimSpace(template) == "" {
		template = defaultProcessConsoleTitle(role)
	}
	return expandProcessConsoleTitle(template, role, serverName)
}

func defaultProcessConsoleTitle(role string) string {
	switch role {
	case "core2":
		return "mdt-server | Core2 | IO"
	case "core3":
		return "mdt-server | Core3 | Snapshot"
	case "core4":
		return "mdt-server | Core4 | Policy"
	default:
		return "mdt-server | 主进程 | {server_name}"
	}
}

func expandProcessConsoleTitle(template, role, serverName string) string {
	title := strings.TrimSpace(template)
	if title == "" {
		return ""
	}
	if strings.TrimSpace(serverName) == "" {
		serverName = "mdt-server"
	}
	processLabel := "main"
	switch role {
	case "core2", "core3", "core4":
		processLabel = role
	}
	replacer := strings.NewReplacer(
		"{server_name}", serverName,
		"{role}", role,
		"{process}", processLabel,
	)
	return replacer.Replace(title)
}
