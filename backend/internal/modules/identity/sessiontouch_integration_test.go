// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package identity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

func TestAuthenticateTouchesSessionWithAbsoluteExpiryCap(t *testing.T) {
	for _, capped := range []bool{false, true} {
		name := "idle-window"
		if capped {
			name = "absolute-cap"
		}
		t.Run(name, func(t *testing.T) {
			e := setupRevocationEnv(t, "touch-"+name)
			token := e.signIn(t, "device")
			lifetime := absoluteTTL
			if capped {
				lifetime = idleTTL / 2
			}
			var previous, absolute time.Time
			err := e.owner.QueryRow(t.Context(), `UPDATE session
    SET last_seen_at = now() - interval '1 hour',
        idle_expires_at = now() + interval '1 hour',
        expires_at = now() + @lifetime::interval
    WHERE token_hash = @token RETURNING last_seen_at, expires_at`,
				pgx.StrictNamedArgs{"lifetime": lifetime.String(), "token": hashToken(token)}).Scan(&previous, &absolute)
			if err != nil {
				t.Fatal(err)
			}
			id, err := e.svc.Authenticate(e.wsOnlyCtx(), token)
			if err != nil {
				t.Fatal(err)
			}
			if id.UserID != e.member.UserID {
				t.Fatalf("authenticated user = %s, want %s", id.UserID, e.member.UserID)
			}
			var seen, idle, expires time.Time
			err = e.owner.QueryRow(t.Context(), `SELECT last_seen_at, idle_expires_at, expires_at
    FROM session WHERE token_hash = @token`, pgx.StrictNamedArgs{"token": hashToken(token)}).Scan(&seen, &idle, &expires)
			if err != nil {
				t.Fatal(err)
			}
			wantIdle := seen.Add(idleTTL)
			if capped {
				wantIdle = absolute
			}
			if !seen.After(previous) || !idle.Equal(wantIdle) || !expires.Equal(absolute) {
				t.Fatalf("touch = (%v, %v, %v), want newer than %v, idle %v, absolute %v", seen, idle, expires, previous, wantIdle, absolute)
			}
		})
	}
}

func TestAuthenticateDenialDoesNotTouchSession(t *testing.T) {
	for _, reason := range []string{"revoked", "idle-expired", "absolute-expired"} {
		t.Run(reason, func(t *testing.T) {
			e := setupRevocationEnv(t, "touch-denial")
			token := e.signIn(t, "device")
			var previousSeen, previousIdle time.Time
			err := e.owner.QueryRow(t.Context(), `UPDATE session SET
    created_at = now() - interval '3 hours',
    last_seen_at = now() - interval '2 hours',
    idle_expires_at = CASE WHEN @reason = 'idle-expired' THEN now() - interval '1 hour' ELSE idle_expires_at END,
    expires_at = CASE WHEN @reason = 'absolute-expired' THEN now() - interval '1 hour' ELSE expires_at END,
    revoked_at = CASE WHEN @reason = 'revoked' THEN now() ELSE revoked_at END
    WHERE token_hash = @token RETURNING last_seen_at, idle_expires_at`,
				pgx.StrictNamedArgs{"reason": reason, "token": hashToken(token)}).Scan(&previousSeen, &previousIdle)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := e.svc.Authenticate(e.wsOnlyCtx(), token); !errors.Is(err, apperrors.ErrNotFound) {
				t.Fatalf("authentication = %v, want ErrNotFound", err)
			}
			var seen, idle time.Time
			err = e.owner.QueryRow(t.Context(), `SELECT last_seen_at, idle_expires_at FROM session WHERE token_hash = @token`,
				pgx.StrictNamedArgs{"token": hashToken(token)}).Scan(&seen, &idle)
			if err != nil {
				t.Fatal(err)
			}
			if !seen.Equal(previousSeen) || !idle.Equal(previousIdle) {
				t.Fatalf("denied authentication touched session: (%v, %v), want (%v, %v)", seen, idle, previousSeen, previousIdle)
			}
		})
	}
}

func TestAuthenticateLoadsGrantsBeforeLockingSession(t *testing.T) {
	e := setupRevocationEnv(t, "touch-lock-order")
	token := e.signIn(t, "device")
	held, err := e.owner.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := held.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	})
	if _, err := held.Exec(t.Context(), `LOCK TABLE role_assignment IN ACCESS EXCLUSIVE MODE`); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(e.wsOnlyCtx())
	returned := make(chan struct{})
	var authErr error
	go func() {
		defer close(returned)
		_, authErr = e.svc.Authenticate(ctx, token)
	}()
	t.Cleanup(func() { cancel(); <-returned })
	testdb.AwaitQueuedBehind(t, held, int32(e.owner.PgConn().PID()), 1, returned, "session authentication")
	if _, err := held.Exec(t.Context(), `SELECT id FROM session WHERE token_hash = @token FOR UPDATE NOWAIT`,
		pgx.StrictNamedArgs{"token": hashToken(token)}); err != nil {
		t.Fatalf("authentication locked the session before grants finished: %v", err)
	}
	if err := held.Rollback(t.Context()); err != nil {
		t.Fatal(err)
	}
	<-returned
	if authErr != nil {
		t.Fatalf("authentication after releasing grants: %v", authErr)
	}
}
