package rangevalues

import (
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

const DefaultExpansionLimit = 10_000

var rangePattern = regexp.MustCompile(`(?i)^\s*([+-]?(?:\d+(?:\.\d*)?|\.\d+))\s*(?:-|to)\s*([+-]?(?:\d+(?:\.\d*)?|\.\d+))\s*$`)

type Bounds struct {
	Min       float64
	Max       float64
	Increment float64
	Precision int
}

type Selection struct {
	Values   []string
	HasRange bool
}

func Parse(input string, bounds Bounds, expansionLimit int) (Selection, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Selection{}, errors.New("a numeric value or range is required")
	}
	if err := validateBounds(bounds); err != nil {
		return Selection{}, err
	}
	if expansionLimit <= 0 {
		expansionLimit = DefaultExpansionLimit
	}

	seen := make(map[string]struct{})
	selection := Selection{}
	for _, rawToken := range strings.Split(input, ",") {
		token := strings.TrimSpace(rawToken)
		if token == "" {
			return Selection{}, errors.New("numeric input contains an empty value")
		}
		if match := rangePattern.FindStringSubmatch(token); match != nil {
			start, err := parseValue(match[1], bounds)
			if err != nil {
				return Selection{}, fmt.Errorf("invalid range start in %q: %w", token, err)
			}
			end, err := parseValue(match[2], bounds)
			if err != nil {
				return Selection{}, fmt.Errorf("invalid range end in %q: %w", token, err)
			}
			if start > end+tolerance(bounds.Increment) {
				return Selection{}, fmt.Errorf("range %q must be ascending", token)
			}
			selection.HasRange = true
			for value := start; value <= end+tolerance(bounds.Increment); value += bounds.Increment {
				if len(selection.Values) >= expansionLimit {
					return Selection{}, fmt.Errorf("numeric range expands beyond the safety limit of %d values", expansionLimit)
				}
				appendValue(&selection.Values, seen, value, bounds.Precision)
			}
			continue
		}
		value, err := parseValue(token, bounds)
		if err != nil {
			return Selection{}, fmt.Errorf("invalid numeric value %q: %w", token, err)
		}
		appendValue(&selection.Values, seen, value, bounds.Precision)
	}
	if len(selection.Values) == 0 {
		return Selection{}, errors.New("at least one numeric value is required")
	}
	return selection, nil
}

func validateBounds(bounds Bounds) error {
	if math.IsNaN(bounds.Min) || math.IsNaN(bounds.Max) || math.IsInf(bounds.Min, 0) || math.IsInf(bounds.Max, 0) {
		return errors.New("numeric bounds must be finite")
	}
	if bounds.Min > bounds.Max {
		return errors.New("numeric minimum exceeds maximum")
	}
	if bounds.Increment <= 0 || math.IsNaN(bounds.Increment) || math.IsInf(bounds.Increment, 0) {
		return errors.New("numeric increment must be greater than zero")
	}
	if bounds.Precision < 0 || bounds.Precision > 9 {
		return errors.New("numeric precision must be between 0 and 9")
	}
	return nil
}

func parseValue(raw string, bounds Bounds) (float64, error) {
	value, err := strconv.ParseFloat(strings.TrimSpace(raw), 64)
	if err != nil || math.IsNaN(value) || math.IsInf(value, 0) {
		return 0, errors.New("use a finite number")
	}
	if value < bounds.Min-tolerance(bounds.Increment) || value > bounds.Max+tolerance(bounds.Increment) {
		return 0, fmt.Errorf("must be between %s and %s", format(bounds.Min, bounds.Precision), format(bounds.Max, bounds.Precision))
	}
	steps := (value - bounds.Min) / bounds.Increment
	if math.Abs(steps-math.Round(steps)) > 1e-7 {
		return 0, fmt.Errorf("must follow increment %s from minimum %s", format(bounds.Increment, bounds.Precision), format(bounds.Min, bounds.Precision))
	}
	return rounded(value, bounds.Precision), nil
}

func appendValue(values *[]string, seen map[string]struct{}, value float64, precision int) {
	formatted := format(rounded(value, precision), precision)
	if _, exists := seen[formatted]; exists {
		return
	}
	seen[formatted] = struct{}{}
	*values = append(*values, formatted)
}

func rounded(value float64, precision int) float64 {
	factor := math.Pow10(precision)
	return math.Round(value*factor) / factor
}

func format(value float64, precision int) string {
	return strconv.FormatFloat(value, 'f', precision, 64)
}

func tolerance(increment float64) float64 {
	return math.Max(math.Abs(increment)*1e-9, 1e-9)
}
