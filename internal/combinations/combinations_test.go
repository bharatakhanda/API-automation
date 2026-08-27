package combinations

import "testing"

func TestGenerateCartesianProduct(t *testing.T) {
	got := Generate([]Axis{
		{Name: "EFResolution", Values: []string{"360x360dpi", "360x720dpi"}},
		{Name: "EFColorMode", Values: []string{"CMYK", "CMYKPLUS"}},
	}, -1)
	if len(got) != 4 {
		t.Fatalf("len = %d, want 4", len(got))
	}
}

func TestGenerateHonorsLimitAndIgnoresEmptyAxes(t *testing.T) {
	got := Generate([]Axis{
		{Name: "EFResolution", Values: []string{"360x360dpi", "360x720dpi"}},
		{Name: "Unsupported", Values: nil},
		{Name: "EFColorMode", Values: []string{"CMYK", "CMYKPLUS"}},
	}, 2)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func TestPairwiseCoversEveryTwoAxisValuePair(t *testing.T) {
	axes := []Axis{
		{Name: "EFResolution", Values: []string{"360x360dpi", "360x720dpi", "360x240dpi", "360x300dpi"}},
		{Name: "EFColorMode", Values: []string{"Grayscale", "CMYK", "CMYKPLUS"}},
		{Name: "EFMediaType", Values: []string{"ContainerBoardsCoatedWhiteTop", "ContainerBoardsDblCoatedWhite"}},
		{Name: "EFPrintSpeed", Values: []string{"Standard", "High"}},
		{Name: "PageSize", Values: []string{"1.8x3mR", "custom"}},
		{Name: "num copies", Values: []string{"1", "2", "5", "10"}},
		{Name: "EFBrightness", Values: []string{"00.00", "0.24", "0.16", "0.08", "-0.08", "-0.16", "-0.24"}},
		{Name: "EFPrintCover", Values: []string{"False", "BeforeJob", "AfterJob", "BeforeAndAfter"}},
		{Name: "EFOutputBin", Values: []string{"Stacker", "Bypass"}},
	}
	got := GenerateWithStrategy(axes, StrategyPairwise, 100)
	if len(got) <= 7 {
		t.Fatalf("pairwise generated only %d cases; want a real pairwise sample larger than diagonal", len(got))
	}
	assertPairwiseCoverage(t, normalizeAxes(axes), got)
}

func TestPairwiseHonorsLimit(t *testing.T) {
	got := GenerateWithStrategy([]Axis{
		{Name: "A", Values: []string{"1", "2", "3"}},
		{Name: "B", Values: []string{"1", "2", "3"}},
		{Name: "C", Values: []string{"1", "2", "3"}},
	}, StrategyPairwise, 2)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
}

func assertPairwiseCoverage(t *testing.T, axes []Axis, got []Combination) {
	t.Helper()
	covered := map[string]struct{}{}
	for _, combo := range got {
		for i := 0; i < len(axes); i++ {
			for j := i + 1; j < len(axes); j++ {
				covered[pairKey(axes[i].Name, combo[axes[i].Name], axes[j].Name, combo[axes[j].Name])] = struct{}{}
			}
		}
	}
	for pair := range allPairs(axes) {
		if _, ok := covered[pair]; !ok {
			t.Fatalf("missing pair coverage for %q", pair)
		}
	}
}
