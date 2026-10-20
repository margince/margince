// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// Savepoint runs fn inside a nested transaction and keeps its writes only when
// fn says to. An error from fn withdraws them and is returned with any failure
// to withdraw. A failed statement aborts the whole transaction unless it is
// rolled back to a savepoint. A caller that tolerates a failed read, or takes a
// write back when it changed nothing, has to go through here.
func Savepoint(ctx context.Context, tx pgx.Tx, fn func(nested pgx.Tx) (keep bool, err error)) (bool, error) {
	nested, err := tx.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("storekit: opening a savepoint: %w", err)
	}
	keep, err := fn(nested)
	if err != nil {
		if rbErr := nested.Rollback(ctx); rbErr != nil {
			return false, errors.Join(err, rbErr)
		}
		return false, err
	}
	if !keep {
		if err := nested.Rollback(ctx); err != nil {
			return false, fmt.Errorf("storekit: withdrawing a savepoint: %w", err)
		}
		return false, nil
	}
	if err := nested.Commit(ctx); err != nil {
		return false, fmt.Errorf("storekit: releasing a savepoint: %w", err)
	}
	return true, nil
}
