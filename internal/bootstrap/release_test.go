package bootstrap

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"mdt-server/internal/config"
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

func TestEnsureStartupConfigTreeReleasesMainConfigWithoutOverwrite(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "configs", "config.toml")

	if err := EnsureStartupConfigTree(cfgPath); err != nil {
		t.Fatalf("EnsureStartupConfigTree: %v", err)
	}

	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read released config: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected released config.toml to be non-empty")
	}

	custom := []byte("# user config\n")
	if err := os.WriteFile(cfgPath, custom, 0o644); err != nil {
		t.Fatalf("overwrite config: %v", err)
	}
	if err := EnsureStartupConfigTree(cfgPath); err != nil {
		t.Fatalf("EnsureStartupConfigTree second pass: %v", err)
	}
	raw, err = os.ReadFile(cfgPath)
	if err != nil {
		t.Fatalf("read user config after second pass: %v", err)
	}
	if string(raw) != string(custom) {
		t.Fatalf("expected existing config not to be overwritten, got:\n%s", string(raw))
	}
}

func TestReleaseEmbeddedConfigsReleasesBundledJSONButSkipsOps(t *testing.T) {
	configDir := filepath.Join(t.TempDir(), "configs")

	if err := releaseEmbeddedConfigs(configDir); err != nil {
		t.Fatalf("releaseEmbeddedConfigs: %v", err)
	}

	if _, err := os.Stat(filepath.Join(configDir, "json", "block_names.json")); err != nil {
		t.Fatalf("expected bundled block_names.json to be released: %v", err)
	}
	if _, err := os.Stat(filepath.Join(configDir, "json", "ops.json")); !os.IsNotExist(err) {
		t.Fatalf("expected ops.json to remain runtime-generated, got err=%v", err)
	}
}

func TestEnsureWorkspaceCreatesStateFilesButNoSampleMods(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "configs", "config.toml")
	cfg := config.Default()
	config.ApplyBaseDir(&cfg, root)

	result, err := EnsureWorkspace(cfgPath, cfg)
	if err != nil {
		t.Fatalf("EnsureWorkspace: %v", err)
	}
	if len(result.CreatedFiles) == 0 {
		t.Fatal("expected bootstrap to create runtime state files")
	}

	for _, path := range []string{
		filepath.Join(root, "mods", "js", "hello.js"),
		filepath.Join(root, "mods", "node", "hello.js"),
		filepath.Join(root, "mods", "go", "hello.go"),
		filepath.Join(root, "data", "events", ".keep"),
		filepath.Join(root, "data", "events", "players", ".keep"),
	} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected deprecated generated file to stay absent: %s", path)
		}
	}

	for _, path := range []string{
		filepath.Join(root, "data", "events", "all.jsonl"),
		filepath.Join(root, "data", "state", "scripts.json"),
		filepath.Join(root, "configs", "json", "ops.json"),
		filepath.Join(root, "configs", "json", "block_names.json"),
		filepath.Join(root, "data", "vanilla", "profiles.json"),
		filepath.Join(root, "data", "vanilla", "content_ids.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected bootstrap output %s: %v", path, err)
		}
	}
	if st, err := os.Stat(filepath.Join(root, "data", "snapshots", "runtime")); err != nil || !st.IsDir() {
		t.Fatalf("expected cold snapshot directory: %v", err)
	}
}
