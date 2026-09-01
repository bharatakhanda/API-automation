package capabilities

import (
	"fmt"
	"sort"
	"strings"
)

// OptionSection preserves the nested ordering used by Fiery Job Properties
// summaries (for example Color input, Color settings, and Edge enhancement).
type OptionSection struct {
	Name    string
	Options []Option
}

type OptionGroup struct {
	Name     string
	Sections []OptionSection
	// Options is the same ordered set flattened for selection/generation code.
	Options []Option
}

type ConstraintConflict struct {
	OptionID          string
	SelectedValue     string
	DependencyID      string
	DependencyValue   string
	ConflictingValues []string
}

func (c ConstraintConflict) Error() string {
	return fmt.Sprintf("%s=%q conflicts with %s=%q (published incompatible values: [%s])", c.OptionID, c.SelectedValue, c.DependencyID, c.DependencyValue, strings.Join(c.ConflictingValues, ", "))
}

var categoryOrder = []string{
	"Job Info",
	"Substrate / Media",
	"Layout",
	"Color",
	"Image",
	"Finishing",
	"VDP",
	"Installable options",
	"Other / Advanced",
}

var sectionOrder = map[string][]string{
	"Job Info":            {"", "Job notes", "Reporting", "Die printing", "Customer proofing", "Inspection marks", "Timestamp", "Additional settings"},
	"Substrate / Media":   {""},
	"Layout":              {"", "Image position", "Impose", "Additional settings"},
	"Color":               {"", "Color input", "Color settings", "Additional settings"},
	"Image":               {"", "Edge enhancement", "Image scaling (non-uniform)", "Advanced", "Barcode", "Additional settings"},
	"Finishing":           {"", "Delivery option", "Banner page", "Cover page", "Additional settings"},
	"VDP":                 {""},
	"Installable options": {""},
	"Other / Advanced":    {""},
}

