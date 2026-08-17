// If you are AI: This file defines the Subscriber type. A Subscriber is a
// cursor into its parent Stream's shared log, plus a small queue of
// init messages replayed at attach time.

package bus

import (
	"sync/atomic"
)

// Subscriber consumes media messages from a Stream.
// One goroutine per Subscriber should call Read; the cursor is local and
// not safe for concurrent use across multiple readers.
type Subscriber struct {
	id      uint64
	stream  *Stream
	cursor  uint64          // next sequence number to read from the log
	pending []*MediaMessage // init messages replayed first; drained in order
	pendIdx int             // index into pending (0..len(pending))
	dropped atomic.Uint64   // count of messages skipped due to slow-consumer wrap
}

// newSubscriber allocates a Subscriber pointing at the end of the stream's
// shared log. Caller must hold s.mu (called from Stream.AttachSubscriber).
func newSubscriber(id uint64, stream *Stream) *Subscriber {
	return &Subscriber{
		id:     id,
		stream: stream,
		cursor: stream.log.LatestSeq(), // start at "now"; init replay handled by pending
	}
}

// ID returns the unique subscriber identifier.
func (s *Subscriber) ID() uint64 { return s.id }

// Read returns the next message available to this subscriber.
// Returns (msg, true) when a message is available, (nil, false) on empty.
// Init messages cached at attach time are returned first; afterwards Read
// consults the shared log via the cursor. If the cursor has fallen more
// than the log size behind the publisher, Read fast-forwards and bumps
// the dropped counter.
// Allocation: zero in steady state.
func (s *Subscriber) Read() (*MediaMessage, bool) {
	// Drain init replay first (one-shot at attach).
	if s.pendIdx < len(s.pending) {
		msg := s.pending[s.pendIdx]
		s.pendIdx++
		return msg, true
	}

	next, res, ok := s.stream.log.readAt(s.cursor)
	if !ok {
		return nil, false
	}
	if res.skipped > 0 {
		s.dropped.Add(res.skipped)
	}
	s.cursor = next
	return res.msg, true
}

// Dropped returns the number of messages this subscriber missed because
// the publisher overwrote unread slots while it was behind.
func (s *Subscriber) Dropped() uint64 { return s.dropped.Load() }

// WaitChan returns the channel a caller should park on when Read returned
// (nil, false). The channel is closed by the next Publish; callers must
// re-call WaitChan after each wakeup because the stream rotates it on
// every publish.
func (s *Subscriber) WaitChan() <-chan struct{} { return s.stream.WaitChan() }
