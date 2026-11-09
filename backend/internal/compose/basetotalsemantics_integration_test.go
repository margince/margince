// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a total says about money it could not price.
//
// A converted amount is null where no rate reaches it, and a SQL aggregate
// skips nulls. So a sum over a group holding one reads as the whole group.
//
// Two answers are right, for different aggregates, and these run both against
// Postgres rather than asserting the SQL they render.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/moneytotal"
)

// pricedAndNot is three priced amounts and two that carry money no rate
// reaches, as a group an aggregate runs over.
const pricedAndNot = `(VALUES (100::bigint, 100::bigint), (200, 200), (300, 300),
	                        (NULL, 400), (NULL, 500)) AS v(base, native)`

// fullyPriced is a group whose every amount has a rate.
const fullyPriced = `(VALUES (100::bigint, 100::bigint), (300, 300)) AS v(base, native)`

// totalOver renders one aggregate over a group and answers what Postgres makes
// of it. A null answer comes back as the empty string.
func totalOver(t *testing.T, e *integration.Env, group, expression string) string {
	t.Helper()
	var out *string
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT (`+expression+`)::text FROM `+group).Scan(&out); err != nil {
		t.Fatalf("running %s: %v", expression, err)
	}
	if out == nil {
		return ""
	}
	return *out
}

func TestASumSaysWhatItCouldPriceAndAStrictTotalSaysNothing(t *testing.T) {
	e := integration.Setup(t)
	unpriced := deals.UnpricedValueCountSQL("v.base", "v.native", "TRUE")

	// The sum keeps its documented answer: the priced part, with the unpriced
	// count beside it for the caller to read.
	if got := totalOver(t, e, pricedAndNot, moneytotal.FinalizeBaseTotalSQL("sum(v.base)", unpriced)); got != "600" {
		t.Errorf("the sum answered %q, want 600, the three amounts it could price", got)
	}
	if got := totalOver(t, e, pricedAndNot, unpriced); got != "2" {
		t.Errorf("the coverage count answered %q, want the two it could not price", got)
	}

	// An average has no zero to fall back on. A group holding an unpriced
	// amount has no average, rather than the average of the rest.
	if got := totalOver(t, e, pricedAndNot, moneytotal.StrictBaseTotalSQL("avg(v.base)", unpriced)); got != "" {
		t.Errorf("the average answered %q, want nothing: 200 is the average of the "+
			"three it could price, read as the average of five", got)
	}
	if got := totalOver(t, e, pricedAndNot, moneytotal.StrictBaseTotalSQL("max(v.base)", unpriced)); got != "" {
		t.Errorf("the maximum answered %q, want nothing: the largest amount may be "+
			"one of the two no rate reaches", got)
	}
}

func TestAFullyPricedGroupStillAnswers(t *testing.T) {
	e := integration.Setup(t)
	strict := moneytotal.StrictBaseTotalSQL("avg(v.base)", deals.UnpricedValueCountSQL("v.base", "v.native", "TRUE"))
	if got := totalOver(t, e, fullyPriced, strict); got != "200.0000000000000000" {
		t.Errorf("a fully priced group answered %q, want its real average", got)
	}
}
