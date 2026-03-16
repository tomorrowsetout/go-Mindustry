// Package logic provides a Mindustry logic processor implementation
// for the mdt-server project.
//
// This package implements a lexer, parser, and virtual machine
// for executing Mindustry logic programs.
package logic

import (
	"fmt"
	"strings"
)

// LogicExecutor executes Mindustry logic programs.
type LogicExecutor struct {
	Program     *Program        // Parsed program
	VM          *VM             // Virtual machine
	Instructions []Instruction   // Generated instructions
	inputs      map[string]int32 // Input values
	outputs     []string        // Output log
}

// NewLogicExecutor creates a new logic executor.
func NewLogicExecutor() *LogicExecutor {
	return &LogicExecutor{
		Program:     nil,
		VM:          NewVM(),
		Instructions: make([]Instruction, 0),
		inputs:      make(map[string]int32),
		outputs:     make([]string, 0),
	}
}

// Compile compiles source code into a program and generates instructions.
func (e *LogicExecutor) Compile(source string) error {
	// Parse the source code
	program, errs := ParseProgram(source)
	if len(errs) > 0 {
		var errStrings []string
		for _, err := range errs {
			errStrings = append(errStrings, err.Error())
		}
		return fmt.Errorf("parse error: %s", strings.Join(errStrings, "; "))
	}

	e.Program = program

	// Generate VM instructions from the program
	e.generateInstructions(program)

	return nil
}

// generateInstructions generates VM instructions from the parsed program.
func (e *LogicExecutor) generateInstructions(program *Program) {
	e.Instructions = make([]Instruction, 0, len(program.Statements)*2)

	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *AssignStatement:
			e.generateAssign(s)

		case *PrintStatement:
			e.generatePrint(s)

		case *JumpStatement:
			e.generateJump(s)

		case *LabelStatement:
			e.generateLabel(s)

		case *IfStatement:
			e.generateIf(s)

		default:
			// Skip unknown statement types
		}
	}

	// Add halt at the end
	e.Instructions = append(e.Instructions, Instruction{
		Opcode: HALT,
		Args:   []int32{},
	})
}

// generateAssign generates instructions for an assignment statement.
func (e *LogicExecutor) generateAssign(stmt *AssignStatement) {
	e.Instructions = append(e.Instructions, Instruction{
		Opcode: MOV,
		Args:   []int32{0, e.getOrSetRegister(stmt.Name.Value)},
	})
}

// generatePrint generates instructions for a print statement.
func (e *LogicExecutor) generatePrint(stmt *PrintStatement) {
	e.Instructions = append(e.Instructions, Instruction{
		Opcode: OUTPUT,
		Args:   []int32{0, 0},
	})
}

// generateJump generates instructions for a jump statement.
func (e *LogicExecutor) generateJump(stmt *JumpStatement) {
	// For now, use a placeholder - labels will be expanded later
	e.Instructions = append(e.Instructions, Instruction{
		Opcode: JMP,
		Args:   []int32{0},
	})
}

// generateLabel generates instructions for a label statement.
func (e *LogicExecutor) generateLabel(stmt *LabelStatement) {
	// Labels are represented as markers in the instruction stream
	// For now, we'll just note the label position
	e.Instructions = append(e.Instructions, Instruction{
		Opcode: NoOp,
		Args:   []int32{0},
	})
}

// generateIf generates instructions for an if statement.
func (e *LogicExecutor) generateIf(stmt *IfStatement) {
	// For now, skip condition evaluation (in real implementation, this would generate compare ops)
	_ = stmt.Condition

	// Jump if false (zero)
	e.Instructions = append(e.Instructions, Instruction{
		Opcode: JZ,
		Args:   []int32{0, 0}, // Will be updated later
	})

	// Generate body
	for _, bodyStmt := range stmt.Body {
		switch s := bodyStmt.(type) {
		case *AssignStatement:
			e.generateAssign(s)

		case *PrintStatement:
			e.generatePrint(s)

		default:
			// Skip unknown statement types
		}
	}

	// After body, jump over else if exists (for now just continue)
	e.Instructions = append(e.Instructions, Instruction{
		Opcode: JMP,
		Args:   []int32{0},
	})
}

// getOrSetRegister gets or sets a register value.
func (e *LogicExecutor) getOrSetRegister(name string) int32 {
	// In a real implementation, this would track register allocation
	// For now, just return a fixed value
	return 0
}

// Run runs the compiled program.
func (e *LogicExecutor) Run() error {
	if e.Program == nil {
		return fmt.Errorf("no program compiled")
	}

	// Load instructions into the VM
	e.VM.LoadInstructions(e.Instructions)

	// Set initial inputs
	for key, val := range e.inputs {
		e.VM.SetInput(key, val)
	}

	// Run the VM
	return e.VM.Run()
}

