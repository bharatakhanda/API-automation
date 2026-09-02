package application

import (
	"fmt"
	mathrand "math/rand"
	"sort"
	"strings"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/copyvalues"
	"api-automation/internal/pagevalues"
	"api-automation/internal/rangevalues"
)

// ValueSource controls which values become planning axes independently from
// the case-generation strategy.
type ValueSource string

const (
	ValueSourceSelected   ValueSource = "selected"
	ValueSourceDefaults   ValueSource = "defaults"
	ValueSourceAdvertised ValueSource = "advertised"
	ValueSourceBaseline   ValueSource = "baseline"
)

// TestIntent distinguishes normal executable cases from deliberately invalid
// combinations derived from Fiery's published constraints.
type TestIntent string

const (
	TestIntentPositive   TestIntent = "positive"
	TestIntentConstraint TestIntent = "constraint"
)

// ConstraintMode controls whether expected constraint rejection stops after
// validation or creates a disposable held job as additional evidence.
type ConstraintMode string

const (
	ConstraintValidationOnly  ConstraintMode = "validation"
	ConstraintControlledApply ConstraintMode = "controlled_apply"
)

// PlanRequest is an immutable planning snapshot suitable for any frontend.
// BuildPlan never mutates the request maps, slices, or capability model.
type PlanRequest struct {
	Capabilities    capabilities.Model
	SelectedValues  map[string][]string
	NumericInputs   map[string]string
	CopiesInput     string
	CustomPageRange string
	ValueSource     ValueSource
	Strategy        combinations.Strategy
	TestIntent      TestIntent
	MaxCases        int
}

// Plan contains generated cases and the evidence needed to explain filtering.
type Plan struct {
	Combinations      []combinations.Combination
	Axes              []combinations.Axis
	ConstraintSkipped int
	ConstraintWarning string
}

// BuildPlan interprets frontend inputs, generates bounded combinations, and
// filters them according to the requested positive or constraint intent.
func BuildPlan(request PlanRequest) (Plan, error) {
	source := normalizeValueSource(request.ValueSource)
	intent := normalizeTestIntent(request.TestIntent)
	if source == ValueSourceBaseline {
		if intent == TestIntentConstraint {
			return Plan{}, fmt.Errorf("constraint testing requires explicit incompatible Job Property values; Server Baseline sends no property updates")
		}
		return Plan{Combinations: []combinations.Combination{{}}}, nil
	}

	included := IncludedPropertyIDs(request.Capabilities, request)
	axes := make([]combinations.Axis, 0, len(included)+1)
	hasRange := false
	for _, id := range included {
		option, ok := request.Capabilities.OptionByID(id)
		if !ok || IsCopiesOption(id) {
			continue
		}
		values, ranged, err := ValuesForSource(request.Capabilities, option, request, source)
		if err != nil {
			return Plan{}, err
		}
		hasRange = hasRange || ranged
		if len(values) > 0 {
			axes = append(axes, combinations.Axis{Name: id, Values: values})
		}
	}

	// Defaults and all-advertised values remain useful before the operator has
	// included properties manually. Use the bounded preferred suite rather than
	// every property on the server in one unsafe ticket.
	if len(axes) == 0 && source != ValueSourceSelected {
		for _, preferred := range DefaultPermutationAxes(request.Capabilities) {
			option, ok := request.Capabilities.OptionByID(preferred.Name)
			if !ok {
				continue
			}
			values := preferred.Values
			if source == ValueSourceDefaults {
				values = nonEmptyDefault(option)
			}
			if len(values) > 0 {
				axes = append(axes, combinations.Axis{Name: preferred.Name, Values: values})
			}
		}
	}

	if copyOption, ok := CopiesOption(request.Capabilities); ok {
		values := nonEmptyDefault(copyOption)
		if source != ValueSourceDefaults {
			selection, err := copyvalues.Parse(request.CopiesInput)
			if err != nil {
				return Plan{}, fmt.Errorf("copies: %w", err)
			}
			values = selection.Values
			hasRange = hasRange || selection.HasRange
		}
		if len(values) == 0 {
			values = []string{"1"}
		}
		axes = append(axes, combinations.Axis{Name: copyOption.ID, Values: values})
	}

	if len(axes) == 0 {
		if intent == TestIntentConstraint {
			return Plan{}, fmt.Errorf("select at least two constrained Job Property values before constraint testing")
		}
		return Plan{Combinations: []combinations.Combination{{}}}, nil
	}

	requestedLimit := NormalizeCaseLimit(request.MaxCases)
	candidateLimit := requestedLimit
	axisIDs := make(map[string]struct{}, len(axes))
	for _, axis := range axes {
		axisIDs[axis.Name] = struct{}{}
	}
	if capabilities.HasExplicitConstraintDependencies(request.Capabilities, axisIDs) {
		candidateLimit = MaximumCaseLimit
	}

	strategy := normalizeStrategy(request.Strategy)
	if strategy == combinations.StrategySelected {
		strategy = combinations.StrategyAll
	}
	if hasRange && strategy == combinations.StrategyAll {
		// Sample the numeric domain directly when a Cartesian product would
		// exceed Max cases, avoiding low-value bias and huge materialization.
		strategy = combinations.StrategyRandom
	}
	generated := combinations.GenerateWithStrategy(axes, strategy, candidateLimit)
	if hasRange && len(generated) > 1 && strategy != combinations.StrategyPairwise {
		mathrand.Shuffle(len(generated), func(left, right int) {
			generated[left], generated[right] = generated[right], generated[left]
		})
	}

	planned := generated[:0]
	skipped := 0
	warning := ""
	for _, combination := range generated {
		conflicts := capabilities.ValidateCombination(request.Capabilities, CombinationForConstraintValidation(combination))
		if intent == TestIntentConstraint {
			if len(conflicts) == 0 {
				continue
			}
			if warning == "" {
				warning = conflicts[0].Error()
			}
			planned = append(planned, combination)
			continue
		}
		if len(conflicts) > 0 {
			skipped++
			if warning == "" {
				warning = conflicts[0].Error()
			}
			continue
		}
		planned = append(planned, combination)
	}
	if len(planned) == 0 && len(generated) > 0 {
		if intent == TestIntentConstraint {
			return Plan{Axes: cloneAxes(axes)}, fmt.Errorf("no explicitly incompatible combinations were generated; choose values with a published Fiery dependency conflict")
		}
		return Plan{Axes: cloneAxes(axes), ConstraintSkipped: skipped, ConstraintWarning: warning}, fmt.Errorf("all generated combinations conflict with published Fiery constraints; first conflict: %s", warning)
	}
	if len(planned) > requestedLimit {
		planned = planned[:requestedLimit]
	}
	return Plan{
		Combinations:      cloneCombinations(planned),
		Axes:              cloneAxes(axes),
		ConstraintSkipped: skipped,
		ConstraintWarning: warning,
	}, nil
}

