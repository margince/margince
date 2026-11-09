// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// BackfillScheduler joins each execution's durable state and its queue entry.
type BackfillScheduler struct {
	Enqueue   func(context.Context, pgx.Tx, ids.UUID, int64, time.Time) error
	Executing func(context.Context, pgx.Tx, ids.UUID, int64) (bool, error)
}

// WithBackfillAdmission sets the installation budget; a hold accepts requests without starting pages.
func (r *Registry) WithBackfillAdmission(limit int, held bool) *Registry {
	r.backfillLimit = max(1, limit)
	r.backfillHeld = held
	return r
}

type admittedBackfill struct {
	id                    ids.UUID
	connection            ids.UUID
	generation            int64
	slot                  *int
	owner                 *int32
	due                   *time.Time
	status                string
	ready, mailboxBackoff bool
}

func backfillExecutionKey(id ids.UUID) string { return "capture:backfill:execution:" + id.String() }
func backfillSlotKey(slot int) string         { return fmt.Sprintf("capture:backfill:slot:%d", slot) }

// DispatchBackfills serializes installation admission without holding the lock during provider work.
func (r *Registry) DispatchBackfills(ctx context.Context, scheduler BackfillScheduler, recoverLost bool) error {
	pass := backfillAdmission{registry: r, scheduler: scheduler}
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		args := []any{"capture:backfill:admission"}
		var claimed bool
		if err := tx.QueryRow(ctx, `SELECT pg_try_advisory_xact_lock(hashtextextended(`+storekit.Placeholders(args)+`,0))`, args...).Scan(&claimed); err != nil {
			return err
		}
		if !claimed || r.backfillHeld {
			return nil
		}
		live, err := readAdmissionRuns(ctx, tx)
		if err != nil {
			return err
		}
		pass.tx = tx
		occupied, err := pass.reconcile(ctx, live, recoverLost)
		if err != nil {
			return err
		}
		return pass.promote(ctx, live, occupied)
	})
	return errors.Join(append(pass.failures, err)...)
}

type backfillAdmission struct {
	registry  *Registry
	tx        pgx.Tx
	scheduler BackfillScheduler
	failures  []error
}

func (p *backfillAdmission) attempt(ctx context.Context, fn func(pgx.Tx) error) error {
	err := database.WithDegradableSavepoint(ctx, p.tx, 500*time.Millisecond, fn)
	if err != nil {
		if p.tx.Conn().PgConn().TxStatus() != 'T' {
			return err
		}
		p.failures = append(p.failures, err)
	}
	return nil
}

func (p *backfillAdmission) reconcile(ctx context.Context, live []admittedBackfill, recoverLost bool) (map[int]bool, error) {
	occupied := map[int]bool{}
	for i, run := range live {
		if run.status != backfillStatusRunning || run.slot == nil {
			continue
		}
		release := *run.slot > p.registry.backfillLimit || (!run.ready && run.mailboxBackoff)
		if release {
			released := false
			if err := p.attempt(ctx, func(tx pgx.Tx) (err error) { released, err = requeueExcessBackfillTx(ctx, tx, run); return err }); err != nil {
				return nil, err
			}
			if released {
				live[i].status, live[i].slot = backfillStatusQueued, nil
				continue
			}
		}
		occupied[*run.slot] = true
		if recoverLost && !release {
			if err := p.attempt(ctx, func(tx pgx.Tx) error { return p.registry.recoverBackfillTx(ctx, tx, run, p.scheduler) }); err != nil {
				return nil, err
			}
		}
	}
	return occupied, nil
}

func (p *backfillAdmission) promote(ctx context.Context, live []admittedBackfill, occupied map[int]bool) error {
	for _, run := range live {
		if len(occupied) >= p.registry.backfillLimit {
			break
		}
		if !run.ready || (run.status == backfillStatusRunning && run.slot != nil) {
			continue
		}
		slot := 1
		for occupied[slot] {
			slot++
		}
		admitted := false
		if err := p.attempt(ctx, func(tx pgx.Tx) (err error) {
			admitted, err = p.registry.admitBackfillTx(ctx, tx, run.id, slot, p.scheduler)
			return err
		}); err != nil {
			return err
		}
		if admitted {
			occupied[slot] = true
		}
	}
	return nil
}

const admissionOrder = `CASE WHEN created_at < now()-interval '1 hour' THEN 0 ELSE 1 END,
 CASE WHEN created_at < now()-interval '1 hour' THEN created_at END,
 CASE WHEN NOT total_estimate_is_floor THEN total_estimate END ASC NULLS LAST,created_at,id`

