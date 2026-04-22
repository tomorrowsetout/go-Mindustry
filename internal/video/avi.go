package video

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/jpeg"
	"os"
)

type AVIWriter struct {
	f            *os.File
	width        int
	height       int
	fps          int
	jpegQuality  int
	totalFrames  uint32
	maxFrameSize uint32
	moviBytes    uint32

	riffSizePos         int64
	avihMaxBytesPos     int64
	avihTotalFramesPos  int64
	avihSuggestedBufPos int64
	strhLengthPos       int64
	strhSuggestedBufPos int64
	moviSizePos         int64
	moviTypePos         int64

	index []aviIndexEntry
}

type aviIndexEntry struct {
	offset uint32
	size   uint32
}

func NewAVIWriter(path string, width, height, fps, jpegQuality int) (*AVIWriter, error) {
	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	w := &AVIWriter{
		f:           f,
		width:       width,
		height:      height,
		fps:         fps,
		jpegQuality: jpegQuality,
	}
	if err := w.writeHeaders(); err != nil {
		_ = f.Close()
		return nil, err
	}
	return w, nil
}

func (w *AVIWriter) AddFrame(img image.Image) error {
	if w == nil || w.f == nil {
		return os.ErrInvalid
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: w.jpegQuality}); err != nil {
		return err
	}
	data := buf.Bytes()
	size := uint32(len(data))
	if size > w.maxFrameSize {
		w.maxFrameSize = size
	}

	chunkStart, err := w.f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if err := writeFourCC(w.f, "00dc"); err != nil {
		return err
	}
	if err := writeU32(w.f, size); err != nil {
		return err
	}
	if _, err := w.f.Write(data); err != nil {
		return err
	}
	pad := uint32(0)
	if size%2 != 0 {
		if _, err := w.f.Write([]byte{0}); err != nil {
			return err
		}
		pad = 1
	}

	w.index = append(w.index, aviIndexEntry{
		offset: uint32(chunkStart - w.moviTypePos),
		size:   size,
	})
	w.moviBytes += 8 + size + pad
	w.totalFrames++
	return nil
}

func (w *AVIWriter) Close() error {
	if w == nil || w.f == nil {
		return nil
	}

	if err := writeFourCC(w.f, "idx1"); err != nil {
		return err
	}
	if err := writeU32(w.f, uint32(len(w.index))*16); err != nil {
		return err
	}
	for _, idx := range w.index {
		if err := writeFourCC(w.f, "00dc"); err != nil {
			return err
		}
		if err := writeU32(w.f, 0x10); err != nil {
			return err
		}
		if err := writeU32(w.f, idx.offset); err != nil {
			return err
		}
		if err := writeU32(w.f, idx.size); err != nil {
			return err
		}
	}

	fileSize, err := w.f.Seek(0, os.SEEK_END)
	if err != nil {
		return err
	}
	if err := patchU32(w.f, w.riffSizePos, uint32(fileSize-8)); err != nil {
		return err
	}
	if err := patchU32(w.f, w.moviSizePos, 4+w.moviBytes); err != nil {
		return err
	}
	if err := patchU32(w.f, w.avihMaxBytesPos, w.maxFrameSize*uint32(w.fps)); err != nil {
		return err
	}
	if err := patchU32(w.f, w.avihTotalFramesPos, w.totalFrames); err != nil {
		return err
	}
	if err := patchU32(w.f, w.avihSuggestedBufPos, w.maxFrameSize); err != nil {
		return err
	}
	if err := patchU32(w.f, w.strhLengthPos, w.totalFrames); err != nil {
		return err
	}
	if err := patchU32(w.f, w.strhSuggestedBufPos, w.maxFrameSize); err != nil {
		return err
	}

	err = w.f.Close()
	w.f = nil
	return err
}

