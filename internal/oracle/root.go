package oracle

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	RootEnvVar          = "MINDUSTRY_158_ROOT"
	LegacyRootEnvVar    = "MINDUSTRY_1574_ROOT"
	DefaultWindowsRoot  = `C:\Users\43551\Desktop\MDT\Mindustry-158`
	javaLauncherPackage = "mdtserver.oracle"
	javaLauncherClass   = "OfficialTraceLauncher"
)

type rootCandidate struct {
	Path   string
	Source string
}

type RootInfo struct {
	Root                 string `json:"root"`
	Source               string `json:"source"`
	BuildGradle          string `json:"buildGradle"`
	SettingsGradle       string `json:"settingsGradle"`
	GradleWrapper        string `json:"gradleWrapper"`
	GradleWrapperWindows string `json:"gradleWrapperWindows"`
	ServerLauncherSource string `json:"serverLauncherSource"`
	CoreClassesDir       string `json:"coreClassesDir"`
	ServerClassesDir     string `json:"serverClassesDir"`
	CoreResourcesDir     string `json:"coreResourcesDir"`
	ServerResourcesDir   string `json:"serverResourcesDir"`
}

func ResolveRoot() (RootInfo, error) {
	if override, source := firstRootOverride(); override != "" {
		info, err := DiscoverRoot(override)
		if err != nil {
			return RootInfo{}, fmt.Errorf("%s=%q is not a valid Mindustry root: %w", source, override, err)
		}
		info.Source = source
		return info, nil
	}

	candidates := defaultRootCandidates()
	var errs []string
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.Path) == "" {
			continue
		}
		info, err := DiscoverRoot(candidate.Path)
		if err == nil {
			info.Source = candidate.Source
			return info, nil
		}
		errs = append(errs, fmt.Sprintf("%s (%v)", candidate.Path, err))
	}

	if len(candidates) == 0 {
		return RootInfo{}, fmt.Errorf("could not resolve official Mindustry 158 root; set %s explicitly", RootEnvVar)
	}
	return RootInfo{}, fmt.Errorf("could not resolve official Mindustry 158 root; set %s or ensure one of these paths is valid: %s", RootEnvVar, strings.Join(errs, "; "))
}

func firstRootOverride() (string, string) {
	for _, name := range []string{RootEnvVar, LegacyRootEnvVar} {
		if value := strings.TrimSpace(os.Getenv(name)); value != "" {
			return value, name
		}
	}
	return "", ""
}

func defaultRootCandidates() []rootCandidate {
	if runtime.GOOS != "windows" {
		return nil
	}
	return []rootCandidate{
		{Path: DefaultWindowsRoot, Source: "default"},
	}
}

func DiscoverRoot(root string) (RootInfo, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return RootInfo{}, fmt.Errorf("root is empty")
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return RootInfo{}, err
	}
	info := RootInfo{
		Root:                 abs,
		BuildGradle:          filepath.Join(abs, "build.gradle"),
		SettingsGradle:       filepath.Join(abs, "settings.gradle"),
		GradleWrapper:        filepath.Join(abs, "gradlew"),
		GradleWrapperWindows: filepath.Join(abs, "gradlew.bat"),
		ServerLauncherSource: filepath.Join(abs, "server", "src", "mindustry", "server", "ServerLauncher.java"),
		CoreClassesDir:       filepath.Join(abs, "core", "build", "classes", "java", "main"),
		ServerClassesDir:     filepath.Join(abs, "server", "build", "classes", "java", "main"),
		CoreResourcesDir:     filepath.Join(abs, "core", "build", "resources", "main"),
		ServerResourcesDir:   filepath.Join(abs, "server", "build", "resources", "main"),
	}
	for _, required := range []string{
		info.BuildGradle,
		info.SettingsGradle,
		info.ServerLauncherSource,
	} {
		if err := requireRegularFile(required); err != nil {
			return RootInfo{}, err
		}
	}
	if runtime.GOOS == "windows" {
		if err := requireRegularFile(info.GradleWrapperWindows); err != nil {
			return RootInfo{}, err
		}
	} else {
		if err := requireRegularFile(info.GradleWrapper); err != nil {
			return RootInfo{}, err
		}
	}
	return info, nil
}

func (r RootInfo) PreferredGradleWrapper() string {
	if runtime.GOOS == "windows" {
		return r.GradleWrapperWindows
	}
	return r.GradleWrapper
}

func (r RootInfo) HasBuiltCoreClasses() bool {
	return dirExists(r.CoreClassesDir)
}

func (r RootInfo) HasBuiltServerClasses() bool {
	return dirExists(r.ServerClassesDir)
}

func requireRegularFile(path string) error {
	st, err := os.Stat(path)
	if err != nil {
		return err
	}
	if st.IsDir() {
		return fmt.Errorf("%s is a directory", path)
	}
	return nil
}

func dirExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}
