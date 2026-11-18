// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture_test

// A history import that finishes proves the provider answers again. A regular
// sync it pushed into rate limiting must not wait out a four-hour backoff.
// So the import's last page makes that sync due at once. The failure record
// stays until the sync itself succeeds or fails again.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// throttledHistory imports its whole window in one page unless pageFault is
// set. Its regular sync fails with syncFault while that is set.
type throttledHistory struct {
	fixtureConnector
	syncFault *error
	pageFault error
}

func (c throttledHistory) Sync(_ context.Context, _ connector.Auth, cursor connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	if *c.syncFault != nil {
		return nil, *c.syncFault
	}
	return cursor, nil
}

func (throttledHistory) EstimateBackfill(context.Context, connector.Auth, time.Time) (connector.BackfillEstimate, error) {
	return connector.BackfillEstimate{Messages: 1}, nil
}

func (c throttledHistory) BackfillPage(context.Context, connector.Auth, time.Time, string, connector.Sink) (connector.BackfillPageResult, error) {
	if c.pageFault != nil {
		return connector.BackfillPageResult{}, c.pageFault
	}
	return connector.BackfillPageResult{Scanned: 1, Captured: 1}, nil
}

// throttledPeer is a second mailbox of the same member that runs no import.
type throttledPeer struct{ throttledHistory }

func (throttledPeer) Descriptor() connector.Descriptor {
	return connector.Descriptor{Name: haltingProvider, Version: "fixture"}
}

var rateLimit error = &connector.RateLimitedError{RetryAfter: 4 * time.Hour}

// importCase is a member whose two mailboxes both failed a sync, after which
// the import on conn ran to its end.
type importCase struct {
	ctx            context.Context
	reg            *capture.Registry
	conn, peer     ids.UUID
	connBackoff    time.Time
	peerBackoff    time.Time
	importFinished bool
}

func failedSyncsThenImport(t *testing.T, syncFault *error, pageFault error) importCase {
	t.Helper()
	history := throttledHistory{syncFault: syncFault, pageFault: pageFault}
	ctx, reg, run := admissionRunWith(t, history)
	reg.Register(throttledPeer{history})
	at := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	reg.SetClock(func() time.Time { return at })
	owner, _ := setupCaptureDB(t)
	c := importCase{ctx: ctx, reg: reg}
	if err := owner.QueryRow(ctx, `SELECT connection_id FROM capture_backfill WHERE id = @run`,
		pgx.NamedArgs{"run": run}).Scan(&c.conn); err != nil {
		t.Fatal(err)
	}
	if err := reg.SyncOnce(ctx, c.conn); err == nil {
		t.Fatal("the importing mailbox's sync succeeded; the case needs it failed")
	}
	if err := syncOnceOf(ctx, t, reg, haltingProvider); err == nil {
		t.Fatal("the peer mailbox's sync succeeded; the case needs it failed")
	}
	c.peer = connectionOf(ctx, t)
	c.connBackoff, c.peerBackoff = nextSyncAt(ctx, t, c.conn), nextSyncAt(ctx, t, c.peer)
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
		t.Fatal(err)
	}
	_, completed, _, err := reg.RunBackfillExecution(ctx, run, queue.generations[run], queue.scheduler())
	if pageFault == nil && (err != nil || !completed) {
		t.Fatalf("the one-page import completed=%v: %v", completed, err)
	}
	c.importFinished = completed
	return c
}

func nextSyncAt(ctx context.Context, t *testing.T, conn ids.UUID) time.Time {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	var next time.Time
	if err := owner.QueryRow(ctx, `SELECT next_sync_at FROM capture_sync_state WHERE connection_id = @id`,
		pgx.NamedArgs{"id": conn}).Scan(&next); err != nil {
		t.Fatal(err)
	}
	return next
}

func syncDueNow(ctx context.Context, t *testing.T, conn ids.UUID) bool {
	t.Helper()
	owner, _ := setupCaptureDB(t)
	var due bool
	if err := owner.QueryRow(ctx, `SELECT next_sync_at <= now() FROM capture_sync_state WHERE connection_id = @id`,
		pgx.NamedArgs{"id": conn}).Scan(&due); err != nil {
		t.Fatal(err)
	}
	return due
}

func concernOf(ctx context.Context, t *testing.T, reg *capture.Registry, conn ids.UUID) string {
	t.Helper()
	views, err := reg.Connections(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, view := range views {
		if view.ID == conn {
			return view.Concern
		}
	}
	t.Fatalf("connection %s is missing from its owner's list", conn)
	return ""
}

// assertBackoffKept fails unless conn still waits for the instant its own
// failure scheduled.
func assertBackoffKept(t *testing.T, c importCase, conn ids.UUID, backoff time.Time, who string) {
	t.Helper()
	if next := nextSyncAt(c.ctx, t, conn); !next.Equal(backoff) {
		t.Fatalf("%s: next sync moved from %s to %s", who, backoff, next)
	}
}

func TestAFinishedImportMakesAWeatheredSyncDueAtOnce(t *testing.T) {
	for name, fault := range map[string]error{"rate_limited": rateLimit, "unreachable": connector.ErrUnreachable} {
		t.Run(name, func(t *testing.T) {
			syncFault := fault
			c := failedSyncsThenImport(t, &syncFault, nil)
			if !syncDueNow(c.ctx, t, c.conn) {
				t.Fatal("the finished import left the failed sync on its backoff")
			}
			assertBackoffKept(t, c, c.peer, c.peerBackoff, "the member's other mailbox, which ran no import")
			if got := concernOf(c.ctx, t, c.reg, c.conn); got != capture.ConcernSyncFailing {
				t.Fatalf("concern before the next sync = %q, want %q: the import alone clears no failure", got, capture.ConcernSyncFailing)
			}

			syncFault = nil
			if err := c.reg.SyncOnce(c.ctx, c.conn); err != nil {
				t.Fatalf("the sync after the import: %v", err)
			}
			if got := concernOf(c.ctx, t, c.reg, c.conn); got != "" {
				t.Fatalf("concern after a good sync = %q, want none", got)
			}
		})
	}
}

func TestAFinishedImportLeavesAnInternalFailureOnItsBackoff(t *testing.T) {
	syncFault := errors.New("fixture defect")
	c := failedSyncsThenImport(t, &syncFault, nil)
	assertBackoffKept(t, c, c.conn, c.connBackoff, "an internal failure the import does not disprove")
}

func TestAnImportThatEndsInErrorLeavesTheBackoff(t *testing.T) {
	syncFault := rateLimit
	c := failedSyncsThenImport(t, &syncFault, apperrors.ErrPermissionDenied)
	if c.importFinished {
		t.Fatal("the refused import completed; the case needs it ended in error")
	}
	assertBackoffKept(t, c, c.conn, c.connBackoff, "an import that ended in error")
}

func TestASyncStillLimitedAfterTheImportKeepsItsConcern(t *testing.T) {
	syncFault := rateLimit
	c := failedSyncsThenImport(t, &syncFault, nil)
	if err := c.reg.SyncOnce(c.ctx, c.conn); !errors.Is(err, connector.ErrRateLimited) {
		t.Fatalf("the sync after the import answered %v, want the rate limit again", err)
	}
	if syncDueNow(c.ctx, t, c.conn) {
		t.Fatal("a sync refused again went back to due at once instead of its backoff")
	}
	if got := concernOf(c.ctx, t, c.reg, c.conn); got != capture.ConcernSyncFailing {
		t.Fatalf("concern = %q, want %q", got, capture.ConcernSyncFailing)
	}
}
