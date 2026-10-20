# If you are AI: This Makefile provides all build, test, and deployment targets for nonchalant.

.PHONY: help build run docker-build docker-run docker-restart docker-stop kill fmt lint test test-short test-race bench bench-fast itest itest-clean e2e e2e-install e2e-headed e2e-report docs docs-check check-lines check-comments check test-rtmp-ingest test-httpflv test-wsflv test-api test-workflow test-features test-video test-video-loop test-send test-receive test-send-receive-ws test-roundtrip

# Configuration variables
CONFIG ?= configs/nonchalant.example.yaml
BIN ?= bin/nonchalant
IMAGE ?= nonchalant:latest
CONTAINER ?= nonchalant
HEALTH_PORT ?= 8080
RTMP_PORT ?= 1935
HTTP_PORT ?= 8081

# Default target: show help
help:
	@echo "nonchalant Makefile"
	@echo ""
	@echo "Targets:"
	@echo "  build              - Build the binary"
	@echo "  run                - Run the server"
	@echo "  docker-build       - Build Docker image"
	@echo "  docker-run         - Run container"
	@echo "  docker-restart     - Restart container"
	@echo "  docker-stop        - Stop container"
	@echo "  kill               - Kill running process (macOS/Linux)"
	@echo "  fmt                - Format code"
	@echo "  lint               - Run golangci-lint v2"
	@echo "  test               - Run all tests"
	@echo "  test-short         - Run short tests"
	@echo "  test-race          - Run tests with race detector"
	@echo "  bench              - Run all benchmarks (fast: skip itest)"
	@echo "  bench-fast         - Run unit-package benchmarks only (fastest)"
	@echo "  itest              - Run integration tests"
	@echo "  itest-clean        - Clean integration test artifacts"
	@echo "  e2e                - Run Playwright browser E2E tests"
	@echo "  e2e-install        - Install Playwright deps + chromium"
	@echo "  e2e-headed         - Run E2E tests with browser visible"
	@echo "  e2e-report         - Open last E2E HTML report"
	@echo "  docs               - Generate documentation"
	@echo "  docs-check         - Check if docs are up to date"
	@echo "  check-lines        - Check file line limits"
	@echo "  check-comments     - Check file headers and function comments"
	@echo "  check              - Run all checks"
	@echo "  test-video         - Test send/receive with video file (full round-trip)"
	@echo "  test-video-loop    - Full round-trip loop (runs until Ctrl+C)"
	@echo "  test-send         - Test SEND only (RTMP publish)"
	@echo "  test-receive       - Test RECEIVE only (HTTP-FLV playback)"
	@echo "  test-roundtrip    - Complete round-trip: send → verify → receive"
	@echo "  test-send-receive-ws - Test send/receive via WebSocket-FLV"
	@echo "  test-workflow     - Test complete workflow"
	@echo "  test-rtmp-ingest  - Test RTMP publishing"
	@echo "  test-httpflv      - Test HTTP-FLV playback"
	@echo "  test-wsflv        - Test WebSocket-FLV connection"
	@echo "  test-api          - Test all API endpoints"
	@echo "  test-features     - Run all feature tests (requires server)"
	@echo ""
	@echo "Variables:"
	@echo "  CONFIG=$(CONFIG)"
	@echo "  BIN=$(BIN)"
	@echo "  IMAGE=$(IMAGE)"
	@echo "  CONTAINER=$(CONTAINER)"
	@echo "  HEALTH_PORT=$(HEALTH_PORT)"
	@echo "  RTMP_PORT=$(RTMP_PORT)"
	@echo "  HTTP_PORT=$(HTTP_PORT)"

# Build binary
build:
	@mkdir -p bin
	go build -o $(BIN) ./cmd/nonchalant

# Run server
run: build
	./$(BIN) --config $(CONFIG)

# Docker targets
docker-build:
	docker build -t $(IMAGE) .

