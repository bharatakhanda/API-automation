//go:build windows

package appgio

import (
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"net/http"
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
	"github.com/rodrigocfd/windigo/co"
	"github.com/rodrigocfd/windigo/win"
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
	window *app.Window
	theme  *material.Theme
	ops    op.Ops
	list   widget.List

	serverIP, secretKey, password widget.Editor
	folderPath, filePath          widget.Editor
	endpoint, workers, maxCases   widget.Editor

	settingsButton, captureButton, runButton, cancelButton widget.Clickable
	testServerButton                                       widget.Clickable
	browseFolderButton, browseFileButton                   widget.Clickable
	navButtons                                             []widget.Clickable
	allFilesButton, singleFileButton, randomFileButton     widget.Clickable
	selectedOnlyButton, allPermButton, pairwiseButton      widget.Clickable
	modeButtons                                            []widget.Clickable
	modeChecks                                             []widget.Bool
	fileModeGroup, runModeGroup                            widget.Enum

	activePage    int
	selectionMode int
	strategy      combinations.Strategy
	runModeIndex  int

	capabilities     capabilities.Model
	selected         map[string]map[string]*widget.Bool
	mu               sync.Mutex
	log              []string
	results          []resultRow
	status           string
	serverTestStatus string
	captureActive    bool
	captureProgress  float32
	capturePhase     string
	running          atomic.Bool
	cancel           context.CancelFunc
	diagnostic       *diagnosticLog
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
	{Label: "Press Print", ImportQueue: "hold", Actions: []string{"rip", "production", "press_print"}},
	{Label: "Ready to Print", ImportQueue: "hold", Actions: []string{"rip", "production"}},
	{Label: "Print", ImportQueue: "hold", Actions: []string{"rip", "production", "press_print", "print"}},
}

func New() *Window {
	w := &Window{window: new(app.Window), theme: material.NewTheme(), selected: map[string]map[string]*widget.Bool{}, strategy: combinations.StrategySelected, status: "Ready · Open Settings, discover capabilities, then run automation.", diagnostic: newDiagnosticLog()}
	w.theme.Palette = material.Palette{Bg: palette.bg, Fg: palette.text, ContrastBg: palette.primary, ContrastFg: rgb(0xffffff)}
	w.theme.TextSize = 15
	w.list.Axis = layout.Vertical
	initEditor(&w.serverIP, "")
	initEditor(&w.secretKey, fiery.DefaultSecretKey)
	initEditor(&w.password, "")
	w.secretKey.Mask = '•'
	w.password.Mask = '•'
	initEditor(&w.folderPath, "")
	initEditor(&w.filePath, "")
	initEditor(&w.endpoint, "/live/api/v5/jobs")
	initEditor(&w.workers, "1")
	initEditor(&w.maxCases, "100")
	w.fileModeGroup.Value = "all"
	w.runModeGroup.Value = "0"
	w.modeChecks = make([]widget.Bool, len(runModes))
	if len(w.modeChecks) > 0 {
		w.modeChecks[0].Value = true
	}
	w.serverTestStatus = "Not tested"
	w.window.Option(app.Title("API Automation"), app.Size(unit.Dp(1240), unit.Dp(900)), app.MinSize(unit.Dp(1100), unit.Dp(760)))
	return w
}

func initEditor(e *widget.Editor, text string) { e.SingleLine = true; e.Submit = true; e.SetText(text) }

