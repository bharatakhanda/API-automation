//go:build windows

package appgio

import (
	"fmt"
	"sort"
	"strings"

	"api-automation/internal/combinations"
	"api-automation/internal/copyvalues"
	"api-automation/internal/pagevalues"
	"api-automation/internal/presets"
	"api-automation/internal/rangevalues"
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
	selected := make(map[string][]string)
	for optionID, values := range w.selected {
		if _, exists := model.OptionByID(optionID); !exists {
			continue
		}
		if chosen := selectedValues(values); len(chosen) > 0 {
			if isPageRangeOption(optionID) {
				filtered := chosen[:0]
				for _, value := range chosen {
					if !strings.EqualFold(strings.TrimSpace(value), pageRangeCustomServerValue) {
						filtered = append(filtered, value)
					}
				}
				chosen = filtered
			}
			if len(chosen) > 0 {
				sort.Strings(chosen)
				selected[optionID] = chosen
			}
		}
	}
	numeric := make(map[string]string)
	if option, ok := copiesOption(model); ok {
		numeric[option.ID] = strings.TrimSpace(w.copiesInput.Text())
	}
	for optionID, input := range w.numericInputs {
		if value := strings.TrimSpace(input.Text()); value != "" {
			numeric[optionID] = value
		}
	}
	if value := strings.TrimSpace(w.pageRangeInput.Text()); value != "" {
		if _, exists := model.OptionByID(pageRangeOptionID); exists {
			numeric[pageRangeDataID] = value
		}
	}
	serverPresetID := ""
	if selectedPreset, err := w.selectedServerPreset(model); err == nil && selectedPreset != nil {
		serverPresetID = selectedPreset.ID
	}
	preset := presets.Preset{
		Name: name, ServerName: model.ServerName, ServerSerial: model.SerialNumber, ServerPresetID: serverPresetID,
		SelectedValues: selected, NumericInputs: numeric,
		Strategy: string(w.strategy), MaxCases: strings.TrimSpace(w.maxCases.Text()), ParallelJobs: strings.TrimSpace(w.workers.Text()),
		RunModes: runModeLabels(w.selectedRunModes()), FileMode: w.fileModeGroup.Value,
	}
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
	w.resetSelections()
	missing := 0
	for optionID, values := range preset.SelectedValues {
		option, exists := model.OptionByID(optionID)
		if !exists || option.Range != nil {
			missing += len(values)
			continue
		}
		available := checkboxOptionValues(option)
		ensureBools(w.selected, optionID, available)
		for _, value := range values {
			if isPageRangeOption(optionID) && strings.EqualFold(strings.TrimSpace(value), pageRangeCustomServerValue) {
				missing++
				continue
			}
			if !containsStringFold(available, value) {
				missing++
				continue
			}
			for _, current := range available {
				if strings.EqualFold(current, value) {
					w.selected[optionID][current].Value = true
					break
				}
			}
		}
	}
	for optionID, value := range preset.NumericInputs {
		if strings.EqualFold(optionID, pageRangeDataID) {
			if _, exists := model.OptionByID(pageRangeOptionID); !exists {
				missing++
				continue
			}
			if _, err := pagevalues.Parse(value, pagevalues.DefaultExpansionLimit); err != nil {
				missing++
				continue
			}
			w.pageRangeInput.SetText(value)
			continue
		}
		if isCopiesOption(optionID) {
			if _, err := copyvalues.Parse(value); err != nil {
				missing++
				continue
			}
			w.copiesInput.SetText(value)
			continue
		}
		option, exists := model.OptionByID(optionID)
		if !exists || option.Range == nil {
			missing++
			continue
		}
		bounds := rangevalues.Bounds{Min: option.Range.Min, Max: option.Range.Max, Increment: option.Range.Increment, Precision: option.Range.Precision}
		if _, err := rangevalues.Parse(value, bounds, rangevalues.DefaultExpansionLimit); err != nil {
			missing++
			continue
		}
		w.numericInput(optionID).SetText(value)
	}
	switch combinations.Strategy(preset.Strategy) {
	case combinations.StrategySelected, combinations.StrategyAll, combinations.StrategyPairwise:
		w.strategy = combinations.Strategy(preset.Strategy)
	}
	if value := strings.TrimSpace(preset.MaxCases); value != "" {
		w.maxCases.SetText(strconvItoa(parseCaseLimit(value)))
	}
	if value := strings.TrimSpace(preset.ParallelJobs); value != "" {
		w.workers.SetText(strconvItoa(parseWorkerCount(value)))
	}
	if preset.FileMode == "all" || preset.FileMode == "single" || preset.FileMode == "random" {
		w.fileModeGroup.Value = preset.FileMode
	}
	if preset.ServerPresetID != "" {
		found := false
		for _, serverPreset := range model.ServerPresets {
			if serverPreset.ID == preset.ServerPresetID {
				w.serverPresetGroup.Value = serverPreset.ID
				found = true
				break
			}
		}
		if !found {
			missing++
		}
	}
	selectedModes := make(map[string]struct{}, len(preset.RunModes))
	for _, label := range preset.RunModes {
		selectedModes[label] = struct{}{}
	}
	for index, mode := range runModes {
		_, w.modeChecks[index].Value = selectedModes[mode.Label]
	}
	if len(selectedModes) == 0 && len(w.modeChecks) > 0 {
		w.modeChecks[0].Value = true
	}
	w.presetName.SetText(preset.Name)
	message := fmt.Sprintf("Loaded preset %q", preset.Name)
	if preset.ServerSerial != "" && model.SerialNumber != "" && !strings.EqualFold(preset.ServerSerial, model.SerialNumber) {
		message += "; warning: it was saved for a different Fiery server"
	}
	if missing > 0 {
		message += fmt.Sprintf("; skipped %d unavailable or invalid value(s)", missing)
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
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}

func strconvItoa(value int) string { return fmt.Sprintf("%d", value) }
