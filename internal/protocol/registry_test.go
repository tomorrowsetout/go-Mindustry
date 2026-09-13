package protocol

import "testing"

func TestPacketRegistryUsesOfficialBasePacketOrder(t *testing.T) {
	reg := NewRegistry()
	assertPacketID := func(name string, packet Packet, want byte) {
		t.Helper()
		got, ok := reg.PacketID(packet)
		if !ok {
			t.Fatalf("packet id missing for %s (%T)", name, packet)
		}
		if got != want {
			t.Fatalf("packet id mismatch for %s: got=%d want=%d", name, got, want)
		}
	}

	// Official 160.3 Call.registerPackets order (global method-name sort).
	assertPacketID("StreamBegin", &StreamBegin{}, 0)
	assertPacketID("StreamChunk", &StreamChunk{}, 1)
	assertPacketID("WorldStream", &WorldStream{}, 2)
	assertPacketID("ConnectPacket", &ConnectPacket{}, 3)
	assertPacketID("AssetRequirementStream", &AssetRequirementStream{}, 4)
	assertPacketID("AssetStream", &AssetStream{}, 5)
	assertPacketID("TextureStream", &TextureStream{}, 6)
}

func TestPacketRegistryUsesOfficial160SyncRemotePacketIDs(t *testing.T) {
	reg := NewRegistry()
	assertPacketID := func(name string, packet Packet, want byte) {
		t.Helper()
		got, ok := reg.PacketID(packet)
		if !ok {
			t.Fatalf("packet id missing for %s (%T)", name, packet)
		}
		if got != want {
			t.Fatalf("packet id mismatch for %s: got=%d want=%d", name, got, want)
		}
	}

	// Extracted from official Mindustry.jar Call.registerPackets.
	assertPacketID("Remote_NetClient_entitySnapshot_32", &Remote_NetClient_entitySnapshot_32{}, 49)
	assertPacketID("Remote_NetClient_hiddenSnapshot_33", &Remote_NetClient_hiddenSnapshot_33{}, 55)
	assertPacketID("Remote_NetClient_blockSnapshot_34", &Remote_NetClient_blockSnapshot_34{}, 14)
	assertPacketID("Remote_NetClient_stateSnapshot_35", &Remote_NetClient_stateSnapshot_35{}, 141)
	assertPacketID("Remote_NetClient_worldDataBegin_28", &Remote_NetClient_worldDataBegin_28{}, 172)
	assertPacketID("Remote_NetServer_clientPlanSnapshot_46", &Remote_NetServer_clientPlanSnapshot_46{}, 27)
	assertPacketID("Remote_NetServer_clientSnapshot_48", &Remote_NetServer_clientSnapshot_48{}, 29)
	assertPacketID("Remote_NetServer_connectConfirm_50", &Remote_NetServer_connectConfirm_50{}, 34)
	assertPacketID("Remote_NetServer_clientLogicDataReliable_43", &Remote_NetServer_clientLogicDataReliable_43{}, 23)
	assertPacketID("Remote_NetClient_ping_18", &Remote_NetClient_ping_18{}, 84)
	assertPacketID("Remote_CoreBlock_playerSpawn_149", &Remote_CoreBlock_playerSpawn_149{}, 89)
	assertPacketID("Remote_Tile_setTile_140", &Remote_Tile_setTile_140{}, 130)
}

func TestCriticalSyncPacketsKeepOfficialPriorities(t *testing.T) {
	checkPriority := func(name string, packet Packet, want int) {
		t.Helper()
		if got := packet.Priority(); got != want {
			t.Fatalf("priority mismatch for %s: got=%d want=%d", name, got, want)
		}
	}

	checkPriority("StreamBegin", &StreamBegin{}, PriorityHigh)
	checkPriority("StreamChunk", &StreamChunk{}, PriorityHigh)
	checkPriority("WorldStream", &WorldStream{}, PriorityNormal)
	checkPriority("ConnectPacket", &ConnectPacket{}, PriorityHigh)
	checkPriority("Remote_NetClient_entitySnapshot_32", &Remote_NetClient_entitySnapshot_32{}, PriorityNormal)
	checkPriority("Remote_NetClient_blockSnapshot_34", &Remote_NetClient_blockSnapshot_34{}, PriorityNormal)
	checkPriority("Remote_NetClient_stateSnapshot_35", &Remote_NetClient_stateSnapshot_35{}, PriorityNormal)
	checkPriority("Remote_NetServer_connectConfirm_50", &Remote_NetServer_connectConfirm_50{}, PriorityNormal)
}
