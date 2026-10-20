// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A lexer that drops a statement can make a rewrite appear to be harmless DDL.

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestRewriteCensusReadsStatementsAndPreservesQuotedNames(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, sql string
		want      []string
	}{
		{"update", `UPDATE contact SET name='DELETE FROM audit';`, []string{"contact"}},
		{"quoted", `UPDATE "public"."Contact" SET name='x';`, []string{"public.Contact"}},
		{"delete", `DELETE FROM ONLY public.contact WHERE id IS NOT NULL;`, []string{"public.contact"}},
		{"copy", `CREATE TABLE archive(id uuid); INSERT INTO archive SELECT id FROM contact;`, []string{"archive"}},
		{"cte", `WITH changed AS (UPDATE contact SET name='x' RETURNING id) SELECT * FROM changed;`, []string{"*", "contact"}},
		{"delete cte", `WITH removed AS (DELETE FROM contact RETURNING id) INSERT INTO archive SELECT id FROM removed;`, []string{"*", "archive", "contact"}},
		{"merge", `MERGE INTO public.contact c USING incoming i ON c.id=i.id WHEN MATCHED THEN UPDATE SET name=i.name;`, []string{"public.contact"}},
		{"conflict", `INSERT INTO role(key) VALUES('ops') ON CONFLICT(key) DO UPDATE SET key=excluded.key;`, []string{"role"}},
		{"cte call", `WITH s AS (SELECT 1) SELECT setval('seq', 1);`, []string{"*"}},
		{"parenthesized call", `(SELECT setval('seq', 1));`, []string{"*"}},
		{"values call", `VALUES (setval('seq', 1));`, []string{"*"}},
		{"create from query", `CREATE TEMP TABLE archived AS SELECT * FROM contact;`, []string{"*"}},
		{"create from table", `CREATE TABLE archived AS TABLE contact;`, []string{"*"}},
		{"materialized query", `CREATE MATERIALIZED VIEW archived AS SELECT * FROM contact;`, []string{"*"}},
		{"ordinary view", `CREATE VIEW visible AS SELECT * FROM contact;`, nil},
		{"bulk copy", `COPY contact FROM STDIN;`, []string{"*"}},
		{"refresh", `REFRESH MATERIALIZED VIEW contact_summary;`, []string{"*"}},
		{"trigger-named column", `CREATE TABLE role_copy(trigger text); INSERT INTO role_copy VALUES ('x');`, nil},
		{"trigger execution", `CREATE TRIGGER change AFTER INSERT ON role EXECUTE FUNCTION f(); INSERT INTO role VALUES ('ops');`, []string{"*"}},
		{"event trigger execution", `CREATE EVENT TRIGGER change ON ddl_command_end EXECUTE FUNCTION f(); CREATE TABLE role_copy(id uuid);`, []string{"*"}},
		{"keyword escape confusion", `UPDATE role SET name='x' WHERE name LIKE'a\'; DELETE FROM contact; SELECT 'x';`, []string{"*", "contact", "role"}},
		{"anonymous", `DO $body$ BEGIN UPDATE contact SET name='x'; END $body$;`, []string{"*"}},
		{"anonymous dynamic", `DO $$ BEGIN EXECUTE 'DELETE FROM contact'; END $$;`, []string{"*"}},
		{"procedure call", `CREATE PROCEDURE f() LANGUAGE sql AS $$ UPDATE contact SET name='x' $$; CALL f();`, []string{"*"}},
		{"function call", `SELECT setval('counter', 100);`, []string{"*"}},
		{"truncate", `TRUNCATE TABLE contact, audit;`, []string{"*"}},
		{"grants", `GRANT SELECT, INSERT, UPDATE ON contact TO app; REVOKE UPDATE ON contact FROM app;`, nil},
		{"instead trigger", `CREATE TRIGGER changed INSTEAD OF UPDATE ON contact FOR EACH ROW EXECUTE FUNCTION f();`, nil},
		{"scalar insert", `INSERT INTO role(key) VALUES ((SELECT key FROM template LIMIT 1));`, []string{"role"}},
		{"function body", `CREATE FUNCTION f() RETURNS void LANGUAGE sql AS $$ UPDATE contact SET name='x' $$;`, nil},
		{"comments", `/* UPDATE contact; /* DELETE FROM audit; */ */ SELECT 'MERGE INTO role'; -- INSERT INTO archive SELECT * FROM contact`, []string{"*"}},
		{"escape string", `SELECT E'it\'s UPDATE contact'; UPDATE role SET name='ops';`, []string{"*", "role"}},
		{"quoted keyword", `CREATE TABLE "update" (id uuid);`, nil},
		{"bounded values", `INSERT INTO role(key) VALUES('ops');`, nil},
		{"read locks", `SELECT * FROM contact FOR UPDATE; SELECT * FROM contact FOR NO KEY UPDATE;`, []string{"*"}},
		{"foreign key", `CREATE TABLE child(id uuid REFERENCES parent ON UPDATE CASCADE ON DELETE CASCADE);`, nil},
		{"trigger event", `CREATE TRIGGER changed AFTER INSERT OR UPDATE OR DELETE ON contact FOR EACH ROW EXECUTE FUNCTION f();`, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := migrationRewrites(tc.sql)
			if err != nil || !slices.Equal(got, tc.want) {
				t.Fatalf("rewrites=%v err=%v want=%v", got, err, tc.want)
			}
		})
	}
}

func TestRewriteCensusRefusesIncompleteSQLAndWaivers(t *testing.T) {
	t.Parallel()
	for _, sql := range []string{`UPDATE "contact`, `SELECT 'unfinished`, `DO $x$ unfinished`, `/* unfinished`} {
		if _, err := migrationRewrites(sql); err == nil {
			t.Fatalf("accepted incomplete SQL: %s", sql)
		}
	}
	for _, body := range []string{"file.sql | role,role | exactly six built in roles", "missing fields", "file.sql | role | small", "../file.sql | role | six built in roles only", "file.sql | role | six built in roles only\nfile.sql | role | six built in roles only"} {
		if _, err := rewriteWaivers(body); err == nil {
			t.Fatalf("accepted incomplete waiver: %s", body)
		}
	}
	waivers, err := rewriteWaivers("# comment\n1799999999_roles.up.sql | role | exactly six built in role documents")
	if err != nil || len(waivers) != 1 || !slices.Equal(waivers["1799999999_roles.up.sql"].tables, []string{"role"}) {
		t.Fatalf("valid waiver=%v err=%v", waivers, err)
	}
}

func TestRewriteWaiversExpireWhenTheFileOrWrittenTableChanges(t *testing.T) {
	t.Parallel()
	const name = "1799999999_roles.up.sql"
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte("UPDATE role SET name='ops';"), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, manifest string
		want           int
	}{
		{"unwaived", "", 1},
		{"bounded", name + " | role | exactly six built in role documents", 0},
		{"wrong table", name + " | contact | exactly six built in role documents", 1},
		{"missing file", "1799999998_gone.up.sql | role | exactly six built in role documents", 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			waivers, err := rewriteWaivers(tc.manifest)
			if err != nil {
				t.Fatal(err)
			}
			findings, err := migrationRewriteErrors([]string{path}, waivers)
			if err != nil || len(findings) != tc.want {
				t.Fatalf("findings=%v error=%v", findings, err)
			}
		})
	}
}
