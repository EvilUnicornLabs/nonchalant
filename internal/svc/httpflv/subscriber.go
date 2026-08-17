// If you are AI: This file implements HTTP-FLV subscriber that reads from bus and writes FLV.
// After Hijack, the subscriber writes raw FLV bytes directly to a net.Conn —
// no bufio, no chunked encoding, no double-flush. Each FLV tag = one syscall.

package httpflv

import (
	"context"
	"io"
	"net"
	"time"

	"github.com/vinq1911/nonchalant/internal/core/bus"
	"github.com/vinq1911/nonchalant/internal/core/protocol/flv"
)

// Subscriber represents an HTTP-FLV client subscriber.
// Reads messages from bus and writes FLV tags directly to the underlying
// TCP connection (post-Hijack). One conn.Write per tag = one syscall.
type Subscriber struct {
	conn          io.Writer       // typically a *net.TCPConn after Hijack
	deadliner     deadlineSetter  // set on conn when available; nil for tests
	busSubscriber *bus.Subscriber
	stream        *bus.Stream
	subscriberID  uint64
	headerWritten bool
	gotKeyframe   bool   // True after first video keyframe received
	tsOffset      uint32 // First non-init timestamp, subtracted from all subsequent
	tsBaseSet     bool   // True after tsOffset is captured
}

// deadlineSetter narrows the net.Conn surface we use for the per-write
// timeout, so tests can pass a plain io.Writer.
type deadlineSetter interface {
	SetWriteDeadline(t time.Time) error
}

// writeDeadline bounds how long a single Write may block on the network
// before we evict the subscriber.
const writeDeadline = 5 * time.Second

// NewSubscriber creates a new HTTP-FLV subscriber.
// w is typically a hijacked net.Conn; the deadline path activates when w
// satisfies SetWriteDeadline(time.Time) error.
func NewSubscriber(w io.Writer, stream *bus.Stream) *Subscriber {
	var d deadlineSetter
	if c, ok := w.(net.Conn); ok {
		d = c
	} else if c, ok := w.(deadlineSetter); ok {
		d = c
	}
	return &Subscriber{
		conn:      w,
		deadliner: d,
		stream:    stream,
	}
}

// WriteHeader writes the FLV file header (9 bytes) plus the initial
// previous-tag-size (4 zero bytes) in one Write — one syscall.
func (s *Subscriber) WriteHeader(hasAudio, hasVideo bool) error {
	if s.headerWritten {
		return nil
	}
	header := flv.NewHeader(hasAudio, hasVideo)
	combined := append(header.Bytes(), 0, 0, 0, 0)
	s.armWriteDeadline()
	if _, err := s.conn.Write(combined); err != nil {
		return err
	}
	s.headerWritten = true
	return nil
}

// armWriteDeadline sets a per-write deadline on the underlying connection
// when supported. No-op for plain io.Writer (test fixtures).
func (s *Subscriber) armWriteDeadline() {
	if s.deadliner == nil {
		return
	}
	_ = s.deadliner.SetWriteDeadline(time.Now().Add(writeDeadline))
}

// ProcessMessages processes messages from the subscriber buffer and writes FLV tags.
// Returns nil on context cancellation; an error on a write failure.
// Each tag is one conn.Write — one syscall — to the hijacked TCP connection.
func (s *Subscriber) ProcessMessages(ctx context.Context) error {
	if s.busSubscriber == nil {
		return nil
	}

	for {
		msg, ok := s.busSubscriber.Read()
		if !ok {
			select {
			case <-ctx.Done():
				return nil
			case <-s.busSubscriber.WaitChan():
				continue
			}
		}

		// Keyframe gating: drop ALL non-init frames until first video keyframe.
		// Init messages (codec config) always pass through.
		if !s.gotKeyframe && !msg.IsInit {
			if msg.Type == bus.MessageTypeVideo && flv.IsVideoKeyframe(msg.Payload) {
				s.gotKeyframe = true
			} else {
				continue
			}
		}

		tagType, ok := flv.TagTypeForMessage(msg)
		if !ok {
			continue
		}
		tagBuf := bus.AcquirePayload()
		tagBuf = flv.AppendTag(tagBuf, tagType, s.rebaseTimestamp(msg), msg.Payload)

		s.armWriteDeadline()
		_, werr := s.conn.Write(tagBuf)
		bus.ReleasePayload(tagBuf)
		if werr != nil {
			return werr
		}
	}
}

// rebaseTimestamp adjusts a message timestamp so the subscriber's stream starts at ts=0.
// Init messages always return ts=0. The first non-init timestamp becomes the offset
// that is subtracted from all subsequent timestamps.
func (s *Subscriber) rebaseTimestamp(msg *bus.MediaMessage) uint32 {
	if msg.IsInit {
		return 0
	}
	if !s.tsBaseSet {
		s.tsOffset = msg.Timestamp
		s.tsBaseSet = true
	}
	if msg.Timestamp < s.tsOffset {
		return 0 // Guard against underflow
	}
	return msg.Timestamp - s.tsOffset
}

// Attach attaches the subscriber to the stream.
// Returns the subscriber ID for later detach.
func (s *Subscriber) Attach() uint64 {
	busSub, id := s.stream.AttachSubscriber(1000, bus.BackpressureDropOldest)
	s.busSubscriber = busSub
	s.subscriberID = id
	return id
}

// Detach detaches the subscriber from the stream.
func (s *Subscriber) Detach() {
	if s.stream != nil && s.subscriberID != 0 {
		s.stream.DetachSubscriber(s.subscriberID)
		s.subscriberID = 0
		s.busSubscriber = nil
	}
}

// BusSubscriber returns the underlying bus subscriber, or nil if not yet attached.
// Used by tests.
func (s *Subscriber) BusSubscriber() *bus.Subscriber {
	return s.busSubscriber
}
