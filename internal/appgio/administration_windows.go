//go:build windows

package appgio

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"api-automation/internal/fiery"
	"api-automation/internal/model"

	"gioui.org/layout"
)

const (
	clearAllJobsConfirmation = "CLEAR ALL JOBS"
	jobInventoryValidity     = 2 * time.Minute
)

func (w *Window) administrationCard(gtx layout.Context) layout.Dimensions {
	w.mu.Lock()
	adminStatus := w.adminStatus
	inventoryServer := w.adminInventoryServer
	inventoryAt := w.adminInventoryAt
	jobCount := w.adminJobCount
	w.mu.Unlock()

	inventory := "Not inspected"
	if inventoryServer != "" && !inventoryAt.IsZero() {
		inventory = fmt.Sprintf("%d job(s) on %s · inspected %s", jobCount, inventoryServer, inventoryAt.Local().Format("15:04:05"))
		if time.Since(inventoryAt) > jobInventoryValidity {
			inventory += " · expired; inspect again"
		}
	}

	return card(gtx, func(gtx layout.Context) layout.Dimensions {
		gtx.Constraints.Max.X = minInt(gtx.Constraints.Max.X, gtx.Dp(900))
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
			layout.Rigid(label(w.theme, adminStatus, 14, palette.primary).Layout),
			layout.Rigid(spacer(14)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return formPanel(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(label(w.theme, "Fiery process control", 20, palette.text).Layout),
						layout.Rigid(spacer(5)),
						layout.Rigid(label(w.theme, "Restart affects Fiery software. Reboot restarts the complete server and is supported only on applicable platforms. Both actions require confirmation, are blocked during concurrent automation, and monitor recovery by re-authenticating.", 13, palette.muted).Layout),
						layout.Rigid(spacer(12)),
						layout.Rigid(func(gtx layout.Context) layout.Dimensions {
							return row(gtx,
								secondaryButton(w.theme, &w.restartServerButton, "Restart Fiery process"),
								dangerButton(w.theme, &w.rebootServerButton, "Reboot server"),
							)
						}),
					)
				})
			}),
			layout.Rigid(spacer(18)),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				return formPanel(gtx, func(gtx layout.Context) layout.Dimensions {
					return layout.Flex{Axis: layout.Vertical}.Layout(gtx,
						layout.Rigid(label(w.theme, "Clear all jobs", 20, palette.danger).Layout),
						layout.Rigid(spacer(5)),
						layout.Rigid(label(w.theme, "This permanently removes every user's jobs from the connected Fiery. It clears only the jobs service; accounting and configuration are never requested.", 13, palette.muted).Layout),
						layout.Rigid(spacer(10)),
						layout.Rigid(label(w.theme, "Inventory: "+inventory, 13, palette.text).Layout),
						layout.Rigid(spacer(10)),
						layout.Rigid(secondaryButton(w.theme, &w.inspectJobsButton, "Inspect current job count")),
						layout.Rigid(spacer(12)),
						layout.Rigid(fieldBox(w.theme, "Typed confirmation", clearAllJobsConfirmation, &w.adminConfirmation, 620)),
						layout.Rigid(spacer(5)),
						layout.Rigid(label(w.theme, "After inspecting, type the exact uppercase phrase shown above. A second native confirmation will include the verified server and job count.", 12, palette.muted).Layout),
						layout.Rigid(spacer(12)),
						layout.Rigid(dangerButton(w.theme, &w.clearAllJobsButton, "Permanently clear all jobs")),
					)
				})
			}),
		)
	})
}

