// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type backfillExecutionKeyType struct{}

type backfillExecution struct {
	id         ids.UUID
	generation int64
	slot       int
	pid        uint32
	nonce      string
	scheduler  BackfillScheduler
	scheduled  *bool
	lease      *database.SessionLease
}

func (e backfillExecution) guard(ctx context.Context, tx pgx.Tx) error {
	args := []any{backfillSlotKey(e.slot)}
	slotArg := len(args)
	args = append(args, e.id)
	idArg := len(args)
	args = append(args, e.generation)
	generationArg := len(args)
	args = append(args, e.pid)
	pidArg := len(args)
	args = append(args, e.nonce)
	nonceArg := len(args)
	query := fmt.Sprintf(`WITH slot AS MATERIALIZED (
 SELECT pg_advisory_xact_lock_shared(hashtextextended($%[1]d,0)))
 SELECT NOT pg_try_advisory_xact_lock_shared(hashtextextended($%[5]d,0))
 AND EXISTS(SELECT 1 FROM capture_backfill b
 WHERE b.id=$%[2]d AND b.status='running' AND b.execution_generation=$%[3]d
 AND b.execution_owner_pid=$%[4]d) FROM slot`, slotArg, idArg, generationArg, pidArg, nonceArg)
	var valid bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&valid); err != nil {
		return err
	}
	if !valid {
		return fmt.Errorf("capture: history execution no longer owns its slot: %w", apperrors.ErrConflict)
	}
	return nil
}

func (r *Registry) leaseBackfill(ctx context.Context, id ids.UUID, generation int64, scheduler BackfillScheduler) (context.Context, func(), bool, error) {
	lease, held, err := database.TrySessionLease(ctx, r.db.Pool(), backfillExecutionKey(id), slog.Default())
	if err != nil || !held {
		return ctx, nil, false, err
	}
	execution := backfillExecution{lease: lease, id: id, generation: generation, nonce: "capture:backfill:page:" + ids.NewV7().String(), pid: lease.PID(), scheduler: scheduler, scheduled: new(bool)}
	args := []any{id, generation}
	err = r.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, fmt.Sprintf(`SELECT admission_slot FROM capture_backfill WHERE id=$%d AND execution_generation=$%d AND status='running'`, len(args)-1, len(args)), args...).Scan(&execution.slot)
	})
	if err != nil {
		lease.Close(ctx)
		return ctx, nil, false, fmt.Errorf("reading an admitted slot: %w", err)
	}
	if execution.slot > r.backfillLimit {
		lease.Close(ctx)
		return ctx, nil, false, nil
	}

	held, err = lease.TryLock(ctx, backfillSlotKey(execution.slot), false)
	if err != nil || !held {
		lease.Close(ctx)
		return ctx, nil, false, err
	}
	if held, err = lease.TryLock(ctx, backfillSlotKey(execution.slot), true); err != nil || !held {
		lease.Close(ctx)
		return ctx, nil, false, err
	}
	if err = lease.Unlock(ctx, backfillSlotKey(execution.slot), false); err != nil {
		lease.Close(ctx)
		return ctx, nil, false, err
	}
	if held, err = lease.TryLock(ctx, execution.nonce, false); err != nil || !held {
		lease.Close(ctx)
		return ctx, nil, false, err
	}
	args = []any{id}
	idArg := len(args)
	args = append(args, generation)
	generationArg := len(args)
	args = append(args, execution.pid)
	pidArg := len(args)
	err = r.db.Tx(ctx, func(tx pgx.Tx) error {
		query := fmt.Sprintf(`UPDATE capture_backfill SET execution_owner_pid=$%d WHERE id=$%d
  AND execution_generation=$%d AND status='running' RETURNING id`, pidArg, idArg, generationArg)
		var found ids.UUID
		return tx.QueryRow(ctx, query, args...).Scan(&found)
	})
	if err != nil {
		lease.Close(ctx)
		return ctx, nil, false, fmt.Errorf("claiming an execution generation: %w", err)
	}
	ctx = context.WithValue(ctx, backfillExecutionKeyType{}, execution)
	return database.WithTransactionGuard(ctx, execution.guard), func() { lease.Close(ctx) }, true, nil
}
