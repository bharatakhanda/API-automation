//go:build windows

package appgio

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/fiery"
	"api-automation/internal/model"
	"api-automation/internal/presets"

	"gioui.org/widget"
)

func TestRunModesHaveExpectedLifecycle(t *testing.T) {
	tests := []struct {
		index   int
		label   string
		actions []string
	}{
		{0, "Hold", nil},
		{1, "Process and Hold", []string{"rip"}},
		{2, "RIP", []string{"rip"}},
		{3, "Press Print", []string{"rip", "production", "press_print"}},
		{4, "Ready to Print", []string{"rip", "production"}},
		{5, "Print", []string{"rip", "production", "press_print", "print"}},
		{6, "Cancel while Processing/Ripping", []string{"cancel_ripping"}},
		{7, "Cancel while Waiting to Print", []string{"rip", "production", "cancel_waiting"}},
		{8, "Cancel while Printing", []string{"rip", "production", "press_print", "cancel_printing"}},
		{9, "Delete", []string{"delete"}},
	}
	if len(runModes) != len(tests) {
		t.Fatalf("run mode count = %d, want %d", len(runModes), len(tests))
	}
	for _, test := range tests {
		mode := runModes[test.index]
		if mode.Label != test.label || mode.ImportQueue != "hold" {
			t.Fatalf("mode %d = %#v", test.index, mode)
		}
		if len(mode.Actions) != len(test.actions) {
			t.Fatalf("mode %q actions = %#v, want %#v", mode.Label, mode.Actions, test.actions)
		}
		for index := range test.actions {
			if mode.Actions[index] != test.actions[index] {
				t.Fatalf("mode %q actions = %#v, want %#v", mode.Label, mode.Actions, test.actions)
			}
		}
	}
}

func TestWorkspaceNavigationSeparatesOperationalPages(t *testing.T) {
	if len(workspacePages) != pageCount {
		t.Fatalf("workspace page count = %d, want %d", len(workspacePages), pageCount)
	}
	pages := []struct {
		page  int
		label string
	}{
		{pageConnection, "Connection"}, {pageOverview, "Overview"}, {pageTestSettings, "Test Settings"},
		{pageJobProperties, "Job Properties"}, {pageAutomation, "Automation"}, {pageResults, "Results"},
	}
	for _, want := range pages {
		if workspacePages[want.page].NavigationLabel != want.label {
			t.Fatalf("workspace page %d = %#v, want %q", want.page, workspacePages[want.page], want.label)
		}
	}
	if workspacePages[pageAdministration].NavigationLabel != "Administration" || pageAdministration == pageResults {
		t.Fatalf("administration page is not separate: %#v", workspacePages[pageAdministration])
	}
	if workspacePages[pageLogs].NavigationLabel != "Activity Logs" || pageLogs == pageResults {
		t.Fatalf("logs page is not separate: %#v", workspacePages[pageLogs])
	}

	window := &Window{activePage: pageConnection}
	window.setActivePage(pageOverview)
	if window.activePage != pageConnection {
		t.Fatalf("connection gate allowed page %d before approval", window.activePage)
	}
	window.hasActiveServer = true
	window.activePage = pageResults
	window.list.Position.Offset = 42
	window.setActivePage(pageLogs)
	if window.activePage != pageLogs || window.list.Position.Offset != 0 {
		t.Fatalf("page switch did not reset scrolling: page=%d position=%#v", window.activePage, window.list.Position)
	}
}

func TestExistingCredentialsStayOutOfConnectionEditors(t *testing.T) {
	window := &Window{
		activeServer:    model.ServerConnection{IPAddress: "fiery.example", SecretKey: "configured-key", Password: "configured-password"},
		hasActiveServer: true,
		activePage:      pageOverview,
	}
	window.secretKey.SetText("stale editor value")
	window.password.SetText("stale editor value")
	window.beginConnectionChange()
	if window.secretKey.Text() != "" || window.password.Text() != "" {
		t.Fatal("existing secret key or password was repopulated into a connection editor")
	}
	draft := window.draftConnectionUnchecked()
	if draft.SecretKey != "configured-key" || draft.Password != "configured-password" {
		t.Fatal("blank replacement fields did not retain configured credentials internally")
	}
}

func TestServerAdministrationIsBlockedByConcurrentOperations(t *testing.T) {
	window := &Window{}
	if err := window.serverAdministrationPrecondition(); err != nil {
		t.Fatalf("idle administration was blocked: %v", err)
	}
	for name, set := range map[string]func(){
		"automation":      func() { window.running.Store(true) },
		"job action":      func() { window.managingJob.Store(true) },
		"connection test": func() { window.testingServer.Store(true) },
	} {
		window.running.Store(false)
		window.managingJob.Store(false)
		window.testingServer.Store(false)
		set()
		if err := window.serverAdministrationPrecondition(); err == nil {
			t.Fatalf("%s did not block server administration", name)
		}
	}
}

