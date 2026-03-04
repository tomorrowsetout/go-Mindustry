// Package logic provides a Mindustry logic processor implementation
// for the mdt-server project.
//
// This package implements a lexer, parser, and virtual machine
// for executing Mindustry logic programs.
package logic

import (
	"fmt"
)

// Opcode represents an operation code for the virtual machine.
type Opcode int

const (
	// NoOp represents no operation.
	NoOp Opcode = iota
	// MOV moves a value to a register or memory location.
	MOV
	// ADD adds two values.
	ADD
	// SUB subtracts two values.
	SUB
	// MUL multiplies two values.
	MUL
	// DIV divides two values.
	DIV
	// MOD computes modulo.
	MOD
	// POW raises to a power.
	POW
	// AND performs bitwise AND.
	AND
	// OR performs bitwise OR.
	OR
	// XOR performs bitwise XOR.
	XOR
	// NOT performs bitwise NOT.
	NOT
	// CMP compares two values.
	CMP
	// JMP jumps to a label.
	JMP
	// JZ jumps if zero.
	JZ
	// JNZ jumps if not zero.
	JNZ
	// CALL calls a function.
	CALL
	// RET returns from a function.
	RET
	// HALT stops execution.
	HALT
	// LOAD loads a value from memory.
	LOAD
	// STORE stores a value to memory.
	STORE
	// PUSH pushes a value onto the stack.
	PUSH
	// POP pops a value from the stack.
	POP
	// INPUT reads an input value.
	INPUT
	// OUTPUT outputs a value.
	OUTPUT
	// PRINT prints a value.
	PRINT
)

// instructionNames maps opcodes to their names.
var instructionNames = map[Opcode]string{
	NoOp:   "noop",
	MOV:    "mov",
	ADD:    "add",
	SUB:    "sub",
	MUL:    "mul",
	DIV:    "div",
	MOD:    "mod",
	POW:    "pow",
	AND:    "and",
	OR:     "or",
	XOR:    "xor",
	NOT:    "not",
	CMP:    "cmp",
	JMP:    "jmp",
	JZ:     "jz",
	JNZ:    "jnz",
	CALL:   "call",
	RET:    "ret",
	HALT:   "halt",
	LOAD:   "load",
	STORE:  "store",
	PUSH:   "push",
	POP:    "pop",
	INPUT:  "input",
	OUTPUT: "output",
	PRINT:  "print",
}

// String returns the string representation of an opcode.
func (op Opcode) String() string {
	if name, ok := instructionNames[op]; ok {
		return name
	}
	return fmt.Sprintf("opcode_%d", op)
}

// Instruction represents a single VM instruction.
type Instruction struct {
	Opcode Opcode   // The operation code
	Args   []int32  // Operation arguments (register indices or values)
}

// String returns a string representation of the instruction.
func (i Instruction) String() string {
	args := make([]string, len(i.Args))
	for j, arg := range i.Args {
		args[j] = fmt.Sprintf("%d", arg)
	}
	return fmt.Sprintf("%s %s", i.Opcode, args)
}

// VM represents a virtual machine for executing logic instructions.
type VM struct {
	Memory       map[int32]int32  // Memory store
	Registers    map[string]int32 // Named registers
	Stack        []int32          // Call stack
	PC           int              // Program counter
	Instructions []Instruction    // loaded instructions
	Inputs       map[string]int32 // External inputs
	Outputs      map[string]int32 // Output values
	Halted       bool             // Whether the VM has halted
	Error        error            // Last error
}

// NewVM creates a new virtual machine.
func NewVM() *VM {
	return &VM{
		Memory:       make(map[int32]int32),
		Registers:    make(map[string]int32),
		Stack:        make([]int32, 0),
		PC:           0,
		Instructions: make([]Instruction, 0),
		Inputs:       make(map[string]int32),
		Outputs:      make(map[string]int32),
		Halted:       false,
	}
}

// LoadInstructions loads instructions into the VM.
func (vm *VM) LoadInstructions(instrs []Instruction) {
	vm.Instructions = make([]Instruction, len(instrs))
	copy(vm.Instructions, instrs)
	vm.PC = 0
	vm.Halted = false
	vm.Error = nil
}

// Run executes all loaded instructions.
func (vm *VM) Run() error {
	vm.Halted = false
	vm.Error = nil

	for !vm.Halted && vm.PC < len(vm.Instructions) {
		if err := vm.Step(); err != nil {
			vm.Error = err
			return err
		}
	}

	return vm.Error
}

