package capabilities

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"api-automation/internal/fiery"
)

var importantPropertyLabels = map[string]string{
	"PageSize":                  "Page size",
	"EFPrintSize":               "Print size",
	"EFMediaType":               "Media type",
	"EFColorMode":               "Color mode",
	"EFResolution":              "Resolution",
	"EFPrintSpeed":              "Print speed",
	"EFCopies":                  "Copies",
	"num copies":                "Copies",
	"InputSlot":                 "Input slot",
	"EFOutputBin":               "Output tray",
	"EFPageDelivery":            "Output delivery",
	"EFSort":                    "Collate setting",
	"EFPrintCover":              "Cover page",
	"EFPageRange":               "Page range",
	"EFRotateDocument":          "Rotation angle",
	"EFOutputCentering":         "Origin",
	"EFBrightness":              "Brightness",
	"EFTextGfxQual":             "Fine line rendering",
	"EFEdgeDropSize":            "Edge enhancement",
	"EFProcessNColorants":       "Extra process colorants",
	"EFColorant1Enable":         "Specialty colorant enable",
	"EFPDF_PS_RGB_Transparency": "PDF transparency",
	"EFImageFrontXOutput":       "Image position X",
	"EFImageFrontYOutput":       "Image position Y",
	"EFRaster":                  "Raster mode",
	"EFFineLineRendering":       "Fine line rendering",
	"EFImageSmooth":             "Image smoothing",
	"EFOutProfile":              "Output profile",
	"EFRGBOverride":             "RGB source",
	"EFSimulation":              "CMYK source",
	"EFGrayOverride":            "Grayscale source",
	"EFSpotColors":              "Spot color matching",
	"EFPureBlack":               "Black text/graphics",
	"EFBlackPointCompCMYK":      "Black point compensation",
	"Scaling":                   "Scale (%)",
	"EFScale":                   "Scale (%)",
}

func FromSnapshot(snapshot fiery.CapabilitySnapshot) Model {
	model := Model{}
	for _, endpoint := range snapshot.Endpoints {
		switch endpoint.Name {
		case "info", "v5_info", "v4_info":
			applyInfo(&model, endpoint.Body)
		case "queues", "v5_queues":
			model.Queues = parseQueues(endpoint.Body)
		case "presets", "v5_presets":
			model.ServerPresets = fiery.ParseServerPresets(endpoint.Body)
		case "properties", "v5_properties", "v4_properties":
			options := parseProperties(endpoint.Body)
			if len(options) > len(model.Options) {
				model.Options = options
			}
		}
	}
	model.PressModel = pressModelFromProperties(model.Options)
	if model.PressModel == "" {
		model.PressModel = pressModelFromQueues(model.Queues)
	}
	configurationFiltered, configurationExcluded, excludedValues := applyFixedConfigurationConstraints(model.Options)
	model.Options, model.ExcludedOptions = eligibleJobProperties(configurationFiltered)
	model.ExcludedOptions = append(model.ExcludedOptions, configurationExcluded...)
	sort.Slice(model.ExcludedOptions, func(i, j int) bool {
		if model.ExcludedOptions[i].Reason != model.ExcludedOptions[j].Reason {
			return model.ExcludedOptions[i].Reason < model.ExcludedOptions[j].Reason
		}
		return model.ExcludedOptions[i].ID < model.ExcludedOptions[j].ID
	})
	model.ExcludedValues = excludedValues
	model.Options = addSyntheticCopiesOption(model.Options)
	model.Options = addSyntheticScaleOption(model.Options)
	sort.Slice(model.Queues, func(i, j int) bool {
		if model.Queues[i].Available != model.Queues[j].Available {
			return model.Queues[i].Available
		}
		return model.Queues[i].Name < model.Queues[j].Name
	})
	return model
}

