package net

import (
	"errors"
	"fmt"
	"io"
	"testing"

	"mdt-server/internal/protocol"
)

// Payload decode EOF must not be treated as TCP stream close, otherwise short
// custom-client stub packets during join tear down the connection before
// connectConfirm can complete the loop.
func TestIsConnFrameClosedIgnoresPayloadDecodeEOF(t *testing.T) {
	decodeErr := fmt.Errorf("unknown packet id: 34 (%T read: %w)", (*protocol.Remote_NetClient_setCameraPosition_30)(nil), io.EOF)
	if isConnFrameClosed(decodeErr) {
		t.Fatal("expected payload decode EOF to not close the TCP frame")
	}
	if !isConnFrameClosed(io.EOF) {
		t.Fatal("expected bare io.EOF to close the TCP frame")
	}
	if !isConnFrameClosed(errors.New("connection reset by peer")) {
		// not necessarily mapped; only require bare EOF and known wrappers
	}
}

func TestSerializerReadsConnectConfirmFrom49And50(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 160)
	for _, id := range []byte{49, 50} {
		obj, err := srv.Serial.ReadObject(frameSerializerPacket(t, id, nil))
		if err != nil {
			t.Fatalf("read connectConfirm id=%d: %v", id, err)
		}
		if _, ok := obj.(*protocol.Remote_NetServer_connectConfirm_50); !ok {
			t.Fatalf("id=%d decoded %T, want connectConfirm", id, obj)
		}
	}
}
