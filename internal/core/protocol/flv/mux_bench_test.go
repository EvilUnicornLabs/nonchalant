// If you are AI: Benchmarks for the FLV muxing hot path. Every subscriber
// runs MuxMessage and Tag.Bytes once per frame; the keyframe detector runs
// once per frame until the first IDR.

package flv

import (
	"testing"

	"github.com/vinq1911/nonchalant/internal/core/bus"
)

// fakeKeyframe builds a minimal AVCC video payload that IsVideoKeyframe
// recognises (codec id 7, frame type 1 = keyframe).
func fakeKeyframe(size int) []byte {
	buf := make([]byte, size)
	buf[0] = 0x17 // frame type 1 (keyframe), codec id 7 (AVC)
	buf[1] = 0x01 // AVC NALU packet
	return buf
}

// fakeInterframe builds a minimal AVCC payload that is NOT a keyframe
// (codec id 7, frame type 2 = inter-frame).
func fakeInterframe(size int) []byte {
	buf := make([]byte, size)
	buf[0] = 0x27 // frame type 2 (inter), codec id 7 (AVC)
	buf[1] = 0x01
	return buf
}

// BenchmarkIsVideoKeyframeMatch measures the keyframe detector on a payload
// that is a keyframe — covers the path that flips the gate ON.
func BenchmarkIsVideoKeyframeMatch(b *testing.B) {
	payload := fakeKeyframe(1024)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if !IsVideoKeyframe(payload) {
			b.Fatal("expected keyframe")
		}
	}
}

// BenchmarkIsVideoKeyframeMiss measures the same for non-keyframes — the
// hot path when the gate is open and we're filtering subsequent frames.
func BenchmarkIsVideoKeyframeMiss(b *testing.B) {
	payload := fakeInterframe(1024)
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		if IsVideoKeyframe(payload) {
			b.Fatal("expected non-keyframe")
		}
	}
}

// BenchmarkMuxMessageVideo measures the video tag construction wired through
// MuxMessage, which is what every FLV subscriber goroutine runs per frame.
func BenchmarkMuxMessageVideo(b *testing.B) {
	msg := &bus.MediaMessage{
		Type:      bus.MessageTypeVideo,
		Timestamp: 1234,
		Payload:   fakeInterframe(2048),
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		tag := MuxMessage(msg)
		if tag == nil {
			b.Fatal("nil tag")
		}
	}
}

// BenchmarkTagBytes measures the per-frame serialisation cost of an FLV tag —
// the path inside Subscriber.ProcessMessages that runs once per delivered frame.
func BenchmarkTagBytes(b *testing.B) {
	tag := NewTag(TagTypeVideo, 1234, fakeInterframe(2048))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = tag.Bytes()
	}
}

// BenchmarkAppendTag measures the allocation-free encoder path used by
// httpflv / wsflv subscribers. The destination buffer is reused, mimicking
// the bus.AcquirePayload / ReleasePayload pool round-trip.
func BenchmarkAppendTag(b *testing.B) {
	payload := fakeInterframe(2048)
	dst := make([]byte, 0, 4096)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		dst = AppendTag(dst[:0], TagTypeVideo, 1234, payload)
	}
}
