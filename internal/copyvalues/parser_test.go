package copyvalues

import (
	"reflect"
	"testing"
)

func TestParseIndividualCopyValues(t *testing.T) {
	got, err := Parse("1, 5, 10, 15")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1", "5", "10", "15"}
	if !reflect.DeepEqual(got.Values, want) || got.HasRange {
		t.Fatalf("selection = %#v, want values %#v without range", got, want)
	}
}

func TestParseSingleCopyValue(t *testing.T) {
	got, err := Parse("999")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.Values, []string{"999"}) {
		t.Fatalf("selection = %#v", got)
	}
}

func TestParseInclusiveCopyRanges(t *testing.T) {
	for _, input := range []string{"5-10", "5 to 10", "5 TO 10"} {
		t.Run(input, func(t *testing.T) {
			got, err := Parse(input)
			if err != nil {
				t.Fatal(err)
			}
			want := []string{"5", "6", "7", "8", "9", "10"}
			if !reflect.DeepEqual(got.Values, want) || !got.HasRange {
				t.Fatalf("selection = %#v, want %#v with range", got, want)
			}
		})
	}
}

func TestParseMixedValuesRangesAndDuplicates(t *testing.T) {
	got, err := Parse("1, 5-7, 7, 10")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1", "5", "6", "7", "10"}
	if !reflect.DeepEqual(got.Values, want) {
		t.Fatalf("selection = %#v, want %#v", got, want)
	}
}

func TestParseRejectsInvalidCopies(t *testing.T) {
	for _, input := range []string{"", "0", "10000", "10-5", "1,,2", "1.5", "abc"} {
		t.Run(input, func(t *testing.T) {
			if _, err := Parse(input); err == nil {
				t.Fatalf("Parse(%q) unexpectedly succeeded", input)
			}
		})
	}
}
