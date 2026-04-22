package net

import (
	"io"
	"net"
	"testing"
	"time"

	"mdt-server/internal/protocol"
)

func TestHandlePacketOfficialBeginBreakUsesDedicatedHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	called := 0
	fallback := 0
	var gotX, gotY int32
	srv.OnBeginBreak = func(c *Conn, x, y int32) {
		called++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotX, gotY = x, y
	}
	srv.OnBuildPlans = func(*Conn, []*protocol.BuildPlan) {
		fallback++
	}

	srv.handlePacket(conn, &protocol.Remote_Build_beginBreak_132{X: 7, Y: 9}, true)

	if called != 1 {
		t.Fatalf("expected OnBeginBreak once, got %d", called)
	}
	if fallback != 0 {
		t.Fatalf("expected shared OnBuildPlans path to stay unused, got %d", fallback)
	}
	if gotX != 7 || gotY != 9 {
		t.Fatalf("expected break pos (7,9), got (%d,%d)", gotX, gotY)
	}
}

func TestHandlePacketOfficialBeginPlaceUsesDedicatedHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	called := 0
	fallback := 0
	var got BeginPlaceRequest
	srv.OnBeginPlace = func(c *Conn, req BeginPlaceRequest) {
		called++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		got = req
	}
	srv.OnBuildPlans = func(*Conn, []*protocol.BuildPlan) {
		fallback++
	}

	srv.handlePacket(conn, &protocol.Remote_Build_beginPlace_133{
		X:           11,
		Y:           13,
		Result:      protocol.BlockRef{BlkID: 425},
		Rotation:    3,
		PlaceConfig: protocol.Point2{X: 2, Y: -1},
	}, true)

	if called != 1 {
		t.Fatalf("expected OnBeginPlace once, got %d", called)
	}
	if fallback != 0 {
		t.Fatalf("expected shared OnBuildPlans path to stay unused, got %d", fallback)
	}
	if got.X != 11 || got.Y != 13 || got.BlockID != 425 || got.Rotation != 3 {
		t.Fatalf("unexpected beginPlace request: %+v", got)
	}
	if point, ok := got.Config.(protocol.Point2); !ok || point.X != 2 || point.Y != -1 {
		t.Fatalf("expected place config Point2(2,-1), got %T %#v", got.Config, got.Config)
	}
}

func TestHandlePacketOfficialRotateBlockUsesDedicatedHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	called := 0
	var gotPos int32
	var gotDir bool
	srv.OnRotateBlock = func(c *Conn, pos int32, direction bool) {
		called++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotPos = pos
		gotDir = direction
	}

	pos := protocol.PackPoint2(6, 8)
	srv.handlePacket(conn, &protocol.Remote_InputHandler_rotateBlock_89{
		Build:     protocol.BuildingBox{PosValue: pos},
		Direction: true,
	}, true)

	if called != 1 {
		t.Fatalf("expected OnRotateBlock once, got %d", called)
	}
	if gotPos != pos || !gotDir {
		t.Fatalf("expected rotate request pos=%d dir=true, got pos=%d dir=%v", pos, gotPos, gotDir)
	}
}

func TestHandlePacketOfficialBuildingControlSelectUsesDedicatedHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	called := 0
	var gotPos int32
	srv.OnBuildingControlSelect = func(c *Conn, pos int32) {
		called++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotPos = pos
	}

	pos := protocol.PackPoint2(3, 4)
	srv.handlePacket(conn, &protocol.Remote_InputHandler_buildingControlSelect_92{
		Player: &protocol.EntityBox{IDValue: 41},
		Build:  protocol.BuildingBox{PosValue: pos},
	}, true)

	if called != 1 {
		t.Fatalf("expected OnBuildingControlSelect once, got %d", called)
	}
	if gotPos != pos {
		t.Fatalf("expected control-select pos=%d, got %d", pos, gotPos)
	}
}

