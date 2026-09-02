package application

import (
	"sort"
	"strconv"
	"strings"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/copyvalues"
	"api-automation/internal/pagevalues"
	"api-automation/internal/presets"
	"api-automation/internal/rangevalues"
)

type PresetCaptureRequest struct {
	Name            string
	SelectedValues  map[string][]string
	NumericInputs   map[string]string
	CopiesInput     string
	CustomPageRange string
	Strategy        combinations.Strategy
	ValueSource     ValueSource
	TestIntent      TestIntent
	ConstraintMode  ConstraintMode
	MaxCases        string
	ParallelJobs    string
	RunModes        []RunMode
	FileMode        string
	ServerPresetID  string
}

func BuildSafePreset(model capabilities.Model, request PresetCaptureRequest) presets.Preset {
	selected := make(map[string][]string)
	for optionID, values := range request.SelectedValues {
		option, exists := model.OptionByID(optionID)
		if !exists || option.Range != nil {
			continue
		}
		chosen := selectedOptionValues(values)
		if len(chosen) > 0 {
			selected[optionID] = chosen
		}
	}
	numeric := make(map[string]string)
	if option, ok := CopiesOption(model); ok {
		numeric[option.ID] = strings.TrimSpace(request.CopiesInput)
	}
	for optionID, value := range request.NumericInputs {
		option, exists := model.OptionByID(optionID)
		if exists && option.Range != nil && strings.TrimSpace(value) != "" {
			numeric[optionID] = strings.TrimSpace(value)
		}
	}
	if value := strings.TrimSpace(request.CustomPageRange); value != "" && CustomPageRangeSupported(model) {
		numeric[PageRangeOptionID] = value
	}
	return presets.Preset{
		Name: strings.TrimSpace(request.Name), ServerName: model.ServerName, ServerSerial: model.SerialNumber,
		ServerPresetID: request.ServerPresetID, SelectedValues: selected, NumericInputs: numeric,
		Strategy: string(request.Strategy), ValueSource: string(request.ValueSource), TestIntent: string(request.TestIntent), ConstraintMode: string(request.ConstraintMode),
		MaxCases: strings.TrimSpace(request.MaxCases), ParallelJobs: strings.TrimSpace(request.ParallelJobs),
		RunModes: RunModeLabels(request.RunModes), FileMode: request.FileMode,
	}
}

type ReconciledPreset struct {
	Name              string
	SelectedValues    map[string][]string
	NumericInputs     map[string]string
	CopiesInput       string
	CustomPageRange   string
	Strategy          combinations.Strategy
	HasStrategy       bool
	ValueSource       ValueSource
	HasValueSource    bool
	TestIntent        TestIntent
	HasTestIntent     bool
	ConstraintMode    ConstraintMode
	HasConstraintMode bool
	MaxCases          string
	ParallelJobs      string
	FileMode          string
	ServerPresetID    string
	RunModeLabels     []string
	Missing           int
	DifferentServer   bool
}

