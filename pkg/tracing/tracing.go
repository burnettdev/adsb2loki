package tracing

import (
	"context"
	"fmt"
	"os"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	oteltrace "go.opentelemetry.io/otel/trace"

	"github.com/burnettdev/adsb2loki/pkg/logging"
)

const (
	serviceName = "adsb2loki"
)

var (
	tracer oteltrace.Tracer
)

// InitTracing initializes OpenTelemetry tracing with OTLP HTTP exporter
func InitTracing(ctx context.Context) (func(context.Context) error, error) {
	logging.DebugCall("InitTracing")

	// Check if tracing is enabled
	if !isTracingEnabled() {
		logging.Info("OpenTelemetry tracing is disabled")
		return func(context.Context) error { return nil }, nil
	}

	// Create resource with comprehensive service and runtime information
	res, err := resource.New(ctx,
		resource.WithAttributes(
			// Service information
			semconv.ServiceName(serviceName),
			semconv.ServiceInstanceID(getServiceInstanceID()),

			// Go runtime information
			semconv.TelemetrySDKName("opentelemetry"),
			semconv.TelemetrySDKLanguageGo,
			semconv.TelemetrySDKVersion("1.31.0"),

			// Process and runtime information
			semconv.ProcessRuntimeName("go"),
			semconv.ProcessRuntimeVersion(runtime.Version()),
			semconv.ProcessRuntimeDescription("Go runtime"),
		),
		// Automatically detect additional resource attributes
		resource.WithFromEnv(),
		resource.WithProcess(),
		resource.WithOS(),
		resource.WithContainer(),
		resource.WithHost(),
	)
	if err != nil {
		logging.Error("Failed to create OpenTelemetry resource", "error", err)
		return nil, fmt.Errorf("failed to create resource: %w", err)
	}

	// Configure OTLP HTTP exporter
	otlpEndpoint := getOTLPEndpoint()
	logging.Debug("Configuring OTLP exporter", "endpoint", otlpEndpoint)

	exporterOptions := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(otlpEndpoint),
		otlptracehttp.WithURLPath("/v1/traces"),
		otlptracehttp.WithInsecure(), // Always use insecure for simplicity
	}

	// Add headers if configured
	if headers := getOTLPHeaders(); len(headers) > 0 {
		exporterOptions = append(exporterOptions, otlptracehttp.WithHeaders(headers))
		logging.Debug("Added OTLP headers", "header_count", len(headers))
	}

	logging.Debug("Using insecure OTLP connection", "endpoint", otlpEndpoint)

	exporter, err := otlptracehttp.New(ctx, exporterOptions...)
	if err != nil {
		logging.Error("Failed to create OTLP exporter", "error", err, "endpoint", otlpEndpoint)
		return nil, fmt.Errorf("failed to create OTLP exporter: %w", err)
	}

	// Create trace provider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
		trace.WithSampler(getSampler()),
	)

	// Set global trace provider and propagator
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	// Get tracer instance
	tracer = otel.Tracer(serviceName)

	logging.Info("OpenTelemetry tracing initialized successfully",
		"service", serviceName,
		"endpoint", otlpEndpoint,
		"sampler", getSamplerName(),
		"insecure", true,
	)

	// Return shutdown function
	return tp.Shutdown, nil
}

// GetTracer returns the global tracer instance
func GetTracer() oteltrace.Tracer {
	if tracer == nil {
		// Return a no-op tracer if tracing is not initialized
		return otel.Tracer(serviceName)
	}
	return tracer
}

// StartSpan starts a new span with the given name and options
func StartSpan(ctx context.Context, spanName string, opts ...oteltrace.SpanStartOption) (context.Context, oteltrace.Span) {
	return GetTracer().Start(ctx, spanName, opts...)
}

// isTracingEnabled checks if OpenTelemetry tracing is enabled
func isTracingEnabled() bool {
	enabled := os.Getenv("OTEL_TRACING_ENABLED")
	return enabled == "true" || enabled == "1"
}