// Step executes a single instruction step.
func (e *LogicExecutor) Step() error {
	if e.Program == nil {
		return fmt.Errorf("no program compiled")
	}

	// Load instructions into the VM if not already loaded
	if len(e.VM.GetInstructions()) == 0 {
		e.VM.LoadInstructions(e.Instructions)
	}

	// Set initial inputs
	for key, val := range e.inputs {
		e.VM.SetInput(key, val)
	}

	return e.VM.Step()
}

// Reset resets the executor.
func (e *LogicExecutor) Reset() {
	e.Program = nil
	e.VM = NewVM()
	e.Instructions = make([]Instruction, 0)
	e.inputs = make(map[string]int32)
	e.outputs = make([]string, 0)
}

// GetOutput returns the output log.
func (e *LogicExecutor) GetOutput() []string {
	return e.outputs
}

// GetOutputValue returns a specific output value.
func (e *LogicExecutor) GetOutputValue(key string) int32 {
	if val, ok := e.VM.GetOutputs()[key]; ok {
		return val
	}
	return 0
}

// SetInput sets an input value.
func (e *LogicExecutor) SetInput(key string, value int32) {
	e.inputs[key] = value
	e.VM.SetInput(key, value)
}

// GetInput returns an input value.
func (e *LogicExecutor) GetInput(key string) int32 {
	if val, ok := e.inputs[key]; ok {
		return val
	}
	return 0
}

// GetRegisters returns all register values.
func (e *LogicExecutor) GetRegisters() map[string]int32 {
	return e.VM.GetRegisters()
}

// GetRegister returns a specific register value.
func (e *LogicExecutor) GetRegister(name string) int32 {
	return e.VM.GetRegister(name)
}

// SetRegister sets a register value.
func (e *LogicExecutor) SetRegister(name string, value int32) {
	e.VM.SetRegister(name, value)
}

// GetMemory returns all memory values.
func (e *LogicExecutor) GetMemory() map[int32]int32 {
	return e.VM.GetMemoryMap()
}

// GetMemoryAt returns a memory value at an address.
func (e *LogicExecutor) GetMemoryAt(addr int32) int32 {
	return e.VM.GetMemory(addr)
}

// SetMemoryAt sets a memory value at an address.
func (e *LogicExecutor) SetMemoryAt(addr int32, value int32) {
	e.VM.SetMemory(addr, value)
}

// GetProgram returns the parsed program.
func (e *LogicExecutor) GetProgram() *Program {
	return e.Program
}

// GetInstructions returns the generated instructions.
func (e *LogicExecutor) GetInstructions() []Instruction {
	return e.Instructions
}

// GetVM returns the VM instance.
func (e *LogicExecutor) GetVM() *VM {
	return e.VM
}

// GetInputMap returns the inputs map.
func (e *LogicExecutor) GetInputMap() map[string]int32 {
	return e.inputs
}

// GetOutputMap returns the outputs map.
func (e *LogicExecutor) GetOutputMap() map[string]int32 {
	return e.VM.GetOutputs()
}

// SetProgram sets the program.
func (e *LogicExecutor) SetProgram(program *Program) {
	e.Program = program
}

// SetInstructions sets the instructions.
func (e *LogicExecutor) SetInstructions(instructions []Instruction) {
	e.Instructions = make([]Instruction, len(instructions))
	copy(e.Instructions, instructions)
}

// Execute runs the executor with source code.
func (e *LogicExecutor) Execute(source string) error {
	if err := e.Compile(source); err != nil {
		return err
	}
	return e.Run()
}

// ExecuteStep executes one step with source code.
func (e *LogicExecutor) ExecuteStep(source string) error {
	if err := e.Compile(source); err != nil {
		return err
	}
	return e.Step()
}

// Debug returns debug information.
func (e *LogicExecutor) Debug() string {
	var sb strings.Builder
	sb.WriteString("LogicExecutor{\n")
	sb.WriteString(fmt.Sprintf("  Program: %v\n", e.Program != nil))
	sb.WriteString(fmt.Sprintf("  VM: %s\n", e.VM.Debug()))
	sb.WriteString(fmt.Sprintf("  Instructions: %d\n", len(e.Instructions)))
	sb.WriteString(fmt.Sprintf("  Inputs: %v\n", e.inputs))
	sb.WriteString(fmt.Sprintf("  Outputs: %v\n", e.outputs))
	sb.WriteString("}")
	return sb.String()
}

// PrintInstructions prints all instructions.
func (e *LogicExecutor) PrintInstructions() {
	for i, inst := range e.Instructions {
		fmt.Printf("%3d: %s\n", i, inst)
	}
}

// PrintProgram prints the parsed program.
func (e *LogicExecutor) PrintProgram() {
	if e.Program != nil {
		fmt.Println(e.Program.String())
	}
}

// GetProgramStats returns program statistics.
func (e *LogicExecutor) GetProgramStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["instructions"] = len(e.Instructions)
	stats["registers"] = len(e.VM.GetRegisters())
	stats["memory"] = len(e.VM.GetMemoryMap())
	stats["inputs"] = len(e.inputs)
	stats["outputs"] = len(e.outputs)
	stats["program"] = e.Program != nil
	return stats
}

