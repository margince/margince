// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestARecordedRequestKeepsItsDealScopeAndSendsNothing(t *testing.T) {
	c := setupConsent(t)
	stages := apptest.DiscoverSeededPipeline(t, c.AppEnv)
	requested := apptest.StakeOnOpenDeal(t, c.AppEnv, "Requested negotiation", stages, c.contactID)
	other := apptest.StakeOnOpenDeal(t, c.AppEnv, "Other negotiation", stages, c.contactID)
	send := func(deal string) int {
		return c.Call(t, "POST", "/v1/emails", AnyMap{
			"to": []string{"subject@consent.test"}, "subject": "Next steps", "body": "The next step in our negotiation.",
			"communication_context": "active_deal_followup",
			"links":                 []AnyMap{{"entity_type": "contact", "entity_id": c.contactID}, {"entity_type": "deal", "entity_id": deal}},
		}, nil, nil)
	}
	if status := send(requested); status != http.StatusConflict {
		t.Fatalf("a deal alone answered %d, want 409", status)
	}
	if status := c.Call(t, "POST", "/v1/contacts/"+c.contactID+"/consent/qualifying-events", AnyMap{
		"kind": "requested_by_subject", "note": "They asked by phone to continue this negotiation.",
		"occurred_at": "2020-03-02T09:00:00Z", "about_deal": requested,
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("recording the request answered %d, want 201", status)
	}
	var deliveries, grants, overrides, instructions int
	if err := c.Owner.QueryRow(t.Context(), `SELECT
 (SELECT count(*) FROM comms_outbound), (SELECT count(*) FROM contact_consent),
 (SELECT count(*) FROM communication_override), (SELECT count(*) FROM communication_instruction)`).
		Scan(&deliveries, &grants, &overrides, &instructions); err != nil {
		t.Fatal(err)
	}
	if deliveries != 0 || grants != 0 || overrides != 0 || instructions != 0 {
		t.Fatalf("recording a fact wrote deliveries=%d consent=%d overrides=%d instructions=%d", deliveries, grants, overrides, instructions)
	}
	if status := send(requested); status != http.StatusAccepted {
		t.Fatalf("the requested negotiation answered %d, want 202", status)
	}
	if status := send(other); status != http.StatusConflict {
		t.Fatalf("a different negotiation answered %d, want 409", status)
	}
}
