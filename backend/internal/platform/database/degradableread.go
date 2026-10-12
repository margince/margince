// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
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
	var page string
	if err := tx.QueryRow(ctx, `SELECT current_setting('statement_timeout')`).Scan(&page); err != nil {
		return fmt.Errorf("pg: reading the page's statement timeout: %w", err)
	}
	// A savepoint by statement rather than tx.Begin: this opens no transaction
	// and takes no pool connection, and transactionopeners_test.go holds
	// runTxWith as the one function that does.
	if _, err := tx.Exec(ctx, `SAVEPOINT degradable_read`); err != nil {
		return fmt.Errorf("pg: opening a savepoint for a degradable read: %w", err)
	}
	if err := BoundStatement(ctx, tx, budget); err != nil {
		return errors.Join(err, abandonLane(ctx, tx))
	}
	if err := fn(ctx); err != nil {
		return errors.Join(err, abandonLane(ctx, tx))
	}
	// A released savepoint keeps its SET LOCAL until the transaction ends, so
	// the page's own ceiling is put back for the lanes that follow.
	if _, err := tx.Exec(ctx, `SELECT set_config('statement_timeout', $1, true)`, page); err != nil {
		return errors.Join(fmt.Errorf("pg: restoring the page's statement timeout: %w", err), abandonLane(ctx, tx))
	}
	if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT degradable_read`); err != nil {
		return fmt.Errorf("pg: releasing a degradable read's savepoint: %w", err)
	}
	return nil
}

// abandonLane undoes everything the lane did, including the abort a failed
// statement leaves on the transaction, and closes the savepoint.
func abandonLane(ctx context.Context, tx pgx.Tx) error {
	if _, err := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT degradable_read`); err != nil {
		return fmt.Errorf("pg: rolling back a degradable read: %w", err)
	}
	if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT degradable_read`); err != nil {
		return fmt.Errorf("pg: closing a degradable read's savepoint: %w", err)
	}
	return nil
}
