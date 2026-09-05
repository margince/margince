// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A percentile below the sample floor answers NULL, and the count beside it
// still answers.
//
// The guard is SQL — a CASE around percentile_cont — so a unit test over this
// package's Go cannot see it. Postgres computes a median over three rows
// happily, which is exactly why the refusal is written rather than assumed.

import (
	"context"
	"os"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/analyticsquery"
	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestAPercentileBelowTheSampleFloorIsBlankRatherThanWrong(t *testing.T) {
	t.Parallel()
	ownerDSN := os.Getenv("MARGINCE_TEST_DSN")
	if ownerDSN == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail " +
			"loudly, they never skip)")
	}
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, ownerDSN)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing owner connection: %v", err)
		}
	})
	if err := testdb.EnsureSchema(ctx, owner); err != nil {
		t.Fatal(err)
	}

	// The expression this spec actually renders, over a values list standing in
	// for the deals. Running the REAL SQL rather than a paraphrase: a test that
	// wrote its own CASE would prove that test's arithmetic, not the engine's.
	sample := func(values ...int) (*float64, int64) {
		t.Helper()
		rows := make([]any, 0, len(values))
		holders := ""
		for i, v := range values {
			if i > 0 {
				holders += ", "
			}
			holders += "($" + strconv.Itoa(i+1) + "::int)"
			rows = append(rows, v)
		}
		if len(values) == 0 {
			holders = "(NULL::int)"
			rows = nil
		}
		var median *float64
		var count int64
		query := `SELECT (CASE WHEN count(d.days) >= ` + strconv.Itoa(analyticsquery.PercentileSampleFloor) +
			` THEN percentile_cont(0.5) WITHIN GROUP (ORDER BY d.days) END),
			         count(d.days)
			  FROM (VALUES ` + holders + `) AS d(days)`
		if err := owner.QueryRow(ctx, query, rows...).Scan(&median, &count); err != nil {
			t.Fatalf("evaluating the percentile expression: %v", err)
		}
		return median, count
	}

	// Three deals. A median here is one deal's value wearing a statistic's
	// name, and a reader comparing teams would take the smallest team's
	// outlier for its norm.
	median, count := sample(2, 40, 300)
	if median != nil {
		t.Errorf("a median over 3 values answered %v — below the floor it must be blank", *median)
	}
	// The COUNT still answers. That is what makes the blank informative rather
	// than a failure: the reader sees n=3 and knows why there is no typical.
	if count != 3 {
		t.Errorf("the count answered %d, want 3 — a blank percentile must not take the "+
			"count with it", count)
	}

	// At the floor it answers. Without this half a guard that refused EVERY
	// sample would pass the assertion above while the column stayed
	// permanently empty.
	median, count = sample(1, 2, 3, 4, 5)
	if median == nil {
		t.Fatal("a median over 5 values was blank — the floor is a minimum, not a bar")
	}
	if *median != 3 {
		t.Errorf("the median of 1..5 is %v, want 3", *median)
	}
	if count != 5 {
		t.Errorf("the count answered %d, want 5", count)
	}
}