func Run() int {
	code := make(chan int, 1)
	go func() {
		_, _ = win.CoInitializeEx(co.COINIT_APARTMENTTHREADED | co.COINIT_DISABLE_OLE1DDE)
		defer win.CoUninitialize()
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
	for w.testServerButton.Clicked(gtx) {
		w.testServerConnection()
	}
	for w.captureButton.Clicked(gtx) {
		w.activePage = 1
		w.captureCapabilities()
	}
	for w.runButton.Clicked(gtx) {
		w.activePage = 2
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
			w.selectionMode = 1
			w.fileModeGroup.Value = "single"
			w.addLog("Selected test file: %s", path)
		}
	}
	for i := range w.navButtons {
		for w.navButtons[i].Clicked(gtx) {
			w.activePage = i
		}
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
}

func (w *Window) layout(gtx layout.Context) layout.Dimensions {
	paint.Fill(gtx.Ops, palette.bg)
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.sidebar(gtx) }),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Inset{Top: unit.Dp(24), Right: unit.Dp(28), Bottom: unit.Dp(20), Left: unit.Dp(28)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return w.list.Layout(gtx, 1, func(gtx layout.Context, _ int) layout.Dimensions { return w.content(gtx) })
			})
		}),
	)
}

func (w *Window) sidebar(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(unit.Dp(210)), gtx.Dp(unit.Dp(210))
	paint.FillShape(gtx.Ops, palette.navy, clip.Rect{Max: gtx.Constraints.Max}.Op())
	return layout.Inset{Top: unit.Dp(24), Left: unit.Dp(18), Right: unit.Dp(18)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if len(w.navButtons) != 3 {
			w.navButtons = make([]widget.Clickable, 3)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(w.theme, "API Automation", 20, rgb(0xffffff)).Layout),
			layout.Rigid(spacer(26)),
			layout.Rigid(label(w.theme, "Workspace", 13, rgb(0x93a4bd)).Layout),
			layout.Rigid(spacer(14)),
			layout.Rigid(navButton(w.theme, &w.navButtons[0], "Settings", w.activePage == 0)),
			layout.Rigid(navButton(w.theme, &w.navButtons[1], "Capabilities", w.activePage == 1)),
			layout.Rigid(navButton(w.theme, &w.navButtons[2], "Results & logs", w.activePage == 2)),
		)
	})
}

func (w *Window) content(gtx layout.Context) layout.Dimensions {
	children := []layout.FlexChild{layout.Rigid(w.header), layout.Rigid(spacer(18))}
	switch w.activePage {
	case 0:
		children = append(children, layout.Rigid(w.settingsCard))
	case 1:
		children = append(children, layout.Rigid(w.capabilitiesCard))
	default:
		children = append(children, layout.Rigid(w.resultsCard))
	}
	children = append(children, layout.Rigid(spacer(20)))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (w *Window) header(gtx layout.Context) layout.Dimensions {
	title := "Settings"
	subtitle := "Configure server connection, test files, and Fiery run mode."
	if w.activePage == 1 {
		title = "Capabilities"
		subtitle = "Discover Fiery capabilities and choose job options for automation."
	} else if w.activePage == 2 {
		title = "Results & logs"
		subtitle = "Review automation outcomes, diagnostics, and runtime messages."
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(label(w.theme, title, 24, palette.text).Layout), layout.Rigid(label(w.theme, subtitle, 14, palette.muted).Layout))
		}),
		layout.Rigid(primaryButton(w.theme, &w.captureButton, "Get server capabilities")), layout.Rigid(spacerX(10)), layout.Rigid(primaryButton(w.theme, &w.runButton, "Run automation")), layout.Rigid(spacerX(10)), layout.Rigid(secondaryButton(w.theme, &w.cancelButton, "Cancel")),
	)
}

