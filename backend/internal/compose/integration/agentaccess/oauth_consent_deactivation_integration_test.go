// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// A pending consent is bound to the human who granted it, and that binding
// has to survive nothing the human's own authority does not survive. This is
// the one case identity/users.go's deactivation cascade exists for: a code
// minted minutes before its human is deactivated must not redeem after a
// later reactivation, or a whole connection gets built on a consent given
// under authority that was taken away in between, with nobody consenting
// again.

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
)

// userIDByEmail resolves a member the suite created through the API but whose
// id it never saw, read as the owner because the acting session may already
// be gone by the time the assertion needs it.
func (o *oauthEnv) userIDByEmail(t *testing.T, email string) string {
	t.Helper()
	var id string
	if err := o.Owner.QueryRow(context.Background(),
		`SELECT id FROM app_user WHERE email = $1`, email).Scan(&id); err != nil {
		t.Fatalf("resolving %s: %v", email, err)
	}
	return id
}

// TestAPendingConsentDoesNotSurviveItsHumansDeactivation proves the UPDATE in
// identity's DeactivateUser (users.go) that expires every still-pending
// authorization code the deactivated human holds: the code's own window is
// closed at deactivation, not merely re-checked at redemption, so a code
// minted before it and redeemed after a later reactivation still fails.
func TestAPendingConsentDoesNotSurviveItsHumansDeactivation(t *testing.T) {
	o := setupOAuth(t)
	code := o.authorize(t, url.Values{"scope": {"read"}})

	// The consenting human is the bootstrap admin, and the last active admin
	// may not be deactivated — the organization would lose user administration
	// with no way back. A second admin is what the guard is protecting
	// against, so inviting one is what lets the real endpoint run.
	if status := o.Call(t, "POST", "/v1/users", integration.AnyMap{
		"email": "second-admin@fable.test", "display_name": "Second Admin", "role": "admin",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("inviting a second admin → %d", status)
	}
	// ACTIVATED, because the invitation alone does not make them an
	// administrator who can hold the installation open: an invited seat signs
	// in nowhere, so the last-admin guard rightly refuses to let the only
	// admin who CAN sign in stand down behind it. Driven over SQL for the same
	// reason the reactivation below is — this suite is about consent, not
	// about redeeming a set-password link. The STATUS alone is what the
	// last-admin guard counts; no credential is set because nobody signs in
	// as this seat here.
	secondAdmin := o.userIDByEmail(t, "second-admin@fable.test")
	if _, err := o.Owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'active' WHERE id = $1`, secondAdmin); err != nil {
		t.Fatalf("activating the second admin: %v", err)
	}

	granter := o.userIDByEmail(t, "granter@fable.test")
	if status := o.Call(t, "POST", "/v1/users/"+granter+"/deactivate", integration.AnyMap{
		"reason": "left the company",
	}, nil, nil); status != http.StatusOK {
		t.Fatalf("deactivate → %d", status)
	}

	// Reactivated the way ReactivateUser does — it flips this one column and
	// restores nothing else. Driven over SQL rather than the endpoint because
	// the deactivation just revoked the only session this suite can call
	// with, and whether an admin can log in is not what this test is about.
	if _, err := o.Owner.Exec(context.Background(),
		`UPDATE app_user SET status = 'active' WHERE id = $1`, granter); err != nil {
		t.Fatalf("reactivating the human: %v", err)
	}

	status, body := o.exchange(t, url.Values{"code": {code}})
	if status != http.StatusBadRequest || body["error"] != "invalid_grant" {
		t.Fatalf("token → %d %v, want 400 invalid_grant: the consent behind this code ended when its human was deactivated",
			status, body)
	}
	// Refused because the code's own window was closed, not because something
	// downstream happened to catch it — and refused without spending the row,
	// so the refusal reads as "this code was never valid again" rather than as
	// a redemption that failed late.
	assertOwnerCount(t, o, 1,
		`SELECT count(*) FROM oauth_authorization_code
		   WHERE user_id = $1 AND consumed_at IS NULL AND expires_at <= now()`, granter)
	assertOwnerCount(t, o, 0, `SELECT count(*) FROM oauth_grant`)
	assertOwnerCount(t, o, 0,
		`SELECT count(*) FROM passport WHERE oauth_grant_id IS NOT NULL`)
}
