// If you are AI: This file implements a per-stream HLS/DASH packager.
// Each packager owns an ffmpeg subprocess that pulls our own HTTP-FLV stream
// and writes segments to a temp directory served by the HTTP handler.

package pkger

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Format selects the packaging output type.
type Format string

const (
	// FormatHLS produces an .m3u8 playlist + .ts segments.
	FormatHLS Format = "hls"
	// FormatDASH produces an .mpd manifest + .m4s segments + an init.mp4.
	FormatDASH Format = "dash"
)

// Packager wraps a single ffmpeg subprocess that re-packages one stream.
// Exactly one Packager exists per (app, name, format) tuple while it is alive.
type Packager struct {
	app, name string
	format    Format
	sourceURL string
	workDir   string
	manifest  string
	opts      Options

	mu         sync.Mutex
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	startedAt  time.Time
	lastAccess time.Time
	stopped    bool
	startErr   error
}

// newPackager allocates a packager. It does not start ffmpeg yet — call Start.
func newPackager(app, name string, format Format, sourceURL, workDir string, opts Options) *Packager {
	manifest := manifestName(format, opts)
	return &Packager{
		app:       app,
		name:      name,
		format:    format,
		sourceURL: sourceURL,
		workDir:   workDir,
		manifest:  manifest,
		opts:      opts,
	}
}

// manifestName returns the top-level manifest filename ffmpeg writes.
// We use "index.m3u8" for both single-rendition and ABR HLS so the canonical
// URL `/hls/{app}/{name}/index.m3u8` has segments as URL siblings — a player
// resolving relative URIs from the playlist gets the right paths. DASH uses
// "manifest.mpd" for the same reason.
func manifestName(format Format, opts Options) string {
	if format == FormatDASH {
		return "manifest.mpd"
	}
	_ = opts
	return "index.m3u8"
}

// Start launches the ffmpeg subprocess. Returns immediately; the caller must
// call WaitReady to block until the manifest appears (or a timeout/abort).
func (p *Packager) Start(parent context.Context) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil {
		return nil
	}

	if err := os.MkdirAll(p.workDir, 0o755); err != nil {
		p.startErr = fmt.Errorf("mkdir workdir: %w", err)
		return p.startErr
	}

	args := p.ffmpegArgs()
	ctx, cancel := context.WithCancel(parent)
	cmd := exec.CommandContext(ctx, "ffmpeg", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		cancel()
		p.startErr = fmt.Errorf("start ffmpeg: %w", err)
		return p.startErr
	}

	p.cmd = cmd
	p.cancel = cancel
	p.startedAt = time.Now()
	p.lastAccess = p.startedAt
	go p.reap()
	return nil
}

// (ffmpegArgs lives in args.go.)

// reap waits for the subprocess and cleans up the work dir.
// On normal stop (Stop -> cancel) the work dir is removed.
func (p *Packager) reap() {
	_ = p.cmd.Wait()
	p.mu.Lock()
	p.stopped = true
	p.mu.Unlock()
	_ = os.RemoveAll(p.workDir)
}

// WaitReady blocks until the manifest file is non-empty, the context is done,
// or the timeout elapses.
func (p *Packager) WaitReady(ctx context.Context, timeout time.Duration) error {
	manifestPath := filepath.Join(p.workDir, p.manifest)
	deadline := time.Now().Add(timeout)
	tick := time.NewTicker(150 * time.Millisecond)
	defer tick.Stop()
	for {
		if info, err := os.Stat(manifestPath); err == nil && info.Size() > 0 {
			return nil
		}
		p.mu.Lock()
		stopped, startErr := p.stopped, p.startErr
		p.mu.Unlock()
		if startErr != nil {
			return startErr
		}
		if stopped {
			return errors.New("packager exited before producing manifest")
		}
		if time.Now().After(deadline) {
			return errors.New("packager not ready before timeout")
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-tick.C:
		}
	}
}

// Touch records that the packager has been used now. Used by idle GC.
func (p *Packager) Touch() {
	p.mu.Lock()
	p.lastAccess = time.Now()
	p.mu.Unlock()
}

// LastAccess returns the most recent Touch time.
func (p *Packager) LastAccess() time.Time {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.lastAccess
}

// WorkDir returns the directory where the packager writes its segments.
func (p *Packager) WorkDir() string { return p.workDir }

// Manifest returns the manifest filename ("stream.m3u8" or "manifest.mpd").
func (p *Packager) Manifest() string { return p.manifest }

// Stop terminates the ffmpeg subprocess and cleans up. Safe to call twice.
func (p *Packager) Stop() {
	p.mu.Lock()
	cancel := p.cancel
	p.cancel = nil
	p.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}
