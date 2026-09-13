package net

import (
	"testing"

	"mdt-server/internal/protocol"
	"mdt-server/internal/world"
)

func TestBuildCoreSnapshotDataFromTeamsEncodesOfficialCoreLayout(t *testing.T) {
	data := BuildCoreSnapshotDataFromTeams([]world.TeamCoreItemSnapshot{
		{
			Team: 1,
			Items: []world.ItemStack{
				{Item: 0, Amount: 12},
				{Item: 1, Amount: 7},
			},
		},
	})

	r := protocol.NewReader(data)
	teams, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read teams failed: %v", err)
	}
	if teams != 1 {
		t.Fatalf("expected 1 team snapshot, got %d", teams)
	}
	teamID, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read team id failed: %v", err)
	}
	if teamID != 1 {
		t.Fatalf("expected team id 1, got %d", teamID)
	}
	itemCount, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read item count failed: %v", err)
	}
	if itemCount != 2 {
		t.Fatalf("expected 2 items, got %d", itemCount)
	}
	firstItem, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read first item id failed: %v", err)
	}
	firstAmount, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read first item amount failed: %v", err)
	}
	secondItem, err := r.ReadInt16()
	if err != nil {
		t.Fatalf("read second item id failed: %v", err)
	}
	secondAmount, err := r.ReadInt32()
	if err != nil {
		t.Fatalf("read second item amount failed: %v", err)
	}
	if firstItem != 0 || firstAmount != 12 {
		t.Fatalf("expected first stack (0,12), got (%d,%d)", firstItem, firstAmount)
	}
	if secondItem != 1 || secondAmount != 7 {
		t.Fatalf("expected second stack (1,7), got (%d,%d)", secondItem, secondAmount)
	}
}

func TestBuildStateSnapshotFromWorldUsesWaveTicksAndRealCoreInventory(t *testing.T) {
	w := world.New(world.Config{TPS: 60})
	model := world.NewWorldModel(16, 16)
	model.BlockNames = map[int16]string{
		339: "core-shard",
	}

	coreTile, err := model.TileAt(5, 5)
	if err != nil || coreTile == nil {
		t.Fatalf("core tile lookup failed: %v", err)
	}
	coreTile.Block = 339
	coreTile.Team = 1
	coreTile.Build = &world.Building{
		Block:     339,
		Team:      1,
		X:         5,
		Y:         5,
		Health:    1000,
		MaxHealth: 1000,
		Items: []world.ItemStack{
			{Item: 0, Amount: 12},
			{Item: 1, Amount: 7},
		},
	}
	w.SetModel(model)
	w.ApplySnapshot(world.Snapshot{
		WaveTime: 10.5,
		Wave:     7,
		Enemies:  3,
		Paused:   true,
		GameOver: false,
		TimeData: 91,
		Tps:      58,
		Rand0:    111,
		Rand1:    222,
		Tick:     999,
	})

	packet := BuildStateSnapshotFromWorld(w)
	if packet == nil {
		t.Fatal("expected state snapshot packet")
	}
	if packet.WaveTime != 630 {
		t.Fatalf("expected wave time in 60Hz ticks 630, got %v", packet.WaveTime)
	}
	if packet.Wave != 7 || packet.Enemies != 0 {
		t.Fatalf("unexpected wave/enemies fields: wave=%d enemies=%d", packet.Wave, packet.Enemies)
	}
	if packet.Paused || packet.GameOver {
		t.Fatalf("expected world snapshot defaults paused=false gameOver=false, got paused=%v gameOver=%v", packet.Paused, packet.GameOver)
	}
	if packet.TimeData != 91 || packet.Tps != 58 || packet.Rand0 != 111 || packet.Rand1 != 222 {
		t.Fatalf("unexpected snapshot metadata: %+v", packet)
	}

	r := protocol.NewReader(packet.CoreData)
	teams, err := r.ReadByte()
	if err != nil {
		t.Fatalf("read coreData teams failed: %v", err)
	}
	if teams != 1 {
		t.Fatalf("expected one core snapshot team, got %d", teams)
	}
}

func TestBuildStateSnapshotFromWorldNilWorldReturnsMinimalPacket(t *testing.T) {
	packet := BuildStateSnapshotFromWorld(nil)
	if packet == nil {
		t.Fatal("expected minimal packet")
	}
	if len(packet.CoreData) != 1 || packet.CoreData[0] != 0 {
		t.Fatalf("expected minimal coreData [0], got %v", packet.CoreData)
	}
}
