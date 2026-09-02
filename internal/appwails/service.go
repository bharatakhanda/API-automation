package appwails

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	core "api-automation/internal/application"
	"api-automation/internal/capabilities"
	"api-automation/internal/fiery"
	"api-automation/internal/model"
	"api-automation/internal/preflight"
	"api-automation/internal/presets"
)

const applicationVersion = "Wails 3"

type ConnectionDraft struct {
	IPAddress string `json:"ipAddress"`
	SecretKey string `json:"secretKey"`
	Password  string `json:"password"`
}

type ApplicationState struct {
	Version        string                  `json:"version"`
	Connection     core.ConnectionSnapshot `json:"connection"`
	Capabilities   *CapabilityView         `json:"capabilities,omitempty"`
	DiagnosticPath string                  `json:"diagnosticPath,omitempty"`
}

type ConnectionResult struct {
	Connection core.ConnectionSnapshot `json:"connection"`
	Message    string                  `json:"message"`
	Changed    bool                    `json:"changed"`
}

type Overview struct {
	ServerAddress string    `json:"serverAddress"`
	ServerName    string    `json:"serverName,omitempty"`
	PressModel    string    `json:"pressModel,omitempty"`
	Status        string    `json:"status"`
	Detail        string    `json:"detail"`
	CheckedAt     time.Time `json:"checkedAt"`
	LatencyMS     int64     `json:"latencyMs"`
	OptionCount   int       `json:"optionCount"`
}

type CapabilityView struct {
	CapturedAt      time.Time          `json:"capturedAt"`
	ServerName      string             `json:"serverName,omitempty"`
	PressModel      string             `json:"pressModel,omitempty"`
	SerialNumber    string             `json:"serialNumber,omitempty"`
	Version         string             `json:"version,omitempty"`
	OptionCount     int                `json:"optionCount"`
	ExcludedCount   int                `json:"excludedCount"`
	Presets         []ServerPresetView `json:"presets,omitempty"`
	Options         []CapabilityOption `json:"options"`
	PreflightStatus string             `json:"preflightStatus,omitempty"`
	CapturePaths    []string           `json:"capturePaths,omitempty"`
	CaptureWarnings []string           `json:"captureWarnings,omitempty"`
}

type ServerPresetView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CapabilityOption struct {
	ID              string   `json:"id"`
	Label           string   `json:"label"`
	Group           string   `json:"group"`
	Value           string   `json:"value,omitempty"`
	Values          []string `json:"values,omitempty"`
	Numeric         bool     `json:"numeric"`
	Synthetic       bool     `json:"synthetic"`
	Min             *float64 `json:"min,omitempty"`
	Max             *float64 `json:"max,omitempty"`
	Increment       *float64 `json:"increment,omitempty"`
	Precision       int      `json:"precision,omitempty"`
	ConstraintCount int      `json:"constraintCount"`
}

type Options struct {
	Dialogs           DialogPort
	EventEmitter      func(string, any)
	DataDirectory     string
	DebugDirectory    string
	DisableDiagnostic bool
}

type Service struct {
	connection  *core.ConnectionState
	rootContext context.Context
	rootCancel  context.CancelFunc

	operationMu  sync.Mutex
	mu           sync.RWMutex
	capability   *CapabilityView
	capabilities *capabilities.Model
	client       *fiery.Client
	session      fiery.Session
	clientKey    string
	dialogs      DialogPort
	presets      *presets.Store
	jobWorkload  fiery.JobWorkloadSummary

	runMu            sync.Mutex
	run              *activeRun
	eventEmitter     func(string, any)
	administration   *core.AdministrationState
	diagnostic       *diagnosticLog
	dataDirectory    string
	captureDirectory string
}

func NewService(defaultSecret string, options Options) *Service {
	ctx, cancel := context.WithCancel(context.Background())
	dataDirectory := strings.TrimSpace(options.DataDirectory)
	if dataDirectory == "" {
		dataDirectory, _ = applicationDataDirectory()
	}
	debugDirectory := strings.TrimSpace(options.DebugDirectory)
	if debugDirectory == "" {
		debugDirectory = dataDirectory
	}
	diagnostic := &diagnosticLog{}
	captureDirectory := ""
	if debugDirectory != "" {
		captureDirectory = filepath.Join(debugDirectory, "captures")
	}
	if !options.DisableDiagnostic {
		diagnostic = newDiagnosticLog(debugDirectory)
	}
	service := &Service{
		connection: core.NewConnectionState(defaultSecret), rootContext: ctx, rootCancel: cancel,
		administration: new(core.AdministrationState), dialogs: options.Dialogs, eventEmitter: options.EventEmitter,
		diagnostic: diagnostic, dataDirectory: dataDirectory, captureDirectory: captureDirectory,
	}
	service.diagnostic.Printf("APPLICATION_START frontend=wails version=%q", applicationVersion)
	return service
}

