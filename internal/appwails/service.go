package appwails

import (
	"context"
	"errors"
	"sort"
	"strings"
	"sync"
	"time"

	core "api-automation/internal/application"
	"api-automation/internal/capabilities"
	"api-automation/internal/fiery"
	"api-automation/internal/model"
)

const previewVersion = "Wails 3 beta preview"

type ConnectionDraft struct {
	IPAddress string `json:"ipAddress"`
	SecretKey string `json:"secretKey"`
	Password  string `json:"password"`
}

type PreviewState struct {
	Version      string                  `json:"version"`
	Connection   core.ConnectionSnapshot `json:"connection"`
	Capabilities *CapabilityView         `json:"capabilities,omitempty"`
}

type ConnectionResult struct {
	Connection core.ConnectionSnapshot `json:"connection"`
	Message    string                  `json:"message"`
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
	CapturedAt    time.Time          `json:"capturedAt"`
	ServerName    string             `json:"serverName,omitempty"`
	PressModel    string             `json:"pressModel,omitempty"`
	SerialNumber  string             `json:"serialNumber,omitempty"`
	Version       string             `json:"version,omitempty"`
	OptionCount   int                `json:"optionCount"`
	ExcludedCount int                `json:"excludedCount"`
	Presets       []ServerPresetView `json:"presets,omitempty"`
	Options       []CapabilityOption `json:"options"`
}

type ServerPresetView struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type CapabilityOption struct {
	ID        string   `json:"id"`
	Label     string   `json:"label"`
	Group     string   `json:"group"`
	Value     string   `json:"value,omitempty"`
	Values    []string `json:"values,omitempty"`
	Numeric   bool     `json:"numeric"`
	Synthetic bool     `json:"synthetic"`
	Min       *float64 `json:"min,omitempty"`
	Max       *float64 `json:"max,omitempty"`
	Increment *float64 `json:"increment,omitempty"`
	Precision int      `json:"precision,omitempty"`
}

type Service struct {
	connection *core.ConnectionState

	operationMu sync.Mutex
	mu          sync.RWMutex
	capability  *CapabilityView
	client      *fiery.Client
	session     fiery.Session
	clientKey   string
}

func NewService(defaultSecret string) *Service {
	return &Service{connection: core.NewConnectionState(defaultSecret)}
}

func (service *Service) State() PreviewState {
	service.mu.RLock()
	view := cloneCapabilityView(service.capability)
	service.mu.RUnlock()
	return PreviewState{Version: previewVersion, Connection: service.connection.Snapshot(), Capabilities: view}
}

func (service *Service) TestConnection(ctx context.Context, input ConnectionDraft) (ConnectionResult, error) {
	service.operationMu.Lock()
	defer service.operationMu.Unlock()

	draft := service.resolve(input)
	service.connection.BeginTest()
	client, err := fiery.New(fiery.Config{ServerIP: draft.IPAddress, SecretKey: draft.SecretKey, Password: draft.Password, InsecureTLS: true})
	if err != nil {
		service.connection.CompleteTest(draft, false, "Connection failed")
		return service.connectionResult("Connection failed"), redactError(err, draft.SecretKey, draft.Password)
	}
	session, err := client.Login(ctx)
	if err != nil {
		service.connection.CompleteTest(draft, false, "Authentication failed")
		return service.connectionResult("Authentication failed"), redactError(err, draft.SecretKey, draft.Password)
	}
	service.connection.CompleteTest(draft, true, "Connection OK · apply to unlock preview")
	service.mu.Lock()
	service.client = client
	service.session = session
	service.clientKey = core.ConnectionKey(draft)
	service.mu.Unlock()
	return service.connectionResult("Connection test passed"), nil
}

func (service *Service) ApplyConnection(input ConnectionDraft) (ConnectionResult, error) {
	service.operationMu.Lock()
	defer service.operationMu.Unlock()

	draft := service.resolve(input)
	_, changed, err := service.connection.Apply(draft)
	if err != nil {
		return service.connectionResult("Connection was not applied"), err
	}
	if changed {
		service.mu.Lock()
		service.capability = nil
		if service.clientKey != core.ConnectionKey(draft) {
			service.client = nil
			service.session = fiery.Session{}
			service.clientKey = ""
		}
		service.mu.Unlock()
	}
	return service.connectionResult("Connection applied"), nil
}

func (service *Service) StartConnectionChange() ConnectionResult {
	service.connection.BeginChange()
	return service.connectionResult("Editing a staged connection")
}

func (service *Service) CancelConnectionChange() ConnectionResult {
	_, ok := service.connection.CancelChange()
	if !ok {
		return service.connectionResult("No active connection to restore")
	}
	return service.connectionResult("Connection change cancelled")
}

func (service *Service) RefreshOverview(ctx context.Context) (Overview, error) {
	started := time.Now()
	client, session, server, err := service.authenticatedClient(ctx)
	if err != nil {
		return Overview{}, redactError(err, server.SecretKey, server.Password)
	}
	status, err := client.ServerStatus(ctx, session)
	if err != nil {
		return Overview{}, redactError(err, server.SecretKey, server.Password)
	}
	service.mu.RLock()
	view := service.capability
	service.mu.RUnlock()
	overview := Overview{ServerAddress: server.IPAddress, Status: strings.TrimSpace(status), CheckedAt: time.Now().UTC(), LatencyMS: time.Since(started).Milliseconds()}
	if overview.Status == "" {
		overview.Status = "unknown"
	}
	overview.Detail = "Fiery status read through the shared authenticated Go client."
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

	client, session, server, err := service.authenticatedClient(ctx)
	if err != nil {
		return CapabilityView{}, redactError(err, server.SecretKey, server.Password)
	}
	snapshot := client.DiscoverCapabilities(ctx, session)
	view := capabilityView(snapshot.CapturedAt, capabilities.FromSnapshot(snapshot))
	service.mu.Lock()
	service.capability = &view
	service.mu.Unlock()
	return *cloneCapabilityView(&view), nil
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
		item := CapabilityOption{ID: option.ID, Label: option.Label, Group: option.Group, Value: option.Value, Values: append([]string(nil), option.Values...), Numeric: option.Numeric, Synthetic: option.Synthetic}
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

func cloneCapabilityView(source *CapabilityView) *CapabilityView {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Presets = append([]ServerPresetView(nil), source.Presets...)
	clone.Options = make([]CapabilityOption, len(source.Options))
	for index, option := range source.Options {
		clone.Options[index] = option
		clone.Options[index].Values = append([]string(nil), option.Values...)
	}
	return &clone
}