func TestSelectedServerPresetMustStillBeAdvertised(t *testing.T) {
	window := &Window{capabilities: capabilities.Model{ServerPresets: []fiery.ServerPreset{{ID: "P-1", Name: "Production"}}}}
	window.serverPresetGroup.Value = "P-1"
	preset, err := window.selectedServerPreset(window.capabilities)
	if err != nil || preset == nil || preset.ID != "P-1" {
		t.Fatalf("preset=%#v err=%v", preset, err)
	}
	window.serverPresetGroup.Value = "STALE"
	if _, err := window.selectedServerPreset(window.capabilities); err == nil {
		t.Fatal("stale server preset unexpectedly accepted")
	}
}

func TestOverviewLabelsAndServerFormatting(t *testing.T) {
	if got := capabilityActionLabel(false); got != "Get Capabilities" {
		t.Fatalf("initial capability action = %q", got)
	}
	if got := capabilityActionLabel(true); got != "Refresh Capabilities" {
		t.Fatalf("loaded capability action = %q", got)
	}
	if got := formatServerUptime(2*24*60*60 + 3*60*60 + 4*60); got != "2d 3h 4m" {
		t.Fatalf("uptime = %q", got)
	}
	if state, _ := effectiveOverviewServerState("Idle", "API running", false); state != "Idle" {
		t.Fatalf("idle API state = %q", state)
	}
	if state, detail := effectiveOverviewServerState("Idle", "API running", true); state != "Busy" || !strings.Contains(detail, "automation active") {
		t.Fatalf("active automation state = %q detail=%q", state, detail)
	}
	workload := fiery.JobWorkloadSummary{TotalItems: 638, ActiveJobs: 4, EvidenceStatus: "ripping", EvidenceState: "processing"}
	if state, detail := effectiveOverviewServerStateWithJobs("Idle", "API running", workload); state != "Busy" || !strings.Contains(detail, "4 active job") || !strings.Contains(detail, "ripping/processing") {
		t.Fatalf("external Fiery workload state = %q detail=%q", state, detail)
	}
	if overviewStatusPollInterval != time.Second || overviewJobPollInterval != 2*time.Second || overviewJobProbeLimit != 64 {
		t.Fatalf("overview polling intervals status=%s jobs=%s limit=%d", overviewStatusPollInterval, overviewJobPollInterval, overviewJobProbeLimit)
	}
	window := &Window{}
	window.running.Store(true)
	window.captureActive = true
	if window.jobAutomationActive() {
		t.Fatal("capability capture was treated as job processing")
	}
	window.captureActive = false
	if !window.jobAutomationActive() {
		t.Fatal("active job automation was not detected")
	}
}

func TestInputLimitsProtectAutomationResources(t *testing.T) {
	if maxWorkerCount != 10 {
		t.Fatalf("maximum parallel jobs = %d, want 10", maxWorkerCount)
	}
	if got := parseWorkerCount("999999"); got != maxWorkerCount {
		t.Fatalf("worker count = %d, want %d", got, maxWorkerCount)
	}
	if got := parseWorkerCount("invalid"); got != 1 {
		t.Fatalf("invalid worker count = %d", got)
	}
	if got := effectiveWorkerCount(1000, 7); got != 7 {
		t.Fatalf("effective worker count = %d, want 7 for 7 planned jobs", got)
	}
	if got := effectiveWorkerCount(1000, 5000); got != maxWorkerCount {
		t.Fatalf("effective worker count = %d, want maximum %d", got, maxWorkerCount)
	}
	if got := parseCaseLimit("999999999"); got != maxCaseLimit {
		t.Fatalf("case limit = %d, want %d", got, maxCaseLimit)
	}
	if got := parseCaseLimit(""); got != defaultCaseLimit {
		t.Fatalf("default case limit = %d, want %d", got, defaultCaseLimit)
	}
}

func TestGroupSelectionSelectsAndClearsEveryValueExceptCopies(t *testing.T) {
	window := &Window{
		selected:     map[string]map[string]*widget.Bool{},
		groupChecks:  map[string]*widget.Bool{},
		optionChecks: map[string]*widget.Bool{},
	}
	group := capabilities.OptionGroup{Name: "Print", Options: []capabilities.Option{
		{ID: "EFResolution", Values: []string{"360x360dpi", "360x720dpi"}},
		{ID: "EFColorMode", Values: []string{"CMYK", "Grayscale"}},
		{ID: "num copies", Values: []string{"1"}},
	}}
	window.setGroupSelection(group, true)
	if all, count := window.groupSelectionState(group); !all || count != 4 {
		t.Fatalf("selected state all=%v count=%d, want true/4", all, count)
	}
	if _, exists := window.selected["num copies"]; exists {
		t.Fatal("group selection must not replace the Copies text field")
	}
	window.setGroupSelection(group, false)
	if all, count := window.groupSelectionState(group); all || count != 4 {
		t.Fatalf("cleared state all=%v count=%d, want false/4", all, count)
	}
	window.setOptionSelection("EFResolution", []string{"360x360dpi", "360x720dpi"}, true)
	if !window.selected["EFResolution"]["360x360dpi"].Value || !window.selected["EFResolution"]["360x720dpi"].Value {
		t.Fatal("option-level Select all did not select every Resolution value")
	}
	if window.selected["EFColorMode"]["CMYK"].Value {
		t.Fatal("option-level Select all changed another capability")
	}
}

