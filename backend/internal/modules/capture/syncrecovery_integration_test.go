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
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// throttledHistory imports its whole window in one page, while its regular sync
// is refused for as long as limited holds.
type throttledHistory struct {
	fixtureConnector
	limited *bool
}

func (c throttledHistory) Sync(_ context.Context, _ connector.Auth, cursor connector.Cursor, _ connector.Sink) (connector.Cursor, error) {
	if *c.limited {
		return nil, &connector.RateLimitedError{RetryAfter: 4 * time.Hour}
	}
	return cursor, nil
}

func (throttledHistory) EstimateBackfill(context.Context, connector.Auth, time.Time) (connector.BackfillEstimate, error) {
	return connector.BackfillEstimate{Messages: 1}, nil
}

func (throttledHistory) BackfillPage(context.Context, connector.Auth, time.Time, string, connector.Sink) (connector.BackfillPageResult, error) {
	return connector.BackfillPageResult{Scanned: 1, Captured: 1}, nil
}

// throttledImport is a connection whose regular sync was just rate limited and
// whose history import then finished.
func throttledImport(t *testing.T, limited *bool) (context.Context, *capture.Registry, ids.UUID) {
	t.Helper()
	ctx, reg, run := admissionRunWith(t, throttledHistory{limited: limited})
	at := time.Date(2026, 11, 18, 9, 0, 0, 0, time.UTC)
	reg.SetClock(func() time.Time { return at })
	owner, _ := setupCaptureDB(t)
	var conn ids.UUID
	if err := owner.QueryRow(ctx, `SELECT connection_id FROM capture_backfill WHERE id = @run`,
		pgx.NamedArgs{"run": run}).Scan(&conn); err != nil {
		t.Fatal(err)
	}
	if err := reg.SyncOnce(ctx, conn); !errors.Is(err, connector.ErrRateLimited) {
		t.Fatalf("the throttled sync answered %v, want the provider's rate limit", err)
	}
	if due := syncDueNow(ctx, t, conn); due {
		t.Fatal("a rate-limited sync is due at once; the case needs it backed off")
	}
	queue := &recordedBackfillQueue{generations: map[ids.UUID]int64{}}
	if err := reg.DispatchBackfills(ctx, queue.scheduler(), false); err != nil {
		t.Fatal(err)
	}
	if _, completed, _, err := reg.RunBackfillExecution(ctx, run, queue.generations[run], queue.scheduler()); err != nil || !completed {
		t.Fatalf("the one-page import completed=%v: %v", completed, err)
	}
	return ctx, reg, conn
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

func TestAFinishedImportMakesARateLimitedSyncDueAtOnce(t *testing.T) {
	limited := true
	ctx, reg, conn := throttledImport(t, &limited)
	if !syncDueNow(ctx, t, conn) {
		t.Fatal("the finished import left the rate-limited sync on its backoff")
	}
	if got := concernOf(ctx, t, reg, conn); got != capture.ConcernSyncFailing {
		t.Fatalf("concern before the next sync = %q, want %q: the import alone clears no failure", got, capture.ConcernSyncFailing)
	}

	limited = false
	if err := reg.SyncOnce(ctx, conn); err != nil {
		t.Fatalf("the sync after the import: %v", err)
	}
	if got := concernOf(ctx, t, reg, conn); got != "" {
		t.Fatalf("concern after a good sync = %q, want none", got)
	}
}

func TestASyncStillLimitedAfterTheImportKeepsItsConcern(t *testing.T) {
	limited := true
	ctx, reg, conn := throttledImport(t, &limited)
	if err := reg.SyncOnce(ctx, conn); !errors.Is(err, connector.ErrRateLimited) {
		t.Fatalf("the sync after the import answered %v, want the rate limit again", err)
	}
	if syncDueNow(ctx, t, conn) {
		t.Fatal("a sync refused again went back to due at once instead of its backoff")
	}
	if got := concernOf(ctx, t, reg, conn); got != capture.ConcernSyncFailing {
		t.Fatalf("concern = %q, want %q", got, capture.ConcernSyncFailing)
	}
}
