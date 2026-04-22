package protocol

import "fmt"

// CloneObjectValue converts a TypeIO-compatible object into a detached, stable
// value form so it can be queued across goroutines safely.
func CloneObjectValue(obj any) (cloned any, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			cloned = nil
			err = fmt.Errorf("panic cloning %T: %v", obj, rec)
		}
	}()
	switch v := obj.(type) {
	case nil:
		return nil, nil
	case int:
		obj = int32(v)
	case UnitStance, Effect, Sound:
		// These values are safe-by-value and are not encoded through WriteObject today.
		return v, nil
	}

	writer := NewWriter()
	if err := WriteObject(writer, obj, nil); err != nil {
		return nil, err
	}
	return ReadObject(NewReader(writer.Bytes()), false, nil)
}