// GetVMStats returns VM statistics.
func (e *LogicExecutor) GetVMStats() map[string]interface{} {
	stats := make(map[string]interface{})
	stats["pc"] = e.VM.GetPC()
	stats["halted"] = e.VM.IsHalted()
	stats["registers"] = len(e.VM.GetRegisters())
	stats["memory"] = len(e.VM.GetMemoryMap())
	stats["stack"] = len(e.VM.GetStack())
	stats["inputs"] = len(e.VM.Inputs)
	stats["outputs"] = len(e.VM.Outputs)
	return stats
}

// ToJSON returns the executor state as a map for JSON serialization.
func (e *LogicExecutor) ToJSON() map[string]interface{} {
	state := make(map[string]interface{})
	state["instructions"] = len(e.Instructions)
	state["pc"] = e.VM.GetPC()
	state["halted"] = e.VM.IsHalted()
	state["registers"] = e.VM.Registers
	state["inputs"] = e.inputs
	state["outputs"] = e.VM.Outputs
	return state
}

// FromJSON loads the executor state from a map.
func (e *LogicExecutor) FromJSON(state map[string]interface{}) error {
	return nil // Not implemented
}

// MoveTo exec moves the executor to a target state.
func (e *LogicExecutor) MoveTo(target *LogicExecutor) {
	e.Program = target.Program
	e.Instructions = make([]Instruction, len(target.Instructions))
	copy(e.Instructions, target.Instructions)
	e.VM = target.VM
	e.inputs = make(map[string]int32)
	for k, v := range target.inputs {
		e.inputs[k] = v
	}
	e.outputs = make([]string, len(target.outputs))
	copy(e.outputs, target.outputs)
}

// Merge merges with another executor.
func (e *LogicExecutor) Merge(other *LogicExecutor) {
	if other.Program != nil {
		e.Program = other.Program
	}
	e.Instructions = append(e.Instructions, other.Instructions...)
	for k, v := range other.inputs {
		e.inputs[k] = v
	}
	e.outputs = append(e.outputs, other.outputs...)
}

// Copy creates a copy of the executor.
func (e *LogicExecutor) Copy() *LogicExecutor {
	other := NewLogicExecutor()
	other.Program = e.Program
	other.Instructions = make([]Instruction, len(e.Instructions))
	copy(other.Instructions, e.Instructions)
	other.VM = e.VM.Clone()
	for k, v := range e.inputs {
		other.inputs[k] = v
	}
	other.outputs = make([]string, len(e.outputs))
	copy(other.outputs, e.outputs)
	return other
}

// Clone creates a clone of the executor.
func (e *LogicExecutor) Clone() *LogicExecutor {
	return e.Copy()
}

// ExecuteLogic executes logic from the executor.
func (e *LogicExecutor) ExecuteLogic() error {
	return e.Run()
}

// ExecuteLogicStep executes one step of logic.
func (e *LogicExecutor) ExecuteLogicStep() error {
	return e.Step()
}

// ExecuteCommand executes a single command.
func (e *LogicExecutor) ExecuteCommand(cmd string) error {
	return e.Execute(cmd)
}

// ExecuteCommandStep executes one step of a command.
func (e *LogicExecutor) ExecuteCommandStep(cmd string) error {
	return e.ExecuteStep(cmd)
}

// CompileAndRun compiles and runs logic.
func (e *LogicExecutor) CompileAndRun(source string) error {
	if err := e.Compile(source); err != nil {
		return err
	}
	return e.Run()
}

// CompileAndStep compiles and runs one step.
func (e *LogicExecutor) CompileAndStep(source string) error {
	if err := e.Compile(source); err != nil {
		return err
	}
	return e.Step()
}

// RunWithInputs runs with specific inputs.
func (e *LogicExecutor) RunWithInputs(inputs map[string]int32) error {
	e.inputs = make(map[string]int32)
	for k, v := range inputs {
		e.inputs[k] = v
	}
	return e.Run()
}

// RunStepWithInputs runs one step with specific inputs.
func (e *LogicExecutor) RunStepWithInputs(inputs map[string]int32) error {
	e.inputs = make(map[string]int32)
	for k, v := range inputs {
		e.inputs[k] = v
	}
	return e.Step()
}

// ExecuteWithInputs executes with specific inputs.
func (e *LogicExecutor) ExecuteWithInputs(source string, inputs map[string]int32) error {
	if err := e.Compile(source); err != nil {
		return err
	}
	return e.RunWithInputs(inputs)
}

// ExecuteStepWithInputs executes one step with specific inputs.
func (e *LogicExecutor) ExecuteStepWithInputs(source string, inputs map[string]int32) error {
	if err := e.Compile(source); err != nil {
		return err
	}
	return e.RunStepWithInputs(inputs)
}

