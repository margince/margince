// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Which app_user a verified (provider, subject, email) tuple belongs to, and the
// session that follows. Split from ssologin.go, which owns the HTTP handlers and
// the cookie/state plumbing: this half never touches a request, and the two were
// already described as separate concerns in that file's own header.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ErrFederatedSignInRefused is the one neutral failure ssologin.go ever
// returns to its callers — deliberately indistinguishable between "no such
// email", "account not live", and "email not verified", for the same
// no-enumeration reason /auth/login refuses to distinguish "no such email"
// from "wrong password".
var ErrFederatedSignInRefused = errors.New("identity: federated sign-in refused")

// resolveFederatedUser answers which app_user a verified (provider, subject,
// email) tuple belongs to, and whether this is the first time this provider
// has been linked to that user. It tries (provider, subject) FIRST: an
// already-linked identity resolves without touching email at all. Only an
// UNLINKED subject falls back to email, through LiveMemberSQL AND
// password_hash IS NOT NULL — the same pair checkCredentials (lockout.go)
// and reset.go's forgot-password lookup already require.
//
// BOTH halves are load-bearing, and neither is redundant now that `invited` is
// a status the tree actually writes. LiveMemberSQL excludes an unredeemed
// invitation by status. password_hash IS NOT NULL excludes the AGENT SEAT,
// which installation.go seeds `active` with a NULL hash and which must never
// be a thing that signs in — and it also holds the line if a future path ever
// activates an account without setting a credential.
//
// Skipping either would let an unredeemed invitation, or the agent seat,
// become reachable by anyone who controls that address on the IdP, forever —
// no token, no expiry, unlike the invitation mail itself. That is the whole
// reason redemption, and not this branch, is what activates an account.
func (s *Service) resolveFederatedUser(ctx context.Context, tx pgx.Tx, provider, subject, email string) (userID ids.UserID, firstLink bool, err error) {
	var linkedUser ids.UserID
	err = tx.QueryRow(ctx,
		`SELECT fi.user_id FROM federated_identity fi
		 JOIN app_user u ON u.id = fi.user_id
		 WHERE fi.provider = $1 AND fi.subject = $2 AND `+LiveMemberSQL("u")+`
		 AND u.password_hash IS NOT NULL AND NOT u.is_agent`,
		provider, subject).Scan(&linkedUser)
	switch {
	case err == nil:
		return linkedUser, false, nil
	case errors.Is(err, pgx.ErrNoRows):
		// Either genuinely unlinked, or linked to a user who is no longer
		// live/activated — both fall through to email resolution below, and
		// both must land on the SAME refusal as an unrecognized password
		// login: a suspended, archived, un-activated, or agent account's
		// still-valid link must not read as a successful sign-in just
		// because the row exists.
	default:
		return ids.UserID{}, false, fmt.Errorf("identity: resolve federated identity: %w", err)
	}

	var byEmail ids.UserID
	err = tx.QueryRow(ctx,
		`SELECT id FROM app_user WHERE `+LiveMemberSQL("")+`
		 AND password_hash IS NOT NULL AND NOT is_agent AND lower(email) = lower($1)`,
		email).Scan(&byEmail)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UserID{}, false, ErrFederatedSignInRefused
	}
	if err != nil {
		return ids.UserID{}, false, fmt.Errorf("identity: resolve app_user by email: %w", err)
	}
	return byEmail, true, nil
}

// linkFederatedIdentity records the (provider, subject) -> user_id mapping.
// The table carries TWO unique constraints — (user_id, provider) and
// (provider, subject) — and this function answers to both. ON CONFLICT
// (user_id, provider) means a SUBJECT CHANGE for an existing link updates
// rather than errors: the email-recycling case, where a different Google
// account now presents the same verified email an old link used. That case
// is not silently indistinguishable from a normal login: the caller passes a
// distinct audit detail for it (see LoginViaFederatedIdentity).
//
// The DELETE before it answers the other constraint: the same (provider,
// subject) can already be linked to a DIFFERENT user_id — the row
// resolveFederatedUser found not live, not activated, or an agent seat, and
// fell through past to resolve a different (live, activated) user by email.
// That stale row still holds the (provider, subject) unique slot, so the
// insert below would hit federated_identity_provider_subject_key instead of
// the (user_id, provider) conflict target it's written for. Retiring it here
// transfers the subject to the user the caller already decided to sign in,
// rather than refusing a login on an internal constraint the caller never
// sees.
func linkFederatedIdentity(ctx context.Context, tx pgx.Tx, userID ids.UserID, provider, subject, email string) (wasRelink bool, err error) {
	var existingSubject string
	scanErr := tx.QueryRow(ctx,
		`SELECT subject FROM federated_identity WHERE user_id = $1 AND provider = $2`,
		userID, provider).Scan(&existingSubject)
	switch {
	case scanErr == nil:
		wasRelink = existingSubject != subject
	case errors.Is(scanErr, pgx.ErrNoRows):
		wasRelink = false
	default:
		return false, fmt.Errorf("identity: read existing federated identity: %w", scanErr)
	}

	if _, err := tx.Exec(ctx,
		`DELETE FROM federated_identity WHERE provider = $1 AND subject = $2 AND user_id <> $3`,
		provider, subject, userID); err != nil {
		return false, fmt.Errorf("identity: retire stale federated identity: %w", err)
	}

	_, err = tx.Exec(ctx,
		`INSERT INTO federated_identity (user_id, provider, subject, email_at_link)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (user_id, provider)
		 DO UPDATE SET subject = EXCLUDED.subject, email_at_link = EXCLUDED.email_at_link`,
		userID, provider, subject, email)
	if err != nil {
		return false, fmt.Errorf("identity: link federated identity: %w", err)
	}
	return wasRelink, nil
}

// LoginViaFederatedIdentity resolves a verified (provider, subject, email)
// tuple to a session, mirroring Service.Login's shape: mint the token first,
// then one transaction that links/resolves, mints the session row, and
// audits — the same unexported session helpers Login already uses, no
// parallel implementation. Sessions carry no workspace column (ADR-0091 §8),
// so unlike Login this needs no bound installation context.
func (s *Service) LoginViaFederatedIdentity(ctx context.Context, provider, subject, email string) (string, error) {
	rawToken, tokenHash, err := mintSessionToken()
	if err != nil {
		return "", fmt.Errorf("identity: mint session token: %w", err)
	}

	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		userID, firstLink, resolveErr := s.resolveFederatedUser(ctx, tx, provider, subject, email)
		if resolveErr != nil {
			return resolveErr
		}
		wasRelink, linkErr := linkFederatedIdentity(ctx, tx, userID, provider, subject, email)
		if linkErr != nil {
			return linkErr
		}
		if insErr := insertSession(ctx, tx, userID, tokenHash); insErr != nil {
			return fmt.Errorf("identity: insert session: %w", insErr)
		}
		detail := fmt.Sprintf("oidc login: %s", provider)
		switch {
		case wasRelink:
			detail = fmt.Sprintf("oidc re-link: %s (subject changed)", provider)
		case firstLink:
			detail = fmt.Sprintf("oidc login: %s (first link)", provider)
		}
		return auditLogin(ctx, tx, userID, detail)
	})
	if err != nil {
		if errors.Is(err, ErrFederatedSignInRefused) {
			return "", ErrFederatedSignInRefused
		}
		return "", err
	}
	return rawToken, nil
}
