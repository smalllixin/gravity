package worker

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds the configuration for the compression worker
type Config struct {
	// S3 configuration
	S3Bucket      string // S3 bucket name (e.g., "traces-bucket")
	S3Region      string // AWS region (e.g., "us-west-2")
	RawSpansPath  string // Path to raw spans in S3 (e.g., "raw-spans/")
	BlobsPath     string // Path to store compressed blobs (e.g., "blobs/")
	IndexesPath   string // Path to store indexes (e.g., "indexes/")

	// Worker behavior
	PollInterval  time.Duration // How often to poll for new files (default: 30s)
	MaxConcurrent int           // Max concurrent file processing (default: 5)

	// Processing options
	ChunkSeparator string // How to split chunks (default: "\n")
}

// LoadConfig loads configuration from environment variables with defaults
func LoadConfig() (*Config, error) {
	cfg := &Config{
		S3Bucket:       getEnv("S3_BUCKET", ""),
		S3Region:       getEnv("S3_REGION", "us-west-2"),
		RawSpansPath:   getEnv("RAW_SPANS_PATH", "raw-spans/"),
		BlobsPath:      getEnv("BLOBS_PATH", "blobs/"),
		IndexesPath:    getEnv("INDEXES_PATH", "indexes/"),
		PollInterval:   getDurationEnv("POLL_INTERVAL", 30*time.Second),
		MaxConcurrent:  getIntEnv("MAX_CONCURRENT", 5),
		ChunkSeparator: getEnv("CHUNK_SEPARATOR", "\n"),
	}

	// Validate required fields
	if cfg.S3Bucket == "" {
		return nil, fmt.Errorf("S3_BUCKET environment variable is required")
	}

	return cfg, nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getIntEnv(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

func getDurationEnv(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}
