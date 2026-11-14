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

// exchangesOf is how many exchanges and statements step sent on the snapshot.
func exchangesOf(ctx context.Context, t *testing.T, step func(context.Context)) database.StatementCost {
	t.Helper()
	meter, ok := database.StatementMeterOf(ctx)
	if !ok {
		t.Fatal("no statement meter on the read")
	}
	before := meter.Cost()
	step(ctx)
	return meter.Cost().Minus(before)
}

func meteredSnapshot(t *testing.T, fn func(context.Context, pgx.Tx)) {
	t.Helper()
	ctx, pool := laneEnv(t)
	ctx, _ = database.WithStatementMeter(ctx)
	onSnapshot(ctx, t, pool, fn)
}

func selectOne(ctx context.Context, t *testing.T, tx pgx.Tx) {
	t.Helper()
	var one int
	if err := tx.QueryRow(ctx, `SELECT 1`).Scan(&one); err != nil || one != 1 {
		t.Fatalf("SELECT 1 answered %d, %v", one, err)
	}
}

// A lane's savepoint goes in one exchange ahead of its first statement, and its
// close rides on the next statement after it. A lane that sends nothing sends
// none of them.
func TestALanesSessionStatementsRideOnTheExchangesAroundIt(t *testing.T) {
	meteredSnapshot(t, func(ctx context.Context, tx pgx.Tx) {
		selectOne(ctx, t, tx)
		empty := exchangesOf(ctx, t, func(ctx context.Context) {
			if err := database.WithDegradableRead(ctx, time.Second, func(context.Context) error { return nil }); err != nil {
				t.Fatal(err)
			}
			selectOne(ctx, t, tx)
		})
		if empty.Exchanges != 1 || empty.Statements != 1 {
			t.Errorf("an empty lane and one read sent %d statements in %d exchanges, want 1 in 1", empty.Statements, empty.Exchanges)
		}
		lane := exchangesOf(ctx, t, func(ctx context.Context) {
			if err := database.WithDegradableRead(ctx, time.Second, func(ctx context.Context) error {
				selectOne(ctx, t, tx)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			selectOne(ctx, t, tx)
		})
		// Open is three statements and close two, around the lane's own read.
		if lane.Exchanges != 3 || lane.Statements != 7 {
			t.Errorf("a one-read lane and one read after it sent %d statements in %d exchanges, want 7 in 3", lane.Statements, lane.Exchanges)
		}
	})
}

// An inner lane that fails rolls back to its own savepoint, even while it
// holds a pending release of a lane inside it. The outer lane's work stays.
func TestAFailedInnerLaneRollsBackOnlyItsOwnWork(t *testing.T) {
	meteredSnapshot(t, func(ctx context.Context, tx pgx.Tx) {
		if _, err := tx.Exec(ctx, `CREATE TEMP TABLE lane_marks (mark text) ON COMMIT DROP`); err != nil {
			t.Fatal(err)
		}
		mark := func(ctx context.Context, name string) error {
			_, err := tx.Exec(ctx, `INSERT INTO lane_marks VALUES ($1)`, name)
			return err
		}
		inner := errors.New("the inner lane gave up")
		outer := database.WithDegradableRead(ctx, time.Second, func(ctx context.Context) error {
			if err := mark(ctx, "outer"); err != nil {
				return err
			}
			got := database.WithDegradableRead(ctx, time.Second, func(ctx context.Context) error {
				if err := mark(ctx, "inner"); err != nil {
					return err
				}
				if err := database.WithDegradableRead(ctx, time.Second, func(ctx context.Context) error { return mark(ctx, "innermost") }); err != nil {
					return err
				}
				return inner
			})
			if !errors.Is(got, inner) {
				t.Errorf("the inner lane answered %v, want its own error", got)
			}
			return nil
		})
		if outer != nil {
			t.Fatalf("the outer lane failed with its inner one: %v", outer)
		}
		var marks []string
		rows, err := tx.Query(ctx, `SELECT mark FROM lane_marks ORDER BY mark`)
		if err == nil {
			marks, err = pgx.CollectRows(rows, pgx.RowTo[string])
		}
		if err != nil || len(marks) != 1 || marks[0] != "outer" {
			t.Errorf("after the inner lane failed the marks are %v (%v), want only the outer lane's", marks, err)
		}
	})
}

// Plan reuse's mode is set for the statements inside it and put back after,
// whichever exchange carries the two.
func TestReusedPlansLeaveThePlanModeAsTheyFoundIt(t *testing.T) {
	meteredSnapshot(t, func(ctx context.Context, tx pgx.Tx) {
		mode := func() string {
			var current string
			if err := tx.QueryRow(ctx, `SELECT current_setting('plan_cache_mode')`).Scan(&current); err != nil {
				t.Fatal(err)
			}
			return current
		}
		before := mode()
		var inside string
		if err := database.ReuseGenericPlans(ctx, func(ctx context.Context) error {
			return tx.QueryRow(ctx, `SELECT current_setting('plan_cache_mode')`).Scan(&inside)
		}); err != nil {
			t.Fatal(err)
		}
		if inside != "force_generic_plan" {
			t.Errorf("inside plan reuse the mode is %q, want force_generic_plan", inside)
		}
		if after := mode(); after != before {
			t.Errorf("after plan reuse the mode is %q, want %q", after, before)
		}
	})
}

// A lane statement can fail before it runs, at prepare or while its arguments
// are encoded. The snapshot stays usable, because the lane's savepoint exists
// before that statement is prepared.
func TestALaneThatFailsBeforeItRunsLeavesTheSnapshotUsable(t *testing.T) {
	meteredSnapshot(t, func(ctx context.Context, tx pgx.Tx) {
		failures := map[string]func(context.Context) error{
			"prepare": func(ctx context.Context) error {
				_, err := tx.Exec(ctx, `SELECT * FROM lane_table_that_does_not_exist`)
				return err
			},
			"encode": func(ctx context.Context) error {
				_, err := tx.Exec(ctx, `SELECT $1::int`, make(chan int))
				return err
			},
		}
		for name, lane := range failures {
			if err := database.WithDegradableRead(ctx, time.Second, lane); err == nil {
				t.Errorf("%s: the failing lane answered no error", name)
			}
			selectOne(ctx, t, tx)
		}
	})
}
