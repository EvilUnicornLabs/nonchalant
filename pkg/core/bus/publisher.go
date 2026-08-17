// If you are AI: This file defines the Publisher type and the Stream methods
// that manage publisher attach/detach. Split out of stream.go to stay under
// the 300-line limit.

package bus

// Publisher represents a stream publisher.
// Only one publisher can be attached to a stream at a time.
type Publisher struct {
	id uint64 // Unique publisher ID
}

// AttachPublisher attaches a publisher to the stream.
// Returns true if attached, false if a publisher is already attached.
func (s *Stream) AttachPublisher(id uint64) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.publisher != nil {
		return false
	}

	s.publisher = &Publisher{id: id}
	return true
}

// DetachPublisher detaches the current publisher from the stream.
// Also clears cached init messages since they belong to the publisher's session.
func (s *Stream) DetachPublisher() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.publisher = nil
	s.initVideo = nil
	s.initAudio = nil
	s.initMeta = nil
}

// HasPublisher returns true if a publisher is currently attached.
func (s *Stream) HasPublisher() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.publisher != nil
}