var canonicalLabels = map[string]string{
	"EFBrightness":                "Brightness",
	"EFResolution":                "Resolution",
	"EFCopies":                    "Copies",
	"num copies":                  "Copies",
	"EFPrintCover":                "Cover page",
	"EFPageRange":                 "Print Range",
	"EFRotateDocument":            "Rotation angle (degrees)",
	"EFRotation":                  "Rotation angle (degrees)",
	"Scaling":                     "Scale (%)",
	"EFScale":                     "Scale (%)",
	"EFTextGfxQual":               "Fine line rendering",
	"EFFineLineRendering":         "Fine line rendering",
	"EFImageSmooth":               "Image smoothing",
	"EFImageSmoothing":            "Image smoothing",
	"EFEdgeDropSize":              "Edge enhancement",
	"EFEdgeEnhancement":           "Edge enhancement",
	"EFColorMode":                 "Color mode",
	"EFColorantDepth":             "Colorant depth",
	"EFOutProfile":                "Output profile",
	"EFOutProfileNonSpot":         "Output profile for non-spot colors",
	"OutputICCProfile":            "Output profile",
	"EFRGBOverride":               "RGB source",
	"RGBSourceProfile":            "RGB source",
	"EFSimulation":                "CMYK source",
	"CMYKSourceProfile":           "CMYK source",
	"EFGrayOverride":              "Grayscale source",
	"GrayProfile":                 "Grayscale source",
	"EFProcessNColorants":         "Extra process colorants",
	"EFColorant1Enable":           "Enabled colorants",
	"EnabledColorants":            "Enabled colorants",
	"InkDropSizes":                "Ink/drop size",
	"EFSpotColors":                "Spot color matching",
	"SpotColorCount":              "Spot color matching",
	"EFMediaType":                 "Type",
	"InputSlot":                   "Input slot",
	"EFInputSlot":                 "Input slot",
	"EFOutputBin":                 "Output tray",
	"SubstrateWidth":              "Substrate width",
	"EFMediaWidth":                "Substrate width",
	"SubstrateHeight":             "Substrate height",
	"EFMediaLength":               "Substrate height",
	"EFMediaThickness":            "Thickness",
	"EFPrintSize":                 "Substrate size",
	"PageSize":                    "Substrate size",
	"EFOrientation":               "Orientation",
	"Orientation":                 "Orientation",
	"EFImageFrontXOutput":         "Front X",
	"ImagePositionX":              "Front X",
	"EFImageFrontYOutput":         "Front Y",
	"ImagePositionY":              "Front Y",
	"EFOutputCentering":           "Origin",
	"EFImageFlagOutput":           "Offset",
	"EFImageUnitOutput":           "Units",
	"EFPDF_PS_RGB_Transparency":   "Optimize RGB transparency",
	"HasTransparency":             "Optimize RGB transparency",
	"Instruct":                    "Instructions",
	"Notes1":                      "Notes 1",
	"Notes2":                      "Notes 2",
	"EFControlBar":                "Control Bar",
	"EFPostFlight":                "Postflight",
	"EFAutoDieControl":            "Override using server's die library",
	"EFPrintDieLine":              "Print die line",
	"EFDieLineLocation":           "Print die line at",
	"EFDieLineDestination":        "Print die line to",
	"EFDieLineContent":            "Die line content",
	"EFDieLineCopies":             "Die line copies",
	"EFCustomerProofing":          "Customer proof",
	"EFCustomerProofingNumCopies": "Customer proof number of copies",
	"EFInspectionMarks":           "Inspection marks",
	"EFDateTimeLocation":          "Date time stamp location",
	"EFFluteDirection":            "Flute direction",
	"EFPageDelivery":              "Output delivery",
	"EFSort":                      "Collate setting",
	"EFPDFXobjects":               "Cache PDF and PS objects",
	"EFVDPFilePath":               "File search path",
	"EFEmbeddedRGB":               "Use RGB embedded profiles",
	"EFColorRendDict":             "RGB rendering intent",
	"EFKOnlyGrayRGB":              "Print RGB gray using black only",
	"EFEmbeddedCMYK":              "Use CMYK embedded profiles",
	"EFCMYKColorRendDict":         "CMYK rendering intent",
	"EFBlackPointCompCMYK":        "Black point compensation",
	"EFKOnlyGrayCMYK":             "Print CMYK gray using black only",
	"EFEmbeddedGray":              "Use Gray embedded profiles",
	"EFGrayColorRendDict":         "Grayscale rendering intent",
	"EFKOnlyGray":                 "Print gray using black only",
	"EFSpotOvpStrategy":           "Spot color overprint",
	"EFSpotPriority":              "Use spot group",
	"EFUsePDFXOutputIntent":       "PDF/X output intent",
	"EFRGBSep":                    "Separate RGB/Lab to CMYK source",
	"EFPureBlack":                 "Black text and graphics",
	"EFBlkOvpCtrl":                "Black overprint (for pure black)",
	"EFCompOverprint":             "Composite overprint",
	"EFTrapping":                  "Auto trapping",
	"EFPureBlackImage":            "Black images",
	"EFCurveAdjust":               "Raster curve presets",
	"EFTrappingCutback":           "Cutback trapping",
	"EFSecurityMarksPure":         "Pure security marks",
	"EFSeparations":               "Combine separations",
	"EFSubstColors":               "Substitute colors",
	"EFUniformityBypass":          "Bypass uniformity correction",
	"EFNozzleoutBypass":           "Bypass nozzle-out compensation",
	"EFGrowReverseTextGfx":        "Reverse text/graphics growth",
	"EFBarcodeErosion":            "Degree of erosion",
}

