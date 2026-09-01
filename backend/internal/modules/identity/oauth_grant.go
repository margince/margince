// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The durable record of what a human approved: one oauth_grant row per
// consent, and the rotating refresh tokens minted beneath it. Without it a
// connector's whole authority lived in a passport that expired with nothing
// able to renew it, and the client, the human, and the audience the consent
// covered were recoverable only from the passport's label.
//
// A grant is one connection per CLIENT REGISTRATION: re-consenting from an
// already-connected client supersedes its earlier grant
// (supersedePriorGrants) rather than adding a row beside it. client_id is a
// REGISTRATION, not a product — a laptop and a desktop install of one
// client are two connections this does not fold together.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// refreshTokenPrefix tags a refresh token the way passportTokenPrefix tags a
// passport. Both are the same 32-byte base64url shape hashed into different
// tables, so without a tag a leaked string names neither its kind nor the
// table that would revoke it.
const refreshTokenPrefix = "mgr_"

// refreshTokenTTL is how long a connection may keep renewing itself before
// the human has to consent again. It is maxPassportTTL rather than a number
// of its own: the window in which a connector renews with no human in the
// loop must not exceed the longest single authority a human can grant in
// one act.
const refreshTokenTTL = maxPassportTTL

// issueGrantInput is the consent as approved: the client it was approved
// for, the passport scopes the credentials under it carry, whether
// offline_access rode the request (the only thing that makes refresh
// possible at all), and the RFC 8707 audience the authorization was bound
// to — nil for a client that named none, which is exactly what the code row
// recorded and must not be upgraded to a binding the client never asked for.
type issueGrantInput struct {
	WorkspaceID    ids.WorkspaceID
	UserID         ids.UserID
	ClientID       string
	Scopes         []string
	RefreshAllowed bool
	Resource       *string
}

// errConsentingUserInactive refuses a consent whose human is no longer live.
// It answers on the wire exactly as a spent code does: whether an account
// exists and is deactivated is not something an unauthenticated token request
// may learn.
var errConsentingUserInactive = errors.New("oauth: the consenting user is not active")

// lockClientRegistration is the FIRST lock issueGrant takes — ahead of
// app_user (requireLiveConsentingUser) and oauth_grant — because "one row per
// client REGISTRATION" is a fact about the client_id, and only a lock on that
// row serializes two consents racing for the same registration. Without it,
// two transactions can each read supersedePriorGrants' "no active grant yet"
// snapshot before either commits its INSERT, and both succeed: two live
// grants for one registration, the exact state the invariant forbids.
//
// It carries liveClientPredicate (passport.go) for the same reason every
// other read of oauth_client does: consumeAuthCode checked liveness before
// this transaction reached here, but READ COMMITTED re-evaluates each
// statement against what is now committed, so a disable racing between that
// check and this lock would otherwise mint a grant for a client an admin
// just killed. The predicate sits in the WHERE, not a check after — a
// disabled client's row is excluded from the SELECT entirely, so it is never
// locked and there is nothing to recheck; pgx.ErrNoRows becomes errCodeSpent,
// the same invalid_grant a spent code gets, matching consumeAuthCode's own
// reasoning for a dead client (oauth_token.go).
func lockClientRegistration(ctx context.Context, tx pgx.Tx, clientID string) error {
	var locked string
	err := tx.QueryRow(ctx, `
		SELECT c.client_id FROM oauth_client c
		WHERE c.client_id = $1 AND `+liveClientPredicate+`
		FOR UPDATE`, clientID).Scan(&locked)
	if errors.Is(err, pgx.ErrNoRows) {
		return errCodeSpent
	}
	return err
}

// requireLiveConsentingUser refuses to build a connection on authority its
// human no longer has, and takes app_user FOR UPDATE to do it.
//
// The predicate has to be re-read HERE rather than trusted from the
// authorization code. A code minted while the human was live redeems into a
// brand-new grant afterwards, and because a grant outlives every passport
// beneath it, per-call re-auth alone does not cover the gap: the connector is
// merely dormant while the human is deactivated, and reactivating them
// silently restores a connector nobody re-approved.
//
// The lock is what makes it a decision rather than a guess. DeactivateUser
// takes the same app_user row FOR UPDATE before revoking what borrows the
// human's authority, so a redemption racing a deactivation serializes instead
// of interleaving: one of the two goes second and sees the other's outcome.
// Taking it BEFORE the grant insert also keeps the app_user → oauth_grant
// order that cascade already uses, so the two cannot deadlock.
func requireLiveConsentingUser(ctx context.Context, tx pgx.Tx, in issueGrantInput) error {
	var status string
	err := tx.QueryRow(ctx, `
		SELECT status FROM app_user
		WHERE id = $1 AND archived_at IS NULL
		FOR UPDATE`, in.UserID).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return errConsentingUserInactive
	}
	if err != nil {
		return err
	}
	if status != userStatusActive {
		return errConsentingUserInactive
	}
	return nil
}

