package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestExecutableDebugDirectoryPolicy(t *testing.T) {
	got := executableDebugDirectory()
	if runtime.GOOS != "windows" {
		if got != "" {
			t.Fatalf("non-Windows debug directory = %q, want application data fallback", got)
		}
		return
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if got != filepath.Dir(executable) {
		t.Fatalf("debug directory = %q, want executable directory %q", got, filepath.Dir(executable))
	}
}
