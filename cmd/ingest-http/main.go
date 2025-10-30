package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/youware/gravity/internal/ingest/http"
	"github.com/youware/gravity/internal/shared/config"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Create HTTP server
	srv, err := http.NewServer(cfg)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		log.Printf("Starting ingest HTTP server on %s", cfg.HTTP.Address)
		if err := srv.Start(); err != nil {
			errChan <- fmt.Errorf("server error: %w", err)
		}
	}()

	// Wait for interrupt signal or server error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		log.Printf("Server error: %v", err)
	case sig := <-sigChan:
		log.Printf("Received signal: %v", sig)
	}

	// Graceful shutdown
	log.Println("Shutting down server...")
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("Error during shutdown: %v", err)
	}

	log.Println("Server stopped")
}
