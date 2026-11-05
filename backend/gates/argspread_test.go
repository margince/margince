// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind prohibition H2

package gates

// A call never grows the slice it also spreads.
//
// Go evaluates the calls in one argument list left to right, but leaves open
// when a plain variable in that list is read. So in
// `tx.Query(ctx, fmt.Sprintf(q, arg(v)), args...)` the spread `args` may be
// read before `arg` appends to it, and the statement then names a placeholder
// with no value behind it. The query string is built in a statement first.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func TestNoCallGrowsTheSliceItSpreads(t *testing.T) {
	t.Parallel()
	type parsed struct {
		path string
		file *ast.File
	}
	var files []parsed
	eachGoFileUnder(t, []string{".", filepath.Join(repoRoot, "extensions")}, func(path string, file *ast.File) {
		files = append(files, parsed{path, file})
	})
	if len(files) == 0 {
		t.Fatal("no Go file found: the walk read nothing")
	}
	methods := map[string]map[string][]string{}
	for _, f := range files {
		dir := filepath.Dir(f.path)
		if methods[dir] == nil {
			methods[dir] = map[string][]string{}
		}
		collectAppendingMethods(f.file, methods[dir])
	}
	spreads := 0
	for _, f := range files {
		sites, seen := spreadGrowthSites(f.file, methods[filepath.Dir(f.path)])
		spreads += seen
		for _, site := range sites {
			t.Errorf("%s: an argument grows the slice this call spreads; build the query in a statement before the call",
				gatekit.SourceFileSet().Position(site))
		}
	}
	if spreads == 0 {
		t.Fatal("no call spreads a slice: the census read nothing")
	}
}

// TestTheSpreadGateSeesEveryWayToGrowTheSlice holds the census to the three
// shapes the tree uses, and to the hoisted form it must pass.
func TestTheSpreadGateSeesEveryWayToGrowTheSlice(t *testing.T) {
	t.Parallel()
	cases := map[string]struct {
		body string
		want int
	}{
		"closure call":     {`arg := func(v any) int { args = append(args, v); return len(args) }; q(fmt.Sprint(arg(1)), args...)`, 1},
		"closure as value": {`arg := func(v any) int { args = append(args, v); return len(args) }; q(clause(arg), args...)`, 1},
		"builder method":   {`q("x="+b.add(1), b.values...)`, 1},
		"direct append":    {`q(len(append(args, 1)), args...)`, 1},
		"hoisted":          {`arg := func(v any) int { args = append(args, v); return len(args) }; s := fmt.Sprint(arg(1)); q(s, args...)`, 0},
		"other slice":      {`arg := func(v any) int { args = append(args, v); return len(args) }; q(arg(1), other...)`, 0},
	}
	for name, c := range cases {
		src := "package p\nfunc (b *bindings) add(v any) string { b.values = append(b.values, v); return \"\" }\nfunc f() {\n" + c.body + "\n}\n"
		file, err := parser.ParseFile(token.NewFileSet(), name+".go", src, 0)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		methods := map[string][]string{}
		collectAppendingMethods(file, methods)
		if sites, _ := spreadGrowthSites(file, methods); len(sites) != c.want {
			t.Errorf("%s: found %d sites, want %d", name, len(sites), c.want)
		}
	}
}

// spreadGrowthSites answers the calls in file that grow the slice they spread,
// and how many spreading calls it read.
func spreadGrowthSites(file *ast.File, methods map[string][]string) ([]token.Pos, int) {
	var sites []token.Pos
	spreads := 0
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		closures := appendingClosures(fn.Body)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || !call.Ellipsis.IsValid() || len(call.Args) < 2 {
				return true
			}
			spreads++
			spread := types.ExprString(call.Args[len(call.Args)-1])
			for _, other := range call.Args[:len(call.Args)-1] {
				if grows(other, spread, closures, methods) {
					sites = append(sites, call.Pos())
					break
				}
			}
			return true
		})
	}
	return sites, spreads
}

// appendingClosures maps each closure bound to a name in body to the slices
// its own body appends to.
func appendingClosures(body *ast.BlockStmt) map[string][]string {
	out := map[string][]string{}
	bind := func(name *ast.Ident, value ast.Expr) {
		lit, ok := value.(*ast.FuncLit)
		if ok {
			out[name.Name] = appendTargets(lit.Body)
		}
	}
	ast.Inspect(body, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.AssignStmt:
			for i, lhs := range v.Lhs {
				if name, ok := lhs.(*ast.Ident); ok && i < len(v.Rhs) {
					bind(name, v.Rhs[i])
				}
			}
		case *ast.ValueSpec:
			for i, name := range v.Names {
				if i < len(v.Values) {
					bind(name, v.Values[i])
				}
			}
		}
		return true
	})
	return out
}

// collectAppendingMethods records, per method name, the receiver fields its
// body appends to, keyed without the receiver so `q.arg` meets `q.args`.
func collectAppendingMethods(file *ast.File, into map[string][]string) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || fn.Body == nil || len(fn.Recv.List[0].Names) == 0 {
			continue
		}
		recv := fn.Recv.List[0].Names[0].Name + "."
		for _, target := range appendTargets(fn.Body) {
			if field, ok := strings.CutPrefix(target, recv); ok {
				into[fn.Name.Name] = append(into[fn.Name.Name], field)
			}
		}
	}
}

// appendTargets are the expressions body assigns an append of themselves to.
func appendTargets(body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(n ast.Node) bool {
		assign, ok := n.(*ast.AssignStmt)
		if !ok {
			return true
		}
		for i, rhs := range assign.Rhs {
			if call, ok := rhs.(*ast.CallExpr); ok && isAppendTo(call, "") && i < len(assign.Lhs) {
				out = append(out, types.ExprString(assign.Lhs[i]))
			}
		}
		return true
	})
	return out
}

// grows reports whether expr appends to spread: directly, through a closure
// or method that does, or by handing such a closure to a callee.
func grows(expr ast.Expr, spread string, closures, methods map[string][]string) bool {
	found := false
	ast.Inspect(expr, func(n ast.Node) bool {
		switch v := n.(type) {
		case *ast.CallExpr:
			if isAppendTo(v, spread) {
				found = true
			}
			if sel, ok := v.Fun.(*ast.SelectorExpr); ok {
				for _, field := range methods[sel.Sel.Name] {
					if types.ExprString(sel.X)+"."+field == spread {
						found = true
					}
				}
			}
		case *ast.Ident:
			if slices.Contains(closures[v.Name], spread) {
				found = true
			}
		}
		return !found
	})
	return found
}

// isAppendTo reports an `append(target, …)` call; an empty target matches any.
func isAppendTo(call *ast.CallExpr, target string) bool {
	fn, ok := call.Fun.(*ast.Ident)
	if !ok || fn.Name != "append" || len(call.Args) == 0 {
		return false
	}
	return target == "" || types.ExprString(call.Args[0]) == target
}
