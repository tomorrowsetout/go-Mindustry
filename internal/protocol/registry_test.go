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

	// Official 159.7 wire ids (tools/gen_registry.py OFFICIAL) + 1 for
	// TextureStream at framework id 6. New 160 remotes sit later in the table
	// and do not shift these NetClient/NetServer slots.
	assertPacketID("Remote_NetClient_entitySnapshot_32", &Remote_NetClient_entitySnapshot_32{}, 23)
	assertPacketID("Remote_NetClient_hiddenSnapshot_33", &Remote_NetClient_hiddenSnapshot_33{}, 24)
	assertPacketID("Remote_NetClient_blockSnapshot_34", &Remote_NetClient_blockSnapshot_34{}, 12)
	assertPacketID("Remote_NetClient_stateSnapshot_35", &Remote_NetClient_stateSnapshot_35{}, 41)
	assertPacketID("Remote_NetServer_clientPlanSnapshot_46", &Remote_NetServer_clientPlanSnapshot_46{}, 47)
	assertPacketID("Remote_NetServer_clientSnapshot_48", &Remote_NetServer_clientSnapshot_48{}, 49)
	assertPacketID("Remote_NetServer_connectConfirm_50", &Remote_NetServer_connectConfirm_50{}, 50)
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
