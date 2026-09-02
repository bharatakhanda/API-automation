package application

import (
	"math"

	"api-automation/internal/combinations"
	"api-automation/internal/joboutcome"
)

// RunMode is platform-neutral lifecycle metadata. ID is stable for frontend
// state while Label retains the existing persisted-preset display contract.
type RunMode struct {
	ID          string
	Label       string
	ImportQueue string
	Actions     []string
}

var standardRunModes = []RunMode{
	{ID: "hold", Label: "Hold", ImportQueue: "hold"},
	{ID: "process-and-hold", Label: "Process and Hold", ImportQueue: "hold", Actions: []string{"rip"}},
	{ID: "rip", Label: "RIP", ImportQueue: "hold", Actions: []string{"rip"}},
	{ID: "press-print", Label: "Press Print", ImportQueue: "hold", Actions: []string{"rip", "production", "press_print"}},
	{ID: "ready-to-print", Label: "Ready to Print", ImportQueue: "hold", Actions: []string{"rip", "production"}},
	{ID: "print", Label: "Print", ImportQueue: "hold", Actions: []string{"rip", "production", "press_print", "print"}},
	{ID: "cancel-ripping", Label: "Cancel while Processing/Ripping", ImportQueue: "hold", Actions: []string{"cancel_ripping"}},
	{ID: "cancel-waiting", Label: "Cancel while Waiting to Print", ImportQueue: "hold", Actions: []string{"rip", "production", "cancel_waiting"}},
	{ID: "cancel-printing", Label: "Cancel while Printing", ImportQueue: "hold", Actions: []string{"rip", "production", "press_print", "cancel_printing"}},
	{ID: "delete", Label: "Delete", ImportQueue: "hold", Actions: []string{"delete"}},
}

func RunModes() []RunMode {
	modes := make([]RunMode, len(standardRunModes))
	for index, mode := range standardRunModes {
		mode.Actions = append([]string(nil), mode.Actions...)
		modes[index] = mode
	}
	return modes
}

func LifecyclePolicy(mode RunMode) joboutcome.Policy {
	policy := joboutcome.Policy{}
	switch mode.Label {
	case "Process and Hold", "RIP":
		policy.RequireProcessedRaster = true
	case "Print":
		policy.RequirePrinted = true
	case "Cancel while Processing/Ripping", "Cancel while Waiting to Print", "Cancel while Printing":
		policy.ExpectCanceled = true
	}
	return policy
}

func ModeIncludesAction(mode RunMode, want string) bool {
	for _, action := range mode.Actions {
		if action == want {
			return true
		}
	}
	return false
}

func RunModesIncludeAction(modes []RunMode, action string) bool {
	for _, mode := range modes {
		if ModeIncludesAction(mode, action) {
			return true
		}
	}
	return false
}

func RequiresRIPReadback(key string) bool {
	switch key {
	case "EFPrintSpeed", "EFRotateDocument":
		return true
	default:
		return false
	}
}

func CombinationsRequireRIPReadback(combinationsToCheck []combinations.Combination) bool {
	for _, combination := range combinationsToCheck {
		for key := range combination {
			if RequiresRIPReadback(key) {
				return true
			}
		}
	}
	return false
}

func RunModeLabels(modes []RunMode) []string {
	labels := make([]string, 0, len(modes))
	for _, mode := range modes {
		labels = append(labels, mode.Label)
	}
	return labels
}

func PlannedTestCount(counts ...int) int64 {
	total := int64(1)
	for _, count := range counts {
		if count <= 0 {
			return 0
		}
		if int64(count) > math.MaxInt64/total {
			return math.MaxInt64
		}
		total *= int64(count)
	}
	return total
}
