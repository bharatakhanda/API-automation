package appwails

import (
	"os"
	"path/filepath"
	"testing"

	core "api-automation/internal/application"
	"api-automation/internal/capabilities"
	"api-automation/internal/presets"
	"api-automation/internal/reportxlsx"
)

func TestResolveTestFilesUsesSharedSelectionRules(t *testing.T) {
	dir := t.TempDir()
	pdf := filepath.Join(dir, "input.pdf")
	if err := os.WriteFile(pdf, []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "ignored.txt"), []byte("test"), 0o600); err != nil {
		t.Fatal(err)
	}
	service := NewService("", Options{DataDirectory: t.TempDir(), DisableDiagnostic: true})
	resolved, err := service.ResolveTestFiles(FileSelection{FolderPath: dir, Mode: "all"})
	if err != nil || resolved.Count != 1 || resolved.Files[0] != pdf {
		t.Fatalf("resolved=%#v err=%v", resolved, err)
	}
}

func TestPreviewPlanReturnsDirectPageRangeWireValue(t *testing.T) {
	service := NewService("", Options{DataDirectory: t.TempDir(), DisableDiagnostic: true})
	model := capabilities.Model{Options: []capabilities.Option{{ID: core.PageRangeOptionID, Label: "Page Range", Values: []string{"All", core.PageRangeRangeValue}, Enabled: true}}}
	service.capabilities = &model
	view, err := service.PreviewPlan(PlanningInput{
		SelectedValues:  map[string][]string{core.PageRangeOptionID: {core.PageRangeRangeValue}},
		CustomPageRange: "5 to 10", ValueSource: "selected", Strategy: "single", TestIntent: "positive", MaxCases: 100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if view.CombinationCount != 1 || view.Combinations[0][core.PageRangeOptionID] != "5-10" {
		t.Fatalf("unexpected plan: %#v", view)
	}
	if _, present := view.Combinations[0][core.PageRangeLegacyDataID]; present {
		t.Fatal("legacy page-range companion leaked into preview")
	}
}

func TestPreviewPlanPreservesOutputProfileWireIdentity(t *testing.T) {
	service := NewService("", Options{DataDirectory: t.TempDir(), DisableDiagnostic: true})
	value := "\uFEFFProfile A"
	model := capabilities.Model{Options: []capabilities.Option{{ID: core.OutputProfileOptionID, Label: "Output Profile", Values: []string{value}, Enabled: true}}}
	service.capabilities = &model
	view, err := service.PreviewPlan(PlanningInput{SelectedValues: map[string][]string{core.OutputProfileOptionID: {value}}, ValueSource: "selected", Strategy: "single", TestIntent: "positive", MaxCases: 10})
	if err != nil {
		t.Fatal(err)
	}
	if got := view.Combinations[0][core.OutputProfileOptionID]; got != value {
		t.Fatalf("wire identity = %q, want %q", got, value)
	}
}

func TestPreviewPresetRoundTripUsesDistinctInjectedStore(t *testing.T) {
	service := NewService("", Options{DataDirectory: t.TempDir(), DisableDiagnostic: true})
	model := capabilities.Model{ServerName: "Fiery", SerialNumber: "serial", Options: []capabilities.Option{{ID: "Duplex", Label: "Duplex", Values: []string{"Off", "On"}, Enabled: true}}}
	service.capabilities = &model
	store, err := presets.New(filepath.Join(t.TempDir(), "presets.json"))
	if err != nil {
		t.Fatal(err)
	}
	service.presets = store
	if err := service.SavePreset(PresetInput{Name: "Safe", SelectedValues: map[string][]string{"Duplex": {"On"}}, Strategy: "single", ValueSource: "selected", TestIntent: "positive", ConstraintMode: "validation", MaxCases: "100", ParallelJobs: "1", RunModeLabels: []string{"Hold"}, FileMode: "all"}); err != nil {
		t.Fatal(err)
	}
	loaded, err := service.LoadPreset("safe")
	if err != nil || loaded.SelectedValues["Duplex"][0] != "On" || loaded.SkippedCount != 0 {
		t.Fatalf("loaded=%#v err=%v", loaded, err)
	}
}

func TestRunStateCloningAndEventApplication(t *testing.T) {
	state := AutomationState{}
	result := reportxlsx.Result{JobID: "job", Result: "PASS", SetValues: map[string]string{"EFPageRange": "5-10"}}
	applyRunEvent(&state, core.RunEvent{Kind: core.RunEventResult, Result: &core.RunResultEvent{Result: result}})
	applyRunEvent(&state, core.RunEvent{Kind: core.RunEventProgress, Progress: &core.RunProgressEvent{Planned: 1, Executed: 1, Passed: 1}})
	clone := cloneAutomationState(state)
	state.Results[0].SetValues["EFPageRange"] = "changed"
	if clone.Results[0].SetValues["EFPageRange"] != "5-10" || clone.Progress.Passed != 1 {
		t.Fatalf("cloned state was aliased or incomplete: %#v", clone)
	}
}

func TestWailsAdministrationUsesSharedAutomationInterlock(t *testing.T) {
	service := NewService("", Options{DataDirectory: t.TempDir(), DisableDiagnostic: true})
	service.run = &activeRun{state: AutomationState{Status: "Running"}}
	if err := service.administrationPrecondition(); err == nil {
		t.Fatal("running automation did not block Wails administration")
	}
}

func TestRunModeIDsRejectFrontendDefinedSemantics(t *testing.T) {
	if _, err := runModesByID([]string{"frontend-invented-mode"}); err == nil {
		t.Fatal("unknown run mode was accepted")
	}
	modes, err := runModesByID([]string{"hold", "hold"})
	if err != nil || len(modes) != 1 || modes[0].Label != "Hold" {
		t.Fatalf("modes=%#v err=%v", modes, err)
	}
}
