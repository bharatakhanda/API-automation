package application

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
	"api-automation/internal/pagevalues"
)

// CombinationForConstraintValidation converts internal planning markers to
// the exact Fiery attribute values used by capability constraint checks.
func CombinationForConstraintValidation(combination combinations.Combination) map[string]string {
	return CombinationToAttributes(combination)
}

// CombinationToAttributes serializes a planned case to exact Fiery wire
// attributes without mutating the source combination.
func CombinationToAttributes(combination combinations.Combination) map[string]string {
	attributes := CloneStringMap(combination)
	if custom, ok := attributes[PageRangeOptionID]; ok && strings.HasPrefix(custom, PageRangeInternalPrefix) {
		attributes[PageRangeOptionID] = strings.TrimPrefix(custom, PageRangeInternalPrefix)
	}
	// CWS/Postman evidence shows custom ranges are represented directly by
	// EFPageRange while DPP_PAGE_RANGE remains empty. Never emit the legacy
	// companion, even if stale data reaches combination generation.
	delete(attributes, PageRangeLegacyDataID)
	return attributes
}

// ValidateCustomPageRange checks a direct EFPageRange expression against the
// original imported document count. Exact advertised enum values need no check.
func ValidateCustomPageRange(attributes, jobAttributes map[string]string) error {
	expression := strings.TrimSpace(attributes[PageRangeOptionID])
	selection, err := pagevalues.Parse(expression, pagevalues.DefaultExpansionLimit)
	if err != nil {
		return nil
	}
	pageCount, ok := ImportedFilePageCount(jobAttributes)
	if !ok {
		return fmt.Errorf("fiery did not report the imported file's original page count after spooling")
	}
	return selection.ValidatePageCount(pageCount)
}

func ImportedFilePageCount(attributes map[string]string) (int, bool) {
	for _, key := range []string{"OrigPageCount", "num document pages", "pqm num pages", "PGM num pages", "num pages"} {
		value, err := strconv.Atoi(strings.TrimSpace(attributes[key]))
		if err == nil && value > 0 {
			return value, true
		}
	}
	return 0, false
}

// AttributeMatcher applies Fiery-specific readback semantics using a snapshot
// of the discovered capabilities.
type AttributeMatcher struct {
	Capabilities capabilities.Model
}

func (matcher AttributeMatcher) AttributesMatch(got, expected map[string]string) bool {
	for key, want := range expected {
		if !matcher.AttributeMapValueMatches(got, key, want) {
			return false
		}
	}
	return true
}

func (matcher AttributeMatcher) AttributeMapValueMatches(got map[string]string, key, want string) bool {
	if strings.EqualFold(key, PageRangeOptionID) {
		if _, err := pagevalues.Parse(want, pagevalues.DefaultExpansionLimit); err == nil {
			return PageRangeValueMatches(got, want)
		}
	}
	return matcher.AttributeValueMatches(key, got[key], want)
}

func PageRangeValueMatches(got map[string]string, want string) bool {
	value := strings.TrimSpace(got[PageRangeOptionID])
	if _, err := pagevalues.Parse(value, pagevalues.DefaultExpansionLimit); err != nil {
		return false
	}
	return pagevalues.Equivalent(value, want)
}

func (matcher AttributeMatcher) AttributeValueMatches(key, got, want string) bool {
	if strings.EqualFold(key, OutputProfileOptionID) {
		got = NormalizeOutputProfileValue(got)
		want = NormalizeOutputProfileValue(want)
	}
	if got == want {
		return true
	}
	// Fiery often omits attributes whose selected value is the discovered
	// default. A different explicit readback must still fail strictly.
	if strings.TrimSpace(got) != "" {
		return false
	}
	option, ok := matcher.Capabilities.OptionByID(key)
	return ok && option.Value == want
}

func NormalizeOutputProfileValue(value string) string {
	// U+FEFF is part of EFOutProfile wire identity. Ignore it only for display
	// and comparison; CombinationToAttributes deliberately preserves it.
	return strings.TrimSpace(strings.Trim(strings.TrimSpace(value), "\ufeff"))
}

func DisplayOptionValue(optionID, value string) string {
	if strings.EqualFold(strings.TrimSpace(optionID), OutputProfileOptionID) {
		return NormalizeOutputProfileValue(value)
	}
	return value
}

func OptionValueMatches(optionID, left, right string) bool {
	if strings.EqualFold(strings.TrimSpace(optionID), OutputProfileOptionID) {
		left = NormalizeOutputProfileValue(left)
		right = NormalizeOutputProfileValue(right)
	}
	return strings.EqualFold(strings.TrimSpace(left), strings.TrimSpace(right))
}

func CloneStringMap[M ~map[string]string](source M) map[string]string {
	if len(source) == 0 {
		return nil
	}
	clone := make(map[string]string, len(source))
	for key, value := range source {
		clone[key] = value
	}
	return clone
}

func SelectedReadbackValues(got, selected map[string]string) map[string]string {
	if len(selected) == 0 {
		return nil
	}
	values := make(map[string]string, len(selected))
	for key := range selected {
		values[key] = got[key]
	}
	return values
}

// ExpectedConstraintRejection accepts only explicit client-side constraint
// evidence and rejects cancellation, timeout, and server/transport failures.
func ExpectedConstraintRejection(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	message := strings.ToLower(err.Error())
	for _, serverFailure := range []string{"http 500", "http 502", "http 503", "http 504"} {
		if strings.Contains(message, serverFailure) {
			return false
		}
	}
	clientStatus := strings.Contains(message, "http 400") || strings.Contains(message, "http 409") || strings.Contains(message, "http 422")
	constraintEvidence := strings.Contains(message, "constraint") || strings.Contains(message, "conflict") || strings.Contains(message, "incompat") || strings.Contains(message, "compatible value")
	return clientStatus && constraintEvidence
}
