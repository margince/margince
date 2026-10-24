// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The helpers several gates share for reading source and the SQL inside it.
//
// They lived in the forecast-movement gate until that invariant's table came
// out. Six of its declarations turned out to be borrowed by other gates, and
// a gate's own machinery belongs in its own file. These belong to none of them.

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// packageFiles pairs each of a package's parsed sources with the path it came
// from, so a finding names a file rather than a syntax tree.
//
// It walks nothing itself. parsePackageDir is the package walk this directory
// already has, and a second one would learn a new exclusion in only one place.
// The half that stopped parsing would read green over whatever it no longer saw.
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
