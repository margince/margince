// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// The password sign-in: how an email and a password become a session. Split
// from service.go so the login gates — SSO enforcement, the second-factor stop,
// the failure audit — read as one piece.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LoginSession is what a successful password login hands the browser: the
// session token its cookie carries, and a device proof that exempts this
// browser from a §27 lock on the same account later (see logindevice.go).
type LoginSession struct {
	Token       string
	DeviceProof string
}

// Login verifies credentials inside the tenant transaction and mints an
// opaque session. Every attempt outcome — success or failure — lands in
// audit_log (the failure row commits in its own transaction, because the
// attempt's transaction rolls back with the error). device is the proof the
// browser presented from an earlier login, empty for none; it matters only
// while the account is §27-locked (see checkCredentials).
func (s *Service) Login(ctx context.Context, email, plaintext, device string) (Identity, LoginSession, error) {
	rawWsID, ok := principal.WorkspaceID(ctx)
	if !ok {
		// The middleware binds the singleton company on every request
		// (installation.go); an unbound context means the installation is
		// not bootstrapped — and the answer must not disclose that:
		// credentials against a not-yet-existing company read exactly
		// like wrong credentials.
		return Identity{}, LoginSession{}, ErrBadCredentials
	}
	wsID := ids.From[ids.WorkspaceKind](rawWsID)
	// Read before the credential check so the outcome of a genuine outage is a
	// refused login rather than a session minted against an unknown policy.
	sso, err := s.enforcedSSO(ctx)
	if err != nil {
		return Identity{}, LoginSession{}, err
	}
	token, tokenHash, err := mintSessionToken()
	if err != nil {
		return Identity{}, LoginSession{}, err
	}

	var id Identity
	session := LoginSession{Token: token}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		account, err := s.checkCredentials(ctx, tx, email, plaintext, device)
		if err != nil {
			return err
		}
		id = Identity{UserID: account.UserID, WorkspaceID: wsID, Email: email, DisplayName: account.DisplayName, SeatType: account.SeatType}
		var loadErr error
		id.Roles, id.Teams, id.Permissions, loadErr = loadGrants(ctx, tx, account.UserID)
		if loadErr != nil {
			return loadErr
		}
		if err := s.enforceSignInPolicy(ctx, tx, id, sso); err != nil {
			return err
		}
		if err := insertSession(ctx, tx, account.UserID, tokenHash); err != nil {
			return err
		}
		if err := auditLogin(ctx, tx, account.UserID, "password login"); err != nil {
			return err
		}
		// Reissued only once the gates have passed: a login the SSO or MFA gate
		// refuses is not a sign-in, and must not hand the browser a standing
		// exemption from the §27 lock.
		session.DeviceProof = account.DeviceProof
		// Failing here would answer correct credentials with a 500 while
		// wrong ones still got 401 — telling an attacker which passwords
		// are right — so InstallationNameOf coalesces an absent row to
		// the empty string rather than erroring.
		var nameErr error
		id.WorkspaceName, nameErr = InstallationNameOf(ctx, tx)
		return nameErr
	})
	if errors.Is(err, errAccountLocked) {
		// A §27-locked account is refused, but INDISTINGUISHABLY from bad
		// credentials: same 401, same body, same Argon2 timing (the decoy
		// verify already ran in checkCredentials). It is deliberately NOT
		// run through recordFailedLogin — a probe against a locked account
		// must neither extend its own lock nor append another failure row
		// (an attacker-drivable DoS, and a distinct audit cadence would
		// itself be an oracle). The in-memory per-IP+email limiter still
		// counts it (the handler Records every 401), so a locked account is
		// no longer a rate-limit blind spot either.
		return Identity{}, LoginSession{}, ErrBadCredentials
	}
	if errors.Is(err, ErrBadCredentials) {
		// The attempt's transaction rolled back with the error, so the
		// failure audit needs its own transaction — an invisible
		// brute-force is exactly what the audit trail exists to catch.
		// A failure writing it outranks the 401.
		if auditErr := s.recordFailedLogin(ctx, email); auditErr != nil {
			return Identity{}, LoginSession{}, auditErr
		}
		return Identity{}, LoginSession{}, err
	}
	if errors.Is(err, errMFARequired) {
		// Password verified; a second factor is owed. The id carries the user the
		// challenge must bind to — populated before the gate rolled the tx back —
		// but no session token: that waits for /auth/mfa.
		return id, LoginSession{}, errMFARequired
	}
	if err != nil {
		return Identity{}, LoginSession{}, err
	}
	return id, session, nil
}

// enforceSignInPolicy applies the two installation gates that stand between a
// verified password and a session — enforced SSO and a required second factor —
// on the already-authenticated Identity, inside the login transaction so a
// refusal rolls back before any session is minted. Split out of Login to keep
// that function over the credential/session/audit spine rather than the policy
// branches; each gate's own reasoning lives at its return below.
func (s *Service) enforceSignInPolicy(ctx context.Context, tx pgx.Tx, id Identity, sso bool) error {
	// Enforced SSO closes the password path to everyone but an admin — with the
	// SAME refusal a wrong password earns. This branch runs only after the
	// credential check has passed, so any distinct answer here would confirm the
	// password was right: a 403 in place of the 401 turns enforcement into an
	// oracle that verifies guessed passwords for every non-admin member. The
	// refused attempt keeps the ordinary failure accounting (audit row, per-IP
	// and per-account throttles) for the same reason — a different cadence would
	// be the same oracle by a side door.
	if sso && !id.hasRole(roleAdmin) {
		return ErrBadCredentials
	}
	// A member with an active second factor gets no session from the password
	// alone: the flow stops here and resumes at /auth/mfa. Checked after the SSO
	// gate so an admin break-glass login is still challenged for its factor.
	//
	// Only where a vault exists: with none, /auth/mfa could never verify the
	// factor, so challenging would turn a misconfigured deployment (enrolled
	// members, seal since removed) into a lockout of exactly those members. The
	// composition root reports that state at boot; here the password alone
	// completes, the posture the account had before it enrolled.
	if s.vault == nil {
		return nil
	}
	mfaConfirmed, err := hasConfirmedMFA(ctx, tx, id.UserID)
	if err != nil {
		return err
	}
	if mfaConfirmed {
		return errMFARequired
	}
	return nil
}
