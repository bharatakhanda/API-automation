package resultcompare

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"api-automation/internal/reportxlsx"
)

func TestCompareIgnoresCompletionOrderJobIDAndDuration(t *testing.T) {
	first := reportxlsx.Result{JobID: "baseline-1", JobName: "sample.pdf", Result: "PASS", Mode: "Print", DurationMS: 100, SetValues: map[string]string{"EFPageRange": "5-10"}, GetValues: map[string]string{"EFPageRange": "5-10"}}
	second := reportxlsx.Result{JobID: "baseline-2", JobName: "sample.pdf", Result: "PASS", Mode: "Hold", DurationMS: 200, Lifecycle: "held"}
	baselinePath := writeResults(t, "baseline.jsonl", first, second)
	first.JobID, first.DurationMS = "candidate-9", 999
	second.JobID, second.DurationMS = "candidate-8", 888
	candidatePath := writeResults(t, "candidate.jsonl", second, first)

	report, err := Compare(baselinePath, candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Equivalent || report.Baseline.Records != 2 || report.Candidate.Records != 2 {
		t.Fatalf("unexpected report: %+v", report)
	}
}

func TestCompareReportsSemanticDifference(t *testing.T) {
	baselinePath := writeResults(t, "baseline.jsonl", reportxlsx.Result{JobName: "sample.pdf", Result: "PASS", Mode: "Hold", JobStatus: "held"})
	candidatePath := writeResults(t, "candidate.jsonl", reportxlsx.Result{JobName: "sample.pdf", Result: "FAIL", Mode: "Hold", JobStatus: "error"})

	report, err := Compare(baselinePath, candidatePath)
	if err != nil {
		t.Fatal(err)
	}
	if report.Equivalent || len(report.OnlyBaseline) != 1 || len(report.OnlyCandidate) != 1 {
		t.Fatalf("difference was not reported: %+v", report)
	}
	if report.OnlyBaseline[0].Sample.JobID != "" || report.OnlyBaseline[0].Sample.DurationMS != 0 {
		t.Fatal("volatile fields leaked into difference sample")
	}
}

func TestCompareBoundsDifferenceSamples(t *testing.T) {
	results := make([]reportxlsx.Result, maxDifferenceSamples+1)
	for index := range results {
		results[index] = reportxlsx.Result{JobName: "sample.pdf", Result: "PASS", Detail: string(rune(index + 1))}
	}
	report, err := Compare(writeResults(t, "baseline.jsonl", results...), writeResults(t, "candidate.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if report.OnlyBaselineRecords != maxDifferenceSamples+1 || len(report.OnlyBaseline) != maxDifferenceSamples {
		t.Fatalf("unexpected bounded differences: records=%d samples=%d", report.OnlyBaselineRecords, len(report.OnlyBaseline))
	}
}

func TestCompareRejectsMalformedStore(t *testing.T) {
	baselinePath := filepath.Join(t.TempDir(), "broken.jsonl")
	if err := os.WriteFile(baselinePath, []byte("{not-json}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	candidatePath := writeResults(t, "candidate.jsonl")
	if _, err := Compare(baselinePath, candidatePath); err == nil {
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
