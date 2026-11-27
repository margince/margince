// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// taxedOffer drafts, sends and accepts a 19% offer of 238.20 net on the deal.
func taxedOffer(t *testing.T, e *Env, deal ids.UUID) crmcontracts.Offer {
	t.Helper()
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, OfferDeskPerms)
	description, price, taxRate := "Retainer", int64(23820), "19.00"
	created, err := e.Deals.CreateOffer(ctx, ids.From[ids.DealKind](deal), deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{
			Description: &description, Quantity: "1", UnitPriceMinor: &price, TaxRate: &taxRate,
		}},
	})
	if err != nil {
		t.Fatalf("create offer: %v", err)
	}
	offer := ids.From[ids.OfferKind](ids.UUID(created.Id))
	if _, err := e.Deals.SendOffer(ctx, offer, nil); err != nil {
		t.Fatalf("send offer: %v", err)
	}
	accepted, err := e.Deals.AcceptOffer(ctx, offer, nil)
	if err != nil {
		t.Fatalf("accept offer: %v", err)
	}
	if *accepted.NetMinor == *accepted.GrossMinor {
		t.Fatalf("the fixture offer carries no tax: net %d, gross %d", *accepted.NetMinor, *accepted.GrossMinor)
	}
	return accepted
}

// A deal closed before its offer was accepted is re-priced on the offer's net,
// and its frozen base value is converted from that net.
func TestAcceptingATaxedOfferOntoAClosedDealFreezesItsNet(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	pipeline, open, won := DealFixture(t, e)
	deal := e.SeedDeal(t, "Closed before the offer", pipeline, open, &e.Rep1)
	amount, currency, reason := int64(100000), "EUR", "imported"
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		AmountMinor: &amount, Currency: &currency,
	}); err != nil {
		t.Fatalf("pricing the deal: %v", err)
	}
	if _, err := e.Deals.AdvanceDeal(e.Admin(), ids.From[ids.DealKind](deal),
		deals.AdvanceDealInput{ToStageID: won, WonWithoutContractReason: &reason}); err != nil {
		t.Fatalf("closing the deal: %v", err)
	}

	accepted := taxedOffer(t, e, deal)

	got := int64(e.WsCount(t, `SELECT amount_minor FROM deal WHERE id = $1`, deal))
	base := int64(e.WsCount(t, `SELECT amount_minor_base FROM deal WHERE id = $1`, deal))
	status := e.WsScalar(t, `SELECT status FROM deal WHERE id = $1`, deal)
	if status != "won" || got != *accepted.NetMinor || base != *accepted.NetMinor {
		t.Fatalf("closed deal = %d (base %d, %s), want the offer's net %d on both", got, base, status, *accepted.NetMinor)
	}
}

// A legacy-mode deal that took an accepted taxed offer's net reads that basis,
// not "unknown". Only legacy-mode deals are read against their offers.
func TestADealPricedFromATaxedAcceptedOfferReadsTheNetBasis(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Net basis", pipeline, open, &e.Rep1)

	taxedOffer(t, e, deal)
	e.WsExec(t, `UPDATE deal SET commercial_mode = 'legacy' WHERE id = $1`, deal)

	commercial, err := e.Deals.GetDealLines(e.As(e.Rep1, []ids.UUID{e.Team1}, OfferDeskPerms), ids.From[ids.DealKind](deal))
	if err != nil {
		t.Fatalf("reading the deal's commercial view: %v", err)
	}
	if commercial.AmountBasis == nil || *commercial.AmountBasis != crmcontracts.DealCommercialAmountBasisOfferNet {
		t.Fatalf("amount basis = %v, want %q", commercial.AmountBasis, crmcontracts.DealCommercialAmountBasisOfferNet)
	}
}
