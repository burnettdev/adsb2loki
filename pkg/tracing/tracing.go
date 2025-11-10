package tracing

import (
	"context"
	"log"
	"os"
	"runtime"
	"strings"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
)

func InitTracing() (func(), error) {
	// Check if tracing is enabled
	if enabled := getEnv("OTEL_TRACING_ENABLED", "false"); !isTrue(enabled) {
		log.Println("OpenTelemetry tracing is disabled")
		return func() {}, nil
	}

	// Get OTLP endpoint from environment variables
	endpoint := getOTLPEndpoint()

	// Check if connection should be insecure
	insecure := isTrue(getEnv("OTEL_EXPORTER_OTLP_TRACES_INSECURE", "true"))

	// Parse headers if provided
	headers := parseHeaders(getEnv("OTEL_EXPORTER_OTLP_TRACES_HEADERS", ""))

	// Create OTLP exporter options
	opts := []otlptracehttp.Option{
		otlptracehttp.WithEndpoint(endpoint),
	}

	if insecure {
		opts = append(opts, otlptracehttp.WithInsecure())
	}

	if len(headers) > 0 {
		opts = append(opts, otlptracehttp.WithHeaders(headers))
	}

	// Create OTLP exporter
	exporter, err := otlptracehttp.New(context.Background(), opts...)
	if err != nil {
		log.Printf("Failed to create OTLP exporter, using noop: %v", err)
		// Return a noop shutdown function if exporter creation fails
		return func() {}, nil
	}

	// Create resource with Go-specific attributes
	res, err := resource.New(context.Background(),
		resource.WithAttributes(
			// Service identification
			semconv.ServiceName("adsb2loki"),
			semconv.ServiceVersion("1.0.0"),

			// Process and runtime information
			semconv.ProcessRuntimeName("go"),
			semconv.ProcessRuntimeVersion(runtime.Version()),
			semconv.ProcessRuntimeDescription("Go runtime"),
			semconv.ProcessPID(os.Getpid()),

			// Telemetry SDK information
			semconv.TelemetrySDKName("opentelemetry"),
			semconv.TelemetrySDKLanguageGo,
			semconv.TelemetrySDKVersion("1.31.0"),
		),
	)
	if err != nil {
		return nil, err
	}

	// Create trace provider
	tp := trace.NewTracerProvider(
		trace.WithBatcher(exporter),
		trace.WithResource(res),
	)

	// Set global trace provider
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.TraceContext{})

	log.Println("OpenTelemetry tracing initialized successfully")

	return func() {
		if err := tp.Shutdown(context.Background()); err != nil {
			log.Printf("Error shutting down tracer provider: %v", err)
		}
	}, nil
}

// getEnv returns the value of an environment variable or a default value if not set
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// isTrue checks if a string represents a true value
func isTrue(s string) bool {
	s = strings.ToLower(strings.TrimSpace(s))
	return s == "true" || s == "1" || s == "yes" || s == "on"
}

// getOTLPEndpoint determines the OTLP endpoint from environment variables
func getOTLPEndpoint() string {
	// Check for traces-specific endpoint first
	if endpoint := getEnv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT", ""); endpoint != "" {
		return cleanEndpoint(endpoint)
	}

	// Fall back to general OTLP endpoint
	if endpoint := getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", ""); endpoint != "" {
		return cleanEndpoint(endpoint)
	}

	// Default to localhost
	return "localhost:4318"
}

// cleanEndpoint removes protocol and path from endpoint URL
func cleanEndpoint(endpoint string) string {
	// Remove http:// or https:// prefix if present
	endpoint = strings.TrimPrefix(endpoint, "http://")
	endpoint = strings.TrimPrefix(endpoint, "https://")
	
	// Remove /v1/traces suffix if present since WithEndpoint handles the path separately
	if strings.HasSuffix(endpoint, "/v1/traces") {
		endpoint = strings.TrimSuffix(endpoint, "/v1/traces")
	}
	
	// Remove any trailing slashes
	endpoint = strings.TrimSuffix(endpoint, "/")
	
	return endpoint
}

// parseHeaders parses header string in format "key1=value1,key2=value2"
func parseHeaders(headerStr string) map[string]string {
	headers := make(map[string]string)
	if headerStr == "" {
		return headers
	}

	pairs := strings.Split(headerStr, ",")
	for _, pair := range pairs {
		if kv := strings.SplitN(strings.TrimSpace(pair), "=", 2); len(kv) == 2 {
			headers[strings.TrimSpace(kv[0])] = strings.TrimSpace(kv[1])
		}
	}

	return headers
}
