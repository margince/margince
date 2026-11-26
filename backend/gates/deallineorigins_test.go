// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/token"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type lineLockFlow struct {
	corpus    []*ast.File
	methods   map[string][]string
	functions map[string]*ast.FuncDecl
	files     map[string]*ast.File
	values    map[string]map[string]map[string]bool
	constants map[string]string
	locals    map[string]map[string]bool
	external  map[string]bool
	changed   bool
}

func lineLocksOf(files []*ast.File) *lineLockFlow {
	flow := &lineLockFlow{corpus: files, methods: map[string][]string{}, functions: map[string]*ast.FuncDecl{}, files: map[string]*ast.File{}, values: map[string]map[string]map[string]bool{}, constants: stringConstants(files), locals: map[string]map[string]bool{}, external: map[string]bool{}}
	for _, file := range files {
		functions, literals := lineFunctions(file)
		for _, fn := range functions {
			if fn.Body == nil {
				continue
			}
			name := scrubKey(receiverTypeName(fn), fn.Name.Name)
			flow.functions[name], flow.files[name] = fn, file
			if _, method, ok := strings.Cut(name, "."); ok {
				flow.methods[method] = append(flow.methods[method], name)
			}
			flow.values[name] = map[string]map[string]bool{}
			flow.locals[name] = declaredNames(fn)
			if ast.IsExported(fn.Name.Name) || literals[fn] {
				flow.exposed(name)
			}
		}
	}
	for flow.changed = true; flow.changed; {
		flow.changed = false
		for name, fn := range flow.functions {
			flow.readFunction(name, fn)
		}
	}
	return flow
}

func (f *lineLockFlow) add(fn, key string, tables map[string]bool) {
	if key == "_" || len(tables) == 0 {
		return
	}
	if f.values[fn][key] == nil {
		f.values[fn][key] = map[string]bool{}
	}
	for table := range tables {
		if !f.values[fn][key][table] {
			f.values[fn][key][table], f.changed = true, true
		}
	}
}

func (f *lineLockFlow) called(fn string, call *ast.CallExpr) string {
	name := localFunctionCall(f.functions[fn], call.Fun, f.locals[fn])
	if f.functions[name] != nil {
		return name
	}
	return ""
}

func (f *lineLockFlow) origins(fn string, expr ast.Expr, result int) map[string]bool {
	local := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if name, ok := node.(*ast.Ident); ok && f.locals[fn][name.Name] {
			local = true
		}
		return !local
	})
	if !local {
		if text, ok := tableArgText(expr, f.constants); ok {
			return map[string]bool{text: true}
		}
	}
	if call, ok := expr.(*ast.CallExpr); ok {
		const storekitPath = "github.com/margince/margince/backend/internal/platform/database/storekit"
		if len(call.Args) > 2 && (unshadowedPackageCall(f.files[fn], f.locals[fn], call.Fun, storekitPath, "LockRow") || unshadowedPackageCall(f.files[fn], f.locals[fn], call.Fun, storekitPath, "LockPair")) {
			return f.origins(fn, call.Args[2], 0)
		}
		if called := f.called(fn, call); called != "" {
			return f.values[called]["$"+strconv.Itoa(result)]
		}
		return map[string]bool{"?": true}
	}
	if address, ok := expr.(*ast.UnaryExpr); ok && address.Op == token.AND {
		return map[string]bool{"?": true}
	}
	if selector, ok := expr.(*ast.SelectorExpr); ok && f.origins(fn, selector.X, 0)["?"] {
		return map[string]bool{"?": true}
	}
	key := exprText(gatekit.SourceFileSet(), expr)
	if tables := f.values[fn][key]; len(tables) > 0 {
		return tables
	}
	return nil
}

func (f *lineLockFlow) bind(targetFn, target, sourceFn string, expr ast.Expr, result int) {
	f.add(targetFn, target, f.origins(sourceFn, expr, result))
	if literal, ok := expr.(*ast.CompositeLit); ok {
		for _, elt := range literal.Elts {
			pair, ok := elt.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			key, ok := pair.Key.(*ast.Ident)
			if ok {
				f.add(targetFn, target+"."+key.Name, f.origins(sourceFn, pair.Value, 0))
			}
		}
	}
	source := exprText(gatekit.SourceFileSet(), expr)
	if call, ok := expr.(*ast.CallExpr); ok {
		sourceFn = f.called(sourceFn, call)
		if sourceFn == "" {
			return
		}
		source = "$" + strconv.Itoa(result)
	}
	for key, tables := range f.values[sourceFn] {
		suffix, matches := strings.CutPrefix(key, source+".")
		if matches && !strings.Contains(suffix, ".") {
			f.add(targetFn, target+"."+suffix, tables)
		}
	}
}

