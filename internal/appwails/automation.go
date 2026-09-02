package appwails

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	core "api-automation/internal/application"
	"api-automation/internal/fiery"
	"api-automation/internal/model"
	"api-automation/internal/reportxlsx"
)

const (
	automationEventName = "automation:event"
	maxFrontendResults  = 500
	maxFrontendLogs     = 1000
)

type WorkspaceMetadata struct {
	RunModes           []RunModeView `json:"runModes"`
	DefaultMaxCases    int           `json:"defaultMaxCases"`
	MaximumMaxCases    int           `json:"maximumMaxCases"`
	DefaultWorkers     int           `json:"defaultWorkers"`
	MaximumWorkers     int           `json:"maximumWorkers"`
	OverviewIntervalMS int64         `json:"overviewIntervalMs"`
}

type RunModeView struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

type AutomationInput struct {
	Files          FileSelection `json:"files"`
	Plan           PlanningInput `json:"plan"`
	Workers        int           `json:"workers"`
	RunModeIDs     []string      `json:"runModeIds"`
	ServerPresetID string        `json:"serverPresetId,omitempty"`
	ConstraintMode string        `json:"constraintMode"`
}

type AutomationState struct {
	OperationID      string                `json:"operationId,omitempty"`
	Status           string                `json:"status"`
	Error            string                `json:"error,omitempty"`
	StorageError     string                `json:"storageError,omitempty"`
	Progress         core.RunProgressEvent `json:"progress"`
	Results          []reportxlsx.Result   `json:"results,omitempty"`
	Logs             []string              `json:"logs,omitempty"`
	ResultsTruncated bool                  `json:"resultsTruncated"`
	StartedAt        time.Time             `json:"startedAt,omitempty"`
	CompletedAt      time.Time             `json:"completedAt,omitempty"`
	Strategy         string                `json:"strategy,omitempty"`
	ServerPreset     string                `json:"serverPreset,omitempty"`
	RunModes         []string              `json:"runModes,omitempty"`
	ResultFileReady  bool                  `json:"resultFileReady"`
}

type ExportResult struct {
	Path   string `json:"path"`
	Total  int    `json:"total"`
	Passed int    `json:"passed"`
	Failed int    `json:"failed"`
	Errors int    `json:"errors"`
}

type activeRun struct {
	operation  *core.Operation
	finished   chan struct{}
	state      AutomationState
	summary    reportxlsx.Summary
	resultPath string
	labels     map[string]string
}

func (service *Service) Metadata() WorkspaceMetadata {
	modes := core.RunModes()
	result := WorkspaceMetadata{
		RunModes: make([]RunModeView, len(modes)), DefaultMaxCases: core.DefaultCaseLimit, MaximumMaxCases: core.MaximumCaseLimit,
		DefaultWorkers: core.DefaultWorkerCount, MaximumWorkers: core.MaximumWorkerCount,
		OverviewIntervalMS: core.DefaultOverviewMonitorPolicy().StatusInterval.Milliseconds(),
	}
	for index, mode := range modes {
		result.RunModes[index] = RunModeView{ID: mode.ID, Label: mode.Label}
	}
	return result
}