func TestHandlePacketOfficialRequestItemUsesDedicatedHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	called := 0
	var gotPos int32
	var gotItem int16
	var gotAmount int32
	srv.OnRequestItem = func(c *Conn, pos int32, itemID int16, amount int32) {
		called++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotPos = pos
		gotItem = itemID
		gotAmount = amount
	}

	pos := protocol.PackPoint2(7, 11)
	srv.handlePacket(conn, &protocol.Remote_InputHandler_requestItem_78{
		Player: &protocol.EntityBox{IDValue: 9},
		Build:  protocol.BuildingBox{PosValue: pos},
		Item:   protocol.ItemRef{ItmID: 4},
		Amount: 13,
	}, true)

	if called != 1 {
		t.Fatalf("expected OnRequestItem once, got %d", called)
	}
	if gotPos != pos || gotItem != 4 || gotAmount != 13 {
		t.Fatalf("expected pos=%d item=4 amount=13, got pos=%d item=%d amount=%d", pos, gotPos, gotItem, gotAmount)
	}
}

func TestHandlePacketOfficialTransferInventoryUsesDedicatedHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	called := 0
	var gotPos int32
	srv.OnTransferInventory = func(c *Conn, pos int32) {
		called++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotPos = pos
	}

	pos := protocol.PackPoint2(2, 14)
	srv.handlePacket(conn, &protocol.Remote_InputHandler_transferInventory_79{
		Player: &protocol.EntityBox{IDValue: 5},
		Build:  protocol.BuildingBox{PosValue: pos},
	}, true)

	if called != 1 {
		t.Fatalf("expected OnTransferInventory once, got %d", called)
	}
	if gotPos != pos {
		t.Fatalf("expected pos=%d, got %d", pos, gotPos)
	}
}

func TestHandlePacketOfficialRequestBlockSnapshotUsesDedicatedHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	called := 0
	var gotPos int32
	srv.OnRequestBlockSnapshot = func(c *Conn, pos int32) {
		called++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotPos = pos
	}

	pos := protocol.PackPoint2(4, 9)
	srv.handlePacket(conn, &protocol.Remote_NetServer_requestBlockSnapshot_45{
		Player: &protocol.EntityBox{IDValue: 11},
		Pos:    pos,
	}, true)

	if called != 1 {
		t.Fatalf("expected OnRequestBlockSnapshot once, got %d", called)
	}
	if gotPos != pos {
		t.Fatalf("expected pos=%d, got %d", pos, gotPos)
	}
}

func TestHandlePacketOfficialDropItemUsesDedicatedHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	called := 0
	var gotAngle float32
	srv.OnDropItem = func(c *Conn, angle float32) {
		called++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotAngle = angle
	}

	srv.handlePacket(conn, &protocol.Remote_InputHandler_dropItem_88{
		Player: &protocol.EntityBox{IDValue: 3},
		Angle:  135,
	}, true)

	if called != 1 {
		t.Fatalf("expected OnDropItem once, got %d", called)
	}
	if gotAngle != 135 {
		t.Fatalf("expected angle 135, got %v", gotAngle)
	}
}

func TestHandlePacketOfficialUnitBuildingControlSelectUsesDedicatedHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	called := 0
	var gotUnitID, gotPos int32
	srv.OnUnitBuildingControlSelect = func(c *Conn, unitID, pos int32) {
		called++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotUnitID = unitID
		gotPos = pos
	}

	pos := protocol.PackPoint2(10, 12)
	srv.handlePacket(conn, &protocol.Remote_InputHandler_unitBuildingControlSelect_93{
		Unit:  protocol.UnitBox{IDValue: 77},
		Build: protocol.BuildingBox{PosValue: pos},
	}, true)

	if called != 1 {
		t.Fatalf("expected OnUnitBuildingControlSelect once, got %d", called)
	}
	if gotUnitID != 77 || gotPos != pos {
		t.Fatalf("expected unit=%d pos=%d, got unit=%d pos=%d", 77, pos, gotUnitID, gotPos)
	}
}

