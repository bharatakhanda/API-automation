//go:build windows

package appgio

import (
	"context"
	"fmt"
	"image/color"
	"strings"
	"time"

	"api-automation/internal/application"
	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/fiery"
	"api-automation/internal/model"

	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

func (w *Window) connectionBackend() *application.ConnectionState {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.connectionState == nil {
		w.connectionState = application.NewConnectionState(fiery.DefaultSecretKey)
	}
	return w.connectionState
}

func (w *Window) draftConnectionUnchecked() model.ServerConnection {
	return w.connectionBackend().ResolveDraft(model.ServerConnection{
		IPAddress: strings.TrimSpace(w.serverIP.Text()),
		SecretKey: strings.TrimSpace(w.secretKey.Text()),
		Password:  strings.TrimSpace(w.password.Text()),
	})
}

func (w *Window) invalidateChangedConnectionTest() {
	w.connectionBackend().InvalidateIfChanged(w.draftConnectionUnchecked())
}

func (w *Window) applyTestedConnection() {
	if w.running.Load() || w.managingJob.Load() || w.managingServer.Load() || w.inspectingJobs.Load() || w.testingServer.Load() {
		w.setStatus("Wait for the active operation to finish before changing the server connection.")
		return
	}
	draft, ok := w.connectionDraft()
	if !ok {
		return
	}
	_, changed, err := w.connectionBackend().Apply(draft)
	if err != nil {
		w.setStatus(capitalizeStatus(err.Error()) + ".")
		return
	}
	if changed {
		w.invalidateServerDependentState()
	}
	w.mu.Lock()
	w.healthStatus = "Checking"
	w.healthDetail = "Waiting for the first lightweight status check."
	w.mu.Unlock()
	// Existing credential values are never repopulated into editors. Empty
	// replacement fields mean "keep configured" on future connection changes.
	w.secretKey.SetText("")
	w.password.SetText("")
	w.setStatus("Connection applied. Continue from Overview.")
	w.addLog("Applied tested server connection for %s", draft.IPAddress)
	w.setActivePage(pageOverview)
}

func (w *Window) beginConnectionChange() {
	if server, ok := w.connectionBackend().BeginChange(); ok {
		w.serverIP.SetText(server.IPAddress)
		w.secretKey.SetText("")
		w.password.SetText("")
	}
	w.setActivePage(pageConnection)
	w.setStatus("Current connection remains active until replacement details pass testing and you press OK.")
}

func (w *Window) cancelConnectionChange() {
	server, ok := w.connectionBackend().CancelChange()
	if !ok {
		return
	}
	w.serverIP.SetText(server.IPAddress)
	w.secretKey.SetText("")
	w.password.SetText("")
	w.setStatus("Connection change cancelled. The current server remains active.")
	w.setActivePage(pageOverview)
}

func (w *Window) invalidateServerDependentState() {
	w.stopOverviewHealthMonitor()
	w.mu.Lock()
	w.capabilities = capabilities.Model{}
	if w.capabilityGuard == nil {
		w.capabilityGuard = new(application.GenerationGuard)
	}
	w.capabilityGuard.Next()
	if w.adminState != nil {
		w.adminState.InvalidateInventory()
	}
	w.healthStatus = "Not checked"
	w.healthDetail = ""
	w.healthCheckedAt = time.Time{}
	w.healthLatency = 0
	w.mu.Unlock()
	w.selected = make(map[string]map[string]*widget.Bool)
	w.groupChecks = make(map[string]*widget.Bool)
	w.optionChecks = make(map[string]*widget.Bool)
	w.numericInputs = make(map[string]*widget.Editor)
	w.categoryButtons = make(map[string]*widget.Clickable)
	w.activeCapabilityGroup = "Job Info"
	w.serverPresetGroup.Value = noServerPresetID
	w.jobActionID.SetText("")
	w.adminConfirmation.SetText("")
}

func (w *Window) workflowSidebar(gtx layout.Context) layout.Dimensions {
	gtx.Constraints.Min.X, gtx.Constraints.Max.X = gtx.Dp(unit.Dp(236)), gtx.Dp(unit.Dp(236))
	paintSidebarBackground(gtx)
	return layout.Inset{Top: unit.Dp(24), Right: unit.Dp(16), Bottom: unit.Dp(18), Left: unit.Dp(16)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		if len(w.navButtons) != len(workspacePages) {
			w.navButtons = make([]widget.Clickable, len(workspacePages))
		}
		connected := w.connectionBackend().Snapshot().HasActive
		children := []layout.FlexChild{
			layout.Rigid(label(w.theme, "API Automation", 20, rgb(0xffffff)).Layout),
			layout.Rigid(spacer(24)),
		}
		if !connected {
			children = append(children,
				layout.Rigid(navButton(w.theme, &w.navButtons[pageConnection], workspacePages[pageConnection].NavigationLabel, true)),
				layout.Rigid(spacer(8)),
				layout.Rigid(label(w.theme, "Test the connection and press OK to unlock the workspace.", 12, rgb(0x93a4bd)).Layout),
			)
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		}
		for pageIndex := pageConnection; pageIndex <= pageResults; pageIndex++ {
			children = append(children, layout.Rigid(navButton(w.theme, &w.navButtons[pageIndex], workspacePages[pageIndex].NavigationLabel, w.activePage == pageIndex)))
		}
		children = append(children,
			layout.Rigid(spacer(14)),
			layout.Rigid(label(w.theme, "OPERATIONS", 11, rgb(0x93a4bd)).Layout),
			layout.Rigid(spacer(10)),
			layout.Rigid(navButton(w.theme, &w.navButtons[pageLogs], workspacePages[pageLogs].NavigationLabel, w.activePage == pageLogs)),
			layout.Rigid(navButton(w.theme, &w.navButtons[pageAdministration], workspacePages[pageAdministration].NavigationLabel, w.activePage == pageAdministration)),
		)
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func paintSidebarBackground(gtx layout.Context) {
	paint.FillShape(gtx.Ops, palette.navy, clip.Rect{Max: gtx.Constraints.Max}.Op())
}

func (w *Window) workflowContent(gtx layout.Context) layout.Dimensions {
	children := []layout.FlexChild{layout.Rigid(w.workflowHeader), layout.Rigid(spacer(18))}
	switch w.activePage {
	case pageConnection:
		children = append(children, layout.Rigid(w.connectionPage))
	case pageOverview:
		children = append(children, layout.Rigid(w.overviewPage))
	case pageTestSettings:
		children = append(children, layout.Rigid(w.testSettingsPage))
	case pageJobProperties:
		children = append(children, layout.Rigid(w.jobPropertiesPage))
	case pageAutomation:
		children = append(children, layout.Rigid(w.automationPage))
	case pageResults:
		children = append(children, layout.Rigid(w.resultsCard))
	case pageLogs:
		children = append(children, layout.Rigid(w.logsCard))
	case pageAdministration:
		children = append(children, layout.Rigid(w.administrationCard))
	default:
		children = append(children, layout.Rigid(w.connectionPage))
	}
	children = append(children, layout.Rigid(spacer(18)))
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (w *Window) workflowHeader(gtx layout.Context) layout.Dimensions {
	pageIndex := w.activePage
	if pageIndex < 0 || pageIndex >= len(workspacePages) {
		pageIndex = pageConnection
	}
	page := workspacePages[pageIndex]
	heading := func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(w.theme, page.Title, 27, palette.text).Layout),
			layout.Rigid(spacer(4)),
			layout.Rigid(label(w.theme, page.Subtitle, 14, palette.muted).Layout),
		)
	}
	// Generic operation text does not belong in the page header. Overview owns
	// the two connection/discovery actions; other pages render only their title.
	if pageIndex != pageOverview {
		return heading(gtx)
	}
	w.mu.Lock()
	loaded := len(w.capabilities.Options) > 0
	captureActive := w.captureActive
	w.mu.Unlock()
	captureLabel := capabilityActionLabel(loaded)
	if captureActive {
		captureLabel = "Getting Capabilities..."
	}
	actions := func(gtx layout.Context) layout.Dimensions {
		return row(gtx,
			primaryButton(w.theme, &w.changeConnectionButton, "Change Server Connection"),
			primaryButton(w.theme, &w.overviewCaptureButton, captureLabel),
		)
	}
	if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(920)) {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(heading), layout.Rigid(spacer(12)), layout.Rigid(actions),
		)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, heading), layout.Rigid(actions),
	)
}

