package application

import (
	"context"
	"errors"
	"testing"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
)

func TestPageRangeSerializationValidationAndReadback(t *testing.T) {
	combination := combinations.Combination{
		PageRangeOptionID:     PageRangeInternalPrefix + "1,3,5-7",
		PageRangeLegacyDataID: "stale-value",
	}
	attributes := CombinationToAttributes(combination)
	if attributes[PageRangeOptionID] != "1,3,5-7" {
		t.Fatalf("serialized range = %#v", attributes)
	}
	if _, exists := attributes[PageRangeLegacyDataID]; exists {
		t.Fatalf("stale legacy companion survived: %#v", attributes)
	}
	if combination[PageRangeOptionID] != PageRangeInternalPrefix+"1,3,5-7" {
		t.Fatalf("source combination was mutated: %#v", combination)
	}
	if err := ValidateCustomPageRange(attributes, map[string]string{"OrigPageCount": "7"}); err != nil {
		t.Fatalf("valid range failed: %v", err)
	}
	if err := ValidateCustomPageRange(attributes, map[string]string{"OrigPageCount": "6"}); err == nil {
		t.Fatal("range beyond original page count unexpectedly passed")
	}

	matcher := AttributeMatcher{}
	if !matcher.AttributesMatch(map[string]string{PageRangeOptionID: "1,3,5,6,7", PageRangeLegacyDataID: ""}, attributes) {
		t.Fatal("equivalent direct page-range readback did not match")
	}
	if matcher.AttributesMatch(map[string]string{PageRangeOptionID: "", PageRangeLegacyDataID: "1,3,5,6,7"}, attributes) {
		t.Fatal("legacy-only page-range readback incorrectly matched")
	}
	if matcher.AttributesMatch(map[string]string{PageRangeOptionID: "1-2", PageRangeLegacyDataID: "1,3,5-7"}, attributes) {
		t.Fatal("wrong direct page range was hidden by legacy data")
	}
}

func TestAdvertisedRange1RemainsExactValue(t *testing.T) {
	attributes := CombinationToAttributes(combinations.Combination{PageRangeOptionID: PageRangeRangeValue})
	if attributes[PageRangeOptionID] != PageRangeRangeValue {
		t.Fatalf("attributes = %#v", attributes)
	}
	if (AttributeMatcher{}).AttributeMapValueMatches(map[string]string{PageRangeLegacyDataID: "1-5"}, PageRangeOptionID, PageRangeRangeValue) {
		t.Fatal("Range1 matched unrelated legacy expression")
	}
}

func TestOutputProfilePreservesWireIdentityAndNormalizesDisplayComparison(t *testing.T) {
	visible := "360x360 CMYKOV v3F - Fiery Edge - Vela"
	wire := "\ufeff" + visible
	attributes := CombinationToAttributes(combinations.Combination{OutputProfileOptionID: wire})
	if attributes[OutputProfileOptionID] != wire {
		t.Fatalf("wire identity changed: %q", attributes[OutputProfileOptionID])
	}
	if got := DisplayOptionValue(OutputProfileOptionID, wire); got != visible {
		t.Fatalf("display value = %q", got)
	}
	matcher := AttributeMatcher{Capabilities: capabilities.Model{Options: []capabilities.Option{{ID: OutputProfileOptionID, Value: "DEFAULT_MEDIA"}}}}
	if !matcher.AttributeValueMatches(OutputProfileOptionID, wire, visible) {
		t.Fatal("BOM-prefixed output profile did not match visible value")
	}
	if !OptionValueMatches(OutputProfileOptionID, wire, visible) {
		t.Fatal("legacy BOM-less preset did not match exact profile")
	}
	if matcher.AttributeValueMatches(OutputProfileOptionID, visible+" changed", visible) {
		t.Fatal("different profile names matched")
	}
	if !matcher.AttributeValueMatches(OutputProfileOptionID, "", "DEFAULT_MEDIA") {
		t.Fatal("omitted discovered default did not match")
	}
}

func TestImportedFilePageCountPrefersOriginalDocumentCount(t *testing.T) {
	count, ok := ImportedFilePageCount(map[string]string{"OrigPageCount": "10", "num pages": "12"})
	if !ok || count != 10 {
		t.Fatalf("page count = %d, %t", count, ok)
	}
	if _, ok := ImportedFilePageCount(map[string]string{"num pages": "0"}); ok {
		t.Fatal("zero page count unexpectedly accepted")
	}
}

func TestExpectedConstraintRejectionRejectsOperationalErrors(t *testing.T) {
	if !ExpectedConstraintRejection(errors.New("HTTP 422 invalid constraint combination")) {
		t.Fatal("explicit 422 constraint rejection was not accepted")
	}
	for _, err := range []error{
		errors.New("HTTP 500 constraint service crashed"),
		errors.New("HTTP 404 endpoint not found"),
		errors.New("HTTP 400 unrelated bad request"),
		errors.New("HTTP 400 invalid JSON payload"),
		context.DeadlineExceeded,
	} {
		if ExpectedConstraintRejection(err) {
			t.Fatalf("operational error treated as expected rejection: %v", err)
		}
	}
}

func TestSelectedReadbackValuesIncludesOnlySelectedKeys(t *testing.T) {
	selected := map[string]string{"EFResolution": "360x720dpi", "EFColorMode": "CMYK"}
	got := map[string]string{"EFResolution": "360x360dpi", "EFColorMode": "CMYK", "unselected": "ignored"}
	readback := SelectedReadbackValues(got, selected)
	if len(readback) != 2 || readback["EFResolution"] != "360x360dpi" || readback["EFColorMode"] != "CMYK" {
		t.Fatalf("readback = %#v", readback)
	}
}
