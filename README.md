<div align="center">
  <img src="assets/nonchalant-logo.png" alt="nonchalant logo" width="400">
</div>

# nonchalant

A high-performance, modular media server written in Go.

## Status

**Phases 1–8 complete + ops hardening + perf hot path** — RTMP ingest with
publish auth, HTTP-FLV, WebSocket-FLV, RTMP relay, HTTP API, native HLS / DASH
endpoints, Prometheus metrics, lint, CI, and a documented performance harness.

This server currently provides:

- Clean startup and graceful shutdown (signal-aware)
- Health endpoint (`/healthz`), Prometheus `/metrics`, `/debug/pprof/`
- YAML configuration with strict validation
- **RTMP ingest** with optional pre-shared-key publish authentication
- **HTTP-FLV output** — `GET /{app}/{name}.flv` (HTTP/1.1 hijack, one syscall per tag)
- **WebSocket-FLV output** — `ws://host/ws/{app}/{name}`
- **HLS** — `GET /hls/{app}/{name}/index.m3u8` (native, ffmpeg-backed, ABR ladder)
- **DASH** — `GET /dash/{app}/{name}.mpd` (native)
- **RTMP relay** — pull remote streams or push local streams (ffmpeg supervised)
- **HTTP API** — `/api/server`, `/api/streams` (with drop counts), `/api/relay`
- **FFmpeg integration** — optional cgo transcoding (build with `-tags ffmpeg`)
- Lock-free single-producer / multi-cursor shared-log bus
- Per-stream wrap-around arena allocator — zero allocations per RTMP frame
- Wake-on-publish — idle server sits near 0% CPU instead of polling
- Three test layers: unit, integration (real ffmpeg), browser E2E (Playwright)
- `golangci-lint v2` config, GitHub Actions CI

## Performance

nonchalant is built to fan one RTMP ingest out to thousands of HTTP-FLV
viewers on a single process, with predictable latency and a near-idle CPU
when nothing is happening.

The numbers below come from `scripts/perf/run.sh` on an **Apple M5 laptop**
(macOS, default kernel limits) using a real ~2.8 Mbps H.264 + AAC stream.
The full report — methodology, pprof tables, and per-client jitter
distributions — is in [reports/PERF-WOP.pdf](reports/PERF-WOP.pdf).

### One publisher, N concurrent HTTP-FLV viewers

| Viewers | Aggregate throughput | Per-viewer rate | Stall p95 | Bus drops |
| ------: | -------------------: | --------------: | --------: | --------: |
|       1 |             0.3 MB/s |        349 KB/s |     94 ms |         0 |
|      64 |              22 MB/s |        345 KB/s |    151 ms |         0 |
|     256 |              88 MB/s |        345 KB/s |    188 ms |         0 |
|    1024 |             358 MB/s |        349 KB/s |    168 ms |         0 |
|    4096 |              33 MB/s |          8 KB/s |     33 s  |         0 |

Up to ~1024 viewers per process the server delivers full bitrate to every
client with sub-200 ms p95 stalls and zero bus drops. Beyond that the laptop
hits a per-socket I/O wall (the bus stays clean — drops are still zero — but
the kernel can no longer drain that many TCP send buffers fast enough). On
real server hardware and Linux the ceiling moves up materially; the bus and
fan-out path themselves microbenchmark past 250 K subscribers.

### Idle behaviour

With a publisher connected and no viewers, the server sits at **~5 % CPU**
on an M5. With no publisher and no viewers, it is essentially asleep —
subscribers park on a channel closed by the publisher rather than polling.

### What makes it fast

- **Lock-free shared-log bus.** A single-producer, multi-cursor ring means
  `Publish` is one atomic add + one atomic store regardless of how many
  viewers are attached. Subscriber count does not appear in the publisher's
  hot path. (`pkg/core/bus/sharedlog.go`)
- **Zero allocations per RTMP frame.** A per-stream wrap-around arena
  serves payload buffers; `flv.AppendTag` writes directly into them. No
  `sync.Pool`, no GC pressure on the ingest path. (`pkg/core/bus/arena.go`)
- **Wake-on-publish.** Idle viewers block on an
  `atomic.Pointer[chan struct{}]` that the publisher closes when a frame
  lands. No timers, no spinning, no background scheduler load.
- **HTTP/1.1 hijack on the FLV path.** Once the headers are sent, the FLV
  handler takes ownership of the raw TCP socket and writes one FLV tag per
  `syscall.write` — no chunked-transfer framing, no `bufio` double flush.
  This was the single biggest win for high-fan-out CPU. (`internal/svc/httpflv/handler.go`)
- **Per-subscriber backpressure with bounded buffers.** Slow viewers drop
  oldest frames instead of blocking the publisher; the drop counter is
  exposed on `/api/streams` and `/metrics`.

### Reproducing the numbers

The harness is fully scripted and the raw outputs live under `reports/data/`:

```bash
scripts/perf/idle.sh                    # idle CPU sample
scripts/perf/run.sh                     # subscriber sweep (n=1..4096)
scripts/perf/profile.sh prof            # 30 s pprof at 1024 viewers
python scripts/perf/build_report.py     # regenerate reports/PERF-WOP.pdf
```

