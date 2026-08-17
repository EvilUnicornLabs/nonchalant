// If you are AI: This file provides HTTP-FLV service integration.
// The service is integrated into the main HTTP server.

package httpflv

import (
	"net/http"

	"github.com/vinq1911/nonchalant/internal/auth"
	"github.com/vinq1911/nonchalant/internal/core/bus"
)

// Service provides HTTP-FLV streaming functionality.
type Service struct {
	handler  *Handler
	playKeys *auth.KeySet
}

// NewService creates a new HTTP-FLV service.
// playKeys may be nil to allow anonymous playback.
func NewService(registry *bus.Registry, playKeys *auth.KeySet) *Service {
	return &Service{
		handler:  NewHandler(registry),
		playKeys: playKeys,
	}
}

// RegisterRoutes registers HTTP-FLV routes on the provided mux.
// When play keys are configured, the catch-all is gated by auth.Gate.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	if s.playKeys == nil {
		s.handler.RegisterRoutes(mux)
		return
	}
	// Re-register the same dispatch logic, but wrapped in the gate.
	mux.Handle("/", auth.Gate(s.playKeys, http.HandlerFunc(s.handler.serveDispatched)))
}
