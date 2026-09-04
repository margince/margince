// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"strings"
	"testing"
)

// Every setting read through settings.ApplyTx is declared MachineryApplied.
//
// The store enforces this already — ApplyTx refuses an undeclared entry — but it
// refuses at RUNTIME, inside whatever machinery was applying the posture. That
// is the worst place to find out. installation.country landed read through
// ApplyTx and not declared (#3976), and the failure surfaced as every outbound
// send job dying with "installation.country is not declared MachineryApplied"
// — a dead send lane on main, discovered by a channel round-trip test timing
// out sixty seconds at a time rather than by anything naming the cause.
//
// A declaration and its reader are two lines in two files, and nothing but this
// held them together. Here they fail at build.
//
// WHAT IT DOES NOT CHECK, and cannot from the AST alone: whether the entry
// SHOULD be machinery-applied. That is a disclosure judgement — the flag lets
// machinery read a value ungated, so declaring one to silence this gate would
// be exactly the "never to skip a read gate for convenience" its own doc warns
// against. This asks only that the two agree; whether they agree on the right
// answer is the reviewer's.
func TestEverySettingReadThroughApplyIsDeclaredMachineryApplied(t *testing.T) {
	t.Parallel()

	fset, files := parseGoFilesUnder(t, "internal")

	// entry name -> declared MachineryApplied. Keyed by the VAR name because
	// that is what a call site names; the key string lives on the entry and is
	// not resolvable from the call.
	declared := map[string]bool{}
	// The reads, as entry name -> where, so a failure names the call site
	// rather than only the entry.
	read := map[string]string{}

	for _, file := range files {
		pkg := ""
		if file.Name != nil {
			pkg = file.Name.Name
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.ValueSpec:
				for i, name := range node.Names {
					if i < len(node.Values) && definesASetting(node.Values[i]) {
						declared[pkg+"."+name.Name] = hasMachineryApplied(node.Values[i])
					}
				}
			case *ast.CallExpr:
				if calleeName(node) != "ApplyTx" || len(node.Args) != 3 {
					return true
				}
				if entry := entryName(node.Args[2], pkg); entry != "" {
					read[entry] = fset.Position(node.Pos()).String()
				}
			}
			return true
		})
	}

	// The floor. This is a prohibition, so an empty walk reads exactly like a
	// clean tree — and the walk is the part most likely to break silently, since
	// it depends on Define staying a call and ApplyTx staying a three-argument
	// one.
	const readFloor = 4
	if len(read) < readFloor {
		t.Fatalf("found %d settings.ApplyTx call site(s), fewer than the %d this gate assumes — "+
			"the walk stopped matching rather than the tree stopping doing it", len(read), readFloor)
	}
	if len(declared) == 0 {
		t.Fatal("found no settings.Define declarations at all, so nothing below could have been judged")
	}

	for entry, where := range read {
		isDeclared, found := declared[entry]
		switch {
		case !found:
			// Not a failure: the call names an entry this walk did not resolve
			// — a different package's, most often. Reported so the gate's reach
			// is visible rather than silently narrower than it looks.
			t.Logf("%s: %s is read through ApplyTx and its declaration was not found in this walk", where, entry)
		case !isDeclared:
			t.Errorf("%s reads %s through settings.ApplyTx, which refuses an entry not declared "+
				"MachineryApplied — at runtime, inside the machinery. Declare it where it is defined, "+
				"or read it through Get/GetTx and its gate.", where, entry)
		}
	}
}

// definesASetting reports whether the expression is a settings.Define call,
// possibly wrapped in the builder methods an entry chains.
func definesASetting(expr ast.Expr) bool {
	for {
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			return false
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok {
			if pkg, ok := sel.X.(*ast.Ident); ok && pkg.Name == "settings" && strings.HasPrefix(sel.Sel.Name, "Define") {
				return true
			}
			// A builder method — unwrap and keep looking for the Define under it.
			expr = sel.X
			continue
		}
		// Define[T] renders its type argument as an IndexExpr around the
		// selector, so the generic form arrives here rather than above.
		if idx, ok := call.Fun.(*ast.IndexExpr); ok {
			expr = &ast.CallExpr{Fun: idx.X}
			continue
		}
		return false
	}
}

// hasMachineryApplied reports whether the chain carries the declaration.
func hasMachineryApplied(expr ast.Expr) bool {
	for {
		call, ok := expr.(*ast.CallExpr)
		if !ok {
			return false
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return false
		}
		if sel.Sel.Name == "MachineryApplied" {
			return true
		}
		expr = sel.X
	}
}

// entryName renders the entry argument as package.Name, resolving a bare
// identifier against the file's own package.
func entryName(expr ast.Expr, pkg string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return pkg + "." + e.Name
	case *ast.SelectorExpr:
		if qualifier, ok := e.X.(*ast.Ident); ok {
			return qualifier.Name + "." + e.Sel.Name
		}
	}
	return ""
}
