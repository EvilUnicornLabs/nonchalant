// If you are AI: This file exposes Prometheus metrics for nonchalant.
// Metrics are computed lazily via a custom collector that reads bus state on scrape.

package metrics

import (
	"net/http"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/vinq1911/nonchalant/internal/core/bus"
)

// Service exposes the /metrics endpoint and registers the collectors.
type Service struct {
	registry *bus.Registry
	relayMgr RelayManager
	promReg  *prometheus.Registry
}

// RelayManager is the minimal surface metrics needs from the relay manager.
type RelayManager interface {
	TaskCount() int
}

// NewService wires Prometheus metrics around the bus registry and relay manager.
// The Prometheus registry is private — clients use the /metrics handler.
func NewService(reg *bus.Registry, relays RelayManager) *Service {
	s := &Service{
		registry: reg,
		relayMgr: relays,
		promReg:  prometheus.NewRegistry(),
	}

	// Standard process + Go runtime metrics.
	s.promReg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
	)

	// Custom collector that walks the registry on each scrape.
	s.promReg.MustRegister(newCollector(reg, relays))
	return s
}

// RegisterRoutes mounts /metrics on the given mux.
func (s *Service) RegisterRoutes(mux *http.ServeMux) {
	mux.Handle("/metrics", promhttp.HandlerFor(s.promReg, promhttp.HandlerOpts{
		ErrorLog:      nil,
		ErrorHandling: promhttp.ContinueOnError,
	}))
}