// Step executes a single instruction.
func (vm *VM) Step() error {
	if vm.PC < 0 || vm.PC >= len(vm.Instructions) {
		return fmt.Errorf("program counter out of bounds: %d", vm.PC)
	}

	if vm.Halted {
		return nil
	}

	inst := vm.Instructions[vm.PC]
	vm.PC++

	switch inst.Opcode {
	case NoOp:
		// Do nothing

	case MOV:
		if len(inst.Args) >= 2 {
			reg := vm.getRegisterName(inst.Args[0])
			val := vm.getValue(inst.Args[1])
			vm.SetRegister(reg, val)
		}

	case ADD:
		if len(inst.Args) >= 3 {
			reg := vm.getRegisterName(inst.Args[0])
			val1 := vm.getValue(inst.Args[1])
			val2 := vm.getValue(inst.Args[2])
			vm.SetRegister(reg, val1+val2)
		}

	case SUB:
		if len(inst.Args) >= 3 {
			reg := vm.getRegisterName(inst.Args[0])
			val1 := vm.getValue(inst.Args[1])
			val2 := vm.getValue(inst.Args[2])
			vm.SetRegister(reg, val1-val2)
		}

	case MUL:
		if len(inst.Args) >= 3 {
			reg := vm.getRegisterName(inst.Args[0])
			val1 := vm.getValue(inst.Args[1])
			val2 := vm.getValue(inst.Args[2])
			vm.SetRegister(reg, val1*val2)
		}

	case DIV:
		if len(inst.Args) >= 3 {
			reg := vm.getRegisterName(inst.Args[0])
			val1 := vm.getValue(inst.Args[1])
			val2 := vm.getValue(inst.Args[2])
			if val2 == 0 {
				return fmt.Errorf("division by zero")
			}
			vm.SetRegister(reg, val1/val2)
		}

	case MOD:
		if len(inst.Args) >= 3 {
			reg := vm.getRegisterName(inst.Args[0])
			val1 := vm.getValue(inst.Args[1])
			val2 := vm.getValue(inst.Args[2])
			if val2 == 0 {
				return fmt.Errorf("modulo by zero")
			}
			vm.SetRegister(reg, val1%val2)
		}

	case CMP:
		if len(inst.Args) >= 2 {
			val1 := vm.getValue(inst.Args[0])
			val2 := vm.getValue(inst.Args[1])
			vm.SetRegister("_cmp", cmp(val1, val2))
		}

	case JMP:
		if len(inst.Args) >= 1 {
			vm.PC = int(vm.getValue(inst.Args[0]))
		}

	case JZ:
		if len(inst.Args) >= 2 {
			val := vm.getValue(inst.Args[0])
			if val == 0 {
				vm.PC = int(vm.getValue(inst.Args[1]))
			}
		}

	case JNZ:
		if len(inst.Args) >= 2 {
			val := vm.getValue(inst.Args[0])
			if val != 0 {
				vm.PC = int(vm.getValue(inst.Args[1]))
			}
		}

	case LOAD:
		if len(inst.Args) >= 2 {
			reg := vm.getRegisterName(inst.Args[0])
			addr := vm.getValue(inst.Args[1])
			vm.SetRegister(reg, vm.GetMemory(addr))
		}

	case STORE:
		if len(inst.Args) >= 2 {
			addr := vm.getValue(inst.Args[0])
			reg := vm.getRegisterName(inst.Args[1])
			vm.SetMemory(addr, vm.GetRegister(reg))
		}

	case PUSH:
		if len(inst.Args) >= 1 {
			val := vm.getValue(inst.Args[0])
			vm.Stack = append(vm.Stack, val)
		}

	case POP:
		if len(inst.Args) >= 1 {
			if len(vm.Stack) > 0 {
				reg := vm.getRegisterName(inst.Args[0])
				vm.Stack = vm.Stack[:len(vm.Stack)-1]
				vm.Registers[reg] = vm.Stack[len(vm.Stack)]
			}
		}

	case CALL:
		if len(inst.Args) >= 1 {
			vm.Stack = append(vm.Stack, int32(vm.PC))
			vm.PC = int(vm.getValue(inst.Args[0]))
		}

	case RET:
		if len(vm.Stack) > 0 {
			vm.PC = int(vm.Stack[len(vm.Stack)-1])
			vm.Stack = vm.Stack[:len(vm.Stack)-1]
		}

	case HALT:
		vm.Halted = true

	case INPUT:
		if len(inst.Args) >= 2 {
			key := vm.getRegisterName(inst.Args[0])
			if val, ok := vm.Inputs[key]; ok {
				vm.SetRegister(key, val)
			}
		}

	case OUTPUT:
		if len(inst.Args) >= 2 {
			key := vm.getRegisterName(inst.Args[0])
			val := vm.getValue(inst.Args[1])
			vm.Outputs[key] = val
		}

	case PRINT:
		if len(inst.Args) >= 1 {
			val := vm.getValue(inst.Args[0])
			fmt.Printf("%d\n", val)
		}

	default:
		return fmt.Errorf("unknown opcode: %d", inst.Opcode)
	}

	return nil
}