func (w *Window) connectionPage(gtx layout.Context) layout.Dimensions {
	connection := w.connectionBackend().Snapshot()
	testStatus := connection.TestStatus
	testOK := connection.TestOK
	hasActive := connection.HasActive
	activeIP := connection.ActiveIPAddress
	secretConfigured := connection.SecretConfigured
	passwordConfigured := connection.PasswordConfigured
	secretTitle, secretHint := "Secret / API key replacement", "Required"
	if secretConfigured {
		secretTitle, secretHint = "Secret / API key · Configured", "Leave blank to keep the configured key, or enter a replacement"
	}
	passwordTitle, passwordHint := "Administrator password", "Required"
	if passwordConfigured {
		passwordTitle, passwordHint = "Administrator password · Configured", "Leave blank to keep the configured password, or enter a replacement"
	}
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = minInt(gtx.Constraints.Max.X, gtx.Dp(unit.Dp(840)))
		children := []layout.FlexChild{
			layout.Rigid(label(w.theme, "Server credentials", 21, palette.text).Layout),
			layout.Rigid(spacer(5)),
			layout.Rigid(label(w.theme, "Credentials are used only for the active Fiery session. Passwords and secret keys are masked, never shown on Overview, and never saved in presets or reports.", 13, palette.muted).Layout),
			layout.Rigid(spacer(16)),
			layout.Rigid(fieldBox(w.theme, "Server address", "10.0.0.25 or https://fiery.example", &w.serverIP, 680)),
			layout.Rigid(spacer(14)),
			layout.Rigid(fieldBox(w.theme, secretTitle, secretHint, &w.secretKey, 680)),
			layout.Rigid(spacer(14)),
			layout.Rigid(fieldBox(w.theme, passwordTitle, passwordHint, &w.password, 680)),
			layout.Rigid(spacer(14)),
			layout.Rigid(statusBadge(w.theme, testStatus, serverStatusColor(testStatus))),
			layout.Rigid(spacer(14)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				buttons := []layout.Widget{primaryButton(w.theme, &w.testServerButton, "Test Connection")}
				if testOK {
					buttons = append(buttons, primaryButton(w.theme, &w.applyConnectionButton, "OK · Use Connection"))
				}
				if hasActive {
					buttons = append(buttons, secondaryButton(w.theme, &w.cancelConnectionChangeButton, "Cancel Change"))
				}
				return row(gtx, buttons...)
			}),
		}
		if hasActive {
			children = append(children,
				layout.Rigid(spacer(16)),
				layout.Rigid(label(w.theme, "Current connection remains active: "+activeIP+". Replacement details are not applied until they pass testing and you press OK.", 13, palette.muted).Layout),
			)
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
}

