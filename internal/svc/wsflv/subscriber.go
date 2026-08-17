// If you are AI: This file implements WebSocket-FLV subscriber that reads from bus and writes FLV.
// Subscriber manages the WebSocket connection lifecycle and message processing.

package wsflv

import (
	"context"
	"time"

	"github.com/vinq1911/nonchalant/internal/core/bus"
	"github.com/vinq1911/nonchalant/internal/core/protocol/flv"
)

// Subscriber represents a WebSocket-FLV client subscriber.
// Reads messages from bus and writes FLV tags to WebSocket connection.
type Subscriber struct {
	conn          WebSocketConn
	busSubscriber *bus.Subscriber
	stream        *bus.Stream
	subscriberID  uint64
	headerWritten bool
	gotKeyframe   bool   // True after first video keyframe received
	tsOffset      uint32 // First non-init timestamp, subtracted from all subsequent
	tsBaseSet     bool   // True after tsOffset is captured
}

// WebSocketConn defines the interface for WebSocket operations.
// This allows for easier testing and abstraction.
type WebSocketConn interface {
	WriteMessage(messageType int, data []byte) error
	SetWriteDeadline(t time.Time) error
	Close() error
}

// writeDeadline bounds the time a single Write to the WebSocket may block.
// Slow / stalled clients are evicted instead of pinning the subscriber goroutine.
const writeDeadline = 5 * time.Second

// NewSubscriber creates a new WebSocket-FLV subscriber.
func NewSubscriber(conn WebSocketConn, stream *bus.Stream) *Subscriber {
	return &Subscriber{
		conn:   conn,
		stream: stream,
	}
}

// WriteHeader writes the FLV file header as the first WebSocket frame.
// Must be called before writing any tags.
func (s *Subscriber) WriteHeader(hasAudio, hasVideo bool) error {
	if s.headerWritten {
		return nil
	}

	header := flv.NewHeader(hasAudio, hasVideo)
	headerBytes := header.Bytes()

	// Write previous tag size (0 for first tag)
	prevSize := make([]byte, 4)

	// Combine header and previous tag size into single frame
	frame := make([]byte, len(headerBytes)+len(prevSize))
	copy(frame, headerBytes)
	copy(frame[len(headerBytes):], prevSize)

	// Write as binary WebSocket frame
	_ = s.conn.SetWriteDeadline(time.Now().Add(writeDeadline))
	if err := s.conn.WriteMessage(2, frame); err != nil {
		return err
	}

	s.headerWritten = true
	return nil
}

// ProcessMessages processes messages from the subscriber buffer and writes FLV tags.
// This runs in a loop until ctx is done, the connection is closed, or an error occurs.
// ALL non-init frames are dropped until the first video keyframe arrives, so that
// audio and video start simultaneously and the decoder can initialize properly.
// Timestamps are rebased so the subscriber's stream starts at ts=0, preventing
// players from buffering a multi-second gap between init (ts=0) and live data.
// Allocation: Tag creation allocates header, but payloads are reused from bus.
// Returns nil on context cancellation; an error on a write failure.
func (s *Subscriber) ProcessMessages(ctx context.Context) error {
	if s.busSubscriber == nil {
		return nil
	}

	for {
		msg, ok := s.busSubscriber.Read()
		if !ok {
			// No data: park on the stream's wake channel until the next
			// publish (broadcast wake). Costs zero CPU while idle.
			select {
			case <-ctx.Done():
				return nil
			case <-s.busSubscriber.WaitChan():
				continue
			}
		}

		// Keyframe gating: drop ALL non-init frames until first video keyframe.
		// This prevents audio from piling up before video, which causes player
		// buffer deadlocks. Init messages (codec config) always pass through.
		if !s.gotKeyframe && !msg.IsInit {
			if msg.Type == bus.MessageTypeVideo && flv.IsVideoKeyframe(msg.Payload) {
				s.gotKeyframe = true
			} else {
				continue // Drop all non-init frames before first keyframe
			}
		}

		// Encode the FLV tag directly into a pooled buffer — zero allocs in
		// steady state. WriteMessage must be a single contiguous frame so
		// we build the buffer rather than doing multiple Writes.
		tagType, ok := flv.TagTypeForMessage(msg)
		if !ok {
			continue
		}
		tagBuf := bus.AcquirePayload()
		tagBuf = flv.AppendTag(tagBuf, tagType, s.rebaseTimestamp(msg), msg.Payload)

		// Write tag as binary WebSocket frame (each FLV tag = one frame).
		// The per-write deadline bounds how long a slow client can block us.
		_ = s.conn.SetWriteDeadline(time.Now().Add(writeDeadline))
		werr := s.conn.WriteMessage(2, tagBuf)
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
// Backpressure strategy: DropOldest - same as HTTP-FLV to ensure consistency.
// Slow WebSocket clients drop oldest frames to prevent blocking publisher.
func (s *Subscriber) Attach() uint64 {
	// Attach with bounded buffer and drop oldest strategy
	// This ensures publisher never blocks on slow WebSocket clients
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
