//go:build windows

package appgio

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"os"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/copyvalues"
	"api-automation/internal/fiery"
	"api-automation/internal/files"
	"api-automation/internal/joboutcome"
	"api-automation/internal/model"
	"api-automation/internal/pagevalues"
	"api-automation/internal/preflight"
	"api-automation/internal/presets"
	"api-automation/internal/reportxlsx"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
)

const (
	pageRangeOptionID       = "EFPageRange"
	pageRangeLegacyDataID   = "DPP_PAGE_RANGE"
	pageRangeRangeValue     = "Range1"
	pageRangeInternalPrefix = "__API_AUTOMATION_CUSTOM_PAGE_RANGE__:"
	outputProfileOptionID   = "EFOutProfile"
	noServerPresetID        = "__API_AUTOMATION_NO_SERVER_PRESET__"

	defaultCaseLimit       = 100
	maxCaseLimit           = 10_000
	maxWorkerCount         = 10
	maxDisplayedResults    = 250
	maxDisplayedLogLines   = 500
	maxRetainedResults     = 2_000
	maxRetainedLogLines    = 2_000
	retainedEntryTrimBatch = 256
)

const (
	pageConnection = iota
	pageOverview
	pageTestSettings
	pageJobProperties
	pageAutomation
	pageResults
	pageLogs
	pageAdministration
	pageCount
)

type automationValueSource string

const (
	valueSourceBaseline   automationValueSource = "baseline"
	valueSourceDefaults   automationValueSource = "defaults"
	valueSourceSelected   automationValueSource = "selected"
	valueSourceAdvertised automationValueSource = "advertised"
)

type automationTestIntent string

const (
	testIntentPositive   automationTestIntent = "positive"
	testIntentConstraint automationTestIntent = "constraint"
)

type constraintTestMode string

const (
	constraintValidationOnly  constraintTestMode = "validation"
	constraintControlledApply constraintTestMode = "controlled_apply"
)

var palette = struct {
	bg, surface, surfaceAlt, navy, text, muted, border, primary, primaryDim, danger, success color.NRGBA
}{
	bg:         rgb(0xeaf0f7),
	surface:    rgb(0xf7faff),
	surfaceAlt: rgb(0xeef4fb),
	navy:       rgb(0x0f172a),
	text:       rgb(0x172033),
	muted:      rgb(0x5f718b),
	border:     rgb(0xcbd8e8),
	primary:    rgb(0x2563eb),
	primaryDim: rgb(0xd7e6fb),
	danger:     rgb(0xb91c1c),
	success:    rgb(0x15803d),
}

type Window struct {
	window       *app.Window
	theme        *material.Theme
	ops          op.Ops
	list         widget.List
	categoryList widget.List

	serverIP, secretKey, password widget.Editor
	folderPath, filePath          widget.Editor
	workers, maxCases             widget.Editor
	copiesInput, pageRangeInput   widget.Editor
	jobActionID                   widget.Editor
	capabilitySearch, presetName  widget.Editor
	adminConfirmation             widget.Editor

	runButton, cancelButton                                widget.Clickable
	testServerButton, apiTraceButton, exportButton         widget.Clickable
	cancelJobButton, deleteJobButton                       widget.Clickable
	savePresetButton, loadPresetButton, deletePresetButton widget.Clickable
	inspectJobsButton, restartServerButton                 widget.Clickable
	rebootServerButton, clearAllJobsButton                 widget.Clickable
	browseFolderButton, browseFileButton                   widget.Clickable
	navButtons                                             []widget.Clickable
	applyConnectionButton, cancelConnectionChangeButton    widget.Clickable
	changeConnectionButton, overviewCaptureButton          widget.Clickable
	resetPropertiesButton, resetAutomationButton           widget.Clickable
	resetTestSetupButton                                   widget.Clickable
	baselineSourceButton, defaultsSourceButton             widget.Clickable
	selectedSourceButton, advertisedSourceButton           widget.Clickable
	positiveIntentButton, constraintIntentButton           widget.Clickable
	validationOnlyButton, controlledApplyButton            widget.Clickable
	singleStrategyButton, randomStrategyButton             widget.Clickable
	allPermButton, pairwiseButton                          widget.Clickable
	modeChecks                                             []widget.Bool
	fileModeGroup                                          widget.Enum

	activePage          int
	strategy            combinations.Strategy
	valueSource         automationValueSource
	testIntent          automationTestIntent
	constraintMode      constraintTestMode
	serverPresetGroup   widget.Enum
	activeServer        model.ServerConnection
	hasActiveServer     bool
	configuredSecret    string
	testedConnectionKey string

	capabilities          capabilities.Model
	selected              map[string]map[string]*widget.Bool
	groupChecks           map[string]*widget.Bool
	optionChecks          map[string]*widget.Bool
	numericInputs         map[string]*widget.Editor
	categoryButtons       map[string]*widget.Clickable
	activeCapabilityGroup string
	presetStore           *presets.Store
	presetList            []presets.Preset
	constraintSkipped     int
	constraintWarning     string
	mu                    sync.Mutex
	backgroundMu          sync.Mutex
	backgroundWG          sync.WaitGroup
	appContext            context.Context
	appCancel             context.CancelFunc
	closing               atomic.Bool
	log                   []string
	logCount              int
	results               []resultRow
	resultCount           int
	passedCount           int
	failedCount           int
	errorCount            int
	status                string
	serverTestStatus      string
	serverTestOK          bool
	healthStatus          string
	healthDetail          string
	healthCheckedAt       time.Time
	healthLatency         time.Duration
	healthCancel          context.CancelFunc
	healthGeneration      uint64
	captureActive         bool
	captureProgress       float32
	capturePhase          string
	running               atomic.Bool
	testingServer         atomic.Bool
	exportingResults      atomic.Bool
	managingJob           atomic.Bool
	managingServer        atomic.Bool
	inspectingJobs        atomic.Bool
	cancel                context.CancelFunc
	diagnostic            *diagnosticLog
	resultStore           *reportxlsx.ResultStore
	resultStoreError      string
	lastRun               reportxlsx.Summary
	adminStatus           string
	adminInventoryServer  string
	adminInventoryAt      time.Time
	adminJobCount         int
}

type resultRow struct{ JobID, JobName, Result, Duration, Status, State, Detail string }

type workspacePage struct {
	NavigationLabel string
	Title           string
	Subtitle        string
}

var workspacePages = []workspacePage{
	{NavigationLabel: "Connection", Title: "Server Connection", Subtitle: "Test and approve a Fiery connection before configuring automation."},
	{NavigationLabel: "Overview", Title: "Overview", Subtitle: "Server details, capability readiness, and automation progress."},
	{NavigationLabel: "Test Settings", Title: "Test Settings", Subtitle: "Choose the local test assets used by this automation run."},
	{NavigationLabel: "Job Properties", Title: "Job Properties", Subtitle: "Choose exact server-advertised values in the Fiery property hierarchy."},
	{NavigationLabel: "Automation", Title: "Automation", Subtitle: "Define test intent, value source, generation strategy, lifecycle, and concurrency."},
	{NavigationLabel: "Results", Title: "Results", Subtitle: "Review strict set/get evidence and lifecycle verdicts."},
	{NavigationLabel: "Activity Logs", Title: "Activity Logs", Subtitle: "Review application activity and the complete diagnostic-log location."},
	{NavigationLabel: "Administration", Title: "Administration", Subtitle: "Guarded Fiery restart, reboot, inventory, and clear-jobs operations."},
}

type apiTraceStage struct {
	Name      string                 `json:"name"`
	Captured  string                 `json:"capturedAt"`
	Responses []fiery.JobRawResponse `json:"responses"`
}

type apiTraceReport struct {
	CapturedAt       string             `json:"capturedAt"`
	Server           string             `json:"server"`
	File             string             `json:"file"`
	Mode             string             `json:"mode"`
	ServerPresetID   string             `json:"serverPresetId,omitempty"`
	ServerPresetName string             `json:"serverPresetName,omitempty"`
	JobID            string             `json:"jobId,omitempty"`
	Attributes       map[string]string  `json:"attributes"`
	UpdateProtocol   string             `json:"updateProtocol"`
	SessionLogin     string             `json:"sessionLogin,omitempty"`
	Import           fiery.ImportResult `json:"import"`
	Stages           []apiTraceStage    `json:"stages"`
	Final            map[string]string  `json:"finalAttributes,omitempty"`
	Lifecycle        string             `json:"lifecycle,omitempty"`
	Result           string             `json:"result"`
	Error            string             `json:"error,omitempty"`
}

type runMode struct {
	Label, ImportQueue string
	Actions            []string
}

type lifecycleFailure struct {
	outcome joboutcome.Outcome
	attrs   map[string]string
}

func (e *lifecycleFailure) Error() string { return e.outcome.Summary() }

var runModes = []runMode{
	{Label: "Hold", ImportQueue: "hold"},
	{Label: "Process and Hold", ImportQueue: "hold", Actions: []string{"rip"}},
	{Label: "RIP", ImportQueue: "hold", Actions: []string{"rip"}},
	{Label: "Press Print", ImportQueue: "hold", Actions: []string{"rip", "production", "press_print"}},
	{Label: "Ready to Print", ImportQueue: "hold", Actions: []string{"rip", "production"}},
	{Label: "Print", ImportQueue: "hold", Actions: []string{"rip", "production", "press_print", "print"}},
	{Label: "Cancel while Processing/Ripping", ImportQueue: "hold", Actions: []string{"cancel_ripping"}},
	{Label: "Cancel while Waiting to Print", ImportQueue: "hold", Actions: []string{"rip", "production", "cancel_waiting"}},
	{Label: "Cancel while Printing", ImportQueue: "hold", Actions: []string{"rip", "production", "press_print", "cancel_printing"}},
	{Label: "Delete", ImportQueue: "hold", Actions: []string{"delete"}},
}

func New() *Window {
	appContext, appCancel := context.WithCancel(context.Background())
	w := &Window{
		window: new(app.Window), theme: material.NewTheme(),
		selected: map[string]map[string]*widget.Bool{}, groupChecks: map[string]*widget.Bool{}, optionChecks: map[string]*widget.Bool{},
		numericInputs: map[string]*widget.Editor{}, categoryButtons: map[string]*widget.Clickable{},
		strategy: combinations.StrategySingle, valueSource: valueSourceSelected,
		testIntent: testIntentPositive, constraintMode: constraintValidationOnly,
		activeCapabilityGroup: "Job Info", activePage: pageConnection,
		configuredSecret: fiery.DefaultSecretKey,
		status:           "Test and approve a server connection to begin.", diagnostic: newDiagnosticLog(),
		appContext: appContext, appCancel: appCancel,
	}
	w.theme.Palette = material.Palette{Bg: palette.bg, Fg: palette.text, ContrastBg: palette.primary, ContrastFg: rgb(0xffffff)}
	w.theme.TextSize = 15
	w.list.Axis = layout.Vertical
	w.categoryList.Axis = layout.Horizontal
	initEditor(&w.serverIP, "")
	initEditor(&w.secretKey, "")
	initEditor(&w.password, "")
	w.secretKey.Mask = '•'
	w.password.Mask = '•'
	initEditor(&w.folderPath, "")
	initEditor(&w.filePath, "")
	initEditor(&w.workers, "1")
	initEditor(&w.maxCases, "100")
	initEditor(&w.copiesInput, "1")
	initEditor(&w.pageRangeInput, "")
	initEditor(&w.jobActionID, "")
	initEditor(&w.capabilitySearch, "")
	initEditor(&w.presetName, "")
	initEditor(&w.adminConfirmation, "")
	if presetPath, err := presets.DefaultPath(); err == nil {
		if store, storeErr := presets.New(presetPath); storeErr == nil {
			w.presetStore = store
			w.presetList, _ = store.List()
		}
	}
	w.fileModeGroup.Value = "all"
	w.serverPresetGroup.Value = noServerPresetID
	w.modeChecks = make([]widget.Bool, len(runModes))
	if len(w.modeChecks) > 0 {
		w.modeChecks[0].Value = true
	}
	w.serverTestStatus = "Not tested"
	w.healthStatus = "Not checked"
	w.adminStatus = "No administrative action is in progress."
	w.window.Option(app.Title("API Automation"), app.Size(unit.Dp(1180), unit.Dp(820)), app.MinSize(unit.Dp(1024), unit.Dp(700)))
	return w
}

func initEditor(e *widget.Editor, text string) { e.SingleLine = true; e.Submit = true; e.SetText(text) }

func Run() int {
	// Gio's Windows app.Main blocks forever by design. The window goroutine
	// must terminate the process after its event loop and cleanup complete;
	// waiting for app.Main to return leaves a headless process alive whenever
	// other runtime goroutines prevent Go's deadlock detector from firing.
	go func() {
		os.Exit(runWindow())
	}()
	app.Main()
	return 0 // Unreachable on Windows.
}

func runWindow() (code int) {
	_, _ = win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
	defer win.CoUninitialize()
	defer func() {
		if recovered := recover(); recovered != nil {
			_ = writeCrashReport(fmt.Sprintf("panic: %v", recovered), debug.Stack())
			code = 1
		}
	}()
	if err := New().Run(); err != nil {
		_ = writeCrashReport(err.Error(), nil)
		return 1
	}
	return 0
}

