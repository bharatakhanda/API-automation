package fiery

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	DefaultUsername = "admin"
	apiV5           = "/live/api/v5"
	apiV4           = "/live/api/v4"
)

// DefaultSecretKey is intentionally empty in source. For temporary field testing,
// it can be populated at build time with:
//
//	-ldflags "-X api-automation/internal/fiery.DefaultSecretKey=<key>"
//
// Do not commit real secrets to source control.
var DefaultSecretKey string

// Config describes the Fiery server connection learned from the reference DATA implementation.
type Config struct {
	ServerIP    string
	SecretKey   string
	Username    string
	Password    string
	InsecureTLS bool
}

// Client is a small, concurrency-safe Fiery API client.
type Client struct {
	baseURL string
	cfg     Config
	http    *http.Client
}

// Session contains the authenticated cookie required by Fiery API endpoints.
type Session struct {
	Cookie string
}

// ImportResult captures the server-side job created from a test file.
type ImportResult struct {
	FilePath   string
	JobID      string
	StatusCode int
	Duration   time.Duration
	RawBody    string
}

func New(cfg Config) (*Client, error) {
	cfg.ServerIP = strings.TrimSpace(cfg.ServerIP)
	cfg.SecretKey = strings.TrimSpace(cfg.SecretKey)
	if cfg.ServerIP == "" {
		return nil, errors.New("server IP address is required")
	}
	if cfg.SecretKey == "" {
		return nil, errors.New("secret key is required")
	}
	if cfg.Username == "" {
		cfg.Username = DefaultUsername
	}
	if cfg.Password == "" {
		return nil, errors.New("server password is required")
	}

	baseURL := cfg.ServerIP
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("parse server URL: %w", err)
	}
	if parsed.Host == "" {
		return nil, errors.New("server URL must include a host or IP address")
	}

	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 100
	transport.MaxIdleConnsPerHost = 25
	transport.IdleConnTimeout = 90 * time.Second
	if cfg.InsecureTLS {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} // Fiery installations often use self-signed certificates.
	}

	return &Client{
		baseURL: strings.TrimRight(parsed.String(), "/"),
		cfg:     cfg,
		http:    &http.Client{Transport: transport, Timeout: 60 * time.Second},
	}, nil
}

func (c *Client) Login(ctx context.Context) (Session, error) {
	session, err := c.login(ctx, apiV5)
	if err == nil {
		return session, nil
	}
	fallbackSession, fallbackErr := c.login(ctx, apiV4)
	if fallbackErr == nil {
		return fallbackSession, nil
	}
	return Session{}, fmt.Errorf("v5 login failed: %w; v4 login failed: %w", err, fallbackErr)
}

func (c *Client) login(ctx context.Context, apiPath string) (Session, error) {
	payload := map[string]string{
		"username":     c.cfg.Username,
		"password":     c.cfg.Password,
		"accessrights": c.cfg.SecretKey,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Session{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+apiPath+"/login", bytes.NewReader(body))
	if err != nil {
		return Session{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return Session{}, fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Session{}, fmt.Errorf("login failed with HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	cookies := resp.Cookies()
	if len(cookies) == 0 {
		return Session{}, errors.New("login succeeded but no session cookie was returned")
	}
	return Session{Cookie: cookies[0].String()}, nil
}

func (c *Client) KeepAlive(ctx context.Context, session Session) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+apiV5+"/info", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", session.Cookie)
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("keep-alive request: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("keep-alive failed with HTTP %d", resp.StatusCode)
	}
	return nil
}

func (c *Client) ImportJob(ctx context.Context, session Session, filePath string) (ImportResult, error) {
	return c.ImportJobToQueue(ctx, session, filePath, "hold")
}

func (c *Client) ImportJobToQueue(ctx context.Context, session Session, filePath, queue string) (ImportResult, error) {
	result, err := c.importJobToQueue(ctx, session, filePath, queue, apiV5)
	if err == nil {
		return result, nil
	}
	fallback, fallbackErr := c.importJobToQueue(ctx, session, filePath, queue, apiV4)
	if fallbackErr == nil {
		return fallback, nil
	}
	return result, fmt.Errorf("v5 import failed: %w; v4 import failed: %w", err, fallbackErr)
}

func (c *Client) importJobToQueue(ctx context.Context, session Session, filePath, queue, apiPath string) (ImportResult, error) {
	started := time.Now()
	file, err := os.Open(filePath)
	if err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, err
	}
	defer file.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	part, err := writer.CreateFormFile("file", filepath.Base(filePath))
	if err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, err
	}
	if _, err := io.Copy(part, file); err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, err
	}
	queue = strings.TrimSpace(queue)
	if queue == "" {
		queue = "hold"
	}
	if err := writer.WriteField("queue", queue); err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, err
	}
	if err := writer.Close(); err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, err
	}

	endpoint := c.baseURL + apiPath + "/jobs"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, err
	}
	req.Header.Set("Cookie", session.Cookie)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, fmt.Errorf("import job request %s queue=%q file=%q: %w", apiPath, queue, filepath.Base(filePath), err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	result := ImportResult{FilePath: filePath, StatusCode: resp.StatusCode, Duration: time.Since(started), RawBody: string(respBody), JobID: extractJobID(respBody)}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("import failed at %s with HTTP %d queue=%q file=%q: %s", apiPath+"/jobs", resp.StatusCode, queue, filepath.Base(filePath), strings.TrimSpace(result.RawBody))
	}
	return result, nil
}

