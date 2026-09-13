package oracle

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type JavaWorkspace struct {
	Dir          string   `json:"dir"`
	ScenarioFile string   `json:"scenarioFile"`
	OutputFile   string   `json:"outputFile"`
	LauncherFile string   `json:"launcherFile"`
	InitScript   string   `json:"initScript"`
	ClassesDir   string   `json:"classesDir"`
	Plan         JavaPlan `json:"plan"`
}

func PrepareJavaWorkspace(root RootInfo, dir string, scenario Scenario) (JavaWorkspace, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return JavaWorkspace{}, fmt.Errorf("dir is empty")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return JavaWorkspace{}, err
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return JavaWorkspace{}, err
	}
	scenario = scenario.Normalized()
	scenarioFile := filepath.Join(absDir, "scenario.json")
	outputFile := filepath.Join(absDir, "trace-official.json")
	plan, err := BuildJavaPlan(root, absDir, scenarioFile, outputFile)
	if err != nil {
		return JavaWorkspace{}, err
	}
	if err := writeScenarioFile(scenarioFile, scenario); err != nil {
		return JavaWorkspace{}, err
	}
	if err := WriteJavaLauncher(plan.LauncherFile); err != nil {
		return JavaWorkspace{}, err
	}
	if err := WriteJavaInitScript(plan.InitScriptFile); err != nil {
		return JavaWorkspace{}, err
	}
	return JavaWorkspace{
		Dir:          absDir,
		ScenarioFile: scenarioFile,
		OutputFile:   outputFile,
		LauncherFile: plan.LauncherFile,
		InitScript:   plan.InitScriptFile,
		ClassesDir:   plan.ClassesDir,
		Plan:         plan,
	}, nil
}

func writeScenarioFile(path string, scenario Scenario) error {
	raw, err := json.MarshalIndent(scenario.Normalized(), "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(path, raw, 0o644)
}
