// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/token"
	"sort"
	"strconv"
	"strings"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type threadSQLSources struct{ known, activity, opaque bool }

func (a threadSQLSources) with(b threadSQLSources) threadSQLSources {
	return threadSQLSources{known: a.known || b.known, activity: a.activity || b.activity, opaque: a.opaque || b.opaque}
}

func (p threadSQLPackage) nonActivitySQL(expr ast.Expr, bindings map[string][]ast.Expr, context *ast.FuncDecl) bool {
	sources := p.sqlSources(expr, bindings, context, map[string]bool{}, 0)
	return sources.known && !sources.activity && !sources.opaque
}

func (p threadSQLPackage) sqlSources(expr ast.Expr, bindings map[string][]ast.Expr, context *ast.FuncDecl, reached map[string]bool, depth int) threadSQLSources {
	if depth > threadSQLMaxDepth {
		return threadSQLSources{opaque: true}
	}
	switch node := expr.(type) {
	case *ast.BasicLit:
		text, ok := gatekit.LiteralText(node)
		if !ok {
			return threadSQLSources{}
		}
		return threadSQLSources{known: looksLikeSQL(stripNoise(text)), activity: len(threadActivityPositions(threadStatementTokens(text))) > 0 || text == "activity"}
	case *ast.ParenExpr:
		return p.sqlSources(node.X, bindings, context, reached, depth+1)
	case *ast.BinaryExpr:
		return p.sqlSources(node.X, bindings, context, reached, depth+1).with(p.sqlSources(node.Y, bindings, context, reached, depth+1))
	case *ast.Ident:
		if reached[node.Name] {
			return threadSQLSources{opaque: true}
		}
		values, ok := bindings[node.Name]
		if !ok {
			return p.parameterSQLSources(node.Name, context, reached, depth)
		}
		sources := threadSQLSources{}
		for _, value := range values {
			sources = sources.with(p.sqlSources(value, bindings, context, cloneThreadSQLReach(reached, node.Name), depth+1))
		}
		return sources
	case *ast.IndexExpr:
		return p.sqlSources(node.X, bindings, context, reached, depth+1)
	case *ast.CompositeLit:
		sources := threadSQLSources{}
		for _, element := range node.Elts {
			if pair, ok := element.(*ast.KeyValueExpr); ok {
				sources = sources.with(p.sqlSources(pair.Value, bindings, context, reached, depth+1))
			} else {
				sources = sources.with(p.sqlSources(element, bindings, context, reached, depth+1))
			}
		}
		return sources
	case *ast.SelectorExpr:
		if expressions, ok := bindings[threadSQLReference(node)]; ok {
			sources := threadSQLSources{}
			for _, value := range expressions {
				sources = sources.with(p.sqlSources(value, bindings, context, reached, depth+1))
			}
			return sources
		}
		return p.sqlSources(node.X, bindings, context, reached, depth+1)
	case *ast.CallExpr:
		if sources, ok := p.closedImportedSQLSources(node, bindings, context, reached, depth); ok {
			return sources
		}
		if method, ok := node.Fun.(*ast.SelectorExpr); ok {
			if owner, ok := method.X.(*ast.Ident); ok && (owner.Name == "fmt" && method.Sel.Name == "Sprintf" || owner.Name == "strings" && method.Sel.Name == "Join") {
				sources := threadSQLSources{}
				for _, argument := range node.Args {
					sources = sources.with(p.sqlSources(argument, bindings, context, reached, depth+1))
				}
				return sources
			}
		}
		if target, ok := p.calledFunction(node, context); ok {
			return p.returnSources(node, target, bindings, context, reached, depth)
		}
		if method, ok := node.Fun.(*ast.SelectorExpr); ok {
			receiver := p.sqlSources(method.X, bindings, context, reached, depth+1)
			if receiver.known && !receiver.activity && !receiver.opaque {
				sources := receiver
				found := false
				for _, target := range p.functions {
					if target.Recv != nil && target.Name.Name == method.Sel.Name {
						sources = sources.with(p.returnSources(node, target, bindings, context, reached, depth))
						found = true
					}
				}
				if found {
					return sources
				}
			}
		}
	}
	return threadSQLSources{opaque: true}
}

func (p threadSQLPackage) returnSources(call *ast.CallExpr, target *ast.FuncDecl, bindings map[string][]ast.Expr, context *ast.FuncDecl, reached map[string]bool, depth int) threadSQLSources {
	key := p.directory + ":" + scrubKey(receiverTypeName(target), target.Name.Name)
	if reached[key] {
		return threadSQLSources{opaque: true}
	}
	next := cloneThreadSQLReach(reached, key)
	sources := threadSQLSources{}
	ast.Inspect(target.Body, func(node ast.Node) bool {
		if _, closure := node.(*ast.FuncLit); closure {
			return false
		}
		if literal, ok := node.(*ast.BasicLit); ok {
			if text, stringValue := gatekit.LiteralText(literal); stringValue && (text == "activity" || len(threadActivityPositions(threadStatementTokens(text))) > 0) {
				sources = sources.with(threadSQLSources{known: true, activity: true})
			}
		}
		result, ok := node.(*ast.ReturnStmt)
		if !ok {
			return true
		}
		if len(result.Results) == 0 {
			return false
		}
		scope := p.bindingsBefore(target, result.Pos())
		if method, ok := call.Fun.(*ast.SelectorExpr); ok && target.Recv != nil {
			for _, field := range target.Recv.List {
				for _, name := range field.Names {
					scope[name.Name] = []ast.Expr{method.X}
				}
			}
			for name, values := range bindings {
				if _, exists := scope[name]; !exists {
					scope[name] = values
				}
			}
		}
		params := threadParameterNames(target)
		for i, param := range params {
			if i < len(call.Args) {
				scope[param] = []ast.Expr{call.Args[i]}
			}
		}
		sources = sources.with(p.sqlSources(result.Results[0], scope, target, next, depth+1))
		return false
	})
	return sources
}