func Shutdown(service *Service) {
	if service == nil {
		return
	}
	service.rootCancel()
	service.runMu.Lock()
	var finished <-chan struct{}
	if service.run != nil && service.run.operation != nil {
		service.run.operation.Cancel()
		finished = service.run.finished
	}
	service.runMu.Unlock()
	if finished != nil {
		select {
		case <-finished:
		case <-time.After(5 * time.Second):
		}
	}
	service.diagnostic.Close()
}

func (service *Service) State() ApplicationState {
	service.mu.RLock()
	view := cloneCapabilityView(service.capability)
	service.mu.RUnlock()
	return ApplicationState{Version: applicationVersion, Connection: service.connection.Snapshot(), Capabilities: view, DiagnosticPath: service.diagnostic.Path()}
}

func (service *Service) TestConnection(ctx context.Context, input ConnectionDraft) (ConnectionResult, error) {
	service.operationMu.Lock()
	defer service.operationMu.Unlock()
	if service.automationRunning() {
		return service.connectionResult("Connection test blocked"), errors.New("connection testing is blocked while automation is running")
	}

	draft := service.resolve(input)
	service.connection.BeginTest()
	client, err := fiery.New(fiery.Config{ServerIP: draft.IPAddress, SecretKey: draft.SecretKey, Password: draft.Password, InsecureTLS: true})
	if err != nil {
		service.connection.CompleteTest(draft, false, "Connection failed")
		safeErr := redactError(err, draft.SecretKey, draft.Password)
		service.diagnostic.Printf("CONNECTION_TEST server=%s result=ERROR error=%v", draft.IPAddress, safeErr)
		return service.connectionResult("Connection failed"), safeErr
	}
	session, err := client.Login(ctx)
	if err != nil {
		service.connection.CompleteTest(draft, false, "Authentication failed")
		safeErr := redactError(err, draft.SecretKey, draft.Password)
		service.diagnostic.Printf("CONNECTION_TEST server=%s result=ERROR error=%v", draft.IPAddress, safeErr)
		return service.connectionResult("Authentication failed"), safeErr
	}
	service.connection.CompleteTest(draft, true, "Connection OK · apply to unlock workspace")
	service.mu.Lock()
	service.client = client
	service.session = session
	service.clientKey = core.ConnectionKey(draft)
	service.mu.Unlock()
	service.diagnostic.Printf("CONNECTION_TEST server=%s result=PASS", draft.IPAddress)
	return service.connectionResult("Connection test passed"), nil
}

func (service *Service) ApplyConnection(input ConnectionDraft) (ConnectionResult, error) {
	service.operationMu.Lock()
	defer service.operationMu.Unlock()
	if service.automationRunning() {
		return service.connectionResult("Connection change blocked"), errors.New("connection changes are blocked while automation is running")
	}

	draft := service.resolve(input)
	_, changed, err := service.connection.Apply(draft)
	if err != nil {
		return service.connectionResult("Connection was not applied"), err
	}
	if changed {
		service.administration.InvalidateInventory()
		service.mu.Lock()
		service.capability = nil
		service.capabilities = nil
		service.jobWorkload = fiery.JobWorkloadSummary{}
		if service.clientKey != core.ConnectionKey(draft) {
			service.client = nil
			service.session = fiery.Session{}
			service.clientKey = ""
		}
		service.mu.Unlock()
	}
	result := service.connectionResult("Connection applied")
	result.Changed = changed
	service.diagnostic.Printf("CONNECTION_APPLY server=%s changed=%t", draft.IPAddress, changed)
	return result, nil
}

