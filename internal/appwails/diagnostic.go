package appwails

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

func newDiagnosticLog(dataDirectory string) *diagnosticLog {
	if dataDirectory == "" {
		return &diagnosticLog{}
	}
	dir := filepath.Join(dataDirectory, "logs")
	if os.MkdirAll(dir, 0o700) != nil {
		return &diagnosticLog{}
	}
	prefix := fmt.Sprintf("api-automation-wails-%s-%d-", time.Now().Format("20060102-150405.000000000"), os.Getpid())
	file, err := os.CreateTemp(dir, prefix+"*.log")
	if err != nil {
		return &diagnosticLog{}
	}
	return &diagnosticLog{file: file, path: file.Name()}
}

func (log *diagnosticLog) Path() string {
	if log == nil {
		return ""
	}
	return log.path
}

func (log *diagnosticLog) Printf(format string, args ...any) {
	if log == nil || log.file == nil {
		return
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return
	}
	_, _ = fmt.Fprintf(log.file, "%s  %s\n", time.Now().Format(time.RFC3339), fmt.Sprintf(format, args...))
}

func (log *diagnosticLog) Close() {
	if log == nil || log.file == nil {
		return
	}
	log.mu.Lock()
	defer log.mu.Unlock()
	if log.closed {
		return
	}
	_, _ = fmt.Fprintf(log.file, "%s  Application exiting\n", time.Now().Format(time.RFC3339))
	_ = log.file.Close()
	log.closed = true
}