func ReconcilePreset(model capabilities.Model, preset presets.Preset) ReconciledPreset {
	result := ReconciledPreset{
		Name: preset.Name, SelectedValues: make(map[string][]string), NumericInputs: make(map[string]string),
		DifferentServer: preset.ServerSerial != "" && model.SerialNumber != "" && !strings.EqualFold(preset.ServerSerial, model.SerialNumber),
	}
	for optionID, values := range preset.SelectedValues {
		option, exists := model.OptionByID(optionID)
		if !exists || option.Range != nil {
			result.Missing += len(values)
			continue
		}
		available := CheckboxOptionValues(option)
		for _, saved := range values {
			matched := ""
			for _, current := range available {
				if OptionValueMatches(optionID, current, saved) {
					matched = current
					break
				}
			}
			if matched == "" {
				result.Missing++
				continue
			}
			result.SelectedValues[optionID] = append(result.SelectedValues[optionID], matched)
		}
		sort.Strings(result.SelectedValues[optionID])
	}
	for optionID, value := range preset.NumericInputs {
		value = strings.TrimSpace(value)
		switch {
		case strings.EqualFold(optionID, PageRangeOptionID) || strings.EqualFold(optionID, PageRangeLegacyDataID):
			if !CustomPageRangeSupported(model) {
				result.Missing++
				continue
			}
			selection, err := pagevalues.Parse(value, pagevalues.DefaultExpansionLimit)
			if err != nil {
				result.Missing++
				continue
			}
			result.CustomPageRange = selection.Normalized
		case IsCopiesOption(optionID):
			if _, err := copyvalues.Parse(value); err != nil {
				result.Missing++
				continue
			}
			result.CopiesInput = value
		default:
			option, exists := model.OptionByID(optionID)
			if !exists || option.Range == nil {
				result.Missing++
				continue
			}
			bounds := rangevalues.Bounds{Min: option.Range.Min, Max: option.Range.Max, Increment: option.Range.Increment, Precision: option.Range.Precision}
			if _, err := rangevalues.Parse(value, bounds, rangevalues.DefaultExpansionLimit); err != nil {
				result.Missing++
				continue
			}
			result.NumericInputs[optionID] = value
		}
	}
	if result.CopiesInput == "" {
		result.CopiesInput = "1"
	}

	switch combinations.Strategy(preset.Strategy) {
	case combinations.StrategySingle, combinations.StrategySelected, combinations.StrategyAll, combinations.StrategyPairwise, combinations.StrategyRandom:
		result.Strategy, result.HasStrategy = combinations.Strategy(preset.Strategy), true
	}
	switch ValueSource(preset.ValueSource) {
	case ValueSourceBaseline, ValueSourceDefaults, ValueSourceSelected, ValueSourceAdvertised:
		result.ValueSource, result.HasValueSource = ValueSource(preset.ValueSource), true
	}
	switch TestIntent(preset.TestIntent) {
	case TestIntentPositive, TestIntentConstraint:
		result.TestIntent, result.HasTestIntent = TestIntent(preset.TestIntent), true
	}
	switch ConstraintMode(preset.ConstraintMode) {
	case ConstraintValidationOnly, ConstraintControlledApply:
		result.ConstraintMode, result.HasConstraintMode = ConstraintMode(preset.ConstraintMode), true
	}
	if value := strings.TrimSpace(preset.MaxCases); value != "" {
		result.MaxCases = strconv.Itoa(ParseCaseLimit(value))
	}
	if value := strings.TrimSpace(preset.ParallelJobs); value != "" {
		result.ParallelJobs = strconv.Itoa(ParseWorkerCount(value))
	}
	if preset.FileMode == "all" || preset.FileMode == "single" || preset.FileMode == "random" {
		result.FileMode = preset.FileMode
	}
	if preset.ServerPresetID != "" {
		for _, serverPreset := range model.ServerPresets {
			if serverPreset.ID == preset.ServerPresetID {
				result.ServerPresetID = serverPreset.ID
				break
			}
		}
		if result.ServerPresetID == "" {
			result.Missing++
		}
	}
	knownModes := make(map[string]string)
	for _, mode := range RunModes() {
		knownModes[strings.ToLower(mode.Label)] = mode.Label
	}
	seenModes := make(map[string]struct{})
	for _, label := range preset.RunModes {
		canonical, ok := knownModes[strings.ToLower(strings.TrimSpace(label))]
		if !ok {
			result.Missing++
			continue
		}
		if _, exists := seenModes[canonical]; !exists {
			seenModes[canonical] = struct{}{}
			result.RunModeLabels = append(result.RunModeLabels, canonical)
		}
	}
	if len(result.RunModeLabels) == 0 {
		result.RunModeLabels = []string{RunModes()[0].Label}
	}
	return result
}