// IncludedPropertyIDs returns property IDs that have explicit user input. The
// order is stable so frontend DTOs and tests do not depend on map iteration.
func IncludedPropertyIDs(model capabilities.Model, request PlanRequest) []string {
	ids := make(map[string]struct{})
	for optionID, values := range request.SelectedValues {
		if len(selectedOptionValues(values)) > 0 {
			ids[optionID] = struct{}{}
		}
	}
	if strings.TrimSpace(request.CustomPageRange) != "" {
		ids[PageRangeOptionID] = struct{}{}
	}
	for optionID, value := range request.NumericInputs {
		if strings.TrimSpace(value) != "" {
			ids[optionID] = struct{}{}
		}
	}
	result := make([]string, 0, len(ids))
	for id := range ids {
		if _, ok := model.OptionByID(id); ok {
			result = append(result, id)
		}
	}
	sort.Strings(result)
	return result
}

// ValuesForSource interprets one capability option for a planning request.
func ValuesForSource(model capabilities.Model, option capabilities.Option, request PlanRequest, source ValueSource) ([]string, bool, error) {
	if source == ValueSourceDefaults {
		return nonEmptyDefault(option), false, nil
	}
	if option.Range != nil {
		input := strings.TrimSpace(request.NumericInputs[option.ID])
		if input == "" {
			if source == ValueSourceAdvertised {
				return nonEmptyDefault(option), false, nil
			}
			return nil, false, nil
		}
		bounds := rangevalues.Bounds{Min: option.Range.Min, Max: option.Range.Max, Increment: option.Range.Increment, Precision: option.Range.Precision}
		selection, err := rangevalues.Parse(input, bounds, rangevalues.DefaultExpansionLimit)
		if err != nil {
			return nil, false, fmt.Errorf("%s (%s): %w", option.Label, option.ID, err)
		}
		return selection.Values, selection.HasRange, nil
	}

	if IsPageRangeOption(option.ID) {
		customInput := strings.TrimSpace(request.CustomPageRange)
		if customInput != "" {
			if !CustomPageRangeSupported(model) {
				return nil, false, fmt.Errorf("page range: this Fiery does not advertise a range-capable %s value; arbitrary custom ranges are disabled", PageRangeOptionID)
			}
			selection, err := pagevalues.Parse(customInput, pagevalues.DefaultExpansionLimit)
			if err != nil {
				return nil, false, fmt.Errorf("page range: %w", err)
			}
			// A populated text field replaces checked enum values, including bare
			// Range1, so the planned value and wire value cannot diverge.
			return []string{PageRangeInternalPrefix + selection.Normalized}, false, nil
		}
		values := selectedOptionValues(request.SelectedValues[option.ID])
		if source == ValueSourceAdvertised {
			values = CheckboxOptionValues(option)
		}
		sort.Strings(values)
		return values, false, nil
	}

	values := selectedOptionValues(request.SelectedValues[option.ID])
	if source == ValueSourceAdvertised {
		values = OptionValues(option)
	}
	sort.Strings(values)
	return values, false, nil
}

