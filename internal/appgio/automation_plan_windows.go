//go:build windows

package appgio

import (
	"context"
	"errors"
	"fmt"
	mathrand "math/rand"
	"sort"
	"strings"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/copyvalues"
	"api-automation/internal/fiery"
	"api-automation/internal/pagevalues"
	"api-automation/internal/rangevalues"
)

// selectedCombinations translates the three independent Automation choices—
// value source, generation strategy, and test intent—into a bounded plan. It
// never infers a writable Fiery attribute from job readback.
func (w *Window) selectedCombinations() ([]combinations.Combination, []combinations.Axis, error) {
	w.mu.Lock()
	capabilityModel := w.capabilities
	w.mu.Unlock()

	source := w.valueSource
	if source == "" { // Backward compatibility for tests and pre-redesign state.
		source = valueSourceSelected
	}
	if source == valueSourceBaseline {
		if w.testIntent == testIntentConstraint {
			return nil, nil, fmt.Errorf("constraint testing requires explicit incompatible Job Property values; Server Baseline sends no property updates")
		}
		w.constraintSkipped = 0
		w.constraintWarning = ""
		return []combinations.Combination{{}}, nil, nil
	}

	included := w.includedPropertyIDs(capabilityModel)
	axes := make([]combinations.Axis, 0, len(included)+1)
	hasRange := false
	for _, id := range included {
		option, ok := capabilityModel.OptionByID(id)
		if !ok || isCopiesOption(id) {
			continue
		}
		values, ranged, err := w.valuesForSource(capabilityModel, option, source)
		if err != nil {
			return nil, nil, err
		}
		hasRange = hasRange || ranged
		if len(values) > 0 {
			axes = append(axes, combinations.Axis{Name: id, Values: values})
		}
	}

	// Defaults and all-advertised values remain useful before the operator has
	// included properties manually. Use the existing bounded preferred suite,
	// never every property on the server in one unsafe ticket.
	if len(axes) == 0 && source != valueSourceSelected {
		for _, preferred := range defaultPermutationAxes(capabilityModel) {
			option, ok := capabilityModel.OptionByID(preferred.Name)
			if !ok {
				continue
			}
			values := preferred.Values
			if source == valueSourceDefaults {
				values = nonEmptyDefault(option)
			}
			if len(values) > 0 {
				axes = append(axes, combinations.Axis{Name: preferred.Name, Values: values})
			}
		}
	}

	if copyOption, ok := copiesOption(capabilityModel); ok {
		values := nonEmptyDefault(copyOption)
		if source != valueSourceDefaults {
			copySelection, err := copyvalues.Parse(w.copiesInput.Text())
			if err != nil {
				return nil, nil, fmt.Errorf("copies: %w", err)
			}
			values = copySelection.Values
			hasRange = hasRange || copySelection.HasRange
		}
		if len(values) == 0 {
			values = []string{"1"}
		}
		axes = append(axes, combinations.Axis{Name: copyOption.ID, Values: values})
	}

	if len(axes) == 0 {
		if w.testIntent == testIntentConstraint {
			return nil, nil, fmt.Errorf("select at least two constrained Job Property values before constraint testing")
		}
		return []combinations.Combination{{}}, nil, nil
	}

	requestedLimit := parseCaseLimit(w.maxCases.Text())
	candidateLimit := requestedLimit
	axisIDs := make(map[string]struct{}, len(axes))
	for _, axis := range axes {
		axisIDs[axis.Name] = struct{}{}
	}
	if capabilities.HasExplicitConstraintDependencies(capabilityModel, axisIDs) {
		candidateLimit = maxCaseLimit
	}

	generationStrategy := w.strategy
	if generationStrategy == "" {
		generationStrategy = combinations.StrategySingle
	}
	if generationStrategy == combinations.StrategySelected {
		generationStrategy = combinations.StrategyAll
	}
	if hasRange && generationStrategy == combinations.StrategyAll {
		// Sample the full numeric domain directly when a Cartesian product would
		// exceed Max cases; this avoids low-value bias and huge materialization.
		generationStrategy = combinations.StrategyRandom
	}
	generated := combinations.GenerateWithStrategy(axes, generationStrategy, candidateLimit)
	if hasRange && len(generated) > 1 && generationStrategy != combinations.StrategyPairwise {
		mathrand.Shuffle(len(generated), func(left, right int) {
			generated[left], generated[right] = generated[right], generated[left]
		})
	}

	planned := generated[:0]
	w.constraintSkipped = 0
	w.constraintWarning = ""
	for _, combination := range generated {
		conflicts := capabilities.ValidateCombination(capabilityModel, combinationForConstraintValidation(combination))
		if w.testIntent == testIntentConstraint {
			if len(conflicts) == 0 {
				continue
			}
			if w.constraintWarning == "" {
				w.constraintWarning = conflicts[0].Error()
			}
			planned = append(planned, combination)
			continue
		}
		if len(conflicts) > 0 {
			w.constraintSkipped++
			if w.constraintWarning == "" {
				w.constraintWarning = conflicts[0].Error()
			}
			continue
		}
		planned = append(planned, combination)
	}
	if len(planned) == 0 && len(generated) > 0 {
		if w.testIntent == testIntentConstraint {
			return nil, axes, fmt.Errorf("no explicitly incompatible combinations were generated; choose values with a published Fiery dependency conflict")
		}
		return nil, axes, fmt.Errorf("all generated combinations conflict with published Fiery constraints; first conflict: %s", w.constraintWarning)
	}
	if len(planned) > requestedLimit {
		planned = planned[:requestedLimit]
	}
	return planned, axes, nil
}

