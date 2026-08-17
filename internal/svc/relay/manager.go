// If you are AI: This file implements the relay manager.
// Manages lifecycle of all relay tasks (start, stop, restart).

package relay

import (
	"context"
	"fmt"
	"log"
	"sync"

	"github.com/vinq1911/nonchalant/pkg/config"
	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

// slot binds one configured relay to its currently-running Task instance and
// the WaitGroup-tracked goroutine running it. Restart() destroys and recreates
// the Task and goroutine while keeping the slot's config in place.
type slot struct {
	cfg  config.RelayConfig
	task Task
	done chan struct{} // closed when the goroutine returns
}

// slotKey is the map key used for restart lookups: app|name uniquely identifies
// a relay because two relays for the same (app,name) make no sense.
func slotKey(app, name string) string { return app + "|" + name }

// Manager manages relay tasks lifecycle.
type Manager struct {
	registry *bus.Registry
	mu       sync.Mutex
	slots    map[string]*slot
	ctx      context.Context
	cancel   context.CancelFunc

	// Endpoint configuration injected by the parent server. These are passed
	// to each Task so it can build URLs to our own RTMP/HTTP listeners.
	rtmpPort   int
	httpPort   int
	publishKey string
	playKey    string
}

// NewManager creates a new relay manager.
func NewManager(registry *bus.Registry) *Manager {
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		registry: registry,
		slots:    make(map[string]*slot),
		ctx:      ctx,
		cancel:   cancel,
		rtmpPort: 1935,
		httpPort: 8081,
	}
}

// SetEndpoints lets the server tell the relay where its own RTMP and HTTP
// services live, and which (optional) auth keys it should use when relays
// connect back into this server. Must be called before StartTasks.
func (m *Manager) SetEndpoints(rtmpPort, httpPort int, publishKey, playKey string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rtmpPort = rtmpPort
	m.httpPort = httpPort
	m.publishKey = publishKey
	m.playKey = playKey
}

// StartTasks starts all relay tasks from configuration.
func (m *Manager) StartTasks(cfg *config.Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, relayCfg := range cfg.Relays {
		if err := validateRelay(relayCfg); err != nil {
			return err
		}
		if _, dup := m.slots[slotKey(relayCfg.App, relayCfg.Name)]; dup {
			return fmt.Errorf("duplicate relay for %s/%s", relayCfg.App, relayCfg.Name)
		}
		m.slots[slotKey(relayCfg.App, relayCfg.Name)] = m.spawn(relayCfg)
	}

	return nil
}

// validateRelay enforces required fields and known modes.
func validateRelay(cfg config.RelayConfig) error {
	if cfg.App == "" || cfg.Name == "" {
		return fmt.Errorf("relay config missing app or name")
	}
	if cfg.Mode != "pull" && cfg.Mode != "push" {
		return fmt.Errorf("invalid relay mode: %s (must be 'pull' or 'push')", cfg.Mode)
	}
	if cfg.RemoteURL == "" {
		return fmt.Errorf("relay config missing remote_url")
	}
	return nil
}

// spawn allocates a fresh Task for cfg and runs it in a goroutine. Caller must
// hold m.mu.
func (m *Manager) spawn(cfg config.RelayConfig) *slot {
	var task Task
	switch cfg.Mode {
	case "pull":
		pt := NewPullTask(m.registry, cfg.App, cfg.Name, cfg.RemoteURL, cfg.Reconnect)
		pt.SetEndpoints(m.rtmpPort, m.httpPort, m.publishKey, m.playKey)
		task = pt
	default: // "push" (validated upstream)
		pt := NewPushTask(m.registry, cfg.App, cfg.Name, cfg.RemoteURL, cfg.Reconnect)
		pt.SetEndpoints(m.rtmpPort, m.httpPort, m.publishKey, m.playKey)
		task = pt
	}
	s := &slot{cfg: cfg, task: task, done: make(chan struct{})}
	go func() {
		defer close(s.done)
		if err := task.Start(m.ctx); err != nil && m.ctx.Err() == nil {
			log.Printf("relay task %s/%s exited: %v", cfg.App, cfg.Name, err)
		}
	}()
	return s
}

// Restart stops the relay for app/name and starts a fresh instance with the
// same configuration. Returns an error if the relay is not configured.
func (m *Manager) Restart(app, name string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := slotKey(app, name)
	s, ok := m.slots[key]
	if !ok {
		return fmt.Errorf("no relay configured for %s/%s", app, name)
	}
	if err := s.task.Stop(); err != nil {
		log.Printf("relay task stop during restart: %v", err)
	}
	<-s.done
	m.slots[key] = m.spawn(s.cfg)
	return nil
}

// Stop stops all relay tasks and waits for them to finish.
// FIXME: If a task cannot stop cleanly, it may block shutdown.
// Workaround: Use context timeout in caller.
func (m *Manager) Stop() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.cancel()
	for _, s := range m.slots {
		if err := s.task.Stop(); err != nil {
			log.Printf("relay task stop: %v", err)
		}
	}
	for _, s := range m.slots {
		<-s.done
	}
	return nil
}

// TaskCount returns the number of active relay tasks.
func (m *Manager) TaskCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.slots)
}

// GetTasks returns information about all relay tasks.
// Used by API for introspection.
func (m *Manager) GetTasks() []TaskInfo {
	m.mu.Lock()
	defer m.mu.Unlock()

	infos := make([]TaskInfo, 0, len(m.slots))
	for _, s := range m.slots {
		var running bool
		switch t := s.task.(type) {
		case *PullTask:
			running = t.IsRunning()
		case *PushTask:
			running = t.IsRunning()
		}
		infos = append(infos, TaskInfo{
			App:       s.cfg.App,
			Name:      s.cfg.Name,
			Mode:      s.cfg.Mode,
			RemoteURL: s.cfg.RemoteURL,
			Running:   running,
		})
	}
	return infos
}

// TaskInfo represents information about a relay task.
type TaskInfo struct {
	App       string
	Name      string
	Mode      string
	RemoteURL string
	Running   bool
}
