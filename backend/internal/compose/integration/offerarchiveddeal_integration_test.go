// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An archived deal opens read-only, and the offers it was made are what it was.
// Listing them is a read: only an id no deal holds answers not found. A deal
// carries no row-scope restriction of its own, so that is the one miss to try.
func TestAnArchivedDealStillListsItsOffers(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Northgate rollout", pipeline, open, &e.Rep1))
	admin := e.Admin()
	desk := e.As(e.Rep1, []ids.UUID{e.Team1}, offerDeskCompanyPerms)
	description, price := "Retainer", int64(10000)
	if _, err := e.Deals.CreateOffer(desk, deal, deals.CreateOfferInput{
		Currency: "EUR", Source: "manual",
		LineItems: []deals.OfferLineInputRow{{Description: &description, Quantity: "1", UnitPriceMinor: &price}},
	}); err != nil {
		t.Fatalf("creating the offer: %v", err)
	}
	if _, err := e.Deals.ArchiveDeal(admin, deal, nil); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}

	page, _, err := e.Deals.ListDealOffers(desk, deal, deals.ListDealOffersInput{})
	if err != nil {
		t.Fatalf("listing the offers of an archived deal: %v", err)
	}
	if len(page) != 1 {
		t.Fatalf("the archived deal lists %d offer(s), want 1", len(page))
	}

	if _, _, err := e.Deals.ListDealOffers(desk, ids.New[ids.DealKind](), deals.ListDealOffersInput{}); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("listing the offers of a deal that does not exist: got %v, want not found", err)
	}
}
