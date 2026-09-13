package net

import (
	"fmt"
	"io"
	"testing"

	"mdt-server/internal/protocol"
)

// Payload decode EOF must not be treated as TCP stream close, otherwise short
// custom-client stub packets during join tear down the connection before
// connectConfirm can complete the loop.
func TestIsConnFrameClosedIgnoresPayloadDecodeEOF(t *testing.T) {
	decodeErr := fmt.Errorf("unknown packet id: 99 (%T read: %w)", (*protocol.Remote_InputHandler_requestItem_78)(nil), io.EOF)
	if isConnFrameClosed(decodeErr) {
		t.Fatal("expected payload decode EOF to not close the TCP frame")
	}
	if !isConnFrameClosed(io.EOF) {
		t.Fatal("expected bare io.EOF to close the TCP frame")
	}
}

func TestSerializerReadsConnectConfirmFromOfficial34(t *testing.T) {
	srv := NewServer("127.0.0.1:0", 160)
	obj, err := srv.Serial.ReadObject(frameSerializerPacket(t, 34, nil))
	if err != nil {
		t.Fatalf("read connectConfirm id=34: %v", err)
	}
	if _, ok := obj.(*protocol.Remote_NetServer_connectConfirm_50); !ok {
		t.Fatalf("id=34 decoded %T, want connectConfirm", obj)
	}
}
