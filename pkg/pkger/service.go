// If you are AI: This file glues the packager Manager + Handler into a single Service
// that the top-level server can construct and tear down with one call.

package pkger

import (
	"net/http"
	"time"

	"github.com/vinq1911/nonchalant/pkg/auth"
	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

// Service exposes HLS and DASH packaging via HTTP.
type Service struct {
	mgr      *Manager
	handler  *Handler
	playKeys *auth.KeySet
}

// Options bundles the optional knobs accepted by NewService so we can grow
// the configuration surface without breaking callers.
type Options struct {
	LowLatency bool         // see HLSConfig.LowLatency
	Ladder     []LadderRung // empty = single-rendition stream-copy mode
}

// LadderRung is the pkger-local view of a single ABR rendition.
// Mirrors config.LadderRung — kept here to avoid a config import cycle.
type LadderRung struct {
	Name         string
	Width        int
	Height       int
	VideoBitrate int // kbit/s
	AudioBitrate int // kbit/s
	AudioOnly    bool
}

// NewService creates a Service. httpPort tells the packager where to fetch
// the FLV source from (i.e. our own HTTP-FLV endpoint). playKeys may be nil
// to allow anonymous HLS / DASH playback.
func NewService(reg *bus.Registry, httpPort int, playKeys *auth.KeySet, opts Options) (*Service, error) {
	mgr, err := NewManager(reg, httpPort, 60*time.Second, opts)
	if err != nil {
		return nil, err
	}
	return &Service{
		mgr:      mgr,
		handler:  NewHandler(mgr),
		playKeys: playKeys,
	}, nil
}

// RegisterRoutes mounts /hls/ and /dash/ on the supplied mux.
// When play keys are configured both prefixes are gated by auth.Gate.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	if s.playKeys == nil {
		s.handler.RegisterRoutes(mux)
		return
	}
	mux.Handle("/hls/", auth.Gate(s.playKeys, http.HandlerFunc(s.handler.serveHLS)))
	mux.Handle("/dash/", auth.Gate(s.playKeys, http.HandlerFunc(s.handler.serveDASH)))
}

// Stop tears down all live packagers and removes temp files.
func (s *Service) Stop() { s.mgr.Stop() }
