// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

import (
	"context"
	"errors"
	"fmt"
)

// WithSavepoint runs fn so that a failure inside it leaves the shared
// transaction as it was before fn.
//
// A statement that errors aborts the transaction it joined. A caller that
// swallows that error still fails at commit, so a skipped row needs its failed
// write rolled back to here.
//
// With no shared transaction on ctx, fn opens its own and fn simply runs.
func WithSavepoint(ctx context.Context, fn func(context.Context) error) error {
	tx, joined := snapshotOf(ctx)
	if !joined {
		return fn(ctx)
	}
	if _, err := tx.Exec(ctx, `SAVEPOINT with_savepoint`); err != nil {
		return fmt.Errorf("pg: opening a savepoint: %w", err)
	}
	if err := fn(ctx); err != nil {
		if _, undo := tx.Exec(ctx, `ROLLBACK TO SAVEPOINT with_savepoint; RELEASE SAVEPOINT with_savepoint`); undo != nil {
			return errors.Join(err, fmt.Errorf("pg: rolling back to the savepoint: %w", undo))
		}
		return err
	}
	if _, err := tx.Exec(ctx, `RELEASE SAVEPOINT with_savepoint`); err != nil {
		return fmt.Errorf("pg: releasing the savepoint: %w", err)
	}
	return nil
}
