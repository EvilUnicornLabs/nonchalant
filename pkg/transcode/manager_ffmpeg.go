//go:build ffmpeg
// +build ffmpeg

// If you are AI: This file implements the transcode manager for FFmpeg-based transcoding.
// Manages lifecycle of transcoding tasks.

package transcode

import (
	"context"
	"github.com/vinq1911/nonchalant/pkg/config"
	"github.com/vinq1911/nonchalant/pkg/core/bus"
	"sync"
)

// Manager manages transcoding tasks.
type Manager struct {
	registry *bus.Registry
	tasks    []Task
	wg       sync.WaitGroup
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
}

// NewManager creates a new transcode manager.
func NewManager(registry *bus.Registry) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		registry: registry,
		tasks:    make([]Task, 0),
		ctx:      ctx,
		cancel:   cancel,
	}
}

// StartTasks starts transcoding tasks from configuration.
func (m *Manager) StartTasks(cfg *config.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// NOTE: Full implementation would:
	// 1. Parse transcode configuration from cfg
	// 2. Create transcoding tasks for each profile
	// 3. Start tasks in goroutines
	// 4. Tasks subscribe to bus streams and transcode

	// For now, return nil (no tasks configured)
	return nil
}

// Stop stops all transcoding tasks and waits for them to finish.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	// Cancel context to signal all tasks to stop
	m.cancel()

	// Stop all tasks
	for _, task := range m.tasks {
		_ = task.Stop()
	}

	// Wait for all tasks to finish
	done := make(chan struct{})
	go func() {
		m.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-m.ctx.Done():
		return nil
	}
}

// TaskCount returns the number of active transcoding tasks.
func (m *Manager) TaskCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.tasks)
}