func (service *Service) StartConnectionChange() (ConnectionResult, error) {
	if service.automationRunning() {
		return service.connectionResult("Connection change blocked"), errors.New("connection changes are blocked while automation is running")
	}
	service.connection.BeginChange()
	return service.connectionResult("Editing a staged connection"), nil
}

func (service *Service) CancelConnectionChange() (ConnectionResult, error) {
	if service.automationRunning() {
		return service.connectionResult("Connection change blocked"), errors.New("connection changes are blocked while automation is running")
	}
	_, ok := service.connection.CancelChange()
	if !ok {
		return service.connectionResult("No active connection to restore"), nil
	}
	return service.connectionResult("Connection change cancelled"), nil
}

func (service *Service) RefreshOverview(ctx context.Context) (Overview, error) {
	started := time.Now()
	client, session, server, err := service.authenticatedClient(ctx)
	if err != nil {
		return Overview{}, redactError(err, server.SecretKey, server.Password)
	}
	activity, err := client.ServerActivityStatus(ctx, session)
	if err != nil {
		return Overview{}, redactError(err, server.SecretKey, server.Password)
	}
	service.mu.RLock()
	view := service.capability
	workload := service.jobWorkload
	service.mu.RUnlock()
	probed, workloadErr := client.ProbeRecentJobWorkload(ctx, session, workload.TotalItems, core.DefaultOverviewMonitorPolicy().JobProbeLimit)
	if workloadErr == nil {
		workload = probed
		service.mu.Lock()
		service.jobWorkload = workload
		service.mu.Unlock()
	}
	detail := fmt.Sprintf("health=%s · extended=%s · active jobs=%d", activity.Health, activity.Extended, workload.ActiveJobs)
	if workloadErr != nil {
		detail += " · bounded job probe unavailable"
	}
	status, detail := core.EffectiveOverviewServerStateWithJobs(activity.Workload, detail, workload)
	status, detail = core.EffectiveOverviewServerState(status, detail, service.automationRunning())
	overview := Overview{ServerAddress: server.IPAddress, Status: status, Detail: detail, CheckedAt: time.Now().UTC(), LatencyMS: time.Since(started).Milliseconds()}
	if view != nil {
		overview.ServerName = view.ServerName
		overview.PressModel = view.PressModel
		overview.OptionCount = view.OptionCount
	}
	return overview, nil
}

func (service *Service) DiscoverCapabilities(ctx context.Context) (CapabilityView, error) {
	service.operationMu.Lock()
	defer service.operationMu.Unlock()
	if service.automationRunning() {
		return CapabilityView{}, errors.New("capability discovery is blocked while automation is running")
	}

	client, session, server, err := service.authenticatedClient(ctx)
	if err != nil {
		return CapabilityView{}, redactError(err, server.SecretKey, server.Password)
	}
	snapshot := client.DiscoverCapabilities(ctx, session)
	capabilityModel := capabilities.FromSnapshot(snapshot)
	view := capabilityView(snapshot.CapturedAt, capabilityModel)
	environment := preflight.Run(snapshot, capabilityModel)
	view.PreflightStatus = environment.OverallStatus
	view.CapturePaths, view.CaptureWarnings = service.saveCapabilityEvidence(client, snapshot, capabilityModel, environment)
	service.mu.Lock()
	service.capability = &view
	service.capabilities = &capabilityModel
	service.jobWorkload = fiery.JobWorkloadSummary{TotalItems: capabilityModel.JobsTotal, ActiveJobs: capabilityModel.ActiveJobs}
	service.mu.Unlock()
	service.diagnostic.Printf("CAPABILITY_DISCOVERY server=%s applicable=%d excluded=%d", server.IPAddress, view.OptionCount, view.ExcludedCount)
	return *cloneCapabilityView(&view), nil
}

func (service *Service) saveCapabilityEvidence(client *fiery.Client, snapshot fiery.CapabilitySnapshot, model capabilities.Model, environment preflight.EnvironmentSnapshot) (paths, warnings []string) {
	if service.captureDirectory == "" {
		return nil, []string{"application capture directory is unavailable"}
	}
	dir := service.captureDirectory
	for label, save := range map[string]func() (string, error){
		"capability snapshot":  func() (string, error) { return client.SaveCapabilitySnapshot(snapshot, dir) },
		"environment snapshot": func() (string, error) { return preflight.Save(environment, dir) },
		"normalization report": func() (string, error) { return capabilities.SaveNormalizationReport(model, snapshot.CapturedAt, dir) },
	} {
		path, err := save()
		if err != nil {
			warnings = append(warnings, label+": "+err.Error())
			continue
		}
		paths = append(paths, path)
	}
	sort.Strings(paths)
	sort.Strings(warnings)
	return paths, warnings
}