// SetProgramFromInstructions sets program from instructions.
func (e *LogicExecutor) SetProgramFromInstructions(instructions []Instruction) {
	e.Instructions = make([]Instruction, len(instructions))
	copy(e.Instructions, instructions)
}

// GetInstructionCount returns the instruction count.
func (e *LogicExecutor) GetInstructionCount() int {
	return len(e.Instructions)
}

// GetPC returns the program counter.
func (e *LogicExecutor) GetPC() int {
	return e.VM.GetPC()
}

// SetPC sets the program counter.
func (e *LogicExecutor) SetPC(pc int) {
	e.VM.SetPC(pc)
}

// IsHalted returns whether execution has halted.
func (e *LogicExecutor) IsHalted() bool {
	return e.VM.IsHalted()
}

// GetLastError returns the last error.
func (e *LogicExecutor) GetLastError() error {
	return e.VM.GetLastError()
}

// Clear clears the executor.
func (e *LogicExecutor) Clear() {
	e.Program = nil
	e.VM.Clear()
	e.Instructions = make([]Instruction, 0)
	e.inputs = make(map[string]int32)
	e.outputs = make([]string, 0)
}

// AddOutput adds to the output log.
func (e *LogicExecutor) AddOutput(output string) {
	e.outputs = append(e.outputs, output)
}

// GetOutputAt returns output at index.
func (e *LogicExecutor) GetOutputAt(idx int) string {
	if idx < 0 || idx >= len(e.outputs) {
		return ""
	}
	return e.outputs[idx]
}

// SetOutputAt sets output at index.
func (e *LogicExecutor) SetOutputAt(idx int, output string) {
	if idx >= 0 && idx < len(e.outputs) {
		e.outputs[idx] = output
	}
}

// PopOutput pops the last output.
func (e *LogicExecutor) PopOutput() string {
	if len(e.outputs) == 0 {
		return ""
	}
	output := e.outputs[len(e.outputs)-1]
	e.outputs = e.outputs[:len(e.outputs)-1]
	return output
}

// PushOutput pushes an output.
func (e *LogicExecutor) PushOutput(output string) {
	e.outputs = append(e.outputs, output)
}

// ClearOutputs clears all outputs.
func (e *LogicExecutor) ClearOutputs() {
	e.outputs = make([]string, 0)
}

// GetOutputLength returns the output length.
func (e *LogicExecutor) GetOutputLength() int {
	return len(e.outputs)
}

// IsOutputsEmpty checks if outputs are empty.
func (e *LogicExecutor) IsOutputsEmpty() bool {
	return len(e.outputs) == 0
}

// IsOutputsNotEmpty checks if outputs are not empty.
func (e *LogicExecutor) IsOutputsNotEmpty() bool {
	return len(e.outputs) > 0
}

// GetFirstOutput returns the first output.
func (e *LogicExecutor) GetFirstOutput() string {
	if len(e.outputs) == 0 {
		return ""
	}
	return e.outputs[0]
}

// GetLastOutput returns the last output.
func (e *LogicExecutor) GetLastOutput() string {
	if len(e.outputs) == 0 {
		return ""
	}
	return e.outputs[len(e.outputs)-1]
}

// ContainsOutput checks if output contains a value.
func (e *LogicExecutor) ContainsOutput(value string) bool {
	for _, output := range e.outputs {
		if output == value {
			return true
		}
	}
	return false
}

// OutputCount returns count of a value in outputs.
func (e *LogicExecutor) OutputCount(value string) int {
	count := 0
	for _, output := range e.outputs {
		if output == value {
			count++
		}
	}
	return count
}

// FindOutputIndex finds the index of an output value.
func (e *LogicExecutor) FindOutputIndex(value string) int {
	for i, output := range e.outputs {
		if output == value {
			return i
		}
	}
	return -1
}

// FindLastOutputIndex finds the last index of an output value.
func (e *LogicExecutor) FindLastOutputIndex(value string) int {
	for i := len(e.outputs) - 1; i >= 0; i-- {
		if e.outputs[i] == value {
			return i
		}
	}
	return -1
}

// OutputSlice returns a slice of outputs.
func (e *LogicExecutor) OutputSlice(start, end int) []string {
	if start < 0 {
		start = 0
	}
	if end > len(e.outputs) {
		end = len(e.outputs)
	}
	if start >= end {
		return nil
	}
	return e.outputs[start:end]
}

// OutputReverse reverses the outputs.
func (e *LogicExecutor) OutputReverse() {
	for i, j := 0, len(e.outputs)-1; i < j; i, j = i+1, j-1 {
		e.outputs[i], e.outputs[j] = e.outputs[j], e.outputs[i]
	}
}

// OutputSort sorts the outputs.
func (e *LogicExecutor) OutputSort() {
	for i := 0; i < len(e.outputs); i++ {
		for j := i + 1; j < len(e.outputs); j++ {
			if e.outputs[i] > e.outputs[j] {
				e.outputs[i], e.outputs[j] = e.outputs[j], e.outputs[i]
			}
		}
	}
}

