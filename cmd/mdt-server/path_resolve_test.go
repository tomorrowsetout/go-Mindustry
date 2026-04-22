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

func TestNormalizeRelativePathUsesPlatformSeparators(t *testing.T) {
	got := normalizeRelativePath("configs/config.toml")
	want := filepath.Clean(filepath.FromSlash("configs/config.toml"))
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}
