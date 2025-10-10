package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/burnettdev/adsb2loki/pkg/flightdata"
	"github.com/burnettdev/adsb2loki/pkg/logging"
	"github.com/burnettdev/adsb2loki/pkg/loki"
	"github.com/burnettdev/adsb2loki/pkg/tracing"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file before initializing logger so LOG_LEVEL is available
	envErr := godotenv.Load()

	logging.Init()
	logger := logging.Get()

	logger.DebugCall("main")

	if envErr != nil {
		logger.Debug("Environment file not found (this is normal in production)", "error", envErr)
	} else {
		logger.Debug("Environment file loaded successfully")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize OpenTelemetry tracing
	tracingShutdown, err := tracing.InitTracing(ctx)
	if err != nil {
		logger.Error("Failed to initialize OpenTelemetry tracing", "error", err)
		// Continue without tracing rather than failing
	} else {
		defer func() {
			shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer shutdownCancel()
			if err := tracingShutdown(shutdownCtx); err != nil {
				logger.Error("Failed to shutdown OpenTelemetry tracing", "error", err)
			} else {
				logger.Debug("OpenTelemetry tracing shutdown successfully")
			}
		}()
	}

	lokiURL := os.Getenv("LOKI_URL")
	logger.Debug("Loki URL configuration", "url", lokiURL)

	var lokiClient *loki.Client

	tenantID := os.Getenv("GRAFANA_TENANT_ID")
	password := os.Getenv("GRAFANA_PASSWORD")

	if tenantID != "" && password != "" {
		logger.Info("Using Grafana Cloud authentication", "tenant_id", tenantID)
		logger.Debug("Grafana Cloud credentials found", "tenant_id", tenantID, "password_set", password != "")
		lokiClient = loki.NewClientWithAuth(lokiURL, tenantID, password)
	} else {
		logger.Info("No authentication configured - using local Grafana instance mode")
		lokiClient = loki.NewClient(lokiURL)
	}

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	logger.Info("Starting data fetch loop", "interval", "5s")

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	logger.Info("Application started successfully")
	for {
		select {
		case <-ticker.C:
			logger.Debug("Ticker fired - fetching data")

			// Create a span for the entire fetch and push operation
			fetchCtx, span := tracing.StartSpan(ctx, "adsb2loki.fetch_cycle")

			if err := flightdata.FetchAndPushToLoki(fetchCtx, lokiClient); err != nil {
				logger.Error("Error fetching and pushing data", "error", err)
				tracing.SetSpanError(fetchCtx, err)
			} else {
				logger.Debug("Data fetch and push completed successfully")
			}

			span.End()
		case sig := <-sigChan:
			logger.Info("Received shutdown signal", "signal", sig)
			logger.Debug("Graceful shutdown initiated")
			return
		case <-ctx.Done():
			logger.Debug("Context cancelled")
			return
		}
	}
}

func getEnvOrDefault(key, defaultValue string) string {
	logging.DebugCall("getEnvOrDefault", "key", key, "default", defaultValue)

	if value, exists := os.LookupEnv(key); exists {
		logging.Debug("Environment variable found", "key", key, "value", value)
		return value
	}

	logging.Debug("Environment variable not found, using default", "key", key, "default", defaultValue)
	return defaultValue
}
