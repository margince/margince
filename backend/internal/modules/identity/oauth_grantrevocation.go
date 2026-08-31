// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The revocation cascade for a connection's rows (oauth_grant, its refresh
// chain, and the passports minted under it), read by the RFC 7009 handler in
// oauth_revoke.go and by every path that ends a connection. Split out of
// oauth_grant.go, which owns issuing a grant and superseding a prior one, so
// revocation's own lock-order essay sits beside the first lock it documents
// rather than above the issuance code it also binds.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The LOCK ORDER for a connection's rows, obeyed by every path that touches
// more than one of them: oauth_grant first, then oauth_refresh_token, then
// passport. Rotation and revokeGrantTx both take them in that order, so a
// human revoking at the instant a connector renews QUEUES on the grant row
// instead of deadlocking against it. DESIGN §5.4's lock exists to serialize
// every concurrent presentation *and any racing revoke*, and a deadlock —
// Postgres aborting one side with a 500 — is not serialization. A new path
// that takes a refresh row before its grant re-opens exactly that hole.
//
// app_user sits AHEAD of all three: requireLiveConsentingUser locks it before
// the grant insert below, and DeactivateUser locks it before revoking the
// human's own passports (users.go). A consent itself takes no passport lock at
// all — the human's ticks are not a row that can go stale between render and
// submit — so the chain a consent can deadlock against is only this one. A
// path that takes a passport or refresh row and then reaches for app_user
// inverts that chain and deadlocks against a deactivation.
//
// Which is why a path that ALREADY holds the grant must not lock app_user at all:
// it is past app_user's place in the chain, so it reads the human's row unlocked
// and decides from the grant lock it holds (lockPresentedRefreshToken's
// FOR UPDATE OF r, g — oauth_refresh.go states why an MVCC read suffices there).

// lockGrant takes that connection-level lock: the FIRST lock any such path
// acquires. It reads no columns on purpose — a grant's state is read
// authoritatively after this, under the lock. pgx.ErrNoRows passes through so
// each caller answers an absent grant in its own vocabulary.
func lockGrant(ctx context.Context, tx pgx.Tx, grantID ids.UUID) error {
	var locked ids.UUID
	return tx.QueryRow(ctx,
		`SELECT id FROM oauth_grant WHERE id = $1 FOR UPDATE`, grantID).Scan(&locked)
}

// revokeGrantTx ends a whole connection inside the caller's transaction: the
// consent, every refresh token that could renew it, and every passport it
// issued. It is the ONE cascade, and four paths reach these three writes:
// detected refresh-token reuse (rotateRefreshToken), a human deleting a
// passport (RevokePassport), a client revoking its own credential
// (revokeToken, RFC 7009) and an admin deactivating the human whose authority
// the connection borrows (revokeGrantsOfUserTx). So no path can end a
// connection halfway and none can leave refresh able to resurrect it.
//
// A client disabled or deleted OUT OF BAND — there is no admin client surface
// yet, so that is raw SQL today — reaches no cascade at all: what stops it is
// the liveness predicate every read of oauth_client carries (liveClientPredicate,
// oauth.go), which refuses authentication, renewal and fresh consent alike.
//
// The actor must already be bound on ctx (actorCtx): the audit row names
// whose action this was, and storekit refuses an unattributed write rather
// than record an anonymous revocation.
//
// Idempotent — the revocation of an already-revoked grant is audited once and
// re-emits nothing, because every write below is conditional on the row it
// touches still being live.
func revokeGrantTx(ctx context.Context, tx pgx.Tx, grantID ids.UUID, reason string) error {
	// Grant row FIRST, then the refresh rows, then the passports — the lock
	// order stated above, which rotation also takes, so the two paths queue
	// instead of deadlocking. Taking it EXPLICITLY, rather than letting the
	// conditional UPDATE below take it as a side effect, is what lets every
	// caller inherit the order simply by entering here: the UPDATE locks
	// nothing on a grant that is already revoked, and this function still
	// walks the rows beneath it in that case.
	if err := lockGrant(ctx, tx, grantID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// The FK from passport (and from oauth_refresh_token) to oauth_grant
			// is RESTRICT precisely so a live credential cannot outlive the
			// consent record that authorized it. An absent grant with rows to
			// revoke beneath it is therefore a broken invariant, not a caller
			// mistake, and must not read as "revoked successfully".
			return fmt.Errorf("identity: cannot revoke grant %s: the grant row is absent", grantID)
		}
		return err
	}
	// The conditional UPDATE is the serialization point for the AUDIT: two
	// simultaneous revokes queue on the lock above and only the first sees the
	// grant live, so one revocation is recorded once. The row walks below are
	// idempotent on their own row state, so a second revoke arriving from
	// another direction re-checks them and finds nothing left to do.
	tag, err := tx.Exec(ctx,
		`UPDATE oauth_grant SET revoked_at = now() WHERE id = $1 AND revoked_at IS NULL`, grantID)
	if err != nil {
		return err
	}
	// A refresh row has no revoked_at of its own: consumed_at IS the spend
	// marker, and a token whose grant is dead is refused on the liveness
	// check before the replay rule ever reads it — so marking the chain spent
	// closes renewal for good without a second column meaning the same thing.
	if _, err := tx.Exec(ctx,
		`UPDATE oauth_refresh_token SET consumed_at = now() WHERE grant_id = $1 AND consumed_at IS NULL`,
		grantID); err != nil {
		return err
	}
	if err := revokeGrantPassportsTx(ctx, tx, grantID); err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return nil // already revoked: the rows beneath it were re-checked, the fact is on record
	}
	// The reason is evidence ABOUT the revocation, not a field of the grant,
	// so it rides evidence rather than the after image. The closed catalog
	// (events.md §5) defines no oauth_grant.* verb, so the passport.revoked
	// events above are the bus-visible half of this cascade — a consumer
	// holding a credential learns it died; the missing grant-level type is
	// raised upstream (P3).
	_, err = storekit.AuditWithEvidence(ctx, tx, "archive", "oauth_grant", grantID, nil, nil,
		map[string]any{"reason": reason})
	return err
}