func (w *Window) includedPropertyIDs(model capabilities.Model) []string {
	ids := make(map[string]struct{})
	for id, values := range w.selected {
		if len(selectedValues(values)) > 0 {
			ids[id] = struct{}{}
		}
	}
	if strings.TrimSpace(w.pageRangeInput.Text()) != "" {
		ids[pageRangeOptionID] = struct{}{}
	}
	for id, input := range w.numericInputs {
		if input != nil && strings.TrimSpace(input.Text()) != "" {
			ids[id] = struct{}{}
		}
	}
	out := make([]string, 0, len(ids))
	for id := range ids {
		if _, exists := model.OptionByID(id); exists {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

func (w *Window) valuesForSource(model capabilities.Model, option capabilities.Option, source automationValueSource) ([]string, bool, error) {
	if source == valueSourceDefaults {
		return nonEmptyDefault(option), false, nil
	}
	if option.Range != nil {
		input := w.numericInputs[option.ID]
		if input == nil || strings.TrimSpace(input.Text()) == "" {
			if source == valueSourceAdvertised {
				return nonEmptyDefault(option), false, nil
			}
			return nil, false, nil
		}
		bounds := rangevalues.Bounds{Min: option.Range.Min, Max: option.Range.Max, Increment: option.Range.Increment, Precision: option.Range.Precision}
		selection, err := rangevalues.Parse(input.Text(), bounds, rangevalues.DefaultExpansionLimit)
		if err != nil {
			return nil, false, fmt.Errorf("%s (%s): %w", option.Label, option.ID, err)
		}
		return selection.Values, selection.HasRange, nil
	}

	if isPageRangeOption(option.ID) {
		customInput := strings.TrimSpace(w.pageRangeInput.Text())
		if customInput != "" {
			if !customPageRangeSupported(model) {
				return nil, false, fmt.Errorf("page range: this Fiery does not advertise a range-capable %s value; arbitrary custom ranges are disabled", pageRangeOptionID)
			}
			selection, err := pagevalues.Parse(customInput, pagevalues.DefaultExpansionLimit)
			if err != nil {
				return nil, false, fmt.Errorf("page range: %w", err)
			}
			// A populated text field is an explicit custom-range request. Do not
			// also plan the bare Range1 enum (or another checked enum): Single
			// Configuration could otherwise send Range1 and RIP the full job while
			// the UI claims that it planned Custom(...).
			return []string{pageRangeInternalPrefix + selection.Normalized}, false, nil
		}
		values := selectedValues(w.selected[option.ID])
		if source == valueSourceAdvertised {
			values = checkboxOptionValues(option)
		}
		sort.Strings(values)
		return values, false, nil
	}

	values := selectedValues(w.selected[option.ID])
	if source == valueSourceAdvertised {
		values = optionValues(option)
	}
	sort.Strings(values)
	return values, false, nil
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
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, serverFailure := range []string{"http 500", "http 502", "http 503", "http 504"} {
		if strings.Contains(message, serverFailure) {
			return false
		}
	}
	clientStatus := strings.Contains(message, "http 400") || strings.Contains(message, "http 409") || strings.Contains(message, "http 422")
	constraintEvidence := strings.Contains(message, "constraint") || strings.Contains(message, "conflict") || strings.Contains(message, "incompat") || strings.Contains(message, "compatible value")
	return clientStatus && constraintEvidence
}

func nonEmptyDefault(option capabilities.Option) []string {
	value := strings.TrimSpace(option.Value)
	if value == "" {
		return nil
	}
	return []string{value}
}
