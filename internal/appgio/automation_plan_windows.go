//go:build windows

package appgio

import (
	"api-automation/internal/application"
	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
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

func expectedConstraintRejection(err error) bool {
	return application.ExpectedConstraintRejection(err)
}