func (w *AVIWriter) writeHeaders() error {
	if err := writeFourCC(w.f, "RIFF"); err != nil {
		return err
	}
	var err error
	w.riffSizePos, err = w.f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeFourCC(w.f, "AVI "); err != nil {
		return err
	}

	if err := writeFourCC(w.f, "LIST"); err != nil {
		return err
	}
	if err := writeU32(w.f, 192); err != nil {
		return err
	}
	if err := writeFourCC(w.f, "hdrl"); err != nil {
		return err
	}

	if err := writeFourCC(w.f, "avih"); err != nil {
		return err
	}
	if err := writeU32(w.f, 56); err != nil {
		return err
	}
	if err := writeU32(w.f, uint32(1000000/w.fps)); err != nil {
		return err
	}
	w.avihMaxBytesPos, err = w.f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeU32(w.f, 0x10); err != nil {
		return err
	}
	w.avihTotalFramesPos, err = w.f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeU32(w.f, 1); err != nil {
		return err
	}
	w.avihSuggestedBufPos, err = w.f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeU32(w.f, uint32(w.width)); err != nil {
		return err
	}
	if err := writeU32(w.f, uint32(w.height)); err != nil {
		return err
	}
	for i := 0; i < 4; i++ {
		if err := writeU32(w.f, 0); err != nil {
			return err
		}
	}

	if err := writeFourCC(w.f, "LIST"); err != nil {
		return err
	}
	if err := writeU32(w.f, 116); err != nil {
		return err
	}
	if err := writeFourCC(w.f, "strl"); err != nil {
		return err
	}

	if err := writeFourCC(w.f, "strh"); err != nil {
		return err
	}
	if err := writeU32(w.f, 56); err != nil {
		return err
	}
	if err := writeFourCC(w.f, "vids"); err != nil {
		return err
	}
	if err := writeFourCC(w.f, "MJPG"); err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeU16(w.f, 0); err != nil {
		return err
	}
	if err := writeU16(w.f, 0); err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeU32(w.f, 1); err != nil {
		return err
	}
	if err := writeU32(w.f, uint32(w.fps)); err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	w.strhLengthPos, err = w.f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	w.strhSuggestedBufPos, err = w.f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeU32(w.f, 0xFFFFFFFF); err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeI16(w.f, 0); err != nil {
		return err
	}
	if err := writeI16(w.f, 0); err != nil {
		return err
	}
	if err := writeI16(w.f, int16(w.width)); err != nil {
		return err
	}
	if err := writeI16(w.f, int16(w.height)); err != nil {
		return err
	}

	if err := writeFourCC(w.f, "strf"); err != nil {
		return err
	}
	if err := writeU32(w.f, 40); err != nil {
		return err
	}
	if err := writeU32(w.f, 40); err != nil {
		return err
	}
	if err := writeI32(w.f, int32(w.width)); err != nil {
		return err
	}
	if err := writeI32(w.f, int32(w.height)); err != nil {
		return err
	}
	if err := writeU16(w.f, 1); err != nil {
		return err
	}
	if err := writeU16(w.f, 24); err != nil {
		return err
	}
	if err := writeFourCC(w.f, "MJPG"); err != nil {
		return err
	}
	if err := writeU32(w.f, uint32(w.width*w.height*3)); err != nil {
		return err
	}
	if err := writeI32(w.f, 0); err != nil {
		return err
	}
	if err := writeI32(w.f, 0); err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}

	if err := writeFourCC(w.f, "LIST"); err != nil {
		return err
	}
	w.moviSizePos, err = w.f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	if err := writeU32(w.f, 0); err != nil {
		return err
	}
	w.moviTypePos, err = w.f.Seek(0, os.SEEK_CUR)
	if err != nil {
		return err
	}
	return writeFourCC(w.f, "movi")
}

func writeFourCC(w *os.File, s string) error {
	_, err := w.Write([]byte(s))
	return err
}

func writeU32(w *os.File, v uint32) error {
	return binary.Write(w, binary.LittleEndian, v)
}

func writeU16(w *os.File, v uint16) error {
	return binary.Write(w, binary.LittleEndian, v)
}

func writeI32(w *os.File, v int32) error {
	return binary.Write(w, binary.LittleEndian, v)
}

func writeI16(w *os.File, v int16) error {
	return binary.Write(w, binary.LittleEndian, v)
}

func patchU32(f *os.File, pos int64, v uint32) error {
	var buf [4]byte
	binary.LittleEndian.PutUint32(buf[:], v)
	_, err := f.WriteAt(buf[:], pos)
	return err
}
