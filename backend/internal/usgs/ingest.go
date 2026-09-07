package usgs

import (
	"context"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/dataquality"
	"github.com/vilsonfr/volcanopredict/backend/internal/earthquake"
	"github.com/vilsonfr/volcanopredict/backend/internal/ingestion"
)

// SourceName is the exact data_sources row this adapter ingests under. It
// must already be registered and enabled; the adapter never creates it.
const SourceName = "USGS Earthquake Hazards Program"

// SafetyOverlap is how far back the incremental anchor reaches before the
// end of the last successful run.
//
// An exact anchor loses any event whose `updated` lands between the
// service answering our query and our transaction committing. Re-ingesting
// an event we already know is free — deduplication by content drops it —
// while losing one is silent and permanent. The asymmetry is the whole
// argument.
const SafetyOverlap = 15 * time.Minute

// Physical bounds used by the quality engine. They are not scientific
// judgements: they are statements that a number outside them cannot be
// describing an earthquake on this planet.
const (
	minPlausibleMagnitude = -3.0
	maxPlausibleMagnitude = 10.5
	minPlausibleDepthKm   = -12.0
	maxPlausibleDepthKm   = 800.0
)

// Store is what the ingester needs from the database.
type Store interface {
	earthquake.Querier
	ingestion.Querier
}

// Ingester runs the fetch → parse → validate → normalize → persist chain
// for one source.
type Ingester struct {
	Client   *Client
	DB       Store
	SourceID int64

	// Quality tunes the checks. Zero value uses DefaultOptions.
	Quality dataquality.Options

	// PageSize bounds each request; zero means the service ceiling.
	PageSize int

	// now is time.Now in production, overridable in tests.
	now func() time.Time
}

// NewIngester builds an Ingester with sane defaults.
func NewIngester(client *Client, db Store, sourceID int64) *Ingester {
	return &Ingester{
		Client:   client,
		DB:       db,
		SourceID: sourceID,
		Quality:  dataquality.DefaultOptions(),
		now:      func() time.Time { return time.Now().UTC() },
	}
}

// Report summarizes one ingestion.
type Report struct {
	Run    ingestion.Run
	Counts ingestion.Counts
	// SourceVersion is what the service called itself during this run.
	SourceVersion string
}

// IngestWindow ingests every event that OCCURRED in [start, end].
//
// This is the backfill shape: it asks about the phenomenon's own timeline.
func (i *Ingester) IngestWindow(ctx context.Context, mode ingestion.Mode, start, end time.Time) (Report, error) {
	return i.ingest(ctx, mode, Query{Start: start, End: end}, start, end)
}

// qualityOptionsFor adapts the checks to how the data was asked for.
//
// The late-arrival check is meaningful only for incremental collection,
// where "this should have reached us by now" is a real statement about the
// pipeline. In a backfill the operator deliberately asked for old events,
// and every one of them is trivially older than any cadence — running the
// check there marked all 642 records of the first real backfill as
// `delayed`, which made the field say nothing and hid every record from a
// consumer filtering for valid data.
//
// Lateness is a property of how we collected, not of the event.
func (i *Ingester) qualityOptionsFor(mode ingestion.Mode) dataquality.Options {
	opts := i.Quality
	if opts == (dataquality.Options{}) {
		opts = dataquality.DefaultOptions()
	}
	if mode != ingestion.ModeIncremental {
		opts.LateAfter = 0
	}
	return opts
}

// IngestIncremental ingests everything the source has CHANGED since the
// last successful run, minus a safety overlap.
//
// This is the shape that surfaces revisions, and the reason FDSN was
// chosen over the real-time feeds: an event corrected weeks after it
// happened comes back through updatedafter and through nothing else.
//
// With no previous successful run, it falls back to a window ending now
// and starting `fallback` earlier, so a first cycle on a fresh database
// does something useful instead of asking for everything since the epoch.
func (i *Ingester) IngestIncremental(ctx context.Context, fallback time.Duration) (Report, error) {
	now := i.clock()

	anchor := now.Add(-fallback)
	last, err := ingestion.LastSuccess(ctx, i.DB, i.SourceID)
	switch {
	case err == nil:
		anchor = last.WindowEnd.Add(-SafetyOverlap)
		// A window recorded with an end in the future — which a manual run
		// with a bad -end can produce — would push the anchor past now and
		// make every later cycle fail at Begin with an inverted window, and
		// fail before there is any run row to explain why. Clamping keeps
		// the loop recoverable.
		if anchor.After(now) {
			log.Printf("usgs: last successful run ends at %s, in the future; clamping the incremental anchor to now",
				last.WindowEnd.Format(time.RFC3339))
			anchor = now
		}
	case errors.Is(err, ingestion.ErrNoRuns):
		log.Printf("usgs: no previous successful run; first incremental cycle covers the last %s", fallback)
	default:
		return Report{}, err
	}

	return i.ingest(ctx, ingestion.ModeIncremental, Query{UpdatedAfter: anchor}, anchor, now)
}

func (i *Ingester) clock() time.Time {
	if i.now != nil {
		return i.now().UTC()
	}
	return time.Now().UTC()
}

