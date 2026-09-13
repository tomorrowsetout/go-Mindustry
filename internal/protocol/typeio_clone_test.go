package protocol

import "testing"

func TestCloneObjectValueDetachesPointSlice(t *testing.T) {
	original := []Point2{{X: 6, Y: 0}, {X: -6, Y: 0}}
	clonedValue, err := CloneObjectValue(original)
	if err != nil {
		t.Fatalf("clone []Point2: %v", err)
	}
	cloned, ok := clonedValue.([]Point2)
	if !ok {
		t.Fatalf("expected []Point2 clone, got %T", clonedValue)
	}
	if len(cloned) != len(original) {
		t.Fatalf("expected %d points, got %d", len(original), len(cloned))
	}
	cloned[0] = Point2{X: 99, Y: 99}
	if original[0].X != 6 || original[0].Y != 0 {
		t.Fatalf("expected original slice to stay unchanged, got %+v", original[0])
	}
}

func TestCloneObjectValueCopiesBytes(t *testing.T) {
	original := []byte{1, 2, 3, 4}
	clonedValue, err := CloneObjectValue(original)
	if err != nil {
		t.Fatalf("clone []byte: %v", err)
	}
	cloned, ok := clonedValue.([]byte)
	if !ok {
		t.Fatalf("expected []byte clone, got %T", clonedValue)
	}
	if len(cloned) != len(original) {
		t.Fatalf("expected %d bytes, got %d", len(original), len(cloned))
	}
	cloned[0] = 9
	if original[0] != 1 {
		t.Fatalf("expected original bytes to stay unchanged, got %v", original)
	}
}
