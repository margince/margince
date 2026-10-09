// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package migrations

// A migration that asks not to be wrapped is held to the shape that survives it.
//
// dbmigrate.NoTransactionMarker buys a concurrent index build and gives up three
// things: atomicity, a bookkeeping row written in the work's transaction, and
// Postgres's own cleanup of a failed build. All three are answered by the file being
// re-runnable from the top, and that is not a property a comment can promise.
//
// So the mode is narrowed to the statements it exists for. A concurrent build pairs
// with a concurrent drop above it, because CREATE INDEX CONCURRENTLY IF NOT EXISTS
// sees the name an earlier failure left behind and skips, leaving an INVALID index
// forever. Anything else in such a file is a file that has lost atomicity for a
// statement that never needed to.
//
// The mirror holds too: a WRAPPED file may not mention CONCURRENTLY outside a comment,
// because Postgres would refuse it at deploy time and the gate is cheaper than a failed
// deploy.

import (
	"io/fs"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

// The two statements an unwrapped migration may say, and nothing else: a concurrent
// index build, and the concurrent drop that makes retrying one safe.
var (
	concurrentBuild = regexp.MustCompile(
		`(?is)^\s*create\s+(unique\s+)?index\s+concurrently\s+(if\s+not\s+exists\s+)?([\w".]+)`,
	)
	concurrentDrop = regexp.MustCompile(
		`(?is)^\s*drop\s+index\s+concurrently\s+if\s+exists\s+([\w".]+)`,
	)
)

// TestEveryUnwrappedMigrationOnlyBuildsIndexesConcurrently reads the embedded tree, so
// a namespace added later is covered without anybody remembering this file.
func TestEveryUnwrappedMigrationOnlyBuildsIndexesConcurrently(t *testing.T) {
	entries, err := fs.ReadDir(files, ".")
	if err != nil {
		t.Fatalf("reading the embedded migration namespaces: %v", err)
	}
	seen := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		loaded, err := dbmigrate.Load(files, entry.Name())
		if err != nil {
			t.Fatalf("loading %s: %v", entry.Name(), err)
		}
		for _, m := range loaded {
			seen++
			checkTransactionShape(t, entry.Name(), m)
		}
	}
	// The floor that stops this passing by reading nothing: 359 core up-migrations
	// when it landed, and a namespace that stops loading would otherwise read clean.
	if seen < 300 {
		t.Fatalf("this gate judged %d migration(s) and expects at least 300 — it has stopped "+
			"reading the tree rather than the tree having shrunk", seen)
	}
}

// checkTransactionShape holds one migration to its own declaration.
func checkTransactionShape(t *testing.T, namespace string, m dbmigrate.Migration) {
	t.Helper()
	for half, sql := range map[string]string{"up": m.UpSQL, "down": m.DownSQL} {
		// The name dropped last, so the pair is checked rather than the presence of a
		// drop: dropping old_idx and building new_idx leaves the build's own retry
		// unsafe, and that is the shape this would otherwise wave through.
		dropped := ""
		for statement := range strings.SplitSeq(sql, ";") {
			bare := strings.TrimSpace(stripSQLComments(statement))
			if bare == "" {
				continue
			}
			if !m.Unwrapped() {
				if strings.Contains(strings.ToUpper(bare), "CONCURRENTLY") {
					t.Errorf("%s %s_%s (%s half) says CONCURRENTLY inside a wrapped migration:"+
						"\n\t%s\nPostgres refuses that in a transaction block, so this fails at "+
						"deploy time. Add %s on a line of its own, and keep the file to index builds.",
						namespace, m.Version, m.Name, half, bare, dbmigrate.NoTransactionMarker)
				}
				continue
			}
			switch drop, build := concurrentDrop.FindStringSubmatch(bare),
				concurrentBuild.FindStringSubmatch(bare); {
			case drop != nil:
				dropped = drop[1]
			case build != nil:
				// The drop above it IS the retry. A build whose earlier attempt failed
				// left an INVALID index under that name, and CREATE ... IF NOT EXISTS
				// sees the name and skips — so the index stays invalid for good unless
				// something drops it first.
				if dropped != build[3] {
					t.Errorf("%s %s_%s (%s half) builds %s with no "+
						"DROP INDEX CONCURRENTLY IF EXISTS %s before it (dropped %q):\n\t%s\n"+
						"An unwrapped file has to survive being run twice: a failed build leaves an "+
						"INVALID index of that name, and the next run skips it on the name alone. "+
						"The drop above has to name the index the build creates.",
						namespace, m.Version, m.Name, half, build[3], build[3], dropped, bare)
				}
				dropped = ""
			default:
				t.Errorf("%s %s_%s (%s half) asked not to be wrapped and then says something "+
					"other than a concurrent index build or the drop that retries it:\n\t%s\n"+
					"An unwrapped file gives up atomicity and its bookkeeping row; it may only "+
					"contain the statements that need it. Move the rest to its own migration.",
					namespace, m.Version, m.Name, half, bare)
			}
		}
	}
}

// stripSQLComments removes line and block comments, so prose naming CONCURRENTLY — of
// which this tree has fifteen, all explaining why it was impossible — is not a
// statement.
func stripSQLComments(sql string) string {
	for {
		start := strings.Index(sql, "/*")
		if start < 0 {
			break
		}
		end := strings.Index(sql[start:], "*/")
		if end < 0 {
			sql = sql[:start]
			break
		}
		sql = sql[:start] + sql[start+end+2:]
	}
	var kept []string
	for line := range strings.SplitSeq(sql, "\n") {
		if cut := strings.Index(line, "--"); cut >= 0 {
			line = line[:cut]
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}
