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
	return filepath.Join(configDir, "release.ini")
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
	toI := func(v bool) string {
		if v {
			return "1"
		}
		return "0"
	}
	content := fmt.Sprintf(`; mdt-server 启动释放控制
;
; 这个文件只控制“是否释放内置资源到磁盘”。
; 目录/文件的正常生成位置为 EXE 根目录（assets/data/mods/logs），configs 目录存放 INI 与 JSON 配置文件。
;
; 重要概念
; - “释放(release)”：把 EXE 内置打包的资源释放到磁盘（assets/worlds 与 configs 下的配置文件）。
;
; 二次启动不再释放的逻辑
; - 当程序完成一次释放后，会自动把 [release].released 写为 1，表示“已释放过”。
; - 想强制再次释放：把 released 改为 0，然后重启。
;
; 注意
; - configs/ 目录永远会保留/创建，但 configs 目录里只放配置文件（INI/JSON），不要放 assets/data/mods/logs。
; - released=0 会执行释放，释放阶段会覆盖写入目标文件；如果你不希望被覆盖，请保持 released=1。

[release]
; released:
;   0 = 下次启动会执行释放
;   1 = 标记为已释放（正常情况下程序释放完会自动写 1，二次启动不会释放）
released = %s
`, toI(p.Released))
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
			// Do not auto-generate release.ini on boot. It should be shipped with the server.
			return p, nil
		}
		return p, err
	}
	if st.IsDir() {
		return p, fmt.Errorf("release.ini is a directory: %s", path)
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
		if ext != ".ini" && !(isJSONConfig && ext == ".json") {
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
