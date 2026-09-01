package rangevalues

import (
	"reflect"
	"testing"
)

func TestParseValuesAndInclusiveRanges(t *testing.T) {
	bounds := Bounds{Min: 1, Max: 10, Increment: 1, Precision: 0}
	got, err := Parse("1, 3-5, 5 to 7", bounds, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1", "3", "4", "5", "6", "7"}
	if !reflect.DeepEqual(got.Values, want) || !got.HasRange {
		t.Fatalf("Parse() = %#v, want values %#v with range", got, want)
	}
}

func TestParseDecimalAndNegativeRange(t *testing.T) {
	bounds := Bounds{Min: -1, Max: 1, Increment: .5, Precision: 1}
	got, err := Parse("-1 to 1", bounds, 100)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-1.0", "-0.5", "0.0", "0.5", "1.0"}
	if !reflect.DeepEqual(got.Values, want) {
		t.Fatalf("values = %#v, want %#v", got.Values, want)
	}
}

func TestParseRejectsInvalidInput(t *testing.T) {
	bounds := Bounds{Min: 1, Max: 10, Increment: 2, Precision: 0}
	for _, input := range []string{"", "0", "11", "5-1", "2", "1,,3", "abc"} {
		if _, err := Parse(input, bounds, 100); err == nil {
			t.Errorf("Parse(%q) unexpectedly succeeded", input)
		}
	}
}

func TestParseEnforcesExpansionLimit(t *testing.T) {
	bounds := Bounds{Min: 1, Max: 100, Increment: 1, Precision: 0}
	if _, err := Parse("1-100", bounds, 10); err == nil {
		t.Fatal("large expansion unexpectedly succeeded")
	}
}
