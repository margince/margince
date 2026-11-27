// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// WithFixtureTriggerDisabled recreates legacy rows without disabling unrelated constraints.
func WithFixtureTriggerDisabled(t *testing.T, table, trigger string, mutate func(pgx.Tx) error) {
	t.Helper()
	statement := `ALTER TABLE ` + pgx.Identifier{table}.Sanitize() + ` %s TRIGGER ` + pgx.Identifier{trigger}.Sanitize()
	withFixtureOwnerTx(t, func(tx pgx.Tx) error {
		// One transaction restores the trigger even when the fixture mutation fails.
		if _, err := tx.Exec(t.Context(), strings.Replace(statement, "%s", "DISABLE", 1)); err != nil {
			return err
		}
		if err := mutate(tx); err != nil {
			return err
		}
		_, err := tx.Exec(t.Context(), strings.Replace(statement, "%s", "ENABLE", 1))
		return err
	})
}

// withFixtureOwnerTx runs mutate as the owner in one transaction on an isolated
// integration database, and commits it.
func withFixtureOwnerTx(t *testing.T, mutate func(pgx.Tx) error) {
	t.Helper()
	ctx := t.Context()
	cfg, err := pgx.ParseConfig(For(t).OwnerDSN)
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
	if err := mutate(tx); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
}
