// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A mailbox import that stops part way continues from where it stopped.
//
// Starting again from the newest message re-reads everything already captured,
// which on a large mailbox under the provider's rate limits costs hours. So a
// page fault the engine can retry is retried, a start after a run that ended on
// an error continues that run with its counts, a page token the provider no
// longer accepts walks the window again, and a message the capture refuses is
// counted rather than ending the run.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// failRunAfterOnePage pages one page of a flaky run, then lets the next page end
// it on a refused credential: a run that stopped on an error, part way, with
// its cursor and counts committed.
func failRunAfterOnePage(t *testing.T, e *integration.SearchEnv) (*capturemod.Registry, ids.UUID) {
	t.Helper()
	registry, runID := startFlakyBackfill(t, e, []error{nil, connector.ErrAuthRejected})
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err != nil {
		t.Fatalf("first page: %v", err)
	}
	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err == nil {
		t.Fatal("the refused credential did not end the run")
	}
	if status, scanned, _, cursor := readBackfillRow(t, e, runID); status != "error" || scanned != 10 || len(cursor) == 0 {
		t.Fatalf("fixture: status=%s scanned=%d cursor=%q, want an error run that stopped after one page", status, scanned, cursor)
	}
	return registry, runID
}

func TestAStartAfterAFailedRunContinuesItFromWhereItStopped(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, runID := failRunAfterOnePage(t, e)
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	rep := ids.From[ids.UserKind](e.Rep1)

	status, err := registry.BackfillStatus(grantCtx, "gmail", rep)
	if err != nil || status == nil || !status.Resumable {
		t.Fatalf("status = %+v, %v; a run that stopped on an error with its cursor must say it can be continued", status, err)
	}

	run, err := registry.StartBackfill(grantCtx, "gmail", rep, 6, connector.BackfillEstimate{Messages: 25}, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}
	if run.ID != runID {
		t.Fatalf("the start opened run %s; it must continue the failed run %s", run.ID, runID)
	}
	if run.Status != "queued" || run.Scanned != 10 || run.Captured != 9 {
		t.Fatalf("continued run = status %s, scanned %d, captured %d; want queued with its counts kept (10/9)", run.Status, run.Scanned, run.Captured)
	}
	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err != nil {
		t.Fatalf("continued page: %v", err)
	}
	if _, scanned, captured, _ := readBackfillRow(t, e, runID); scanned != 20 || captured != 18 {
		t.Fatalf("after the continued page scanned=%d captured=%d, want 20/18: it must page on from the stored cursor", scanned, captured)
	}
}

func TestStartingOverReadsTheWindowAgainInANewRun(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, failed := failRunAfterOnePage(t, e)
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})

	run, err := registry.StartBackfillOver(grantCtx, "gmail", ids.From[ids.UserKind](e.Rep1), 6, connector.BackfillEstimate{Messages: 25}, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfillOver: %v", err)
	}
	if run.ID == failed || run.Scanned != 0 || len(run.Cursor) != 0 {
		t.Fatalf("start over = %+v; want a new run from the top of the window", run)
	}
	if status, _, _, _ := readBackfillRow(t, e, failed); status != "error" {
		t.Fatalf("the failed run became %s; starting over leaves it as it was", status)
	}
}

func TestAnInternalPageFaultIsRetriedInsteadOfEndingTheRun(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, runID := startFlakyBackfill(t, e, []error{errors.New("capture: a fault of our own")})
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)

	done, _, retryAfter, err := registry.RunBackfillStep(wsCtx, runID)
	if err == nil || done || retryAfter <= 0 {
		t.Fatalf("done=%v retryAfter=%v err=%v; an internal page fault must leave the run live and come back later", done, retryAfter, err)
	}
	status, failures, errClass, cursor := readBackfillRetryState(t, e, runID)
	if status != "running" || failures != 1 || errClass == nil || *errClass != "internal" || len(cursor) != 0 {
		t.Fatalf("after the fault: status=%s failures=%d class=%v cursor=%q; want running/1/internal and no cursor", status, failures, errClass, cursor)
	}
	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, failures, _, _ = readBackfillRetryState(t, e, runID); failures != 0 {
		t.Fatalf("consecutive_failures = %d after a committed page, want 0", failures)
	}
}

