// If you are AI: Benchmarks for the JSON API handlers. Throughput here
// matters for ops dashboards and Prometheus scrapers that hit /api/streams.

package api

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/vinq1911/nonchalant/internal/svc/relay"
	"github.com/vinq1911/nonchalant/pkg/core/bus"
)

// fakeRelayMgr is a minimal RelayManager for benchmarks.
type fakeRelayMgr struct{}

func (fakeRelayMgr) TaskCount() int                 { return 0 }
func (fakeRelayMgr) GetTasks() []relay.TaskInfo     { return nil }
func (fakeRelayMgr) Restart(app, name string) error { return nil }

// BenchmarkHandleStreams measures /api/streams response time as a function of
// the number of streams in the registry. The handler walks the registry on
// each call and serialises one StreamInfo per stream.
func BenchmarkHandleStreams(b *testing.B) {
	for _, n := range []int{0, 1, 10, 100, 1000} {
		b.Run(fmt.Sprintf("streams=%d", n), func(b *testing.B) {
			reg := bus.NewRegistry()
			for i := 0; i < n; i++ {
				stream, _ := reg.GetOrCreate(bus.NewStreamKey("live", fmt.Sprintf("s%d", i)))
				stream.AttachPublisher(uint64(i + 1))
			}
			svc := NewService(reg, fakeRelayMgr{})
			req := httptest.NewRequest(http.MethodGet, "/api/streams", nil)

			b.ResetTimer()
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				rec := httptest.NewRecorder()
				svc.handleStreams(rec, req)
				if rec.Code != http.StatusOK {
					b.Fatalf("status %d", rec.Code)
				}
			}
		})
	}
}

// BenchmarkHandleServer is the cheapest API endpoint — fixed-size response.
// Good baseline for the JSON encode + http.ResponseWriter overhead.
func BenchmarkHandleServer(b *testing.B) {
	reg := bus.NewRegistry()
	svc := NewService(reg, fakeRelayMgr{})
	req := httptest.NewRequest(http.MethodGet, "/api/server", nil)

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		svc.handleServer(rec, req)
		if rec.Code != http.StatusOK {
			b.Fatalf("status %d", rec.Code)
		}
	}
}