func TestResetSelectionsRestoresAutomationDefaults(t *testing.T) {
	window := &Window{
		selected: map[string]map[string]*widget.Bool{
			"EFResolution": {"360x720dpi": &widget.Bool{Value: true}},
		},
		groupChecks:  map[string]*widget.Bool{"Print": &widget.Bool{Value: true}},
		optionChecks: map[string]*widget.Bool{"EFResolution": &widget.Bool{Value: true}},
		strategy:     combinations.StrategyAll,
		modeChecks:   []widget.Bool{{Value: false}, {Value: true}},
	}
	window.copiesInput.SetText("5-10")
	window.pageRangeInput.SetText("1,3-5")
	window.workers.SetText("1000")
	window.maxCases.SetText("999")
	window.fileModeGroup.Value = "single"
	window.jobActionID.SetText("JOB-123")
	window.adminConfirmation.SetText(clearAllJobsConfirmation)
	window.serverPresetGroup.Value = "SERVER-PRESET-1"
	window.resetSelections()
	if window.selected["EFResolution"]["360x720dpi"].Value || window.groupChecks["Print"].Value || window.optionChecks["EFResolution"].Value {
		t.Fatal("checkbox selections were not cleared")
	}
	if window.strategy != combinations.StrategySingle || window.valueSource != valueSourceSelected || window.testIntent != testIntentPositive || window.constraintMode != constraintValidationOnly || window.copiesInput.Text() != "1" || window.pageRangeInput.Text() != "" || window.workers.Text() != "1" || window.maxCases.Text() != "100" {
		t.Fatalf("defaults not restored: strategy=%s copies=%q pageRange=%q workers=%q cases=%q", window.strategy, window.copiesInput.Text(), window.pageRangeInput.Text(), window.workers.Text(), window.maxCases.Text())
	}
	if window.fileModeGroup.Value != "all" || !window.modeChecks[0].Value || window.modeChecks[1].Value || window.jobActionID.Text() != "" {
		t.Fatal("file mode, run modes, or job ID were not reset")
	}
	if window.serverPresetGroup.Value != noServerPresetID || window.adminConfirmation.Text() != "" {
		t.Fatal("server preset or destructive confirmation was not reset")
	}
}

func TestOptionValuesReturnsEveryServerAdvertisedValue(t *testing.T) {
	values := make([]string, 25)
	for index := range values {
		values[index] = string(rune('A' + index))
	}
	option := capabilities.Option{ID: "Example", Values: values}
	got := optionValues(option)
	if len(got) != 25 {
		t.Fatalf("displayed option values = %d, want all 25", len(got))
	}
	for index := range values {
		if got[index] != values[index] {
			t.Fatalf("value %d = %q, want %q", index, got[index], values[index])
		}
	}
}

