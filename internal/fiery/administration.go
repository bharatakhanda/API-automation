package fiery

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
)

// ServerPreset is a read-only representation of a preset advertised by Fiery.
// Applying a preset to a job does not modify the preset itself.
type ServerPreset struct {
	ID         string            `json:"id"`
	Name       string            `json:"name"`
	Attributes map[string]string `json:"attributes,omitempty"`
}

// ServerJob is the minimal non-sensitive job inventory used by guarded server
// administration. It intentionally omits job content and ticket attributes.
type ServerJob struct {
	ID       string `json:"id"`
	Name     string `json:"name,omitempty"`
	Username string `json:"username,omitempty"`
	Status   string `json:"status,omitempty"`
	State    string `json:"state,omitempty"`
}

func (c *Client) ListServerPresets(ctx context.Context, session Session) ([]ServerPreset, error) {
	body, err := c.v5JSONRequest(ctx, session, http.MethodGet, "/presets", nil, nil)
	if err != nil {
		return nil, err
	}
	return ParseServerPresets(body), nil
}

// ParseServerPresets normalizes the documented data.items response while also
// accepting a single data.item response used by some Fiery releases.
func ParseServerPresets(body []byte) []ServerPreset {
	var payload struct {
		Data struct {
			Items []map[string]any `json:"items"`
			Item  map[string]any   `json:"item"`
		} `json:"data"`
		Items []map[string]any `json:"items"`
	}
	if json.Unmarshal(body, &payload) != nil {
		return nil
	}
	items := payload.Data.Items
	if len(items) == 0 {
		items = payload.Items
	}
	if len(items) == 0 && len(payload.Data.Item) > 0 {
		items = []map[string]any{payload.Data.Item}
	}
	presets := make([]ServerPreset, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		id := firstMapScalar(item, "id", "presetid", "preset id")
		if id == "" {
			continue
		}
		if _, duplicate := seen[id]; duplicate {
			continue
		}
		seen[id] = struct{}{}
		name := firstMapScalar(item, "name", "display_name", "display name", "title")
		if name == "" {
			name = id
		}
		preset := ServerPreset{ID: id, Name: name}
		if raw, ok := lookupMapValue(item, "attributes"); ok {
			preset.Attributes = scalarStringMap(raw)
		}
		presets = append(presets, preset)
	}
	sort.Slice(presets, func(i, j int) bool {
		if strings.EqualFold(presets[i].Name, presets[j].Name) {
			return presets[i].ID < presets[j].ID
		}
		return strings.ToLower(presets[i].Name) < strings.ToLower(presets[j].Name)
	})
	return presets
}

// ApplyServerPreset applies an existing server preset to one job. It never
// creates, updates, or deletes the preset. Explicit job attributes can be sent
// afterward to override individual preset values.
func (c *Client) ApplyServerPreset(ctx context.Context, session Session, jobID, presetID string) error {
	jobID = strings.TrimSpace(jobID)
	presetID = strings.TrimSpace(presetID)
	if jobID == "" || presetID == "" {
		return errors.New("job ID and server preset ID are required")
	}
	payload := map[string]string{"preset": presetID}
	query := url.Values{"preset": []string{presetID}}
	body, err := c.v5JSONRequest(ctx, session, http.MethodPut, "/jobs/"+url.PathEscape(jobID), query, payload)
	if err != nil {
		return fmt.Errorf("apply server preset %q to job %s: %w", presetID, jobID, err)
	}
	if responseHasExplicitFalse(body, "preset") {
		return fmt.Errorf("fiery reported that server preset %q was not applied to job %s", presetID, jobID)
	}
	return nil
}

