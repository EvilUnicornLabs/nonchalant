//go:build ffmpeg
// +build ffmpeg

// If you are AI: This file implements transcoding pipelines using FFmpeg.
// Pipelines process media through FFmpeg input/output contexts.

package transcode

import (
	"errors"
	"github.com/vinq1911/nonchalant/internal/ffx"
)

// Pipeline represents a transcoding pipeline.
// Manages FFmpeg input and output contexts.
type Pipeline struct {
	input  *ffx.Input
	output *ffx.Output
}

// NewPipeline creates a new transcoding pipeline.
// Allocation: Creates pipeline structure, allocates FFmpeg contexts.
func NewPipeline(inputURL, outputURL, format string) (*Pipeline, error) {
	// Create input context
	input, err := ffx.NewInput(inputURL)
	if err != nil {
		return nil, err
	}

	// Create output context
	output, err := ffx.NewOutput(outputURL, format)
	if err != nil {
		_ = input.Close()
		return nil, err
	}

	return &Pipeline{
		input:  input,
		output: output,
	}, nil
}

// Close closes the pipeline and frees all resources.
// All FFmpeg allocations must be freed here.
func (p *Pipeline) Close() error {
	var err error
	if p.output != nil {
		if e := p.output.Close(); e != nil {
			err = e
		}
	}
	if p.input != nil {
		if e := p.input.Close(); e != nil {
			err = e
		}
	}
	return err
}

// Process processes media data through the pipeline.
// Allocation: May allocate buffers for transcoding.
func (p *Pipeline) Process(data []byte) error {
	if p == nil {
		return errors.New("pipeline is nil")
	}

	// NOTE: Full implementation would:
	// 1. Decode input packet
	// 2. Transcode if needed
	// 3. Encode output packet
	// 4. Write to output

	// For now, just write packet directly (no transcoding)
	return p.output.WritePacket(data)
}