func (w *Window) Run() error {
	defer w.diagnostic.close()
	defer w.closeResultStore()
	w.diagnostic.printf("Application started. Diagnostic log: %s", w.diagnostic.path)
	for {
		e := w.window.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			w.shutdownBackground(5 * time.Second)
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&w.ops, e)
			w.handleClicks(gtx)
			w.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (w *Window) launchBackground(operation string, work func()) {
	w.backgroundMu.Lock()
	if w.closing.Load() {
		w.backgroundMu.Unlock()
		return
	}
	w.backgroundWG.Add(1)
	w.backgroundMu.Unlock()
	go func() {
		defer w.backgroundWG.Done()
		defer func() {
			if recovered := recover(); recovered != nil {
				stack := debug.Stack()
				_ = writeCrashReport(fmt.Sprintf("panic in %s: %v", operation, recovered), stack)
				w.diagnostic.printf("PANIC: operation=%s value=%v stack=%s", operation, recovered, stack)
				w.setStatus(operation + " failed unexpectedly. See logs/crash.log.")
			}
		}()
		work()
	}()
}

func (w *Window) shutdownBackground(timeout time.Duration) bool {
	w.backgroundMu.Lock()
	w.closing.Store(true)
	if w.appCancel != nil {
		w.appCancel()
	}
	if w.cancel != nil {
		w.cancel()
	}
	w.backgroundMu.Unlock()

	stopped := make(chan struct{})
	go func() {
		w.backgroundWG.Wait()
		close(stopped)
	}()
	select {
	case <-stopped:
		w.diagnostic.printf("SHUTDOWN: all background operations stopped")
		return true
	case <-time.After(timeout):
		// Returning from the window loop is more important than waiting for a
		// slow server socket. Every Fiery request is context-bound and also has
		// a client timeout, so remaining goroutines cannot own the GUI lifetime.
		w.diagnostic.printf("SHUTDOWN: timed out after %s waiting for background operations; continuing process exit", timeout)
		return false
	}
}

func (w *Window) rootContext() context.Context {
	if w.appContext != nil {
		return w.appContext
	}
	return context.Background()
}

func (w *Window) invalidate() {
	if !w.closing.Load() && w.window != nil {
		w.window.Invalidate()
	}
}

func (w *Window) handleClicks(gtx layout.Context) {
	// Editors consume key events during Update/Layout. Drain them before button
	// actions read Text(), including editors on a page being left this frame.
	w.updateEditors(gtx)
	w.invalidateChangedConnectionTest()

	for w.testServerButton.Clicked(gtx) {
		w.testServerConnection()
	}
	for w.applyConnectionButton.Clicked(gtx) {
		w.applyTestedConnection()
	}
	for w.cancelConnectionChangeButton.Clicked(gtx) {
		w.cancelConnectionChange()
	}
	for w.changeConnectionButton.Clicked(gtx) {
		w.beginConnectionChange()
	}
	for w.overviewCaptureButton.Clicked(gtx) {
		w.captureCapabilities()
	}
	for w.resetPropertiesButton.Clicked(gtx) {
		w.resetJobProperties()
	}
	for w.resetAutomationButton.Clicked(gtx) {
		w.resetAutomationSettings()
	}
	for w.resetTestSetupButton.Clicked(gtx) {
		w.resetTestSetup()
	}
	for w.cancelJobButton.Clicked(gtx) {
		w.startManualJobAction("cancel")
	}
	for w.deleteJobButton.Clicked(gtx) {
		w.startManualJobAction("delete")
	}
	for w.exportButton.Clicked(gtx) {
		w.startExcelExport()
	}
	for w.apiTraceButton.Clicked(gtx) {
		w.startAPITrace()
	}
	for w.savePresetButton.Clicked(gtx) {
		w.saveCurrentPreset()
	}
	for w.loadPresetButton.Clicked(gtx) {
		w.loadNamedPreset()
	}
	for w.deletePresetButton.Clicked(gtx) {
		w.deleteNamedPreset()
	}
	for w.inspectJobsButton.Clicked(gtx) {
		w.inspectServerJobs()
	}
	for w.restartServerButton.Clicked(gtx) {
		w.startServerControl("restart")
	}
	for w.rebootServerButton.Clicked(gtx) {
		w.startServerControl("reboot")
	}
	for w.clearAllJobsButton.Clicked(gtx) {
		w.startClearAllJobs()
	}
	for name, button := range w.categoryButtons {
		for button.Clicked(gtx) {
			w.activeCapabilityGroup = name
			w.capabilitySearch.SetText("")
			w.list.Position = layout.Position{}
		}
	}
	for w.runButton.Clicked(gtx) {
		w.setActivePage(pageResults)
		w.startRun()
	}
	for w.browseFolderButton.Clicked(gtx) {
		if path, err := browsePath(true); err != nil {
			w.setStatus("Folder selection failed: " + err.Error())
		} else if path != "" {
			w.folderPath.SetText(path)
			w.addLog("Selected test folder: %s", path)
		}
	}
	for w.browseFileButton.Clicked(gtx) {
		if path, err := browsePath(false); err != nil {
			w.setStatus("File selection failed: " + err.Error())
		} else if path != "" {
			w.filePath.SetText(path)
			w.fileModeGroup.Value = "single"
			w.addLog("Selected test file: %s", path)
		}
	}
	for i := range w.navButtons {
		for w.navButtons[i].Clicked(gtx) {
			w.setActivePage(i)
		}
	}
	for w.cancelButton.Clicked(gtx) {
		if w.cancel != nil {
			w.cancel()
			w.addLog("Cancellation requested")
		}
	}
	for w.singleStrategyButton.Clicked(gtx) {
		w.strategy = combinations.StrategySingle
	}
	for w.allPermButton.Clicked(gtx) {
		w.strategy = combinations.StrategyAll
	}
	for w.pairwiseButton.Clicked(gtx) {
		w.strategy = combinations.StrategyPairwise
	}
	for w.randomStrategyButton.Clicked(gtx) {
		w.strategy = combinations.StrategyRandom
	}
	for w.baselineSourceButton.Clicked(gtx) {
		w.valueSource = valueSourceBaseline
	}
	for w.defaultsSourceButton.Clicked(gtx) {
		w.valueSource = valueSourceDefaults
	}
	for w.selectedSourceButton.Clicked(gtx) {
		w.valueSource = valueSourceSelected
	}
	for w.advertisedSourceButton.Clicked(gtx) {
		w.valueSource = valueSourceAdvertised
	}
	for w.positiveIntentButton.Clicked(gtx) {
		w.testIntent = testIntentPositive
	}
	for w.constraintIntentButton.Clicked(gtx) {
		w.testIntent = testIntentConstraint
	}
	for w.validationOnlyButton.Clicked(gtx) {
		w.constraintMode = constraintValidationOnly
	}
	for w.controlledApplyButton.Clicked(gtx) {
		w.constraintMode = constraintControlledApply
	}
}

func (w *Window) updateEditors(gtx layout.Context) {
	editors := []*widget.Editor{
		&w.serverIP, &w.secretKey, &w.password,
		&w.folderPath, &w.filePath, &w.workers, &w.maxCases,
		&w.copiesInput, &w.pageRangeInput, &w.jobActionID,
		&w.capabilitySearch, &w.presetName, &w.adminConfirmation,
	}
	for _, editor := range w.numericInputs {
		editors = append(editors, editor)
	}
	for _, editor := range editors {
		for {
			if _, ok := editor.Update(gtx); !ok {
				break
			}
		}
	}
}

func (w *Window) resetSelections() {
	if w.running.Load() || w.managingServer.Load() || w.inspectingJobs.Load() {
		w.setStatus("Wait for the active operation to finish before resetting selections.")
		return
	}
	for _, values := range w.selected {
		for _, selected := range values {
			selected.Value = false
		}
	}
	for _, selected := range w.groupChecks {
		selected.Value = false
	}
	for _, selected := range w.optionChecks {
		selected.Value = false
	}
	w.strategy = combinations.StrategySingle
	w.valueSource = valueSourceSelected
	w.testIntent = testIntentPositive
	w.constraintMode = constraintValidationOnly
	w.copiesInput.SetText("1")
	w.pageRangeInput.SetText("")
	for _, input := range w.numericInputs {
		input.SetText("")
	}
	w.capabilitySearch.SetText("")
	w.activeCapabilityGroup = "Job Info"
	w.workers.SetText("1")
	w.maxCases.SetText(strconv.Itoa(defaultCaseLimit))
	w.fileModeGroup.Value = "all"
	w.serverPresetGroup.Value = noServerPresetID
	for index := range w.modeChecks {
		w.modeChecks[index].Value = index == 0
	}
	w.jobActionID.SetText("")
	w.adminConfirmation.SetText("")
	w.mu.Lock()
	w.adminInventoryServer = ""
	w.adminInventoryAt = time.Time{}
	w.adminJobCount = 0
	w.mu.Unlock()
	w.setStatus("Selections reset to defaults. Server details, discovered capabilities, and file paths were preserved.")
	w.addLog("Reset capability selections, Copies, custom page range, strategy, run modes, parallel jobs, and case limit to defaults")
}

func (w *Window) setActivePage(page int) {
	if page < 0 || page >= len(workspacePages) || page == w.activePage {
		return
	}
	w.mu.Lock()
	connected := w.hasActiveServer
	w.mu.Unlock()
	if !connected && page != pageConnection {
		w.setStatus("Test the server connection and press OK before opening other pages.")
		return
	}
	w.stopOverviewHealthMonitor()
	w.activePage = page
	w.list.Position = layout.Position{}
	if page == pageOverview {
		w.startOverviewHealthMonitor()
	}
}

func (w *Window) layout(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, palette.bg)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.workflowSidebar(gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(24), Right: unit.Dp(20), Bottom: unit.Dp(20), Left: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				listStyle := material.List(w.theme, &w.list)
				listStyle.ScrollbarStyle.Track.Color = withAlpha(palette.primaryDim, 160)
				listStyle.ScrollbarStyle.Indicator.Color = withAlpha(palette.primary, 190)
				listStyle.ScrollbarStyle.Indicator.HoverColor = palette.primary
				listStyle.ScrollbarStyle.Indicator.MinorWidth = unit.Dp(8)
				listStyle.ScrollbarStyle.Indicator.CornerRadius = unit.Dp(4)
				return listStyle.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions { return w.workflowContent(gtx) })
			})
		}),
	)
}

func (w *Window) fileSelectionRadioGroup(gtx layout.Context) layout.Dimensions {
	return radioGroup(gtx, w.theme, "File selection", "Choose how files are picked for this run.", []radioOption{
		{Key: "all", Label: "All files in folder"},
		{Key: "single", Label: "Specific file only"},
		{Key: "random", Label: "Random file from folder"},
	}, &w.fileModeGroup)
}

func (w *Window) runModeRadioGroup(gtx layout.Context) layout.Dimensions {
	if len(w.modeChecks) != len(runModes) {
		w.modeChecks = make([]widget.Bool, len(runModes))
		if len(w.modeChecks) > 0 {
			w.modeChecks[0].Value = true
		}
	}
	children := []layout.FlexChild{
		layout.Rigid(label(w.theme, "Run modes", 16, palette.text).Layout),
		layout.Rigid(spacer(4)),
		layout.Rigid(label(w.theme, "Select one or more Fiery workflows to execute.", 13, palette.muted).Layout),
		layout.Rigid(spacer(10)),
	}
	for i := range runModes {
		idx := i
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				check := material.CheckBox(w.theme, &w.modeChecks[idx], runModes[idx].Label)
				check.Color = palette.text
				check.IconColor = palette.primary
				return check.Layout(gtx)
			})
		}))
	}
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

type radioOption struct{ Key, Label string }

func radioGroup(gtx layout.Context, th *material.Theme, title, subtitle string, options []radioOption, group *widget.Enum) layout.Dimensions {
	children := []layout.FlexChild{
		layout.Rigid(label(th, title, 16, palette.text).Layout),
		layout.Rigid(spacer(4)),
		layout.Rigid(label(th, subtitle, 13, palette.muted).Layout),
		layout.Rigid(spacer(10)),
	}
	for _, option := range options {
		opt := option
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Bottom: unit.Dp(6)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				radio := material.RadioButton(th, group, opt.Key, opt.Label)
				radio.Color = palette.text
				radio.IconColor = palette.primary
				return radio.Layout(gtx)
			})
		}))
	}
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (w *Window) capabilityToolbar(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Alignment: layout.End}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return fieldBox(w.theme, "Search features", "Search by feature name, API key, value, or category", &w.capabilitySearch, 620)(gtx)
		}),
		layout.Rigid(spacerX(12)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			w.mu.Lock()
			model := w.capabilities
			w.mu.Unlock()
			return label(w.theme, fmt.Sprintf("%d applicable properties · %d excluded entries · %d incompatible values removed · %d constrained", len(model.Options), len(model.ExcludedOptions), len(model.ExcludedValues), capabilities.ConstraintCount(model)), 13, palette.muted).Layout(gtx)
		}),
	)
}

func (w *Window) categoryTabs(gtx layout.Context, model capabilities.Model) layout.Dimensions {
	groups := capabilities.GroupedOptions(model)
	if len(groups) == 0 {
		return layout.Dimensions{}
	}
	available := false
	for _, group := range groups {
		if group.Name == w.activeCapabilityGroup {
			available = true
			break
		}
	}
	if !available {
		w.activeCapabilityGroup = groups[0].Name
	}
	// Keep categories in one fixed-height row. Compact, fixed-width tabs avoid
	// the previous oversized wrapping layout; narrow windows scroll horizontally.
	listStyle := material.List(w.theme, &w.categoryList)
	listStyle.ScrollbarStyle.Track.Color = withAlpha(palette.primaryDim, 120)
	listStyle.ScrollbarStyle.Indicator.Color = withAlpha(palette.primary, 180)
	return listStyle.Layout(gtx, len(groups), func(gtx layout.Context, index int) layout.Dimensions {
		group := groups[index]
		button := w.categoryButton(group.Name)
		count := w.selectedCountForGroup(group)
		caption := compactCategoryLabel(group.Name)
		if count > 0 {
			caption += fmt.Sprintf("  %d", count)
		}
		return layout.Inset{Right: unit.Dp(6), Bottom: unit.Dp(5)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			width := gtx.Dp(unit.Dp(126))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = width, width
			return toggle(w.theme, button, caption, w.activeCapabilityGroup == group.Name)(gtx)
		})
	})
}

func compactCategoryLabel(name string) string {
	switch name {
	case "Substrate / Media":
		return "Media"
	case "Installable options":
		return "Installable"
	case "Other / Advanced":
		return "More"
	default:
		return name
	}
}

func (w *Window) selectedCountForGroup(group capabilities.OptionGroup) int {
	count := 0
	for _, option := range group.Options {
		if input := w.numericInputs[option.ID]; input != nil && strings.TrimSpace(input.Text()) != "" {
			count++
		}
		for _, selected := range w.selected[option.ID] {
			if selected != nil && selected.Value {
				count++
			}
		}
	}
	return count
}

func (w *Window) categoryButton(name string) *widget.Clickable {
	if w.categoryButtons[name] == nil {
		w.categoryButtons[name] = new(widget.Clickable)
	}
	return w.categoryButtons[name]
}

func (w *Window) presetPanel(gtx layout.Context) layout.Dimensions {
	names := make([]string, 0, len(w.presetList))
	for _, preset := range w.presetList {
		names = append(names, preset.Name)
	}
	available := "No saved presets"
	if len(names) > 0 {
		available = "Saved: " + strings.Join(names, " · ")
	}
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(w.theme, "Reusable settings preset", 15, palette.text).Layout),
			layout.Rigid(spacer(3)),
			layout.Rigid(label(w.theme, "Presets save Job Property selections, numeric inputs, value source, test intent, generation, workers, and run modes. Credentials and file paths are never saved.", 12, palette.muted).Layout),
			layout.Rigid(spacer(7)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return row(gtx,
					field(w.theme, "Preset name", &w.presetName, 360),
					secondaryButton(w.theme, &w.savePresetButton, "Save"),
					secondaryButton(w.theme, &w.loadPresetButton, "Load"),
					dangerButton(w.theme, &w.deletePresetButton, "Delete"),
				)
			}),
			layout.Rigid(spacer(5)),
			layout.Rigid(label(w.theme, available, 12, palette.muted).Layout),
		)
	})
}

func (w *Window) strategySelector(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return row(gtx,
				toggle(w.theme, &w.singleStrategyButton, "Single Configuration", w.strategy == combinations.StrategySingle),
				toggle(w.theme, &w.allPermButton, "All Combinations", w.strategy == combinations.StrategyAll || w.strategy == combinations.StrategySelected),
				toggle(w.theme, &w.pairwiseButton, "Pairwise", w.strategy == combinations.StrategyPairwise),
				toggle(w.theme, &w.randomStrategyButton, "Bounded Random Sample", w.strategy == combinations.StrategyRandom),
			)
		}),
		layout.Rigid(spacer(10)),
		layout.Rigid(field(w.theme, "Max cases (1–10,000)", &w.maxCases, 180)),
	)
}

