package main

import (
	"os"
	"strings"
	"testing"
)

func TestFrontendUsesGeneratedConnectionDTOFieldNames(t *testing.T) {
	app, err := os.ReadFile("frontend/src/app.js")
	if err != nil {
		t.Fatal(err)
	}
	source := string(app)
	for _, forbidden := range []string{".testOK", ".activeIPAddress"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("frontend uses non-generated connection field %q", forbidden)
		}
	}
	for _, required := range []string{".testOk", ".activeIpAddress"} {
		if !strings.Contains(source, required) {
			t.Fatalf("frontend does not consume generated connection field %q", required)
		}
	}
	binding, err := os.ReadFile("frontend/src/bindings/api-automation/internal/application/models.js")
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{`this["testOk"]`, `this["activeIpAddress"]`} {
		if !strings.Contains(string(binding), field) {
			t.Fatalf("generated binding no longer provides %s", field)
		}
	}
}
