package protocol

import (
	"strings"
	"testing"
)

func TestReadObjectRejectsOversizedByteArray(t *testing.T) {
	w := NewWriter()
	if err := w.WriteByte(14); err != nil {
		t.Fatalf("write object type: %v", err)
	}
	if err := w.WriteInt32(typeIOMaxByteArraySize + 1); err != nil {
		t.Fatalf("write byte array length: %v", err)
	}

	_, err := ReadObject(NewReader(w.Bytes()), false, nil)
	if err == nil || !strings.Contains(err.Error(), "byte array length too large") {
		t.Fatalf("expected oversized byte array error, got %v", err)
	}
}

func TestReadObjectRejectsOversizedObjectArray(t *testing.T) {
	w := NewWriter()
	if err := w.WriteByte(22); err != nil {
		t.Fatalf("write object type: %v", err)
	}
	if err := w.WriteInt32(typeIOReadMaxArraySize + 1); err != nil {
		t.Fatalf("write object array length: %v", err)
	}

	_, err := ReadObject(NewReader(w.Bytes()), false, nil)
	if err == nil || !strings.Contains(err.Error(), "object array length too large") {
		t.Fatalf("expected oversized object array error, got %v", err)
	}
}

func TestReadObjectRejectsNegativeArrayLength(t *testing.T) {
	w := NewWriter()
	if err := w.WriteByte(16); err != nil {
		t.Fatalf("write object type: %v", err)
	}
	if err := w.WriteInt32(-1); err != nil {
		t.Fatalf("write bool array length: %v", err)
	}

	_, err := ReadObject(NewReader(w.Bytes()), false, nil)
	if err == nil || !strings.Contains(err.Error(), "bool array length is negative") {
		t.Fatalf("expected negative bool array error, got %v", err)
	}
}

func TestReadObjectRejectsNestedArrays(t *testing.T) {
	w := NewWriter()
	if err := WriteObject(w, []any{[]bool{true}}, nil); err != nil {
		t.Fatalf("write nested object array: %v", err)
	}

	_, err := ReadObject(NewReader(w.Bytes()), false, nil)
	if err == nil || !strings.Contains(err.Error(), "nested arrays are not allowed") {
		t.Fatalf("expected nested array rejection, got %v", err)
	}
}

func TestWriteObjectRejectsOversizedArrays(t *testing.T) {
	if err := WriteObject(NewWriter(), make([]byte, typeIOMaxByteArraySize+1), nil); err == nil {
		t.Fatal("expected oversized byte array write to fail")
	}
	if err := WriteObject(NewWriter(), make([]int32, typeIOWriteMaxArraySize+1), nil); err == nil {
		t.Fatal("expected oversized int array write to fail")
	}
}
