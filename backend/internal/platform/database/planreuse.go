// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package database

// Plan reuse for a statement that plans slowly and runs fast. Row-scope clauses
// make such a statement, and Postgres plans it at every execution because its
// generic plan looks costlier. Reuse is opt-in: a generic plan cannot see a
// bound value, so it cannot use a partial index whose predicate is `status = $1`.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

type (
	genericPlanScope struct{}
	customPlanScope  struct{}
)

// GenericPlansEnabled reports whether the ambient snapshot keeps generic plans.
// Callers with costly custom-plan fallbacks can keep named statements only here.
func GenericPlansEnabled(ctx context.Context) bool {
	scoped, ok := ctx.Value(genericPlanScope{}).(pgx.Tx)
	tx, joined := snapshotOf(ctx)
	custom, _ := ctx.Value(customPlanScope{}).(pgx.Tx)
	return ok && joined && tx == scoped && custom != tx
}

// ReuseGenericPlans runs fn with each statement on the ambient snapshot reusing
// its generic plan, then restores the snapshot's mode. A caller opts in only
// when its statements spell every partial-index predicate as a literal. With no
// ambient snapshot it only runs fn.
func ReuseGenericPlans(ctx context.Context, fn func(context.Context) error) error {
	tx, ok := snapshotOf(ctx)
	if !ok || tx == nil {
		return fn(ctx)
	}
	if GenericPlansEnabled(ctx) {
		return fn(ctx)
	}
	return ReuseGenericPlansTx(ctx, tx, func() error {
		scoped := context.WithValue(ctx, genericPlanScope{}, tx)
		return fn(context.WithValue(scoped, customPlanScope{}, nil))
	})
}

// UseCustomPlansTx applies the custom-plan exception to an explicit transaction.
func UseCustomPlansTx(ctx context.Context, tx pgx.Tx, fn func(context.Context) error) error {
	return withPlanCacheMode(ctx, tx, "force_custom_plan", func() error {
		return fn(context.WithValue(ctx, customPlanScope{}, tx))
	})
}

// ReuseGenericPlansTx applies the same scoped mode to an explicit transaction.
// Its statements must keep partial-index predicates literal, as for snapshot reuse.
func ReuseGenericPlansTx(ctx context.Context, tx pgx.Tx, fn func() error) error {
	return withPlanCacheMode(ctx, tx, "force_generic_plan", fn)
}

func withPlanCacheMode(ctx context.Context, tx pgx.Tx, mode string, fn func() error) error {
	// The materialized CTE reads the old mode before the outer row sets the new one.
	var before, forced string
	statement := `WITH previous AS MATERIALIZED (SELECT current_setting('plan_cache_mode') AS mode)
SELECT previous.mode, set_config('plan_cache_mode', 'force_generic_plan', true) FROM previous`
	if mode == "force_custom_plan" {
		statement = `WITH previous AS MATERIALIZED (SELECT current_setting('plan_cache_mode') AS mode)
SELECT previous.mode, set_config('plan_cache_mode', 'force_custom_plan', true) FROM previous`
	}
	if err := tx.QueryRow(ctx, statement).Scan(&before, &forced); err != nil {
		return fmt.Errorf("pg: setting plan_cache_mode %s: %w", mode, err)
	}
	err := fn()
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
