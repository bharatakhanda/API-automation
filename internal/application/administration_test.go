package application

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestAdministrationInterlocksAndInventoryLease(t *testing.T) {
	for name, activity := range map[string]AdministrationActivity{
		"automation": {AutomationRunning: true}, "job": {ManualJobAction: true}, "connection": {ConnectionTest: true}, "server": {ServerOperation: true}, "inventory": {InventoryOperation: true},
	} {
		if err := AdministrationPrecondition(activity); err == nil {
			t.Fatalf("%s activity did not block administration", name)
		}
	}
	state := new(AdministrationState)
	now := time.Unix(1000, 0)
	state.RecordInventory("fiery.example", 3, now)
	if _, err := state.ValidateClear("fiery.example", "wrong", now, DefaultInventoryValidity); err == nil {
		t.Fatal("wrong typed phrase was accepted")
	}
	if _, err := state.ValidateClear("other", ClearAllJobsConfirmation, now, DefaultInventoryValidity); err == nil {
		t.Fatal("inventory from another server was accepted")
	}
	if _, err := state.ValidateClear("fiery.example", ClearAllJobsConfirmation, now.Add(DefaultInventoryValidity+time.Second), DefaultInventoryValidity); err == nil {
		t.Fatal("expired inventory was accepted")
	}
	if snapshot, err := state.ValidateClear("fiery.example", ClearAllJobsConfirmation, now, DefaultInventoryValidity); err != nil || snapshot.Count != 3 {
		t.Fatalf("snapshot=%#v err=%v", snapshot, err)
	}
}

type fakeJobAdministrator struct {
	counts     []int
	clearCalls int
}

func (client *fakeJobAdministrator) JobCount(context.Context) (int, error) {
	if len(client.counts) == 0 {
		return 0, nil
	}
	count := client.counts[0]
	client.counts = client.counts[1:]
	return count, nil
}

func (client *fakeJobAdministrator) ClearAllJobs(context.Context) error {
	client.clearCalls++
	return nil
}

func TestRevalidateAndClearJobsRequiresStableCountAndEmptyVerification(t *testing.T) {
	client := &fakeJobAdministrator{counts: []int{3, 0}}
	outcome, err := RevalidateAndClearJobs(context.Background(), client, 3, time.Millisecond)
	if err != nil || outcome.Remaining != 0 || !outcome.Accepted || !outcome.UpdateInventory || client.clearCalls != 1 {
		t.Fatalf("outcome=%#v clearCalls=%d err=%v", outcome, client.clearCalls, err)
	}
	changed := &fakeJobAdministrator{counts: []int{4}}
	outcome, err = RevalidateAndClearJobs(context.Background(), changed, 3, time.Millisecond)
	if err == nil || outcome.Remaining != 4 || outcome.Accepted || !outcome.UpdateInventory || changed.clearCalls != 0 {
		t.Fatalf("changed outcome=%#v clearCalls=%d err=%v", outcome, changed.clearCalls, err)
	}
}

type fakeRecoveryProbe struct {
	statuses []string
	errors   []error
}

func (probe *fakeRecoveryProbe) Probe(context.Context) (string, error) {
	status := ""
	if len(probe.statuses) > 0 {
		status, probe.statuses = probe.statuses[0], probe.statuses[1:]
	}
	var err error
	if len(probe.errors) > 0 {
		err, probe.errors = probe.errors[0], probe.errors[1:]
	}
	return status, err
}

func TestWaitForRecoveryRequiresReadyStatus(t *testing.T) {
	probe := &fakeRecoveryProbe{statuses: []string{"", "starting", "ready"}, errors: []error{errors.New("offline"), nil, nil}}
	if err := WaitForRecovery(context.Background(), probe, RecoveryPolicy{Interval: time.Millisecond}); err != nil {
		t.Fatal(err)
	}
}
