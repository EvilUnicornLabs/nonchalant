// If you are AI: This file provides FLV muxing helpers for converting MediaMessage to FLV tags.
// Muxing preserves original payloads without transcoding.

package flv

import (
	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

// MuxAudio converts a bus MediaMessage to an FLV audio tag.
// The payload is used directly without modification.
// Allocation: Creates tag structure, reuses payload slice.
func MuxAudio(msg *bus.MediaMessage) *Tag {
	if msg == nil || msg.Type != bus.MessageTypeAudio {
		return nil
	}
	return NewTag(TagTypeAudio, msg.Timestamp, msg.Payload)
}

// MuxVideo converts a bus MediaMessage to an FLV video tag.
// The payload is used directly without modification.
// Allocation: Creates tag structure, reuses payload slice.
func MuxVideo(msg *bus.MediaMessage) *Tag {
	if msg == nil || msg.Type != bus.MessageTypeVideo {
		return nil
	}
	return NewTag(TagTypeVideo, msg.Timestamp, msg.Payload)
}

// MuxScript converts a bus MediaMessage to an FLV script tag.
// The payload is used directly without modification.
// Allocation: Creates tag structure, reuses payload slice.
func MuxScript(msg *bus.MediaMessage) *Tag {
	if msg == nil || msg.Type != bus.MessageTypeMetadata {
		return nil
	}
	return NewTag(TagTypeScript, msg.Timestamp, msg.Payload)
}

// MuxMessage converts a bus MediaMessage to an FLV tag based on message type.
// Returns nil if message type is not supported.
func MuxMessage(msg *bus.MediaMessage) *Tag {
	if msg == nil {
		return nil
	}

	switch msg.Type {
	case bus.MessageTypeAudio:
		return MuxAudio(msg)
	case bus.MessageTypeVideo:
		return MuxVideo(msg)
	case bus.MessageTypeMetadata:
		return MuxScript(msg)
	default:
		return nil
	}
}
