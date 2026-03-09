package net

import (
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func TestPlayerUnitStackHelpers(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	c := &Conn{playerID: 1, unitID: 101}

	s.entities[c.unitID] = &protocol.UnitEntitySync{
		IDValue: c.unitID,
		Stack: protocol.ItemStack{
			Item:   protocol.ItemRef{ItmID: 0, ItmName: ""},
			Amount: 0,
		},
	}

	if !s.CanPlayerUnitCarry(c, 3) {
		t.Fatalf("expected empty stack can carry item 3")
	}
	if added := s.AddPlayerUnitItem(c, 3, 40); added != 30 {
		t.Fatalf("expected add clamped to 30, got=%d", added)
	}
	if s.CanPlayerUnitCarry(c, 4) {
		t.Fatalf("expected cannot carry different item while stack occupied")
	}
	itemID, amount, ok := s.ConsumePlayerUnitStack(c, 10)
	if !ok || itemID != 3 || amount != 10 {
		t.Fatalf("unexpected consume partial result: ok=%v item=%d amount=%d", ok, itemID, amount)
	}
	itemID, amount, ok = s.ConsumePlayerUnitStack(c, 0)
	if !ok || itemID != 3 || amount != 20 {
		t.Fatalf("unexpected consume all result: ok=%v item=%d amount=%d", ok, itemID, amount)
	}
	_, _, ok = s.ConsumePlayerUnitStack(c, 0)
	if ok {
		t.Fatalf("expected consume on empty stack to fail")
	}
}

func TestWithinItemTransferRange(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	c := &Conn{}

	// tile (10,10) => world center (84,84)
	pos := int32((10 << 16) | 10)

	c.snapX = 84
	c.snapY = 84
	if !s.withinItemTransferRange(c, pos) {
		t.Fatalf("expected in-range at tile center")
	}

	// 221 units away on X axis, should be out of range (official range=220).
	c.snapX = 84 + 221
	c.snapY = 84
	if s.withinItemTransferRange(c, pos) {
		t.Fatalf("expected out-of-range beyond 220 units")
	}
}

func TestUnitStackByIDHelpers(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	unitID := int32(202)
	s.entities[unitID] = &protocol.UnitEntitySync{
		IDValue: unitID,
		Stack: protocol.ItemStack{
			Item:   protocol.ItemRef{ItmID: 4, ItmName: ""},
			Amount: 12,
		},
	}

	if got := s.ConsumeUnitStackByID(unitID, 5, 8); got != 0 {
		t.Fatalf("expected mismatched item consume=0, got=%d", got)
	}
	if added := s.AddUnitItemByID(unitID, 4, 3); added != 3 {
		t.Fatalf("expected add same item=3, got=%d", added)
	}
	if got := s.ConsumeUnitStackByID(unitID, 4, 10); got != 10 {
		t.Fatalf("expected consume=10, got=%d", got)
	}
}

func TestAddUnitItemByIDRespectsCapacity(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	unitID := int32(303)
	s.entities[unitID] = &protocol.UnitEntitySync{
		IDValue: unitID,
		Stack: protocol.ItemStack{
			Item:   protocol.ItemRef{ItmID: 6, ItmName: ""},
			Amount: 28,
		},
		TypeID: 0, // unknown type => default capacity(30)
	}

	if added := s.AddUnitItemByID(unitID, 6, 10); added != 2 {
		t.Fatalf("expected add clamped to 2, got=%d", added)
	}
	if added := s.AddUnitItemByID(unitID, 6, 1); added != 0 {
		t.Fatalf("expected add=0 when full, got=%d", added)
	}
}

func TestIsConnControlledUnit(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	c := &Conn{unitID: 999}
	if !s.isConnControlledUnit(c, 999) {
		t.Fatalf("expected controlled unit match")
	}
	if s.isConnControlledUnit(c, 1000) {
		t.Fatalf("expected non-controlled unit mismatch")
	}
}

func TestAllowItemDepositWindowLimit(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	s.SetItemDepositCooldown(0.01) // 10ms => 20ms window
	c := &Conn{}

	if !s.allowItemDeposit(c) {
		t.Fatalf("first deposit should pass")
	}
	if !s.allowItemDeposit(c) {
		t.Fatalf("second deposit in window should pass")
	}
	if s.allowItemDeposit(c) {
		t.Fatalf("third deposit in same window should be blocked")
	}
	time.Sleep(25 * time.Millisecond)
	if !s.allowItemDeposit(c) {
		t.Fatalf("deposit after window should pass")
	}
}

func TestCanInteractBuildHook(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	c := &Conn{}
	pos := int32((1 << 16) | 2)

	// default permissive when hook is unset
	if !s.canInteractBuild(c, pos, "request_item") {
		t.Fatalf("expected default canInteractBuild=true without hook")
	}

	s.CanInteractBuildFn = func(_ *Conn, p int32, action string) bool {
		return p == pos && action == "request_item"
	}
	if !s.canInteractBuild(c, pos, "request_item") {
		t.Fatalf("expected hook-allowed request_item")
	}
	if s.canInteractBuild(c, pos, "transfer_inventory") {
		t.Fatalf("expected hook deny for different action")
	}
}

func TestConnTeamIDPrefersUnitThenPlayer(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	c := &Conn{playerID: 11, unitID: 22}

	// Default fallback
	if team := s.connTeamID(c); team != 1 {
		t.Fatalf("expected fallback team=1, got=%d", team)
	}

	s.entities[c.playerID] = &protocol.PlayerEntity{IDValue: c.playerID, TeamID: 3}
	if team := s.connTeamID(c); team != 3 {
		t.Fatalf("expected player team=3, got=%d", team)
	}

	s.entities[c.unitID] = &protocol.UnitEntitySync{IDValue: c.unitID, TeamID: 7}
	if team := s.connTeamID(c); team != 7 {
		t.Fatalf("expected unit team=7, got=%d", team)
	}
}

func TestSetConnTeamUpdatesEntities(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	c := &Conn{playerID: 21, unitID: 22, teamID: 1}
	s.entities[c.playerID] = &protocol.PlayerEntity{IDValue: c.playerID, TeamID: 1}
	s.entities[c.unitID] = &protocol.UnitEntitySync{IDValue: c.unitID, TeamID: 1}

	s.setConnTeam(c, 6)
	if got := s.connTeamID(c); got != 6 {
		t.Fatalf("expected conn team=6, got=%d", got)
	}
	if p, _ := s.entities[c.playerID].(*protocol.PlayerEntity); p.TeamID != 6 {
		t.Fatalf("expected player team=6, got=%d", p.TeamID)
	}
	if u, _ := s.entities[c.unitID].(*protocol.UnitEntitySync); u.TeamID != 6 {
		t.Fatalf("expected unit team=6, got=%d", u.TeamID)
	}
}

func TestTeamEditorCallbackGate(t *testing.T) {
	s := NewServer("127.0.0.1:0", 155)
	c := &Conn{playerID: 31, unitID: 32, teamID: 1}
	s.entities[c.playerID] = &protocol.PlayerEntity{IDValue: c.playerID, TeamID: 1}
	s.entities[c.unitID] = &protocol.UnitEntitySync{IDValue: c.unitID, TeamID: 1}

	// blocked
	s.OnSetPlayerTeamEditor = func(_ *Conn, _ byte) bool { return false }
	if s.OnSetPlayerTeamEditor(c, 7) {
		t.Fatalf("expected callback to block")
	}
	if got := s.connTeamID(c); got != 1 {
		t.Fatalf("expected team unchanged=1, got=%d", got)
	}

	// allowed
	s.OnSetPlayerTeamEditor = func(_ *Conn, _ byte) bool { return true }
	if !s.OnSetPlayerTeamEditor(c, 7) {
		t.Fatalf("expected callback to allow")
	}
	s.setConnTeam(c, 7)
	if got := s.connTeamID(c); got != 7 {
		t.Fatalf("expected team changed to 7, got=%d", got)
	}
}
