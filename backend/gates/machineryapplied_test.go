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

	// The call sites whose entry expression this walk could not name at all.
	// Kept rather than skipped: an entry the walk drops is one it agrees with,
	// while ApplyTx refuses it at runtime — which is the defect this gate is
	// for, one level up.
	var unreadable []string

	for _, file := range files {
		pkg := ""
		if file.Name != nil {
			pkg = file.Name.Name
		}
		// The file's own import aliases, so a qualified entry resolves to the
		// package that DECLARED it rather than to whatever this file happens to
		// call that package. An alias is not exotic — the tree carries several —
		// and an unresolved qualifier would land in the "declaration not found"
		// arm below, which is precisely where a false negative could hide.
		aliases := importAliases(file)
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
				entry := entryName(node.Args[2], pkg, aliases)
				if entry == "" {
					unreadable = append(unreadable, fset.Position(node.Pos()).String())
					return true
				}
				read[entry] = fset.Position(node.Pos()).String()
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

	for _, where := range unreadable {
		t.Errorf("%s: this gate cannot name the entry passed to settings.ApplyTx here, so it cannot "+
			"check that entry's declaration — and an entry it cannot check is one it agrees with, "+
			"while ApplyTx refuses an undeclared one at runtime. Pass the entry as a plain "+
			"identifier or a qualified one.", where)
	}

	for entry, where := range read {
		isDeclared, found := declared[entry]
		switch {
		case !found:
			// A FAILURE, not a note. This walk covers all of internal/, which is
			// where every settings.Define lives, so an entry whose declaration
			// it cannot find is one it did not resolve rather than one declared
			// out of reach — and an unresolved entry is exactly the shape a
			// false negative takes here: reported as "not found" while its
			// machineryApplied is false and the send lane dies.
			t.Errorf("%s reads %s through settings.ApplyTx and this walk found no settings.Define "+
				"for it. Either the entry is declared outside internal/ — in which case this gate's "+
				"reach is wrong and should say so — or the name was not resolved, and an entry this "+
				"gate cannot resolve is one it silently passes.", where, entry)
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
func entryName(expr ast.Expr, pkg string, aliases map[string]string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return pkg + "." + e.Name
	case *ast.SelectorExpr:
		if qualifier, ok := e.X.(*ast.Ident); ok {
			// Through the file's aliases, so `idp "…/identity"` names identity
			// and not idp. An unaliased import maps to itself, so the ordinary
			// case is unchanged.
			if declaring, aliased := aliases[qualifier.Name]; aliased {
				return declaring + "." + e.Sel.Name
			}
			return qualifier.Name + "." + e.Sel.Name
		}
	}
	return ""
}

// importAliases maps the names a file refers to its imports BY onto the package
// names those imports actually declare.
//
// The last path segment is the package name in this tree — the module layout
// keeps the two the same, and a package whose name differed from its directory
// would be caught by the "declaration not found" arm rather than passed.
func importAliases(file *ast.File) map[string]string {
	out := map[string]string{}
	for _, spec := range file.Imports {
		if spec.Name == nil || spec.Name.Name == "_" || spec.Name.Name == "." {
			continue
		}
		path := strings.Trim(spec.Path.Value, `"`)
		if at := strings.LastIndex(path, "/"); at >= 0 {
			path = path[at+1:]
		}
		out[spec.Name.Name] = path
	}
	return out
}
