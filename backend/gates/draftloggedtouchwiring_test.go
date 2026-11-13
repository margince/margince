// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// draftEngines are the packages whose draft classifies a conversation through
// draftcore.ClassifyWithLogged, and so reads the newest logged touch.
var draftEngines = map[string]bool{"contactdraft": true, "accountdraft": true, "leaddraft": true}

// Every draft service the root compose package builds binds the logged-touch
// read. Without it the draft sees only the timeline page. A contact met once
// behind a page of newer activity is then written to as a stranger.
func TestEveryDraftServiceBindsTheLoggedTouchRead(t *testing.T) {
	t.Parallel()
	files, err := gatekit.ReadDir(composeRootDir)
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	built := 0
	for _, entry := range files {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, err := gatekit.ReadFile(filepath.Join(composeRootDir, name))
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, name, body, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			stmt, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, rhs := range stmt.Rhs {
				if engine := builtEngine(rhs); engine != "" {
					built++
					if !chainCalls(rhs, "WithLoggedTouches") {
						t.Errorf("%s: a %s service is built without WithLoggedTouches", fset.Position(stmt.Pos()), engine)
					}
				}
			}
			return true
		})
	}
	if built < len(draftEngines) {
		t.Fatalf("found %d draft services built in this package, want at least one per engine; the census is short", built)
	}
}

// builtEngine names the draft package whose NewService expr calls, or "".
func builtEngine(expr ast.Expr) string {
	var engine string
	ast.Inspect(expr, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "NewService" {
			if pkg, ok := sel.X.(*ast.Ident); ok && draftEngines[pkg.Name] {
				engine = pkg.Name
			}
		}
		return engine == ""
	})
	return engine
}

// chainCalls reports a method named method anywhere in expr's call chain.
func chainCalls(expr ast.Expr, method string) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == method {
			found = true
		}
		return !found
	})
	return found
}