func CustomPageRangeSupported(model capabilities.Model) bool {
	option, exists := model.OptionByID(PageRangeOptionID)
	if !exists {
		return false
	}
	for _, value := range option.Values {
		if strings.EqualFold(strings.TrimSpace(value), PageRangeRangeValue) {
			return true
		}
	}
	return false
}

func IsPageRangeOption(optionID string) bool {
	return strings.EqualFold(strings.TrimSpace(optionID), PageRangeOptionID)
}

func IsCopiesOption(optionID string) bool {
	return optionID == CopiesOptionID || optionID == "EFCopies"
}

func CopiesOption(model capabilities.Model) (capabilities.Option, bool) {
	for _, id := range []string{"EFCopies", CopiesOptionID} {
		if option, ok := model.OptionByID(id); ok {
			return option, true
		}
	}
	return capabilities.Option{}, false
}

func OptionValues(option capabilities.Option) []string {
	if len(option.Values) > 0 {
		return append([]string(nil), option.Values...)
	}
	if strings.TrimSpace(option.Value) != "" {
		return []string{option.Value}
	}
	return nil
}

func CheckboxOptionValues(option capabilities.Option) []string {
	return OptionValues(option)
}

// DefaultPermutationAxes returns the bounded preferred suite used when a
// defaults/all-advertised request has no explicitly included properties.
func DefaultPermutationAxes(model capabilities.Model) []combinations.Axis {
	preferred := []string{"EFResolution", "EFColorMode", "EFMediaType", "EFPrintSpeed", "PageSize", "EFBrightness", "EFPrintCover", "EFOutputBin"}
	axes := make([]combinations.Axis, 0, len(preferred))
	seen := map[string]struct{}{}
	for _, id := range preferred {
		if option, ok := model.OptionByID(id); ok {
			values := OptionValues(option)
			if len(values) > 1 {
				axes = append(axes, combinations.Axis{Name: option.ID, Values: values})
				seen[option.ID] = struct{}{}
			}
		}
	}
	if len(axes) > 0 {
		return axes
	}
	for _, option := range model.Options {
		if IsCopiesOption(option.ID) {
			continue
		}
		if _, ok := seen[option.ID]; ok {
			continue
		}
		values := CheckboxOptionValues(option)
		if len(values) > 1 && len(values) <= 12 && IsLikelyJobAttribute(option) {
			axes = append(axes, combinations.Axis{Name: option.ID, Values: values})
		}
		if len(axes) >= 8 {
			break
		}
	}
	return axes
}

func IsLikelyJobAttribute(option capabilities.Option) bool {
	for _, scope := range option.Scopes {
		scope = strings.ToLower(scope)
		if scope == "command" || scope == "ps" || scope == "appe" || scope == "uimenu" || strings.HasPrefix(scope, "fp") {
			return true
		}
	}
	return false
}

func selectedOptionValues(values []string) []string {
	selected := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		if strings.TrimSpace(value) == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		selected = append(selected, value)
	}
	sort.Strings(selected)
	return selected
}

func nonEmptyDefault(option capabilities.Option) []string {
	value := strings.TrimSpace(option.Value)
	if value == "" {
		return nil
	}
	return []string{value}
}

func normalizeValueSource(source ValueSource) ValueSource {
	switch source {
	case ValueSourceDefaults, ValueSourceAdvertised, ValueSourceBaseline:
		return source
	default:
		return ValueSourceSelected
	}
}

func normalizeTestIntent(intent TestIntent) TestIntent {
	if intent == TestIntentConstraint {
		return intent
	}
	return TestIntentPositive
}

func normalizeStrategy(strategy combinations.Strategy) combinations.Strategy {
	if strategy == "" {
		return combinations.StrategySingle
	}
	return strategy
}

func cloneAxes(source []combinations.Axis) []combinations.Axis {
	out := make([]combinations.Axis, len(source))
	for index, axis := range source {
		out[index] = combinations.Axis{Name: axis.Name, Values: append([]string(nil), axis.Values...)}
	}
	return out
}

func cloneCombinations(source []combinations.Combination) []combinations.Combination {
	out := make([]combinations.Combination, len(source))
	for index, combination := range source {
		out[index] = combinations.Combination(CloneStringMap(combination))
	}
	return out
}
