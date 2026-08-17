// If you are AI: This file provides HTTP API service integration.
// The API exposes server state and relay control without blocking media paths.

package api

import (
	"net/http"
	"time"

	"github.com/vinq1911/nonchalant/internal/core/bus"
	"github.com/vinq1911/nonchalant/internal/svc/relay"
)

// Service provides HTTP API functionality.
type Service struct {
	registry  *bus.Registry
	relayMgr  RelayManager
	startTime int64
}

// RelayManager defines the interface for relay management.
// This allows the API to work with relay manager without tight coupling.
type RelayManager interface {
	TaskCount() int
	GetTasks() []relay.TaskInfo
	Restart(app, name string) error
}

// RelayTaskInfo represents information about a relay task for API responses.
type RelayTaskInfo struct {
	App       string `json:"app"`
	Name      string `json:"name"`
	Mode      string `json:"mode"`
	RemoteURL string `json:"remote_url"`
	Running   bool   `json:"running"`
}

// NewService creates a new API service.
func NewService(registry *bus.Registry, relayMgr RelayManager) *Service {
	return &Service{
		registry:  registry,
		relayMgr:  relayMgr,
		startTime: getCurrentTime(),
	}
}

// RegisterRoutes registers API routes on the provided mux.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	// API routes
	mux.HandleFunc("/api/server", s.handleServer)
	mux.HandleFunc("/api/streams", s.handleStreams)
	mux.HandleFunc("/api/relay", s.handleRelay)
	mux.HandleFunc("/api/relay/restart", s.handleRelayRestart)
}

// getCurrentTime returns current Unix timestamp.
// Extracted for testability.
func getCurrentTime() int64 {
	return time.Now().Unix()
}