func threadPureQueryTransport(fn *ast.FuncDecl) bool {
	if fn.Recv == nil || !strings.EqualFold(fn.Name.Name, "Query") && !strings.EqualFold(fn.Name.Name, "QueryRow") && !strings.EqualFold(fn.Name.Name, "Exec") {
		return false
	}
	slot, forwarded := threadWrapperParameter(fn)
	if !forwarded {
		return false
	}
	hasSQL := false
	parameter := threadParameterNames(fn)[slot]
	ast.Inspect(fn.Body, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok {
			if index, isQuery := threadDirectQueryArgument(call); isQuery {
				if len(call.Args) <= index {
					hasSQL = true
				} else if name, ident := call.Args[index].(*ast.Ident); !ident || name.Name != parameter {
					hasSQL = true
				}
			}
		}
		if assignment, ok := node.(*ast.AssignStmt); ok {
			for _, left := range assignment.Lhs {
				if name, ident := left.(*ast.Ident); ident && name.Name == parameter {
					hasSQL = true
				}
			}
		}
		if literal, ok := node.(*ast.BasicLit); ok {
			if text, stringValue := gatekit.LiteralText(literal); stringValue && looksLikeSQL(stripNoise(text)) {
				hasSQL = true
			}
		}
		return !hasSQL
	})
	return !hasSQL
}

func (p threadSQLPackage) closedImportedSQLSources(call *ast.CallExpr, bindings map[string][]ast.Expr, context *ast.FuncDecl, reached map[string]bool, depth int) (threadSQLSources, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || p.resolvePackage == nil {
		return threadSQLSources{}, false
	}
	owner, ok := selector.X.(*ast.Ident)
	if !ok {
		return threadSQLSources{}, false
	}
	directory, ok := p.imports[owner.Name]
	if !ok || !strings.HasPrefix(directory, "internal/") {
		return threadSQLSources{}, false
	}
	targetPackage := p.resolvePackage(directory)
	target, ok := targetPackage.functions[selector.Sel.Name]
	if !ok {
		return threadSQLSources{}, false
	}
	key := directory + ":" + selector.Sel.Name
	if reached[key] || depth > threadSQLMaxDepth {
		return threadSQLSources{opaque: true}, true
	}
	args := make([]ast.Expr, len(call.Args))
	for i, argument := range call.Args {
		values := p.values(argument, bindings, reached, context, depth+1)
		if len(values) != 1 || values[0].unresolved || threadStructuralUnknown(values[0].text) {
			args[i] = ast.NewIdent("unresolvedSQLArgument")
			continue
		}
		args[i] = &ast.BasicLit{Kind: token.STRING, Value: strconv.Quote(values[0].text)}
	}
	substituted := *call
	substituted.Args = args
	return targetPackage.returnSources(&substituted, target, nil, nil, reached, depth+1), true
}

func (p threadSQLPackage) parameterSQLSources(name string, target *ast.FuncDecl, reached map[string]bool, depth int) (sources threadSQLSources) {
	if target == nil {
		return threadSQLSources{opaque: true}
	}
	declarations := map[string]threadProducerDeclaration{}
	for name, shape := range p.structs {
		declarations[name] = threadProducerDeclaration{node: &ast.TypeSpec{Name: ast.NewIdent(name), Type: shape}}
	}
	position, ordinal := -1, 0
	for _, field := range target.Type.Params.List {
		for _, parameter := range field.Names {
			if name == parameter.Name && threadCatalogType(field.Type, declarations, map[string]bool{}) {
				position = ordinal
			}
			ordinal++
		}
	}
	key := scrubKey(receiverTypeName(target), target.Name.Name)
	if position < 0 || reached[key+":"+name] {
		return threadSQLSources{opaque: true}
	}
	// Every field of one parameter asks for its callers again, so the answer is
	// kept per reach. Depth is left out: reach already ends every cycle.
	memoKey := key + ":" + name + ":" + threadSQLReachKey(reached)
	if sources, ok := p.parameterMemo[memoKey]; ok {
		return sources
	}
	defer func() { p.parameterMemo[memoKey] = sources }()
	next := cloneThreadSQLReach(reached, key+":"+name)
	found := false
	for _, caller := range p.functions {
		ast.Inspect(caller.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok || len(call.Args) <= position {
				return true
			}
			callee, ok := p.calledFunction(call, caller)
			if !ok || scrubKey(receiverTypeName(callee), callee.Name.Name) != key {
				return true
			}
			found = true
			sources = sources.with(p.sqlSources(call.Args[position], p.bindingsBefore(caller, call.Pos()), caller, next, depth+1))
			return true
		})
	}
	if !found {
		sources = threadSQLSources{opaque: true}
	}
	return sources
}

// threadSQLReachKey names a reach set by its members, in order.
func threadSQLReachKey(reached map[string]bool) string {
	names := make([]string, 0, len(reached))
	for name, on := range reached {
		if on {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return strings.Join(names, "\x00")
}
