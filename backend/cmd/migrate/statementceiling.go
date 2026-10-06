// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// How long a migration may hold what it took.
//
// Every migration file bounds ACQUISITION with `SET LOCAL lock_timeout`, which
// migrations/locktimeout_test.go requires. That decides how long to queue; nothing in
// the files decides how long the queue behind them may grow, and one migration says
// so in its own comment. These are the other half, on the connection rather than in
// each file: a session ceiling reaches every migration ever written, where a per-file
// rule would reach only the ones written after it — and a shipped migration is not
// editable, because changing one alters what a fresh installation gets while deployed
// databases keep what they ran.

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// boundStatementHold caps how long one migration statement may run on this
// connection.
//
// Set on the SESSION, so it survives into every migration's transaction without
// each file repeating it. A file that needs longer raises it with `SET LOCAL`
// beside the statement that needs it, which states the exception where the next
// reader will be standing instead of in a deploy runbook.
//
// Zero removes the ceiling, because Postgres reads statement_timeout = 0 as "no
// timeout" and an operator who has decided to accept an unbounded hold should be
// able to say so explicitly rather than by passing a number large enough to mean
// the same thing.
func boundStatementHold(ctx context.Context, conn *pgx.Conn, ceiling time.Duration) error {
	if ceiling < 0 {
		return fmt.Errorf("migrate: statement-timeout cannot be negative, got %s", ceiling)
	}
	if _, err := conn.Exec(ctx,
		`SELECT set_config('statement_timeout', $1, false)`,
		ceilingMillis(ceiling)); err != nil {
		return fmt.Errorf("migrate: bounding the statement ceiling: %w", err)
	}
	return nil
}

// defaultStatementCeiling is the migration budget an installation gets without
// saying otherwise. Generous for DDL on this product's tables and far from the
// forever a missing ceiling means; raised per run by the flag when a backfill
// legitimately needs longer.
const defaultStatementCeiling = 5 * time.Minute

// ceilingMillis renders a ceiling the way Postgres takes it, in milliseconds.
//
// A POSITIVE ceiling never renders as "0". Postgres reads statement_timeout = 0 as no
// timeout, so truncation would turn `--statement-timeout=500us` — an operator asking
// for the tightest bound they can name — into no bound at all, which is the one way
// this setting must not fail. Anything under a millisecond becomes one.
func ceilingMillis(ceiling time.Duration) string {
	milliseconds := ceiling.Milliseconds()
	if ceiling > 0 && milliseconds == 0 {
		milliseconds = 1
	}
	return strconv.FormatInt(milliseconds, 10)
}

// riverMigrationDSN carries the migration ceiling onto River's pool while lifting the
// request ceilings its DDL cannot live under.
//
// ORDER IS LOAD-BEARING, and the obvious order is wrong: WithDSNParam returns the DSN
// untouched when the parameter is already there, and WithoutRequestCeilings writes
// statement_timeout=0 itself. Wrapping the other way round leaves that zero in place
// and the pool unbounded — which reads exactly like a bounded one. Setting the
// ceiling first makes WithoutRequestCeilings' own statement_timeout the no-op instead,
// and its idle_in_transaction lift still applies. cmd/api wraps these the same way.
func riverMigrationDSN(dsn string, ceiling time.Duration) string {
	return database.WithoutRequestCeilings(
		database.WithDSNParam(dsn, "statement_timeout", ceilingMillis(ceiling)))
}
