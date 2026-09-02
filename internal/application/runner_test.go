package application

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/fiery"
	"api-automation/internal/reportxlsx"
)

type fakeConnector struct {
	client AutomationClient
	err    error
}

func (connector fakeConnector) Connect(context.Context) (AutomationClient, ConnectionInfo, error) {
	return connector.client, ConnectionInfo{SessionLoginPath: "/live/api/v5/login"}, connector.err
}

type fakeAutomationClient struct {
	mu sync.Mutex

	getResponses []map[string]string
	getError     error
	importCalls  int
	panicImport  bool
	actions      []string
	updates      []map[string]string
	presets      []string
	deleted      []string
	cancelled    []string
	check        fiery.ConstraintCheck
	checkErr     error
}

func (client *fakeAutomationClient) ImportJobToQueue(ctx context.Context, file, queue string) (fiery.ImportResult, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	client.importCalls++
	if client.panicImport && client.importCalls == 1 {
		panic("fake import panic")
	}
	return fiery.ImportResult{FilePath: file, JobID: "J" + string(rune('0'+client.importCalls)), StatusCode: 200}, nil
}

func (client *fakeAutomationClient) GetJobAttributes(ctx context.Context, jobID string) (map[string]string, error) {
	client.mu.Lock()
	defer client.mu.Unlock()
	if client.getError != nil {
		return nil, client.getError
	}
	if len(client.getResponses) == 0 {
		return map[string]string{"id": jobID, "status": "done spooling", "state": "held"}, nil
	}
	response := CloneStringMap(client.getResponses[0])
	if len(client.getResponses) > 1 {
		client.getResponses = client.getResponses[1:]
	}
	if response["id"] == "" {
		response["id"] = jobID
	}
	return response, nil
}

func (client *fakeAutomationClient) ApplyServerPreset(_ context.Context, jobID, presetID string) error {
	client.mu.Lock()
	client.presets = append(client.presets, jobID+":"+presetID)
	client.mu.Unlock()
	return nil
}

func (client *fakeAutomationClient) CheckJobConstraints(context.Context, string, map[string]string) (fiery.ConstraintCheck, error) {
	return client.check, client.checkErr
}

func (client *fakeAutomationClient) UpdateJobAttributes(_ context.Context, _ string, attributes map[string]string) error {
	client.mu.Lock()
	client.updates = append(client.updates, CloneStringMap(attributes))
	client.mu.Unlock()
	return nil
}

func (client *fakeAutomationClient) DeleteJob(_ context.Context, jobID string) error {
	client.mu.Lock()
	client.deleted = append(client.deleted, jobID)
	client.mu.Unlock()
	return nil
}

func (client *fakeAutomationClient) CancelJob(_ context.Context, jobID string) error {
	client.mu.Lock()
	client.cancelled = append(client.cancelled, jobID)
	client.mu.Unlock()
	return nil
}

func (client *fakeAutomationClient) JobAction(_ context.Context, jobID, action string) error {
	client.mu.Lock()
	client.actions = append(client.actions, jobID+":"+action)
	client.mu.Unlock()
	return nil
}

func (client *fakeAutomationClient) GetRawJobResponses(context.Context, string) []fiery.JobRawResponse {
	return nil
}

type fakeRecorder struct {
	mu        sync.Mutex
	results   []reportxlsx.Result
	appendErr error
	closeErr  error
	closed    bool
}

func (recorder *fakeRecorder) Append(result reportxlsx.Result) error {
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	recorder.results = append(recorder.results, cloneReportResult(result))
	return recorder.appendErr
}

func (recorder *fakeRecorder) Close() error {
	recorder.mu.Lock()
	recorder.closed = true
	recorder.mu.Unlock()
	return recorder.closeErr
}

