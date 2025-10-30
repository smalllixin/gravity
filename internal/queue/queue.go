package queue

import (
	"context"

	"github.com/youware/gravity/internal/ingest/pipeline"
)

// Producer defines the interface for publishing envelopes to a queue
type Producer interface {
	// Publish sends a batch of envelopes to the queue
	Publish(ctx context.Context, batch *pipeline.Batch) error

	// Close gracefully shuts down the producer
	Close() error
}

// Consumer defines the interface for consuming envelopes from a queue
type Consumer interface {
	// Consume starts consuming messages from the queue
	Consume(ctx context.Context, handler func(envelope pipeline.Envelope) error) error

	// Close gracefully shuts down the consumer
	Close() error
}

// Config holds queue-specific configuration
type Config struct {
	Type       string
	Brokers    []string
	Topic      string
	MaxRetries int
}