func TestHandlePacketClientSnapshotUsesAuthoritativeHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{hasConnected: true}

	preview := 0
	authoritative := 0
	srv.OnBuildPlanPreview = func(c *Conn, plans []*protocol.BuildPlan) {
		preview++
		if c != conn {
			t.Fatalf("expected preview hook to receive original conn")
		}
		if len(plans) != 1 || plans[0] == nil || plans[0].X != 4 || plans[0].Y != 6 {
			t.Fatalf("unexpected preview plans: %#v", plans)
		}
	}
	srv.OnBuildPlanSnapshot = func(*Conn, []*protocol.BuildPlan) {
		authoritative++
	}

	srv.handlePacket(conn, &protocol.Remote_NetServer_clientSnapshot_48{
		SnapshotID: 1,
		Plans: []*protocol.BuildPlan{{
			X:     4,
			Y:     6,
			Block: protocol.BlockRef{BlkID: 12, BlkName: "conveyor"},
		}},
	}, true)

	if authoritative != 1 {
		t.Fatalf("expected authoritative hook once, got %d", authoritative)
	}
	if preview != 0 {
		t.Fatalf("expected preview hook to stay unused for clientSnapshot, got %d", preview)
	}
}

func TestHandlePacketClientPlanSnapshotUsesPreviewHook(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	preview := 0
	authoritative := 0
	srv.OnBuildPlanPreview = func(c *Conn, plans []*protocol.BuildPlan) {
		preview++
		if c != conn {
			t.Fatalf("expected preview hook to receive original conn")
		}
		if len(plans) != 1 || plans[0] == nil || plans[0].X != 7 || plans[0].Y != 9 {
			t.Fatalf("unexpected preview plans: %#v", plans)
		}
	}
	srv.OnBuildPlanSnapshot = func(*Conn, []*protocol.BuildPlan) {
		authoritative++
	}

	srv.handlePacket(conn, &protocol.Remote_NetServer_clientPlanSnapshot_46{
		GroupId: 3,
		Plans: []*protocol.BuildPlan{{
			X:     7,
			Y:     9,
			Block: protocol.BlockRef{BlkID: 13, BlkName: "router"},
		}},
	}, true)

	if preview != 1 {
		t.Fatalf("expected preview hook once, got %d", preview)
	}
	if authoritative != 0 {
		t.Fatalf("expected authoritative hook to stay unused for clientPlanSnapshot, got %d", authoritative)
	}
}

func TestHandlePacketClientPlanSnapshotBroadcastsToTeammates(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	receiver := NewConn(serverSide, srv.Serial)
	defer receiver.Close()
	receiver.hasConnected = true
	receiver.playerID = 9
	receiver.teamID = 1
	srv.addConn(receiver)

	sender := &Conn{playerID: 7, teamID: 1}
	srv.handlePacket(sender, &protocol.Remote_NetServer_clientPlanSnapshot_46{
		GroupId: 5,
		Plans: []*protocol.BuildPlan{{
			X:     11,
			Y:     13,
			Block: protocol.BlockRef{BlkID: 12, BlkName: "conveyor"},
		}},
	}, true)

	srv.BroadcastStoredClientPlanPreviewsAt(time.Now().Add(200 * time.Millisecond))

	packetID, payload := readFramedPacketForMenuTest(t, clientSide)
	wantID, ok := srv.Registry.PacketID(&protocol.Remote_NetServer_clientPlanSnapshotReceived_47{})
	if !ok {
		t.Fatal("resolve clientPlanSnapshotReceived packet id")
	}
	if packetID != byte(wantID) {
		t.Fatalf("expected packet id %d, got %d", wantID, packetID)
	}
	packet := &protocol.Remote_NetServer_clientPlanSnapshotReceived_47{}
	if err := packet.Read(protocol.NewReaderWithContext(payload, srv.TypeIO), 0); err != nil {
		t.Fatalf("decode clientPlanSnapshotReceived failed: %v", err)
	}
	if packet.Player == nil || packet.Player.ID() != 7 {
		t.Fatalf("expected preview sender player id 7, got %#v", packet.Player)
	}
	if packet.GroupId != 1 {
		t.Fatalf("expected preview broadcast group id 1, got %d", packet.GroupId)
	}
	plans, ok := packet.Plans.([]*protocol.BuildPlan)
	if !ok || len(plans) != 1 || plans[0] == nil || plans[0].X != 11 || plans[0].Y != 13 {
		t.Fatalf("unexpected preview plans %#v", packet.Plans)
	}
}

