// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package dbtest holds database stand-ins for unit tests that run without a
// live Postgres.
package dbtest

import (
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

// ClosedPool returns a pool that never dialled and is already closed. Opening a
// transaction on it fails with an error, where a nil pool would panic.
func ClosedPool(t testing.TB) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(t.Context(), "postgres://unused@127.0.0.1:1/unused")
	if err != nil {
		t.Fatalf("building a pool that has not dialled: %v", err)
	}
	pool.Close()
	return pool
}
