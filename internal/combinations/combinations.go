package combinations

import (
	"crypto/rand"
	"math/big"
	"sort"
)

type Strategy string

const (
	// StrategySingle creates one configuration from the first value on each
	// normalized axis. StrategySelected is retained for legacy presets and API
	// callers where it means the Cartesian product of explicitly selected values.
	StrategySingle   Strategy = "single"
	StrategySelected Strategy = "selected"
	StrategyAll      Strategy = "all"
	StrategyPairwise Strategy = "pairwise"
	StrategyRandom   Strategy = "random"
)

// Axis represents one selectable API capability and its selected values.
type Axis struct {
	Name   string
	Values []string
}

// Combination represents one executable test case generated from selected axes.
type Combination map[string]string

func Generate(axes []Axis, limit int) []Combination { return cartesian(axes, limit) }

func GenerateWithStrategy(axes []Axis, strategy Strategy, limit int) []Combination {
	switch strategy {
	case StrategySingle:
		return single(axes, limit)
	case StrategyPairwise:
		return pairwise(axes, limit)
	case StrategyRandom:
		return randomCombinations(axes, limit)
	default:
		return cartesian(axes, limit)
	}
}

func single(axes []Axis, limit int) []Combination {
	axes = normalizeAxes(axes)
	if len(axes) == 0 || limit == 0 {
		return nil
	}
	combination := make(Combination, len(axes))
	for _, axis := range axes {
		combination[axis.Name] = axis.Values[0]
	}
	return []Combination{combination}
}

func cartesian(axes []Axis, limit int) []Combination {
	filtered := normalizeAxes(axes)
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
			out = append(out, cloneCombination(current))
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

// pairwise uses incremental horizontal and vertical growth. Unlike an
// exhaustive candidate-based implementation, its memory use is proportional
// to the number of value pairs rather than the full Cartesian product.
func pairwise(axes []Axis, limit int) []Combination {
	axes = normalizeAxes(axes)
	if len(axes) == 0 || limit == 0 {
		return nil
	}
	if len(axes) == 1 {
		return cartesian(axes, limit)
	}

	rows := cartesian(axes[:2], limit)
	for next := 2; next < len(axes); next++ {
		uncovered := pairsWithAxis(axes, next)

		// Horizontal growth assigns the value that covers the most new pairs to
		// every existing row.
		for _, row := range rows {
			bestValue := axes[next].Values[0]
			bestScore := -1
			for _, candidate := range axes[next].Values {
				score := 0
				for previous := 0; previous < next; previous++ {
					if _, ok := uncovered[pairKey(axes[previous].Name, row[axes[previous].Name], axes[next].Name, candidate)]; ok {
						score++
					}
				}
				if score > bestScore {
					bestValue, bestScore = candidate, score
				}
			}
			row[axes[next].Name] = bestValue
			removeCoveredPairs(uncovered, row, axes, next)
		}

		// Vertical growth adds rows only for pairs that horizontal growth could
		// not cover. Every added row is complete and can cover several pairs.
		for len(uncovered) > 0 && (limit < 0 || len(rows) < limit) {
			previous, previousValue, nextValue, ok := firstUncoveredPair(uncovered, axes, next)
			if !ok {
				break
			}
			row := make(Combination, next+1)
			row[axes[next].Name] = nextValue
			for axisIndex := 0; axisIndex < next; axisIndex++ {
				value := axes[axisIndex].Values[0]
				if axisIndex == previous {
					value = previousValue
				} else {
					for _, candidate := range axes[axisIndex].Values {
						key := pairKey(axes[axisIndex].Name, candidate, axes[next].Name, nextValue)
						if _, found := uncovered[key]; found {
							value = candidate
							break
						}
					}
				}
				row[axes[axisIndex].Name] = value
			}
			removeCoveredPairs(uncovered, row, axes, next)
			rows = append(rows, row)
		}
	}
	return rows
}

func pairsWithAxis(axes []Axis, next int) map[string]struct{} {
	pairs := make(map[string]struct{})
	for previous := 0; previous < next; previous++ {
		for _, previousValue := range axes[previous].Values {
			for _, nextValue := range axes[next].Values {
				pairs[pairKey(axes[previous].Name, previousValue, axes[next].Name, nextValue)] = struct{}{}
			}
		}
	}
	return pairs
}

func removeCoveredPairs(uncovered map[string]struct{}, row Combination, axes []Axis, next int) {
	for previous := 0; previous < next; previous++ {
		delete(uncovered, pairKey(axes[previous].Name, row[axes[previous].Name], axes[next].Name, row[axes[next].Name]))
	}
}

func firstUncoveredPair(uncovered map[string]struct{}, axes []Axis, next int) (int, string, string, bool) {
	for _, nextValue := range axes[next].Values {
		for previous := 0; previous < next; previous++ {
			for _, previousValue := range axes[previous].Values {
				key := pairKey(axes[previous].Name, previousValue, axes[next].Name, nextValue)
				if _, ok := uncovered[key]; ok {
					return previous, previousValue, nextValue, true
				}
			}
		}
	}
	return 0, "", "", false
}

func pairKey(leftName, leftValue, rightName, rightValue string) string {
	return leftName + "=" + leftValue + "\x00" + rightName + "=" + rightValue
}

func cloneCombination(in Combination) Combination {
	out := make(Combination, len(in))
	for key, value := range in {
		out[key] = value
	}
	return out
}

// randomCombinations samples mixed-radix Cartesian indexes directly. It never
// materializes the complete product just to return a small random subset.
func randomCombinations(axes []Axis, limit int) []Combination {
	axes = normalizeAxes(axes)
	if len(axes) == 0 || limit == 0 {
		return nil
	}
	if limit < 0 {
		return cartesian(axes, limit)
	}

	total := big.NewInt(1)
	for _, axis := range axes {
		total.Mul(total, big.NewInt(int64(len(axis.Values))))
	}
	if total.Cmp(big.NewInt(int64(limit))) <= 0 {
		return cartesian(axes, limit)
	}

	out := make([]Combination, 0, limit)
	used := make(map[string]struct{}, limit)
	for len(out) < limit {
		index, err := rand.Int(rand.Reader, total)
		if err != nil {
			return cartesian(axes, limit)
		}
		key := index.String()
		if _, exists := used[key]; exists {
			continue
		}
		used[key] = struct{}{}

		remaining := new(big.Int).Set(index)
		combo := make(Combination, len(axes))
		for axisIndex := len(axes) - 1; axisIndex >= 0; axisIndex-- {
			base := big.NewInt(int64(len(axes[axisIndex].Values)))
			remainder := new(big.Int)
			remaining.QuoRem(remaining, base, remainder)
			combo[axes[axisIndex].Name] = axes[axisIndex].Values[remainder.Int64()]
		}
		out = append(out, combo)
	}
	return out
}

func normalizeAxes(axes []Axis) []Axis {
	filtered := make([]Axis, 0, len(axes))
	for _, axis := range axes {
		values := uniqueNonEmpty(axis.Values)
		if axis.Name == "" || len(values) == 0 {
			continue
		}
		filtered = append(filtered, Axis{Name: axis.Name, Values: values})
	}
	return filtered
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