var directSectionOptions = buildOptionSet([]string{
	"EFCopies", "num copies", "EFPageRange",
	"EFRotateDocument", "EFRotation", "Scaling", "EFScale", "EFIntentDuplex",
	"EFOutProfileNonSpot", "EFOutProfile", "OutputICCProfile", "EFColorMode", "EFProcessColorantsSpotOnly", "EFProcessNColorants", "EFColorant1Enable", "EnabledColorants",
	"EFResolution", "EFBrightness", "EFImageSmooth", "EFImageSmoothing", "EFHTScreen", "EFScreenBitsPerPixel", "EFPrintSpeed",
})

var sectionByOption = buildSectionMap(map[string][]string{
	"Job notes": {
		"Instruct", "Notes1", "Notes2",
	},
	"Reporting": {
		"EFControlBar", "EFPostFlight", "EFProgressives",
	},
	"Die printing": {
		"EFAutoDieControl", "EFPrintDieLine", "EFDieLineLocation", "EFDieLineDestination", "EFDieLineContent", "EFDieLineCopies",
	},
	"Customer proofing": {
		"EFCustomerProofing", "EFCustomerProofingNumCopies", "EFPressProofing", "EFPressProofingNumCopies",
	},
	"Inspection marks": {"EFInspectionMarks"},
	"Timestamp":        {"EFDateTimeLocation"},
	"Image position": {
		"EFOutputCentering", "EFImageFlagOutput", "EFImageUnitOutput", "EFImageFrontXOutput", "EFImageFrontYOutput", "ImagePositionX", "ImagePositionY",
	},
	"Impose": {"EFIntentPreset", "EFIntentPrintSize"},
	"Color input": {
		"EFRGBOverride", "EFEmbeddedRGB", "EFColorRendDict", "EFKOnlyGrayRGB",
		"EFSimulation", "EFEmbeddedCMYK", "EFCMYKColorRendDict", "EFBlackPointCompCMYK", "EFKOnlyGrayCMYK",
		"EFGrayOverride", "EFEmbeddedGray", "EFGrayColorRendDict", "EFKOnlyGray",
		"EFSpotColors", "EFSpotPriority", "EFSpotOvpStrategy",
	},
	"Color settings": {
		"EFTrappingCutback", "EFUsePDFXOutputIntent", "EFSecurityMarksPure", "EFRGBSep", "EFPureBlack", "EFBlkOvpCtrl", "EFCompOverprint", "EFSubstColors", "EFSeparations", "EFTrapping", "EFPDF_PS_RGB_Transparency", "EFCurveAdjust", "EFPureBlackImage", "EFCurveAdjSpotBypass",
	},
	"Edge enhancement": {
		"EFTextGfxQual", "EFFineLineRendering", "EFEdgeDropSize", "EFEdgeEnhancement", "EFGrowReverseTextGfx", "EFTextGfxEdgeMult",
	},
	"Advanced": {
		"EFUniformityBypass", "EFNozzleOutLUT", "EFNozzleOutLevel", "EFNozzleoutBypass", "EFDropSizes", "EFDropVolume", "EFMaxBoostDropSize", "EFCompression", "EFJobExpertRule", "EFTonerReduce", "EFCopierMode",
	},
	"Barcode": {"EFBarcodeErosion"},
	"Delivery option": {
		"EFUseSPDOutputMapping", "EFOutputBin", "EFPageDelivery", "EFSort",
	},
	"Banner page": {"EFSP_Freq", "EFSP_OutputBin", "EFSP_FreqNumPagesMode", "EFSP_FreqNumPagesJob", "EFSP_FreqNumPagesSys", "EFSP_Content"},
	"Cover page":  {"EFPrintCover"},
})

