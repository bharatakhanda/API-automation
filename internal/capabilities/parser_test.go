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

func TestFromSnapshotPopulatesReadOnlyServerPresets(t *testing.T) {
	snapshot := fiery.CapabilitySnapshot{Endpoints: []fiery.EndpointSnapshot{{
		Name: "v5_presets",
		Body: raw(`{"data":{"items":[{"id":"PRESET-1","name":"Production","attributes":{"EFColorMode":"CMYK"}}]}}`),
	}}}
	model := FromSnapshot(snapshot)
	if len(model.ServerPresets) != 1 || model.ServerPresets[0].ID != "PRESET-1" || model.ServerPresets[0].Attributes["EFColorMode"] != "CMYK" {
		t.Fatalf("server presets = %#v", model.ServerPresets)
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

func TestParsePropertyMetadataRangeAndConstraints(t *testing.T) {
	options := parseProperties(raw(`{"data":{"items":[{"id":"EFMediaThickness","group":"fppapersource","ppdtype":"efirange","value":2,"value_attributes":{"editable":true,"increment":1,"min":1,"max":50,"precision":0},"constraints":{}},{"id":"EFResolution","group":"fpimage","ppdtype":"uimenu","value":"360x360dpi","values":["360x360dpi","360x720dpi"],"constraints":{"360x720dpi":{"EFEdgeDropSize":["0_1_2_2_2"]}}}]}}`))
	if len(options) != 2 {
		t.Fatalf("options = %#v", options)
	}
	rangeOption := options[0]
	if rangeOption.ID != "EFMediaThickness" || rangeOption.Group != "fppapersource" || rangeOption.Range == nil || rangeOption.Range.Min != 1 || rangeOption.Range.Max != 50 {
		t.Fatalf("range option = %#v", rangeOption)
	}
	resolution := options[1]
	if got := resolution.Constraints["360x720dpi"]["EFEdgeDropSize"]; len(got) != 1 || got[0] != "0_1_2_2_2" {
		t.Fatalf("constraints = %#v", resolution.Constraints)
	}
}

func TestCategorySearchAndExplicitConstraintValidation(t *testing.T) {
	model := Model{Options: []Option{
		{ID: "EFResolution", Label: "Resolution", Group: "fpimage", Values: []string{"360x360dpi", "360x720dpi"}, Constraints: Constraints{"360x720dpi": {"EFEdgeDropSize": {"0_1_2_2_2"}}}},
		{ID: "EFEdgeDropSize", Label: "Edge enhancement", Group: "fpimage", Values: []string{"None", "0_1_2_2_2"}},
		{ID: "EFMediaThickness", Label: "Media thickness", Group: "fppapersource", Range: &NumericRange{Min: 1, Max: 50, Increment: 1}},
	}}
	groups := FilteredGroups(model, "thickness")
	if len(groups) != 1 || groups[0].Name != "Substrate / Media" || len(groups[0].Options) != 1 {
		t.Fatalf("filtered groups = %#v", groups)
	}
	valid := map[string]string{"EFResolution": "360x720dpi", "EFEdgeDropSize": "0_1_2_2_2"}
	if conflicts := ValidateCombination(model, valid); len(conflicts) != 0 {
		t.Fatalf("valid conflicts = %#v", conflicts)
	}
	invalid := map[string]string{"EFResolution": "360x720dpi", "EFEdgeDropSize": "None"}
	if conflicts := ValidateCombination(model, invalid); len(conflicts) != 1 {
		t.Fatalf("invalid conflicts = %#v", conflicts)
	}
	if !NeedsConstraintCheck(model, map[string]string{"EFResolution": "360x720dpi"}) || NeedsConstraintCheck(model, map[string]string{"Unrelated": "x"}) {
		t.Fatal("constraint-check requirement detection is incorrect")
	}
}

func TestGroupedOptionsUsesPDFHeadingsNestedSectionsAndOrder(t *testing.T) {
	model := Model{Options: []Option{
		{ID: "EFResolution", Label: "Resolution", Group: "fpimage"},
		{ID: "EFEdgeDropSize", Label: "Edge", Group: "fpimage"},
		{ID: "EFColorMode", Label: "Color", Group: "fpcolorwise"},
		{ID: "EFRGBOverride", Label: "RGB", Group: "fpcolorwise"},
		{ID: "EFPrintDieLine", Label: "Die", Group: "fpjobinfo"},
		{ID: "Instruct", Label: "Instructions", Group: "fpjobinfo"},
	}}
	groups := GroupedOptions(model)
	if names := CategoryNames(model); len(names) != 3 || names[0] != "Job Info" || names[1] != "Color" || names[2] != "Image" {
		t.Fatalf("PDF category order = %v", names)
	}
	for _, name := range CategoryNames(model) {
		if name == "Quick Access" || name == "Color and Image" {
			t.Fatalf("obsolete category remained: %q", name)
		}
	}
	job := groups[0]
	if len(job.Sections) != 2 || job.Sections[0].Name != "Job notes" || job.Sections[1].Name != "Die printing" {
		t.Fatalf("job sections = %#v", job.Sections)
	}
	color := groups[1]
	if len(color.Sections) != 2 || color.Sections[0].Name != "" || color.Sections[1].Name != "Color input" {
		t.Fatalf("color sections = %#v", color.Sections)
	}
	image := groups[2]
	if len(image.Sections) != 2 || image.Sections[0].Name != "" || image.Sections[1].Name != "Edge enhancement" {
		t.Fatalf("image sections = %#v", image.Sections)
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
