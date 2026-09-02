package application

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"api-automation/internal/joboutcome"
)

type PollingPolicy struct {
	ImportVisibleTimeout   time.Duration
	ImportVisibleInterval  time.Duration
	SpoolingTimeout        time.Duration
	SpoolingInterval       time.Duration
	RIPTimeout             time.Duration
	RIPInterval            time.Duration
	ProductionTimeout      time.Duration
	ProductionInterval     time.Duration
	PressPrintTimeout      time.Duration
	PressPrintInterval     time.Duration
	PrintTimeout           time.Duration
	PrintInterval          time.Duration
	CancelRIPTimeout       time.Duration
	CancelWaitingTimeout   time.Duration
	CancelPrintingTimeout  time.Duration
	CancelReadyInterval    time.Duration
	CancelObservedTimeout  time.Duration
	CancelObservedInterval time.Duration
	ReadbackTimeout        time.Duration
	ReadbackInterval       time.Duration
}

func DefaultPollingPolicy() PollingPolicy {
	return PollingPolicy{
		ImportVisibleTimeout:   2 * time.Minute,
		ImportVisibleInterval:  time.Second,
		SpoolingTimeout:        4 * time.Minute,
		SpoolingInterval:       time.Second,
		RIPTimeout:             6 * time.Minute,
		RIPInterval:            2 * time.Second,
		ProductionTimeout:      2 * time.Minute,
		ProductionInterval:     2 * time.Second,
		PressPrintTimeout:      2 * time.Minute,
		PressPrintInterval:     2 * time.Second,
		PrintTimeout:           10 * time.Minute,
		PrintInterval:          3 * time.Second,
		CancelRIPTimeout:       3 * time.Minute,
		CancelWaitingTimeout:   2 * time.Minute,
		CancelPrintingTimeout:  3 * time.Minute,
		CancelReadyInterval:    250 * time.Millisecond,
		CancelObservedTimeout:  2 * time.Minute,
		CancelObservedInterval: 500 * time.Millisecond,
		ReadbackTimeout:        20 * time.Second,
		ReadbackInterval:       2 * time.Second,
	}
}

func normalizePollingPolicy(policy PollingPolicy) PollingPolicy {
	defaults := DefaultPollingPolicy()
	if policy.ImportVisibleTimeout <= 0 {
		policy.ImportVisibleTimeout = defaults.ImportVisibleTimeout
	}
	if policy.ImportVisibleInterval <= 0 {
		policy.ImportVisibleInterval = defaults.ImportVisibleInterval
	}
	if policy.SpoolingTimeout <= 0 {
		policy.SpoolingTimeout = defaults.SpoolingTimeout
	}
	if policy.SpoolingInterval <= 0 {
		policy.SpoolingInterval = defaults.SpoolingInterval
	}
	if policy.RIPTimeout <= 0 {
		policy.RIPTimeout = defaults.RIPTimeout
	}
	if policy.RIPInterval <= 0 {
		policy.RIPInterval = defaults.RIPInterval
	}
	if policy.ProductionTimeout <= 0 {
		policy.ProductionTimeout = defaults.ProductionTimeout
	}
	if policy.ProductionInterval <= 0 {
		policy.ProductionInterval = defaults.ProductionInterval
	}
	if policy.PressPrintTimeout <= 0 {
		policy.PressPrintTimeout = defaults.PressPrintTimeout
	}
	if policy.PressPrintInterval <= 0 {
		policy.PressPrintInterval = defaults.PressPrintInterval
	}
	if policy.PrintTimeout <= 0 {
		policy.PrintTimeout = defaults.PrintTimeout
	}
	if policy.PrintInterval <= 0 {
		policy.PrintInterval = defaults.PrintInterval
	}
	if policy.CancelRIPTimeout <= 0 {
		policy.CancelRIPTimeout = defaults.CancelRIPTimeout
	}
	if policy.CancelWaitingTimeout <= 0 {
		policy.CancelWaitingTimeout = defaults.CancelWaitingTimeout
	}
	if policy.CancelPrintingTimeout <= 0 {
		policy.CancelPrintingTimeout = defaults.CancelPrintingTimeout
	}
	if policy.CancelReadyInterval <= 0 {
		policy.CancelReadyInterval = defaults.CancelReadyInterval
	}
	if policy.CancelObservedTimeout <= 0 {
		policy.CancelObservedTimeout = defaults.CancelObservedTimeout
	}
	if policy.CancelObservedInterval <= 0 {
		policy.CancelObservedInterval = defaults.CancelObservedInterval
	}
	if policy.ReadbackTimeout <= 0 {
		policy.ReadbackTimeout = defaults.ReadbackTimeout
	}
	if policy.ReadbackInterval <= 0 {
		policy.ReadbackInterval = defaults.ReadbackInterval
	}
	return policy
}