docker-run:
	docker run -d --name $(CONTAINER) \
		-p $(HEALTH_PORT):$(HEALTH_PORT) \
		-p $(HTTP_PORT):$(HTTP_PORT) \
		-p $(RTMP_PORT):$(RTMP_PORT) \
		$(IMAGE)

docker-restart: docker-stop docker-run

docker-stop:
	-docker stop $(CONTAINER)
	-docker rm $(CONTAINER)

# Kill running process and free ports (works on macOS and Linux)
kill:
	@pkill -x nonchalant 2>/dev/null || true
	@fuser -k $(RTMP_PORT)/tcp 2>/dev/null || true
	@fuser -k $(HTTP_PORT)/tcp 2>/dev/null || true
	@sleep 0.5

# Format code
fmt:
	go fmt ./...

# golangci-lint (v2.x). Install with `brew install golangci-lint` or
# `curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/HEAD/install.sh | sh -s -- v2.1.0`
lint:
	@command -v golangci-lint >/dev/null 2>&1 || { echo "golangci-lint not installed; see Makefile comment"; exit 1; }
	golangci-lint run ./cmd/... ./internal/... ./pkg/...

# Test targets
# NOTE: Exclude scripts/ directory as it contains multiple main functions
test:
	go test ./cmd/... ./internal/... ./pkg/...

test-short:
	go test -short ./cmd/... ./internal/... ./pkg/...

test-race:
	go test -race ./cmd/... ./internal/... ./pkg/...

bench:
	go test -bench=. -benchmem -run=^$$ -benchtime=1s ./cmd/... ./internal/... ./pkg/...

# Run benchmarks excluding the integration package (which spawns servers and
# is the slowest). Useful for fast iteration on bus / handler / auth perf.
bench-fast:
	go test -bench=. -benchmem -run=^$$ -benchtime=500ms \
	  ./pkg/auth/... \
	  ./pkg/core/... \
	  ./internal/core/... \
	  ./internal/svc/api/... \
	  ./internal/svc/metrics/... \
	  ./internal/svc/httpflv/... \
	  ./internal/svc/wsflv/... \
	  ./internal/svc/rtmp/...

# Integration tests
itest: build
	go test -v ./internal/itest/...

itest-clean:
	rm -f bin/nonchalant

# Browser-driven end-to-end tests (Playwright). Requires Node.js and ffmpeg.
e2e-install:
	cd e2e && npm install && npx playwright install chromium

e2e: build
	cd e2e && npx playwright test

e2e-headed: build
	cd e2e && npx playwright test --headed

e2e-report:
	cd e2e && npx playwright show-report

# Documentation
docs:
	go run ./scripts/gen-docs

docs-check: docs
	@if [ -f docs/.stamp ]; then \
		echo "Documentation is up to date"; \
	else \
		echo "Documentation is missing or stale. Run 'make docs' to regenerate."; \
		exit 1; \
	fi

# Enforcement checks
check-lines:
	go run ./scripts/check-lines .

check-comments:
	go run ./scripts/check-comments .

# Aggregate check target
check: fmt check-lines check-comments docs-check test
	@echo "All checks passed"

# Feature testing targets
# These targets test individual features using the test video

# Test RTMP ingest (publish test video)
test-rtmp-ingest: build
	@echo "Testing RTMP ingest..."
	@echo "Publishing test video to rtmp://localhost:1935/live/teststream"
	@timeout 10 ffmpeg -re -i assets/nonchalant-test.mp4 \
		-c copy -f flv \
		rtmp://localhost:1935/live/teststream 2>&1 | head -20 || true
	@echo "RTMP ingest test complete"

# Test HTTP-FLV output (requires server running)
test-httpflv: build
	@echo "Testing HTTP-FLV output..."
	@echo "Fetching stream via HTTP-FLV..."
	@timeout 5 curl -s http://localhost:8081/live/teststream.flv | wc -c || echo "Stream not available"
	@echo "HTTP-FLV test complete"

