// If you are AI: This file contains unit tests for WebSocket-FLV handler.
// Tests verify WebSocket upgrade and subscriber lifecycle.

package wsflv

import (
	"github.com/vinq1911/nonchalant/pkg/core/bus"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gorilla/websocket"
)

func TestWSFLVHandlerNotFound(t *testing.T) {
	registry := bus.NewRegistry()
	handler := NewHandler(registry)

	req := httptest.NewRequest("GET", "/ws/live/nonexistent", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestWSFLVHandlerNoPublisher(t *testing.T) {
	registry := bus.NewRegistry()
	handler := NewHandler(registry)

	// Create stream without publisher
	key := bus.NewStreamKey("live", "test")
	registry.GetOrCreate(key)

	req := httptest.NewRequest("GET", "/ws/live/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404 (no publisher), got %d", w.Code)
	}
}

func TestWSFLVHandlerBadPath(t *testing.T) {
	registry := bus.NewRegistry()
	handler := NewHandler(registry)

	// Test path without /ws/ prefix
	req := httptest.NewRequest("GET", "/live/test", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestWSFLVHandlerUpgrade(t *testing.T) {
	registry := bus.NewRegistry()
	handler := NewHandler(registry)

	// Create stream with publisher
	key := bus.NewStreamKey("live", "test")
	stream, _ := registry.GetOrCreate(key)
	stream.AttachPublisher(1)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(handler.ServeHTTP))
	defer server.Close()

	// Convert to WebSocket URL
	wsURL := "ws" + server.URL[4:] + "/ws/live/test"

	// Connect WebSocket client
	conn, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect WebSocket: %v", err)
	}
	defer conn.Close()
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusSwitchingProtocols {
		t.Errorf("Expected status 101, got %d", resp.StatusCode)
	}

	// Read first frame (should be FLV header)
	messageType, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if messageType != websocket.BinaryMessage {
		t.Errorf("Expected binary message, got %d", messageType)
	}

	// Check FLV signature
	if len(data) < 9 {
		t.Error("Response too short for FLV header")
	}

	if string(data[:3]) != "FLV" {
		t.Errorf("Response does not start with FLV signature, got: %v", data[:3])
	}

	// Close connection
	conn.Close()
}
