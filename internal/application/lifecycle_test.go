package application

import "testing"

func TestActivelyProcessingJobClassification(t *testing.T) {
	for _, attributes := range []map[string]string{
		{"status": "printing"},
		{"state": "Processing"},
		{"is printing?": "yes"},
		{"is ripping?": "true"},
	} {
		if active, _ := ActivelyProcessingJob(attributes); !active {
			t.Fatalf("attributes %#v should be active", attributes)
		}
	}
	for _, attributes := range []map[string]string{
		{"status": "done printing"},
		{"state": "held"},
		{"status": "queued for printing"},
		{"is printing?": "no"},
	} {
		if active, _ := ActivelyProcessingJob(attributes); active {
			t.Fatalf("attributes %#v should not be active", attributes)
		}
	}
}

func TestCancelableJobSupportsFieryLifecycleScenarios(t *testing.T) {
	for _, attributes := range []map[string]string{
		{"state": "processing"},
		{"status": "ripping"},
		{"is ripping?": "yes"},
		{"status": "waiting to print"},
		{"queued for printing?": "yes"},
		{"job release state": "production"},
		{"status": "printing"},
		{"is printing?": "yes"},
	} {
		if cancelable, _ := CancelableJob(attributes); !cancelable {
			t.Fatalf("attributes %#v should be cancelable", attributes)
		}
	}
	for _, attributes := range []map[string]string{
		{"status": "spooling"},
		{"status": "done ripping"},
		{"status": "done printing"},
		{"state": "held"},
	} {
		if cancelable, _ := CancelableJob(attributes); cancelable {
			t.Fatalf("attributes %#v should not be cancelable", attributes)
		}
	}
}

func TestCancelObservedRequiresCancellationOrNonCompletedStop(t *testing.T) {
	for _, attributes := range []map[string]string{
		{"status": "cancelled"},
		{"recent action": "cancel"},
		{"status": "held", "is printing?": "no"},
	} {
		if !CancelObserved(attributes) {
			t.Fatalf("attributes %#v should acknowledge cancellation", attributes)
		}
	}
	for _, attributes := range []map[string]string{
		{"status": "printing"},
		{"status": "waiting to print"},
		{"status": "done printing"},
		{},
	} {
		if CancelObserved(attributes) {
			t.Fatalf("attributes %#v should not acknowledge cancellation", attributes)
		}
	}
}
