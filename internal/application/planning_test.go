package application

import (
	"strconv"
	"strings"
	"testing"

	"api-automation/internal/capabilities"
	"api-automation/internal/combinations"
)

func TestBuildPlanCustomPageRangeReplacesSelectedEnums(t *testing.T) {
	request := PlanRequest{
		Capabilities: capabilities.Model{Options: []capabilities.Option{
			{ID: PageRangeOptionID, Label: "Page range", Value: "All", Values: []string{"All", "Odd", "Even", PageRangeRangeValue}},
			{ID: CopiesOptionID, Label: "Copies", Value: "1"},
		}},
		SelectedValues:  map[string][]string{PageRangeOptionID: {"Odd", PageRangeRangeValue}},
		CopiesInput:     "1",
		CustomPageRange: "1,3,5-7",
		ValueSource:     ValueSourceSelected,
		Strategy:        combinations.StrategySelected,
		MaxCases:        100,
	}
	plan, err := BuildPlan(request)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Combinations) != 1 {
		t.Fatalf("combinations = %#v", plan.Combinations)
	}
	planned := plan.Combinations[0][PageRangeOptionID]
	if planned != PageRangeInternalPrefix+"1,3,5-7" {
		t.Fatalf("planned page range = %q", planned)
	}
	attributes := CombinationToAttributes(plan.Combinations[0])
	if attributes[PageRangeOptionID] != "1,3,5-7" {
		t.Fatalf("wire attributes = %#v", attributes)
	}
	if _, exists := attributes[PageRangeLegacyDataID]; exists {
		t.Fatalf("legacy companion was synthesized: %#v", attributes)
	}
	if got := request.SelectedValues[PageRangeOptionID]; len(got) != 2 || got[0] != "Odd" {
		t.Fatalf("BuildPlan mutated request selections: %#v", got)
	}
}

func TestBuildPlanCustomPageRangeRequiresRangeCapableSchema(t *testing.T) {
	_, err := BuildPlan(PlanRequest{
		Capabilities: capabilities.Model{Options: []capabilities.Option{
			{ID: PageRangeOptionID, Values: []string{"All", "Odd", "Even"}},
			{ID: CopiesOptionID, Value: "1"},
		}},
		CopiesInput:     "1",
		CustomPageRange: "1-5",
		Strategy:        combinations.StrategySelected,
	})
	if err == nil || !strings.Contains(err.Error(), PageRangeOptionID) {
		t.Fatalf("unsupported custom page range error = %v", err)
	}
}

