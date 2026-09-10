// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Admin-side session management: a user_admin holder lists and ends the sessions
// open under ANOTHER member's account. The counterpart to sessions.go's
// self-service surface — where that half takes the user id off the bound
// principal and is ungated because it can reach no other account, this half
// names a target and so carries the object grant, the delegated-admin rank guard
// (a lesser admin may not surveil or end a full admin's sessions), and the
// not-found an out-of-scope row earns everywhere.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// UserSession is one session open under a member's account as an ADMIN sees it:
// the same view MySession gives its owner, minus `current` — the admin's own
// request is never one of the target's sessions, so there is nothing to mark.
type UserSession struct {
	ID           ids.UUID
	UserAgent    *string
	SignedInAt   time.Time
	LastActiveAt time.Time
}

// ListUserSessions returns another member's live sessions for a user_admin
// holder. An unknown target is not-found — checked explicitly, because the rank
// guard passes a full admin straight through without touching the target row.
func (s *Service) ListUserSessions(ctx context.Context, actor Identity, targetUserID ids.UserID) ([]UserSession, error) {
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionRead)
	if err != nil {
		return nil, err
	}
	var sessions []UserSession
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := refuseUnlessCallerOutranksTarget(ctx, tx, actor, targetUserID); err != nil {
			return err
		}
		if err := ensureUserExists(ctx, tx, targetUserID); err != nil {
			return err
		}
		rows, err := tx.Query(ctx,
			`SELECT id, user_agent, created_at, last_seen_at
			   FROM session
			  WHERE user_id = $1
			    AND revoked_at IS NULL
			    AND now() < idle_expires_at
			    AND now() < expires_at
			  ORDER BY last_seen_at DESC`,
			targetUserID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var us UserSession
			if err := rows.Scan(&us.ID, &us.UserAgent, &us.SignedInAt, &us.LastActiveAt); err != nil {
				return err
			}
			sessions = append(sessions, us)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("identity: list user sessions: %w", err)
	}
	return sessions, nil
}

// RevokeUserSession ends one of another member's sessions for a user_admin
// holder. A session id the target does not hold is not-found, so an admin
// cannot probe another workspace's ids by whose revoke lands, and an unknown
// target falls into the same answer for the same reason. The audit names the
// acting admin AND the target, because unlike self-revoke this is one principal
// acting on another.
func (s *Service) RevokeUserSession(ctx context.Context, actor Identity, targetUserID ids.UserID, sessionID ids.UUID) error {
	ctx, err := admit(ctx, actor, objectUserAdmin, principal.ActionDelete)
	if err != nil {
		return err
	}
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := refuseUnlessCallerOutranksTarget(ctx, tx, actor, targetUserID); err != nil {
			return err
		}
		var present bool
		err := tx.QueryRow(ctx,
			`SELECT true FROM session WHERE id = $1 AND user_id = $2`,
			sessionID, targetUserID).Scan(&present)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("identity: locate user session to revoke: %w", err)
		}
		tag, err := tx.Exec(ctx,
			`UPDATE session SET revoked_at = now()
			  WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
			sessionID, targetUserID)
		if err != nil {
			return fmt.Errorf("identity: revoke user session: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		return logAuthEvent(ctx, tx, actor.UserID, "session_revoked",
			fmt.Sprintf("ended a session of user %s", targetUserID))
	})
}

// ensureUserExists is the target-existence check the reads share: a live (not
// hard-archived) app_user, or not-found. Deactivated is still a target — an
// admin may review the sessions of an account they just suspended.
func ensureUserExists(ctx context.Context, tx pgx.Tx, userID ids.UserID) error {
	var present bool
	switch err := tx.QueryRow(ctx,
		`SELECT true FROM app_user WHERE id = $1 AND archived_at IS NULL`,
		userID).Scan(&present); {
	case errors.Is(err, pgx.ErrNoRows):
		return apperrors.ErrNotFound
	case err != nil:
		return fmt.Errorf("identity: confirm user exists: %w", err)
	}
	return nil
}
