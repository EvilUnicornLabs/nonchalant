// If you are AI: This file defines MediaMessage and related types for the stream bus.
// MediaMessage represents a unit of media flowing through the bus with pooled memory.

package bus

import (
	"sync"
)

// MessageType represents the type of media message.
type MessageType uint8

const (
	// MessageTypeAudio represents an audio frame.
	MessageTypeAudio MessageType = iota
	// MessageTypeVideo represents a video frame.
	MessageTypeVideo
	// MessageTypeMetadata represents metadata or script data.
	MessageTypeMetadata
)

// MediaMessage represents a unit of media flowing through the bus.
// Payload memory comes from a pool to avoid allocations in hot paths.
// Ownership: The message owns the payload buffer. When a message is published,
// subscribers receive references. The publisher retains ownership until all
// subscribers have processed the message, then releases it back to the pool.
type MediaMessage struct {
	Type      MessageType // Type of media (audio, video, metadata)
	Timestamp uint32      // Media timestamp in timebase units
	Payload   []byte      // Media payload (owned by message, returned to pool on release)
	IsInit    bool        // True for codec init data (AVC/AAC sequence headers) that late joiners need
}

// messagePool is a sync.Pool for MediaMessage instances.
// This eliminates per-message allocations in steady state.
var messagePool = sync.Pool{
	New: func() interface{} {
		return &MediaMessage{}
	},
}

// AcquireMessage acquires a MediaMessage from the pool.
// The caller must call ReleaseMessage when done to return it to the pool.
func AcquireMessage() *MediaMessage {
	msg := messagePool.Get().(*MediaMessage)
	// Reset fields to zero values
	msg.Type = 0
	msg.Timestamp = 0
	msg.Payload = nil
	msg.IsInit = false
	return msg
}

// ReleaseMessage returns a MediaMessage to the pool.
// The message and its payload should not be used after release.
// NOTE: Payload is not pooled here; payload pooling is handled separately.
func ReleaseMessage(msg *MediaMessage) {
	if msg == nil {
		return
	}
	// Clear payload reference to allow GC
	msg.Payload = nil
	messagePool.Put(msg)
}

// payloadPool is a sync.Pool for payload buffers.
// This eliminates per-message payload allocations in steady state.
var payloadPool = sync.Pool{
	New: func() interface{} {
		// Preallocate 64KB buffer for typical frame sizes
		buf := make([]byte, 0, 64*1024)
		return &buf
	},
}

// AcquirePayload acquires a payload buffer from the pool.
// The caller must call ReleasePayload when done.
func AcquirePayload() []byte {
	bufPtr := payloadPool.Get().(*[]byte)
	return (*bufPtr)[:0] // Reset length but keep capacity
}

// ReleasePayload returns a payload buffer to the pool.
// The buffer should not be used after release.
func ReleasePayload(buf []byte) {
	if buf == nil {
		return
	}
	// Reset to zero length, keep capacity for reuse
	buf = buf[:0]
	// Only pool buffers with reasonable capacity to avoid memory bloat
	if cap(buf) <= 256*1024 {
		bufPtr := &buf
		payloadPool.Put(bufPtr)
	}
}

// SetPayload sets the payload on a message, acquiring it from the pool.
// The previous payload (if any) should be released before calling this.
func (m *MediaMessage) SetPayload(data []byte) {
	// Acquire a new buffer from pool
	buf := AcquirePayload()
	// Copy data into the pooled buffer
	m.Payload = append(buf, data...)
}

// Clone creates a deep copy of the message with a new payload (not pooled, for long-lived cache).
// The clone owns its own payload and must be released independently.
func (m *MediaMessage) Clone() *MediaMessage {
	clone := &MediaMessage{
		Type:      m.Type,
		Timestamp: m.Timestamp,
		IsInit:    m.IsInit,
	}
	if len(m.Payload) > 0 {
		clone.Payload = make([]byte, len(m.Payload))
		copy(clone.Payload, m.Payload)
	}
	return clone
}

// String returns a human-readable representation of the message type.
func (t MessageType) String() string {
	switch t {
	case MessageTypeAudio:
		return "audio"
	case MessageTypeVideo:
		return "video"
	case MessageTypeMetadata:
		return "metadata"
	default:
		return "unknown"
	}
}
