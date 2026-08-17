// If you are AI: This file implements the HTTP handler for FLV stream requests.
// Handles GET /{app}/{name}.flv requests and manages subscriber lifecycle.

package httpflv

import (
	"context"
	"net/http"
	"path"
	"strings"
	"time"

	"github.com/vinq1911/nonchalant/internal/core/bus"
)

// waitForStreams polls the stream's cached init data and returns the
// (hasAudio, hasVideo) flags suitable for the FLV header. It returns
// when both flags are true, when ctx ends, or when timeout elapses —
// whichever comes first. If a publisher only ever produces video, this
// drops out at timeout with hasAudio=false.
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

// Handler handles HTTP-FLV requests.
type Handler struct {
	registry *bus.Registry
}

// NewHandler creates a new HTTP-FLV handler.
func NewHandler(registry *bus.Registry) *Handler {
	return &Handler{
		registry: registry,
	}
}

// ServeHTTP handles HTTP requests for FLV streams.
// Endpoint: GET /{app}/{name}.flv
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Parse path: /{app}/{name}.flv
	urlPath := strings.TrimPrefix(r.URL.Path, "/")
	if !strings.HasSuffix(urlPath, ".flv") {
		w.WriteHeader(http.StatusBadRequest)
		return
	}

	// Remove .flv extension
	streamPath := strings.TrimSuffix(urlPath, ".flv")

	// Split into app and name
	parts := strings.SplitN(streamPath, "/", 2)
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

	// Hijack the connection so we can write raw FLV bytes directly to the
	// TCP socket. This bypasses Go's HTTP chunked-transfer encoding and the
	// double-flush (bufio + http.ResponseWriter.Flush) that pprof showed
	// dominating CPU at high fan-out — each FLV tag becomes exactly one
	// syscall.write instead of two.
	hijacker, ok := w.(http.Hijacker)
	if !ok {
		http.Error(w, "stream hijack unsupported", http.StatusInternalServerError)
		return
	}
	conn, _, err := hijacker.Hijack()
	if err != nil {
		http.Error(w, "hijack failed", http.StatusInternalServerError)
		return
	}
	defer conn.Close()

	// Send a minimal HTTP/1.1 response. NOTE: no Transfer-Encoding — without
	// it the response is "until close", which is what we want for live FLV.
	// We're now responsible for the connection's lifetime.
	headers := "HTTP/1.1 200 OK\r\n" +
		"Content-Type: video/x-flv\r\n" +
		"Cache-Control: no-cache\r\n" +
		"Connection: close\r\n" +
		"Access-Control-Allow-Origin: *\r\n" +
		"\r\n"
	if _, err := conn.Write([]byte(headers)); err != nil {
		return
	}

	sub := NewSubscriber(conn, stream)
	defer sub.Detach()
	sub.Attach()

	// Wait briefly for the publisher's codec init data so we can claim only
	// the streams that actually exist in the FLV header. Claiming audio when
	// none is present makes ffmpeg's analyzer hang in find_stream_info.
	hasAudio, hasVideo := waitForStreams(r.Context(), stream, 2*time.Second)
	if err := sub.WriteHeader(hasAudio, hasVideo); err != nil {
		return
	}

	// Process messages until the request context is cancelled (server shutdown
	// or client disconnect) or a write error fires.
	_ = sub.ProcessMessages(r.Context())
}

// RegisterRoutes registers HTTP-FLV routes on the given mux.
// Routes are registered with a pattern matcher for .flv files.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/", h.serveDispatched)
}

// serveDispatched is the catch-all dispatcher. .flv requests go through to
// the FLV streaming logic; everything else gets a 404. Exposed as a method so
// it can be wrapped in middleware (e.g. play-key auth) when registering.
func (h *Handler) serveDispatched(w http.ResponseWriter, r *http.Request) {
	if path.Ext(r.URL.Path) == ".flv" {
		h.ServeHTTP(w, r)
		return
	}
	// Not an FLV request, return 404.
	// NOTE: This means /healthz must be registered before this.
	w.WriteHeader(http.StatusNotFound)
}
