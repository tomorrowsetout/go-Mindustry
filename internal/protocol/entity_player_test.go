package protocol

import (
	"bytes"
	"testing"
)

func TestPlayerEntityWriteSyncMatchesVanillaFieldOrder(t *testing.T) {
	player := &PlayerEntity{
		IDValue:          1,
		Admin:            true,
		Boosting:         false,
		ColorRGBA:        0x11223344,
		MouseX:           12.5,
		MouseY:           -3.25,
		Name:             "tester",
		SelectedBlock:    303,
		SelectedRotation: 2,
		Shooting:         true,
		TeamID:           1,
		Typing:           false,
		Unit:             UnitBox{IDValue: 42},
		X:                64,
		Y:                96,
	}

	got := NewWriter()
	if err := player.WriteSync(got); err != nil {
		t.Fatalf("WriteSync failed: %v", err)
	}

	want := NewWriter()
	if err := want.WriteBool(player.Admin); err != nil {
		t.Fatalf("write admin failed: %v", err)
	}
	if err := want.WriteBool(player.Boosting); err != nil {
		t.Fatalf("write boosting failed: %v", err)
	}
	if err := WriteColor(want, Color{RGBA: player.ColorRGBA}); err != nil {
		t.Fatalf("write color failed: %v", err)
	}
	if err := want.WriteFloat32(player.MouseX); err != nil {
		t.Fatalf("write mouseX failed: %v", err)
	}
	if err := want.WriteFloat32(player.MouseY); err != nil {
		t.Fatalf("write mouseY failed: %v", err)
	}
	name := player.Name
	if err := WriteString(want, &name); err != nil {
		t.Fatalf("write name failed: %v", err)
	}
	if err := want.WriteInt16(player.SelectedBlock); err != nil {
		t.Fatalf("write selectedBlock failed: %v", err)
	}
	if err := want.WriteInt32(player.SelectedRotation); err != nil {
		t.Fatalf("write selectedRotation failed: %v", err)
	}
	if err := want.WriteBool(player.Shooting); err != nil {
		t.Fatalf("write shooting failed: %v", err)
	}
	if err := WriteTeam(want, &Team{ID: player.TeamID}); err != nil {
		t.Fatalf("write team failed: %v", err)
	}
	if err := want.WriteBool(player.Typing); err != nil {
		t.Fatalf("write typing failed: %v", err)
	}
	if err := WriteUnit(want, player.Unit); err != nil {
		t.Fatalf("write unit failed: %v", err)
	}
	if err := want.WriteFloat32(player.X); err != nil {
		t.Fatalf("write x failed: %v", err)
	}
	if err := want.WriteFloat32(player.Y); err != nil {
		t.Fatalf("write y failed: %v", err)
	}

	if !bytes.Equal(got.Bytes(), want.Bytes()) {
		t.Fatalf("player sync bytes mismatch:\n got=%x\nwant=%x", got.Bytes(), want.Bytes())
	}
}

func TestPlayerEntityReadSyncMatchesVanillaFieldOrder(t *testing.T) {
	w := NewWriter()
	if err := w.WriteBool(true); err != nil {
		t.Fatalf("write admin failed: %v", err)
	}
	if err := w.WriteBool(true); err != nil {
		t.Fatalf("write boosting failed: %v", err)
	}
	if err := WriteColor(w, Color{RGBA: 0x55667788}); err != nil {
		t.Fatalf("write color failed: %v", err)
	}
	if err := w.WriteFloat32(10); err != nil {
		t.Fatalf("write mouseX failed: %v", err)
	}
	if err := w.WriteFloat32(20); err != nil {
		t.Fatalf("write mouseY failed: %v", err)
	}
	name := "player"
	if err := WriteString(w, &name); err != nil {
		t.Fatalf("write name failed: %v", err)
	}
	if err := w.WriteInt16(302); err != nil {
		t.Fatalf("write selectedBlock failed: %v", err)
	}
	if err := w.WriteInt32(3); err != nil {
		t.Fatalf("write selectedRotation failed: %v", err)
	}
	if err := w.WriteBool(false); err != nil {
		t.Fatalf("write shooting failed: %v", err)
	}
	if err := WriteTeam(w, &Team{ID: 2}); err != nil {
		t.Fatalf("write team failed: %v", err)
	}
	if err := w.WriteBool(true); err != nil {
		t.Fatalf("write typing failed: %v", err)
	}
	if err := WriteUnit(w, UnitBox{IDValue: 77}); err != nil {
		t.Fatalf("write unit failed: %v", err)
	}
	if err := w.WriteFloat32(128); err != nil {
		t.Fatalf("write x failed: %v", err)
	}
	if err := w.WriteFloat32(256); err != nil {
		t.Fatalf("write y failed: %v", err)
	}

	r := NewReader(w.Bytes())
	var player PlayerEntity
	if err := player.ReadSync(r); err != nil {
		t.Fatalf("ReadSync failed: %v", err)
	}
	if r.Remaining() != 0 {
		t.Fatalf("expected player sync bytes to be fully consumed, remaining=%d", r.Remaining())
	}
	if !player.Admin || !player.Boosting {
		t.Fatalf("expected admin+boosting true, got admin=%v boosting=%v", player.Admin, player.Boosting)
	}
	if player.ColorRGBA != 0x55667788 {
		t.Fatalf("expected color 0x55667788, got 0x%x", player.ColorRGBA)
	}
	if player.MouseX != 10 || player.MouseY != 20 {
		t.Fatalf("expected mouse=(10,20), got (%v,%v)", player.MouseX, player.MouseY)
	}
	if player.Name != "player" {
		t.Fatalf("expected name 'player', got %q", player.Name)
	}
	if player.SelectedBlock != 302 || player.SelectedRotation != 3 {
		t.Fatalf("expected selection=(302,3), got (%d,%d)", player.SelectedBlock, player.SelectedRotation)
	}
	if player.Shooting {
		t.Fatalf("expected shooting false")
	}
	if player.TeamID != 2 {
		t.Fatalf("expected team 2, got %d", player.TeamID)
	}
	if !player.Typing {
		t.Fatalf("expected typing true")
	}
	if player.Unit == nil || player.Unit.ID() != 77 {
		t.Fatalf("expected unit ref 77, got %#v", player.Unit)
	}
	if player.X != 128 || player.Y != 256 {
		t.Fatalf("expected position=(128,256), got (%v,%v)", player.X, player.Y)
	}
}