func (w *Window) settingsCard(gtx layout.Context) layout.Dimensions {
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = minInt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(900)))
		w.mu.Lock()
		serverStatus := w.serverTestStatus
		w.mu.Unlock()
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return formPanel(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(label(w.theme, "Server connection", 24, palette.text).Layout),
						layout.Rigid(spacer(6)),
						layout.Rigid(label(w.theme, "Enter the Fiery server details used for discovery and automation.", 14, palette.muted).Layout),
						layout.Rigid(spacer(22)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return row(gtx,
								fieldBox(w.theme, "Server IP address", "Example: 10.220.129.85", &w.serverIP, 390),
								fieldBox(w.theme, "Admin password", "Administrator password", &w.password, 390),
							)
						}),
						layout.Rigid(spacer(16)),
						layout.Rigid(fieldBox(w.theme, "Secret key", "Fiery API access key", &w.secretKey, 794)),
						layout.Rigid(spacer(18)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return row(gtx, primaryButton(w.theme, &w.testServerButton, "Test server connection"), statusBadge(w.theme, serverStatus, serverStatusColor(serverStatus)))
						}),
					)
				})
			}),
			layout.Rigid(spacer(22)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return formPanel(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(label(w.theme, "Test File Setup", 24, palette.text).Layout),
						layout.Rigid(spacer(6)),
						layout.Rigid(label(w.theme, "Choose the files to import during automation.", 14, palette.muted).Layout),
						layout.Rigid(spacer(22)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return row(gtx, fieldBox(w.theme, "Folder path", "Folder containing test files", &w.folderPath, 640), browseButton(w.theme, &w.browseFolderButton, "Browse folder"))
						}),
						layout.Rigid(spacer(16)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return row(gtx, fieldBox(w.theme, "Specific file path", "Optional single PDF/job file", &w.filePath, 640), browseButton(w.theme, &w.browseFileButton, "Browse file"))
						}),
						layout.Rigid(spacer(18)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return row(gtx, fieldBox(w.theme, "Parallel jobs", "1", &w.workers, 150))
						}),
						layout.Rigid(spacer(22)),
						layout.Rigid(w.fileSelectionRadioGroup),
						layout.Rigid(spacer(18)),
						layout.Rigid(w.runModeRadioGroup),
					)
				})
			}),
		)
	})
}
func (w *Window) assetsCard(gtx layout.Context) layout.Dimensions {
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(sectionTitle(w.theme, "02 Test assets and run setup")),
			layout.Rigid(spacer(12)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return row(gtx, field(w.theme, "Folder path", &w.folderPath, 610), secondaryButton(w.theme, &w.browseFolderButton, "Browse folder"), field(w.theme, "Workers", &w.workers, 90))
			}),
			layout.Rigid(spacer(12)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return row(gtx, field(w.theme, "Specific file", &w.filePath, 610), secondaryButton(w.theme, &w.browseFileButton, "Browse file"), field(w.theme, "Endpoint", &w.endpoint, 260))
			}),
			layout.Rigid(spacer(12)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return row(gtx, toggle(w.theme, &w.allFilesButton, "All files", w.selectionMode == 0), toggle(w.theme, &w.singleFileButton, "Single file", w.selectionMode == 1), toggle(w.theme, &w.randomFileButton, "Random file", w.selectionMode == 2))
			}),
			layout.Rigid(spacer(12)), layout.Rigid(w.modeSelector))
	})
}

func (w *Window) modeSelector(gtx layout.Context) layout.Dimensions {
	return w.runModeRadioGroup(gtx)
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

func (w *Window) capabilitiesCard(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	model := w.capabilities
	active := w.captureActive
	w.mu.Unlock()
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		if len(model.Options) == 0 && len(model.Queues) == 0 {
			children := []layout.FlexChild{layout.Rigid(sectionTitle(w.theme, "Server capabilities")), layout.Rigid(spacer(10))}
			if active {
				children = append(children, layout.Rigid(w.captureProgressPanel), layout.Rigid(spacer(10)))
			}
			children = append(children, layout.Rigid(label(w.theme, "Click Get server capabilities. Options will appear here after the server responds.", 14, palette.muted).Layout))
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}
		children := []layout.FlexChild{layout.Rigid(sectionTitle(w.theme, fmt.Sprintf("Server capabilities · %s", fallback(model.ServerName, "discovered")))), layout.Rigid(spacer(10))}
		if active {
			children = append(children, layout.Rigid(w.captureProgressPanel), layout.Rigid(spacer(10)))
		}
		children = append(children, layout.Rigid(w.strategySelector), layout.Rigid(spacer(12)), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.optionGrid(gtx, model) }))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (w *Window) captureProgressPanel(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	phase := w.capturePhase
	progress := w.captureProgress
	w.mu.Unlock()
	if phase == "" {
		phase = "Getting capabilities from server..."
	}
	bar := material.ProgressBar(w.theme, progress)
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(w.theme, phase, 14, palette.primary).Layout),
			layout.Rigid(spacer(8)),
			layout.Rigid(bar.Layout),
		)
	})
}

