package application

import (
	"strconv"
	"strings"
)

func EffectiveWorkerCount(requested int, plannedTests int64) int {
	if requested < 1 {
		requested = DefaultWorkerCount
	}
	requested = min(requested, MaximumWorkerCount)
	if plannedTests > 0 && plannedTests < int64(requested) {
		return int(plannedTests)
	}
	return requested
}

func ParseWorkerCount(value string) int {
	workers, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || workers < 1 {
		return DefaultWorkerCount
	}
	return min(workers, MaximumWorkerCount)
}

func ParseCaseLimit(value string) int {
	limit, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil || limit < 1 {
		return DefaultCaseLimit
	}
	return min(limit, MaximumCaseLimit)
}

func NormalizeCaseLimit(limit int) int {
	if limit < 1 {
		return DefaultCaseLimit
	}
	return min(limit, MaximumCaseLimit)
}