func (w *Window) overviewPage(gtx layout.Context) layout.Dimensions {
	automationActive := w.running.Load()
	server, _ := w.connectionBackend().Active()
	w.mu.Lock()
	capabilityModel := w.capabilities
	healthStatus, healthDetail := w.healthStatus, w.healthDetail
	healthAt, latency := w.healthCheckedAt, w.healthLatency
	summary := w.lastRun
	completed, passed, failed, errorsCount := w.resultCount, w.passedCount, w.failedCount, w.errorCount
	captureActive, captureProgress, capturePhase := w.captureActive, w.captureProgress, w.capturePhase
	w.mu.Unlock()
	healthStatus, healthDetail = effectiveOverviewServerState(healthStatus, healthDetail, automationActive && !captureActive)
	capabilityState := "Not loaded"
	capabilityDetail := "Use Get Capabilities to discover this server's writable job controls."
	if len(capabilityModel.Options) > 0 {
		capabilityState = "Ready"
		capabilityDetail = fmt.Sprintf("%d applicable · %d excluded · %d values removed · %d constrained", len(capabilityModel.Options), len(capabilityModel.ExcludedOptions), len(capabilityModel.ExcludedValues), capabilities.ConstraintCount(capabilityModel))
	}
	checked := "Waiting for status check"
	if !healthAt.IsZero() {
		checked = fmt.Sprintf("Checked %s · %s", healthAt.Local().Format("15:04:05"), latency.Round(time.Millisecond))
	}
	progress := float32(0)
	if summary.PlannedTests > 0 {
		progress = float32(completed) / float32(summary.PlannedTests)
		if progress > 1 {
			progress = 1
		}
	}
	var capabilityActivity layout.Widget
	if captureActive {
		capabilityActivity = func(gtx layout.Context) layout.Dimensions {
			phase := fallback(capturePhase, "Getting capabilities...")
			bar := material.ProgressBar(w.theme, captureProgress)
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
				layout.Rigid(label(w.theme, phase, 12, palette.primary).Layout),
				layout.Rigid(spacer(6)),
				layout.Rigid(bar.Layout),
			)
		}
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return w.serverDetailsCard(gtx, server, capabilityModel, healthStatus, healthDetail, checked)
		}),
		layout.Rigid(spacer(14)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return w.dashboardTriple(gtx,
				w.dashboardCard("Capabilities", capabilityState, capabilityDetail, capabilityActivity),
				w.dashboardCard("Automation progress", automationStatus(automationActive, summary.Status), fmt.Sprintf("%d of %d complete · %d pass · %d fail · %d error", completed, summary.PlannedTests, passed, failed, errorsCount), func(gtx layout.Context) layout.Dimensions {
					bar := material.ProgressBar(w.theme, progress)
					return bar.Layout(gtx)
				}),
				w.dashboardCard("Reset actions", "Scoped resets", "Connection, discovery, results, logs, and presets are preserved.", func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(secondaryButton(w.theme, &w.resetPropertiesButton, "Reset Job Properties")),
						layout.Rigid(spacer(6)),
						layout.Rigid(secondaryButton(w.theme, &w.resetAutomationButton, "Reset Automation")),
						layout.Rigid(spacer(6)),
						layout.Rigid(secondaryButton(w.theme, &w.resetTestSetupButton, "Reset Files")),
					)
				}),
			)
		}),
	)
}