func readAdmissionRuns(ctx context.Context, tx pgx.Tx) ([]admittedBackfill, error) {
	args := []any{64}
	query := fmt.Sprintf(`SELECT id,connection_id,execution_generation,admission_slot,execution_owner_pid,execution_due_at,status,COALESCE(execution_due_at<=now(),true),backoff_is_mailbox_scoped
 FROM capture_backfill WHERE (status='running' AND admission_slot IS NOT NULL)
 OR id IN (SELECT id FROM capture_backfill WHERE (status='queued' OR (status='running' AND admission_slot IS NULL))
 AND COALESCE(execution_due_at<=now(),true) ORDER BY `+admissionOrder+` LIMIT $%d) ORDER BY `+admissionOrder, len(args))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	runs := []admittedBackfill{}
	for rows.Next() {
		var run admittedBackfill
		if err := rows.Scan(&run.id, &run.connection, &run.generation, &run.slot, &run.owner, &run.due, &run.status, &run.ready, &run.mailboxBackoff); err != nil {
			return nil, err
		}
		runs = append(runs, run)
	}
	return runs, rows.Err()
}

func (r *Registry) admitBackfillTx(ctx context.Context, tx pgx.Tx, id ids.UUID, slot int, scheduler BackfillScheduler) (bool, error) {
	args := []any{id}
	idArg := len(args)
	args = append(args, slot)
	slotArg := len(args)
	var generation int64
	var due time.Time
	query := fmt.Sprintf(`UPDATE capture_backfill SET status='running',admission_slot=$%d,
 execution_generation=execution_generation+1,execution_owner_pid=NULL,execution_due_at=GREATEST(COALESCE(execution_due_at,now()),now()),
 started_at=COALESCE(started_at,now())`+resetInflightProgress+` WHERE id=$%d AND (status='queued' OR (status='running' AND admission_slot IS NULL)) RETURNING execution_generation,execution_due_at`, slotArg, idArg)
	if err := tx.QueryRow(ctx, query, args...).Scan(&generation, &due); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	if err := scheduler.Enqueue(ctx, tx, id, generation, due); err != nil {
		return false, err
	}
	return true, nil
}

func (r *Registry) recoverBackfillTx(ctx context.Context, tx pgx.Tx, run admittedBackfill, scheduler BackfillScheduler) error {
	free, err := claimBackfillRecoveryTx(ctx, tx, run)
	if err != nil || !free {
		return err
	}
	executing, err := scheduler.Executing(ctx, tx, run.id, run.generation)
	if err != nil {
		return err
	}
	if run.owner != nil || executing {
		args := []any{run.id, run.generation}
		query := fmt.Sprintf(`UPDATE capture_backfill SET execution_generation=execution_generation+1,execution_owner_pid=NULL`+resetInflightProgress+`
  WHERE id=$%d AND execution_generation=$%d AND status='running' RETURNING execution_generation`, len(args)-1, len(args))
		if err := tx.QueryRow(ctx, query, args...).Scan(&run.generation); err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				return nil
			}
			return err
		}
	}
	due := r.now()
	if run.due != nil {
		due = *run.due
	}
	return scheduler.Enqueue(ctx, tx, run.id, run.generation, due)
}

func claimBackfillRecoveryTx(ctx context.Context, tx pgx.Tx, run admittedBackfill) (bool, error) {
	for _, key := range []string{backfillExecutionKey(run.id), backfillSlotKey(*run.slot)} {
		args := []any{key}
		var free bool
		query := `SELECT pg_try_advisory_xact_lock(hashtextextended(` + storekit.Placeholders(args) + `,0))`
		if err := tx.QueryRow(ctx, query, args...).Scan(&free); err != nil || !free {
			return false, err
		}
	}
	return true, nil
}

func requeueExcessBackfillTx(ctx context.Context, tx pgx.Tx, run admittedBackfill) (bool, error) {
	free, err := claimBackfillRecoveryTx(ctx, tx, run)
	if err != nil || !free {
		return false, err
	}
	if err := lockConnectionTx(ctx, tx, run.connection); err != nil {
		return false, err
	}
	args := []any{run.id, run.generation}
	query := fmt.Sprintf(`UPDATE capture_backfill SET status = 'queued',admission_slot=NULL,
 execution_owner_pid=NULL,execution_generation=execution_generation+1`+resetInflightProgress+`
 WHERE id=$%d AND execution_generation=$%d AND status='running'`, len(args)-1, len(args))
	tag, err := tx.Exec(ctx, query, args...)
	return tag.RowsAffected() > 0, err
}
