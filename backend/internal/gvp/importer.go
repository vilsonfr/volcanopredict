package gvp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"

	"github.com/vilsonfr/volcanopredict/backend/internal/volcano"
)

// Rejection records a row that parsed structurally but was rejected by
// catalog business rules (currently: geographic range), per the
// catalogo-vulcoes spec: "esse registro é rejeitado, o erro é registrado
// em log identificando o vulcão, e a importação prossegue com os
// demais".
type Rejection struct {
	Line      int
	SourceRef string
	Name      string
	Err       error
}

// Report summarizes one importer run (spec: "relata quantos registros
// foram inseridos, atualizados, inalterados e rejeitados").
type Report struct {
	Inserted     int
	Updated      int
	Unchanged    int
	MarkedAbsent int
	Rejected     []Rejection
}

// Querier is what the importer needs from the database: it must support
// both volcano.Querier operations (via embedding-compatible interfaces),
// so callers typically pass a *pgxpool.Pool or a pgx.Tx.
type Querier = volcano.Querier

// Import reads a GVP Holocene Volcano List CSV from r and upserts it into
// the catalog under sourceID (the data_sources.id resolved by the caller
// via internal/source — this package deliberately does not look up the
// source itself, keeping "toda fonte externa é registrada antes de ser
// usada" enforced at the call site that already has to resolve
// source.GetEnabledByName).
//
// Rows that fail structural parsing abort the whole import (a malformed
// file is exactly the case design.md's Risks section calls out: "falha
// explicitamente diante de estrutura inesperada, em vez de importar
// registros parciais"). Rows that parse fine but carry an invalid
// coordinate are rejected individually and logged, and the import
// continues with the rest (catalogo-vulcoes spec, "Coordenada inválida na
// fonte").
//
// Volcanoes previously imported from sourceID that do not appear in this
// run are marked absent_from_source_at, never deleted.
func Import(ctx context.Context, q Querier, sourceID int64, r io.Reader) (Report, error) {
	rows, parseRowErrs, err := Parse(r)
	if err != nil {
		return Report{}, fmt.Errorf("gvp: import aborted, source snapshot is malformed: %w", err)
	}

	var report Report
	for _, rowErr := range parseRowErrs {
		log.Printf("gvp: import: rejected row %d: %v", rowErr.Line, rowErr.Err)
		report.Rejected = append(report.Rejected, Rejection{Line: rowErr.Line, Err: rowErr.Err})
	}

	seenRefs := make([]string, 0, len(rows))
	for _, row := range rows {
		rec := volcano.Record{
			SourceID:   sourceID,
			SourceRef:  row.SourceRef,
			Name:       row.Name,
			Country:    row.Country,
			Latitude:   row.Latitude,
			Longitude:  row.Longitude,
			ElevationM: row.ElevationM,
			Status:     row.Status,
		}

		_, outcome, err := volcano.Upsert(ctx, q, rec)
		if err != nil {
			if errors.Is(err, volcano.ErrInvalidCoordinate) || errors.Is(err, volcano.ErrMissingAttribution) {
				log.Printf("gvp: import: rejected volcano source_ref=%s name=%q (line %d): %v", row.SourceRef, row.Name, row.Line, err)
				report.Rejected = append(report.Rejected, Rejection{Line: row.Line, SourceRef: row.SourceRef, Name: row.Name, Err: err})
				continue
			}
			return Report{}, fmt.Errorf("gvp: import failed on source_ref=%s (line %d): %w", row.SourceRef, row.Line, err)
		}

		seenRefs = append(seenRefs, row.SourceRef)
		switch outcome {
		case volcano.OutcomeInserted:
			report.Inserted++
		case volcano.OutcomeUpdated:
			report.Updated++
		case volcano.OutcomeUnchanged:
			report.Unchanged++
		}
	}

	marked, err := volcano.MarkAbsent(ctx, q, sourceID, seenRefs)
	if err != nil {
		return Report{}, fmt.Errorf("gvp: failed to mark absent volcanoes: %w", err)
	}
	report.MarkedAbsent = int(marked)

	log.Printf("gvp: import complete: inserted=%d updated=%d unchanged=%d marked_absent=%d rejected=%d",
		report.Inserted, report.Updated, report.Unchanged, report.MarkedAbsent, len(report.Rejected))

	return report, nil
}