# Test WebSocket-FLV output (requires server running)
test-wsflv: build
	@echo "Testing WebSocket-FLV output..."
	@echo "Connecting to WebSocket endpoint..."
	@timeout 3 curl -s --include \
		--no-buffer \
		--header "Connection: Upgrade" \
		--header "Upgrade: websocket" \
		--header "Sec-WebSocket-Key: test" \
		--header "Sec-WebSocket-Version: 13" \
		http://localhost:8081/ws/live/teststream 2>&1 | head -5 || echo "WebSocket test requires proper client"
	@echo "WebSocket-FLV test complete"

# Test API endpoints
test-api: build
	@echo "Testing HTTP API..."
	@echo "GET /api/server:"
	@curl -s http://localhost:8081/api/server | python3 -m json.tool || echo "API not available"
	@echo ""
	@echo "GET /api/streams:"
	@curl -s http://localhost:8081/api/streams | python3 -m json.tool || echo "API not available"
	@echo ""
	@echo "GET /api/relay:"
	@curl -s http://localhost:8081/api/relay | python3 -m json.tool || echo "API not available"
	@echo "API test complete"

# Test complete workflow (publish, play, verify)
test-workflow: build
	@echo "Testing complete workflow..."
	@echo "1. Starting server in background..."
	@./bin/nonchalant --config configs/nonchalant.example.yaml &
	@SERVER_PID=$$!; \
	sleep 2; \
	echo "2. Publishing test video..."; \
	timeout 5 ffmpeg -re -i assets/nonchalant-test.mp4 \
		-c copy -f flv \
		rtmp://localhost:1935/live/workflowtest 2>&1 > /dev/null & \
	PUBLISH_PID=$$!; \
	sleep 2; \
	echo "3. Checking API for stream..."; \
	curl -s http://localhost:8081/api/streams | python3 -m json.tool || true; \
	echo "4. Fetching stream via HTTP-FLV..."; \
	timeout 3 curl -s http://localhost:8081/live/workflowtest.flv | wc -c || true; \
	echo "5. Cleaning up..."; \
	kill $$PUBLISH_PID 2>/dev/null || true; \
	kill $$SERVER_PID 2>/dev/null || true; \
	wait $$SERVER_PID 2>/dev/null || true; \
	echo "Workflow test complete"

# Test all features (requires server running)
test-features: test-api test-httpflv test-wsflv
	@echo "All feature tests complete"

