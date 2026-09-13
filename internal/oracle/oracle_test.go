package oracle

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestDiscoverRootValidatesExpectedMindustryTree(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "server", "src", "mindustry", "server"))
	mustWriteFile(t, filepath.Join(root, "build.gradle"), "plugins {}\n")
	mustWriteFile(t, filepath.Join(root, "settings.gradle"), "rootProject.name = 'mindustry'\n")
	mustWriteFile(t, filepath.Join(root, "server", "src", "mindustry", "server", "ServerLauncher.java"), "package mindustry.server;\n")
	if runtime.GOOS == "windows" {
		mustWriteFile(t, filepath.Join(root, "gradlew.bat"), "@echo off\r\n")
	} else {
		mustWriteFile(t, filepath.Join(root, "gradlew"), "#!/bin/sh\n")
	}

	info, err := DiscoverRoot(root)
	if err != nil {
		t.Fatalf("DiscoverRoot: %v", err)
	}
	if info.Root == "" {
		t.Fatal("expected resolved root")
	}
	if filepath.Clean(info.ServerLauncherSource) != filepath.Join(root, "server", "src", "mindustry", "server", "ServerLauncher.java") {
		t.Fatalf("unexpected launcher source path: %s", info.ServerLauncherSource)
	}
}

func TestBuildJavaPlanUsesOfficialClassDirsAndLauncher(t *testing.T) {
	root := RootInfo{
		Root:                 filepath.Join("C:", "oracle"),
		GradleWrapper:        filepath.Join("C:", "oracle", "gradlew"),
		GradleWrapperWindows: filepath.Join("C:", "oracle", "gradlew.bat"),
		CoreClassesDir:       filepath.Join("C:", "oracle", "core", "build", "classes", "java", "main"),
		ServerClassesDir:     filepath.Join("C:", "oracle", "server", "build", "classes", "java", "main"),
		CoreResourcesDir:     filepath.Join("C:", "oracle", "core", "build", "resources", "main"),
		ServerResourcesDir:   filepath.Join("C:", "oracle", "server", "build", "resources", "main"),
	}
	workspace := filepath.Join(t.TempDir(), "oracle-work")

	plan, err := BuildJavaPlan(root, workspace, "scenario.json", "trace.json")
	if err != nil {
		t.Fatalf("BuildJavaPlan: %v", err)
	}
	if !strings.Contains(strings.Join(plan.Prepare.Args, " "), ":server:classes") {
		t.Fatalf("expected prepare command to compile server classes, got %v", plan.Prepare.Args)
	}
	if !strings.Contains(strings.Join(plan.Compile.Args, " "), "mdtOracleCompile") {
		t.Fatalf("expected compile command to invoke mdtOracleCompile, got %v", plan.Compile.Args)
	}
	if !strings.Contains(strings.Join(plan.Compile.Args, " "), plan.InitScriptFile) {
		t.Fatalf("expected compile command to use generated init script %s, got %v", plan.InitScriptFile, plan.Compile.Args)
	}
	if !strings.Contains(strings.Join(plan.Run.Args, " "), "-PmdtOracleOut=trace.json") {
		t.Fatalf("expected run args to include trace output property, got %v", plan.Run.Args)
	}
}

func TestResolveRootUsesLocalDefaultWhenAvailable(t *testing.T) {
	if _, err := os.Stat(DefaultWindowsRoot); err != nil {
		if os.IsNotExist(err) {
			t.Skipf("default oracle root not present: %s", DefaultWindowsRoot)
		}
		t.Fatalf("stat default oracle root: %v", err)
	}
	previous, had := os.LookupEnv(RootEnvVar)
	if had {
		defer os.Setenv(RootEnvVar, previous)
	} else {
		defer os.Unsetenv(RootEnvVar)
	}
	if err := os.Unsetenv(RootEnvVar); err != nil {
		t.Fatalf("unset env: %v", err)
	}

	info, err := ResolveRoot()
	if err != nil {
		t.Fatalf("ResolveRoot: %v", err)
	}
	if filepath.Clean(info.Root) != filepath.Clean(DefaultWindowsRoot) {
		t.Fatalf("expected default root %s, got %s", DefaultWindowsRoot, info.Root)
	}
	if info.Source != "default" {
		t.Fatalf("expected default source marker, got %s", info.Source)
	}
}

func TestDefaultRootCandidatesPreferWorkspaceMirror(t *testing.T) {
	candidates := defaultRootCandidates()
	if runtime.GOOS != "windows" {
		if len(candidates) != 0 {
			t.Fatalf("expected no default candidates outside windows, got %#v", candidates)
		}
		return
	}
	if len(candidates) != 1 {
		t.Fatalf("expected one windows default candidate, got %#v", candidates)
	}
	if candidates[0].Path != DefaultWindowsRoot || candidates[0].Source != "default" {
		t.Fatalf("expected first candidate to be Mindustry 158 mirror, got %#v", candidates[0])
	}
}

