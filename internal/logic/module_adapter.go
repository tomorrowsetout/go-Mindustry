package logic

import (
	"context"
	"strings"

	"mdt-server/internal/core"
)

// Module adapts the Mindustry Logic interpreter to the core.Module /
// core.LogicModule contract.
//
// Each Run call uses a fresh executor so concurrent programs do not share VM
// state. The narrow contract intentionally exposes only source-in / output-out;
// a foreign-language reimplementation only needs to honor Run.
type Module struct{}

// NewModule returns the logic module.
func NewModule() *Module { return &Module{} }

// Name implements core.Module.
func (m *Module) Name() string { return "logic" }

// Dependencies implements core.Module. The interpreter is standalone.
func (m *Module) Dependencies() []string { return nil }

// Init implements core.Module.
func (m *Module) Init(_ core.Deps) error { return nil }

// Start implements core.Module. Execution is demand-driven.
func (m *Module) Start() error { return nil }

// Stop implements core.Module. Nothing to release.
func (m *Module) Stop(_ context.Context) error { return nil }

// Run implements core.LogicModule by compiling and executing the program,
// returning the joined output log.
func (m *Module) Run(source string) (string, error) {
	exec := NewLogicExecutor()
	if err := exec.Execute(source); err != nil {
		return "", err
	}
	return strings.Join(exec.GetOutput(), "\n"), nil
}

// Compile-time guarantees that the adapter satisfies the contracts.
var (
	_ core.Module      = (*Module)(nil)
	_ core.LogicModule = (*Module)(nil)
)
