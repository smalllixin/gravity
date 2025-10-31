package worker

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseOTLPFile_RealData(t *testing.T) {
	// Open the real test file
	testFile := filepath.Join("..", "..", "testdata", "batch-traces_529804582.json.gz")
	file, err := os.Open(testFile)
	if err != nil {
		t.Fatalf("Failed to open test file: %v", err)
	}
	defer file.Close()

	// Parse the file
	extracted, err := ParseOTLPFile(file)
	if err != nil {
		t.Fatalf("Failed to parse OTLP file: %v", err)
	}

	// Log results
	t.Logf("Successfully parsed file: found %d spans with content", len(extracted))

	for i, content := range extracted {
		t.Logf("\nSpan %d:", i+1)
		t.Logf("  TraceID: %s", content.TraceID)
		t.Logf("  SpanID: %s", content.SpanID)
		t.Logf("  Content length: %d bytes", len(content.Content))

		// Show first 200 chars of content
		preview := content.Content
		if len(preview) > 200 {
			preview = preview[:200] + "..."
		}
		t.Logf("  Content preview: %s", preview)
	}

	// Basic validation
	if len(extracted) == 0 {
		t.Error("Expected at least one span with content, got 0")
	}

	for i, content := range extracted {
		if content.TraceID == "" {
			t.Errorf("Span %d: TraceID is empty", i)
		}
		if content.SpanID == "" {
			t.Errorf("Span %d: SpanID is empty", i)
		}
		// Content can be empty for some spans (e.g., non-LLM spans)
	}
}

func TestChunkContent(t *testing.T) {
	cfg := &Config{
		ChunkSeparator: "\n",
	}
	p := &Processor{cfg: cfg}

	tests := []struct {
		name     string
		input    string
		expected int // expected number of chunks
	}{
		{
			name:     "empty string",
			input:    "",
			expected: 0,
		},
		{
			name:     "single line",
			input:    "Hello world",
			expected: 1,
		},
		{
			name:     "multiple lines",
			input:    "Line 1\nLine 2\nLine 3",
			expected: 3,
		},
		{
			name:     "with empty lines",
			input:    "Line 1\n\nLine 2\n\n\nLine 3",
			expected: 3, // Empty lines should be filtered out
		},
		{
			name:     "only newlines",
			input:    "\n\n\n",
			expected: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunks := p.chunkContent(tt.input)
			if len(chunks) != tt.expected {
				t.Errorf("Expected %d chunks, got %d", tt.expected, len(chunks))
			}
		})
	}
}

func TestHashChunk(t *testing.T) {
	cfg := &Config{}
	p := &Processor{cfg: cfg}

	// Test that same content produces same hash
	content := "Hello, World!"
	hash1 := p.hashChunk(content)
	hash2 := p.hashChunk(content)

	if hash1 != hash2 {
		t.Errorf("Same content should produce same hash, got %s and %s", hash1, hash2)
	}

	// Test that hash is 64 hex characters (BLAKE3 256-bit)
	if len(hash1) != 64 {
		t.Errorf("Expected hash length 64, got %d", len(hash1))
	}

	// Test different content produces different hash
	hash3 := p.hashChunk("Different content")
	if hash1 == hash3 {
		t.Error("Different content should produce different hashes")
	}
}

func TestCompressChunk(t *testing.T) {
	p := &Processor{}

	content := "This is a test string that should be compressed using gzip."
	compressed, err := p.compressChunk(content)
	if err != nil {
		t.Fatalf("Failed to compress chunk: %v", err)
	}

	// Compressed data should be non-empty
	if len(compressed) == 0 {
		t.Error("Compressed data is empty")
	}

	t.Logf("Original size: %d bytes, Compressed size: %d bytes", len(content), len(compressed))

	// For short strings, compression might actually increase size due to gzip headers
	// Just verify we got valid gzip data by checking magic bytes
	if len(compressed) < 2 || compressed[0] != 0x1f || compressed[1] != 0x8b {
		t.Error("Compressed data doesn't have valid gzip magic bytes")
	}
}