// OutputCopy copies the outputs.
func (e *LogicExecutor) OutputCopy() []string {
	result := make([]string, len(e.outputs))
	copy(result, e.outputs)
	return result
}

// OutputClone clones the outputs.
func (e *LogicExecutor) OutputClone() []string {
	return e.OutputCopy()
}

// OutputConcat concatenates outputs.
func (e *LogicExecutor) OutputConcat(outputs ...string) {
	e.outputs = append(e.outputs, outputs...)
}

// OutputPrepend prepends outputs.
func (e *LogicExecutor) OutputPrepend(outputs ...string) {
	e.outputs = append(outputs, e.outputs...)
}

// OutputInsert inserts an output at a position.
func (e *LogicExecutor) OutputInsert(pos int, output string) bool {
	if pos < 0 || pos > len(e.outputs) {
		return false
	}
	e.outputs = append(e.outputs[:pos], append([]string{output}, e.outputs[pos:]...)...)
	return true
}

// OutputRemove removes an output at a position.
func (e *LogicExecutor) OutputRemove(pos int) bool {
	if pos < 0 || pos >= len(e.outputs) {
		return false
	}
	e.outputs = append(e.outputs[:pos], e.outputs[pos+1:]...)
	return true
}

// OutputReplace replaces an output at a position.
func (e *LogicExecutor) OutputReplace(pos int, output string) bool {
	if pos < 0 || pos >= len(e.outputs) {
		return false
	}
	e.outputs[pos] = output
	return true
}

// OutputSwap swaps two outputs.
func (e *LogicExecutor) OutputSwap(i, j int) bool {
	if i < 0 || j < 0 || i >= len(e.outputs) || j >= len(e.outputs) {
		return false
	}
	e.outputs[i], e.outputs[j] = e.outputs[j], e.outputs[i]
	return true
}

// OutputFilter filters outputs by a predicate.
func (e *LogicExecutor) OutputFilter(pred func(string) bool) {
	result := make([]string, 0, len(e.outputs))
	for _, output := range e.outputs {
		if pred(output) {
			result = append(result, output)
		}
	}
	e.outputs = result
}

// OutputMap maps outputs by a function.
func (e *LogicExecutor) OutputMap(fn func(string) string) {
	for i, output := range e.outputs {
		e.outputs[i] = fn(output)
	}
}

// OutputReduce reduces outputs by a function.
func (e *LogicExecutor) OutputReduce(initial string, fn func(string, string) string) string {
	result := initial
	for _, output := range e.outputs {
		result = fn(result, output)
	}
	return result
}

// OutputAllMatch checks if all outputs match a predicate.
func (e *LogicExecutor) OutputAllMatch(pred func(string) bool) bool {
	for _, output := range e.outputs {
		if !pred(output) {
			return false
		}
	}
	return true
}

// OutputAnyMatch checks if any output matches a predicate.
func (e *LogicExecutor) OutputAnyMatch(pred func(string) bool) bool {
	for _, output := range e.outputs {
		if pred(output) {
			return true
		}
	}
	return false
}

// OutputFind finds an output matching a predicate.
func (e *LogicExecutor) OutputFind(pred func(string) bool) (string, bool) {
	for _, output := range e.outputs {
		if pred(output) {
			return output, true
		}
	}
	return "", false
}

// OutputFindIndex finds the index of an output matching a predicate.
func (e *LogicExecutor) OutputFindIndex(pred func(string) bool) int {
	for i, output := range e.outputs {
		if pred(output) {
			return i
		}
	}
	return -1
}

// OutputFindLast finds the last output matching a predicate.
func (e *LogicExecutor) OutputFindLast(pred func(string) bool) (string, bool) {
	for i := len(e.outputs) - 1; i >= 0; i-- {
		if pred(e.outputs[i]) {
			return e.outputs[i], true
		}
	}
	return "", false
}

// OutputFindLastIndex finds the last index of an output matching a predicate.
func (e *LogicExecutor) OutputFindLastIndex(pred func(string) bool) int {
	for i := len(e.outputs) - 1; i >= 0; i-- {
		if pred(e.outputs[i]) {
			return i
		}
	}
	return -1
}

// OutputCountBy returns count of each output value.
func (e *LogicExecutor) OutputCountBy() map[string]int {
	counts := make(map[string]int)
	for _, output := range e.outputs {
		counts[output]++
	}
	return counts
}

// OutputUnique returns unique outputs.
func (e *LogicExecutor) OutputUnique() []string {
	seen := make(map[string]bool)
	result := make([]string, 0)
	for _, output := range e.outputs {
		if !seen[output] {
			seen[output] = true
			result = append(result, output)
		}
	}
	return result
}

