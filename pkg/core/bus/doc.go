// If you are AI: This file holds the package-level doc comment for bus. Keep
// it in sync with the actual subscribe/tap API when either changes.

// Package bus implements the in-process stream bus that sits between
// publishers (RTMP ingest) and every output service (HTTP-FLV, WebSocket-FLV,
// HLS/DASH, relay). It is also the extension point for an embedding
// application that wants to tap a live stream directly — e.g. to record it
// to object storage — without speaking RTMP, HTTP-FLV, or any other wire
// protocol to itself.
//
// # Taps: subscribing to a live stream in-process
//
// An embedded server exposes its *Registry via server.Server.Registry(). A
// consumer taps a stream in four steps:
//
//  1. Build the StreamKey for the app/stream name you want to observe:
//
//     key := bus.NewStreamKey("live", "mystream")
//
//  2. Look up (or wait for) the Stream. GetOrCreate returns the Stream
//     immediately even before a publisher attaches — consumers that want to
//     tap a stream from the moment it goes live should call GetOrCreate
//     once at startup and hold onto the result, or poll Get / GetOrCreate
//     until HasPublisher() is true:
//
//     stream, _ := registry.GetOrCreate(key)
//
//  3. Attach a Subscriber. This gives the tap its own cursor into the
//     stream's shared log; it does not affect the publisher or any other
//     subscriber, and slow taps only ever cost themselves dropped frames
//     (see Subscriber.Dropped), never the publisher or other readers:
//
//     sub, id := stream.AttachSubscriber(0, bus.BackpressureDropOldest)
//     defer stream.DetachSubscriber(id)
//
//  4. Read in a loop. Read returns the next MediaMessage (audio, video, or
//     metadata) and true, or (nil, false) when the tap has caught up to the
//     live edge. On (nil, false), park on WaitChan() until the next Publish
//     wakes you rather than busy-polling; WaitChan's channel is rotated on
//     every publish, so re-fetch it after each wakeup:
//
//     for {
//     msg, ok := sub.Read()
//     if !ok {
//     select {
//     case <-sub.WaitChan():
//     continue
//     case <-ctx.Done():
//     return ctx.Err()
//     }
//     }
//     // msg.Type is MessageTypeAudio, MessageTypeVideo, or
//     // MessageTypeMetadata; msg.IsInit marks AVC/AAC sequence
//     // headers and the initial onMetaData that a late-joining tap
//     // needs before it can decode subsequent frames. Package the
//     // bytes with pkg/core/protocol/flv, hand them to your own
//     // muxer, or write msg.Payload straight to storage — the bus
//     // itself is codec-agnostic.
//     handle(msg)
//     }
//
// A tap that attaches after the stream already has cached init messages
// (AVC/AAC sequence headers, onMetaData) receives a one-shot replay of them
// first, exactly like any other subscriber (see Stream.AttachSubscriber),
// so it can start decoding immediately without waiting for the publisher's
// next keyframe.
//
// This package intentionally has no knowledge of storage, encoding, or
// wire protocols — it only fans out MediaMessage values. Building a
// recorder on top of these taps (e.g. writing to S3-compatible object
// storage) is left to the embedding application.
package bus