func TestHandlePacketPingLocationBroadcastsToPeers(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)

	serverSide, clientSide := net.Pipe()
	defer serverSide.Close()
	defer clientSide.Close()

	receiver := NewConn(serverSide, srv.Serial)
	defer receiver.Close()
	receiver.hasConnected = true
	receiver.playerID = 9
	receiver.teamID = 2
	srv.addConn(receiver)

	sender := &Conn{playerID: 7, teamID: 1}
	srv.handlePacket(sender, &protocol.Remote_InputHandler_pingLocation_73{
		Player: &protocol.EntityBox{IDValue: 7},
		X:      144,
		Y:      88,
		Text:   "danger",
	}, true)

	packetID, payload := readFramedPacketForMenuTest(t, clientSide)
	wantID, ok := srv.Registry.PacketID(&protocol.Remote_InputHandler_pingLocation_73{})
	if !ok {
		t.Fatal("resolve pingLocation packet id")
	}
	if packetID != byte(wantID) {
		t.Fatalf("expected packet id %d, got %d", wantID, packetID)
	}
	packet := &protocol.Remote_InputHandler_pingLocation_73{}
	if err := packet.Read(protocol.NewReaderWithContext(payload, srv.TypeIO), 0); err != nil {
		t.Fatalf("decode pingLocation failed: %v", err)
	}
	if packet.Player == nil || packet.Player.ID() != 7 {
		t.Fatalf("expected ping player id 7, got %#v", packet.Player)
	}
	if packet.X != 144 || packet.Y != 88 {
		t.Fatalf("expected ping coords (144,88), got (%f,%f)", packet.X, packet.Y)
	}
	if text, ok := packet.Text.(string); !ok || text != "danger" {
		t.Fatalf("expected ping text 'danger', got %T %#v", packet.Text, packet.Text)
	}

	time.Sleep(10 * time.Millisecond)
}

func TestHandlePacketEmptyClientPlanSnapshotDoesNotClearAuthoritativeQueue(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	authoritative := 0
	srv.OnBuildPlanSnapshot = func(*Conn, []*protocol.BuildPlan) {
		authoritative++
	}

	srv.handlePacket(conn, &protocol.Remote_NetServer_clientPlanSnapshot_46{
		GroupId: 3,
		Plans:   nil,
	}, true)

	if authoritative != 0 {
		t.Fatalf("expected empty clientPlanSnapshot to be ignored, got %d authoritative calls", authoritative)
	}
}

func TestHandlePacketConstructAndAssemblerHooksUseDedicatedChains(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{}

	constructCalled := 0
	droneCalled := 0
	var gotConstruct ConstructFinishRequest
	var gotDrone AssemblerDroneSpawnedRequest
	srv.OnConstructFinish = func(c *Conn, req ConstructFinishRequest) {
		constructCalled++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotConstruct = req
	}
	srv.OnAssemblerDroneSpawned = func(c *Conn, req AssemblerDroneSpawnedRequest) {
		droneCalled++
		if c != conn {
			t.Fatalf("expected original conn pointer")
		}
		gotDrone = req
	}

	buildPos := protocol.PackPoint2(14, 5)
	srv.handlePacket(conn, &protocol.Remote_ConstructBlock_constructFinish_146{
		Tile:     protocol.TileBox{PosValue: buildPos},
		Block:    protocol.BlockRef{BlkID: 500},
		Builder:  protocol.UnitBox{IDValue: 99},
		Rotation: 2,
		Team:     protocol.Team{ID: 3},
		Config:   protocol.Point2{X: -2, Y: 1},
	}, true)
	srv.handlePacket(conn, &protocol.Remote_UnitAssembler_assemblerDroneSpawned_151{
		Tile: protocol.TileBox{PosValue: buildPos},
		Id:   123456,
	}, true)

	if constructCalled != 1 {
		t.Fatalf("expected OnConstructFinish once, got %d", constructCalled)
	}
	if gotConstruct.Pos != buildPos || gotConstruct.BlockID != 500 || gotConstruct.BuilderID != 99 || gotConstruct.Rotation != 2 || gotConstruct.TeamID != 3 {
		t.Fatalf("unexpected construct request: %+v", gotConstruct)
	}
	if point, ok := gotConstruct.Config.(protocol.Point2); !ok || point.X != -2 || point.Y != 1 {
		t.Fatalf("expected construct config Point2(-2,1), got %T %#v", gotConstruct.Config, gotConstruct.Config)
	}
	if droneCalled != 1 {
		t.Fatalf("expected OnAssemblerDroneSpawned once, got %d", droneCalled)
	}
	if gotDrone.Pos != buildPos || gotDrone.UnitID != 123456 {
		t.Fatalf("unexpected assembler drone request: %+v", gotDrone)
	}
}