// revokeGrantsOfUserTx ends every connection one human consented to, through
// the ONE cascade, inside the caller's transaction. It is what a path that ends
// a HUMAN's access calls: revoking their passports alone is not enough,
// because the grant beneath one can mint a replacement on the connector's next
// renewal and each renewal slides the refresh window forward, so a connection
// nobody is watching outlives the access it borrows.
//
// It enumerates the GRANTS, not the passports beneath them: a grant whose
// passports are all already revoked still has a spendable refresh chain, so
// "this human holds no live passport" is not "this human has no live
// connection". Ordering by id makes the lock sequence deterministic when a
// human consented more than once.
func revokeGrantsOfUserTx(ctx context.Context, tx pgx.Tx, userID ids.UserID, reason string) error {
	rows, err := tx.Query(ctx,
		`SELECT id FROM oauth_grant WHERE user_id = $1 AND revoked_at IS NULL ORDER BY id`, userID)
	if err != nil {
		return err
	}
	// Collected before the first write: the cascade below runs statements on
	// this same connection, which an open row set would be reading from.
	grantIDs, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return err
	}
	for _, grantID := range grantIDs {
		if err := revokeGrantTx(ctx, tx, grantID, reason); err != nil {
			return err
		}
	}
	return nil
}

// revokeGrantPassportsTx kills every live passport under a grant and puts
// each death on the bus. Rotation uses it to retire the predecessor and the
// cascade uses it to end the connection, so "the credentials under this
// consent stop working" has one spelling. The event is per passport because a
// long-lived holder has to drop THAT credential — not learn that some
// connection changed.
func revokeGrantPassportsTx(ctx context.Context, tx pgx.Tx, grantID ids.UUID) error {
	by, err := revokingUser(ctx)
	if err != nil {
		return err
	}
	rows, err := tx.Query(ctx,
		`UPDATE passport SET revoked_at = now()
		 WHERE oauth_grant_id = $1 AND revoked_at IS NULL
		 RETURNING id`, grantID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var revoked []ids.PassportID
	for rows.Next() {
		var passportID ids.PassportID
		if err := rows.Scan(&passportID); err != nil {
			return err
		}
		revoked = append(revoked, passportID)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	// The audit + event rows are written after the walk, not inside it: the
	// connection is busy while rows are being read.
	for _, passportID := range revoked {
		if err := auditPassportRevoked(ctx, tx, passportID, by); err != nil {
			return err
		}
	}
	return nil
}

// revokingUser reads the human a cascade is attributed to back off the
// context, so the passport.revoked payload names them — the same principal
// the audit row is stamped from, never a second guess at who acted.
func revokingUser(ctx context.Context) (ids.UserID, error) {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return ids.UserID{}, err
	}
	return ids.From[ids.UserKind](actor.UserID), nil
}

// revokeTokenInput is a presented token as RFC 7009 places it on the wire:
// the token itself and an optional hint at which kind it is.
type revokeTokenInput struct {
	Token         string
	TokenTypeHint string
}

// clientRevokeReason is what the audit row says when the CLIENT ended the
// connection from its own side (RFC 7009) — distinct from a human deleting a
// passport in Settings (passportRevokedReason) and from detected
// refresh-token reuse (reuseRevokeReason).
const clientRevokeReason = "revoked via RFC 7009 /oauth/revoke"

// revokeToken is RFC 7009 revocation: whichever half of a connection's
// credential pair was presented, the whole connection dies through the ONE
// cascade. Resolution is unlocked exactly like grantOfPresentedToken — it
// names which grant to lock and decides nothing else, so revokeGrantTx alone
// takes the grant → refresh → passport order (oauth_grant.go) and no read
// here can invert it by locking a passport or refresh row first.
//
// An unresolved token — unknown, from another workspace, or naming no grant
// at all (a locally minted passport answers to no connection) — commits
// nothing and reports success exactly like a genuine revocation: RFC 7009
// forbids this endpoint from ever becoming an oracle for whether a token
// string is real.
func (s *Service) revokeToken(ctx context.Context, in revokeTokenInput) error {
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		grantID, err := resolveGrantID(ctx, tx, in)
		if err != nil {
			return err
		}
		if grantID == ids.Nil {
			return nil
		}
		var userID ids.UserID
		if err := tx.QueryRow(ctx, `SELECT user_id FROM oauth_grant WHERE id = $1`, grantID).
			Scan(&userID); err != nil {
			return err
		}
		// The human attributed is the one who consented, not whoever
		// presented the token — there is no session on this call, exactly as
		// a refresh rotation has none (lockedGrant.identity()).
		return revokeGrantTx(actorCtx(ctx, Identity{UserID: userID}), tx, grantID, clientRevokeReason)
	})
}

