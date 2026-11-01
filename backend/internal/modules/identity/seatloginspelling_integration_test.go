// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A seat invited at a unicode domain signs in with the punycode spelling of the
// same domain, and the other way about. The login is one address however it is
// typed, which is what storekit.SeatLoginIsSQL answers for every seat match.
func TestASeatSignsInWithEitherSpellingOfItsDomain(t *testing.T) {
	e := setupRevocationEnv(t, "seat-login-spelling")
	ctx := context.Background()
	const unicodeAddress = "rep@bücher.example"
	const punycodeAddress = "rep@xn--bcher-kva.example"
	const password = "a rep password!"

	_, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: unicodeAddress, DisplayName: "Rep One", Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting a rep at a unicode domain: %v", err)
	}
	if err := e.svc.RedeemPasswordReset(
		principal.WithCorrelationID(principal.WithWorkspaceID(ctx, e.ws.UUID), ids.NewV7()),
		rawToken, password,
	); err != nil {
		t.Fatalf("redeeming the invite: %v", err)
	}

	invited, _, err := e.svc.Login(principal.WithWorkspaceID(ctx, e.ws.UUID), unicodeAddress, password, "")
	if err != nil {
		t.Fatalf("signing in with the spelling the invite used: %v", err)
	}
	other, _, err := e.svc.Login(principal.WithWorkspaceID(ctx, e.ws.UUID), punycodeAddress, password, "")
	if err != nil {
		t.Fatalf("signing in with the punycode spelling of the same domain: %v", err)
	}
	if other.UserID != invited.UserID {
		t.Fatalf("the punycode spelling signed in as %s and the unicode one as %s, want one seat",
			other.UserID, invited.UserID)
	}
}
