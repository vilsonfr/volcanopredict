package dataquality

import (
	"time"
)

// Observation is what the checks below need to know about a record, kept
// deliberately narrow so this package does not depend on any particular
// source's shape.
type Observation struct {
	// Key identifies the record within one response, for duplicate
	// detection.
	Key string
	// OccurredAt is when the phenomenon happened, per the source.
	OccurredAt time.Time
	// SeenAt is when we are looking at it — the collection instant, not
	// the process clock at some later moment.
	SeenAt time.Time

	// Numeric is the set of measured values to range-check, keyed by a
	// name that appears in no rule output but helps a reader of this
	// call site. Absent values are simply not present in the map.
	Numeric map[string]RangeCheck
}

// RangeCheck is one measured value together with the range that is
// physically possible for it. A value outside the range is not a
// judgement about the science — it is a statement that the number cannot
// be what it claims to be.
type RangeCheck struct {
	Value float64
	Min   float64
	Max   float64
}

// Options tunes the checks that need a threshold.
type Options struct {
	// LateAfter is how far behind the occurrence a record may arrive
	// before it is marked delayed. It comes from the source's collection
	// cadence: "late" only means something relative to how often we look.
	LateAfter time.Duration
	// FutureTolerance absorbs ordinary clock skew between the source and
	// this system before an occurrence instant counts as impossible.
	FutureTolerance time.Duration
}

// DefaultOptions are conservative: a record has to be a full day late, or
// more than five minutes in the future, before it is marked.
func DefaultOptions() Options {
	return Options{
		LateAfter:       24 * time.Hour,
		FutureTolerance: 5 * time.Minute,
	}
}

// Evaluate runs every check against one record and returns the verdict.
//
// seenKeys is the set of record keys already encountered in this same
// response; Evaluate adds to it. Passing nil disables duplicate detection,
// which is only correct for a caller that has no notion of a batch.
func Evaluate(obs Observation, opts Options, seenKeys map[string]bool) Verdict {
	b := NewBuilder()

	// A phenomenon cannot happen after we learned of it. A small
	// tolerance absorbs clock skew; beyond that, the timestamp is not
	// describing reality.
	if !obs.SeenAt.IsZero() && obs.OccurredAt.After(obs.SeenAt.Add(opts.FutureTolerance)) {
		b.Fail(StateSuspect, RuleFutureTime)
	}

	// A zero instant is not a time; it is a field nobody filled in.
	if obs.OccurredAt.IsZero() {
		b.Fail(StateSuspect, RuleInvalidTime)
	}

	// Values outside what is physically possible.
	for _, rc := range obs.Numeric {
		if rc.Value < rc.Min || rc.Value > rc.Max {
			b.Fail(StateOutlier, RuleImpossibleValue)
			break
		}
	}

	// Arrived long after it happened. Not wrong — but a consumer
	// computing a rate over a window that has since closed needs to know
	// this record showed up afterwards.
	if opts.LateAfter > 0 && !obs.SeenAt.IsZero() && !obs.OccurredAt.IsZero() {
		if obs.SeenAt.Sub(obs.OccurredAt) > opts.LateAfter {
			b.Fail(StateDelayed, RuleLateArrival)
		}
	}

	// The same record twice in one response. The second occurrence is not
	// a new version of anything — the source just repeated itself.
	if seenKeys != nil && obs.Key != "" {
		if seenKeys[obs.Key] {
			b.Fail(StateDuplicate, RuleDuplicate)
		} else {
			seenKeys[obs.Key] = true
		}
	}

	return b.Verdict()
}
