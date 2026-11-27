// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package testdb

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// SeedBeforeSourceCheck inserts a row as one written before table's source
// check existed, which refuses it today. The check is re-added unvalidated in
// the same transaction, so a concurrent write still meets it. The table's
// source_channel trigger is held off too, since it maps the word on insert.
func SeedBeforeSourceCheck(t *testing.T, owner *pgx.Conn, table, sql string, args ...any) ids.UUID {
	t.Helper()
	ctx := context.Background()
	id := ids.NewV7()
	check := pgx.Identifier{table + "_source_check"}.Sanitize()
	trigger := pgx.Identifier{table + "_source_channel"}.Sanitize()
	target := pgx.Identifier{table}.Sanitize()
	tx, err := owner.Begin(ctx)
	if err != nil {
		t.Fatalf("seeding an older %s: %v", table, err)
	}
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{sql: `ALTER TABLE ` + target + ` DROP CONSTRAINT ` + check},
		{sql: `ALTER TABLE ` + target + ` DISABLE TRIGGER ` + trigger},
		{sql: sql, args: append([]any{id}, args...)},
		{sql: `ALTER TABLE ` + target + ` ENABLE TRIGGER ` + trigger},
		{sql: `ALTER TABLE ` + target + ` ADD CONSTRAINT ` + check + ` CHECK (` + provenance.ListedRecordSourceSQL("source") + `) NOT VALID`},
	} {
		if _, err := tx.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("seeding an older %s: %v", table, errors.Join(err, tx.Rollback(ctx)))
		}
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatalf("seeding an older %s: %v", table, err)
	}
	return id
}
