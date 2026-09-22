// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The tier a partner is on is frozen onto every commission entry accrued under
// it, so a mask that stopped at the partner record would be a mask that does
// not mask.

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The cross-object half. The tier is frozen onto every entry accrued under it,
// so a mask that stopped at the partner record would be a mask that does not
// mask. Neither module knows the other: the reach is declared in auth's group
// closure and each withholds its own field.
func TestAMaskedSeatReadsACommissionEntryWithoutTheTierItWasAccruedOn(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	winAndDeliver(t, e, fx)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, marginMaskedSeat())

	page, err := fx.ledger.List(ctx, commissions.ListInput{DealID: &fx.deal})
	if err != nil {
		t.Fatalf("listing the ledger: %v", err)
	}
	if len(page.Data) == 0 {
		t.Fatal("the ledger came back empty; the win accrued nothing to mask")
	}
	for _, entry := range page.Data {
		assertAccruedTierWithheld(t, entry)
	}

	one, err := fx.ledger.GetCommissionEntry(ctx, ids.From[ids.CommissionEntryKind](ids.UUID(page.Data[0].Id)))
	if err != nil {
		t.Fatalf("reading one ledger entry: %v", err)
	}
	assertAccruedTierWithheld(t, one)
}

func assertAccruedTierWithheld(t *testing.T, e crmcontracts.CommissionEntry) {
	t.Helper()
	if e.MarginTierAtAccrual != nil {
		t.Errorf("margin_tier_at_accrual = %v, want the partner mask to reach it", *e.MarginTierAtAccrual)
	}
	if e.MaskedFields == nil || !slices.Contains(*e.MaskedFields, "margin_tier_at_accrual") {
		t.Errorf("masked_fields = %v, want margin_tier_at_accrual named", e.MaskedFields)
	}
}
