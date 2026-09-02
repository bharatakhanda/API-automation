//go:build windows

package appgio

import (
	"fmt"
	"strings"

	"api-automation/internal/application"
	"api-automation/internal/presets"
)

func (w *Window) saveCurrentPreset() {
	if w.running.Load() || w.managingServer.Load() || w.inspectingJobs.Load() {
		w.setStatus("Wait for the active operation to finish before saving a preset.")
		return
	}
	if w.presetStore == nil {
		w.setStatus("Preset storage is unavailable on this computer.")
		return
	}
	name := strings.TrimSpace(w.presetName.Text())
	if _, exists := w.findPreset(name); exists {
		confirmed, err := confirmDestructiveAction("Replace settings preset", fmt.Sprintf("Replace existing local preset %q with the current selections?", name))
		if err != nil {
			w.setStatus("Preset confirmation failed: " + err.Error())
			return
		}
		if !confirmed {
			return
		}
	}
	w.mu.Lock()
	model := w.capabilities
	w.mu.Unlock()
	selected := make(map[string][]string, len(w.selected))
	for optionID, values := range w.selected {
		selected[optionID] = selectedValues(values)
	}
	numeric := make(map[string]string, len(w.numericInputs))
	for optionID, input := range w.numericInputs {
		if input != nil {
			numeric[optionID] = input.Text()
		}
	}
	serverPresetID := ""
	if selectedPreset, err := w.selectedServerPreset(model); err == nil && selectedPreset != nil {
		serverPresetID = selectedPreset.ID
	}
	preset := application.BuildSafePreset(model, application.PresetCaptureRequest{
		Name: name, SelectedValues: selected, NumericInputs: numeric,
		CopiesInput: w.copiesInput.Text(), CustomPageRange: w.pageRangeInput.Text(),
		Strategy: w.strategy, ValueSource: w.valueSource, TestIntent: w.testIntent, ConstraintMode: w.constraintMode,
		MaxCases: w.maxCases.Text(), ParallelJobs: w.workers.Text(), RunModes: w.selectedRunModes(), FileMode: w.fileModeGroup.Value,
		ServerPresetID: serverPresetID,
	})
	if err := w.presetStore.Save(preset); err != nil {
		w.setStatus("Preset save failed: " + err.Error())
		return
	}
	w.refreshPresetList()
	w.setStatus(fmt.Sprintf("Saved preset %q. Credentials and file paths were not stored.", name))
	w.addLog("Saved local settings preset %q to %s", name, w.presetStore.Path())
}

func (w *Window) loadNamedPreset() {
	if w.running.Load() || w.managingServer.Load() || w.inspectingJobs.Load() {
		w.setStatus("Wait for the active operation to finish before loading a preset.")
		return
	}
	name := strings.TrimSpace(w.presetName.Text())
	preset, ok := w.findPreset(name)
	if !ok {
		w.setStatus(fmt.Sprintf("Preset %q was not found. Enter one of the names shown below the preset controls.", name))
		return
	}
	w.mu.Lock()
	model := w.capabilities
	w.mu.Unlock()
	reconciled := application.ReconcilePreset(model, preset)
	w.resetSelections()
	for optionID, values := range reconciled.SelectedValues {
		option, _ := model.OptionByID(optionID)
		available := checkboxOptionValues(option)
		ensureBools(w.selected, optionID, available)
		for _, value := range values {
			w.selected[optionID][value].Value = true
		}
	}
	for optionID, value := range reconciled.NumericInputs {
		w.numericInput(optionID).SetText(value)
	}
	w.copiesInput.SetText(reconciled.CopiesInput)
	w.pageRangeInput.SetText(reconciled.CustomPageRange)
	if reconciled.HasStrategy {
		w.strategy = reconciled.Strategy
	}
	if reconciled.HasValueSource {
		w.valueSource = reconciled.ValueSource
	}
	if reconciled.HasTestIntent {
		w.testIntent = reconciled.TestIntent
	}
	if reconciled.HasConstraintMode {
		w.constraintMode = reconciled.ConstraintMode
	}
	if reconciled.MaxCases != "" {
		w.maxCases.SetText(strconvItoa(parseCaseLimit(reconciled.MaxCases)))
	}
	if reconciled.ParallelJobs != "" {
		w.workers.SetText(strconvItoa(parseWorkerCount(reconciled.ParallelJobs)))
	}
	if reconciled.FileMode != "" {
		w.fileModeGroup.Value = reconciled.FileMode
	}
	if reconciled.ServerPresetID != "" {
		w.serverPresetGroup.Value = reconciled.ServerPresetID
	}
	selectedModes := make(map[string]struct{}, len(reconciled.RunModeLabels))
	for _, label := range reconciled.RunModeLabels {
		selectedModes[label] = struct{}{}
	}
	for index, mode := range runModes {
		_, w.modeChecks[index].Value = selectedModes[mode.Label]
	}
	w.presetName.SetText(preset.Name)
	message := fmt.Sprintf("Loaded preset %q", preset.Name)
	if reconciled.DifferentServer {
		message += "; warning: it was saved for a different Fiery server"
	}
	if reconciled.Missing > 0 {
		message += fmt.Sprintf("; skipped %d unavailable or invalid value(s)", reconciled.Missing)
	}
	w.setStatus(message)
	w.addLog("%s", message)
}

func (w *Window) deleteNamedPreset() {
	if w.running.Load() || w.managingServer.Load() || w.inspectingJobs.Load() {
		w.setStatus("Wait for the active operation to finish before deleting a preset.")
		return
	}
	if w.presetStore == nil {
		w.setStatus("Preset storage is unavailable on this computer.")
		return
	}
	name := strings.TrimSpace(w.presetName.Text())
	if _, ok := w.findPreset(name); !ok {
		w.setStatus(fmt.Sprintf("Preset %q was not found.", name))
		return
	}
	confirmed, err := confirmDestructiveAction("Delete settings preset", fmt.Sprintf("Delete local preset %q?\n\nThis does not affect any Fiery server preset.", name))
	if err != nil {
		w.setStatus("Preset confirmation failed: " + err.Error())
		return
	}
	if !confirmed {
		return
	}
	if err := w.presetStore.Delete(name); err != nil {
		w.setStatus("Preset delete failed: " + err.Error())
		return
	}
	w.refreshPresetList()
	w.presetName.SetText("")
	w.setStatus(fmt.Sprintf("Deleted local preset %q", name))
}

func (w *Window) refreshPresetList() {
	if w.presetStore == nil {
		w.presetList = nil
		return
	}
	list, err := w.presetStore.List()
	if err != nil {
		w.setStatus("Preset refresh failed: " + err.Error())
		return
	}
	w.presetList = list
}

func (w *Window) findPreset(name string) (presets.Preset, bool) {
	for _, preset := range w.presetList {
		if strings.EqualFold(strings.TrimSpace(preset.Name), strings.TrimSpace(name)) {
			return preset, true
		}
	}
	return presets.Preset{}, false
}

func containsStringFold(values []string, want string) bool {
	return containsOptionValue(values, "", want)
}

func containsOptionValue(values []string, optionID, want string) bool {
	for _, value := range values {
		if optionValueMatches(optionID, value, want) {
			return true
		}
	}
	return false
}

func strconvItoa(value int) string { return fmt.Sprintf("%d", value) }
