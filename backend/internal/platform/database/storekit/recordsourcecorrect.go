// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RecordSourceField names the corrected column in the audit images and in the
// event a module announces. recordSourceRunKey names the run in the evidence.
const (
	RecordSourceField   = "source"
	recordSourceRunKey  = "data_task_run"
	sourceNormalizedKey = "source_normalized"
)

// RecordSourceCorrectionSQL tests that the audit row aliased alias is one a
// source correction wrote. It moved no field a member typed, so a reader asking
// whether somebody worked on the record leaves it out.
func RecordSourceCorrectionSQL(alias string) string {
	return "((coalesce(" + alias + ".evidence, '{}'::jsonb) ? '" + recordSourceRunKey + "' OR coalesce(" + alias + ".evidence,'{}'::jsonb) ? '" + sourceNormalizedKey + "') AND " +
		alias + ".after - '" + RecordSourceField + "' - 'source_system' = '{}'::jsonb)"
}

// RecordSourceCorrection is one row a source correction moved, with the audit
// row that keeps the value it replaced.
type RecordSourceCorrection struct {
	ID, AuditID ids.UUID
	Source      string
}

// CorrectRecordSources runs one page of a source correction and audits each
// row it moved. page takes the cursor and the page size, locks the rows and
// returns each id with its old and new source. update takes the ids and the
// new sources as two arrays.
func CorrectRecordSources(
	ctx context.Context, tx pgx.Tx, entityType, page, update string, after ids.UUID, limit int, runID ids.UUID,
) ([]RecordSourceCorrection, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, page, after, limit)
	if err != nil {
		return nil, fmt.Errorf("store: reading %s rows with an unlisted source: %w", entityType, err)
	}
	type moved struct {
		ID       ids.UUID
		Old, New string
	}
	found, err := pgx.CollectRows(rows, pgx.RowToStructByPos[moved])
	if err != nil {
		return nil, fmt.Errorf("store: reading %s rows with an unlisted source: %w", entityType, err)
	}
	if len(found) == 0 {
		return nil, nil
	}
	rowIDs, sources := make([]ids.UUID, len(found)), make([]string, len(found))
	for i, row := range found {
		rowIDs[i], sources[i] = row.ID, row.New
	}
	if _, err := tx.Exec(ctx, update, rowIDs, sources); err != nil {
		return nil, fmt.Errorf("store: correcting the source of %s rows: %w", entityType, err)
	}

	prepared := make([]auditRow, 0, len(found))
	for _, row := range found {
		entry, err := prepareAuditRow(ctx, entityType, row.ID,
			map[string]any{RecordSourceField: row.Old}, map[string]any{RecordSourceField: row.New},
			map[string]any{recordSourceRunKey: runID.String()})
		if err != nil {
			return nil, err
		}
		prepared = append(prepared, entry)
	}
	written, err := insertAuditRows(ctx, tx, "update", entityType, prepared)
	if err != nil {
		return nil, err
	}
	corrected := make([]RecordSourceCorrection, 0, len(found))
	for index, row := range found {
		corrected = append(corrected, RecordSourceCorrection{ID: row.ID, AuditID: written[index], Source: row.New})
	}
	return corrected, nil
}
