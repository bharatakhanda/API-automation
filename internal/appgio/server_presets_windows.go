//go:build windows

package appgio

import (
	"fmt"

	"api-automation/internal/capabilities"
	"api-automation/internal/fiery"

	"gioui.org/layout"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

func (w *Window) serverPresetPanel(gtx layout.Context, model capabilities.Model) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(label(w.theme, "Fiery server preset", 15, palette.text).Layout),
		layout.Rigid(spacer(3)),
		layout.Rigid(label(w.theme, "Presets are discovered read-only from API v5. The selected preset is applied to each imported job before explicitly selected capability values; this application cannot create, edit, or delete server presets.", 12, palette.muted).Layout),
		layout.Rigid(spacer(8)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			radio := material.RadioButton(w.theme, &w.serverPresetGroup, noServerPresetID, "Do not apply a server preset")
			radio.Color = palette.text
			radio.IconColor = palette.primary
			return radio.Layout(gtx)
		}),
	}
	for _, discovered := range model.ServerPresets {
		preset := discovered
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			caption := fmt.Sprintf("%s · ID %s", preset.Name, preset.ID)
			if len(preset.Attributes) > 0 {
				caption += fmt.Sprintf(" · %d advertised setting(s)", len(preset.Attributes))
			}
			return layout.Inset{Top: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				radio := material.RadioButton(w.theme, &w.serverPresetGroup, preset.ID, caption)
				radio.Color = palette.text
				radio.IconColor = palette.primary
				return radio.Layout(gtx)
			})
		}))
	}
	if len(model.ServerPresets) == 0 {
		children = append(children,
			layout.Rigid(spacer(4)),
			layout.Rigid(label(w.theme, "The connected Fiery did not return any server presets, or the endpoint is unavailable for this account.", 12, palette.muted).Layout),
		)
	}
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (w *Window) selectedServerPreset(model capabilities.Model) (*fiery.ServerPreset, error) {
	id := w.serverPresetGroup.Value
	if id == "" || id == noServerPresetID {
		return nil, nil
	}
	for _, preset := range model.ServerPresets {
		if preset.ID == id {
			copy := preset
			copy.Attributes = cloneStringMap(preset.Attributes)
			return &copy, nil
		}
	}
	return nil, fmt.Errorf("selected Fiery server preset %q is no longer advertised; refresh capabilities or select another preset", id)
}

func serverPresetDescription(preset *fiery.ServerPreset) string {
	if preset == nil {
		return "none"
	}
	return fmt.Sprintf("%s (ID %s)", preset.Name, preset.ID)
}
