// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"sort"
)

func threadForwardsCatalog(caller *ast.FuncDecl, arguments []ast.Expr, declarations map[string]threadProducerDeclaration) bool {
	parameters := map[string]bool{}
	for _, field := range caller.Type.Params.List {
		if !threadCatalogType(field.Type, declarations, map[string]bool{}) {
			continue
		}
		for _, name := range field.Names {
			parameters[name.Name] = true
		}
	}
	forwarded := false
	for _, argument := range arguments {
		ast.Inspect(argument, func(node ast.Node) bool {
			if name, ok := node.(*ast.Ident); ok && parameters[name.Name] {
				forwarded = true
			}
			return !forwarded
		})
	}
	return forwarded
}

func threadCatalogType(expression ast.Expr, declarations map[string]threadProducerDeclaration, seen map[string]bool) bool {
	switch expression := expression.(type) {
	case *ast.Ident:
		if expression.Name == "string" {
			return true
		}
		if seen[expression.Name] {
			return false
		}
		declaration, exists := declarations[expression.Name]
		shape, typed := declaration.node.(*ast.TypeSpec)
		if !exists || !typed {
			return false
		}
		return threadCatalogType(shape.Type, declarations, cloneThreadSQLReach(seen, expression.Name))
	case *ast.StarExpr:
		return threadCatalogType(expression.X, declarations, seen)
	case *ast.IndexExpr:
		return threadCatalogType(expression.X, declarations, seen)
	case *ast.IndexListExpr:
		return threadCatalogType(expression.X, declarations, seen)
	case *ast.ArrayType:
		return threadCatalogType(expression.Elt, declarations, seen)
	case *ast.MapType:
		return threadCatalogType(expression.Value, declarations, seen)
	case *ast.FuncType:
		if expression.Results == nil {
			return false
		}
		for _, field := range expression.Results.List {
			if threadCatalogType(field.Type, declarations, seen) {
				return true
			}
		}
	case *ast.StructType:
		for _, field := range expression.Fields.List {
			if threadCatalogType(field.Type, declarations, seen) {
				return true
			}
		}
	case *ast.InterfaceType:
		for _, field := range expression.Methods.List {
			if threadCatalogType(field.Type, declarations, seen) {
				return true
			}
		}

	}
	return false
}

func (index *threadProducerIndex) catalogArguments(directory, requested string, call *ast.CallExpr) []ast.Expr {
	slots := index.sqlParameterSlots(directory, requested, map[string]bool{})
	var arguments []ast.Expr
	for position, argument := range call.Args {
		if slots[position] {
			arguments = append(arguments, argument)
		}
	}
	return arguments
}

func (index *threadProducerIndex) sqlParameterSlots(directory, requested string, _ map[string]bool) map[int]bool {
	if index.sqlSlots == nil {
		index.sqlSlots = map[string]map[int]bool{}
	}
	if index.sqlSlotPackages == nil {
		index.sqlSlotPackages = map[string]bool{}
	}
	if !index.sqlSlotPackages[directory] {
		index.sqlSlotPackages[directory] = true
		declarations := index.declarations(directory)
		names := make([]string, 0, len(declarations))
		for name, declaration := range declarations {
			if _, ok := declaration.node.(*ast.FuncDecl); ok {
				names = append(names, name)
				index.sqlSlots[directory+":"+name] = map[int]bool{}
			}
		}
		sort.Strings(names)
		changed := true
		for changed {
			changed = false
			for _, name := range names {
				for position := range index.deriveSQLParameterSlots(directory, name) {
					slots := index.sqlSlots[directory+":"+name]
					if !slots[position] {
						slots[position], changed = true, true
					}
				}
			}
		}
	}
	return index.sqlSlots[directory+":"+requested]
}

func (index *threadProducerIndex) deriveSQLParameterSlots(directory, requested string) map[int]bool {
	declarations := index.declarations(directory)
	declaration := declarations[requested]
	target, ok := declaration.node.(*ast.FuncDecl)
	if !ok {
		return nil
	}
	positions := map[string]int{}
	for position, name := range threadParameterNames(target) {
		positions[name] = position
	}
	slots := map[int]bool{}
	functions := map[string]*ast.FuncDecl{}
	for name, declaration := range declarations {
		if fn, ok := declaration.node.(*ast.FuncDecl); ok {
			functions[name] = fn
		}
	}
	pkg := threadSQLPackage{functions: functions}
	ast.Inspect(target.Body, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok {
			return true
		}
		var sources []ast.Expr
		if slot, direct := threadDirectQueryArgument(call); direct && len(call.Args) > slot {
			sources = []ast.Expr{call.Args[slot]}
		} else if callee, local := pkg.calledFunction(call, target); local {
			for position := range index.sqlSlots[directory+":"+scrubKey(receiverTypeName(callee), callee.Name.Name)] {
				if position < len(call.Args) {
					sources = append(sources, call.Args[position])
				}
			}
		}
		if len(sources) == 0 {
			return true
		}
		for _, source := range index.argumentSources(directory, target, &ast.CallExpr{Fun: call.Fun, Args: sources}) {
			ast.Inspect(source, func(node ast.Node) bool {
				if name, ok := node.(*ast.Ident); ok {
					if position, parameter := positions[name.Name]; parameter {
						slots[position] = true
					}
				}
				return true
			})
		}
		return true
	})
	return slots
}