func TestRunnerCompletesHoldWithStrictSetGetAndTypedEvents(t *testing.T) {
	client := &fakeAutomationClient{getResponses: []map[string]string{
		{"status": "done spooling", "state": "held"},
		{"status": "done spooling", "state": "held", "EFColorMode": "CMYK", "job name": "Server Job"},
	}}
	recorder := &fakeRecorder{}
	runner := NewRunner(fakeConnector{client: client}, recorder)
	runner.NewID = func() string { return "operation-1" }
	request := RunRequest{
		Files:        []string{"test.pdf"},
		Combinations: []combinations.Combination{{"EFColorMode": "CMYK"}},
		Modes:        []RunMode{RunModes()[0]},
		Workers:      4,
		Capabilities: capabilities.Model{Options: []capabilities.Option{{ID: "EFColorMode", Value: "Grayscale"}}},
		ServerPreset: &ServerPresetSelection{ID: "P-1", Description: "Production (ID P-1)"},
	}

	events, terminal := collectOperation(runner.Start(context.Background(), request))
	if terminal.Status != RunStatusCompleted || terminal.Progress.Executed != 1 || terminal.Progress.Passed != 1 {
		t.Fatalf("terminal = %#v", terminal)
	}
	if !recorder.closed || len(recorder.results) != 1 {
		t.Fatalf("recorder closed=%t results=%#v", recorder.closed, recorder.results)
	}
	result := recorder.results[0]
	if result.Result != "PASS" || result.JobID != "J1" || result.JobName != "Server Job" || result.GetValues["EFColorMode"] != "CMYK" {
		t.Fatalf("result = %#v", result)
	}
	if len(client.updates) != 1 || client.updates[0]["EFColorMode"] != "CMYK" {
		t.Fatalf("updates = %#v", client.updates)
	}
	if len(client.presets) != 1 || client.presets[0] != "J1:P-1" || !strings.Contains(result.Detail, "server preset=Production (ID P-1)") {
		t.Fatalf("presets=%#v detail=%q", client.presets, result.Detail)
	}
	assertEventKinds(t, events, RunEventStarted, RunEventAttributeWrite, RunEventReadback, RunEventResult, RunEventTerminal)
	for _, event := range events {
		if event.OperationID != "operation-1" {
			t.Fatalf("event operation ID = %q", event.OperationID)
		}
		if event.Kind == RunEventStarted && (event.Started == nil || event.Started.Workers != 1 || event.Started.PlannedTests != 1) {
			t.Fatalf("started event = %#v", event.Started)
		}
	}
}

func TestRunnerRIPRequiresProcessedRasterEvidence(t *testing.T) {
	client := &fakeAutomationClient{getResponses: []map[string]string{
		{"status": "done spooling", "state": "held"},
		{"status": "done spooling", "state": "held"},
		{"status": "done ripping", "state": "processed", "has disk raster?": "yes"},
		{"status": "done ripping", "state": "processed", "has disk raster?": "yes", "EFResolution": "360x720dpi"},
	}}
	runner := NewRunner(fakeConnector{client: client}, &fakeRecorder{})
	_, terminal := collectOperation(runner.Start(context.Background(), RunRequest{
		Files: []string{"test.pdf"}, Combinations: []combinations.Combination{{"EFResolution": "360x720dpi"}},
		Modes: []RunMode{RunModes()[1]}, Workers: 1,
	}))
	if terminal.Status != RunStatusCompleted || terminal.Progress.Passed != 1 {
		t.Fatalf("terminal = %#v", terminal)
	}
	if len(client.actions) != 1 || client.actions[0] != "J1:rip" {
		t.Fatalf("actions = %#v", client.actions)
	}
}

func TestRunnerLifecycleFailureCannotBeOverriddenByMatchingAttributes(t *testing.T) {
	client := &fakeAutomationClient{getResponses: []map[string]string{
		{"status": "done spooling", "state": "held"},
		{"status": "done spooling", "state": "held"},
		{"status": "process error", "state": "process error", "EFResolution": "360x720dpi"},
	}}
	recorder := &fakeRecorder{}
	runner := NewRunner(fakeConnector{client: client}, recorder)
	_, terminal := collectOperation(runner.Start(context.Background(), RunRequest{
		Files: []string{"test.pdf"}, Combinations: []combinations.Combination{{"EFResolution": "360x720dpi"}},
		Modes: []RunMode{RunModes()[1]}, Workers: 1,
	}))
	if terminal.Status != RunStatusCompleted || terminal.Progress.Failed != 1 || terminal.Progress.Passed != 0 {
		t.Fatalf("terminal = %#v", terminal)
	}
	if got := recorder.results[0]; got.Result != "FAIL" || got.Lifecycle == "" || got.GetValues["EFResolution"] != "360x720dpi" {
		t.Fatalf("lifecycle failure result = %#v", got)
	}
}

