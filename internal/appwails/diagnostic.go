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
	path := filepath.Join(dir, "api-automation-wails-"+time.Now().Format("20060102-150405")+".log")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return &diagnosticLog{}
	}
	return &diagnosticLog{file: file, path: path}
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
