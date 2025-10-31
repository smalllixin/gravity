package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// ChunkIndex represents the index mapping trace_id to content hashes
type ChunkIndex struct {
	TraceID string   `json:"trace_id"`
	SpanID  string   `json:"span_id"`
	Hashes  []string `json:"hashes"`
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: %s <trace_id>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nExample:\n")
		fmt.Fprintf(os.Stderr, "  %s 44e0c73c00b2914b0b08945fd2665935\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "\nEnvironment variables:\n")
		fmt.Fprintf(os.Stderr, "  S3_BUCKET (default: traces)\n")
		fmt.Fprintf(os.Stderr, "  S3_REGION (default: us-east-1)\n")
		fmt.Fprintf(os.Stderr, "  AWS_ENDPOINT_URL (for MinIO)\n")
		os.Exit(1)
	}

	traceID := os.Args[1]

	// Get config from environment
	bucket := getEnv("S3_BUCKET", "traces")
	region := getEnv("S3_REGION", "us-east-1")

	log.Printf("Reconstructing trace: %s", traceID)
	log.Printf("Using S3 bucket: %s (region: %s)", bucket, region)

	// Load AWS config
	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
	)
	if err != nil {
		log.Fatalf("Failed to load AWS config: %v", err)
	}

	// Create S3 client with path-style addressing for MinIO compatibility
	s3Client := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		o.UsePathStyle = true
	})

	// Step 1: Download and parse index
	log.Printf("Step 1: Fetching index for trace %s...", traceID)
	index, err := downloadIndex(context.Background(), s3Client, bucket, traceID)
	if err != nil {
		log.Fatalf("Failed to download index: %v", err)
	}

	log.Printf("Found index with %d chunks", len(index.Hashes))

	// Step 2: Download and decompress each blob
	log.Printf("Step 2: Downloading and decompressing %d blobs...", len(index.Hashes))
	var reconstructed strings.Builder
	for i, hash := range index.Hashes {
		log.Printf("  [%d/%d] Fetching blob %s...", i+1, len(index.Hashes), hash[:12])

		content, err := downloadAndDecompressBlob(context.Background(), s3Client, bucket, hash)
		if err != nil {
			log.Fatalf("Failed to download blob %s: %v", hash, err)
		}

		// Append with newline separator (since we chunked by newlines)
		if i > 0 {
			reconstructed.WriteString("\n")
		}
		reconstructed.WriteString(content)
	}

	// Step 3: Output reconstructed content
	fmt.Println("\n" + strings.Repeat("=", 80))
	fmt.Printf("Reconstructed content for trace %s (span %s):\n", index.TraceID, index.SpanID)
	fmt.Println(strings.Repeat("=", 80))
	fmt.Println(reconstructed.String())
	fmt.Println(strings.Repeat("=", 80))

	log.Printf("✓ Successfully reconstructed %d bytes from %d chunks", reconstructed.Len(), len(index.Hashes))
}

func downloadIndex(ctx context.Context, s3Client *s3.Client, bucket, traceID string) (*ChunkIndex, error) {
	key := fmt.Sprintf("indexes/%s.json", traceID)

	result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to get index from S3: %w", err)
	}
	defer result.Body.Close()

	data, err := io.ReadAll(result.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read index body: %w", err)
	}

	var index ChunkIndex
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("failed to parse index JSON: %w", err)
	}

	return &index, nil
}

func downloadAndDecompressBlob(ctx context.Context, s3Client *s3.Client, bucket, hash string) (string, error) {
	// Construct blob key: blobs/{hash[0:2]}/{hash}.gz
	key := fmt.Sprintf("blobs/%s/%s.gz", hash[:2], hash)

	result, err := s3Client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: &bucket,
		Key:    &key,
	})
	if err != nil {
		return "", fmt.Errorf("failed to get blob from S3: %w", err)
	}
	defer result.Body.Close()

	// Decompress gzip
	gzReader, err := gzip.NewReader(result.Body)
	if err != nil {
		return "", fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gzReader.Close()

	var buf bytes.Buffer
	if _, err := io.Copy(&buf, gzReader); err != nil {
		return "", fmt.Errorf("failed to decompress blob: %w", err)
	}

	return buf.String(), nil
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
