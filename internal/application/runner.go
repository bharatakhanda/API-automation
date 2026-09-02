package application

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/joboutcome"
	"api-automation/internal/reportxlsx"
)

type ServerPresetSelection struct {
	ID          string `json:"id"`
	Description string `json:"description"`
}

type RunRequest struct {
	Files          []string
	Combinations   []combinations.Combination
	Modes          []RunMode
	Workers        int
	Capabilities   capabilities.Model
	ServerPreset   *ServerPresetSelection
	TestIntent     TestIntent
	ConstraintMode ConstraintMode
}

type Runner struct {
	Connector AutomationConnector
	Recorder  ResultRecorder
	Polling   PollingPolicy
	Now       func() time.Time
	NewID     func() string
}

var operationSequence atomic.Uint64

func NewRunner(connector AutomationConnector, recorder ResultRecorder) *Runner {
	return &Runner{Connector: connector, Recorder: recorder, Polling: DefaultPollingPolicy()}
}

func (runner *Runner) Start(parent context.Context, request RunRequest) *Operation {
	if parent == nil {
		parent = context.Background()
	}
	now := runner.Now
	if now == nil {
		now = time.Now
	}
	newID := runner.NewID
	if newID == nil {
		newID = func() string {
			return fmt.Sprintf("run-%d-%d", now().UnixNano(), operationSequence.Add(1))
		}
	}
	operationID := newID()
	ctx, cancel := context.WithCancel(parent)
	queue := newEventQueue(operationID, now)
	done := make(chan RunTerminalEvent, 1)
	operation := &Operation{ID: operationID, Events: queue.output, Done: done, cancel: cancel}

	execution := &runExecution{
		runner:  runner,
		request: cloneRunRequest(request),
		queue:   queue,
		now:     now,
	}
	go func() {
		defer cancel()
		terminal := execution.run(ctx)
		if runner.Recorder != nil {
			if err := runner.Recorder.Close(); err != nil {
				execution.setStorageError(err)
			}
		}
		terminal.StorageError = execution.storageError()
		terminal.Progress = execution.progressSnapshot()
		queue.publish(RunEvent{Kind: RunEventTerminal, Terminal: &terminal})
		queue.close()
		done <- terminal
		close(done)
	}()
	return operation
}

type runExecution struct {
	runner  *Runner
	request RunRequest
	queue   *eventQueue
	now     func() time.Time

	mu               sync.Mutex
	progress         RunProgressEvent
	storageErrorText string
}

func (execution *runExecution) run(ctx context.Context) RunTerminalEvent {
	request := &execution.request
	request.TestIntent = normalizeTestIntent(request.TestIntent)
	if request.ConstraintMode != ConstraintControlledApply {
		request.ConstraintMode = ConstraintValidationOnly
	}
	if request.TestIntent == TestIntentConstraint {
		request.Modes = []RunMode{RunModes()[0]}
	}
	planned := PlannedTestCount(len(request.Files), len(request.Combinations), len(request.Modes))
	workers := EffectiveWorkerCount(request.Workers, planned)
	execution.mu.Lock()
	execution.progress.Planned = planned
	execution.mu.Unlock()

	if len(request.Files) == 0 {
		return failedTerminal("No test files were selected.")
	}
	if len(request.Combinations) == 0 {
		return failedTerminal("No executable combinations were provided.")
	}
	if len(request.Modes) == 0 {
		return failedTerminal("No run modes were selected.")
	}
	if planned == 0 {
		return failedTerminal("No executable tests were generated from the selected files, values, and run modes.")
	}
	if request.TestIntent == TestIntentPositive && CombinationsRequireRIPReadback(request.Combinations) && !RunModesIncludeAction(request.Modes, "rip") {
		return failedTerminal("Selected capabilities require RIP before strict verification. Select Process and Hold or RIP run mode.")
	}
	if execution.runner.Connector == nil {
		return failedTerminal("Automation connector is unavailable.")
	}

	client, connection, err := execution.runner.Connector.Connect(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
			return RunTerminalEvent{Status: RunStatusCancelled}
		}
		return failedTerminal(err.Error())
	}
	execution.queue.publish(RunEvent{Kind: RunEventStarted, Started: &RunStartedEvent{
		PlannedTests: planned, Workers: workers, SessionLoginPath: connection.SessionLoginPath,
	}})
	execution.publishProgress()

	type executionCase struct {
		file       string
		attributes map[string]string
		mode       RunMode
	}
	jobs := make(chan executionCase)
	var workersWG sync.WaitGroup
	for worker := 0; worker < workers; worker++ {
		workersWG.Add(1)
		go func() {
			defer workersWG.Done()
			for job := range jobs {
				result := execution.executeJobSafely(ctx, client, job.file, job.attributes, job.mode)
				execution.recordResult(result)
			}
		}()
	}

	cancelled := false
