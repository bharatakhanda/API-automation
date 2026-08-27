package capabilities

import (
	"encoding/json"
	"fmt"
	"sort"
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
	if json.Unmarshal(body, &payload) == nil {
		model.ServerName = payload.Data.Item.Name
		model.SerialNumber = payload.Data.Item.SerialNumber
		model.Version = payload.Data.Item.Version
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
				ID     string   `json:"id"`
				Value  any      `json:"value"`
				Values []any    `json:"values"`
				Scopes []string `json:"scopes"`
			} `json:"items"`
		} `json:"data"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	options := make([]Option, 0, len(importantPropertyLabels))
	for _, item := range payload.Data.Items {
		label, important := importantPropertyLabels[item.ID]
		if !important {
			continue
		}
		values := cleanValues(item.Values)
		options = append(options, Option{
			ID:      item.ID,
			Label:   label,
			Value:   cleanValue(item.Value),
			Values:  values,
			Scopes:  append([]string(nil), item.Scopes...),
			Enabled: len(values) > 0,
		})
	}
	sort.Slice(options, func(i, j int) bool { return options[i].Label < options[j].Label })
	return options
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
	return strings.Trim(strings.TrimSpace(fmt.Sprint(value)), "\ufeff")
}