func (w *Window) optionGrid(gtx layout.Context, model capabilities.Model) layout.Dimensions {
	query := strings.TrimSpace(w.capabilitySearch.Text())
	groups := capabilities.FilteredGroups(model, query)
	if query == "" {
		filtered := groups[:0]
		for _, group := range groups {
			if group.Name == w.activeCapabilityGroup {
				filtered = append(filtered, group)
				break
			}
		}
		groups = filtered
	}
	if len(groups) == 0 {
		return formPanel(gtx, label(w.theme, "No features match the current search.", 14, palette.muted).Layout)
	}
	children := make([]layout.FlexChild, 0, len(groups)*2+1)
	if query == "" && w.activeCapabilityGroup == "Job Info" {
		children = append(children, layout.Rigid(w.queueGroup(model)), layout.Rigid(spacer(12)))
	}
	for index, group := range groups {
		g := group
		if index > 0 {
			children = append(children, layout.Rigid(spacer(12)))
		}
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.capabilityGroup(gtx, g) }))
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (w *Window) queueGroup(model capabilities.Model) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		items := []layout.FlexChild{layout.Rigid(label(w.theme, "Queues", 16, palette.text).Layout)}
		available := 0
		for _, q := range model.Queues {
			state := "unavailable"
			if q.Available {
				state = "available"
				available++
			}
			queueText := fmt.Sprintf("%s · %s", q.Name, state)
			items = append(items, layout.Rigid(label(w.theme, queueText, 13, palette.text).Layout))
		}
		if len(model.Queues) == 0 {
			items = append(items, layout.Rigid(label(w.theme, "No queue data returned by server", 13, palette.muted).Layout))
		} else {
			items = append(items, layout.Rigid(label(w.theme, fmt.Sprintf("Available queues: %d of %d", available, len(model.Queues)), 13, palette.muted).Layout))
		}
		return formPanel(gtx, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
		})
	}
}

func (w *Window) capabilityGroup(gtx layout.Context, group capabilities.OptionGroup) layout.Dimensions {
	children := []layout.FlexChild{layout.Rigid(func(gtx layout.Context) layout.Dimensions {
		return w.capabilityGroupHeader(gtx, group)
	}), layout.Rigid(spacer(8))}
	for sectionIndex, section := range group.Sections {
		if section.Name != "" {
			if sectionIndex > 0 {
				children = append(children, layout.Rigid(spacer(10)))
			}
			sectionName := section.Name
			children = append(children,
				layout.Rigid(label(w.theme, sectionName, 16, palette.primary).Layout),
				layout.Rigid(spacer(7)),
			)
		}
		for _, opt := range section.Options {
			option := opt
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.optionRow(gtx, option) }), layout.Rigid(spacer(8)))
		}
	}
	return formPanel(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (w *Window) capabilityGroupHeader(gtx layout.Context, group capabilities.OptionGroup) layout.Dimensions {
	if w.groupChecks == nil {
		w.groupChecks = make(map[string]*widget.Bool)
	}
	allSelected, selectableCount := w.groupSelectionState(group)
	if selectableCount == 0 {
		return label(w.theme, group.Name, 16, palette.text).Layout(gtx)
	}
	selector := w.headerCheckbox(w.groupChecks, group.Name)
	selector.Value = allSelected
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, label(w.theme, group.Name, 16, palette.text).Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			before := selector.Value
			control := material.CheckBox(w.theme, selector, "Select all in group")
			control.Color = palette.primary
			dimensions := control.Layout(gtx)
			if selector.Value != before {
				w.setGroupSelection(group, selector.Value)
			}
			return dimensions
		}),
	)
}

func (w *Window) groupSelectionState(group capabilities.OptionGroup) (bool, int) {
	allSelected := true
	selectableCount := 0
	for _, option := range group.Options {
		if isCopiesOption(option.ID) || option.Range != nil {
			continue
		}
		values := checkboxOptionValues(option)
		ensureBools(w.selected, option.ID, values)
		for _, value := range values {
			selectableCount++
			if !w.selected[option.ID][value].Value {
				allSelected = false
			}
		}
	}
	return allSelected && selectableCount > 0, selectableCount
}

func (w *Window) setGroupSelection(group capabilities.OptionGroup, selected bool) {
	for _, option := range group.Options {
		if isCopiesOption(option.ID) || option.Range != nil {
			continue
		}
		values := checkboxOptionValues(option)
		ensureBools(w.selected, option.ID, values)
		for _, value := range values {
			w.selected[option.ID][value].Value = selected
		}
		if w.optionChecks == nil {
			w.optionChecks = make(map[string]*widget.Bool)
		}
		w.headerCheckbox(w.optionChecks, option.ID).Value = selected && len(values) > 0
	}
}

func (w *Window) optionRow(gtx layout.Context, opt capabilities.Option) layout.Dimensions {
	if isCopiesOption(opt.ID) {
		return w.copiesOptionRow(gtx, opt)
	}
	if isPageRangeOption(opt.ID) {
		return w.pageRangeOptionRow(gtx, opt)
	}
	if opt.Range != nil {
		return w.numericOptionRow(gtx, opt)
	}
	// Render every value advertised by Fiery. The page-level material list and
	// scrollbar handle long option groups, so manual selection is never limited
	// to an arbitrary subset of the server's values.
	values := optionValues(opt)
	ensureBools(w.selected, opt.ID, values)
	defaultValue := fallback(displayOptionValue(opt.ID, opt.Value), "not reported")
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		metadata := fmt.Sprintf("%s · %d value(s) · default: %s", opt.ID, len(values), defaultValue)
		if len(opt.Constraints) > 0 {
			metadata += fmt.Sprintf(" · constraints on %d value(s)", len(opt.Constraints))
		}
		items := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.optionHeader(gtx, opt, values) }),
			layout.Rigid(spacer(3)),
			layout.Rigid(label(w.theme, metadata, 12, palette.muted).Layout),
			layout.Rigid(spacer(8)),
		}
		for _, value := range values {
			value := value
			checkbox := w.selected[opt.ID][value]
			displayValue := displayOptionValue(opt.ID, value)
			if optionValueMatches(opt.ID, value, opt.Value) {
				displayValue += "  · default"
			}
			items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					control := material.CheckBox(w.theme, checkbox, displayValue)
					control.Color = palette.primary
					return control.Layout(gtx)
				})
			}))
		}
		if len(values) == 0 {
			items = append(items, layout.Rigid(label(w.theme, "No selectable values were reported by the server.", 13, palette.muted).Layout))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
	})
}

func (w *Window) optionHeader(gtx layout.Context, option capabilities.Option, values []string) layout.Dimensions {
	if len(values) == 0 {
		return label(w.theme, option.Label, 15, palette.text).Layout(gtx)
	}
	if w.optionChecks == nil {
		w.optionChecks = make(map[string]*widget.Bool)
	}
	allSelected := true
	for _, value := range values {
		if !w.selected[option.ID][value].Value {
			allSelected = false
			break
		}
	}
	selector := w.headerCheckbox(w.optionChecks, option.ID)
	selector.Value = allSelected
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, label(w.theme, option.Label, 15, palette.text).Layout),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			before := selector.Value
			control := material.CheckBox(w.theme, selector, "Select all values")
			control.Color = palette.primary
			dimensions := control.Layout(gtx)
			if selector.Value != before {
				w.setOptionSelection(option.ID, values, selector.Value)
			}
			return dimensions
		}),
	)
}

func (w *Window) setOptionSelection(optionID string, values []string, selected bool) {
	ensureBools(w.selected, optionID, values)
	for _, value := range values {
		w.selected[optionID][value].Value = selected
	}
}

func (w *Window) headerCheckbox(store map[string]*widget.Bool, key string) *widget.Bool {
	if store[key] == nil {
		store[key] = new(widget.Bool)
	}
	return store[key]
}

func (w *Window) pageRangeOptionRow(gtx layout.Context, opt capabilities.Option) layout.Dimensions {
	values := checkboxOptionValues(opt)
	ensureBools(w.selected, opt.ID, values)
	w.mu.Lock()
	customSupported := customPageRangeSupported(w.capabilities)
	w.mu.Unlock()
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		items := []layout.FlexChild{
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.optionHeader(gtx, opt, values) }),
			layout.Rigid(spacer(3)),
			layout.Rigid(label(w.theme, fmt.Sprintf("%s · server-advertised values · default: %s", opt.ID, fallback(opt.Value, "not reported")), 12, palette.muted).Layout),
			layout.Rigid(spacer(7)),
		}
		for _, value := range values {
			value := value
			checkbox := w.selected[opt.ID][value]
			displayValue := value
			if value == opt.Value {
				displayValue += "  · default"
			}
			items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Inset{Bottom: unit.Dp(3)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					control := material.CheckBox(w.theme, checkbox, displayValue)
					control.Color = palette.primary
					return control.Layout(gtx)
				})
			}))
		}
		items = append(items, layout.Rigid(spacer(5)))
		if customSupported {
			items = append(items,
				layout.Rigid(label(w.theme, "Custom page range", 14, palette.text).Layout),
				layout.Rigid(spacer(3)),
				layout.Rigid(label(w.theme, "Enter pages like 1,3,5-8 or 5 to 8. The text is validated against each imported file's original page count and sent directly as EFPageRange. DPP_PAGE_RANGE is never sent.", 13, palette.muted).Layout),
				layout.Rigid(spacer(7)),
				layout.Rigid(fieldBox(w.theme, "Custom page range", "1,3,5-8", &w.pageRangeInput, 620)),
			)
		} else {
			items = append(items,
				layout.Rigid(label(w.theme, "Custom page range", 14, palette.text).Layout),
				layout.Rigid(spacer(3)),
				layout.Rigid(label(w.theme, "Disabled: this Fiery does not advertise a range-capable EFPageRange value. Exact advertised values remain available and the app will not send DPP_PAGE_RANGE.", 13, palette.muted).Layout),
				layout.Rigid(spacer(7)),
				layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					return fieldBox(w.theme, "Custom page range (not supported)", "Unavailable on this Fiery", &w.pageRangeInput, 620)(gtx.Disabled())
				}),
			)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
	})
}

func (w *Window) numericOptionRow(gtx layout.Context, opt capabilities.Option) layout.Dimensions {
	input := w.numericInput(opt.ID)
	rangeInfo := fmt.Sprintf("allowed: %s to %s · increment %s", formatNumber(opt.Range.Min, opt.Range.Precision), formatNumber(opt.Range.Max, opt.Range.Precision), formatNumber(opt.Range.Increment, opt.Range.Precision))
	description := "Enter one value, comma-separated values, or an inclusive range such as 5-10 or 5 to 10. Leave blank to omit this setting."
	if opt.Synthetic && (opt.ID == "Scaling" || opt.ID == "EFScale") {
		description = "Optional scale percentage. This standard job-ticket field is sent only when populated; Fiery remains authoritative and validates support for the imported job."
	}
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(w.theme, opt.Label, 15, palette.text).Layout),
			layout.Rigid(spacer(3)),
			layout.Rigid(label(w.theme, fmt.Sprintf("%s · %s · default: %s", opt.ID, rangeInfo, fallback(opt.Value, "not reported")), 12, palette.muted).Layout),
			layout.Rigid(spacer(7)),
			layout.Rigid(label(w.theme, description, 13, palette.muted).Layout),
			layout.Rigid(spacer(7)),
			layout.Rigid(fieldBox(w.theme, opt.Label, "Single value, list, or range", input, 620)),
		)
	})
}

func (w *Window) numericInput(optionID string) *widget.Editor {
	if w.numericInputs[optionID] == nil {
		editor := new(widget.Editor)
		initEditor(editor, "")
		w.numericInputs[optionID] = editor
	}
	return w.numericInputs[optionID]
}

func formatNumber(value float64, precision int) string {
	return strconv.FormatFloat(value, 'f', precision, 64)
}

func (w *Window) copiesOptionRow(gtx layout.Context, opt capabilities.Option) layout.Dimensions {
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(w.theme, opt.Label, 15, palette.text).Layout),
			layout.Rigid(spacer(3)),
			layout.Rigid(label(w.theme, fmt.Sprintf("%s · allowed range: %d-%d", opt.ID, copyvalues.Minimum, copyvalues.Maximum), 12, palette.muted).Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(label(w.theme, "Enter comma-separated copies or inclusive ranges. Examples: 1,5,10,15 · 999 · 5-10 · 5 to 10", 13, palette.muted).Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(fieldBox(w.theme, "Copies", "1,5,10,15 or 5-10", &w.copiesInput, 620)),
		)
	})
}

func isCopiesOption(optionID string) bool {
	return optionID == "num copies" || optionID == "EFCopies"
}

func isPageRangeOption(optionID string) bool {
	return strings.EqualFold(strings.TrimSpace(optionID), pageRangeOptionID)
}

func customPageRangeSupported(model capabilities.Model) bool {
	option, exists := model.OptionByID(pageRangeOptionID)
	// This Fiery's range-capable schema advertises Range1 and CWS materializes
	// arbitrary selections such as 5-10 directly in EFPageRange. Do not enable
	// free-form text on servers that advertise only All/Odd/Even semantics.
	return exists && containsStringFold(option.Values, pageRangeRangeValue)
}

func (w *Window) resultsCard(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	status := w.status
	w.mu.Unlock()
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, sectionTitle(w.theme, "Automation results")),
					layout.Rigid(secondaryButton(w.theme, &w.exportButton, "Export Excel")),
				)
			}),
			layout.Rigid(spacer(8)),
			layout.Rigid(label(w.theme, status, 14, palette.primary).Layout),
			layout.Rigid(spacer(10)),
			layout.Rigid(w.jobActionsPanel),
			layout.Rigid(spacer(12)),
			layout.Rigid(w.resultsTable),
		)
	})
}

func (w *Window) jobActionsPanel(gtx layout.Context) layout.Dimensions {
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(w.theme, "Job actions", 15, palette.text).Layout),
			layout.Rigid(spacer(3)),
			layout.Rigid(label(w.theme, "Cancel supports processing/ripping, waiting-to-print, and printing jobs. Delete permanently removes the specified job in any state.", 13, palette.muted).Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(fieldBox(w.theme, "Job ID", "Enter the exact Fiery job ID", &w.jobActionID, 620)),
			layout.Rigid(spacer(8)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return row(gtx,
					secondaryButton(w.theme, &w.cancelJobButton, "Cancel job"),
					dangerButton(w.theme, &w.deleteJobButton, "Delete job"),
				)
			}),
		)
	})
}

func (w *Window) logsCard(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	status := w.status
	w.mu.Unlock()
	diagnosticPath := "Unavailable"
	if w.diagnostic != nil && w.diagnostic.path != "" {
		diagnosticPath = w.diagnostic.path
	}
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(sectionTitle(w.theme, "Activity logs")),
			layout.Rigid(spacer(8)),
			layout.Rigid(label(w.theme, status, 14, palette.primary).Layout),
			layout.Rigid(spacer(6)),
			layout.Rigid(label(w.theme, "Complete diagnostic file: "+diagnosticPath, 13, palette.muted).Layout),
			layout.Rigid(spacer(10)),
			layout.Rigid(w.logPanel),
		)
	})
}

func (w *Window) resultsTable(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	results := append([]resultRow(nil), w.results...)
	resultCount := w.resultCount
	w.mu.Unlock()
	rows := []layout.FlexChild{layout.Rigid(label(w.theme, "Job ID              Job name                    Result  Status          State           Duration  Detail", 13, palette.muted).Layout)}
	if len(results) == 0 {
		rows = append(rows, layout.Rigid(label(w.theme, "No automation results yet.", 13, palette.text).Layout))
	}
	if len(results) > maxDisplayedResults {
		results = results[len(results)-maxDisplayedResults:]
		rows = append(rows, layout.Rigid(label(w.theme, fmt.Sprintf("Showing latest %d of %d result(s); complete activity remains in the diagnostic log.", len(results), resultCount), 13, palette.muted).Layout))
	}
	for _, r := range results {
		rr := r
		rows = append(rows, layout.Rigid(label(w.theme, fmt.Sprintf("%-19s %-27s %-7s %-15s %-15s %-9s %s", short(rr.JobID, 17), short(rr.JobName, 25), rr.Result, short(rr.Status, 13), short(rr.State, 13), rr.Duration, short(rr.Detail, 65)), 13, palette.text).Layout))
	}
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}
func (w *Window) logPanel(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	lines := append([]string(nil), w.log...)
	logCount := w.logCount
	w.mu.Unlock()
	if len(lines) > maxDisplayedLogLines {
		lines = lines[len(lines)-maxDisplayedLogLines:]
	}
	rows := make([]layout.FlexChild, 0, len(lines)+1)
	if len(lines) == 0 {
		rows = append(rows, layout.Rigid(label(w.theme, "No activity messages yet.", 13, palette.text).Layout))
	} else if logCount > len(lines) {
		rows = append(rows, layout.Rigid(label(w.theme, fmt.Sprintf("Showing latest %d of %d message(s); the diagnostic file contains the complete log.", len(lines), logCount), 13, palette.muted).Layout))
	}
	for _, l := range lines {
		line := l
		rows = append(rows, layout.Rigid(label(w.theme, line, 13, palette.text).Layout))
	}
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}

