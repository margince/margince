// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testdb

import (
	"context"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
)

// OwnerStatementCeiling bounds each statement a test sends on an owner
// connection. A runaway plan then fails as its test, well inside the 900s an
// integration package gets, while a healthy seed on a slow runner, under a
// minute, stays far below it.
const OwnerStatementCeiling = 3 * time.Minute

// CapStatements holds every later statement on conn to OwnerStatementCeiling,
// for the rest of its session. Call it after EnsureSchema, whose rebuild it
// must not bound.
func CapStatements(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, "SELECT set_config('statement_timeout', $1, false)",
		strconv.FormatInt(OwnerStatementCeiling.Milliseconds(), 10))
	return err
}

// StatementsCapped reports whether conn's session holds OwnerStatementCeiling,
// and the setting it holds instead when it does not.
func StatementsCapped(ctx context.Context, conn *pgx.Conn) (bool, string, error) {
	var setting string
	var capped bool
	err := conn.QueryRow(ctx, "SELECT current_setting('statement_timeout'), "+
		"current_setting('statement_timeout')::interval = make_interval(secs => $1)",
		OwnerStatementCeiling.Seconds()).Scan(&setting, &capped)
	return capped, setting, err
}