producer:
	for _, file := range request.Files {
		for _, combination := range request.Combinations {
			for _, mode := range request.Modes {
				job := executionCase{file: file, attributes: CombinationToAttributes(combination), mode: cloneRunMode(mode)}
				select {
				case <-ctx.Done():
					cancelled = true
					break producer
				case jobs <- job:
				}
			}
		}
	}
	close(jobs)
	workersWG.Wait()
	if cancelled || ctx.Err() != nil {
		return RunTerminalEvent{Status: RunStatusCancelled}
	}
	return RunTerminalEvent{Status: RunStatusCompleted}
}

func (execution *runExecution) executeJobSafely(ctx context.Context, client AutomationClient, file string, attributes map[string]string, mode RunMode) (result reportxlsx.Result) {
	started := execution.now()
	result = reportxlsx.Result{JobName: filepath.Base(file), Mode: mode.Label, SetValues: CloneStringMap(attributes)}
	defer func() {
		if recovered := recover(); recovered != nil {
			stack := debug.Stack()
			execution.queue.publish(RunEvent{Kind: RunEventPanic, Panic: &RunPanicEvent{
				File: filepath.Base(file), Mode: mode.Label, Value: fmt.Sprint(recovered), Stack: string(stack),
			}})
			result.Result = "ERROR"
			result.Detail = fmt.Sprintf("mode=%s: unexpected internal error; see logs/crash.log", mode.Label)
			result.DurationMS = elapsedMilliseconds(started, execution.now())
		}
	}()
	return execution.executeJob(ctx, client, file, attributes, mode, started, result)
}

