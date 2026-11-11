// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

// A consent mints its code in a transaction of its own, later than the request
// admission validated on a live session. A password change, reset or email
// change that revokes that session in the gap must stop the code. Otherwise a
// stolen session the owner just locked out keeps a route to an OAuth grant. The
// mint takes the member's app_user row FOR SHARE. Every revoke caller holds that
// row exclusively, so the two serialize over a real Postgres. The mint then
// re-reads the revoked session. A lock order is only ever a property of real
// transactions, so this runs in the integration lane.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedLiveSession writes one unrevoked, unexpired session for a member and
// answers with its id, the value admission binds onto the consenting Identity.
func (e *revocationEnv) seedLiveSession(t *testing.T, userID ids.UserID) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`INSERT INTO session (user_id, token_hash, idle_expires_at, expires_at)
		 VALUES ($1, $2, now() + interval '1 hour', now() + interval '1 hour')
		 RETURNING id`, userID, "race-"+ids.NewV7().String()).Scan(&id); err != nil {
		t.Fatalf("seeding a live session: %v", err)
	}
	return id
}

// consentRaceRequest is the validated authorize request a mint commits, with a
// registered client so the code row names one the token endpoint will accept.
func consentRaceRequest(clientID string) authorizeRequest {
	return authorizeRequest{
		ClientID:      clientID,
		RedirectURI:   "https://client.example/cb",
		CodeChallenge: "challenge",
		Scopes:        []string{"read"},
	}
}

// A consent whose session is live mints a code. The negative cases below only
// mean something if the common path is not refused by the same re-check.
func TestAConsentWithALiveSessionMintsACode(t *testing.T) {
	e := setupRevocationEnv(t, "consent-live-session")
	clientID := e.registerOAuthClient(t, "consent live")
	consenter := e.member
	consenter.SessionID = e.seedLiveSession(t, e.member.UserID)

	code, err := e.svc.mintConsentedAuthorizationCode(
		e.wsCtx(consenter), consenter, consentChoice{scopes: "read"}, consentRaceRequest(clientID))
	if err != nil {
		t.Fatalf("a consent on a live session was refused: %v", err)
	}
	if code == "" {
		t.Fatal("a successful consent returned an empty code")
	}
	var live bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT consumed_at IS NULL AND expires_at > now()
		   FROM oauth_authorization_code WHERE code_hash = $1`,
		hashOAuthCode(code)).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if !live {
		t.Fatal("the minted code is not a live, unredeemed authorization code")
	}
}

// A revocation holds the member's app_user row the way every caller does, and
// revokes the session, while a consent mints behind it. The mint must queue on
// the row, then find the session gone and refuse. No live code is left for a
// stolen session to redeem.
func TestAConsentMintingWhileARevocationHoldsTheMemberIsRefused(t *testing.T) {
	e := setupRevocationEnv(t, "consent-revoke-race")
	clientID := e.registerOAuthClient(t, "consent race")
	consenter := e.member
	consenter.SessionID = e.seedLiveSession(t, e.member.UserID)

	// A revocation holds the member's app_user row FOR UPDATE, the exclusive lock
	// every caller takes before the cascade. It then runs the real cascade, held
	// open so the mint must queue behind the row.
	revoke := e.holdOpen(t, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT 1 FROM app_user WHERE id = $1 FOR UPDATE`, e.member.UserID); err != nil {
			return err
		}
		return endCredentialAuthority(selfActorCtx(ctx, e.member.UserID), tx, e.member.UserID, passwordChangeRevokeReason)
	})

	minted := make(chan error, 1)
	returned := make(chan struct{})
	go func() {
		defer close(returned)
		_, err := e.svc.mintConsentedAuthorizationCode(
			e.wsCtx(consenter), consenter, consentChoice{scopes: "read"}, consentRaceRequest(clientID))
		minted <- err
	}()
	e.awaitQueuedBehind(t, revoke.pid, returned, "the mint")
	revoke.release()

	if err := <-minted; !errors.Is(err, errConsentAuthorityEnded) {
		t.Fatalf("a consent minting as the session was revoked: got %v, want errConsentAuthorityEnded", err)
	}
	var live int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM oauth_authorization_code
		  WHERE user_id = $1 AND consumed_at IS NULL AND expires_at > now()`,
		e.member.UserID).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live != 0 {
		t.Fatalf("the refused consent left %d live authorization code(s) behind", live)
	}
}

// The other order: the mint commits its code first, with the revocation behind
// it. The revocation's own sweep then ends the code it now sees. The mint is not
// refused here. The code is ended instead, so a stolen session still redeems
// nothing.
func TestACodeMintedBeforeTheRevocationIsEndedByIt(t *testing.T) {
	e := setupRevocationEnv(t, "consent-mint-first")
	clientID := e.registerOAuthClient(t, "mint first")
	consenter := e.member
	consenter.SessionID = e.seedLiveSession(t, e.member.UserID)

	code, err := e.svc.mintConsentedAuthorizationCode(
		e.wsCtx(consenter), consenter, consentChoice{scopes: "read"}, consentRaceRequest(clientID))
	if err != nil {
		t.Fatalf("minting the code before the revocation: %v", err)
	}
	// The same exclusive app_user lock a real caller holds, then the real cascade.
	revokeCtx := e.wsCtx(e.admin)
	if err := e.svc.db.Tx(revokeCtx, func(tx pgx.Tx) error {
		if _, err := tx.Exec(revokeCtx, `SELECT 1 FROM app_user WHERE id = $1 FOR UPDATE`, e.member.UserID); err != nil {
			return err
		}
		return endCredentialAuthority(selfActorCtx(revokeCtx, e.member.UserID), tx, e.member.UserID, passwordChangeRevokeReason)
	}); err != nil {
		t.Fatalf("the revocation after the mint: %v", err)
	}

	var live bool
	if err := e.owner.QueryRow(context.Background(),
		`SELECT consumed_at IS NULL AND expires_at > now()
		   FROM oauth_authorization_code WHERE code_hash = $1`,
		hashOAuthCode(code)).Scan(&live); err != nil {
		t.Fatal(err)
	}
	if live {
		t.Fatal("a code minted before the revocation is still live after it")
	}
}
