package runtimeassets

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const bootstrapWorldFile = "bootstrap-world.bin"

func bootstrapSearchRoots() []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 8)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		clean := filepath.Clean(path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}

	add(".")
	if wd, err := os.Getwd(); err == nil {
		add(wd)
		add(filepath.Dir(wd))
	}
	if exe, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exe)
		add(exeDir)
		add(filepath.Dir(exeDir))
	}
	return out
}

func bootstrapWorldCandidates(runtimeAssetsDir string, roots []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, 24)
	add := func(path string) {
		path = strings.TrimSpace(path)
		if path == "" {
			return
		}
		clean := cleanBootstrapPath(path)
		if _, ok := seen[clean]; ok {
			return
		}
		seen[clean] = struct{}{}
		out = append(out, clean)
	}
	addSubpath := func(subpath string) {
		subpath = strings.TrimSpace(subpath)
		if subpath == "" {
			return
		}
		if isBootstrapAbsPath(subpath) {
			add(subpath)
			return
		}
		for _, root := range roots {
			add(joinBootstrapPath(root, subpath))
		}
	}

	if runtimeAssetsDir = strings.TrimSpace(runtimeAssetsDir); runtimeAssetsDir != "" {
		addSubpath(joinBootstrapPath(runtimeAssetsDir, bootstrapWorldFile))
	}
	addSubpath(joinBootstrapPath("assets", bootstrapWorldFile))
	addSubpath(joinBootstrapPath("bin", "assets", bootstrapWorldFile))
	addSubpath(joinBootstrapPath("go-server", "assets", bootstrapWorldFile))
	addSubpath(bootstrapWorldFile)
	return out
}

func isBootstrapAbsPath(path string) bool {
	path = strings.TrimSpace(path)
	if filepath.IsAbs(path) {
		return true
	}
	return len(path) >= 3 && isASCIILetter(path[0]) && path[1] == ':' && (path[2] == '\\' || path[2] == '/')
}

func isASCIILetter(b byte) bool {
	return (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func cleanBootstrapPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if strings.Contains(path, `\`) || isBootstrapAbsPath(path) {
		return strings.ReplaceAll(path, "/", `\`)
	}
	return filepath.Clean(path)
}

func joinBootstrapPath(base string, elems ...string) string {
	base = strings.TrimSpace(base)
	if len(elems) == 0 {
		return cleanBootstrapPath(base)
	}
	if strings.Contains(base, `\`) || isBootstrapAbsPath(base) {
		parts := make([]string, 0, 1+len(elems))
		if base != "" {
			parts = append(parts, strings.TrimRight(strings.ReplaceAll(base, "/", `\`), `\`))
		}
		for _, elem := range elems {
			elem = strings.TrimSpace(elem)
			if elem == "" {
				continue
			}
			if isBootstrapAbsPath(elem) {
				return cleanBootstrapPath(elem)
			}
			elem = strings.Trim(strings.ReplaceAll(elem, "/", `\`), `\`)
			if elem != "" {
				parts = append(parts, elem)
			}
		}
		return cleanBootstrapPath(strings.Join(parts, `\`))
	}
	args := append([]string{base}, elems...)
	return filepath.Join(args...)
}

func BootstrapWorldCandidates(runtimeAssetsDir string) []string {
	return bootstrapWorldCandidates(runtimeAssetsDir, bootstrapSearchRoots())
}

func LoadBootstrapWorld(runtimeAssetsDir string) ([]byte, string, error) {
	candidates := BootstrapWorldCandidates(runtimeAssetsDir)
	for _, path := range candidates {
		data, err := os.ReadFile(path)
		if err == nil && len(data) > 0 {
			return data, path, nil
		}
	}
	return nil, "", fmt.Errorf("%s not found; candidates=%s", bootstrapWorldFile, strings.Join(candidates, ", "))
}
