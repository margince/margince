// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// "This is not a company", from the wire.
//
// The store's own suite proves the transaction — both halves, both refusals,
// the domain read under the lock. What it cannot reach is everything between a
// request and that store: the route, the mode shadow that guards it, the
// transport's shape checks, and the response the caller actually reads. Those
// are three files with no other caller, and the wire is where they meet.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// rejectResponse is what the endpoint answers: both halves of the one decision.
type rejectResponse struct {
	Company struct {
		ID         string  `json:"id"`
		ArchivedAt *string `json:"archived_at"`
	} `json:"company"`
	Domain struct {
		Domain    string `json:"domain"`
		Admission string `json:"admission"`
		Reason    string `json:"reason"`
		Source    string `json:"source"`
	} `json:"domain"`
}

func TestRejectingACompanyOverHTTPAnswersBothHalves(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Rejection", "admin@reject.test", "Admin")

	var created struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", map[string]any{
		"display_name": "Expensify Ltd",
		"domains":      []map[string]any{{"domain": "expensify.test", "is_primary": true}},
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("seeding the company → %d", status)
	}

	var out rejectResponse
	if status := e.Call(t, "POST", "/v1/companies/"+created.ID+"/reject", map[string]any{
		"reason": "a tool we use, not a customer",
	}, nil, &out); status != http.StatusOK {
		t.Fatalf("rejecting → %d", status)
	}

	if out.Company.ArchivedAt == nil {
		t.Error("the answer's company is not archived — the caller is told the record survived")
	}
	if out.Domain.Domain != "expensify.test" || out.Domain.Admission != "suppressed" {
		t.Errorf("the answer's decision is %+v, want expensify.test suppressed", out.Domain)
	}
	// The SOURCE is what makes the decision sticky against every later machine
	// verdict, and it is stamped server-side — a caller cannot ask for it.
	if out.Domain.Source != "human" {
		t.Errorf("the decision's source is %q, want human", out.Domain.Source)
	}
	if out.Domain.Reason != "a tool we use, not a customer" {
		t.Errorf("the stored reason is %q, want the caller's", out.Domain.Reason)
	}

	// And the blocked-domain surface serves it, which is where an operator
	// reviews the refusal months later.
	var blocked struct {
		Data []struct {
			Domain    string `json:"domain"`
			Admission string `json:"admission"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/capture/blocked-domains", nil, nil, &blocked); status != http.StatusOK {
		t.Fatalf("reading the blocked list → %d", status)
	}
	var listed bool
	for _, row := range blocked.Data {
		if row.Domain == "expensify.test" && row.Admission == "suppressed" {
			listed = true
		}
	}
	if !listed {
		t.Errorf("the refused domain is not on the blocked list (%+v) — nobody can review a decision they cannot find", blocked.Data)
	}
}

// The transport's own refusals, over the wire, where a client reads them.
func TestRejectingACompanyOverHTTPRefusesAReasonItCannotStore(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Rejection", "admin@reject.test", "Admin")

	var created struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", map[string]any{
		"display_name": "Typed By Hand Ltd",
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("seeding the company → %d", status)
	}

	for _, tc := range []struct {
		name   string
		reason string
	}{
		{"no reason", ""},
		{"whitespace only", "   "},
		{"past the contract's ceiling", longReason()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var problem struct {
				Code string `json:"code"`
			}
			if status := e.Call(t, "POST", "/v1/companies/"+created.ID+"/reject",
				map[string]any{"reason": tc.reason}, nil, &problem); status != http.StatusUnprocessableEntity {
				t.Fatalf("→ %d, want 422", status)
			}
			if problem.Code != "validation_error" {
				t.Errorf("code = %q, want validation_error", problem.Code)
			}
		})
	}

	// A company with no domain is refused too — and this one has none, so the
	// refusal above is not the only thing standing between it and an archive.
	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "POST", "/v1/companies/"+created.ID+"/reject",
		map[string]any{"reason": "a vendor"}, nil, &problem); status != http.StatusUnprocessableEntity {
		t.Fatalf("rejecting a company with no domain → %d, want 422", status)
	}
}

// longReason is one character past the contract's maxLength, in MULTI-BYTE
// runes: measured in bytes this is three times over, and a ceiling counted that
// way refuses a legal reason at a third of its allowance.
func longReason() string {
	runes := make([]rune, 501)
	for i := range runes {
		runes[i] = 'é'
	}
	return string(runes)
}