func TestARejectedPageTokenWalksTheWindowAgain(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, runID := startFlakyBackfill(t, e, []error{nil, connector.ErrCursorGone, nil, connector.ErrCursorGone})
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err != nil {
		t.Fatalf("first page: %v", err)
	}

	done, _, retryAfter, err := registry.RunBackfillStep(wsCtx, runID)
	if err == nil || done || retryAfter <= 0 {
		t.Fatalf("done=%v retryAfter=%v err=%v; a rejected token must restart the walk, not end the run", done, retryAfter, err)
	}
	status, scanned, _, cursor := readBackfillRow(t, e, runID)
	if status != "running" || scanned != 0 || len(cursor) != 0 {
		t.Fatalf("after the rejected token: status=%s scanned=%d cursor=%q; want a live run back at the top of the window", status, scanned, cursor)
	}
	if _, _, _, err := registry.RunBackfillStep(wsCtx, runID); err != nil {
		t.Fatalf("first page again: %v", err)
	}
	if _, scanned, _, _ = readBackfillRow(t, e, runID); scanned != 10 {
		t.Fatalf("scanned = %d after walking the first page again, want 10", scanned)
	}

	// Once per run: a provider that rejects the token again is not answered
	// by walking the window forever.
	if done, _, _, err := registry.RunBackfillStep(wsCtx, runID); err == nil || !done {
		t.Fatalf("second rejection: done=%v err=%v, want the run ended", done, err)
	}
	if status, _, _, _ := readBackfillRow(t, e, runID); status != "error" {
		t.Fatalf("status = %s after a second rejected token, want error", status)
	}
}

// failingMessagesConnector serves pagedConnector's pages and reports two of
// each page's messages as ones the capture refused.
type failingMessagesConnector struct{ *pagedConnector }

func (f failingMessagesConnector) BackfillPage(ctx context.Context, auth connector.Auth, after time.Time, pageToken string, sink connector.Sink) (connector.BackfillPageResult, error) {
	res, err := f.pagedConnector.BackfillPage(ctx, auth, after, pageToken, sink)
	res.Failed = 2
	return res, err
}

func TestARunCountsTheMessagesItCouldNotCapture(t *testing.T) {
	e := integration.SetupSearch(t)
	registry := newTestCaptureRegistry(e, newTestKeyvault(t, e))
	registry.Register(failingMessagesConnector{&pagedConnector{messages: 25, pageSize: 10}})
	grantCtx := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(grantCtx, "gmail", connector.Auth("refresh")); err != nil {
		t.Fatalf("Connect: %v", err)
	}
	rep := ids.From[ids.UserKind](e.Rep1)
	run, err := registry.StartBackfill(grantCtx, "gmail", rep, 6, connector.BackfillEstimate{Messages: 25}, enqueueNothing)
	if err != nil {
		t.Fatalf("StartBackfill: %v", err)
	}
	wsCtx := principal.WithWorkspaceID(context.Background(), e.WS)
	if _, _, _, err := registry.RunBackfillStep(wsCtx, run.ID); err != nil {
		t.Fatalf("page: %v", err)
	}
	var failed int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `SELECT failed FROM capture_backfill WHERE id = $1`, run.ID).Scan(&failed)
	}); err != nil {
		t.Fatal(err)
	}
	status, err := registry.BackfillStatus(grantCtx, "gmail", rep)
	if err != nil {
		t.Fatal(err)
	}
	if failed != 2 || status.Failed != 2 || status.Status != "running" {
		t.Fatalf("failed column=%d, status read=%d, state=%s; want 2 counted and the run still live", failed, status.Failed, status.Status)
	}
}
