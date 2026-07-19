package main

// Pure unit tests for validateSuggestions — the gate both the AI's output
// and the apply endpoint's client-submitted batch pass through. No network
// and no database (though the package's TestMain still requires Postgres
// to be up, matching the rest of the suite).

import (
	"errors"
	"testing"
)

func suggestions(n int) []BreakdownSuggestion {
	out := make([]BreakdownSuggestion, n)
	for i := range out {
		out[i] = BreakdownSuggestion{Title: "t", Priority: "low"}
	}
	return out
}

func TestValidateSuggestionsRejects(t *testing.T) {
	cases := map[string][]BreakdownSuggestion{
		"empty":              {},
		"more than 10":       suggestions(11),
		"blank title":        {{Title: " \t "}},
		"out of range index": {{Title: "a", DependsOn: []int{1}}},
		"negative index":     {{Title: "a", DependsOn: []int{-1}}},
		"self index":         {{Title: "a"}, {Title: "b", DependsOn: []int{1}}},
		"two-node cycle": {
			{Title: "a", DependsOn: []int{1}},
			{Title: "b", DependsOn: []int{0}},
		},
		"long cycle": {
			{Title: "a", DependsOn: []int{2}},
			{Title: "b", DependsOn: []int{0}},
			{Title: "c", DependsOn: []int{1}},
		},
	}
	for name, subs := range cases {
		if err := validateSuggestions(subs); !errors.Is(err, ErrInvalidBatch) {
			t.Errorf("%s: err = %v, want ErrInvalidBatch", name, err)
		}
	}
}

func TestValidateSuggestionsNormalizes(t *testing.T) {
	subs := []BreakdownSuggestion{
		{Title: "  padded  ", Priority: "URGENT"},
		{Title: "b", Priority: "high", DependsOn: []int{0, 0, 0}},
	}
	if err := validateSuggestions(subs); err != nil {
		t.Fatal(err)
	}
	if subs[0].Title != "padded" {
		t.Errorf("title = %q, want trimmed", subs[0].Title)
	}
	if subs[0].Priority != "medium" {
		t.Errorf("unknown priority normalized to %q, want medium", subs[0].Priority)
	}
	if subs[0].Tags == nil {
		t.Error("nil tags not normalized to empty slice")
	}
	if len(subs[1].DependsOn) != 1 {
		t.Errorf("dependsOn = %v, want deduped to one entry", subs[1].DependsOn)
	}
}

func TestValidateSuggestionsAcceptsDAG(t *testing.T) {
	subs := []BreakdownSuggestion{
		{Title: "schema"},
		{Title: "migrations", DependsOn: []int{0}},
		{Title: "handlers", DependsOn: []int{1}},
		{Title: "tests", DependsOn: []int{1, 2}},
	}
	if err := validateSuggestions(subs); err != nil {
		t.Errorf("valid DAG rejected: %v", err)
	}
}
