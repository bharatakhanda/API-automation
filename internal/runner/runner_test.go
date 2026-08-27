package runner

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"api-automation/internal/engine"
	"api-automation/internal/model"
)

func TestRunnerRunExecutesWorkflow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	wf := model.Workflow{Requests: []model.Request{
		{ID: "1", Name: "one", Method: http.MethodGet, URL: srv.URL, Timeout: 5 * time.Second},
		{ID: "2", Name: "two", Method: http.MethodGet, URL: srv.URL, Timeout: 5 * time.Second},
	}}

	r := New(engine.NewExecutor())
	results := r.Run(context.Background(), wf, Options{Concurrency: 2})

	count := 0
	for res := range results {
		count++
		if res.Error != "" {
			t.Fatalf("unexpected error: %s", res.Error)
		}
		if res.StatusCode != http.StatusAccepted {
			t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusAccepted)
		}
	}
	if count != 2 {
		t.Fatalf("count = %d, want 2", count)
	}
}
