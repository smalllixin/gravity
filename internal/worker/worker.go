package worker

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// Worker is the compression worker that polls S3 and processes raw spans
type Worker struct {
	cfg       *Config
	s3Client  *s3.Client
	processor *Processor

	// Track processed files to avoid reprocessing
	processedFiles map[string]bool
	mu             sync.RWMutex
}

// New creates a new compression worker
func New(cfg *Config) (*Worker, error) {
	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(cfg.S3Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// Create S3 client with path-style addressing for MinIO compatibility
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	// Create processor
	processor := NewProcessor(cfg, s3Client)

	return &Worker{
		cfg:            cfg,
		s3Client:       s3Client,
		processor:      processor,
		processedFiles: make(map[string]bool),
	}, nil
}

// Start begins the worker polling loop
func (w *Worker) Start(ctx context.Context) error {
	log.Println("Worker started, polling for new files...")

	ticker := time.NewTicker(w.cfg.PollInterval)
	defer ticker.Stop()

	// Process immediately on start
	if err := w.pollAndProcess(ctx); err != nil {
		log.Printf("Error during initial poll: %v", err)
	}

	for {
		select {
		case <-ctx.Done():
			log.Println("Worker stopping due to context cancellation")
			return ctx.Err()
		case <-ticker.C:
			if err := w.pollAndProcess(ctx); err != nil {
				log.Printf("Error during poll: %v", err)
			}
		}
	}
}

// pollAndProcess lists new files in S3 and processes them
func (w *Worker) pollAndProcess(ctx context.Context) error {
	log.Printf("Polling S3 bucket %s for new files in %s", w.cfg.S3Bucket, w.cfg.RawSpansPath)

	// List objects in raw-spans/
	input := &s3.ListObjectsV2Input{
		Bucket: &w.cfg.S3Bucket,
		Prefix: &w.cfg.RawSpansPath,
	}

	paginator := s3.NewListObjectsV2Paginator(w.s3Client, input)

	filesFound := 0
	filesProcessed := 0

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("failed to list S3 objects: %w", err)
		}

		for _, obj := range page.Contents {
			filesFound++

			key := *obj.Key

			// Skip if already processed
			if w.isProcessed(key) {
				continue
			}

			// Process file
			log.Printf("Processing new file: %s (size: %d bytes)", key, obj.Size)
			if err := w.processor.ProcessFile(ctx, key); err != nil {
				log.Printf("ERROR: Failed to process %s: %v", key, err)
				continue
			}

			// Mark as processed
			w.markProcessed(key)
			filesProcessed++
		}
	}

	if filesFound > 0 {
		log.Printf("Poll complete: found %d files, processed %d new files", filesFound, filesProcessed)
	}

	return nil
}

// isProcessed checks if a file has already been processed
func (w *Worker) isProcessed(key string) bool {
	w.mu.RLock()
	defer w.mu.RUnlock()
	return w.processedFiles[key]
}

// markProcessed marks a file as processed
func (w *Worker) markProcessed(key string) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.processedFiles[key] = true
}
