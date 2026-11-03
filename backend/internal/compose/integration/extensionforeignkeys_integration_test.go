// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// An extension unit's foreign keys are held to the same index rule as core's.
// A unit migrates outside public, where the core census never reads. An
// unindexed key there turns an erasure or the installation reset into a scan
// of the unit's table per deleted row.
//
// This package's template carries every enabled unit's tables, which the
// migrations package's own head schema does not.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

// nonCoreSchemas are the schemas holding tables that are not core's: every one
// but public and the system's own, read from the database rather than listed.
const nonCoreSchemas = `
SELECT DISTINCT n.nspname FROM pg_namespace n JOIN pg_class t ON t.relnamespace = n.oid
 WHERE t.relkind = 'r' AND n.nspname <> 'public' AND n.nspname <> 'information_schema'
   AND n.nspname NOT LIKE 'pg\_%' ORDER BY 1`

func TestEveryExtensionForeignKeyCanFindItsChildren(t *testing.T) {
	Setup(t)
	owner := OwnerConn(t)
	ctx := context.Background()
	rows, err := owner.Query(ctx, nonCoreSchemas)
	if err != nil {
		t.Fatal(err)
	}
	schemas, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatal(err)
	}

	var keys int
	for _, schema := range schemas {
		for _, action := range []string{testdb.CascadingKeys, testdb.OtherKeys} {
			n, err := testdb.ForeignKeyCount(ctx, owner, schema, action)
			if err != nil {
				t.Fatal(err)
			}
			keys += n
			uncovered, err := testdb.UncoveredForeignKeys(ctx, owner, schema, action)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range uncovered {
				t.Errorf("%s has no index its parent's delete can use. Index the referencing "+
					"columns in the unit's migration", key)
			}
		}
	}
	if keys == 0 {
		t.Fatalf("no foreign key was read outside public (schemas %v), so this census judged nothing. "+
			"The template carries no unit's tables, or the units moved somewhere it does not read", schemas)
	}
}
