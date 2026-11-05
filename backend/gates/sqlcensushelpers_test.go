// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The SQL-statement helpers several censuses share.
//
// They cover the parsed files of one package, the statements a file's string
// literals hold, and the fragments a write is recognised by.
//
// They live on their own because the callers have nothing else in common. Six
// gates reach for them: module table spelling, partial index inference,
// request key conflict, account reach copies and the two PII censuses.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func packageFiles(t *testing.T, fset *token.FileSet, dir string) []gatekit.ParsedFile {
	t.Helper()
	var out []gatekit.ParsedFile
	for _, file := range parsePackageDir(t, fset, dir) {
		out = append(out, gatekit.ParsedFile{
			Path: filepath.ToSlash(fset.Position(file.Pos()).Filename),
			File: file,
		})
	}
	return out
}

// statementsIn returns every SQL-shaped string in one file, comments stripped
// and whitespace collapsed, with adjacent literals joined the way the compiler
// joins them.
//
// It matches statements rather than lines. A statement broken across lines is
// one write however it is laid out. A column named in a `--` comment is prose,
// and one a Go author split with `+` is still a single statement.
func statementsIn(file *ast.File) []string {
	var out []string
	for _, literal := range gatekit.SQLStatementsOf(file) {
		// Split on `;` after stripping comments, so a semicolon inside a `--`
		// comment does not cut a statement in half.
		//
		// One Go string can hold two statements, and placing the pair by its
		// first table passes falsely. `UPDATE offer SET status = 'x'; UPDATE
		// deal SET amount_minor = $1` would resolve to `offer` and report
		// nothing. That is the one outcome this census must not produce, so
		// each statement is placed on its own.
		for statement := range strings.SplitSeq(stripSQLComments(literal), ";") {
			if collapsed := collapsedSQL(statement); collapsed != "" {
				out = append(out, collapsed)
			}
		}
	}
	return out
}

// stripSQLComments removes `--` line comments so a column named in one is not
// read as an assignment.
func stripSQLComments(sql string) string {
	var kept []string
	for line := range strings.SplitSeq(sql, "\n") {
		if at := strings.Index(line, "--"); at >= 0 {
			line = line[:at]
		}
		kept = append(kept, line)
	}
	return strings.Join(kept, "\n")
}

// What a statement assigns, and the table it assigns into.
//
// Shared because more than one gate asks a statement the same question. A
// second spelling of these patterns would read a clean tree over whatever it
// did not know.
var (
	// setClause isolates what a statement assigns from what it merely names. A
	// column read in a `WHERE` predicate or listed in a `SELECT` is not a
	// write. `SET` is the one token every assigning form shares: a plain
	// `UPDATE`, an upsert's `DO UPDATE`, and the update inside a `MERGE`.
	// `\b` keeps it out of `offset`.
	setClause = regexp.MustCompile(`(?is)\bset\b(.*?)(?:\bwhere\b|\breturning\b|;|$)`)
	// writeTarget matches the table a write names, through every identifier
	// spelling Postgres accepts: quoted, schema-qualified, `ONLY`-prefixed. The
	// writes are `UPDATE`, `INSERT` (whose `ON CONFLICT` can carry an update)
	// and `MERGE`.
	writeTarget = regexp.MustCompile(
		`(?is)\b(?:update|insert\s+into|merge\s+into)\s+(?:only\s+)?(?:"?[a-z_][a-z0-9_]*"?\.)?"?([a-z_][a-z0-9_]*)"?`)
	// assignmentFmt matches `col = …` and the multi-column form
	// `(a, col) = (…)`, with or without the identifier quoted.
	//
	// Anchored on the whole identifier, so `currency` is not found inside
	// `base_currency`: the substring reading is the half that survives a
	// careless escape. The parenthesised branch may not cross a `(` of its own,
	// so a column list cannot reach forward to an unrelated `=`.
	assignmentFmt = `(?i)(?:(?:^|[,\s"])%[1]s"?\s*=)|(?:\([^()=]*[,\s"]?%[1]s"?[^()=]*\)\s*=)`
)
