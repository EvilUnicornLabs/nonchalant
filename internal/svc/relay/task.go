// If you are AI: This file defines the relay task interface and a shared
// ffmpeg-supervisor base. Pull and push tasks both shell out to ffmpeg so we
// avoid implementing the full RTMP client command protocol ourselves.

package relay

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

// Task represents a relay task (pull or push).
// Tasks run in their own goroutines and manage connection lifecycle.
type Task interface {
	Start(ctx context.Context) error
	Stop() error
	IsRunning() bool
}

// BaseTask provides common functionality for relay tasks.
// It owns the ffmpeg supervisor loop and the two endpoint configuration knobs
// (local RTMP port for pull targets, local HTTP port for push sources).
type BaseTask struct {
	registry  *bus.Registry
	app       string
	name      string
	remoteURL string
	reconnect bool

	mu       sync.Mutex
	running  bool
	stopChan chan struct{}
	stopped  bool

	// Local endpoint configuration. Populated by the relay Manager so pull
	// targets can reach our RTMP ingest and push sources can reach our HTTP-FLV.
	rtmpPort int
	httpPort int
	pubKey   string
	playKey  string
}

// NewBaseTask creates a new base task with common configuration.
// Default ports match the application defaults; the Manager overwrites these
// from the live config before Start() runs.
func NewBaseTask(registry *bus.Registry, app, name, remoteURL string, reconnect bool) *BaseTask {
	return &BaseTask{
		registry:  registry,
		app:       app,
		name:      name,
		remoteURL: remoteURL,
		reconnect: reconnect,
		stopChan:  make(chan struct{}),
		rtmpPort:  1935,
		httpPort:  8081,
	}
}

// App returns the application name.
func (t *BaseTask) App() string { return t.app }

// Name returns the stream name.
func (t *BaseTask) Name() string { return t.name }

// RemoteURL returns the remote RTMP URL.
func (t *BaseTask) RemoteURL() string { return t.remoteURL }

// Registry returns the bus registry.
func (t *BaseTask) Registry() *bus.Registry { return t.registry }

// IsRunning reports whether Start is currently active.
func (t *BaseTask) IsRunning() bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.running
}

// SetRunning is used by Start/Stop to update the running flag.
func (t *BaseTask) SetRunning(running bool) {
	t.mu.Lock()
	t.running = running
	t.mu.Unlock()
}

// StopChan exposes the close-on-stop channel to subclasses.
func (t *BaseTask) StopChan() <-chan struct{} { return t.stopChan }

// Stop signals the task to stop. Safe to call multiple times.
func (t *BaseTask) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stopped {
		return nil
	}
	t.stopped = true
	close(t.stopChan)
	return nil
}

// SetEndpoints lets the Manager push runtime configuration (ports + auth keys)
// down to the task before Start runs.
func (t *BaseTask) SetEndpoints(rtmpPort, httpPort int, publishKey, playKey string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.rtmpPort = rtmpPort
	t.httpPort = httpPort
	t.pubKey = publishKey
	t.playKey = playKey
}

// PublishKey returns the local publish key (or "" for none).
func (t *BaseTask) PublishKey() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.pubKey
}

// PlayKey returns the local play key (or "" for none).
func (t *BaseTask) PlayKey() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.playKey
}

// RTMPPort returns the local RTMP ingest port (relay targets use this).
func (t *BaseTask) RTMPPort() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.rtmpPort
}

// HTTPPort returns the local HTTP service port (relay sources use this).
func (t *BaseTask) HTTPPort() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.httpPort
}

// localRTMPTarget builds rtmp://127.0.0.1:port/{app}/{name}[?key=...] for the
// local ingest endpoint. Used by pull tasks as the ffmpeg output URL.
func (t *BaseTask) localRTMPTarget() string {
	url := fmt.Sprintf("rtmp://127.0.0.1:%d/%s/%s", t.RTMPPort(), t.app, t.name)
	if k := t.PublishKey(); k != "" {
		url += "?key=" + k
	}
	return url
}

// localFLVSource builds http://127.0.0.1:port/{app}/{name}.flv[?key=...] for
// the local HTTP-FLV endpoint. Used by push tasks as the ffmpeg input URL.
func (t *BaseTask) localFLVSource() string {
	url := fmt.Sprintf("http://127.0.0.1:%d/%s/%s.flv", t.HTTPPort(), t.app, t.name)
	if k := t.PlayKey(); k != "" {
		url += "?key=" + k
	}
	return url
}

// runFFmpegLoop spawns ffmpeg with the given args and supervises it.
// It restarts on subprocess exit while reconnect is true, with bounded
// exponential backoff. Returns nil on context cancellation or Stop.
func (t *BaseTask) runFFmpegLoop(parent context.Context, label string, args []string) error {
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		return fmt.Errorf("relay %s: ffmpeg not on PATH", label)
	}
	log.Printf("relay %s: starting", label)
	const minBackoff = 500 * time.Millisecond
	const maxBackoff = 5 * time.Second
	backoff := minBackoff
	for {
		select {
		case <-parent.Done():
			return nil
		case <-t.StopChan():
			return nil
		default:
		}

		ctx, cancel := context.WithCancel(parent)
		stopped := watchStop(ctx, cancel, t.StopChan())

		cmd := exec.CommandContext(ctx, "ffmpeg", args...)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		// Cancel the per-attempt context so the watchStop goroutine returns
		// even on a clean ffmpeg exit (Run does not cancel ctx by itself).
		cancel()
		<-stopped

		if parent.Err() != nil || ctxStopped(t.StopChan()) {
			// Subprocess was killed because we're shutting down. The ffmpeg
			// exit error is the intentional cause of the kill, so swallow it.
			return nil //nolint:nilerr // shutdown is the cause of err
		}
		if !t.reconnect {
			return err
		}
		log.Printf("relay %s: ffmpeg exited (%v); retry in %s", label, err, backoff)
		select {
		case <-time.After(backoff):
		case <-parent.Done():
			return nil
		case <-t.StopChan():
			return nil
		}
		if backoff < maxBackoff {
			backoff *= 2
			if backoff > maxBackoff {
				backoff = maxBackoff
			}
		}
	}
}

// watchStop bridges the StopChan into the per-attempt context so a Stop()
// during ffmpeg execution promptly terminates the subprocess.
func watchStop(ctx context.Context, cancel context.CancelFunc, stop <-chan struct{}) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		select {
		case <-ctx.Done():
		case <-stop:
			cancel()
		}
	}()
	return done
}

// ctxStopped is true if the stop channel has been closed.
func ctxStopped(c <-chan struct{}) bool {
	select {
	case <-c:
		return true
	default:
		return false
	}
}
