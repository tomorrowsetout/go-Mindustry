package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"mdt-server/internal/protocol"
	"mdt-server/internal/vanilla"
	"mdt-server/internal/worldstream"
)

type TileStats struct {
	FloorIDs   map[int16]int `json:"floor_ids"`
	OverlayIDs map[int16]int `json:"overlay_ids"`
	BlockIDs   map[int16]int `json:"block_ids"`
	TotalTiles int           `json:"total_tiles"`
}

type MapReport struct {
	Path           string            `json:"path"`
	Error          string            `json:"error,omitempty"`
	WithRemap      *TileStats        `json:"with_remap,omitempty"`
	WithoutRemap   *TileStats        `json:"without_remap,omitempty"`
	RemappedHeader map[string]string `json:"remapped_header_blocks,omitempty"`
	OriginalHeader map[string]string `json:"original_header_blocks,omitempty"`
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintf(os.Stderr, "usage: %s <content_ids.json> <worlds_dir>\n", os.Args[0])
		os.Exit(1)
	}
	contentIDsPath := os.Args[1]
	worldsDir := os.Args[2]

	ids, err := vanilla.LoadContentIDs(contentIDsPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "LoadContentIDs: %v\n", err)
		os.Exit(1)
	}

	reg := protocol.NewContentRegistry()
	vanilla.ApplyContentIDs(reg, ids)

	entries, err := os.ReadDir(worldsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ReadDir: %v\n", err)
		os.Exit(1)
	}

	var maps []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".msav") {
			maps = append(maps, filepath.Join(worldsDir, e.Name()))
		}
	}
	sort.Strings(maps)

	reports := make([]MapReport, 0, len(maps))
	for _, mp := range maps {
		rpt := MapReport{Path: filepath.Base(mp)}

		modelR, errR := worldstream.LoadWorldModelFromMSAV(mp, reg)
		if errR != nil {
			rpt.Error = fmt.Sprintf("with_remap error: %v", errR)
		} else {
			stats := &TileStats{
				FloorIDs:   make(map[int16]int),
				OverlayIDs: make(map[int16]int),
				BlockIDs:   make(map[int16]int),
				TotalTiles: len(modelR.Tiles),
			}
			for _, t := range modelR.Tiles {
				stats.FloorIDs[int16(t.Floor)]++
				stats.OverlayIDs[int16(t.Overlay)]++
				stats.BlockIDs[int16(t.Block)]++
			}
			rpt.WithRemap = stats
			if hdr, herr := extractBlockHeader(modelR.Content); herr == nil {
				rpt.RemappedHeader = hdr
			}
		}

		modelO, errO := worldstream.LoadWorldModelFromMSAV(mp, nil)
		if errO != nil {
			if rpt.Error == "" {
				rpt.Error = fmt.Sprintf("without_remap error: %v", errO)
			}
		} else {
			stats := &TileStats{
				FloorIDs:   make(map[int16]int),
				OverlayIDs: make(map[int16]int),
				BlockIDs:   make(map[int16]int),
				TotalTiles: len(modelO.Tiles),
			}
			for _, t := range modelO.Tiles {
				stats.FloorIDs[int16(t.Floor)]++
				stats.OverlayIDs[int16(t.Overlay)]++
				stats.BlockIDs[int16(t.Block)]++
			}
			rpt.WithoutRemap = stats
			if hdr, herr := extractBlockHeader(modelO.Content); herr == nil {
				rpt.OriginalHeader = hdr
			}
		}

		reports = append(reports, rpt)
	}

	out, _ := json.MarshalIndent(reports, "", "  ")
	fmt.Println(string(out))
}

func extractBlockHeader(content []byte) (map[string]string, error) {
	if len(content) == 0 {
		return nil, fmt.Errorf("empty content")
	}
	off := 0
	readByte := func() (byte, error) {
		if off >= len(content) {
			return 0, fmt.Errorf("EOF")
		}
		b := content[off]
		off++
		return b, nil
	}
	readInt16 := func() (int16, error) {
		if off+2 > len(content) {
			return 0, fmt.Errorf("EOF")
		}
		v := int16(int(content[off])<<8 | int(content[off+1]))
		off += 2
		return v, nil
	}
	readUTF := func() (string, error) {
		n, err := readInt16()
		if err != nil {
			return "", err
		}
		if n < 0 || off+int(n) > len(content) {
			return "", fmt.Errorf("bad UTF len %d", n)
		}
		s := string(content[off : off+int(n)])
		off += int(n)
		return s, nil
	}

	mapped, err := readByte()
	if err != nil {
		return nil, err
	}
	out := make(map[string]string)
	for i := 0; i < int(mapped); i++ {
		ct, err := readByte()
		if err != nil {
			return nil, err
		}
		total, err := readInt16()
		if err != nil {
			return nil, err
		}
		for id := int16(0); id < total; id++ {
			name, err := readUTF()
			if err != nil {
				return nil, err
			}
			if ct == 1 {
				out[fmt.Sprintf("%d", id)] = strings.ToLower(strings.TrimSpace(name))
			}
		}
	}
	return out, nil
}
