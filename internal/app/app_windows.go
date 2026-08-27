//go:build windows

package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/fiery"
	"api-automation/internal/files"
	"api-automation/internal/model"
	"api-automation/internal/preflight"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

type hwndControl interface {
	Hwnd() win.HWND
}

type MainWindow struct {
	wnd               *ui.Main
	serverIP          *ui.Edit
	secretKey         *ui.Edit
	password          *ui.Edit
	folderPath        *ui.Edit
	filePath          *ui.Edit
	selectionMode     *ui.ComboBox
	url               *ui.Edit
	method            *ui.Edit
	queue             *ui.ComboBox
	pageSize          *ui.ComboBox
	resolution        *ui.ComboBox
	colorMode         *ui.ComboBox
	mediaType         *ui.ComboBox
	printSpeed        *ui.ComboBox
	multiSelects      map[string][]*ui.CheckBox
	multiSelectValues map[string][]string
	runMode           *ui.ComboBox
	strategy          *ui.ComboBox
	maxCases          *ui.Edit
	concurrency       *ui.Edit
	runButton         *ui.Button
	captureButton     *ui.Button
	settingsButton    *ui.Button
	cancelButton      *ui.Button
	browseFolder      *ui.Button
	browseFile        *ui.Button
	results           *ui.ListView
	log               *ui.Edit
	status            *ui.Static
	progress          *ui.ProgressBar
	settingsCtrls     []hwndControl
	capabilityCtrls   []hwndControl
	settingsVisible   bool
	theme             appTheme

	capabilities   capabilities.Model
	cancel         context.CancelFunc
	running        atomic.Bool
	closeRequested atomic.Bool
}