// AddInstruction adds an instruction.
func (e *LogicExecutor) AddInstruction(inst Instruction) {
	e.Instructions = append(e.Instructions, inst)
}

// InsertInstruction inserts an instruction at a position.
func (e *LogicExecutor) InsertInstruction(pos int, inst Instruction) bool {
	if pos < 0 || pos > len(e.Instructions) {
		return false
	}
	e.Instructions = append(e.Instructions[:pos], append([]Instruction{inst}, e.Instructions[pos:]...)...)
	return true
}

// RemoveInstruction removes an instruction at a position.
func (e *LogicExecutor) RemoveInstruction(pos int) bool {
	if pos < 0 || pos >= len(e.Instructions) {
		return false
	}
	e.Instructions = append(e.Instructions[:pos], e.Instructions[pos+1:]...)
	return true
}

// ReplaceInstruction replaces an instruction at a position.
func (e *LogicExecutor) ReplaceInstruction(pos int, inst Instruction) bool {
	if pos < 0 || pos >= len(e.Instructions) {
		return false
	}
	e.Instructions[pos] = inst
	return true
}

// SwapInstructions swaps two instructions.
func (e *LogicExecutor) SwapInstructions(i, j int) bool {
	if i < 0 || j < 0 || i >= len(e.Instructions) || j >= len(e.Instructions) {
		return false
	}
	e.Instructions[i], e.Instructions[j] = e.Instructions[j], e.Instructions[i]
	return true
}

// GetInstructionAt returns an instruction at a position.
func (e *LogicExecutor) GetInstructionAt(pos int) (Instruction, bool) {
	if pos < 0 || pos >= len(e.Instructions) {
		return Instruction{}, false
	}
	return e.Instructions[pos], true
}

// SetInstructionAt sets an instruction at a position.
func (e *LogicExecutor) SetInstructionAt(pos int, inst Instruction) bool {
	if pos < 0 || pos >= len(e.Instructions) {
		return false
	}
	e.Instructions[pos] = inst
	return true
}

// FindInstructionIndex finds the index of an instruction.
func (e *LogicExecutor) FindInstructionIndex(pred func(Instruction) bool) int {
	for i, inst := range e.Instructions {
		if pred(inst) {
			return i
		}
	}
	return -1
}

// FindLastInstructionIndex finds the last index of an instruction.
func (e *LogicExecutor) FindLastInstructionIndex(pred func(Instruction) bool) int {
	for i := len(e.Instructions) - 1; i >= 0; i-- {
		if pred(e.Instructions[i]) {
			return i
		}
	}
	return -1
}

// CountInstructionsByOpcode counts instructions by opcode.
func (e *LogicExecutor) CountInstructionsByOpcode() map[Opcode]int {
	counts := make(map[Opcode]int)
	for _, inst := range e.Instructions {
		counts[inst.Opcode]++
	}
	return counts
}

// FilterInstructions filters instructions by a predicate.
func (e *LogicExecutor) FilterInstructions(pred func(Instruction) bool) {
	result := make([]Instruction, 0, len(e.Instructions))
	for _, inst := range e.Instructions {
		if pred(inst) {
			result = append(result, inst)
		}
	}
	e.Instructions = result
}

// MapInstructions maps instructions by a function.
func (e *LogicExecutor) MapInstructions(fn func(Instruction) Instruction) {
	for i, inst := range e.Instructions {
		e.Instructions[i] = fn(inst)
	}
}

// ReduceInstructions reduces instructions by a function.
func (e *LogicExecutor) ReduceInstructions(initial Instruction, fn func(Instruction, Instruction) Instruction) Instruction {
	result := initial
	for _, inst := range e.Instructions {
		result = fn(result, inst)
	}
	return result
}

// AllInstructionsMatch checks if all instructions match a predicate.
func (e *LogicExecutor) AllInstructionsMatch(pred func(Instruction) bool) bool {
	for _, inst := range e.Instructions {
		if !pred(inst) {
			return false
		}
	}
	return true
}

// AnyInstructionMatches checks if any instruction matches a predicate.
func (e *LogicExecutor) AnyInstructionMatches(pred func(Instruction) bool) bool {
	for _, inst := range e.Instructions {
		if pred(inst) {
			return true
		}
	}
	return false
}

// FindInstruction finds an instruction matching a predicate.
func (e *LogicExecutor) FindInstruction(pred func(Instruction) bool) (Instruction, bool) {
	for _, inst := range e.Instructions {
		if pred(inst) {
			return inst, true
		}
	}
	return Instruction{}, false
}

// FindLastInstruction finds the last instruction matching a predicate.
func (e *LogicExecutor) FindLastInstruction(pred func(Instruction) bool) (Instruction, bool) {
	for i := len(e.Instructions) - 1; i >= 0; i-- {
		if pred(e.Instructions[i]) {
			return e.Instructions[i], true
		}
	}
	return Instruction{}, false
}