type LifecycleFailure struct {
	Outcome    joboutcome.Outcome
	Attributes map[string]string
}

func (failure *LifecycleFailure) Error() string { return failure.Outcome.Summary() }

type LifecycleLogFunc func(string)

type ReadbackObserver func(actual, expected map[string]string, matched bool, err error)

func ExecuteModeLifecycle(ctx context.Context, client AutomationClient, jobID string, mode RunMode, policy PollingPolicy, log LifecycleLogFunc) error {
	policy = normalizePollingPolicy(policy)
	writeLog := func(format string, args ...any) {
		if log != nil {
			log(fmt.Sprintf(format, args...))
		}
	}
	for _, action := range mode.Actions {
		switch action {
		case "rip":
			writeLog("Running RIP for job %s", jobID)
			if err := client.JobAction(ctx, jobID, "rip"); err != nil {
				return err
			}
			writeLog("Waiting for job %s status=done ripping or a terminal Fiery failure after RIP", jobID)
			observed, err := WaitJobCondition(ctx, client, jobID, "RIP completion", policy.RIPTimeout, policy.RIPInterval, func(attributes map[string]string) bool {
				if StatusEquals("done ripping")(attributes) {
					return true
				}
				return !joboutcome.Evaluate(attributes, joboutcome.Policy{}).Pass
			})
			if err != nil {
				return err
			}
			if outcome := joboutcome.Evaluate(observed, joboutcome.Policy{}); !outcome.Pass {
				return &LifecycleFailure{Outcome: outcome, Attributes: observed}
			}
		case "production":
			writeLog("Moving job %s to production release state", jobID)
			if err := client.UpdateJobAttributes(ctx, jobID, map[string]string{"job release state": "production"}); err != nil {
				return err
			}
			writeLog("Waiting for job %s job release state=production", jobID)
			if _, err := WaitJobCondition(ctx, client, jobID, "production release state", policy.ProductionTimeout, policy.ProductionInterval, AttrEquals("job release state", "production")); err != nil {
				return err
			}
		case "press_print":
			writeLog("Running press_print for job %s", jobID)
			if err := client.JobAction(ctx, jobID, "press_print"); err != nil {
				return err
			}
			writeLog("Confirming press_print accepted for job %s", jobID)
			if _, err := WaitJobCondition(ctx, client, jobID, "press_print accepted", policy.PressPrintTimeout, policy.PressPrintInterval, PressPrintAccepted); err != nil {
				return err
			}
		case "print":
			writeLog("Running print for job %s", jobID)
			if err := client.JobAction(ctx, jobID, "print"); err != nil {
				return err
			}
			writeLog("Waiting for job %s print completion", jobID)
			if _, err := WaitJobCondition(ctx, client, jobID, "print completion", policy.PrintTimeout, policy.PrintInterval, PrintCompleted); err != nil {
				return err
			}
		case "cancel_ripping":
			writeLog("Starting RIP for dedicated cancel test job %s after stable spooling", jobID)
			if err := client.JobAction(ctx, jobID, "rip"); err != nil {
				return err
			}
			if err := cancelDedicatedJob(ctx, client, jobID, "processing/ripping", policy.CancelRIPTimeout, policy, func(attributes map[string]string) bool {
				active, state := ActivelyProcessingJob(attributes)
				return active && !strings.Contains(strings.ToLower(state), "printing")
			}, writeLog); err != nil {
				return err
			}
		case "cancel_waiting":
			if err := cancelDedicatedJob(ctx, client, jobID, "waiting to print", policy.CancelWaitingTimeout, policy, func(attributes map[string]string) bool {
				waiting, _ := WaitingToPrintJob(attributes)
				return waiting
			}, writeLog); err != nil {
				return err
			}
		case "cancel_printing":
			writeLog("Starting print for dedicated cancel test job %s", jobID)
			if err := client.JobAction(ctx, jobID, "print"); err != nil {
				return err
			}
			if err := cancelDedicatedJob(ctx, client, jobID, "printing", policy.CancelPrintingTimeout, policy, PrintingJob, writeLog); err != nil {
				return err
			}
		}
	}
	return nil
}

