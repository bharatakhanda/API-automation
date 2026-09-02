// Package resultcompare compares Gio and Wails disk-backed automation results
// without depending on completion order, generated Fiery job IDs, or timing.
package resultcompare

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"

	"api-automation/internal/reportxlsx"
)

type Summary struct {
	Records  int            `json:"records"`
	Verdicts map[string]int `json:"verdicts"`
	Modes    map[string]int `json:"modes"`
}

type Difference struct {
	Digest string            `json:"digest"`
	Count  int               `json:"count"`
	Sample reportxlsx.Result `json:"sample"`
}

type Report struct {
	Equivalent       bool         `json:"equivalent"`
	Gio              Summary      `json:"gio"`
	Wails            Summary      `json:"wails"`
	OnlyGioRecords   int          `json:"onlyGioRecords"`
	OnlyWailsRecords int          `json:"onlyWailsRecords"`
	OnlyGio          []Difference `json:"onlyGio,omitempty"`
	OnlyWails        []Difference `json:"onlyWails,omitempty"`
}

const maxDifferenceSamples = 100

type comparableResult struct {
	JobName   string            `json:"jobName"`
	Result    string            `json:"result"`
	Mode      string            `json:"mode,omitempty"`
	JobStatus string            `json:"jobStatus,omitempty"`
	JobState  string            `json:"jobState,omitempty"`
	JobError  string            `json:"jobError,omitempty"`
	LastEvent string            `json:"lastEvent,omitempty"`
	Lifecycle string            `json:"lifecycle,omitempty"`
	Detail    string            `json:"detail,omitempty"`
	SetValues map[string]string `json:"setValues,omitempty"`
	GetValues map[string]string `json:"getValues,omitempty"`
}

type recordSet struct {
	summary Summary
	counts  map[string]int
	samples map[string]reportxlsx.Result
}

func Compare(gioPath, wailsPath string) (Report, error) {
	gio, err := read(gioPath)
	if err != nil {
		return Report{}, fmt.Errorf("read Gio results: %w", err)
	}
	wails, err := read(wailsPath)
	if err != nil {
		return Report{}, fmt.Errorf("read Wails results: %w", err)
	}
	report := Report{Gio: gio.summary, Wails: wails.summary}
	for digest, gioCount := range gio.counts {
		if difference := gioCount - wails.counts[digest]; difference > 0 {
			report.OnlyGioRecords += difference
			report.OnlyGio = append(report.OnlyGio, Difference{Digest: digest, Count: difference, Sample: gio.samples[digest]})
		}
	}
	for digest, wailsCount := range wails.counts {
		if difference := wailsCount - gio.counts[digest]; difference > 0 {
			report.OnlyWailsRecords += difference
			report.OnlyWails = append(report.OnlyWails, Difference{Digest: digest, Count: difference, Sample: wails.samples[digest]})
		}
	}
	sort.Slice(report.OnlyGio, func(i, j int) bool { return report.OnlyGio[i].Digest < report.OnlyGio[j].Digest })
	sort.Slice(report.OnlyWails, func(i, j int) bool { return report.OnlyWails[i].Digest < report.OnlyWails[j].Digest })
	if len(report.OnlyGio) > maxDifferenceSamples {
		report.OnlyGio = report.OnlyGio[:maxDifferenceSamples]
	}
	if len(report.OnlyWails) > maxDifferenceSamples {
		report.OnlyWails = report.OnlyWails[:maxDifferenceSamples]
	}
	report.Equivalent = report.OnlyGioRecords == 0 && report.OnlyWailsRecords == 0
	return report, nil
}

func read(path string) (recordSet, error) {
	file, err := os.Open(path)
	if err != nil {
		return recordSet{}, err
	}
	defer file.Close()
	set := recordSet{
		summary: Summary{Verdicts: make(map[string]int), Modes: make(map[string]int)},
		counts:  make(map[string]int), samples: make(map[string]reportxlsx.Result),
	}
	decoder := json.NewDecoder(bufio.NewReaderSize(file, 64*1024))
	for {
		var result reportxlsx.Result
		if err := decoder.Decode(&result); err != nil {
			if err == io.EOF {
				return set, nil
			}
			return recordSet{}, fmt.Errorf("decode record %d: %w", set.summary.Records+1, err)
		}
		body, err := json.Marshal(comparableResult{
			JobName: result.JobName, Result: result.Result, Mode: result.Mode,
			JobStatus: result.JobStatus, JobState: result.JobState, JobError: result.JobError,
			LastEvent: result.LastEvent, Lifecycle: result.Lifecycle, Detail: result.Detail,
			SetValues: result.SetValues, GetValues: result.GetValues,
		})
		if err != nil {
			return recordSet{}, fmt.Errorf("normalize record %d: %w", set.summary.Records+1, err)
		}
		digestBytes := sha256.Sum256(body)
		digest := hex.EncodeToString(digestBytes[:])
		set.summary.Records++
		set.summary.Verdicts[result.Result]++
		set.summary.Modes[result.Mode]++
		set.counts[digest]++
		if _, exists := set.samples[digest]; !exists {
			result.JobID = ""
			result.DurationMS = 0
			set.samples[digest] = result
		}
	}
}
