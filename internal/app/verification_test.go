package app

import "testing"

func TestVerifyAttributesPassesWhenSetAndGetValuesMatch(t *testing.T) {
	got := verifyAttributes(
		map[string]string{"EFResolution": "360x720dpi", "EFColorMode": "CMYKPLUS"},
		map[string]string{"EFResolution": "360x720dpi", "EFColorMode": "CMYKPLUS"},
	)
	if !got.Passed {
		t.Fatalf("expected pass, failures=%v", got.Failures)
	}
}

func TestVerifyAttributesFailsWhenSetAndGetValuesDiffer(t *testing.T) {
	got := verifyAttributes(
		map[string]string{"EFResolution": "360x720dpi"},
		map[string]string{"EFResolution": "360x360dpi"},
	)
	if got.Passed {
		t.Fatal("expected failure")
	}
	if len(got.Failures) != 1 {
		t.Fatalf("failures=%v", got.Failures)
	}
}
