package fiery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverV5CapturesOnlyV5Endpoints(t *testing.T) {
	seen := map[string]bool{}
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "session=abc" {
			t.Fatalf("missing session cookie")
		}
		if len(r.URL.Path) < len(apiV5) || r.URL.Path[:len(apiV5)] != apiV5 {
			t.Fatalf("non-v5 endpoint requested: %s", r.URL.Path)
		}
		seen[r.URL.Path] = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client, err := New(Config{ServerIP: srv.URL, SecretKey: "secret", Password: "password", InsecureTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := client.DiscoverV5(context.Background(), Session{Cookie: "session=abc"})
	if snapshot.APIVersion != "v5" {
		t.Fatalf("api version = %q", snapshot.APIVersion)
	}
	if len(snapshot.Endpoints) != len(V5DiscoveryEndpoints) {
		t.Fatalf("endpoint count = %d, want %d", len(snapshot.Endpoints), len(V5DiscoveryEndpoints))
	}
	for _, endpoint := range V5DiscoveryEndpoints {
		if !seen[endpoint.Path] {
			t.Fatalf("endpoint not requested: %s", endpoint.Path)
		}
	}
}

func TestSaveCapabilitySnapshot(t *testing.T) {
	client, err := New(Config{ServerIP: "https://127.0.0.1", SecretKey: "secret", Password: "password", InsecureTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := CapabilitySnapshot{APIVersion: "v5"}
	path, err := client.SaveCapabilitySnapshot(snapshot, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(path) == "" {
		t.Fatalf("invalid path: %q", path)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatal(err)
	}
}
