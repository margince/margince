// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The contract requires `source` on every create below, and a contact, company
// and deal already refuse a create without one.

import (
	"maps"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestARecordCreateWithoutASourceIsRefusedOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Origin Contact", "source": "manual"})
	company := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Origin Co", "source": "manual"})

	stages := apptest.DiscoverSeededPipeline(t, e)
	dealID := apptest.CreateOpenDeal(t, e, stages)
	roomlessDealID := createdID(t, e, "/v1/deals", AnyMap{
		"name": "Roomless deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "company_id": company, "source": "manual",
	})
	roomID := createdID(t, e, "/v1/deal-rooms", AnyMap{"deal_id": dealID, "title": "Origin room", "source": "manual"})

	// A document needs a stored attachment to succeed, so it is only refused here.
	creates := map[string]struct {
		path        string
		body        AnyMap
		refusalOnly bool
	}{
		"lead":         {"/v1/leads", AnyMap{"full_name": "Lee Lead"}, false},
		"relationship": {"/v1/relationships", AnyMap{"kind": "employment", "contact_id": contact, "company_id": company}, false},
		"note":         {"/v1/activities", AnyMap{"kind": "note", "body": "no origin"}, false},
		"task":         {"/v1/tasks", AnyMap{"subject": "no origin"}, false},
		"product":      {"/v1/products", AnyMap{"name": "Seat", "unit_price_minor": 1000, "currency": "EUR"}, false},
		"offer":        {"/v1/deals/" + dealID + "/offers", AnyMap{"currency": "EUR"}, false},
		"signal":       {"/v1/signals", AnyMap{"kind": "risk", "summary": "Quiet for three weeks"}, false},
		"deal room":    {"/v1/deal-rooms", AnyMap{"deal_id": roomlessDealID, "title": "Second room"}, false},
		"invitation": {
			"/v1/deal-rooms/" + roomID + "/participants",
			AnyMap{"full_name": "Laura Buyer", "email": "laura@buyer.example"},
			false,
		},
		"document": {
			"/v1/deal-rooms/" + roomID + "/documents",
			AnyMap{"attachment_id": ids.NewV7().String(), "group_key": "proposal"},
			true,
		},
	}
	for name, create := range creates {
		for label, stated := range map[string]AnyMap{"absent": {}, "empty": {"source": ""}, "spaces": {"source": "   "}} {
			t.Run(name+"/"+label, func(t *testing.T) {
				body := maps.Clone(create.body)
				maps.Copy(body, stated)
				if name == "lead" && label == "absent" {
					if status := e.CallVerbatim(t, "POST", create.path, body, nil, nil); status != http.StatusCreated {
						t.Fatalf("POST %s with no source → %d, want 201: a lead that names none is filed as manual", create.path, status)
					}
					return
				}
				var refusal namedRefusal
				if status := e.CallVerbatim(t, "POST", create.path, body, nil, &refusal); status != http.StatusUnprocessableEntity {
					t.Fatalf("POST %s %v → %d, want 422", create.path, body, status)
				}
				if !refusal.names("source") {
					t.Errorf("POST %s refusal names %+v, want field source", create.path, refusal.Details.Errors)
				}
			})
		}
		if create.refusalOnly {
			continue
		}
		t.Run(name+"/stated", func(t *testing.T) {
			body := maps.Clone(create.body)
			body["source"] = "manual"
			if status := e.CallVerbatim(t, "POST", create.path, body, nil, nil); status != http.StatusCreated {
				t.Errorf("POST %s with a source → %d, want 201", create.path, status)
			}
		})
	}
}
