// If you are AI: This file contains unit tests for HTTP-FLV handler.
// Tests verify FLV header generation and subscriber lifecycle.

package httpflv

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

func TestHTTPFLVHandlerNotFound(t *testing.T) {
	registry := bus.NewRegistry()
	handler := NewHandler(registry)

	req := httptest.NewRequest("GET", "/live/nonexistent.flv", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestHTTPFLVHandlerNoPublisher(t *testing.T) {
	registry := bus.NewRegistry()
	handler := NewHandler(registry)

	// Create stream without publisher
	key := bus.NewStreamKey("live", "test")
	registry.GetOrCreate(key)

	req := httptest.NewRequest("GET", "/live/test.flv", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 (no publisher), got %d", w.Code)
	}
}

func TestHTTPFLVHandlerWithPublisher(t *testing.T) {
	registry := bus.NewRegistry()
	handler := NewHandler(registry)

	key := bus.NewStreamKey("live", "test")
	stream, _ := registry.GetOrCreate(key)
	stream.AttachPublisher(1)

	// Real TCP server — Hijack needs a connection that exposes a *net.TCPConn,
	// which httptest.ResponseRecorder doesn't.
	srv := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/live/test.flv")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Content-Type"); got != "video/x-flv" {
		t.Errorf("Expected Content-Type video/x-flv, got %s", got)
	}

	// Read just the FLV header bytes — the connection stays open as a live
	// stream. Closing the body will terminate the handler.
	hdr := make([]byte, 9)
	if _, err := io.ReadFull(resp.Body, hdr); err != nil {
		t.Fatalf("read header: %v", err)
	}
	if !bytes.HasPrefix(hdr, []byte("FLV")) {
		t.Errorf("Response does not start with FLV signature, got: %v", hdr[:3])
	}
}
