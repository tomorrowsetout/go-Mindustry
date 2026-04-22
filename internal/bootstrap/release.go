package bootstrap

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	mdtserver "mdt-server"
)

type releasePolicy struct {
	Released bool
}

func defaultReleasePolicy() releasePolicy {
	return releasePolicy{
		Released: false,
	}
}

func releaseINIPath(configDir string) string {
	return filepath.Join(configDir, "release.toml")
}

func parseReleaseINI(path string, p releasePolicy) (releasePolicy, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return p, err
	}
	lines := strings.Split(strings.ReplaceAll(string(b), "\r\n", "\n"), "\n")
	sec := ""
	for _, ln := range lines {
		s := strings.TrimSpace(ln)
		if s == "" || strings.HasPrefix(s, ";") || strings.HasPrefix(s, "#") {
			continue
		}
		if strings.HasPrefix(s, "[") && strings.Contains(s, "]") {
			sec = strings.ToLower(strings.TrimSpace(s[1:strings.Index(s, "]")]))
			continue
		}
		if i := strings.IndexAny(s, ";#"); i >= 0 {
			s = strings.TrimSpace(s[:i])
			if s == "" {
				continue
			}
		}
		eq := strings.Index(s, "=")
		if eq <= 0 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(s[:eq]))
		val := strings.TrimSpace(s[eq+1:])
		bval := func(def bool) bool {
			switch strings.ToLower(val) {
			case "1", "true", "yes", "on":
				return true
			case "0", "false", "no", "off":
				return false
			default:
				return def
			}
		}
		switch sec + "." + key {
		case "release.released":
			p.Released = bval(p.Released)
		}
	}
	return p, nil
}

func writeReleaseINI(path string, p releasePolicy) error {
	content := fmt.Sprintf(`# mdt-server 资源释放控制
#
# 本文件只控制“是否把程序内置资源释放到磁盘”。
# 首次释放会把内置地图与 configs 下的配置文件写到工作区。
#
# released:
#   false = 下次启动执行释放
#   true  = 标记为已释放，后续启动默认不再重复覆盖
#
# 如果你想强制重新释放内置资源，把它改回 false 后重启即可。

[release]
released = %t
`, p.Released)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

func loadReleasePolicy(configDir string) (releasePolicy, error) {
	p := defaultReleasePolicy()
	path := releaseINIPath(configDir)
	st, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			// 不在启动时自动生成 release.toml；应由程序包直接提供。
			return p, nil
		}
		return p, err
	}
	if st.IsDir() {
		return p, fmt.Errorf("release.toml is a directory: %s", path)
	}
	return parseReleaseINI(path, p)
}

func shouldReleaseEmbedded(p releasePolicy) bool {
	return !p.Released
}

func markEmbeddedReleasedAt(configDir string, p releasePolicy) error {
	p.Released = true
	return writeReleaseINI(releaseINIPath(configDir), p)
}

func releaseEmbeddedWorlds(worldsDir string) error {
	return fs.WalkDir(mdtserver.BundledFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		isWorld := path == "assets/worlds" || strings.HasPrefix(path, "assets/worlds/")
		if !isWorld {
			return nil
		}

		if d.IsDir() {
			rel, _ := filepath.Rel("assets/worlds", path)
			return os.MkdirAll(filepath.Join(worldsDir, rel), 0o755)
		}

		data, err := fs.ReadFile(mdtserver.BundledFiles, path)
		if err != nil {
			return err
		}
		var target string
		rel, _ := filepath.Rel("assets/worlds", path)
		target = filepath.Join(worldsDir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func releaseEmbeddedConfigs(configDir string) error {
	return fs.WalkDir(mdtserver.BundledFiles, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == "." {
			return nil
		}
		isCfg := path == "configs" || strings.HasPrefix(path, "configs/")
		if !isCfg {
			return nil
		}
		if d.IsDir() {
			rel, _ := filepath.Rel("configs", path)
			return os.MkdirAll(filepath.Join(configDir, rel), 0o755)
		}
		ext := strings.ToLower(filepath.Ext(path))
		isJSONConfig := strings.HasPrefix(filepath.ToSlash(path), "configs/json/")
		isConfigDoc := strings.HasPrefix(filepath.ToSlash(path), "configs/")
		if ext != ".toml" && !(isJSONConfig && ext == ".json") && !(isConfigDoc && ext == ".md") {
			return nil
		}
		data, err := fs.ReadFile(mdtserver.BundledFiles, path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel("configs", path)
		target := filepath.Join(configDir, rel)
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func uniqDirs(dirs []string) []string {
	uniq := map[string]struct{}{}
	out := make([]string, 0, len(dirs))
	for _, d := range dirs {
		d = strings.TrimSpace(d)
		if d == "" {
			continue
		}
		if _, ok := uniq[d]; ok {
			continue
		}
		uniq[d] = struct{}{}
		out = append(out, d)
	}
	return out
}