func applyInfo(model *Model, body json.RawMessage) {
	var payload struct {
		Data struct {
			Item struct {
				Name            string `json:"name"`
				SerialNumber    string `json:"serial_number"`
				Version         string `json:"version"`
				TimeZone        string `json:"timezone"`
				Locale          string `json:"locale"`
				UptimeSeconds   int64  `json:"uptime"`
				DiskAvailable   int64  `json:"disk_available"`
				DiskTotal       int64  `json:"disk_total"`
				MemoryAvailable int64  `json:"memory_available"`
				MemoryTotal     int64  `json:"memory_total"`
			} `json:"item"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return
	}
	// Discovery may return both v5 and v4 info. Do not let a later endpoint
	// with a partial response erase values already learned from another version.
	if value := strings.TrimSpace(payload.Data.Item.Name); value != "" {
		model.ServerName = value
	}
	if value := strings.TrimSpace(payload.Data.Item.SerialNumber); value != "" {
		model.SerialNumber = value
	}
	if value := strings.TrimSpace(payload.Data.Item.Version); value != "" {
		model.Version = value
	}
	if value := strings.TrimSpace(payload.Data.Item.TimeZone); value != "" {
		model.TimeZone = value
	}
	if value := strings.TrimSpace(payload.Data.Item.Locale); value != "" {
		model.Locale = value
	}
	if payload.Data.Item.UptimeSeconds > 0 {
		model.UptimeSeconds = payload.Data.Item.UptimeSeconds
	}
	if payload.Data.Item.DiskAvailable > 0 {
		model.DiskAvailable = payload.Data.Item.DiskAvailable
	}
	if payload.Data.Item.DiskTotal > 0 {
		model.DiskTotal = payload.Data.Item.DiskTotal
	}
	if payload.Data.Item.MemoryAvailable > 0 {
		model.MemoryAvailable = payload.Data.Item.MemoryAvailable
	}
	if payload.Data.Item.MemoryTotal > 0 {
		model.MemoryTotal = payload.Data.Item.MemoryTotal
	}
}

func parseQueues(body json.RawMessage) []Queue {
	var payload struct {
		Data struct {
			Items []struct {
				ID        any    `json:"id"`
				Name      string `json:"name"`
				Available bool   `json:"available"`
				Editable  bool   `json:"editable"`
			} `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	queues := make([]Queue, 0, len(payload.Data.Items))
	for _, item := range payload.Data.Items {
		queues = append(queues, Queue{
			ID:        fmt.Sprint(item.ID),
			Name:      strings.TrimSpace(item.Name),
			Available: item.Available,
			Editable:  item.Editable,
		})
	}
	return queues
}

func parseProperties(body json.RawMessage) []Option {
	var payload struct {
		Data struct {
			Items []struct {
				ID              string          `json:"id"`
				Group           string          `json:"group"`
				PPDType         string          `json:"ppdtype"`
				Value           any             `json:"value"`
				Values          []any           `json:"values"`
				Scopes          []string        `json:"scopes"`
				Editable        any             `json:"editable"`
				Enabled         any             `json:"enabled"`
				Available       any             `json:"available"`
				Hidden          any             `json:"hidden"`
				Visible         any             `json:"visible"`
				Numeric         any             `json:"Numeric"`
				Length          any             `json:"length"`
				ValueAttributes json.RawMessage `json:"value_attributes"`
				Constraints     json.RawMessage `json:"constraints"`
			} `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	options := make([]Option, 0, len(payload.Data.Items))
	for _, item := range payload.Data.Items {
		id := strings.TrimSpace(item.ID)
		if id == "" {
			continue
		}
		label := importantPropertyLabels[id]
		if label == "" {
			label = id
		}
		// Preserve each advertised EFOutProfile value byte-for-byte. Fiery uses a
		// leading U+FEFF as part of the profile menu identity; it can be hidden in
		// labels and ignored for readback comparison, but stripping it from the
		// update prevents CWS from resolving the selected menu entry.
		values := cleanPropertyValues(id, item.Values)
		value := cleanPropertyValue(id, item.Value)
		available := combinedOptionalBool(item.Enabled, item.Available)
		editable := combinedOptionalBool(item.Editable, valueAttribute(item.ValueAttributes, "editable"))
		hidden := cleanBool(item.Hidden)
		if visible, specified := optionalBool(item.Visible); specified && !visible {
			hidden = true
		}
		option := Option{
			ID:                id,
			Label:             label,
			Group:             strings.TrimSpace(item.Group),
			PPDType:           strings.ToLower(strings.TrimSpace(item.PPDType)),
			Value:             value,
			Values:            values,
			Scopes:            append([]string(nil), item.Scopes...),
			Enabled:           len(values) > 0 || value != "" || strings.EqualFold(item.PPDType, "efirange"),
			Editable:          editable != nil && *editable,
			EditableSpecified: editable != nil,
			Available:         available,
			Hidden:            hidden,
			Numeric:           cleanBool(item.Numeric),
			Length:            cleanInt(item.Length),
			Range:             parseNumericRange(item.PPDType, item.ValueAttributes),
			Constraints:       parseConstraints(id, item.Constraints),
		}
		if available != nil && !*available {
			option.Enabled = false
		}
		if (id == "Scaling" || id == "EFScale") && option.Range == nil {
			option.PPDType = "efirange"
			option.Editable = true
			option.Numeric = true
			option.Range = &NumericRange{Min: 25, Max: 400, Increment: 1, Precision: 0}
		}
		options = append(options, option)
	}
	sort.Slice(options, func(i, j int) bool {
		if options[i].Label != options[j].Label {
			return options[i].Label < options[j].Label
		}
		return options[i].ID < options[j].ID
	})
	return options
}

var jobPropertyGroups = map[string]struct{}{
	"fpjobinfo": {}, "fppapersource": {}, "fplayout": {}, "fpcolorwise": {},
	"fpcolorant": {}, "common": {}, "fpimage": {}, "fpfinishing": {}, "fpvdp": {},
}

// These scopes provide affirmative evidence that a property participates in a
// direct job operation or a visible CWS job context. Container, mixed-media,
// setup, devdict, intent, PS, APPE, and Impose scopes describe schema nesting or
// consumers; by themselves they do not prove that a root job-ticket control is
// selectable.
var jobApplicabilityScopes = map[string]struct{}{
	"command": {}, "rerip": {}, "joblog": {}, "column": {}, "dynamic": {},
	"summarypane": {}, "calibrator": {}, "mediakey": {}, "efiraster": {},
}

var visibleJobApplicabilityScopes = map[string]struct{}{
	"rerip": {}, "joblog": {}, "column": {}, "dynamic": {}, "summarypane": {},
	"calibrator": {}, "mediakey": {}, "efiraster": {},
}

func applyFixedConfigurationConstraints(options []Option) ([]Option, []ExcludedOption, []ExcludedValue) {
	byID := make(map[string]Option, len(options))
	fixedDefaults := make(map[string]string)
	for _, option := range options {
		byID[option.ID] = option
		if strings.EqualFold(strings.TrimSpace(option.Group), "efppinstallableoptions") && strings.TrimSpace(option.Value) != "" {
			fixedDefaults[option.ID] = option.Value
		}
	}
	filtered := make([]Option, 0, len(options))
	excludedOptions := make([]ExcludedOption, 0)
	excludedValues := make([]ExcludedValue, 0)
	for _, option := range options {
		if len(option.Values) == 0 || len(option.Constraints) == 0 || strings.EqualFold(strings.TrimSpace(option.Group), "efppinstallableoptions") {
			filtered = append(filtered, option)
			continue
		}
		originalCount := len(option.Values)
		kept := make([]string, 0, originalCount)
		for _, value := range option.Values {
			reason := fixedConfigurationConflict(option, value, fixedDefaults, byID)
			if reason == "" {
				kept = append(kept, value)
				continue
			}
			excludedValues = append(excludedValues, ExcludedValue{OptionID: option.ID, Value: value, Reason: reason})
		}
		option.Values = kept
		if len(kept) == 0 || (originalCount > 1 && len(kept) == 1 && strings.EqualFold(strings.TrimSpace(kept[0]), strings.TrimSpace(option.Value))) {
			excludedOptions = append(excludedOptions, ExcludedOption{ID: option.ID, Reason: "installed server configuration leaves no selectable value beyond the current default", Property: option})
			continue
		}
		filtered = append(filtered, option)
	}
	sort.Slice(excludedValues, func(i, j int) bool {
		if excludedValues[i].OptionID != excludedValues[j].OptionID {
			return excludedValues[i].OptionID < excludedValues[j].OptionID
		}
		return excludedValues[i].Value < excludedValues[j].Value
	})
	return filtered, excludedOptions, excludedValues
}

func fixedConfigurationConflict(option Option, value string, fixedDefaults map[string]string, byID map[string]Option) string {
	dependencies := option.Constraints[value]
	for dependencyID, incompatible := range dependencies {
		current, fixed := fixedDefaults[dependencyID]
		if !fixed || !containsFold(incompatible, current) {
			continue
		}
		dependencyLabel := dependencyID
		if dependency, ok := byID[dependencyID]; ok && strings.TrimSpace(dependency.Label) != "" {
			dependencyLabel = dependency.Label + " (" + dependencyID + ")"
		}
		return fmt.Sprintf("%s is fixed at incompatible value %q", dependencyLabel, current)
	}
	return ""
}

func eligibleJobProperties(options []Option) ([]Option, []ExcludedOption) {
	fixedConfiguration := make(map[string]string)
	for _, option := range options {
		if strings.EqualFold(strings.TrimSpace(option.Group), "efppinstallableoptions") {
			fixedConfiguration[strings.ToLower(strings.TrimSpace(option.ID))] = strings.TrimSpace(option.Value)
		}
	}
	eligible := make([]Option, 0, len(options))
	excluded := make([]ExcludedOption, 0)
	for _, option := range options {
		if reason := jobPropertyExclusionReason(option); reason != "" {
			excluded = append(excluded, ExcludedOption{ID: option.ID, Reason: reason, Property: option})
			continue
		}
		if reason := fixedPropertyFamilyExclusionReason(option, fixedConfiguration); reason != "" {
			excluded = append(excluded, ExcludedOption{ID: option.ID, Reason: reason, Property: option})
			continue
		}
		eligible = append(eligible, option)
	}
	var aliasExclusions []ExcludedOption
	eligible, aliasExclusions = collapseAliasedJobProperties(eligible)
	excluded = append(excluded, aliasExclusions...)
	sort.Slice(excluded, func(i, j int) bool {
		if excluded[i].Reason != excluded[j].Reason {
			return excluded[i].Reason < excluded[j].Reason
		}
		return excluded[i].ID < excluded[j].ID
	})
	return eligible, excluded
}

func jobPropertyExclusionReason(option Option) string {
	if option.Available != nil && !*option.Available {
		return "server explicitly reports the property as disabled or unavailable"
	}
	if option.EditableSpecified && !option.Editable {
		return "server explicitly reports the property as non-editable"
	}
	if option.Hidden {
		return "server explicitly reports the property as hidden"
	}
	group := strings.ToLower(strings.TrimSpace(option.Group))
	if group == "" {
		return "server schema or identity metadata has no job-property group"
	}
	if group == "efppinstallableoptions" {
		return "installable/server configuration option is not a writable job property"
	}
	if _, supportedGroup := jobPropertyGroups[group]; !supportedGroup {
		return "property group is not a recognized writable job-property group"
	}
	if !documentedCWSJobProperty(option.ID) {
		return "property is not mapped in the documented CWS Job Properties taxonomy"
	}
	switch strings.ToLower(strings.TrimSpace(option.PPDType)) {
	case "uimenu":
		if len(option.Values) == 0 {
			return "menu property has no selectable values"
		}
		if len(option.Values) == 1 {
			return "menu property advertises only its current value and has no selectable alternative"
		}
	case "efirange":
		if option.Range == nil {
			return "numeric property has no valid server-advertised range"
		}
	default:
		return "property type is not supported by the application as a selectable control"
	}
	if !hasAnyScope(option.Scopes, jobApplicabilityScopes) {
		return "property is advertised only for generic or nested schema contexts, not direct job operation"
	}
	if hasScope(option.Scopes, "driveruserdisplayhidden") && !hasAnyScope(option.Scopes, visibleJobApplicabilityScopes) {
		return "property is marked hidden and has no independent job-operation visibility"
	}
	return ""
}

func fixedPropertyFamilyExclusionReason(option Option, fixedConfiguration map[string]string) string {
	group := strings.ToLower(strings.TrimSpace(option.Group))
	if group == "fpcolorant" {
		hasColorantConfiguration := false
		hasEnabledColorant := false
		for id, value := range fixedConfiguration {
			if !isNumberedColorantInstallableOption(id) {
				continue
			}
			hasColorantConfiguration = true
			if !configurationValueDisabled(value) {
				hasEnabledColorant = true
			}
		}
		if hasColorantConfiguration && !hasEnabledColorant {
			return "specialty-colorant job controls are disabled by the installed server configuration"
		}
	}
	if strings.Contains(strings.ToLower(option.ID), "duplex") {
		if value, reported := fixedConfiguration["efduplexopt"]; reported && configurationValueDisabled(value) {
			return "duplex job controls are disabled by the installed server configuration"
		}
	}
	return ""
}

func isNumberedColorantInstallableOption(id string) bool {
	id = strings.ToLower(strings.TrimSpace(id))
	if !strings.HasPrefix(id, "efcolorant") || !strings.HasSuffix(id, "opt") {
		return false
	}
	number := strings.TrimSuffix(strings.TrimPrefix(id, "efcolorant"), "opt")
	if number == "" {
		return false
	}
	_, err := strconv.Atoi(number)
	return err == nil
}

func configurationValueDisabled(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "0", "false", "no", "none", "off", "disabled", "notinstalled", "not installed":
		return true
	default:
		return false
	}
}

func collapseAliasedJobProperties(options []Option) ([]Option, []ExcludedOption) {
	kept := make([]Option, 0, len(options))
	indexByAlias := make(map[string]int, len(options))
	var excluded []ExcludedOption
	for _, option := range options {
		alias := optionAlias(option.ID)
		index, duplicate := indexByAlias[alias]
		if !duplicate {
			indexByAlias[alias] = len(kept)
			kept = append(kept, option)
			continue
		}
		current := kept[index]
		if preferJobProperty(option, current) {
			excluded = append(excluded, ExcludedOption{
				ID: current.ID, Reason: fmt.Sprintf("equivalent job-property alias %s provides stronger user-visible metadata", option.ID), Property: current,
			})
			kept[index] = option
			continue
		}
		excluded = append(excluded, ExcludedOption{
			ID: option.ID, Reason: fmt.Sprintf("equivalent job-property alias %s provides stronger user-visible metadata", current.ID), Property: option,
		})
	}
	return kept, excluded
}

func preferJobProperty(candidate, current Option) bool {
	candidateHidden := hasScope(candidate.Scopes, "driveruserdisplayhidden")
	currentHidden := hasScope(current.Scopes, "driveruserdisplayhidden")
	if candidateHidden != currentHidden {
		return !candidateHidden
	}
	candidateCommand := hasScope(candidate.Scopes, "command")
	currentCommand := hasScope(current.Scopes, "command")
	if candidateCommand != currentCommand {
		return candidateCommand
	}
	if len(candidate.Values) != len(current.Values) {
		return len(candidate.Values) > len(current.Values)
	}
	return candidate.ID < current.ID
}

func hasAnyScope(scopes []string, accepted map[string]struct{}) bool {
	for _, scope := range scopes {
		if _, ok := accepted[strings.ToLower(strings.TrimSpace(scope))]; ok {
			return true
		}
	}
	return false
}

func hasScope(scopes []string, expected string) bool {
	for _, scope := range scopes {
		if strings.EqualFold(strings.TrimSpace(scope), expected) {
			return true
		}
	}
	return false
}

func pressModelFromProperties(options []Option) string {
	for _, preferredID := range []string{"CWS_DEVICE_DISPLAY_NAME", "ModelName", "Product"} {
		for _, option := range options {
			if strings.EqualFold(option.ID, preferredID) {
				if value := cleanIdentityValue(option.Value); value != "" {
					return value
				}
			}
		}
	}
	return ""
}

func pressModelFromQueues(queues []Queue) string {
	for _, queue := range queues {
		name := strings.TrimSpace(queue.Name)
		lower := strings.ToLower(name)
		for _, suffix := range []string{" press-queue", " interactive", " patch", " font", " hold", " print"} {
			if strings.HasSuffix(lower, suffix) {
				if model := strings.TrimSpace(name[:len(name)-len(suffix)]); model != "" {
					return model
				}
			}
		}
	}
	return ""
}

func cleanIdentityValue(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "\ufeff"))
	for {
		before := value
		if len(value) >= 2 && ((value[0] == '"' && value[len(value)-1] == '"') || (value[0] == '(' && value[len(value)-1] == ')')) {
			value = strings.TrimSpace(value[1 : len(value)-1])
		}
		if value == before {
			return value
		}
	}
}

func addSyntheticCopiesOption(options []Option) []Option {
	for _, option := range options {
		if option.ID == "num copies" || option.ID == "EFCopies" {
			return options
		}
	}
	options = append(options, Option{
		ID:        "num copies",
		Label:     "Copies",
		Group:     "fpjobinfo",
		PPDType:   "efirange",
		Value:     "1",
		Values:    []string{"1", "2", "5", "10"},
		Scopes:    []string{"command", "cws", "job", "manual-from-scope-guidance"},
		Enabled:   true,
		Editable:  true,
		Numeric:   true,
		Range:     &NumericRange{Min: 1, Max: 9999, Increment: 1, Precision: 0},
		Synthetic: true,
	})
	sort.Slice(options, func(i, j int) bool {
		if options[i].Label != options[j].Label {
			return options[i].Label < options[j].Label
		}
		return options[i].ID < options[j].ID
	})
	return options
}

func addSyntheticScaleOption(options []Option) []Option {
	for _, option := range options {
		if option.ID == "Scaling" || option.ID == "EFScale" {
			return options
		}
	}
	options = append(options, Option{
		ID:        "Scaling",
		Label:     "Scale (%)",
		Group:     "fplayout",
		PPDType:   "efirange",
		Value:     "100",
		Scopes:    []string{"command", "ps", "job", "manual-standard-ppd"},
		Enabled:   true,
		Editable:  true,
		Numeric:   true,
		Range:     &NumericRange{Min: 25, Max: 400, Increment: 1, Precision: 0},
		Synthetic: true,
	})
	sort.Slice(options, func(i, j int) bool {
		if options[i].Label != options[j].Label {
			return options[i].Label < options[j].Label
		}
		return options[i].ID < options[j].ID
	})
	return options
}

func parseNumericRange(ppdType string, raw json.RawMessage) *NumericRange {
	if !strings.EqualFold(strings.TrimSpace(ppdType), "efirange") || len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var attributes map[string]any
	if json.Unmarshal(raw, &attributes) != nil {
		return nil
	}
	minimum, minOK := number(attributes["min"])
	maximum, maxOK := number(attributes["max"])
	if !minOK || !maxOK || minimum > maximum {
		return nil
	}
	increment, ok := number(attributes["increment"])
	if !ok || increment <= 0 {
		increment = 1
	}
	precision := cleanInt(attributes["precision"])
	if precision < 0 || precision > 9 {
		precision = 0
	}
	return &NumericRange{Min: minimum, Max: maximum, Increment: increment, Precision: precision}
}

func parseConstraints(optionID string, raw json.RawMessage) Constraints {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var values map[string]json.RawMessage
	if json.Unmarshal(raw, &values) != nil {
		return nil
	}
	constraints := make(Constraints)
	for selectedValue, selectedRaw := range values {
		if len(selectedRaw) == 0 || string(selectedRaw) == "null" {
			continue
		}
		var dependencies map[string][]any
		if json.Unmarshal(selectedRaw, &dependencies) != nil {
			continue
		}
		cleaned := make(map[string][]string)
		for dependency, incompatible := range dependencies {
			values := cleanPropertyValues(dependency, incompatible)
			if len(values) > 0 {
				cleaned[strings.TrimSpace(dependency)] = values
			}
		}
		if len(cleaned) > 0 {
			constraints[cleanPropertyValue(optionID, selectedValue)] = cleaned
		}
	}
	if len(constraints) == 0 {
		return nil
	}
	return constraints
}

func combinedOptionalBool(values ...any) *bool {
	specified := false
	for _, value := range values {
		if parsed, ok := optionalBool(value); ok {
			specified = true
			if !parsed {
				result := false
				return &result
			}
		}
	}
	if specified {
		result := true
		return &result
	}
	return nil
}

func optionalBool(value any) (bool, bool) {
	if value == nil {
		return false, false
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprint(value)))
	return parsed, err == nil
}

func valueAttribute(raw json.RawMessage, key string) any {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var attributes map[string]any
	if json.Unmarshal(raw, &attributes) != nil {
		return nil
	}
	return attributes[key]
}

func cleanBool(value any) bool {
	if value == nil {
		return false
	}
	parsed, err := strconv.ParseBool(strings.TrimSpace(fmt.Sprint(value)))
	return err == nil && parsed
}

func cleanInt(value any) int {
	if value == nil {
		return 0
	}
	parsed, err := strconv.Atoi(strings.TrimSpace(fmt.Sprint(value)))
	if err != nil {
		return 0
	}
	return parsed
}

func number(value any) (float64, bool) {
	if value == nil {
		return 0, false
	}
	parsed, err := strconv.ParseFloat(strings.TrimSpace(fmt.Sprint(value)), 64)
	return parsed, err == nil
}

func cleanPropertyValues(optionID string, values []any) []string {
	seen := make(map[string]struct{}, len(values))
	cleaned := make([]string, 0, len(values))
	for _, rawValue := range values {
		value := cleanPropertyValue(optionID, rawValue)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func cleanPropertyValue(optionID string, value any) string {
	if value == nil {
		return ""
	}
	text := fmt.Sprint(value)
	if strings.EqualFold(strings.TrimSpace(optionID), "EFOutProfile") {
		return text
	}
	return strings.Trim(strings.TrimSpace(text), "\ufeff")
}