func (service *Service) StartAutomation(input AutomationInput) (AutomationState, error) {
	service.operationMu.Lock()
	defer service.operationMu.Unlock()
	service.runMu.Lock()
	alreadyRunning := service.run != nil && service.run.state.Status == "Running"
	service.runMu.Unlock()
	if alreadyRunning {
		return AutomationState{}, errors.New("automation is already running")
	}
	server, ok := service.connection.Active()
	if !ok {
		return AutomationState{}, errors.New("test and apply a server connection first")
	}
	capability, err := service.capabilityModel()
	if err != nil {
		return AutomationState{}, err
	}
	selectedFiles, err := service.ResolveTestFiles(input.Files)
	if err != nil {
		return AutomationState{}, err
	}
	plan, err := core.BuildPlan(planRequest(capability, input.Plan))
	if err != nil {
		return AutomationState{}, err
	}
	modes, err := runModesByID(input.RunModeIDs)
	if err != nil {
		return AutomationState{}, err
	}
	if len(modes) == 0 {
		return AutomationState{}, errors.New("select at least one run mode")
	}
	if input.Plan.TestIntent == string(core.TestIntentConstraint) {
		modes = []core.RunMode{core.RunModes()[0]}
	}
	var presetSelection *core.ServerPresetSelection
	presetDescription := "None"
	if input.Plan.ValueSource != string(core.ValueSourceBaseline) && !(input.Plan.TestIntent == string(core.TestIntentConstraint) && input.ConstraintMode == string(core.ConstraintValidationOnly)) {
		for _, preset := range capability.ServerPresets {
			if preset.ID == strings.TrimSpace(input.ServerPresetID) {
				presetDescription = preset.Name + " (" + preset.ID + ")"
				presetSelection = &core.ServerPresetSelection{ID: preset.ID, Description: presetDescription}
				break
			}
		}
		if strings.TrimSpace(input.ServerPresetID) != "" && presetSelection == nil {
			return AutomationState{}, errors.New("selected Fiery server preset is no longer available")
		}
	}
	started := time.Now()
	resultDir, err := service.resultDirectory()
	if err != nil {
		return AutomationState{}, err
	}
	store, err := reportxlsx.NewResultStore(resultDir, started)
	if err != nil {
		return AutomationState{}, err
	}
	runner := core.NewRunner(fieryAutomationConnector{server: server}, store)
	request := core.RunRequest{
		Files: selectedFiles.Files, Combinations: plan.Combinations, Modes: modes, Workers: input.Workers, Capabilities: capability,
		ServerPreset: presetSelection, TestIntent: core.TestIntent(input.Plan.TestIntent), ConstraintMode: core.ConstraintMode(input.ConstraintMode),
	}
	planned := core.PlannedTestCount(len(request.Files), len(request.Combinations), len(request.Modes))
	workers := core.EffectiveWorkerCount(input.Workers, planned)
	summary := reportxlsx.Summary{
		StartedAt: started, Status: "Running", ServerIP: server.IPAddress, ServerName: capability.ServerName, SerialNumber: capability.SerialNumber,
		ServerVersion: capability.Version, QueuesDiscovered: len(capability.Queues), OptionsDiscovered: len(capability.Options), TestFileCount: len(request.Files),
		CombinationCount: len(request.Combinations), ConstraintSkipped: plan.ConstraintSkipped, PlannedTests: planned, Workers: workers,
		Strategy: fmt.Sprintf("%s · %s · %s", input.Plan.Strategy, input.Plan.ValueSource, input.Plan.TestIntent), ServerPreset: presetDescription, RunModes: core.RunModeLabels(modes),
	}
	operation := runner.Start(service.rootContext, request)
	current := &activeRun{
		operation: operation, finished: make(chan struct{}), summary: summary, resultPath: store.Path(), labels: capabilityLabels(capability),
		state: AutomationState{OperationID: operation.ID, Status: "Running", StartedAt: started, Strategy: summary.Strategy, ServerPreset: presetDescription, RunModes: append([]string(nil), summary.RunModes...)},
	}
	service.runMu.Lock()
	service.run = current
	state := cloneAutomationState(current.state)
	service.runMu.Unlock()
	service.diagnostic.Printf("AUTOMATION_START operation=%s server=%s files=%d combinations=%d modes=%d planned=%d workers=%d", operation.ID, server.IPAddress, len(request.Files), len(request.Combinations), len(request.Modes), planned, workers)
	go service.consumeRun(current)
	return state, nil
}

func (service *Service) automationRunning() bool {
	service.runMu.Lock()
	defer service.runMu.Unlock()
	return service.run != nil && service.run.state.Status == "Running"
}

func (service *Service) CancelAutomation() AutomationState {
	service.runMu.Lock()
	if service.run != nil && service.run.operation != nil && service.run.state.Status == "Running" {
		service.run.operation.Cancel()
		service.run.state.Logs = appendBounded(service.run.state.Logs, "Cancellation requested", maxFrontendLogs)
	}
	state := AutomationState{Status: "Not started"}
	if service.run != nil {
		state = cloneAutomationState(service.run.state)
	}
	service.runMu.Unlock()
	return state
}

func (service *Service) AutomationState() AutomationState {
	service.runMu.Lock()
	defer service.runMu.Unlock()
	if service.run == nil {
		return AutomationState{Status: "Not started"}
	}
	return cloneAutomationState(service.run.state)
}

