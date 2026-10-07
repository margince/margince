// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// withCustomPlans runs fn with plan_cache_mode forced to custom plans, then
// puts back the value it found. The transaction may be an ambient snapshot
// that later lanes share, so a tx-local setting must not outlive this read.
func withCustomPlans(ctx context.Context, tx pgx.Tx, fn func() error) error {
	var previous string
	if err := tx.QueryRow(ctx, `SELECT current_setting('plan_cache_mode')`).Scan(&previous); err != nil {
		return fmt.Errorf("search: reading the plan cache mode: %w", err)
	}
	// A generic plan holds the query text as a parameter, so it rebuilds every
	// tsquery for each row it matches and scores; a custom plan folds each once.
	if _, err := tx.Exec(ctx, `SELECT set_config('plan_cache_mode', 'force_custom_plan', true)`); err != nil {
		return fmt.Errorf("search: planning the ranking with its literals: %w", err)
	}
	if err := fn(); err != nil {
		// Not restored: a failed statement aborts the transaction, so the
		// restore would fail too, and no later statement in it can run.
		return err
	}
	if _, err := tx.Exec(ctx, `SELECT set_config('plan_cache_mode', $1, true)`, previous); err != nil {
		return fmt.Errorf("search: restoring the plan cache mode: %w", err)
	}
	return nil
}