func (w *Window) strategySelector(gtx layout.Context) layout.Dimensions {
	return row(gtx, toggle(w.theme, &w.selectedOnlyButton, "Selected only", w.strategy == combinations.StrategySelected), toggle(w.theme, &w.allPermButton, "All permutations", w.strategy == combinations.StrategyAll), toggle(w.theme, &w.pairwiseButton, "Pairwise", w.strategy == combinations.StrategyPairwise), field(w.theme, "Max cases", &w.maxCases, 110))
}

func (w *Window) optionGrid(gtx layout.Context, model capabilities.Model) layout.Dimensions {
	groups := capabilities.GroupedOptions(model)
	children := []layout.FlexChild{layout.Rigid(w.queueGroup(model))}
	for _, group := range groups {
		g := group
		children = append(children, layout.Rigid(spacer(14)), layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.capabilityGroup(gtx, g) }))
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
	children := []layout.FlexChild{layout.Rigid(label(w.theme, group.Name, 16, palette.text).Layout), layout.Rigid(spacer(8))}
	for _, opt := range group.Options {
		option := opt
		children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.optionRow(gtx, option) }), layout.Rigid(spacer(8)))
	}
	return formPanel(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (w *Window) optionRow(gtx layout.Context, opt capabilities.Option) layout.Dimensions {
	ensureBools(w.selected, opt.ID, optionValues(opt))
	vals := optionValues(opt)
	shown := vals
	if len(shown) > 12 {
		shown = shown[:12]
	}
	return layout.Flex{Axis: layout.Horizontal}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(unit.Dp(220)), gtx.Dp(unit.Dp(220))
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(label(w.theme, opt.Label, 14, palette.text).Layout),
				layout.Rigid(label(w.theme, opt.ID, 12, palette.muted).Layout),
				layout.Rigid(label(w.theme, fmt.Sprintf("%d value(s)", len(vals)), 12, palette.muted).Layout),
			)
		}),
		layout.Rigid(spacerX(12)),
		layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
			items := make([]layout.FlexChild, 0, len(shown)+1)
			for _, v := range shown {
				val := v
				cb := w.selected[opt.ID][val]
				items = append(items, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
					c := material.CheckBox(w.theme, cb, val)
					c.Color = palette.primary
					return c.Layout(gtx)
				}))
			}
			if len(vals) > len(shown) {
				items = append(items, layout.Rigid(label(w.theme, fmt.Sprintf("+%d more values captured in snapshot/logs", len(vals)-len(shown)), 13, palette.muted).Layout))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, items...)
		}),
	)
}

