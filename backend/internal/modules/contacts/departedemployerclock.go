// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// departureLookbackDays reaches back past the day a departure took effect, so
// a pass that missed a few days still refolds what those days left stale.
const departureLookbackDays = 7

// departedEmployerBatch bounds one transaction, so no pass holds company row
// locks long enough to stall the activity writes queued behind them.
const departedEmployerBatch = 300

// RefoldDepartedEmployerClocks refolds each company clock a recent departure left
// stale, since the day a notice period ends changes no row a trigger watches.
func (s *Store) RefoldDepartedEmployerClocks(ctx context.Context) (int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return 0, err
	}
	moved := 0
	var after ids.UUID
	for {
		var batch []ids.UUID
		err := s.db.Tx(ctx, func(tx pgx.Tx) error {
			var err error
			batch, err = refoldDepartedEmployerBatch(ctx, tx, after)
			return err
		})
		if err != nil {
			return moved, err
		}
		moved += len(batch)
		if len(batch) < departedEmployerBatch {
			return moved, nil
		}
		after = batch[len(batch)-1]
	}
}

// refoldDepartedEmployerBatch refolds the next batch past `after` in company id
// order, the lock order every last-activity writer takes.
func refoldDepartedEmployerBatch(ctx context.Context, tx pgx.Tx, after ids.UUID) ([]ids.UUID, error) {
	end := employment.EffectiveEndSQL("r.ended_at", "r.ended_precision")
	// OFFSET 0 fences the candidates: unfenced, the planner folds the clock of
	// every company before the departure filter narrows them.
	rows, err := tx.Query(ctx, `
		SELECT cand.id FROM (
		  SELECT c.id, c.last_activity_at FROM company c
		   WHERE c.id > $1
		     AND EXISTS (SELECT 1 FROM relationship r
		                  WHERE r.company_id = c.id AND r.kind = 'employment' AND r.archived_at IS NULL
		                    AND `+end+` BETWEEN current_date - $2::int AND current_date)
		  OFFSET 0) cand
		 WHERE cand.last_activity_at IS DISTINCT FROM last_activity_of_company(cand.id)
		 ORDER BY cand.id
		 LIMIT $3`,
		after, departureLookbackDays, departedEmployerBatch)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading companies a departure left with a stale clock: %w", err)
	}
	stale, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading companies a departure left with a stale clock: %w", err)
	}
	for _, id := range stale {
		if _, err := tx.Exec(ctx, `SELECT move_last_activity('company'::regclass, $1)`, id); err != nil {
			return nil, fmt.Errorf("contacts: refolding company %s's last-activity clock: %w", id, err)
		}
	}
	return stale, nil
}
