package files

import (
	"os"
	"path/filepath"
	"testing"

	"api-automation/internal/model"
)

func TestSelectAllSingleAndRandom(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "a.json")
	second := filepath.Join(dir, "b.json")
	if err := os.WriteFile(first, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	all, err := Select(model.TestFileSelection{FolderPath: dir, Mode: model.FileSelectionAll})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 2 {
		t.Fatalf("all count = %d, want 2", len(all))
	}

	single, err := Select(model.TestFileSelection{FolderPath: dir, Mode: model.FileSelectionSingle, FilePath: first})
	if err != nil {
		t.Fatal(err)
	}
	if len(single) != 1 || filepath.Base(single[0]) != "a.json" {
		t.Fatalf("single = %#v", single)
	}

	random, err := Select(model.TestFileSelection{FolderPath: dir, Mode: model.FileSelectionRandom})
	if err != nil {
		t.Fatal(err)
	}
	if len(random) != 1 {
		t.Fatalf("random count = %d, want 1", len(random))
	}
}

func TestSelectSingleRejectsOutsideFolder(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Select(model.TestFileSelection{FolderPath: dir, Mode: model.FileSelectionSingle, FilePath: outside})
	if err == nil {
		t.Fatal("expected outside file to be rejected")
	}
}
