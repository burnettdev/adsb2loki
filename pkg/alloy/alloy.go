package alloy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client represents a Loki client
type Client struct {
	url    string
	client *http.Client
}

// NewClient creates a new Loki client
func NewClient(url string) *Client {
	return &Client{
		url: url,
		client: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

// LogEntry represents a single log entry to be sent to Loki
type LogEntry struct {
	Timestamp time.Time
	Labels    map[string]string
	Line      string
}

// PushLogs sends log entries to Loki
func (c *Client) PushLogs(ctx context.Context, entries []LogEntry) error {
	if len(entries) == 0 {
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
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	// Create the request
	req, err := http.NewRequestWithContext(ctx, "POST", c.url+"/loki/api/v1/push", bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	// Send the request
	resp, err := c.client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	return nil
}
