// If you are AI: This file contains benchmarks for publish/fanout performance.
// Benchmarks must prove stable allocations and predictable throughput. The
// subscriber/publisher-count scaling sweeps and the concurrent multi-stream
// benchmark live in stream_bench_scale_test.go to stay under the line limit.

package bus

import (
	"fmt"
	"testing"
)

// BenchmarkPublishSingleSubscriber benchmarks publish to a single subscriber.
// This measures the hot path for single consumer scenarios.
func BenchmarkPublishSingleSubscriber(b *testing.B) {
	key := NewStreamKey("live", "bench")
	stream := NewStream(key)
	stream.AttachPublisher(1)

	sub, _ := stream.AttachSubscriber(1000, BackpressureDropOldest)

	// Pre-allocate message
	msg := AcquireMessage()
	msg.Type = MessageTypeVideo
	msg.Timestamp = 1000
	msg.SetPayload(make([]byte, 1024))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg.Timestamp = uint32(i * 1000)
		stream.Publish(msg)
		// Read to keep buffer from filling
		sub.Read()
	}

	ReleaseMessage(msg)
}

// BenchmarkPublishMultipleSubscribers benchmarks publish to multiple subscribers.
// This measures fanout performance with concurrent consumers.
func BenchmarkPublishMultipleSubscribers(b *testing.B) {
	key := NewStreamKey("live", "bench")
	stream := NewStream(key)
	stream.AttachPublisher(1)

	// Create 10 subscribers
	subs := make([]*Subscriber, 10)
	for i := 0; i < 10; i++ {
		sub, _ := stream.AttachSubscriber(1000, BackpressureDropOldest)
		subs[i] = sub
	}

	// Pre-allocate message
	msg := AcquireMessage()
	msg.Type = MessageTypeVideo
	msg.Timestamp = 1000
	msg.SetPayload(make([]byte, 1024))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg.Timestamp = uint32(i * 1000)
		stream.Publish(msg)
		// Read from all subscribers to keep buffers from filling
		for _, sub := range subs {
			sub.Read()
		}
	}

	ReleaseMessage(msg)
}

// BenchmarkPublishFanoutOnly benchmarks the fanout operation without reading.
// This isolates the publish/fanout overhead.
func BenchmarkPublishFanoutOnly(b *testing.B) {
	key := NewStreamKey("live", "bench")
	stream := NewStream(key)
	stream.AttachPublisher(1)

	// Create 10 subscribers with large buffers
	for i := 0; i < 10; i++ {
		stream.AttachSubscriber(10000, BackpressureDropOldest)
	}

	// Pre-allocate message
	msg := AcquireMessage()
	msg.Type = MessageTypeVideo
	msg.Timestamp = 1000
	msg.SetPayload(make([]byte, 1024))

	b.ResetTimer()
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg.Timestamp = uint32(i * 1000)
		stream.Publish(msg)
	}

	ReleaseMessage(msg)
}

// BenchmarkMessagePool benchmarks message acquisition and release.
// This verifies the pool eliminates allocations in steady state.
func BenchmarkMessagePool(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		msg := AcquireMessage()
		msg.Type = MessageTypeVideo
		msg.Timestamp = uint32(i)
		ReleaseMessage(msg)
	}
}

// BenchmarkPayloadPool benchmarks payload buffer acquisition and release.
// This verifies the payload pool eliminates allocations.
func BenchmarkPayloadPool(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		buf := AcquirePayload()
		buf = append(buf, make([]byte, 1024)...)
		ReleasePayload(buf)
	}
}

// BenchmarkPublishHighFanout measures how the lock-free fanout scales with
// subscriber count. We don't drain the buffers — this isolates the publish
// hot path. With the atomic.Pointer snapshot, ns/op should grow roughly
// linearly with fan-out; the test reports allocs/op to flag any regressions
// that introduce per-publish allocation.
func BenchmarkPublishHighFanout(b *testing.B) {
	for _, n := range []int{1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("subs=%d", n), func(b *testing.B) {
			stream := NewStream(NewStreamKey("live", "bench"))
			stream.AttachPublisher(1)
			for i := 0; i < n; i++ {
				stream.AttachSubscriber(1024, BackpressureDropOldest)
			}
			msg := AcquireMessage()
			msg.Type = MessageTypeVideo
			msg.SetPayload(make([]byte, 1024))
			defer ReleaseMessage(msg)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				stream.Publish(msg)
			}
		})
	}
}

