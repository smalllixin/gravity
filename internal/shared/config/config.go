package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds all application configuration
type Config struct {
	HTTP HTTPConfig
}

// HTTPConfig holds HTTP server configuration
type HTTPConfig struct {
	Address         string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	ShutdownTimeout time.Duration
}

// Load reads configuration from environment variables with sensible defaults
func Load() (*Config, error) {
	cfg := &Config{
		HTTP: HTTPConfig{
			Address:         getEnv("HTTP_ADDRESS", ":8080"),
			ReadTimeout:     getDuration("HTTP_READ_TIMEOUT", 10*time.Second),
			WriteTimeout:    getDuration("HTTP_WRITE_TIMEOUT", 10*time.Second),
			ShutdownTimeout: getDuration("HTTP_SHUTDOWN_TIMEOUT", 30*time.Second),
		},
	}

	return cfg, nil
}

// getEnv gets an environment variable or returns a default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// getEnvInt gets an integer environment variable or returns a default value
func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if intVal, err := strconv.Atoi(value); err == nil {
			return intVal
		}
	}
	return defaultValue
}

// getDuration gets a duration environment variable or returns a default value
func getDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if duration, err := time.ParseDuration(value); err == nil {
			return duration
		}
	}
	return defaultValue
}

// getEnvSlice gets a comma-separated environment variable as a slice
func getEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		return []string{value}
	}
	return defaultValue
}

// Validate checks if the configuration is valid
func (c *Config) Validate() error {
	if c.HTTP.Address == "" {
		return fmt.Errorf("HTTP address cannot be empty")
	}

	return nil
}
