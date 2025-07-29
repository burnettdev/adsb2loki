package flightdata

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/burnettdev/adsb2loki/pkg/logging"
	"github.com/burnettdev/adsb2loki/pkg/loki"
	"github.com/burnettdev/adsb2loki/pkg/models"
)

func FetchAndPushToLoki(ctx context.Context, lokiClient *loki.Client) error {
	logging.DebugCall("FetchAndPushToLoki")

	flightDataURL := os.Getenv("FLIGHT_DATA_URL")
	logging.Debug("Flight data URL configured", "url", flightDataURL)

	start := time.Now()
	resp, err := http.Get(flightDataURL)
	duration := time.Since(start)

	if err != nil {
		logging.Error("Failed to fetch dump1090-fa data", "error", err, "url", flightDataURL, "duration_ms", duration.Milliseconds())
		return fmt.Errorf("failed to fetch dump1090-fa data: %w", err)
	}
	defer resp.Body.Close()

	logging.DebugHTTP("GET", flightDataURL, resp.StatusCode, duration)

	if resp.StatusCode != http.StatusOK {
		logging.Error("HTTP request returned non-200 status", "status_code", resp.StatusCode, "status", resp.Status)
		return fmt.Errorf("HTTP request failed with status: %s", resp.Status)
	}

	var data models.Dump1090fa
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		logging.Error("Failed to decode dump1090-fa data", "error", err)
		return fmt.Errorf("failed to decode dump1090-fa data: %w", err)
	}

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
		logging.Error("Failed to push logs to Loki", "error", err, "entries_count", len(entries))
		return fmt.Errorf("failed to push logs to Loki: %w", err)
	}

	logging.Info("Successfully fetched and pushed aircraft data", "aircraft_count", len(data.Aircraft), "entries_pushed", len(entries))
	return nil
}
