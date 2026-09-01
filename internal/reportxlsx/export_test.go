package reportxlsx

import (
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/xuri/excelize/v2"
)

func TestExportCreatesSummaryAndDynamicSetGetColumns(t *testing.T) {
	started := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	store, err := NewResultStore(t.TempDir(), started)
	if err != nil {
		t.Fatal(err)
	}
	results := []Result{
		{JobID: "JOB-1", JobName: "first.pdf", Result: "PASS", Mode: "Process and Hold", JobStatus: "done ripping", JobState: "processed", Lifecycle: "processed with raster", DurationMS: 1200, SetValues: map[string]string{"EFResolution": "360x720dpi"}, GetValues: map[string]string{"EFResolution": "360x720dpi"}},
		{JobID: "JOB-2", JobName: "second.pdf", Result: "FAIL", DurationMS: 800, SetValues: map[string]string{"EFResolution": "360x720dpi"}, GetValues: map[string]string{"EFResolution": "360x360dpi"}},
	}
	for _, result := range results {
		if err := store.Append(result); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "results.xlsx")
	stats, err := Export(path, Report{
		Summary: Summary{
			StartedAt:         started,
			CompletedAt:       started.Add(2 * time.Second),
			Status:            "Completed",
			ServerIP:          "server.example",
			ServerName:        "Fiery Server",
			SerialNumber:      "SERIAL-1",
			ServerVersion:     "1.4",
			SessionLoginPath:  "/live/api/v5",
			QueuesDiscovered:  2,
			OptionsDiscovered: 50,
			TestFileCount:     2,
			CombinationCount:  1,
			PlannedTests:      2,
			Workers:           1,
			Strategy:          "selected",
			RunModes:          []string{"Hold"},
		},
		ResultsPath:     store.Path(),
		AttributeLabels: map[string]string{"EFResolution": "Resolution"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if stats.Total != 2 || stats.Passed != 1 || stats.Failed != 1 || stats.Errors != 0 || stats.TotalDurationMS != 2000 {
		t.Fatalf("unexpected stats: %#v", stats)
	}

	workbook, err := excelize.OpenFile(path)
	if err != nil {
		t.Fatal(err)
	}
	defer workbook.Close()
	if got := workbook.GetSheetList(); len(got) != 2 || got[0] != "Summary" || got[1] != "Results" {
		t.Fatalf("sheet list = %#v", got)
	}
	assertCell(t, workbook, "Summary", "A1", "Fiery API Automation Test Report")
	assertCell(t, workbook, "Summary", "A22", "Executed tests")
	assertCell(t, workbook, "Summary", "B22", "2")
	assertCell(t, workbook, "Results", "A1", "Job ID")
	assertCell(t, workbook, "Results", "B1", "Job Name")
	assertCell(t, workbook, "Results", "C1", "Result")
	assertCell(t, workbook, "Results", "D1", "Mode")
	assertCell(t, workbook, "Results", "E1", "Job Status")
	assertCell(t, workbook, "Results", "F1", "Job State")
	assertCell(t, workbook, "Results", "H1", "Lifecycle Verification")
	assertCell(t, workbook, "Results", "I1", "Resolution")
	assertCell(t, workbook, "Results", "I2", "Set Value")
	assertCell(t, workbook, "Results", "J2", "Get Value")
	assertCell(t, workbook, "Results", "A3", "JOB-1")
	assertCell(t, workbook, "Results", "B3", "first.pdf")
	assertCell(t, workbook, "Results", "C3", "PASS")
	assertCell(t, workbook, "Results", "D3", "Process and Hold")
	assertCell(t, workbook, "Results", "E3", "done ripping")
	assertCell(t, workbook, "Results", "F3", "processed")
	assertCell(t, workbook, "Results", "I4", "360x720dpi")
	assertCell(t, workbook, "Results", "J4", "360x360dpi")
}

func TestResultStoreSupportsConcurrentWriters(t *testing.T) {
	store, err := NewResultStore(t.TempDir(), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	const workers, perWorker = 8, 25
	var wait sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		wait.Add(1)
		go func(worker int) {
			defer wait.Done()
			for index := 0; index < perWorker; index++ {
				if err := store.Append(Result{JobID: fmt.Sprintf("%d-%d", worker, index), Result: "PASS"}); err != nil {
					t.Errorf("append result: %v", err)
					return
				}
			}
		}(worker)
	}
	wait.Wait()
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	count := 0
	if err := forEachResult(store.Path(), func(Result) error { count++; return nil }); err != nil {
		t.Fatal(err)
	}
	if count != workers*perWorker {
		t.Fatalf("stored result count = %d, want %d", count, workers*perWorker)
	}
}

func assertCell(t *testing.T, workbook *excelize.File, sheet, cell, want string) {
	t.Helper()
	got, err := workbook.GetCellValue(sheet, cell)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("%s!%s = %q, want %q", sheet, cell, got, want)
	}
}