func (w *Window) resultsCard(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	status := w.status
	w.mu.Unlock()
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, layout.Rigid(sectionTitle(w.theme, "04 Results and activity log")), layout.Rigid(spacer(8)), layout.Rigid(label(w.theme, status, 14, palette.primary).Layout), layout.Rigid(spacer(10)), layout.Rigid(w.resultsTable), layout.Rigid(spacer(10)), layout.Rigid(w.logPanel))
	})
}
func (w *Window) resultsTable(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	results := append([]resultRow(nil), w.results...)
	w.mu.Unlock()
	rows := []layout.FlexChild{layout.Rigid(label(w.theme, "Request                                              Method   Status   Duration   Detail", 13, palette.muted).Layout)}
	for _, r := range results {
		rr := r
		rows = append(rows, layout.Rigid(label(w.theme, fmt.Sprintf("%-48s %-7s %-8s %-9s %s", short(rr.File, 46), rr.Method, rr.Status, rr.Duration, short(rr.Detail, 80)), 13, palette.text).Layout))
	}
	return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
	})
}
func (w *Window) logPanel(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	lines := append([]string(nil), w.log...)
	w.mu.Unlock()
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

func (w *Window) testServerConnection() {
	server, ok := w.server()
	if !ok {
		w.setServerTestStatus("Missing server details")
		return
	}
	w.setServerTestStatus("Testing...")
	w.setStatus("Testing server connection...")
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		client, err := fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
		if err != nil {
			w.setServerTestStatus("Connection failed")
			w.setStatus("Server test failed: " + err.Error())
			w.addLog("Server connection test failed: %v", err)
			return
		}
		if _, err := client.Login(ctx); err != nil {
			w.setServerTestStatus("Authentication failed")
			w.setStatus("Server test failed: " + err.Error())
			w.addLog("Server connection test failed: %v", err)
			return
		}
		w.setServerTestStatus("Connection OK")
		w.setStatus("Server connection OK")
		w.addLog("Server connection test passed for %s", server.IPAddress)
	}()
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
	w.setCaptureProgress(true, 0.05, "Preparing server capability capture...")
	w.setStatus("Getting capabilities from server...")
	go func() {
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
		w.setCaptureProgress(true, 0.75, "Normalizing capabilities and running preflight checks...")
		model := capabilities.FromSnapshot(snap)
		env := preflight.Run(snap, model)
		w.setCaptureProgress(true, 0.90, "Saving capability and environment snapshots...")
		path, err := client.SaveCapabilitySnapshot(snap, captureDirectory())
		if err != nil {
			w.addLog("Capability snapshot save failed: %s", err)
		}
		w.mu.Lock()
		w.capabilities = model
		w.mu.Unlock()
		w.setCaptureProgress(true, 1.0, "Capabilities loaded successfully.")
		w.setStatus("Capabilities loaded. Preflight: " + env.OverallStatus)
		w.logCapabilitySummary(model)
		w.addLog("Saved capability snapshot: %s", path)
	}()
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
	w.window.Invalidate()
}

func (w *Window) isCaptureActive() bool {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.captureActive
}

func (w *Window) logCapabilitySummary(model capabilities.Model) {
	groups := capabilities.GroupedOptions(model)
	w.addLog("Discovered server %s serial=%s version=%s queues=%d options=%d groups=%d", fallback(model.ServerName, "unknown"), fallback(model.SerialNumber, "unknown"), fallback(model.Version, "unknown"), len(model.Queues), len(model.Options), len(groups))
	for _, group := range groups {
		keys := make([]string, 0, len(group.Options))
		for _, opt := range group.Options {
			keys = append(keys, fmt.Sprintf("%s(%d)", opt.ID, len(optionValues(opt))))
		}
		w.addLog("Capability group %s: %s", group.Name, strings.Join(keys, ", "))
	}
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
	w.logSelectedCombinations(combos)
	modes := w.selectedRunModes()
	if len(modes) == 0 {
		w.setStatus("Select at least one run mode.")
		return
	}
	if combinationsRequireRipReadback(combos) && !runModesIncludeAction(modes, "rip") {
		w.setStatus("Selected capabilities require RIP before strict verification. Select Process and Hold or RIP run mode.")
		return
	}
	w.addLog("Selected run modes: %s", formatRunModes(modes))
	ctx, cancel := context.WithCancel(context.Background())
	w.cancel = cancel
	w.running.Store(true)
	w.mu.Lock()
	w.results = nil
	w.mu.Unlock()
	w.setStatus("Running automation...")
	go w.runAutomation(ctx, server, selectedFiles, workers, combos, modes)
}

