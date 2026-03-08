package main

import (
	"bytes"
	"compress/zlib"
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

type JavaReader struct {
	buf *bytes.Reader
}

func newJavaReader(data []byte) *JavaReader {
	return &JavaReader{buf: bytes.NewReader(data)}
}

func (r *JavaReader) ReadByte() (byte, error) {
	return r.buf.ReadByte()
}

func (r *JavaReader) ReadBytes(n int) ([]byte, error) {
	data := make([]byte, n)
	_, err := io.ReadFull(r.buf, data)
	return data, err
}

func (r *JavaReader) ReadInt16() (int16, error) {
	var v int16
	err := binary.Read(r.buf, binary.BigEndian, &v)
	return v, err
}

func (r *JavaReader) ReadInt32() (int32, error) {
	var v int32
	err := binary.Read(r.buf, binary.BigEndian, &v)
	return v, err
}

func (r *JavaReader) ReadUTF() (string, error) {
	length, err := r.ReadInt16()
	if err != nil {
		return "", err
	}
	if length < 0 {
		return "", fmt.Errorf("invalid UTF length: %d", length)
	}
	data := make([]byte, length)
	_, err = io.ReadFull(r.buf, data)
	if err != nil {
		return "", err
	}
	
	// Decode modified UTF-8
	var result []rune
	for i := 0; i < len(data); {
		b := data[i]
		if b&0x80 == 0 {
			result = append(result, rune(b))
			i++
		} else if b&0xE0 == 0xC0 {
			if i+1 >= len(data) {
				break
			}
			result = append(result, rune((int(b&0x1F)<<6)|(int(data[i+1]&0x3F))))
			i += 2
		} else if b&0xF0 == 0xE0 {
			if i+2 >= len(data) {
				break
			}
			result = append(result, rune((int(b&0x0F)<<12)|(int(data[i+1]&0x3F)<<6)|(int(data[i+2]&0x3F))))
			i += 3
		} else {
			result = append(result, rune(b))
			i++
		}
	}
	return string(result), nil
}

func (r *JavaReader) ReadChunk() ([]byte, error) {
	length, err := r.ReadInt32()
	if err != nil {
		return nil, err
	}
	if length < 0 {
		return nil, fmt.Errorf("invalid chunk length: %d", length)
	}
	data := make([]byte, length)
	_, err = io.ReadFull(r.buf, data)
	if err != nil {
		return nil, err
	}
	
	// Check for zlib compression (first byte == 0x78)
	if len(data) > 0 && data[0] == 0x78 {
		decompressed, deerr := zlib.NewReader(bytes.NewReader(data))
		if deerr == nil {
			defer decompressed.Close()
			var buf bytes.Buffer
			io.Copy(&buf, decompressed)
			return buf.Bytes(), nil
		}
	}
	
	return data, nil
}

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: debug_msav <path>")
		return
	}

	rawData, err := os.ReadFile(os.Args[1])
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		return
	}

	// Decompress zlib
	zr, err := zlib.NewReader(bytes.NewReader(rawData))
	if err != nil {
		fmt.Printf("Error creating zlib reader: %v\n", err)
		return
	}
	defer zr.Close()

	decompressed, err := io.ReadAll(zr)
	if err != nil {
		fmt.Printf("Error decompressing: %v\n", err)
		return
	}

	fmt.Printf("Decompressed length: %d\n", len(decompressed))

	// Check for MSAV header
	if len(decompressed) < 4 {
		fmt.Println("File too short")
		return
	}
	header := string(decompressed[:4])
	fmt.Printf("Header: %q\n", header)
	if header != "MSAV" {
		fmt.Println("Not a valid MSAV file")
		return
	}

	r := newJavaReader(decompressed[4:]) // Skip "MSAV" header

	// Read version
	version, err := r.ReadInt32()
	if err != nil {
		fmt.Printf("Error reading version: %v\n", err)
		return
	}
	fmt.Printf("Version: %d\n", version)

	// Read meta chunk
	meta, err := r.ReadChunk()
	if err != nil {
		fmt.Printf("Error reading meta: %v\n", err)
		return
	}
	fmt.Printf("Meta chunk length: %d\n", len(meta))

	// Read content chunk
	content, err := r.ReadChunk()
	if err != nil {
		fmt.Printf("Error reading content: %v\n", err)
		return
	}
	fmt.Printf("Content chunk length: %d\n", len(content))

	// Parse content chunk
	cr := newJavaReader(content)
	mapped, err := cr.ReadByte()
	if err != nil {
		fmt.Printf("Error reading mapped: %v\n", err)
		return
	}
	fmt.Printf("Mapped types: %d\n", mapped)

	blockCount := 0
	for i := 0; i < int(mapped); i++ {
		ct, err := cr.ReadByte()
		if err != nil {
			fmt.Printf("Error reading ct[%d]: %v\n", i, err)
			return
		}
		total, err := cr.ReadInt16()
		if err != nil {
			fmt.Printf("Error reading total[%d]: %v\n", i, err)
			return
		}
		fmt.Printf("  Type %d: total=%d\n", ct, total)

		if ct == 1 { // ContentType.block
			for id := int16(0); id < total; id++ {
				name, err := cr.ReadUTF()
				if err != nil {
					fmt.Printf("    Error reading name[%d]: %v\n", id, err)
					return
				}
				if id == 234 || id == 257 || id == 266 || id < 30 {
					fmt.Printf("    id=%d name=%s\n", id, name)
				}
				blockCount++
			}
		} else {
			for id := int16(0); id < total; id++ {
				_, err := cr.ReadUTF()
				if err != nil {
					fmt.Printf("    Error reading name[%d][%d]: %v\n", ct, id, err)
					return
				}
			}
		}
	}

	fmt.Printf("Total blocks: %d\n", blockCount)
}
