package net

import (
	"bytes"
	"sync"
)

// Size classes for pooled byte buffers. Frame payloads are bounded by the
// 16-bit outer length field, so three classes cover every inbound frame
// without over-allocating.
const (
	bufClassSmall  = 256
	bufClassMedium = 4096
	bufClassLarge  = 1 << 16
)

var (
	smallBufPool  = newBytePool(bufClassSmall)
	mediumBufPool = newBytePool(bufClassMedium)
	largeBufPool  = newBytePool(bufClassLarge)
)

func newBytePool(size int) *sync.Pool {
	return &sync.Pool{
		New: func() any {
			b := make([]byte, size)
			return &b
		},
	}
}

// getPooledBuf returns a buffer with len exactly n from the smallest size
// class that fits. n is expected to stay within the 16-bit frame bound; larger
// requests fall back to a fresh allocation that is simply dropped on put.
func getPooledBuf(n int) []byte {
	if n < 0 {
		n = 0
	}
	switch {
	case n <= bufClassSmall:
		bp := smallBufPool.Get().(*[]byte)
		return (*bp)[:n]
	case n <= bufClassMedium:
		bp := mediumBufPool.Get().(*[]byte)
		return (*bp)[:n]
	case n <= bufClassLarge:
		bp := largeBufPool.Get().(*[]byte)
		return (*bp)[:n]
	default:
		return make([]byte, n)
	}
}

// putPooledBuf returns a buffer to its size-class pool. Buffers whose capacity
// no longer matches a class (e.g. grown by append) are left for the GC.
func putPooledBuf(b []byte) {
	switch cap(b) {
	case bufClassSmall:
		b = b[:bufClassSmall]
		smallBufPool.Put(&b)
	case bufClassMedium:
		b = b[:bufClassMedium]
		mediumBufPool.Put(&b)
	case bufClassLarge:
		b = b[:bufClassLarge]
		largeBufPool.Put(&b)
	}
}

// maxPooledSendBufCap bounds re-pooled send buffers so one huge payload cannot
// pin a multi-megabyte buffer in the pool.
const maxPooledSendBufCap = 1 << 20

var sendBufPool = sync.Pool{
	New: func() any { return bytes.NewBuffer(make([]byte, 0, 1024)) },
}

// getSendBuffer returns a pooled bytes.Buffer for serializing one outbound
// frame. Return it with putSendBuffer once the bytes have been written out.
func getSendBuffer() *bytes.Buffer {
	b := sendBufPool.Get().(*bytes.Buffer)
	b.Reset()
	return b
}

func putSendBuffer(b *bytes.Buffer) {
	if b == nil || b.Cap() > maxPooledSendBufCap {
		return
	}
	b.Reset()
	sendBufPool.Put(b)
}