// GetLastInstruction returns the last instruction.
func (e *LogicExecutor) GetLastInstruction() (Instruction, bool) {
	if len(e.Instructions) == 0 {
		return Instruction{}, false
	}
	return e.Instructions[len(e.Instructions)-1], true
}

// GetFirstInstruction returns the first instruction.
func (e *LogicExecutor) GetFirstInstruction() (Instruction, bool) {
	if len(e.Instructions) == 0 {
		return Instruction{}, false
	}
	return e.Instructions[0], true
}

// InstructionsCopy copies the instructions.
func (e *LogicExecutor) InstructionsCopy() []Instruction {
	result := make([]Instruction, len(e.Instructions))
	copy(result, e.Instructions)
	return result
}

// InstructionsClone clones the instructions.
func (e *LogicExecutor) InstructionsClone() []Instruction {
	return e.InstructionsCopy()
}

// InstructionsConcat concatenates instructions.
func (e *LogicExecutor) InstructionsConcat(instructions ...Instruction) {
	e.Instructions = append(e.Instructions, instructions...)
}

// InstructionsPrepend prepends instructions.
func (e *LogicExecutor) InstructionsPrepend(instructions ...Instruction) {
	e.Instructions = append(instructions, e.Instructions...)
}

// InstructionsInsert inserts an instruction at a position.
func (e *LogicExecutor) InstructionsInsert(pos int, inst Instruction) bool {
	return e.InsertInstruction(pos, inst)
}

// InstructionsRemove removes an instruction at a position.
func (e *LogicExecutor) InstructionsRemove(pos int) bool {
	return e.RemoveInstruction(pos)
}

// InstructionsReplace replaces an instruction at a position.
func (e *LogicExecutor) InstructionsReplace(pos int, inst Instruction) bool {
	return e.ReplaceInstruction(pos, inst)
}

// InstructionsSwap swaps two instructions.
func (e *LogicExecutor) InstructionsSwap(i, j int) bool {
	return e.SwapInstructions(i, j)
}

// InstructionsFilter filters instructions by a predicate.
func (e *LogicExecutor) InstructionsFilter(pred func(Instruction) bool) {
	e.FilterInstructions(pred)
}

// InstructionsMap maps instructions by a function.
func (e *LogicExecutor) InstructionsMap(fn func(Instruction) Instruction) {
	e.MapInstructions(fn)
}

// InstructionsReduce reduces instructions by a function.
func (e *LogicExecutor) InstructionsReduce(initial Instruction, fn func(Instruction, Instruction) Instruction) Instruction {
	return e.ReduceInstructions(initial, fn)
}

// InstructionsAllMatch checks if all instructions match a predicate.
func (e *LogicExecutor) InstructionsAllMatch(pred func(Instruction) bool) bool {
	return e.AllInstructionsMatch(pred)
}

// InstructionsAnyMatch checks if any instruction matches a predicate.
func (e *LogicExecutor) InstructionsAnyMatch(pred func(Instruction) bool) bool {
	return e.AnyInstructionMatches(pred)
}

// InstructionsFind finds an instruction matching a predicate.
func (e *LogicExecutor) InstructionsFind(pred func(Instruction) bool) (Instruction, bool) {
	return e.FindInstruction(pred)
}

// InstructionsFindLast finds the last instruction matching a predicate.
func (e *LogicExecutor) InstructionsFindLast(pred func(Instruction) bool) (Instruction, bool) {
	return e.FindLastInstruction(pred)
}

// InstructionsCountByOpcode counts instructions by opcode.
func (e *LogicExecutor) InstructionsCountByOpcode() map[Opcode]int {
	return e.CountInstructionsByOpcode()
}

// InstructionsReverse reverses the instructions.
func (e *LogicExecutor) InstructionsReverse() {
	for i, j := 0, len(e.Instructions)-1; i < j; i, j = i+1, j-1 {
		e.Instructions[i], e.Instructions[j] = e.Instructions[j], e.Instructions[i]
	}
}

// InstructionsSort sorts instructions by opcode.
func (e *LogicExecutor) InstructionsSort() {
	for i := 0; i < len(e.Instructions); i++ {
		for j := i + 1; j < len(e.Instructions); j++ {
			if e.Instructions[i].Opcode > e.Instructions[j].Opcode {
				e.Instructions[i], e.Instructions[j] = e.Instructions[j], e.Instructions[i]
			}
		}
	}
}

// InstructionsCopyInstructions copies instructions from another executor.
func (e *LogicExecutor) InstructionsCopyInstructions(source *LogicExecutor) {
	e.Instructions = make([]Instruction, len(source.Instructions))
	copy(e.Instructions, source.Instructions)
}

// InstructionsMerge merges instructions from another executor.
func (e *LogicExecutor) InstructionsMerge(source *LogicExecutor) {
	e.Instructions = append(e.Instructions, source.Instructions...)
}