func TestRunnerExpectedConstraintValidationUsesDisposableHeldJob(t *testing.T) {
	model := capabilities.Model{Options: []capabilities.Option{
		{ID: "EFResolution", Constraints: capabilities.Constraints{"360x720dpi": {"EFEdgeDropSize": {"0_1_2_2_2"}}}},
		{ID: "EFEdgeDropSize"},
	}}
	client := &fakeAutomationClient{
		getResponses: []map[string]string{{"status": "done spooling", "state": "held"}, {"status": "done spooling", "state": "held"}},
		check:        fiery.ConstraintCheck{Supported: true, Conflicts: map[string]string{"EFEdgeDropSize": "incompatible"}},
	}
	recorder := &fakeRecorder{}
	runner := NewRunner(fakeConnector{client: client}, recorder)
	_, terminal := collectOperation(runner.Start(context.Background(), RunRequest{
		Files: []string{"test.pdf"}, Combinations: []combinations.Combination{{"EFResolution": "360x720dpi", "EFEdgeDropSize": "0_1_2_2_2"}},
		Modes: []RunMode{RunModes()[5]}, Workers: 1, Capabilities: model,
		TestIntent: TestIntentConstraint, ConstraintMode: ConstraintValidationOnly,
	}))
	if terminal.Status != RunStatusCompleted || terminal.Progress.Passed != 1 {
		t.Fatalf("terminal = %#v results=%#v", terminal, recorder.results)
	}
	if len(client.deleted) != 1 || len(client.actions) != 0 || len(client.updates) != 0 {
		t.Fatalf("deleted=%#v actions=%#v updates=%#v", client.deleted, client.actions, client.updates)
	}
	if got := recorder.results[0]; got.Mode != "Hold" || got.Result != "PASS" {
		t.Fatalf("constraint result = %#v", got)
	}
}

func TestRunnerCancellationIsTerminalAndClosesRecorder(t *testing.T) {
	client := &fakeAutomationClient{getError: context.Canceled}
	recorder := &fakeRecorder{}
	runner := NewRunner(fakeConnector{client: client}, recorder)
	operation := runner.Start(context.Background(), RunRequest{
		Files: []string{"test.pdf"}, Combinations: []combinations.Combination{{}}, Modes: []RunMode{RunModes()[0]}, Workers: 1,
	})
	operation.Cancel()
	_, terminal := collectOperation(operation)
	if terminal.Status != RunStatusCancelled {
		t.Fatalf("terminal = %#v", terminal)
	}
	if !recorder.closed {
		t.Fatal("recorder was not closed after cancellation")
	}
}

func TestRunnerRecoversPerJobPanicAndContinues(t *testing.T) {
	client := &fakeAutomationClient{panicImport: true}
	recorder := &fakeRecorder{}
	runner := NewRunner(fakeConnector{client: client}, recorder)
	events, terminal := collectOperation(runner.Start(context.Background(), RunRequest{
		Files: []string{"panic.pdf", "ok.pdf"}, Combinations: []combinations.Combination{{}}, Modes: []RunMode{RunModes()[0]}, Workers: 1,
	}))
	if terminal.Status != RunStatusCompleted || terminal.Progress.Executed != 2 || terminal.Progress.Passed != 1 || terminal.Progress.Errors != 1 {
		t.Fatalf("terminal = %#v results=%#v", terminal, recorder.results)
	}
	assertEventKinds(t, events, RunEventPanic, RunEventResult, RunEventTerminal)
}

func TestRunnerReportsResultStorageFailureWithoutDroppingResult(t *testing.T) {
	recorder := &fakeRecorder{appendErr: errors.New("disk full")}
	runner := NewRunner(fakeConnector{client: &fakeAutomationClient{}}, recorder)
	events, terminal := collectOperation(runner.Start(context.Background(), RunRequest{
		Files: []string{"test.pdf"}, Combinations: []combinations.Combination{{}}, Modes: []RunMode{RunModes()[0]}, Workers: 1,
	}))
	if terminal.Status != RunStatusCompleted || terminal.StorageError != "disk full" || terminal.Progress.Executed != 1 {
		t.Fatalf("terminal = %#v", terminal)
	}
	foundStoredResultEvent := false
	for _, event := range events {
		if event.Kind == RunEventResult && event.Result != nil && event.Result.StorageError == "disk full" {
			foundStoredResultEvent = true
		}
	}
	if !foundStoredResultEvent {
		t.Fatal("result event with storage failure was dropped")
	}
}

func collectOperation(operation *Operation) ([]RunEvent, RunTerminalEvent) {
	events := make([]RunEvent, 0)
	for event := range operation.Events {
		events = append(events, event)
	}
	terminal := <-operation.Done
	return events, terminal
}

func assertEventKinds(t *testing.T, events []RunEvent, kinds ...RunEventKind) {
	t.Helper()
	seen := make(map[RunEventKind]bool)
	for _, event := range events {
		seen[event.Kind] = true
	}
	for _, kind := range kinds {
		if !seen[kind] {
			t.Fatalf("missing event kind %q in %#v", kind, events)
		}
	}
}