func (f *lineLockFlow) readFunction(name string, fn *ast.FuncDecl) {
	called := map[ast.Expr]bool{}
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			called[call.Fun] = true
		}
		return true
	})
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		switch n := node.(type) {
		case *ast.Ident:
			if !called[n] && !f.locals[name][n.Name] && f.functions[n.Name] != nil {
				f.exposed(n.Name)
			}
		case *ast.SelectorExpr:
			if f.called(name, &ast.CallExpr{Fun: n}) == "" {
				for _, method := range f.methods[n.Sel.Name] {
					f.exposed(method)
				}
			}
			if !called[n] {
				if callee := f.called(name, &ast.CallExpr{Fun: n}); callee != "" {
					f.exposed(callee)
				}
			}
		case *ast.UnaryExpr:
			if n.Op == token.AND {
				f.add(name, exprText(gatekit.SourceFileSet(), n.X), map[string]bool{"?": true})
			}
		case *ast.AssignStmt:
			for i, lhs := range n.Lhs {
				if len(n.Rhs) == 1 {
					f.bind(name, exprText(gatekit.SourceFileSet(), lhs), name, n.Rhs[0], i)
				} else if i < len(n.Rhs) {
					f.bind(name, exprText(gatekit.SourceFileSet(), lhs), name, n.Rhs[i], 0)
				}
			}
		case *ast.ValueSpec:
			for i, lhs := range n.Names {
				if i < len(n.Values) {
					f.bind(name, lhs.Name, name, n.Values[i], 0)
				}
			}
		case *ast.ReturnStmt:
			for i, value := range n.Results {
				f.bind(name, "$"+strconv.Itoa(i), name, value, 0)
			}
		case *ast.CallExpr:
			called := f.called(name, n)
			if called == "" {
				return true
			}
			i := 0
			for _, field := range f.functions[called].Type.Params.List {
				for _, param := range field.Names {
					if i < len(n.Args) {
						f.bind(called, param.Name, name, n.Args[i], 0)
					}
					i++
				}
			}
		}
		return true
	})
}

func (f *lineLockFlow) patchTables(name string, call *ast.CallExpr) (map[string]bool, bool) {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || len(call.Args) < 3 {
		return nil, false
	}
	switch method.Sel.Name {
	case "ApplyLocked":
		return f.origins(name, call.Args[2], 0), true
	case "ApplyWithVersion", "ApplyGuarded", "ApplyGuardedIn":
		tables := f.origins(name, call.Args[2], 0)
		if len(tables) == 0 {
			tables = map[string]bool{"?": true}
		}
		return tables, true
	}
	return nil, false
}

type linePatchSite struct {
	name   string
	call   *ast.CallExpr
	tables map[string]bool
}

func (f *lineLockFlow) dealPatchSites(table string) []linePatchSite {
	var found []linePatchSite
	for name, fn := range f.functions {
		called := map[ast.Expr]bool{}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok {
				called[call.Fun] = true
				tables, patch := f.patchTables(name, call)
				if patch && (len(tables) == 0 || tables["?"] || tables[table]) {
					found = append(found, linePatchSite{name, call, tables})
				}
			}
			return true
		})
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			selector, ok := node.(*ast.SelectorExpr)
			if ok && !called[selector] {
				switch selector.Sel.Name {
				case "ApplyLocked", "ApplyGuarded", "ApplyGuardedIn", "ApplyWithVersion":
					found = append(found, linePatchSite{name, &ast.CallExpr{Fun: selector}, map[string]bool{"?": true}})
				}
			}
			return true
		})
	}
	return found
}

func (f *lineLockFlow) exposed(name string) {
	f.external[name] = true
	for _, field := range f.functions[name].Type.Params.List {
		for _, param := range field.Names {
			f.add(name, param.Name, map[string]bool{"?": true})
		}
	}
}

func unshadowedPackageCall(file *ast.File, locals map[string]bool, fun ast.Expr, path, method string) bool {
	selector, ok := fun.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	alias, ok := selector.X.(*ast.Ident)
	return ok && !locals[alias.Name] && callsPackageFunc(file, fun, path, method)
}

func lineFunctions(file *ast.File) ([]*ast.FuncDecl, map[*ast.FuncDecl]bool) {
	var functions []*ast.FuncDecl
	literals := map[*ast.FuncDecl]bool{}
	for _, declaration := range file.Decls {
		if fn, ok := declaration.(*ast.FuncDecl); ok {
			functions = append(functions, fn)
		}
		group, ok := declaration.(*ast.GenDecl)
		if !ok {
			continue
		}
		for _, spec := range group.Specs {
			values, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			for i, value := range values.Values {
				literal, ok := value.(*ast.FuncLit)
				if ok && i < len(values.Names) {
					fn := &ast.FuncDecl{Name: values.Names[i], Type: literal.Type, Body: literal.Body}
					functions = append(functions, fn)
					literals[fn] = true
				}
			}
		}
	}
	return functions, literals
}

func (f *lineLockFlow) productPatchSites(tables map[string]bool) []linePatchSite {
	var sites []linePatchSite
	for table := range tables {
		for _, site := range f.dealPatchSites(table) {
			if site.tables[table] {
				sites = append(sites, site)
			}
		}
	}
	return sites
}
