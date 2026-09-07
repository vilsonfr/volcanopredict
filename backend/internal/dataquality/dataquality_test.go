package dataquality_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/dataquality"
)

// --- 5.1: no useful zero value -----------------------------------------

// The whole point of the zero value being unevaluated: a caller that
// forgets to evaluate must get an error, not a silent pass.
func TestZeroVerdict_IsRejected(t *testing.T) {
	var v dataquality.Verdict
	err := v.Validate()
	if err == nil {
		t.Fatal("a record with no quality verdict must be rejected, not treated as valid")
	}
	if !errors.Is(err, dataquality.ErrUnevaluated) {
		t.Fatalf("expected ErrUnevaluated, got %v", err)
	}
	if v.State != dataquality.StateUnevaluated {
		t.Fatalf("the zero state must be unevaluated, got %s", v.State)
	}
	if v.State.String() == "valid" {
		t.Fatal("the zero state must never render as valid")
	}
}

func TestNonValidVerdictWithoutRule_IsRejected(t *testing.T) {
	v := dataquality.Verdict{State: dataquality.StateSuspect}
	if err := v.Validate(); err == nil {
		t.Fatal("a suspect verdict with no rule naming why must be rejected: it is a mark nobody can act on")
	}
}

func TestStateRoundTrip(t *testing.T) {
	for _, s := range []dataquality.State{
		dataquality.StateValid, dataquality.StateSuspect, dataquality.StateDuplicate,
		dataquality.StateOutlier, dataquality.StateCorrupted, dataquality.StateDelayed,
	} {
		got, err := dataquality.ParseState(s.String())
		if err != nil {
			t.Fatalf("ParseState(%q): %v", s.String(), err)
		}
		if got != s {
			t.Fatalf("round trip changed %s into %s", s, got)
		}
	}
	if _, err := dataquality.ParseState("probably-fine"); err == nil {
		t.Fatal("an invented state must not parse")
	}
}

// --- 5.2: one case per named rule ---------------------------------------

func TestEvaluate_CleanRecordIsValid(t *testing.T) {
	now := time.Now().UTC()
	v := dataquality.Evaluate(dataquality.Observation{
		Key:        "ev1",
		OccurredAt: now.Add(-10 * time.Minute),
		SeenAt:     now,
		Numeric: map[string]dataquality.RangeCheck{
			"magnitude": {Value: 5.1, Min: -2, Max: 10.5},
		},
	}, dataquality.DefaultOptions(), map[string]bool{})

	if !v.Valid() {
		t.Fatalf("a clean record must be valid, got %s (%s)", v.State, v.Reason())
	}
	if v.Reason() != "" {
		t.Fatalf("a valid record carries no reason, got %q", v.Reason())
	}
	if err := v.Validate(); err != nil {
		t.Fatalf("a valid verdict must be persistable: %v", err)
	}
}

func TestEvaluate_ImpossibleValueNamesItsRule(t *testing.T) {
	now := time.Now().UTC()
	v := dataquality.Evaluate(dataquality.Observation{
		Key:        "ev1",
		OccurredAt: now.Add(-time.Minute),
		SeenAt:     now,
		Numeric: map[string]dataquality.RangeCheck{
			// No earthquake has magnitude 42.
			"magnitude": {Value: 42, Min: -2, Max: 10.5},
		},
	}, dataquality.DefaultOptions(), map[string]bool{})

	if v.State != dataquality.StateOutlier {
		t.Fatalf("expected outlier, got %s", v.State)
	}
	if !strings.Contains(v.Reason(), dataquality.RuleImpossibleValue) {
		t.Fatalf("the reason must name the rule that fired, got %q", v.Reason())
	}
	if err := v.Validate(); err != nil {
		t.Fatalf("an outlier verdict must still be persistable: %v", err)
	}
}

func TestEvaluate_FutureTimestampNamesItsRule(t *testing.T) {
	now := time.Now().UTC()
	v := dataquality.Evaluate(dataquality.Observation{
		Key:        "ev1",
		OccurredAt: now.Add(2 * time.Hour),
		SeenAt:     now,
	}, dataquality.DefaultOptions(), map[string]bool{})

	if v.Valid() {
		t.Fatal("an event that claims to have happened after we saw it must not be valid")
	}
	if !strings.Contains(v.Reason(), dataquality.RuleFutureTime) {
		t.Fatalf("the reason must name the timestamp rule, got %q", v.Reason())
	}
}

// Ordinary clock skew between the source and this system must not flag
// every record.
func TestEvaluate_SmallClockSkewIsTolerated(t *testing.T) {
	now := time.Now().UTC()
	v := dataquality.Evaluate(dataquality.Observation{
		Key:        "ev1",
		OccurredAt: now.Add(30 * time.Second),
		SeenAt:     now,
	}, dataquality.DefaultOptions(), map[string]bool{})

	if !v.Valid() {
		t.Fatalf("30s of clock skew must not mark a record, got %s (%s)", v.State, v.Reason())
	}
}

func TestEvaluate_DuplicateInSameResponseNamesItsRule(t *testing.T) {
	now := time.Now().UTC()
	seen := map[string]bool{}
	obs := dataquality.Observation{Key: "us7000tdrv", OccurredAt: now.Add(-time.Minute), SeenAt: now}

	first := dataquality.Evaluate(obs, dataquality.DefaultOptions(), seen)
	if !first.Valid() {
		t.Fatalf("the first occurrence is not a duplicate, got %s", first.State)
	}

	second := dataquality.Evaluate(obs, dataquality.DefaultOptions(), seen)
	if second.State != dataquality.StateDuplicate {
		t.Fatalf("expected the repeat to be marked duplicate, got %s", second.State)
	}
	if !strings.Contains(second.Reason(), dataquality.RuleDuplicate) {
		t.Fatalf("the reason must name the duplicate rule, got %q", second.Reason())
	}
}

func TestEvaluate_LateArrivalNamesItsRule(t *testing.T) {
	now := time.Now().UTC()
	v := dataquality.Evaluate(dataquality.Observation{
		Key:        "ev1",
		OccurredAt: now.Add(-72 * time.Hour),
		SeenAt:     now,
	}, dataquality.DefaultOptions(), map[string]bool{})

	if v.State != dataquality.StateDelayed {
		t.Fatalf("expected delayed, got %s", v.State)
	}
	if !strings.Contains(v.Reason(), dataquality.RuleLateArrival) {
		t.Fatalf("the reason must name the late-arrival rule, got %q", v.Reason())
	}
}

// A record failing several checks must be classified by the worst thing
// found, not by whichever check ran last.
func TestEvaluate_WorstStateWins(t *testing.T) {
	now := time.Now().UTC()
	v := dataquality.Evaluate(dataquality.Observation{
		Key:        "ev1",
		OccurredAt: now.Add(-72 * time.Hour), // delayed
		SeenAt:     now,
		Numeric: map[string]dataquality.RangeCheck{
			"magnitude": {Value: 99, Min: -2, Max: 10.5}, // outlier, worse
		},
	}, dataquality.DefaultOptions(), map[string]bool{})

	if v.State != dataquality.StateOutlier {
		t.Fatalf("expected the worst state (outlier) to win, got %s", v.State)
	}
	// Both rules must still be recorded: the classification is the worst
	// one, but the evidence is everything that fired.
	if !strings.Contains(v.Reason(), dataquality.RuleImpossibleValue) ||
		!strings.Contains(v.Reason(), dataquality.RuleLateArrival) {
		t.Fatalf("every rule that fired must appear in the reason, got %q", v.Reason())
	}
}