func TestLoadPresetRestoresSafeSettingsAndPreservesCredentials(t *testing.T) {
	window := &Window{
		selected: map[string]map[string]*widget.Bool{}, numericInputs: map[string]*widget.Editor{},
		groupChecks: map[string]*widget.Bool{}, optionChecks: map[string]*widget.Bool{},
		capabilities: capabilities.Model{SerialNumber: "SERVER-1", ServerPresets: []fiery.ServerPreset{{ID: "SERVER-PRESET-1", Name: "Press Ready"}}, Options: []capabilities.Option{
			{ID: "EFColorMode", Values: []string{"CMYK", "Grayscale"}},
			{ID: "num copies", Range: &capabilities.NumericRange{Min: 1, Max: 9999, Increment: 1}},
			{ID: "Scaling", Range: &capabilities.NumericRange{Min: 25, Max: 400, Increment: 1}},
			{ID: pageRangeOptionID, Values: []string{"All", "Odd", "Even", pageRangeRangeValue}},
		}},
		presetList: []presets.Preset{{
			Name: "Production", ServerSerial: "SERVER-1", ServerPresetID: "SERVER-PRESET-1",
			SelectedValues: map[string][]string{"EFColorMode": {"CMYK"}},
			NumericInputs:  map[string]string{"num copies": "5-7", "Scaling": "100", pageRangeOptionID: "1,3-5"},
			Strategy:       "pairwise", ValueSource: "advertised", TestIntent: "constraint", ConstraintMode: "validation",
			MaxCases: "250", ParallelJobs: "8", RunModes: []string{"Process and Hold"}, FileMode: "random",
		}},
		modeChecks: make([]widget.Bool, len(runModes)),
	}
	window.serverIP.SetText("server.example")
	window.secretKey.SetText("secret-not-saved-in-preset")
	window.password.SetText("password-not-saved-in-preset")
	window.presetName.SetText("Production")
	window.loadNamedPreset()
	if !window.selected["EFColorMode"]["CMYK"].Value || window.copiesInput.Text() != "5-7" || window.numericInputs["Scaling"].Text() != "100" || window.pageRangeInput.Text() != "1,3-5" {
		t.Fatalf("preset values were not restored: selected=%#v copies=%q scale=%q pageRange=%q", window.selected, window.copiesInput.Text(), window.numericInputs["Scaling"].Text(), window.pageRangeInput.Text())
	}
	if window.strategy != combinations.StrategyPairwise || window.valueSource != valueSourceAdvertised || window.testIntent != testIntentConstraint || window.constraintMode != constraintValidationOnly || window.maxCases.Text() != "250" || window.workers.Text() != "8" || window.fileModeGroup.Value != "random" || !window.modeChecks[1].Value {
		t.Fatal("preset execution controls were not restored")
	}
	if window.serverPresetGroup.Value != "SERVER-PRESET-1" {
		t.Fatalf("server preset selection = %q", window.serverPresetGroup.Value)
	}
	if window.serverIP.Text() != "server.example" || window.secretKey.Text() != "secret-not-saved-in-preset" || window.password.Text() != "password-not-saved-in-preset" {
		t.Fatal("loading a preset changed connection credentials")
	}

	// Preserve compatibility with local presets saved by the old companion-field
	// implementation, but migrate their value into the direct EFPageRange input.
	window.presetList[0].NumericInputs = map[string]string{pageRangeLegacyDataID: "2-4"}
	window.loadNamedPreset()
	if window.pageRangeInput.Text() != "2-4" {
		t.Fatalf("legacy custom page-range preset was not migrated: %q", window.pageRangeInput.Text())
	}
}

func TestCustomPageRangeUsesDirectEFPageRangeAndValidatesImportedPageCount(t *testing.T) {
	window := &Window{
		selected: map[string]map[string]*widget.Bool{pageRangeOptionID: {
			"All": {}, "Odd": {Value: true}, "Even": {}, pageRangeRangeValue: {Value: true},
		}},
		capabilities: capabilities.Model{Options: []capabilities.Option{
			{ID: pageRangeOptionID, Label: "Page range", Value: "All", Values: []string{"All", "Odd", "Even", pageRangeRangeValue}},
			{ID: "num copies", Label: "Copies", Value: "1"},
		}},
		strategy: combinations.StrategySelected,
	}
	window.copiesInput.SetText("1")
	window.pageRangeInput.SetText("1,3,5-7")
	generated, _, err := window.selectedCombinations()
	if err != nil {
		t.Fatal(err)
	}
	if len(generated) != 1 {
		t.Fatalf("generated combinations = %#v", generated)
	}
	attributes := combinationToAttributes(generated[0])
	if attributes[pageRangeOptionID] != "1,3,5-7" {
		t.Fatalf("custom page-range did not replace checked exact values: %#v", attributes)
	}
	if _, exists := attributes[pageRangeLegacyDataID]; exists {
		t.Fatalf("legacy page-range companion was synthesized: %#v", attributes)
	}
	constraintValues := combinationForConstraintValidation(generated[0])
	if constraintValues[pageRangeOptionID] != "1,3,5-7" {
		t.Fatalf("custom page-range constraint value = %#v", constraintValues)
	}
	if err := validateCustomPageRange(attributes, map[string]string{"OrigPageCount": "7"}); err != nil {
		t.Fatalf("valid page range failed: %v", err)
	}
	if err := validateCustomPageRange(attributes, map[string]string{"OrigPageCount": "6"}); err == nil {
		t.Fatal("page range beyond the imported file unexpectedly passed")
	}
	if values := checkboxOptionValues(window.capabilities.Options[0]); len(values) != 4 || !containsStringFold(values, pageRangeRangeValue) {
		t.Fatalf("server-advertised Range1 was not retained as an ordinary option: %v", values)
	}
	if !window.attributesMatch(map[string]string{pageRangeOptionID: "1,3,5,6,7", pageRangeLegacyDataID: ""}, attributes) {
		t.Fatal("semantic direct EFPageRange readback did not match")
	}
	if window.attributesMatch(map[string]string{pageRangeOptionID: "", pageRangeLegacyDataID: "1,3,5,6,7"}, attributes) {
		t.Fatal("legacy DPP_PAGE_RANGE-only readback incorrectly matched")
	}
	if window.attributesMatch(map[string]string{pageRangeOptionID: "1-2", pageRangeLegacyDataID: "1,3,5-7"}, attributes) {
		t.Fatal("wrong direct EFPageRange was hidden by the legacy companion")
	}
	readback := selectedReadbackValues(map[string]string{pageRangeOptionID: "1,3,5-7", pageRangeLegacyDataID: ""}, attributes)
	if readback[pageRangeOptionID] != "1,3,5-7" {
		t.Fatalf("materialized page-range result = %#v", readback)
	}
	if _, exists := readback[pageRangeLegacyDataID]; exists {
		t.Fatalf("legacy page-range value was materialized: %#v", readback)
	}
	withStaleCompanion := combinationToAttributes(combinations.Combination{
		pageRangeOptionID:     pageRangeInternalPrefix + "5-10",
		pageRangeLegacyDataID: "stale-value",
	})
	if withStaleCompanion[pageRangeOptionID] != "5-10" {
		t.Fatalf("direct custom range = %#v", withStaleCompanion)
	}
	if _, exists := withStaleCompanion[pageRangeLegacyDataID]; exists {
		t.Fatalf("stale legacy companion survived serialization: %#v", withStaleCompanion)
	}
}

