//go:build windows

package appgio

import (
	"context"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"

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

func TestWorkspaceSeparatesResultsAndActivityLogs(t *testing.T) {
	if len(workspacePages) != 4 {
		t.Fatalf("workspace page count = %d, want 4", len(workspacePages))
	}
	if workspacePages[pageResults].NavigationLabel != "Results" {
		t.Fatalf("results page = %#v", workspacePages[pageResults])
	}
	if workspacePages[pageLogs].NavigationLabel != "Activity logs" || pageLogs == pageResults {
		t.Fatalf("logs page is not separate: %#v", workspacePages[pageLogs])
	}

	window := &Window{activePage: pageResults}
	window.list.Position.Offset = 42
	window.setActivePage(pageLogs)
	if window.activePage != pageLogs || window.list.Position.Offset != 0 {
		t.Fatalf("page switch did not reset scrolling: page=%d position=%#v", window.activePage, window.list.Position)
	}
}

func TestInputLimitsProtectAutomationResources(t *testing.T) {
	if maxWorkerCount != 1000 {
		t.Fatalf("maximum parallel jobs = %d, want 1000", maxWorkerCount)
	}
	if got := parseWorkerCount("999999"); got != maxWorkerCount {
		t.Fatalf("worker count = %d, want %d", got, maxWorkerCount)
	}
	if got := parseWorkerCount("invalid"); got != 1 {
		t.Fatalf("invalid worker count = %d", got)
	}
	if got := effectiveWorkerCount(1000, 12); got != 12 {
		t.Fatalf("effective worker count = %d, want 12 for 12 planned jobs", got)
	}
	if got := effectiveWorkerCount(1000, 5000); got != 1000 {
		t.Fatalf("effective worker count = %d, want maximum 1000", got)
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
	window.workers.SetText("1000")
	window.maxCases.SetText("999")
	window.fileModeGroup.Value = "single"
	window.jobActionID.SetText("JOB-123")
	window.resetSelections()
	if window.selected["EFResolution"]["360x720dpi"].Value || window.groupChecks["Print"].Value || window.optionChecks["EFResolution"].Value {
		t.Fatal("checkbox selections were not cleared")
	}
	if window.strategy != combinations.StrategySelected || window.copiesInput.Text() != "1" || window.workers.Text() != "1" || window.maxCases.Text() != "100" {
		t.Fatalf("defaults not restored: strategy=%s copies=%q workers=%q cases=%q", window.strategy, window.copiesInput.Text(), window.workers.Text(), window.maxCases.Text())
	}
	if window.fileModeGroup.Value != "all" || !window.modeChecks[0].Value || window.modeChecks[1].Value || window.jobActionID.Text() != "" {
		t.Fatal("file mode, run modes, or job ID were not reset")
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