func (execution *runExecution) executeJob(ctx context.Context, client AutomationClient, file string, attributes map[string]string, mode RunMode, started time.Time, result reportxlsx.Result) reportxlsx.Result {
	finish := func(status, detail string, actual map[string]string) reportxlsx.Result {
		result.Result = status
		result.DurationMS = elapsedMilliseconds(started, execution.now())
		if execution.request.ServerPreset != nil {
			detail += "; server preset=" + execution.request.ServerPreset.Description
		}
		result.Detail = detail
		result.JobName = jobNameFromAttributes(actual, result.JobName)
		result.GetValues = SelectedReadbackValues(actual, attributes)
		result.JobStatus = actual["status"]
		result.JobState = actual["state"]
		result.JobError = firstNonEmptyText(actual["error"], actual["pdl error"])
		result.LastEvent = actual["last joblog event"]
		return result
	}
	log := execution.log
	polling := normalizePollingPolicy(execution.runner.Polling)
	matcher := AttributeMatcher{Capabilities: execution.request.Capabilities}

	imported, err := client.ImportJobToQueue(ctx, file, mode.ImportQueue)
	if err != nil {
		return finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), nil)
	}
	result.JobID = imported.JobID
	log("Imported %s as job %s into queue %s for mode %s", filepath.Base(file), imported.JobID, mode.ImportQueue, mode.Label)
	log("Confirming job %s is visible after import", imported.JobID)
	if _, err := WaitJobCondition(ctx, client, imported.JobID, "job visible after import", polling.ImportVisibleTimeout, polling.ImportVisibleInterval, func(attributes map[string]string) bool {
		return strings.TrimSpace(attributes["id"]) == imported.JobID || strings.TrimSpace(attributes["status"]) != "" || strings.TrimSpace(attributes["state"]) != ""
	}); err != nil {
		return finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), nil)
	}

	log("Waiting for job %s status=done spooling", imported.JobID)
	spooled, err := WaitJobCondition(ctx, client, imported.JobID, "done spooling after import", polling.SpoolingTimeout, polling.SpoolingInterval, func(attributes map[string]string) bool {
		return StatusEquals("done spooling")(attributes) || !joboutcome.Evaluate(attributes, joboutcome.Policy{}).Pass
	})
	if err != nil {
		return finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), spooled)
	}
	if outcome := joboutcome.Evaluate(spooled, joboutcome.Policy{}); !outcome.Pass {
		result.Lifecycle = outcome.Summary()
		return finish("FAIL", fmt.Sprintf("mode=%s: job failed while spooling: %s", mode.Label, outcome.Summary()), spooled)
	}
	if err := ValidateCustomPageRange(attributes, spooled); err != nil {
		return finish("FAIL", fmt.Sprintf("mode=%s: custom page range is invalid for %s: %v", mode.Label, filepath.Base(file), err), spooled)
	}

	if preset := execution.request.ServerPreset; preset != nil {
		log("Applying Fiery server preset %s to job %s", preset.Description, imported.JobID)
		if err := client.ApplyServerPreset(ctx, imported.JobID, preset.ID); err != nil {
			return finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), spooled)
		}
		log("Fiery server preset %s was accepted for job %s", preset.Description, imported.JobID)
	}

	if execution.request.TestIntent == TestIntentConstraint {
		status, detail, actual := execution.executeConstraintCase(ctx, client, imported.JobID, attributes, spooled)
		result.Lifecycle = detail
		return finish(status, detail, actual)
	}

	if len(attributes) > 0 {
		if capabilities.NeedsConstraintCheck(execution.request.Capabilities, attributes) {
			check, constraintErr := client.CheckJobConstraints(ctx, imported.JobID, attributes)
			if constraintErr != nil {
				return finish("ERROR", fmt.Sprintf("mode=%s: job constraint validation failed: %v", mode.Label, constraintErr), nil)
			}
			if check.HasConflicts() {
				actual, _ := client.GetJobAttributes(ctx, imported.JobID)
				result.Lifecycle = "Fiery constraint conflict: " + formatAttributeMap(check.Conflicts)
				return finish("FAIL", fmt.Sprintf("mode=%s: Fiery rejected the selected settings as constrained: %s; solutions=%v", mode.Label, formatAttributeMap(check.Conflicts), check.Solutions), actual)
			}
			if check.Supported {
				log("Fiery job constraint check passed for job %s", imported.JobID)
			} else if check.Warning != "" {
				log("Fiery job constraint endpoint unavailable for job %s; update response remains authoritative: %s", imported.JobID, shortText(check.Warning, 300))
			}
		}
		log("Setting job %s attributes after done spooling: %s", imported.JobID, formatAttributeMap(attributes))
		execution.queue.publish(RunEvent{Kind: RunEventAttributeWrite, AttributeWrite: &AttributeWriteEvent{JobID: imported.JobID, Attributes: CloneStringMap(attributes)}})
		if err := client.UpdateJobAttributes(ctx, imported.JobID, attributes); err != nil {
			return finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), nil)
		}
		if ModeIncludesAction(mode, "rip") {
			log("Attribute update accepted for job %s; final set/get verification will run after RIP", imported.JobID)
		} else {
			log("Attribute update accepted for job %s; final set/get verification will run after mode %s", imported.JobID, mode.Label)
		}
	}

	observeReadback := execution.readbackObserver(imported.JobID)
	if ModeIncludesAction(mode, "delete") {
		actual, readErr := ReadBackAttributes(ctx, client, imported.JobID, attributes, matcher, polling, observeReadback)
		deleteErr := client.DeleteJob(ctx, imported.JobID)
		if deleteErr != nil {
			return finish("ERROR", fmt.Sprintf("mode=%s: delete failed: %v", mode.Label, deleteErr), actual)
		}
		log("Deleted job %s successfully for Delete mode", imported.JobID)
		if readErr != nil {
			return finish("ERROR", fmt.Sprintf("mode=%s: job deleted, but pre-delete readback failed: %v", mode.Label, readErr), actual)
		}
		for key, want := range attributes {
			if !matcher.AttributeMapValueMatches(actual, key, want) {
				return finish("FAIL", fmt.Sprintf("mode=%s: job deleted, but pre-delete verification failed for %s set=%q got=%q", mode.Label, key, want, actual[key]), actual)
			}
		}
		return finish("PASS", fmt.Sprintf("mode=%s: job was deleted successfully using its dedicated test job", mode.Label), actual)
	}

	if err := ExecuteModeLifecycle(ctx, client, imported.JobID, mode, polling, func(message string) { log("%s", message) }); err != nil {
		var failed *LifecycleFailure
		if errors.As(err, &failed) {
			result.Lifecycle = failed.Outcome.Summary()
			return finish("FAIL", fmt.Sprintf("mode=%s: lifecycle verification failed: %s", mode.Label, failed.Outcome.Summary()), failed.Attributes)
		}
		return finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), nil)
	}

	actual, err := ReadBackAttributes(ctx, client, imported.JobID, attributes, matcher, polling, observeReadback)
	if err != nil {
		return finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), actual)
	}
	outcome := joboutcome.Evaluate(actual, LifecyclePolicy(mode))
	result.Lifecycle = outcome.Summary()
	if !outcome.Pass {
		return finish("FAIL", fmt.Sprintf("mode=%s: lifecycle verification failed: %s", mode.Label, outcome.Summary()), actual)
	}
	status := "PASS"
	detail := fmt.Sprintf("mode=%s: lifecycle passed (%s); set values matched get values", mode.Label, outcome.Summary())
	if len(attributes) == 0 {
		detail = fmt.Sprintf("mode=%s: lifecycle passed (%s); no job attributes were selected for set/get verification", mode.Label, outcome.Summary())
	}
	for key, want := range attributes {
		if !matcher.AttributeMapValueMatches(actual, key, want) {
			status = "FAIL"
			detail = fmt.Sprintf("mode=%s: %s set=%q got=%q status=%q state=%q display=%q recent=%q related=%s availableKeys=%s", mode.Label, key, want, actual[key], actual["status"], actual["state"], actual["display status"], actual["recent action"], relatedReadbackValues(actual), shortText(strings.Join(sortedStringKeys(actual), ","), 220))
			if RequiresRIPReadback(key) && !ModeIncludesAction(mode, "rip") {
				detail += "; note=this attribute may require RIP for strict verification"
			}
			responses := client.GetRawJobResponses(ctx, imported.JobID)
			safeResponses := make([]RawComparisonResponse, 0, len(responses))
			for _, response := range responses {
				safeResponses = append(safeResponses, RawComparisonResponse{
					Method: response.Method, Endpoint: response.Endpoint, ResponseProto: response.ResponseProto,
					StatusCode: response.StatusCode, Body: response.Body,
				})
			}
			execution.queue.publish(RunEvent{Kind: RunEventRawComparison, RawComparison: &RawComparisonEvent{
				JobID: imported.JobID, Responses: safeResponses,
			}})
			break
		}
	}
	return finish(status, detail, actual)
}

