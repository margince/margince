// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

// Plan reuse for a statement that plans slowly and runs fast.
//
// Row-scope clauses make some list statements slow to plan and fast to run: on
// the daily bench the worklist's task lists planned for about 50 ms and ran in
// under one. Postgres keeps planning such a statement at every execution,
// because its generic plan looks costlier than a custom one.
//
// Reuse is opt-in per call. A generic plan cannot see a bound value, so it
// cannot prove a partial index's predicate from `status = $1`. A statement may
// opt in only when every partial-index predicate it relies on is spelled as a
// literal, and its plan does not depend on a skewed parameter.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ReuseGenericPlans runs fn with every statement it issues on the ambient
// snapshot reusing that statement's generic plan, and then restores the mode
// the snapshot had, so a nested call leaves the outer one in force. With no ambient snapshot it only runs fn: a statement in a
// transaction of its own is planned once anyway.
func ReuseGenericPlans(ctx context.Context, fn func(context.Context) error) error {
	tx, ok := snapshotOf(ctx)
	if !ok || tx == nil {
		return fn(ctx)
	}
	var before string
	if err := tx.QueryRow(ctx, `SELECT current_setting('plan_cache_mode')`).Scan(&before); err != nil {
		return fmt.Errorf("pg: reading plan_cache_mode: %w", err)
	}
	if err := setPlanCacheMode(ctx, tx, "force_generic_plan"); err != nil {
		return err
	}
	err := fn(ctx)
	// A failed statement leaves the transaction aborted, and its rollback
	// restores the mode. Any other failure leaves it usable, so the mode is put
	// back for the statements after this one.
	if tx.Conn().PgConn().TxStatus() == 'E' {
		return err
	}
	if restoreErr := setPlanCacheMode(ctx, tx, before); restoreErr != nil {
		return errors.Join(err, restoreErr)
	}
	return err
}

// setPlanCacheMode is local to the transaction, or to the savepoint tx is in;
// rolling either back restores the mode before it.
func setPlanCacheMode(ctx context.Context, tx pgx.Tx, mode string) error {
	if _, err := tx.Exec(ctx, "SELECT set_config('plan_cache_mode', $1, true)", mode); err != nil {
		return fmt.Errorf("pg: setting plan_cache_mode %s: %w", mode, err)
	}
	return nil
}
