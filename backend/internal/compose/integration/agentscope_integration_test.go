// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The passport cap a verb spends is the CONTRACT's, over real HTTP.
//
// The REST agent gate used to admit any verb with no registered MCP tool
// under `write`, so a passport holding read+write could enrich from a
// third-party website or ship an offer to a counterparty — two egressing
// acts — on a cap that says nothing about leaving the workspace. The verbs
// now carry `x-mcp-tool.scope`, and `principal.ScopeSet.Has` is exact
// membership: `write` does not imply `enrich` or `send`.
//
// The unit suites prove contract↔registry parity. What only this lane can
// prove is the gate: that the missing cap is REFUSED at the wire, with the
// scope sentinel's own status and code, and — the part a "not 200" check
// would have passed on the old code too — that nothing was staged.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// scopeRefusalCode is what apperrors.ErrScopeExceeded maps to in
// httperr's sentinel registry, and problemType is the RFC 7807 type URI
// writeProblem derives from it.
const (
	scopeRefusalCode = "scope_exceeds_grantor"
	scopeRefusalType = "https://errors.gradion.com/scope_exceeds_grantor"
)

// capRefusal is the slice of the RFC 7807 problem the scope refusals are
// asserted on: the machine code and type a client branches on, plus the
// detail a human reads.
type capRefusal struct {
	Type   string `json:"type"`
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

func TestEnrichRefusesAPassportWithoutTheEnrichCap(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Enrich Scope", "enrich@fable.test", "Admin")

	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", AnyMap{
		"display_name": "Acme GmbH", "source": "manual",
		"domains": []AnyMap{{"domain": "acme.example", "is_primary": true}},
	}, nil, &company); status != http.StatusCreated {
		t.Fatalf("create company → %d", status)
	}

	// The old default: everything a mutating agent did cost `write`.
	writeOnly := apptest.PassportBearer(t, e, "write-only agent", "read", "write")

	var refusal capRefusal
	status := e.Call(t, "POST", "/v1/companies/"+company.ID+"/enrich", nil, writeOnly, &refusal)
	if status != http.StatusForbidden || refusal.Code != scopeRefusalCode || refusal.Type != scopeRefusalType {
		t.Fatalf("enrich on read+write → %d %q %q, want 403 %s / %s",
			status, refusal.Code, refusal.Type, scopeRefusalCode, scopeRefusalType)
	}
	// The distinguishing assertion. Before the scope came from the contract
	// this same call was ADMITTED and staged a 🟡 approval — a 403 either
	// way, so only the code and the absence of a staged row separate the
	// refusal from the admission.
	if refusal.Code == "approval_required" || strings.Contains(refusal.Detail, "staged as approval") {
		t.Fatalf("a passport without the enrich cap reached the 🟡 gate: %q", refusal.Detail)
	}
	assertNothingStaged(t, e, "refused enrich")
	assertRefusalLeaksNothing(t, refusal.Detail, "enrich")

	// The same principal shape with the cap the contract declares. There is
	// no route that widens a minted passport's scopes (mint and revoke only),
	// so the grant is a fresh passport differing in exactly that one cap.
	withEnrich := apptest.PassportBearer(t, e, "enriching agent", "read", "write", "enrich")

	// What this proves: auth.Admit checks scope BEFORE tier, so
	// approval_required is unreachable unless the enrich cap was spent — the
	// outcome is the verb's real 🟡 behaviour, not a different refusal. And
	// it is deterministic offline: the gate stages the whole unapplied
	// request and answers before the handler runs, so no website is fetched
	// on this path at all.
	var staged capRefusal
	status = e.Call(t, "POST", "/v1/companies/"+company.ID+"/enrich", nil, withEnrich, &staged)
	if status != http.StatusForbidden || staged.Code != "approval_required" {
		t.Fatalf("enrich with the enrich cap → %d %q, want 403 approval_required (the 🟡 gate)", status, staged.Code)
	}
	approvalID := ExtractStagedApprovalID(t, staged.Detail)
	var kind string
	if err := e.Owner.QueryRow(t.Context(),
		`SELECT kind FROM approval WHERE id = $1`, approvalID).Scan(&kind); err != nil {
		t.Fatal(err)
	}
	if kind != "enrich" {
		t.Fatalf("staged approval kind = %q, want enrich", kind)
	}
}

// Sending an offer is a route-backed action with the send scope. It runs at once
// as the app's Send does, and the action tells the agent to ask its user first.
func TestAnAgentSendsAnOfferAsItsUser(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Send Scope", "send@fable.test", "Admin")
	dealID := offerFixture(t, e)

	bearer := apptest.PassportBearer(t, e, "drafting agent", "read", "write", "send", "enrich")

	var offer offerBody
	if status := e.Call(t, "POST", "/v1/deals/"+dealID+"/offers", AnyMap{
		"currency": "EUR", "source": "manual",
		"line_items": []AnyMap{{"description": "Pilot", "quantity": 1, "unit_price_minor": 250000, "tax_rate": 19.0}},
	}, bearer, &offer); status != http.StatusCreated {
		t.Fatalf("agent 🟢 offer draft → %d", status)
	}

	if status := e.Call(t, "POST", "/v1/offers/"+offer.ID+"/send", nil, bearer, nil); status != http.StatusOK {
		t.Fatalf("agent send → %d, want 200", status)
	}
	assertNothingStaged(t, e, "an offer sent at once")
	var sent offerBody
	if status := e.Call(t, "GET", "/v1/offers/"+offer.ID, nil, bearer, &sent); status != http.StatusOK || sent.Status != "sent" {
		t.Fatalf("offer after the send = %q, want sent", sent.Status)
	}
}

// assertNothingStaged proves the refusal was a cap refusal and not the 🟡
// admission wearing the same status: a staged approval is a durable
// authority object, so its absence is the observable difference.
func assertNothingStaged(t *testing.T, e *apptest.AppEnv, what string) {
	t.Helper()
	var staged int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM approval`).Scan(&staged); err != nil {
		t.Fatal(err)
	}
	if staged != 0 {
		t.Fatalf("%s staged %d approval(s) — a missing cap must refuse, never mint authority to spend it", what, staged)
	}
}

// assertRefusalLeaksNothing holds the refusal body to both halves of the
// error bar: it names the cap the caller must be granted, and it carries no
// operator-only detail.
func assertRefusalLeaksNothing(t *testing.T, detail, missingCap string) {
	t.Helper()
	if !strings.Contains(detail, missingCap) {
		t.Fatalf("refusal %q does not name the missing cap %q — the caller cannot act on it", detail, missingCap)
	}
	lower := strings.ToLower(detail)
	for _, leak := range []string{
		"select ", "insert ", "update ", "app_user", "workspace_id", "approval_effect",
		"pgx", "errscopeexceeded", "apperrors", ".go:", "goroutine", "/users/", "internal/platform",
	} {
		if strings.Contains(lower, leak) {
			t.Fatalf("refusal %q leaks internals (%q)", detail, leak)
		}
	}
}