func (w *Window) serverDetailsCard(gtx layout.Context, server model.ServerConnection, capabilityModel capabilities.Model, state, statusDetail, checked string) layout.Dimensions {
	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(w.theme, "Server Details", 20, palette.text).Layout),
			layout.Rigid(spacer(14)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return w.serverDetailRow(gtx, "Server name", fallback(capabilityModel.ServerName, "Not reported"), "IP address", fallback(server.IPAddress, "Not configured"))
			}),
			layout.Rigid(spacer(10)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return w.serverDetailRow(gtx, "Connected press", fallback(capabilityModel.PressModel, "Available after capability discovery"), "State", fallback(state, "Checking"))
			}),
			layout.Rigid(spacer(10)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return w.serverDetailRow(gtx, "Version", fallback(capabilityModel.Version, "Not reported"), "Serial number", fallback(capabilityModel.SerialNumber, "Not reported"))
			}),
			layout.Rigid(spacer(10)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return w.serverDetailRow(gtx, "Time zone", fallback(capabilityModel.TimeZone, "Not reported"), "Locale", fallback(capabilityModel.Locale, "Not reported"))
			}),
			layout.Rigid(spacer(10)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return w.serverDetailRow(gtx, "Uptime", formatServerUptime(capabilityModel.UptimeSeconds), "Last checked", checked)
			}),
			layout.Rigid(spacer(12)),
			layout.Rigid(label(w.theme, fallback(statusDetail, "Waiting for the Fiery status endpoint"), 12, palette.muted).Layout),
		)
	})
}

func (w *Window) serverDetailRow(gtx layout.Context, leftLabel, leftValue, rightLabel, rightValue string) layout.Dimensions {
	left := w.serverDetailField(leftLabel, leftValue)
	right := w.serverDetailField(rightLabel, rightValue)
	if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(760)) {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(left), layout.Rigid(spacer(8)), layout.Rigid(right),
		)
	}
	return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
		layout.Flexed(1, left), layout.Rigid(spacerX(22)), layout.Flexed(1, right),
	)
}

func (w *Window) serverDetailField(name, value string) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
			layout.Rigid(label(w.theme, name+":", 14, palette.muted).Layout),
			layout.Rigid(spacerX(8)),
			layout.Flexed(1, label(w.theme, fallback(value, "Not reported"), 16, palette.text).Layout),
		)
	}
}

func formatServerUptime(seconds int64) string {
	if seconds <= 0 {
		return "Not reported"
	}
	duration := time.Duration(seconds) * time.Second
	days := duration / (24 * time.Hour)
	duration %= 24 * time.Hour
	if days > 0 {
		return fmt.Sprintf("%dd %dh %dm", days, duration/time.Hour, (duration%time.Hour)/time.Minute)
	}
	return fmt.Sprintf("%dh %dm", duration/time.Hour, (duration%time.Hour)/time.Minute)
}

func (w *Window) jobAutomationActive() bool {
	if !w.running.Load() {
		return false
	}
	w.mu.Lock()
	capabilityCaptureActive := w.captureActive
	w.mu.Unlock()
	return !capabilityCaptureActive
}

func effectiveOverviewServerStateWithJobs(apiState, apiDetail string, workload fiery.JobWorkloadSummary) (string, string) {
	return application.EffectiveOverviewServerStateWithJobs(apiState, apiDetail, workload)
}

func effectiveOverviewServerState(apiState, apiDetail string, automationActive bool) (string, string) {
	return application.EffectiveOverviewServerState(apiState, apiDetail, automationActive)
}

func automationStatus(running bool, status string) string {
	if running {
		return "Running"
	}
	if strings.TrimSpace(status) == "" {
		return "No run started"
	}
	return status
}

func capabilityActionLabel(loaded bool) string {
	if loaded {
		return "Refresh Capabilities"
	}
	return "Get Capabilities"
}

func (w *Window) dashboardCard(title, value, detail string, action layout.Widget) layout.Widget {
	return func(gtx layout.Context) layout.Dimensions {
		return card(gtx, func(gtx layout.Context) layout.Dimensions {
			children := []layout.FlexChild{
				layout.Rigid(label(w.theme, strings.ToUpper(title), 11, palette.muted).Layout),
				layout.Rigid(spacer(8)),
				layout.Rigid(label(w.theme, fallback(value, "Not available"), 18, palette.text).Layout),
				layout.Rigid(spacer(5)),
				layout.Rigid(label(w.theme, fallback(detail, "No additional details"), 12, palette.muted).Layout),
			}
			if action != nil {
				children = append(children, layout.Rigid(spacer(12)), layout.Rigid(action))
			}
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
		})
	}
}

