//go:build windows

package appgio

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type diagnosticLog struct {
	mu     sync.Mutex
	file   *os.File
	path   string
	closed bool
}

func newDiagnosticLog() *diagnosticLog {
	dir := diagnosticsDirectory()
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "api-automation-"+time.Now().Format("20060102-150405")+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		path = filepath.Join(os.TempDir(), "api-automation-"+time.Now().Format("20060102-150405")+".log")
		file, _ = os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	}
	return &diagnosticLog{file: file, path: path}
}

func diagnosticsDirectory() string {
	exe, err := os.Executable()
	if err != nil {
		return "logs"
	}
	return filepath.Join(filepath.Dir(exe), "logs")
}

func captureDirectory() string {
	exe, err := os.Executable()
	if err != nil {
		return "captures"
	}
	return filepath.Join(filepath.Dir(exe), "captures")
}

func writeCrashReport(message string, stack []byte) error {
	dir := diagnosticsDirectory()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := filepath.Join(dir, "crash.log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer file.Close()
	_, _ = fmt.Fprintf(file, "%s  %s\n", time.Now().Format(time.RFC3339), message)
	if len(stack) != 0 {
		_, _ = fmt.Fprintf(file, "%s\n", stack)
	}
	_, _ = fmt.Fprintln(file, "---")
	return file.Sync()
}

func (l *diagnosticLog) printf(format string, args ...any) {
	if l == nil || l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	_, _ = fmt.Fprintf(l.file, "%s  %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
	_ = l.file.Sync()
}

func (l *diagnosticLog) close() {
	if l == nil || l.file == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.closed {
		return
	}
	_, _ = fmt.Fprintf(l.file, "%s  Application exiting\n", time.Now().Format(time.RFC3339))
	_ = l.file.Sync()
	_ = l.file.Close()
	l.closed = true
}
