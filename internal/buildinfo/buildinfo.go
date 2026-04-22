package buildinfo

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
)

// These may be set via -ldflags at build time.
var (
	Version = "dev"
	Commit  = "none"

	ProductName = "mdt-server"
	DisplayName = "mdt-server"
	GameVersion = "Mindustry 157"
	QQGroup     = ""
	FooterText  = ""
	IconPNG     = "FBF.png"
	IconICO     = "FBF.ico"
)

var loadOnce sync.Once

func init() {
	loadCenterMetadata()
}

func CenterDir() string {
	loadCenterMetadata()
	for _, dir := range centerDirCandidates() {
		if st, err := os.Stat(dir); err == nil && st.IsDir() {
			return dir
		}
	}
	return ""
}

func CenterFile(name string) string {
	dir := CenterDir()
	if dir == "" || strings.TrimSpace(name) == "" {
		return ""
	}
	return filepath.Join(dir, name)
}

func loadCenterMetadata() {
	loadOnce.Do(func() {
		versionMap := firstCenterFileMap("version.toml")
		businessMap := firstCenterFileMap("Business.toml")

		if Version == "dev" {
			if v := firstNonEmpty(versionMap["version"], versionMap["server_version"], versionMap["product_version"]); v != "" {
				Version = v
			}
		}
		if Commit == "none" {
			if v := firstNonEmpty(versionMap["commit"], versionMap["build"], versionMap["channel"]); v != "" {
				Commit = v
			}
		}
		if v := firstNonEmpty(businessMap["product_name"], businessMap["name"], businessMap["brand"]); v != "" {
			ProductName = v
		}
		if v := firstNonEmpty(businessMap["display_name"], businessMap["product_name"], businessMap["name"], businessMap["brand"]); v != "" {
			DisplayName = v
		}
		if v := firstNonEmpty(versionMap["game_version"], versionMap["mindustry_version"], businessMap["game_version"]); v != "" {
			GameVersion = v
		}
		if v := firstNonEmpty(businessMap["qq_group"], businessMap["group"], businessMap["qq"]); v != "" {
			QQGroup = v
		}
		if v := firstNonEmpty(businessMap["footer_text"], businessMap["signature"], businessMap["banner"]); v != "" {
			FooterText = v
		}
		if v := firstNonEmpty(businessMap["icon_png"], businessMap["icon_png_file"]); v != "" {
			IconPNG = v
		}
		if v := firstNonEmpty(businessMap["icon_ico"], businessMap["icon_ico_file"]); v != "" {
			IconICO = v
		}
	})
}

func centerDirCandidates() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 4)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		path = filepath.Clean(path)
		if _, ok := seen[path]; ok {
			return
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	if wd, err := os.Getwd(); err == nil {
		add(filepath.Join(wd, "Center"))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(filepath.Clean(exe))
		add(filepath.Join(exeDir, "Center"))
		add(filepath.Join(exeDir, "..", "Center"))
	}
	return out
}

func firstCenterFileMap(name string) map[string]string {
	for _, dir := range centerDirCandidates() {
		path := filepath.Join(dir, name)
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			if values, err := parseSimpleTOML(path); err == nil {
				return values
			}
		}
	}
	return nil
}

func parseSimpleTOML(path string) (map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	lines := strings.Split(strings.ReplaceAll(string(raw), "\r\n", "\n"), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.Contains(line, "]") {
			continue
		}
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if idx := strings.Index(line, ";"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		eq := strings.Index(line, "=")
		if eq <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(line[:eq]))
		value := strings.TrimSpace(line[eq+1:])
		value = strings.Trim(value, "\"'")
		if key != "" {
			out[key] = value
		}
	}
	return out, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