func Run() int {
	_, _ = win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
	defer win.CoUninitialize()

	wnd := ui.NewMain(ui.OptsMain().Title("API Automation").Size(ui.Dpi(windowWidth, windowHeight)))

	// Enterprise shell: persistent navigation on the left, work area on the right.
	ui.NewStatic(wnd, ui.OptsStatic().Text("API Automation").Position(ui.Dpi(18, 18)).Size(ui.Dpi(150, 22)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Workspace").Position(ui.Dpi(18, 58)).Size(ui.Dpi(150, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Server").Position(ui.Dpi(30, 88)).Size(ui.Dpi(130, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Test Files").Position(ui.Dpi(30, 116)).Size(ui.Dpi(130, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Requests").Position(ui.Dpi(30, 144)).Size(ui.Dpi(130, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Run History").Position(ui.Dpi(30, 172)).Size(ui.Dpi(130, 20)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("Server Execution Workspace").Position(ui.Dpi(220, 18)).Size(ui.Dpi(300, 24)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Discover Fiery capabilities, select job options, and run API automation.").Position(ui.Dpi(220, 48)).Size(ui.Dpi(500, 22)))

	settingsButton := ui.NewButton(wnd, ui.OptsButton().Text("&Settings").Position(ui.Dpi(620, 24)).Width(ui.DpiX(110)).Height(ui.DpiY(30)))
	captureButton := ui.NewButton(wnd, ui.OptsButton().Text("Get server &capabilities").Position(ui.Dpi(742, 24)).Width(ui.DpiX(240)).Height(ui.DpiY(30)))
	runButton := ui.NewButton(wnd, ui.OptsButton().Text("&Run automation").Position(ui.Dpi(994, 24)).Width(ui.DpiX(142)).Height(ui.DpiY(30)))
	cancelButton := ui.NewButton(wnd, ui.OptsButton().Text("&Cancel run").Position(ui.Dpi(994, 60)).Width(ui.DpiX(142)).Height(ui.DpiY(28)))
	progress := ui.NewProgressBar(wnd, ui.OptsProgressBar().Position(ui.Dpi(742, 62)).Size(ui.Dpi(240, 18)))

	var settingsCtrls []hwndControl
	settingsTitle := ui.NewStatic(wnd, ui.OptsStatic().Text("SETTINGS  SERVER CONNECTION").Position(ui.Dpi(220, 80)).Size(ui.Dpi(320, 18)))
	settingsCtrls = append(settingsCtrls, settingsTitle)
	serverIPLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Server IP address").Position(ui.Dpi(220, 100)).Size(ui.Dpi(140, 20)))
	settingsCtrls = append(settingsCtrls, serverIPLabel)
	serverIP := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(220, 124)).Width(ui.DpiX(220)).Height(ui.DpiY(26)))
	settingsCtrls = append(settingsCtrls, serverIP)
	secretLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Secret key").Position(ui.Dpi(456, 100)).Size(ui.Dpi(120, 20)))
	settingsCtrls = append(settingsCtrls, secretLabel)
	secretKey := ui.NewEdit(wnd, ui.OptsEdit().Text(fiery.DefaultSecretKey).Position(ui.Dpi(456, 124)).Width(ui.DpiX(310)).Height(ui.DpiY(26)).CtrlStyle(co.ES_PASSWORD|co.ES_AUTOHSCROLL|co.ES_NOHIDESEL))
	settingsCtrls = append(settingsCtrls, secretKey)
	passwordLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Admin password").Position(ui.Dpi(782, 100)).Size(ui.Dpi(130, 20)))
	settingsCtrls = append(settingsCtrls, passwordLabel)
	password := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(782, 124)).Width(ui.DpiX(160)).Height(ui.DpiY(26)).CtrlStyle(co.ES_PASSWORD|co.ES_AUTOHSCROLL|co.ES_NOHIDESEL))
	settingsCtrls = append(settingsCtrls, password)

	ui.NewStatic(wnd, ui.OptsStatic().Text("01  TEST ASSETS").Position(ui.Dpi(220, 82)).Size(ui.Dpi(180, 18)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Folder").Position(ui.Dpi(220, 102)).Size(ui.Dpi(90, 20)))
	folderPath := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(220, 126)).Width(ui.DpiX(650)).Height(ui.DpiY(26)))
	browseFolder := ui.NewButton(wnd, ui.OptsButton().Text("Browse...").Position(ui.Dpi(884, 125)).Width(ui.DpiX(96)).Height(ui.DpiY(28)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("Selection").Position(ui.Dpi(220, 160)).Size(ui.Dpi(90, 20)))
	selectionMode := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(220, 184)).Width(ui.DpiX(150)).Texts("All files", "Single file", "Random file").Select(0))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Specific file (used only for Single file)").Position(ui.Dpi(396, 160)).Size(ui.Dpi(260, 20)))
	filePath := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(396, 184)).Width(ui.DpiX(474)).Height(ui.DpiY(26)))
	browseFile := ui.NewButton(wnd, ui.OptsButton().Text("Browse...").Position(ui.Dpi(884, 183)).Width(ui.DpiX(96)).Height(ui.DpiY(28)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("02  FIERY IMPORT").Position(ui.Dpi(220, 234)).Size(ui.Dpi(180, 18)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Method").Position(ui.Dpi(220, 254)).Size(ui.Dpi(70, 20)))
	method := ui.NewEdit(wnd, ui.OptsEdit().Text(http.MethodPost).Position(ui.Dpi(220, 278)).Width(ui.DpiX(92)).Height(ui.DpiY(26)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Import endpoint").Position(ui.Dpi(336, 254)).Size(ui.Dpi(120, 20)))
	url := ui.NewEdit(wnd, ui.OptsEdit().Text("/live/api/v5/jobs").Position(ui.Dpi(336, 278)).Width(ui.DpiX(510)).Height(ui.DpiY(26)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Workers").Position(ui.Dpi(860, 254)).Size(ui.Dpi(90, 20)))
	concurrency := ui.NewEdit(wnd, ui.OptsEdit().Text("1").Position(ui.Dpi(860, 278)).Width(ui.DpiX(76)).Height(ui.DpiY(26)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Run mode").Position(ui.Dpi(952, 254)).Size(ui.Dpi(90, 20)))
	runMode := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(952, 278)).Width(ui.DpiX(170)).Texts("Hold", "Process and Hold", "RIP", "Press Print", "Ready to Print", "Print").Select(0))
	strategyLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Strategy").Position(ui.Dpi(660, 398)).Size(ui.Dpi(90, 20)))
	strategy := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(660, 420)).Width(ui.DpiX(160)).Texts("Selected only", "All permutations", "Pairwise", "Random sample").Select(0))
	maxCasesLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Max cases").Position(ui.Dpi(840, 398)).Size(ui.Dpi(90, 20)))
	maxCases := ui.NewEdit(wnd, ui.OptsEdit().Text("100").Position(ui.Dpi(840, 420)).Width(ui.DpiX(76)).Height(ui.DpiY(26)))

	var capabilityCtrls []hwndControl
	capTitle := ui.NewStatic(wnd, ui.OptsStatic().Text("03  SERVER CAPABILITIES").Position(ui.Dpi(220, 326)).Size(ui.Dpi(220, 18)))
	capabilityCtrls = append(capabilityCtrls, capTitle)
	queueLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Queue").Position(ui.Dpi(220, 330)).Size(ui.Dpi(90, 20)))
	capabilityCtrls = append(capabilityCtrls, queueLabel)
	queue := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(220, 354)).Width(ui.DpiX(170)).Texts("Capture capabilities first").Select(0))
	capabilityCtrls = append(capabilityCtrls, queue)
	pageSizeLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Page size").Position(ui.Dpi(410, 330)).Size(ui.Dpi(90, 20)))
	capabilityCtrls = append(capabilityCtrls, pageSizeLabel)
	pageSize := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(410, 354)).Width(ui.DpiX(170)).Texts("Capture capabilities first").Select(0))
	capabilityCtrls = append(capabilityCtrls, pageSize)
	resolutionLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Resolution").Position(ui.Dpi(600, 330)).Size(ui.Dpi(90, 20)))
	capabilityCtrls = append(capabilityCtrls, resolutionLabel)
	resolution := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(600, 354)).Width(ui.DpiX(140)).Texts("Capture first").Select(0))
	capabilityCtrls = append(capabilityCtrls, resolution)
	colorModeLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Color mode").Position(ui.Dpi(760, 330)).Size(ui.Dpi(90, 20)))
	capabilityCtrls = append(capabilityCtrls, colorModeLabel)
	colorMode := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(760, 354)).Width(ui.DpiX(140)).Texts("Capture first").Select(0))
	capabilityCtrls = append(capabilityCtrls, colorMode)
	mediaLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Media type").Position(ui.Dpi(220, 396)).Size(ui.Dpi(90, 20)))
	capabilityCtrls = append(capabilityCtrls, mediaLabel)
	mediaType := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(220, 420)).Width(ui.DpiX(240)).Texts("Capture capabilities first").Select(0))
	capabilityCtrls = append(capabilityCtrls, mediaType)
	printSpeedLabel := ui.NewStatic(wnd, ui.OptsStatic().Text("Print speed").Position(ui.Dpi(480, 396)).Size(ui.Dpi(90, 20)))
	capabilityCtrls = append(capabilityCtrls, printSpeedLabel)
	printSpeed := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(480, 420)).Width(ui.DpiX(150)).Texts("Capture first").Select(0))
	capabilityCtrls = append(capabilityCtrls, printSpeed)

	multiTitle := ui.NewStatic(wnd, ui.OptsStatic().Text("Multi-select values for permutation strategies").Position(ui.Dpi(220, 462)).Size(ui.Dpi(320, 18)))
	capabilityCtrls = append(capabilityCtrls, multiTitle)
	multiSelects, multiHandles := buildMultiSelectControls(wnd)
	capabilityCtrls = append(capabilityCtrls, multiHandles...)
	capabilityCtrls = append(capabilityCtrls, strategyLabel, strategy, maxCasesLabel, maxCases)

	status := ui.NewStatic(wnd, ui.OptsStatic().Text("Open Settings, enter server details, then click Get capabilities of the server.").Position(ui.Dpi(220, 560)).Size(ui.Dpi(940, 22)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("04  EXECUTION RESULTS").Position(ui.Dpi(220, 586)).Size(ui.Dpi(200, 18)))
	results := ui.NewListView(wnd, ui.OptsListView().
		Position(ui.Dpi(220, 612)).Size(ui.Dpi(940, 86)).
		CtrlExStyle(co.LVS_EX_FULLROWSELECT|co.LVS_EX_GRIDLINES).
		Column("Request", ui.DpiX(190)).Column("Method", ui.DpiX(90)).Column("Status", ui.DpiX(90)).Column("Duration", ui.DpiX(120)).Column("URL", ui.DpiX(420)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("05  ACTIVITY LOG").Position(ui.Dpi(220, 706)).Size(ui.Dpi(180, 18)))
	log := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(220, 730)).Width(ui.DpiX(940)).Height(ui.DpiY(24)).
		CtrlStyle(co.ES_MULTILINE|co.ES_AUTOVSCROLL|co.ES_READONLY|co.ES_WANTRETURN).
		WndStyle(co.WS_CHILD|co.WS_VISIBLE|co.WS_VSCROLL|co.WS_TABSTOP))

	mw := &MainWindow{wnd: wnd, serverIP: serverIP, secretKey: secretKey, password: password, folderPath: folderPath, filePath: filePath, selectionMode: selectionMode, url: url, method: method, queue: queue, pageSize: pageSize, resolution: resolution, colorMode: colorMode, mediaType: mediaType, printSpeed: printSpeed, multiSelects: multiSelects, multiSelectValues: map[string][]string{}, runMode: runMode, strategy: strategy, maxCases: maxCases, concurrency: concurrency, runButton: runButton, captureButton: captureButton, settingsButton: settingsButton, cancelButton: cancelButton, browseFolder: browseFolder, browseFile: browseFile, results: results, log: log, status: status, progress: progress, settingsCtrls: settingsCtrls, capabilityCtrls: capabilityCtrls}
	mw.events()
	return wnd.RunAsMain()
}

