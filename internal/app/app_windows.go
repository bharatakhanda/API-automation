//go:build windows

package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"api-automation/internal/engine"
	"api-automation/internal/files"
	"api-automation/internal/model"
	"api-automation/internal/runner"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

type MainWindow struct {
	wnd           *ui.Main
	serverIP      *ui.Edit
	secretKey     *ui.Edit
	folderPath    *ui.Edit
	filePath      *ui.Edit
	selectionMode *ui.ComboBox
	url           *ui.Edit
	method        *ui.Edit
	concurrency   *ui.Edit
	runButton     *ui.Button
	cancelButton  *ui.Button
	browseFolder  *ui.Button
	browseFile    *ui.Button
	results       *ui.ListView
	log           *ui.Edit
	status        *ui.Static

	runner  *runner.Runner
	cancel  context.CancelFunc
	running atomic.Bool
}

func Run() int {
	_, _ = win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
	defer win.CoUninitialize()

	exec := engine.NewExecutor()
	wnd := ui.NewMain(ui.OptsMain().Title("API Automation").Size(ui.Dpi(1240, 820)))

	// Enterprise shell: persistent navigation on the left, work area on the right.
	ui.NewStatic(wnd, ui.OptsStatic().Text("API Automation").Position(ui.Dpi(18, 18)).Size(ui.Dpi(150, 22)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Workspace").Position(ui.Dpi(18, 58)).Size(ui.Dpi(150, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Server").Position(ui.Dpi(30, 88)).Size(ui.Dpi(130, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Test Files").Position(ui.Dpi(30, 116)).Size(ui.Dpi(130, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Requests").Position(ui.Dpi(30, 144)).Size(ui.Dpi(130, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Run History").Position(ui.Dpi(30, 172)).Size(ui.Dpi(130, 20)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("Server Execution Workspace").Position(ui.Dpi(200, 18)).Size(ui.Dpi(260, 24)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Connect securely, choose test assets, and execute API automation against the server.").Position(ui.Dpi(200, 44)).Size(ui.Dpi(680, 22)))

	runButton := ui.NewButton(wnd, ui.OptsButton().Text("&Run automation").Position(ui.Dpi(930, 24)).Width(ui.DpiX(130)))
	cancelButton := ui.NewButton(wnd, ui.OptsButton().Text("&Cancel").Position(ui.Dpi(1070, 24)).Width(ui.DpiX(90)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("SERVER CONNECTION").Position(ui.Dpi(200, 88)).Size(ui.Dpi(180, 18)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Server IP address").Position(ui.Dpi(200, 118)).Size(ui.Dpi(140, 20)))
	serverIP := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(200, 142)).Width(ui.DpiX(250)).Height(ui.DpiY(26)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Secret key").Position(ui.Dpi(470, 118)).Size(ui.Dpi(120, 20)))
	secretKey := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(470, 142)).Width(ui.DpiX(300)).Height(ui.DpiY(26)).CtrlStyle(co.ES_PASSWORD|co.ES_AUTOHSCROLL|co.ES_NOHIDESEL))

	ui.NewStatic(wnd, ui.OptsStatic().Text("TEST FILES").Position(ui.Dpi(200, 194)).Size(ui.Dpi(160, 18)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Folder").Position(ui.Dpi(200, 224)).Size(ui.Dpi(90, 20)))
	folderPath := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(200, 248)).Width(ui.DpiX(650)).Height(ui.DpiY(26)))
	browseFolder := ui.NewButton(wnd, ui.OptsButton().Text("Browse...").Position(ui.Dpi(862, 247)).Width(ui.DpiX(92)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("Selection").Position(ui.Dpi(200, 288)).Size(ui.Dpi(90, 20)))
	selectionMode := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(200, 312)).Width(ui.DpiX(150)).Texts("All files", "Single file", "Random file").Select(0))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Specific file (used only for Single file)").Position(ui.Dpi(372, 288)).Size(ui.Dpi(260, 20)))
	filePath := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(372, 312)).Width(ui.DpiX(478)).Height(ui.DpiY(26)))
	browseFile := ui.NewButton(wnd, ui.OptsButton().Text("Browse...").Position(ui.Dpi(862, 311)).Width(ui.DpiX(92)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("REQUEST").Position(ui.Dpi(200, 364)).Size(ui.Dpi(120, 18)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Method").Position(ui.Dpi(200, 394)).Size(ui.Dpi(70, 20)))
	method := ui.NewEdit(wnd, ui.OptsEdit().Text(http.MethodGet).Position(ui.Dpi(200, 418)).Width(ui.DpiX(92)).Height(ui.DpiY(26)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Endpoint URL").Position(ui.Dpi(312, 394)).Size(ui.Dpi(120, 20)))
	url := ui.NewEdit(wnd, ui.OptsEdit().Text("https://httpbin.org/get").Position(ui.Dpi(312, 418)).Width(ui.DpiX(610)).Height(ui.DpiY(26)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Workers").Position(ui.Dpi(944, 394)).Size(ui.Dpi(90, 20)))
	concurrency := ui.NewEdit(wnd, ui.OptsEdit().Text("1").Position(ui.Dpi(944, 418)).Width(ui.DpiX(76)).Height(ui.DpiY(26)))

	status := ui.NewStatic(wnd, ui.OptsStatic().Text("Ready. Provide server IP, secret key, and a test file folder.").Position(ui.Dpi(200, 464)).Size(ui.Dpi(940, 22)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("EXECUTION RESULTS").Position(ui.Dpi(200, 500)).Size(ui.Dpi(160, 18)))
	results := ui.NewListView(wnd, ui.OptsListView().
		Position(ui.Dpi(200, 526)).Size(ui.Dpi(940, 170)).
		CtrlExStyle(co.LVS_EX_FULLROWSELECT|co.LVS_EX_GRIDLINES).
		Column("Request", ui.DpiX(190)).Column("Method", ui.DpiX(90)).Column("Status", ui.DpiX(90)).Column("Duration", ui.DpiX(120)).Column("URL", ui.DpiX(420)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("ACTIVITY LOG").Position(ui.Dpi(200, 716)).Size(ui.Dpi(160, 18)))
	log := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(200, 742)).Width(ui.DpiX(940)).Height(ui.DpiY(42)).
		CtrlStyle(co.ES_MULTILINE|co.ES_AUTOVSCROLL|co.ES_READONLY|co.ES_WANTRETURN).
		WndStyle(co.WS_CHILD|co.WS_VISIBLE|co.WS_VSCROLL|co.WS_TABSTOP))

	mw := &MainWindow{wnd: wnd, serverIP: serverIP, secretKey: secretKey, folderPath: folderPath, filePath: filePath, selectionMode: selectionMode, url: url, method: method, concurrency: concurrency, runButton: runButton, cancelButton: cancelButton, browseFolder: browseFolder, browseFile: browseFile, results: results, log: log, status: status, runner: runner.New(exec)}
	mw.events()
	return wnd.RunAsMain()
}

func (m *MainWindow) events() {
	m.runButton.On().BnClicked(func() { m.startRun() })
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
	m.wnd.On().WmClose(func() { m.cancelRun() })
}

func (m *MainWindow) startRun() {
	if m.running.Load() {
		return
	}
	server := model.ServerConnection{
		IPAddress: strings.TrimSpace(m.serverIP.Text()),
		SecretKey: strings.TrimSpace(m.secretKey.Text()),
	}
	if server.IPAddress == "" || server.SecretKey == "" {
		m.setStatus("Server IP address and secret key are required.")
		m.appendLog("Validation failed: server IP address and secret key are required")
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

	url := strings.TrimSpace(m.url.Text())
	method := strings.ToUpper(strings.TrimSpace(m.method.Text()))
	workers, err := strconv.Atoi(strings.TrimSpace(m.concurrency.Text()))
	if err != nil || workers < 1 {
		workers = 1
	}

	workflow := model.Workflow{Name: "Ad hoc request", Requests: []model.Request{{ID: "adhoc-1", Name: "Ad hoc", Method: method, URL: url, Timeout: 60 * time.Second}}}
	ctx, cancel := context.WithCancel(context.Background())
	m.cancel = cancel
	m.running.Store(true)
	m.results.DeleteAllItems()
	m.setStatus("Running automation against server " + server.IPAddress + "...")
	m.appendLog("Starting workflow with %d worker(s), %d selected test file(s)", workers, len(selectedFiles))

	go func() {
		count := 0
		for res := range m.runner.Run(ctx, workflow, runner.Options{Concurrency: workers}) {
			count++
			res := res
			m.wnd.UiThread(func() { m.addResult(res) })
		}
		m.wnd.UiThread(func() {
			m.running.Store(false)
			m.setStatus(fmt.Sprintf("Completed. %d result(s).", count))
			m.appendLog("Workflow finished")
		})
	}()
}

func (m *MainWindow) cancelRun() {
	if m.cancel != nil {
		m.cancel()
		m.appendLog("Cancellation requested")
	}
}

func (m *MainWindow) addResult(res model.Result) {
	status := strconv.Itoa(res.StatusCode)
	if res.Error != "" {
		status = "ERR"
	}
	m.results.AddItem(res.RequestName, res.Method, status, res.Duration.Round(time.Millisecond).String(), res.URL)
	if res.Error != "" {
		m.appendLog("%s failed: %s", res.RequestName, res.Error)
		return
	}
	m.appendLog("%s %s -> %d in %s", res.Method, res.URL, res.StatusCode, res.Duration.Round(time.Millisecond))
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
