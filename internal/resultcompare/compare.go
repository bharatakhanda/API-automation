// Package resultcompare compares baseline and candidate disk-backed automation
// results without depending on completion order, generated Fiery job IDs, or timing.
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
	Equivalent           bool         `json:"equivalent"`
	Baseline             Summary      `json:"baseline"`
	Candidate            Summary      `json:"candidate"`
	OnlyBaselineRecords  int          `json:"onlyBaselineRecords"`
	OnlyCandidateRecords int          `json:"onlyCandidateRecords"`
	OnlyBaseline         []Difference `json:"onlyBaseline,omitempty"`
	OnlyCandidate        []Difference `json:"onlyCandidate,omitempty"`
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

func Compare(baselinePath, candidatePath string) (Report, error) {
	baseline, err := read(baselinePath)
	if err != nil {
		return Report{}, fmt.Errorf("read baseline results: %w", err)
	}
	candidate, err := read(candidatePath)
	if err != nil {
		return Report{}, fmt.Errorf("read candidate results: %w", err)
	}
	report := Report{Baseline: baseline.summary, Candidate: candidate.summary}
	for digest, baselineCount := range baseline.counts {
		if difference := baselineCount - candidate.counts[digest]; difference > 0 {
			report.OnlyBaselineRecords += difference
			report.OnlyBaseline = append(report.OnlyBaseline, Difference{Digest: digest, Count: difference, Sample: baseline.samples[digest]})
		}
	}
	for digest, candidateCount := range candidate.counts {
		if difference := candidateCount - baseline.counts[digest]; difference > 0 {
			report.OnlyCandidateRecords += difference
			report.OnlyCandidate = append(report.OnlyCandidate, Difference{Digest: digest, Count: difference, Sample: candidate.samples[digest]})
		}
	}
	sort.Slice(report.OnlyBaseline, func(i, j int) bool { return report.OnlyBaseline[i].Digest < report.OnlyBaseline[j].Digest })
	sort.Slice(report.OnlyCandidate, func(i, j int) bool { return report.OnlyCandidate[i].Digest < report.OnlyCandidate[j].Digest })
	if len(report.OnlyBaseline) > maxDifferenceSamples {
		report.OnlyBaseline = report.OnlyBaseline[:maxDifferenceSamples]
	}
	if len(report.OnlyCandidate) > maxDifferenceSamples {
		report.OnlyCandidate = report.OnlyCandidate[:maxDifferenceSamples]
	}
	report.Equivalent = report.OnlyBaselineRecords == 0 && report.OnlyCandidateRecords == 0
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