func TestCustomPageRangeRequiresRangeCapableEFPageRange(t *testing.T) {
	window := &Window{
		selected: map[string]map[string]*widget.Bool{pageRangeOptionID: {
			"All": {}, "Odd": {}, "Even": {},
		}},
		capabilities: capabilities.Model{Options: []capabilities.Option{
			{ID: pageRangeOptionID, Values: []string{"All", "Odd", "Even"}},
			{ID: "num copies", Value: "1"},
		}},
		strategy: combinations.StrategySelected,
	}
	window.copiesInput.SetText("1")
	window.pageRangeInput.SetText("1-5")
	if _, _, err := window.selectedCombinations(); err == nil || !strings.Contains(err.Error(), pageRangeOptionID) {
		t.Fatalf("unsupported custom page range error = %v", err)
	}
}

func TestAdvertisedRange1RemainsAnExactIndependentValue(t *testing.T) {
	attributes := combinationToAttributes(combinations.Combination{pageRangeOptionID: pageRangeRangeValue})
	if attributes[pageRangeOptionID] != pageRangeRangeValue {
		t.Fatalf("fixed Range1 attribute = %#v", attributes)
	}
	if _, exists := attributes[pageRangeLegacyDataID]; exists {
		t.Fatalf("legacy companion was synthesized: %#v", attributes)
	}
	window := &Window{}
	if window.expectedAttributeMatches(map[string]string{pageRangeLegacyDataID: "1-5"}, attributes, pageRangeOptionID, pageRangeRangeValue) {
		t.Fatal("fixed Range1 was accepted from an unrelated DPP_PAGE_RANGE expression")
	}
}

func TestOutputProfilePreservesWireIdentityAndNormalizesOnlyDisplayAndComparison(t *testing.T) {
	window := &Window{capabilities: capabilities.Model{Options: []capabilities.Option{{ID: outputProfileOptionID, Value: "DEFAULT_MEDIA"}}}}
	visible := "360x360 CMYKOV v3F - Fiery Edge - Vela"
	wire := "\ufeff" + visible
	if got := combinationToAttributes(combinations.Combination{outputProfileOptionID: wire}); got[outputProfileOptionID] != wire {
		t.Fatalf("output profile wire value = %q, want exact advertised %q", got[outputProfileOptionID], wire)
	}
	if got := displayOptionValue(outputProfileOptionID, wire); got != visible {
		t.Fatalf("output profile display value = %q, want %q", got, visible)
	}
	if !window.attributeValueMatches(outputProfileOptionID, wire, visible) {
		t.Fatal("output-profile readback with U+FEFF did not match the visible advertised value")
	}
	if !optionValueMatches(outputProfileOptionID, wire, visible) {
		t.Fatal("legacy BOM-less preset value did not match the exact advertised profile")
	}
	if window.attributeValueMatches(outputProfileOptionID, visible+" changed", visible) {
		t.Fatal("different output-profile names unexpectedly matched")
	}
	if !window.attributeValueMatches(outputProfileOptionID, "", "DEFAULT_MEDIA") {
		t.Fatal("omitted default output-profile readback did not retain default-value semantics")
	}
}

