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
	return rd.r.Len()
}

func (rd *Reader) ReadByte() (byte, error) {
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
	var v int16
	err := binary.Read(rd.r, charset, &v)
	return v, err
}

func (rd *Reader) ReadUint16() (uint16, error) {
	var v uint16
	err := binary.Read(rd.r, charset, &v)
	return v, err
}

func (rd *Reader) ReadInt32() (int32, error) {
	var v int32
	err := binary.Read(rd.r, charset, &v)
	return v, err
}

func (rd *Reader) ReadInt64() (int64, error) {
	var v int64
	err := binary.Read(rd.r, charset, &v)
	return v, err
}

func (rd *Reader) ReadFloat32() (float32, error) {
	var v uint32
	if err := binary.Read(rd.r, charset, &v); err != nil {
		return 0, err
	}
	return math.Float32frombits(v), nil
}

func (rd *Reader) ReadFloat64() (float64, error) {
	var v uint64
	if err := binary.Read(rd.r, charset, &v); err != nil {
		return 0, err
	}
	return math.Float64frombits(v), nil
}

func (rd *Reader) ReadBytes(n int) ([]byte, error) {
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
	b   *bytes.Buffer
	Ctx *TypeIOContext
}

func NewWriter() *Writer {
	return &Writer{b: &bytes.Buffer{}}
}

func NewWriterWithContext(ctx *TypeIOContext) *Writer {
	return &Writer{b: &bytes.Buffer{}, Ctx: ctx}
}
func (w *Writer) Bytes() []byte {
	return w.b.Bytes()
}

func (w *Writer) WriteByte(v byte) error {
	return w.b.WriteByte(v)
}

func (w *Writer) WriteBool(v bool) error {
	if v {
		return w.WriteByte(1)
	}
	return w.WriteByte(0)
}

func (w *Writer) WriteInt16(v int16) error {
	return binary.Write(w.b, charset, v)
}

func (w *Writer) WriteUint16(v uint16) error {
	return binary.Write(w.b, charset, v)
}

func (w *Writer) WriteInt32(v int32) error {
	return binary.Write(w.b, charset, v)
}

func (w *Writer) WriteInt64(v int64) error {
	return binary.Write(w.b, charset, v)
}

func (w *Writer) WriteFloat32(v float32) error {
	return binary.Write(w.b, charset, math.Float32bits(v))
}

func (w *Writer) WriteFloat64(v float64) error {
	return binary.Write(w.b, charset, math.Float64bits(v))
}

func (w *Writer) WriteBytes(b []byte) error {
	_, err := w.b.Write(b)
	return err
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
	return w.WriteBytes([]byte(s))
}
