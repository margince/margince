// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A commission entry IS the partner's margin tier applied to money, and it
// publishes the arithmetic three ways: the tier frozen at accrual, the rate it
// became (tier2_20 is 2000bps), and the amount over the basis the rate produced
// that quotient from. Two of the three are `int` on the wire and cannot carry a
// withheld value, so a seat that may not read the tier reads no entry at all —
// the row leaves the ledger's reads rather than the column leaving the row.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/commissions"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The cross-object half. Neither module knows the other: the reach is declared
// in auth's group closure and the ledger asks it which of its rows survive.
func TestAMaskedSeatReadsNoCommissionEntryPricedFromTheTier(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	winAndDeliver(t, e, fx)
	masked := e.As(e.Rep1, []ids.UUID{e.Team1}, marginMaskedSeat())

	page, err := fx.ledger.List(masked, commissions.ListInput{DealID: &fx.deal})
	if err != nil {
		t.Fatalf("listing the ledger: %v", err)
	}
	if len(page.Data) != 0 {
		t.Errorf("the ledger listed %d entry(ies) to a seat that may not read the tier; "+
			"rate_bps IS the tier and amount over basis gives it back", len(page.Data))
	}

	// The summary is the same disclosure through a total: one partner with one
	// entry sums to that entry's amount, and the basis beside it is the deal's.
	summary, err := fx.ledger.Summary(masked)
	if err != nil {
		t.Fatalf("summarising the ledger: %v", err)
	}
	if len(summary.Data) != 0 {
		t.Errorf("the open-liability summary answered %d row(s) built from amounts the "+
			"caller may not read", len(summary.Data))
	}
}

// The single read answers not-found rather than refusing, which is the ledger's
// existing shape: a row outside the caller's reach must not be told apart from
// one that does not exist.
func TestAMaskedSeatAsksForOneEntryAndIsToldItIsNotThere(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	winAndDeliver(t, e, fx)

	entry := onlyLedgerEntry(t, e, fx)
	masked := e.As(e.Rep1, []ids.UUID{e.Team1}, marginMaskedSeat())

	if _, err := fx.ledger.GetCommissionEntry(masked, entry); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("reading one entry under the mask = %v, want ErrNotFound", err)
	}
}

// The other direction, or the exclusion above would pass over a ledger that
// answers nobody. The accrual itself is the same assertion from the write side:
// it runs as the system actor, which carries no mask, and every test here is
// built on an entry it wrote.
func TestAnUnmaskedSeatReadsTheLedgerWhole(t *testing.T) {
	e := Setup(t)
	fx := seedAccrualFixture(t, e, "tier2_20")
	winAndDeliver(t, e, fx)
	reader := e.As(e.AdminUser, nil, commissionAdminPerms)

	page, err := fx.ledger.List(reader, commissions.ListInput{DealID: &fx.deal})
	if err != nil {
		t.Fatalf("listing the ledger: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("the ledger listed %d entry(ies) to an unmasked seat, want the one accrued", len(page.Data))
	}
	got := page.Data[0]
	if got.MarginTierAtAccrual == nil || *got.MarginTierAtAccrual != "tier2_20" {
		t.Errorf("margin_tier_at_accrual = %v, want the tier it accrued on", got.MarginTierAtAccrual)
	}
	if got.RateBps != 2000 {
		t.Errorf("rate_bps = %d, want the 2000 the tier means", got.RateBps)
	}
	summary, err := fx.ledger.Summary(reader)
	if err != nil {
		t.Fatalf("summarising the ledger: %v", err)
	}
	if len(summary.Data) == 0 {
		t.Error("the open-liability summary is empty for a seat nothing is withheld from")
	}
}

// onlyLedgerEntry reads the id of the one entry the fixture's win accrued.
func onlyLedgerEntry(t *testing.T, e *Env, fx accrualFixture) ids.CommissionEntryID {
	t.Helper()
	page, err := fx.ledger.List(e.As(e.AdminUser, nil, commissionAdminPerms),
		commissions.ListInput{DealID: &fx.deal})
	if err != nil {
		t.Fatalf("listing the ledger: %v", err)
	}
	if len(page.Data) != 1 {
		t.Fatalf("the win accrued %d entry(ies), want one", len(page.Data))
	}
	return ids.From[ids.CommissionEntryKind](ids.UUID(page.Data[0].Id))
}
