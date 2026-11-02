// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

func threadSQLPackageOf(files []*ast.File) threadSQLPackage {
	out := threadSQLPackage{bindings: map[string][]ast.Expr{}, functions: map[string]*ast.FuncDecl{}, imports: map[string]string{}, structs: map[string]*ast.StructType{}, parameterMemo: map[string]threadSQLSources{}}
	for _, file := range files {
		for _, spec := range file.Imports {
			module, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				continue
			}
			prefix := "github.com/margince/margince/backend/"

			name := path.Base(module)
			if spec.Name != nil {
				name = spec.Name.Name
			}
			out.imports[name] = strings.TrimPrefix(module, prefix)
		}
		for _, decl := range file.Decls {
			if group, ok := decl.(*ast.GenDecl); ok {
				for _, spec := range group.Specs {
					if named, ok := spec.(*ast.TypeSpec); ok {
						if shape, ok := named.Type.(*ast.StructType); ok {
							out.structs[named.Name.Name] = shape
						}
					}
				}
			}
			if fn, ok := decl.(*ast.FuncDecl); ok {
				out.functions[scrubKey(receiverTypeName(fn), fn.Name.Name)] = fn
				continue
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				binding, ok := n.(*ast.ValueSpec)
				if !ok {
					return true
				}
				for i, name := range binding.Names {
					if i < len(binding.Values) {
						out.bindings[name.Name] = append(out.bindings[name.Name], binding.Values[i])
					}
				}
				return false
			})
		}
	}
	return out
}

func (p threadSQLPackage) calledFunction(call *ast.CallExpr, caller *ast.FuncDecl) (*ast.FuncDecl, bool) {
	fun := call.Fun
	for {
		if generic, ok := fun.(*ast.IndexExpr); ok {
			fun = generic.X
			continue
		}
		if generic, ok := fun.(*ast.IndexListExpr); ok {
			fun = generic.X
			continue
		}
		break
	}
	switch target := fun.(type) {
	case *ast.Ident:
		fn, ok := p.functions[target.Name]
		return fn, ok
	case *ast.SelectorExpr:
		receiver, ok := target.X.(*ast.Ident)
		if !ok || caller == nil {
			return nil, false
		}
		if caller.Recv != nil {
			for _, field := range caller.Recv.List {
				for _, name := range field.Names {
					if name.Name == receiver.Name {
						fn, exists := p.functions[scrubKey(receiverTypeName(caller), target.Sel.Name)]
						return fn, exists
					}
				}
			}
		}
		for _, field := range caller.Type.Params.List {
			for _, name := range field.Names {
				if name.Name == receiver.Name {
					typ := field.Type
					if pointer, ok := typ.(*ast.StarExpr); ok {
						typ = pointer.X
					}
					if named, ok := typ.(*ast.Ident); ok {
						fn, exists := p.functions[scrubKey(named.Name, target.Sel.Name)]
						return fn, exists
					}
				}
			}
		}
	}
	return nil, false
}

func threadStatementSources(t *testing.T) []gatekit.ParsedFile {
	t.Helper()
	packages := map[string]bool{}
	var sources []gatekit.ParsedFile
	scope := activityReaderScope
	scope.Subject = threadFileHasActivitySQL
	for _, subject := range scope.Files(t) {
		directory := path.Dir(subject.Path)
		if packages[directory] {
			continue
		}
		packages[directory] = true
		paths, err := filepath.Glob(filepath.Join(directory, "*.go"))
		if err != nil {
			t.Fatal(err)
		}
		sort.Strings(paths)
		for _, name := range paths {
			if strings.HasSuffix(name, "_test.go") || strings.HasSuffix(name, "_gen.go") {
				continue
			}
			file, err := gatekit.ParseFile(name, 0)
			if err != nil {
				t.Fatal(err)
			}
			sources = append(sources, gatekit.ParsedFile{Path: filepath.ToSlash(name), File: file})
		}
	}
	return sources
}

func threadFileHasActivitySQL(name string, file *ast.File) bool {
	if readsTheAuditImage(name, file) {
		return true
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		expression, ok := node.(ast.Expr)
		if !ok {
			return true
		}
		switch expression.(type) {
		case *ast.BasicLit, *ast.BinaryExpr:
		default:
			return true
		}
		text, known := gatekit.StringExpr(expression, nil, gatekit.FoldTotal)
		if !known {
			return true
		}
		if len(threadActivityPositions(threadStatementTokens(text))) > 0 || strings.Contains(text, "activity_thread_available") {
			found = true
		}
		return !found
	})
	return found
}
