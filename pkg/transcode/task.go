//go:build !ffmpeg
// +build !ffmpeg

// If you are AI: This file provides stub implementations for transcode tasks.

package transcode

// Task represents a transcoding task.
// Stub implementation.
type Task struct{}

// Start starts the transcoding task.
// Stub: returns immediately.
func (t *Task) Start() error {
	return nil
}

// Stop stops the transcoding task.
// Stub: no-op.
func (t *Task) Stop() error {
	return nil
}
