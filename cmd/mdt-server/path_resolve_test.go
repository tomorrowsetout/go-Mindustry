package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolvePathFromBasesPrefersFirstExistingBase(t *testing.T) {
	rootA := filepath.Join(t.TempDir(), "rootA")
	rootB := filepath.Join(t.TempDir(), "rootB")
	if err := os.MkdirAll(filepath.Join(rootA, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir rootA configs: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(rootB, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir rootB configs: %v", err)
	}

	pathA := filepath.Join(rootA, "configs", "config.toml")
	pathB := filepath.Join(rootB, "configs", "config.toml")
	if err := os.WriteFile(pathA, []byte("a"), 0o644); err != nil {
		t.Fatalf("write pathA: %v", err)
	}
	if err := os.WriteFile(pathB, []byte("b"), 0o644); err != nil {
		t.Fatalf("write pathB: %v", err)
	}

	got := resolvePathFromBases("configs/config.toml", []string{rootA, rootB})
	want, _ := filepath.Abs(pathA)
	if got != want {
		t.Fatalf("expected first existing base %q, got %q", want, got)
	}
}

func TestResolvePathFromBasesFallsBackToFirstBaseWhenMissing(t *testing.T) {
	rootA := filepath.Join(t.TempDir(), "exe-root")
	rootB := filepath.Join(t.TempDir(), "cwd-root")

	got := resolvePathFromBases("configs/config.toml", []string{rootA, rootB})
	want, _ := filepath.Abs(filepath.Join(rootA, "configs", "config.toml"))
	if got != want {
		t.Fatalf("expected missing config to fall back to first preferred base %q, got %q", want, got)
	}
}

func TestResolvePathFromBasesKeepsBinWorkspaceEvenWhenParentHasConfig(t *testing.T) {
	root := t.TempDir()
	binDir := filepath.Join(root, "bin")
	if err := os.MkdirAll(filepath.Join(root, "configs"), 0o755); err != nil {
		t.Fatalf("mkdir parent configs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "configs", "config.toml"), []byte("parent"), 0o644); err != nil {
		t.Fatalf("write parent config: %v", err)
	}

	got := resolvePathFromBases("configs/config.toml", []string{binDir})
	want, _ := filepath.Abs(filepath.Join(binDir, "configs", "config.toml"))
	if got != want {
		t.Fatalf("expected bin-local config path %q, got %q", want, got)
	}
}

func TestNormalizeRelativePathUsesPlatformSeparators(t *testing.T) {
	got := normalizeRelativePath("configs/config.toml")
	want := filepath.Clean(filepath.FromSlash("configs/config.toml"))
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
