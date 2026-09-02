package appwails

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	core "api-automation/internal/application"
	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/fiery"
	"api-automation/internal/files"
	"api-automation/internal/model"
	"api-automation/internal/presets"
)

type DialogPort interface {
	SelectFolder() (string, error)
	SelectFile() (string, error)
	SelectExcelPath() (string, error)
	Confirm(title, message string) (bool, error)
}

type FileSelection struct {
	FolderPath string `json:"folderPath"`
	FilePath   string `json:"filePath"`
	Mode       string `json:"mode"`
}

type ResolvedFiles struct {
	Files []string `json:"files"`
	Count int      `json:"count"`
}

type PlanningInput struct {
	SelectedValues  map[string][]string `json:"selectedValues,omitempty"`
	NumericInputs   map[string]string   `json:"numericInputs,omitempty"`
	CopiesInput     string              `json:"copiesInput,omitempty"`
	CustomPageRange string              `json:"customPageRange,omitempty"`
	ValueSource     string              `json:"valueSource"`
	Strategy        string              `json:"strategy"`
	TestIntent      string              `json:"testIntent"`
	MaxCases        int                 `json:"maxCases"`
}

type PlanView struct {
	Axes              []PlanAxis          `json:"axes"`
	CombinationCount  int                 `json:"combinationCount"`
	Combinations      []map[string]string `json:"combinations"`
	ConstraintSkipped int                 `json:"constraintSkipped"`
	ConstraintWarning string              `json:"constraintWarning,omitempty"`
	Truncated         bool                `json:"truncated"`
}

type PlanAxis struct {
	ID     string   `json:"id"`
	Values []string `json:"values"`
}

type PresetInput struct {
	Name            string              `json:"name"`
	SelectedValues  map[string][]string `json:"selectedValues,omitempty"`
	NumericInputs   map[string]string   `json:"numericInputs,omitempty"`
	CopiesInput     string              `json:"copiesInput,omitempty"`
	CustomPageRange string              `json:"customPageRange,omitempty"`
	Strategy        string              `json:"strategy"`
	ValueSource     string              `json:"valueSource"`
	TestIntent      string              `json:"testIntent"`
	ConstraintMode  string              `json:"constraintMode"`
	MaxCases        string              `json:"maxCases"`
	ParallelJobs    string              `json:"parallelJobs"`
	RunModeLabels   []string            `json:"runModeLabels,omitempty"`
	FileMode        string              `json:"fileMode"`
	ServerPresetID  string              `json:"serverPresetId,omitempty"`
}

