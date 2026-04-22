package logic

import "testing"

func TestLogicExecutorJSONRoundTripRestoresState(t *testing.T) {
	exec := NewLogicExecutor()
	exec.SetInstructions([]Instruction{
		{Opcode: MOV, Args: []int32{1, 2}},
		{Opcode: ADD, Args: []int32{2, 1, 3}},
		{Opcode: HALT},
	})
	exec.VM.LoadInstructions(exec.Instructions)
	exec.VM.Registers["r1"] = 7
	exec.VM.Memory[3] = 9
	exec.VM.Stack = []int32{5, 6}
	exec.VM.Inputs["sensor"] = 4
	exec.inputs["sensor"] = 4
	exec.VM.Outputs["result"] = 11
	exec.outputs = []string{"result=11"}
	exec.VM.SetPC(2)
	exec.VM.Halted = true

	state := exec.ToJSON()

	restored := NewLogicExecutor()
	if err := restored.FromJSON(state); err != nil {
		t.Fatalf("FromJSON: %v", err)
	}

	if len(restored.Instructions) != len(exec.Instructions) {
		t.Fatalf("expected %d instructions, got %d", len(exec.Instructions), len(restored.Instructions))
	}
	if restored.VM.GetPC() != 2 {
		t.Fatalf("expected pc=2, got %d", restored.VM.GetPC())
	}
	if !restored.VM.IsHalted() {
		t.Fatal("expected halted state to round-trip")
	}
	if got := restored.VM.Registers["r1"]; got != 7 {
		t.Fatalf("expected register r1=7, got %d", got)
	}
	if got := restored.VM.Memory[3]; got != 9 {
		t.Fatalf("expected memory[3]=9, got %d", got)
	}
	if len(restored.VM.Stack) != 2 || restored.VM.Stack[0] != 5 || restored.VM.Stack[1] != 6 {
		t.Fatalf("unexpected stack after restore: %#v", restored.VM.Stack)
	}
	if got := restored.inputs["sensor"]; got != 4 {
		t.Fatalf("expected input sensor=4, got %d", got)
	}
	if got := restored.VM.Outputs["result"]; got != 11 {
		t.Fatalf("expected output result=11, got %d", got)
	}
	if len(restored.outputs) != 1 || restored.outputs[0] != "result=11" {
		t.Fatalf("unexpected output log after restore: %#v", restored.outputs)
	}
}

func TestVMCloneCopiesMutableState(t *testing.T) {
	vm := NewVM()
	vm.LoadInstructions([]Instruction{{Opcode: MOV, Args: []int32{1, 2}}})
	vm.Memory[1] = 10
	vm.Registers["r1"] = 20
	vm.Stack = []int32{30}
	vm.Inputs["in"] = 40
	vm.Outputs["out"] = 50
	vm.SetPC(1)
	vm.Halted = true

	cloned := vm.Clone()
	cloned.Memory[1] = 100
	cloned.Registers["r1"] = 200
	cloned.Stack[0] = 300
	cloned.Inputs["in"] = 400
	cloned.Outputs["out"] = 500

	if vm.Memory[1] != 10 || vm.Registers["r1"] != 20 || vm.Stack[0] != 30 || vm.Inputs["in"] != 40 || vm.Outputs["out"] != 50 {
		t.Fatal("expected clone mutations not to affect original VM state")
	}
	if cloned.GetPC() != 1 || !cloned.IsHalted() {
		t.Fatalf("expected clone to preserve pc/halted, got pc=%d halted=%v", cloned.GetPC(), cloned.IsHalted())
	}
}

func TestLogicExecutorResolvesLabelJumpTargets(t *testing.T) {
	exec := NewLogicExecutor()
	exec.generateInstructions(&Program{Statements: []Statement{
		&LabelStatement{Name: "loop"},
		&JumpStatement{Token: Token{Type: Jump}, Target: "loop"},
		&JumpStatement{Token: Token{Type: Jz}, Target: "loop"},
	}})

	if len(exec.Instructions) != 4 {
		t.Fatalf("expected label, jump, jz and halt instructions, got %d", len(exec.Instructions))
	}
	if exec.Instructions[1].Opcode != JMP || len(exec.Instructions[1].Args) != 1 || exec.Instructions[1].Args[0] != 0 {
		t.Fatalf("expected jump to resolve to label position 0, got %+v", exec.Instructions[1])
	}
	if exec.Instructions[2].Opcode != JZ || len(exec.Instructions[2].Args) != 2 || exec.Instructions[2].Args[1] != 0 {
		t.Fatalf("expected jz to resolve to label position 0, got %+v", exec.Instructions[2])
	}
}
