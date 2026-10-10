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
)

func TestARecordCreateWithoutASourceIsRefusedOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := createdID(t, e, "/v1/contacts", AnyMap{"full_name": "Origin Contact", "source": "manual"})
	company := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Origin Co", "source": "manual"})

	creates := map[string]struct {
		path string
		body AnyMap
	}{
		"lead":         {"/v1/leads", AnyMap{"full_name": "Lee Lead"}},
		"relationship": {"/v1/relationships", AnyMap{"kind": "employment", "contact_id": contact, "company_id": company}},
		"note":         {"/v1/activities", AnyMap{"kind": "note", "body": "no origin"}},
		"task":         {"/v1/tasks", AnyMap{"subject": "no origin"}},
		"product":      {"/v1/products", AnyMap{"name": "Seat", "unit_price_minor": 1000, "currency": "EUR"}},
	}
	for name, create := range creates {
		for label, stated := range map[string]AnyMap{"absent": {}, "empty": {"source": ""}, "spaces": {"source": "   "}} {
			t.Run(name+"/"+label, func(t *testing.T) {
				body := maps.Clone(create.body)
				maps.Copy(body, stated)
				var refusal namedRefusal
				if status := e.CallVerbatim(t, "POST", create.path, body, nil, &refusal); status != http.StatusUnprocessableEntity {
					t.Fatalf("POST %s %v → %d, want 422", create.path, body, status)
				}
				if !refusal.names("source") {
					t.Errorf("POST %s refusal names %+v, want field source", create.path, refusal.Details.Errors)
				}
			})
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