func buildMultiSelectControls(wnd *ui.Main) (map[string][]*ui.CheckBox, []hwndControl) {
	defs := []struct {
		id    string
		label string
		x, y  int
	}{
		{id: "PageSize", label: "Page size", x: 220, y: 486},
		{id: "EFResolution", label: "Resolution", x: 410, y: 486},
		{id: "EFColorMode", label: "Color mode", x: 600, y: 486},
		{id: "EFMediaType", label: "Media type", x: 790, y: 486},
		{id: "EFPrintSpeed", label: "Print speed", x: 980, y: 486},
	}
	controls := make(map[string][]*ui.CheckBox, len(defs))
	var handles []hwndControl
	for _, def := range defs {
		label := ui.NewStatic(wnd, ui.OptsStatic().Text(def.label).Position(ui.Dpi(def.x, def.y)).Size(ui.Dpi(150, 18)))
		handles = append(handles, label)
		for i := 0; i < 4; i++ {
			chk := ui.NewCheckBox(wnd, ui.OptsCheckBox().Text("-").Position(ui.Dpi(def.x, def.y+24+(i*22))).Size(ui.Dpi(178, 20)))
			handles = append(handles, chk)
			controls[def.id] = append(controls[def.id], chk)
		}
	}
	return controls, handles
}

