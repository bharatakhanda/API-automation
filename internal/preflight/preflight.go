package preflight

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"api-automation/internal/capabilities"
	"api-automation/internal/fiery"
)

type Check struct {
	Passed   bool   `json:"passed"`
	Property string `json:"property"`
	Value    string `json:"value,omitempty"`
	Display  string `json:"display,omitempty"`
}

type Summary struct {
	TotalChecks  int `json:"totalChecks"`
	PassedChecks int `json:"passedChecks"`
	FailedChecks int `json:"failedChecks"`
}

type EnvironmentSnapshot struct {
	ExecutionInfo    ExecutionInfo      `json:"executionInfo"`
	DiscoverySummary DiscoverySummary   `json:"discoverySummary"`
	Capabilities     capabilities.Model `json:"capabilities"`
	Checks           map[string]Check   `json:"checks"`
	FailedProperties []string           `json:"failedProperties"`
	Summary          Summary            `json:"summary"`
	OverallStatus    string             `json:"overallStatus"`
	Certification    Certification      `json:"certification"`
}

type ExecutionInfo struct {
	Timestamp  time.Time `json:"timestamp"`
	Server     string    `json:"server"`
	APIVersion string    `json:"apiVersion"`
}

type DiscoverySummary struct {
	ServerName       string `json:"serverName"`
	ServerVersion    string `json:"serverVersion"`
	SerialNumber     string `json:"serialNumber"`
	QueuesCount      int    `json:"queuesCount"`
	OptionsCount     int    `json:"optionsCount"`
	RawEndpointCount int    `json:"rawEndpointCount"`
}

type Certification struct {
	Certified bool   `json:"certified"`
	Message   string `json:"message"`
}

func Run(snapshot fiery.CapabilitySnapshot, model capabilities.Model) EnvironmentSnapshot {
	checks := map[string]Check{
		"ServerVersion":  checkPresent("ServerVersion", model.Version),
		"ServerName":     checkPresent("ServerName", model.ServerName),
		"Queues":         checkCount("Queues", len(model.Queues)),
		"EFResolution":   checkOption("EFResolution", model, "EFResolution"),
		"EFBrightness":   checkOption("EFBrightness", model, "EFBrightness"),
		"EFColorMode":    checkOption("EFColorMode", model, "EFColorMode"),
		"EFMediaType":    checkOption("EFMediaType", model, "EFMediaType"),
		"EFInputSlot":    checkOption("EFInputSlot", model, "InputSlot"),
		"EFOutputBin":    checkOption("EFOutputBin", model, "EFOutputBin"),
		"EFPrintQuality": checkOption("EFPrintQuality", model, "EFTextGfxQual"),
		"EFRotation":     checkOption("EFRotation", model, "EFRotateDocument"),
		"EFPrintSpeed":   checkOption("EFPrintSpeed", model, "EFPrintSpeed"),
	}

	passed := 0
	failed := make([]string, 0)
	for _, check := range checks {
		if check.Passed {
			passed++
		} else {
			failed = append(failed, check.Property)
		}
	}
	sort.Strings(failed)
	status := "FAIL"
	message := "Environment Validation Failed"
	if passed == len(checks) {
		status = "PASS"
		message = "Environment Ready For Automation"
	}
	return EnvironmentSnapshot{
		ExecutionInfo:    ExecutionInfo{Timestamp: time.Now().UTC(), Server: snapshot.Server, APIVersion: snapshot.APIVersion},
		DiscoverySummary: DiscoverySummary{ServerName: model.ServerName, ServerVersion: model.Version, SerialNumber: model.SerialNumber, QueuesCount: len(model.Queues), OptionsCount: len(model.Options), RawEndpointCount: len(snapshot.Endpoints)},
		Capabilities:     model,
		Checks:           checks,
		FailedProperties: failed,
		Summary:          Summary{TotalChecks: len(checks), PassedChecks: passed, FailedChecks: len(checks) - passed},
		OverallStatus:    status,
		Certification:    Certification{Certified: status == "PASS", Message: message},
	}
}

func Save(snapshot EnvironmentSnapshot, dir string) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "EnvironmentSnapshot-"+snapshot.ExecutionInfo.Timestamp.Format("20060102-150405")+".json")
	data, err := json.MarshalIndent(snapshot, "", "  ")
	if err != nil {
		return "", err
	}
	return path, os.WriteFile(path, data, 0o600)
}

func checkPresent(property, value string) Check {
	return Check{Passed: value != "", Property: property, Value: value}
}

func checkCount(property string, count int) Check {
	return Check{Passed: count > 0, Property: property, Value: fmt.Sprint(count)}
}

func checkOption(property string, model capabilities.Model, optionID string) Check {
	option, ok := model.OptionByID(optionID)
	return Check{Passed: ok && len(option.Values) > 0, Property: property, Value: option.Value, Display: option.Label}
}
