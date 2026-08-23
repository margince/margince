// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// Reading the SQL a Go file sends, for the censuses that judge statements.
//
// ONE reading, because there were two. `statementsIn` and `sqlLiteralsIn` were
// the same walk under different names, and they had already drifted: one
// decodes escapes and flattens `+` chains, the other read source text a literal
// at a time — so a probe written in double quotes was invisible to it while its
// sibling saw it. Two answers to "what does this file send the database" is the
// defect a census exists to find, standing inside the censuses.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// sqlLiteralsIn returns every string this file builds, one entry per STATEMENT
// rather than per literal.
//
// Adjacent literals joined with `+` are flattened into one entry, because a
// statement assembled that way is one statement to Postgres and three fragments
// to the parser — and no fragment on its own carries `organization_id`, `FROM
// organization_domain` and `domain =` together, so a probe written with a `+`
// would match nothing at all. A detector that reads a statement in pieces
// reports a clean tree over exactly the form somebody reaches for when a line
// gets long.
func sqlStatementsIn(t *testing.T, path, source string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	var out []string
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.BinaryExpr:
			if typed.Op != token.ADD {
				return true
			}
			if joined, ok := concatenatedString(typed); ok {
				out = append(out, joined)
				// Do not descend: the parts are this statement, already read.
				return false
			}
		case *ast.BasicLit:
			if typed.Kind == token.STRING {
				out = append(out, unquoted(typed))
			}
		}
		return true
	})
	return out
}

// unquoted is the string the DATABASE receives, not the source text.
//
// `ast.BasicLit.Value` is what the author typed, so an interpreted literal
// keeps its escapes: `"…organization_id\nFROM…"` holds a backslash and an `n`
// where Postgres receives a newline, and a pattern asking for `\s` finds
// neither. A probe written in double quotes would pass this census untouched
// while sending the very statement it exists to find.
//
// A literal that will not unquote is returned as it stands: the census is a
// finding-machine, and a fragment it cannot decode is better reported than
// dropped.
func unquoted(lit *ast.BasicLit) string {
	if text, err := strconv.Unquote(lit.Value); err == nil {
		return text
	}
	return lit.Value
}

// concatenatedString flattens a `+` chain of string literals into one text.
//
// A non-literal operand — an interpolated identifier — contributes nothing it
// can read, so it becomes a space: the surrounding SQL still joins up, and the
// pattern sees the shape rather than being broken by the hole.
func concatenatedString(expr ast.Expr) (string, bool) {
	switch typed := expr.(type) {
	case *ast.BasicLit:
		if typed.Kind != token.STRING {
			return " ", false
		}
		return unquoted(typed), true
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return " ", false
		}
		left, leftOK := concatenatedString(typed.X)
		right, rightOK := concatenatedString(typed.Y)
		return left + right, leftOK || rightOK
	default:
		return " ", false
	}
}

func firstSQLStatementLine(statement string) string {
	for _, line := range strings.Split(statement, "\n") {
		if trimmed := strings.TrimSpace(strings.Trim(line, "`\"")); trimmed != "" {
			return trimmed
		}
	}
	return strings.TrimSpace(statement)
}