func (m *MainWindow) events() {
	m.theme = newAppTheme()
	m.theme.apply(m)
	m.wnd.On().WmCreate(func(_ ui.WmCreate) int {
		_ = m.wnd.Hwnd().DwmSetWindowAttribute(win.DwmAttrCaptionColor(win.RGB(9, 30, 66)))
		_ = m.wnd.Hwnd().DwmSetWindowAttribute(win.DwmAttrTextColor(win.RGB(255, 255, 255)))
		_ = m.wnd.Hwnd().DwmSetWindowAttribute(win.DwmAttrBorderColor(win.RGB(0, 82, 204)))
		m.expandComboDropDowns()
		m.showControls(m.settingsCtrls, false)
		m.showControls(m.capabilityCtrls, false)
		m.progress.Hwnd().ShowWindow(co.SW_HIDE)
		return 0
	})
	m.runButton.On().BnClicked(func() { m.startRun() })
	m.captureButton.On().BnClicked(func() { m.captureCapabilities() })
	m.settingsButton.On().BnClicked(func() { m.toggleSettings() })
	m.cancelButton.On().BnClicked(func() { m.cancelRun() })
	m.browseFolder.On().BnClicked(func() {
		path, err := browsePath(m.wnd.Hwnd(), true)
		if err != nil {
			m.appendLog("Folder selection failed: %s", err)
			return
		}
		if path != "" {
			m.folderPath.SetText(path)
		}
	})
	m.browseFile.On().BnClicked(func() {
		path, err := browsePath(m.wnd.Hwnd(), false)
		if err != nil {
			m.appendLog("File selection failed: %s", err)
			return
		}
		if path != "" {
			m.filePath.SetText(path)
		}
	})
	m.wnd.On().Wm(co.WM_CLOSE, func(_ ui.Wm) uintptr {
		if m.running.Load() {
			m.closeRequested.Store(true)
			m.cancelRun()
			m.setStatus("Cancellation requested. Waiting for running work to stop before closing...")
			return 0
		}
		m.destroyWindow()
		return 0
	})
}

func (m *MainWindow) destroyWindow() {
	m.theme.dispose()
	_ = m.wnd.Hwnd().DestroyWindow()
}

func (m *MainWindow) finishAsyncWork() {
	m.running.Store(false)
	if m.closeRequested.Load() {
		m.destroyWindow()
	}
}

func (m *MainWindow) showControls(handles []hwndControl, visible bool) {
	cmd := co.SW_HIDE
	if visible {
		cmd = co.SW_SHOW
	}
	for _, ctrl := range handles {
		ctrl.Hwnd().ShowWindow(cmd)
	}
}

func (m *MainWindow) toggleSettings() {
	m.settingsVisible = !m.settingsVisible
	m.showControls(m.settingsCtrls, m.settingsVisible)
}

func (m *MainWindow) startRun() {
	if m.running.Load() {
		return
	}
	server, ok := m.serverConnection()
	if !ok {
		return
	}

	selectedFiles, err := files.Select(model.TestFileSelection{
		FolderPath: strings.TrimSpace(m.folderPath.Text()),
		Mode:       m.selectedFileMode(),
		FilePath:   strings.TrimSpace(m.filePath.Text()),
	})
	if err != nil {
		m.setStatus("Test file selection is invalid.")
		m.appendLog("Validation failed: %s", err)
		return
	}

	workers, err := strconv.Atoi(strings.TrimSpace(m.concurrency.Text()))
	if err != nil || workers < 1 {
		workers = 1
	}
	if workers > len(selectedFiles) {
		workers = len(selectedFiles)
	}

	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running.Store(true)
	mode := m.selectedRunMode()
	strategy := m.selectedStrategy()
	maxCases := m.selectedMaxCases()
	combos := m.selectedCombinations(strategy, maxCases)
	m.results.DeleteAllItems()
	m.setStatus("Connecting to Fiery server " + server.IPAddress + "...")
	m.appendLog("Starting server automation with %d worker(s), %d selected test file(s), mode=%s, strategy=%s", workers, len(selectedFiles), mode.Label, strategy)
	m.appendLog("Generated %d executable test combination(s)", len(combos))

	go m.runFieryAutomation(ctx, server, selectedFiles, workers, combos, mode)
}

