// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"go/ast"
	"go/types"
	"sync"
)

type historyFieldSource struct {
	image historyImageArg
	key   bool
}
type (
	historyFieldSources map[*types.Var][]historyFieldSource
	historyFieldFactory func() historyFieldSources
)

var historyFieldSourceCache sync.Map

// Index the typed field objects once; each image then reads only its own writers.
func historyFieldsOf(index *auditIndex) historyFieldSources {
	factory := historyFieldFactory(sync.OnceValue(func() historyFieldSources {
		out := historyFieldSources{}
		for at := range index.pkgs {
			pkg := &index.pkgs[at]
			for _, file := range pkg.files {
				var declaration *ast.FuncDecl
				ast.Inspect(file, func(node ast.Node) bool {
					if fn, ok := node.(*ast.FuncDecl); ok {
						declaration = fn
					}
					image := historyImageArg{pkg: pkg, decl: declaration}
					if pair, ok := node.(*ast.KeyValueExpr); ok {
						if name, ok := pair.Key.(*ast.Ident); ok {
							if field, ok := pkg.info.Uses[name].(*types.Var); ok && field.IsField() {
								image.expr = pair.Value
								out[field] = append(out[field], historyFieldSource{image: image})
							}
						}
					}
					assignment, ok := node.(*ast.AssignStmt)
					if !ok {
						return true
					}
					for i, target := range assignment.Lhs {
						if indexed, ok := target.(*ast.IndexExpr); ok {
							if member, ok := indexed.X.(*ast.SelectorExpr); ok {
								if field, ok := pkg.info.Uses[member.Sel].(*types.Var); ok {
									image.expr = indexed.Index
									out[field] = append(out[field], historyFieldSource{image: image, key: true})
								}
							}
						}
						if member, ok := target.(*ast.SelectorExpr); ok && i < len(assignment.Rhs) {
							if field, ok := pkg.info.Uses[member.Sel].(*types.Var); ok {
								image.expr = assignment.Rhs[i]
								out[field] = append(out[field], historyFieldSource{image: image})
							}
						}
					}
					return true
				})
			}
		}
		return out
	}))
	actual, _ := historyFieldSourceCache.LoadOrStore(index, factory)
	return actual.(historyFieldFactory)()
}

// A map-valued struct member can be filled before another function receives it.
func (r historyImageReader) structFieldImages(in historyImageArg, selector *ast.SelectorExpr, depth int) ([]string, bool) {
	if depth > 32 {
		return nil, false
	}
	field, ok := in.pkg.info.Uses[selector.Sel].(*types.Var)
	if !ok {
		return nil, false
	}
	if r.fieldScanBusy == nil {
		r.fieldScanBusy = map[*types.Var]bool{}
	}
	if r.fieldScanBusy[field] {
		return nil, true
	}
	r.fieldScanBusy[field] = true
	defer delete(r.fieldScanBusy, field)
	sources := historyFieldsOf(r.index)[field]
	var keys []string
	known := len(sources) > 0
	for _, source := range sources {
		var got []string
		var ok bool
		if source.key {
			got, ok = r.strings(source.image, source.image.expr, 0)
		} else {
			got, ok = r.keys(source.image, 0)
		}
		keys = append(keys, got...)
		known = known && ok
	}
	return keys, known
}
