// If you are AI: This file provides WebSocket-FLV service integration.
// The service is integrated into the main HTTP server.

package wsflv

import (
	"net/http"

	"github.com/vinq1911/nonchalant/pkg/auth"
	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

// Service provides WebSocket-FLV streaming functionality.
type Service struct {
	handler  *Handler
	playKeys *auth.KeySet
}

// NewService creates a new WebSocket-FLV service.
// playKeys may be nil to allow anonymous playback.
func NewService(registry *bus.Registry, playKeys *auth.KeySet) *Service {
	return &Service{
		handler:  NewHandler(registry),
		playKeys: playKeys,
	}
}

// RegisterRoutes registers WebSocket-FLV routes on the provided mux.
// When play keys are configured, the /ws/ prefix is gated by auth.Gate.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	if s.playKeys == nil {
		s.handler.RegisterRoutes(mux)
		return
	}
	mux.Handle("/ws/", auth.Gate(s.playKeys, http.HandlerFunc(s.handler.ServeHTTP)))
}
