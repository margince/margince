// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migration

// A commit and an undo run detached from the request that started them. These
// transitions keep such a run movable when its work dies. A failed run is
// resumable by approve, and an undoing run by undo.
//
// Only `running` is ever moved to `failed`. An `undoing` run is already
// resumable, and failing it would make approve re-run the commit over a
// half-undone run.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// FailRunning records why a running run's work stopped, when the engine could
// not record it itself. A run in any other state is left as it is.
func (s *RunStore) FailRunning(ctx context.Context, id RunID, cause string) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `
			UPDATE import_run SET status = $2, error = $3, updated_at = now()
			 WHERE id = $1 AND status = $4`, id, StatusFailed, cause, StatusRunning)
		if err != nil {
			return fmt.Errorf("failing import run %s: %w", id, err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		_, err = storekit.Audit(ctx, tx, "update", ImportRunObject, id,
			map[string]any{auditFieldStatus: StatusRunning}, map[string]any{auditFieldStatus: StatusFailed})
		return err
	})
}

// FailStaleRunning fails every running import whose last checkpoint is older
// than idle, and answers how many. updated_at moves on every checkpoint, so a
// run that has not moved for that long has no process left working it. The
// cutoff is taken on the database clock that wrote updated_at.
func (s *RunStore) FailStaleRunning(ctx context.Context, idle time.Duration, cause string) (int, error) {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return 0, err
	}
	failed := 0
	err := s.tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			UPDATE import_run SET status = $1, error = $2, updated_at = now()
			 WHERE status = $3 AND connector = $4 AND updated_at < now() - make_interval(secs => $5)
			RETURNING id`, StatusFailed, cause, StatusRunning, ConnectorCSV, idle.Seconds())
		if err != nil {
			return fmt.Errorf("failing stale import runs: %w", err)
		}
		stale, err := pgx.CollectRows(rows, pgx.RowTo[RunID])
		if err != nil {
			return fmt.Errorf("failing stale import runs: %w", err)
		}
		for _, id := range stale {
			if _, err := storekit.Audit(ctx, tx, "update", ImportRunObject, id,
				map[string]any{auditFieldStatus: StatusRunning}, map[string]any{auditFieldStatus: StatusFailed}); err != nil {
				return err
			}
		}
		failed = len(stale)
		return nil
	})
	return failed, err
}

// ClaimUndo moves a complete run to undoing, or confirms one already undoing,
// so the caller can answer before the reversal runs. It refuses while another
// undo of the run holds the claim, as Undo does.
func (s *RunStore) ClaimUndo(ctx context.Context, id RunID) error {
	if err := auth.Require(ctx, ImportRunObject, principal.ActionUpdate); err != nil {
		return err
	}
	release, err := s.lockUndo(ctx, id)
	if err != nil {
		return err
	}
	defer release()
	_, _, err = s.beginUndo(ctx, id)
	return err
}
