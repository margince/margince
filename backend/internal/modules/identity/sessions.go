// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// Self-service session management: a signed-in human lists the sessions open
// under their own account and ends any one of them. The read and the revoke
// both scope every statement to the caller's user_id — a session is not a
// first-class entity with a row-scope gate of its own (ADR-0043: it is keyed by
// its opaque token), so the user_id predicate IS the gate, and another member's
// session id lands on the same not-found a stranger's record earns everywhere.
//
// The device a session records is captured at creation, not here — see
// withUserAgent below.

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httpserver"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MySession is one open session as its owner sees it: which device opened it
// and when, when it was last active, and whether it is the one making this very
// request. The opaque token and its hash never appear — the id here is the
// session row's own uuid, the handle a revoke names. No IP: the row carries one
// but the list deliberately withholds it (a coarser view than the address).
type MySession struct {
	ID           ids.UUID
	UserAgent    *string
	SignedInAt   time.Time
	LastActiveAt time.Time
	Current      bool
}

// revokeSessionPathPrefix is the mounted address of DELETE /me/sessions/{id},
// up to the id. The trailing slash is load-bearing: it matches the item route a
// revoke takes and NOT the collection GET beside it, which is a safe read no
// exemption is owed.
const revokeSessionPathPrefix = httpserver.BaseURL + "/me/sessions/"

// isOwnSessionRevoke reports the one further mutating call a read seat may make
// beyond replacing its password: ending one of its own sessions. The store
// scopes the revoke to the caller, so the method and path are all this admission
// test needs — ownership is proven where the row is, not here.
func isOwnSessionRevoke(r *http.Request) bool {
	return r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, revokeSessionPathPrefix)
}

// userAgentContextKey carries the request's User-Agent from the login handler
// down to insertSession, which records it on the session row.
//
// It rides the context rather than Login's signature because a User-Agent is a
// transport detail, not a domain argument: threading it through Login would push
// an HTTP header into the thirty-odd call sites that open a session with no
// request to name it. The login handlers set it; insertSession reads it.
type userAgentContextKey struct{}

// withUserAgent binds the request's User-Agent so the session opened later in
// the same context records the device it came from.
func withUserAgent(ctx context.Context, userAgent string) context.Context {
	return context.WithValue(ctx, userAgentContextKey{}, userAgent)
}

// userAgentFromContext returns the captured User-Agent, or nil when none was
// set or the header was empty. A missing device is stored as NULL, not as an
// empty string, so "we never learned it" and "the client sent a blank one" stay
// distinguishable — the same reason locale and timezone keep NULL off app_user.
func userAgentFromContext(ctx context.Context) *string {
	userAgent, ok := ctx.Value(userAgentContextKey{}).(string)
	if !ok || userAgent == "" {
		return nil
	}
	return &userAgent
}

// ListSessions returns the caller's own live sessions, newest activity first,
// marking the one whose token hashes to currentHash. The user id comes off the
// bound principal, never a parameter, so there is no other account it could
// reach — the same self-scoping ChangePassword and MyWorkingHours rely on.
// Expired and revoked sessions are excluded by the same predicates Authenticate
// applies, so the list shows exactly what a request could still be admitted on.
func (s *Service) ListSessions(ctx context.Context, currentHash string) ([]MySession, error) {
	human, err := selfScoped(ctx)
	if err != nil {
		return nil, err
	}
	userID := ids.From[ids.UserKind](human)
	var sessions []MySession
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT id, user_agent, created_at, last_seen_at, token_hash = $2
			   FROM session
			  WHERE user_id = $1
			    AND revoked_at IS NULL
			    AND now() < idle_expires_at
			    AND now() < expires_at
			  ORDER BY last_seen_at DESC`,
			userID, currentHash)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var session MySession
			if err := rows.Scan(&session.ID, &session.UserAgent,
				&session.SignedInAt, &session.LastActiveAt, &session.Current); err != nil {
				return err
			}
			sessions = append(sessions, session)
		}
		return rows.Err()
	})
	if err != nil {
		return nil, fmt.Errorf("identity: list sessions: %w", err)
	}
	return sessions, nil
}

// RevokeSession ends one of the caller's own sessions. The user id comes off the
// bound principal, never a parameter, so the revoke can reach no other account.
// A session id that is not this user's — including one that does not exist — is
// ErrNotFound, so who may revoke a session never discloses whether it is real.
// Revoking a session the caller already ended is a no-op, the same idempotence
// Logout gives the cookie path; only a revoke that closes a live session records
// the fact.
func (s *Service) RevokeSession(ctx context.Context, sessionID ids.UUID) error {
	human, err := selfScoped(ctx)
	if err != nil {
		return err
	}
	userID := ids.From[ids.UserKind](human)
	return s.db.Tx(ctx, func(tx pgx.Tx) error {
		var present bool
		err := tx.QueryRow(ctx,
			`SELECT true FROM session WHERE id = $1 AND user_id = $2`,
			sessionID, userID).Scan(&present)
		if errors.Is(err, pgx.ErrNoRows) {
			return apperrors.ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("identity: locate session to revoke: %w", err)
		}

		tag, err := tx.Exec(ctx,
			`UPDATE session SET revoked_at = now()
			  WHERE id = $1 AND user_id = $2 AND revoked_at IS NULL`,
			sessionID, userID)
		if err != nil {
			return fmt.Errorf("identity: revoke session: %w", err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		return logAuthEvent(ctx, tx, userID, "session_revoked", "session ended by its owner")
	})
}
