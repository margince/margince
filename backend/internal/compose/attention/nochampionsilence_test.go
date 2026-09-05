// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// `false` is never sent, so silence is the only negative.
//
// `no_champion` carries a finding or it carries nothing. The contract says a
// covered committee is absent alongside the three other non-findings, which
// makes the four indistinguishable on the wire on purpose: a reader who cannot
// see the seats must not be able to tell a covered committee from one they were
// refused, because telling them apart IS the disclosure.
//
// That rule lives in two places — the field's description in crm.yaml and
// noChampionOf, which maps Covered to nil — and a description enforces nothing.
// This is the half that fails.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Both answers the seam can give, and neither may reach the wire as a false.
//
// The loop is the point: a covered committee (false) and an uncovered one (true)
// take different routes through noChampionOf, and only the second may surface.
// Asserting the covered case alone would leave a projection free to invert the
// finding, which is the same defect wearing the other sign.
func TestTheCoverageSeamMintsNoStatedFalse(t *testing.T) {
	t.Parallel()
	for _, covered := range []bool{true, false} {
		row := oneRiskRow(t, withDealFacts(RiskyDeal{
			DealID: ids.NewV7(), Name: "Fleet retrofit", QuietDays: 19,
			NoChampion: &covered,
		}))
		if row.Deal == nil {
			t.Fatal("the row carries no deal facts at all")
		}
		if row.Deal.NoChampion != nil && !*row.Deal.NoChampion {
			t.Errorf("a stated false reached the wire from a %v answer", covered)
		}
	}
}

// The same rule on the lane's own projection, which is the other producer.
//
// `AttentionDealFacts` and `WorklistDealFacts` carry one field under one rule,
// and two functions build them. Holding only the worklist side would leave the
// lane free to send the `false` its own contract text says is never sent — the
// shape where a rule is fixed at one call site and the sibling keeps the defect.
func TestTheLaneSendsNoStatedFalseEither(t *testing.T) {
	t.Parallel()
	for _, covered := range []bool{true, false} {
		minor, currency := int64(4_000_000), "EUR"
		item := riskItem(RiskyDeal{
			DealID: ids.NewV7(), Name: "Fleet retrofit", QuietDays: 19,
			AmountMinor: &minor, Currency: &currency, NoChampion: &covered,
		})
		if item.Deal == nil {
			t.Fatal("the lane row carries no deal facts at all")
		}
		if item.Deal.NoChampion != nil && !*item.Deal.NoChampion {
			t.Errorf("the lane sent a stated false from a %v answer", covered)
		}
	}
}