func (w *Window) testServerConnection() {
	if w.managingServer.Load() || w.inspectingJobs.Load() {
		w.setStatus("Wait for the server administration operation to finish before testing the connection.")
		return
	}
	server, ok := w.connectionDraft()
	if !ok {
		w.setServerTestStatus("Missing server details")
		return
	}
	testedKey := connectionKey(server)
	w.mu.Lock()
	w.serverTestOK = false
	w.testedConnectionKey = ""
	w.mu.Unlock()
	if !w.testingServer.CompareAndSwap(false, true) {
		return
	}
	w.setServerTestStatus("Testing...")
	w.setStatus("Testing server connection...")
	w.launchBackground("Server connection test", func() {
		defer w.testingServer.Store(false)
		ctx, cancel := context.WithTimeout(w.rootContext(), 30*time.Second)
		defer cancel()
		client, err := fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
		if err != nil {
			w.mu.Lock()
			w.serverTestOK = false
			w.testedConnectionKey = ""
			w.mu.Unlock()
			w.setServerTestStatus("Connection failed")
			w.setStatus("Server test failed: " + err.Error())
			w.addLog("Server connection test failed: %v", err)
			return
		}
		if _, err := client.Login(ctx); err != nil {
			w.mu.Lock()
			w.serverTestOK = false
			w.testedConnectionKey = ""
			w.mu.Unlock()
			w.setServerTestStatus("Authentication failed")
			w.setStatus("Server test failed: " + err.Error())
			w.addLog("Server connection test failed: %v", err)
			return
		}
		w.mu.Lock()
		w.serverTestOK = true
		w.testedConnectionKey = testedKey
		w.mu.Unlock()
		w.setServerTestStatus("Connection OK · press OK to apply")
		w.setStatus("Server connection passed. Press OK to use this connection.")
		w.addLog("Server connection test passed for %s", server.IPAddress)
	})
}

func (w *Window) startManualJobAction(action string) {
	if w.running.Load() || w.managingServer.Load() || w.inspectingJobs.Load() {
		w.setStatus("Wait for the active automation or server administration operation before managing a job.")
		return
	}
	jobID := strings.TrimSpace(w.jobActionID.Text())
	if jobID == "" {
		w.setStatus("Enter the exact Fiery job ID before using a job action.")
		return
	}
	server, ok := w.server()
	if !ok {
		return
	}
	var title, message string
	switch action {
	case "cancel":
		title = "Cancel Fiery job"
		message = fmt.Sprintf("Cancel job %s?\n\nThe application will proceed only if Fiery reports processing/ripping, waiting to print, or printing.", jobID)
	case "delete":
		title = "Permanently delete Fiery job"
		message = fmt.Sprintf("Permanently delete job %s?\n\nDelete is allowed in any job state and cannot be undone.", jobID)
	default:
		w.setStatus("Unsupported job action: " + action)
		return
	}
	confirmed, err := confirmDestructiveAction(title, message)
	if err != nil {
		w.setStatus("Could not open job action confirmation: " + err.Error())
		return
	}
	if !confirmed || !w.managingJob.CompareAndSwap(false, true) {
		return
	}
	w.setStatus(fmt.Sprintf("Preparing to %s job %s...", action, jobID))
	w.launchBackground("Fiery job "+action, func() {
		defer func() {
			w.managingJob.Store(false)
			w.invalidate()
		}()
		ctx, cancel := context.WithTimeout(w.rootContext(), 90*time.Second)
		defer cancel()
		client, err := fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
		if err != nil {
			w.finishManualJobAction(action, jobID, err)
			return
		}
		session, err := client.Login(ctx)
		if err != nil {
			w.finishManualJobAction(action, jobID, fmt.Errorf("login: %w", err))
			return
		}
		switch action {
		case "cancel":
			attributes, getErr := client.GetJobAttributes(ctx, session, jobID)
			if getErr != nil {
				w.finishManualJobAction(action, jobID, fmt.Errorf("check job state: %w", getErr))
				return
			}
			cancelable, state := cancelableJob(attributes)
			if !cancelable {
				w.finishManualJobAction(action, jobID, fmt.Errorf("job is not processing/ripping, waiting to print, or printing (reported state: %s)", state))
				return
			}
			w.addLog("Cancel job %s accepted precondition: %s", jobID, state)
			err = client.CancelJob(ctx, session, jobID)
		case "delete":
			err = client.DeleteJob(ctx, session, jobID)
		}
		w.finishManualJobAction(action, jobID, err)
	})
}

func (w *Window) finishManualJobAction(action, jobID string, err error) {
	if err != nil {
		w.setStatus(fmt.Sprintf("Could not %s job %s: %v", action, jobID, err))
		w.addLog("JOB_ACTION action=%s job_id=%s result=ERROR error=%v", action, jobID, err)
		return
	}
	w.setStatus(fmt.Sprintf("Job %s %s request completed successfully.", jobID, action))
	w.addLog("JOB_ACTION action=%s job_id=%s result=PASS", action, jobID)
}

func activelyProcessingJob(attributes map[string]string) (bool, string) {
	reported := make([]string, 0, 4)
	for key, value := range attributes {
		keyLower := strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if strings.HasPrefix(keyLower, "is ") {
			activeKey := strings.Contains(keyLower, "printing") || strings.Contains(keyLower, "processing") || strings.Contains(keyLower, "ripping")
			if activeKey && isTruthy(value) {
				return true, strings.TrimSpace(key) + "=" + value
			}
		}
		stateKey := keyLower == "status" || keyLower == "state" || keyLower == "display status" || strings.Contains(keyLower, "job status") || strings.Contains(keyLower, "current action")
		if !stateKey || value == "" {
			continue
		}
		reported = append(reported, strings.TrimSpace(key)+"="+value)
		valueLower := strings.ToLower(value)
		waitingOrTerminal := false
		for _, term := range []string{"done", "complete", "cancel", "abort", "error", "fail", "held", "queue", "wait", "ready"} {
			if strings.Contains(valueLower, term) {
				waitingOrTerminal = true
				break
			}
		}
		if waitingOrTerminal {
			continue
		}
		for _, term := range []string{"printing", "processing", "ripping"} {
			if strings.Contains(valueLower, term) {
				return true, strings.TrimSpace(key) + "=" + value
			}
		}
	}
	if len(reported) == 0 {
		return false, "unknown"
	}
	sort.Strings(reported)
	return false, strings.Join(reported, ", ")
}