// getValue returns the value for an argument (register or immediate).
func (vm *VM) getValue(arg int32) int32 {
	// If negative, it's a register index; otherwise it's an immediate value
	if arg < 0 {
		// Convert to register name
		regName := fmt.Sprintf("r%d", -arg)
		return vm.GetRegister(regName)
	}
	return arg
}

// getRegisterName returns the register name for an index.
func (vm *VM) getRegisterName(index int32) string {
	return fmt.Sprintf("r%d", index)
}

// cmp compares two values and returns -1, 0, or 1.
func cmp(a, b int32) int32 {
	if a < b {
		return -1
	} else if a > b {
		return 1
	}
	return 0
}

// GetRegister returns the value of a register.
func (vm *VM) GetRegister(name string) int32 {
	if val, ok := vm.Registers[name]; ok {
		return val
	}
	return 0
}

// SetRegister sets the value of a register.
func (vm *VM) SetRegister(name string, value int32) {
	vm.Registers[name] = value
}

// GetMemory returns the value at a memory address.
func (vm *VM) GetMemory(addr int32) int32 {
	if val, ok := vm.Memory[addr]; ok {
		return val
	}
	return 0
}

// SetMemory sets the value at a memory address.
func (vm *VM) SetMemory(addr int32, value int32) {
	vm.Memory[addr] = value
}

// GetPC returns the current program counter.
func (vm *VM) GetPC() int {
	return vm.PC
}

// SetPC sets the program counter.
func (vm *VM) SetPC(pc int) {
	vm.PC = pc
}

// GetRegisters returns all registers.
func (vm *VM) GetRegisters() map[string]int32 {
	return vm.Registers
}

// GetMemoryMap returns a copy of the memory map.
func (vm *VM) GetMemoryMap() map[int32]int32 {
	result := make(map[int32]int32)
	for k, v := range vm.Memory {
		result[k] = v
	}
	return result
}

// GetStack returns the current stack.
func (vm *VM) GetStack() []int32 {
	return vm.Stack
}

// Clear resets the VM.
func (vm *VM) Clear() {
	vm.Memory = make(map[int32]int32)
	vm.Registers = make(map[string]int32)
	vm.Stack = make([]int32, 0)
	vm.PC = 0
	vm.Halted = false
	vm.Error = nil
}

// SetInput sets an input value.
func (vm *VM) SetInput(key string, value int32) {
	vm.Inputs[key] = value
}

// GetOutput returns an output value.
func (vm *VM) GetOutput(key string) int32 {
	if val, ok := vm.Outputs[key]; ok {
		return val
	}
	return 0
}

// GetOutputs returns all outputs.
func (vm *VM) GetOutputs() map[string]int32 {
	return vm.Outputs
}

// IsHalted returns whether the VM has halted.
func (vm *VM) IsHalted() bool {
	return vm.Halted
}

// GetLastError returns the last error.
func (vm *VM) GetLastError() error {
	return vm.Error
}

// GetInstructions returns the loaded instructions.
func (vm *VM) GetInstructions() []Instruction {
	return vm.Instructions
}

// SetInstructions sets the instructions.
func (vm *VM) SetInstructions(instrs []Instruction) {
	vm.LoadInstructions(instrs)
}

// Debug returns a debug string.
func (vm *VM) Debug() string {
	return fmt.Sprintf("VM{PC=%d, Halted=%v}", vm.PC, vm.Halted)
}

// CountInstructions returns the number of loaded instructions.
func (vm *VM) CountInstructions() int {
	return len(vm.Instructions)
}

