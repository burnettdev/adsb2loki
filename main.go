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
	"github.com/joho/godotenv"
)

func main() {
	// Initialize logger first
	logging.Init()
	logger := logging.Get()

	logger.DebugCall("main")

	// Load .env file
	if err := godotenv.Load(); err != nil {
		logger.Warn("Environment file not found", "error", err)
	} else {
		logger.Debug("Environment file loaded successfully")
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Loki client with optional Grafana Cloud authentication
	lokiURL := os.Getenv("LOKI_URL")
	logger.Debug("Loki URL configuration", "url", lokiURL)

	var lokiClient *loki.Client

	// Check for Grafana Cloud authentication credentials
	tenantID := os.Getenv("GRAFANA_TENANT_ID")
	password := os.Getenv("GRAFANA_PASSWORD")

	if tenantID != "" && password != "" {
		// Use Grafana Cloud authentication
		logger.Info("Using Grafana Cloud authentication", "tenant_id", tenantID)
		logger.Debug("Grafana Cloud credentials found", "tenant_id", tenantID, "password_set", password != "")
		lokiClient = loki.NewClientWithAuth(lokiURL, tenantID, password)
	} else {
		// No authentication (for local Grafana instances)
		logger.Info("No authentication configured - using local Grafana instance mode")
		lokiClient = loki.NewClient(lokiURL)
	}

	// Create a ticker to fetch data periodically
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	logger.Info("Starting data fetch loop", "interval", "5s")

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the main loop
	logger.Info("Application started successfully")
	for {
		select {
		case <-ticker.C:
			logger.Debug("Ticker fired - fetching data")
			if err := flightdata.FetchAndPushToLoki(ctx, lokiClient); err != nil {
				logger.Error("Error fetching and pushing data", "error", err)
			} else {
				logger.Debug("Data fetch and push completed successfully")
			}
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

// getEnvOrDefault returns the value of the environment variable or a default value
func getEnvOrDefault(key, defaultValue string) string {
	logging.DebugCall("getEnvOrDefault", "key", key, "default", defaultValue)

	if value, exists := os.LookupEnv(key); exists {
		logging.Debug("Environment variable found", "key", key, "value", value)
		return value
	}

	logging.Debug("Environment variable not found, using default", "key", key, "default", defaultValue)
	return defaultValue
}
