// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// A truncated import is resumable, and a successful sync is what resumes it.
//
// The run that ends on a terminal fault keeps its cursor, so the mailbox never
// has to be re-read from the top — but nothing used that cursor, and the run sat
// at `error` forever. The commonest cause is a credential the operator then
// fixes: rotating the connector's client app invalidates the grant mid-import,
// the human reconnects, mail flows again, and the history import stays dead with
// no sign that it is.
//
// These run on a real database because every assertion is about what a row is
// after a write nobody mocked: the status the revival moved it to, the cap that
// refused to move it, and the cursor it kept while doing so.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// truncatedRun leaves the fixture's connection holding a run that ended on a
// terminal fault with its resume point intact — the state failBackfill leaves
// behind. errClass is what ended it, which is what decides whether reconnecting
// is the remedy.
func truncatedRun(ctx context.Context, t *testing.T, errClass string) (connID, backfillID ids.UUID) {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("fixture context carries no actor")
	}
	if err := owner.QueryRow(ctx,
		`SELECT id FROM capture_connection WHERE user_id = $1 AND provider = 'gmail'`,
		actor.UserID).Scan(&connID); err != nil {
		t.Fatalf("reading the fixture's connection: %v", err)
	}
	if err := owner.QueryRow(ctx, `
		UPDATE capture_backfill
		   SET status = 'error', last_error_class = $2, completed_at = now(),
		       cursor = '{"page_token":"resume-here"}'
		 WHERE connection_id = $1
		 RETURNING id`, connID, errClass).Scan(&backfillID); err != nil {
		t.Fatalf("truncating the run: %v", err)
	}
	return connID, backfillID
}

func runState(ctx context.Context, t *testing.T, backfillID ids.UUID) (status string, completedAt *string, cursor []byte) {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	if err := owner.QueryRow(ctx,
		`SELECT status, completed_at::text, cursor FROM capture_backfill WHERE id = $1`,
		backfillID).Scan(&status, &completedAt, &cursor); err != nil {
		t.Fatalf("reading the run: %v", err)
	}
	return status, completedAt, cursor
}

// startFixtureBackfill puts a run on the fixture's connection. The enqueue is a
// no-op because these tests are about the ROW's lifecycle; the reconcile pass
// owns putting a job behind a live run, and asserting that here would be
// asserting somebody else's contract.
func startFixtureBackfill(ctx context.Context, t *testing.T, reg *capture.Registry) {
	t.Helper()
	actor, _ := principal.Actor(ctx)
	if _, err := reg.StartBackfill(ctx, "gmail", ids.From[ids.UserKind](actor.UserID), 3, connector.BackfillEstimate{Messages: 100},
		func(context.Context, pgx.Tx, ids.UUID) error { return nil }); err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}
}

// The case the change exists for. The credential that ended the run has been
// fixed — that is what a successful sync proves — so the run goes back to being
// live and keeps the cursor it stopped on. Re-enqueueing it is the reconcile
// pass's job, which already puts a job behind every live run.
func TestASuccessfulSyncRevivesATruncatedImport(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	connectFixtureConnection(ctx, t, reg)
	startFixtureBackfill(ctx, t, reg)
	connID, backfillID := truncatedRun(ctx, t, "auth")

	if err := reg.SyncOnce(ctx, connID); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}

	status, completedAt, cursor := runState(ctx, t, backfillID)
	if status != "queued" {
		t.Errorf("status = %q, want %q — a healthy connection is what makes the run live again", status, "queued")
	}
	if completedAt != nil {
		t.Errorf("completed_at = %v, want NULL — a revived run has not completed", *completedAt)
	}
	if len(cursor) == 0 {
		t.Error("the resume point was dropped; the mailbox would be re-read from the top and its counters inflated")
	}
}

// Reconnecting answers a refused credential and nothing else. A run ended by our
// own bug would be revived by every successful sync — one every couple of
// minutes — so it is left where it is. This is the guard that keeps the revival
// from becoming a loop, and it is why the class is the predicate rather than a
// counter: `internal` does not become true again because a mailbox synced.
func TestAFaultTheCredentialDidNotCauseIsNotRevived(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	connectFixtureConnection(ctx, t, reg)
	startFixtureBackfill(ctx, t, reg)
	connID, backfillID := truncatedRun(ctx, t, "internal")

	if err := reg.SyncOnce(ctx, connID); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}

	if status, _, _ := runState(ctx, t, backfillID); status != "error" {
		t.Errorf("status = %q, want %q — a sync succeeding says nothing about our own bug", status, "error")
	}
}

// A run that never committed a page has no resume point, and reviving it would
// re-page the window from the top — which the cursor reader refuses for the
// reason it states: the counters would count the same messages twice. Starting
// that mailbox over is a decision with a cost, so it stays the operator's.
func TestARunWithNoResumePointIsNotRevived(t *testing.T) {
	ctx, reg, _, _ := newCaptureRegistryFixture(t)
	connectFixtureConnection(ctx, t, reg)
	startFixtureBackfill(ctx, t, reg)
	connID, backfillID := truncatedRun(ctx, t, "auth")

	owner, _ := setupCaptureDB(t)
	if _, err := owner.Exec(ctx,
		`UPDATE capture_backfill SET cursor = NULL WHERE id = $1`, backfillID); err != nil {
		t.Fatalf("clearing the cursor: %v", err)
	}

	if err := reg.SyncOnce(ctx, connID); err != nil {
		t.Fatalf("SyncOnce: %v", err)
	}

	if status, _, _ := runState(ctx, t, backfillID); status != "error" {
		t.Errorf("status = %q, want %q — with no cursor there is nothing to resume from", status, "error")
	}
}
