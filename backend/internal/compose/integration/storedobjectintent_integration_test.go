// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Reading the intent ledger, for the suites that assert a writer recorded a key
// before its put and cleared it with its row. Taking the handle rather than an
// env serves the bare harness and the app harness alike.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// provisionalKeys is what the intent ledger holds for db's workspace.
func provisionalKeys(t *testing.T, db *database.DB) map[string]bool {
	t.Helper()
	keys := map[string]bool{}
	if err := db.Tx(context.Background(), func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `SELECT storage_key FROM stored_object_intent`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var key string
			if err := rows.Scan(&key); err != nil {
				return err
			}
			keys[key] = true
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the intent ledger: %v", err)
	}
	return keys
}

// keyIsProvisional answers whether the ledger still holds key.
func keyIsProvisional(t *testing.T, db *database.DB, key string) bool {
	t.Helper()
	return provisionalKeys(t, db)[key]
}
