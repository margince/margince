// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A constraint's name goes in the operator's log, never in the caller's refusal.
//
// storekit.CheckViolation and UniqueViolation hand back the name of the rule
// that fired, and there are two things to do with it. Switch on it, to answer
// with a refusal written for that rule — which every site in this tree does, and
// which is the whole point of exposing it. Or put it in the message, which
// `activities/handlers.go` once did:
//
//	"the request violates the " + constraint + " business rule"
//
// A schema identifier tells a caller the shape of a table they cannot otherwise
// see and nothing they can act on — `activity_link_shape` names no field in the
// published contract — and it is not API: renaming a constraint in a migration
// would silently change a response body. httperr's own constraint net says so
// at its generic path, and refuses to name one there (issue #1522).
//
// A PROHIBITION rather than a census, because the failure is a leak by default:
// interpolation costs nothing to write and the next constraint inherits it. The
// rule is that the value never becomes message text — passing it to a mapper
// that switches on it is the sanctioned use and is what the sweep expects to
// find.
//
// What this cannot see, stated rather than implied: a name copied into another
// variable first, or one that reaches a message through a helper this walk does
// not follow. The walk is syntactic. It catches the shape that was actually
// written here and the shape the next author would reach for, which is the
// concatenation and the format verb — not an author determined to launder it.

import (
	"go/ast"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The two storekit doors that hand back a constraint name.
var constraintDoors = map[string]bool{
	"CheckViolation":  true,
	"UniqueViolation": true,
}

// The formatters a leak would travel through. A name reaching one of these is
// on its way into a string, and every string this tree builds from an error is
// a string somebody may write to a client.
var stringBuilders = map[string]bool{
	"Sprintf": true, "Errorf": true, "Sprint": true, "Sprintln": true,
	"Join": true, "Replace": true, "ReplaceAll": true,
}

func TestNoConstraintNameIsBuiltIntoAMessage(t *testing.T) {
	t.Parallel()

	files := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: fileReadsAConstraintName,
		// Not extensions/: a unit cannot import storekit at all, so it never
		// holds one of these names to leak.
	}.Files(t)

	sites := 0
	for _, parsed := range files {
		// storekit itself is where the name is produced and named; its own
		// error text is the operator's, not a caller's.
		if strings.Contains(parsed.Path, "/platform/database/storekit/") {
			continue
		}
		for _, leak := range constraintNameLeaksIn(parsed) {
			sites++
			t.Errorf("%s: %s\n"+
				"\tA constraint name is schema the caller cannot otherwise see, and nothing they "+
				"can act on — it names no field in the published contract, and renaming it in a "+
				"migration would silently change a response body.\n"+
				"\tSwitch on it to answer with a refusal written for that rule, and let anything "+
				"unmapped fall through to httperr's field-less one. Log the name if an operator "+
				"needs it.", parsed.Path, leak)
		}
	}
	// The gate has to have swept the sites it judges. Every module that answers
	// a schema refusal reads one of these doors, so nothing found means the
	// walk broke or the doors were renamed.
	if len(files) == 0 {
		t.Fatal("no file reads a constraint name at all, so this prohibition judged nothing — " +
			"the walk is broken, or storekit's doors were renamed and this rule now binds names " +
			"nobody calls")
	}
	if sites == 0 {
		t.Logf("swept %d file(s) that read a constraint name; none builds it into a message", len(files))
	}
}

func fileReadsAConstraintName(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && constraintDoors[sel.Sel.Name] {
			found = true
		}
		return true
	})
	return found
}

// constraintNameLeaksIn reports each place a name bound from one of the doors is
// concatenated or formatted, described for the message above.
func constraintNameLeaksIn(parsed gatekit.ParsedFile) []string {
	var leaks []string
	for _, name := range constraintBindings(parsed.File) {
		ast.Inspect(parsed.File, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.BinaryExpr:
				// `"…" + constraint + "…"` — the shape that was written.
				if node.Op == token.ADD && (identIs(node.X, name) || identIs(node.Y, name)) {
					leaks = append(leaks, "`"+name+"` is concatenated into a string")
				}
			case *ast.CallExpr:
				sel, ok := node.Fun.(*ast.SelectorExpr)
				if !ok || !stringBuilders[sel.Sel.Name] {
					return true
				}
				for _, arg := range node.Args {
					if identIs(arg, name) {
						leaks = append(leaks, "`"+name+"` is formatted by "+sel.Sel.Name)
					}
				}
			}
			return true
		})
	}
	return dedupeLeaks(leaks)
}

// constraintBindings reads the identifiers this file binds from a door — the
// `constraint` in `constraint, ok := storekit.CheckViolation(err)`, whatever it
// is spelled.
//
// Held by: TestNoConstraintNameIsBuiltIntoAMessage
func constraintBindings(file *ast.File) []string {
	var names []string
	ast.Inspect(file, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok || len(assign.Rhs) != 1 || len(assign.Lhs) == 0 {
			return true
		}
		call, ok := assign.Rhs[0].(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || !constraintDoors[sel.Sel.Name] {
			return true
		}
		if ident, ok := assign.Lhs[0].(*ast.Ident); ok && ident.Name != "_" {
			names = append(names, ident.Name)
		}
		return true
	})
	return names
}

func identIs(expr ast.Expr, name string) bool {
	ident, ok := expr.(*ast.Ident)
	return ok && ident.Name == name
}

func dedupeLeaks(items []string) []string {
	var kept []string
	seen := map[string]bool{}
	for _, item := range items {
		if !seen[item] {
			seen[item] = true
			kept = append(kept, item)
		}
	}
	return kept
}
