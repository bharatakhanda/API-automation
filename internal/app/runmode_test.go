package app

import "testing"

func TestRunModeForIndex(t *testing.T) {
	tests := []struct {
		index  int
		label  string
		queue  string
		action string
	}{
		{index: 0, label: "Hold", queue: "hold"},
		{index: 1, label: "Process and Hold", queue: "hold", action: "rip"},
		{index: 2, label: "RIP", queue: "hold", action: "rip"},
		{index: 3, label: "Press Print", queue: "hold", action: "press_print"},
		{index: 4, label: "Ready to Print", queue: "hold", action: "press_print"},
		{index: 5, label: "Print", queue: "print", action: "print"},
	}
	for _, tt := range tests {
		got := runModeForIndex(tt.index)
		if got.Label != tt.label || got.ImportQueue != tt.queue {
			t.Fatalf("index %d got %#v", tt.index, got)
		}
		if tt.action == "" && len(got.Actions) != 0 {
			t.Fatalf("index %d expected no action, got %#v", tt.index, got.Actions)
		}
		if tt.action != "" && (len(got.Actions) != 1 || got.Actions[0] != tt.action) {
			t.Fatalf("index %d expected action %q, got %#v", tt.index, tt.action, got.Actions)
		}
	}
}
