package worker

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/zeebo/blake3"
)

// Processor handles the compression pipeline for a single file
type Processor struct {
	cfg      *Config
	s3Client *s3.Client
}

// NewProcessor creates a new processor
func NewProcessor(cfg *Config, s3Client *s3.Client) *Processor {
	return &Processor{
		cfg:      cfg,
		s3Client: s3Client,
	}
}

// ChunkIndex represents the index mapping trace_id to content hashes
type ChunkIndex struct {
	TraceID string   `json:"trace_id"`
	SpanID  string   `json:"span_id"`
	Hashes  []string `json:"hashes"` // Ordered list of blake3 hashes
}

// ProcessFile downloads, parses, chunks, compresses, and stores a single OTLP file
func (p *Processor) ProcessFile(ctx context.Context, s3Key string) error {
	// Download file from S3
	log.Printf("Downloading %s from S3...", s3Key)
	result, err := p.s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &p.cfg.S3Bucket,
		Key:    &s3Key,
	})
	if err != nil {
		return fmt.Errorf("failed to download S3 object: %w", err)
	}
	defer result.Body.Close()

	// Parse OTLP JSON
	log.Printf("Parsing OTLP JSON from %s...", s3Key)
	extracted, err := ParseOTLPFile(result.Body)
	if err != nil {
		return fmt.Errorf("failed to parse OTLP file: %w", err)
	}

	log.Printf("Extracted %d spans with content", len(extracted))

	// Process each extracted span
	for i, content := range extracted {
		log.Printf("Processing span %d/%d (trace=%s, span=%s)",
			i+1, len(extracted), content.TraceID, content.SpanID)

		if err := p.processSpanContent(ctx, content); err != nil {
			log.Printf("ERROR: Failed to process span %s: %v", content.SpanID, err)
			continue
		}
	}

	log.Printf("Successfully processed file %s", s3Key)
	return nil
}

// processSpanContent handles chunking, hashing, compression, and storage for a single span
func (p *Processor) processSpanContent(ctx context.Context, content ExtractedContent) error {
	// Chunk content (split by separator)
	chunks := p.chunkContent(content.Content)
	if len(chunks) == 0 {
		log.Printf("No chunks generated for span %s (empty content)", content.SpanID)
		return nil
	}

	log.Printf("Generated %d chunks for span %s", len(chunks), content.SpanID)

	// Process each chunk: hash, compress, store
	var hashes []string
	for i, chunk := range chunks {
		if chunk == "" {
			continue
		}

		// Hash the chunk (BLAKE3)
		hash := p.hashChunk(chunk)
		hashes = append(hashes, hash)

		log.Printf("  Chunk %d/%d: hash=%s, size=%d bytes", i+1, len(chunks), hash[:12], len(chunk))

		// Check if blob already exists (idempotency)
		blobKey := p.getBlobKey(hash)
		if exists, err := p.blobExists(ctx, blobKey); err != nil {
			return fmt.Errorf("failed to check blob existence: %w", err)
		} else if exists {
			log.Printf("  Blob %s already exists, skipping upload", hash[:12])
			continue
		}

		// Compress chunk
		compressed, err := p.compressChunk(chunk)
		if err != nil {
			return fmt.Errorf("failed to compress chunk: %w", err)
		}

		// Store compressed blob to S3
		if err := p.storeBlob(ctx, blobKey, compressed); err != nil {
			return fmt.Errorf("failed to store blob: %w", err)
		}

		log.Printf("  Stored blob %s (%d bytes → %d bytes compressed)",
			hash[:12], len(chunk), len(compressed))
	}

	// Create and store index
	if len(hashes) > 0 {
		index := ChunkIndex{
			TraceID: content.TraceID,
			SpanID:  content.SpanID,
			Hashes:  hashes,
		}

		if err := p.storeIndex(ctx, content.TraceID, index); err != nil {
			return fmt.Errorf("failed to store index: %w", err)
		}

		log.Printf("Stored index for trace %s with %d hashes", content.TraceID, len(hashes))
	}

	return nil
}

// chunkContent splits content by the configured separator
func (p *Processor) chunkContent(content string) []string {
	if content == "" {
		return nil
	}

	// Split by separator (newlines for MVP)
	chunks := strings.Split(content, p.cfg.ChunkSeparator)

	// Filter out empty chunks
	var result []string
	for _, chunk := range chunks {
		trimmed := strings.TrimSpace(chunk)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}

	return result
}

// hashChunk computes BLAKE3 hash of a chunk
func (p *Processor) hashChunk(chunk string) string {
	hash := blake3.Sum256([]byte(chunk))
	return hex.EncodeToString(hash[:])
}

// compressChunk compresses a chunk using gzip
func (p *Processor) compressChunk(chunk string) ([]byte, error) {
	var buf bytes.Buffer
	gzWriter := gzip.NewWriter(&buf)

	if _, err := gzWriter.Write([]byte(chunk)); err != nil {
		return nil, err
	}

	if err := gzWriter.Close(); err != nil {
		return nil, err
	}

	return buf.Bytes(), nil
}

// getBlobKey returns the S3 key for a blob given its hash
// Format: blobs/{hash[0:2]}/{hash}.gz
func (p *Processor) getBlobKey(hash string) string {
	prefix := hash[:2]
	return fmt.Sprintf("%s%s/%s.gz", p.cfg.BlobsPath, prefix, hash)
}

// blobExists checks if a blob already exists in S3 (idempotency)
func (p *Processor) blobExists(ctx context.Context, key string) (bool, error) {
	_, err := p.s3Client.HeadObject(ctx, &s3.HeadObjectInput{
		Bucket: &p.cfg.S3Bucket,
		Key:    &key,
	})
	if err != nil {
		// Check if it's a NotFound error
		if strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// storeBlob stores a compressed blob to S3
func (p *Processor) storeBlob(ctx context.Context, key string, data []byte) error {
	_, err := p.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &p.cfg.S3Bucket,
		Key:         &key,
		Body:        bytes.NewReader(data),
		ContentType: aws.String("application/gzip"),
	})
	return err
}

// storeIndex stores the chunk index as JSON to S3
// Format: indexes/{trace_id}.json
func (p *Processor) storeIndex(ctx context.Context, traceID string, index ChunkIndex) error {
	// Serialize index to JSON
	jsonData, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal index: %w", err)
	}

	// Store to S3
	key := fmt.Sprintf("%s%s.json", p.cfg.IndexesPath, traceID)
	_, err = p.s3Client.PutObject(ctx, &s3.PutObjectInput{
		Bucket:      &p.cfg.S3Bucket,
		Key:         &key,
		Body:        bytes.NewReader(jsonData),
		ContentType: aws.String("application/json"),
	})
	if err != nil {
		return fmt.Errorf("failed to put index to S3: %w", err)
	}

	return nil
}