func TestImportedFilePageCountPrefersOriginalDocumentCount(t *testing.T) {
	count, ok := importedFilePageCount(map[string]string{"OrigPageCount": "10", "num pages": "12"})
	if !ok || count != 10 {
		t.Fatalf("page count = %d, %t; want original count 10", count, ok)
	}
	if _, ok := importedFilePageCount(map[string]string{"num pages": "0"}); ok {
		t.Fatal("zero page count unexpectedly accepted")
	}
}

func TestNumericRangeInputFeedsCombinationGeneration(t *testing.T) {
	window := &Window{
		strategy:      combinations.StrategySelected,
		selected:      map[string]map[string]*widget.Bool{},
		numericInputs: map[string]*widget.Editor{},
		capabilities: capabilities.Model{Options: []capabilities.Option{{
			ID: "EFMediaThickness", Label: "Media thickness", Group: "fppapersource", Value: "1",
			Range: &capabilities.NumericRange{Min: 1, Max: 10, Increment: 1, Precision: 0},
		}}},
	}
	window.maxCases.SetText("100")
	window.numericInput("EFMediaThickness").SetText("2-4")
	got, axes, err := window.selectedCombinations()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 || len(axes) != 1 {
		t.Fatalf("combinations=%#v axes=%#v", got, axes)
	}
	seen := map[string]bool{}
	for _, combination := range got {
		seen[combination["EFMediaThickness"]] = true
	}
	for _, value := range []string{"2", "3", "4"} {
		if !seen[value] {
			t.Fatalf("missing numeric range value %s in %#v", value, got)
		}
	}
}

func TestSelectedCombinationsSkipsPublishedConstraintConflicts(t *testing.T) {
	window := &Window{
		strategy: combinations.StrategySelected,
		selected: map[string]map[string]*widget.Bool{
			"EFResolution":   {"360x720dpi": &widget.Bool{Value: true}},
			"EFEdgeDropSize": {"None": &widget.Bool{Value: true}, "0_1_2_2_2": &widget.Bool{Value: true}},
		},
		capabilities: capabilities.Model{Options: []capabilities.Option{
			{ID: "EFResolution", Values: []string{"360x720dpi"}, Constraints: capabilities.Constraints{"360x720dpi": {"EFEdgeDropSize": {"0_1_2_2_2"}}}},
			{ID: "EFEdgeDropSize", Values: []string{"None", "0_1_2_2_2"}},
		}},
	}
	window.maxCases.SetText("100")
	got, _, err := window.selectedCombinations()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["EFEdgeDropSize"] != "None" || window.constraintSkipped != 1 {
		t.Fatalf("combinations=%#v skipped=%d warning=%q", got, window.constraintSkipped, window.constraintWarning)
	}
}

func TestAutomationValueSourcesAreIndependentFromGenerationStrategy(t *testing.T) {
	window := &Window{
		strategy: combinations.StrategyAll,
		selected: map[string]map[string]*widget.Bool{
			"EFColorMode": {"CMYK": &widget.Bool{Value: true}},
		},
		capabilities: capabilities.Model{Options: []capabilities.Option{
			{ID: "EFColorMode", Value: "Grayscale", Values: []string{"CMYK", "Grayscale"}},
			{ID: "num copies", Value: "1"},
		}},
	}
	window.copiesInput.SetText("1")
	window.maxCases.SetText("100")

	window.valueSource = valueSourceSelected
	selected, _, err := window.selectedCombinations()
	if err != nil || len(selected) != 1 || selected[0]["EFColorMode"] != "CMYK" {
		t.Fatalf("user-selected plan=%#v err=%v", selected, err)
	}
	window.valueSource = valueSourceDefaults
	defaults, _, err := window.selectedCombinations()
	if err != nil || len(defaults) != 1 || defaults[0]["EFColorMode"] != "Grayscale" {
		t.Fatalf("advertised-default plan=%#v err=%v", defaults, err)
	}
	window.valueSource = valueSourceAdvertised
	advertised, _, err := window.selectedCombinations()
	if err != nil || len(advertised) != 2 {
		t.Fatalf("all-advertised plan=%#v err=%v", advertised, err)
	}
	window.valueSource = valueSourceBaseline
	baseline, axes, err := window.selectedCombinations()
	if err != nil || len(baseline) != 1 || len(baseline[0]) != 0 || len(axes) != 0 {
		t.Fatalf("baseline plan=%#v axes=%#v err=%v", baseline, axes, err)
	}
}

