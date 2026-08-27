package engine

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"api-automation/internal/model"
)

const defaultTimeout = 30 * time.Second

// Executor executes individual API requests. It is safe for concurrent use.
type Executor struct {
	client *http.Client
}

func NewExecutor() *Executor {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConns = 200
	transport.MaxIdleConnsPerHost = 50
	transport.MaxConnsPerHost = 100
	transport.IdleConnTimeout = 90 * time.Second
	transport.ResponseHeaderTimeout = 30 * time.Second

	return &Executor{client: &http.Client{Transport: transport, Timeout: defaultTimeout}}
}

func (e *Executor) Do(ctx context.Context, req model.Request) model.Result {
	started := time.Now()
	result := model.Result{
		RequestID:   req.ID,
		RequestName: req.Name,
		Method:      strings.ToUpper(req.Method),
		URL:         req.URL,
		CompletedAt: time.Now(),
	}

	if req.URL == "" {
		result.Error = "url is required"
		return result
	}
	if result.Method == "" {
		result.Method = http.MethodGet
	}

	if req.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, req.Timeout)
		defer cancel()
	}

	httpReq, err := http.NewRequestWithContext(ctx, result.Method, req.URL, bytes.NewBufferString(req.Body))
	if err != nil {
		result.Error = err.Error()
		return result
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if req.Body != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := e.client.Do(httpReq)
	result.Duration = time.Since(started)
	result.CompletedAt = time.Now()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			result.Error = "request timed out"
		} else {
			result.Error = err.Error()
		}
		return result
	}
	defer resp.Body.Close()

	result.StatusCode = resp.StatusCode
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 4096))
	if readErr != nil {
		result.Error = fmt.Sprintf("read response: %v", readErr)
		return result
	}
	result.BodyPreview = string(body)
	return result
}
