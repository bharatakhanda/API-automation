//go:build windows

package appgio

import (
	"context"
	"fmt"
	"image"
	"image/color"
	"net/http"
	"path/filepath"
	"runtime/debug"
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

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

var palette = struct {
	bg, surface, surfaceAlt, navy, text, muted, border, primary, primaryDim, danger, success color.NRGBA
}{
	bg:         rgb(0xf5f7fb),
	surface:    rgb(0xffffff),
	surfaceAlt: rgb(0xf8fafc),
	navy:       rgb(0x0f172a),
	text:       rgb(0x172033),
	muted:      rgb(0x64748b),
	border:     rgb(0xdce3ed),
	primary:    rgb(0x2563eb),
	primaryDim: rgb(0xdbeafe),
	danger:     rgb(0xb91c1c),
	success:    rgb(0x15803d),
}

type Window struct {
	window *app.Window
	theme  *material.Theme
	ops    op.Ops
	list   widget.List

	serverIP, secretKey, password widget.Editor
	folderPath, filePath          widget.Editor
	endpoint, workers, maxCases   widget.Editor

	settingsButton, captureButton, runButton, cancelButton widget.Clickable
	allFilesButton, singleFileButton, randomFileButton     widget.Clickable
	selectedOnlyButton, allPermButton, pairwiseButton      widget.Clickable
	modeButtons                                            []widget.Clickable

	selectionMode int
	strategy      combinations.Strategy
	runModeIndex  int

	capabilities capabilities.Model
	selected     map[string]map[string]*widget.Bool
	log          []string
	results      []resultRow
	status       string
	running      atomic.Bool
	cancel       context.CancelFunc
	diagnostic   *diagnosticLog
}

type resultRow struct{ File, Method, Status, Duration, Detail string }

type runMode struct {
	Label, ImportQueue string
	Actions            []string
}

var runModes = []runMode{
	{Label: "Hold", ImportQueue: "hold"},
	{Label: "Process and Hold", ImportQueue: "hold", Actions: []string{"rip"}},
	{Label: "RIP", ImportQueue: "hold", Actions: []string{"rip"}},
	{Label: "Press Print", ImportQueue: "hold", Actions: []string{"press_print"}},
	{Label: "Ready to Print", ImportQueue: "hold", Actions: []string{"press_print"}},
	{Label: "Print", ImportQueue: "print", Actions: []string{"print"}},
}

func New() *Window {
	w := &Window{window: new(app.Window), theme: material.NewTheme(), selected: map[string]map[string]*widget.Bool{}, strategy: combinations.StrategySelected, status: "Ready · Open Settings, discover capabilities, then run automation.", diagnostic: newDiagnosticLog()}
	w.theme.Palette = material.Palette{Bg: palette.bg, Fg: palette.text, ContrastBg: palette.primary, ContrastFg: rgb(0xffffff)}
	w.theme.TextSize = 15
	w.list.Axis = layout.Vertical
	initEditor(&w.serverIP, "")
	initEditor(&w.secretKey, fiery.DefaultSecretKey)
	initEditor(&w.password, "")
	initEditor(&w.folderPath, "")
	initEditor(&w.filePath, "")
	initEditor(&w.endpoint, "/live/api/v5/jobs")
	initEditor(&w.workers, "1")
	initEditor(&w.maxCases, "100")
	w.window.Option(app.Title("API Automation"), app.Size(unit.Dp(1240), unit.Dp(900)), app.MinSize(unit.Dp(1100), unit.Dp(760)))
	return w
}

func initEditor(e *widget.Editor, text string) { e.SingleLine = true; e.Submit = true; e.SetText(text) }

func Run() int {
	code := make(chan int, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				_ = writeCrashReport(fmt.Sprintf("panic: %v", r), debug.Stack())
				code <- 1
			}
		}()
		if err := New().Run(); err != nil {
			_ = writeCrashReport(err.Error(), nil)
			code <- 1
			return
		}
		code <- 0
	}()
	app.Main()
	return <-code
}