func (c *Client) ListJobs(ctx context.Context, session Session) ([]ServerJob, error) {
	body, err := c.v5JSONRequest(ctx, session, http.MethodGet, "/jobs", nil, nil)
	if err != nil {
		return nil, err
	}
	var payload map[string]json.RawMessage
	if json.Unmarshal(body, &payload) != nil {
		return nil, errors.New("fiery job inventory response was not valid JSON")
	}
	itemsRaw, found := payload["items"]
	reportedTotal := -1
	if dataRaw, hasData := payload["data"]; hasData {
		var data map[string]json.RawMessage
		if json.Unmarshal(dataRaw, &data) != nil {
			return nil, errors.New("fiery job inventory data was not a JSON object")
		}
		if nested, hasItems := data["items"]; hasItems {
			itemsRaw, found = nested, true
		}
		if totalRaw, hasTotal := data["totalItems"]; hasTotal {
			var totalValue any
			if json.Unmarshal(totalRaw, &totalValue) == nil {
				if total, parseErr := strconv.Atoi(cleanScalar(totalValue)); parseErr == nil && total >= 0 {
					reportedTotal = total
				}
			}
		}
	}
	if !found || len(itemsRaw) == 0 || string(itemsRaw) == "null" {
		return nil, errors.New("fiery job inventory response did not contain an items array")
	}
	var items []map[string]any
	if json.Unmarshal(itemsRaw, &items) != nil {
		return nil, errors.New("fiery job inventory items was not a JSON array")
	}
	if reportedTotal >= 0 && reportedTotal != len(items) {
		return nil, fmt.Errorf("fiery job inventory is incomplete: response reports %d job(s) but returned %d item(s)", reportedTotal, len(items))
	}
	jobs := make([]ServerJob, 0, len(items))
	for _, item := range items {
		jobs = append(jobs, ServerJob{
			ID:       firstMapScalar(item, "id"),
			Name:     firstMapScalar(item, "title", "job name", "name"),
			Username: firstMapScalar(item, "username", "user name", "owner"),
			Status:   firstMapScalar(item, "status", "display status"),
			State:    firstMapScalar(item, "state"),
		})
	}
	return jobs, nil
}

// RestartFieryProcess restarts Fiery software without rebooting the operating
// system. Recovery monitoring and re-authentication are handled by the caller.
func (c *Client) RestartFieryProcess(ctx context.Context, session Session) error {
	body, err := c.v5JSONRequest(ctx, session, http.MethodPost, "/server", url.Values{"method": []string{"restart"}}, nil)
	if err != nil {
		return fmt.Errorf("restart Fiery process: %w", err)
	}
	if responseHasExplicitFalse(body, "restart") {
		return errors.New("fiery reported that the process restart was not accepted")
	}
	return nil
}

// RebootServer requests the dedicated v5 reboot endpoint documented for FS150
// and later platforms.
func (c *Client) RebootServer(ctx context.Context, session Session) error {
	body, err := c.v5JSONRequest(ctx, session, http.MethodPost, "/server/reboot", nil, nil)
	if err != nil {
		return fmt.Errorf("reboot Fiery server: %w", err)
	}
	if responseHasExplicitFalse(body, "reboot", "status") {
		return errors.New("fiery reported that the server reboot was not accepted")
	}
	return nil
}

// ClearAllJobs invokes the documented server clear operation with only the jobs
// service selected. It never clears accounting, configuration, or global data.
func (c *Client) ClearAllJobs(ctx context.Context, session Session) error {
	body, err := c.v5JSONRequest(ctx, session, http.MethodPost, "/server", url.Values{
		"method":   []string{"clear"},
		"services": []string{"jobs"},
	}, nil)
	if err != nil {
		return fmt.Errorf("clear all Fiery jobs: %w", err)
	}
	if responseHasExplicitFalse(body, "clear") {
		return errors.New("fiery reported that jobs were not cleared")
	}
	return nil
}

type ServerActivityStatus struct {
	Health   string
	Extended string
	Workload string
}

func (c *Client) ServerStatus(ctx context.Context, session Session) (string, error) {
	activity, err := c.ServerActivityStatus(ctx, session)
	return activity.Health, err
}

