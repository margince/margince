// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dbmigrate

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// CopyDatabase creates name as a copy of the migrated template. The template
// must have no open session, or the server refuses the copy.
//
// FILE_COPY forces a checkpoint, which costs nothing when the server does not
// sync to disk; a durable server copies through the WAL instead.
func CopyDatabase(ctx context.Context, conn *pgx.Conn, name, template string) error {
	var unsynced bool
	if err := conn.QueryRow(ctx, "SELECT current_setting('fsync') = 'off'").Scan(&unsynced); err != nil {
		return fmt.Errorf("reading fsync: %w", err)
	}
	strategy := "WAL_LOG"
	if unsynced {
		strategy = "FILE_COPY"
	}
	create := "CREATE DATABASE " + pgx.Identifier{name}.Sanitize() +
		" TEMPLATE " + pgx.Identifier{template}.Sanitize() + " STRATEGY " + strategy
	if _, err := conn.Exec(ctx, create); err != nil {
		return fmt.Errorf("copying %q into %q: %w", template, name, err)
	}
	return nil
}
