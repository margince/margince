// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The owner-identity endpoints over the wire.
//
// The store's own suite proves what a claim DOES to captured mail; this proves
// the surface a person reaches it through: that a claim round-trips, that a
// malformed one is refused rather than stored, and that withdrawing somebody
// else's answers not-found rather than confirming it exists.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func TestTheOwnerIdentityEndpointsRoundTripAClaim(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var created struct {
		ID     string `json:"id"`
		Kind   string `json:"kind"`
		Value  string `json:"value"`
		Source string `json:"source"`
	}
	// Declared with capitals, because that is what a person types. It comes
	// back folded: one stored form is what lets the capture gates compare
	// without a runtime case fold.
	if status := e.Call(t, "POST", "/v1/capture/owner-identities",
		map[string]any{"kind": "address", "value": "Lars@Private.Example"}, nil, &created); status != http.StatusCreated {
		t.Fatalf("POST = %d, want 201", status)
	}
	if created.Value != "lars@private.example" {
		t.Errorf("stored value = %q, want it folded", created.Value)
	}
	if created.Source != "user" {
		t.Errorf("source = %q, want user — a provider-attested claim is a different fact and nothing writes it yet", created.Source)
	}

	var listed struct {
		Data []struct {
			ID    string `json:"id"`
			Value string `json:"value"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/capture/owner-identities", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("GET = %d, want 200", status)
	}
	if len(listed.Data) != 1 || listed.Data[0].ID != created.ID {
		t.Fatalf("the list carried %d identity(ies), want the one just declared", len(listed.Data))
	}

	// Idempotent on the folded value: declaring the same address again is the
	// same fact, and answers the row that already stands.
	var again struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/capture/owner-identities",
		map[string]any{"kind": "address", "value": "lars@private.example"}, nil, &again); status != http.StatusCreated {
		t.Fatalf("re-declaring = %d, want 201", status)
	}
	if again.ID != created.ID {
		t.Errorf("re-declaring minted a second row — the same address twice is one claim")
	}

	if status := e.Call(t, "DELETE", "/v1/capture/owner-identities/"+created.ID, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("DELETE = %d, want 204", status)
	}
	if status := e.Call(t, "GET", "/v1/capture/owner-identities", nil, nil, &listed); status != http.StatusOK || len(listed.Data) != 0 {
		t.Errorf("after withdrawing, the list carried %d identity(ies), want none", len(listed.Data))
	}
}

func TestAMalformedOwnerIdentityIsRefusedRatherThanStored(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	for _, body := range []map[string]any{
		{"kind": "address", "value": "not-an-address"},
		{"kind": "address", "value": ""},
		{"kind": "domain", "value": "not a domain"},
		{"kind": "mailbox", "value": "lars@private.example"},
	} {
		if status := e.Call(t, "POST", "/v1/capture/owner-identities", body, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("POST %v = %d, want 422 — a claim the gates cannot compare against is worse than no claim, "+
				"because the seat believes an address is covered when it is not", body, status)
		}
	}
}

func TestWithdrawingAnIdentityThatIsNotYoursAnswersNotFound(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	// A well-formed id nobody's row carries. Not-found rather than forbidden:
	// the two are the same answer to somebody who may not know it exists, and
	// distinguishing them would confirm a colleague's claim.
	if status := e.Call(t, "DELETE", "/v1/capture/owner-identities/01a05100-0000-7000-8000-000000000000",
		nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("DELETE of an identity that is not the caller's = %d, want 404", status)
	}
}
