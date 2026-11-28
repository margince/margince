// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailhooks

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/unitsql"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/pkg/extension"
)

// lookupFor is the read-only Lookup a hook gets. Each read opens a transaction
// of its own, so a hook abandoned at its time limit never touches the
// transaction capture runs in. ctx is the hook's own, so the read ends with it.
func lookupFor(ctx context.Context, pool *pgxpool.Pool) func(string, ...any) extension.Row {
	return func(sql string, args ...any) extension.Row {
		return lookupRow{ctx: ctx, pool: pool, sql: sql, args: args}
	}
}

// lookupRow runs its statement when it is scanned, as the unit role, in a
// transaction the database holds read-only.
type lookupRow struct {
	ctx  context.Context //nolint:containedctx // a deferred read keeps the context it was made under
	pool *pgxpool.Pool
	sql  string
	args []any
}

// Scan reads the row. The statement after the two below cannot undo read-only,
// because Postgres refuses read-write once a query has run in the transaction.
func (r lookupRow) Scan(dest ...any) error {
	ctx, cancel := context.WithTimeout(r.ctx, mailHookLimit)
	defer cancel()
	ctx = database.Detached(ctx)
	return database.WithTx(ctx, r.pool, func(tx pgx.Tx) error {
		for _, setup := range []string{"SET TRANSACTION READ ONLY", "SELECT 1"} {
			if _, err := tx.Exec(ctx, setup); err != nil {
				return fmt.Errorf("mailhooks: opening a read-only lookup: %w", err)
			}
		}
		return unitsql.Run(tx, func(sql unitsql.Tx) error {
			return sql.QueryRow(ctx, r.sql, r.args...).Scan(dest...)
		})
	})
}