var optionOrder = buildOptionOrder([]string{
	"EFCopies", "num copies", "EFPageRange",
	"Instruct", "Notes1", "Notes2", "EFControlBar", "EFPostFlight", "EFProgressives",
	"EFAutoDieControl", "EFPrintDieLine", "EFDieLineLocation", "EFDieLineDestination", "EFDieLineContent", "EFDieLineCopies",
	"EFCustomerProofing", "EFCustomerProofingNumCopies", "EFInspectionMarks", "EFDateTimeLocation",
	"EFPaperCatalog", "EFPCName", "EFPCMID", "EFPrintSize", "PageSize", "EFSizeName", "EFMediaType", "EFMediaThickness", "EFFluteDirection",
	"EFRotateDocument", "EFRotation", "Scaling", "EFScale", "EFOutputCentering", "EFImageFlagOutput", "EFImageUnitOutput", "EFImageFrontXOutput", "EFImageFrontYOutput", "EFIntentPreset",
	"EFOutProfileNonSpot", "EFOutProfile", "EFColorMode", "EFProcessColorantsSpotOnly", "EFProcessNColorants", "EFColorant1Enable",
	"EFRGBOverride", "EFEmbeddedRGB", "EFColorRendDict", "EFKOnlyGrayRGB", "EFSimulation", "EFEmbeddedCMYK", "EFCMYKColorRendDict", "EFBlackPointCompCMYK", "EFKOnlyGrayCMYK", "EFGrayOverride", "EFEmbeddedGray", "EFGrayColorRendDict", "EFKOnlyGray", "EFSpotColors", "EFSpotPriority", "EFSpotOvpStrategy",
	"EFTrappingCutback", "EFUsePDFXOutputIntent", "EFSecurityMarksPure", "EFRGBSep", "EFPureBlack", "EFBlkOvpCtrl", "EFCompOverprint", "EFSubstColors", "EFSeparations", "EFTrapping", "EFPDF_PS_RGB_Transparency", "EFCurveAdjust", "EFPureBlackImage",
	"EFResolution", "EFBrightness", "EFImageSmooth", "EFHTScreen", "EFScreenBitsPerPixel", "EFTextGfxQual", "EFEdgeDropSize", "EFGrowReverseTextGfx", "EFFineLineRendering", "EFUniformityBypass", "EFNozzleoutBypass", "EFBarcodeErosion",
	"EFOutputBin", "EFPageDelivery", "EFSort", "EFPrintCover",
	"EFPDFXobjects", "EFVDPFilePath", "EFEnFFTotalPagesInRecord",
})

func CategoryNames(model Model) []string {
	groups := GroupedOptions(model)
	names := make([]string, 0, len(groups))
	for _, group := range groups {
		names = append(names, group.Name)
	}
	return names
}

