// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Accepting an offer re-prices the deal, and the deal's own History has to say
// what the value was before and what it is now.
func TestAcceptingAnOfferRecordsTheDealsOldAndNewValueInItsHistory(t *testing.T) {
	t.Parallel()
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := e.SeedDeal(t, "Repriced by offer", pipeline, open, &e.Rep1)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, OfferDeskPerms)

	amount, currency := int64(100000), "EUR"
	if _, err := e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		AmountMinor: &amount, Currency: &currency,
	}); err != nil {
		t.Fatalf("pricing the deal: %v", err)
	}
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
	net := *accepted.NetMinor

	field := "amount_minor"
	changes, err := privacy.ListFieldHistory(e.Admin(), e.DB(), privacy.FieldHistoryFilter{
		EntityType: "deal", EntityID: deal, Field: &field,
	})
	if err != nil {
		t.Fatalf("reading the deal's changes: %v", err)
	}
	newest := changes.Entries[0]
	if newest.OldValue == nil || *newest.OldValue != "100000" || newest.NewValue == nil || *newest.NewValue != strconv.FormatInt(net, 10) {
		t.Fatalf("the newest change is %v -> %v, want 100000 -> the accepted net %d", newest.OldValue, newest.NewValue, net)
	}

	full, err := privacy.ListRecordHistory(e.Admin(), e.DB(), privacy.RecordHistoryFilter{
		EntityType: "deal", EntityID: deal, Limit: new(20),
	})
	if err != nil {
		t.Fatalf("reading the deal's full history: %v", err)
	}
	if line := full.Entries[0]; line.Action != "update" || line.Before[field] == nil || line.After[field] == nil {
		t.Errorf("the newest line is %+v, want an update with the amount before and after", line)
	}

	if got := e.WsCount(t, `
		SELECT count(*) FROM event_outbox o JOIN audit_log a ON a.id::text = o.envelope->'trace'->>'audit_log_id'
		 WHERE o.envelope->>'type' = 'deal.updated' AND a.entity_type = 'deal' AND a.entity_id = $1 AND a.evidence ? 'offer_id'`,
		deal); got == 0 {
		t.Error("the deal.updated event is not linked to the deal's own audit row")
	}
}
