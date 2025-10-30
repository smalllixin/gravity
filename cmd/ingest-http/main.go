package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lmittmann/tint"

	"github.com/youware/gravity/internal/ingest/http"
	"github.com/youware/gravity/internal/shared/config"
)

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	handler := tint.NewHandler(os.Stdout, &tint.Options{
		Level:      slog.LevelInfo,
		TimeFormat: time.RFC3339,
		AddSource:  true,
	})
	logger := slog.New(handler)
	slog.SetDefault(logger)

	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		logger.Error("failed to load config", "error", err)
		os.Exit(1)
	}

	// Create HTTP server
	srv, err := http.NewServer(cfg)
	if err != nil {
		logger.Error("failed to create server", "error", err)
		os.Exit(1)
	}

	// Start server in a goroutine
	errChan := make(chan error, 1)
	go func() {
		logger.Info("starting ingest HTTP server", "address", cfg.HTTP.Address)
		if err := srv.Start(); err != nil {
			errChan <- fmt.Errorf("server error: %w", err)
		}
	}()

	// Wait for interrupt signal or server error
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt, syscall.SIGTERM)

	select {
	case err := <-errChan:
		logger.Error("server error", "error", err)
	case sig := <-sigChan:
		logger.Info("received signal", "signal", sig)
	}

	// Graceful shutdown
	logger.Info("shutting down server")
	shutdownCtx, shutdownCancel := context.WithTimeout(ctx, 30*time.Second)
	defer shutdownCancel()

	if err := srv.Shutdown(shutdownCtx); err != nil {
		logger.Error("error during shutdown", "error", err)
	}

	logger.Info("server stopped")
}
