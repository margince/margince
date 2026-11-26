// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// noOpProbeHelpers are the helpers that send a save of the stored values and
// check nothing moved, with where each takes its method and path.
var noOpProbeHelpers = map[string]bool{"assertSaveChangesNothing": true, "assertResendChangesNothing": true}

// probeSendsRoute reports that test, or a package function it calls, hands a
// probe helper a save with route's method and a path matching its pattern.
func probeSendsRoute(funcs map[string]*ast.FuncDecl, test, route string) bool {
	method, pattern, ok := strings.Cut(route, " ")
	if !ok {
		return false
	}
	seen := map[string]bool{}
	var walk func(name string) bool
	walk = func(name string) bool {
		fn := funcs[name]
		if fn == nil || seen[name] || fn.Body == nil {
			return false
		}
		seen[name] = true
		scope := pathBindings(fn.Body)
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall || found {
				return !found
			}
			callee, isIdent := call.Fun.(*ast.Ident)
			if !isIdent {
				return true
			}
			if noOpProbeHelpers[callee.Name] {
				sentMethod, path := probeArguments(call)
				found = sentMethod == method && anyPathMatches(renderPath(path, scope, 0), pattern)
				return !found
			}
			found = walk(callee.Name)
			return !found
		})
		return found
	}
	return walk(test)
}

// probeArguments reads the method and path a probe helper call sends: the
// positional pair of assertSaveChangesNothing, or the fields of a resend.
func probeArguments(call *ast.CallExpr) (string, ast.Expr) {
	if len(call.Args) >= 4 {
		method, err := stringLiteralOf(call.Args[2])
		if err == nil {
			return method, call.Args[3]
		}
	}
	if len(call.Args) < 3 {
		return "", nil
	}
	save, isLit := call.Args[2].(*ast.CompositeLit)
	if !isLit {
		return "", nil
	}
	var method string
	var path ast.Expr
	for _, element := range save.Elts {
		field, isField := element.(*ast.KeyValueExpr)
		if !isField {
			continue
		}
		switch key, _ := field.Key.(*ast.Ident); {
		case key == nil:
		case key.Name == "method":
			method, _ = stringLiteralOf(field.Value)
		case key.Name == "path":
			path = field.Value
		}
	}
	return method, path
}

// pathBindings maps each name the body assigns, or ranges over a list of
// string literals, to the expressions it may hold.
func pathBindings(body *ast.BlockStmt) map[string][]ast.Expr {
	scope := map[string][]ast.Expr{}
	ast.Inspect(body, func(n ast.Node) bool {
		switch stmt := n.(type) {
		case *ast.AssignStmt:
			if len(stmt.Lhs) == len(stmt.Rhs) {
				for i, lhs := range stmt.Lhs {
					if name, isIdent := lhs.(*ast.Ident); isIdent {
						scope[name.Name] = append(scope[name.Name], stmt.Rhs[i])
					}
				}
			}
		case *ast.RangeStmt:
			name, isIdent := stmt.Value.(*ast.Ident)
			list, isList := stmt.X.(*ast.CompositeLit)
			if isIdent && isList {
				scope[name.Name] = append(scope[name.Name], list.Elts...)
			}
		}
		return true
	})
	return scope
}

// renderPath spells the paths expr may build: literals as written, anything
// else as one unknown segment `{}`.
func renderPath(expr ast.Expr, scope map[string][]ast.Expr, depth int) []string {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if value, err := strconv.Unquote(e.Value); err == nil && e.Kind == token.STRING {
			return []string{value}
		}
	case *ast.ParenExpr:
		return renderPath(e.X, scope, depth)
	case *ast.BinaryExpr:
		if e.Op == token.ADD {
			var out []string
			for _, left := range renderPath(e.X, scope, depth) {
				for _, right := range renderPath(e.Y, scope, depth) {
					out = append(out, left+right)
				}
			}
			return out
		}
	case *ast.Ident:
		if bound := scope[e.Name]; len(bound) > 0 && depth < 4 {
			var out []string
			for _, each := range bound {
				out = append(out, renderPath(each, scope, depth+1)...)
			}
			return out
		}
	}
	return []string{"{}"}
}

// anyPathMatches reports a rendered path under /v1 whose segments match the
// route pattern: a literal segment by its text, a `{param}` segment by any.
func anyPathMatches(paths []string, pattern string) bool {
	want := strings.Split(pattern, "/")
	for _, path := range paths {
		got := strings.Split(strings.TrimPrefix(path, "/v1"), "/")
		if len(got) != len(want) {
			continue
		}
		matched := true
		for i := range want {
			if !strings.HasPrefix(want[i], "{") && want[i] != got[i] {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

// The census leans on the match. It must refuse a test that probes another
// route, and accept one that probes this route through a helper.
func TestTheProbeMatchRefusesATestThatProbesAnotherRoute(t *testing.T) {
	t.Parallel()
	const source = `package integration
func TestProbesStages(t *testing.T) {
	probeLadder(t)
}
func probeLadder(t *testing.T) {
	path := "/v1/stages/" + stage.ID
	assertResendChangesNothing(t, e, resend{method: "PATCH", path: path + "/exit-criteria/" + criterion.ID})
}
func TestProbesViews(t *testing.T) {
	for _, route := range []string{"/v1/views", "/v1/tags"} {
		assertSaveChangesNothing(t, e, "PATCH", route+"/"+id, nil, id, 1)
	}
}`
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	funcs := map[string]*ast.FuncDecl{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok {
			funcs[fn.Name.Name] = fn
		}
	}
	for _, c := range []struct {
		test, route string
		want        bool
	}{
		{"TestProbesStages", "PATCH /stages/{id}/exit-criteria/{criterion_id}", true},
		{"TestProbesStages", "PATCH /stages/{id}", false},
		{"TestProbesStages", "PUT /stages/{id}/exit-criteria/{criterion_id}", false},
		{"TestProbesViews", "PATCH /tags/{id}", true},
		{"TestProbesViews", "PATCH /roles/{key}", false},
	} {
		if got := probeSendsRoute(funcs, c.test, c.route); got != c.want {
			t.Errorf("%s probing %s = %t, want %t", c.test, c.route, got, c.want)
		}
	}
}
