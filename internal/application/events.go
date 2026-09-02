package application

import (
	"context"
	"sync"
	"time"

	"api-automation/internal/reportxlsx"
)

type RunEventKind string

const (
	RunEventStarted        RunEventKind = "started"
	RunEventLog            RunEventKind = "log"
	RunEventProgress       RunEventKind = "progress"
	RunEventResult         RunEventKind = "result"
	RunEventAttributeWrite RunEventKind = "attribute_write"
	RunEventReadback       RunEventKind = "readback"
	RunEventRawComparison  RunEventKind = "raw_comparison"
	RunEventPanic          RunEventKind = "panic"
	RunEventTerminal       RunEventKind = "terminal"
)

type RunStatus string

const (
	RunStatusCompleted RunStatus = "Completed"
	RunStatusCancelled RunStatus = "Cancelled"
	RunStatusFailed    RunStatus = "Failed"
)

// RunEvent is a discriminated, JSON-friendly event DTO. Exactly one typed
// payload is populated according to Kind.
type RunEvent struct {
	Kind           RunEventKind         `json:"kind"`
	OperationID    string               `json:"operationId"`
	At             time.Time            `json:"at"`
	Started        *RunStartedEvent     `json:"started,omitempty"`
	Log            *RunLogEvent         `json:"log,omitempty"`
	Progress       *RunProgressEvent    `json:"progress,omitempty"`
	Result         *RunResultEvent      `json:"result,omitempty"`
	AttributeWrite *AttributeWriteEvent `json:"attributeWrite,omitempty"`
	Readback       *ReadbackEvent       `json:"readback,omitempty"`
	RawComparison  *RawComparisonEvent  `json:"rawComparison,omitempty"`
	Panic          *RunPanicEvent       `json:"panic,omitempty"`
	Terminal       *RunTerminalEvent    `json:"terminal,omitempty"`
}

type RunStartedEvent struct {
	PlannedTests     int64  `json:"plannedTests"`
	Workers          int    `json:"workers"`
	SessionLoginPath string `json:"sessionLoginPath,omitempty"`
}

type RunLogEvent struct {
	Message string `json:"message"`
}

type RunProgressEvent struct {
	Planned  int64 `json:"planned"`
	Executed int64 `json:"executed"`
	Passed   int64 `json:"passed"`
	Failed   int64 `json:"failed"`
	Errors   int64 `json:"errors"`
}

type RunResultEvent struct {
	Result       reportxlsx.Result `json:"result"`
	StorageError string            `json:"storageError,omitempty"`
}

type AttributeWriteEvent struct {
	JobID      string            `json:"jobId"`
	Attributes map[string]string `json:"attributes"`
}

type ReadbackEvent struct {
	JobID    string            `json:"jobId"`
	Expected map[string]string `json:"expected"`
	Actual   map[string]string `json:"actual"`
	Matched  bool              `json:"matched"`
	Error    string            `json:"error,omitempty"`
}

type RawComparisonEvent struct {
	JobID     string                  `json:"jobId"`
	Responses []RawComparisonResponse `json:"responses"`
}

type RawComparisonResponse struct {
	Method        string `json:"method"`
	Endpoint      string `json:"endpoint"`
	ResponseProto string `json:"responseProto,omitempty"`
	StatusCode    int    `json:"statusCode"`
	Body          string `json:"body"`
}

type RunPanicEvent struct {
	File  string `json:"file"`
	Mode  string `json:"mode"`
	Value string `json:"value"`
	Stack string `json:"stack"`
}

type RunTerminalEvent struct {
	Status       RunStatus        `json:"status"`
	Error        string           `json:"error,omitempty"`
	StorageError string           `json:"storageError,omitempty"`
	Progress     RunProgressEvent `json:"progress"`
}

// Operation represents one independently cancellable run. Consumers must drain
// Events until it closes; the internal unbounded critical-event queue ensures
// Fiery workers never wait on frontend delivery and never drops results or the
// terminal event.
type Operation struct {
	ID     string
	Events <-chan RunEvent
	Done   <-chan RunTerminalEvent
	cancel context.CancelFunc
}

func (operation *Operation) Cancel() {
	if operation != nil && operation.cancel != nil {
		operation.cancel()
	}
}

type eventQueue struct {
	operationID string
	now         func() time.Time
	output      chan RunEvent
	wake        chan struct{}
	mu          sync.Mutex
	items       []RunEvent
	closed      bool
}

func newEventQueue(operationID string, now func() time.Time) *eventQueue {
	queue := &eventQueue{
		operationID: operationID,
		now:         now,
		output:      make(chan RunEvent),
		wake:        make(chan struct{}, 1),
	}
	go queue.dispatch()
	return queue
}

func (queue *eventQueue) publish(event RunEvent) {
	queue.mu.Lock()
	if queue.closed {
		queue.mu.Unlock()
		return
	}
	event.OperationID = queue.operationID
	if event.At.IsZero() {
		event.At = queue.now()
	}
	// Progress is a snapshot and may be coalesced. Critical result/terminal
	// events are always appended and never replaced.
	if event.Kind == RunEventProgress && len(queue.items) > 0 && queue.items[len(queue.items)-1].Kind == RunEventProgress {
		queue.items[len(queue.items)-1] = event
	} else {
		queue.items = append(queue.items, event)
	}
	queue.mu.Unlock()
	queue.notify()
}

func (queue *eventQueue) close() {
	queue.mu.Lock()
	queue.closed = true
	queue.mu.Unlock()
	queue.notify()
}

func (queue *eventQueue) notify() {
	select {
	case queue.wake <- struct{}{}:
	default:
	}
}

func (queue *eventQueue) dispatch() {
	defer close(queue.output)
	for {
		queue.mu.Lock()
		if len(queue.items) > 0 {
			event := queue.items[0]
			queue.items[0] = RunEvent{}
			queue.items = queue.items[1:]
			queue.mu.Unlock()
			queue.output <- event
			continue
		}
		closed := queue.closed
		queue.mu.Unlock()
		if closed {
			return
		}
		<-queue.wake
	}
}
