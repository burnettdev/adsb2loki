package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/burnettdev/flightaware2loki/pkg/alloy"
	"github.com/burnettdev/flightaware2loki/pkg/flightaware"
	"github.com/joho/godotenv"
)

func main() {
	// Load .env file
	if err := godotenv.Load(); err != nil {
		log.Printf("Warning: .env file not found: %v", err)
	}

	// Create a context that we can cancel
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Initialize Loki client
	alloyURL := os.Getenv("ALLOY_URL")
	alloyClient := alloy.NewClient(alloyURL)

	// Create a ticker to fetch data periodically
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	// Handle graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// Start the main loop
	for {
		select {
		case <-ticker.C:
			if err := flightaware.FetchAndPushToLoki(ctx, alloyClient); err != nil {
				log.Printf("Error fetching and pushing data: %v", err)
			}
		case <-sigChan:
			log.Println("Received shutdown signal, exiting...")
			return
		case <-ctx.Done():
			return
		}
	}
}

// getEnvOrDefault returns the value of the environment variable or a default value
func getEnvOrDefault(key, defaultValue string) string {
	if value, exists := os.LookupEnv(key); exists {
		return value
	}
	return defaultValue
}
