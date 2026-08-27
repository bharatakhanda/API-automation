//go:build windows

package app

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"api-automation/internal/engine"
	"api-automation/internal/model"
	"api-automation/internal/runner"

	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/ui"
)

type MainWindow struct {
	wnd          *ui.Main
	url          *ui.Edit
	method       *ui.Edit
	concurrency  *ui.Edit
	runButton    *ui.Button
	cancelButton *ui.Button
	results      *ui.ListView
	log          *ui.Edit
	status       *ui.Static

	runner  *runner.Runner
	cancel  context.CancelFunc
	running atomic.Bool
}

func Run() int {
	exec := engine.NewExecutor()
	wnd := ui.NewMain(ui.OptsMain().Title("API Automation").Size(ui.Dpi(1180, 760)))

	// Enterprise shell: persistent navigation on the left, work area on the right.
	ui.NewStatic(wnd, ui.OptsStatic().Text("API Automation").Position(ui.Dpi(18, 18)).Size(ui.Dpi(150, 22)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Workspace").Position(ui.Dpi(18, 58)).Size(ui.Dpi(150, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Requests").Position(ui.Dpi(30, 88)).Size(ui.Dpi(130, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Collections").Position(ui.Dpi(30, 116)).Size(ui.Dpi(130, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Environments").Position(ui.Dpi(30, 144)).Size(ui.Dpi(130, 20)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("› Run History").Position(ui.Dpi(30, 172)).Size(ui.Dpi(130, 20)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("Request Builder").Position(ui.Dpi(200, 18)).Size(ui.Dpi(180, 24)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Create, execute, and inspect API requests from a single controlled workspace.").Position(ui.Dpi(200, 44)).Size(ui.Dpi(620, 22)))

	runButton := ui.NewButton(wnd, ui.OptsButton().Text("&Run request").Position(ui.Dpi(910, 24)).Width(ui.DpiX(110)))
	cancelButton := ui.NewButton(wnd, ui.OptsButton().Text("&Cancel").Position(ui.Dpi(1030, 24)).Width(ui.DpiX(90)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("REQUEST").Position(ui.Dpi(200, 88)).Size(ui.Dpi(120, 18)))
	ui.NewStatic(wnd, ui.OptsStatic().Text("Method").Position(ui.Dpi(200, 118)).Size(ui.Dpi(70, 20)))
	method := ui.NewEdit(wnd, ui.OptsEdit().Text(http.MethodGet).Position(ui.Dpi(200, 142)).Width(ui.DpiX(92)).Height(ui.DpiY(26)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("Endpoint URL").Position(ui.Dpi(312, 118)).Size(ui.Dpi(120, 20)))
	url := ui.NewEdit(wnd, ui.OptsEdit().Text("https://httpbin.org/get").Position(ui.Dpi(312, 142)).Width(ui.DpiX(610)).Height(ui.DpiY(26)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("Workers").Position(ui.Dpi(944, 118)).Size(ui.Dpi(90, 20)))
	concurrency := ui.NewEdit(wnd, ui.OptsEdit().Text("1").Position(ui.Dpi(944, 142)).Width(ui.DpiX(76)).Height(ui.DpiY(26)))

	status := ui.NewStatic(wnd, ui.OptsStatic().Text("Ready").Position(ui.Dpi(200, 184)).Size(ui.Dpi(920, 22)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("EXECUTION RESULTS").Position(ui.Dpi(200, 222)).Size(ui.Dpi(160, 18)))
	results := ui.NewListView(wnd, ui.OptsListView().
		Position(ui.Dpi(200, 248)).Size(ui.Dpi(920, 300)).
		CtrlExStyle(co.LVS_EX_FULLROWSELECT|co.LVS_EX_GRIDLINES).
		Column("Request", ui.DpiX(190)).Column("Method", ui.DpiX(90)).Column("Status", ui.DpiX(90)).Column("Duration", ui.DpiX(120)).Column("URL", ui.DpiX(420)))

	ui.NewStatic(wnd, ui.OptsStatic().Text("ACTIVITY LOG").Position(ui.Dpi(200, 572)).Size(ui.Dpi(160, 18)))
	log := ui.NewEdit(wnd, ui.OptsEdit().Position(ui.Dpi(200, 598)).Width(ui.DpiX(920)).Height(ui.DpiY(100)).
		CtrlStyle(co.ES_MULTILINE|co.ES_AUTOVSCROLL|co.ES_READONLY|co.ES_WANTRETURN).
		WndStyle(co.WS_CHILD|co.WS_VISIBLE|co.WS_VSCROLL|co.WS_TABSTOP))

	mw := &MainWindow{wnd: wnd, url: url, method: method, concurrency: concurrency, runButton: runButton, cancelButton: cancelButton, results: results, log: log, status: status, runner: runner.New(exec)}
	mw.events()
	return wnd.RunAsMain()
}

func (m *MainWindow) events() {
	m.runButton.On().BnClicked(func() { m.startRun() })
	m.cancelButton.On().BnClicked(func() { m.cancelRun() })
	m.wnd.On().WmClose(func() { m.cancelRun() })
}

func (m *MainWindow) startRun() {
	if m.running.Load() {
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
	m.setStatus("Running...")
	m.appendLog("Starting workflow with %d worker(s)", workers)

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

func (m *MainWindow) setStatus(text string) { m.status.Hwnd().SetWindowText(text) }

func (m *MainWindow) appendLog(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	current := m.log.Text()
	if current != "" {
		current += "\r\n"
	}
	m.log.SetText(current + time.Now().Format("15:04:05") + "  " + line)
}
