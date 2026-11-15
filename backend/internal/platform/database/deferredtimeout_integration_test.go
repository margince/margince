// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

func statementTimeout(ctx context.Context, t *testing.T, tx pgx.Tx) string {
	t.Helper()
	var timeout string
	if err := tx.QueryRow(ctx, `SELECT current_setting('statement_timeout')`).Scan(&timeout); err != nil {
		t.Fatal(err)
	}
	return timeout
}

// laneSeeing runs a lane with budget that reads the timeout in force inside it.
func laneSeeing(ctx context.Context, t *testing.T, tx pgx.Tx, budget time.Duration, inside *string) error {
	t.Helper()
	return database.WithDegradableRead(ctx, budget, func(ctx context.Context) error {
		*inside = statementTimeout(ctx, t, tx)
		return nil
	})
}

// Two lanes in a row on one budget send no timeout statement between them.
// Each still runs under its budget, and the page's timeout is back after them.
func TestAdjacentLanesOnOneBudgetSendNoTimeoutStatementsBetweenThem(t *testing.T) {
	meteredSnapshot(t, func(ctx context.Context, tx pgx.Tx) {
		page := statementTimeout(ctx, t, tx)
		var first, second string
		cost := exchangesOf(ctx, t, func(ctx context.Context) {
			if err := laneSeeing(ctx, t, tx, time.Second, &first); err != nil {
				t.Fatal(err)
			}
			if err := laneSeeing(ctx, t, tx, time.Second, &second); err != nil {
				t.Fatal(err)
			}
			if after := statementTimeout(ctx, t, tx); after != page {
				t.Errorf("after the lanes the timeout is %q, want the page's %q", after, page)
			}
		})
		if first != "1s" || second != "1s" {
			t.Errorf("the lanes ran under %q and %q, want 1s each", first, second)
		}
		// The first lane reads the page's timeout, opens and sets its budget. The
		// second opens with its savepoint alone. Without the shortcut it would also
		// restore, read and set: 13 statements.
		if cost.Statements != 10 || cost.Exchanges != 5 {
			t.Errorf("two lanes and a read after them sent %d statements in %d exchanges, want 10 in 5", cost.Statements, cost.Exchanges)
		}
	})
}

// A lane leaves the timeout as the next statement must find it, whether it ran
// on another budget, failed, or sent nothing.
func TestALaneLeavesTheTimeoutTheNextStatementExpects(t *testing.T) {
	meteredSnapshot(t, func(ctx context.Context, tx pgx.Tx) {
		page := statementTimeout(ctx, t, tx)
		var inside string
		if err := laneSeeing(ctx, t, tx, time.Second, &inside); err != nil {
			t.Fatal(err)
		}
		if err := laneSeeing(ctx, t, tx, 2*time.Second, &inside); err != nil || inside != "2s" {
			t.Errorf("a lane on another budget ran under %q (%v), want 2s", inside, err)
		}
		if after := statementTimeout(ctx, t, tx); after != page {
			t.Errorf("after a lane on another budget the timeout is %q, want %q", after, page)
		}

		if err := laneSeeing(ctx, t, tx, time.Second, &inside); err != nil {
			t.Fatal(err)
		}
		failed := errors.New("the lane gave up")
		if err := database.WithDegradableRead(ctx, time.Second, func(ctx context.Context) error {
			selectOne(ctx, t, tx)
			return failed
		}); !errors.Is(err, failed) {
			t.Fatalf("the failing lane answered %v", err)
		}
		if after := statementTimeout(ctx, t, tx); after != page {
			t.Errorf("after a failed lane the timeout is %q, want %q", after, page)
		}

		if err := laneSeeing(ctx, t, tx, time.Second, &inside); err != nil {
			t.Fatal(err)
		}
		if err := database.WithDegradableRead(ctx, time.Second, func(context.Context) error { return nil }); err != nil {
			t.Fatal(err)
		}
		if after := statementTimeout(ctx, t, tx); after != page {
			t.Errorf("after a lane that sent nothing the timeout is %q, want %q", after, page)
		}
	})
}

// An inner lane puts back its outer lane's budget, and a budget bound by the
// caller is read again rather than assumed.
func TestALanePutsBackTheTimeoutItFound(t *testing.T) {
	meteredSnapshot(t, func(ctx context.Context, tx pgx.Tx) {
		page := statementTimeout(ctx, t, tx)
		var inner, outerAfter string
		if err := database.WithDegradableRead(ctx, 2*time.Second, func(ctx context.Context) error {
			if err := laneSeeing(ctx, t, tx, time.Second, &inner); err != nil {
				return err
			}
			outerAfter = statementTimeout(ctx, t, tx)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if inner != "1s" || outerAfter != "2s" {
			t.Errorf("the inner lane ran under %q and left %q, want 1s then the outer 2s", inner, outerAfter)
		}
		if after := statementTimeout(ctx, t, tx); after != page {
			t.Errorf("after the outer lane the timeout is %q, want %q", after, page)
		}

		if err := database.BoundStatement(ctx, tx, 3*time.Second); err != nil {
			t.Fatal(err)
		}
		var inside string
		if err := laneSeeing(ctx, t, tx, time.Second, &inside); err != nil || inside != "1s" {
			t.Errorf("a lane after a bound statement ran under %q (%v), want 1s", inside, err)
		}
		if after := statementTimeout(ctx, t, tx); after != "3s" {
			t.Errorf("after the lane the timeout is %q, want the bound 3s", after)
		}
	})
}

// The outer lane here took over the restore of the lane before it. An inner
// lane that fails leaves the outer budget, and then the page's timeout.
func TestAFailedInnerLaneKeepsTheRestoreItsOuterLaneHolds(t *testing.T) {
	meteredSnapshot(t, func(ctx context.Context, tx pgx.Tx) {
		page := statementTimeout(ctx, t, tx)
		var first string
		if err := laneSeeing(ctx, t, tx, time.Second, &first); err != nil {
			t.Fatal(err)
		}
		failed := errors.New("the inner lane gave up")
		var afterInner string
		if err := database.WithDegradableRead(ctx, time.Second, func(ctx context.Context) error {
			selectOne(ctx, t, tx)
			if err := database.WithDegradableRead(ctx, 2*time.Second, func(ctx context.Context) error {
				selectOne(ctx, t, tx)
				return failed
			}); !errors.Is(err, failed) {
				t.Errorf("the inner lane answered %v", err)
			}
			afterInner = statementTimeout(ctx, t, tx)
			return nil
		}); err != nil {
			t.Fatal(err)
		}
		if afterInner != "1s" {
			t.Errorf("after the failed inner lane the outer lane runs under %q, want 1s", afterInner)
		}
		if after := statementTimeout(ctx, t, tx); after != page {
			t.Errorf("after the outer lane the timeout is %q, want %q", after, page)
		}
	})
}
