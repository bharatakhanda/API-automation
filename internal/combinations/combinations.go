package combinations

import "sort"

// Axis represents one selectable API capability and its selected values.
type Axis struct {
	Name   string
	Values []string
}

// Combination represents one executable test case generated from selected axes.
type Combination map[string]string

// Generate returns the Cartesian product of all axes with at least one value.
// Empty axes are ignored so unsupported capabilities do not block execution.
func Generate(axes []Axis, limit int) []Combination {
	filtered := make([]Axis, 0, len(axes))
	for _, axis := range axes {
		values := uniqueNonEmpty(axis.Values)
		if axis.Name == "" || len(values) == 0 {
			continue
		}
		filtered = append(filtered, Axis{Name: axis.Name, Values: values})
	}
	if len(filtered) == 0 || limit == 0 {
		return nil
	}

	out := make([]Combination, 0)
	var walk func(int, Combination)
	walk = func(idx int, current Combination) {
		if limit > 0 && len(out) >= limit {
			return
		}
		if idx == len(filtered) {
			combo := make(Combination, len(current))
			for k, v := range current {
				combo[k] = v
			}
			out = append(out, combo)
			return
		}
		axis := filtered[idx]
		for _, value := range axis.Values {
			current[axis.Name] = value
			walk(idx+1, current)
		}
		delete(current, axis.Name)
	}
	walk(0, Combination{})
	return out
}

func uniqueNonEmpty(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}
