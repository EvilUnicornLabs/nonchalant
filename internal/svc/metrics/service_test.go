// If you are AI: This file tests the metrics service end-to-end via httptest.

package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

// fakeRelayMgr is a minimal RelayManager for tests.
type fakeRelayMgr struct{ count int }

// TaskCount returns the configured task count.
func (f *fakeRelayMgr) TaskCount() int { return f.count }

// TestMetricsExposedOnEmptyRegistry confirms the endpoint serves Prometheus
// text format and includes our custom metric names even when there are no streams.
func TestMetricsExposedOnEmptyRegistry(t *testing.T) {
	registry := bus.NewRegistry()
	svc := NewService(registry, &fakeRelayMgr{count: 0})
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"nonchalant_streams ",
		"nonchalant_publishers ",
		"nonchalant_relay_tasks ",
		"# HELP go_goroutines",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("metrics body missing %q\nbody: %s", want, body)
		}
	}
}

// TestPerStreamCountersIncrement verifies that publishing through a stream
// shows up as nonchalant_messages_published_total{app,name}.
func TestPerStreamCountersIncrement(t *testing.T) {
	registry := bus.NewRegistry()
	svc := NewService(registry, &fakeRelayMgr{count: 1})
	mux := http.NewServeMux()
	svc.RegisterRoutes(mux)

	stream, _ := registry.GetOrCreate(bus.NewStreamKey("live", "x"))
	stream.AttachPublisher(1)
	for i := 0; i < 3; i++ {
		msg := bus.AcquireMessage()
		msg.Type = bus.MessageTypeAudio
		msg.Payload = bus.AcquirePayload()
		stream.Publish(msg)
	}

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	body := rec.Body.String()

	if !strings.Contains(body, `nonchalant_messages_published_total{app="live",name="x"} 3`) {
		t.Errorf("expected published_total=3 sample, body:\n%s", body)
	}
	if !strings.Contains(body, `nonchalant_relay_tasks 1`) {
		t.Errorf("expected relay_tasks=1, body:\n%s", body)
	}
}