func (w *Window) Run() error {
	defer w.diagnostic.close()
	w.diagnostic.printf("Application started. Diagnostic log: %s", w.diagnostic.path)
	for {
		e := w.window.Event()
		switch e := e.(type) {
		case app.DestroyEvent:
			if w.cancel != nil {
				w.cancel()
			}
			return e.Err
		case app.FrameEvent:
			gtx := app.NewContext(&w.ops, e)
			w.handleClicks(gtx)
			w.layout(gtx)
			e.Frame(gtx.Ops)
		}
	}
}

func (w *Window) handleClicks(gtx layout.Context) {
	for w.captureButton.Clicked(gtx) {
		w.captureCapabilities()
	}
	for w.runButton.Clicked(gtx) {
		w.startRun()
	}
	for w.cancelButton.Clicked(gtx) {
		if w.cancel != nil {
			w.cancel()
			w.addLog("Cancellation requested")
		}
	}
	for w.allFilesButton.Clicked(gtx) {
		w.selectionMode = 0
	}
	for w.singleFileButton.Clicked(gtx) {
		w.selectionMode = 1
	}
	for w.randomFileButton.Clicked(gtx) {
		w.selectionMode = 2
	}
	for w.selectedOnlyButton.Clicked(gtx) {
		w.strategy = combinations.StrategySelected
	}
	for w.allPermButton.Clicked(gtx) {
		w.strategy = combinations.StrategyAll
	}
	for w.pairwiseButton.Clicked(gtx) {
		w.strategy = combinations.StrategyPairwise
	}
	for i := range w.modeButtons {
		for w.modeButtons[i].Clicked(gtx) {
			w.runModeIndex = i
		}
	}
}

func (w *Window) layout(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, palette.bg)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.sidebar(gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(24), Right: unit.Dp(28), Bottom: unit.Dp(20), Left: unit.Dp(24)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return w.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions { return w.content(gtx) })
			})
		}),
	)
}

func (w *Window) sidebar(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(unit.Dp(190)), gtx.Dp(unit.Dp(190))
	paint.FillShape(gtx.Ops, palette.navy, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Inset{Top: unit.Dp(24), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(w.theme, "API Automation", 20, rgb(0xffffff)).Layout),
			layout.Rigid(spacer(26)),
			layout.Rigid(label(w.theme, "Workspace", 13, rgb(0x93a4bd)).Layout),
			layout.Rigid(spacer(14)),
			layout.Rigid(navItem(w.theme, "Server")), layout.Rigid(navItem(w.theme, "Test files")), layout.Rigid(navItem(w.theme, "Capabilities")), layout.Rigid(navItem(w.theme, "Run history")),
		)
	})
}

func (w *Window) content(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(w.header), layout.Rigid(spacer(18)), layout.Rigid(w.settingsCard), layout.Rigid(spacer(14)), layout.Rigid(w.assetsCard), layout.Rigid(spacer(14)), layout.Rigid(w.capabilitiesCard), layout.Rigid(spacer(14)), layout.Rigid(w.resultsCard), layout.Rigid(spacer(20)),
	)
}

func (w *Window) header(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(label(w.theme, "Server Execution Workspace", 24, palette.text).Layout), layout.Rigid(label(w.theme, "Discover Fiery capabilities, select job options, and run API automation.", 14, palette.muted).Layout))
		}),
		layout.Rigid(primaryButton(w.theme, &w.captureButton, "Get server capabilities")), layout.Rigid(spacerX(10)), layout.Rigid(primaryButton(w.theme, &w.runButton, "Run automation")), layout.Rigid(spacerX(10)), layout.Rigid(secondaryButton(w.theme, &w.cancelButton, "Cancel")),
	)
}

