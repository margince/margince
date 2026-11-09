// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type checkpointHistory struct {
	fixtureConnector
	calls            int
	beforeCheckpoint func() error
}

func (*checkpointHistory) AccountLabel(auth connector.Auth) (string, error) {
	return string(auth) + "@mail.example", nil
}

func (*checkpointHistory) EstimateBackfill(context.Context, connector.Auth, time.Time) (connector.BackfillEstimate, error) {
	return connector.BackfillEstimate{Messages: 20}, nil
}

func (p *checkpointHistory) BackfillPage(ctx context.Context, _ connector.Auth, _ time.Time, token string, _ connector.Sink) (connector.BackfillPageResult, error) {
	p.calls++
	if p.beforeCheckpoint != nil {
		if err := p.beforeCheckpoint(); err != nil {
			return connector.BackfillPageResult{}, err
		}
	}
	progress := connector.BackfillProgressFrom(ctx)
	if p.calls == 1 {
		if token != "" {
			return connector.BackfillPageResult{}, errors.New("unexpected initial cursor")
		}
		if supported, err := progress.Checkpoint(ctx, "exact-remaining-ids", connector.BackfillPageResult{Scanned: 10, Captured: 6, Skipped: 3, Failed: 1}); !supported || err != nil {
			return connector.BackfillPageResult{}, errors.Join(errors.New("initial checkpoint failed"), err)
		}
		progress.Observed(ctx, 15, 9, 6)
		return connector.BackfillPageResult{}, connector.ErrUnreachable
	}
	if token != "exact-remaining-ids" {
		return connector.BackfillPageResult{}, errors.New("checkpoint cursor was lost")
	}
	res := connector.BackfillPageResult{Scanned: 10, Captured: 7, Skipped: 3}
	if _, err := progress.Checkpoint(ctx, "prepared-empty-page", res); err != nil {
		return connector.BackfillPageResult{}, err
	}
	progress.Observed(ctx, 10, 7, 3)
	return res, nil
}

func TestHistoryCheckpointsCommitCountsOnceAcrossPageFailure(t *testing.T) {
	provider := &checkpointHistory{}
	ctx, reg, id := admissionRunWith(t, provider)
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler()); !errors.Is(err, connector.ErrUnreachable) {
		t.Fatalf("first page: %v", err)
	}
	owner, _ := setupCaptureDB(t)
	var scanned, captured, skipped, failed, inflight int
	read := func() {
		t.Helper()
		if err := owner.QueryRow(ctx, `SELECT scanned,captured,skipped,failed,inflight_scanned FROM capture_backfill WHERE id=@run`, pgx.NamedArgs{"run": id}).Scan(&scanned, &captured, &skipped, &failed, &inflight); err != nil {
			t.Fatal(err)
		}
	}
	read()
	if scanned != 10 || captured != 6 || skipped != 3 || failed != 1 || inflight != 0 {
		t.Fatalf("failed page counters: %d %d %d %d inflight=%d", scanned, captured, skipped, failed, inflight)
	}
	if _, err := owner.Exec(ctx, `UPDATE capture_backfill SET execution_due_at=now()-interval '1 second' WHERE id=@run`, pgx.NamedArgs{"run": id}); err != nil {
		t.Fatal(err)
	}
	if _, completed, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler()); err != nil || !completed {
		t.Fatalf("retry completed=%v: %v", completed, err)
	}
	read()
	if scanned != 20 || captured != 13 || skipped != 6 || failed != 1 || inflight != 0 {
		t.Fatalf("completion counted checkpoints twice: %d %d %d %d inflight=%d", scanned, captured, skipped, failed, inflight)
	}
}

