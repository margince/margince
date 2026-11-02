// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/token"
	"strconv"
)

func threadSQLReference(expr ast.Expr) string {
	switch node := expr.(type) {
	case *ast.Ident:
		return node.Name
	case *ast.SelectorExpr:
		if owner := threadSQLReference(node.X); owner != "" {
			return owner + "." + node.Sel.Name
		}
	}
	return ""
}

func (p threadSQLPackage) fieldValues(field *ast.SelectorExpr, bindings map[string][]ast.Expr, reached map[string]bool, context *ast.FuncDecl, depth int) []threadSQLValue {
	reference := threadSQLReference(field)
	key := threadBindingIdentity(context, reference)
	if !reached[key] {
		if values, ok := bindings[reference]; ok {
			var out []threadSQLValue
			for _, expr := range values {
				out = append(out, p.values(expr, bindings, cloneThreadSQLReach(reached, key), context, depth+1)...)
			}
			return out
		}
	}
	for _, expr := range bindings[threadSQLReference(field.X)] {
		literal, ok := expr.(*ast.CompositeLit)
		if !ok {
			if indexed, index := expr.(*ast.IndexExpr); index {
				key := "$threadIndexedField:" + reference
				scope := make(map[string][]ast.Expr, len(bindings)+1)
				for name, values := range bindings {
					scope[name] = values
				}
				scope[key] = threadRangeElements(indexed.X, bindings, map[string]bool{})
				if len(scope[key]) > 0 {
					return p.fieldValues(&ast.SelectorExpr{X: ast.NewIdent(key), Sel: field.Sel}, scope, reached, context, depth+1)
				}
			}
			continue
		}
		shape, _ := literal.Type.(*ast.StructType)
		if name, ok := literal.Type.(*ast.Ident); ok {
			shape = p.structs[name.Name]
		}
		if shape != nil {
			index := 0
			for _, slot := range shape.Fields.List {
				for _, name := range slot.Names {
					if name.Name == field.Sel.Name && index < len(literal.Elts) {
						if _, keyed := literal.Elts[index].(*ast.KeyValueExpr); !keyed {
							return p.values(literal.Elts[index], bindings, reached, context, depth+1)
						}
					}
					index++
				}
			}
		}
		for _, element := range literal.Elts {
			pair, ok := element.(*ast.KeyValueExpr)
			if !ok {
				continue
			}
			name, ok := pair.Key.(*ast.Ident)
			if ok && name.Name == field.Sel.Name {
				return p.values(pair.Value, bindings, reached, context, depth+1)
			}
		}
	}
	return []threadSQLValue{{text: "?"}}
}

func (p threadSQLPackage) callBindings(call *ast.CallExpr, callerBindings map[string][]ast.Expr, reached map[string]bool, caller, target *ast.FuncDecl, before token.Pos, depth int) map[string][]ast.Expr {
	bindings := map[string][]ast.Expr{}
	index := 0
	for _, field := range target.Type.Params.List {
		for _, name := range field.Names {
			if index >= len(call.Args) {
				return p.bindingsWith(target, before, bindings)
			}
			if _, assigned := bindings[name.Name]; !assigned {
				if _, literal := call.Args[index].(*ast.CompositeLit); literal {
					bindings[name.Name] = []ast.Expr{call.Args[index]}
					index++
					continue
				}
				if literal, ok := call.Args[index].(*ast.BasicLit); ok && literal.Kind != token.STRING {
					bindings[name.Name] = []ast.Expr{literal}
					index++
					continue
				}
				if literal, ok := call.Args[index].(*ast.Ident); ok && (literal.Name == "true" || literal.Name == "false" || literal.Name == "nil") {
					bindings[name.Name] = []ast.Expr{literal}
					index++
					continue
				}
				for _, value := range p.values(call.Args[index], callerBindings, reached, caller, depth+1) {
					bindings[name.Name] = append(bindings[name.Name], &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(value.text)})
				}
			}
			index++
		}
	}
	return p.bindingsWith(target, before, bindings)
}

func (p threadSQLPackage) recordBuilderWrite(bindings map[string][]ast.Expr, call *ast.CallExpr, context *ast.FuncDecl) {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || method.Sel.Name != "WriteString" || len(call.Args) != 1 {
		return
	}
	owner := threadSQLReference(method.X)
	if owner == "" {
		return
	}
	key := owner + ".String"
	previous := bindings[key]
	if len(previous) == 0 {
		previous = []ast.Expr{&ast.BasicLit{Kind: token.STRING, Value: `""`}}
	}
	var next []ast.Expr
	for _, value := range previous {
		next = append(next, &ast.BinaryExpr{X: value, Op: token.ADD, Y: call.Args[0]})
	}
	if threadConditionalBinding(context, call.Pos()) {
		next = append(next, previous...)
	}
	bindings[key] = next
}

func (p threadSQLPackage) builderValues(call *ast.CallExpr, bindings map[string][]ast.Expr, reached map[string]bool, context *ast.FuncDecl, depth int) ([]threadSQLValue, bool) {
	method, ok := call.Fun.(*ast.SelectorExpr)
	if !ok {
		return nil, false
	}
	key := threadSQLReference(method.X) + ".String"
	expressions, ok := bindings[key]
	if !ok {
		return nil, false
	}
	var out []threadSQLValue
	for _, expr := range expressions {
		out = append(out, p.values(expr, bindings, reached, context, depth+1)...)
	}
	return out, true
}

func threadRangeElements(expr ast.Expr, bindings map[string][]ast.Expr, reached map[string]bool) []ast.Expr {
	switch node := expr.(type) {
	case *ast.CompositeLit:
		var elements []ast.Expr
		for _, element := range node.Elts {
			if pair, ok := element.(*ast.KeyValueExpr); ok {
				elements = append(elements, pair.Value)
			} else {
				if item, ok := element.(*ast.CompositeLit); ok && item.Type == nil {
					if array, ok := node.Type.(*ast.ArrayType); ok {
						copy := *item
						copy.Type = array.Elt
						element = &copy
					}
				}
				elements = append(elements, element)
			}
		}
		return elements
	case *ast.Ident:
		if reached[node.Name] {
			return nil
		}
		next := cloneThreadSQLReach(reached, node.Name)
		var elements []ast.Expr
		for _, value := range bindings[node.Name] {
			elements = append(elements, threadRangeElements(value, bindings, next)...)
		}
		return elements
	}
	return nil
}

func threadBindingIdentity(context *ast.FuncDecl, name string) string {
	position := 0
	scope := ""
	if context != nil {
		position = int(context.Pos())
		scope = scrubKey(receiverTypeName(context), context.Name.Name)
	}
	return "binding:" + scope + ":" + strconv.Itoa(position) + ":" + name
}

func (p threadSQLPackage) indexValues(index *ast.IndexExpr, bindings map[string][]ast.Expr, reached map[string]bool, context *ast.FuncDecl, depth int) []threadSQLValue {
	expressions := threadRangeElements(index.X, bindings, map[string]bool{})
	if len(expressions) == 0 {
		return []threadSQLValue{{text: "?"}}
	}
	var values []threadSQLValue
	for _, expr := range expressions {
		values = append(values, p.values(expr, bindings, reached, context, depth+1)...)
	}
	return values
}
