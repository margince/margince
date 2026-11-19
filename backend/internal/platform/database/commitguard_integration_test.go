// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package database_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A transaction whose caller's context ended before COMMIT rolls back. Its
// handler is answering a timeout, so a commit there is a save nobody is told of.
func TestATransactionWhoseContextEndedBeforeCommitRollsBack(t *testing.T) {
	ctx, pool := laneEnv(t)
	table := pgx.Identifier{"commit_probe_" + ids.NewV7().String()[:8]}.Sanitize()
	t.Cleanup(func() {
		if _, err := pool.Exec(context.Background(), `DROP TABLE IF EXISTS `+table); err != nil {
			t.Errorf("dropping the probe table: %v", err)
		}
	})

	work, end := context.WithCancel(ctx)
	err := database.WithWorkspaceTx(work, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(work, `CREATE TABLE `+table+` (k int)`); err != nil {
			return err
		}
		end()
		return nil
	})

	if !errors.Is(err, context.Canceled) {
		t.Fatalf("the transaction answered %v, want the ended context", err)
	}
	var kept bool
	if err := pool.QueryRow(context.Background(), `SELECT to_regclass($1) IS NOT NULL`, table).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if kept {
		t.Fatal("the transaction committed after its context ended")
	}
}
