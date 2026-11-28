// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/jackc/pgx/v5"
)

// commitBudget bounds a COMMIT or ROLLBACK that runs past a caller context with
// no later deadline of its own. It is a ceiling on a stuck database.
const commitBudget = 10 * time.Second

type commitDeadlineKey struct{}

// WithCommitDeadline states the latest moment a COMMIT under ctx is still worth
// finishing: for a request, its response write deadline. A COMMIT that lands
// later saves a change whose answer can no longer be delivered.
func WithCommitDeadline(ctx context.Context, by time.Time) context.Context {
	return context.WithValue(ctx, commitDeadlineKey{}, by)
}

// CommitDeadline answers the deadline WithCommitDeadline stated on ctx.
func CommitDeadline(ctx context.Context) (time.Time, bool) {
	by, stated := ctx.Value(commitDeadlineKey{}).(time.Time)
	return by, stated
}

type commitWitnessKey struct{}

// WatchCommits reports whether any transaction under the returned context
// committed a write. A caller deciding whether a failed request may run again
// asks it, because a refusal after a commit is not "nothing happened".
func WatchCommits(ctx context.Context) (context.Context, func() bool) {
	committed := new(atomic.Bool)
	return context.WithValue(ctx, commitWitnessKey{}, committed), committed.Load
}

// commitWhileAnswerable commits only while the caller's context is live, and
// then lets the COMMIT finish. Cancelled mid-flight, a COMMIT may land with the
// caller told it failed, so a retry saves the record twice.
func commitWhileAnswerable(ctx context.Context, tx pgx.Tx) error {
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("pg: commit: %w", err)
	}
	if err := runBeforeCommit(ctx, tx); err != nil {
		return err
	}
	if err := witnessWrite(ctx, tx); err != nil {
		return err
	}
	finish, cancel := finishContext(ctx, time.Now())
	defer cancel()
	return tx.Commit(finish)
}

// rollbackDetached rolls back on the finishing context. A rollback on an ended
// context fails before it is sent, and pgx then closes the pooled connection.
func rollbackDetached(ctx context.Context, tx pgx.Tx) error {
	finish, cancel := finishContext(ctx, time.Now())
	defer cancel()
	return tx.Rollback(finish)
}

// witnessWrite marks the watching request before a COMMIT of a transaction that
// wrote. A read-only one is left out: a refusal after a read changed nothing.
// Marked before the COMMIT, so a COMMIT whose outcome is unknown counts.
func witnessWrite(ctx context.Context, tx pgx.Tx) error {
	committed, watched := ctx.Value(commitWitnessKey{}).(*atomic.Bool)
	if !watched {
		return nil
	}
	var wrote bool
	if err := tx.QueryRow(ctx, `SELECT txid_current_if_assigned() IS NOT NULL`).Scan(&wrote); err != nil {
		return fmt.Errorf("pg: commit: reading whether the transaction wrote: %w", err)
	}
	if wrote {
		committed.Store(true)
	}
	return nil
}

// finishContext is the context a COMMIT or ROLLBACK runs on. A context that can
// never end is used as it is. Any other is detached from its cancellation and
// given the deadline finishBy picks.
func finishContext(ctx context.Context, now time.Time) (context.Context, context.CancelFunc) {
	if ctx.Done() == nil {
		return ctx, func() {}
	}
	return context.WithDeadline(context.WithoutCancel(ctx), finishBy(ctx, now))
}

// finishBy is when a COMMIT or ROLLBACK must be over. A request stops at its
// commit deadline. Any other caller keeps its own deadline when that is later
// than commitBudget, so a background job loses no time it had before.
func finishBy(ctx context.Context, now time.Time) time.Time {
	if by, stated := CommitDeadline(ctx); stated {
		return by
	}
	by := now.Add(commitBudget)
	if own, ok := ctx.Deadline(); ok && own.After(by) {
		return own
	}
	return by
}
