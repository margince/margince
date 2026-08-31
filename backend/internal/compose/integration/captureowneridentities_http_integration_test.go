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
	"context"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
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

	// A COLLEAGUE'S row, not a fabricated id: an id nobody carries proves only
	// that a missing row is 404, which is a different claim. What matters is
	// that a row which EXISTS and belongs to somebody else answers the same,
	// because distinguishing "not yours" from "not there" confirms a
	// colleague's private address exists.
	theirs := colleagueIdentity(t, e)
	if status := e.Call(t, "DELETE", "/v1/capture/owner-identities/"+theirs, nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("DELETE of a colleague's identity = %d, want 404", status)
	}
	// And it is still there afterwards.
	var stillTheirs int
	if err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM capture_owner_identity WHERE id = $1`, theirs).Scan(&stillTheirs)
	}); err != nil {
		t.Fatal(err)
	}
	if stillTheirs != 1 {
		t.Error("a colleague's identity was withdrawn by somebody else")
	}
}

// colleagueIdentity plants one identity owned by a DIFFERENT seat, written
// straight to the table because the endpoint only ever writes the caller's own
// — which is the property under test, so it cannot be used to set it up.
func colleagueIdentity(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	id := ids.NewV7()
	if err := e.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		var colleague ids.UUID
		if err := tx.QueryRow(context.Background(), `
			INSERT INTO app_user (id, email, display_name)
			VALUES ($1, $2, 'A Colleague') RETURNING id`,
			ids.NewV7(), "colleague-"+id.String()+"@seed.test").Scan(&colleague); err != nil {
			return err
		}
		_, err := tx.Exec(context.Background(), `
			INSERT INTO capture_owner_identity (id, user_id, kind, value, source, created_by)
			VALUES ($1, $2, 'address', $3, 'user', 'human:seed')`,
			id, colleague, "theirs-"+id.String()+"@private.example")
		return err
	}); err != nil {
		t.Fatalf("planting a colleague's identity: %v", err)
	}
	return id.String()
}
