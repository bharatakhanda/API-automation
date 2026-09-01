package joboutcome

import (
	"fmt"
	"strconv"
	"strings"
)

type Policy struct {
	RequireProcessedRaster bool
	RequirePrinted         bool
	ExpectCanceled         bool
}

type Outcome struct {
	Pass           bool
	Reason         string
	Status         string
	State          string
	Error          string
	PDLError       string
	LastEvent      string
	RasterEvidence string
}

func Evaluate(attributes map[string]string, policy Policy) Outcome {
	outcome := Outcome{
		Status:    first(attributes, "status", "display status", "log status"),
		State:     first(attributes, "state", "pqm state", "pqm job state (old)"),
		Error:     meaningfulError(first(attributes, "error")),
		PDLError:  meaningfulError(first(attributes, "pdl error")),
		LastEvent: first(attributes, "last joblog event", "last event"),
	}
	outcome.RasterEvidence = rasterEvidence(attributes)

	if policy.ExpectCanceled {
		if cancellationObserved(attributes) {
			outcome.Pass = true
			outcome.Reason = "Fiery acknowledged the expected cancellation"
			return outcome
		}
		outcome.Reason = "expected cancellation was not reflected in the final Fiery job state"
		return outcome
	}

	if failure := explicitFailure(outcome, attributes); failure != "" {
		outcome.Reason = failure
		return outcome
	}

	if policy.RequireProcessedRaster {
		if !equalsAny(outcome.Status, "done ripping") {
			outcome.Reason = fmt.Sprintf("expected status done ripping after processing, got %q", outcome.Status)
			return outcome
		}
		if !equalsAny(outcome.State, "processed") {
			outcome.Reason = fmt.Sprintf("expected state processed after processing, got %q", outcome.State)
			return outcome
		}
		if !hasRaster(attributes) {
			outcome.Reason = "processing completed without raster/page evidence"
			return outcome
		}
		outcome.Pass = true
		outcome.Reason = "Fiery reports done ripping, processed state, and raster/page evidence"
		return outcome
	}

	if policy.RequirePrinted {
		if !isTruthy(first(attributes, "has been printed?")) && !containsAny(outcome.Status, "done printing", "printed") {
			outcome.Reason = fmt.Sprintf("expected printed completion, got status %q state %q", outcome.Status, outcome.State)
			return outcome
		}
		outcome.Pass = true
		outcome.Reason = "Fiery reports successful print completion"
		return outcome
	}

	outcome.Pass = true
	outcome.Reason = "Fiery job has no terminal error or cancellation state"
	return outcome
}

func (o Outcome) Summary() string {
	return fmt.Sprintf("%s; status=%q state=%q error=%q pdl_error=%q last_event=%q raster=%q", o.Reason, o.Status, o.State, o.Error, o.PDLError, o.LastEvent, o.RasterEvidence)
}

func explicitFailure(outcome Outcome, attributes map[string]string) string {
	if outcome.Error != "" {
		return "Fiery job error: " + outcome.Error
	}
	if outcome.PDLError != "" {
		return "Fiery PDL error: " + outcome.PDLError
	}
	combined := strings.ToLower(strings.Join([]string{outcome.Status, outcome.State, first(attributes, "display status"), first(attributes, "print status"), outcome.LastEvent}, " "))
	for _, phrase := range []string{"process error", "process canceled", "process cancelled", "pdl error", "log_aborted", "failed", "failure", "aborted", "unsupported"} {
		if strings.Contains(combined, phrase) {
			return fmt.Sprintf("Fiery job ended in a failure state: status=%q state=%q", outcome.Status, outcome.State)
		}
	}
	return ""
}

func cancellationObserved(attributes map[string]string) bool {
	if isTruthy(first(attributes, "has been canceled?", "has been cancelled?")) {
		return true
	}
	for _, key := range []string{"status", "state", "display status", "print status", "last joblog event", "recent action"} {
		value := strings.ToLower(first(attributes, key))
		if containsAny(value, "cancel", "abort") {
			return true
		}
	}
	return false
}

func hasRaster(attributes map[string]string) bool {
	if isTruthy(first(attributes, "has disk raster?", "has memory raster?")) {
		return true
	}
	for _, key := range []string{"total pages ripped", "total pages rendered", "TotalPagesRipped", "num pages"} {
		if number(first(attributes, key)) > 0 {
			return true
		}
	}
	return false
}

func rasterEvidence(attributes map[string]string) string {
	return fmt.Sprintf("disk=%s memory=%s pages=%s ripped=%s rendered=%s",
		fallback(first(attributes, "has disk raster?"), "unknown"),
		fallback(first(attributes, "has memory raster?"), "unknown"),
		fallback(first(attributes, "num pages"), "unknown"),
		fallback(first(attributes, "total pages ripped", "TotalPagesRipped"), "unknown"),
		fallback(first(attributes, "total pages rendered"), "unknown"),
	)
}

func meaningfulError(value string) string {
	value = strings.TrimSpace(value)
	if equalsAny(value, "", "none", "no", "false", "0", "ok", "uninit", "null") {
		return ""
	}
	return value
}

func first(attributes map[string]string, keys ...string) string {
	for _, key := range keys {
		if value := strings.TrimSpace(attributes[key]); value != "" {
			return value
		}
		for actual, value := range attributes {
			if strings.EqualFold(actual, key) && strings.TrimSpace(value) != "" {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

func number(value string) float64 {
	parsed, _ := strconv.ParseFloat(strings.TrimSpace(value), 64)
	return parsed
}

func isTruthy(value string) bool {
	return equalsAny(value, "yes", "true", "1", "on")
}

func equalsAny(value string, candidates ...string) bool {
	value = strings.TrimSpace(value)
	for _, candidate := range candidates {
		if strings.EqualFold(value, candidate) {
			return true
		}
	}
	return false
}

func containsAny(value string, candidates ...string) bool {
	value = strings.ToLower(value)
	for _, candidate := range candidates {
		if strings.Contains(value, strings.ToLower(candidate)) {
			return true
		}
	}
	return false
}

func fallback(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
