// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The second half of an MFA sign-in. A password login that lands on a member
// with an active factor mints no session; it stops with errMFARequired, the
// handler hands back a signed challenge, and the member returns it here with a
// code. The challenge is stateless — an HMAC the composition root signs — so it
// binds the member and expires without a row to keep.

import (
	"context"
	"errors"
	"fmt"
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

// MFAChallengeSigner signs the token that stands in for "this member passed the
// password step and owes a second factor". Defined here so identity never
// imports compose — the HMAC and its key live in the composition root, the same
// arrangement OIDCStateSigner uses.
type MFAChallengeSigner interface {
	Sign(userID string, ttl time.Duration) string
	Verify(token string) (userID string, err error)
}

// CompleteMFAChallenge verifies a second factor for a member the password step
// already identified — via the signed challenge the handler checked — and mints
// the session that password-plus-factor earns. Pre-auth by construction: the
// challenge token is the authorization, so there is no principal to gate on,
// the same posture checkCredentials has on the password path. A wrong code is a
// neutral errBadMFACode.
func (s *Service) CompleteMFAChallenge(ctx context.Context, userID ids.UserID, code string) (Identity, string, error) {
	rawWsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		return Identity{}, "", ErrBadCredentials
	}
	wsID := ids.From[ids.WorkspaceKind](rawWsID)
	token, tokenHash, err := mintSessionToken()
	if err != nil {
		return Identity{}, "", err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
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
		return auditLogin(ctx, tx, userID, "password + second factor login")
	})
	if err != nil {
		return Identity{}, "", err
	}
	// Build the response identity from the session just minted, so the shape the
	// caller gets is exactly what /me and the password path already produce —
	// one Identity assembler, not a second spelled here.
	id, err := s.Authenticate(ctx, token)
	if err != nil {
		return Identity{}, "", fmt.Errorf("identity: resolve session after mfa: %w", err)
	}
	return id, token, nil
}
