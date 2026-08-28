package fiery

import (
	"context"
	"encoding/json"
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
			if payload["username"] != DefaultUsername || payload["password"] != "password" || payload["accessrights"] != "secret" {
				t.Fatalf("unexpected login payload: %#v", payload)
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
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				t.Fatal(err)
			}
			if got := r.FormValue("queue"); got != "hold" {
				t.Fatalf("queue = %q", got)
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
