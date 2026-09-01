package capabilities

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSaveNormalizationReportRetainsDecisionMetadataAndWireValues(t *testing.T) {
	capturedAt := time.Date(2026, 9, 2, 3, 4, 5, 0, time.UTC)
	model := Model{
		ServerName: "SERVER-1", TimeZone: "UTC", UptimeSeconds: 42, DiskAvailable: 100, DiskTotal: 200,
		Options: []Option{{
			ID: "EFOutProfile", Values: []string{"\ufeffProfile A"}, Group: "fpcolorwise", PPDType: "uimenu",
		}},
		ExcludedOptions: []ExcludedOption{{
			ID: "HiddenSetting", Reason: "hidden", Property: Option{ID: "HiddenSetting", Hidden: true},
		}},
	}
	path, err := SaveNormalizationReport(model, capturedAt, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(path, "normalized-capabilities-20260902-030405.json") {
		t.Fatalf("report path = %q", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var report normalizationReport
	if err := json.Unmarshal(data, &report); err != nil {
		t.Fatal(err)
	}
	if report.SchemaVersion != NormalizationReportSchemaVersion || report.TimeZone != "UTC" || report.UptimeSeconds != 42 || report.DiskAvailable != 100 || report.DiskTotal != 200 || len(report.IncludedOptions) != 1 || report.IncludedOptions[0].Values[0] != "\ufeffProfile A" {
		t.Fatalf("included report metadata = %#v", report)
	}
	if len(report.ExcludedOptions) != 1 || !report.ExcludedOptions[0].Property.Hidden {
		t.Fatalf("excluded report metadata = %#v", report.ExcludedOptions)
	}
	if report.FilterSummary.RawServerProperties != 2 || report.FilterSummary.IncludedProperties != 1 || report.FilterSummary.DisplayedProperties != 1 || report.FilterSummary.ExcludedProperties != 1 || report.FilterSummary.ExcludedPropertiesByReason["hidden"] != 1 {
		t.Fatalf("filter summary = %#v", report.FilterSummary)
	}
	if len(report.DisplayedOptions) != 1 || report.DisplayedOptions[0].Category != "Color" || report.DisplayedOptions[0].Property.ID != "EFOutProfile" || !strings.Contains(report.DisplayedOptions[0].InclusionReason, "eligible menu") {
		t.Fatalf("displayed option audit = %#v", report.DisplayedOptions)
	}
}
