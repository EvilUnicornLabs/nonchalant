// If you are AI: This file defines FLV protocol constants and tag types.

package flv

// FLV file signature
const FLVSignature = "FLV"

// FLV version
const FLVVersion = 1

// FLV header size
const FLVHeaderSize = 9

// Previous tag size (4 bytes) before first tag
const FirstPreviousTagSize = 0

// Tag types
const (
	TagTypeAudio  = 8
	TagTypeVideo  = 9
	TagTypeScript = 18
)

// Audio format constants
const (
	AudioFormatAAC = 10
)

// Video codec constants
const (
	VideoCodecAVC = 7
)

// Video frame types
const (
	VideoFrameKeyFrame   = 1
	VideoFrameInterFrame = 2
)

// AVCPacketType constants
const (
	AVCPacketTypeSequenceHeader = 0
	AVCPacketTypeNALU           = 1
)

// IsVideoKeyframe returns true if the FLV video payload represents a keyframe.
// In RTMP/FLV format: byte[0] upper nibble = frame type (1=keyframe).
func IsVideoKeyframe(payload []byte) bool {
	return len(payload) >= 1 && (payload[0]>>4) == VideoFrameKeyFrame
}
