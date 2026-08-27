package fiery

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var V5DiscoveryEndpoints = []DiscoveryEndpoint{
	{Name: "properties", Method: http.MethodGet, Path: apiV5 + "/properties"},
	{Name: "jobs", Method: http.MethodGet, Path: apiV5 + "/jobs"},
	{Name: "queues", Method: http.MethodGet, Path: apiV5 + "/queues"},
	{Name: "printers", Method: http.MethodGet, Path: apiV5 + "/printers"},
	{Name: "device", Method: http.MethodGet, Path: apiV5 + "/device"},
	{Name: "server", Method: http.MethodGet, Path: apiV5 + "/server"},
	{Name: "system", Method: http.MethodGet, Path: apiV5 + "/system"},
	{Name: "status", Method: http.MethodGet, Path: apiV5 + "/status"},
}

type DiscoveryEndpoint struct {
	Name   string `json:"name"`
	Method string `json:"method"`
	Path   string `json:"path"`
}

type CapabilitySnapshot struct {
	CapturedAt time.Time          `json:"capturedAt"`
	Server     string             `json:"server"`
	APIVersion string             `json:"apiVersion"`
	Endpoints  []EndpointSnapshot `json:"endpoints"`
}

type EndpointSnapshot struct {
	Name       string          `json:"name"`
	Method     string          `json:"method"`
	Path       string          `json:"path"`
	StatusCode int             `json:"statusCode,omitempty"`
	DurationMS int64           `json:"durationMs"`
	Body       json.RawMessage `json:"body,omitempty"`
	RawBody    string          `json:"rawBody,omitempty"`
	Error      string          `json:"error,omitempty"`
}

func (c *Client) DiscoverV5(ctx context.Context, session Session) CapabilitySnapshot {
	snapshot := CapabilitySnapshot{
		CapturedAt: time.Now().UTC(),
		Server:     c.baseURL,
		APIVersion: "v5",
		Endpoints:  make([]EndpointSnapshot, 0, len(V5DiscoveryEndpoints)),
	}
	for _, endpoint := range V5DiscoveryEndpoints {
		snapshot.Endpoints = append(snapshot.Endpoints, c.discoverEndpoint(ctx, session, endpoint))
	}
	return snapshot
}

func (c *Client) SaveCapabilitySnapshot(snapshot CapabilitySnapshot, dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("capture directory is required")
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	fileName := "server-capabilities-snapshot-" + snapshot.CapturedAt.Format("20060102-150405") + ".json"
	path := filepath.Join(dir, fileName)
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (c *Client) discoverEndpoint(ctx context.Context, session Session, endpoint DiscoveryEndpoint) EndpointSnapshot {
	started := time.Now()
	result := EndpointSnapshot{Name: endpoint.Name, Method: endpoint.Method, Path: endpoint.Path}
	req, err := http.NewRequestWithContext(ctx, endpoint.Method, c.baseURL+endpoint.Path, nil)
	if err != nil {
		result.Error = err.Error()
		result.DurationMS = time.Since(started).Milliseconds()
		return result
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Cookie", session.Cookie)

	resp, err := c.http.Do(req)
	result.DurationMS = time.Since(started).Milliseconds()
	if err != nil {
		result.Error = err.Error()
		return result
	}
	defer resp.Body.Close()
	result.StatusCode = resp.StatusCode
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	trimmed := strings.TrimSpace(string(body))
	if trimmed == "" {
		return result
	}
	if json.Valid(body) {
		result.Body = json.RawMessage(body)
		return result
	}
	result.RawBody = trimmed
	return result
}
