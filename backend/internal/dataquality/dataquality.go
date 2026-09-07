// Package dataquality is the quality engine every ingested record passes
// through (qualidade-de-dados spec, §20 da master spec).
//
// Two rules shape everything here:
//
//   - A record that fails a check is MARKED and kept, never dropped. The
//     platform's value is in not asserting what it did not measure; that
//     applies to its own intake too. Only a record that cannot be
//     interpreted at all is refused, and even then it is counted.
//   - Quality is decided when the version is WRITTEN, with the rules in
//     force at that moment, and stored (design.md D5). Computing it at
//     read time would make the past change whenever a rule changed —
//     exactly what the bitemporal model exists to prevent.
package dataquality

import (
	"errors"
	"fmt"
	"strings"
)

// State is the quality verdict on one ingested record.
//
// There is deliberately no useful zero value: StateUnevaluated is what a
// caller gets for forgetting to evaluate, and persisting it is rejected.
// A zero value meaning "valid" would turn "nobody looked" into "this is
// fine" — the single most expensive default this package could have.
type State int

const (
	// StateUnevaluated is the zero value and is never persistable.
	StateUnevaluated State = iota
	StateValid
	StateSuspect
	StateDuplicate
	StateOutlier
	StateCorrupted
	StateDelayed
)

// ErrUnevaluated is returned when a record reaches persistence without a
// quality verdict.
var ErrUnevaluated = errors.New("dataquality: record has no quality verdict; evaluation is not optional")

// String renders the state as the database stores it. The strings are the
// vocabulary of §20 and are part of the schema's CHECK constraint, so they
// are not free to drift.
func (s State) String() string {
	switch s {
	case StateValid:
		return "valid"
	case StateSuspect:
		return "suspect"
	case StateDuplicate:
		return "duplicate"
	case StateOutlier:
		return "outlier"
	case StateCorrupted:
		return "corrupted"
	case StateDelayed:
		return "delayed"
	default:
		return "unevaluated"
	}
}

// ParseState maps a stored string back to a State.
func ParseState(s string) (State, error) {
	switch s {
	case "valid":
		return StateValid, nil
	case "suspect":
		return StateSuspect, nil
	case "duplicate":
		return StateDuplicate, nil
	case "outlier":
		return StateOutlier, nil
	case "corrupted":
		return StateCorrupted, nil
	case "delayed":
		return StateDelayed, nil
	default:
		return StateUnevaluated, fmt.Errorf("dataquality: unknown quality state %q", s)
	}
}

// Verdict is the outcome of evaluating one record: a state plus, when the
// state is not valid, the names of the rules that produced it.
//
// The reason is a list of rule names rather than prose so that "why is
// this suspect?" is answerable by query, and so a rule can be found by
// grep in both directions — from the stored row to the code and back.
type Verdict struct {
	State State
	// Rules are the stable names of the checks that fired, in the order
	// they were evaluated.
	Rules []string
}

// Reason renders the fired rules as the database stores them.
func (v Verdict) Reason() string {
	return strings.Join(v.Rules, ",")
}

// Valid reports whether the verdict is a clean pass.
func (v Verdict) Valid() bool { return v.State == StateValid }

// Validate rejects a verdict that must never reach the database: an
// unevaluated one, or a non-valid one with no rule naming why.
func (v Verdict) Validate() error {
	if v.State == StateUnevaluated {
		return ErrUnevaluated
	}
	if v.State != StateValid && len(v.Rules) == 0 {
		return fmt.Errorf("dataquality: state %s carries no rule name; a mark nobody can explain is worse than no mark", v.State)
	}
	return nil
}

// Rule names. These are stable identifiers, stored verbatim in
// quality_reason. Renaming one silently reclassifies history, so treat
// them the way you would treat a column name.
const (
	RuleImpossibleValue = "impossible_value"
	RuleInvalidTime     = "invalid_timestamp"
	RuleFutureTime      = "future_timestamp"
	RuleDuplicate       = "duplicate_in_response"
	RuleLateArrival     = "late_arrival"
)

// worst returns the more severe of two states, so a record failing
// several checks is classified by the worst thing found rather than by
// whichever check happened to run last.
func worst(a, b State) State {
	severity := map[State]int{
		StateValid:       0,
		StateDelayed:     1,
		StateDuplicate:   2,
		StateSuspect:     3,
		StateOutlier:     4,
		StateCorrupted:   5,
		StateUnevaluated: 6,
	}
	if severity[b] > severity[a] {
		return b
	}
	return a
}

// Builder accumulates rule outcomes into a Verdict.
//
// It starts at StateValid — but only a Builder that was actually run
// produces one, because callers obtain it through Evaluate rather than by
// declaring a zero value.
type Builder struct {
	state State
	rules []string
}

// NewBuilder starts an evaluation. The record is valid until a check says
// otherwise.
func NewBuilder() *Builder {
	return &Builder{state: StateValid}
}

// Fail records that a named rule fired, moving the verdict to at least
// the given state.
func (b *Builder) Fail(state State, rule string) *Builder {
	b.state = worst(b.state, state)
	b.rules = append(b.rules, rule)
	return b
}

// Verdict finishes the evaluation.
func (b *Builder) Verdict() Verdict {
	return Verdict{State: b.state, Rules: b.rules}
}