func (w *Window) dashboardTriple(gtx layout.Context, first, second, third layout.Widget) layout.Dimensions {
	if gtx.Constraints.Max.X < gtx.Dp(unit.Dp(900)) {
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(first), layout.Rigid(spacer(14)),
			layout.Rigid(second), layout.Rigid(spacer(14)),
			layout.Rigid(third),
		)
	}
	return layout.Flex{Alignment: layout.Start}.Layout(gtx,
		layout.Flexed(1, first), layout.Rigid(spacerX(14)),
		layout.Flexed(1, second), layout.Rigid(spacerX(14)),
		layout.Flexed(1, third),
	)
}

func (w *Window) testSettingsPage(gtx layout.Context) layout.Dimensions {
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(label(w.theme, "Test file source", 20, palette.text).Layout),
					layout.Rigid(spacer(5)),
					layout.Rigid(label(w.theme, "Choose a folder containing supported print files. Single file must be inside that folder; Random file is chosen by the application for each run.", 13, palette.muted).Layout),
					layout.Rigid(spacer(14)),
					layout.Rigid(fieldBox(w.theme, "Test folder", "Choose a folder", &w.folderPath, 760)),
					layout.Rigid(spacer(9)),
					layout.Rigid(browseButton(w.theme, &w.browseFolderButton, "Browse Folder")),
					layout.Rigid(spacer(14)),
					layout.Rigid(fieldBox(w.theme, "Specific file", "Required only for Specific file", &w.filePath, 760)),
					layout.Rigid(spacer(9)),
					layout.Rigid(browseButton(w.theme, &w.browseFileButton, "Browse File")),
					layout.Rigid(spacer(14)),
					layout.Rigid(w.fileSelectionRadioGroup),
				)
			})
		}),
	)
}

func (w *Window) jobPropertiesPage(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	model := w.capabilities
	w.mu.Unlock()
	children := []layout.FlexChild{}
	if len(model.Options) == 0 {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return card(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(label(w.theme, "Capabilities are required", 20, palette.text).Layout),
						layout.Rigid(spacer(5)),
						layout.Rigid(label(w.theme, "Use Get Capabilities in the Overview header before selecting Job Properties. Existing selections are invalidated whenever the server connection changes.", 13, palette.muted).Layout),
					)
				})
			}),
		)
	} else {
		children = append(children,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.presetPanel(gtx) }),
			layout.Rigid(spacer(12)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.serverPresetPanel(gtx, model) }),
			layout.Rigid(spacer(12)),
			layout.Rigid(w.capabilityToolbar),
			layout.Rigid(spacer(10)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.categoryTabs(gtx, model) }),
			layout.Rigid(spacer(10)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return label(w.theme, fmt.Sprintf("%d value(s) selected · exact Fiery property IDs are preserved", w.selectedPropertyValueCount()), 13, palette.muted).Layout(gtx)
			}),
			layout.Rigid(spacer(10)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions { return w.optionGrid(gtx, model) }),
		)
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
}

