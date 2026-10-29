// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// A retried create under its key answers the first contact and the first
// employer, and makes no second company.
func TestAContactCreatedWithACompanyReplaysItsFirstAnswer(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	headers := map[string]string{"Idempotency-Key": "contact-with-company"}
	body := AnyMap{"source": "manual", "full_name": "Replay Hire", "company_name": "Replay Employer GmbH"}

	var first, again AnyMap
	if status := e.Call(t, http.MethodPost, "/v1/contacts", body, headers, &first); status != http.StatusCreated {
		t.Fatalf("create = %d %v", status, first)
	}
	employer, _ := first["employer"].(map[string]any)
	if employer["company_name"] != "Replay Employer GmbH" {
		t.Fatalf("employer = %v, want the company the request named", first["employer"])
	}
	if status := e.Call(t, http.MethodPost, "/v1/contacts", body, headers, &again); status != http.StatusCreated {
		t.Fatalf("replay = %d %v", status, again)
	}
	replayed, _ := again["employer"].(map[string]any)
	if again["id"] != first["id"] || replayed["company_id"] != employer["company_id"] {
		t.Errorf("replay answered %v at %v, want %v at %v", again["id"], replayed["company_id"], first["id"], employer["company_id"])
	}
	var companies AnyMap
	if status := e.Call(t, http.MethodGet, "/v1/companies?q=Replay+Employer", nil, nil, &companies); status != http.StatusOK {
		t.Fatalf("company list = %d %v", status, companies)
	}
	if rows, _ := companies["data"].([]any); len(rows) != 1 {
		t.Errorf("%d companies after a replayed create, want 1", len(rows))
	}
}

// The wire the company page, the New contact form and the signature card call.
// Candidates, the address's company, the link, and create-and-link on a held domain.
func TestTheEmployerLinkRoutesAnswerTheirContract(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	var company, contact, colleague AnyMap
	if status := e.Call(t, http.MethodPost, "/v1/companies", AnyMap{
		"source": "manual", "display_name": "Wireco", "domains": []AnyMap{{"domain": "wireco.test", "is_primary": true}},
	}, nil, &company); status != http.StatusCreated {
		t.Fatalf("company = %d %v", status, company)
	}
	for _, seed := range []struct {
		into  *AnyMap
		name  string
		email string
	}{{&contact, "Wanda Wire", "wanda@wireco.test"}, {&colleague, "Walt Wire", "walt@wireco.test"}} {
		if status := e.Call(t, http.MethodPost, "/v1/contacts", AnyMap{
			"source": "manual", "full_name": seed.name, "emails": []AnyMap{{"email": seed.email, "email_type": "work", "is_primary": true}},
		}, nil, seed.into); status != http.StatusCreated {
			t.Fatalf("contact %s = %d %v", seed.name, status, *seed.into)
		}
	}
	companyPath := "/v1/companies/" + company["id"].(string)

	var candidates AnyMap
	if status := e.Call(t, http.MethodGet, companyPath+"/link-candidates", nil, nil, &candidates); status != http.StatusOK {
		t.Fatalf("candidates = %d %v", status, candidates)
	}
	if rows, _ := candidates["data"].([]any); len(rows) != 2 || candidates["truncated"] != false {
		t.Fatalf("candidates = %v, want both contacts and truncated=false", candidates)
	}

	for address, want := range map[string]any{"nora@eu.wireco.test": company["id"], "nora@gmail.com": nil} {
		var answer AnyMap
		if status := e.Call(t, http.MethodGet, "/v1/companies/for-address?email="+url.QueryEscape(address), nil, nil, &answer); status != http.StatusOK {
			t.Fatalf("company for %s = %d %v", address, status, answer)
		}
		found, _ := answer["company"].(map[string]any)
		if found["company_id"] != want {
			t.Errorf("company for %s = %v, want %v", address, answer, want)
		}
	}

	var linked AnyMap
	if status := e.Call(t, http.MethodPost, companyPath+"/link-contacts", AnyMap{"contact_ids": []any{contact["id"]}},
		map[string]string{"Idempotency-Key": "link-wanda"}, &linked); status != http.StatusCreated {
		t.Fatalf("link = %d %v", status, linked)
	}
	if rows, _ := linked["data"].([]any); len(rows) != 1 || rows[0].(map[string]any)["contact_id"] != contact["id"] {
		t.Errorf("linked = %v, want one edge for the ticked contact", linked)
	}
	var replayed AnyMap
	if status := e.Call(t, http.MethodPost, companyPath+"/link-contacts", AnyMap{"contact_ids": []any{contact["id"]}},
		map[string]string{"Idempotency-Key": "link-wanda"}, &replayed); status != http.StatusCreated {
		t.Fatalf("link replay = %d %v, want the first answer", status, replayed)
	}

	var refused AnyMap
	if status := e.Call(t, http.MethodPost, "/v1/contacts/"+colleague["id"].(string)+"/employer",
		AnyMap{"company_name": "Wireco Again"}, map[string]string{"Idempotency-Key": "near-duplicate"}, &refused); status != http.StatusConflict {
		t.Fatalf("create and link on a held domain = %d %v, want 409", status, refused)
	}
	details, _ := refused["details"].(map[string]any)
	if refused["code"] != "duplicate_domain" || details["existing_id"] != company["id"] {
		t.Errorf("refusal = %v, want duplicate_domain naming %v", refused, company["id"])
	}
}