func (service *Service) ExportResults() (ExportResult, error) {
	service.runMu.Lock()
	if service.run == nil || service.run.resultPath == "" || service.run.state.Status == "Running" {
		service.runMu.Unlock()
		return ExportResult{}, errors.New("completed stored automation results are unavailable")
	}
	summary := service.run.summary
	resultPath := service.run.resultPath
	labels := cloneStrings(service.run.labels)
	service.runMu.Unlock()
	dialogs, err := service.dialogPort()
	if err != nil {
		return ExportResult{}, err
	}
	path, err := dialogs.SelectExcelPath()
	if err != nil || strings.TrimSpace(path) == "" {
		return ExportResult{}, err
	}
	stats, err := reportxlsx.Export(path, reportxlsx.Report{Summary: summary, ResultsPath: resultPath, AttributeLabels: labels})
	if err != nil {
		return ExportResult{}, err
	}
	if !strings.EqualFold(filepath.Ext(path), ".xlsx") {
		path += ".xlsx"
	}
	service.diagnostic.Printf("EXCEL_EXPORT path=%s total=%d passed=%d failed=%d errors=%d", path, stats.Total, stats.Passed, stats.Failed, stats.Errors)
	return ExportResult{Path: path, Total: stats.Total, Passed: stats.Passed, Failed: stats.Failed, Errors: stats.Errors}, nil
}

func (service *Service) consumeRun(current *activeRun) {
	defer close(current.finished)
	for event := range current.operation.Events {
		service.runMu.Lock()
		if service.run != current {
			service.runMu.Unlock()
			continue
		}
		applyRunEvent(&current.state, event)
		emitter := service.eventEmitter
		service.runMu.Unlock()
		service.diagnostic.Printf("RUN_EVENT operation=%s kind=%s", event.OperationID, event.Kind)
		if emitter != nil {
			emitter(automationEventName, event)
		}
	}
	terminal := <-current.operation.Done
	service.runMu.Lock()
	if service.run == current {
		current.state.Status = string(terminal.Status)
		current.state.Error = terminal.Error
		current.state.StorageError = terminal.StorageError
		current.state.Progress = terminal.Progress
		current.state.CompletedAt = time.Now()
		current.state.ResultFileReady = current.resultPath != ""
		current.summary.Status = string(terminal.Status)
		current.summary.CompletedAt = current.state.CompletedAt
		if terminal.Status == core.RunStatusCompleted && terminal.Progress.Failed+terminal.Progress.Errors > 0 {
			current.summary.Status = "Completed with failures"
		}
	}
	service.runMu.Unlock()
	service.diagnostic.Printf("AUTOMATION_TERMINAL operation=%s status=%s executed=%d passed=%d failed=%d errors=%d", current.state.OperationID, terminal.Status, terminal.Progress.Executed, terminal.Progress.Passed, terminal.Progress.Failed, terminal.Progress.Errors)
}

func applyRunEvent(state *AutomationState, event core.RunEvent) {
	switch event.Kind {
	case core.RunEventStarted:
		if event.Started != nil {
			state.Progress.Planned = event.Started.PlannedTests
		}
	case core.RunEventProgress:
		if event.Progress != nil {
			state.Progress = *event.Progress
		}
	case core.RunEventResult:
		if event.Result != nil {
			if len(state.Results) >= maxFrontendResults {
				copy(state.Results, state.Results[1:])
				state.Results[len(state.Results)-1] = event.Result.Result
				state.ResultsTruncated = true
			} else {
				state.Results = append(state.Results, event.Result.Result)
			}
			if event.Result.StorageError != "" {
				state.StorageError = event.Result.StorageError
			}
		}
	case core.RunEventLog:
		if event.Log != nil {
			state.Logs = appendBounded(state.Logs, event.Log.Message, maxFrontendLogs)
		}
	case core.RunEventAttributeWrite:
		if event.AttributeWrite != nil {
			state.Logs = appendBounded(state.Logs, fmt.Sprintf("ATTRIBUTE_WRITE job=%s values=%d", event.AttributeWrite.JobID, len(event.AttributeWrite.Attributes)), maxFrontendLogs)
		}
	case core.RunEventReadback:
		if event.Readback != nil {
			state.Logs = appendBounded(state.Logs, fmt.Sprintf("READBACK job=%s matched=%t error=%s", event.Readback.JobID, event.Readback.Matched, event.Readback.Error), maxFrontendLogs)
		}
	case core.RunEventRawComparison:
		if event.RawComparison != nil {
			for _, response := range event.RawComparison.Responses {
				state.Logs = appendBounded(state.Logs, fmt.Sprintf("RAW_COMPARISON job=%s method=%s endpoint=%s status=%d", event.RawComparison.JobID, response.Method, response.Endpoint, response.StatusCode), maxFrontendLogs)
			}
		}
	case core.RunEventPanic:
		if event.Panic != nil {
			state.Logs = appendBounded(state.Logs, "Internal worker panic; inspect crash diagnostics", maxFrontendLogs)
		}
	case core.RunEventTerminal:
		if event.Terminal != nil {
			state.Status = string(event.Terminal.Status)
			state.Error = event.Terminal.Error
			state.StorageError = event.Terminal.StorageError
			state.Progress = event.Terminal.Progress
		}
	}
}

