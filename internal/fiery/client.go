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
)

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
	payload := map[string]string{
		"username":     c.cfg.Username,
		"password":     c.cfg.Password,
		"accessrights": c.cfg.SecretKey,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return Session{}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+apiV5+"/login", bytes.NewReader(body))
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+apiV5+"/status", nil)
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
	if err := writer.WriteField("queue", "hold"); err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, err
	}
	if err := writer.Close(); err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+apiV5+"/jobs", &body)
	if err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, err
	}
	req.Header.Set("Cookie", session.Cookie)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return ImportResult{FilePath: filePath, Duration: time.Since(started)}, fmt.Errorf("import job request: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 8192))
	result := ImportResult{FilePath: filePath, StatusCode: resp.StatusCode, Duration: time.Since(started), RawBody: string(respBody), JobID: extractJobID(respBody)}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return result, fmt.Errorf("import failed with HTTP %d: %s", resp.StatusCode, strings.TrimSpace(result.RawBody))
	}
	return result, nil
}

func (c *Client) JobAction(ctx context.Context, session Session, jobID, action string) error {
	jobID = strings.TrimSpace(jobID)
	action = strings.Trim(strings.TrimSpace(action), "/")
	if jobID == "" || action == "" {
		return errors.New("job ID and action are required")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, c.baseURL+apiV5+"/jobs/"+url.PathEscape(jobID)+"/"+action, nil)
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