func cancelDedicatedJob(ctx context.Context, client AutomationClient, jobID, scenario string, timeout time.Duration, policy PollingPolicy, ready func(map[string]string) bool, log func(string, ...any)) error {
	log("Waiting for job %s cancellation scenario=%s", jobID, scenario)
	if _, err := WaitJobCondition(ctx, client, jobID, scenario+" before cancel", timeout, policy.CancelReadyInterval, ready); err != nil {
		return fmt.Errorf("cancel was not sent because the job never reached %s: %w", scenario, err)
	}
	log("Cancelling job %s in scenario=%s", jobID, scenario)
	if err := client.CancelJob(ctx, jobID); err != nil {
		return err
	}
	if _, err := WaitJobCondition(ctx, client, jobID, "cancel acknowledgement", policy.CancelObservedTimeout, policy.CancelObservedInterval, CancelObserved); err != nil {
		return err
	}
	log("Cancel acknowledged for job %s in scenario=%s", jobID, scenario)
	return nil
}

func WaitJobCondition(ctx context.Context, client AutomationClient, jobID, description string, timeout, interval time.Duration, match func(map[string]string) bool) (map[string]string, error) {
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	if interval <= 0 {
		interval = time.Second
	}
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	var last map[string]string
	var lastErr error
	for {
		attributes, err := client.GetJobAttributes(ctx, jobID)
		if err == nil {
			last = attributes
			if match(attributes) {
				return attributes, nil
			}
		} else {
			lastErr = err
		}
		select {
		case <-ctx.Done():
			return last, ctx.Err()
		case <-deadline.C:
			if lastErr != nil {
				return last, fmt.Errorf("wait for job %s %s timed out after %s; last GET error: %w", jobID, description, timeout, lastErr)
			}
			return last, fmt.Errorf("wait for job %s %s timed out after %s; last status=%q state=%q release=%q recent=%q keys=%s", jobID, description, timeout, last["status"], last["state"], last["job release state"], last["recent action"], shortText(strings.Join(sortedStringKeys(last), ","), 220))
		case <-ticker.C:
		}
	}
}

func ReadBackAttributes(ctx context.Context, client AutomationClient, jobID string, expected map[string]string, matcher AttributeMatcher, policy PollingPolicy, observe ReadbackObserver) (map[string]string, error) {
	policy = normalizePollingPolicy(policy)
	if len(expected) == 0 {
		actual, err := client.GetJobAttributes(ctx, jobID)
		if observe != nil {
			observe(actual, expected, err == nil, err)
		}
		return actual, err
	}
	deadline := time.NewTimer(policy.ReadbackTimeout)
	defer deadline.Stop()
	ticker := time.NewTicker(policy.ReadbackInterval)
	defer ticker.Stop()
	var actual map[string]string
	var lastErr error
	for {
		attempt, err := client.GetJobAttributes(ctx, jobID)
		lastErr = err
		if err == nil {
			actual = attempt
			matched := matcher.AttributesMatch(actual, expected)
			if observe != nil {
				observe(actual, expected, matched, nil)
			}
			if matched {
				return actual, nil
			}
		} else if observe != nil {
			observe(nil, expected, false, err)
		}
		select {
		case <-ctx.Done():
			return actual, ctx.Err()
		case <-deadline.C:
			if actual == nil && lastErr != nil {
				return nil, fmt.Errorf("read back job %s attributes: %w", jobID, lastErr)
			}
			return actual, nil
		case <-ticker.C:
		}
	}
}

func StatusEquals(want string) func(map[string]string) bool {
	return func(attributes map[string]string) bool {
		return strings.EqualFold(strings.TrimSpace(attributes["status"]), want)
	}
}

func AttrEquals(key, want string) func(map[string]string) bool {
	return func(attributes map[string]string) bool {
		return strings.EqualFold(strings.TrimSpace(attributes[key]), want)
	}
}

func PressPrintAccepted(attributes map[string]string) bool {
	if strings.EqualFold(attributes["queued for printing?"], "yes") || strings.EqualFold(attributes["is committed to print?"], "yes") || strings.EqualFold(attributes["is printing?"], "yes") {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(attributes["status"]))
	recent := strings.ToLower(strings.TrimSpace(attributes["recent action"]))
	return strings.Contains(status, "print") || strings.Contains(recent, "press_print") || strings.Contains(recent, "press print")
}

