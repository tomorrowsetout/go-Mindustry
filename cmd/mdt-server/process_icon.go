package main

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"os"
	"path/filepath"
	"strings"

	"mdt-server/internal/buildinfo"
)

func applyProcessWindowIcon() {
	icoPath := ensureProcessIconFile()
	if strings.TrimSpace(icoPath) == "" {
		return
	}
	setProcessConsoleIcon(icoPath)
}

func ensureProcessIconFile() string {
	iconICO := strings.TrimSpace(buildinfo.IconICO)
	if iconICO == "" {
		iconICO = "FBF.ico"
	}
	iconPNG := strings.TrimSpace(buildinfo.IconPNG)
	if iconPNG == "" {
		iconPNG = "FBF.png"
	}
	centerDir := buildinfo.CenterDir()
	if centerDir == "" {
		return ""
	}
	icoPath := filepath.Join(centerDir, iconICO)
	if st, err := os.Stat(icoPath); err == nil && !st.IsDir() && st.Size() > 0 {
		return icoPath
	}
	pngPath := filepath.Join(centerDir, iconPNG)
	if err := writeICOFromPNG(pngPath, icoPath); err != nil {
		return ""
	}
	return icoPath
}

func writeICOFromPNG(pngPath, icoPath string) error {
	raw, err := os.ReadFile(pngPath)
	if err != nil {
		return err
	}
	cfg, err := png.DecodeConfig(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	width := cfg.Width
	height := cfg.Height
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}

	var out bytes.Buffer
	_ = binary.Write(&out, binary.LittleEndian, uint16(0))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))

	widthByte := byte(width)
	heightByte := byte(height)
	if width >= 256 {
		widthByte = 0
	}
	if height >= 256 {
		heightByte = 0
	}
	out.WriteByte(widthByte)
	out.WriteByte(heightByte)
	out.WriteByte(0)
	out.WriteByte(0)
	_ = binary.Write(&out, binary.LittleEndian, uint16(1))
	_ = binary.Write(&out, binary.LittleEndian, uint16(32))
	_ = binary.Write(&out, binary.LittleEndian, uint32(len(raw)))
	_ = binary.Write(&out, binary.LittleEndian, uint32(22))
	_, _ = out.Write(raw)

	if err := os.MkdirAll(filepath.Dir(icoPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(icoPath, out.Bytes(), 0o644)
}
