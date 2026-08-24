// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package dealstatus

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// The stakeholder role strings are wire values compose/network puts on a
// coverage seat, and this package MATCHES on them. A second spelling does not
// fail to compile — it fails to match, so the opening move silently stops
// finding the champion on every deal, and a test that does not already know
// the right string cannot tell.
//
// WHAT THIS GATE CANNOT SEE, stated because it decides the gate's shape: two
// of the wire values are also ordinary English words, and `roleWords` renders
// "champion" as the word "champion". A literal alone therefore does not say
// whether the author meant the wire value or the prose. So the gate reads
// COMPARISONS and switch cases — the places a mistyped wire value silently
// stops matching — and deliberately not every string in the package. A
// display string that happens to equal a role is not the defect; a `==` or a
// `case` against a bare literal is.
//
// A parse rather than a grep for the same reason: a grep counts the const
// declarations and the words inside these comments.
func TestTheRoleVocabularyIsSpelledOnce(t *testing.T) {
	declared := map[string]string{
		"champion":       "roleChampion",
		"economic_buyer": "roleEconomicBuyer",
		"decision_maker": "roleDecisionMaker",
		"influencer":     "roleInfluencer",
	}

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	fset := token.NewFileSet()
	offenders := 0
	scanned := 0
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		scanned++
		ast.Inspect(file, func(n ast.Node) bool {
			for _, lit := range comparedLiterals(n) {
				value, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				if constName, governed := declared[value]; governed {
					offenders++
					t.Errorf("%s compares against the role %q as a bare literal; use %s, which is the one spelling this package matches on",
						fset.Position(lit.Pos()), value, constName)
				}
			}
			return true
		})
	}
	if scanned == 0 {
		t.Fatal("scanned no files, so this gate would pass over an empty package")
	}
	if offenders > 0 {
		t.Logf("scanned %d non-test file(s) in the package", scanned)
	}
}

// comparedLiterals is the string literals a node TESTS a value against: the
// operands of == and !=, and the values of a switch case. Those are the sites
// where a mistyped role silently stops matching. A literal being returned,
// assigned or printed is not one of them.
func comparedLiterals(n ast.Node) []*ast.BasicLit {
	var out []*ast.BasicLit
	add := func(e ast.Expr) {
		if lit, ok := e.(*ast.BasicLit); ok && lit.Kind == token.STRING {
			out = append(out, lit)
		}
	}
	switch node := n.(type) {
	case *ast.BinaryExpr:
		if node.Op == token.EQL || node.Op == token.NEQ {
			add(node.X)
			add(node.Y)
		}
	case *ast.CaseClause:
		for _, e := range node.List {
			add(e)
		}
	}
	return out
}
