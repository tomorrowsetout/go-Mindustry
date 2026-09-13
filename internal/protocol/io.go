package protocol

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
)

var charset = binary.BigEndian

type Reader struct {
	r   *bytes.Reader
	Ctx *TypeIOContext
}

func NewReader(b []byte) *Reader {
	return &Reader{r: bytes.NewReader(b)}
}

func NewReaderWithContext(b []byte, ctx *TypeIOContext) *Reader {
	return &Reader{r: bytes.NewReader(b), Ctx: ctx}
}

func (rd *Reader) Remaining() int {
	if rd == nil || rd.r == nil {
		return 0
	}
	return rd.r.Len()
}

func (rd *Reader) ReadByte() (byte, error) {
	if rd == nil || rd.r == nil {
		return 0, io.ErrUnexpectedEOF
	}
	return rd.r.ReadByte()
}

func (rd *Reader) ReadUByte() (uint8, error) {
	b, err := rd.ReadByte()
	return uint8(b), err
}

func (rd *Reader) ReadBool() (bool, error) {
	b, err := rd.ReadByte()
	if err != nil {
		return false, err
	}
	return b == 1, nil
}

func (rd *Reader) ReadInt16() (int16, error) {
	if rd == nil || rd.r == nil {
		return 0, io.ErrUnexpectedEOF
	}
	var buf [2]byte
	if _, err := io.ReadFull(rd.r, buf[:]); err != nil {
		return 0, err
	}
	return int16(charset.Uint16(buf[:])), nil
}

func (rd *Reader) ReadUint16() (uint16, error) {
	if rd == nil || rd.r == nil {
		return 0, io.ErrUnexpectedEOF
	}
	var buf [2]byte
	if _, err := io.ReadFull(rd.r, buf[:]); err != nil {
		return 0, err
	}
	return charset.Uint16(buf[:]), nil
}

func (rd *Reader) ReadInt32() (int32, error) {
	if rd == nil || rd.r == nil {
		return 0, io.ErrUnexpectedEOF
	}
	var buf [4]byte
	if _, err := io.ReadFull(rd.r, buf[:]); err != nil {
		return 0, err
	}
	return int32(charset.Uint32(buf[:])), nil
}

func (rd *Reader) ReadInt64() (int64, error) {
	if rd == nil || rd.r == nil {
		return 0, io.ErrUnexpectedEOF
	}
	var buf [8]byte
	if _, err := io.ReadFull(rd.r, buf[:]); err != nil {
		return 0, err
	}
	return int64(charset.Uint64(buf[:])), nil
}

func (rd *Reader) ReadFloat32() (float32, error) {
	if rd == nil || rd.r == nil {
		return 0, io.ErrUnexpectedEOF
	}
	var buf [4]byte
	if _, err := io.ReadFull(rd.r, buf[:]); err != nil {
		return 0, err
	}
	v := charset.Uint32(buf[:])
	return math.Float32frombits(v), nil
}

func (rd *Reader) ReadFloat64() (float64, error) {
	if rd == nil || rd.r == nil {
		return 0, io.ErrUnexpectedEOF
	}
	var buf [8]byte
	if _, err := io.ReadFull(rd.r, buf[:]); err != nil {
		return 0, err
	}
	v := charset.Uint64(buf[:])
	return math.Float64frombits(v), nil
}

func (rd *Reader) ReadBytes(n int) ([]byte, error) {
	if rd == nil || rd.r == nil {
		return nil, io.ErrUnexpectedEOF
	}
	buf := make([]byte, n)
	_, err := io.ReadFull(rd.r, buf)
	return buf, err
}

func (rd *Reader) ReadStringNullable() (*string, error) {
	exists, err := rd.ReadByte()
	if err != nil {
		return nil, err
	}
	if exists == 0 {
		return nil, nil
	}
	s, err := rd.ReadStringRaw()
	if err != nil {
		return nil, err
	}
	return &s, nil
}

func (rd *Reader) ReadStringRaw() (string, error) {
	// Arc Writes.str writes length as short, then bytes (UTF-8)
	l, err := rd.ReadUint16()
	if err != nil {
		return "", err
	}
	if l == 0xFFFF {
		return "", nil
	}
	b, err := rd.ReadBytes(int(l))
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type Writer struct {
	buf []byte
	Ctx *TypeIOContext
}

func NewWriter() *Writer {
	return &Writer{}
}

func NewWriterWithCapacity(capacity int) *Writer {
	if capacity <= 0 {
		return &Writer{}
	}
	return &Writer{buf: make([]byte, 0, capacity)}
}

func NewWriterWithContext(ctx *TypeIOContext) *Writer {
	return &Writer{Ctx: ctx}
}

func NewWriterWithContextCapacity(ctx *TypeIOContext, capacity int) *Writer {
	if capacity <= 0 {
		return &Writer{Ctx: ctx}
	}
	return &Writer{buf: make([]byte, 0, capacity), Ctx: ctx}
}

func (w *Writer) Bytes() []byte {
	return w.buf
}

func (w *Writer) Reset() {
	w.buf = w.buf[:0]
}

func (w *Writer) WriteByte(v byte) error {
	w.buf = append(w.buf, v)
	return nil
}

func (w *Writer) WriteBool(v bool) error {
	if v {
		return w.WriteByte(1)
	}
	return w.WriteByte(0)
}

func (w *Writer) WriteInt16(v int16) error {
	var buf [2]byte
	charset.PutUint16(buf[:], uint16(v))
	w.buf = append(w.buf, buf[:]...)
	return nil
}

func (w *Writer) WriteUint16(v uint16) error {
	var buf [2]byte
	charset.PutUint16(buf[:], v)
	w.buf = append(w.buf, buf[:]...)
	return nil
}

func (w *Writer) WriteInt32(v int32) error {
	var buf [4]byte
	charset.PutUint32(buf[:], uint32(v))
	w.buf = append(w.buf, buf[:]...)
	return nil
}

func (w *Writer) WriteInt64(v int64) error {
	var buf [8]byte
	charset.PutUint64(buf[:], uint64(v))
	w.buf = append(w.buf, buf[:]...)
	return nil
}

func (w *Writer) WriteFloat32(v float32) error {
	var buf [4]byte
	charset.PutUint32(buf[:], math.Float32bits(v))
	w.buf = append(w.buf, buf[:]...)
	return nil
}

func (w *Writer) WriteFloat64(v float64) error {
	var buf [8]byte
	charset.PutUint64(buf[:], math.Float64bits(v))
	w.buf = append(w.buf, buf[:]...)
	return nil
}

func (w *Writer) WriteBytes(b []byte) error {
	w.buf = append(w.buf, b...)
	return nil
}

func (w *Writer) WriteStringNullable(s *string) error {
	if s == nil {
		return w.WriteByte(0)
	}
	if err := w.WriteByte(1); err != nil {
		return err
	}
	return w.WriteStringRaw(*s)
}

func (w *Writer) WriteStringRaw(s string) error {
	if s == "" {
		if err := w.WriteUint16(0); err != nil {
			return err
		}
		return nil
	}
	if err := w.WriteUint16(uint16(len(s))); err != nil {
		return err
	}
	w.buf = append(w.buf, s...)
	return nil
}
