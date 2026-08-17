// If you are AI: This file implements an off-heap-style payload arena.
// One arena per Stream: the publisher acquires payload buffers by bumping
// a wrap-around cursor instead of allocating fresh. Slot reuse is implicit
// via wrap, mirroring the SharedLog's wraparound timing.
//
// Why this exists: the previous bus.AcquirePayload sync.Pool was vestigial
// — nothing ever Released, so every Acquire fell through to its New func
// and allocated a fresh 64 KB buffer. At 30 fps that's ~2 MB/s of pure GC
// churn per stream. With the arena, steady-state Acquire is allocation-free.

package bus

import (
	"sync/atomic"
)

// Arena is a fixed-size byte slab divided into equally-sized slots.
// Acquire bumps a wrap-around cursor and hands out a zero-length view of
// the next slot; the caller fills it via append. Slots automatically
// recycle once the cursor wraps past them.
//
// Safety: callers must finish using a slot before the cursor wraps far
// enough to hand it back out. With slots configured to match the
// SharedLog size (or larger), wrap intervals are seconds at typical
// frame rates — far longer than any subscriber's per-frame work.
type Arena struct {
	base   []byte
	slot   int
	mask   uint64
	cursor atomic.Uint64
}

// NewArena allocates an arena of `slots` × `slotSize` bytes. `slots` must
// be a power of two (rounded up if not) so the modulo is a bitmask.
func NewArena(slots, slotSize int) *Arena {
	n := uint64(1)
	for n < uint64(slots) {
		n <<= 1
	}
	return &Arena{
		base: make([]byte, int(n)*slotSize),
		slot: slotSize,
		mask: n - 1,
	}
}

// Acquire returns a zero-length view backed by the next arena slot. The
// returned slice has cap == slotSize when `size <= slotSize`, otherwise
// it falls back to a fresh heap slice of cap `size` (rare — only oversized
// keyframes). The caller appends payload bytes into the returned buffer.
func (a *Arena) Acquire(size int) []byte {
	if size > a.slot {
		// Oversized: fall back to a one-shot heap allocation. GC reclaims
		// it once the message wraps out of the SharedLog.
		return make([]byte, 0, size)
	}
	n := a.cursor.Add(1) - 1
	off := int(n&a.mask) * a.slot
	return a.base[off : off : off+a.slot]
}

// Stats returns the cumulative number of slots handed out (wraps included)
// — useful for benchmarking.
func (a *Arena) Stats() uint64 { return a.cursor.Load() }
