// If you are AI: This file implements FLV tag creation and encoding.
// Subscribers stream live media at high rate; the AppendTag-based path is
// allocation-free (caller supplies the destination buffer, typically pooled).

package flv

import (
	"encoding/binary"

	"github.com/vinq1911/nonchalant/internal/core/bus"
)

// Tag represents an FLV tag (audio, video, or script).
// NOTE: Bytes() is retained for backward compatibility; it allocates. The
// hot path (subscriber loop) uses AppendTag — no struct, no allocation.
type Tag struct {
	Type      byte
	Timestamp uint32
	Data      []byte
}

// AppendTag appends a complete FLV tag (11-byte header + payload + 4-byte
// previous-tag-size trailer) to dst and returns the extended slice. This is
// the zero-allocation path used by HTTP-FLV / WS-FLV subscribers.
//
// The caller owns dst; pass a buffer from bus.AcquirePayload to reuse memory.
// For payloads <= cap(dst)-15 this performs no heap allocation.
func AppendTag(dst []byte, tagType byte, timestamp uint32, payload []byte) []byte {
	size := uint32(len(payload))
	dst = append(dst,
		tagType,
		byte(size>>16), byte(size>>8), byte(size),
		// Timestamp: lower 24 bits then high byte (TimestampExtended).
		byte(timestamp>>16), byte(timestamp>>8), byte(timestamp),
		byte(timestamp>>24),
		// Stream ID is always 0.
		0, 0, 0,
	)
	dst = append(dst, payload...)
	prev := 11 + size
	dst = append(dst,
		byte(prev>>24), byte(prev>>16), byte(prev>>8), byte(prev),
	)
	return dst
}

// TagTypeForMessage returns the FLV tag type for a bus.MediaMessage, plus
// ok=false if the message type has no FLV tag (we only emit audio, video,
// and metadata as script-data; everything else is silently dropped).
func TagTypeForMessage(msg *bus.MediaMessage) (byte, bool) {
	if msg == nil {
		return 0, false
	}
	switch msg.Type {
	case bus.MessageTypeAudio:
		return TagTypeAudio, true
	case bus.MessageTypeVideo:
		return TagTypeVideo, true
	case bus.MessageTypeMetadata:
		return TagTypeScript, true
	}
	return 0, false
}

// Bytes encodes the tag as FLV tag bytes.
// Format: tag type (1) + data size (3) + timestamp lower (3) + timestamp upper (1) + stream ID (3) + data (N) + previous tag size (4)
// Allocation: allocates a new slice; prefer AppendTag for hot paths.
func (t *Tag) Bytes() []byte {
	dataSize := uint32(len(t.Data))

	// Total: 11-byte header + data + 4-byte previous tag size
	totalSize := 11 + len(t.Data) + 4
	result := make([]byte, totalSize)

	// Tag type (1 byte)
	result[0] = t.Type

	// Data size (3 bytes, big-endian)
	result[1] = byte(dataSize >> 16)
	result[2] = byte(dataSize >> 8)
	result[3] = byte(dataSize)

	// Timestamp: lower 24 bits in bytes 4-6, upper 8 bits in byte 7 (per FLV spec)
	result[4] = byte(t.Timestamp >> 16)
	result[5] = byte(t.Timestamp >> 8)
	result[6] = byte(t.Timestamp)
	result[7] = byte(t.Timestamp >> 24) // TimestampExtended

	// Stream ID (3 bytes, always 0)
	result[8] = 0
	result[9] = 0
	result[10] = 0

	// Data
	copy(result[11:], t.Data)

	// Previous tag size (4 bytes, big-endian) = 11 + data size
	prevSize := uint32(11 + len(t.Data))
	binary.BigEndian.PutUint32(result[11+len(t.Data):], prevSize)

	return result
}

// NewTag creates a new FLV tag from type, timestamp, and data.
func NewTag(tagType byte, timestamp uint32, data []byte) *Tag {
	return &Tag{
		Type:      tagType,
		Timestamp: timestamp,
		Data:      data,
	}
}