func (m *MainWindow) serverConnection() (model.ServerConnection, bool) {
	server := model.ServerConnection{
		IPAddress: strings.TrimSpace(m.serverIP.Text()),
		SecretKey: strings.TrimSpace(m.secretKey.Text()),
		Password:  strings.TrimSpace(m.password.Text()),
	}
	if server.IPAddress == "" || server.SecretKey == "" || server.Password == "" {
		m.setStatus("Server IP address, secret key, and admin password are required.")
		m.appendLog("Validation failed: server IP address, secret key, and admin password are required")
		return model.ServerConnection{}, false
	}
	return server, true
}

func (m *MainWindow) cancelRun() {
	if m.cancel != nil {
		m.cancel()
		m.appendLog("Cancellation requested")
	}
}

func (m *MainWindow) captureCapabilities() {
	if m.running.Load() {
		return
	}
	server, ok := m.serverConnection()
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running.Store(true)
	m.showControls(m.capabilityCtrls, false)
	m.progress.Hwnd().ShowWindow(co.SW_SHOW)
	m.progress.SetMarquee(true)
	m.setStatus("Getting capabilities from the server...")
	m.appendLog("Starting v5 capability capture for %s", server.IPAddress)
	go m.runCapabilityCapture(ctx, server)
}

func (m *MainWindow) runCapabilityCapture(ctx context.Context, server model.ServerConnection) {
	defer func() {
		m.wnd.UiThread(func() {
			m.progress.SetMarquee(false)
			m.progress.Hwnd().ShowWindow(co.SW_HIDE)
			m.finishAsyncWork()
		})
	}()
	client, err := fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
	if err != nil {
		m.wnd.UiThread(func() {
			m.setStatus("Server configuration is invalid.")
			m.appendLog("Server configuration failed: %s", err)
		})
		return
	}
	session, err := client.Login(ctx)
	if err != nil {
		m.wnd.UiThread(func() {
			m.setStatus("Unable to authenticate with the server.")
			m.appendLog("Login failed: %s", err)
		})
		return
	}
	snapshot := client.DiscoverCapabilities(ctx, session)
	capabilityModel := capabilities.FromSnapshot(snapshot)
	environmentSnapshot := preflight.Run(snapshot, capabilityModel)
	path, err := client.SaveCapabilitySnapshot(snapshot, captureDirectory())
	envPath, envErr := preflight.Save(environmentSnapshot, captureDirectory())
	m.wnd.UiThread(func() {
		if err != nil {
			m.setStatus("Capability capture failed.")
			m.appendLog("Capture failed: %s", err)
			return
		}
		if envErr != nil {
			m.appendLog("Environment snapshot failed: %s", envErr)
		}
		m.capabilities = capabilityModel
		m.populateCapabilities(capabilityModel)
		m.showControls(m.capabilityCtrls, true)
		m.setStatus("Capability capture saved and UI populated from server response. Preflight: " + environmentSnapshot.OverallStatus)
		m.appendLog("Captured %d v4/v5 endpoint response(s)", len(snapshot.Endpoints))
		m.appendLog("Preflight checks: passed=%d failed=%d status=%s", environmentSnapshot.Summary.PassedChecks, environmentSnapshot.Summary.FailedChecks, environmentSnapshot.OverallStatus)
		m.appendLog("Saved capability snapshot: %s", path)
		if envPath != "" {
			m.appendLog("Saved environment snapshot: %s", envPath)
		}
	})
}

func captureDirectory() string {
	exe, err := os.Executable()
	if err != nil {
		return "captures"
	}
	return filepath.Join(filepath.Dir(exe), "captures")
}

func (m *MainWindow) populateCapabilities(model capabilities.Model) {
	queues := availableQueueNames(model.Queues)
	m.replaceComboItems(m.queue, queues, "No available queues")
	m.replaceOptionItems(m.pageSize, model, "PageSize")
	m.replaceOptionItems(m.resolution, model, "EFResolution")
	m.replaceOptionItems(m.colorMode, model, "EFColorMode")
	m.replaceOptionItems(m.mediaType, model, "EFMediaType")
	m.replaceOptionItems(m.printSpeed, model, "EFPrintSpeed")
	m.populateMultiSelect("PageSize", model)
	m.populateMultiSelect("EFResolution", model)
	m.populateMultiSelect("EFColorMode", model)
	m.populateMultiSelect("EFMediaType", model)
	m.populateMultiSelect("EFPrintSpeed", model)
	m.appendLog("Server: %s serial=%s version=%s", fallback(model.ServerName, "unknown"), fallback(model.SerialNumber, "unknown"), fallback(model.Version, "unknown"))
	m.appendLog("Loaded capabilities: queues=%d pageSizes=%d resolutions=%d colorModes=%d mediaTypes=%d printSpeeds=%d",
		len(queues), optionValueCount(model, "PageSize"), optionValueCount(model, "EFResolution"), optionValueCount(model, "EFColorMode"), optionValueCount(model, "EFMediaType"), optionValueCount(model, "EFPrintSpeed"))
}

