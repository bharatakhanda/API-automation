package resultcompare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"api-automation/internal/reportxlsx"
)

func TestCompareIgnoresCompletionOrderJobIDAndDuration(t *testing.T) {
	first := reportxlsx.Result{JobID: "gio-1", JobName: "sample.pdf", Result: "PASS", Mode: "Print", DurationMS: 100, SetValues: map[string]string{"EFPageRange": "5-10"}, GetValues: map[string]string{"EFPageRange": "5-10"}}
	second := reportxlsx.Result{JobID: "gio-2", JobName: "sample.pdf", Result: "PASS", Mode: "Hold", DurationMS: 200, Lifecycle: "held"}
	gioPath := writeResults(t, "gio.jsonl", first, second)
	first.JobID, first.DurationMS = "wails-9", 999
	second.JobID, second.DurationMS = "wails-8", 888
	wailsPath := writeResults(t, "wails.jsonl", second, first)

	report, err := Compare(gioPath, wailsPath)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Equivalent || report.Gio.Records != 2 || report.Wails.Records != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestCompareReportsSemanticDifference(t *testing.T) {
	gioPath := writeResults(t, "gio.jsonl", reportxlsx.Result{JobName: "sample.pdf", Result: "PASS", Mode: "Hold", JobStatus: "held"})
	wailsPath := writeResults(t, "wails.jsonl", reportxlsx.Result{JobName: "sample.pdf", Result: "FAIL", Mode: "Hold", JobStatus: "error"})

	report, err := Compare(gioPath, wailsPath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Equivalent || len(report.OnlyGio) != 1 || len(report.OnlyWails) != 1 {
		t.Fatalf("difference was not reported: %+v", report)
	}
	if report.OnlyGio[0].Sample.JobID != "" || report.OnlyGio[0].Sample.DurationMS != 0 {
		t.Fatal("volatile fields leaked into difference sample")
	}
}

func TestCompareBoundsDifferenceSamples(t *testing.T) {
	results := make([]reportxlsx.Result, maxDifferenceSamples+1)
	for index := range results {
		results[index] = reportxlsx.Result{JobName: "sample.pdf", Result: "PASS", Detail: string(rune(index + 1))}
	}
	report, err := Compare(writeResults(t, "gio.jsonl", results...), writeResults(t, "wails.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if report.OnlyGioRecords != maxDifferenceSamples+1 || len(report.OnlyGio) != maxDifferenceSamples {
		t.Fatalf("unexpected bounded differences: records=%d samples=%d", report.OnlyGioRecords, len(report.OnlyGio))
	}
}

func TestCompareRejectsMalformedStore(t *testing.T) {
	gioPath := filepath.Join(t.TempDir(), "broken.jsonl")
	if err := os.WriteFile(gioPath, []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wailsPath := writeResults(t, "wails.jsonl")
	if _, err := Compare(gioPath, wailsPath); err == nil {
		t.Fatal("expected malformed store error")
	}
}

func writeResults(t *testing.T, name string, results ...reportxlsx.Result) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	encoder := json.NewEncoder(file)
	for _, result := range results {
		if err := encoder.Encode(result); err != nil {
			file.Close()
			t.Fatal(err)
		}
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}