func (w *Window) inspectServerJobs() {
	if err := w.serverAdministrationPrecondition(); err != nil {
		w.setAdminStatus(err.Error())
		return
	}
	server, ok := w.server()
	if !ok || !w.inspectingJobs.CompareAndSwap(false, true) {
		return
	}
	w.setAdminStatus("Inspecting Fiery job count...")
	w.launchBackground("Fiery job inventory", func() {
		defer func() {
			w.inspectingJobs.Store(false)
			w.invalidate()
		}()
		ctx, cancel := context.WithTimeout(w.rootContext(), 60*time.Second)
		defer cancel()
		client, err := newFieryClient(server)
		if err != nil {
			w.setAdminStatus("Job inspection failed: " + err.Error())
			return
		}
		session, err := client.Login(ctx)
		if err != nil {
			w.setAdminStatus("Job inspection login failed: " + err.Error())
			return
		}
		jobs, err := client.ListJobs(ctx, session)
		if err != nil {
			w.setAdminStatus("Job inspection failed: " + err.Error())
			return
		}
		w.recordJobInventory(server.IPAddress, len(jobs))
		w.setAdminStatus(fmt.Sprintf("Inspection complete: %d job(s) currently exist on %s.", len(jobs), server.IPAddress))
		w.addLog("SERVER_ADMIN action=inspect_jobs server=%s jobs=%d result=PASS", server.IPAddress, len(jobs))
	})
}

func (w *Window) startServerControl(action string) {
	if err := w.serverAdministrationPrecondition(); err != nil {
		w.setAdminStatus(err.Error())
		return
	}
	server, ok := w.server()
	if !ok {
		return
	}
	var title, message string
	switch action {
	case "restart":
		title = "Restart Fiery process"
		message = fmt.Sprintf("Restart Fiery software on %s?\n\nActive Fiery processing may be interrupted. The application will monitor API recovery.", server.IPAddress)
	case "reboot":
		title = "Reboot Fiery server"
		message = fmt.Sprintf("Reboot the complete Fiery server %s?\n\nAll server services will become unavailable. This operation is supported only by applicable Fiery platforms.", server.IPAddress)
	default:
		w.setAdminStatus("Unsupported server control action: " + action)
		return
	}
	confirmed, err := confirmDestructiveAction(title, message)
	if err != nil {
		w.setAdminStatus("Could not open server action confirmation: " + err.Error())
		return
	}
	if !confirmed || !w.managingServer.CompareAndSwap(false, true) {
		return
	}
	w.invalidateJobInventory()
	w.setAdminStatus(fmt.Sprintf("Sending %s request to %s...", action, server.IPAddress))
	w.launchBackground("Fiery server "+action, func() {
		defer func() {
			w.managingServer.Store(false)
			w.invalidate()
		}()
		client, err := newFieryClient(server)
		if err != nil {
			w.finishServerControl(action, server.IPAddress, false, err)
			return
		}
		requestContext, cancel := context.WithTimeout(w.rootContext(), 60*time.Second)
		session, err := client.Login(requestContext)
		if err == nil {
			switch action {
			case "restart":
				err = client.RestartFieryProcess(requestContext, session)
			case "reboot":
				err = client.RebootServer(requestContext, session)
			}
		}
		cancel()
		if err != nil {
			w.finishServerControl(action, server.IPAddress, false, err)
			return
		}
		w.addLog("SERVER_ADMIN action=%s server=%s request=ACCEPTED", action, server.IPAddress)
		w.setAdminStatus(fmt.Sprintf("%s request accepted. Waiting for %s to become API-ready...", serverActionTitle(action), server.IPAddress))
		recoveryTimeout := 8 * time.Minute
		if action == "reboot" {
			recoveryTimeout = 12 * time.Minute
		}
		recoveryContext, recoveryCancel := context.WithTimeout(w.rootContext(), recoveryTimeout)
		err = waitForFieryRecovery(recoveryContext, server)
		recoveryCancel()
		w.finishServerControl(action, server.IPAddress, true, err)
	})
}

