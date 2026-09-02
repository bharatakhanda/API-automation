package application

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"sync"

	"api-automation/internal/model"
)

type ConnectionSnapshot struct {
	HasActive          bool   `json:"hasActive"`
	ActiveIPAddress    string `json:"activeIpAddress,omitempty"`
	TestOK             bool   `json:"testOk"`
	TestStatus         string `json:"testStatus"`
	SecretConfigured   bool   `json:"secretConfigured"`
	PasswordConfigured bool   `json:"passwordConfigured"`
}

type ConnectionState struct {
	mu               sync.Mutex
	configuredSecret string
	active           model.ServerConnection
	hasActive        bool
	testedKey        string
	testOK           bool
	testStatus       string
}

func NewConnectionState(configuredSecret string) *ConnectionState {
	return &ConnectionState{configuredSecret: strings.TrimSpace(configuredSecret), testStatus: "Not tested"}
}

// NewConnectionStateWithActive is primarily useful to adapt an already active
// legacy frontend state without exposing credentials in snapshots.
func NewConnectionStateWithActive(configuredSecret string, active model.ServerConnection) *ConnectionState {
	state := NewConnectionState(configuredSecret)
	if strings.TrimSpace(active.IPAddress) != "" {
		state.active = normalizeConnection(active)
		state.hasActive = true
		state.testStatus = "Active connection · test replacements before applying"
	}
	return state
}

func ConnectionKey(server model.ServerConnection) string {
	server = normalizeConnection(server)
	digest := sha256.Sum256([]byte(server.IPAddress + "\x00" + server.SecretKey + "\x00" + server.Password))
	return hex.EncodeToString(digest[:])
}

func (state *ConnectionState) ResolveDraft(draft model.ServerConnection) model.ServerConnection {
	state.mu.Lock()
	defer state.mu.Unlock()
	draft = normalizeConnection(draft)
	if draft.SecretKey == "" {
		if state.hasActive {
			draft.SecretKey = state.active.SecretKey
		} else {
			draft.SecretKey = state.configuredSecret
		}
	}
	if draft.Password == "" && state.hasActive {
		draft.Password = state.active.Password
	}
	return draft
}

func ValidateConnectionDraft(draft model.ServerConnection) error {
	draft = normalizeConnection(draft)
	if draft.IPAddress == "" || draft.SecretKey == "" || draft.Password == "" {
		return errors.New("server address, configured or replacement secret key, and administrator password are required")
	}
	return nil
}

func (state *ConnectionState) InvalidateIfChanged(draft model.ServerConnection) bool {
	draft = state.ResolveDraft(draft)
	key := ConnectionKey(draft)
	state.mu.Lock()
	defer state.mu.Unlock()
	if state.testOK && key != state.testedKey {
		state.testOK = false
		state.testedKey = ""
		state.testStatus = "Details changed · test again"
		return true
	}
	return false
}

func (state *ConnectionState) BeginTest() {
	state.mu.Lock()
	state.testOK = false
	state.testedKey = ""
	state.testStatus = "Testing..."
	state.mu.Unlock()
}

func (state *ConnectionState) CompleteTest(draft model.ServerConnection, success bool, status string) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.testOK = success
	state.testedKey = ""
	if success {
		state.testedKey = ConnectionKey(draft)
	}
	state.testStatus = status
}

func (state *ConnectionState) SetTestStatus(status string) {
	state.mu.Lock()
	state.testStatus = status
	state.mu.Unlock()
}

func (state *ConnectionState) Apply(draft model.ServerConnection) (model.ServerConnection, bool, error) {
	draft = state.ResolveDraft(draft)
	if err := ValidateConnectionDraft(draft); err != nil {
		return model.ServerConnection{}, false, err
	}
	key := ConnectionKey(draft)
	state.mu.Lock()
	defer state.mu.Unlock()
	if !state.testOK || state.testedKey != key {
		return model.ServerConnection{}, false, errors.New("test these exact connection details successfully before pressing OK")
	}
	changed := !state.hasActive || ConnectionKey(state.active) != key
	state.active = draft
	state.hasActive = true
	state.testOK = false
	state.testedKey = ""
	state.testStatus = "Active connection · test replacements before applying"
	return draft, changed, nil
}

func (state *ConnectionState) BeginChange() (model.ServerConnection, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	state.testOK = false
	state.testedKey = ""
	state.testStatus = "Not tested"
	return state.active, state.hasActive
}

func (state *ConnectionState) CancelChange() (model.ServerConnection, bool) {
	return state.BeginChange()
}

func (state *ConnectionState) Active() (model.ServerConnection, bool) {
	state.mu.Lock()
	defer state.mu.Unlock()
	return state.active, state.hasActive
}

func (state *ConnectionState) Snapshot() ConnectionSnapshot {
	state.mu.Lock()
	defer state.mu.Unlock()
	return ConnectionSnapshot{
		HasActive: state.hasActive, ActiveIPAddress: state.active.IPAddress,
		TestOK: state.testOK, TestStatus: state.testStatus,
		SecretConfigured:   state.configuredSecret != "" || (state.hasActive && state.active.SecretKey != ""),
		PasswordConfigured: state.hasActive && state.active.Password != "",
	}
}

func normalizeConnection(server model.ServerConnection) model.ServerConnection {
	server.IPAddress = strings.TrimSpace(server.IPAddress)
	server.SecretKey = strings.TrimSpace(server.SecretKey)
	server.Password = strings.TrimSpace(server.Password)
	return server
}
