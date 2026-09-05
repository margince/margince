// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every forecasting store entry point that RECORDS against a scope asks whether
// its caller answers for that scope.
//
// The object grant and the scope are different questions, and the module asked
// only the first: auth.Require says a seat may make forecast calls at all and
// nothing about WHOSE. So a manager holding forecast.create recorded a
// commitment against a rival team — attributed to that team, superseding its
// standing call, and unremovable, since no seat holds forecast.update or
// forecast.delete — and any forecast.read holder read back every owner's
// committed number and the note explaining it.
//
// The corpus is DERIVED three ways over, because the defect was one entry point
// looking exactly like its gated siblings:
//
//   - the scope-bearing TYPES are read off the package's own struct
//     declarations — anything with a `Scope` field — so a future NewProjection
//     carrying one is in the corpus without being named here;
//   - the ENTRY POINTS are the exported methods on the store whose parameters
//     name Scope or one of those types;
//   - and of those, the ones that WRITE, found by following the package's own
//     calls to a statement that inserts a forecast row.
//
// The write half is what makes the corpus right rather than merely large. A
// READ does not belong here: measuring a population and asserting a number for
// it are different questions, and the read one is answered once in the
// composition layer with a live-membership query this module cannot make. A
// census that demanded this gate on the readers too would be demanding a second,
// narrower copy of that rule — and it would refuse the team manager the resolver
// had just admitted to their teammate's figures.
//
// A gate that hard-codes part of its subject has become a second copy of it,
// and this one owns no list to go stale. What it does name is the two symbols
// it selects and certifies on, and it fails loudly rather than sweeping an
// empty corpus if either is renamed away.

import (
	"go/ast"
	"regexp"
	"sort"
	"testing"
)

const (
	// forecastingPackage is the module this census governs.
	forecastingPackage = "internal/modules/forecasting"

	// forecastStoreType is the store whose exported methods are the entry points.
	forecastStoreType = "Store"

	// forecastScopeType is the type that makes an entry point's subject a
	// particular team or owner rather than the installation.
	forecastScopeType = "Scope"

	// forecastScopeGate names the gate every such entry point owes: "may this
	// caller record against this scope".
	forecastScopeGate = "requireForecastScope"
)

func TestEveryForecastScopeWriterAsksWhoAnswersForTheScope(t *testing.T) {
	t.Parallel()
	files := parsePackageFiles(t, forecastingPackage)
	graph := packageCallGraph(t, forecastingPackage)

	if _, declared := graph[forecastScopeGate]; !declared {
		t.Fatalf("%s is not declared in %s — this census certifies on that name, so a rename "+
			"leaves it passing every entry point while none of them gates a scope",
			forecastScopeGate, forecastingPackage)
	}

	// The types that carry a scope: Scope itself, and every struct in the
	// package holding one. Read off the declarations so an input type added
	// later inherits the obligation instead of quietly escaping the corpus.
	carriers := map[string]bool{forecastScopeType: true}
	for _, file := range files {
		for _, decl := range file.Decls {
			generic, isGeneric := decl.(*ast.GenDecl)
			if !isGeneric {
				continue
			}
			for _, spec := range generic.Specs {
				typeSpec, isType := spec.(*ast.TypeSpec)
				if !isType {
					continue
				}
				structType, isStruct := typeSpec.Type.(*ast.StructType)
				if !isStruct {
					continue
				}
				for _, field := range structType.Fields.List {
					if ident, isIdent := field.Type.(*ast.Ident); isIdent && ident.Name == forecastScopeType {
						carriers[typeSpec.Name.Name] = true
					}
				}
			}
		}
	}
	if len(carriers) < 2 {
		t.Fatalf("only %s itself was found to carry a scope in %s — the walk over the package's "+
			"struct declarations found nothing, which is this census reporting PASS over a tree it cannot read",
			forecastScopeType, forecastingPackage)
	}

	var corpus, missing []string
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil || !fn.Name.IsExported() ||
				receiverTypeName(fn) != forecastStoreType || !namesACarrier(fn, carriers) {
				continue
			}
			name := scrubKey(forecastStoreType, fn.Name.Name)
			if !reachesAForecastInsert(graph, name, map[string]bool{}) {
				continue
			}
			corpus = append(corpus, name)
			if !reachesForecastScopeGate(graph, name, map[string]bool{}) {
				missing = append(missing, name)
			}
		}
	}
	if len(corpus) == 0 {
		t.Fatalf("no exported %s method in %s both names a scope-bearing type and reaches a forecast "+
			"INSERT — the corpus is empty, which is the one way a census fails without saying so",
			forecastStoreType, forecastingPackage)
	}
	sort.Strings(missing)
	for _, name := range missing {
		t.Errorf("%s records against a scope but never reaches %s: the object grant is its whole gate, "+
			"so a seat that may call a forecast at all may call it for every team and every owner",
			name, forecastScopeGate)
	}
}

// namesACarrier reports whether any parameter's type is one of the scope-bearing
// types — written flat rather than as a nested type walk because these
// signatures pass them by value or by pointer and nothing else.
func namesACarrier(fn *ast.FuncDecl, carriers map[string]bool) bool {
	if fn.Type.Params == nil {
		return false
	}
	for _, param := range fn.Type.Params.List {
		typ := param.Type
		if star, isStar := typ.(*ast.StarExpr); isStar {
			typ = star.X
		}
		if ident, isIdent := typ.(*ast.Ident); isIdent && carriers[ident.Name] {
			return true
		}
	}
	return false
}

// forecastInsert matches a statement that writes a forecast row. Matched against
// the statements the walk collects rather than against a function name, so a
// writer spelled differently tomorrow is still recognised as one.
var forecastInsert = regexp.MustCompile(`(?is)INSERT\s+INTO\s+forecast`)

// reachesAForecastInsert follows same-package calls to the statement itself,
// because the exported entry points do not hold their own SQL — RecordCall
// validates and delegates, and the INSERT lives a frame or two down.
func reachesAForecastInsert(graph map[string]*graphFunc, from string, seen map[string]bool) bool {
	if seen[from] {
		return false
	}
	seen[from] = true
	fn, known := graph[from]
	if !known {
		return false
	}
	for _, statement := range fn.statements {
		if forecastInsert.MatchString(statement) {
			return true
		}
	}
	for callee := range fn.calls {
		if reachesAForecastInsert(graph, callee, seen) {
			return true
		}
	}
	return false
}

// reachesForecastScopeGate follows same-package calls, so an entry point that
// discharges the obligation through a helper is certified by what the helper
// does rather than by where the words appear.
func reachesForecastScopeGate(graph map[string]*graphFunc, from string, seen map[string]bool) bool {
	if seen[from] {
		return false
	}
	seen[from] = true
	fn, known := graph[from]
	if !known {
		return false
	}
	for callee := range fn.calls {
		if callee == forecastScopeGate || reachesForecastScopeGate(graph, callee, seen) {
			return true
		}
	}
	return false
}
