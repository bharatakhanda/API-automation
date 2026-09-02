//go:build windows

package appgio

import (
	"context"

	"api-automation/internal/application"
	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/fiery"
)

// selectedCombinations snapshots Gio controls into the platform-neutral
// planning request. Gio owns widgets; internal/application owns semantics.
func (w *Window) selectedCombinations() ([]combinations.Combination, []combinations.Axis, error) {
	w.mu.Lock()
	capabilityModel := w.capabilities
	w.mu.Unlock()

	plan, err := application.BuildPlan(w.planningRequest(capabilityModel))
	w.constraintSkipped = plan.ConstraintSkipped
	w.constraintWarning = plan.ConstraintWarning
	if err != nil {
		return nil, plan.Axes, err
	}
	return plan.Combinations, plan.Axes, nil
}

func (w *Window) planningRequest(capabilityModel capabilities.Model) application.PlanRequest {
	selected := make(map[string][]string, len(w.selected))
	for optionID, values := range w.selected {
		selected[optionID] = append([]string(nil), selectedValues(values)...)
	}
	numeric := make(map[string]string, len(w.numericInputs))
	for optionID, input := range w.numericInputs {
		if input != nil {
			numeric[optionID] = input.Text()
		}
	}
	return application.PlanRequest{
		Capabilities:    capabilityModel,
		SelectedValues:  selected,
		NumericInputs:   numeric,
		CopiesInput:     w.copiesInput.Text(),
		CustomPageRange: w.pageRangeInput.Text(),
		ValueSource:     w.valueSource,
		Strategy:        w.strategy,
		TestIntent:      w.testIntent,
		MaxCases:        parseCaseLimit(w.maxCases.Text()),
	}
}

func (w *Window) executeConstraintCase(ctx context.Context, client *fiery.Client, session fiery.Session, jobID string, attributes map[string]string, mode constraintTestMode, spooled map[string]string) (string, string, map[string]string) {
	w.mu.Lock()
	capabilityModel := w.capabilities
	w.mu.Unlock()
	localConflicts := capabilities.ValidateCombination(capabilityModel, combinationForConstraintValidation(attributes))
	if len(localConflicts) == 0 {
		return "ERROR", "constraint test plan lost its published local conflict; no intentionally invalid update was attempted", spooled
	}

	status, detail := "ERROR", "constraint test did not complete"
	if mode == constraintControlledApply {
		w.addLog("Controlled constraint test job %s: sending locally proven incompatible attributes", jobID)
		err := client.UpdateJobAttributes(ctx, session, jobID, attributes)
		switch {
		case err == nil:
			status = "FAIL"
			detail = "controlled constraint apply was accepted; expected a constraint rejection"
		case expectedConstraintRejection(err):
			status = "PASS"
			detail = "controlled constraint apply received the expected client-side constraint rejection: " + short(err.Error(), 500)
		default:
			status = "ERROR"
			detail = "controlled constraint apply failed for an operational or unrelated reason, not an expected constraint rejection: " + short(err.Error(), 500)
		}
	} else {
		w.addLog("Validation-only constraint test job %s: checking incompatible attributes without applying them", jobID)
		check, err := client.CheckJobConstraints(ctx, session, jobID, attributes)
		switch {
		case err != nil:
			status = "ERROR"
			detail = "constraint validation endpoint failed: " + err.Error()
		case !check.Supported:
			status = "ERROR"
			detail = "constraint validation endpoint is unavailable; the expected rejection cannot be proven safely"
		case !check.HasConflicts():
			status = "FAIL"
			detail = "Fiery validation accepted the locally incompatible values; expected a constraint conflict"
		default:
			status = "PASS"
			detail = "Fiery returned the expected constraint conflict without applying the invalid settings: " + formatStringMap(check.Conflicts)
		}
	}

	got, readErr := client.GetJobAttributes(ctx, session, jobID)
	if readErr != nil {
		got = spooled
		detail += "; final held-job readback unavailable: " + short(readErr.Error(), 240)
	}
	if deleteErr := client.DeleteJob(ctx, session, jobID); deleteErr != nil {
		return "ERROR", detail + "; disposable held-job cleanup failed: " + short(deleteErr.Error(), 300), got
	}
	w.addLog("Deleted disposable constraint-test job %s", jobID)
	return status, detail + "; disposable held job deleted", got
}

func expectedConstraintRejection(err error) bool {
	return application.ExpectedConstraintRejection(err)
}
