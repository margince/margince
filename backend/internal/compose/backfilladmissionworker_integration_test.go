// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAnAdmissionWorkerPreservesTheSharedSessionBudgetSnooze(t *testing.T) {
	b := setupBackfillWire(t)
	runID := strandedBackfill(t, b)
	ctx := backfillWorkContext(principal.WithWorkspaceID(context.Background(), b.env.WS), t, b)
	if err := b.registry.DispatchBackfills(ctx, backfillScheduler(b.env.WS), false); err != nil {
		t.Fatal(err)
	}
	var generation int64
	args := []any{runID}
	if err := b.env.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT execution_generation FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	ctx = database.WithSessionBudget(ctx, func(context.Context) (func(), error) { return nil, river.JobSnooze(5 * time.Second) })
	worker := &captureBackfillWorker{registry: b.registry, log: b.handlers.log}
	err := worker.Work(ctx, &river.Job[CaptureBackfillArgs]{JobRow: &rivertype.JobRow{}, Args: CaptureBackfillArgs{Workspace: b.env.WS, BackfillID: runID.String(), ExecutionGeneration: generation}})
	if _, snoozed := errors.AsType[*river.JobSnoozeError](err); !snoozed {
		t.Fatalf("resource deferral swallowed: %v", err)
	}
	var status string
	var failures int
	args = []any{runID}
	if err := b.env.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT status,consecutive_failures FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&status, &failures); err != nil {
		t.Fatal(err)
	}
	if status != "running" || failures != 0 {
		t.Fatalf("resource deferral changed run: status=%s failures=%d", status, failures)
	}
}

func TestARecoveredRunningJobGetsANewExecutionWithoutDuplicatingTheSuccessor(t *testing.T) {
	b := setupBackfillWire(t)
	runID := strandedBackfill(t, b)
	ctx := backfillWorkContext(principal.WithWorkspaceID(context.Background(), b.env.WS), t, b)
	if err := b.registry.DispatchBackfills(ctx, backfillScheduler(b.env.WS), false); err != nil {
		t.Fatal(err)
	}
	var previous int64
	args := []any{runID}
	if err := b.env.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT execution_generation FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&previous); err != nil {
		t.Fatal(err)
	}
	args = []any{runID}
	if _, err := b.env.Pool.Exec(ctx, fmt.Sprintf(`UPDATE river_job SET state='running',attempted_at=now() WHERE kind='capture_backfill' AND args->>'backfill_id'=$%d`, len(args)), args...); err != nil {
		t.Fatal(err)
	}
	if err := b.registry.DispatchBackfills(ctx, backfillScheduler(b.env.WS), true); err != nil {
		t.Fatal(err)
	}
	var generation int64
	args = []any{runID}
	if err := b.env.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT execution_generation FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&generation); err != nil {
		t.Fatal(err)
	}
	if generation <= previous {
		t.Fatalf("lost running job reused generation %d", generation)
	}
	if err := b.registry.DispatchBackfills(ctx, backfillScheduler(b.env.WS), true); err != nil {
		t.Fatal(err)
	}
	var current, jobs int
	args = []any{runID}
	if err := b.env.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM river_job WHERE kind='capture_backfill' AND args->>'backfill_id'=$%d`, len(args)), args...).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	args = []any{runID, generation}
	if err := b.env.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT count(*) FROM river_job WHERE kind='capture_backfill' AND args->>'backfill_id'=$%d AND (args->>'execution_generation')::bigint=$%d`, len(args)-1, len(args)), args...).Scan(&current); err != nil {
		t.Fatal(err)
	}
	if jobs != 2 || current != 1 {
		t.Fatalf("recovered jobs=%d successor=%d", jobs, current)
	}
}

func TestALegacyRunningImportEntersTheAdmissionBudget(t *testing.T) {
	b := setupBackfillWire(t)
	runID := strandedBackfill(t, b)
	ctx := backfillWorkContext(principal.WithWorkspaceID(context.Background(), b.env.WS), t, b)
	args := []any{runID}
	if _, err := b.env.Pool.Exec(ctx, fmt.Sprintf(`UPDATE capture_backfill SET status='running' WHERE id=$%d`, len(args)), args...); err != nil {
		t.Fatal(err)
	}
	worker := &captureBackfillWorker{registry: b.registry, log: b.handlers.log}
	jobArgs := CaptureBackfillArgs{Workspace: b.env.WS, BackfillID: runID.String()}
	if err := worker.Work(ctx, &river.Job[CaptureBackfillArgs]{JobRow: &rivertype.JobRow{}, Args: jobArgs}); err != nil {
		t.Fatal(err)
	}
	var generation int64
	var slot int
	var scanned int
	args = []any{runID}
	if err := b.env.Pool.QueryRow(ctx, fmt.Sprintf(`SELECT execution_generation,admission_slot,scanned FROM capture_backfill WHERE id=$%d`, len(args)), args...).Scan(&generation, &slot, &scanned); err != nil {
		t.Fatal(err)
	}
	if generation != 1 || slot != 1 || scanned != 0 {
		t.Fatalf("legacy kick ran a page instead of admitting it: generation=%d slot=%d scanned=%d", generation, slot, scanned)
	}
}

type admissionJobClock struct{ at time.Time }

func (c *admissionJobClock) Now() time.Time       { return c.at }
func (c *admissionJobClock) NowOrNil() *time.Time { return &c.at }

func TestARunningReconcileCannotSuppressALaterRecoveryWindow(t *testing.T) {
	b := setupBackfillWire(t)
	ctx := context.Background()
	clock := &admissionJobClock{at: clocktest.Now(t)}
	client, err := river.NewClient(riverpgxv5.New(b.env.Pool), &river.Config{Logger: b.handlers.log, Test: river.TestConfig{Time: clock}})
	if err != nil {
		t.Fatal(err)
	}
	kind := CaptureBackfillReconcileArgs{}
	opts := periodicInsertOpts(kind)
	first, err := client.Insert(ctx, kind, opts)
	if err != nil {
		t.Fatal(err)
	}
	parameters := []any{first.Job.ID}
	if _, err := b.env.Pool.Exec(ctx, fmt.Sprintf(`UPDATE river_job SET state='running',attempted_at=now() WHERE id=$%d`, len(parameters)), parameters...); err != nil {
		t.Fatal(err)
	}
	duplicate, err := client.Insert(ctx, kind, opts)
	if err != nil {
		t.Fatal(err)
	}
	if !duplicate.UniqueSkippedAsDuplicate {
		t.Fatal("same recovery window did not coalesce")
	}
	clock.at = clock.at.Add(specFor(t, kind.Kind()).Cadence.Fixed)
	next, err := client.Insert(ctx, kind, opts)
	if err != nil {
		t.Fatal(err)
	}
	if next.UniqueSkippedAsDuplicate || next.Job.ID == first.Job.ID {
		t.Fatal("an abandoned running row suppressed a later recovery window")
	}
}