func (w *Window) runAutomation(ctx context.Context, server model.ServerConnection, selectedFiles []string, workers int, combos []combinations.Combination, modes []runMode) {
	defer func() {
		w.running.Store(false)
		w.window.Invalidate()
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
				w.executeJob(ctx, client, session, job.file, job.attrs, job.mode)
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
					w.setStatus("Cancelled")
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
	w.setStatus("Completed. See results and logs.")
}

func (w *Window) executeJob(ctx context.Context, client *fiery.Client, session fiery.Session, file string, attrs map[string]string, mode runMode) {
	start := time.Now()
	imp, err := client.ImportJobToQueue(ctx, session, file, mode.ImportQueue)
	if err != nil {
		w.addResult(file, "POST", "ERR", time.Since(start), fmt.Sprintf("mode=%s: %v", mode.Label, err))
		return
	}
	w.addLog("Imported %s as job %s into queue %s for mode %s", filepath.Base(file), imp.JobID, mode.ImportQueue, mode.Label)
	if err := w.confirmImport(ctx, client, session, imp.JobID); err != nil {
		w.addResult(file, "GET", "ERR", time.Since(start), fmt.Sprintf("mode=%s: %v", mode.Label, err))
		return
	}
	if len(attrs) > 0 {
		// Wait for the imported ticket to finish spooling before changing it.
		// Command/rerip attributes can otherwise be accepted and subsequently
		// overwritten when Fiery finishes constructing the job ticket.
		w.addLog("Waiting for job %s status=done spooling before setting attributes", imp.JobID)
		if _, err := w.waitJobCondition(ctx, client, session, imp.JobID, "done spooling before attribute update", 4*time.Minute, time.Second, statusEquals("done spooling")); err != nil {
			w.addResult(file, "GET", "ERR", time.Since(start), fmt.Sprintf("mode=%s: %v", mode.Label, err))
			return
		}
		w.addLog("Setting job %s attributes after done spooling: %s", imp.JobID, formatAttributes(attrs))
		if err := client.UpdateJobAttributes(ctx, session, imp.JobID, attrs); err != nil {
			w.addResult(file, "POST", "ERR", time.Since(start), fmt.Sprintf("mode=%s: %v", mode.Label, err))
			return
		}
		if err := w.confirmAttributeUpdate(ctx, client, session, imp.JobID, attrs, mode); err != nil {
			w.addResult(file, "GET", "ERR", time.Since(start), fmt.Sprintf("mode=%s: %v", mode.Label, err))
			return
		}
	}
	if err := w.performModeLifecycle(ctx, client, session, imp.JobID, mode); err != nil {
		w.addResult(file, "POST", "ERR", time.Since(start), fmt.Sprintf("mode=%s: %v", mode.Label, err))
		return
	}
	got, err := w.readBackAttributes(ctx, client, session, imp.JobID, attrs)
	if err != nil {
		w.addResult(file, "GET", "ERR", time.Since(start), fmt.Sprintf("mode=%s: %v", mode.Label, err))
		return
	}
	status := "PASS"
	detail := fmt.Sprintf("mode=%s: set values matched get values", mode.Label)
	if len(attrs) == 0 {
		detail = fmt.Sprintf("mode=%s: import/lifecycle completed; no job attributes were selected for set/get verification", mode.Label)
	}
	for k, v := range attrs {
		if got[k] != v {
			status = "FAIL"
			detail = fmt.Sprintf("mode=%s: %s set=%q got=%q status=%q state=%q display=%q recent=%q related=%s availableKeys=%s", mode.Label, k, v, got[k], got["status"], got["state"], got["display status"], got["recent action"], relatedReadbackValues(got), short(strings.Join(sortedKeys(got), ","), 220))
			if requiresRipReadback(k) && !modeIncludesAction(mode, "rip") {
				detail += "; note=this attribute is typically readable only after RIP, select RIP or Process and Hold for strict verification"
			}
			w.logRawPostmanComparison(ctx, client, session, imp.JobID)
			break
		}
	}
	w.addResult(file, http.MethodPost, status, time.Since(start), detail)
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
	// Fiery attributes are not readable immediately, and some command/rerip
	// attributes such as EFResolution are materialized only after RIP. Therefore
	// do not fail the run here; final strict set/get verification runs after the
	// selected lifecycle actions complete.
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
	deadline := time.NewTimer(45 * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	var got map[string]string
	var err error
	for {
		got, err = client.GetJobAttributes(ctx, session, jobID)
		if err != nil {
			w.diagnostic.printf("READBACK: job=%s attempt=error error=%v", jobID, err)
			return nil, err
		}
		w.logAttributeReadback(jobID, got, expected)
		if attributesMatch(got, expected) {
			return got, nil
		}
		select {
		case <-ctx.Done():
			return got, ctx.Err()
		case <-deadline.C:
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
		Matched:       attributesMatch(got, expected),
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
		w.diagnostic.printf("POSTMAN_COMPARE: method=%s endpoint=%s status=%d accept=application/json body=%s", response.Method, response.Endpoint, response.StatusCode, response.Body)
	}
}

func attributesPresent(got, expected map[string]string) bool {
	for key := range expected {
		if _, ok := got[key]; !ok {
			return false
		}
	}
	return true
}

func attributesMatch(got, expected map[string]string) bool {
	for key, want := range expected {
		if got[key] != want {
			return false
		}
	}
	return true
}

func (w *Window) performModeLifecycle(ctx context.Context, client *fiery.Client, session fiery.Session, jobID string, mode runMode) error {
	for _, action := range mode.Actions {
		switch action {
		case "rip":
			w.addLog("Waiting for job %s status=done spooling before RIP", jobID)
			if _, err := w.waitJobCondition(ctx, client, session, jobID, "done spooling before RIP", 4*time.Minute, 2*time.Second, statusEquals("done spooling")); err != nil {
				return err
			}
			w.addLog("Running RIP for job %s", jobID)
			if err := client.JobAction(ctx, session, jobID, "rip"); err != nil {
				return err
			}
			w.addLog("Waiting for job %s status=done ripping after RIP", jobID)
			if _, err := w.waitJobCondition(ctx, client, session, jobID, "done ripping after RIP", 6*time.Minute, 2*time.Second, statusEquals("done ripping")); err != nil {
				return err
			}
		case "production":
			w.addLog("Waiting for job %s status=done ripping before Ready to Print", jobID)
			if _, err := w.waitJobCondition(ctx, client, session, jobID, "done ripping before production", 6*time.Minute, 2*time.Second, statusEquals("done ripping")); err != nil {
				return err
			}
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
		}
	}
	return nil
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
	case "EFResolution", "EFPrintSpeed", "EFRotateDocument":
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

func (w *Window) server() (model.ServerConnection, bool) {
	s := model.ServerConnection{IPAddress: strings.TrimSpace(w.serverIP.Text()), SecretKey: strings.TrimSpace(w.secretKey.Text()), Password: strings.TrimSpace(w.password.Text())}
	if s.IPAddress == "" || s.SecretKey == "" || s.Password == "" {
		w.setStatus("Server IP, secret key, and admin password are required.")
		return model.ServerConnection{}, false
	}
	return s, true
}
func (w *Window) fileMode() model.FileSelectionMode {
	switch w.fileModeGroup.Value {
	case "single":
		w.selectionMode = 1
		return model.FileSelectionSingle
	case "random":
		w.selectionMode = 2
		return model.FileSelectionRandom
	default:
		w.selectionMode = 0
		return model.FileSelectionAll
	}
}

func (w *Window) currentRunModeIndex() int {
	idx, err := strconv.Atoi(w.runModeGroup.Value)
	if err != nil || idx < 0 || idx >= len(runModes) {
		return w.runModeIndex
	}
	w.runModeIndex = idx
	return idx
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

func formatRunModes(modes []runMode) string {
	labels := make([]string, 0, len(modes))
	for _, mode := range modes {
		labels = append(labels, mode.Label)
	}
	return strings.Join(labels, ", ")
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

func (w *Window) selectedCombinations() []combinations.Combination {
	w.mu.Lock()
	model := w.capabilities
	w.mu.Unlock()
	axes := make([]combinations.Axis, 0, len(w.selected))
	ids := make([]string, 0, len(w.selected))
	for id := range w.selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		vals := selectedValues(w.selected[id])
		if len(vals) == 0 {
			continue
		}
		if w.strategy != combinations.StrategySelected {
			if option, ok := model.OptionByID(id); ok {
				allValues := optionValues(option)
				if len(allValues) > len(vals) {
					vals = allValues
				}
			}
		}
		sort.Strings(vals)
		axes = append(axes, combinations.Axis{Name: id, Values: vals})
	}
	if len(axes) == 0 {
		if w.strategy == combinations.StrategySelected {
			return []combinations.Combination{{}}
		}
		axes = defaultPermutationAxes(model)
		if len(axes) == 0 {
			return []combinations.Combination{{}}
		}
	}
	limit, _ := strconv.Atoi(strings.TrimSpace(w.maxCases.Text()))
	if limit < 1 {
		limit = 100
	}
	return combinations.GenerateWithStrategy(axes, w.strategy, limit)
}

func (w *Window) logSelectedCombinations(combos []combinations.Combination) {
	selected := make([]string, 0, len(w.selected))
	w.mu.Lock()
	model := w.capabilities
	w.mu.Unlock()
	for id, values := range w.selected {
		vals := selectedValues(values)
		if len(vals) == 0 {
			continue
		}
		if w.strategy != combinations.StrategySelected {
			if option, ok := model.OptionByID(id); ok && len(optionValues(option)) > len(vals) {
				vals = optionValues(option)
			}
		}
		sort.Strings(vals)
		selected = append(selected, fmt.Sprintf("%s=%v", id, vals))
	}
	sort.Strings(selected)
	if len(selected) == 0 {
		if w.strategy == combinations.StrategySelected {
			w.addLog("Selected %d combination(s) for strategy=%s; no job attributes selected, running import/lifecycle only", len(combos), w.strategy)
			return
		}
		axes := defaultPermutationAxes(model)
		for _, axis := range axes {
			selected = append(selected, fmt.Sprintf("%s=%v", axis.Name, axis.Values))
		}
		w.addLog("Selected %d combination(s) for strategy=%s; no checkbox axes selected, using default discovered permutation axes", len(combos), w.strategy)
	}
	w.addLog("Selected %d combination(s) for strategy=%s; axes: %s", len(combos), w.strategy, strings.Join(selected, "; "))
}
func defaultPermutationAxes(model capabilities.Model) []combinations.Axis {
	preferred := []string{"EFResolution", "EFColorMode", "EFMediaType", "EFPrintSpeed", "PageSize", "num copies", "EFBrightness", "EFPrintCover", "EFOutputBin"}
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
		if _, ok := seen[opt.ID]; ok {
			continue
		}
		vals := optionValues(opt)
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

func (w *Window) setStatus(s string) {
	w.mu.Lock()
	w.status = s
	w.mu.Unlock()
	w.diagnostic.printf("STATUS: %s", s)
	w.window.Invalidate()
}

func (w *Window) setServerTestStatus(s string) {
	w.mu.Lock()
	w.serverTestStatus = s
	w.mu.Unlock()
	w.diagnostic.printf("SERVER_TEST: %s", s)
	w.window.Invalidate()
}

func (w *Window) addLog(format string, args ...any) {
	line := fmt.Sprintf(format, args...)
	w.diagnostic.printf("UI: %s", line)
	w.mu.Lock()
	w.log = append(w.log, time.Now().Format("15:04:05  ")+line)
	w.mu.Unlock()
	w.window.Invalidate()
}
func (w *Window) addResult(file, method, status string, d time.Duration, detail string) {
	w.mu.Lock()
	w.results = append(w.results, resultRow{File: filepath.Base(file), Method: method, Status: status, Duration: d.Round(time.Millisecond).String(), Detail: detail})
	w.mu.Unlock()
	w.addLog("%s %s %s", filepath.Base(file), status, detail)
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
func navItem(th *material.Theme, text string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(12)}.Layout(gtx, label(th, "› "+text, 15, rgb(0xe2e8f0)).Layout)
	}
}

func navButton(th *material.Theme, b *widget.Clickable, text string, active bool) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Inset{Bottom: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
			btn := material.Button(th, b, "› "+text)
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