// ingest is the whole chain, with the run recorded whatever happens.
func (i *Ingester) ingest(ctx context.Context, mode ingestion.Mode, q Query, windowStart, windowEnd time.Time) (Report, error) {
	run, err := ingestion.Begin(ctx, i.DB, i.SourceID, mode, windowStart, windowEnd)
	if err != nil {
		return Report{}, err
	}

	report, ingestErr := i.collect(ctx, q, i.qualityOptionsFor(mode))
	if ingestErr != nil {
		// The run is closed as failed with whatever it managed before
		// breaking. A failure that left no trace would be
		// indistinguishable from a quiet period (§74).
		failed, err := ingestion.Fail(ctx, i.DB, run.ID, ingestErr, report.Counts)
		if err != nil {
			return Report{}, fmt.Errorf("usgs: ingestion failed (%v) and recording the failure also failed: %w", ingestErr, err)
		}
		report.Run = failed
		return report, ingestErr
	}

	done, err := ingestion.Finish(ctx, i.DB, run.ID, report.Counts)
	if err != nil {
		return Report{}, err
	}
	report.Run = done

	log.Printf("usgs: ingestion complete: mode=%s window=%s..%s inserted=%d updated=%d unchanged=%d rejected=%d",
		mode, windowStart.Format(time.RFC3339), windowEnd.Format(time.RFC3339),
		report.Counts.Inserted, report.Counts.Updated, report.Counts.Unchanged, report.Counts.Rejected)

	return report, nil
}

func (i *Ingester) collect(ctx context.Context, q Query, opts dataquality.Options) (Report, error) {
	var report Report

	// Duplicate detection is scoped to one ingestion: the same event
	// appearing twice across two runs is a revision question, not a
	// duplicate one.
	seen := map[string]bool{}
	seenAt := i.clock()

	err := i.Client.FetchPages(ctx, q, i.PageSize, func(body []byte, delivered int) error {
		resp, recErrs, err := Parse(body)
		if err != nil {
			return err
		}
		if resp.SourceVersion != "" {
			report.SourceVersion = resp.SourceVersion
		}

		for _, re := range recErrs {
			// A record that could not be extracted is counted and logged,
			// never silently dropped.
			log.Printf("usgs: rejected record: %v", re)
			report.Counts.Rejected++
		}

		for _, ev := range resp.Events {
			outcome, err := i.persist(ctx, ev, resp.SourceVersion, opts, seen, seenAt)
			if err != nil {
				// A record rejected by business rules does not abort the
				// batch (ingestao-fontes spec).
				if errors.Is(err, earthquake.ErrInvalidCoordinate) {
					log.Printf("usgs: rejected event %s: %v", ev.ID, err)
					report.Counts.Rejected++
					continue
				}
				return fmt.Errorf("usgs: persisting event %s failed: %w", ev.ID, err)
			}
			switch outcome {
			case earthquake.OutcomeInserted:
				report.Counts.Inserted++
			case earthquake.OutcomeUpdated:
				report.Counts.Updated++
			case earthquake.OutcomeUnchanged:
				report.Counts.Unchanged++
			}
		}
		return nil
	})

	return report, err
}

func (i *Ingester) persist(
	ctx context.Context,
	ev Event,
	sourceVersion string,
	opts dataquality.Options,
	seen map[string]bool,
	seenAt time.Time,
) (earthquake.Outcome, error) {
	numeric := map[string]dataquality.RangeCheck{}
	if ev.Magnitude != nil {
		numeric["magnitude"] = dataquality.RangeCheck{
			Value: *ev.Magnitude, Min: minPlausibleMagnitude, Max: maxPlausibleMagnitude,
		}
	}
	if ev.DepthKm != nil {
		numeric["depth_km"] = dataquality.RangeCheck{
			Value: *ev.DepthKm, Min: minPlausibleDepthKm, Max: maxPlausibleDepthKm,
		}
	}

	verdict := dataquality.Evaluate(dataquality.Observation{
		Key:        ev.ID,
		OccurredAt: ev.Time,
		SeenAt:     seenAt,
		Numeric:    numeric,
	}, opts, seen)

	n := earthquake.New{
		ExternalID:     ev.ID,
		SourceID:       i.SourceID,
		OccurredAt:     ev.Time,
		Latitude:       ev.Latitude,
		Longitude:      ev.Longitude,
		DepthKm:        ev.DepthKm,
		Magnitude:      ev.Magnitude,
		MagnitudeType:  ev.MagnitudeType,
		SourceStatus:   ev.Status,
		RMS:            ev.RMS,
		AzimuthalGap:   ev.AzimuthalGap,
		StationCount:   ev.StationCount,
		MinDistanceDeg: ev.MinDistanceDeg,
		// Everything this adapter writes is real. Synthetic data has no
		// path into this table through here.
		IsSynthetic:   false,
		Quality:       verdict,
		ParserVersion: ParserVersion,
		SourceVersion: sourceVersion,
		Raw:           ev.Raw,
	}
	if !ev.SourceUpdatedAt.IsZero() {
		u := ev.SourceUpdatedAt
		n.SourceUpdatedAt = &u
	}

	_, outcome, err := earthquake.Save(ctx, i.DB, n)
	return outcome, err
}
