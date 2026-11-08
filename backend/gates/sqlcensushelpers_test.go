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
	// writeTarget matches the table a write names, through every identifier
	// spelling Postgres accepts: quoted, schema-qualified, `ONLY`-prefixed. The
	// writes are `UPDATE`, `INSERT` (whose `ON CONFLICT` can carry an update)
	// and `MERGE`.
	writeTarget = regexp.MustCompile(
		`(?is)\b(?:update|insert\s+into|merge\s+into)\s+(?:only\s+)?(?:"?[a-z_][a-z0-9_]*"?\.)?"?([a-z_][a-z0-9_]*)"?`)
)

// setClauses isolates what a statement assigns from what it merely names. A
// column read in a `WHERE` predicate or listed in a `SELECT` is not a write.
// `SET` is the one token every assigning form shares: a plain `UPDATE`, an
// upsert's `DO UPDATE`, and the update inside a `MERGE`.
//
// A clause ends at `WHERE`, `RETURNING` or `;`, found outside quoted strings
// and comments. A literal holding one of those words is still a value, and
// ending there would hide every assignment after it. A span that never closes
// runs to the end of the text, so splitSetClause can report it.
func setClauses(statement string) []string {
	var clauses []string
	start := -1
	for i := 0; i < len(statement); i++ {
		if span, _ := gatekit.SQLSpanAt(statement, i); span > 0 {
			i += span - 1
			continue
		}
		switch {
		case start < 0 && sqlWordAt(statement, i, "set"):
			start = i + len("set")
			i = start - 1
		case start >= 0 && (statement[i] == ';' || sqlWordAt(statement, i, "where") || sqlWordAt(statement, i, "returning")):
			clauses = append(clauses, statement[start:i])
			start = -1
		}
	}
	if start >= 0 {
		clauses = append(clauses, statement[start:])
	}
	return clauses
}

// sqlWordAt reports whether word, in any case, stands at pos as a whole word.
// The boundary keeps `set` out of `offset`.
func sqlWordAt(text string, pos int, word string) bool {
	end := pos + len(word)
	if end > len(text) || !strings.EqualFold(text[pos:end], word) {
		return false
	}
	return (pos == 0 || !sqlIdentByte(text[pos-1])) && (end == len(text) || !sqlIdentByte(text[end]))
}

func sqlIdentByte(b byte) bool {
	return b == '_' || b >= '0' && b <= '9' || b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z'
}
