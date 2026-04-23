package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseEmbeddedConfigsDoesNotOverwriteReleasePolicyFile(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "configs")
	if err := os.MkdirAll(configDir, 0o755); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	releasePath := filepath.Join(configDir, "release.toml")
	original := "# user-owned release policy\n\n[release]\nreleased = true\n"
	if err := os.WriteFile(releasePath, []byte(original), 0o644); err != nil {
		t.Fatalf("write release policy: %v", err)
	}

	if err := releaseEmbeddedConfigs(configDir); err != nil {
		t.Fatalf("releaseEmbeddedConfigs: %v", err)
	}

	raw, err := os.ReadFile(releasePath)
	if err != nil {
		t.Fatalf("read release policy: %v", err)
	}
	if string(raw) != original {
		t.Fatalf("expected release policy file to remain untouched, got:\n%s", string(raw))
	}
	if strings.Contains(string(raw), "首次释放会把内置地图与 configs 下的配置文件写到工作区") {
		t.Fatalf("expected bundled release template not to overwrite user policy file")
	}
}

func TestWriteBundledRuntimeFileIfMissingResolvesAbsoluteTargetUnderRoot(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "data", "vanilla", "profiles.json")

	if err := writeBundledRuntimeFileIfMissing(root, target); err != nil {
		t.Fatalf("write bundled runtime file: %v", err)
	}

	st, err := os.Stat(target)
	if err != nil {
		t.Fatalf("expected bundled vanilla profiles to be released: %v", err)
	}
	if st.Size() == 0 {
		t.Fatal("expected released vanilla profiles to be non-empty")
	}
}