func waitingToPrintJob(attributes map[string]string) (bool, string) {
	for _, key := range []string{"queued for printing?", "is committed to print?", "waiting to print?", "ready to print?"} {
		if isTruthy(attributes[key]) {
			return true, key + "=" + strings.TrimSpace(attributes[key])
		}
	}
	for _, key := range []string{"status", "state", "display status", "current action"} {
		value := strings.TrimSpace(attributes[key])
		valueLower := strings.ToLower(value)
		for _, phrase := range []string{"waiting to print", "ready to print", "queued for print", "print queue"} {
			if strings.Contains(valueLower, phrase) {
				return true, key + "=" + value
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(attributes["job release state"]), "production") {
		if active, _ := activelyProcessingJob(attributes); !active && !printCompleted(attributes) {
			return true, "job release state=production"
		}
	}
	return false, "unknown"
}

func cancelableJob(attributes map[string]string) (bool, string) {
	if active, state := activelyProcessingJob(attributes); active {
		return true, state
	}
	if waiting, state := waitingToPrintJob(attributes); waiting {
		return true, state
	}
	_, state := activelyProcessingJob(attributes)
	return false, state
}

func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func (w *Window) captureCapabilities() {
	if w.running.Load() || w.managingServer.Load() || w.inspectingJobs.Load() {
		w.setStatus("Wait for the active server operation before capturing capabilities.")
		return
	}
	server, ok := w.server()
	if !ok {
		return
	}
	if !w.running.CompareAndSwap(false, true) {
		return
	}
	ctx, cancel := context.WithCancel(w.rootContext())
	w.cancel = cancel
	w.setCaptureProgress(true, 0.05, "Preparing server capability capture...")
	w.setStatus("Getting capabilities from server...")
	w.launchBackground("Capability capture", func() {
		defer func() {
			w.running.Store(false)
			w.setCaptureProgress(false, 1, "Capability capture finished")
		}()
		w.setCaptureProgress(true, 0.15, "Creating Fiery API client...")
		client, err := fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
		if err != nil {
			w.setStatus("Server configuration invalid: " + err.Error())
			return
		}
		w.setCaptureProgress(true, 0.30, "Authenticating with Fiery server...")
		session, err := client.Login(ctx)
		if err != nil {
			w.setStatus("Login failed: " + err.Error())
			return
		}
		w.setCaptureProgress(true, 0.45, "Discovering v5/v4 server endpoints and properties...")
		snap := client.DiscoverCapabilities(ctx, session)
		if err := ctx.Err(); err != nil {
			w.setStatus("Capability capture cancelled")
			return
		}
		w.setCaptureProgress(true, 0.75, "Normalizing capabilities and running preflight checks...")
		model := capabilities.FromSnapshot(snap)
		env := preflight.Run(snap, model)
		w.setCaptureProgress(true, 0.90, "Saving capability and environment snapshots...")
		capabilityPath, capabilityErr := client.SaveCapabilitySnapshot(snap, captureDirectory())
		if capabilityErr != nil {
			w.addLog("Capability snapshot save failed: %s", capabilityErr)
		} else {
			w.addLog("Saved capability snapshot: %s", capabilityPath)
		}
		environmentPath, environmentErr := preflight.Save(env, captureDirectory())
		if environmentErr != nil {
			w.addLog("Environment snapshot save failed: %s", environmentErr)
		} else {
			w.addLog("Saved environment snapshot: %s", environmentPath)
		}
		normalizedPath, normalizedErr := capabilities.SaveNormalizationReport(model, snap.CapturedAt, captureDirectory())
		if normalizedErr != nil {
			w.addLog("Normalized capability decision report save failed: %s", normalizedErr)
		} else {
			w.addLog("Saved normalized capability decision report: %s", normalizedPath)
		}
		w.mu.Lock()
		w.capabilities = model
		w.mu.Unlock()
		w.setCaptureProgress(true, 1.0, "Capabilities loaded successfully.")
		w.setStatus("Capabilities loaded. Preflight: " + env.OverallStatus)
		w.logCapabilitySummary(model)
	})
}

func (w *Window) setCaptureProgress(active bool, progress float32, phase string) {
	if progress < 0 {
		progress = 0
	}
	if progress > 1 {
		progress = 1
	}
	w.mu.Lock()
	w.captureActive = active
	w.captureProgress = progress
	w.capturePhase = phase
	w.mu.Unlock()
	if phase != "" {
		w.diagnostic.printf("CAPTURE: %.0f%% %s", progress*100, phase)
	}
	w.invalidate()
}

func (w *Window) logCapabilitySummary(model capabilities.Model) {
	groups := capabilities.GroupedOptions(model)
	w.addLog("Discovered server %s press=%s serial=%s version=%s queues=%d server_presets=%d applicable_options=%d excluded_schema_entries=%d groups=%d", fallback(model.ServerName, "unknown"), fallback(model.PressModel, "unknown"), fallback(model.SerialNumber, "unknown"), fallback(model.Version, "unknown"), len(model.Queues), len(model.ServerPresets), len(model.Options), len(model.ExcludedOptions), len(groups))
	w.logCapabilityFilterAudit(model)
	w.logExcludedCapabilitySummary(model.ExcludedOptions, model.ExcludedValues)
	for _, group := range groups {
		keys := make([]string, 0, len(group.Options))
		for _, opt := range group.Options {
			keys = append(keys, fmt.Sprintf("%s(%d)", opt.ID, len(optionValues(opt))))
		}
		w.addLog("Capability group %s: %s", group.Name, strings.Join(keys, ", "))
	}
	if _, hasPageRange := model.OptionByID(pageRangeOptionID); hasPageRange {
		if customPageRangeSupported(model) {
			w.addLog("Custom page-range text enabled: normalized expressions are sent directly as %s; %s is never emitted", pageRangeOptionID, pageRangeLegacyDataID)
		} else {
			w.addLog("Custom page-range text disabled: %s does not advertise %s; exact advertised values remain available and %s will not be emitted", pageRangeOptionID, pageRangeRangeValue, pageRangeLegacyDataID)
		}
	}
	w.logCriticalCapabilityDiagnostics(model)
}

func (w *Window) logCapabilityFilterAudit(model capabilities.Model) {
	audit := capabilities.BuildFilterAudit(model)
	summary := audit.Summary
	w.diagnostic.printf("CAPABILITY_FILTER_AUDIT: schema=%d raw=%d included=%d included_server=%d synthetic=%d displayed=%d excluded=%d excluded_values=%d constrained=%d", capabilities.NormalizationReportSchemaVersion, summary.RawServerProperties, summary.IncludedProperties, summary.IncludedServerProperties, summary.SyntheticProperties, summary.DisplayedProperties, summary.ExcludedProperties, summary.ExcludedValues, summary.ConstrainedProperties)
	for _, decision := range audit.DisplayedOptions {
		option := decision.Property
		w.diagnostic.printf("CAPABILITY_UI_DECISION: decision=included category=%q section=%q id=%s label=%q group=%q ppdtype=%q default=%q values=%q scopes=%q synthetic=%t reason=%q", decision.Category, decision.Section, option.ID, option.Label, option.Group, option.PPDType, option.Value, option.Values, option.Scopes, option.Synthetic, decision.InclusionReason)
	}
	for _, excluded := range model.ExcludedOptions {
		option := excluded.Property
		availability := "unspecified"
		if option.Available != nil {
			availability = strconv.FormatBool(*option.Available)
		}
		w.diagnostic.printf("CAPABILITY_UI_DECISION: decision=excluded id=%s label=%q group=%q ppdtype=%q default=%q values=%q scopes=%q editable=%t editableSpecified=%t available=%s hidden=%t reason=%q", excluded.ID, option.Label, option.Group, option.PPDType, option.Value, option.Values, option.Scopes, option.Editable, option.EditableSpecified, availability, option.Hidden, excluded.Reason)
	}
	for _, excluded := range model.ExcludedValues {
		w.diagnostic.printf("CAPABILITY_VALUE_DECISION: decision=excluded id=%s value=%q reason=%q", excluded.OptionID, excluded.Value, excluded.Reason)
	}
}

func (w *Window) logExcludedCapabilitySummary(excluded []capabilities.ExcludedOption, excludedValues []capabilities.ExcludedValue) {
	if len(excluded) == 0 && len(excludedValues) == 0 {
		return
	}
	byReason := make(map[string][]string)
	for _, option := range excluded {
		byReason[option.Reason] = append(byReason[option.Reason], option.ID)
	}
	reasons := make([]string, 0, len(byReason))
	for reason := range byReason {
		reasons = append(reasons, reason)
	}
	sort.Strings(reasons)
	for _, reason := range reasons {
		ids := byReason[reason]
		sort.Strings(ids)
		displayed := ids
		if len(displayed) > 24 {
			displayed = append(append([]string(nil), displayed[:24]...), fmt.Sprintf("… and %d more", len(ids)-24))
		}
		w.addLog("Excluded %d non-applicable property schema item(s): %s · IDs: %s", len(ids), reason, strings.Join(displayed, ", "))
	}
	byOption := make(map[string][]string)
	for _, value := range excludedValues {
		byOption[value.OptionID] = append(byOption[value.OptionID], value.Value)
	}
	optionIDs := make([]string, 0, len(byOption))
	for optionID := range byOption {
		optionIDs = append(optionIDs, optionID)
	}
	sort.Strings(optionIDs)
	for _, optionID := range optionIDs {
		values := byOption[optionID]
		sort.Strings(values)
		w.addLog("Removed %d value(s) from %s because they conflict with the installed server configuration: %s", len(values), optionID, strings.Join(values, ", "))
	}
}

func (w *Window) logCriticalCapabilityDiagnostics(model capabilities.Model) {
	for _, optionID := range []string{outputProfileOptionID, pageRangeOptionID, pageRangeLegacyDataID} {
		if option, ok := model.OptionByID(optionID); ok {
			quotedValues := make([]string, 0, len(option.Values))
			for _, value := range option.Values {
				quotedValues = append(quotedValues, strconv.QuoteToASCII(value))
			}
			w.diagnostic.printf("CAPABILITY_DECISION: id=%s decision=included group=%q ppdtype=%q editable=%t editableSpecified=%t hidden=%t scopes=%q default=%s values=[%s]", option.ID, option.Group, option.PPDType, option.Editable, option.EditableSpecified, option.Hidden, option.Scopes, strconv.QuoteToASCII(option.Value), strings.Join(quotedValues, ","))
			continue
		}
		excluded := false
		for _, decision := range model.ExcludedOptions {
			if !strings.EqualFold(decision.ID, optionID) {
				continue
			}
			excluded = true
			property := decision.Property
			w.diagnostic.printf("CAPABILITY_DECISION: id=%s decision=excluded reason=%q group=%q ppdtype=%q editable=%t editableSpecified=%t hidden=%t scopes=%q default=%s", decision.ID, decision.Reason, property.Group, property.PPDType, property.Editable, property.EditableSpecified, property.Hidden, property.Scopes, strconv.QuoteToASCII(property.Value))
			break
		}
		if !excluded {
			w.diagnostic.printf("CAPABILITY_DECISION: id=%s decision=not-advertised", optionID)
		}
	}
}

func (w *Window) logCriticalAttributeWire(jobID string, attributes map[string]string) {
	if value, exists := attributes[pageRangeOptionID]; exists {
		_, parseErr := pagevalues.Parse(value, pagevalues.DefaultExpansionLimit)
		_, legacyPresent := attributes[pageRangeLegacyDataID]
		w.diagnostic.printf("PAGE_RANGE_WIRE: job=%s carrier=%s custom=%t legacy_companion=%s present=%t", jobID, pageRangeOptionID, parseErr == nil, pageRangeLegacyDataID, legacyPresent)
	}
	for _, optionID := range []string{outputProfileOptionID, pageRangeOptionID, pageRangeLegacyDataID} {
		value, exists := attributes[optionID]
		if !exists {
			continue
		}
		leading := "none"
		for _, r := range value {
			leading = fmt.Sprintf("U+%04X", r)
			break
		}
		prefix := []byte(value)
		if len(prefix) > 12 {
			prefix = prefix[:12]
		}
		w.diagnostic.printf("ATTRIBUTE_WIRE: job=%s key=%s leading=%s utf8Prefix=% X bytes=%d value=%s", jobID, optionID, leading, prefix, len([]byte(value)), strconv.QuoteToASCII(value))
	}
}

func (w *Window) startAPITrace() {
	if w.running.Load() || w.managingServer.Load() || w.inspectingJobs.Load() {
		w.setStatus("Wait for the active server operation before capturing an API trace.")
		return
	}
	server, ok := w.server()
	if !ok {
		return
	}
	selectedFiles, err := files.Select(model.TestFileSelection{FolderPath: strings.TrimSpace(w.folderPath.Text()), FilePath: strings.TrimSpace(w.filePath.Text()), Mode: w.fileMode()})
	if err != nil {
		w.setStatus("API trace file selection failed: " + err.Error())
		return
	}
	if len(selectedFiles) == 0 {
		w.setStatus("API trace requires at least one supported test file.")
		return
	}
	combos, _, err := w.selectedCombinations()
	if err != nil {
		w.setStatus("API trace feature input is invalid: " + err.Error())
		return
	}
	if len(combos) == 0 || len(combos[0]) == 0 {
		w.setStatus("Select at least one capability value before capturing an API trace.")
		return
	}
	attrs := combinationToAttributes(combos[0])
	w.mu.Lock()
	capabilityModel := w.capabilities
	w.mu.Unlock()
	serverPreset, err := w.selectedServerPreset(capabilityModel)
	if err != nil {
		w.setStatus("API trace server preset selection is invalid: " + err.Error())
		return
	}
	mode := runModes[0]
	if modes := w.selectedRunModes(); len(modes) > 0 {
		mode = modes[0]
	}
	if !w.running.CompareAndSwap(false, true) {
		return
	}
	ctx, cancel := context.WithCancel(w.rootContext())
	w.cancel = cancel
	w.setStatus("Capturing controlled API trace...")
	w.launchBackground("API trace", func() {
		defer func() {
			w.running.Store(false)
			w.invalidate()
		}()
		report := w.runAPITrace(ctx, server, selectedFiles[0], attrs, mode, serverPreset)
		path, saveErr := saveAPITrace(report)
		if saveErr != nil {
			w.setStatus("API trace save failed: " + saveErr.Error())
			return
		}
		w.addLog("Saved API trace: %s", path)
		if report.Error != "" {
			w.setStatus("API trace completed with error. See " + path)
			return
		}
		w.setStatus("API trace completed: " + report.Result + ". See " + path)
	})
}

func (w *Window) runAPITrace(ctx context.Context, server model.ServerConnection, file string, attrs map[string]string, mode runMode, serverPreset *fiery.ServerPreset) apiTraceReport {
	report := apiTraceReport{
		CapturedAt:     time.Now().Format(time.RFC3339Nano),
		Server:         server.IPAddress,
		File:           filepath.Base(file),
		Mode:           mode.Label,
		Attributes:     attrs,
		UpdateProtocol: "POST /live/api/v4/jobs/{id}; POST /live/api/v5/jobs/{id} fallback",
		Result:         "ERROR",
	}
	if serverPreset != nil {
		report.ServerPresetID = serverPreset.ID
		report.ServerPresetName = serverPreset.Name
	}
	client, err := fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
	if err != nil {
		report.Error = err.Error()
		return report
	}
	session, err := client.Login(ctx)
	if err != nil {
		report.Error = err.Error()
		return report
	}
	report.SessionLogin = session.LoginPath
	imp, err := client.ImportJobToQueue(ctx, session, file, "hold")
	report.Import = imp
	report.JobID = imp.JobID
	if err != nil {
		report.Error = err.Error()
		return report
	}
	capture := func(name string) {
		report.Stages = append(report.Stages, apiTraceStage{Name: name, Captured: time.Now().Format(time.RFC3339Nano), Responses: client.GetRawJobResponses(ctx, session, imp.JobID)})
	}
	capture("after import")
	spooled, err := w.waitJobCondition(ctx, client, session, imp.JobID, "done spooling before diagnostic update", 4*time.Minute, time.Second, statusEquals("done spooling"))
	if err != nil {
		report.Error = err.Error()
		capture("spooling wait failed")
		return report
	}
	capture("done spooling before update")
	if err := validateCustomPageRange(attrs, spooled); err != nil {
		report.Result = "FAIL"
		report.Lifecycle = "Custom page range validation failed: " + err.Error()
		capture("page range validation failed")
		return report
	}
	if serverPreset != nil {
		if err := client.ApplyServerPreset(ctx, session, imp.JobID, serverPreset.ID); err != nil {
			report.Error = err.Error()
			capture("server preset application failed")
			return report
		}
		capture("after server preset application")
	}
	w.mu.Lock()
	capabilityModel := w.capabilities
	w.mu.Unlock()
	if capabilities.NeedsConstraintCheck(capabilityModel, attrs) {
		constraintCheck, constraintErr := client.CheckJobConstraints(ctx, session, imp.JobID, attrs)
		if constraintErr != nil {
			report.Error = "constraint validation: " + constraintErr.Error()
			capture("constraint check failed")
			return report
		}
		if constraintCheck.HasConflicts() {
			report.Result = "FAIL"
			report.Lifecycle = "Fiery constraint conflict: " + formatStringMap(constraintCheck.Conflicts)
			capture("constraint conflict")
			return report
		}
	}
	w.logCriticalAttributeWire(imp.JobID, attrs)
	if err := client.UpdateJobAttributes(ctx, session, imp.JobID, attrs); err != nil {
		report.Error = err.Error()
		capture("update failed")
		return report
	}
	capture("immediately after update")
	if err := w.performModeLifecycle(ctx, client, session, imp.JobID, mode); err != nil {
		var failed *lifecycleFailure
		if errors.As(err, &failed) {
			report.Final = failed.attrs
			report.Lifecycle = failed.outcome.Summary()
			report.Result = "FAIL"
			capture("lifecycle failed")
			return report
		}
		report.Error = err.Error()
		capture("lifecycle failed")
		return report
	}
	if modeIncludesAction(mode, "rip") {
		capture("done ripping")
	} else {
		capture("after " + strings.ToLower(mode.Label) + " lifecycle")
	}
	final, err := w.readBackAttributes(ctx, client, session, imp.JobID, attrs)
	report.Final = final
	if err != nil {
		report.Error = err.Error()
		capture("final readback failed")
		return report
	}
	capture("final verification")
	outcome := joboutcome.Evaluate(final, lifecyclePolicy(mode))
	report.Lifecycle = outcome.Summary()
	if outcome.Pass && w.attributesMatch(final, attrs) {
		report.Result = "PASS"
	} else {
		report.Result = "FAIL"
	}
	return report
}

func saveAPITrace(report apiTraceReport) (string, error) {
	dir := captureDirectory()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	body, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return "", err
	}
	path := filepath.Join(dir, "api-trace-"+time.Now().Format("20060102-150405")+".json")
	if err := os.WriteFile(path, body, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func (w *Window) startRun() {
	if w.running.Load() || w.managingServer.Load() || w.inspectingJobs.Load() {
		w.setStatus("Wait for the active server operation before starting automation.")
		return
	}
	server, ok := w.server()
	if !ok {
		return
	}
	selectedFiles, err := files.Select(model.TestFileSelection{FolderPath: strings.TrimSpace(w.folderPath.Text()), FilePath: strings.TrimSpace(w.filePath.Text()), Mode: w.fileMode()})
	if err != nil {
		w.setStatus("File selection failed: " + err.Error())
		return
	}
	workers := parseWorkerCount(w.workers.Text())
	combos, axes, err := w.selectedCombinations()
	if err != nil {
		w.setStatus("Feature input is invalid: " + err.Error())
		return
	}
	w.mu.Lock()
	capabilityModel := w.capabilities
	w.mu.Unlock()
	serverPreset, err := w.selectedServerPreset(capabilityModel)
	if err != nil {
		w.setStatus("Server preset selection is invalid: " + err.Error())
		return
	}
	intent := w.testIntent
	if intent == "" {
		intent = testIntentPositive
	}
	constraintMode := w.constraintMode
	if constraintMode == "" {
		constraintMode = constraintValidationOnly
	}
	if w.valueSource == valueSourceBaseline || (intent == testIntentConstraint && constraintMode == constraintValidationOnly) {
		serverPreset = nil
	}
	w.logSelectedCombinations(combos, axes)
	modes := w.selectedRunModes()
	if len(modes) == 0 {
		w.setStatus("Select at least one run mode.")
		return
	}
	if intent == testIntentConstraint {
		// Constraint verdicts are evaluated on a dedicated disposable held job;
		// lifecycle modes must not multiply or process intentionally invalid cases.
		modes = []runMode{runModes[0]}
	}
	if intent == testIntentPositive && combinationsRequireRipReadback(combos) && !runModesIncludeAction(modes, "rip") {
		w.setStatus("Selected capabilities require RIP before strict verification. Select Process and Hold or RIP run mode.")
		return
	}
	plannedTests := plannedTestCount(len(selectedFiles), len(combos), len(modes))
	if plannedTests == 0 {
		w.setStatus("No executable tests were generated from the selected files, values, and run modes.")
		return
	}
	requestedWorkers := workers
	workers = effectiveWorkerCount(workers, plannedTests)
	w.addLog("Selected run modes: %s", formatRunModes(modes))
	w.addLog("Selected Fiery server preset: %s", serverPresetDescription(serverPreset))
	w.addLog("Parallel jobs requested=%d effective=%d planned_tests=%d", requestedWorkers, workers, plannedTests)
	if !w.running.CompareAndSwap(false, true) {
		return
	}
	startedAt := time.Now()
	resultStore, err := reportxlsx.NewResultStore(diagnosticsDirectory(), startedAt)
	if err != nil {
		w.running.Store(false)
		w.setStatus("Unable to initialize Excel result data: " + err.Error())
		return
	}
	w.mu.Lock()
	previousStore := w.resultStore
	w.resultStore = resultStore
	w.resultStoreError = ""
	w.lastRun = reportxlsx.Summary{
		StartedAt:         startedAt,
		Status:            "Running",
		ServerIP:          server.IPAddress,
		ServerName:        capabilityModel.ServerName,
		SerialNumber:      capabilityModel.SerialNumber,
		ServerVersion:     capabilityModel.Version,
		QueuesDiscovered:  len(capabilityModel.Queues),
		OptionsDiscovered: len(capabilityModel.Options),
		TestFileCount:     len(selectedFiles),
		CombinationCount:  len(combos),
		ConstraintSkipped: w.constraintSkipped,
		PlannedTests:      plannedTests,
		Workers:           workers,
		Strategy:          fmt.Sprintf("%s · %s · %s", strategyLabel(w.strategy), valueSourceLabel(w.valueSource), testIntentLabel(intent)),
		ServerPreset:      serverPresetDescription(serverPreset),
		RunModes:          runModeLabels(modes),
	}
	w.results = nil
	w.resultCount = 0
	w.passedCount = 0
	w.failedCount = 0
	w.errorCount = 0
	w.mu.Unlock()
	if previousStore != nil {
		if err := previousStore.Close(); err != nil {
			w.diagnostic.printf("RESULT_STORE: close previous store error=%v", err)
		}
	}
	ctx, cancel := context.WithCancel(w.rootContext())
	w.cancel = cancel
	w.setStatus("Running automation...")
	w.launchBackground("Automation run", func() {
		w.runAutomation(ctx, server, selectedFiles, workers, combos, modes, serverPreset, intent, constraintMode)
	})
}

func (w *Window) runAutomation(ctx context.Context, server model.ServerConnection, selectedFiles []string, workers int, combos []combinations.Combination, modes []runMode, serverPreset *fiery.ServerPreset, intent automationTestIntent, constraintMode constraintTestMode) {
	finalStatus := "Failed"
	defer func() {
		finalizeErr := w.finishRun(finalStatus)
		w.running.Store(false)
		switch {
		case finalizeErr != nil:
			w.setStatus("Run ended, but Excel result data could not be finalized: " + finalizeErr.Error())
		case finalStatus == "Completed":
			w.setStatus("Completed. See Results or Activity logs.")
		case finalStatus == "Cancelled":
			w.setStatus("Cancelled")
		default:
			w.invalidate()
		}
	}()
	client, err := fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
	if err != nil {
		w.setStatus("Server configuration invalid: " + err.Error())
		return
	}
	session, err := client.Login(ctx)
	if err != nil {
		w.setStatus("Login failed: " + err.Error())
		return
	}
	w.mu.Lock()
	w.lastRun.SessionLoginPath = session.LoginPath
	w.mu.Unlock()
	jobs := make(chan struct {
		file  string
		attrs map[string]string
		mode  runMode
	})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				w.executeJobSafely(ctx, client, session, job.file, job.attrs, job.mode, serverPreset, intent, constraintMode)
			}
		}()
	}
	for _, f := range selectedFiles {
		for _, c := range combos {
			for _, mode := range modes {
				select {
				case <-ctx.Done():
					close(jobs)
					wg.Wait()
					finalStatus = "Cancelled"
					w.setStatus("Cancelling and finalizing automation results...")
					return
				case jobs <- struct {
					file  string
					attrs map[string]string
					mode  runMode
				}{f, combinationToAttributes(c), mode}:
				}
			}
		}
	}
	close(jobs)
	wg.Wait()
	if ctx.Err() != nil {
		finalStatus = "Cancelled"
		w.setStatus("Cancelling and finalizing automation results...")
		return
	}
	finalStatus = "Completed"
	w.setStatus("Finalizing automation results...")
}