func (c *Client) GetJobAttributes(ctx context.Context, session Session, jobID string) (map[string]string, error) {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return nil, errors.New("job ID is required")
	}
	merged := map[string]string{}
	var failures []string
	for _, apiPath := range []string{apiV4, apiV5} {
		for _, suffix := range []string{"", "/attributes", "/properties"} {
			attrs, err := c.getJobAttributesAt(ctx, session, apiPath, jobID, suffix)
			if err != nil {
				failures = append(failures, err.Error())
				continue
			}
			for key, value := range attrs {
				if _, exists := merged[key]; !exists || merged[key] == "" {
					merged[key] = value
				}
			}
		}
	}
	if len(merged) == 0 {
		return nil, fmt.Errorf("job response did not contain readable attributes; attempts: %s", strings.Join(failures, " | "))
	}
	return merged, nil
}

func (c *Client) getJobAttributesAt(ctx context.Context, session Session, apiPath, jobID, suffix string) (map[string]string, error) {
	endpoint := c.baseURL + apiPath + "/jobs/" + url.PathEscape(jobID) + suffix
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", session.Cookie)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("get job %s/jobs/%s%s failed with HTTP %d: %s", apiPath, jobID, suffix, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	attrs := extractJobAttributes(body)
	if len(attrs) == 0 {
		return nil, fmt.Errorf("job response from %s/jobs/%s%s did not contain readable attributes; body=%s", apiPath, jobID, suffix, truncateForError(body, 2048))
	}
	return attrs, nil
}

func (c *Client) UpdateJobAttributes(ctx context.Context, session Session, jobID string, attributes map[string]string) error {
	if err := c.updateJobAttributes(ctx, session, apiV4, jobID, attributes); err == nil {
		return nil
	} else if fallbackErr := c.updateJobAttributes(ctx, session, apiV5, jobID, attributes); fallbackErr != nil {
		return fmt.Errorf("v4 job attribute update failed: %w; v5 job attribute update failed: %w", err, fallbackErr)
	}
	return nil
}

func (c *Client) updateJobAttributes(ctx context.Context, session Session, apiPath, jobID string, attributes map[string]string) error {
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return errors.New("job ID is required")
	}
	if len(attributes) == 0 {
		return nil
	}
	payload, err := json.Marshal(map[string]map[string]string{"attributes": attributes})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+apiPath+"/jobs/"+url.PathEscape(jobID), bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", session.Cookie)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("job attribute update %s/jobs/%s failed with HTTP %d: %s", apiPath, jobID, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return nil
}

