// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The passport's own SCOPE on the REST read door, over the real HTTP stack.
//
// Two doors admit an agent to the same read, and only one of them used to check
// the passport's scope. The MCP door asks spec.RequiredScope in auth.Admit; the
// REST read door applied the volume bound and nothing else, on the reasoning
// that a GET's authority is the granting human's RBAC at the store. That is
// true of the ROW SCOPE, and it is not the thing a narrow passport is for: the
// point of minting one without `read` is that it cannot read, and it could —
// over /v1, on every agent-readable GET in the table (#2242).
//
// Driven over the real router with a real minted passport rather than against
// the check, because which layer answers is the whole question. #2242 read
// refuseAgentRead — which does apply the volume bound and nothing else — and
// concluded the ceiling was missing. It is not: identity's serveAsAgent
// refuses a non-mutating method to a passport without the read scope before
// the agent principal is ever bound, so the gate never sees the request.
//
// What WAS missing is this test. The arm was written once and held by nothing,
// so the reading that led to #2242 could not be settled by running anything —
// and an untested security arm is one refactor away from being the gap it was
// mistaken for.

import (
	"net/http"
	"testing"
)

// A passport minted WITHOUT the read scope is refused an agent-readable GET.
//
// The write scope is what it holds, so this is not "a credential with nothing"
// being turned away at a door that wants something — it is a credential that
// was deliberately narrowed being held to the narrowing.
func TestAPassportWithoutTheReadScopeIsRefusedOnTheRestDoor(t *testing.T) {
	e, _ := boundedApp(t, "read-scope-rest", 100)
	seedContacts(t, e, 2)

	reader, _ := passportWithID(t, e, "reading agent", "read")
	writer, _ := passportWithID(t, e, "writing agent", "write")

	// The route is open to an agent that holds the scope, so the refusal below
	// is the scope ceiling rather than the route being closed to agents.
	if status := e.Call(t, "GET", "/v1/contacts", nil, reader, nil); status != http.StatusOK {
		t.Fatalf("an agent read with the read scope → %d, want 200", status)
	}

	// The write-only credential is LIVE, proven by using it: without this the
	// 403 below is equally consistent with a token that never resolved, and the
	// test would pass just as happily against a passport that was revoked,
	// malformed or never minted.
	if status := e.Call(t, "POST", "/v1/contacts", AnyMap{
		"full_name": "Scope Probe",
	}, writer, nil); status != http.StatusCreated {
		t.Fatalf("the write-only passport could not write → %d, want 201; the read refusal below would not be about the read scope", status)
	}

	var problem struct {
		Code string `json:"code"`
	}
	status := e.Call(t, "GET", "/v1/contacts", nil, writer, &problem)

	// 403 specifically. A 429 would mean the bound answered — the term that was
	// already there — and a 401 would mean the token never resolved, neither of
	// which is this property.
	if status != http.StatusForbidden {
		t.Errorf("a passport with no read scope → %d, want 403; the REST door is outside the scope ceiling the MCP door applies", status)
	}
	if problem.Code == "rate_limited" {
		t.Errorf("the refusal came from the volume bound, not the scope ceiling — a passport with no read scope must be refused before its window is consulted")
	}
}
