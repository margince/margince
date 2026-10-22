// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The record writes the first test of this kind leaves out, over HTTP. They are
// the contract, product, offer and activity routes and the contract renewal. A
// key misspelt inside a nested row is held too, and so is the order of refusals.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestTheRemainingRecordRoutesRefuseAMisspeltKey(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)

	company := createAndID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Surface Co"})
	contract := createAndID(t, e, "/v1/contracts", AnyMap{"company_id": company, "title": "MSA", "value_basis": "total"})
	product := createAndID(t, e, "/v1/products", AnyMap{"source": "manual", "name": "Day", "unit_price_minor": 100, "currency": "EUR"})
	deal := createAndID(t, e, "/v1/deals", AnyMap{
		"name": "Surface deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
	})
	offer := createAndID(t, e, "/v1/deals/"+deal+"/offers", AnyMap{"currency": "EUR", "source": "manual"})
	activity := createAndID(t, e, "/v1/activities", AnyMap{"kind": "note", "body": "probe"})

	writes := []struct {
		name, method, path string
		body               AnyMap
	}{
		{"contract create", "POST", "/v1/contracts", AnyMap{"company_id": company, "title": "x", "value_basis": "total", "titel": "y"}},
		{"contract edit", "PATCH", "/v1/contracts/" + contract, AnyMap{"titel": "y"}},
		{"contract renewal", "POST", "/v1/contracts/" + contract + "/renewal", AnyMap{"title": "x", "value_basis": "total", "titel": "y"}},
		{"product create", "POST", "/v1/products", AnyMap{"source": "manual", "name": "x", "unit_price_minor": 1, "currency": "EUR", "nam": "y"}},
		{"product edit", "PATCH", "/v1/products/" + product, AnyMap{"nam": "y"}},
		{"product edit naming a cf_ key", "PATCH", "/v1/products/" + product, AnyMap{"cf_tier": "gold"}},
		{"offer create", "POST", "/v1/deals/" + deal + "/offers", AnyMap{"currency": "EUR", "source": "manual", "valid_unti": "2099-01-01"}},
		{"offer edit", "PATCH", "/v1/offers/" + offer, AnyMap{"intro_txt": "y"}},
		{"offer edit naming a cf_ key", "PATCH", "/v1/offers/" + offer, AnyMap{"cf_tier": "gold"}},
		{"activity create", "POST", "/v1/activities", AnyMap{"kind": "note", "body": "x", "bdy": "y"}},
		{"activity edit", "PATCH", "/v1/activities/" + activity, AnyMap{"bdy": "y"}},
		{"contact create, nested row", "POST", "/v1/contacts", AnyMap{
			"source": "manual", "full_name": "x", "emails": []AnyMap{{"emial": "a@b.example"}},
		}},
	}
	for _, write := range writes {
		t.Run(write.name, func(t *testing.T) {
			var problem problemBody
			headers := map[string]string{}
			if write.method == "PATCH" {
				headers["If-Match"] = "1"
			}
			status := e.Call(t, write.method, write.path, write.body, headers, &problem)
			if status != http.StatusUnprocessableEntity || !refusesUnknownField(problem) {
				t.Fatalf("%s %s → %d %+v, want 422 unknown_field", write.method, write.path, status, problem)
			}
		})
	}
}

// A client that echoes a field the server wrote (id, version, captured_by,
// created_at) on a PATCH names a field the route does not take. The web client
// sends only the fields the user changed.
func TestAPatchEchoingReadFieldsIsRefused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	contact := createAndID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Echo"})
	for _, field := range []string{"id", "version", "captured_by", "created_at"} {
		var problem problemBody
		status := e.Call(t, "PATCH", "/v1/contacts/"+contact, AnyMap{"full_name": "Echo", field: "x"},
			map[string]string{"If-Match": "1"}, &problem)
		if status != http.StatusUnprocessableEntity || !refusesUnknownField(problem) {
			t.Errorf("a PATCH echoing %s → %d %+v, want 422 unknown_field", field, status, problem)
		}
	}
}

// A missing record answers 404 and a stale version 409 before a refused custom
// field answers 422, so a caller learns the larger fault first.
func TestAStaleOrMissingRecordIsAnsweredBeforeARefusedCustomField(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	stages := apptest.DiscoverSeededPipeline(t, e)
	contact := createAndID(t, e, "/v1/contacts", AnyMap{"source": "manual", "full_name": "Order"})
	company := createAndID(t, e, "/v1/companies", AnyMap{"source": "manual", "display_name": "Order Co"})
	lead := createAndID(t, e, "/v1/leads", AnyMap{"full_name": "Order Lead", "source": "manual"})
	deal := createAndID(t, e, "/v1/deals", AnyMap{
		"name": "Order deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "source": "manual",
	})
	stale := map[string]string{"If-Match": "99"}
	for name, path := range map[string]string{
		"contact": "/v1/contacts/" + contact, "company": "/v1/companies/" + company,
		"lead": "/v1/leads/" + lead, "deal": "/v1/deals/" + deal,
	} {
		if status := e.Call(t, "PATCH", path, AnyMap{"cf_never_defined": "x"}, stale, nil); status != http.StatusConflict {
			t.Errorf("%s: a stale version beside a refused cf_ key → %d, want 409", name, status)
		}
	}
	if status := e.Call(t, "PATCH", "/v1/activities/01a15c79-76f9-7650-be50-4b1a27c6b2b0",
		AnyMap{"cf_never_defined": "x"}, nil, nil); status != http.StatusNotFound {
		t.Errorf("a missing activity beside a refused cf_ key → %d, want 404", status)
	}
}