func GroupedOptions(model Model) []OptionGroup {
	byCategory := make(map[string]map[string][]Option)
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
		section := sectionForOption(category, option)
		if byCategory[category] == nil {
			byCategory[category] = make(map[string][]Option)
		}
		byCategory[category][section] = append(byCategory[category][section], option)
	}

	groups := make([]OptionGroup, 0, len(byCategory))
	for _, category := range categoryOrder {
		sectionsByName := byCategory[category]
		if len(sectionsByName) == 0 {
			continue
		}
		group := OptionGroup{Name: category}
		for _, sectionName := range sectionOrder[category] {
			options := sectionsByName[sectionName]
			if len(options) == 0 {
				continue
			}
			sortOptions(options)
			group.Sections = append(group.Sections, OptionSection{Name: sectionName, Options: options})
			group.Options = append(group.Options, options...)
			delete(sectionsByName, sectionName)
		}
		// Defensive fallback for a future nested section not yet in sectionOrder.
		extraNames := make([]string, 0, len(sectionsByName))
		for name := range sectionsByName {
			extraNames = append(extraNames, name)
		}
		sort.Strings(extraNames)
		for _, name := range extraNames {
			options := sectionsByName[name]
			sortOptions(options)
			group.Sections = append(group.Sections, OptionSection{Name: name, Options: options})
			group.Options = append(group.Options, options...)
		}
		groups = append(groups, group)
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
		for _, section := range group.Sections {
			filteredSection := OptionSection{Name: section.Name}
			for _, option := range section.Options {
				if optionMatches(option, group.Name, section.Name, query) {
					filteredSection.Options = append(filteredSection.Options, option)
					out.Options = append(out.Options, option)
				}
			}
			if len(filteredSection.Options) > 0 {
				out.Sections = append(out.Sections, filteredSection)
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
		for dependencyID, incompatibleValues := range dependencies {
			dependencyValue, explicitlySelected := combination[dependencyID]
			if !explicitlySelected {
				// Defaults and hidden job-ticket values are checked against the
				// imported job by Fiery's job constraint endpoint at execution time.
				continue
			}
			if !containsFold(incompatibleValues, dependencyValue) {
				continue
			}
			conflicts = append(conflicts, ConstraintConflict{
				OptionID: optionID, SelectedValue: selectedValue,
				DependencyID: dependencyID, DependencyValue: dependencyValue,
				ConflictingValues: append([]string(nil), incompatibleValues...),
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

// documentedCWSJobProperty is a necessary, but not sufficient, condition for
// rendering a property. The supplied Antares/Capella/Vela Job Properties
// taxonomy prevents backend-only schema controls from leaking into the UI;
// normal metadata eligibility checks still decide whether a documented control
// is actually supported by the connected server.
func documentedCWSJobProperty(id string) bool {
	if _, ok := canonicalLabels[id]; ok {
		return true
	}
	if _, ok := directSectionOptions[id]; ok {
		return true
	}
	if _, ok := sectionByOption[id]; ok {
		return true
	}
	_, ok := optionOrder[id]
	return ok
}

func categoryForOption(option Option) string {
	switch strings.ToLower(strings.TrimSpace(option.Group)) {
	case "fpjobinfo":
		return "Job Info"
	case "fppapersource":
		return "Substrate / Media"
	case "fplayout":
		return "Layout"
	case "fpcolorwise", "fpcolorant", "common":
		return "Color"
	case "fpimage":
		return "Image"
	case "fpfinishing":
		return "Finishing"
	case "fpvdp":
		return "VDP"
	case "efppinstallableoptions":
		return "Installable options"
	}
	if option.ID == "EFCopies" || option.ID == "num copies" || option.ID == "EFPageRange" {
		return "Job Info"
	}
	if option.ID == "Scaling" || option.ID == "EFScale" {
		return "Layout"
	}
	return "Other / Advanced"
}

func sectionForOption(category string, option Option) string {
	if _, direct := directSectionOptions[option.ID]; direct {
		return ""
	}
	if section := sectionByOption[option.ID]; section != "" {
		return section
	}
	switch category {
	case "Finishing":
		return ""
	case "Job Info", "Layout", "Image":
		return "Additional settings"
	case "Color":
		if strings.EqualFold(option.Group, "fpcolorant") {
			return ""
		}
		return "Color settings"
	default:
		return ""
	}
}

func sortOptions(options []Option) {
	sort.SliceStable(options, func(i, j int) bool {
		left, leftRanked := optionOrder[options[i].ID]
		right, rightRanked := optionOrder[options[j].ID]
		if leftRanked != rightRanked {
			return leftRanked
		}
		if leftRanked && left != right {
			return left < right
		}
		if strings.EqualFold(options[i].Label, options[j].Label) {
			return options[i].ID < options[j].ID
		}
		return strings.ToLower(options[i].Label) < strings.ToLower(options[j].Label)
	})
}

func buildSectionMap(sections map[string][]string) map[string]string {
	out := make(map[string]string)
	for section, ids := range sections {
		for _, id := range ids {
			out[id] = section
		}
	}
	return out
}

func buildOptionSet(ids []string) map[string]struct{} {
	out := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		out[id] = struct{}{}
	}
	return out
}

func buildOptionOrder(ids []string) map[string]int {
	out := make(map[string]int, len(ids))
	for rank, id := range ids {
		if _, exists := out[id]; !exists {
			out[id] = rank
		}
	}
	return out
}

func optionMatches(option Option, category, section, query string) bool {
	fields := []string{option.Label, option.ID, option.Group, option.PPDType, category, section, option.Value}
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