func TestCancelledHistoryRejectsItsInFlightCheckpoint(t *testing.T) {
	provider := &checkpointHistory{}
	ctx, reg, id := admissionRunWith(t, provider)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture actor missing")
	}
	provider.beforeCheckpoint = func() error {
		_, err := reg.CancelBackfill(ctx, "gmail", ids.From[ids.UserKind](actor.UserID))
		return err
	}
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
		t.Fatal(err)
	}
	_, completed, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler())
	if completed {
		t.Fatalf("cancelled page completed: %v", err)
	}
	owner, _ := setupCaptureDB(t)
	var status string
	var count int
	if err := owner.QueryRow(ctx, `SELECT status,scanned FROM capture_backfill WHERE id=@run`, pgx.NamedArgs{"run": id}).Scan(&status, &count); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" || count != 0 {
		t.Fatalf("cancelled checkpoint changed state: %s %d", status, count)
	}
}

func TestReconnectedHistoryEndsBeforeSavingItsCheckpoint(t *testing.T) {
	provider := &checkpointHistory{}
	ctx, reg, id := admissionRunWith(t, provider)
	provider.beforeCheckpoint = func() error { _, err := reg.Connect(ctx, "gmail", connector.Auth("replacement-grant")); return err }
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
		t.Fatal(err)
	}
	done, completed, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler())
	if err != nil || !done || completed {
		t.Fatalf("rebound page done=%v completed=%v err=%v", done, completed, err)
	}
	owner, _ := setupCaptureDB(t)
	var status string
	var scanned int
	if err := owner.QueryRow(ctx, `SELECT status,scanned FROM capture_backfill WHERE id=@run`, pgx.NamedArgs{"run": id}).Scan(&status, &scanned); err != nil {
		t.Fatal(err)
	}
	if status != "cancelled" || scanned != 0 {
		t.Fatalf("rebound checkpoint stayed live: %s %d", status, scanned)
	}
}

type dripCheckpointHistory struct {
	fixtureConnector
	calls int
}

func (*dripCheckpointHistory) EstimateBackfill(context.Context, connector.Auth, time.Time) (connector.BackfillEstimate, error) {
	return connector.BackfillEstimate{Messages: 13}, nil
}

func (p *dripCheckpointHistory) BackfillPage(ctx context.Context, _ connector.Auth, _ time.Time, _ string, _ connector.Sink) (connector.BackfillPageResult, error) {
	p.calls++
	result := connector.BackfillPageResult{Scanned: 1, Captured: 1}
	if p.calls > 12 {
		return result, nil
	}
	if _, err := connector.BackfillProgressFrom(ctx).Checkpoint(ctx, fmt.Sprintf("remaining-after-%d", p.calls), result); err != nil {
		return connector.BackfillPageResult{}, err
	}
	return connector.BackfillPageResult{}, &connector.RateLimitedError{RetryAfter: time.Minute, Reason: connector.RateLimitUser}
}

func TestSavedProgressKeepsAThrottledImportAlivePastTheFailureLimit(t *testing.T) {
	provider := &dripCheckpointHistory{}
	ctx, reg, id := admissionRunWith(t, provider)
	owner, _ := setupCaptureDB(t)
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	for attempt := range 13 {
		if _, err := owner.Exec(ctx, `UPDATE capture_backfill SET execution_due_at=now()-interval '1 second' WHERE id=@run`, pgx.NamedArgs{"run": id}); err != nil {
			t.Fatal(err)
		}
		if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
			t.Fatal(err)
		}
		_, completed, _, err := reg.RunBackfillExecution(ctx, id, queue.generations[id], queue.scheduler())
		if attempt < 12 && (!errors.Is(err, connector.ErrRateLimited) || completed) {
			t.Fatalf("attempt %d: completed=%v err=%v", attempt, completed, err)
		}
		if attempt == 12 && (err != nil || !completed) {
			t.Fatalf("final attempt: completed=%v err=%v", completed, err)
		}
	}
	var scanned, captured int
	var status string
	if err := owner.QueryRow(ctx, `SELECT status,scanned,captured FROM capture_backfill WHERE id=@run`, pgx.NamedArgs{"run": id}).Scan(&status, &scanned, &captured); err != nil {
		t.Fatal(err)
	}
	if status != "done" || scanned != 13 || captured != 13 {
		t.Fatalf("progress was abandoned: %s %d %d", status, scanned, captured)
	}
}