func (m *MainWindow) replaceOptionItems(combo *ui.ComboBox, model capabilities.Model, optionID string) {
	option, ok := model.OptionByID(optionID)
	if !ok {
		m.replaceComboItems(combo, nil, "Not reported")
		return
	}
	m.replaceComboItems(combo, option.Values, "No values reported")
}

func (m *MainWindow) populateMultiSelect(optionID string, model capabilities.Model) {
	boxes := m.multiSelects[optionID]
	option, ok := model.OptionByID(optionID)
	m.multiSelectValues[optionID] = nil
	if !ok || len(option.Values) == 0 {
		for _, box := range boxes {
			box.Hwnd().SetWindowText("-")
			box.SetCheck(false)
		}
		return
	}
	values := option.Values
	if len(values) > len(boxes) {
		values = values[:len(boxes)]
	}
	m.multiSelectValues[optionID] = append([]string(nil), values...)
	for i, box := range boxes {
		if i >= len(values) {
			box.Hwnd().SetWindowText("-")
			box.SetCheck(false)
			continue
		}
		box.Hwnd().SetWindowText(shortLabel(values[i], 22))
		box.SetCheck(i == 0)
	}
}

func (m *MainWindow) expandComboDropDowns() {
	m.setComboDropDownHeight(m.selectionMode, 150)
	m.setComboDropDownHeight(m.queue, 190)
	m.setComboDropDownHeight(m.pageSize, 190)
	m.setComboDropDownHeight(m.resolution, 150)
	m.setComboDropDownHeight(m.colorMode, 150)
	m.setComboDropDownHeight(m.mediaType, 260)
	m.setComboDropDownHeight(m.printSpeed, 150)
	m.setComboDropDownHeight(m.runMode, 180)
	m.setComboDropDownHeight(m.strategy, 150)
}

func (m *MainWindow) setComboDropDownHeight(combo *ui.ComboBox, width int) {
	_ = combo.Hwnd().SetWindowPos(
		win.HWND(0),
		win.POINT{},
		win.SIZE{Cx: int32(ui.DpiX(width)), Cy: int32(ui.DpiY(220))},
		co.SWP_NOMOVE|co.SWP_NOZORDER|co.SWP_NOACTIVATE,
	)
}

func (m *MainWindow) replaceComboItems(combo *ui.ComboBox, items []string, emptyText string) {
	combo.DeleteAllItems()
	if len(items) == 0 {
		combo.AddItem(emptyText)
		combo.SelectIndex(0)
		return
	}
	combo.AddItem(items...)
	combo.SelectIndex(0)
}

func (m *MainWindow) selectedJobAttributes() map[string]string {
	attributes := map[string]string{}
	addSelectedAttribute(attributes, "PageSize", m.pageSize)
	addSelectedAttribute(attributes, "EFResolution", m.resolution)
	addSelectedAttribute(attributes, "EFColorMode", m.colorMode)
	addSelectedAttribute(attributes, "EFMediaType", m.mediaType)
	addSelectedAttribute(attributes, "EFPrintSpeed", m.printSpeed)
	return attributes
}

func (m *MainWindow) selectedCombinations(strategy combinations.Strategy, limit int) []combinations.Combination {
	if strategy == combinations.StrategySelected {
		combo := combinations.Combination{}
		for key, value := range m.selectedJobAttributes() {
			combo[key] = value
		}
		if len(combo) == 0 {
			return nil
		}
		return []combinations.Combination{combo}
	}
	axes := []combinations.Axis{
		m.multiSelectAxis("PageSize"),
		m.multiSelectAxis("EFResolution"),
		m.multiSelectAxis("EFColorMode"),
		m.multiSelectAxis("EFMediaType"),
		m.multiSelectAxis("EFPrintSpeed"),
	}
	return combinations.GenerateWithStrategy(axes, strategy, limit)
}

func (m *MainWindow) multiSelectAxis(optionID string) combinations.Axis {
	boxes := m.multiSelects[optionID]
	values := m.multiSelectValues[optionID]
	selected := make([]string, 0, len(values))
	for i, value := range values {
		if i < len(boxes) && boxes[i].IsChecked() {
			selected = append(selected, value)
		}
	}
	return combinations.Axis{Name: optionID, Values: selected}
}

func (m *MainWindow) selectedStrategy() combinations.Strategy {
	switch m.strategy.SelectedIndex() {
	case 1:
		return combinations.StrategyAll
	case 2:
		return combinations.StrategyPairwise
	case 3:
		return combinations.StrategyRandom
	default:
		return combinations.StrategySelected
	}
}

func (m *MainWindow) selectedMaxCases() int {
	limit, err := strconv.Atoi(strings.TrimSpace(m.maxCases.Text()))
	if err != nil || limit < 1 {
		return 100
	}
	return limit
}

