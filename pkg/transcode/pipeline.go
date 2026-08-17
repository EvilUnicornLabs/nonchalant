//go:build !ffmpeg
// +build !ffmpeg

// If you are AI: This file provides stub implementations for transcode pipelines.

package transcode

// Pipeline represents a transcoding pipeline.
// Stub implementation.
type Pipeline struct{}

// NewPipeline creates a new transcoding pipeline.
// Stub: returns error.
func NewPipeline(inputURL, outputURL, format string) (*Pipeline, error) {
	return nil, nil
}

// Close closes the pipeline.
// Stub: no-op.
func (p *Pipeline) Close() error {
	return nil
}

// Process processes media through the pipeline.
// Stub: returns error.
func (p *Pipeline) Process(data []byte) error {
	return nil
}
