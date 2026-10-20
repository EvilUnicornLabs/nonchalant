//go:build ffmpeg
// +build ffmpeg

// If you are AI: This file contains tests for transcode manager.
// Tests verify manager lifecycle and task management.

package transcode

import (
	"github.com/vinq1911/nonchalant/pkg/core/bus"
	"testing"
)

func TestManagerNew(t *testing.T) {
	registry := bus.NewRegistry()
	manager := NewManager(registry)

	if manager == nil {
		t.Fatal("NewManager returned nil")
	}

	if manager.TaskCount() != 0 {
		t.Errorf("Expected 0 tasks, got %d", manager.TaskCount())
	}
}

func TestManagerStartTasks(t *testing.T) {
	registry := bus.NewRegistry()
	manager := NewManager(registry)

	// NOTE: Full test would require config with transcode profiles
	// For now, test that it doesn't crash with nil config
	// This should not crash even without proper config
	// (manager will handle nil/empty config gracefully)
	_ = manager.StartTasks(nil)

	if manager.TaskCount() != 0 {
		t.Errorf("Expected 0 tasks, got %d", manager.TaskCount())
	}
}

func TestManagerStop(t *testing.T) {
	registry := bus.NewRegistry()
	manager := NewManager(registry)

	// Stop should not crash even with no tasks
	if err := manager.Stop(); err != nil {
		t.Errorf("Stop() returned error: %v", err)
	}
}
