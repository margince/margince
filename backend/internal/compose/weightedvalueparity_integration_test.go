// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Weighted value (formulas §6, AC-F1) is computed TWICE and the two are not
// interchangeable. The report engine folds it into an SQL aggregate, because
// deals-by-stage has no per-deal rows on the client to round; the account
// roll-up computes it in Go over big.Int, because amount_minor may reach the
// bigint bounds and a native multiply wraps before the ÷100 ever widens it. Neither can become the other.
//
// So the obligation is not one implementation — it is that the two ANSWER THE
// SAME, and that they refuse the same input rather than one of them wrapping.
// This is that gate. The SQL side is the production constant itself, not a
// copy of it: weightedAmountMinorExpr is embedded verbatim, over a one-row
// derived table standing in for the deal and its stage, so an edit to the
// expression is what runs here.

import (
	"context"
	"errors"
	"fmt"
	"github.com/margince/margince/backend/internal/modules/deals"
	"math"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// numericValueOutOfRange is the SQLSTATE a ::bigint cast raises when the
// numeric it narrows does not fit. Named because the refusal half of this
// parity is about WHICH failure each side gives: an error either side is fine,
// a silently wrapped money total is not.
const numericValueOutOfRange = "22003"

func TestTheTwoSpellingsOfWeightedValueAgree(t *testing.T) {
	conn := connectForExpressionParity(t)
	// The two column references the expression carries, supplied by a derived
	// table each, so the constant under test needs no rewriting to run.
	query := fmt.Sprintf(
		`SELECT %s FROM (SELECT $1::bigint AS amount_minor) t, (SELECT $2::int AS win_probability) s`,
		weightedAmountMinorExpr)

	cases := []struct {
		name        string
		amountMinor int64
		probability int
	}{
		{"an exact quotient", 100_000, 50},
		{"a positive half, which must round away from zero", 1, 50},
		{"a negative half, which must round away from zero the other way", -1, 50},
		{"a positive one-and-a-half", 3, 50},
		{"a negative one-and-a-half", -3, 50},
		{"a zero probability is a real zero, not a null", 123_456, 0},
		{"a full probability passes the amount through", 123_456, 100},
		{"a zero amount at a live probability", 0, 75},
		// The reason the Go side is big.Int at all: amount_minor sits at the
		// top of int64's range, the true answer is representable, and a native
		// multiply wraps before the division would have widened it.
		{"the largest amount the column can hold, passed through", math.MaxInt64, 100},
		{"the smallest amount the column can hold, passed through", math.MinInt64, 100},
		// Rounding at the extreme: MaxInt64 is odd, so half of it is not an
		// integer and the two sides must agree about which way the last minor
		// unit goes. MinInt64 is even, so its half is exact — that case is not
		// the rounding decision mirrored, it is the SIGN carried to the bottom
		// of the range, where a native negation would be the thing to overflow.
		{"half of the largest amount, where the last unit is decided by rounding", math.MaxInt64, 50},
		{"half of the smallest amount, which is exact and only tests the sign", math.MinInt64, 50},
		// The case that separates exact scaling from a numeric DIVISION, which
		// computes to a selected scale and rounds there. The exact product is
		// 4230000000000000016.45; a quotient this large is rendered at one
		// decimal as …16.5, and round() then lifts it to …17 — a minor unit
		// above the truth, on a figure a forecast puts in front of a buyer.
		// Every case above has a quotient exact at one decimal, so this is the
		// only one that can see the difference.
		{"a quotient too large for a division's selected scale to hold two decimals", 9_000_000_000_000_000_035, 47},
		{"the same quotient mirrored below zero", -9_000_000_000_000_000_035, 47},
		// Its neighbour on the other side of the halfway point, so a scaling
		// that rounded the wrong way ROUTINELY is caught too, not only one
		// that double-rounds.
		{"a quotient just past the halfway point at the same magnitude", 9_000_000_000_000_000_055, 47},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			want, err := deals.WeightedValue(tc.amountMinor, tc.probability)
			if err != nil {
				t.Fatalf("deals.WeightedValue(%d, %d): %v", tc.amountMinor, tc.probability, err)
			}
			var got int64
			if err := conn.QueryRow(context.Background(), query, tc.amountMinor, tc.probability).Scan(&got); err != nil {
				t.Fatalf("evaluating weightedAmountMinorExpr over (%d, %d): %v", tc.amountMinor, tc.probability, err)
			}
			if got != want {
				t.Errorf("the report engine and the account roll-up disagree about the weighted value of "+
					"%d minor units at %d%%: SQL says %d, Go says %d. One screen would show each",
					tc.amountMinor, tc.probability, got, want)
			}
		})
	}
}

// The refusal half, and it runs in BOTH directions on purpose.
//
// win_probability is DB-CHECKed to [0,100], so this input cannot arise from a
// stored row — which is exactly why it is worth asserting. The property under
// test is that neither spelling answers with a wrapped number when the true
// result does not fit: the Go side refuses in words, the SQL side refuses with
// a range error from its own narrowing cast, and a change that made either one
// return quietly would be a lie about money that no report would flag.
func TestNeitherSpellingOfWeightedValueWrapsWhenTheResultDoesNotFit(t *testing.T) {
	conn := connectForExpressionParity(t)
	query := fmt.Sprintf(
		`SELECT %s FROM (SELECT $1::bigint AS amount_minor) t, (SELECT $2::int AS win_probability) s`,
		weightedAmountMinorExpr)

	const beyondTheColumn = 300
	// deals.ErrWeightedValueOutOfRange specifically, not any error: a domain guard
	// added in front of the multiply would refuse this input too, and the test
	// would stay green with the overflow check itself deleted.
	if _, err := deals.WeightedValue(math.MaxInt64, beyondTheColumn); !errors.Is(err, deals.ErrWeightedValueOutOfRange) {
		t.Errorf("the Go spelling answered %v for a weighted value that cannot fit int64; expected its own "+
			"out-of-range refusal, so what is proven is the arithmetic and not a guard in front of it", err)
	}

	var got int64
	err := conn.QueryRow(context.Background(), query, int64(math.MaxInt64), beyondTheColumn).Scan(&got)
	if err == nil {
		t.Fatalf("the SQL spelling returned %d for a weighted value that cannot fit bigint instead of refusing", got)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != numericValueOutOfRange {
		t.Errorf("the SQL spelling refused with %v; expected a %s range error from its narrowing cast, so "+
			"the refusal is the arithmetic's rather than a query that failed for some other reason",
			err, numericValueOutOfRange)
	}
}

// connectForExpressionParity opens one connection for evaluating an SQL
// expression over literals. It reads no table, so it needs neither the schema
// reset nor the workspace GUC every row-reading suite in this package sets up.
func connectForExpressionParity(t *testing.T) *pgx.Conn {
	t.Helper()
	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to the test database: %v", err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing the parity connection: %v", err)
		}
	})
	return conn
}
