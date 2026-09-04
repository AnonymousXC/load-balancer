package pool

import (
	"sync"
)

// BytePool manages a pool of byte slices for efficient memory reuse
type BytePool struct {
	pool    sync.Pool
	size    int
	maxSize int
}

// NewBytePool creates a new byte pool with the specified size
func NewBytePool(size, maxSize int) *BytePool {
	return &BytePool{
		size:    size,
		maxSize: maxSize,
		pool: sync.Pool{
			New: func() interface{} {
				return make([]byte, size)
			},
		},
	}
}

// Get retrieves a byte slice from the pool
func (bp *BytePool) Get() []byte {
	return bp.pool.Get().([]byte)
}

// Put returns a byte slice to the pool
func (bp *BytePool) Put(b []byte) {
	// Only return slices of the expected size
	if cap(b) == bp.size {
		bp.pool.Put(b)
	}
}

// ResizePool manages pools of different sizes
type ResizePool struct {
	pools map[int]*BytePool
	mu    sync.RWMutex
}

// NewResizePool creates a new resize pool
func NewResizePool() *ResizePool {
	return &ResizePool{
		pools: make(map[int]*BytePool),
	}
}

// Get retrieves a byte slice of at least the requested size
func (rp *ResizePool) Get(size int) []byte {
	rp.mu.RLock()
	pool, exists := rp.pools[size]
	rp.mu.RUnlock()
	
	if !exists {
		rp.mu.Lock()
		pool, exists = rp.pools[size]
		if !exists {
			pool = NewBytePool(size, size*2)
			rp.pools[size] = pool
		}
		rp.mu.Unlock()
	}
	
	return pool.Get()
}

// Put returns a byte slice to the appropriate pool
func (rp *ResizePool) Put(b []byte) {
	size := cap(b)
	rp.mu.RLock()
	pool, exists := rp.pools[size]
	rp.mu.RUnlock()
	
	if exists {
		pool.Put(b)
	}
}

// Global resize pool instance
var globalResizePool = NewResizePool()

// GetBytes retrieves a byte slice of at least the requested size from the global pool
func GetBytes(size int) []byte {
	return globalResizePool.Get(size)
}

// PutBytes returns a byte slice to the global pool
func PutBytes(b []byte) {
	globalResizePool.Put(b)
}
