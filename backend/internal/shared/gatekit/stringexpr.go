// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

// Reading what string a Go expression holds, for the censuses that judge one.
//
// ONE reader, with the question it is answering made a PARAMETER. There were
// two, in two files, and neither was a superset of the other: one followed
// `string("v1")` and the other did not; one refused a concatenation with an
// unresolvable half and the other folded the half it could read. Both
// behaviours are legitimate — a census asking "is this DEFINITELY this string"
// wants the first, one asking "what can I SEE of this string" wants the second
// — but which a census got was decided by which file its author had read most
// recently. A census's blast radius is exactly what its reader can see, and
// picking the narrower one does not fail, error, or look any different: it
// reports a clean tree over a shape the other reader would have caught.

import (
	"go/ast"
	"go/token"
	"strconv"
)

// StringFold says what a reading does with a part it cannot resolve.
type StringFold int

const (
	// FoldStrict answers "is this expression definitely this string". A part it
	// cannot resolve makes the whole expression not a string, so a census built
	// on it never judges text it half-invented.
	FoldStrict StringFold = iota
	// FoldTotal answers "what can I see of this string". A part it cannot
	// resolve becomes ComputedFragment and the fold carries on, so a statement
	// assembled from a literal and a variable is still judged on the literal.
	FoldTotal
)

// ComputedFragment stands in for a fragment a total fold cannot read — a
// function call, a parameter, an identifier from another package. It is one
// character so that a shape test over the folded text reads the fragment as a
// hole rather than as words the author did not write.
const ComputedFragment = "?"

// StringExpr reads expr as the string it produces, resolving identifiers
// through consts.
//
// It takes ast.Node rather than ast.Expr because most callers reach it from
// inside an ast.Inspect, where every node is a Node and asserting each one to
// an Expr first is a line that answers nothing.
//
// The bool means "this is definitely a string", not "the text is complete":
// under FoldTotal a concatenation with one readable half returns that half with
// true, and an unresolvable identifier on its own returns ComputedFragment with
// FALSE — which is what keeps a caller's walk descending into a call's
// arguments instead of stopping at the call.
func StringExpr(expr ast.Node, consts map[string]string, fold StringFold) (string, bool) {
	switch node := expr.(type) {
	case *ast.BasicLit:
		return basicLitString(node, fold)
	case *ast.ParenExpr:
		return StringExpr(node.X, consts, fold)
	case *ast.Ident:
		if text, known := consts[node.Name]; known {
			return text, true
		}
		if fold == FoldTotal {
			return ComputedFragment, false
		}
		return "", false
	case *ast.CallExpr:
		return conversionString(node, consts, fold)
	case *ast.BinaryExpr:
		return concatString(node, consts, fold)
	}
	if fold == FoldTotal {
		return ComputedFragment, false
	}
	return "", false
}

func basicLitString(lit *ast.BasicLit, fold StringFold) (string, bool) {
	if lit.Kind != token.STRING {
		return "", false
	}
	text, err := strconv.Unquote(lit.Value)
	if err == nil {
		return text, true
	}
	// A STRING literal the parser accepted and strconv cannot decode is not a
	// shape Go admits. A total fold stands a hole in for it rather than
	// inventing text; a strict one calls the expression unreadable.
	if fold == FoldTotal {
		return ComputedFragment, true
	}
	return "", false
}

// conversionString follows `string("gmail.com")`, which is a constant
// conversion and not a function this reader has to run — it is the identity on
// a string constant. Only the builtin spelled `string` with exactly one
// argument qualifies; a package-qualified or shadowed name is a different thing
// and is not followed.
func conversionString(call *ast.CallExpr, consts map[string]string, fold StringFold) (string, bool) {
	name, isIdent := call.Fun.(*ast.Ident)
	if !isIdent || name.Name != "string" || len(call.Args) != 1 {
		if fold == FoldTotal {
			return ComputedFragment, false
		}
		return "", false
	}
	return StringExpr(call.Args[0], consts, fold)
}

func concatString(expr *ast.BinaryExpr, consts map[string]string, fold StringFold) (string, bool) {
	if expr.Op != token.ADD {
		return "", false
	}
	left, leftIsString := StringExpr(expr.X, consts, fold)
	right, rightIsString := StringExpr(expr.Y, consts, fold)
	if fold == FoldTotal {
		// One readable half is enough: a statement assembled from a literal and
		// a variable is still judged on the literal, which is the half that
		// carries the words a census looks for.
		//
		// With NEITHER half readable the text is still both holes, not empty.
		// Returning "" there let a surrounding concatenation close over the gap
		// — `"a" + (x + y) + "b"` folding to `ab` — and a census then reads a
		// word the source does not contain, which is the one way a total fold
		// can be worse than no fold at all.
		return left + right, leftIsString || rightIsString
	}
	if !leftIsString || !rightIsString {
		return "", false
	}
	return left + right, true
}
