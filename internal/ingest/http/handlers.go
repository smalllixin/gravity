package http

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"

	collectormetricspb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	collectortracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/youware/gravity/internal/ingest/filter"
	"github.com/youware/gravity/internal/ingest/pipeline"
	"github.com/youware/gravity/internal/shared/config"
)

// Handler handles HTTP requests for OTLP ingestion
type Handler struct {
	config     *config.Config
	batch      *pipeline.Batch
	spanFilter *filter.Config
}

// NewHandler creates a new handler instance
func NewHandler(cfg *config.Config) *Handler {
	return &Handler{
		config:     cfg,
		batch:      pipeline.NewBatch(),
		spanFilter: filter.Default(),
	}
}

// HandleTraces processes incoming OTLP trace data
func (h *Handler) HandleTraces(w http.ResponseWriter, r *http.Request) {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("failed to read trace request body", "error", err, "path", r.URL.Path)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Extract org_id from header for multi-tenancy
	orgID := r.Header.Get("x-org-id")
	if orgID == "" {
		orgID = "default"
	}

	// Decode OTLP protobuf
	var exportReq collectortracepb.ExportTraceServiceRequest
	if err := proto.Unmarshal(body, &exportReq); err != nil {
		slog.Error("failed to unmarshal OTLP trace data", "error", err, "org_id", orgID)
		http.Error(w, "Invalid OTLP protobuf data", http.StatusBadRequest)
		return
	}

	// Process each span and convert to envelopes
	envelopes := h.convertOTLPToEnvelopes(orgID, &exportReq)

	// Log envelope details for debugging
	for i, envelope := range envelopes {
		// Log structured data summary
		slog.Info("trace envelope summary", "org_id", orgID, "index", i+1)
		slog.Info(envelope.LogSummary())
		// slog.Info("trace envelope summary", "org_id", orgID, "index", i+1, "summary", envelope.LogSummary())
	}

	// Acknowledge receipt
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"status":     "accepted",
		"span_count": len(envelopes),
		"org_id":     orgID,
	})
}

// HandleMetrics processes incoming OTLP metrics data
func (h *Handler) HandleMetrics(w http.ResponseWriter, r *http.Request) {
	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		slog.Error("failed to read metrics request body", "error", err, "path", r.URL.Path)
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	// Extract org_id from header
	orgID := r.Header.Get("x-org-id")
	if orgID == "" {
		orgID = "default"
	}

	// Decode OTLP metrics protobuf
	var exportReq collectormetricspb.ExportMetricsServiceRequest
	if err := proto.Unmarshal(body, &exportReq); err != nil {
		slog.Error("failed to unmarshal OTLP metrics data", "error", err, "org_id", orgID)
		http.Error(w, "Invalid OTLP protobuf data", http.StatusBadRequest)
		return
	}

	// Convert to JSON for readable logging
	marshaler := protojson.MarshalOptions{
		Multiline:       true,
		Indent:          "  ",
		EmitUnpopulated: false,
	}
	jsonData, err := marshaler.Marshal(&exportReq)
	if err != nil {
		slog.Error("failed to marshal metrics payload", "error", err, "org_id", orgID)
	} else {
		slog.Info("received OTLP metrics payload", "org_id", orgID)
		fmt.Println(string(jsonData))
	}

	// Count metrics for summary
	metricsCount := 0
	for _, rm := range exportReq.GetResourceMetrics() {
		for _, sm := range rm.GetScopeMetrics() {
			metricsCount += len(sm.GetMetrics())
		}
	}

	// Acknowledge receipt
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]any{
		"status":        "accepted",
		"metrics_count": metricsCount,
		"org_id":        orgID,
	})
}