// resolveGrantID names which grant a presented token belongs to, checking
// passport and oauth_refresh_token by hash — the two tables a client may hand
// back either half of a connection's credential pair from. token_type_hint
// only orders which table is tried first: a miss on the hinted table still
// falls through to the other, so a wrong hint never turns into a refusal
// (RFC 7009 §2.1 — the hint "SHOULD" be honored, but a server "MUST NOT" rely
// on it as authoritative).
func resolveGrantID(ctx context.Context, tx pgx.Tx, in revokeTokenInput) (ids.UUID, error) {
	hash := hashToken(in.Token)
	if in.TokenTypeHint == oauthRefreshToken {
		if grantID, err := refreshGrantID(ctx, tx, hash); err != nil || grantID != ids.Nil {
			return grantID, err
		}
		return passportGrantID(ctx, tx, hash)
	}
	if grantID, err := passportGrantID(ctx, tx, hash); err != nil || grantID != ids.Nil {
		return grantID, err
	}
	return refreshGrantID(ctx, tx, hash)
}

// passportGrantID resolves a presented access token to the grant it was
// issued under. A locally minted passport — one with no OAuth grant beneath
// it — reports no grant: this endpoint's one cascade ends a CONNECTION, and a
// passport with no connection to revoke is, for this purpose, indistinguishable
// from an unknown token.
func passportGrantID(ctx context.Context, tx pgx.Tx, hash string) (ids.UUID, error) {
	var grantID *ids.UUID
	err := tx.QueryRow(ctx, `SELECT oauth_grant_id FROM passport WHERE token_hash = $1`, hash).
		Scan(&grantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	if err != nil {
		return ids.Nil, err
	}
	if grantID == nil {
		return ids.Nil, nil
	}
	return *grantID, nil
}

// refreshGrantID resolves a presented refresh token to the grant it renews.
// Unlocked for the same reason grantOfPresentedToken is (oauth_refresh.go):
// naming the grant is all a resolution read may decide — the lock itself is
// revokeGrantTx's, taken in the one order this connection's rows obey.
func refreshGrantID(ctx context.Context, tx pgx.Tx, hash string) (ids.UUID, error) {
	var grantID ids.UUID
	err := tx.QueryRow(ctx, `SELECT grant_id FROM oauth_refresh_token WHERE token_hash = $1`, hash).
		Scan(&grantID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.Nil, nil
	}
	return grantID, err
}