func TestResolveOfficialRootUsesRootPathOption(t *testing.T) {
	root := t.TempDir()
	mustMkdirAll(t, filepath.Join(root, "server", "src", "mindustry", "server"))
	mustWriteFile(t, filepath.Join(root, "build.gradle"), "plugins {}\n")
	mustWriteFile(t, filepath.Join(root, "settings.gradle"), "rootProject.name = 'mindustry'\n")
	mustWriteFile(t, filepath.Join(root, "server", "src", "mindustry", "server", "ServerLauncher.java"), "package mindustry.server;\n")
	if runtime.GOOS == "windows" {
		mustWriteFile(t, filepath.Join(root, "gradlew.bat"), "@echo off\r\n")
	} else {
		mustWriteFile(t, filepath.Join(root, "gradlew"), "#!/bin/sh\n")
	}

	info, err := resolveOfficialRoot(OfficialTraceOptions{RootPath: root})
	if err != nil {
		t.Fatalf("resolveOfficialRoot: %v", err)
	}
	if filepath.Clean(info.Root) != filepath.Clean(root) {
		t.Fatalf("expected root %s, got %s", root, info.Root)
	}
	if info.Source != "options.rootPath" {
		t.Fatalf("expected rootPath source marker, got %s", info.Source)
	}
}

func TestWriteAndReadTraceRoundTrip(t *testing.T) {
	trace := NewTrace("go", Scenario{Name: "baseline", MapPath: "assets/worlds/file.msav", Seed: 7, Ticks: 10})
	trace.Ticks = append(trace.Ticks, TickTrace{
		Index: 1,
		Tiles: []TileState{{
			X:       10,
			Y:       20,
			BlockID: 339,
			TeamID:  1,
			Logic: LogicState{
				Enabled: true,
			},
		}},
	})

	path := filepath.Join(t.TempDir(), "trace.json")
	if err := WriteTrace(path, trace); err != nil {
		t.Fatalf("WriteTrace: %v", err)
	}
	loaded, err := ReadTrace(path)
	if err != nil {
		t.Fatalf("ReadTrace: %v", err)
	}
	if loaded.Producer != trace.Producer || loaded.Scenario.Name != trace.Scenario.Name {
		t.Fatalf("round trip mismatch: %#v vs %#v", loaded, trace)
	}
	if loaded.Tolerances.Position != PositionTolerance {
		t.Fatalf("unexpected default tolerance: %#v", loaded.Tolerances)
	}
}

func TestJavaLauncherSourceMatchesToolMirror(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
	mirrorPath := filepath.Join(repoRoot, "tools", "oracle", "java", "OfficialTraceLauncher.java")
	if _, err := os.Stat(mirrorPath); err != nil {
		if os.IsNotExist(err) {
			t.Skipf("launcher mirror not present in repo: %s", mirrorPath)
		}
		t.Fatalf("stat mirrored launcher source: %v", err)
	}
	raw, err := os.ReadFile(mirrorPath)
	if err != nil {
		t.Fatalf("read mirrored launcher source: %v", err)
	}
	if string(raw) != JavaLauncherSource {
		t.Fatalf("tool mirror is out of sync with embedded JavaLauncherSource")
	}
}

func TestPrepareJavaWorkspaceWritesScenarioAndLauncher(t *testing.T) {
	root := RootInfo{
		Root:                 filepath.Join("C:", "oracle"),
		GradleWrapper:        filepath.Join("C:", "oracle", "gradlew"),
		GradleWrapperWindows: filepath.Join("C:", "oracle", "gradlew.bat"),
		CoreClassesDir:       filepath.Join("C:", "oracle", "core", "build", "classes", "java", "main"),
		ServerClassesDir:     filepath.Join("C:", "oracle", "server", "build", "classes", "java", "main"),
		CoreResourcesDir:     filepath.Join("C:", "oracle", "core", "build", "resources", "main"),
		ServerResourcesDir:   filepath.Join("C:", "oracle", "server", "build", "resources", "main"),
	}
	workspace, err := PrepareJavaWorkspace(root, filepath.Join(t.TempDir(), "java-work"), Scenario{
		Name:    "workspace",
		MapPath: "assets/worlds/file.msav",
		Ticks:   5,
	})
	if err != nil {
		t.Fatalf("PrepareJavaWorkspace: %v", err)
	}
	for _, path := range []string{workspace.ScenarioFile, workspace.LauncherFile, workspace.InitScript} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected generated file %s: %v", path, err)
		}
	}
	if !strings.Contains(strings.Join(workspace.Plan.Run.Args, " "), "-PmdtOracleOut="+workspace.OutputFile) {
		t.Fatalf("expected run args to reference output file %s, got %v", workspace.OutputFile, workspace.Plan.Run.Args)
	}
}

func mustMkdirAll(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
}

func mustWriteFile(t *testing.T, path, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir parent for %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