func (w *Window) executeJobSafely(ctx context.Context, client *fiery.Client, session fiery.Session, file string, attrs map[string]string, mode runMode, serverPreset *fiery.ServerPreset, intent automationTestIntent, constraintMode constraintTestMode) {
	result := reportxlsx.Result{JobName: filepath.Base(file), Mode: mode.Label, SetValues: cloneStringMap(attrs)}
	defer func() {
		if recovered := recover(); recovered != nil {
			stack := debug.Stack()
			_ = writeCrashReport(fmt.Sprintf("panic while processing %s in mode %s: %v", filepath.Base(file), mode.Label, recovered), stack)
			w.diagnostic.printf("PANIC: file=%s mode=%s value=%v stack=%s", filepath.Base(file), mode.Label, recovered, stack)
			result.Result = "ERROR"
			result.Detail = fmt.Sprintf("mode=%s: unexpected internal error; see logs/crash.log", mode.Label)
			w.addResult(result)
		}
	}()
	w.executeJob(ctx, client, session, file, attrs, mode, serverPreset, intent, constraintMode, &result)
}

func (w *Window) executeJob(ctx context.Context, client *fiery.Client, session fiery.Session, file string, attrs map[string]string, mode runMode, serverPreset *fiery.ServerPreset, intent automationTestIntent, constraintMode constraintTestMode, result *reportxlsx.Result) {
	start := time.Now()
	finish := func(status, detail string, got map[string]string) {
		result.Result = status
		result.DurationMS = time.Since(start).Milliseconds()
		if serverPreset != nil {
			detail += "; server preset=" + serverPresetDescription(serverPreset)
		}
		result.Detail = detail
		result.JobName = jobNameFromAttributes(got, result.JobName)
		result.GetValues = selectedReadbackValues(got, attrs)
		result.JobStatus = got["status"]
		result.JobState = got["state"]
		result.JobError = firstNonEmpty(got["error"], got["pdl error"])
		result.LastEvent = got["last joblog event"]
		w.addResult(*result)
	}

	imp, err := client.ImportJobToQueue(ctx, session, file, mode.ImportQueue)
	if err != nil {
		finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), nil)
		return
	}
	result.JobID = imp.JobID
	w.addLog("Imported %s as job %s into queue %s for mode %s", filepath.Base(file), imp.JobID, mode.ImportQueue, mode.Label)
	if err := w.confirmImport(ctx, client, session, imp.JobID); err != nil {
		finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), nil)
		return
	}
	// A visible job can still be spooling. Wait for the imported ticket to be
	// stable before any update or lifecycle decision so Hold-only runs do not
	// report a premature pass and ticket updates are not overwritten.
	w.addLog("Waiting for job %s status=done spooling", imp.JobID)
	spooled, err := w.waitJobCondition(ctx, client, session, imp.JobID, "done spooling after import", 4*time.Minute, time.Second, func(attributes map[string]string) bool {
		return statusEquals("done spooling")(attributes) || !joboutcome.Evaluate(attributes, joboutcome.Policy{}).Pass
	})
	if err != nil {
		finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), spooled)
		return
	}
	if outcome := joboutcome.Evaluate(spooled, joboutcome.Policy{}); !outcome.Pass {
		result.Lifecycle = outcome.Summary()
		finish("FAIL", fmt.Sprintf("mode=%s: job failed while spooling: %s", mode.Label, outcome.Summary()), spooled)
		return
	}
	if err := validateCustomPageRange(attrs, spooled); err != nil {
		finish("FAIL", fmt.Sprintf("mode=%s: custom page range is invalid for %s: %v", mode.Label, filepath.Base(file), err), spooled)
		return
	}
	if serverPreset != nil {
		w.addLog("Applying Fiery server preset %s to job %s", serverPresetDescription(serverPreset), imp.JobID)
		if err := client.ApplyServerPreset(ctx, session, imp.JobID, serverPreset.ID); err != nil {
			finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), spooled)
			return
		}
		w.addLog("Fiery server preset %s was accepted for job %s", serverPresetDescription(serverPreset), imp.JobID)
	}
	if intent == testIntentConstraint {
		status, detail, got := w.executeConstraintCase(ctx, client, session, imp.JobID, attrs, constraintMode, spooled)
		result.Lifecycle = detail
		finish(status, detail, got)
		return
	}
	if len(attrs) > 0 {
		w.mu.Lock()
		capabilityModel := w.capabilities
		w.mu.Unlock()
		if capabilities.NeedsConstraintCheck(capabilityModel, attrs) {
			constraintCheck, constraintErr := client.CheckJobConstraints(ctx, session, imp.JobID, attrs)
			if constraintErr != nil {
				finish("ERROR", fmt.Sprintf("mode=%s: job constraint validation failed: %v", mode.Label, constraintErr), nil)
				return
			}
			if constraintCheck.HasConflicts() {
				got, _ := client.GetJobAttributes(ctx, session, imp.JobID)
				result.Lifecycle = "Fiery constraint conflict: " + formatStringMap(constraintCheck.Conflicts)
				finish("FAIL", fmt.Sprintf("mode=%s: Fiery rejected the selected settings as constrained: %s; solutions=%v", mode.Label, formatStringMap(constraintCheck.Conflicts), constraintCheck.Solutions), got)
				return
			}
			if constraintCheck.Supported {
				w.addLog("Fiery job constraint check passed for job %s", imp.JobID)
			} else if constraintCheck.Warning != "" {
				w.addLog("Fiery job constraint endpoint unavailable for job %s; update response remains authoritative: %s", imp.JobID, short(constraintCheck.Warning, 300))
			}
		}
		w.addLog("Setting job %s attributes after done spooling: %s", imp.JobID, formatAttributes(attrs))
		w.logCriticalAttributeWire(imp.JobID, attrs)
		if err := client.UpdateJobAttributes(ctx, session, imp.JobID, attrs); err != nil {
			finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), nil)
			return
		}
		if err := w.confirmAttributeUpdate(ctx, client, session, imp.JobID, attrs, mode); err != nil {
			finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), nil)
			return
		}
	}
	if modeIncludesAction(mode, "delete") {
		got, readErr := w.readBackAttributes(ctx, client, session, imp.JobID, attrs)
		deleteErr := client.DeleteJob(ctx, session, imp.JobID)
		if deleteErr != nil {
			finish("ERROR", fmt.Sprintf("mode=%s: delete failed: %v", mode.Label, deleteErr), got)
			return
		}
		w.addLog("Deleted job %s successfully for Delete mode", imp.JobID)
		if readErr != nil {
			finish("ERROR", fmt.Sprintf("mode=%s: job deleted, but pre-delete readback failed: %v", mode.Label, readErr), got)
			return
		}
		for key, want := range attrs {
			if !w.expectedAttributeMatches(got, attrs, key, want) {
				finish("FAIL", fmt.Sprintf("mode=%s: job deleted, but pre-delete verification failed for %s set=%q got=%q", mode.Label, key, want, got[key]), got)
				return
			}
		}
		finish("PASS", fmt.Sprintf("mode=%s: job was deleted successfully using its dedicated test job", mode.Label), got)
		return
	}
	if err := w.performModeLifecycle(ctx, client, session, imp.JobID, mode); err != nil {
		var failed *lifecycleFailure
		if errors.As(err, &failed) {
			result.Lifecycle = failed.outcome.Summary()
			finish("FAIL", fmt.Sprintf("mode=%s: lifecycle verification failed: %s", mode.Label, failed.outcome.Summary()), failed.attrs)
		} else {
			finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), nil)
		}
		return
	}
	got, err := w.readBackAttributes(ctx, client, session, imp.JobID, attrs)
	if err != nil {
		finish("ERROR", fmt.Sprintf("mode=%s: %v", mode.Label, err), got)
		return
	}
	outcome := joboutcome.Evaluate(got, lifecyclePolicy(mode))
	result.Lifecycle = outcome.Summary()
	if !outcome.Pass {
		finish("FAIL", fmt.Sprintf("mode=%s: lifecycle verification failed: %s", mode.Label, outcome.Summary()), got)
		return
	}
	status := "PASS"
	detail := fmt.Sprintf("mode=%s: lifecycle passed (%s); set values matched get values", mode.Label, outcome.Summary())
	if len(attrs) == 0 {
		detail = fmt.Sprintf("mode=%s: lifecycle passed (%s); no job attributes were selected for set/get verification", mode.Label, outcome.Summary())
	}
	for k, v := range attrs {
		if !w.expectedAttributeMatches(got, attrs, k, v) {
			status = "FAIL"
			detail = fmt.Sprintf("mode=%s: %s set=%q got=%q status=%q state=%q display=%q recent=%q related=%s availableKeys=%s", mode.Label, k, v, got[k], got["status"], got["state"], got["display status"], got["recent action"], relatedReadbackValues(got), short(strings.Join(sortedKeys(got), ","), 220))
			if requiresRipReadback(k) && !modeIncludesAction(mode, "rip") {
				detail += "; note=this attribute may require RIP for strict verification"
			}
			w.logRawPostmanComparison(ctx, client, session, imp.JobID)
			break
		}
	}
	finish(status, detail, got)
}

func (w *Window) confirmImport(ctx context.Context, client *fiery.Client, session fiery.Session, jobID string) error {
	w.addLog("Confirming job %s is visible after import", jobID)
	_, err := w.waitJobCondition(ctx, client, session, jobID, "job visible after import", 2*time.Minute, 1*time.Second, func(attrs map[string]string) bool {
		return strings.TrimSpace(attrs["id"]) == jobID || strings.TrimSpace(attrs["status"]) != "" || strings.TrimSpace(attrs["state"]) != ""
	})
	return err
}

func (w *Window) confirmAttributeUpdate(ctx context.Context, client *fiery.Client, session fiery.Session, jobID string, expected map[string]string, mode runMode) error {
	if len(expected) == 0 {
		return nil
	}
	// Attribute POST success means the server accepted the update request. Some
	// Fiery attributes are not readable immediately, so do not fail the run here;
	// final strict set/get verification runs after the selected lifecycle actions
	// complete.
	if modeIncludesAction(mode, "rip") {
		w.addLog("Attribute update accepted for job %s; final set/get verification will run after RIP", jobID)
	} else {
		w.addLog("Attribute update accepted for job %s; final set/get verification will run after mode %s", jobID, mode.Label)
	}
	return nil
}

func (w *Window) readBackAttributes(ctx context.Context, client *fiery.Client, session fiery.Session, jobID string, expected map[string]string) (map[string]string, error) {
	if len(expected) == 0 {
		return client.GetJobAttributes(ctx, session, jobID)
	}
	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var got map[string]string
	var err error
	for {
		gotAttempt, getErr := client.GetJobAttributes(ctx, session, jobID)
		err = getErr
		if getErr != nil {
			w.diagnostic.printf("READBACK: job=%s attempt=error error=%v", jobID, getErr)
		} else {
			got = gotAttempt
			w.logAttributeReadback(jobID, got, expected)
			if w.attributesMatch(got, expected) {
				return got, nil
			}
		}
		select {
		case <-ctx.Done():
			return got, ctx.Err()
		case <-deadline.C:
			if got == nil && err != nil {
				return nil, fmt.Errorf("read back job %s attributes: %w", jobID, err)
			}
			return got, nil
		case <-ticker.C:
		}
	}
}

func (w *Window) logAttributeReadback(jobID string, got, expected map[string]string) {
	payload := struct {
		JobID         string            `json:"jobId"`
		Expected      map[string]string `json:"expected"`
		Matched       bool              `json:"matched"`
		Status        string            `json:"status"`
		State         string            `json:"state"`
		DisplayStatus string            `json:"displayStatus"`
		RecentAction  string            `json:"recentAction"`
		Related       map[string]string `json:"related"`
		AvailableKeys []string          `json:"availableKeys"`
		AllAttributes map[string]string `json:"allAttributes"`
	}{
		JobID:         jobID,
		Expected:      expected,
		Matched:       w.attributesMatch(got, expected),
		Status:        got["status"],
		State:         got["state"],
		DisplayStatus: got["display status"],
		RecentAction:  got["recent action"],
		Related:       relatedReadbackMap(got),
		AvailableKeys: sortedKeys(got),
		AllAttributes: got,
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		w.diagnostic.printf("READBACK: job=%s marshal error=%v", jobID, err)
		return
	}
	w.diagnostic.printf("READBACK: %s", string(encoded))
}

func (w *Window) logRawPostmanComparison(ctx context.Context, client *fiery.Client, session fiery.Session, jobID string) {
	for _, response := range client.GetRawJobResponses(ctx, session, jobID) {
		w.diagnostic.printf("POSTMAN_COMPARE: method=%s endpoint=%s proto=%s status=%d accept=*/* login=%s body=%s", response.Method, response.Endpoint, response.ResponseProto, response.StatusCode, session.LoginPath, response.Body)
	}
}

func validateCustomPageRange(attributes, jobAttributes map[string]string) error {
	expression := strings.TrimSpace(attributes[pageRangeOptionID])
	selection, err := pagevalues.Parse(expression, pagevalues.DefaultExpansionLimit)
	if err != nil {
		// All, Odd, Even, Range1, and any other exact server-advertised menu
		// values are not arbitrary page expressions and need no page-count check.
		return nil
	}
	pageCount, ok := importedFilePageCount(jobAttributes)
	if !ok {
		return fmt.Errorf("fiery did not report the imported file's original page count after spooling")
	}
	return selection.ValidatePageCount(pageCount)
}

func importedFilePageCount(attributes map[string]string) (int, bool) {
	for _, key := range []string{"OrigPageCount", "num document pages", "pqm num pages", "PGM num pages", "num pages"} {
		value, err := strconv.Atoi(strings.TrimSpace(attributes[key]))
		if err == nil && value > 0 {
			return value, true
		}
	}
	return 0, false
}

