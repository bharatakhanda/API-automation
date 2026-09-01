package fiery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func TestDiscoverCapabilitiesCapturesAllEndpointsInStableOrder(t *testing.T) {
	seen := map[string]bool{}
	var mu sync.Mutex
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Cookie") != "session=abc" {
			t.Errorf("missing session cookie")
		}
		mu.Lock()
		seen[r.URL.Path] = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	client, err := New(Config{ServerIP: srv.URL, SecretKey: "secret", Password: "password", InsecureTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	snapshot := client.DiscoverCapabilities(context.Background(), Session{Cookie: "session=abc"})
	if snapshot.APIVersion != "v5+v4" {
		t.Fatalf("api version = %q", snapshot.APIVersion)
	}
	if len(snapshot.Endpoints) != len(capabilityDiscoveryEndpoints) {
		t.Fatalf("endpoint count = %d, want %d", len(snapshot.Endpoints), len(capabilityDiscoveryEndpoints))
	}
	for index, endpoint := range capabilityDiscoveryEndpoints {
		if snapshot.Endpoints[index].Path != endpoint.Path {
			t.Fatalf("endpoint %d path = %q, want %q", index, snapshot.Endpoints[index].Path, endpoint.Path)
		}
		mu.Lock()
		wasSeen := seen[endpoint.Path]
		mu.Unlock()
		if !wasSeen {
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