func (w *Window) settingsCard(gtx layout.Context) layout.Dimensions {
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(sectionTitle(w.theme, "01 Settings · Server connection")), layout.Rigid(spacer(12)), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return row(gtx, field(w.theme, "Server IP", &w.serverIP, 220), field(w.theme, "Secret key", &w.secretKey, 420), field(w.theme, "Admin password", &w.password, 220))
		}))
	})
}
func (w *Window) assetsCard(gtx layout.Context) layout.Dimensions {
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(sectionTitle(w.theme, "02 Test assets and run setup")), layout.Rigid(spacer(12)), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return row(gtx, field(w.theme, "Folder path", &w.folderPath, 520), field(w.theme, "Specific file", &w.filePath, 360), field(w.theme, "Workers", &w.workers, 90))
		}), layout.Rigid(spacer(12)), layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return row(gtx, toggle(w.theme, &w.allFilesButton, "All files", w.selectionMode == 0), toggle(w.theme, &w.singleFileButton, "Single file", w.selectionMode == 1), toggle(w.theme, &w.randomFileButton, "Random file", w.selectionMode == 2), field(w.theme, "Endpoint", &w.endpoint, 260))
		}), layout.Rigid(spacer(12)), layout.Rigid(w.modeSelector))
	})
}

func (w *Window) modeSelector(gtx layout.Context) layout.Dimensions {
	children := []layout.FlexChild{layout.Rigid(label(w.theme, "Run mode", 14, palette.muted).Layout), layout.Rigid(spacerX(10))}
	if len(w.modeButtons) != len(runModes) {
		w.modeButtons = make([]widget.Clickable, len(runModes))
	}
	for i := range runModes {
		idx := i
		children = append(children, layout.Rigid(toggle(w.theme, &w.modeButtons[idx], runModes[idx].Label, w.runModeIndex == idx)), layout.Rigid(spacerX(8)))
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx, children...)
}

func (w *Window) capabilitiesCard(gtx layout.Context) layout.Dimensions {
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		if len(w.capabilities.Options) == 0 && len(w.capabilities.Queues) == 0 {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(sectionTitle(w.theme, "03 Server capabilities")), layout.Rigid(spacer(10)), layout.Rigid(label(w.theme, "Click Get server capabilities. Options will appear here after the server responds.", 14, palette.muted).Layout))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(sectionTitle(w.theme, fmt.Sprintf("03 Server capabilities · %s", fallback(w.capabilities.ServerName, "discovered")))), layout.Rigid(spacer(10)), layout.Rigid(w.strategySelector), layout.Rigid(spacer(12)), layout.Rigid(w.optionGrid))
	})
}

func (w *Window) strategySelector(gtx layout.Context) layout.Dimensions {
	return row(gtx, toggle(w.theme, &w.selectedOnlyButton, "Selected only", w.strategy == combinations.StrategySelected), toggle(w.theme, &w.allPermButton, "All permutations", w.strategy == combinations.StrategyAll), toggle(w.theme, &w.pairwiseButton, "Pairwise", w.strategy == combinations.StrategyPairwise), field(w.theme, "Max cases", &w.maxCases, 110))
}

