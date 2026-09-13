package worldstream

import (
	"bytes"
	"compress/zlib"
	"os"
)

// WriteMSAVSnapshot rewrites an existing .msav file with updated tag values,
// preserving content/map/entities/markers/custom chunks for compatibility.
func WriteMSAVSnapshot(srcPath, dstPath string, updates map[string]string) error {
	data, err := readMSAV(srcPath)
	if err != nil {
		return err
	}
	tags := make(map[string]string, len(data.Tags)+len(updates))
	for k, v := range data.Tags {
		tags[k] = v
	}
	for k, v := range updates {
		tags[k] = v
	}

	var meta bytes.Buffer
	metaWriter := &javaWriter{buf: &meta}
	if err := metaWriter.WriteStringMap(tags); err != nil {
		return err
	}

	var raw bytes.Buffer
	if _, err := raw.Write([]byte("MSAV")); err != nil {
		return err
	}
	if err := writeBE(&raw, data.Version); err != nil {
		return err
	}
	if err := writeChunk(&raw, meta.Bytes()); err != nil {
		return err
	}
	if err := writeChunk(&raw, data.Content); err != nil {
		return err
	}
	if data.Version >= 11 {
		if err := writeChunk(&raw, data.Patches); err != nil {
			return err
		}
	}
	if err := writeChunk(&raw, data.Map); err != nil {
		return err
	}
	if err := writeChunk(&raw, data.RawEntities); err != nil {
		return err
	}
	if data.Version >= 8 {
		if err := writeChunk(&raw, data.Markers); err != nil {
			return err
		}
	}
	if data.Version >= 7 {
		if err := writeChunk(&raw, data.Custom); err != nil {
			return err
		}
	}

	var out bytes.Buffer
	zw := zlib.NewWriter(&out)
	if _, err := zw.Write(raw.Bytes()); err != nil {
		_ = zw.Close()
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}
	return os.WriteFile(dstPath, out.Bytes(), 0644)
}

func writeChunk(w *bytes.Buffer, payload []byte) error {
	if payload == nil {
		payload = []byte{}
	}
	if err := writeBE(w, int32(len(payload))); err != nil {
		return err
	}
	_, err := w.Write(payload)
	return err
}
