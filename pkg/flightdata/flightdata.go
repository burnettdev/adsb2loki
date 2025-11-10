package flightdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	"github.com/burnettdev/adsb2loki/pkg/logging"
	"github.com/burnettdev/adsb2loki/pkg/loki"
	"github.com/burnettdev/adsb2loki/pkg/models"
)

var (
	httpClient = &http.Client{
		Transport: otelhttp.NewTransport(http.DefaultTransport),
		Timeout:   30 * time.Second,
	}
	tracer = otel.Tracer("flightdata-client")
)

func FetchAndPushToLoki(ctx context.Context, lokiClient *loki.Client) error {
	ctx, span := tracer.Start(ctx, "flightdata.fetch_and_push",
		trace.WithAttributes(
			attribute.String("service", "adsb"),
		),
	)
	defer span.End()

	logging.DebugCall("FetchAndPushToLoki")

	flightDataURL := os.Getenv("FLIGHT_DATA_URL")
	logging.Debug("Flight data URL configured", "url", flightDataURL)

	span.SetAttributes(
		attribute.String("http.url", flightDataURL),
		attribute.String("http.method", "GET"),
	)

	// Create HTTP request with context for automatic tracing via otelhttp
	req, err := http.NewRequestWithContext(ctx, "GET", flightDataURL, nil)
	if err != nil {
		span.RecordError(err)
		logging.Error("Failed to create HTTP request", "error", err, "url", flightDataURL)
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("User-Agent", "adsb2loki/1.0.0")

	start := time.Now()
	resp, err := httpClient.Do(req)
	duration := time.Since(start)

	if err != nil {
		span.RecordError(err)
		logging.Error("Failed to fetch dump1090-fa data", "error", err, "url", flightDataURL, "duration_ms", duration.Milliseconds())
		return fmt.Errorf("failed to fetch dump1090-fa data: %w", err)
	}
	defer resp.Body.Close()

	span.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	logging.DebugHTTP("GET", flightDataURL, resp.StatusCode, duration)

	if resp.StatusCode != http.StatusOK {
		err := fmt.Errorf("HTTP request failed with status: %s", resp.Status)
		span.RecordError(err)
		logging.Error("HTTP request returned non-200 status", "status_code", resp.StatusCode, "status", resp.Status)
		return err
	}

	var data models.Dump1090fa
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		span.RecordError(err)
		logging.Error("Failed to decode dump1090-fa data", "error", err)
		return fmt.Errorf("failed to decode dump1090-fa data: %w", err)
	}

	span.SetAttributes(
		attribute.Int("aircraft.count", len(data.Aircraft)),
		attribute.Int64("data.timestamp", int64(data.Now)),
		attribute.Int("data.messages", data.Messages),
	)

	logging.Debug("Successfully parsed flight data", "aircraft_count", len(data.Aircraft), "timestamp", data.Now, "messages", data.Messages)

	entries := make([]loki.LogEntry, 0, len(data.Aircraft))
	for i, aircraft := range data.Aircraft {
		logging.Debug("Processing aircraft", "index", i, "hex", aircraft.Hex, "flight", aircraft.Flight, "lat", aircraft.Lat, "lon", aircraft.Lon, "alt_baro", aircraft.AltBaro.String())

		aircraftJSON, err := json.Marshal(aircraft)
		if err != nil {
			logging.Error("Failed to marshal aircraft data", "error", err, "aircraft_hex", aircraft.Hex)
			return fmt.Errorf("failed to marshal aircraft data: %w", err)
		}

		labels := map[string]string{
			"service": "adsb",
		}

		entry := loki.LogEntry{
			Timestamp: time.Unix(int64(data.Now), 0),
			Labels:    labels,
			Line:      string(aircraftJSON),
		}

		entries = append(entries, entry)
	}

	logging.Debug("Converted aircraft data to Loki entries", "entries_count", len(entries))

	if err := lokiClient.PushLogs(ctx, entries); err != nil {
		span.RecordError(err)
		logging.Error("Failed to push logs to Loki", "error", err, "entries_count", len(entries))
		return fmt.Errorf("failed to push logs to Loki: %w", err)
	}

	span.SetAttributes(
		attribute.Int("loki.entries_pushed", len(entries)),
	)

	logging.Info("Successfully fetched and pushed aircraft data", "aircraft_count", len(data.Aircraft), "entries_pushed", len(entries))
	return nil
}