func TestReleaseConnUnitControlClearsConnectionButKeepsWorldUnit(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{playerID: 7, unitID: 100}

	var gotUnitID, gotPlayerID int32
	called := 0
	srv.SetUnitPlayerControllerFn = func(unitID int32, playerID int32) bool {
		called++
		gotUnitID = unitID
		gotPlayerID = playerID
		return true
	}

	srv.entityMu.Lock()
	srv.entities[100] = testPlayerControlledUnit(100, 7, false)
	srv.entityMu.Unlock()

	if !srv.ReleaseConnUnitControl(conn) {
		t.Fatal("expected release to succeed")
	}
	if conn.unitID != 0 {
		t.Fatalf("expected connection unit to clear, got %d", conn.unitID)
	}
	if called != 1 || gotUnitID != 100 || gotPlayerID != 0 {
		t.Fatalf("expected controller release call (100,0), got called=%d unit=%d player=%d", called, gotUnitID, gotPlayerID)
	}

	srv.entityMu.Lock()
	defer srv.entityMu.Unlock()
	ent, ok := srv.entities[100]
	if !ok {
		t.Fatal("expected released world unit to remain present")
	}
	unit, ok := ent.(*protocol.UnitEntitySync)
	if !ok || unit == nil {
		t.Fatalf("expected stored entity to stay a unit, got %T", ent)
	}
	state, ok := unit.Controller.(*protocol.ControllerState)
	if !ok || state == nil || state.Type != protocol.ControllerGenericAI {
		t.Fatalf("expected released unit controller to switch to generic AI, got %#v", unit.Controller)
	}
	if unit.UpdateBuilding {
		t.Fatal("expected released unit to stop updateBuilding")
	}
}

func TestConsumeConnUnitClearsConnectionAndDeletesWorldUnit(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{playerID: 7, unitID: 100}

	srv.entityMu.Lock()
	srv.entities[100] = testPlayerControlledUnit(100, 7, false)
	srv.entityMu.Unlock()

	if !srv.ConsumeConnUnit(conn, 100) {
		t.Fatal("expected consume to succeed")
	}
	if conn.unitID != 0 {
		t.Fatalf("expected connection unit to clear, got %d", conn.unitID)
	}

	srv.entityMu.Lock()
	defer srv.entityMu.Unlock()
	if _, ok := srv.entities[100]; ok {
		t.Fatal("expected consumed unit entity to be removed")
	}
}

