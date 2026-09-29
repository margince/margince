// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A calendar verb acting on a row it found by the provider's event id proves
// the acting seat holds that row. The proof travels as a seam, so the whole
// obligation is one argument in one file — and an argument is deleted by
// accident in a way a rule is not.
//
// This gate reads how compose wires the two verbs. Each closer and mover must
// be a CALL to the constructor that carries the standing seam, never the bare
// writer beside it: the bare writers stay exported for the RSVP backfill, which
// re-reads a row's own stored original under no seat at all and would be turned
// into a permanent no-op by a guard.
//
// What this gate CANNOT see: whether the seam compose passes answers honestly.
// A constructor handed a predicate that always says yes passes here. The
// integration cases in compose/meetingseat_integration_test.go are what cover
// that, for both verbs and in both directions.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"
)

// verbGuard names one seam's guarded constructor: the FIRST argument
// `seam` must be built from, in every call that wires it up. A slice rather
// than a map from subject to reason — this is the assertion, not a waived
// exception, and the gate census over the package's own reason maps
// (TestEveryPackageLevelReasonMapIsAWaiverOrADeclaredFixture) is about the
// latter.
type verbGuard struct {
	seam        string
	constructor string
}

// verbGuards names, for each seam, the constructor that carries the standing
// check — and, for WithMeetingCloser, only its FIRST argument: the by-identity
// closer beside it resolves through the bindable identity rule instead and
// takes no seat argument of its own.
var verbGuards = []verbGuard{
	{seam: "WithMeetingCloser", constructor: "CancelCapturedMeetingFor"},
	{seam: "WithMeetingMover", constructor: "MoveCapturedMeetingFor"},
}

// constructorFor reports the constructor a seam must be wired with, or false
// if the name is not one of the two calendar verbs this gate holds.
func constructorFor(seam string) (string, bool) {
	for _, g := range verbGuards {
		if g.seam == seam {
			return g.constructor, true
		}
	}
	return "", false
}

func TestComposeWiresNoUnguardedCalendarVerb(t *testing.T) {
	t.Parallel()
	seen := map[string]int{}
	for _, path := range goSourceFiles(t, ".") {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			want, guarded := constructorFor(sel.Sel.Name)
			if !guarded || len(call.Args) == 0 {
				return true
			}
			seen[sel.Sel.Name]++
			if got := constructorOf(call.Args[0]); got != want {
				t.Errorf("%s at %s is wired with %s, but only %s carries the standing check.\n\n"+
					"A calendar verb finds its row by the provider's event id. The provider check "+
					"binds that key to the acting CONNECTOR and never to the seat whose calendar is "+
					"syncing, so without the seam a connection stating another seat's event id "+
					"reaches that seat's meeting.",
					sel.Sel.Name, fset.Position(call.Pos()), got, want)
			}
			return true
		})
	}
	// The census's own proof of life: a scan that stopped finding the wiring
	// would otherwise report PASS over nothing at all.
	for _, g := range verbGuards {
		if seen[g.seam] == 0 {
			t.Errorf("no call to %s found anywhere — the scan for it has stopped working, "+
				"or the seam was renamed without updating this gate", g.seam)
		}
	}
}

// constructorOf names the function an argument is a call to, or describes what
// it is instead, so the failure names what somebody actually wrote.
func constructorOf(arg ast.Expr) string {
	call, ok := arg.(*ast.CallExpr)
	if !ok {
		if sel, ok := arg.(*ast.SelectorExpr); ok {
			return sel.Sel.Name + " (passed bare, not through a constructor)"
		}
		return "something other than a constructor call"
	}
	switch fn := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fn.Sel.Name
	case *ast.Ident:
		return fn.Name
	}
	return "an unnamed call"
}
