// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A money value is an integer count of the units ONE currency defines. A deal
// update records only the fields the request set, so an edit to the amount
// alone carries no currency — and putting that amount back under a currency
// somebody has since changed states a price that never existed, wrong by the
// scale difference values.MinorUnitExceptions() encodes and wrong silently,
// because the figure is plausible in both denominations.
//
// The unit test beside moneyMovedUnderIt covers the branches that need no
// trail. This is the one that needs it: a real later currency change, and the
// refusal that follows.

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnAmountRestoreIsRefusedWhenTheCurrencyMovedUnderIt(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.Admin()
	pipeline, open, _ := integration.DealFixture(t, e)

	amount, currency := int64(150_000), "EUR"
	deal, err := e.Deals.CreateDeal(ctx, deals.CreateDealInput{
		Name: "Scaled", PipelineID: pipeline, StageID: open, Source: "manual",
		AmountMinor: &amount, Currency: &currency,
	})
	if err != nil {
		t.Fatalf("seed the deal through the real writer: %v", err)
	}
	id := ids.UUID(deal.Id)
	dealID := ids.From[ids.DealKind](id)

	// The amount alone. The store permits it — the pair rule only refuses a
	// result with one half null — so the audit row names amount_minor and no
	// currency to compare against.
	raised := int64(225_000)
	if _, err := e.Deals.UpdateDeal(ctx, dealID, deals.UpdateDealInput{AmountMinor: &raised}); err != nil {
		t.Fatalf("raise the amount: %v", err)
	}
	entry := latestAuditRowID(t, e, "deal", id, "update")

	// The currency alone, afterwards. 225000 minor units meant euro cents when
	// it was written; it does not mean the same thing now.
	moved := "JPY"
	if _, err := e.Deals.UpdateDeal(ctx, dealID, deals.UpdateDealInput{Currency: &moved}); err != nil {
		t.Fatalf("move the currency: %v", err)
	}

	seam := NewRestoreSeam(e.Pool, NewDispatcher(NewProvider(e.Pool),
		NewOverlayProvider(e.Pool, failClosedOverlayMeter(), nil), e.Pool))
	_, err = seam.Restore(ctx, "deal", id, entry, currentVersion(t, e, "deal", id))

	var refusal RefusedRestore
	if !errors.As(err, &refusal) {
		t.Fatalf("putting the amount back answered %v; the currency moved under it "+
			"and the old figure is a price that never existed", err)
	}
	if refusal.Reason != ReasonSuperseded {
		t.Errorf("reason = %q, want %q", refusal.Reason, ReasonSuperseded)
	}
	// Named as the field the caller asked to put back, not as the sibling that
	// moved: the person pressed Undo on an amount change.
	if refusal.Detail != "amount_minor" {
		t.Errorf("detail = %q, want the field the caller asked about", refusal.Detail)
	}
}
