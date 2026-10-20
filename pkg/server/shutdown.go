// If you are AI: This file handles graceful shutdown orchestration for the server process.

package server

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"
)

// ShutdownHandler manages graceful shutdown on SIGINT or SIGTERM.
type ShutdownHandler struct {
	server *Server
	ctx    context.Context
	cancel context.CancelFunc
}

// NewShutdownHandler creates a handler that listens for termination signals.
// The provided context is used as the parent for shutdown operations.
func NewShutdownHandler(server *Server, ctx context.Context) *ShutdownHandler {
	shutdownCtx, cancel := context.WithCancel(ctx)
	return &ShutdownHandler{
		server: server,
		ctx:    shutdownCtx,
		cancel: cancel,
	}
}

// Wait blocks until a termination signal is received, then initiates shutdown.
// This method should be called from the main goroutine.
func (h *ShutdownHandler) Wait() error {
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	// Wait for signal
	sig := <-sigChan

	// Cancel context to signal shutdown
	h.cancel()

	// Shutdown server with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := h.server.Shutdown(shutdownCtx); err != nil {
		return err
	}

	_ = sig // Signal received, shutdown complete
	return nil
}

// Context returns the shutdown context that is cancelled when shutdown begins.
func (h *ShutdownHandler) Context() context.Context {
	return h.ctx
}