func (c *Client) WaitJobAttribute(ctx context.Context, session Session, jobID, key, want string, timeout, interval time.Duration) error {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		attrs, err := c.GetJobAttributes(ctx, session, jobID)
		if err == nil && strings.EqualFold(strings.TrimSpace(attrs[key]), strings.TrimSpace(want)) {
			return nil
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return fmt.Errorf("wait for job %s %s=%q timed out after %s; last GET error: %w", jobID, key, want, timeout, err)
			}
			return fmt.Errorf("wait for job %s %s=%q timed out after %s", jobID, key, want, timeout)
		case <-ticker.C:
		}
	}
}

func (c *Client) JobAction(ctx context.Context, session Session, jobID, action string) error {
	if err := c.jobAction(ctx, session, apiV4, jobID, action); err == nil {
		return nil
	} else if fallbackErr := c.jobAction(ctx, session, apiV5, jobID, action); fallbackErr != nil {
		return fmt.Errorf("v4 job action failed: %w; v5 job action failed: %w", err, fallbackErr)
	}
	return nil
}

func (c *Client) jobAction(ctx context.Context, session Session, apiPath, jobID, action string) error {
	jobID = strings.TrimSpace(jobID)
	action = strings.Trim(strings.TrimSpace(action), "/")
	if jobID == "" || action == "" {
		return errors.New("job ID and action are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+apiPath+"/jobs/"+url.PathEscape(jobID)+"/"+action, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Cookie", session.Cookie)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("job action %q failed with HTTP %d", action, resp.StatusCode)
	}
	return nil
}

func extractJobAttributes(body []byte) map[string]string {
	var payload struct {
		Data struct {
			Item  map[string]any   `json:"item"`
			Items []map[string]any `json:"items"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}

	item := payload.Data.Item
	if len(item) == 0 && len(payload.Data.Items) > 0 {
		item = payload.Data.Items[0]
	}
	if len(item) == 0 {
		return nil
	}

	attrs := make(map[string]string, len(item))
	collectJobAttributes(attrs, item)
	if len(attrs) == 0 {
		return nil
	}
	return attrs
}

func collectJobAttributes(out map[string]string, item map[string]any) {
	collectAttributeTree(out, item)
}

func collectAttributeTree(out map[string]string, value any) {
	switch typed := value.(type) {
	case map[string]any:
		for key, raw := range typed {
			if key == "job" || key == "attributes" {
				collectAttributeTree(out, raw)
				continue
			}
			if scalarAttribute(raw) {
				out[key] = normalizeAttributeValue(raw)
				continue
			}
			if wrappedAttributeValue(raw) {
				out[key] = normalizeAttributeValue(raw)
				continue
			}
			collectAttributeTree(out, raw)
		}
	case []any:
		for _, item := range typed {
			collectAttributeTree(out, item)
		}
	}
}

func scalarAttribute(value any) bool {
	switch value.(type) {
	case nil, string, bool, float64, int, int64, json.Number:
		return true
	default:
		return false
	}
}

func wrappedAttributeValue(value any) bool {
	m, ok := value.(map[string]any)
	if !ok {
		return false
	}
	_, hasValue := m["value"]
	_, hasName := m["name"]
	_, hasID := m["id"]
	return hasValue || hasName || hasID
}

func collectAnyMap(out map[string]string, value any) {
	collectAttributeTree(out, value)
}

func asStringAnyMap(value any) (map[string]any, bool) {
	if typed, ok := value.(map[string]any); ok {
		return typed, true
	}
	return nil, false
}

func truncateForError(body []byte, max int) string {
	text := strings.TrimSpace(string(body))
	if len(text) <= max {
		return text
	}
	return text[:max] + "..."
}

func normalizeAttributeValue(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(typed)
	case fmt.Stringer:
		return strings.TrimSpace(typed.String())
	case map[string]any:
		for _, key := range []string{"value", "name", "id", "status"} {
			if nested, ok := typed[key]; ok {
				return normalizeAttributeValue(nested)
			}
		}
		return ""
	default:
		return strings.TrimSpace(fmt.Sprint(typed))
	}
}

func extractJobID(body []byte) string {
	var payload struct {
		Data struct {
			Item struct {
				ID string `json:"id"`
			} `json:"item"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return ""
	}
	return payload.Data.Item.ID
}