// BenchmarkPublisherFrame simulates an end-to-end RTMP publisher path:
// acquire a payload buffer (arena), copy in raw payload bytes, build a
// MediaMessage, and Publish to the stream. This is the realistic per-frame
// cost the RTMP ingest goroutine pays at runtime. With the per-stream
// arena, allocs/op should drop to zero (modulo the message struct).
func BenchmarkPublisherFrame(b *testing.B) {
	for _, payloadSize := range []int{1024, 4096, 16384} {
		b.Run(fmt.Sprintf("payload=%d", payloadSize), func(b *testing.B) {
			stream := NewStream(NewStreamKey("live", "bench"))
			stream.AttachPublisher(1)
			// One subscriber so the log doesn't optimise itself out.
			stream.AttachSubscriber(1024, BackpressureDropOldest)
			rawIn := make([]byte, payloadSize)

			b.SetBytes(int64(payloadSize))
			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				msg := stream.AcquireMessage()
				msg.Type = MessageTypeVideo
				msg.Timestamp = uint32(i)
				buf := stream.AcquirePayload(len(rawIn))
				msg.Payload = append(buf, rawIn...)
				stream.Publish(msg)
			}
		})
	}
}

// BenchmarkPublisherFrameLegacy measures the previous publisher path that
// goes through the global bus.AcquirePayload sync.Pool — kept for
// comparison so regressions are obvious. The pool is empty in steady state
// (nothing Releases) so every Acquire allocates a 64 KB buffer.
func BenchmarkPublisherFrameLegacy(b *testing.B) {
	stream := NewStream(NewStreamKey("live", "bench"))
	stream.AttachPublisher(1)
	stream.AttachSubscriber(1024, BackpressureDropOldest)
	rawIn := make([]byte, 4096)

	b.SetBytes(int64(len(rawIn)))
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		msg := AcquireMessage()
		msg.Type = MessageTypeVideo
		msg.Timestamp = uint32(i)
		buf := AcquirePayload() // pool path — no Release ⇒ allocates per frame
		msg.Payload = append(buf, rawIn...)
		stream.Publish(msg)
	}
}

// BenchmarkAttachDetachChurn measures the cost of subscribers joining and
// leaving a stream. This is the path the snapshot rebuild fires on; if it
// regresses, every viewer connect / disconnect pays for it.
func BenchmarkAttachDetachChurn(b *testing.B) {
	stream := NewStream(NewStreamKey("live", "bench"))
	stream.AttachPublisher(1)
	// Hold a steady population so the snapshot rebuild has work to copy.
	const steady = 50
	steadyIDs := make([]uint64, steady)
	for i := 0; i < steady; i++ {
		_, id := stream.AttachSubscriber(64, BackpressureDropOldest)
		steadyIDs[i] = id
	}
	defer func() {
		for _, id := range steadyIDs {
			stream.DetachSubscriber(id)
		}
	}()

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_, id := stream.AttachSubscriber(64, BackpressureDropOldest)
		stream.DetachSubscriber(id)
	}
}

// BenchmarkRingBufferSaturated forces the ring buffer to overflow on every
// write so the backpressure path is taken every iteration. Measures the
// dropped-message accounting cost — the slow-client steady state.
func BenchmarkRingBufferSaturated(b *testing.B) {
	rb := NewRingBuffer(8, BackpressureDropOldest)
	msg := AcquireMessage()
	msg.Type = MessageTypeVideo
	msg.SetPayload(make([]byte, 64))
	defer ReleaseMessage(msg)

	// Fill the buffer once so subsequent writes always trigger overflow.
	for i := 0; i < 8; i++ {
		rb.Write(msg)
	}

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rb.Write(msg) // every call overflows + drops oldest
	}
}

// BenchmarkRingBufferReadEmpty measures the empty-buffer Read path — the
// hot loop that subscribers run when their publisher stalls.
func BenchmarkRingBufferReadEmpty(b *testing.B) {
	rb := NewRingBuffer(8, BackpressureDropOldest)
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rb.Read()
	}
}
