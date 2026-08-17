// If you are AI: This file holds the aggregate report / CSV writers for the
// load-test client, split out of main.go to stay under the 300-line limit.

package main

import (
	"fmt"
	"io"
	"os"
	"sort"
	"time"
)

// report prints a human-readable summary of the steady-state window to
// stdout: throughput, per-client averages, stall percentiles, jitter, and
// bus drops observed during the window.
func report(results []clientStat, window time.Duration, connected int, drops uint64) {
	var ok, errs int
	var totalBytes int64
	stalls := []float64{}
	cvs := []float64{}
	for _, r := range results {
		if !r.flvOK {
			continue
		}
		if r.firstErr != nil && r.firstErr != io.EOF {
			errs++
		}
		ok++
		totalBytes += r.windowBytes
		stalls = append(stalls, r.maxStallMS)
		cvs = append(cvs, r.cvPercent)
	}
	sort.Float64s(stalls)
	sort.Float64s(cvs)
	pct := func(s []float64, p float64) float64 {
		if len(s) == 0 {
			return 0
		}
		i := int(float64(len(s)-1) * p)
		return s[i]
	}
	mean := func(s []float64) float64 {
		if len(s) == 0 {
			return 0
		}
		var sum float64
		for _, v := range s {
			sum += v
		}
		return sum / float64(len(s))
	}
	fmt.Printf("\n=== nonchalant load test ===\n")
	fmt.Printf("clients spawned       : %d\n", len(results))
	fmt.Printf("clients first-byte    : %d\n", connected)
	fmt.Printf("FLV-validated         : %d\n", ok)
	fmt.Printf("read errors           : %d\n", errs)
	fmt.Printf("steady-state window   : %.2f s\n", window.Seconds())
	fmt.Printf("steady-state bytes    : %.2f GB\n", float64(totalBytes)/1e9)
	fmt.Printf("steady-state aggregate: %.1f MB/s\n",
		float64(totalBytes)/1e6/window.Seconds())
	if ok > 0 {
		fmt.Printf("per-client avg        : %.1f KB/s (%.2f MB total)\n",
			float64(totalBytes)/float64(ok)/1e3/window.Seconds(),
			float64(totalBytes)/float64(ok)/1e6)
	}
	fmt.Printf("max-stall ms (median) : %.0f\n", pct(stalls, 0.50))
	fmt.Printf("max-stall ms (p95)    : %.0f\n", pct(stalls, 0.95))
	fmt.Printf("max-stall ms (worst)  : %.0f\n", pct(stalls, 1.0))
	fmt.Printf("jitter CV%% (mean)    : %.1f\n", mean(cvs))
	fmt.Printf("jitter CV%% (p95)     : %.1f\n", pct(cvs, 0.95))
	fmt.Printf("bus drops (window)    : %d msgs\n", drops)
}

// writeCSV writes one row per client with its raw measurements. Silently
// returns on any file error since CSV output is optional tooling, not a
// pass/fail signal for the load test itself.
func writeCSV(path string, results []clientStat) {
	f, err := os.Create(path)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintln(f, "client,flv_ok,connect_ms,window_bytes,max_stall_ms,cv_percent,err")
	for _, r := range results {
		errs := ""
		if r.firstErr != nil {
			errs = r.firstErr.Error()
		}
		fmt.Fprintf(f, "%d,%v,%.1f,%d,%.0f,%.2f,%q\n",
			r.id, r.flvOK, r.connectMS, r.windowBytes, r.maxStallMS, r.cvPercent, errs)
	}
}

// writeSummary appends one aggregate row to a shared CSV, writing the
// header first if the file is new. Intended for diffing runs across
// `scripts/perf` sweeps via the LOADTEST_LABEL env var.
func writeSummary(path string, results []clientStat, window time.Duration, connected int, drops uint64, connectElapsed time.Duration) {
	var ok int
	var totalBytes int64
	stalls := []float64{}
	cvs := []float64{}
	for _, r := range results {
		if !r.flvOK {
			continue
		}
		ok++
		totalBytes += r.windowBytes
		stalls = append(stalls, r.maxStallMS)
		cvs = append(cvs, r.cvPercent)
	}
	sort.Float64s(stalls)
	sort.Float64s(cvs)
	pct := func(s []float64, p float64) float64 {
		if len(s) == 0 {
			return 0
		}
		return s[int(float64(len(s)-1)*p)]
	}
	mean := func(s []float64) float64 {
		if len(s) == 0 {
			return 0
		}
		var sum float64
		for _, v := range s {
			sum += v
		}
		return sum / float64(len(s))
	}

	// Append-or-create with header.
	exists := false
	if fi, _ := os.Stat(path); fi != nil {
		exists = true
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	if !exists {
		fmt.Fprintln(f,
			"label,n,connected,flv_ok,connect_storm_s,window_s,window_bytes,"+
				"agg_mbs,per_client_kbs,stall_p50_ms,stall_p95_ms,stall_max_ms,"+
				"cv_mean_pct,cv_p95_pct,bus_drops")
	}
	label := os.Getenv("LOADTEST_LABEL")
	aggMB := float64(totalBytes) / 1e6 / window.Seconds()
	perKB := 0.0
	if ok > 0 {
		perKB = float64(totalBytes) / float64(ok) / 1e3 / window.Seconds()
	}
	fmt.Fprintf(f, "%s,%d,%d,%d,%.2f,%.2f,%d,%.1f,%.1f,%.0f,%.0f,%.0f,%.1f,%.1f,%d\n",
		label, len(results), connected, ok, connectElapsed.Seconds(), window.Seconds(),
		totalBytes, aggMB, perKB,
		pct(stalls, 0.5), pct(stalls, 0.95), pct(stalls, 1.0),
		mean(cvs), pct(cvs, 0.95), drops)
}
