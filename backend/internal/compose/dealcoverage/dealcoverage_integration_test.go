// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package dealcoverage_test

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/dealcoverage"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/shared/kernel/moneytotal"
)

// A group sums the deals that convert and counts the others apart, in
// Postgres and not in the SQL text. A group with nothing that converts has no
// total, and an empty group is zero.
func TestAPartialSumTotalsWhatConvertsAndCountsTheRestApart(t *testing.T) {
	e := integration.Setup(t)
	measures := dealcoverage.Measures("t.base")
	for _, tc := range []struct {
		name, rows      string
		sum             string
		noValue, noRate int
	}{
		{"some convert", "(100::bigint,100::bigint),(200,200),(300,NULL),(NULL,NULL)", "300", 1, 1},
		{"none convert", "(100::bigint,NULL::bigint),(200,NULL)", "", 0, 2},
		{"only a deal with no value", "(NULL::bigint,NULL::bigint)", "", 1, 0},
		{"no deals", "(1::bigint,1::bigint) LIMIT 0", "0", 0, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var sum *string
			var noValue, noRate int
			err := e.Pool.QueryRow(context.Background(), `SELECT (`+moneytotal.PartialSumSQL("sum(t.base)")+`)::text,
				count(`+measures[dealcoverage.MeasureNoValue]+`), count(`+measures[dealcoverage.MeasureNoRate]+`)
				FROM (VALUES `+tc.rows+`) AS t(amount_minor, base)`).Scan(&sum, &noValue, &noRate)
			if err != nil {
				t.Fatal(err)
			}
			got := ""
			if sum != nil {
				got = *sum
			}
			if got != tc.sum {
				t.Errorf("sum = %q, want %q", got, tc.sum)
			}
			if noValue != tc.noValue || noRate != tc.noRate {
				t.Errorf("no value %d, no rate %d, want %d and %d", noValue, noRate, tc.noValue, tc.noRate)
			}
		})
	}
}