func TestConstraintIntentKeepsOnlyPublishedConflicts(t *testing.T) {
	window := &Window{
		strategy:    combinations.StrategyAll,
		valueSource: valueSourceSelected,
		testIntent:  testIntentConstraint,
		selected: map[string]map[string]*widget.Bool{
			"EFResolution":   {"360x720dpi": &widget.Bool{Value: true}},
			"EFEdgeDropSize": {"None": &widget.Bool{Value: true}, "0_1_2_2_2": &widget.Bool{Value: true}},
		},
		capabilities: capabilities.Model{Options: []capabilities.Option{
			{ID: "EFResolution", Values: []string{"360x720dpi"}, Constraints: capabilities.Constraints{"360x720dpi": {"EFEdgeDropSize": {"0_1_2_2_2"}}}},
			{ID: "EFEdgeDropSize", Values: []string{"None", "0_1_2_2_2"}},
		}},
	}
	window.maxCases.SetText("100")
	got, _, err := window.selectedCombinations()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["EFEdgeDropSize"] != "0_1_2_2_2" || window.constraintWarning == "" {
		t.Fatalf("constraint cases=%#v skipped=%d warning=%q", got, window.constraintSkipped, window.constraintWarning)
	}
}

func TestExpectedConstraintRejectionClassificationRejectsOperationalErrors(t *testing.T) {
	if !expectedConstraintRejection(errors.New("HTTP 422 invalid constraint combination")) {
		t.Fatal("explicit HTTP 422 constraint rejection was not accepted")
	}
	for _, message := range []string{
		"HTTP 500 constraint service crashed",
		"HTTP 404 endpoint not found",
		"HTTP 400 unrelated bad request",
		"HTTP 400 invalid JSON payload",
		"context deadline exceeded",
	} {
		if expectedConstraintRejection(errors.New(message)) {
			t.Fatalf("operational/unrelated error was treated as expected rejection: %s", message)
		}
	}
}

func TestLifecyclePolicyRequiresProcessedRasterOnlyForProcessingModes(t *testing.T) {
	if !lifecyclePolicy(runModes[1]).RequireProcessedRaster || !lifecyclePolicy(runModes[2]).RequireProcessedRaster {
		t.Fatal("Process and Hold and RIP must require processed raster evidence")
	}
	if lifecyclePolicy(runModes[0]).RequireProcessedRaster {
		t.Fatal("Hold must not require processed raster evidence")
	}
	if !lifecyclePolicy(runModes[5]).RequirePrinted || !lifecyclePolicy(runModes[6]).ExpectCanceled {
		t.Fatal("print/cancel lifecycle policies are incorrect")
	}
}

func TestCopiesInputFeedsEveryGenerationStrategy(t *testing.T) {
	for _, strategy := range []combinations.Strategy{combinations.StrategySelected, combinations.StrategyAll, combinations.StrategyPairwise} {
		t.Run(string(strategy), func(t *testing.T) {
			window := copiesTestWindow("1, 5, 10, 15", strategy)
			got, axes, err := window.selectedCombinations()
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 4 || len(axes) != 1 {
				t.Fatalf("combinations=%#v axes=%#v", got, axes)
			}
			seen := map[string]bool{}
			for _, combination := range got {
				seen[combination["num copies"]] = true
			}
			for _, value := range []string{"1", "5", "10", "15"} {
				if !seen[value] {
					t.Fatalf("copies value %s was not generated: %#v", value, got)
				}
			}
		})
	}
}

func TestSingleCopiesValueAppliesToEveryCombination(t *testing.T) {
	window := copiesTestWindow("999", combinations.StrategySelected)
	got, _, err := window.selectedCombinations()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0]["num copies"] != "999" {
		t.Fatalf("combinations = %#v", got)
	}
}

func TestCopiesRangeExpandsInclusively(t *testing.T) {
	window := copiesTestWindow("5 to 10", combinations.StrategySelected)
	got, _, err := window.selectedCombinations()
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, combination := range got {
		seen[combination["num copies"]] = true
	}
	for value := 5; value <= 10; value++ {
		if !seen[strconv.Itoa(value)] {
			t.Fatalf("copies value %d was not generated: %#v", value, got)
		}
	}
}

func TestCopiesRangeRespectsMaxCasesWithoutChangingIt(t *testing.T) {
	window := copiesTestWindow("1-1000", combinations.StrategySelected)
	got, _, err := window.selectedCombinations()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 100 {
		t.Fatalf("generated combinations = %d, want Max cases limit 100", len(got))
	}
	if window.maxCases.Text() != "100" {
		t.Fatalf("Max cases changed to %q, want unchanged value 100", window.maxCases.Text())
	}
}

func TestCopiesInputValidationStopsGeneration(t *testing.T) {
	window := copiesTestWindow("10000", combinations.StrategySelected)
	if _, _, err := window.selectedCombinations(); err == nil {
		t.Fatal("out-of-range copies unexpectedly succeeded")
	}
}

func copiesTestWindow(input string, strategy combinations.Strategy) *Window {
	window := &Window{
		strategy: strategy,
		selected: map[string]map[string]*widget.Bool{},
		capabilities: capabilities.Model{Options: []capabilities.Option{{
			ID:    "num copies",
			Label: "Copies",
			Value: "1",
		}}},
	}
	window.copiesInput.SetText(input)
	window.maxCases.SetText("100")
	return window
}