func (w *Window) optionGrid(gtx layout.Context) layout.Dimensions {
	ids := []string{"PageSize", "EFResolution", "EFColorMode", "EFMediaType", "EFPrintSpeed"}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, layout.Rigid(w.queueColumn), layout.Rigid(spacerX(12)), layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
		children := make([]layout.FlexChild, 0, len(ids)*2)
		for _, id := range ids {
			opt, ok := w.capabilities.OptionByID(id)
			if !ok {
				continue
			}
			children = append(children, layout.Rigid(w.optionColumn(opt)), layout.Rigid(spacerX(12)))
		}
		return layout.Flex{Axis: layout.Horizontal}.Layout(gtx, children...)
	}))
}
func (w *Window) queueColumn(gtx layout.Context) layout.Dimensions {
	items := []layout.FlexChild{layout.Rigid(label(w.theme, "Queue", 14, palette.muted).Layout)}
	for _, q := range w.capabilities.Queues {
		if q.Available {
			items = append(items, layout.Rigid(label(w.theme, q.Name, 13, palette.text).Layout))
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
}
func (w *Window) optionColumn(opt capabilities.Option) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		ensureBools(w.selected, opt.ID, optionValues(opt))
		items := []layout.FlexChild{layout.Rigid(label(w.theme, opt.Label, 14, palette.muted).Layout)}
		vals := optionValues(opt)
		if len(vals) > 5 {
			vals = vals[:5]
		}
		for _, v := range vals {
			val := v
			cb := w.selected[opt.ID][val]
			items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				c := material.CheckBox(w.theme, cb, short(val, 24))
				c.Color = palette.primary
				return c.Layout(gtx)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
	}
}

func (w *Window) resultsCard(gtx layout.Context) layout.Dimensions {
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(sectionTitle(w.theme, "04 Results and activity log")), layout.Rigid(spacer(8)), layout.Rigid(label(w.theme, w.status, 14, palette.primary).Layout), layout.Rigid(spacer(10)), layout.Rigid(w.resultsTable), layout.Rigid(spacer(10)), layout.Rigid(w.logPanel))
	})
}
func (w *Window) resultsTable(gtx layout.Context) layout.Dimensions {
	rows := []layout.FlexChild{layout.Rigid(label(w.theme, "Request                                              Method   Status   Duration   Detail", 13, palette.muted).Layout)}
	for _, r := range w.results {
		rr := r
		rows = append(rows, layout.Rigid(label(w.theme, fmt.Sprintf("%-48s %-7s %-8s %-9s %s", short(rr.File, 46), rr.Method, rr.Status, rr.Duration, short(rr.Detail, 80)), 13, palette.text).Layout))
	}
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}
func (w *Window) logPanel(gtx layout.Context) layout.Dimensions {
	lines := w.log
	if len(lines) > 8 {
		lines = lines[len(lines)-8:]
	}
	rows := make([]layout.FlexChild, 0, len(lines))
	for _, l := range lines {
		line := l
		rows = append(rows, layout.Rigid(label(w.theme, line, 13, palette.text).Layout))
	}
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}

func (w *Window) captureCapabilities() {
	if w.running.Load() {
		return
	}
	server, ok := w.server()
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.running.Store(true)
	w.setStatus("Getting capabilities from server...")
	go func() {
		defer w.running.Store(false)
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
		snap := client.DiscoverCapabilities(ctx, session)
		model := capabilities.FromSnapshot(snap)
		env := preflight.Run(snap, model)
		path, err := client.SaveCapabilitySnapshot(snap, captureDirectory())
		if err != nil {
			w.addLog("Capability snapshot save failed: %s", err)
		}
		w.capabilities = model
		w.setStatus("Capabilities loaded. Preflight: " + env.OverallStatus)
		w.addLog("Saved capability snapshot: %s", path)
	}()
}

func (w *Window) startRun() {
	if w.running.Load() {
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
	workers, _ := strconv.Atoi(strings.TrimSpace(w.workers.Text()))
	if workers < 1 {
		workers = 1
	}
	combos := w.selectedCombinations()
	mode := runModes[w.runModeIndex]
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.running.Store(true)
	w.results = nil
	w.setStatus("Running automation...")
	go w.runAutomation(ctx, server, selectedFiles, workers, combos, mode)
}

func (w *Window) runAutomation(ctx context.Context, server model.ServerConnection, selectedFiles []string, workers int, combos []combinations.Combination, mode runMode) {
	defer w.running.Store(false)
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
	jobs := make(chan struct {
		file  string
		attrs map[string]string
	})
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for job := range jobs {
				w.executeJob(ctx, client, session, job.file, job.attrs, mode)
			}
		}()
	}
	for _, f := range selectedFiles {
		for _, c := range combos {
			select {
			case <-ctx.Done():
				close(jobs)
				wg.Wait()
				w.setStatus("Cancelled")
				return
			case jobs <- struct {
				file  string
				attrs map[string]string
			}{f, combinationToAttributes(c)}:
			}
		}
	}
	close(jobs)
	wg.Wait()
	w.setStatus("Completed. See results and logs.")
}

