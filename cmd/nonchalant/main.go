// If you are AI: This is the main entrypoint for the nonchalant server.
// It handles configuration loading, server startup, and graceful shutdown.

package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"

	"github.com/vinq1911/nonchalant/internal/config"
	"github.com/vinq1911/nonchalant/internal/server"
)

// main is the entrypoint for the nonchalant server.
// It loads configuration, starts the server, and handles graceful shutdown.
func main() {
	// Parse command-line flags
	configPath := flag.String("config", "configs/nonchalant.example.yaml", "Path to configuration file")
	flag.Parse()

	// Load configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Validate configuration
	if err := cfg.Validate(); err != nil {
		log.Fatalf("Invalid config: %v", err)
	}

	// Create root context
	ctx := context.Background()

	// Create server
	srv := server.New(cfg)

	// Create shutdown handler
	shutdownHandler := server.NewShutdownHandler(srv, ctx)

	// Start server in a goroutine
	go func() {
		if err := srv.Start(cfg); err != nil && err != http.ErrServerClosed {
			log.Printf("Server error: %v", err)
			os.Exit(1)
		}
	}()

	// Wait for shutdown signal
	if err := shutdownHandler.Wait(); err != nil {
		log.Printf("Shutdown error: %v", err)
		os.Exit(1)
	}

	log.Println("Server shut down cleanly")
}
