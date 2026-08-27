//go:build windows

package app

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"api-automation/internal/fiery"
	"api-automation/internal/files"
	"api-automation/internal/model"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
	"github.com/rodrigocfd/windigo/win"
)

type MainWindow struct {
	wnd           *ui.Main
	serverIP      *ui.Edit
	secretKey     *ui.Edit
	password      *ui.Edit
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

	cancel  context.CancelFunc
	running atomic.Bool
}

func Run() int {
	_, _ = win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
	defer win.CoUninitialize()

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
	ui.NewStatic(wnd, ui.OptsStatic().Text("Admin password").Position(ui.Dpi(790, 118)).Size(ui.Dpi(130, 20)))
	password := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(790, 142)).Width(ui.DpiX(164)).Height(ui.DpiY(26)).CtrlStyle(co.ES_PASSWORD|co.ES_AUTOHSCROLL|co.ES_NOHIDESEL))

	ui.NewStatic(wnd, ui.OptsStatic().Text("TEST FILES").Position(ui.Dpi(200, 194)).Size(ui.Dpi(160, 18)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Folder").Position(ui.Dpi(200, 224)).Size(ui.Dpi(90, 20)))
	folderPath := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(200, 248)).Width(ui.DpiX(650)).Height(ui.DpiY(26)))
	browseFolder := ui.NewButton(wnd, ui.OptsButton().Text("Browse...").Position(ui.Dpi(862, 247)).Width(ui.DpiX(92)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("Selection").Position(ui.Dpi(200, 288)).Size(ui.Dpi(90, 20)))
	selectionMode := ui.NewComboBox(wnd, ui.OptsComboBox().Position(ui.Dpi(200, 312)).Width(ui.DpiX(150)).Texts("All files", "Single file", "Random file").Select(0))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Specific file (used only for Single file)").Position(ui.Dpi(372, 288)).Size(ui.Dpi(260, 20)))
	filePath := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(372, 312)).Width(ui.DpiX(478)).Height(ui.DpiY(26)))
	browseFile := ui.NewButton(wnd, ui.OptsButton().Text("Browse...").Position(ui.Dpi(862, 311)).Width(ui.DpiX(92)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("FIERY IMPORT").Position(ui.Dpi(200, 364)).Size(ui.Dpi(120, 18)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Method").Position(ui.Dpi(200, 394)).Size(ui.Dpi(70, 20)))
	method := ui.NewEdit(wnd, ui.OptsEdit().Text(http.MethodPost).Position(ui.Dpi(200, 418)).Width(ui.DpiX(92)).Height(ui.DpiY(26)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Import endpoint").Position(ui.Dpi(312, 394)).Size(ui.Dpi(120, 20)))
	url := ui.NewEdit(wnd, ui.OptsEdit().Text("/live/api/v5/jobs").Position(ui.Dpi(312, 418)).Width(ui.DpiX(610)).Height(ui.DpiY(26)))
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

	mw := &MainWindow{wnd: wnd, serverIP: serverIP, secretKey: secretKey, password: password, folderPath: folderPath, filePath: filePath, selectionMode: selectionMode, url: url, method: method, concurrency: concurrency, runButton: runButton, cancelButton: cancelButton, browseFolder: browseFolder, browseFile: browseFile, results: results, log: log, status: status}
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
		Password:  strings.TrimSpace(m.password.Text()),
	}
	if server.IPAddress == "" || server.SecretKey == "" || server.Password == "" {
		m.setStatus("Server IP address, secret key, and admin password are required.")
		m.appendLog("Validation failed: server IP address, secret key, and admin password are required")
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
	m.results.DeleteAllItems()
	m.setStatus("Connecting to Fiery server " + server.IPAddress + "...")
	m.appendLog("Starting server automation with %d worker(s), %d selected test file(s)", workers, len(selectedFiles))

	go m.runFieryAutomation(ctx, server, selectedFiles, workers)
}

func (m *MainWindow) cancelRun() {
	if m.cancel != nil {
		m.cancel()
		m.appendLog("Cancellation requested")
	}
}

func (m *MainWindow) runFieryAutomation(ctx context.Context, server model.ServerConnection, selectedFiles []string, workers int) {
	defer func() {
		m.wnd.UiThread(func() { m.running.Store(false) })
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

	jobs := make(chan string)
	results := make(chan model.Result)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for file := range jobs {
				result, err := client.ImportJob(ctx, session, file)
				res := importResultToModel(result, err)
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
		for _, file := range selectedFiles {
			select {
			case jobs <- file:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	count := 0
	for res := range results {
		count++
		res := res
		m.wnd.UiThread(func() { m.addResult(res) })
	}
	m.wnd.UiThread(func() {
		if ctx.Err() != nil {
			m.setStatus(fmt.Sprintf("Cancelled after %d result(s).", count))
			m.appendLog("Automation cancelled")
			return
		}
		m.setStatus(fmt.Sprintf("Completed. %d file import result(s).", count))
		m.appendLog("Server automation finished")
	})
}

func importResultToModel(result fiery.ImportResult, err error) model.Result {
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
		res.BodyPreview = "Imported job " + result.JobID
	}
	if err != nil {
		res.Error = err.Error()
	}
	return res
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
