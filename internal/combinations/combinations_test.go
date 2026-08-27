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