// InstructionsToJSON returns instructions as JSON.
func (e *LogicExecutor) InstructionsToJSON() []map[string]interface{} {
	result := make([]map[string]interface{}, len(e.Instructions))
	for i, inst := range e.Instructions {
		result[i] = map[string]interface{}{
			"opcode": inst.Opcode.String(),
			"args":   inst.Args,
		}
	}
	return result
}

// InstructionsFromJSON loads instructions from JSON.
func (e *LogicExecutor) InstructionsFromJSON(data []map[string]interface{}) error {
	for _, item := range data {
		opcodeStr := ""
		if v, ok := item["opcode"].(string); ok {
			opcodeStr = v
		}
		args := make([]interface{}, 0)
		if v, ok := item["args"].([]interface{}); ok {
			args = v
		}
		var argsInt32 []int32
		for _, arg := range args {
			if v, ok := arg.(float64); ok {
				argsInt32 = append(argsInt32, int32(v))
			}
		}
		e.AddInstruction(Instruction{
			Opcode: stringToOpcode(opcodeStr),
			Args:   argsInt32,
		})
	}
	return nil
}

// stringToOpcode converts a string to an opcode.
func stringToOpcode(name string) Opcode {
	opcodes := map[string]Opcode{
		"noop":   NoOp,
		"mov":    MOV,
		"add":    ADD,
		"sub":    SUB,
		"mul":    MUL,
		"div":    DIV,
		"mod":    MOD,
		"pow":    POW,
		"and":    AND,
		"or":     OR,
		"xor":    XOR,
		"not":    NOT,
		"cmp":    CMP,
		"jmp":    JMP,
		"jz":     JZ,
		"jnz":    JNZ,
		"call":   CALL,
		"ret":    RET,
		"halt":   HALT,
		"load":   LOAD,
		"store":  STORE,
		"push":   PUSH,
		"pop":    POP,
		"input":  INPUT,
		"output": OUTPUT,
		"print":  PRINT,
	}
	if op, ok := opcodes[name]; ok {
		return op
	}
	return NoOp
}

// InstructionsToMap returns instructions as a map.
func (e *LogicExecutor) InstructionsToMap() map[int]Instruction {
	result := make(map[int]Instruction)
	for i, inst := range e.Instructions {
		result[i] = inst
	}
	return result
}

// InstructionsFromMap loads instructions from a map.
func (e *LogicExecutor) InstructionsFromMap(data map[int]Instruction) {
	e.Instructions = make([]Instruction, len(data))
	for i, inst := range data {
		e.Instructions[i] = inst
	}
}

// InstructionsGetRange returns instructions in a range.
func (e *LogicExecutor) InstructionsGetRange(start, end int) []Instruction {
	if start < 0 {
		start = 0
	}
	if end > len(e.Instructions) {
		end = len(e.Instructions)
	}
	if start >= end {
		return nil
	}
	return e.Instructions[start:end]
}

// InstructionsSetRange sets instructions in a range.
func (e *LogicExecutor) InstructionsSetRange(start, end int, instructions []Instruction) bool {
	if start < 0 || start > len(e.Instructions) || end < start || end > len(e.Instructions) {
		return false
	}
	e.Instructions = append(e.Instructions[:start], append(instructions, e.Instructions[end:]...)...)
	return true
}

// InstructionsReplaceRange replaces instructions in a range.
func (e *LogicExecutor) InstructionsReplaceRange(start, end int, instructions []Instruction) bool {
	return e.InstructionsSetRange(start, end, instructions)
}

// InstructionsInsertRange inserts instructions at a position.
func (e *LogicExecutor) InstructionsInsertRange(pos int, instructions []Instruction) bool {
	if pos < 0 || pos > len(e.Instructions) {
		return false
	}
	e.Instructions = append(e.Instructions[:pos], append(instructions, e.Instructions[pos:]...)...)
	return true
}

// InstructionsRemoveRange removes instructions in a range.
func (e *LogicExecutor) InstructionsRemoveRange(start, end int) bool {
	if start < 0 || start > len(e.Instructions) || end < start || end > len(e.Instructions) {
		return false
	}
	e.Instructions = append(e.Instructions[:start], e.Instructions[end:]...)
	return true
}

// InstructionsSlice returns a slice of instructions.
func (e *LogicExecutor) InstructionsSlice(start, end int) []Instruction {
	return e.InstructionsGetRange(start, end)
}

// InstructionsLength returns the length of instructions.
func (e *LogicExecutor) InstructionsLength() int {
	return len(e.Instructions)
}

// InstructionsEmpty checks if instructions are empty.
func (e *LogicExecutor) InstructionsEmpty() bool {
	return len(e.Instructions) == 0
}

// InstructionsNotEmpty checks if instructions are not empty.
func (e *LogicExecutor) InstructionsNotEmpty() bool {
	return len(e.Instructions) > 0
}

// InstructionsCount returns the count of instructions.
func (e *LogicExecutor) InstructionsCount() int {
	return len(e.Instructions)
}
