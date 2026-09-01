package capabilities

import (
	"fmt"
	"sort"
	"strings"
)

type OptionGroup struct {
	Name    string
	Options []Option
}

type ConstraintConflict struct {
	OptionID         string
	SelectedValue    string
	DependencyID     string
	DependencyValue  string
	CompatibleValues []string
}

func (c ConstraintConflict) Error() string {
	return fmt.Sprintf("%s=%q requires %s to be one of [%s], got %q", c.OptionID, c.SelectedValue, c.DependencyID, strings.Join(c.CompatibleValues, ", "), c.DependencyValue)
}

var categoryOrder = []string{
	"Job info",
	"Layout",
	"Substrate",
	"Color and Image",
	"Finishing",
	"VDP",
	"Installable options",
	"Other / Advanced",
}

var canonicalLabels = map[string]string{
	"EFBrightness":              "Brightness",
	"EFResolution":              "Resolution",
	"EFCopies":                  "Copies",
	"num copies":                "Copies",
	"EFPrintCover":              "Cover page",
	"EFRotateDocument":          "Rotation",
	"EFRotation":                "Rotation",
	"Scaling":                   "Scale (%)",
	"EFScale":                   "Scale (%)",
	"EFTextGfxQual":             "Fine line rendering",
	"EFFineLineRendering":       "Fine line rendering",
	"EFImageSmooth":             "Image smoothing",
	"EFImageSmoothing":          "Image smoothing",
	"EFEdgeDropSize":            "Edge enhancement",
	"EFEdgeEnhancement":         "Edge enhancement",
	"EFColorMode":               "Color mode",
	"EFColorantDepth":           "Colorant depth",
	"EFOutProfile":              "Output ICC profile",
	"OutputICCProfile":          "Output ICC profile",
	"EFRGBOverride":             "RGB source profile",
	"RGBSourceProfile":          "RGB source profile",
	"EFSimulation":              "CMYK source profile",
	"CMYKSourceProfile":         "CMYK source profile",
	"EFGrayOverride":            "Gray profile",
	"GrayProfile":               "Gray profile",
	"EFProcessNColorants":       "Enabled colorants",
	"EFColorant1Enable":         "Enabled colorants",
	"EnabledColorants":          "Enabled colorants",
	"InkDropSizes":              "Ink/drop size",
	"EFSpotColors":              "Spot color matching",
	"SpotColorCount":            "Spot color matching",
	"EFMediaType":               "Media type",
	"InputSlot":                 "Input slot",
	"EFInputSlot":               "Input slot",
	"EFOutputBin":               "Output tray",
	"SubstrateWidth":            "Substrate width",
	"EFMediaWidth":              "Substrate width",
	"SubstrateHeight":           "Substrate height",
	"EFMediaLength":             "Substrate height",
	"EFOrientation":             "Orientation",
	"Orientation":               "Orientation",
	"EFImageFrontXOutput":       "Image position X",
	"ImagePositionX":            "Image position X",
	"EFImageFrontYOutput":       "Image position Y",
	"ImagePositionY":            "Image position Y",
	"EFPDF_PS_RGB_Transparency": "PDF transparency",
	"HasTransparency":           "PDF transparency",
}

func CategoryNames(model Model) []string {
	groups := GroupedOptions(model)
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return names
}

func GroupedOptions(model Model) []OptionGroup {
	byCategory := make(map[string][]Option)
	seenAliases := make(map[string]struct{})
	for _, option := range model.Options {
		alias := optionAlias(option.ID)
		if _, duplicate := seenAliases[alias]; duplicate {
			continue
		}
		seenAliases[alias] = struct{}{}
		if label := canonicalLabels[option.ID]; label != "" {
			option.Label = label
		}
		category := categoryForOption(option)
		byCategory[category] = append(byCategory[category], option)
	}
	groups := make([]OptionGroup, 0, len(byCategory))
	for _, name := range categoryOrder {
		options := byCategory[name]
		if len(options) == 0 {
			continue
		}
		sort.Slice(options, func(i, j int) bool {
			if strings.EqualFold(options[i].Label, options[j].Label) {
				return options[i].ID < options[j].ID
			}
			return strings.ToLower(options[i].Label) < strings.ToLower(options[j].Label)
		})
		groups = append(groups, OptionGroup{Name: name, Options: options})
	}
	return groups
}