func (execution *runExecution) executeConstraintCase(ctx context.Context, client AutomationClient, jobID string, attributes, spooled map[string]string) (string, string, map[string]string) {
	localConflicts := capabilities.ValidateCombination(execution.request.Capabilities, CombinationForConstraintValidation(attributes))
	if len(localConflicts) == 0 {
		return "ERROR", "constraint test plan lost its published local conflict; no intentionally invalid update was attempted", spooled
	}
	status, detail := "ERROR", "constraint test did not complete"
	if execution.request.ConstraintMode == ConstraintControlledApply {
		execution.log("Controlled constraint test job %s: sending locally proven incompatible attributes", jobID)
		err := client.UpdateJobAttributes(ctx, jobID, attributes)
		switch {
		case err == nil:
			status = "FAIL"
			detail = "controlled constraint apply was accepted; expected a constraint rejection"
		case ExpectedConstraintRejection(err):
			status = "PASS"
			detail = "controlled constraint apply received the expected client-side constraint rejection: " + shortText(err.Error(), 500)
		default:
			status = "ERROR"
			detail = "controlled constraint apply failed for an operational or unrelated reason, not an expected constraint rejection: " + shortText(err.Error(), 500)
		}
	} else {
		execution.log("Validation-only constraint test job %s: checking incompatible attributes without applying them", jobID)
		check, err := client.CheckJobConstraints(ctx, jobID, attributes)
		switch {
		case err != nil:
			status = "ERROR"
			detail = "constraint validation endpoint failed: " + err.Error()
		case !check.Supported:
			status = "ERROR"
			detail = "constraint validation endpoint is unavailable; the expected rejection cannot be proven safely"
		case !check.HasConflicts():
			status = "FAIL"
			detail = "Fiery validation accepted the locally incompatible values; expected a constraint conflict"
		default:
			status = "PASS"
			detail = "Fiery returned the expected constraint conflict without applying the invalid settings: " + formatAttributeMap(check.Conflicts)
		}
	}
	actual, readErr := client.GetJobAttributes(ctx, jobID)
	if readErr != nil {
		actual = spooled
		detail += "; final held-job readback unavailable: " + shortText(readErr.Error(), 240)
	}
	if deleteErr := client.DeleteJob(ctx, jobID); deleteErr != nil {
		return "ERROR", detail + "; disposable held-job cleanup failed: " + shortText(deleteErr.Error(), 300), actual
	}
	execution.log("Deleted disposable constraint-test job %s", jobID)
	return status, detail + "; disposable held job deleted", actual
}

