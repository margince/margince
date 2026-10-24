// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/pkg/extension"
)

// IngestBatch lands the records one at a time through Ingest, then saves the
// position. The save is a separate commit after the last record is answered, so
// a stop in the middle leaves the old position and the next run offers the
// batch again; the natural key makes that replay a no-op.
//
// A refusal ends the batch where it happened. The results answered so far come
// back with the error and the position stays where it was.
func (r *callRuntime) IngestBatch(ctx context.Context, on extension.UserID, system extension.SourceName, recs []extension.Record, position extension.BatchPosition) ([]extension.Result, error) {
	if err := extension.ValidateBatch(system, recs, position); err != nil {
		return nil, err
	}
	if _, err := r.declaredIngress(string(system)); err != nil {
		return nil, err
	}
	if !r.unattended {
		return nil, extension.ErrAttendedIngest
	}
	results := make([]extension.Result, 0, len(recs))
	for _, rec := range recs {
		result, err := r.Ingest(ctx, on, rec)
		if err != nil {
			return results, err
		}
		results = append(results, result)
	}
	return results, r.savePosition(ctx, system, position)
}

// IngestPosition answers the position the last saved batch for this source
// ended at, or empty when none was saved.
func (r *callRuntime) IngestPosition(ctx context.Context, system extension.SourceName) (extension.BatchPosition, error) {
	if _, err := r.declaredIngress(string(system)); err != nil {
		return "", err
	}
	scoped, err := r.scoped(ctx)
	if err != nil {
		return "", err
	}
	var position string
	err = database.WithWorkspaceTx(scoped, r.deps.pool, func(tx pgx.Tx) error {
		err := tx.QueryRow(scoped,
			`SELECT position FROM extension_ingest_position WHERE unit = $1 AND source_system = $2`,
			r.unit, string(system)).Scan(&position)
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		return err
	})
	return extension.BatchPosition(position), err
}

func (r *callRuntime) savePosition(ctx context.Context, system extension.SourceName, position extension.BatchPosition) error {
	scoped, err := r.scoped(ctx)
	if err != nil {
		return err
	}
	return database.WithWorkspaceTx(scoped, r.deps.pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(scoped, `
			INSERT INTO extension_ingest_position (unit, source_system, position)
			VALUES ($1, $2, $3)
			ON CONFLICT (unit, source_system) DO UPDATE
			   SET position = EXCLUDED.position, saved_at = now()`,
			r.unit, string(system), string(position))
		return err
	})
}
