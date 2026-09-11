// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestReadSeatMayEndItsOwnSession(t *testing.T) {
	// Ending your own session is credential self-management, not a business
	// write — a read seat whose device is compromised must be able to sign it
	// out. The store scopes the revoke to the caller, so the ceiling need not.
	revoke := httptest.NewRequest(http.MethodDelete, "/v1/me/sessions/8d1b3a3e-0000-7000-8000-000000000000", nil)
	if !readSeatMayMutate(revoke) {
		t.Error("a read seat cannot end its own session; the seat ceiling stranded it")
	}

	// The exemption is for the item revoke, not a licence to write elsewhere.
	business := httptest.NewRequest(http.MethodDelete, "/v1/people/8d1b3a3e-0000-7000-8000-000000000000", nil)
	if readSeatMayMutate(business) {
		t.Error("the read-seat exemption admits a business delete — it is wider than session self-management")
	}
}

func TestSessionRevokeIsNotTheMustChangePasswordExit(t *testing.T) {
	// The must-change-password gate confines an account on an operator's
	// password to exactly the route that replaces it. Revoking a session must
	// NOT count as that exit, or such an account could wander past the lock.
	revoke := httptest.NewRequest(http.MethodDelete, "/v1/me/sessions/8d1b3a3e-0000-7000-8000-000000000000", nil)
	if isOwnCredentialRequest(revoke) {
		t.Error("session revoke reads as the credential-change exit; a locked account could leave the one route it may reach")
	}
}