func FilteredGroups(model Model, query string) []OptionGroup {
	groups := GroupedOptions(model)
	query = strings.ToLower(strings.TrimSpace(query))
	if query == "" {
		return groups
	}
	filtered := make([]OptionGroup, 0, len(groups))
	for _, group := range groups {
		out := OptionGroup{Name: group.Name}
		for _, option := range group.Options {
			if optionMatches(option, group.Name, query) {
				out.Options = append(out.Options, option)
			}
		}
		if len(out.Options) > 0 {
			filtered = append(filtered, out)
		}
	}
	return filtered
}

func HasExplicitConstraintDependencies(model Model, optionIDs map[string]struct{}) bool {
	for _, option := range model.Options {
		if _, selected := optionIDs[option.ID]; !selected || len(option.Constraints) == 0 {
			continue
		}
		for _, dependencies := range option.Constraints {
			for dependencyID := range dependencies {
				if _, selectedDependency := optionIDs[dependencyID]; selectedDependency {
					return true
				}
			}
		}
	}
	return false
}

func ValidateCombination(model Model, combination map[string]string) []ConstraintConflict {
	if len(combination) == 0 {
		return nil
	}
	var conflicts []ConstraintConflict
	for optionID, selectedValue := range combination {
		option, ok := model.OptionByID(optionID)
		if !ok || len(option.Constraints) == 0 {
			continue
		}
		dependencies := option.Constraints[selectedValue]
		for dependencyID, compatibleValues := range dependencies {
			dependencyValue, explicitlySelected := combination[dependencyID]
			if !explicitlySelected {
				// Defaults and hidden job-ticket values are checked against the
				// imported job by Fiery's job constraint endpoint at execution time.
				continue
			}
			if containsFold(compatibleValues, dependencyValue) {
				continue
			}
			conflicts = append(conflicts, ConstraintConflict{
				OptionID: optionID, SelectedValue: selectedValue,
				DependencyID: dependencyID, DependencyValue: dependencyValue,
				CompatibleValues: append([]string(nil), compatibleValues...),
			})
		}
	}
	sort.Slice(conflicts, func(i, j int) bool { return conflicts[i].Error() < conflicts[j].Error() })
	return conflicts
}

func NeedsConstraintCheck(model Model, selected map[string]string) bool {
	if len(selected) == 0 {
		return false
	}
	for _, option := range model.Options {
		if option.Synthetic && (option.ID == "Scaling" || option.ID == "EFScale") {
			if _, selectedScale := selected[option.ID]; selectedScale {
				return true
			}
		}
		if len(option.Constraints) == 0 {
			continue
		}
		if _, selectedOption := selected[option.ID]; selectedOption {
			return true
		}
		for _, dependencies := range option.Constraints {
			for dependencyID := range dependencies {
				if _, selectedDependency := selected[dependencyID]; selectedDependency {
					return true
				}
			}
		}
	}
	return false
}

func ConstraintCount(model Model) int {
	count := 0
	for _, option := range model.Options {
		if len(option.Constraints) > 0 {
			count++
		}
	}
	return count
}

func optionAlias(id string) string {
	switch id {
	case "EFCopies", "num copies":
		return "copies"
	case "Scaling", "EFScale":
		return "scaling"
	case "EFRotateDocument", "EFRotation":
		return "rotation"
	case "EFTextGfxQual", "EFFineLineRendering":
		return "fine-line"
	case "EFImageSmooth", "EFImageSmoothing":
		return "image-smoothing"
	case "EFEdgeDropSize", "EFEdgeEnhancement":
		return "edge-enhancement"
	}
	return id
}

func categoryForOption(option Option) string {
	switch strings.ToLower(strings.TrimSpace(option.Group)) {
	case "fpjobinfo":
		return "Job info"
	case "fplayout":
		return "Layout"
	case "fppapersource":
		return "Substrate"
	case "fpcolorwise", "fpcolorant", "fpimage", "common":
		return "Color and Image"
	case "fpfinishing":
		return "Finishing"
	case "fpvdp":
		return "VDP"
	case "efppinstallableoptions":
		return "Installable options"
	}
	if option.ID == "EFCopies" || option.ID == "num copies" {
		return "Job info"
	}
	if option.ID == "Scaling" || option.ID == "EFScale" {
		return "Layout"
	}
	return "Other / Advanced"
}

func optionMatches(option Option, category, query string) bool {
	fields := []string{option.Label, option.ID, option.Group, option.PPDType, category, option.Value}
	fields = append(fields, option.Values...)
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func containsFold(values []string, want string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(want)) {
			return true
		}
	}
	return false
}
