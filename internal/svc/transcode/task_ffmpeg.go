//go:build ffmpeg
// +build ffmpeg

// If you are AI: This file implements transcoding tasks.
// Tasks subscribe to bus streams and transcode using FFmpeg.

package transcode

import (
	"context"
	"github.com/vinq1911/nonchalant/internal/core/bus"
)

// Task represents a transcoding task.
// Subscribes to a stream and transcodes it.
type Task struct {
	stream       *bus.Stream
	subscriber   *bus.Subscriber
	subscriberID uint64
	pipeline     *Pipeline
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewTask creates a new transcoding task.
func NewTask(stream *bus.Stream, pipeline *Pipeline) *Task {
	ctx, cancel := context.WithCancel(context.Background())
	return &Task{
		stream:   stream,
		pipeline: pipeline,
		ctx:      ctx,
		cancel:   cancel,
	}
}

// Start starts the transcoding task.
// Subscribes to stream and processes messages through pipeline.
func (t *Task) Start() error {
	// Attach subscriber with bounded buffer
	// Use DropOldest to prevent blocking publisher
	sub, id := t.stream.AttachSubscriber(1000, bus.BackpressureDropOldest)
	t.subscriber = sub
	t.subscriberID = id

	// Process messages in goroutine
	go t.process()

	return nil
}

// process processes messages from the subscriber.
// Runs until context is cancelled.
func (t *Task) process() {
	for {
		select {
		case <-t.ctx.Done():
			return
		default:
		}

		// Read message from subscriber buffer
		msg, ok := t.subscriber.Read()
		if !ok {
			// Buffer empty, continue waiting
			continue
		}

		// Process through pipeline
		// NOTE: Pipeline may copy payload if needed
		if err := t.pipeline.Process(msg.Payload); err != nil {
			// Log error but continue processing
			// NOTE: In production, this should be logged
			continue
		}
	}
}

// Stop stops the transcoding task.
func (t *Task) Stop() error {
	t.cancel()
	if t.stream != nil && t.subscriberID != 0 {
		t.stream.DetachSubscriber(t.subscriberID)
	}
	if t.pipeline != nil {
		_ = t.pipeline.Close()
	}
	return nil
}
