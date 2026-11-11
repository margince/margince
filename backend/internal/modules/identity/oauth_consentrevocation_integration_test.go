// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A consent and the end of its member's credentials are ordered on the member's
// row. A code either commits before the revocation and is ended by it, or is
// never minted. Both orders are staged on real transactions, because a lock
// order is only ever a property of real ones.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// signedInConsenter invites and signs in a seat. It returns the seat and its
// session row's hash, which the consent handler reads off the cookie.
func signedInConsenter(t *testing.T, e *revocationEnv) (Identity, string) {
	t.Helper()
	email := "consenter@" + e.slug + ".test"
	id := inviteAndLogin(t, e, email, "Consenter")
	_, session, err := e.svc.Login(principal.WithWorkspaceID(context.Background(), e.ws.UUID), email, seatNamePassword, noDevice)
	if err != nil {
		t.Fatalf("signing the consenter in: %v", err)
	}
	return id, hashToken(session.Token)
}

func consentRequestFor(clientID string) authorizeRequest {
	return authorizeRequest{
		ClientID: clientID, RedirectURI: "https://client.example/cb",
		Scopes: []string{"read"}, CodeChallenge: "challenge",
	}
}

func liveCodesOf(t *testing.T, e *revocationEnv, userID ids.UserID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_authorization_code WHERE user_id = $1 AND expires_at > now()`,
		userID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAConsentOnALiveSessionMintsACode(t *testing.T) {
	e := setupRevocationEnv(t, "consent-live")
	id, sessionHash := signedInConsenter(t, e)
	clientID := e.registerOAuthClient(t, "consent live")

	code, err := e.svc.mintConsentedAuthorizationCode(e.wsCtx(id), id, sessionHash,
		consentChoice{scopes: "read"}, consentRequestFor(clientID))
	if err != nil || code == "" {
		t.Fatalf("a consent on a live session: code %q, err %v", code, err)
	}
	if got := liveCodesOf(t, e, id.UserID); got != 1 {
		t.Errorf("live codes after the consent = %d, want 1", got)
	}
}

func TestAConsentWaitingOnARevocationMintsNoCode(t *testing.T) {
	e := setupRevocationEnv(t, "consent-after-revoke")
	id, sessionHash := signedInConsenter(t, e)
	clientID := e.registerOAuthClient(t, "consent after revoke")
	ctx := e.wsCtx(id)

	minted := make(chan error, 1)
	err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := endCredentialAuthority(selfActorCtx(ctx, id.UserID), tx, id.UserID, passwordChangeRevokeReason); err != nil {
			return err
		}
		go func() {
			_, err := e.svc.mintConsentedAuthorizationCode(ctx, id, sessionHash,
				consentChoice{scopes: "read"}, consentRequestFor(clientID))
			minted <- err
		}()
		waitForALockWaiter(t, e)
		return nil
	})
	if err != nil {
		t.Fatalf("the revocation: %v", err)
	}
	if err := <-minted; !errors.Is(err, errConsentingSessionEnded) {
		t.Errorf("a consent that waited on the revocation: %v, want errConsentingSessionEnded", err)
	}
	if got := liveCodesOf(t, e, id.UserID); got != 0 {
		t.Errorf("live codes after the revocation = %d, want 0", got)
	}
}

func TestARevocationWaitingOnAConsentEndsItsCode(t *testing.T) {
	e := setupRevocationEnv(t, "revoke-after-consent")
	id, sessionHash := signedInConsenter(t, e)
	clientID := e.registerOAuthClient(t, "revoke after consent")
	ctx := e.wsCtx(id)

	revoked := make(chan error, 1)
	err := e.svc.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := holdConsentAgainstRevocation(ctx, tx, clientID, id.UserID, sessionHash); err != nil {
			return err
		}
		go func() {
			revoked <- e.svc.db.Tx(ctx, func(rtx pgx.Tx) error {
				return endCredentialAuthority(selfActorCtx(ctx, id.UserID), rtx, id.UserID, passwordChangeRevokeReason)
			})
		}()
		waitForALockWaiter(t, e)
		// The code row the consent commits, written while the revocation waits.
		_, err := tx.Exec(ctx, `INSERT INTO oauth_authorization_code
			(client_id, code_hash, code_challenge, redirect_uri, scopes, user_id, expires_at)
			VALUES ($1, $2, 'challenge', 'https://client.example/cb', '{read}', $3, now() + interval '5 minutes')`,
			clientID, "hash-"+ids.NewV7().String(), id.UserID)
		return err
	})
	if err != nil {
		t.Fatalf("the consent: %v", err)
	}
	if err := <-revoked; err != nil {
		t.Fatalf("the revocation that waited on the consent: %v", err)
	}
	if got := liveCodesOf(t, e, id.UserID); got != 0 {
		t.Errorf("live codes after the revocation = %d, want 0: the code committed during it survived", got)
	}
}
