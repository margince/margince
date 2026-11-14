// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// WithDegradableRead runs fn as a lane the composed read can lose without
// losing the page: fn gets its own statement budget, and when it fails the
// snapshot is left as it was.
//
// BOTH HALVES ARE NEEDED. A savepoint recovers the transaction from a SQL error
// but not from a cancelled context, because pgx closes the connection when the
// request's deadline fires, and a lane that only inherits that deadline dies
// with the whole page. So the budget is enforced by Postgres, inside the
// savepoint, and is shorter than the request's: the server cancels the
// statement, the connection lives, and ROLLBACK TO SAVEPOINT clears the abort.
//
// THE BUDGET IS PER STATEMENT, not for the lane: Postgres applies it to each
// statement fn runs. A lane stops at its first failed statement, so one that
// stalls costs the page one budget; a lane made of several statements each
// under it is making progress and is not what this guards against.
//
// With no snapshot on ctx every reader opens its own transaction, a failure is
// already contained to it, and fn simply runs.
func WithDegradableRead(ctx context.Context, budget time.Duration, fn func(context.Context) error) error {
	tx, joined := snapshotOf(ctx)
	if !joined {
		return fn(ctx)
	}
	return WithDegradableSavepoint(ctx, tx, budget, func(pgx.Tx) error { return fn(ctx) })
}

// WithDegradableSavepoint isolates one bounded attempt without aborting its enclosing transaction.
func WithDegradableSavepoint(ctx context.Context, tx pgx.Tx, budget time.Duration, fn func(pgx.Tx) error) error {
	if deferred, ok := deferredOf(tx); ok {
		return deferredLane(ctx, deferred, budget, fn)
	}
	page, err := openLane(ctx, tx, budget)
	if err != nil {
		return err
	}
	if err := fn(tx); err != nil {
		return errors.Join(err, abandonLane(ctx, tx))
	}
	// A released savepoint keeps its SET LOCAL until the transaction ends, so
	// the page's own ceiling is put back for the lanes that follow.
	closing := &pgx.Batch{}
	closing.Queue(boundStatementSQL, page)
	closing.Queue(`RELEASE SAVEPOINT degradable_read`)
	if err := tx.SendBatch(ctx, closing).Close(); err != nil {
		return errors.Join(fmt.Errorf("pg: closing a degradable read: %w", err), abandonLane(ctx, tx))
	}
	return nil
}

// deferredLane is WithDegradableSavepoint on a snapshot that carries the lane's
// opening and closing on the exchanges around it. A lane that sends nothing
// costs nothing.
func deferredLane(ctx context.Context, tx *deferredTx, budget time.Duration, fn func(pgx.Tx) error) error {
	milliseconds, err := budgetMilliseconds(budget)
	if err != nil {
		return err
	}
	if aborted(tx) {
		return errors.New("pg: opening a degradable read: the snapshot is already aborted")
	}
	var page string
	savepoint := fmt.Sprintf("degradable_read_%d", tx.scopes+1)
	lane := tx.open(
		deferredStatement{sql: `SELECT current_setting('statement_timeout')`, scan: func(row pgx.Row) error { return row.Scan(&page) }},
		deferredStatement{sql: `SAVEPOINT ` + savepoint, opens: true, savepoint: true},
		deferredStatement{sql: boundStatementSQL, args: []any{milliseconds}},
	)
	if err := fn(tx); err != nil {
		return errors.Join(err, abandonDeferredLane(ctx, tx, lane, savepoint))
	}
	// No closing exchange runs here to find a failure the lane swallowed.
	if aborted(tx) {
		return errors.Join(errors.New("pg: a degradable read left its transaction aborted"), abandonDeferredLane(ctx, tx, lane, savepoint))
	}
	tx.close(lane, deferredStatement{sql: boundStatementSQL, args: []any{page}}, deferredStatement{sql: `RELEASE SAVEPOINT ` + savepoint})
	return nil
}

// aborted is whether a failed statement left tx's transaction aborted; the
// driver tracks it from the server's last answer, so asking costs nothing.
func aborted(tx pgx.Tx) bool { return tx.Conn().PgConn().TxStatus() == 'E' }

// abandonDeferredLane is abandonLane for a deferred lane: one that never
// reached the server has nothing to undo, and what it left pending goes.
func abandonDeferredLane(ctx context.Context, tx *deferredTx, lane *deferredScope, savepoint string) error {
	tx.drop(lane)
	if !lane.sent {
		return nil
	}
	if _, err := tx.Tx.Exec(ctx, `ROLLBACK TO SAVEPOINT `+savepoint+`; RELEASE SAVEPOINT `+savepoint); err != nil {
		return fmt.Errorf("pg: rolling back a degradable read: %w", err)
	}
	Forget(tx, "")
	return nil
}

// openLane reads the page's statement timeout, opens the savepoint and sets
// the lane's budget in one round trip, and answers the timeout to restore.
func openLane(ctx context.Context, tx pgx.Tx, budget time.Duration) (string, error) {
	milliseconds, err := budgetMilliseconds(budget)
	if err != nil {
		return "", err
	}
	var page string
	opened := false
	opening := &pgx.Batch{}
	opening.Queue(`SELECT current_setting('statement_timeout')`).QueryRow(func(row pgx.Row) error {
		return row.Scan(&page)
	})
	// A savepoint by statement rather than tx.Begin opens no transaction and
	// takes no pool connection. transactionopeners_test.go holds runTxWith as
	// the one function that does.
	opening.Queue(`SAVEPOINT degradable_read`).Exec(func(pgconn.CommandTag) error {
		opened = true
		return nil
	})
	opening.Queue(boundStatementSQL, milliseconds)
	if err := tx.SendBatch(ctx, opening).Close(); err != nil {
		err = fmt.Errorf("pg: opening a degradable read: %w", err)
		if opened {
			return "", errors.Join(err, abandonLane(ctx, tx))
		}
		return "", err
	}
	return page, nil
}

// abandonLane undoes everything the lane did, including the abort a failed
// statement leaves on the transaction, and closes the savepoint.
//
// One Exec with no arguments: pgx sends it over the simple protocol, which an
// aborted transaction still accepts and a batch's extended protocol does not.
func abandonLane(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT degradable_read; RELEASE SAVEPOINT degradable_read`); err != nil {
		return fmt.Errorf("pg: rolling back a degradable read: %w", err)
	}
	Forget(tx, "")
	return nil
}