func (service *Service) resolve(input ConnectionDraft) model.ServerConnection {
	return service.connection.ResolveDraft(model.ServerConnection{IPAddress: strings.TrimSpace(input.IPAddress), SecretKey: strings.TrimSpace(input.SecretKey), Password: strings.TrimSpace(input.Password)})
}

func (service *Service) connectionResult(message string) ConnectionResult {
	return ConnectionResult{Connection: service.connection.Snapshot(), Message: message}
}

func (service *Service) authenticatedClient(ctx context.Context) (*fiery.Client, fiery.Session, model.ServerConnection, error) {
	server, ok := service.connection.Active()
	if !ok {
		return nil, fiery.Session{}, model.ServerConnection{}, errors.New("test and apply a server connection first")
	}
	key := core.ConnectionKey(server)
	service.mu.RLock()
	client, session, clientKey := service.client, service.session, service.clientKey
	service.mu.RUnlock()
	if client != nil && clientKey == key {
		return client, session, server, nil
	}
	client, err := fiery.New(fiery.Config{ServerIP: server.IPAddress, SecretKey: server.SecretKey, Password: server.Password, InsecureTLS: true})
	if err != nil {
		return nil, fiery.Session{}, server, err
	}
	session, err = client.Login(ctx)
	if err != nil {
		return nil, fiery.Session{}, server, err
	}
	service.mu.Lock()
	service.client, service.session, service.clientKey = client, session, key
	service.mu.Unlock()
	return client, session, server, nil
}

func capabilityView(capturedAt time.Time, source capabilities.Model) CapabilityView {
	view := CapabilityView{
		CapturedAt: capturedAt, ServerName: source.ServerName, PressModel: source.PressModel, SerialNumber: source.SerialNumber,
		Version: source.Version, OptionCount: len(source.Options), ExcludedCount: len(source.ExcludedOptions),
		Options: make([]CapabilityOption, 0, len(source.Options)), Presets: make([]ServerPresetView, 0, len(source.ServerPresets)),
	}
	for _, preset := range source.ServerPresets {
		view.Presets = append(view.Presets, ServerPresetView{ID: preset.ID, Name: preset.Name})
	}
	for _, option := range source.Options {
		item := CapabilityOption{ID: option.ID, Label: option.Label, Group: option.Group, Value: option.Value, Values: append([]string(nil), option.Values...), Numeric: option.Numeric, Synthetic: option.Synthetic, ConstraintCount: optionConstraintCount(option)}
		if option.Range != nil {
			minimum, maximum, increment := option.Range.Min, option.Range.Max, option.Range.Increment
			item.Min, item.Max, item.Increment, item.Precision = &minimum, &maximum, &increment, option.Range.Precision
		}
		view.Options = append(view.Options, item)
	}
	sort.SliceStable(view.Options, func(i, j int) bool {
		if view.Options[i].Group != view.Options[j].Group {
			return view.Options[i].Group < view.Options[j].Group
		}
		return view.Options[i].Label < view.Options[j].Label
	})
	return view
}

func redactError(err error, values ...string) error {
	if err == nil {
		return nil
	}
	message := err.Error()
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			message = strings.ReplaceAll(message, value, "[redacted]")
		}
	}
	return errors.New(message)
}

func optionConstraintCount(option capabilities.Option) int {
	count := 0
	for _, dependencies := range option.Constraints {
		for _, values := range dependencies {
			count += len(values)
		}
	}
	return count
}

func cloneCapabilityView(source *CapabilityView) *CapabilityView {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Presets = append([]ServerPresetView(nil), source.Presets...)
	clone.CapturePaths = append([]string(nil), source.CapturePaths...)
	clone.CaptureWarnings = append([]string(nil), source.CaptureWarnings...)
	clone.Options = make([]CapabilityOption, len(source.Options))
	for index, option := range source.Options {
		clone.Options[index] = option
		clone.Options[index].Values = append([]string(nil), option.Values...)
	}
	return &clone
}