func PrintCompleted(attributes map[string]string) bool {
	if strings.EqualFold(attributes["has been printed?"], "yes") || strings.EqualFold(attributes["status"], "done printing") || strings.EqualFold(attributes["display status"], "done printing") {
		return true
	}
	status := strings.ToLower(strings.TrimSpace(attributes["status"]))
	return strings.Contains(status, "done print") || strings.Contains(status, "printed")
}

func PrintingJob(attributes map[string]string) bool {
	if IsTruthy(attributes["is printing?"]) {
		return true
	}
	for _, key := range []string{"status", "state", "display status", "current action"} {
		value := strings.ToLower(strings.TrimSpace(attributes[key]))
		if strings.Contains(value, "printing") && !strings.Contains(value, "done") && !strings.Contains(value, "complete") {
			return true
		}
	}
	return false
}

func CancelObserved(attributes map[string]string) bool {
	for _, key := range []string{"status", "state", "display status", "recent action", "current action"} {
		value := strings.ToLower(strings.TrimSpace(attributes[key]))
		if strings.Contains(value, "cancel") || strings.Contains(value, "abort") {
			return true
		}
	}
	cancelable, state := CancelableJob(attributes)
	if cancelable || state == "unknown" {
		return false
	}
	state = strings.ToLower(state)
	return !strings.Contains(state, "done print") && !strings.Contains(state, "complete") && !strings.Contains(state, "printed")
}

func ActivelyProcessingJob(attributes map[string]string) (bool, string) {
	reported := make([]string, 0, 4)
	for key, value := range attributes {
		keyLower := strings.ToLower(strings.TrimSpace(key))
		value = strings.TrimSpace(value)
		if strings.HasPrefix(keyLower, "is ") {
			activeKey := strings.Contains(keyLower, "printing") || strings.Contains(keyLower, "processing") || strings.Contains(keyLower, "ripping")
			if activeKey && IsTruthy(value) {
				return true, strings.TrimSpace(key) + "=" + value
			}
		}
		stateKey := keyLower == "status" || keyLower == "state" || keyLower == "display status" || strings.Contains(keyLower, "job status") || strings.Contains(keyLower, "current action")
		if !stateKey || value == "" {
			continue
		}
		reported = append(reported, strings.TrimSpace(key)+"="+value)
		valueLower := strings.ToLower(value)
		waitingOrTerminal := false
		for _, term := range []string{"done", "complete", "cancel", "abort", "error", "fail", "held", "queue", "wait", "ready"} {
			if strings.Contains(valueLower, term) {
				waitingOrTerminal = true
				break
			}
		}
		if waitingOrTerminal {
			continue
		}
		for _, term := range []string{"printing", "processing", "ripping"} {
			if strings.Contains(valueLower, term) {
				return true, strings.TrimSpace(key) + "=" + value
			}
		}
	}
	if len(reported) == 0 {
		return false, "unknown"
	}
	sort.Strings(reported)
	return false, strings.Join(reported, ", ")
}

func WaitingToPrintJob(attributes map[string]string) (bool, string) {
	for _, key := range []string{"queued for printing?", "is committed to print?", "waiting to print?", "ready to print?"} {
		if IsTruthy(attributes[key]) {
			return true, key + "=" + strings.TrimSpace(attributes[key])
		}
	}
	for _, key := range []string{"status", "state", "display status", "current action"} {
		value := strings.TrimSpace(attributes[key])
		valueLower := strings.ToLower(value)
		for _, phrase := range []string{"waiting to print", "ready to print", "queued for print", "print queue"} {
			if strings.Contains(valueLower, phrase) {
				return true, key + "=" + value
			}
		}
	}
	if strings.EqualFold(strings.TrimSpace(attributes["job release state"]), "production") {
		if active, _ := ActivelyProcessingJob(attributes); !active && !PrintCompleted(attributes) {
			return true, "job release state=production"
		}
	}
	return false, "unknown"
}

func CancelableJob(attributes map[string]string) (bool, string) {
	if active, state := ActivelyProcessingJob(attributes); active {
		return true, state
	}
	if waiting, state := WaitingToPrintJob(attributes); waiting {
		return true, state
	}
	_, state := ActivelyProcessingJob(attributes)
	return false, state
}

func IsTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}