func TestHandleCoreBuildingControlSelectRespawnsAtRequestedCoreAndReleasesNonCoreUnit(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
	}()

	conn.playerID = 7
	conn.unitID = 100
	conn.teamID = 1

	spawnPackets := 0
	gotSpawnPos := int32(-1)
	conn.onSend = func(obj any, _ int, _ int, _ int) {
		pkt, ok := obj.(*protocol.Remote_CoreBlock_playerSpawn_149)
		if !ok {
			return
		}
		spawnPackets++
		gotSpawnPos = pkt.Tile.Pos()
		if pkt.Player == nil || pkt.Player.ID() != 7 {
			t.Fatalf("expected playerSpawn packet for player 7, got %#v", pkt.Player)
		}
	}

	srv.entityMu.Lock()
	srv.entities[100] = testPlayerControlledUnit(100, 7, false)
	srv.entityMu.Unlock()

	released := 0
	var releasedUnitID, releasedPlayerID int32
	srv.SetUnitPlayerControllerFn = func(unitID int32, playerID int32) bool {
		released++
		releasedUnitID = unitID
		releasedPlayerID = playerID
		return true
	}
	srv.ReserveUnitIDFn = func() int32 { return 500 }

	gotSpawnUnitID := int32(0)
	gotSpawnTile := protocol.Point2{}
	gotSpawnType := int16(0)
	srv.SpawnUnitFn = func(c *Conn, unitID int32, tile protocol.Point2, unitType int16) (float32, float32, bool) {
		gotSpawnUnitID = unitID
		gotSpawnTile = tile
		gotSpawnType = unitType
		return 72, 88, true
	}

	corePos := protocol.Point2{X: 9, Y: 11}
	if !srv.HandleCoreBuildingControlSelect(conn, corePos) {
		t.Fatal("expected core control-select respawn to succeed")
	}
	if released != 1 || releasedUnitID != 100 || releasedPlayerID != 0 {
		t.Fatalf("expected old non-core unit to release once, got released=%d unit=%d player=%d", released, releasedUnitID, releasedPlayerID)
	}
	if gotSpawnUnitID != 500 || conn.unitID != 500 {
		t.Fatalf("expected fresh respawn unit id 500, got spawned=%d conn=%d", gotSpawnUnitID, conn.unitID)
	}
	if gotSpawnTile != corePos {
		t.Fatalf("expected respawn tile %+v, got %+v", corePos, gotSpawnTile)
	}
	if gotSpawnType != 35 {
		t.Fatalf("expected respawn unit type 35, got %d", gotSpawnType)
	}
	if conn.snapX != 72 || conn.snapY != 88 {
		t.Fatalf("expected connection snapshot to move to spawn (72,88), got (%.1f, %.1f)", conn.snapX, conn.snapY)
	}
	if spawnPackets != 1 || gotSpawnPos != protocol.PackPoint2(corePos.X, corePos.Y) {
		t.Fatalf("expected one playerSpawn packet at requested core pos, got packets=%d pos=%d", spawnPackets, gotSpawnPos)
	}

	srv.entityMu.Lock()
	defer srv.entityMu.Unlock()
	oldEnt, ok := srv.entities[100]
	if !ok {
		t.Fatal("expected released non-core unit to remain in world")
	}
	oldUnit := oldEnt.(*protocol.UnitEntitySync)
	if state, ok := oldUnit.Controller.(*protocol.ControllerState); !ok || state == nil || state.Type != protocol.ControllerGenericAI {
		t.Fatalf("expected old unit controller to switch to generic AI, got %#v", oldUnit.Controller)
	}
	newEnt, ok := srv.entities[500]
	if !ok {
		t.Fatal("expected new respawned unit entity to exist")
	}
	newUnit := newEnt.(*protocol.UnitEntitySync)
	if !newUnit.SpawnedByCore {
		t.Fatal("expected new respawned unit to be marked spawnedByCore")
	}
	state, ok := newUnit.Controller.(*protocol.ControllerState)
	if !ok || state == nil || state.Type != protocol.ControllerPlayer || state.PlayerID != 7 {
		t.Fatalf("expected new unit controller to belong to player 7, got %#v", newUnit.Controller)
	}
}

func TestHandleCoreBuildingControlSelectDropsExistingCoreUnitBeforeRespawn(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})
	srv.PlayerUnitTypeFn = func() int16 { return 35 }

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
	}()

	conn.playerID = 7
	conn.unitID = 100
	conn.teamID = 1

	srv.entityMu.Lock()
	srv.entities[100] = testPlayerControlledUnit(100, 7, true)
	srv.entityMu.Unlock()

	released := 0
	srv.SetUnitPlayerControllerFn = func(unitID int32, playerID int32) bool {
		released++
		return true
	}

	dropped := 0
	var droppedUnitID int32
	srv.DropUnitFn = func(unitID int32) {
		dropped++
		droppedUnitID = unitID
	}
	srv.ReserveUnitIDFn = func() int32 { return 501 }
	srv.SpawnUnitFn = func(c *Conn, unitID int32, tile protocol.Point2, unitType int16) (float32, float32, bool) {
		return 96, 104, true
	}

	corePos := protocol.Point2{X: 4, Y: 6}
	if !srv.HandleCoreBuildingControlSelect(conn, corePos) {
		t.Fatal("expected core respawn to succeed")
	}
	if released != 0 {
		t.Fatalf("expected spawnedByCore old unit to despawn instead of release, got release calls=%d", released)
	}
	if dropped != 1 || droppedUnitID != 100 {
		t.Fatalf("expected old spawnedByCore unit 100 to drop once, got dropped=%d unit=%d", dropped, droppedUnitID)
	}
	if conn.unitID != 501 {
		t.Fatalf("expected new respawn unit id 501, got %d", conn.unitID)
	}

	srv.entityMu.Lock()
	defer srv.entityMu.Unlock()
	if _, ok := srv.entities[100]; ok {
		t.Fatal("expected old spawnedByCore unit entity to be removed")
	}
	if _, ok := srv.entities[501]; !ok {
		t.Fatal("expected new respawned unit entity to exist")
	}
}

