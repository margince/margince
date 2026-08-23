// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Every path that mints an organization runs the dedupe ladder first.
//
// `createOrganization` is the one INSERT INTO organization, and it takes the
// match as an argument rather than computing it — which makes the agreement
// checkable but does not enforce it: a caller may pass a zero OrganizationMatch
// and mint a twin nobody detected. That is not hypothetical. The CSV import was
// believed for most of a day to have bypassed this seam entirely, and settling
// the question took reading four call sites by hand.
//
// So the rule is held here instead of in a comment: a caller of the one INSERT
// must obtain its match from the ladder, in the same function. CLAUDE.md's own
// reuse rule is that a uniqueness claim without a test is worth nothing.
func TestEveryOrganizationCreatePathRunsTheDedupeLadder(t *testing.T) {
	const (
		theInsert = "createOrganization"
		ladderTx  = "DedupeOrganizationForCreate"
	)

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the package directory: %v", err)
	}
	fset := token.NewFileSet()
	files := map[string]*ast.File{}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		files[name] = parsed
	}

	{
		for path, file := range files {
			for _, decl := range file.Decls {
				fn, ok := decl.(*ast.FuncDecl)
				if !ok || fn.Body == nil || fn.Name.Name == theInsert {
					continue
				}
				var mentionsInsert, callsLadder, escapes bool
				ast.Inspect(fn.Body, func(n ast.Node) bool {
					call, ok := n.(*ast.CallExpr)
					if ok {
						if name := calleeName(call.Fun); name == theInsert {
							mentionsInsert = true
						} else if strings.Contains(name, "Dedupe") {
							// The ladder, or one of the wrappers whose whole
							// job is to run it (manualDedupeOrganization).
							callsLadder = true
						}
						// The insert named anywhere OTHER than as the callee —
						// passed as a value, assigned, taken as a method value
						// — leaves the reach of this check. Refused rather than
						// ignored: a seam nobody can verify is not a seam.
						for _, arg := range call.Args {
							if id, ok := arg.(*ast.Ident); ok && id.Name == theInsert {
								escapes = true
							}
						}
						return true
					}
					if assign, ok := n.(*ast.AssignStmt); ok {
						for _, rhs := range assign.Rhs {
							if id, ok := rhs.(*ast.Ident); ok && id.Name == theInsert {
								escapes = true
							}
						}
					}
					return true
				})
				if escapes {
					t.Errorf("%s: %s takes %s as a VALUE rather than calling it; "+
						"this check follows direct calls only, so a reference that escapes it "+
						"must not exist — call the insert directly, beside its ladder",
						path, fn.Name.Name, theInsert)
				}
				if mentionsInsert && !callsLadder {
					t.Errorf("%s: %s calls %s without running the dedupe ladder in the same function; "+
						"every path that mints an organization must obtain its match from %s",
						path, fn.Name.Name, theInsert, ladderTx)
				}
			}
		}
	}
}

// calleeName is the identifier a call expression invokes, bare or selected.
// Anything else — a call through a value, an immediately-invoked literal —
// answers empty, and the escape check above is what refuses those.
func calleeName(fun ast.Expr) string {
	switch f := fun.(type) {
	case *ast.Ident:
		return f.Name
	case *ast.SelectorExpr:
		return f.Sel.Name
	default:
		return ""
	}
}