func (w *Window) attributesMatch(got, expected map[string]string) bool {
	for key, want := range expected {
		if !w.expectedAttributeMatches(got, expected, key, want) {
			return false
		}
	}
	return true
}

func (w *Window) expectedAttributeMatches(got, _ map[string]string, key, want string) bool {
	return w.attributeMapValueMatches(got, key, want)
}

func (w *Window) attributeMapValueMatches(got map[string]string, key, want string) bool {
	if strings.EqualFold(key, pageRangeOptionID) {
		if _, err := pagevalues.Parse(want, pagevalues.DefaultExpansionLimit); err == nil {
			return pageRangeValueMatches(got, want)
		}
	}
	return w.attributeValueMatches(key, got[key], want)
}

func pageRangeValueMatches(got map[string]string, want string) bool {
	value := strings.TrimSpace(got[pageRangeOptionID])
	if _, err := pagevalues.Parse(value, pagevalues.DefaultExpansionLimit); err != nil {
		return false
	}
	return pagevalues.Equivalent(value, want)
}

func (w *Window) attributeValueMatches(key, got, want string) bool {
	if strings.EqualFold(key, outputProfileOptionID) {
		got = normalizeOutputProfileValue(got)
		want = normalizeOutputProfileValue(want)
	}
	if got == want {
		return true
	}
	// Fiery often omits job attributes whose value is the server default. Treat a
	// missing/empty readback as a match only when the selected value equals the
	// discovered default for that option.
	if strings.TrimSpace(got) != "" {
		return false
	}
	w.mu.Lock()
	model := w.capabilities
	w.mu.Unlock()
	option, ok := model.OptionByID(key)
	return ok && option.Value == want
}

func (w *Window) performModeLifecycle(ctx context.Context, client *fiery.Client, session fiery.Session, jobID string, mode runMode) error {
	for _, action := range mode.Actions {
		switch action {
		case "rip":
			// executeJob and the API-trace workflow both stabilize the imported
			// ticket at done spooling before entering lifecycle actions. Avoid a
			// second six-endpoint readback here, especially at high concurrency.
			w.addLog("Running RIP for job %s", jobID)
			if err := client.JobAction(ctx, session, jobID, "rip"); err != nil {
				return err
			}
			w.addLog("Waiting for job %s status=done ripping or a terminal Fiery failure after RIP", jobID)
			observed, err := w.waitJobCondition(ctx, client, session, jobID, "RIP completion", 6*time.Minute, 2*time.Second, func(attributes map[string]string) bool {
				if statusEquals("done ripping")(attributes) {
					return true
				}
				return !joboutcome.Evaluate(attributes, joboutcome.Policy{}).Pass
			})
			if err != nil {
				return err
			}
			if outcome := joboutcome.Evaluate(observed, joboutcome.Policy{}); !outcome.Pass {
				return &lifecycleFailure{outcome: outcome, attrs: observed}
			}
		case "production":
			// Production always follows a successful RIP in the declared modes.
			// Reuse that lifecycle guarantee rather than polling done ripping twice.
			w.addLog("Moving job %s to production release state", jobID)
			if err := client.UpdateJobAttributes(ctx, session, jobID, map[string]string{"job release state": "production"}); err != nil {
				return err
			}
			w.addLog("Waiting for job %s job release state=production", jobID)
			if _, err := w.waitJobCondition(ctx, client, session, jobID, "production release state", 2*time.Minute, 2*time.Second, attrEquals("job release state", "production")); err != nil {
				return err
			}
		case "press_print":
			w.addLog("Running press_print for job %s", jobID)
			if err := client.JobAction(ctx, session, jobID, "press_print"); err != nil {
				return err
			}
			w.addLog("Confirming press_print accepted for job %s", jobID)
			if _, err := w.waitJobCondition(ctx, client, session, jobID, "press_print accepted", 2*time.Minute, 2*time.Second, pressPrintAccepted); err != nil {
				return err
			}
		case "print":
			w.addLog("Running print for job %s", jobID)
			if err := client.JobAction(ctx, session, jobID, "print"); err != nil {
				return err
			}
			w.addLog("Waiting for job %s print completion", jobID)
			if _, err := w.waitJobCondition(ctx, client, session, jobID, "print completion", 10*time.Minute, 3*time.Second, printCompleted); err != nil {
				return err
			}
		case "cancel_ripping":
			w.addLog("Starting RIP for dedicated cancel test job %s after stable spooling", jobID)
			if err := client.JobAction(ctx, session, jobID, "rip"); err != nil {
				return err
			}
			if err := w.cancelDedicatedJob(ctx, client, session, jobID, "processing/ripping", 3*time.Minute, func(attributes map[string]string) bool {
				active, state := activelyProcessingJob(attributes)
				return active && !strings.Contains(strings.ToLower(state), "printing")
			}); err != nil {
				return err
			}
		case "cancel_waiting":
			if err := w.cancelDedicatedJob(ctx, client, session, jobID, "waiting to print", 2*time.Minute, func(attributes map[string]string) bool {
				waiting, _ := waitingToPrintJob(attributes)
				return waiting
			}); err != nil {
				return err
			}
		case "cancel_printing":
			w.addLog("Starting print for dedicated cancel test job %s", jobID)
			if err := client.JobAction(ctx, session, jobID, "print"); err != nil {
				return err
			}
			if err := w.cancelDedicatedJob(ctx, client, session, jobID, "printing", 3*time.Minute, printingJob); err != nil {
				return err
			}
		}
	}
	return nil
}

func (w *Window) cancelDedicatedJob(ctx context.Context, client *fiery.Client, session fiery.Session, jobID, scenario string, timeout time.Duration, ready func(map[string]string) bool) error {
	w.addLog("Waiting for job %s cancellation scenario=%s", jobID, scenario)
	if _, err := w.waitJobCondition(ctx, client, session, jobID, scenario+" before cancel", timeout, 250*time.Millisecond, ready); err != nil {
		return fmt.Errorf("cancel was not sent because the job never reached %s: %w", scenario, err)
	}
	w.addLog("Cancelling job %s in scenario=%s", jobID, scenario)
	if err := client.CancelJob(ctx, session, jobID); err != nil {
		return err
	}
	if _, err := w.waitJobCondition(ctx, client, session, jobID, "cancel acknowledgement", 2*time.Minute, 500*time.Millisecond, cancelObserved); err != nil {
		return err
	}
	w.addLog("Cancel acknowledged for job %s in scenario=%s", jobID, scenario)
	return nil
}

func printingJob(attributes map[string]string) bool {
	if isTruthy(attributes["is printing?"]) {
		return true
	}
	for _, key := range []string{"status", "state", "display status", "current action"} {
		value := strings.ToLower(strings.TrimSpace(attributes[key]))
		if strings.Contains(value, "printing") && !strings.Contains(value, "done") && !strings.Contains(value, "complete") {
			return true
		}
	}
	return false
}

func (w *Window) waitJobCondition(ctx context.Context, client *fiery.Client, session fiery.Session, jobID, description string, timeout, interval time.Duration, match func(map[string]string) bool) (map[string]string, error) {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	if interval <= 0 {
		interval = time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var last map[string]string
	var lastErr error
	for {
		attrs, err := client.GetJobAttributes(ctx, session, jobID)
		if err == nil {
			last = attrs
			if match(attrs) {
				return attrs, nil
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-deadline.C:
			if lastErr != nil {
				return last, fmt.Errorf("wait for job %s %s timed out after %s; last GET error: %w", jobID, description, timeout, lastErr)
			}
			return last, fmt.Errorf("wait for job %s %s timed out after %s; last status=%q state=%q release=%q recent=%q keys=%s", jobID, description, timeout, last["status"], last["state"], last["job release state"], last["recent action"], short(strings.Join(sortedKeys(last), ","), 220))
		case <-ticker.C:
		}
	}
}

func statusEquals(want string) func(map[string]string) bool {
	return func(attrs map[string]string) bool { return strings.EqualFold(strings.TrimSpace(attrs["status"]), want) }
}

func attrEquals(key, want string) func(map[string]string) bool {
	return func(attrs map[string]string) bool { return strings.EqualFold(strings.TrimSpace(attrs[key]), want) }
}

func pressPrintAccepted(attrs map[string]string) bool {
	if strings.EqualFold(attrs["queued for printing?"], "yes") || strings.EqualFold(attrs["is committed to print?"], "yes") || strings.EqualFold(attrs["is printing?"], "yes") {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(attrs["status"]))
	recent := strings.ToLower(strings.TrimSpace(attrs["recent action"]))
	return strings.Contains(status, "print") || strings.Contains(recent, "press_print") || strings.Contains(recent, "press print")
}

func printCompleted(attrs map[string]string) bool {
	if strings.EqualFold(attrs["has been printed?"], "yes") || strings.EqualFold(attrs["status"], "done printing") || strings.EqualFold(attrs["display status"], "done printing") {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(attrs["status"]))
	return strings.Contains(status, "done print") || strings.Contains(status, "printed")
}

func cancelObserved(attrs map[string]string) bool {
	for _, key := range []string{"status", "state", "display status", "recent action", "current action"} {
		value := strings.ToLower(strings.TrimSpace(attrs[key]))
		if strings.Contains(value, "cancel") || strings.Contains(value, "abort") {
			return true
		}
	}
	cancelable, state := cancelableJob(attrs)
	if cancelable || state == "unknown" {
		return false
	}
	state = strings.ToLower(state)
	return !strings.Contains(state, "done print") && !strings.Contains(state, "complete") && !strings.Contains(state, "printed")
}

func lifecyclePolicy(mode runMode) joboutcome.Policy {
	policy := joboutcome.Policy{}
	switch mode.Label {
	case "Process and Hold", "RIP":
		policy.RequireProcessedRaster = true
	case "Print":
		policy.RequirePrinted = true
	case "Cancel while Processing/Ripping", "Cancel while Waiting to Print", "Cancel while Printing":
		policy.ExpectCanceled = true
	}
	return policy
}

func modeIncludesAction(mode runMode, want string) bool {
	for _, action := range mode.Actions {
		if action == want {
			return true
		}
	}
	return false
}

func requiresRipReadback(key string) bool {
	switch key {
	case "EFPrintSpeed", "EFRotateDocument":
		return true
	default:
		return false
	}
}

func relatedReadbackValues(attrs map[string]string) string {
	related := relatedReadbackMap(attrs)
	if len(related) == 0 {
		return "none"
	}
	keys := sortedKeys(related)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", key, related[key]))
	}
	return strings.Join(parts, ", ")
}

func relatedReadbackMap(attrs map[string]string) map[string]string {
	keys := []string{"EFResolution", "Resolution", "EFPrintSpeed", "EFRaster", "EFPrintSize", "PageSize", "CustomPrintSize", "has disk raster?", "EFBrightness", "EFColorMode", "num copies"}
	out := make(map[string]string, len(keys))
	for _, key := range keys {
		if value, ok := attrs[key]; ok {
			out[key] = value
		}
	}
	return out
}

func (w *Window) connectionDraft() (model.ServerConnection, bool) {
	s := w.draftConnectionUnchecked()
	if s.IPAddress == "" || s.SecretKey == "" || s.Password == "" {
		w.setStatus("Server address, configured or replacement secret key, and administrator password are required.")
		return model.ServerConnection{}, false
	}
	return s, true
}

func (w *Window) server() (model.ServerConnection, bool) {
	w.mu.Lock()
	s, ok := w.activeServer, w.hasActiveServer
	w.mu.Unlock()
	if !ok {
		w.setStatus("Test the server connection and press OK before continuing.")
		return model.ServerConnection{}, false
	}
	return s, true
}
func (w *Window) fileMode() model.FileSelectionMode {
	switch w.fileModeGroup.Value {
	case "single":
		return model.FileSelectionSingle
	case "random":
		return model.FileSelectionRandom
	default:
		return model.FileSelectionAll
	}
}

func (w *Window) selectedRunModes() []runMode {
	if len(w.modeChecks) != len(runModes) {
		return []runMode{runModes[0]}
	}
	modes := make([]runMode, 0, len(runModes))
	for i := range runModes {
		if w.modeChecks[i].Value {
			modes = append(modes, runModes[i])
		}
	}
	return modes
}

func runModeLabels(modes []runMode) []string {
	labels := make([]string, 0, len(modes))
	for _, mode := range modes {
		labels = append(labels, mode.Label)
	}
	return labels
}

func formatRunModes(modes []runMode) string {
	return strings.Join(runModeLabels(modes), ", ")
}

func plannedTestCount(counts ...int) int64 {
	total := int64(1)
	for _, count := range counts {
		if count <= 0 {
			return 0
		}
		if int64(count) > math.MaxInt64/total {
			return math.MaxInt64
		}
		total *= int64(count)
	}
	return total
}

func runModesIncludeAction(modes []runMode, action string) bool {
	for _, mode := range modes {
		if modeIncludesAction(mode, action) {
			return true
		}
	}
	return false
}

func combinationsRequireRipReadback(combos []combinations.Combination) bool {
	for _, combo := range combos {
		for key := range combo {
			if requiresRipReadback(key) {
				return true
			}
		}
	}
	return false
}

func copiesOption(model capabilities.Model) (capabilities.Option, bool) {
	for _, id := range []string{"EFCopies", "num copies"} {
		if option, ok := model.OptionByID(id); ok {
			return option, true
		}
	}
	return capabilities.Option{}, false
}

func (w *Window) logSelectedCombinations(combos []combinations.Combination, axes []combinations.Axis) {
	if copySelection, err := copyvalues.Parse(w.copiesInput.Text()); err == nil && copySelection.HasRange {
		w.addLog("Copies range expanded to %d value(s) and randomized within Max cases=%s", len(copySelection.Values), w.maxCases.Text())
	}
	if w.constraintSkipped > 0 {
		w.addLog("Skipped %d locally conflicting combination(s) using published Fiery constraints; first conflict: %s", w.constraintSkipped, w.constraintWarning)
	}
	selected := make([]string, 0, len(axes))
	for _, axis := range axes {
		values := append([]string(nil), axis.Values...)
		if isPageRangeOption(axis.Name) {
			for index, value := range values {
				if strings.HasPrefix(value, pageRangeInternalPrefix) {
					values[index] = "Custom(" + strings.TrimPrefix(value, pageRangeInternalPrefix) + ")"
				}
			}
		}
		selected = append(selected, fmt.Sprintf("%s=%v", axis.Name, values))
	}
	if len(selected) == 0 {
		w.addLog("Selected %d combination(s) for strategy=%s; no job attributes selected, running import/lifecycle only", len(combos), w.strategy)
		return
	}
	w.addLog("Selected %d combination(s) for strategy=%s; axes: %s", len(combos), w.strategy, strings.Join(selected, "; "))
}
func defaultPermutationAxes(model capabilities.Model) []combinations.Axis {
	preferred := []string{"EFResolution", "EFColorMode", "EFMediaType", "EFPrintSpeed", "PageSize", "EFBrightness", "EFPrintCover", "EFOutputBin"}
	axes := make([]combinations.Axis, 0, len(preferred))
	seen := map[string]struct{}{}
	for _, id := range preferred {
		if opt, ok := model.OptionByID(id); ok {
			vals := optionValues(opt)
			if len(vals) > 1 {
				axes = append(axes, combinations.Axis{Name: opt.ID, Values: vals})
				seen[opt.ID] = struct{}{}
			}
		}
	}
	if len(axes) > 0 {
		return axes
	}
	for _, opt := range model.Options {
		if isCopiesOption(opt.ID) {
			continue
		}
		if _, ok := seen[opt.ID]; ok {
			continue
		}
		vals := checkboxOptionValues(opt)
		if len(vals) > 1 && len(vals) <= 12 && isLikelyJobAttribute(opt) {
			axes = append(axes, combinations.Axis{Name: opt.ID, Values: vals})
		}
		if len(axes) >= 8 {
			break
		}
	}
	return axes
}

func isLikelyJobAttribute(opt capabilities.Option) bool {
	for _, scope := range opt.Scopes {
		s := strings.ToLower(scope)
		if s == "command" || s == "ps" || s == "appe" || s == "uimenu" || strings.HasPrefix(s, "fp") {
			return true
		}
	}
	return false
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func formatStringMap(values map[string]string) string { return formatAttributes(values) }

func formatAttributes(attrs map[string]string) string {
	keys := make([]string, 0, len(attrs))
	for key := range attrs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%q", key, attrs[key]))
	}
	return strings.Join(parts, ", ")
}

func sortedKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func effectiveWorkerCount(requested int, plannedTests int64) int {
	if requested < 1 {
		requested = 1
	}
	requested = min(requested, maxWorkerCount)
	if plannedTests > 0 && plannedTests < int64(requested) {
		return int(plannedTests)
	}
	return requested
}

func parseWorkerCount(value string) int {
	workers, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || workers < 1 {
		return 1
	}
	return min(workers, maxWorkerCount)
}

func parseCaseLimit(value string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || limit < 1 {
		return defaultCaseLimit
	}
	return min(limit, maxCaseLimit)
}

func selectedValues(m map[string]*widget.Bool) []string {
	var vals []string
	for v, b := range m {
		if b.Value {
			vals = append(vals, v)
		}
	}
	return vals
}
func combinationForConstraintValidation(combination combinations.Combination) map[string]string {
	return combinationToAttributes(combination)
}

func combinationToAttributes(combination combinations.Combination) map[string]string {
	attributes := cloneStringMap(combination)
	if custom, ok := attributes[pageRangeOptionID]; ok && strings.HasPrefix(custom, pageRangeInternalPrefix) {
		attributes[pageRangeOptionID] = strings.TrimPrefix(custom, pageRangeInternalPrefix)
	}
	// CWS/Postman evidence shows custom ranges are represented directly by
	// EFPageRange while DPP_PAGE_RANGE remains empty. Never emit the legacy
	// companion, even if stale data reaches combination generation.
	delete(attributes, pageRangeLegacyDataID)
	return attributes
}

func normalizeOutputProfileValue(value string) string {
	// U+FEFF is part of Fiery's advertised EFOutProfile wire identity, but is
	// not a visible part of the profile name. Ignore it only for presentation
	// and readback comparison; combinationToAttributes must preserve it.
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(value), "\ufeff"))
}

