package loki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/burnettdev/adsb2loki/pkg/logging"
)

// Client represents a Loki client
type Client struct {
	url      string
	client   *http.Client
	tenantID string
	password string
}

// NewClient creates a new Loki client
func NewClient(url string) *Client {
	logging.DebugCall("NewClient", "url", url)

	client := &Client{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	logging.Debug("Loki client created", "url", url, "timeout", "10s", "auth", false)
	return client
}

// NewClientWithAuth creates a new Loki client with Grafana Cloud authentication
func NewClientWithAuth(url, tenantID, password string) *Client {
	logging.DebugCall("NewClientWithAuth", "url", url, "tenant_id", tenantID, "password_set", password != "")

	client := &Client{
		url:      url,
		tenantID: tenantID,
		password: password,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}

	logging.Debug("Loki client created with auth", "url", url, "tenant_id", tenantID, "timeout", "10s", "auth", true)
	return client
}

// LogEntry represents a single log entry to be sent to Loki
type LogEntry struct {
	Timestamp time.Time
	Labels    map[string]string
	Line      string
}

// PushLogs sends log entries to Loki
func (c *Client) PushLogs(ctx context.Context, entries []LogEntry) error {
	logging.DebugCall("PushLogs", "entries_count", len(entries))

	if len(entries) == 0 {
		logging.Debug("No entries to push, skipping")
		return nil
	}

	// Create the request payload
	streams := make([]map[string]interface{}, 0)
	for _, entry := range entries {
		stream := map[string]interface{}{
			"stream": entry.Labels,
			"values": [][]string{
				{fmt.Sprintf("%d", entry.Timestamp.UnixNano()), entry.Line},
			},
		}
		streams = append(streams, stream)
	}

	payload := map[string]interface{}{
		"streams": streams,
	}

	// Marshal the payload
	data, err := json.Marshal(payload)
	if err != nil {
		logging.Error("Failed to marshal Loki payload", "error", err, "entries_count", len(entries))
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	logging.Debug("Loki payload marshaled", "payload_size", len(data), "streams_count", len(streams))

	// Create the request
	url := c.url + "/loki/api/v1/push"
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewReader(data))
	if err != nil {
		logging.Error("Failed to create HTTP request", "error", err, "url", url)
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Add authentication for Grafana Cloud if configured
	if c.tenantID != "" && c.password != "" {
		req.SetBasicAuth(c.tenantID, c.password)
		logging.Debug("Added basic authentication to request", "tenant_id", c.tenantID)
	}

	// Send the request
	start := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(start)

	if err != nil {
		logging.Error("HTTP request failed", "error", err, "url", url, "duration_ms", duration.Milliseconds())
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	// Log the HTTP response
	logging.DebugHTTP("POST", url, resp.StatusCode, duration, "entries_count", len(entries))

	// Check for authentication errors
	if resp.StatusCode == http.StatusUnauthorized {
		logging.Error("Authentication failed", "status", resp.Status, "tenant_id", c.tenantID)
		return fmt.Errorf("authentication failed: %s", resp.Status)
	}

	if resp.StatusCode >= 400 {
		logging.Error("HTTP request failed with bad status", "status", resp.Status, "status_code", resp.StatusCode)
		return fmt.Errorf("request failed with status: %s", resp.Status)
	}

	logging.Debug("Successfully pushed logs to Loki", "entries_count", len(entries), "status_code", resp.StatusCode)
	return nil
}
