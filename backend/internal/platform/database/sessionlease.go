// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrSessionBudgetBusy is capacity pressure, so callers postpone without failing work.
var ErrSessionBudgetBusy = errors.New("pg: the session lease budget is busy")

// SessionLeaseCleanupTimeout bounds cleanup after the execution was canceled.
const SessionLeaseCleanupTimeout = 5 * time.Second

type (
	sessionBudgetKey struct{}
	sessionBudget    func(context.Context) (func(), error)
)

// WithSessionBudget shares the worker's pinned-connection allowance with module executors.
func WithSessionBudget(ctx context.Context, acquire func(context.Context) (func(), error)) context.Context {
	return context.WithValue(ctx, sessionBudgetKey{}, sessionBudget(acquire))
}

type sessionKey struct {
	name   string
	shared bool
}

// SessionLease owns its connection until all advisory locks are released or the connection closes.
type SessionLease struct {
	mu      sync.Mutex
	conn    *pgxpool.Conn
	keys    []sessionKey
	release func()
	log     *slog.Logger
}

// TrySessionLease never waits for another execution's advisory lock.
func TrySessionLease(ctx context.Context, pool *pgxpool.Pool, key string, log *slog.Logger) (*SessionLease, bool, error) {
	release := func() {}
	if acquire, bounded := ctx.Value(sessionBudgetKey{}).(sessionBudget); bounded {
		var err error
		release, err = acquire(ctx)
		if err != nil {
			return nil, false, err
		}
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		release()
		return nil, false, err
	}
	lease := &SessionLease{conn: conn, release: release, log: log}
	held, err := lease.TryLock(ctx, key, false)
	if err != nil || !held {
		lease.Close(ctx)
		return nil, false, err
	}
	return lease, true, nil
}

// PID identifies the pinned backend; callers also use a unique lock key to prevent backend reuse.
func (l *SessionLease) PID() uint32 {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return 0
	}
	return l.conn.Conn().PgConn().PID()
}

type sessionOperation uint8

const (
	trySessionLock sessionOperation = iota
	unlockSessionLock
)

func lockStatement(shared bool, operation sessionOperation) string {
	name := "pg_try_advisory_lock"
	if operation == unlockSessionLock {
		name = "pg_advisory_unlock"
	}
	if shared {
		name += "_shared"
	}
	return name
}

// TryLock adds another lock on this same connection, without consuming another pool slot.
func (l *SessionLease) TryLock(ctx context.Context, key string, shared bool) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return false, errors.New("pg: session lease is closed")
	}
	args := []any{key}
	var held bool
	query := fmt.Sprintf("SELECT %s(hashtextextended($%d, 0))", lockStatement(shared, trySessionLock), len(args))
	if err := l.conn.QueryRow(ctx, query, args...).Scan(&held); err != nil {
		l.discard(ctx)
		return false, err
	}
	if held {
		l.keys = append(l.keys, sessionKey{key, shared})
	}
	return held, nil
}

// Unlock drops one acquisition; a failed unlock discards the potentially locked connection.
func (l *SessionLease) Unlock(ctx context.Context, keyName string, shared bool) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return errors.New("pg: session lease is closed")
	}
	for i, key := range slices.Backward(l.keys) {
		if key != (sessionKey{keyName, shared}) {
			continue
		}
		if err := l.releaseKey(ctx, key); err != nil {
			l.discard(ctx)
			return err
		}
		l.keys = append(l.keys[:i], l.keys[i+1:]...)
		return nil
	}
	return errors.New("pg: session lease does not hold the requested lock")
}

func (l *SessionLease) releaseKey(ctx context.Context, key sessionKey) error {
	args := []any{key.name}
	query := fmt.Sprintf("SELECT %s(hashtextextended($%d, 0))", lockStatement(key.shared, unlockSessionLock), len(args))
	var released bool
	if err := l.conn.QueryRow(ctx, query, args...).Scan(&released); err != nil {
		return err
	}
	if !released {
		return errors.New("pg: session lock ownership was lost")
	}
	return nil
}

func (l *SessionLease) discard(ctx context.Context) {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), SessionLeaseCleanupTimeout)
	defer cancel()
	l.discardBounded(ctx)
}

func (l *SessionLease) discardBounded(ctx context.Context) {
	conn := l.conn.Hijack()
	l.conn = nil
	if err := conn.Close(ctx); err != nil {
		l.log.WarnContext(ctx, "closing a session lease connection", "err", err)
	}
	l.release()
}

// Ping keeps a pinned owner active; an uncertain connection is discarded so its locks cannot linger.
func (l *SessionLease) Ping(ctx context.Context) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return errors.New("pg: session lease is closed")
	}
	if err := l.conn.Ping(ctx); err != nil {
		l.discard(ctx)
		return err
	}
	return nil
}

// Close is idempotent and works after the execution context was cancelled.
func (l *SessionLease) Close(ctx context.Context) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), SessionLeaseCleanupTimeout)
	defer cancel()
	for _, key := range slices.Backward(l.keys) {
		if err := l.releaseKey(ctx, key); err != nil {
			l.log.WarnContext(ctx, "releasing a session lease; closing its connection", "err", err)
			l.discardBounded(ctx)
			return
		}
	}
	l.conn.Release()
	l.conn = nil
	l.release()
}
