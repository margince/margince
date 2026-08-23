// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// What a human approves must bind to the record they were shown.
//
// The version pin is taken server-side for every version-pinnable record
// type (approvals.versionTables), never from the caller's own If-Match
// header — a header the contract declares optional, so an agent staging
// without one must still pin to the row's current state rather than opt
// out of the binding. `offer` is the sharp case: DELETE /v1/offers/{id}
// (the archive_record 🟡 tool's REST twin) carries no body, so its
// diff_hash is a CONSTANT for that offer id, while the line-item routes
// underneath it are auto_execute — the agent could rewrite the priced
// terms between the human's approval and the redemption and archive (or,
// with a different verb, ship) the offer at its own number instead of the
// one the human was shown.
//
// This drives that exact sequence over real HTTP: stage, approve, rewrite
// underneath, redeem — the redemption must refuse on the version mismatch.

import (
	"net/http"
	"testing"

	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
)

func TestAnAgentDraftsPricesAndArchivesAnOfferOnItsOwnPassport(t *testing.T) {
	e := apptest.SetupApp(t)
	e.Slug = "offers-pin"
	apptest.BootstrapWorkspaceSession(t, e, "Offers Pin", "pin@fable.test", "Admin")
	dealID := offerFixture(t, e)

	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", apptest.AnyMap{
		// The version-pin race this test proves happens inside archive_record's
		// approval, so the passport must be able to spend `write` to get there.
		"label": "pin agent", "scopes": []string{"read", "write"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}

	var offer struct {
		ID        string `json:"id"`
		Version   int64  `json:"version"`
		LineItems []struct {
			ID string `json:"id"`
		} `json:"line_items"`
	}
	if status := e.Call(t, "POST", "/v1/deals/"+dealID+"/offers", apptest.AnyMap{
		"currency": "EUR", "source": "mcp",
		"line_items": []apptest.AnyMap{{"description": "Pilot", "quantity": 1, "unit_price_minor": 250000, "tax_rate": 19.0}},
	}, bearer, &offer); status != http.StatusCreated {
		t.Fatalf("agent offer draft → %d", status)
	}
	if len(offer.LineItems) != 1 {
		t.Fatalf("draft carries %d line items, want 1", len(offer.LineItems))
	}

	// The agent prices the terms, then archives — neither asks a human.
	if status := e.Call(t, "PATCH", "/v1/offers/"+offer.ID+"/line-items/"+offer.LineItems[0].ID, apptest.AnyMap{
		"unit_price_minor": 100,
	}, bearer, nil); status != http.StatusOK {
		t.Fatalf("agent line-item rewrite → %d", status)
	}
	if status := e.Call(t, "DELETE", "/v1/offers/"+offer.ID, nil, bearer, nil); status == http.StatusForbidden {
		t.Fatal("agent offer archive → 403 — a passport archives what its holder could archive unaided")
	}

	// An archive sets archived_at; the offer's own status stays whatever it was.
	var archived bool
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT archived_at IS NOT NULL FROM offer WHERE id = $1`, offer.ID).Scan(&archived); err != nil {
		t.Fatalf("reading the offer back: %v", err)
	}
	if !archived {
		t.Error("the offer is still live after the agent archived it")
	}
}
