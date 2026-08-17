// If you are AI: This file contains unit tests for API handlers.
// Tests verify JSON responses and error handling.

package api

import (
	"encoding/json"
	"github.com/vinq1911/nonchalant/internal/svc/relay"
	"github.com/vinq1911/nonchalant/pkg/core/bus"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandleServer(t *testing.T) {
	registry := bus.NewRegistry()
	relayMgr := relay.NewManager(registry)
	service := NewService(registry, relayMgr)

	req := httptest.NewRequest("GET", "/api/server", nil)
	w := httptest.NewRecorder()

	service.handleServer(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response ServerResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if response.Version == "" {
		t.Error("Version should not be empty")
	}
	if response.Uptime < 0 {
		t.Error("Uptime should be non-negative")
	}
	if response.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}
	if len(response.EnabledServices) == 0 {
		t.Error("EnabledServices should not be empty")
	}
}

func TestHandleStreams(t *testing.T) {
	registry := bus.NewRegistry()
	relayMgr := relay.NewManager(registry)
	service := NewService(registry, relayMgr)

	// Test empty streams
	req := httptest.NewRequest("GET", "/api/streams", nil)
	w := httptest.NewRecorder()

	service.handleStreams(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response StreamsResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response.Streams) != 0 {
		t.Errorf("Expected 0 streams, got %d", len(response.Streams))
	}

	// Test with a stream
	key := bus.NewStreamKey("live", "test")
	stream, _ := registry.GetOrCreate(key)
	stream.AttachPublisher(1)

	req2 := httptest.NewRequest("GET", "/api/streams", nil)
	w2 := httptest.NewRecorder()

	service.handleStreams(w2, req2)

	if w2.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w2.Code)
	}

	var response2 StreamsResponse
	if err := json.NewDecoder(w2.Body).Decode(&response2); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if len(response2.Streams) != 1 {
		t.Errorf("Expected 1 stream, got %d", len(response2.Streams))
	}

	if response2.Streams[0].App != "live" || response2.Streams[0].Name != "test" {
		t.Error("Stream info incorrect")
	}

	if !response2.Streams[0].HasPublisher {
		t.Error("Stream should have publisher")
	}
}

func TestHandleRelay(t *testing.T) {
	registry := bus.NewRegistry()
	relayMgr := relay.NewManager(registry)
	service := NewService(registry, relayMgr)

	req := httptest.NewRequest("GET", "/api/relay", nil)
	w := httptest.NewRecorder()

	service.handleRelay(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response RelayResponse
	if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	// Should return empty list if no relays configured
	if response.Tasks == nil {
		t.Error("Tasks should not be nil")
	}
}

func TestHandleRelayRestart(t *testing.T) {
	registry := bus.NewRegistry()
	relayMgr := relay.NewManager(registry)
	service := NewService(registry, relayMgr)

	// Test invalid method
	req := httptest.NewRequest("GET", "/api/relay/restart", nil)
	w := httptest.NewRecorder()

	service.handleRelayRestart(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}

	// Test missing fields
	req2 := httptest.NewRequest("POST", "/api/relay/restart", nil)
	w2 := httptest.NewRecorder()

	service.handleRelayRestart(w2, req2)

	if w2.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w2.Code)
	}
}
