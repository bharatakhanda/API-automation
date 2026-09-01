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

func TestFromSnapshotKeepsExistingInfoWhenLaterResponseIsPartial(t *testing.T) {
	snapshot := fiery.CapabilitySnapshot{Endpoints: []fiery.EndpointSnapshot{
		{Name: "v5_info", Body: raw(`{"data":{"item":{"name":"SERVER-85","serial_number":"P00014754","version":"1.4"}}}`)},
		{Name: "v4_info", Body: raw(`{"data":{"item":{"name":""}}}`)},
	}}
	model := FromSnapshot(snapshot)
	if model.ServerName != "SERVER-85" || model.SerialNumber != "P00014754" || model.Version != "1.4" {
		t.Fatalf("later partial info erased metadata: %#v", model)
	}
}

func TestParsePropertiesIgnoresNullValues(t *testing.T) {
	options := parseProperties(raw(`{"data":{"items":[{"id":"NullOption","value":null,"values":[null,"", "valid"]}]}}`))
	if len(options) != 1 || options[0].Value != "" || len(options[0].Values) != 1 || options[0].Values[0] != "valid" {
		t.Fatalf("unexpected null normalization: %#v", options)
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

func TestGroupedOptionsDoesNotDisplayOneAPIOptionTwice(t *testing.T) {
	model := Model{Options: []Option{{ID: "EFEdgeDropSize", Label: "Edge", Values: []string{"1"}}}}
	groups := GroupedOptions(model)
	count := 0
	for _, group := range groups {
		for _, option := range group.Options {
			if option.ID == "EFEdgeDropSize" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("EFEdgeDropSize displayed %d times, want 1", count)
	}
}

func TestGroupedOptionsDoesNotDisplayCopiesAliasesTwice(t *testing.T) {
	model := Model{Options: []Option{
		{ID: "EFCopies", Values: []string{"1"}},
		{ID: "num copies", Values: []string{"1"}},
	}}
	count := 0
	for _, group := range GroupedOptions(model) {
		for _, option := range group.Options {
			if option.ID == "EFCopies" || option.ID == "num copies" {
				count++
			}
		}
	}
	if count != 1 {
		t.Fatalf("copies options displayed %d times, want 1", count)
	}
}

func raw(s string) json.RawMessage { return json.RawMessage(s) }