func TestHandleUnitControlBlockUnitClaimsControlledBuild(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	conn := &Conn{playerID: 7, unitID: 100, teamID: 1}

	srv.entityMu.Lock()
	srv.entities[100] = testPlayerControlledUnit(100, 7, false)
	srv.entityMu.Unlock()

	released := 0
	var releasedUnitID, releasedPlayerID int32
	srv.SetUnitPlayerControllerFn = func(unitID int32, playerID int32) bool {
		released++
		releasedUnitID = unitID
		releasedPlayerID = playerID
		return true
	}

	pos := protocol.PackPoint2(12, 9)
	claimed := 0
	srv.ClaimControlledBuildFn = func(playerID int32, buildPos int32) (ControlledBuildInfo, bool) {
		claimed++
		if playerID != 7 || buildPos != pos {
			t.Fatalf("unexpected controlled-build claim player=%d pos=%d", playerID, buildPos)
		}
		return ControlledBuildInfo{
			Pos:    buildPos,
			X:      100,
			Y:      76,
			TeamID: 1,
		}, true
	}
	srv.ControlledBuildInfoFn = func(playerID int32, buildPos int32) (ControlledBuildInfo, bool) {
		if playerID != 7 || buildPos != pos {
			return ControlledBuildInfo{}, false
		}
		return ControlledBuildInfo{
			Pos:    buildPos,
			X:      100,
			Y:      76,
			TeamID: 1,
		}, true
	}

	srv.handlePacket(conn, &protocol.Remote_InputHandler_unitControl_94{
		Unit: protocol.BlockUnitRef{
			TileRef: protocol.BlockUnitTileRef{PosValue: pos},
		},
	}, true)

	if claimed != 1 {
		t.Fatalf("expected exactly one controlled-build claim, got %d", claimed)
	}
	if released != 1 || releasedUnitID != 100 || releasedPlayerID != 0 {
		t.Fatalf("expected old unit control release, got released=%d unit=%d player=%d", released, releasedUnitID, releasedPlayerID)
	}
	if conn.unitID != 0 {
		t.Fatalf("expected unitID to clear after entering controlled build, got %d", conn.unitID)
	}
	if !conn.controlBuildActive || conn.controlBuildPos != pos {
		t.Fatalf("expected controlled build pos=%d to be active, got active=%v pos=%d", pos, conn.controlBuildActive, conn.controlBuildPos)
	}
	if conn.snapX != 100 || conn.snapY != 76 {
		t.Fatalf("expected snap to controlled build center, got (%.1f, %.1f)", conn.snapX, conn.snapY)
	}

	player := &protocol.PlayerEntity{IDValue: conn.playerID}
	srv.updatePlayerEntity(player, conn)
	if _, ok := player.Unit.(protocol.BlockUnit); !ok {
		t.Fatalf("expected player entity to point at a block unit, got %T", player.Unit)
	}
}

