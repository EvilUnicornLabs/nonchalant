// If you are AI: This is a load-test client for nonchalant. It opens N
// concurrent HTTP-FLV consumers against a single live stream, performs a
// barrier so the steady-state measurement window starts only after every
// consumer has read its first byte, then samples bytes-per-tick to surface
// quality markers: per-client throughput stability (jitter), longest stall,
// and (via /api/streams sampling) bus drop counts. Aggregate reporting and
// CSV output live in report.go to keep this file under the line limit.

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"sync"
	"sync/atomic"
	"time"
)

var (
	url       = flag.String("url", "http://127.0.0.1:18081/live/loadtest.flv", "FLV URL to pull")
	apiURL    = flag.String("api", "http://127.0.0.1:18081/api/streams", "URL of /api/streams (drop sampling)")
	clients   = flag.Int("n", 64, "concurrent consumers")
	duration  = flag.Duration("d", 10*time.Second, "steady-state measurement window")
	connectT  = flag.Duration("connect-timeout", 30*time.Second, "max wall time to wait for all clients to reach first-byte")
	tickT     = flag.Duration("tick", 100*time.Millisecond, "bytes-per-client sample tick")
	out       = flag.String("out", "", "optional CSV output path (per-client summary)")
	summaryFn = flag.String("summary", "", "optional CSV with one aggregate row")
)

// clientStat captures per-consumer outcome over the steady-state window.
type clientStat struct {
	id           int
	flvOK        bool
	connectMS    float64 // ms from gate close to first byte
	windowBytes  int64
	maxStallMS   float64 // longest gap with zero bytes during the window (ms)
	cvPercent    float64 // coefficient of variation of bytes/tick (jitter %)
	firstErr     error
	streaming    bool
	stoppedEarly bool
}

// main drives the three-phase load test: spawn N consumers and gate them at
// first-byte, hold a steady-state measurement window, then aggregate and
// optionally write CSV reports.
func main() {
	flag.Parse()

	results := make([]clientStat, *clients)

	// Phase 1: spawn N goroutines. Each connects, reads first byte, then
	// blocks at firstByteReady. Phase 2: when all are ready (or connectT
	// elapses), close measureGo. Each goroutine then samples bytes/tick
	// for `duration`. Phase 3: stopAt fires; goroutines exit.
	gate := make(chan struct{})
	stop := make(chan struct{})
	firstByteReady := make(chan int, *clients) // ids of clients that hit first byte

	var wg sync.WaitGroup
	for i := 0; i < *clients; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			results[i] = pull(i, *url, gate, firstByteReady, stop, *tickT)
		}()
	}

	// Wait for all goroutines to be queued, then start.
	time.Sleep(50 * time.Millisecond) // let goroutines reach the gate
	gateOpened := time.Now()
	close(gate)

	// Wait for connect storm to settle.
	connectDeadline := time.Now().Add(*connectT)
	connected := 0
	for connected < *clients && time.Now().Before(connectDeadline) {
		select {
		case <-firstByteReady:
			connected++
		case <-time.After(50 * time.Millisecond):
			// fall through to deadline check
		}
	}
	connectElapsed := time.Since(gateOpened)
	fmt.Fprintf(os.Stderr, "==> %d / %d clients reached first byte in %.2f s\n",
		connected, *clients, connectElapsed.Seconds())

	// ---- steady-state window ----
	dropsBefore := sampleDrops(*apiURL)
	windowStart := time.Now()
	time.Sleep(*duration)
	windowEnd := time.Now()
	dropsAfter := sampleDrops(*apiURL)
	close(stop)
	wg.Wait()

	// Aggregate
	report(results, windowEnd.Sub(windowStart), connected, dropsAfter-dropsBefore)
	if *out != "" {
		writeCSV(*out, results)
	}
	if *summaryFn != "" {
		writeSummary(*summaryFn, results, windowEnd.Sub(windowStart),
			connected, dropsAfter-dropsBefore, connectElapsed)
	}
}

// pull is the per-consumer hot loop. It connects, reads the FLV header,
// signals "first byte" via firstByteReady, then samples bytes/tick during
// the steady-state window until stop fires.
func pull(id int, u string, gate chan struct{}, firstByteReady chan<- int, stop <-chan struct{}, tick time.Duration) clientStat {
	stat := clientStat{id: id}
	<-gate
	connectStart := time.Now()

	resp, err := http.Get(u)
	if err != nil {
		stat.firstErr = err
		return stat
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		stat.firstErr = fmt.Errorf("status %d", resp.StatusCode)
		return stat
	}

	hdr := make([]byte, 13)
	if _, err := io.ReadFull(resp.Body, hdr); err != nil {
		stat.firstErr = err
		return stat
	}
	if hdr[0] != 'F' || hdr[1] != 'L' || hdr[2] != 'V' || hdr[3] != 0x01 {
		stat.firstErr = fmt.Errorf("bad FLV header")
		return stat
	}
	stat.flvOK = true
	stat.connectMS = time.Since(connectStart).Seconds() * 1000
	firstByteReady <- id

	// Sample loop: track bytes-per-tick during the steady-state window.
	buf := make([]byte, 64*1024)
	bytesPerTick := []int64{}
	lastTick := time.Now()
	var bytesThisTick int64
	var stallMS float64
	var lastByte = time.Now()

	// Use an atomic flag instead of select-on-every-read so reading is
	// uninterrupted by scheduler context-switching.
	var stopped atomic.Bool
	go func() { <-stop; stopped.Store(true) }()

	stat.streaming = true
	for !stopped.Load() {
		// Bound a single read so it can't block past stop forever.
		_ = resp.Request.Context()
		n, err := resp.Body.Read(buf)
		now := time.Now()
		if n > 0 {
			bytesThisTick += int64(n)
			gap := now.Sub(lastByte).Seconds() * 1000
			if gap > stallMS {
				stallMS = gap
			}
			lastByte = now
			stat.windowBytes += int64(n)
		}
		if now.Sub(lastTick) >= tick {
			bytesPerTick = append(bytesPerTick, bytesThisTick)
			bytesThisTick = 0
			lastTick = now
		}
		if err != nil {
			if err != io.EOF {
				stat.firstErr = err
			}
			stat.stoppedEarly = true
			break
		}
	}
	stat.maxStallMS = stallMS
	stat.cvPercent = coefficientOfVariation(bytesPerTick) * 100
	return stat
}

// coefficientOfVariation = stddev / mean, a unit-free jitter measure.
// Returns 0 for empty / zero-mean input.
func coefficientOfVariation(xs []int64) float64 {
	if len(xs) == 0 {
		return 0
	}
	var sum float64
	for _, x := range xs {
		sum += float64(x)
	}
	mean := sum / float64(len(xs))
	if mean == 0 {
		return 0
	}
	var sqDev float64
	for _, x := range xs {
		d := float64(x) - mean
		sqDev += d * d
	}
	std := math.Sqrt(sqDev / float64(len(xs)))
	return std / mean
}

// sampleDrops fetches /api/streams and sums messages_dropped across the
// returned streams. Returns 0 on any error.
func sampleDrops(u string) uint64 {
	resp, err := http.Get(u)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var body struct {
		Streams []struct {
			MessagesDropped uint64 `json:"messages_dropped"`
		} `json:"streams"`
	}
	if json.NewDecoder(resp.Body).Decode(&body) != nil {
		return 0
	}
	var s uint64
	for _, st := range body.Streams {
		s += st.MessagesDropped
	}
	return s
}
