"""
Custom OpenTelemetry callback using pure OpenInference semantic conventions
Based on Arize Phoenix implementation but for any OTEL collector

This module provides a custom OTEL logger that:
- Uses pure OpenInference semantic conventions (Arize/Phoenix standard)
- Always logs tools (no message_logging guard)
- Connects directly to your OTEL collector
- No vendor-specific attributes
- Industry-standard semantic conventions
"""

import os
from typing import Optional

from litellm._logging import verbose_logger
from litellm.integrations.opentelemetry import (
    OpenTelemetry,
    OpenTelemetryConfig,
)

# Constants for OTEL tracing/metrics
LITELLM_TRACER_NAME = os.getenv("OTEL_TRACER_NAME", "litellm")


class OpenInferenceOtelLogger(OpenTelemetry):
    """
    Pure OpenInference OTEL logger (Arize Phoenix style).

    Uses industry-standard OpenInference semantic conventions without
    vendor-specific attributes.

    This logger:
    - Uses pure OpenInference semantic conventions (same as Arize Phoenix)
    - Always logs tools (no message_logging guard)
    - Connects directly to your OTEL collector
    - No vendor lock-in
    - Cleaner, simpler attribute structure

    Configuration via environment variables:
    - OTEL_EXPORTER_OTLP_ENDPOINT: Your OTEL collector endpoint
    - OTEL_EXPORTER_OTLP_PROTOCOL: Protocol (grpc or http/protobuf)
    - OTEL_EXPORTER_OTLP_HEADERS: Optional headers (if needed)
    - LITELLM_OTEL_INTEGRATION_ENABLE_METRICS: Enable metrics (true/false)
    - LITELLM_OTEL_INTEGRATION_ENABLE_EVENTS: Enable events/logs (true/false)
    """

    def __init__(self, *args, **kwargs):
        """
        Initialize with pure OpenInference conventions.
        """
        # Get standard OTEL configuration from environment
        config = OpenTelemetryConfig.from_env()

        # Override config if not provided
        if "config" not in kwargs:
            kwargs["config"] = config

        # Call parent constructor
        super().__init__(*args, **kwargs)

        verbose_logger.info(
            "✓ OpenInference OTEL Logger initialized (pure OpenInference/Arize Phoenix style)"
        )
        verbose_logger.info(f"  Endpoint: {config.endpoint}")
        verbose_logger.info(f"  Protocol: {config.exporter}")
        verbose_logger.info(f"  Metrics: {config.enable_metrics}")
        verbose_logger.info(f"  Events: {config.enable_events}")

    def _init_metrics(self, meter_provider):
        """
        Override to add TTFT histogram to metrics.
        """
        # Call parent to initialize standard metrics
        super()._init_metrics(meter_provider)

        # Add TTFT histogram if metrics are enabled
        if self.config.enable_metrics:
            from opentelemetry import metrics

            meter = metrics.get_meter(LITELLM_TRACER_NAME)
            self._ttft_histogram = meter.create_histogram(
                name="gen_ai.client.time_to_first_token",
                description="Time to first token (prompt processing latency)",
                unit="s",
            )
            verbose_logger.info("  TTFT Metric: Enabled")
        else:
            self._ttft_histogram = None

    def _record_metrics(self, kwargs, response_obj, start_time, end_time):
        """
        Override to record TTFT metric.
        """
        # Call parent to record standard metrics
        super()._record_metrics(kwargs, response_obj, start_time, end_time)

        # Record TTFT if available
        if self._ttft_histogram:
            standard_logging_payload = kwargs.get("standard_logging_object")
            if standard_logging_payload:
                start = standard_logging_payload.get("startTime")
                completion_start = standard_logging_payload.get("completionStartTime")

                if start and completion_start:
                    ttft_seconds = completion_start - start

                    # Get common attributes from parent (model, provider, etc.)
                    common_attrs = self._get_common_metric_attributes(kwargs, response_obj)

                    self._ttft_histogram.record(ttft_seconds, attributes=common_attrs)

    def _get_common_metric_attributes(self, kwargs, response_obj):
        """
        Extract common attributes for metrics (matches parent's format).
        """
        litellm_params = kwargs.get("litellm_params", {})
        standard_logging_payload = kwargs.get("standard_logging_object")

        common_attrs = {
            "gen_ai.operation.name": standard_logging_payload.get("call_type") if standard_logging_payload else "unknown",
            "gen_ai.request.model": kwargs.get("model", "unknown"),
            "gen_ai.system": litellm_params.get("custom_llm_provider", "unknown"),
        }

        if response_obj and response_obj.get("model"):
            common_attrs["gen_ai.response.model"] = response_obj.get("model")

        return common_attrs

    def set_attributes(self, span, kwargs, response_obj):
        """
        Override to use pure OpenInference attributes with tool format fix.

        This bypasses the callback_name routing in the parent class
        and uses a custom implementation that handles both nested and flattened tool formats.
        """
        # Use our custom OpenInference attribute setter (defined below)
        self._set_openinference_attributes(span, kwargs, response_obj)

    # def set_raw_request_attributes(self, span, kwargs, response_obj):
    #     """
    #     Override to use pure OpenInference attributes on raw request sub-span.

    #     This ensures tool attributes appear on the raw_gen_ai_request span
    #     which is the primary span shown in Jaeger/observability tools.
    #     """
    #     # Use our custom OpenInference attribute setter
    #     self._set_openinference_attributes(span, kwargs, response_obj)

    #     # Also call parent to get any additional raw request attributes
    #     super().set_raw_request_attributes(span, kwargs, response_obj)

    def _set_openinference_attributes(self, span, kwargs, response_obj):
        """
        Custom implementation of OpenInference attribute setting with tool format fix.

        This is based on litellm.integrations.arize._utils.set_attributes but includes
        a fix to handle both nested and flattened tool formats.
        """
        import json
        from litellm.integrations._types.open_inference import (
            MessageAttributes,
            OpenInferenceSpanKindValues,
            SpanAttributes,
        )
        from litellm.litellm_core_utils.safe_json_dumps import safe_dumps
        from litellm.types.utils import StandardLoggingPayload

        try:
            optional_params = kwargs.get("optional_params", {})
            litellm_params = kwargs.get("litellm_params", {})
            standard_logging_payload: Optional[StandardLoggingPayload] = kwargs.get(
                "standard_logging_object"
            )
            if standard_logging_payload is None:
                raise ValueError("standard_logging_object not found in kwargs")

            # Set metadata
            metadata = standard_logging_payload.get("metadata") if standard_logging_payload else None
            if metadata is not None:
                self.safe_set_attribute(span, SpanAttributes.METADATA, safe_dumps(metadata))

            # Set model name
            if kwargs.get("model"):
                self.safe_set_attribute(span, SpanAttributes.LLM_MODEL_NAME, kwargs.get("model"))

            # Set request type
            self.safe_set_attribute(span, "llm.request.type", standard_logging_payload["call_type"])

            # Set provider
            self.safe_set_attribute(
                span,
                SpanAttributes.LLM_PROVIDER,
                litellm_params.get("custom_llm_provider", "Unknown"),
            )

            # Set optional params
            if optional_params.get("max_tokens"):
                self.safe_set_attribute(span, "llm.request.max_tokens", optional_params.get("max_tokens"))
            if optional_params.get("temperature"):
                self.safe_set_attribute(span, "llm.request.temperature", optional_params.get("temperature"))
            if optional_params.get("top_p"):
                self.safe_set_attribute(span, "llm.request.top_p", optional_params.get("top_p"))

            # Set streaming flag
            self.safe_set_attribute(span, "llm.is_streaming", str(optional_params.get("stream", False)))

            # Set user if present
            if optional_params.get("user"):
                self.safe_set_attribute(span, "llm.user", optional_params.get("user"))

            # Set response ID and model
            if response_obj and response_obj.get("id"):
                self.safe_set_attribute(span, "llm.response.id", response_obj.get("id"))
            if response_obj and response_obj.get("model"):
                self.safe_set_attribute(span, "llm.response.model", response_obj.get("model"))

            # Set span kind
            self.safe_set_attribute(
                span,
                SpanAttributes.OPENINFERENCE_SPAN_KIND,
                OpenInferenceSpanKindValues.LLM.value,
            )

            # Set input messages
            messages = kwargs.get("messages")
            if messages:
                last_message = messages[-1]
                self.safe_set_attribute(span, SpanAttributes.INPUT_VALUE, last_message.get("content", ""))

                for idx, msg in enumerate(messages):
                    prefix = f"{SpanAttributes.LLM_INPUT_MESSAGES}.{idx}"
                    self.safe_set_attribute(span, f"{prefix}.{MessageAttributes.MESSAGE_ROLE}", msg.get("role"))
                    self.safe_set_attribute(span, f"{prefix}.{MessageAttributes.MESSAGE_CONTENT}", msg.get("content", ""))

            # Set tools (function definitions) - WITH FIX FOR FLATTENED FORMAT
            tools = optional_params.get("tools")
            if tools:
                for idx, tool in enumerate(tools):
                    # Handle both formats:
                    # 1. OpenAI nested format: {"type": "function", "function": {"name": "...", "description": "...", "parameters": {...}}}
                    # 2. Flattened format: {"name": "...", "description": "...", "parameters": {...}, "type": "function"}
                    function = tool.get("function")
                    if not function:
                        # Flattened format - use the tool object directly
                        if "name" in tool:
                            function = tool
                        else:
                            continue

                    prefix = f"{SpanAttributes.LLM_TOOLS}.{idx}"
                    self.safe_set_attribute(span, f"{prefix}.{SpanAttributes.TOOL_NAME}", function.get("name"))
                    self.safe_set_attribute(span, f"{prefix}.{SpanAttributes.TOOL_DESCRIPTION}", function.get("description"))
                    self.safe_set_attribute(span, f"{prefix}.{SpanAttributes.TOOL_PARAMETERS}", json.dumps(function.get("parameters")))

            # Set invocation parameters
            model_params = standard_logging_payload.get("model_parameters") if standard_logging_payload else None
            if model_params:
                self.safe_set_attribute(span, SpanAttributes.LLM_INVOCATION_PARAMETERS, safe_dumps(model_params))
                if model_params.get("user"):
                    user_id = model_params.get("user")
                    if user_id is not None:
                        self.safe_set_attribute(span, SpanAttributes.USER_ID, user_id)

            # Set TTFT timing (Time to First Token) - Following OpenInference naming pattern
            if standard_logging_payload:
                start = standard_logging_payload.get("startTime")
                completion_start = standard_logging_payload.get("completionStartTime")
                end = standard_logging_payload.get("endTime")

                if start and completion_start:
                    # Time to first token (prompt processing latency)
                    ttft_seconds = completion_start - start
                    self.safe_set_attribute(span, "llm.latency.time_to_first_token", ttft_seconds)

                    # Optional: Token generation time (time from first to last token)
                    if end:
                        token_gen_time = end - completion_start
                        self.safe_set_attribute(span, "llm.latency.token_generation", token_gen_time)

                        # Total latency
                        total_latency = end - start
                        self.safe_set_attribute(span, "llm.latency.total", total_latency)

            # Set output messages and tokens
            if hasattr(response_obj, "get"):
                for idx, choice in enumerate(response_obj.get("choices", [])):
                    response_message = choice.get("message", {})
                    self.safe_set_attribute(span, SpanAttributes.OUTPUT_VALUE, response_message.get("content", ""))

                    prefix = f"{SpanAttributes.LLM_OUTPUT_MESSAGES}.{idx}"
                    self.safe_set_attribute(span, f"{prefix}.{MessageAttributes.MESSAGE_ROLE}", response_message.get("role"))
                    self.safe_set_attribute(span, f"{prefix}.{MessageAttributes.MESSAGE_CONTENT}", response_message.get("content", ""))

                # Set token usage
                usage = response_obj and response_obj.get("usage")
                if usage:
                    self.safe_set_attribute(span, SpanAttributes.LLM_TOKEN_COUNT_TOTAL, usage.get("total_tokens"))
                    self.safe_set_attribute(span, SpanAttributes.LLM_TOKEN_COUNT_COMPLETION, usage.get("completion_tokens"))
                    self.safe_set_attribute(span, SpanAttributes.LLM_TOKEN_COUNT_PROMPT, usage.get("prompt_tokens"))

        except Exception as e:
            verbose_logger.error(f"[OpenInference] Failed to set span attributes: {e}")
            if hasattr(span, "record_exception"):
                span.record_exception(e)


# Create a singleton instance for LiteLLM to use
# LiteLLM will import this instance when you specify:
# callbacks: openinference_otel.openinference_logger
openinference_logger = OpenInferenceOtelLogger()
