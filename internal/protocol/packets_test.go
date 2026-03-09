package protocol

import (
	"encoding/binary"
	"strings"
	"testing"
)

func TestDisconnectRoundTrip(t *testing.T) {
	in := &Disconnect{Reason: "server restart"}
	w := NewWriter()
	if err := in.Write(w); err != nil {
		t.Fatalf("write disconnect: %v", err)
	}

	out := &Disconnect{}
	if err := out.Read(NewReader(w.Bytes()), len(w.Bytes())); err != nil {
		t.Fatalf("read disconnect: %v", err)
	}
	if out.Reason != in.Reason {
		t.Fatalf("reason mismatch: got %q want %q", out.Reason, in.Reason)
	}
}

func TestWorldStreamRoundTrip(t *testing.T) {
	in := &WorldStream{Data: []byte{1, 2, 3, 4, 5}}
	w := NewWriter()
	if err := in.Write(w); err != nil {
		t.Fatalf("write world stream: %v", err)
	}

	out := &WorldStream{}
	if err := out.Read(NewReader(w.Bytes()), len(w.Bytes())); err != nil {
		t.Fatalf("read world stream: %v", err)
	}
	if len(out.Data) != len(in.Data) {
		t.Fatalf("data len mismatch: got %d want %d", len(out.Data), len(in.Data))
	}
	for i := range in.Data {
		if out.Data[i] != in.Data[i] {
			t.Fatalf("data[%d] mismatch: got %d want %d", i, out.Data[i], in.Data[i])
		}
	}
}

func TestStreamChunkRejectsOversizeWrite(t *testing.T) {
	in := &StreamChunk{
		ID:   1,
		Data: make([]byte, 32768),
	}
	err := in.Write(NewWriter())
	if err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("expected too large error, got: %v", err)
	}
}

func TestStreamChunkRejectsNegativeLengthRead(t *testing.T) {
	raw := make([]byte, 6)
	binary.BigEndian.PutUint32(raw[0:4], uint32(1))
	binary.BigEndian.PutUint16(raw[4:6], uint16(0xFFFF)) // -1 as int16

	out := &StreamChunk{}
	err := out.Read(NewReader(raw), len(raw))
	if err == nil || !strings.Contains(err.Error(), "invalid stream chunk length") {
		t.Fatalf("expected invalid length error, got: %v", err)
	}
}
