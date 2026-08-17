// If you are AI: This file implements the HTTP server lifecycle and routing.

package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/http/pprof"
	"time"

	"github.com/vinq1911/nonchalant/internal/auth"
	"github.com/vinq1911/nonchalant/internal/config"
	"github.com/vinq1911/nonchalant/internal/core/bus"
	"github.com/vinq1911/nonchalant/internal/svc/api"
	"github.com/vinq1911/nonchalant/internal/svc/health"
	"github.com/vinq1911/nonchalant/internal/svc/httpflv"
	"github.com/vinq1911/nonchalant/internal/svc/metrics"
	"github.com/vinq1911/nonchalant/internal/svc/pkger"
	"github.com/vinq1911/nonchalant/internal/svc/relay"
	"github.com/vinq1911/nonchalant/internal/svc/rtmp"
	"github.com/vinq1911/nonchalant/internal/svc/transcode"
	"github.com/vinq1911/nonchalant/internal/svc/wsflv"
)

// Server wraps the HTTP server and its dependencies.
type Server struct {
	httpServer   *http.Server
	healthSvc    *health.Service
	apiSvc       *api.Service
	metricsSvc   *metrics.Service
	pkgerSvc     *pkger.Service
	httpflvSvc   *httpflv.Service
	wsflvSvc     *wsflv.Service
	rtmpServer   *rtmp.Server
	relayMgr     *relay.Manager
	transcodeMgr *transcode.Manager
	registry     *bus.Registry
}

// New creates a new server instance with the given configuration.
// The server is not started until Start is called.
func New(cfg *config.Config) *Server {
	mux := http.NewServeMux()

	healthSvc := health.New()
	healthSvc.RegisterRoutes(mux)

	// Create bus registry
	registry := bus.NewRegistry()

	// Build the publish and play key sets. nil → anonymous (backward
	// compatible). Both come from cfg.Auth.
	publishKeys := auth.NewKeySet(cfg.Auth.PublishKeys)
	playKeys := auth.NewKeySet(cfg.Auth.PlayKeys)

	rtmpServer := rtmp.NewServer(registry, publishKeys)

	// Create relay manager and tell it where our local listeners are so it
	// can build URLs back into us when relays reconnect via ffmpeg. The first
	// configured publish/play key (if any) is reused for the relay's own auth
	// against our endpoints.
	relayMgr := relay.NewManager(registry)
	relayMgr.SetEndpoints(
		cfg.Server.RTMPPort, cfg.Server.HTTPPort,
		firstKey(cfg.Auth.PublishKeys), firstKey(cfg.Auth.PlayKeys),
	)

	// Create transcode manager (optional, works with or without FFmpeg)
	transcodeMgr := transcode.NewManager(registry)

	// Register API and metrics BEFORE httpflv — httpflv mounts a catch-all "/"
	// pattern which would otherwise mask /api/* and /metrics.
	apiSvc := api.NewService(registry, relayMgr)
	apiSvc.RegisterRoutes(mux)

	metricsSvc := metrics.NewService(registry, relayMgr)
	metricsSvc.RegisterRoutes(mux)

	// pprof: /debug/pprof/* (CPU, heap, goroutine, allocs, mutex, block).
	// Mounted before httpflv's catch-all so the routes are reachable.
	mux.HandleFunc("/debug/pprof/", pprof.Index)
	mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

	// HLS / DASH packager service. If creation fails (e.g. no writable temp
	// directory) we log and continue — the rest of the server still works.
	pkgerSvc, pkgerErr := pkger.NewService(registry, cfg.Server.HTTPPort, playKeys,
		pkger.Options{
			LowLatency: cfg.HLS.LowLatency,
			Ladder:     ladderToPkger(cfg.HLS.Ladder),
		})
	if pkgerErr != nil {
		log.Printf("HLS/DASH packager disabled: %v", pkgerErr)
	} else {
		pkgerSvc.RegisterRoutes(mux)
	}

	// Create WebSocket-FLV service (uses a distinct /ws/ prefix)
	wsflvSvc := wsflv.NewService(registry, playKeys)
	wsflvSvc.RegisterRoutes(mux)

	// Create HTTP-FLV service (catch-all on "/", must register last)
	httpflvSvc := httpflv.NewService(registry, playKeys)
	httpflvSvc.RegisterRoutes(mux)

	// HTTP server listens on HTTP port
	// Health endpoint is also available on this port
	// NOTE: Health port is kept for backward compatibility but not used
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.HTTPPort),
		Handler: mux,
	}

	return &Server{
		httpServer:   httpServer,
		healthSvc:    healthSvc,
		apiSvc:       apiSvc,
		metricsSvc:   metricsSvc,
		pkgerSvc:     pkgerSvc,
		httpflvSvc:   httpflvSvc,
		wsflvSvc:     wsflvSvc,
		rtmpServer:   rtmpServer,
		relayMgr:     relayMgr,
		transcodeMgr: transcodeMgr,
		registry:     registry,
	}
}

