// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// WithFixtureTriggerDisabled recreates legacy rows without disabling unrelated constraints.
func WithFixtureTriggerDisabled(t *testing.T, table, trigger string, mutate func(pgx.Tx) error) {
	t.Helper()
	ctx := t.Context()
	cfg, err := pgx.ParseConfig(os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal("parsing fixture owner configuration")
	}
	if !strings.HasPrefix(cfg.Database, "margince_it_") && !strings.HasPrefix(cfg.Database, "margince_test") {
		t.Fatal("legacy fixtures require an isolated integration database")
	}
	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal("connecting fixture owner")
	}
	defer func() {
		if err := conn.Close(ctx); err != nil {
			t.Error(err)
		}
	}()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	statement := `ALTER TABLE ` + pgx.Identifier{table}.Sanitize() + ` %s TRIGGER ` + pgx.Identifier{trigger}.Sanitize()
	// One transaction restores the trigger even when the fixture mutation fails.
	if _, err := tx.Exec(ctx, strings.Replace(statement, "%s", "DISABLE", 1)); err != nil {
		t.Fatal(err)
	}
	if err := mutate(tx); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, strings.Replace(statement, "%s", "ENABLE", 1)); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