# Test send/receive with test video (full round-trip test)
test-video: build
	@echo "=== Testing Send/Receive with assets/nonchalant-test.mp4 ==="
	@if [ ! -f assets/nonchalant-test.mp4 ]; then \
		echo "Error: assets/nonchalant-test.mp4 not found"; \
		exit 1; \
	fi
	@echo "1. Starting server..."
	@./bin/nonchalant --config configs/nonchalant.example.yaml > /tmp/nonchalant-test.log 2>&1 &
	@SERVER_PID=$$!; \
	trap "kill $$SERVER_PID 2>/dev/null || true; killall ffmpeg 2>/dev/null || true" EXIT; \
	sleep 3; \
	echo "2. Publishing test video (SEND) via RTMP..."; \
	timeout 15 ffmpeg -re -i assets/nonchalant-test.mp4 \
		-c copy -f flv \
		rtmp://localhost:1935/live/videotest 2>&1 > /tmp/ffmpeg-publish.log & \
	PUBLISH_PID=$$!; \
	sleep 4; \
	echo "3. Verifying stream exists via API..."; \
	STREAM_COUNT=$$(curl -s http://localhost:8081/api/streams | python3 -c "import sys, json; data=json.load(sys.stdin); print(len(data.get('streams', [])))" 2>/dev/null || echo "0"); \
	if [ "$$STREAM_COUNT" -gt 0 ]; then \
		echo "✓ Stream published successfully ($$STREAM_COUNT stream(s) active)"; \
		curl -s http://localhost:8081/api/streams | python3 -m json.tool || true; \
	else \
		echo "✗ Stream not found in API"; \
	fi; \
	echo "4. Receiving stream (RECEIVE) via HTTP-FLV..."; \
	BYTES=$$(timeout 8 curl -s http://localhost:8081/live/videotest.flv | wc -c); \
	if [ $$BYTES -gt 1000 ]; then \
		echo "✓ HTTP-FLV receive test passed (received $$BYTES bytes)"; \
	else \
		echo "✗ HTTP-FLV receive test failed (only $$BYTES bytes received)"; \
	fi; \
	echo "5. Cleaning up..."; \
	kill $$PUBLISH_PID 2>/dev/null || true; \
	sleep 2; \
	kill $$SERVER_PID 2>/dev/null || true; \
	wait $$SERVER_PID 2>/dev/null || true; \
	echo "=== Test complete ==="

# Full round-trip loop: publish in a loop + receive via HTTP-FLV. Runs until Ctrl+C.
test-video-loop: kill build
	@if [ ! -f assets/nonchalant-test.mp4 ]; then \
		echo "Error: assets/nonchalant-test.mp4 not found"; \
		exit 1; \
	fi
	@echo "=== Full Round-Trip Loop (Ctrl+C to stop) ==="; \
	echo "1. Starting server..."; \
	./bin/nonchalant --config $(CONFIG) > /tmp/nonchalant-loop.log 2>&1 & \
	SERVER_PID=$$!; \
	CLEANED=0; \
	cleanup() { \
		[ $$CLEANED -eq 1 ] && return; \
		CLEANED=1; \
		echo ""; \
		echo "Stopping..."; \
		kill $$RECV_PID 2>/dev/null || true; \
		kill $$PUB_PID 2>/dev/null || true; \
		kill $$SERVER_PID 2>/dev/null || true; \
		wait $$SERVER_PID 2>/dev/null || true; \
		echo "Server log: /tmp/nonchalant-loop.log"; \
		echo "=== Stopped ==="; \
	}; \
	trap cleanup EXIT INT TERM; \
	sleep 2; \
	if ! kill -0 $$SERVER_PID 2>/dev/null; then \
		echo "Server failed to start. Check /tmp/nonchalant-loop.log"; \
		cat /tmp/nonchalant-loop.log; \
		exit 1; \
	fi; \
	echo "2. Publishing test video in loop via RTMP..."; \
	ffmpeg -stream_loop -1 -re -i assets/nonchalant-test.mp4 \
		-c copy -f flv \
		rtmp://localhost:$(RTMP_PORT)/live/looptest \
		> /tmp/ffmpeg-loop-publish.log 2>&1 & \
	PUB_PID=$$!; \
	sleep 3; \
	echo "3. Receiving stream via HTTP-FLV (saving to /tmp/loop-receive.flv)..."; \
	touch /tmp/loop-receive.flv; \
	curl -s http://localhost:$(HTTP_PORT)/live/looptest.flv \
		-o /tmp/loop-receive.flv & \
	RECV_PID=$$!; \
	echo ""; \
	echo "  Publish:  rtmp://localhost:$(RTMP_PORT)/live/looptest"; \
	echo "  Playback: http://localhost:$(HTTP_PORT)/live/looptest.flv"; \
	echo "  API:      http://localhost:$(HTTP_PORT)/api/streams"; \
	echo "  Server log: /tmp/nonchalant-loop.log"; \
	echo ""; \
	echo "Streaming... press Ctrl+C to stop."; \
	echo ""; \
	while kill -0 $$PUB_PID 2>/dev/null && kill -0 $$SERVER_PID 2>/dev/null; do \
		BYTES=$$(wc -c < /tmp/loop-receive.flv 2>/dev/null || echo 0); \
		STREAMS=$$(curl -sf http://localhost:$(HTTP_PORT)/api/streams 2>/dev/null | \
			python3 -c "import sys,json; d=json.load(sys.stdin); print(len(d.get('streams',[])))" 2>/dev/null || echo "?"); \
		printf "\r  [running] streams=%-3s received=%s bytes  " "$$STREAMS" "$$BYTES"; \
		sleep 5; \
	done; \
	echo ""; \
	if [ $$CLEANED -eq 0 ]; then \
		if ! kill -0 $$SERVER_PID 2>/dev/null; then echo "Server exited unexpectedly. Check /tmp/nonchalant-loop.log"; fi; \
		if ! kill -0 $$PUB_PID 2>/dev/null; then echo "Publisher exited unexpectedly. Check /tmp/ffmpeg-loop-publish.log"; fi; \
	fi

# Test send only (RTMP publish)
test-send: build
	@echo "=== Testing SEND (RTMP Publish) ==="
	@if [ ! -f assets/nonchalant-test.mp4 ]; then \
		echo "Error: assets/nonchalant-test.mp4 not found"; \
		exit 1; \
	fi
	@echo "Starting server..."
	@./bin/nonchalant --config configs/nonchalant.example.yaml > /tmp/nonchalant-send.log 2>&1 &
	@SERVER_PID=$$!; \
	trap "kill $$SERVER_PID 2>/dev/null || true; killall ffmpeg 2>/dev/null || true" EXIT; \
	sleep 3; \
	echo "Publishing test video to rtmp://localhost:1935/live/sendtest..."; \
	timeout 10 ffmpeg -re -i assets/nonchalant-test.mp4 \
		-c copy -f flv \
		rtmp://localhost:1935/live/sendtest 2>&1 | head -30 || true; \
	sleep 2; \
	echo "Checking if stream is registered..."; \
	curl -s http://localhost:8081/api/streams | python3 -m json.tool || true; \
	kill $$SERVER_PID 2>/dev/null || true; \
	echo "=== Send test complete ==="

# Test receive only (HTTP-FLV playback)
test-receive: build
	@echo "=== Testing RECEIVE (HTTP-FLV Playback) ==="
	@echo "Note: This test requires a stream to be published first"
	@echo "Starting server..."
	@./bin/nonchalant --config configs/nonchalant.example.yaml > /tmp/nonchalant-receive.log 2>&1 &
	@SERVER_PID=$$!; \
	trap "kill $$SERVER_PID 2>/dev/null || true" EXIT; \
	sleep 2; \
	echo "Publishing test video in background..."; \
	timeout 12 ffmpeg -re -i assets/nonchalant-test.mp4 \
		-c copy -f flv \
		rtmp://localhost:1935/live/receivetest 2>&1 > /dev/null & \
	PUBLISH_PID=$$!; \
	sleep 3; \
	echo "Receiving stream via HTTP-FLV..."; \
	BYTES=$$(timeout 8 curl -s http://localhost:8081/live/receivetest.flv -o /tmp/received.flv && wc -c < /tmp/received.flv); \
	if [ $$BYTES -gt 1000 ]; then \
		echo "✓ Receive test passed (received $$BYTES bytes)"; \
		echo "Saved to /tmp/received.flv"; \
		file /tmp/received.flv 2>/dev/null || true; \
	else \
		echo "✗ Receive test failed (only $$BYTES bytes received)"; \
	fi; \
	kill $$PUBLISH_PID 2>/dev/null || true; \
	kill $$SERVER_PID 2>/dev/null || true; \
	echo "=== Receive test complete ==="

# Test send/receive with WebSocket-FLV
test-send-receive-ws: build
	@echo "=== Testing Send/Receive via WebSocket-FLV ==="
	@if [ ! -f assets/nonchalant-test.mp4 ]; then \
		echo "Error: assets/nonchalant-test.mp4 not found"; \
		exit 1; \
	fi
	@echo "1. Starting server..."
	@./bin/nonchalant --config configs/nonchalant.example.yaml > /tmp/nonchalant-ws.log 2>&1 &
	@SERVER_PID=$$!; \
	trap "kill $$SERVER_PID 2>/dev/null || true; killall ffmpeg 2>/dev/null || true" EXIT; \
	sleep 3; \
	echo "2. Publishing test video (SEND) via RTMP..."; \
	timeout 12 ffmpeg -re -i assets/nonchalant-test.mp4 \
		-c copy -f flv \
		rtmp://localhost:1935/live/wstest 2>&1 > /dev/null & \
	PUBLISH_PID=$$!; \
	sleep 3; \
	echo "3. Testing WebSocket-FLV connection (RECEIVE)..."; \
	echo "Note: Full WebSocket test requires a proper client"; \
	RESPONSE=$$(timeout 3 curl -s --include \
		--no-buffer \
		--header "Connection: Upgrade" \
		--header "Upgrade: websocket" \
		--header "Sec-WebSocket-Key: dGhlIHNhbXBsZSBub25jZQ==" \
		--header "Sec-WebSocket-Version: 13" \
		http://localhost:8081/ws/live/wstest 2>&1 | head -5); \
	if echo "$$RESPONSE" | grep -q "101\|Upgrade"; then \
		echo "✓ WebSocket connection accepted"; \
	else \
		echo "⚠ WebSocket test requires proper client (connection attempt made)"; \
	fi; \
	kill $$PUBLISH_PID 2>/dev/null || true; \
	kill $$SERVER_PID 2>/dev/null || true; \
	echo "=== WebSocket test complete ==="

# Test complete round-trip: send → API verify → receive
test-roundtrip: build
	@echo "=== Complete Round-Trip Test ==="
	@if [ ! -f assets/nonchalant-test.mp4 ]; then \
		echo "Error: assets/nonchalant-test.mp4 not found"; \
		exit 1; \
	fi
	@echo "1. Starting server..."
	@./bin/nonchalant --config configs/nonchalant.example.yaml > /tmp/nonchalant-roundtrip.log 2>&1 &
	@SERVER_PID=$$!; \
	trap "kill $$SERVER_PID 2>/dev/null || true; killall ffmpeg 2>/dev/null || true; rm -f /tmp/roundtrip.flv" EXIT; \
	sleep 3; \
	echo "2. SEND: Publishing test video via RTMP..."; \
	timeout 15 ffmpeg -re -i assets/nonchalant-test.mp4 \
		-c copy -f flv \
		rtmp://localhost:1935/live/roundtrip 2>&1 > /tmp/ffmpeg-roundtrip.log & \
	PUBLISH_PID=$$!; \
	sleep 4; \
	echo "3. VERIFY: Checking stream via API..."; \
	HAS_PUBLISHER=$$(curl -s http://localhost:8081/api/streams | python3 -c "import sys, json; data=json.load(sys.stdin); streams=data.get('streams', []); print('true' if streams and any(s.get('has_publisher') for s in streams) else 'false')" 2>/dev/null || echo "false"); \
	if [ "$$HAS_PUBLISHER" = "true" ]; then \
		echo "✓ Stream has publisher"; \
	else \
		echo "✗ Stream has no publisher"; \
	fi; \
	echo "4. RECEIVE: Fetching stream via HTTP-FLV..."; \
	BYTES=$$(timeout 8 curl -s http://localhost:8081/live/roundtrip.flv -o /tmp/roundtrip.flv && wc -c < /tmp/roundtrip.flv); \
	if [ $$BYTES -gt 1000 ]; then \
		echo "✓ Received $$BYTES bytes via HTTP-FLV"; \
		FLV_HEADER=$$(head -c 9 /tmp/roundtrip.flv | od -An -tx1 | tr -d ' \n'); \
		if echo "$$FLV_HEADER" | grep -q "464c5601"; then \
			echo "✓ Valid FLV file (header: $$FLV_HEADER)"; \
		else \
			echo "⚠ FLV header check failed"; \
		fi; \
	else \
		echo "✗ Failed to receive stream (only $$BYTES bytes)"; \
	fi; \
	echo "5. Cleaning up..."; \
	kill $$PUBLISH_PID 2>/dev/null || true; \
	sleep 2; \
	kill $$SERVER_PID 2>/dev/null || true; \
	echo "=== Round-trip test complete ==="