// issueGrant records one approved consent and mints the first refresh token
// beneath it inside the CALLER's transaction, so the grant commits together
// with the authorization-code consumption that authorized it and the
// passport that follows: a client holding a refresh token for a grant that
// does not exist, or a passport with no grant to revoke it through, are
// states this flow cannot reach.
//
// The refresh plaintext is returned exactly once and only its hash is
// stored. It is empty when the grant does not allow refresh — then there is
// no credential to hand back.
func issueGrant(ctx context.Context, tx pgx.Tx, in issueGrantInput) (grantID ids.UUID, refresh string, err error) {
	if err := lockClientRegistration(ctx, tx, in.ClientID); err != nil {
		return ids.Nil, "", err
	}
	if err := requireLiveConsentingUser(ctx, tx, in); err != nil {
		return ids.Nil, "", err
	}
	auditCtx := actorCtx(ctx, Identity{UserID: in.UserID, WorkspaceID: in.WorkspaceID})
	if err := supersedePriorGrants(auditCtx, tx, in.ClientID); err != nil {
		return ids.Nil, "", err
	}
	// oauth_grant.lent_passport_id and oauth_authorization_code.lent_passport_id
	// are dead columns kept by the additive-only migration rule. Nothing writes
	// them and nothing reads them: a connection's authority is the scopes its
	// human ticked, and there is no second passport standing behind it.
	err = tx.QueryRow(ctx, `
		INSERT INTO oauth_grant (client_id, user_id, scopes, refresh_allowed, resource)
		VALUES ($1, $2, $3, $4, $5)
		RETURNING id`,
		in.ClientID, in.UserID, in.Scopes, in.RefreshAllowed, in.Resource).Scan(&grantID)
	if err != nil {
		return ids.Nil, "", err
	}
	// Granting a remote client standing, renewable authority over the human's
	// own records is audited as its own fact, separate from the passport
	// minted under it: the consent outlives every passport it issues and is
	// what an admin later disables.
	//
	// This call is audit-ONLY: issuing a grant publishes nothing of its own —
	// events.md's closed catalog defines no oauth_grant.* creation verb — and
	// the only reason writeshape_test.go's auditOnlyWrites gate does not flag
	// this function is that supersedePriorGrants above happens to reach an
	// Emit when it revokes an earlier grant for the same client
	// (revokeGrantTx → revokeGrantPassportsTx → passport.revoked), which does
	// not run on a first-time consent. A future change to
	// supersedePriorGrants that stops emitting there — or a path that calls
	// this without ever superseding anything — would make this ungated again
	// with no test catching it. This comment is the record until a real
	// waiver entry or an oauth_grant creation event exists.
	if _, err := storekit.Audit(auditCtx, tx, "create", "oauth_grant", grantID, nil,
		map[string]any{
			auditFieldClientID:       in.ClientID,
			auditFieldScopes:         in.Scopes,
			auditFieldRefreshAllowed: in.RefreshAllowed,
			auditFieldResource:       in.Resource,
		}); err != nil {
		return ids.Nil, "", err
	}
	if !in.RefreshAllowed {
		return grantID, "", nil
	}

	raw, err := randomToken()
	if err != nil {
		return ids.Nil, "", err
	}
	// The stored hash covers the PREFIXED token, exactly as a passport's
	// does, so there is one token spelling and the lookup hashes what the
	// wire carried.
	refresh = refreshTokenPrefix + raw
	// replaced_by stays NULL: the first token in a chain succeeds nothing,
	// and rotation is what fills the forward link.
	if _, err := tx.Exec(ctx, `
		INSERT INTO oauth_refresh_token (grant_id, token_hash, expires_at)
		VALUES ($1, $2, now() + $3::interval)`,
		grantID, hashToken(refresh), refreshTokenTTL.String()); err != nil {
		return ids.Nil, "", err
	}
	return grantID, refresh, nil
}

// oauthPassportLabel names the client a passport was issued for, so Settings
// shows which connection a credential belongs to. Spelled once because the
// code exchange and every later rotation must produce the same label — a
// renewal that relabelled the connection would read as a second connector.
func oauthPassportLabel(clientID string) string { return "oauth:" + clientID }

// reuseRevokeReason is what the audit row says when the cascade was triggered
// by detection rather than by a human.
const reuseRevokeReason = "refresh token reuse detected"

// passportRevokedReason is what the audit row says when a human killed the
// credential and the connection went with it, rather than the other way round.
const passportRevokedReason = "the passport issued under the grant was revoked"

// deactivatedUserRevokeReason is what the audit row says when the connection
// died because the human whose authority it borrows lost their own access.
const deactivatedUserRevokeReason = "the human who consented was deactivated"

// supersededRevokeReason is what the audit row says when the human consented
// again from the same client and the earlier connection made way for it.
const supersededRevokeReason = "superseded by a later consent from the same client"

// supersedePriorGrants ends the REGISTRATION's earlier connection, whoever
// consented to it — not only this human's own prior grant — because the file
// header's invariant is one row per client REGISTRATION, and a client_id
// re-authorized by a second human (a shared machine, a handed-off install)
// is still one registration changing hands, not two connections coexisting.
// issueGrant already holds oauth_client FOR UPDATE (lockClientRegistration)
// before calling here, which is what makes "no active grant yet" a fact
// rather than a snapshot two racing consents could both read; the query below
// orders by id so a registration with more than one prior grant releases its
// locks in a fixed sequence. revokeGrantTx takes the grant lock first
// regardless of whose grant it is, so entering through it preserves the
// cascade's existing lock order without needing the prior human's own
// app_user row.
func supersedePriorGrants(ctx context.Context, tx pgx.Tx, clientID string) error {
	rows, err := tx.Query(ctx, `
		SELECT id FROM oauth_grant
		WHERE client_id = $1 AND revoked_at IS NULL
		ORDER BY id`, clientID)
	if err != nil {
		return err
	}
	prior, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	for _, grantID := range prior {
		if err := revokeGrantTx(ctx, tx, grantID, supersededRevokeReason); err != nil {
			return err
		}
	}
	return nil
}