func (execution *runExecution) readbackObserver(jobID string) ReadbackObserver {
	return func(actual, expected map[string]string, matched bool, err error) {
		event := ReadbackEvent{JobID: jobID, Actual: CloneStringMap(actual), Expected: CloneStringMap(expected), Matched: matched}
		if err != nil {
			event.Error = err.Error()
		}
		execution.queue.publish(RunEvent{Kind: RunEventReadback, Readback: &event})
	}
}

func (execution *runExecution) recordResult(result reportxlsx.Result) {
	storageError := ""
	if execution.runner.Recorder != nil {
		if err := execution.runner.Recorder.Append(result); err != nil {
			storageError = err.Error()
			execution.setStorageError(err)
		}
	}
	execution.mu.Lock()
	execution.progress.Executed++
	switch strings.ToUpper(strings.TrimSpace(result.Result)) {
	case "PASS":
		execution.progress.Passed++
	case "FAIL":
		execution.progress.Failed++
	default:
		execution.progress.Errors++
	}
	progress := execution.progress
	execution.mu.Unlock()
	execution.queue.publish(RunEvent{Kind: RunEventResult, Result: &RunResultEvent{Result: cloneReportResult(result), StorageError: storageError}})
	execution.queue.publish(RunEvent{Kind: RunEventProgress, Progress: &progress})
}

