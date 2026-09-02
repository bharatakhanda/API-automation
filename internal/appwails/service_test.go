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
)

func TestPreviewStateNeverSerializesCredentials(t *testing.T) {
	service := NewService("embedded-secret-marker", Options{DataDirectory: t.TempDir(), DisableDiagnostic: true})
	payload, err := json.Marshal(service.State())
	if err != nil {
		t.Fatal(err)
	}
	text := string(payload)
	for _, forbidden := range []string{"embedded-secret-marker", `"secretKey":`, `"password":`, `"cookie":`} {
		if strings.Contains(strings.ToLower(text), strings.ToLower(forbidden)) {
			t.Fatalf("safe preview state contains %q: %s", forbidden, text)
		}
	}
}

func TestPreviewDiagnosticsUseInjectedDataIdentityAndClose(t *testing.T) {
	root := t.TempDir()
	service := NewService("secret-not-for-log", Options{DataDirectory: root})
	path := service.State().DiagnosticPath
	if filepath.Dir(filepath.Dir(path)) != root {
		t.Fatalf("diagnostic path %q is outside preview data root %q", path, root)
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