func TestUnitControlClaimsNewUnitAndReleasesOldUnitWithoutRangeGate(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	srv.Content.RegisterUnitType(testUnitType{id: 35, name: "alpha"})

	serverSide, clientSide := net.Pipe()
	defer clientSide.Close()

	conn := NewConn(serverSide, srv.Serial)
	defer conn.Close()
	go func() {
		_, _ = io.Copy(io.Discard, clientSide)
	}()

	conn.playerID = 7
	conn.unitID = 100
	conn.teamID = 1
	conn.snapX = 32
	conn.snapY = 32

	srv.entityMu.Lock()
	srv.entities[100] = testPlayerControlledUnit(100, 7, false)
	srv.entities[200] = testAIControlledUnit(200, 1, 35, 960, 1120)
	srv.entityMu.Unlock()

	srv.UnitInfoFn = func(unitID int32) (UnitInfo, bool) {
		switch unitID {
		case 200:
			return UnitInfo{
				ID:        200,
				X:         960,
				Y:         1120,
				Health:    150,
				MaxHealth: 150,
				TeamID:    1,
				TypeID:    35,
			}, true
		case 100:
			return UnitInfo{
				ID:        100,
				X:         80,
				Y:         120,
				Health:    150,
				MaxHealth: 150,
				TeamID:    1,
				TypeID:    35,
			}, true
		default:
			return UnitInfo{}, false
		}
	}

	var controlCalls [][2]int32
	srv.SetUnitPlayerControllerFn = func(unitID int32, playerID int32) bool {
		controlCalls = append(controlCalls, [2]int32{unitID, playerID})
		return true
	}

	srv.unitControl(conn, 200)

	if conn.unitID != 200 {
		t.Fatalf("expected connection to switch to unit 200, got %d", conn.unitID)
	}
	if conn.snapX != 960 || conn.snapY != 1120 {
		t.Fatalf("expected connection snapshot to move to new unit, got (%.1f, %.1f)", conn.snapX, conn.snapY)
	}
	if len(controlCalls) != 2 {
		t.Fatalf("expected claim+release controller calls, got %d", len(controlCalls))
	}
	if controlCalls[0] != ([2]int32{200, 7}) {
		t.Fatalf("expected first controller call to claim unit 200, got %+v", controlCalls[0])
	}
	if controlCalls[1] != ([2]int32{100, 0}) {
		t.Fatalf("expected second controller call to release unit 100, got %+v", controlCalls[1])
	}

	srv.entityMu.Lock()
	defer srv.entityMu.Unlock()

	oldEnt := srv.entities[100].(*protocol.UnitEntitySync)
	if state, ok := oldEnt.Controller.(*protocol.ControllerState); !ok || state == nil || state.Type != protocol.ControllerGenericAI {
		t.Fatalf("expected old unit controller to revert to generic AI, got %#v", oldEnt.Controller)
	}
	newEnt := srv.entities[200].(*protocol.UnitEntitySync)
	if state, ok := newEnt.Controller.(*protocol.ControllerState); !ok || state == nil || state.Type != protocol.ControllerPlayer || state.PlayerID != 7 {
		t.Fatalf("expected new unit controller to belong to player 7, got %#v", newEnt.Controller)
	}
}

func testPlayerControlledUnit(id int32, playerID int32, spawnedByCore bool) *protocol.UnitEntitySync {
	return &protocol.UnitEntitySync{
		IDValue:        id,
		Controller:     &protocol.ControllerState{Type: protocol.ControllerPlayer, PlayerID: playerID},
		TypeID:         35,
		TeamID:         1,
		X:              80,
		Y:              120,
		Rotation:       90,
		SpawnedByCore:  spawnedByCore,
		UpdateBuilding: true,
		Abilities:      []protocol.Ability{},
		Mounts:         []protocol.WeaponMount{},
		Plans:          []*protocol.BuildPlan{},
		Statuses:       []protocol.StatusEntry{},
		Stack:          protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
	}
}

func testAIControlledUnit(id int32, teamID byte, typeID int16, x, y float32) *protocol.UnitEntitySync {
	return &protocol.UnitEntitySync{
		IDValue:        id,
		Controller:     &protocol.ControllerState{Type: protocol.ControllerGenericAI},
		TypeID:         typeID,
		TeamID:         teamID,
		X:              x,
		Y:              y,
		Health:         150,
		Rotation:       90,
		Abilities:      []protocol.Ability{},
		Mounts:         []protocol.WeaponMount{},
		Plans:          []*protocol.BuildPlan{},
		Statuses:       []protocol.StatusEntry{},
		Stack:          protocol.ItemStack{Item: protocol.ItemRef{ItmID: 0, ItmName: ""}},
		UpdateBuilding: false,
	}
}