type PresetSummary struct {
	Name      string    `json:"name"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type PresetLoadView struct {
	Name            string              `json:"name"`
	SelectedValues  map[string][]string `json:"selectedValues,omitempty"`
	NumericInputs   map[string]string   `json:"numericInputs,omitempty"`
	CopiesInput     string              `json:"copiesInput,omitempty"`
	CustomPageRange string              `json:"customPageRange,omitempty"`
	Strategy        string              `json:"strategy,omitempty"`
	ValueSource     string              `json:"valueSource,omitempty"`
	TestIntent      string              `json:"testIntent,omitempty"`
	ConstraintMode  string              `json:"constraintMode,omitempty"`
	MaxCases        string              `json:"maxCases,omitempty"`
	ParallelJobs    string              `json:"parallelJobs,omitempty"`
	FileMode        string              `json:"fileMode,omitempty"`
	RunModeLabels   []string            `json:"runModeLabels,omitempty"`
	ServerPresetID  string              `json:"serverPresetId,omitempty"`
	SkippedCount    int                 `json:"skippedCount"`
	DifferentServer bool                `json:"differentServer"`
}

func (service *Service) SelectTestFolder() (string, error) {
	dialogs, err := service.dialogPort()
	if err != nil {
		return "", err
	}
	return dialogs.SelectFolder()
}

func (service *Service) SelectTestFile() (string, error) {
	dialogs, err := service.dialogPort()
	if err != nil {
		return "", err
	}
	return dialogs.SelectFile()
}

func (service *Service) ResolveTestFiles(selection FileSelection) (ResolvedFiles, error) {
	selected, err := files.Select(model.TestFileSelection{
		FolderPath: strings.TrimSpace(selection.FolderPath), FilePath: strings.TrimSpace(selection.FilePath), Mode: model.FileSelectionMode(strings.TrimSpace(selection.Mode)),
	})
	if err != nil {
		return ResolvedFiles{}, err
	}
	return ResolvedFiles{Files: append([]string(nil), selected...), Count: len(selected)}, nil
}

func (service *Service) PreviewPlan(input PlanningInput) (PlanView, error) {
	capability, err := service.capabilityModel()
	if err != nil {
		return PlanView{}, err
	}
	plan, err := core.BuildPlan(planRequest(capability, input))
	view := planView(plan)
	if err != nil {
		return view, err
	}
	return view, nil
}

func (service *Service) ListPresets() ([]PresetSummary, error) {
	store, err := service.presetStore()
	if err != nil {
		return nil, err
	}
	items, err := store.List()
	if err != nil {
		return nil, err
	}
	result := make([]PresetSummary, 0, len(items))
	for _, item := range items {
		result = append(result, PresetSummary{Name: item.Name, UpdatedAt: item.UpdatedAt})
	}
	return result, nil
}

func (service *Service) SavePreset(input PresetInput) error {
	capability, err := service.capabilityModel()
	if err != nil {
		return err
	}
	store, err := service.presetStore()
	if err != nil {
		return err
	}
	preset := core.BuildSafePreset(capability, core.PresetCaptureRequest{
		Name: input.Name, SelectedValues: input.SelectedValues, NumericInputs: input.NumericInputs,
		CopiesInput: input.CopiesInput, CustomPageRange: input.CustomPageRange,
		Strategy: combinations.Strategy(input.Strategy), ValueSource: core.ValueSource(input.ValueSource), TestIntent: core.TestIntent(input.TestIntent), ConstraintMode: core.ConstraintMode(input.ConstraintMode),
		MaxCases: input.MaxCases, ParallelJobs: input.ParallelJobs, RunModes: runModesByLabel(input.RunModeLabels), FileMode: input.FileMode, ServerPresetID: input.ServerPresetID,
	})
	return store.Save(preset)
}

func (service *Service) LoadPreset(name string) (PresetLoadView, error) {
	capability, err := service.capabilityModel()
	if err != nil {
		return PresetLoadView{}, err
	}
	store, err := service.presetStore()
	if err != nil {
		return PresetLoadView{}, err
	}
	items, err := store.List()
	if err != nil {
		return PresetLoadView{}, err
	}
	var found *presets.Preset
	for index := range items {
		if strings.EqualFold(items[index].Name, strings.TrimSpace(name)) {
			copy := items[index]
			found = &copy
			break
		}
	}
	if found == nil {
		return PresetLoadView{}, fmt.Errorf("preset %q was not found", strings.TrimSpace(name))
	}
	applied := core.ReconcilePreset(capability, *found)
	return PresetLoadView{
		Name: found.Name, SelectedValues: applied.SelectedValues, NumericInputs: applied.NumericInputs,
		CopiesInput: applied.CopiesInput, CustomPageRange: applied.CustomPageRange,
		Strategy: string(applied.Strategy), ValueSource: string(applied.ValueSource), TestIntent: string(applied.TestIntent), ConstraintMode: string(applied.ConstraintMode),
		MaxCases: applied.MaxCases, ParallelJobs: applied.ParallelJobs, FileMode: applied.FileMode, RunModeLabels: applied.RunModeLabels,
		ServerPresetID: applied.ServerPresetID, SkippedCount: applied.Missing, DifferentServer: applied.DifferentServer,
	}, nil
}

func (service *Service) DeletePreset(name string) error {
	store, err := service.presetStore()
	if err != nil {
		return err
	}
	return store.Delete(name)
}

func (service *Service) dialogPort() (DialogPort, error) {
	service.mu.RLock()
	dialogs := service.dialogs
	service.mu.RUnlock()
	if dialogs == nil {
		return nil, errors.New("native dialog service is unavailable")
	}
	return dialogs, nil
}

func (service *Service) capabilityModel() (capabilitiesModel, error) {
	service.mu.RLock()
	capability := cloneCapabilities(service.capabilities)
	service.mu.RUnlock()
	if len(capability.Options) == 0 {
		return capabilitiesModel{}, errors.New("discover capabilities before planning or using presets")
	}
	return capability, nil
}

// Alias keeps workspace helpers concise while retaining the authoritative model type.
type capabilitiesModel = capabilities.Model

func cloneCapabilities(source *capabilities.Model) capabilities.Model {
	if source == nil {
		return capabilities.Model{}
	}
	clone := *source
	clone.Options = append([]capabilities.Option(nil), source.Options...)
	clone.ServerPresets = append([]fiery.ServerPreset(nil), source.ServerPresets...)
	for index := range clone.Options {
		clone.Options[index].Values = append([]string(nil), source.Options[index].Values...)
	}
	return clone
}

func planRequest(capability capabilities.Model, input PlanningInput) core.PlanRequest {
	return core.PlanRequest{
		Capabilities: capability, SelectedValues: cloneStringSlices(input.SelectedValues), NumericInputs: cloneStrings(input.NumericInputs),
		CopiesInput: input.CopiesInput, CustomPageRange: input.CustomPageRange, ValueSource: core.ValueSource(input.ValueSource),
		Strategy: combinations.Strategy(input.Strategy), TestIntent: core.TestIntent(input.TestIntent), MaxCases: input.MaxCases,
	}
}

func planView(plan core.Plan) PlanView {
	const previewLimit = 100
	count := len(plan.Combinations)
	visible := min(count, previewLimit)
	view := PlanView{
		Axes: make([]PlanAxis, len(plan.Axes)), CombinationCount: count, Combinations: make([]map[string]string, visible),
		ConstraintSkipped: plan.ConstraintSkipped, ConstraintWarning: plan.ConstraintWarning, Truncated: count > visible,
	}
	for index, axis := range plan.Axes {
		view.Axes[index] = PlanAxis{ID: axis.Name, Values: append([]string(nil), axis.Values...)}
	}
	for index := range visible {
		view.Combinations[index] = core.CombinationToAttributes(plan.Combinations[index])
	}
	return view
}

func (service *Service) presetStore() (*presets.Store, error) {
	service.mu.Lock()
	defer service.mu.Unlock()
	if service.presets != nil {
		return service.presets, nil
	}
	if service.dataDirectory == "" {
		return nil, errors.New("preview configuration directory is unavailable")
	}
	store, err := presets.New(filepath.Join(service.dataDirectory, "presets.json"))
	if err != nil {
		return nil, err
	}
	service.presets = store
	return store, nil
}

func runModesByLabel(labels []string) []core.RunMode {
	known := make(map[string]core.RunMode)
	for _, mode := range core.RunModes() {
		known[strings.ToLower(mode.Label)] = mode
	}
	result := make([]core.RunMode, 0, len(labels))
	seen := make(map[string]struct{})
	for _, label := range labels {
		mode, ok := known[strings.ToLower(strings.TrimSpace(label))]
		if !ok {
			continue
		}
		if _, duplicate := seen[mode.ID]; duplicate {
			continue
		}
		seen[mode.ID] = struct{}{}
		result = append(result, mode)
	}
	return result
}

func previewDataDirectory() (string, error) {
	config, err := os.UserConfigDir()
	if err != nil {
		return "", fmt.Errorf("locate preview configuration directory: %w", err)
	}
	return filepath.Join(config, "API Automation Wails Preview"), nil
}

func cloneStrings(source map[string]string) map[string]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string]string, len(source))
	for key, value := range source {
		result[key] = value
	}
	return result
}

func cloneStringSlices(source map[string][]string) map[string][]string {
	if len(source) == 0 {
		return nil
	}
	result := make(map[string][]string, len(source))
	keys := make([]string, 0, len(source))
	for key := range source {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		result[key] = append([]string(nil), source[key]...)
	}
	return result
}
