package application

import (
	"strings"
	"testing"
	"time"

	"api-automation/internal/fiery"
)

func TestOverviewMonitorPolicyAndGenerationGuards(t *testing.T) {
	policy := DefaultOverviewMonitorPolicy()
	if policy.StatusInterval != time.Second || policy.JobInterval != 2*time.Second || policy.JobProbeLimit != 64 || policy.MaximumBackoff != 30*time.Second {
		t.Fatalf("policy = %#v", policy)
	}
	if delay := FailureBackoff(1, policy.JobInterval, policy.MaximumBackoff); delay != 4*time.Second {
		t.Fatalf("first failure delay = %s", delay)
	}
	if delay := FailureBackoff(100, policy.JobInterval, policy.MaximumBackoff); delay != 30*time.Second {
		t.Fatalf("bounded delay = %s", delay)
	}
	guard := new(GenerationGuard)
	first := guard.Next()
	second := guard.Next()
	if guard.IsCurrent(first) || !guard.IsCurrent(second) {
		t.Fatalf("generation first=%d second=%d current=%d", first, second, guard.Current())
	}
}

func TestEffectiveOverviewStateCombinesExternalAndApplicationWork(t *testing.T) {
	workload := fiery.JobWorkloadSummary{ActiveJobs: 4, EvidenceStatus: "ripping", EvidenceState: "processing"}
	state, detail := EffectiveOverviewServerStateWithJobs("Idle", "API running", workload)
	if state != "Busy" || !strings.Contains(detail, "4 active job") || !strings.Contains(detail, "ripping/processing") {
		t.Fatalf("external state=%q detail=%q", state, detail)
	}
	state, detail = EffectiveOverviewServerState("Idle", "API running", true)
	if state != "Busy" || !strings.Contains(detail, "automation active") {
		t.Fatalf("application state=%q detail=%q", state, detail)
	}
}
