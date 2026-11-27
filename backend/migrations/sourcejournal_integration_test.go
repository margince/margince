// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
)

func TestSourceJournalKeepsNamespacesSavepointsAndConnectionReuseSeparate(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()
	const row = "00000000-0000-4000-8000-000000000123"
	for _, namespace := range []string{"journal_first", "journal_second"} {
		quoted := pgx.Identifier{namespace}.Sanitize()
		if _, err := conn.Exec(ctx, `CREATE SCHEMA `+quoted+`;
   CREATE TABLE `+quoted+`.company(id uuid PRIMARY KEY,source text,source_system text);
   CREATE TRIGGER remember AFTER UPDATE OF source ON `+quoted+`.company
    FOR EACH ROW EXECUTE FUNCTION public.remember_a_triggered_source_change()`); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := conn.Exec(ctx, `DROP SCHEMA `+quoted+` CASCADE`); err != nil {
				t.Error(err)
			}
		})
		if _, err := conn.Exec(ctx, `INSERT INTO `+quoted+`.company VALUES($1,$2,NULL)`, row, namespace); err != nil {
			t.Fatal(err)
		}
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Error(err)
		}
	}()
	execute := func(sql string, args ...any) {
		t.Helper()
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	execute(`UPDATE journal_first.company SET source='first edit'`)
	execute(`UPDATE journal_second.company SET source='second edit'`)
	execute(`SAVEPOINT discarded_edit`)
	execute(`UPDATE journal_first.company SET source='discarded edit'`)
	execute(`ROLLBACK TO SAVEPOINT discarded_edit`)
	execute(`UPDATE journal_first.company SET source='latest first edit'`)
	read := func(namespace string) []map[string]any {
		t.Helper()
		execute(`SET LOCAL search_path=` + pgx.Identifier{namespace}.Sanitize() + `,public`)
		rows, err := tx.Query(ctx, `SELECT public.source_changes_for_audit(current_schema(),'company',ARRAY[$1]::uuid[])`, row)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		var out []map[string]any
		for rows.Next() {
			var raw []byte
			if err := rows.Scan(&raw); err != nil {
				t.Fatal(err)
			}
			var item map[string]any
			if err := json.Unmarshal(raw, &item); err != nil {
				t.Fatal(err)
			}
			out = append(out, item)
		}
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		return out
	}
	first, second := read("journal_first"), read("journal_second")
	if len(first) != 1 || len(second) != 1 {
		t.Fatalf("journals crossed namespaces: first=%v second=%v", first, second)
	}
	if first[0]["before"] != "journal_first" || first[0]["after"] != "latest first edit" {
		t.Fatalf("savepoint or earliest before-image was lost: %v", first)
	}
	if second[0]["before"] != "journal_second" || second[0]["after"] != "second edit" {
		t.Fatalf("same-id row used another namespace's image: %v", second)
	}
	if first[0]["change_id"] == second[0]["change_id"] {
		t.Fatal("two namespaces share a source change identity")
	}
	execute(`SET LOCAL search_path=journal_first,public`)
	execute(`SELECT public.consume_source_changes(current_schema(),ARRAY[$1::uuid])`, first[0]["change_id"])
	if len(read("journal_first")) != 0 || len(read("journal_second")) != 1 {
		t.Fatal("consuming one namespace changed another namespace's journal")
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(read("journal_second")) != 0 {
		t.Fatal("a reused connection retained a committed source journal")
	}
	execute(`UPDATE journal_first.company SET source='rolled back edit'`)
	if len(read("journal_first")) != 1 {
		t.Fatal("a reused connection lost its next source change")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	tx, err = conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(read("journal_first")) != 0 {
		t.Fatal("a rolled-back edit survived in the source journal")
	}
	execute(`UPDATE journal_first.company SET source='wrong binding'`)
	execute(`SET LOCAL search_path=public`)
	if _, err := tx.Exec(ctx, `SELECT public.source_changes_for_audit(current_schema(),NULL,NULL)`); err == nil {
		t.Fatal("a mismatched namespace silently discarded a source change")
	}
}
