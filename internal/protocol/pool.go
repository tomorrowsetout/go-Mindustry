package protocol

import "sync"

// maxPooledWriterCap bounds re-pooled writers so a single huge payload cannot
// pin a multi-megabyte buffer inside the pool forever.
const maxPooledWriterCap = 1 << 20

var writerPool = sync.Pool{
	New: func() any { return &Writer{} },
}

// GetWriter returns a pooled Writer bound to ctx. Callers must return it via
// PutWriter once the serialized bytes are no longer needed.
func GetWriter(ctx *TypeIOContext) *Writer {
	w := writerPool.Get().(*Writer)
	w.Reset()
	w.Ctx = ctx
	return w
}

// PutWriter returns a Writer to the pool. Oversized buffers are dropped so
// the garbage collector can reclaim them.
func PutWriter(w *Writer) {
	if w == nil || cap(w.buf) > maxPooledWriterCap {
		return
	}
	w.Reset()
	w.Ctx = nil
	writerPool.Put(w)
}