func (w *Window) executeJob(ctx context.Context, client *fiery.Client, session fiery.Session, file string, attrs map[string]string, mode runMode) {
	start := time.Now()
	imp, err := client.ImportJobToQueue(ctx, session, file, mode.ImportQueue)
	if err != nil {
		w.addResult(file, "POST", "ERR", time.Since(start), err.Error())
		return
	}
	if len(attrs) > 0 {
		if err := client.UpdateJobAttributes(ctx, session, imp.JobID, attrs); err != nil {
			w.addResult(file, "POST", "ERR", time.Since(start), err.Error())
			return
		}
	}
	got, err := client.GetJobAttributes(ctx, session, imp.JobID)
	if err != nil {
		w.addResult(file, "GET", "ERR", time.Since(start), err.Error())
		return
	}
	status := "PASS"
	detail := "set values matched get values"
	for k, v := range attrs {
		if got[k] != v {
			status = "FAIL"
			detail = fmt.Sprintf("%s set=%q got=%q", k, v, got[k])
			break
		}
	}
	w.addResult(file, http.MethodPost, status, time.Since(start), detail)
}

func (w *Window) server() (model.ServerConnection, bool) {
	s := model.ServerConnection{IPAddress: strings.TrimSpace(w.serverIP.Text()), SecretKey: strings.TrimSpace(w.secretKey.Text()), Password: strings.TrimSpace(w.password.Text())}
	if s.IPAddress == "" || s.SecretKey == "" || s.Password == "" {
		w.setStatus("Server IP, secret key, and admin password are required.")
		return model.ServerConnection{}, false
	}
	return s, true
}
func (w *Window) fileMode() model.FileSelectionMode {
	switch w.selectionMode {
	case 1:
		return model.FileSelectionSingle
	case 2:
		return model.FileSelectionRandom
	default:
		return model.FileSelectionAll
	}
}
func (w *Window) selectedCombinations() []combinations.Combination {
	axes := []combinations.Axis{}
	for _, id := range []string{"PageSize", "EFResolution", "EFColorMode", "EFMediaType", "EFPrintSpeed"} {
		vals := selectedValues(w.selected[id])
		if len(vals) > 0 {
			axes = append(axes, combinations.Axis{Name: id, Values: vals})
		}
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(w.maxCases.Text()))
	if limit < 1 {
		limit = 100
	}
	return combinations.GenerateWithStrategy(axes, w.strategy, limit)
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
func combinationToAttributes(c combinations.Combination) map[string]string {
	attrs := map[string]string{}
	for k, v := range c {
		attrs[k] = v
	}
	return attrs
}
func ensureBools(store map[string]map[string]*widget.Bool, id string, vals []string) {
	if store[id] == nil {
		store[id] = map[string]*widget.Bool{}
	}
	for i, v := range vals {
		if store[id][v] == nil {
			store[id][v] = &widget.Bool{Value: i == 0}
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

func (w *Window) setStatus(s string) {
	w.status = s
	w.diagnostic.printf("STATUS: %s", s)
	w.window.Invalidate()
}
func (w *Window) addLog(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	w.diagnostic.printf("UI: %s", line)
	w.log = append(w.log, time.Now().Format("15:04:05  ")+line)
	w.window.Invalidate()
}
func (w *Window) addResult(file, method, status string, d time.Duration, detail string) {
	w.results = append(w.results, resultRow{File: filepath.Base(file), Method: method, Status: status, Duration: d.Round(time.Millisecond).String(), Detail: detail})
	w.addLog("%s %s %s", filepath.Base(file), status, detail)
}

func card(gtx layout.Context, child layout.Widget) layout.Dimensions {
	return layout.UniformInset(unit.Dp(18)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, palette.surface, clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Max}, SE: 12, SW: 12, NE: 12, NW: 12}.Op(gtx.Ops))
		return child(gtx)
	})
}
func surfaceAlt(gtx layout.Context, child layout.Widget) layout.Dimensions {
	return layout.UniformInset(unit.Dp(12)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		paint.FillShape(gtx.Ops, palette.surfaceAlt, clip.RRect{Rect: image.Rectangle{Max: gtx.Constraints.Max}, SE: 8, SW: 8, NE: 8, NW: 8}.Op(gtx.Ops))
		return child(gtx)
	})
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
func navItem(th *material.Theme, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, label(th, "› "+text, 15, rgb(0xe2e8f0)).Layout)
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