// CountInstructionsByOpcode returns a count of each opcode.
func (vm *VM) CountInstructionsByOpcode() map[Opcode]int {
	counts := make(map[Opcode]int)
	for _, inst := range vm.Instructions {
		counts[inst.Opcode]++
	}
	return counts
}

// FindLabels finds label positions in instructions.
func (vm *VM) FindLabels() map[string]int {
	labels := make(map[string]int)
	for i, inst := range vm.Instructions {
		if inst.Opcode == JMP && len(inst.Args) > 0 {
			// Assuming labels are encoded in args for now
			labels[fmt.Sprintf("label_%d", i)] = i
		}
	}
	return labels
}

// GetInstructionAt returns the instruction at a position.
func (vm *VM) GetInstructionAt(pos int) (Instruction, bool) {
	if pos < 0 || pos >= len(vm.Instructions) {
		return Instruction{}, false
	}
	return vm.Instructions[pos], true
}

// SetInstructionAt sets the instruction at a position.
func (vm *VM) SetInstructionAt(pos int, inst Instruction) bool {
	if pos < 0 || pos >= len(vm.Instructions) {
		return false
	}
	vm.Instructions[pos] = inst
	return true
}

// InsertInstruction inserts an instruction at a position.
func (vm *VM) InsertInstruction(pos int, inst Instruction) bool {
	if pos < 0 || pos > len(vm.Instructions) {
		return false
	}
	vm.Instructions = append(vm.Instructions[:pos], append([]Instruction{inst}, vm.Instructions[pos:]...)...)
	return true
}

// DeleteInstruction deletes an instruction at a position.
func (vm *VM) DeleteInstruction(pos int) bool {
	if pos < 0 || pos >= len(vm.Instructions) {
		return false
	}
	vm.Instructions = append(vm.Instructions[:pos], vm.Instructions[pos+1:]...)
	return true
}

// ReplaceInstruction replaces an instruction at a position.
func (vm *VM) ReplaceInstruction(pos int, inst Instruction) bool {
	if pos < 0 || pos >= len(vm.Instructions) {
		return false
	}
	vm.Instructions[pos] = inst
	return true
}

// Clone returns a copy of the VM.
func (vm *VM) Clone() *VM {
	newVM := NewVM()
	newVM.Instructions = make([]Instruction, len(vm.Instructions))
	copy(newVM.Instructions, vm.Instructions)
	newVM.PC = vm.PC
	newVM.Halted = vm.Halted
	return newVM
}

// Reset resets the VM to initial state but keeps instructions.
func (vm *VM) Reset() {
	vm.PC = 0
	vm.Halted = false
	vm.Error = nil
	vm.Registers = make(map[string]int32)
	vm.Stack = make([]int32, 0)
	vm.Outputs = make(map[string]int32)
}

// GetRegisterNames returns all register names.
func (vm *VM) GetRegisterNames() []string {
	names := make([]string, 0, len(vm.Registers))
	for name := range vm.Registers {
		names = append(names, name)
	}
	return names
}

// GetMemoryAddresses returns all memory addresses.
func (vm *VM) GetMemoryAddresses() []int32 {
	addresses := make([]int32, 0, len(vm.Memory))
	for addr := range vm.Memory {
		addresses = append(addresses, addr)
	}
	return addresses
}

// GetMemoryValues returns all memory values.
func (vm *VM) GetMemoryValues() []int32 {
	values := make([]int32, 0, len(vm.Memory))
	for _, val := range vm.Memory {
		values = append(values, val)
	}
	return values
}

// GetInputKeys returns all input keys.
func (vm *VM) GetInputKeys() []string {
	keys := make([]string, 0, len(vm.Inputs))
	for key := range vm.Inputs {
		keys = append(keys, key)
	}
	return keys
}

// GetOutputKeys returns all output keys.
func (vm *VM) GetOutputKeys() []string {
	keys := make([]string, 0, len(vm.Outputs))
	for key := range vm.Outputs {
		keys = append(keys, key)
	}
	return keys
}

// GetInput returns an input value.
func (vm *VM) GetInput(key string) int32 {
	if val, ok := vm.Inputs[key]; ok {
		return val
	}
	return 0
}

// SetRegisterInt32 sets a register value.
func (vm *VM) SetRegisterInt32(name string, value int32) {
	vm.Registers[name] = value
}

// SetRegisterInt sets a register value from int.
func (vm *VM) SetRegisterInt(name string, value int) {
	vm.Registers[name] = int32(value)
}

