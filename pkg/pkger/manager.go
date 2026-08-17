// If you are AI: This file manages the per-stream packager lifecycle.
// One Manager owns N packagers (one per (app,name,format) tuple).

package pkger

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

// Manager owns and reaps packager instances.
// It is safe for concurrent use.
type Manager struct {
	registry *bus.Registry
	httpPort int
	idleTTL  time.Duration
	opts     Options

	mu      sync.Mutex
	pkgers  map[string]*Packager
	rootDir string
	ctx     context.Context
	cancel  context.CancelFunc
	gcDone  chan struct{}
}

// NewManager creates a Manager. httpPort is used to construct the source URL
// that ffmpeg pulls FLV from (we re-package our own output).
// idleTTL is how long a packager may sit unused before it is reaped.
func NewManager(registry *bus.Registry, httpPort int, idleTTL time.Duration, opts Options) (*Manager, error) {
	rootDir, err := os.MkdirTemp("", "nonchalant-pkger-")
	if err != nil {
		return nil, fmt.Errorf("mkdir root: %w", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		registry: registry,
		httpPort: httpPort,
		idleTTL:  idleTTL,
		opts:     opts,
		pkgers:   make(map[string]*Packager),
		rootDir:  rootDir,
		ctx:      ctx,
		cancel:   cancel,
		gcDone:   make(chan struct{}),
	}
	go m.gcLoop()
	return m, nil
}

// GetOrCreate returns the live packager for (app,name,format), starting one
// if necessary. Returns an error if the underlying stream has no publisher
// or if ffmpeg fails to launch.
func (m *Manager) GetOrCreate(app, name string, format Format) (*Packager, error) {
	stream := m.registry.Get(bus.NewStreamKey(app, name))
	if stream == nil || !stream.HasPublisher() {
		return nil, fmt.Errorf("stream not live: %s/%s", app, name)
	}

	key := keyFor(app, name, format)
	m.mu.Lock()
	defer m.mu.Unlock()
	if p, ok := m.pkgers[key]; ok {
		p.Touch()
		return p, nil
	}

	workDir := filepath.Join(m.rootDir, fmt.Sprintf("%s-%s-%s", app, name, format))
	sourceURL := fmt.Sprintf("http://127.0.0.1:%d/%s/%s.flv", m.httpPort, app, name)
	p := newPackager(app, name, format, sourceURL, workDir, m.opts)
	if err := p.Start(m.ctx); err != nil {
		return nil, err
	}
	m.pkgers[key] = p
	return p, nil
}

// Stop terminates all running packagers and removes the root temp dir.
func (m *Manager) Stop() {
	m.mu.Lock()
	pkgers := m.pkgers
	m.pkgers = nil
	m.mu.Unlock()
	for _, p := range pkgers {
		p.Stop()
	}
	m.cancel()
	<-m.gcDone
	_ = os.RemoveAll(m.rootDir)
}

// gcLoop periodically reaps idle packagers.
func (m *Manager) gcLoop() {
	defer close(m.gcDone)
	tick := time.NewTicker(15 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-tick.C:
			m.gcOnce()
		}
	}
}

// gcOnce kills any packager that has been idle for >= idleTTL.
func (m *Manager) gcOnce() {
	now := time.Now()
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, p := range m.pkgers {
		if now.Sub(p.LastAccess()) >= m.idleTTL {
			p.Stop()
			delete(m.pkgers, k)
		}
	}
}

// keyFor produces the map key for a packager tuple.
func keyFor(app, name string, format Format) string {
	return fmt.Sprintf("%s|%s|%s", app, name, format)
}
