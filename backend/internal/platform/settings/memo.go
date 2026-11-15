// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// ValueMemoKey is the key a transaction remembers an applied setting under.
// Every setting writer invalidates it before its change hooks read again.
func ValueMemoKey(key string) string {
	return "setting:" + key
}

// storedRow is a setting's stored value, or no row, as one transaction read it.
type storedRow struct {
	raw   json.RawMessage
	found bool
}

// storedValue reads key's row once per transaction, under ValueMemoKey, so a
// setting's writer forgets it. A locking read always goes to the database.
func storedValue(ctx context.Context, tx pgx.Tx, key, lock string) (json.RawMessage, bool, error) {
	read := func() (storedRow, error) {
		var row storedRow
		err := tx.QueryRow(ctx, `SELECT value FROM setting WHERE key = $1`+lock, key).Scan(&row.raw)
		if errors.Is(err, pgx.ErrNoRows) {
			return storedRow{}, nil
		}
		row.found = err == nil
		return row, err
	}
	if lock != "" {
		row, err := read()
		return row.raw, row.found, err
	}
	row, err := database.Remember(tx, ValueMemoKey(key)+"#row", read)
	return row.raw, row.found, err
}
