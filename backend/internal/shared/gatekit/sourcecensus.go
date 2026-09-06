// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

// Two source censuses, read with the language's own parser.
//
// Both rules used to be awk scanning text. Six defects shipped in that
// machinery and none of them was in the rule: `-f lib.awk 'inline'` reading the
// program as a filename so the gate ran no rules at all, `log` being an awk
// built-in, POSIX ERE having no word boundary, an unlisted local becoming
// global, a flat flag that cannot express nesting, and a backslash meaning two
// different things in two grammars. Every one is answered for nothing by a
// parser that already knows where a string ends.
//
// A parse ERROR is the point of the shape, not an inconvenience: a scanner that
// reaches the end of a file still inside a string has gone blind, and a census
// reporting OK over the rest of that file is the one failure a census must not
// have. Here it cannot happen silently — the error is the answer.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"regexp"
	"strconv"
	"strings"
)

// A Finding is one place a census refuses, at the line a reader should open.
type Finding struct {
	Path string
	Line int
	// Text is the source the census judged, for a message that shows the
	// reader what it saw rather than only where.
	Text string
}

func (f Finding) String() string { return fmt.Sprintf("%s:%d: %s", f.Path, f.Line, f.Text) }

// minorUnitName is an identifier naming an amount already in minor units. Both
// the Go and the TypeScript census spell it, and the corpus holds them to the
// same cases — see backend/gates/testdata/sourcecensus.json.
var minorUnitName = regexp.MustCompile(`[Mm]inor[A-Za-z_]*|MINOR[A-Z_]*`)

// hardCodedScale is a power of ten a currency's decimals are not allowed to be
// assumed to be. A currency with no minor unit (VND, JPY, KRW) is understated a
// hundredfold by /100, and a three-decimal one (KWD) is overstated tenfold.
var hardCodedScale = map[string]bool{"10": true, "100": true, "1000": true, "10000": true}

// MoneyScaleFindings reports every minor-unit amount scaled by a hard-coded
// power of ten in one Go file.
//
// The unit judged is the STATEMENT, not the line: a formatter routinely breaks
// `Math.Round(amount * 100)` across four lines, and a line-scoped matcher reads
// the multiply and the name as unrelated. An AST has no lines to lose.
func MoneyScaleFindings(path string, src []byte) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("%s: the parser could not finish this file, so a census over it would "+
			"be a census of nothing: %w", path, err)
	}
	waived := waivedLines(fset, file, "money-scale-exempt:")
	var found []Finding
	for _, scaled := range scalingExpressions(file) {
		holder := enclosing(file, scaled)
		if !mentionsMinorUnit(holder) {
			continue
		}
		line := fset.Position(holder.Pos()).Line
		last := fset.Position(holder.End()).Line
		if waivedBetween(waived, line, last) {
			continue
		}
		found = append(found, Finding{Path: path, Line: line, Text: exprText(fset, src, scaled)})
	}
	return found, nil
}

// scalingExpressions is every `x / 100`, `x * 100` or `x % 100` in the file.
func scalingExpressions(file *ast.File) []ast.Node {
	var found []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		binary, ok := n.(*ast.BinaryExpr)
		if !ok {
			return true
		}
		switch binary.Op {
		case token.QUO, token.MUL, token.REM:
		default:
			return true
		}
		if isHardCodedScale(binary.X) || isHardCodedScale(binary.Y) {
			found = append(found, n)
		}
		return true
	})
	return found
}

// isHardCodedScale reads an integer literal as its value, so the house style
// for a grouped literal — 10_000 — is the same finding as 10000. The awk had to
// spell both, and spelled one of them wrong for a while.
func isHardCodedScale(expr ast.Expr) bool {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.INT {
		return false
	}
	return hardCodedScale[strings.ReplaceAll(literal.Value, "_", "")]
}

// enclosing is the span a minor-unit name may be found in: the statement or
// declaration the scaling sits in — but never PAST a composite literal.
//
// The statement alone is too wide, and the tree says so. A feature vector
// builds five fields in one assignment, and one of them divides a percentage by
// 100 while another names a minor-unit base; a rule reading the whole statement
// calls that money. Stopping at the literal's own element is the difference
// between a census people read and one whose escape hatch they use routinely.
//
// The statement is still the unit everywhere else, and it has to be: a
// formatter breaks `amountMinor := int64(major * 100)` across three lines, and
// the name is on the first of them.
func enclosing(file *ast.File, target ast.Node) ast.Node {
	path := ancestors(file, target)
	for i := len(path) - 1; i >= 0; i-- {
		switch path[i].(type) {
		case *ast.CompositeLit:
			// The element we arrived through, not the literal holding it: its
			// siblings are other fields, and they are not this one's context.
			return path[i+1]
		case ast.Stmt, ast.Spec:
			return path[i]
		}
	}
	return target
}