func addSelectedAttribute(attributes map[string]string, name string, combo *ui.ComboBox) {
	value := strings.TrimSpace(combo.CurrentText())
	if value == "" || strings.Contains(value, "Capture") || strings.Contains(value, "Not reported") || strings.Contains(value, "No values") {
		return
	}
	attributes[name] = value
}

func attributesToAxis(attributes map[string]string) combinations.Axis {
	values := make([]string, 0, len(attributes))
	for key, value := range attributes {
		values = append(values, key+"="+value)
	}
	return combinations.Axis{Name: "SelectedAttributes", Values: values}
}

func availableQueueNames(queues []capabilities.Queue) []string {
	names := make([]string, 0, len(queues))
	for _, queue := range queues {
		if queue.Available {
			names = append(names, queue.Name)
		}
	}
	return names
}

func shortLabel(value string, max int) string {
	if len(value) <= max {
		return value
	}
	if max <= 1 {
		return value[:max]
	}
	return value[:max-1] + "…"
}

func optionValueCount(model capabilities.Model, optionID string) int {
	option, ok := model.OptionByID(optionID)
	if !ok {
		return 0
	}
	return len(option.Values)
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

type runMode struct {
	Label       string
	ImportQueue string
	Actions     []string
}

func (m *MainWindow) selectedRunMode() runMode { return runModeForIndex(m.runMode.SelectedIndex()) }

func runModeForIndex(index int) runMode {
	switch index {
	case 1:
		return runMode{Label: "Process and Hold", ImportQueue: "hold", Actions: []string{"rip"}}
	case 2:
		return runMode{Label: "RIP", ImportQueue: "hold", Actions: []string{"rip"}}
	case 3:
		return runMode{Label: "Press Print", ImportQueue: "hold", Actions: []string{"press_print"}}
	case 4:
		return runMode{Label: "Ready to Print", ImportQueue: "hold", Actions: []string{"press_print"}}
	case 5:
		return runMode{Label: "Print", ImportQueue: "print", Actions: []string{"print"}}
	default:
		return runMode{Label: "Hold", ImportQueue: "hold"}
	}
}

type testJob struct {
	File       string
	Attributes map[string]string
}

func (m *MainWindow) runFieryAutomation(ctx context.Context, server model.ServerConnection, selectedFiles []string, workers int, combos []combinations.Combination, mode runMode) {
	defer func() {
		m.wnd.UiThread(func() { m.finishAsyncWork() })
	}()

	client, err := fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
	if err != nil {
		m.wnd.UiThread(func() {
			m.setStatus("Server configuration is invalid.")
			m.appendLog("Server configuration failed: %s", err)
		})
		return
	}

	session, err := client.Login(ctx)
	if err != nil {
		m.wnd.UiThread(func() {
			m.setStatus("Unable to authenticate with the server.")
			m.appendLog("Login failed: %s", err)
		})
		return
	}
	m.wnd.UiThread(func() {
		m.setStatus("Connected. Importing selected test files...")
		m.appendLog("Login successful; session cookie received")
	})

	if err := client.KeepAlive(ctx, session); err != nil {
		m.wnd.UiThread(func() { m.appendLog("Keep-alive check failed: %s", err) })
	}

	jobs := make(chan testJob)
	results := make(chan model.Result)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				result, err := client.ImportJobToQueue(ctx, session, job.File, mode.ImportQueue)
				verification := attributeVerification{Passed: len(job.Attributes) == 0, Mode: mode.Label}
				if err == nil && result.JobID != "" {
					if len(job.Attributes) > 0 {
						err = client.UpdateJobAttributes(ctx, session, result.JobID, job.Attributes)
					}
					if err == nil {
						err = runJobActions(ctx, client, session, result.JobID, mode)
					}
					if err == nil && len(job.Attributes) > 0 {
						actual, getErr := client.GetJobAttributes(ctx, session, result.JobID)
						if getErr != nil {
							err = getErr
						} else {
							verification = verifyAttributes(job.Attributes, actual)
							verification.Mode = mode.Label
						}
					}
				}
				res := importResultToModel(result, err, verification)
				select {
				case results <- res:
				case <-ctx.Done():
					return
				}
			}
		}()
	}
	go func() {
		defer close(jobs)
		if len(combos) == 0 {
			combos = []combinations.Combination{{}}
		}
		for _, file := range selectedFiles {
			for _, combo := range combos {
				select {
				case jobs <- testJob{File: file, Attributes: combinationToAttributes(combo)}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	count, passed, failed, errored := 0, 0, 0, 0
	for res := range results {
		count++
		switch {
		case res.Error != "":
			errored++
		case strings.HasPrefix(res.BodyPreview, "PASS:"):
			passed++
		case strings.HasPrefix(res.BodyPreview, "FAIL:"):
			failed++
		}
		res := res
		m.wnd.UiThread(func() { m.addResult(res) })
	}
	m.wnd.UiThread(func() {
		if ctx.Err() != nil {
			m.setStatus(fmt.Sprintf("Cancelled after %d result(s).", count))
			m.appendLog("Automation cancelled")
			return
		}
		m.setStatus(fmt.Sprintf("Completed. total=%d pass=%d fail=%d error=%d", count, passed, failed, errored))
		m.appendLog("Server automation finished: total=%d pass=%d fail=%d error=%d", count, passed, failed, errored)
	})
}

func combinationToAttributes(combo combinations.Combination) map[string]string {
	attrs := make(map[string]string, len(combo))
	for key, value := range combo {
		attrs[key] = value
	}
	return attrs
}

func runJobActions(ctx context.Context, client *fiery.Client, session fiery.Session, jobID string, mode runMode) error {
	for _, action := range mode.Actions {
		if err := client.JobAction(ctx, session, jobID, action); err != nil {
			return err
		}
	}
	return nil
}

type attributeVerification struct {
	Passed   bool
	Mode     string
	Expected map[string]string
	Actual   map[string]string
	Failures []string
}

func verifyAttributes(expected, actual map[string]string) attributeVerification {
	verification := attributeVerification{Passed: true, Expected: expected, Actual: actual}
	for key, expectedValue := range expected {
		actualValue, ok := actual[key]
		if !ok || strings.TrimSpace(actualValue) != strings.TrimSpace(expectedValue) {
			verification.Passed = false
			verification.Failures = append(verification.Failures, fmt.Sprintf("%s expected=%q actual=%q", key, expectedValue, actualValue))
		}
	}
	return verification
}

func importResultToModel(result fiery.ImportResult, err error, verification attributeVerification) model.Result {
	res := model.Result{
		RequestID:   result.JobID,
		RequestName: filepath.Base(result.FilePath),
		Method:      http.MethodPost,
		URL:         "/live/api/v5/jobs",
		StatusCode:  result.StatusCode,
		Duration:    result.Duration,
		CompletedAt: time.Now(),
	}
	if result.JobID != "" {
		if verification.Passed {
			res.BodyPreview = "PASS: mode=" + verification.Mode + "; set values match get values for job " + result.JobID
		} else {
			res.BodyPreview = "FAIL: mode=" + verification.Mode + "; " + strings.Join(verification.Failures, "; ")
		}
	}
	if err != nil {
		res.Error = err.Error()
	}
	return res
}

func (m *MainWindow) addResult(res model.Result) {
	status := strconv.Itoa(res.StatusCode)
	if strings.HasPrefix(res.BodyPreview, "PASS:") {
		status = "PASS"
	}
	if strings.HasPrefix(res.BodyPreview, "FAIL:") {
		status = "FAIL"
	}
	if res.Error != "" {
		status = "ERR"
	}
	m.results.AddItem(res.RequestName, res.Method, status, res.Duration.Round(time.Millisecond).String(), res.URL)
	if res.Error != "" {
		m.appendLog("%s failed: %s", res.RequestName, res.Error)
		return
	}
	if res.BodyPreview != "" {
		m.appendLog("%s", res.BodyPreview)
	}
	m.appendLog("%s %s -> %s in %s", res.Method, res.URL, status, res.Duration.Round(time.Millisecond))
}

func (m *MainWindow) selectedFileMode() model.FileSelectionMode {
	switch m.selectionMode.SelectedIndex() {
	case 1:
		return model.FileSelectionSingle
	case 2:
		return model.FileSelectionRandom
	default:
		return model.FileSelectionAll
	}
}

func browsePath(owner win.HWND, folder bool) (string, error) {
	releaser := win.NewOleReleaser()
	defer releaser.Release()

	var dialog *win.IFileOpenDialog
	if err := win.CoCreateInstance(releaser, &co.CLSID_FileOpenDialog, nil, co.CLSCTX_INPROC_SERVER, &dialog); err != nil {
		return "", err
	}

	options, err := dialog.GetOptions()
	if err != nil {
		return "", err
	}
	options |= co.FOS_FORCEFILESYSTEM | co.FOS_PATHMUSTEXIST
	if folder {
		options |= co.FOS_PICKFOLDERS
	} else {
		options |= co.FOS_FILEMUSTEXIST
	}
	if err := dialog.SetOptions(options); err != nil {
		return "", err
	}

	ok, err := dialog.Show(owner)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", nil
	}
	item, err := dialog.GetResult(releaser)
	if err != nil {
		return "", err
	}
	path, err := item.GetDisplayName(co.SIGDN_FILESYSPATH)
	if err != nil {
		return "", err
	}
	if path == "" {
		return "", errors.New("no path selected")
	}
	return path, nil
}

func (m *MainWindow) setStatus(text string) { m.status.Hwnd().SetWindowText(text) }

func (m *MainWindow) appendLog(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	current := m.log.Text()
	if current != "" {
		current += "\r\n"
	}
	m.log.SetText(current + time.Now().Format("15:04:05") + "  " + line)
}
