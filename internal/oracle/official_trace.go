package oracle

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

type OfficialTraceOptions struct {
	Root          RootInfo
	RootPath      string
	WorkspaceDir  string
	WorkspaceRoot string
}

func CollectOfficialTrace(scenario Scenario, opts OfficialTraceOptions) (Trace, error) {
	root, err := resolveOfficialRoot(opts)
	if err != nil {
		return Trace{}, err
	}

	workdir := strings.TrimSpace(opts.WorkspaceDir)
	if workdir == "" {
		workdir = filepath.Join(os.TempDir(), "mdt-oracle-work")
	}
	resolvedScenario, err := resolveOfficialScenario(scenario, opts.WorkspaceRoot)
	if err != nil {
		return Trace{}, err
	}
	workspace, err := PrepareJavaWorkspace(root, workdir, resolvedScenario)
	if err != nil {
		return Trace{}, err
	}

	for _, cmd := range []Command{workspace.Plan.Prepare, workspace.Plan.Compile, workspace.Plan.Run} {
		if err := runCommand(cmd); err != nil {
			return Trace{}, err
		}
	}

	trace, err := ReadTrace(workspace.OutputFile)
	if err != nil {
		return Trace{}, fmt.Errorf("read official trace %s: %w", workspace.OutputFile, err)
	}
	return trace, nil
}

func resolveOfficialRoot(opts OfficialTraceOptions) (RootInfo, error) {
	if opts.Root.Root != "" {
		return opts.Root, nil
	}
	if rootPath := strings.TrimSpace(opts.RootPath); rootPath != "" {
		info, err := DiscoverRoot(rootPath)
		if err != nil {
			return RootInfo{}, fmt.Errorf("official trace rootPath %q is invalid: %w", rootPath, err)
		}
		info.Source = "options.rootPath"
		return info, nil
	}
	return ResolveRoot()
}

func runCommand(command Command) error {
	if strings.TrimSpace(command.Program) == "" {
		return fmt.Errorf("command program is empty")
	}
	cmd := exec.Command(command.Program, command.Args...)
	cmd.Dir = command.Dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(output))
		if text == "" {
			return fmt.Errorf("run %s %v: %w", command.Program, command.Args, err)
		}
		return fmt.Errorf("run %s %v: %w\n%s", command.Program, command.Args, err, text)
	}
	return nil
}

func resolveOfficialScenario(scenario Scenario, workspaceRoot string) (Scenario, error) {
	scenario = scenario.Normalized()
	var err error
	scenario.MapPath, err = resolveScenarioFile(workspaceRoot, scenario.MapPath)
	if err != nil {
		return Scenario{}, fmt.Errorf("resolve official map path: %w", err)
	}
	if strings.TrimSpace(scenario.HotReloadMapPath) != "" {
		scenario.HotReloadMapPath, err = resolveScenarioFile(workspaceRoot, scenario.HotReloadMapPath)
		if err != nil {
			return Scenario{}, fmt.Errorf("resolve official hot reload map path: %w", err)
		}
	}
	if strings.TrimSpace(scenario.VanillaProfilesPath) != "" {
		scenario.VanillaProfilesPath, err = resolveScenarioFile(workspaceRoot, scenario.VanillaProfilesPath)
		if err != nil {
			return Scenario{}, fmt.Errorf("resolve official vanilla profiles path: %w", err)
		}
	}
	return scenario, nil
}