func (execution *runExecution) publishProgress() {
	progress := execution.progressSnapshot()
	execution.queue.publish(RunEvent{Kind: RunEventProgress, Progress: &progress})
}

func (execution *runExecution) progressSnapshot() RunProgressEvent {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	return execution.progress
}

func (execution *runExecution) setStorageError(err error) {
	if err == nil {
		return
	}
	execution.mu.Lock()
	if execution.storageErrorText == "" {
		execution.storageErrorText = err.Error()
	}
	execution.mu.Unlock()
}

func (execution *runExecution) storageError() string {
	execution.mu.Lock()
	defer execution.mu.Unlock()
	return execution.storageErrorText
}

func (execution *runExecution) log(format string, args ...any) {
	execution.queue.publish(RunEvent{Kind: RunEventLog, Log: &RunLogEvent{Message: fmt.Sprintf(format, args...)}})
}

func failedTerminal(message string) RunTerminalEvent {
	return RunTerminalEvent{Status: RunStatusFailed, Error: message}
}

func cloneRunRequest(request RunRequest) RunRequest {
	request.Files = append([]string(nil), request.Files...)
	request.Combinations = cloneCombinations(request.Combinations)
	modes := request.Modes
	request.Modes = make([]RunMode, len(modes))
	for index, mode := range modes {
		request.Modes[index] = cloneRunMode(mode)
	}
	request.Capabilities = cloneCapabilityModel(request.Capabilities)
	if request.ServerPreset != nil {
		preset := *request.ServerPreset
		request.ServerPreset = &preset
	}
	return request
}

func cloneRunMode(mode RunMode) RunMode {
	mode.Actions = append([]string(nil), mode.Actions...)
	return mode
}

func cloneCapabilityModel(model capabilities.Model) capabilities.Model {
	model.Options = append([]capabilities.Option(nil), model.Options...)
	for optionIndex := range model.Options {
		option := &model.Options[optionIndex]
		option.Values = append([]string(nil), option.Values...)
		option.Scopes = append([]string(nil), option.Scopes...)
		if option.Range != nil {
			rangeCopy := *option.Range
			option.Range = &rangeCopy
		}
		if option.Constraints != nil {
			constraints := make(capabilities.Constraints, len(option.Constraints))
			for value, dependencies := range option.Constraints {
				dependencyCopy := make(map[string][]string, len(dependencies))
				for dependency, values := range dependencies {
					dependencyCopy[dependency] = append([]string(nil), values...)
				}
				constraints[value] = dependencyCopy
			}
			option.Constraints = constraints
		}
	}
	return model
}

func cloneReportResult(result reportxlsx.Result) reportxlsx.Result {
	result.SetValues = CloneStringMap(result.SetValues)
	result.GetValues = CloneStringMap(result.GetValues)
	return result
}

func elapsedMilliseconds(started, completed time.Time) int64 {
	if completed.Before(started) {
		return 0
	}
	return completed.Sub(started).Milliseconds()
}

func firstNonEmptyText(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func jobNameFromAttributes(attributes map[string]string, fallback string) string {
	for _, key := range []string{"job name", "name", "document name", "title"} {
		if value := strings.TrimSpace(attributes[key]); value != "" {
			return value
		}
	}
	return fallback
}

func formatAttributeMap(attributes map[string]string) string {
	keys := sortedStringKeys(attributes)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", key, attributes[key]))
	}
	return strings.Join(parts, ", ")
}

func sortedStringKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func relatedReadbackValues(attributes map[string]string) string {
	keys := []string{"EFResolution", "Resolution", "EFPrintSpeed", "EFRaster", "EFPrintSize", "PageSize", "CustomPrintSize", "has disk raster?", "EFBrightness", "EFColorMode", "num copies"}
	related := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := attributes[key]; ok {
			related[key] = value
		}
	}
	if len(related) == 0 {
		return "none"
	}
	return formatAttributeMap(related)
}

func shortText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	return value[:limit] + "..."
}
