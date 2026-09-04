// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/token"
	"path/filepath"
	"strconv"
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

	// Which package each import path actually declares, so an ALIAS cannot
	// hide an entry. `identitymod "…/identity"` makes a call read
	// identitymod.Country while the declaration is identity.Country, and a gate
	// comparing the two strings finds no match — which used to mean the entry
	// was logged and accepted, i.e. exactly the case this exists to catch
	// slipping through under a rename.
	//
	// Read from the parsed files rather than assumed from the path's last
	// segment, because those differ in this tree (contracts declares
	// crmcontracts).
	packageOfDir := map[string]string{}
	for _, path := range goFilePaths(t, "internal") {
		if file, ok := parsedByPath(files, fset, path); ok && file.Name != nil {
			packageOfDir[filepath.ToSlash(filepath.Dir(path))] = file.Name.Name
		}
	}

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
		aliases := aliasesOf(file, packageOfDir)
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
				if entry := entryName(node.Args[2], pkg, aliases); entry != "" {
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
			// A FAILURE, not a note. Aliases are resolved above, so an entry
			// this walk cannot place is one the gate genuinely cannot see —
			// and accepting it would mean the one shape that hides a missing
			// declaration is the one shape that passes.
			t.Errorf("%s reads %s through settings.ApplyTx and this gate cannot find its "+
				"declaration. Either the entry is declared outside internal/, or the walk "+
				"stopped recognising a declaration shape — both leave a MachineryApplied "+
				"omission unable to fail here, which is what this gate is for.", where, entry)
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

// entryName renders the entry argument as declaringPackage.Name.
//
// A bare identifier resolves against the file's own package; a qualified one
// resolves the QUALIFIER through the file's imports, so an alias names the
// package that actually declares the entry rather than whatever the importer
// chose to call it.
func entryName(expr ast.Expr, pkg string, aliases map[string]string) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return pkg + "." + e.Name
	case *ast.SelectorExpr:
		qualifier, ok := e.X.(*ast.Ident)
		if !ok {
			return ""
		}
		name := qualifier.Name
		if declared, found := aliases[name]; found {
			name = declared
		}
		return name + "." + e.Sel.Name
	}
	return ""
}

// aliasesOf maps each import qualifier in this file to the package name that
// import path actually declares.
func aliasesOf(file *ast.File, packageOfDir map[string]string) map[string]string {
	out := map[string]string{}
	for _, spec := range file.Imports {
		path, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		// The tree's own packages only. A third-party import can never be the
		// qualifier on a settings entry, and guessing its package name from a
		// module path is how a resolver starts inventing answers.
		declared, known := packageOfDir[strings.TrimPrefix(path, modulePath+"/")]
		if !known {
			continue
		}
		qualifier := declared
		if spec.Name != nil {
			qualifier = spec.Name.Name
		}
		out[qualifier] = declared
	}
	return out
}

// goFilePaths is goFilesUnder with the walk error turned into a test failure.
func goFilePaths(t *testing.T, dir string) []string {
	t.Helper()
	paths, err := goFilesUnder(dir)
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	return paths
}

// parsedByPath finds the already-parsed file for one path, so the walk is not
// repeated just to learn package names.
func parsedByPath(files []*ast.File, fset *token.FileSet, path string) (*ast.File, bool) {
	for _, file := range files {
		if fset.Position(file.Pos()).Filename == path {
			return file, true
		}
	}
	return nil, false
}
