//go:build !ffmpeg
// +build !ffmpeg

// If you are AI: This file provides stub implementations for transcode manager.
// Used when FFmpeg is not available.

package transcode

import (
	"github.com/vinq1911/nonchalant/internal/config"
	"github.com/vinq1911/nonchalant/internal/core/bus"
)

// Manager manages transcoding tasks.
// Stub implementation.
type Manager struct {
	registry *bus.Registry
}

// NewManager creates a new transcode manager.
// Stub: returns manager that does nothing.
func NewManager(registry *bus.Registry) *Manager {
	return &Manager{
		registry: registry,
	}
}

// StartTasks starts transcoding tasks from configuration.
// Stub: returns nil (no tasks to start).
func (m *Manager) StartTasks(cfg *config.Config) error {
	// FFmpeg not available, no tasks to start
	return nil
}

// Stop stops all transcoding tasks.
// Stub: no-op.
func (m *Manager) Stop() error {
	return nil
}

// TaskCount returns the number of active transcoding tasks.
// Stub: always returns 0.
func (m *Manager) TaskCount() int {
	return 0
}
