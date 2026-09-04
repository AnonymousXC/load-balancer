package pool

import (
	"bytes"
	"sync"
)

// BufferPool manages a pool of byte buffers for efficient memory reuse
type BufferPool struct {
	pool sync.Pool
}

// NewBufferPool creates a new buffer pool
func NewBufferPool() *BufferPool {
	return &BufferPool{
		pool: sync.Pool{
			New: func() interface{} {
				return new(bytes.Buffer)
			},
		},
	}
}

// Get retrieves a buffer from the pool
func (bp *BufferPool) Get() *bytes.Buffer {
	buf := bp.pool.Get().(*bytes.Buffer)
	buf.Reset()
	return buf
}

// Put returns a buffer to the pool
func (bp *BufferPool) Put(buf *bytes.Buffer) {
	// Only return buffers under a certain size to prevent memory bloat
	if buf.Cap() < 1<<20 { // 1MB
		bp.pool.Put(buf)
	}
}

// Global buffer pool instance
var globalBufferPool = NewBufferPool()

// GetBuffer retrieves a buffer from the global pool
func GetBuffer() *bytes.Buffer {
	return globalBufferPool.Get()
}

// PutBuffer returns a buffer to the global pool
func PutBuffer(buf *bytes.Buffer) {
	globalBufferPool.Put(buf)
}
