package capabilities

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const NormalizationReportSchemaVersion = 3

// FilterSummary reconciles the raw server schema, normalized automation model,
// and exact controls rendered by Job Properties.
type FilterSummary struct {
	RawServerProperties        int            `json:"rawServerProperties"`
	IncludedProperties         int            `json:"includedProperties"`
	IncludedServerProperties   int            `json:"includedServerProperties"`
	SyntheticProperties        int            `json:"syntheticProperties"`
	DisplayedProperties        int            `json:"displayedProperties"`
	ExcludedProperties         int            `json:"excludedProperties"`
	ExcludedValues             int            `json:"excludedValues"`
	ConstrainedProperties      int            `json:"constrainedProperties"`
	ExcludedPropertiesByReason map[string]int `json:"excludedPropertiesByReason"`
}

type DisplayedOptionDecision struct {
	Category        string `json:"category"`
	Section         string `json:"section,omitempty"`
	InclusionReason string `json:"inclusionReason"`
	Property        Option `json:"property"`
}

type FilterAudit struct {
	Summary          FilterSummary             `json:"summary"`
	DisplayedOptions []DisplayedOptionDecision `json:"displayedOptions"`
}

type normalizationReport struct {
	SchemaVersion    int                       `json:"schemaVersion"`
	CapturedAt       time.Time                 `json:"capturedAt"`
	ServerName       string                    `json:"serverName,omitempty"`
	PressModel       string                    `json:"pressModel,omitempty"`
	SerialNumber     string                    `json:"serialNumber,omitempty"`
	Version          string                    `json:"version,omitempty"`
	TimeZone         string                    `json:"timeZone,omitempty"`
	Locale           string                    `json:"locale,omitempty"`
	UptimeSeconds    int64                     `json:"uptimeSeconds,omitempty"`
	DiskAvailable    int64                     `json:"diskAvailable,omitempty"`
	DiskTotal        int64                     `json:"diskTotal,omitempty"`
	MemoryAvailable  int64                     `json:"memoryAvailable,omitempty"`
	MemoryTotal      int64                     `json:"memoryTotal,omitempty"`
	JobsTotal        int                       `json:"jobsTotal,omitempty"`
	ActiveJobs       int                       `json:"activeJobs,omitempty"`
	ActiveJobID      string                    `json:"activeJobId,omitempty"`
	ActiveJobStatus  string                    `json:"activeJobStatus,omitempty"`
	ActiveJobState   string                    `json:"activeJobState,omitempty"`
	Queues           []Queue                   `json:"queues,omitempty"`
	FilterSummary    FilterSummary             `json:"filterSummary"`
	DisplayedOptions []DisplayedOptionDecision `json:"displayedOptions"`
	IncludedOptions  []Option                  `json:"includedOptions"`
	ExcludedOptions  []ExcludedOption          `json:"excludedOptions"`
	ExcludedValues   []ExcludedValue           `json:"excludedValues"`
}

// BuildFilterAudit records exactly what Job Properties renders, not merely what
// survived parsing. This makes normalization-versus-UI disagreements explicit.
func BuildFilterAudit(model Model) FilterAudit {
	audit := FilterAudit{Summary: FilterSummary{ExcludedPropertiesByReason: make(map[string]int)}}
	for _, option := range model.Options {
		if option.Synthetic {
			audit.Summary.SyntheticProperties++
		} else {
			audit.Summary.IncludedServerProperties++
		}
	}
	audit.Summary.IncludedProperties = len(model.Options)
	audit.Summary.ExcludedProperties = len(model.ExcludedOptions)
	audit.Summary.ExcludedValues = len(model.ExcludedValues)
	audit.Summary.ConstrainedProperties = ConstraintCount(model)
	audit.Summary.RawServerProperties = audit.Summary.IncludedServerProperties + audit.Summary.ExcludedProperties
	for _, option := range model.ExcludedOptions {
		audit.Summary.ExcludedPropertiesByReason[option.Reason]++
	}
	for _, group := range GroupedOptions(model) {
		for _, section := range group.Sections {
			for _, option := range section.Options {
				audit.DisplayedOptions = append(audit.DisplayedOptions, DisplayedOptionDecision{
					Category: group.Name, Section: section.Name, InclusionReason: optionInclusionReason(option), Property: option,
				})
			}
		}
	}
	audit.Summary.DisplayedProperties = len(audit.DisplayedOptions)
	return audit
}

func optionInclusionReason(option Option) string {
	if option.Synthetic {
		return "application-standard synthetic control"
	}
	control := fmt.Sprintf("%s control", fallbackDiagnostic(option.PPDType, "server-advertised"))
	if option.PPDType == "uimenu" {
		control = fmt.Sprintf("menu with %d advertised alternatives", len(option.Values))
	} else if option.Range != nil {
		control = fmt.Sprintf("numeric range %.9g..%.9g increment %.9g", option.Range.Min, option.Range.Max, option.Range.Increment)
	}
	var direct []string
	for _, scope := range option.Scopes {
		if _, accepted := jobApplicabilityScopes[strings.ToLower(strings.TrimSpace(scope))]; accepted {
			direct = append(direct, scope)
		}
	}
	return fmt.Sprintf("eligible %s; direct job/CWS scopes [%s]", control, strings.Join(direct, ", "))
}

func fallbackDiagnostic(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// SaveNormalizationReport records the exact metadata-driven include/exclude
// decisions and UI rendering audit next to the raw capability snapshot. It
// contains no credentials, cookies, or API keys.
func SaveNormalizationReport(model Model, capturedAt time.Time, dir string) (string, error) {
	if dir == "" {
		return "", fmt.Errorf("capture directory is required")
	}
	if capturedAt.IsZero() {
		capturedAt = time.Now().UTC()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	audit := BuildFilterAudit(model)
	report := normalizationReport{
		SchemaVersion:    NormalizationReportSchemaVersion,
		CapturedAt:       capturedAt,
		ServerName:       model.ServerName,
		PressModel:       model.PressModel,
		SerialNumber:     model.SerialNumber,
		Version:          model.Version,
		TimeZone:         model.TimeZone,
		Locale:           model.Locale,
		UptimeSeconds:    model.UptimeSeconds,
		DiskAvailable:    model.DiskAvailable,
		DiskTotal:        model.DiskTotal,
		MemoryAvailable:  model.MemoryAvailable,
		MemoryTotal:      model.MemoryTotal,
		JobsTotal:        model.JobsTotal,
		ActiveJobs:       model.ActiveJobs,
		ActiveJobID:      model.ActiveJobID,
		ActiveJobStatus:  model.ActiveJobStatus,
		ActiveJobState:   model.ActiveJobState,
		Queues:           model.Queues,
		FilterSummary:    audit.Summary,
		DisplayedOptions: audit.DisplayedOptions,
		IncludedOptions:  model.Options,
		ExcludedOptions:  model.ExcludedOptions,
		ExcludedValues:   model.ExcludedValues,
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "normalized-capabilities-"+capturedAt.Format("20060102-150405")+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}
