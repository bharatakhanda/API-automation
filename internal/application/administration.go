package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	ClearAllJobsConfirmation = "CLEAR ALL JOBS"
	DefaultInventoryValidity = 2 * time.Minute
)

type AdministrationActivity struct {
	AutomationRunning  bool
	ManualJobAction    bool
	ConnectionTest     bool
	ServerOperation    bool
	InventoryOperation bool
}

func AdministrationPrecondition(activity AdministrationActivity) error {
	switch {
	case activity.AutomationRunning:
		return errors.New("server administration is blocked while capability capture or automation is running")
	case activity.ManualJobAction:
		return errors.New("server administration is blocked while a manual job action is running")
	case activity.ConnectionTest:
		return errors.New("server administration is blocked while the connection test is running")
	case activity.ServerOperation || activity.InventoryOperation:
		return errors.New("wait for the current server administration operation to finish")
	default:
		return nil
	}
}

type InventorySnapshot struct {
	Server    string    `json:"server,omitempty"`
	Count     int       `json:"count"`
	Inspected time.Time `json:"inspected,omitempty"`
}

type AdministrationState struct {
	mu        sync.Mutex
	inventory InventorySnapshot
}

func (state *AdministrationState) RecordInventory(server string, count int, now time.Time) InventorySnapshot {
	if count < 0 {
		count = 0
	}
	if now.IsZero() {
		now = time.Now()
	}
	state.mu.Lock()
	state.inventory = InventorySnapshot{Server: strings.TrimSpace(server), Count: count, Inspected: now}
	snapshot := state.inventory
	state.mu.Unlock()
	return snapshot
}

func (state *AdministrationState) InvalidateInventory() {
	state.mu.Lock()
	state.inventory = InventorySnapshot{}
	state.mu.Unlock()
}

func (state *AdministrationState) Inventory() InventorySnapshot {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.inventory
}

func (state *AdministrationState) ValidateClear(server, confirmation string, now time.Time, validity time.Duration) (InventorySnapshot, error) {
	if confirmation != ClearAllJobsConfirmation {
		return InventorySnapshot{}, errors.New("clear all jobs blocked: enter the exact uppercase confirmation phrase")
	}
	if validity <= 0 {
		validity = DefaultInventoryValidity
	}
	if now.IsZero() {
		now = time.Now()
	}
	snapshot := state.Inventory()
	if snapshot.Server != strings.TrimSpace(server) || snapshot.Inspected.IsZero() || now.Sub(snapshot.Inspected) > validity {
		return InventorySnapshot{}, errors.New("clear all jobs blocked: inspect the current job count for this server first")
	}
	if snapshot.Count == 0 {
		return InventorySnapshot{}, errors.New("there are no inspected jobs to clear")
	}
	return snapshot, nil
}

type JobAdministrationClient interface {
	JobCount(context.Context) (int, error)
	ClearAllJobs(context.Context) error
}

type ClearJobsOutcome struct {
	Remaining       int
	Accepted        bool
	UpdateInventory bool
}

func RevalidateAndClearJobs(ctx context.Context, client JobAdministrationClient, expectedCount int, pollInterval time.Duration) (outcome ClearJobsOutcome, err error) {
	if client == nil {
		return outcome, errors.New("job administration client is unavailable")
	}
	count, err := client.JobCount(ctx)
	if err != nil {
		return outcome, fmt.Errorf("revalidate job inventory: %w", err)
	}
	if count != expectedCount {
		return ClearJobsOutcome{Remaining: count, UpdateInventory: true}, fmt.Errorf("job count changed from %d to %d; inspect and confirm again", expectedCount, count)
	}
	if err := client.ClearAllJobs(ctx); err != nil {
		return outcome, err
	}
	outcome.Accepted = true
	outcome.UpdateInventory = true
	if pollInterval <= 0 {
		pollInterval = 2 * time.Second
	}
	outcome.Remaining = count
	for {
		outcome.Remaining, err = client.JobCount(ctx)
		if err == nil && outcome.Remaining == 0 {
			return outcome, nil
		}
		timer := time.NewTimer(pollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			if err != nil {
				outcome.Remaining = max(outcome.Remaining, 0)
				return outcome, fmt.Errorf("verify empty job inventory: %w", err)
			}
			outcome.Remaining = max(outcome.Remaining, 0)
			return outcome, fmt.Errorf("verify empty job inventory: %w with %d job(s) remaining", ctx.Err(), outcome.Remaining)
		case <-timer.C:
		}
	}
}

type RecoveryProbe interface {
	Probe(context.Context) (string, error)
}

type RecoveryPolicy struct {
	InitialDelay time.Duration
	Interval     time.Duration
}

func DefaultRecoveryPolicy() RecoveryPolicy {
	return RecoveryPolicy{InitialDelay: 10 * time.Second, Interval: 5 * time.Second}
}

func WaitForRecovery(ctx context.Context, probe RecoveryProbe, policy RecoveryPolicy) error {
	if probe == nil {
		return errors.New("recovery probe is unavailable")
	}
	if policy.InitialDelay > 0 {
		timer := time.NewTimer(policy.InitialDelay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
	if policy.Interval <= 0 {
		policy.Interval = 5 * time.Second
	}
	var lastErr error
	for {
		status, err := probe.Probe(ctx)
		if err == nil {
			normalized := strings.ToLower(strings.TrimSpace(status))
			if normalized == "running" || normalized == "started" || normalized == "ready" {
				return nil
			}
			err = fmt.Errorf("fiery status is %q", status)
		}
		lastErr = err
		timer := time.NewTimer(policy.Interval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			if lastErr != nil {
				return fmt.Errorf("%w (last recovery check: %v)", ctx.Err(), lastErr)
			}
			return ctx.Err()
		case <-timer.C:
		}
	}
}