func displayOptionValue(optionID, value string) string {
	if strings.EqualFold(strings.TrimSpace(optionID), outputProfileOptionID) {
		return normalizeOutputProfileValue(value)
	}
	return value
}

func optionValueMatches(optionID, left, right string) bool {
	if strings.EqualFold(strings.TrimSpace(optionID), outputProfileOptionID) {
		left = normalizeOutputProfileValue(left)
		right = normalizeOutputProfileValue(right)
	}
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func cloneStringMap[M ~map[string]string](source M) map[string]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func selectedReadbackValues(got, selected map[string]string) map[string]string {
	if len(selected) == 0 {
		return nil
	}
	values := make(map[string]string, len(selected))
	for key := range selected {
		values[key] = got[key]
	}
	return values
}

func jobNameFromAttributes(attributes map[string]string, fallbackName string) string {
	for _, key := range []string{"job name", "name", "document name", "title"} {
		if value := strings.TrimSpace(attributes[key]); value != "" {
			return value
		}
	}
	return fallbackName
}
func ensureBools(store map[string]map[string]*widget.Bool, id string, vals []string) {
	if store[id] == nil {
		store[id] = map[string]*widget.Bool{}
	}
	for _, v := range vals {
		if store[id][v] == nil {
			store[id][v] = &widget.Bool{}
		}
	}
}
func optionValues(opt capabilities.Option) []string {
	if len(opt.Values) > 0 {
		return opt.Values
	}
	if opt.Value != "" {
		return []string{opt.Value}
	}
	return nil
}

func checkboxOptionValues(opt capabilities.Option) []string {
	// Every advertised enum remains an independent exact value. Custom text is
	// a separate direct EFPageRange expression and never replaces Range1.
	return optionValues(opt)
}

func (w *Window) finishRun(status string) error {
	completedAt := time.Now()
	w.mu.Lock()
	w.lastRun.Status = status
	w.lastRun.CompletedAt = completedAt
	store := w.resultStore
	storeError := w.resultStoreError
	w.mu.Unlock()
	if store != nil {
		if err := store.Close(); err != nil {
			storeError = err.Error()
			w.diagnostic.printf("RESULT_STORE: close error=%v", err)
		}
	}
	if storeError != "" {
		w.mu.Lock()
		w.lastRun.Status = status + " with result-storage error"
		w.resultStoreError = storeError
		w.mu.Unlock()
		return errors.New(storeError)
	}
	return nil
}

func (w *Window) closeResultStore() {
	w.mu.Lock()
	store := w.resultStore
	w.mu.Unlock()
	if store != nil {
		if err := store.Close(); err != nil && w.diagnostic != nil {
			w.diagnostic.printf("RESULT_STORE: shutdown close error=%v", err)
		}
	}
}

func (w *Window) startExcelExport() {
	if w.running.Load() {
		w.setStatus("Wait for the automation run to finish before exporting Excel results.")
		return
	}
	if !w.exportingResults.CompareAndSwap(false, true) {
		return
	}
	w.mu.Lock()
	summary := w.lastRun
	store := w.resultStore
	storeError := w.resultStoreError
	capabilityModel := w.capabilities
	w.mu.Unlock()
	if summary.StartedAt.IsZero() || store == nil || store.Path() == "" {
		w.exportingResults.Store(false)
		w.setStatus("Run automation before exporting Excel results.")
		return
	}
	if storeError != "" {
		w.exportingResults.Store(false)
		w.setStatus("Excel export is unavailable because result storage failed: " + storeError)
		return
	}

	defaultName := "api-automation-results-" + summary.StartedAt.Format("20060102-150405") + ".xlsx"
	path, err := saveExcelPath(defaultName)
	if err != nil {
		w.exportingResults.Store(false)
		w.setStatus("Excel export selection failed: " + err.Error())
		return
	}
	if path == "" {
		w.exportingResults.Store(false)
		return
	}
	labels := make(map[string]string, len(capabilityModel.Options))
	for _, option := range capabilityModel.Options {
		labels[option.ID] = option.Label
	}
	resultsPath := store.Path()
	w.setStatus("Exporting Excel test report...")
	w.launchBackground("Excel export", func() {
		defer w.exportingResults.Store(false)
		stats, err := reportxlsx.Export(path, reportxlsx.Report{Summary: summary, ResultsPath: resultsPath, AttributeLabels: labels})
		if err != nil {
			w.setStatus("Excel export failed: " + err.Error())
			w.addLog("Excel export failed: %v", err)
			return
		}
		w.setStatus(fmt.Sprintf("Excel report saved: %s", path))
		w.addLog("Excel report saved: %s (tests=%d pass=%d fail=%d errors=%d)", path, stats.Total, stats.Passed, stats.Failed, stats.Errors)
	})
}

func (w *Window) setStatus(s string) {
	w.mu.Lock()
	w.status = s
	w.mu.Unlock()
	w.diagnostic.printf("STATUS: %s", s)
	w.invalidate()
}

func (w *Window) setServerTestStatus(s string) {
	w.mu.Lock()
	w.serverTestStatus = s
	w.mu.Unlock()
	w.diagnostic.printf("SERVER_TEST: %s", s)
	w.invalidate()
}

func (w *Window) addLog(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	w.diagnostic.printf("UI: %s", line)
	w.mu.Lock()
	w.logCount++
	w.log = append(w.log, time.Now().Format("15:04:05  ")+line)
	if len(w.log) > maxRetainedLogLines+retainedEntryTrimBatch {
		w.log = append([]string(nil), w.log[len(w.log)-maxRetainedLogLines:]...)
	}
	w.mu.Unlock()
	w.invalidate()
}
func (w *Window) addResult(result reportxlsx.Result) {
	duration := time.Duration(result.DurationMS) * time.Millisecond
	w.mu.Lock()
	w.resultCount++
	switch strings.ToUpper(strings.TrimSpace(result.Result)) {
	case "PASS":
		w.passedCount++
	case "FAIL":
		w.failedCount++
	default:
		w.errorCount++
	}
	w.results = append(w.results, resultRow{JobID: result.JobID, JobName: result.JobName, Result: result.Result, Status: result.JobStatus, State: result.JobState, Duration: duration.Round(time.Millisecond).String(), Detail: result.Detail})
	if len(w.results) > maxRetainedResults+retainedEntryTrimBatch {
		w.results = append([]resultRow(nil), w.results[len(w.results)-maxRetainedResults:]...)
	}
	store := w.resultStore
	w.mu.Unlock()
	if store == nil {
		w.diagnostic.printf("RESULT_STORE: result was not persisted because the store is unavailable job=%s", result.JobID)
	} else if err := store.Append(result); err != nil {
		w.mu.Lock()
		if w.resultStoreError == "" {
			w.resultStoreError = err.Error()
		}
		w.mu.Unlock()
		w.diagnostic.printf("RESULT_STORE: append error job=%s error=%v", result.JobID, err)
	}
	w.addLog("%s %s %s", result.JobName, result.Result, result.Detail)
}

func card(gtx layout.Context, child layout.Widget) layout.Dimensions {
	return roundedPanel(gtx, unit.Dp(18), 16, palette.surface, rgb(0xd8e2ef), child)
}
func surfaceAlt(gtx layout.Context, child layout.Widget) layout.Dimensions {
	return roundedPanel(gtx, unit.Dp(12), 10, palette.surfaceAlt, rgb(0xd8e2ef), child)
}

func formPanel(gtx layout.Context, child layout.Widget) layout.Dimensions {
	return roundedPanel(gtx, unit.Dp(20), 16, palette.surfaceAlt, rgb(0xd8e2ef), child)
}

func roundedPanel(gtx layout.Context, inset unit.Dp, radius int, fill, border color.NRGBA, child layout.Widget) layout.Dimensions {
	macro := op.Record(gtx.Ops)
	dims := layout.UniformInset(inset).Layout(gtx, child)
	call := macro.Stop()
	rr := clip.RRect{Rect: image.Rectangle{Max: dims.Size}, SE: radius, SW: radius, NE: radius, NW: radius}
	paint.FillShape(gtx.Ops, fill, rr.Op(gtx.Ops))
	paint.FillShape(gtx.Ops, border, clip.Stroke{Path: rr.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
	call.Add(gtx.Ops)
	return dims
}
func label(th *material.Theme, text string, size unit.Sp, c color.NRGBA) material.LabelStyle {
	l := material.Label(th, size, text)
	l.Color = c
	return l
}
func sectionTitle(th *material.Theme, text string) layout.Widget {
	return label(th, text, 17, palette.text).Layout
}
func field(th *material.Theme, title string, ed *widget.Editor, width int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(unit.Dp(width)), gtx.Dp(unit.Dp(width))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(label(th, title, 13, palette.muted).Layout), layout.Rigid(func(gtx layout.Context) layout.Dimensions { e := material.Editor(th, ed, ""); return e.Layout(gtx) }))
	}
}

func fieldBox(th *material.Theme, title, hint string, ed *widget.Editor, width int) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		if width > 0 {
			w := minInt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(width)))
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = w, w
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(th, title, 14, palette.text).Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				gtx.Constraints.Min.X = gtx.Constraints.Max.X
				gtx.Constraints.Min.Y = gtx.Dp(unit.Dp(52))
				rr := clip.UniformRRect(image.Rectangle{Max: gtx.Constraints.Min}, gtx.Dp(unit.Dp(10)))
				paint.FillShape(gtx.Ops, rgb(0xfbfdff), rr.Op(gtx.Ops))
				paint.FillShape(gtx.Ops, palette.border, clip.Stroke{Path: rr.Path(gtx.Ops), Width: float32(gtx.Dp(unit.Dp(1)))}.Op())
				return layout.Inset{Top: unit.Dp(10), Right: unit.Dp(14), Bottom: unit.Dp(8), Left: unit.Dp(14)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
					editor := material.Editor(th, ed, hint)
					editor.TextSize = 16
					return editor.Layout(gtx)
				})
			}),
		)
	}
}
func row(gtx layout.Context, widgets ...layout.Widget) layout.Dimensions {
	ch := []layout.FlexChild{}
	for _, w := range widgets {
		ww := w
		ch = append(ch, layout.Rigid(ww), layout.Rigid(spacerX(14)))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, ch...)
}
func primaryButton(th *material.Theme, b *widget.Clickable, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, b, text)
		btn.Background = palette.primary
		return btn.Layout(gtx)
	}
}
func secondaryButton(th *material.Theme, b *widget.Clickable, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, b, text)
		btn.Background = palette.surfaceAlt
		btn.Color = palette.text
		return btn.Layout(gtx)
	}
}

func dangerButton(th *material.Theme, b *widget.Clickable, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, b, text)
		btn.Background = palette.danger
		return btn.Layout(gtx)
	}
}

func browseButton(th *material.Theme, b *widget.Clickable, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, b, text)
		btn.Background = palette.primaryDim
		btn.Color = palette.primary
		return btn.Layout(gtx)
	}
}

func statusBadge(th *material.Theme, text string, c color.NRGBA) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return roundedPanel(gtx, unit.Dp(10), 18, withAlpha(c, 0x22), withAlpha(c, 0x55), func(gtx layout.Context) layout.Dimensions {
			return label(th, text, 14, c).Layout(gtx)
		})
	}
}

func serverStatusColor(status string) color.NRGBA {
	lower := strings.ToLower(status)
	switch {
	case strings.Contains(lower, "ok"):
		return palette.success
	case strings.Contains(lower, "failed") || strings.Contains(lower, "missing"):
		return palette.danger
	case strings.Contains(lower, "testing"):
		return palette.primary
	default:
		return palette.muted
	}
}

func withAlpha(c color.NRGBA, alpha uint8) color.NRGBA {
	c.A = alpha
	return c
}

func toggle(th *material.Theme, b *widget.Clickable, text string, active bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		btn := material.Button(th, b, text)
		if active {
			btn.Background = palette.primary
			btn.Color = rgb(0xffffff)
		} else {
			btn.Background = palette.primaryDim
			btn.Color = palette.text
		}
		return btn.Layout(gtx)
	}
}
func navButton(th *material.Theme, b *widget.Clickable, text string, active bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(th, b, text)
			btn.Background = rgb(0x1e293b)
			btn.Color = rgb(0xe2e8f0)
			if active {
				btn.Background = palette.primary
				btn.Color = rgb(0xffffff)
			}
			return btn.Layout(gtx)
		})
	}
}
func spacer(h unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(0, gtx.Dp(h))} }
}
func spacerX(w unit.Dp) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions { return layout.Dimensions{Size: image.Pt(gtx.Dp(w), 0)} }
}
func rgb(v uint32) color.NRGBA {
	return color.NRGBA{R: uint8(v >> 16), G: uint8(v >> 8), B: uint8(v), A: 0xff}
}
func fallback(v, f string) string {
	if strings.TrimSpace(v) == "" {
		return f
	}
	return v
}
func short(s string, n int) string {
	if len(s) <= n {
		return s
	}
	if n <= 1 {
		return s[:n]
	}
	return s[:n-1] + "…"
}
