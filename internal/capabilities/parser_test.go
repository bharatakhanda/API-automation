package capabilities

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"api-automation/internal/fiery"
)

func TestFromSnapshotExtractsServerQueuesAndOptions(t *testing.T) {
	snapshot := fiery.CapabilitySnapshot{Endpoints: []fiery.EndpointSnapshot{
		{Name: "info", Body: raw(`{"data":{"item":{"name":"SERVER-85","serial_number":"P00014754","version":"1.4"}}}`)},
		{Name: "queues", Body: raw(`{"data":{"items":[{"id":1,"name":"hold","available":true,"editable":true},{"id":2,"name":"font","available":false,"editable":true}]}}`)},
		{Name: "properties", Body: raw(`{"data":{"items":[{"id":"EFResolution","value":"360x360dpi","values":["360x360dpi","360x720dpi"],"scopes":["ps"]},{"id":"EFColorMode","value":"CMYK","values":["CMYK","CMYKPLUS"],"scopes":["ps"]},{"id":"Ignored","value":"x","values":["x"]}]}}`)},
	}}

	model := FromSnapshot(snapshot)
	if model.ServerName != "SERVER-85" || model.SerialNumber != "P00014754" || model.Version != "1.4" {
		t.Fatalf("unexpected server metadata: %#v", model)
	}
	if len(model.Queues) != 2 || model.Queues[0].Name != "hold" || !model.Queues[0].Available {
		t.Fatalf("unexpected queues: %#v", model.Queues)
	}
	if option, ok := model.OptionByID("EFResolution"); !ok || len(option.Values) != 2 {
		t.Fatalf("missing resolution option: %#v", model.Options)
	}
	if option, ok := model.OptionByID("Ignored"); !ok || option.Label != "Ignored" {
		t.Fatalf("all discovered options should be exposed, got %#v", option)
	}
	copies, ok := model.OptionByID("num copies")
	if !ok || copies.Label != "Copies" || len(copies.Values) == 0 {
		t.Fatalf("expected synthetic copies option, got %#v", copies)
	}
}

func TestCapturedSnapshotProducesUIPopulatableModel(t *testing.T) {
	path := filepath.Join("..", "..", "server-capabilities-snapshot-20260827-174908.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		t.Skip("captured snapshot not present")
	}
	if err != nil {
		t.Fatal(err)
	}
	var snapshot fiery.CapabilitySnapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		t.Fatal(err)
	}
	model := FromSnapshot(snapshot)
	if model.ServerName != "SERVER-85" {
		t.Fatalf("server name = %q", model.ServerName)
	}
	if len(model.Queues) == 0 {
		t.Fatal("expected queues")
	}
	for _, id := range []string{"PageSize", "EFResolution", "EFColorMode", "EFMediaType", "EFPrintSpeed", "num copies"} {
		option, ok := model.OptionByID(id)
		if !ok || len(option.Values) == 0 {
			t.Fatalf("expected option %s with values, got %#v", id, option)
		}
	}
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }
