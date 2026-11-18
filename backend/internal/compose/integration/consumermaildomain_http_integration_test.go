// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The workspace consumer-mail list over the real wire (CAP-PARAM-5): what an
// admin can say about a domain, what the surface refuses, and that a withdrawal
// really returns the workspace to the shipped baseline's answer.
//
// The list decides whether a domain can ever name a company, so its refusals
// matter as much as its writes: an entry the matcher could never read would
// leave an admin believing they had corrected something.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type consumerMailDomainDTO struct {
	ID     string `json:"id"`
	Domain string `json:"domain"`
	Kind   string `json:"kind"`
}

type consumerMailDomainListDTO struct {
	Data []consumerMailDomainDTO `json:"data"`
}

func TestConsumerMailDomainsOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// A workspace that has said nothing: the shipped baseline decides every
	// domain, and the list is empty rather than absent.
	var list consumerMailDomainListDTO
	if status := e.Call(t, "GET", "/v1/capture/consumer-mail-domains", nil, nil, &list); status != http.StatusOK {
		t.Fatalf("GET → %d, want 200", status)
	}
	if len(list.Data) != 0 {
		t.Fatalf("a fresh workspace lists %d entries, want 0", len(list.Data))
	}

	// A provider the shipped dataset missed, and a domain it wrongly claims.
	var added consumerMailDomainDTO
	if status := e.Call(t, "POST", "/v1/capture/consumer-mail-domains",
		map[string]string{"domain": "Regional-Mail.Example", "kind": "extra"}, nil, &added); status != http.StatusCreated {
		t.Fatalf("POST extra → %d, want 201", status)
	}
	// Stored in the form the matcher keys on, lower-cased: an entry that keeps
	// the caller's spelling is an entry that never matches anything.
	if added.Domain != "regional-mail.example" || added.Kind != "extra" {
		t.Fatalf("created %+v, want the normalized domain", added)
	}
	var carved consumerMailDomainDTO
	if status := e.Call(t, "POST", "/v1/capture/consumer-mail-domains",
		map[string]string{"domain": "mail.gmx.de", "kind": "never"}, nil, &carved); status != http.StatusCreated {
		t.Fatalf("POST never → %d, want 201", status)
	}
	// A subdomain means its registrable domain — the matcher reads eTLD+1.
	if carved.Domain != "gmx.de" {
		t.Fatalf("carve-out stored as %q, want gmx.de", carved.Domain)
	}

	if status := e.Call(t, "GET", "/v1/capture/consumer-mail-domains", nil, nil, &list); status != http.StatusOK {
		t.Fatalf("GET → %d, want 200", status)
	}
	if len(list.Data) != 2 {
		t.Fatalf("list holds %d entries, want 2", len(list.Data))
	}

	// Re-adding the same domain is the same entry, not a second row: a domain
	// cannot be both added and carved out.
	var readded consumerMailDomainDTO
	if status := e.Call(t, "POST", "/v1/capture/consumer-mail-domains",
		map[string]string{"domain": "regional-mail.example", "kind": "never"}, nil, &readded); status != http.StatusCreated {
		t.Fatalf("re-POST → %d, want 201", status)
	}
	if readded.ID != added.ID || readded.Kind != "never" {
		t.Fatalf("re-add = %+v, want the existing row %s switched to never", readded, added.ID)
	}

	// Withdrawing returns the workspace to the baseline's own answer, and doing
	// it twice is not an error — the caller's intent is already satisfied.
	if status := e.Call(t, "DELETE", "/v1/capture/consumer-mail-domains/"+added.ID, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("DELETE → %d, want 204", status)
	}
	if status := e.Call(t, "DELETE", "/v1/capture/consumer-mail-domains/"+added.ID, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("repeat DELETE → %d, want 204", status)
	}
	if status := e.Call(t, "GET", "/v1/capture/consumer-mail-domains", nil, nil, &list); status != http.StatusOK {
		t.Fatalf("GET → %d, want 200", status)
	}
	if len(list.Data) != 1 || list.Data[0].Domain != "gmx.de" {
		t.Fatalf("after the withdrawal the list is %+v, want the carve-out alone", list.Data)
	}
}

