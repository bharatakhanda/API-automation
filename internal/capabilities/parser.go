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
		case "properties", "v5_properties", "v4_properties":
			options := parseProperties(endpoint.Body)
			if len(options) > len(model.Options) {
				model.Options = options
			}
		}
	}
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
				Name         string `json:"name"`
				SerialNumber string `json:"serial_number"`
				Version      string `json:"version"`
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
		values := cleanValues(item.Values)
		value := cleanValue(item.Value)
		option := Option{
			ID:          id,
			Label:       label,
			Group:       strings.TrimSpace(item.Group),
			PPDType:     strings.ToLower(strings.TrimSpace(item.PPDType)),
			Value:       value,
			Values:      values,
			Scopes:      append([]string(nil), item.Scopes...),
			Enabled:     len(values) > 0 || value != "" || strings.EqualFold(item.PPDType, "efirange"),
			Editable:    cleanBool(item.Editable),
			Numeric:     cleanBool(item.Numeric),
			Length:      cleanInt(item.Length),
			Range:       parseNumericRange(item.PPDType, item.ValueAttributes),
			Constraints: parseConstraints(item.Constraints),
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

func parseConstraints(raw json.RawMessage) Constraints {
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
		for dependency, allowed := range dependencies {
			values := cleanValues(allowed)
			if len(values) > 0 {
				cleaned[strings.TrimSpace(dependency)] = values
			}
		}
		if len(cleaned) > 0 {
			constraints[cleanValue(selectedValue)] = cleaned
		}
	}
	if len(constraints) == 0 {
		return nil
	}
	return constraints
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

func cleanValues(values []any) []string {
	seen := make(map[string]struct{}, len(values))
	cleaned := make([]string, 0, len(values))
	for _, rawValue := range values {
		value := cleanValue(rawValue)
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

func cleanValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.Trim(strings.TrimSpace(fmt.Sprint(value)), "\ufeff")
}
