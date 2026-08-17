// If you are AI: This file implements a lock-free ring buffer for subscriber message delivery.
// The ring buffer provides bounded buffering with configurable backpressure behavior.
// CRITICAL: Both writePos and readPos increment freely (never masked). Only use the mask
// when indexing into the buffer array. The emptiness check readPos==writePos relies on
// both counters using the same domain.

package bus

import (
	"sync/atomic"
)

// BackpressureStrategy defines how the ring buffer handles overflow.
type BackpressureStrategy uint8

const (
	// BackpressureDropOldest drops the oldest message when buffer is full.
	BackpressureDropOldest BackpressureStrategy = iota
	// BackpressureDropNewest drops the newest message when buffer is full.
	BackpressureDropNewest
)

// RingBuffer is a bounded circular buffer for MediaMessage delivery.
// It is lock-free for single producer, single consumer scenarios.
// NOTE: This implementation uses atomic operations for thread-safety. Each
// slot is an atomic.Pointer so Write and Read can safely run on different
// goroutines without the race detector flagging the slot store / load —
// even though the index ordering already provides the happens-before edge.
// Allocation: Pre-allocated buffer, no per-message allocations.
type RingBuffer struct {
	buffer   []atomic.Pointer[MediaMessage] // Pre-allocated message slots
	size     uint32                         // Buffer size (power of 2 for efficient modulo)
	mask     uint32                         // size - 1, for efficient modulo (index = pos & mask)
	writePos uint32                         // Write position (atomic, free-running)
	readPos  uint32                         // Read position (atomic, free-running)
	strategy BackpressureStrategy
	dropped  uint64 // Counter for dropped messages (atomic)
}

// NewRingBuffer creates a new ring buffer with the specified capacity.
// Capacity is rounded up to a power of 2 for efficient modulo via bitmask.
func NewRingBuffer(capacity uint32, strategy BackpressureStrategy) *RingBuffer {
	// Round up to next power of 2
	actualSize := uint32(1)
	for actualSize < capacity {
		actualSize <<= 1
	}

	return &RingBuffer{
		buffer:   make([]atomic.Pointer[MediaMessage], actualSize),
		size:     actualSize,
		mask:     actualSize - 1,
		strategy: strategy,
	}
}

// Write attempts to write a message to the buffer.
// Returns true if written, false if buffer was full and message was dropped (DropNewest).
// Lock expectations: Single writer (publisher goroutine).
// Both writePos and readPos are free-running; only masked when indexing the buffer array.
func (rb *RingBuffer) Write(msg *MediaMessage) bool {
	if msg == nil {
		return false
	}

	writePos := atomic.LoadUint32(&rb.writePos)
	readPos := atomic.LoadUint32(&rb.readPos)

	// Buffer full when used count equals capacity.
	// Unsigned subtraction works correctly even after uint32 wrap.
	if writePos-readPos >= rb.size {
		atomic.AddUint64(&rb.dropped, 1)
		if rb.strategy == BackpressureDropOldest {
			// Advance read position to drop oldest
			atomic.AddUint32(&rb.readPos, 1)
		} else {
			// Drop newest (current message)
			return false
		}
	}

	// Write message at current write position (masked for array index).
	// Atomic Store on the slot — combined with the writePos store below — gives
	// the consumer a release/acquire pair so the slot's contents are visible.
	rb.buffer[writePos&rb.mask].Store(msg)
	atomic.StoreUint32(&rb.writePos, writePos+1)
	return true
}

// Read attempts to read a message from the buffer.
// Returns the message and true if available, nil and false if empty.
// Lock expectations: Single reader (subscriber goroutine).
func (rb *RingBuffer) Read() (*MediaMessage, bool) {
	readPos := atomic.LoadUint32(&rb.readPos)
	writePos := atomic.LoadUint32(&rb.writePos)

	if readPos == writePos {
		// Buffer empty
		return nil, false
	}

	msg := rb.buffer[readPos&rb.mask].Load()
	atomic.AddUint32(&rb.readPos, 1)
	return msg, true
}

// Dropped returns the number of messages dropped due to backpressure.
func (rb *RingBuffer) Dropped() uint64 {
	return atomic.LoadUint64(&rb.dropped)
}

// Available returns the number of free slots in the buffer.
func (rb *RingBuffer) Available() uint32 {
	writePos := atomic.LoadUint32(&rb.writePos)
	readPos := atomic.LoadUint32(&rb.readPos)
	used := writePos - readPos // unsigned subtraction handles wrap
	return rb.size - used
}
