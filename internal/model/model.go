package model

import "time"

// Request describes one executable API request in a workflow.
type Request struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
	Timeout time.Duration     `json:"timeout,omitempty"`
}

// Workflow is an ordered set of API requests. Requests can be run sequentially
// or with controlled concurrency depending on RunnerOptions.
type Workflow struct {
	ID       string    `json:"id"`
	Name     string    `json:"name"`
	Requests []Request `json:"requests"`
}

// Result captures the observable outcome of a request execution.
type Result struct {
	RequestID   string        `json:"request_id"`
	RequestName string        `json:"request_name"`
	Method      string        `json:"method"`
	URL         string        `json:"url"`
	StatusCode  int           `json:"status_code"`
	Duration    time.Duration `json:"duration"`
	BodyPreview string        `json:"body_preview,omitempty"`
	Error       string        `json:"error,omitempty"`
	CompletedAt time.Time     `json:"completed_at"`
}