// GetRegisterInt32 returns a register value as int32.
func (vm *VM) GetRegisterInt32(name string) int32 {
	if val, ok := vm.Registers[name]; ok {
		return val
	}
	return 0
}

// GetRegisterInt returns a register value as int.
func (vm *VM) GetRegisterInt(name string) int {
	if val, ok := vm.Registers[name]; ok {
		return int(val)
	}
	return 0
}

// StackEmpty checks if the stack is empty.
func (vm *VM) StackEmpty() bool {
	return len(vm.Stack) == 0
}

// StackSize returns the stack size.
func (vm *VM) StackSize() int {
	return len(vm.Stack)
}

// StackPeek returns the top of the stack.
func (vm *VM) StackPeek() (int32, bool) {
	if len(vm.Stack) == 0 {
		return 0, false
	}
	return vm.Stack[len(vm.Stack)-1], true
}

// StackPush pushes a value onto the stack.
func (vm *VM) StackPush(val int32) {
	vm.Stack = append(vm.Stack, val)
}

// StackPop pops a value from the stack.
func (vm *VM) StackPop() (int32, bool) {
	if len(vm.Stack) == 0 {
		return 0, false
	}
	val := vm.Stack[len(vm.Stack)-1]
	vm.Stack = vm.Stack[:len(vm.Stack)-1]
	return val, true
}

// StackClear clears the stack.
func (vm *VM) StackClear() {
	vm.Stack = make([]int32, 0)
}

// StackReverse reverses the stack.
func (vm *VM) StackReverse() {
	for i, j := 0, len(vm.Stack)-1; i < j; i, j = i+1, j-1 {
		vm.Stack[i], vm.Stack[j] = vm.Stack[j], vm.Stack[i]
	}
}

// StackSlice returns a slice of the stack.
func (vm *VM) StackSlice(start, end int) []int32 {
	if start < 0 {
		start = 0
	}
	if end > len(vm.Stack) {
		end = len(vm.Stack)
	}
	if start >= end {
		return nil
	}
	return vm.Stack[start:end]
}

// StackContains checks if a value is in the stack.
func (vm *VM) StackContains(val int32) bool {
	for _, v := range vm.Stack {
		if v == val {
			return true
		}
	}
	return false
}

// StackCount counts occurrences of a value in the stack.
func (vm *VM) StackCount(val int32) int {
	count := 0
	for _, v := range vm.Stack {
		if v == val {
			count++
		}
	}
	return count
}

// StackIndex returns the index of a value in the stack.
func (vm *VM) StackIndex(val int32) int {
	for i, v := range vm.Stack {
		if v == val {
			return i
		}
	}
	return -1
}

// StackLastIndex returns the last index of a value in the stack.
func (vm *VM) StackLastIndex(val int32) int {
	for i := len(vm.Stack) - 1; i >= 0; i-- {
		if vm.Stack[i] == val {
			return i
		}
	}
	return -1
}

