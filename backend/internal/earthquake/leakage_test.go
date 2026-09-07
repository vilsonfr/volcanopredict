package earthquake_test

import (
	"context"
	"testing"
	"time"

	"github.com/vilsonfr/volcanopredict/backend/internal/dbtest"
	"github.com/vilsonfr/volcanopredict/backend/internal/earthquake"
)

// TestDataLeakage_AsOfNeverSeesFutureKnowledge is the executable form of
// the guarantee that makes a back-test meaningful: an as-of query must
// never return a row whose ingested_at is later than the instant asked
// about.
//
// The case that breaks a naive implementation is the LATE-ARRIVING event:
// a phenomenon that happened before another one, but that the system only
// learned about afterwards. Filtering on occurred_at alone looks correct
// until exactly this shape appears.
//
// Sabotage verification: deleting the `ingested_at <= $1` predicate from
// earthquake.List — a change that compiles — turns this test red at the
// checkpoint assertions below. Redo that sabotage after touching anything
// on this path: a temporal test that still passes with the guarantee
// broken is worse than no test.
func TestDataLeakage_AsOfNeverSeesFutureKnowledge(t *testing.T) {
	tdb := dbtest.Start(t)
	pool := mustPool(t, tdb.ConnString)
	srcID := mustSource(t, pool, "eq-data-leakage")
	ctx := context.Background()

	base := time.Now().UTC().Add(-100 * time.Hour)

	save := func(id string, occurred time.Time) earthquake.Earthquake {
		t.Helper()
		e, _ := mustSave(t, pool, earthquake.ForTest(id, srcID, occurred, -6.1, 105.4))
		return e
	}

	save("eq-1", base)
	tick()
	checkpoint1 := time.Now().UTC()
	tick()

	save("eq-2", base.Add(10*time.Hour))
	tick()
	checkpoint2 := time.Now().UTC()
	tick()

	// The late arrival: it happened at base+2h, BEFORE eq-2, but the
	// system only learns of it now, after checkpoint2.
	lateArriving := save("eq-late", base.Add(2*time.Hour))
	tick()
	checkpoint3 := time.Now().UTC()
	tick()

	save("eq-3", base.Add(20*time.Hour))
	tick()
	checkpoint4 := time.Now().UTC()
	tick()

	// A revision of an already-known event is also future knowledge
	// relative to earlier checkpoints.
	revised := earthquake.ForTest("eq-1", srcID, base, -6.1, 105.4)
	revised.Magnitude = f64(6.4)
	mustSave(t, pool, revised)
	tick()
	checkpoint5 := time.Now().UTC()

	checkpoints := []time.Time{checkpoint1, checkpoint2, checkpoint3, checkpoint4, checkpoint5}
	for i, asOf := range checkpoints {
		asOf := asOf
		results, err := earthquake.List(ctx, pool, earthquake.Query{
			SourceID:   &srcID,
			AsOf:       &asOf,
			Provenance: earthquake.ProvenanceAll,
		})
		if err != nil {
			t.Fatalf("List at checkpoint %d (%s) failed: %v", i+1, asOf, err)
		}
		for _, r := range results {
			if r.IngestedAt.After(asOf) {
				t.Fatalf("DATA LEAKAGE: as-of query for %s returned earthquake id=%d (%s) with ingested_at=%s, which is knowledge from the future relative to the query instant",
					asOf, r.ID, r.ExternalID, r.IngestedAt)
			}
		}
	}

	// Without this half, the assertion above could pass vacuously by
	// returning nothing at all. The late arrival must be absent before its
	// own ingested_at and present after it.
	before, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID: &srcID, AsOf: &checkpoint2, Provenance: earthquake.ProvenanceAll,
	})
	if err != nil {
		t.Fatalf("List at checkpoint2: %v", err)
	}
	for _, r := range before {
		if r.ID == lateArriving.ID {
			t.Fatal("the late-arriving earthquake must be invisible before its own ingested_at")
		}
	}
	if len(before) != 2 {
		t.Fatalf("expected the 2 events known at checkpoint2, got %d", len(before))
	}

	after, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID: &srcID, AsOf: &checkpoint3, Provenance: earthquake.ProvenanceAll,
	})
	if err != nil {
		t.Fatalf("List at checkpoint3: %v", err)
	}
	var found bool
	for _, r := range after {
		if r.ID == lateArriving.ID {
			found = true
		}
	}
	if !found {
		t.Fatal("the late-arriving earthquake must be visible from its own ingested_at onward")
	}

	// And the revision must not leak backwards: at checkpoint4, eq-1 was
	// still the pre-revision version.
	atFour, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID: &srcID, AsOf: &checkpoint4, Provenance: earthquake.ProvenanceAll,
	})
	if err != nil {
		t.Fatalf("List at checkpoint4: %v", err)
	}
	for _, r := range atFour {
		if r.ExternalID == "eq-1" && r.Magnitude != nil {
			t.Fatalf("as of checkpoint4 the revision had not happened yet, but the query returned magnitude %v", *r.Magnitude)
		}
	}

	atFive, err := earthquake.List(ctx, pool, earthquake.Query{
		SourceID: &srcID, AsOf: &checkpoint5, Provenance: earthquake.ProvenanceAll,
	})
	if err != nil {
		t.Fatalf("List at checkpoint5: %v", err)
	}
	var sawRevision bool
	for _, r := range atFive {
		if r.ExternalID == "eq-1" && r.Magnitude != nil && *r.Magnitude == 6.4 {
			sawRevision = true
		}
	}
	if !sawRevision {
		t.Fatal("as of checkpoint5 the revision had happened and must be visible")
	}
}
