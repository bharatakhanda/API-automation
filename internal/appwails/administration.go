package appwails

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	core "api-automation/internal/application"
	"api-automation/internal/fiery"
	"api-automation/internal/model"
)

type AdministrationView struct {
	Inventory core.InventorySnapshot `json:"inventory"`
	Message   string                 `json:"message"`
	Accepted  bool                   `json:"accepted"`
}

type JobActionResult struct {
	JobID   string `json:"jobId"`
	Action  string `json:"action"`
	Message string `json:"message"`
}

func (service *Service) AdministrationState() AdministrationView {
	return AdministrationView{Inventory: service.administration.Inventory(), Message: "Administration is ready."}
}

func (service *Service) InspectJobs(ctx context.Context) (AdministrationView, error) {
	service.operationMu.Lock()
	defer service.operationMu.Unlock()
	if err := service.administrationPrecondition(); err != nil {
		return AdministrationView{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	client, session, server, err := service.authenticatedClient(ctx)
	if err != nil {
		return AdministrationView{}, redactError(err, server.SecretKey, server.Password)
	}
	jobs, err := client.ListJobs(ctx, session)
	if err != nil {
		return AdministrationView{}, redactError(err, server.SecretKey, server.Password)
	}
	inventory := service.administration.RecordInventory(server.IPAddress, len(jobs), time.Now())
	service.diagnostic.Printf("SERVER_ADMIN action=inspect_jobs server=%s jobs=%d result=PASS", server.IPAddress, len(jobs))
	return AdministrationView{Inventory: inventory, Message: fmt.Sprintf("Inspection complete: %d job(s) currently exist on %s.", len(jobs), server.IPAddress)}, nil
}

func (service *Service) ControlServer(ctx context.Context, action string) (AdministrationView, error) {
	service.operationMu.Lock()
	defer service.operationMu.Unlock()
	if err := service.administrationPrecondition(); err != nil {
		return AdministrationView{}, err
	}
	server, ok := service.connection.Active()
	if !ok {
		return AdministrationView{}, errors.New("test and apply a server connection first")
	}
	var title, message string
	switch action {
	case "restart":
		title = "Restart Fiery process"
		message = fmt.Sprintf("Restart Fiery software on %s?\n\nActive Fiery processing may be interrupted. The preview will monitor API recovery.", server.IPAddress)
	case "reboot":
		title = "Reboot Fiery server"
		message = fmt.Sprintf("Reboot the complete Fiery server %s?\n\nAll server services will become unavailable. This is supported only by applicable Fiery platforms.", server.IPAddress)
	default:
		return AdministrationView{}, fmt.Errorf("unsupported server control action %q", action)
	}
	dialogs, err := service.dialogPort()
	if err != nil {
		return AdministrationView{}, err
	}
	confirmed, err := dialogs.Confirm(title, message)
	if err != nil || !confirmed {
		return AdministrationView{Inventory: service.administration.Inventory(), Message: "Server operation cancelled."}, err
	}
	service.administration.InvalidateInventory()
	requestContext, requestCancel := context.WithTimeout(ctx, 60*time.Second)
	client, session, _, err := service.authenticatedClient(requestContext)
	if err == nil {
		if action == "restart" {
			err = client.RestartFieryProcess(requestContext, session)
		} else {
			err = client.RebootServer(requestContext, session)
		}
	}
	requestCancel()
	if err != nil {
		return AdministrationView{}, redactError(err, server.SecretKey, server.Password)
	}
	service.mu.Lock()
	service.client, service.session, service.clientKey = nil, fiery.Session{}, ""
	service.mu.Unlock()
	recoveryTimeout := 8 * time.Minute
	if action == "reboot" {
		recoveryTimeout = 12 * time.Minute
	}
	recoveryContext, recoveryCancel := context.WithTimeout(ctx, recoveryTimeout)
	err = core.WaitForRecovery(recoveryContext, recoveryProbe{server: server}, core.DefaultRecoveryPolicy())
	recoveryCancel()
	if err != nil {
		safeErr := redactError(err, server.SecretKey, server.Password)
		service.diagnostic.Printf("SERVER_ADMIN action=%s server=%s result=WARNING recovery_error=%v", action, server.IPAddress, safeErr)
		return AdministrationView{Message: title + " was accepted, but API recovery was not confirmed.", Accepted: true}, safeErr
	}
	service.diagnostic.Printf("SERVER_ADMIN action=%s server=%s result=PASS recovery=confirmed", action, server.IPAddress)
	return AdministrationView{Message: fmt.Sprintf("%s completed and %s is API-ready.", title, server.IPAddress), Accepted: true}, nil
}

func (service *Service) ClearAllJobs(ctx context.Context, confirmation string) (AdministrationView, error) {
	service.operationMu.Lock()
	defer service.operationMu.Unlock()
	if err := service.administrationPrecondition(); err != nil {
		return AdministrationView{}, err
	}
	server, ok := service.connection.Active()
	if !ok {
		return AdministrationView{}, errors.New("test and apply a server connection first")
	}
	inventory, err := service.administration.ValidateClear(server.IPAddress, confirmation, time.Now(), core.DefaultInventoryValidity)
	if err != nil {
		return AdministrationView{Inventory: service.administration.Inventory()}, err
	}
	dialogs, err := service.dialogPort()
	if err != nil {
		return AdministrationView{}, err
	}
	confirmed, err := dialogs.Confirm("Permanently clear all Fiery jobs", fmt.Sprintf("Permanently remove all %d job(s) currently inspected on %s?\n\nThis affects every user and cannot be undone. Accounting and configuration will not be cleared.", inventory.Count, server.IPAddress))
	if err != nil || !confirmed {
		return AdministrationView{Inventory: inventory, Message: "Clear all jobs cancelled."}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Minute)
	defer cancel()
	client, session, _, err := service.authenticatedClient(ctx)
	if err != nil {
		return AdministrationView{}, redactError(err, server.SecretKey, server.Password)
	}
	outcome, err := core.RevalidateAndClearJobs(ctx, jobAdministrationClient{client: client, session: session}, inventory.Count, 2*time.Second)
	if outcome.UpdateInventory {
		service.administration.RecordInventory(server.IPAddress, outcome.Remaining, time.Now())
	}
	view := AdministrationView{Inventory: service.administration.Inventory(), Accepted: outcome.Accepted}
	if err != nil {
		if outcome.Accepted {
			view.Message = "Clear all jobs was accepted, but an empty inventory was not verified."
		}
		return view, redactError(err, server.SecretKey, server.Password)
	}
	view.Message = fmt.Sprintf("All jobs were cleared from %s and an empty inventory was verified.", server.IPAddress)
	service.diagnostic.Printf("SERVER_ADMIN action=clear_jobs server=%s result=PASS remaining_jobs=0", server.IPAddress)
	return view, nil
}

func (service *Service) ManageJob(ctx context.Context, jobID, action string) (JobActionResult, error) {
	service.operationMu.Lock()
	defer service.operationMu.Unlock()
	if err := service.administrationPrecondition(); err != nil {
		return JobActionResult{}, err
	}
	jobID = strings.TrimSpace(jobID)
	if jobID == "" {
		return JobActionResult{}, errors.New("enter the exact Fiery job ID")
	}
	if action != "cancel" && action != "delete" {
		return JobActionResult{}, fmt.Errorf("unsupported job action %q", action)
	}
	dialogs, err := service.dialogPort()
	if err != nil {
		return JobActionResult{}, err
	}
	verb := "Cancel"
	message := fmt.Sprintf("Cancel job %s?\n\nCancellation proceeds only if Fiery reports processing/ripping, waiting to print, or printing.", jobID)
	if action == "delete" {
		verb = "Permanently delete"
		message = fmt.Sprintf("Permanently delete job %s?\n\nDelete is allowed in any job state and cannot be undone.", jobID)
	}
	confirmed, err := dialogs.Confirm(verb+" Fiery job", message)
	if err != nil || !confirmed {
		return JobActionResult{JobID: jobID, Action: action, Message: "Job action cancelled."}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	client, session, server, err := service.authenticatedClient(ctx)
	if err != nil {
		return JobActionResult{}, redactError(err, server.SecretKey, server.Password)
	}
	if action == "cancel" {
		attributes, getErr := client.GetJobAttributes(ctx, session, jobID)
		if getErr != nil {
			return JobActionResult{}, redactError(fmt.Errorf("check job state: %w", getErr), server.SecretKey, server.Password)
		}
		cancelable, state := core.CancelableJob(attributes)
		if !cancelable {
			return JobActionResult{}, fmt.Errorf("job is not processing/ripping, waiting to print, or printing (reported state: %s)", state)
		}
		err = client.CancelJob(ctx, session, jobID)
	} else {
		err = client.DeleteJob(ctx, session, jobID)
	}
	if err != nil {
		return JobActionResult{}, redactError(err, server.SecretKey, server.Password)
	}
	service.diagnostic.Printf("JOB_ACTION action=%s job_id=%s result=PASS", action, jobID)
	return JobActionResult{JobID: jobID, Action: action, Message: fmt.Sprintf("Job %s %s request completed successfully.", jobID, action)}, nil
}

func (service *Service) administrationPrecondition() error {
	return core.AdministrationPrecondition(core.AdministrationActivity{AutomationRunning: service.automationRunning()})
}

type recoveryProbe struct{ server model.ServerConnection }

func (probe recoveryProbe) Probe(ctx context.Context) (string, error) {
	attempt, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	client, err := fiery.New(fiery.Config{ServerIP: probe.server.IPAddress, SecretKey: probe.server.SecretKey, Password: probe.server.Password, InsecureTLS: true})
	if err != nil {
		return "", err
	}
	session, err := client.Login(attempt)
	if err != nil {
		return "", err
	}
	return client.ServerStatus(attempt, session)
}

type jobAdministrationClient struct {
	client  *fiery.Client
	session fiery.Session
}

func (client jobAdministrationClient) JobCount(ctx context.Context) (int, error) {
	jobs, err := client.client.ListJobs(ctx, client.session)
	return len(jobs), err
}
func (client jobAdministrationClient) ClearAllJobs(ctx context.Context) error {
	return client.client.ClearAllJobs(ctx, client.session)
}

var _ core.RecoveryProbe = recoveryProbe{}
var _ core.JobAdministrationClient = jobAdministrationClient{}
