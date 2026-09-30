// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The second half of an MFA sign-in. A password login that lands on a member
// with an active factor mints no session; it stops with errMFARequired, the
// handler hands back a signed challenge, and the member returns it here with a
// code. The challenge is stateless to VERIFY — an HMAC the composition root
// signs — but single-use to SPEND: each carries a random nonce, consumed in the
// session-minting transaction, so one challenge opens at most one session.

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// errMFARequired stops a password login one step short of a session: the
// credentials were right, but the member owes a second factor. It is not a
// failure — the handler answers it with a 202 challenge, not a 401.
var errMFARequired = errors.New("identity: a second factor is required")

// mfaChallengeTTL bounds how long a 202 challenge is good for. Short, because it
// is minted the instant the password verifies and spent seconds later; a stale
// one is a replay window with no reason to stay open.
const mfaChallengeTTL = 5 * time.Minute

// spentChallengeReapLimit bounds the opportunistic sweep of expired
// spent-challenge nonces. Each completion deletes at most this many dead rows
// in its own transaction — enough to keep the table bounded at any plausible
// login rate without ever turning one sign-in into an unbounded scan.
const spentChallengeReapLimit = 100

// MFAChallengeSigner signs the token that stands in for "this member passed the
// password step and owes a second factor". Defined here so identity never
// imports compose — the HMAC and its key live in the composition root, the same
// arrangement OIDCStateSigner uses. Every minted token carries a fresh random
// nonce (jti); Verify returns it so the service can spend it exactly once.
type MFAChallengeSigner interface {
	Sign(userID string, ttl time.Duration) (token string, err error)
	Verify(token string) (userID, jti string, err error)
}

// WithRequireMFA injects the require-MFA policy reader, read at admission so an
// admin turning it on takes effect without a restart. Unset leaves MFA optional.
func (s *Service) WithRequireMFA(fn func(ctx context.Context) (bool, error)) *Service {
	s.requireMFA = fn
	return s
}

// mfaMandatory reports whether the installation requires a second factor. An
// unwired reader is "not required"; a read that fails propagates, so a policy
// outage confines rather than silently admits.
//
// A service with no vault also answers "not required", whatever the stored
// policy says: with no seal there is no enrolment, so confining a member to
// enrolment routes that can only refuse would lock every factorless member out
// of the whole product with no way back in. The composition root reports the
// misconfiguration at boot; admission must not convert it into a lockout.
func (s *Service) mfaMandatory(ctx context.Context) (bool, error) {
	if s.requireMFA == nil || s.vault == nil {
		return false, nil
	}
	return s.requireMFA(ctx)
}

// CompleteMFAChallenge verifies a second factor for a member the password step
// already identified — via the signed challenge the handler checked — and mints
// the session that password-plus-factor earns. Pre-auth by construction: the
// challenge token is the authorization, so there is no principal to gate on,
// the same posture checkCredentials has on the password path. A wrong code is a
// neutral errBadMFACode, and so is a challenge nonce that was already spent.
//
// Everything the response needs — the session row, the audit fact, the spent
// nonce and the member's resolved Identity — commits or rolls back as ONE
// transaction. Resolving the identity afterwards (as a fresh Authenticate
// would) could fail AFTER the commit, answering the member an error while a
// live session and a success audit row already exist.
func (s *Service) CompleteMFAChallenge(ctx context.Context, userID ids.UserID, jti, code string) (Identity, string, error) {
	rawWsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return Identity{}, "", ErrBadCredentials
	}
	if jti == "" {
		// A signer that mints no nonce cannot make its challenges single-use;
		// refuse rather than admit an infinitely replayable token.
		return Identity{}, "", errBadMFACode
	}
	wsID := ids.From[ids.WorkspaceKind](rawWsID)
	token, tokenHash, err := mintSessionToken()
	if err != nil {
		return Identity{}, "", err
	}
	var id Identity
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := spendMFAChallenge(ctx, tx, jti); err != nil {
			return err
		}
		verified, err := s.verifyMFA(ctx, tx, wsID, userID, code)
		if err != nil {
			return err
		}
		if !verified {
			return errBadMFACode
		}
		if err := insertSession(ctx, tx, userID, tokenHash); err != nil {
			return err
		}
		if err := auditLogin(ctx, tx, userID, "password + second factor login"); err != nil {
			return err
		}
		var idErr error
		id, idErr = signedInIdentity(ctx, tx, wsID, userID)
		return idErr
	})
	if err != nil {
		return Identity{}, "", err
	}
	return id, token, nil
}

// spendMFAChallenge consumes a challenge's one-time nonce: the first completion
// inserts it, and any later presentation of the same challenge collides and is
// refused with the same neutral errBadMFACode a wrong code earns. The row's
// expires_at is an upper bound on how long the ledger must remember the nonce
// (past the signature's own TTL a replay cannot verify anyway), taken from the
// database clock like every other deadline this package writes. The bounded
// sweep of expired nonces rides the same transaction rather than a job: a
// dedicated reaper for rows that die within minutes would be machinery without
// a tenant.
func spendMFAChallenge(ctx context.Context, tx pgx.Tx, jti string) error {
	if _, err := tx.Exec(ctx,
		`DELETE FROM mfa_challenge_spent
		  WHERE jti IN (SELECT jti FROM mfa_challenge_spent WHERE expires_at < now() LIMIT $1)`,
		spentChallengeReapLimit); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		`INSERT INTO mfa_challenge_spent (jti, expires_at) VALUES ($1, now() + $2::interval)
		 ON CONFLICT (jti) DO NOTHING`,
		jti, mfaChallengeTTL.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return errBadMFACode
	}
	return nil
}

// signedInIdentity assembles the Identity a fresh sign-in answers with, inside
// the caller's transaction — the same shape Login builds inline (which reads
// the member row through its credential check, so the two cannot share one
// query). A member who stopped being live between the password step and the
// second factor is refused as neutrally as a wrong code: the challenge outlives
// the standing it was minted under, and must not out-vote it.
func signedInIdentity(ctx context.Context, tx pgx.Tx, wsID ids.WorkspaceID, userID ids.UserID) (Identity, error) {
	id := Identity{UserID: userID, WorkspaceID: wsID}
	err := tx.QueryRow(ctx,
		`SELECT email, display_name, seat_type FROM app_user
		  WHERE id = $1 AND `+LiveMemberSQL("")+``,
		userID).Scan(&id.Email, &id.DisplayName, &id.SeatType)
	if errors.Is(err, pgx.ErrNoRows) {
		return Identity{}, errBadMFACode
	}
	if err != nil {
		return Identity{}, err
	}
	if id.Roles, id.Teams, id.Permissions, err = loadGrants(ctx, tx, userID); err != nil {
		return Identity{}, err
	}
	// Coalesced to empty for the reason Login's read is: failing on an unnamed
	// installation would answer a correct second factor with a 500.
	id.WorkspaceName, err = InstallationNameOf(ctx, tx)
	return id, err
}