func runModesByID(ids []string) ([]core.RunMode, error) {
	known := make(map[string]core.RunMode)
	for _, mode := range core.RunModes() {
		known[mode.ID] = mode
	}
	result := make([]core.RunMode, 0, len(ids))
	seen := make(map[string]struct{})
	for _, id := range ids {
		id = strings.TrimSpace(id)
		mode, ok := known[id]
		if !ok {
			return nil, fmt.Errorf("unknown run mode %q", id)
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, mode)
	}
	return result, nil
}

func capabilityLabels(capability capabilitiesModel) map[string]string {
	labels := make(map[string]string, len(capability.Options))
	for _, option := range capability.Options {
		labels[option.ID] = option.Label
	}
	return labels
}

func (service *Service) resultDirectory() (string, error) {
	if service.dataDirectory == "" {
		return "", errors.New("application result directory is unavailable")
	}
	return filepath.Join(service.dataDirectory, "results"), nil
}

func cloneAutomationState(source AutomationState) AutomationState {
	clone := source
	clone.Results = make([]reportxlsx.Result, len(source.Results))
	for index, result := range source.Results {
		clone.Results[index] = result
		clone.Results[index].SetValues = cloneStrings(result.SetValues)
		clone.Results[index].GetValues = cloneStrings(result.GetValues)
	}
	clone.Logs = append([]string(nil), source.Logs...)
	clone.RunModes = append([]string(nil), source.RunModes...)
	return clone
}

func appendBounded(values []string, value string, limit int) []string {
	if strings.TrimSpace(value) == "" {
		return values
	}
	if len(values) >= limit {
		copy(values, values[len(values)-limit+1:])
		values = values[:limit-1]
	}
	return append(values, value)
}

type fieryAutomationConnector struct {
	server model.ServerConnection
}

func (connector fieryAutomationConnector) Connect(ctx context.Context) (core.AutomationClient, core.ConnectionInfo, error) {
	client, err := fiery.New(fiery.Config{ServerIP: connector.server.IPAddress, SecretKey: connector.server.SecretKey, Password: connector.server.Password, InsecureTLS: true})
	if err != nil {
		return nil, core.ConnectionInfo{}, redactError(fmt.Errorf("server configuration invalid: %w", err), connector.server.SecretKey, connector.server.Password)
	}
	session, err := client.Login(ctx)
	if err != nil {
		return nil, core.ConnectionInfo{}, redactError(fmt.Errorf("login failed: %w", err), connector.server.SecretKey, connector.server.Password)
	}
	return fieryAutomationClient{client: client, session: session}, core.ConnectionInfo{SessionLoginPath: session.LoginPath}, nil
}

type fieryAutomationClient struct {
	client  *fiery.Client
	session fiery.Session
}

func (client fieryAutomationClient) ImportJobToQueue(ctx context.Context, file, queue string) (fiery.ImportResult, error) {
	return client.client.ImportJobToQueue(ctx, client.session, file, queue)
}
func (client fieryAutomationClient) GetJobAttributes(ctx context.Context, jobID string) (map[string]string, error) {
	return client.client.GetJobAttributes(ctx, client.session, jobID)
}
func (client fieryAutomationClient) ApplyServerPreset(ctx context.Context, jobID, presetID string) error {
	return client.client.ApplyServerPreset(ctx, client.session, jobID, presetID)
}
func (client fieryAutomationClient) CheckJobConstraints(ctx context.Context, jobID string, attributes map[string]string) (fiery.ConstraintCheck, error) {
	return client.client.CheckJobConstraints(ctx, client.session, jobID, attributes)
}
func (client fieryAutomationClient) UpdateJobAttributes(ctx context.Context, jobID string, attributes map[string]string) error {
	return client.client.UpdateJobAttributes(ctx, client.session, jobID, attributes)
}
func (client fieryAutomationClient) DeleteJob(ctx context.Context, jobID string) error {
	return client.client.DeleteJob(ctx, client.session, jobID)
}
func (client fieryAutomationClient) CancelJob(ctx context.Context, jobID string) error {
	return client.client.CancelJob(ctx, client.session, jobID)
}
func (client fieryAutomationClient) JobAction(ctx context.Context, jobID, action string) error {
	return client.client.JobAction(ctx, client.session, jobID, action)
}
func (client fieryAutomationClient) GetRawJobResponses(ctx context.Context, jobID string) []fiery.JobRawResponse {
	return client.client.GetRawJobResponses(ctx, client.session, jobID)
}

var _ core.AutomationConnector = fieryAutomationConnector{}
var _ core.AutomationClient = fieryAutomationClient{}
