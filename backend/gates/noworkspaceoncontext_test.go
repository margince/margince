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
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
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
func workspaceOnContextFindings(fset *token.FileSet, file *ast.File, waived func(name string) bool) []string {
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
			} else if readsAnIDFromAContext(x) && !waived(x.Name.Name) {
				report(x.Pos(), "func "+x.Name.Name+" takes a context and returns an id read from ctx.Value")
			}
		case *ast.TypeSpec:
			if isAContextKeyShape(x.Type) && namesACompany(x.Name.Name) && strings.HasSuffix(strings.ToLower(x.Name.Name), "key") {
				report(x.Pos(), "type "+x.Name.Name+" is a context key naming a company")
			}
		case *ast.ValueSpec:
			if typeName := types.ExprString(declaredTypeOf(x)); strings.HasSuffix(strings.ToLower(typeName), "contextkey") || strings.HasSuffix(strings.ToLower(typeName), "ctxkey") {
				for _, name := range x.Names {
					if namesACompany(name.Name) {
						report(name.Pos(), name.Name+" is a context key naming a company")
					}
				}
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
			if namesACompany(key) {
				report(x.Pos(), "a context value keyed "+key+" carries a workspace")
			}
		}
		return true
	})
	return findings
}

// contextIDReaders are the functions that legitimately read an id from a
// context: the trace ids of one operation. None of them names a company.
var contextIDReaders = gatekit.Waive(map[string]string{
	"CorrelationID":   "the trace id of one operation",
	"CausationEvent":  "the event that caused one operation",
	"AgentRunID":      "the agent run an operation belongs to",
	"SendingHuman":    "the human a send acts for",
	"ReleasedBy":      "the approver who released a staged change",
	"chatSeatOf":      "the seat a chat call acts for",
	"seatMailboxTx":   "the mailbox of a seat, read from a context the caller built",
	"sharedMailboxOf": "the shared mailbox a call acts on",
})

var companyWords = []string{"workspace", "company", "installation", "tenant"}

func namesACompany(text string) bool {
	lower := strings.ToLower(text)
	return slices.ContainsFunc(companyWords, func(word string) bool { return strings.Contains(lower, word) })
}

// isAContextKeyShape is the type a context key takes: an empty struct or an int.
func isAContextKeyShape(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.StructType:
		return t.Fields == nil || len(t.Fields.List) == 0
	case *ast.Ident:
		return t.Name == "int"
	}
	return false
}

func declaredTypeOf(spec *ast.ValueSpec) ast.Expr {
	if spec.Type == nil {
		return &ast.Ident{Name: ""}
	}
	return spec.Type
}

// readsAnIDFromAContext reports a function that takes a context.Context,
// returns an id and calls Value in its body, whatever it is named.
func readsAnIDFromAContext(fn *ast.FuncDecl) bool {
	if fn.Body == nil || fn.Type.Params == nil || fn.Type.Results == nil {
		return false
	}
	takes := slices.ContainsFunc(fn.Type.Params.List, func(p *ast.Field) bool { return types.ExprString(p.Type) == "context.Context" })
	returns := slices.ContainsFunc(fn.Type.Results.List, func(r *ast.Field) bool {
		t := types.ExprString(r.Type)
		return strings.HasPrefix(t, "ids.") && strings.HasSuffix(t, "ID") || t == "ids.UUID"
	})
	if !takes || !returns {
		return false
	}
	reads := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		if call, ok := n.(*ast.CallExpr); ok {
			if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Value" && len(call.Args) == 1 {
				reads = true
			}
		}
		return true
	})
	return reads
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
	defer contextIDReaders.AssertAllMatched(t)
	seen := 0
	for _, root := range []string{"internal", "cmd", "pkg", filepath.Join("..", "extensions")} {
		fset, files := parseGoFilesUnder(t, root)
		for _, file := range files {
			seen++
			for _, finding := range workspaceOnContextFindings(fset, file, func(name string) bool { return contextIDReaders.Waived(t, name) }) {
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
		{"a renamed key", `package p
import "context"
type companyKey struct{}
var _ = context.WithValue(context.Background(), companyKey{}, 1)`, 2},
		{"a renamed reader", `package p
import ("context"; "ids")
type installationKey struct{}
func CompanyFrom(ctx context.Context) ids.UUID { id, _ := ctx.Value(k).(ids.UUID); return id }`, 2},
		{"the trace id reader", `package p
import ("context"; "ids")
func CorrelationID(ctx context.Context) (ids.UUID, bool) { id, ok := ctx.Value(k).(ids.UUID); return id, ok }`, 0},
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
		if got := workspaceOnContextFindings(fset, file, func(name string) bool { return name == "CorrelationID" }); len(got) != tc.want {
			t.Errorf("%s: %d findings, want %d: %v", tc.name, len(got), tc.want, got)
		}
	}
}