func (w *Window) automationPage(gtx layout.Context) layout.Dimensions {
	preview, _, previewErr := w.selectedCombinations()
	previewText := fmt.Sprintf("%d configuration(s) generated", len(preview))
	if previewErr != nil {
		previewText = "Configuration needs attention: " + previewErr.Error()
	}
	return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(label(w.theme, "1. Test intent", 19, palette.text).Layout),
					layout.Rigid(spacer(8)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return row(gtx,
							toggle(w.theme, &w.positiveIntentButton, "Positive Validation", w.testIntent == testIntentPositive),
							toggle(w.theme, &w.constraintIntentButton, "Expected Constraint Rejection", w.testIntent == testIntentConstraint),
						)
					}),
					layout.Rigid(spacer(7)),
					layout.Rigid(label(w.theme, testIntentDescription(w.testIntent), 13, palette.muted).Layout),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						if w.testIntent != testIntentConstraint {
							return layout.Dimensions{}
						}
						return layout.Inset{Top: unit.Dp(10)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
							return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
								layout.Rigid(func(gtx layout.Context) layout.Dimensions {
									return row(gtx,
										toggle(w.theme, &w.validationOnlyButton, "Validation Only · recommended", w.constraintMode == constraintValidationOnly),
										toggle(w.theme, &w.controlledApplyButton, "Controlled Apply", w.constraintMode == constraintControlledApply),
									)
								}),
								layout.Rigid(spacer(6)),
								layout.Rigid(label(w.theme, constraintModeDescription(w.constraintMode), 12, palette.muted).Layout),
							)
						})
					}),
				)
			})
		}),
		layout.Rigid(spacer(12)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(label(w.theme, "2. Value source", 19, palette.text).Layout),
					layout.Rigid(spacer(8)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return row(gtx,
							toggle(w.theme, &w.baselineSourceButton, "Server Baseline", w.valueSource == valueSourceBaseline),
							toggle(w.theme, &w.defaultsSourceButton, "Advertised Defaults", w.valueSource == valueSourceDefaults),
							toggle(w.theme, &w.selectedSourceButton, "User-Selected Values", w.valueSource == valueSourceSelected),
							toggle(w.theme, &w.advertisedSourceButton, "All Advertised Values", w.valueSource == valueSourceAdvertised),
						)
					}),
					layout.Rigid(spacer(7)),
					layout.Rigid(label(w.theme, valueSourceDescription(w.valueSource), 13, palette.muted).Layout),
				)
			})
		}),
		layout.Rigid(spacer(12)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(label(w.theme, "3. Case generation", 19, palette.text).Layout),
					layout.Rigid(spacer(8)),
					layout.Rigid(w.strategySelector),
					layout.Rigid(spacer(7)),
					layout.Rigid(label(w.theme, generationDescription(w.strategy), 13, palette.muted).Layout),
				)
			})
		}),
		layout.Rigid(spacer(12)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return card(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
					layout.Rigid(label(w.theme, "4. Lifecycle and concurrency", 19, palette.text).Layout),
					layout.Rigid(spacer(8)),
					layout.Rigid(w.runModeRadioGroup),
					layout.Rigid(spacer(12)),
					layout.Rigid(func(gtx layout.Context) layout.Dimensions {
						return row(gtx,
							field(w.theme, "Parallel jobs (1–10)", &w.workers, 170),
							browseButton(w.theme, &w.apiTraceButton, "Capture API Trace"),
						)
					}),
				)
			})
		}),
		layout.Rigid(spacer(12)),
		layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			return surfaceAlt(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.Flex{Alignment: layout.Middle}.Layout(gtx,
					layout.Flexed(1, func(gtx layout.Context) layout.Dimensions {
						return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
							layout.Rigid(label(w.theme, "Plan preview", 15, palette.text).Layout),
							layout.Rigid(spacer(3)),
							layout.Rigid(label(w.theme, previewText, 13, previewColor(previewErr)).Layout),
						)
					}),
					layout.Rigid(primaryButton(w.theme, &w.runButton, "Run Automation")),
					layout.Rigid(spacerX(10)),
					layout.Rigid(secondaryButton(w.theme, &w.cancelButton, "Cancel Run")),
				)
			})
		}),
	)
}

func previewColor(err error) color.NRGBA {
	if err != nil {
		return palette.danger
	}
	return palette.success
}

func (w *Window) selectedPropertyValueCount() int {
	count := 0
	for id, values := range w.selected {
		if isCopiesOption(id) {
			continue
		}
		for _, selected := range values {
			if selected != nil && selected.Value {
				count++
			}
		}
	}
	for _, input := range w.numericInputs {
		if input != nil && strings.TrimSpace(input.Text()) != "" {
			count++
		}
	}
	return count
}

