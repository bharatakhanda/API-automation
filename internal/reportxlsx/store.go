package reportxlsx

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Result is one completed automation test. SetValues and GetValues use Fiery
// API attribute IDs as keys so workbook columns can be generated dynamically.
type Result struct {
	JobID      string            `json:"jobId"`
	JobName    string            `json:"jobName"`
	Result     string            `json:"result"`
	Mode       string            `json:"mode,omitempty"`
	DurationMS int64             `json:"durationMs"`
	JobStatus  string            `json:"jobStatus,omitempty"`
	JobState   string            `json:"jobState,omitempty"`
	JobError   string            `json:"jobError,omitempty"`
	LastEvent  string            `json:"lastEvent,omitempty"`
	Lifecycle  string            `json:"lifecycle,omitempty"`
	Detail     string            `json:"detail,omitempty"`
	SetValues  map[string]string `json:"setValues,omitempty"`
	GetValues  map[string]string `json:"getValues,omitempty"`
}

// Summary captures general, non-secret information about one automation run.
type Summary struct {
	StartedAt         time.Time
	CompletedAt       time.Time
	Status            string
	ServerIP          string
	ServerName        string
	SerialNumber      string
	ServerVersion     string
	SessionLoginPath  string
	QueuesDiscovered  int
	OptionsDiscovered int
	TestFileCount     int
	CombinationCount  int
	ConstraintSkipped int
	PlannedTests      int64
	Workers           int
	Strategy          string
	ServerPreset      string
	RunModes          []string
}

// ResultStore appends complete result records to disk so long runs do not need
// to retain every dynamic attribute map in memory.
type ResultStore struct {
	mu     sync.Mutex
	file   *os.File
	writer *bufio.Writer
	path   string
	closed bool
}

func NewResultStore(dir string, started time.Time) (*ResultStore, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, errors.New("result-store directory is required")
	}
	if started.IsZero() {
		started = time.Now()
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("create result-store directory: %w", err)
	}
	name := "automation-run-results-" + started.Format("20060102-150405.000000000") + ".jsonl"
	path := filepath.Join(dir, name)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return nil, fmt.Errorf("create result store: %w", err)
	}
	return &ResultStore{file: file, writer: bufio.NewWriterSize(file, 64*1024), path: path}, nil
}

func (s *ResultStore) Path() string {
	if s == nil {
		return ""
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path
}

func (s *ResultStore) Append(result Result) error {
	if s == nil {
		return errors.New("result store is unavailable")
	}
	body, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || s.writer == nil {
		return errors.New("result store is closed")
	}
	if _, err := s.writer.Write(body); err != nil {
		return fmt.Errorf("write result: %w", err)
	}
	if err := s.writer.WriteByte('\n'); err != nil {
		return fmt.Errorf("write result separator: %w", err)
	}
	return nil
}

func (s *ResultStore) Close() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	var errs []error
	if s.writer != nil {
		if err := s.writer.Flush(); err != nil {
			errs = append(errs, fmt.Errorf("flush result store: %w", err))
		}
	}
	if s.file != nil {
		// Closing the file commits buffered writes without a potentially lengthy
		// full-file fsync. Long automation runs can produce large stores, and a
		// blocking Sync here previously delayed both finalization and GUI exit.
		if err := s.file.Close(); err != nil {
			errs = append(errs, fmt.Errorf("close result store: %w", err))
		}
	}
	return errors.Join(errs...)
}

func forEachResult(path string, visit func(Result) error) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open stored results: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(bufio.NewReaderSize(file, 64*1024))
	for {
		var result Result
		if err := decoder.Decode(&result); err != nil {
			if errors.Is(err, io.EOF) {
				return nil
			}
			return fmt.Errorf("decode stored result: %w", err)
		}
		if err := visit(result); err != nil {
			return err
		}
	}
}
