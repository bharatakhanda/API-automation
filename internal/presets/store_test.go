package presets

import (
	"path/filepath"
	"reflect"
	"testing"
)

func TestStoreSaveReplaceListAndDelete(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "nested", "presets.json"))
	if err != nil {
		t.Fatal(err)
	}
	first := Preset{Name: "Production", SelectedValues: map[string][]string{"EFColorMode": {"CMYK"}}, NumericInputs: map[string]string{"Scaling": "100"}, Strategy: "selected", MaxCases: "50", ParallelJobs: "4", RunModes: []string{"Process and Hold"}}
	if err := store.Save(first); err != nil {
		t.Fatal(err)
	}
	first.SelectedValues["EFColorMode"][0] = "mutated"
	if err := store.Save(Preset{Name: "production", Strategy: "pairwise", MaxCases: "10", ParallelJobs: "2"}); err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Preset{Name: "Archive", Strategy: "selected"}); err != nil {
		t.Fatal(err)
	}
	presets, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if names := []string{presets[0].Name, presets[1].Name}; !reflect.DeepEqual(names, []string{"Archive", "production"}) {
		t.Fatalf("names = %#v", names)
	}
	if presets[1].Strategy != "pairwise" {
		t.Fatalf("replacement = %#v", presets[1])
	}
	if err := store.Delete("PRODUCTION"); err != nil {
		t.Fatal(err)
	}
	presets, err = store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(presets) != 1 || presets[0].Name != "Archive" {
		t.Fatalf("after delete = %#v", presets)
	}
}

func TestStorePersistsSafeServerPresetSelection(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "presets.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Save(Preset{Name: "Production", ServerPresetID: "SERVER-PRESET-1", Strategy: "pairwise", ValueSource: "advertised", TestIntent: "constraint", ConstraintMode: "validation"}); err != nil {
		t.Fatal(err)
	}
	got, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].ServerPresetID != "SERVER-PRESET-1" || got[0].ValueSource != "advertised" || got[0].TestIntent != "constraint" || got[0].ConstraintMode != "validation" {
		t.Fatalf("presets = %#v", got)
	}
}

func TestStoreRejectsInvalidNamesAndCorruptFiles(t *testing.T) {
	store, err := New(filepath.Join(t.TempDir(), "presets.json"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"", "bad\nname"} {
		if err := store.Save(Preset{Name: name}); err == nil {
			t.Errorf("Save(%q) unexpectedly succeeded", name)
		}
	}
}