// StackContainsAll checks if all values are in the stack.
func (vm *VM) StackContainsAll(vals ...int32) bool {
	for _, v := range vals {
		found := false
		for _, sv := range vm.Stack {
			if sv == v {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

// StackContainsAny checks if any value is in the stack.
func (vm *VM) StackContainsAny(vals ...int32) bool {
	for _, v := range vals {
		for _, sv := range vm.Stack {
			if sv == v {
				return true
			}
		}
	}
	return false
}

// StackFilter filters the stack by a predicate.
func (vm *VM) StackFilter(pred func(int32) bool) {
	result := make([]int32, 0, len(vm.Stack))
	for _, v := range vm.Stack {
		if pred(v) {
			result = append(result, v)
		}
	}
	vm.Stack = result
}

// StackMap maps the stack by a function.
func (vm *VM) StackMap(fn func(int32) int32) {
	for i, v := range vm.Stack {
		vm.Stack[i] = fn(v)
	}
}

// StackReduce reduces the stack by a function.
func (vm *VM) StackReduce(initial int32, fn func(int32, int32) int32) int32 {
	result := initial
	for _, v := range vm.Stack {
		result = fn(result, v)
	}
	return result
}

// StackAllMatch checks if all values match a predicate.
func (vm *VM) StackAllMatch(pred func(int32) bool) bool {
	for _, v := range vm.Stack {
		if !pred(v) {
			return false
		}
	}
	return true
}

// StackAnyMatch checks if any value matches a predicate.
func (vm *VM) StackAnyMatch(pred func(int32) bool) bool {
	for _, v := range vm.Stack {
		if pred(v) {
			return true
		}
	}
	return false
}

// StackFind finds a value matching a predicate.
func (vm *VM) StackFind(pred func(int32) bool) (int32, bool) {
	for _, v := range vm.Stack {
		if pred(v) {
			return v, true
		}
	}
	return 0, false
}

// StackFindIndex finds the index of a value matching a predicate.
func (vm *VM) StackFindIndex(pred func(int32) bool) int {
	for i, v := range vm.Stack {
		if pred(v) {
			return i
		}
	}
	return -1
}

// StackFindLast finds the last value matching a predicate.
func (vm *VM) StackFindLast(pred func(int32) bool) (int32, bool) {
	for i := len(vm.Stack) - 1; i >= 0; i-- {
		if pred(vm.Stack[i]) {
			return vm.Stack[i], true
		}
	}
	return 0, false
}

// StackFindLastIndex finds the last index of a value matching a predicate.
func (vm *VM) StackFindLastIndex(pred func(int32) bool) int {
	for i := len(vm.Stack) - 1; i >= 0; i-- {
		if pred(vm.Stack[i]) {
			return i
		}
	}
	return -1
}

// StackCountBy returns count of each value.
func (vm *VM) StackCountBy() map[int32]int {
	counts := make(map[int32]int)
	for _, v := range vm.Stack {
		counts[v]++
	}
	return counts
}

// StackUnique returns unique values in the stack.
func (vm *VM) StackUnique() []int32 {
	seen := make(map[int32]bool)
	result := make([]int32, 0)
	for _, v := range vm.Stack {
		if !seen[v] {
			seen[v] = true
			result = append(result, v)
		}
	}
	return result
}

// StackSort sorts the stack.
func (vm *VM) StackSort() {
	for i := 0; i < len(vm.Stack); i++ {
		for j := i + 1; j < len(vm.Stack); j++ {
			if vm.Stack[i] > vm.Stack[j] {
				vm.Stack[i], vm.Stack[j] = vm.Stack[j], vm.Stack[i]
			}
		}
	}
}

// StackReverseSort sorts the stack in reverse.
func (vm *VM) StackReverseSort() {
	for i := 0; i < len(vm.Stack); i++ {
		for j := i + 1; j < len(vm.Stack); j++ {
			if vm.Stack[i] < vm.Stack[j] {
				vm.Stack[i], vm.Stack[j] = vm.Stack[j], vm.Stack[i]
			}
		}
	}
}

// StackCopy copies the stack.
func (vm *VM) StackCopy() []int32 {
	result := make([]int32, len(vm.Stack))
	copy(result, vm.Stack)
	return result
}

// StackClone clones the stack.
func (vm *VM) StackClone() []int32 {
	return vm.StackCopy()
}

// StackConcat concatenates the stack with another.
func (vm *VM) StackConcat(vals ...int32) {
	vm.Stack = append(vm.Stack, vals...)
}

// StackPrepend prepends values to the stack.
func (vm *VM) StackPrepend(vals ...int32) {
	vm.Stack = append(vals, vm.Stack...)
}

// StackInsert inserts a value at a position.
func (vm *VM) StackInsert(pos int, val int32) bool {
	if pos < 0 || pos > len(vm.Stack) {
		return false
	}
	vm.Stack = append(vm.Stack[:pos], append([]int32{val}, vm.Stack[pos:]...)...)
	return true
}

// StackRemove removes a value at a position.
func (vm *VM) StackRemove(pos int) bool {
	if pos < 0 || pos >= len(vm.Stack) {
		return false
	}
	vm.Stack = append(vm.Stack[:pos], vm.Stack[pos+1:]...)
	return true
}

// StackReplace replaces a value at a position.
func (vm *VM) StackReplace(pos int, val int32) bool {
	if pos < 0 || pos >= len(vm.Stack) {
		return false
	}
	vm.Stack[pos] = val
	return true
}

// StackSwap swaps two values.
func (vm *VM) StackSwap(i, j int) bool {
	if i < 0 || j < 0 || i >= len(vm.Stack) || j >= len(vm.Stack) {
		return false
	}
	vm.Stack[i], vm.Stack[j] = vm.Stack[j], vm.Stack[i]
	return true
}