func TestConsumerMailDomainsRefusesWhatTheMatcherCouldNeverRead(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// Each of these would sit in the list looking like a correction while
	// matching nothing — or, for a bare public suffix, matching a whole TLD.
	for _, bad := range []map[string]string{
		{"domain": "localhost", "kind": "extra"},
		{"domain": "co.uk", "kind": "never"},
		{"domain": "not a domain", "kind": "extra"},
		{"domain": "acme.example", "kind": "maybe"},
		{"domain": "", "kind": "extra"},
	} {
		if status := e.Call(t, "POST", "/v1/capture/consumer-mail-domains", bad, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("POST %v → %d, want 422", bad, status)
		}
	}
}

type domainProblemDTO struct {
	Code    string `json:"code"`
	Details struct {
		Role    string `json:"role"`
		Company string `json:"company_name"`
		Errors  []struct {
			Field string `json:"field"`
			Code  string `json:"code"`
		} `json:"errors"`
	} `json:"details"`
}

// The shipped list already answers for its own domains, and a customer's
// domain is stored only after the request says yes.
func TestConsumerMailDomainsAnswerWhatIsAlreadyOnFile(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	const path = "/v1/capture/consumer-mail-domains"

	var shipped domainProblemDTO
	if status := e.Call(t, "POST", path, map[string]any{"domain": "gmail.com", "kind": "extra"}, nil, &shipped); status != http.StatusUnprocessableEntity {
		t.Fatalf("a shipped domain → %d, want 422", status)
	}
	if len(shipped.Details.Errors) != 1 || shipped.Details.Errors[0].Code != "already_on_shipped_list" {
		t.Fatalf("shipped refusal = %+v", shipped)
	}

	if status := e.Call(t, "POST", "/v1/companies", map[string]any{
		"display_name": "Demo GmbH",
		"source":       "manual",
		"domains":      []map[string]any{{"domain": "demo-customer.test", "is_primary": true}},
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("seeding the customer → %d, want 201", status)
	}

	var question domainProblemDTO
	if status := e.Call(t, "POST", path, map[string]any{"domain": "demo-customer.test", "kind": "extra"}, nil, &question); status != http.StatusConflict {
		t.Fatalf("a customer's domain → %d, want 409", status)
	}
	if question.Code != "domain_needs_confirmation" || question.Details.Company != "Demo GmbH" {
		t.Fatalf("the question = %+v, want domain_needs_confirmation naming Demo GmbH", question)
	}
	var list consumerMailDomainListDTO
	if e.Call(t, "GET", path, nil, nil, &list); len(list.Data) != 0 {
		t.Fatalf("an unanswered question stored %+v", list.Data)
	}

	if status := e.Call(t, "POST", path, map[string]any{"domain": "demo-customer.test", "kind": "extra", "confirm": true}, nil, nil); status != http.StatusCreated {
		t.Fatalf("the confirmed domain → %d, want 201", status)
	}
}

// A company's domain list refuses consumer mail, and the other-address list
// refuses it as a whole domain but asks about a customer's.
func TestConsumerMailDomainIsRefusedWhereItNamesACompanyOrASeat(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var refusal domainProblemDTO
	if status := e.Call(t, "POST", "/v1/companies", map[string]any{
		"display_name": "Webmail GmbH",
		"source":       "manual",
		"domains":      []map[string]any{{"domain": "gmail.com", "is_primary": true}},
	}, nil, &refusal); status != http.StatusUnprocessableEntity {
		t.Fatalf("a company on gmail.com → %d, want 422", status)
	}
	if len(refusal.Details.Errors) != 1 || refusal.Details.Errors[0].Code != "consumer_mail_domain" {
		t.Fatalf("company refusal = %+v", refusal)
	}

	var identity domainProblemDTO
	if status := e.Call(t, "POST", "/v1/capture/owner-identities",
		map[string]any{"kind": "domain", "value": "gmail.com", "confirm": true}, nil, &identity); status != http.StatusUnprocessableEntity {
		t.Fatalf("gmail.com as a whole domain → %d, want 422 even when confirmed", status)
	}
	if len(identity.Details.Errors) != 1 || identity.Details.Errors[0].Code != "consumer_mail_domain" {
		t.Fatalf("identity refusal = %+v", identity)
	}
}
