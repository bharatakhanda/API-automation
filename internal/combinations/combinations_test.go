package combinations

import "testing"

func TestGenerateSingleConfigurationUsesFirstNormalizedValue(t *testing.T) {
	got := GenerateWithStrategy([]Axis{
		{Name: "Color", Values: []string{"Grayscale", "CMYK"}},
		{Name: "Duplex", Values: []string{"On", "Off"}},
	}, StrategySingle, 100)
	if len(got) != 1 || got[0]["Color"] != "CMYK" || got[0]["Duplex"] != "Off" {
		t.Fatalf("single configuration = %#v", got)
	}
	if got := GenerateWithStrategy([]Axis{{Name: "Color", Values: []string{"CMYK"}}}, StrategySingle, 0); got != nil {
		t.Fatalf("zero-limit single configuration = %#v", got)
	}
}

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

func TestPairwiseDoesNotMaterializeHugeCartesianProduct(t *testing.T) {
	axes := make([]Axis, 12)
	for axisIndex := range axes {
		axes[axisIndex].Name = string(rune('A' + axisIndex))
		for value := 0; value < 12; value++ {
			axes[axisIndex].Values = append(axes[axisIndex].Values, string(rune('a'+value)))
		}
	}
	got := GenerateWithStrategy(axes, StrategyPairwise, 100)
	if len(got) != 100 {
		t.Fatalf("len = %d, want bounded result of 100", len(got))
	}
	for _, combo := range got {
		if len(combo) != len(axes) {
			t.Fatalf("incomplete combination: %#v", combo)
		}
	}
}

func TestRandomStrategySamplesHugeProductWithoutDuplicates(t *testing.T) {
	axes := make([]Axis, 20)
	for axisIndex := range axes {
		axes[axisIndex] = Axis{Name: string(rune('A' + axisIndex)), Values: []string{"0", "1", "2", "3", "4", "5", "6", "7", "8", "9"}}
	}
	got := GenerateWithStrategy(axes, StrategyRandom, 100)
	if len(got) != 100 {
		t.Fatalf("len = %d, want 100", len(got))
	}
	seen := map[string]struct{}{}
	for _, combo := range got {
		key := ""
		for _, axis := range axes {
			key += combo[axis.Name]
		}
		if _, duplicate := seen[key]; duplicate {
			t.Fatalf("duplicate random combination %q", key)
		}
		seen[key] = struct{}{}
	}
}

func allPairs(axes []Axis) map[string]struct{} {
	pairs := map[string]struct{}{}
	for i := 0; i < len(axes); i++ {
		for j := i + 1; j < len(axes); j++ {
			for _, left := range axes[i].Values {
				for _, right := range axes[j].Values {
					pairs[pairKey(axes[i].Name, left, axes[j].Name, right)] = struct{}{}
				}
			}
		}
	}
	return pairs
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
