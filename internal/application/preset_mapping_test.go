package application

import (
	"testing"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/fiery"
	"api-automation/internal/presets"
)

func presetCapabilityModel() capabilities.Model {
	return capabilities.Model{ServerName: "Fiery", SerialNumber: "SERVER-1", ServerPresets: []fiery.ServerPreset{{ID: "P-1", Name: "Production"}}, Options: []capabilities.Option{
		{ID: "EFColorMode", Values: []string{"CMYK", "Grayscale"}},
		{ID: CopiesOptionID, Value: "1"},
		{ID: "Scaling", Range: &capabilities.NumericRange{Min: 25, Max: 400, Increment: 1}},
		{ID: PageRangeOptionID, Values: []string{"All", PageRangeRangeValue}},
	}}
}

func TestBuildSafePresetFiltersUnknownProperties(t *testing.T) {
	preset := BuildSafePreset(presetCapabilityModel(), PresetCaptureRequest{
		Name: " Production ", SelectedValues: map[string][]string{"EFColorMode": {"CMYK"}, "unknown": {"secret"}},
		NumericInputs: map[string]string{"Scaling": "100", "unknown": "5"}, CopiesInput: "5-7", CustomPageRange: "1,3-5",
		Strategy: combinations.StrategyPairwise, ValueSource: ValueSourceAdvertised, TestIntent: TestIntentConstraint,
		ConstraintMode: ConstraintValidationOnly, MaxCases: "250", ParallelJobs: "8", RunModes: []RunMode{RunModes()[1]}, FileMode: "random", ServerPresetID: "P-1",
	})
	if preset.Name != "Production" || preset.SelectedValues["EFColorMode"][0] != "CMYK" || preset.NumericInputs["Scaling"] != "100" || preset.NumericInputs[CopiesOptionID] != "5-7" || preset.NumericInputs[PageRangeOptionID] != "1,3-5" {
		t.Fatalf("preset = %#v", preset)
	}
	if _, exists := preset.SelectedValues["unknown"]; exists {
		t.Fatal("unknown selected property survived safe capture")
	}
}

func TestReconcilePresetCanonicalizesAndRejectsStaleValues(t *testing.T) {
	preset := presets.Preset{
		Name: "Production", ServerSerial: "OTHER", ServerPresetID: "missing",
		SelectedValues: map[string][]string{"EFColorMode": {"cmyk", "stale"}},
		NumericInputs:  map[string]string{CopiesOptionID: "5-7", "Scaling": "100", PageRangeLegacyDataID: "1,3-5", "missing": "2"},
		Strategy:       "pairwise", ValueSource: "advertised", TestIntent: "constraint", ConstraintMode: "validation",
		MaxCases: "999999", ParallelJobs: "999", RunModes: []string{"Process and Hold", "stale mode"}, FileMode: "random",
	}
	result := ReconcilePreset(presetCapabilityModel(), preset)
	if result.SelectedValues["EFColorMode"][0] != "CMYK" || result.CopiesInput != "5-7" || result.NumericInputs["Scaling"] != "100" || result.CustomPageRange != "1,3-5" {
		t.Fatalf("reconciled = %#v", result)
	}
	if !result.DifferentServer || result.Missing != 4 || result.ServerPresetID != "" {
		t.Fatalf("warnings = %#v", result)
	}
	if !result.HasStrategy || result.Strategy != combinations.StrategyPairwise || result.MaxCases != "10000" || result.ParallelJobs != "10" || len(result.RunModeLabels) != 1 || result.RunModeLabels[0] != "Process and Hold" {
		t.Fatalf("controls = %#v", result)
	}
}
