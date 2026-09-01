package pagevalues

import "testing"

func TestParseNormalizesPageListsAndRanges(t *testing.T) {
	selection, err := Parse("1, 3, 5-8, 7, 10 to 11", DefaultExpansionLimit)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Normalized != "1,3,5-8,10-11" || selection.MaxPage != 11 || len(selection.Pages) != 8 {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestParseMatchesObservedFieryCustomRange(t *testing.T) {
	selection, err := Parse("5 to 10", DefaultExpansionLimit)
	if err != nil {
		t.Fatal(err)
	}
	if selection.Normalized != "5-10" || len(selection.Pages) != 6 || selection.MaxPage != 10 {
		t.Fatalf("observed Fiery range normalization = %#v", selection)
	}
	if err := selection.ValidatePageCount(12); err != nil {
		t.Fatalf("5-10 should fit the observed 12-page source: %v", err)
	}
}

func TestParseRejectsInvalidOrExcessiveRanges(t *testing.T) {
	for _, input := range []string{"", "0", "4-2", "1,,2", "one"} {
		if _, err := Parse(input, DefaultExpansionLimit); err == nil {
			t.Fatalf("Parse(%q) unexpectedly passed", input)
		}
	}
	if _, err := Parse("1-11", 10); err == nil {
		t.Fatal("range beyond expansion limit unexpectedly passed")
	}
}

func TestSelectionValidatesImportedFilePageCount(t *testing.T) {
	selection, err := Parse("1-5,8", DefaultExpansionLimit)
	if err != nil {
		t.Fatal(err)
	}
	if err := selection.ValidatePageCount(8); err != nil {
		t.Fatalf("valid page count failed: %v", err)
	}
	if err := selection.ValidatePageCount(7); err == nil {
		t.Fatal("out-of-bounds page unexpectedly passed")
	}
	if err := selection.ValidatePageCount(0); err == nil {
		t.Fatal("unknown page count unexpectedly passed")
	}
}

func TestEquivalentIgnoresFormattingAndDuplicatePages(t *testing.T) {
	if !Equivalent("1,2,3,5", "1-3,5,5") {
		t.Fatal("equivalent page expressions did not match")
	}
	if Equivalent("1-3", "1-4") {
		t.Fatal("different page expressions matched")
	}
}