func TestActivelyProcessingJob(t *testing.T) {
	for _, attributes := range []map[string]string{
		{"status": "printing"},
		{"state": "Processing"},
		{"is printing?": "yes"},
		{"is ripping?": "true"},
	} {
		if active, _ := activelyProcessingJob(attributes); !active {
			t.Fatalf("attributes %#v should be active", attributes)
		}
	}
	for _, attributes := range []map[string]string{
		{"status": "done printing"},
		{"state": "held"},
		{"status": "queued for printing"},
		{"is printing?": "no"},
	} {
		if active, _ := activelyProcessingJob(attributes); active {
			t.Fatalf("attributes %#v should not be active", attributes)
		}
	}
}

func TestCancelableJobSupportsThreeFieryScenarios(t *testing.T) {
	for _, attributes := range []map[string]string{
		{"state": "processing"},
		{"status": "ripping"},
		{"is ripping?": "yes"},
		{"status": "waiting to print"},
		{"queued for printing?": "yes"},
		{"job release state": "production"},
		{"status": "printing"},
		{"is printing?": "yes"},
	} {
		if cancelable, _ := cancelableJob(attributes); !cancelable {
			t.Fatalf("attributes %#v should be cancelable", attributes)
		}
	}
	for _, attributes := range []map[string]string{
		{"status": "spooling"},
		{"status": "done ripping"},
		{"status": "done printing"},
		{"state": "held"},
	} {
		if cancelable, _ := cancelableJob(attributes); cancelable {
			t.Fatalf("attributes %#v should not be cancelable", attributes)
		}
	}
}

func TestCancelObservedRequiresCancellationOrNonCompletedStop(t *testing.T) {
	for _, attributes := range []map[string]string{
		{"status": "cancelled"},
		{"recent action": "cancel"},
		{"status": "held", "is printing?": "no"},
	} {
		if !cancelObserved(attributes) {
			t.Fatalf("attributes %#v should acknowledge cancellation", attributes)
		}
	}
	for _, attributes := range []map[string]string{
		{"status": "printing"},
		{"status": "waiting to print"},
		{"status": "done printing"},
		{},
	} {
		if cancelObserved(attributes) {
			t.Fatalf("attributes %#v should not acknowledge cancellation", attributes)
		}
	}
}

func TestShutdownCancelsAndWaitsForBackgroundOperations(t *testing.T) {
	appContext, appCancel := context.WithCancel(context.Background())
	window := &Window{appContext: appContext, appCancel: appCancel}
	started := make(chan struct{})
	stopped := make(chan struct{})
	window.launchBackground("shutdown test", func() {
		close(started)
		<-appContext.Done()
		close(stopped)
	})
	<-started
	if !window.shutdownBackground(time.Second) {
		t.Fatal("background operation did not stop during shutdown")
	}
	select {
	case <-stopped:
	default:
		t.Fatal("background operation was not cancelled")
	}
	var ran atomic.Bool
	window.launchBackground("must not start", func() { ran.Store(true) })
	if ran.Load() {
		t.Fatal("background operation started after shutdown")
	}
}

func TestExcelResultHelpersPreserveSelectedAndReadbackValues(t *testing.T) {
	selected := map[string]string{"EFResolution": "360x720dpi", "EFColorMode": "CMYK"}
	got := map[string]string{"EFResolution": "360x360dpi", "EFColorMode": "CMYK", "unselected": "ignored", "job name": "Fiery Job"}
	readback := selectedReadbackValues(got, selected)
	if len(readback) != 2 || readback["EFResolution"] != "360x360dpi" || readback["EFColorMode"] != "CMYK" {
		t.Fatalf("selected readback = %#v", readback)
	}
	if _, exists := readback["unselected"]; exists {
		t.Fatalf("unselected value was exported: %#v", readback)
	}
	if name := jobNameFromAttributes(got, "fallback.pdf"); name != "Fiery Job" {
		t.Fatalf("job name = %q", name)
	}
	if count := plannedTestCount(2, 3, 4); count != 24 {
		t.Fatalf("planned tests = %d", count)
	}
}

func TestAttributeValueMatchesDiscoveredDefaultWhenFieryOmitsIt(t *testing.T) {
	window := &Window{capabilities: capabilities.Model{Options: []capabilities.Option{{ID: "EFResolution", Value: "360x720dpi"}}}}
	if !window.attributeValueMatches("EFResolution", "", "360x720dpi") {
		t.Fatal("missing Fiery default should match the selected discovered default")
	}
	if window.attributeValueMatches("EFResolution", "360x360dpi", "360x720dpi") {
		t.Fatal("a different explicit readback must not match")
	}
}
