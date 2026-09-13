package net

import (
	"testing"

	"mdt-server/internal/protocol"
)

// makeChatPacket builds a representative broadcast payload.
func makeChatPacket(msg string) *protocol.Remote_NetClient_sendChatMessage_16 {
	return &protocol.Remote_NetClient_sendChatMessage_16{Message: msg}
}

// BenchmarkBroadcastFanOut compares serializing once per recipient (the old
// Broadcast behaviour) against pre-encoding once and sharing the frame bytes
// (the optimized path). It measures only the encode cost, which is the part
// the optimization eliminates; socket writes are identical either way.
func BenchmarkBroadcastFanOut(b *testing.B) {
	srv := NewServer("127.0.0.1:0", 157)
	pkt := makeChatPacket("benchmark broadcast message payload for serialization")

	b.Run("per-connection-encode", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			// Simulate N recipients each serializing the same object.
			for recipient := 0; recipient < 32; recipient++ {
				buf := getSendBuffer()
				if err := srv.Serial.WriteObject(buf, pkt); err != nil {
					b.Fatal(err)
				}
				putSendBuffer(buf)
			}
		}
	})

	b.Run("pre-encoded-fanout", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			encoded, ok := srv.preEncode(pkt)
			if !ok {
				b.Fatal("preEncode failed")
			}
			// N recipients share the single pre-encoded frame.
			for recipient := 0; recipient < 32; recipient++ {
				if len(encoded.payload) == 0 {
					b.Fatal("empty pre-encoded payload")
				}
			}
		}
	})
}

// BenchmarkPreEncodePriority verifies the wrapper preserves packet priority
// (a correctness invariant the fan-out relies on).
func TestPreEncodePreservesPriority(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 157)
	high := &protocol.StreamBegin{ID: 7} // high priority
	encoded, ok := srv.preEncode(high)
	if !ok {
		t.Fatal("preEncode failed")
	}
	if encoded.Priority() != protocol.PriorityHigh {
		t.Fatalf("priority = %d, want %d (high must be preserved)", encoded.Priority(), protocol.PriorityHigh)
	}
	if unwrapPreEncoded(encoded) != high {
		t.Fatal("unwrap must return the original object")
	}
}
