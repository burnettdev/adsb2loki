package loki

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"

	"github.com/burnettdev/adsb2loki/pkg/logging"
	"github.com/burnettdev/adsb2loki/pkg/tracing"
)

type Client struct {
	url      string
	client   *http.Client
	tenantID string
	password string
}

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

type LogEntry struct {
	Timestamp time.Time
	Labels    map[string]string
	Line      string
}

func (c *Client) PushLogs(ctx context.Context, entries []LogEntry) error {
	ctx, span := tracing.StartSpan(ctx, "loki.PushLogs")
	defer span.End()

	span.SetAttributes(
		attribute.Int("loki.entries_count", len(entries)),
		attribute.String("loki.url", c.url),
		attribute.Bool("loki.auth_enabled", c.tenantID != "" && c.password != ""),
	)

	logging.DebugCall("PushLogs", "entries_count", len(entries))

	if len(entries) == 0 {
		logging.Debug("No entries to push, skipping")
		span.SetAttributes(attribute.String("loki.result", "skipped_empty"))
		return nil
	}

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

	// Create span for JSON marshaling
	_, marshalSpan := tracing.StartSpan(ctx, "loki.marshal_payload")

	data, err := json.Marshal(payload)
	if err != nil {
		marshalSpan.RecordError(err)
		marshalSpan.SetStatus(codes.Error, err.Error())
		marshalSpan.End()
		logging.Error("Failed to marshal Loki payload", "error", err, "entries_count", len(entries))
		return fmt.Errorf("failed to marshal payload: %w", err)
	}

	marshalSpan.SetAttributes(
		attribute.Int("loki.payload_size", len(data)),
		attribute.Int("loki.streams_count", len(streams)),
	)
	marshalSpan.SetStatus(codes.Ok, "Payload marshaled successfully")
	marshalSpan.End()

	logging.Debug("Loki payload marshaled", "payload_size", len(data), "streams_count", len(streams))

	url := c.url + "/loki/api/v1/push"

	// Create span for HTTP request
	httpCtx, httpSpan := tracing.StartSpan(ctx, "loki.http_request")
	httpSpan.SetAttributes(
		attribute.String("http.method", "POST"),
		attribute.String("http.url", url),
		attribute.Int("http.request_size", len(data)),
	)

	req, err := http.NewRequestWithContext(httpCtx, "POST", url, bytes.NewReader(data))
	if err != nil {
		httpSpan.RecordError(err)
		httpSpan.SetStatus(codes.Error, err.Error())
		httpSpan.End()
		logging.Error("Failed to create HTTP request", "error", err, "url", url)
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	if c.tenantID != "" && c.password != "" {
		req.SetBasicAuth(c.tenantID, c.password)
		httpSpan.SetAttributes(attribute.String("http.auth_type", "basic"))
		logging.Debug("Added basic authentication to request", "tenant_id", c.tenantID)
	}

	start := time.Now()
	resp, err := c.client.Do(req)
	duration := time.Since(start)

	httpSpan.SetAttributes(attribute.Int64("http.duration_ms", duration.Milliseconds()))

	if err != nil {
		httpSpan.RecordError(err)
		httpSpan.SetStatus(codes.Error, err.Error())
		httpSpan.End()
		logging.Error("HTTP request failed", "error", err, "url", url, "duration_ms", duration.Milliseconds())
		return fmt.Errorf("failed to send request: %w", err)
	}
	defer resp.Body.Close()

	httpSpan.SetAttributes(attribute.Int("http.status_code", resp.StatusCode))
	logging.DebugHTTP("POST", url, resp.StatusCode, duration, "entries_count", len(entries))

	if resp.StatusCode == http.StatusUnauthorized {
		httpSpan.SetStatus(codes.Error, "Authentication failed")
		httpSpan.End()
		logging.Error("Authentication failed", "status", resp.Status, "tenant_id", c.tenantID)
		return fmt.Errorf("authentication failed: %s", resp.Status)
	}

	if resp.StatusCode >= 400 {
		httpSpan.SetStatus(codes.Error, "HTTP request failed")
		httpSpan.End()
		logging.Error("HTTP request failed with bad status", "status", resp.Status, "status_code", resp.StatusCode)
		return fmt.Errorf("request failed with status: %s", resp.Status)
	}

	httpSpan.SetStatus(codes.Ok, "HTTP request successful")
	httpSpan.End()

	// Set final span attributes
	span.SetAttributes(
		attribute.Int("loki.http_status_code", resp.StatusCode),
		attribute.String("loki.result", "success"),
	)

	logging.Debug("Successfully pushed logs to Loki", "entries_count", len(entries), "status_code", resp.StatusCode)
	return nil
}
