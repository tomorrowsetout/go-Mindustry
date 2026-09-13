package worldstream

import (
	"bytes"
	"compress/zlib"
	"io"
	"testing"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/world"
)

// TestVerifyDataPatchesHeader proves the world-stream payloads now begin with the
// data-patches header that Java's NetworkIO.writeWorld() emits FIRST:
//   writeInt(DataPatcher.patchFormatVersion=2) + writeInt(assets.size=0)
// i.e. exactly 8 bytes: 02 00 00 00 00 00 00 00.
//
// The 159.7 client's loadWorld() calls readDataPatches() first and reads those 8
// bytes; without them it misreads the rules UTF as the patch header -> throws ->
// "connect aborted before confirm". This harness is temporary (zz_ prefix).
func TestVerifyDataPatchesHeader(t *testing.T) {
	reg := protocol.NewContentRegistry()
	ids, err := vanilla.LoadContentIDs("../../data/vanilla/content_ids.json")
	if err != nil {
		t.Fatalf("load content ids: %v", err)
	}
	vanilla.ApplyContentIDs(reg, ids)

	// Big-endian: writeInt(2) = 00 00 00 02, writeInt(0) = 00 00 00 00.
	const wantHeader = "\x00\x00\x00\x02\x00\x00\x00\x00"

	check := func(label string, payload []byte) {
		if len(payload) == 0 {
			t.Fatalf("%s: empty payload", label)
		}
		zr, zerr := zlib.NewReader(bytes.NewReader(payload))
		if zerr != nil {
			t.Fatalf("%s: zlib reader: %v", label, zerr)
		}
		raw, rerr := io.ReadAll(zr)
		_ = zr.Close()
		if rerr != nil {
			t.Fatalf("%s: read zlib: %v", label, rerr)
		}
		if len(raw) < 8 {
			t.Fatalf("%s: decompressed payload too short (%d bytes)", label, len(raw))
		}
		got := string(raw[:8])
		if got != wantHeader {
			t.Fatalf("%s: data-patches header = %x, want %x (raw[:8]=%x)", label, []byte(got), []byte(wantHeader), raw[:8])
		}
		if _, ierr := InspectWorldStreamPayload(payload); ierr != nil {
			t.Fatalf("%s: inspect after header fix: %v", label, ierr)
		}
		t.Logf("%s: OK header=%x rawlen=%d", label, raw[:8], len(raw))
	}

	// 1) Raw MSAV byte-copy path.
	rawPath := "../../assets/worlds/23157.msav"
	rawPayload, err := BuildWorldStreamFromMSAV(rawPath)
	if err != nil {
		t.Fatalf("BuildWorldStreamFromMSAV: %v", err)
	}
	check("raw-msav", rawPayload)

	// 2) Registry-aware model rebuild path (the production join path in main.go).
	model, err := LoadWorldModelFromMSAV(rawPath, reg)
	if err != nil {
		t.Fatalf("LoadWorldModelFromMSAV(reg): %v", err)
	}
	modelPayload, err := BuildWorldStreamFromModel(model, 1)
	if err != nil {
		t.Fatalf("BuildWorldStreamFromModel: %v", err)
	}
	check("model-rebuild", modelPayload)

	// 3) Snapshot path also must carry the header.
	snapPayload, err := BuildWorldStreamFromModelSnapshot(model, 1, world.Snapshot{})
	if err != nil {
		t.Fatalf("BuildWorldStreamFromModelSnapshot: %v", err)
	}
	check("model-snapshot", snapPayload)

	// 4) Rewrite helpers (used by core.rewriteWorldStreamPayload) must PRESERVE
	// the leading data-patches header, otherwise the client's readDataPatches
	// would misread the rewritten payload and abort.
	ruleRewritten, err := RewriteRulesInWorldStream(rawPayload, `{"spawn":1}`)
	if err != nil {
		t.Fatalf("RewriteRulesInWorldStream: %v", err)
	}
	check("rewrite-rules", ruleRewritten)

	stateRewritten, err := RewriteRuntimeStateInWorldStream(rawPayload, 7, 123.5, 999.25, 3)
	if err != nil {
		t.Fatalf("RewriteRuntimeStateInWorldStream: %v", err)
	}
	check("rewrite-runtime", stateRewritten)

	playerRewritten, err := RewritePlayerIDInWorldStream(rawPayload, 4)
	if err != nil {
		t.Fatalf("RewritePlayerIDInWorldStream: %v", err)
	}
	check("rewrite-playerid", playerRewritten)
}
