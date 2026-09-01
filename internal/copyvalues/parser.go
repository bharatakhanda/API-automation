package copyvalues

import (
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

const (
	Minimum = 1
	Maximum = 9999
)

var rangePattern = regexp.MustCompile(`(?i)^\s*(\d+)\s*(?:-|to)\s*(\d+)\s*$`)

type Selection struct {
	Values   []string
	HasRange bool
}

// Parse accepts individual values and inclusive ranges, for example:
//
//	1
//	1, 5, 10, 15
//	5-10
//	1, 5 to 10, 15
func Parse(input string) (Selection, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return Selection{}, errors.New("copies is required")
	}
	seen := make([]bool, Maximum+1)
	selection := Selection{}
	for _, rawToken := range strings.Split(input, ",") {
		token := strings.TrimSpace(rawToken)
		if token == "" {
			return Selection{}, errors.New("copies contains an empty value")
		}
		if match := rangePattern.FindStringSubmatch(token); match != nil {
			start, err := parseOne(match[1])
			if err != nil {
				return Selection{}, err
			}
			end, err := parseOne(match[2])
			if err != nil {
				return Selection{}, err
			}
			if start > end {
				return Selection{}, fmt.Errorf("copies range %q must be ascending", token)
			}
			selection.HasRange = true
			for value := start; value <= end; value++ {
				appendValue(&selection.Values, seen, value)
			}
			continue
		}
		value, err := parseOne(token)
		if err != nil {
			return Selection{}, fmt.Errorf("invalid copies value %q: %w", token, err)
		}
		appendValue(&selection.Values, seen, value)
	}
	if len(selection.Values) == 0 {
		return Selection{}, errors.New("copies must include at least one value")
	}
	return selection, nil
}

func parseOne(value string) (int, error) {
	parsed, err := strconv.Atoi(strings.TrimSpace(value))
	if err != nil {
		return 0, errors.New("copies must contain whole numbers or ranges such as 5-10")
	}
	if parsed < Minimum || parsed > Maximum {
		return 0, fmt.Errorf("copies must be between %d and %d", Minimum, Maximum)
	}
	return parsed, nil
}

func appendValue(values *[]string, seen []bool, value int) {
	if seen[value] {
		return
	}
	seen[value] = true
	*values = append(*values, strconv.Itoa(value))
}