func (w *Window) resetJobProperties() {
	if w.running.Load() {
		w.setStatus("Wait for automation to finish before resetting Job Properties.")
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
	for _, input := range w.numericInputs {
		input.SetText("")
	}
	w.copiesInput.SetText("1")
	w.pageRangeInput.SetText("")
	w.serverPresetGroup.Value = noServerPresetID
	w.capabilitySearch.SetText("")
	w.activeCapabilityGroup = "Job Info"
	w.setStatus("Job Properties reset. Files, automation settings, connection, discovery, and saved presets were preserved.")
}

func (w *Window) resetAutomationSettings() {
	if w.running.Load() {
		w.setStatus("Wait for automation to finish before resetting automation settings.")
		return
	}
	w.strategy = combinations.StrategySingle
	w.valueSource = valueSourceSelected
	w.testIntent = testIntentPositive
	w.constraintMode = constraintValidationOnly
	w.workers.SetText("1")
	w.maxCases.SetText(fmt.Sprint(defaultCaseLimit))
	for index := range w.modeChecks {
		w.modeChecks[index].Value = index == 0
	}
	w.setStatus("Automation settings reset. Files, Job Properties, connection, and discovery were preserved.")
}

func (w *Window) resetTestSetup() {
	if w.running.Load() {
		w.setStatus("Wait for automation to finish before resetting test files.")
		return
	}
	w.folderPath.SetText("")
	w.filePath.SetText("")
	w.fileModeGroup.Value = "all"
	w.setStatus("Test file selection reset. Connection, discovery, Job Properties, and automation settings were preserved.")
}

func valueSourceLabel(source automationValueSource) string {
	switch source {
	case valueSourceBaseline:
		return "Server Baseline"
	case valueSourceDefaults:
		return "Advertised Defaults"
	case valueSourceAdvertised:
		return "All Advertised Values"
	default:
		return "User-Selected Values"
	}
}

func valueSourceDescription(source automationValueSource) string {
	switch source {
	case valueSourceBaseline:
		return "Imports and runs lifecycle checks without sending explicit Job Property updates. A selected Fiery server preset is also omitted."
	case valueSourceDefaults:
		return "Uses each included property's server-advertised default as one explicit value."
	case valueSourceAdvertised:
		return "Uses all server-advertised values for included properties; Max cases always bounds generation."
	default:
		return "Uses only values and validated numeric inputs explicitly chosen on Job Properties."
	}
}

func testIntentLabel(intent automationTestIntent) string {
	if intent == testIntentConstraint {
		return "Expected Constraint Rejection"
	}
	return "Positive Validation"
}

func testIntentDescription(intent automationTestIntent) string {
	if intent == testIntentConstraint {
		return "Generates explicitly incompatible selected values. An expected constraint rejection is PASS; timeout, HTTP 500, unrelated rejection, or server failure is ERROR."
	}
	return "Generates compatible configurations and requires lifecycle plus strict Set/Get verification to pass."
}

func constraintModeDescription(mode constraintTestMode) string {
	if mode == constraintControlledApply {
		return "Advanced: sends the incompatible update only after local metadata proves the conflict. Use disposable jobs on an isolated server."
	}
	return "Recommended: imports a disposable held job and asks Fiery's constraint endpoint to validate the selected values without applying them."
}

func strategyLabel(strategy combinations.Strategy) string {
	switch strategy {
	case combinations.StrategySingle:
		return "Single Configuration"
	case combinations.StrategyAll, combinations.StrategySelected:
		return "All Combinations"
	case combinations.StrategyPairwise:
		return "Pairwise"
	case combinations.StrategyRandom:
		return "Bounded Random Sample"
	default:
		return string(strategy)
	}
}

func generationDescription(strategy combinations.Strategy) string {
	switch strategy {
	case combinations.StrategySingle:
		return "Creates one configuration using the first value on every included property."
	case combinations.StrategyPairwise:
		return "Covers value pairs with a memory-safe reduced set rather than the complete Cartesian product."
	case combinations.StrategyRandom:
		return "Samples unique Cartesian positions directly without materializing the complete product."
	default:
		return "Creates the bounded Cartesian product (all combinations), not mathematical permutations."
	}
}

func (w *Window) stopOverviewHealthMonitor() {
	w.mu.Lock()
	cancel := w.healthCancel
	w.healthCancel = nil
	if w.healthGuard == nil {
		w.healthGuard = new(application.GenerationGuard)
	}
	w.healthGuard.Next()
	w.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

var overviewMonitorPolicy = application.DefaultOverviewMonitorPolicy()

var (
	overviewStatusPollInterval = overviewMonitorPolicy.StatusInterval
	overviewJobPollInterval    = overviewMonitorPolicy.JobInterval
	overviewJobProbeLimit      = overviewMonitorPolicy.JobProbeLimit
)

func (w *Window) startOverviewHealthMonitor() {
	server, ok := w.server()
	if !ok {
		return
	}
	ctx, cancel := context.WithCancel(w.rootContext())
	w.mu.Lock()
	if w.healthGuard == nil {
		w.healthGuard = new(application.GenerationGuard)
	}
	generation := w.healthGuard.Next()
	w.healthCancel = cancel
	w.healthStatus = "Checking"
	w.healthDetail = "Contacting the Fiery status endpoint."
	w.mu.Unlock()
	w.diagnostic.printf("OVERVIEW_STATUS_MONITOR: endpoint=/live/api/v5/status interval=%s", overviewStatusPollInterval)
	w.launchBackground("Overview server health monitor", func() {
		w.monitorServerHealth(ctx, generation, server)
	})
}

func (w *Window) monitorServerHealth(ctx context.Context, generation uint64, server model.ServerConnection) {
	client, newErr := fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
	if newErr != nil {
		w.setHealthSnapshot(generation, "Unavailable", newErr.Error(), 0)
		return
	}
	var session fiery.Session
	failures := 0
	jobProbeFailures := 0
	var observedCapabilityGeneration uint64
	var jobWorkload fiery.JobWorkloadSummary
	nextJobProbe := time.Now()
	for {
		w.mu.Lock()
		if w.capabilityGuard == nil {
			w.capabilityGuard = new(application.GenerationGuard)
		}
		capabilityGeneration := w.capabilityGuard.Current()
		capabilityModel := w.capabilities
		w.mu.Unlock()
		capabilitiesLoaded := len(capabilityModel.Options) > 0
		if capabilityGeneration != observedCapabilityGeneration {
			observedCapabilityGeneration = capabilityGeneration
			jobWorkload = fiery.JobWorkloadSummary{
				TotalItems: capabilityModel.JobsTotal, ActiveJobs: capabilityModel.ActiveJobs,
				EvidenceID: capabilityModel.ActiveJobID, EvidenceStatus: capabilityModel.ActiveJobStatus,
				EvidenceState: capabilityModel.ActiveJobState,
			}
			nextJobProbe = time.Now()
			jobProbeFailures = 0
		}

		started := time.Now()
		attemptContext, cancel := context.WithTimeout(ctx, 20*time.Second)
		var activity fiery.ServerActivityStatus
		var err error
		if session.Cookie == "" {
			session, err = client.Login(attemptContext)
		}
		if err == nil {
			activity, err = client.ServerActivityStatus(attemptContext, session)
		}
		if err == nil && capabilitiesLoaded && !time.Now().Before(nextJobProbe) {
			probe, probeErr := client.ProbeRecentJobWorkload(attemptContext, session, jobWorkload.TotalItems, overviewJobProbeLimit)
			if probeErr != nil {
				// If this Fiery ignored pagination but still returned a valid
				// inventory, use that evidence once and back off aggressively rather
				// than discarding externally started job activity.
				if probe.InspectedItems > 0 {
					jobWorkload = probe
				}
				jobProbeFailures++
				probeDelay := application.FailureBackoff(jobProbeFailures, overviewJobPollInterval, overviewMonitorPolicy.MaximumBackoff)
				nextJobProbe = time.Now().Add(probeDelay)
				w.diagnostic.printf("OVERVIEW_JOB_POLL: endpoint=/live/api/v5/jobs bounded=true result=error failures=%d next=%s error=%q", jobProbeFailures, probeDelay, short(probeErr.Error(), 220))
			} else {
				jobProbeFailures = 0
				jobWorkload = probe
				nextJobProbe = time.Now().Add(overviewJobPollInterval)
				w.diagnostic.printf("OVERVIEW_JOB_POLL: endpoint=/live/api/v5/jobs bounded=true result=ok offset=%d limit=%d inspected=%d total=%d active=%d evidence_id=%q evidence_status=%q evidence_state=%q next=%s", probe.Offset, overviewJobProbeLimit, probe.InspectedItems, probe.TotalItems, probe.ActiveJobs, probe.EvidenceID, probe.EvidenceStatus, probe.EvidenceState, overviewJobPollInterval)
			}
		}
		cancel()
		latency := time.Since(started)
		delay := overviewStatusPollInterval
		if err != nil {
			session = fiery.Session{}
			failures++
			delay = application.FailureBackoff(failures, overviewStatusPollInterval, overviewMonitorPolicy.MaximumBackoff)
			w.diagnostic.printf("OVERVIEW_STATUS_POLL: endpoint=/live/api/v5/status result=error failures=%d next=%s latency=%s error=%q", failures, delay, latency.Round(time.Millisecond), short(err.Error(), 220))
			w.setHealthSnapshot(generation, "Unavailable", short(err.Error(), 220), latency)
		} else {
			failures = 0
			apiState := fallback(activity.Workload, "Idle")
			detail := fmt.Sprintf("API /status health %s · extended status %s", fallback(activity.Health, "not reported"), fallback(activity.Extended, "not reported"))
			state, detail := effectiveOverviewServerStateWithJobs(apiState, detail, jobWorkload)
			jobAutomationActive := w.jobAutomationActive()
			state, detail = effectiveOverviewServerState(state, detail, jobAutomationActive)
			w.diagnostic.printf("OVERVIEW_STATUS_POLL: endpoint=/live/api/v5/status result=ok health=%q extended=%q api_workload=%q inventory_active=%d inventory_total=%d inventory_evidence=%q job_automation_active=%t displayed=%q next=%s latency=%s", activity.Health, activity.Extended, apiState, jobWorkload.ActiveJobs, jobWorkload.TotalItems, jobWorkload.EvidenceStatus, jobAutomationActive, state, delay, latency.Round(time.Millisecond))
			w.setHealthSnapshot(generation, state, detail, latency)
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			return
		case <-timer.C:
		}
	}
}

func (w *Window) setHealthSnapshot(generation uint64, status, detail string, latency time.Duration) {
	w.mu.Lock()
	if w.healthGuard == nil || !w.healthGuard.IsCurrent(generation) {
		w.mu.Unlock()
		return
	}
	w.healthStatus = status
	w.healthDetail = detail
	w.healthCheckedAt = time.Now()
	w.healthLatency = latency
	w.mu.Unlock()
	w.invalidate()
}
