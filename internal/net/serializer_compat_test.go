package net

import (
	"testing"

	"mdt-server/internal/protocol"
)

func TestReadBeginBreakCompat(t *testing.T) {
	in := &protocol.Remote_Build_beginBreak_123{
		Unit: int32(7),
		Team: protocol.Team{ID: 1},
		X:    33,
		Y:    44,
	}
	w := protocol.NewWriter()
	if err := in.Write(w); err != nil {
		t.Fatalf("encode beginBreak payload: %v", err)
	}

	out, err := readBeginBreakCompat(w.Bytes(), nil)
	if err != nil {
		t.Fatalf("decode beginBreak payload: %v", err)
	}
	if out.X != in.X || out.Y != in.Y || out.Team.ID != in.Team.ID {
		t.Fatalf("decoded beginBreak mismatch: got (%d,%d,team=%d)", out.X, out.Y, out.Team.ID)
	}
}

func TestReadBeginPlaceCompat(t *testing.T) {
	in := &protocol.Remote_Build_beginPlace_124{
		Unit:        int32(9),
		Result:      nil,
		Team:        protocol.Team{ID: 2},
		X:           120,
		Y:           80,
		Rotation:    3,
		PlaceConfig: nil,
	}
	w := protocol.NewWriter()
	if err := in.Write(w); err != nil {
		t.Fatalf("encode beginPlace payload: %v", err)
	}

	out, err := readBeginPlaceCompat(w.Bytes(), nil)
	if err != nil {
		t.Fatalf("decode beginPlace payload: %v", err)
	}
	if out.X != in.X || out.Y != in.Y || out.Rotation != in.Rotation || out.Team.ID != in.Team.ID {
		t.Fatalf("decoded beginPlace mismatch: got (%d,%d,rot=%d,team=%d)", out.X, out.Y, out.Rotation, out.Team.ID)
	}
}
