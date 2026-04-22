package protocol

import "testing"

func TestReadClientPlansHandlesUnknownRotatingBlock(t *testing.T) {
	w := NewWriter()
	if err := w.WriteInt16(1); err != nil {
		t.Fatalf("write amount: %v", err)
	}
	if err := w.WriteUint16(17); err != nil {
		t.Fatalf("write x: %v", err)
	}
	if err := w.WriteUint16(23); err != nil {
		t.Fatalf("write y: %v", err)
	}
	if err := w.WriteInt16(12); err != nil {
		t.Fatalf("write block id: %v", err)
	}
	if err := w.WriteByte(3); err != nil {
		t.Fatalf("write rotation: %v", err)
	}
	if err := w.WriteByte(0); err != nil {
		t.Fatalf("write nil config: %v", err)
	}

	r := NewReaderWithContext(w.Bytes(), &TypeIOContext{
		BlockLookup: func(id int16) Block { return nil },
	})
	plans, err := ReadClientPlans(r, r.Ctx)
	if err != nil {
		t.Fatalf("ReadClientPlans failed: %v", err)
	}
	if len(plans) != 1 || plans[0] == nil {
		t.Fatalf("expected one plan, got %#v", plans)
	}
	if plans[0].X != 17 || plans[0].Y != 23 {
		t.Fatalf("unexpected plan position: (%d,%d)", plans[0].X, plans[0].Y)
	}
	if plans[0].Rotation != 3 {
		t.Fatalf("expected rotation 3, got %d", plans[0].Rotation)
	}
	if plans[0].Block == nil || plans[0].Block.ID() != 12 {
		t.Fatalf("expected fallback block id 12, got %#v", plans[0].Block)
	}
	if plans[0].Config != nil {
		t.Fatalf("expected nil config, got %#v", plans[0].Config)
	}
}
