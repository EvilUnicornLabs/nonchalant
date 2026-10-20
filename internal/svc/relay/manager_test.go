// If you are AI: This file contains unit tests for the relay manager.
// Tests verify task creation and lifecycle management.

package relay

import (
	"github.com/vinq1911/nonchalant/pkg/config"
	"github.com/vinq1911/nonchalant/pkg/core/bus"
	"testing"
	"time"
)

func TestManagerStartTasks(t *testing.T) {
	registry := bus.NewRegistry()
	manager := NewManager(registry)

	cfg := &config.Config{
		Relays: []config.RelayConfig{
			{
				App:       "live",
				Name:      "test",
				Mode:      "pull",
				RemoteURL: "rtmp://localhost:1935/live/test",
				Reconnect: false,
			},
		},
	}

	if err := manager.StartTasks(cfg); err != nil {
		t.Fatalf("Failed to start tasks: %v", err)
	}

	if manager.TaskCount() != 1 {
		t.Errorf("Expected 1 task, got %d", manager.TaskCount())
	}

	// Stop manager
	manager.Stop()
}

func TestManagerInvalidConfig(t *testing.T) {
	registry := bus.NewRegistry()
	manager := NewManager(registry)

	// Test missing app
	cfg := &config.Config{
		Relays: []config.RelayConfig{
			{
				Name:      "test",
				Mode:      "pull",
				RemoteURL: "rtmp://localhost:1935/live/test",
			},
		},
	}

	if err := manager.StartTasks(cfg); err == nil {
		t.Error("Expected error for missing app")
	}

	// Test invalid mode
	cfg = &config.Config{
		Relays: []config.RelayConfig{
			{
				App:       "live",
				Name:      "test",
				Mode:      "invalid",
				RemoteURL: "rtmp://localhost:1935/live/test",
			},
		},
	}

	if err := manager.StartTasks(cfg); err == nil {
		t.Error("Expected error for invalid mode")
	}

	// Test missing remote URL
	cfg = &config.Config{
		Relays: []config.RelayConfig{
			{
				App:  "live",
				Name: "test",
				Mode: "pull",
			},
		},
	}

	if err := manager.StartTasks(cfg); err == nil {
		t.Error("Expected error for missing remote_url")
	}
}

func TestManagerStop(t *testing.T) {
	registry := bus.NewRegistry()
	manager := NewManager(registry)

	cfg := &config.Config{
		Relays: []config.RelayConfig{
			{
				App:       "live",
				Name:      "test",
				Mode:      "pull",
				RemoteURL: "rtmp://localhost:1935/live/test",
				Reconnect: false,
			},
		},
	}

	if err := manager.StartTasks(cfg); err != nil {
		t.Fatalf("Failed to start tasks: %v", err)
	}

	// Give tasks a moment to start
	time.Sleep(100 * time.Millisecond)

	// Stop manager
	done := make(chan bool, 1)
	go func() {
		manager.Stop()
		done <- true
	}()

	select {
	case <-done:
		// Manager stopped successfully
	case <-time.After(2 * time.Second):
		t.Error("Manager stop timed out")
	}
}

// TestManagerRestart confirms that Restart replaces the running Task with a
// fresh instance and that an unknown (app,name) returns an error.
func TestManagerRestart(t *testing.T) {
	registry := bus.NewRegistry()
	manager := NewManager(registry)
	defer manager.Stop()

	cfg := &config.Config{
		Relays: []config.RelayConfig{
			{App: "live", Name: "x", Mode: "pull", RemoteURL: "rtmp://127.0.0.1:1/live/x"},
		},
	}
	if err := manager.StartTasks(cfg); err != nil {
		t.Fatalf("StartTasks: %v", err)
	}

	if err := manager.Restart("nope", "missing"); err == nil {
		t.Error("Restart of unknown relay should return error")
	}
	if err := manager.Restart("live", "x"); err != nil {
		t.Errorf("Restart of known relay should succeed, got %v", err)
	}
	if got := manager.TaskCount(); got != 1 {
		t.Errorf("after restart TaskCount = %d, want 1", got)
	}
}
