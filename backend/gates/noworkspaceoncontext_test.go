// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const ctxWorkspaceFloor = 1000

// workspaceOnContextFindings judges one production file. An installation has
// one company, which a database handle or an open transaction names. A
// workspace on the context would be a second answer that drifts when the
// company is replaced.
//
// It refuses a call of the removed principal accessors. It also refuses a
// function that reads or binds a workspace on a context, and a
// context.WithValue or ctx.Value whose key names a workspace.
func workspaceOnContextFindings(fset *token.FileSet, file *ast.File) []string {
	var findings []string
	report := func(pos token.Pos, what string) {
		findings = append(findings, fset.Position(pos).String()+": "+what+
			". Name the company through the database handle (DB.Workspace), the open transaction (storekit.Workspace) or database.ProvenWorkspace, never through the context.")
	}
	principalName := ""
	for _, spec := range file.Imports {
		if importPath, err := strconv.Unquote(spec.Path.Value); err == nil && importPath == principalPackage {
			principalName = "principal"
			if spec.Name != nil {
				principalName = spec.Name.Name
			}
		}
	}
	ast.Inspect(file, func(n ast.Node) bool {
		switch x := n.(type) {
		case *ast.SelectorExpr:
			if pkg, ok := x.X.(*ast.Ident); ok && principalName != "" && pkg.Name == principalName &&
				(x.Sel.Name == "WorkspaceID" || x.Sel.Name == "WithWorkspaceID") {
				report(x.Pos(), "principal."+x.Sel.Name+" carries a workspace on the context")
			}
		case *ast.FuncDecl:
			if (x.Name.Name == "WorkspaceID" || x.Name.Name == "WithWorkspaceID") && takesContextFirst(x) {
				report(x.Pos(), "func "+x.Name.Name+" reads or binds a workspace on a context")
			}
		case *ast.CallExpr:
			sel, ok := x.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			key := ""
			switch {
			case sel.Sel.Name == "WithValue" && len(x.Args) == 3:
				key = types.ExprString(x.Args[1])
			case sel.Sel.Name == "Value" && len(x.Args) == 1:
				key = types.ExprString(x.Args[0])
			}
			if strings.Contains(strings.ToLower(key), "workspace") {
				report(x.Pos(), "a context value keyed "+key+" carries a workspace")
			}
		}
		return true
	})
	return findings
}

func takesContextFirst(fn *ast.FuncDecl) bool {
	params := fn.Type.Params
	if params == nil || len(params.List) == 0 {
		return false
	}
	return types.ExprString(params.List[0].Type) == "context.Context"
}

// TestNoProductionCodeCarriesAWorkspaceOnTheContext keeps the per-request
// workspace out. The installation's company is read where it is needed, from a
// handle or a transaction that proves it live.
func TestNoProductionCodeCarriesAWorkspaceOnTheContext(t *testing.T) {
	t.Parallel()
	seen := 0
	for _, root := range []string{"internal", "cmd", "pkg", filepath.Join("..", "extensions")} {
		fset, files := parseGoFilesUnder(t, root)
		for _, file := range files {
			seen++
			for _, finding := range workspaceOnContextFindings(fset, file) {
				t.Error(finding)
			}
		}
	}
	if seen < ctxWorkspaceFloor {
		t.Fatalf("inspected only %d files, expected at least %d: the walker matched almost nothing and this gate would pass vacuously", seen, ctxWorkspaceFloor)
	}
}

// TestTheWorkspaceOnContextGateSeesTheSpellingsItMustRefuse feeds the gate source
// that goes around a literal principal.WorkspaceID match.
func TestTheWorkspaceOnContextGateSeesTheSpellingsItMustRefuse(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, source string
		want         int
	}{
		{"the removed reader", `package p
import "` + principalPackage + `"
var _, _ = principal.WorkspaceID(nil)`, 1},
		{"the removed binder through an alias", `package p
import pr "` + principalPackage + `"
var _ = pr.WithWorkspaceID(nil, nil)`, 1},
		{"a reader of its own", `package p
import "context"
func WorkspaceID(ctx context.Context) (string, bool) { return "", false }`, 1},
		{"a binder of its own", `package p
import "context"
func WithWorkspaceID(ctx context.Context, id string) context.Context { return ctx }`, 1},
		{"a context value named for a workspace", `package p
import "context"
var _ = context.WithValue(context.Background(), workspaceKey{}, 1)`, 1},
		{"a context read named for a workspace", `package p
import "context"
func f(ctx context.Context) any { return ctx.Value(workspaceKey) }`, 1},
		{"an unrelated context value", `package p
import "context"
var _ = context.WithValue(context.Background(), actorKey{}, 1)`, 0},
		{"a function of the same name that takes no context", `package p
func WorkspaceID(id string) string { return id }`, 0},
	}
	for _, tc := range cases {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, "x.go", tc.source, 0)
		if err != nil {
			t.Fatalf("%s: %v", tc.name, err)
		}
		if got := workspaceOnContextFindings(fset, file); len(got) != tc.want {
			t.Errorf("%s: %d findings, want %d: %v", tc.name, len(got), tc.want, got)
		}
	}
}
