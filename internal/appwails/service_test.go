package appwails

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"api-automation/internal/capabilities"
	"api-automation/internal/fiery"
	"api-automation/internal/preflight"
)

func TestApplicationStateNeverSerializesCredentials(t *testing.T) {
	service := NewService("embedded-secret-marker", Options{DataDirectory: t.TempDir(), DisableDiagnostic: true})
	payload, err := json.Marshal(service.State())
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, forbidden := range []string{"embedded-secret-marker", `"secretKey":`, `"password":`, `"cookie":`} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("safe application state contains %q: %s", forbidden, text)
		}
	}
}

func TestDiagnosticsUseInjectedDataIdentityAndClose(t *testing.T) {
	root := t.TempDir()
	service := NewService("secret-not-for-log", Options{DataDirectory: root})
	path := service.State().DiagnosticPath
	if filepath.Dir(filepath.Dir(path)) != root {
		t.Fatalf("diagnostic path %q is outside application data root %q", path, root)
	}
	Shutdown(service)
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "secret-not-for-log") || !strings.Contains(string(body), "Application exiting") {
		t.Fatalf("unsafe or incomplete diagnostic: %q", body)
	}
}

func TestDiagnosticLogsUseUniqueFiles(t *testing.T) {
	root := t.TempDir()
	first := newDiagnosticLog(root)
	second := newDiagnosticLog(root)
	defer first.Close()
	defer second.Close()
	if first.Path() == "" || second.Path() == "" || first.Path() == second.Path() {
		t.Fatalf("diagnostic paths must be non-empty and unique: %q %q", first.Path(), second.Path())
	}
}

func TestDiagnosticsAndCapturesUseInjectedDebugDirectory(t *testing.T) {
	dataRoot := t.TempDir()
	debugRoot := t.TempDir()
	service := NewService("secret-not-for-log", Options{DataDirectory: dataRoot, DebugDirectory: debugRoot})
	defer Shutdown(service)
	if got := filepath.Dir(filepath.Dir(service.State().DiagnosticPath)); got != debugRoot {
		t.Fatalf("diagnostic root = %q, want %q", got, debugRoot)
	}
	client, err := fiery.New(fiery.Config{ServerIP: "127.0.0.1", SecretKey: "key", Password: "password"})
	if err != nil {
		t.Fatal(err)
	}
	paths, warnings := service.saveCapabilityEvidence(client, fiery.CapabilitySnapshot{CapturedAt: time.Now()}, capabilities.Model{}, preflight.EnvironmentSnapshot{})
	if len(warnings) != 0 || len(paths) != 3 {
		t.Fatalf("paths=%v warnings=%v", paths, warnings)
	}
	captureRoot := filepath.Join(debugRoot, "captures")
	for _, path := range paths {
		if filepath.Dir(path) != captureRoot {
			t.Fatalf("capture path %q is outside %q", path, captureRoot)
		}
	}
	if _, err := os.Stat(filepath.Join(dataRoot, "captures")); !os.IsNotExist(err) {
		t.Fatalf("captures unexpectedly written below data directory: %v", err)
	}
}

func TestBoundErrorsRedactCredentialValues(t *testing.T) {
	err := redactError(errors.New("server echoed secret-value and password-value"), "secret-value", "password-value")
	if strings.Contains(err.Error(), "secret-value") || strings.Contains(err.Error(), "password-value") {
		t.Fatalf("credential remained in bound error: %v", err)
	}
}

func TestCapabilityViewClonesAndSortsSafeReadOnlyData(t *testing.T) {
	minimum, maximum, increment := 1.0, 10.0, 1.0
	source := capabilities.Model{
		ServerName: "Fiery", PressModel: "Press", SerialNumber: "serial", Version: "1",
		ServerPresets:   []fiery.ServerPreset{{ID: "preset", Name: "Production"}},
		ExcludedOptions: []capabilities.ExcludedOption{{ID: "hidden"}},
		Options: []capabilities.Option{
			{ID: "B", Label: "Second", Group: "Layout", Values: []string{"One"}},
			{ID: "A", Label: "First", Group: "Job Info", Numeric: true, Range: &capabilities.NumericRange{Min: minimum, Max: maximum, Increment: increment}},
		},
	}
	view := capabilityView(time.Unix(1, 0), source)
	if view.OptionCount != 2 || view.ExcludedCount != 1 || view.Options[0].ID != "A" || view.Presets[0].ID != "preset" {
		t.Fatalf("unexpected view: %#v", view)
	}
	clone := cloneCapabilityView(&view)
	view.Options[1].Values[0] = "changed"
	if clone.Options[1].Values[0] != "One" {
		t.Fatal("capability DTO aliases mutable option values")
	}
	payload, err := json.Marshal(clone)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{`"secretKey":`, `"password":`, `"cookie":`} {
		if strings.Contains(strings.ToLower(string(payload)), strings.ToLower(forbidden)) {
			t.Fatalf("capability DTO contains %q", forbidden)
		}
	}
}