For ad-hoc load tests against a running server, the standalone
`cmd/loadtest` binary takes `-url`, `-api`, `-n`, and `-d` flags and emits a
CSV summary suitable for diffing across runs.

## Quick Start

### Build and Run

**Default build (without FFmpeg):**
```bash
make build
make run
```

**Build with FFmpeg support:**
```bash
go build -tags ffmpeg -o bin/nonchalant ./cmd/nonchalant
```
Requires FFmpeg development libraries installed.

### Run Tests

```bash
make test          # unit tests (Go)
make test-race     # unit tests with the race detector
make itest         # integration tests (real binary + ffmpeg CLI)
make e2e-install   # one-time: install Playwright + Chromium for browser E2E
make e2e           # browser E2E tests (real flv.js playback in Chromium)
```

See [docs/TESTING.md](docs/TESTING.md) and [e2e/README.md](e2e/README.md) for
details on each layer.

### Run Checks

```bash
make check
```

### Docker

```bash
make docker-build
make docker-run
```

## Configuration

Copy `configs/nonchalant.example.yaml` and modify as needed:

```yaml
server:
  health_port: 8080  # Port for /healthz endpoint
  http_port: 8081    # Port for HTTP-FLV, WS-FLV, HLS, DASH, API, /metrics
  rtmp_port: 1935    # Port for RTMP ingest

auth:                # Optional. Omit for anonymous publishing.
  publish_keys:
    - changeme       # Publishers must use rtmp://host/live/foo?key=changeme

relays:              # Optional: RTMP relay tasks
  - app: live
    name: mystream
    mode: pull       # or "push"
    remote_url: rtmp://remote-server:1935/live/mystream
    reconnect: true
```

See [docs/CONFIG.md](docs/CONFIG.md) for the full schema.

## Usage

### Publishing a Stream

Publish a stream via RTMP:

```bash
ffmpeg -re -i input.mp4 -c copy -f flv rtmp://localhost:1935/live/mystream
```

If `auth.publish_keys` is configured, append the key:

```bash
ffmpeg -re -i input.mp4 -c copy -f flv \
  'rtmp://localhost:1935/live/mystream?key=changeme'
```

### Playing a Stream

Play a stream via HTTP-FLV:

```bash
ffplay http://localhost:8081/live/mystream.flv
```

Or use any FLV-compatible player with the URL: `http://localhost:8081/{app}/{name}.flv`

Play a stream via WebSocket-FLV (browser-compatible):

```javascript
const ws = new WebSocket('ws://localhost:8081/ws/live/mystream');
ws.binaryType = 'arraybuffer';
ws.onmessage = (event) => {
  // First message contains FLV header
  // Subsequent messages contain FLV tags
  const flvData = new Uint8Array(event.data);
  // Process FLV data with your player
};
```

Or use any WebSocket-FLV compatible player with the URL: `ws://host:port/ws/{app}/{name}`

### HLS / DASH

nonchalant ships native HLS and DASH endpoints. The first request lazily spawns
an `ffmpeg` subprocess that pulls the server's own HTTP-FLV stream and serves
segments from a temp directory. Idle packagers are GC'd after 60 s.

```bash
# HLS — single rendition (stream-copy)
ffplay http://localhost:8081/hls/live/mystream.m3u8

# DASH
http://localhost:8081/dash/live/mystream.mpd
```

**ABR (multi-bitrate)** kicks in when `hls.ladder` is configured with one
entry per rendition. The `.m3u8` becomes a master playlist listing each rung,
and per-rendition files live under `/hls/{app}/{name}/{rung}/`:

```yaml
hls:
  ladder:
    - {name: 720p, width: 1280, height: 720, video_bitrate: 2500}
    - {name: 480p, width: 854,  height: 480, video_bitrate: 1100}
    - {name: 240p, width: 426,  height: 240, video_bitrate: 400}
```

`ffmpeg` must be on the server's PATH. If it's missing, the endpoints return
503. See [docs/OPERATIONS.md](docs/OPERATIONS.md) for details.

### API Endpoints

Query server state via HTTP API:

```bash
# Server information
curl http://localhost:8081/api/server

# Active streams
curl http://localhost:8081/api/streams

# Relay tasks
curl http://localhost:8081/api/relay

# Restart a relay task
curl -X POST http://localhost:8081/api/relay/restart \
  -H "Content-Type: application/json" \
  -d '{"app":"live","name":"mystream"}'

# Prometheus metrics
curl http://localhost:8081/metrics
```

## Documentation

- [Architecture](docs/ARCHITECTURE.md) - System design and structure
- [Configuration](docs/CONFIG.md) - Configuration schema (incl. auth + relays)
- [Operations](docs/OPERATIONS.md) - Endpoints, metrics, auth, HLS/DASH ops
- [Usage Guide](docs/USAGE.md) - Step-by-step feature usage
- [Testing](docs/TESTING.md) - Unit, integration, and browser E2E layers
- [Browser E2E](e2e/README.md) - Playwright suite for WS-FLV / HTTP-FLV / API

## License

MIT License with Attribution Requirement

This project is licensed under the MIT License with an additional attribution
requirement. See [LICENSE](LICENSE) for full details.

**Important:** Any use, modification, or derivative work must prominently
acknowledge the original nonchalant project. This acknowledgment must appear
in all documentation, source code headers, and published materials.
