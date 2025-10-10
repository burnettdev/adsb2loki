package flightdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/burnettdev/adsb2loki/pkg/logging"
	"github.com/burnettdev/adsb2loki/pkg/loki"
	"github.com/burnettdev/adsb2loki/pkg/models"
	"github.com/burnettdev/adsb2loki/pkg/tracing"
)

func FetchAndPushToLoki(ctx context.Context, lokiClient *loki.Client) error {
	ctx, span := tracing.StartSpan(ctx, "flightdata.FetchAndPushToLoki")
	defer span.End()

	logging.DebugCall("FetchAndPushToLoki")

	flightDataURL := os.Getenv("FLIGHT_DATA_URL")
	logging.Debug("Flight data URL configured", "url", flightDataURL)

	// Add URL as span attribute
	span.SetAttributes(attribute.String("flight_data.url", flightDataURL))

	// Create span for HTTP fetch operation
	_, fetchSpan := tracing.StartSpan(ctx, "flightdata.fetch_http")
	fetchSpan.SetAttributes(
		attribute.String("http.method", "GET"),
		attribute.String("http.url", flightDataURL),
	)

	start := time.Now()
	resp, err := http.Get(flightDataURL)
	duration := time.Since(start)

	fetchSpan.SetAttributes(attribute.Int64("http.duration_ms", duration.Milliseconds()))

	if err != nil {
		fetchSpan.RecordError(err)
		fetchSpan.SetStatus(codes.Error, err.Error())
		fetchSpan.End()
		logging.Error("Failed to fetch dump1090-fa data", "error", err, "url", flightDataURL, "duration_ms", duration.Milliseconds())
		return fmt.Errorf("failed to fetch dump1090-fa data: %w", err)
	}
	defer resp.Body.Close()

	fetchSpan.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	logging.DebugHTTP("GET", flightDataURL, resp.StatusCode, duration)

	if resp.StatusCode != http.StatusOK {
		fetchSpan.SetStatus(codes.Error, "HTTP request failed")
		fetchSpan.End()
		logging.Error("HTTP request returned non-200 status", "status_code", resp.StatusCode, "status", resp.Status)
		return fmt.Errorf("HTTP request failed with status: %s", resp.Status)
	}

	fetchSpan.SetStatus(codes.Ok, "HTTP request successful")
	fetchSpan.End()

	// Create span for JSON parsing
	_, parseSpan := tracing.StartSpan(ctx, "flightdata.parse_json")

	var data models.Dump1090fa
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		parseSpan.RecordError(err)
		parseSpan.SetStatus(codes.Error, err.Error())
		parseSpan.End()
		logging.Error("Failed to decode dump1090-fa data", "error", err)
		return fmt.Errorf("failed to decode dump1090-fa data: %w", err)
	}

	parseSpan.SetAttributes(
		attribute.Int("aircraft.count", len(data.Aircraft)),
		attribute.Int64("data.timestamp", int64(data.Now)),
		attribute.Int("data.messages", data.Messages),
	)
	parseSpan.SetStatus(codes.Ok, "JSON parsing successful")
	parseSpan.End()

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

	// Create span for Loki push operation
	pushCtx, pushSpan := tracing.StartSpan(ctx, "flightdata.push_to_loki")
	pushSpan.SetAttributes(attribute.Int("loki.entries_count", len(entries)))

	if err := lokiClient.PushLogs(pushCtx, entries); err != nil {
		pushSpan.RecordError(err)
		pushSpan.SetStatus(codes.Error, err.Error())
		pushSpan.End()
		logging.Error("Failed to push logs to Loki", "error", err, "entries_count", len(entries))
		return fmt.Errorf("failed to push logs to Loki: %w", err)
	}

	pushSpan.SetStatus(codes.Ok, "Loki push successful")
	pushSpan.End()

	// Set final span attributes
	span.SetAttributes(
		attribute.Int("aircraft.processed", len(data.Aircraft)),
		attribute.Int("loki.entries_pushed", len(entries)),
	)

	logging.Info("Successfully fetched and pushed aircraft data", "aircraft_count", len(data.Aircraft), "entries_pushed", len(entries))
	return nil
}
