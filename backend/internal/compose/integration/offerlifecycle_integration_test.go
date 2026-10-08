// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An offer past its valid-until date cannot be sent or accepted, and an
// accepted offer, which prices its deal, cannot be archived out from under it.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func offerValidUntil(t *testing.T, e *apptest.AppEnv, dealID, validUntil string) string {
	t.Helper()
	var offer struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/deals/"+dealID+"/offers", AnyMap{
		"currency": "EUR", "source": "manual", "valid_until": validUntil,
		"line_items": []AnyMap{{"description": "Retainer", "quantity": 1, "unit_price_minor": 500000, "tax_rate": 19.0}},
	}, nil, &offer); status != http.StatusCreated {
		t.Fatalf("create offer valid until %s → %d", validUntil, status)
	}
	return offer.ID
}

func TestALapsedOfferCanNeitherBeSentNorAccepted(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	dealID := offerFixture(t, e)

	lapsed := offerValidUntil(t, e, dealID, "2020-01-01")
	var fault faultBody
	if status := e.Call(t, "POST", "/v1/offers/"+lapsed+"/send", nil, nil, &fault); status != http.StatusUnprocessableEntity {
		t.Errorf("sending an offer valid until 2020 → %d %+v, want 422", status, fault)
	}

	// An offer that lapses after it was sent is refused at acceptance, which is
	// the moment it would re-price the deal.
	late := offerValidUntil(t, e, dealID, "2099-01-01")
	if status := e.Call(t, "POST", "/v1/offers/"+late+"/send", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("sending an offer valid until 2099 → %d, want 200", status)
	}
	if _, err := e.Owner.Exec(t.Context(), `UPDATE offer SET valid_until = '2020-01-01' WHERE id = $1`, late); err != nil {
		t.Fatalf("lapsing the sent offer: %v", err)
	}
	var deal struct {
		AmountMinor *int64 `json:"amount_minor"`
	}
	e.Call(t, "GET", "/v1/deals/"+dealID, nil, nil, &deal)
	priced := deal.AmountMinor
	if status := e.Call(t, "POST", "/v1/offers/"+late+"/accept", nil, nil, &fault); status != http.StatusUnprocessableEntity {
		t.Errorf("accepting a lapsed offer → %d %+v, want 422", status, fault)
	}
	var after struct {
		AmountMinor *int64 `json:"amount_minor"`
	}
	e.Call(t, "GET", "/v1/deals/"+dealID, nil, nil, &after)
	if (priced == nil) != (after.AmountMinor == nil) || (priced != nil && *priced != *after.AmountMinor) {
		t.Errorf("the refused acceptance still re-priced the deal: %v → %v", priced, after.AmountMinor)
	}
}

func TestAnAcceptedOfferCannotBeArchivedWhileItPricesTheDeal(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	dealID := offerFixture(t, e)
	offerID := offerValidUntil(t, e, dealID, "2099-01-01")
	if status := e.Call(t, "POST", "/v1/offers/"+offerID+"/send", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("send → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/offers/"+offerID+"/accept", nil, nil, nil); status != http.StatusOK {
		t.Fatalf("accept → %d", status)
	}

	var fault faultBody
	if status := e.Call(t, "DELETE", "/v1/offers/"+offerID, nil, nil, &fault); status != http.StatusConflict {
		t.Errorf("archiving the accepted offer → %d %+v, want 409", status, fault)
	}
	if status := e.Call(t, "GET", "/v1/offers/"+offerID, nil, nil, nil); status != http.StatusOK {
		t.Errorf("the accepted offer reads %d after the refused archive, want 200", status)
	}
}
