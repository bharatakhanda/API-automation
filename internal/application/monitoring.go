package application

import (
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"api-automation/internal/fiery"
)

type OverviewMonitorPolicy struct {
	StatusInterval time.Duration
	JobInterval    time.Duration
	JobProbeLimit  int
	AttemptTimeout time.Duration
	MaximumBackoff time.Duration
}

func DefaultOverviewMonitorPolicy() OverviewMonitorPolicy {
	return OverviewMonitorPolicy{
		StatusInterval: time.Second,
		JobInterval:    2 * time.Second,
		JobProbeLimit:  64,
		AttemptTimeout: 20 * time.Second,
		MaximumBackoff: 30 * time.Second,
	}
}

func FailureBackoff(failures int, base, maximum time.Duration) time.Duration {
	if base <= 0 {
		base = time.Second
	}
	if maximum <= 0 {
		maximum = 30 * time.Second
	}
	if failures < 1 {
		return base
	}
	shift := failures
	if shift > 4 {
		shift = 4
	}
	delay := time.Duration(1<<shift) * base
	if delay > maximum {
		return maximum
	}
	return delay
}

// GenerationGuard rejects stale monitor/capability updates without coupling
// background services to a frontend mutex.
type GenerationGuard struct {
	generation atomic.Uint64
}

func (guard *GenerationGuard) Next() uint64 {
	if guard == nil {
		return 0
	}
	return guard.generation.Add(1)
}

func (guard *GenerationGuard) Current() uint64 {
	if guard == nil {
		return 0
	}
	return guard.generation.Load()
}

func (guard *GenerationGuard) IsCurrent(generation uint64) bool {
	return guard != nil && generation == guard.generation.Load()
}

func EffectiveOverviewServerStateWithJobs(apiState, apiDetail string, workload fiery.JobWorkloadSummary) (string, string) {
	if workload.ActiveJobs < 1 {
		return apiState, apiDetail
	}
	detail := strings.TrimSpace(apiDetail)
	if detail == "" {
		detail = "Fiery API status pending"
	}
	evidence := strings.Trim(strings.Join([]string{workload.EvidenceStatus, workload.EvidenceState}, "/"), "/")
	if evidence == "" {
		evidence = "active job"
	}
	return "Busy", fmt.Sprintf("%s · Fiery inventory reports %d active job(s): %s", detail, workload.ActiveJobs, evidence)
}

func EffectiveOverviewServerState(apiState, apiDetail string, automationActive bool) (string, string) {
	if !automationActive {
		return apiState, apiDetail
	}
	detail := strings.TrimSpace(apiDetail)
	if detail == "" {
		detail = "Fiery API status pending"
	}
	return "Busy", detail + " · application automation active"
}
