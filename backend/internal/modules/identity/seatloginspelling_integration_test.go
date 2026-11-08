// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

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

// A seat stored before the folded column existed still signs in with the
// spelling it was stored under.
//
// FoldLoginDomainsTx writes app_user.email_domain_folded a page at a time.
//
// Until it reaches a seat, the raw arm of the comparison is the only one that
// can answer for it.
func TestASeatWithNoFoldedDomainStillSignsIn(t *testing.T) {
	e := setupRevocationEnv(t, "seat-login-unfolded")
	ctx := context.Background()
	const stored = "altrep@bücher.example"
	const password = "another rep password!"

	_, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: stored, DisplayName: "Rep Two", Role: "rep",
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
	// The state of every row written before the fold's migration.
	if _, err := e.owner.Exec(ctx,
		`UPDATE app_user SET email_domain_folded = NULL WHERE lower(email) = lower($1)`, stored); err != nil {
		t.Fatalf("clearing the folded domain: %v", err)
	}

	if _, _, err := e.svc.Login(principal.WithWorkspaceID(ctx, e.ws.UUID), stored, password, ""); err != nil {
		t.Fatalf("signing in as a seat whose folded domain was never written: %v", err)
	}
}

// Two seats can hold one mailbox under both spellings of its domain, because
// the login's unique index reads the raw spelling. The folded comparison then
// matches both, and the seat spelled as typed is the one that signs in.
func TestTheSeatSpelledAsTypedWinsAnAmbiguousLogin(t *testing.T) {
	e := setupRevocationEnv(t, "seat-login-ambiguous")
	ctx := context.Background()
	const unicodeAddress = "amb@bücher.example"
	const punycodeAddress = "amb@xn--bcher-kva.example"
	const unicodePassword = "the unicode password!"
	const punycodePassword = "the punycode password!"

	for _, seat := range []struct{ email, password, name string }{
		{unicodeAddress, unicodePassword, "Amb Unicode"},
		{punycodeAddress, punycodePassword, "Amb Punycode"},
	} {
		_, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
			Email: seat.email, DisplayName: seat.name, Role: "rep",
		})
		if err != nil {
			t.Fatalf("inviting %s: %v", seat.email, err)
		}
		if err := e.svc.RedeemPasswordReset(
			principal.WithCorrelationID(principal.WithWorkspaceID(ctx, e.ws.UUID), ids.NewV7()),
			rawToken, seat.password,
		); err != nil {
			t.Fatalf("redeeming the invite for %s: %v", seat.email, err)
		}
	}

	// Each spelling signs in with its own password, which it could not do if the
	// comparison returned whichever of the two the database reached first.
	for _, attempt := range []struct{ address, password, want string }{
		{unicodeAddress, unicodePassword, "Amb Unicode"},
		{punycodeAddress, punycodePassword, "Amb Punycode"},
	} {
		id, _, err := e.svc.Login(principal.WithWorkspaceID(ctx, e.ws.UUID),
			attempt.address, attempt.password, "")
		if err != nil {
			t.Fatalf("signing in as %s with its own password: %v", attempt.address, err)
		}
		if id.DisplayName != attempt.want {
			t.Errorf("%s signed in as %q, want %q", attempt.address, id.DisplayName, attempt.want)
		}
	}
}

// A sign-in subject linked to a seat that is no longer live does not pass to its
// twin. The twin holds the other spelling of the same login. The two name one mailbox,
// so the disabled seat's link is not a way into the live one.
func TestADisabledSeatsLinkDoesNotSignInItsOtherSpelling(t *testing.T) {
	e := setupRevocationEnv(t, "seat-login-federated")
	ctx := context.Background()
	const unicodeAddress = "fed@bücher.example"
	const punycodeAddress = "fed@xn--bcher-kva.example"
	disabled := redeemedSeat(t, e, unicodeAddress)
	twin := redeemedSeat(t, e, punycodeAddress)

	resolve := func(subject string) (ids.UserID, error) {
		var got ids.UserID
		err := e.svc.db.Tx(principal.WithWorkspaceID(ctx, e.ws.UUID), func(tx pgx.Tx) error {
			var err error
			got, _, err = e.svc.resolveFederatedUser(ctx, tx, "google", subject, punycodeAddress)
			return err
		})
		return got, err
	}
	// The control: a subject nobody linked reaches the live seat by its login.
	if got, err := resolve("subject-unlinked"); err != nil || got != twin {
		t.Fatalf("an unlinked subject resolved %v, %v; want the punycode seat", got, err)
	}

	if _, err := e.owner.Exec(ctx, `INSERT INTO federated_identity (user_id, provider, subject, email_at_link)
		VALUES ($1, 'google', 'subject-linked', $2)`, disabled, unicodeAddress); err != nil {
		t.Fatal(err)
	}
	if _, err := e.owner.Exec(ctx, `UPDATE app_user SET status = 'suspended' WHERE id = $1`, disabled); err != nil {
		t.Fatal(err)
	}
	if got, err := resolve("subject-linked"); !errors.Is(err, ErrFederatedSignInRefused) {
		t.Errorf("the disabled seat's subject resolved %v, %v; want it refused, not moved to its twin", got, err)
	}
}

// redeemedSeat invites a rep at address and redeems the invitation, so the seat
// is live with a password, as one that signs in is.
func redeemedSeat(t *testing.T, e *revocationEnv, address string) ids.UserID {
	t.Helper()
	ctx := context.Background()
	invited, rawToken, err := e.svc.InviteUser(e.wsCtx(e.admin), e.admin, InviteUserInput{
		Email: address, DisplayName: "Rep at " + address, Role: "rep",
	})
	if err != nil {
		t.Fatalf("inviting %s: %v", address, err)
	}
	if err := e.svc.RedeemPasswordReset(
		principal.WithCorrelationID(principal.WithWorkspaceID(ctx, e.ws.UUID), ids.NewV7()),
		rawToken, "a federated rep password!",
	); err != nil {
		t.Fatalf("redeeming %s: %v", address, err)
	}
	return invited
}
