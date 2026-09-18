// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// Who may bind an arrival to the activity already holding its identity is
// decided in ONE place: activities.bindableIdentityUnder, which both exported
// entry points delegate to.
//
// Two ingestion doors ask the question — the import door through
// boundToKnownMessage, the capture door through the IdentityResolver seam
// compose injects. A Message-ID is a header its sender types, so the answer
// decides whether one colleague's mail becomes reachable through another's.
// Two copies of that rule would drift silently: each door would go on
// answering, just differently, and the weaker one is the one an attacker picks.
//
// This gate holds the claim by counting the callers of the two predicates the
// rule is made of. Each may be called only from inside that one body — a second
// caller means somebody has assembled their own version of the rule rather than
// asking the function that states it.
//
// What this gate CANNOT see: whether a caller obeys the answer. A door that
// calls ResolveBindableIdentity and then binds anyway passes here. The
// integration tests in compose/importthencapture_integration_test.go are what
// cover that, and the cross-seat cases there are mutation-checked against the
// removal of the arm each one covers.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// theBindingRule names the predicates that together decide a bind. Neither is
// meaningful alone: BindableTo without the liveness read binds to tombstones,
// and the liveness read without BindableTo binds across seats.
var theBindingRule = []string{"BindableTo", "identityHolderIsLive"}

// theOneDecider is the function allowed to call them.
//
// The UNEXPORTED body, not either exported entry point. There are two of those
// — ResolveBindableIdentity for a door with nothing to prove, and
// ResolveBindableIdentityProving for one that carries an address prover — and
// they exist so a caller chooses which rule applies, not so either restates it.
// Both delegate here, so this is still exactly one place where a bind is
// decided, which is the property this gate holds.
const theOneDecider = "bindableIdentityUnder"

func TestOneFunctionDecidesWhoMayBindToAMessage(t *testing.T) {
	t.Parallel()
	callers := map[string][]string{}
	for _, path := range goSourceFiles(t, ".") {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		for predicate, in := range bindingRuleCallers(t, path) {
			callers[predicate] = append(callers[predicate], in...)
		}
	}
	for _, predicate := range theBindingRule {
		found := callers[predicate]
		// The census's own proof of life: a scan that stopped finding the call
		// would otherwise report PASS over nothing at all.
		if len(found) == 0 {
			t.Errorf("no caller of %s found anywhere — the scan for it has stopped working, "+
				"or the predicate was renamed without updating this gate", predicate)
			continue
		}
		sort.Strings(found)
		var strangers []string
		for _, in := range found {
			if in != theOneDecider {
				strangers = append(strangers, in)
			}
		}
		if len(strangers) > 0 {
			t.Errorf("%s is called from %s, but only %s may call it.\n\n"+
				"Binding an arrival to an existing message is what makes one row's content "+
				"reachable through the other, on the strength of a Message-ID its sender typed. "+
				"Ask %s instead of assembling a second copy of the rule: a copy drifts, and "+
				"the drift is silent because both doors go on answering.",
				predicate, strings.Join(strangers, ", "), theOneDecider, theOneDecider)
		}
	}
}

// bindingRuleCallers maps each predicate of the rule to the functions in this
// file that call it.
func bindingRuleCallers(t *testing.T, path string) map[string][]string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	wanted := map[string]bool{}
	for _, predicate := range theBindingRule {
		wanted[predicate] = true
	}
	out := map[string][]string{}
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok {
			continue
		}
		ast.Inspect(fn, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			// Both spellings: the bare name inside the owning package, and the
			// qualified one from anywhere else.
			name := ""
			switch f := call.Fun.(type) {
			case *ast.Ident:
				name = f.Name
			case *ast.SelectorExpr:
				name = f.Sel.Name
			}
			if wanted[name] {
				out[name] = append(out[name], fn.Name.Name)
			}
			return true
		})
	}
	return out
}
