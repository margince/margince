// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package settings

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

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
		// What the lock saw replaces what an unlocked read remembered.
		database.Forget(tx, ValueMemoKey(key))
		row, err := read()
		return row.raw, row.found, err
	}
	row, err := database.Remember(tx, ValueMemoKey(key)+"#row", read)
	return row.raw, row.found, err
}

// GetStoredTx is GetTx that also reports whether a row stands behind the
// value, for a reader whose answer differs when nobody chose one.
func GetStoredTx[T any](ctx context.Context, tx pgx.Tx, e *Entry[T]) (T, bool, error) {
	var zero T
	if err := RequireRead(ctx, e); err != nil {
		return zero, false, err
	}
	// The presence and the value come from one read, so they always agree.
	raw, stored, err := storedValue(ctx, tx, e.Key(), "")
	if err != nil {
		return zero, false, fmt.Errorf("settings: reading %s: %w", e.Key(), err)
	}
	if !stored {
		if raw, err = e.DefaultJSON(); err != nil {
			return zero, false, err
		}
	}
	var out T
	if err := json.Unmarshal(raw, &out); err != nil {
		return zero, false, fmt.Errorf("settings: decoding %s: %w", e.Key(), err)
	}
	return out, stored, nil
}