func (w *Window) startClearAllJobs() {
	if err := w.serverAdministrationPrecondition(); err != nil {
		w.setAdminStatus(err.Error())
		return
	}
	server, ok := w.server()
	if !ok {
		return
	}
	if w.adminConfirmation.Text() != clearAllJobsConfirmation {
		w.setAdminStatus("Clear all jobs blocked: enter the exact uppercase confirmation phrase.")
		return
	}
	w.mu.Lock()
	inventoryServer := w.adminInventoryServer
	inventoryAt := w.adminInventoryAt
	jobCount := w.adminJobCount
	w.mu.Unlock()
	if inventoryServer != server.IPAddress || inventoryAt.IsZero() || time.Since(inventoryAt) > jobInventoryValidity {
		w.setAdminStatus("Clear all jobs blocked: inspect the current job count for this server first.")
		return
	}
	if jobCount == 0 {
		w.setAdminStatus("There are no inspected jobs to clear.")
		return
	}
	confirmed, err := confirmDestructiveAction(
		"Permanently clear all Fiery jobs",
		fmt.Sprintf("Permanently remove all %d job(s) currently inspected on %s?\n\nThis affects every user and cannot be undone. Accounting and configuration will not be cleared.", jobCount, server.IPAddress),
	)
	if err != nil {
		w.setAdminStatus("Could not open clear-jobs confirmation: " + err.Error())
		return
	}
	if !confirmed || !w.managingServer.CompareAndSwap(false, true) {
		return
	}
	// Consume the typed phrase on the Gio event thread. Any retry—including a
	// changed inventory—requires the operator to type it again.
	w.adminConfirmation.SetText("")
	w.setAdminStatus(fmt.Sprintf("Revalidating %d job(s) on %s before clearing...", jobCount, server.IPAddress))
	w.launchBackground("Clear all Fiery jobs", func() {
		defer func() {
			w.managingServer.Store(false)
			w.invalidate()
		}()
		ctx, cancel := context.WithTimeout(w.rootContext(), 3*time.Minute)
		defer cancel()
		client, err := newFieryClient(server)
		if err != nil {
			w.finishClearAllJobs(server.IPAddress, false, err)
			return
		}
		session, err := client.Login(ctx)
		if err != nil {
			w.finishClearAllJobs(server.IPAddress, false, fmt.Errorf("login: %w", err))
			return
		}
		jobs, err := client.ListJobs(ctx, session)
		if err != nil {
			w.finishClearAllJobs(server.IPAddress, false, fmt.Errorf("revalidate job inventory: %w", err))
			return
		}
		if len(jobs) != jobCount {
			w.recordJobInventory(server.IPAddress, len(jobs))
			w.finishClearAllJobs(server.IPAddress, false, fmt.Errorf("job count changed from %d to %d; inspect and confirm again", jobCount, len(jobs)))
			return
		}
		if err := client.ClearAllJobs(ctx, session); err != nil {
			w.finishClearAllJobs(server.IPAddress, false, err)
			return
		}
		remaining, err := waitForEmptyJobInventory(ctx, client, session)
		w.recordJobInventory(server.IPAddress, remaining)
		if err != nil {
			w.finishClearAllJobs(server.IPAddress, true, err)
			return
		}
		w.finishClearAllJobs(server.IPAddress, true, nil)
	})
}

func (w *Window) serverAdministrationPrecondition() error {
	switch {
	case w.running.Load():
		return errors.New("server administration is blocked while capability capture or automation is running")
	case w.managingJob.Load():
		return errors.New("server administration is blocked while a manual job action is running")
	case w.testingServer.Load():
		return errors.New("server administration is blocked while the connection test is running")
	case w.managingServer.Load() || w.inspectingJobs.Load():
		return errors.New("wait for the current server administration operation to finish")
	default:
		return nil
	}
}