// Start begins serving HTTP requests and RTMP connections.
// This method blocks until the server is stopped or encounters an error.
func (s *Server) Start(cfg *config.Config) error {
	// Start relay tasks
	if err := s.relayMgr.StartTasks(cfg); err != nil {
		return fmt.Errorf("start relay tasks: %w", err)
	}

	// Start transcode tasks (optional, only if configured)
	if s.transcodeMgr != nil {
		if err := s.transcodeMgr.StartTasks(cfg); err != nil {
			return fmt.Errorf("start transcode tasks: %w", err)
		}
	}

	// Start RTMP server
	if err := s.rtmpServer.Listen(fmt.Sprintf(":%d", cfg.Server.RTMPPort)); err != nil {
		return fmt.Errorf("RTMP server listen: %w", err)
	}
	go func() {
		if err := s.rtmpServer.Accept(); err != nil {
			log.Printf("RTMP accept loop exited: %v", err)
		}
	}()

	// Start HTTP server (blocks)
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully stops the server with a timeout.
// Returns an error if shutdown fails or times out.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

// ShutdownWithTimeout stops the server with a fixed 5-second timeout.
// This is a convenience wrapper around Shutdown.
func (s *Server) ShutdownWithTimeout() error {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stop relay manager
	if s.relayMgr != nil {
		if err := s.relayMgr.Stop(); err != nil {
			log.Printf("relay manager stop: %v", err)
		}
	}

	// Stop transcode manager
	if s.transcodeMgr != nil {
		if err := s.transcodeMgr.Stop(); err != nil {
			log.Printf("transcode manager stop: %v", err)
		}
	}

	// Close RTMP server
	if s.rtmpServer != nil {
		if err := s.rtmpServer.Close(); err != nil {
			log.Printf("rtmp server close: %v", err)
		}
	}

	// Stop HLS/DASH packager (kills any spawned ffmpeg subprocesses)
	if s.pkgerSvc != nil {
		s.pkgerSvc.Stop()
	}

	return s.Shutdown(ctx)
}

// firstKey returns the first non-empty key in keys, or "" if there are none.
// Used to pick a default identity for outbound relay connections.
func firstKey(keys []string) string {
	for _, k := range keys {
		if k != "" {
			return k
		}
	}
	return ""
}

// ladderToPkger maps the YAML ABR ladder onto the pkger-local rung type.
// Avoids forcing the pkger package to import config (and the cycle that comes
// with it).
func ladderToPkger(rungs []config.LadderRung) []pkger.LadderRung {
	if len(rungs) == 0 {
		return nil
	}
	out := make([]pkger.LadderRung, len(rungs))
	for i, r := range rungs {
		out[i] = pkger.LadderRung{
			Name:         r.Name,
			Width:        r.Width,
			Height:       r.Height,
			VideoBitrate: r.VideoBitrate,
			AudioBitrate: r.AudioBitrate,
			AudioOnly:    r.AudioOnly,
		}
	}
	return out
}
