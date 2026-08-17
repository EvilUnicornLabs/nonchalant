// If you are AI: This file implements the WebSocket handler for FLV stream requests.
// Handles GET /ws/{app}/{name} requests and manages subscriber lifecycle.

package wsflv

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/vinq1911/nonchalant/pkg/core/bus"

	"github.com/gorilla/websocket"
)

// waitForStreams polls the stream's cached init data and returns the
// (hasAudio, hasVideo) flags suitable for the FLV header. Mirrors the
// httpflv version — see that file for rationale.
func waitForStreams(ctx context.Context, stream *bus.Stream, timeout time.Duration) (bool, bool) {
	deadline := time.Now().Add(timeout)
	for {
		hasAudio := stream.HasAudioInit()
		hasVideo := stream.HasVideoInit()
		if (hasAudio && hasVideo) || time.Now().After(deadline) {
			return hasAudio, hasVideo
		}
		select {
		case <-ctx.Done():
			return hasAudio, hasVideo
		case <-time.After(50 * time.Millisecond):
		}
	}
}

// Handler handles WebSocket-FLV requests.
type Handler struct {
	registry *bus.Registry
	upgrader websocket.Upgrader
}

// NewHandler creates a new WebSocket-FLV handler.
func NewHandler(registry *bus.Registry) *Handler {
	return &Handler{
		registry: registry,
		upgrader: websocket.Upgrader{
			CheckOrigin: func(r *http.Request) bool {
				// Allow all origins for now
				// NOTE: In production, this should be restricted
				return true
			},
		},
	}
}

// ServeHTTP handles WebSocket upgrade and FLV streaming.
// Endpoint: GET /ws/{app}/{name}
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /ws/{app}/{name}
	urlPath := strings.TrimPrefix(r.URL.Path, "/ws/")
	if urlPath == r.URL.Path {
		// Path doesn't start with /ws/
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Split into app and name
	parts := strings.SplitN(urlPath, "/", 2)
	if len(parts) != 2 {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	app := parts[0]
	name := parts[1]

	// Get stream from registry
	streamKey := bus.NewStreamKey(app, name)
	stream := h.registry.Get(streamKey)
	if stream == nil {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Check if stream has a publisher
	if !stream.HasPublisher() {
		w.WriteHeader(http.StatusNotFound)
		return
	}

	// Upgrade to WebSocket
	conn, err := h.upgrader.Upgrade(w, r, nil)
	if err != nil {
		// Upgrade failed, response already sent
		return
	}

	// Create subscriber
	sub := NewSubscriber(conn, stream)
	defer func() {
		sub.Detach()
		_ = conn.Close()
	}()

	// Attach to stream
	sub.Attach()

	// Wait briefly for codec init data so the FLV header reflects only the
	// streams that actually exist; otherwise ffmpeg/flv.js can hang waiting
	// for an audio packet that never arrives.
	hasAudio, hasVideo := waitForStreams(r.Context(), stream, 2*time.Second)
	if err := sub.WriteHeader(hasAudio, hasVideo); err != nil {
		return
	}

	// Process messages until the request context is cancelled (server shutdown
	// or client disconnect) or a write error fires.
	if err := sub.ProcessMessages(r.Context()); err != nil {
		return
	}
}

// RegisterRoutes registers WebSocket-FLV routes on the given mux.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/ws/", h.ServeHTTP)
}