func (w *Window) finishServerControl(action, server string, accepted bool, err error) {
	if err != nil {
		if accepted {
			w.setAdminStatus(fmt.Sprintf("%s was accepted, but API recovery was not confirmed: %v", serverActionTitle(action), err))
			w.addLog("SERVER_ADMIN action=%s server=%s result=WARNING recovery_error=%v", action, server, err)
			return
		}
		w.setAdminStatus(fmt.Sprintf("Could not %s %s: %v", action, server, err))
		w.addLog("SERVER_ADMIN action=%s server=%s result=ERROR error=%v", action, server, err)
		return
	}
	w.setAdminStatus(fmt.Sprintf("%s completed and %s is API-ready.", serverActionTitle(action), server))
	w.addLog("SERVER_ADMIN action=%s server=%s result=PASS recovery=confirmed", action, server)
}

func serverActionTitle(action string) string {
	switch action {
	case "restart":
		return "Restart"
	case "reboot":
		return "Reboot"
	default:
		return action
	}
}

func (w *Window) finishClearAllJobs(server string, accepted bool, err error) {
	if err != nil {
		if accepted {
			w.setAdminStatus("Clear all jobs was accepted, but an empty inventory was not verified: " + err.Error())
			w.addLog("SERVER_ADMIN action=clear_jobs server=%s result=WARNING request=ACCEPTED verification_error=%v", server, err)
			return
		}
		w.setAdminStatus("Clear all jobs did not complete: " + err.Error())
		w.addLog("SERVER_ADMIN action=clear_jobs server=%s result=ERROR error=%v", server, err)
		return
	}
	w.setAdminStatus(fmt.Sprintf("All jobs were cleared from %s and an empty inventory was verified.", server))
	w.addLog("SERVER_ADMIN action=clear_jobs server=%s result=PASS remaining_jobs=0", server)
}

func (w *Window) setAdminStatus(status string) {
	w.mu.Lock()
	w.adminStatus = status
	w.mu.Unlock()
	w.setStatus(status)
}

func (w *Window) recordJobInventory(server string, count int) {
	w.mu.Lock()
	w.adminInventoryServer = server
	w.adminInventoryAt = time.Now()
	w.adminJobCount = count
	w.mu.Unlock()
	w.invalidate()
}

func (w *Window) invalidateJobInventory() {
	w.mu.Lock()
	w.adminInventoryServer = ""
	w.adminInventoryAt = time.Time{}
	w.adminJobCount = 0
	w.mu.Unlock()
}

func newFieryClient(server model.ServerConnection) (*fiery.Client, error) {
	return fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
}

func waitForFieryRecovery(ctx context.Context, server model.ServerConnection) error {
	// Avoid treating the still-running pre-action process as recovered before the
	// restart/reboot has had time to begin.
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(10 * time.Second):
	}
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	var lastErr error
	for {
		attemptContext, cancel := context.WithTimeout(ctx, 25*time.Second)
		client, err := newFieryClient(server)
		if err == nil {
			var session fiery.Session
			session, err = client.Login(attemptContext)
			if err == nil {
				var status string
				status, err = client.ServerStatus(attemptContext, session)
				if err == nil {
					normalized := strings.ToLower(strings.TrimSpace(status))
					if normalized == "running" || normalized == "started" || normalized == "ready" {
						cancel()
						return nil
					}
					err = fmt.Errorf("fiery status is %q", status)
				}
			}
		}
		cancel()
		lastErr = err
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("%w (last recovery check: %v)", ctx.Err(), lastErr)
			}
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func waitForEmptyJobInventory(ctx context.Context, client *fiery.Client, session fiery.Session) (int, error) {
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()
	remaining := -1
	for {
		jobs, err := client.ListJobs(ctx, session)
		if err == nil {
			remaining = len(jobs)
			if remaining == 0 {
				return 0, nil
			}
		}
		select {
		case <-ctx.Done():
			if err != nil {
				return max(remaining, 0), fmt.Errorf("verify empty job inventory: %w", err)
			}
			return max(remaining, 0), fmt.Errorf("verify empty job inventory: %w with %d job(s) remaining", ctx.Err(), remaining)
		case <-ticker.C:
		}
	}
}
