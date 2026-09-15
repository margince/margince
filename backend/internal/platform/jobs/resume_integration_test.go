// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package jobs_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type resumeProbe struct {
	Workspace ids.UUID `json:"workspace_id"`
	Carrier   ids.UUID `json:"carrier_id"`
}

func (resumeProbe) Kind() string { return "budget_resume_probe" }

func TestBudgetRecoveryPreservesJobAttemptsAndIsolation(t *testing.T) {
	ctx, pool := quiesceTestPool(t)
	runner := quiesceTestRunner(t, pool)
	for _, state := range []string{"scheduled", "retryable", "available", "running", "completed", "cancelled", "discarded"} {
		t.Run(state, func(t *testing.T) {
			args := resumeProbe{Workspace: ids.NewV7(), Carrier: ids.NewV7()}
			if err := runner.Enqueue(ctx, args, &river.InsertOpts{ScheduledAt: time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC), MaxAttempts: 3}); err != nil {
				t.Fatal(err)
			}
			values := []any{state, args.Carrier.String()}
			_, err := pool.Exec(ctx, `UPDATE river_job SET state=$1::river_job_state,attempt=1,attempted_at=now(),finalized_at=CASE WHEN $1 IN ('completed','cancelled','discarded') THEN now() ELSE NULL END WHERE args->>'carrier_id'=$2`, values...)
			if err != nil {
				t.Fatal(err)
			}
			resume := func(workspace ids.UUID) (bool, error) {
				var woke bool
				err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
					var err error
					woke, err = runner.ResumeScheduledTx(ctx, tx, args.Kind(), "carrier_id", args.Carrier, workspace)
					return err
				})
				return woke, err
			}
			if woke, err := resume(ids.NewV7()); err == nil || woke {
				t.Fatal("another workspace resumed this job")
			}
			woke, err := resume(args.Workspace)
			terminal := state == "completed" || state == "cancelled" || state == "discarded"
			if terminal {
				if err == nil || woke {
					t.Fatalf("terminal %s revived", state)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if woke != (state != "running") {
				t.Fatalf("state %s woke=%v", state, woke)
			}
			var attempt, maxAttempts int
			if err := pool.QueryRow(ctx, `SELECT attempt,max_attempts FROM river_job WHERE args->>'carrier_id'=$1`, args.Carrier.String()).Scan(&attempt, &maxAttempts); err != nil {
				t.Fatal(err)
			}
			if attempt != 1 || maxAttempts != 3 {
				t.Fatalf("attempt accounting reset: %d/%d", attempt, maxAttempts)
			}
		})
	}
}

func TestBudgetJobRecoveryRollsBackWithItsCarrier(t *testing.T) {
	ctx, pool := quiesceTestPool(t)
	runner := quiesceTestRunner(t, pool)
	args := resumeProbe{Workspace: ids.NewV7(), Carrier: ids.NewV7()}
	future := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	if err := runner.Enqueue(ctx, args, &river.InsertOpts{ScheduledAt: future, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}
	forced := errors.New("carrier could not commit")
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := runner.ResumeScheduledTx(ctx, tx, args.Kind(), "carrier_id", args.Carrier, args.Workspace); err != nil {
			return err
		}
		return forced
	})
	if !errors.Is(err, forced) {
		t.Fatal(err)
	}
	var scheduled time.Time
	if err := pool.QueryRow(context.Background(), `SELECT scheduled_at FROM river_job WHERE args->>'carrier_id'=$1`, args.Carrier.String()).Scan(&scheduled); err != nil {
		t.Fatal(err)
	}
	if !scheduled.Equal(future) {
		t.Fatal("job escaped the carrier rollback")
	}
}

func TestFailedCarrierSavepointRollsBackOnlyItsRetriedJob(t *testing.T) {
	ctx, pool := quiesceTestPool(t)
	runner := quiesceTestRunner(t, pool)
	workspace := ids.NewV7()
	carriers := []ids.UUID{ids.NewV7(), ids.NewV7()}
	next := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)
	for _, id := range carriers {
		if err := runner.Enqueue(ctx, resumeProbe{Workspace: workspace, Carrier: id}, &river.InsertOpts{ScheduledAt: next, MaxAttempts: 3}); err != nil {
			t.Fatal(err)
		}
	}
	refused := errors.New("carrier write refused after retry")
	err := storekit.RecoverPages(ctx, func(ctx context.Context, visit func(pgx.Tx) error) error { return pgx.BeginFunc(ctx, pool, visit) },
		func(pgx.Tx, ids.UUID) ([]ids.UUID, error) { return carriers, nil }, func(id ids.UUID) ids.UUID { return id },
		func(tx pgx.Tx, id ids.UUID) error {
			woke, err := runner.ResumeScheduledTx(ctx, tx, resumeProbe{}.Kind(), "carrier_id", id, workspace)
			if err != nil {
				return err
			}
			if !woke {
				t.Fatal("queued job was not retried")
			}
			if id == carriers[0] {
				return refused
			}
			return nil
		})
	if !errors.Is(err, refused) {
		t.Fatalf("lost failed row: %v", err)
	}
	for i, id := range carriers {
		var at time.Time
		if err := pool.QueryRow(ctx, `SELECT scheduled_at FROM river_job WHERE kind=$1 AND args->>'carrier_id'=$2`, resumeProbe{}.Kind(), id.String()).Scan(&at); err != nil {
			t.Fatal(err)
		}
		if (i == 0 && !at.Equal(next)) || (i == 1 && !at.Before(next)) {
			t.Fatalf("carrier %d scheduled at %s", i, at)
		}
	}
}
