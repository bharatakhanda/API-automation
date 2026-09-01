package fiery

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestImportJobFallsBackToV4(t *testing.T) {
	var sawV4Import bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiV5 + "/login":
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc"})
			_, _ = w.Write([]byte(`{"data":{"item":{"authenticated":true}}}`))
		case apiV5 + "/jobs":
			http.Error(w, `{"error":{"code":400,"message":"Bad Request"}}`, http.StatusBadRequest)
		case apiV4 + "/jobs":
			sawV4Import = true
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				t.Fatal(err)
			}
			if got := r.FormValue("queue"); got != "hold" {
				t.Fatalf("queue = %q", got)
			}
			_, _ = w.Write([]byte(`{"data":{"item":{"id":"JOB-V4"}}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client, err := New(Config{ServerIP: srv.URL, SecretKey: "secret", Password: "password", InsecureTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "sample.pdf")
	if err := os.WriteFile(file, []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := client.ImportJob(context.Background(), Session{Cookie: "session=abc"}, file)
	if err != nil {
		t.Fatal(err)
	}
	if result.JobID != "JOB-V4" || !sawV4Import {
		t.Fatalf("fallback result=%#v sawV4Import=%v", result, sawV4Import)
	}
}

func TestCancelAndDeleteJobUseFieryJobRoutes(t *testing.T) {
	var sawCancel, sawDelete bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == apiV4+"/jobs/JOB-123/cancel":
			sawCancel = true
		case r.Method == http.MethodDelete && r.URL.Path == apiV4+"/jobs/JOB-123":
			sawDelete = true
		default:
			t.Fatalf("unexpected job operation: %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Cookie"); got != "session=abc" {
			t.Fatalf("cookie = %q", got)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()
	client, err := New(Config{ServerIP: srv.URL, SecretKey: "secret", Password: "password", InsecureTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	session := Session{Cookie: "session=abc"}
	if err := client.CancelJob(context.Background(), session, "JOB-123"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteJob(context.Background(), session, "JOB-123"); err != nil {
		t.Fatal(err)
	}
	if !sawCancel || !sawDelete {
		t.Fatalf("sawCancel=%v sawDelete=%v", sawCancel, sawDelete)
	}
}

func TestDeleteJobFallsBackToV5(t *testing.T) {
	var sawV5 bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == apiV4+"/jobs/JOB-123" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodDelete || r.URL.Path != apiV5+"/jobs/JOB-123" {
			t.Fatalf("unexpected job operation: %s %s", r.Method, r.URL.Path)
		}
		sawV5 = true
	}))
	defer srv.Close()
	client, err := New(Config{ServerIP: srv.URL, SecretKey: "secret", Password: "password", InsecureTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteJob(context.Background(), Session{Cookie: "session=abc"}, "JOB-123"); err != nil {
		t.Fatal(err)
	}
	if !sawV5 {
		t.Fatal("v5 fallback was not called")
	}
}

func TestNewRejectsUnsafeOrMalformedServerURLs(t *testing.T) {
	for _, server := range []string{
		"ftp://server.example",
		"https://user:password@server.example",
		"https://server.example/unexpected/path",
		"https://server.example?query=value",
	} {
		t.Run(server, func(t *testing.T) {
			if _, err := New(Config{ServerIP: server, SecretKey: "secret", Password: "password"}); err == nil {
				t.Fatalf("New accepted invalid server URL %q", server)
			}
		})
	}
}

func TestNewNormalizesServerURL(t *testing.T) {
	client, err := New(Config{ServerIP: "HTTPS://server.example/", SecretKey: "secret", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	if client.baseURL != "https://server.example" {
		t.Fatalf("base URL = %q", client.baseURL)
	}
}

func TestHeaderSnapshotRedactsCredentialHeaders(t *testing.T) {
	snapshot := headerSnapshot(http.Header{
		"Set-Cookie":    []string{"session=sensitive; Path=/"},
		"Authorization": []string{"Bearer sensitive"},
		"Content-Type":  []string{"application/json"},
	})
	if snapshot["Set-Cookie"] != "<redacted>" || snapshot["Authorization"] != "<redacted>" {
		t.Fatalf("credential headers were not redacted: %#v", snapshot)
	}
	if snapshot["Content-Type"] != "application/json" {
		t.Fatalf("non-sensitive header missing: %#v", snapshot)
	}
}

func TestCheckJobConstraintsParsesConflictsAndSolutions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/live/api/v5/jobs/JOB-1/constraint" || r.Method != http.MethodPost {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"conflict":{"EFResolution":"360x720dpi"},"solutions":["360x360dpi"]}}`))
	}))
	defer server.Close()
	client, err := New(Config{ServerIP: server.URL, SecretKey: "secret", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	check, err := client.CheckJobConstraints(context.Background(), Session{Cookie: "session=abc"}, "JOB-1", map[string]string{"EFResolution": "360x720dpi"})
	if err != nil {
		t.Fatal(err)
	}
	if !check.Supported || check.Conflicts["EFResolution"] != "360x720dpi" || len(check.Solutions) != 1 {
		t.Fatalf("check = %#v", check)
	}
}

func TestCheckJobConstraintsAllowsAndCachesUnsupportedEndpoint(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		http.Error(w, "not supported", http.StatusNotFound)
	}))
	defer server.Close()
	client, err := New(Config{ServerIP: server.URL, SecretKey: "secret", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	check, err := client.CheckJobConstraints(context.Background(), Session{}, "JOB-1", map[string]string{"EFResolution": "360x720dpi"})
	if err != nil {
		t.Fatal(err)
	}
	if check.Supported || check.Warning == "" {
		t.Fatalf("check = %#v", check)
	}
	if _, err := client.CheckJobConstraints(context.Background(), Session{}, "JOB-2", map[string]string{"EFResolution": "360x720dpi"}); err != nil {
		t.Fatal(err)
	}
	if calls != 2 { // one POST plus one PUT probe; the second check is cached
		t.Fatalf("constraint endpoint calls = %d, want 2", calls)
	}
}

func TestLoginPayloadUsesVersionSpecificAPIKeyField(t *testing.T) {
	client := &Client{cfg: Config{Username: "admin", Password: "password", SecretKey: "secret"}}
	v5 := client.loginPayload(apiV5)
	if v5["apikey"] != "secret" || v5["accessrights"] != "" {
		t.Fatalf("unexpected v5 payload: %#v", v5)
	}
	v4 := client.loginPayload(apiV4)
	if v4["accessrights"] != "secret" || v4["apikey"] != "" {
		t.Fatalf("unexpected v4 payload: %#v", v4)
	}
}

func TestLoginAndImportJob(t *testing.T) {
	var sawLogin bool
	var sawImport bool
	var sawUpdate bool
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case apiV5 + "/login":
			sawLogin = true
			if r.Method != http.MethodPost {
				t.Fatalf("login method = %s", r.Method)
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["username"] != DefaultUsername || payload["password"] != "password" || payload["apikey"] != "secret" {
				t.Fatalf("unexpected v5 login payload: %#v", payload)
			}
			if _, legacy := payload["accessrights"]; legacy {
				t.Fatalf("v5 login unexpectedly used legacy accessrights: %#v", payload)
			}
			http.SetCookie(w, &http.Cookie{Name: "session", Value: "abc"})
			_, _ = w.Write([]byte(`{"data":{"item":{"authenticated":true}}}`))
		case apiV4 + "/jobs/JOB-123/attributes", apiV5 + "/jobs/JOB-123/attributes", apiV4 + "/jobs/JOB-123/properties", apiV5 + "/jobs/JOB-123/properties":
			http.NotFound(w, r)
		case apiV4 + "/jobs/JOB-123", apiV5 + "/jobs/JOB-123":
			if r.Method == http.MethodPut || r.Method == http.MethodPost {
				sawUpdate = true
				_, _ = w.Write([]byte(`{"ok":true}`))
				return
			}
			if r.Method == http.MethodGet {
				_, _ = w.Write([]byte(`{"data":{"item":{"id":"JOB-123","EFResolution":"360x720dpi"}}}`))
				return
			}
			t.Fatalf("job method = %s", r.Method)
		case apiV5 + "/jobs":
			sawImport = true
			if got := r.Header.Get("Cookie"); got != "session=abc" {
				t.Fatalf("cookie = %q", got)
			}
			if r.ContentLength <= int64(len("pdf")) {
				t.Fatalf("streaming multipart content length = %d", r.ContentLength)
			}
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				t.Fatal(err)
			}
			if got := r.FormValue("queue"); got != "hold" {
				t.Fatalf("queue = %q", got)
			}
			uploaded, _, err := r.FormFile("file")
			if err != nil {
				t.Fatal(err)
			}
			defer uploaded.Close()
			contents, err := io.ReadAll(uploaded)
			if err != nil || string(contents) != "pdf" {
				t.Fatalf("uploaded file = %q, err=%v", contents, err)
			}
			_, _ = w.Write([]byte(`{"data":{"item":{"id":"JOB-123"}}}`))
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client, err := New(Config{ServerIP: srv.URL, SecretKey: "secret", Password: "password", InsecureTLS: true})
	if err != nil {
		t.Fatal(err)
	}
	session, err := client.Login(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	file := filepath.Join(t.TempDir(), "sample.pdf")
	if err := os.WriteFile(file, []byte("pdf"), 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := client.ImportJob(context.Background(), session, file)
	if err != nil {
		t.Fatal(err)
	}
	if result.JobID != "JOB-123" {
		t.Fatalf("job ID = %q", result.JobID)
	}
	if err := client.UpdateJobAttributes(context.Background(), session, result.JobID, map[string]string{"EFResolution": "360x720dpi"}); err != nil {
		t.Fatal(err)
	}
	attrs, err := client.GetJobAttributes(context.Background(), session, result.JobID)
	if err != nil {
		t.Fatal(err)
	}
	if attrs["EFResolution"] != "360x720dpi" {
		t.Fatalf("EFResolution = %q", attrs["EFResolution"])
	}
	if !sawLogin || !sawImport || !sawUpdate {
		t.Fatalf("sawLogin=%v sawImport=%v sawUpdate=%v", sawLogin, sawImport, sawUpdate)
	}
}