// ancestors is the node path from the file down to the target, target last.
func ancestors(file *ast.File, target ast.Node) []ast.Node {
	var path, found []ast.Node
	ast.Inspect(file, func(n ast.Node) bool {
		if found != nil {
			return false
		}
		if n == nil {
			path = path[:len(path)-1]
			return false
		}
		path = append(path, n)
		if n == target {
			found = append([]ast.Node(nil), path...)
			return false
		}
		return true
	})
	return found
}

func mentionsMinorUnit(holder ast.Node) bool {
	named := false
	ast.Inspect(holder, func(n ast.Node) bool {
		ident, ok := n.(*ast.Ident)
		if ok && minorUnitName.MatchString(ident.Name) {
			named = true
		}
		return !named
	})
	return named
}

// exprText is the source the finding is about, so the message shows the reader
// what was read rather than asking them to go and find it.
func exprText(fset *token.FileSet, src []byte, node ast.Node) string {
	from, to := fset.Position(node.Pos()).Offset, fset.Position(node.End()).Offset
	if from < 0 || to > len(src) || from >= to {
		return ""
	}
	return strings.Join(strings.Fields(string(src[from:to])), " ")
}

// waivedLines collects the lines carrying an in-source waiver marker.
//
// A COMMENT only. The marker written inside a string literal waives nothing,
// which the awk had to be taught and a parser knows: the two are different
// nodes.
func waivedLines(fset *token.FileSet, file *ast.File, marker string) map[int]bool {
	waived := map[int]bool{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if !strings.Contains(comment.Text, marker) {
				continue
			}
			from := fset.Position(comment.Pos()).Line
			for line := from; line <= fset.Position(comment.End()).Line; line++ {
				waived[line] = true
			}
		}
	}
	return waived
}

// waivedBetween reports whether a waiver falls anywhere in the span judged. The
// span IS the statement, so a waiver written beside a wrapped expression covers
// the expression it is beside and nothing further.
func waivedBetween(waived map[int]bool, from, to int) bool {
	for line := from; line <= to; line++ {
		if waived[line] {
			return true
		}
	}
	return false
}

// OneSpellingFindings reports the three predicates this tree owns in exactly one
// place, spelled again as a literal: a SQLSTATE code, the wire code a
// hand-rolled CHECK translation used, and the ISO-4217 shape.
//
// The literal's VALUE is what is judged, so a SQLSTATE quoted inside a
// paragraph of SQL is not a second spelling of the judgement — it is SQL. The
// awk could only match the token wherever it appeared.
func OneSpellingFindings(path string, src []byte, sqlstates []string) ([]Finding, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, src, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("%s: the parser could not finish this file, so a census over it would "+
			"be a census of nothing: %w", path, err)
	}
	codes := make(map[string]bool, len(sqlstates))
	for _, code := range sqlstates {
		codes[code] = true
	}
	waived := waivedLines(fset, file, "one-spelling-exempt:")
	var found []Finding
	ast.Inspect(file, func(n ast.Node) bool {
		literal, ok := n.(*ast.BasicLit)
		if !ok || literal.Kind != token.STRING {
			return true
		}
		line := fset.Position(literal.Pos()).Line
		if waivedBetween(waived, line, fset.Position(literal.End()).Line) {
			return true
		}
		if reason := oneSpellingReason(literalValue(literal), codes); reason != "" {
			found = append(found, Finding{Path: path, Line: line, Text: reason})
		}
		return true
	})
	return found, nil
}

// isoShape is the ISO-4217 check spelled as a regexp, which values.ValidCurrency
// owns. The rune-loop spelling is deliberately NOT matched: it is the shape any
// string scanner uses, and matching it refuses an alphanumeric word splitter
// that has nothing to do with currency. A gate whose escape hatch gets used
// routinely stops being read.
const isoShape = `^[A-Z]{3}$`

func oneSpellingReason(value string, codes map[string]bool) string {
	switch {
	case codes[value]:
		return fmt.Sprintf("SQLSTATE %q outside storekit — use storekit.UniqueViolation / "+
			"IsForeignKeyViolation / CheckViolation / ExclusionViolation / IsQueryCanceled", value)
	case value == "constraint_violated":
		return fmt.Sprintf("%q is a hand-rolled CHECK-to-422 translation — let httperr's constraint "+
			"net answer, or refuse earlier naming the caller's own field with its own code", value)
	case strings.Contains(value, isoShape):
		return fmt.Sprintf("%q is a private ISO-4217 shape check — use values.ValidCurrency, so Go "+
			"and the schema's CHECK admit the same set", value)
	}
	return ""
}

// literalValue is the string a literal HOLDS. An interpreted literal is
// unquoted so an escape is read the way the compiler reads it; a raw one keeps
// its bytes.
func literalValue(literal *ast.BasicLit) string {
	if unquoted, err := strconv.Unquote(literal.Value); err == nil {
		return unquoted
	}
	return strings.Trim(literal.Value, "`\"")
}
