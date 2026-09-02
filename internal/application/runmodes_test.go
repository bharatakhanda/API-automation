package application

import (
	"math"
	"testing"

	"api-automation/internal/combinations"
)

func TestRunModesPreserveLifecycleContract(t *testing.T) {
	tests := []struct {
		label   string
		actions []string
	}{
		{"Hold", nil},
		{"Process and Hold", []string{"rip"}},
		{"RIP", []string{"rip"}},
		{"Press Print", []string{"rip", "production", "press_print"}},
		{"Ready to Print", []string{"rip", "production"}},
		{"Print", []string{"rip", "production", "press_print", "print"}},
		{"Cancel while Processing/Ripping", []string{"cancel_ripping"}},
		{"Cancel while Waiting to Print", []string{"rip", "production", "cancel_waiting"}},
		{"Cancel while Printing", []string{"rip", "production", "press_print", "cancel_printing"}},
		{"Delete", []string{"delete"}},
	}
	modes := RunModes()
	if len(modes) != len(tests) {
		t.Fatalf("mode count = %d", len(modes))
	}
	for index, want := range tests {
		mode := modes[index]
		if mode.ID == "" || mode.Label != want.label || mode.ImportQueue != "hold" {
			t.Fatalf("mode %d = %#v", index, mode)
		}
		if len(mode.Actions) != len(want.actions) {
			t.Fatalf("mode %q actions = %#v", mode.Label, mode.Actions)
		}
		for actionIndex := range want.actions {
			if mode.Actions[actionIndex] != want.actions[actionIndex] {
				t.Fatalf("mode %q actions = %#v", mode.Label, mode.Actions)
			}
		}
	}

	// Callers receive independent metadata snapshots.
	modes[1].Actions[0] = "changed"
	if RunModes()[1].Actions[0] != "rip" {
		t.Fatal("RunModes exposed mutable package state")
	}
}

func TestLifecycleAndRIPReadbackMetadata(t *testing.T) {
	modes := RunModes()
	if !LifecyclePolicy(modes[1]).RequireProcessedRaster || !LifecyclePolicy(modes[2]).RequireProcessedRaster {
		t.Fatal("processing modes do not require raster evidence")
	}
	if LifecyclePolicy(modes[0]).RequireProcessedRaster {
		t.Fatal("Hold requires raster evidence")
	}
	if !LifecyclePolicy(modes[5]).RequirePrinted || !LifecyclePolicy(modes[6]).ExpectCanceled {
		t.Fatal("print/cancel policies are incorrect")
	}
	if !RunModesIncludeAction(modes, "print") || RunModesIncludeAction(modes[:1], "rip") {
		t.Fatal("run-mode action metadata is incorrect")
	}
	if !CombinationsRequireRIPReadback([]combinations.Combination{{"EFPrintSpeed": "100"}}) {
		t.Fatal("EFPrintSpeed did not require RIP readback")
	}
}

func TestPlanningLimits(t *testing.T) {
	if ParseWorkerCount("999999") != MaximumWorkerCount || ParseWorkerCount("invalid") != DefaultWorkerCount {
		t.Fatal("worker limits are incorrect")
	}
	if EffectiveWorkerCount(1000, 7) != 7 || EffectiveWorkerCount(1000, 5000) != MaximumWorkerCount {
		t.Fatal("effective worker bounds are incorrect")
	}
	if ParseCaseLimit("999999999") != MaximumCaseLimit || ParseCaseLimit("") != DefaultCaseLimit {
		t.Fatal("case limits are incorrect")
	}
	if PlannedTestCount(2, 3, 4) != 24 || PlannedTestCount(1, 0, 4) != 0 {
		t.Fatal("planned test count is incorrect")
	}
	if PlannedTestCount(math.MaxInt, math.MaxInt, math.MaxInt) != math.MaxInt64 {
		t.Fatal("planned test count did not saturate")
	}
}