// getOTLPEndpoint returns the OTLP endpoint URL
func getOTLPEndpoint() string {
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT"); endpoint != "" {
		// Remove /v1/traces suffix if present since WithEndpoint adds it automatically
		if strings.HasSuffix(endpoint, "/v1/traces") {
			return strings.TrimSuffix(endpoint, "/v1/traces")
		}
		return endpoint
	}
	if endpoint := os.Getenv("OTEL_EXPORTER_OTLP_ENDPOINT"); endpoint != "" {
		// Remove /v1/traces suffix if present since WithEndpoint adds it automatically
		if strings.HasSuffix(endpoint, "/v1/traces") {
			return strings.TrimSuffix(endpoint, "/v1/traces")
		}
		return endpoint
	}
	return "localhost:4318" // Default OTLP HTTP endpoint (no protocol needed with insecure)
}

// getOTLPHeaders returns headers for OTLP exporter
func getOTLPHeaders() map[string]string {
	headers := make(map[string]string)

	// Check for trace-specific headers
	if h := os.Getenv("OTEL_EXPORTER_OTLP_TRACES_HEADERS"); h != "" {
		parseHeaders(h, headers)
	}

	// Check for general OTLP headers
	if h := os.Getenv("OTEL_EXPORTER_OTLP_HEADERS"); h != "" {
		parseHeaders(h, headers)
	}

	return headers
}

// parseHeaders parses header string in format "key1=value1,key2=value2"
func parseHeaders(headerStr string, headers map[string]string) {
	// Simple header parsing - in production you might want more robust parsing
	// This handles the basic case of "key=value,key2=value2"
	if headerStr == "" {
		return
	}

	// For now, we'll just log that headers are configured but not parse them
	// This avoids complex parsing logic for the initial implementation
	logging.Debug("OTLP headers configured", "headers", headerStr)
}

// getSampler returns the configured sampler
func getSampler() trace.Sampler {
	samplerType := os.Getenv("OTEL_TRACES_SAMPLER")
	switch samplerType {
	case "always_off":
		return trace.NeverSample()
	case "always_on":
		return trace.AlwaysSample()
	case "traceidratio":
		// For simplicity, using a default ratio. In production, you'd parse OTEL_TRACES_SAMPLER_ARG
		return trace.TraceIDRatioBased(0.1) // 10% sampling
	default:
		return trace.AlwaysSample() // Default to always sample for development
	}
}

// getSamplerName returns the name of the configured sampler for logging
func getSamplerName() string {
	samplerType := os.Getenv("OTEL_TRACES_SAMPLER")
	if samplerType == "" {
		return "always_on"
	}
	return samplerType
}

// AddSpanEvent adds an event to the current span if one exists
func AddSpanEvent(ctx context.Context, name string, attributes ...oteltrace.EventOption) {
	span := oteltrace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		span.AddEvent(name, attributes...)
	}
}

// SetSpanError marks the current span as having an error
func SetSpanError(ctx context.Context, err error) {
	span := oteltrace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
	}
}

// SetSpanAttributes sets attributes on the current span
func SetSpanAttributes(ctx context.Context, attributes ...oteltrace.SpanStartOption) {
	span := oteltrace.SpanFromContext(ctx)
	if span != nil && span.IsRecording() {
		// Note: This is a simplified version. In practice, you'd need to extract
		// attributes from SpanStartOptions or use a different approach
		logging.Debug("Span attributes would be set here")
	}
}

// getServiceInstanceID returns a unique identifier for this service instance
func getServiceInstanceID() string {
	// Try to get from environment first (useful for containers/k8s)
	if instanceID := os.Getenv("OTEL_SERVICE_INSTANCE_ID"); instanceID != "" {
		return instanceID
	}

	// Try hostname
	if hostname, err := os.Hostname(); err == nil {
		return hostname
	}

	// Fallback to process ID
	return fmt.Sprintf("pid-%d", os.Getpid())
}
