package fiery

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServerPresetDiscoveryAndApplicationAreReadOnly(t *testing.T) {
	var applied bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == apiV5+"/presets":
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":"P-2","name":"Proof","attributes":{"EFColorMode":"CMYK"}},{"id":"P-1","name":"Archive"}]}}`))
		case r.Method == http.MethodPut && r.URL.Path == apiV5+"/jobs/JOB-1":
			if r.URL.Query().Get("preset") != "P-2" {
				t.Fatalf("preset query = %q", r.URL.Query().Get("preset"))
			}
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["preset"] != "P-2" {
				t.Fatalf("preset payload = %#v", payload)
			}
			applied = true
			_, _ = w.Write([]byte(`{"data":{"item":{"preset":true}}}`))
		default:
			t.Fatalf("unexpected preset request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client, err := New(Config{ServerIP: server.URL, SecretKey: "secret", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	session := Session{Cookie: "session=abc"}
	presets, err := client.ListServerPresets(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) != 2 || presets[0].ID != "P-1" || presets[1].Attributes["EFColorMode"] != "CMYK" {
		t.Fatalf("presets = %#v", presets)
	}
	if err := client.ApplyServerPreset(context.Background(), session, "JOB-1", "P-2"); err != nil {
		t.Fatal(err)
	}
	if !applied {
		t.Fatal("server preset was not applied")
	}
}

func TestServerAdministrationUsesOnlyDocumentedV5Operations(t *testing.T) {
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == apiV5+"/jobs":
			seen["jobs"] = true
			_, _ = w.Write([]byte(`{"data":{"items":[{"id":"1","title":"A","username":"u","status":"held","state":"spooled"},{"id":"2","title":"B"}]}}`))
		case r.Method == http.MethodPost && r.URL.Path == apiV5+"/server/reboot":
			seen["reboot"] = true
			_, _ = w.Write([]byte(`{"status":true}`))
		case r.Method == http.MethodPost && r.URL.Path == apiV5+"/server" && r.URL.Query().Get("method") == "restart":
			seen["restart"] = true
			_, _ = w.Write([]byte(`{"restart":true}`))
		case r.Method == http.MethodPost && r.URL.Path == apiV5+"/server" && r.URL.Query().Get("method") == "clear":
			if r.URL.Query().Get("services") != "jobs" || r.URL.Query().Get("status") != "" {
				t.Fatalf("unsafe clear query: %s", r.URL.RawQuery)
			}
			seen["clear"] = true
			_, _ = w.Write([]byte(`{"clear":true}`))
		case r.Method == http.MethodGet && r.URL.Path == apiV5+"/status":
			seen["status"] = true
			_, _ = w.Write([]byte(`{"data":{"item":{"fiery":"running","fieryExtendedStatus":"none"}}}`))
		default:
			t.Fatalf("unexpected administration request: %s %s", r.Method, r.URL.String())
		}
	}))
	defer server.Close()
	client, err := New(Config{ServerIP: server.URL, SecretKey: "secret", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	session := Session{Cookie: "session=abc"}
	jobs, err := client.ListJobs(context.Background(), session)
	if err != nil || len(jobs) != 2 || jobs[0].Name != "A" {
		t.Fatalf("jobs=%#v err=%v", jobs, err)
	}
	if err := client.RestartFieryProcess(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if err := client.RebootServer(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if err := client.ClearAllJobs(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	status, err := client.ServerStatus(context.Background(), session)
	if err != nil || status != "running" {
		t.Fatalf("status=%q err=%v", status, err)
	}
	activity, err := client.ServerActivityStatus(context.Background(), session)
	if err != nil || activity.Health != "running" || activity.Extended != "none" || activity.Workload != "Idle" {
		t.Fatalf("activity=%#v err=%v", activity, err)
	}
	for _, operation := range []string{"jobs", "restart", "reboot", "clear", "status"} {
		if !seen[operation] {
			t.Fatalf("operation %q was not called: %#v", operation, seen)
		}
	}
}

func TestJobWorkloadDetectsExternalRIPAndUsesBoundedTailProbe(t *testing.T) {
	payload := []byte(`{"data":{"totalItems":3,"items":[
		{"id":"1","status":"done ripping","state":"processed","is ripping?":"no","JOBIS":"JOB_IS_RIPPING"},
		{"id":"2","status":"waiting to rip","state":"waiting to process"},
		{"id":"3","status":"ripping","state":"processing","is ripping?":"yes"}
	]}}`)
	summary, err := ParseJobWorkload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalItems != 3 || summary.InspectedItems != 3 || summary.ActiveJobs != 2 || summary.EvidenceID != "2" {
		t.Fatalf("job workload = %#v", summary)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != apiV5+"/jobs" || r.URL.Query().Get("limit") != "64" || r.URL.Query().Get("offset") != "36" || r.URL.Query().Get("start") != "36" {
			t.Fatalf("unexpected workload probe: %s", r.URL.String())
		}
		_, _ = w.Write([]byte(`{"data":{"totalItems":100,"items":[{"id":"99","status":"printing","state":"printing"}]}}`))
	}))
	defer server.Close()
	client, err := New(Config{ServerIP: server.URL, SecretKey: "secret", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	probe, err := client.ProbeRecentJobWorkload(context.Background(), Session{Cookie: "session=abc"}, 100, 64)
	if err != nil || probe.Offset != 36 || probe.ActiveJobs != 1 || probe.TotalItems != 100 {
		t.Fatalf("probe=%#v err=%v", probe, err)
	}
}

func TestBoundedJobProbeRejectsIgnoredPagination(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		items := make([]map[string]string, 65)
		for index := range items {
			items[index] = map[string]string{"id": string(rune('A' + index%26)), "status": "done spooling"}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"totalItems": 65, "items": items}})
	}))
	defer server.Close()
	client, err := New(Config{ServerIP: server.URL, SecretKey: "secret", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.ProbeRecentJobWorkload(context.Background(), Session{}, 65, 64); err == nil {
		t.Fatal("ignored pagination was accepted as a bounded workload probe")
	}
}

func TestFieryWorkloadStateUsesExtendedAPIStatus(t *testing.T) {
	for _, test := range []struct {
		health, extended, want string
	}{
		{"running", "none", "Idle"},
		{"running", "idle", "Idle"},
		{"running", "printing", "Busy"},
		{"running", "processing job", "Busy"},
		{"restarting", "none", "Busy"},
		{"stopped", "none", "Unavailable"},
	} {
		if got := fieryWorkloadState(test.health, test.extended); got != test.want {
			t.Fatalf("workload(%q, %q) = %q, want %q", test.health, test.extended, got, test.want)
		}
	}
}

func TestListJobsNeverTreatsMalformedOrPartialSuccessAsEmptyInventory(t *testing.T) {
	response := `{"data":{"unexpected":[]}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()
	client, err := New(Config{ServerIP: server.URL, SecretKey: "secret", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	if jobs, err := client.ListJobs(context.Background(), Session{}); err == nil || jobs != nil {
		t.Fatalf("malformed inventory was treated as empty: jobs=%#v err=%v", jobs, err)
	}
	response = `{"data":{"totalItems":2,"items":[{"id":"1"}]}}`
	if jobs, err := client.ListJobs(context.Background(), Session{}); err == nil || jobs != nil {
		t.Fatalf("partial inventory was accepted: jobs=%#v err=%v", jobs, err)
	}
}

func TestServerAdministrationRejectsExplicitFalseAcknowledgement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"item":{"clear":false}}}`))
	}))
	defer server.Close()
	client, err := New(Config{ServerIP: server.URL, SecretKey: "secret", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	if err := client.ClearAllJobs(context.Background(), Session{}); err == nil {
		t.Fatal("explicit clear=false unexpectedly succeeded")
	}
}

func TestParseServerPresetsRejectsMissingAndDuplicateIDs(t *testing.T) {
	presets := ParseServerPresets([]byte(`{"data":{"items":[{"id":"A","name":"One"},{"id":"A","name":"Duplicate"},{"name":"Missing"}]}}`))
	if len(presets) != 1 || presets[0].ID != "A" || presets[0].Name != "One" {
		t.Fatalf("presets = %#v", presets)
	}
}
