package files

import (
	"os"
	"path/filepath"
	"testing"

	"api-automation/internal/model"
)

func TestSelectSingleRejectsUnsupportedFile(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "api-automation.exe")
	if err := os.WriteFile(exe, []byte("exe"), 0o600); err != nil {
		t.Fatal(err)
	}

	_, err := Select(model.TestFileSelection{FolderPath: dir, Mode: model.FileSelectionSingle, FilePath: exe})
	if err == nil {
		t.Fatal("expected unsupported exe to be rejected")
	}
}
