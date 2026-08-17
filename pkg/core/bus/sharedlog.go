// If you are AI: This file implements a per-stream shared log used for
// fan-out to many subscribers. Replaces the per-subscriber RingBuffer in
// Stream.Publish: a single publisher does one atomic.Add (sequence) plus
// one atomic.Store (slot) regardless of subscriber count, so publish is
// O(1) instead of O(N).
//
// Subscribers each hold a cursor (their next-read sequence). When a slow
// subscriber falls more than `size` slots behind, the publisher has
// overwritten the missed slots; the subscriber jumps forward and bumps a
// per-subscriber drop counter.

package bus

import (
	"sync/atomic"
)

// SharedLog is a single-producer, multi-consumer ring of MediaMessage
// pointers. The producer (one per Stream) appends; consumers (any number)
// read independently via their own cursor.
//
// Design notes:
//   - Slot is atomic.Pointer[MediaMessage] so the slot store / load gives
//     us a release/acquire pair the race detector can see.
//   - nextSeq is monotonically increasing across the log's lifetime. The
//     index used to address the buffer is `(seq - 1) & mask`. Wrap-around
//     is handled implicitly via uint64.
//   - The producer never blocks on a slow consumer. Slow consumers detect
//     they fell behind and skip forward.
type SharedLog struct {
	buf     []atomic.Pointer[MediaMessage]
	mask    uint64
	size    uint64
	nextSeq atomic.Uint64
}

// NewSharedLog allocates a log with `capacity` slots, rounded up to a
// power of 2 so the modulo can be done with a bitmask.
func NewSharedLog(capacity uint32) *SharedLog {
	size := uint64(1)
	for size < uint64(capacity) {
		size <<= 1
	}
	return &SharedLog{
		buf:  make([]atomic.Pointer[MediaMessage], size),
		mask: size - 1,
		size: size,
	}
}

// Publish writes msg at the next sequence and returns the sequence number
// (1-based: first publish returns 1, second returns 2, ...). The single
// producer must call this from one goroutine; consumers read concurrently.
func (l *SharedLog) Publish(msg *MediaMessage) uint64 {
	if msg == nil {
		return l.nextSeq.Load()
	}
	seq := l.nextSeq.Add(1)
	l.buf[(seq-1)&l.mask].Store(msg)
	return seq
}

// LatestSeq returns the most-recently-published sequence number (0 before
// any publish, otherwise the value of the last Publish return).
func (l *SharedLog) LatestSeq() uint64 { return l.nextSeq.Load() }

// Size returns the log's slot count (a power of 2).
func (l *SharedLog) Size() uint64 { return l.size }

// readResult is the bundle returned by Read.
type readResult struct {
	msg     *MediaMessage
	skipped uint64 // count of messages this read jumped past (slow-consumer drops)
}

// readAt reads the message at the consumer's cursor `at`, advances the
// cursor, and returns the new cursor + message. If the log is empty
// (cursor caught up to the producer) ok is false. If the consumer fell
// more than `size` slots behind the producer, the cursor is fast-forwarded
// to the oldest still-available slot and `skipped` reports the loss.
func (l *SharedLog) readAt(at uint64) (next uint64, res readResult, ok bool) {
	pubSeq := l.nextSeq.Load()
	if at >= pubSeq {
		return at, readResult{}, false
	}

	// If we've fallen behind by more than the log size, the producer has
	// already overwritten our oldest unread slots.
	gap := pubSeq - at
	if gap > l.size {
		res.skipped = gap - l.size
		at = pubSeq - l.size
	}

	res.msg = l.buf[at&l.mask].Load()
	return at + 1, res, true
}
