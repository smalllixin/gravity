package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/youware/gravity/internal/worker"
)

func main() {
	log.SetFlags(log.LstdFlags | log.Lshortfile)
	log.Println("Starting Gravity Compression Worker...")

	// Load configuration
	cfg, err := worker.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	log.Printf("Configuration loaded:")
	log.Printf("  S3 Bucket: %s", cfg.S3Bucket)
	log.Printf("  Region: %s", cfg.S3Region)
	log.Printf("  Raw Spans Path: %s", cfg.RawSpansPath)
	log.Printf("  Blobs Path: %s", cfg.BlobsPath)
	log.Printf("  Indexes Path: %s", cfg.IndexesPath)
	log.Printf("  Poll Interval: %v", cfg.PollInterval)
	log.Printf("  Max Concurrent: %d", cfg.MaxConcurrent)

	// Create worker
	w, err := worker.New(cfg)
	if err != nil {
		log.Fatalf("Failed to create worker: %v", err)
	}

	// Setup graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start worker in background
	go func() {
		if err := w.Start(ctx); err != nil {
			log.Printf("Worker error: %v", err)
			cancel()
		}
	}()

	// Wait for shutdown signal
	sig := <-sigChan
	log.Printf("Received signal %v, shutting down gracefully...", sig)
	cancel()

	log.Println("Shutdown complete")
}
