package worldstream

import (
	"bytes"
	"compress/zlib"
	"encoding/hex"
	"fmt"
	"io"
)

type WorldStreamInspection struct {
	CompressedLen int
	RawLen        int
	PlayerStart   int
	PlayerEnd     int
	ContentStart  int
	ContentEnd    int
	PatchesEnd    int
	MapEnd        int
	TeamBlocksLen int
	TailLen       int
	TailPrefixHex string
}

func InspectWorldStreamPayload(payload []byte) (WorldStreamInspection, error) {
	inspection := WorldStreamInspection{CompressedLen: len(payload)}

	zr, err := zlib.NewReader(bytes.NewReader(payload))
	if err != nil {
		return inspection, fmt.Errorf("zlib reader: %w", err)
	}
	raw, err := io.ReadAll(zr)
	_ = zr.Close()
	if err != nil {
		return inspection, fmt.Errorf("read zlib payload: %w", err)
	}
	inspection.RawLen = len(raw)

	playerStart, err := locatePlayerStart(raw)
	if err != nil {
		return inspection, fmt.Errorf("locate player start: %w", err)
	}
	inspection.PlayerStart = playerStart

	r := newJavaReader(raw[playerStart:])
	playerRev, err := r.ReadInt16()
	if err != nil {
		return inspection, fmt.Errorf("read player revision: %w", err)
	}
	if err := skipPlayerPayload(r, playerRev); err != nil {
		return inspection, fmt.Errorf("skip player payload: %w", err)
	}
	inspection.PlayerEnd = playerStart + r.Offset()
	inspection.ContentStart = inspection.PlayerEnd

	contentEnd, patchesEnd, mapEnd, teamBlocksLen, markersLen, customLen, _, err := inspectWorldSections(raw, inspection.ContentStart)
	if err != nil {
		return inspection, err
	}
	inspection.ContentEnd = contentEnd
	inspection.PatchesEnd = patchesEnd
	inspection.MapEnd = mapEnd
	inspection.TeamBlocksLen = teamBlocksLen
	inspection.TailLen = len(raw) - inspection.MapEnd - inspection.TeamBlocksLen - markersLen - customLen
	if inspection.TailLen < 0 {
		return inspection, fmt.Errorf("negative tail length")
	}
	if inspection.TailLen > 0 {
		tail := raw[len(raw)-inspection.TailLen:]
		prefixLen := len(tail)
		if prefixLen > 16 {
			prefixLen = 16
		}
		inspection.TailPrefixHex = hex.EncodeToString(tail[:prefixLen])
	}
	return inspection, nil
}
