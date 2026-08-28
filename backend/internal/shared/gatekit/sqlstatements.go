// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

// Reading the SQL a Go file sends, for every census that judges statements.
//
// ONE reading, here rather than in each census. Each would otherwise answer
// "what does this file send the database" for itself, and two answers to that
// question is the defect a census exists to find — a census whose reader is
// narrower than its sibling's reports a clean tree over what the sibling sees.
//
// It lives in gatekit rather than in the gates package because the readers are
// not all gates: internal/shared/gatekit's own schedule-clock census and
// several modules' package-local censuses ask the same question, and a reader
// they cannot import is a reader they will write again.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// SQLStatementsIn returns every string the file builds, one entry per STATEMENT
// rather than per literal, with escapes decoded.
//
// Two properties, and a census silently under-reports without either.
//
// DECODED, because ast.BasicLit.Value is SOURCE TEXT. For a backticked literal
// the text and the string are the same; for a double-quoted one they are not —
// "UPDATE x\nSET y" carries a backslash and an `n` where Postgres receives a
// newline. A pattern asking for `\s`, or a reader splitting on "\n", sees one
// unbroken line and reports nothing. Nothing in this tree makes SQL backticked;
// a census that is only correct because of that convention is blind the day
// somebody writes one statement the other way.
//
// FLATTENED, because a statement assembled with `+` is one statement to
// Postgres and several fragments to the parser, and no fragment on its own
// carries the words a probe looks for together. A detector that reads a
// statement in pieces reports a clean tree over exactly the form somebody
// reaches for when a line gets long.
func SQLStatementsIn(t testing.TB, path, source string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, source, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return SQLStatementsOf(file)
}

// SQLStatementsOf is SQLStatementsIn over a subtree already parsed — the shape
// a census wants when it is walking declarations rather than whole files.
func SQLStatementsOf(node ast.Node) []string {
	var out []string
	ast.Inspect(node, func(n ast.Node) bool {
		switch typed := n.(type) {
		case *ast.BinaryExpr:
			if typed.Op != token.ADD {
				return true
			}
			if joined, ok := ConcatenatedString(typed); ok {
				out = append(out, joined)
				// The literal parts are this statement, already read. The parts
				// that are NOT literals are still walked: `"x " + fmt.Sprintf(
				// "SELECT …")` folds the readable half and would otherwise take
				// the whole Sprintf subtree with it — a statement the
				// per-literal readers this replaced could all see.
				for _, opaque := range opaqueOperands(typed) {
					out = append(out, SQLStatementsOf(opaque)...)
				}
				return false
			}
		case *ast.BasicLit:
			if text, isString := LiteralText(typed); isString {
				out = append(out, text)
			}
		}
		return true
	})
	return out
}

// ConcatenatedString flattens a `+` chain of string literals into one text, and
// reports whether any operand was readable.
//
// A non-literal operand — an interpolated identifier — contributes nothing it
// can read, so it becomes a space: the surrounding SQL still joins up, and the
// pattern sees the shape rather than being broken by the hole. A hole that
// closed up instead would let two fragments spell a word neither wrote.
func ConcatenatedString(expr ast.Expr) (string, bool) {
	switch typed := expr.(type) {
	case *ast.BasicLit:
		text, isString := LiteralText(typed)
		if !isString {
			return " ", false
		}
		return text, true
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return " ", false
		}
		left, leftOK := ConcatenatedString(typed.X)
		right, rightOK := ConcatenatedString(typed.Y)
		return left + right, leftOK || rightOK
	default:
		return " ", false
	}
}

// opaqueOperands are the parts of a `+` chain ConcatenatedString could not
// read — everything that is neither a string literal nor another `+`.
func opaqueOperands(expr ast.Expr) []ast.Expr {
	switch typed := expr.(type) {
	case *ast.BasicLit:
		return nil
	case *ast.BinaryExpr:
		if typed.Op != token.ADD {
			return []ast.Expr{typed}
		}
		return append(opaqueOperands(typed.X), opaqueOperands(typed.Y)...)
	default:
		return []ast.Expr{expr}
	}
}

// SQLTextOf joins the file's statements with newlines — for a
// reader that scans lines rather than statements. Separated rather than
// concatenated so a pattern anchored to a line cannot run off one statement's
// end into the next one's beginning.
func SQLTextOf(t testing.TB, path string) string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Errorf("parsing %s: %v", path, err)
		return ""
	}
	return strings.Join(SQLStatementsOf(parsed), "\n")
}
