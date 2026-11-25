// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations

// An index on a table this file did not create is built CONCURRENTLY. A plain
// build holds SHARE until it has read every row, and every write waits for it.
// The concurrent build takes SHARE UPDATE EXCLUSIVE, which writers pass.
// notransactionshape_test.go holds the marker that the concurrent form needs.
// A table the file creates earlier has no rows and no writer yet, so it is exempt.

import (
	"io/fs"
	"strings"
	"testing"
)

// concurrentIndexBaseline is where this obligation starts in each namespace.
//
// Pinned at the second this gate was written. Dozens of shipped files build a
// plain index on a live table, and a shipped migration is never edited.
//
// gatekit:fixture the oldest version this gate binds in each namespace. It is data,
// not a cost: a namespace's entry names where the rule starts applying.
var concurrentIndexBaseline = map[string]string{
	"core":   "1795632000",
	"custom": "20261125190000",
}

func TestAnIndexOnAPreExistingTableIsBuiltConcurrently(t *testing.T) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		t.Fatalf("reading the embedded migration namespaces: %v", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		namespace := entry.Name()
		t.Run(namespace, func(t *testing.T) {
			baseline, set := concurrentIndexBaseline[namespace]
			if !set {
				t.Fatalf("namespace %q has no concurrentIndexBaseline: pick the version this "+
					"obligation starts at for it, in the same change that adds the namespace", namespace)
			}
			dir, subErr := fs.Sub(files, namespace)
			if subErr != nil {
				t.Fatalf("reading the %s namespace: %v", namespace, subErr)
			}
			checkConcurrentIndexBuilds(t, namespace, baseline, dir)
		})
	}
}

func checkConcurrentIndexBuilds(t *testing.T, namespace, baseline string, dir fs.FS) {
	t.Helper()
	var examined, skipped int
	err := fs.WalkDir(dir, ".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".sql") {
			return err
		}
		if version(path) < baseline {
			skipped++
			return nil
		}
		examined++
		body, readErr := fs.ReadFile(dir, path)
		if readErr != nil {
			return readErr
		}
		for _, table := range plainIndexBuilds(string(body)) {
			t.Errorf("%s/%s builds an index on %s, which it did not create, without CONCURRENTLY.\n"+
				"A plain build blocks every write to that table until it has read all its rows. "+
				"Move the build to a file of its own that says -- pgmigrate:no-transaction, and "+
				"write DROP INDEX CONCURRENTLY IF EXISTS then CREATE INDEX CONCURRENTLY "+
				"(docs/how-to/apply-migrations.md, \"Build an index while the app still writes\").",
				namespace, path, table)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the migration files: %v", err)
	}
	if examined == 0 && skipped == 0 {
		t.Errorf("%s: no migration at all, so this gate read nothing and passed", namespace)
	}
	t.Logf("%s: examined %d file(s), %d below the %s pin", namespace, examined, skipped, baseline)
}

// plainIndexBuilds lists the table of every non-concurrent index build aimed at a
// table not created earlier in the same file.
func plainIndexBuilds(sql string) []string {
	statements := executableSQL(sql)
	own := tablesCreatedIn(statements)
	var found []string
	for _, m := range indexBuild.FindAllStringSubmatchIndex(statements, -1) {
		if buildsConcurrently.MatchString(statements[m[0]:m[1]]) {
			continue
		}
		table := strings.ToLower(strings.Trim(statements[m[2]:m[3]], `"`))
		if at, isOwn := own[table]; isOwn && at < m[0] {
			continue
		}
		found = append(found, table)
	}
	return found
}

// Every namespace sorts below its pin today, so the walk above reads nothing and
// these cases are the evidence that the reading is right.
func TestTheConcurrentIndexGateReportsWhatItClaimsTo(t *testing.T) {
	for _, c := range []struct {
		name  string
		sql   string
		finds int
	}{
		{"plain index on a table it did not create", `SET LOCAL lock_timeout = '3s'; CREATE INDEX i ON activity (owner_id);`, 1},
		{"plain unique index", `CREATE UNIQUE INDEX IF NOT EXISTS i ON activity (slug);`, 1},
		{"an index with no name", `CREATE INDEX ON activity (owner_id);`, 1},
		{"ON ONLY names the same table", `CREATE INDEX i ON ONLY activity (owner_id);`, 1},
		{"a quoted table", `CREATE INDEX "i" ON "Activity" (owner_id);`, 1},
		{"concurrent build", "-- pgmigrate:no-transaction\nDROP INDEX CONCURRENTLY IF EXISTS i;\nCREATE INDEX CONCURRENTLY i ON activity (owner_id);", 0},
		{"concurrent unique build", `CREATE UNIQUE INDEX CONCURRENTLY i ON activity (slug);`, 0},
		{"index on a table created earlier in the file", `CREATE TABLE fresh (x int); CREATE INDEX i ON fresh (x);`, 0},
		// IF NOT EXISTS creates nothing when the table is already there.
		{"index after CREATE TABLE IF NOT EXISTS", `CREATE TABLE IF NOT EXISTS activity (x int); CREATE INDEX i ON activity (x);`, 1},
		{"index before the CREATE that names its table", `CREATE INDEX i ON fresh (x); CREATE TABLE fresh (x int);`, 1},
		{"a build that is only in a comment", "-- CREATE INDEX i ON activity (owner_id);\nSELECT 1;", 0},
		{"a build that is only a string", `SELECT 'CREATE INDEX i ON activity (owner_id)';`, 0},
		{"two plain builds", `CREATE INDEX a ON activity (x); CREATE INDEX b ON deal (y);`, 2},
		{"a block comment between the words", `CREATE /* why */ UNIQUE INDEX i ON activity (slug);`, 1},
		{"a nested block comment hides a build", "/* outer /* inner */ CREATE INDEX i ON activity (x); */ SELECT 1;", 0},
		{"a block comment before CONCURRENTLY", `CREATE INDEX /* see docs */ CONCURRENTLY i ON activity (x);`, 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			if found := plainIndexBuilds(c.sql); len(found) != c.finds {
				t.Errorf("read %d finding(s) %v, want %d: %s", len(found), found, c.finds, c.sql)
			}
		})
	}
}
