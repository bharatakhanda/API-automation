package combinations

import (
	"crypto/rand"
	"math/big"
	"sort"
)

type Strategy string

const (
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
	case StrategyPairwise:
		return pairwise(axes, limit)
	case StrategyRandom:
		return randomSample(cartesian(axes, -1), limit)
	default:
		return cartesian(axes, limit)
	}
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

// pairwise creates a deterministic greedy pairwise sample. It covers every
// value pair across every two axes when the supplied limit is large enough.
func pairwise(axes []Axis, limit int) []Combination {
	axes = normalizeAxes(axes)
	if len(axes) == 0 || limit == 0 {
		return nil
	}
	if len(axes) == 1 {
		return cartesian(axes, limit)
	}
	uncovered := allPairs(axes)
	if len(uncovered) == 0 {
		return nil
	}

	candidates := cartesian(axes, -1)
	out := make([]Combination, 0)
	used := map[int]struct{}{}
	for len(uncovered) > 0 && (limit < 0 || len(out) < limit) {
		bestIdx := -1
		bestCovered := 0
		var bestPairs []string
		for idx, candidate := range candidates {
			if _, ok := used[idx]; ok {
				continue
			}
			pairs := coveredPairs(candidate, axes, uncovered)
			if len(pairs) > bestCovered {
				bestIdx = idx
				bestCovered = len(pairs)
				bestPairs = pairs
			}
		}
		if bestIdx < 0 || bestCovered == 0 {
			break
		}
		used[bestIdx] = struct{}{}
		out = append(out, cloneCombination(candidates[bestIdx]))
		for _, pair := range bestPairs {
			delete(uncovered, pair)
		}
	}
	return out
}

func allPairs(axes []Axis) map[string]struct{} {
	pairs := map[string]struct{}{}
	for i := 0; i < len(axes); i++ {
		for j := i + 1; j < len(axes); j++ {
			for _, left := range axes[i].Values {
				for _, right := range axes[j].Values {
					pairs[pairKey(axes[i].Name, left, axes[j].Name, right)] = struct{}{}
				}
			}
		}
	}
	return pairs
}

func coveredPairs(combo Combination, axes []Axis, uncovered map[string]struct{}) []string {
	pairs := make([]string, 0)
	for i := 0; i < len(axes); i++ {
		for j := i + 1; j < len(axes); j++ {
			key := pairKey(axes[i].Name, combo[axes[i].Name], axes[j].Name, combo[axes[j].Name])
			if _, ok := uncovered[key]; ok {
				pairs = append(pairs, key)
			}
		}
	}
	return pairs
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

func randomSample(all []Combination, limit int) []Combination {
	if limit <= 0 || limit >= len(all) {
		return all
	}
	out := make([]Combination, 0, limit)
	used := map[int]struct{}{}
	for len(out) < limit {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(all))))
		if err != nil {
			return all[:limit]
		}
		idx := int(n.Int64())
		if _, ok := used[idx]; ok {
			continue
		}
		used[idx] = struct{}{}
		out = append(out, all[idx])
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
