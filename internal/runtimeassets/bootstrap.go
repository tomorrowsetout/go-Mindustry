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
		clean := filepath.Clean(path)
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
		if filepath.IsAbs(subpath) {
			add(subpath)
			return
		}
		for _, root := range roots {
			add(filepath.Join(root, subpath))
		}
	}

	if runtimeAssetsDir = strings.TrimSpace(runtimeAssetsDir); runtimeAssetsDir != "" {
		addSubpath(filepath.Join(runtimeAssetsDir, bootstrapWorldFile))
	}
	addSubpath(filepath.Join("assets", bootstrapWorldFile))
	addSubpath(filepath.Join("bin", "assets", bootstrapWorldFile))
	addSubpath(filepath.Join("go-server", "assets", bootstrapWorldFile))
	addSubpath(bootstrapWorldFile)
	return out
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