func TestBuildPlanNumericInputExpandsRange(t *testing.T) {
	plan, err := BuildPlan(PlanRequest{
		Capabilities: capabilities.Model{Options: []capabilities.Option{{
			ID: "EFMediaThickness", Label: "Media thickness", Value: "1",
			Range: &capabilities.NumericRange{Min: 1, Max: 10, Increment: 1},
		}}},
		NumericInputs: map[string]string{"EFMediaThickness": "2-4"},
		Strategy:      combinations.StrategySelected,
		MaxCases:      100,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Combinations) != 3 || len(plan.Axes) != 1 {
		t.Fatalf("combinations=%#v axes=%#v", plan.Combinations, plan.Axes)
	}
	seen := map[string]bool{}
	for _, combination := range plan.Combinations {
		seen[combination["EFMediaThickness"]] = true
	}
	for _, value := range []string{"2", "3", "4"} {
		if !seen[value] {
			t.Fatalf("missing numeric value %q in %#v", value, plan.Combinations)
		}
	}
}

func TestBuildPlanValueSourcesAreIndependentFromStrategy(t *testing.T) {
	base := PlanRequest{
		Capabilities: capabilities.Model{Options: []capabilities.Option{
			{ID: "EFColorMode", Value: "Grayscale", Values: []string{"CMYK", "Grayscale"}},
			{ID: CopiesOptionID, Value: "1"},
		}},
		SelectedValues: map[string][]string{"EFColorMode": {"CMYK"}},
		CopiesInput:    "1",
		Strategy:       combinations.StrategyAll,
		MaxCases:       100,
	}

	base.ValueSource = ValueSourceSelected
	selected := mustBuildPlan(t, base)
	if len(selected.Combinations) != 1 || selected.Combinations[0]["EFColorMode"] != "CMYK" {
		t.Fatalf("selected plan = %#v", selected.Combinations)
	}
	base.ValueSource = ValueSourceDefaults
	defaults := mustBuildPlan(t, base)
	if len(defaults.Combinations) != 1 || defaults.Combinations[0]["EFColorMode"] != "Grayscale" {
		t.Fatalf("defaults plan = %#v", defaults.Combinations)
	}
	base.ValueSource = ValueSourceAdvertised
	advertised := mustBuildPlan(t, base)
	if len(advertised.Combinations) != 2 {
		t.Fatalf("advertised plan = %#v", advertised.Combinations)
	}
	base.ValueSource = ValueSourceBaseline
	baseline := mustBuildPlan(t, base)
	if len(baseline.Combinations) != 1 || len(baseline.Combinations[0]) != 0 || len(baseline.Axes) != 0 {
		t.Fatalf("baseline plan = %#v axes=%#v", baseline.Combinations, baseline.Axes)
	}
}

func TestBuildPlanFiltersPublishedConstraintsByIntent(t *testing.T) {
	base := PlanRequest{
		Capabilities: capabilities.Model{Options: []capabilities.Option{
			{ID: "EFResolution", Values: []string{"360x720dpi"}, Constraints: capabilities.Constraints{"360x720dpi": {"EFEdgeDropSize": {"0_1_2_2_2"}}}},
			{ID: "EFEdgeDropSize", Values: []string{"None", "0_1_2_2_2"}},
		}},
		SelectedValues: map[string][]string{
			"EFResolution":   {"360x720dpi"},
			"EFEdgeDropSize": {"None", "0_1_2_2_2"},
		},
		ValueSource: ValueSourceSelected,
		Strategy:    combinations.StrategyAll,
		MaxCases:    100,
	}

	positive := mustBuildPlan(t, base)
	if len(positive.Combinations) != 1 || positive.Combinations[0]["EFEdgeDropSize"] != "None" || positive.ConstraintSkipped != 1 {
		t.Fatalf("positive plan = %#v skipped=%d", positive.Combinations, positive.ConstraintSkipped)
	}
	base.TestIntent = TestIntentConstraint
	constraint := mustBuildPlan(t, base)
	if len(constraint.Combinations) != 1 || constraint.Combinations[0]["EFEdgeDropSize"] != "0_1_2_2_2" || constraint.ConstraintWarning == "" {
		t.Fatalf("constraint plan = %#v warning=%q", constraint.Combinations, constraint.ConstraintWarning)
	}
}

func TestBuildPlanCopiesFeedEveryGenerationStrategy(t *testing.T) {
	for _, strategy := range []combinations.Strategy{combinations.StrategySelected, combinations.StrategyAll, combinations.StrategyPairwise} {
		t.Run(string(strategy), func(t *testing.T) {
			plan := mustBuildPlan(t, PlanRequest{
				Capabilities: capabilities.Model{Options: []capabilities.Option{{ID: CopiesOptionID, Label: "Copies", Value: "1"}}},
				CopiesInput:  "1,5,10,15",
				Strategy:     strategy,
				MaxCases:     100,
			})
			if len(plan.Combinations) != 4 || len(plan.Axes) != 1 {
				t.Fatalf("combinations=%#v axes=%#v", plan.Combinations, plan.Axes)
			}
			seen := map[string]bool{}
			for _, combination := range plan.Combinations {
				seen[combination[CopiesOptionID]] = true
			}
			for _, value := range []string{"1", "5", "10", "15"} {
				if !seen[value] {
					t.Fatalf("missing copies value %s in %#v", value, plan.Combinations)
				}
			}
		})
	}
}

func TestBuildPlanCopiesRangeIsBoundedWithoutChangingRequest(t *testing.T) {
	request := PlanRequest{
		Capabilities: capabilities.Model{Options: []capabilities.Option{{ID: CopiesOptionID, Value: "1"}}},
		CopiesInput:  "1-1000",
		Strategy:     combinations.StrategySelected,
		MaxCases:     100,
	}
	plan := mustBuildPlan(t, request)
	if len(plan.Combinations) != 100 {
		t.Fatalf("combination count = %d", len(plan.Combinations))
	}
	if request.MaxCases != 100 || request.CopiesInput != "1-1000" {
		t.Fatalf("request mutated: %#v", request)
	}
	for _, combination := range plan.Combinations {
		value, err := strconv.Atoi(combination[CopiesOptionID])
		if err != nil || value < 1 || value > 1000 {
			t.Fatalf("invalid sampled copies value: %#v", combination)
		}
	}
}

func TestBuildPlanConstraintBaselineIsRejected(t *testing.T) {
	_, err := BuildPlan(PlanRequest{ValueSource: ValueSourceBaseline, TestIntent: TestIntentConstraint})
	if err == nil {
		t.Fatal("constraint baseline unexpectedly accepted")
	}
}

func mustBuildPlan(t *testing.T, request PlanRequest) Plan {
	t.Helper()
	plan, err := BuildPlan(request)
	if err != nil {
		t.Fatal(err)
	}
	return plan
}