// ServerActivityStatus keeps Fiery process health separate from workload. The
// status endpoint commonly reports fiery=running while fieryExtendedStatus=none;
// that means the service is healthy and idle, not that a job is running.
func (c *Client) ServerActivityStatus(ctx context.Context, session Session) (ServerActivityStatus, error) {
	// Capability discovery and the live Fiery API expose workload at /status.
	// /server/status is an administration route and can remain at running/none
	// while a job is actively processing.
	body, err := c.v5JSONRequest(ctx, session, http.MethodGet, "/status", nil, nil)
	if err != nil {
		return ServerActivityStatus{}, err
	}
	var payload any
	if json.Unmarshal(body, &payload) != nil {
		return ServerActivityStatus{}, errors.New("fiery server status response was not valid JSON")
	}
	health := findScalarByKey(payload, "fiery")
	if health == "" {
		health = findScalarByKey(payload, "status")
	}
	if health == "" {
		return ServerActivityStatus{}, errors.New("fiery server status response did not contain a status")
	}
	extended := findScalarByKey(payload, "fieryExtendedStatus")
	return ServerActivityStatus{
		Health: health, Extended: extended, Workload: fieryWorkloadState(health, extended),
	}, nil
}

func fieryWorkloadState(health, extended string) string {
	health = strings.ToLower(strings.TrimSpace(health))
	extended = strings.ToLower(strings.TrimSpace(extended))
	for _, token := range []string{"busy", "print", "rip", "process", "spool", "calibrat", "warming", "restart", "reboot"} {
		if strings.Contains(extended, token) || strings.Contains(health, token) {
			return "Busy"
		}
	}
	switch extended {
	case "", "none", "idle", "ready", "running":
		switch health {
		case "running", "online", "ready", "idle":
			return "Idle"
		}
	}
	return "Unavailable"
}

func (c *Client) v5JSONRequest(ctx context.Context, session Session, method, path string, query url.Values, payload any) ([]byte, error) {
	var reader io.Reader
	if payload != nil {
		encoded, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}
		reader = bytes.NewReader(encoded)
	}
	endpoint := c.baseURL + apiV5 + path
	if len(query) > 0 {
		endpoint += "?" + query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, method, endpoint, reader)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Cookie", session.Cookie)
	req.Header.Set("Accept", "application/json")
	if payload != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
	if readErr != nil {
		return nil, fmt.Errorf("read %s %s response: %w", method, path, readErr)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("%s %s failed with HTTP %d: %s", method, apiV5+path, resp.StatusCode, truncateForError(body, 2048))
	}
	return body, nil
}

func responseHasExplicitFalse(body []byte, keys ...string) bool {
	if len(bytes.TrimSpace(body)) == 0 {
		return false
	}
	var payload any
	if json.Unmarshal(body, &payload) != nil {
		return false
	}
	for _, key := range keys {
		if found, value := findBoolByKey(payload, key); found {
			return !value
		}
	}
	return false
}

func findBoolByKey(value any, want string) (bool, bool) {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if strings.EqualFold(strings.TrimSpace(key), want) {
				if value, ok := nested.(bool); ok {
					return true, value
				}
			}
			if found, value := findBoolByKey(nested, want); found {
				return true, value
			}
		}
	case []any:
		for _, nested := range typed {
			if found, value := findBoolByKey(nested, want); found {
				return true, value
			}
		}
	}
	return false, false
}

func findScalarByKey(value any, want string) string {
	switch typed := value.(type) {
	case map[string]any:
		for key, nested := range typed {
			if strings.EqualFold(strings.TrimSpace(key), want) {
				if scalar := cleanScalar(nested); scalar != "" {
					return scalar
				}
			}
			if scalar := findScalarByKey(nested, want); scalar != "" {
				return scalar
			}
		}
	case []any:
		for _, nested := range typed {
			if scalar := findScalarByKey(nested, want); scalar != "" {
				return scalar
			}
		}
	}
	return ""
}

func firstMapScalar(item map[string]any, keys ...string) string {
	for _, want := range keys {
		if value, ok := lookupMapValue(item, want); ok {
			if scalar := cleanScalar(value); scalar != "" {
				return scalar
			}
		}
	}
	return ""
}

func lookupMapValue(item map[string]any, want string) (any, bool) {
	for key, value := range item {
		if strings.EqualFold(strings.TrimSpace(key), want) {
			return value, true
		}
	}
	return nil, false
}

func scalarStringMap(value any) map[string]string {
	typed, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]string, len(typed))
	for key, raw := range typed {
		if scalar := cleanScalar(raw); scalar != "" {
			out[key] = scalar
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
